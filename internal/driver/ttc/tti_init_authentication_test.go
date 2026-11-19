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
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestReadTTIInitAuthenticationResponse consumes negotiation responses and
// decodes the function-specific authentication RPA through the real streamer.
func TestReadTTIInitAuthenticationResponse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		function driverCommon.FunctionType
		payload  func() []byte
		wantType string
	}{
		{name: "password", function: oSesskey, payload: makeOSesskeyRPAPayload, wantType: "*ttc.oSesskeyRPA"},
		{name: "token", function: oauth, payload: makeOauthRPAPayload, wantType: "*ttc.OAuthRPA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			shelf, realStreamer, buffer := newAuthTestShelf(1 << 16)
			// PRO and DTY have already been decoded by the negotiator's callbacks.
			negotiationResponses := list.New()
			negotiationResponses.PushBack(newTTIpro())
			negotiationResponses.PushBack(NewTTIdty())
			streamer := NewWrappedMockStreamer(negotiationResponses, realStreamer)
			writeAuthWarning(t, ctx, buffer, 28002, "password will expire", 0)
			response := append([]byte{byte(TTIRPA)}, tc.payload()...)
			if tc.function == oauth {
				// The OAuth capture includes four capture bytes before its OER header.
				// Feed only TTC messages to the streamer, with the capture's EOCS format.
				oerFrame, err := ExtractBytesFromDump(oerDump)
				if err != nil || !bytes.HasSuffix(response, oerFrame) {
					t.Fatal("OAuth fixture does not end with the expected OER frame")
				}
				response = append(response[:len(response)-len(oerFrame)], byte(TTIOER))
				response = append(response, makeOerPayload(oerDump)...)
				streamer.RegisterPreUnmarshallCallback(TTIOER, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
					return newTTIoer14WithEndOfCallStatusSupport(), nil
				})
			}
			// The golden payload includes OER. Leave the next message untouched.
			response = append(response, byte(TTISTA))
			if err := buffer.WriteBytesWithContext(ctx, response); err != nil {
				t.Fatalf("prepare authentication response: %v", err)
			}

			rpa, err := readTTIInitAuthenticationResponse(ctx, shelf, streamer, tc.function)

			if err != nil {
				t.Fatalf("read authentication response: %v", err)
			}
			if got := fmt.Sprintf("%T", rpa); got != tc.wantType {
				t.Fatalf("RPA type = %s, want %s", got, tc.wantType)
			}
			if negotiationResponses.Len() != 0 {
				t.Error("reader did not consume the negotiation responses")
			}
			if next, err := buffer.ReadByteWithContext(ctx); err != nil || next != byte(TTISTA) {
				t.Fatalf("next message = %d, error = %v; want untouched STA after OER", next, err)
			}
			if realStreamer.preUCallbacks[TTIRPA] != nil {
				t.Error("authentication RPA callback remains registered")
			}
		})
	}
}

// TestReadTTIInitAuthenticationResponseTTIRENEG verifies production decoding of
// the empty marker and preserves the following PRO for normal negotiation.
func TestReadTTIInitAuthenticationResponseTTIRENEG(t *testing.T) {
	ctx := context.Background()
	buffer := NewArrayDataBuffer(4096)
	negotiator := newTTIInitConnectionNegotiator(buffer)
	_, shelf, err := negotiator.Negotiate(ctx)
	if err != nil {
		t.Fatalf("prepare negotiation: %v", err)
	}
	streamer := shelf.GetMessageStreamer().(*MessageStreamer)
	defer unregisterTTIInitNegotiationCallbacks(streamer)
	if err := buffer.WriteBytesWithContext(ctx, []byte{byte(TTIRENEG), byte(TTIPRO)}); err != nil {
		t.Fatalf("prepare renegotiation response: %v", err)
	}

	rpa, err := readTTIInitAuthenticationResponse(ctx, shelf, streamer, oSesskey)

	if !errors.Is(err, driverCommon.ErrTTIRENEG) || rpa != nil {
		t.Fatalf("response = %v, error = %v; want nil and ErrTTIRENEG", rpa, err)
	}
	if next, err := buffer.ReadByteWithContext(ctx); err != nil || next != byte(TTIPRO) {
		t.Fatalf("next message = %d, error = %v; want pending PRO", next, err)
	}
	if streamer.preUCallbacks[TTIRPA] != nil {
		t.Error("authentication RPA callback remains registered after TTIRENEG")
	}
}

// TestReadTTIInitAuthenticationResponseFailure verifies that read errors,
// server rejection, missing RPA, unexpected messages, and invalid state fail.
func TestReadTTIInitAuthenticationResponseFailure(t *testing.T) {
	t.Parallel()
	serverErr := errors.New("authentication rejected")
	for _, tc := range []struct {
		name         string
		messages     []driverCommon.Message[driverCommon.MessageType]
		wantCause    error
		wantCode     oracleErrors.ErrorCode
		invalidState bool
	}{
		{
			name:      "server rejects authentication after RPA",
			messages:  []driverCommon.Message[driverCommon.MessageType]{&OAuthRPA{}, &mockOer{err: serverErr}},
			wantCause: serverErr,
		},
		{
			name:     "successful OER without RPA",
			messages: []driverCommon.Message[driverCommon.MessageType]{&mockOer{}},
			wantCode: oracleErrors.InternalError,
		},
		{
			name:      "response read fails",
			wantCause: io.EOF,
			wantCode:  oracleErrors.AuthenticatorError,
		},
		{
			name:     "unexpected message",
			messages: []driverCommon.Message[driverCommon.MessageType]{&dummyMsg{}},
			wantCode: oracleErrors.InternalError,
		},
		{
			name:         "invalid connection state",
			messages:     []driverCommon.Message[driverCommon.MessageType]{&OAuthRPA{}, &mockOer{}},
			wantCode:     oracleErrors.InternalError,
			invalidState: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shelf := newShelf[driverCommon.MessageType]()
			if tc.invalidState {
				shelf.registerStateValidator(&shelfConnectionValidator{valid: false})
			}
			streamer := &mockStreamer{pullMsgs: tc.messages, pullErr: io.EOF}

			rpa, err := readTTIInitAuthenticationResponse(context.Background(), shelf, streamer, oauth)

			if err == nil || rpa != nil {
				t.Fatalf("response = %v, error = %v; want nil and an error", rpa, err)
			}
			if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
				t.Fatalf("error = %v, want cause %v", err, tc.wantCause)
			}
			if tc.wantCode != "" {
				sqlErr, ok := err.(oracleErrors.SQLError)
				if !ok || sqlErr.ErrorCode() != string(tc.wantCode) {
					t.Fatalf("error = %v, want code %s", err, tc.wantCode)
				}
			}
		})
	}
}
