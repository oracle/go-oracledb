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
	"strconv"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/ttc/converters"
)

// scalarNode implements common.JSONScalarNode.
type scalarNode struct {
	nodeBase
	// opcode identifies the scalar type.
	opcode common.UB1
}

// Kind implements common.JSONNode.Kind.
//
// Input:
//   - none.
//
// Output:
//   - common.KindScalar.
//
// Errors:
//   - none.
func (scalar *scalarNode) Kind() common.Kind {
	return common.KindScalar
}

// newScalarNodeAt parses one scalar node at offset and returns *scalarNode.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - offset: absolute document offset of the scalar node.
//
// Output:
//   - *scalarNode rooted at offset.
//
// Errors:
//   - invalid scalar opcode.
//   - buffer-read failure.
func newScalarNodeAt(buf *osonBuffer, header *osonHeader, offset int) (*scalarNode, error) {
	opcode, err := buf.readUB1At(offset)
	if err != nil {
		common.Odl.Error("newScalarNodeAt: failed", "error", err, "offset", offset)
		return nil, err
	}

	return &scalarNode{
		nodeBase: nodeBase{
			buf:    buf,
			header: header,
			offset: offset,
		},
		opcode: opcode,
	}, nil
}

// GetValue implements common.JSONNode.GetValue.
//
// Input:
//   - opt: JSON materialization option.
//
// Output:
//   - decoded scalar value.
//
// Errors:
//   - unsupported scalar opcode or payload decode failure.
func (scalar *scalarNode) GetValue(opt common.JSONOption) (any, error) {
	return scalar.Value(opt)
}

// StringWithOption implements common.JSONNode.StringWithOption.
//
// Input:
//   - opts: JSON materialization option.
//
// Output:
//   - JSON text for the scalar value.
//
// Errors:
//   - unsupported scalar opcode, payload decode failure, or JSON marshal failure.
func (scalar *scalarNode) StringWithOption(opts common.JSONOption) (string, error) {
	value, err := scalar.Value(opts)
	if err != nil {
		return "", err
	}

	text, err := json.Marshal(value)
	if err != nil {
		common.Odl.Error("scalarNode.StringWithOption: failed", "error", err, "offset", scalar.offset, "opcode", scalar.opcode)
		return "", common.NewOracleError(common.OsonBufferError, err)
	}
	return string(text), nil
}

// Value implements common.JSONScalarNode.Value.
//
// Input:
//   - opts: JSON materialization option.
//
// Output:
//   - decoded scalar value.
//
// Errors:
//   - unsupported scalar opcode or payload decode failure.
func (scalar *scalarNode) Value(opts common.JSONOption) (any, error) {
	value, err := _decodeScalarValue(scalar, opts)
	if err != nil {
		common.Odl.Error("scalarNode.Value: failed", "error", err, "offset", scalar.offset, "opcode", scalar.opcode)
		return nil, err
	}

	return value, nil
}

const (
	// Compact numeric opcode families encode payload width in the low bits.
	_compactSigned32LengthMask = 0x07
	_compactSigned64LengthMask = 0x0F
	_compactNumberLengthMask   = 0x0F
	_compactNumberLengthBias   = 1

	_jsonFloatBitSize = 64
)

// _decodeScalarValue decodes one scalar payload from its opcode.
func _decodeScalarValue(scalar *scalarNode, opts common.JSONOption) (any, error) {
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
		common.Odl.Debug("decodeScalarValue: short string",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return converters.DecodeVarchar(raw)

	// Compact signed32 stores the payload width in the opcode.
	case isCompactSigned32Opcode(opcode):
		payloadLen := int(opcode & _compactSigned32LengthMask)
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: compact signed32",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return decodeOracleNumberValue(raw, opts)

	// Compact signed64 stores the payload width in the opcode.
	case isCompactSigned64Opcode(opcode):
		payloadLen := int(opcode & _compactSigned64LengthMask)
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: compact signed64",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return decodeOracleNumberValue(raw, opts)

	// Compact NUMBER stores the payload width in the opcode.
	case isCompactOracleNumberOpcode(opcode):
		payloadLen := int(opcode&_compactNumberLengthMask) + _compactNumberLengthBias
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: compact oracle number",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return decodeOracleNumberValue(raw, opts)

	// Compact decimal stores the payload width in the opcode.
	case isCompactDecimalOpcode(opcode):
		payloadLen := int(opcode&_compactNumberLengthMask) + _compactNumberLengthBias
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: compact decimal number",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: string ub1",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: string ub2",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: string ub4",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: explicit oracle number",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: string number",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return decodeStringNumberValue(raw, opts)

	// Binary float uses a fixed 4-byte payload.
	case opcode == osonOpBinaryFloat:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonBinaryFloatPayloadSize)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: binary float",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		value, err := converters.DecodeBinaryFloat(raw)
		if err != nil {
			return nil, err
		}
		if opts == common.JSONOptNumberAsString {
			return common.JSONNumber(strconv.FormatFloat(float64(value), 'g', -1, 32)), nil
		}

		return float64(value), nil

	// Binary double uses a fixed 8-byte payload.
	case opcode == osonOpBinaryDouble:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonBinaryDoublePayloadSize)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: binary double",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		value, err := converters.DecodeBinaryDouble(raw)
		if err != nil {
			return nil, err
		}
		if opts == common.JSONOptNumberAsString {
			return common.JSONNumber(strconv.FormatFloat(value, 'g', -1, 64)), nil
		}
		return value, nil

	// DATE uses the fixed 7-byte Oracle DATE layout.
	case opcode == osonOpDate:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonDatePayloadSize)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: date",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return converters.DecodeDate(raw)

	// TIMESTAMP and TIMESTAMP7 share a decoder with different widths.
	case opcode == osonOpTimestamp || opcode == osonOpTimestamp7:
		payloadLen := osonTimestampPayloadSize
		if opcode == osonOpTimestamp7 {
			payloadLen = osonTimestamp7PayloadSize
		}
		raw, err := buf.readSliceAt(offset+osonUB1Size, payloadLen)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: timestamp",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return converters.DecodeTimestamp(raw)

	// TIMESTAMP WITH TIME ZONE uses a fixed payload.
	case opcode == osonOpTimestampTZ:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonTimestampTZPayloadSize)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: timestamp tz",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return converters.DecodeTimestampWithTimeZone(raw)

	// INTERVAL YEAR TO MONTH uses a fixed payload.
	case opcode == osonOpIntervalYM:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonIntervalYMPayloadSize)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: interval ym",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return converters.DecodeIntervalYearToMonth(raw)

	// INTERVAL DAY TO SECOND uses a fixed payload.
	case opcode == osonOpIntervalDS:
		raw, err := buf.readSliceAt(offset+osonUB1Size, osonIntervalDSPayloadSize)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: interval ds",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return converters.DecodeIntervalDayToSecond(raw)

	// ID uses a UB1 length with a bounded payload.
	case opcode == osonOpID:
		length, err := buf.readUB1At(offset + osonUB1Size)
		if err != nil {
			return nil, err
		}
		if length > osonIDMaxPayloadSize {
			return nil, common.NewOracleError(common.OsonBufferError, nil)
		}
		raw, err := buf.readSliceAt(offset+osonUB2Size, int(length))
		if err != nil {
			return nil, err
		}
		common.Odl.Debug("decodeScalarValue: id",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: binary ub2",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
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
		common.Odl.Debug("decodeScalarValue: binary ub4",
			"offset", offset,
			"opcode", opcode,
			"payloadLen", len(raw))
		return append([]byte(nil), raw...), nil

	default:
		return nil, common.NewOracleError(common.OsonUnsupportedScalarError, nil, opcode)
	}
}

// decodeOracleNumberValue decodes an Oracle NUMBER payload and preserves its
// decimal text when JSONOptNumberAsString is requested.
func decodeOracleNumberValue(payload common.B1Array, opts common.JSONOption) (any, error) {
	text, err := converters.DecodeExactDecimal(payload)
	if err != nil {
		return nil, err
	}
	if opts == common.JSONOptNumberAsString {
		return common.JSONNumber(text), nil
	}
	value, err := strconv.ParseFloat(text, _jsonFloatBitSize)
	if err != nil {
		return nil, common.NewOracleError(common.OsonParsingError, fmt.Errorf("invalid oracle-number payload %q: %w", text, err))
	}
	return value, nil
}

// decodeStringNumberValue decodes Oracle numbers that are represented as strings.
func decodeStringNumberValue(payload common.B1Array, opts common.JSONOption) (any, error) {
	text := string(payload)
	if opts == common.JSONOptNumberAsString {
		return common.JSONNumber(text), nil
	}
	value, err := strconv.ParseFloat(text, _jsonFloatBitSize)
	if err != nil {
		return nil, common.NewOracleError(common.OsonParsingError, fmt.Errorf("invalid string-number payload %q: %w", text, err))
	}
	return value, nil
}
