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

package ttc

import (
	"bytes"
	"context"
	"errors"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type occaHeaderStub struct {
	payload []byte
	err     error
}

func (h *occaHeaderStub) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	if h.err != nil {
		return h.err
	}
	return engine.MarshalB1Array(ctx, h.payload)
}

func TestOcca_New_Getters(t *testing.T) {
	t.Parallel()

	msg18 := newOcca18()
	if msg18 == nil {
		t.Fatal("newOcca18 returned nil")
	}
	if msg18.GetMsgCode() != TTIPFN {
		t.Fatalf("expected TTIPFN, got %v", msg18.GetMsgCode())
	}
	if msg18.(interface {
		GetFuncCode() driverCommon.FunctionType
	}).GetFuncCode() != occa {
		t.Fatalf("expected occa function code, got %v", msg18.(interface {
			GetFuncCode() driverCommon.FunctionType
		}).GetFuncCode())
	}
	if _, ok := msg18.(*tTIOcca).headerMarshaller.(*ttiFunHeader18); !ok {
		t.Fatal("expected newOcca18 to use the 18+ function header")
	}

	msg := newOcca()
	if msg == nil {
		t.Fatal("newOcca returned nil")
	}
	if msg.GetMsgCode() != TTIPFN {
		t.Fatalf("expected TTIPFN, got %v", msg.GetMsgCode())
	}
	if _, ok := msg.(*tTIOcca).headerMarshaller.(*ttiFunHeader); !ok {
		t.Fatal("expected newOcca to use the legacy function header")
	}
}

func TestOcca_SetCursorIDs(t *testing.T) {
	t.Parallel()

	m := newOcca().(*tTIOcca)
	want := []driverCommon.UB4{1, 7, 99}
	m.setCursorIDs(want)

	if len(m.cursorIDs) != len(want) {
		t.Fatalf("expected %d cursor IDs, got %d", len(want), len(m.cursorIDs))
	}
	for i, id := range want {
		if m.cursorIDs[i] != id {
			t.Fatalf("expected cursor ID %d at position %d, got %d", id, i, m.cursorIDs[i])
		}
	}
}

func TestOcca_MarshalTo_Success(t *testing.T) {
	t.Parallel()

	m := &tTIOcca{
		headerMarshaller: &occaHeaderStub{payload: []byte{0xAA, 0xBB}},
		cursorIDs:        []driverCommon.UB4{5, 9},
	}

	buf := NewArrayDataBuffer(64)
	engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	want := []byte{
		0xAA, 0xBB,
		0x01,       // PTR sentinel
		0x01, 0x02, // offset = 2
		0x01, 0x05, // cursor 5
		0x01, 0x09, // cursor 9
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected OCCA payload:\n got: % X\nwant: % X", got, want)
	}
}

func TestOcca_MarshalTo_EmptyCursorIDs(t *testing.T) {
	t.Parallel()

	m := &tTIOcca{
		headerMarshaller: &occaHeaderStub{payload: []byte{0xCC}},
		cursorIDs:        nil,
	}

	buf := NewArrayDataBuffer(16)
	engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	want := []byte{
		0xCC,
		0x01, // PTR sentinel
		0x00, // offset = 0
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected OCCA payload for empty cursor list:\n got: % X\nwant: % X", got, want)
	}
}

func TestOcca_MarshalTo_Failures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	assertFailMarshal := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected MarshalTo to fail")
		}
		var oraErr oracleErrors.SQLError
		if !errors.As(err, &oraErr) {
			t.Fatalf("expected SQLError wrapper, got %T", err)
		}
		if oraErr.ErrorCode() != string(oracleErrors.FailMarshal) {
			t.Fatalf("expected FailMarshal error code, got %v", oraErr.ErrorCode())
		}
	}

	t.Run("Header", func(t *testing.T) {
		m := &tTIOcca{
			headerMarshaller: &occaHeaderStub{err: errors.New("boom")},
			cursorIDs:        []driverCommon.UB4{3},
		}

		buf := NewArrayDataBuffer(16)
		engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

		assertFailMarshal(t, m.MarshalTo(ctx, engine))
	})

	t.Run("PTR", func(t *testing.T) {
		m := &tTIOcca{
			headerMarshaller: &occaHeaderStub{},
			cursorIDs:        []driverCommon.UB4{3},
		}

		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(16),
			FailOnWriteByteCall:  1, // MarshalPTR writes the first byte after the stub header
		}
		engine := NewMarshalEngine(faulty, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

		assertFailMarshal(t, m.MarshalTo(ctx, engine))
	})

	t.Run("Offset", func(t *testing.T) {
		m := &tTIOcca{
			headerMarshaller: &occaHeaderStub{payload: []byte{0xFF}},
			cursorIDs:        []driverCommon.UB4{3},
		}

		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(16),
			FailOnWriteBytesCall: 1, // first WriteBytes call is the offset UB4
		}
		engine := NewMarshalEngine(faulty, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

		assertFailMarshal(t, m.MarshalTo(ctx, engine))
	})

	t.Run("CursorID", func(t *testing.T) {
		m := &tTIOcca{
			headerMarshaller: &occaHeaderStub{},
			cursorIDs:        []driverCommon.UB4{3},
		}

		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(16),
			FailOnWriteBytesCall: 2, // second WriteBytes call is the first cursor ID UB4
		}
		engine := NewMarshalEngine(faulty, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

		assertFailMarshal(t, m.MarshalTo(ctx, engine))
	})
}
