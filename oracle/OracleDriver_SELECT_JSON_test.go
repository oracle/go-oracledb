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
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// TestDriver_Table_Select_JSON
// What it does: Creates a table with a JSON column, inserts JSON payloads of different sizes,
// then selects them back.
// Expectation: Selecting the JSON column and scanning into string returns the exact JSON text inserted.
// Notes:
//   - JSON data type is supported in Oracle 21c+ (and enhanced in 23c). If unsupported in the test DB,
//     CREATE TABLE will fail and this test will Skip.
//   - Includes coverage for small JSON, 0MB JSON, 20MB JSON, and 30MB JSON.
func TestDriver_Table_Select_JSON(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("JSON Type is not supported for DB < 21")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_select_json")

	// JSON is a native type on 21c+ (and 23c). On older DBs, this should error and we'll Skip.
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"jdoc": "JSON",
	}

	// Drop if exists (ignore error) to make reruns stable in dev environments.
	_ = dropTable(ctx, db, table)

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping JSON select test (create failed): %v", err)
		return
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	normalize := func(s string) string {
		// Minimal normalization: remove spaces, tabs, newlines, carriage returns.
		out := make([]rune, 0, len(s))
		for _, r := range s {
			switch r {
			case ' ', '\t', '\n', '\r':
				continue
			default:
				out = append(out, r)
			}
		}
		return string(out)
	}

	buildSizedJSON := func(sizeBytes int, ch string) string {
		if sizeBytes <= 0 {
			return `{}`
		}
		baseLen := len(`{"payload":""}`)
		if sizeBytes <= baseLen {
			return `{}`
		}
		payloadLen := sizeBytes - baseLen
		return `{"payload":"` + strings.Repeat(ch, payloadLen) + `"}`
	}

	testCases := []struct {
		id      int64
		name    string
		jsonIn  string
		useHash bool
	}{
		{
			id:     1,
			name:   "small-json",
			jsonIn: `{"id":1,"name":"Alice","active":true,"skills":["go","java","oracle"]}`,
		},
		{
			id:     2,
			name:   "zero-mb-json",
			jsonIn: buildSizedJSON(0, "z"),
		},
		{
			id:      3,
			name:    "20mb-json",
			jsonIn:  buildSizedJSON(20*1024*1024, "m"),
			useHash: true,
		},
		{
			id:      4,
			name:    "30mb-json",
			jsonIn:  buildSizedJSON(30*1024*1024, "t"),
			useHash: true,
		},
	}

	insSQL := "INSERT INTO " + table + " (id, jdoc) VALUES (:id, :jdoc)"

	for _, tc := range testCases {
		t.Run("insert-"+tc.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx,
				insSQL,
				sql.Named("id", tc.id),
				sql.Named("jdoc", tc.jsonIn),
			); err != nil {
				t.Fatalf("insert %s failed: %v", tc.name, err)
			}
		})
	}

	selSQL := "SELECT JDOC FROM " + table + " WHERE id = :id"

	for _, tc := range testCases {
		t.Run("select-"+tc.name, func(t *testing.T) {
			var jsonOut string
			if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", tc.id)).Scan(&jsonOut); err != nil {
				t.Fatalf("select/scan %s failed: %v", tc.name, err)
			}

			if tc.useHash {
				gotHash := sha256.Sum256([]byte(normalize(jsonOut)))
				wantHash := sha256.Sum256([]byte(normalize(tc.jsonIn)))
				if gotHash != wantHash {
					t.Fatalf("%s mismatch (sha256 differs):\n got:  %x\nwant: %x", tc.name, gotHash, wantHash)
				}
				return
			}

			if normalize(jsonOut) != normalize(tc.jsonIn) {
				t.Fatalf("%s mismatch:\n got: %s\nwant: %s", tc.name, jsonOut, tc.jsonIn)
			}

			if tc.name == "zero-mb-json" && normalize(jsonOut) != "{}" {
				t.Fatalf("zero-mb-json expected empty object, got: %s", fmt.Sprintf("%q", jsonOut))
			}
		})
	}
}

// TestDriver_Table_Select_NullJSON
// What it does: Creates a table with a JSON column, inserts a NULL JSON value, then selects it back.
// Expectation: Selecting the JSON column and scanning into sql.NullString reports an invalid value.
// Notes:
//   - JSON data type is supported in Oracle 21c+ (and enhanced in 23c). If unsupported in the test DB,
//     CREATE TABLE will fail and this test will Skip.
func TestDriver_Table_Select_NullJSON(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("Boolean Type is not supported for DB < 23")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_select_json_null")

	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"jdoc": "JSON",
	}

	_ = dropTable(ctx, db, table)

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping NULL JSON select test (create failed): %v", err)
		return
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	insSQL := "INSERT INTO " + table + " (id, jdoc) VALUES (:id, NULL)"
	if _, err := db.ExecContext(ctx, insSQL, sql.Named("id", int64(1))); err != nil {
		t.Fatalf("insert null json failed: %v", err)
	}

	selSQL := "SELECT JDOC FROM " + table + " WHERE id = :id"

	var jsonOut sql.NullString
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(1))).Scan(&jsonOut); err != nil {
		t.Fatalf("select/scan null json failed: %v", err)
	}
	if jsonOut.Valid {
		t.Fatalf("expected NULL json, got valid string: %q", jsonOut.String)
	}
}

// TestDriver_Table_Insert_Select_JSON_MultiRows
// What it does: Inserts 15 rows containing JSON data, then reads them back using SELECT *.
// Expectation: Every row selected from the table matches exactly what was inserted.
// Notes:
//   - Uses JSON normalization (whitespace removal) for text comparison.
//   - Query uses `SELECT * FROM <table>`.
func TestDriver_Table_Insert_Select_JSON_MultiRows(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("JSON Type is not supported for DB < 21")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_ins_sel_json_15")
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"jdoc": "JSON",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping JSON multi-row insert/select test (create failed): %v", err)
		return
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	// Define JSON normalization helper for robust text comparison.
	normalize := func(s string) string {
		out := make([]rune, 0, len(s))
		for _, r := range s {
			switch r {
			case ' ', '\t', '\n', '\r':
				continue
			default:
				out = append(out, r)
			}
		}
		return string(out)
	}

	// Insert 15 rows of JSON data and store expected values by ID.
	expected := make(map[int64]string, 15)
	insSQL := "INSERT INTO " + table + " (id, jdoc) VALUES (:id, :jdoc)"
	for i := 1; i <= 15; i++ {
		id := int64(i)
		payload := strings.Repeat(string(rune('a'+(i%26))), i+5) // 6..20
		jsonVal := fmt.Sprintf(`{"id":%d,"name":"row-%d","payload":"%s","active":true}`, id, id, payload)
		expected[id] = jsonVal

		if _, err := db.ExecContext(ctx, insSQL,
			sql.Named("id", id),
			sql.Named("jdoc", jsonVal),
		); err != nil {
			t.Fatalf("insert failed for id=%d: %v", id, err)
		}
	}

	// Read all rows using SELECT * and validate query setup.
	selSQL := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := db.QueryContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("select rows failed: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })

	colsReturned, err := rows.Columns()
	if err != nil {
		t.Fatalf("failed to read selected columns: %v", err)
	}
	if len(colsReturned) != 2 {
		t.Fatalf("unexpected selected column count: got=%d want=2", len(colsReturned))
	}

	// Iterate over result rows, map columns, and compare returned JSON with expected values.
	seen := make(map[int64]bool, 15)
	for rows.Next() {
		var (
			id   int64
			jdoc string
		)

		scanTargets := make([]any, len(colsReturned))
		for i, c := range colsReturned {
			switch strings.ToUpper(c) {
			case "ID":
				scanTargets[i] = &id
			case "JDOC":
				scanTargets[i] = &jdoc
			default:
				var discard any
				scanTargets[i] = &discard
			}
		}

		if err := rows.Scan(scanTargets...); err != nil {
			t.Fatalf("row scan failed: %v", err)
		}

		exp, ok := expected[id]
		if !ok {
			t.Fatalf("unexpected id returned from select: %d", id)
		}

		if normalize(jdoc) != normalize(exp) {
			t.Fatalf("json mismatch for id=%d:\n got: %s\nwant: %s", id, jdoc, exp)
		}

		seen[id] = true
	}

	// Ensure row iteration completed cleanly and all 15 rows were returned.
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration failed: %v", err)
	}

	if len(seen) != 15 {
		t.Fatalf("row count mismatch: got=%d want=15", len(seen))
	}
}
