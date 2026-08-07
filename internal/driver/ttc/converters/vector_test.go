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
