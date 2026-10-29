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
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestLobManager_OwnsTemporaryReference verifies the manager delegates
// temporary-locator retention and release to the session registry.
func TestLobManager_OwnsTemporaryReference(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	manager, err := newLobManager(shelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	loc := newTestLobReferenceLocator(71)

	reference, err := manager.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retainLobReference: %v", err)
	}
	if err := manager.releaseLobReference(reference); err != nil {
		t.Fatalf("releaseLobReference: %v", err)
	}
	if loc.isTemporaryLocator() {
		t.Fatal("released locator still appears temporary")
	}
	batch, err := shelf.lobState.lobReferenceRegistry.ReservePending()
	if err != nil || batch == nil {
		t.Fatalf("pending temporary free batch = %v, %v; want one batch", batch, err)
	}
	shelf.lobState.lobReferenceRegistry.RestorePending(batch)
}

// TestLobManager_RejectsUnsupportedKind verifies every manager dispatch method
// rejects an unsupported kind before it reaches a type-specific executor.
func TestLobManager_RejectsUnsupportedKind(t *testing.T) {
	t.Parallel()
	manager, err := newLobManager(newShelf[common.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	ctx := context.Background()
	unsupported := internallob.Kind(99)
	loc := (*locator)(nil)
	cases := []struct {
		name string
		call func() error
	}{
		{name: "createTemporary", call: func() error { _, err := manager.createTemporary(ctx, unsupported); return err }},
		{name: "read", call: func() error { _, _, err := manager.read(ctx, unsupported, loc, 1, 0); return err }},
		{name: "write", call: func() error { _, err := manager.write(ctx, unsupported, loc, nil); return err }},
		{name: "decodePrefix", call: func() error { _, _, _, err := manager.decodePrefix(unsupported, loc, nil, false); return err }},
		{name: "length", call: func() error { _, err := manager.length(ctx, unsupported, loc); return err }},
		{name: "chunkSize", call: func() error { _, err := manager.chunkSize(ctx, unsupported, loc); return err }},
		{name: "trim", call: func() error { _, err := manager.trim(ctx, unsupported, loc, 1); return err }},
		{name: "open", call: func() error { _, err := manager.open(ctx, unsupported, loc, lobOpenModeReadOnly); return err }},
		{name: "close", call: func() error { return manager.close(ctx, unsupported, loc) }},
		{name: "isOpen", call: func() error { _, err := manager.isOpen(ctx, unsupported, loc); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var coded oracleErrors.SQLError
			err := tc.call()
			if err == nil || !errors.As(err, &coded) || coded.ErrorCode() != string(oracleErrors.UnsupportedLobOperation) {
				t.Fatalf("unsupported kind error = %v, want %s", err, oracleErrors.UnsupportedLobOperation)
			}
		})
	}
}

// TestLobManager_RejectsInvalidOperationOptionsAndKinds verifies invalid
// modes, text data, and BLOB text options are rejected before a TTC exchange.
func TestLobManager_RejectsInvalidOperationOptionsAndKinds(t *testing.T) {
	t.Parallel()
	manager, err := newLobManager(newShelf[common.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	loc := newTestLobReferenceLocator(74)
	tests := []struct {
		name string
		call func() error
		want oracleErrors.ErrorCode
	}{
		{
			name: "BLOB read with UTF-16 carry",
			call: func() error {
				_, _, err := manager.read(context.Background(), internallob.BLOB, loc, 1, 0xD83D)
				return err
			},
			want: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "unsupported open mode",
			call: func() error {
				_, err := manager.open(context.Background(), internallob.BLOB, loc, lobOpenMode(99))
				return err
			},
			want: oracleErrors.UnsupportedLobOperation,
		},
		{
			name: "BLOB prefix decode",
			call: func() error {
				_, _, _, err := manager.decodePrefix(internallob.BLOB, loc, nil, false)
				return err
			},
			want: oracleErrors.UnsupportedLobOperation,
		},
		{
			name: "CLOB write with invalid UTF-8",
			call: func() error {
				_, err := manager.write(context.Background(), internallob.CLOB, loc, []byte{0xff})
				return err
			},
			want: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "NCLOB write with invalid UTF-8",
			call: func() error {
				_, err := manager.write(context.Background(), internallob.NCLOB, loc, []byte{0xc3, 0x28})
				return err
			},
			want: oracleErrors.InvalidLOBBuffer,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireErrorCode(t, test.call(), test.want)
		})
	}
}

// TestLobManager_CreateTemporaryReturnsExecutorError verifies create errors
// are returned to the caller without fabricating a locator.
func TestLobManager_CreateTemporaryReturnsExecutorError(t *testing.T) {
	t.Parallel()
	manager, streamer := newLobManagerRPCFixture(t)
	wantErr := errors.New("temporary LOB create flush failed")
	streamer.flushErr = wantErr
	if locator, err := manager.createTemporary(context.Background(), internallob.BLOB); !errors.Is(err, wantErr) || locator != nil {
		t.Fatalf("createTemporary = (%v, %v), want nil locator and %v", locator, err, wantErr)
	}
}

// TestLobManager_DecodePrefixEmptyPayload verifies a valid character-LOB
// prefix with no bytes decodes as empty data without a TTC exchange.
func TestLobManager_DecodePrefixEmptyPayload(t *testing.T) {
	t.Parallel()
	manager, err := newLobManager(newShelf[common.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	data, logical, carry, err := manager.decodePrefix(internallob.CLOB, newTestLobReferenceLocator(75), nil, false)
	if err != nil {
		t.Fatalf("decode empty prefix: %v", err)
	}
	if len(data) != 0 || logical != 0 || carry != 0 {
		t.Fatalf("decode empty prefix = (%q, %d, %#x), want empty data and zero units/carry", data, logical, carry)
	}
}

// TestLobManager_RejectsNilLocator verifies each locator operation returns
// InvalidLOBBuffer for a missing locator.
func TestLobManager_RejectsNilLocator(t *testing.T) {
	t.Parallel()
	manager, err := newLobManager(newShelf[common.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
	}{
		{name: "read", call: func() error { _, _, err := manager.read(ctx, internallob.BLOB, nil, 1, 0); return err }},
		{name: "write", call: func() error { _, err := manager.write(ctx, internallob.BLOB, nil, nil); return err }},
		{name: "decodePrefix", call: func() error { _, _, _, err := manager.decodePrefix(internallob.CLOB, nil, nil, false); return err }},
		{name: "length", call: func() error { _, err := manager.length(ctx, internallob.BLOB, nil); return err }},
		{name: "chunkSize", call: func() error { _, err := manager.chunkSize(ctx, internallob.BLOB, nil); return err }},
		{name: "trim", call: func() error { _, err := manager.trim(ctx, internallob.BLOB, nil, 0); return err }},
		{name: "open", call: func() error { _, err := manager.open(ctx, internallob.BLOB, nil, lobOpenModeReadOnly); return err }},
		{name: "close", call: func() error { return manager.close(ctx, internallob.BLOB, nil) }},
		{name: "isOpen", call: func() error { _, err := manager.isOpen(ctx, internallob.BLOB, nil); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var coded oracleErrors.SQLError
			err := tc.call()
			if err == nil || !errors.As(err, &coded) || coded.ErrorCode() != string(oracleErrors.InvalidLOBBuffer) {
				t.Fatalf("nil-locator error = %v, want %s", err, oracleErrors.InvalidLOBBuffer)
			}
		})
	}
}

// TestLobManager_SharesSessionExecutors verifies that managers created for one
// physical session reuse one executor for each LOB family.
func TestLobManager_SharesSessionExecutors(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	sessionContext := newTestSessionContext()
	first, err := newLobManager(shelf, sessionContext)
	if err != nil {
		t.Fatalf("newLobManager first: %v", err)
	}
	second, err := newLobManager(shelf, sessionContext)
	if err != nil {
		t.Fatalf("newLobManager second: %v", err)
	}
	if first.getBlobExecutor() != second.getBlobExecutor() {
		t.Fatal("BLOB executor was not shared by session managers")
	}
	if first.getClobExecutor() != second.getClobExecutor() {
		t.Fatal("CLOB executor was not shared by session managers")
	}
}

// newLobManagerRPCFixture creates a manager backed by the deterministic LOB
// message factory and fake streamer used by executor tests. Each operation gets
// a fresh fixture because the fake streamer consumes its response messages.
func newLobManagerRPCFixture(t *testing.T) (*lobManager, *fakeStreamer) {
	t.Helper()
	ttcShelf := newShelf[common.MessageType]()
	fixtureShelf, _, _ := newLobTestShelf(8192)
	ttcShelf.RegisterMessageFactory(fixtureShelf.GetMessageFactory())
	factory := ttcShelf.GetMessageFactory().(Factory)
	rpa, err := factory.GetMessageForFunction(TTIRPA, oLobOps)
	if err != nil {
		t.Fatalf("GetMessageForFunction: %v", err)
	}
	streamer := &fakeStreamer{
		events:    []common.Message[common.MessageType]{rpa, &mockOer{}},
		preHooks:  make(map[common.MessageType]StreamerPreUnmarshallCallback),
		postHooks: make(map[common.MessageType]StreamerPostUnmarshallCallback),
		// A zero response is valid for every dispatcher operation and avoids
		// claiming bytes that the one-byte read fixture did not return.
		lobRpaAmounts: []common.UB8{0},
	}
	ttcShelf.RegisterMessageStreamer(streamer)
	manager, err := newLobManager(ttcShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	return manager, streamer
}

// TestLobManager_DispatchesAllLOBOperations verifies every supported kind and
// manager operation sends the expected OLOBOPS operation code.
func TestLobManager_DispatchesAllLOBOperations(t *testing.T) {
	t.Parallel()
	type operationCase struct {
		name string
		want lobOperationCode
		call func(context.Context, *lobManager, internallob.Kind, *locator) error
	}
	kinds := []struct {
		name string
		kind internallob.Kind
	}{
		{name: "BLOB", kind: internallob.BLOB},
		{name: "CLOB", kind: internallob.CLOB},
		{name: "NCLOB", kind: internallob.NCLOB},
	}
	operations := []operationCase{
		{name: "createTemporary", want: kplobTmpCreate, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, _ *locator) error {
			_, err := manager.createTemporary(ctx, kind)
			return err
		}},
		{name: "read", want: kplobRead, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, _, err := manager.read(ctx, kind, loc, 1, 0)
			return err
		}},
		{name: "write", want: kplobWrite, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, err := manager.write(ctx, kind, loc, []byte("A"))
			return err
		}},
		{name: "length", want: kplobGetLength, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, err := manager.length(ctx, kind, loc)
			return err
		}},
		{name: "chunkSize", want: kplobPageSize, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, err := manager.chunkSize(ctx, kind, loc)
			return err
		}},
		{name: "trim", want: kplobTrim, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, err := manager.trim(ctx, kind, loc, 1)
			return err
		}},
		{name: "open", want: kplobOpen, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, err := manager.open(ctx, kind, loc, lobOpenModeReadWrite)
			return err
		}},
		{name: "close", want: kplobClose, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			return manager.close(ctx, kind, loc)
		}},
		{name: "isOpen", want: kplobIsOpen, call: func(ctx context.Context, manager *lobManager, kind internallob.Kind, loc *locator) error {
			_, err := manager.isOpen(ctx, kind, loc)
			return err
		}},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			for _, lobKind := range kinds {
				lobKind := lobKind
				t.Run(lobKind.name, func(t *testing.T) {
					manager, streamer := newLobManagerRPCFixture(t)
					loc := newLocator(newTestLocator(false), 1)
					if err := operation.call(context.Background(), manager, lobKind.kind, loc); err != nil {
						t.Fatalf("%s %s: %v", lobKind.name, operation.name, err)
					}
					if streamer.definition == nil {
						t.Fatalf("%s %s did not push a LOB definition", lobKind.name, operation.name)
					}
					if got := streamer.definition.operation; got != operation.want {
						t.Fatalf("%s %s operation = %v, want %v", lobKind.name, operation.name, got, operation.want)
					}
				})
			}
		})
	}
}

// TestLobManager_HandlesSessionAbandonmentAndReferenceErrors verifies
// cross-session reference rejection, standalone free, and stale events.
func TestLobManager_HandlesSessionAbandonmentAndReferenceErrors(t *testing.T) {
	t.Parallel()
	firstShelf := newShelf[common.MessageType]()
	firstManager, err := newLobManager(firstShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager first: %v", err)
	}
	managedAbandonmentShelf := newShelf[common.MessageType]()
	managedAbandonment, err := newLobManager(managedAbandonmentShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager for abandonment: %v", err)
	}
	managedAbandonment.abandonSession()
	if !managedAbandonmentShelf.lobState.isInvalid() {
		t.Fatal("manager abandonment callback did not invalidate its LOB session")
	}
	secondManager, err := newLobManager(newShelf[common.MessageType](), newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager second: %v", err)
	}
	reference, err := secondManager.retainLobReference(newTestLobReferenceLocator(72))
	if err != nil {
		t.Fatalf("retain second-session reference: %v", err)
	}
	if err := firstManager.releaseLobReference(reference); err == nil {
		t.Fatal("cross-session reference release succeeded")
	}
	if err := secondManager.releaseLobReference(reference); err != nil {
		t.Fatalf("release owning-session reference: %v", err)
	}
	if err := firstManager.freeLobReference(nil); err == nil {
		t.Fatal("free nil locator succeeded")
	}
	if err := firstManager.freeLobReference(newTestLobReferenceLocator(73)); err != nil {
		t.Fatalf("free standalone locator: %v", err)
	}

	recorder := &lobRegistryEventRecorder{}
	firstShelf.getEventService().register(recorder, streamerStaleEvent)
	abandonLobSession(firstShelf)
	if !firstShelf.lobState.isInvalid() {
		t.Fatal("abandoned LOB session was not invalidated")
	}
	if batch, err := firstShelf.lobState.lobReferenceRegistry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("pending frees after abandonment = %v, %v; want none", batch, err)
	}
	if len(recorder.events) != 1 || recorder.events[0] != streamerStaleEvent {
		t.Fatalf("events = %v, want [streamerStaleEvent]", recorder.events)
	}
}

// TestLobManager_RejectsIncompleteSession verifies nil shelf and session
// context arguments cannot produce an operational manager.
func TestLobManager_RejectsIncompleteSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		shelf   *ttiShelf[common.MessageType]
		context *common.SessionContext
	}{
		{name: "nil shelf", context: newTestSessionContext()},
		{name: "nil session context", shelf: newShelf[common.MessageType]()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, err := newLobManager(test.shelf, test.context)
			if err == nil || manager != nil {
				t.Fatalf("newLobManager = (%v, %v), want nil manager and error", manager, err)
			}
			var coded oracleErrors.SQLError
			if !errors.As(err, &coded) || coded.ErrorCode() != string(oracleErrors.InternalError) {
				t.Fatalf("newLobManager error = %v, want %s", err, oracleErrors.InternalError)
			}
		})
	}
}
