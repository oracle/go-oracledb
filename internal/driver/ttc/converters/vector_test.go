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
** is contributed to or provided by such licensors),
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

package converters

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestVectorFloat64RoundTrip verifies FLOAT64 values survive VECTOR encoding and decoding.
func TestVectorFloat64RoundTrip(t *testing.T) {
	input := datatype.VectorFloat64{1.25, -2.5, 0, math.Pi}
	wire, err := EncodeVectorFloat64(input)
	if err != nil {
		t.Fatalf("EncodeVectorFloat64 failed: %v", err)
	}

	got, err := DecodeVector(wire)
	if err != nil {
		t.Fatalf("DecodeVectorColumn failed: %v", err)
	}
	values, ok := got.([]float64)
	if !ok {
		t.Fatalf("decoded type mismatch: got %T want []float64", got)
	}
	if len(values) != len(input) {
		t.Fatalf("decoded length mismatch: got %d want %d", len(values), len(input))
	}
	for i := range input {
		if values[i] != input[i] {
			t.Fatalf("decoded value mismatch at %d: got %v want %v", i, values[i], input[i])
		}
	}
}

// TestVectorSpecialFloatRoundTrip verifies the codec preserves the IEEE class
// and sign of NaN and infinity without requiring a special encoding path.
func TestVectorSpecialFloatRoundTrip(t *testing.T) {
	float64Wire, err := EncodeVectorFloat64(datatype.VectorFloat64{math.NaN(), math.Inf(1), math.Inf(-1)})
	if err != nil {
		t.Fatalf("encode FLOAT64 special values: %v", err)
	}
	float64Values, err := DecodeVector(float64Wire)
	if err != nil {
		t.Fatalf("decode FLOAT64 special values: %v", err)
	}
	got64 := float64Values.([]float64)
	if !math.IsNaN(got64[0]) || !math.IsInf(got64[1], 1) || !math.IsInf(got64[2], -1) {
		t.Fatalf("FLOAT64 special values = %v", got64)
	}

	float32Wire, err := EncodeVectorFloat32(datatype.VectorFloat32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))})
	if err != nil {
		t.Fatalf("encode FLOAT32 special values: %v", err)
	}
	float32Values, err := DecodeVector(float32Wire)
	if err != nil {
		t.Fatalf("decode FLOAT32 special values: %v", err)
	}
	got32 := float32Values.([]float32)
	if !math.IsNaN(float64(got32[0])) || !math.IsInf(float64(got32[1]), 1) || !math.IsInf(float64(got32[2]), -1) {
		t.Fatalf("FLOAT32 special values = %v", got32)
	}
}

// TestVectorPublicEncodersValidateTypeAndNil verifies public encoders reject
// mismatched values and preserve typed nil vectors as SQL NULL.
func TestVectorPublicEncodersValidateTypeAndNil(t *testing.T) {
	tests := []struct {
		name   string
		encode func(any) (driverCommon.B1Array, error)
		nil    any
	}{
		{"float64", func(v any) (driverCommon.B1Array, error) { return EncodeVectorFloat64(v) }, datatype.VectorFloat64(nil)},
		{"float32", func(v any) (driverCommon.B1Array, error) { return EncodeVectorFloat32(v) }, datatype.VectorFloat32(nil)},
		{"int8", func(v any) (driverCommon.B1Array, error) { return EncodeVectorInt8(v) }, datatype.VectorInt8(nil)},
		{"binary", func(v any) (driverCommon.B1Array, error) { return EncodeVectorBinary(v) }, datatype.VectorBinary(nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := tc.encode(tc.nil); err != nil || got != nil {
				t.Fatalf("nil vector: got %v, err=%v", got, err)
			}
			if _, err := tc.encode("not a vector"); err == nil {
				t.Fatal("expected type validation error")
			}
		})
	}
	if got, err := DecodeVector(nil); err != nil || got != nil {
		t.Fatalf("nil vector decode: got %v, err=%v", got, err)
	}
	emptyWire, err := EncodeVectorFloat32(datatype.VectorFloat32{})
	if err != nil {
		t.Fatalf("encode empty vector: %v", err)
	}
	empty, err := DecodeVector(emptyWire)
	if err != nil {
		t.Fatalf("decode empty vector: %v", err)
	}
	if values, ok := empty.([]float32); !ok || values == nil || len(values) != 0 {
		t.Fatalf("empty vector decode: got %#v", empty)
	}
	if _, err := DecodeVector(driverCommon.B1Array{}); err == nil {
		t.Fatal("empty payload must not be treated as an empty vector")
	}
}

// TestVectorDecodeWireVariantsAndBounds verifies unsupported flags, versions,
// types, and dimension counts are rejected.
func TestVectorDecodeWireVariantsAndBounds(t *testing.T) {
	valid, err := EncodeVectorFloat32(datatype.VectorFloat32{1})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]byte){
		func(data []byte) { data[2] |= 0x80 },
		func(data []byte) { data[1] = 1 },
		func(data []byte) { data[4] = 0xFF },
		func(data []byte) { data[2], data[3] = 0, 0 }, // omitted norm field changes expected length
	} {
		data := append([]byte(nil), valid...)
		mutate(data)
		if _, err := DecodeVector(data); err == nil {
			t.Fatal("expected malformed vector error")
		}
	}
	if _, err := vectorValueByteLength(vectorTypeBinary, 7); err == nil {
		t.Fatal("expected unaligned binary dimension error")
	}
	if _, err := vectorValueByteLength(vectorTypeFloat64, maxVectorDimensions+1); err == nil {
		t.Fatal("expected out-of-range dimension error")
	}
	for _, typeCode := range []byte{vectorTypeFloat32, vectorTypeInt8, vectorTypeBinary} {
		if _, err := vectorValueByteLength(typeCode, maxVectorDimensions+1); err == nil {
			t.Fatalf("type %d: expected out-of-range dimension error", typeCode)
		}
	}
	if _, err := vectorValueByteLength(0xFF, 1); err == nil {
		t.Fatal("expected unknown type error")
	}
	if _, err := EncodeVectorFloat64(make(datatype.VectorFloat64, maxVectorDimensions+1)); err == nil {
		t.Fatal("expected FLOAT64 encode bound error")
	}
	if _, err := EncodeVectorFloat32(make(datatype.VectorFloat32, maxVectorDimensions+1)); err == nil {
		t.Fatal("expected FLOAT32 encode bound error")
	}
	if _, err := EncodeVectorInt8(make(datatype.VectorInt8, maxVectorDimensions+1)); err == nil {
		t.Fatal("expected INT8 encode bound error")
	}
	if _, err := EncodeVectorBinary(make(datatype.VectorBinary, maxVectorBinaryDimensions/vectorBitsPerByte+1)); err == nil {
		t.Fatal("expected BINARY encode bound error")
	}
}

// TestVectorIEEEAndLittleEndianDimensions verifies IEEE and little-endian flags.
func TestVectorIEEEAndLittleEndianDimensions(t *testing.T) {
	if value, err := decodeFloat64Dimension([]byte{0, 0, 0, 0, 0, 0, 0xF0, 0x3F}, vectorFlagIEEE|vectorFlagLittle); err != nil || value != 1 {
		t.Fatalf("float64 IEEE little-endian: value=%v err=%v", value, err)
	}
	if value, err := decodeFloat32Dimension([]byte{0, 0, 0x20, 0x40}, vectorFlagIEEE|vectorFlagLittle); err != nil || value != 2.5 {
		t.Fatalf("float32 IEEE little-endian: value=%v err=%v", value, err)
	}
	if _, err := decodeFloat64Dimension(nil, 0); err == nil {
		t.Fatal("expected invalid float64 width")
	}
	if _, err := decodeFloat32Dimension(nil, 0); err == nil {
		t.Fatal("expected invalid float32 width")
	}
}

// TestVectorDecodeEndianRepresentations checks complete envelopes containing
// multiple ordered or IEEE dimensions in both supported byte orders.
func TestVectorDecodeEndianRepresentations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		ieee   bool
		little bool
	}{
		{"float32 ordered big", 4, false, false},
		{"float32 ordered little", 4, false, true},
		{"float32 IEEE big", 4, true, false},
		{"float32 IEEE little", 4, true, true},
		{"float64 ordered big", 8, false, false},
		{"float64 ordered little", 8, false, true},
		{"float64 IEEE big", 8, true, false},
		{"float64 IEEE little", 8, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := uint16(vectorFlagNormSource)
			if tc.ieee {
				flags |= vectorFlagIEEE
			}
			if tc.little {
				flags |= vectorFlagLittle
			}
			typeCode := byte(vectorTypeFloat32)
			if tc.width == 8 {
				typeCode = vectorTypeFloat64
			}
			wire := make([]byte, vectorHeaderBytes+2*tc.width)
			writeVectorHeader(wire, vectorVersionV0, flags, typeCode, 2)
			values := []float64{1.25, -2.5}
			for i, value := range values {
				dst := wire[vectorHeaderBytes+i*tc.width:]
				if tc.width == 4 {
					bits := math.Float32bits(float32(value))
					if !tc.ieee {
						encoded, _ := EncodeBinaryFloat(float32(value))
						bits = binary.BigEndian.Uint32(encoded)
					}
					if tc.little {
						binary.LittleEndian.PutUint32(dst, bits)
					} else {
						binary.BigEndian.PutUint32(dst, bits)
					}
				} else {
					bits := math.Float64bits(value)
					if !tc.ieee {
						encoded, _ := EncodeBinaryDouble(value)
						bits = binary.BigEndian.Uint64(encoded)
					}
					if tc.little {
						binary.LittleEndian.PutUint64(dst, bits)
					} else {
						binary.BigEndian.PutUint64(dst, bits)
					}
				}
			}
			got, err := DecodeVector(wire)
			if err != nil {
				t.Fatal(err)
			}
			var want any = values
			if tc.width == 4 {
				want = []float32{1.25, -2.5}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

// TestVectorFloat32RoundTrip verifies FLOAT32 values survive VECTOR encoding and decoding.
func TestVectorFloat32RoundTrip(t *testing.T) {
	input := datatype.VectorFloat32{1.5, -2.25, 0, 3.75}
	wire, err := EncodeVectorFloat32(input)
	if err != nil {
		t.Fatalf("EncodeVectorFloat32 failed: %v", err)
	}

	got, err := DecodeVector(wire)
	if err != nil {
		t.Fatalf("DecodeVectorColumn failed: %v", err)
	}
	values, ok := got.([]float32)
	if !ok {
		t.Fatalf("decoded type mismatch: got %T want []float32", got)
	}
	if len(values) != len(input) {
		t.Fatalf("decoded length mismatch: got %d want %d", len(values), len(input))
	}
	for i := range input {
		if values[i] != input[i] {
			t.Fatalf("decoded value mismatch at %d: got %v want %v", i, values[i], input[i])
		}
	}
}

// TestVectorInt8RoundTrip verifies INT8 values survive VECTOR encoding and decoding.
func TestVectorInt8RoundTrip(t *testing.T) {
	input := datatype.VectorInt8{1, -2, 0, 127, -128}
	wire, err := EncodeVectorInt8(input)
	if err != nil {
		t.Fatalf("EncodeVectorInt8 failed: %v", err)
	}

	got, err := DecodeVector(wire)
	if err != nil {
		t.Fatalf("DecodeVectorColumn failed: %v", err)
	}
	values, ok := got.([]int8)
	if !ok {
		t.Fatalf("decoded type mismatch: got %T want []int8", got)
	}
	if len(values) != len(input) {
		t.Fatalf("decoded length mismatch: got %d want %d", len(values), len(input))
	}
	for i := range input {
		if values[i] != input[i] {
			t.Fatalf("decoded value mismatch at %d: got %v want %v", i, values[i], input[i])
		}
	}
}

// TestVectorBinaryRoundTrip verifies packed BINARY bytes survive VECTOR encoding and decoding.
func TestVectorBinaryRoundTrip(t *testing.T) {
	input := datatype.VectorBinary{0xA5, 0x0F}
	wire, err := EncodeVectorBinary(input)
	if err != nil {
		t.Fatalf("EncodeVectorBinary failed: %v", err)
	}

	got, err := DecodeVector(wire)
	if err != nil {
		t.Fatalf("DecodeVectorColumn failed: %v", err)
	}
	values, ok := got.([]byte)
	if !ok {
		t.Fatalf("decoded type mismatch: got %T want []byte", got)
	}
	if len(values) != len(input) {
		t.Fatalf("decoded length mismatch: got %d want %d", len(values), len(input))
	}
	for i := range input {
		if values[i] != input[i] {
			t.Fatalf("decoded value mismatch at %d: got %v want %v", i, values[i], input[i])
		}
	}
}

// TestDecodeVectorRejectsMalformedPayload verifies malformed envelopes are rejected.
func TestDecodeVectorRejectsMalformedPayload(t *testing.T) {
	valid, err := EncodeVectorFloat32(datatype.VectorFloat32{1})
	if err != nil {
		t.Fatalf("encode valid vector: %v", err)
	}

	tests := []struct {
		name string
		data driverCommon.B1Array
	}{
		{name: "short header", data: driverCommon.B1Array{vectorMagic}},
		{name: "invalid magic", data: append(driverCommon.B1Array{0}, valid[1:]...)},
		{name: "sparse vector", data: func() driverCommon.B1Array {
			data := append(driverCommon.B1Array(nil), valid...)
			data[3] |= byte(vectorFlagSparse)
			return data
		}()},
		{name: "invalid payload length", data: valid[:len(valid)-1]},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeVector(tc.data); err == nil {
				t.Fatal("expected malformed vector to fail decoding")
			}
		})
	}
}
