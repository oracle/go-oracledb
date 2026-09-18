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
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func assertDeferredTransactionStart(t *testing.T, streamer *mockStreamer, opts driver.TxOptions) {
	t.Helper()
	if streamer.pushedMsg.Len() != 1 {
		t.Fatalf("BeginTx pushed %d messages, want 1", streamer.pushedMsg.Len())
	}
	if streamer.pullCalled {
		t.Fatal("BeginTx should not pull a response for a deferred start")
	}

	msg := streamer.pushedMsg.Front().Value.(*common.Message[common.MessageType])
	otxse, ok := (*msg).(*tTIOtxse)
	if !ok {
		t.Fatalf("BeginTx message = %T, want *tTIOtxse", *msg)
	}
	if otxse.GetMsgCode() != TTIPFN {
		t.Fatalf("BeginTx message code = %v, want TTIPFN", otxse.GetMsgCode())
	}
	if otxse.operation != otxseStart {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseStart)
	}
	if otxse.flags != convertTxOptionsToFlags(opts) {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, convertTxOptionsToFlags(opts))
	}
}

// TestTransactionCommitSuccess verifies that BeginTx queues a deferred OTXSE
// start and that commit succeeds for all supported transaction options.
func TestTransactionCommitSuccess(t *testing.T) {
	t.Parallel()
	tests := []driver.TxOptions{
		{Isolation: driver.IsolationLevel(sql.LevelReadCommitted), ReadOnly: true},
		{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)},
		{Isolation: driver.IsolationLevel(sql.LevelSerializable), ReadOnly: true},
		{Isolation: driver.IsolationLevel(sql.LevelSerializable)},
	}

	for _, opts := range tests {
		name := "read-write"
		if opts.ReadOnly {
			name = "read-only"
		}
		t.Run(name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsg: &mockOer{}}
			conn := newTransactionTestConnection(streamer)

			tx, err := conn.BeginTx(context.Background(), opts)
			if err != nil {
				t.Fatalf("BeginTx returned error: %v", err)
			}
			assertDeferredTransactionStart(t, streamer, opts)

			streamer.pushedMsg.Init()
			if err := tx.Commit(); err != nil {
				t.Fatalf("Commit returned error: %v", err)
			}
			assertTransactionFunction(t, streamer, common.SB4(otxenCommit), k2cmdCommit)
			if got := transactionErrorCode(t, tx.Commit()); got != oracleErrors.NotInTransaction {
				t.Fatalf("second Commit error code = %s, want %s", got, oracleErrors.NotInTransaction)
			}
		})
	}
}

// TestTransactionRollbackSuccess verifies that BeginTx queues a deferred OTXSE
// start and that rollback succeeds for all supported transaction options.
func TestTransactionRollbackSuccess(t *testing.T) {
	t.Parallel()
	tests := []driver.TxOptions{
		{Isolation: driver.IsolationLevel(sql.LevelReadCommitted), ReadOnly: true},
		{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)},
		{Isolation: driver.IsolationLevel(sql.LevelSerializable), ReadOnly: true},
		{Isolation: driver.IsolationLevel(sql.LevelSerializable)},
	}

	for _, opts := range tests {
		name := "read-write"
		if opts.ReadOnly {
			name = "read-only"
		}
		t.Run(name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsg: &mockOer{}}
			conn := newTransactionTestConnection(streamer)

			tx, err := conn.BeginTx(context.Background(), opts)
			if err != nil {
				t.Fatalf("BeginTx returned error: %v", err)
			}
			assertDeferredTransactionStart(t, streamer, opts)

			streamer.pushedMsg.Init()
			if err := tx.Rollback(); err != nil {
				t.Fatalf("Rollback returned error: %v", err)
			}
			assertTransactionFunction(t, streamer, common.SB4(otxenAbort), k2cmdAbort)
			if got := transactionErrorCode(t, tx.Rollback()); got != oracleErrors.NotInTransaction {
				t.Fatalf("second Rollback error code = %s, want %s", got, oracleErrors.NotInTransaction)
			}
		})
	}
}

// TestTransactionEndConsumesOTxEnRPA verifies that transaction end consumes
// OTXEN return parameters before reading the terminal status message.
func TestTransactionEndConsumesOTxEnRPA(t *testing.T) {
	t.Parallel()
	streamer := &mockStreamer{
		pullMsgs: []common.Message[common.MessageType]{
			&ttiOTxEnRPA{outState: 1},
			&mockOer{},
		},
	}
	conn := newTransactionTestConnection(streamer)

	tx, err := conn.BeginTx(context.Background(), driver.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx returned error: %v", err)
	}
	streamer.pushedMsg.Init()
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if len(streamer.pullTypes) != 3 || streamer.pullTypes[0] != TTIRPA || streamer.pullTypes[1] != TTIOER || streamer.pullTypes[2] != TTISTA {
		t.Fatalf("OTXEN pull types = %v, want [TTIRPA TTIOER TTISTA]", streamer.pullTypes)
	}
}

func assertTransactionFunction(t *testing.T, streamer *mockStreamer, operation common.SB4, inState common.UB4) {
	t.Helper()
	if streamer.pushedMsg.Len() != 1 {
		t.Fatalf("transaction operation pushed %d messages, want 1", streamer.pushedMsg.Len())
	}
	msg := streamer.pushedMsg.Front().Value.(*common.Message[common.MessageType])
	otxen, ok := (*msg).(*tTIOtxen)
	if !ok {
		t.Fatalf("transaction operation message = %T, want *tTIOtxen", *msg)
	}
	if otxen.operation != operation {
		t.Fatalf("OTXEN operation = %d, want %d", otxen.operation, operation)
	}
	if otxen.inState != inState {
		t.Fatalf("OTXEN in-state = %d, want %d", otxen.inState, inState)
	}
	if otxen.flags != 0 {
		t.Fatalf("OTXEN transaction state change flags = %d, want 0", otxen.flags)
	}
}

// TestCallBeginTxTwice verifies that beginning a second transaction on the same
// connection returns an already-in-transaction error.
func TestCallBeginTxTwice(t *testing.T) {
	t.Parallel()
	mockNs := &mockNetworkSession{disconnectCalls: 0, disconnectErr: nil, sleepDuration: 0}
	messageRegistry := NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIOER, 1, newTTIoer)
	functionRegistry := NewRegistry[functionRegistryKey]()
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: logOff}, 1, newLogOff)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: rollback}, 1, newRollback)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	functionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: oTxSe}, 18, newOTxSePfn18)
	messageFactory := &SimpleFactory{ttcVersion: 18, msgregistry: messageRegistry, funcregistry: functionRegistry}
	mockStr := &mockStreamer{pullMsg: &mockOer{err: nil}}
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).RegisterMessageStreamer(mockStr)

	conn := newTestConnection(shelf, nil, mockNs)

	// Clean error message so that connection close succeeds
	mockStr.pullMsg = &mockOer{}
	_, err := conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted), ReadOnly: false})
	if err != nil {
		t.Fatalf("Unexpected error starting transaction %v", err)
	}

	_, err = conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted), ReadOnly: false})
	if err == nil {
		t.Fatalf("Expected already in transaction error")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Expected error to be OracleError")
	}
	if sqlErr.ErrorCode() != string(oracleErrors.AlreadyInTransaction) {
		t.Fatalf("Wrong error expected %s, but was %s", oracleErrors.AlreadyInTransaction, sqlErr.ErrorCode())
	}

}

func newTransactionTestConnection(streamer *mockStreamer) *connection {
	messageRegistry := NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIOER, 1, newTTIoer)
	functionRegistry := NewRegistry[functionRegistryKey]()
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxEn}, 18, newOTxEn18)
	functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxEn}, 1, newOTxEnRPA)
	functionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: oTxSe}, 18, newOTxSePfn18)
	messageFactory := &SimpleFactory{ttcVersion: 18, msgregistry: messageRegistry, funcregistry: functionRegistry}
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).RegisterMessageStreamer(streamer)
	return newTestConnection(shelf, nil, nil)
}

func transactionErrorCode(t *testing.T, err error) oracleErrors.ErrorCode {
	t.Helper()
	if err == nil {
		t.Fatal("expected transaction error, got nil")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T: %v", err, err)
	}
	return oracleErrors.ErrorCode(sqlErr.ErrorCode())
}

// TestConnectionBeginUsesDefaultIsolationLevel verifies that Begin queues a
// read-committed, read-write OTXSE start when no options are supplied.
func TestConnectionBeginUsesDefaultIsolationLevel(t *testing.T) {
	t.Parallel()
	streamer := &mockStreamer{pullMsg: &mockOer{}}
	conn := newTransactionTestConnection(streamer)

	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	assertDeferredTransactionStart(t, streamer, driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)})
	if tx == nil {
		t.Fatal("Begin returned a nil transaction")
	}
}

// TestConnectionBeginTxRejectsUnsupportedIsolationLevel verifies that an
// unsupported isolation level is rejected without sending a setup message.
func TestConnectionBeginTxRejectsUnsupportedIsolationLevel(t *testing.T) {
	t.Parallel()
	streamer := &mockStreamer{}
	conn := newTransactionTestConnection(streamer)

	_, err := conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(12345)})
	if got := transactionErrorCode(t, err); got != oracleErrors.IsolationLevelNotSupported {
		t.Fatalf("error code = %s, want %s", got, oracleErrors.IsolationLevelNotSupported)
	}
	if streamer.pushCalled {
		t.Fatal("unsupported isolation level should not push a message")
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("unsupported isolation level should not register a transaction")
	}
}

// TestConnectionBeginTxReturnsPushError verifies that an error queuing the
// deferred start operation is returned to the caller.
func TestConnectionBeginTxReturnsPushError(t *testing.T) {
	t.Parallel()
	streamer := &mockStreamer{pushErr: errors.New("start transaction failed")}
	conn := newTransactionTestConnection(streamer)

	_, err := conn.BeginTx(context.Background(), driver.TxOptions{})
	if got := transactionErrorCode(t, err); got != oracleErrors.StreamerWriteError {
		t.Fatalf("error code = %s, want %s", got, oracleErrors.StreamerWriteError)
	}
	if streamer.pushedMsg.Len() != 1 {
		t.Fatalf("pushed messages = %d, want 1", streamer.pushedMsg.Len())
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("BeginTx push failure should unregister the transaction")
	}
}

// TestTransactionOperationErrors verifies that commit and rollback errors are
// wrapped as transaction errors and that the local registration follows the
// transaction state reported by the server.
func TestTransactionOperationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		operation            func(*transaction) error
		message              string
		serverInTransaction  bool
		wantLocalTransaction bool
	}{
		{
			name:                 "commit with transaction ended on server",
			operation:            (*transaction).Commit,
			message:              "commit failed",
			serverInTransaction:  false,
			wantLocalTransaction: false,
		},
		{
			name:                 "commit with transaction active on server",
			operation:            (*transaction).Commit,
			message:              "commit failed",
			serverInTransaction:  true,
			wantLocalTransaction: true,
		},
		{
			name:                 "rollback with transaction ended on server",
			operation:            (*transaction).Rollback,
			message:              "rollback failed",
			serverInTransaction:  false,
			wantLocalTransaction: false,
		},
		{
			name:                 "rollback with transaction active on server",
			operation:            (*transaction).Rollback,
			message:              "rollback failed",
			serverInTransaction:  true,
			wantLocalTransaction: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsg: &mockOer{err: errors.New(tt.message)}}
			conn := newTransactionTestConnection(streamer)
			conn._isInTransaction = tt.serverInTransaction
			tx := newTransaction(conn, context.Background())
			conn.shelf.registerTransaction(tx)

			if got := transactionErrorCode(t, tt.operation(tx)); got != oracleErrors.ErrorInTransaction {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.ErrorInTransaction)
			}
			if got := conn.shelf.isInTransaction(); got != tt.wantLocalTransaction {
				t.Fatalf("local transaction registration = %v, want %v", got, tt.wantLocalTransaction)
			}
		})
	}
}

// TestTransactionOperationRejectsStaleMessages verifies that commit and
// rollback fail when connection validation reports stale messages.
func TestTransactionOperationRejectsStaleMessages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		operation func(*transaction) error
	}{
		{name: "commit", operation: (*transaction).Commit},
		{name: "rollback", operation: (*transaction).Rollback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsg: &mockOer{}, drainIn: 1}
			conn := newTransactionTestConnection(streamer)
			tx := newTransaction(conn, context.Background())
			conn.shelf.registerTransaction(tx)
			conn.shelf.registerStateValidator(&shelfConnectionValidator{valid: false})

			if got := transactionErrorCode(t, tt.operation(tx)); got != oracleErrors.InternalError {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.InternalError)
			}
		})
	}
}

// TestTransactionOperationsRejectStaleTransactions verifies that transaction
// operations do not act on a different transaction registered on the connection.
func TestTransactionOperationsRejectStaleTransactions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation func(*transaction) error
	}{
		{name: "commit", operation: (*transaction).Commit},
		{name: "rollback", operation: (*transaction).Rollback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamer := &mockStreamer{pullMsg: &mockOer{}}
			conn := newTransactionTestConnection(streamer)
			staleTransaction := newTransaction(conn, context.Background())
			currentTransaction := newTransaction(conn, context.Background())
			conn.shelf.registerTransaction(currentTransaction)

			if got := transactionErrorCode(t, tt.operation(staleTransaction)); got != oracleErrors.NotInTransaction {
				t.Fatalf("error code = %s, want %s", got, oracleErrors.NotInTransaction)
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

// TestPromotedTransactionPreservesIdentity verifies that a regular transaction
// handle remains current after it is promoted to a sessionless transaction.
func TestPromotedTransactionPreservesIdentity(t *testing.T) {
	t.Parallel()

	conn := newTransactionTestConnection(&mockStreamer{})
	regularTransaction := newTransaction(conn, context.Background())
	promotedTransaction := upgradeFromTransaction(regularTransaction, nil, 0)
	conn.shelf.registerTransaction(promotedTransaction)

	if !regularTransaction.isCurrentTransaction() {
		t.Fatal("regular transaction handle is not current after promotion")
	}
	if !promotedTransaction.isCurrentTransaction() {
		t.Fatal("promoted transaction handle is not current")
	}
	if regularTransaction.transactionIdentity() != promotedTransaction.transactionIdentity() {
		t.Fatal("regular and promoted transaction handles do not share identity")
	}

	staleTransaction := newTransaction(conn, context.Background())
	if staleTransaction.isCurrentTransaction() {
		t.Fatal("unrelated transaction handle should not be current after promotion")
	}
}
