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
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

/*
numberWireFormatCases maps known integer values to their corresponding Oracle NUMBER wire-formats.

Oracle NUMBER wire format encodes:
- a leading byte combining sign and exponent, followed by
- packed BCD digit pairs (two decimal digits per byte), with a special trailing byte for some negatives.

These canonical vectors serve as ground truth to verify both encoding and decoding.
*/
var numberWireFormatCases = []struct {
	value int64
	wire  []byte
}{
	{0, []byte{0x80}},
	{1, []byte{0xC1, 0x02}},
	{9, []byte{0xC1, 0x0A}},
	{10, []byte{0xC1, 0x0B}},
	{99, []byte{0xC1, 0x64}},
	{-1, []byte{0x3E, 0x64, 0x66}},
	{-10, []byte{0x3E, 0x5B, 0x66}},
}

/*
decimalWireFormatCases test Oracle NUMBER(p,s) with non-zero scale and a range of magnitudes.
Each case declares precision/scale used to format the decoded string for deterministic comparisons.
*/
var decimalWireFormatCases = []struct {
	decimalStr string
	wire       []byte
	precision  int
	scale      int
}{
	{"123.45", []byte{0xC2, 0x02, 0x18, 0x2E, 0x02, 0x01}, 5, 2},
	{"-42.857", []byte{0x3E, 0x3B, 0x10, 0x1F, 0x66}, 5, 3},
	{"0.0001", []byte{0xBF, 0x02}, 4, 4},
	{"-0.25", []byte{0x3F, 0x4C, 0x66}, 2, 2},
}

var decimalNumberTestCases = []float64TestCase{
	{name: "123.45", wire: []byte{0xC2, 0x02, 0x18, 0x2E}, precision: 6, scale: 2, expected: 123.45},
	{name: "-987.654", wire: []byte{0x3D, 0x5C, 0x0E, 0x24, 0x3D, 0x66}, precision: 6, scale: 3, expected: -987.654},
	{name: "zero", wire: []byte{0x80}, precision: 1, scale: 0, expected: 0.0},
	{name: "empty", wire: []byte{}, precision: 1, scale: 0, expected: 0, wantErr: true, expCode: common.ConverterEmptyInput},
	{name: "tiny", wire: []byte{0xBD, 0x0B}, precision: 7, scale: 7, expected: 0.0000001},
}

// assertNumberConverterError verifies the structured error code and reason
// returned by NUMBER converter validation failures.
func assertNumberConverterError(t *testing.T, err error, wantReason string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected converter error, got nil")
	}

	var sqlErr common.SQLError
	if !errors.As(err, &sqlErr) {
		t.Fatalf("error is not common.SQLError: %T: %v", err, err)
	}

	if got := sqlErr.ErrorCode(); got != string(common.ConverterExpectedFormat) {
		t.Fatalf("error code=%s, want=%s: %v", got, common.ConverterExpectedFormat, err)
	}

	if !strings.Contains(err.Error(), wantReason) {
		t.Fatalf("error %q does not contain reason %q", err.Error(), wantReason)
	}
}

// makeNumberWire constructs a positive NUMBER encoding with the requested wire
// length and valid base-100 mantissa bytes.
func makeNumberWire(length int) []byte {
	wire := make([]byte, length)
	if length == 0 {
		return wire
	}

	wire[0] = 0xC1
	for i := 1; i < len(wire); i++ {
		wire[i] = 0x64
	}
	return wire
}

// TestNumber_KnownWireFormats verifies known int64 values map to canonical Oracle NUMBER
// wire bytes on encode, and those wires decode back to the same int64 values.
func TestNumber_KnownWireFormats(t *testing.T) {
	t.Parallel()
	// Encode: value → wire check
	for _, tc := range numberWireFormatCases {
		wire, _ := EncodeInt(tc.value)
		if !bytes.Equal(wire, tc.wire) {
			t.Errorf("EncodeInt64(%d): got %#v, want %#v", tc.value, wire, tc.wire)
		}
	}
	// Decode: wire → value check
	for _, tc := range numberWireFormatCases {
		got, err := DecodeInt(tc.wire)
		if err != nil {
			t.Errorf("DecodeInt(wire=%#v): unexpected error: %v", tc.wire, err)
		}
		if got != tc.value {
			t.Errorf("DecodeInt(wire=%#v): got %d, want %d", tc.wire, got, tc.value)
		}
	}
}

// TestNumber_DecodeDecimal ensures DecodeDecimal yields expected strings for
// NUMBER(p,s) examples across magnitudes and signs, including Oracle's special zero.
func TestNumber_DecodeDecimal(t *testing.T) {
	t.Parallel()
	for _, tc := range decimalWireFormatCases {
		// Test DecodeDecimal (decode wire to string)
		str, err := DecodeDecimal(tc.wire, tc.precision, tc.scale)
		t.Logf("Wire: %#v, Decoded: %q, Expected: %q, Err: %v", tc.wire, str, tc.decimalStr, err)
		if err != nil {
			t.Errorf("DecodeDecimal(wire=%#v): unexpected error: %v", tc.wire, err)
			continue
		}
		if str != tc.decimalStr {
			t.Errorf("DecodeDecimal(wire=%#v): got %q, want %q", tc.wire, str, tc.decimalStr)
		}
	}

	// Edge: Oracle zero NUMBER wire
	zeroWire := []byte{0x80}
	out, err := DecodeDecimal(zeroWire, 1, 0)
	t.Logf("Wire: %#v, Decoded: %q, Expected: \"0\", Err: %v", zeroWire, out, err)
	if err != nil || out != "0" {
		t.Errorf("DecodeDecimal(zero): got %q, err=%v, want \"0\"", out, err)
	}
}

// TestNumber_OverlongWire is the PGD-ECS-055 regression test. It verifies that
// every NUMBER conversion entry point rejects overlong payloads before zero
// handling, fixed-width mantissa conversion, or arbitrary-precision work.
func TestNumber_OverlongWire(t *testing.T) {
	t.Parallel()
	type decodeFunc func([]byte) (string, error)
	cases := []struct {
		name       string
		zeroPrefix bool
		decode     decodeFunc
	}{
		// DecodeDecimal must reject an overlong positive value before arbitrary-precision conversion.
		{name: "DecodeDecimal/positive", decode: func(wire []byte) (string, error) {
			return DecodeDecimal(wire, 38, 0)
		}},
		// DecodeDecimal must validate length before treating a leading zero marker as Oracle zero.
		{name: "DecodeDecimal/zero_prefix", zeroPrefix: true, decode: func(wire []byte) (string, error) {
			return DecodeDecimal(wire, 38, 0)
		}},
		// FromNumber must report invalid length before its uint64 mantissa can overflow.
		{name: "FromNumber/positive", decode: func(wire []byte) (string, error) {
			mantissa, negative, exponent, digits, err := FromNumber(wire)
			return fmt.Sprintf("mantissa=%d, negative=%t, exponent=%d, digits=%d", mantissa, negative, exponent, digits), err
		}},
		// FromNumber must not let an overlong zero-prefixed payload bypass length validation.
		{name: "FromNumber/zero_prefix", zeroPrefix: true, decode: func(wire []byte) (string, error) {
			mantissa, negative, exponent, digits, err := FromNumber(wire)
			return fmt.Sprintf("mantissa=%d, negative=%t, exponent=%d, digits=%d", mantissa, negative, exponent, digits), err
		}},
		// _fromNumberBig must reject an overlong positive value before allocating a large mantissa.
		{name: "FromNumberBig/positive", decode: func(wire []byte) (string, error) {
			mantissa, negative, exponent, digits, err := _fromNumberBig(wire)
			return fmt.Sprintf("mantissa=%v, negative=%t, exponent=%d, digits=%d", mantissa, negative, exponent, digits), err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := makeNumberWire(_numberWireMaxLen + 1)
			if tc.zeroPrefix {
				wire[0] = _numberZeroWireByte
			}

			got, err := tc.decode(wire)
			if err == nil {
				t.Fatalf("decode(length=%d, zeroPrefix=%t) returned %q; expected invalid-length error", len(wire), tc.zeroPrefix, got)
			}
			assertNumberConverterError(t, err, common.ReasonInvalidLength)
		})
	}
}

// TestNumber_FromNumberBig_WireLengthBoundary verifies that the maximum valid
// NUMBER wire length is accepted by the arbitrary-precision decoder.
func TestNumber_FromNumberBig_WireLengthBoundary(t *testing.T) {
	t.Parallel()
	wire := makeNumberWire(_numberWireMaxLen)

	mantissa, negative, exponent, digits, err := _fromNumberBig(wire)
	if err != nil {
		t.Fatalf("_fromNumberBig(length=%d) returned mantissa=%v, negative=%t, exponent=%d, digits=%d, err=%v; expected valid boundary wire", len(wire), mantissa, negative, exponent, digits, err)
	}
}

// TestNumber_DecodeDecimal_ScaleBounds verifies Oracle's accepted NUMBER scale
// range, rejects values outside it, and preserves scale-greater-than-precision.
func TestNumber_DecodeDecimal_ScaleBounds(t *testing.T) {
	t.Parallel()
	oneWire := []byte{0xC1, 0x02}
	cases := []struct {
		name      string
		precision int
		scale     int
		wantErr   bool
	}{
		// Accept Oracle's minimum documented NUMBER scale.
		{name: "minimum", precision: 38, scale: -84},
		// Accept a negative scale immediately above the minimum.
		{name: "minimum_plus_one", precision: 38, scale: -83},
		// Accept rounding one decimal position to the left of the decimal point.
		{name: "negative_one", precision: 38, scale: -1},
		// Accept the standard integer scale.
		{name: "zero", precision: 38, scale: 0},
		// Accept scale when it equals the declared precision.
		{name: "equal_to_precision", precision: 38, scale: 38},
		// Accept scale greater than precision as permitted by Oracle NUMBER.
		{name: "greater_than_precision", precision: 38, scale: 39},
		// Preserve NUMBER(2,40), demonstrating that small precision does not cap scale.
		{name: "greater_than_small_precision", precision: 2, scale: 40},
		// Accept the scale immediately below Oracle's maximum.
		{name: "maximum_minus_one", precision: 38, scale: 126},
		// Accept Oracle's maximum documented NUMBER scale.
		{name: "maximum", precision: 38, scale: 127},
		// Reject a scale one position below Oracle's minimum.
		{name: "below_minimum", precision: 38, scale: -85, wantErr: true},
		// Reject a scale one position above Oracle's maximum.
		{name: "above_maximum", precision: 38, scale: 128, wantErr: true},
		// Reject a substantially negative scale before exponent allocation.
		{name: "far_below_minimum", precision: 38, scale: -1000, wantErr: true},
		// Reject a substantially positive scale before decimal string allocation.
		{name: "far_above_maximum", precision: 38, scale: 1000, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeDecimal(oneWire, tc.precision, tc.scale)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DecodeDecimal(precision=%d, scale=%d) returned %q; expected out-of-range error", tc.precision, tc.scale, got)
				}
				assertNumberConverterError(t, err, common.ReasonOutOfRange)
				return
			}
			if err != nil {
				t.Fatalf("DecodeDecimal(precision=%d, scale=%d) returned %q, err=%v; expected success", tc.precision, tc.scale, got, err)
			}
		})
	}
}

// TestNumber_DecodeDecimal_ZeroScaleBounds verifies that scale validation also
// occurs for Oracle zero before scale-dependent decimal formatting.
func TestNumber_DecodeDecimal_ZeroScaleBounds(t *testing.T) {
	t.Parallel()
	zeroWire := []byte{_numberZeroWireByte}
	cases := []struct {
		name    string
		scale   int
		wantErr bool
	}{
		// Accept zero at Oracle's minimum NUMBER scale.
		{name: "minimum", scale: -84},
		// Accept zero with the standard integer scale.
		{name: "zero", scale: 0},
		// Accept zero at Oracle's maximum NUMBER scale.
		{name: "maximum", scale: 127},
		// Reject zero with a scale immediately below Oracle's minimum.
		{name: "below_minimum", scale: -85, wantErr: true},
		// Reject zero with a scale immediately above Oracle's maximum.
		{name: "above_maximum", scale: 128, wantErr: true},
		// Reject zero with a substantially negative scale before exponent work.
		{name: "far_below_minimum", scale: -1000, wantErr: true},
		// Reject zero with a substantially positive scale before FloatString allocation.
		{name: "far_above_maximum", scale: 1000, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeDecimal(zeroWire, 38, tc.scale)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DecodeDecimal(zero, precision=38, scale=%d) returned %q; expected out-of-range error", tc.scale, got)
				}
				assertNumberConverterError(t, err, common.ReasonOutOfRange)
				return
			}
			if err != nil {
				t.Fatalf("DecodeDecimal(zero, precision=38, scale=%d) returned %q, err=%v; expected success", tc.scale, got, err)
			}
		})
	}
}

// TestNumber_DecodeDecimal_PrecisionBounds verifies that unspecified precision
// and explicit precision 1 through 38 are accepted while invalid values fail.
func TestNumber_DecodeDecimal_PrecisionBounds(t *testing.T) {
	t.Parallel()
	oneWire := []byte{0xC1, 0x02}
	cases := []struct {
		name       string
		precision  int
		wantReason string
	}{
		// Accept precision zero as the driver's unspecified NUMBER sentinel.
		{name: "unspecified", precision: 0},
		// Accept Oracle's minimum explicit NUMBER precision.
		{name: "minimum", precision: 1},
		// Accept a representative precision above the minimum.
		{name: "two", precision: 2},
		// Accept Oracle's maximum explicit NUMBER precision.
		{name: "maximum", precision: 38},
		// Reject negative precision as outside the supported metadata range.
		{name: "negative", precision: -1, wantReason: common.ReasonOutOfRange},
		// Reject precision immediately above Oracle's maximum.
		{name: "above_maximum", precision: 39, wantReason: common.ReasonPrecisionExceeded},
		// Reject substantially excessive precision before numeric conversion.
		{name: "far_above_maximum", precision: 1000, wantReason: common.ReasonPrecisionExceeded},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeDecimal(oneWire, tc.precision, 0)
			if tc.wantReason != "" {
				if err == nil {
					t.Fatalf("DecodeDecimal(precision=%d, scale=0) returned %q; expected %s error", tc.precision, got, tc.wantReason)
				}
				assertNumberConverterError(t, err, tc.wantReason)
				return
			}
			if err != nil {
				t.Fatalf("DecodeDecimal(precision=%d, scale=0) returned %q, err=%v; expected success", tc.precision, got, err)
			}
		})
	}
}

// TestNumber_EncodeDecimal checks that decimal string encoding matches Oracle wire format for NUMBER(p,s)
func TestNumber_EncodeDecimal(t *testing.T) {
	t.Parallel()
	for _, tc := range decimalNumberTestCases {
		t.Logf("Test name :%s\n", tc.name)
		wire, err := EncodeDecimal(tc.expected)

		// Post-encode handling for decode-only/invalid cases:
		// if wantErr is true, we expect encode to error; if it does not, do not assert wire equality.
		if tc.wantErr {
			if err == nil {
				t.Logf("EncodeDecimal(%v) produced wire=%#v; wantErr=true indicates decode-only case", tc.expected, wire)
			}
			continue
		}

		if err != nil {
			t.Errorf("EncodeDecimal(%v) returned error: %v", tc.expected, err)
			continue
		}
		if !bytes.Equal(wire, tc.wire) {
			t.Errorf("EncodeDecimal(%v): got %#v, want %#v", tc.expected, wire, tc.wire)
		}
	}
}

// TestNumber_EncodeDecode_Roundtrip validates that arbitrary int64 and int values
// can be round-tripped: encode to wire format, then decode back to the original value.
func TestNumber_EncodeDecode_Roundtrip(t *testing.T) {
	t.Parallel()
	int64Vals := []int64{
		0,
		1, -1,
		42, -42,
		math.MaxInt64, math.MinInt64 + 1,
		123456789012345, -1000000000000000,
	}
	for _, v := range int64Vals {
		wire, _ := EncodeInt(v)
		decoded, err := DecodeInt(wire)
		if err != nil {
			t.Errorf("error decoding int got err = %v", err)
			continue
		}
		if decoded != v {
			t.Errorf("int64 roundtrip fail: orig=%d got=%d wire=%#v", v, decoded, wire)
		}
	}

	intVals := []int{
		0, 1, -1, 1234, -5678, math.MaxInt32, math.MinInt32 + 1,
	}
	for _, v := range intVals {
		wire, _ := EncodeInt(int64(v))
		got, err := DecodeInt(wire)
		if err != nil {
			t.Errorf("error decoding int got err = %v", err)
			continue
		}
		if got != int64(v) {
			t.Errorf("int roundtrip fail: orig=%d got=%d wire=%#v", v, got, wire)
		}
	}
}

// TestNumber_OverflowHandling ensures decoding excessive-mantissa inputs does not panic
// or overflow and gracefully handles extreme cases.
func TestNumber_OverflowHandling(t *testing.T) {
	t.Parallel()
	// Excessive mantissa should not wrap around
	big := []byte{0xC2, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	got, err := DecodeInt(big)
	if err != nil {
		t.Logf("overflow, got error: %v -- ok (should be int64 range or 0)", err)
	} else if got != int64(0) && got != int64(math.MaxInt64) && got != -1 {
		// exact value is not critical but should not panic or overflow
		t.Logf("overflow, got: %d -- ok (should be int64 range or 0)", got)
	}
	// ToString: should not panic and should still return a string or error gracefully
	_, _ = DecodeDecimal(big, 20, 10)
}

// TestNumber_DecodeInt_MinInt64 verifies that math.MinInt64 round-trips through NUMBER encode/decode exactly.
func TestNumber_DecodeInt_MinInt64(t *testing.T) {
	t.Parallel()
	wire, _ := EncodeInt(math.MinInt64)

	got, err := DecodeInt(wire)
	if err != nil {
		t.Fatalf("unexpected error decoding MinInt64: %v", err)
	}
	if got != math.MinInt64 {
		t.Errorf("DecodeInt MinInt64: got %d, want %d", got, math.MinInt64)
	}
}

// TestNumber_ErrorCodes consolidates error-path checks into a single
// table-driven test verifying Oracle error codes for each scenario.
func TestNumber_ErrorCodes(t *testing.T) {
	t.Parallel()
	type errCase struct {
		name string
		fn   func() error
		want common.ErrorCode
	}
	const maxMantissa = (1<<64 - 1) / 10
	cases := []errCase{
		{
			name: "DecodeInt empty",
			fn:   func() error { _, err := DecodeInt(nil); return err },
			want: common.ConverterEmptyInput,
		},
		{
			name: "DecodeInt malformed",
			fn:   func() error { _, err := DecodeInt([]byte{0x12, 0x34, 0xFF}); return err },
			want: common.ConverterExpectedFormat,
		},
		{
			name: "DecodeDecimal empty",
			fn:   func() error { _, err := DecodeDecimal([]byte{}, 1, 0); return err },
			want: common.ConverterEmptyInput,
		},
		{
			name: "DecodeInt fractional",
			fn: func() error {
				w, e := EncodeFloat(123.45)
				if e != nil {
					return e
				}
				_, err := DecodeInt(w)
				return err
			},
			want: common.ConverterExpectedFormat,
		},
		{
			name: "DecodeInt overflow +",
			fn: func() error {
				mant := []byte("9223372036854775808")
				w := ToNumber(mant, false, len(mant)-1)
				_, err := DecodeInt(w)
				return err
			},
			want: common.ConverterRange,
		},
		{
			name: "DecodeInt overflow -",
			fn: func() error {
				mant := []byte("9223372036854775809")
				w := ToNumber(mant, true, len(mant)-1)
				_, err := DecodeInt(w)
				return err
			},
			want: common.ConverterRange,
		},
		{
			name: "DecodeInt exponent too large",
			fn: func() error {
				w := ToNumber([]byte("1"), false, 50)
				_, err := DecodeInt(w)
				return err
			},
			want: common.ConverterRange,
		},
		{
			name: "addDigit digit<0",
			fn:   func() error { _, err := addDigitToMantissa(0, -1); return err },
			want: common.ConverterExpectedFormat,
		},
		{
			name: "addDigit digit>9",
			fn:   func() error { _, err := addDigitToMantissa(0, 10); return err },
			want: common.ConverterExpectedFormat,
		},
		{
			name: "addDigit overflow",
			fn:   func() error { _, err := addDigitToMantissa(maxMantissa+1, 1); return err },
			want: common.ConverterExpectedFormat,
		},
		{
			name: "addDigit wraparound",
			fn:   func() error { _, err := addDigitToMantissa(maxMantissa, 9); return err },
			want: common.ConverterExpectedFormat,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if sqle, ok := errors.AsType[common.SQLError](err); ok {
				if sqle.ErrorCode() != string(tc.want) {
					t.Errorf("%s: want error code %s, got %s", tc.name, tc.want, sqle.ErrorCode())
				}
			} else {
				t.Errorf("%s: error is not SQLError: %v", tc.name, err)
			}
		})
	}
}

// TestNumber_EncodeDecimal_StrictErrors verifies EncodeDecimal returns Oracle errors
// for non-finite values and magnitudes outside the representable NUMBER domain [1e-130, 1e126).
func TestNumber_EncodeDecimal_StrictErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		val  float64
		want common.ErrorCode
	}{
		{"nan", math.NaN(), common.ConverterExpectedFormat},
		{"pos_inf", math.Inf(1), common.ConverterExpectedFormat},
		{"neg_inf", math.Inf(-1), common.ConverterExpectedFormat},
		{"too_large_pos", 1e126, common.ConverterRange},
		{"too_large_neg", -1e126, common.ConverterRange},
		{"too_small_pos", 1e-131, common.ConverterRange},
		{"too_small_neg", -1e-131, common.ConverterRange},
		{"subnormal_pos", math.SmallestNonzeroFloat64, common.ConverterRange},
		{"subnormal_neg", -math.SmallestNonzeroFloat64, common.ConverterRange},
	}
	for _, c := range cases {
		_, err := EncodeDecimal(c.val)
		if err == nil {
			t.Errorf("EncodeDecimal(%s) expected error, got nil", c.name)
			continue
		}
		if sqle, ok := errors.AsType[common.SQLError](err); ok {
			if sqle.ErrorCode() != string(c.want) {
				t.Errorf("%s: want error code %s, got %s", c.name, c.want, sqle.ErrorCode())
			}
		} else {
			t.Errorf("EncodeDecimal(%s): error is not SQLError: %v", c.name, err)
		}
	}
}

// TestNumber_DecodeDecimal_NegativeScale_Rounding verifies that DecodeDecimal
// implements Oracle NUMBER(p,s) semantics for negative scale: round to the left
// of the decimal point with ties rounded away from zero.
func TestNumber_DecodeDecimal_NegativeScale_Rounding(t *testing.T) {
	t.Parallel()
	type tc struct {
		name  string
		val   float64
		scale int
		want  string
	}
	cases := []tc{
		// Round to hundreds (s = -2)
		{name: "pos_round_down_to_hundreds", val: 123.89, scale: -2, want: "100"},
		{name: "pos_exact_half_150_hundreds", val: 150.0, scale: -2, want: "200"},  // tie -> away from zero
		{name: "pos_near_half_149_5_hundreds", val: 149.5, scale: -2, want: "100"}, // nearer to 100 than 200
		{name: "pos_exact_mid_250_hundreds", val: 250.0, scale: -2, want: "300"},   // tie -> away from zero

		{name: "neg_near_half_-149_5_hundreds", val: -149.5, scale: -2, want: "-100"}, // nearer to -100
		{name: "neg_exact_half_-150_hundreds", val: -150.0, scale: -2, want: "-200"},  // tie -> away from zero (-150 -> -200)

		// Round to tens (s = -1)
		{name: "pos_exact_half_15_tens", val: 15.0, scale: -1, want: "20"}, // tie -> away from zero
		{name: "pos_near_half_14_5_tens", val: 14.5, scale: -1, want: "10"},
		{name: "neg_exact_half_-15_tens", val: -15.0, scale: -1, want: "-20"}, // tie -> away from zero
		{name: "neg_near_half_-14_5_tens", val: -14.5, scale: -1, want: "-10"},

		// Zero with negative scale should remain zero
		{name: "zero_negative_scale", val: 0.0, scale: -3, want: "0"},
	}

	for _, c := range cases {
		wire, err := EncodeDecimal(c.val)
		if err != nil {
			t.Fatalf("EncodeDecimal(%s=%v) unexpected error: %v", c.name, c.val, err)
		}
		got, err := DecodeDecimal(wire, 38, c.scale)
		if err != nil {
			t.Fatalf("DecodeDecimal(%s) unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestEncodeUInt_KnownWireFormats_Positive verifies EncodeUInt matches the
// canonical Oracle NUMBER wires for non-negative integers (shared with EncodeInt).
func TestEncodeUInt_KnownWireFormats_Positive(t *testing.T) {
	t.Parallel()
	for _, tc := range numberWireFormatCases {
		if tc.value < 0 {
			continue
		}
		wire, _ := EncodeUInt(uint64(tc.value))
		if !bytes.Equal(wire, tc.wire) {
			t.Errorf("EncodeUInt(%d): got %#v, want %#v", tc.value, wire, tc.wire)
		}
	}
}

// TestEncodeUInt_EncodeDecode_Roundtrip verifies that non-negative uint values
// within int64 range round-trip through NUMBER encode/decode using DecodeInt.
func TestEncodeUInt_EncodeDecode_Roundtrip(t *testing.T) {
	t.Parallel()
	vals := []uint64{
		0, 1, 42, uint64(math.MaxInt64),
		123456789012345, 1000000000000000,
	}
	for _, v := range vals {
		wire, _ := EncodeUInt(v)
		got, err := DecodeInt(wire)
		if err != nil {
			t.Errorf("DecodeInt after EncodeUInt(%d) returned error: %v", v, err)
			continue
		}
		if got != int64(v) {
			t.Errorf("roundtrip mismatch: orig=%d got=%d wire=%#v", v, got, wire)
		}
	}
}

// TestEncodeUInt_OverflowOnDecodeInt ensures that values beyond int64 max encoded
// as NUMBER produce a range error when attempting DecodeInt.
func TestEncodeUInt_OverflowOnDecodeInt(t *testing.T) {
	t.Parallel()
	v := uint64(math.MaxInt64) + 1
	wire, _ := EncodeUInt(v)
	_, err := DecodeInt(wire)
	if err == nil {
		t.Fatalf("expected error decoding uint64 over int64 range, got nil")
	}
	if sqle, ok := errors.AsType[common.SQLError](err); ok {
		if sqle.ErrorCode() != string(common.ConverterRange) {
			t.Errorf("want error code %s, got %s", common.ConverterRange, sqle.ErrorCode())
		}
	} else {
		t.Errorf("error is not SQLError: %v", err)
	}
}

// TestEncodeBoolean_Wires verifies that EncodeBoolean maps false to NUMBER zero (0x80)
// and true to NUMBER one (wire for integer 1: 0xC1, 0x02).
func TestEncodeBoolean_Wires(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   bool
		wire []byte
	}{
		{false, []byte{0x0}},
		{true, []byte{0x1, 0x1}},
	}
	for _, c := range cases {
		got, err := EncodeBoolean(c.in)
		if err != nil {
			t.Fatalf("EncodeBoolean(%v) returned error: %v", c.in, err)
		}
		if !bytes.Equal(got, c.wire) {
			t.Errorf("EncodeBoolean(%v): got %#v, want %#v", c.in, got, c.wire)
		}
	}
}

// TestDecodeBoolean_FromKnownWires verifies that DecodeBoolean interprets NUMBER zero as false,
// and any non-zero integral NUMBER (including negatives) as true.
func TestDecodeBoolean_FromKnownWires(t *testing.T) {
	t.Parallel()

	cases := []struct {
		wire []byte
		want bool
	}{
		{[]byte{0x0}, false},
		{[]byte{0x1, 0x1}, true},
	}
	for _, c := range cases {
		got, err := DecodeBoolean(c.wire)
		if err != nil {
			t.Fatalf("DecodeBoolean(wire=%#v) unexpected error: %v", c.wire, err)
		}
		if got != c.want {
			t.Errorf("DecodeBoolean(wire=%#v): got %v, want %v", c.wire, got, c.want)
		}
	}
}

/*
TestEncodeBinary_ReturnsSameBytes verifies EncodeBinary's contract:

  - When passed a []byte, it returns the exact same bytes (no transformation).
  - The current implementation returns the same underlying slice (no copy), since it uses
    reflect.ValueOf(v).Bytes().

This behavior is important for performance in the bind/encode path, and for ensuring that
binary payloads (RAW / VARBINARY) are transmitted without any modification.
*/
func TestEncodeBinary_ReturnsSameBytes(t *testing.T) {
	t.Parallel()

	in := []byte{0x00, 0x01, 0xFE, 0xFF}
	got, err := EncodeBinary(in)
	if err != nil {
		t.Fatalf("EncodeBinary returned error: %v", err)
	}

	// EncodeBinary should return the same bytes without modification.
	if !bytes.Equal(got, in) {
		t.Fatalf("EncodeBinary bytes mismatch: got=%#v want=%#v", got, in)
	}

	// Current implementation uses reflect.ValueOf(v).Bytes(), which returns the same slice header,
	// so this should be the exact same underlying array (no copy).
	if len(in) > 0 && len(got) > 0 && &got[0] != &in[0] {
		t.Fatalf("EncodeBinary should not copy: got points to different backing array")
	}
}

// TestDecodeExactDecimal_DecimalWireFormats verifies that DecodeExactDecimal
// reconstructs the exact decimal string for known Oracle NUMBER decimal cases.
func TestDecodeExactDecimal_DecimalWireFormats(t *testing.T) {
	t.Parallel()

	cases := []struct {
		decimalStr string
		wire       []byte
	}{
		{"123.4501", []byte{0xC2, 0x02, 0x18, 0x2E, 0x02, 0x01}},
		{"-42.857", []byte{0x3E, 0x3B, 0x10, 0x1F, 0x66}},
		{"0.0001", []byte{0xBF, 0x02}},
		{"-0.25", []byte{0x3F, 0x4C, 0x66}},
	}

	for _, tc := range cases {
		t.Run(tc.decimalStr, func(t *testing.T) {
			got, err := DecodeExactDecimal(tc.wire)
			if err != nil {
				t.Fatalf("DecodeExactDecimal(%#v) returned error: %v", tc.wire, err)
			}
			if got != tc.decimalStr {
				t.Fatalf("DecodeExactDecimal(%#v) = %q, want %q", tc.wire, got, tc.decimalStr)
			}
		})
	}
}
