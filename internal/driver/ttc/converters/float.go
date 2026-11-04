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
	"encoding/binary"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// _floatParseBitSize is the bitSize used for ParseFloat/FormatFloat calls for float64.
	_floatParseBitSize = 64

	// Decimal formatting: use non-scientific 'f' format with -1 precision (smallest digits).
	_decimalFmt    byte = 'f'
	_decimalDigits int  = -1
	// Single-byte tokens for parsing/formatting.
	_dotByte byte = '.'
	// Sign selector for math.IsInf: 0 accepts either +Inf or -Inf; +1 for +Inf only; -1 for -Inf only.
	_signAny int = 0

	// Wire lengths of Oracle binary floating types.
	_binaryFloatLen  = 4
	_binaryDoubleLen = 8

	// Sign-bit masks used by Oracle's sortable bit transformation.
	_signMask32 uint32 = 0x80000000
	_signMask64 uint64 = 0x8000000000000000

	// Canonical zero used for comparisons that should treat +0.0 and -0.0 equally.
	_zeroFloat64 = 0.0
)

// DecodeFloat decodes an Oracle NUMBER wire representation to float64.
//
// Input:
//   - inputData: Oracle NUMBER wire-encoded bytes (as used for NUMBER/FLOAT columns)
//   - precision: declared NUMBER precision; for FLOAT(p), Oracle exposes NUMBER metadata
//     derived from its internal NUMBER representation and this is used to
//     format the decimal string prior to parsing
//   - scale: declared NUMBER scale; for FLOAT(p) this is determined by Oracle and used
//     here only to format the decimal string
//
// Output:
// - float64 parsed from the full-precision decimal string
//
// Errors:
// - Propagates DecodeDecimal/FromNumber errors, including:
//   - OGD-00021 (ConverterEmptyInput) when input is empty
//   - OGD-00023 (ConverterExpectedFormat) on invalid wire format, digit range, precision overflow, etc.
//
// Notes:
//   - Oracle FLOAT is a subtype of NUMBER. The database enforces FLOAT(p) binary precision
//     (1..126) on insert/select. The driver does not enforce or emulate p; it encodes/decodes
//     the exact numeric value and relies on the server for rounding/truncation per p.
//   - The returned float64 is subject to IEEE-754 53-bit mantissa precision limits.
func DecodeFloat(inputData []byte, precision, scale int) (float64, error) {
	// Decode NUMBER wire directly to mantissa/sign/exponent to avoid any
	// presentation rounding based on (p,s). FLOAT should not be rounded per scale.
	mantissa, negative, exponent, _, err := FromNumber(inputData)
	if err != nil {
		return 0, err
	}
	// Oracle zero wire => 0.0
	if len(inputData) == 1 && inputData[0] == _numberZeroWireByte {
		return 0, nil
	}
	// decoding has to go through string, to not lose precision.
	mantissaStr := strconv.FormatUint(mantissa, 10)
	if exponent > 0 {
		mantissaStr += strings.Repeat("0", exponent)
	} else if exponent < 0 {
		pos := len(mantissaStr) + exponent // exponent is negative
		if pos < 0 {
			pos = -pos
			mantissaStr = strings.Repeat("0", pos) + mantissaStr
			pos = 0
		}
		mantissaStr = mantissaStr[:pos] + "." + mantissaStr[pos:]
		mantissaStr = strings.TrimRight(mantissaStr, "0")
	}

	if mantissaStr[0] == _dotByte {
		mantissaStr = "0" + mantissaStr
	}
	if negative {
		mantissaStr = "-" + mantissaStr
	}

	return strconv.ParseFloat(mantissaStr, _floatParseBitSize)
}

// EncodeFloat encodes a Go float64 into Oracle NUMBER wire format.
//
// Input:
// - num: finite float64 (NaN and ±Inf are not representable as NUMBER)
//
// Output:
// - []byte NUMBER wire encoding compatible with NUMBER/FLOAT columns
//
// Errors:
// - OGD-00023 (ConverterExpectedFormat) if num is NaN or ±Inf
//
// Notes:
//   - Zero encodes to single byte 0x80; +0.0 and -0.0 are treated equally.
//   - Oracle FLOAT is stored as NUMBER; the server enforces FLOAT(p) binary precision.
//     The driver does not emulate p; any rounding/truncation occurs in the database.
//   - Encoding uses ToNumber to apply Oracle's packed BCD rules and normalization.
func EncodeFloat(v driver.Value) (driverCommon.B1Array, error) {
	num := reflect.ValueOf(v).Float()

	// Reject non-finite values: Oracle NUMBER cannot represent NaN or ±Inf
	if math.IsNaN(num) || math.IsInf(num, _signAny) {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "FLOAT", "Encode", driverCommon.ReasonInvalidValue, "finite number")
	}
	// Oracle encodes zero as a single byte 0x80; handles +0.0 and -0.0
	if num == _zeroFloat64 {
		return []byte{_numberZeroWireByte}, nil
	}

	negative := num < 0
	if negative {
		num = -num
	}

	// Format the absolute value as a non-scientific decimal string
	dec := strconv.FormatFloat(num, _decimalFmt, _decimalDigits, _floatParseBitSize)

	// Split into integer and fractional parts
	intPart := dec
	fracPart := ""
	if idx := strings.IndexByte(dec, _dotByte); idx != -1 {
		intPart = dec[:idx]
		fracPart = dec[idx+1:]
	}

	// Compute integer digits count (without leading zeros in int part)
	intTrim := strings.TrimLeft(intPart, string(_zeroChar))
	intDigitsCount := len(intTrim)
	if intPart == "" {
		intDigitsCount = 0
	}

	// Count leading zeros in fractional part when number < 1
	leadingZerosInFrac := 0
	if intDigitsCount == 0 {
		for leadingZerosInFrac < len(fracPart) && fracPart[leadingZerosInFrac] == _zeroChar {
			leadingZerosInFrac++
		}
	}

	// Build mantissa as significant digits only (no decimal point)
	var mantissaStr string
	if intDigitsCount > 0 {
		mantissaStr = intTrim + fracPart
	} else {
		// number < 1: skip fractional leading zeros
		if leadingZerosInFrac < len(fracPart) {
			mantissaStr = fracPart[leadingZerosInFrac:]
		} else {
			mantissaStr = ""
		}
	}

	// Trim any remaining leading zeros (safety)
	mantissaStr = strings.TrimLeft(mantissaStr, string(_zeroChar))
	if mantissaStr == "" {
		// Still zero after trimming
		return []byte{_numberZeroWireByte}, nil
	}

	// Base-10 exponent relative to first significant digit
	// = digits before decimal (after trimming) - 1 - leading zeros in fractional (if any)
	exponent := intDigitsCount - 1 - leadingZerosInFrac

	// Delegate final wire packing to ToNumber to match Oracle protocol
	wire := ToNumber([]byte(mantissaStr), negative, exponent)
	return wire, nil
}

// DecodeBinaryFloat decodes Oracle BINARY_FLOAT wire representation (Oracle's sortable encoding) to float32.
//
// Input:
// - inputData: exactly 4 bytes of big-endian Oracle BINARY_FLOAT wire data (sortable IEEE-754 transform)
//
// Output:
// - float32 value decoded from inputData
//
// Errors:
//   - OGD-00023 (ConverterExpectedFormat) when len(inputData) != 4
//     Reason: InvalidLength; Expected: 4
//
// Notes:
// - Oracle uses a sortable transformation of IEEE-754:
//   - if original value >= 0: store bits ^ 0x80000000
//   - if original value < 0:  store ^bits (bitwise NOT)
//
// - Decoding reverses the transform. Special values (NaN, ±Inf, signed zeros) and subnormals round-trip.
func DecodeBinaryFloat(inputData []byte) (float32, error) {
	if len(inputData) != _binaryFloatLen {
		return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "BINARY_FLOAT", "Decode", driverCommon.ReasonInvalidLength, _binaryFloatLen)
	}
	stored := binary.BigEndian.Uint32(inputData)
	var bits uint32
	if (stored & _signMask32) != 0 {
		// original was non-negative
		bits = stored ^ _signMask32
	} else {
		// original was negative
		bits = ^stored
	}
	return math.Float32frombits(bits), nil
}

// EncodeBinaryFloat encodes a Go float32 to Oracle BINARY_FLOAT (Oracle's sortable encoding).
//
// Input:
// - f: any IEEE-754 float32, including NaN, ±Inf, ±0.0, and subnormals
//
// Output:
// - []byte of length 4: big-endian Oracle BINARY_FLOAT wire representation (sortable transform)
//
// Errors:
// - None (function does not return error)
//
// Notes:
// - Oracle's sortable transform preserves total ordering and special values via reversible bit operations.
func EncodeBinaryFloat(v driver.Value) ([]byte, error) {
	f := v.(float32)
	bits := math.Float32bits(f)
	var stored uint32
	// Apply Oracle's sortable transform: negate bit ordering for negatives,
	// flip sign bit for non-negatives. This preserves total ordering by bytes.
	if math.Signbit(float64(f)) {
		// original was negative
		stored = ^bits
	} else {
		// original was non-negative
		stored = bits ^ _signMask32
	}
	// Allocate output buffer (big-endian) for wire representation
	bytes := make([]byte, _binaryFloatLen)
	binary.BigEndian.PutUint32(bytes, stored)
	return bytes, nil
}

// DecodeBinaryDouble decodes Oracle BINARY_DOUBLE wire representation (Oracle's sortable encoding) to float64.
//
// Input:
// - inputData: exactly 8 bytes of big-endian Oracle BINARY_DOUBLE wire data (sortable IEEE-754 transform)
//
// Output:
// - float64 value decoded from inputData
//
// Errors:
//   - OGD-00023 (ConverterExpectedFormat) when len(inputData) != 8
//     Reason: InvalidLength; Expected: 8
//
// Notes:
// - Oracle uses a sortable transformation of IEEE-754 for BINARY_DOUBLE:
//   - if original value >= 0: store bits ^ 0x8000000000000000
//   - if original value < 0:  store ^bits (bitwise NOT)
//
// - Decoding reverses the transform. Special values (NaN, ±Inf, signed zeros) and subnormals round-trip.
func DecodeBinaryDouble(inputData []byte) (float64, error) {
	if len(inputData) != _binaryDoubleLen {
		return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "BINARY_DOUBLE", "Decode", driverCommon.ReasonInvalidLength, _binaryDoubleLen)
	}
	stored := binary.BigEndian.Uint64(inputData)
	var bits uint64
	if (stored & _signMask64) != 0 {
		// original was non-negative
		bits = stored ^ _signMask64
	} else {
		// original was negative
		bits = ^stored
	}
	return math.Float64frombits(bits), nil
}

// EncodeBinaryDouble encodes a Go float64 to Oracle BINARY_DOUBLE (Oracle's sortable encoding).
//
// Input:
// - f: any IEEE-754 float64, including NaN, ±Inf, ±0.0, and subnormals
//
// Output:
// - []byte of length 8: big-endian Oracle BINARY_DOUBLE wire representation (sortable transform)
//
// Errors:
// - None (function does not return error)
//
// Notes:
// - Oracle's sortable transform preserves total ordering and special values via reversible bit operations.
func EncodeBinaryDouble(v driver.Value) ([]byte, error) {
	f := v.(float64)
	bits := math.Float64bits(f)
	var stored uint64
	if math.Signbit(f) {
		// original was negative
		stored = ^bits
	} else {
		// original was non-negative
		stored = bits ^ _signMask64
	}
	// Allocate output buffer (big-endian) for wire representation
	bytes := make([]byte, _binaryDoubleLen)
	binary.BigEndian.PutUint64(bytes, stored)
	return bytes, nil
}
