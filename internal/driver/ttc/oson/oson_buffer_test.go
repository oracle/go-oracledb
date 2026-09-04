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
	"reflect"
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestOsonBuffer_NewBufferInitialState verifies that a new osonBuffer starts at the
// beginning of the supplied document.
func TestOsonBuffer_NewBufferInitialState(t *testing.T) {
	buffer := newOsonBuffer(drvCommon.B1Array{0x01, 0x02, 0x03})

	if got, want := buffer.position(), 0; got != want {
		t.Fatalf("position = %d, want %d", got, want)
	}
	if got, want := buffer.remaining(), 3; got != want {
		t.Fatalf("remaining = %d, want %d", got, want)
	}
	if got, want := buffer.size(), 3; got != want {
		t.Fatalf("size = %d, want %d", got, want)
	}
}

// TestOsonBuffer_RejectsInvalidInternalCursor verifies reads remain safe if a
// caller corrupts the internal cursor position.
func TestOsonBuffer_RejectsInvalidInternalCursor(t *testing.T) {
	for _, test := range []struct {
		name     string
		position int
	}{
		{name: "negative", position: -1},
		{name: "past end", position: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			buffer := newOsonBuffer(drvCommon.B1Array{0x01, 0x02, 0x03})
			buffer.pos = test.position
			if got := buffer.remaining(); got != 0 {
				t.Fatalf("remaining() = %d, want 0", got)
			}
			if err := buffer.ensureAvailable(1); err == nil {
				t.Fatal("ensureAvailable() error = nil, want invalid cursor failure")
			} else {
				assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
			}
		})
	}

	for _, test := range []struct {
		data drvCommon.B1Array
		read func(*osonBuffer) error
	}{
		{
			data: nil,
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readUB1()
				return err
			},
		},
		{
			data: drvCommon.B1Array{0x01, 0x02, 0x03},
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readUB4()
				return err
			},
		},
	} {
		buffer := newOsonBuffer(test.data)
		if err := test.read(buffer); err == nil {
			t.Fatal("sequential read error = nil, want underflow")
		} else {
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
		}
	}
}

// TestOsonBuffer_SetPositionValidatesBounds verifies valid boundary positions
// and preserves the cursor when an invalid position is rejected.
func TestOsonBuffer_SetPositionValidatesBounds(t *testing.T) {
	buffer := newOsonBuffer(drvCommon.B1Array{0x01, 0x02, 0x03})

	for _, pos := range []int{0, len(buffer.data)} {
		if err := buffer.setPosition(pos); err != nil {
			t.Fatalf("setPosition(%d) error = %v", pos, err)
		}
	}

	if err := buffer.setPosition(1); err != nil {
		t.Fatalf("setPosition(1) error = %v", err)
	}
	for _, pos := range []int{-1, len(buffer.data) + 1} {
		if err := buffer.setPosition(pos); err == nil {
			t.Fatalf("setPosition(%d) expected error", pos)
		}
		if got, want := buffer.position(), 1; got != want {
			t.Fatalf("position after setPosition(%d) = %d, want %d", pos, got, want)
		}
	}
}

// TestOsonBuffer_ReadsSequentialValues verifies cursor-based reads decode
// big-endian values and advance the cursor by the consumed width.
func TestOsonBuffer_ReadsSequentialValues(t *testing.T) {
	cases := []struct {
		name    string
		data    drvCommon.B1Array
		read    func(*osonBuffer) (any, error)
		want    any
		wantPos int
	}{
		{
			name:    "readSlice",
			data:    drvCommon.B1Array{0x01, 0x02},
			read:    func(b *osonBuffer) (any, error) { return b.readSlice(2) },
			want:    drvCommon.B1Array{0x01, 0x02},
			wantPos: 2,
		},
		{
			name:    "readUB1",
			data:    drvCommon.B1Array{0x7f},
			read:    func(b *osonBuffer) (any, error) { return b.readUB1() },
			want:    drvCommon.UB1(0x7f),
			wantPos: 1,
		},
		{
			name:    "readUB2",
			data:    drvCommon.B1Array{0x01, 0x02},
			read:    func(b *osonBuffer) (any, error) { return b.readUB2() },
			want:    drvCommon.UB2(0x0102),
			wantPos: 2,
		},
		{
			name:    "readUB4",
			data:    drvCommon.B1Array{0x01, 0x02, 0x03, 0x04},
			read:    func(b *osonBuffer) (any, error) { return b.readUB4() },
			want:    drvCommon.UB4(0x01020304),
			wantPos: 4,
		},
		{
			name: "readUB1ThenUB2",
			data: drvCommon.B1Array{0x7f, 0x01, 0x02},
			read: func(b *osonBuffer) (any, error) {
				first, err := b.readUB1()
				if err != nil {
					return nil, err
				}
				second, err := b.readUB2()
				if err != nil {
					return nil, err
				}
				return []any{first, second}, nil
			},
			want:    []any{drvCommon.UB1(0x7f), drvCommon.UB2(0x0102)},
			wantPos: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buffer := newOsonBuffer(tc.data)

			got, err := tc.read(buffer)
			if err != nil {
				t.Fatalf("%s returned error: %v", tc.name, err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s value = %#v, want %#v", tc.name, got, tc.want)
			}

			if got, want := buffer.position(), tc.wantPos; got != want {
				t.Fatalf("%s position = %d, want %d", tc.name, got, want)
			}
		})
	}
}

// TestOsonBuffer_AbsoluteReads verifies absolute reads decode values without
// changing the sequential cursor.
func TestOsonBuffer_AbsoluteReads(t *testing.T) {
	cases := []struct {
		name string
		data drvCommon.B1Array
		read func(*osonBuffer) (any, error)
		want any
	}{
		{
			name: "readSliceAt",
			data: drvCommon.B1Array{0x10, 0x20, 0x30, 0x40},
			read: func(b *osonBuffer) (any, error) { return b.readSliceAt(1, 2) },
			want: drvCommon.B1Array{0x20, 0x30},
		},
		{
			name: "readUB1At",
			data: drvCommon.B1Array{0x10, 0x20},
			read: func(b *osonBuffer) (any, error) { return b.readUB1At(1) },
			want: drvCommon.UB1(0x20),
		},
		{
			name: "readUB2At",
			data: drvCommon.B1Array{0x10, 0x20, 0x30},
			read: func(b *osonBuffer) (any, error) { return b.readUB2At(1) },
			want: drvCommon.UB2(0x2030),
		},
		{
			name: "readUB4At",
			data: drvCommon.B1Array{0x01, 0x02, 0x03, 0x04, 0x05},
			read: func(b *osonBuffer) (any, error) { return b.readUB4At(1) },
			want: drvCommon.UB4(0x02030405),
		},
		{
			name: "readSB2At",
			data: drvCommon.B1Array{0x00, 0xff, 0xfe},
			read: func(b *osonBuffer) (any, error) { return b.readSB2At(1) },
			want: drvCommon.SB2(-2),
		},
		{
			name: "readSB4At",
			data: drvCommon.B1Array{0x00, 0xff, 0xff, 0xff, 0xfe},
			read: func(b *osonBuffer) (any, error) { return b.readSB4At(1) },
			want: drvCommon.SB4(-2),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buffer := newOsonBuffer(tc.data)
			buffer.pos = 1

			got, err := tc.read(buffer)
			if err != nil {
				t.Fatalf("%s returned error: %v", tc.name, err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s value = %#v, want %#v", tc.name, got, tc.want)
			}

			if got, want := buffer.position(), 1; got != want {
				t.Fatalf("%s changed position to %d, want %d", tc.name, got, want)
			}
		})
	}
}

// TestOsonBuffer_RejectsInvalidSequentialReads verifies invalid cursor-based
// reads return OSON buffer errors without consuming bytes.
func TestOsonBuffer_RejectsInvalidSequentialReads(t *testing.T) {
	tests := []struct {
		name string
		read func(*osonBuffer) error
	}{
		{
			name: "negative advance",
			read: func(b *osonBuffer) error {
				_, err := b.advance(-1)
				return err
			},
		},
		{
			name: "underflow ub2",
			read: func(b *osonBuffer) error {
				_, err := b.readUB2()
				return err
			},
		},
		{
			name: "underflow slice",
			read: func(b *osonBuffer) error {
				_, err := b.readSlice(4)
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buffer := newOsonBuffer(drvCommon.B1Array{0x01})
			err := tc.read(buffer)
			if err == nil {
				t.Fatal("read error = nil, want failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
			if got, want := buffer.position(), 0; got != want {
				t.Fatalf("position after failed read = %d, want %d", got, want)
			}
		})
	}
}

// TestOsonBuffer_RejectsInvalidAbsoluteRanges verifies invalid absolute reads
// return OSON buffer errors without changing the sequential cursor.
func TestOsonBuffer_RejectsInvalidAbsoluteRanges(t *testing.T) {
	tests := []struct {
		name string
		read func(*osonBuffer) error
	}{
		{
			name: "negative offset",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readSliceAt(-1, 1)
				return err
			},
		},
		{
			name: "negative length",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readSliceAt(0, -1)
				return err
			},
		},
		{
			name: "offset past limit",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readUB1At(4)
				return err
			},
		},
		{
			name: "ub4 underflow",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readUB4At(0)
				return err
			},
		},
		{
			name: "sb2 underflow",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readSB2At(2)
				return err
			},
		},
		{
			name: "sb4 underflow",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readSB4At(0)
				return err
			},
		},
		{
			name: "slice underflow",
			read: func(buffer *osonBuffer) error {
				_, err := buffer.readSliceAt(1, 3)
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buffer := newOsonBuffer(drvCommon.B1Array{0x01, 0x02, 0x03})
			if err := buffer.setPosition(1); err != nil {
				t.Fatalf("setPosition(1) error = %v", err)
			}

			err := tc.read(buffer)
			if err == nil {
				t.Fatal("absolute read error = nil, want failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
			if got, want := buffer.position(), 1; got != want {
				t.Fatalf("position after failed absolute read = %d, want %d", got, want)
			}
		})
	}
}
