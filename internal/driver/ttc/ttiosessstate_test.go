/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
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
		flags         sessionState
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
			connection, streamer := newSessionStateTestConnection(18, tt.capability)

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

// TestConnectionResetSessionPushesRequestEndThenBegin verifies ResetSession queues end before begin.
func TestConnectionResetSessionPushesRequestEndThenBegin(t *testing.T) {
	t.Parallel()
	connection, streamer := newSessionStateTestConnection(18, driverCommon.Capability{IsSet: true})

	if err := connection.ResetSession(context.Background()); err != nil {
		t.Fatalf("ResetSession() error = %v", err)
	}

	got := pushedSessionStateFlags(t, streamer)
	want := []driverCommon.UB8{
		driverCommon.UB8(sessionStateExplBound | sessionStateRequestEnd),
		driverCommon.UB8(sessionStateExplBound | sessionStateRequestBegin),
	}
	if len(got) != len(want) {
		t.Fatalf("pushed flags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pushed flags[%d] = %d, want %d", i, got[i], want[i])
		}
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
		flags    sessionState
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

func newSessionStateTestConnection(protocol int8, capability driverCommon.Capability) (*connection, *mockStreamer) {
	caps := map[string]driverCommon.Capability{kpccapRtbTtcSessstateops: capability}
	shelf := newShelf[driverCommon.MessageType]()
	streamer := &mockStreamer{}
	shelf.RegisterCapabilities(caps)
	shelf.RegisterMessageFactory(NewMessageFactoryForProtocol(protocol, caps))
	shelf.RegisterMessageStreamer(streamer)

	conn := newTestConnection(shelf, driverCommon.NewSessionContext(), &mockNetworkSession{})
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
