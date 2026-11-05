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
** Works (as defined below), to deal in both (a) the Software, and (b) any piece
** of software and/or hardware listed in the lrgrwrks.txt file if one is
** included with the Software (each a "Larger Work" to which the Software is
** contributed by such licensors), without restriction, including without
** limitation the rights to copy, create derivative works of, display, perform,
** and distribute the Software and make, use, sell, offer for sale, import,
** export, have made, and have sold the Software and the Larger Work(s), and to
** sublicense the foregoing rights on either these or other terms.
**
** This license is subject to the following condition: The above copyright
** notice and either this complete permission notice or at a minimum a reference
** to the UPL must be included in all copies or substantial portions of the
** Software.
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
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/oson"
	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
	ojson "github.com/oracle/go-oracledb/v26/oracle/datatype/json"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_OSON_RebindFetchedDocument verifies a document fetched from a
// native JSON column can be bound to another native JSON column without
// materializing it first. This guards the OSON fetch-to-bind regression path.
func TestDriver_OSON_RebindFetchedDocument(t *testing.T) {
	db, ctx, table := setupOSONTable(t)

	want := map[string]any{
		// This value would change if it were first materialized as float64.
		"id":     stdjson.Number("9007199254740993"),
		"values": []any{true, "oracle"},
	}
	source, err := ojson.NewJSON(want)
	if err != nil {
		t.Fatalf("create source JSON failed: %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", source); err != nil {
		t.Fatalf("insert source document failed: %v", err)
	}

	var fetched ojson.JSON
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 1").Scan(&fetched); err != nil {
		t.Fatalf("fetch source document failed: %v", err)
	}
	// Bind the fetched OSON bytes directly; GetValue would test a different,
	// decode-and-re-encode path.
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (2, :1)", fetched); err != nil {
		t.Fatalf("rebind fetched document failed: %v", err)
	}

	var rebound ojson.JSON
	rebound.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 2").Scan(&rebound); err != nil {
		t.Fatalf("fetch rebound document failed: %v", err)
	}
	got, err := rebound.GetValue()
	if err != nil {
		t.Fatalf("materialize rebound document failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rebound document = %#v, want %#v", got, want)
	}
}

// TestDriver_OSON_ContainerTypes verifies native JSON preserves object and
// array root documents across an OSON database round trip.
func TestDriver_OSON_ContainerTypes(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	cases := []struct {
		id   int
		name string
		want any
	}{
		// Root containers follow different OSON node layouts.
		{id: 1, name: "object", want: map[string]any{"items": []any{"oracle"}}},
		{id: 2, name: "array", want: []any{"oracle", true}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			doc, err := ojson.NewJSON(test.want)
			if err != nil {
				t.Fatalf("create JSON document failed: %v", err)
			}
			if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", test.id, doc); err != nil {
				t.Fatalf("insert %s document failed: %v", test.name, err)
			}

			var got ojson.JSON
			if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = :1", test.id).Scan(&got); err != nil {
				t.Fatalf("fetch %s document failed: %v", test.name, err)
			}
			value, err := got.GetValue()
			if err != nil {
				t.Fatalf("materialize %s document failed: %v", test.name, err)
			}
			if !reflect.DeepEqual(value, test.want) {
				t.Fatalf("%s document = %#v, want %#v", test.name, value, test.want)
			}
		})
	}
}

// TestDriver_OSON_Scalar verifies an OSON-native binary scalar survives a
// native JSON database round trip.
func TestDriver_OSON_Scalar(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	// Binary is an OSON-native scalar and cannot be represented by JSON text.
	want := []byte{0x00, 0xff}
	doc, err := ojson.NewJSON(want)
	if err != nil {
		t.Fatalf("create binary JSON scalar failed: %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", doc); err != nil {
		t.Fatalf("insert binary JSON scalar failed: %v", err)
	}

	var got ojson.JSON
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 1").Scan(&got); err != nil {
		t.Fatalf("fetch binary JSON scalar failed: %v", err)
	}
	value, err := got.GetValue()
	if err != nil {
		t.Fatalf("materialize binary JSON scalar failed: %v", err)
	}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("binary JSON scalar = %#v, want %#v", value, want)
	}
	for index, test := range []struct {
		value any
		want  string
	}{
		{math.NaN(), `"Nan"`},
		{math.Inf(1), `"Inf"`},
		{math.Inf(-1), `"-Inf"`},
		{float32(math.NaN()), `"Nan"`},
		{float32(math.Inf(1)), `"Inf"`},
		{float32(math.Inf(-1)), `"-Inf"`},
	} {
		doc, err := ojson.NewJSON(test.value)
		if err != nil {
			t.Fatalf("create nonfinite scalar failed: %v", err)
		}
		id := index + 2
		if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", id, doc); err != nil {
			t.Fatalf("insert nonfinite scalar failed: %v", err)
		}
		var fetched ojson.JSON
		if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = :1", id).Scan(&fetched); err != nil {
			t.Fatalf("fetch nonfinite scalar failed: %v", err)
		}
		for _, mode := range []ojson.NumberMode{ojson.NumberDefault, ojson.NumberAsJSONNumber, ojson.NumberAsFloat64} {
			fetched.SetOptions(ojson.Options{NumberMode: mode})
			value, err := fetched.GetValue()
			if err != nil {
				t.Fatalf("materialize nonfinite scalar failed: %v", err)
			}
			number, ok := value.(float64)
			if !ok {
				t.Fatalf("nonfinite scalar = %T, want float64", value)
			}
			if test.want == `"Nan"` && !math.IsNaN(number) ||
				test.want == `"Inf"` && !math.IsInf(number, 1) ||
				test.want == `"-Inf"` && !math.IsInf(number, -1) {
				t.Fatalf("nonfinite scalar = %v, want %v", number, test.value)
			}
			text, err := stdjson.Marshal(fetched)
			if err != nil || string(text) != test.want {
				t.Fatalf("marshal fetched scalar = (%s, %v), want %s", text, err, test.want)
			}
		}
	}
}

// TestDriver_OSON_TimeTypes verifies OSON preserves the selected temporal
// scalar encoding through a native JSON database round trip.
func TestDriver_OSON_TimeTypes(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	// A non-UTC offset and nanoseconds expose loss of zone or precision.
	value := time.Date(2025, 2, 3, 4, 5, 6, 123456789, time.FixedZone("UTC+2", 2*60*60))
	cases := []struct {
		id   int
		name string
		opts ojson.Options
		want string
	}{
		{id: 1, name: "timestamp", opts: ojson.Options{TimeEncoding: ojson.TimeAsTimestamp}, want: `"2025-02-03T04:05:06.123456789"`},
		{id: 2, name: "timestamp with time zone", opts: ojson.Options{TimeEncoding: ojson.TimeAsTimestampTZ}, want: `"2025-02-03T04:05:06.123456789+02:00"`},
		{id: 3, name: "date", opts: ojson.Options{TimeEncoding: ojson.TimeAsDate}, want: `"2025-02-03T04:05:06"`},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			doc, err := ojson.NewJSONWithOptions(value, test.opts)
			if err != nil {
				t.Fatalf("create temporal JSON scalar failed: %v", err)
			}
			if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", test.id, doc); err != nil {
				t.Fatalf("insert temporal JSON scalar failed: %v", err)
			}

			var got ojson.JSON
			if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = :1", test.id).Scan(&got); err != nil {
				t.Fatalf("fetch temporal JSON scalar failed: %v", err)
			}
			if got.String() != test.want {
				t.Fatalf("temporal JSON scalar = %q, want %q", got.String(), test.want)
			}
		})
	}
}

// TestDriver_OSON_Number verifies an OSON number larger than float64's exact
// integer range is materialized exactly when JSON-number mode is selected.
func TestDriver_OSON_Number(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	// 2^53+1 is the first integer that cannot be represented exactly by float64.
	want := stdjson.Number("9007199254740993")
	doc, err := ojson.NewJSON(want)
	if err != nil {
		t.Fatalf("create numeric JSON scalar failed: %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", doc); err != nil {
		t.Fatalf("insert numeric JSON scalar failed: %v", err)
	}

	var got ojson.JSON
	got.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 1").Scan(&got); err != nil {
		t.Fatalf("fetch numeric JSON scalar failed: %v", err)
	}
	value, err := got.GetValue()
	if err != nil {
		t.Fatalf("materialize numeric JSON scalar failed: %v", err)
	}
	if value != want {
		t.Fatalf("numeric JSON scalar = %#v, want %#v", value, want)
	}
}

// TestDriver_OSON_Accessors verifies lazy object, array, and scalar accessors
// navigate a document fetched from a native JSON column.
func TestDriver_OSON_Accessors(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	doc, err := ojson.NewJSON(map[string]any{"items": []any{"oracle"}})
	if err != nil {
		t.Fatalf("create JSON document failed: %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", doc); err != nil {
		t.Fatalf("insert JSON document failed: %v", err)
	}

	var got ojson.JSON
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 1").Scan(&got); err != nil {
		t.Fatalf("fetch JSON document failed: %v", err)
	}
	// Exercise each lazy view against database-fetched OSON, not a locally
	// constructed document.
	object, err := got.AsJSONObject()
	if err != nil {
		t.Fatalf("access fetched document as object failed: %v", err)
	}
	items, err := object.Get("items")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	array, err := items.AsJSONArray()
	if err != nil {
		t.Fatalf("access items as array failed: %v", err)
	}
	item, err := array.Get(0)
	if err != nil {
		t.Fatalf("access first array item failed: %v", err)
	}
	scalar, err := item.AsJSONScalar()
	if err != nil {
		t.Fatalf("access first array item as scalar failed: %v", err)
	}
	value, err := scalar.GetValue()
	if err != nil {
		t.Fatalf("materialize accessed scalar failed: %v", err)
	}
	if value != "oracle" {
		t.Fatalf("accessed scalar = %#v, want %q", value, "oracle")
	}
}

// TestDriver_OSON_ZeroValue verifies an uninitialized JSON is neither a bind
// value nor a materialized JSON null document.
func TestDriver_OSON_ZeroValue(t *testing.T) {
	// The zero value has no OSON document, so this contract is necessarily unitary.
	var doc ojson.JSON
	if _, err := doc.Value(); err == nil {
		t.Fatal("zero JSON Value() error = nil, want error")
	}
	if _, err := doc.GetValue(); err == nil {
		t.Fatal("zero JSON GetValue() error = nil, want error")
	}
	if doc.String() != "<JSON: uninitialized>" {
		t.Fatalf("zero JSON String() = %q, want uninitialized marker", doc.String())
	}
}

// TestDriver_OSON_NullDistinction verifies JSON null remains a valid document
// while SQL NULL is represented by an invalid sql.Null[json.JSON].
func TestDriver_OSON_NullDistinction(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	jsonNull, err := ojson.NewJSON(nil)
	if err != nil {
		t.Fatalf("create JSON null document failed: %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", jsonNull); err != nil {
		t.Fatalf("insert JSON null value failed: %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (2, null)"); err != nil {
		t.Fatalf("insert SQL null value failed: %v", err)
	}

	// sql.Null retains the database NULL distinction that JSON itself cannot
	// represent: JSON null is a valid scalar document.
	var fetchedJSONNull, fetchedSQLNull sql.Null[ojson.JSON]
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 1").Scan(&fetchedJSONNull); err != nil {
		t.Fatalf("fetch JSON null failed: %v", err)
	}
	if err = db.QueryRowContext(ctx, "select doc from "+table+" where id = 2").Scan(&fetchedSQLNull); err != nil {
		t.Fatalf("fetch SQL null failed: %v", err)
	}
	if !fetchedJSONNull.Valid || fetchedSQLNull.Valid {
		t.Fatalf("JSON and SQL null validity = (%v, %v), want (true, false)", fetchedJSONNull.Valid, fetchedSQLNull.Valid)
	}
	value, err := fetchedJSONNull.V.GetValue()
	if err != nil {
		t.Fatalf("materialize JSON null failed: %v", err)
	}
	if value != nil {
		t.Fatalf("JSON null materialized as %#v, want nil", value)
	}
}

// setupOSONTable creates an isolated native JSON table for one OSON functional
// test and registers cleanup for the table and its database handle.
func setupOSONTable(t *testing.T) (*sql.DB, context.Context, string) {
	t.Helper()
	db := openOSONTestDB(t)
	ctx := osonTestContext(t, time.Minute)
	// A unique name allows the functional category to run tests concurrently.
	table := createObjectName("t_oson")
	if err := createTable(ctx, db, table, map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"doc": "JSON",
	}); err != nil {
		t.Fatalf("create native JSON table failed: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := dropTable(cleanupCtx, db, table); err != nil {
			t.Errorf("drop OSON test table %s: %v", table, err)
		}
	})
	return db, ctx, table
}

var osonBindStyles = []struct {
	name     string
	prepared bool
	named    bool
}{
	{name: "direct_positional"},
	{name: "direct_named_reordered", named: true},
	{name: "prepared_positional", prepared: true},
	{name: "prepared_named_reordered", prepared: true, named: true},
}

func osonInsertSQL(table string, named bool) string {
	if named {
		return "INSERT INTO " + table + " (id, doc) VALUES (:row_id, :document)"
	}
	return "INSERT INTO " + table + " (id, doc) VALUES (:1, :2)"
}

func osonInsertArgs(id int, value any, named bool) []any {
	if named {
		return []any{sql.Named("document", value), sql.Named("row_id", id)}
	}
	return []any{id, value}
}

// TestDriver_OSON_F01_InputAndBindPaths verifies constructor semantics and exact numbers across direct, prepared, positional and reordered named binds.
func TestDriver_OSON_F01_InputAndBindPaths(t *testing.T) {
	db, _, table := setupOSONTable(t)
	const text = `{"id":9007199254740993,"name":"Alice","active":true}`
	wantObject := map[string]any{"id": stdjson.Number("9007199254740993"), "name": "Alice", "active": true}
	inputs := []struct {
		name   string
		create func() (any, error)
		kind   ojson.JSONKind
		want   any
	}{
		{"go_object", func() (any, error) { return ojson.NewJSON(wantObject) }, ojson.JSONObjectKind, wantObject},
		{"json_text", func() (any, error) { return ojson.NewJSONFromString(text), nil }, ojson.JSONObjectKind, wantObject},
		{"json_unmarshal", func() (any, error) {
			var doc ojson.JSON
			err := stdjson.Unmarshal([]byte(text), &doc)
			return doc, err
		}, ojson.JSONObjectKind, wantObject},
		{"text_as_string_scalar", func() (any, error) { return ojson.NewJSON(text) }, ojson.JSONScalarKind, text},
	}
	for styleIndex, style := range osonBindStyles {
		t.Run(style.name, func(t *testing.T) {
			ctx := osonTestContext(t, time.Minute)
			query := osonInsertSQL(table, style.named)
			exec := func(ctx context.Context, args ...any) (sql.Result, error) { return db.ExecContext(ctx, query, args...) }
			if style.prepared {
				stmt, err := db.PrepareContext(ctx, query)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = stmt.Close() })
				exec = stmt.ExecContext
			}
			for inputIndex, input := range inputs {
				t.Run(input.name, func(t *testing.T) {
					ctx := osonTestContext(t, time.Minute)
					value, err := input.create()
					if err != nil {
						t.Fatalf("construct input: %v", err)
					}
					id := styleIndex*len(inputs) + inputIndex + 1
					result, err := exec(ctx, osonInsertArgs(id, value, style.named)...)
					if err != nil {
						t.Fatalf("insert input: %v", err)
					}
					if n, err := result.RowsAffected(); err != nil || n != 1 {
						t.Fatalf("insert rows affected=(%d,%v), want 1", n, err)
					}
					var got ojson.JSON
					got.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
					var serverText string
					if err := db.QueryRowContext(ctx, "SELECT doc, JSON_SERIALIZE(doc RETURNING VARCHAR2(4000) ERROR ON ERROR) FROM "+table+" WHERE id=:1", id).Scan(&got, &serverText); err != nil {
						t.Fatalf("fetch document and independent SQL serialization: %v", err)
					}
					if kind, err := got.Kind(); err != nil || kind != input.kind {
						t.Fatalf("Kind=(%v,%v), want %v", kind, err, input.kind)
					}
					if decoded, err := got.GetValue(); err != nil || !reflect.DeepEqual(decoded, input.want) {
						t.Fatalf("GetValue=(%#v,%v), want %#v", decoded, err, input.want)
					}
					wantText, err := stdjson.Marshal(input.want)
					if err != nil {
						t.Fatal(err)
					}
					rendered, err := stdjson.Marshal(got)
					if err != nil {
						t.Fatalf("marshal fetched document: %v", err)
					}
					assertOSONSemanticJSON(t, string(rendered), string(wantText))
					assertOSONSemanticJSON(t, serverText, string(wantText))
					if input.kind == ojson.JSONObjectKind {
						var number, name, active string
						query := "SELECT JSON_VALUE(doc, '$.id' RETURNING VARCHAR2(100) ERROR ON ERROR), JSON_VALUE(doc, '$.name' RETURNING VARCHAR2(100) ERROR ON ERROR), JSON_VALUE(doc, '$.active' RETURNING VARCHAR2(10) ERROR ON ERROR) FROM " + table + " WHERE id=:1"
						if err := db.QueryRowContext(ctx, query, id).Scan(&number, &name, &active); err != nil {
							t.Fatal(err)
						}
						if number != "9007199254740993" || name != "Alice" || active != "true" {
							t.Fatalf("SQL extraction=(%q,%q,%q), want exact input fields", number, name, active)
						}
					} else {
						var scalar string
						if err := db.QueryRowContext(ctx, "SELECT JSON_VALUE(doc, '$' RETURNING VARCHAR2(4000) ERROR ON ERROR) FROM "+table+" WHERE id=:1", id).Scan(&scalar); err != nil || scalar != text {
							t.Fatalf("SQL root extraction=(%q,%v), want original text", scalar, err)
						}
					}
				})
			}
		})
	}
	var count int
	if err := db.QueryRowContext(osonTestContext(t, time.Minute), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 16 {
		t.Fatalf("row count=(%d,%v), want all 16 matrix entries", count, err)
	}
}

// TestDriver_OSON_F02_RootValuesAndNulls verifies root kinds, typed nil values and the distinction between JSON null and SQL NULL.
func TestDriver_OSON_F02_RootValuesAndNulls(t *testing.T) {
	db, _, table := setupOSONTable(t)
	tests := []struct {
		name  string
		input any
		want  any
		kind  ojson.JSONKind
	}{
		{"object", map[string]any{"x": "value"}, map[string]any{"x": "value"}, ojson.JSONObjectKind},
		{"array", []any{"value", true}, []any{"value", true}, ojson.JSONArrayKind},
		{"string", "value", "value", ojson.JSONScalarKind},
		{"empty_string", "", "", ojson.JSONScalarKind},
		{"true", true, true, ojson.JSONScalarKind},
		{"false", false, false, ojson.JSONScalarKind},
		{"number", stdjson.Number("9007199254740993"), stdjson.Number("9007199254740993"), ojson.JSONScalarKind},
		{"json_null", nil, nil, ojson.JSONScalarKind},
		{"empty_object", map[string]any{}, map[string]any{}, ojson.JSONObjectKind},
		{"nil_object", map[string]any(nil), map[string]any{}, ojson.JSONObjectKind},
		{"empty_array", []any{}, []any{}, ojson.JSONArrayKind},
		{"nil_array", []any(nil), []any{}, ojson.JSONArrayKind},
		{"empty_binary", []byte{}, []byte{}, ojson.JSONScalarKind},
		{"nil_binary", []byte(nil), []byte{}, ojson.JSONScalarKind},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := osonTestContext(t, time.Minute)
			doc, err := ojson.NewJSON(test.input)
			if err != nil {
				t.Fatal(err)
			}
			id := index + 1
			if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id,doc) VALUES (:1,:2)", id, doc); err != nil {
				t.Fatal(err)
			}
			var got sql.Null[ojson.JSON]
			got.V.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
			if err := db.QueryRowContext(ctx, "SELECT doc FROM "+table+" WHERE id=:1", id).Scan(&got); err != nil || !got.Valid {
				t.Fatalf("fetch=(valid:%v,%v), want valid JSON", got.Valid, err)
			}
			if kind, err := got.V.Kind(); err != nil || kind != test.kind {
				t.Fatalf("Kind=(%v,%v), want %v", kind, err, test.kind)
			}
			value, err := got.V.GetValue()
			if err != nil {
				t.Fatal(err)
			}
			if want, ok := test.want.([]byte); ok {
				actual, ok := value.([]byte)
				if !ok || !bytes.Equal(actual, want) {
					t.Fatalf("binary=%#v, want empty binary", value)
				}
			} else if !reflect.DeepEqual(value, test.want) {
				t.Fatalf("value=%#v, want %#v", value, test.want)
			}
			text, err := stdjson.Marshal(got.V)
			if err != nil {
				t.Fatal(err)
			}
			wantText := `""`
			if _, binary := test.want.([]byte); !binary {
				encoded, err := stdjson.Marshal(test.want)
				if err != nil {
					t.Fatal(err)
				}
				wantText = string(encoded)
			}
			assertOSONSemanticJSON(t, string(text), wantText)
			var serverText string
			if err := db.QueryRowContext(ctx, "SELECT JSON_SERIALIZE(doc RETURNING VARCHAR2(4000) ERROR ON ERROR) FROM "+table+" WHERE id=:1", id).Scan(&serverText); err != nil {
				t.Fatal(err)
			}
			assertOSONSemanticJSON(t, serverText, wantText)
		})
	}
	t.Run("reused_nullable_receiver", func(t *testing.T) {
		ctx := osonTestContext(t, time.Minute)
		if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id,doc) VALUES (100,:1)", nil); err != nil {
			t.Fatal(err)
		}
		var receiver sql.Null[ojson.JSON]
		for _, id := range []int{1, 100, 8, 1} {
			if err := db.QueryRowContext(ctx, "SELECT doc FROM "+table+" WHERE id=:1", id).Scan(&receiver); err != nil {
				t.Fatal(err)
			}
			if receiver.Valid != (id != 100) {
				t.Fatalf("row %d: Valid=%v", id, receiver.Valid)
			}
			if id == 100 {
				if _, err := receiver.V.GetValue(); err == nil {
					t.Fatal("SQL NULL left the previous document in a reused nullable receiver")
				}
			} else {
				value, err := receiver.V.GetValue()
				var want any
				if id == 1 {
					want = map[string]any{"x": "value"}
				}
				if err != nil || !reflect.DeepEqual(value, want) {
					t.Fatalf("row %d: value=(%#v,%v), want %#v", id, value, err, want)
				}
			}
		}
	})
	t.Run("uninitialized_is_not_json_null", func(t *testing.T) {
		var doc ojson.JSON
		_, err := doc.Kind()
		assertOSONErrorCode(t, err, oracleErrors.JSONUninitializedValue)
		_, err = doc.GetValue()
		assertOSONErrorCode(t, err, oracleErrors.JSONUninitializedValue)
	})
}

// TestDriver_OSON_F09_InvalidInputRecovery verifies invalid binds insert no rows and the same connection and prepared statements remain usable.
func TestDriver_OSON_F09_InvalidInputRecovery(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, time.Minute)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	invalid := []struct {
		name  string
		value any
		code  oracleErrors.ErrorCode
	}{
		{"invalid_text", ojson.NewJSONFromString(`{"broken":`), oracleErrors.OsonEncodingError},
		{"uninitialized", ojson.JSON{}, oracleErrors.JSONUninitializedValue},
	}
	for styleIndex, style := range osonBindStyles {
		t.Run(style.name, func(t *testing.T) {
			ctx := osonTestContext(t, time.Minute)
			query := osonInsertSQL(table, style.named)
			exec := func(ctx context.Context, args ...any) (sql.Result, error) {
				return conn.ExecContext(ctx, query, args...)
			}
			if style.prepared {
				stmt, err := conn.PrepareContext(ctx, query)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = stmt.Close() })
				exec = stmt.ExecContext
			}
			for invalidIndex, input := range invalid {
				t.Run(input.name, func(t *testing.T) {
					ctx := osonTestContext(t, time.Minute)
					id := styleIndex*len(invalid) + invalidIndex + 1
					_, err := exec(ctx, osonInsertArgs(id, input.value, style.named)...)
					assertOSONErrorCode(t, err, input.code)
					var count int
					if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE id=:1", id).Scan(&count); err != nil || count != 0 {
						t.Fatalf("failed bind row count=(%d,%v), want 0", count, err)
					}
					valid, err := ojson.NewJSON(map[string]any{"recovered": true})
					if err != nil {
						t.Fatal(err)
					}
					if _, err := exec(ctx, osonInsertArgs(id, valid, style.named)...); err != nil {
						t.Fatalf("valid bind after failed bind: %v", err)
					}
					var got ojson.JSON
					if err := conn.QueryRowContext(ctx, "SELECT doc FROM "+table+" WHERE id=:1", id).Scan(&got); err != nil {
						t.Fatalf("fetch after recovery: %v", err)
					}
					text, err := stdjson.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					assertOSONSemanticJSON(t, string(text), `{"recovered":true}`)
				})
			}
		})
	}
}

// TestDriver_OSON_F10_LargeBinaryDocuments verifies large OSON payloads retain complete content across fetching, child access and rebinding.
func TestDriver_OSON_F10_LargeBinaryDocuments(t *testing.T) {
	db, _, table := setupOSONTable(t)
	for index, size := range []int{20 * 1024 * 1024, 30 * 1024 * 1024} {
		t.Run(fmt.Sprintf("%dMiB", size/(1024*1024)), func(t *testing.T) {
			ctx := osonTestContext(t, 5*time.Minute)
			const pattern = "OSON0123456789ab"
			payload := strings.Repeat(pattern, size/len(pattern)) + strings.Repeat("x", size%len(pattern))
			if len(payload) != size {
				t.Fatalf("fixture length=%d, want %d", len(payload), size)
			}
			wantDigest := sha256.Sum256([]byte(payload))
			doc, err := ojson.NewJSON(map[string]any{"payload": payload, "tail": "complete"})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := doc.Value()
			if err != nil {
				t.Fatal(err)
			}
			encoded, ok := raw.([]byte)
			if !ok || !oson.IsOson(encoded) || len(encoded) <= size {
				t.Fatalf("source is not the expected large binary OSON: type=%T", raw)
			}
			t.Logf("payload_bytes=%d encoded_oson_bytes=%d sha256=%x", size, len(encoded), wantDigest)
			id := index*2 + 1
			if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id,doc) VALUES (:1,:2)", id, doc); err != nil {
				t.Fatal(err)
			}
			var fetched ojson.JSON
			var sentinel int64
			if err := db.QueryRowContext(ctx, "SELECT doc,987654321 FROM "+table+" WHERE id=:1", id).Scan(&fetched, &sentinel); err != nil || sentinel != 987654321 {
				t.Fatalf("large fetch/trailing column=(%d,%v)", sentinel, err)
			}
			verify := func(t *testing.T, value ojson.JSON) {
				t.Helper()
				object, err := value.AsJSONObject()
				if err != nil {
					t.Fatal(err)
				}
				child, err := object.Get("payload")
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := child.GetValue()
				if err != nil {
					t.Fatal(err)
				}
				text, ok := decoded.(string)
				if !ok || len(text) != size || sha256.Sum256([]byte(text)) != wantDigest {
					t.Fatalf("large payload content mismatch: type=%T", decoded)
				}
			}
			verify(t, fetched)
			if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id,doc) VALUES (:1,:2)", id+1, fetched); err != nil {
				t.Fatalf("rebind fetched OSON: %v", err)
			}
			var rebound ojson.JSON
			if err := db.QueryRowContext(ctx, "SELECT doc FROM "+table+" WHERE id=:1", id+1).Scan(&rebound); err != nil {
				t.Fatal(err)
			}
			verify(t, rebound)
			var serializedLength int
			var tail string
			if err := db.QueryRowContext(ctx, "SELECT LENGTH(JSON_SERIALIZE(doc RETURNING CLOB ERROR ON ERROR)), JSON_VALUE(doc,'$.tail' RETURNING VARCHAR2(100) ERROR ON ERROR) FROM "+table+" WHERE id=:1", id+1).Scan(&serializedLength, &tail); err != nil {
				t.Fatalf("independent SQL verification: %v", err)
			}
			emptyText, err := stdjson.Marshal(map[string]any{"payload": "", "tail": "complete"})
			if err != nil {
				t.Fatal(err)
			}
			if serializedLength != size+len(emptyText) || tail != "complete" {
				t.Fatalf("SQL serialized length/tail=(%d,%q), want (%d,complete)", serializedLength, tail, size+len(emptyText))
			}
		})
	}
}

// OSON-F03 keeps the guide's default Go types strict. Source/fetched wire
// diagnostics distinguish a server normalization from a driver conversion bug.
func TestDriver_OSON_F03_Numbers(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, 5*time.Minute)
	type numberCase struct {
		name        string
		input       any
		wantDefault any
		exact       string
		floatBits   int
	}
	cases := []numberCase{
		{"int-min", int(math.MinInt), int64(math.MinInt), strconv.FormatInt(int64(math.MinInt), 10), 0},
		{"int-max", int(math.MaxInt), int64(math.MaxInt), strconv.FormatInt(int64(math.MaxInt), 10), 0},
		{"int-zero", int(0), int64(0), "0", 0},
		{"int-representative", int(-42), int64(-42), "-42", 0},
		{"int8-min", int8(math.MinInt8), int32(math.MinInt8), "-128", 0},
		{"int8-max", int8(math.MaxInt8), int32(math.MaxInt8), "127", 0},
		{"int8-zero", int8(0), int32(0), "0", 0},
		{"int8-representative", int8(-8), int32(-8), "-8", 0},
		{"int16-min", int16(math.MinInt16), int32(math.MinInt16), "-32768", 0},
		{"int16-max", int16(math.MaxInt16), int32(math.MaxInt16), "32767", 0},
		{"int16-zero", int16(0), int32(0), "0", 0},
		{"int16-representative", int16(-16), int32(-16), "-16", 0},
		{"int32-min", int32(math.MinInt32), int32(math.MinInt32), "-2147483648", 0},
		{"int32-max", int32(math.MaxInt32), int32(math.MaxInt32), "2147483647", 0},
		{"int32-representative", int32(-32), int32(-32), "-32", 0},
		{"int64-min", int64(math.MinInt64), int64(math.MinInt64), "-9223372036854775808", 0},
		{"int64-max", int64(math.MaxInt64), int64(math.MaxInt64), "9223372036854775807", 0},
		{"int64-zero", int64(0), int64(0), "0", 0},
		{"int64-representative", int64(-64), int64(-64), "-64", 0},
		{"uint-max", ^uint(0), stdjson.Number(strconv.FormatUint(uint64(^uint(0)), 10)), strconv.FormatUint(uint64(^uint(0)), 10), 0},
		{"uint-zero", uint(0), stdjson.Number("0"), "0", 0},
		{"uint-representative", uint(42), stdjson.Number("42"), "42", 0},
		{"uint8-max", uint8(math.MaxUint8), int32(math.MaxUint8), "255", 0},
		{"uint8-zero", uint8(0), int32(0), "0", 0},
		{"uint8-representative", uint8(8), int32(8), "8", 0},
		{"uint16-max", uint16(math.MaxUint16), int32(math.MaxUint16), "65535", 0},
		{"uint16-zero", uint16(0), int32(0), "0", 0},
		{"uint16-representative", uint16(16), int32(16), "16", 0},
		{"uint32-max", uint32(math.MaxUint32), int64(math.MaxUint32), "4294967295", 0},
		{"uint32-zero", uint32(0), int64(0), "0", 0},
		{"uint32-representative", uint32(32), int64(32), "32", 0},
		{"uint64-max", uint64(math.MaxUint64), stdjson.Number("18446744073709551615"), "18446744073709551615", 0},
		{"uint64-zero", uint64(0), stdjson.Number("0"), "0", 0},
		{"uint64-representative", uint64(64), stdjson.Number("64"), "64", 0},
		{"zero", int32(0), int32(0), "0", 0},
		{"below-2pow53", int64(9007199254740991), int64(9007199254740991), "9007199254740991", 0},
		{"at-2pow53", int64(9007199254740992), int64(9007199254740992), "9007199254740992", 0},
		{"above-2pow53", int64(9007199254740993), int64(9007199254740993), "9007199254740993", 0},
		{"negative-below-2pow53", int64(-9007199254740991), int64(-9007199254740991), "-9007199254740991", 0},
		{"negative-at-2pow53", int64(-9007199254740992), int64(-9007199254740992), "-9007199254740992", 0},
		{"negative-above-2pow53", int64(-9007199254740993), int64(-9007199254740993), "-9007199254740993", 0},
		{"decimal-38-digits", stdjson.Number("12345678901234567890.123456789012345678"), stdjson.Number("12345678901234567890.123456789012345678"), "12345678901234567890.123456789012345678", 0},
		{"negative-fraction", stdjson.Number("-0.00000000000000000012345"), stdjson.Number("-0.00000000000000000012345"), "-0.00000000000000000012345", 0},
		{"negative-exponent", stdjson.Number("1e-20"), stdjson.Number("1e-20"), "1e-20", 0},
		{"positive-exponent", stdjson.Number("1.2345e20"), stdjson.Number("1.2345e20"), "1.2345e20", 0},
		{"decimal-trailing-zeros", stdjson.Number("1.000"), stdjson.Number("1.000"), "1", 0},
		{"decimal-zero", stdjson.Number("0"), stdjson.Number("0"), "0", 0},
		{"float32-fraction", float32(0.1), float64(float32(0.1)), "0.1", 32},
		{"float32-negative", float32(-12.5), float64(-12.5), "-12.5", 32},
		{"float32-zero", float32(0), float64(0), "0", 32},
		{"float32-negative-zero", float32(math.Copysign(0, -1)), math.Copysign(0, -1), "-0", 32},
		{"float32-subnormal", float32(math.SmallestNonzeroFloat32), float64(float32(math.SmallestNonzeroFloat32)), "1e-45", 32},
		{"float32-max", float32(math.MaxFloat32), float64(float32(math.MaxFloat32)), "3.4028235e38", 32},
		{"float64-fraction", float64(0.1), float64(0.1), "0.1", 64},
		{"float64-negative", float64(-42.25), float64(-42.25), "-42.25", 64},
		{"float64-zero", float64(0), float64(0), "0", 64},
		{"float64-negative-zero", math.Copysign(0, -1), math.Copysign(0, -1), "-0", 64},
		{"float64-subnormal", math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, "5e-324", 64},
		{"float64-max", math.MaxFloat64, math.MaxFloat64, "1.7976931348623157e308", 64},
	}
	for _, input := range []struct {
		name string
		v    float64
		text string
	}{{"nan", math.NaN(), "Nan"}, {"positive-infinity", math.Inf(1), "Inf"}, {"negative-infinity", math.Inf(-1), "-Inf"}} {
		cases = append(cases, numberCase{"float32-" + input.name, float32(input.v), input.v, input.text, 32}, numberCase{"float64-" + input.name, input.v, input.v, input.text, 64})
	}
	insert, err := db.PrepareContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)")
	if err != nil {
		t.Fatalf("prepare numeric insert: %v", err)
	}
	defer insert.Close()
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := ojson.NewJSON(tc.input)
			if err != nil {
				t.Fatalf("construct numeric root: %v", err)
			}
			nested, err := ojson.NewJSON(map[string]any{"n": tc.input, "items": []any{tc.input}})
			if err != nil {
				t.Fatalf("construct nested numbers: %v", err)
			}
			rootID, nestedID := i*2+1, i*2+2
			if _, err = insert.ExecContext(ctx, rootID, root); err != nil {
				t.Fatalf("bind numeric root: %v", err)
			}
			if _, err = insert.ExecContext(ctx, nestedID, nested); err != nil {
				t.Fatalf("bind nested numbers: %v", err)
			}
			var fetched, fetchedNested ojson.JSON
			var serverText, serverType string
			if err = db.QueryRowContext(ctx, "select doc, json_serialize(doc returning clob), json_value(doc, '$.type()' returning varchar2(80) error on error) from "+table+" where id=:1", rootID).Scan(&fetched, &serverText, &serverType); err != nil {
				t.Fatalf("fetch numeric root and independent SQL checks: %v", err)
			}
			if err = db.QueryRowContext(ctx, "select doc from "+table+" where id=:1", nestedID).Scan(&fetchedNested); err != nil {
				t.Fatalf("fetch nested numbers: %v", err)
			}
			sourceWire := osonNumericDocumentBytes(t, root)
			fetchedWire := osonNumericDocumentBytes(t, fetched)
			if !bytes.Equal(sourceWire, fetchedWire) {
				t.Logf("server changed numeric wire representation; source=%x fetched=%x; classify normalization before a default-type failure", sourceWire, fetchedWire)
			}
			wantType := "number"
			if tc.floatBits == 32 {
				wantType = "float"
			} else if tc.floatBits == 64 {
				wantType = "double"
			}
			if serverType != wantType {
				t.Errorf("SQL numeric type=%q, want %q; source=%x fetched=%x", serverType, wantType, sourceWire, fetchedWire)
			}
			if f, ok := tc.wantDefault.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
				osonAssertJSONScalarString(t, serverText, tc.exact)
			} else {
				osonAssertExactNumericText(t, serverText, tc.exact, tc.floatBits, false)
			}
			originalText, err := stdjson.Marshal(fetched)
			if err != nil {
				t.Fatalf("marshal numeric root: %v", err)
			}
			for _, mode := range osonFunctionalNumberModes() {
				t.Run(mode.name, func(t *testing.T) {
					fetched.SetOptions(ojson.Options{NumberMode: mode.value})
					fetchedNested.SetOptions(ojson.Options{NumberMode: mode.value})
					actual, err := fetched.GetValue()
					if err != nil {
						t.Fatalf("materialize numeric root: %v", err)
					}
					osonAssertNumericMode(t, actual, tc.wantDefault, tc.exact, tc.floatBits, mode.value)
					nestedValue, err := fetchedNested.GetValue()
					if err != nil {
						t.Fatalf("materialize nested numbers: %v", err)
					}
					object, ok := nestedValue.(map[string]any)
					if !ok {
						t.Fatalf("nested numeric document type=%T, want map[string]any", nestedValue)
					}
					items, ok := object["items"].([]any)
					if !ok || len(items) != 1 {
						t.Fatalf("nested items=%#v, want one-element array", object["items"])
					}
					osonAssertNumericMode(t, object["n"], tc.wantDefault, tc.exact, tc.floatBits, mode.value)
					osonAssertNumericMode(t, items[0], tc.wantDefault, tc.exact, tc.floatBits, mode.value)
					objectView, err := fetchedNested.AsJSONObject()
					if err != nil {
						t.Fatalf("lazy numeric object: %v", err)
					}
					child, err := objectView.Get("n")
					if err != nil {
						t.Fatalf("lazy numeric child: %v", err)
					}
					childValue, err := child.GetValue()
					if err != nil {
						t.Fatalf("materialize lazy numeric child: %v", err)
					}
					osonAssertNumericMode(t, childValue, tc.wantDefault, tc.exact, tc.floatBits, mode.value)
					text, err := stdjson.Marshal(fetched)
					if err != nil || string(text) != string(originalText) {
						t.Fatalf("number mode changed JSON rendering: got %s (%v), want %s", text, err, originalText)
					}
				})
			}
		})
	}
}

// OSON-F04: these expressions construct JSON in SQL without the Go encoder.
// Intervals use the driver's documented canonical Go strings when rendered,
// which need not match Oracle JSON_SERIALIZE's ISO interval spelling.
func TestDriver_OSON_F04_ServerScalars(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, 3*time.Minute)
	type scalarCase struct {
		name, expression, sqlType, jsonText string
		want                                any
		idCandidate                         bool
	}
	cases := []scalarCase{
		{"exact-number", "json_scalar(12345678901234567890.123456789012345678)", "number", "12345678901234567890.123456789012345678", stdjson.Number("12345678901234567890.123456789012345678"), false},
		{"small-number", "json_scalar(-0.0001)", "number", "-0.0001", stdjson.Number("-0.0001"), false},
		{"number-exponent-low", "json_scalar(1e-130)", "number", "1e-130", stdjson.Number("1e-130"), false},
		{"number-exponent-high", "json_scalar(1e125)", "number", "1e125", stdjson.Number("1e125"), false},
		{"binary-float", "json_scalar(cast(12.5 as binary_float))", "float", "12.5", float64(12.5), false},
		{"binary-double", "json_scalar(cast(-42.25 as binary_double))", "double", "-42.25", float64(-42.25), false},
		{"sql-nonfinite-nan", "json_scalar(binary_double_nan)", "string", `"Nan"`, "Nan", false},
		{"sql-nonfinite-positive-infinity", "json_scalar(binary_double_infinity)", "string", `"Inf"`, "Inf", false},
		{"sql-nonfinite-negative-infinity", "json_scalar(-binary_double_infinity)", "string", `"-Inf"`, "-Inf", false},
		{"raw", "json_scalar(hextoraw('00AAFF'))", "binary", `"00AAFF"`, []byte{0x00, 0xaa, 0xff}, false},
		{"oid-extended", `json('{"$oid":"0102030405060708090A0B0C"}' extended)`, "binary", `"0102030405060708090A0B0C"`, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, true},
		{"rawid-extended", `json('{"$rawid":"000102030405060708090A0B0C0D0E0F"}' extended)`, "binary", `"000102030405060708090A0B0C0D0E0F"`, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, true},
		{"date", "json_scalar(to_date('2024-02-29 23:59:58','YYYY-MM-DD HH24:MI:SS'))", "date", `"2024-02-29T23:59:58"`, time.Date(2024, 2, 29, 23, 59, 58, 0, time.Local), false},
		{"timestamp-fraction", "json_scalar(timestamp '2024-02-29 23:59:59.123456789')", "timestamp", `"2024-02-29T23:59:59.123456789"`, time.Date(2024, 2, 29, 23, 59, 59, 123456789, time.Local), false},
		{"timestamp-whole-seconds", "json_scalar(timestamp '2000-01-01 00:00:00')", "timestamp", `"2000-01-01T00:00:00"`, time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local), false},
		{"timestamp-tz-extended", `json('{"$oracleTimestampTZ":"2024-02-29T23:59:59.123456789+05:30"}' extended)`, "timestamp with time zone", `"2024-02-29T23:59:59.123456789+05:30"`, time.Date(2024, 2, 29, 23, 59, 59, 123456789, time.FixedZone("", 19800)), false},
		{"interval-year-month", "json_scalar(interval '2-3' year to month)", "yearmonthInterval", `"02-03"`, "02-03", false},
		{"interval-negative-month", "json_scalar(interval '-0-5' year to month)", "yearmonthInterval", `"-00-05"`, "-00-05", false},
		{"interval-zero-month", "json_scalar(interval '0-0' year to month)", "yearmonthInterval", `"00-00"`, "00-00", false},
		{"interval-large-year", "json_scalar(interval '123456789-11' year(9) to month)", "yearmonthInterval", `"123456789-11"`, "123456789-11", false},
		{"interval-day-second", "json_scalar(interval '10 05:30:02.123456789' day(2) to second(9))", "daysecondInterval", `"10 05:30:02.123456789"`, "10 05:30:02.123456789", false},
		{"interval-negative-fraction", "json_scalar(interval '-0 00:00:00.000000001' day to second(9))", "daysecondInterval", `"-0 00:00:00.000000001"`, "-0 00:00:00.000000001", false},
		{"interval-zero-seconds", "json_scalar(interval '0 00:00:00' day to second)", "daysecondInterval", `"00 00:00:00.0"`, "00 00:00:00.0", false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) select :1, "+tc.expression+" from dual", i+1); err != nil {
				t.Fatalf("construct SQL scalar: %v", err)
			}
			var doc ojson.JSON
			var sqlType, serverText string
			if err := db.QueryRowContext(ctx, "select doc, json_value(doc, '$.type()' returning varchar2(80) error on error), json_serialize(doc returning clob) from "+table+" where id=:1", i+1).Scan(&doc, &sqlType, &serverText); err != nil {
				t.Fatalf("fetch SQL scalar: %v", err)
			}
			if tc.idCandidate {
				opcode, ok := osonFunctionalRootScalarOpcode(osonNumericDocumentBytes(t, doc))
				if ok && opcode == 0x7e {
					t.Log("OSON ID opcode 0x7e confirmed")
				} else {
					t.Logf("ID-shaped SQL input returned opcode 0x%02x (located=%v); binary value tested, ID opcode remains unconfirmed", opcode, ok)
				}
			}
			nonfinite := strings.HasPrefix(tc.name, "sql-nonfinite-")
			if nonfinite {
				// Server releases differ: JSON_SCALAR can retain a native
				// nonfinite float or convert it to an ordinary JSON string.
				if sqlType != "double" && sqlType != "float" && sqlType != "string" {
					t.Fatalf("SQL nonfinite category=%q, want double, float or string", sqlType)
				}
			} else if sqlType != tc.sqlType {
				t.Errorf("SQL scalar type=%q, want %q", sqlType, tc.sqlType)
			}
			if kind, err := doc.Kind(); err != nil || kind != ojson.JSONScalarKind {
				t.Fatalf("root kind=%v (%v), want scalar", kind, err)
			}
			value, err := doc.GetValue()
			if err != nil {
				t.Fatalf("materialize SQL-generated scalar: %v; wire=%x", err, osonNumericDocumentBytes(t, doc))
			}
			if nonfinite {
				osonAssertSQLNonfiniteCategory(t, &doc, sqlType, tc.want.(string))
			} else if number, ok := tc.want.(stdjson.Number); ok {
				osonAssertNumericMode(t, value, number, string(number), 0, ojson.NumberDefault)
				osonAssertExactNumericText(t, serverText, string(number), 0, false)
			} else if wantTime, ok := tc.want.(time.Time); ok {
				osonAssertFunctionalTime(t, value, wantTime, tc.sqlType == "timestamp with time zone")
			} else if !reflect.DeepEqual(value, tc.want) {
				t.Fatalf("SQL scalar value=%#v (%T), want %#v (%T); wire=%x", value, value, tc.want, tc.want, osonNumericDocumentBytes(t, doc))
			}
			text, err := stdjson.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal SQL-generated scalar: %v", err)
			}
			if number, ok := tc.want.(stdjson.Number); ok {
				osonAssertExactNumericText(t, string(text), string(number), 0, false)
			} else {
				assertOSONSemanticJSON(t, string(text), tc.jsonText)
			}
			if tc.idCandidate {
				osonAssertServerIdentifierText(t, serverText, tc.want.([]byte))
			} else if tc.sqlType == "date" || tc.sqlType == "timestamp" {
				osonAssertZoneFreeServerText(t, serverText, tc.want.(time.Time))
			} else if tc.sqlType != "yearmonthInterval" && tc.sqlType != "daysecondInterval" {
				assertOSONSemanticJSON(t, serverText, tc.jsonText)
			}
		})
	}
	t.Run("nested-native-object-and-array", func(t *testing.T) {
		expression := "json_object(" +
			"'n' value json_scalar(9007199254740993.25), " +
			"'raw' value json_scalar(hextoraw('00AAFF')), " +
			"'when' value json_scalar(timestamp '2024-02-29 23:59:59.123456789'), " +
			"'ym' value json_scalar(interval '-0-5' year to month), " +
			"'ds' value json_scalar(interval '3 04:05:06.000000007' day to second(9)), " +
			"'items' value json_array(json_scalar(9007199254740993.25), json_scalar(hextoraw('00AAFF')), " +
			"json_scalar(timestamp '2024-02-29 23:59:59.123456789'), json_scalar(interval '-0-5' year to month), " +
			"json_scalar(interval '3 04:05:06.000000007' day to second(9)) returning json) returning json)"
		if _, err := db.ExecContext(ctx, "insert into "+table+" (id,doc) select 1000,"+expression+" from dual"); err != nil {
			t.Fatalf("construct SQL nested native scalars: %v", err)
		}
		var doc ojson.JSON
		var numberText, numberType, binaryType, timeType, ymType, dsType string
		if err := db.QueryRowContext(ctx, "select doc, json_value(doc,'$.n' returning varchar2(100) error on error), "+
			"json_value(doc,'$.n.type()'),json_value(doc,'$.raw.type()'),json_value(doc,'$.when.type()'),"+
			"json_value(doc,'$.ym.type()'),json_value(doc,'$.ds.type()') from "+table+" where id=1000").
			Scan(&doc, &numberText, &numberType, &binaryType, &timeType, &ymType, &dsType); err != nil {
			t.Fatalf("fetch SQL nested native scalars and references: %v", err)
		}
		osonAssertExactNumericText(t, numberText, "9007199254740993.25", 0, false)
		if !reflect.DeepEqual([]string{numberType, binaryType, timeType, ymType, dsType}, []string{"number", "binary", "timestamp", "yearmonthInterval", "daysecondInterval"}) {
			t.Fatalf("SQL nested native types=%v", []string{numberType, binaryType, timeType, ymType, dsType})
		}
		wantTime := time.Date(2024, 2, 29, 23, 59, 59, 123456789, time.Local)
		const wantJSON = `{"n":9007199254740993.25,"raw":"00AAFF","when":"2024-02-29T23:59:59.123456789","ym":"-00-05","ds":"03 04:05:06.000000007","items":[9007199254740993.25,"00AAFF","2024-02-29T23:59:59.123456789","-00-05","03 04:05:06.000000007"]}`
		for _, mode := range osonFunctionalNumberModes() {
			t.Run(mode.name, func(t *testing.T) {
				doc.SetOptions(ojson.Options{NumberMode: mode.value})
				var wantNumber any = stdjson.Number("9007199254740993.25")
				if mode.value == ojson.NumberAsFloat64 {
					wantNumber = float64(9007199254740994)
				}
				want := map[string]any{
					"n": wantNumber, "raw": []byte{0, 0xaa, 0xff}, "when": wantTime, "ym": "-00-05", "ds": "03 04:05:06.000000007",
					"items": []any{wantNumber, []byte{0, 0xaa, 0xff}, wantTime, "-00-05", "03 04:05:06.000000007"},
				}
				value, err := doc.GetValue()
				if err != nil || !reflect.DeepEqual(value, want) {
					t.Fatalf("nested SQL-native values=%#v (%v), want %#v", value, err, want)
				}
				text, err := stdjson.Marshal(doc)
				if err != nil {
					t.Fatalf("marshal nested native scalars: %v", err)
				}
				assertOSONSemanticJSON(t, string(text), wantJSON)
			})
		}
	})
	t.Run("SQL-timestamp-tz-conversion-policy", func(t *testing.T) {
		input := time.Date(2024, 2, 29, 23, 59, 59, 123456789, time.FixedZone("", 19800))
		expression := "json_scalar(to_timestamp_tz('2024-02-29 23:59:59.123456789 +05:30','YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM'))"
		if _, err := db.ExecContext(ctx, "insert into "+table+" (id,doc) select 1001,"+expression+" from dual"); err != nil {
			t.Fatalf("construct SQL timestamp-tz scalar: %v", err)
		}
		var doc ojson.JSON
		var sqlType, serverText string
		if err := db.QueryRowContext(ctx, "select doc,json_value(doc,'$.type()'),json_serialize(doc returning clob) from "+table+" where id=1001").Scan(&doc, &sqlType, &serverText); err != nil {
			t.Fatalf("fetch SQL timestamp-tz conversion: %v", err)
		}
		value, err := doc.GetValue()
		if err != nil {
			t.Fatalf("materialize SQL timestamp-tz conversion: %v", err)
		}
		switch sqlType {
		case "timestamp with time zone":
			osonAssertFunctionalTime(t, value, input, true)
			var text string
			if err := stdjson.Unmarshal([]byte(serverText), &text); err != nil {
				t.Fatalf("SQL timestamp-tz JSON reference: %v", err)
			}
			parsed, err := time.Parse(time.RFC3339Nano, text)
			if err != nil || !parsed.Equal(input) {
				t.Fatalf("SQL timestamp-tz changed instant: %q (%v)", text, err)
			}
		case "timestamp":
			utc := input.UTC()
			want := time.Date(utc.Year(), utc.Month(), utc.Day(), utc.Hour(), utc.Minute(), utc.Second(), utc.Nanosecond(), time.Local)
			osonAssertFunctionalTime(t, value, want, false)
			osonAssertJSONScalarString(t, serverText, utc.Format("2006-01-02T15:04:05.999999999"))
		default:
			t.Fatalf("unexpected SQL timestamp-tz conversion type=%q", sqlType)
		}
		t.Logf("SQL JSON_SCALAR(TIMESTAMP WITH TIME ZONE) returned native type %q; Go bind policy is tested separately", sqlType)
	})
	// A BC SQL date is valid database input, distinct from an invalid Go year.
	// Characterize support without inventing a BC-to-Go astronomical-year map.
	for i, expression := range []string{
		"json_scalar(to_date('0001-01-01 BC','YYYY-MM-DD BC'))",
		"json_scalar(to_timestamp('0001-01-01 01:02:03.123456789 BC','YYYY-MM-DD HH24:MI:SS.FF9 BC'))",
	} {
		t.Run(fmt.Sprintf("BCE_SQL_characterization_%d", i), func(t *testing.T) {
			var doc ojson.JSON
			if err := db.QueryRowContext(ctx, "select "+expression+" from dual").Scan(&doc); err != nil {
				t.Fatalf("fetch valid SQL BC date/timestamp: %v", err)
			}
			value, err := doc.GetValue()
			if err != nil {
				t.Fatalf("valid SQL BC temporal value cannot be materialized: %v; shared DATE decoder support gap, wire=%x", err, osonNumericDocumentBytes(t, doc))
			}
			v, ok := value.(time.Time)
			if !ok || v.Year() > 0 || v.Month() != time.January || v.Day() != 1 {
				t.Fatalf("SQL BC temporal value=%#v (%T), want a BC time.Time on January 1", value, value)
			}
			t.Logf("SQL 1 BC materializes as Go year %d; explicit era mapping must be documented", v.Year())
		})
	}
}

// OSON-F05 checks fields/precision and independent server rendering. Boundary
// dates with non-UTC offsets keep both local and UTC years in the SQL range.
func TestDriver_OSON_F05_TimeConversions(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, 5*time.Minute)
	zones := []struct {
		name   string
		offset int
	}{{"UTC", 0}, {"plus0530", 19800}, {"minus0330", -12600}, {"plus1400", 50400}, {"minus1200", -43200}}
	dates := []struct {
		name    string
		value   time.Time
		utcOnly bool
	}{
		{"new-year", time.Date(2000, 1, 1, 0, 0, 15, 123456789, time.UTC), false},
		{"leap-day", time.Date(2024, 2, 29, 23, 59, 59, 999999999, time.UTC), false},
		{"year-end", time.Date(2025, 12, 31, 23, 59, 59, 1, time.UTC), false},
		{"year0001-safe-UTC", time.Date(1, 1, 2, 14, 0, 0, 123456789, time.UTC), false},
		{"year9999-safe-UTC", time.Date(9999, 12, 30, 12, 0, 0, 123456789, time.UTC), false},
		{"first-CE-instant", time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"last-CE-second", time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC), true},
	}
	id := 0
	for _, zone := range zones {
		for _, date := range dates {
			if date.utcOnly && zone.offset != 0 {
				continue // These two cases are deliberately defined only in UTC.
			}
			for _, encoding := range osonFunctionalTimeEncodings() {
				id++
				t.Run(zone.name+"/"+date.name+"/"+encoding.name, func(t *testing.T) {
					v := date.value
					input := time.Date(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), time.FixedZone("source-name", zone.offset))
					doc, err := ojson.NewJSONWithOptions(input, ojson.Options{TimeEncoding: encoding.value})
					if err != nil {
						t.Fatalf("construct temporal scalar: %v", err)
					}
					if _, err = db.ExecContext(ctx, "insert into "+table+" (id,doc) values (:1,:2)", id, doc); err != nil {
						t.Fatalf("bind temporal scalar: %v", err)
					}
					var fetched ojson.JSON
					var serverText, serverType string
					if err = db.QueryRowContext(ctx, "select doc,json_serialize(doc returning clob),json_value(doc,'$.type()' returning varchar2(80) error on error) from "+table+" where id=:1", id).Scan(&fetched, &serverText, &serverType); err != nil {
						t.Fatalf("fetch temporal scalar and SQL reference: %v", err)
					}
					if serverType != encoding.sqlType {
						t.Errorf("SQL temporal type=%q, want %q", serverType, encoding.sqlType)
					}
					value, err := fetched.GetValue()
					if err != nil {
						t.Fatalf("materialize temporal scalar: %v", err)
					}
					actual, ok := value.(time.Time)
					if !ok {
						t.Fatalf("temporal type=%T, want time.Time", value)
					}
					want := input
					if encoding.value == ojson.TimeAsDate {
						want = want.Truncate(time.Second)
					}
					if osonFunctionalTimeFields(actual) != osonFunctionalTimeFields(want) {
						t.Errorf("calendar fields=%v, want %v", osonFunctionalTimeFields(actual), osonFunctionalTimeFields(want))
					}
					if encoding.value == ojson.TimeAsTimestampTZ {
						_, offset := actual.Zone()
						if !actual.Equal(input) || offset != zone.offset {
							t.Errorf("timestamp-tz instant equal=%v offset=%d, want offset=%d", actual.Equal(input), offset, zone.offset)
						}
					} else if actual.Location() != time.Local {
						t.Errorf("zone-free location=%v, want time.Local", actual.Location())
					}
					rendered, err := stdjson.Marshal(fetched)
					if err != nil {
						t.Fatalf("marshal temporal scalar: %v", err)
					}
					for name, text := range map[string]string{"server": serverText, "driver": string(rendered)} {
						var scalar string
						if err := stdjson.Unmarshal([]byte(text), &scalar); err != nil {
							t.Fatalf("%s temporal text is not a JSON string: %v", name, err)
						}
						if encoding.value == ojson.TimeAsTimestampTZ {
							parsed, err := time.Parse(time.RFC3339Nano, scalar)
							_, offset := parsed.Zone()
							if err != nil || !parsed.Equal(input) || offset != zone.offset {
								t.Errorf("%s temporal rendering=%q (%v), want instant/offset of %s", name, scalar, err, input.Format(time.RFC3339Nano))
							}
						} else if name == "server" {
							osonAssertZoneFreeServerText(t, text, want)
						} else if scalar != want.Format("2006-01-02T15:04:05.999999999") {
							t.Errorf("driver temporal rendering=%q, want %q", scalar, want.Format("2006-01-02T15:04:05.999999999"))
						}
					}
				})
			}
		}
	}
	// Exercise temporal payloads below both object and array roots. Root-only
	// round trips do not establish propagation through nested tree layouts.
	for _, zone := range zones {
		for _, encoding := range osonFunctionalTimeEncodings() {
			for _, objectRoot := range []bool{true, false} {
				id++
				t.Run(fmt.Sprintf("nested/%s/%s/object-root-%t", zone.name, encoding.name, objectRoot), func(t *testing.T) {
					input := time.Date(2024, 2, 29, 23, 59, 58, 123456789, time.FixedZone("nested-source", zone.offset))
					want := input
					if encoding.value == ojson.TimeAsDate {
						want = input.Truncate(time.Second)
					}
					wantText := want.Format("2006-01-02T15:04:05.999999999")
					if encoding.value == ojson.TimeAsTimestampTZ {
						wantText = want.Format(time.RFC3339Nano)
					}
					var source, renderedWant any
					paths := []string{"$.direct", "$.items[0]"}
					if objectRoot {
						source = map[string]any{"direct": input, "items": []any{input}}
						renderedWant = map[string]any{"direct": wantText, "items": []any{wantText}}
					} else {
						source = []any{input, map[string]any{"deeper": input}}
						renderedWant = []any{wantText, map[string]any{"deeper": wantText}}
						paths = []string{"$[0]", "$[1].deeper"}
					}
					doc, err := ojson.NewJSONWithOptions(source, ojson.Options{TimeEncoding: encoding.value})
					if err != nil {
						t.Fatalf("construct nested temporal payload: %v", err)
					}
					if _, err = db.ExecContext(ctx, "insert into "+table+" (id,doc) values (:1,:2)", id, doc); err != nil {
						t.Fatalf("bind nested temporal payload: %v", err)
					}
					var fetched ojson.JSON
					var firstType, secondType string
					query := "select doc,json_value(doc,'" + paths[0] + ".type()'),json_value(doc,'" + paths[1] + ".type()') from " + table + " where id=:1"
					if err = db.QueryRowContext(ctx, query, id).Scan(&fetched, &firstType, &secondType); err != nil {
						t.Fatalf("fetch nested temporal payload and SQL types: %v", err)
					}
					if firstType != encoding.sqlType || secondType != encoding.sqlType {
						t.Errorf("nested SQL types=(%q,%q), want %q", firstType, secondType, encoding.sqlType)
					}
					value, err := fetched.GetValue()
					if err != nil {
						t.Fatalf("materialize nested temporal payload: %v", err)
					}
					if objectRoot {
						object, ok := value.(map[string]any)
						if !ok {
							t.Fatalf("nested temporal object type=%T", value)
						}
						items, ok := object["items"].([]any)
						if !ok || len(items) != 1 {
							t.Fatalf("nested temporal items=%#v", object["items"])
						}
						osonAssertFunctionalTime(t, object["direct"], want, encoding.value == ojson.TimeAsTimestampTZ)
						osonAssertFunctionalTime(t, items[0], want, encoding.value == ojson.TimeAsTimestampTZ)
					} else {
						array, ok := value.([]any)
						if !ok || len(array) != 2 {
							t.Fatalf("nested temporal array=%#v", value)
						}
						object, ok := array[1].(map[string]any)
						if !ok {
							t.Fatalf("nested temporal object=%#v", array[1])
						}
						osonAssertFunctionalTime(t, array[0], want, encoding.value == ojson.TimeAsTimestampTZ)
						osonAssertFunctionalTime(t, object["deeper"], want, encoding.value == ojson.TimeAsTimestampTZ)
					}
					text, err := stdjson.Marshal(fetched)
					if err != nil {
						t.Fatalf("marshal nested temporal payload: %v", err)
					}
					expected, err := stdjson.Marshal(renderedWant)
					if err != nil {
						t.Fatalf("marshal nested temporal expectation: %v", err)
					}
					assertOSONSemanticJSON(t, string(text), string(expected))
				})
			}
		}
	}
	for _, year := range []int{0, 10000, 30000, -30000} {
		for _, encoding := range osonFunctionalTimeEncodings() {
			id++
			t.Run(fmt.Sprintf("unsupported-year-%d/%s", year, encoding.name), func(t *testing.T) {
				input := time.Date(year, 2, 3, 4, 5, 6, 123456789, time.UTC)
				doc, err := ojson.NewJSONWithOptions(input, ojson.Options{TimeEncoding: encoding.value})
				if err != nil {
					if !osonTemporalEncodingRejection(err) {
						t.Fatalf("unexpected constructor error for temporal input: %v", err)
					}
					t.Logf("unsupported year rejected at construction: %v", err)
					return
				}
				if _, err = db.ExecContext(ctx, "insert into "+table+" (id,doc) values (:1,:2)", id, doc); err != nil {
					if !osonTemporalDataRejection(err) {
						t.Fatalf("unexpected error for unsupported temporal input: %v", err)
					}
					var count int
					if err := db.QueryRowContext(ctx, "select count(*) from "+table+" where id=:1", id).Scan(&count); err != nil || count != 0 {
						t.Fatalf("rejected temporal insert count=%d (%v), want zero", count, err)
					}
					t.Log("unsupported year cleanly rejected by server")
					return
				}
				var fetched ojson.JSON
				if err = db.QueryRowContext(ctx, "select doc from "+table+" where id=:1", id).Scan(&fetched); err != nil {
					t.Fatalf("accepted temporal input cannot be fetched: %v", err)
				}
				value, err := fetched.GetValue()
				if err != nil {
					t.Fatalf("accepted/stored temporal input cannot be materialized: %v", err)
				}
				actual, ok := value.(time.Time)
				want := input
				if encoding.value == ojson.TimeAsDate {
					want = want.Truncate(time.Second)
				}
				if !ok || osonFunctionalTimeFields(actual) != osonFunctionalTimeFields(want) {
					t.Fatalf("accepted temporal input silently changed: got=%#v, wanted fields=%v", value, osonFunctionalTimeFields(want))
				}
				if year == 0 || year > 9999 || year < -4712 {
					t.Fatalf("database accepted unsupported SQL temporal year %d; expected clean range rejection even when local bytes retain the year", year)
				}
			})
		}
	}
	// User-selected policy: sub-minute zones may be rejected or normalized,
	// but an accepted timestamp must preserve its UTC instant. Offset seconds
	// lost during normalization are recorded, not classified as a failure.
	for _, offset := range []int{30, -30, 19830, -12630} {
		id++
		t.Run(fmt.Sprintf("seconds-offset-characterization/%d", offset), func(t *testing.T) {
			input := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.FixedZone("offset-seconds", offset))
			doc, err := ojson.NewJSONWithOptions(input, ojson.Options{TimeEncoding: ojson.TimeAsTimestampTZ})
			if err != nil {
				if !osonTemporalEncodingRejection(err) {
					t.Fatalf("unexpected constructor error for seconds offset: %v", err)
				}
				t.Logf("sub-minute offset rejected at construction: %v", err)
				return
			}
			if _, err = db.ExecContext(ctx, "insert into "+table+" (id,doc) values (:1,:2)", id, doc); err != nil {
				if !osonTemporalDataRejection(err) {
					t.Fatalf("unexpected sub-minute offset error: %v", err)
				}
				t.Log("sub-minute offset cleanly rejected by server")
				return
			}
			var fetched ojson.JSON
			var serverText string
			if err = db.QueryRowContext(ctx, "select doc,json_serialize(doc returning clob) from "+table+" where id=:1", id).Scan(&fetched, &serverText); err != nil {
				t.Fatalf("fetch accepted sub-minute timestamp: %v", err)
			}
			value, err := fetched.GetValue()
			if err != nil {
				t.Fatalf("materialize accepted sub-minute timestamp: %v", err)
			}
			actual, ok := value.(time.Time)
			if !ok || !actual.Equal(input) {
				t.Fatalf("sub-minute offset changed UTC instant: got=%#v, want=%s", value, input.UTC().Format(time.RFC3339Nano))
			}
			_, normalized := actual.Zone()
			var rendered string
			if err := stdjson.Unmarshal([]byte(serverText), &rendered); err != nil {
				t.Fatalf("SQL sub-minute timestamp reference is not a string: %v", err)
			}
			parsed, err := time.Parse(time.RFC3339Nano, rendered)
			if err != nil || !parsed.Equal(input) {
				t.Fatalf("SQL persisted sub-minute timestamp changed UTC instant: %q (%v)", rendered, err)
			}
			t.Logf("offset-seconds policy: requested=%d returned=%d lost=%d; UTC instant preserved", offset, normalized, offset-normalized)
		})
	}
}

func osonFunctionalNumberModes() []struct {
	name  string
	value ojson.NumberMode
} {
	return []struct {
		name  string
		value ojson.NumberMode
	}{{"default", ojson.NumberDefault}, {"JSONNumber", ojson.NumberAsJSONNumber}, {"Float64", ojson.NumberAsFloat64}}
}

func osonFunctionalTimeEncodings() []struct {
	name, sqlType string
	value         ojson.TimeEncoding
} {
	return []struct {
		name, sqlType string
		value         ojson.TimeEncoding
	}{{"timestamp", "timestamp", ojson.TimeAsTimestamp}, {"timestamp-tz", "timestamp with time zone", ojson.TimeAsTimestampTZ}, {"date", "date", ojson.TimeAsDate}}
}

func osonFunctionalTimeFields(v time.Time) [7]int {
	return [7]int{v.Year(), int(v.Month()), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond()}
}

func osonAssertFunctionalTime(t *testing.T, value any, want time.Time, withZone bool) {
	t.Helper()
	actual, ok := value.(time.Time)
	if !ok {
		t.Fatalf("temporal value type=%T, want time.Time", value)
	}
	if osonFunctionalTimeFields(actual) != osonFunctionalTimeFields(want) {
		t.Errorf("temporal fields=%v, want %v", osonFunctionalTimeFields(actual), osonFunctionalTimeFields(want))
	}
	if withZone {
		_, actualOffset := actual.Zone()
		_, wantOffset := want.Zone()
		if !actual.Equal(want) || actualOffset != wantOffset {
			t.Errorf("temporal instant equal=%v offset=%d, want offset=%d", actual.Equal(want), actualOffset, wantOffset)
		}
	} else if actual.Location() != time.Local {
		t.Errorf("zone-free temporal location=%v, want time.Local", actual.Location())
	}
}

func osonAssertZoneFreeServerText(t *testing.T, text string, want time.Time) {
	t.Helper()
	var rendered string
	if err := stdjson.Unmarshal([]byte(text), &rendered); err != nil {
		t.Fatalf("SQL temporal reference is not a JSON string: %v", err)
	}
	actual, err := time.Parse("2006-01-02T15:04:05.999999999", rendered)
	if err != nil || osonFunctionalTimeFields(actual) != osonFunctionalTimeFields(want) {
		t.Errorf("SQL temporal reference=%q (%v), want calendar fields %v", rendered, err, osonFunctionalTimeFields(want))
	}
}

func osonAssertSQLNonfiniteCategory(t *testing.T, doc *ojson.JSON, serverType, wantText string) {
	t.Helper()
	var expected float64
	switch wantText {
	case "Nan":
		expected = math.NaN()
	case "Inf":
		expected = math.Inf(1)
	case "-Inf":
		expected = math.Inf(-1)
	default:
		t.Fatalf("unexpected nonfinite fixture %q", wantText)
	}
	for _, mode := range osonFunctionalNumberModes() {
		doc.SetOptions(ojson.Options{NumberMode: mode.value})
		value, err := doc.GetValue()
		if err != nil {
			t.Fatalf("materialize SQL nonfinite category %q in %s mode: %v", serverType, mode.name, err)
		}
		if serverType == "string" {
			if actual, ok := value.(string); !ok || actual != wantText {
				t.Fatalf("SQL string nonfinite category decoded as %#v (%T), want string %q", value, value, wantText)
			}
		} else {
			// The independent SQL type selected the native numeric contract;
			// all number modes must preserve NaN/Inf as float64.
			osonAssertNumericMode(t, value, expected, wantText, 64, mode.value)
		}
	}
	t.Logf("SQL nonfinite category=%q; Go type/value agrees in all number modes", serverType)
}

func osonAssertServerIdentifierText(t *testing.T, text string, want []byte) {
	t.Helper()
	var rendered string
	if err := stdjson.Unmarshal([]byte(text), &rendered); err != nil {
		t.Fatalf("SQL identifier serialization is not a JSON string: %v", err)
	}
	// Oracle can serialize an OID as lowercase hex and a UUID using
	// lowercase hyphenated notation. Both identify the same literal bytes.
	actual, err := hex.DecodeString(strings.ReplaceAll(rendered, "-", ""))
	if err != nil || !bytes.Equal(actual, want) {
		t.Fatalf("SQL identifier rendering=%q (%v), want identifier bytes %x", rendered, err, want)
	}
}

func osonNumericDocumentBytes(t *testing.T, doc ojson.JSON) []byte {
	t.Helper()
	value, err := doc.Value()
	if err != nil {
		t.Fatalf("read OSON diagnostic bytes: %v", err)
	}
	data, ok := value.([]byte)
	if !ok {
		t.Fatalf("JSON bind representation=%T, want []byte", value)
	}
	return data
}

// This independent diagnostic reads only the fixed scalar header and opcode;
// it does not materialize a number through the codec under test.
func osonFunctionalRootScalarOpcode(data []byte) (byte, bool) {
	if len(data) < 9 || !bytes.Equal(data[:3], []byte{0xff, 0x4a, 0x5a}) {
		return 0, false
	}
	flags := binary.BigEndian.Uint16(data[4:6])
	if flags&0x0010 == 0 {
		return 0, false
	}
	root := 8
	if flags&0x1000 != 0 {
		root = 10
	}
	if root >= len(data) {
		return 0, false
	}
	return data[root], true
}

func osonAssertNumericMode(t *testing.T, got, wantDefault any, exact string, floatBits int, mode ojson.NumberMode) {
	t.Helper()
	if f, ok := wantDefault.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
		actual, ok := got.(float64)
		if !ok || !(math.IsNaN(f) && math.IsNaN(actual) || math.IsInf(f, 1) && math.IsInf(actual, 1) || math.IsInf(f, -1) && math.IsInf(actual, -1)) {
			t.Fatalf("special number=%#v (%T), want float64 %s", got, got, exact)
		}
		return
	}
	switch mode {
	case ojson.NumberAsJSONNumber:
		number, ok := got.(stdjson.Number)
		if !ok {
			t.Fatalf("numeric Go type=%T, want json.Number", got)
		}
		osonAssertExactNumericText(t, string(number), exact, floatBits, true)
	case ojson.NumberAsFloat64:
		want, err := strconv.ParseFloat(exact, 64)
		if floatBits != 0 {
			want, err = wantDefault.(float64), nil
		}
		if err != nil {
			t.Fatalf("invalid numeric expectation %q: %v", exact, err)
		}
		number, ok := got.(float64)
		if !ok || math.Float64bits(number) != math.Float64bits(want) {
			t.Fatalf("numeric value=%#v (%T), want float64 %v (signbit=%v)", got, got, want, math.Signbit(want))
		}
	default:
		if reflect.TypeOf(got) != reflect.TypeOf(wantDefault) {
			t.Fatalf("default Go type=%T, want %T per guide; inspect captured OSON for server normalization", got, wantDefault)
		}
		switch want := wantDefault.(type) {
		case stdjson.Number:
			osonAssertExactNumericText(t, string(got.(stdjson.Number)), exact, 0, true)
		case float64:
			if math.Float64bits(got.(float64)) != math.Float64bits(want) {
				t.Fatalf("binary numeric value=%v, want %v (signbit=%v)", got, want, math.Signbit(want))
			}
		default:
			if !reflect.DeepEqual(got, wantDefault) {
				t.Fatalf("numeric value=%#v, want %#v", got, wantDefault)
			}
		}
	}
}

func osonAssertExactNumericText(t *testing.T, text, want string, floatBits int, zeroSign bool) {
	t.Helper()
	if !stdjson.Valid([]byte(text)) {
		t.Fatalf("numeric JSON text is invalid: %q", text)
	}
	decoder := stdjson.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode numeric text %q: %v", text, err)
	}
	number, ok := value.(stdjson.Number)
	if !ok {
		t.Fatalf("numeric text %q decoded as %T, want a number", text, value)
	}
	if floatBits != 0 {
		actual, err := strconv.ParseFloat(string(number), floatBits)
		if err != nil {
			t.Fatalf("parse floating text %q: %v", number, err)
		}
		expected, err := strconv.ParseFloat(want, floatBits)
		if err != nil || actual != expected || zeroSign && actual == 0 && math.Signbit(actual) != math.Signbit(expected) {
			t.Fatalf("floating text=%q, want value of %q at %d bits (%v)", number, want, floatBits, err)
		}
		return
	}
	actual, ok := new(big.Rat).SetString(string(number))
	if !ok {
		t.Fatalf("parse exact numeric text %q", number)
	}
	expected, ok := new(big.Rat).SetString(want)
	if !ok || actual.Cmp(expected) != 0 {
		t.Fatalf("numeric text=%q, want exact value of %q", number, want)
	}
}

func osonAssertJSONScalarString(t *testing.T, text, want string) {
	t.Helper()
	var actual string
	if err := stdjson.Unmarshal([]byte(text), &actual); err != nil || actual != want {
		t.Fatalf("JSON scalar text=%q (%v), want string %q", text, err, want)
	}
}

func osonTemporalDataRejection(err error) bool {
	var coded oracleErrors.SQLError
	if !errors.As(err, &coded) {
		return false
	}
	code := coded.ErrorCode()
	return strings.HasPrefix(code, "ORA-018") || strings.HasPrefix(code, "ORA-404") || strings.HasPrefix(code, "ORA-405") || strings.HasPrefix(code, "ORA-408")
}

func osonTemporalEncodingRejection(err error) bool {
	var coded oracleErrors.SQLError
	return errors.As(err, &coded) && coded.ErrorCode() == string(oracleErrors.OsonEncodingError)
}

// F06: following columns and nullable receivers must survive alternating native
// documents, JSON null and SQL NULL on both sides of a text sentinel.
// TestDriver_OSON_F06_MixedRowsAndColumns verifies mixed JSON roots and NULL values preserve column alignment when scan destinations are reused.
func TestDriver_OSON_F06_MixedRowsAndColumns(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	if _, err := db.ExecContext(ctx, "alter table "+table+" add (doc2 json)"); err != nil {
		t.Fatalf("add second JSON column: %v", err)
	}
	cases := []struct {
		left, right           any
		leftWant, rightWant   any
		leftValid, rightValid bool
	}{
		{osonIntegrationJSON(t, map[string]any{"id": int64(1)}), osonIntegrationJSON(t, []any{"right"}), map[string]any{"id": stdjson.Number("1")}, []any{"right"}, true, true},
		{nil, osonIntegrationJSON(t, map[string]any{"side": "right"}), nil, map[string]any{"side": "right"}, false, true},
		{osonIntegrationJSON(t, nil), nil, nil, nil, true, false},
		{osonIntegrationJSON(t, []any{"after null", true}), osonIntegrationJSON(t, nil), []any{"after null", true}, nil, true, true},
		{osonIntegrationJSON(t, "last"), osonIntegrationJSON(t, false), "last", false, true, true},
	}
	for i, tc := range cases {
		if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc, doc2) values (:1, :2, :3)", i+1, tc.left, tc.right); err != nil {
			t.Fatalf("insert mixed row %d: %v", i+1, err)
		}
	}
	rows, err := db.QueryContext(ctx, "select id, doc, 'after-json', doc2, id+100 from "+table+" order by id")
	if err != nil {
		t.Fatalf("query mixed columns: %v", err)
	}
	defer rows.Close()
	var left, right sql.Null[ojson.JSON]
	count := 0
	for rows.Next() {
		if count == len(cases) {
			t.Fatal("mixed query returned extra rows")
		}
		var id, trailing int64
		var sentinel string
		left.V.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
		right.V.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
		if err := rows.Scan(&id, &left, &sentinel, &right, &trailing); err != nil {
			t.Fatalf("scan mixed row %d: %v", count+1, err)
		}
		if id != int64(count+1) || trailing != id+100 || sentinel != "after-json" {
			t.Fatalf("misaligned row: id=%d sentinel=%q trailing=%d", id, sentinel, trailing)
		}
		tc := cases[count]
		for _, side := range []struct {
			name  string
			doc   sql.Null[ojson.JSON]
			valid bool
			want  any
		}{{"left", left, tc.leftValid, tc.leftWant}, {"right", right, tc.rightValid, tc.rightWant}} {
			if side.doc.Valid != side.valid {
				t.Fatalf("row %d %s validity = %v, want %v", id, side.name, side.doc.Valid, side.valid)
			}
			if side.valid {
				osonIntegrationValue(t, side.doc.V, side.want)
			} else if _, err := side.doc.V.GetValue(); err == nil {
				t.Fatalf("row %d %s SQL NULL retained the previous document", id, side.name)
			}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate mixed rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close mixed rows: %v", err)
	}
	if count != len(cases) {
		t.Fatalf("mixed row count = %d, want %d", count, len(cases))
	}
	// This independent SQL check also distinguishes JSON null from SQL NULL.
	var sqlNulls, jsonNulls int64
	if err := db.QueryRowContext(ctx, "select sum(case when doc is null then 1 else 0 end), sum(case when json_value(doc, '$.type()') = 'null' then 1 else 0 end) from "+table).Scan(&sqlNulls, &jsonNulls); err != nil {
		t.Fatalf("SQL null classification: %v", err)
	}
	if sqlNulls != 1 || jsonNulls != 1 {
		t.Fatalf("SQL/JSON null counts = (%d, %d), want (1, 1)", sqlNulls, jsonNulls)
	}
}

// F07: whole-document binding keeps original native scalar types and bytes
// even when materialization options are subsequently replaced.
// TestDriver_OSON_F07_WholeDocumentRebind verifies option changes leave fetched wire bytes unchanged and whole-document rebinding preserves native values.
func TestDriver_OSON_F07_WholeDocumentRebind(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	wantTime := time.Date(2024, 12, 31, 23, 59, 58, 123456789, time.FixedZone("input", 19800))
	source, err := ojson.NewJSONWithOptions(map[string]any{
		"number": stdjson.Number("9007199254740993"), "binary": []byte{0, 1, 0xfe, 0xff}, "when": wantTime,
	}, ojson.Options{TimeEncoding: ojson.TimeAsTimestampTZ})
	if err != nil {
		t.Fatalf("construct native document: %v", err)
	}
	if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", source); err != nil {
		t.Fatalf("insert native document: %v", err)
	}
	var fetched ojson.JSON
	if err := db.QueryRowContext(ctx, "select doc from "+table+" where id=1").Scan(&fetched); err != nil {
		t.Fatalf("fetch native document: %v", err)
	}
	before := osonIntegrationBytes(t, fetched)
	fetched.SetOptions(ojson.Options{NumberMode: ojson.NumberAsFloat64, TimeEncoding: ojson.TimeAsDate})
	after := osonIntegrationBytes(t, fetched)
	if !bytes.Equal(before, after) {
		t.Fatal("whole-document Value bytes changed after SetOptions")
	}
	before[0] ^= 0xff
	if !bytes.Equal(osonIntegrationBytes(t, fetched), after) {
		t.Fatal("Value returned document-owned mutable storage")
	}
	if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (2, :1)", fetched); err != nil {
		t.Fatalf("rebind whole native document: %v", err)
	}
	var rebound ojson.JSON
	rebound.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	if err := db.QueryRowContext(ctx, "select doc from "+table+" where id=2").Scan(&rebound); err != nil {
		t.Fatalf("fetch rebound native document: %v", err)
	}
	value, err := rebound.GetValue()
	if err != nil {
		t.Fatalf("decode rebound native document: %v", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("rebound type = %T, want object", value)
	}
	if object["number"] != stdjson.Number("9007199254740993") || !reflect.DeepEqual(object["binary"], []byte{0, 1, 0xfe, 0xff}) {
		t.Fatalf("rebound number or binary changed: number=%#v binary=%#v", object["number"], object["binary"])
	}
	actualTime, ok := object["when"].(time.Time)
	if !ok {
		t.Fatalf("rebound temporal type = %T", object["when"])
	}
	_, offset := actualTime.Zone()
	if !actualTime.Equal(wantTime) || offset != 19800 || actualTime.Nanosecond() != 123456789 {
		t.Fatalf("rebound time = %v, want instant %v with offset +05:30", actualTime, wantTime)
	}
	var binaryType, timeType, number string
	if err := db.QueryRowContext(ctx, "select json_value(doc, '$.binary.type()'), json_value(doc, '$.when.type()'), json_value(doc, '$.number' returning varchar2(100) error on error) from "+table+" where id=2").Scan(&binaryType, &timeType, &number); err != nil {
		t.Fatalf("independent native type/value check: %v", err)
	}
	if binaryType != "binary" || timeType != "timestamp with time zone" || number != "9007199254740993" {
		t.Fatalf("SQL native types/value = (%q, %q, %q)", binaryType, timeType, number)
	}
	text, err := stdjson.Marshal(rebound)
	if err != nil {
		t.Fatalf("marshal rebound native document: %v", err)
	}
	assertOSONSemanticJSON(t, string(text), `{"number":9007199254740993,"binary":"0001FEFF","when":"2024-12-31T23:59:58.123456789+05:30"}`)
}

// F08: unlike whole documents, child binds materialize and re-encode with
// their inherited or explicitly replaced conversion options.
// TestDriver_OSON_F08_ChildRebind verifies child views re-encode with inherited or replaced options and preserve their expected standalone values.
func TestDriver_OSON_F08_ChildRebind(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	source, err := ojson.NewJSONWithOptions(map[string]any{
		"object": map[string]any{"number": stdjson.Number("9007199254740993")}, "array": []any{"x", true},
		"number": stdjson.Number("9007199254740993"), "binary": []byte{0, 0xff},
		"when": time.Date(2024, 2, 29, 23, 59, 58, 123456789, time.FixedZone("input", 19800)),
	}, ojson.Options{TimeEncoding: ojson.TimeAsTimestampTZ})
	if err != nil {
		t.Fatalf("construct child source: %v", err)
	}
	if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", source); err != nil {
		t.Fatalf("insert child source: %v", err)
	}
	var parent ojson.JSON
	parent.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	if err := db.QueryRowContext(ctx, "select doc from "+table+" where id=1").Scan(&parent); err != nil {
		t.Fatalf("fetch child source: %v", err)
	}
	object, err := parent.AsJSONObject()
	if err != nil {
		t.Fatalf("child source object view: %v", err)
	}
	cases := []struct {
		name, key, sqlType, text string
		options                  *ojson.Options
		want                     any
	}{
		{"object", "object", "object", `{"number":9007199254740993}`, nil, map[string]any{"number": stdjson.Number("9007199254740993")}},
		{"array", "array", "array", `["x",true]`, nil, []any{"x", true}},
		{"exact_number", "number", "number", `9007199254740993`, nil, stdjson.Number("9007199254740993")},
		{"rounded_number", "number", "double", `9007199254740992`, &ojson.Options{NumberMode: ojson.NumberAsFloat64}, float64(9007199254740992)},
		{"binary", "binary", "binary", `"00FF"`, nil, []byte{0, 0xff}},
		{"timestamp_default", "when", "timestamp", `"2024-02-29T23:59:58.123456789"`, nil, time.Date(2024, 2, 29, 23, 59, 58, 123456789, time.Local)},
		{"timestamp_tz", "when", "timestamp with time zone", `"2024-02-29T23:59:58.123456789+05:30"`, &ojson.Options{TimeEncoding: ojson.TimeAsTimestampTZ}, time.Date(2024, 2, 29, 23, 59, 58, 123456789, time.FixedZone("", 19800))},
		{"date", "when", "date", `"2024-02-29T23:59:58"`, &ojson.Options{TimeEncoding: ojson.TimeAsDate}, time.Date(2024, 2, 29, 23, 59, 58, 0, time.Local)},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child, err := object.Get(tc.key)
			if err != nil {
				t.Fatalf("get child: %v", err)
			}
			if tc.options != nil {
				child.SetOptions(*tc.options)
			}
			id := i + 10
			if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", id, child); err != nil {
				t.Fatalf("bind standalone child: %v", err)
			}
			var rebound ojson.JSON
			if err := db.QueryRowContext(ctx, "select doc from "+table+" where id=:1", id).Scan(&rebound); err != nil {
				t.Fatalf("fetch standalone child: %v", err)
			}
			if want, temporal := tc.want.(time.Time); temporal {
				value, err := rebound.GetValue()
				if err != nil {
					t.Fatalf("materialize temporal child: %v", err)
				}
				actual, ok := value.(time.Time)
				_, wantOffset := want.Zone()
				_, actualOffset := actual.Zone()
				if !ok || !actual.Equal(want) || actualOffset != wantOffset || actual.Nanosecond() != want.Nanosecond() {
					t.Fatalf("rebound temporal child = %#v, want %v", value, want)
				}
				if tc.sqlType != "timestamp with time zone" && actual.Location() != time.Local {
					t.Fatalf("zone-free child location = %v, want time.Local", actual.Location())
				}
			} else {
				osonIntegrationValue(t, rebound, tc.want)
			}
			text, err := stdjson.Marshal(rebound)
			if err != nil {
				t.Fatalf("marshal child: %v", err)
			}
			assertOSONSemanticJSON(t, string(text), tc.text)
			var kind string
			if err := db.QueryRowContext(ctx, "select json_value(doc, '$.type()') from "+table+" where id=:1", id).Scan(&kind); err != nil {
				t.Fatalf("independent child type check: %v", err)
			}
			if kind != tc.sqlType {
				t.Fatalf("child SQL type = %q, want %q", kind, tc.sqlType)
			}
		})
	}
	for i, tc := range []struct{ name, expression, want string }{
		{"year_month", "numtoyminterval(27, 'MONTH')", "02-03"},
		{"day_second", "numtodsinterval(1.5, 'SECOND')", "00 00:00:01.5"},
	} {
		t.Run(tc.name+"_becomes_string", func(t *testing.T) {
			var source ojson.JSON
			if err := db.QueryRowContext(ctx, "select json_object('value' value json_scalar("+tc.expression+") returning json) from dual").Scan(&source); err != nil {
				t.Fatalf("generate native interval source: %v", err)
			}
			view, err := source.AsJSONObject()
			if err != nil {
				t.Fatalf("native interval object: %v", err)
			}
			child, err := view.Get("value")
			if err != nil {
				t.Fatalf("native interval child: %v", err)
			}
			osonIntegrationValue(t, child, tc.want)
			id := i + 30
			if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", id, child); err != nil {
				t.Fatalf("bind materialized interval child: %v", err)
			}
			wantText, err := stdjson.Marshal(tc.want)
			if err != nil {
				t.Fatalf("marshal interval expectation: %v", err)
			}
			osonIntegrationStored(t, ctx, db, table, id, string(wantText))
			var kind string
			if err := db.QueryRowContext(ctx, "select json_value(doc, '$.type()') from "+table+" where id=:1", id).Scan(&kind); err != nil {
				t.Fatalf("interval child SQL type: %v", err)
			}
			if kind != "string" {
				t.Fatalf("interval child SQL type = %q, want string", kind)
			}
		})
	}
}

// F11: access errors are checked on fetched values. Options are recursive,
// survive Scan, and are captured by already obtained views and children.
// TestDriver_OSON_F11_AccessorsAndOptions verifies literal key access, bounds errors and option snapshots across scanning and lazy views.
func TestDriver_OSON_F11_AccessorsAndOptions(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	const input = `{"items":[7,9007199254740993,null],"number":9007199254740993,"a.b":"literal dot","items[0]":"literal brackets","null":null}`
	if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1)", ojson.NewJSONFromString(input)); err != nil {
		t.Fatalf("insert accessor source: %v", err)
	}
	var doc ojson.JSON
	doc.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	if err := db.QueryRowContext(ctx, "select doc from "+table+" where id=1").Scan(&doc); err != nil {
		t.Fatalf("fetch accessor source: %v", err)
	}
	oldObject, err := doc.AsJSONObject()
	if err != nil {
		t.Fatalf("root object view: %v", err)
	}
	keys := oldObject.Keys()
	slices.Sort(keys)
	if !reflect.DeepEqual(keys, []string{"a.b", "items", "items[0]", "null", "number"}) {
		t.Fatalf("object keys = %v", keys)
	}
	for _, tc := range []struct{ key, want string }{{"a.b", "literal dot"}, {"items[0]", "literal brackets"}} {
		child, err := oldObject.Get(tc.key)
		if err != nil {
			t.Fatalf("literal key %q: %v", tc.key, err)
		}
		osonIntegrationValue(t, child, tc.want)
	}
	items, err := oldObject.Get("items")
	if err != nil {
		t.Fatalf("items child: %v", err)
	}
	array, err := items.AsJSONArray()
	if err != nil || array.Len() != 3 {
		t.Fatalf("array length = (%d, %v), want three", array.Len(), err)
	}
	first, err := array.Get(0)
	if err != nil {
		t.Fatalf("first child: %v", err)
	}
	scalar, err := first.AsJSONScalar()
	if err != nil {
		t.Fatalf("scalar view: %v", err)
	}
	null, err := oldObject.Get("null")
	if err != nil {
		t.Fatalf("null child: %v", err)
	}
	for _, tc := range []struct {
		name  string
		value ojson.JSON
		kind  ojson.JSONKind
	}{
		{"object", doc, ojson.JSONObjectKind}, {"array", items, ojson.JSONArrayKind},
		{"scalar", first, ojson.JSONScalarKind}, {"null", null, ojson.JSONScalarKind},
	} {
		if kind, err := tc.value.Kind(); err != nil || kind != tc.kind {
			t.Fatalf("%s kind = (%v, %v), want %v", tc.name, kind, err, tc.kind)
		}
	}
	osonIntegrationValue(t, null, nil)
	_, err = oldObject.Get("missing")
	osonIntegrationError(t, err, oracleErrors.JSONObjectKeyNotFoundError)
	for _, index := range []int{-1, array.Len()} {
		_, err := array.Get(index)
		osonIntegrationError(t, err, oracleErrors.JSONArrayIndexOutOfRangeError)
	}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"object_as_array", func() error { _, err := doc.AsJSONArray(); return err }},
		{"object_as_scalar", func() error { _, err := doc.AsJSONScalar(); return err }},
		{"array_as_object", func() error { _, err := items.AsJSONObject(); return err }},
		{"array_as_scalar", func() error { _, err := items.AsJSONScalar(); return err }},
		{"scalar_as_object", func() error { _, err := first.AsJSONObject(); return err }},
		{"scalar_as_array", func() error { _, err := first.AsJSONArray(); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) { osonIntegrationError(t, tc.run(), oracleErrors.JSONAccessError) })
	}
	for _, tc := range []struct {
		name string
		view any
		want string
	}{
		{"document", doc, input}, {"object", oldObject, input},
		{"array", array, `[7,9007199254740993,null]`}, {"scalar", scalar, `7`},
	} {
		text, err := stdjson.Marshal(tc.view)
		if err != nil {
			t.Fatalf("marshal %s view: %v", tc.name, err)
		}
		assertOSONSemanticJSON(t, string(text), tc.want)
	}
	oldNumber, err := oldObject.Get("number")
	if err != nil {
		t.Fatalf("old number child: %v", err)
	}
	doc.SetOptions(ojson.Options{NumberMode: ojson.NumberAsFloat64})
	newObject, err := doc.AsJSONObject()
	if err != nil {
		t.Fatalf("new object view: %v", err)
	}
	newNumber, err := newObject.Get("number")
	if err != nil {
		t.Fatalf("new number child: %v", err)
	}
	osonIntegrationValue(t, oldNumber, stdjson.Number("9007199254740993"))
	osonIntegrationValue(t, newNumber, float64(9007199254740992))
	oldValue, err := oldObject.GetValue()
	if err != nil {
		t.Fatalf("old recursive materialization: %v", err)
	}
	newValue, err := newObject.GetValue()
	if err != nil {
		t.Fatalf("new recursive materialization: %v", err)
	}
	if !reflect.DeepEqual(oldValue["items"], []any{stdjson.Number("7"), stdjson.Number("9007199254740993"), nil}) ||
		!reflect.DeepEqual(newValue["items"], []any{float64(7), float64(9007199254740992), nil}) {
		t.Fatalf("recursive options not preserved: old=%#v new=%#v", oldValue["items"], newValue["items"])
	}
	text, err := stdjson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal after changing options: %v", err)
	}
	assertOSONSemanticJSON(t, string(text), input)
	if array.Len() != 3 {
		t.Fatalf("retained array length = %d, want three", array.Len())
	}
	retainedKeys := oldObject.Keys()
	slices.Sort(retainedKeys)
	if !reflect.DeepEqual(retainedKeys, keys) {
		t.Fatalf("retained object keys changed: %v", retainedKeys)
	}
	var stored string
	if err := db.QueryRowContext(ctx, "select json_serialize(doc returning varchar2(4000)) from "+table+" where id=1").Scan(&stored); err != nil {
		t.Fatalf("independent stored accessor document: %v", err)
	}
	assertOSONSemanticJSON(t, stored, input)
}

// F14: SQL generates the source independently; nested replacements grow and
// shrink while unrelated members and repeated object records remain intact.
// TestDriver_OSON_F14_ServerGenerationAndUpdates verifies SQL-generated and merge-patched documents retain unchanged siblings and remain navigable and rebindable.
func TestDriver_OSON_F14_ServerGenerationAndUpdates(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	query := "insert into " + table + " (id, doc) select 1, json_object(" +
		"'state' value 'before', 'nested' value json_object('keep' value 'yes', 'changed' value 'small' returning json), " +
		"'items' value json_array(json_object('id' value 1, 'name' value 'one' returning json), json_object('id' value 2, 'name' value 'two' returning json) returning json) returning json) from dual"
	if _, err := db.ExecContext(ctx, query); err != nil {
		t.Fatalf("SQL JSON generation: %v", err)
	}
	const initial = `{"state":"before","nested":{"keep":"yes","changed":"small"},"items":[{"id":1,"name":"one"},{"id":2,"name":"two"}]}`
	osonIntegrationStored(t, ctx, db, table, 1, initial)
	for i, replacement := range []string{strings.Repeat("long replacement ", 120), "tiny"} {
		t.Run(fmt.Sprintf("replacement_%d_bytes", len(replacement)), func(t *testing.T) {
			patch, err := stdjson.Marshal(map[string]any{"state": "after", "nested": map[string]any{"changed": replacement}})
			if err != nil {
				t.Fatalf("marshal merge patch: %v", err)
			}
			if _, err := db.ExecContext(ctx, "update "+table+" set doc=json_mergepatch(doc, :1 returning json) where id=1", ojson.NewJSONFromString(string(patch))); err != nil {
				t.Fatalf("server JSON update: %v", err)
			}
			want, err := stdjson.Marshal(map[string]any{"state": "after", "nested": map[string]any{"keep": "yes", "changed": replacement}, "items": []any{map[string]any{"id": 1, "name": "one"}, map[string]any{"id": 2, "name": "two"}}})
			if err != nil {
				t.Fatalf("marshal update expectation: %v", err)
			}
			fetched := osonIntegrationStored(t, ctx, db, table, 1, string(want))
			var changed, kept string
			if err := db.QueryRowContext(ctx, "select json_value(doc, '$.nested.changed' returning varchar2(4000) error on error), json_value(doc, '$.nested.keep' error on error) from "+table+" where id=1").Scan(&changed, &kept); err != nil {
				t.Fatalf("independent update extraction: %v", err)
			}
			if changed != replacement || kept != "yes" {
				t.Fatalf("SQL update changed=%q keep=%q", changed, kept)
			}
			if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", i+2, fetched); err != nil {
				t.Fatalf("rebind server-updated document: %v", err)
			}
			osonIntegrationStored(t, ctx, db, table, i+2, string(want))
		})
	}
}

// F15: the saved scalar children remain lazy until the rows and statement
// close; both full documents and children can then bind on another connection.
// TestDriver_OSON_F15_DocumentLifetime verifies retained documents and views survive scan reuse, row and statement closure, and connection reuse.
func TestDriver_OSON_F15_DocumentLifetime(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for id := 1; id <= 4; id++ {
		value := map[string]any{"id": int64(id), "name": fmt.Sprintf("saved-%d", id), "padding": strings.Repeat("x", id*64)}
		if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", id, osonIntegrationJSON(t, value)); err != nil {
			t.Fatalf("insert lifetime row %d: %v", id, err)
		}
	}
	statement, err := db.PrepareContext(ctx, "select doc from "+table+" order by id")
	if err != nil {
		t.Fatalf("prepare lifetime query: %v", err)
	}
	defer statement.Close()
	rows, err := statement.QueryContext(ctx)
	if err != nil {
		t.Fatalf("query lifetime rows: %v", err)
	}
	defer rows.Close()
	var documents, children []ojson.JSON
	var receiver ojson.JSON
	receiver.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	for rows.Next() {
		if len(documents) == 4 {
			t.Fatal("lifetime query returned extra rows")
		}
		if err := rows.Scan(&receiver); err != nil {
			t.Fatalf("scan lifetime row: %v", err)
		}
		documents = append(documents, receiver)
		view, err := receiver.AsJSONObject()
		if err != nil {
			t.Fatalf("lifetime object view: %v", err)
		}
		child, err := view.Get("name")
		if err != nil {
			t.Fatalf("lifetime name child: %v", err)
		}
		children = append(children, child)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate lifetime rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close lifetime rows: %v", err)
	}
	if err := statement.Close(); err != nil {
		t.Fatalf("close lifetime statement: %v", err)
	}
	if len(documents) != 4 {
		t.Fatalf("retained document count = %d, want four", len(documents))
	}
	var probe int64
	if err := db.QueryRowContext(ctx, "select 1 from dual").Scan(&probe); err != nil || probe != 1 {
		t.Fatalf("reuse single connection = (%d, %v)", probe, err)
	}
	// With all rows and statements closed, zero idle capacity closes the old
	// physical connection. Restore capacity for subsequent independent queries.
	db.SetMaxIdleConns(0)
	db.SetMaxIdleConns(1)
	for index, doc := range documents {
		id := index + 1
		name := fmt.Sprintf("saved-%d", id)
		want := map[string]any{"id": stdjson.Number(fmt.Sprint(id)), "name": name, "padding": strings.Repeat("x", id*64)}
		osonIntegrationValue(t, doc, want)
		osonIntegrationValue(t, children[index], name)
		text, err := stdjson.Marshal(want)
		if err != nil {
			t.Fatalf("marshal lifetime expectation: %v", err)
		}
		if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", id+100, doc); err != nil {
			t.Fatalf("rebind retained document %d: %v", id, err)
		}
		osonIntegrationStored(t, ctx, db, table, id+100, string(text))
		if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", id+200, children[index]); err != nil {
			t.Fatalf("rebind retained lazy child %d: %v", id, err)
		}
		nameText, err := stdjson.Marshal(name)
		if err != nil {
			t.Fatalf("marshal lifetime name expectation: %v", err)
		}
		osonIntegrationStored(t, ctx, db, table, id+200, string(nameText))
	}
}

// F16: prepared JSON statements are reused inside and outside transactions;
// rollback restores an existing document as well as removing a new insert.
// TestDriver_OSON_F16_Transactions verifies committed changes persist, rolled-back changes disappear and prepared statements remain usable.
func TestDriver_OSON_F16_Transactions(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	db.SetMaxOpenConns(1)
	insert, err := db.PrepareContext(ctx, "insert into "+table+" (id, doc) values (:id, :doc)")
	if err != nil {
		t.Fatalf("prepare transaction insert: %v", err)
	}
	defer insert.Close()
	update, err := db.PrepareContext(ctx, "update "+table+" set doc=:doc where id=:id")
	if err != nil {
		t.Fatalf("prepare transaction update: %v", err)
	}
	defer update.Close()
	t.Run("commit", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin commit transaction: %v", err)
		}
		defer tx.Rollback()
		txInsert := tx.StmtContext(ctx, insert)
		defer txInsert.Close()
		if _, err := txInsert.ExecContext(ctx, sql.Named("doc", osonIntegrationJSON(t, map[string]any{"state": "inserted", "keep": true})), sql.Named("id", 1)); err != nil {
			t.Fatalf("insert transaction document: %v", err)
		}
		txUpdate := tx.StmtContext(ctx, update)
		defer txUpdate.Close()
		if _, err := txUpdate.ExecContext(ctx, sql.Named("id", 1), sql.Named("doc", ojson.NewJSONFromString(`{"state":"committed","keep":true}`))); err != nil {
			t.Fatalf("update transaction document: %v", err)
		}
		osonIntegrationStored(t, ctx, tx, table, 1, `{"state":"committed","keep":true}`)
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit JSON transaction: %v", err)
		}
		osonIntegrationStored(t, ctx, db, table, 1, `{"state":"committed","keep":true}`)
	})
	t.Run("rollback_new_and_existing_rows", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin rollback transaction: %v", err)
		}
		defer tx.Rollback()
		txInsert := tx.StmtContext(ctx, insert)
		defer txInsert.Close()
		if _, err := txInsert.ExecContext(ctx, sql.Named("id", 2), sql.Named("doc", ojson.NewJSONFromString(`{"state":"rollback insert"}`))); err != nil {
			t.Fatalf("insert rollback document: %v", err)
		}
		txUpdate := tx.StmtContext(ctx, update)
		defer txUpdate.Close()
		if _, err := txUpdate.ExecContext(ctx, sql.Named("doc", osonIntegrationJSON(t, map[string]any{"state": "rollback update", "keep": false})), sql.Named("id", 1)); err != nil {
			t.Fatalf("update existing rollback document: %v", err)
		}
		osonIntegrationStored(t, ctx, tx, table, 1, `{"state":"rollback update","keep":false}`)
		osonIntegrationStored(t, ctx, tx, table, 2, `{"state":"rollback insert"}`)
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback JSON transaction: %v", err)
		}
		osonIntegrationStored(t, ctx, db, table, 1, `{"state":"committed","keep":true}`)
		var count int64
		if err := db.QueryRowContext(ctx, "select count(*) from "+table+" where id=2").Scan(&count); err != nil || count != 0 {
			t.Fatalf("rolled-back insert count = (%d, %v), want zero", count, err)
		}
	})
	if _, err := insert.ExecContext(ctx, sql.Named("doc", ojson.NewJSONFromString(`{"state":"after transactions"}`)), sql.Named("id", 3)); err != nil {
		t.Fatalf("reuse prepared insert after transactions: %v", err)
	}
	if _, err := update.ExecContext(ctx, sql.Named("id", 3), sql.Named("doc", osonIntegrationJSON(t, map[string]any{"state": "statement reused", "keep": true}))); err != nil {
		t.Fatalf("reuse prepared update after transactions: %v", err)
	}
	osonIntegrationStored(t, ctx, db, table, 3, `{"state":"statement reused","keep":true}`)
}

// F17: native JSON metadata describes the public JSON scanner even though
// generic scans receive OSON bytes; legacy text columns keep their own values.
// TestDriver_OSON_F17_ScanCompatibility verifies native JSON metadata and scan destinations alongside explicit serialization and legacy JSON storage.
func TestDriver_OSON_F17_ScanCompatibility(t *testing.T) {
	db, ctx, table := setupOSONTable(t)
	if _, err := db.ExecContext(ctx, "alter table "+table+" add (vdoc varchar2(4000), cdoc clob, bdoc blob)"); err != nil {
		t.Fatalf("add textual JSON columns: %v", err)
	}
	const text = ` {"score":42,"message":"two  spaces\nline"} `
	query := "insert into " + table + " (id,doc,vdoc,cdoc,bdoc) values (1,:doc,:text,to_clob(:clobtext),to_blob(utl_raw.cast_to_raw(:blobtext)))"
	if _, err := db.ExecContext(ctx, query, sql.Named("doc", ojson.NewJSONFromString(text)), sql.Named("text", text), sql.Named("clobtext", text), sql.Named("blobtext", text)); err != nil {
		t.Fatalf("insert native/textual representations: %v", err)
	}
	rows, err := db.QueryContext(ctx, "select doc,doc,json_serialize(doc returning varchar2(4000)),vdoc,cdoc,bdoc from "+table+" where id=1")
	if err != nil {
		t.Fatalf("query native/textual representations: %v", err)
	}
	defer rows.Close()
	columns, err := rows.ColumnTypes()
	if err != nil {
		t.Fatalf("read native scan metadata: %v", err)
	}
	if len(columns) != 6 {
		t.Fatalf("representation column count = %d, want six", len(columns))
	}
	for i := 0; i < 2; i++ {
		if columns[i].ScanType() != reflect.TypeFor[ojson.JSON]() || columns[i].DatabaseTypeName() != "JSON" {
			t.Fatalf("native column %d metadata: ScanType=%v DatabaseType=%s", i, columns[i].ScanType(), columns[i].DatabaseTypeName())
		}
	}
	if columns[4].DatabaseTypeName() != "CLOB" || columns[5].DatabaseTypeName() != "BLOB" {
		t.Fatalf("legacy LOB metadata: CLOB=%q BLOB=%q", columns[4].DatabaseTypeName(), columns[5].DatabaseTypeName())
	}
	if !rows.Next() {
		t.Fatalf("representation query returned no row: %v", rows.Err())
	}
	var raw any
	var binary, blob []byte
	var serialized, varchar, clob string
	if err := rows.Scan(&raw, &binary, &serialized, &varchar, &clob, &blob); err != nil {
		t.Fatalf("scan native/textual values: %v", err)
	}
	underlying, ok := raw.([]byte)
	if !ok || len(underlying) < 4 || !bytes.Equal(underlying[:3], []byte{0xff, 0x4a, 0x5a}) || !bytes.Equal(underlying, binary) {
		t.Fatalf("native generic scan = %T, want matching OSON byte slices", raw)
	}
	var parsed ojson.JSON
	parsed.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	if err := parsed.Scan(raw); err != nil {
		t.Fatalf("scan generic OSON bytes into JSON: %v", err)
	}
	osonIntegrationValue(t, parsed, map[string]any{"score": stdjson.Number("42"), "message": "two  spaces\nline"})
	assertOSONSemanticJSON(t, serialized, text)
	if varchar != text || clob != text || string(blob) != text {
		t.Fatalf("textual storage changed: VARCHAR2=%q CLOB=%q BLOB=%q", varchar, clob, blob)
	}
	if rows.Next() {
		t.Fatal("representation query returned an extra row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("finish representation query: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close representation rows: %v", err)
	}
	// A textual JSON column is not silently treated as an OSON column by Scan.
	if err := parsed.Scan(varchar); err == nil {
		t.Fatal("native JSON scanner accepted VARCHAR2 text as OSON")
	}
	if err := parsed.Scan(blob); err == nil {
		t.Fatal("native JSON scanner accepted textual BLOB bytes as OSON")
	}
}

func osonIntegrationJSON(t *testing.T, value any) ojson.JSON {
	t.Helper()
	doc, err := ojson.NewJSON(value)
	if err != nil {
		t.Fatalf("construct integration JSON: %v", err)
	}
	return doc
}

func osonIntegrationValue(t *testing.T, doc ojson.JSON, want any) {
	t.Helper()
	actual, err := doc.GetValue()
	if err != nil {
		t.Fatalf("materialize integration JSON: %v", err)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("JSON value = %#v (%T), want %#v (%T)", actual, actual, want, want)
	}
}

func osonIntegrationBytes(t *testing.T, doc ojson.JSON) []byte {
	t.Helper()
	value, err := doc.Value()
	if err != nil {
		t.Fatalf("obtain OSON bind bytes: %v", err)
	}
	data, ok := value.([]byte)
	if !ok || len(data) < 4 {
		t.Fatalf("OSON bind value = %T, want a document byte slice", value)
	}
	return data
}

func osonIntegrationError(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var coded oracleErrors.SQLError
	if err == nil || !errors.As(err, &coded) || coded.ErrorCode() != string(want) {
		t.Fatalf("access error = %v, want code %s", err, want)
	}
}

// Small standard JSON fixtures only: native binary and temporal scalar policies
// are checked separately instead of marshaling their extracted Go values.
type osonIntegrationQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func osonIntegrationStored(t *testing.T, ctx context.Context, db osonIntegrationQuerier, table string, id int, want string) ojson.JSON {
	t.Helper()
	var doc ojson.JSON
	doc.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})
	var serialized string
	if err := db.QueryRowContext(ctx, "select doc, json_serialize(doc returning varchar2(4000)) from "+table+" where id=:1", id).Scan(&doc, &serialized); err != nil {
		t.Fatalf("fetch stored JSON and SQL serialization for id %d: %v", id, err)
	}
	assertOSONSemanticJSON(t, serialized, want)
	value, err := doc.GetValue()
	if err != nil {
		t.Fatalf("materialize stored JSON for id %d: %v", id, err)
	}
	materializedText, err := stdjson.Marshal(value)
	if err != nil {
		t.Fatalf("marshal materialized standard JSON for id %d: %v", id, err)
	}
	assertOSONSemanticJSON(t, string(materializedText), want)
	text, err := stdjson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fetched JSON for id %d: %v", id, err)
	}
	assertOSONSemanticJSON(t, string(text), want)
	return doc
}

// TestDriver_OSON_F12_StringAndBinaryBoundaries verifies values, rather than
// just opcode selection, across scalar and tree length boundaries (OSON-F12).
func TestDriver_OSON_F12_StringAndBinaryBoundaries(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, 5*time.Minute)
	id := 0
	for _, length := range []int{0, 1, 30, 31, 32, 255, 256, 65532, 65533, 65535, 65536} {
		text := strings.Repeat("é🙂", length/6) + strings.Repeat("x", length%6)
		binaryValue := make([]byte, length)
		for i := range binaryValue {
			binaryValue[i] = byte(i*73 + 19)
		}
		for _, shape := range []string{"root", "nested"} {
			for _, scalar := range []struct {
				name  string
				value any
			}{{"string", text}, {"binary", binaryValue}} {
				id++
				t.Run(fmt.Sprintf("%s/%s/bytes_%d", shape, scalar.name, length), func(t *testing.T) {
					want := scalar.value
					if shape == "nested" {
						want = map[string]any{"payload": scalar.value, "unchanged": "sentinel"}
					}
					osonBoundaryRoundTrip(t, db, ctx, table, id, want)
				})
			}
		}
	}
	for _, value := range []string{"日本語 العربية e\u0301 🙂", " quote\" slash\\ line\n tab\t zero\x00 \r end ", "  keep  internal\n whitespace  "} {
		id++
		t.Run(fmt.Sprintf("unicode_and_escapes_%d", id), func(t *testing.T) {
			osonBoundaryRoundTrip(t, db, ctx, table, id, map[string]any{"a.b": value, "items[0]": []any{value}})
		})
	}
}

// TestDriver_OSON_F13_DictionaryAndContainerBoundaries exercises complete
// materialization across count, field-ID and dictionary heap widths (OSON-F13).
func TestDriver_OSON_F13_DictionaryAndContainerBoundaries(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, 5*time.Minute)
	id := 0
	for _, count := range []int{255, 256, 65535, 65536} {
		for _, shape := range []string{"array", "object"} {
			id++
			t.Run(fmt.Sprintf("%s/count_%d", shape, count), func(t *testing.T) {
				var value any
				if shape == "array" {
					values := make([]any, count)
					for i := range values {
						values[i] = strconv.Itoa(i)
					}
					value = values
				} else {
					values := make(map[string]any, count)
					for i := 0; i < count; i++ {
						values[fmt.Sprintf("field_%05d", i)] = strconv.Itoa(i)
					}
					value = values
				}
				osonBoundaryRoundTrip(t, db, ctx, table, id, value)
			})
		}
	}
	for _, length := range []int{255, 256} {
		id++
		t.Run(fmt.Sprintf("key_bytes_%d", length), func(t *testing.T) {
			key := strings.Repeat("é", length/2) + strings.Repeat("x", length%2)
			osonBoundaryRoundTrip(t, db, ctx, table, id, map[string]any{key: "long-key-value", "short": "sentinel"})
		})
	}
	for _, tier := range []string{"primary", "secondary"} {
		for _, heapSize := range []int{65534, 65535, 65536} {
			id++
			t.Run(fmt.Sprintf("%s/heap_bytes_%d", tier, heapSize), func(t *testing.T) {
				values := make(map[string]any)
				if tier == "primary" {
					// 255*(1+255)+(1+lastLength) = requested heap size.
					for i := 0; i < 256; i++ {
						length := 255
						if i == 255 {
							length = heapSize - 255*256 - 1
						}
						key := fmt.Sprintf("%03d_", i) + strings.Repeat("p", length-4)
						values[key] = strconv.Itoa(i)
					}
				} else {
					// 253*(2+256)+(2+lastLength) = requested heap size.
					for i := 0; i < 254; i++ {
						length := 256
						if i == 253 {
							length = heapSize - 253*258 - 2
						}
						key := fmt.Sprintf("%03d_", i) + strings.Repeat("s", length-4)
						values[key] = strconv.Itoa(i)
					}
				}
				osonBoundaryRoundTrip(t, db, ctx, table, id, values)
			})
		}
	}
	for _, count := range []int{255, 256} {
		id++
		t.Run(fmt.Sprintf("mixed_dictionaries/fields_%d", count), func(t *testing.T) {
			values := make(map[string]any, count)
			for i := 0; i < count; i++ {
				key := fmt.Sprintf("field_%03d", i)
				if i >= 127 {
					key += strings.Repeat("é", 128)
				}
				values[key] = strconv.Itoa(i)
			}
			osonBoundaryRoundTrip(t, db, ctx, table, id, values)
		})
	}
}

// TestDriver_OSON_F18_LimitsAndUnsupported verifies the encoded-size limit at
// the public bind boundary, including a maximum-size database round trip.
func TestDriver_OSON_F18_LimitsAndUnsupported(t *testing.T) {
	db, _, table := setupOSONTable(t)
	ctx := osonTestContext(t, 5*time.Minute)
	const maximum = 32 * 1024 * 1024
	const largeScalarOverhead = 15 // magic/flags/tree size (10), opcode/length (5).
	t.Run("maximum_encoded_document", func(t *testing.T) {
		want := strings.Repeat("x", maximum-largeScalarOverhead)
		doc, err := ojson.NewJSON(want)
		if err != nil {
			t.Fatalf("construct maximum document: %v", err)
		}
		raw, err := doc.Value()
		if err != nil {
			t.Fatalf("get encoded maximum document: %v", err)
		}
		encoded, ok := raw.([]byte)
		if !ok || len(encoded) != maximum {
			t.Fatalf("encoded value type=%T length=%d, want %d", raw, len(encoded), maximum)
		}
		if _, err = db.ExecContext(ctx, "insert into "+table+" (id,doc) values (1,:1)", doc); err != nil {
			t.Fatalf("bind maximum document: %v", err)
		}
		var fetched ojson.JSON
		var sentinel int
		if err = db.QueryRowContext(ctx, "select doc, id+100 from "+table+" where id=1").Scan(&fetched, &sentinel); err != nil {
			t.Fatalf("fetch maximum document: %v", err)
		}
		value, err := fetched.GetValue()
		if err != nil {
			t.Fatalf("materialize maximum document: %v", err)
		}
		got, ok := value.(string)
		if !ok || len(got) != len(want) || sha256.Sum256([]byte(got)) != sha256.Sum256([]byte(want)) || sentinel != 101 {
			t.Fatalf("maximum document mismatch: type=%T got bytes=%d want bytes=%d sentinel=%d", value, len(got), len(want), sentinel)
		}
	})
	t.Run("encoded_document_one_byte_over", func(t *testing.T) {
		_, err := ojson.NewJSON(strings.Repeat("x", maximum-largeScalarOverhead+1))
		requireOSONBoundaryError(t, err, oracleErrors.OsonEncodingError)
	})
	type definedString string
	type definedBytes []byte
	var nilPointer *int
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	for _, value := range []struct {
		name  string
		value any
	}{
		{"struct", struct{ Value string }{"x"}}, {"pointer", new(int)}, {"nil_pointer", nilPointer},
		{"typed_slice", []string{"x"}}, {"typed_map", map[string]int{"x": 1}}, {"non_string_keys", map[int]any{1: "x"}},
		{"defined_string", definedString("x")}, {"defined_bytes", definedBytes{1, 2}}, {"raw_message", stdjson.RawMessage(`{"x":1}`)},
		{"array", [1]string{"x"}}, {"nested_unsupported", map[string]any{"x": []any{new(int)}}}, {"cycle", cyclic},
		{"number_text_256", stdjson.Number("1." + strings.Repeat("0", 254))},
	} {
		t.Run(value.name, func(t *testing.T) {
			_, err := ojson.NewJSON(value.value)
			requireOSONBoundaryError(t, err, oracleErrors.OsonEncodingError)
		})
	}
	t.Run("aliases_are_supported", func(t *testing.T) {
		type stringAlias = string
		type bytesAlias = []byte
		osonBoundaryRoundTrip(t, db, ctx, table, 2, map[string]any{"text": stringAlias("oracle"), "bytes": bytesAlias{0, 255}})
	})
	t.Run("number_text_255", func(t *testing.T) {
		want := stdjson.Number("1." + strings.Repeat("0", 253))
		doc, err := ojson.NewJSON(want)
		if err != nil {
			t.Fatalf("construct 255-byte number: %v", err)
		}
		got, err := doc.GetValue()
		if err != nil || got != want {
			t.Fatalf("255-byte number value=%v error=%v", got, err)
		}
	})
}

func osonBoundaryRoundTrip(t *testing.T, db *sql.DB, ctx context.Context, table string, id int, want any) {
	t.Helper()
	doc, err := ojson.NewJSON(want)
	if err != nil {
		t.Fatalf("construct %T: %v", want, err)
	}
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id,doc) values (:1,:2)", id, doc); err != nil {
		t.Fatalf("bind %T: %v", want, err)
	}
	var got ojson.JSON
	var sentinel int
	var serverText string
	if err = db.QueryRowContext(ctx, "select doc,id+100,json_serialize(doc returning clob error on error) from "+table+" where id=:1", id).Scan(&got, &sentinel, &serverText); err != nil {
		t.Fatalf("fetch %T: %v", want, err)
	}
	value, err := got.GetValue()
	if err != nil {
		t.Fatalf("materialize %T: %v", want, err)
	}
	if sentinel != id+100 || !osonBoundaryValueEqual(value, want) {
		t.Fatalf("boundary mismatch: got type=%T want type=%T sentinel=%d want=%d", value, want, sentinel, id+100)
	}
	rendered, err := stdjson.Marshal(got)
	if err != nil {
		t.Fatalf("render fetched %T: %v", want, err)
	}
	expected, err := stdjson.Marshal(osonBoundaryRenderedValue(want))
	if err != nil {
		t.Fatalf("render expected %T: %v", want, err)
	}
	assertOSONSemanticJSON(t, string(rendered), string(expected))
	assertOSONSemanticJSON(t, serverText, string(expected))
}

func osonBoundaryValueEqual(got, want any) bool {
	switch expected := want.(type) {
	case []byte:
		value, ok := got.([]byte)
		return ok && bytes.Equal(value, expected)
	case map[string]any:
		value, ok := got.(map[string]any)
		if !ok || len(value) != len(expected) {
			return false
		}
		for key, item := range expected {
			other, present := value[key]
			if !present || !osonBoundaryValueEqual(other, item) {
				return false
			}
		}
		return true
	case []any:
		value, ok := got.([]any)
		if !ok || len(value) != len(expected) {
			return false
		}
		for i, item := range expected {
			if !osonBoundaryValueEqual(value[i], item) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}

func osonBoundaryRenderedValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return fmt.Sprintf("%X", v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = osonBoundaryRenderedValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = osonBoundaryRenderedValue(item)
		}
		return out
	default:
		return value
	}
}

func requireOSONBoundaryError(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var coded interface{ ErrorCode() string }
	if err == nil || !errors.As(err, &coded) || coded.ErrorCode() != string(want) {
		t.Fatalf("error=%v, want code %s", err, want)
	}
}

func osonTestContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// openOSONTestDB makes an explicitly selected but unusable environment a
// failure, rather than allowing a successful run consisting entirely of skips.
func openOSONTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if TestingConfig == nil {
		if oracleTest.ConfigName == "" && oracleTest.ConfigFileName == "" {
			t.Skip("native JSON functional test requires an explicitly selected database configuration")
		}
		_, err := TestEnvironement.GetConfig(oracleTest.ConfigName)
		t.Fatalf("cannot load selected OSON configuration %q: %v; set -driver.config.filename and -driver.config.name", oracleTest.ConfigName, err)
	}
	if !TestingConfig.Enabled {
		t.Fatalf("selected OSON configuration %q is disabled", TestingConfig.ConfigName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := openTestDBWithConfigContext(ctx, TestingConfig)
	if err != nil {
		t.Fatalf("connect using selected OSON configuration %q: %v", TestingConfig.ConfigName, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close OSON test database: %v", err)
		}
	})
	var version string
	if err := db.QueryRowContext(ctx, "SELECT version_full FROM product_component_version WHERE product LIKE 'Oracle%Database%'").Scan(&version); err != nil {
		t.Fatalf("detect actual database version: %v", err)
	}
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		t.Fatalf("unexpected database version %q", version)
	}
	var actual Version
	for i, dst := range []*int{&actual.Major, &actual.Minor, &actual.Micro} {
		value, err := strconv.Atoi(parts[i])
		if err != nil {
			t.Fatalf("parse database version %q: %v", version, err)
		}
		*dst = value
	}
	t.Logf("OSON environment: config=%s actual_database_version=%s", TestingConfig.ConfigName, version)
	configured := TestingConfig.DatabaseVersion
	if configured.Major == 0 {
		t.Fatalf("configuration %q needs database_version (JSON key uses underscores), e.g. %q for this server", TestingConfig.ConfigName, fmt.Sprintf("%d.%d.%d", actual.Major, actual.Minor, actual.Micro))
	}
	if configured != actual {
		t.Fatalf("configured database_version=%d.%d.%d disagrees with actual database version %s", configured.Major, configured.Minor, configured.Micro, version)
	}
	if actual.Major < 21 {
		t.Skipf("native JSON requires Oracle Database 21c or later; actual database version=%s", version)
	}
	return db
}

func assertOSONErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var coded oracleErrors.SQLError
	if !errors.As(err, &coded) || coded.ErrorCode() != string(want) {
		t.Fatalf("error = %v, want error code %s", err, want)
	}
}

// assertOSONSemanticJSON permits equivalent decimal/exponent spellings, but
// never compares exact JSON numbers by converting them to floating point.
func assertOSONSemanticJSON(t *testing.T, got, want string) {
	t.Helper()
	decode := func(text string) any {
		decoder := stdjson.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatalf("decode JSON assertion input (%d bytes): %v", len(text), err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			t.Fatalf("JSON assertion input has trailing data: %v", err)
		}
		return value
	}
	if !osonSemanticEqual(decode(got), decode(want)) {
		t.Fatalf("JSON mismatch: got %s; want %s", osonBoundedText(got), osonBoundedText(want))
	}
}

func osonBoundedText(s string) string {
	if len(s) > 512 {
		return fmt.Sprintf("%s... (%d bytes)", s[:512], len(s))
	}
	return s
}

func osonSemanticEqual(got, want any) bool {
	switch expected := want.(type) {
	case stdjson.Number:
		actual, ok := got.(stdjson.Number)
		if !ok {
			return false
		}
		a, aOK := new(big.Rat).SetString(actual.String())
		b, bOK := new(big.Rat).SetString(expected.String())
		return aOK && bOK && a.Cmp(b) == 0
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok || len(actual) != len(expected) {
			return false
		}
		for key, value := range expected {
			candidate, exists := actual[key]
			if !exists || !osonSemanticEqual(candidate, value) {
				return false
			}
		}
		return true
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(expected) {
			return false
		}
		for i, value := range expected {
			if !osonSemanticEqual(actual[i], value) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}
