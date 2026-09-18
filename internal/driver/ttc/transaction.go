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
	// transactionIdentity returns the stable identity shared by handles for the
	// same server transaction.
	transactionIdentity() *transaction
}

type transaction struct {
	_underlyingConnection *connection
	// the current transaction context
	_transactionContext context.Context
	// _transactionIdentity is shared by transaction handles that represent the
	// same server transaction after a regular transaction is promoted.
	_transactionIdentity *transaction
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

// Commit commits the transaction.
//
// Returns:
//   - error: Error if no transaction is active or the commit fails.
func (t *transaction) Commit() error {
	common.Odl.Debug("Transaction commit")

	// check that the transaction is the active transaction on the connection
	if !t.isCurrentTransaction() {
		return t.underlyingConnection().shelf.LocalizeError(newNotInTransactionError())
	}

	// get the transaction, the context and execute commit
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	ctx := currentTransaction.transactionContext()
	readFuncError := t.underlyingConnection().runOTxEn(ctx, otxenCommit, currentTransaction)

	// validate the current connection state
	if err := t.underlyingConnection().shelf.checkCurrentState(ctx); err != nil {
		t.underlyingConnection().unregisterTransactionOnError()
		return err
	}

	// check for errors during the commit round-trip
	if readFuncError != nil {
		t.underlyingConnection().unregisterTransactionOnError()
		return t.underlyingConnection().shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, readFuncError, "Commit"))
	}

	// unregister the transaction
	t.underlyingConnection().shelf.unregisterTransaction()
	return nil
}

// Rollback rolls back the transaction.
//
// Returns:
//   - error: Error if no transaction is active or the rollback fails.
func (t *transaction) Rollback() error {
	common.Odl.Debug("Transaction rollback")

	// check that the transaction is the active transaction on the connection
	if !t.isCurrentTransaction() {
		return t.underlyingConnection().shelf.LocalizeError(newNotInTransactionError())
	}

	// get the transaction and execute rollback
	currentTransaction := t.underlyingConnection().shelf.getTransaction()
	runFuncErr := t.underlyingConnection().runOTxEn(common.BackgroundContext, otxenAbort, currentTransaction)

	// validate the current connection state
	if err := t.underlyingConnection().shelf.checkCurrentState(common.BackgroundContext); err != nil {
		t.underlyingConnection().unregisterTransactionOnError()
		return err
	}

	// check for errors during the rollback round-trip
	if runFuncErr != nil {
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
