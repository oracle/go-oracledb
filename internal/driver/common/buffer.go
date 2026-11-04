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

// A ReadBuffer reads fixed-width integers and byte slices from a byte buffer.
// Reads are either sequential, advancing a cursor through the buffer, or
// absolute, addressing a fixed offset without moving the cursor. Values are
// decoded with the byte order selected when the buffer was created.
//
// A failed read never moves the cursor and never modifies the data. A
// ReadBuffer is not safe for concurrent use.
type ReadBuffer interface {
	// Position Returns the offset of the next sequential read
	// Returns:
	//  - The offset of the next sequential read
	// Errors:
	//  - None.
	Position() uint
	// Size Returns the number of bytes available to read
	// Returns:
	//  - The number of bytes available to read
	// Errors:
	//  - None.
	Size() uint
	// SetPosition Moves the cursor to an absolute offset
	// Parameters:
	//  - The target offset, which must not exceed Size
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the position is outside the buffer
	SetPosition(position uint) error
	// ReadUB1 Reads an unsigned byte
	// Returns:
	//  - The unsigned byte
	// Errors:
	//  - InvalidBufferOperation when no byte remains to read
	ReadUB1() (UB1, error)
	// ReadUB2 Reads a two unsigned bytes long value
	// Returns:
	//  - The two unsigned bytes long value
	// Errors:
	//  - InvalidBufferOperation when fewer than two bytes remain
	ReadUB2() (UB2, error)
	// ReadUB4 Reads a four unsigned bytes long value
	// Returns:
	//  - The four unsigned bytes long value
	// Errors:
	//  - InvalidBufferOperation when fewer than four bytes remain
	ReadUB4() (UB4, error)
	// ReadUB8 Reads an eight unsigned bytes long value
	// Returns:
	//  - The eight unsigned bytes long value
	// Errors:
	//  - InvalidBufferOperation when fewer than eight bytes remain
	ReadUB8() (UB8, error)
	// ReadSB1 Reads a signed byte
	// Returns:
	//  - The signed byte
	// Errors:
	//  - InvalidBufferOperation when no byte remains to read
	ReadSB1() (SB1, error)
	// ReadSB2 Reads a two signed bytes long value
	// Returns:
	//  - The two signed bytes long value
	// Errors:
	//  - InvalidBufferOperation when fewer than two bytes remain
	ReadSB2() (SB2, error)
	// ReadSB4 Reads a four signed bytes long value
	// Returns:
	//  - The four signed bytes long value
	// Errors:
	//  - InvalidBufferOperation when fewer than four bytes remain
	ReadSB4() (SB4, error)
	// ReadBytes Reads a sequence of bytes and advances the cursor
	// Parameters:
	//  - The number of bytes to read
	// Returns:
	//  - A view aliasing the buffer's data
	// Errors:
	//  - InvalidBufferOperation when fewer bytes remain than requested
	ReadBytes(length uint) (B1Array, error)
	// ReadUB1At Reads an unsigned byte at an absolute offset
	// Parameters:
	//  - The offset of the byte to read
	// Returns:
	//  - The unsigned byte
	// Errors:
	//  - InvalidBufferOperation when the offset is outside the buffer
	ReadUB1At(offset uint) (UB1, error)
	// ReadUB2At Reads a two unsigned bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to read
	// Returns:
	//  - The two unsigned bytes long value
	// Errors:
	//  - InvalidBufferOperation when the value range extends past the end of the buffer
	ReadUB2At(offset uint) (UB2, error)
	// ReadUB4At Reads a four unsigned bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to read
	// Returns:
	//  - The four unsigned bytes long value
	// Errors:
	//  - InvalidBufferOperation when the value range extends past the end of the buffer
	ReadUB4At(offset uint) (UB4, error)
	// ReadUB8At Reads an eight unsigned bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to read
	// Returns:
	//  - The eight unsigned bytes long value
	// Errors:
	//  - InvalidBufferOperation when the value range extends past the end of the buffer
	ReadUB8At(offset uint) (UB8, error)
	// ReadSB1At Reads a signed byte at an absolute offset
	// Parameters:
	//  - The offset of the byte to read
	// Returns:
	//  - The signed byte
	// Errors:
	//  - InvalidBufferOperation when the offset is outside the buffer
	ReadSB1At(offset uint) (SB1, error)
	// ReadSB2At Reads a two signed bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to read
	// Returns:
	//  - The two signed bytes long value
	// Errors:
	//  - InvalidBufferOperation when the value range extends past the end of the buffer
	ReadSB2At(offset uint) (SB2, error)
	// ReadSB4At Reads a four signed bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to read
	// Returns:
	//  - The four signed bytes long value
	// Errors:
	//  - InvalidBufferOperation when the value range extends past the end of the buffer
	ReadSB4At(offset uint) (SB4, error)
	// ReadBytesAt Reads a sequence of bytes at an absolute offset
	// Parameters:
	//  - The offset of the bytes to read
	//  - The number of bytes to read
	// Returns:
	//  - A view aliasing the buffer's data
	// Errors:
	//  - InvalidBufferOperation when the range extends past the end of the buffer
	ReadBytesAt(offset, length uint) (B1Array, error)
}

// A WriteBuffer appends fixed-width integers and byte slices to a byte buffer
// that grows automatically. Appends advance the write position; the Write*At
// methods overwrite bytes that were already written or reserved and leave the
// position unchanged. Values are encoded with the byte order selected when
// the buffer was created.
//
// Appends cannot fail because the buffer grows as needed. A WriteBuffer is
// not safe for concurrent use.
type WriteBuffer interface {
	// Position Returns the offset at which the next byte is appended
	// Returns:
	//  - The offset at which the next byte is appended
	// Errors:
	//  - None.
	Position() uint
	// Size Returns the number of bytes written
	// Returns:
	//  - The number of bytes written, including reserved bytes
	// Errors:
	//  - None.
	Size() uint
	// WriteUB1 Appends an unsigned byte
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteUB1(value UB1)
	// WriteUB2 Appends a two unsigned bytes long value
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteUB2(value UB2)
	// WriteUB4 Appends a four unsigned bytes long value
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteUB4(value UB4)
	// WriteUB8 Appends an eight unsigned bytes long value
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteUB8(value UB8)
	// WriteSB1 Appends a signed byte
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteSB1(value SB1)
	// WriteSB2 Appends a two signed bytes long value
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteSB2(value SB2)
	// WriteSB4 Appends a four signed bytes long value
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteSB4(value SB4)
	// WriteBytes Appends a sequence of bytes
	// Parameters:
	//  - The bytes to append
	// Returns:
	//  - None.
	// Errors:
	//  - None.
	WriteBytes(value B1Array)
	// Reserve Appends zero-filled bytes for a later Write*At call. The
	// reserved bytes count toward Size and appear in Bytes as zeros until
	// they are overwritten. Reserve(0) returns the current position without
	// changing the buffer
	// Parameters:
	//  - The number of bytes to reserve
	// Returns:
	//  - The initial offset of the reserved bytes
	// Errors:
	//  - None.
	Reserve(length uint) uint
	// WriteUB1At Overwrites an unsigned byte at an absolute offset
	// Parameters:
	//  - The offset of the byte to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the offset is outside the bytes written so far
	WriteUB1At(offset uint, value UB1) error
	// WriteUB2At Overwrites a two unsigned bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the value range is outside the bytes written so far
	WriteUB2At(offset uint, value UB2) error
	// WriteUB4At Overwrites a four unsigned bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the value range is outside the bytes written so far
	WriteUB4At(offset uint, value UB4) error
	// WriteUB8At Overwrites an eight unsigned bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the value range is outside the bytes written so far
	WriteUB8At(offset uint, value UB8) error
	// WriteSB1At Overwrites a signed byte at an absolute offset
	// Parameters:
	//  - The offset of the byte to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the offset is outside the bytes written so far
	WriteSB1At(offset uint, value SB1) error
	// WriteSB2At Overwrites a two signed bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the value range is outside the bytes written so far
	WriteSB2At(offset uint, value SB2) error
	// WriteSB4At Overwrites a four signed bytes long value at an absolute offset
	// Parameters:
	//  - The offset of the value to overwrite
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the value range is outside the bytes written so far
	WriteSB4At(offset uint, value SB4) error
	// WriteBytesAt Overwrites a sequence of bytes at an absolute offset
	// Parameters:
	//  - The offset of the bytes to overwrite
	//  - The bytes to write
	// Returns:
	//  - None.
	// Errors:
	//  - InvalidBufferOperation when the range is outside the bytes written so far
	WriteBytesAt(offset uint, value B1Array) error
	// Bytes Returns an independent copy of the bytes written so far
	// Returns:
	//  - A non-nil copy of the written bytes
	// Errors:
	//  - None.
	Bytes() B1Array
}

// readBuffer is the ReadBuffer implementation over a fixed byte slice.
type readBuffer struct {
	data      B1Array
	pos       uint
	converter binary.ByteOrder
}

// writeBuffer is the WriteBuffer implementation over an automatically
// growing byte slice.
type writeBuffer struct {
	data      B1Array
	converter binary.ByteOrder
}

// NewReadBuffer returns a ReadBuffer that reads data in the given byte
// order. The buffer aliases data and observes later modifications to it.
// Values other than LITTLE_ENDIAN select big-endian encoding.
func NewReadBuffer(data B1Array, order ByteOrder) ReadBuffer {
	return &readBuffer{data: data, converter: bufferByteOrder(order)}
}

// NewWriteBuffer returns an empty WriteBuffer that encodes values in the
// given byte order. Values other than LITTLE_ENDIAN select big-endian
// encoding.
func NewWriteBuffer(order ByteOrder) WriteBuffer {
	return NewWriteBufferWithCapacity(0, order)
}

// NewWriteBufferWithCapacity returns a WriteBuffer preallocated for capacity
// bytes. The capacity is a performance hint only; the buffer grows
// automatically as bytes are appended. Values other than LITTLE_ENDIAN select
// big-endian encoding.
func NewWriteBufferWithCapacity(capacity uint, order ByteOrder) WriteBuffer {
	return &writeBuffer{data: make(B1Array, 0, int(capacity)), converter: bufferByteOrder(order)}
}

// bufferByteOrder maps the driver's byte-order marker to its standard library
// counterpart. Big-endian encoding is returned for any value other than
// LITTLE_ENDIAN.
func bufferByteOrder(order ByteOrder) binary.ByteOrder {
	if order == LITTLE_ENDIAN {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// Position returns the offset of the next sequential read.
func (b *readBuffer) Position() uint {
	return b.pos
}

// Size returns the number of bytes in the buffer.
func (b *readBuffer) Size() uint {
	return uint(len(b.data))
}

// SetPosition moves the sequential read cursor to position.
func (b *readBuffer) SetPosition(position uint) error {
	if position <= b.Size() {
		b.pos = position
		return nil
	}
	common.Odl.Finest("readBuffer.SetPosition: failed", "reason", "position outside buffer", "position", position, "size", b.Size())
	return common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
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

// ReadBytes reads length bytes and advances the cursor.
func (b *readBuffer) ReadBytes(length uint) (B1Array, error) {
	return b._read(length)
}

// ReadUB1At reads one unsigned byte at offset.
func (b *readBuffer) ReadUB1At(offset uint) (UB1, error) {
	data, err := b._sliceAt(offset, 1)
	if err != nil {
		return 0, err
	}
	return UB1(data[0]), nil
}

// ReadUB2At reads a two-byte unsigned value at offset.
func (b *readBuffer) ReadUB2At(offset uint) (UB2, error) {
	data, err := b._sliceAt(offset, 2)
	if err != nil {
		return 0, err
	}
	return UB2(b.converter.Uint16(data)), nil
}

// ReadUB4At reads a four-byte unsigned value at offset.
func (b *readBuffer) ReadUB4At(offset uint) (UB4, error) {
	data, err := b._sliceAt(offset, 4)
	if err != nil {
		return 0, err
	}
	return UB4(b.converter.Uint32(data)), nil
}

// ReadUB8At reads an eight-byte unsigned value at offset.
func (b *readBuffer) ReadUB8At(offset uint) (UB8, error) {
	data, err := b._sliceAt(offset, 8)
	if err != nil {
		return 0, err
	}
	return UB8(b.converter.Uint64(data)), nil
}

// ReadSB1At reads one signed byte at offset.
func (b *readBuffer) ReadSB1At(offset uint) (SB1, error) {
	value, err := b.ReadUB1At(offset)
	return SB1(value), err
}

// ReadSB2At reads a two-byte signed value at offset.
func (b *readBuffer) ReadSB2At(offset uint) (SB2, error) {
	value, err := b.ReadUB2At(offset)
	return SB2(value), err
}

// ReadSB4At reads a four-byte signed value at offset.
func (b *readBuffer) ReadSB4At(offset uint) (SB4, error) {
	value, err := b.ReadUB4At(offset)
	return SB4(value), err
}

// ReadBytesAt reads length bytes at offset without moving the cursor.
func (b *readBuffer) ReadBytesAt(offset, length uint) (B1Array, error) {
	return b._sliceAt(offset, length)
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

// _sliceAt returns length bytes at offset without moving the cursor, or fails
// when the offset is outside the buffer or the range extends past its end.
// The bounds comparison is arranged so that arithmetic cannot overflow even
// for hostile offsets.
func (b *readBuffer) _sliceAt(offset, length uint) (B1Array, error) {
	size := b.Size()
	if offset > size {
		common.Odl.Finest("readBuffer._sliceAt: failed", "reason", "offset outside buffer", "offset", offset, "size", size)
		return nil, common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	if length > size-offset {
		common.Odl.Finest("readBuffer._sliceAt: failed", "reason", "range extends past end of buffer", "offset", offset, "length", length, "size", size)
		return nil, common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
	}
	return b.data[int(offset):int(offset+length)], nil
}

// Position returns the offset at which the next byte is appended.
func (b *writeBuffer) Position() uint {
	return uint(len(b.data))
}

// Size returns the number of bytes written.
func (b *writeBuffer) Size() uint {
	return uint(len(b.data))
}

// WriteUB1 appends one unsigned byte.
func (b *writeBuffer) WriteUB1(value UB1) {
	b.data = append(b.data, byte(value))
}

// WriteUB2 appends a two-byte unsigned value.
func (b *writeBuffer) WriteUB2(value UB2) {
	b.converter.PutUint16(b.grow(2), uint16(value))
}

// WriteUB4 appends a four-byte unsigned value.
func (b *writeBuffer) WriteUB4(value UB4) {
	b.converter.PutUint32(b.grow(4), uint32(value))
}

// WriteUB8 appends an eight-byte unsigned value.
func (b *writeBuffer) WriteUB8(value UB8) {
	b.converter.PutUint64(b.grow(8), uint64(value))
}

// WriteSB1 appends one signed byte.
func (b *writeBuffer) WriteSB1(value SB1) {
	b.WriteUB1(UB1(value))
}

// WriteSB2 appends a two-byte signed value.
func (b *writeBuffer) WriteSB2(value SB2) {
	b.WriteUB2(UB2(value))
}

// WriteSB4 appends a four-byte signed value.
func (b *writeBuffer) WriteSB4(value SB4) {
	b.WriteUB4(UB4(value))
}

// WriteBytes appends the bytes of value.
func (b *writeBuffer) WriteBytes(value B1Array) {
	b.data = append(b.data, value...)
}

// Reserve appends length zero-filled bytes and returns their initial offset.
func (b *writeBuffer) Reserve(length uint) uint {
	start := b.Position()
	b.grow(length)
	return start
}

// WriteUB1At overwrites one unsigned byte at offset.
func (b *writeBuffer) WriteUB1At(offset uint, value UB1) error {
	data, err := b._sliceAt(offset, 1)
	if err != nil {
		return err
	}
	data[0] = byte(value)
	return nil
}

// WriteUB2At overwrites a two-byte unsigned value at offset.
func (b *writeBuffer) WriteUB2At(offset uint, value UB2) error {
	data, err := b._sliceAt(offset, 2)
	if err != nil {
		return err
	}
	b.converter.PutUint16(data, uint16(value))
	return nil
}

// WriteUB4At overwrites a four-byte unsigned value at offset.
func (b *writeBuffer) WriteUB4At(offset uint, value UB4) error {
	data, err := b._sliceAt(offset, 4)
	if err != nil {
		return err
	}
	b.converter.PutUint32(data, uint32(value))
	return nil
}

// WriteUB8At overwrites an eight-byte unsigned value at offset.
func (b *writeBuffer) WriteUB8At(offset uint, value UB8) error {
	data, err := b._sliceAt(offset, 8)
	if err != nil {
		return err
	}
	b.converter.PutUint64(data, uint64(value))
	return nil
}

// WriteSB1At overwrites one signed byte at offset.
func (b *writeBuffer) WriteSB1At(offset uint, value SB1) error {
	return b.WriteUB1At(offset, UB1(value))
}

// WriteSB2At overwrites a two-byte signed value at offset.
func (b *writeBuffer) WriteSB2At(offset uint, value SB2) error {
	return b.WriteUB2At(offset, UB2(value))
}

// WriteSB4At overwrites a four-byte signed value at offset.
func (b *writeBuffer) WriteSB4At(offset uint, value SB4) error {
	return b.WriteUB4At(offset, UB4(value))
}

// WriteBytesAt overwrites the bytes of value at offset.
func (b *writeBuffer) WriteBytesAt(offset uint, value B1Array) error {
	data, err := b._sliceAt(offset, uint(len(value)))
	if err != nil {
		return err
	}
	copy(data, value)
	return nil
}

// Bytes returns an independent, non-nil copy of the written bytes.
func (b *writeBuffer) Bytes() B1Array {
	return append(B1Array{}, b.data...)
}

// grow appends length zero-filled bytes and returns the appended range,
// reusing spare capacity when available. The buffer never writes past its
// length, so bytes beyond the current length are always zero.
func (b *writeBuffer) grow(length uint) B1Array {
	start := len(b.data)
	end := start + int(length)
	if end > cap(b.data) {
		b.data = slices.Grow(b.data, int(length))
	}
	b.data = b.data[:end]
	return b.data[start:end]
}

// _sliceAt returns the existing range offset..offset+length for in-place
// overwrites, without changing the append position.
func (b *writeBuffer) _sliceAt(offset, length uint) (B1Array, error) {
	size := b.Size()
	if offset <= size && length <= size-offset {
		return b.data[int(offset):int(offset+length)], nil
	}
	common.Odl.Finest("writeBuffer._sliceAt: failed", "reason", "range outside buffer", "offset", offset, "length", length, "size", size)
	return nil, common.NewOracleError(oracleErrors.InvalidBufferOperation, nil)
}
