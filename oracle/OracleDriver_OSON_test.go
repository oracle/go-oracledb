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
	"reflect"
	"strconv"
	"strings"
	"testing"

	ojson "github.com/oracle/go-driver/oracle/json"
)

// TestDriver_OSON_ScalarDocuments inserts scalar-root JSON documents using
// JSONValue. JSONValue creates the client-side OSON payload for the bind, and
// the selected JSON column is scanned back through oracle/json.JSON.
func TestDriver_OSON_ScalarDocuments(t *testing.T) {
	tests := []osonFunctionalCase{
		{name: "null", input: nil, want: nil},
		{name: "string", input: "oracle-json", want: "oracle-json"},
		{name: "true", input: true, want: true},
		{name: "false", input: false, want: false},
		{name: "signed integer", input: int64(-9007199254740993), want: ojson.Number("-9007199254740993")},
		{name: "unsigned integer", input: uint64(9007199254740993), want: ojson.Number("9007199254740993")},
		{name: "float", input: float64(-12345.625), want: ojson.Number("-12345.625")},
		{name: "explicit number", input: ojson.Number("12345678901234567890.125"), want: ojson.Number("12345678901234567890.125")},
	}

	runOSONFunctionalCases(t, "t_oson_scalar", tests)
}

// TestDriver_OSON_RichObject inserts a representative object with nested
// maps, arrays, nulls, booleans, Unicode strings, and numbers.
func TestDriver_OSON_RichObject(t *testing.T) {
	value := map[string]any{
		"id":      ojson.Number("42"),
		"name":    "Mona",
		"active":  true,
		"balance": ojson.Number("-98765.125"),
		"tags":    []any{"go", "oracle", nil, false},
		"profile": map[string]any{
			"city":       "Casablanca",
			"unicode":    "日本語 العربية é",
			"reputation": ojson.Number("9007199254740993"),
			"flags": map[string]any{
				"staff": false,
				"beta":  true,
			},
		},
	}

	runOSONFunctionalCases(t, "t_oson_object", []osonFunctionalCase{
		{name: "rich-object", input: value, want: value},
	})
}

// TestDriver_OSON_RichNestedArray inserts an array-root document with
// traversal after the database stores and returns the JSON document.
func TestDriver_OSON_RichNestedArray(t *testing.T) {
	value := []any{
		nil,
		true,
		ojson.Number("7"),
		"root-string",
		[]any{
			ojson.Number("-1"),
			false,
			[]any{"deep", ojson.Number("2.5")},
		},
		map[string]any{
			"type": "event",
			"payload": []any{
				map[string]any{"k": "v", "n": ojson.Number("10")},
				nil,
			},
		},
	}

	runOSONFunctionalCases(t, "t_oson_array", []osonFunctionalCase{
		{name: "rich-nested-array", input: value, want: value},
	})
}

// TestDriver_OSON_LargeDocument tests inserting and fetching large JSON documents.
func TestDriver_OSON_LargeDocument(t *testing.T) {
	const rowCount = 1024

	rows := make([]any, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		rowID := strconv.Itoa(i)
		rows = append(rows, map[string]any{
			"id":      ojson.Number(rowID),
			"name":    "row-" + rowID,
			"payload": strings.Repeat(string(rune('a'+(i%26))), 2048),
			"active":  i%2 == 0,
		})
	}

	value := map[string]any{
		"kind": "large-oson-document",
		"rows": rows,
	}

	runOSONFunctionalCases(t, "t_oson_large", []osonFunctionalCase{
		{name: "large-document", input: value, want: value},
	})
}

// TestDriver_OSON_LongUTF8DictionaryKey inserts an object with a field
// name longer than 255 UTF-8 bytes.
func TestDriver_OSON_LongUTF8DictionaryKey(t *testing.T) {
	longKey := strings.Repeat("é", 200) //  > 255 UTF-8 bytes.
	value := map[string]any{
		"short": "primary-dictionary-value",
		longKey: map[string]any{
			"value": "secondary-dictionary-value",
			"index": ojson.Number("1"),
		},
	}

	runOSONFunctionalCases(t, "t_oson_long_key", []osonFunctionalCase{
		{name: "long-utf8-dictionary-key", input: value, want: value},
	})
}

// osonFunctionalCase defines an OSON document test case.
type osonFunctionalCase struct {
	name  string
	input any
	want  any
}

// runOSONFunctionalCases runs OSON document test cases.
func runOSONFunctionalCases(t *testing.T, table string, cases []osonFunctionalCase) {
	t.Helper()

	db, ctx := setupOSONFunctionalTable(t, table)
	insSQL := "INSERT INTO " + table + " (id, jdoc) VALUES (:id, :jdoc)"
	selSQL := "SELECT jdoc FROM " + table + " WHERE id = :id"

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := int64(i + 1)
			if _, err := db.ExecContext(ctx,
				insSQL,
				sql.Named("id", id),
				sql.Named("jdoc", ojson.JSONValue{Data: tc.input}),
			); err != nil {
				t.Fatalf("insert JSONValue %s failed: %v", tc.name, err)
			}

			var gotJSON ojson.JSON
			if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", id)).Scan(&gotJSON); err != nil {
				t.Fatalf("select/scan JSONValue %s failed: %v", tc.name, err)
			}
			assertOSONDocument(t, gotJSON, tc.want)
		})
	}
}

// setupOSONFunctionalTable creates the JSON table for an OSON test.
func setupOSONFunctionalTable(t *testing.T, table string) (*sql.DB, context.Context) {
	t.Helper()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion < 21 {
		t.Skip("JSON Type is not supported for DB < 21")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"jdoc": "JSON",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping OSON JSONValue test (create failed): %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	return db, ctx
}

// assertOSONDocument verifies the decoded JSON value against the wanted value.
func assertOSONDocument(t *testing.T, gotJSON ojson.JSON, want any) {
	t.Helper()

	got, err := gotJSON.GetValue(ojson.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("JSON.GetValue(JSONOptNumberAsString) failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		text, textErr := gotJSON.StringWithOption(ojson.JSONOptNumberAsString)
		if textErr != nil {
			t.Fatalf("JSON mismatch and StringWithOption failed: %v", textErr)
		}
		t.Fatalf("OSON document mismatch:\n got value:  %#v\nwant value: %#v\n got text:  %s", got, want, text)
	}
}
