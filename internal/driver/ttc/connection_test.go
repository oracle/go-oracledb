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
	"container/list"
	"context"
	"fmt"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func init() {
	message.SetString(language.French, string(oracleErrors.InternalError), "erreur interne factice.")
	message.SetString(language.French, string(oracleErrors.StatementExecutionFailed), "echec factice de preparation de %s: %s.")
}

func TestConnection_ParseTimeZoneRejectsMalformedValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{name: "valid positive timezone", input: "+05:30", wantValid: true},
		{name: "valid negative timezone", input: "-08:15", wantValid: true},
		{name: "empty timezone", input: "", wantValid: false},
		{name: "sign only timezone", input: "+", wantValid: false},
		{name: "missing separator", input: "0530", wantValid: false},
		{name: "missing minute field", input: "+05:", wantValid: false},
		{name: "non numeric values", input: "+ab:cd", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parseTimeZone unexpectedly panicked for input %q: %v", tt.input, r)
				}
			}()

			h, m, err := parseTimeZone(tt.input)
			if tt.wantValid {
				if err != nil {
					t.Fatalf("parseTimeZone(%q) should be valid: %v", tt.input, err)
				}
				fmt.Printf("PASS: accepted valid input %q -> (%d, %d)\n", tt.input, h, m)
				return
			}

			if err == nil {
				t.Fatalf("parseTimeZone(%q) should reject malformed input", tt.input)
			}
			if sqlErr, ok := err.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.ServerTimeZoneError) {
				t.Fatalf("parseTimeZone(%q) should return %s, got %T: %v", tt.input, oracleErrors.ServerTimeZoneError, err, err)
			}
			fmt.Printf("PASS: rejected malformed input %q with error: %v\n", tt.input, err)
		})
	}
}

func TestNewConnectionReturnsServerTimezoneError(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})
	shelf.RegisterMessageStreamer(&mockStreamer{
		pullMsg: &mockOer{err: common.NewOERMessageError("ORA-12345", "timezone query failed")},
	})
	shelf.RegisterLocalizationService(common.NewLocalizationService(language.English))

	conn, err := newConnection(context.Background(), shelf, driverCommon.NewSessionContext(), &mockNetworkSession{})
	if err == nil {
		t.Fatal("expected server timezone initialization error")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected common.SQLError, got %T", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ServerTimeZoneError) {
		t.Fatalf("expected error code %s, got %s", oracleErrors.ServerTimeZoneError, sqlErr.ErrorCode())
	}
	if conn != nil {
		t.Fatalf("expected nil connection, got %T", conn)
	}
}

func TestConnection_ExecContext_LocalizesError(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})
	shelf.RegisterMessageStreamer(&mockStreamer{pullMsg: &mockOer{err: nil}})
	shelf.RegisterLocalizationService(common.NewLocalizationService(language.French))
	conn := newTestConnection(shelf, nil, &mockNetworkSession{})

	_, err := conn.ExecContext(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected common.SQLError, got %T", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.StatementExecutionFailed) {
		t.Fatalf("expected error code %s, got %s", oracleErrors.StatementExecutionFailed, sqlErr.ErrorCode())
	}
	if got, want := err.Error(), "OGD-00050 - echec factice de preparation de query: empty SQL."; got != want {
		t.Fatalf("unexpected localized error %q, want %q", got, want)
	}
}

func TestConnection_QueryContext_LocalizesError(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})
	shelf.RegisterMessageStreamer(&mockStreamer{pullMsg: &mockOer{err: nil}})
	shelf.RegisterLocalizationService(common.NewLocalizationService(language.French))
	conn := newTestConnection(shelf, nil, &mockNetworkSession{})

	_, err := conn.QueryContext(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected common.SQLError, got %T", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.StatementExecutionFailed) {
		t.Fatalf("expected error code %s, got %s", oracleErrors.StatementExecutionFailed, sqlErr.ErrorCode())
	}
	if got, want := err.Error(), "OGD-00050 - echec factice de preparation de query: empty SQL."; got != want {
		t.Fatalf("unexpected localized error %q, want %q", got, want)
	}
}

func TestConnection_LocalizationStaysBoundToEachShelf(t *testing.T) {
	t.Parallel()
	newConn := func(lang language.Tag) *connection {
		shelf := newShelf[driverCommon.MessageType]()
		shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})
		shelf.RegisterMessageStreamer(&mockStreamer{pullMsg: &mockOer{err: nil}})
		shelf.RegisterLocalizationService(common.NewLocalizationService(lang))
		return newTestConnection(shelf, nil, &mockNetworkSession{})
	}

	englishConn := newConn(language.English)
	englishErr := func() error {
		_, err := englishConn.ExecContext(context.Background(), "", nil)
		return err
	}()
	if englishErr == nil {
		t.Fatal("expected an English localization error, got nil")
	}
	if got, want := englishErr.Error(), "OGD-00050 - failed to prepare query statement: empty SQL."; got != want {
		t.Fatalf("unexpected English localized error %q, want %q", got, want)
	}

	frenchConn := newConn(language.French)
	frenchErr := func() error {
		_, err := frenchConn.ExecContext(context.Background(), "", nil)
		return err
	}()
	if frenchErr == nil {
		t.Fatal("expected a French localization error, got nil")
	}
	if got, want := frenchErr.Error(), "OGD-00050 - failed to prepare query statement: empty SQL."; got == want {
		t.Fatalf("expected French localization to differ from English, but got %q", got)
	}

	if got, want := englishErr.Error(), "OGD-00050 - failed to prepare query statement: empty SQL."; got != want {
		t.Fatalf("English error changed after French service was used: got %q, want %q", got, want)
	}
}

func TestConnection_InvalidateOnOEROrSTA(t *testing.T) {
	t.Parallel()
	shelf := newShelf[driverCommon.MessageType]()
	// Create mock factory that will always return the expected message for the test
	mockFac := &mockFactory{}
	// Use mockFactoryWithList at startup to bypass server time zone
	mockFactoryWithList := &mockFactoryWithList{
		returnMsg: []driverCommon.Message[driverCommon.MessageType]{
			NewOall18(),
			&mockOer{err: common.NewOERMessageError("ORA-12345", "dont read server time zone")}},
	}
	// Initialize the buffer will the message headers
	databuffer := &ArrayBasedDataBuffer{
		bytes:                []byte{byte(TTIOER), byte(TTIOER), byte(TTISTA), byte(TTISTA)},
		currentReadPosition:  0,
		currentWritePosition: 4,
	}
	marshaller := NewMarshalEngine(databuffer, driverCommon.BIG_ENDIAN, newTypeRep().nativeTypesRepresentation)
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMarshaller(marshaller)
	shelf.RegisterMessageFactory(mockFactoryWithList)
	shelf.RegisterMessageStreamer(streamer)

	connection := newTestConnection(shelf, nil, nil)

	// Reset to factory that will return the expected message to the test
	shelf.RegisterMessageFactory(mockFac)

	tests := []connInvalidationMsg{
		{
			msgType:                   TTIOER,
			connectionShouldBeDropped: false,
		},
		{
			msgType:                   TTIOER,
			connectionShouldBeDropped: true,
		},
		{
			msgType:                   TTISTA,
			connectionShouldBeDropped: false,
		},
		{
			msgType:                   TTISTA,
			connectionShouldBeDropped: true,
		},
	}

	for _, connInvalidationMsg := range tests {
		t.Run(fmt.Sprintf("Running with %v", connInvalidationMsg), func(t *testing.T) {
			// The factory will return the expected message
			mockFac.returnMsg = &connInvalidationMsg

			// pull message from streamer to trigger callback
			msg, err := streamer.Pull(context.Background(), connInvalidationMsg.msgType)
			if err != nil {
				t.Fatalf("Unexpected error occurred while pulling message %v", err)
			}
			if msg.GetMsgCode() != connInvalidationMsg.msgType {
				t.Fatalf("Unexpected message type, expected %d, but was %d", connInvalidationMsg.msgType, msg.GetMsgCode())
			}
			connectionStatus, ok := msg.(connectionStatusProvider)
			if !ok {
				t.Fatal("Message should implement connectionStatusReceiver")
			}
			if connection._isValid != !connectionStatus.isBeingDrainned() {
				t.Fatalf("isValid should be %t, but was %t", !connectionStatus.isBeingDrainned(), connection._isValid)
			}
		})
	}
}

// corrupted streamer test.
// 1 - create a data buffer marshaller , shelf etc..
// 2 - create a NewWrappedMockStreamer with TTIiov message in the incoming queue
// 3 - pre-populate the data buffer with SELECT statement bits
// 4 - Create a connection
// 5 - perform the SELECT stmt
// expectations:
//   - The select statement should fail as the underlying streamer incoming queue is not empty
//   - the connection is marked as faulty
func TestConnection_FaultyOnDrain(t *testing.T) {

	ctx := context.Background()
	shelf, streamer, dbuf := newExecTestShelf(8192)

	incomingMsgL := list.New()
	incomingMsgL.PushBack(&mockOer{err: common.NewOERMessageError("ORA-12345", "dont read server time zone")})

	// these one won't get pulled by anyone.
	// this will make the select call to fail
	incomingMsgL.PushBack(newTTIiov())

	mStreamer := NewWrappedMockStreamer(incomingMsgL, streamer)

	shelf.RegisterMessageStreamer(mStreamer)
	shelf.registerStateValidator(mStreamer)

	// Pre-seed incoming wire with the full valid SELECT query stream dump.
	stream := Oall8Payload(validSelectQueryDump)
	if len(stream) == 0 {
		t.Fatal("validSelectQueryDump decode returned empty")
	}
	// Minimal priming: DCB header to start, then rest of stream which contains DCB/RXD/.. and OER-01403
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIDCB)); err != nil {
		t.Fatalf("write TTIDCB header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, stream); err != nil {
		t.Fatalf("write validSelectQueryDump stream failed: %v", err)
	}

	factory2 := &testCodecFactory{
		encode:    nil,
		decode:    nil,
		bindOac:   nil,
		defineOac: nil,
	}

	shelf.RegisterCodecFactory(factory2)

	mockNs := &mockNetworkSession{
		disconnectCalls: 0,
		disconnectErr:   nil,
		sleepDuration:   0,
		cancelErr:       nil,
	}

	connection := newTestConnection(shelf, driverCommon.NewSessionContext(), mockNs)

	_, err := connection.QueryContext(context.Background(), "select * from DUAL", nil)
	if err == nil {
		t.Fatalf("expected error not raised")
	}
	if connection.IsValid() != false {
		t.Fatalf("Connection should not be valid")
	}

}

// corrupted streamer test.
// 1 - create a data buffer marshaller , shelf etc..
// 2 - create a NewWrappedMockStreamer with TTIiov message in the incoming queue
// 3 - pre-populate the data buffer with SELECT statement bits
// 4 - Create a statement
// 5 - perform the SELECT stmt
// expectations:
//   - The select statement should fail as the underlying streamer incoming queue is not empty
//   - the connection is marked as faulty
func TestConnection_FaultyOnDrainInStatement(t *testing.T) {

	ctx := context.Background()
	shelf, streamer, dbuf := newExecTestShelf(8192)

	incomingMsgL := list.New()
	incomingMsgL.PushBack(&mockOer{err: common.NewOERMessageError("ORA-12345", "dont read server time zone")})

	// these one won't get pulled by anyone.
	// this will make the select call to fail
	incomingMsgL.PushBack(newTTIiov())

	mStreamer := NewWrappedMockStreamer(incomingMsgL, streamer)

	shelf.RegisterMessageStreamer(mStreamer)
	shelf.registerStateValidator(mStreamer)

	// Pre-seed incoming wire with the full valid SELECT query stream dump.
	stream := Oall8Payload(validSelectQueryDump)
	if len(stream) == 0 {
		t.Fatal("validSelectQueryDump decode returned empty")
	}
	// Minimal priming: DCB header to start, then rest of stream which contains DCB/RXD/.. and OER-01403
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIDCB)); err != nil {
		t.Fatalf("write TTIDCB header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, stream); err != nil {
		t.Fatalf("write validSelectQueryDump stream failed: %v", err)
	}

	factory2 := &testCodecFactory{
		encode:    nil,
		decode:    nil,
		bindOac:   nil,
		defineOac: nil,
	}

	shelf.RegisterCodecFactory(factory2)

	mockNs := &mockNetworkSession{
		disconnectCalls: 0,
		disconnectErr:   nil,
		sleepDuration:   0,
		cancelErr:       nil,
	}

	connection := newTestConnection(shelf, driverCommon.NewSessionContext(), mockNs)

	_, err := connection.QueryContext(context.Background(), "select * from DUAL", nil)
	if err == nil {
		t.Fatalf("expected error not raised")
	}
	if connection.IsValid() != false {
		t.Fatalf("Connection should not be valid")
	}

	stmt, err := newStatement(connection.shelf, connection.sessCtx, "select * from DUAL")
	defer func() { _ = stmt.Close() }()
	if err != nil {
		t.Fatalf("Cannot create statement: %v", err)
	}
	_, err = stmt.Query(nil)
	if err == nil {
		t.Fatalf("expected error not raised")
	}
	if connection.IsValid() != false {
		t.Fatalf("Connection should not be valid")
	}

}

type connInvalidationMsg struct {
	msgType                   driverCommon.MessageType
	connectionShouldBeDropped bool
}

func (m *connInvalidationMsg) GetMsgCode() driverCommon.MessageType { return m.msgType }
func (m *connInvalidationMsg) UnMarshalFrom(_ context.Context, _ driverCommon.Marshaller) error {
	return nil
}
func (m *connInvalidationMsg) isBeingDrainned() bool {
	return m.connectionShouldBeDropped
}
