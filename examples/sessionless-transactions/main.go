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
** one is included with the Software (each a "Larger Work" to which the Software
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

// Package main demonstrates suspending a sessionless transaction and resuming
// it on a different database connection.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		return fmt.Errorf("set ORACLE_DSN, for example: user/password@localhost:1521/freepdb1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("oracledb", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	// This setup is outside the sessionless transaction so that the table can
	// be dropped after the example finishes.
	tableName := "notes"
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+tableName+" (id NUMBER PRIMARY KEY, note VARCHAR2(100))"); err != nil {
		return fmt.Errorf("create example table: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE "+tableName+" PURGE")
	}()

	// Sessionless transaction operations use dedicated *sql.Conn values rather
	// than the database pool directly.
	conn1, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get first connection: %w", err)
	}
	defer conn1.Close()

	oracleConnection, err := oracle.NewConnectionWrapper(conn1)
	if err != nil {
		return fmt.Errorf("wrap first connection: %w", err)
	}
	tx, err := oracleConnection.BeginSessionlessTx(ctx, sql.TxOptions{Isolation: sql.LevelReadCommitted}, 300)
	if err != nil {
		return fmt.Errorf("begin sessionless transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	globalTransactionID := tx.GlobalTransactionID()
	if len(globalTransactionID) == 0 {
		return fmt.Errorf("begin sessionless transaction returned an empty global transaction ID")
	}

	if _, err := conn1.ExecContext(ctx, "INSERT INTO "+tableName+" (id, note) VALUES (1, 'inserted before suspend')"); err != nil {
		return fmt.Errorf("insert on first connection: %w", err)
	}

	var count int
	if err := conn1.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count); err != nil {
		return fmt.Errorf("count rows before suspend: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("row count before suspend = %d, want 1", count)
	}
	fmt.Println("The first connection can see its uncommitted insert before suspend")

	// Suspend detaches the transaction. The global transaction ID is retained
	// so that another connection can resume it.
	if err := tx.Suspend(); err != nil {
		return fmt.Errorf("suspend sessionless transaction: %w", err)
	}
	tx = nil

	if err := conn1.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count); err != nil {
		return fmt.Errorf("count rows after suspend: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("row count after suspend on first connection = %d, want 0", count)
	}
	fmt.Println("The suspended insert is no longer visible on the first connection")

	conn2, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get second connection: %w", err)
	}
	defer conn2.Close()

	resumeOracleConnection, err := oracle.NewConnectionWrapper(conn2)
	if err != nil {
		return fmt.Errorf("wrap second connection: %w", err)
	}
	resumedTx, err := resumeOracleConnection.ResumeSessionlessTx(ctx, globalTransactionID, 300)
	if err != nil {
		return fmt.Errorf("resume sessionless transaction: %w", err)
	}
	defer func() {
		if resumedTx != nil {
			_ = resumedTx.Rollback()
		}
	}()

	// Resume is a piggyback operation. The first database operation sends the
	// resume request together with this insert.
	if _, err := conn2.ExecContext(ctx, "INSERT INTO "+tableName+" (id, note) VALUES (2, 'inserted after resume')"); err != nil {
		return fmt.Errorf("insert on resumed connection: %w", err)
	}

	if err := conn2.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count); err != nil {
		return fmt.Errorf("count rows after resume: %w", err)
	}
	if count != 2 {
		return fmt.Errorf("row count after resume on second connection = %d, want 2", count)
	}
	fmt.Println("The second connection can see both uncommitted inserts after resume")

	if err := resumedTx.Commit(); err != nil {
		return fmt.Errorf("commit sessionless transaction: %w", err)
	}

	// Prevent the deferred cleanup from attempting to end an already completed
	// transaction.
	resumedTx = nil
	tx = nil

	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count); err != nil {
		return fmt.Errorf("count rows after commit: %w", err)
	}
	if count != 2 {
		return fmt.Errorf("row count after commit = %d, want 2", count)
	}
	fmt.Printf("Sessionless transaction committed (global transaction ID: %s)\n", globalTransactionID.String())
	return nil
}
