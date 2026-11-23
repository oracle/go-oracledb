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

package json

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestOptionsFields verifies public option fields control encoding and materialization together.
func TestOptionsFields(t *testing.T) {
	value := map[string]any{
		"number": int64(42),
		"time":   time.Date(2025, 2, 3, 4, 5, 6, 123456789, time.UTC),
	}
	doc, err := NewJSONWithOptions(value, Options{
		NumberMode:   NumberAsJSONNumber,
		TimeEncoding: TimeAsDate,
	})
	if err != nil {
		t.Fatalf("NewJSONWithOptions() failed: %v", err)
	}
	got, err := doc.GetValue()
	if err != nil {
		t.Fatalf("GetValue() failed: %v", err)
	}
	if number := got.(map[string]any)["number"]; number != stdjson.Number("42") {
		t.Fatalf("number = %#v, want json.Number(42)", number)
	}
	object, err := doc.AsJSONObject()
	if err != nil {
		t.Fatalf("AsJSONObject() failed: %v", err)
	}
	child, err := object.Get("time")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if got := child.String(); got != `"2025-02-03T04:05:06"` {
		t.Fatalf("date = %q, want whole seconds without time zone", got)
	}
}

// TestJSONStringValue verifies JSONString accepts valid JSON text and rejects
// invalid JSON text at the bind boundary.
func TestJSONStringValue(t *testing.T) {
	if _, err := JSONString(`{"missing":`).Value(); err == nil {
		t.Fatal("JSONString.Value() error = nil, want invalid JSON error")
	}
	if value, err := JSONString(`{"ok":true}`).Value(); err != nil || value != `{"ok":true}` {
		t.Fatalf("JSONString.Value() = (%#v, %v), want valid text", value, err)
	}
}

// TestJSONMarshalJSON verifies all public JSON views implement the standard
// library JSON marshaling interface and report errors for zero values.
func TestJSONMarshalJSON(t *testing.T) {
	doc, err := NewJSON(map[string]any{"items": []any{"value"}})
	if err != nil {
		t.Fatalf("NewJSON() failed: %v", err)
	}
	object, err := doc.AsJSONObject()
	if err != nil {
		t.Fatalf("AsJSONObject() failed: %v", err)
	}
	items, err := object.Get("items")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	array, err := items.AsJSONArray()
	if err != nil {
		t.Fatalf("AsJSONArray() failed: %v", err)
	}
	item, err := array.Get(0)
	if err != nil {
		t.Fatalf("JSONArray.Get(0) failed: %v", err)
	}
	scalar, err := item.AsJSONScalar()
	if err != nil {
		t.Fatalf("AsJSONScalar() failed: %v", err)
	}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "document", value: doc, want: `{"items":["value"]}`},
		{name: "object", value: object, want: `{"items":["value"]}`},
		{name: "array", value: array, want: `["value"]`},
		{name: "scalar", value: scalar, want: `"value"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := stdjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("json.Marshal() failed: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("json.Marshal() = %s, want %s", got, tc.want)
			}
		})
	}

	for _, test := range []struct {
		value any
		want  string
	}{
		{float32(1.25), `1.25`},
		{float64(1.25), `1.25`},
		{math.NaN(), `"Nan"`},
		{math.Inf(1), `"Inf"`},
		{math.Inf(-1), `"-Inf"`},
		{float32(math.NaN()), `"Nan"`},
		{float32(math.Inf(1)), `"Inf"`},
		{float32(math.Inf(-1)), `"-Inf"`},
		{"Inf", `"Inf"`},
	} {
		doc, err := NewJSONWithOptions(test.value, Options{NumberMode: NumberAsJSONNumber})
		if err != nil {
			t.Fatalf("NewJSON(%v) failed: %v", test.value, err)
		}
		value, err := doc.GetValue()
		if err != nil {
			t.Fatalf("GetValue() failed: %v", err)
		}
		switch test.value.(type) {
		case float32, float64:
			if test.want == "1.25" {
				if value != stdjson.Number("1.25") {
					t.Fatalf("finite value = %T %v, want json.Number(1.25)", value, value)
				}
				break
			}
			number, ok := value.(float64)
			if !ok {
				t.Fatalf("special value = %T, want float64", value)
			}
			if test.want == `"Nan"` && !math.IsNaN(number) || test.want == `"Inf"` && !math.IsInf(number, 1) || test.want == `"-Inf"` && !math.IsInf(number, -1) {
				t.Fatalf("special value = %v, want %v", number, test.value)
			}
			if _, err := stdjson.Marshal(value); err == nil {
				t.Fatal("json.Marshal(extracted special) succeeded, want unsupported-value error")
			}
		}
		got, err := stdjson.Marshal(doc)
		if err != nil || string(got) != test.want {
			t.Fatalf("json.Marshal(%v) = (%s, %v), want %s", test.value, got, err, test.want)
		}
		if got := doc.String(); got != test.want {
			t.Fatalf("String(%v) = %s, want %s", test.value, got, test.want)
		}
	}

	zeroValues := []struct {
		name  string
		value any
	}{
		{name: "document", value: JSON{}},
		{name: "object", value: JSONObject{}},
		{name: "array", value: JSONArray{}},
		{name: "scalar", value: JSONScalar{}},
	}
	for _, tc := range zeroValues {
		t.Run("zero "+tc.name, func(t *testing.T) {
			if _, err := stdjson.Marshal(tc.value); err == nil {
				t.Fatalf("json.Marshal(zero %s) error = nil, want error", tc.name)
			}
		})
	}
}

// TestJSONUnmarshalJSON verifies JSON accepts standard JSON text and preserves
// number precision while rejecting invalid JSON without replacing the receiver.
func TestJSONUnmarshalJSON(t *testing.T) {
	var doc JSON
	input := []byte(`{"items":["value"],"score":9007199254740993}`)
	if err := stdjson.Unmarshal(input, &doc); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}
	output, err := stdjson.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	if string(output) != string(input) {
		t.Fatalf("json.Marshal() = %s, want %s", output, input)
	}

	err = doc.UnmarshalJSON([]byte(`{"invalid":`))
	if sqlErr, ok := err.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.OsonEncodingError) {
		t.Fatalf("UnmarshalJSON() error = %v, want OsonEncodingError", err)
	}

	output, err = stdjson.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal() after invalid input failed: %v", err)
	}
	if string(output) != string(input) {
		t.Fatalf("json.Unmarshal(invalid JSON) replaced document with %s, want %s", output, input)
	}

	var nilDoc *JSON
	if err := stdjson.Unmarshal(input, nilDoc); err == nil {
		t.Fatal("json.Unmarshal(nil *JSON) error = nil, want error")
	}
}

// TestJSONNilValues verifies nil and empty values encode to their
// respective empty JSON values rather than JSON null.
func TestJSONNilValues(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		wantKind JSONKind
	}{
		{name: "nil map", value: map[string]any(nil), wantKind: JSONObjectKind},
		{name: "empty map", value: map[string]any{}, wantKind: JSONObjectKind},
		{name: "nil slice", value: []any(nil), wantKind: JSONArrayKind},
		{name: "empty slice", value: []any{}, wantKind: JSONArrayKind},
		{name: "nil bytes", value: []byte(nil), wantKind: JSONScalarKind},
		{name: "empty bytes", value: []byte{}, wantKind: JSONScalarKind},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := NewJSON(tc.value)
			if err != nil {
				t.Fatalf("NewJSON(%#v) failed: %v", tc.value, err)
			}
			kind, err := doc.Kind()
			if err != nil {
				t.Fatalf("Kind() failed: %v", err)
			}
			if kind != tc.wantKind {
				t.Fatalf("Kind() = %d, want %d", kind, tc.wantKind)
			}
		})
	}
}

// TestJSONScanRejectsInvalidSources verifies Scan accepts only OSON bytes.
func TestJSONScanRejectsInvalidSources(t *testing.T) {
	var doc JSON
	if err := doc.Scan([]byte(`{"not":"oson"}`)); err == nil {
		t.Fatal("JSON.Scan(JSON text) error = nil, want OSON error")
	}
	if err := doc.Scan("not bytes"); err == nil {
		t.Fatal("JSON.Scan(string) error = nil, want type error")
	}
}

// TestNewJSONWithOptionsRejectsUnsupportedValue verifies construction reports
// an encoding error instead of returning a partial document for unsupported Go values.
func TestNewJSONWithOptionsRejectsUnsupportedValue(t *testing.T) {
	if _, err := NewJSONWithOptions(make(chan int), Options{}); err == nil {
		t.Fatal("NewJSONWithOptions(chan) error = nil, want encoding error")
	}
}

// TestNewJSONRejectsContainerCycles verifies cyclic maps and slices fail with
// an encoding error while shared acyclic containers remain valid.
func TestNewJSONRejectsContainerCycles(t *testing.T) {
	mapCycle := map[string]any{}
	mapCycle["self"] = mapCycle

	sliceCycle := make([]any, 1)
	sliceCycle[0] = sliceCycle

	mixedCycle := map[string]any{}
	mixedSlice := []any{mixedCycle}
	mixedCycle["self"] = mixedSlice

	for _, value := range []any{mapCycle, sliceCycle, mixedCycle} {
		if _, err := NewJSON(value); err == nil {
			t.Fatalf("NewJSON(%T cycle) error = nil, want encoding error", value)
		} else if coded, ok := err.(interface{ ErrorCode() string }); !ok || coded.ErrorCode() != string(oracleErrors.OsonEncodingError) {
			t.Fatalf("NewJSON(%T cycle) error = %v, want %s", value, err, oracleErrors.OsonEncodingError)
		}
	}

	shared := map[string]any{"value": "ok"}
	sharedSlice := []any{"ok"}
	overlapping := make([]any, 2)
	overlapping[0] = "ok"
	overlapping[1] = overlapping[:1]
	for _, value := range []any{
		map[string]any{"left": shared, "right": shared},
		[]any{sharedSlice, sharedSlice},
		overlapping,
	} {
		if _, err := NewJSON(value); err != nil {
			t.Fatalf("NewJSON(%T acyclic container) failed: %v", value, err)
		}
	}
}

// TestJSONSetOptionsSurvivesScan verifies Scan retains number materialization
// options selected before receiving an OSON document.
func TestJSONSetOptionsSurvivesScan(t *testing.T) {
	source, err := NewJSON(stdjson.Number("9007199254740993"))
	if err != nil {
		t.Fatalf("NewJSON() failed: %v", err)
	}
	value, err := source.Value()
	if err != nil {
		t.Fatalf("JSON.Value() failed: %v", err)
	}

	var got JSON
	got.SetOptions(Options{NumberMode: NumberAsJSONNumber})
	if err = got.Scan(value); err != nil {
		t.Fatalf("JSON.Scan() failed: %v", err)
	}
	materialized, err := got.GetValue()
	if err != nil {
		t.Fatalf("JSON.GetValue() failed: %v", err)
	}
	if materialized != stdjson.Number("9007199254740993") {
		t.Fatalf("JSON.GetValue() = %#v, want exact json.Number", materialized)
	}
}

// TestJSONChildValueUsesInheritedOptions verifies binding a child view applies
// its parent's number option when constructing a standalone OSON document.
func TestJSONChildValueUsesInheritedOptions(t *testing.T) {
	parent, err := NewJSONWithOptions(
		map[string]any{"number": stdjson.Number("9007199254740993")},
		Options{NumberMode: NumberAsFloat64},
	)
	if err != nil {
		t.Fatalf("NewJSONWithOptions() failed: %v", err)
	}
	object, err := parent.AsJSONObject()
	if err != nil {
		t.Fatalf("AsJSONObject() failed: %v", err)
	}
	child, err := object.Get("number")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	value, err := child.Value()
	if err != nil {
		t.Fatalf("child.Value() failed: %v", err)
	}
	var rebound JSON
	if err := rebound.Scan(value); err != nil {
		t.Fatalf("JSON.Scan(child.Value()) failed: %v", err)
	}
	got, err := rebound.GetValue()
	if err != nil {
		t.Fatalf("JSON.GetValue() failed: %v", err)
	}
	// The inherited float64 mode intentionally rounds the original exact number.
	if got != float64(9007199254740992) {
		t.Fatalf("rebound value = %T(%v), want float64(9007199254740992)", got, got)
	}
}

// TestJSONZeroValues verifies that a zero JSON and zero lazy views are
// uninitialized API values, rather than JSON null documents.
func TestJSONZeroValues(t *testing.T) {
	var doc JSON
	if _, err := doc.Value(); err == nil {
		t.Fatal("JSON.Value() error = nil, want error")
	}
	if _, err := doc.Kind(); err == nil {
		t.Fatal("JSON.Kind() error = nil, want error")
	}
	if _, err := doc.GetValue(); err == nil {
		t.Fatal("JSON.GetValue() error = nil, want error")
	} else if sqlErr, ok := err.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.JSONUninitializedValue) {
		t.Fatalf("GetValue() error = %v, want JSONUninitializedValue", err)
	}
	if _, err := doc.AsJSONObject(); err == nil {
		t.Fatal("JSON.AsJSONObject() error = nil, want error")
	}
	if doc.String() != "<JSON: uninitialized>" {
		t.Fatalf("JSON.String() = %q, want uninitialized marker", doc.String())
	}

	var object JSONObject
	if object.Keys() != nil || object.String() != "<JSONObject: uninitialized>" {
		t.Fatal("zero JSONObject did not return its documented values")
	}
	if _, err := object.Get("missing"); err == nil {
		t.Fatal("zero JSONObject.Get() error = nil, want error")
	}
	if _, err := object.GetValue(); err == nil {
		t.Fatal("zero JSONObject.GetValue() error = nil, want error")
	}

	var array JSONArray
	if array.Len() != -1 || array.String() != "<JSONArray: uninitialized>" {
		t.Fatal("zero JSONArray did not return its documented values")
	}
	if _, err := array.Get(0); err == nil {
		t.Fatal("zero JSONArray.Get() error = nil, want error")
	}
	if _, err := array.GetValue(); err == nil {
		t.Fatal("zero JSONArray.GetValue() error = nil, want error")
	}

	var scalar JSONScalar
	if scalar.String() != "<JSONScalar: uninitialized>" {
		t.Fatalf("JSONScalar.String() = %q, want uninitialized marker", scalar.String())
	}
	if _, err := scalar.GetValue(); err == nil {
		t.Fatal("zero JSONScalar.GetValue() error = nil, want error")
	}
}

// TestJSONObjectAccessors verifies an object view exposes its members and
// rejects conversion to an incompatible array view.
func TestJSONObjectAccessors(t *testing.T) {
	objectDoc, err := NewJSON(map[string]any{"items": []any{"value"}})
	if err != nil {
		t.Fatalf("NewJSON(object) failed: %v", err)
	}
	object, err := objectDoc.AsJSONObject()
	if err != nil {
		t.Fatalf("AsJSONObject() failed: %v", err)
	}
	if _, err := object.Get("missing"); err == nil {
		t.Fatal("JSONObject.Get(missing) error = nil, want error")
	} else if sqlErr, ok := err.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.JSONObjectKeyNotFoundError) {
		t.Fatalf("Get(missing) error = %v, want JSONObjectKeyNotFoundError", err)
	}
	if len(object.Keys()) != 1 {
		t.Fatalf("JSONObject.Keys() = %v, want one key", object.Keys())
	}
	objectValue, err := object.GetValue()
	if err != nil || !reflect.DeepEqual(objectValue, map[string]any{"items": []any{"value"}}) {
		t.Fatalf("JSONObject.GetValue() = (%#v, %v), want object", objectValue, err)
	}
	if value, err := objectDoc.GetValue(); err != nil || !reflect.DeepEqual(value, objectValue) {
		t.Fatalf("JSON.GetValue() = (%#v, %v), want object", value, err)
	}
	if objectDoc.String() != `{"items":["value"]}` {
		t.Fatalf("JSON.String() = %q, want JSON text", objectDoc.String())
	}
	if object.String() != `{"items":["value"]}` {
		t.Fatalf("JSONObject.String() = %q, want JSON text", object.String())
	}
	if _, err := objectDoc.AsJSONArray(); err == nil {
		t.Fatal("object AsJSONArray() error = nil, want error")
	}
}

// TestJSONArrayAccessors verifies an array view exposes valid indexes and
// rejects invalid indexes and scalar conversion.
func TestJSONArrayAccessors(t *testing.T) {
	arrayDoc, err := NewJSON([]any{"value"})
	if err != nil {
		t.Fatalf("NewJSON(array) failed: %v", err)
	}
	array, err := arrayDoc.AsJSONArray()
	if err != nil {
		t.Fatalf("AsJSONArray() failed: %v", err)
	}
	item, err := array.Get(0)
	if err != nil {
		t.Fatalf("JSONArray.Get(0) failed: %v", err)
	}
	if value, err := item.GetValue(); err != nil || value != "value" {
		t.Fatalf("JSONArray.Get(0).GetValue() = (%#v, %v), want value", value, err)
	}
	if _, err = array.Get(1); err == nil {
		t.Fatal("JSONArray.Get(1) error = nil, want error")
	}
	if array.Len() != 1 {
		t.Fatalf("JSONArray.Len() = %d, want 1", array.Len())
	}
	arrayValue, err := array.GetValue()
	if err != nil || !reflect.DeepEqual(arrayValue, []any{"value"}) {
		t.Fatalf("JSONArray.GetValue() = (%#v, %v), want array", arrayValue, err)
	}
	if array.String() != `["value"]` {
		t.Fatalf("JSONArray.String() = %q, want JSON text", array.String())
	}
	if _, err := arrayDoc.AsJSONScalar(); err == nil {
		t.Fatal("array AsJSONScalar() error = nil, want error")
	}
}

// TestJSONScalarAccessors verifies a scalar view materializes its value and
// rejects conversion to an incompatible object view.
func TestJSONScalarAccessors(t *testing.T) {
	scalarDoc, err := NewJSON(true)
	if err != nil {
		t.Fatalf("NewJSON(scalar) failed: %v", err)
	}
	scalar, err := scalarDoc.AsJSONScalar()
	if err != nil {
		t.Fatalf("AsJSONScalar() failed: %v", err)
	}
	if scalar.String() != "true" {
		t.Fatalf("JSONScalar.String() = %q, want true", scalar.String())
	}
	if value, err := scalar.GetValue(); err != nil || value != true {
		t.Fatalf("JSONScalar.GetValue() = (%#v, %v), want true", value, err)
	}
	if _, err := scalarDoc.AsJSONObject(); err == nil {
		t.Fatal("scalar AsJSONObject() error = nil, want error")
	}
}

// TestJSONScanCopiesSourceBytes verifies that JSON.Scan takes ownership of OSON
// bytes instead of retaining the caller-provided source slice.
func TestJSONScanCopiesSourceBytes(t *testing.T) {
	input, err := NewJSON(map[string]any{"id": int64(1), "name": "first"})
	if err != nil {
		t.Fatalf("NewJSON() failed: %v", err)
	}

	driverValue, err := input.Value()
	if err != nil {
		t.Fatalf("JSON.Value() failed: %v", err)
	}

	src, ok := driverValue.([]byte)
	if !ok {
		t.Fatalf("JSON.Value() returned %T, want []byte", driverValue)
	}

	var got JSON
	if err := got.Scan(src); err != nil {
		t.Fatalf("JSON.Scan() failed: %v", err)
	}

	// keep the replacement byte length identical so OSON structural offsets stay valid
	at := bytes.Index(src, []byte("first"))
	if at < 0 {
		t.Fatal(`encoded OSON does not contain "first" payload`)
	}
	copy(src[at:at+len("pwned")], []byte("pwned"))

	obj, err := got.AsJSONObject()
	if err != nil {
		t.Fatalf("AsJSONObject() failed: %v", err)
	}

	nameJSON, err := obj.Get("name")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	nameScalar, err := nameJSON.AsJSONScalar()
	if err != nil {
		t.Fatalf("AsJSONScalar() failed: %v", err)
	}

	nameValue, err := nameScalar.GetValue()
	if err != nil {
		t.Fatalf("GetValue() failed: %v", err)
	}

	if nameValue != "first" {
		t.Fatalf(`name = %#v, want "first"; JSON.Scan retained caller-owned []byte`, nameValue)
	}
}

// TestJSONViewsPropagateNodeErrors checks the public wrappers without depending
// on a particular OSON byte sequence. Codec fixtures are tested in oson.
func TestJSONViewsPropagateNodeErrors(t *testing.T) {
	sentinel := errors.New("test node error")
	document := JSON{node: jsonErrorScalarNode{err: sentinel}}
	object := JSONObject{node: jsonErrorObjectNode{err: sentinel}}
	array := JSONArray{node: jsonErrorArrayNode{err: sentinel}}
	scalar := JSONScalar{node: jsonErrorScalarNode{err: sentinel}}
	tests := []struct {
		name  string
		value interface {
			MarshalJSON() ([]byte, error)
			String() string
		}
		getValue    func() (any, error)
		placeholder string
	}{
		{"document", document, document.GetValue, "<JSON: rendering failed>"},
		{"object", object, func() (any, error) { return object.GetValue() }, "<JSONObject: rendering failed>"},
		{"array", array, func() (any, error) { return array.GetValue() }, "<JSONArray: rendering failed>"},
		{"scalar", scalar, scalar.GetValue, "<JSONScalar: rendering failed>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.getValue(); !errors.Is(err, sentinel) {
				t.Fatalf("GetValue() error = %v, want wrapped sentinel", err)
			}
			if _, err := stdjson.Marshal(tc.value); !errors.Is(err, sentinel) {
				t.Fatalf("json.Marshal() error = %v, want wrapped sentinel", err)
			}
			if got := tc.value.String(); got != tc.placeholder {
				t.Fatalf("String() = %q, want %q", got, tc.placeholder)
			}
		})
	}
}

type jsonErrorObjectNode struct {
	drvCommon.JSONObjectNode
	err error
}

func (n jsonErrorObjectNode) GetValue(drvCommon.JSONConversionOptions) (any, error) {
	return nil, n.err
}
func (n jsonErrorObjectNode) Value(drvCommon.JSONConversionOptions) (map[string]any, error) {
	return nil, n.err
}
func (n jsonErrorObjectNode) MarshalJSON() ([]byte, error) { return nil, n.err }
func (n jsonErrorObjectNode) String() (string, error)      { return "", n.err }

type jsonErrorArrayNode struct {
	drvCommon.JSONArrayNode
	err error
}

func (n jsonErrorArrayNode) GetValue(drvCommon.JSONConversionOptions) (any, error) {
	return nil, n.err
}
func (n jsonErrorArrayNode) Value(drvCommon.JSONConversionOptions) ([]any, error) {
	return nil, n.err
}
func (n jsonErrorArrayNode) MarshalJSON() ([]byte, error) { return nil, n.err }
func (n jsonErrorArrayNode) String() (string, error)      { return "", n.err }

type jsonErrorScalarNode struct {
	drvCommon.JSONScalarNode
	err error
}

func (n jsonErrorScalarNode) GetValue(drvCommon.JSONConversionOptions) (any, error) {
	return nil, n.err
}
func (n jsonErrorScalarNode) Value(drvCommon.JSONConversionOptions) (any, error) {
	return nil, n.err
}
func (n jsonErrorScalarNode) MarshalJSON() ([]byte, error) { return nil, n.err }
func (n jsonErrorScalarNode) String() (string, error)      { return "", n.err }

// TestJSONUnsupportedGoInputMatrix checks API type boundaries without requiring
// a configured database. Defined Go types differ from true type aliases.
func TestJSONUnsupportedGoInputMatrix(t *testing.T) {
	type definedString string
	type definedNumber int64
	type definedBytes []byte
	type definedMap map[string]any
	type definedSlice []any
	var nilPointer *int
	for _, input := range []struct {
		name  string
		value any
	}{
		{"defined_string", definedString("oracle")},
		{"defined_number", definedNumber(42)},
		{"defined_bytes", definedBytes{0, 255}},
		{"defined_map", definedMap{"value": "oracle"}},
		{"defined_slice", definedSlice{"oracle"}},
		{"pointer", new(int)},
		{"nil_pointer", nilPointer},
		{"typed_string_slice", []string{"oracle"}},
		{"typed_integer_slice", []int{1, 2}},
		{"typed_value_map", map[string]int{"value": 42}},
		{"non_string_key_map", map[int]any{1: "oracle"}},
		{"array", [1]string{"oracle"}},
		{"raw_message", stdjson.RawMessage(`{"value":"oracle"}`)},
		{"nested_pointer", map[string]any{"value": []any{new(int)}}},
	} {
		t.Run("unsupported/"+input.name, func(t *testing.T) {
			_, err := NewJSON(input.value)
			requirePublicOSONRegressionCode(t, err, oracleErrors.OsonEncodingError)
		})
	}

	type stringAlias = string
	type numberAlias = int64
	type booleanAlias = bool
	type bytesAlias = []byte
	type mapAlias = map[string]any
	type sliceAlias = []any
	type jsonNumberAlias = stdjson.Number
	for _, input := range []struct {
		name  string
		value any
		want  any
	}{
		{"string", stringAlias("oracle"), "oracle"},
		{"number", numberAlias(42), int64(42)},
		{"boolean", booleanAlias(true), true},
		{"bytes", bytesAlias{0, 255}, []byte{0, 255}},
		{"map", mapAlias{"value": "oracle"}, map[string]any{"value": "oracle"}},
		{"slice", sliceAlias{"oracle", true}, []any{"oracle", true}},
		{"json_number", jsonNumberAlias("123.45"), stdjson.Number("123.45")},
	} {
		t.Run("supported_alias/"+input.name, func(t *testing.T) {
			doc, err := NewJSON(input.value)
			if err != nil {
				t.Fatalf("true alias rejected: %v", err)
			}
			got, err := doc.GetValue()
			if err != nil || !reflect.DeepEqual(got, input.want) {
				t.Fatalf("true alias value=(%#v,%v), want %#v", got, err, input.want)
			}
		})
	}
}

func requirePublicOSONRegressionCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var coded interface{ ErrorCode() string }
	if err == nil || !errors.As(err, &coded) || coded.ErrorCode() != string(want) {
		t.Fatalf("error=%v, want %s", err, want)
	}
}

// Each timezone runs in a fresh process. Mutating time.Local in this process
// would interfere with other tests and would not test initialization from TZ.
func TestJSONTemporal_LocalZoneSubprocess(t *testing.T) {
	const childZone = "GO_ORACLEDB_OSON_TEMPORAL_ZONE_CHILD"
	if zone := os.Getenv(childZone); zone != "" {
		location, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatalf("load requested child timezone %q: %v", zone, err)
		}
		input := time.Date(2024, 2, 29, 14, 15, 16, 123456789, time.FixedZone("source", 19800))
		for _, encoding := range temporalRegressionEncodings() {
			t.Run(encoding.name, func(t *testing.T) {
				for _, nested := range []bool{false, true} {
					t.Run(fmt.Sprintf("nested-%t", nested), func(t *testing.T) {
						var source any = input
						if nested {
							source = map[string]any{"when": input}
						}
						doc, err := NewJSONWithOptions(source, Options{TimeEncoding: encoding.value})
						if err != nil {
							t.Fatalf("construct temporal document: %v", err)
						}
						if nested {
							view, err := doc.AsJSONObject()
							if err != nil {
								t.Fatalf("obtain temporal object view: %v", err)
							}
							doc, err = view.Get("when")
							if err != nil {
								t.Fatalf("obtain temporal child: %v", err)
							}
						}
						value, err := doc.GetValue()
						if err != nil {
							t.Fatalf("materialize temporal value: %v", err)
						}
						actual, ok := value.(time.Time)
						if !ok {
							t.Fatalf("temporal value=%T, want time.Time", value)
						}
						want := input
						if encoding.value == TimeAsDate {
							want = input.Truncate(time.Second)
						}
						if temporalRegressionFields(actual) != temporalRegressionFields(want) {
							t.Fatalf("temporal fields=%v, want %v", temporalRegressionFields(actual), temporalRegressionFields(want))
						}
						if encoding.value == TimeAsTimestampTZ {
							_, offset := actual.Zone()
							if !actual.Equal(input) || offset != 19800 {
								t.Fatalf("timestamp-tz changed instant/offset: %v", actual)
							}
						} else {
							_, actualOffset := actual.Zone()
							expectedLocal := time.Date(want.Year(), want.Month(), want.Day(), want.Hour(), want.Minute(), want.Second(), want.Nanosecond(), location)
							_, expectedOffset := expectedLocal.Zone()
							if actual.Location() != time.Local || actualOffset != expectedOffset {
								t.Fatalf("zone-free value location=%v offset=%d, want local %s offset=%d", actual.Location(), actualOffset, zone, expectedOffset)
							}
						}
						text, err := stdjson.Marshal(doc)
						if err != nil {
							t.Fatalf("marshal temporal value: %v", err)
						}
						var rendered string
						if err := stdjson.Unmarshal(text, &rendered); err != nil {
							t.Fatalf("temporal rendering is not a JSON string: %v", err)
						}
						if encoding.value != TimeAsTimestampTZ && rendered != want.Format("2006-01-02T15:04:05.999999999") {
							t.Fatalf("zone-free rendering unexpectedly depends on TZ: %q", rendered)
						}
					})
				}
			})
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate temporal test executable: %v", err)
	}
	for _, zone := range []string{"UTC", "Asia/Kolkata", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestJSONTemporal_LocalZoneSubprocess$", "-test.count=1")
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "TZ=") && !strings.HasPrefix(entry, childZone+"=") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "TZ="+zone, childZone+"="+zone)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if len(output) > 4000 {
					output = output[:4000]
				}
				t.Fatalf("isolated timezone %s failed: %v (context=%v)\n%s", zone, err, ctx.Err(), output)
			}
		})
	}
}

// This local regression accepts explicit rejection, but not a successfully
// materialized year that differs from the input due to century-byte wrapping.
// Functional tests separately establish server rejection/persistence behavior.
func TestJSONTemporal_YearValidation(t *testing.T) {
	for _, year := range []int{0, 10000, 30000, -30000} {
		for _, encoding := range temporalRegressionEncodings() {
			t.Run(fmt.Sprintf("year-%d/%s", year, encoding.name), func(t *testing.T) {
				input := time.Date(year, 2, 3, 4, 5, 6, 123456789, time.UTC)
				doc, err := NewJSONWithOptions(input, Options{TimeEncoding: encoding.value})
				if err != nil {
					if !temporalRegressionErrorCode(err, oracleErrors.OsonEncodingError) {
						t.Fatalf("unexpected temporal constructor error: %v", err)
					}
					t.Logf("temporal input rejected at construction: %v", err)
					return
				}
				value, err := doc.GetValue()
				if err != nil {
					if !temporalRegressionErrorCode(err, oracleErrors.OsonEncodingError, oracleErrors.ConverterExpectedFormat, oracleErrors.ConverterRange) {
						t.Fatalf("unexpected temporal materialization error: %v", err)
					}
					t.Logf("temporal input rejected lazily: %v", err)
					return
				}
				actual, ok := value.(time.Time)
				want := input
				if encoding.value == TimeAsDate {
					want = want.Truncate(time.Second)
				}
				if !ok || temporalRegressionFields(actual) != temporalRegressionFields(want) {
					t.Fatalf("accepted temporal year changed: got=%#v, want fields=%v", value, temporalRegressionFields(want))
				}
				t.Logf("year %d survives local conversion; server range validation is checked separately", year)
			})
		}
	}
}

// User-selected policy for unrepresentable offset seconds: explicit rejection
// or normalization is acceptable, provided an accepted value keeps its instant.
// TestJSONTemporal_SecondsOffsetCharacterization accepts rejection or minute-offset normalization that preserves the original UTC instant.
func TestJSONTemporal_SecondsOffsetCharacterization(t *testing.T) {
	for _, offset := range []int{30, -30, 19830, -12630} {
		t.Run(fmt.Sprintf("offset-%d", offset), func(t *testing.T) {
			input := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.FixedZone("offset-seconds", offset))
			doc, err := NewJSONWithOptions(input, Options{TimeEncoding: TimeAsTimestampTZ})
			if err != nil {
				if !temporalRegressionErrorCode(err, oracleErrors.OsonEncodingError) {
					t.Fatalf("unexpected seconds-offset constructor error: %v", err)
				}
				t.Logf("sub-minute offset rejected at construction: %v", err)
				return
			}
			value, err := doc.GetValue()
			if err != nil {
				if !temporalRegressionErrorCode(err, oracleErrors.OsonEncodingError, oracleErrors.ConverterExpectedFormat, oracleErrors.ConverterRange) {
					t.Fatalf("unexpected seconds-offset materialization error: %v", err)
				}
				t.Logf("sub-minute offset rejected lazily: %v", err)
				return
			}
			actual, ok := value.(time.Time)
			if !ok || !actual.Equal(input) {
				t.Fatalf("sub-minute offset changed instant: got=%#v, want UTC=%s", value, input.UTC().Format(time.RFC3339Nano))
			}
			text, err := stdjson.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal normalized timestamp: %v", err)
			}
			var rendered string
			if err := stdjson.Unmarshal(text, &rendered); err != nil {
				t.Fatalf("normalized timestamp is not a JSON string: %v", err)
			}
			parsed, err := time.Parse(time.RFC3339Nano, rendered)
			if err != nil || !parsed.Equal(input) {
				t.Fatalf("normalized rendering changed instant: %q (%v)", rendered, err)
			}
			_, normalized := actual.Zone()
			t.Logf("offset-seconds policy: requested=%d returned=%d lost=%d; instant preserved", offset, normalized, offset-normalized)
		})
	}
}

func temporalRegressionErrorCode(err error, accepted ...oracleErrors.ErrorCode) bool {
	var coded oracleErrors.SQLError
	if !errors.As(err, &coded) {
		return false
	}
	for _, code := range accepted {
		if coded.ErrorCode() == string(code) {
			return true
		}
	}
	return false
}

func temporalRegressionFields(v time.Time) [7]int {
	return [7]int{v.Year(), int(v.Month()), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond()}
}

func temporalRegressionEncodings() []struct {
	name  string
	value TimeEncoding
} {
	return []struct {
		name  string
		value TimeEncoding
	}{{"timestamp", TimeAsTimestamp}, {"timestamp-tz", TimeAsTimestampTZ}, {"date", TimeAsDate}}
}
