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

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type oracleTx interface {
	// transactionContext returns the context associated with the transaction.
	transactionContext() context.Context
	// underlyingConnection returns the connection associated with the transaction.
	underlyingConnection() *connection
	// transactionIdentity returns the identity of the transaction
	transactionIdentity() *transaction
}

// transactionState tracks the local and server-visible lifecycle of a
// transaction. The server-started and server-ended states are set by the
// server acknowledgment path: End-of-Call status for standard transactions
// and SESSIONLESS_GTRID session-property changes for sessionless transactions.
type transactionState uint8

const (
	// transactionStartedClient means the client has requested the transaction,
	// but the server has not acknowledged it yet.
	transactionStartedClient transactionState = iota
	// transactionStartedServer means the server has acknowledged the start.
	transactionStartedServer
	// transactionEndedClient means a client-side ending operation is in progress.
	transactionEndedClient
	// transactionEndedServer means the server has acknowledged the end.
	transactionEndedServer
)

func (state transactionState) isStarted() bool {
	return state == transactionStartedClient || state == transactionStartedServer
}

type transaction struct {
	// the underlying connection
	_underlyingConnection *connection
	// the current transaction context
	_transactionContext context.Context
	// _transactionIdentity identifies the transaction. The identity is constant even
	// when a regular transaction is promoted to sessionless transaction. It allows
	// the driver to check that the transaction used to execute an operation is the
	// current active transaction on the connection. This prevents a stale
	// transaction from executing an operation on the active transaction.
	_transactionIdentity *transaction
	// the local and server-visible lifecycle state of the transaction
	transactionState transactionState
}

// newTransaction creates a new transaction with the given connection and context.
//
// Parameters:
//   - conn: Connection associated with the transaction.
//   - ctx: Context associated with the transaction.
//
// Returns:
//   - *transaction: Initialized transaction.
func newTransaction(conn *connection, ctx context.Context) *transaction {
	tx := &transaction{
		_underlyingConnection: conn,
		_transactionContext:   ctx,
		transactionState:      transactionStartedClient,
	}
	tx._transactionIdentity = tx
	return tx
}

// transactionContext returns the current transaction context. This function
// can be used by statements to register after functions on the context in case
// the context is cancelled during the execution.
//
// Returns:
//   - context.Context: Transaction context.
func (t *transaction) transactionContext() context.Context {
	return t._transactionContext
}

// underlyingConnection returns the connection associated with the transaction.
//
// Returns:
//   - *connection: Transaction connection.
func (t *transaction) underlyingConnection() *connection {
	return t._underlyingConnection
}

// transactionIdentity returns the stable identity used to determine whether
// this transaction is still registered on its connection. Promoted transaction
// handles retain the identity of the original transaction.
//
// Returns:
//   - *transaction: Transaction identity.
func (t *transaction) transactionIdentity() *transaction {
	if t._transactionIdentity != nil {
		return t._transactionIdentity
	}
	return t
}

// isCurrentTransaction reports whether this transaction is the transaction
// currently registered on its connection.
//
// Returns:
//   - bool: True when this transaction is current; otherwise false.
func (t *transaction) isCurrentTransaction() bool {
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	return currentTransaction != nil && currentTransaction.transactionIdentity() == t.transactionIdentity()
}

// checkCurrentTransaction returns the error that corresponds to this handle's
// relationship with the transaction registered on its connection. A missing
// transaction and a different current transaction are distinct conditions.
//
// Returns:
//   - error: Nil when this transaction is current; otherwise a transaction
//     state error describing the condition.
func (t *transaction) checkCurrentTransaction() error {
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	if currentTransaction == nil {
		return newNotInTransactionError()
	}
	if currentTransaction.transactionIdentity() != t.transactionIdentity() {
		return newNotCurrentTransactionError()
	}
	return nil
}

// restoreTransactionState restores a state saved before a transaction-ending
// operation. A server end notification takes precedence over the restore,
// because it is the authoritative terminal state.
func (t *transaction) restoreTransactionState(state transactionState) {
	if t.transactionState == transactionEndedClient {
		t.transactionState = state
	}
}

// Commit commits the transaction. The client-ended state is set before the
// TTC operation begins; a failed operation restores the previous state, while
// the server acknowledgment path records the server-ended state.
//
// Returns:
//   - error: Error if no transaction is active or the commit fails.
func (t *transaction) Commit() error {
	common.Odl.Debug("Transaction commit")

	// check that the transaction is the active transaction on the connection
	if err := t.checkCurrentTransaction(); err != nil {
		return t.underlyingConnection().shelf.LocalizeError(err)
	}
	// get the current transaction, its context and execute commit
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	ctx := t.transactionContext()
	previousState := t.transactionState
	t.transactionState = transactionEndedClient
	readFuncError := t.underlyingConnection().runOTxEn(ctx, otxenCommit, currentTransaction)

	// validate the current connection state
	if err := t.underlyingConnection().shelf.checkCurrentState(ctx); err != nil {
		t.restoreTransactionState(previousState)
		t.underlyingConnection().unregisterTransactionOnError()
		return err
	}

	// check for errors during the commit round-trip
	if readFuncError != nil {
		t.restoreTransactionState(previousState)
		t.underlyingConnection().unregisterTransactionOnError()
		return t.underlyingConnection().shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, readFuncError, "Commit"))
	}

	// unregister the transaction
	t.underlyingConnection().shelf.unregisterTransaction()
	return nil
}

// Rollback rolls back the transaction. The client-ended state is set before
// the TTC operation begins; a failed operation restores the previous state,
// while the server acknowledgment path records the server-ended state.
//
// Returns:
//   - error: Error if no transaction is active or the rollback fails.
func (t *transaction) Rollback() error {
	return t.rollback(common.BackgroundContext)
}

// rollback rolls back the transaction using the supplied context. The public
// Rollback method uses a background context, while lifecycle cleanup can use a
// bounded context so a cancellation watcher cannot block indefinitely.
func (t *transaction) rollback(ctx context.Context) error {
	common.Odl.Debug("Transaction rollback")

	// check that the transaction is the active transaction on the connection
	if err := t.checkCurrentTransaction(); err != nil {
		return t.underlyingConnection().shelf.LocalizeError(err)
	}
	// get the current transaction and execute rollback
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	previousState := t.transactionState
	t.transactionState = transactionEndedClient
	runFuncErr := t.underlyingConnection().runOTxEn(ctx, otxenAbort, currentTransaction)

	// validate the current connection state
	if err := t.underlyingConnection().shelf.checkCurrentState(ctx); err != nil {
		t.restoreTransactionState(previousState)
		t.underlyingConnection().unregisterTransactionOnError()
		return err
	}

	// check for errors during the rollback round-trip
	if runFuncErr != nil {
		t.restoreTransactionState(previousState)
		t.underlyingConnection().unregisterTransactionOnError()
		return t.underlyingConnection().shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, runFuncErr, "Rollback"))
	}

	t.underlyingConnection().shelf.unregisterTransaction()
	return nil
}

// newNotInTransactionError returns a not-in-transaction error.
//
// Returns:
//   - error: NotInTransaction Oracle error.
func newNotInTransactionError() error {
	return common.NewOracleError(oracleErrors.NotInTransaction, nil, nil)
}

// newNotCurrentTransactionError returns an error for a transaction handle that
// is not the transaction currently registered on its connection.
//
// Returns:
//   - error: NotCurrentTransaction Oracle error.
func newNotCurrentTransactionError() error {
	return common.NewOracleError(oracleErrors.NotCurrentTransaction, nil, nil)
}
