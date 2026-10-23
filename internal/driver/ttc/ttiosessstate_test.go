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
	"encoding/binary"
	"errors"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestConnectionPushSessionStateRequest verifies begin and end flags are queued only when the server supports session-state operations.
func TestConnectionPushSessionStateRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		flags         sessionStateOperationFlags
		capability    driverCommon.Capability
		wantPushCount int
	}{
		{
			name:          "begin request",
			flags:         sessionStateExplBound | sessionStateRequestBegin,
			capability:    driverCommon.Capability{IsSet: true},
			wantPushCount: 1,
		},
		{
			name:          "end request",
			flags:         sessionStateExplBound | sessionStateRequestEnd,
			capability:    driverCommon.Capability{IsSet: true},
			wantPushCount: 1,
		},
		{
			name:  "capability not set",
			flags: sessionStateExplBound | sessionStateRequestBegin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connection, streamer := newSessionStateTestConnection(18, tt.capability, false)

			if err := connection.pushSessionStateRequest(context.Background(), tt.flags); err != nil {
				t.Fatalf("pushSessionStateRequest() error = %v", err)
			}
			if got := streamer.pushedMsg.Len(); got != tt.wantPushCount {
				t.Fatalf("pushed message count = %d, want %d", got, tt.wantPushCount)
			}
			if tt.wantPushCount == 1 {
				got := pushedSessionStateFlags(t, streamer)
				if len(got) != 1 || got[0] != driverCommon.UB8(tt.flags) {
					t.Fatalf("pushed flags = %v, want [%d]", got, tt.flags)
				}
			}
		})
	}
}

// TestConnectionPushSessionStateRequestErrors verifies factory, message-type, and streamer failures are returned.
func TestConnectionPushSessionStateRequestErrors(t *testing.T) {
	t.Parallel()

	factoryErr := errors.New("factory failed")
	pushErr := errors.New("push failed")
	tests := []struct {
		name          string
		factory       driverCommon.Factory
		streamer      *mockStreamer
		wantErrorCode string
		wantCause     error
		wantPushCount int
	}{
		{
			name: "factory error",
			factory: &mockFactory{
				returnErr: factoryErr,
			},
			streamer:      &mockStreamer{},
			wantErrorCode: string(oracleErrors.InternalError),
			wantCause:     factoryErr,
		},
		{
			name: "unexpected message type",
			factory: &mockFactory{
				returnMsg: NewOall18(),
			},
			streamer:      &mockStreamer{},
			wantErrorCode: string(oracleErrors.InternalError),
		},
		{
			name: "streamer push error",
			factory: &mockFactory{
				returnMsg: newOSessStatePfn18(),
			},
			streamer:      &mockStreamer{pushErr: pushErr},
			wantErrorCode: string(oracleErrors.StreamerWriteError),
			wantCause:     pushErr,
			wantPushCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			caps := map[string]driverCommon.Capability{
				kpccapRtbTtcSessstateops: {IsSet: true},
			}
			shelf := newShelf[driverCommon.MessageType]()
			shelf.RegisterCapabilities(caps)
			shelf.RegisterMessageFactory(tt.factory)
			shelf.RegisterMessageStreamer(tt.streamer)
			connection := newTestConnection(shelf, driverCommon.NewSessionContext(), &mockNetworkSession{})

			err := connection.pushSessionStateRequest(context.Background(), sessionStateExplBound|sessionStateRequestBegin)
			if err == nil {
				t.Fatal("pushSessionStateRequest() error = nil, want an error")
			}
			sqlErr, ok := err.(oracleErrors.SQLError)
			if !ok {
				t.Fatalf("pushSessionStateRequest() error type = %T, want oracle SQLError", err)
			}
			if got := sqlErr.ErrorCode(); got != tt.wantErrorCode {
				t.Errorf("pushSessionStateRequest() error code = %q, want %q", got, tt.wantErrorCode)
			}
			if tt.wantCause != nil && !errors.Is(err, tt.wantCause) {
				t.Errorf("pushSessionStateRequest() error = %v, want cause %v", err, tt.wantCause)
			}
			if got := tt.streamer.pushedMsg.Len(); got != tt.wantPushCount {
				t.Errorf("pushed message count = %d, want %d", got, tt.wantPushCount)
			}
		})
	}
}

// TestConnectionResetSessionPushesRequestBegin verifies ResetSession queues a begin request.
func TestConnectionResetSessionPushesRequestBegin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		explicitBoundSupported bool
		want                   []driverCommon.UB8
	}{
		{
			name:                   "explicit bound supported",
			explicitBoundSupported: true,
			want: []driverCommon.UB8{
				driverCommon.UB8(sessionStateExplBound | sessionStateRequestBegin),
			},
		},
		{
			name: "explicit bound unsupported",
			want: []driverCommon.UB8{
				driverCommon.UB8(sessionStateRequestBegin),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connection, streamer := newSessionStateTestConnection(18, driverCommon.Capability{IsSet: true}, tt.explicitBoundSupported)

			if err := connection.ResetSession(context.Background()); err != nil {
				t.Fatalf("ResetSession() error = %v", err)
			}

			got := pushedSessionStateFlags(t, streamer)
			if len(got) != len(tt.want) {
				t.Fatalf("pushed flags = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("pushed flags[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestConnectionInitializeRequestBoundaryCallbacksQueuesInitialBegin verifies
// connection initialization queues a begin request when the server supports
// session-state operations.
func TestConnectionInitializeRequestBoundaryCallbacksQueuesInitialBegin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		sessionStateSupported  bool
		explicitBoundSupported bool
		want                   []driverCommon.UB8
	}{
		{
			name:                   "explicit bound supported",
			sessionStateSupported:  true,
			explicitBoundSupported: true,
			want:                   []driverCommon.UB8{driverCommon.UB8(sessionStateExplBound | sessionStateRequestBegin)},
		},
		{
			name:                  "explicit bound unsupported",
			sessionStateSupported: true,
			want:                  []driverCommon.UB8{driverCommon.UB8(sessionStateRequestBegin)},
		},
		{
			name: "session-state operations unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connection, streamer := newSessionStateTestConnection(
				18,
				driverCommon.Capability{IsSet: tt.sessionStateSupported},
				tt.explicitBoundSupported,
			)
			callbacks := buildRequestBoundaryCallbacks(connection)
			connection.initializeRequestBoundaryCallbacks(callbacks.requestStartCallbacks, callbacks.requestEndCallbacks)
			if err := runRequestBoundaryCallbacks(context.Background(), callbacks.newConnectionCallbacks); err != nil {
				t.Fatalf("runRequestBoundaryCallbacks() error = %v", err)
			}

			got := pushedSessionStateFlags(t, streamer)
			if len(got) != len(tt.want) {
				t.Fatalf("pushed flags = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("pushed flags[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestSessionStateFunctionRegistrationByProtocol verifies factory selection of legacy and TTC 18 headers.
func TestSessionStateFunctionRegistrationByProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		protocol       int8
		wantHeaderIs18 bool
	}{
		{name: "pre TTC 18", protocol: 17},
		{name: "TTC 18", protocol: 18, wantHeaderIs18: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg, err := NewMessageFactoryForProtocol(tt.protocol, nil).GetMessageForFunction(TTIPFN, oSessState)
			if err != nil {
				t.Fatalf("GetMessageForFunction() error = %v", err)
			}
			request, ok := msg.(*tTIOsessstate)
			if !ok {
				t.Fatalf("message type = %T, want *tTIOsessstate", msg)
			}
			if request.GetMsgCode() != TTIPFN || request.GetFuncCode() != oSessState {
				t.Errorf("message codes = (%v, %v), want (%v, %v)", request.GetMsgCode(), request.GetFuncCode(), TTIPFN, oSessState)
			}
			_, headerIs18 := request.headerMarshaller.(*ttiFunHeader18)
			if headerIs18 != tt.wantHeaderIs18 {
				t.Errorf("TTC 18 header = %t, want %t", headerIs18, tt.wantHeaderIs18)
			}
			if !tt.wantHeaderIs18 {
				if _, ok := request.headerMarshaller.(*ttiFunHeader); !ok {
					t.Errorf("pre-TTC 18 header type = %T, want *ttiFunHeader", request.headerMarshaller)
				}
			}
		})
	}
}

// TestTTIOSessstateMarshalRequestFlags verifies begin and end flags are serialized for both header versions.
func TestTTIOSessstateMarshalRequestFlags(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		protocol int8
		flags    sessionStateOperationFlags
	}{
		{name: "pre TTC 18 explicit begin", protocol: 17, flags: sessionStateExplBound | sessionStateRequestBegin},
		{name: "pre TTC 18 explicit end", protocol: 17, flags: sessionStateExplBound | sessionStateRequestEnd},
		{name: "TTC 18 explicit begin", protocol: 18, flags: sessionStateExplBound | sessionStateRequestBegin},
		{name: "TTC 18 explicit end", protocol: 18, flags: sessionStateExplBound | sessionStateRequestEnd},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg, err := NewMessageFactoryForProtocol(tt.protocol, nil).GetMessageForFunction(TTIPFN, oSessState)
			if err != nil {
				t.Fatalf("GetMessageForFunction() error = %v", err)
			}
			request := msg.(*tTIOsessstate)
			request.flags = driverCommon.UB8(tt.flags)

			buf := NewArrayDataBuffer(32)
			engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Native, Native, Native, Native})
			if err := request.MarshalTo(context.Background(), engine); err != nil {
				t.Fatalf("MarshalTo() error = %v", err)
			}

			payload := buf.bytes[buf.currentWritePosition-8 : buf.currentWritePosition]
			want := make([]byte, 8)
			binary.BigEndian.PutUint64(want, uint64(tt.flags))
			for i := range want {
				if payload[i] != want[i] {
					t.Errorf("marshalled request flag byte[%d] = %#x, want %#x", i, payload[i], want[i])
				}
			}
		})
	}
}

func newSessionStateTestConnection(protocol int8, capability driverCommon.Capability, explicitBoundSupported bool) (*connection, *mockStreamer) {
	caps := map[string]driverCommon.Capability{
		kpccapRtbTtcSessstateops: capability,
		kpccapCtbTtc4ExplBound:   {IsSet: explicitBoundSupported},
	}
	shelf := newShelf[driverCommon.MessageType]()
	streamer := &mockStreamer{}
	shelf.RegisterCapabilities(caps)
	shelf.RegisterMessageFactory(NewMessageFactoryForProtocol(protocol, caps))
	shelf.RegisterMessageStreamer(streamer)

	conn := newTestConnection(shelf, driverCommon.NewSessionContext(), &mockNetworkSession{})
	callbacks := buildRequestBoundaryCallbacks(conn)
	conn._requestStartCallbacks = callbacks.requestStartCallbacks
	conn._requestEndCallbacks = callbacks.requestEndCallbacks
	return conn, streamer
}

func pushedSessionStateFlags(t *testing.T, streamer *mockStreamer) []driverCommon.UB8 {
	t.Helper()

	flags := make([]driverCommon.UB8, 0, streamer.pushedMsg.Len())
	for element := streamer.pushedMsg.Front(); element != nil; element = element.Next() {
		msgRef, ok := element.Value.(*driverCommon.Message[driverCommon.MessageType])
		if !ok || msgRef == nil {
			t.Fatalf("pushed message value has type %T, want pointer to message", element.Value)
		}
		msg := *msgRef
		request, ok := msg.(*tTIOsessstate)
		if !ok {
			t.Fatalf("pushed message has type %T, want *tTIOsessstate", msg)
		}
		flags = append(flags, request.flags)
	}
	return flags
}
