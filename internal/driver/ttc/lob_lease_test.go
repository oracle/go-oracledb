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
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestLobLease_NewLeaseRejectsInvalidatedSession verifies that a typed LOB
// lease cannot be created after its physical session becomes unusable.
func TestLobLease_NewLeaseRejectsInvalidatedSession(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	shelf.lobState.markInvalid()
	lease, err := newLobLease(shelf, newTestSessionContext())
	if lease != nil {
		t.Fatal("lease returned for an invalidated session")
	}
	requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
}

// TestLobLease_LeaseValidationRejectsInvalidState covers invalidated and
// generation-expired lease capabilities.
func TestLobLease_LeaseValidationRejectsInvalidState(t *testing.T) {
	t.Parallel()

	invalidatedShelf := newShelf[driverCommon.MessageType]()
	invalidatedLease, err := newLobLease(invalidatedShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease for invalidation case: %v", err)
	}
	invalidatedShelf.lobState.markInvalid()
	expiredShelf := newShelf[driverCommon.MessageType]()
	expiredLease, err := newLobLease(expiredShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease for expiry case: %v", err)
	}
	expiredShelf.lobState.expireLeases()
	tests := []struct {
		name  string
		lease *lobLease
	}{
		{name: "invalidated session", lease: invalidatedLease},
		{name: "expired generation", lease: expiredLease},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorCode(t, test.lease.ValidateLOBLease(), oracleErrors.LobValueInvalidated)
		})
	}
	if _, _, err := expiredLease.LobRead(context.Background(), internallob.BLOB, nil, 1, 1); err == nil {
		t.Fatal("expired lease accepted an invalid locator")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
}

// TestLobLease_AcquireLeaseRejectsInvalidState covers lease invalidation
// after construction and expiry when the physical connection returns to the pool.
func TestLobLease_AcquireLeaseRejectsInvalidState(t *testing.T) {
	t.Parallel()

	invalidatedShelf := newShelf[driverCommon.MessageType]()
	invalidatedLease, err := newLobLease(invalidatedShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease for invalidation case: %v", err)
	}
	invalidatedShelf.lobState.markInvalid()
	expiredShelf := newShelf[driverCommon.MessageType]()
	expiredLease, err := newLobLease(expiredShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease for expiry case: %v", err)
	}
	expiredShelf.lobState.expireLeases()
	tests := []struct {
		name  string
		lease *lobLease
	}{
		{name: "invalidated session", lease: invalidatedLease},
		{name: "expired generation", lease: expiredLease},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			release, err := test.lease.AcquireLOBLease()
			if release != nil {
				release()
				t.Fatal("release function returned for invalid lease")
			}
			requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
		})
	}
}

// TestLobLease_LeaseExpiresAfterPoolReturn verifies the pool-return
// validation boundary revokes a locator capability from the previous lease.
func TestLobLease_LeaseExpiresAfterPoolReturn(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	sessionContext := newTestSessionContext()
	physicalConnection := &connection{
		shelf:    shelf,
		sessCtx:  sessionContext,
		ns:       &mockNetworkSession{},
		_isValid: true,
	}
	lease, err := newLobLease(shelf, sessionContext)
	if err != nil {
		t.Fatalf("create connection LOB lease: %v", err)
	}
	if err := lease.ValidateLOBLease(); err != nil {
		t.Fatalf("new LOB lease is invalid: %v", err)
	}

	if !physicalConnection.IsValid() {
		t.Fatal("connection was invalidated while returning it to the pool")
	}
	requireErrorCode(t, lease.ValidateLOBLease(), oracleErrors.LobValueInvalidated)
}

// TestLobLease_LeaseExpiryWaitsForOperation verifies pool-return
// validation waits until an in-flight LOB operation releases its lease pin.
func TestLobLease_LeaseExpiryWaitsForOperation(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	sessionContext := newTestSessionContext()
	physicalConnection := &connection{
		shelf:    shelf,
		sessCtx:  sessionContext,
		ns:       &mockNetworkSession{},
		_isValid: true,
	}
	lease, err := newLobLease(shelf, sessionContext)
	if err != nil {
		t.Fatalf("create connection LOB lease: %v", err)
	}
	if err := lease.ValidateLOBLease(); err != nil {
		t.Fatalf("new LOB lease is invalid: %v", err)
	}

	releaseOperation, err := lease.AcquireLOBLease()
	if err != nil {
		t.Fatalf("acquire LOB operation lease: %v", err)
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(releaseOperation) }
	defer release()

	validationDone := make(chan bool, 1)
	go func() { validationDone <- physicalConnection.IsValid() }()

	// Wait until the validator is queued for the lease write lock. TryRLock
	// fails once that writer is waiting, making the blocked-state assertion
	// independent of scheduler timing.
	queueTimer := time.NewTimer(time.Second)
	defer queueTimer.Stop()
	for {
		if !shelf.lobState.leaseMu.TryRLock() {
			break
		}
		shelf.lobState.leaseMu.RUnlock()
		select {
		case valid := <-validationDone:
			t.Fatalf("pool validation completed while a LOB operation held its lease: valid=%v", valid)
		case <-queueTimer.C:
			t.Fatal("pool validator did not wait on the active LOB operation")
		default:
			runtime.Gosched()
		}
	}

	// The validator is now known to be waiting for this operation's read lock.
	release()
	if valid := <-validationDone; !valid {
		t.Fatal("connection was invalidated while returning it to the pool")
	}
	requireErrorCode(t, lease.ValidateLOBLease(), oracleErrors.LobValueInvalidated)
}

// TestLobLease_LobOperationRejectsUnavailableState exercises invalidated and
// expired lease paths before an RPC.
func TestLobLease_LobOperationRejectsUnavailableState(t *testing.T) {
	t.Parallel()

	invalidatedShelf := newShelf[driverCommon.MessageType]()
	invalidatedLease, err := newLobLease(invalidatedShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease for invalidation case: %v", err)
	}
	invalidatedShelf.lobState.markInvalid()
	expiredShelf := newShelf[driverCommon.MessageType]()
	expiredLease, err := newLobLease(expiredShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease for expiry case: %v", err)
	}
	expiredShelf.lobState.expireLeases()
	tests := []struct {
		name string
		conn *lobLease
	}{
		{name: "invalidated session", conn: invalidatedLease},
		{name: "expired lease", conn: expiredLease},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.conn._lobOperation(context.Background(), func(context.Context) error {
				t.Fatal("operation ran for an unavailable connection")
				return nil
			})
			requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
		})
	}
}

// TestLobLease_LobOperationAbandonsSessionAfterTerminalError ensures an
// unconsumed TTC error invalidates the LOB session before another request.
func TestLobLease_LobOperationAbandonsSessionAfterTerminalError(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	conn, err := newLobLease(shelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	wantErr := internalCommon.NewOracleError(oracleErrors.LobExecError, errors.New("wire failure"), "test")
	err = conn._lobOperation(context.Background(), func(context.Context) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("LOB operation error = %v, want wrapped %v", err, wantErr)
	}
	if !shelf.lobState.isInvalid() {
		t.Fatal("LOB session remained usable after a terminal TTC error")
	}
}

// TestLobLease_LobFreeQueuesTemporaryLocator covers the successful local
// free path and verifies the locator is queued for piggyback cleanup.
func TestLobLease_LobFreeQueuesTemporaryLocator(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	conn, err := newLobLease(shelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	loc := newTestLobReferenceLocator(81)
	if err := conn.LobFree(context.Background(), loc.locatorBytes); err != nil {
		t.Fatalf("LobFree: %v", err)
	}
	batch, err := shelf.lobState.lobReferenceRegistry.ReservePending()
	if err != nil || batch == nil {
		t.Fatalf("pending free = %v, %v; want queued temporary locator", batch, err)
	}
}

// TestLobLease_LocatorOperationsRejectEmptyLocator verifies every
// locator-based LOB operation rejects an empty locator before any TTC exchange.
func TestLobLease_LocatorOperationsRejectEmptyLocator(t *testing.T) {
	t.Parallel()

	conn, err := newLobLease(newShelf[driverCommon.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "read",
			call: func() error {
				_, _, err := conn.LobRead(ctx, internallob.BLOB, nil, 1, 1)
				return err
			},
		},
		{
			name: "write",
			call: func() error {
				_, err := conn.LobWrite(ctx, internallob.BLOB, nil, 1, nil)
				return err
			},
		},
		{
			name: "length",
			call: func() error {
				_, err := conn.LobLength(ctx, internallob.BLOB, nil)
				return err
			},
		},
		{
			name: "chunk size",
			call: func() error {
				_, err := conn.LobChunkSize(ctx, internallob.BLOB, nil)
				return err
			},
		},
		{
			name: "trim",
			call: func() error {
				_, err := conn.LobTrim(ctx, internallob.BLOB, nil, 0)
				return err
			},
		},
		{
			name: "open",
			call: func() error {
				_, _, err := conn.LobOpen(ctx, internallob.BLOB, nil, uint8(lobOpenModeReadOnly))
				return err
			},
		},
		{
			name: "close",
			call: func() error {
				_, err := conn.LobClose(ctx, internallob.BLOB, nil)
				return err
			},
		},
		{
			name: "is open",
			call: func() error {
				_, err := conn.LobIsOpen(ctx, internallob.BLOB, nil)
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

// TestLobLease_ReadWriteRejectZeroOffset verifies the 1-based locator
// contract is enforced before starting a TTC exchange.
func TestLobLease_ReadWriteRejectZeroOffset(t *testing.T) {
	t.Parallel()

	conn, err := newLobLease(newShelf[driverCommon.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "read",
			call: func() error {
				_, _, err := conn.LobRead(ctx, internallob.BLOB, []byte("locator"), 0, 1)
				return err
			},
		},
		{
			name: "write",
			call: func() error {
				_, err := conn.LobWrite(ctx, internallob.BLOB, []byte("locator"), 0, []byte("data"))
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorCode(t, test.call(), oracleErrors.InvalidLOBBuffer)
		})
	}
}

// TestLobLease_ReadReturnsPayloadAndLogicalAmount verifies a successful
// connection LOB read uses the manager and TTC executor while
// preserving the caller's locator bytes.
func TestLobLease_ReadReturnsPayloadAndLogicalAmount(t *testing.T) {
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
	conn, err := newLobLease(shelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}

	got, logical, err := conn.LobRead(ctx, internallob.BLOB, locator, 1, uint64(len(want)))
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

// TestLobLease_LobOperationsRejectUnknownKind verifies LOB operations
// reject an unsupported LOB family before attempting a TTC exchange.
func TestLobLease_LobOperationsRejectUnknownKind(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	conn, err := newLobLease(shelf, driverCommon.NewSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
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
				_, err := conn.LobCreate(ctx, internallob.Unknown)
				return err
			},
		},
		{
			name: "read",
			call: func() error {
				_, _, err := conn.LobRead(ctx, internallob.Unknown, locator, 1, 1)
				return err
			},
		},
		{
			name: "write",
			call: func() error {
				_, err := conn.LobWrite(ctx, internallob.Unknown, locator, 1, nil)
				return err
			},
		},
		{
			name: "length",
			call: func() error {
				_, err := conn.LobLength(ctx, internallob.Unknown, locator)
				return err
			},
		},
		{
			name: "chunk size",
			call: func() error {
				_, err := conn.LobChunkSize(ctx, internallob.Unknown, locator)
				return err
			},
		},
		{
			name: "trim",
			call: func() error {
				_, err := conn.LobTrim(ctx, internallob.Unknown, locator, 0)
				return err
			},
		},
		{
			name: "open",
			call: func() error {
				_, _, err := conn.LobOpen(ctx, internallob.Unknown, locator, uint8(lobOpenModeReadOnly))
				return err
			},
		},
		{
			name: "close",
			call: func() error {
				_, err := conn.LobClose(ctx, internallob.Unknown, locator)
				return err
			},
		},
		{
			name: "is open",
			call: func() error {
				_, err := conn.LobIsOpen(ctx, internallob.Unknown, locator)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorCode(t, test.call(), oracleErrors.UnsupportedLobOperation)
		})
	}
}

// TestLobLease_LobSessionKeyReturnsPhysicalSession verifies the session
// key identifies the physical shelf that owns the LOB locator.
func TestLobLease_LobSessionKeyReturnsPhysicalSession(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	conn, err := newLobLease(shelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	if got := conn.LobSessionKey(); got != shelf {
		t.Fatalf("LobSessionKey() = %p, want %p", got, shelf)
	}
}
