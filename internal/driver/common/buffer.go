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
	"slices"

	common "github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Seekable controls a buffer's cursor and saved positions.
type Seekable interface {
	// Position returns the offset of the next read or write.
	Position() uint

	// Size returns the number of bytes in the buffer.
	Size() uint

	// Seek moves the cursor to position. Position may equal Size. If position
	// exceeds Size, Seek returns InvalidBufferOperation and leaves the cursor
	// unchanged.
	Seek(position uint) error

	// Set saves the current position. Calls to Set may be nested.
	Set()

	// Rewind restores and removes the most recently saved position. If no
	// position is saved, it returns InvalidBufferOperation.
	Rewind() error
}

// ReadBuffer reads values at its cursor using the byte order selected at
// construction. Each successful read advances the cursor by the number of
// bytes read. A read that exceeds the buffer returns InvalidBufferOperation
// and leaves the cursor unchanged.
type ReadBuffer interface {
	Seekable

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

	// ReadBytes reads length bytes and returns an independent copy.
	ReadBytes(length uint) (B1Array, error)
}

// WriteBuffer writes values at its cursor using the byte order selected at
// construction. Each write advances the cursor, overwriting existing bytes and
// extending the buffer as needed.
type WriteBuffer interface {
	Seekable

	// WriteUB1 writes a one-byte unsigned integer.
	WriteUB1(value UB1)
	// WriteUB2 writes a two-byte unsigned integer.
	WriteUB2(value UB2)
	// WriteUB4 writes a four-byte unsigned integer.
	WriteUB4(value UB4)
	// WriteUB8 writes an eight-byte unsigned integer.
	WriteUB8(value UB8)
	// WriteSB1 writes a one-byte signed integer.
	WriteSB1(value SB1)
	// WriteSB2 writes a two-byte signed integer.
	WriteSB2(value SB2)
	// WriteSB4 writes a four-byte signed integer.
	WriteSB4(value SB4)

	// WriteBytes writes value at the cursor.
	WriteBytes(value B1Array)

	// Reserve writes length zero bytes at the cursor and returns their starting
	// offset.
	Reserve(length uint) uint

	// Bytes returns a non-nil copy of all bytes in the buffer.
	Bytes() B1Array
}

// NewReadBuffer returns a ReadBuffer that reads data in the given byte order.
// It uses data directly, so changes to data affect subsequent reads.
// Values other than LITTLE_ENDIAN select big-endian encoding.
func NewReadBuffer(data B1Array, order ByteOrder) ReadBuffer {
	return &readBuffer{data: data, converter: bufferByteOrder(order)}
}

// NewWriteBuffer returns an empty WriteBuffer that encodes values in the
// given byte order.
func NewWriteBuffer(order ByteOrder) WriteBuffer {
	return NewWriteBufferWithCapacity(order, 0)
}

// NewWriteBufferWithCapacity returns a WriteBuffer preallocated for capacity
// bytes. The capacity is a performance hint only; the buffer grows
// automatically as bytes are written.
func NewWriteBufferWithCapacity(order ByteOrder, capacity uint) WriteBuffer {
	return &writeBuffer{data: make(B1Array, 0, int(capacity)), converter: bufferByteOrder(order)}
}

// bufferByteOrder maps the driver's byte-order marker to its standard library
// counterpart.
func bufferByteOrder(order ByteOrder) binary.ByteOrder {
	if order == LITTLE_ENDIAN {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// readBuffer is the ReadBuffer implementation over a fixed byte slice.
type readBuffer struct {
	data      B1Array
	pos       uint
	marks     []uint
	converter binary.ByteOrder
}

// Position returns the offset of the next sequential read.
func (b *readBuffer) Position() uint {
	return b.pos
}

// Size returns the number of bytes in the buffer.
func (b *readBuffer) Size() uint {
	return uint(len(b.data))
}

// Seek moves the read cursor to position.
func (b *readBuffer) Seek(position uint) error {
	if position <= b.Size() {
		b.pos = position
		return nil
	}
	common.Odl.Finest("readBuffer.Seek: failed", "reason", "position outside buffer", "position", position, "size", b.Size())
	return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
}

// Set saves the current read position.
func (b *readBuffer) Set() {
	b.marks = append(b.marks, b.pos)
}

// Rewind restores the last saved read position.
func (b *readBuffer) Rewind() error {
	if len(b.marks) == 0 {
		common.Odl.Finest("readBuffer.Rewind: failed", "reason", "no saved position")
		return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	b.pos = b.marks[len(b.marks)-1]
	b.marks = b.marks[:len(b.marks)-1]
	return nil
}

// ReadUB1 reads one unsigned byte.
func (b *readBuffer) ReadUB1() (UB1, error) {
	data, err := b._read(1)
	if err != nil {
		return 0, err
	}
	return UB1(data[0]), nil
}

// ReadUB2 reads a two-byte unsigned value.
func (b *readBuffer) ReadUB2() (UB2, error) {
	data, err := b._read(2)
	if err != nil {
		return 0, err
	}
	return UB2(b.converter.Uint16(data)), nil
}

// ReadUB4 reads a four-byte unsigned value.
func (b *readBuffer) ReadUB4() (UB4, error) {
	data, err := b._read(4)
	if err != nil {
		return 0, err
	}
	return UB4(b.converter.Uint32(data)), nil
}

// ReadUB8 reads an eight-byte unsigned value.
func (b *readBuffer) ReadUB8() (UB8, error) {
	data, err := b._read(8)
	if err != nil {
		return 0, err
	}
	return UB8(b.converter.Uint64(data)), nil
}

// ReadSB1 reads one signed byte.
func (b *readBuffer) ReadSB1() (SB1, error) {
	value, err := b.ReadUB1()
	return SB1(value), err
}

// ReadSB2 reads a two-byte signed value.
func (b *readBuffer) ReadSB2() (SB2, error) {
	value, err := b.ReadUB2()
	return SB2(value), err
}

// ReadSB4 reads a four-byte signed value.
func (b *readBuffer) ReadSB4() (SB4, error) {
	value, err := b.ReadUB4()
	return SB4(value), err
}

// ReadBytes reads length bytes into an independent copy and advances the cursor.
func (b *readBuffer) ReadBytes(length uint) (B1Array, error) {
	data, err := b._read(length)
	if err != nil {
		return nil, err
	}
	return slices.Clone(data), nil
}

// _read returns the next length bytes and advances the cursor, or fails when
// fewer bytes remain.
func (b *readBuffer) _read(length uint) (B1Array, error) {
	remaining := b.Size() - b.pos
	if length > remaining {
		common.Odl.Finest("readBuffer._read: failed", "reason", "buffer underflow", "position", b.pos, "length", length, "remaining", remaining, "size", b.Size())
		return nil, common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	start := b.pos
	b.pos += length
	return b.data[int(start):int(b.pos)], nil
}

// writeBuffer is the WriteBuffer implementation over an automatically
// growing byte slice.
type writeBuffer struct {
	data      B1Array
	pos       uint
	marks     []uint
	converter binary.ByteOrder
}

// Position returns the offset of the next write.
func (b *writeBuffer) Position() uint {
	return b.pos
}

// Size returns the number of bytes written.
func (b *writeBuffer) Size() uint {
	return uint(len(b.data))
}

// Seek moves the write cursor within the written bytes.
func (b *writeBuffer) Seek(position uint) error {
	if position > b.Size() {
		common.Odl.Finest("writeBuffer.Seek: failed", "reason", "position outside buffer", "position", position, "size", b.Size())
		return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	b.pos = position
	return nil
}

// Set saves the current write position.
func (b *writeBuffer) Set() {
	b.marks = append(b.marks, b.pos)
}

// Rewind restores the last saved write position.
func (b *writeBuffer) Rewind() error {
	if len(b.marks) == 0 {
		common.Odl.Finest("writeBuffer.Rewind: failed", "reason", "no saved position")
		return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	b.pos = b.marks[len(b.marks)-1]
	b.marks = b.marks[:len(b.marks)-1]
	return nil
}

// WriteUB1 writes one unsigned byte.
func (b *writeBuffer) WriteUB1(value UB1) {
	dest := b.prepareWrite(1)
	dest[0] = byte(value)
}

// WriteUB2 writes a two-byte unsigned value.
func (b *writeBuffer) WriteUB2(value UB2) {
	dest := b.prepareWrite(2)
	b.converter.PutUint16(dest, uint16(value))
}

// WriteUB4 writes a four-byte unsigned value.
func (b *writeBuffer) WriteUB4(value UB4) {
	dest := b.prepareWrite(4)
	b.converter.PutUint32(dest, uint32(value))
}

// WriteUB8 writes an eight-byte unsigned value.
func (b *writeBuffer) WriteUB8(value UB8) {
	dest := b.prepareWrite(8)
	b.converter.PutUint64(dest, uint64(value))
}

// WriteSB1 writes one signed byte.
func (b *writeBuffer) WriteSB1(value SB1) {
	b.WriteUB1(UB1(value))
}

// WriteSB2 writes a two-byte signed value.
func (b *writeBuffer) WriteSB2(value SB2) {
	b.WriteUB2(UB2(value))
}

// WriteSB4 writes a four-byte signed value.
func (b *writeBuffer) WriteSB4(value SB4) {
	b.WriteUB4(UB4(value))
}

// WriteBytes writes the bytes of value.
func (b *writeBuffer) WriteBytes(value B1Array) {
	dest := b.prepareWrite(uint(len(value)))
	copy(dest, value)
}

// Reserve writes length zero-filled bytes and returns their initial offset.
func (b *writeBuffer) Reserve(length uint) uint {
	start := b.Position()
	dest := b.prepareWrite(length)
	clear(dest)
	return start
}

// Bytes returns an independent, non-nil copy of the written bytes.
func (b *writeBuffer) Bytes() B1Array {
	return append(B1Array{}, b.data...)
}

// prepareWrite advances the cursor and returns the writable range, extending data if needed.
func (b *writeBuffer) prepareWrite(length uint) B1Array {
	start := int(b.pos)
	end := start + int(length)
	if end > len(b.data) {
		b.data = slices.Grow(b.data, end-len(b.data))
		b.data = b.data[:end]
	}
	b.pos = uint(end)
	return b.data[start:end]
}
