package converters

import (
	"math"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestVectorFloat64RoundTrip(t *testing.T) {
	input := driverCommon.VectorFloat64{1.25, -2.5, 0, math.Pi}
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

func TestVectorPublicEncodersValidateTypeAndNil(t *testing.T) {
	tests := []struct {
		name   string
		encode func(any) (driverCommon.B1Array, error)
		nil    any
	}{
		{"float64", func(v any) (driverCommon.B1Array, error) { return EncodeVectorFloat64(v) }, driverCommon.VectorFloat64(nil)},
		{"float32", func(v any) (driverCommon.B1Array, error) { return EncodeVectorFloat32(v) }, driverCommon.VectorFloat32(nil)},
		{"int8", func(v any) (driverCommon.B1Array, error) { return EncodeVectorInt8(v) }, driverCommon.VectorInt8(nil)},
		{"binary", func(v any) (driverCommon.B1Array, error) { return EncodeVectorBinary(v) }, driverCommon.VectorBinary(nil)},
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
}

func TestVectorDecodeWireVariantsAndBounds(t *testing.T) {
	valid, err := EncodeVectorFloat32(driverCommon.VectorFloat32{1})
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
	if _, err := encodeVectorFloat64(make([]float64, maxVectorDimensions+1)); err == nil {
		t.Fatal("expected FLOAT64 encode bound error")
	}
	if _, err := encodeVectorFloat32(make([]float32, maxVectorDimensions+1)); err == nil {
		t.Fatal("expected FLOAT32 encode bound error")
	}
	if _, err := encodeVectorInt8(make([]int8, maxVectorDimensions+1)); err == nil {
		t.Fatal("expected INT8 encode bound error")
	}
	if _, err := encodeVectorBinary(make(driverCommon.VectorBinary, maxVectorBinaryDimensions/8+1)); err == nil {
		t.Fatal("expected BINARY encode bound error")
	}
}

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

func TestVectorFloat32RoundTrip(t *testing.T) {
	input := driverCommon.VectorFloat32{1.5, -2.25, 0, 3.75}
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

func TestVectorInt8RoundTrip(t *testing.T) {
	input := driverCommon.VectorInt8{1, -2, 0, 127, -128}
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

func TestVectorBinaryRoundTrip(t *testing.T) {
	input := driverCommon.VectorBinary{0xA5, 0x0F}
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

func TestDecodeVectorRejectsMalformedPayload(t *testing.T) {
	valid, err := EncodeVectorFloat32(driverCommon.VectorFloat32{1})
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
