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
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"unicode/utf16"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// EncodeVarchar encodes a Go string into an Oracle VARCHAR2 payload.
// It encodes the string into a B1Array (byte slice), suitable for use with Oracle VARCHAR2 columns.
//
// Input:
//   - v (string): The Go string to be encoded into Oracle VARCHAR2.
//
// Output:
//   - common.B1Array: A byte slice representing the Oracle VARCHAR2 payload.
//
// Errors:
//   - None.
//
// Example:
//
//	v := "Hello World"
//	encoded := EncodeVarchar(v)
func EncodeVarchar(v driver.Value) (driverCommon.B1Array, error) {
	return driverCommon.B1Array([]byte(v.(string))), nil
}

// DecodeVarchar converts an Oracle VARCHAR2 payload (as a common.B1Array) back into a Go string.
// It simply converts the byte slice back into a string; no Oracle-specific processing is needed.
// No error is returned for nil/empty input; it decodes to an empty string.
//
// Input:
//   - value (common.B1Array): The byte slice containing the Oracle VARCHAR2 payload.
//
// Output:
//   - (string, error): The decoded Go string (never trims or modifies content), error is always nil.
//
// Example:
//
//	encoded := common.B1Array([]byte("Hello"))
//	decoded, err := DecodeVarchar(encoded)
func DecodeVarchar(value driverCommon.B1Array) (string, error) {
	return string(value), nil
}

// EncodeChar encodes a Go string for Oracle CHAR (fixed-length, space-padded by the server based on metadata).
// The client does not pad; the server is responsible for padding to the column width.
//
// Input:
//   - v (string): The Go string to be encoded into Oracle CHAR.
//
// Output:
//   - common.B1Array: A byte slice representing the Oracle CHAR payload.
//
// Errors:
//   - None.
//
// Example:
//
//	encoded := EncodeChar("A1")
func EncodeChar(v driver.Value) (driverCommon.B1Array, error) { return EncodeVarchar(v.(string)) }

// DecodeChar converts an Oracle CHAR payload into a Go string without trimming trailing spaces.
// Padding spaces (added by the server for CHAR columns) are preserved as-is.
// No error is returned for nil/empty input; it decodes to an empty string.
//
// Input:
//   - value (common.B1Array): The byte slice containing the Oracle CHAR payload.
//
// Output:
//   - (string, error): The decoded Go string (preserving any trailing spaces). Error is always nil.
//
// Example:
//
//	decoded, err := DecodeChar(common.B1Array([]byte{'A','1',' ',' ',' '})) // returns "A1   "
func DecodeChar(value driverCommon.B1Array) (string, error) {
	return string(value), nil
}

// DecodeCharNullString converts a CHAR payload into sql.NullString without trimming spaces.
// Nil or zero-length input yields an empty string with Valid=true (no special NULL mapping at this layer).
//
// Input:
//   - value (common.B1Array): The byte slice containing the Oracle CHAR payload.
//
// Output:
//   - (sql.NullString, error): The NullString populated from the payload. Error is always nil.
//
// Example:
//
//	ns, err := DecodeCharNullString(common.B1Array("A1   ")) // ns.String == "A1   ", ns.Valid == true
func DecodeCharNullString(value driverCommon.B1Array) (sql.NullString, error) {
	s, err := DecodeChar(value)
	if err != nil {
		return sql.NullString{String: "", Valid: false}, err
	}
	return sql.NullString{String: s, Valid: true}, nil
}

// EncodeNVarchar2 encodes a Go string as an Oracle NVARCHAR2 payload.
// At this layer, strings are assumed to be UTF-8 (AL32UTF8), so the function returns the raw UTF-8 bytes.
//
// Input:
//   - v (string): The Go string to be encoded into Oracle NVARCHAR2.
//
// Output:
//   - common.B1Array: A byte slice containing the UTF-8 encoded payload.
//
// Errors:
//   - None.
//
// Example:
//
//	encoded := EncodeNVarchar2("ã“ã‚“ã«ã¡ã¯")
func EncodeNVarchar2(v driver.Value) (driverCommon.B1Array, error) {
	// Encode string into a byte slice (UTF-8).
	return driverCommon.B1Array([]byte(v.(string))), nil
}

// DecodeNVarchar2UTF8 decodes an Oracle NVARCHAR2 payload (UTF-8 encoded) into a Go string.
// No error is returned for nil/empty input; it decodes to an empty string.
//
// Input:
//   - value (common.B1Array): The UTF-8 encoded byte slice received for NVARCHAR2.
//
// Output:
//   - (string, error): The decoded Go string (no trimming or transformation). Error is always nil.
//
// Example:
//
//	decoded, err := DecodeNVarchar2UTF8(common.B1Array([]byte("å±±ç”°å¤ªéƒŽ")))
func DecodeNVarchar2UTF8(value driverCommon.B1Array) (string, error) {
	// Directly convert the byte slice to a Go string (for AL32UTF8 NVARCHAR2).
	return string(value), nil
}

// DecodeNVarchar2AL16UTF16 decodes an NVARCHAR2 payload encoded in AL16UTF16 (UTF-16BE) to a Go string.
func DecodeNVarchar2AL16UTF16(value driverCommon.B1Array) (string, error) {
	return DecodeUTF16BEToString(value)
}

// EncodeNChar encodes a Go string for Oracle NCHAR (fixed-length Unicode, space-padded by the server).
// The client does not pad; Oracle's server is responsible for padding to the column width.
//
// Input:
//   - v (string): The Go string to be encoded into Oracle NCHAR.
//
// Output:
//   - common.B1Array: A byte slice representing the Oracle NCHAR payload.
//
// Errors:
//   - None.
//
// Example:
//
//	encoded := EncodeNChar("US")
func EncodeNChar(v driver.Value) (driverCommon.B1Array, error) {
	// Convert string to a byte slice (UTF-8 encoding).
	return EncodeChar(v.(string))
}

// DecodeNCharUTF8 decodes an Oracle NCHAR payload into a Go string without trimming.
// Padding spaces (added by the server for NCHAR columns) are preserved as-is.
//
// Input:
//   - value (common.B1Array): The byte slice containing the Oracle NCHAR payload.
//
// Output:
//   - (string, error): The decoded Go string (preserving any trailing spaces). Error is always nil.
//
// Example:
//
//	decoded, err := DecodeNCharUTF8(common.B1Array([]byte{'U','S',' '})) // returns "US "
func DecodeNCharUTF8(value driverCommon.B1Array) (string, error) {
	return string(value), nil
}

// DecodeNCharAL16UTF16 decodes an NCHAR payload encoded in AL16UTF16 (UTF-16BE) without trimming.
// Padding spaces are preserved as-is.
func DecodeNCharAL16UTF16(value driverCommon.B1Array) (string, error) {
	decoded, err := DecodeUTF16BEToString(value)
	if err != nil {
		return "", err
	}
	return decoded, nil
}

// DecodeUTF16BEToString converts a UTF-16BE byte slice (no BOM) into a Go string.
func DecodeUTF16BEToString(value driverCommon.B1Array) (string, error) {
	// Length must be even for UTF-16 code units.
	if len(value)%2 != 0 {
		common.Odl.Error("DecodeUTF16BEToString: odd length input for UTF-16BE")
		return "", common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "UTF16BE", "Decode", driverCommon.ReasonInvalidLength, "even length")
	}
	units := make([]uint16, len(value)/2)
	for i := 0; i < len(units); i++ {
		units[i] = binary.BigEndian.Uint16(value[i*2 : i*2+2])
	}
	runes := utf16.Decode(units)
	return string(runes), nil
}
