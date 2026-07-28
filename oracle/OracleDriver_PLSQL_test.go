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
	"testing"
	"time"

	"github.com/oracle/go-driver/driver/common"
)

// TestDriver_PLSQL_AnonymousBlock_Sanity
// Executes a trivial anonymous PL/SQL block to validate PL/SQL execution path.
// Note: Adjust the block content once PL/SQL bind semantics are expanded.
func TestDriver_PLSQL_AnonymousBlock_Sanity(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	// Simple no-op PL/SQL block
	_, err = db.ExecContext(context.Background(), "BEGIN NULL; END;")
	if err != nil {
		t.Fatalf("PL/SQL anonymous block failed: %v", err)
	}
}

// TestDriver_PLSQL_CreateInsertSelectDrop
// Creates a table using PL/SQL, inserts a row via PL/SQL,
// selects the row using a regular SQL SELECT (not PL/SQL), and drops the table via PL/SQL.
func TestDriver_PLSQL_CreateInsertSelectDrop(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	table := createObjectName("t_plsql")

	// Create table using PL/SQL (no IF EXISTS; unique name avoids conflicts)
	_, err = db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'CREATE TABLE "+table+" (id number, name varchar2(100))'; END;")
	if err != nil {
		t.Fatalf("PL/SQL create table failed: %v", err)
	}

	// Deterministic lifecycle: unique table name, ensure cleanup even on failure
	defer func() {
		// Drop table using PL/SQL
		if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+"'; END;"); err != nil {
			t.Errorf("PL/SQL drop table failed: %v", err)
		}
	}()

	// Insert via PL/SQL without binds
	plsqlIns := "BEGIN INSERT INTO " + table + " (id, name) VALUES (555, 'plsql'); END;"
	if _, err := db.ExecContext(ctx, plsqlIns); err != nil {
		t.Fatalf("PL/SQL insert failed: %v", err)
	}

	// Select (not PL/SQL, no binds)
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM "+table+" WHERE id = 555")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if id != 555 || name != "plsql" {
			t.Fatalf("unexpected row: got (id=%d, name=%q), want (555, \"plsql\")", id, name)
		}
		found = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if !found {
		t.Fatalf("expected row not found")
	}
}

func TestDriver_PLSQL_BreakCausedByTimeout(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer conn.Close()

	common.Odl.Info("before execute", "time", time.Now())
	// PL/SQL block that sleeps for 5 mins and an context than expires in 10s
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = conn.ExecContext(ctx, "BEGIN DBMS_SESSION.SLEEP(300); END;")
	common.Odl.Info("After execute", "time", time.Now())
	sqlError, ok := err.(SQLError)
	if !ok {
		t.Fatalf("Error should be SQLError, but was %v", err)
	}
	if sqlError.ErrorCode() != "ORA-01013" {
		t.Fatalf("Error code should be %s, but was %s, %v", "ORA-01013", sqlError.ErrorCode(), err)
	}

	// Check that the connection can still be used after the cancel
	common.Odl.Info("Running query SELECT 1 FROM DUAL")
	rows, err := conn.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
	if err != nil {
		t.Fatalf("Error running query: %v", err)
	}
	defer rows.Close()

}

// TestDriver_PLSQL_Prepared_Binds
// Purpose: Validate prepared statement binds (named and ordinal) with a PL/SQL anonymous block.
// Steps:
//   - Create a test table (SQL DDL)
//   - Prepare a PL/SQL block with named binds and execute
//   - Prepare a PL/SQL block with ordinal binds and execute
//   - Verify inserted rows via regular SELECT (Query + Next + Scan)
//   - Cleanup table
func TestDriver_PLSQL_Prepared_Binds(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	table := createObjectName("t_plsql_ps")
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	})

	// Prepared PL/SQL with named binds
	namedBlock := "BEGIN INSERT INTO " + table + " (id, name) VALUES (:id, :name); END;"
	namedStmt, err := db.PrepareContext(ctx, namedBlock)
	if err != nil {
		t.Fatalf("prepare named PL/SQL failed: %v", err)
	}
	t.Cleanup(func() { _ = namedStmt.Close() })

	if _, err := namedStmt.ExecContext(ctx,
		sql.Named("id", int64(1001)),
		sql.Named("name", "ps-named"),
	); err != nil {
		t.Fatalf("exec named PL/SQL failed: %v", err)
	}

	// Prepared PL/SQL with ordinal binds
	ordinalBlock := "BEGIN INSERT INTO " + table + " (id, name) VALUES (:1, :2); END;"
	ordinalStmt, err := db.PrepareContext(ctx, ordinalBlock)
	if err != nil {
		t.Fatalf("prepare ordinal PL/SQL failed: %v", err)
	}
	t.Cleanup(func() { _ = ordinalStmt.Close() })

	if _, err := ordinalStmt.ExecContext(ctx, int64(1002), "ps-ord"); err != nil {
		t.Fatalf("exec ordinal PL/SQL failed: %v", err)
	}

	// Verify via SELECT using Query + Next + Scan
	q := "SELECT id, name FROM " + table + " WHERE id IN (1001, 1002) ORDER BY id"
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type row struct {
		id   int64
		name string
	}
	var got []row
	for rs.Next() {
		var id int64
		var name string
		if err := rs.Scan(&id, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		got = append(got, row{id, name})
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("row count mismatch: got %d want 2", len(got))
	}
	if got[0].id != 1001 || got[0].name != "ps-named" {
		t.Fatalf("first row mismatch: got (id=%d,name=%q) want (1001,\"ps-named\")", got[0].id, got[0].name)
	}
	if got[1].id != 1002 || got[1].name != "ps-ord" {
		t.Fatalf("second row mismatch: got (id=%d,name=%q) want (1002,\"ps-ord\")", got[1].id, got[1].name)
	}
}
