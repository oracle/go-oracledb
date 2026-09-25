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
	stdjson "encoding/json"
	"reflect"
	"testing"
)

// TestOptionsComposition verifies that explicit later options override earlier settings.
func TestOptionsComposition(t *testing.T) {
	options := JoinOptions(
		NumberModeOption(NumberAsJSONNumber),
		TimeEncodingOption(TimeAsDate),
	)
	options = JoinOptions(options, NumberModeOption(NumberAsFloat64))

	if options.set&numberModeSet == 0 || options.conversion.NumberMode != NumberAsFloat64 {
		t.Fatalf("number option = (%v, %v), want (set, %v)", options.set, options.conversion.NumberMode, NumberAsFloat64)
	}
	if options.set&timeEncodingSet == 0 || options.conversion.TimeEncoding != TimeAsDate {
		t.Fatalf("time option = (%v, %v), want (set, %v)", options.set, options.conversion.TimeEncoding, TimeAsDate)
	}
}

// TestOptionsDefaultOverride verifies that an explicitly selected default
// overrides an earlier value when options are joined.
func TestOptionsDefaultOverride(t *testing.T) {
	options := JoinOptions(
		NumberModeOption(NumberAsFloat64),
		NumberModeOption(NumberDefault),
	)
	if options.conversion.NumberMode != NumberDefault {
		t.Fatalf("number mode = %v, want NumberDefault", options.conversion.NumberMode)
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

// TestNewJSONFromString verifies NewJSONFromString accepts valid JSON text and
// rejects empty, whitespace-only, and malformed input.
func TestNewJSONFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "object", input: `{"ok":true}`, wantErr: false},
		{name: "array", input: `[1,2,3]`, wantErr: false},
		{name: "string", input: `"hello"`, wantErr: false},
		{name: "number", input: `42`, wantErr: false},
		{name: "null", input: `null`, wantErr: false},
		{name: "whitespace padded", input: `  {"ok":true}  `, wantErr: false},
		{name: "empty", input: ``, wantErr: true},
		{name: "whitespace only", input: `   `, wantErr: true},
		{name: "malformed", input: `{"missing":`, wantErr: true},
		{name: "trailing comma", input: `{"a":1,}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewJSONFromString(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewJSONFromString(%q) error = nil, want error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewJSONFromString(%q) failed: %v", tc.input, err)
			}
			if string(got) != tc.input {
				t.Fatalf("NewJSONFromString(%q) = %q, want input preserved", tc.input, string(got))
			}
		})
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

// TestJSONNilReceiver verifies pointer methods reject a nil JSON receiver.
func TestJSONNilReceiver(t *testing.T) {
	var nilDoc *JSON
	if err := nilDoc.Scan([]byte{0xff, 0x4a, 0x5a, 0x01}); err == nil {
		t.Fatal("nil JSON.Scan() error = nil, want error")
	}
	if err := nilDoc.SetOptions(Options{}); err == nil {
		t.Fatal("nil JSON.SetOptions() error = nil, want error")
	}
}

// TestNewJSONWithOptionsRejectsUnsupportedValue verifies construction reports
// an encoding error instead of returning a partial document for unsupported Go values.
func TestNewJSONWithOptionsRejectsUnsupportedValue(t *testing.T) {
	if _, err := NewJSONWithOptions(make(chan int), Options{}); err == nil {
		t.Fatal("NewJSONWithOptions(chan) error = nil, want encoding error")
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
	if err = got.SetOptions(NumberModeOption(NumberAsJSONNumber)); err != nil {
		t.Fatalf("JSON.SetOptions() failed: %v", err)
	}
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
		NumberModeOption(NumberAsFloat64),
	)
	if err != nil {
		t.Fatalf("NewJSONWithOptions() failed: %v", err)
	}
	object, err := parent.AsJSONObject()
	if err != nil {
		t.Fatalf("AsJSONObject() failed: %v", err)
	}
	child, ok := object.Get("number")
	if !ok {
		t.Fatal("JSONObject.Get(number) = false, want true")
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
	if _, ok := object.Get("missing"); ok {
		t.Fatal("zero JSONObject.Get() = true, want false")
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
	if _, ok := object.Get("missing"); ok {
		t.Fatal("JSONObject.Get(missing) = true, want false")
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

	nameJSON, ok := obj.Get("name")
	if !ok {
		t.Fatal(`field "name" missing`)
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
