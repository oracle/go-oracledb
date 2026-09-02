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
)

// TestDriver_Select_Intervals
// Purpose: End-to-end SELECT test for INTERVAL YEAR TO MONTH and INTERVAL DAY TO SECOND
// using literal inserts and verifying canonical decoded string representations.
// Modeled after TestDriver_Select_CharacterTypes.
func TestDriver_Select_Intervals(t *testing.T) {
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
	table := createObjectName("interval_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"name":              "VARCHAR2(20)",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	inserts := []string{
		// id, name, contract_duration (Y2M), event_duration (D2S)
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (1, 'abc', INTERVAL '2-3' YEAR TO MONTH, INTERVAL '10 05:30:02' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (2, 'xyz', INTERVAL '-1-0' YEAR TO MONTH, INTERVAL '2 12:15:30' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (3, 'mno', INTERVAL '5-6' YEAR TO MONTH, INTERVAL '-1 06:45:00' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (4, 'pqr', INTERVAL '123-2' YEAR(3) TO MONTH, INTERVAL '5 00:00:00' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (5, 'stu', INTERVAL '123-0' YEAR(3) TO MONTH, INTERVAL '0 10:15:45' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (6, 'vwx', INTERVAL '25-0' YEAR TO MONTH, INTERVAL '2 00:00:00' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (7, 'lmn', INTERVAL '4-0' YEAR TO MONTH, INTERVAL '5 00:00:00' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (8, 'opq', INTERVAL '4-2' YEAR TO MONTH, INTERVAL '0 10:15:45' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (9, 'rst', INTERVAL '123-0' YEAR(3) TO MONTH, INTERVAL '2 00:00:00' DAY TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (10, 'rst2', INTERVAL '123456789-0' YEAR(9) TO MONTH, INTERVAL '123456789 12:55:17.123456' DAY(9) TO SECOND)",
		// Additional negative/edge representations
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (11, 'mmm', INTERVAL '-0-5' YEAR TO MONTH, INTERVAL '123456789 12:55:17.123456' DAY(9) TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (12, 'nnn', INTERVAL '-0-11' YEAR TO MONTH, INTERVAL '123456789 12:55:17.123456' DAY(9) TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (13, 'ooo', INTERVAL '-1-2' YEAR TO MONTH, INTERVAL '123456789 12:55:17.123456' DAY(9) TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (14, 'ppp', INTERVAL '-2-11  ' YEAR TO MONTH, INTERVAL '123456789 12:55:17.123456' DAY(9) TO SECOND)",
		"INSERT INTO " + table + " (id, name, contract_duration, event_duration) VALUES (15, 'qqq', INTERVAL '    0-11' YEAR TO MONTH, INTERVAL '123456789 12:55:17.123456' DAY(9) TO SECOND)",
	}
	for i, sql := range inserts {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	q := "SELECT id, name, contract_duration, event_duration FROM " + table + " ORDER BY id"
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type row struct {
		id   int
		name string
		y2m  string
		d2s  string
	}
	exp := []row{
		{1, "abc", "02-03", "10 05:30:02.0"},
		{2, "xyz", "-01-00", "02 12:15:30.0"},
		{3, "mno", "05-06", "-1 06:45:00.0"},
		{4, "pqr", "123-02", "05 00:00:00.0"},
		{5, "stu", "123-00", "00 10:15:45.0"},
		{6, "vwx", "25-00", "02 00:00:00.0"},
		{7, "lmn", "04-00", "05 00:00:00.0"},
		{8, "opq", "04-02", "00 10:15:45.0"},
		{9, "rst", "123-00", "02 00:00:00.0"},
		{10, "rst2", "123456789-00", "123456789 12:55:17.123456"},
		{11, "mmm", "-00-05", "123456789 12:55:17.123456"},
		{12, "nnn", "-00-11", "123456789 12:55:17.123456"},
		{13, "ooo", "-01-02", "123456789 12:55:17.123456"},
		{14, "ppp", "-02-11", "123456789 12:55:17.123456"},
		{15, "qqq", "00-11", "123456789 12:55:17.123456"},
	}

	idx := 0
	for rs.Next() {
		var (
			id   int
			name string
			y2m  string
			d2s  string
		)
		if err := rs.Scan(&id, &name, &y2m, &d2s); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Log each fetched row for visibility in -v test output
		t.Logf("Row %d: id=%d name=%q y2m=%q d2s=%q", idx, id, name, y2m, d2s)
		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := exp[idx]
		if id != e.id || name != e.name || y2m != e.y2m || d2s != e.d2s {
			t.Fatalf("row %d mismatch:\n got: id=%d name=%q y2m=%q d2s=%q\nwant: id=%d name=%q y2m=%q d2s=%q",
				idx, id, name, y2m, d2s, e.id, e.name, e.y2m, e.d2s)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// Migrated from the OraHub test coverage MR.
func TestDriver_Select_Intervals_Prepared_Named(t *testing.T) {
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
	table := createObjectName("interval_ps_named_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"name":              "VARCHAR2(20)",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	type intervalRow struct {
		id   int
		name string
		// Strings passed to the bind; Oracle parses INTERVAL literals from strings.
		y2mIn string
		d2sIn string
		// Expected canonical strings returned by Oracle on SELECT.
		y2mExp string
		d2sExp string
	}

	cases := []intervalRow{
		{1, "a_named", "2-3", "10 05:30:02", "02-03", "10 05:30:02.0"},
		{2, "b_named", "-1-0", "2 12:15:30", "-01-00", "02 12:15:30.0"},
		{3, "c_named", "5-6", "-1 06:45:00", "05-06", "-1 06:45:00.0"},
		{4, "d_named", "123-2", "5 00:00:00", "123-02", "05 00:00:00.0"},
		{5, "e_named", "123456789-0", "123456789 12:55:17.123456", "123456789-00", "123456789 12:55:17.123456"},
	}

	// Prepared INSERT with named binds: the interval columns are passed as strings
	// wrapped in TO_YMINTERVAL / TO_DSINTERVAL Oracle conversion functions so that
	// the driver binds a VARCHAR2 and Oracle performs the string→interval cast.
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table +
		" (id, name, contract_duration, event_duration)" +
		" VALUES (:id, :name, TO_YMINTERVAL(:y2m), TO_DSINTERVAL(:d2s))"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })

	for _, c := range cases {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("id", c.id),
			sql.Named("name", c.name),
			sql.Named("y2m", c.y2mIn),
			sql.Named("d2s", c.d2sIn),
		); err != nil {
			t.Fatalf("insert id=%d failed: %v", c.id, err)
		}
	}

	// Prepared SELECT (no binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, name, contract_duration, event_duration FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var (
			id   int
			name string
			y2m  string
			d2s  string
		)
		if err := rs.Scan(&id, &name, &y2m, &d2s); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("Named prepared row %d: id=%d name=%q y2m=%q d2s=%q", idx, id, name, y2m, d2s)

		if idx >= len(cases) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := cases[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		if name != e.name {
			t.Fatalf("name mismatch at row %d: got %q want %q", idx, name, e.name)
		}
		if y2m != e.y2mExp {
			t.Fatalf("y2m mismatch at row %d: got %q want %q", idx, y2m, e.y2mExp)
		}
		if d2s != e.d2sExp {
			t.Fatalf("d2s mismatch at row %d: got %q want %q", idx, d2s, e.d2sExp)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(cases) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(cases))
	}
}

// Migrated from the OraHub test coverage MR.
func TestDriver_Select_Intervals_Prepared_Ordinal(t *testing.T) {
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
	table := createObjectName("interval_ps_ordinal_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"name":              "VARCHAR2(20)",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	type intervalRow struct {
		id     int
		name   string
		y2mIn  string
		d2sIn  string
		y2mExp string
		d2sExp string
	}

	cases := []intervalRow{
		{1, "a_ord", "2-3", "10 05:30:02", "02-03", "10 05:30:02.0"},
		{2, "b_ord", "-1-0", "2 12:15:30", "-01-00", "02 12:15:30.0"},
		{3, "c_ord", "5-6", "-1 06:45:00", "05-06", "-1 06:45:00.0"},
		{4, "d_ord", "123-2", "5 00:00:00", "123-02", "05 00:00:00.0"},
		{5, "e_ord", "123456789-0", "123456789 12:55:17.123456", "123456789-00", "123456789 12:55:17.123456"},
	}

	// Prepared INSERT with ordinal (positional) binds
	ins := "INSERT INTO " + table +
		" (id, name, contract_duration, event_duration)" +
		" VALUES (:1, :2, TO_YMINTERVAL(:3), TO_DSINTERVAL(:4))"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })

	for _, c := range cases {
		// Pass args positionally
		if _, err := insStmt.ExecContext(ctx, c.id, c.name, c.y2mIn, c.d2sIn); err != nil {
			t.Fatalf("insert id=%d failed: %v", c.id, err)
		}
	}

	// Prepared SELECT (no binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, name, contract_duration, event_duration FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var (
			id   int
			name string
			y2m  string
			d2s  string
		)
		if err := rs.Scan(&id, &name, &y2m, &d2s); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("Ordinal prepared row %d: id=%d name=%q y2m=%q d2s=%q", idx, id, name, y2m, d2s)

		if idx >= len(cases) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		e := cases[idx]
		if id != e.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, e.id)
		}
		if name != e.name {
			t.Fatalf("name mismatch at row %d: got %q want %q", idx, name, e.name)
		}
		if y2m != e.y2mExp {
			t.Fatalf("y2m mismatch at row %d: got %q want %q", idx, y2m, e.y2mExp)
		}
		if d2s != e.d2sExp {
			t.Fatalf("d2s mismatch at row %d: got %q want %q", idx, d2s, e.d2sExp)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(cases) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(cases))
	}
}

// Migrated from the OraHub test coverage MR.
func TestDriver_Select_Intervals_Negative_D2S_SubSecond(t *testing.T) {
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
	table := createObjectName("interval_neg_d2s_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	stmt := "INSERT INTO " + table + " (id, contract_duration, event_duration) VALUES (1, INTERVAL '-2-11' YEAR TO MONTH, INTERVAL '-2 23:59:59.999999' DAY TO SECOND)"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	rs, err := db.QueryContext(ctx, "SELECT id, contract_duration, event_duration FROM "+table)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	if !rs.Next() {
		t.Fatalf("no rows returned")
	}

	var id int
	var y2m, d2s string
	if err := rs.Scan(&id, &y2m, &d2s); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if id != 1 {
		t.Fatalf("id mismatch: got %d want 1", id)
	}
	if y2m != "-02-11" {
		t.Fatalf("y2m mismatch: got %q want %q", y2m, "-02-11")
	}
	if d2s != "-2 23:59:59.999999" {
		t.Fatalf("d2s mismatch: got %q want %q", d2s, "-2 23:59:59.999999")
	}

	if rs.Next() {
		t.Fatalf("unexpected additional rows")
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

// Migrated from the OraHub test coverage MR.
func TestDriver_Select_Intervals_Negative_Y2M_Positive_D2S(t *testing.T) {
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
	table := createObjectName("interval_mixed_sign_neg_pos_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	stmt := "INSERT INTO " + table + " (id, contract_duration, event_duration) VALUES (1, INTERVAL '-5-6' YEAR TO MONTH, INTERVAL '10 05:30:02' DAY TO SECOND)"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	rs, err := db.QueryContext(ctx, "SELECT id, contract_duration, event_duration FROM "+table)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	if !rs.Next() {
		t.Fatalf("no rows returned")
	}

	var id int
	var y2m, d2s string
	if err := rs.Scan(&id, &y2m, &d2s); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if id != 1 {
		t.Fatalf("id mismatch: got %d want 1", id)
	}
	if y2m != "-05-06" {
		t.Fatalf("y2m mismatch: got %q want %q", y2m, "-05-06")
	}
	if d2s != "10 05:30:02.0" {
		t.Fatalf("d2s mismatch: got %q want %q", d2s, "10 05:30:02.0")
	}

	if rs.Next() {
		t.Fatalf("unexpected additional rows")
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

// Migrated from the OraHub test coverage MR.
func TestDriver_Select_Intervals_Positive_Y2M_Negative_D2S(t *testing.T) {
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
	table := createObjectName("interval_mixed_sign_pos_neg_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	stmt := "INSERT INTO " + table + " (id, contract_duration, event_duration) VALUES (1, INTERVAL '3-4' YEAR TO MONTH, INTERVAL '-3 12:30:45.123456' DAY TO SECOND)"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	rs, err := db.QueryContext(ctx, "SELECT id, contract_duration, event_duration FROM "+table)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	if !rs.Next() {
		t.Fatalf("no rows returned")
	}

	var id int
	var y2m, d2s string
	if err := rs.Scan(&id, &y2m, &d2s); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if id != 1 {
		t.Fatalf("id mismatch: got %d want 1", id)
	}
	if y2m != "03-04" {
		t.Fatalf("y2m mismatch: got %q want %q", y2m, "03-04")
	}
	if d2s != "-3 12:30:45.123456" {
		t.Fatalf("d2s mismatch: got %q want %q", d2s, "-3 12:30:45.123456")
	}

	if rs.Next() {
		t.Fatalf("unexpected additional rows")
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

// Migrated from the OraHub test coverage MR.
func TestDriver_Select_Intervals_Negative_LargeScale(t *testing.T) {
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
	table := createObjectName("interval_neg_large_test")
	cols := map[string]string{
		"id":                "NUMBER PRIMARY KEY",
		"contract_duration": "INTERVAL YEAR(9) TO MONTH",
		"event_duration":    "INTERVAL DAY(9) TO SECOND",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	stmt := "INSERT INTO " + table + " (id, contract_duration, event_duration) VALUES (1, INTERVAL '-123-0' YEAR(3) TO MONTH, INTERVAL '-999 23:59:59.0' DAY(3) TO SECOND)"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	rs, err := db.QueryContext(ctx, "SELECT id, contract_duration, event_duration FROM "+table)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	if !rs.Next() {
		t.Fatalf("no rows returned")
	}

	var id int
	var y2m, d2s string
	if err := rs.Scan(&id, &y2m, &d2s); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if id != 1 {
		t.Fatalf("id mismatch: got %d want 1", id)
	}
	if y2m != "-123-00" {
		t.Fatalf("y2m mismatch: got %q want %q", y2m, "-123-00")
	}
	if d2s != "-999 23:59:59.0" {
		t.Fatalf("d2s mismatch: got %q want %q", d2s, "-999 23:59:59.0")
	}

	if rs.Next() {
		t.Fatalf("unexpected additional rows")
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}
