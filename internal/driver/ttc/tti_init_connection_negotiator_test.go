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
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestTTIInitNegotiationQueuesMessages verifies queue order, initial client settings,
// and that negotiation performs no I/O before authentication sends the batch.
func TestTTIInitNegotiationQueuesMessages(t *testing.T) {
	buffer := NewArrayDataBuffer(4096)
	negotiator := newTTIInitConnectionNegotiator(buffer, nil)

	_, shelf, err := negotiator.Negotiate(context.Background())

	if err != nil {
		t.Fatalf("Negotiate failed: %v", err)
	}
	streamer := shelf.GetMessageStreamer().(*MessageStreamer)
	defer unregisterTTIInitNegotiationCallbacks(streamer)
	if buffer.currentWritePosition != 0 || buffer.currentReadPosition != 0 || buffer.hasBeenFlushed {
		t.Fatal("negotiation performed I/O before authentication")
	}
	var got []driverCommon.MessageType
	for entry := streamer.outgoingMessages.Front(); entry != nil; entry = entry.Next() {
		got = append(got, entry.Value.(driverCommon.Message[driverCommon.MessageType]).GetMsgCode())
	}
	if want := []driverCommon.MessageType{TTIINIT, TTIDTY}; !reflect.DeepEqual(got, want) {
		t.Fatalf("queued message types = %v, want %v", got, want)
	}
	dty := streamer.outgoingMessages.Back().Value.(*tTIdty)
	if dty.negotiatedCapsMap[kpccapRtTzEx].IsSet {
		t.Fatal("initial DTY requested timezone exchange")
	}
}

// TestTTIInitNegotiationProcessesResponses verifies that real PRO/DTY install
// server settings and leave the next message readable.
func TestTTIInitNegotiationProcessesResponses(t *testing.T) {
	ctx := context.Background()
	buffer := NewArrayDataBuffer(4096)
	negotiator := newTTIInitConnectionNegotiator(buffer, nil)
	sessCtx, shelf, err := negotiator.Negotiate(ctx)
	if err != nil {
		t.Fatalf("Negotiate failed: %v", err)
	}
	streamer := shelf.GetMessageStreamer().(*MessageStreamer)
	defer unregisterTTIInitNegotiationCallbacks(streamer)
	server := newCapability()
	// This PRO advertises field version 17 and TTC3 = 0x7f with timezone exchange enabled.
	response := defaultTTIproSuccessPayload(6, true, true, server.compileTimeCapabilities, server.runTimeCapabilities)
	response = append(response, byte(TTIDTY))
	// Empty datatype table, followed by a message with no body.
	response = append(response, 0, 0, byte(TTIFOB))
	if err := buffer.WriteBytesWithContext(ctx, response); err != nil {
		t.Fatalf("prepare server response: %v", err)
	}

	msg, err := streamer.Pull(ctx, TTIPRO, TTIDTY, TTIFOB)
	if err != nil {
		t.Fatalf("read through DTY reply: %v", err)
	}
	if msg.GetMsgCode() != TTIFOB {
		t.Fatalf("got message %v after negotiation, want TTIFOB", msg.GetMsgCode())
	}

	if got := shelf.GetCodecFactory().(*CodecFactoryImpl).ttcVersion; got != int8(ttcFldVsn231) {
		t.Errorf("confirmed codec version = %d, want %d", got, ttcFldVsn231)
	}
	// The existing PRO fixture supplies NCHAR character set 775 in its FDO.
	if got := sessCtx.SessionNCharCharacterSet(); got != 775 {
		t.Errorf("confirmed NCHAR character set = %d, want 775", got)
	}
}

// TestTTIInitNegotiationDecodeErrors verifies that the authentication reader returns PRO and DTY decode errors.
func TestTTIInitNegotiationDecodeErrors(t *testing.T) {
	for _, messageType := range []driverCommon.MessageType{TTIPRO, TTIDTY} {
		t.Run(TTCMsgTypeDescription[messageType], func(t *testing.T) {
			ctx := context.Background()
			buffer := NewArrayDataBuffer(4096)
			negotiator := newTTIInitConnectionNegotiator(buffer, nil)
			_, shelf, err := negotiator.Negotiate(ctx)
			if err != nil {
				t.Fatalf("Negotiate failed: %v", err)
			}
			streamer := shelf.GetMessageStreamer().(*MessageStreamer)
			defer unregisterTTIInitNegotiationCallbacks(streamer)

			response := []byte{byte(TTIPRO)} // PRO missing its body.
			if messageType == TTIDTY {
				response = DefaultTTIproSuccessPayloadWithEOCS(6)
				response = append(response, byte(TTIDTY)) // DTY missing its datatype table.
			}
			if err := buffer.WriteBytesWithContext(ctx, response); err != nil {
				t.Fatalf("prepare response: %v", err)
			}

			_, err = readTTIInitAuthenticationResponse(ctx, shelf, streamer, oSesskey)

			cause, ok := errors.Unwrap(err).(oracleErrors.SQLError)
			if !ok || cause.ErrorCode() != string(oracleErrors.FailUnmarshal) {
				t.Fatalf("authentication error = %v, want a negotiation message decode failure", err)
			}
		})
	}
}
