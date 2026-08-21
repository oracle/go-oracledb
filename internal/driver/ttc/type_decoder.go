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

package ttc

import (
	"database/sql/driver"
	"reflect"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// NumberScaleFloatSentinel is the scale value the server uses to denote FLOAT columns
	// that are marshalled over the NUMBER wire representation.
	NumberScaleFloatSentinel int8 = -127
)

/*
DecodeNumberColumn decodes a TTC NUMBER payload into the closest database/sql compatible Go value.

Description:

	Oracle NUMBER payloads can represent integers, fixed-point decimals, or floating-point values
	depending on precision/scale metadata. This helper attempts integer decoding first to preserve
	exactness, then falls back to a decimal string or float64 representation as appropriate.

Parameters:
  - columnContext: Column metadata (precision/scale, name/index) required to interpret the wire value.
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: One of:
  - int64 for integer values (and for float-sentinel values that are actually integers),
  - string for fixed-point decimals (canonical decimal representation),
  - float64 for FLOAT/NUMBER values with the float sentinel scale that require float decoding.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded for the given metadata.
*/
func DecodeNumberColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	// Prefer int64 for integer values; fallback to decimal string otherwise.
	if columnContext.Scale == 0 {
		v, err := converters.DecodeInt(data)
		if err == nil {
			return v, nil
		}
		if s, err2 := converters.DecodeDecimal(data, int(columnContext.Precision), int(columnContext.Scale)); err2 == nil {
			return s, nil
		}
		return nil, rowDecodeError(columnContext, err, "NUMBER")
	}

	if columnContext.Scale == NumberScaleFloatSentinel {
		// Attempt integer decoding first to preserve exactness when the wire value
		// represents an integer (e.g., SELECT 1.0). This mirrors decodeColumnValue
		// behaviour, falling back to float decoding only when necessary.
		if v, err := converters.DecodeInt(data); err == nil {
			return v, nil
		}
		fv, err := converters.DecodeFloat(data, int(columnContext.Precision), int(columnContext.Scale))
		if err != nil {
			return nil, rowDecodeError(columnContext, err, "FLOAT")
		}
		return float64(fv), nil
	}

	val, err := converters.DecodeDecimal(data, int(columnContext.Precision), int(columnContext.Scale))
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "NUMBER")
	}
	return val, nil
}

func GetScanTypeForNumberColumn(colCtx columnContext) reflect.Type {
	// Prefer int64 for integer values; fallback to decimal string otherwise.
	if colCtx.Scale == 0 {
		return reflect.TypeFor[int64]()
	}

	if colCtx.Scale == NumberScaleFloatSentinel {
		return reflect.TypeFor[float64]()
	}

	return reflect.TypeFor[string]()
}

/*
DecodeVarcharColumn decodes a TTC VARCHAR2/NVARCHAR2 payload into a Go string.

Description:

	Decodes VARCHAR2 and NVARCHAR2 values while honoring the declared character set form and ID.
	For NCHAR/NVARCHAR2 (charset form 2), the converter is selected based on charset ID.

Parameters:
  - columnContext: Column metadata, including charset form and charset ID.
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: A Go string containing the decoded text.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeVarcharColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	if columnContext.CharsetForm == 2 {
		switch columnContext.CharsetID {
		case uint16(al16Utf16CharSet):
			s, err := converters.DecodeNVarchar2AL16UTF16(data)
			if err != nil {
				return nil, rowDecodeError(columnContext, err, "NVARCHAR2")
			}
			return s, nil
		case uint16(al32Utf8CharSet):
			s, err := converters.DecodeNVarchar2UTF8(data)
			if err != nil {
				return nil, rowDecodeError(columnContext, err, "NVARCHAR2")
			}
			return s, nil
		}
	}

	s, err := converters.DecodeVarchar(data)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "VARCHAR2")
	}
	return s, nil
}

/*
DecodeCharColumn decodes a TTC CHAR/NCHAR payload into a Go string.

Description:

	Decodes CHAR and NCHAR values, preserving Oracle fixed-width semantics and handling multi-byte
	character sets when required. For NCHAR (charset form 2), the converter is selected based on
	charset ID.

Parameters:
  - columnContext: Column metadata, including charset form and charset ID.
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: A Go string containing the decoded text.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeCharColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	if columnContext.CharsetForm == 2 {
		switch columnContext.CharsetID {
		case uint16(al16Utf16CharSet):
			s, err := converters.DecodeNCharAL16UTF16(data)
			if err != nil {
				return nil, rowDecodeError(columnContext, err, "NCHAR")
			}
			return s, nil
		case uint16(al32Utf8CharSet):
			s, err := converters.DecodeNCharUTF8(data)
			if err != nil {
				return nil, rowDecodeError(columnContext, err, "NCHAR")
			}
			return s, nil
		}
	}

	s, err := converters.DecodeChar(data)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "CHAR")
	}
	return s, nil
}

/*
DecodeBooleanColumn decodes a TTC BOOLEAN payload into a Go bool.

Description:

	Maps the single-byte TTC boolean encoding into a Go bool. Non-zero bytes are treated as true.

Parameters:
  - _: Unused column metadata (present for a uniform decode function signature).
  - data: Raw TTC payload bytes for the column (at least one byte).

Returns:
  - driver.Value: A Go bool value.
  - error: Always nil.

Errors:
  - None.
*/
func DecodeBooleanColumn(_ columnContext, data driverCommon.B1Array) (driver.Value, error) {
	return data[0] != 0, nil
}

/*
DecodeBinaryFloatColumn decodes a TTC BINARY_FLOAT payload into a Go float64.

Description:

	Decodes Oracle BINARY_FLOAT and returns float64 to satisfy database/sql expectations.

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: float64 containing the decoded value.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeBinaryFloatColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	fv, err := converters.DecodeBinaryFloat(data)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "BINARY_FLOAT")
	}
	return float64(fv), nil
}

/*
DecodeBinaryDoubleColumn decodes a TTC BINARY_DOUBLE payload into a Go float64.

Description:

	Decodes Oracle BINARY_DOUBLE and returns float64 to satisfy database/sql expectations.

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: float64 containing the decoded value.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeBinaryDoubleColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	fv, err := converters.DecodeBinaryDouble(data)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "BINARY_DOUBLE")
	}
	return float64(fv), nil
}

/*
DecodeIntervalYearToMonthColumn decodes a TTC INTERVAL YEAR TO MONTH payload into its canonical text representation.

Description:

	Converts INTERVAL YEAR TO MONTH payloads into their canonical text representation (for example, "02-03").

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: A string containing the interval value in canonical form.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeIntervalYearToMonthColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	s, err := converters.DecodeIntervalYearToMonth(data)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "INTERVAL YEAR TO MONTH")
	}
	return s, nil
}

/*
DecodeIntervalDayToSecondColumn decodes a TTC INTERVAL DAY TO SECOND payload into its canonical text representation.

Description:

	Converts INTERVAL DAY TO SECOND payloads into their canonical text representation (for example, "10 05:30:02.0").

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: A string containing the interval value in canonical form.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeIntervalDayToSecondColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	s, err := converters.DecodeIntervalDayToSecond(data)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "INTERVAL DAY TO SECOND")
	}
	return s, nil
}

/*
DecodeDateColumn decodes a TTC DATE payload into a time.Time value.

Description:

	Transforms Oracle DATE payloads into time.Time values with Oracle timezone semantics preserved by the
	underlying converter implementation.

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: time.Time containing the decoded date/time value.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeDateColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	tm, err := converters.DecodeDate(driverCommon.B1Array(data))
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "DATE")
	}
	return tm, nil
}

/*
DecodeTimestampColumn decodes a TTC TIMESTAMP payload into a time.Time value.

Description:

	Decodes Oracle TIMESTAMP payloads into time.Time values with sub-second precision maintained.

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: time.Time containing the decoded timestamp.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeTimestampColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	tm, err := converters.DecodeTimestamp(driverCommon.B1Array(data))
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "TIMESTAMP")
	}
	return tm, nil
}

/*
DecodeTimestampWithTimeZoneColumn decodes a TTC TIMESTAMP WITH TIME ZONE payload into a time.Time value.

Description:

	Decodes TIMESTAMP WITH TIME ZONE payloads and returns time.Time values normalized by the underlying converter.

Parameters:
  - columnContext: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: time.Time containing the decoded timestamp with time zone.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeTimestampWithTimeZoneColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	tm, err := converters.DecodeTimestampWithTimeZone(driverCommon.B1Array(data))
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "TIMESTAMP WITH TIME ZONE")
	}
	return tm, nil
}

/*
DecodeTimestampWithLocalTimeZoneColumn decodes a TTC TIMESTAMP WITH LOCAL TIME ZONE payload into a time.Time value.

Description:

	Decodes TIMESTAMP WITH LOCAL TIME ZONE payloads, which are normalized to the session time zone during decoding.

Parameters:
  - ctx: Column metadata used for error reporting (name/index).
  - data: Raw TTC payload bytes for the column.

Returns:
  - driver.Value: time.Time containing the decoded timestamp.
  - error: Non-nil if decoding fails.

Errors:
  - Returns a common.OracleError with code common.RowDecodeError via rowDecodeError when the payload
    cannot be decoded.
*/
func DecodeTimestampWithLocalTimeZoneColumn(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	tm, err := converters.DecodeTimestampWithLocalTimeZone(driverCommon.B1Array(data), columnContext.serverTimeZoneOffset)
	if err != nil {
		return nil, rowDecodeError(columnContext, err, "TIMESTAMP WITH LOCAL TIME ZONE")
	}
	return tm, nil
}

/*
DecodeClob decodes a TTC CLOB/NCLOB payload into a Go string.

Description:

	Decodes prefetched text LOB payloads using the LOB-specific charset metadata when available.
	AL16UTF16 payloads are decoded as big-endian UTF-16, while other payloads are treated as
	UTF-8 compatible text.

Parameters:
  - columnContext: Column metadata, including optional LOB charset information in LobContext.
  - data: Raw prefetched TTC payload bytes for the LOB column.

Returns:
  - driver.Value: A Go string containing the decoded CLOB/NCLOB text.
  - error: Always nil.

Errors:
  - None.
*/
func DecodeClob(columnContext columnContext, data driverCommon.B1Array) (driver.Value, error) {
	// LOB columns supply their own charset metadata via LobContext.
	if columnContext.LobContext.CharsetID == al16Utf16CharSet {
		return converters.DecodeUTF16BEToString(data)
	}
	// Default path: assume payload is UTF-8 compatible.
	return string(data), nil
}

/*
DecodeJson decodes a TTC JSON payload into a Go string. In V1, we are requesting BLOB
for JSON. As a result, the server sends it as utf-8 string. Hence, no decoding is
necessary.

Description:

	The current JSON fetch path provides prefetched JSON data as text-compatible bytes.
	This helper converts that payload directly into a Go string.

Parameters:
  - _: Unused column metadata (present for a uniform decode function signature).
  - data: Raw TTC payload bytes for the JSON column.

Returns:
  - driver.Value: A Go string containing the decoded JSON document.
  - error: Always nil.

Errors:
  - None.
*/
func DecodeJson(_ columnContext, data driverCommon.B1Array) (driver.Value, error) {
	// Default path: assume payload is UTF-8 compatible.
	return string(data), nil
}

/*
DecodeBlob decodes a TTC BLOB payload into raw bytes.

Description:

	BLOB payloads are already delivered as raw binary data, so this helper returns the
	prefetched bytes unchanged.

Parameters:
  - _: Unused column metadata (present for a uniform decode function signature).
  - data: Raw TTC payload bytes for the BLOB column.

Returns:
  - driver.Value: The raw BLOB bytes as common.B1Array.
  - error: Always nil.

Errors:
  - None.
*/
func DecodeBlob(_ columnContext, data driverCommon.B1Array) (driver.Value, error) {
	// LOB columns supply their own charset metadata via LobContext.
	return data, nil
}

/*
rowDecodeError wraps a decode failure with consistent Oracle error metadata.

Description:

	Provides the SQL type name along with the column name and zero-based column index to make row scan
	failures easier to diagnose.

Parameters:
  - columnContext: Column metadata (name/index) used to populate the error.
  - err: The underlying decode error.
  - typeName: Human readable SQL type name (for example, "NUMBER", "VARCHAR2").

Returns:
  - error: A common.OracleError with code common.RowDecodeError wrapping err.

Errors:
  - None (this function always returns an error value wrapping the provided err).
*/
func rowDecodeError(columnContext columnContext, err error, typeName string) error {
	return common.NewOracleError(oracleErrors.RowDecodeError, err, typeName, columnContext.Name, columnContext.Index)
}

/*
DecodeBinaryColumn returns the binary data as driver.Value

Parameters:
- columnContext: not used for binary
- data: []byte received in the wire.

Returns:
- common.B1Array

Errors:
- None
*/
func DecodeBinaryColumn(_ columnContext, data driverCommon.B1Array) (driver.Value, error) {
	return []byte(data), nil
}

func GetScanTypeForVarcharColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[string]()
}

func GetScanTypeForCharColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[string]()
}

func GetScanTypeForBooleanColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[bool]()
}

func GetScanTypeForBinaryFloatColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[float64]()
}

func GetScanTypeForDoubleColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[float64]()
}

func GetScanTypeForBinaryColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[[]byte]()
}

func GetScanTypeForTimestampWithLocalTimeZonColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[time.Time]()
}

func GetScanTypeForTimestampWithTimeZoneColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[time.Time]()
}

func GetScanTypeForTimestampColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[time.Time]()
}

func GetScanTypeForDateColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[time.Time]()
}

func GetScanTypeForIntervalDayToSecondColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[string]()
}

func GetScanTypeForIntervalYearToMonthColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[string]()
}

func GetScanTypeForJsonColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[string]()
}

func GetScanTypeForCLOBColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[string]()
}

func GetScanTypeForBLOBColumn(_ columnContext) reflect.Type {
	return reflect.TypeFor[[]byte]()
}
