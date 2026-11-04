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
)

func negotiateCookieForTest(t *testing.T) (*driverCommon.SessionContext, *MessageStreamer, *ArrayBasedDataBuffer) {
	t.Helper()
	pro := newTTIpro().(*tTIpro)
	pro.proSvrVer, pro.svrCharSet, pro.nCharCharset = 6, 873, 2000
	buffer := NewArrayDataBuffer(4096)
	negotiator := newCookieConnectionNegotiator(buffer, newTTICookieFromTTIpro(pro))
	sessCtx, shelf, returnedCookie, err := negotiator.Negotiate(context.Background())
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if sessCtx == nil || shelf == nil || returnedCookie != nil {
		t.Fatal("unexpected cookie negotiation result")
	}
	return sessCtx, shelf.GetMessageStreamer().(*MessageStreamer), buffer
}

// TestCookieNegotiationQueuesMessages verifies that PRO, COOKIE, and DTY
// stay queued in order without reading, writing, or flushing the connection buffer.
func TestCookieNegotiationQueuesMessages(t *testing.T) {
	pro := newTTIpro().(*tTIpro)
	pro.proSvrVer, pro.svrCharSet, pro.nCharCharset = 6, 873, 2000
	buffer := NewArrayDataBuffer(4096)
	negotiator := newCookieConnectionNegotiator(buffer, newTTICookieFromTTIpro(pro))

	sessCtx, shelf, returnedCookie, err := negotiator.Negotiate(context.Background())

	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if sessCtx == nil || shelf == nil || returnedCookie != nil {
		t.Fatal("unexpected cookie negotiation result")
	}
	streamer := shelf.GetMessageStreamer().(*MessageStreamer)
	if buffer.currentWritePosition != 0 || buffer.currentReadPosition != 0 || buffer.hasBeenFlushed {
		t.Fatal("cookie negotiation performed I/O before authentication")
	}
	var got []driverCommon.MessageType
	for entry := streamer.outgoingMessages.Front(); entry != nil; entry = entry.Next() {
		got = append(got, entry.Value.(driverCommon.Message[driverCommon.MessageType]).GetMsgCode())
	}
	want := []driverCommon.MessageType{TTIPRO, TTICOOKIE, TTIDTY}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queued message types = %v, want %v", got, want)
	}
}

// TestCookieNegotiationUsesCachedSettings verifies that cached capabilities
// prepare the outgoing DTY and configure the session's NCHAR character set.
func TestCookieNegotiationUsesCachedSettings(t *testing.T) {
	pro := newTTIpro().(*tTIpro)
	pro.proSvrVer, pro.svrCharSet, pro.nCharCharset = 6, 873, 2000
	buffer := NewArrayDataBuffer(4096)
	negotiator := newCookieConnectionNegotiator(buffer, newTTICookieFromTTIpro(pro))

	sessCtx, shelf, _, err := negotiator.Negotiate(context.Background())

	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	streamer := shelf.GetMessageStreamer().(*MessageStreamer)
	preparedPRO := streamer.outgoingMessages.Front().Value.(*tTIpro)
	dty := streamer.outgoingMessages.Back().Value.(*tTIdty)
	if !reflect.DeepEqual(dty.negotiatedCapsMap, preparedPRO.clientCaps.toMap()) {
		t.Fatal("queued DTY does not use the prepared PRO's client capabilities")
	}
	if got := sessCtx.SessionNCharCharacterSet(); got != 2000 {
		t.Fatalf("NCHAR character set = %d, want 2000", got)
	}
	if got := shelf.GetCapabilities()[kpccapCtTtcFldVsn].Value; got != ttcFldVsn231 {
		t.Fatalf("server TTC capability = %d, want %d", got, ttcFldVsn231)
	}
}

// TestCookieNegotiationSelectsMarshallers verifies native encoding for PRO,
// COOKIE, and DTY, and the default universal encoding for authentication.
func TestCookieNegotiationSelectsMarshallers(t *testing.T) {
	pro := newTTIpro().(*tTIpro)
	pro.proSvrVer, pro.svrCharSet, pro.nCharCharset = 6, 873, 2000
	buffer := NewArrayDataBuffer(4096)
	negotiator := newCookieConnectionNegotiator(buffer, newTTICookieFromTTIpro(pro))

	_, shelf, _, err := negotiator.Negotiate(context.Background())

	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	for _, tc := range []struct {
		messageType driverCommon.MessageType
		wantTypeRep byte
	}{
		{TTIPRO, Native},
		{TTICOOKIE, Native},
		{TTIDTY, Native},
		{TTIFUN, Universal},
	} {
		marshaller := shelf.GetMarshallerForMessageType(tc.messageType).(*MarshalEngine)
		if got := marshaller._numericTypeRep[UB2Index].typeRep; got != tc.wantTypeRep {
			t.Errorf("message %v: UB2 representation = %d, want %d", tc.messageType, got, tc.wantTypeRep)
		}
		if marshaller._dataBuffer != buffer {
			t.Errorf("message %v: marshaller does not use the connection buffer", tc.messageType)
		}
	}
	if shelf.GetMarshallerForMessageType(TTIFUN) != shelf.GetMarshaller() {
		t.Error("authentication does not use the default marshaller")
	}
}

// TestCookieNegotiationPreparesDTYResponse verifies that the pre-decode callback
// supplies the client capabilities needed to decode the server's DTY.
func TestCookieNegotiationPreparesDTYResponse(t *testing.T) {
	_, streamer, _ := negotiateCookieForTest(t)
	callback := streamer.preUCallbacks[TTIDTY]
	if callback == nil {
		t.Fatal("DTY pre-decode callback was not registered")
	}
	msg, err := callback(&messageHeader{messageType: TTIDTY})
	if err != nil {
		t.Fatalf("prepare DTY response: %v", err)
	}
	dty, ok := msg.(*tTIdty)
	if !ok {
		t.Fatalf("response type = %T, want *tTIdty", msg)
	}
	pro := streamer.outgoingMessages.Front().Value.(*tTIpro)
	if !reflect.DeepEqual(dty.negotiatedCapsMap, pro.clientCaps.toMap()) {
		t.Fatal("response DTY does not use the prepared PRO's client capabilities")
	}
}

// TestCookieNegotiationCompletesDTYResponse verifies that timezone state changes
// only after a successful DTY decode and that decode errors are preserved.
func TestCookieNegotiationCompletesDTYResponse(t *testing.T) {
	// Distinct sample versions let us detect whether the callback updated the session.
	const (
		existingTimeZoneVersion byte = 3
		serverTimeZoneVersion   byte = 77
	)
	decodeErr := errors.New("datatype decode failed")
	for _, tc := range []struct {
		name                string
		decodeErr           error
		wantTimeZoneVersion byte
	}{
		{
			name:                "successful decode installs server timezone version",
			wantTimeZoneVersion: serverTimeZoneVersion,
		},
		{
			name:                "failed decode preserves existing timezone version",
			decodeErr:           decodeErr,
			wantTimeZoneVersion: existingTimeZoneVersion,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessCtx, streamer, _ := negotiateCookieForTest(t)
			// The session already has a version; DTY supplies a different one.
			sessCtx.SetTimeZoneVersionNumber(existingTimeZoneVersion)
			dtyResponse := &tTIdty{timeZoneVersionNumber: serverTimeZoneVersion}
			callback := streamer.postUCallbacks[TTIDTY]
			if callback == nil {
				t.Fatal("DTY post-decode callback was not registered")
			}

			// Simulate the streamer calling this callback after decoding DTY.
			keep, err := callback(dtyResponse, tc.decodeErr)

			if err != tc.decodeErr {
				t.Fatalf("DTY callback error = %v, want %v", err, tc.decodeErr)
			}
			if !keep {
				t.Fatal("DTY callback discarded the response")
			}
			// Install the server version only on success; a failed decode must not change session state.
			if got := sessCtx.GetClientProperties().GetProperty(driverCommon.TimeZoneVersionNumber); got != tc.wantTimeZoneVersion {
				t.Fatalf("timezone version = %v, want %d", got, tc.wantTimeZoneVersion)
			}
		})
	}
}

// TestUnregisterCookieNegotiationCallbacks verifies that cleanup removes only
// the cookie negotiation callbacks, preserving authentication callbacks.
func TestUnregisterCookieNegotiationCallbacks(t *testing.T) {
	_, streamer, _ := negotiateCookieForTest(t)
	if streamer.preUCallbacks[TTIDTY] == nil || streamer.postUCallbacks[TTIDTY] == nil {
		t.Fatal("cookie DTY callbacks were not registered")
	}
	streamer.RegisterPreUnmarshallCallback(TTIRPA, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) { return nil, nil })
	unregisterCookieNegotiationCallbacks(streamer)
	if streamer.preUCallbacks[TTIDTY] != nil || streamer.postUCallbacks[TTIDTY] != nil {
		t.Fatal("cookie DTY callbacks were not removed")
	}
	if streamer.preUCallbacks[TTIRPA] == nil {
		t.Fatal("cookie cleanup removed an unrelated authentication callback")
	}
}
