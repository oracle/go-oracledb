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
	"strconv"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestDriver_RefCursorOut verifies that datatype.Rows exposes a REF CURSOR OUT
// bind as standard database/sql rows.
func TestDriver_RefCursorOut(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var raw datatype.Rows
	_, err = conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 42 AS n, 'cursor' AS label FROM dual;
END;`, sql.Out{Dest: &raw})
	if err != nil {
		t.Fatalf("open REF CURSOR: %v", err)
	}
	rows, err := raw.GetRows(ctx, conn)
	if err != nil {
		t.Fatalf("get REF CURSOR rows: %v", err)
	}
	if rows == nil {
		t.Fatal("REF CURSOR returned nil rows")
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("fetch REF CURSOR row: %v", rows.Err())
	}
	var number int64
	var label string
	if err = rows.Scan(&number, &label); err != nil {
		t.Fatalf("scan REF CURSOR row: %v", err)
	}
	if number != 42 || label != "cursor" {
		t.Fatalf("REF CURSOR row = (%d, %q), want (42, cursor)", number, label)
	}
	if rows.Next() {
		t.Fatal("REF CURSOR returned an unexpected second row")
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("REF CURSOR rows: %v", err)
	}
}

// TestDriver_RefCursorOutAsSQLRows verifies that datatype.Rows fetches a REF
// CURSOR through the supplied context and exposes it as standard *sql.Rows.
func TestDriver_RefCursorOutAsSQLRows(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	stmt, err := conn.PrepareContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 42 AS n, 'cursor' AS label FROM dual;
END;`)
	if err != nil {
		t.Fatalf("prepare REF CURSOR statement: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	var raw datatype.Rows
	if _, err = stmt.ExecContext(ctx, sql.Out{Dest: &raw}); err != nil {
		t.Fatalf("open REF CURSOR: %v", err)
	}

	rows, err := raw.GetRows(ctx, conn)
	if err != nil {
		t.Fatalf("wrap REF CURSOR rows: %v", err)
	}
	if rows == nil {
		t.Fatal("REF CURSOR returned nil rows")
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("fetch REF CURSOR row: %v", rows.Err())
	}
	var number int64
	var label string
	if err = rows.Scan(&number, &label); err != nil {
		t.Fatalf("scan REF CURSOR row: %v", err)
	}
	if number != 42 || label != "cursor" {
		t.Fatalf("REF CURSOR row = (%d, %q), want (42, cursor)", number, label)
	}
	if rows.Next() {
		t.Fatal("REF CURSOR returned an unexpected second row")
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("REF CURSOR rows: %v", err)
	}
}

// TestDriver_RefCursorMultipleOut verifies that multiple REF CURSOR OUT binds
// retain their positional order and can be consumed independently.
func TestDriver_RefCursorMultipleOut(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var first, second datatype.Rows
	_, err = conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 11 AS n FROM dual;
  OPEN :2 FOR SELECT 22 AS n FROM dual;
END;`, sql.Out{Dest: &first}, sql.Out{Dest: &second})
	if err != nil {
		t.Fatalf("open multiple REF CURSORs: %v", err)
	}
	for _, cursor := range []struct {
		name string
		raw  *datatype.Rows
		want int64
	}{{"first", &first, 11}, {"second", &second, 22}} {
		rows, err := cursor.raw.GetRows(ctx, conn)
		if err != nil {
			t.Fatalf("get %s REF CURSOR rows: %v", cursor.name, err)
		}
		if rows == nil {
			t.Fatalf("%s REF CURSOR returned nil rows", cursor.name)
		}
		if !rows.Next() {
			_ = rows.Close()
			t.Fatalf("fetch %s REF CURSOR: %v", cursor.name, rows.Err())
		}
		var got int64
		if err = rows.Scan(&got); err != nil {
			_ = rows.Close()
			t.Fatalf("scan %s REF CURSOR: %v", cursor.name, err)
		}
		if got != cursor.want {
			_ = rows.Close()
			t.Fatalf("%s REF CURSOR value = %d, want %d", cursor.name, got, cursor.want)
		}
		if rows.Next() {
			_ = rows.Close()
			t.Fatalf("%s REF CURSOR returned an unexpected second row", cursor.name)
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("%s REF CURSOR rows: %v", cursor.name, err)
		}
		if err = rows.Close(); err != nil {
			t.Fatalf("close %s REF CURSOR: %v", cursor.name, err)
		}
	}
}

// TestDriver_ImplicitResults verifies that DBMS_SQL.RETURN_RESULT is surfaced as
// standard database/sql result sets, including the prefetched first result set.
func TestDriver_ImplicitResults(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.QueryContext(context.Background(), `
DECLARE
  c1 SYS_REFCURSOR;
  c2 SYS_REFCURSOR;
BEGIN
  OPEN c1 FOR SELECT 11 AS n FROM dual;
  DBMS_SQL.RETURN_RESULT(c1);
  OPEN c2 FOR SELECT 22 AS n FROM dual;
  DBMS_SQL.RETURN_RESULT(c2);
END;`)
	if err != nil {
		t.Fatalf("query implicit results: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })

	for _, want := range []int64{11, 22} {
		if !rows.Next() {
			t.Fatalf("implicit result %d has no row: %v", want, rows.Err())
		}
		var got int64
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan implicit result %d: %v", want, err)
		}
		if got != want {
			t.Fatalf("implicit result value = %d, want %d", got, want)
		}
		if rows.Next() {
			t.Fatalf("implicit result %d returned an unexpected second row", want)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("implicit result %d iteration: %v", want, err)
		}
		if want == 11 && !rows.NextResultSet() {
			t.Fatalf("missing second implicit result: %v", rows.Err())
		}
	}
	if rows.NextResultSet() {
		t.Fatal("unexpected third implicit result")
	}
}

// TestDriver_ImplicitResultsPrefetchesAllRows verifies that TTIIMPLRES carries
// complete buffered data for two large implicit result sets. A later lazy fetch
// would not be required to consume either cursor.
func TestDriver_ImplicitResultsPrefetchesAllRows(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tableNames := []string{createObjectName("implres_a"), createObjectName("implres_b")}
	for _, tableName := range tableNames {
		if _, err := db.ExecContext(ctx, "CREATE TABLE "+tableName+" (id NUMBER PRIMARY KEY, label VARCHAR2(30))"); err != nil {
			t.Fatalf("create table %s: %v", tableName, err)
		}
		t.Cleanup(func() { _ = dropTable(ctx, db, tableName) })
		if _, err := db.ExecContext(ctx, "INSERT INTO "+tableName+" SELECT LEVEL, 'row-' || LEVEL FROM dual CONNECT BY LEVEL <= 1000"); err != nil {
			t.Fatalf("insert rows into %s: %v", tableName, err)
		}
	}

	rows, err := db.QueryContext(ctx, `
DECLARE
  c1 SYS_REFCURSOR;
  c2 SYS_REFCURSOR;
BEGIN
  OPEN c1 FOR SELECT id, label FROM `+tableNames[0]+` ORDER BY id;
  DBMS_SQL.RETURN_RESULT(c1);
  OPEN c2 FOR SELECT id, label FROM `+tableNames[1]+` ORDER BY id;
  DBMS_SQL.RETURN_RESULT(c2);
END;`)
	if err != nil {
		t.Fatalf("query implicit results: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })

	for resultSet := 0; resultSet < len(tableNames); resultSet++ {
		rowCount := 0
		for rows.Next() {
			var id int
			var label string
			if err := rows.Scan(&id, &label); err != nil {
				t.Fatalf("scan result set %d row %d: %v", resultSet+1, rowCount+1, err)
			}
			rowCount++
			wantLabel := "row-" + strconv.Itoa(rowCount)
			if id != rowCount || label != wantLabel {
				t.Fatalf("result set %d row %d = (%d, %q), want (%d, %q)", resultSet+1, rowCount, id, label, rowCount, wantLabel)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("read result set %d: %v", resultSet+1, err)
		}
		if rowCount != 1000 {
			t.Fatalf("result set %d row count = %d, want 1000", resultSet+1, rowCount)
		}
		if resultSet+1 < len(tableNames) && !rows.NextResultSet() {
			t.Fatalf("missing implicit result set %d: %v", resultSet+2, rows.Err())
		}
	}
	if rows.NextResultSet() {
		t.Fatal("unexpected third implicit result set")
	}
}

// TestDriver_RefCursorOutWithScalar verifies scalar values and REF CURSOR OUT binds
// returned together by the same PL/SQL block.
func TestDriver_RefCursorOutWithScalar(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var answer int64
	var label string
	var raw datatype.Rows
	_, err = conn.ExecContext(ctx, `
BEGIN
  :1 := 42;
  :2 := 'scalar';
  OPEN :3 FOR SELECT 7 AS n, 'cursor-row' AS label FROM dual;
END;`,
		sql.Out{Dest: &answer},
		sql.Out{Dest: &label},
		sql.Out{Dest: &raw},
	)
	if err != nil {
		t.Fatalf("execute scalar and REF CURSOR OUT binds: %v", err)
	}
	if answer != 42 {
		t.Fatalf("numeric scalar OUT value = %d, want 42", answer)
	}
	if label != "scalar" {
		t.Fatalf("scalar OUT value = %q, want scalar", label)
	}
	rows, err := raw.GetRows(ctx, conn)
	if err != nil {
		t.Fatalf("get REF CURSOR rows: %v", err)
	}
	if rows == nil {
		t.Fatal("REF CURSOR returned nil rows")
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("fetch REF CURSOR row: %v", rows.Err())
	}
	var number int64
	var cursorLabel string
	if err = rows.Scan(&number, &cursorLabel); err != nil {
		t.Fatalf("scan REF CURSOR row: %v", err)
	}
	if number != 7 || cursorLabel != "cursor-row" {
		t.Fatalf("REF CURSOR row = (%d, %q), want (7, cursor-row)", number, cursorLabel)
	}
	if rows.Next() {
		t.Fatal("REF CURSOR returned an unexpected second row")
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("REF CURSOR rows: %v", err)
	}
}
