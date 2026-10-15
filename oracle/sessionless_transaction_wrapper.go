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

package oracle

import (
	"context"
	"database/sql"
	"encoding/hex"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

// GlobalTransactionID identifies a sessionless transaction as the raw bytes
// supplied to or returned by the database.
type GlobalTransactionID []byte

// String returns the hexadecimal encoding of the global transaction identifier.
//
// Returns:
//   - string: Hexadecimal-encoded transaction identifier.
func (value GlobalTransactionID) String() string {
	return hex.EncodeToString([]byte(value))
}

// sessionlessTx adapts the driver's internal sessionless transaction to the
// public Oracle connection API.
type sessionlessTx struct {
	// transactionContext is the context used to begin or resume the transaction.
	transactionContext context.Context
	// transaction is the database/sql transaction used to execute operations.
	transaction *sql.Tx
	// underlyingTx supplies the driver-specific sessionless lifecycle operations.
	underlyingTx common.SessionlessTransaction
}

// Commit commits the sessionless transaction on its underlying connection.
//
// Returns:
//   - error: Error if the transaction cannot be committed.
func (tx *sessionlessTx) Commit() error {
	return tx.transaction.Commit()
}

// Rollback rolls back the sessionless transaction on its underlying
// connection.
//
// Returns:
//   - error: Error if the transaction cannot be rolled back.
func (tx *sessionlessTx) Rollback() error {
	return tx.transaction.Rollback()
}

// Suspend detaches the transaction from its session.
//
// Returns:
//   - error: Error if the transaction cannot be detached.
func (tx *sessionlessTx) Suspend() error {
	// run rollback on the sql.Tx object so that the sql package will know that
	// the transaction is is ended.
	defer tx.transaction.Rollback()
	return tx.underlyingTx.Suspend()
}

// Exec executes a statement using a background context.
//
// Returns:
//   - sql.Result: Result returned by the database.
//   - error: Error if the statement cannot be executed.
func (tx *sessionlessTx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(common.BackgroundContext, query, args...)
}

// ExecContext executes a statement using the supplied context.
//
// Returns:
//   - sql.Result: Result returned by the database.
//   - error: Error if the statement cannot be executed.
func (tx *sessionlessTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.transaction.ExecContext(ctx, query, args...)
}

// Prepare prepares a statement using a background context.
//
// Returns:
//   - *sql.Stmt: Prepared statement.
//   - error: Error if the statement cannot be prepared.
func (tx *sessionlessTx) Prepare(query string) (*sql.Stmt, error) {
	return tx.PrepareContext(common.BackgroundContext, query)
}

// PrepareContext prepares a statement using the supplied context. The
// returned statement is owned by the transaction and is closed automatically
// when Commit, Rollback, or Suspend is called.
//
// Returns:
//   - *sql.Stmt: Prepared statement.
//   - error: Error if the statement cannot be prepared.
func (tx *sessionlessTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return tx.transaction.PrepareContext(ctx, query)
}

// Stmt returns a transaction-scoped version of stmt using a background
// context. The returned statement is closed when the transaction ends.
//
// Returns:
//   - *sql.Stmt: Statement bound to this transaction.
func (tx *sessionlessTx) Stmt(stmt *sql.Stmt) *sql.Stmt {
	return tx.StmtContext(common.BackgroundContext, stmt)
}

// StmtContext returns a transaction-scoped version of stmt using the supplied
// context. The returned statement is closed when the transaction ends.
//
// Returns:
//   - *sql.Stmt: Statement bound to this transaction.
func (tx *sessionlessTx) StmtContext(ctx context.Context, stmt *sql.Stmt) *sql.Stmt {
	return tx.transaction.StmtContext(ctx, stmt)
}

// Query executes a query using a background context.
//
// Returns:
//   - *sql.Rows: Rows returned by the database.
//   - error: Error if the query cannot be executed.
func (tx *sessionlessTx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.QueryContext(common.BackgroundContext, query, args...)
}

// QueryContext executes a query using the supplied context.
//
// Returns:
//   - *sql.Rows: Rows returned by the database.
//   - error: Error if the query cannot be executed.
func (tx *sessionlessTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.transaction.QueryContext(ctx, query, args...)
}

// QueryRow executes a query expected to return at most one row using a
// background context.
//
// Returns:
//   - *sql.Row: Row returned by the database.
func (tx *sessionlessTx) QueryRow(query string, args ...any) *sql.Row {
	return tx.QueryRowContext(common.BackgroundContext, query, args...)
}

// QueryRowContext executes a query expected to return at most one row using
// the supplied context.
//
// Returns:
//   - *sql.Row: Row returned by the database.
func (tx *sessionlessTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.transaction.QueryRowContext(ctx, query, args...)
}

// GlobalTransactionID returns the identifier currently stored for this
// sessionless transaction. Save the identifier before Suspend, Commit, or
// Rollback because server synchronization may clear it when the transaction
// ends on the current connection.
func (tx *sessionlessTx) GlobalTransactionID() GlobalTransactionID {
	return GlobalTransactionID(tx.underlyingTx.GlobalTransactionID())
}
