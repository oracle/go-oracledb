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
	"bytes"
	"errors"
	"math"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// assertInvalidBufferOperation verifies that err carries the InvalidBufferOperation driver code.
func assertInvalidBufferOperation(t *testing.T, err error) {
	t.Helper()
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.InvalidBufferOperation) {
		t.Fatalf("error = %v, want code %s", err, oracleErrors.InvalidBufferOperation)
	}
}

// TestReadBuffer verifies the ReadBuffer contract: byte order, sequential and absolute reads,
// the cursor, and the views returned by ReadBytes and ReadBytesAt.
func TestReadBuffer(t *testing.T) {
	t.Run("byte order", func(t *testing.T) {
		data := B1Array{0x12, 0x34}
		tests := []struct {
			name  string
			order ByteOrder
			want  UB2
		}{
			{"big endian", BIG_ENDIAN, 0x1234},
			{"little endian", LITTLE_ENDIAN, 0x3412},
			{"unknown order", ByteOrder{name: "OTHER"}, 0x1234},
		}
		for _, tt := range tests {
			got, err := NewReadBuffer(data, tt.order).ReadUB2()
			if err != nil || got != tt.want {
				t.Fatalf("%s: ReadUB2() = %#x, %v; want %#x", tt.name, got, err, tt.want)
			}
		}
	})

	t.Run("sequential reads and position", func(t *testing.T) {
		r := NewReadBuffer(B1Array{7, 8, 1, 2}, BIG_ENDIAN)
		if r.Size() != 4 || r.Position() != 0 {
			t.Fatalf("Size() = %d, Position() = %d; want 4, 0", r.Size(), r.Position())
		}
		if got, err := r.ReadUB2(); err != nil || got != 0x0708 {
			t.Fatalf("ReadUB2() = %#x, %v; want 0x0708", got, err)
		}
		if r.Position() != 2 {
			t.Fatalf("Position() = %d after ReadUB2(); want 2", r.Position())
		}
		if err := r.SetPosition(1); err != nil {
			t.Fatal(err)
		}
		if got, err := r.ReadUB1(); err != nil || got != 8 {
			t.Fatalf("ReadUB1() after SetPosition(1) = %d, %v; want 8", got, err)
		}
		if err := r.SetPosition(r.Size()); err != nil {
			t.Fatal(err)
		}
		if r.Position() != 4 {
			t.Fatalf("Position() = %d after SetPosition(Size()); want 4", r.Position())
		}
	})

	t.Run("absolute reads do not move the cursor", func(t *testing.T) {
		r := NewReadBuffer(B1Array{1, 2, 3, 4, 5, 6, 7, 8, 0xFF, 0xFF, 0xFF, 0xFD}, BIG_ENDIAN)
		reads := []struct {
			name string
			read func() (any, error)
			want any
		}{
			{"UB1At", func() (any, error) { return r.ReadUB1At(0) }, UB1(1)},
			{"UB2At", func() (any, error) { return r.ReadUB2At(1) }, UB2(0x0203)},
			{"UB4At", func() (any, error) { return r.ReadUB4At(2) }, UB4(0x03040506)},
			{"UB8At", func() (any, error) { return r.ReadUB8At(0) }, UB8(0x0102030405060708)},
			{"SB1At", func() (any, error) { return r.ReadSB1At(8) }, SB1(-1)},
			{"SB2At", func() (any, error) { return r.ReadSB2At(10) }, SB2(-3)},
			{"SB4At", func() (any, error) { return r.ReadSB4At(8) }, SB4(-3)},
		}
		for _, rd := range reads {
			got, err := rd.read()
			if err != nil || got != rd.want {
				t.Fatalf("%s = %v, %v; want %v", rd.name, got, err, rd.want)
			}
		}
		if r.Position() != 0 {
			t.Fatalf("Position() = %d after absolute reads; want 0", r.Position())
		}
	})

	t.Run("ReadBytes returns an aliased view and advances", func(t *testing.T) {
		data := B1Array{1, 2, 3}
		r := NewReadBuffer(data, BIG_ENDIAN)
		view, err := r.ReadBytes(2)
		if err != nil || !bytes.Equal(view, B1Array{1, 2}) {
			t.Fatalf("ReadBytes(2) = %v, %v; want [1 2]", view, err)
		}
		view[0] = 9
		if data[0] != 9 {
			t.Fatal("ReadBytes() did not return a view aliasing the buffer data")
		}
		if r.Position() != 2 {
			t.Fatalf("Position() = %d after ReadBytes(2); want 2", r.Position())
		}
	})

	t.Run("ReadBytesAt returns the range without moving the cursor", func(t *testing.T) {
		r := NewReadBuffer(B1Array{1, 2, 3, 4}, BIG_ENDIAN)
		got, err := r.ReadBytesAt(1, 2)
		if err != nil || !bytes.Equal(got, B1Array{2, 3}) {
			t.Fatalf("ReadBytesAt(1, 2) = %v, %v; want [2 3]", got, err)
		}
		if r.Position() != 0 {
			t.Fatalf("Position() = %d after ReadBytesAt(); want 0", r.Position())
		}
	})
}

// TestWriteBuffer verifies the WriteBuffer contract: appends in both byte orders, growth,
// independent copies from Bytes, Reserve, and in-place overwrites with Write*At.
func TestWriteBuffer(t *testing.T) {
	t.Run("round trip in both byte orders", func(t *testing.T) {
		for _, order := range []ByteOrder{BIG_ENDIAN, LITTLE_ENDIAN} {
			w := NewWriteBuffer(order)
			w.WriteUB1(0xAB)
			w.WriteUB2(0x1234)
			w.WriteUB4(0x12345678)
			w.WriteUB8(0x0123456789abcdef)
			w.WriteSB1(-1)
			w.WriteSB2(-2)
			w.WriteSB4(-3)

			r := NewReadBuffer(w.Bytes(), order)
			reads := []struct {
				name string
				read func() (any, error)
				want any
			}{
				{"UB1", func() (any, error) { return r.ReadUB1() }, UB1(0xAB)},
				{"UB2", func() (any, error) { return r.ReadUB2() }, UB2(0x1234)},
				{"UB4", func() (any, error) { return r.ReadUB4() }, UB4(0x12345678)},
				{"UB8", func() (any, error) { return r.ReadUB8() }, UB8(0x0123456789abcdef)},
				{"SB1", func() (any, error) { return r.ReadSB1() }, SB1(-1)},
				{"SB2", func() (any, error) { return r.ReadSB2() }, SB2(-2)},
				{"SB4", func() (any, error) { return r.ReadSB4() }, SB4(-3)},
			}
			for _, rd := range reads {
				got, err := rd.read()
				if err != nil || got != rd.want {
					t.Fatalf("%s: %s = %v, %v; want %v", order.name, rd.name, got, err, rd.want)
				}
			}
		}
	})

	t.Run("appends advance position and size", func(t *testing.T) {
		w := NewWriteBuffer(BIG_ENDIAN)
		if w.Position() != 0 || w.Size() != 0 {
			t.Fatalf("new buffer Position() = %d, Size() = %d; want 0, 0", w.Position(), w.Size())
		}
		w.WriteUB1(1)
		w.WriteUB2(0x0203)
		w.WriteBytes(B1Array{4})
		if w.Position() != 4 || w.Size() != 4 {
			t.Fatalf("Position() = %d, Size() = %d; want 4, 4", w.Position(), w.Size())
		}
		if !bytes.Equal(w.Bytes(), B1Array{1, 2, 3, 4}) {
			t.Fatalf("Bytes() = %v; want [1 2 3 4]", w.Bytes())
		}
	})

	t.Run("growth beyond preallocated capacity", func(t *testing.T) {
		w := NewWriteBufferWithCapacity(1, BIG_ENDIAN)
		w.WriteUB4(0x01020304)
		if !bytes.Equal(w.Bytes(), B1Array{1, 2, 3, 4}) {
			t.Fatalf("Bytes() = %v; want [1 2 3 4]", w.Bytes())
		}
	})

	t.Run("Bytes returns an independent copy", func(t *testing.T) {
		w := NewWriteBuffer(BIG_ENDIAN)
		w.WriteUB1(5)
		copied := w.Bytes()
		copied[0] = 9
		if w.Bytes()[0] != 5 {
			t.Fatal("Bytes() returned a slice sharing the buffer data")
		}
	})

	t.Run("Bytes is non-nil when empty", func(t *testing.T) {
		if b := NewWriteBuffer(BIG_ENDIAN).Bytes(); b == nil || len(b) != 0 {
			t.Fatalf("Bytes() = %#v; want non-nil empty slice", b)
		}
	})

	t.Run("Reserve", func(t *testing.T) {
		w := NewWriteBuffer(BIG_ENDIAN)
		w.WriteUB1(1)
		if got := w.Reserve(0); got != 1 || w.Size() != 1 {
			t.Fatalf("Reserve(0) = %d, Size() = %d; want 1, 1", got, w.Size())
		}
		if got := w.Reserve(2); got != 1 {
			t.Fatalf("Reserve(2) = %d; want 1", got)
		}
		if !bytes.Equal(w.Bytes(), B1Array{1, 0, 0}) {
			t.Fatalf("Bytes() = %v; want [1 0 0]", w.Bytes())
		}
	})

	t.Run("Write*At overwrites without moving position", func(t *testing.T) {
		tests := []struct {
			name  string
			write func(w WriteBuffer) error
			want  B1Array
		}{
			{"UB1At", func(w WriteBuffer) error { return w.WriteUB1At(0, 0xAB) }, B1Array{0xAB, 0, 0, 0, 0, 0, 0, 0}},
			{"UB2At", func(w WriteBuffer) error { return w.WriteUB2At(1, 0x1234) }, B1Array{0, 0x12, 0x34, 0, 0, 0, 0, 0}},
			{"UB4At", func(w WriteBuffer) error { return w.WriteUB4At(0, 0x01020304) }, B1Array{1, 2, 3, 4, 0, 0, 0, 0}},
			{"UB8At", func(w WriteBuffer) error { return w.WriteUB8At(0, 0x0102030405060708) }, B1Array{1, 2, 3, 4, 5, 6, 7, 8}},
			{"SB1At", func(w WriteBuffer) error { return w.WriteSB1At(7, -1) }, B1Array{0, 0, 0, 0, 0, 0, 0, 0xFF}},
			{"SB2At", func(w WriteBuffer) error { return w.WriteSB2At(6, -2) }, B1Array{0, 0, 0, 0, 0, 0, 0xFF, 0xFE}},
			{"SB4At", func(w WriteBuffer) error { return w.WriteSB4At(4, -3) }, B1Array{0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFD}},
			{"BytesAt", func(w WriteBuffer) error { return w.WriteBytesAt(2, B1Array{9, 8}) }, B1Array{0, 0, 9, 8, 0, 0, 0, 0}},
		}
		for _, tt := range tests {
			w := NewWriteBuffer(BIG_ENDIAN)
			w.Reserve(8)
			if err := tt.write(w); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if w.Position() != 8 {
				t.Fatalf("%s: Position() = %d; want 8", tt.name, w.Position())
			}
			if !bytes.Equal(w.Bytes(), tt.want) {
				t.Fatalf("%s: Bytes() = %v; want %v", tt.name, w.Bytes(), tt.want)
			}
		}
	})
}

// TestInvalidBufferOperation verifies that every read or write whose position, offset, or range
// falls outside the data fails with InvalidBufferOperation, and that failed reads leave the cursor unchanged.
func TestInvalidBufferOperation(t *testing.T) {
	t.Run("reads", func(t *testing.T) {
		r := NewReadBuffer(B1Array{1, 2}, BIG_ENDIAN)
		if _, err := r.ReadUB2(); err != nil {
			t.Fatal(err)
		}
		reads := []struct {
			name string
			read func() error
		}{
			{"SetPosition past size", func() error { return r.SetPosition(3) }},
			{"UB1", func() error { _, err := r.ReadUB1(); return err }},
			{"UB2", func() error { _, err := r.ReadUB2(); return err }},
			{"UB4", func() error { _, err := r.ReadUB4(); return err }},
			{"UB8", func() error { _, err := r.ReadUB8(); return err }},
			{"SB1", func() error { _, err := r.ReadSB1(); return err }},
			{"SB2", func() error { _, err := r.ReadSB2(); return err }},
			{"SB4", func() error { _, err := r.ReadSB4(); return err }},
			{"ReadBytes", func() error { _, err := r.ReadBytes(1); return err }},
			{"UB1At offset past size", func() error { _, err := r.ReadUB1At(3); return err }},
			{"UB1At range past end", func() error { _, err := r.ReadUB1At(2); return err }},
			{"UB2At range past end", func() error { _, err := r.ReadUB2At(2); return err }},
			{"UB4At range past end", func() error { _, err := r.ReadUB4At(2); return err }},
			{"UB8At range past end", func() error { _, err := r.ReadUB8At(2); return err }},
			{"SB1At range past end", func() error { _, err := r.ReadSB1At(2); return err }},
			{"SB2At range past end", func() error { _, err := r.ReadSB2At(2); return err }},
			{"SB4At range past end", func() error { _, err := r.ReadSB4At(2); return err }},
			{"ReadBytesAt range past end", func() error { _, err := r.ReadBytesAt(2, 1); return err }},
		}
		for _, rd := range reads {
			t.Run(rd.name, func(t *testing.T) {
				assertInvalidBufferOperation(t, rd.read())
			})
		}
		if r.Position() != 2 {
			t.Fatalf("Position() = %d after failed reads; want 2", r.Position())
		}
	})

	t.Run("writes", func(t *testing.T) {
		tests := []struct {
			name  string
			write func(w WriteBuffer) error
		}{
			{"offset past end", func(w WriteBuffer) error { return w.WriteUB1At(math.MaxUint, 0) }},
			{"UB1At", func(w WriteBuffer) error { return w.WriteUB1At(1, 0) }},
			{"UB2At", func(w WriteBuffer) error { return w.WriteUB2At(1, 0) }},
			{"UB4At", func(w WriteBuffer) error { return w.WriteUB4At(1, 0) }},
			{"UB8At", func(w WriteBuffer) error { return w.WriteUB8At(1, 0) }},
			{"SB1At", func(w WriteBuffer) error { return w.WriteSB1At(1, 0) }},
			{"SB2At", func(w WriteBuffer) error { return w.WriteSB2At(1, 0) }},
			{"SB4At", func(w WriteBuffer) error { return w.WriteSB4At(1, 0) }},
			{"BytesAt", func(w WriteBuffer) error { return w.WriteBytesAt(1, B1Array{0, 0}) }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := NewWriteBuffer(BIG_ENDIAN)
				w.WriteUB1(1)
				assertInvalidBufferOperation(t, tt.write(w))
			})
		}
	})
}
