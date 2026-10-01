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
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func countRows(ctx context.Context, db *sql.DB, table string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
	if err != nil {
		return count, fmt.Errorf("query row count for table %q: %w", table, err)
	}
	return count, err
}

func openSessionlessTestDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	db.SetMaxOpenConns(6)
	return context.Background(), db
}

func requireSessionlessSQLError(t *testing.T, operation string, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected Oracle error %s, got nil", operation, want)
	}
	sqlError, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("%s: expected Oracle SQLError %s, got %T: %v", operation, want, err, err)
	}
	if sqlError.ErrorCode() != string(want) {
		t.Fatalf("%s: expected Oracle error %s, got %s", operation, want, sqlError.ErrorCode())
	}
}

// requireSessionlessStatementClosed verifies that err reports a closed
// statement.
func requireSessionlessStatementClosed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected closed statement error, got nil")
	}
	if !strings.Contains(err.Error(), "statement is closed") {
		t.Fatalf("expected closed statement error, got %v", err)
	}
}

// waitForSessionlessTransactionTimeout waits up to timeout while honoring
// cancellation of ctx.
func waitForSessionlessTransactionTimeout(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !time.Now().Before(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// TestSessionlessTransactionCommit verifies the public sessionless transaction
// API can start, suspend, resume on another connection, and commit while
// uncommitted changes remain isolated from other connections.
func TestSessionlessTransactionCommit(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID GlobalTransactionID
	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction on initial connection: %v", err)
	}
	globalTransactionID = tx.GlobalTransactionID()
	if globalTransactionID == nil {
		t.Fatal("begin sessionless transaction returned an empty global transaction ID")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-start')"); err != nil {
		t.Fatalf("insert initial row on initial connection into table %q: %v", table, err)
	}

	err = tx.Suspend()
	if err != nil {
		t.Fatalf("suspend sessionless transaction on initial connection: %v", err)
	}

	var count int
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
	if err != nil {
		t.Fatalf("count rows after suspend on initial connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after suspend on initial connection for table %q = %d, want 0", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows before resume/commit on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows before resume/commit on another connection for table %q = %d, want 0", table, count)
	}

	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume dedicated connection: %v", err)
	}
	defer resumeConn.Close()
	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	tx2, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("resume sessionless transaction on resume connection: %v", err)
	}
	if err := tx2.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after resume on resume connection for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows after resume on resume connection for table %q = %d, want 1", table, count)
	}
	if _, err := tx2.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-resume')"); err != nil {
		t.Fatalf("insert resumed row on resume connection into table %q: %v", table, err)
	}

	// before commit in other connection, count should still be 0
	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows before commit on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows before commit on another connection for table %q = %d, want 0", table, count)
	}

	err = tx2.Commit()
	if err != nil {
		t.Fatalf("commit resumed sessionless transaction on resume connection: %v", err)
	}

	if err := resumeConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after commit on resume connection for table %q: %v", table, err)
	}
	if count != 2 {
		t.Fatalf("count rows after commit on resume connection for table %q = %d, want 2", table, count)
	}

	// after commit, count should now be 2
	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after commit on another connection for table %q: %v", table, err)
	}
	if count != 2 {
		t.Fatalf("count rows after commit on another connection for table %q = %d, want 2", table, count)
	}
}

// TestSessionlessTransactionRollback verifies the public sessionless
// transaction API can start, suspend, resume on another connection, and roll
// back so that none of its changes become visible to other connections.
func TestSessionlessTransactionRollback(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_rollback")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID GlobalTransactionID

	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction on initial connection: %v", err)
	}

	globalTransactionID = tx.GlobalTransactionID()
	if globalTransactionID == nil {
		t.Fatal("begin sessionless transaction returned an empty global transaction ID")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-rollback')"); err != nil {
		t.Fatalf("insert initial row on initial connection into table %q: %v", table, err)
	}

	err = tx.Suspend()
	if err != nil {
		t.Fatalf("suspend sessionless transaction on initial connection: %v", err)
	}

	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after suspend on initial connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after suspend on initial connection for table %q = %d, want 0", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows before resume/rollback on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows before resume/rollback on another connection for table %q = %d, want 0", table, count)
	}

	conn2, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume dedicated connection: %v", err)
	}
	defer conn2.Close()

	resumeConnectionWrapper, err := NewConnectionWrapper(conn2)
	tx2, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("resume sessionless transaction with global transaction ID %s on resume connection: %v", globalTransactionID, err)
	}

	if err := tx2.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after resume on resume connection for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows after resume on resume connection for table %q = %d, want 1", table, count)
	}
	if _, err := tx2.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-rollback-resume')"); err != nil {
		t.Fatalf("insert resumed row on resume connection into table %q: %v", table, err)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows before rollback on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows before rollback on another connection for table %q = %d, want 0", table, count)
	}

	err = tx2.Rollback()
	if err != nil {
		t.Fatalf("rollback resumed sessionless transaction on resume connection: %v", err)
	}

	if err := conn2.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after rollback on resume connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after rollback on resume connection for table %q = %d, want 0", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after rollback on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after rollback on another connection for table %q = %d, want 0", table, count)
	}
}

// TestSessionlessTransactionContextCancellationRollsBack verifies that
// canceling the context that started a sessionless transaction rolls back its
// uncommitted changes on the server.
func TestSessionlessTransactionContextCancellationRollsBack(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_context_cancel")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for context cancellation test: %v", err)
	}
	defer conn.Close()

	txCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	connectionWrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap connection for context cancellation test: %v", err)
	}
	tx, err := connectionWrapper.BeginSessionlessTx(txCtx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction with cancellable context: %v", err)
	}

	if _, err := tx.ExecContext(txCtx,
		"INSERT INTO "+table+" (str_value) VALUES ('context-cancelled')"); err != nil {
		t.Fatalf("insert row before context cancellation into table %q: %v", table, err)
	}

	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows before context cancellation for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows before context cancellation for table %q = %d, want 1", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows from another connection before context cancellation for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows from another connection before context cancellation for table %q = %d, want 0", table, count)
	}

	cancel()
	deadline := time.Now().Add(10 * time.Second)
	for len(tx.GlobalTransactionID()) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(tx.GlobalTransactionID()) != 0 {
		t.Fatal("sessionless transaction remained registered after context cancellation")
	}

	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after context cancellation for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after context cancellation for table %q = %d, want 0", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows from another connection after context cancellation for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows from another connection after context cancellation for table %q = %d, want 0", table, count)
	}
}

// TestSessionlessTransactionOperationsAfterEnd verifies that every operation
// on a transaction handle returns sql.ErrTxDone after commit, rollback, or
// suspend.
func TestSessionlessTransactionOperationsAfterEnd(t *testing.T) {
	t.Parallel()

	endingOperations := []struct {
		name string
		end  func(*sessionlessTx) error
	}{
		// Commit ends the server transaction and the public transaction handle.
		{name: "commit", end: (*sessionlessTx).Commit},
		// Rollback ends the server transaction and the public transaction handle.
		{name: "rollback", end: (*sessionlessTx).Rollback},
		// Suspend ends this public handle while leaving the server transaction resumable.
		{name: "suspend", end: (*sessionlessTx).Suspend},
	}

	for _, endingOperation := range endingOperations {
		t.Run(endingOperation.name, func(t *testing.T) {
			ctx, db := openSessionlessTestDB(t)
			defer db.Close()

			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("acquire connection after %s: %v", endingOperation.name, err)
			}
			defer conn.Close()

			connectionWrapper, err := NewConnectionWrapper(conn)
			if err != nil {
				t.Fatalf("wrap connection after %s: %v", endingOperation.name, err)
			}
			tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("begin transaction before %s: %v", endingOperation.name, err)
			}
			if err := endingOperation.end(tx); err != nil {
				t.Fatalf("%s transaction: %v", endingOperation.name, err)
			}

			// These subtests cover every SQL operation exposed by sessionlessTx.
			tests := []struct {
				name string
				call func() error
			}{
				// Exec variants must reject writes and statements after the handle ends.
				{name: "Exec", call: func() error {
					_, err := tx.Exec("SELECT 1 FROM DUAL")
					return err
				}},
				{name: "ExecContext", call: func() error {
					_, err := tx.ExecContext(ctx, "SELECT 1 FROM DUAL")
					return err
				}},
				// Prepare variants must reject new statements after the handle ends.
				{name: "Prepare", call: func() error {
					stmt, err := tx.Prepare("SELECT 1 FROM DUAL")
					if stmt != nil {
						_ = stmt.Close()
					}
					return err
				}},
				{name: "PrepareContext", call: func() error {
					stmt, err := tx.PrepareContext(ctx, "SELECT 1 FROM DUAL")
					if stmt != nil {
						_ = stmt.Close()
					}
					return err
				}},
				// Query variants must return ErrTxDone without reaching the database.
				{name: "Query", call: func() error {
					rows, err := tx.Query("SELECT 1 FROM DUAL")
					if rows != nil {
						_ = rows.Close()
					}
					return err
				}},
				{name: "QueryContext", call: func() error {
					rows, err := tx.QueryContext(ctx, "SELECT 1 FROM DUAL")
					if rows != nil {
						_ = rows.Close()
					}
					return err
				}},
				{name: "QueryRow", call: func() error {
					return tx.QueryRow("SELECT 1 FROM DUAL").Scan(new(int))
				}},
				{name: "QueryRowContext", call: func() error {
					return tx.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(new(int))
				}},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					if err := test.call(); !errors.Is(err, sql.ErrTxDone) {
						t.Fatalf("%s after %s error = %v, want %v", test.name, endingOperation.name, err, sql.ErrTxDone)
					}
				})
			}

			if tx.GlobalTransactionID() != nil {
				t.Fatalf("GlobalTransactionID after %s returned a value", endingOperation.name)
			}
			if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("second Commit after %s error = %v, want %v", endingOperation.name, err, sql.ErrTxDone)
			}
			if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("Rollback after %s error = %v, want %v", endingOperation.name, err, sql.ErrTxDone)
			}
			if err := tx.Suspend(); !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("Suspend after %s error = %v, want %v", endingOperation.name, err, sql.ErrTxDone)
			}
		})
	}
}

// TestSessionlessTransactionDirectConnectionOperationsRejected verifies that
// a client-started sessionless transaction blocks direct connection SQL. The
// subtests cover ExecContext, PrepareContext, QueryContext, and
// QueryRowContext so both statement and row-query entry points are checked.
func TestSessionlessTransactionDirectConnectionOperationsRejected(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap connection: %v", err)
	}
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Each subtest uses the connection directly, so every SQL entry point must
	// reject access while the client-owned transaction is active.
	tests := []struct {
		name string
		call func() error
	}{
		{name: "ExecContext", call: func() error {
			_, err := conn.ExecContext(ctx, "SELECT 1 FROM DUAL")
			return err
		}},
		{name: "PrepareContext", call: func() error {
			stmt, err := conn.PrepareContext(ctx, "SELECT 1 FROM DUAL")
			if stmt != nil {
				_ = stmt.Close()
			}
			return err
		}},
		{name: "QueryContext", call: func() error {
			rows, err := conn.QueryContext(ctx, "SELECT 1 FROM DUAL")
			if rows != nil {
				_ = rows.Close()
			}
			return err
		}},
		{name: "QueryRowContext", call: func() error {
			return conn.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(new(int))
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSessionlessSQLError(t, test.name, test.call(), oracleErrors.AlreadyInTransaction)
		})
	}
}

// TestSessionlessTransactionOperationContextCancellation verifies that
// canceled contexts are honored while the transaction remains active. The
// subtests cover ExecContext, QueryContext, and PrepareContext independently.
func TestSessionlessTransactionOperationContextCancellation(t *testing.T) {
	t.Parallel()

	// Each subtest verifies that a canceled operation context is returned to
	// the caller without ending the active sessionless transaction.
	tests := []struct {
		name string
		call func(*sessionlessTx, context.Context) error
	}{
		{name: "ExecContext", call: func(tx *sessionlessTx, ctx context.Context) error {
			_, err := tx.ExecContext(ctx, "SELECT 1 FROM DUAL")
			return err
		}},
		{name: "QueryContext", call: func(tx *sessionlessTx, ctx context.Context) error {
			rows, err := tx.QueryContext(ctx, "SELECT 1 FROM DUAL")
			if rows != nil {
				_ = rows.Close()
			}
			return err
		}},
		{name: "PrepareContext", call: func(tx *sessionlessTx, ctx context.Context) error {
			stmt, err := tx.PrepareContext(ctx, "SELECT 1 FROM DUAL")
			if stmt != nil {
				_ = stmt.Close()
			}
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, db := openSessionlessTestDB(t)
			defer db.Close()

			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("acquire connection: %v", err)
			}
			defer conn.Close()

			connectionWrapper, err := NewConnectionWrapper(conn)
			if err != nil {
				t.Fatalf("wrap connection: %v", err)
			}
			tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("begin sessionless transaction: %v", err)
			}

			operationCtx, cancel := context.WithCancel(ctx)
			cancel()
			if err := test.call(tx, operationCtx); !errors.Is(err, context.Canceled) {
				t.Fatalf("%s with canceled context error = %v, want %v", test.name, err, context.Canceled)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatalf("rollback after canceled %s: %v", test.name, err)
			}
		})
	}
}

// TestSessionlessTransactionPreparedStatement verifies that a prepared
// statement created through the sessionless transaction can be executed after
// the prepare call has returned.
func TestSessionlessTransactionPreparedStatement(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_prepared")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for prepared statement test: %v", err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap connection for prepared statement test: %v", err)
	}
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction for prepared statement test: %v", err)
	}

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO "+table+" (str_value) VALUES (:1)")
	if err != nil {
		t.Fatalf("prepare statement through sessionless transaction: %v", err)
	}
	defer stmt.Close()

	if _, err := stmt.ExecContext(ctx, "prepared-sessionless"); err != nil {
		t.Fatalf("execute prepared statement through sessionless transaction: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit sessionless transaction after prepared statement: %v", err)
	}

	count, err := countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after prepared statement commit for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows after prepared statement commit for table %q = %d, want 1", table, count)
	}
}

// TestSessionlessTransactionPreparedStatementClosedAfterEnd verifies that a
// statement prepared through a sessionless transaction is closed by commit,
// rollback, and suspend.
func TestSessionlessTransactionPreparedStatementClosedAfterEnd(t *testing.T) {
	t.Parallel()

	endingOperations := []struct {
		name string
		end  func(*sessionlessTx) error
	}{
		// Commit must close statements even when the statement was never executed.
		{name: "commit", end: (*sessionlessTx).Commit},
		// Rollback must close statements even though no changes are committed.
		{name: "rollback", end: (*sessionlessTx).Rollback},
		// Suspend must close statements because the public handle cannot be reused.
		{name: "suspend", end: (*sessionlessTx).Suspend},
	}

	for _, endingOperation := range endingOperations {
		t.Run(endingOperation.name, func(t *testing.T) {
			ctx, db := openSessionlessTestDB(t)
			defer db.Close()

			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("acquire connection: %v", err)
			}
			defer conn.Close()

			connectionWrapper, err := NewConnectionWrapper(conn)
			if err != nil {
				t.Fatalf("wrap connection: %v", err)
			}
			tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("begin sessionless transaction: %v", err)
			}

			stmt, err := tx.PrepareContext(ctx, "SELECT 1 FROM DUAL")
			if err != nil {
				t.Fatalf("prepare statement: %v", err)
			}
			defer stmt.Close()

			if err := endingOperation.end(tx); err != nil {
				t.Fatalf("%s transaction: %v", endingOperation.name, err)
			}

			_, err = stmt.ExecContext(ctx)
			requireSessionlessStatementClosed(t, err)
		})
	}
}

// TestSessionlessTransactionPreparedStatementBeforeStart verifies that a
// statement prepared before a client-started sessionless transaction cannot be
// executed after that transaction becomes active on the connection.
func TestSessionlessTransactionPreparedStatementBeforeStart(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_prepared_before")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for pre-sessionless prepared statement test: %v", err)
	}
	defer conn.Close()

	stmt, err := conn.PrepareContext(ctx, "INSERT INTO "+table+" (str_value) VALUES (:1)")
	if err != nil {
		t.Fatalf("prepare statement before starting sessionless transaction: %v", err)
	}
	defer stmt.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap connection for pre-sessionless prepared statement test: %v", err)
	}
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction after preparing statement: %v", err)
	}

	_, err = stmt.ExecContext(ctx, "prepared-before-sessionless")
	requireSessionlessSQLError(t, "execute prepared statement after starting sessionless transaction", err, oracleErrors.AlreadyInTransaction)

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback sessionless transaction after rejected prepared statement: %v", err)
	}
}

// TestSessionlessTransactionStartTwice verifies starting a second sessionless
// transaction on a connection that already has one returns
// AlreadyInTransaction.
func TestSessionlessTransactionStartTwice(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID GlobalTransactionID
	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err != nil {
		t.Fatalf("begin first sessionless transaction: %v", err)
	}
	globalTransactionID = tx.GlobalTransactionID()
	if globalTransactionID == nil {
		t.Fatal("first sessionless transaction returned an empty global transaction ID")
	}

	_, err = connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err == nil {
		tx.Rollback()
		t.Fatal("second BeginSessionlessTx succeeded; expected AlreadyInTransaction")
	}
	if sqlError, ok := err.(oracleErrors.SQLError); ok {
		if sqlError.ErrorCode() != string(oracleErrors.AlreadyInTransaction) {
			t.Fatalf("second BeginSessionlessTx error code = %s, want %s", sqlError.ErrorCode(), oracleErrors.AlreadyInTransaction)
		}
	} else {
		t.Fatalf("second BeginSessionlessTx returned %T, want Oracle SQLError: %v", err, err)
	}
	tx.Rollback()

}

// TestSessionlessTransactionSuspendAndResume verifies that Suspend ends the
// public transaction handle while leaving the server transaction available to
// resume and end through a new handle.
func TestSessionlessTransactionSuspendAndResume(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID GlobalTransactionID
	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction on initial connection: %v", err)
	}
	globalTransactionID = tx.GlobalTransactionID()
	if globalTransactionID == nil {
		t.Fatal("begin sessionless transaction returned an empty global transaction ID")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-start')"); err != nil {
		t.Fatalf("insert initial row on initial connection into table %q: %v", table, err)
	}

	err = tx.Suspend()
	if err != nil {
		t.Fatalf("first suspend on initial connection: %v", err)
	}

	// The original handle is no longer usable after Suspend. The server-side
	// transaction remains resumable through a new connection and handle.
	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume dedicated connection: %v", err)
	}
	defer resumeConn.Close()
	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	tx2, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("resume sessionless transaction on resume connection: %v", err)
	}

	err = tx2.Rollback()
	if err != nil {
		t.Fatalf("rollback resumed sessionless transaction: %v", err)
	}
}

// TestSessionlessTransactionResumeTwice verifies resuming a sessionless
// transaction twice on the same connection returns AlreadyInTransaction.
func TestSessionlessTransactionResumeTwice(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID GlobalTransactionID
	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction on initial connection: %v", err)
	}
	globalTransactionID = tx.GlobalTransactionID()
	if globalTransactionID == nil {
		t.Fatal("begin sessionless transaction returned an empty global transaction ID")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-start')"); err != nil {
		t.Fatalf("insert initial row on initial connection into table %q: %v", table, err)
	}

	err = tx.Suspend()
	if err != nil {
		t.Fatalf("suspend sessionless transaction on initial connection: %v", err)
	}

	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume dedicated connection: %v", err)
	}
	defer resumeConn.Close()
	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	tx2, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("first resume on resume connection: %v", err)
	}

	_, err = resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err == nil {
		tx2.Rollback()
		t.Fatal("second resume on the same connection succeeded; expected AlreadyInTransaction")
	}
	if sqlError, ok := err.(oracleErrors.SQLError); ok {
		if sqlError.ErrorCode() != string(oracleErrors.AlreadyInTransaction) {
			t.Fatalf("second resume error code = %s, want %s", sqlError.ErrorCode(), oracleErrors.AlreadyInTransaction)
		}
	} else {
		t.Fatalf("second resume returned %T, want Oracle SQLError: %v", err, err)
	}
	tx2.Rollback()

}

// TestSessionlessTransactionResumeTwiceDifferentConnection verifies a
// sessionless transaction cannot be resumed concurrently on a second
// connection while it is active on the first resumed connection.
func TestSessionlessTransactionResumeTwiceDifferentConnection(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID GlobalTransactionID
	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{
		Isolation: sql.LevelReadCommitted, ReadOnly: false}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction on initial connection: %v", err)
	}
	globalTransactionID = tx.GlobalTransactionID()
	if globalTransactionID == nil {
		t.Fatal("begin sessionless transaction returned an empty global transaction ID")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-start')"); err != nil {
		t.Fatalf("insert initial row on initial connection into table %q: %v", table, err)
	}

	err = tx.Suspend()
	if err != nil {
		t.Fatalf("suspend sessionless transaction on initial connection: %v", err)
	}

	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire first resume connection: %v", err)
	}
	defer resumeConn.Close()
	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	tx2, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 5)
	if err != nil {
		t.Fatalf("resume sessionless transaction on first resume connection: %v", err)
	}
	err = resumeConn.PingContext(ctx)
	if err != nil {
		t.Fatalf("ping first resume connection after resume: %v", err)
	}

	resumeConn2, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire second resume connection: %v", err)
	}
	defer resumeConn2.Close()
	resumeConnectionWrapper2, err := NewConnectionWrapper(resumeConn2)
	_, err = resumeConnectionWrapper2.ResumeSessionlessTx(ctx, globalTransactionID, 5)
	if err != nil {
		tx2.Rollback()
		t.Fatalf("queue resume on second connection before round trip: %v", err)
	}
	err = resumeConn2.PingContext(ctx)
	requireSessionlessSQLError(t, "ping second resume connection after concurrent resume", err, "ORA-25351")
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("rollback after failed concurrent resume: %v", err)
	}

}

// TestSessionlessTransactionCommitPLSQL verifies the server-side PL/SQL
// sessionless transaction lifecycle can start and suspend through one SQL
// transaction, resume through PL/SQL on another connection, and commit.
func TestSessionlessTransactionCommitPLSQL(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_plsql_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	tx, err := conn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelDefault, ReadOnly: false})
	if err != nil {
		t.Fatalf("begin regular transaction on initial connection: %v", err)
	}
	defer tx.Rollback()

	var globalTransactionID string
	if _, err := tx.ExecContext(ctx, `
		BEGIN
			:global_transaction_id := DBMS_TRANSACTION.START_TRANSACTION(
				xid              => NULL,
				transaction_type => DBMS_TRANSACTION.TRANSACTION_TYPE_SESSIONLESS,
				timeout          => 300,
				flag             => DBMS_TRANSACTION.TRANSACTION_NEW);
		END;`, sql.Named("global_transaction_id", sql.Out{Dest: &globalTransactionID})); err != nil {
		t.Fatalf("start sessionless transaction through PL/SQL on initial connection: %v", err)
	}
	if globalTransactionID == "" {
		t.Fatal("PL/SQL sessionless start returned an empty global transaction ID")
	}

	_, err = tx.ExecContext(context.Background(), "INSERT INTO "+table+" (str_value) VALUES ('sessionless-plsql-commit')")
	if err != nil {
		t.Fatalf("insert row after PL/SQL sessionless start into table %q: %v", table, err)
	}
	_, err = tx.ExecContext(context.Background(), "BEGIN DBMS_TRANSACTION.SUSPEND_TRANSACTION; END;")
	if err != nil {
		t.Fatalf("suspend sessionless transaction through PL/SQL on initial connection: %v", err)
	}

	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after PL/SQL suspend on initial connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after PL/SQL suspend on initial connection for table %q = %d, want 0", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows before PL/SQL resume/commit on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows before PL/SQL resume/commit on another connection for table %q = %d, want 0", table, count)
	}

	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume dedicated connection: %v", err)
	}
	defer resumeConn.Close()

	resumeTx, err := resumeConn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelDefault, ReadOnly: false})
	if err != nil {
		t.Fatalf("begin regular transaction on resume connection: %v", err)
	}
	defer resumeTx.Rollback()
	var resumedGlobalTransactionID string
	if _, err := resumeTx.ExecContext(ctx, `
		BEGIN
			:resumed_global_transaction_id := DBMS_TRANSACTION.START_TRANSACTION(
				xid              => HEXTORAW(:global_transaction_id),
				transaction_type => DBMS_TRANSACTION.TRANSACTION_TYPE_SESSIONLESS,
				timeout          => 300,
				flag             => DBMS_TRANSACTION.TRANSACTION_RESUME);
		END;`,
		sql.Named("global_transaction_id", globalTransactionID),
		sql.Named("resumed_global_transaction_id", sql.Out{Dest: &resumedGlobalTransactionID}),
	); err != nil {
		t.Fatalf("resume sessionless transaction through PL/SQL on resume connection: %v", err)
	}
	if resumedGlobalTransactionID != globalTransactionID {
		t.Fatalf("PL/SQL resume returned global transaction ID %q, want %q", resumedGlobalTransactionID, globalTransactionID)
	}

	if err := resumeConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after PL/SQL resume on resume connection for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows after PL/SQL resume on resume connection for table %q = %d, want 1", table, count)
	}
	if _, err := resumeConn.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) values ('sessionless-plsql-resume')"); err != nil {
		t.Fatalf("insert row after PL/SQL resume on resume connection into table %q: %v", table, err)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows before PL/SQL transaction commit on another connection for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows before PL/SQL transaction commit on another connection for table %q = %d, want 0", table, count)
	}

	if err := resumeTx.Commit(); err != nil {
		t.Fatalf("commit PL/SQL-resumed transaction through resume transaction handle: %v", err)
	}

	if err := resumeConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after PL/SQL transaction commit on resume connection for table %q: %v", table, err)
	}
	if count != 2 {
		t.Fatalf("count rows after PL/SQL transaction commit on resume connection for table %q = %d, want 2", table, count)
	}

	count, err = countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after PL/SQL transaction commit on another connection for table %q: %v", table, err)
	}
	if count != 2 {
		t.Fatalf("count rows after PL/SQL transaction commit on another connection for table %q = %d, want 2", table, count)
	}
}

// TestSessionlessTransactionAPIBeginPLSQLSuspendThenAPISuspend verifies that a
// sessionless transaction started through the API rejects a PL/SQL suspend
// attempt, and that the transaction can then be suspended through the API.
func TestSessionlessTransactionAPIBeginPLSQLSuspendThenAPISuspend(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_api_plsql_suspend")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial dedicated connection: %v", err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction through API: %v", err)
	}
	globalTransactionID := tx.GlobalTransactionID()
	if len(globalTransactionID) == 0 {
		t.Fatal("API sessionless start returned an empty global transaction ID")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('api-plsql-suspend')"); err != nil {
		t.Fatalf("insert row after API sessionless start into table %q: %v", table, err)
	}
	_, err = tx.ExecContext(ctx, "BEGIN DBMS_TRANSACTION.SUSPEND_TRANSACTION; END;")
	requireSessionlessSQLError(t, "suspend API sessionless transaction through PL/SQL", err, "ORA-26211")

	if err := tx.Suspend(); err != nil {
		t.Fatalf("suspend sessionless transaction through API after PL/SQL suspend rejection: %v", err)
	}

	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after API suspend for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after API suspend for table %q = %d, want 0", table, count)
	}

	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume dedicated connection: %v", err)
	}
	defer resumeConn.Close()

	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	resumedTx, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("resume sessionless transaction after repeated suspend: %v", err)
	}
	if err := resumedTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after API resume for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows after API resume for table %q = %d, want 1", table, count)
	}

	if err := resumedTx.Rollback(); err != nil {
		t.Fatalf("rollback resumed sessionless transaction after repeated suspend: %v", err)
	}
}

// TestSessionlessTransactionCommitPLSQLRunQueryBeforeSessionless verifies
// starting a sessionless transaction through PL/SQL fails with ORA-24776 when
// the current SQL transaction has already executed a query or DML statement.
func TestSessionlessTransactionCommitPLSQLRunQueryBeforeSessionless(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_plsql_run_query_commit")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	defer conn.Close()

	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (str_value) VALUES ('sessionless-plsql-query')"); err != nil {
		t.Fatalf("insert row for prior query scenario into table %q: %v", table, err)
	}

	for _, test := range []struct {
		name  string
		query bool
	}{
		{name: "DML"},
		{name: "query", query: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := conn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelDefault, ReadOnly: false})
			if err != nil {
				t.Fatalf("begin regular transaction: %v", err)
			}
			defer tx.Rollback()

			if test.query {
				var value string
				if err := tx.QueryRowContext(ctx, "SELECT str_value FROM "+table+" FOR UPDATE").Scan(&value); err != nil {
					t.Fatalf("query before PL/SQL sessionless start: %v", err)
				}
			} else if _, err := tx.ExecContext(context.Background(), "INSERT INTO "+table+" (str_value) VALUES ('sessionless-plsql-commit')"); err != nil {
				t.Fatalf("insert before PL/SQL sessionless start into table %q: %v", table, err)
			}

			var globalTransactionID string
			_, err = tx.ExecContext(ctx, `
				BEGIN
					:global_transaction_id := DBMS_TRANSACTION.START_TRANSACTION(
						xid              => NULL,
						transaction_type => DBMS_TRANSACTION.TRANSACTION_TYPE_SESSIONLESS,
						timeout          => 300,
						flag             => DBMS_TRANSACTION.TRANSACTION_NEW);
				END;`, sql.Named("global_transaction_id", sql.Out{Dest: &globalTransactionID}))
			if err == nil {
				t.Fatalf("PL/SQL sessionless start succeeded after prior %s; expected ORA-24776", test.name)
			}

			if sqlError, ok := err.(oracleErrors.SQLError); ok {
				if sqlError.ErrorCode() != "ORA-24776" {
					t.Fatalf("PL/SQL sessionless start after prior %s error code = %s, want ORA-24776", test.name, sqlError.ErrorCode())
				}
				t.Logf("Got expected sqlError %v", err)
			} else {
				t.Fatalf("PL/SQL sessionless start after prior %s returned %T, want Oracle SQLError: %v", test.name, err, err)
			}
		})
	}
}

// TestSessionlessTransactionCommitPLSQLConn verifies the server-side PL/SQL
// sessionless transaction calls work directly on a dedicated auto-commit
// connection and that suspend does not hide the already committed change.
func TestSessionlessTransactionCommitPLSQLConn(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)

	ctx := context.Background()
	table := createObjectName("sessionless_tx_plsql_conn")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	defer conn.Close()

	var globalTransactionID string
	if _, err := conn.ExecContext(ctx, `
		BEGIN
			:global_transaction_id := DBMS_TRANSACTION.START_TRANSACTION(
				xid              => NULL,
				transaction_type => DBMS_TRANSACTION.TRANSACTION_TYPE_SESSIONLESS,
				timeout          => 300,
				flag             => DBMS_TRANSACTION.TRANSACTION_NEW);
		END;`, sql.Named("global_transaction_id", sql.Out{Dest: &globalTransactionID})); err != nil {
		t.Fatalf("start sessionless transaction through PL/SQL on dedicated connection: %v", err)
	}
	if globalTransactionID == "" {
		t.Fatal("PL/SQL sessionless start returned an empty global transaction ID")
	}

	if _, err := conn.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('sessionless-plsql-conn')"); err != nil {
		t.Fatalf("insert row after PL/SQL sessionless start into table %q: %v", table, err)
	}
	if _, err := conn.ExecContext(ctx, "BEGIN DBMS_TRANSACTION.SUSPEND_TRANSACTION; END;"); err != nil {
		t.Fatalf("suspend PL/SQL sessionless transaction on dedicated connection: %v", err)
	}

	// since a connection is always on auto-commit mode, the count should be 1, the
	// sessionless transaction was created and committed in the same call and the
	// suspend was a noop
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after PL/SQL suspend on dedicated connection for table %q: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("count rows after PL/SQL suspend on dedicated connection for table %q = %d, want 1", table, count)
	}
}

// TestSessionlessTransactionBeginOptions verifies that the public API accepts
// every supported isolation/read-only combination and that each transaction
// can be ended cleanly.
func TestSessionlessTransactionBeginOptions(t *testing.T) {
	// this test should not be run in parallel, v$transaction will contain all transactions for the
	// current user
	tests := []struct {
		name string
		opts sql.TxOptions
	}{
		// Default isolation, read-write transaction.
		{name: "default read-write", opts: sql.TxOptions{}},
		// Explicit read-committed isolation, read-write transaction.
		{name: "read-committed read-write", opts: sql.TxOptions{Isolation: sql.LevelReadCommitted}},
		// Serializable isolation, read-write transaction.
		{name: "serializable read-write", opts: sql.TxOptions{Isolation: sql.LevelSerializable}},
		// Default isolation, read-only transaction.
		{name: "default read-only", opts: sql.TxOptions{ReadOnly: true}},
		// Explicit read-committed isolation, read-only transaction.
		{name: "read-committed read-only", opts: sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true}},
		// Serializable isolation, read-only transaction.
		{name: "serializable read-only", opts: sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, db := openSessionlessTestDB(t)
			defer db.Close()

			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("acquire connection for options %q: %v", test.name, err)
			}
			defer conn.Close()

			table := createObjectName("sessionless_tx_commit")
			if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
				t.Fatalf("create table %q for options %q: %v", table, test.name, err)
			}
			defer dropTable(ctx, db, table)

			connectionWrapper, err := NewConnectionWrapper(conn)
			tx, err := connectionWrapper.BeginSessionlessTx(ctx, test.opts, 300)
			if err != nil {
				t.Fatalf("begin sessionless transaction with options %q: %v", test.name, err)
			}
			if tx.GlobalTransactionID() == nil {
				t.Fatalf("begin sessionless transaction with options %q returned an empty global transaction ID", test.name)
			}

			_, err = tx.ExecContext(ctx, "INSERT INTO "+table+" (str_value) values ('sessionless-start')")
			if test.opts.ReadOnly && err == nil {
				t.Fatalf("insert into table %q succeeded for read-only options %q; expected an error", table, test.name)
			}
			if !test.opts.ReadOnly && err != nil {
				t.Fatalf("insert into table %q with options %q: %v", table, test.name, err)
			}

			var flag int
			if rows, err := tx.QueryContext(ctx, "select bitand(flag, power(2, 28)) from v$transaction"); err != nil {
				if sqlError, ok := err.(oracleErrors.SQLError); ok && sqlError.ErrorCode() == "ORA-00942" {
					t.Skip("User does not have privileges to read V$TRANSACTION")
				}
				t.Fatalf("query serializable flag with options %q: %v", test.name, err)
			} else {
				defer rows.Close()
				i := 0
				for rows.Next() {
					if rows.Err() != nil {
						t.Fatalf("Unexpected error on rows.Next %v", rows.Err())
					}
					err := rows.Scan(&flag)
					t.Logf("row %d, flag: %d", i, flag)
					if err != nil {
						t.Fatalf("Unexpected error getting flags %v", err)
					}
					if test.opts.Isolation == sql.LevelSerializable && !test.opts.ReadOnly {
						if flag == 0 {
							t.Errorf("serializable flag with options %q = %d, want non-zero", test.name, flag)
						}
					} else {
						if flag != 0 {
							t.Errorf("serializable flag with options %q = %d, want 0", test.name, flag)
						}
					}
				}
			}
			if err := tx.Rollback(); err != nil {
				t.Fatalf("rollback sessionless transaction with options %q: %v", test.name, err)
			}

		})
	}
}

// TestSessionlessTransactionResumeValidation verifies that invalid global transaction IDs are
// rejected locally and that a well-formed but unknown global transaction ID is rejected by the
// server.
func TestSessionlessTransactionResumeValidation(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for global transaction ID validation: %v", err)
	}
	defer conn.Close()
	connectionWrapper, err := NewConnectionWrapper(conn)

	for _, test := range []struct {
		name                string
		globalTransactionID []byte
	}{
		// An empty identifier must be rejected before any server round trip.
		{name: "empty", globalTransactionID: nil},
		// An identifier longer than the protocol limit must also be rejected locally.
		{name: "too long", globalTransactionID: slices.Repeat([]byte{70}, 65)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := connectionWrapper.ResumeSessionlessTx(ctx, test.globalTransactionID, 300)
			requireSessionlessSQLError(t, "resume with "+test.name+" global transaction ID", err, oracleErrors.InvalidGlobalTransactionIDValue)
		})
	}

	tx, err := connectionWrapper.ResumeSessionlessTx(ctx, GlobalTransactionID("sessionless-global-transaction-id-that-does-not-exist"), 300)
	if err != nil {
		t.Fatalf("queue resume with unknown global transaction ID: %v", err)
	}
	if tx == nil {
		t.Fatal("resume with unknown global transaction ID returned a nil transaction")
	}
	err = conn.PingContext(ctx)
	if err == nil {
		_ = tx.Rollback()
		t.Fatal("resume with unknown global transaction ID succeeded after ping; expected ORA-26218")
	}
	requireSessionlessSQLError(t, "ping after resuming unknown global transaction ID", err, "ORA-26218")
}

// TestSessionlessTransactionSQLCommitOrRollbackThenSuspend verifies that a
// transaction ended by executing SQL COMMIT or ROLLBACK is already inactive
// when the API Suspend method is called afterward.
func TestSessionlessTransactionSQLCommitOrRollbackThenSuspend(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		endStatement string
		wantRows     int
	}{
		// SQL COMMIT ends the transaction and leaves both inserted rows visible.
		{name: "commit", endStatement: "COMMIT", wantRows: 2},
		// SQL ROLLBACK ends the transaction and leaves no inserted rows.
		{name: "rollback", endStatement: "ROLLBACK", wantRows: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, db := openSessionlessTestDB(t)
			defer db.Close()

			table := createObjectName("sessionless_tx_sql_end")
			if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
				t.Fatalf("create table %q for SQL %s scenario: %v", table, test.endStatement, err)
			}
			defer dropTable(ctx, db, table)

			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("acquire connection for SQL %s scenario: %v", test.endStatement, err)
			}
			defer conn.Close()

			connectionWrapper, err := NewConnectionWrapper(conn)
			tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
			if err != nil {
				t.Fatalf("begin sessionless transaction before SQL %s: %v", test.endStatement, err)
			}
			for _, value := range []string{"sql-end-one", "sql-end-two"} {
				if _, err := tx.ExecContext(ctx,
					"INSERT INTO "+table+" (str_value) VALUES ('"+value+"')"); err != nil {
					t.Fatalf("insert %q into table %q before SQL %s: %v", value, table, test.endStatement, err)
				}
			}

			if _, err := tx.ExecContext(ctx, test.endStatement); err != nil {
				t.Fatalf("execute SQL %s on sessionless transaction: %v", test.endStatement, err)
			}
			if err := tx.Suspend(); err != nil {
				t.Fatalf("suspend after SQL %s on sessionless transaction: %v", test.endStatement, err)
			}

			count, err := countRows(ctx, db, table)
			if err != nil {
				t.Fatalf("count rows after SQL %s for table %q: %v", test.endStatement, table, err)
			}
			if count != test.wantRows {
				t.Fatalf("count rows after SQL %s for table %q = %d, want %d", test.endStatement, table, count, test.wantRows)
			}
		})
	}
}

// TestSessionlessTransactionPLSQLEndThenSuspend verifies that an API-started
// transaction can be ended through PL/SQL and then safely suspended through
// the API. The commit subtest expects one committed row; the rollback subtest
// expects no rows.
func TestSessionlessTransactionPLSQLEndThenSuspend(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		endStatement string
		wantRows     int
	}{
		{name: "commit", endStatement: "DBMS_TRANSACTION.COMMIT", wantRows: 1},
		{name: "rollback", endStatement: "DBMS_TRANSACTION.ROLLBACK", wantRows: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			testSessionlessTransactionPLSQLEndThenSuspend(t, test.endStatement, test.wantRows)
		})
	}
}

func testSessionlessTransactionPLSQLEndThenSuspend(t *testing.T, endStatement string, wantRows int) {
	t.Helper()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_plsql_end")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q for PL/SQL %s: %v", table, endStatement, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for PL/SQL %s: %v", endStatement, err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction before PL/SQL %s: %v", endStatement, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('plsql-end')"); err != nil {
		t.Fatalf("insert row before PL/SQL %s into table %q: %v", endStatement, table, err)
	}

	if _, err := tx.ExecContext(ctx, "BEGIN "+endStatement+"; END;"); err != nil {
		t.Fatalf("execute PL/SQL %s on sessionless transaction: %v", endStatement, err)
	}
	if err := tx.Suspend(); err != nil {
		t.Fatalf("suspend after PL/SQL %s on sessionless transaction: %v", endStatement, err)
	}

	count, err := countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after PL/SQL %s for table %q: %v", endStatement, table, err)
	}
	if count != wantRows {
		t.Fatalf("count rows after PL/SQL %s for table %q = %d, want %d", endStatement, table, count, wantRows)
	}
}

// TestSessionlessTransactionPLSQLEndThenAPIEnd verifies that an API terminal
// operation remains safe after PL/SQL has already ended the transaction. The
// commit subtest preserves the row; the rollback subtest leaves no row.
func TestSessionlessTransactionPLSQLEndThenAPIEnd(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name           string
		plsqlStatement string
		apiOperation   string
		wantRows       int
	}{
		{name: "commit", plsqlStatement: "DBMS_TRANSACTION.COMMIT", apiOperation: "commit", wantRows: 1},
		{name: "rollback", plsqlStatement: "DBMS_TRANSACTION.ROLLBACK", apiOperation: "rollback", wantRows: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			testSessionlessTransactionPLSQLEndThenAPIEnd(t, test.plsqlStatement, test.apiOperation, test.wantRows)
		})
	}
}

func testSessionlessTransactionPLSQLEndThenAPIEnd(t *testing.T, plsqlStatement, apiOperation string, wantRows int) {
	t.Helper()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_plsql_api_end")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q for PL/SQL %s and API %s: %v", table, plsqlStatement, apiOperation, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for PL/SQL %s and API %s: %v", plsqlStatement, apiOperation, err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction before PL/SQL %s: %v", plsqlStatement, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('plsql-api-end')"); err != nil {
		t.Fatalf("insert row before PL/SQL %s into table %q: %v", plsqlStatement, table, err)
	}

	if _, err := tx.ExecContext(ctx, "BEGIN "+plsqlStatement+"; END;"); err != nil {
		t.Fatalf("execute PL/SQL %s on sessionless transaction: %v", plsqlStatement, err)
	}

	if apiOperation == "commit" {
		err = tx.Commit()
	} else {
		err = tx.Rollback()
	}
	if err != nil {
		t.Fatalf("API %s after PL/SQL %s: %v", apiOperation, plsqlStatement, err)
	}

	count, err := countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after PL/SQL %s and API %s for table %q: %v", plsqlStatement, apiOperation, table, err)
	}
	if count != wantRows {
		t.Fatalf("count rows after PL/SQL %s and API %s for table %q = %d, want %d", plsqlStatement, apiOperation, table, count, wantRows)
	}
}

// TestSessionlessTransactionAPIOperationResults verifies the database effects
// of the two public terminal operations:
//   - commit makes the inserted row visible to other connections;
//   - rollback leaves no inserted row visible.
func TestSessionlessTransactionAPIOperationResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		commit   bool
		wantRows int
	}{
		// Commit must preserve the inserted row after the transaction ends.
		{name: "commit", commit: true, wantRows: 1},
		// Rollback must discard the inserted row after the transaction ends.
		{name: "rollback", commit: false, wantRows: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, db := openSessionlessTestDB(t)
			defer db.Close()

			table := createObjectName("sessionless_tx_api_end")
			if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
				t.Fatalf("create table %q for API %s scenario: %v", table, test.name, err)
			}
			defer dropTable(ctx, db, table)

			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("acquire connection for API %s scenario: %v", test.name, err)
			}
			defer conn.Close()

			connectionWrapper, err := NewConnectionWrapper(conn)
			tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
			if err != nil {
				t.Fatalf("begin sessionless transaction for API %s: %v", test.name, err)
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO "+table+" (str_value) VALUES ('api-end')"); err != nil {
				t.Fatalf("insert API %s row into table %q: %v", test.name, table, err)
			}

			if test.commit {
				err = tx.Commit()
			} else {
				err = tx.Rollback()
			}
			if err != nil {
				t.Fatalf("%s sessionless transaction: %v", test.name, err)
			}
			count, err := countRows(ctx, db, table)
			if err != nil {
				t.Fatalf("count rows after API %s for table %q: %v", test.name, table, err)
			}
			if count != test.wantRows {
				t.Fatalf("count rows after API %s for table %q = %d, want %d", test.name, table, count, test.wantRows)
			}
		})
	}
}

// TestSessionlessTransactionResumeSameConnection verifies that a suspended
// transaction can be resumed on the same *sql.Conn and retains its global
// transaction ID.
func TestSessionlessTransactionResumeSameConnection(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_same_conn")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection for same-connection resume: %v", err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		t.Fatalf("begin sessionless transaction before same-connection resume: %v", err)
	}
	globalTransactionID := tx.GlobalTransactionID()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('same-connection-before')"); err != nil {
		t.Fatalf("insert row before same-connection suspend into table %q: %v", table, err)
	}
	if err := tx.Suspend(); err != nil {
		t.Fatalf("suspend before same-connection resume: %v", err)
	}

	resumedTx, err := connectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("resume on same dedicated connection: %v", err)
	}
	if !slices.Equal(resumedTx.GlobalTransactionID(), globalTransactionID) {
		t.Fatalf("same-connection resume returned global transaction ID %q, want %q", resumedTx.GlobalTransactionID(), globalTransactionID)
	}
	if _, err := resumedTx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('same-connection-after')"); err != nil {
		t.Fatalf("insert row after same-connection resume into table %q: %v", table, err)
	}
	if err := resumedTx.Commit(); err != nil {
		t.Fatalf("commit after same-connection resume on dedicated connection: %v", err)
	}

	count, err := countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after same-connection commit for table %q: %v", table, err)
	}
	if count != 2 {
		t.Fatalf("count rows after same-connection commit for table %q = %d, want 2", table, count)
	}
}

// TestSessionlessTransactionResumeAfterConnectionClose verifies that closing
// the connection that suspended a transaction does not prevent resuming it on
// a newly acquired connection.
func TestSessionlessTransactionResumeAfterConnectionClose(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_closed_conn")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire original dedicated connection: %v", err)
	}
	connectionWrapper, err := NewConnectionWrapper(conn)
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		conn.Close()
		t.Fatalf("begin sessionless transaction before closing original connection: %v", err)
	}
	globalTransactionID := tx.GlobalTransactionID()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('closed-connection')"); err != nil {
		conn.Close()
		t.Fatalf("insert row before closing original connection into table %q: %v", table, err)
	}
	if err := tx.Suspend(); err != nil {
		conn.Close()
		t.Fatalf("suspend before closing original connection: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close original connection: %v", err)
	}

	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume connection after closing original connection: %v", err)
	}
	defer resumeConn.Close()
	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	resumedTx, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("resume after closing original connection: %v", err)
	}
	if err := resumedTx.Rollback(); err != nil {
		t.Fatalf("rollback transaction resumed after closing original connection: %v", err)
	}

	count, err := countRows(ctx, db, table)
	if err != nil {
		t.Fatalf("count rows after rollback following connection close for table %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("count rows after rollback following connection close for table %q = %d, want 0", table, count)
	}
}

// TestSessionlessTransactionRegularTransactionConflict verifies that the API
// refuses to start a sessionless transaction while a regular SQL transaction
// is active on the same connection.
func TestSessionlessTransactionRegularTransactionConflict(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for regular-transaction conflict: %v", err)
	}
	defer conn.Close()

	regularTx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin regular transaction before sessionless start: %v", err)
	}
	defer regularTx.Rollback()

	connectionWrapper, err := NewConnectionWrapper(conn)
	_, err = connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	requireSessionlessSQLError(t, "begin sessionless transaction while regular transaction is active", err, oracleErrors.AlreadyInTransaction)
}

// TestSessionlessTransactionGlobalTransactionIDUniqueness verifies that independent API
// starts receive non-empty, distinct global transaction identifiers.
func TestSessionlessTransactionGlobalTransactionIDUniqueness(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	conn1, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire first connection for global transaction ID uniqueness: %v", err)
	}
	defer conn1.Close()
	conn2, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire second connection for global transaction ID uniqueness: %v", err)
	}
	defer conn2.Close()

	connectionWrapper1, err := NewConnectionWrapper(conn1)
	tx1, err := connectionWrapper1.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("begin first sessionless transaction on connection 1: %v", err)
	}
	connectionWrapper2, err := NewConnectionWrapper(conn2)
	tx2, err := connectionWrapper2.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
	if err != nil {
		_ = tx1.Rollback()
		t.Fatalf("begin second sessionless transaction on connection 2: %v", err)
	}
	globalTransactionID1 := tx1.GlobalTransactionID()
	globalTransactionID2 := tx2.GlobalTransactionID()
	if globalTransactionID1 == nil || globalTransactionID2 == nil {
		t.Fatalf("global transaction IDs from connections 1 and 2 must be non-empty: %q, %q", globalTransactionID1, globalTransactionID2)
	}
	if slices.Equal(globalTransactionID1, globalTransactionID2) {
		t.Fatalf("independent transactions on connections 1 and 2 reused global transaction ID %q", globalTransactionID1)
	}
	if err := tx1.Rollback(); err != nil {
		t.Fatalf("rollback first sessionless transaction on connection 1: %v", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("rollback second sessionless transaction on connection 2: %v", err)
	}
}

// TestSessionlessTransactionTimeout verifies that a suspended transaction
// cannot be resumed after its server-side timeout has elapsed.
func TestSessionlessTransactionTimeout(t *testing.T) {
	t.Parallel()
	ctx, db := openSessionlessTestDB(t)
	defer db.Close()

	table := createObjectName("sessionless_tx_timeout")
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table %q: %v", table, err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire initial connection for timeout test: %v", err)
	}
	defer conn.Close()

	connectionWrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap initial connection for timeout test: %v", err)
	}
	tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 1)
	if err != nil {
		t.Fatalf("begin one-second sessionless transaction: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+table+" (str_value) VALUES ('timeout')"); err != nil {
		t.Fatalf("insert timeout-test row into table %q: %v", table, err)
	}
	globalTransactionID := tx.GlobalTransactionID()
	if err := tx.Suspend(); err != nil {
		t.Fatalf("suspend one-second sessionless transaction before timeout: %v", err)
	}

	if err := waitForSessionlessTransactionTimeout(ctx, 5*time.Second); err != nil {
		t.Fatalf("wait for one-second sessionless transaction timeout: %v", err)
	}
	resumeConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire resume connection after timeout: %v", err)
	}
	defer resumeConn.Close()
	resumeConnectionWrapper, err := NewConnectionWrapper(resumeConn)
	if err != nil {
		t.Fatalf("wrap resume connection after timeout: %v", err)
	}
	resumedTx, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		t.Fatalf("queue resume after timeout: %v", err)
	}
	err = resumeConn.PingContext(ctx)
	if err == nil {
		_ = resumedTx.Rollback()
		t.Fatal("resume after one-second timeout succeeded after ping; expected ORA-26218")
	}
	requireSessionlessSQLError(t, "ping after resuming timed-out transaction", err, "ORA-26218")
}

// TestSessionlessTransactionUnsupportedConnection verifies that the public API
// returns its documented unsupported-connection error for a non-Oracle driver.
func TestSessionlessTransactionUnsupportedConnection(t *testing.T) {
	const driverName = "oracle-sessionless-unsupported"
	sql.Register(driverName, unsupportedSessionlessDriver{})

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open unsupported driver: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for unsupported-driver test: %v", err)
	}
	defer conn.Close()

	if _, err := NewConnectionWrapper(conn); err == nil {
		t.Fatal("NewConnectionWrapper unexpectedly accepted unsupported connection")
	} else {
		sqlError, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("NewConnectionWrapper on unsupported connection returned unexpected error type %T: %v", err, err)
		}
		if sqlError.ErrorCode() != string(oracleErrors.UnsupportedFeature) {
			t.Fatalf("NewConnectionWrapper on unsupported connection returned error code %s, want %s", sqlError.ErrorCode(), oracleErrors.UnsupportedFeature)
		}
	}
}

type unsupportedSessionlessDriver struct{}

func (unsupportedSessionlessDriver) Open(string) (driver.Conn, error) {
	return unsupportedSessionlessConn{}, nil
}

type unsupportedSessionlessConn struct{}

func (unsupportedSessionlessConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("unsupported test connection")
}

func (unsupportedSessionlessConn) Close() error {
	return nil
}

func (unsupportedSessionlessConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("unsupported test connection")
}
