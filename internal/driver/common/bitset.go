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

// BitSet efficiently stores a collection of boolean flags using bits packed into bytes.
// It is suitable for protocol bitfield representations, column/capability presence, and dense on-off flag arrays.
// This type is thread-unsafe and intended for local, protocol, or container use.
type BitSet struct {
	bits []byte
}

// NewBitSet returns a new BitSet that can hold at least size bits (index 0..size-1).
func NewBitSet(size int) *BitSet {
	return &BitSet{bits: make([]byte, (size+7)/8)}
}

// SetBytes copies data into the bits slice starting at the given byte index.
// If data exceeded bounds, it copies as much as will fit.
func (bs *BitSet) SetBytes(start int, data []byte) {
	if start < 0 || start >= len(bs.bits) {
		return
	}
	n := copy(bs.bits[start:], data)
	_ = n // Copy operation suffices; nothing to return
}

// Get returns true if the bit at the given index is set.
func (bs *BitSet) Get(index int) bool {
	byteIdx, bitMask := index/8, byte(1<<(index%8))
	return bs.bits[byteIdx]&bitMask != 0
}

// ClearAll resets all bits to 0 (false).
func (bs *BitSet) ClearAll() {
	for i := range bs.bits {
		bs.bits[i] = 0
	}
}

// Length returns the maximum number of bits addressable in this BitSet.
func (bs *BitSet) Length() int {
	return len(bs.bits) * 8
}

// Cardinality returns the number of bits set to 1 in the BitSet.
func (bs *BitSet) Cardinality() int {
	count := 0
	for i := 0; i < bs.Length(); i++ {
		if bs.Get(i) {
			count++
		}
	}
	return count
}

// String returns a compact bit representation (little-endian by byte, MSB first in each byte).
func (bs *BitSet) String() string {
	res := make([]byte, 0, len(bs.bits)*8)
	for i := 0; i < len(bs.bits); i++ {
		for bit := 7; bit >= 0; bit-- {
			if bs.bits[i]&(1<<uint(bit)) != 0 {
				res = append(res, '1')
			} else {
				res = append(res, '0')
			}
		}
	}
	return string(res)
}
