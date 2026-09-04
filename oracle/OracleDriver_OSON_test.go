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

	ojson "github.com/oracle/go-oracledb/v26/oracle/json"
)

// TestDriver_OSON_ScalarDocuments inserts scalar-root JSON documents using JSONValue.
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

// TestDriver_OSON_NestedObject inserts a representative object with nested
// maps, arrays, nulls, booleans, Unicode strings, and numbers.
func TestDriver_OSON_NestedObject(t *testing.T) {
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
		{name: "object", input: value, want: value},
	})
}

// TestDriver_OSON_NestedArray inserts an array-root document with
// traversal after the database stores and returns the JSON document.
func TestDriver_OSON_NestedArray(t *testing.T) {
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
		{name: "array", input: value, want: value},
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
		{name: "document", input: value, want: value},
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
		{name: "long-key", input: value, want: value},
	})
}

// TestDriver_OSON_JSONWrappers verifies the oracle/json wrappers over
// OSON values produced by the database after normal bind and fetch flows.
func TestDriver_OSON_JSONWrappers(t *testing.T) {
	table := createObjectName("t_oson_public_api")
	db, ctx := setupOSONFunctionalTable(t, table)
	insert := func(id int64, value any) {
		t.Helper()
		if _, err := db.ExecContext(ctx,
			"INSERT INTO "+table+" (id, jdoc) VALUES (:id, :jdoc)",
			sql.Named("id", id),
			sql.Named("jdoc", value),
		); err != nil {
			t.Fatalf("insert %d failed: %v", id, err)
		}
	}
	fetch := func(id int64) ojson.JSON {
		t.Helper()
		var value ojson.JSON
		if err := db.QueryRowContext(ctx,
			"SELECT jdoc FROM "+table+" WHERE id = :id",
			sql.Named("id", id),
		).Scan(&value); err != nil {
			t.Fatalf("select %d failed: %v", id, err)
		}
		return value
	}

	wantObject := map[string]any{
		"id":     ojson.Number("42"),
		"name":   "Mona",
		"active": true,
		"items":  []any{"go", ojson.Number("2"), map[string]any{"enabled": true}},
	}
	insert(1, ojson.JSONValue{Data: wantObject})
	objectJSON := fetch(1)
	if kind, err := objectJSON.Kind(); err != nil || kind != ojson.JSONObjectKind {
		t.Fatalf("object JSON.Kind() = (%v, %v), want (%v, nil)", kind, err, ojson.JSONObjectKind)
	}
	assertOSONDocument(t, objectJSON, wantObject)
	if text := objectJSON.String(); text == "" {
		t.Fatal("JSON.String() returned empty text")
	}
	if text, err := objectJSON.StringWithOption(ojson.JSONOptNumberAsString); err != nil || text == "" {
		t.Fatalf("JSON.StringWithOption() = (%q, %v), want non-empty text and nil error", text, err)
	}

	object, err := objectJSON.GetJSONObject(ojson.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("GetJSONObject() failed: %v", err)
	}
	if got, want := object.Len(), len(wantObject); got != want {
		t.Fatalf("JSONObject.Len() = %d, want %d", got, want)
	}
	if got := object.Keys(); len(got) != len(wantObject) {
		t.Fatalf("JSONObject.Keys() = %v, want %d keys", got, len(wantObject))
	}
	if !object.Has("items") || object.Has("missing") {
		t.Fatalf("JSONObject.Has() returned inconsistent membership")
	}
	if _, ok := object.Get("missing"); ok {
		t.Fatal(`JSONObject.Get("missing") = true, want false`)
	}
	if got, err := object.GetValue(); err != nil || !reflect.DeepEqual(got, wantObject) {
		t.Fatalf("JSONObject.GetValue() = (%#v, %v), want (%#v, nil)", got, err, wantObject)
	}
	if text := object.String(); text == "" {
		t.Fatal("JSONObject.String() returned empty text")
	}

	itemsJSON, ok := object.Get("items")
	if !ok {
		t.Fatal(`JSONObject.Get("items") = false, want true`)
	}
	items, err := itemsJSON.GetJSONArray(ojson.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("items.GetJSONArray() failed: %v", err)
	}
	if got, want := items.Len(), 3; got != want {
		t.Fatalf("nested JSONArray.Len() = %d, want %d", got, want)
	}
	itemJSON, err := items.Get(1)
	if err != nil {
		t.Fatalf("JSONArray.Get(1) failed: %v", err)
	}
	item, err := itemJSON.GetJSONScalar(ojson.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("GetJSONScalar() failed: %v", err)
	}
	if got, err := item.GetValue(); err != nil || got != ojson.Number("2") {
		t.Fatalf("JSONScalar.GetValue() = (%#v, %v), want (%q, nil)", got, err, ojson.Number("2"))
	}
	if _, err := items.Get(-1); err == nil {
		t.Fatal("JSONArray.Get(-1) error = nil, want out-of-range error")
	}

	wantArray := []any{true, "entry", ojson.Number("3.5")}
	insert(2, ojson.JSONValue{Data: wantArray})
	arrayJSON := fetch(2)
	if kind, err := arrayJSON.Kind(); err != nil || kind != ojson.JSONArrayKind {
		t.Fatalf("array JSON.Kind() = (%v, %v), want (%v, nil)", kind, err, ojson.JSONArrayKind)
	}
	array, err := arrayJSON.GetJSONArray(ojson.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("root GetJSONArray() failed: %v", err)
	}
	if got, err := array.GetValue(); err != nil || !reflect.DeepEqual(got, wantArray) {
		t.Fatalf("JSONArray.GetValue() = (%#v, %v), want (%#v, nil)", got, err, wantArray)
	}
	if text := array.String(); text == "" {
		t.Fatal("JSONArray.String() returned empty text")
	}
	if _, err := arrayJSON.GetJSONObject(ojson.JSONOptNumberAsString); err == nil {
		t.Fatal("array GetJSONObject() error = nil, want access error")
	}

	insert(3, ojson.JSONValue{Data: true})
	scalarJSON := fetch(3)
	if kind, err := scalarJSON.Kind(); err != nil || kind != ojson.JSONScalarKind {
		t.Fatalf("scalar JSON.Kind() = (%v, %v), want (%v, nil)", kind, err, ojson.JSONScalarKind)
	}
	scalar, err := scalarJSON.GetJSONScalar(ojson.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("root GetJSONScalar() failed: %v", err)
	}
	if got, err := scalar.GetValue(); err != nil || got != true {
		t.Fatalf("root JSONScalar.GetValue() = (%#v, %v), want (true, nil)", got, err)
	}
	if _, err := scalarJSON.GetJSONArray(ojson.JSONOptNumberAsString); err == nil {
		t.Fatal("scalar GetJSONArray() error = nil, want access error")
	}

	insert(4, ojson.JSONString{Data: `{"source":"json-string"}`})
	assertOSONDocument(t, fetch(4), map[string]any{"source": "json-string"})

	// Bind the fetched public JSON value again; this exercises JSON.Value().
	insert(5, objectJSON)
	assertOSONDocument(t, fetch(5), wantObject)
}

// TestDriver_OSON_JSONWrapperErrors verifies wrapper behavior
// that cannot arise from a successful database scan, such as zero values and
// invalid client input.
func TestDriver_OSON_JSONWrapperErrors(t *testing.T) {
	assertError := func(operation string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s error = nil, want JSON API error", operation)
		}
	}

	var value ojson.JSON
	_, err := value.Value()
	assertError("JSON.Value", err)
	_, err = value.Kind()
	assertError("JSON.Kind", err)
	_, err = value.GetJSONObject(ojson.JSONOptDefault)
	assertError("JSON.GetJSONObject", err)
	_, err = value.GetJSONArray(ojson.JSONOptDefault)
	assertError("JSON.GetJSONArray", err)
	_, err = value.GetJSONScalar(ojson.JSONOptDefault)
	assertError("JSON.GetJSONScalar", err)
	_, err = value.GetValue(ojson.JSONOptDefault)
	assertError("JSON.GetValue", err)
	if got := value.String(); got != "" {
		t.Fatalf("zero JSON.String() = %q, want empty string", got)
	}
	_, err = value.StringWithOption(ojson.JSONOptDefault)
	assertError("JSON.StringWithOption", err)

	var nilValue *ojson.JSON
	assertError("nil JSON.Scan", nilValue.Scan([]byte{0xFF, 0x4A, 0x5A, 0x01}))
	assertError("JSON.Scan text", value.Scan([]byte(`{"not":"oson"}`)))
	assertError("JSON.Scan unsupported source", value.Scan("not bytes"))
	_, err = (ojson.JSONValue{Data: struct{}{}}).Value()
	assertError("JSONValue.Value unsupported input", err)

	var object ojson.JSONObject
	if got := object.Len(); got != -1 {
		t.Fatalf("zero JSONObject.Len() = %d, want -1", got)
	}
	if got := object.Keys(); got != nil {
		t.Fatalf("zero JSONObject.Keys() = %v, want nil", got)
	}
	if object.Has("missing") {
		t.Fatal("zero JSONObject.Has() = true, want false")
	}
	if _, ok := object.Get("missing"); ok {
		t.Fatal("zero JSONObject.Get() = true, want false")
	}
	_, err = object.GetValue()
	assertError("JSONObject.GetValue", err)
	if got := object.String(); got != "" {
		t.Fatalf("zero JSONObject.String() = %q, want empty string", got)
	}

	var array ojson.JSONArray
	if got := array.Len(); got != -1 {
		t.Fatalf("zero JSONArray.Len() = %d, want -1", got)
	}
	_, err = array.GetValue()
	assertError("JSONArray.GetValue", err)
	_, err = array.Get(0)
	assertError("JSONArray.Get", err)
	if got := array.String(); got != "" {
		t.Fatalf("zero JSONArray.String() = %q, want empty string", got)
	}

	var scalar ojson.JSONScalar
	_, err = scalar.GetValue()
	assertError("JSONScalar.GetValue", err)
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
	if TestingConfig.DatabaseVersion.Major < 21 {
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
