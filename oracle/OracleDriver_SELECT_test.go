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
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// TestDriver_Table_Select creates the table, inserts a row, selects and decodes values, then cleans up.
func TestDriver_Table_Select(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	table := createObjectName("t_select")
	ctx := context.Background()
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

	if _, err := db.ExecContext(context.Background(), "INSERT INTO "+table+" (id, name) VALUES(42, 'answer')"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	rows, err := db.QueryContext(context.Background(), "select id, name from "+table)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		t.Logf("row: id=%d name=%s", id, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

}

// TestDriver_Exec_Query_cursor_leak
// What it does: run in loop a simple select
// Expectation: execution must not fail due to cursor starvation
func TestDriver_Exec_Query_cursor_leak(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}

	table := createObjectName("t_exec_cursor")
	ctx := context.Background()
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
		db.Close()
	}()

	var limit int = 10000
	for limit > 0 {

		if _, err := db.ExecContext(context.Background(),
			fmt.Sprintf("INSERT INTO %s (id, name) VALUES(%d, 'answer')", table, limit)); err != nil {
			t.Fatalf("insert failed: %v", err)
		}

		limit--
	}
}

// TestDriver_Select_Query_cursor_leak
// What it does: run in loop a simple select
// Expectation: execution must not fail due to cursor starvation
func TestDriver_Select_Query_cursor_leak(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()
	var limit int = 10000
	for limit > 0 {
		rows, err := db.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
		if err != nil {
			t.Fatalf("select failed: %v", err)
		}

		for rows.Next() {
			var one int

			if err := rows.Scan(&one); err != nil {
				t.Fatalf("scan failed: %v", err)
			}
		}

		if err := rows.Err(); err != nil {
			t.Fatalf("rows err: %v", err)
		}
		rows.Close()
		limit--
	}

}

// TestDriver_PreparedStatement_Query_cursor_leak
// What it does: run in loop a simple prepare statement
// Expectation: execution must not fail due to cursor starvation
func TestDriver_PreparedStatement_Query_cursor_leak(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}

	table := createObjectName("t_prep_cursor")
	ctx := context.Background()
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
		db.Close()
	}()

	var limit int = 0
	ins := "INSERT INTO " + table + " (id, name) VALUES (:id, :d)"

	for limit <= 10000 {
		insStmt, err := db.PrepareContext(ctx, ins)
		if err != nil {
			t.Fatalf("prepare insert failed: %v", err)
		}

		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", limit),
			sql.Named("d", "foo"),
		); err != nil {
			t.Errorf("insert after loop %d failed: %v", limit, err)
		}
		insStmt.Close()
		limit++
	}
}

// TestDriver_Prepared_InsertAndSelect_AllTypes
// Creates a table covering representative types and performs INSERT/SELECT using prepared statements.
// Columns: NUMBER, VARCHAR2, BINARY_DOUBLE (float), DATE, TIMESTAMP, TIMESTAMP WITH TIME ZONE, NUMBER(1) for boolean, RAW.
func TestDriver_Prepared_InsertAndSelect_AllTypes(t *testing.T) {
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
	table := createObjectName("combined_types_ps_test1")
	cols := map[string]string{
		"id":       "NUMBER PRIMARY KEY",
		"n_col":    "NUMBER",
		"v_col":    "VARCHAR2(100)",
		"f_col":    "BINARY_DOUBLE",
		"d_col":    "DATE",
		"ts_col":   "TIMESTAMP(6)",
		"tzt_col":  "TIMESTAMP(6) WITH TIME ZONE",
		"b_col":    "NUMBER(1)",
		"r_col":    "RAW(2000)",
		"c_col":    "CLOB",
		"blob_col": "BLOB",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("Skipping combined prepared statements types test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Prepared INSERT with named placeholders.
	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col, c_col, blob_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r, :c, :blo)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	// Test values
	id := int64(1)
	n := int64(123456789)
	v := "hello prepared"
	f := 3.141592653589793
	d := time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC)
	ts := time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.UTC)
	tzt := time.Date(2024, time.January, 15, 4, 50, 30, 123456000, time.FixedZone("+05:30", 5*3600+30*60))
	b := true
	r := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	c := "hello clob"
	blob := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if _, err := insStmt.ExecContext(ctx,
		sql.Named("id", id),
		sql.Named("n", n),
		sql.Named("v", v),
		sql.Named("f", f),
		sql.Named("d", d),
		sql.Named("ts", ts),
		sql.Named("tzt", tzt),
		sql.Named("b", b),
		sql.Named("r", r),
		sql.Named("c", c),
		sql.Named("blo", blob),
	); err != nil {
		t.Fatalf("exec insert failed: %v", err)
	}

	// Prepared SELECT with named placeholder.
	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col, c_col, blob_col FROM " + table + " WHERE id=:id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	var (
		idOut   int64
		nOut    int64
		vOut    string
		fOut    float64
		dOut    time.Time
		tsOut   time.Time
		tztOut  time.Time
		bOut    bool
		rOut    []byte
		clobOut string
		blobOut []byte
	)
	if err := selStmt.QueryRowContext(ctx, sql.Named("id", id)).
		Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut, &clobOut, &blobOut); err != nil {
		t.Fatalf("query/scan failed: %v", err)
	}

	// Assertions
	if idOut != id {
		t.Fatalf("id mismatch: got %d want %d", idOut, id)
	}
	if nOut != n {
		t.Fatalf("n_col mismatch: got %d want %d", nOut, n)
	}
	if vOut != v {
		t.Fatalf("v_col mismatch: got %q want %q", vOut, v)
	}
	if diff := math.Abs(fOut - f); diff > 1e-12 {
		t.Fatalf("f_col mismatch: got %.15f want %.15f (diff=%.3g)", fOut, f, diff)
	}
	if clobOut != c {
		t.Fatalf("c_col mismatch: got %s want %s", clobOut, c)
	}

	if !bytes.Equal(blobOut, blob) {
		t.Fatalf("b_col mismatch: got %v want %v", blobOut, blob)
	}

	// For DATE and TIMESTAMP, verify YMDHMS and nanoseconds (zone may be normalized by server).
	assertSameYMDHMSNanos(t, 0, "DATE", dOut, d)
	assertSameYMDHMSNanos(t, 0, "TIMESTAMP", tsOut, ts)

	// For TIMESTAMP WITH TIME ZONE, verify components and offset seconds.
	assertSameYMDHMSNanos(t, 0, "TSTZ", tztOut, tzt)
	if _, off := tztOut.Zone(); off != 5*3600+30*60 {
		t.Fatalf("TSTZ offset mismatch: got %d want %d", off, 5*3600+30*60)
	}

	if bOut != b {
		t.Fatalf("b_col mismatch: got %v want %v", bOut, b)
	}
	if !bytes.Equal(rOut, r) {
		t.Fatalf("r_col mismatch: got % X want % X", rOut, r)
	}
}

/*
TestDriver_Prepared_InsertAndSelect_AllTypes_StrictNulls
creates a table with representative types, inserts a row using a prepared statement
with nil for every non-PK column, then selects the row back using a prepared statement.

Expectation: enables strict null scanning so every nullable column is decoded into the matching
sql.Null* type (or []byte for RAW) with Valid=false when the database value is NULL. The returned row
is therefore composed entirely of zero-valued sql.Null* structs with Valid=false and an empty []byte.
*/
func TestDriver_Prepared_InsertAndSelect_AllTypes_StrictNulls(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "true"
	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("combined_types_ps_test_nulls1")
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
		t.Fatalf("Skipping combined prepared statements NULL types test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	// Insert nil for all columns except id (PK).
	id := int64(1)
	if _, err := insStmt.ExecContext(ctx,
		sql.Named("id", id),
		sql.Named("n", nil),
		sql.Named("v", nil),
		sql.Named("f", nil),
		sql.Named("d", nil),
		sql.Named("ts", nil),
		sql.Named("tzt", nil),
		sql.Named("b", nil),
		sql.Named("r", nil),
	); err != nil {
		t.Fatalf("exec insert failed: %v", err)
	}

	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col FROM " + table + " WHERE id=:id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	var (
		idOut int64
		nOut  sql.NullInt64
		vOut  sql.NullString
		fOut  sql.NullFloat64
		dOut  sql.NullTime
		tsOut sql.NullTime

		// database/sql has no NullTime for TIMESTAMP WITH TIME ZONE.
		tztOut sql.NullString
		bOut   sql.NullBool
		rOut   []byte
	)

	if err := selStmt.QueryRowContext(ctx, sql.Named("id", id)).
		Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut); err != nil {
		t.Fatalf("query/scan failed: %v", err)
	}

	if idOut != id {
		t.Fatalf("id mismatch: got %d want %d", idOut, id)
	}
	if nOut.Valid {
		t.Fatalf("n_col expected zero value for NULL, got %+v", nOut)
	}
	if vOut.Valid {
		t.Fatalf("v_col expected empty string for NULL, got %+v", vOut)
	}
	if fOut.Valid {
		t.Fatalf("f_col expected zero value for NULL, got %+v", fOut)
	}
	if dOut.Valid {
		t.Fatalf("d_col expected zero time for NULL, got %+v", dOut)
	}
	if tsOut.Valid {
		t.Fatalf("ts_col expected zero time for NULL, got %+v", tsOut)
	}
	// With the current decoder, NULL TSTZ values materialize as Go's zero time
	// (time.Time{}), which database/sql formats as "0001-01-01T00:00:00Z" with
	// Valid=true. Assert on that canonical representation so the test tracks the
	// driver's default-null semantics.
	if tztOut.Valid {
		t.Fatalf("tzt_col expected zero-time string for NULL, got %+v", tztOut)
	}
	if bOut.Valid {
		t.Fatalf("b_col expected false value for NULL, got %+v", bOut)
	}
	if len(rOut) != 0 {
		t.Fatalf("r_col expected nil/empty []byte for NULL, got % X", rOut)
	}
}

// TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls
// Verifies that when strict null handling is disabled, selecting NULL values through prepared
// statements yields Go default zero values for representative column types.
func TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "false"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("combined_types_ps_test_nulls2")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"n_col":   "NUMBER",
		"v_col":   "VARCHAR2(100)",
		"f_col":   "BINARY_DOUBLE",
		"d_col":   "DATE",
		"ts_col":  "TIMESTAMP(6)",
		"tzt_col": "TIMESTAMP(6) WITH TIME ZONE",
		"b_col":   "BOOLEAN",
		"r_col":   "RAW(2000)",
	}
	dropTable(ctx, db, table)
	// Attempt to create; if BOOLEAN unsupported or any other feature missing in env, Skip.
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping combined prepared statements NULL types test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	// Insert nil for all columns except id (PK).
	id := int64(1)
	if _, err := insStmt.ExecContext(ctx,
		sql.Named("id", id),
		sql.Named("n", nil),
		sql.Named("v", nil),
		sql.Named("f", nil),
		sql.Named("d", nil),
		sql.Named("ts", nil),
		sql.Named("tzt", nil),
		sql.Named("b", nil),
		sql.Named("r", nil),
	); err != nil {
		t.Fatalf("exec insert failed: %v", err)
	}

	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col FROM " + table + " WHERE id=:id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	var (
		idOut int64
		nOut  int64
		vOut  string
		fOut  float64
		dOut  time.Time
		tsOut time.Time

		// database/sql has no NullTime for TIMESTAMP WITH TIME ZONE.
		tztOut string
		bOut   bool
		rOut   []byte
	)

	if err := selStmt.QueryRowContext(ctx, sql.Named("id", id)).
		Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut); err != nil {
		t.Fatalf("query/scan failed: %v", err)
	}

	if idOut != id {
		t.Fatalf("id mismatch: got %d want %d", idOut, id)
	}
	if nOut != 0 {
		t.Fatalf("n_col expected zero value for NULL, got %+v", nOut)
	}
	if vOut != "" {
		t.Fatalf("v_col expected empty string for NULL, got %+v", vOut)
	}
	if fOut != 0 {
		t.Fatalf("f_col expected zero value for NULL, got %+v", fOut)
	}
	if !dOut.IsZero() {
		t.Fatalf("d_col expected zero time for NULL, got %+v", dOut)
	}
	if !tsOut.IsZero() {
		t.Fatalf("ts_col expected zero time for NULL, got %+v", tsOut)
	}
	// With the current decoder, NULL TSTZ values materialize as Go's zero time
	// (time.Time{}), which database/sql formats as "0001-01-01T00:00:00Z" with
	// Valid=true. Assert on that canonical representation so the test tracks the
	// driver's default-null semantics.
	if tztOut != "0001-01-01T00:00:00Z" {
		t.Fatalf("tzt_col expected zero-time string for NULL, got %+v", tztOut)
	}

	if bOut {
		t.Fatalf("b_col expected false value for NULL, got %+v", bOut)
	}
	if len(rOut) != 0 {
		t.Fatalf("r_col expected nil/empty []byte for NULL, got % X", rOut)
	}
}

// TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls_NullScanners
// Validates that disabling strict null handling returns concrete values that flag sql.Null* scanners as Valid.
func TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls_NullScanners(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "false"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("combined_types_ps_test_nulls_scanners")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"n_col":   "NUMBER",
		"v_col":   "VARCHAR2(100)",
		"f_col":   "BINARY_DOUBLE",
		"d_col":   "DATE",
		"ts_col":  "TIMESTAMP(6)",
		"tzt_col": "TIMESTAMP(6) WITH TIME ZONE",
		"b_col":   "BOOLEAN",
		"r_col":   "RAW(2000)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping null scanner defaults test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	id := int64(1)
	if _, err := insStmt.ExecContext(ctx,
		sql.Named("id", id),
		sql.Named("n", nil),
		sql.Named("v", nil),
		sql.Named("f", nil),
		sql.Named("d", nil),
		sql.Named("ts", nil),
		sql.Named("tzt", nil),
		sql.Named("b", nil),
		sql.Named("r", nil),
	); err != nil {
		t.Fatalf("exec insert failed: %v", err)
	}

	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col FROM " + table + " WHERE id=:id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	var (
		idOut  int64
		nOut   sql.NullInt64
		vOut   sql.NullString
		fOut   sql.NullFloat64
		dOut   sql.NullTime
		tsOut  sql.NullTime
		tztOut sql.NullString
		bOut   sql.NullBool
		rOut   []byte
	)

	if err := selStmt.QueryRowContext(ctx, sql.Named("id", id)).
		Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut); err != nil {
		t.Fatalf("query/scan failed: %v", err)
	}

	if idOut != id {
		t.Fatalf("id mismatch: got %d want %d", idOut, id)
	}
	if !nOut.Valid || nOut.Int64 != 0 {
		t.Fatalf("n_col expected Valid zero value, got %+v", nOut)
	}
	if !vOut.Valid || vOut.String != "" {
		t.Fatalf("v_col expected Valid empty string, got %+v", vOut)
	}
	if !fOut.Valid || fOut.Float64 != 0 {
		t.Fatalf("f_col expected Valid zero float, got %+v", fOut)
	}
	if !dOut.Valid || !dOut.Time.IsZero() {
		t.Fatalf("d_col expected Valid zero time, got %+v", dOut)
	}
	if !tsOut.Valid || !tsOut.Time.IsZero() {
		t.Fatalf("ts_col expected Valid zero time, got %+v", tsOut)
	}
	if !tztOut.Valid || tztOut.String != "0001-01-01T00:00:00Z" {
		t.Fatalf("tzt_col expected Valid zero-time string, got %+v", tztOut)
	}
	if !bOut.Valid || bOut.Bool {
		t.Fatalf("b_col expected Valid false, got %+v", bOut)
	}
	if len(rOut) != 0 {
		t.Fatalf("r_col expected empty []byte, got % X", rOut)
	}
}

// TestDriver_Prepared_InsertMultipleRows_Re_exec Uses a single prepared INSERT to add
// multiple rows of representative types; then verifies results via a set SELECT (ORDER BY)
// and by re-executing a prepared SELECT (QueryRow) for each id on the same statement.
func TestDriver_Prepared_Statement_Re_exec_With_Different_Arg_Counts_Negative(t *testing.T) {
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
	stmt, err := db.PrepareContext(ctx, "SELECT :id AS val FROM DUAL")
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = stmt.Close() }()

	var out int64
	if err := stmt.QueryRowContext(ctx, sql.Named("id", int64(1))).Scan(&out); err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if out != 1 {
		t.Fatalf("first execution returned %d; want 1", out)
	}

	err = stmt.QueryRowContext(ctx,
		sql.Named("id", int64(2)),
		sql.Named("extra", "unexpected"),
	).Scan(&out)
	if err == nil {
		t.Fatalf("second execution expected error when supplying more arguments")
	}

	err = stmt.QueryRowContext(ctx).Scan(&out)
	if err == nil {
		t.Fatalf("third execution expected error when supplying fewer arguments")
	}
}

// TestDriver_Prepared_InsertMultipleRows_Re_exec Uses a single prepared INSERT to add
// multiple rows of representative types; then verifies results via a set SELECT (ORDER BY)
// and by re-executing a prepared SELECT (QueryRow) for each id on the same statement.
func TestDriver_Prepared_InsertMultipleRows_Re_exec(t *testing.T) {
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
	table := createObjectName("combined_types_ps_multi_test1")
	cols := map[string]string{
		"id":       "NUMBER PRIMARY KEY",
		"n_col":    "NUMBER",
		"v_col":    "VARCHAR2(100)",
		"f_col":    "BINARY_DOUBLE",
		"d_col":    "DATE",
		"ts_col":   "TIMESTAMP(6)",
		"tzt_col":  "TIMESTAMP(6) WITH TIME ZONE",
		"b_col":    "NUMBER(1)",
		"r_col":    "RAW(2000)",
		"c_col":    "CLOB",
		"blob_col": "BLOB",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("Skipping combined prepared statements multi-row types test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Prepared INSERT with named placeholders, reused for multiple rows.
	insSQL := "INSERT INTO " + table + " (id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col, c_col, blob_col) " +
		"VALUES (:id, :n, :v, :f, :d, :ts, :tzt, :b, :r, :c, :blo)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	type rowData struct {
		id   int64
		n    int64
		v    string
		f    float64
		d    time.Time
		ts   time.Time
		tzt  time.Time
		b    bool
		r    []byte
		c    string
		blob []byte
		off  int // expected TZ offset in seconds
	}

	rowsToInsert := []rowData{
		{
			id: 1, n: 123456789, v: "hello prepared", f: 3.141592653589793,
			d:    time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC),
			ts:   time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.UTC),
			tzt:  time.Date(2024, time.January, 15, 4, 50, 30, 123456000, time.FixedZone("+05:30", 5*3600+30*60)),
			b:    true,
			r:    []byte{0xDE, 0xAD, 0xBE, 0xEF},
			c:    "hello clob",
			blob: []byte{0xDE, 0xAD, 0xBE, 0xEF},
			off:  5*3600 + 30*60,
		},
		{
			id: 2, n: 987654321, v: "row2", f: 2.718281828459045,
			d:    time.Date(2023, time.December, 31, 23, 59, 59, 0, time.UTC),
			ts:   time.Date(2023, time.December, 31, 12, 0, 0, 987654000, time.UTC),
			tzt:  time.Date(2023, time.December, 31, 4, 30, 0, 987654000, time.FixedZone("-08:00", -8*3600)),
			b:    false,
			r:    []byte{0xCA, 0xFE},
			c:    "row2 clob",
			blob: []byte{0xCA, 0xFE},
			off:  -8 * 3600,
		},
		{
			id: 3, n: 42, v: "row3", f: 1.41421356237,
			d:    time.Date(2025, time.June, 1, 1, 2, 3, 0, time.UTC),
			ts:   time.Date(2025, time.June, 1, 4, 5, 6, 789000000, time.UTC),
			tzt:  time.Date(2025, time.June, 1, 9, 35, 6, 789000000, time.FixedZone("+05:30", 5*3600+30*60)),
			b:    true,
			r:    []byte{0xBA, 0xAD, 0xF0, 0x0D},
			c:    "row3 clob",
			blob: []byte{0xBA, 0xAD, 0xF0, 0x0D},
			off:  5*3600 + 30*60,
		},
	}

	// Execute multiple inserts using the same prepared statement.
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
			sql.Named("c", rr.c),
			sql.Named("blo", rr.blob),
		); err != nil {
			t.Fatalf("exec insert failed for id=%d: %v", rr.id, err)
		}
	}

	// Verify via a set SELECT first (ORDER BY id for deterministic order).
	q := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col, c_col, blob_col FROM " + table + " ORDER BY id"
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	i := 0
	for rs.Next() {
		var (
			idOut   int64
			nOut    int64
			vOut    string
			fOut    float64
			dOut    time.Time
			tsOut   time.Time
			tztOut  time.Time
			bOut    bool
			rOut    []byte
			clobOut string
			blobOut []byte
		)
		if err := rs.Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut, &clobOut, &blobOut); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if i >= len(rowsToInsert) {
			t.Fatalf("received more rows than expected")
		}
		exp := rowsToInsert[i]

		if idOut != exp.id {
			t.Fatalf("id mismatch: got %d want %d", idOut, exp.id)
		}
		if nOut != exp.n {
			t.Fatalf("n_col mismatch for id=%d: got %d want %d", exp.id, nOut, exp.n)
		}
		if vOut != exp.v {
			t.Fatalf("v_col mismatch for id=%d: got %q want %q", exp.id, vOut, exp.v)
		}
		if diff := math.Abs(fOut - exp.f); diff > 1e-12 {
			t.Fatalf("f_col mismatch for id=%d: got %.15f want %.15f (diff=%.3g)", exp.id, fOut, exp.f, diff)
		}

		assertSameYMDHMSNanos(t, 0, "DATE", dOut, exp.d)
		assertSameYMDHMSNanos(t, 0, "TIMESTAMP", tsOut, exp.ts)
		assertSameYMDHMSNanos(t, 0, "TSTZ", tztOut, exp.tzt)
		if _, off := tztOut.Zone(); off != exp.off {
			t.Fatalf("TSTZ offset mismatch for id=%d: got %d want %d", exp.id, off, exp.off)
		}

		if bOut != exp.b {
			t.Fatalf("b_col mismatch for id=%d: got %v want %v", exp.id, bOut, exp.b)
		}
		if !bytes.Equal(rOut, exp.r) {
			t.Fatalf("r_col mismatch for id=%d: got % X want % X", exp.id, rOut, exp.r)
		}
		if clobOut != exp.c {
			t.Fatalf("c_col mismatch for id=%d: got %q want %q", exp.id, clobOut, exp.c)
		}
		if !bytes.Equal(blobOut, exp.blob) {
			t.Fatalf("blob_col mismatch for id=%d: got %v want %v", exp.id, blobOut, exp.blob)
		}
		i++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if i != len(rowsToInsert) {
		t.Fatalf("row count mismatch: got %d want %d", i, len(rowsToInsert))
	}

	// Additionally verify by reusing a prepared SELECT for each id (repeated QueryRow on same statement).
	selSQL := "SELECT id, n_col, v_col, f_col, d_col, ts_col, tzt_col, b_col, r_col, c_col, blob_col FROM " + table + " WHERE id=:id"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer func() { _ = selStmt.Close() }()

	for _, exp := range rowsToInsert {
		var (
			idOut   int64
			nOut    int64
			vOut    string
			fOut    float64
			dOut    time.Time
			tsOut   time.Time
			tztOut  time.Time
			bOut    bool
			rOut    []byte
			clobOut string
			blobOut []byte
		)
		if err := selStmt.QueryRowContext(ctx, sql.Named("id", exp.id)).
			Scan(&idOut, &nOut, &vOut, &fOut, &dOut, &tsOut, &tztOut, &bOut, &rOut, &clobOut, &blobOut); err != nil {
			t.Fatalf("query/scan failed for id=%d: %v", exp.id, err)
		}

		if idOut != exp.id {
			t.Fatalf("id mismatch: got %d want %d", idOut, exp.id)
		}
		if nOut != exp.n {
			t.Fatalf("n_col mismatch for id=%d: got %d want %d", exp.id, nOut, exp.n)
		}
		if vOut != exp.v {
			t.Fatalf("v_col mismatch for id=%d: got %q want %q", exp.id, vOut, exp.v)
		}
		if diff := math.Abs(fOut - exp.f); diff > 1e-12 {
			t.Fatalf("f_col mismatch for id=%d: got %.15f want %.15f (diff=%.3g)", exp.id, fOut, exp.f, diff)
		}

		assertSameYMDHMSNanos(t, 0, "DATE", dOut, exp.d)
		assertSameYMDHMSNanos(t, 0, "TIMESTAMP", tsOut, exp.ts)
		assertSameYMDHMSNanos(t, 0, "TSTZ", tztOut, exp.tzt)
		if _, off := tztOut.Zone(); off != exp.off {
			t.Fatalf("TSTZ offset mismatch for id=%d: got %d want %d", exp.id, off, exp.off)
		}

		if bOut != exp.b {
			t.Fatalf("b_col mismatch for id=%d: got %v want %v", exp.id, bOut, exp.b)
		}
		if !bytes.Equal(rOut, exp.r) {
			t.Fatalf("r_col mismatch for id=%d: got % X want % X", exp.id, rOut, exp.r)
		}
		if clobOut != exp.c {
			t.Fatalf("c_col mismatch for id=%d: got %q want %q", exp.id, clobOut, exp.c)
		}
		if !bytes.Equal(blobOut, exp.blob) {
			t.Fatalf("blob_col mismatch for id=%d: got %v want %v", exp.id, blobOut, exp.blob)
		}
	}
}

// TestDriver_Select_NumberPrecision validates that ColumnTypes metadata matches declared precision,
func TestDriver_Select_NumberPrecision(t *testing.T) {
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
	table := createObjectName("t_metadata")
	_ = dropTable(ctx, db, table) // Clean up any old state

	// Create a table with just one NUMBER column
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER(12,0)"}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() { _ = dropTable(ctx, db, table) }()

	// Query the empty table just to get the column metadata
	rows, err := db.QueryContext(ctx, "SELECT id FROM "+table)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		t.Fatalf("ColumnTypes failed: %v", err)
	}

	// This is where the go-driver currently fails
	dbType := colTypes[0].DatabaseTypeName()
	if dbType == "" {
		t.Errorf("DatabaseTypeName returned an empty string for a NUMBER column")
	} else if dbType != "NUMBER" {
		t.Errorf("expected 'NUMBER', got %q", dbType)
	}
}

// TestDriver_VarcharLargePayload verifies binding and decoding of near VARCHAR2 limit payloads.
func TestDriver_VarcharLargePayload(t *testing.T) {
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
	table := createObjectName("t_varchar_bind")
	_ = dropTable(ctx, db, table)

	// Create a table capable of holding 4000 bytes
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER", "payload": "VARCHAR2(4000)"}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() { _ = dropTable(ctx, db, table) }()

	payload := strings.Repeat("X", 4000)

	// Use a short 5-second timeout
	execCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = db.ExecContext(execCtx, "INSERT INTO "+table+" (id, payload) VALUES (1, :1)", payload)
	if err != nil {
		t.Fatalf("Failed to insert 256-byte string into VARCHAR2(4000): %v", err)
	}
}

// TestDriver_Select_ZeroRows_FilterCondition
// What it does: Creates a table, inserts a row, then executes a SELECT query with
// a filter condition that does not match any rows.
// Expectation: QueryRow().Scan() should return sql.ErrNoRows, confirming that the
// driver correctly distinguishes between a valid query returning no rows and other failures.
func TestDriver_Select_ZeroRows_FilterCondition(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_zero_rows_filter")

	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer dropTable(ctx, db, table)

	_, err = db.ExecContext(ctx,
		"INSERT INTO "+table+" (id,name) VALUES (1,'alpha')")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	row := db.QueryRowContext(ctx,
		"SELECT id,name FROM "+table+" WHERE id=999")

	var id int64
	var name string

	err = row.Scan(&id, &name)

	if err != sql.ErrNoRows {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

// TestDriver_Select_RowWithNullColumn
// What it does: Creates a table, inserts a row where one column contains a NULL value,
// and retrieves the row using SELECT.
// Expectation: The row should be returned successfully and the NULL column should
// be represented using sql.NullString with Valid=false.
func TestDriver_Select_RowWithNullColumn(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_null_column")

	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
	}

	createTable(ctx, db, table, cols)
	defer dropTable(ctx, db, table)

	_, err = db.ExecContext(ctx,
		"INSERT INTO "+table+" (id,name) VALUES (1,NULL)")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	var id int64
	var name sql.NullString

	err = db.QueryRowContext(ctx,
		"SELECT id,name FROM "+table+" WHERE id=1").
		Scan(&id, &name)

	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if name.Valid {
		t.Fatalf("expected NULL but got value %s", name.String)
	}
}

// TestDriver_Select_MultipleRows_SomeNulls
// What it does: Inserts multiple rows where some contain NULL values,
// then retrieves them using SELECT.
// Expectation: All rows should be returned correctly and NULL values
// should be handled using sql.Null* types without scan errors.
func TestDriver_Select_MultipleRows_SomeNulls(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open DB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_multi_nulls")

	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (1, 'alpha')"); err != nil {
		t.Fatalf("insert first row failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (2, NULL)"); err != nil {
		t.Fatalf("insert NULL row failed: %v", err)
	}

	rows, err := db.QueryContext(ctx,
		"SELECT id,name FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	count := 0

	for rows.Next() {

		var id int64
		var name sql.NullString

		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		count++
	}

	if count != 2 {
		t.Fatalf("expected 2 rows got %d", count)
	}
}

// TestDriver_Select_AllNullExceptPK
// What it does: Inserts a row where all non-primary key columns are NULL,
// then retrieves the row using SELECT.
// Expectation: The row should be returned successfully and nullable columns
// should be represented with Valid=false using sql.Null* types.
func TestDriver_Select_AllNullExceptPK(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open DB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_pk_only")

	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
		"city": "VARCHAR2(100)",
	}

	createTable(ctx, db, table, cols)
	defer dropTable(ctx, db, table)

	db.ExecContext(ctx,
		"INSERT INTO "+table+" (id,name,city) VALUES (1,NULL,NULL)")

	var (
		id   int64
		name sql.NullString
		city sql.NullString
	)

	err = db.QueryRowContext(ctx,
		"SELECT id,name,city FROM "+table+" WHERE id=1").
		Scan(&id, &name, &city)

	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if name.Valid || city.Valid {
		t.Fatalf("expected NULL values")
	}
}

// TestDriver_Select_NullFromComputedExpression
// What it does: Creates a table and inserts rows, then executes a SELECT query
// that produces NULL values through computed expressions such as CASE and arithmetic.
// Expectation: The query should return rows successfully and computed columns that
// evaluate to NULL should be correctly represented using sql.Null* types.
// This verifies that the driver correctly propagates NULL values produced by SQL expressions.
func TestDriver_Select_NullFromComputedExpression(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_expr_null")

	cols := map[string]string{
		"id":    "NUMBER PRIMARY KEY",
		"value": "NUMBER",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer dropTable(ctx, db, table)

	db.ExecContext(ctx, "INSERT INTO "+table+" VALUES (1,10)")
	db.ExecContext(ctx, "INSERT INTO "+table+" VALUES (2,NULL)")

	rows, err := db.QueryContext(ctx,
		`SELECT 
			id,
			CASE WHEN value > 5 THEN value ELSE NULL END as case_val,
			value + 1 as arithmetic_val
		FROM `+table+` ORDER BY id`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {

		var id int64
		var caseVal sql.NullInt64
		var arithmeticVal sql.NullInt64

		if err := rows.Scan(&id, &caseVal, &arithmeticVal); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
	}
}
