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

package converters

import (
	"errors"
	"math"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

// float64TestCase defines a single table-driven case for NUMBER/BINARY_* tests.
// It provides the input wire representation (or expected value for encode),
// precision/scale for NUMBER decoding, and the expected numeric result or error flag.
type float64TestCase struct {
	// name is a human-readable label to identify the test case in logs.
	name string
	// wire holds the Oracle wire representation for the value (NUMBER/BINARY_*).
	wire []byte
	// precision and scale parameterize NUMBER decoding where applicable.
	precision int
	scale     int
	// expected is the numeric value expected after decoding (and used for encode tests).
	expected float64
	// wantErr indicates this case should trigger an error on decode.
	wantErr bool
	// expCode is the expected Oracle error code when wantErr is true.
	expCode common.ErrorCode
}

// float64TestCases contains NUMBER wire <-> float64 pairs used for both encode and decode tests.
// Cases include positive/negative numbers, zero, tiny and very small magnitudes, and invalid input.
var float64TestCases = []float64TestCase{
	{name: "123.456", wire: []byte{0xC2, 0x02, 0x18, 0x2E, 0x3D}, precision: 6, scale: 3, expected: 123.456},
	{name: "minus_987.654", wire: []byte{0x3D, 0x5C, 0x0E, 0x24, 0x3D, 0x66}, precision: 6, scale: 3, expected: -987.654},
	{name: "zero", wire: []byte{0x80}, precision: 1, scale: 0, expected: 0.0},
	{name: "small", wire: []byte{0xB2, 0x02}, precision: 28, scale: 27, expected: 0.000000000000000000000000000001},
	{name: "empty", wire: []byte{}, precision: 1, scale: 0, expected: 0, wantErr: true, expCode: common.ConverterEmptyInput},
	{name: "tiny", wire: []byte{0xBD, 0x0B}, precision: 7, scale: 7, expected: 0.0000001},
}

// BinaryFloat32TestCases mirrors float64 cases but targets Oracle BINARY_FLOAT (float32).
// The expected field is still float64 for convenience in comparisons/tables.
var BinaryFloat32TestCases = []float64TestCase{
	{name: "123.456", wire: []byte{0xC2, 0xF6, 0xE9, 0x79}, precision: 6, scale: 3, expected: 123.456},
	{name: "minus_987.654", wire: []byte{0x3B, 0x89, 0x16, 0x24}, precision: 6, scale: 3, expected: -987.654},
	{name: "zero", wire: []byte{0x80, 0x00, 0x00, 0x00}, precision: 1, scale: 0, expected: 0.0},
	{name: "small", wire: []byte{0x8D, 0xA2, 0x42, 0x60}, precision: 28, scale: 27, expected: 0.000000000000000000000000000001},
	{name: "empty", wire: []byte{}, precision: 1, scale: 0, expected: 0, wantErr: true, expCode: common.ConverterExpectedFormat},
	{name: "tiny", wire: []byte{0xB3, 0xD6, 0xBF, 0x95}, precision: 7, scale: 7, expected: 0.0000001},
}

// BinaryDoubleTestCases contains Oracle BINARY_DOUBLE (float64) sortable encodings and their values.
var BinaryDoubleTestCases = []float64TestCase{
	{name: "123.456", wire: []byte{0xC0, 0x5E, 0xDD, 0x2F, 0x1A, 0x9F, 0xBE, 0x77}, precision: 6, scale: 3, expected: 123.456},
	{name: "minus_987.654", wire: []byte{0x3F, 0x71, 0x22, 0xC4, 0x9B, 0xA5, 0xE3, 0x53}, precision: 6, scale: 3, expected: -987.654},
	{name: "zero", wire: []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, precision: 1, scale: 0, expected: 0.0},
	{name: "small", wire: []byte{0xB9, 0xB4, 0x48, 0x4B, 0xFE, 0xEB, 0xC2, 0xA0}, precision: 28, scale: 27, expected: 0.000000000000000000000000000001},
	{name: "empty", wire: []byte{}, precision: 1, scale: 0, expected: 0, wantErr: true, expCode: common.ConverterExpectedFormat},
	{name: "tiny", wire: []byte{0xBE, 0x7A, 0xD7, 0xF2, 0x9A, 0xBC, 0xAF, 0x48}, precision: 7, scale: 7, expected: 0.0000001},
}

// TestFloat_Decode verifies decoding Oracle NUMBER into float64 across table-driven cases.
// It exercises zero, positive/negative numbers, tiny magnitudes, and error scenarios.
func TestFloat_Decode(t *testing.T) {
	t.Parallel()
	for _, tc := range float64TestCases {
		// Act
		got, err := DecodeFloat(tc.wire, tc.precision, tc.scale)

		// Assert: expected error and code
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error but got none", tc.name)
				continue
			}
			if sqle, ok := errors.AsType[common.SQLError](err); ok {
				if tc.expCode != "" && sqle.ErrorCode() != string(tc.expCode) {
					t.Errorf("%s: want error code %s, got %s", tc.name, tc.expCode, sqle.ErrorCode())
				}
			} else {
				t.Errorf("%s: error is not SQLError: %v", tc.name, err)
			}
			continue
		}
		// Assert: unexpected error
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
			continue
		}
		// Assert: numeric equality within tolerance for non-error cases
		if math.Abs(got-tc.expected) > 1e-9 {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.expected)
		}
	}
}

// TestFloat_Encode checks that Encode produces Oracle NUMBER wire that matches the decode table.
// It ensures round-trip compatibility by comparing produced wire bytes with reference encodings.
func TestFloat_Encode(t *testing.T) {
	t.Parallel()
	for _, tc := range float64TestCases {
		// Skip cases intended as decode-only error scenarios
		if tc.wantErr {
			continue
		}

		// Act
		wire, err := EncodeFloat(tc.expected)
		t.Logf("EncodeFloat %s: %v", tc.name, wire)

		// Assert: expected wire length and no error
		if len(wire) != len(tc.wire) || err != nil {
			t.Errorf("%s: EncodeFloat64 wrong length: got %v, want %v", tc.name, wire, tc.wire)
			continue
		}
		// Assert: per-byte equality
		for i := range wire {
			if wire[i] != tc.wire[i] {
				t.Errorf("%s: EncodeFloat64 mismatch at byte %d: got 0x%X, want 0x%X; full got %v, want %v",
					tc.name, i, wire[i], tc.wire[i], wire, tc.wire)
				break
			}
		}
	}
}

// TestBinaryFloat_Encode validates the sortable-encoding transform for Oracle BINARY_FLOAT.
// It compares the produced big-endian bytes with precomputed references.
func TestBinaryFloat_Encode(t *testing.T) {
	t.Parallel()
	for _, tc := range BinaryFloat32TestCases {
		// Act
		got, _ := EncodeBinaryFloat(float32(tc.expected))
		t.Logf("BinaryFloat Encode %s: value=%v wire=%v", tc.name, tc.expected, got)

		// Assert: fixed wire length
		if len(got) != 4 {
			t.Errorf("%s: got wrong length %v bytes", tc.name, got)
			continue
		}
		// Assert: per-byte equality
		for i := range tc.wire {
			if got[i] != tc.wire[i] {
				t.Errorf("%s: EncodeBinaryFloat got %v, want %v", tc.name, got, tc.wire)
				break
			}
		}
	}
}

// TestBinaryFloat_Decode verifies reversibility of the BINARY_FLOAT sortable encoding.
// It also checks error handling on invalid input length.
func TestBinaryFloat_Decode(t *testing.T) {
	t.Parallel()
	for _, tc := range BinaryFloat32TestCases {
		// Act
		got, err := DecodeBinaryFloat(tc.wire)

		// Assert: error pathway
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error but got none", tc.name)
				continue
			}
			if sqle, ok := errors.AsType[common.SQLError](err); ok {
				if tc.expCode != "" && sqle.ErrorCode() != string(tc.expCode) {
					t.Errorf("%s: want error code %s, got %s", tc.name, tc.expCode, sqle.ErrorCode())
				}
			} else {
				t.Errorf("%s: error is not SQLError: %v", tc.name, err)
			}
			continue
		}
		// Assert: unexpected error
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}

		t.Logf("BinaryFloat Decode %s: wire=%v value=%v", tc.name, tc.wire, got)

		// Assert: NaN handling (cannot compare directly)
		if math.IsNaN(float64(tc.expected)) && !math.IsNaN(float64(got)) {
			t.Errorf("%s: got %v, want NaN", tc.name, got)
			continue
		}
		// Assert: numeric equality within tolerance for finite numbers
		if !math.IsNaN(float64(tc.expected)) && math.Abs(float64(got)-float64(tc.expected)) > 1e-5 {
			t.Errorf("%s: got %v, want %v , diff: %v", tc.name, got, tc.expected, math.Abs(float64(got)-float64(tc.expected)))
		}
	}
}

// TestBinaryDouble_Encode validates the sortable-encoding transform for Oracle BINARY_DOUBLE.
// It compares the produced big-endian bytes with precomputed references.
func TestBinaryDouble_Encode(t *testing.T) {
	t.Parallel()
	for _, tc := range BinaryDoubleTestCases {
		// Act
		got, _ := EncodeBinaryDouble(tc.expected)
		t.Logf("BinaryDouble Encode->Decode %s: value=%v wire=%v", tc.name, tc.expected, got)

		// Assert: fixed wire length
		if len(got) != 8 {
			t.Errorf("%s: got wrong length %v bytes", tc.name, got)
			continue
		}
		// Assert: per-byte equality
		for i := range tc.wire {
			if got[i] != tc.wire[i] {
				t.Errorf("%s: EncodeBinaryDouble got %v, want %v", tc.name, got, tc.wire)
				break
			}
		}
	}
}

// TestBinaryDouble_Decode verifies reversibility of the BINARY_DOUBLE sortable encoding.
// It also checks error handling on invalid input length.
func TestBinaryDouble_Decode(t *testing.T) {
	t.Parallel()
	for _, tc := range BinaryDoubleTestCases {
		// Act
		got, err := DecodeBinaryDouble(tc.wire)

		// Assert: error pathway
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error but got none", tc.name)
				continue
			}
			if sqle, ok := errors.AsType[common.SQLError](err); ok {
				if tc.expCode != "" && sqle.ErrorCode() != string(tc.expCode) {
					t.Errorf("%s: want error code %s, got %s", tc.name, tc.expCode, sqle.ErrorCode())
				}
			} else {
				t.Errorf("%s: error is not SQLError: %v", tc.name, err)
			}
			continue
		}
		// Assert: unexpected error
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}

		t.Logf("BinaryDouble Decode->Encode %s: wire=%v value=%v", tc.name, tc.wire, got)

		// Assert: NaN handling (cannot compare directly)
		if math.IsNaN(tc.expected) && !math.IsNaN(got) {
			t.Errorf("%s: got %v, want NaN", tc.name, got)
			continue
		}
		// Assert: numeric equality within tight tolerance for float64
		if !math.IsNaN(tc.expected) && math.Abs(got-tc.expected) > 1e-9 {
			t.Errorf("%s: got %v, want %v , diff: %v", tc.name, got, tc.expected, math.Abs(got-tc.expected))
		}
	}
}

// TestFloat_Encode_Specials ensures EncodeFloat rejects NaN and ±Inf which are not representable as NUMBER.
func TestFloat_Encode_Specials(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		val  float64
	}{
		{"nan", math.NaN()},
		{"+inf", math.Inf(1)},
		{"-inf", math.Inf(-1)},
	}
	for _, c := range cases {
		_, err := EncodeFloat(c.val)
		if err == nil {
			t.Errorf("EncodeFloat should error for %s", c.name)
			continue
		}
		if sqle, ok := errors.AsType[common.SQLError](err); ok {
			if sqle.ErrorCode() != string(common.ConverterExpectedFormat) {
				t.Errorf("EncodeFloat(%s): want error code %s, got %s", c.name, common.ConverterExpectedFormat, sqle.ErrorCode())
			}
		} else {
			t.Errorf("EncodeFloat(%s): error is not SQLError: %v", c.name, err)
		}
	}
}
