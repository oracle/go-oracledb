/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and under any and all patent rights owned or freely
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

package lob

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func TestLOBValue_ZeroValueStateAndOperations(t *testing.T) {
	t.Parallel()

	var value lobValue
	if value.current() != nil || value.Kind() != Unknown || value.Valid() || value.IsTemporary() {
		t.Fatalf("zero-value state = handle:%v kind:%v valid:%t temporary:%t", value.current(), value.Kind(), value.Valid(), value.IsTemporary())
	}

	operations := []struct {
		name string
		call func() error
	}{
		{name: "Read", call: func() error { _, err := value.Read(make([]byte, 1)); return err }},
		{name: "ReadContext", call: func() error { _, err := value.ReadContext(context.Background(), make([]byte, 1)); return err }},
		{name: "hasUnreadDataContext", call: func() error { _, err := value.hasUnreadDataContext(context.Background()); return err }},
		{name: "Write", call: func() error { _, err := value.Write([]byte("x")); return err }},
		{name: "WriteContext", call: func() error { _, err := value.WriteContext(context.Background(), []byte("x")); return err }},
		{name: "writeStringContext", call: func() error { _, err := value.writeStringContext(context.Background(), "x"); return err }},
		{name: "WriteTo", call: func() error { _, err := value.WriteTo(io.Discard); return err }},
		{name: "WriteToContext", call: func() error { _, err := value.WriteToContext(context.Background(), io.Discard); return err }},
		{name: "Size", call: func() error { _, err := value.Size(); return err }},
		{name: "SizeContext", call: func() error { _, err := value.SizeContext(context.Background()); return err }},
		{name: "ChunkSize", call: func() error { _, err := value.ChunkSize(); return err }},
		{name: "ChunkSizeContext", call: func() error { _, err := value.ChunkSizeContext(context.Background()); return err }},
		{name: "Trim", call: func() error { return value.Trim(0) }},
		{name: "TrimContext", call: func() error { return value.TrimContext(context.Background(), 0) }},
		{name: "Open", call: func() error { _, err := value.Open(ReadOnly); return err }},
		{name: "OpenContext", call: func() error { _, err := value.OpenContext(context.Background(), ReadOnly); return err }},
		{name: "CloseServer", call: func() error { return value.CloseServer() }},
		{name: "CloseServerContext", call: func() error { return value.CloseServerContext(context.Background()) }},
		{name: "IsOpen", call: func() error { _, err := value.IsOpen(); return err }},
		{name: "IsOpenContext", call: func() error { _, err := value.IsOpenContext(context.Background()); return err }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); err == nil {
				t.Fatal("operation on an unscanned value succeeded")
			} else {
				requireLOBValueErrorCode(t, err, oracleErrors.NullLobValue)
			}
		})
	}

	if err := value.Close(); err != nil {
		t.Fatalf("Close on zero value returned %v", err)
	}
	if err := value.Free(); err != nil {
		t.Fatalf("Free on zero value returned %v", err)
	}
	if err := value.FreeContext(context.Background()); err != nil {
		t.Fatalf("FreeContext on zero value returned %v", err)
	}
}

func TestLOBValue_ForwardsOperationsToHandle(t *testing.T) {
	t.Parallel()

	t.Run("Read", func(t *testing.T) {
		value := newLOBValueForTest(t, BLOB, false, &lobHandleTestRawConn{})
		value.handle.pending = []byte("read")
		value.handle.readComplete = true
		if unread, err := value.hasUnreadDataContext(context.Background()); !unread || err != nil {
			t.Fatalf("hasUnreadDataContext = (%t, %v), want (true, nil)", unread, err)
		}
		buffer := make([]byte, 4)
		if n, err := value.Read(buffer); n != 4 || err != nil || string(buffer) != "read" {
			t.Fatalf("Read = (%d, %v, %q), want (4, nil, read)", n, err, buffer)
		}
	})

	t.Run("Write", func(t *testing.T) {
		var got []byte
		raw := &lobHandleTestRawConn{writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
			got = append([]byte(nil), data...)
			return uint64(len(data)), nil
		}}
		value := newLOBValueForTest(t, BLOB, false, raw)
		if n, err := value.Write([]byte("write")); n != 5 || err != nil || string(got) != "write" {
			t.Fatalf("Write = (%d, %v), sent %q; want (5, nil), write", n, err, got)
		}
	})

	t.Run("WriteContext", func(t *testing.T) {
		var got []byte
		raw := &lobHandleTestRawConn{writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
			got = append([]byte(nil), data...)
			return uint64(len(data)), nil
		}}
		value := newLOBValueForTest(t, BLOB, false, raw)
		if n, err := value.WriteContext(context.Background(), []byte("ctx")); n != 3 || err != nil || string(got) != "ctx" {
			t.Fatalf("WriteContext = (%d, %v), sent %q; want (3, nil), ctx", n, err, got)
		}
	})

	t.Run("writeStringContext", func(t *testing.T) {
		var got []byte
		raw := &lobHandleTestRawConn{writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
			got = append([]byte(nil), data...)
			return 1, nil // é is one UTF-16 code unit and two UTF-8 bytes.
		}}
		value := newLOBValueForTest(t, CLOB, false, raw)
		if n, err := value.writeStringContext(context.Background(), "é"); n != 2 || err != nil || string(got) != "é" {
			t.Fatalf("writeStringContext = (%d, %v), sent %q; want (2, nil), é", n, err, got)
		}
	})

	t.Run("writeStringContextRejectsBLOB", func(t *testing.T) {
		writeCalls := 0
		raw := &lobHandleTestRawConn{writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) {
			writeCalls++
			return 0, nil
		}}
		value := newLOBValueForTest(t, BLOB, false, raw)
		if _, err := value.writeStringContext(context.Background(), "text"); err == nil {
			t.Fatal("writeStringContext accepted a BLOB value")
		} else {
			requireLOBValueErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
		}
		if writeCalls != 0 {
			t.Fatalf("writeStringContext issued %d write calls for a BLOB", writeCalls)
		}
	})

	t.Run("WriteTo", func(t *testing.T) {
		value := newLOBValueForTest(t, BLOB, false, &lobHandleTestRawConn{})
		value.handle.pending = []byte("copy")
		value.handle.readComplete = true
		var output bytes.Buffer
		if n, err := value.WriteTo(&output); n != 4 || err != nil || output.String() != "copy" {
			t.Fatalf("WriteTo = (%d, %v, %q), want (4, nil, copy)", n, err, output.String())
		}
	})

	t.Run("WriteToContext", func(t *testing.T) {
		value := newLOBValueForTest(t, BLOB, false, &lobHandleTestRawConn{})
		value.handle.pending = []byte("ctx")
		value.handle.readComplete = true
		var output bytes.Buffer
		if n, err := value.WriteToContext(context.Background(), &output); n != 3 || err != nil || output.String() != "ctx" {
			t.Fatalf("WriteToContext = (%d, %v, %q), want (3, nil, ctx)", n, err, output.String())
		}
	})

	t.Run("WriteToContextValidatesWriterAndShortWrites", func(t *testing.T) {
		value := newLOBValueForTest(t, BLOB, false, &lobHandleTestRawConn{})
		value.handle.pending = []byte("copy")
		value.handle.readComplete = true
		if _, err := value.WriteToContext(context.Background(), nil); err == nil {
			t.Fatal("WriteToContext accepted a nil writer")
		} else {
			requireLOBValueErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
		}
		if _, err := value.WriteToContext(context.Background(), lobValueWriterFunc(func(data []byte) (int, error) {
			return len(data) - 1, nil
		})); !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("WriteToContext short-write error = %v, want io.ErrShortWrite", err)
		}
	})

	t.Run("SizeAndChunkSize", func(t *testing.T) {
		raw := &lobHandleTestRawConn{
			lengthFn:    func(context.Context, uint8, []byte) (uint64, error) { return 12, nil },
			chunkSizeFn: func(context.Context, uint8, []byte) (uint64, error) { return 8, nil },
		}
		value := newLOBValueForTest(t, BLOB, false, raw)
		if size, err := value.Size(); size != 12 || err != nil {
			t.Fatalf("Size = (%d, %v), want (12, nil)", size, err)
		}
		if size, err := value.SizeContext(context.Background()); size != 12 || err != nil {
			t.Fatalf("SizeContext = (%d, %v), want (12, nil)", size, err)
		}
		if size, err := value.ChunkSize(); size != 8 || err != nil {
			t.Fatalf("ChunkSize = (%d, %v), want (8, nil)", size, err)
		}
		if size, err := value.ChunkSizeContext(context.Background()); size != 8 || err != nil {
			t.Fatalf("ChunkSizeContext = (%d, %v), want (8, nil)", size, err)
		}
	})

	t.Run("Trim", func(t *testing.T) {
		raw := &lobHandleTestRawConn{trimFn: func(_ context.Context, _ uint8, _ []byte, length uint64) (uint64, error) {
			return length, nil
		}}
		value := newLOBValueForTest(t, BLOB, false, raw)
		if err := value.Trim(3); err != nil {
			t.Fatalf("Trim returned %v", err)
		}
		if err := value.TrimContext(context.Background(), 2); err != nil {
			t.Fatalf("TrimContext returned %v", err)
		}
	})

	t.Run("OpenCloseServerAndIsOpen", func(t *testing.T) {
		raw := &lobHandleTestRawConn{
			openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
				return true, locator, nil
			},
			closeFn:  func(_ context.Context, _ uint8, locator []byte) ([]byte, error) { return locator, nil },
			isOpenFn: func(context.Context, uint8, []byte) (bool, error) { return true, nil },
		}
		value := newLOBValueForTest(t, BLOB, false, raw)
		if opened, err := value.Open(ReadOnly); !opened || err != nil {
			t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
		}
		if opened, err := value.OpenContext(context.Background(), ReadWrite); !opened || err != nil {
			t.Fatalf("OpenContext = (%t, %v), want (true, nil)", opened, err)
		}
		if err := value.CloseServer(); err != nil {
			t.Fatalf("CloseServer returned %v", err)
		}
		if err := value.CloseServerContext(context.Background()); err != nil {
			t.Fatalf("CloseServerContext returned %v", err)
		}
		if open, err := value.IsOpen(); !open || err != nil {
			t.Fatalf("IsOpen = (%t, %v), want (true, nil)", open, err)
		}
		if open, err := value.IsOpenContext(context.Background()); !open || err != nil {
			t.Fatalf("IsOpenContext = (%t, %v), want (true, nil)", open, err)
		}
	})

	t.Run("metadataAndFree", func(t *testing.T) {
		raw := &lobHandleTestRawConn{freeFn: func(context.Context, []byte) error { return nil }}
		value := newLOBValueForTest(t, NCLOB, true, raw)
		if value.Kind() != NCLOB || !value.Valid() || !value.IsTemporary() {
			t.Fatalf("metadata = kind:%v valid:%t temporary:%t", value.Kind(), value.Valid(), value.IsTemporary())
		}
		if err := value.Free(); err != nil {
			t.Fatalf("Free returned %v", err)
		}
		if value.Valid() || !value.IsTemporary() {
			t.Fatalf("metadata after Free = valid:%t temporary:%t, want false and true", value.Valid(), value.IsTemporary())
		}
	})
}

func TestLOBValue_TemporaryConstructorsRejectNilConn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	constructors := []struct {
		name string
		call func() (LOB, error)
	}{
		{name: "BLOB", call: func() (LOB, error) { return CreateTemporaryBlob(ctx, nil) }},
		{name: "CLOB", call: func() (LOB, error) { return CreateTemporaryClob(ctx, nil) }},
		{name: "NCLOB", call: func() (LOB, error) { return CreateTemporaryNClob(ctx, nil) }},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			if _, err := constructor.call(); err == nil {
				t.Fatal("temporary constructor accepted a nil connection")
			} else {
				requireLOBValueErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
			}
		})
	}
}

func TestLOBValue_ImplementsPublicContracts(t *testing.T) {
	var (
		_ sql.Scanner = (*Blob)(nil)
		_ sql.Scanner = (*Clob)(nil)
		_ sql.Scanner = (*NClob)(nil)
		_ LOB         = (*Blob)(nil)
		_ LOB         = (*Clob)(nil)
		_ LOB         = (*NClob)(nil)
	)
}

func TestLOBValue_TemporaryConstructors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	for _, test := range []struct {
		name string
		kind internallob.Kind
		new  func(context.Context, *sql.Conn) (LOB, error)
	}{
		{name: "BLOB", kind: internallob.BLOB, new: func(ctx context.Context, conn *sql.Conn) (LOB, error) {
			return CreateTemporaryBlob(ctx, conn)
		}},
		{name: "CLOB", kind: internallob.CLOB, new: func(ctx context.Context, conn *sql.Conn) (LOB, error) {
			return CreateTemporaryClob(ctx, conn)
		}},
		{name: "NCLOB", kind: internallob.NCLOB, new: func(ctx context.Context, conn *sql.Conn) (LOB, error) {
			return CreateTemporaryNClob(ctx, conn)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, _ := newLOBTestConn(t, test.kind, nil, nil)
			value, err := test.new(ctx, conn)
			if err != nil {
				t.Fatalf("temporary constructor returned error: %v", err)
			}
			if value.Kind() != Kind(test.kind) || !value.Valid() || !value.IsTemporary() {
				t.Fatalf("temporary metadata = kind:%v valid:%t temporary:%t", value.Kind(), value.Valid(), value.IsTemporary())
			}
			if err := value.FreeContext(ctx); err != nil {
				t.Fatalf("FreeContext returned error: %v", err)
			}
		})
	}
}

func TestLOBValue_ContextMethodsCheckCancellation(t *testing.T) {
	t.Parallel()

	_, raw := newLOBTestConn(t, internallob.BLOB, []byte("data"), nil)
	var blob Blob
	defer blob.Close()
	if err := blob.Scan(newPersistentTestSource(raw, internallob.BLOB, []byte("data"))); err != nil {
		t.Fatalf("Blob.Scan returned error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := blob.ReadContext(ctx, make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadContext error = %v, want context.Canceled", err)
	}
	if _, err := blob.SizeContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("SizeContext error = %v, want context.Canceled", err)
	}
	if _, err := blob.ReadAllContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadAllContext error = %v, want context.Canceled", err)
	}
}

// TestLOBValue_WriteStringMethods exercises both public CLOB and NCLOB string
// writers, including a supplementary character at an internal chunk boundary.

func TestLOBValue_WriteStringMethods(t *testing.T) {
	for _, test := range []struct {
		name string
		kind Kind
	}{{name: "CLOB", kind: CLOB}, {name: "NCLOB", kind: NCLOB}} {
		t.Run(test.name, func(t *testing.T) {
			kind := test.kind
			maxChars := internallob.DefaultCharacterLobChunkChars
			firstInput := "é🙂"
			largeInput := strings.Repeat("a", maxChars-1) + "🙂Z"
			type writeCall struct {
				offset  uint64
				payload string
			}
			var calls []writeCall
			raw := &lobHandleTestRawConn{
				writeFn: func(_ context.Context, _ uint8, _ []byte, offset uint64, data []byte) (uint64, error) {
					if !utf8.Valid(data) {
						t.Fatalf("LobWrite chunk is invalid UTF-8: %x", data)
					}
					if runes := utf8.RuneCount(data); runes > maxChars {
						t.Fatalf("LobWrite chunk has %d runes, exceeds %d", runes, maxChars)
					}
					calls = append(calls, writeCall{offset: offset, payload: string(data)})
					return lobTestUTF16Units(data), nil
				},
			}
			handle := newActiveLOBHandle(t, raw, kind)
			var writer interface {
				WriteString(string) (int, error)
				WriteStringContext(context.Context, string) (int, error)
			}
			switch kind {
			case CLOB:
				value := &Clob{lobValue: lobValue{kind: CLOB, handle: handle}}
				t.Cleanup(func() { _ = value.Close() })
				writer = value
			case NCLOB:
				value := &NClob{lobValue: lobValue{kind: NCLOB, handle: handle}}
				t.Cleanup(func() { _ = value.Close() })
				writer = value
			}

			if n, err := writer.WriteString(firstInput); n != len(firstInput) || err != nil {
				t.Fatalf("WriteString = (%d, %v), want (%d, nil)", n, err, len(firstInput))
			}
			if n, err := writer.WriteStringContext(context.Background(), largeInput); n != len(largeInput) || err != nil {
				t.Fatalf("WriteStringContext = (%d, %v), want (%d, nil)", n, err, len(largeInput))
			}
			wantChunks := []string{firstInput, strings.Repeat("a", maxChars-1) + "🙂", "Z"}
			wantOffsets := []uint64{1, 4, uint64(maxChars + 5)}
			if len(calls) != len(wantChunks) {
				t.Fatalf("LobWrite calls = %d, want %d", len(calls), len(wantChunks))
			}
			for index, call := range calls {
				if call.payload != wantChunks[index] || call.offset != wantOffsets[index] {
					t.Errorf("LobWrite[%d] = (offset %d, payload length %d), want (offset %d, payload length %d)", index, call.offset, len(call.payload), wantOffsets[index], len(wantChunks[index]))
				}
			}
			if wantOffset := uint64(maxChars + 6); handle.offset != wantOffset {
				t.Fatalf("final cursor = %d, want %d", handle.offset, wantOffset)
			}
		})
	}
}

// TestLOBValue_WriteStringContextReturnsAcknowledgedPrefix verifies string
// writes report input bytes accepted before a later request fails.

func TestLOBValue_WriteStringContextReturnsAcknowledgedPrefix(t *testing.T) {
	maxChars := internallob.DefaultCharacterLobChunkChars
	text := strings.Repeat("x", maxChars+3)
	writeErr := errors.New("second text chunk failed after dispatch")

	for _, test := range []struct {
		name string
		kind Kind
	}{{name: "CLOB", kind: CLOB}, {name: "NCLOB", kind: NCLOB}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			raw := &lobHandleTestRawConn{
				writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
					calls++
					if calls == 2 {
						return 0, writeErr
					}
					return uint64(len(data)), nil
				},
			}
			handle := newActiveLOBHandle(t, raw, test.kind)
			var writer interface {
				WriteStringContext(context.Context, string) (int, error)
			}
			switch test.kind {
			case CLOB:
				value := &Clob{lobValue: lobValue{kind: CLOB, handle: handle}}
				t.Cleanup(func() { _ = value.Close() })
				writer = value
			case NCLOB:
				value := &NClob{lobValue: lobValue{kind: NCLOB, handle: handle}}
				t.Cleanup(func() { _ = value.Close() })
				writer = value
			}

			if n, err := writer.WriteStringContext(context.Background(), text); n != maxChars || !errors.Is(err, writeErr) {
				t.Fatalf("WriteStringContext = (%d, %v), want (%d, %v)", n, err, maxChars, writeErr)
			}
			if calls != 2 || handle.offset != uint64(maxChars+1) || !handle.invalidated {
				t.Fatalf("state after later error = calls:%d offset:%d invalidated:%t", calls, handle.offset, handle.invalidated)
			}
		})
	}
}

func newLOBValueForTest(t *testing.T, kind Kind, temporary bool, raw *lobHandleTestRawConn) *lobValue {
	t.Helper()
	handle := newActiveLOBHandle(t, raw, kind)
	handle.temporary = temporary
	return &lobValue{kind: kind, handle: handle}
}

func requireLOBValueErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(want) {
		t.Fatalf("error = %v, want Oracle error %s", err, want)
	}
}

type lobValueWriterFunc func([]byte) (int, error)

func (writer lobValueWriterFunc) Write(data []byte) (int, error) {
	return writer(data)
}

func TestLOBScan_ValidatesKindAndSupportsReadAll(t *testing.T) {
	t.Parallel()

	blobData := []byte{0, 1, 2, 255}
	_, blobRaw := newLOBTestConn(t, internallob.BLOB, blobData, nil)
	var blob Blob
	defer blob.Close()
	blobSource := newPersistentTestSource(blobRaw, internallob.BLOB, blobData)
	if err := blob.Scan(blobSource); err != nil {
		t.Fatalf("Blob.Scan returned error: %v", err)
	}
	if blob.Kind() != BLOB || !blob.Valid() {
		t.Fatalf("Blob state = kind:%d valid:%t", blob.Kind(), blob.Valid())
	}
	data, err := blob.ReadAll()
	if err != nil || string(data) != string(blobSource.data) {
		t.Fatalf("Blob.ReadAll = (%x, %v), want (%x, nil)", data, err, blobSource.data)
	}
	if blobSource.closed {
		t.Fatal("Blob.ReadAll closed source")
	}

	clobData := []byte("Aé中🙂")
	_, clobRaw := newLOBTestConn(t, internallob.CLOB, clobData, nil)
	var clob Clob
	defer clob.Close()
	clobSource := newPersistentTestSource(clobRaw, internallob.CLOB, clobData)
	if err := clob.Scan(clobSource); err != nil {
		t.Fatalf("Clob.Scan returned error: %v", err)
	}
	text, err := clob.ReadAll()
	if err != nil || text != "Aé中🙂" {
		t.Fatalf("Clob.ReadAll = (%q, %v)", text, err)
	}

	_, nclobRaw := newLOBTestConn(t, internallob.NCLOB, clobData, nil)
	var nclob NClob
	defer nclob.Close()
	nclobSource := newPersistentTestSource(nclobRaw, internallob.NCLOB, clobData)
	if err := nclob.Scan(nclobSource); err != nil {
		t.Fatalf("NClob.Scan returned error: %v", err)
	}
	nationalText, err := nclob.ReadAll()
	if err != nil || nationalText != "Aé中🙂" {
		t.Fatalf("NClob.ReadAll = (%q, %v)", nationalText, err)
	}
}

func TestLOBScan_RejectsWrongKindAndInvalidSourceAndHandlesNull(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		scan       func(any) error
		sourceKind Kind
	}{
		{name: "Blob rejects CLOB", scan: func(source any) error { var value Blob; return value.Scan(source) }, sourceKind: internallob.CLOB},
		{name: "Clob rejects NCLOB", scan: func(source any) error { var value Clob; return value.Scan(source) }, sourceKind: internallob.NCLOB},
		{name: "NClob rejects BLOB", scan: func(source any) error { var value NClob; return value.Scan(source) }, sourceKind: internallob.BLOB},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &testSource{kind: test.sourceKind, data: []byte("wrong kind")}
			err := test.scan(source)
			var sqlErr oracleErrors.SQLError
			if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.InvalidLobSource) {
				t.Fatalf("Scan wrong-kind error = %v", err)
			}
			if !source.closed {
				t.Fatal("wrong-kind source was not closed")
			}
		})
	}

	var blob Blob
	defer blob.Close()
	if err := blob.Scan("not a LOB source"); err == nil {
		t.Fatal("Blob.Scan accepted a non-source value")
	} else {
		var sourceErr oracleErrors.SQLError
		if !errors.As(err, &sourceErr) || sourceErr.ErrorCode() != string(oracleErrors.InvalidLobSource) {
			t.Fatalf("Blob.Scan invalid-source error = %v", err)
		}
	}

	for _, test := range []struct {
		name  string
		value LOB
		kind  Kind
	}{
		{name: "BLOB", value: &Blob{}, kind: BLOB},
		{name: "CLOB", value: &Clob{}, kind: CLOB},
		{name: "NCLOB", value: &NClob{}, kind: NCLOB},
	} {
		t.Run(test.name+" NULL", func(t *testing.T) {
			if err := test.value.Scan(nil); err != nil {
				t.Fatalf("Scan(nil) returned error: %v", err)
			}
			if test.value.Kind() != test.kind || test.value.Valid() {
				t.Fatalf("NULL state = kind:%v valid:%t, want kind:%v valid:false", test.value.Kind(), test.value.Valid(), test.kind)
			}
			if err := test.value.Close(); err != nil {
				t.Fatalf("Close returned error: %v", err)
			}
		})
	}
}
func TestLOBScan_ReadAllPropagatesReadError(t *testing.T) {
	t.Parallel()

	readErr := errors.New("read failed")
	_, raw := newLOBTestConn(t, internallob.BLOB, nil, readErr)
	var blob Blob
	defer blob.Close()
	if err := blob.Scan(newPersistentTestSource(raw, internallob.BLOB, nil)); err != nil {
		t.Fatalf("Blob.Scan returned error: %v", err)
	}
	if _, err := blob.ReadAll(); !errors.Is(err, readErr) {
		t.Fatalf("Blob.ReadAll error = %v, want %v", err, readErr)
	}
}

func TestLOBScan_ZeroValueAdoptsUnreadSource(t *testing.T) {
	t.Parallel()

	raw := &lobTestRawConn{data: []byte("promoted BLOB value"), kind: uint8(internallob.BLOB)}
	raw.sessionKey = raw
	db := sql.OpenDB(lobTestConnector{raw: raw})
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db.Conn returned error: %v", err)
	}
	defer conn.Close()

	var blob Blob
	defer blob.Close()
	source := &persistentTestSource{
		testSource: &testSource{kind: internallob.BLOB},
		sessionKey: raw.sessionKey,
		session:    raw,
		locator:    []byte("typed-test-locator"),
	}
	if err := blob.Scan(source); err != nil {
		t.Fatalf("Blob.Scan returned error: %v", err)
	}
	if !source.detached {
		t.Fatal("zero-value typed scan did not adopt the persistent locator")
	}
	// Closing the original query source simulates Rows.Close after adoption.
	if err := source.Close(); err != nil {
		t.Fatalf("close query source after promotion: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := blob.ReadContext(canceled, make([]byte, 4)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled adopted read = %v, want context.Canceled", err)
	}
	data, err := blob.ReadAllContext(context.Background())
	if err != nil || string(data) != string(raw.data) {
		t.Fatalf("ReadAllContext after query close = (%q, %v), want (%q, nil)", data, err, raw.data)
	}
	if size, err := blob.SizeContext(context.Background()); err != nil || size != int64(len(raw.data)) {
		t.Fatalf("SizeContext = (%d, %v), want (%d, nil)", size, err, len(raw.data))
	}
	if chunkSize, err := blob.ChunkSizeContext(context.Background()); err != nil || chunkSize != 16 {
		t.Fatalf("ChunkSizeContext = (%d, %v), want (16, nil)", chunkSize, err)
	}

}

// TestLOBScan_AdoptedPrefixSurvivesRowsClose verifies that scanning transfers
// unread row-prefetched bytes and resumes locator reads after those bytes.

func TestLOBScan_AdoptedPrefixSurvivesRowsClose(t *testing.T) {
	t.Parallel()

	data := []byte("promoted BLOB value")
	_, raw := newLOBTestConn(t, internallob.BLOB, data, nil)
	var blob Blob
	defer blob.Close()
	source := newPersistentTestSource(raw, internallob.BLOB, data)
	source.handoff = internallob.PersistentLOB{
		Prefix:      append([]byte(nil), data[:5]...),
		FetchOffset: 6,
	}
	if err := blob.Scan(source); err != nil {
		t.Fatalf("Blob.Scan returned error: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close query source after promotion: %v", err)
	}

	first := make([]byte, 3)
	if n, err := blob.ReadContext(context.Background(), first); n != len(first) || err != nil || string(first) != "pro" {
		t.Fatalf("first adopted ReadContext = (%d, %v, %q), want (3, nil, pro)", n, err, first)
	}
	if len(raw.readOffsets) != 0 {
		t.Fatalf("reading the transferred prefix issued locator reads at %v", raw.readOffsets)
	}

	remaining, err := blob.ReadAllContext(context.Background())
	if err != nil || string(append(first, remaining...)) != string(data) {
		t.Fatalf("remaining adopted value = (%q, %v), want %q", remaining, err, data[len(first):])
	}
	if len(raw.readOffsets) == 0 || raw.readOffsets[0] != 6 {
		t.Fatalf("first locator read offsets = %v, want the first read at 6", raw.readOffsets)
	}
}

// TestLOBScan_CompletePrefixNeedsNoLocatorRead verifies complete and
// empty row prefixes reach EOF locally after adoption.

func TestLOBScan_CompletePrefixNeedsNoLocatorRead(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "small value", data: []byte("small")},
		{name: "empty value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, raw := newLOBTestConn(t, internallob.BLOB, test.data, nil)
			var blob Blob
			defer blob.Close()
			source := newPersistentTestSource(raw, internallob.BLOB, test.data)
			source.handoff = internallob.PersistentLOB{
				Prefix:         append([]byte(nil), test.data...),
				FetchOffset:    uint64(len(test.data)) + 1,
				PrefixComplete: true,
			}
			if err := blob.Scan(source); err != nil {
				t.Fatalf("Blob.Scan returned error: %v", err)
			}
			value, err := blob.ReadAllContext(context.Background())
			if err != nil || string(value) != string(test.data) {
				t.Fatalf("ReadAllContext = (%q, %v), want (%q, nil)", value, err, test.data)
			}
			if len(raw.readOffsets) != 0 {
				t.Fatalf("complete prefix issued locator reads at %v", raw.readOffsets)
			}
		})
	}
}

func TestLOBScan_InternalRejectsInvalidExpectedKindAndMissingAdoption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     any
		expected  Kind
		wantCode  oracleErrors.ErrorCode
		wantClose bool
	}{
		{name: "invalid expected kind", expected: Unknown, wantCode: oracleErrors.InvalidLobSource},
		{name: "source without locator adoption", input: &testSource{kind: internallob.BLOB}, expected: BLOB, wantCode: oracleErrors.InvalidLOBBuffer, wantClose: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := lobValue{}
			err := value.scan(test.input, test.expected)
			requireLOBValueErrorCode(t, err, test.wantCode)
			if value.current() != nil || value.Kind() != Unknown {
				t.Fatalf("failed Scan changed value state: handle:%v kind:%v", value.current(), value.Kind())
			}
			if source, ok := test.input.(*testSource); ok && source.closed != test.wantClose {
				t.Fatalf("source closed = %t, want %t", source.closed, test.wantClose)
			}
		})
	}
}

type testSource struct {
	kind      internallob.Kind
	data      []byte
	offset    int
	closed    bool
	closeErr  error
	readErr   error
	size      int64
	chunkSize int64
}

func (source *testSource) Read(dst []byte) (int, error) {
	if source.readErr != nil {
		return 0, source.readErr
	}
	if source.offset == len(source.data) {
		return 0, io.EOF
	}
	n := copy(dst, source.data[source.offset:])
	source.offset += n
	return n, nil
}

func (source *testSource) WriteTo(writer io.Writer) (int64, error) {
	n, err := writer.Write(source.data[source.offset:])
	source.offset += n
	return int64(n), err
}

func (source *testSource) Close() error {
	source.closed = true
	return source.closeErr
}

func (source *testSource) Size() (int64, error)      { return source.size, nil }
func (source *testSource) ChunkSize() (int64, error) { return source.chunkSize, nil }
func (source *testSource) Kind() internallob.Kind    { return source.kind }

type persistentTestSource struct {
	*testSource
	sessionKey any
	session    internallob.LOBSession
	locator    []byte
	handoff    internallob.PersistentLOB
	detached   bool
}

func newLOBTestConn(t *testing.T, kind internallob.Kind, data []byte, readErr error) (*sql.Conn, *lobTestRawConn) {
	t.Helper()
	raw := &lobTestRawConn{
		data:    append([]byte(nil), data...),
		readErr: readErr,
		kind:    uint8(kind),
	}
	raw.sessionKey = raw
	db := sql.OpenDB(lobTestConnector{raw: raw})
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db.Conn returned error: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, raw
}

func newPersistentTestSource(raw *lobTestRawConn, kind internallob.Kind, data []byte) *persistentTestSource {
	return &persistentTestSource{
		testSource: &testSource{kind: kind, data: append([]byte(nil), data...)},
		sessionKey: raw.sessionKey,
		session:    raw,
		locator:    []byte("typed-test-locator"),
	}
}

func (source *persistentTestSource) DetachPersistentLocator(sessionKey any) ([]byte, error) {
	if sessionKey != source.sessionKey {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "promote-persistent", "LOB", "source belongs to another session")
	}
	source.detached = true
	return append([]byte(nil), source.locator...), nil
}

func (source *persistentTestSource) AdoptPersistentLocator(expected internallob.Kind) (internallob.PersistentLOB, internallob.LOBSession, error) {
	if expected != source.kind || source.session == nil {
		return internallob.PersistentLOB{}, nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "adopt-persistent", "LOB", "invalid source capability")
	}
	locator, err := source.DetachPersistentLocator(source.sessionKey)
	if err != nil {
		return internallob.PersistentLOB{}, nil, err
	}
	state := source.handoff
	state.Locator = locator
	if state.FetchOffset == 0 {
		state.FetchOffset = 1
	}
	return state, source.session, nil
}

type lobTestConnector struct{ raw *lobTestRawConn }

func (connector lobTestConnector) Connect(context.Context) (driver.Conn, error) {
	return connector.raw, nil
}

func (lobTestConnector) Driver() driver.Driver { return lobTestDriver{} }

type lobTestDriver struct{}

func (lobTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("Open should not be called by sql.OpenDB")
}

type lobTestRawConn struct {
	data        []byte
	readErr     error
	kind        uint8
	sessionKey  any
	readOffsets []uint64
}

func (*lobTestRawConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("Prepare is not supported in the LOB scanner test")
}

func (*lobTestRawConn) Close() error { return nil }

func (*lobTestRawConn) Begin() (driver.Tx, error) {
	return nil, errors.New("Begin is not supported in the LOB scanner test")
}

func (connection *lobTestRawConn) LobCreate(context.Context, uint8) ([]byte, error) {
	return []byte("typed-test-locator"), nil
}

func (connection *lobTestRawConn) LobRead(ctx context.Context, kind uint8, locator []byte, offset, amount uint64) ([]byte, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if connection.readErr != nil {
		return nil, 0, connection.readErr
	}
	if kind != connection.kind || string(locator) != "typed-test-locator" {
		return nil, 0, errors.New("unexpected LOB read arguments")
	}
	connection.readOffsets = append(connection.readOffsets, offset)
	start := int(offset - 1)
	if start >= len(connection.data) {
		return nil, 0, nil
	}
	end := start + int(amount)
	if end > len(connection.data) {
		end = len(connection.data)
	}
	data := append([]byte(nil), connection.data[start:end]...)
	return data, uint64(len(data)), nil
}

func (*lobTestRawConn) LobWrite(context.Context, uint8, []byte, uint64, []byte) (uint64, error) {
	return 0, nil
}

func (connection *lobTestRawConn) LobLength(ctx context.Context, _ uint8, _ []byte) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return uint64(len(connection.data)), nil
}

func (*lobTestRawConn) LobChunkSize(context.Context, uint8, []byte) (uint64, error) {
	return 16, nil
}

func (*lobTestRawConn) LobTrim(context.Context, uint8, []byte, uint64) (uint64, error) {
	return 0, nil
}

func (*lobTestRawConn) LobOpen(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
	return false, append([]byte(nil), locator...), nil
}

func (*lobTestRawConn) LobClose(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
	return append([]byte(nil), locator...), nil
}

func (*lobTestRawConn) LobIsOpen(context.Context, uint8, []byte) (bool, error) {
	return false, nil
}

func (*lobTestRawConn) LobFree(context.Context, []byte) error { return nil }

func (connection *lobTestRawConn) LobSessionKey() any { return connection.sessionKey }

func (*lobTestRawConn) ValidateLOBLease() error { return nil }

func (*lobTestRawConn) AcquireLOBLease() (func(), error) { return func() {}, nil }
