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

type transaction struct {
	_underlyingConnection *connection
	// the current transaction context
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

// Commit commits the transaction
func (t *transaction) Commit() error {
	common.Odl.Debug("Transaction commit")
	if !t._underlyingConnection.shelf.isInTransaction() {
		return t._underlyingConnection.shelf.LocalizeError(newNotInTransactionError())
	}

	ctx := t._underlyingConnection.shelf.getTransaction().getTransactionContext()
	readFuncError := t._underlyingConnection.runFunctionWithFunHeader(ctx, commit)

	if err := t._underlyingConnection.shelf.checkCurrentState(ctx); err != nil {
		return err
	}

	if readFuncError != nil {
		return t._underlyingConnection.shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, readFuncError, "Commit"))
	}

	t._underlyingConnection.shelf.unregisterTransaction()
	return nil
}

// Rollback rolls back the transaction
func (t *transaction) Rollback() error {
	common.Odl.Debug("Transaction rollback")
	if !t._underlyingConnection.shelf.isInTransaction() {
		return t._underlyingConnection.shelf.LocalizeError(newNotInTransactionError())
	}

	runFuncErr := t._underlyingConnection.runFunctionWithFunHeader(common.BackgroundContext, rollback)

	if err := t._underlyingConnection.shelf.checkCurrentState(common.BackgroundContext); err != nil {
		return err
	}

	if runFuncErr != nil {
		return t._underlyingConnection.shelf.LocalizeError(common.NewOracleError(oracleErrors.ErrorInTransaction, runFuncErr, "Rollback"))
	}

	t._underlyingConnection.shelf.unregisterTransaction()
	return nil
}

// newNotInTransactionError return a not in transaction error
func newNotInTransactionError() error {
	return common.NewOracleError(oracleErrors.NotInTransaction, nil, nil)
}
