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
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/ttc/converters"
)

// TestEncodeStringScalar_UsesExpectedStringOpcodes verifies that string roots choose the expected length opcode and decode correctly.
func TestEncodeStringScalar_UsesExpectedStringOpcodes(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantOpcode common.UB1
		wantHeader int
	}{
		{
			name:       "empty string uses compact length opcode",
			value:      "",
			wantOpcode: 0x00,
			wantHeader: osonScalarOpcodeSize,
		},
		{
			name:       "max compact length uses length opcode",
			value:      strings.Repeat("a", 31),
			wantOpcode: common.UB1(31),
			wantHeader: osonScalarOpcodeSize,
		},
		{
			name:       "uint8 length string",
			value:      strings.Repeat("b", 32),
			wantOpcode: osonOpStringUB1,
			wantHeader: osonScalarHeaderSizeUB1,
		},
		{
			name:       "max uint8 length string",
			value:      strings.Repeat("c", 254),
			wantOpcode: osonOpStringUB1,
			wantHeader: osonScalarHeaderSizeUB1,
		},
		{
			name:       "uint16 boundary string",
			value:      strings.Repeat("c", 255),
			wantOpcode: osonOpStringUB2,
			wantHeader: osonScalarHeaderSizeUB2,
		},
		{
			name:       "uint16 length string",
			value:      strings.Repeat("d", 256),
			wantOpcode: osonOpStringUB2,
			wantHeader: osonScalarHeaderSizeUB2,
		},
		{
			name:       "utf8 byte length string",
			value:      strings.Repeat("é", 16),
			wantOpcode: osonOpStringUB1,
			wantHeader: osonScalarHeaderSizeUB1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Encode(tt.value)
			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			if !IsOson(doc) {
				t.Fatalf("Encode produced non-OSON document: %x", []byte(doc[:4]))
			}

			buf := newOsonBuffer(doc)
			header, err := newOsonHeader(buf)
			if err != nil {
				t.Fatalf("newOsonHeader returned error: %v", err)
			}
			if got, want := header.version(), common.UB1(osonFormatMinVersion); got != want {
				t.Fatalf("version = %d, want %d", got, want)
			}
			if !header.isScalar() {
				t.Fatal("isScalar = false, want true")
			}
			if !header.isInlineLeaf() {
				t.Fatal("isInlineLeaf = false, want true")
			}

			wantTreeSize := len([]byte(tt.value)) + tt.wantHeader
			if got := int(header.treeSegmentSize()); got != wantTreeSize {
				t.Fatalf("treeSegmentSize = %d, want %d", got, wantTreeSize)
			}
			if got, want := header.treeSegmentOffset(), osonHeaderMinSize+osonUB2Size; got != want {
				t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
			}

			opcode, err := buf.readUB1At(header.treeSegmentOffset())
			if err != nil {
				t.Fatalf("read opcode returned error: %v", err)
			}
			if opcode != tt.wantOpcode {
				t.Fatalf("opcode = 0x%02x, want 0x%02x", opcode, tt.wantOpcode)
			}
			if int(opcode) <= osonOpShortStringMax && int(opcode) != len([]byte(tt.value)) {
				t.Fatalf("compact string opcode length = %d, want %d", opcode, len([]byte(tt.value)))
			}

			assertEncodedValueDecodesTo(t, doc, tt.value)
		})
	}
}

// TestEncodeStringScalar_UsesUB4TreeSegmentSizeWhenTreeExceedsUB2 verifies that
// large scalar documents switch the tree-size header field from UB2 to UB4.
func TestEncodeStringScalar_UsesUB4TreeSegmentSizeWhenTreeExceedsUB2(t *testing.T) {
	value := strings.Repeat("x", _maxUB2)
	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}
	if got, want := header.treeSegmentOffset(), osonHeaderMinSize+osonUB4Size; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}
	if !header.isSet(osonFlagTreeSegmentSizeUB4Mask) {
		t.Fatal("tree size UB4 flag is not set")
	}
	if got, want := int(header.treeSegmentSize()), len(value)+osonScalarHeaderSizeUB4; got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got := doc[header.treeSegmentOffset()]; got != byte(osonOpStringUB4) {
		t.Fatalf("opcode = 0x%02x, want 0x%02x", got, byte(osonOpStringUB4))
	}
	if got := int(binary.BigEndian.Uint32(doc[header.treeSegmentOffset()+1:])); got != len(value) {
		t.Fatalf("encoded string length = %d, want %d", got, len(value))
	}
	assertEncodedValueDecodesTo(t, doc, value)
}

// TestEncodeContainers_EncodesNestedObjectAndArray verifies dictionary creation, object field IDs, and child offsets for a mixed nested document.
func TestEncodeContainers_EncodesNestedObjectAndArray(t *testing.T) {
	value := map[string]any{
		"name": "Ada",
		"tags": []any{"engineer", "driver"},
		"profile": map[string]any{
			"city": "London",
		},
	}

	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}
	if header.isScalar() {
		t.Fatal("isScalar = true, want false")
	}
	if !header.isInlineLeaf() {
		t.Fatal("isInlineLeaf = false, want true")
	}
	if got, want := header.version(), common.UB1(osonFormatMinVersion); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if got, want := header.uniqueFields(), 4; got != want {
		t.Fatalf("uniqueFields = %d, want %d", got, want)
	}
	opcode, err := buf.readUB1At(header.treeSegmentOffset())
	if err != nil {
		t.Fatalf("read root opcode returned error: %v", err)
	}
	if opcode&osonOpChildOffsetUB4Bit == 0 {
		t.Fatalf("root opcode = 0x%02x has unknown offset bit, want UB4", opcode)
	}
	for _, key := range []string{"name", "tags", "profile", "city"} {
		if got := header.fieldID(key); got <= 0 {
			t.Fatalf("fieldID(%q) = %d, want positive field id", key, got)
		}
	}

	assertEncodedValueDecodesTo(t, doc, value)
}

// TestEncodeContainers_EncodesArrayRootWithoutDictionary verifies that an array-root document does not emit a field-name dictionary.
func TestEncodeContainers_EncodesArrayRootWithoutDictionary(t *testing.T) {
	value := []any{"alpha", []any{"beta", "gamma"}}

	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}
	if got, want := header.uniqueFields(), 0; got != want {
		t.Fatalf("uniqueFields = %d, want %d", got, want)
	}
	assertEncodedValueDecodesTo(t, doc, value)
}

// TestEncodeContainers_ProducesDeterministicBytesForObjectMaps verifies that
// map iteration order cannot change encoded object bytes.
func TestEncodeContainers_ProducesDeterministicBytesForObjectMaps(t *testing.T) {
	value := map[string]any{
		"z": "last",
		"a": "first",
		"m": map[string]any{
			"b": "nested-b",
			"a": "nested-a",
		},
	}

	first, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	for i := 0; i < 10; i++ {
		next, err := Encode(value)
		if err != nil {
			t.Fatalf("Encode iteration %d returned error: %v", i, err)
		}
		if !reflect.DeepEqual(next, first) {
			t.Fatalf("Encode iteration %d produced non-deterministic bytes", i)
		}
	}
}

// TestEncodeScalarValues_CoverEssentialScalarOpcodes verifies the scalar
// families used by the encoder, including compact integers and Oracle NUMBER.
func TestEncodeScalarValues_CoverEssentialScalarOpcodes(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		want      any
		wantOp    common.UB1
		numberOpt bool
	}{
		{name: "null", value: nil, want: nil, wantOp: osonOpNull},
		{name: "true", value: true, want: true, wantOp: osonOpTrue},
		{name: "false", value: false, want: false, wantOp: osonOpFalse},
		{name: "int8 uses sb4", value: int8(-42), want: common.JSONNumber("-42"), wantOp: signedIntOpcode(t, int64(-42), osonOpCompactSigned32Prefix), numberOpt: true},
		{name: "int64 uses sb8", value: int64(-1 << 40), want: common.JSONNumber("-1099511627776"), wantOp: signedIntOpcode(t, int64(-1<<40), osonOpCompactSigned64Prefix), numberOpt: true},
		{name: "uint64 uses oracle number", value: uint64(1 << 40), want: common.JSONNumber("1099511627776"), wantOp: unsignedOracleNumberOpcode(t, uint64(1<<40)), numberOpt: true},
		{name: "float32 uses binary float", value: float32(12.25), want: common.JSONNumber("12.25"), wantOp: osonOpBinaryFloat, numberOpt: true},
		{name: "float64 uses binary double", value: float64(123.5), want: common.JSONNumber("123.5"), wantOp: osonOpBinaryDouble, numberOpt: true},
		{name: "string number preserves text", value: common.JSONNumber("9876543210.25"), want: common.JSONNumber("9876543210.25"), wantOp: osonOpStringNumber, numberOpt: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Encode(tt.value)
			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			if got := encodedRootOpcode(t, doc); got != tt.wantOp {
				t.Fatalf("root opcode = 0x%02x, want 0x%02x", got, tt.wantOp)
			}
			assertEncodedValueDecodesTo(t, doc, tt.want, func() common.JSONOption {
				if tt.numberOpt {
					return common.JSONOptNumberAsString
				}
				return common.JSONOptDefault
			}())
		})
	}
}

// TestEncodeBinaryFloatAndDoubleScalars_CoverSpecialValues verifies binary
// float and double preserve sign and special values through decoding.
func TestEncodeBinaryFloatAndDoubleScalars_CoverSpecialValues(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		want   any
		wantOp common.UB1
	}{
		{name: "binary float negative zero", value: float32(math.Copysign(0, -1)), want: math.Copysign(0, -1), wantOp: osonOpBinaryFloat},
		{name: "binary float nan", value: float32(math.NaN()), want: math.NaN(), wantOp: osonOpBinaryFloat},
		{name: "binary double negative zero", value: math.Copysign(0, -1), want: math.Copysign(0, -1), wantOp: osonOpBinaryDouble},
		{name: "binary double infinity", value: math.Inf(1), want: math.Inf(1), wantOp: osonOpBinaryDouble},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Encode(tt.value)
			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			if got := encodedRootOpcode(t, doc); got != tt.wantOp {
				t.Fatalf("root opcode = 0x%02x, want 0x%02x", got, tt.wantOp)
			}
			assertEncodedValueDecodesTo(t, doc, tt.want)
		})
	}
}

// TestEncodeBinaryScalars_CoverLengthBoundaries verifies binary and ID payloads
// select the expected length forms at boundary sizes.
func TestEncodeBinaryScalars_CoverLengthBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		want   []byte
		wantOp common.UB1
	}{
		{name: "empty binary", value: []byte{}, want: []byte(nil), wantOp: osonOpBinaryUB2},
		{name: "small binary", value: []byte{0x01, 0x02}, want: []byte{0x01, 0x02}, wantOp: osonOpBinaryUB2},
		{name: "max binary ub2", value: bytesForTest(_maxUB2 - 1), want: bytesForTest(_maxUB2 - 1), wantOp: osonOpBinaryUB2},
		{name: "binary ub4 boundary", value: bytesForTest(_maxUB2), want: bytesForTest(_maxUB2), wantOp: osonOpBinaryUB4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Encode(tt.value)
			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}
			if got := encodedRootOpcode(t, doc); got != tt.wantOp {
				t.Fatalf("root opcode = 0x%02x, want 0x%02x", got, tt.wantOp)
			}
			assertEncodedValueDecodesTo(t, doc, tt.want)
		})
	}
}

// TestEncodeInvalidValues_ReturnOsonEncodingError verifies unsupported values
// and invalid field names are rejected.
func TestEncodeInvalidValues_ReturnOsonEncodingError(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "unsupported root", value: struct{}{}},
		{name: "unsupported object child", value: map[string]any{"bad": struct{}{}}},
		{name: "unsupported array child", value: []any{"ok", struct{}{}}},
		{name: "unsupported nested object child", value: map[string]any{"nested": map[string]any{"bad": struct{}{}}}},
		{name: "unsupported nested array child", value: []any{[]any{struct{}{}}}},
		{name: "invalid scalar text", value: common.JSONNumber("not-a-number")},
		{name: "nan scalar text", value: common.JSONNumber("NaN")},
		{name: "infinity scalar text", value: common.JSONNumber("+Inf")},
		{name: "field name too long", value: map[string]any{strings.Repeat("x", osonMaxSecondaryDictKeyLength+1): "value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertEncodeOsonError(t, tt.value)
		})
	}
}

// TestEncodeContainers_UsesUB4OffsetsWhenTreeExceedsUB2 verifies large
// container trees are re-emitted with UB4 child offsets.
func TestEncodeContainers_UsesUB4OffsetsWhenTreeExceedsUB2(t *testing.T) {
	value := []any{strings.Repeat("x", _maxUB2)}

	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}
	if header.isScalar() {
		t.Fatal("isScalar = true, want false")
	}
	if !header.isSet(osonFlagTreeSegmentSizeUB4Mask) {
		t.Fatal("tree size UB4 flag is not set")
	}
	opcode, err := buf.readUB1At(header.treeSegmentOffset())
	if err != nil {
		t.Fatalf("read root opcode returned error: %v", err)
	}
	if opcode&osonOpChildOffsetUB4Bit == 0 {
		t.Fatalf("root opcode = 0x%02x has UB2 offset width, want UB4 offsets", opcode)
	}
	assertEncodedValueDecodesTo(t, doc, value)
}

// TestEncodeContainers_SupportsLongFieldNames verifies long UTF-8 field names
// are stored in the secondary dictionary and raise the document version.
func TestEncodeContainers_SupportsLongFieldNames(t *testing.T) {
	longKey := strings.Repeat("é", 128) // 256 UTF-8 bytes, so it must use the secondary dictionary.
	value := map[string]any{
		longKey: "long-name-value",
		"short": "short-name-value",
	}

	doc, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}
	// Long field names live in the secondary dictionary, which is an OSON v3+
	// extension. The limited encoder emits v3 exactly when this tier is needed.
	if got, want := header.version(), common.UB1(3); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if got, want := header.primaryFieldsCount, 1; got != want {
		t.Fatalf("primaryFieldsCount = %d, want %d", got, want)
	}
	if got := header.fieldID(longKey); got <= header.primaryFieldsCount {
		t.Fatalf("fieldID(longKey) = %d, want secondary field id > %d", got, header.primaryFieldsCount)
	}
	assertEncodedValueDecodesTo(t, doc, value)
}

// TestOsonWriteBufferPatchUint_WritesExpectedWidths verifies reserved table
// slots are patched using the requested UB1, UB2, or UB4 width.
func TestOsonWriteBufferPatchUint_WritesExpectedWidths(t *testing.T) {
	tests := []struct {
		name  string
		width int
		value int
		want  common.B1Array
	}{
		{name: "ub1", width: osonUB1Size, value: 0x7f, want: common.B1Array{0x00, 0x7f, 0x00, 0x00, 0x00}},
		{name: "ub2", width: osonUB2Size, value: 0x1234, want: common.B1Array{0x00, 0x12, 0x34, 0x00, 0x00}},
		{name: "ub4", width: osonUB4Size, value: 0x12345678, want: common.B1Array{0x00, 0x12, 0x34, 0x56, 0x78}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &osonWriteBuffer{data: make(common.B1Array, 5)}
			if err := buf.patchUint(1, tt.width, tt.value); err != nil {
				t.Fatalf("patchUint returned error: %v", err)
			}
			if !reflect.DeepEqual(buf.data, tt.want) {
				t.Fatalf("buffer = %v, want %v", buf.data, tt.want)
			}
		})
	}
}

// TestOsonWriteBufferPatchUint_RejectsInvalidPatch verifies patchUint rejects
// invalid widths, negative values, overflow, and out-of-range offsets.
func TestOsonWriteBufferPatchUint_RejectsInvalidPatch(t *testing.T) {
	tests := []struct {
		name      string
		offset    int
		width     int
		value     int
		wantError string
	}{
		{name: "negative offset", offset: -1, width: osonUB1Size, value: 1, wantError: "negative"},
		{name: "negative value", offset: 0, width: osonUB1Size, value: -1, wantError: "negative"},
		{name: "invalid width", offset: 0, width: 3, value: 1, wantError: "unsupported patch width"},
		{name: "ub1 overflow", offset: 0, width: osonUB1Size, value: _maxUB1 + 1, wantError: "overflows UB1"},
		{name: "ub2 overflow", offset: 0, width: osonUB2Size, value: _maxUB2 + 1, wantError: "overflows UB2"},
		{name: "out of bounds", offset: 3, width: osonUB4Size, value: 1, wantError: "exceeds buffer length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &osonWriteBuffer{data: make(common.B1Array, 4)}
			err := buf.patchUint(tt.offset, tt.width, tt.value)
			if err == nil {
				t.Fatal("patchUint returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("patchUint error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

// assertEncodeOsonError verifies Encode rejects value with the public OSON
// encoding error code.
func assertEncodeOsonError(t *testing.T, value any) {
	t.Helper()
	_, err := Encode(value)
	if err == nil {
		t.Fatal("Encode returned nil error, want OsonEncodingError")
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("error type = %T, want common.SQLError", err)
	}
	if got, want := sqlErr.ErrorCode(), string(common.OsonEncodingError); got != want {
		t.Fatalf("ErrorCode = %s, want %s", got, want)
	}
}

// assertEncodedValueDecodesTo checks decoded output with an optional JSON option.
func assertEncodedValueDecodesTo(t *testing.T, doc common.B1Array, want any, opt ...common.JSONOption) {
	t.Helper()

	useOpt := common.JSONOptDefault
	if len(opt) > 0 {
		useOpt = opt[0]
	}

	node, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	got, err := node.GetValue(useOpt)
	if err != nil {
		t.Fatalf("GetValue returned error: %v", err)
	}
	assertDecodedEqual(t, got, want)
}

// assertDecodedEqual compares decoded values while handling OSON-specific
// materialization details such as time zones and NaN.
func assertDecodedEqual(t *testing.T, got, want any) {
	t.Helper()
	if gotTime, ok := got.(time.Time); ok {
		wantTime, ok := want.(time.Time)
		if !ok {
			t.Fatalf("decoded value = %#v, want %#v", got, want)
		}
		if gotTime != wantTime {
			t.Fatalf("decoded value = %#v, want %#v", got, want)
		}
		return
	}
	if gotFloat, ok := got.(float64); ok {
		if wantFloat, ok := want.(float64); ok {
			if math.IsNaN(gotFloat) && math.IsNaN(wantFloat) {
				return
			}
			if gotFloat == 0 && wantFloat == 0 && math.Signbit(gotFloat) != math.Signbit(wantFloat) {
				t.Fatalf("decoded value = %v with signbit %v, want signbit %v", gotFloat, math.Signbit(gotFloat), math.Signbit(wantFloat))
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded value = %#v, want %#v", got, want)
	}
}

// encodedRootOpcode returns the first scalar/tree opcode from an encoded root
// document.
func encodedRootOpcode(t *testing.T, doc common.B1Array) common.UB1 {
	t.Helper()
	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}
	opcode, err := buf.readUB1At(header.treeSegmentOffset())
	if err != nil {
		t.Fatalf("read root opcode returned error: %v", err)
	}
	return opcode
}

// signedIntOpcode returns the compact signed-integer opcode for a known opcode
// family and value payload.
func signedIntOpcode(t *testing.T, value int64, opcodeMask common.UB1) common.UB1 {
	t.Helper()

	payload, err := converters.EncodeInt(value)
	if err != nil {
		t.Fatalf("EncodeInt() error = %v", err)
	}
	return opcodeMask | common.UB1(len(payload))
}

// unsignedOracleNumberOpcode returns the expected compact or explicit Oracle
// NUMBER opcode for an unsigned integer payload.
func unsignedOracleNumberOpcode(t *testing.T, value uint64) common.UB1 {
	t.Helper()

	payload, err := converters.EncodeUInt(value)
	if err != nil {
		t.Fatalf("EncodeUInt() error = %v", err)
	}
	if len(payload) <= _compactOracleNumberMaxPayloadLen {
		return osonOpCompactOracleNumberPrefix | common.UB1(len(payload)-1)
	}
	return osonOpOracleNumber
}

// bytesForTest returns deterministic binary payload bytes of the requested
// length.
func bytesForTest(length int) []byte {
	value := make([]byte, length)
	for i := range value {
		value[i] = byte(i)
	}
	return value
}
