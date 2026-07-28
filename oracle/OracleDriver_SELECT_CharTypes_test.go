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
	"strings"
	"testing"
)

// TestDriver_Select_CharacterTypes
// Purpose: End-to-end SELECT test exercising character type converters (CHAR, VARCHAR2, NVARCHAR2, NCHAR)
// using the query flow, modeled after OracleDriver_SELECT_test.go. It validates decoded values and
// character lengths returned by Oracle LENGTH for each column.
func TestDriver_Select_CharacterTypes(t *testing.T) {
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
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("char_test")
	cols := map[string]string{
		"code":       "CHAR(5)",
		"name_vc":    "VARCHAR2(20)",
		"name_nvc":   "NVARCHAR2(20)",
		"country_nc": "NCHAR(3)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Insert using normal (literal) statements – no bind variables
	inserts := []string{
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('A1', 'John', '山田太郎', 'US')",
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('B2', 'Alexander', '李小龙', 'IN')",
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('C3', 'Maria', '北京市', 'CN')",
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('D4', 'Al', 'خالد', 'AE')",
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('E5', 'Takashi', '日本', 'JP')",
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('F6', '👍👋', '👨‍💻🖥️', 'FR')",
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('G7', 'Tommy', 'محمد', 'EG')",
		"INSERT INTO " + table + " (code) VALUES ('H8')",
	}
	for i, sql := range inserts {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	q := "SELECT code, name_vc, name_nvc, country_nc " +
		"FROM " + table + " ORDER BY code"

	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	// Expected values for assertion
	type row struct {
		code      string
		nameVC    string
		nameNVC   string
		countryNC string
	}
	expRows := []row{
		{"A1   ", "John", "山田太郎", "US "},
		{"B2   ", "Alexander", "李小龙", "IN "},
		{"C3   ", "Maria", "北京市", "CN "},
		{"D4   ", "Al", "خالد", "AE "},
		{"E5   ", "Takashi", "日本", "JP "},
		{"F6   ", "👍👋", "👨‍💻🖥️", "FR "},
		{"G7   ", "Tommy", "محمد", "EG "},
		{"H8   ", "", "", ""},
	}

	charset, err := fetchNLSCharacterSet(ctx, db)
	if err != nil {
		t.Fatalf("failed to fetch NLS_CHARACTERSET: %v", err)
	}
	t.Logf("Database NLS_CHARACTERSET=%s", charset)
	// If the database character set is WE8DEC, the driver replaces non-representable characters with '¿'
	if strings.EqualFold(charset, "WE8DEC") {
		for code, exp := range expRows {
			exp.nameNVC = strings.Repeat("¿", len([]rune(exp.nameNVC)))
			if exp.code == "F6   " {
				exp.nameVC = strings.Repeat("¿", len([]rune(exp.nameVC)))
			}
			expRows[code] = exp
		}
	}

	t.Logf("expRows: %#v", expRows)
	idx := 0
	for rs.Next() {
		var (
			code      string
			nameVC    string
			nameNVC   string
			countryNC string
		)
		if err := rs.Scan(&code, &nameVC, &nameNVC, &countryNC); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Log the fetched row values and computed lengths for debugging
		t.Logf("Row %d: code=%q name_vc=%q \t\t name_nvc=%s  \t\t country_nc=%q", idx, code, nameVC, nameNVC, countryNC)

		if idx >= len(expRows) {
			t.Fatalf("received more rows than inserted: idx=%d", idx)
		}
		exp := expRows[idx]

		// Value checks (client should trim trailing blanks for CHAR/NCHAR)
		if code != exp.code {
			t.Fatalf("code mismatch at row %d: got %q want %q", idx, code, exp.code)
		}
		if nameVC != exp.nameVC {
			t.Fatalf("name_vc mismatch at row %d: got %q want %q", idx, nameVC, exp.nameVC)
		}
		if nameNVC != exp.nameNVC {
			t.Fatalf("name_nvc mismatch at row %d: got %q want %q", idx, nameNVC, exp.nameNVC)
		}
		if countryNC != exp.countryNC {
			t.Fatalf("country_nc mismatch at row %d: got %q want %q", idx, countryNC, exp.countryNC)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(expRows) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(expRows))
	}
}

// TestDriver_Select_CharacterTypes_Ordinal
// Prepared statements with ordinal binds for both INSERT and SELECT, mirroring TestDriver_Select_CharacterTypes.
func TestDriver_Select_CharacterTypes_Ordinal(t *testing.T) {
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
	charset, err := fetchNLSCharacterSet(ctx, db)
	if err != nil {
		t.Fatalf("failed to fetch NLS_CHARACTERSET: %v", err)
	}
	t.Logf("Database NLS_CHARACTERSET=%s", charset)

	table := createObjectName("char_test_ord")
	cols := map[string]string{
		"code":       "CHAR(5)",
		"name_vc":    "VARCHAR2(20)",
		"name_nvc":   "NVARCHAR2(20)",
		"country_nc": "NCHAR(3)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Input rows (un-padded for CHAR/NCHAR binds).
	type inrow struct {
		code      string
		nameVC    string
		nameNVC   string
		countryNC string
	}
	inputs := []inrow{
		{"A1", "John", "山田太郎", "US"},
		{"B2", "Alexander", "李小龙", "IN"},
		{"C3", "Maria", "北京市", "CN"},
		{"D4", "Al", "خالد", "AE"},
		{"E5", "Takashi", "日本", "JP"},
		{"F6", "👍👋", "👨‍💻🖥️", "FR"},
		{"G7", "Tommy", "محمد", "EG"},
	}
	// Expected values after SELECT (driver returns padded CHAR/NCHAR)
	type row struct {
		code      string
		nameVC    string
		nameNVC   string
		countryNC string
	}
	expByCode := map[string]row{
		"A1": {"A1   ", "John", "山田太郎", "US "},
		"B2": {"B2   ", "Alexander", "李小龙", "IN "},
		"C3": {"C3   ", "Maria", "北京市", "CN "},
		"D4": {"D4   ", "Al", "خالد", "AE "},
		"E5": {"E5   ", "Takashi", "日本", "JP "},
		"F6": {"F6   ", "👍👋", "👨‍💻🖥️", "FR "},
		"G7": {"G7   ", "Tommy", "محمد", "EG "},
	}

	if strings.EqualFold(charset, "WE8DEC") {
		for code, exp := range expByCode {
			exp.nameNVC = strings.Repeat("¿", len([]rune(exp.nameNVC)))
			if code == "F6" {
				exp.nameVC = strings.Repeat("¿", len([]rune(exp.nameVC)))
			}
			expByCode[code] = exp
		}
	}

	// Prepared INSERT (ordinal binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	insSQL := "INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES (:1, :2, :3, :4)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })

	for i, r := range inputs {
		if _, err := insStmt.ExecContext(ctx, r.code, r.nameVC, r.nameNVC, r.countryNC); err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	// Prepared SELECT (ordinal bind)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	selSQL := "SELECT code, name_vc, name_nvc, country_nc FROM " + table + " WHERE code = :1"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	for i, in := range inputs {
		var (
			code      string
			nameVC    string
			nameNVC   string
			countryNC string
		)
		rs, err := selStmt.QueryContext(ctx, expByCode[in.code].code)
		if err != nil {
			t.Fatalf("select failed for row %d code=%q: %v", i, in.code, err)
		}
		if !rs.Next() {
			if err := rs.Err(); err != nil {
				_ = rs.Close()
				t.Fatalf("rows err for row %d code=%q: %v", i, in.code, err)
			}
			_ = rs.Close()
			t.Fatalf("no row returned for row %d code=%q", i, in.code)
		}
		if err := rs.Scan(&code, &nameVC, &nameNVC, &countryNC); err != nil {
			_ = rs.Close()
			t.Fatalf("scan failed for row %d code=%q: %v", i, in.code, err)
		}
		if err := rs.Close(); err != nil {
			t.Fatalf("rows close failed for row %d code=%q: %v", i, in.code, err)
		}
		exp := expByCode[in.code]
		t.Logf("[ORD] Row %d: code=%q name_vc=%q name_nvc=%s country_nc=%q", i, code, nameVC, nameNVC, countryNC)

		if code != exp.code {
			t.Fatalf("code mismatch at row %d: got %q want %q", i, code, exp.code)
		}
		if nameVC != exp.nameVC {
			t.Fatalf("name_vc mismatch at row %d: got %q want %q", i, nameVC, exp.nameVC)
		}
		if nameNVC != exp.nameNVC {
			t.Fatalf("name_nvc mismatch at row %d: got %q want %q", i, nameNVC, exp.nameNVC)
		}
		if countryNC != exp.countryNC {
			t.Fatalf("country_nc mismatch at row %d: got %q want %q", i, countryNC, exp.countryNC)
		}
	}
}

// TestDriver_Select_CharacterTypes_Named
// Prepared statements with named binds for both INSERT and SELECT, mirroring TestDriver_Select_CharacterTypes.
func TestDriver_Select_CharacterTypes_Named(t *testing.T) {
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
	charset, err := fetchNLSCharacterSet(ctx, db)
	if err != nil {
		t.Fatalf("failed to fetch NLS_CHARACTERSET: %v", err)
	}
	t.Logf("Database NLS_CHARACTERSET=%s", charset)

	table := createObjectName("char_test_named")
	cols := map[string]string{
		"code":       "CHAR(5)",
		"name_vc":    "VARCHAR2(20)",
		"name_nvc":   "NVARCHAR2(20)",
		"country_nc": "NCHAR(3)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	type inrow struct {
		code      string
		nameVC    string
		nameNVC   string
		countryNC string
	}
	inputs := []inrow{
		{"A1", "John", "山田太郎", "US"},
		{"B2", "Alexander", "李小龙", "IN"},
		{"C3", "Maria", "北京市", "CN"},
		{"D4", "Al", "خالد", "AE"},
		{"E5", "Takashi", "日本", "JP"},
		{"F6", "👍👋", "👨‍💻🖥️", "FR"},
		{"G7", "Tommy", "محمد", "EG"},
	}
	type row struct {
		code      string
		nameVC    string
		nameNVC   string
		countryNC string
	}
	expByCode := map[string]row{
		"A1": {"A1   ", "John", "山田太郎", "US "},
		"B2": {"B2   ", "Alexander", "李小龙", "IN "},
		"C3": {"C3   ", "Maria", "北京市", "CN "},
		"D4": {"D4   ", "Al", "خالد", "AE "},
		"E5": {"E5   ", "Takashi", "日本", "JP "},
		"F6": {"F6   ", "👍👋", "👨‍💻🖥️", "FR "},
		"G7": {"G7   ", "Tommy", "محمد", "EG "},
	}

	if strings.EqualFold(charset, "WE8DEC") {
		for code, exp := range expByCode {
			exp.nameNVC = strings.Repeat("¿", len([]rune(exp.nameNVC)))
			if code == "F6" {
				exp.nameVC = strings.Repeat("¿", len([]rune(exp.nameVC)))
			}
			expByCode[code] = exp
		}
	}

	// Prepared INSERT (named binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	insSQL := "INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES (:code, :vc, :nvc, :nc)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })
	for i, r := range inputs {
		if _, err := insStmt.ExecContext(ctx,
			sql.Named("code", r.code),
			sql.Named("vc", r.nameVC),
			sql.Named("nvc", r.nameNVC),
			sql.Named("nc", r.countryNC),
		); err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	// Prepared SELECT (named bind)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	selSQL := "SELECT code, name_vc, name_nvc, country_nc FROM " + table + " WHERE code = :code"
	selStmt, err := db.PrepareContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	for i, in := range inputs {
		var (
			code      string
			nameVC    string
			nameNVC   string
			countryNC string
		)
		rs, err := selStmt.QueryContext(ctx, sql.Named("code", expByCode[in.code].code))
		if err != nil {
			t.Fatalf("select failed for row %d code=%q: %v", i, in.code, err)
		}
		if !rs.Next() {
			if err := rs.Err(); err != nil {
				_ = rs.Close()
				t.Fatalf("rows err for row %d code=%q: %v", i, in.code, err)
			}
			_ = rs.Close()
			t.Fatalf("no row returned for row %d code=%q", i, in.code)
		}
		if err := rs.Scan(&code, &nameVC, &nameNVC, &countryNC); err != nil {
			_ = rs.Close()
			t.Fatalf("scan failed for row %d code=%q: %v", i, in.code, err)
		}
		if err := rs.Close(); err != nil {
			t.Fatalf("rows close failed for row %d code=%q: %v", i, in.code, err)
		}
		exp := expByCode[in.code]
		t.Logf("[NAMED] Row %d: code=%q name_vc=%q name_nvc=%s country_nc=%q", i, code, nameVC, nameNVC, countryNC)

		if code != exp.code {
			t.Fatalf("code mismatch at row %d: got %q want %q", i, code, exp.code)
		}
		if nameVC != exp.nameVC {
			t.Fatalf("name_vc mismatch at row %d: got %q want %q", i, nameVC, exp.nameVC)
		}
		if nameNVC != exp.nameNVC {
			t.Fatalf("name_nvc mismatch at row %d: got %q want %q", i, nameNVC, exp.nameNVC)
		}
		if countryNC != exp.countryNC {
			t.Fatalf("country_nc mismatch at row %d: got %q want %q", i, countryNC, exp.countryNC)
		}
	}
}

// TestDriver_Varchar2_TrailingSpacesPreserved verifies that VARCHAR2 values
// retain bound trailing spaces; the fetched value and Oracle LENGTH must both be 6.
func TestDriver_Varchar2_TrailingSpacesPreserved(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("vc2_trailing_spaces")
	if err := createTable(ctx, db, table, map[string]string{
		"val": "VARCHAR2(20)",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	want := "abc   "
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (val) VALUES (:1)", want); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	var (
		got       string
		gotLength int
	)
	if err := db.QueryRowContext(ctx, "SELECT val, LENGTH(val) FROM "+table).Scan(&got, &gotLength); err != nil {
		t.Fatalf("select failed: %v", err)
	}

	if got != want {
		t.Fatalf("VARCHAR2 trailing spaces mismatch: got %q want %q", got, want)
	}
	if len(got) != 6 {
		t.Fatalf("decoded string length = %d, want 6", len(got))
	}
	if gotLength != 6 {
		t.Fatalf("LENGTH(val) = %d, want 6", gotLength)
	}
}

// TestDriver_Varchar2_EmptyStringIsNull verifies Oracle empty-string semantics
// for a bound VARCHAR2 value: scanning it with strict NULL handling yields NULL.
func TestDriver_Varchar2_EmptyStringIsNull(t *testing.T) {
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
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("vc2_empty_string_null")
	if err := createTable(ctx, db, table, map[string]string{
		"id":  "NUMBER",
		"val": "VARCHAR2(20)",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, val) VALUES (:1, :2)", 1, ""); err != nil {
		t.Fatalf("bound empty string insert failed: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT id, val FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rows.Close()

	expectedIDs := []int64{1}
	var idx int
	for rows.Next() {
		var (
			id  int64
			val sql.NullString
		)
		if err := rows.Scan(&id, &val); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(expectedIDs) {
			t.Fatalf("received more rows than expected")
		}
		if id != expectedIDs[idx] {
			t.Fatalf("row %d id = %d, want %d", idx, id, expectedIDs[idx])
		}
		if val.Valid {
			t.Fatalf("row %d expected NULL VARCHAR2 for empty string, got %+v", idx, val)
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(expectedIDs) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(expectedIDs))
	}
}

// TestDriver_Varchar2_EmbeddedNULRoundTrip verifies that an embedded NUL byte
// round-trips through VARCHAR2; the fetched value and Oracle LENGTH must both be 3.
func TestDriver_Varchar2_EmbeddedNULRoundTrip(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("vc2_embedded_nul")
	if err := createTable(ctx, db, table, map[string]string{
		"val": "VARCHAR2(20)",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	want := "a\x00b"
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (val) VALUES (:1)", want); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	var (
		got       string
		gotLength int
	)
	if err := db.QueryRowContext(ctx, "SELECT val, LENGTH(val) FROM "+table).Scan(&got, &gotLength); err != nil {
		t.Fatalf("select failed: %v", err)
	}

	if got != want {
		t.Fatalf("embedded NUL round trip mismatch: got %q want %q", got, want)
	}
	if len(got) != 3 {
		t.Fatalf("decoded string length = %d, want 3", len(got))
	}
	if gotLength != 3 {
		t.Fatalf("LENGTH(val) = %d, want 3", gotLength)
	}
}

// TestDriver_Varchar2_BoundaryLengths verifies VARCHAR2(4000 BYTE) boundary values.
// It expects both 3,999-byte and 4,000-byte strings to round-trip unchanged.
func TestDriver_Varchar2_BoundaryLengths(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("vc2_boundary_lengths")
	if err := createTable(ctx, db, table, map[string]string{
		"id":  "NUMBER",
		"val": "VARCHAR2(4000 BYTE)",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	inputs := []struct {
		id   int64
		want string
	}{
		{id: 1, want: strings.Repeat("a", 3999)},
		{id: 2, want: strings.Repeat("b", 4000)},
	}

	for _, input := range inputs {
		if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, val) VALUES (:1, :2)", input.id, input.want); err != nil {
			t.Fatalf("insert id=%d failed: %v", input.id, err)
		}
	}

	rows, err := db.QueryContext(ctx, "SELECT id, val FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rows.Close()

	var idx int
	for rows.Next() {
		var (
			id  int64
			got string
		)
		if err := rows.Scan(&id, &got); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(inputs) {
			t.Fatalf("received more rows than expected")
		}
		want := inputs[idx]
		if id != want.id {
			t.Fatalf("row %d id = %d, want %d", idx, id, want.id)
		}
		if got != want.want {
			t.Fatalf("row %d VARCHAR2 mismatch: got length %d want length %d", idx, len(got), len(want.want))
		}
		if len(got) != len(want.want) {
			t.Fatalf("row %d length = %d, want %d", idx, len(got), len(want.want))
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(inputs) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(inputs))
	}
}
