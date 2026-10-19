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

package datatype

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

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

func TestTypedLOB_ImplementsPublicContracts(t *testing.T) {
	var (
		_ sql.Scanner = (*Blob)(nil)
		_ sql.Scanner = (*Clob)(nil)
		_ sql.Scanner = (*NClob)(nil)
		_ LOB         = (*Blob)(nil)
		_ LOB         = (*Clob)(nil)
		_ LOB         = (*NClob)(nil)
	)
}

func TestTypedLOB_ScanValidatesKindAndMaterializes(t *testing.T) {
	t.Parallel()

	blobData := []byte{0, 1, 2, 255}
	_, blobRaw := newTypedLOBTestConn(t, internallob.BLOB, blobData, nil)
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
	_, clobRaw := newTypedLOBTestConn(t, internallob.CLOB, clobData, nil)
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
}

func TestTypedLOB_ScanRejectsWrongKindAndNull(t *testing.T) {
	t.Parallel()

	var blob Blob
	defer blob.Close()
	wrong := &testSource{kind: internallob.CLOB, data: []byte("text")}
	err := blob.Scan(wrong)
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.InvalidLobSource) {
		t.Fatalf("Blob.Scan wrong kind error = %v", err)
	}
	if !wrong.closed {
		t.Fatal("wrong-kind source was not closed")
	}

	if err := blob.Scan(nil); err != nil {
		t.Fatalf("Blob.Scan(nil) returned error: %v", err)
	}
	if blob.Kind() != BLOB || blob.Valid() {
		t.Fatalf("NULL Blob state = kind:%d valid:%t", blob.Kind(), blob.Valid())
	}
	var zeroValue Blob
	if err := zeroValue.Scan(nil); err != nil || zeroValue.Valid() || zeroValue.Kind() != BLOB {
		t.Fatalf("zero-value NULL scan = (%v, valid:%t kind:%d), want nil, false, BLOB", err, zeroValue.Valid(), zeroValue.Kind())
	}
}

func TestTypedLOB_MaterializerPropagatesReadError(t *testing.T) {
	t.Parallel()

	readErr := errors.New("read failed")
	_, raw := newTypedLOBTestConn(t, internallob.BLOB, nil, readErr)
	var blob Blob
	defer blob.Close()
	if err := blob.Scan(newPersistentTestSource(raw, internallob.BLOB, nil)); err != nil {
		t.Fatalf("Blob.Scan returned error: %v", err)
	}
	if _, err := blob.ReadAll(); !errors.Is(err, readErr) {
		t.Fatalf("Blob.ReadAll error = %v, want %v", err, readErr)
	}
}

func TestTypedLOB_TemporaryConstructors(t *testing.T) {
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
			conn, _ := newTypedLOBTestConn(t, test.kind, nil, nil)
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

func TestTypedLOB_ContextMethodsCheckCancellation(t *testing.T) {
	t.Parallel()

	_, raw := newTypedLOBTestConn(t, internallob.BLOB, []byte("data"), nil)
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

func TestTypedLOB_ZeroValueScanAdoptsUnreadSource(t *testing.T) {
	t.Parallel()

	raw := &typedLOBTestRawConn{data: []byte("promoted BLOB value"), kind: uint8(internallob.BLOB)}
	raw.sessionKey = raw
	db := sql.OpenDB(typedLOBTestConnector{raw: raw})
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

// TestTypedLOB_AdoptedPrefixSurvivesRowsClose verifies that scanning transfers
// unread row-prefetched bytes and resumes locator reads after those bytes.
func TestTypedLOB_AdoptedPrefixSurvivesRowsClose(t *testing.T) {
	t.Parallel()

	data := []byte("promoted BLOB value")
	_, raw := newTypedLOBTestConn(t, internallob.BLOB, data, nil)
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

// TestTypedLOB_CompleteAdoptedPrefixNeedsNoLocatorRead verifies complete and
// empty row prefixes reach EOF locally after adoption.
func TestTypedLOB_CompleteAdoptedPrefixNeedsNoLocatorRead(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "small value", data: []byte("small")},
		{name: "empty value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, raw := newTypedLOBTestConn(t, internallob.BLOB, test.data, nil)
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

type persistentTestSource struct {
	*testSource
	sessionKey any
	session    internallob.LOBSession
	locator    []byte
	handoff    internallob.PersistentLOB
	detached   bool
}

func newTypedLOBTestConn(t *testing.T, kind internallob.Kind, data []byte, readErr error) (*sql.Conn, *typedLOBTestRawConn) {
	t.Helper()
	raw := &typedLOBTestRawConn{
		data:    append([]byte(nil), data...),
		readErr: readErr,
		kind:    uint8(kind),
	}
	raw.sessionKey = raw
	db := sql.OpenDB(typedLOBTestConnector{raw: raw})
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db.Conn returned error: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, raw
}

func newPersistentTestSource(raw *typedLOBTestRawConn, kind internallob.Kind, data []byte) *persistentTestSource {
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

type typedLOBTestConnector struct{ raw *typedLOBTestRawConn }

func (connector typedLOBTestConnector) Connect(context.Context) (driver.Conn, error) {
	return connector.raw, nil
}

func (typedLOBTestConnector) Driver() driver.Driver { return typedLOBTestDriver{} }

type typedLOBTestDriver struct{}

func (typedLOBTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("Open should not be called by sql.OpenDB")
}

type typedLOBTestRawConn struct {
	data        []byte
	readErr     error
	kind        uint8
	sessionKey  any
	readOffsets []uint64
}

func (*typedLOBTestRawConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("Prepare is not supported in the LOB scanner test")
}

func (*typedLOBTestRawConn) Close() error { return nil }

func (*typedLOBTestRawConn) Begin() (driver.Tx, error) {
	return nil, errors.New("Begin is not supported in the LOB scanner test")
}

func (connection *typedLOBTestRawConn) LobCreate(context.Context, uint8) ([]byte, error) {
	return []byte("typed-test-locator"), nil
}

func (connection *typedLOBTestRawConn) LobRead(ctx context.Context, kind uint8, locator []byte, offset, amount uint64) ([]byte, uint64, error) {
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

func (*typedLOBTestRawConn) LobWrite(context.Context, uint8, []byte, uint64, []byte) (uint64, error) {
	return 0, nil
}

func (connection *typedLOBTestRawConn) LobLength(ctx context.Context, _ uint8, _ []byte) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return uint64(len(connection.data)), nil
}

func (*typedLOBTestRawConn) LobChunkSize(context.Context, uint8, []byte) (uint64, error) {
	return 16, nil
}

func (*typedLOBTestRawConn) LobTrim(context.Context, uint8, []byte, uint64) (uint64, error) {
	return 0, nil
}

func (*typedLOBTestRawConn) LobOpen(_ context.Context, _ uint8, locator []byte, _ uint8) (bool, []byte, error) {
	return false, append([]byte(nil), locator...), nil
}

func (*typedLOBTestRawConn) LobClose(_ context.Context, _ uint8, locator []byte) ([]byte, error) {
	return append([]byte(nil), locator...), nil
}

func (*typedLOBTestRawConn) LobIsOpen(context.Context, uint8, []byte) (bool, error) {
	return false, nil
}

func (*typedLOBTestRawConn) LobFree(context.Context, []byte) error { return nil }

func (connection *typedLOBTestRawConn) LobSessionKey() any { return connection.sessionKey }

func (*typedLOBTestRawConn) ValidateLOBLease() error { return nil }

func (*typedLOBTestRawConn) AcquireLOBLease() (func(), error) { return func() {}, nil }
