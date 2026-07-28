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
	"testing"
)

func TestNewBitSet_SizeAlignment(t *testing.T) {
	t.Parallel()
	// Aligned to byte boundary
	bs := NewBitSet(16)
	if len(bs.bits) != 2 {
		t.Errorf("Expected 2 bytes for 16 bits, got %d", len(bs.bits))
	}
	// Not aligned (22): should be 3 bytes
	bs = NewBitSet(22)
	if len(bs.bits) != 3 {
		t.Errorf("Expected 3 bytes for 22 bits, got %d", len(bs.bits))
	}
	// Zero-sized BitSet
	bs = NewBitSet(0)
	if len(bs.bits) != 0 {
		t.Errorf("Expected 0 bytes for 0 bits, got %d", len(bs.bits))
	}
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(10)
	for i := 0; i < bs.Length(); i++ {
		bs.Set(i, i%2 == 0)
	}
	for i := 0; i < bs.Length(); i++ {
		want := i%2 == 0
		if got := bs.Get(i); got != want {
			t.Errorf("Bit %d: want %v, got %v", i, want, got)
		}
	}
}

func TestSetBytesAndSetByte(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(16)
	data := []byte{0xAA, 0x55}
	bs.SetBytes(0, data)
	if bs.bits[0] != 0xAA || bs.bits[1] != 0x55 {
		t.Errorf("Expected SetBytes to set bits to 0xAA 0x55, got %x %x", bs.bits[0], bs.bits[1])
	}
	// Partial overwrite (should only change 1 byte)
	bs.SetBytes(1, []byte{0xFF})
	if bs.bits[1] != 0xFF {
		t.Errorf("SetBytes(1, [0xFF]): bits[1] = %02x, want 0xFF", bs.bits[1])
	}
	// SetByte (should work and match SetBytes(1, [0xBC]))
	bs.SetByte(1, 0xBC)
	if bs.bits[1] != 0xBC {
		t.Errorf("SetByte(1, 0xBC): bits[1] = %02x, want 0xBC", bs.bits[1])
	}
}

func TestCardinality(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(12)
	// Set even bits
	for i := 0; i < 12; i += 2 {
		bs.Set(i, true)
	}
	if n := bs.Cardinality(); n != 6 {
		t.Errorf("Want 6 bits set, got %d", n)
	}
	// SetBytes with 0xFF
	bs.SetBytes(0, []byte{0xFF, 0x0F})
	if n := bs.Cardinality(); n != 12 {
		t.Errorf("SetBytes should set all bits, got cardinality %d", n)
	}
}

func TestClearAllAndLength(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(9)
	bs.SetBytes(0, []byte{0xFF, 0xFF})
	bs.ClearAll()
	for i := 0; i < 9; i++ {
		if bs.Get(i) {
			t.Errorf("All bits should be cleared. Bit %d is set", i)
		}
	}
	if bs.Length() != 16 {
		t.Errorf("Length for 2 bytes = %d, want 16", bs.Length())
	}
}

func TestStringFormat(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(8)
	bs.SetBytes(0, []byte{0xC3})
	want := "11000011"
	got := bs.String()[0:8]
	if got != want {
		t.Errorf("String: got %q, want %q", got, want)
	}
}

func TestSetBytes_OutOfBounds(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(8)
	// out-of-bounds start
	bs.SetBytes(100, []byte{0xFC})
	// start at 0, length > size
	bs.SetBytes(0, []byte{0xFE, 0xEE, 0xDD})
	if bs.bits[0] != 0xFE {
		t.Errorf("Out-of-bounds SetBytes should only write within bounds, bits[0]=%02x", bs.bits[0])
	}
}

func TestGetSet_OutOfBounds(t *testing.T) {
	t.Parallel()
	bs := NewBitSet(8)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic on out-of-bounds Get/Set")
		}
	}()
	_ = bs.Get(10)
	bs.Set(10, true)
}

func TestNewBitSetFromBytes(t *testing.T) {
	t.Parallel()
	data := []byte{0x12, 0x34}
	bs := NewBitSetFromBytes(data)
	if bs.bits[0] != 0x12 || bs.bits[1] != 0x34 {
		t.Errorf("Expected bits to match input slice, got %x %x", bs.bits[0], bs.bits[1])
	}
	// Should not alias input
	data[0] = 0xFF
	if bs.bits[0] == 0xFF {
		t.Errorf("BitSetFromBytes does not copy input, is aliased")
	}
}
