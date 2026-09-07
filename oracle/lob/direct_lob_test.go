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
	"testing"

	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDirectLOB_AcceptedWriteBytes verifies that write acknowledgements map to
// UTF-8 byte counts only at Unicode scalar boundaries.
func TestDirectLOB_AcceptedWriteBytes(t *testing.T) {
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
			gotBytes, err := _acceptedWriteBytes(test.kind, []byte(test.data), test.logical)
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
				t.Fatalf("_acceptedWriteBytes returned error: %v", err)
			}
		})
	}
}

// TestDirectLOB_IsTemporary reports temporary status for temporary and
// persistent direct LOB handles without performing an RPC.
func TestDirectLOB_IsTemporary(t *testing.T) {
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
			value := &DirectLOB{temporary: test.temporary}
			if got := value.IsTemporary(); got != test.want {
				t.Fatalf("IsTemporary() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestDirectLOB_OpenPersistentSerializesWithClose verifies that a source close
// cannot win while persistent-locator promotion is detaching the locator.
func TestDirectLOB_OpenPersistentSerializesWithClose(t *testing.T) {
	t.Parallel()

	source := &directLOBPromotionSource{
		testSource:    testSource{kind: BLOB},
		detachStarted: make(chan struct{}),
		allowDetach:   make(chan struct{}),
	}
	conn := newDirectLOBTestConn(t, &directLOBTestRawConn{sessionKey: "session"})
	var value LOB
	if err := value.Scan(source); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	type promotionResult struct {
		value *DirectLOB
		err   error
	}
	promotionDone := make(chan promotionResult, 1)
	go func() {
		promoted, err := OpenPersistent(context.Background(), conn, &value)
		promotionDone <- promotionResult{value: promoted, err: err}
	}()
	<-source.detachStarted

	closeDone := make(chan error, 1)
	go func() { closeDone <- value.Close() }()
	if value.mu.TryLock() {
		value.mu.Unlock()
		close(source.allowDetach)
		<-promotionDone
		t.Fatal("OpenPersistent did not hold the LOB state lock during detach")
	}

	close(source.allowDetach)
	result := <-promotionDone
	if result.err != nil || result.value == nil {
		t.Fatalf("OpenPersistent returned (%v, %v), want a promoted value", result.value, result.err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("LOB.Close returned error: %v", err)
	}
	if source.closeCalls != 0 {
		t.Fatalf("source Close calls = %d, want 0 after promotion", source.closeCalls)
	}
}

// TestDirectLOB_TrimClearsPendingCLOB verifies that successful trimming drops
// UTF-8 bytes prefetched before the server-side length changed.
func TestDirectLOB_TrimClearsPendingCLOB(t *testing.T) {
	t.Parallel()

	readCalls := 0
	raw := &directLOBTestRawConn{
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
	conn := newDirectLOBTestConn(t, raw)
	value := &DirectLOB{
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

// TestDirectLOB_OpenRequiresCloseServerBeforeClose verifies an explicitly
// opened persistent LOB cannot be locally closed before server close succeeds.
func TestDirectLOB_OpenRequiresCloseServerBeforeClose(t *testing.T) {
	t.Parallel()

	raw := &directLOBTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			return locator, nil
		},
	}
	conn := newDirectLOBTestConn(t, raw)
	value := &DirectLOB{conn: conn, kind: BLOB, locator: []byte("locator"), offset: 1}

	if opened, err := value.Open(context.Background(), ReadWrite); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	requireLOBTestErrorCode(t, value.Close(), oracleErrors.LobOpen)
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

// TestDirectLOB_OpenRequiresCloseServerBeforeFree verifies Free also preserves
// an explicitly opened persistent handle until CloseServer succeeds.
func TestDirectLOB_OpenRequiresCloseServerBeforeFree(t *testing.T) {
	t.Parallel()

	raw := &directLOBTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			return locator, nil
		},
	}
	conn := newDirectLOBTestConn(t, raw)
	value := &DirectLOB{conn: conn, kind: CLOB, locator: []byte("locator"), offset: 1}

	if opened, err := value.Open(context.Background(), ReadOnly); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	requireLOBTestErrorCode(t, value.Free(context.Background()), oracleErrors.LobOpen)
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

// TestDirectLOB_CloseServerFailurePreservesOpenState verifies a failed server
// close is retryable and does not permit local handle release in the interim.
func TestDirectLOB_CloseServerFailurePreservesOpenState(t *testing.T) {
	t.Parallel()

	closeCalls := 0
	closeErr := errors.New("server close failed")
	raw := &directLOBTestRawConn{
		openFn: func(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
			return true, locator, nil
		},
		closeFn: func(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
			closeCalls++
			if closeCalls == 1 {
				return nil, closeErr
			}
			return locator, nil
		},
	}
	conn := newDirectLOBTestConn(t, raw)
	value := &DirectLOB{conn: conn, kind: BLOB, locator: []byte("locator"), offset: 1}

	if opened, err := value.Open(context.Background(), ReadWrite); !opened || err != nil {
		t.Fatalf("Open = (%t, %v), want (true, nil)", opened, err)
	}
	if err := value.CloseServer(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("first CloseServer error = %v, want %v", err, closeErr)
	}
	requireLOBTestErrorCode(t, value.Close(), oracleErrors.LobOpen)
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

// TestDirectLOB_BlobReadCapsRefillAndConsumesPending verifies BLOB refills use
// the bounded TTC chunk and that prefetched bytes are served before another RPC.
func TestDirectLOB_BlobReadCapsRefillAndConsumesPending(t *testing.T) {
	t.Parallel()

	readCalls := 0
	var requested []uint64
	raw := &directLOBTestRawConn{
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
	conn := newDirectLOBTestConn(t, raw)
	value := &DirectLOB{
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

type directLOBPromotionSource struct {
	testSource
	detachStarted chan struct{}
	allowDetach   chan struct{}
	detachErr     error
}

func (source *directLOBPromotionSource) DetachPersistentLocator(any) ([]byte, error) {
	if source.detachStarted != nil {
		close(source.detachStarted)
		<-source.allowDetach
	}
	return []byte("promoted-locator"), source.detachErr
}

type directLOBTestConnector struct {
	raw driver.Conn
}

func (connector directLOBTestConnector) Connect(context.Context) (driver.Conn, error) {
	return connector.raw, nil
}

func (connector directLOBTestConnector) Driver() driver.Driver {
	return directLOBTestDriver{raw: connector.raw}
}

type directLOBTestDriver struct {
	raw driver.Conn
}

func (driver directLOBTestDriver) Open(string) (driver.Conn, error) {
	return driver.raw, nil
}

func newDirectLOBTestConn(t *testing.T, raw driver.Conn) *sql.Conn {
	t.Helper()
	db := sql.OpenDB(directLOBTestConnector{raw: raw})
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("DB.Conn returned error: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// directLOBTestRawConn embeds the private driver contract so each test only
// overrides the raw operation it exercises.
type directLOBTestRawConn struct {
	directLOBDriver
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

func (driver *directLOBTestRawConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unused")
}

func (driver *directLOBTestRawConn) Close() error { return nil }

func (driver *directLOBTestRawConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unused")
}

func (driver *directLOBTestRawConn) LobSessionKey() any {
	return driver.sessionKey
}

func (driver *directLOBTestRawConn) LobCreate(ctx context.Context, kind uint8) ([]byte, error) {
	if driver.createFn == nil {
		return nil, errors.New("LobCreate unused")
	}
	return driver.createFn(ctx, kind)
}

func (driver *directLOBTestRawConn) LobRead(ctx context.Context, kind uint8, locator []byte, offset, amount uint64) ([]byte, uint64, error) {
	if driver.readFn == nil {
		return nil, 0, errors.New("LobRead unused")
	}
	return driver.readFn(ctx, kind, locator, offset, amount)
}

func (driver *directLOBTestRawConn) LobWrite(ctx context.Context, kind uint8, locator []byte, offset uint64, data []byte) (uint64, error) {
	if driver.writeFn == nil {
		return 0, errors.New("LobWrite unused")
	}
	return driver.writeFn(ctx, kind, locator, offset, data)
}

func (driver *directLOBTestRawConn) LobLength(ctx context.Context, kind uint8, locator []byte) (uint64, error) {
	if driver.lengthFn == nil {
		return 0, errors.New("LobLength unused")
	}
	return driver.lengthFn(ctx, kind, locator)
}

func (driver *directLOBTestRawConn) LobChunkSize(ctx context.Context, kind uint8, locator []byte) (uint64, error) {
	if driver.chunkSizeFn == nil {
		return 0, errors.New("LobChunkSize unused")
	}
	return driver.chunkSizeFn(ctx, kind, locator)
}

func (driver *directLOBTestRawConn) LobTrim(ctx context.Context, kind uint8, locator []byte, length uint64) (uint64, error) {
	if driver.trimFn == nil {
		return 0, errors.New("LobTrim unused")
	}
	return driver.trimFn(ctx, kind, locator, length)
}

func (driver *directLOBTestRawConn) LobOpen(ctx context.Context, kind uint8, locator []byte, mode uint8) (bool, []byte, error) {
	if driver.openFn == nil {
		return false, nil, errors.New("LobOpen unused")
	}
	return driver.openFn(ctx, kind, locator, mode)
}

func (driver *directLOBTestRawConn) LobClose(ctx context.Context, kind uint8, locator []byte) ([]byte, error) {
	if driver.closeFn == nil {
		return nil, errors.New("LobClose unused")
	}
	return driver.closeFn(ctx, kind, locator)
}

func (driver *directLOBTestRawConn) LobIsOpen(ctx context.Context, kind uint8, locator []byte) (bool, error) {
	if driver.isOpenFn == nil {
		return false, errors.New("LobIsOpen unused")
	}
	return driver.isOpenFn(ctx, kind, locator)
}

func (driver *directLOBTestRawConn) LobFree(ctx context.Context, locator []byte) error {
	if driver.freeFn == nil {
		return errors.New("LobFree unused")
	}
	return driver.freeFn(ctx, locator)
}

// newActiveDirectLOB creates an active DirectLOB backed by a fake raw
// connection so API operations can be tested without an Oracle database.
func newActiveDirectLOB(t *testing.T, raw *directLOBTestRawConn, kind Kind) *DirectLOB {
	t.Helper()
	return &DirectLOB{conn: newDirectLOBTestConn(t, raw), kind: kind, locator: []byte("locator"), offset: 1}
}

// TestDirectLOB_CreateAndPromoteValidation verifies temporary creation and
// persistent promotion reject invalid arguments, state, and driver responses.
func TestDirectLOB_CreateAndPromoteValidation(t *testing.T) {
	t.Parallel()

	// A nil connection and an invalid kind must be rejected before any RPC.
	if _, err := CreateTemporary(context.Background(), nil, BLOB); err == nil {
		t.Fatal("CreateTemporary accepted a nil connection")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	created := 0
	conn := newDirectLOBTestConn(t, &directLOBTestRawConn{
		createFn: func(_ context.Context, kind uint8) ([]byte, error) {
			created++
			if kind != uint8(CLOB) {
				t.Fatalf("LobCreate kind = %d, want %d", kind, CLOB)
			}
			return []byte("temporary-locator"), nil
		},
	})
	if _, err := CreateTemporary(context.Background(), conn, Unknown); err == nil {
		t.Fatal("CreateTemporary accepted an invalid LOB kind")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	value, err := CreateTemporary(context.Background(), conn, CLOB)
	if err != nil {
		t.Fatalf("CreateTemporary returned error: %v", err)
	}
	if value.Kind() != CLOB || !value.IsTemporary() || value.offset != 1 || string(value.locator) != "temporary-locator" || created != 1 {
		t.Fatalf("created DirectLOB = kind:%d temporary:%t offset:%d locator:%q calls:%d", value.Kind(), value.IsTemporary(), value.offset, value.locator, created)
	}

	// A raw connection without the private capability contract is unsupported.
	unsupportedConn := newDirectLOBTestConn(t, &unsupportedDirectLOBTestRawConn{})
	if _, err := CreateTemporary(context.Background(), unsupportedConn, BLOB); err == nil {
		t.Fatal("CreateTemporary succeeded with an unsupported raw connection")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.UnsupportedLobOperation)
	}

	if _, err := OpenPersistent(context.Background(), nil, nil); err == nil {
		t.Fatal("OpenPersistent accepted nil arguments")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenPersistent(canceled, conn, &LOB{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled OpenPersistent error = %v, want context.Canceled", err)
	}

	// An unscanned LOB and a materialized source cannot be promoted.
	var nullValue LOB
	if _, err := OpenPersistent(context.Background(), conn, &nullValue); err == nil {
		t.Fatal("OpenPersistent accepted an unscanned LOB")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	var materializedValue LOB
	if err := materializedValue.Scan(&testSource{kind: BLOB}); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if _, err := OpenPersistent(context.Background(), conn, &materializedValue); err == nil {
		t.Fatal("OpenPersistent accepted a non-persistent source")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}

	// Promotion also reports unsupported raw drivers and detach failures.
	unsupportedSource := &directLOBPromotionSource{testSource: testSource{kind: BLOB}}
	var unsupportedValue LOB
	if err := unsupportedValue.Scan(unsupportedSource); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if _, err := OpenPersistent(context.Background(), unsupportedConn, &unsupportedValue); err == nil {
		t.Fatal("OpenPersistent succeeded with an unsupported raw connection")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.UnsupportedLobOperation)
	}
	detachErr := errors.New("detach failed")
	detachingSource := &directLOBPromotionSource{testSource: testSource{kind: CLOB}, detachErr: detachErr}
	var detachingValue LOB
	if err := detachingValue.Scan(detachingSource); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if _, err := OpenPersistent(context.Background(), conn, &detachingValue); !errors.Is(err, detachErr) {
		t.Fatalf("OpenPersistent detach error = %v, want %v", err, detachErr)
	}
}

// TestDirectLOB_ReadWriteAndWriteTo verifies read/write wrappers, validation,
// cursor invalidation, and the success and error paths of WriteTo.
func TestDirectLOB_ReadWriteAndWriteTo(t *testing.T) {
	t.Parallel()

	// Read and Write use context.Background and advance the shared cursor.
	var writeOffset uint64
	value := newActiveDirectLOB(t, &directLOBTestRawConn{
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
	empty := newActiveDirectLOB(t, &directLOBTestRawConn{
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
	failedRead := newActiveDirectLOB(t, &directLOBTestRawConn{readFn: readOnce(nil, 0, readErr)}, CLOB)
	if n, err := failedRead.ReadContext(context.Background(), make([]byte, 1)); n != 0 || !errors.Is(err, readErr) {
		t.Fatalf("read-error ReadContext = (%d, %v), want (0, %v)", n, err, readErr)
	}

	// Character LOBs reject malformed UTF-8 before entering the raw driver.
	invalidUTF8 := newActiveDirectLOB(t, &directLOBTestRawConn{}, CLOB)
	if _, err := invalidUTF8.Write([]byte{0xff}); err == nil {
		t.Fatal("Write accepted invalid CLOB UTF-8")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	// An impossible acknowledgement invalidates the cursor after the response.
	invalidAck := newActiveDirectLOB(t, &directLOBTestRawConn{
		writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) { return 3, nil },
	}, BLOB)
	if _, err := invalidAck.Write([]byte("ab")); err == nil {
		t.Fatal("Write accepted an acknowledgement larger than the payload")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if _, err := invalidAck.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read succeeded after an invalid write acknowledgement")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	writeErr := errors.New("write failed")
	failedWrite := newActiveDirectLOB(t, &directLOBTestRawConn{
		writeFn: func(context.Context, uint8, []byte, uint64, []byte) (uint64, error) { return 0, writeErr },
	}, BLOB)
	if _, err := failedWrite.Write([]byte("data")); !errors.Is(err, writeErr) {
		t.Fatalf("Write error = %v, want %v", err, writeErr)
	}

	// Table-driven cases cover all WriteTo result classifications.
	writerErr := errors.New("writer failed")
	cases := []struct {
		name       string
		raw        *directLOBTestRawConn
		writer     io.Writer
		wantBytes  int64
		wantErr    error
		wantCode   oracleErrors.ErrorCode
		wantOutput string
	}{
		{name: "nil writer", wantCode: oracleErrors.InvalidLOBBuffer},
		{name: "complete", raw: &directLOBTestRawConn{readFn: readOnce([]byte("data"), 4, nil)}, writer: &bytes.Buffer{}, wantBytes: 4, wantOutput: "data"},
		{name: "writer error", raw: &directLOBTestRawConn{readFn: readOnce([]byte("data"), 4, nil)}, writer: &directLOBTestWriter{err: writerErr, max: -1}, wantBytes: 4, wantErr: writerErr},
		{name: "short writer", raw: &directLOBTestRawConn{readFn: readOnce([]byte("data"), 4, nil)}, writer: &directLOBTestWriter{max: 1}, wantBytes: 1, wantErr: io.ErrShortWrite},
		{name: "read error", raw: &directLOBTestRawConn{readFn: readOnce(nil, 0, readErr)}, writer: io.Discard, wantErr: readErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var value *DirectLOB
			if tc.raw != nil {
				value = newActiveDirectLOB(t, tc.raw, BLOB)
			}
			var got int64
			var err error
			if value == nil {
				var zero DirectLOB
				got, err = zero.WriteTo(tc.writer)
			} else {
				got, err = value.WriteTo(tc.writer)
			}
			if got != tc.wantBytes {
				t.Errorf("WriteTo bytes = %d, want %d", got, tc.wantBytes)
			}
			if tc.wantCode != "" {
				requireLOBTestErrorCode(t, err, tc.wantCode)
			} else if !errors.Is(err, tc.wantErr) {
				t.Errorf("WriteTo error = %v, want %v", err, tc.wantErr)
			}
			if output, ok := tc.writer.(*bytes.Buffer); ok && output.String() != tc.wantOutput {
				t.Errorf("WriteTo output = %q, want %q", output.String(), tc.wantOutput)
			}
		})
	}
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

// directLOBTestWriter provides controllable writer errors and short writes for
// testing DirectLOB.WriteTo result handling.
type directLOBTestWriter struct {
	err error
	max int
}

func (writer *directLOBTestWriter) Write(data []byte) (int, error) {
	written := len(data)
	if writer.max >= 0 && written > writer.max {
		written = writer.max
	}
	return written, writer.err
}

// TestDirectLOB_OperationsAndLifecycle verifies scalar operations, closed-state
// guards, trim/open behavior, temporary and persistent cleanup, and retries.
func TestDirectLOB_OperationsAndLifecycle(t *testing.T) {
	t.Parallel()

	// Successful scalar operations delegate their values and local properties.
	scalar := newActiveDirectLOB(t, &directLOBTestRawConn{
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
	errorsValue := newActiveDirectLOB(t, &directLOBTestRawConn{
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
	overflow := newActiveDirectLOB(t, &directLOBTestRawConn{
		lengthFn: func(context.Context, uint8, []byte) (uint64, error) { return ^uint64(0), nil },
	}, BLOB)
	if _, err := overflow.Size(context.Background()); err == nil {
		t.Fatal("Size accepted a value larger than int64")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}

	// Every operation must reject a locally closed handle before an RPC.
	closed := newActiveDirectLOB(t, &directLOBTestRawConn{}, BLOB)
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
				requireLOBTestErrorCode(t, err, oracleErrors.LobValueClosed)
			}
		})
	}
	// The Raw bridge rejects a driver without DirectLOB capabilities.
	unsupported := &DirectLOB{conn: newDirectLOBTestConn(t, &unsupportedDirectLOBTestRawConn{}), kind: BLOB, locator: []byte("locator"), offset: 1}
	if _, err := unsupported.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read succeeded with an unsupported raw connection")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.UnsupportedLobOperation)
	}

	// Trim validates negative lengths and preserves pending data on RPC failure.
	var zero DirectLOB
	if err := zero.Trim(context.Background(), -1); err == nil {
		t.Fatal("Trim accepted a negative length")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	trimErr := errors.New("trim failed")
	trimValue := newActiveDirectLOB(t, &directLOBTestRawConn{
		trimFn: func(context.Context, uint8, []byte, uint64) (uint64, error) { return 0, trimErr },
	}, BLOB)
	trimValue.pending = []byte("pending")
	if err := trimValue.Trim(context.Background(), 3); !errors.Is(err, trimErr) || string(trimValue.pending) != "pending" {
		t.Fatalf("failed Trim = error:%v pending:%q, want %v and pending", err, trimValue.pending, trimErr)
	}

	// Temporary Open updates the locator without setting persistent serverOpen.
	temporary := newActiveDirectLOB(t, &directLOBTestRawConn{
		openFn: func(_ context.Context, kind uint8, locator []byte, mode uint8) (bool, []byte, error) {
			if kind != uint8(BLOB) || mode != uint8(ReadWrite) || string(locator) != "locator" {
				t.Fatalf("LobOpen arguments = kind:%d mode:%d locator:%q", kind, mode, locator)
			}
			return true, []byte("opened-locator"), nil
		},
	}, BLOB)
	temporary.temporary = true
	if opened, err := temporary.Open(context.Background(), ReadWrite); !opened || err != nil || temporary.serverOpen || string(temporary.locator) != "opened-locator" {
		t.Fatalf("temporary Open = (%t, %v), state serverOpen:%t locator:%q", opened, err, temporary.serverOpen, temporary.locator)
	}
	var invalidMode DirectLOB
	if _, err := invalidMode.Open(context.Background(), OpenMode(99)); err == nil {
		t.Fatal("Open accepted an invalid mode")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	openErr = errors.New("open failed")
	openFailure := newActiveDirectLOB(t, &directLOBTestRawConn{
		openFn: func(context.Context, uint8, []byte, uint8) (bool, []byte, error) { return false, nil, openErr },
	}, BLOB)
	if opened, err := openFailure.Open(context.Background(), ReadOnly); opened || !errors.Is(err, openErr) {
		t.Fatalf("Open failure = (%t, %v), want (false, %v)", opened, err, openErr)
	}

	// Free is idempotent, supports persistent handles, and retries temporary RPCs.
	persistent := newActiveDirectLOB(t, &directLOBTestRawConn{}, BLOB)
	if err := persistent.Free(context.Background()); err != nil {
		t.Fatalf("persistent Free returned error: %v", err)
	}
	if err := persistent.Free(context.Background()); err != nil {
		t.Fatalf("second persistent Free returned error: %v", err)
	}
	if _, err := persistent.Size(context.Background()); err == nil {
		t.Fatal("Size succeeded after persistent Free")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.LobValueClosed)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newActiveDirectLOB(t, &directLOBTestRawConn{}, BLOB).Free(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Free error = %v, want context.Canceled", err)
	}
	freeErr := errors.New("free failed")
	freeCalls := 0
	temporaryFree := newActiveDirectLOB(t, &directLOBTestRawConn{
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

// unsupportedDirectLOBTestRawConn is a database/sql connection that does not
// implement the private DirectLOB driver capability contract.
type unsupportedDirectLOBTestRawConn struct{}

func (*unsupportedDirectLOBTestRawConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unused")
}

func (*unsupportedDirectLOBTestRawConn) Close() error { return nil }

func (*unsupportedDirectLOBTestRawConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unused")
}
