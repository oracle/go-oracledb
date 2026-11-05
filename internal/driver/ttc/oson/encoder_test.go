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
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
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
		"NaN JSON number":          json.Number("NaN"),
		"positive infinity number": json.Number("+Inf"),
		"negative infinity number": json.Number("-Inf"),
		"quoted JSON number":       json.Number(`"123"`),
		"empty JSON number":        json.Number(""),
		"leading zero":             json.Number("01"),
		"null number":              json.Number("null"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Encode(value)
			assertOracleErrorCode(t, err, oracleErrors.OsonEncodingError)
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

// TestOSONScalarBoundaryContent fills content-check gaps in the existing
// opcode-focused boundary cases, including the independent tree-size change.
func TestOSONScalarBoundaryContent(t *testing.T) {
	for _, kind := range []string{"string", "binary"} {
		lengths := []int{0, 1, 30, 65532, 65533, 65535}
		if kind == "binary" {
			lengths = append(lengths, 65536)
		}
		for _, length := range lengths {
			for _, nested := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/bytes_%d/nested_%v", kind, length, nested), func(t *testing.T) {
					var scalar any
					if kind == "string" {
						scalar = strings.Repeat("é🙂", length/6) + strings.Repeat("x", length%6)
					} else {
						data := make([]byte, length)
						for i := range data {
							data[i] = byte(i*73 + 19)
						}
						scalar = data
					}
					value := scalar
					if nested {
						value = map[string]any{"payload": scalar, "next": "sentinel"}
					}
					doc := requireOSONBoundaryRoundTrip(t, value)
					if !nested && (length == 65532 || length == 65533) {
						header, err := newOsonHeader(newOsonBuffer(doc))
						if err != nil {
							t.Fatal(err)
						}
						if got, want := header.treeSegmentByteLength, uint(length+3); got != want {
							t.Fatalf("tree bytes=%d, want %d", got, want)
						}
						if got, want := header.isSet(osonFlagTreeSegmentSizeUB4Mask), length == 65533; got != want {
							t.Fatalf("wide tree size flag=%v, want %v", got, want)
						}
					}
				})
			}
		}
	}
}

// TestOSONContainerBoundaryContent materializes every element/member so wide
// counts and field IDs cannot pass merely because their header bits are set.
func TestOSONContainerBoundaryContent(t *testing.T) {
	for _, count := range []int{255, 256, 65535, 65536} {
		for _, shape := range []string{"array", "object"} {
			t.Run(fmt.Sprintf("%s/count_%d", shape, count), func(t *testing.T) {
				var value any
				if shape == "array" {
					items := make([]any, count)
					for i := range items {
						items[i] = strconv.Itoa(i)
					}
					value = items
				} else {
					fields := make(map[string]any, count)
					for i := 0; i < count; i++ {
						fields[fmt.Sprintf("field_%05d", i)] = strconv.Itoa(i)
					}
					value = fields
				}
				doc := requireOSONBoundaryRoundTrip(t, value)
				if shape == "object" {
					header, err := newOsonHeader(newOsonBuffer(doc))
					if err != nil {
						t.Fatal(err)
					}
					width := 1
					if count > 255 {
						width = 2
					}
					if count > 65535 {
						width = 4
					}
					if got := header.numFieldIDBytes(); got != width {
						t.Fatalf("field-ID bytes=%d, want %d", got, width)
					}
				}
			})
		}
	}
}

// TestOSONDictionaryHeapBoundaryContent holds key counts stable while the
// dictionary heap crosses 65535 bytes, then checks every decoded member.
func TestOSONDictionaryHeapBoundaryContent(t *testing.T) {
	for _, tier := range []string{"primary", "secondary"} {
		for _, size := range []int{65534, 65535, 65536} {
			t.Run(fmt.Sprintf("%s/bytes_%d", tier, size), func(t *testing.T) {
				fields := make(map[string]any)
				if tier == "primary" {
					for i := 0; i < 256; i++ {
						length := 255
						if i == 255 {
							length = size - 255*256 - 1
						}
						fields[fmt.Sprintf("%03d_", i)+strings.Repeat("p", length-4)] = strconv.Itoa(i)
					}
				} else {
					for i := 0; i < 254; i++ {
						length := 256
						if i == 253 {
							length = size - 253*258 - 2
						}
						fields[fmt.Sprintf("%03d_", i)+strings.Repeat("s", length-4)] = strconv.Itoa(i)
					}
				}
				doc := requireOSONBoundaryRoundTrip(t, fields)
				header, err := newOsonHeader(newOsonBuffer(doc))
				if err != nil {
					t.Fatal(err)
				}
				if tier == "primary" {
					if got, want := header.isSet(osonFlagFieldHeapSizeUB4Mask), size > 65535; got != want {
						t.Fatalf("wide primary offsets=%v, want %v", got, want)
					}
				} else {
					if got, want := header.secondaryFlags&osonFlagSecondaryFieldOffsetsUB2Mask != 0, size <= 65535; got != want {
						t.Fatalf("narrow secondary offsets=%v, want %v", got, want)
					}
				}
			})
		}
	}
}

// TestOSONMixedDictionaryFieldIDBoundary covers field-ID width selected by the
// total count across primary and secondary dictionaries, not either alone.
func TestOSONMixedDictionaryFieldIDBoundary(t *testing.T) {
	for _, count := range []int{255, 256} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			fields := make(map[string]any, count)
			for i := 0; i < count; i++ {
				key := fmt.Sprintf("field_%03d", i)
				if i >= 127 {
					key += strings.Repeat("é", 128)
				}
				fields[key] = strconv.Itoa(i)
			}
			doc := requireOSONBoundaryRoundTrip(t, fields)
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatal(err)
			}
			if header.primaryFieldsCount != 127 || len(header.fieldDictionary.fieldNames) != count {
				t.Fatalf("dictionary counts: primary=%d total=%d", header.primaryFieldsCount, len(header.fieldDictionary.fieldNames))
			}
			width := 1
			if count == 256 {
				width = 2
			}
			if got := header.numFieldIDBytes(); got != width {
				t.Fatalf("field-ID bytes=%d, want %d", got, width)
			}
		})
	}
}

// TestOSONFieldNameMaximum verifies maximum-length field names retain their content and oversized names return an encoding error.
func TestOSONFieldNameMaximum(t *testing.T) {
	for _, length := range []int{255, 256, 65535, 65536} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			key := strings.Repeat("é", length/2) + strings.Repeat("x", length%2)
			fields := map[string]any{key: "long-key-value", "short": "sentinel"}
			if length > osonMaxSecondaryDictKeyLength {
				_, err := Encode(fields)
				assertOracleErrorCode(t, err, oracleErrors.OsonEncodingError)
				return
			}
			doc := requireOSONBoundaryRoundTrip(t, fields)
			version := byte(1)
			if length > 255 {
				version = 3
			}
			if doc[3] != version {
				t.Fatalf("version=%d, want %d", doc[3], version)
			}
		})
	}
}

// TestOSONExactDocumentSizeLimit measures the complete encoded document,
// rather than treating payload bytes as the native JSON storage size.
func TestOSONExactDocumentSizeLimit(t *testing.T) {
	const overhead = 15 // ten-byte large scalar document header, five-byte scalar prefix.
	for _, kind := range []string{"string", "binary"} {
		for _, delta := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/limit_delta_%d", kind, delta), func(t *testing.T) {
				length := osonMaxDocumentSize - overhead + delta
				var input any
				if kind == "string" {
					input = strings.Repeat("x", length)
				} else {
					data := make([]byte, length)
					for i := range data {
						data[i] = byte(i*73 + 19)
					}
					input = data
				}
				doc, err := Encode(input)
				if delta > 0 {
					assertOracleErrorCode(t, err, oracleErrors.OsonEncodingError)
					return
				}
				if err != nil {
					t.Fatalf("Encode accepted boundary value: %v", err)
				}
				if len(doc) != osonMaxDocumentSize+delta {
					t.Fatalf("encoded bytes=%d, want %d", len(doc), osonMaxDocumentSize+delta)
				}
				root, err := Parse(doc)
				if err != nil {
					t.Fatalf("Parse boundary document: %v", err)
				}
				got, err := root.GetValue(drvCommon.JSONConversionOptions{})
				if err != nil {
					t.Fatalf("materialize boundary document: %v", err)
				}
				if !osonBoundaryCodecEqual(got, input) {
					t.Fatalf("boundary document content changed: got type=%T want type=%T", got, input)
				}
			})
		}
	}
}

// TestOSONNumberTextLengthLimit verifies number text at the one-byte length boundary encodes intact and oversized text is rejected.
func TestOSONNumberTextLengthLimit(t *testing.T) {
	for _, length := range []int{255, 256} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			want := json.Number("1." + strings.Repeat("0", length-2))
			if length > 255 {
				_, err := Encode(want)
				assertOracleErrorCode(t, err, oracleErrors.OsonEncodingError)
				return
			}
			doc := requireOSONBoundaryRoundTrip(t, want)
			root, err := Parse(doc)
			if err != nil {
				t.Fatal(err)
			}
			text, err := root.MarshalJSON()
			if err != nil || string(text) != string(want) {
				t.Fatalf("number text bytes=%d error=%v, want original %d-byte number", len(text), err, length)
			}
		})
	}
}

func requireOSONBoundaryRoundTrip(t *testing.T, want any) drvCommon.B1Array {
	t.Helper()
	doc, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode %T: %v", want, err)
	}
	root, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse %T: %v", want, err)
	}
	got, err := root.GetValue(drvCommon.JSONConversionOptions{})
	if err != nil {
		t.Fatalf("GetValue %T: %v", want, err)
	}
	if !osonBoundaryCodecEqual(got, want) {
		t.Fatalf("boundary content mismatch: got type=%T want type=%T", got, want)
	}
	return doc
}

func osonBoundaryCodecEqual(got, want any) bool {
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
			other, ok := value[key]
			if !ok || !osonBoundaryCodecEqual(other, item) {
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
			if !osonBoundaryCodecEqual(value[i], item) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}
