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
** one is included in the Software (each a "Larger Work" to which the Software
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
	underlyingConn *sql.Conn
	transaction    common.SessionlessTransaction

	statements []*sql.Stmt
}

// addStatement records a statement prepared through this transaction.
func (tx *sessionlessTx) addStatement(stmt *sql.Stmt) {
	tx.statements = append(tx.statements, stmt)
}

// checkTransactionEnded reports sql.ErrTxDone after the transaction has
// completed through Commit, Rollback, or Suspend.
func (tx *sessionlessTx) checkTransactionEnded() error {
	if tx.transaction.IsTransactionEnded() {
		return sql.ErrTxDone
	}
	return nil
}

// closeStatements closes every statement prepared through the transaction.
// Statement close errors are intentionally ignored, matching database/sql's
// transaction cleanup behavior. The transaction state is updated by the
// underlying transaction operation itself.
func (tx *sessionlessTx) closeStatements() {
	statements := tx.statements
	tx.statements = nil

	for _, stmt := range statements {
		_ = stmt.Close()
	}
}

// Commit commits the sessionless transaction on its underlying connection.
//
// Returns:
//   - error: Error if the transaction cannot be committed.
func (tx *sessionlessTx) Commit() error {
	if err := tx.checkTransactionEnded(); err != nil {
		return err
	}
	defer tx.closeStatements()
	return tx.underlyingConn.Raw(func(driverConn any) error {
		return tx.transaction.Commit()
	})
}

// Rollback rolls back the sessionless transaction on its underlying
// connection.
//
// Returns:
//   - error: Error if the transaction cannot be rolled back.
func (tx *sessionlessTx) Rollback() error {
	if err := tx.checkTransactionEnded(); err != nil {
		return err
	}
	defer tx.closeStatements()
	return tx.underlyingConn.Raw(func(driverConn any) error {
		return tx.transaction.Rollback()
	})
}

// Suspend detaches the transaction from its current connection.
//
// Returns:
//   - error: Error if the transaction cannot be detached.
func (tx *sessionlessTx) Suspend() error {
	if err := tx.checkTransactionEnded(); err != nil {
		return err
	}
	defer tx.closeStatements()
	return tx.underlyingConn.Raw(func(driverConn any) error {
		return tx.transaction.Suspend()
	})
}

// GlobalTransactionID returns the identifier associated with the transaction.
//
// Returns:
//   - GlobalTransactionID: Transaction identifier, or nil when unavailable.
func (tx *sessionlessTx) GlobalTransactionID() GlobalTransactionID {
	if err := tx.checkTransactionEnded(); err != nil {
		return nil
	}
	var gtrid GlobalTransactionID
	_ = tx.underlyingConn.Raw(func(driverConn any) error {
		gtrid = tx.transaction.GlobalTransactionID()
		return nil
	})
	return gtrid
}

// Exec executes a statement using a background context.
//
// Returns:
//   - sql.Result: Result returned by the database.
//   - error: Error if the statement cannot be executed.
func (tx *sessionlessTx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(context.Background(), query, args...)
}

// ExecContext executes a statement using the supplied context.
//
// Returns:
//   - sql.Result: Result returned by the database.
//   - error: Error if the statement cannot be executed.
func (tx *sessionlessTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := tx.checkTransactionEnded(); err != nil {
		return nil, err
	}
	defer tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(false)
		return nil
	})
	_ = tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(true)
		return nil
	})
	return tx.underlyingConn.ExecContext(ctx, query, args...)
}

// Prepare prepares a statement using a background context.
//
// Returns:
//   - *sql.Stmt: Prepared statement.
//   - error: Error if the statement cannot be prepared.
func (tx *sessionlessTx) Prepare(query string) (*sql.Stmt, error) {
	return tx.PrepareContext(context.Background(), query)
}

// PrepareContext prepares a statement using the supplied context. The
// returned statement is owned by the transaction and is closed automatically
// when Commit, Rollback, or Suspend is called.
//
// Returns:
//   - *sql.Stmt: Prepared statement.
//   - error: Error if the statement cannot be prepared.
func (tx *sessionlessTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if err := tx.checkTransactionEnded(); err != nil {
		return nil, err
	}
	defer tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(false)
		return nil
	})
	_ = tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(true)
		return nil
	})
	stmt, err := tx.underlyingConn.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	tx.addStatement(stmt)
	return stmt, nil
}

// Query executes a query using a background context.
//
// Returns:
//   - *sql.Rows: Rows returned by the database.
//   - error: Error if the query cannot be executed.
func (tx *sessionlessTx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.QueryContext(context.Background(), query, args...)
}

// QueryContext executes a query using the supplied context.
//
// Returns:
//   - *sql.Rows: Rows returned by the database.
//   - error: Error if the query cannot be executed.
func (tx *sessionlessTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if err := tx.checkTransactionEnded(); err != nil {
		return nil, err
	}
	defer tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(false)
		return nil
	})
	_ = tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(true)
		return nil
	})
	return tx.underlyingConn.QueryContext(ctx, query, args...)
}

// QueryRow executes a query expected to return at most one row using a
// background context.
//
// Returns:
//   - *sql.Row: Row returned by the database.
func (tx *sessionlessTx) QueryRow(query string, args ...any) *sql.Row {
	return tx.QueryRowContext(context.Background(), query, args...)
}

// QueryRowContext executes a query expected to return at most one row using
// the supplied context.
//
// Returns:
//   - *sql.Row: Row returned by the database.
func (tx *sessionlessTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if err := tx.checkTransactionEnded(); err != nil {
		return tx.underlyingConn.QueryRowContext(
			common.WithSessionlessTransactionEnded(ctx), "SELECT 1")
	}
	defer tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(false)
		return nil
	})
	_ = tx.underlyingConn.Raw(func(driverConn any) error {
		tx.transaction.SetRunningFromSessionlessTx(true)
		return nil
	})
	return tx.underlyingConn.QueryRowContext(ctx, query, args...)
}
