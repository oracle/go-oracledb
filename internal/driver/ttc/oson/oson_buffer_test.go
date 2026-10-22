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
	"encoding/binary"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"testing"
)

// TestBufferReadsAndBounds verifies buffer reads decode big-endian values and
// respect document bounds.
func TestBufferReadsAndBounds(t *testing.T) {
	buf := newOsonBuffer(drvCommon.B1Array{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0})
	if got, err := buf.readUB1(); err != nil || got != 0x12 {
		t.Fatalf("readUB1() = %#x, %v; want 0x12, nil", got, err)
	}
	if got, err := buf.readUB2(); err != nil || got != 0x3456 {
		t.Fatalf("readUB2() = %#x, %v; want 0x3456, nil", got, err)
	}
	if got, err := buf.readUB4(); err != nil || got != 0x789abcde {
		t.Fatalf("readUB4() = %#x, %v; want 0x789abcde, nil", got, err)
	}
	if got, err := buf.readUB4At(2); err != nil || got != 0x56789abc {
		t.Fatalf("readUB4At() = %#x, %v; want 0x56789abc, nil", got, err)
	}
	if got, err := buf.readSB2At(4); err != nil || got != drvCommon.SB2(int16(binary.BigEndian.Uint16([]byte{0x9a, 0xbc}))) {
		t.Fatalf("readSB2At() = %d, %v; want signed 0x9abc, nil", got, err)
	}
	if got, err := buf.readSB4At(4); err != nil || got != drvCommon.SB4(int32(binary.BigEndian.Uint32([]byte{0x9a, 0xbc, 0xde, 0xf0}))) {
		t.Fatalf("readSB4At() = %d, %v; want signed 0x9abcdef0, nil", got, err)
	}
	if err := buf.setPosition(buf.size()); err != nil {
		t.Fatalf("setPosition(end) error = %v", err)
	}
	if _, err := buf.readUB1(); err == nil {
		t.Fatal("readUB1() at end of input error = nil, want bounds error")
	}
	if _, err := buf.readSliceAt(-1, 1); err == nil {
		t.Fatal("readSliceAt(negative offset) error = nil, want bounds error")
	}
	if err := buf.setPosition(buf.size() + 1); err == nil {
		t.Fatal("setPosition(past end) error = nil, want bounds error")
	}
}
