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
	"bytes"
	"context"
	"database/sql"
	"math"
	"testing"
	"time"
)

// TestDriver_Table_Insert
// Creates the table, inserts rows, optionally validates, and cleans up.
// Expectation: INSERT succeeds; table and data are removed.
func TestDriver_Table_Insert(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion < 23 {
		t.Skip("Inserting multiple rows with values is not supported with 19c")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	table := createObjectName("t_dml_insert")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	res, err := db.ExecContext(context.Background(),
		"INSERT INTO "+table+" (id, name) VALUES(1, 'abc'), (2, 'xyz'), (3, 'pqr')")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}

	if rowsAffected != 3 {
		// Oracle should report the number of rows inserted when available.
		t.Fatalf("unexpected rows affected: got %d, want 3", rowsAffected)
	}
}

// TestDriver_Prepared_Insert_Select
// Creates a table, uses prepared statements without binds to insert and then select a row.
// Expectation: the selected row matches the inserted values; table is cleaned up.
func TestDriver_Insert_Select(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_prepared")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Prepare and execute an insert without binds (literals)
	insertRes, err := db.ExecContext(context.Background(),
		"INSERT INTO "+table+" (id, name) VALUES(1001, 'prepared')")

	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	insertRowsAffected, err := insertRes.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}

	if insertRowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", insertRowsAffected)
	}

	// Prepare and execute a select without binds (literals)
	rows, err := db.QueryContext(context.Background(),
		"SELECT id, name FROM "+table+" WHERE id = 1001")
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		// For now, scan NUMBER as []byte and defer decoding to row reader tests.
		var idRaw []byte
		var name string
		if err := rows.Scan(&idRaw, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Minimal assertion on the string column; NUMBER decoding verified elsewhere.
		if name == "prepared" {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if !found {
		t.Fatalf("expected row not found")
	}
}

// TestDriver_Prepared_Insert_Select_Ordinal
// What it does: creates a table, uses prepared statements to insert and then select a row.
// Expectation: the selected row matches the inserted values; table is cleaned up.
func TestDriver_Prepared_Insert_Select_Ordinal(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_prepared_ord")
	dropTable(ctx, db, table)
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Prepare and execute an insert with binds (:1, :2)
	ins, err := db.PrepareContext(ctx, "INSERT INTO "+table+" (id, name) VALUES(:1, :2)")
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer ins.Close()

	result, err := ins.ExecContext(ctx, int64(1001), "prepared")
	if err != nil {
		t.Fatalf("exec insert failed: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}

	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	// Prepare and execute a select with a bind (:1)
	sel, err := db.PrepareContext(ctx, "SELECT id, name FROM "+table+" WHERE id = :1")
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer sel.Close()

	rows, err := sel.QueryContext(ctx, int64(1001))
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		if id != 1001 || name != "prepared" {
			t.Fatalf("unexpected row: got (id=%d, name=%q), want (1001, \"prepared\")", id, name)
		}
		t.Logf("ID: %d name: %s", id, name)
		found = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if !found {
		t.Fatalf("expected row not found")
	}
}

// TestDriver_Prepared_Insert_Select_Named
// What it does: creates a table, uses prepared statements to insert and then select a row.
// Expectation: the selected row matches the inserted values; table is cleaned up.
func TestDriver_Prepared_Insert_Select_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_prepared_named")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Prepare and execute an insert with named binds
	ins, err := db.PrepareContext(ctx, "INSERT INTO "+table+" (id, name) VALUES(:id, :name)")
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer ins.Close()
	result, err := ins.ExecContext(ctx, sql.Named("id", int64(1001)), sql.Named("name", "prepared"))
	if err != nil {
		t.Fatalf("exec insert failed: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}

	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	// Prepare and execute a select with a bind (:1)
	sel, err := db.PrepareContext(ctx, "SELECT id, name FROM "+table+" WHERE id = :1")
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer sel.Close()

	rows, err := sel.QueryContext(ctx, int64(1001))
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if id != 1001 || name != "prepared" {
			t.Fatalf("unexpected row: got (id=%d, name=%q), want (1001, \"prepared\")", id, name)
		}
		t.Logf("ID: %d name: %s", id, name)
		found = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if !found {
		t.Fatalf("expected row not found")
	}
}

// TestDriver_Bind_ReusedNamedParameter ensures named bind values map correctly regardless of order and reuse.
func TestDriver_Bind_ReusedNamedParameter(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_bind_reused_named")
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(50)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (1, 'alpha'), (2, 'beta')"); err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}

	updateSQL := "UPDATE " + table + " SET name = :new_name WHERE id = :shared_id"
	stmt, err := db.PrepareContext(ctx, updateSQL)
	if err != nil {
		t.Fatalf("prepare update failed: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	if _, err := stmt.ExecContext(ctx,
		sql.Named("new_name", "alpha-updated"),
		sql.Named("shared_id", int64(1)),
	); err != nil {
		t.Fatalf("exec with ordered named binds failed: %v", err)
	}

	// Re-execute with arguments swapped to ensure mapping by name rather than position.
	if _, err := stmt.ExecContext(ctx,
		sql.Named("shared_id", 2),
		sql.Named("new_name", "beta-updated"),
	); err != nil {
		t.Fatalf("exec with reordered named binds failed: %v", err)
	}

	// Mixing named and positional binds should still succeed: positional argument fills :shared_id.
	if _, err := stmt.ExecContext(ctx,
		sql.Named("new_name", "alpha-again"),
		1,
	); err != nil {
		t.Fatalf("exec mixing named and positional binds failed: %v", err)
	}

	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM "+table+" WHERE id = :1", 1).Scan(&name); err != nil {
		t.Fatalf("verify row 1 failed: %v", err)
	}
	if name != "alpha-again" {
		t.Fatalf("expected updated name for id 1, got %q", name)
	}

	if err := db.QueryRowContext(ctx, "SELECT name FROM "+table+" WHERE id = :1", 2).Scan(&name); err != nil {
		t.Fatalf("verify row 2 failed: %v", err)
	}
	if name != "beta-updated" {
		t.Fatalf("expected updated name for id 2, got %q", name)
	}
}

// TestDriver_Prepared_SelectMultipleRows_Re_exec
// What it does: creates a table with representative types, inserts multiple rows, prepares a SELECT
// with multiple named binds (IN (:a, :b, :c) ORDER BY id), and re-executes the prepared statement
// with different bind sets, validating all returned columns including TSTZ offsets.
// Expectation: each re-execution returns rows in the expected ORDER BY id with correct values and
// offsets; prepared statement reuse across executions works; Skip if BOOLEAN (23c+) is unsupported.
func TestDriver_Prepared_SelectMultipleRows_Re_exec(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("combined_types_ps_multi_select_test1")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"n_col":   "NUMBER",
		"v_col":   "VARCHAR2(100)",
		"f_col":   "BINARY_DOUBLE",
		"d_col":   "DATE",
		"ts_col":  "TIMESTAMP(6)",
		"tzt_col": "TIMESTAMP(6) WITH TIME ZONE",
		"b_col":   "NUMBER(1)",
		"r_col":   "RAW(2000)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("Skipping prepared SELECT multi-binds test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Prepared INSERT reused for multiple rows (use values different from other tests)
	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	type rowData struct {
		id  int64
		n   int64
		v   string
		f   float64
		d   time.Time
		ts  time.Time
		tzt time.Time
		b   bool
		r   []byte
		off int // expected TZ offset seconds
	}

	rowsToInsert := []rowData{
		{
			id: 10, n: 100, v: "alpha", f: 0.57721,
			d:   time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2022, time.January, 1, 8, 9, 10, 111222000, time.UTC),
			tzt: time.Date(2022, time.January, 1, 12, 39, 10, 111222000, time.FixedZone("+04:30", 4*3600+30*60)),
			b:   true,
			r:   []byte{0x00, 0x01},
			off: 4*3600 + 30*60,
		},
		{
			id: 11, n: 200, v: "beta", f: 2.5,
			d:   time.Date(2021, time.December, 31, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2021, time.December, 31, 23, 59, 59, 1000, time.UTC),
			tzt: time.Date(2021, time.December, 31, 16, 59, 59, 1000, time.FixedZone("-07:00", -7*3600)),
			b:   false,
			r:   []byte{0xAA},
			off: -7 * 3600,
		},
		{
			id: 12, n: 300, v: "gamma", f: -1.25,
			d:   time.Date(2020, time.February, 29, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2020, time.February, 29, 12, 34, 56, 654321000, time.UTC),
			tzt: time.Date(2020, time.February, 29, 18, 19, 56, 654321000, time.FixedZone("+05:45", 5*3600+45*60)),
			b:   true,
			r:   []byte{0xBE, 0xEF},
			off: 5*3600 + 45*60,
		},
		{
			id: 13, n: 400, v: "delta", f: 9.81,
			d:   time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2025, time.June, 15, 1, 2, 3, 400500000, time.UTC),
			tzt: time.Date(2025, time.June, 14, 21, 32, 3, 400500000, time.FixedZone("-03:30", -3*3600-30*60)),
			b:   false,
			r:   []byte{0xDE, 0xAD},
			off: -3*3600 - 30*60,
		},
	}

	for _, rr := range rowsToInsert {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", rr.id),
			sql.Named("n", rr.n),
			sql.Named("v", rr.v),
			sql.Named("f", rr.f),
			sql.Named("d", rr.d),
			sql.Named("ts", rr.ts),
			sql.Named("tzt", rr.tzt),
			sql.Named("b", rr.b),
			sql.Named("r", rr.r),
		); err != nil {
			t.Fatalf("exec insert failed for id=%d: %v", rr.id, err)
		}
	}

	// Prepare SELECT with multiple bind placeholders (named) and reuse it with different bind sets.
	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col FROM " + table + " WHERE id IN (:a, :b, :c) ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	type bindSet struct {
		a, b, c int64
		expIDs  []int64 // expected ids in ORDER BY id order
	}
	tests := []bindSet{
		{a: 10, b: 12, c: 13, expIDs: []int64{10, 12, 13}},
		{a: 11, b: 10, c: 12, expIDs: []int64{10, 11, 12}},
		{a: 13, b: 11, c: 10, expIDs: []int64{10, 11, 13}},
	}

	// Helper to fetch expected row by id
	getExp := func(id int64) rowData {
		for _, r := range rowsToInsert {
			if r.id == id {
				return r
			}
		}
		t.Fatalf("internal error: expected id %d not found", id)
		return rowData{}
	}

	for _, tc := range tests {
		rows, err := selStmt.QueryContext(ctx,
			sql.Named("a", tc.a),
			sql.Named("b", tc.b),
			sql.Named("c", tc.c),
		)
		if err != nil {
			t.Fatalf("query failed for binds (%d,%d,%d): %v", tc.a, tc.b, tc.c, err)
		}

		i := 0
		for rows.Next() {
			var (
				idOut  int64
				nOut   int64
				vOut   string
				fOut   float64
				dOut   time.Time
				tsOut  time.Time
				tztOut time.Time
				bOut   bool
				rOut   []byte
			)
			if err := rows.Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut); err != nil {
				_ = rows.Close()
				t.Fatalf("scan failed for binds (%d,%d,%d): %v", tc.a, tc.b, tc.c, err)
			}
			if i >= len(tc.expIDs) {
				_ = rows.Close()
				t.Fatalf("received more rows than expected")
			}
			exp := getExp(tc.expIDs[i])

			if idOut != exp.id {
				_ = rows.Close()
				t.Fatalf("id mismatch: got %d want %d", idOut, exp.id)
			}
			if nOut != exp.n {
				_ = rows.Close()
				t.Fatalf("n_col mismatch for id=%d: got %d want %d", exp.id, nOut, exp.n)
			}
			if vOut != exp.v {
				_ = rows.Close()
				t.Fatalf("v_col mismatch for id=%d: got %q want %q", exp.id, vOut, exp.v)
			}
			if diff := math.Abs(fOut - exp.f); diff > 1e-12 {
				_ = rows.Close()
				t.Fatalf("f_col mismatch for id=%d: got %.15f want %.15f (diff=%.3g)", exp.id, fOut, exp.f, diff)
			}
			assertSameYMDHMSNanos(t, 0, "DATE", dOut, exp.d)
			assertSameYMDHMSNanos(t, 0, "TIMESTAMP", tsOut, exp.ts)
			assertSameYMDHMSNanos(t, 0, "TSTZ", tztOut, exp.tzt)
			if _, off := tztOut.Zone(); off != exp.off {
				_ = rows.Close()
				t.Fatalf("TSTZ offset mismatch for id=%d: got %d want %d", exp.id, off, exp.off)
			}
			if bOut != exp.b {
				_ = rows.Close()
				t.Fatalf("b_col mismatch for id=%d: got %v want %v", exp.id, bOut, exp.b)
			}
			if !bytes.Equal(rOut, exp.r) {
				_ = rows.Close()
				t.Fatalf("r_col mismatch for id=%d: got % X want % X", exp.id, rOut, exp.r)
			}
			i++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("rows err for binds (%d,%d,%d): %v", tc.a, tc.b, tc.c, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("rows close err: %v", err)
		}
		if i != len(tc.expIDs) {
			t.Fatalf("row count mismatch: got %d want %d", i, len(tc.expIDs))
		}
	}
}

// TestDriver_Prepared_SelectMultipleRows_Re_exec_BindTypeChange
// What it does: creates a table with representative types, inserts multiple rows, prepares a SELECT
// with multiple named binds (IN (:a, :b, :c) ORDER BY id), and re-executes the prepared statement
// but changes the bind value types between executions (int64, int, string). Verifies that the same
// prepared statement can be reused across executions even when bind types change, and that all
// returned columns (including TSTZ offsets) are correct.
// Expectation: each re-execution returns rows in the expected ORDER BY id with correct values and
// offsets; prepared statement reuse across executions works; Skip if BOOLEAN (23c+) is unsupported.
func TestDriver_Prepared_SelectMultipleRows_Re_exec_BindTypeChange(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("combined_types_ps_multi_select_bind_change1")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"n_col":   "NUMBER",
		"v_col":   "VARCHAR2(100)",
		"f_col":   "BINARY_DOUBLE",
		"d_col":   "DATE",
		"ts_col":  "TIMESTAMP(6)",
		"tzt_col": "TIMESTAMP(6) WITH TIME ZONE",
		"b_col":   "NUMBER(1)",
		"r_col":   "RAW(2000)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("Skipping bind type change test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Prepared INSERT reused for multiple rows
	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	type rowData struct {
		id  int64
		n   int64
		v   string
		f   float64
		d   time.Time
		ts  time.Time
		tzt time.Time
		b   bool
		r   []byte
		off int // expected TZ offset seconds
	}

	rowsToInsert := []rowData{
		{
			id: 10, n: 100, v: "alpha", f: 0.57721,
			d:   time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2022, time.January, 1, 8, 9, 10, 111222000, time.UTC),
			tzt: time.Date(2022, time.January, 1, 12, 39, 10, 111222000, time.FixedZone("+04:30", 4*3600+30*60)),
			b:   true,
			r:   []byte{0x00, 0x01},
			off: 4*3600 + 30*60,
		},
		{
			id: 11, n: 200, v: "beta", f: 2.5,
			d:   time.Date(2021, time.December, 31, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2021, time.December, 31, 23, 59, 59, 1000, time.UTC),
			tzt: time.Date(2021, time.December, 31, 16, 59, 59, 1000, time.FixedZone("-07:00", -7*3600)),
			b:   false,
			r:   []byte{0xAA},
			off: -7 * 3600,
		},
		{
			id: 12, n: 300, v: "gamma", f: -1.25,
			d:   time.Date(2020, time.February, 29, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2020, time.February, 29, 12, 34, 56, 654321000, time.UTC),
			tzt: time.Date(2020, time.February, 29, 18, 19, 56, 654321000, time.FixedZone("+05:45", 5*3600+45*60)),
			b:   true,
			r:   []byte{0xBE, 0xEF},
			off: 5*3600 + 45*60,
		},
		{
			id: 13, n: 400, v: "delta", f: 9.81,
			d:   time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			ts:  time.Date(2025, time.June, 15, 1, 2, 3, 400500000, time.UTC),
			tzt: time.Date(2025, time.June, 14, 21, 32, 3, 400500000, time.FixedZone("-03:30", -3*3600-30*60)),
			b:   false,
			r:   []byte{0xDE, 0xAD},
			off: -3*3600 - 30*60,
		},
	}

	for _, rr := range rowsToInsert {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", rr.id),
			sql.Named("n", rr.n),
			sql.Named("v", rr.v),
			sql.Named("f", rr.f),
			sql.Named("d", rr.d),
			sql.Named("ts", rr.ts),
			sql.Named("tzt", rr.tzt),
			sql.Named("b", rr.b),
			sql.Named("r", rr.r),
		); err != nil {
			t.Fatalf("exec insert failed for id=%d: %v", rr.id, err)
		}
	}

	// Prepare SELECT with multiple bind placeholders (named) and reuse it with different bind sets.
	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col FROM " + table + " WHERE id IN (:a, :b, :c) ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	// Define bind cases with varying Go types for the same named binds across re-executions.
	type bindCase struct {
		a, b, c any
		expIDs  []int64 // expected ids in ORDER BY id order
		label   string
	}
	cases := []bindCase{
		{a: int64(10), b: int64(12), c: int64(13), expIDs: []int64{10, 12, 13}, label: "int64"},
		{a: int(11), b: int(10), c: int(12), expIDs: []int64{10, 11, 12}, label: "int"},
		{a: "13", b: "11", c: "10", expIDs: []int64{10, 11, 13}, label: "string"},
	}

	// Helper to fetch expected row by id
	getExp := func(id int64) rowData {
		for _, r := range rowsToInsert {
			if r.id == id {
				return r
			}
		}
		t.Fatalf("internal error: expected id %d not found", id)
		return rowData{}
	}

	for _, bc := range cases {
		rows, err := selStmt.QueryContext(ctx,
			sql.Named("a", bc.a),
			sql.Named("b", bc.b),
			sql.Named("c", bc.c),
		)
		if err != nil {
			t.Fatalf("query failed for case %q with binds (%v,%v,%v): %v", bc.label, bc.a, bc.b, bc.c, err)
		}

		i := 0
		for rows.Next() {
			var (
				idOut  int64
				nOut   int64
				vOut   string
				fOut   float64
				dOut   time.Time
				tsOut  time.Time
				tztOut time.Time
				bOut   bool
				rOut   []byte
			)
			if err := rows.Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut); err != nil {
				_ = rows.Close()
				t.Fatalf("scan failed for case %q with binds (%v,%v,%v): %v", bc.label, bc.a, bc.b, bc.c, err)
			}
			if i >= len(bc.expIDs) {
				_ = rows.Close()
				t.Fatalf("received more rows than expected (case %q)", bc.label)
			}
			exp := getExp(bc.expIDs[i])

			if idOut != exp.id {
				_ = rows.Close()
				t.Fatalf("id mismatch (case %q): got %d want %d", bc.label, idOut, exp.id)
			}
			if nOut != exp.n {
				_ = rows.Close()
				t.Fatalf("n_col mismatch for id=%d (case %q): got %d want %d", exp.id, bc.label, nOut, exp.n)
			}
			if vOut != exp.v {
				_ = rows.Close()
				t.Fatalf("v_col mismatch for id=%d (case %q): got %q want %q", exp.id, bc.label, vOut, exp.v)
			}
			if diff := math.Abs(fOut - exp.f); diff > 1e-12 {
				_ = rows.Close()
				t.Fatalf("f_col mismatch for id=%d (case %q): got %.15f want %.15f (diff=%.3g)", exp.id, bc.label, fOut, exp.f, diff)
			}
			assertSameYMDHMSNanos(t, 0, "DATE", dOut, exp.d)
			assertSameYMDHMSNanos(t, 0, "TIMESTAMP", tsOut, exp.ts)
			assertSameYMDHMSNanos(t, 0, "TSTZ", tztOut, exp.tzt)
			if _, off := tztOut.Zone(); off != exp.off {
				_ = rows.Close()
				t.Fatalf("TSTZ offset mismatch for id=%d (case %q): got %d want %d", exp.id, bc.label, off, exp.off)
			}
			if bOut != exp.b {
				_ = rows.Close()
				t.Fatalf("b_col mismatch for id=%d (case %q): got %v want %v", exp.id, bc.label, bOut, exp.b)
			}
			if !bytes.Equal(rOut, exp.r) {
				_ = rows.Close()
				t.Fatalf("r_col mismatch for id=%d (case %q): got % X want % X", exp.id, bc.label, rOut, exp.r)
			}
			i++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("rows err for case %q with binds (%v,%v,%v): %v", bc.label, bc.a, bc.b, bc.c, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("rows close err (case %q): %v", bc.label, err)
		}
		if i != len(bc.expIDs) {
			t.Fatalf("row count mismatch (case %q): got %d want %d", bc.label, i, len(bc.expIDs))
		}
	}
}
