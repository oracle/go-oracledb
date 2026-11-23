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

// TestReadBuffer verifies byte order, sequential reads, the cursor, and ReadBytes copies.
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
		s := r.(Seekable)
		if r.Size() != 4 || s.Position() != 0 {
			t.Fatalf("Size() = %d, Position() = %d; want 4, 0", r.Size(), s.Position())
		}
		if got, err := r.ReadUB2(); err != nil || got != 0x0708 {
			t.Fatalf("ReadUB2() = %#x, %v; want 0x0708", got, err)
		}
		if s.Position() != 2 {
			t.Fatalf("Position() = %d after ReadUB2(); want 2", s.Position())
		}
		if err := s.Seek(-1); err != nil {
			t.Fatal(err)
		}
		if got, err := r.ReadUB1(); err != nil || got != 8 {
			t.Fatalf("ReadUB1() after Seek(1) = %d, %v; want 8", got, err)
		}
		if err := s.Seek(2); err != nil {
			t.Fatal(err)
		}
		if s.Position() != 4 {
			t.Fatalf("Position() = %d after seeking forward; want 4", s.Position())
		}
	})

	t.Run("ReadBytes returns a copy and advances", func(t *testing.T) {
		data := B1Array{1, 2, 3}
		r := NewReadBuffer(data, BIG_ENDIAN)
		s := r.(Seekable)
		view, err := r.ReadBytes(2)
		if err != nil || !bytes.Equal(view, B1Array{1, 2}) {
			t.Fatalf("ReadBytes(2) = %v, %v; want [1 2]", view, err)
		}
		view[0] = 9
		if data[0] != 1 {
			t.Fatal("ReadBytes() returned a slice sharing the buffer data")
		}
		data[1] = 9
		if view[1] != 2 {
			t.Fatal("ReadBytes() result changed when the buffer data changed")
		}
		if s.Position() != 2 {
			t.Fatalf("Position() = %d after ReadBytes(2); want 2", s.Position())
		}
	})

}

// TestWriteBuffer verifies writes in both byte orders, growth,
// independent copies from Bytes, Reserve, and cursor-based overwrites.
func TestWriteBuffer(t *testing.T) {
	t.Run("round trip in both byte orders", func(t *testing.T) {
		for _, order := range []ByteOrder{BIG_ENDIAN, LITTLE_ENDIAN} {
			w := NewGrowableWriteBuffer(order)
			if err := w.WriteUB1(0xAB); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteUB2(0x1234); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteUB4(0x12345678); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteUB8(0x0123456789abcdef); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteSB1(-1); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteSB2(-2); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteSB4(-3); err != nil {
				t.Fatal(err)
			}

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
		w := NewGrowableWriteBuffer(BIG_ENDIAN)
		s := w.(Seekable)
		if s.Position() != 0 || w.Size() != 0 {
			t.Fatalf("new buffer Position() = %d, Size() = %d; want 0, 0", s.Position(), w.Size())
		}
		if err := w.WriteUB1(1); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteUB2(0x0203); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteBytes(B1Array{4}); err != nil {
			t.Fatal(err)
		}
		if s.Position() != 4 || w.Size() != 4 {
			t.Fatalf("Position() = %d, Size() = %d; want 4, 4", s.Position(), w.Size())
		}
		if !bytes.Equal(w.Bytes(), B1Array{1, 2, 3, 4}) {
			t.Fatalf("Bytes() = %v; want [1 2 3 4]", w.Bytes())
		}
	})

	t.Run("growth beyond preallocated capacity", func(t *testing.T) {
		w := NewGrowableWriteBufferWithCapacity(BIG_ENDIAN, 1)
		if err := w.WriteUB4(0x01020304); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(w.Bytes(), B1Array{1, 2, 3, 4}) {
			t.Fatalf("Bytes() = %v; want [1 2 3 4]", w.Bytes())
		}
	})

	t.Run("Bytes returns an independent copy", func(t *testing.T) {
		w := NewGrowableWriteBuffer(BIG_ENDIAN)
		if err := w.WriteUB1(5); err != nil {
			t.Fatal(err)
		}
		copied := w.Bytes()
		copied[0] = 9
		if w.Bytes()[0] != 5 {
			t.Fatal("Bytes() returned a slice sharing the buffer data")
		}
	})

	t.Run("Bytes is non-nil when empty", func(t *testing.T) {
		if b := NewGrowableWriteBuffer(BIG_ENDIAN).Bytes(); b == nil || len(b) != 0 {
			t.Fatalf("Bytes() = %#v; want non-nil empty slice", b)
		}
	})

	t.Run("Reserve", func(t *testing.T) {
		w := NewGrowableWriteBuffer(BIG_ENDIAN)
		if err := w.WriteUB1(1); err != nil {
			t.Fatal(err)
		}
		if got, err := w.Reserve(0); err != nil || got != 1 || w.Size() != 1 {
			t.Fatalf("Reserve(0) = %d, Size() = %d; want 1, 1", got, w.Size())
		}
		if got, err := w.Reserve(2); err != nil || got != 1 {
			t.Fatalf("Reserve(2) = %d; want 1", got)
		}
		if got := w.Bytes(); len(got) != 3 || got[0] != 1 {
			t.Fatalf("Bytes() = %v; want length 3 and first byte 1", got)
		}
		if _, err := w.Reserve(-1); err == nil {
			t.Fatal("Reserve(-1) succeeded")
		} else {
			assertInvalidBufferOperation(t, err)
		}
		if w.Size() != 3 {
			t.Fatalf("Size() after failed Reserve = %d; want 3", w.Size())
		}
	})

	t.Run("seek, overwrite, and extend", func(t *testing.T) {
		w := NewGrowableWriteBuffer(BIG_ENDIAN)
		s := w.(Seekable)
		if err := w.WriteBytes(B1Array{1, 2, 3, 4}); err != nil {
			t.Fatal(err)
		}
		s.Set()
		if err := s.Seek(-3); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteUB2(0x0908); err != nil {
			t.Fatal(err)
		}
		if w.Size() != 4 || s.Position() != 3 {
			t.Fatalf("size = %d, position = %d", w.Size(), s.Position())
		}
		s.Rewind()
		if err := w.WriteUB1(5); err != nil {
			t.Fatal(err)
		}
		if got := w.Bytes(); !bytes.Equal(got, B1Array{1, 9, 8, 4, 5}) {
			t.Fatalf("Bytes() = %v", got)
		}
		if err := s.Seek(-2); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Reserve(3); err != nil {
			t.Fatal(err)
		}
		if got := w.Bytes(); len(got) != 6 || !bytes.Equal(got[:5], B1Array{1, 9, 8, 4, 5}) {
			t.Fatalf("Bytes() after Reserve = %v; want length 6 and existing bytes unchanged", got)
		}
	})

}

// TestInvalidBufferOperation verifies that every read or write whose position, offset, or range
// falls outside the data fails with InvalidBufferOperation, and that failed reads leave the cursor unchanged.
func TestInvalidBufferOperation(t *testing.T) {
	t.Run("reads", func(t *testing.T) {
		r := NewReadBuffer(B1Array{1, 2}, BIG_ENDIAN)
		s := r.(Seekable)
		if _, err := r.ReadUB2(); err != nil {
			t.Fatal(err)
		}
		maxInt := int(^uint(0) >> 1)
		minInt := -maxInt - 1
		reads := []struct {
			name string
			read func() error
		}{
			{"Seek past size", func() error { return s.Seek(3) }},
			{"Seek before zero", func() error { return s.Seek(-3) }},
			{"Seek with maximum offset", func() error { return s.Seek(maxInt) }},
			{"Seek with minimum offset", func() error { return s.Seek(minInt) }},
			{"UB1", func() error { _, err := r.ReadUB1(); return err }},
			{"UB2", func() error { _, err := r.ReadUB2(); return err }},
			{"UB4", func() error { _, err := r.ReadUB4(); return err }},
			{"UB8", func() error { _, err := r.ReadUB8(); return err }},
			{"SB1", func() error { _, err := r.ReadSB1(); return err }},
			{"SB2", func() error { _, err := r.ReadSB2(); return err }},
			{"SB4", func() error { _, err := r.ReadSB4(); return err }},
			{"ReadBytes", func() error { _, err := r.ReadBytes(1); return err }},
			{"ReadBytes negative length", func() error { _, err := r.ReadBytes(-1); return err }},
		}
		for _, rd := range reads {
			t.Run(rd.name, func(t *testing.T) {
				assertInvalidBufferOperation(t, rd.read())
			})
		}
		if s.Position() != 2 {
			t.Fatalf("Position() = %d after failed reads; want 2", s.Position())
		}
	})

	t.Run("seek and saved position replacement", func(t *testing.T) {
		w := NewGrowableWriteBuffer(BIG_ENDIAN)
		if err := w.WriteBytes(B1Array{1, 2}); err != nil {
			t.Fatal(err)
		}
		writeCursor := w.(Seekable)
		if err := writeCursor.Seek(-2); err != nil {
			t.Fatal(err)
		}
		buffers := []Seekable{NewReadBuffer(B1Array{1, 2}, BIG_ENDIAN).(Seekable), writeCursor}
		for _, b := range buffers {
			if err := b.Seek(1); err != nil {
				t.Fatal(err)
			}
			b.Rewind()
			if b.Position() != 0 {
				t.Fatal("rewind before Set did not restore the initial position")
			}
			if err := b.Seek(0); err != nil {
				t.Fatal(err)
			}
			b.Set()
			if err := b.Seek(1); err != nil {
				t.Fatal(err)
			}
			b.Set()
			if err := b.Seek(1); err != nil {
				t.Fatal(err)
			}
			assertInvalidBufferOperation(t, b.Seek(1))
			assertInvalidBufferOperation(t, b.Seek(-3))
			maxInt := int(^uint(0) >> 1)
			minInt := -maxInt - 1
			assertInvalidBufferOperation(t, b.Seek(maxInt))
			assertInvalidBufferOperation(t, b.Seek(minInt))
			if b.Position() != 2 {
				t.Fatal("failed seek changed position")
			}
			b.Rewind()
			if b.Position() != 1 {
				t.Fatalf("rewind position = %d; want 1", b.Position())
			}
			if err := b.Seek(1); err != nil {
				t.Fatal(err)
			}
			b.Rewind()
			if b.Position() != 1 {
				t.Fatal("repeated rewind did not restore the saved position")
			}
		}
	})
}
