package converters

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

func TestVectorFloat64RoundTrip(t *testing.T) {
	input := common.VectorFloat64{1.25, -2.5, 0, math.Pi}
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

// TestDecodeVectorWithoutNormField verifies decoding vectors whose values begin
// immediately after the base header because no norm metadata was sent.
func TestDecodeVectorWithoutNormField(t *testing.T) {
	withNorm, err := EncodeVectorFloat32(common.VectorFloat32{1.5, -2.25})
	if err != nil {
		t.Fatalf("EncodeVectorFloat32 failed: %v", err)
	}

	// A VECTOR may omit NORM and NORMSRC. Its values then begin immediately
	// after the nine-byte base header instead of after the padded norm field.
	withoutNorm := make(common.B1Array, vectorBaseHeaderBytes+len(withNorm)-vectorHeaderBytes)
	copy(withoutNorm[:vectorBaseHeaderBytes], withNorm[:vectorBaseHeaderBytes])
	binary.BigEndian.PutUint16(withoutNorm[2:4], 0)
	copy(withoutNorm[vectorBaseHeaderBytes:], withNorm[vectorHeaderBytes:])

	got, err := DecodeVector(withoutNorm)
	if err != nil {
		t.Fatalf("DecodeVector failed: %v", err)
	}
	values, ok := got.([]float32)
	if !ok {
		t.Fatalf("decoded type mismatch: got %T want []float32", got)
	}
	want := []float32{1.5, -2.25}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("decoded value mismatch at %d: got %v want %v", i, values[i], want[i])
		}
	}
}

func TestVectorFloat32RoundTrip(t *testing.T) {
	input := common.VectorFloat32{1.5, -2.25, 0, 3.75}
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
	input := common.VectorInt8{1, -2, 0, 127, -128}
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
	input := common.VectorBinary{0xA5, 0x0F}
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
	valid, err := EncodeVectorFloat32(common.VectorFloat32{1})
	if err != nil {
		t.Fatalf("encode valid vector: %v", err)
	}

	tests := []struct {
		name string
		data common.B1Array
	}{
		{name: "short header", data: common.B1Array{vectorMagic}},
		{name: "invalid magic", data: append(common.B1Array{0}, valid[1:]...)},
		{name: "sparse vector", data: func() common.B1Array {
			data := append(common.B1Array(nil), valid...)
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
