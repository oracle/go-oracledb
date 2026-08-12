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
	"database/sql/driver"
	"math"
	"math/big"
	"math/bits"
	"reflect"
	"strconv"
	"strings"

	"github.com/oracle/go-driver/driver/common"
)

const (
	// Oracle NUMBER: special terminator for small negative numbers.
	_negTerminatorByte byte = 0x66
	// Oracle NUMBER: leading-byte sign bit mask and exponent masks.
	_leadSignBitMask byte = 0x80
	_exponentMask    byte = 0x7f

	// Decimal and packed pair bases.
	_decimalBase         = 10
	_packedPairBase      = 100
	_packByteBias   byte = 1

	// Exponent bias used in Oracle NUMBER leading byte.
	_exponentBias = 64

	// Maximum single decimal digit value.
	_maxSingleDigit = 9

	// Oracle NUMBER encodes zero as a single byte 0x80 on the wire.
	_numberZeroWireByte byte = 0x80

	// ASCII '0' used across encoders/decoders
	_zeroChar byte = '0'

	// _numberWireMaxLen is the maximum Oracle NUMBER wire length: one lead byte
	// followed by up to 20 payload bytes.
	_numberWireMaxLen = 21

	// _numberMinPrecision is the minimum explicit Oracle NUMBER precision.
	// Precision zero is reserved for an unconstrained or unspecified NUMBER.
	_numberMinPrecision = 1
	// _numberMaxPrecision is the maximum explicit Oracle NUMBER precision.
	_numberMaxPrecision = 38
	// _numberMinScale is the minimum supported Oracle NUMBER scale.
	_numberMinScale = -84
	// _numberMaxScale is the maximum supported Oracle NUMBER scale.
	_numberMaxScale = 127
)

// Precomputed powers of 10 for fast scaling during integer decode (10^0..10^19).
var _pow10 = [...]uint64{
	1,
	10,
	100,
	1000,
	10000,
	100000,
	1000000,
	10000000,
	100000000,
	1000000000,
	10000000000,
	100000000000,
	1000000000000,
	10000000000000,
	100000000000000,
	1000000000000000,
	10000000000000000,
	100000000000000000,
	1000000000000000000,
	10000000000000000000,
}

// validateNumberWireLength rejects NUMBER encodings larger than Oracle's
// supported wire payload before any mantissa conversion is attempted.
func validateNumberWireLength(inputData []byte) error {
	if len(inputData) > _numberWireMaxLen {
		return common.NewOracleError(
			common.ConverterExpectedFormat,
			nil,
			"NUMBER",
			"Decode",
			common.ReasonInvalidLength,
			strconv.Itoa(_numberWireMaxLen),
		)
	}
	return nil
}

// validateNumberMetadata validates Oracle NUMBER precision and scale metadata.
// Precision zero is accepted as unspecified, and scale may exceed precision.
func validateNumberMetadata(precision, scale int) error {
	if precision != 0 && precision < _numberMinPrecision {
		return common.NewOracleError(
			common.ConverterExpectedFormat,
			nil,
			"NUMBER",
			"Decode",
			common.ReasonOutOfRange,
			"0.."+strconv.Itoa(_numberMaxPrecision),
		)
	}
	if precision > _numberMaxPrecision {
		return common.NewOracleError(
			common.ConverterExpectedFormat,
			nil,
			"NUMBER",
			"Decode",
			common.ReasonPrecisionExceeded,
			"<="+strconv.Itoa(_numberMaxPrecision),
		)
	}
	if scale < _numberMinScale || scale > _numberMaxScale {
		return common.NewOracleError(
			common.ConverterExpectedFormat,
			nil,
			"NUMBER",
			"Decode",
			common.ReasonOutOfRange,
			strconv.Itoa(_numberMinScale)+".."+strconv.Itoa(_numberMaxScale),
		)
	}
	return nil
}

/*
DecodeInt converts Oracle NUMBER wire format to int64.

Input:
- inputData: Oracle NUMBER wire-encoded bytes

Output:
- int64 value when the NUMBER has no fractional part and is within int64 range

Errors:
- OGD-00021 (ConverterEmptyInput) when inputData is empty
- OGD-00023 (ConverterExpectedFormat) when wire format is invalid or fractional part is present
- OGD-00024 (ConverterRange) when the value overflows int64
*/
func DecodeInt(inputData []byte) (int64, error) {
	// Parse Oracle NUMBER wire encoding into mantissa, sign, and exponent
	mantissa, negative, exponent, _, err := FromNumber(inputData)
	if err != nil {
		return 0, err
	}
	// Reject fractional values for int64 decode
	if exponent < 0 {
		return 0, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonInvalidFormat, "integer")
	}

	// Scale mantissa by 10^exponent using a single multiply with overflow check
	const maxInt64 = uint64(1<<63 - 1)
	if exponent > 0 {
		if exponent >= len(_pow10) {
			return 0, common.NewOracleError(common.ConverterRange, nil, "NUMBER", "Decode", common.ReasonOutOfRange, math.MinInt64, int64(maxInt64))
		}
		hi, lo := bits.Mul64(mantissa, _pow10[exponent])
		if hi != 0 {
			return 0, common.NewOracleError(common.ConverterRange, nil, "NUMBER", "Decode", common.ReasonOutOfRange, math.MinInt64, int64(maxInt64))
		}
		mantissa = lo
	}

	if negative {
		// permit -2^63 explicitly
		if mantissa > 1<<63 {
			return 0, common.NewOracleError(common.ConverterRange, nil, "NUMBER", "Decode", common.ReasonOutOfRange, math.MinInt64, int64(maxInt64))
		}
		if mantissa == 1<<63 {
			return math.MinInt64, nil
		}
		return -int64(mantissa), nil
	}

	if mantissa > maxInt64 {
		return 0, common.NewOracleError(common.ConverterRange, nil, "NUMBER", "Decode", common.ReasonOutOfRange, math.MinInt64, int64(maxInt64))
	}
	return int64(mantissa), nil
}

/*
EncodeInt converts an int64 to Oracle NUMBER wire encoding.

Input:
- val: any int64 (including MinInt64 and 0)

Output:
- []byte NUMBER wire encoding; zero encodes as a single 0x80 byte

Errors:
- None
*/
func EncodeInt(v driver.Value) (common.B1Array, error) {
	val := reflect.ValueOf(v).Int()
	// Oracle encodes zero as a single byte 0x80
	if val == 0 {
		return []byte{_numberZeroWireByte}, nil
	}
	negative := val < 0
	var digits []byte
	if negative {
		// strconv.FormatInt handles MinInt64; drop leading '-' for mantissa
		digits = []byte(strconv.FormatInt(val, 10))[1:]
	} else {
		digits = []byte(strconv.FormatInt(val, 10))
	}
	return encodeIntegerDigitsStrict(digits, negative), nil
}

// encodeIntegerDigitsStrict centralizes the common integer-to-NUMBER encoding path
// for both signed and unsigned integer encoders. It expects ASCII digits without sign.
func encodeIntegerDigitsStrict(mantissa []byte, negative bool) common.B1Array {
	// Strip trailing zeros to keep canonical compact wire; adjust exponent accordingly
	trim := 0
	for i := len(mantissa) - 1; i >= 0 && mantissa[i] == _zeroChar; i-- {
		trim++
	}
	if trim > 0 {
		mantissa = mantissa[:len(mantissa)-trim]
	}
	if len(mantissa) == 0 {
		return []byte{_numberZeroWireByte}
	}
	// Exponent is msd index; add back the number of trimmed zeros
	exponent := (len(mantissa) - 1) + trim
	return _toNumberStrict(mantissa, negative, exponent)
}

/*
EncodeUInt converts an unsigned integer (uint, uint8/16/32/64) to Oracle NUMBER wire encoding.

Input:
- val: any unsigned integer (including 0)

Output:
- []byte NUMBER wire encoding; zero encodes as a single 0x80 byte

Errors:
- None
*/
func EncodeUInt(v driver.Value) (common.B1Array, error) {
	uv := reflect.ValueOf(v).Uint()
	if uv == 0 {
		return []byte{_numberZeroWireByte}, nil
	}
	digits := []byte(strconv.FormatUint(uv, 10))
	return encodeIntegerDigitsStrict(digits, false), nil
}

/*
EncodeBinary returns the underlying byte slice for a binary value.

This encoder is used for TTC types that are already represented as raw bytes
(e.g., BINARY, RAW, and similar payloads). It performs no transformation and
returns the bytes as-is.

Input:
- v: driver.Value whose dynamic type must be []byte

Output:
- common.B1Array containing the same bytes (no copy)

Errors / Panics:
- Panics if v is not a []byte (reflect.Value.Bytes requires a slice/array of bytes)
*/
func EncodeBinary(v driver.Value) (common.B1Array, error) {
	return reflect.ValueOf(v).Bytes(), nil
}

/*
EncodeNull encodes a SQL NULL bind value.

This is used when the bind value is untyped nil (i.e., driver.Value == nil) and
the caller has no concrete Go type information to select a more specific codec.

Output:
- nil

Errors:
- None
*/
func EncodeNull(_ driver.Value) (common.B1Array, error) {
	return nil, nil
}

/*
EncodeBoolean converts a Go bool into Oracle Boolean wire format.

Encoding:
- false -> single byte 0x0
- true  -> []byte{0x1, 0x1}

Errors:
- nil
*/
func EncodeBoolean(v driver.Value) (common.B1Array, error) {
	b := v.(bool)
	if b {
		return []byte{0x1, 0x1}, nil
	}
	return []byte{0x0}, nil
}

/*
EncodeBooleanAsNumber converts a Go bool into Oracle Number wire format.

Encoding:
- false -> EncodeInt(0)
- true  -> EncodeInt(1)

Errors:
- error returned by EncodeInt
*/
func EncodeBooleanAsNumber(v driver.Value) (common.B1Array, error) {
	b := v.(bool)
	if b {
		return EncodeInt(1)
	}
	return EncodeInt(0)
}

/*
DecodeBoolean interprets an Oracle Boolean wire encoding as a boolean.

Decoding:
- NUMBER zero -> false
- Any non-zero -> true
*/
func DecodeBoolean(inputData []byte) (bool, error) {
	return inputData[0] != 0, nil
}

/*
DecodeDecimal decodes a NUMBER (including fractional values) to a full-precision decimal string.

Input:
  - inputData: Oracle NUMBER wire-encoded bytes
  - precision: declared NUMBER precision; 0 means unspecified, otherwise it must be 1 through 38
  - scale: declared NUMBER scale; if >= 0, the result is formatted with exactly scale digits after the decimal point;
    if < 0, rounds to the left of the decimal point per Oracle NUMBER(p,s) semantics; valid scales are -84 through 127

Output:
- string representation of the exact value, rounded per negative scale rules when scale < 0

Errors:
- OGD-00021 (ConverterEmptyInput) when inputData is empty
- OGD-00023 (ConverterExpectedFormat) for invalid wire length, metadata, wire format, or digit ranges
*/
func DecodeDecimal(inputData []byte, precision, scale int) (string, error) {
	if err := validateNumberMetadata(precision, scale); err != nil {
		return "", err
	}

	// Oracle zero is a complete one-byte wire value with no mantissa to parse.
	if len(inputData) == 1 && inputData[0] == _numberZeroWireByte {
		if scale >= 0 {
			return new(big.Rat).SetInt64(0).FloatString(scale), nil
		}
		return "0", nil
	}

	bigMantissa, negative, exponent, _, err := _fromNumberBig(inputData)
	if err != nil {
		return "", err
	}

	// Assemble integer/fractional as string using big.Int/big.Rat to preserve precision.
	big10 := big.NewInt(int64(_decimalBase))
	bigExp := exponent
	bigRat := new(big.Rat).SetInt(bigMantissa)

	// Adjust for sign
	if negative {
		bigRat.Neg(bigRat)
	}

	// Exponent is base 10 and can be negative for decimals
	if bigExp > 0 {
		expInt := big.NewInt(1)
		expInt.Exp(big10, big.NewInt(int64(bigExp)), nil)
		bigRat.Mul(bigRat, new(big.Rat).SetInt(expInt))
	} else if bigExp < 0 {
		expInt := big.NewInt(1)
		expInt.Exp(big10, big.NewInt(int64(-bigExp)), nil)
		bigRat.Quo(bigRat, new(big.Rat).SetInt(expInt))
	}

	// If scale >= 0, format with that many digits after the decimal point.
	if scale >= 0 {
		return bigRat.FloatString(scale), nil
	}

	// Negative scale: round to the left of the decimal point per Oracle NUMBER(p,s) semantics.
	// Example: s = -2 => round to hundreds; 123.89 -> 100; -149.5 -> -200 (half away from zero).
	k := -scale
	factor := new(big.Int).Exp(big10, big.NewInt(int64(k)), nil)

	// Divide by factor and round half away from zero to the nearest integer.
	divRat := new(big.Rat).Quo(bigRat, new(big.Rat).SetInt(factor))
	nNum := new(big.Int).Set(divRat.Num())
	nDen := new(big.Int).Set(divRat.Denom())

	// Work with absolute numerator for rounding; track sign.
	sign := nNum.Sign()
	nAbs := new(big.Int).Abs(nNum)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(nAbs, nDen, r) // q = floor(nAbs/nDen), r = nAbs % nDen

	// If 2*r >= d, increment q (half-away-from-zero rounding).
	twiceR := new(big.Int).Lsh(r, 1)
	if twiceR.Cmp(nDen) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if sign < 0 {
		q.Neg(q)
	}

	// Multiply back by factor; result is an integer with no fractional digits.
	rounded := new(big.Int).Mul(q, factor)
	return rounded.String(), nil
}

/*
DecodeExactDecimal decodes an Oracle NUMBER wire payload to its exact
decimal string without applying declared precision/scale formatting.

Input:
- inputData: Oracle NUMBER wire-encoded bytes

Output:
- string representation of the exact value

Errors:
- OGD-00021 (ConverterEmptyInput) when inputData is empty
- OGD-00023 (ConverterExpectedFormat) for invalid wire format or digit ranges
*/
func DecodeExactDecimal(inputData []byte) (string, error) {
	// Oracle zero is encoded as a single byte.
	if len(inputData) == 1 && inputData[0] == _numberZeroWireByte {
		return "0", nil
	}

	mantissa, negative, exponent, _, err := _fromNumberBig(inputData)
	if err != nil {
		return "", err
	}

	mantissaString := mantissa.String()

	// normalizeDecimalString removes trailing fractional zeros and a trailing dot.
	normalizeDecimalString := func(decimalString string) string {
		integerPart, fractionPart, hasFraction := strings.Cut(decimalString, ".")
		if hasFraction {
			fractionPart = strings.TrimRight(fractionPart, "0")
			if fractionPart == "" {
				decimalString = integerPart
			} else {
				decimalString = integerPart + "." + fractionPart
			}
		}

		// Treat empty and "-0" as plain zero.
		if decimalString == "" || decimalString == "-0" {
			return "0"
		}
		return decimalString
	}

	// decimal point position relative to the mantissa string
	decimalPoint := exponent + len(mantissaString)

	var decimalString string
	switch {
	case decimalPoint >= len(mantissaString):
		// The number is an integer. Append the required trailing zeros.
		decimalString = mantissaString + strings.Repeat("0", decimalPoint-len(mantissaString))

	case decimalPoint > 0:
		// Insert the decimal point inside the mantissa digits.
		decimalString = normalizeDecimalString(
			mantissaString[:decimalPoint] + "." + mantissaString[decimalPoint:],
		)

	default:
		// The decimal point is left of the first digit.
		decimalString = normalizeDecimalString(
			"0." + strings.Repeat("0", -decimalPoint) + mantissaString,
		)
	}

	if negative {
		return "-" + decimalString, nil
	}
	return decimalString, nil
}

/*
EncodeDecimal encodes a Go float64 into Oracle NUMBER wire format.

Input:
- num: finite float64

Output:
- []byte NUMBER wire encoding

Errors:
- OGD-00023 (ConverterExpectedFormat) if num is NaN or ±Inf (propagated from EncodeFloat)
*/
func EncodeDecimal(v driver.Value) ([]byte, error) {
	num := v.(float64)
	if math.IsNaN(num) || math.IsInf(num, _signAny) {
		return nil, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Encode", common.ReasonInvalidValue, "finite number")
	}
	abs := math.Abs(num)
	if abs >= 1e126 || (abs > 0 && abs < 1e-130) {
		return nil, common.NewOracleError(common.ConverterRange, nil, "NUMBER", "Encode", common.ReasonOutOfRange, "[1e-130, 1e126)")
	}
	return EncodeFloat(num)
}

/*
ToNumber encodes (mantissa, sign, exponent) as Oracle NUMBER wire format []byte.

Input:
- mantissa: ASCII decimal digits without sign (e.g., []byte("12345"))
- negative: sign flag; true for negative
- exponent: base-10 exponent relative to the first mantissa digit

Output:
- []byte Oracle NUMBER wire encoding following packed BCD rules and Oracle normalization

Errors:
- None
*/
func ToNumber(mantissa []byte, negative bool, exponent int) []byte {
	// remove the zeroes at the end
	endZeroCount := 0
	for i := len(mantissa) - 1; i >= 0 && mantissa[i] == _zeroChar; i-- {
		endZeroCount++
	}
	mantissa = mantissa[:len(mantissa)-endZeroCount]

	// Oracle encodes zero as single byte 0x80
	if len(mantissa) == 0 {
		return []byte{_numberZeroWireByte}
	}

	// Oracle normalization: prepend a leading zero digit when the base-10 exponent is even
	if exponent%2 == 0 {
		mantissa = append([]byte{_zeroChar}, mantissa...)
	}

	mantissaLen := len(mantissa)
	packedLen := (mantissaLen + 1) / 2
	baseLen := 1 + packedLen
	appendNegTerm := negative && (baseLen+1 <= _numberWireMaxLen)
	size := baseLen
	if appendNegTerm {
		size++
	}
	buf := make([]byte, size, size)

	// Pack digits into wire encoding, 2 digits per byte, with sign adjustment
	for i := 0; i < mantissaLen; i += 2 {
		b := _decimalBase * (mantissa[i] - '0')
		if i < mantissaLen-1 {
			b += mantissa[i+1] - '0'
		}
		if negative {
			b = _packedPairBase - b
		}
		buf[1+i/2] = b + _packByteBias
	}

	// Exponent normalization per Oracle
	if exponent < 0 {
		exponent--
	}
	exponent = (exponent / 2) + 1
	// Wire-encode exponent and sign into leading byte
	if negative {
		buf[0] = byte(exponent+_exponentBias) ^ _exponentMask
	} else {
		buf[0] = byte(exponent+_exponentBias) | _leadSignBitMask
	}
	// Add trailing byte for small negative numbers if wire fits into 21 bytes
	if appendNegTerm {
		buf[len(buf)-1] = _negTerminatorByte
	}
	return buf
}

/*
_toNumberStrict encodes using the same rules as ToNumber but enforces the 21-byte
wire length cap (1 lead + up to 20 payload bytes) when deciding whether to append
the negative terminator (0x66). This avoids altering ToNumber used by FLOAT paths.

Input:
- mantissa: ASCII decimal digits without sign
- negative: sign flag
- exponent: base-10 exponent relative to the first mantissa digit

Output:
- []byte Oracle NUMBER wire encoding, with 0x66 terminator only when total length <= 21 bytes

Errors:
- None
*/
func _toNumberStrict(mantissa []byte, negative bool, exponent int) []byte {
	if len(mantissa) == 0 {
		return []byte{_numberZeroWireByte}
	}
	// Oracle normalization: prepend a leading zero digit when the base-10 exponent is even
	if exponent%2 == 0 {
		mantissa = append([]byte{_zeroChar}, mantissa...)
	}
	mantissaLen := len(mantissa)
	packedLen := (mantissaLen + 1) / 2
	baseLen := 1 + packedLen
	appendNegTerm := negative && (baseLen+1 <= _numberWireMaxLen)
	size := baseLen
	if appendNegTerm {
		size++
	}
	buf := make([]byte, size, size)
	for i := 0; i < mantissaLen; i += 2 {
		b := _decimalBase * (mantissa[i] - '0')
		if i < mantissaLen-1 {
			b += mantissa[i+1] - '0'
		}
		if negative {
			b = _packedPairBase - b
		}
		buf[1+i/2] = b + _packByteBias
	}
	if exponent < 0 {
		exponent--
	}
	exponent = (exponent / 2) + 1
	if negative {
		buf[0] = byte(exponent+_exponentBias) ^ _exponentMask
	} else {
		buf[0] = byte(exponent+_exponentBias) | _leadSignBitMask
	}
	if appendNegTerm {
		buf[len(buf)-1] = _negTerminatorByte
	}
	return buf
}

/*
FromNumber parses Oracle NUMBER wire-encoded bytes to mantissa/sign/exponent.

Input:
- inputData: Oracle NUMBER wire-encoded bytes

Output:
- mantissa: uint64 of packed digits (base-10)
- negative: sign flag
- exponent: base-10 exponent relative to mantissa's first digit
- mantissaDigits: total count of base-10 digits in mantissa (adjusted for leading zero when present)

Errors:
- OGD-00021 (ConverterEmptyInput) when inputData is empty
- OGD-00023 (ConverterExpectedFormat) for invalid wire format, digit ranges, or precision overflow

Notes:
- Small negative numbers may include a trailing 0x66 terminator byte.
*/
func FromNumber(inputData []byte) (mantissa uint64, negative bool, exponent int, mantissaDigits int, err error) {
	if len(inputData) == 0 {
		return 0, false, 0, 0, common.NewOracleError(common.ConverterEmptyInput, nil, "NUMBER", "Decode")
	}
	if err = validateNumberWireLength(inputData); err != nil {
		return 0, false, 0, 0, err
	}
	// Special case: Oracle NUMBER zero is 0x80
	if inputData[0] == _numberZeroWireByte {
		return 0, false, 0, 0, nil
	}

	// First byte encodes sign and exponent
	negative = inputData[0]&_leadSignBitMask == 0
	if negative {
		exponent = int(inputData[0]^_exponentMask) - _exponentBias
	} else {
		exponent = int(inputData[0]&_exponentMask) - _exponentBias
	}

	buf := inputData[1:]
	// Small negative numbers get a trailing 0x66 byte per Oracle convention
	if negative && inputData[len(inputData)-1] == _negTerminatorByte {
		if len(inputData) == 1 {
			// Protection against panic because of  a malicious or compromised Oracle TTC peer
			return 0, negative, exponent, mantissaDigits,
				common.NewOracleError(common.ConverterExpectedFormat, nil,
					"NUMBER", "Decode", common.ReasonInvalidFormat, "_negTerminatorByte")
		}
		buf = inputData[1 : len(inputData)-1]
	}

	firstDigitWasZero := 0

	// Unpack each pair of digits from the Oracle wire format, accumulating mantissa
	mantissaDigits = 0
	for p, digit100 := range buf {
		if p == 0 {
			firstDigitWasZero = -1 // Adjusts digits count for leading zero handling
		}

		// Normalize packed pair to 0..99 after decrement and sign handling
		d := uint16(digit100) - 1
		if negative {
			d = uint16(_packedPairBase) - d
		}
		if d > _packedPairBase-1 {
			return 0, negative, exponent, mantissaDigits, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonInvalidFormat, "packed-digit[0..99]")
		}
		highDigit := int(d / _decimalBase)
		lowDigit := int(d % _decimalBase)
		if highDigit < 0 || highDigit > _maxSingleDigit || lowDigit < 0 || lowDigit > _maxSingleDigit {
			return 0, negative, exponent, mantissaDigits, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonInvalidFormat, "single-digit[0..9]")
		}
		pair := int(d)
		// Overflow check for mantissa = mantissa*100 + pair
		if mantissa > (math.MaxUint64-uint64(pair))/uint64(_packedPairBase) {
			return 0, negative, exponent, mantissaDigits, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonPrecisionExceeded, "uint64 mantissa")
		}
		mantissa = mantissa*uint64(_packedPairBase) + uint64(pair)
		mantissaDigits += 2
	}

	// Final exponent adjustment: Oracle wire is base-100, Go uses base-10 digits
	exponent = exponent*2 - mantissaDigits // Adjust exponent to base-10
	return mantissa, negative, exponent, mantissaDigits + firstDigitWasZero, nil
}

/*
_fromNumberBig parses a nonzero Oracle NUMBER wire value to a big.Int mantissa
to preserve full precision for values beyond 20 decimal digits. Exponent and
mantissaDigits semantics match FromNumber.

Input:
- inputData: Oracle NUMBER wire-encoded bytes

Output:
- mantissa: *big.Int for full-precision mantissa
- negative: sign flag
- exponent: base-10 exponent relative to the first mantissa digit
- mantissaDigits: total count of base-10 digits in mantissa (adjusted for leading zero when present)

Errors:
- OGD-00021 (ConverterEmptyInput) when inputData is empty
- OGD-00023 (ConverterExpectedFormat) for invalid wire format or digit ranges
*/
func _fromNumberBig(inputData []byte) (mantissa *big.Int, negative bool, exponent int, mantissaDigits int, err error) {
	if len(inputData) == 0 {
		return nil, false, 0, 0, common.NewOracleError(common.ConverterEmptyInput, nil, "NUMBER", "Decode")
	}
	if err = validateNumberWireLength(inputData); err != nil {
		return nil, false, 0, 0, err
	}
	// First byte encodes sign and exponent
	negative = inputData[0]&_leadSignBitMask == 0
	if negative {
		exponent = int(inputData[0]^_exponentMask) - _exponentBias
	} else {
		exponent = int(inputData[0]&_exponentMask) - _exponentBias
	}

	buf := inputData[1:]
	// Small negative numbers may have trailing 0x66 terminator
	if negative && inputData[len(inputData)-1] == _negTerminatorByte {
		if len(inputData) == 1 {
			// Protection against panic because of  a malicious or compromised Oracle TTC peer
			return nil, negative, exponent, mantissaDigits,
				common.NewOracleError(common.ConverterExpectedFormat, nil,
					"NUMBER", "Decode", common.ReasonInvalidFormat, "_negTerminatorByte")
		}
		buf = inputData[1 : len(inputData)-1]
	}

	firstDigitWasZero := 0
	mant := new(big.Int)
	mant.SetInt64(0)

	mantissaDigits = 0
	b100 := big.NewInt(int64(_packedPairBase))
	pairInt := new(big.Int)
	for p, digit100 := range buf {
		if p == 0 {
			firstDigitWasZero = -1
		}
		d := int(digit100) - 1
		if negative {
			d = _packedPairBase - d
		}
		if d < 0 || d > _packedPairBase-1 {
			return nil, negative, exponent, mantissaDigits, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonInvalidFormat, "packed-digit[0..99]")
		}
		highDigit := int(d / _decimalBase)
		lowDigit := int(d % _decimalBase)
		if highDigit < 0 || highDigit > _maxSingleDigit || lowDigit < 0 || lowDigit > _maxSingleDigit {
			return nil, negative, exponent, mantissaDigits, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonInvalidFormat, "single-digit[0..9]")
		}
		pair := highDigit*_decimalBase + lowDigit
		mant.Mul(mant, b100)
		pairInt.SetInt64(int64(pair))
		mant.Add(mant, pairInt)
		mantissaDigits += 2
	}

	exponent = exponent*2 - mantissaDigits
	return mant, negative, exponent, mantissaDigits + firstDigitWasZero, nil
}

/*
addDigitToMantissa appends a decimal digit to the uint64 mantissa accumulator.

Input:
- current: current uint64 mantissa
- digit: next base-10 digit to append [0..9]

Output:
- next mantissa value if no overflow occurs

Errors:
- OGD-00023 (ConverterExpectedFormat) when digit is outside [0..9] or if precision overflows uint64
*/
func addDigitToMantissa(current uint64, digit int) (uint64, error) {
	if digit < 0 || digit > _maxSingleDigit {
		return current, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonInvalidFormat, "single-digit[0..9]")
	}
	const maxMantissa = (1<<64 - 1) / _decimalBase // Prevent multiplication overflow
	if current > maxMantissa {
		return current, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonPrecisionExceeded, "uint64 mantissa")
	}
	next := current*_decimalBase + uint64(digit)
	if next < current { // detect uint64 wraparound overflow
		return current, common.NewOracleError(common.ConverterExpectedFormat, nil, "NUMBER", "Decode", common.ReasonPrecisionExceeded, "uint64 mantissa")
	}
	return next, nil
}
