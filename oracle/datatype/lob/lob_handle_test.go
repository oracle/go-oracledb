/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and to any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors), and
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

	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type lobHandleTestConnector struct {
	raw driver.Conn
}

func (connector lobHandleTestConnector) Connect(context.Context) (driver.Conn, error) {
	return connector.raw, nil
}

func (connector lobHandleTestConnector) Driver() driver.Driver {
	return lobHandleTestDriver{raw: connector.raw}
}

type lobHandleTestDriver struct {
	raw driver.Conn
}

func (driver lobHandleTestDriver) Open(string) (driver.Conn, error) {
	return driver.raw, nil
}

func newLOBHandleTestConn(t *testing.T, raw driver.Conn) *sql.Conn {
	t.Helper()
	db := sql.OpenDB(lobHandleTestConnector{raw: raw})
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("DB.Conn returned error: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// lobHandleTestRawConn embeds the private driver contract so each test only
// overrides the raw operation it exercises.
type lobHandleTestRawConn struct {
	lobHandleDriver
	sessionKey  any
	createFn    func(context.Context, uint8) ([]byte, error)
	readFn      func(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error)
	writeFn     func(context.Context, uint8, []byte, uint64, []byte) (uint64, error)
	lengthFn    func(context.Context, uint8, []byte) (uint64, error)
	chunkSizeFn func(context.Context, uint8, []byte) (uint64, error)
	trimFn      func(context.Context, uint8, []byte, uint64) (uint64, error)
	openFn      func(context.Context, uint8, []byte, uint8) (bool, []byte, error)
	closeFn     func(context.Context, uint8, []byte) ([]byte, error)
	isOpenFn    func(context.Context, uint8, []byte) (bool, error)
	freeFn      func(context.Context, []byte) error
}

func (driver *lobHandleTestRawConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unused")
}

func (driver *lobHandleTestRawConn) Close() error { return nil }

func (driver *lobHandleTestRawConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unused")
}

func (driver *lobHandleTestRawConn) LobSessionKey() any {
	return driver.sessionKey
}

func (driver *lobHandleTestRawConn) LobCreate(ctx context.Context, kind uint8) ([]byte, error) {
	if driver.createFn == nil {
		return nil, errors.New("LobCreate unused")
	}
	return driver.createFn(ctx, kind)
}

func (driver *lobHandleTestRawConn) LobRead(ctx context.Context, kind uint8, locator []byte, offset, amount uint64) ([]byte, uint64, error) {
	if driver.readFn == nil {
		return nil, 0, errors.New("LobRead unused")
	}
	return driver.readFn(ctx, kind, locator, offset, amount)
}

func (driver *lobHandleTestRawConn) LobWrite(ctx context.Context, kind uint8, locator []byte, offset uint64, data []byte) (uint64, error) {
	if driver.writeFn == nil {
		return 0, errors.New("LobWrite unused")
	}
	return driver.writeFn(ctx, kind, locator, offset, data)
}

func (driver *lobHandleTestRawConn) LobLength(ctx context.Context, kind uint8, locator []byte) (uint64, error) {
	if driver.lengthFn == nil {
		return 0, errors.New("LobLength unused")
	}
	return driver.lengthFn(ctx, kind, locator)
}

func (driver *lobHandleTestRawConn) LobChunkSize(ctx context.Context, kind uint8, locator []byte) (uint64, error) {
	if driver.chunkSizeFn == nil {
		return 0, errors.New("LobChunkSize unused")
	}
	return driver.chunkSizeFn(ctx, kind, locator)
}

func (driver *lobHandleTestRawConn) LobTrim(ctx context.Context, kind uint8, locator []byte, length uint64) (uint64, error) {
	if driver.trimFn == nil {
		return 0, errors.New("LobTrim unused")
	}
	return driver.trimFn(ctx, kind, locator, length)
}

func (driver *lobHandleTestRawConn) LobOpen(ctx context.Context, kind uint8, locator []byte, mode uint8) (bool, []byte, error) {
	if driver.openFn == nil {
		return false, nil, errors.New("LobOpen unused")
	}
	return driver.openFn(ctx, kind, locator, mode)
}

func (driver *lobHandleTestRawConn) LobClose(ctx context.Context, kind uint8, locator []byte) ([]byte, error) {
	if driver.closeFn == nil {
		return nil, errors.New("LobClose unused")
	}
	return driver.closeFn(ctx, kind, locator)
}

func (driver *lobHandleTestRawConn) LobIsOpen(ctx context.Context, kind uint8, locator []byte) (bool, error) {
	if driver.isOpenFn == nil {
		return false, errors.New("LobIsOpen unused")
	}
	return driver.isOpenFn(ctx, kind, locator)
}

func (driver *lobHandleTestRawConn) LobFree(ctx context.Context, locator []byte) error {
	if driver.freeFn == nil {
		return errors.New("LobFree unused")
	}
	return driver.freeFn(ctx, locator)
}

// newActiveLOBHandle creates an active lobHandle backed by a fake raw
// connection so API operations can be tested without an Oracle database.
func newActiveLOBHandle(t *testing.T, raw *lobHandleTestRawConn, kind Kind) *lobHandle {
	t.Helper()
	return &lobHandle{conn: newLOBHandleTestConn(t, raw), kind: kind, locator: []byte("locator"), offset: 1, fetchOffset: 1}
}

func readLOBTextUnits(text string, offset, amount uint64) ([]byte, uint64, error) {
	if offset == 0 {
		return nil, 0, errors.New("zero Oracle text offset")
	}
	position := uint64(1)
	var payload []byte
	var units uint64
	for _, r := range text {
		characterUnits := uint64(1)
		if r > 0xFFFF {
			characterUnits = 2
		}
		end := position + characterUnits
		if position < offset && offset < end {
			return nil, 0, errors.New("offset inside surrogate pair")
		}
		if position >= offset && units < amount {
			payload = append(payload, string(r)...)
			units += characterUnits
		}
		position = end
		if units >= amount {
			break
		}
	}
	return payload, units, nil
}

func lobTestUTF16Units(data []byte) uint64 {
	var units uint64
	for _, character := range string(data) {
		units++
		if character > 0xFFFF {
			units++
		}
	}
	return units
}

// readOnce returns one configured response followed by the logical EOF used by
// ReadContext and WriteTo tests.
func readOnce(data []byte, logical uint64, err error) func(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error) {
	used := false
	return func(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error) {
		if used {
			return nil, 0, nil
		}
		used = true
		return data, logical, err
	}
}

// lobHandleTestWriter provides controllable writer errors and short writes for
// testing lobHandle.WriteTo result handling.
type lobHandleTestWriter struct {
	err error
	max int
}

func (writer *lobHandleTestWriter) Write(data []byte) (int, error) {
	written := len(data)
	if writer.max >= 0 && written > writer.max {
		written = writer.max
	}
	return written, writer.err
}

// unsupportedLOBHandleTestRawConn is a database/sql connection that does not
// implement the private lobHandle driver capability contract.
type unsupportedLOBHandleTestRawConn struct{}

func (*unsupportedLOBHandleTestRawConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unused")
}

func (*unsupportedLOBHandleTestRawConn) Close() error { return nil }

func (*unsupportedLOBHandleTestRawConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unused")
}

func requireLOBHandleTestErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(want) {
		t.Fatalf("error = %v, want %s", err, want)
	}
}

// TestLOBHandle_ReadAheadDoesNotMoveWriteCursor verifies BLOB read-ahead does
// not move the caller-visible cursor and that a write invalidates stale bytes.
func TestLOBHandle_ReadAheadDoesNotMoveWriteCursor(t *testing.T) {
	t.Parallel()

	data := make([]byte, internallob.DefaultBlobLobChunkBytes+4)
	for index := range data {
		data[index] = byte(index % 251)
	}
	readOffsets := make([]uint64, 0, 2)
	var writeOffset uint64
	raw := &lobHandleTestRawConn{
		readFn: func(_ context.Context, _ uint8, _ []byte, offset, amount uint64) ([]byte, uint64, error) {
			readOffsets = append(readOffsets, offset)
			start := int(offset - 1)
			if start >= len(data) {
				return nil, 0, nil
			}
			end := start + int(amount)
			if end > len(data) {
				end = len(data)
			}
			return append([]byte(nil), data[start:end]...), uint64(end - start), nil
		},
		writeFn: func(_ context.Context, _ uint8, _ []byte, offset uint64, payload []byte) (uint64, error) {
			writeOffset = offset
			start := int(offset - 1)
			copy(data[start:], payload)
			return uint64(len(payload)), nil
		},
	}
	value := newActiveLOBHandle(t, raw, BLOB)

	first := make([]byte, 1)
	if n, err := value.ReadContext(context.Background(), first); n != 1 || err != nil || first[0] != data[0] {
		t.Fatalf("first ReadContext = (%d, %v, %v), want first byte %v", n, err, first[0], data[0])
	}
	if value.offset != 2 || value.fetchOffset != uint64(internallob.DefaultBlobLobChunkBytes+1) {
		t.Fatalf("positions after one-byte read = caller:%d fetch:%d, want caller:2 fetch:%d", value.offset, value.fetchOffset, internallob.DefaultBlobLobChunkBytes+1)
	}

	if n, err := value.WriteContext(context.Background(), []byte{'Z'}); n != 1 || err != nil {
		t.Fatalf("WriteContext = (%d, %v), want (1, nil)", n, err)
	}
	if writeOffset != 2 {
		t.Fatalf("write offset = %d, want 2", writeOffset)
	}
	if len(value.pending) != 0 || value.offset != 3 || value.fetchOffset != 3 {
		t.Fatalf("state after write = pending:%d caller:%d fetch:%d, want pending:0 caller:3 fetch:3", len(value.pending), value.offset, value.fetchOffset)
	}

	next := make([]byte, 1)
	if n, err := value.ReadContext(context.Background(), next); n != 1 || err != nil || next[0] != data[2] {
		t.Fatalf("read after write = (%d, %v, %v), want server byte at position 3 (%v)", n, err, next[0], data[2])
	}
	if len(readOffsets) != 2 || readOffsets[0] != 1 || readOffsets[1] != 3 {
		t.Fatalf("read offsets = %v, want [1 3]", readOffsets)
	}
}

// TestLOBHandle_BlobReadCapsRefillAndConsumesPending verifies BLOB refills use
// the bounded TTC chunk and that prefetched bytes are served before another RPC.
func TestLOBHandle_BlobReadCapsRefillAndConsumesPending(t *testing.T) {
	t.Parallel()

	readCalls := 0
	var requested []uint64
	raw := &lobHandleTestRawConn{
		readFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, amount uint64) ([]byte, uint64, error) {
			readCalls++
			requested = append(requested, amount)
			data := make([]byte, int(amount))
			for index := range data {
				data[index] = 'x'
			}
			return data, amount, nil
		},
	}
	conn := newLOBHandleTestConn(t, raw)
	value := &lobHandle{
		conn:    conn,
		kind:    BLOB,
		locator: []byte("locator"),
		offset:  1,
	}

	first := make([]byte, 1)
	if n, err := value.ReadContext(context.Background(), first); n != 1 || err != nil || string(first) != "x" {
		t.Fatalf("first ReadContext = (%d, %v, %q), want (1, nil, x)", n, err, first)
	}
	if len(requested) != 1 || requested[0] != uint64(internallob.DefaultBlobLobChunkBytes) {
		t.Fatalf("BLOB refill requests = %v, want one request of %d bytes", requested, internallob.DefaultBlobLobChunkBytes)
	}

	second := make([]byte, 2)
	if n, err := value.ReadContext(context.Background(), second); n != 2 || err != nil || string(second) != "xx" {
		t.Fatalf("second ReadContext = (%d, %v, %q), want (2, nil, xx)", n, err, second)
	}
	if readCalls != 1 {
		t.Fatalf("LobRead calls after consuming pending data = %d, want 1", readCalls)
	}
}

func TestLOBHandle_OneBasedLOBPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		offset  int64
		advance uint64
		want    uint64
		wantErr bool
	}{
		{name: "first byte", offset: 0, want: 1},
		{name: "offset and advance", offset: 5, advance: 3, want: 9},
		{name: "largest public offset", offset: int64(^uint64(0) >> 1), want: uint64(1) << 63},
		{name: "negative offset", offset: -1, wantErr: true},
		{name: "advance overflow", offset: 0, advance: ^uint64(0), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := oneBasedLOBPosition(test.offset, test.advance)
			if test.wantErr {
				if err == nil {
					t.Fatal("oneBasedLOBPosition succeeded, want error")
				}
				requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("oneBasedLOBPosition(%d, %d) = (%d, %v), want (%d, nil)", test.offset, test.advance, got, err, test.want)
			}
		})
	}
}

func TestLOBHandle_ReadAtKeepsSequentialCursorAndReadAhead(t *testing.T) {
	t.Parallel()

	data := []byte{0, 1, 2, 255, 128, 5, 6}
	var readOffsets []uint64
	raw := &lobHandleTestRawConn{
		lengthFn: func(_ context.Context, kind uint8, _ []byte) (uint64, error) {
			if Kind(kind) != BLOB {
				t.Fatalf("LobLength kind = %d, want BLOB", kind)
			}
			return uint64(len(data)), nil
		},
		readFn: func(_ context.Context, kind uint8, _ []byte, offset, amount uint64) ([]byte, uint64, error) {
			if Kind(kind) != BLOB {
				t.Fatalf("LobRead kind = %d, want BLOB", kind)
			}
			readOffsets = append(readOffsets, offset)
			start := offset - 1
			if start >= uint64(len(data)) {
				return nil, 0, nil
			}
			end := start + amount
			if end > uint64(len(data)) {
				end = uint64(len(data))
			}
			payload := append([]byte(nil), data[start:end]...)
			return payload, uint64(len(payload)), nil
		},
	}
	handle := newActiveLOBHandle(t, raw, BLOB)

	var first [1]byte
	if n, err := handle.ReadContext(context.Background(), first[:]); n != 1 || err != nil || first[0] != data[0] {
		t.Fatalf("initial sequential read = (%d, %v, %v), want first byte %d", n, err, first[0], data[0])
	}
	beforeOffset, beforeFetch := handle.offset, handle.fetchOffset
	beforePending := append([]byte(nil), handle.pending...)
	beforeCarry := append([]byte(nil), handle.utf8Carry...)
	readOffsets = nil

	value := &Blob{lobValue: lobValue{kind: BLOB, handle: handle}}
	start := make([]byte, 2)
	if n, err := value.ReadAt(start, 0); n != 2 || err != nil || !bytes.Equal(start, data[:2]) {
		t.Fatalf("ReadAt at zero = (%d, %v, %v), want (%d, nil, %v)", n, err, start, 2, data[:2])
	}
	middle := make([]byte, 2)
	if n, err := value.ReadAtContext(context.Background(), middle, 2); n != 2 || err != nil || !bytes.Equal(middle, data[2:4]) {
		t.Fatalf("ReadAtContext at middle = (%d, %v, %v), want (%d, nil, %v)", n, err, middle, 2, data[2:4])
	}
	if len(readOffsets) != 2 || readOffsets[0] != 1 || readOffsets[1] != 3 {
		t.Fatalf("locator read offsets = %v, want [1 3]", readOffsets)
	}
	atEOF := make([]byte, 1)
	if n, err := value.ReadAt(atEOF, int64(len(data))); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatal("ReadAt at EOF did not return (0, io.EOF)")
	}
	if handle.offset != beforeOffset || handle.fetchOffset != beforeFetch || !bytes.Equal(handle.pending, beforePending) || !bytes.Equal(handle.utf8Carry, beforeCarry) {
		t.Fatalf("positional read changed sequential state: offset=%d fetch=%d pending=%v carry=%v", handle.offset, handle.fetchOffset, handle.pending, handle.utf8Carry)
	}

	remaining := make([]byte, len(data)-1)
	if n, err := handle.ReadContext(context.Background(), remaining); n != len(remaining) || err != nil || !bytes.Equal(remaining, data[1:]) {
		t.Fatalf("sequential read after ReadAt = (%d, %v, %v), want remaining bytes %v", n, err, remaining, data[1:])
	}
}

func TestLOBHandle_ReadAtValidationAndChunking(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte{0xA5}, internallob.DefaultBlobLobChunkBytes+3)
	lengthCalls := 0
	readOffsets := make([]uint64, 0, 2)
	raw := &lobHandleTestRawConn{
		lengthFn: func(context.Context, uint8, []byte) (uint64, error) {
			lengthCalls++
			return uint64(len(data)), nil
		},
		readFn: func(_ context.Context, _ uint8, _ []byte, offset, amount uint64) ([]byte, uint64, error) {
			readOffsets = append(readOffsets, offset)
			start := offset - 1
			end := start + amount
			if end > uint64(len(data)) {
				end = uint64(len(data))
			}
			payload := append([]byte(nil), data[start:end]...)
			return payload, uint64(len(payload)), nil
		},
	}
	handle := newActiveLOBHandle(t, raw, BLOB)
	value := &Blob{lobValue: lobValue{kind: BLOB, handle: handle}}

	if n, err := value.ReadAt(nil, 0); n != 0 || err != nil {
		t.Fatalf("zero-length ReadAt = (%d, %v), want (0, nil)", n, err)
	}
	if lengthCalls != 0 {
		t.Fatalf("zero-length ReadAt made %d length calls, want 0", lengthCalls)
	}
	if _, err := value.ReadAt([]byte{1}, -1); err == nil {
		t.Fatal("ReadAt accepted a negative offset")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if lengthCalls != 0 {
		t.Fatalf("invalid ReadAt made %d length calls, want 0", lengthCalls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := value.ReadAtContext(ctx, []byte{1}, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled ReadAtContext error = %v, want context.Canceled", err)
	}
	if lengthCalls != 0 {
		t.Fatalf("canceled ReadAtContext made %d length calls, want 0", lengthCalls)
	}

	got := make([]byte, len(data))
	if n, err := value.ReadAt(got, 0); n != len(data) || err != nil || !bytes.Equal(got, data) {
		t.Fatalf("large ReadAt = (%d, %v), want (%d, nil)", n, err, len(data))
	}
	if len(readOffsets) != 2 || readOffsets[0] != 1 || readOffsets[1] != uint64(internallob.DefaultBlobLobChunkBytes+1) {
		t.Fatalf("large ReadAt locator offsets = %v, want [1 %d]", readOffsets, internallob.DefaultBlobLobChunkBytes+1)
	}

	for _, test := range []struct {
		name string
		init func(*lobHandle)
		want oracleErrors.ErrorCode
	}{
		{name: "closed", init: func(handle *lobHandle) { handle.closed = true }, want: oracleErrors.LobValueClosed},
		{name: "freed", init: func(handle *lobHandle) { handle.freed = true }, want: oracleErrors.LobValueClosed},
		{name: "invalidated", init: func(handle *lobHandle) { handle.invalidated = true }, want: oracleErrors.LobValueInvalidated},
	} {
		t.Run(test.name, func(t *testing.T) {
			inactive := &lobHandle{kind: BLOB, locator: []byte("locator")}
			test.init(inactive)
			if _, err := inactive.ReadAtContext(context.Background(), []byte{1}, 0); err == nil {
				t.Fatal("ReadAtContext on inactive handle succeeded")
			} else {
				requireLOBHandleTestErrorCode(t, err, test.want)
			}
		})
	}
}

func TestLOBHandle_ReadTextAtUsesUTF16UnitsAndKeepsSequentialState(t *testing.T) {
	t.Parallel()

	text := "Aé🙂Z"
	readOffsets := make([]uint64, 0, 8)
	raw := &lobHandleTestRawConn{
		lengthFn: func(_ context.Context, kind uint8, _ []byte) (uint64, error) {
			if Kind(kind) != CLOB && Kind(kind) != NCLOB {
				t.Fatalf("LobLength kind = %d, want CLOB or NCLOB", kind)
			}
			return lobTestUTF16Units([]byte(text)), nil
		},
		readFn: func(_ context.Context, kind uint8, _ []byte, offset, amount uint64) ([]byte, uint64, error) {
			readOffsets = append(readOffsets, offset)
			return readLOBTextUnits(text, offset, amount)
		},
	}

	for _, test := range []struct {
		name string
		kind Kind
	}{{name: "CLOB", kind: CLOB}, {name: "NCLOB", kind: NCLOB}} {
		t.Run(test.name, func(t *testing.T) {
			handle := newActiveLOBHandle(t, raw, test.kind)
			var value interface {
				ReadTextAt([]byte, int64) (int, int64, error)
				ReadTextAtContext(context.Context, []byte, int64) (int, int64, error)
			}
			switch test.kind {
			case CLOB:
				value = &Clob{lobValue: lobValue{kind: CLOB, handle: handle}}
			case NCLOB:
				value = &NClob{lobValue: lobValue{kind: NCLOB, handle: handle}}
			}

			var next [1]byte
			if n, err := handle.ReadContext(context.Background(), next[:]); n != 1 || err != nil || string(next[:]) != "A" {
				t.Fatalf("sequential read before positional read = (%d, %v, %q), want A", n, err, next[:])
			}
			beforeOffset, beforeFetch := handle.offset, handle.fetchOffset
			beforePending := append([]byte(nil), handle.pending...)
			beforeCarry := append([]byte(nil), handle.utf8Carry...)

			exactPrefix := make([]byte, len("Aé"))
			if n, units, err := value.ReadTextAt(exactPrefix, 0); n != len(exactPrefix) || units != 2 || err != nil || string(exactPrefix) != "Aé" {
				t.Fatalf("ReadTextAt exact prefix = (%d, %d, %v, %q), want (%d, 2, nil, Aé)", n, units, err, exactPrefix, len(exactPrefix))
			}

			prefix := make([]byte, 4)
			if n, units, err := value.ReadTextAt(prefix, 0); n != 3 || units != 2 || !errors.Is(err, io.ErrShortBuffer) || string(prefix[:n]) != "Aé" || !utf8.Valid(prefix[:n]) {
				t.Fatalf("ReadTextAt short range = (%d, %d, %v, %q), want (3, 2, ErrShortBuffer, Aé)", n, units, err, prefix[:n])
			}

			tooSmall := make([]byte, 3)
			if n, units, err := value.ReadTextAtContext(context.Background(), tooSmall, 2); n != 0 || units != 0 || !errors.Is(err, io.ErrShortBuffer) {
				t.Fatalf("ReadTextAt supplementary short buffer = (%d, %d, %v), want (0, 0, ErrShortBuffer)", n, units, err)
			}

			full := make([]byte, len(text))
			if n, units, err := value.ReadTextAtContext(context.Background(), full, 0); n != len(text) || units != 5 || err != nil || string(full) != text || !utf8.Valid(full) {
				t.Fatalf("ReadTextAt full value = (%d, %d, %v, %q), want (%d, 5, nil, %q)", n, units, err, full, len(text), text)
			}

			insidePair := make([]byte, 4)
			if n, units, err := value.ReadTextAt(insidePair, 3); n != 0 || units != 0 || err == nil {
				t.Fatalf("ReadTextAt inside surrogate pair = (%d, %d, %v), want error", n, units, err)
			}

			if handle.offset != beforeOffset || handle.fetchOffset != beforeFetch || !bytes.Equal(handle.pending, beforePending) || !bytes.Equal(handle.utf8Carry, beforeCarry) {
				t.Fatalf("ReadTextAt changed sequential state: offset=%d fetch=%d pending=%q carry=%q", handle.offset, handle.fetchOffset, handle.pending, handle.utf8Carry)
			}
			remaining := make([]byte, len("é"))
			if n, err := handle.ReadContext(context.Background(), remaining); n != len(remaining) || err != nil || string(remaining) != "é" {
				t.Fatalf("sequential read after ReadTextAt = (%d, %v, %q), want é", n, err, remaining)
			}

			if len(readOffsets) == 0 || readOffsets[0] != 1 {
				t.Fatalf("first Oracle text read offset = %v, want 1", readOffsets)
			}
		})
	}
}

// TestLOBHandle_AcceptedWriteBytes verifies that write acknowledgements map to
// UTF-8 byte counts only at Unicode scalar boundaries.
func TestLOBHandle_AcceptedWriteBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		kind        Kind
		data        string
		logical     uint64
		wantBytes   int
		wantShort   bool
		wantInvalid bool
	}{
		{name: "blob complete", kind: BLOB, data: "abc", logical: 3, wantBytes: 3},
		{name: "blob short", kind: BLOB, data: "abc", logical: 2, wantBytes: 2, wantShort: true},
		{name: "blob over acknowledgement", kind: BLOB, data: "abc", logical: 4, wantInvalid: true},
		{name: "clob complete", kind: CLOB, data: "Aé中", logical: 3, wantBytes: len("Aé中")},
		{name: "clob short before supplementary character", kind: CLOB, data: "A🙂B", logical: 1, wantBytes: 1, wantShort: true},
		{name: "clob short after supplementary character", kind: CLOB, data: "A🙂B", logical: 3, wantBytes: len("A🙂"), wantShort: true},
		{name: "clob split supplementary character", kind: CLOB, data: "A🙂B", logical: 2, wantInvalid: true},
		{name: "clob over acknowledgement", kind: CLOB, data: "A🙂B", logical: 5, wantInvalid: true},
		{name: "nclob complete", kind: NCLOB, data: "🙂", logical: 2, wantBytes: len("🙂")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotBytes, err := acceptedWriteBytes(test.kind, []byte(test.data), test.logical)
			if gotBytes != test.wantBytes {
				t.Fatalf("accepted bytes = %d, want %d", gotBytes, test.wantBytes)
			}
			if test.wantInvalid {
				var sqlErr oracleErrors.SQLError
				if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.InvalidLOBBuffer) {
					t.Fatalf("error = %v, want %s", err, oracleErrors.InvalidLOBBuffer)
				}
				return
			}
			if test.wantShort {
				if !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("error = %v, want io.ErrShortWrite", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("acceptedWriteBytes returned error: %v", err)
			}
		})
	}
}

// TestLOBHandle_PreCanceledMutationsDoNotInvalidate verifies canceled calls
// rejected before RPC dispatch leave the connection-scoped handle usable.
func TestLOBHandle_PreCanceledMutationsDoNotInvalidate(t *testing.T) {
	t.Parallel()

	writeCalls := 0
	trimCalls := 0
	raw := &lobHandleTestRawConn{
		writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
			writeCalls++
			return uint64(len(data)), nil
		},
		trimFn: func(_ context.Context, _ uint8, _ []byte, length uint64) (uint64, error) {
			trimCalls++
			return length, nil
		},
	}
	value := newActiveLOBHandle(t, raw, BLOB)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := value.WriteContext(canceled, []byte("x")); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled WriteContext = (%d, %v), want (0, context.Canceled)", n, err)
	}
	if err := value.Trim(canceled, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Trim = %v, want context.Canceled", err)
	}
	if writeCalls != 0 || trimCalls != 0 {
		t.Fatalf("pre-canceled RPC calls = (write:%d trim:%d), want (0, 0)", writeCalls, trimCalls)
	}
	if value.invalidated {
		t.Fatal("pre-canceled mutations invalidated the handle")
	}
	if n, err := value.WriteContext(context.Background(), []byte("ok")); n != 2 || err != nil {
		t.Fatalf("WriteContext after canceled calls = (%d, %v), want (2, nil)", n, err)
	}
	if writeCalls != 1 {
		t.Fatalf("WriteContext RPC calls = %d, want 1", writeCalls)
	}
}

// TestLOBHandle_MutationsUpdatePrefixCompleteness ensures writes discard the
// transferred prefix state, while Trim(0) records a known empty value.
func TestLOBHandle_MutationsUpdatePrefixCompleteness(t *testing.T) {
	t.Parallel()

	t.Run("short write clears complete prefix", func(t *testing.T) {
		raw := &lobHandleTestRawConn{
			writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) {
				return 0, nil
			},
		}
		value := newActiveLOBHandle(t, raw, BLOB)
		value.pending = []byte("old")
		value.readComplete = true

		if n, err := value.WriteContext(context.Background(), []byte("x")); n != 0 || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("WriteContext = (%d, %v), want (0, io.ErrShortWrite)", n, err)
		}
		if value.readComplete || len(value.pending) != 0 {
			t.Fatalf("state after short write = complete:%t pending:%q, want false and empty", value.readComplete, value.pending)
		}
	})

	t.Run("trim updates complete prefix", func(t *testing.T) {
		readCalls := 0
		raw := &lobHandleTestRawConn{
			readFn: func(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error) {
				readCalls++
				return nil, 0, nil
			},
			trimFn: func(_ context.Context, _ uint8, _ []byte, length uint64) (uint64, error) {
				return length, nil
			},
		}
		value := newActiveLOBHandle(t, raw, BLOB)
		value.pending = []byte("old")
		value.readComplete = true
		if err := value.Trim(context.Background(), 3); err != nil {
			t.Fatalf("Trim(3) returned error: %v", err)
		}
		if value.readComplete || len(value.pending) != 0 {
			t.Fatalf("state after Trim(3) = complete:%t pending:%q, want false and empty", value.readComplete, value.pending)
		}

		value.readComplete = false
		if err := value.Trim(context.Background(), 0); err != nil {
			t.Fatalf("Trim(0) returned error: %v", err)
		}
		if !value.readComplete {
			t.Fatal("Trim(0) did not mark the resulting empty LOB complete")
		}
		if n, err := value.hasUnreadDataContext(context.Background()); n || err != nil {
			t.Fatalf("hasUnreadDataContext after Trim(0) = (%t, %v), want (false, nil)", n, err)
		}
		buffer := make([]byte, 1)
		if n, err := value.ReadContext(context.Background(), buffer); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("ReadContext after Trim(0) = (%d, %v), want (0, EOF)", n, err)
		}
		if readCalls != 0 {
			t.Fatalf("empty complete prefix issued %d locator reads, want 0", readCalls)
		}
	})
}

// TestLOBHandle_CLOBWriteRequiresCharacterBoundary verifies a byte-sized read
// can split UTF-8 output, but writes wait until the caller reaches an Oracle
// character boundary.
func TestLOBHandle_CLOBWriteRequiresCharacterBoundary(t *testing.T) {
	t.Parallel()

	var writeOffset uint64
	writeCalls := 0
	raw := &lobHandleTestRawConn{
		readFn: func(_ context.Context, _ uint8, _ []byte, offset, _ uint64) ([]byte, uint64, error) {
			if offset != 1 {
				return nil, 0, nil
			}
			return []byte("🙂A"), 3, nil
		},
		writeFn: func(_ context.Context, _ uint8, _ []byte, offset uint64, payload []byte) (uint64, error) {
			writeCalls++
			writeOffset = offset
			if string(payload) != "Z" {
				t.Fatalf("write payload = %q, want Z", payload)
			}
			return 1, nil
		},
	}
	value := newActiveLOBHandle(t, raw, CLOB)

	first := make([]byte, 1)
	if n, err := value.ReadContext(context.Background(), first); n != 1 || err != nil {
		t.Fatalf("first ReadContext = (%d, %v), want one byte", n, err)
	}
	if value.offset != 1 || len(value.utf8Carry) == 0 {
		t.Fatalf("state inside supplementary character = caller:%d carry:%q, want caller:1 with carry", value.offset, value.utf8Carry)
	}
	if n, err := value.WriteContext(context.Background(), []byte("Z")); n != 0 {
		t.Fatalf("WriteContext inside character returned %d bytes, want 0 (error %v)", n, err)
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if writeCalls != 0 {
		t.Fatalf("write calls inside character = %d, want 0", writeCalls)
	}

	remainder := make([]byte, 3)
	if n, err := value.ReadContext(context.Background(), remainder); n != 3 || err != nil || string(remainder) != "🙂"[1:] {
		t.Fatalf("read remainder = (%d, %v, %q), want the final three emoji bytes", n, err, remainder)
	}
	if value.offset != 3 || len(value.utf8Carry) != 0 {
		t.Fatalf("state after character = caller:%d carry:%q, want caller:3 at boundary", value.offset, value.utf8Carry)
	}
	if n, err := value.WriteContext(context.Background(), []byte("Z")); n != 1 || err != nil {
		t.Fatalf("WriteContext at character boundary = (%d, %v), want (1, nil)", n, err)
	}
	if writeOffset != 3 || writeCalls != 1 {
		t.Fatalf("write = offset:%d calls:%d, want offset:3 calls:1", writeOffset, writeCalls)
	}
}

// TestLOBHandle_ReadWriteAndWriteTo verifies read/write wrappers, validation,
// cursor invalidation, and the success and error paths of WriteTo.
func TestLOBHandle_ReadWriteAndWriteTo(t *testing.T) {
	t.Parallel()

	// Read and Write use context.Background and advance the shared cursor.
	var writeOffset uint64
	value := newActiveLOBHandle(t, &lobHandleTestRawConn{
		readFn: func(_ context.Context, _ uint8, _ []byte, offset, _ uint64) ([]byte, uint64, error) {
			if offset != 1 {
				t.Fatalf("LobRead offset = %d, want 1", offset)
			}
			return []byte("abc"), 3, nil
		},
		writeFn: func(_ context.Context, _ uint8, _ []byte, offset uint64, data []byte) (uint64, error) {
			writeOffset = offset
			if string(data) != "xy" {
				t.Fatalf("LobWrite data = %q, want xy", data)
			}
			return 2, nil
		},
	}, BLOB)
	if n, err := value.Read(make([]byte, 3)); n != 3 || err != nil {
		t.Fatalf("Read = (%d, %v), want (3, nil)", n, err)
	}
	if n, err := value.Write([]byte("xy")); n != 2 || err != nil || writeOffset != 4 {
		t.Fatalf("Write = (%d, %v), offset %d; want (2, nil), offset 4", n, err, writeOffset)
	}
	if n, err := value.Write(nil); n != 0 || err != nil {
		t.Fatalf("empty Write = (%d, %v), want (0, nil)", n, err)
	}

	// ReadContext returns immediately for an empty destination and reports EOF.
	readCalls := 0
	empty := newActiveLOBHandle(t, &lobHandleTestRawConn{
		readFn: func(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error) {
			readCalls++
			return nil, 0, nil
		},
	}, BLOB)
	if n, err := empty.ReadContext(context.Background(), nil); n != 0 || err != nil || readCalls != 0 {
		t.Fatalf("empty ReadContext = (%d, %v), calls %d; want (0, nil), calls 0", n, err, readCalls)
	}
	if n, err := empty.ReadContext(context.Background(), make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("EOF ReadContext = (%d, %v), want (0, io.EOF)", n, err)
	}
	readErr := errors.New("read failed")
	failedRead := newActiveLOBHandle(t, &lobHandleTestRawConn{readFn: readOnce(nil, 0, readErr)}, CLOB)
	if n, err := failedRead.ReadContext(context.Background(), make([]byte, 1)); n != 0 || !errors.Is(err, readErr) {
		t.Fatalf("read-error ReadContext = (%d, %v), want (0, %v)", n, err, readErr)
	}

	// Character LOBs reject malformed UTF-8 before entering the raw driver.
	invalidUTF8 := newActiveLOBHandle(t, &lobHandleTestRawConn{}, CLOB)
	if _, err := invalidUTF8.Write([]byte{0xff}); err == nil {
		t.Fatal("Write accepted invalid CLOB UTF-8")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	// An impossible acknowledgement invalidates the cursor after the response.
	invalidAck := newActiveLOBHandle(t, &lobHandleTestRawConn{
		writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) { return 3, nil },
	}, BLOB)
	if _, err := invalidAck.Write([]byte("ab")); err == nil {
		t.Fatal("Write accepted an acknowledgement larger than the payload")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if _, err := invalidAck.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read succeeded after an invalid write acknowledgement")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	writeErr := errors.New("write failed")
	failedWrite := newActiveLOBHandle(t, &lobHandleTestRawConn{
		writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) { return 0, writeErr },
	}, BLOB)
	if _, err := failedWrite.Write([]byte("data")); !errors.Is(err, writeErr) {
		t.Fatalf("Write error = %v, want %v", err, writeErr)
	}

	// Table-driven cases cover all WriteTo result classifications.
	writerErr := errors.New("writer failed")
	cases := []struct {
		name       string
		raw        *lobHandleTestRawConn
		writer     io.Writer
		wantBytes  int64
		wantErr    error
		wantCode   oracleErrors.ErrorCode
		wantOutput string
	}{
		{name: "nil writer", wantCode: oracleErrors.InvalidLOBBuffer},
		{name: "complete", raw: &lobHandleTestRawConn{readFn: readOnce([]byte("data"), 4, nil)}, writer: &bytes.Buffer{}, wantBytes: 4, wantOutput: "data"},
		{name: "writer error", raw: &lobHandleTestRawConn{readFn: readOnce([]byte("data"), 4, nil)}, writer: &lobHandleTestWriter{err: writerErr, max: -1}, wantBytes: 4, wantErr: writerErr},
		{name: "short writer", raw: &lobHandleTestRawConn{readFn: readOnce([]byte("data"), 4, nil)}, writer: &lobHandleTestWriter{max: 1}, wantBytes: 1, wantErr: io.ErrShortWrite},
		{name: "read error", raw: &lobHandleTestRawConn{readFn: readOnce(nil, 0, readErr)}, writer: io.Discard, wantErr: readErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var value *lobHandle
			if tc.raw != nil {
				value = newActiveLOBHandle(t, tc.raw, BLOB)
			}
			var got int64
			var err error
			if value == nil {
				var zero lobHandle
				got, err = zero.WriteTo(tc.writer)
			} else {
				got, err = value.WriteTo(tc.writer)
			}
			if got != tc.wantBytes {
				t.Errorf("WriteTo bytes = %d, want %d", got, tc.wantBytes)
			}
			if tc.wantCode != "" {
				requireLOBHandleTestErrorCode(t, err, tc.wantCode)
			} else if !errors.Is(err, tc.wantErr) {
				t.Errorf("WriteTo error = %v, want %v", err, tc.wantErr)
			}
			if output, ok := tc.writer.(*bytes.Buffer); ok && output.String() != tc.wantOutput {
				t.Errorf("WriteTo output = %q, want %q", output.String(), tc.wantOutput)
			}
		})
	}
}

// TestLOBHandle_WriteContextChunksBoundsRequestsAndPreservesTextCharacters
// verifies bounded BLOB writes and UTF-8-safe CLOB/NCLOB chunks with UTF-16
// cursor accounting.
func TestLOBHandle_WriteContextChunksBoundsRequestsAndPreservesTextCharacters(t *testing.T) {
	t.Run("BLOB bounded byte chunks", func(t *testing.T) {
		chunkSize := internallob.DefaultBlobLobChunkBytes
		payload := bytes.Repeat([]byte{'b'}, chunkSize*2+7)
		type writeCall struct {
			offset  uint64
			payload []byte
		}
		var calls []writeCall
		raw := &lobHandleTestRawConn{
			writeFn: func(_ context.Context, _ uint8, _ []byte, offset uint64, data []byte) (uint64, error) {
				calls = append(calls, writeCall{offset: offset, payload: append([]byte(nil), data...)})
				return uint64(len(data)), nil
			},
		}
		handle := newActiveLOBHandle(t, raw, BLOB)

		if n, err := handle.WriteContext(context.Background(), payload); n != len(payload) || err != nil {
			t.Fatalf("WriteContext = (%d, %v), want (%d, nil)", n, err, len(payload))
		}
		if len(calls) != 3 {
			t.Fatalf("LobWrite calls = %d, want 3", len(calls))
		}
		var joined bytes.Buffer
		for index, call := range calls {
			wantSize := chunkSize
			if index == 2 {
				wantSize = 7
			}
			wantOffset := uint64(1 + index*chunkSize)
			if len(call.payload) != wantSize || call.offset != wantOffset {
				t.Errorf("LobWrite[%d] = (offset %d, %d bytes), want (offset %d, %d bytes)", index, call.offset, len(call.payload), wantOffset, wantSize)
			}
			joined.Write(call.payload)
		}
		if !bytes.Equal(joined.Bytes(), payload) {
			t.Fatal("bounded BLOB chunks did not preserve the input bytes")
		}
		if wantOffset := uint64(len(payload) + 1); handle.offset != wantOffset {
			t.Fatalf("final cursor = %d, want %d", handle.offset, wantOffset)
		}
	})

	for _, test := range []struct {
		name string
		kind Kind
	}{{name: "CLOB", kind: CLOB}, {name: "NCLOB", kind: NCLOB}} {
		t.Run(test.name, func(t *testing.T) {
			kind := test.kind
			maxChars := internallob.DefaultCharacterLobChunkChars
			firstChunk := strings.Repeat("a", maxChars-1) + "🙂"
			payload := []byte(firstChunk + "Z")
			type writeCall struct {
				offset  uint64
				payload []byte
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
					calls = append(calls, writeCall{offset: offset, payload: append([]byte(nil), data...)})
					return lobTestUTF16Units(data), nil
				},
			}
			handle := newActiveLOBHandle(t, raw, kind)

			if n, err := handle.WriteContext(context.Background(), payload); n != len(payload) || err != nil {
				t.Fatalf("WriteContext = (%d, %v), want (%d, nil)", n, err, len(payload))
			}
			if len(calls) != 2 {
				t.Fatalf("LobWrite calls = %d, want 2", len(calls))
			}
			if got := string(calls[0].payload); got != firstChunk {
				t.Fatalf("first chunk = %d bytes/%d runes, want %d bytes/%d runes ending at the complete supplementary character", len(got), utf8.RuneCountInString(got), len(firstChunk), utf8.RuneCountInString(firstChunk))
			}
			if got := string(calls[1].payload); got != "Z" {
				t.Fatalf("second chunk = %q, want Z", got)
			}
			if calls[0].offset != 1 || calls[1].offset != uint64(maxChars+2) {
				t.Fatalf("chunk offsets = [%d %d], want [1 %d]", calls[0].offset, calls[1].offset, maxChars+2)
			}
			if wantOffset := uint64(maxChars + 3); handle.offset != wantOffset {
				t.Fatalf("final cursor = %d, want %d", handle.offset, wantOffset)
			}
		})
	}
}

// TestLOBHandle_WriteContextPartialProgress verifies short acknowledgements,
// cancellation between acknowledged chunks, and ambiguous errors after an
// acknowledged prefix.
func TestLOBHandle_WriteContextPartialProgress(t *testing.T) {
	chunkSize := internallob.DefaultBlobLobChunkBytes
	payload := bytes.Repeat([]byte{'x'}, chunkSize+3)

	t.Run("short acknowledgement", func(t *testing.T) {
		calls := 0
		handle := newActiveLOBHandle(t, &lobHandleTestRawConn{
			writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) {
				calls++
				return 5, nil
			},
		}, BLOB)

		if n, err := handle.WriteContext(context.Background(), payload); n != 5 || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("WriteContext = (%d, %v), want (5, io.ErrShortWrite)", n, err)
		}
		if calls != 1 || handle.offset != 6 || handle.invalidated {
			t.Fatalf("state after short ack = calls:%d offset:%d invalidated:%t, want 1, 6, false", calls, handle.offset, handle.invalidated)
		}
	})

	t.Run("cancellation between acknowledged chunks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		handle := newActiveLOBHandle(t, &lobHandleTestRawConn{
			writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
				calls++
				if calls == 1 {
					cancel()
				}
				return uint64(len(data)), nil
			},
		}, BLOB)

		if n, err := handle.WriteContext(ctx, payload); n != chunkSize || !errors.Is(err, context.Canceled) {
			t.Fatalf("WriteContext = (%d, %v), want (%d, context.Canceled)", n, err, chunkSize)
		}
		if calls != 1 || handle.offset != uint64(chunkSize+1) || handle.invalidated {
			t.Fatalf("state after cancellation = calls:%d offset:%d invalidated:%t", calls, handle.offset, handle.invalidated)
		}
		if n, err := handle.Write([]byte("z")); n != 1 || err != nil {
			t.Fatalf("Write after inter-chunk cancellation = (%d, %v), want (1, nil)", n, err)
		}
	})

	t.Run("ambiguous error after acknowledged chunk", func(t *testing.T) {
		writeErr := errors.New("second chunk failed after dispatch")
		calls := 0
		handle := newActiveLOBHandle(t, &lobHandleTestRawConn{
			writeFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, data []byte) (uint64, error) {
				calls++
				if calls == 2 {
					return 0, writeErr
				}
				return uint64(len(data)), nil
			},
		}, BLOB)

		if n, err := handle.WriteContext(context.Background(), payload); n != chunkSize || !errors.Is(err, writeErr) {
			t.Fatalf("WriteContext = (%d, %v), want (%d, %v)", n, err, chunkSize, writeErr)
		}
		if calls != 2 || handle.offset != uint64(chunkSize+1) || !handle.invalidated {
			t.Fatalf("state after ambiguous error = calls:%d offset:%d invalidated:%t", calls, handle.offset, handle.invalidated)
		}
	})
}

// TestLOBHandle_TextWriteRequiresCompleteUTF8PerCall locks in the documented
// CLOB/NCLOB contract that incomplete characters are not carried across writes.
func TestLOBHandle_TextWriteRequiresCompleteUTF8PerCall(t *testing.T) {
	for _, test := range []struct {
		name string
		kind Kind
	}{{name: "CLOB", kind: CLOB}, {name: "NCLOB", kind: NCLOB}} {
		t.Run(test.name, func(t *testing.T) {
			writeCalls := 0
			handle := newActiveLOBHandle(t, &lobHandleTestRawConn{
				writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) {
					writeCalls++
					return 0, nil
				},
			}, test.kind)

			if n, err := handle.WriteContext(context.Background(), []byte("🙂")[:1]); n != 0 {
				t.Fatalf("WriteContext accepted %d incomplete UTF-8 bytes, want 0 (error %v)", n, err)
			} else {
				requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
			}
			if writeCalls != 0 {
				t.Fatalf("incomplete UTF-8 triggered %d LobWrite calls, want 0", writeCalls)
			}
		})
	}
}

// TestLOBHandle_IsTemporary reports temporary status for temporary and
// persistent typed LOB handles without performing an RPC.
func TestLOBHandle_IsTemporary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		temporary bool
		want      bool
	}{
		{name: "temporary", temporary: true, want: true},
		{name: "persistent", temporary: false, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := &lobHandle{temporary: test.temporary}
			if got := value.IsTemporary(); got != test.want {
				t.Fatalf("IsTemporary() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestLOBHandle_TrimClearsPendingCLOB verifies that successful trimming drops
// UTF-8 bytes prefetched before the server-side length changed.
func TestLOBHandle_TrimClearsPendingCLOB(t *testing.T) {
	t.Parallel()

	readCalls := 0
	raw := &lobHandleTestRawConn{
		readFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, _ uint64) ([]byte, uint64, error) {
			readCalls++
			if readCalls == 1 {
				return []byte("A🙂B"), 4, nil
			}
			return nil, 0, nil
		},
		trimFn: func(_ context.Context, _ uint8, _ []byte, length uint64) (uint64, error) {
			if length != 1 {
				t.Fatalf("Trim length = %d, want 1", length)
			}
			return length, nil
		},
	}
	conn := newLOBHandleTestConn(t, raw)
	value := &lobHandle{
		conn:    conn,
		kind:    CLOB,
		locator: []byte("locator"),
		offset:  1,
	}

	first := make([]byte, 1)
	if n, err := value.ReadContext(context.Background(), first); n != 1 || err != nil || string(first) != "A" {
		t.Fatalf("first ReadContext = (%d, %v, %q), want (1, nil, A)", n, err, first)
	}
	if len(value.pending) == 0 {
		t.Fatal("CLOB read did not retain prefetched data")
	}
	if err := value.Trim(context.Background(), 1); err != nil {
		t.Fatalf("Trim returned error: %v", err)
	}
	if len(value.pending) != 0 {
		t.Fatalf("pending bytes after Trim = %q, want empty", value.pending)
	}

	if n, err := value.ReadContext(context.Background(), make([]byte, 8)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadContext after Trim = (%d, %v), want (0, io.EOF)", n, err)
	}
}

// TestLOBHandle_TrimTextCarryRequiresCharacterBoundary verifies a nonzero
// CLOB/NCLOB trim is rejected before the RPC while a returned UTF-8 character
// is incomplete, while Trim(0) can still reset that read state.
func TestLOBHandle_TrimTextCarryRequiresCharacterBoundary(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		kind Kind
	}{{name: "CLOB", kind: CLOB}, {name: "NCLOB", kind: NCLOB}} {
		t.Run(test.name, func(t *testing.T) {
			const text = "éZ"
			trimCalls := 0
			raw := &lobHandleTestRawConn{
				readFn: func(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error) {
					return []byte(text), 2, nil
				},
				trimFn: func(_ context.Context, _ uint8, _ []byte, length uint64) (uint64, error) {
					trimCalls++
					return length, nil
				},
			}

			t.Run("nonzero trim rejected before RPC", func(t *testing.T) {
				trimCalls = 0
				handle := newActiveLOBHandle(t, raw, test.kind)
				first := make([]byte, 1)
				if n, err := handle.ReadContext(context.Background(), first); n != 1 || err != nil || first[0] != 0xC3 {
					t.Fatalf("first ReadContext = (%d, %v, %x), want (1, nil, c3)", n, err, first)
				}
				beforeOffset, beforeFetch := handle.offset, handle.fetchOffset
				beforePending := append([]byte(nil), handle.pending...)
				beforeCarry := append([]byte(nil), handle.utf8Carry...)
				if len(beforeCarry) == 0 {
					t.Fatal("read did not retain the partial UTF-8 character")
				}

				if err := handle.Trim(context.Background(), 1); err == nil {
					t.Fatal("nonzero Trim succeeded with a partial UTF-8 character")
				} else {
					requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
				}
				if trimCalls != 0 {
					t.Fatalf("LobTrim calls = %d, want 0", trimCalls)
				}
				if handle.offset != beforeOffset || handle.fetchOffset != beforeFetch || !bytes.Equal(handle.pending, beforePending) || !bytes.Equal(handle.utf8Carry, beforeCarry) {
					t.Fatalf("state changed after rejected Trim: offset=%d fetch=%d pending=%x carry=%x", handle.offset, handle.fetchOffset, handle.pending, handle.utf8Carry)
				}

				remainder := make([]byte, len(text)-len(first))
				if n, err := handle.ReadContext(context.Background(), remainder); n != len(remainder) || err != nil || !bytes.Equal(append(first, remainder...), []byte(text)) {
					t.Fatalf("read after rejected Trim = (%d, %v, %x), want remaining bytes completing %q", n, err, remainder, text)
				}
				if err := handle.Trim(context.Background(), 1); err != nil {
					t.Fatalf("Trim at a character boundary returned error: %v", err)
				}
				if trimCalls != 1 {
					t.Fatalf("LobTrim calls after boundary trim = %d, want 1", trimCalls)
				}
			})

			t.Run("zero trim resets partial read state", func(t *testing.T) {
				trimCalls = 0
				handle := newActiveLOBHandle(t, raw, test.kind)
				if n, err := handle.ReadContext(context.Background(), make([]byte, 1)); n != 1 || err != nil {
					t.Fatalf("ReadContext = (%d, %v), want one byte", n, err)
				}
				if len(handle.utf8Carry) == 0 {
					t.Fatal("read did not retain the partial UTF-8 character")
				}
				if err := handle.Trim(context.Background(), 0); err != nil {
					t.Fatalf("Trim(0) returned error: %v", err)
				}
				if trimCalls != 1 || handle.offset != 1 || handle.fetchOffset != 1 || len(handle.pending) != 0 || len(handle.utf8Carry) != 0 || !handle.readComplete {
					t.Fatalf("state after Trim(0) = calls:%d offset:%d fetch:%d pending:%x carry:%x complete:%t", trimCalls, handle.offset, handle.fetchOffset, handle.pending, handle.utf8Carry, handle.readComplete)
				}
			})
		})
	}
}

// TestLOBHandle_TrimZeroResetsCursor verifies trim-to-zero restarts both
// sequential read and write positions so a following write replaces content.
func TestLOBHandle_TrimZeroResetsCursor(t *testing.T) {
	t.Parallel()

	var writeOffset uint64
	raw := &lobHandleTestRawConn{
		readFn: func(_ context.Context, _ uint8, _ []byte, _ uint64, _ uint64) ([]byte, uint64, error) {
			return []byte("data"), 4, nil
		},
		trimFn: func(_ context.Context, _ uint8, _ []byte, length uint64) (uint64, error) {
			if length != 0 {
				t.Fatalf("trim length = %d, want 0", length)
			}
			return 0, nil
		},
		writeFn: func(_ context.Context, _ uint8, _ []byte, offset uint64, payload []byte) (uint64, error) {
			writeOffset = offset
			return uint64(len(payload)), nil
		},
	}
	value := newActiveLOBHandle(t, raw, BLOB)
	if n, err := value.ReadContext(context.Background(), make([]byte, 1)); n != 1 || err != nil {
		t.Fatalf("ReadContext = (%d, %v), want one byte", n, err)
	}
	if err := value.Trim(context.Background(), 0); err != nil {
		t.Fatalf("Trim(0) returned error: %v", err)
	}
	if value.offset != 1 || value.fetchOffset != 1 || len(value.pending) != 0 {
		t.Fatalf("state after Trim(0) = caller:%d fetch:%d pending:%d, want 1, 1, 0", value.offset, value.fetchOffset, len(value.pending))
	}
	if n, err := value.WriteContext(context.Background(), []byte("new")); n != 3 || err != nil {
		t.Fatalf("WriteContext after Trim(0) = (%d, %v), want (3, nil)", n, err)
	}
	if writeOffset != 1 {
		t.Fatalf("write offset after Trim(0) = %d, want 1", writeOffset)
	}
}

// TestLOBHandle_OpenRequiresCloseServerBeforeClose verifies an explicitly
// opened persistent LOB cannot be locally closed before server close succeeds.
func TestLOBHandle_OpenRequiresCloseServerBeforeClose(t *testing.T) {
	t.Parallel()

	raw := &lobHandleTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			return locator, nil
		},
	}
	conn := newLOBHandleTestConn(t, raw)
	value := &lobHandle{conn: conn, kind: BLOB, locator: []byte("locator"), offset: 1}

	if opened, err := value.Open(context.Background(), ReadWrite); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	requireLOBHandleTestErrorCode(t, value.Close(), oracleErrors.LobOpen)
	if value.closed {
		t.Fatal("Close marked the handle closed while server close was required")
	}
	if err := value.CloseServer(context.Background()); err != nil {
		t.Fatalf("CloseServer returned error: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("Close after CloseServer returned error: %v", err)
	}
}

// TestLOBHandle_OpenRequiresCloseServerBeforeFree verifies Free also preserves
// an explicitly opened persistent handle until CloseServer succeeds.
func TestLOBHandle_OpenRequiresCloseServerBeforeFree(t *testing.T) {
	t.Parallel()

	raw := &lobHandleTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			return locator, nil
		},
	}
	conn := newLOBHandleTestConn(t, raw)
	value := &lobHandle{conn: conn, kind: CLOB, locator: []byte("locator"), offset: 1}

	if opened, err := value.Open(context.Background(), ReadOnly); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	requireLOBHandleTestErrorCode(t, value.Free(context.Background()), oracleErrors.LobOpen)
	if value.closed || value.freed {
		t.Fatal("Free changed local state while server close was required")
	}
	if err := value.CloseServer(context.Background()); err != nil {
		t.Fatalf("CloseServer returned error: %v", err)
	}
	if err := value.Free(context.Background()); err != nil {
		t.Fatalf("Free after CloseServer returned error: %v", err)
	}
}

// TestLOBHandle_CloseServerFailureRequiresReconciliation verifies a failed
// server close marks the cached state unknown until IsOpen checks Oracle.
func TestLOBHandle_CloseServerFailureRequiresReconciliation(t *testing.T) {
	t.Parallel()

	closeCalls := 0
	serverOpen := false
	closeErr := errors.New("server close failed")
	raw := &lobHandleTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			serverOpen = true
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			closeCalls++
			if closeCalls == 1 {
				return nil, closeErr
			}
			serverOpen = false
			return locator, nil
		},
		isOpenFn: func(context.Context, uint8, []byte) (bool, error) { return serverOpen, nil },
	}
	conn := newLOBHandleTestConn(t, raw)
	value := &lobHandle{conn: conn, kind: BLOB, locator: []byte("locator"), offset: 1}

	if opened, err := value.Open(context.Background(), ReadWrite); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	if err := value.CloseServer(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("first CloseServer error = %v, want %v", err, closeErr)
	}
	if value.serverOpenState != persistentOpenUnknown {
		t.Fatalf("state after failed CloseServer = %d, want unknown", value.serverOpenState)
	}
	requireLOBHandleTestErrorCode(t, value.Close(), oracleErrors.LobOpenStateUnknown)
	if err := value.CloseServer(context.Background()); err == nil {
		t.Fatal("CloseServer retried without a state probe")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if closeCalls != 1 {
		t.Fatalf("CloseServer calls before reconciliation = %d, want 1", closeCalls)
	}
	if open, err := value.IsOpen(context.Background()); !open || err != nil {
		t.Fatalf("IsOpen reconciliation = (%t, %v), want (true, nil)", open, err)
	}
	if value.serverOpenState != persistentOpenOpen {
		t.Fatalf("state after IsOpen = %d, want open", value.serverOpenState)
	}
	if err := value.CloseServer(context.Background()); err != nil {
		t.Fatalf("retry CloseServer returned error: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("Close after successful retry returned error: %v", err)
	}
	if closeCalls != 2 {
		t.Fatalf("CloseServer calls = %d, want 2", closeCalls)
	}
}

// TestLOBHandle_OpenCancellationRequiresIsOpenReconciliation covers an Open
// that changes Oracle state before cancellation is returned to the caller.
func TestLOBHandle_OpenCancellationRequiresIsOpenReconciliation(t *testing.T) {
	t.Parallel()

	serverOpen := false
	openCalls := 0
	probeCalls := 0
	probeErr := errors.New("state probe failed")
	canceled, cancel := context.WithCancel(context.Background())
	raw := &lobHandleTestRawConn{
		openFn: func(ctx context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			openCalls++
			serverOpen = true
			cancel()
			if err := ctx.Err(); err != nil {
				return false, nil, err
			}
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			serverOpen = false
			return locator, nil
		},
		isOpenFn: func(context.Context, uint8, []byte) (bool, error) {
			probeCalls++
			if probeCalls == 1 {
				return false, probeErr
			}
			return serverOpen, nil
		},
	}
	value := newActiveLOBHandle(t, raw, BLOB)
	preCanceled, cancelPreCanceled := context.WithCancel(context.Background())
	cancelPreCanceled()
	if _, err := value.Open(preCanceled, ReadWrite); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Open = %v, want context.Canceled", err)
	}
	if value.serverOpenState != persistentOpenClosed || openCalls != 0 {
		t.Fatalf("pre-canceled Open changed state=%d calls=%d, want closed and no RPC", value.serverOpenState, openCalls)
	}
	if opened, err := value.Open(canceled, ReadWrite); opened || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Open = (%t, %v), want (false, context.Canceled)", opened, err)
	}
	if value.serverOpenState != persistentOpenUnknown || !serverOpen {
		t.Fatalf("after canceled Open state=%d serverOpen=%t, want unknown and open", value.serverOpenState, serverOpen)
	}
	if opened, err := value.Open(context.Background(), ReadWrite); opened || err == nil {
		t.Fatalf("Open retry without probe = (%t, %v), want unknown-state error", opened, err)
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if err := value.CloseServer(context.Background()); err == nil {
		t.Fatal("CloseServer succeeded without state reconciliation")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if err := value.Close(); err == nil {
		t.Fatal("Close released a handle with unknown server state")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if err := value.Free(context.Background()); err == nil {
		t.Fatal("Free released a handle with unknown server state")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if probeCalls != 0 {
		t.Fatalf("Close/Free performed %d hidden state probes, want 0", probeCalls)
	}
	if _, err := value.IsOpen(context.Background()); !errors.Is(err, probeErr) {
		t.Fatalf("failed IsOpen probe = %v, want %v", err, probeErr)
	}
	if value.serverOpenState != persistentOpenUnknown {
		t.Fatalf("state after failed IsOpen probe = %d, want unknown", value.serverOpenState)
	}
	if open, err := value.IsOpen(context.Background()); !open || err != nil {
		t.Fatalf("successful IsOpen probe = (%t, %v), want (true, nil)", open, err)
	}
	if value.serverOpenState != persistentOpenOpen {
		t.Fatalf("state after successful IsOpen probe = %d, want open", value.serverOpenState)
	}
	if openCalls != 1 {
		t.Fatalf("Open calls = %d, want 1", openCalls)
	}
	if err := value.CloseServer(context.Background()); err != nil {
		t.Fatalf("CloseServer after reconciliation returned error: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("Close after reconciliation returned error: %v", err)
	}
}

// TestLOBHandle_CloseServerCancellationRequiresIsOpenReconciliation covers a
// CloseServer that takes effect before cancellation is returned to the caller.
func TestLOBHandle_CloseServerCancellationRequiresIsOpenReconciliation(t *testing.T) {
	t.Parallel()

	serverOpen := false
	closeCalls := 0
	openCalls := 0
	probeCalls := 0
	canceled, cancel := context.WithCancel(context.Background())
	raw := &lobHandleTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			openCalls++
			serverOpen = true
			return true, locator, nil
		},
		closeFn: func(ctx context.Context, _ uint8, locator []byte) ([]byte, error) {
			closeCalls++
			serverOpen = false
			cancel()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return locator, nil
		},
		isOpenFn: func(context.Context, uint8, []byte) (bool, error) {
			probeCalls++
			return serverOpen, nil
		},
	}
	value := newActiveLOBHandle(t, raw, BLOB)
	if opened, err := value.Open(context.Background(), ReadWrite); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	preCanceled, cancelPreCanceled := context.WithCancel(context.Background())
	cancelPreCanceled()
	if err := value.CloseServer(preCanceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled CloseServer = %v, want context.Canceled", err)
	}
	if value.serverOpenState != persistentOpenOpen || closeCalls != 0 {
		t.Fatalf("pre-canceled CloseServer changed state=%d calls=%d, want open and no RPC", value.serverOpenState, closeCalls)
	}
	if err := value.CloseServer(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled CloseServer = %v, want context.Canceled", err)
	}
	if value.serverOpenState != persistentOpenUnknown || serverOpen {
		t.Fatalf("after canceled CloseServer state=%d serverOpen=%t, want unknown and closed", value.serverOpenState, serverOpen)
	}
	if err := value.CloseServer(context.Background()); err == nil {
		t.Fatal("CloseServer retry succeeded without state reconciliation")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if err := value.Close(); err == nil {
		t.Fatal("Close released a handle with unknown server state")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if err := value.Free(context.Background()); err == nil {
		t.Fatal("Free released a handle with unknown server state")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobOpenStateUnknown)
	}
	if probeCalls != 0 {
		t.Fatalf("Close/Free performed %d hidden state probes, want 0", probeCalls)
	}
	if open, err := value.IsOpen(context.Background()); open || err != nil {
		t.Fatalf("IsOpen reconciliation = (%t, %v), want (false, nil)", open, err)
	}
	if value.serverOpenState != persistentOpenClosed {
		t.Fatalf("state after IsOpen = %d, want closed", value.serverOpenState)
	}
	if probeCalls != 1 {
		t.Fatalf("IsOpen calls = %d, want 1 explicit probe", probeCalls)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("Close after reconciliation returned error: %v", err)
	}
	if err := value.Free(context.Background()); err != nil {
		t.Fatalf("Free after reconciliation returned error: %v", err)
	}
	if openCalls != 1 || closeCalls != 1 {
		t.Fatalf("transition calls = open:%d close:%d, want 1 each", openCalls, closeCalls)
	}
}

// TestLOBHandle_CreateTemporaryValidation verifies temporary creation rejects invalid arguments and unsupported drivers.
func TestLOBHandle_CreateTemporaryValidation(t *testing.T) {
	t.Parallel()

	// A nil connection and an invalid kind must be rejected before any RPC.
	if _, err := createTemporaryLOB(context.Background(), nil, BLOB); err == nil {
		t.Fatal("createTemporary accepted a nil connection")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	created := 0
	conn := newLOBHandleTestConn(t, &lobHandleTestRawConn{
		createFn: func(_ context.Context, kind uint8) ([]byte, error) {
			created++
			if kind != uint8(CLOB) {
				t.Fatalf("LobCreate kind = %d, want %d", kind, CLOB)
			}
			return []byte("temporary-locator"), nil
		},
	})
	if _, err := createTemporaryLOB(context.Background(), conn, Unknown); err == nil {
		t.Fatal("createTemporary accepted an invalid LOB kind")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	value, err := createTemporaryLOB(context.Background(), conn, CLOB)
	if err != nil {
		t.Fatalf("createTemporary returned error: %v", err)
	}
	if value.Kind() != CLOB || !value.IsTemporary() || value.offset != 1 || string(value.locator) != "temporary-locator" || created != 1 {
		t.Fatalf("created lobHandle = kind:%d temporary:%t offset:%d locator:%q calls:%d", value.Kind(), value.IsTemporary(), value.offset, value.locator, created)
	}

	// A raw connection without the private capability contract is unsupported.
	unsupportedConn := newLOBHandleTestConn(t, &unsupportedLOBHandleTestRawConn{})
	if _, err := createTemporaryLOB(context.Background(), unsupportedConn, BLOB); err == nil {
		t.Fatal("createTemporary succeeded with an unsupported raw connection")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.UnsupportedLobOperation)
	}

}

// TestLOBHandle_OperationsAndLifecycle verifies scalar operations, closed-state
// guards, trim/open behavior, temporary and persistent cleanup, and retries.
func TestLOBHandle_OperationsAndLifecycle(t *testing.T) {
	t.Parallel()

	// Successful scalar operations delegate their values and local properties.
	scalar := newActiveLOBHandle(t, &lobHandleTestRawConn{
		lengthFn:    func(context.Context, uint8, []byte) (uint64, error) { return 17, nil },
		chunkSizeFn: func(context.Context, uint8, []byte) (uint64, error) { return 8, nil },
		isOpenFn:    func(context.Context, uint8, []byte) (bool, error) { return true, nil },
	}, NCLOB)
	if size, err := scalar.Size(context.Background()); size != 17 || err != nil {
		t.Fatalf("Size = (%d, %v), want (17, nil)", size, err)
	}
	if size, err := scalar.ChunkSize(context.Background()); size != 8 || err != nil {
		t.Fatalf("ChunkSize = (%d, %v), want (8, nil)", size, err)
	}
	if open, err := scalar.IsOpen(context.Background()); !open || err != nil || scalar.Kind() != NCLOB || scalar.IsTemporary() {
		t.Fatalf("scalar state = open:%t err:%v kind:%d temporary:%t", open, err, scalar.Kind(), scalar.IsTemporary())
	}

	// Scalar RPC errors and an unsigned-to-int64 overflow are returned unchanged.
	lengthErr := errors.New("length failed")
	chunkErr := errors.New("chunk size failed")
	openErr := errors.New("open-state failed")
	errorsValue := newActiveLOBHandle(t, &lobHandleTestRawConn{
		lengthFn:    func(context.Context, uint8, []byte) (uint64, error) { return 0, lengthErr },
		chunkSizeFn: func(context.Context, uint8, []byte) (uint64, error) { return 0, chunkErr },
		isOpenFn:    func(context.Context, uint8, []byte) (bool, error) { return false, openErr },
	}, BLOB)
	if _, err := errorsValue.Size(context.Background()); !errors.Is(err, lengthErr) {
		t.Fatalf("Size error = %v, want %v", err, lengthErr)
	}
	if _, err := errorsValue.ChunkSize(context.Background()); !errors.Is(err, chunkErr) {
		t.Fatalf("ChunkSize error = %v, want %v", err, chunkErr)
	}
	if _, err := errorsValue.IsOpen(context.Background()); !errors.Is(err, openErr) {
		t.Fatalf("IsOpen error = %v, want %v", err, openErr)
	}
	overflow := newActiveLOBHandle(t, &lobHandleTestRawConn{
		lengthFn: func(context.Context, uint8, []byte) (uint64, error) { return ^uint64(0), nil },
	}, BLOB)
	if _, err := overflow.Size(context.Background()); err == nil {
		t.Fatal("Size accepted a value larger than int64")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}

	// Every operation must reject a locally closed handle before an RPC.
	closed := newActiveLOBHandle(t, &lobHandleTestRawConn{}, BLOB)
	if err := closed.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	closedOperations := []struct {
		name string
		call func() error
	}{
		{name: "WriteContext", call: func() error { _, err := closed.WriteContext(context.Background(), []byte("data")); return err }},
		{name: "Trim", call: func() error { return closed.Trim(context.Background(), 0) }},
		{name: "Open", call: func() error { _, err := closed.Open(context.Background(), ReadOnly); return err }},
		{name: "CloseServer", call: func() error { return closed.CloseServer(context.Background()) }},
		{name: "IsOpen", call: func() error { _, err := closed.IsOpen(context.Background()); return err }},
	}
	for _, operation := range closedOperations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); err == nil {
				t.Fatal("operation succeeded after Close")
			} else {
				requireLOBHandleTestErrorCode(t, err, oracleErrors.LobValueClosed)
			}
		})
	}
	// The Raw bridge rejects a driver without lobHandle capabilities.
	unsupported := &lobHandle{conn: newLOBHandleTestConn(t, &unsupportedLOBHandleTestRawConn{}), kind: BLOB, locator: []byte("locator"), offset: 1}
	if _, err := unsupported.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read succeeded with an unsupported raw connection")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.UnsupportedLobOperation)
	}

	// Trim validates negative lengths and invalidates the handle after an RPC error.
	var zero lobHandle
	if err := zero.Trim(context.Background(), -1); err == nil {
		t.Fatal("Trim accepted a negative length")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	trimErr := errors.New("trim failed")
	trimValue := newActiveLOBHandle(t, &lobHandleTestRawConn{
		trimFn: func(context.Context, uint8, []byte, uint64) (uint64, error) { return 0, trimErr },
	}, BLOB)
	trimValue.pending = []byte("pending")
	if err := trimValue.Trim(context.Background(), 3); !errors.Is(err, trimErr) || len(trimValue.pending) != 0 || !trimValue.invalidated {
		t.Fatalf("failed Trim = error:%v pending:%q invalidated:%t, want %v, no pending data, and invalidated handle", err, trimValue.pending, trimValue.invalidated, trimErr)
	}

	// Temporary Open updates the locator without setting persistent open state.
	temporary := newActiveLOBHandle(t, &lobHandleTestRawConn{
		openFn: func(_ context.Context, kind uint8, locator []byte, mode uint8) (bool, []byte, error) {
			if kind != uint8(BLOB) || mode != uint8(ReadWrite) || string(locator) != "locator" {
				t.Fatalf("LobOpen arguments = kind:%d mode:%d locator:%q", kind, mode, locator)
			}
			return true, []byte("opened-locator"), nil
		},
	}, BLOB)
	temporary.temporary = true
	if opened, err := temporary.Open(context.Background(), ReadWrite); !opened || err != nil || temporary.serverOpenState != persistentOpenClosed || string(temporary.locator) != "opened-locator" {
		t.Fatalf("temporary Open = (%t, %v), state serverOpenState:%d locator:%q", opened, err, temporary.serverOpenState, temporary.locator)
	}
	var invalidMode lobHandle
	if _, err := invalidMode.Open(context.Background(), OpenMode(99)); err == nil {
		t.Fatal("Open accepted an invalid mode")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	openErr = errors.New("open failed")
	openFailure := newActiveLOBHandle(t, &lobHandleTestRawConn{
		openFn: func(context.Context, uint8, []byte, uint8) (bool, []byte, error) { return false, nil, openErr },
	}, BLOB)
	if opened, err := openFailure.Open(context.Background(), ReadOnly); opened || !errors.Is(err, openErr) {
		t.Fatalf("Open failure = (%t, %v), want (false, %v)", opened, err, openErr)
	}

	// Free is idempotent, supports persistent handles, and retries temporary RPCs.
	persistent := newActiveLOBHandle(t, &lobHandleTestRawConn{}, BLOB)
	if err := persistent.Free(context.Background()); err != nil {
		t.Fatalf("persistent Free returned error: %v", err)
	}
	if err := persistent.Free(context.Background()); err != nil {
		t.Fatalf("second persistent Free returned error: %v", err)
	}
	if _, err := persistent.Size(context.Background()); err == nil {
		t.Fatal("Size succeeded after persistent Free")
	} else {
		requireLOBHandleTestErrorCode(t, err, oracleErrors.LobValueClosed)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newActiveLOBHandle(t, &lobHandleTestRawConn{}, BLOB).Free(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Free error = %v, want context.Canceled", err)
	}
	freeErr := errors.New("free failed")
	freeCalls := 0
	temporaryFree := newActiveLOBHandle(t, &lobHandleTestRawConn{
		freeFn: func(context.Context, []byte) error {
			freeCalls++
			if freeCalls == 1 {
				return freeErr
			}
			return nil
		},
	}, CLOB)
	temporaryFree.temporary = true
	if err := temporaryFree.Free(context.Background()); !errors.Is(err, freeErr) {
		t.Fatalf("first temporary Free error = %v, want %v", err, freeErr)
	}
	if err := temporaryFree.Free(context.Background()); err != nil {
		t.Fatalf("retry temporary Free returned error: %v", err)
	}
	if err := temporaryFree.Free(context.Background()); err != nil || freeCalls != 2 || !temporaryFree.closed || !temporaryFree.freed {
		t.Fatalf("temporary Free state = err:%v calls:%d closed:%t freed:%t", err, freeCalls, temporaryFree.closed, temporaryFree.freed)
	}
}
