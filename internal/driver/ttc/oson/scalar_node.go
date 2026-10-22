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

package oson

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// scalarNode implements JSONScalarNode for an OSON scalar.
//
// The concrete type is scoped to package oson. Callers obtain a scalar node
// through Parse, which exposes it as JSONNode and keeps the OSON wire
// representation inside this package. Because JSONScalarNode embeds JSONNode,
// *scalarNode satisfies both interfaces. After checking Kind, a caller can type
// assert the returned JSONNode to JSONScalarNode for scalar-specific
// access.
//
// Creating a scalarNode reads only the scalar opcode. nodeBase retains the OSON
// document context and opcode identifies how to decode the payload. The payload
// remains encoded until Value, GetValue, or String requests its Go or
// JSON representation.
type scalarNode struct {
	nodeBase
	// opcode identifies the scalar type.
	opcode drvCommon.UB1
}

// Kind implements the JSONNode interface.
//
// Parameters:
//   - none.
//
// Returns:
//   - drvCommon.KindScalar.
//
// Errors:
//   - none.
func (scalar *scalarNode) Kind() drvCommon.Kind {
	return drvCommon.KindScalar
}

// newScalarNodeAt parses one scalar node at offset and returns *scalarNode.
//
// Parameters:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - offset: absolute document offset of the scalar node.
//   - opcode: opcode already read by newNodeAt; payload validation is lazy.
//
// Returns:
//   - *scalarNode rooted at offset.
//
// Errors:
//   - none; Value validates the scalar opcode and payload.
func newScalarNodeAt(buf *osonBuffer, header *osonHeader, offset int, opcode drvCommon.UB1) *scalarNode {
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("newScalarNodeAt: parsed", "offset", offset, "opcode", opcode)
	}
	return &scalarNode{
		nodeBase: nodeBase{
			buf:    buf,
			header: header,
			offset: offset,
		},
		opcode: opcode,
	}
}

// GetValue implements the JSONNode interface.
//
// Parameters:
//   - opt: JSON materialization option.
//
// Returns:
//   - decoded scalar value.
//
// Errors:
//   - unsupported scalar opcode or payload decode failure.
func (scalar *scalarNode) GetValue(opt drvCommon.JSONConversionOptions) (any, error) {
	return scalar.Value(opt)
}

// String implements the JSONNode interface.
//
// Returns:
//   - JSON text for the scalar value.
//
// Errors:
//   - unsupported scalar opcode, payload decode failure, or JSON marshal failure.
func (scalar *scalarNode) String() (string, error) {
	text, err := json.Marshal(scalar)
	if err != nil {
		common.Odl.Debug("scalarNode.String: failed", "error", err, "offset", scalar.offset, "opcode", scalar.opcode)
		return "", common.NewOracleError(oracleErrors.JSONRenderingError, err)
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("scalarNode.String: completed", "offset", scalar.offset, "opcode", scalar.opcode, "textBytes", len(text))
	}
	return string(text), nil
}

// MarshalJSON implements encoding/json.Marshaler.
//
// Scalar marshaling inspects the decoded Go type and OSON opcode so each scalar
// can choose its JSON representation without requiring a materialized parent
// map or slice to rewrite the value.
func (scalar *scalarNode) MarshalJSON() ([]byte, error) {
	value, err := scalar.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
	if err != nil {
		return nil, err
	}

	switch scalarValue := value.(type) {
	case float64:
		// Nonfinite binary numbers use Oracle's JSON string spellings.
		switch {
		case math.IsNaN(scalarValue):
			value = "Nan"
		case math.IsInf(scalarValue, 1):
			value = "Inf"
		case math.IsInf(scalarValue, -1):
			value = "-Inf"
		}
	case time.Time:
		switch scalar.opcode {
		case osonOpDate, osonOpTimestamp, osonOpTimestamp7:
			// DATE and TIMESTAMP do not carry a timezone in their OSON payload.
			// Avoid time.Time's default JSON form, which adds the local offset.
			value = scalarValue.Format("2006-01-02T15:04:05.999999999")
		}
	case []byte:
		// OSON binary values render as hexadecimal JSON strings rather than
		// encoding/json's default base64 strings.
		value = fmt.Sprintf("%X", scalarValue)
	}
	text, err := json.Marshal(value)
	if err != nil {
		common.Odl.Debug("scalarNode.MarshalJSON: failed", "error", err, "offset", scalar.offset, "opcode", scalar.opcode)
		return nil, common.NewOracleError(oracleErrors.JSONRenderingError, err)
	}
	return text, nil
}

// Value implements the JSONScalarNode interface.
//
// Parameters:
//   - opts: JSON materialization option.
//
// Returns:
//   - decoded scalar value.
//
// Errors:
//   - unsupported scalar opcode or payload decode failure.
func (scalar *scalarNode) Value(opts drvCommon.JSONConversionOptions) (any, error) {
	value, err := _decodeScalarValue(scalar, opts)
	if err != nil {
		common.Odl.Debug("scalarNode.Value: failed", "error", err, "offset", scalar.offset, "opcode", scalar.opcode)
		return nil, err
	}

	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("scalarNode.Value: completed", "offset", scalar.offset, "opcode", scalar.opcode, "options", opts)
	}
	return value, nil
}

const (
	// Compact numeric opcode families encode payload width in the low bits.
	_compactSigned32LengthMask = 0x07
	_compactSigned64LengthMask = 0x0F
	_compactNumberLengthMask   = 0x0F
	_compactNumberLengthBias   = 1

	// _compactSigned64MaxPayloadLen is the largest NUMBER payload the compact
	// signed-64 opcode family can promise.
	_compactSigned64MaxPayloadLen = 12

	_jsonFloatBitSize = 64
)

// _decodeScalarValue decodes one scalar payload from its opcode.
func _decodeScalarValue(scalar *scalarNode, opts drvCommon.JSONConversionOptions) (any, error) {
	offset := scalar.offset
	opcode := scalar.opcode
	buf := scalar.buf

	switch {
	case opcode == osonOpNull:
		return nil, nil

	case opcode == osonOpTrue:
		return true, nil

	case opcode == osonOpFalse:
		return false, nil

	// Short strings encode the byte length in the opcode.
	case isShortStringOpcode(opcode):
		raw, err := buf.readSliceAt(offset+osonUB1Size, int(opcode))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: short string",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeVarchar(raw)

	// Compact signed32 stores the payload width in the opcode.
	case isCompactSigned32Opcode(opcode):
		// The 0x40-0x47 family promises a 1-7 byte Oracle NUMBER, so a
		// zero width (opcode 0x40) is a malformed document, not a value.
		payloadLen := int(opcode & _compactSigned32LengthMask)
		if payloadLen == 0 {
			details := "compact signed32 has zero NUMBER payload length"
			common.Odl.Debug("decodeScalarValue: failed", "reason", details, "offset", offset, "opcode", opcode)
			return nil, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: compact signed32",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeSignedIntegerValue[int32](raw, opts)

	// Compact signed64 stores the payload width in the opcode.
	case isCompactSigned64Opcode(opcode):
		// The 0x50-0x5f family promises a 1-12 byte Oracle NUMBER. The 4-bit
		// width can claim 0 or 13-15 bytes (0x50, 0x5d-0x5f); those opcodes
		// are malformed and must not reach the integer decoder.
		payloadLen := int(opcode & _compactSigned64LengthMask)
		if payloadLen == 0 || payloadLen > _compactSigned64MaxPayloadLen {
			details := fmt.Sprintf("compact signed64 has invalid NUMBER payload length %d (max %d)", payloadLen, _compactSigned64MaxPayloadLen)
			common.Odl.Debug("decodeScalarValue: failed", "reason", details, "offset", offset, "opcode", opcode, "payloadLength", payloadLen, "maxPayloadLength", _compactSigned64MaxPayloadLen)
			return nil, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: compact signed64",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeSignedIntegerValue[int64](raw, opts)

	// Compact NUMBER stores the payload width in the opcode.
	case isCompactOracleNumberOpcode(opcode):
		payloadLen := int(opcode&_compactNumberLengthMask) + _compactNumberLengthBias
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: compact oracle number",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeOracleNumberValue(raw, opts)

	// Compact decimal stores the payload width in the opcode.
	case isCompactDecimalOpcode(opcode):
		payloadLen := int(opcode&_compactNumberLengthMask) + _compactNumberLengthBias
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: compact decimal number",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeOracleNumberValue(raw, opts)

	// String UB1 uses a 1-byte length prefix.
	case opcode == osonOpStringUB1:
		length, err := buf.readUB1At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonUB2Size, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: string ub1",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeVarchar(raw)

	// String UB2 uses a 2-byte length prefix.
	case opcode == osonOpStringUB2:
		length, err := buf.readUB2At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonScalarHeaderSizeUB2, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: string ub2",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeVarchar(raw)

	// String UB4 uses a 4-byte length prefix.
	case opcode == osonOpStringUB4:
		length, err := buf.readUB4At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonScalarHeaderSizeUB4, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: string ub4",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeVarchar(raw)

	// Oracle NUMBER and DECIMAL use a 1-byte length prefix.
	case opcode == osonOpOracleNumber || opcode == osonOpOracleDecimal:
		length, err := buf.readUB1At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonUB2Size, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: explicit oracle number",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeOracleNumberValue(raw, opts)

	// String number stores decimal text bytes.
	case opcode == osonOpStringNumber:
		length, err := buf.readUB1At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonUB2Size, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: string number",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeStringNumberValue(raw, opts)

	// Binary float uses a fixed 4-byte payload.
	case opcode == osonOpBinaryFloat:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonBinaryFloatPayloadSize)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: binary float",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		value, err := converters.DecodeBinaryFloat(raw)
		if err != nil {
			return nil, err
		}
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			common.Odl.Debug("decodeScalarValue: preserving nonfinite binary number as float64", "offset", offset, "opcode", opcode)
			return float64(value), nil
		}
		if opts.NumberMode == drvCommon.JSONNumberAsJSONNumber {
			return json.Number(strconv.FormatFloat(float64(value), 'g', -1, 32)), nil
		}

		return float64(value), nil

	// Binary double uses a fixed 8-byte payload.
	case opcode == osonOpBinaryDouble:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonBinaryDoublePayloadSize)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: binary double",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		value, err := converters.DecodeBinaryDouble(raw)
		if err != nil {
			return nil, err
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			common.Odl.Debug("decodeScalarValue: preserving nonfinite binary number as float64", "offset", offset, "opcode", opcode)
			return value, nil
		}
		if opts.NumberMode == drvCommon.JSONNumberAsJSONNumber {
			return json.Number(strconv.FormatFloat(value, 'g', -1, 64)), nil
		}
		return value, nil

	// DATE uses the fixed 7-byte Oracle DATE layout.
	case opcode == osonOpDate:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonDatePayloadSize)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: date",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeDate(raw)

	// TIMESTAMP and TIMESTAMP7 share a decoder with different widths.
	case opcode == osonOpTimestamp || opcode == osonOpTimestamp7:
		payloadLen := int(converters.MaxDateLength)
		if opcode == osonOpTimestamp7 {
			payloadLen = osonTimestamp7PayloadSize
		}
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: timestamp",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeTimestamp(raw)

	// TIMESTAMP WITH TIME ZONE uses a fixed payload.
	case opcode == osonOpTimestampTZ:
		raw, err := buf.readSliceAt(offset+osonUB1Size, int(converters.MaxTimeStampLength))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: timestamp tz",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return decodeTimestampTZ(raw)

	// INTERVAL YEAR TO MONTH uses a fixed payload.
	case opcode == osonOpIntervalYM:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonIntervalYMPayloadSize)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: interval ym",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeIntervalYearToMonth(raw)

	// INTERVAL DAY TO SECOND uses a fixed payload.
	case opcode == osonOpIntervalDS:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonIntervalDSPayloadSize)
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: interval ds",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return converters.DecodeIntervalDayToSecond(raw)

	// ID uses a UB1 length with a bounded payload.
	case opcode == osonOpID:
		length, err := buf.readUB1At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonUB2Size, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: id",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return append([]byte(nil), raw...), nil

	// Binary UB2 uses a 2-byte length prefix.
	case opcode == osonOpBinaryUB2:
		length, err := buf.readUB2At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		raw, err := buf.readSliceAt(offset+osonScalarHeaderSizeUB2, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: binary ub2",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return append([]byte(nil), raw...), nil

	// Binary UB4 uses a 4-byte length prefix.
	case opcode == osonOpBinaryUB4:
		length, err := buf.readUB4At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}

		raw, err := buf.readSliceAt(offset+osonScalarHeaderSizeUB4, int(length))
		if err != nil {
			return nil, err
		}
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: binary ub4",
				"offset", offset,
				"opcode", opcode,
				"payloadLen", len(raw))
		}
		return append([]byte(nil), raw...), nil

	case opcode == osonOpNativeInteger || opcode == osonOpExtendedBinary:
		if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
			common.Odl.Finest("decodeScalarValue: unsupported scalar", "offset", offset, "opcode", opcode)
		}
		return nil, common.NewOracleError(oracleErrors.OsonUnsupportedScalarError, nil, opcode)

	default:
		details := fmt.Sprintf("unknown scalar opcode 0x%02x", opcode)
		common.Odl.Debug("decodeScalarValue: failed", "reason", details, "offset", offset, "opcode", opcode)
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}
}

// decodeSignedIntegerValue decodes an SB4 or SB8 Oracle NUMBER using the
// signed integer type associated with its OSON opcode family.
func decodeSignedIntegerValue[T ~int32 | ~int64](payload drvCommon.B1Array, opts drvCommon.JSONConversionOptions) (any, error) {
	text, err := converters.DecodeExactDecimalAsString(payload)
	if err != nil {
		return nil, err
	}
	value, _ := strconv.ParseInt(text, 10, 64)
	result := T(value)

	switch opts.NumberMode {
	case drvCommon.JSONNumberAsJSONNumber:
		return json.Number(text), nil
	case drvCommon.JSONNumberAsFloat64:
		return float64(value), nil
	default:
		return result, nil
	}
}

// decodeOracleNumberValue decodes an Oracle NUMBER payload and preserves its
// decimal text when JSONNumberAsJSONNumber is requested.
func decodeOracleNumberValue(payload drvCommon.B1Array, opts drvCommon.JSONConversionOptions) (any, error) {
	text, err := converters.DecodeExactDecimalAsString(payload)
	if err != nil {
		return nil, err
	}
	if opts.NumberMode != drvCommon.JSONNumberAsFloat64 {
		return json.Number(text), nil
	}
	value, err := strconv.ParseFloat(text, _jsonFloatBitSize)
	if err != nil {
		common.Odl.Debug("decodeOracleNumberValue: failed", "reason", "invalid Oracle NUMBER text", "length", len(text))
		return nil, common.NewOracleError(oracleErrors.InternalError, err)
	}
	return value, nil
}

// decodeStringNumberValue decodes Oracle numbers that are represented as strings.
func decodeStringNumberValue(payload drvCommon.B1Array, opts drvCommon.JSONConversionOptions) (any, error) {
	text := string(payload)
	if text == "" {
		common.Odl.Debug("decodeStringNumberValue: failed", "reason", "empty string NUMBER text")
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}
	if _, err := json.Marshal(json.Number(text)); err != nil {
		common.Odl.Debug("decodeStringNumberValue: failed", "reason", "invalid string NUMBER text", "length", len(text))
		return nil, common.NewOracleError(oracleErrors.InternalError, err)
	}
	if opts.NumberMode != drvCommon.JSONNumberAsFloat64 {
		return json.Number(text), nil
	}
	value, err := strconv.ParseFloat(text, _jsonFloatBitSize)
	if err != nil {
		common.Odl.Debug("decodeStringNumberValue: failed", "reason", "invalid string NUMBER text", "length", len(text))
		return nil, common.NewOracleError(oracleErrors.InternalError, err)
	}
	return value, nil
}
