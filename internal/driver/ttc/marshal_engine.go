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
	"context"
	"encoding/binary"
	"math"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// unmarshalTypes lists the concrete TTC numeric wrapper types supported by the
// generic marshal and unmarshal helpers in this file.
type unmarshalTypes interface {
	driverCommon.UB1 | driverCommon.UB2 | driverCommon.UB4 | driverCommon.UB8 | driverCommon.SB1 | driverCommon.SB2 | driverCommon.SB4
}

// numericTypeRepresentation describes how a TTC numeric type is encoded and
// bounds.
type numericTypeRepresentation struct {
	// typeName is the human-readable type label used in error reporting.
	typeName string
	// typeRep is the TTC representation mode, such as Native or Universal.
	typeRep byte
	// isUnsigned indicates the type is unsigned.
	isUnsigned bool
	// nbBytes is the maximum byte width of the numeric type.
	nbBytes uint8
	// minValue is the smallest allowed value for the type.
	minValue int64
	// maxValue is the largest allowed value for the type.
	maxValue uint64
}

// MarshalEngine is responsible for marshaling and unmarshaling data between
// Go structures and the Oracle wire protocol.
// It provides methods for encoding and decoding various data types according to
// the Oracle TTC protocol.
//
// The MarshalEngine is configured with a DataBuffer for storing marshaled data,
// a ByteOrder for determining the byte order of the data, and a TypeRep for
// type information used during marshaling and unmarshaling.
type MarshalEngine struct {
	// PTR type representation
	_PTRTypeRep byte
	// numeric type representation
	_numericTypeRep [7]numericTypeRepresentation
	// data buffer
	_dataBuffer driverCommon.DataBuffer
	// byte order
	_byteOrder driverCommon.ByteOrder
	// sequence number maintained by its marshaller.
	// a sequence number is value from 1 to max int8 encoded on a UB1.
	// Each time it is fetched the value is incremented by 1.
	// When the value reaches the maximum it is reseted to 1
	_sequenceNumber int8
	// default LOB prefetch size configured for this connection, in bytes.
	_defaultLobPrefetchSize int64
}

const (
	// maximum short value length
	_maximumShortValueLength byte = 0xFC
	// Escape char. Followed by one or more ub1's
	_espapeValue byte = 0xFD
	// indicates that we are using long chunks
	_longLengthIndicator byte = 0xFE
	// null value indicator
	_nullLengthIndicator byte = 0xFF

	// Chunk size
	_checkSize = 32767

	// maximum accepted long-CLR chunk size
	_maximumLongCLRChunkLength = _checkSize * _longCLRAggregateLengthMultiplier

	// expected aggregate long-CLR size for scalar values before extra headroom is applied.
	_maximumScalarLongCLRLength = 32 * 1024 * 1024

	// multiplier used to leave additional permissible space for long-CLR aggregate limits.
	_longCLRAggregateLengthMultiplier = 2

	// maximum accepted aggregate long-CLR size for scalar values.
	_maximumLongCLRAggregateLength = _maximumScalarLongCLRLength * _longCLRAggregateLengthMultiplier

	// UNIVERSAL negative number flag
	_negativeValueIndicator byte = 0x80
)

// Pointer encodings used by TTC native and universal representations.
var (
	_defaultPTRTypeRep = Native
	_nullPTR           = driverCommon.B1Array{0x00, 0x00, 0x00, 0x00}
	_notNullPTR        = driverCommon.B1Array{0x7F, 0x7F, 0x7F, 0x7F}
)

const (
	UB1Index uint8 = iota
	UB2Index
	UB4Index
	UB8Index
	SB1Index
	SB2Index
	SB4Index
)

var (
	defaultNumericTypeRep = [7]numericTypeRepresentation{
		{
			typeName:   "UB1",
			typeRep:    Native,
			isUnsigned: true,
			nbBytes:    1,
			minValue:   0,
			maxValue:   math.MaxUint8,
		},
		{
			typeName:   "UB2",
			typeRep:    Native,
			isUnsigned: true,
			nbBytes:    2,
			minValue:   0,
			maxValue:   math.MaxUint16,
		},
		{
			typeName:   "UB4",
			typeRep:    Native,
			isUnsigned: true,
			nbBytes:    4,
			minValue:   0,
			maxValue:   math.MaxUint32,
		},
		{
			typeName:   "UB8",
			typeRep:    Native,
			isUnsigned: true,
			nbBytes:    8,
			minValue:   0,
			maxValue:   math.MaxUint64,
		},
		{
			typeName:   "SB1",
			typeRep:    Native,
			isUnsigned: false,
			nbBytes:    1,
			minValue:   math.MinInt8,
			maxValue:   math.MaxInt8,
		},
		{
			typeName:   "SB2",
			typeRep:    Native,
			isUnsigned: false,
			nbBytes:    2,
			minValue:   math.MinInt16,
			maxValue:   math.MaxInt16,
		},
		{
			typeName:   "SB4",
			typeRep:    Native,
			isUnsigned: false,
			nbBytes:    4,
			minValue:   math.MinInt32,
			maxValue:   math.MaxInt32,
		},
	}
)

// NewNativeMarshalEngine returns a MarshalEngine configured to use native
// representations for all numeric TTC types.
//
// Parameters:
//   - dataBuffer: The DataBuffer to use for marshaling and unmarshaling.
//   - byteOrder: The byte order to use for native encoding.
//
// Returns:
//   - A pointer to a new MarshalEngine instance.
func NewNativeMarshalEngine(dataBuffer driverCommon.DataBuffer, byteOrder driverCommon.ByteOrder) *MarshalEngine {
	engine := MarshalEngine{
		_PTRTypeRep:     _defaultPTRTypeRep,
		_dataBuffer:     dataBuffer,
		_byteOrder:      byteOrder,
		_sequenceNumber: 1,
	}
	copy(engine._numericTypeRep[:], defaultNumericTypeRep[:])
	return &engine
}

// NewMarshalEngine returns a new instance of MarshalEngine.
//
// Parameters:
//   - dataBuffer: The DataBuffer to use for marshaling and unmarshaling.
//   - byteOrder: The byte order to use for marshaling and unmarshaling.
//   - types: The TypeRep to use for type information.
//
// Returns:
//   - A pointer to a new MarshalEngine instance.
func NewMarshalEngine(dataBuffer driverCommon.DataBuffer, byteOrder driverCommon.ByteOrder, types [5]byte) *MarshalEngine {
	engine := MarshalEngine{
		_PTRTypeRep:     types[PTR],
		_dataBuffer:     dataBuffer,
		_byteOrder:      byteOrder,
		_sequenceNumber: 1,
	}
	copy(engine._numericTypeRep[:], defaultNumericTypeRep[:])
	engine._numericTypeRep[UB1Index].typeRep = types[B1]
	engine._numericTypeRep[UB2Index].typeRep = types[B2]
	engine._numericTypeRep[UB4Index].typeRep = types[B4]
	engine._numericTypeRep[UB8Index].typeRep = types[B8]
	engine._numericTypeRep[SB1Index].typeRep = types[B1]
	engine._numericTypeRep[SB2Index].typeRep = types[B2]
	engine._numericTypeRep[SB4Index].typeRep = types[B4]

	return &engine
}

// setDefaultLobPrefetchSize stores the connection-level default LOB prefetch
// size, in bytes.
//
// The value is expected to come from the oracle.go.default_lob_prefetch_size
// connection property after parsing as int64. The marshal engine uses it when
// validating aggregate long-CLR payloads for prefetched LOB column data.
func (m *MarshalEngine) setDefaultLobPrefetchSize(prefetchSize int64) {
	m._defaultLobPrefetchSize = prefetchSize
}

// marshalSeqNo marshals the next sequence number from the engine's internal
// counter and returns the value written to the buffer.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//
// Returns:
//   - The sequence number written to the buffer.
//   - An error if marshaling fails.
func (m *MarshalEngine) marshalSeqNo(ctx context.Context) (driverCommon.UB1, error) {
	if m._sequenceNumber <= 0 {
		// max int8 value has rotated the counter
		// put it back to 1
		m._sequenceNumber = 1
	}
	err := m.MarshalUB1(ctx, driverCommon.UB1(m._sequenceNumber))
	ret := m._sequenceNumber
	m._sequenceNumber++
	return driverCommon.UB1(ret), err
}

// marshalTokenNo marshals the function token number to the underlying buffer.
//
// The TTC protocol currently uses zero for this field, so this helper keeps the
// write path consistent with marshalSeqNo while returning the fixed value it
// emitted.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//
// Returns:
//   - The token number written to the buffer.
//   - An error if marshaling fails.
func (m *MarshalEngine) marshalTokenNo(ctx context.Context) (driverCommon.UB1, error) {
	err := m.MarshalUB8(ctx, 0)
	return 0, err
}

// MarshalUB1 marshals a UB1 value into the data buffer.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The UB1 value to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalUB1(ctx context.Context, value driverCommon.UB1) error {
	err := m._dataBuffer.WriteByteWithContext(ctx, byte(value))
	return _wrapError(err, m._numericTypeRep[UB1Index].typeName)
}

// MarshalUB2 marshals a UB2 value into the data buffer, using either native or
// universal encoding based on TypeRep.
// In native encoding all bytes will be written in the order based on byteOrder
// and in universal mode the first byte contains the number of bytes written,
// and the remaining bytes contain the the value encoded in big endian encoding.
// Leading bytes of value zero are not encoded. If the value is zero, a single
// byte of value zero is written.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The UB2 value to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalUB2(ctx context.Context, value driverCommon.UB2) error {
	if m._numericTypeRep[UB2Index].typeRep != Universal {
		return _writeNative(ctx, value, m, m._numericTypeRep[UB2Index],
			func(byteArray []byte, value driverCommon.UB2) {
				binary.LittleEndian.PutUint16(byteArray, uint16(value))
			},
			func(byteArray []byte, value driverCommon.UB2) {
				binary.BigEndian.PutUint16(byteArray, uint16(value))
			})
	}

	return _wrapError(_writeUniversal(ctx, uint64(value), false, m._dataBuffer), m._numericTypeRep[UB2Index].typeName)
}

// MarshalUB4 marshals a UB4 value into the data buffer, using either native or
// universal encoding based on TypeRep.
//
// In native encoding all bytes are written using the configured byte order. In
// universal encoding the first byte stores the number of encoded value bytes
// and the remaining bytes store the value in big-endian order without leading
// zero bytes. Zero is encoded as a single zero byte.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The UB4 value to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalUB4(ctx context.Context, value driverCommon.UB4) error {
	if m._numericTypeRep[UB4Index].typeRep != Universal {
		return _writeNative(ctx, value, m, m._numericTypeRep[UB4Index],
			func(byteArray []byte, value driverCommon.UB4) {
				binary.LittleEndian.PutUint32(byteArray, uint32(value))
			},
			func(byteArray []byte, value driverCommon.UB4) {
				binary.BigEndian.PutUint32(byteArray, uint32(value))
			})
	}
	return _wrapError(_writeUniversal(ctx, uint64(value), false, m._dataBuffer), m._numericTypeRep[UB4Index].typeName)
}

// MarshalUB8 marshals a UB8 value into the data buffer, using either native or
// universal encoding based on TypeRep.
//
// In native encoding all bytes are written using the configured byte order. In
// universal encoding the first byte stores the number of encoded value bytes
// and the remaining bytes store the value in big-endian order without leading
// zero bytes. Zero is encoded as a single zero byte.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The UB8 value to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalUB8(ctx context.Context, value driverCommon.UB8) error {
	if m._numericTypeRep[UB8Index].typeRep != Universal {
		return _writeNative(ctx, value, m, m._numericTypeRep[UB8Index],
			func(byteArray []byte, value driverCommon.UB8) {
				binary.LittleEndian.PutUint64(byteArray, uint64(value))
			},
			func(byteArray []byte, value driverCommon.UB8) {
				binary.BigEndian.PutUint64(byteArray, uint64(value))
			})
	}
	return _wrapError(_writeUniversal(ctx, uint64(value), false, m._dataBuffer), m._numericTypeRep[UB8Index].typeName)
}

// MarshalSB4 marshals a SB4 value into the data buffer, using either native or
// universal encoding based on TypeRep.
//
// In native encoding all bytes are written using the configured byte order. In
// universal encoding the first byte stores the number of encoded value bytes
// together with the sign flag, and the remaining bytes store the absolute
// value in big-endian order without leading zero bytes. Zero is encoded as a
// single zero byte.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The SB4 value to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalSB4(ctx context.Context, value driverCommon.SB4) error {
	if m._numericTypeRep[SB4Index].typeRep != Universal {
		return _writeNative(ctx, value, m, m._numericTypeRep[SB4Index],
			func(byteArray []byte, value driverCommon.SB4) {
				binary.LittleEndian.PutUint32(byteArray, uint32(value))
			},
			func(byteArray []byte, value driverCommon.SB4) {
				binary.BigEndian.PutUint32(byteArray, uint32(value))
			})
	}
	absoluteValue, isNegative := _getAbsoluteValue(int64(value))
	return _wrapError(_writeUniversal(ctx, absoluteValue, isNegative, m._dataBuffer), m._numericTypeRep[SB4Index].typeName)
}

// MarshalB1Array marshals a B1Array into the data buffer.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The B1Array to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalB1Array(ctx context.Context, value driverCommon.B1Array) error {
	err := m._dataBuffer.WriteBytesWithContext(ctx, value)
	return _wrapError(err, "B1Array")
}

// MarshalPTR marshals a non-null PTR value into the data buffer.
//
// PTR uses `_notNullPTR` in native encoding and a single byte of value `0x01`
// in universal encoding.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalPTR(ctx context.Context) error {
	if m._PTRTypeRep != Universal {
		err := m.MarshalB1Array(ctx, _notNullPTR)
		return _wrapError(err, "PTR")
	}
	err := m.MarshalUB1(ctx, 0x01)
	return _wrapError(err, "PTR")
}

// MarshalNullPTR marshals a null PTR value into the data buffer.
//
// PTR uses `_nullPTR` in native encoding and a single byte of value `0x00` in
// universal encoding.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalNullPTR(ctx context.Context) error {
	if m._PTRTypeRep != Universal {
		err := m.MarshalB1Array(ctx, _nullPTR)
		return _wrapError(err, "null PTR")
	}
	err := m.MarshalUB1(ctx, 0x00)
	return _wrapError(err, "null PTR")
}

// MarshalChar marshals a Char value into the data buffer. Since client-side
// character-set conversion is not implemented, this writes the supplied bytes
// directly or via CLR encoding depending on the type-representation flags.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The Char value to be marshaled.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalChar(ctx context.Context, value driverCommon.B1Array) error {
	if len(value) > 0 {
		if typeRepresentationTable.getFlags() == TTCLXMULTI {
			err := m._dataBuffer.WriteBytesWithContext(ctx, value)
			return _wrapError(err, "Char")
		} else {
			return m.MarshalCLR(ctx, value, 0, len(value))
		}
	}
	return nil
}

// MarshalCLR marshals a CLR value into the data buffer, handling both short and
// long forms.
//
// A CLR is encoded as follows:
//   - if the total length is smaller or equal to _maximumShortValueLength then
//     the length will be written as a UB1 and the following bytes will contain
//     the data
//   - If the total length is larger than _maximumShortValueLength
//   - a first byte containing _longLengthIndicator will be written to indicate
//     that the data will be written in large chunks
//   - The data will be written in chuncks not more than _chunkSize length
//   - first a SB4 will be written containing the chunk length
//   - the SB4 will be followed by check length bytes of data
//   - A chunk size ZERO will be written to indicate the end of the data
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The CLR value to be marshaled.
//   - offset: The offset into the value.
//   - totalLength: The total number of bytes to encode.
//
// Returns:
//   - An error if the marshaling operation fails.
func (m *MarshalEngine) MarshalCLR(ctx context.Context, value driverCommon.B1Array, offset, totalLength int) error {
	if totalLength > int(_maximumShortValueLength) {
		nbBytesWritten := 0

		// Write escape byte
		if err := m._dataBuffer.WriteByteWithContext(ctx, _longLengthIndicator); err != nil {
			return _wrapError(err, "CLR")
		}

		for nbBytesWritten < totalLength {
			bytesLeft := totalLength - nbBytesWritten
			length := bytesLeft
			if length > _checkSize {
				length = _checkSize
			}

			if err := m.MarshalSB4(ctx, driverCommon.SB4(length)); err != nil {
				return _wrapError(err, "CLR")
			}

			// Write chunk bytes
			if err := m._dataBuffer.WriteBytesWithContext(ctx, (value)[offset+nbBytesWritten:offset+nbBytesWritten+length]); err != nil {
				return _wrapError(err, "CLR")
			}

			nbBytesWritten += length
		}

		// Write terminating zero length
		if err := m.MarshalSB4(ctx, 0); err != nil {
			return _wrapError(err, "CLR")
		}
	} else {
		err := m._dataBuffer.WriteByteWithContext(ctx, byte(totalLength&0xFF))
		if err != nil {
			return _wrapError(err, "CLR")
		}
		if len(value) != 0 {
			err := m._dataBuffer.WriteBytesWithContext(ctx, (value)[offset:offset+totalLength])
			if err != nil {
				return _wrapError(err, "CLR")
			}
		}
	}
	return nil
}

// UnmarshalUB1 unmarshals a UB1 value from the data buffer.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//
// Returns:
//   - The unmarshaled UB1 value.
//   - An error if the unmarshaling operation fails.
func (m *MarshalEngine) UnmarshalUB1(ctx context.Context) (driverCommon.UB1, error) {
	value, err := m._dataBuffer.ReadByteWithContext(ctx)
	return driverCommon.UB1(value), _wrapError(err, m._numericTypeRep[UB1Index].typeName)
}

// UnmarshalUB2 unmarshals a UB2 value from the data buffer, using either native
// or universal decoding based on TypeRep.
//
// In native encoding all bytes will be read in the order based on byteOrder
// and in universal mode the first byte contains the number of bytes to read,
// and the remaining bytes contain the the value encoded in big endian encoding.
// Leading bytes of value zero are not encoded, the value is prepended with
// zeros to fill the size of the expected value. If the value is zero, a single
// byte of value zero is written.
//
// Returns:
//   - The unmarshaled UB2 value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalUB2(ctx context.Context) (driverCommon.UB2, error) {
	return _unmarshalNumeric(ctx, m, m._numericTypeRep[UB2Index],
		func(b []byte) driverCommon.UB2 {
			return driverCommon.UB2(binary.BigEndian.Uint16(b))
		},
		func(b []byte) driverCommon.UB2 {
			return driverCommon.UB2(binary.LittleEndian.Uint16(b))
		},
		func(value driverCommon.UB2) driverCommon.UB2 {
			return value
		})
}

// UnmarshalUB4 unmarshals a UB4 value from the data buffer, using either native
// or universal decoding based on TypeRep.
//
// In native encoding all bytes will be read in the order based on byteOrder
// and in universal mode the first byte contains the number of bytes to read,
// and the remaining bytes contain the the value encoded in big endian encoding.
// Leading bytes of value zero are not encoded, the value is prepended with
// zeros to fill the size of the expected value. If the value is zero, a single
// byte of value zero is written.
//
// Returns:
//   - The unmarshaled UB4 value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalUB4(ctx context.Context) (driverCommon.UB4, error) {
	return _unmarshalNumeric(ctx, m, m._numericTypeRep[UB4Index],
		func(b []byte) driverCommon.UB4 {
			return driverCommon.UB4(binary.BigEndian.Uint32(b))
		},
		func(b []byte) driverCommon.UB4 {
			return driverCommon.UB4(binary.LittleEndian.Uint32(b))
		},
		func(value driverCommon.UB4) driverCommon.UB4 {
			return value
		})
}

// UnmarshalUB8 unmarshals a UB8 value from the data buffer, using either native
// or universal decoding based on TypeRep.
//
// In native encoding all bytes will be read in the order based on byteOrder
// and in universal mode the first byte contains the number of bytes to read,
// and the remaining bytes contain the the value encoded in big endian encoding.
// Leading bytes of value zero are not encoded, the value is prepended with
// zeros to fill the size of the expected value. If the value is zero, a single
// byte of value zero is written.
//
// Returns:
//   - The unmarshaled UB8 value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalUB8(ctx context.Context) (driverCommon.UB8, error) {
	return _unmarshalNumeric(ctx, m, m._numericTypeRep[UB8Index],
		func(b []byte) driverCommon.UB8 {
			return driverCommon.UB8(binary.BigEndian.Uint64(b))
		},
		func(b []byte) driverCommon.UB8 {
			return driverCommon.UB8(binary.LittleEndian.Uint64(b))
		},
		func(value driverCommon.UB8) driverCommon.UB8 {
			return value
		})
}

// UnmarshalSB1 unmarshals a SB1 value from the data buffer.
//
// The TTC Java implementation decodes SB1 by unmarshalling SB2 first and then
// casting to byte; this preserves the same wrapping semantics here.
//
// Returns:
//   - The unmarshaled SB1 value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalSB1(ctx context.Context) (driverCommon.SB1, error) {
	value, err := m.UnmarshalSB2(ctx)
	if err != nil {
		return 0, _wrapError(err, m._numericTypeRep[SB1Index].typeName)
	}
	return driverCommon.SB1(value), nil
}

// UnmarshalSB2 unmarshals a SB2 value from the data buffer, using either native
// or universal decoding based on TypeRep.
//
// In native encoding two bytes are read in the order based on byteOrder. In
// universal mode the value is decoded with the signed SB2 UNIVERSAL config and
// cast to SB2, avoiding the unsigned UB2 negative-flag validation.
//
// Returns:
//   - The unmarshaled SB2 value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalSB2(ctx context.Context) (driverCommon.SB2, error) {
	return _unmarshalNumeric(ctx, m, m._numericTypeRep[SB2Index],
		func(b []byte) driverCommon.UB2 {
			return driverCommon.UB2(binary.BigEndian.Uint16(b))
		},
		func(b []byte) driverCommon.UB2 {
			return driverCommon.UB2(binary.LittleEndian.Uint16(b))
		},
		func(value driverCommon.UB2) driverCommon.SB2 {
			return driverCommon.SB2(-1) * driverCommon.SB2(value)
		})
}

// UnmarshalSB4 unmarshals a SB4 value from the data buffer, using either native
// or universal decoding based on TypeRep.
//
// In native encoding all bytes will be read in the order based on byteOrder
// and in universal mode the first byte contains the number of bytes to read,
// and the remaining bytes contain the the value encoded in big endian encoding.
// Leading bytes of value zero are not encoded, the value is prepended with
// zeros to fill the size of the expected value. If the value is zero, a single
// byte of value zero is written.
//
// Returns:
//   - The unmarshaled SB4 value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalSB4(ctx context.Context) (driverCommon.SB4, error) {
	return _unmarshalNumeric(ctx, m, m._numericTypeRep[SB4Index],
		func(b []byte) driverCommon.UB4 {
			return driverCommon.UB4(binary.BigEndian.Uint32(b))
		},
		func(b []byte) driverCommon.UB4 {
			return driverCommon.UB4(binary.LittleEndian.Uint32(b))
		},
		func(value driverCommon.UB4) driverCommon.SB4 {
			return driverCommon.SB4(-1) * driverCommon.SB4(value)
		})
}

// UnmarshalB1Array unmarshals "length" bytes from the data buffer.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - length: The length of the array to unmarshal.
//
// Returns:
//   - A pointer to the unmarshaled B1Array.
//   - An error if the unmarshaling operation fails.
func (m *MarshalEngine) UnmarshalB1Array(ctx context.Context, length int) (driverCommon.B1Array, error) {
	readValues, err := m._dataBuffer.ReadBytesWithContext(ctx, int32(length))
	if err != nil {
		return nil, _wrapError(err, "B1Array")
	}
	value := make([]byte, length)
	copy(value, *readValues)
	return value, nil
}

// UnmarshalText unmarshal a text value from the data buffer. Reads bytes one
// by one and stops either when a byte of value 0x00 is reached or "length" bytes
// have been read
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - length: The maximum length of the text to unmarshal.
//
// Returns:
//   - A pointer to the unmarshaled B1Array.
//   - An error if the unmarshaling operation fails.
func (m *MarshalEngine) UnmarshalText(ctx context.Context, length int) (driverCommon.B1Array, error) {
	offset := 0
	tmpBuffer := make([]byte, length)
	for offset < length {
		b, err := m._dataBuffer.ReadByteWithContext(ctx)
		if err != nil {
			return nil, _wrapError(err, "Text")
		}
		if b == 0 {
			break
		}
		tmpBuffer[offset] = b
		offset++
	}

	buffer := tmpBuffer[:offset]
	return buffer, nil
}

// UnmarshalCLR unmarshals a CLR value from the data buffer, handling both short
// and long forms.
//
// A CLR is decoded as follows:
//   - The first byte is read; it can contain either the length of the data or
//     a flag.
//   - If the first byte is an escape value, a protocol violation error is
//     returned.
//   - If the first byte is a null-length indicator, no bytes are returned.
//   - If the first byte is less than or equal to
//     _maximumShortValueLength, it indicates the data size and the data will
//     be read and returned.
//   - Otherwise the long-chunk protocol is used.
//   - The chunk length is read as an SB4.
//   - If the chunk length is zero, decoding stops.
//   - Otherwise the chunk data is read into the destination buffer.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - bytes: The B1Array to store the unmarshaled value.
//   - maxSize: The maximum size of the unmarshaled value.
//
// Returns:
//   - The length of the unmarshaled value.
//   - An error if the unmarshaling operation fails.
func (m *MarshalEngine) UnmarshalCLR(ctx context.Context, bytes driverCommon.B1Array, maxSize int) (int, error) {

	// first byte is either length or marker (0xFE)
	lenflag, err := m.UnmarshalUB1(ctx)
	if err != nil {
		return 0, _wrapError(err, "CLR")
	}

	if byte(lenflag) == _espapeValue {
		return 0, _wrapError(common.NewOracleError(oracleErrors.ProtocolViolation, nil, nil), "CLR")
	}
	if byte(lenflag) == _nullLengthIndicator {
		return 0, nil
	}

	if byte(lenflag) <= _maximumShortValueLength { // Smaller than MAX length
		err := m._unmarshalBuffer(ctx, bytes, 0, int(lenflag))
		return int(lenflag), _wrapError(err, "CLR")
	}
	// Got FE marker, long chunks
	nbBytesWritten := 0
	for {
		length, err := m.UnmarshalSB4(ctx)
		if err != nil {
			return 0, _wrapError(err, "CLR")
		}
		if length == 0 {
			break
		}
		if length < 0 {
			common.Odl.Debug("Invalid length value while unmarshalling CLR", "length", length)
			return -1, common.NewOracleError(oracleErrors.MarshalEngineError, nil, "CLR")
		}

		locallen := int(length)
		if locallen > 0 {
			keepThem := int(math.Min(float64(maxSize-nbBytesWritten), float64(locallen)))
			err = m._unmarshalBuffer(ctx, bytes, nbBytesWritten, keepThem)
			if err != nil {
				return 0, _wrapError(err, "CLR")
			}
			nbBytesWritten += keepThem

			// TODO check if it is normal to ignore the end of the field if the max
			// is lower than the total size
			rest := locallen - keepThem
			if rest > 0 {
				if _, err := m._dataBuffer.ReadBytesWithContext(ctx, int32(rest)); err != nil {
					return 0, _wrapError(err, "CLR")
				}
			}
		}
	}
	return nbBytesWritten, nil
}

// UnmarshalCLRColumnData unmarshals a CLR value from the data buffer, handling
// both short and long forms.
//
// A CLR is decoded as follows:
//   - The first byte is read; it can contain either the length of the data or
//     a flag.
//   - If the first byte is an escape value, a protocol violation error is
//     returned.
//   - If the first byte is a null-length indicator, no bytes are returned.
//   - If the first byte is less than or equal to
//     _maximumShortValueLength, it indicates the data size and the data will
//     be read and returned.
//   - Otherwise the long-chunk protocol is used.
//   - The chunk length is read as an SB4.
//   - If the chunk length is zero, decoding stops.
//   - Otherwise the chunk data is read and appended to the result slice.
//
// Returns:
//   - A B1Array containing the unmarshaled value.
//   - The length of the unmarshaled value.
//   - An error if the unmarshaling operation fails.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
func (m *MarshalEngine) UnmarshalCLRColumnData(ctx context.Context) (driverCommon.B1Array, int, error) {
	// Long CLR data should stay within _maximumLongCLRAggregateLength for scalar
	// data or the configured LOB prefetch allowance, including the shared
	// _longCLRAggregateLengthMultiplier headroom. Larger payloads suggest the
	// server is possibly compromised.
	maxAllowedCLRLength := math.Max(
		_maximumLongCLRAggregateLength,
		float64(m._defaultLobPrefetchSize)*_longCLRAggregateLengthMultiplier,
	)
	// first byte is either length or marker (0xFE)
	lenflag, err := m.UnmarshalUB1(ctx)
	if err != nil {
		return nil, 0, _wrapError(err, "CLR")
	}

	if byte(lenflag) == _espapeValue {
		return nil, 0, _wrapError(common.NewOracleError(oracleErrors.ProtocolViolation, nil, nil), "CLR")
	}
	if byte(lenflag) == _nullLengthIndicator {
		return nil, 0, nil
	}

	if byte(lenflag) <= _maximumShortValueLength { // Smaller than MAX length
		bytes := make(driverCommon.B1Array, lenflag)
		err := m._unmarshalBuffer(ctx, bytes, 0, int(lenflag))
		return bytes, int(lenflag), _wrapError(err, "CLR")
	}
	// Got FE marker, long chunks
	nbBytesWritten := 0
	var bytes driverCommon.B1Array
	for {
		length, err := m.UnmarshalSB4(ctx)
		if err != nil {
			return nil, 0, _wrapError(err, "CLR")
		}
		if length == 0 {
			break
		}
		if length < 0 {
			common.Odl.Debug("Invalid length value while unmarshalling CLR", "length", length)
			return nil, -1, _wrapError(common.NewOracleError(oracleErrors.ProtocolViolation, nil), "CLR")
		}
		if length > _maximumLongCLRChunkLength {
			common.Odl.Debug("Long CLR chunk length exceeds maximum allowed size",
				"length", length, "maxLength", _maximumLongCLRChunkLength)
			return nil, -1, _wrapError(common.NewOracleError(oracleErrors.ProtocolViolation, nil), "CLR")
		}

		currentBytes := make(driverCommon.B1Array, length)
		err = m._unmarshalBuffer(ctx, currentBytes, 0, int(length))
		if err != nil {
			return nil, 0, _wrapError(err, "CLR")
		}
		nbBytesWritten += int(length)

		if nbBytesWritten > int(maxAllowedCLRLength) {
			common.Odl.Debug("CLR data length exceeds maximum allowed size",
				"length", nbBytesWritten, "maxLength", _maximumLongCLRChunkLength)
			return nil, -1, _wrapError(common.NewOracleError(oracleErrors.ProtocolViolation, nil), "CLR")
		}
		bytes = append(bytes, currentBytes...)
	}
	return bytes, nbBytesWritten, nil
}

// Flush flushes the data buffer.
//
// Parameters:
//   - ctx: The request context used for the flush operation.
//
// Returns:
//   - An error if flushing fails.
func (m *MarshalEngine) Flush(ctx context.Context) error {
	err := m._dataBuffer.Flush(ctx)
	if err != nil {
		return common.NewOracleError(oracleErrors.MarshalEngineFlushError, err)
	}
	return nil
}

// _unmarshalBuffer reads `length` bytes from the data buffer and copies them
// into `byteValue` starting at `offset`.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - byteValue: The destination buffer.
//   - offset: The starting offset in the destination buffer.
//   - length: The number of bytes to copy.
//
// Returns:
//   - An error if the bytes cannot be copied into the destination buffer.
func (m *MarshalEngine) _unmarshalBuffer(ctx context.Context, byteValue []byte, offset int, length int) error {
	if length <= 0 {
		return nil
	}

	// Check that the buffer is big enough to receive the data
	if len(byteValue) < offset+length {
		common.Odl.Debug("Invalid buffer length", "buffer length", len(byteValue),
			"data length", length)
		return common.NewOracleError(oracleErrors.MarshalEngineError, nil, "CLR")
	}
	// Store len bytes of data into original buffer
	byteValueP := byteValue
	if err := m._readAndCopyToBuffer(ctx, new(byteValueP[offset:offset+length]), length); err != nil {
		return err
	}
	offset += length
	return nil
}

// _readAndCopyToBuffer reads `length` bytes from the data buffer and copies
// them into `buffer`.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - buffer: The destination buffer.
//   - length: The number of bytes to copy.
//
// Returns:
//   - An error if the bytes cannot be copied into the destination buffer.
func (m *MarshalEngine) _readAndCopyToBuffer(ctx context.Context, buffer *[]byte, length int) error {
	data, err := m._dataBuffer.ReadBytesWithContext(ctx, int32(length))
	if err != nil {
		return err
	}
	copy(*buffer, *data)
	return nil
}

// _writeUniversal writes a signed integer value using TTC universal encoding.
//
// The absolute value of the value will be encoded as an array of bytes using
// big endian byte order. All leading bytes equal to zero will be discarded.
// The first byte will contain the remaining number of bytes (after then leading
// zeros have been discarded) and a flag indicating whether the value is
// negative, the following bytes will contain the absolute value in big endian
// byte order (after the leading zeros have been discarded).
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - absoluteValue: The absolute value to encode.
//   - isNegative: Whether the encoded value should be marked negative.
//   - dataBuffer: The destination buffer.
//
// Returns:
//   - An error if the encoded bytes cannot be written to the data buffer.
func _writeUniversal(ctx context.Context, absoluteValue uint64, isNegative bool, dataBuffer driverCommon.DataBuffer) error {
	if absoluteValue == 0 {
		buffer := make([]byte, 1)
		buffer[0] = 0
		return dataBuffer.WriteBytesWithContext(ctx, buffer)
	}

	bytes := _encodeAbsoluteUniversalValue(absoluteValue, isNegative)
	return dataBuffer.WriteBytesWithContext(ctx, bytes)
}

// _getAbsoluteValue splits a signed integer into its absolute magnitude and a
// boolean flag indicating whether it was negative.
//
// Parameters:
//   - value: The signed integer to split.
//
// Returns:
//   - The absolute magnitude.
//   - Whether the original value was negative.
func _getAbsoluteValue(value int64) (uint64, bool) {
	if value < 0 {
		return uint64(-value), true
	}
	return uint64(value), false
}

// _encodeAbsoluteUniversalValue encodes an absolute integer magnitude as TTC
// universal bytes, optionally setting the negative-value flag in the first
// byte.
//
// Parameters:
//   - absoluteValue: The absolute value to encode.
//   - isNegative: Whether the encoded bytes should set the
//     `_negativeValueIndicator` flag.
//
// Returns:
//   - The TTC universal byte representation of the supplied magnitude.
func _encodeAbsoluteUniversalValue(absoluteValue uint64, isNegative bool) []byte {
	bigEndianValue := make([]byte, 0)
	bigEndianValue = binary.BigEndian.AppendUint64(bigEndianValue, absoluteValue)

	// count the number of zeros
	zeros := true
	numberOfZeros := 0
	for zeros {
		if bigEndianValue[numberOfZeros] == 0 {
			numberOfZeros++
		} else {
			zeros = false
		}
	}

	// calculate the size and allocate return value
	size := byte(len(bigEndianValue) - numberOfZeros)
	returnValue := make([]byte, 1+size)

	// set size and negative flag
	returnValue[0] = size
	if isNegative {
		returnValue[0] |= _negativeValueIndicator
	}

	// append bytes
	returnValue = append(returnValue[:1], bigEndianValue[numberOfZeros:]...)
	return returnValue
}

// _unmarshalNumeric unmarshals a numeric value using either native or
// universal TTC encoding, based on the supplied type metadata and conversion
// callbacks.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - marshalEngine: The engine holding the data buffer and byte order.
//   - numTypeRep: The metadata describing the numeric type.
//   - bigEndianFunc: Converter used for big-endian native reads and universal
//     magnitude parsing.
//   - littleEndianFunc: Converter used for little-endian native reads.
//   - toNegative: Converter used to turn a decoded magnitude into the signed
//     target type when UNIVERSAL encodes a negative value.
//
// Returns:
//   - The decoded numeric value.
//   - An error if the value cannot be decoded.
func _unmarshalNumeric[T unmarshalTypes, V unmarshalTypes](
	ctx context.Context,
	marshalEngine *MarshalEngine,
	numTypeRep numericTypeRepresentation,
	bigEndianFunc func([]byte) V,
	littleEndianFunc func([]byte) V,
	toNegative func(V) T) (T, error) {

	var value T
	var err error
	if numTypeRep.typeRep != Universal {
		value, err = _readNative[T, V](ctx, marshalEngine._dataBuffer, marshalEngine._byteOrder, numTypeRep.typeName, numTypeRep.nbBytes, littleEndianFunc, bigEndianFunc)
		if err != nil {
			return value, _wrapError(err, numTypeRep.typeName)
		}
		return value, nil
	}

	value, err = _decodeUniversal(ctx, marshalEngine, numTypeRep, bigEndianFunc, toNegative)
	if err != nil {
		return value, _wrapError(err, numTypeRep.typeName)
	}
	return value, nil
}

// _decodeUniversal decodes a TTC universal integer value from the buffer.
//
// The first byte stores the value byte count in the low seven bits and the
// negative flag in the high bit. A non-zero value must declare between one and
// the configured maximum value bytes. Unsigned types reject values that set the
// negative flag. The following bytes contain the magnitude in big endian
// encoding. The value bytes are left-padded with zeros to the declared type
// width before being converted with `bigEndianFunc`. If the negative flag is
// set, `toNegative` is used to convert the decoded magnitude to the target
// signed value.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - m: The marshal engine holding the data buffer.
//   - typeRep: The metadata describing the numeric type being decoded.
//   - bigEndianFunc: The decoder used to turn the padded bytes into a numeric
//     value.
//   - toNegative: The converter used when the UNIVERSAL sign bit is set.
//
// Returns:
//   - The decoded value in the requested target type.
//   - An error if the buffer cannot be read, the byte count is invalid, or an
//     unsigned type sets the negative flag.
func _decodeUniversal[T unmarshalTypes, V unmarshalTypes](
	ctx context.Context,
	m *MarshalEngine,
	typeRep numericTypeRepresentation,
	bigEndianFunc func([]byte) V, toNegative func(V) T) (T, error) {
	var zero T
	// first byte contains the length and the negative flag
	firstByte, err := m._dataBuffer.ReadByteWithContext(ctx)
	if err != nil {
		return zero, err
	}
	if firstByte == 0 {
		return zero, nil
	}

	numberOfBytes := firstByte & 0x7F // Remove sign bit
	negativeUNV := (firstByte & _negativeValueIndicator) == _negativeValueIndicator

	if err := _validateUniversalByteCount(firstByte, numberOfBytes, typeRep.nbBytes, typeRep.typeName); err != nil {
		return zero, err
	}
	if negativeUNV && typeRep.isUnsigned {
		common.Odl.Debug("UNIVERSAL unsigned value cannot be negative", "first byte", firstByte)
		return zero, common.NewOracleError(oracleErrors.MarshalEngineError, nil, typeRep.typeName)
	}

	// Read the bytes from the data buffer
	bytes, err := m._dataBuffer.ReadBytesWithContext(ctx, int32(numberOfBytes))
	if err != nil {
		return zero, err
	}

	var bytesValues = make([]byte, typeRep.nbBytes)
	copy(bytesValues[len(bytesValues)-int(numberOfBytes):], *bytes)

	// convert the byte array in big endian encoding to an uint64
	calculatedValue := bigEndianFunc(bytesValues[:])

	// Calculated value must be positive since the negative flag has already been read
	if calculatedValue < 0 {
		return zero, common.NewOracleError(oracleErrors.MarshalEngineError, nil, typeRep.typeName)
	}

	var finalValue T
	if negativeUNV {
		finalValue = toNegative(calculatedValue)
	} else {
		finalValue = T(calculatedValue)
	}
	// Negative flag should not be set if the value is unsigned
	// Calculated value should not be higher than maxValue
	// Value is not negative and final value is negative (positive overflow)
	if (negativeUNV && (typeRep.minValue >= 0 || finalValue > 0)) || finalValue > T(typeRep.maxValue) || (!negativeUNV && finalValue < 0) {
		return zero, common.NewOracleError(oracleErrors.MarshalEngineError, nil, typeRep.typeName)
	}

	return finalValue, nil
}

// _validateUniversalByteCount validates the value-byte count encoded in the
// UNIVERSAL first byte.
//
// Parameters:
//   - firstByte: The raw first UNIVERSAL byte read from the buffer.
//   - numberOfBytes: The decoded byte-count from the first byte.
//   - maxBytes: The maximum allowed value width for the target type.
//   - typeName: The type name used in error reporting.
//
// Returns:
//   - An error if the byte-count is zero or exceeds the target width.
func _validateUniversalByteCount(firstByte byte, numberOfBytes byte, maxBytes uint8, typeName string) error {
	// this should never be zero, we already checked that the first byte was not
	// zero, numberOfBytes being zero would mean that the value was negative zero
	// which is not possible.
	if numberOfBytes == 0 {
		common.Odl.Debug("Number of bytes zero and first byte different of zero",
			"first byte", firstByte, "number of bytes", numberOfBytes)
		return common.NewOracleError(oracleErrors.MarshalEngineError, nil, typeName)
	}
	if numberOfBytes > maxBytes {
		common.Odl.Debug("Number of bytes greater than UNIVERSAL target width",
			"first byte", firstByte, "number of bytes", numberOfBytes, "max bytes", maxBytes)
		return common.NewOracleError(oracleErrors.MarshalEngineError, nil, typeName)
	}

	return nil
}

// _writeNative writes a numeric value using the engine's configured native
// byte order and type metadata.
//
// Parameters:
//   - ctx: The request context used for buffer writes.
//   - value: The numeric value to write.
//   - m: The marshal engine holding the byte order and buffer.
//   - numTypeRep: The metadata describing the numeric type.
//   - littleEndianFunc: The encoder to use when the engine is configured for
//     little-endian writes.
//   - bigEndianFunc: The encoder to use when the engine is configured for
//     big-endian writes.
//
// Returns:
//   - An error if the value cannot be written.
func _writeNative[T unmarshalTypes](ctx context.Context, value T, m *MarshalEngine, numTypeRep numericTypeRepresentation, littleEndianFunc func([]byte, T), bigEndianFunc func([]byte, T)) error {
	buffer := make([]byte, numTypeRep.nbBytes)
	if m._byteOrder == driverCommon.LITTLE_ENDIAN {
		littleEndianFunc(buffer, value)
	} else {
		bigEndianFunc(buffer, value)
	}
	err := m._dataBuffer.WriteBytesWithContext(ctx, buffer)
	return _wrapError(err, "marshal "+numTypeRep.typeName)
}

// _readNative reads a native-encoded numeric value using the provided byte
// order and conversion callbacks.
//
// Parameters:
//   - ctx: The request context used for buffer reads.
//   - dataBuffer: The source buffer.
//   - byteOrder: The byte order to use when decoding.
//   - typeName: The type name used in error reporting.
//   - maxBytes: The number of bytes to read.
//   - littleEndianFunc: The decoder to use for little-endian values.
//   - bigEndianFunc: The decoder to use for big-endian values.
//
// Returns:
//   - The decoded numeric value.
//   - An error if the value cannot be read or decoded.
func _readNative[T unmarshalTypes, V unmarshalTypes](ctx context.Context, dataBuffer driverCommon.DataBuffer, byteOrder driverCommon.ByteOrder, typeName string, maxBytes uint8,
	littleEndianFunc func([]byte) V, bigEndianFunc func([]byte) V) (T, error) {
	buffer, err := dataBuffer.ReadBytesWithContext(ctx, int32(maxBytes))
	if err != nil {
		return 0, _wrapError(err, "unmarshal "+typeName)
	}
	if byteOrder == driverCommon.LITTLE_ENDIAN {
		return T(littleEndianFunc(*buffer)), nil
	}
	return T(bigEndianFunc(*buffer)), nil
}

// Wraps the original error with message containing the data type name.
//
// Parameters:
//   - err: the original error
//   - dataType: the data type to add to the error message.
//
// Returns:
//   - The wrapped error, or nil if `err` is nil.
func _wrapError(err error, dataType string) error {
	if err != nil {
		err = common.NewOracleError(oracleErrors.MarshalEngineError, err, dataType)
	}
	return err
}
