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
	"errors"
	"fmt"
	"testing"
	"time"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestCommit checks that changes are not available to other transactions before
// the transaction is committed and that they are available after the
// transaction is committed
func TestCommit(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	// Open a database connection
	dsn := TestingConfig.GetConnectionString()
	fmt.Printf("connecting to %q\n", dsn)
	db, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("TestDriver: failed to open connection to %q: %v\n", dsn, err)
	}
	defer db.Close()

	// Setup and clean-up
	table := createObjectName("transaction_test_commit")
	createTable(context.Background(), db, table, map[string]string{"str_value": "VARCHAR(50)"})
	defer dropTable(context.Background(), db, table)

	// Start the transaction
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: false})
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Insert data
	result, err := tx.ExecContext(context.Background(), "INSERT INTO "+table+" (str_value) values ('test')")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("Wrong number of rows affected, expected 1 but was %d", rowsAffected)
	}

	// Before the transaction is committed the line inserted above should not be seen by other connections
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if rows.Next() {
		t.Fatalf("No rows should be returned")
	}

	// But the same connection as the transaction should still see it
	rows, err = tx.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if !rows.Next() {
		t.Fatalf("Rows should be returned")
	}

	// Commit
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// After the transaction is committed the line inserted above should be seen all connections
	rows, err = db.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if !rows.Next() {
		t.Fatalf("Rows should be returned")
	}

}

// TestRollback checks that changes are not available to other transactions while
// the transaction is opened and that they are not available after the
// transaction is rolled back
func TestRollback(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	// Open a database connection
	dsn := TestingConfig.GetConnectionString()
	fmt.Printf("connecting to %q\n", dsn)
	db, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("TestDriver: failed to open connection to %q: %v\n", dsn, err)
	}
	defer db.Close()

	// Setup and clean-up
	table := createObjectName("transaction_test_rollback")
	createTable(context.Background(), db, table, map[string]string{"str_value": "VARCHAR(50)"})
	defer dropTable(context.Background(), db, table)

	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Insert data
	tx.ExecContext(context.Background(), "INSERT INTO "+table+" (str_value) values ('test')")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// Before the transaction is committed the line inserted above should not be
	// seen by other connections
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if rows.Next() {
		t.Fatalf("No rows should be returned")
	}

	// Rollback
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// After the transaction is rolled back the line inserted above should not be
	// seen by other connections
	rows, err = db.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if rows.Next() {
		t.Fatalf("Rows should be returned")
	}

}

// TestRollbackThroughContextServerSleep verifies that the transaction is rolled
// back and execution stops when the transaction context is
// cancelled before the end of the statement execution.
func TestRollbackThroughContextServerSleep(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	// Open a database connection
	dsn := TestingConfig.GetConnectionString()
	fmt.Printf("connecting to %q\n", dsn)
	db, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("TestDriver: failed to open connection to %q: %v\n", dsn, err)
	}
	defer db.Close()

	table := createObjectName("transaction_test_sleep")
	err = createTable(context.Background(), db, table, map[string]string{"str_value": "VARCHAR(50)"})
	defer dropTable(context.Background(), db, table)
	if err != nil {
		t.Fatalf("Could not create table %v", err)
	}

	// Transaction context should expire and cause the transaction to rollback
	txContext, txCancel := context.WithTimeout(context.Background(), time.Second*10)
	defer txCancel()
	tx, err := db.BeginTx(txContext, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	defer tx.Commit()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// we do not want this context to expire
	stmtCtx := context.Background()
	_, err = tx.ExecContext(stmtCtx, "BEGIN INSERT INTO "+table+" (str_value) values ('test'); DBMS_SESSION.SLEEP(300); END;")
	if errors.Is(err, stmtCtx.Err()) {
		t.Fatalf("Is context error")
	}
	if err == nil {
		t.Fatalf("Expected error ORA-01013")
	}
	sqlError, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Error should be SQLError, but was %v", err)
	}
	if sqlError.ErrorCode() != "ORA-01013" {
		t.Fatalf("Error code should be %s, but was %s, %v", "ORA-01013", sqlError.ErrorCode(), err)
	}

	// After the transaction is rolled back the line inserted above should not be
	// seen by other connections
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Fatalf("Rows should be returned")
	}

}

// TestRollbackThroughContextCancel verifies that the transaction is rolled back
// when the transaction context is cancelled.
func TestRollbackThroughContextCancel(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	// Open a database connection
	dsn := TestingConfig.GetConnectionString()
	fmt.Printf("connecting to %q\n", dsn)
	db, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("TestDriver: failed to open connection to %q: %v\n", dsn, err)
	}
	defer db.Close()

	// setup and clean-up
	table := createObjectName("transaction_test_cancel")
	err = createTable(context.Background(), db, table, map[string]string{"str_value": "VARCHAR(50)"})
	defer dropTable(context.Background(), db, table)
	if err != nil {
		t.Fatalf("Could not create table %v", err)
	}

	// begin transaction
	txContext, txCancel := context.WithCancel(context.Background())
	tx, err := db.BeginTx(txContext, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// execute query
	_, err = tx.ExecContext(context.Background(), "INSERT INTO "+table+" (str_value) values ('test')")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	// cancelling the context should trigger rollback
	txCancel()

	// check that rows were not committed
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM "+table+"")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	defer rows.Close()
	if rows.Next() {
		t.Fatal("The table shoud be empty")
	}

	err = tx.Rollback()
	if err == nil {
		t.Errorf("Transaction should be closed, expected error")
	}
	if sqlErr, ok := err.(oracleErrors.SQLError); ok {
		if sqlErr.ErrorCode() != string(oracleErrors.NotInTransaction) {
			t.Errorf("Wrong error, expected %s but was %s", oracleErrors.NotInTransaction, sqlErr.ErrorCode())
		}
	}
}

// TestReadOnlyTransaction verifies that a read-only transaction rejects writes.
func TestReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("transaction_test_read_only")
	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, map[string]string{"str_value": "VARCHAR(50)"}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer dropTable(ctx, db, table)

	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
		ReadOnly:  true,
	})
	if err != nil {
		t.Fatalf("BeginTx(ReadOnly=true): %v\n"+
			"Driver concatenated ALTER SESSION and SET TRANSACTION READ ONLY "+
			"into one invalid SQL statement.", err)
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM DUAL").Scan(&n); err != nil {
		t.Errorf("SELECT in read-only tx: %v", err)
	}

	if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" (str_value) values ('test')"); err == nil {
		t.Fatal("INSERT in read-only tx should fail")
	}
}

const transactionIsolationQuery = "SELECT BITAND(flag, POWER(2, 28)) FROM V$TRANSACTION"

// transactionIsolationFlags returns the SERIALIZABLE bit from the active
// transactions visible to the current session. A nonzero value means that the
// transaction is SERIALIZABLE.
func transactionIsolationFlags(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, transactionIsolationQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []int64
	for rows.Next() {
		var flag int64
		if err := rows.Scan(&flag); err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return flags, nil
}

func hasSerializableTransaction(flags []int64) bool {
	for _, flag := range flags {
		if flag != 0 {
			return true
		}
	}
	return false
}

// TestConnectionTransactionPoolIsolation checks whether SERIALIZABLE
// transaction state leaks across reuse of the same physical connection in a
// single-connection pool after the transaction has ended.
func TestConnectionTransactionPoolIsolation(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	table := createObjectName("transaction_test_pool_isolation")
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER"}); err != nil {
		t.Fatalf("create test table: %v", err)
	}
	defer dropTable(ctx, db, table)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get first connection from pool: %v", err)
	}

	serializableTx, err := conn.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		_ = conn.Close()
		t.Fatalf("begin SERIALIZABLE transaction: %v", err)
	}
	if _, err := serializableTx.ExecContext(ctx,
		"INSERT INTO "+table+" (id) VALUES (1)"); err != nil {
		_ = serializableTx.Rollback()
		_ = conn.Close()
		t.Fatalf("start SERIALIZABLE transaction: %v", err)
	}

	firstFlags, err := transactionIsolationFlags(ctx, serializableTx)
	if err != nil {
		_ = serializableTx.Rollback()
		_ = conn.Close()
		t.Skip("User does not have access to V$TRANSACTION")
	}
	t.Logf("SERIALIZABLE transaction flags: %v", firstFlags)
	if !hasSerializableTransaction(firstFlags) {
		_ = serializableTx.Rollback()
		_ = conn.Close()
		t.Fatalf("SERIALIZABLE transaction has no nonzero isolation flag: %v", firstFlags)
	}

	// database/sql keeps a connection held by an active *sql.Tx, so finish the
	// transaction before returning its *sql.Conn to the pool.
	if err := serializableTx.Commit(); err != nil {
		_ = conn.Close()
		t.Fatalf("rollback SERIALIZABLE transaction: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("return first connection to pool: %v", err)
	}

	conn, err = db.Conn(ctx)
	if err != nil {
		t.Fatalf("get second connection from pool: %v", err)
	}
	defer conn.Close()

	defaultTx, err := conn.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		t.Fatalf("begin default-isolation transaction after pool reuse: %v", err)
	}
	defer defaultTx.Rollback()
	if _, err := defaultTx.ExecContext(ctx,
		"INSERT INTO "+table+" (id) VALUES (2)"); err != nil {
		t.Fatalf("start default-isolation transaction: %v", err)
	}

	secondFlags, err := transactionIsolationFlags(ctx, defaultTx)
	if err != nil {
		t.Fatalf("check default-isolation transaction: %v", err)
	}
	t.Logf("default-isolation transaction flags after pool reuse: %v", secondFlags)
	if hasSerializableTransaction(secondFlags) {
		t.Errorf("default-isolation transaction retained SERIALIZABLE isolation: %v", secondFlags)
	}
}
