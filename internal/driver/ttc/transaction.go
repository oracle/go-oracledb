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

// transaction implements driver.Tx for one transaction registered on a
// physical Connection. Commit and Rollback serialize their entire TTC exchange
// through the connection-wide operation guard; their context is also observed
// by statement cancellation and, later, locator-backed LOB operations.
type transaction struct {
	// _underlyingConnection owns the physical session and transaction registry.
	_underlyingConnection *connection
	// _transactionContext controls statement and future LOB cancellation for the
	// lifetime of this transaction.
	_transactionContext context.Context
}

// newTransaction creates a new transaction with the given context
//
// Parameters
//   - ctx: the transaction context
func newTransaction(conn *connection, ctx context.Context) *transaction {
	return &transaction{
		_underlyingConnection: conn,
		_transactionContext:   ctx,
	}
}

// getTransactionContext returns the current transaction context. This function
// can be used by statements to register after functions on the context in case
// the context is cancelled during the execution.
func (t *transaction) getTransactionContext() context.Context {
	return t._transactionContext
}

// Commit implements driver.Tx.Commit. It completes the TTC exchange and
// unregisters the transaction only after success.
func (t *transaction) Commit() error {
	if !t._underlyingConnection.shelf.isCurrentTransaction(t) {
		return t._underlyingConnection.shelf.LocalizeError(newNotInTransactionError())
	}
	ctx := t.getTransactionContext()
	unlock, err := t._underlyingConnection.shelf.synchronizer.begin(ctx)
	if err != nil {
		return t._underlyingConnection.shelf.LocalizeError(err)
	}
	// Another terminal transaction call may have won while this call waited
	// for the physical TTC stream. Recheck ownership under exclusive stream
	// access so Commit and Rollback cannot both reach the server.
	if !t._underlyingConnection.shelf.isCurrentTransaction(t) {
		unlock()
		return t._underlyingConnection.shelf.LocalizeError(newNotInTransactionError())
	}
	common.Odl.Debug("Transaction commit")
	readFuncError := t._underlyingConnection.runFunctionWithFunHeader(ctx, commit)

	if err := t._underlyingConnection.shelf.checkCurrentState(ctx); err != nil {
		unlock()
		return err
	}

	if readFuncError != nil {
		unlock()
		return t._underlyingConnection.shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, readFuncError, "Commit"))
	}

	t._underlyingConnection.shelf.unregisterTransaction(t)
	unlock()
	return nil
}

// Rollback implements driver.Tx.Rollback. It rolls back using the driver's
// background context and unregisters the transaction only after success.
func (t *transaction) Rollback() error {
	if !t._underlyingConnection.shelf.isCurrentTransaction(t) {
		return t._underlyingConnection.shelf.LocalizeError(newNotInTransactionError())
	}
	ctx := common.BackgroundContext
	unlock, err := t._underlyingConnection.shelf.synchronizer.begin(ctx)
	if err != nil {
		return t._underlyingConnection.shelf.LocalizeError(err)
	}
	if !t._underlyingConnection.shelf.isCurrentTransaction(t) {
		unlock()
		return t._underlyingConnection.shelf.LocalizeError(newNotInTransactionError())
	}
	common.Odl.Debug("Transaction rollback")
	runFuncErr := t._underlyingConnection.runFunctionWithFunHeader(ctx, rollback)

	if err := t._underlyingConnection.shelf.checkCurrentState(ctx); err != nil {
		unlock()
		return err
	}

	if runFuncErr != nil {
		unlock()
		return t._underlyingConnection.shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, runFuncErr, "Rollback"))
	}

	t._underlyingConnection.shelf.unregisterTransaction(t)
	unlock()
	return nil
}

// newNotInTransactionError return a not in transaction error
func newNotInTransactionError() error {
	return common.NewOracleError(oracleErrors.NotInTransaction, nil, nil)
}
