/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** and patent rights owned by each licensor hereunder covering either (i) the
** unmodified Software as contributed to or provided by such licensor, or (ii)
** the Larger Works (as defined below), to deal in both (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors), without restriction, including without
** limitation the rights to copy, create derivative works of, display, perform,
** and distribute the Software and the Larger Work(s), and to sublicense the
** foregoing rights on either these or other terms.
**
** This license is subject to the condition that the above copyright notice and
** either this complete permission notice or at a minimum a reference to the UPL
** must be included in all copies or substantial portions of the Software.
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
	"database/sql"
	"errors"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestSessionlessTransactionBeginResumeValidation covers connection-state,
// capability, and isolation-level validation for both public start paths.
func TestSessionlessTransactionBeginResumeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*connection)
		operation func(*connection) error
		want      oracleErrors.ErrorCode
	}{
		{
			name: "begin rejects an active transaction",
			setup: func(conn *connection) {
				conn.shelf.registerTransaction(newTransaction(conn, context.Background()))
			},
			operation: func(conn *connection) error {
				_, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
				return err
			},
			want: oracleErrors.AlreadyInTransaction,
		},
		{
			name: "begin rejects an unsupported capability set",
			setup: func(conn *connection) {
				conn.shelf.RegisterCapabilities(map[string]driverCommon.Capability{})
			},
			operation: func(conn *connection) error {
				_, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
				return err
			},
			want: oracleErrors.UnsupportedFeature,
		},
		{
			name:  "begin rejects an unsupported isolation level",
			setup: func(*connection) {},
			operation: func(conn *connection) error {
				_, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{
					Isolation: sql.LevelReadUncommitted,
				}, 300)
				return err
			},
			want: oracleErrors.IsolationLevelNotSupported,
		},
		{
			name: "resume rejects an active transaction",
			setup: func(conn *connection) {
				conn.shelf.registerTransaction(newTransaction(conn, context.Background()))
			},
			operation: func(conn *connection) error {
				_, err := conn.ResumeSessionlessTx(context.Background(), []byte("resume-id"), 300)
				return err
			},
			want: oracleErrors.AlreadyInTransaction,
		},
		{
			name: "resume rejects an unsupported capability set",
			setup: func(conn *connection) {
				conn.shelf.RegisterCapabilities(map[string]driverCommon.Capability{})
			},
			operation: func(conn *connection) error {
				_, err := conn.ResumeSessionlessTx(context.Background(), []byte("resume-id"), 300)
				return err
			},
			want: oracleErrors.UnsupportedFeature,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			test.setup(conn)

			if got := transactionErrorCode(t, test.operation(conn)); got != test.want {
				t.Fatalf("error code = %s, want %s", got, test.want)
			}
		})
	}
}

// TestSessionlessTransactionXIDTruncation verifies that oversized global
// transaction IDs and connection instance names are truncated to the TTC XID
// field limits while preserving the corresponding lengths.
func TestSessionlessTransactionXIDTruncation(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	globalTransactionID := strings.Repeat("g", maxSessionlessGlobalTransactionIDSize+8)
	instanceNameValue := strings.Repeat("i", maxSessionlessBQUALSize+8)
	conn.sessCtx.GetSessionProperties().SetProperty(instanceName, instanceNameValue)

	tx := newSessionlessTransaction(context.Background(), conn, []byte(globalTransactionID), 300)
	if got, want := tx.globalTransactionIDLength, driverCommon.UB4(maxSessionlessGlobalTransactionIDSize); got != want {
		t.Fatalf("global transaction ID length = %d, want %d", got, want)
	}
	if got, want := tx.bqualLength, driverCommon.UB4(maxSessionlessBQUALSize); got != want {
		t.Fatalf("BQUAL length = %d, want %d", got, want)
	}
	if !bytes.Equal(tx.xid[:maxSessionlessGlobalTransactionIDSize], []byte(globalTransactionID[:maxSessionlessGlobalTransactionIDSize])) {
		t.Fatal("XID does not contain the truncated global transaction ID")
	}
	if !bytes.Equal(tx.xid[maxSessionlessGlobalTransactionIDSize:], []byte(instanceNameValue[:maxSessionlessBQUALSize])) {
		t.Fatal("XID does not contain the truncated instance name")
	}
}

// TestSessionlessTransactionLifecycleNoOpPaths verifies that watcher setup and
// cancellation cleanup do nothing for background contexts, ended transaction
// states, duplicate watchers, and stale transaction handles.
func TestSessionlessTransactionLifecycleNoOpPaths(t *testing.T) {
	t.Parallel()

	conn, streamer := newSessionlessTransactionTestConnection()
	tx := newSessionlessTransaction(context.Background(), conn, []byte("sessionless-id"), 300)
	tx.SetRunningFromSessionlessTx(true)
	if !tx.fromSessionlessTx {
		t.Fatal("SetRunningFromSessionlessTx did not enable wrapper authorization")
	}
	tx.transactionState = transactionEndedClient
	if !tx.IsTransactionEnded() {
		t.Fatal("client-ended state was not reported as ended")
	}
	tx.startContextWatcherLocked()
	if tx.contextWatcherStop != nil {
		t.Fatal("background context installed a cancellation watcher")
	}

	tx.transactionState = transactionEndedClient
	tx.startContextWatcherLocked()
	if tx.contextWatcherStop != nil {
		t.Fatal("ending transaction installed a cancellation watcher")
	}

	tx.transactionState = transactionStartedClient
	tx.contextWatcherStop = func() bool { return true }
	tx.startContextWatcherLocked()
	if tx.contextWatcherStop == nil {
		t.Fatal("duplicate watcher setup replaced the existing watcher")
	}
	tx.contextWatcherStop = nil

	conn.shelf.registerTransaction(tx)
	tx.transactionState = transactionEndedClient
	tx.rollbackOnContextCancellation()
	if streamer.pushCalled {
		t.Fatal("ended transaction cancellation attempted a rollback")
	}

	stale := newSessionlessTransaction(context.Background(), conn, []byte("stale-id"), 300)
	conn.shelf.registerTransaction(tx)
	stale.rollbackOnContextCancellation()
	if streamer.pushCalled {
		t.Fatal("stale transaction cancellation attempted a rollback")
	}
}

// TestSuspendSessionlessTransactionCancellationCleanup verifies that a failed
// suspend with a canceled context attempts bounded rollback cleanup and
// invalidates the connection when the cleanup result is ambiguous.
func TestSuspendSessionlessTransactionCancellationCleanup(t *testing.T) {
	t.Parallel()

	conn, streamer := newSessionlessTransactionTestConnection()
	streamer.pullMsg = &mockOer{err: errors.New("detach failed")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tx, err := conn.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}
	cancel()

	if got := transactionErrorCode(t, tx.Suspend()); got != oracleErrors.ErrorInTransaction {
		t.Fatalf("Suspend error code = %s, want %s", got, oracleErrors.ErrorInTransaction)
	}
	if conn._isValid {
		t.Fatal("connection remained valid after ambiguous canceled suspend cleanup")
	}
}

// TestSuspendSessionlessTransactionStateValidation verifies that a successful
// detach followed by an invalid connection-state check unregisters the local
// transaction and returns the validator error.
func TestSuspendSessionlessTransactionStateValidation(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}
	conn.shelf.registerStateValidator(&shelfConnectionValidator{valid: false})

	if got := transactionErrorCode(t, tx.Suspend()); got != oracleErrors.InternalError {
		t.Fatalf("Suspend error code = %s, want %s", got, oracleErrors.InternalError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("invalid connection state retained the suspended transaction")
	}
}

type sessionlessTransactionBasicStreamer struct{}

// Push implements the basic streamer used to exercise callback support checks.
func (*sessionlessTransactionBasicStreamer) Push(context.Context, driverCommon.Message[driverCommon.MessageType]) error {
	return nil
}

// Pull implements the basic streamer used to exercise callback support checks.
func (*sessionlessTransactionBasicStreamer) Pull(context.Context, ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
	return nil, errors.New("basic streamer does not pull messages")
}

// Flush implements the basic streamer used to exercise callback support checks.
func (*sessionlessTransactionBasicStreamer) Flush(context.Context) error { return nil }

// Drain implements the basic streamer used to exercise callback support checks.
func (*sessionlessTransactionBasicStreamer) Drain(context.Context, driverCommon.StreamDirection) (int, int) {
	return 0, 0
}

// TestSessionlessTransactionMessageSetupFailures verifies the internal resume
// and detach helpers' setup failures: missing callback support, factory errors,
// and unexpected message types.
func TestSessionlessTransactionMessageSetupFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*connection)
		operation func(*connection, *sessionlessTransaction) error
	}{
		{
			name: "resume requires callback support",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageStreamer(&sessionlessTransactionBasicStreamer{})
			},
			operation: func(conn *connection, tx *sessionlessTransaction) error {
				return conn.resumeSessionlessTx(tx.transactionContext(), tx)
			},
		},
		{
			name: "resume factory error",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
			},
			operation: func(conn *connection, tx *sessionlessTransaction) error {
				return conn.resumeSessionlessTx(tx.transactionContext(), tx)
			},
		},
		{
			name: "resume unexpected message type",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnMsg: &dummyMsg{}})
			},
			operation: func(conn *connection, tx *sessionlessTransaction) error {
				return conn.resumeSessionlessTx(tx.transactionContext(), tx)
			},
		},
		{
			name: "detach requires callback support",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageStreamer(&sessionlessTransactionBasicStreamer{})
			},
			operation: func(conn *connection, tx *sessionlessTransaction) error {
				return conn.detachTransaction(tx.transactionContext())
			},
		},
		{
			name: "detach factory error",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
			},
			operation: func(conn *connection, tx *sessionlessTransaction) error {
				return conn.detachTransaction(tx.transactionContext())
			},
		},
		{
			name: "detach unexpected message type",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnMsg: &dummyMsg{}})
			},
			operation: func(conn *connection, tx *sessionlessTransaction) error {
				return conn.detachTransaction(tx.transactionContext())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			test.setup(conn)
			tx := newSessionlessTransaction(context.Background(), conn, []byte("sessionless-id"), 300)

			if got := transactionErrorCode(t, test.operation(conn, tx)); got != oracleErrors.InternalError {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.InternalError)
			}
		})
	}
}

// TestSessionlessTransactionDetachResponsePaths verifies that detach ignores
// an intermediate TTIRPA response and completes successfully on TTISTA.
func TestSessionlessTransactionDetachResponsePaths(t *testing.T) {
	t.Parallel()

	conn, streamer := newSessionlessTransactionTestConnection()
	streamer.pullMsgs = []driverCommon.Message[driverCommon.MessageType]{
		newOTxSeRPA(),
		newTTISTA(),
	}

	tx := newSessionlessTransaction(context.Background(), conn, []byte("sessionless-id"), 300)
	if err := conn.detachTransaction(tx.transactionContext()); err != nil {
		t.Fatalf("detachTransaction returned error: %v", err)
	}
}
