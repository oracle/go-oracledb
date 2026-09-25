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
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type mockStreamerWithFlushError struct {
	*mockStreamer
	flushErr error
}

func (m *mockStreamerWithFlushError) Flush(_ context.Context) error {
	return m.flushErr
}

func newSessionlessTransactionTestConnection() (*connection, *mockStreamer) {
	messageRegistry := NewRegistry[driverCommon.MessageType]()
	_ = messageRegistry.Register(TTIOER, 1, newTTIoer)
	_ = messageRegistry.Register(TTISTA, 1, newTTISTA)

	functionRegistry := NewRegistry[functionRegistryKey]()
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxSe}, 18, newOTxSe18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxEn}, 18, newOTxEn18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: oTxSe}, 18, newOTxSePfn18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxSe}, 1, newOTxSeRPA)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxEn}, 1, newOTxEnRPA)

	messageFactory := &SimpleFactory{
		ttcVersion:   18,
		msgregistry:  messageRegistry,
		funcregistry: functionRegistry,
	}
	mockStr := &mockStreamer{pullMsg: &mockOer{}}
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).
		RegisterMessageStreamer(mockStr).
		RegisterCapabilities(map[string]driverCommon.Capability{
			kpccapCtbTtc5SessionlessTxn: {IsSet: true},
		})
	sessionCtx := driverCommon.NewSessionContext()
	sessionCtx.GetSessionProperties().SetProperty(instanceName, "test-instance")

	return newTestConnection(shelf, sessionCtx, nil), mockStr
}

func waitForSessionlessTransactionState(t *testing.T, conn *connection, wantRegistered bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if conn.shelf.isInTransaction() == wantRegistered {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("sessionless transaction registration = %v, want %v", conn.shelf.isInTransaction(), wantRegistered)
}

// TestSessionlessTransactionContextCancellationRollsBack verifies that
// cancellation cleanup is installed for both ways of obtaining a sessionless
// transaction:
//   - begin starts a new transaction and rolls it back after cancellation;
//   - resume attaches an existing transaction and applies the same cleanup.
func TestSessionlessTransactionContextCancellationRollsBack(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		start func(*connection, context.Context) (common.SessionlessTransaction, error)
	}{
		{
			// Begin installs a watcher for a newly created transaction.
			name: "begin",
			start: func(conn *connection, ctx context.Context) (common.SessionlessTransaction, error) {
				return conn.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
			},
		},
		{
			// Resume must install the same watcher for an attached transaction.
			name: "resume",
			start: func(conn *connection, ctx context.Context) (common.SessionlessTransaction, error) {
				return conn.ResumeSessionlessTx(ctx, []byte("resume-id"), 300)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if _, err := test.start(conn, ctx); err != nil {
				t.Fatalf("%s sessionless transaction failed: %v", test.name, err)
			}
			cancel()
			waitForSessionlessTransactionState(t, conn, false)
			if got, want := streamer.pushedMsg.Len(), 2; got != want {
				t.Fatalf("messages pushed after %s cancellation = %d, want %d", test.name, got, want)
			}
		})
	}
}

// TestSessionlessTransactionSuspendStopsContextWatcher verifies that a
// suspended transaction remains resumable after its original context is
// canceled.
func TestSessionlessTransactionSuspendStopsContextWatcher(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transaction, err := conn.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}
	tx, ok := transaction.(*sessionlessTransaction)
	if !ok {
		t.Fatalf("BeginSessionlessTx returned %T, want *sessionlessTransaction", transaction)
	}
	if err := tx.Suspend(); err != nil {
		t.Fatalf("Suspend failed: %v", err)
	}

	cancel()
	tx.lifecycleMu.Lock()
	state := tx.transactionState
	watcherInstalled := tx.contextWatcherStop != nil
	tx.lifecycleMu.Unlock()
	if state != transactionEndedServer {
		t.Fatalf("transaction state after suspend = %d, want server ended", state)
	}
	if watcherInstalled {
		t.Fatal("context watcher remained installed after suspend")
	}
}

// TestSessionlessTransactionContextCancellationInvalidatesOnRollbackFailure
// verifies that an ambiguous cleanup result cannot return the connection to a
// pool.
func TestSessionlessTransactionContextCancellationInvalidatesOnRollbackFailure(t *testing.T) {
	t.Parallel()

	conn, streamer := newSessionlessTransactionTestConnection()
	streamer.pullMsg = &mockOer{err: errors.New("rollback failed")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := conn.BeginSessionlessTx(ctx, sql.TxOptions{}, 300); err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for conn._isValid && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if conn._isValid {
		t.Fatal("connection remained valid after failed cancellation rollback")
	}
}

// TestSessionlessTransactionEndUsesOTxEn verifies that sessionless commit and
// rollback send OTXEN with the sessionless XID and the matching K2 command.
func TestSessionlessTransactionEndUsesOTxEn(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		end       func(driver.Tx) error
		operation driverCommon.SB4
		inState   driverCommon.UB4
	}{
		{name: "commit", end: func(tx driver.Tx) error { return tx.Commit() }, operation: driverCommon.SB4(otxenCommit), inState: k2cmdCommit},
		{name: "rollback", end: func(tx driver.Tx) error { return tx.Rollback() }, operation: driverCommon.SB4(otxenAbort), inState: k2cmdAbort},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			transaction, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("BeginSessionlessTx failed: %v", err)
			}
			tx, ok := transaction.(*sessionlessTransaction)
			if !ok {
				t.Fatalf("BeginSessionlessTx returned %T, want *sessionlessTransaction", transaction)
			}
			streamer.pushedMsg.Init()

			if err := test.end(tx); err != nil {
				t.Fatalf("sessionless %s returned error: %v", test.name, err)
			}
			if streamer.pushedMsg.Len() != 1 {
				t.Fatalf("%s pushed %d messages, want 1", test.name, streamer.pushedMsg.Len())
			}
			msg := streamer.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
			otxen, ok := (*msg).(*tTIOtxen)
			if !ok {
				t.Fatalf("%s message = %T, want *tTIOtxen", test.name, *msg)
			}
			if otxen.operation != test.operation {
				t.Fatalf("%s OTXEN operation = %d, want %d", test.name, otxen.operation, test.operation)
			}
			if otxen.inState != test.inState {
				t.Fatalf("%s OTXEN in-state = %d, want %d", test.name, otxen.inState, test.inState)
			}
			if otxen.flags != 0 {
				t.Fatalf("%s OTXEN transaction state change flags = %d, want 0", test.name, otxen.flags)
			}
			if otxen.formatID != k2gSessionless {
				t.Fatalf("%s OTXEN format ID = %#x, want %#x", test.name, otxen.formatID, k2gSessionless)
			}
			if otxen.timeout != 300 {
				t.Fatalf("%s OTXEN timeout = %d, want 300", test.name, otxen.timeout)
			}
			if otxen.globalTransactionIDLength != 16 || otxen.bqualLength != driverCommon.UB4(len("test-instance")) {
				t.Fatalf("%s OTXEN XID lengths = (%d, %d), want (16, %d)", test.name, otxen.globalTransactionIDLength, otxen.bqualLength, len("test-instance"))
			}
		})
	}
}

// TestBeginSessionlessTx verifies that starting a new sessionless transaction
// returns the Oracle-specific SessionlessTx contract and sends an OTXSE start
// request with the generated global transaction ID and new-sessionless flags.
func TestBeginSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()

	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}
	if tx == nil {
		t.Fatal("BeginSessionlessTx returned nil transaction")
	}
	if got := tx.GlobalTransactionID(); got == nil {
		t.Fatal("BeginSessionlessTx returned an empty global transaction ID")
	} else if len([]byte(got)) != 16 {
		t.Fatalf("generated global transaction ID length = %d, want 16", len([]byte(got)))
	}

	if mockStr.pushedMsg.Len() != 1 {
		t.Fatalf("pushed message count = %d, want 1", mockStr.pushedMsg.Len())
	}
	if len(mockStr.pullTypes) != 0 {
		t.Fatalf("pull types = %v, want none for deferred start", mockStr.pullTypes)
	}

	pushedMsg := mockStr.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
	otxse, ok := (*pushedMsg).(*tTIOtxse)
	if !ok {
		t.Fatalf("second pushed message = %T, want *tTIOtxse", *pushedMsg)
	}
	if otxse.operation != otxseStart {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseStart)
	}
	if otxse.flags != otxseTransSessionless|otxseTransNew|otxseTransReadWrite {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, otxseTransSessionless|otxseTransNew|otxseTransReadWrite)
	}
	if otxse.formatID != k2gSessionless {
		t.Fatalf("OTXSE formatID = %#x, want %#x", otxse.formatID, k2gSessionless)
	}
	if len(otxse.xid) != maxSessionlessGlobalTransactionIDSize+maxSessionlessBQUALSize {
		t.Fatalf("OTXSE xid length = %d, want %d", len(otxse.xid), maxSessionlessGlobalTransactionIDSize+maxSessionlessBQUALSize)
	}
	if got := []byte(otxse.xid[:len(tx.GlobalTransactionID())]); !slices.Equal(got, tx.GlobalTransactionID()) {
		t.Fatalf("OTXSE xid global transaction ID prefix = %q, want %q", got, tx.GlobalTransactionID())
	}
	if got := string(otxse.xid[len(tx.GlobalTransactionID()) : len(tx.GlobalTransactionID())+len("test-instance")]); got != "test-instance" {
		t.Fatalf("OTXSE xid bqual segment = %q, want %q", got, "test-instance")
	}
	if otxse.globalTransactionIDLength != driverCommon.UB4(len(tx.GlobalTransactionID())) {
		t.Fatalf("OTXSE globalTransactionIDLength = %d, want %d", otxse.globalTransactionIDLength, len(tx.GlobalTransactionID()))
	}
	if otxse.bqualLength != driverCommon.UB4(len("test-instance")) {
		t.Fatalf("OTXSE bqualLength = %d, want %d", otxse.bqualLength, len("test-instance"))
	}
}

// TestResumeSessionlessTx verifies that resuming a sessionless transaction
// preserves the caller-supplied global transaction ID and sends an OTXSE start request with the
// resume-sessionless flags.
func TestResumeSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	globalTransactionID := []byte("resume-global-transaction-id")
	wantGlobalTransactionID := append([]byte(nil), globalTransactionID...)

	tx, err := conn.ResumeSessionlessTx(context.Background(), globalTransactionID, 300)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx failed: %v", err)
	}
	if tx == nil {
		t.Fatal("ResumeSessionlessTx returned nil transaction")
	}
	if got := tx.GlobalTransactionID(); !slices.Equal(got, globalTransactionID) {
		t.Fatalf("GlobalTransactionID = %q, want %q", got, globalTransactionID)
	}

	if mockStr.pushedMsg.Len() != 1 {
		t.Fatalf("pushed message count = %d, want 1", mockStr.pushedMsg.Len())
	}
	if len(mockStr.pullTypes) != 0 {
		t.Fatalf("pull types = %v, want none for deferred resume", mockStr.pullTypes)
	}

	pushedMsg := mockStr.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
	otxse, ok := (*pushedMsg).(*tTIOtxse)
	if !ok {
		t.Fatalf("pushed message = %T, want *tTIOtxse", *pushedMsg)
	}
	if otxse.operation != otxseStart {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseStart)
	}
	if otxse.flags != otxseTransSessionless|otxseTransResume {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, otxseTransSessionless|otxseTransResume)
	}
	if otxse.timeout != 300 {
		t.Fatalf("OTXSE timeout = %d, want 300", otxse.timeout)
	}
	if len(otxse.xid) != maxSessionlessGlobalTransactionIDSize+maxSessionlessBQUALSize {
		t.Fatalf("OTXSE xid length = %d, want %d", len(otxse.xid), maxSessionlessGlobalTransactionIDSize+maxSessionlessBQUALSize)
	}
	if got := []byte(otxse.xid[:len(globalTransactionID)]); !slices.Equal(got, globalTransactionID) {
		t.Fatalf("OTXSE xid global transaction ID prefix = %q, want %q", got, globalTransactionID)
	}
	if otxse.globalTransactionIDLength != driverCommon.UB4(len(globalTransactionID)) {
		t.Fatalf("OTXSE globalTransactionIDLength = %d, want %d", otxse.globalTransactionIDLength, len(globalTransactionID))
	}
	if otxse.bqualLength != driverCommon.UB4(len("test-instance")) {
		t.Fatalf("OTXSE bqualLength = %d, want %d", otxse.bqualLength, len("test-instance"))
	}

	// The transaction must retain its own copy of the caller-provided ID.
	globalTransactionID[0] = 'X'
	if got := tx.GlobalTransactionID(); !slices.Equal(got, wantGlobalTransactionID) {
		t.Fatalf("GlobalTransactionID changed when the input was modified: %q", got)
	}

	// The accessor must also return a copy so callers cannot mutate transaction state.
	returnedGlobalTransactionID := tx.GlobalTransactionID()
	returnedGlobalTransactionID[0] = 'Y'
	if got := tx.GlobalTransactionID(); !slices.Equal(got, wantGlobalTransactionID) {
		t.Fatalf("GlobalTransactionID changed through the returned slice: %q", got)
	}

	// An old transaction handle must not expose an ID after the shelf points to
	// another transaction, even when operations are performed sequentially.
	conn.shelf.registerTransaction(newTransaction(conn, context.Background()))
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID from a non-current transaction = %q, want empty", got)
	}
}

// TestBeginSessionlessTxPushFailure verifies that a write failure during the
// OTXSE start request is surfaced and unregisters the partially started
// transaction.
func TestBeginSessionlessTxPushFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pushErr = errors.New("push failed")

	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err == nil {
		t.Fatal("BeginSessionlessTx returned nil error on push failure")
	}
	if tx != nil {
		t.Fatalf("BeginSessionlessTx returned transaction %T on push failure, want nil", tx)
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("BeginSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.StartResumeTransactionFailure) {
		t.Fatalf("BeginSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.StartResumeTransactionFailure)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx push failure should unregister the transaction")
	}
}

// TestBeginSessionlessTxDefersFlush verifies that a deferred OTXSE start does
// not flush during BeginSessionlessTx.
func TestBeginSessionlessTxDefersFlush(t *testing.T) {
	t.Parallel()

	baseConn, baseMock := newSessionlessTransactionTestConnection()
	streamer := &mockStreamerWithFlushError{mockStreamer: baseMock}
	baseConn.shelf.RegisterMessageStreamer(streamer)
	streamer.flushErr = errors.New("flush failed")

	tx, err := baseConn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed despite deferred flush: %v", err)
	}
	if tx == nil || !baseConn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx should register a deferred transaction")
	}
}

// TestBeginSessionlessTxDefersPull verifies that a deferred OTXSE start does
// not read a response during BeginSessionlessTx.
func TestBeginSessionlessTxDefersPull(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullErr = errors.New("pull failed")

	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed despite deferred pull: %v", err)
	}
	if tx == nil || !conn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx should register a deferred transaction")
	}
}

// TestBeginSessionlessTxDefersOER verifies that a deferred OTXSE start does
// not consume a server response during BeginSessionlessTx.
func TestBeginSessionlessTxDefersOER(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullMsg = &mockOer{err: common.NewOERMessageError("ORA-24776", "start failed")}

	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed despite deferred response: %v", err)
	}
	if tx == nil || !conn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx should register a deferred transaction")
	}
}

// TestResumeSessionlessTxInvalidGlobalTransactionID verifies that invalid caller-provided
// identifiers are rejected before any transaction is started.
func TestResumeSessionlessTxInvalidGlobalTransactionID(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()

	tx, err := conn.ResumeSessionlessTx(context.Background(), nil, 300)
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error for invalid global transaction ID")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T for invalid global transaction ID, want nil", tx)
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.InvalidGlobalTransactionIDValue) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.InvalidGlobalTransactionIDValue)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx invalid global transaction ID should not register a transaction")
	}
}

// TestResumeSessionlessTxPushFailure verifies that a write failure during the
// OTXSE resume request is surfaced and unregisters the partially started
// transaction.
func TestResumeSessionlessTxPushFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pushErr = errors.New("push failed")

	tx, err := conn.ResumeSessionlessTx(context.Background(), []byte("resume-global-transaction-id"), 300)
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error on push failure")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T on push failure, want nil", tx)
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.StartResumeTransactionFailure) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.StartResumeTransactionFailure)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx push failure should unregister the transaction")
	}
}

// TestResumeSessionlessTxDefersFlush verifies that a deferred OTXSE resume does
// not flush during ResumeSessionlessTx.
func TestResumeSessionlessTxDefersFlush(t *testing.T) {
	t.Parallel()

	baseConn, baseMock := newSessionlessTransactionTestConnection()
	streamer := &mockStreamerWithFlushError{mockStreamer: baseMock}
	baseConn.shelf.RegisterMessageStreamer(streamer)
	streamer.flushErr = errors.New("flush failed")

	tx, err := baseConn.ResumeSessionlessTx(context.Background(), []byte("resume-global-transaction-id"), 300)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx failed despite deferred flush: %v", err)
	}
	if tx == nil || !baseConn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx should register a deferred transaction")
	}
}

// TestResumeSessionlessTxDefersPull verifies that a deferred OTXSE resume does
// not read a response during ResumeSessionlessTx.
func TestResumeSessionlessTxDefersPull(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullErr = errors.New("pull failed")

	tx, err := conn.ResumeSessionlessTx(context.Background(), []byte("resume-global-transaction-id"), 300)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx failed despite deferred pull: %v", err)
	}
	if tx == nil || !conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx should register a deferred transaction")
	}
}

// TestResumeSessionlessTxDefersOER verifies that a deferred OTXSE resume does
// not consume a server response during ResumeSessionlessTx.
func TestResumeSessionlessTxDefersOER(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullMsg = &mockOer{err: common.NewOERMessageError("ORA-24776", "resume failed")}

	tx, err := conn.ResumeSessionlessTx(context.Background(), []byte("resume-global-transaction-id"), 300)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx failed despite deferred response: %v", err)
	}
	if tx == nil || !conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx should register a deferred transaction")
	}
}

// TestSuspendSessionlessTx verifies that suspending the active sessionless
// transaction uses the immediate OTXSE detach path, clears the transaction
// global transaction ID, and unregisters the transaction from the connection shelf.
func TestSuspendSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	mockStr.pushedMsg.Init()

	if err := tx.Suspend(); err != nil {
		t.Fatalf("Suspend failed: %v", err)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID after suspend = %q, want empty", got)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("Suspend should unregister the active transaction from the shelf")
	}
	if mockStr.pushedMsg.Len() != 1 {
		t.Fatalf("pushed message count = %d, want 1", mockStr.pushedMsg.Len())
	}
	if len(mockStr.pullTypes) != 3 || mockStr.pullTypes[0] != TTIRPA || mockStr.pullTypes[1] != TTIOER || mockStr.pullTypes[2] != TTISTA {
		t.Fatalf("pull types = %v, want [TTIRPA TTIOER TTISTA]", mockStr.pullTypes)
	}

	pushedMsg := mockStr.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
	otxse, ok := (*pushedMsg).(*tTIOtxse)
	if !ok {
		t.Fatalf("pushed message = %T, want *tTIOtxse", *pushedMsg)
	}
	if otxse.operation != otxseDetach {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseDetach)
	}
	if otxse.flags != otxseTransSessionless {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, otxseTransSessionless)
	}
	if len(otxse.xid) != 0 {
		t.Fatalf("OTXSE xid length = %d, want 0", len(otxse.xid))
	}
	if otxse.globalTransactionIDLength != 0 {
		t.Fatalf("OTXSE globalTransactionIDLength = %d, want 0", otxse.globalTransactionIDLength)
	}
	if otxse.timeout != 0 {
		t.Fatalf("OTXSE timeout = %d, want 0", otxse.timeout)
	}
}

// TestSuspendNonSessionlessTx verifies that regular transactions do not expose
// the sessionless Suspend API.
func TestSuspendNonSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	if _, ok := tx.(*sessionlessTransaction); ok {
		t.Fatalf("BeginTx returned %T, want regular transaction", tx)
	}
}

// TestSuspendSessionlessTxPushFailure verifies that a write failure while
// sending the OTXSE detach unregisters the local sessionless transaction.
func TestSuspendSessionlessTxPushFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	mockStr.pushedMsg.Init()
	mockStr.pushErr = errors.New("push failed")

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on push failure")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.ErrorInTransaction)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want empty", got)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("Suspend push failure should have unregistered the transaction")
	}
}

// TestSuspendSessionlessTxFlushFailure verifies that a flush failure while
// sending the OTXSE detach unregisters the local sessionless transaction.
func TestSuspendSessionlessTxFlushFailure(t *testing.T) {
	t.Parallel()

	baseConn, baseMock := newSessionlessTransactionTestConnection()
	streamer := &mockStreamerWithFlushError{
		mockStreamer: baseMock,
	}
	baseConn.shelf.RegisterMessageStreamer(streamer)

	tx, err := baseConn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	streamer.pushedMsg.Init()
	streamer.flushErr = errors.New("flush failed")

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on flush failure")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.ErrorInTransaction)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want empty", got)
	}
	if baseConn.shelf.isInTransaction() {
		t.Fatal("Suspend flush failure should have unregistered the transaction")
	}
}

// TestSuspendSessionlessTxPullFailure verifies that a read failure while waiting
// for the detach response unregisters the local sessionless transaction.
func TestSuspendSessionlessTxPullFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	mockStr.pushedMsg.Init()
	mockStr.pullErr = errors.New("pull failed")

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on pull failure")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.ErrorInTransaction)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want empty", got)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("Suspend pull failure should have unregistered the transaction")
	}
}

// TestSuspendSessionlessTxOERFailure verifies that a server-side detach error is
// surfaced and unregisters the local sessionless transaction.
func TestSuspendSessionlessTxOERFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	mockStr.pushedMsg.Init()
	mockStr.pullMsg = &mockOer{err: common.NewOERMessageError("ORA-24776", "detach failed")}

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on OER failure")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.ErrorInTransaction)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want empty", got)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("Suspend OER failure should have unregistered the transaction")
	}
}

// TestSessionlessTransactionOperationErrors verifies that commit and rollback
// retain or remove the local transaction according to server-side state.
func TestSessionlessTransactionOperationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		operation              func(*sessionlessTransaction) error
		serverTransactionState endOfCallStatusTransactionState
		transactionState       transactionState
		wantLocalTransaction   bool
	}{
		{
			name:                   "commit with transaction active on server",
			operation:              func(tx *sessionlessTransaction) error { return tx.Commit() },
			serverTransactionState: active,
			transactionState:       transactionStartedServer,
			wantLocalTransaction:   true,
		},
		{
			name:                   "commit with transaction ended on server",
			operation:              func(tx *sessionlessTransaction) error { return tx.Commit() },
			serverTransactionState: inactive,
			transactionState:       transactionEndedServer,
			wantLocalTransaction:   false,
		},
		{
			name:                   "rollback with transaction active on server",
			operation:              func(tx *sessionlessTransaction) error { return tx.Rollback() },
			serverTransactionState: active,
			transactionState:       transactionStartedServer,
			wantLocalTransaction:   true,
		},
		{
			name:                   "rollback with transaction ended on server",
			operation:              func(tx *sessionlessTransaction) error { return tx.Rollback() },
			serverTransactionState: inactive,
			transactionState:       transactionEndedServer,
			wantLocalTransaction:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			streamer.pullMsg = &mockOer{err: errors.New("transaction operation failed")}
			tx := newSessionlessTransaction(context.Background(), conn, []byte("sessionless-id"), 300)
			conn.shelf.registerTransaction(tx)
			conn._transactionState = tt.serverTransactionState
			tx.transactionState = tt.transactionState

			if got := transactionErrorCode(t, tt.operation(tx)); got != oracleErrors.ErrorInTransaction {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.ErrorInTransaction)
			}
			if got := conn.shelf.isInTransaction(); got != tt.wantLocalTransaction {
				t.Fatalf("local transaction registration = %v, want %v", got, tt.wantLocalTransaction)
			}
		})
	}
}

// TestSuspendSessionlessTxErrorRegistration verifies that a detach failure
// retains the local transaction only while it is still active on the server
// and has not received an end notification.
func TestSuspendSessionlessTxErrorRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		serverTransactionState endOfCallStatusTransactionState
		transactionState       transactionState
		wantLocalTransaction   bool
	}{
		{
			name:                   "transaction active on server",
			serverTransactionState: active,
			transactionState:       transactionStartedServer,
			wantLocalTransaction:   true,
		},
		{
			name:                   "transaction ended on server",
			serverTransactionState: inactive,
			transactionState:       transactionEndedServer,
			wantLocalTransaction:   false,
		},
		{
			name:                   "sessionless end notification received",
			serverTransactionState: active,
			transactionState:       transactionEndedServer,
			wantLocalTransaction:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			streamer.pushErr = errors.New("detach failed")
			tx := newSessionlessTransaction(context.Background(), conn, []byte("sessionless-id"), 300)
			conn.shelf.registerTransaction(tx)
			conn._transactionState = tt.serverTransactionState
			tx.transactionState = tt.transactionState

			if got := transactionErrorCode(t, tx.Suspend()); got != oracleErrors.ErrorInTransaction {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.ErrorInTransaction)
			}
			if got := conn.shelf.isInTransaction(); got != tt.wantLocalTransaction {
				t.Fatalf("local transaction registration = %v, want %v", got, tt.wantLocalTransaction)
			}
		})
	}
}

// TestSessionlessTransactionOperationsRejectStaleTransaction verifies that
// sessionless transaction operations do not act on a different transaction
// registered on the connection.
func TestSessionlessTransactionOperationsRejectStaleTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation func(*sessionlessTransaction) error
	}{
		{name: "commit", operation: func(tx *sessionlessTransaction) error { return tx.Commit() }},
		{name: "rollback", operation: func(tx *sessionlessTransaction) error { return tx.Rollback() }},
		{name: "suspend", operation: func(tx *sessionlessTransaction) error { return tx.Suspend() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			staleTransaction := newSessionlessTransaction(context.Background(), conn, []byte("stale"), 300)
			currentTransaction := newSessionlessTransaction(context.Background(), conn, []byte("current"), 300)
			conn.shelf.registerTransaction(currentTransaction)

			if got := transactionErrorCode(t, tt.operation(staleTransaction)); got != oracleErrors.NotCurrentTransaction {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.NotCurrentTransaction)
			}
			if streamer.pushCalled {
				t.Fatalf("stale %s should not send a transaction message", tt.name)
			}
			if conn.shelf.getTransaction() != currentTransaction {
				t.Fatalf("stale %s changed the current transaction", tt.name)
			}
		})
	}
}

// TestSessionlessTransactionRejectsReuseAfterEndingOperation verifies that
// commit and rollback cannot reuse a sessionless transaction handle after
// suspend, commit, or rollback, while repeated Suspend is a no-op.
func TestSessionlessTransactionRejectsReuseAfterEndingOperation(t *testing.T) {
	t.Parallel()

	endingOperations := []struct {
		name      string
		operation func(*sessionlessTransaction) error
	}{
		{name: "suspend", operation: func(tx *sessionlessTransaction) error { return tx.Suspend() }},
		{name: "commit", operation: func(tx *sessionlessTransaction) error { return tx.Commit() }},
		{name: "rollback", operation: func(tx *sessionlessTransaction) error { return tx.Rollback() }},
	}
	followUpOperations := []struct {
		name      string
		operation func(*sessionlessTransaction) error
	}{
		{name: "commit", operation: func(tx *sessionlessTransaction) error { return tx.Commit() }},
		{name: "rollback", operation: func(tx *sessionlessTransaction) error { return tx.Rollback() }},
	}

	for _, endingOperation := range endingOperations {
		t.Run(endingOperation.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			transaction, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("BeginSessionlessTx failed: %v", err)
			}
			tx, ok := transaction.(*sessionlessTransaction)
			if !ok {
				t.Fatalf("BeginSessionlessTx returned %T, want *sessionlessTransaction", transaction)
			}
			streamer.pushedMsg.Init()

			if err := endingOperation.operation(tx); err != nil {
				t.Fatalf("%s failed: %v", endingOperation.name, err)
			}
			messageCount := streamer.pushedMsg.Len()

			for _, followUpOperation := range followUpOperations {
				t.Run(followUpOperation.name, func(t *testing.T) {
					if got := transactionErrorCode(t, followUpOperation.operation(tx)); got != oracleErrors.NotInTransaction {
						t.Fatalf("error code after %s = %s, want %s", endingOperation.name, got, oracleErrors.NotInTransaction)
					}
					if got := streamer.pushedMsg.Len(); got != messageCount {
						t.Fatalf("messages pushed after reusing transaction following %s = %d, want %d", endingOperation.name, got, messageCount)
					}
				})
			}

			if err := tx.Suspend(); err != nil {
				t.Fatalf("suspend after %s = %v, want no error", endingOperation.name, err)
			}
			if got := streamer.pushedMsg.Len(); got != messageCount {
				t.Fatalf("messages pushed after suspend following %s = %d, want %d", endingOperation.name, got, messageCount)
			}
		})
	}
}
