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

package oson

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestEncodeSignedIntegerWidths verifies signed integers use compact opcodes
// and decode back as the same Go type.
func TestEncodeSignedIntegerWidths(t *testing.T) {
	tests := []struct {
		name  string
		value any
		check func(drvCommon.UB1) bool
		want  any
	}{
		{"int32 minimum", int32(math.MinInt32), isCompactSigned32Opcode, int32(math.MinInt32)},
		{"int32 maximum", int32(math.MaxInt32), isCompactSigned32Opcode, int32(math.MaxInt32)},
		{"int64 minimum", int64(math.MinInt64), isCompactSigned64Opcode, int64(math.MinInt64)},
		{"int64 maximum", int64(math.MaxInt64), isCompactSigned64Opcode, int64(math.MaxInt64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Encode(test.value)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if opcode := encodedRootOpcode(t, doc); !test.check(opcode) {
				t.Fatalf("root opcode = 0x%02x, wrong signed integer family", opcode)
			}
			root, err := Parse(doc)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got, err := root.GetValue(drvCommon.JSONConversionOptions{})
			if err != nil {
				t.Fatalf("GetValue() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("GetValue() = %#v (%T), want %#v (%T)", got, got, test.want, test.want)
			}
		})
	}
}

// TestEncodeOtherScalarTypes verifies every supported scalar type round trips
// through Encode and Parse without losing precision.
func TestEncodeOtherScalarTypes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  any
	}{
		{"null", nil, nil},
		{"true", true, true},
		{"false", false, false},
		{"int8", int8(-8), json.Number("-8")},
		{"int16", int16(-16), json.Number("-16")},
		{"int", int(-42), json.Number("-42")},
		{"uint", uint(42), json.Number("42")},
		{"uint8", uint8(8), json.Number("8")},
		{"uint16", uint16(16), json.Number("16")},
		{"uint32", uint32(32), json.Number("32")},
		{"uint64 beyond int64", uint64(math.MaxUint64), json.Number("18446744073709551615")},
		{"float32", float32(12.5), json.Number("12.5")},
		{"float64", float64(123.5), json.Number("123.5")},
		{"json.Number", json.Number("9876543210.25"), json.Number("9876543210.25")},
		{"binary", []byte{0x01, 0x02}, []byte{0x01, 0x02}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Encode(test.value)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			root, err := Parse(doc)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got, err := root.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
			if err != nil {
				t.Fatalf("GetValue() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("round trip = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestEncodeStringLengthForms verifies strings use the smallest length form
// and decode back unchanged.
func TestEncodeStringLengthForms(t *testing.T) {
	tests := []struct {
		length int
		want   drvCommon.UB1
	}{
		{31, drvCommon.UB1(31)},
		{32, osonOpStringUB1},
		{255, osonOpStringUB1},
		{256, osonOpStringUB2},
		{math.MaxUint16 + 1, osonOpStringUB4},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.length), func(t *testing.T) {
			value := strings.Repeat("x", test.length)
			doc, err := Encode(value)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if got := encodedRootOpcode(t, doc); got != test.want {
				t.Fatalf("root opcode = 0x%02x, want 0x%02x", got, test.want)
			}
			root, err := Parse(doc)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got, err := root.GetValue(drvCommon.JSONConversionOptions{})
			if err != nil || got != value {
				t.Fatalf("GetValue() = %v, %v; want original string", got, err)
			}
		})
	}
}

// TestEncodeContainerCountWidths verifies containers encode their child count
// with the narrowest width.
func TestEncodeContainerCountWidths(t *testing.T) {
	for _, count := range []int{255, 256, 65536} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			doc, err := Encode(make([]any, count))
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			opcode := encodedRootOpcode(t, doc)
			wantBits := drvCommon.UB1(osonOpChildCountUB1)
			if count > math.MaxUint8 {
				wantBits = osonOpChildCountUB2
			}
			if count > math.MaxUint16 {
				wantBits = osonOpChildCountUB4
			}
			if got := opcode & osonOpChildSizeBits; got != wantBits {
				t.Fatalf("child-count opcode bits = 0x%02x, want 0x%02x", got, wantBits)
			}
		})
	}
}

// TestEncodeContainerRoundTrip verifies nested objects and arrays round trip
// to the same JSON content.
func TestEncodeContainerRoundTrip(t *testing.T) {
	value := map[string]any{
		"name":  "Ada",
		"items": []any{int32(7), true, nil, map[string]any{"ok": "yes"}},
	}
	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	root, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := root.String()
	if err != nil {
		t.Fatalf("String() error = %v", err)
	}
	want, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	assertJSONEqual(t, got, string(want))
}

// TestEncodeBinaryLengthForms verifies binary payloads select their length
// form by size.
func TestEncodeBinaryLengthForms(t *testing.T) {
	for _, test := range []struct {
		length int
		want   drvCommon.UB1
	}{
		{0, osonOpBinaryUB2},
		{math.MaxUint16, osonOpBinaryUB2},
		{math.MaxUint16 + 1, osonOpBinaryUB4},
	} {
		t.Run(strconv.Itoa(test.length), func(t *testing.T) {
			value := make([]byte, test.length)
			doc, err := Encode(value)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if got := encodedRootOpcode(t, doc); got != test.want {
				t.Fatalf("root opcode = 0x%02x, want 0x%02x", got, test.want)
			}
		})
	}
}

// TestEncodeLongFieldNames verifies that encoding documents with
// long field-names (> 255) produces OSON v3.
func TestEncodeLongFieldNames(t *testing.T) {
	key := strings.Repeat("é", 128)
	doc, err := Encode(map[string]any{key: "value", "short": "also here"})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if got, want := header.version(), drvCommon.UB1(3); got != want {
		t.Fatalf("OSON version = %d, want %d for a secondary dictionary", got, want)
	}
}

// TestEncodeWideObjectFieldIDs verifies objects with many distinct names
// widen the field-ID table and keep every member reachable.
func TestEncodeWideObjectFieldIDs(t *testing.T) {
	value := make(map[string]any, 256)
	for i := range 256 {
		value["field-"+strconv.Itoa(i)] = i
	}
	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if got := header.numFieldIDBytes(); got != 2 {
		t.Fatalf("field-ID width = %d, want UB2 for 256 fields", got)
	}
}

// TestEncodeWidePrimaryDictionaryOffsets verifies the primary dictionary
// widens its entry offsets as its heap grows.
func TestEncodeWidePrimaryDictionaryOffsets(t *testing.T) {
	value := make(map[string]any, 300)
	prefix := strings.Repeat("p", 240)
	for i := range 300 {
		value[prefix+strconv.Itoa(i)] = nil
	}
	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if !header.isSet(osonFlagFieldHeapSizeUB4Mask) {
		t.Fatal("primary dictionary heap uses UB2 offsets, want UB4 after its heap exceeds 65535 bytes")
	}
}

// TestEncodeWideSecondaryDictionaryOffsets verifies the secondary dictionary
// widens its entry offsets as its heap grows.
func TestEncodeWideSecondaryDictionaryOffsets(t *testing.T) {
	value := make(map[string]any, 300)
	prefix := strings.Repeat("s", 255)
	for i := range 300 {
		value[prefix+strconv.Itoa(i)] = nil
	}
	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if got, want := header.version(), drvCommon.UB1(3); got != want {
		t.Fatalf("OSON version = %d, want %d", got, want)
	}
	if header.secondaryFlags&osonFlagSecondaryFieldOffsetsUB2Mask != 0 {
		t.Fatal("secondary dictionary still uses UB2 offsets, want UB4 after its heap exceeds 65535 bytes")
	}
}

// TestEncodeLargeContainerOffsets verifies large containers widen child
// offsets so every child stays addressable.
func TestEncodeLargeContainerOffsets(t *testing.T) {
	doc, err := Encode([]any{strings.Repeat("x", math.MaxUint16)})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	opcode, err := buf.readUB1At(header.treeSegmentOffset())
	if err != nil {
		t.Fatalf("read root opcode error = %v", err)
	}
	if opcode&osonOpChildOffsetUB4Bit == 0 {
		t.Fatalf("root opcode = 0x%02x, want UB4 child offsets", opcode)
	}
}

// TestEncodeTimeOptions verifies time values encode using the configured
// time type.
func TestEncodeTimeOptions(t *testing.T) {
	value := time.Date(2024, time.January, 2, 3, 4, 5, 123456789, time.FixedZone("plus two", 2*60*60))
	for name, encoding := range map[string]drvCommon.JSONTimeEncoding{
		"timestamp":                drvCommon.JSONTimeAsTimestamp,
		"timestamp with time zone": drvCommon.JSONTimeAsTimestampTZ,
		"date":                     drvCommon.JSONTimeAsDate,
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := EncodeWithOptions(value, drvCommon.JSONConversionOptions{TimeEncoding: encoding})
			if err != nil {
				t.Fatalf("EncodeWithOptions() error = %v", err)
			}
			want := map[drvCommon.JSONTimeEncoding]drvCommon.UB1{
				drvCommon.JSONTimeAsTimestamp:   osonOpTimestamp,
				drvCommon.JSONTimeAsTimestampTZ: osonOpTimestampTZ,
				drvCommon.JSONTimeAsDate:        osonOpDate,
			}[encoding]
			if got := encodedRootOpcode(t, doc); got != want {
				t.Fatalf("root opcode = 0x%02x, want 0x%02x", got, want)
			}
		})
	}
}

// TestEncodeRejectsUnsupportedValues verifies Encode rejects values that have
// no OSON representation.
func TestEncodeRejectsUnsupportedValues(t *testing.T) {
	for name, value := range map[string]any{
		"unsupported Go value":     struct{}{},
		"unsupported nested value": map[string]any{"bad": []any{struct{}{}}},
		"invalid JSON number":      json.Number("not-a-number"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Encode(value); err == nil {
				t.Fatal("Encode() error = nil, want OSON encoding error")
			}
		})
	}
}

// TestEncodeRejectsOversizedDocument verifies Encode rejects documents beyond
// the maximum OSON size.
func TestEncodeRejectsOversizedDocument(t *testing.T) {
	if _, err := Encode(strings.Repeat("x", osonMaxDocumentSize)); err == nil {
		t.Fatal("Encode() error = nil, want document-size error")
	}
}

// encodedRootOpcode returns the root node opcode in an encoded OSON document.
func encodedRootOpcode(t *testing.T, doc drvCommon.B1Array) drvCommon.UB1 {
	t.Helper()
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	opcode, err := newOsonBuffer(doc).readUB1At(header.treeSegmentOffset())
	if err != nil {
		t.Fatalf("read root opcode error = %v", err)
	}
	return opcode
}
