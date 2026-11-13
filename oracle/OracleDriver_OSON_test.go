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
	"database/sql"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

// setupOSONBoundaryTable creates and cleans up a native JSON table using the
// configured database version to determine whether native JSON is supported.
// Missing optional configuration skips; an unusable explicit selection fails.
func setupOSONBoundaryTable(t *testing.T) (*sql.DB, context.Context, string) {
	t.Helper()
	if TestingConfig == nil {
		if oracleTest.ConfigName == "" && oracleTest.ConfigFileName == "" {
			t.Skip("native JSON functional test requires an explicitly selected database configuration")
		}
		t.Fatalf("cannot load selected OSON configuration %q; set -driver.config.filename and -driver.config.name", oracleTest.ConfigName)
	}
	if !TestingConfig.Enabled {
		t.Fatalf("selected OSON configuration %q is disabled", TestingConfig.ConfigName)
	}
	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("create selected OSON connector: %v", err)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close OSON test database: %v", err)
		}
	})
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer probeCancel()
	if err := db.PingContext(probeCtx); err != nil {
		t.Fatalf("connect using selected OSON configuration: %v", err)
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("native JSON requires Oracle Database 21c or later")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	table := createObjectName("t_oson")
	if err := createTable(ctx, db, table, map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"doc": "JSON",
	}); err != nil {
		t.Fatalf("create native JSON table failed: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := dropTable(cleanupCtx, db, table); err != nil {
			t.Errorf("drop OSON test table %s: %v", table, err)
		}
	})
	return db, ctx, table
}

// TestDriver_OSON_F12_StringAndBinaryBoundaries verifies values, rather than
// just opcode selection, across scalar and tree length boundaries (OSON-F12).
func TestDriver_OSON_F12_StringAndBinaryBoundaries(t *testing.T) {
	db, ctx, table := setupOSONBoundaryTable(t)
	var charset string
	if err := db.QueryRowContext(ctx, "SELECT value FROM nls_database_parameters WHERE parameter='NLS_CHARACTERSET'").Scan(&charset); err != nil {
		t.Fatalf("read database character set: %v", err)
	}
	t.Logf("OSON Unicode reference: database_character_set=%s, reference=AL32UTF8 BLOB", charset)
	t.Run("sql_generated_unicode_reference", func(t *testing.T) {
		want := map[string]any{"text": "é🙂 日本語 العربية e\u0301"}
		// ASCII-only input keeps client-side text conversion out of this control.
		const query = `SELECT doc, JSON_SERIALIZE(doc RETURNING BLOB ERROR ON ERROR)
FROM (SELECT JSON('{"text":"\u00e9\ud83d\ude42 \u65e5\u672c\u8a9e \u0627\u0644\u0639\u0631\u0628\u064a\u0629 e\u0301"}') doc FROM DUAL)`
		var doc ojson.JSON
		var reference []byte
		if err := db.QueryRowContext(ctx, query).Scan(&doc, &reference); err != nil {
			t.Fatalf("fetch SQL-generated Unicode control: %v", err)
		}
		value, err := doc.GetValue()
		if err != nil || !osonBoundaryValueEqual(value, want) {
			t.Fatalf("SQL-generated Unicode value mismatch: value=%v error=%v", value, err)
		}
		rendered, err := stdjson.Marshal(doc)
		if err != nil {
			t.Fatalf("render SQL-generated Unicode: %v", err)
		}
		expected, err := stdjson.Marshal(want)
		if err != nil {
			t.Fatalf("render Unicode control expectation: %v", err)
		}
		if !utf8.Valid(reference) {
			t.Fatal("SQL-generated BLOB reference is not valid UTF-8")
		}
		assertOSONSemanticJSON(t, string(rendered), string(expected))
		assertOSONSemanticJSON(t, string(reference), string(expected))
	})
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
	db, ctx, table := setupOSONBoundaryTable(t)
	major := TestingConfig.DatabaseVersion.Major
	if major == 21 && !t.Run("sql_field_name_limit_control", func(t *testing.T) {
		osonCheck21cFieldNameLimit(t, db, ctx)
	}) {
		t.Fatal("independent SQL field-name controls failed; native-bind rejections cannot be assessed")
	}
	roundTrip := func(t *testing.T, id int, want any) {
		t.Helper()
		// Decide the expectation before executing the insert, never from its error.
		if osonBoundaryExpectsLongKeyRejection(major, want) {
			osonBoundaryRejectLongKeys(t, db, ctx, table, id, want)
			return
		}
		osonBoundaryRoundTrip(t, db, ctx, table, id, want)
	}
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
				roundTrip(t, id, value)
			})
		}
	}
	for _, length := range []int{255, 256} {
		id++
		t.Run(fmt.Sprintf("key_bytes_%d", length), func(t *testing.T) {
			key := strings.Repeat("é", length/2) + strings.Repeat("x", length%2)
			roundTrip(t, id, map[string]any{key: "long-key-value", "short": "sentinel"})
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
				roundTrip(t, id, values)
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
			roundTrip(t, id, values)
		})
	}
}

// osonBoundaryMaxFieldNameBytes returns the largest UTF-8 key length in the
// object and array fixtures accepted by the boundary tests.
func osonBoundaryMaxFieldNameBytes(value any) int {
	maximum := 0
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			maximum = max(maximum, len(key), osonBoundaryMaxFieldNameBytes(child))
		}
	case []any:
		for _, child := range value {
			maximum = max(maximum, osonBoundaryMaxFieldNameBytes(child))
		}
	}
	return maximum
}

// osonBoundaryExpectsLongKeyRejection classifies fixtures before database I/O.
// The native-JSON environment check handles unsupported database versions first.
func osonBoundaryExpectsLongKeyRejection(major int, value any) bool {
	return major == 21 && osonBoundaryMaxFieldNameBytes(value) > 255
}

// osonLongFieldDatabaseError accepts the two observed native-bind rejections.
// Callers must first classify a 21c overlong key and verify the SQL limit controls.
func osonLongFieldDatabaseError(err error) bool {
	var coded oracleErrors.SQLError
	return errors.As(err, &coded) && (coded.ErrorCode() == "ORA-40441" || coded.ErrorCode() == "ORA-40476")
}

// osonCheck21cFieldNameLimit verifies the 255/256-byte boundary using ASCII SQL
// input, independently of native OSON binds, and fails t on any unexpected result.
func osonCheck21cFieldNameLimit(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	for _, length := range []int{255, 256} {
		key := strings.Repeat("k", length)
		// Controlled ASCII literals avoid input-side charset conversion and OSON binds.
		query := `SELECT JSON_SERIALIZE(JSON('{"` + key + `":"synthetic-value"}') RETURNING BLOB ERROR ON ERROR) FROM DUAL`
		var reference []byte
		err := db.QueryRowContext(ctx, query).Scan(&reference)
		if length == 256 {
			var coded oracleErrors.SQLError
			if err == nil || !errors.As(err, &coded) || coded.ErrorCode() != "ORA-40476" {
				t.Fatalf("21c SQL 256-byte field control: error=%v, want ORA-40476", err)
			}
			t.Log("SQL control: 256-byte field rejected with ORA-40476")
			continue
		}
		if err != nil {
			t.Fatalf("21c SQL 255-byte field control: %v", err)
		}
		if !utf8.Valid(reference) {
			t.Fatal("SQL field-name reference is not valid UTF-8")
		}
		expected, err := stdjson.Marshal(map[string]any{key: "synthetic-value"})
		if err != nil {
			t.Fatalf("render SQL field-name expectation: %v", err)
		}
		assertOSONSemanticJSON(t, string(reference), string(expected))
		t.Log("SQL control: 255-byte field accepted and preserved")
	}
}

// osonBoundaryRejectLongKeys requires the expected 21c rejection, no inserted
// row and a healthy connection; the caller must have verified the SQL controls.
func osonBoundaryRejectLongKeys(t *testing.T, db *sql.DB, ctx context.Context, table string, id int, want any) {
	t.Helper()
	doc, err := ojson.NewJSON(want)
	if err != nil {
		t.Fatalf("construct long-field document: %v", err)
	}
	value, err := doc.GetValue()
	if err != nil || !osonBoundaryValueEqual(value, want) {
		t.Fatalf("local long-field materialization changed the fixture: error=%v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire connection for expected field-name rejection: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("release field-name control connection: %v", err)
		}
	}()
	_, err = conn.ExecContext(ctx, "insert into "+table+" (id,doc) values (:1,:2)", id, doc)
	if !osonLongFieldDatabaseError(err) {
		t.Fatalf("21c field-name rejection: maximum_key_bytes=%d error=%v, want ORA-40441 or ORA-40476", osonBoundaryMaxFieldNameBytes(want), err)
	}
	var count, control int
	if countErr := conn.QueryRowContext(ctx, "select count(*) from "+table+" where id=:1", id).Scan(&count); countErr != nil || count != 0 {
		t.Fatalf("expected rejected insert to leave no row: count=%d error=%v", count, countErr)
	}
	if controlErr := conn.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&control); controlErr != nil || control != 1 {
		t.Fatalf("same-connection recovery failed: result=%d error=%v", control, controlErr)
	}
	t.Logf("Expected 21c long-field rejection: maximum_key_bytes=%d error=%v; no row inserted; same-connection SELECT 1 passed", osonBoundaryMaxFieldNameBytes(want), err)
}

// TestOSONBoundaryFieldNamePolicy checks byte-based fixture expectations across
// database versions without contacting a database.
func TestOSONBoundaryFieldNamePolicy(t *testing.T) {
	large := make(map[string]any, 1024)
	for i := 0; i < 1024; i++ {
		large[fmt.Sprintf("field_%04d", i)] = strings.Repeat("v", 16)
	}
	for _, test := range []struct {
		name    string
		value   any
		maximum int
	}{
		{"ascii_255", map[string]any{strings.Repeat("a", 255): "value"}, 255},
		{"ascii_256", map[string]any{strings.Repeat("a", 256): "value"}, 256},
		{"utf8_255", map[string]any{strings.Repeat("é", 127) + "x": "value"}, 255},
		{"utf8_256", map[string]any{strings.Repeat("é", 128): "value"}, 256},
		{"nested_object", map[string]any{"short": map[string]any{strings.Repeat("é", 128): "value"}}, 256},
		{"object_in_array", []any{"scalar", map[string]any{strings.Repeat("a", 256): "value"}}, 256},
		{"large_short_key_object", large, 10},
		{"long_scalar_is_not_a_key", map[string]any{"short": strings.Repeat("x", 65536)}, 5},
		{"binary_is_not_a_container", make([]byte, 65536), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := osonBoundaryMaxFieldNameBytes(test.value); got != test.maximum {
				t.Fatalf("maximum key bytes=%d, want %d", got, test.maximum)
			}
			for _, major := range []int{21, 23, 26} {
				want := major == 21 && test.maximum > 255
				if got := osonBoundaryExpectsLongKeyRejection(major, test.value); got != want {
					t.Fatalf("database major=%d rejection=%v, want %v", major, got, want)
				}
			}
		})
	}
}

// osonBoundaryRoundTrip checks native values, the sentinel and both JSON renderings.
// A BLOB reference uses AL32UTF8 instead of the database character set.
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
	var serverText []byte
	if err = db.QueryRowContext(ctx, "select doc,id+100,json_serialize(doc returning blob error on error) from "+table+" where id=:1", id).Scan(&got, &sentinel, &serverText); err != nil {
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
	if !utf8.Valid(serverText) {
		t.Fatalf("SQL BLOB reference for %T is not valid UTF-8", want)
	}
	assertOSONSemanticJSON(t, string(serverText), string(expected))
}

// osonBoundaryValueEqual compares materialized fixture types and contents,
// retaining byte slices as binary values rather than treating them as arrays.
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

// osonBoundaryRenderedValue converts binary fixtures to Oracle JSON hexadecimal
// strings while preserving all other values and container structure.
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
		t.Fatalf("JSON mismatch: got %.512s; want %.512s", got, want)
	}
}

// osonSemanticEqual compares JSON values recursively, using exact rational
// arithmetic for numbers so equivalent decimal spellings compare equally.
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
