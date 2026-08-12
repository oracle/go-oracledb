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

// oson_buffer.go provides the byte-reader abstraction used by the OSON parser.
package oson

import (
	"encoding/binary"
	"fmt"

	"github.com/oracle/go-driver/driver/common"
)

// osonBuffer provides sequential and absolute reads over an OSON document.
type osonBuffer struct {
	// data is OSON document bytes.
	data common.B1Array
	// pos is the next byte position for cursor-based reads.
	pos int
}

// newOsonBuffer returns an OSON buffer reader positioned at the start of buf.
func newOsonBuffer(buf common.B1Array) *osonBuffer {
	return &osonBuffer{data: buf}
}

// position returns the offset of the next cursor-based read.
func (b *osonBuffer) position() int {
	return b.pos
}

// setPosition moves the cursor to an absolute offset, including the end of data.
func (b *osonBuffer) setPosition(pos int) error {
	if pos < 0 || pos > len(b.data) {
		cause := fmt.Errorf("position %d out of bounds [0,%d]", pos, len(b.data))
		common.Odl.Error("osonBuffer.setPosition: failed", "error", cause, "position", pos, "limit", len(b.data))
		return common.NewOracleError(common.OsonBufferError, cause)
	}
	b.pos = pos
	return nil
}

// remaining reports bytes available to cursor-based reads.
func (b *osonBuffer) remaining() int {
	if b.pos < 0 || b.pos >= len(b.data) {
		return 0
	}
	return len(b.data) - b.pos
}

// size returns the document length in bytes.
func (b *osonBuffer) size() int {
	return len(b.data)
}

// readSlice returns a view of the next length bytes and advances the cursor.
func (b *osonBuffer) readSlice(length int) (common.B1Array, error) {
	start, err := b.advance(length)
	if err != nil {
		return nil, err
	}
	end := start + length
	return b.data[start:end], nil
}

// readUB1 reads an unsigned 1-byte OSON value.
func (b *osonBuffer) readUB1() (common.UB1, error) {
	value, err := b.readUint8()
	return common.UB1(value), err
}

// readUB2 reads an unsigned 2-byte big-endian OSON value.
func (b *osonBuffer) readUB2() (common.UB2, error) {
	value, err := b.readUint16()
	return common.UB2(value), err
}

// readUB4 reads an unsigned 4-byte big-endian OSON value.
func (b *osonBuffer) readUB4() (common.UB4, error) {
	value, err := b.readUint32()
	return common.UB4(value), err
}

// readSB1 reads a signed 1-byte OSON value.
func (b *osonBuffer) readSB1() (common.SB1, error) {
	value, err := b.readUint8()
	return common.SB1(int8(value)), err
}

// readSB2 reads a signed 2-byte big-endian OSON value.
func (b *osonBuffer) readSB2() (common.SB2, error) {
	value, err := b.readUint16()
	return common.SB2(int16(value)), err
}

// readSB4 reads a signed 4-byte big-endian OSON value.
func (b *osonBuffer) readSB4() (common.SB4, error) {
	value, err := b.readUint32()
	return common.SB4(int32(value)), err
}

// readSliceAt returns a view of an absolute range without moving the cursor.
func (b *osonBuffer) readSliceAt(offset, length int) (common.B1Array, error) {
	if err := b.ensureRange(offset, length, "osonBuffer.readSliceAt"); err != nil {
		return nil, err
	}
	return b.data[offset : offset+length], nil
}

// readUB1At reads an unsigned byte at an absolute offset.
func (b *osonBuffer) readUB1At(offset int) (common.UB1, error) {
	if err := b.ensureRange(offset, osonUB1Size, "osonBuffer.readUB1At"); err != nil {
		return 0, err
	}
	return common.UB1(b.data[offset]), nil
}

// readUB2At reads an unsigned 2-byte big-endian value at an absolute offset.
func (b *osonBuffer) readUB2At(offset int) (common.UB2, error) {
	if err := b.ensureRange(offset, osonUB2Size, "osonBuffer.readUB2At"); err != nil {
		return 0, err
	}
	return common.UB2(binary.BigEndian.Uint16(b.data[offset:])), nil
}

// readUB4At reads an unsigned 4-byte big-endian value at an absolute offset.
func (b *osonBuffer) readUB4At(offset int) (common.UB4, error) {
	if err := b.ensureRange(offset, osonUB4Size, "osonBuffer.readUB4At"); err != nil {
		return 0, err
	}
	return common.UB4(binary.BigEndian.Uint32(b.data[offset:])), nil
}

// readSB2At reads a signed 2-byte big-endian value at an absolute offset.
func (b *osonBuffer) readSB2At(offset int) (common.SB2, error) {
	if err := b.ensureRange(offset, osonUB2Size, "osonBuffer.readSB2At"); err != nil {
		return 0, err
	}
	return common.SB2(int16(binary.BigEndian.Uint16(b.data[offset:]))), nil
}

// readSB4At reads a signed 4-byte big-endian value at an absolute offset.
func (b *osonBuffer) readSB4At(offset int) (common.SB4, error) {
	if err := b.ensureRange(offset, osonUB4Size, "osonBuffer.readSB4At"); err != nil {
		return 0, err
	}
	return common.SB4(int32(binary.BigEndian.Uint32(b.data[offset:]))), nil
}

// ensureAvailable validates a cursor-based read before it changes pos.
func (b *osonBuffer) ensureAvailable(length int) error {
	if length < 0 {
		cause := fmt.Errorf("negative length %d requested", length)
		common.Odl.Error("osonBuffer.ensureAvailable: failed", "error", cause, "length", length, "remaining", b.remaining())
		return common.NewOracleError(common.OsonBufferError, cause)
	}
	if b.pos < 0 || b.pos > len(b.data) {
		cause := fmt.Errorf("cursor position %d out of bounds [0,%d]", b.pos, len(b.data))
		common.Odl.Error("osonBuffer.ensureAvailable: failed", "error", cause, "position", b.pos, "limit", len(b.data))
		return common.NewOracleError(common.OsonBufferError, cause)
	}
	remaining := b.remaining()
	if remaining < length {
		cause := fmt.Errorf("buffer underflow, need %d bytes, available %d", length, remaining)
		common.Odl.Error("osonBuffer.ensureAvailable: failed", "error", cause, "required", length, "remaining", remaining)
		return common.NewOracleError(common.OsonBufferError, cause)
	}
	return nil
}

// ensureRange validates an absolute range before direct indexing.
func (b *osonBuffer) ensureRange(offset, length int, stage string) error {
	if offset < 0 {
		cause := fmt.Errorf("negative offset %d requested", offset)
		common.Odl.Error(stage+": failed", "error", cause, "offset", offset, "length", length, "limit", len(b.data))
		return common.NewOracleError(common.OsonBufferError, cause)
	}

	if length < 0 {
		cause := fmt.Errorf("negative length %d requested", length)
		common.Odl.Error(stage+": failed", "error", cause, "offset", offset, "length", length, "limit", len(b.data))
		return common.NewOracleError(common.OsonBufferError, cause)
	}

	// `length > len(data)-offset` keeps the bounds check overflow-safe for large offsets.
	if offset > len(b.data) || length > len(b.data)-offset {
		cause := fmt.Errorf("buffer range [%d,%d) out of bounds [0,%d)", offset, offset+length, len(b.data))
		common.Odl.Error(stage+": failed", "error", cause, "offset", offset, "length", length, "limit", len(b.data))
		return common.NewOracleError(common.OsonBufferError, cause)
	}
	return nil
}

// advance reserves length bytes from the cursor and returns the starting offset.
func (b *osonBuffer) advance(length int) (int, error) {
	if err := b.ensureAvailable(length); err != nil {
		return 0, err
	}

	start := b.pos
	b.pos = start + length
	return start, nil
}

// readUint8 reads one raw byte using the moving cursor.
func (b *osonBuffer) readUint8() (uint8, error) {
	start, err := b.advance(osonUB1Size)
	if err != nil {
		return 0, err
	}
	value := b.data[start]
	return value, nil
}

// readUint16 reads one big-endian uint16 using the moving cursor.
func (b *osonBuffer) readUint16() (uint16, error) {
	start, err := b.advance(osonUB2Size)
	if err != nil {
		return 0, err
	}
	value := binary.BigEndian.Uint16(b.data[start:])
	return value, nil
}

// readUint32 reads one big-endian uint32 using the moving cursor.
func (b *osonBuffer) readUint32() (uint32, error) {
	start, err := b.advance(osonUB4Size)
	if err != nil {
		return 0, err
	}
	value := binary.BigEndian.Uint32(b.data[start:])
	return value, nil
}
