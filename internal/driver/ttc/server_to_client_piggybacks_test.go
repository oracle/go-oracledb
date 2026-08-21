/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package ttc

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Test that the SPF message is handled by the registered post unmarshal call back
//  1. create a buffer contain a SPF message and a mocked TTIPRO message
//  2. create and populate the shelf
//  3. create registry, register messages and create factory
//  4. Pull the TTIPRO, this should cause the SPF to be treated by the callback
//     and the TTIPRO to be returned. No message should be left in the streamer
//  5. Pull the SPF, this should fail, there are no more messages, and the mock
//     data buffer will return an EOF error.
func TestRegisterServerToClientPiggybacks(t *testing.T) {
	t.Parallel()
	// Build payload from captured dump
	buf, err := ExtractBytesFromDump(validttispfUnmarshalDump)
	if err != nil {
		t.Fatalf("ExtractBytesFromDump failed: %v", err)
	}
	if len(buf) == 0 {
		t.Fatal("validttispfUnmarshalDump produced empty payload")
	}
	// Use universal for UB2/UB4 as in other tests to exercise _decodeUniversal path
	data := NewArrayDataBuffer(len(buf) + 64)
	// Add header for SPF message
	data.WriteByteWithContext(context.Background(), byte(TTISPF))
	data.WriteByteWithContext(context.Background(), ocssync)
	// Add Payload
	if werr := data.WriteBytesWithContext(context.Background(), buf); werr != nil {
		t.Fatalf("failed to seed buffer: %v", werr)
	}
	// Add TTIPRO header, and UB2 payload for mock message should be 3 UB2 of value 11, 12 and 13
	data.WriteByteWithContext(context.Background(), byte(TTIPRO))
	data.WriteBytesWithContext(context.Background(), []byte{0x01, 0x0B, 0x01, 0x0C, 0x01, 0x0D})
	// Register mock message for TTIRPRO
	engine := NewMarshalEngine(data, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	// create factory and register messages
	messageRegistry := NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIPRO, -1, NewMockTTIpro)
	functionRegistry := NewRegistry[functionRegistryKey]()
	functionRegistry.Register(functionRegistryKey{messageType: TTISPF, functionType: common.FunctionType(ocssync)}, -1, newttiSPFOCSSync)
	messageFactory := &SimpleFactory{msgregistry: messageRegistry, funcregistry: functionRegistry, ttcVersion: 25}

	shelf := newShelf[common.MessageType]()
	sessionCtx := common.NewSessionContext()
	messageStreamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(messageStreamer)
	shelf.RegisterMarshaller(engine)
	shelf.RegisterMessageFactory(messageFactory)

	registerServerToClientPiggybacks(shelf, sessionCtx)

	msgTTIPRO, err := messageStreamer.Pull(context.Background(), TTIPRO)
	if err != nil {
		t.Fatalf("Unexpected error, %v", err)
	}
	if msgTTIPRO.GetMsgCode() != TTIPRO {
		t.Fatalf("Unexpected message type, expected TTIPRO, but was %d", msgTTIPRO.GetMsgCode())
	}

	_, err = messageStreamer.Pull(context.Background(), TTIPRO)
	if err == nil {
		t.Fatalf("Streamer should throw error, if SPF was correcly handled by callback, mock data buffer returns EOR")
	}

}

// Test the OCSSYNC piggyback is correctly handled by the post unmarshal call
// back and that the session is correctly updated
func TestHandleServerToClientPiggyback_ValidOCSSYNC(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	sessionCtx := common.NewSessionContext()
	updater := serverToClientPiggybackUpdater{
		shelf:      shelf,
		sessionCtx: sessionCtx,
	}

	mockMsg := newttiSPFOCSSync()
	mockMsg.(*ttiSPFOCSSync).keyValueArr = &keywordValueArray{
		{keyword: 0, binaryValue: dynamicAllocatedArray{value: []byte("USD")}},
	}

	handled, err := updater.handleServerToClientPiggyback(mockMsg, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if handled {
		t.Error("Expected handled to be false")
	}

	// Check if session properties were updated
	props := sessionCtx.GetSessionProperties()
	val := props.GetProperty(authNlsLxcCurrency)
	if val == nil || val != "USD" {
		t.Errorf("Expected property %s to be USD, got %v", authNlsLxcCurrency, val)
	}
}

// Test that the correct error is returned when the SPF message does not
// implement the function interface
func TestHandleServerToClientPiggyback_NotFunction(t *testing.T) {
	t.Parallel()
	updater := serverToClientPiggybackUpdater{}

	mockMsg := &dummyMsg{} // Not implementing Function

	handled, err := updater.handleServerToClientPiggyback(mockMsg, nil)
	if handled {
		t.Error("Expected handled to be false")
	}
	if err == nil {
		t.Errorf("Expected error not returned")
	}

	oracleErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Errorf("Expected oracle error but got standard other error type")
	}

	if oracleErr.ErrorCode() != string(oracleErrors.SPFNotFunction) {
		t.Errorf("Expected error to be UnknownSPFFunction: %s, but was %s", oracleErrors.SPFNotFunction, oracleErr.ErrorCode())
	}
}

// Test that the correct error is returned when the SPF message implements and
// unknown function type
func TestHandleServerToClientPiggyback_UnknownFunction(t *testing.T) {
	t.Parallel()
	updater := serverToClientPiggybackUpdater{}

	mockMsg := &mockFunction{funcCode: common.FunctionType(0xFF)} // Unknown, assuming FunctionType is uint8

	handled, err := updater.handleServerToClientPiggyback(mockMsg, nil)
	if handled {
		t.Error("Expected handled to be false")
	}
	if err == nil {
		t.Errorf("Expected error not returned")
	}

	oracleErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Errorf("Expected oracle error but got standard other error type")
	}

	if oracleErr.ErrorCode() != string(oracleErrors.UnknownSPFFunction) {
		t.Errorf("Expected error to be UnknownSPFFunction: %s, but was %s", oracleErrors.UnknownSPFFunction, oracleErr.ErrorCode())
	}
}

// Test that unmarshalling errors are handled correctly by the post unmarshal
// call back
func TestHandleServerToClientPiggyback_ErrorInMessage(t *testing.T) {
	t.Parallel()
	updater := serverToClientPiggybackUpdater{}

	mockMsg := &mockFunction{funcCode: common.FunctionType(ocssync)}

	handled, err := updater.handleServerToClientPiggyback(mockMsg, errors.New("test error"))
	if handled {
		t.Error("Expected handled to be false")
	}
	if err == nil || err.Error() != "test error" {
		t.Errorf("Expected test error, got %v", err)
	}
}

// mockFunction implements common.Function for testing
type mockFunction struct {
	common.Message[common.MessageType]
	funcCode common.FunctionType
}

func (m *mockFunction) GetFuncCode() common.FunctionType {
	return m.funcCode
}
