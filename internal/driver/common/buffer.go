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

package common

import (
	"encoding/binary"
	"math"
	"slices"

	common "github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Seekable provides access to a buffer's current position. It allows callers to
// move the cursor, save its position, and later restore that position.
type Seekable interface {
	// Position returns the current cursor position, measured in bytes.
	Position() int

	// Seek moves the cursor using an offset measured in bytes. Implementations
	// define the reference point for the offset and the valid target positions.
	// If the target position is invalid, Seek returns an error.
	// On any error, the cursor remains unchanged.
	Seek(offset int) error

	// Set saves the current cursor position.
	Set()

	// Rewind restores the saved cursor position.
	Rewind()
}

// BufferReader provides operations for reading signed and unsigned integers and
// byte sequences from a buffer.
type BufferReader interface {
	// ReadUB1 reads a one-byte unsigned integer.
	ReadUB1() (UB1, error)
	// ReadUB2 reads a two-byte unsigned integer.
	ReadUB2() (UB2, error)
	// ReadUB4 reads a four-byte unsigned integer.
	ReadUB4() (UB4, error)
	// ReadUB8 reads an eight-byte unsigned integer.
	ReadUB8() (UB8, error)
	// ReadSB1 reads a one-byte signed integer.
	ReadSB1() (SB1, error)
	// ReadSB2 reads a two-byte signed integer.
	ReadSB2() (SB2, error)
	// ReadSB4 reads a four-byte signed integer.
	ReadSB4() (SB4, error)

	// ReadBytes reads length bytes.
	ReadBytes(length int) (B1Array, error)
}

// BufferWriter provides operations for writing signed and unsigned integers and
// byte sequences to a buffer.
type BufferWriter interface {
	// WriteUB1 writes a one-byte unsigned integer.
	WriteUB1(value UB1) error
	// WriteUB2 writes a two-byte unsigned integer.
	WriteUB2(value UB2) error
	// WriteUB4 writes a four-byte unsigned integer.
	WriteUB4(value UB4) error
	// WriteUB8 writes an eight-byte unsigned integer.
	WriteUB8(value UB8) error
	// WriteSB1 writes a one-byte signed integer.
	WriteSB1(value SB1) error
	// WriteSB2 writes a two-byte signed integer.
	WriteSB2(value SB2) error
	// WriteSB4 writes a four-byte signed integer.
	WriteSB4(value SB4) error

	// WriteBytes writes value.
	WriteBytes(value B1Array) error

	// Reserve reserves length bytes and returns their starting offset.
	Reserve(length int) (int, error)
}

// NewReadBuffer returns an implementation of [BufferReader] that reads data in the
// given byte order. The implementation also satisfies [Seekable]. Seek offsets
// are relative to the current position. The target must be within the buffer
// or immediately after its last byte.
//
// It uses data directly, so changes to data affect subsequent reads. ReadBytes
// returns an independent copy, and invalid reads leave the cursor unchanged.
//
// The saved position is initially zero. Set replaces it with the current
// position, and Rewind restores it without clearing it.
func NewReadBuffer(data B1Array, order ByteOrder) *ReadBuffer {
	return &ReadBuffer{data: data, converter: bufferByteOrder(order)}
}

// NewGrowableWriteBuffer returns an implementation of [BufferWriter] that starts
// empty, encodes values in the given byte order, and grows as needed. It also
// satisfies [Seekable]. Seek offsets are relative to the current position. The
// saved position starts at zero. Set replaces it, and Rewind restores it without
// clearing it.
func NewGrowableWriteBuffer(order ByteOrder) *GrowableWriteBuffer {
	return NewGrowableWriteBufferWithCapacity(order, 0)
}

// NewGrowableWriteBufferWithCapacity returns the same [BufferWriter] and [Seekable]
// implementation as [NewGrowableWriteBuffer], preallocated for capacity bytes.
// Capacity must be non-negative and is a performance hint only. The buffer
// grows automatically as bytes are written.
func NewGrowableWriteBufferWithCapacity(order ByteOrder, capacity int) *GrowableWriteBuffer {
	return &GrowableWriteBuffer{data: make(B1Array, 0, capacity), converter: bufferByteOrder(order)}
}

// bufferByteOrder maps the driver's byte-order marker to its standard library
// counterpart.
func bufferByteOrder(order ByteOrder) binary.ByteOrder {
	if order == LITTLE_ENDIAN {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// ReadBuffer implements [BufferReader] over a fixed byte slice. Create one with
// [NewReadBuffer]. It keeps one saved position, initially zero. Set replaces
// that position, and Rewind moves the cursor back to it without clearing it.
type ReadBuffer struct {
	data      B1Array
	pos       int
	mark      int
	converter binary.ByteOrder
}

// Position returns the offset of the next sequential read.
func (b *ReadBuffer) Position() int {
	return b.pos
}

// Size returns the size of the buffer.
func (b *ReadBuffer) Size() int {
	return len(b.data)
}

// Seek moves the read cursor by offset bytes relative to its current position.
func (b *ReadBuffer) Seek(offset int) error {
	position := b.pos + offset
	if position < 0 || position > b.Size() {
		common.Odl.Finest("ReadBuffer.Seek: failed", "reason", "target outside buffer", "offset", offset, "position", b.pos, "size", b.Size())
		return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	b.pos = position
	return nil
}

// Set saves the current read position.
func (b *ReadBuffer) Set() {
	b.mark = b.pos
}

// Rewind restores the last saved read position.
func (b *ReadBuffer) Rewind() {
	b.pos = b.mark
}

// ReadUB1 reads one unsigned byte.
func (b *ReadBuffer) ReadUB1() (UB1, error) {
	data, err := b._read(1)
	if err != nil {
		return 0, err
	}
	return UB1(data[0]), nil
}

// ReadUB2 reads a two-byte unsigned value.
func (b *ReadBuffer) ReadUB2() (UB2, error) {
	data, err := b._read(2)
	if err != nil {
		return 0, err
	}
	return UB2(b.converter.Uint16(data)), nil
}

// ReadUB4 reads a four-byte unsigned value.
func (b *ReadBuffer) ReadUB4() (UB4, error) {
	data, err := b._read(4)
	if err != nil {
		return 0, err
	}
	return UB4(b.converter.Uint32(data)), nil
}

// ReadUB8 reads an eight-byte unsigned value.
func (b *ReadBuffer) ReadUB8() (UB8, error) {
	data, err := b._read(8)
	if err != nil {
		return 0, err
	}
	return UB8(b.converter.Uint64(data)), nil
}

// ReadSB1 reads one signed byte.
func (b *ReadBuffer) ReadSB1() (SB1, error) {
	value, err := b.ReadUB1()
	return SB1(value), err
}

// ReadSB2 reads a two-byte signed value.
func (b *ReadBuffer) ReadSB2() (SB2, error) {
	value, err := b.ReadUB2()
	return SB2(value), err
}

// ReadSB4 reads a four-byte signed value.
func (b *ReadBuffer) ReadSB4() (SB4, error) {
	value, err := b.ReadUB4()
	return SB4(value), err
}

// ReadBytes reads length bytes into an independent copy and advances the cursor.
func (b *ReadBuffer) ReadBytes(length int) (B1Array, error) {
	data, err := b._read(length)
	if err != nil {
		return nil, err
	}
	return slices.Clone(data), nil
}

// _read returns the next length bytes and advances the cursor, or fails when
// fewer bytes remain.
func (b *ReadBuffer) _read(length int) (B1Array, error) {
	remaining := b.Size() - b.pos
	if length < 0 || length > remaining {
		common.Odl.Finest("ReadBuffer._read: failed", "reason", "invalid read range", "position", b.pos, "length", length, "remaining", remaining, "size", b.Size())
		return nil, common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	start := b.pos
	b.pos += length
	return b.data[start:b.pos], nil
}

// GrowableWriteBuffer implements [BufferWriter] over an automatically growing byte
// slice. Create one with [NewGrowableWriteBuffer] or
// [NewGrowableWriteBufferWithCapacity]. It keeps one saved position, initially
// zero. Set replaces that position, and Rewind moves the cursor back to it
// without clearing it.
type GrowableWriteBuffer struct {
	data      B1Array
	pos       int
	mark      int
	converter binary.ByteOrder
}

// Position returns the offset of the next write.
func (b *GrowableWriteBuffer) Position() int {
	return b.pos
}

// Size returns the size of the buffer.
func (b *GrowableWriteBuffer) Size() int {
	return len(b.data)
}

// Seek moves the write cursor by offset bytes relative to its current position.
func (b *GrowableWriteBuffer) Seek(offset int) error {
	position := b.pos + offset
	if position < 0 || position > b.Size() {
		common.Odl.Finest("GrowableWriteBuffer.Seek: failed", "reason", "target outside buffer", "offset", offset, "position", b.pos, "size", b.Size())
		return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	b.pos = position
	return nil
}

// Set saves the current write position.
func (b *GrowableWriteBuffer) Set() {
	b.mark = b.pos
}

// Rewind restores the last saved write position.
func (b *GrowableWriteBuffer) Rewind() {
	b.pos = b.mark
}

// WriteUB1 writes one unsigned byte.
func (b *GrowableWriteBuffer) WriteUB1(value UB1) error {
	dest := b.prepareWrite(1)
	dest[0] = byte(value)
	return nil
}

// WriteUB2 writes a two-byte unsigned value.
func (b *GrowableWriteBuffer) WriteUB2(value UB2) error {
	dest := b.prepareWrite(2)
	b.converter.PutUint16(dest, uint16(value))
	return nil
}

// WriteUB4 writes a four-byte unsigned value.
func (b *GrowableWriteBuffer) WriteUB4(value UB4) error {
	dest := b.prepareWrite(4)
	b.converter.PutUint32(dest, uint32(value))
	return nil
}

// WriteUB8 writes an eight-byte unsigned value.
func (b *GrowableWriteBuffer) WriteUB8(value UB8) error {
	dest := b.prepareWrite(8)
	b.converter.PutUint64(dest, uint64(value))
	return nil
}

// WriteSB1 writes one signed byte.
func (b *GrowableWriteBuffer) WriteSB1(value SB1) error {
	return b.WriteUB1(UB1(value))
}

// WriteSB2 writes a two-byte signed value.
func (b *GrowableWriteBuffer) WriteSB2(value SB2) error {
	return b.WriteUB2(UB2(value))
}

// WriteSB4 writes a four-byte signed value.
func (b *GrowableWriteBuffer) WriteSB4(value SB4) error {
	return b.WriteUB4(UB4(value))
}

// WriteBytes writes the bytes of value.
func (b *GrowableWriteBuffer) WriteBytes(value B1Array) error {
	dest := b.prepareWrite(len(value))
	copy(dest, value)
	return nil
}

// Reserve advances the cursor by length bytes and returns their starting offset.
func (b *GrowableWriteBuffer) Reserve(length int) (int, error) {
	if length < 0 || length > math.MaxInt-b.pos {
		return 0, common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	start := b.Position()
	b.prepareWrite(length)
	return start, nil
}

// Bytes returns the buffer contents.
func (b *GrowableWriteBuffer) Bytes() B1Array {
	return append(B1Array{}, b.data...)
}

// prepareWrite advances the cursor and returns the writable range, extending
// data if needed.
func (b *GrowableWriteBuffer) prepareWrite(length int) B1Array {
	start := b.pos
	end := start + length
	if end > len(b.data) {
		b.data = slices.Grow(b.data, end-len(b.data))
		b.data = b.data[:end]
	}
	b.pos = end
	return b.data[start:end]
}
