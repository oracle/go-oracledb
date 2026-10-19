/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** and related or neighboring rights under any and all patent rights owned or
** freely licensable by each licensor hereunder covering either (i) the
** unmodified Software as contributed to or provided by such licensors, or (ii)
** the Larger Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software, and to
** sublicense the foregoing rights on either these or other terms.
**
** This license is subject to the condition that the above copyright notice and
** this permission notice shall be included in all copies or substantial portions
** of the Software.
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
	"context"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestConnectionLob_LocatorOperationsRejectEmptyLocator verifies every
// locator-based LOB operation rejects an empty locator before session admission.
func TestConnectionLob_LocatorOperationsRejectEmptyLocator(t *testing.T) {
	t.Parallel()

	conn := &connection{}
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "read",
			call: func() error {
				_, _, err := conn.LobRead(ctx, uint8(internallob.BLOB), nil, 1, 1)
				return err
			},
		},
		{
			name: "write",
			call: func() error {
				_, err := conn.LobWrite(ctx, uint8(internallob.BLOB), nil, 1, nil)
				return err
			},
		},
		{
			name: "length",
			call: func() error {
				_, err := conn.LobLength(ctx, uint8(internallob.BLOB), nil)
				return err
			},
		},
		{
			name: "chunk size",
			call: func() error {
				_, err := conn.LobChunkSize(ctx, uint8(internallob.BLOB), nil)
				return err
			},
		},
		{
			name: "trim",
			call: func() error {
				_, err := conn.LobTrim(ctx, uint8(internallob.BLOB), nil, 0)
				return err
			},
		},
		{
			name: "open",
			call: func() error {
				_, _, err := conn.LobOpen(ctx, uint8(internallob.BLOB), nil, uint8(lobOpenModeReadOnly))
				return err
			},
		},
		{
			name: "close",
			call: func() error {
				_, err := conn.LobClose(ctx, uint8(internallob.BLOB), nil)
				return err
			},
		},
		{
			name: "is open",
			call: func() error {
				_, err := conn.LobIsOpen(ctx, uint8(internallob.BLOB), nil)
				return err
			},
		},
		{
			name: "free",
			call: func() error { return conn.LobFree(ctx, nil) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorCode(t, test.call(), oracleErrors.InvalidLOBBuffer)
		})
	}
}

// TestConnectionLob_ReadReturnsPayloadAndLogicalAmount verifies a successful
// connection LOB read uses the manager, synchronizer, and TTC executor while
// preserving the caller's locator bytes.
func TestConnectionLob_ReadReturnsPayloadAndLogicalAmount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	want := driverCommon.B1Array{0x00, 0xff, 0x10, 0x80, 0x41}
	locator := newTestLocator(false)
	locator[koll1FlagOffset] = kolblBlobFlag
	originalLocator := append(driverCommon.B1Array(nil), locator...)
	fixtureShelf, _, _ := newLobTestShelf(8192)
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageFactory(fixtureShelf.GetMessageFactory())
	shelf.RegisterMessageStreamer(&fakeStreamer{
		events: []driverCommon.Message[driverCommon.MessageType]{
			newTTIlobd(),
			newTTILobRPA(),
			&mockOer{},
		},
		preHooks:      make(map[driverCommon.MessageType]StreamerPreUnmarshallCallback),
		postHooks:     make(map[driverCommon.MessageType]StreamerPostUnmarshallCallback),
		lobdPayloads:  [][]byte{want},
		lobRpaAmounts: []driverCommon.UB8{driverCommon.UB8(len(want))},
	})
	conn := &connection{
		shelf:    shelf,
		sessCtx:  newTestSessionContext(),
		_isValid: true,
	}

	got, logical, err := conn.LobRead(ctx, uint8(internallob.BLOB), locator, 1, uint64(len(want)))
	if err != nil {
		t.Fatalf("LobRead() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("LobRead() payload = % X, want % X", got, want)
	}
	if logical != uint64(len(want)) {
		t.Fatalf("LobRead() logical amount = %d, want %d", logical, len(want))
	}
	if !bytes.Equal(locator, originalLocator) {
		t.Fatalf("LobRead() changed locator bytes: got % X, want % X", locator, originalLocator)
	}
}

// TestConnectionLob_ReadRejectsCloseAfterAdmissionWait verifies that a direct
// LOB read does not send TTC traffic when Close wins while admission is held.
func TestConnectionLob_ReadRejectsCloseAfterAdmissionWait(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	streamer := &mockStreamer{}
	shelf.RegisterMessageStreamer(streamer)
	connection := newTestConnection(shelf, newTestSessionContext(), &mockNetworkSession{})
	locator := newTestLocator(false)
	locator[koll1FlagOffset] = kolblBlobFlag

	heldRelease, err := shelf.synchronizer.begin(context.Background())
	if err != nil {
		t.Fatalf("hold synchronizer: %v", err)
	}

	entered := make(chan struct{})
	readContext := &admissionProbeContext{
		Context: context.Background(),
		entered: entered,
	}
	readDone := make(chan error, 1)
	go func() {
		_, _, readErr := connection.LobRead(readContext, uint8(internallob.BLOB), locator, 1, 1)
		readDone <- readErr
	}()
	<-entered

	if !connection.markClosed() {
		t.Fatal("markClosed returned false")
	}
	heldRelease()

	if err := <-readDone; err == nil {
		t.Fatal("LobRead succeeded after connection close")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	if streamer.pushCalled {
		t.Fatal("LobRead sent TTC traffic after connection close")
	}
}

// TestConnectionLob_LobOperationsRejectUnknownKind verifies LOB operations
// reject an unsupported LOB family before attempting a TTC exchange.
func TestConnectionLob_LobOperationsRejectUnknownKind(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	conn := &connection{
		shelf:    shelf,
		sessCtx:  driverCommon.NewSessionContext(),
		_isValid: true,
	}
	ctx := context.Background()
	locator := []byte("locator")
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				_, err := conn.LobCreate(ctx, uint8(internallob.Unknown))
				return err
			},
		},
		{
			name: "read",
			call: func() error {
				_, _, err := conn.LobRead(ctx, uint8(internallob.Unknown), locator, 1, 1)
				return err
			},
		},
		{
			name: "write",
			call: func() error {
				_, err := conn.LobWrite(ctx, uint8(internallob.Unknown), locator, 1, nil)
				return err
			},
		},
		{
			name: "length",
			call: func() error {
				_, err := conn.LobLength(ctx, uint8(internallob.Unknown), locator)
				return err
			},
		},
		{
			name: "chunk size",
			call: func() error {
				_, err := conn.LobChunkSize(ctx, uint8(internallob.Unknown), locator)
				return err
			},
		},
		{
			name: "trim",
			call: func() error {
				_, err := conn.LobTrim(ctx, uint8(internallob.Unknown), locator, 0)
				return err
			},
		},
		{
			name: "open",
			call: func() error {
				_, _, err := conn.LobOpen(ctx, uint8(internallob.Unknown), locator, uint8(lobOpenModeReadOnly))
				return err
			},
		},
		{
			name: "close",
			call: func() error {
				_, err := conn.LobClose(ctx, uint8(internallob.Unknown), locator)
				return err
			},
		},
		{
			name: "is open",
			call: func() error {
				_, err := conn.LobIsOpen(ctx, uint8(internallob.Unknown), locator)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorCode(t, test.call(), oracleErrors.InvalidLobSource)
		})
	}
}

// TestConnectionLob_LobSessionKeyReturnsPhysicalSession verifies the session
// key identifies the physical shelf that owns the LOB locator.
func TestConnectionLob_LobSessionKeyReturnsPhysicalSession(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	conn := &connection{shelf: shelf}
	if got := conn.LobSessionKey(); got != shelf {
		t.Fatalf("LobSessionKey() = %p, want %p", got, shelf)
	}
}
