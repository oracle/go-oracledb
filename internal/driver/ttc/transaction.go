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

// oracleTx is the common transaction contract used by TTC transaction
// lifecycle and message code.
type oracleTx interface {
	// transactionContext returns the context associated with the transaction.
	transactionContext() context.Context
	// underlyingConnection returns the connection associated with the transaction.
	underlyingConnection() *connection
	// transactionState changes the local state and returns the previous state.
	transactionState(transactionState) transactionState
	// currentTransactionState returns the current lifecycle state.
	currentTransactionState() transactionState
	// endTransaction records that the server has ended the transaction and
	// applies any transaction-specific local cleanup.
	endTransaction()
}

// transactionState tracks the local and server-visible lifecycle of a
// transaction. The server-started and server-ended states are set by the
// server acknowledgment path: End-of-Call status for standard transactions
// and SESSIONLESS_GTRID session-property changes for sessionless transactions.
type transactionState uint8

const (
	// transactionPrepared means that the transaction has been prepared but not
	// yet started (or resumed)
	transactionPrepared transactionState = iota
	// transactionStartedClient means the client has requested the transaction,
	// but the server has not acknowledged it yet.
	transactionStartedClient
	// transactionStartedServer means the server has acknowledged the start.
	transactionStartedServer
	// transactionEndedClient means a client-side ending operation is in progress.
	transactionEndedClient
	// transactionEndedServer means the server has acknowledged the end.
	transactionEndedServer
)

type transaction struct {
	// _underlyingConnection is the connection that owns this transaction.
	_underlyingConnection *connection
	// _transactionContext is the context supplied when the transaction began.
	_transactionContext context.Context
	// _transactionState is the local lifecycle state and server acknowledgment.
	_transactionState transactionState
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
	return &transaction{
		_underlyingConnection: conn,
		_transactionContext:   ctx,
		_transactionState:     transactionPrepared,
	}
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

// transactionState stores a lifecycle state and returns the previous one. It
// is not synchronized; connection operations must be serialized.
func (t *transaction) transactionState(state transactionState) transactionState {
	previousState := t._transactionState
	t._transactionState = state
	return previousState
}

// currentTransactionState returns the current lifecycle state. Access relies
// on serialized connection operations; concurrent use of a connection is not
// supported.
func (t *transaction) currentTransactionState() transactionState {
	return t._transactionState
}

// endTransaction records the server-confirmed end of a standard transaction.
func (t *transaction) endTransaction() {
	t.transactionState(transactionEndedServer)
	t.underlyingConnection().shelf.unregisterTransaction()
}

// Commit commits the transaction. The client-ended state is set before the
// TTC operation begins; a failed operation restores the previous state, while
// the server acknowledgment path records the server-ended state.
//
// Returns:
//   - error: Error if no transaction is active or the commit fails.
func (t *transaction) Commit() error {
	return commitTransaction(t)
}

// commitTransaction commits the current transaction and updates local state
// from the server response.
func commitTransaction(t oracleTx) error {
	common.Odl.Fine("Transaction commit")

	// get the current transaction, its context and execute commit
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	ctx := t.transactionContext()
	previousState := t.transactionState(transactionEndedClient)
	readFuncError := t.underlyingConnection().runTransactionEndingOperation(ctx, otxenCommit, currentTransaction)

	// check for errors during the commit round-trip
	if readFuncError != nil {
		if t.currentTransactionState() != transactionEndedServer {
			common.Odl.Fine("An error occurred during transaction commit, changing transaction state back to previous state", "transaction state", previousState)
			t.transactionState(previousState)
		}
		return t.underlyingConnection().shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, readFuncError, "Commit"))
	}

	return nil
}

// Rollback rolls back the transaction. The client-ended state is set before
// the TTC operation begins; a failed operation restores the previous state
// unless the server has already acknowledged that the transaction ended.
//
// Returns:
//   - error: Error if no transaction is active or the rollback fails.
func (t *transaction) Rollback() error {
	common.Odl.Fine("Transaction rollback")
	return endTransaction(t, common.BackgroundContext, t.rollback)
}

// rollback sends the transaction abort operation using ctx.
func (t *transaction) rollback(ctx context.Context) error {
	return t.underlyingConnection().runTransactionEndingOperation(ctx, otxenAbort, t)
}

// endTransaction runs a transaction-ending operation and reconciles local
// state with the server response.
func endTransaction(t oracleTx, ctx context.Context, transactionEndingFunc func(context.Context) error) error {
	// get the current transaction and execute rollback
	previousState := t.transactionState(transactionEndedClient)
	runFuncErr := transactionEndingFunc(ctx)

	// check for errors during the rollback round-trip
	if runFuncErr != nil {
		if t.currentTransactionState() != transactionEndedServer {
			common.Odl.Fine("An error occurred during transaction commit, changing transaction state back to previous state", "transaction state", previousState)
			t.transactionState(previousState)
		}
		return t.underlyingConnection().shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, runFuncErr, "Rollback"))
	}

	return nil
}
