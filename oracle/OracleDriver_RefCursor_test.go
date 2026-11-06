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
	"time"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestDriver_RefCursorOut verifies that datatype.Cursor exposes a REF CURSOR OUT
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
	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap dedicated connection: %v", err)
	}

	var raw datatype.Cursor
	_, err = conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 42 AS n, 'cursor' AS label FROM dual;
END;`, sql.Out{Dest: &raw})
	if err != nil {
		t.Fatalf("open REF CURSOR: %v", err)
	}
	rows, err := wrapper.Fetch(ctx, &raw)
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

// TestDriver_RefCursorOutAsSQLRows verifies that datatype.Cursor fetches a REF
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
	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap dedicated connection: %v", err)
	}

	var raw datatype.Cursor
	if _, err = stmt.ExecContext(ctx, sql.Out{Dest: &raw}); err != nil {
		t.Fatalf("open REF CURSOR: %v", err)
	}

	rows, err := wrapper.Fetch(ctx, &raw)
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
	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap dedicated connection: %v", err)
	}

	var first, second datatype.Cursor
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
		raw  *datatype.Cursor
		want int64
	}{{"first", &first, 11}, {"second", &second, 22}} {
		rows, err := wrapper.Fetch(ctx, cursor.raw)
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
	var raw datatype.Cursor
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
	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap dedicated connection: %v", err)
	}
	rows, err := wrapper.Fetch(ctx, &raw)
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

// TestDriver_RefCursorConnectionOwnership verifies that a REF CURSOR can only
// be fetched through the connection that received it as an OUT bind.
func TestDriver_RefCursorConnectionOwnership(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(2)

	conn1, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire first dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn1.Close() })
	conn2, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire second dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })

	wrapper1, err := NewConnectionWrapper(conn1)
	if err != nil {
		t.Fatalf("wrap first dedicated connection: %v", err)
	}
	wrapper2, err := NewConnectionWrapper(conn2)
	if err != nil {
		t.Fatalf("wrap second dedicated connection: %v", err)
	}

	var cursor1 datatype.Cursor
	_, err = conn1.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 101 AS n, 'first' AS label FROM dual;
END;`, sql.Out{Dest: &cursor1})
	if err != nil {
		t.Fatalf("open first REF CURSOR: %v", err)
	}

	var cursor2 datatype.Cursor
	_, err = conn2.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 202 AS n, 'second' AS label FROM dual;
END;`, sql.Out{Dest: &cursor2})
	if err != nil {
		t.Fatalf("open second REF CURSOR: %v", err)
	}

	checkCrossFetch := func(name string, wrapper *connectionWrapper, cursor *datatype.Cursor, passedNumber int64, passedLabel string, connectionNumber int64, connectionLabel string) bool {
		t.Helper()
		rows, err := wrapper.Fetch(ctx, cursor)
		if err != nil {
			return false
		}
		if rows == nil {
			t.Errorf("%s accepted a cross-connection REF CURSOR and returned nil rows", name)
			return true
		}
		defer rows.Close()

		if !rows.Next() {
			t.Errorf("%s accepted a cross-connection REF CURSOR but returned no row: %v", name, rows.Err())
			return true
		}
		var number int64
		var label string
		if err = rows.Scan(&number, &label); err != nil {
			t.Errorf("%s accepted a cross-connection REF CURSOR but row scan failed: %v", name, err)
			return true
		}
		t.Errorf("%s accepted a cross-connection REF CURSOR and returned (%d, %q); passed cursor row is (%d, %q), cursor opened on the wrapper connection is (%d, %q)", name, number, label, passedNumber, passedLabel, connectionNumber, connectionLabel)
		return true
	}
	crossConnectionFetchAccepted := false
	crossConnectionFetchAccepted = checkCrossFetch("first connection with second cursor", wrapper1, &cursor2, 202, "second", 101, "first") || crossConnectionFetchAccepted
	crossConnectionFetchAccepted = checkCrossFetch("second connection with first cursor", wrapper2, &cursor1, 101, "first", 202, "second") || crossConnectionFetchAccepted
	if crossConnectionFetchAccepted {
		return
	}

	verifyFetch := func(name string, wrapper *connectionWrapper, cursor *datatype.Cursor, wantNumber int64, wantLabel string) {
		t.Helper()
		rows, err := wrapper.Fetch(ctx, cursor)
		if err != nil {
			t.Fatalf("%s fetch: %v", name, err)
		}
		if rows == nil {
			t.Fatalf("%s fetch returned nil rows", name)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatalf("%s fetch row: %v", name, rows.Err())
		}
		var number int64
		var label string
		if err = rows.Scan(&number, &label); err != nil {
			t.Fatalf("%s scan: %v", name, err)
		}
		if number != wantNumber || label != wantLabel {
			t.Fatalf("%s row = (%d, %q), want (%d, %q)", name, number, label, wantNumber, wantLabel)
		}
		if rows.Next() {
			t.Fatalf("%s returned an unexpected second row", name)
		}
		if err = rows.Err(); err != nil {
			t.Fatalf("%s rows: %v", name, err)
		}
	}
	verifyFetch("first connection", wrapper1, &cursor1, 101, "first")
	verifyFetch("second connection", wrapper2, &cursor2, 202, "second")
}

// TestDriver_RefCursorNullOutClearsDestination verifies that a NULL REF CURSOR
// OUT bind clears a destination that previously held a non-NULL cursor.
func TestDriver_RefCursorNullOutClearsDestination(t *testing.T) {
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

	var raw datatype.Cursor
	_, err = conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 42 AS n FROM dual;
END;`, sql.Out{Dest: &raw})
	if err != nil {
		t.Fatalf("open non-NULL REF CURSOR: %v", err)
	}
	if raw.DriverRows() == nil {
		t.Fatal("non-NULL REF CURSOR did not populate the destination")
	}

	_, err = conn.ExecContext(ctx, `
BEGIN
  :1 := NULL;
END;`, sql.Out{Dest: &raw})
	if err != nil {
		t.Fatalf("return NULL REF CURSOR: %v", err)
	}
	if raw.DriverRows() != nil {
		t.Fatal("NULL REF CURSOR left the previously returned cursor in the reused destination")
	}
}

func openRefCursorTestConnection(t *testing.T) (context.Context, *sql.DB, *sql.Conn, *connectionWrapper) {
	t.Helper()
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

	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("wrap dedicated connection: %v", err)
	}
	return ctx, db, conn, wrapper
}

// TestDriver_RefCursorEmptyResult verifies that an opened cursor with no rows
// is exposed as an empty sql.Rows without a fetch error.
func TestDriver_RefCursorEmptyResult(t *testing.T) {
	ctx, _, conn, wrapper := openRefCursorTestConnection(t)

	var cursor datatype.Cursor
	if _, err := conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT 42 AS n FROM dual WHERE 1 = 0;
END;`, sql.Out{Dest: &cursor}); err != nil {
		t.Fatalf("open empty REF CURSOR: %v", err)
	}

	rows, err := wrapper.Fetch(ctx, &cursor)
	if err != nil {
		t.Fatalf("fetch empty REF CURSOR: %v", err)
	}
	if rows == nil {
		t.Fatal("non-NULL empty REF CURSOR returned nil rows")
	}
	defer rows.Close()

	if rows.Next() {
		t.Fatal("empty REF CURSOR returned an unexpected row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate empty REF CURSOR: %v", err)
	}
}

// TestDriver_RefCursorInterleavedRows verifies that two REF CURSORs on one
// physical connection keep independent positions when callers alternate reads.
func TestDriver_RefCursorInterleavedRows(t *testing.T) {
	ctx, _, conn, wrapper := openRefCursorTestConnection(t)

	var first, second datatype.Cursor
	if _, err := conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT LEVEL AS n FROM dual CONNECT BY LEVEL <= 3;
  OPEN :2 FOR SELECT LEVEL + 100 AS n FROM dual CONNECT BY LEVEL <= 3;
END;`, sql.Out{Dest: &first}, sql.Out{Dest: &second}); err != nil {
		t.Fatalf("open interleaved REF CURSORs: %v", err)
	}

	rowsA, err := wrapper.Fetch(ctx, &first)
	if err != nil {
		t.Fatalf("fetch first REF CURSOR: %v", err)
	}
	if rowsA == nil {
		t.Fatal("first REF CURSOR returned nil rows")
	}
	defer rowsA.Close()

	rowsB, err := wrapper.Fetch(ctx, &second)
	if err != nil {
		t.Fatalf("fetch second REF CURSOR while first is open: %v", err)
	}
	if rowsB == nil {
		t.Fatal("second REF CURSOR returned nil rows")
	}
	defer rowsB.Close()

	for i := int64(1); i <= 3; i++ {
		if !rowsA.Next() {
			t.Fatalf("first REF CURSOR row %d: %v", i, rowsA.Err())
		}
		var gotA int64
		if err := rowsA.Scan(&gotA); err != nil {
			t.Fatalf("scan first REF CURSOR row %d: %v", i, err)
		}
		if gotA != i {
			t.Fatalf("first REF CURSOR row %d = %d, want %d", i, gotA, i)
		}

		if !rowsB.Next() {
			t.Fatalf("second REF CURSOR row %d: %v", i, rowsB.Err())
		}
		var gotB int64
		if err := rowsB.Scan(&gotB); err != nil {
			t.Fatalf("scan second REF CURSOR row %d: %v", i, err)
		}
		if gotB != i+100 {
			t.Fatalf("second REF CURSOR row %d = %d, want %d", i, gotB, i+100)
		}
	}
	if rowsA.Next() || rowsB.Next() {
		t.Fatal("a REF CURSOR returned more than three rows")
	}
	if err := rowsA.Err(); err != nil {
		t.Fatalf("first REF CURSOR iteration: %v", err)
	}
	if err := rowsB.Err(); err != nil {
		t.Fatalf("second REF CURSOR iteration: %v", err)
	}
}

// TestDriver_RefCursorRowsAcrossFetchBoundary verifies every row is returned
// from an explicit cursor whose row count exceeds common array-fetch sizes.
func TestDriver_RefCursorRowsAcrossFetchBoundary(t *testing.T) {
	ctx, _, conn, wrapper := openRefCursorTestConnection(t)

	var cursor datatype.Cursor
	if _, err := conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT LEVEL AS n FROM dual CONNECT BY LEVEL <= 1001 ORDER BY LEVEL;
END;`, sql.Out{Dest: &cursor}); err != nil {
		t.Fatalf("open 1,001-row REF CURSOR: %v", err)
	}

	rows, err := wrapper.Fetch(ctx, &cursor)
	if err != nil {
		t.Fatalf("fetch 1,001-row REF CURSOR: %v", err)
	}
	if rows == nil {
		t.Fatal("non-NULL REF CURSOR returned nil rows")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
		var got int64
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan row %d: %v", count, err)
		}
		if got != int64(count) {
			t.Fatalf("row %d = %d, want %d", count, got, count)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate 1,001-row REF CURSOR: %v", err)
	}
	if count != 1001 {
		t.Fatalf("row count = %d, want 1001", count)
	}
}

// TestDriver_ImplicitResultsUpdateColumns verifies that NextResultSet exposes
// the new column metadata and values when an implicit result changes schema.
func TestDriver_ImplicitResultsUpdateColumns(t *testing.T) {
	ctx, _, conn, _ := openRefCursorTestConnection(t)
	rows, err := conn.QueryContext(ctx, `
DECLARE
  c SYS_REFCURSOR;
BEGIN
  OPEN c FOR SELECT 7 AS id, 'first' AS label FROM dual;
  DBMS_SQL.RETURN_RESULT(c);
  OPEN c FOR
    SELECT DATE '2024-02-03' AS event_date,
           CAST(2.5 AS BINARY_DOUBLE) AS measure,
           CAST(17 AS NUMBER(9)) AS count_value,
           'second' AS label,
           CAST(NULL AS VARCHAR2(20)) AS optional_value
      FROM dual;
  DBMS_SQL.RETURN_RESULT(c);
END;`)
	if err != nil {
		t.Fatalf("query implicit result sets: %v", err)
	}
	defer rows.Close()

	firstColumns, err := rows.Columns()
	if err != nil {
		t.Fatalf("get first result columns: %v", err)
	}
	wantFirstColumns := []string{"ID", "LABEL"}
	if !equalStringSlices(firstColumns, wantFirstColumns) {
		t.Fatalf("first result columns = %v, want %v", firstColumns, wantFirstColumns)
	}
	if !rows.Next() {
		t.Fatalf("first implicit result row: %v", rows.Err())
	}
	var id int64
	var firstLabel string
	if err := rows.Scan(&id, &firstLabel); err != nil {
		t.Fatalf("scan first implicit result: %v", err)
	}
	if id != 7 || firstLabel != "first" {
		t.Fatalf("first implicit result = (%d, %q), want (7, first)", id, firstLabel)
	}
	if rows.Next() {
		t.Fatal("first implicit result returned an unexpected second row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate first implicit result: %v", err)
	}
	if !rows.NextResultSet() {
		t.Fatalf("second implicit result is missing: %v", rows.Err())
	}

	secondColumns, err := rows.Columns()
	if err != nil {
		t.Fatalf("get second result columns: %v", err)
	}
	wantSecondColumns := []string{"EVENT_DATE", "MEASURE", "COUNT_VALUE", "LABEL", "OPTIONAL_VALUE"}
	if !equalStringSlices(secondColumns, wantSecondColumns) {
		t.Fatalf("second result columns = %v, want %v", secondColumns, wantSecondColumns)
	}
	if !rows.Next() {
		t.Fatalf("second implicit result row: %v", rows.Err())
	}
	var eventDate time.Time
	var measure float64
	var countValue int64
	var secondLabel string
	var optionalValue sql.NullString
	if err := rows.Scan(&eventDate, &measure, &countValue, &secondLabel, &optionalValue); err != nil {
		t.Fatalf("scan second implicit result: %v", err)
	}
	if eventDate.Year() != 2024 || eventDate.Month() != time.February || eventDate.Day() != 3 || eventDate.Hour() != 0 || eventDate.Minute() != 0 || eventDate.Second() != 0 {
		t.Fatalf("event date = %v, want date fields 2024-02-03 00:00:00", eventDate)
	}
	if measure != 2.5 || countValue != 17 || secondLabel != "second" || optionalValue.Valid {
		t.Fatalf("second implicit result = (%v, %v, %d, %q, %+v), want (2024-02-03, 2.5, 17, second, NULL)", eventDate, measure, countValue, secondLabel, optionalValue)
	}
	if rows.Next() {
		t.Fatal("second implicit result returned an unexpected second row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate second implicit result: %v", err)
	}
	if rows.NextResultSet() {
		t.Fatal("unexpected third implicit result")
	}
}

// TestDriver_ImplicitResultsWithScalarOut verifies scalar OUT binds remain
// available when the same PL/SQL execution returns an implicit result set.
func TestDriver_ImplicitResultsWithScalarOut(t *testing.T) {
	ctx, _, conn, _ := openRefCursorTestConnection(t)
	var answer int64
	rows, err := conn.QueryContext(ctx, `
DECLARE
  c SYS_REFCURSOR;
BEGIN
  :1 := 42;
  OPEN c FOR SELECT 7 AS n FROM dual;
  DBMS_SQL.RETURN_RESULT(c);
END;`, sql.Out{Dest: &answer})
	if err != nil {
		t.Fatalf("query implicit result with scalar OUT bind: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("implicit result row: %v", rows.Err())
	}
	var got int64
	if err := rows.Scan(&got); err != nil {
		t.Fatalf("scan implicit result row: %v", err)
	}
	if got != 7 {
		t.Fatalf("implicit result value = %d, want 7", got)
	}
	if rows.Next() {
		t.Fatal("implicit result returned an unexpected second row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate implicit result: %v", err)
	}
	if answer != 42 {
		t.Fatalf("scalar OUT value = %d, want 42", answer)
	}
}

// TestDriver_ImplicitResultsCloseEarlyKeepsConnectionUsable verifies closing
// the first of five implicit result sets leaves the dedicated connection usable.
func TestDriver_ImplicitResultsCloseEarlyKeepsConnectionUsable(t *testing.T) {
	ctx, _, conn, _ := openRefCursorTestConnection(t)
	rows, err := conn.QueryContext(ctx, `
DECLARE
  c SYS_REFCURSOR;
BEGIN
  OPEN c FOR SELECT 1 AS n FROM dual; DBMS_SQL.RETURN_RESULT(c);
  OPEN c FOR SELECT 2 AS n FROM dual; DBMS_SQL.RETURN_RESULT(c);
  OPEN c FOR SELECT 3 AS n FROM dual; DBMS_SQL.RETURN_RESULT(c);
  OPEN c FOR SELECT 4 AS n FROM dual; DBMS_SQL.RETURN_RESULT(c);
  OPEN c FOR SELECT 5 AS n FROM dual; DBMS_SQL.RETURN_RESULT(c);
END;`)
	if err != nil {
		t.Fatalf("query five implicit result sets: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatalf("first implicit result row: %v", rows.Err())
	}
	var first int64
	if err := rows.Scan(&first); err != nil {
		_ = rows.Close()
		t.Fatalf("scan first implicit result: %v", err)
	}
	if first != 1 {
		_ = rows.Close()
		t.Fatalf("first implicit result = %d, want 1", first)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close before consuming remaining implicit results: %v", err)
	}

	var connectionValue int64
	if err := conn.QueryRowContext(ctx, "SELECT 99 AS n FROM dual").Scan(&connectionValue); err != nil {
		t.Fatalf("query after early close: %v", err)
	}
	if connectionValue != 99 {
		t.Fatalf("query after early close = %d, want 99", connectionValue)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
