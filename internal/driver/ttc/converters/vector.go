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
** is contributed to or provided by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
**
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
	"strconv"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// vectorTypeName identifies this converter in driver errors.
	vectorTypeName = "VECTOR"
	// maxVectorDimensions is Oracle's largest supported dense numeric VECTOR dimension count.
	maxVectorDimensions = 65535
	// maxVectorBinaryDimensions is the largest supported dense BINARY VECTOR dimension count.
	maxVectorBinaryDimensions = 65528

	// VECTOR envelope layout constants.
	vectorMagic           = 0xDB
	vectorVersionV0       = 0
	vectorBaseHeaderBytes = 9
	vectorFloat64Bytes    = 8
	vectorFloat32Bytes    = 4
	vectorNormBytes       = vectorFloat64Bytes
	vectorHeaderBytes     = vectorBaseHeaderBytes + vectorNormBytes
	vectorBitsPerByte     = 8

	// VECTOR envelope flags.
	vectorFlagOptional   = 0x8000
	vectorFlagLittle     = 0x0001
	vectorFlagNorm       = 0x0002
	vectorFlagIEEE       = 0x0008
	vectorFlagNormSource = 0x0010
	vectorFlagSparse     = 0x0020

	// VECTOR element type codes.
	vectorTypeFloat32 = 0x02
	vectorTypeFloat64 = 0x03
	vectorTypeInt8    = 0x04
	vectorTypeBinary  = 0x05
)

/*
EncodeVectorFloat64 encodes datatype.VectorFloat64 as a dense Oracle VECTOR(FLOAT64).

Input:
  - v: A datatype.VectorFloat64. A typed nil value represents SQL NULL.

Output:
  - The complete VECTOR envelope and dimensions in Oracle's ordered binary format.
  - An error when v has a different type or exceeds the dense VECTOR dimension limit.
*/
func EncodeVectorFloat64(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(datatype.VectorFloat64)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "datatype.VectorFloat64")
	}
	if values == nil {
		return nil, nil
	}
	if len(values) > maxVectorDimensions {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65535")
	}
	out := make(driverCommon.B1Array, vectorHeaderBytes+len(values)*vectorFloat64Bytes)
	writeVectorHeader(out, vectorVersionV0, vectorFlagNorm|vectorFlagNormSource, vectorTypeFloat64, uint32(len(values)))
	for i, value := range values {
		start := vectorHeaderBytes + i*vectorFloat64Bytes
		putOracleBinaryDouble(out[start:start+vectorFloat64Bytes], value)
	}
	return out, nil
}

/*
EncodeVectorFloat32 encodes datatype.VectorFloat32 as a dense Oracle VECTOR(FLOAT32).

Input:
  - v: A datatype.VectorFloat32. A typed nil value represents SQL NULL.

Output:
  - The complete VECTOR envelope and dimensions in Oracle's ordered binary format.
  - An error when v has a different type or exceeds the dense VECTOR dimension limit.
*/
func EncodeVectorFloat32(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(datatype.VectorFloat32)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "datatype.VectorFloat32")
	}
	if values == nil {
		return nil, nil
	}
	if len(values) > maxVectorDimensions {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65535")
	}
	out := make(driverCommon.B1Array, vectorHeaderBytes+len(values)*vectorFloat32Bytes)
	writeVectorHeader(out, vectorVersionV0, vectorFlagNorm|vectorFlagNormSource, vectorTypeFloat32, uint32(len(values)))
	for i, value := range values {
		start := vectorHeaderBytes + i*vectorFloat32Bytes
		putOracleBinaryFloat(out[start:start+vectorFloat32Bytes], value)
	}
	return out, nil
}

/*
EncodeVectorInt8 encodes datatype.VectorInt8 as a dense Oracle VECTOR(INT8).

Input:
  - v: A datatype.VectorInt8. A typed nil value represents SQL NULL.

Output:
  - The complete VECTOR envelope and one signed byte per dimension.
  - An error when v has a different type or exceeds the dense VECTOR dimension limit.
*/
func EncodeVectorInt8(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(datatype.VectorInt8)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "datatype.VectorInt8")
	}
	if values == nil {
		return nil, nil
	}
	if len(values) > maxVectorDimensions {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65535")
	}
	out := make(driverCommon.B1Array, vectorHeaderBytes+len(values))
	writeVectorHeader(out, vectorVersionV0, vectorFlagNorm|vectorFlagNormSource, vectorTypeInt8, uint32(len(values)))
	for i, value := range values {
		out[vectorHeaderBytes+i] = byte(value)
	}
	return out, nil
}

/*
EncodeVectorBinary encodes datatype.VectorBinary as a dense Oracle VECTOR(BINARY).

Input:
  - v: Packed dimensions in datatype.VectorBinary, with eight dimensions per byte.
    A typed nil value represents SQL NULL.

Output:
  - The complete VECTOR envelope and packed dimension bytes.
  - An error when v has a different type or exceeds the dense VECTOR dimension limit.
*/
func EncodeVectorBinary(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(datatype.VectorBinary)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "datatype.VectorBinary")
	}
	if values == nil {
		return nil, nil
	}
	if len(values) > maxVectorBinaryDimensions/8 {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65528")
	}
	out := make(driverCommon.B1Array, vectorHeaderBytes+len(values))
	writeVectorHeader(out, vectorVersionV0, vectorFlagNormSource, vectorTypeBinary, uint32(len(values))*vectorBitsPerByte)
	copy(out[vectorHeaderBytes:], values)
	return out, nil
}

/*
writeVectorHeader writes the fixed VECTOR envelope header and reserved norm field.

Input:
  - out: Storage for the complete header.
  - version, flags, typeCode, dimensions: The VECTOR envelope fields.

Output:
  - The header bytes are written into out. The reserved norm field is zeroed.
*/
func writeVectorHeader(out []byte, version byte, flags uint16, typeCode byte, dimensions uint32) {
	out[0] = vectorMagic
	out[1] = version
	binary.BigEndian.PutUint16(out[2:4], flags)
	out[4] = typeCode
	binary.BigEndian.PutUint32(out[5:9], dimensions)
	// TTC clients reserve this FLOAT64-sized field. For numeric vectors,
	// NORM|NORMSRC asks the database to compute the norm; BINARY reserves the
	// field with NORMSRC because a BINARY vector has no norm. A zero value is
	// therefore intentional, not an encoded norm calculated by this driver.
	clear(out[vectorBaseHeaderBytes:vectorHeaderBytes])
}

/*
DecodeVector validates and decodes one complete Oracle VECTOR envelope.

Input:
  - data: The raw VECTOR bytes. A nil slice represents SQL NULL. An empty,
    non-nil slice is invalid because an empty vector still has its header.

Output:
  - []float64, []float32, []int8, or packed []byte according to the type code.
  - An error when the envelope, flags, dimensions, or payload are unsupported.
*/
func DecodeVector(data driverCommon.B1Array) (driver.Value, error) {
	if data == nil {
		return nil, nil
	}
	if len(data) < vectorBaseHeaderBytes {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidLength, ">="+strconv.Itoa(vectorBaseHeaderBytes))
	}
	if data[0] != vectorMagic {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "magic=0xDB")
	}

	version := data[1]
	flags := binary.BigEndian.Uint16(data[2:4])
	typeCode := data[4]
	dimensionCount := binary.BigEndian.Uint32(data[5:9])

	if flags&vectorFlagOptional != 0 {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "optional-flags-unsupported")
	}
	if flags&vectorFlagSparse != 0 {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "sparse-unsupported")
	}
	if version != vectorVersionV0 {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidValue, "version=0")
	}

	valueBytes, err := vectorValueByteLength(typeCode, dimensionCount)
	if err != nil {
		return nil, err
	}
	payloadOffset := vectorBaseHeaderBytes
	if flags&(vectorFlagNorm|vectorFlagNormSource) != 0 {
		// The norm field is present whenever either flag is set. Accepting both
		// forms keeps decoding compatible with clients that omit norm metadata.
		payloadOffset += vectorNormBytes
	}
	expected := payloadOffset + valueBytes
	if len(data) != expected {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidLength, "=="+strconv.Itoa(expected))
	}

	payload := data[payloadOffset:]
	switch typeCode {
	case vectorTypeFloat64:
		return decodeVectorFloat64(payload, dimensionCount, flags)
	case vectorTypeFloat32:
		return decodeVectorFloat32(payload, dimensionCount, flags)
	case vectorTypeInt8:
		return decodeVectorInt8(payload, dimensionCount), nil
	case vectorTypeBinary:
		return append([]byte(nil), payload...), nil
	}
	return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "type=2|3|4|5")
}

/*
vectorValueByteLength returns the payload length implied by a VECTOR type and dimension count.

Input:
  - typeCode: The VECTOR element type code.
  - dimensions: The declared dense dimension count.

Output:
  - The payload size in bytes, or an error for an unsupported type or invalid count.
*/
func vectorValueByteLength(typeCode byte, dimensions uint32) (int, error) {
	maxDimensions := uint32(maxVectorDimensions)
	switch typeCode {
	case vectorTypeFloat64:
		if dimensions <= maxDimensions {
			return int(dimensions) * vectorFloat64Bytes, nil
		}
	case vectorTypeFloat32:
		if dimensions <= maxDimensions {
			return int(dimensions) * vectorFloat32Bytes, nil
		}
	case vectorTypeInt8:
		if dimensions <= maxDimensions {
			return int(dimensions), nil
		}
	case vectorTypeBinary:
		maxDimensions = maxVectorBinaryDimensions
		if dimensions > maxDimensions {
			break
		}
		if dimensions%vectorBitsPerByte != 0 {
			return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidValue, "binary-dimension-count-multiple-of-8")
		}
		return int(dimensions / vectorBitsPerByte), nil
	default:
		return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "unknown-type")
	}
	return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonOutOfRange, "dimension-count")
}

/*
decodeVectorFloat64 decodes a FLOAT64 payload.

Input:
  - payload: Exactly dimensions * vectorFloat64Bytes encoded bytes.
  - dimensions: Number of FLOAT64 values in payload.
  - flags: VECTOR byte-order and IEEE representation flags.

Output:
  - The decoded values or an error when an individual dimension is malformed.
*/
func decodeVectorFloat64(payload []byte, dimensions uint32, flags uint16) ([]float64, error) {
	out := make([]float64, dimensions)
	for i := range out {
		start := i * vectorFloat64Bytes
		value, err := decodeFloat64Dimension(payload[start:start+vectorFloat64Bytes], flags)
		if err != nil {
			return nil, err
		}
		out[i] = value
	}
	return out, nil
}

/*
decodeVectorFloat32 decodes a FLOAT32 payload.

Input:
  - payload: Exactly dimensions * vectorFloat32Bytes encoded bytes.
  - dimensions: Number of FLOAT32 values in payload.
  - flags: VECTOR byte-order and IEEE representation flags.

Output:
  - The decoded values or an error when an individual dimension is malformed.
*/
func decodeVectorFloat32(payload []byte, dimensions uint32, flags uint16) ([]float32, error) {
	out := make([]float32, dimensions)
	for i := range out {
		start := i * vectorFloat32Bytes
		value, err := decodeFloat32Dimension(payload[start:start+vectorFloat32Bytes], flags)
		if err != nil {
			return nil, err
		}
		out[i] = value
	}
	return out, nil
}

/*
decodeVectorInt8 decodes an INT8 payload.

Input:
  - payload: One signed byte per declared dimension.
  - dimensions: Number of INT8 values in payload.

Output:
  - The decoded values.
*/
func decodeVectorInt8(payload []byte, dimensions uint32) []int8 {
	out := make([]int8, dimensions)
	for i := range out {
		out[i] = int8(payload[i])
	}
	return out
}

/*
decodeFloat64Dimension decodes one FLOAT64 dimension.

Input:
  - raw: Exactly vectorFloat64Bytes bytes.
  - flags: VECTOR byte-order and IEEE representation flags.

Output:
  - The decoded value or an error when raw has an invalid width.
*/
func decodeFloat64Dimension(raw []byte, flags uint16) (float64, error) {
	if len(raw) != 8 {
		return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidLength, 8)
	}
	ordered := orderVectorBytes(raw, flags)
	if (flags & vectorFlagIEEE) != 0 {
		return math.Float64frombits(binary.BigEndian.Uint64(ordered)), nil
	}
	return DecodeBinaryDouble(ordered)
}

/*
decodeFloat32Dimension decodes one FLOAT32 dimension.

Input:
  - raw: Exactly vectorFloat32Bytes bytes.
  - flags: VECTOR byte-order and IEEE representation flags.

Output:
  - The decoded value or an error when raw has an invalid width.
*/
func decodeFloat32Dimension(raw []byte, flags uint16) (float32, error) {
	if len(raw) != 4 {
		return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidLength, 4)
	}
	ordered := orderVectorBytes(raw, flags)
	if (flags & vectorFlagIEEE) != 0 {
		return math.Float32frombits(binary.BigEndian.Uint32(ordered)), nil
	}
	return DecodeBinaryFloat(ordered)
}

/*
orderVectorBytes normalizes one VECTOR dimension to big-endian byte order.

Input:
  - raw: Encoded bytes for one numeric dimension.
  - flags: VECTOR byte-order flags.

Output:
  - raw unchanged for big-endian vectors, or a reversed copy for little-endian vectors.
*/
func orderVectorBytes(raw []byte, flags uint16) []byte {
	ordered := raw
	if (flags & vectorFlagLittle) != 0 {
		ordered = make([]byte, len(raw))
		for i := range raw {
			ordered[len(raw)-1-i] = raw[i]
		}
	}
	return ordered
}

/*
putOracleBinaryDouble writes FLOAT64 in Oracle's ordered binary representation.

Input:
  - dst: Storage for vectorFloat64Bytes bytes.
  - value: Go FLOAT64 value to encode.
*/
func putOracleBinaryDouble(dst []byte, value float64) {
	bits := math.Float64bits(value)
	var stored uint64
	if math.Signbit(value) {
		stored = ^bits
	} else {
		stored = bits ^ 0x8000000000000000
	}
	binary.BigEndian.PutUint64(dst, stored)
}

/*
putOracleBinaryFloat writes FLOAT32 in Oracle's ordered binary representation.

Input:
  - dst: Storage for vectorFloat32Bytes bytes.
  - value: Go FLOAT32 value to encode.
*/
func putOracleBinaryFloat(dst []byte, value float32) {
	bits := math.Float32bits(value)
	var stored uint32
	if math.Signbit(float64(value)) {
		stored = ^bits
	} else {
		stored = bits ^ 0x80000000
	}
	binary.BigEndian.PutUint32(dst, stored)
}
