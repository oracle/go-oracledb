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

// TestTTIInitNegotiationQueuesMessages verifies queue order, initial client settings,
// and that negotiation performs no I/O before authentication sends the batch.
func TestTTIInitNegotiationQueuesMessages(t *testing.T) {
	buffer := NewArrayDataBuffer(4096)
	negotiator := newTTIInitConnectionNegotiator(buffer, nil)

	sessCtx, shelf, err := negotiator.Negotiate(context.Background())

	if err != nil {
		t.Fatalf("Negotiate failed: %v", err)
	}
	if sessCtx == nil || shelf == nil {
		t.Fatal("Negotiate did not return session resources")
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
	init := streamer.outgoingMessages.Front().Value.(*ttiInitMessage)
	dty := streamer.outgoingMessages.Back().Value.(*tTIdty)
	if !reflect.DeepEqual(dty.negotiatedCapsMap, init._protocolMessage.clientCaps.toMap()) {
		t.Error("DTY does not use the prepared client capabilities")
	}
	initialFactory := shelf.GetMessageFactory().(*SimpleFactory)
	if got := initialFactory.ttcVersion; got != int8(init._fieldVersion) {
		t.Errorf("initial factory version = %d, envelope version = %d", got, init._fieldVersion)
	}
	oer, err := initialFactory.GetMessage(TTIOER)
	if err != nil {
		t.Fatalf("create initial OER decoder: %v", err)
	}
	if got, want := oer.(*tTIoer14)._supportsEndOfCallStatus, dty.negotiatedCapsMap[kpccapCtbTtc1Eocs].IsSet; got != want {
		t.Errorf("initial OER end-of-call status support = %t, advertised = %t", got, want)
	}
	if shelf.GetCapabilities() != nil {
		t.Error("server capabilities were installed before receiving PRO")
	}
	if shelf.GetCodecFactory() != nil {
		t.Error("session codec factory was installed before datatype negotiation completed")
	}
	if got := sessCtx.DriverCharacterSet(); got != 0 {
		t.Errorf("initial driver character set = %d, want unset", got)
	}
	defaultCaps := newDefaultCapability()
	if !reflect.DeepEqual(init._protocolMessage.clientCaps.compileTimeCapabilities, defaultCaps.compileTimeCapabilities) {
		t.Error("TTIINIT changed the default client compile-time capabilities")
	}
	if !reflect.DeepEqual(init._protocolMessage.clientCaps.runTimeCapabilities, defaultCaps.runTimeCapabilities) {
		t.Error("TTIINIT changed the default client runtime capabilities")
	}
	if got := sessCtx.SessionNCharCharacterSet(); got != 0 {
		t.Errorf("initial NCHAR character set = %d, want unset", got)
	}
}

// TestTTIInitNegotiationProcessesResponses verifies that real PRO/DTY install
// server settings while DTY decoding retains the capabilities already sent.
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
	sentDTY := streamer.outgoingMessages.Back().Value.(*tTIdty)
	server := newCapability()
	fieldVersion := server.knownUsedCompileTimeCapabilities[kpccapCtTtcFldVsn]
	server.compileTimeCapabilities[fieldVersion.index] = ttcFldVsn234
	const serverTimezoneVersion byte = 42
	response := defaultTTIproSuccessPayload(6, true, true, server.compileTimeCapabilities, server.runTimeCapabilities)
	response = append(response, byte(TTIDTY))
	// The default client capabilities request the timezone interval and its four-byte version.
	response = append(response, driverCommon.GetTimeZoneBytes()...)
	response = append(response, 0, 0, 0, serverTimezoneVersion)
	// Empty datatype table, followed by a message with no body.
	response = append(response, 0, 0, byte(TTIFOB))
	if err := buffer.WriteBytesWithContext(ctx, response); err != nil {
		t.Fatalf("prepare server response: %v", err)
	}

	prepareDTY := streamer.preUCallbacks[TTIDTY]
	streamer.RegisterPreUnmarshallCallback(TTIDTY, func(header *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		if got := shelf.GetMessageFactory().(*SimpleFactory).ttcVersion; got != int8(ttiInitInitialFieldVersion) {
			t.Errorf("factory version before DTY = %d, want initial version %d", got, ttiInitInitialFieldVersion)
		}
		if shelf.GetCodecFactory() != nil {
			t.Error("session codec factory was installed before DTY was decoded")
		}
		return prepareDTY(header)
	})

	msg, err := streamer.Pull(ctx, TTIPRO, TTIDTY, TTIFOB)
	if err != nil {
		t.Fatalf("read through DTY reply: %v", err)
	}
	if msg.GetMsgCode() != TTIFOB {
		t.Fatalf("got message %v after negotiation, want TTIFOB", msg.GetMsgCode())
	}

	if got := shelf.GetCapabilities()[kpccapCtTtcFldVsn].Value; got != ttcFldVsn234 {
		t.Fatalf("server field version after PRO = %d, want %d", got, ttcFldVsn234)
	}

	if !sentDTY.negotiatedCapsMap[kpccapRtTzEx].IsSet || !sentDTY.negotiatedCapsMap[kpccapCtbTtc3Tzver].IsSet {
		t.Error("fresh PRO changed the capabilities used to decode this DTY response")
	}
	if got := sessCtx.GetClientProperties().GetProperty(driverCommon.TimeZoneVersionNumber); got != serverTimezoneVersion {
		t.Errorf("timezone version = %v, want %d", got, serverTimezoneVersion)
	}
	if got := shelf.GetMessageFactory().(*SimpleFactory).ttcVersion; got != int8(ttcFldVsn234) {
		t.Errorf("confirmed factory version = %d, want %d", got, ttcFldVsn234)
	}
	if got := shelf.GetCodecFactory().(*CodecFactoryImpl).ttcVersion; got != int8(ttcFldVsn234) {
		t.Errorf("confirmed codec version = %d, want %d", got, ttcFldVsn234)
	}
	// The existing PRO fixture supplies NCHAR character set 775 in its FDO.
	if got := sessCtx.SessionNCharCharacterSet(); got != 775 {
		t.Errorf("confirmed NCHAR character set = %d, want 775", got)
	}
	unregisterTTIInitNegotiationCallbacks(streamer)
	if streamer.postUCallbacks[TTIPRO] != nil || streamer.preUCallbacks[TTIDTY] != nil || streamer.postUCallbacks[TTIDTY] != nil {
		t.Error("setup callbacks remain registered after cleanup")
	}
}

// TestTTIInitNegotiationDecodeErrors verifies that failed PRO/DTY decoding
// does not install negotiated settings or complete negotiation.
func TestTTIInitNegotiationDecodeErrors(t *testing.T) {
	decodeErr := errors.New("negotiation response decode failed")
	for _, messageType := range []driverCommon.MessageType{TTIPRO, TTIDTY} {
		t.Run(TTCMsgTypeDescription[messageType], func(t *testing.T) {
			negotiator := newTTIInitConnectionNegotiator(NewArrayDataBuffer(4096), nil)
			sessCtx, shelf, err := negotiator.Negotiate(context.Background())
			if err != nil {
				t.Fatalf("Negotiate failed: %v", err)
			}
			streamer := shelf.GetMessageStreamer().(*MessageStreamer)
			defer unregisterTTIInitNegotiationCallbacks(streamer)
			const previousTimezoneVersion byte = 3
			sessCtx.SetTimeZoneVersionNumber(previousTimezoneVersion)

			keep, err := streamer.postUCallbacks[messageType](nil, decodeErr)

			if !keep || err != decodeErr {
				t.Fatalf("callback returned (%t, %v), want (true, %v)", keep, err, decodeErr)
			}
			if got := sessCtx.GetClientProperties().GetProperty(driverCommon.TimeZoneVersionNumber); got != previousTimezoneVersion {
				t.Errorf("timezone version = %v, want %d", got, previousTimezoneVersion)
			}
		})
	}
}
