package converters

import (
	"database/sql/driver"
	"encoding/binary"
	"math"
	"strconv"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	vectorTypeName            = "VECTOR"
	maxVectorDimensions       = 65535
	maxVectorBinaryDimensions = 65528

	vectorMagic           = 0xDB
	vectorVersionV0       = 0
	vectorBaseHeaderBytes = 9
	vectorNormBytes       = 8
	vectorHeaderBytes     = vectorBaseHeaderBytes + vectorNormBytes

	vectorFlagOptional   = 0x8000
	vectorFlagLittle     = 0x0001
	vectorFlagNorm       = 0x0002
	vectorFlagIEEE       = 0x0008
	vectorFlagNormSource = 0x0010
	vectorFlagSparse     = 0x0020

	vectorTypeFloat32 = 0x02
	vectorTypeFloat64 = 0x03
	vectorTypeInt8    = 0x04
	vectorTypeBinary  = 0x05
)

// EncodeVectorFloat64 encodes common.VectorFloat64 into Oracle VECTOR(FLOAT64) wire layout.
func EncodeVectorFloat64(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(driverCommon.VectorFloat64)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "common.VectorFloat64")
	}
	if values == nil {
		return nil, nil
	}
	return encodeVectorFloat64([]float64(values))
}

// EncodeVectorFloat32 encodes common.VectorFloat32 into Oracle VECTOR(FLOAT32) wire layout.
func EncodeVectorFloat32(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(driverCommon.VectorFloat32)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "common.VectorFloat32")
	}
	if values == nil {
		return nil, nil
	}
	return encodeVectorFloat32([]float32(values))
}

// EncodeVectorInt8 encodes common.VectorInt8 into Oracle VECTOR(INT8) wire layout.
func EncodeVectorInt8(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(driverCommon.VectorInt8)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "common.VectorInt8")
	}
	if values == nil {
		return nil, nil
	}
	return encodeVectorInt8([]int8(values))
}

// EncodeVectorBinary encodes packed bits into Oracle VECTOR(BINARY) wire layout.
func EncodeVectorBinary(v driver.Value) (driverCommon.B1Array, error) {
	values, ok := v.(driverCommon.VectorBinary)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonInvalidValue, "common.VectorBinary")
	}
	if values == nil {
		return nil, nil
	}
	return encodeVectorBinary(values)
}

// DecodeVector decodes Oracle VECTOR data into the matching Go slice type:
// []float64, []float32, []int8, or []byte (packed bits for BINARY vectors).
func DecodeVector(data driverCommon.B1Array) (driver.Value, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return decodeVector(data)
}

func encodeVectorFloat64(values []float64) (driverCommon.B1Array, error) {
	if len(values) > maxVectorDimensions {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65535")
	}
	payloadBytes := len(values) * 8
	out := make([]byte, vectorHeaderBytes+payloadBytes)
	writeVectorHeader(out, vectorVersionV0, vectorFlagNorm|vectorFlagNormSource, vectorTypeFloat64, uint32(len(values)))
	offset := vectorHeaderBytes
	for _, value := range values {
		putOracleBinaryDouble(out[offset:offset+8], value)
		offset += 8
	}
	return out, nil
}

func encodeVectorFloat32(values []float32) (driverCommon.B1Array, error) {
	if len(values) > maxVectorDimensions {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65535")
	}
	payloadBytes := len(values) * 4
	out := make([]byte, vectorHeaderBytes+payloadBytes)
	writeVectorHeader(out, vectorVersionV0, vectorFlagNorm|vectorFlagNormSource, vectorTypeFloat32, uint32(len(values)))
	offset := vectorHeaderBytes
	for _, value := range values {
		putOracleBinaryFloat(out[offset:offset+4], value)
		offset += 4
	}
	return out, nil
}

func encodeVectorInt8(values []int8) (driverCommon.B1Array, error) {
	if len(values) > maxVectorDimensions {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65535")
	}
	out := make([]byte, vectorHeaderBytes+len(values))
	writeVectorHeader(out, vectorVersionV0, vectorFlagNorm|vectorFlagNormSource, vectorTypeInt8, uint32(len(values)))
	for i := range values {
		out[vectorHeaderBytes+i] = byte(values[i])
	}
	return out, nil
}

func encodeVectorBinary(values driverCommon.VectorBinary) (driverCommon.B1Array, error) {
	if len(values) > maxVectorBinaryDimensions/8 {
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Encode", driverCommon.ReasonOutOfRange, "dimensions<=65528")
	}
	out := make([]byte, vectorHeaderBytes+len(values))
	writeVectorHeader(out, vectorVersionV0, vectorFlagNormSource, vectorTypeBinary, uint32(len(values))*8)
	copy(out[vectorHeaderBytes:], values)
	return out, nil
}

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

func decodeVector(data []byte) (driver.Value, error) {
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
		out := make([]float64, dimensionCount)
		for i := range out {
			start := i * 8
			value, err := decodeFloat64Dimension(payload[start:start+8], flags)
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	case vectorTypeFloat32:
		out := make([]float32, dimensionCount)
		for i := range out {
			start := i * 4
			value, err := decodeFloat32Dimension(payload[start:start+4], flags)
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	case vectorTypeInt8:
		out := make([]int8, dimensionCount)
		for i := range out {
			out[i] = int8(payload[i])
		}
		return out, nil
	case vectorTypeBinary:
		out := make([]byte, len(payload))
		copy(out, payload)
		return out, nil
	default:
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "type=2|3|4|5")
	}
}

func vectorValueByteLength(typeCode byte, dimensions uint32) (int, error) {
	switch typeCode {
	case vectorTypeFloat64:
		if dimensions > maxVectorDimensions {
			return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonOutOfRange, "dimension-count")
		}
		return int(dimensions) * 8, nil
	case vectorTypeFloat32:
		if dimensions > maxVectorDimensions {
			return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonOutOfRange, "dimension-count")
		}
		return int(dimensions) * 4, nil
	case vectorTypeInt8:
		if dimensions > maxVectorDimensions {
			return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonOutOfRange, "dimension-count")
		}
		return int(dimensions), nil
	case vectorTypeBinary:
		if dimensions > maxVectorBinaryDimensions {
			return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonOutOfRange, "dimension-count")
		}
		if dimensions%8 != 0 {
			return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidValue, "binary-dimension-count-multiple-of-8")
		}
		return int(dimensions / 8), nil
	default:
		return 0, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, vectorTypeName, "Decode", driverCommon.ReasonInvalidFormat, "unknown-type")
	}
}

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
