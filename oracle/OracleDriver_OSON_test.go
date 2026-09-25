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
	"context"
	"database/sql"
	stdjson "encoding/json"
	"reflect"
	"testing"
	"time"

	ojson "github.com/oracle/go-oracledb/v26/oracle/json"
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
	if err = rebound.SetOptions(ojson.NumberModeOption(ojson.NumberAsJSONNumber)); err != nil {
		t.Fatalf("set number option failed: %v", err)
	}
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
		{id: 1, name: "timestamp", opts: ojson.TimeEncodingOption(ojson.TimeAsTimestamp), want: `"2025-02-03T04:05:06.123456789"`},
		{id: 2, name: "timestamp with time zone", opts: ojson.TimeEncodingOption(ojson.TimeAsTimestampTZ), want: `"2025-02-03T04:05:06.123456789+02:00"`},
		{id: 3, name: "date", opts: ojson.TimeEncodingOption(ojson.TimeAsDate), want: `"2025-02-03T04:05:06"`},
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
	if err = got.SetOptions(ojson.NumberModeOption(ojson.NumberAsJSONNumber)); err != nil {
		t.Fatalf("set JSON number option failed: %v", err)
	}
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
	items, ok := object.Get("items")
	if !ok {
		t.Fatal("fetched object does not contain items")
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
	if _, err = db.ExecContext(ctx, "insert into "+table+" (id, doc) values (1, :1), (2, null)", jsonNull); err != nil {
		t.Fatalf("insert JSON and SQL null values failed: %v", err)
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
	if TestingConfig == nil {
		t.Skip("no database configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("native JSON requires Oracle Database 21c or later")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open database failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	// A unique name allows the functional category to run tests concurrently.
	table := createObjectName("t_oson")
	if err = createTable(ctx, db, table, map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"doc": "JSON",
	}); err != nil {
		t.Fatalf("create native JSON table failed: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })
	return db, ctx, table
}
