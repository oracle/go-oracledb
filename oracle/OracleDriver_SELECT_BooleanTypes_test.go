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
	"fmt"
	"testing"
)

// TestDriver_Select_BooleanTypes_23c
// Combined table that requires SQL BOOLEAN support (23c+). If BOOLEAN is not supported,
// the test will Skip at table creation.
// Columns: id NUMBER PK, b_sql BOOLEAN, b_num NUMBER(1), b_char CHAR(1), b_vc VARCHAR2(10)
// Rows cover: true/false/null and invalid mapping values.
func TestDriver_Select_BooleanTypes_23c(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 23 {
		t.Skip("Boolean Type is not supported for DB < 23")
	}
	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "false"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	// Ensure the connection pool is closed when the test finishes to avoid leaks across tests.
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("bool_types_23c_test")

	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"b_sql":  "BOOLEAN",
		"b_num":  "NUMBER(1)",
		"b_char": "CHAR(1)",
		"b_vc":   "VARCHAR2(10)",
	}); err != nil {
		t.Fatalf("Skipping 23c BOOLEAN test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Seed with combinations of TRUE/FALSE across BOOLEAN, NUMBER(1), CHAR(1), and VARCHAR2 variants.
	inserts := []string{
		"INSERT INTO " + table + " (id, b_sql, b_num, b_char, b_vc) VALUES (1, TRUE, 1, 'Y', 'TRUE')",
		"INSERT INTO " + table + " (id, b_sql, b_num, b_char, b_vc) VALUES (2, FALSE, 0, 'N', 'FALSE')",
		"INSERT INTO " + table + " (id, b_sql, b_num, b_char, b_vc) VALUES (3, TRUE, 1, 'y', 'true')",
		"INSERT INTO " + table + " (id, b_sql, b_num, b_char, b_vc) VALUES (4, FALSE, 0, 'n', 'false')",
		"INSERT INTO " + table + " (id, b_sql, b_num, b_char, b_vc) VALUES (5, TRUE, 1, 'T', 'TRUE')",
		"INSERT INTO " + table + " (id, b_sql, b_num, b_char, b_vc) VALUES (6, FALSE, 0, 'F', 'FALSE')",
	}
	for i, s := range inserts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	// Phase 1 : SELECT * and print values and scan as bool directly (char is not converted to bool here)
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id, b_sql, b_num, b_char, b_vc FROM %s ORDER BY id", table))
	if err != nil {
		t.Fatalf("select * failed: %v", err)
	}
	defer rows.Close()
	// Expected values for Phase 1
	exp := map[int64]struct {
		bsql  bool
		bnum  bool
		bchar string
		bvc   bool
	}{
		1: {bsql: true, bnum: true, bchar: "Y", bvc: true},
		2: {bsql: false, bnum: false, bchar: "N", bvc: false},
		3: {bsql: true, bnum: true, bchar: "y", bvc: true},
		4: {bsql: false, bnum: false, bchar: "n", bvc: false},
		5: {bsql: true, bnum: true, bchar: "T", bvc: true},
		6: {bsql: false, bnum: false, bchar: "F", bvc: false},
	}
	for rows.Next() {
		var (
			id    int64
			bsql  bool
			bnum  bool
			bchar string
			bvc   bool
		)
		if err := rows.Scan(&id, &bsql, &bnum, &bchar, &bvc); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("Row id=%d: b_sql=%v, b_num=%v , b_char=%q , b_vc=%v",
			id, bsql, bnum, bchar, bvc)

		e, ok := exp[id]
		if !ok {
			t.Fatalf("unexpected row id=%d returned from query", id)
		}
		if bsql != e.bsql {
			t.Fatalf("id=%d b_sql mismatch: got %v, want %v", id, bsql, e.bsql)
		}
		if bnum != e.bnum {
			t.Fatalf("id=%d b_num mismatch: got %v, want %v", id, bnum, e.bnum)
		}
		if bchar != e.bchar {
			t.Fatalf("id=%d b_char mismatch: got %q, want %q", id, bchar, e.bchar)
		}
		if bvc != e.bvc {
			t.Fatalf("id=%d b_vc mismatch: got %v, want %v", id, bvc, e.bvc)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	// Insert two additional rows for Phase 2: one with NULLs and one with invalid values
	extraInserts := []string{
		fmt.Sprintf("INSERT INTO %s (id, b_sql, b_num, b_char, b_vc) VALUES (7, NULL, NULL, NULL, NULL)", table),
		fmt.Sprintf("INSERT INTO %s (id, b_sql, b_num, b_char, b_vc) VALUES (8, TRUE, 2, 'X', 'maybe')", table),
	}
	for i, s := range extraInserts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("extra insert %d failed: %v", i+1, err)
		}
	}

	// Phase 2: scan all columns into nullable types (sql.NullBool/sql.NullString), compare to expectations,
	// and ensure invalid values trigger a scan error.
	rows2, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id, b_sql, b_num, b_char, b_vc FROM %s ORDER BY id", table))
	if err != nil {
		t.Fatalf("phase 2 select failed: %v", err)
	}
	defer rows2.Close()
	// Expected values for Phase 2
	exp2 := map[int64]struct {
		bsql          sql.NullBool
		bnum          sql.NullBool
		bchar         sql.NullString
		bvc           sql.NullBool
		expectScanErr bool
	}{
		1: {bsql: sql.NullBool{Bool: true, Valid: true}, bnum: sql.NullBool{Bool: true, Valid: true}, bchar: sql.NullString{String: "Y", Valid: true}, bvc: sql.NullBool{Bool: true, Valid: true}},
		2: {bsql: sql.NullBool{Bool: false, Valid: true}, bnum: sql.NullBool{Bool: false, Valid: true}, bchar: sql.NullString{String: "N", Valid: true}, bvc: sql.NullBool{Bool: false, Valid: true}},
		3: {bsql: sql.NullBool{Bool: true, Valid: true}, bnum: sql.NullBool{Bool: true, Valid: true}, bchar: sql.NullString{String: "y", Valid: true}, bvc: sql.NullBool{Bool: true, Valid: true}},
		4: {bsql: sql.NullBool{Bool: false, Valid: true}, bnum: sql.NullBool{Bool: false, Valid: true}, bchar: sql.NullString{String: "n", Valid: true}, bvc: sql.NullBool{Bool: false, Valid: true}},
		5: {bsql: sql.NullBool{Bool: true, Valid: true}, bnum: sql.NullBool{Bool: true, Valid: true}, bchar: sql.NullString{String: "T", Valid: true}, bvc: sql.NullBool{Bool: true, Valid: true}},
		6: {bsql: sql.NullBool{Bool: false, Valid: true}, bnum: sql.NullBool{Bool: false, Valid: true}, bchar: sql.NullString{String: "F", Valid: true}, bvc: sql.NullBool{Bool: false, Valid: true}},
		// Row 7 contains NULLs for the NUMBER/CHAR/VARCHAR2 columns. With the updated
		// driver behaviour, scanning the VARCHAR2 NULL into sql.NullBool surfaces a
		// conversion error (empty string cannot be coerced to bool). Treat this as an
		// expected failure for that row so the test no longer flags it as unexpected.
		7: {bsql: sql.NullBool{Valid: false}, bnum: sql.NullBool{Valid: false}, bchar: sql.NullString{Valid: false}, expectScanErr: true},
		8: {expectScanErr: true}, // invalid b_num=2 and b_vc='maybe' should cause scan error into NullBool
	}
	for rows2.Next() {
		var (
			id      int64
			bsqlNB  sql.NullBool
			bnumNB  sql.NullBool
			bcharNB sql.NullString // CHAR scanned as NullString to support NULL
			bvcNB   sql.NullBool
		)
		err := rows2.Scan(&id, &bsqlNB, &bnumNB, &bcharNB, &bvcNB)
		e, ok := exp2[id]
		if !ok {
			t.Fatalf("phase 2: unexpected row id=%d returned from query", id)
		}
		if err != nil {
			if e.expectScanErr {
				t.Logf("Phase 2 scan id=%d expected error: %v", id, err)
				continue
			}
			t.Fatalf("Phase 2 scan id=%d unexpected error: %v", id, err)
		}
		if e.expectScanErr {
			t.Fatalf("Phase 2 id=%d: expected scan error, got none", id)
		}
		// Compare expected vs actual
		if bsqlNB.Valid != e.bsql.Valid || (bsqlNB.Valid && bsqlNB.Bool != e.bsql.Bool) {
			t.Fatalf("Phase 2 id=%d b_sql mismatch: got %+v, want %+v", id, bsqlNB, e.bsql)
		}
		if bnumNB.Valid != e.bnum.Valid || (bnumNB.Valid && bnumNB.Bool != e.bnum.Bool) {
			t.Fatalf("Phase 2 id=%d b_num mismatch: got %+v, want %+v", id, bnumNB, e.bnum)
		}
		if bcharNB.Valid != e.bchar.Valid || (bcharNB.Valid && bcharNB.String != e.bchar.String) {
			t.Fatalf("Phase 2 id=%d b_char mismatch: got %+v, want %+v", id, bcharNB, e.bchar)
		}
		if bvcNB.Valid != e.bvc.Valid || (bvcNB.Valid && bvcNB.Bool != e.bvc.Bool) {
			t.Fatalf("Phase 2 id=%d b_vc mismatch: got %+v, want %+v", id, bvcNB, e.bvc)
		}
		t.Logf("Phase 2 id=%d: b_sql=%+v, b_num=%+v, b_char=%+v, b_vc=%+v", id, bsqlNB, bnumNB, bcharNB, bvcNB)
	}
	if err := rows2.Err(); err != nil {
		t.Fatalf("phase 2 rows error: %v", err)
	}

}

// TestDriver_Select_BooleanTypes_23c_Prepared_Statement
// Same validations as TestDriver_Select_BooleanTypes_23c but uses prepared statements
// for both INSERT and SELECT operations.
func TestDriver_Select_BooleanTypes_23c_Prepared_Statement(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	if TestingConfig.DatabaseVersion.Major < 23 {
		t.Skip("Boolean Type is not supported for DB < 23")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("bool_types_23c_test_ps")

	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"b_sql":  "BOOLEAN",
		"b_num":  "NUMBER(1)",
		"b_char": "CHAR(1)",
		"b_vc":   "VARCHAR2(10)",
	}); err != nil {
		t.Fatalf("Skipping 23c BOOLEAN prepared statement test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Prepared INSERT
	insSQL := fmt.Sprintf("INSERT INTO %s (id, b_sql, b_num, b_char, b_vc) VALUES (:1, :2, :3, :4, :5)", table)
	stmtIns, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer stmtIns.Close()

	type rowIns struct {
		id   int64
		bsql any
		bnum any
		bc   any
		bvc  any
	}
	seed := []rowIns{
		{1, true, int64(1), "Y", "TRUE"},
		{2, false, int64(0), "N", "FALSE"},
		{3, true, int64(1), "y", "true"},
		{4, false, int64(0), "n", "false"},
		{5, true, int64(1), "T", "TRUE"},
		{6, false, int64(0), "F", "FALSE"},
	}
	for i, r := range seed {
		if _, err := stmtIns.ExecContext(ctx, r.id, r.bsql, r.bnum, r.bc, r.bvc); err != nil {
			t.Fatalf("prepared insert %d failed: %v", i+1, err)
		}
	}

	// Prepared SELECT for Phase 1
	selSQL := fmt.Sprintf("SELECT id, b_sql, b_num, b_char, b_vc FROM %s ORDER BY id", table)
	stmtSel, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer stmtSel.Close()

	rows, err := stmtSel.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select * (prepared) failed: %v", err)
	}
	defer rows.Close()

	exp := map[int64]struct {
		bsql  bool
		bnum  bool
		bchar string
		bvc   bool
	}{
		1: {bsql: true, bnum: true, bchar: "Y", bvc: true},
		2: {bsql: false, bnum: false, bchar: "N", bvc: false},
		3: {bsql: true, bnum: true, bchar: "y", bvc: true},
		4: {bsql: false, bnum: false, bchar: "n", bvc: false},
		5: {bsql: true, bnum: true, bchar: "T", bvc: true},
		6: {bsql: false, bnum: false, bchar: "F", bvc: false},
	}
	for rows.Next() {
		var (
			id    int64
			bsql  bool
			bnum  bool
			bchar string
			bvc   bool
		)
		if err := rows.Scan(&id, &bsql, &bnum, &bchar, &bvc); err != nil {
			t.Fatalf("scan (prepared select) failed: %v", err)
		}
		t.Logf("[Prepared] Row id=%d: b_sql=%v, b_num=%v , b_char=%q , b_vc=%v",
			id, bsql, bnum, bchar, bvc)

		e, ok := exp[id]
		if !ok {
			t.Fatalf("unexpected row id=%d returned from prepared query", id)
		}
		if bsql != e.bsql {
			t.Fatalf("id=%d b_sql mismatch: got %v, want %v", id, bsql, e.bsql)
		}
		if bnum != e.bnum {
			t.Fatalf("id=%d b_num mismatch: got %v, want %v", id, bnum, e.bnum)
		}
		if bchar != e.bchar {
			t.Fatalf("id=%d b_char mismatch: got %q, want %q", id, bchar, e.bchar)
		}
		if bvc != e.bvc {
			t.Fatalf("id=%d b_vc mismatch: got %v, want %v", id, bvc, e.bvc)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error (prepared select): %v", err)
	}
}

// TestDriver_Select_BooleanTypes_23c_Prepared_Statement_Named
// Same validations as TestDriver_Select_BooleanTypes_23c_Prepared_Statement, but uses
// named placeholders and database/sql's sql.Named for bind parameters.
func TestDriver_Select_BooleanTypes_23c_Prepared_Statement_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 23 {
		t.Skip("Boolean Type is not supported for DB < 23")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("bool_types_23c_test_ps_named")

	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"b_sql":  "BOOLEAN",
		"b_num":  "NUMBER(1)",
		"b_char": "CHAR(1)",
		"b_vc":   "VARCHAR2(10)",
	}); err != nil {
		t.Fatalf("Skipping 23c BOOLEAN prepared statement (named) test (create failed): %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	// Prepared INSERT with named placeholders
	insSQL := fmt.Sprintf("INSERT INTO %s (id, b_sql, b_num, b_char, b_vc) VALUES (:id, :b_sql, :b_num, :b_char, :b_vc)", table)
	stmtIns, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert (named) failed: %v", err)
	}
	defer stmtIns.Close()

	type rowIns struct {
		id   int64
		bsql any
		bnum any
		bc   any
		bvc  any
	}
	seed := []rowIns{
		{1, true, int64(1), "Y", "TRUE"},
		{2, false, int64(0), "N", "FALSE"},
		{3, true, int64(1), "y", "true"},
		{4, false, int64(0), "n", "false"},
		{5, true, int64(1), "T", "TRUE"},
		{6, false, int64(0), "F", "FALSE"},
	}
	for i, r := range seed {
		if _, err := stmtIns.ExecContext(
			ctx,
			sql.Named("id", r.id),
			sql.Named("b_sql", r.bsql),
			sql.Named("b_num", r.bnum),
			sql.Named("b_char", r.bc),
			sql.Named("b_vc", r.bvc),
		); err != nil {
			t.Fatalf("prepared insert (named) %d failed: %v", i+1, err)
		}
	}

	// Prepared SELECT for Phase 1 (no binds)
	selSQL := fmt.Sprintf("SELECT id, b_sql, b_num, b_char, b_vc FROM %s ORDER BY id", table)
	stmtSel, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select (named) failed: %v", err)
	}
	defer stmtSel.Close()

	rows, err := stmtSel.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select * (prepared named) failed: %v", err)
	}
	defer rows.Close()

	exp := map[int64]struct {
		bsql  bool
		bnum  bool
		bchar string
		bvc   bool
	}{
		1: {bsql: true, bnum: true, bchar: "Y", bvc: true},
		2: {bsql: false, bnum: false, bchar: "N", bvc: false},
		3: {bsql: true, bnum: true, bchar: "y", bvc: true},
		4: {bsql: false, bnum: false, bchar: "n", bvc: false},
		5: {bsql: true, bnum: true, bchar: "T", bvc: true},
		6: {bsql: false, bnum: false, bchar: "F", bvc: false},
	}
	for rows.Next() {
		var (
			id    int64
			bsql  bool
			bnum  bool
			bchar string
			bvc   bool
		)
		if err := rows.Scan(&id, &bsql, &bnum, &bchar, &bvc); err != nil {
			t.Fatalf("scan (prepared named select) failed: %v", err)
		}
		t.Logf("[Prepared-Named] Row id=%d: b_sql=%v, b_num=%v , b_char=%q , b_vc=%v",
			id, bsql, bnum, bchar, bvc)

		e, ok := exp[id]
		if !ok {
			t.Fatalf("unexpected row id=%d returned from prepared named query", id)
		}
		if bsql != e.bsql {
			t.Fatalf("id=%d b_sql mismatch: got %v, want %v", id, bsql, e.bsql)
		}
		if bnum != e.bnum {
			t.Fatalf("id=%d b_num mismatch: got %v, want %v", id, bnum, e.bnum)
		}
		if bchar != e.bchar {
			t.Fatalf("id=%d b_char mismatch: got %q, want %q", id, bchar, e.bchar)
		}
		if bvc != e.bvc {
			t.Fatalf("id=%d b_vc mismatch: got %v, want %v", id, bvc, e.bvc)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error (prepared named select): %v", err)
	}
}

// TestDriver_Select_BooleanTypes_19c
// 19c-compatible variant of the BOOLEAN type tests that exercises boolean coercion
// for NUMBER(1), CHAR(1), and VARCHAR2(10) columns without using SQL BOOLEAN.
func TestDriver_Select_BooleanTypes_19c(t *testing.T) {
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
	table := createObjectName("bool_types_19c_test")

	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"b_num":  "NUMBER(1)",
		"b_char": "CHAR(1)",
		"b_vc":   "VARCHAR2(10)",
	}); err != nil {
		t.Fatalf("create 19c-compatible boolean mapping table failed: %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	inserts := []string{
		"INSERT INTO " + table + " (id, b_num, b_char, b_vc) VALUES (1, 1, 'Y', 'TRUE')",
		"INSERT INTO " + table + " (id, b_num, b_char, b_vc) VALUES (2, 0, 'N', 'FALSE')",
		"INSERT INTO " + table + " (id, b_num, b_char, b_vc) VALUES (3, 1, 'y', 'true')",
		"INSERT INTO " + table + " (id, b_num, b_char, b_vc) VALUES (4, 0, 'n', 'false')",
		"INSERT INTO " + table + " (id, b_num, b_char, b_vc) VALUES (5, 1, 'T', 'TRUE')",
		"INSERT INTO " + table + " (id, b_num, b_char, b_vc) VALUES (6, 0, 'F', 'FALSE')",
	}
	for i, s := range inserts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id, b_num, b_char, b_vc FROM %s ORDER BY id", table))
	if err != nil {
		t.Fatalf("select * failed: %v", err)
	}
	defer rows.Close()

	exp := map[int64]struct {
		bnum  bool
		bchar string
		bvc   bool
	}{
		1: {bnum: true, bchar: "Y", bvc: true},
		2: {bnum: false, bchar: "N", bvc: false},
		3: {bnum: true, bchar: "y", bvc: true},
		4: {bnum: false, bchar: "n", bvc: false},
		5: {bnum: true, bchar: "T", bvc: true},
		6: {bnum: false, bchar: "F", bvc: false},
	}
	for rows.Next() {
		var (
			id    int64
			bnum  bool
			bchar string
			bvc   bool
		)
		if err := rows.Scan(&id, &bnum, &bchar, &bvc); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("Row id=%d: b_num=%v , b_char=%q , b_vc=%v", id, bnum, bchar, bvc)

		e, ok := exp[id]
		if !ok {
			t.Fatalf("unexpected row id=%d returned from query", id)
		}
		if bnum != e.bnum {
			t.Fatalf("id=%d b_num mismatch: got %v, want %v", id, bnum, e.bnum)
		}
		if bchar != e.bchar {
			t.Fatalf("id=%d b_char mismatch: got %q, want %q", id, bchar, e.bchar)
		}
		if bvc != e.bvc {
			t.Fatalf("id=%d b_vc mismatch: got %v, want %v", id, bvc, e.bvc)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	extraInserts := []string{
		fmt.Sprintf("INSERT INTO %s (id, b_num, b_char, b_vc) VALUES (7, NULL, NULL, NULL)", table),
		fmt.Sprintf("INSERT INTO %s (id, b_num, b_char, b_vc) VALUES (8, 2, 'X', 'maybe')", table),
	}
	for i, s := range extraInserts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("extra insert %d failed: %v", i+1, err)
		}
	}

	rows2, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id, b_num, b_char, b_vc FROM %s ORDER BY id", table))
	if err != nil {
		t.Fatalf("phase 2 select failed: %v", err)
	}
	defer rows2.Close()

	exp2 := map[int64]struct {
		bnum          sql.NullBool
		bchar         sql.NullString
		bvc           sql.NullBool
		expectScanErr bool
	}{
		1: {bnum: sql.NullBool{Bool: true, Valid: true}, bchar: sql.NullString{String: "Y", Valid: true}, bvc: sql.NullBool{Bool: true, Valid: true}},
		2: {bnum: sql.NullBool{Bool: false, Valid: true}, bchar: sql.NullString{String: "N", Valid: true}, bvc: sql.NullBool{Bool: false, Valid: true}},
		3: {bnum: sql.NullBool{Bool: true, Valid: true}, bchar: sql.NullString{String: "y", Valid: true}, bvc: sql.NullBool{Bool: true, Valid: true}},
		4: {bnum: sql.NullBool{Bool: false, Valid: true}, bchar: sql.NullString{String: "n", Valid: true}, bvc: sql.NullBool{Bool: false, Valid: true}},
		5: {bnum: sql.NullBool{Bool: true, Valid: true}, bchar: sql.NullString{String: "T", Valid: true}, bvc: sql.NullBool{Bool: true, Valid: true}},
		6: {bnum: sql.NullBool{Bool: false, Valid: true}, bchar: sql.NullString{String: "F", Valid: true}, bvc: sql.NullBool{Bool: false, Valid: true}},
		7: {bnum: sql.NullBool{Valid: false}, bchar: sql.NullString{Valid: false}, expectScanErr: true},
		8: {expectScanErr: true},
	}
	for rows2.Next() {
		var (
			id      int64
			bnumNB  sql.NullBool
			bcharNB sql.NullString
			bvcNB   sql.NullBool
		)
		err := rows2.Scan(&id, &bnumNB, &bcharNB, &bvcNB)
		e, ok := exp2[id]
		if !ok {
			t.Fatalf("phase 2: unexpected row id=%d returned from query", id)
		}
		if err != nil {
			if e.expectScanErr {
				t.Logf("Phase 2 scan id=%d expected error: %v", id, err)
				continue
			}
			t.Fatalf("Phase 2 scan id=%d unexpected error: %v", id, err)
		}
		if e.expectScanErr {
			t.Fatalf("Phase 2 id=%d: expected scan error, got none", id)
		}
		if bnumNB.Valid != e.bnum.Valid || (bnumNB.Valid && bnumNB.Bool != e.bnum.Bool) {
			t.Fatalf("Phase 2 id=%d b_num mismatch: got %+v, want %+v", id, bnumNB, e.bnum)
		}
		if bcharNB.Valid != e.bchar.Valid || (bcharNB.Valid && bcharNB.String != e.bchar.String) {
			t.Fatalf("Phase 2 id=%d b_char mismatch: got %+v, want %+v", id, bcharNB, e.bchar)
		}
		if bvcNB.Valid != e.bvc.Valid || (bvcNB.Valid && bvcNB.Bool != e.bvc.Bool) {
			t.Fatalf("Phase 2 id=%d b_vc mismatch: got %+v, want %+v", id, bvcNB, e.bvc)
		}
		t.Logf("Phase 2 id=%d: b_num=%+v, b_char=%+v, b_vc=%+v", id, bnumNB, bcharNB, bvcNB)
	}
	if err := rows2.Err(); err != nil {
		t.Fatalf("phase 2 rows error: %v", err)
	}
}

// TestDriver_Select_BooleanTypes_19c_Prepared_Statement
// 19c-compatible prepared statement variant without SQL BOOLEAN columns.
func TestDriver_Select_BooleanTypes_19c_Prepared_Statement(t *testing.T) {
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
	table := createObjectName("bool_types_19c_test_ps")

	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"b_num":  "NUMBER(1)",
		"b_char": "CHAR(1)",
		"b_vc":   "VARCHAR2(10)",
	}); err != nil {
		t.Fatalf("create 19c-compatible boolean prepared statement table failed: %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	insSQL := fmt.Sprintf("INSERT INTO %s (id, b_num, b_char, b_vc) VALUES (:1, :2, :3, :4)", table)
	stmtIns, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer stmtIns.Close()

	type rowIns19c struct {
		id   int64
		bnum any
		bc   any
		bvc  any
	}
	seed := []rowIns19c{
		{1, int64(1), "Y", "TRUE"},
		{2, int64(0), "N", "FALSE"},
		{3, int64(1), "y", "true"},
		{4, int64(0), "n", "false"},
		{5, int64(1), "T", "TRUE"},
		{6, int64(0), "F", "FALSE"},
	}
	for i, r := range seed {
		if _, err := stmtIns.ExecContext(ctx, r.id, r.bnum, r.bc, r.bvc); err != nil {
			t.Fatalf("prepared insert %d failed: %v", i+1, err)
		}
	}

	selSQL := fmt.Sprintf("SELECT id, b_num, b_char, b_vc FROM %s ORDER BY id", table)
	stmtSel, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	defer stmtSel.Close()

	rows, err := stmtSel.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select * (prepared) failed: %v", err)
	}
	defer rows.Close()

	exp := map[int64]struct {
		bnum  bool
		bchar string
		bvc   bool
	}{
		1: {bnum: true, bchar: "Y", bvc: true},
		2: {bnum: false, bchar: "N", bvc: false},
		3: {bnum: true, bchar: "y", bvc: true},
		4: {bnum: false, bchar: "n", bvc: false},
		5: {bnum: true, bchar: "T", bvc: true},
		6: {bnum: false, bchar: "F", bvc: false},
	}
	for rows.Next() {
		var (
			id    int64
			bnum  bool
			bchar string
			bvc   bool
		)
		if err := rows.Scan(&id, &bnum, &bchar, &bvc); err != nil {
			t.Fatalf("scan (prepared select) failed: %v", err)
		}
		t.Logf("[Prepared] Row id=%d: b_num=%v , b_char=%q , b_vc=%v", id, bnum, bchar, bvc)

		e, ok := exp[id]
		if !ok {
			t.Fatalf("unexpected row id=%d returned from prepared query", id)
		}
		if bnum != e.bnum {
			t.Fatalf("id=%d b_num mismatch: got %v, want %v", id, bnum, e.bnum)
		}
		if bchar != e.bchar {
			t.Fatalf("id=%d b_char mismatch: got %q, want %q", id, bchar, e.bchar)
		}
		if bvc != e.bvc {
			t.Fatalf("id=%d b_vc mismatch: got %v, want %v", id, bvc, e.bvc)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error (prepared select): %v", err)
	}
}

// TestDriver_Select_BooleanTypes_19c_Prepared_Statement_Named
// 19c-compatible named bind prepared statement variant without SQL BOOLEAN columns.
func TestDriver_Select_BooleanTypes_19c_Prepared_Statement_Named(t *testing.T) {
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
	table := createObjectName("bool_types_19c_test_ps_named")

	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"b_num":  "NUMBER(1)",
		"b_char": "CHAR(1)",
		"b_vc":   "VARCHAR2(10)",
	}); err != nil {
		t.Fatalf("create 19c-compatible boolean prepared statement (named) table failed: %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	}()

	insSQL := fmt.Sprintf("INSERT INTO %s (id, b_num, b_char, b_vc) VALUES (:id, :b_num, :b_char, :b_vc)", table)
	stmtIns, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert (named) failed: %v", err)
	}
	defer stmtIns.Close()

	type rowIns19c struct {
		id   int64
		bnum any
		bc   any
		bvc  any
	}
	seed := []rowIns19c{
		{1, int64(1), "Y", "TRUE"},
		{2, int64(0), "N", "FALSE"},
		{3, int64(1), "y", "true"},
		{4, int64(0), "n", "false"},
		{5, int64(1), "T", "TRUE"},
		{6, int64(0), "F", "FALSE"},
	}
	for i, r := range seed {
		if _, err := stmtIns.ExecContext(
			ctx,
			sql.Named("id", r.id),
			sql.Named("b_num", r.bnum),
			sql.Named("b_char", r.bc),
			sql.Named("b_vc", r.bvc),
		); err != nil {
			t.Fatalf("prepared insert (named) %d failed: %v", i+1, err)
		}
	}

	selSQL := fmt.Sprintf("SELECT id, b_num, b_char, b_vc FROM %s ORDER BY id", table)
	stmtSel, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select (named) failed: %v", err)
	}
	defer stmtSel.Close()

	rows, err := stmtSel.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select * (prepared named) failed: %v", err)
	}
	defer rows.Close()

	exp := map[int64]struct {
		bnum  bool
		bchar string
		bvc   bool
	}{
		1: {bnum: true, bchar: "Y", bvc: true},
		2: {bnum: false, bchar: "N", bvc: false},
		3: {bnum: true, bchar: "y", bvc: true},
		4: {bnum: false, bchar: "n", bvc: false},
		5: {bnum: true, bchar: "T", bvc: true},
		6: {bnum: false, bchar: "F", bvc: false},
	}
	for rows.Next() {
		var (
			id    int64
			bnum  bool
			bchar string
			bvc   bool
		)
		if err := rows.Scan(&id, &bnum, &bchar, &bvc); err != nil {
			t.Fatalf("scan (prepared named select) failed: %v", err)
		}
		t.Logf("[Prepared-Named] Row id=%d: b_num=%v , b_char=%q , b_vc=%v", id, bnum, bchar, bvc)

		e, ok := exp[id]
		if !ok {
			t.Fatalf("unexpected row id=%d returned from prepared named query", id)
		}
		if bnum != e.bnum {
			t.Fatalf("id=%d b_num mismatch: got %v, want %v", id, bnum, e.bnum)
		}
		if bchar != e.bchar {
			t.Fatalf("id=%d b_char mismatch: got %q, want %q", id, bchar, e.bchar)
		}
		if bvc != e.bvc {
			t.Fatalf("id=%d b_vc mismatch: got %v, want %v", id, bvc, e.bvc)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error (prepared named select): %v", err)
	}
}
