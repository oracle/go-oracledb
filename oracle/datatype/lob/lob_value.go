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

// Package lob provides connection-scoped Oracle LOB values and operations.
//
// Blob, Clob, and NClob implement sql.Scanner for locator-backed query values
// supplied by go-oracledb. This is a go-oracledb-specific database/sql scanning
// extension: the locator source is neither a standard database/sql/driver.Value
// result nor a documented Scanner source type. These scanner implementations
// are therefore not portable to arbitrary database/sql drivers. Scanning
// directly with Rows.Scan preserves the live LOB locator and enables streaming
// operations.
package lob

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"math"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Kind identifies the Oracle LOB family represented by a typed datatype.
type Kind = internallob.Kind

const (
	// Unknown represents an absent or unrecognized LOB family.
	Unknown = internallob.Unknown
	// BLOB represents binary LOB data.
	BLOB = internallob.BLOB
	// CLOB represents database-character-set LOB data.
	CLOB = internallob.CLOB
	// NCLOB represents national-character-set LOB data.
	NCLOB = internallob.NCLOB
)

// LOB is the common connection-scoped query, mutation, and temporary-lifecycle
// contract for typed BLOB, CLOB, and NCLOB values. A zero-value typed LOB can be
// used as a Scan target when the query runs through a dedicated *sql.Conn. Keep
// that connection open while using the scanned value. If the query uses *sql.DB,
// Rows.Close may return its physical connection to the pool and expire the
// scanned value's session capability; this prevents use against a reused
// session and does not itself destroy the Oracle locator or delete persistent
// LOB data. If only the data is needed with *sql.DB, materialize it after Scan
// and before consuming the LOB, while the current row still holds the
// connection: use ReadAll or ReadAllContext on the typed value before calling
// Next again or closing Rows.
// A materialized Go value returned by ReadAll or ReadAllContext remains usable
// after Rows.Close. A live locator-backed value requires its dedicated
// *sql.Conn to remain open. Query scanners accept persistent table LOB
// locators only. A query returning a locator marked temporary or abstract—for
// example, some SQL expression or function results—is rejected with
// InvalidLobSource. Use a typed temporary-LOB constructor for a temporary LOB.
// Close releases only local Go state; it does not release temporary Oracle
// storage. Call Free or FreeContext before closing the owning connection when a
// LOB is temporary.
// A LOB value is not safe for concurrent use. Callers must serialize all
// operations on a value, including Scan, reads, writes, Close, and Free. Scan
// may replace the current locator; Close and Free may invalidate it.
type LOB interface {
	sql.Scanner
	io.Reader
	io.WriterTo
	// Close releases only local Go state; use Free for temporary Oracle storage.
	Close() error
	// Kind reports the declared Oracle LOB family, including for SQL NULL.
	Kind() Kind
	// Valid reports whether the value represents a non-NULL LOB that has not been
	// successfully freed. A zero-length LOB is valid. A closed or invalidated
	// handle may still report true, so this is not a usability probe.
	Valid() bool
	// IsTemporary reports whether the value owns Oracle temporary storage.
	IsTemporary() bool
	// ReadContext reads BLOB bytes or UTF-8 text from the sequential cursor using ctx.
	ReadContext(context.Context, []byte) (int, error)
	// Write writes at the sequential cursor using context.Background. It returns
	// the input bytes acknowledged by Oracle; a large write may be partially
	// applied before an error. For CLOB/NCLOB, each call must contain complete,
	// valid UTF-8 characters. When writing from a reader, provide chunks that end
	// at UTF-8 character boundaries; a large call is split internally at those
	// boundaries.
	Write([]byte) (int, error)
	// WriteContext writes BLOB bytes or UTF-8 text at the sequential cursor
	// using ctx. It returns the input bytes acknowledged by Oracle; a large
	// write may be partially applied before an error. For CLOB/NCLOB, each call
	// must contain complete, valid UTF-8 characters. When writing from a reader,
	// provide chunks that end at UTF-8 character boundaries; a large call is
	// split internally at those boundaries.
	WriteContext(context.Context, []byte) (int, error)
	// WriteToContext copies the remaining value to writer using ctx.
	WriteToContext(context.Context, io.Writer) (int64, error)
	// Size returns bytes for BLOBs or UTF-16 code units for CLOBs and NCLOBs.
	Size() (int64, error)
	// SizeContext returns bytes for BLOBs or UTF-16 code units for text LOBs using ctx.
	SizeContext(context.Context) (int64, error)
	// ChunkSize returns Oracle's storage chunk size in bytes.
	ChunkSize() (int64, error)
	// ChunkSizeContext returns Oracle's storage chunk size in bytes using ctx.
	ChunkSizeContext(context.Context) (int64, error)
	// Trim changes the logical length using context.Background; units match Size.
	// For CLOB/NCLOB, nonzero trims require the sequential read cursor to be at
	// a UTF-8 character boundary.
	Trim(int64) error
	// TrimContext changes the logical length using ctx; units match Size. A
	// nonzero CLOB/NCLOB trim requires the sequential cursor to be at a UTF-8
	// character boundary.
	TrimContext(context.Context, int64) error
	// Open opens the LOB using context.Background.
	Open(OpenMode) (bool, error)
	// OpenContext opens the LOB using ctx. After an attempted operation returns
	// an error, use IsOpenContext to reconcile the server state before retrying.
	OpenContext(context.Context, OpenMode) (bool, error)
	// CloseServer closes an explicit server-side open using context.Background.
	CloseServer() error
	// CloseServerContext closes an explicit server-side open using ctx. After an
	// attempted operation returns an error, use IsOpenContext to reconcile the
	// server state before retrying.
	CloseServerContext(context.Context) error
	// IsOpen reports server-side open state using context.Background.
	IsOpen() (bool, error)
	// IsOpenContext reports server-side open state using ctx and reconciles the
	// cached state after a successful query.
	IsOpenContext(context.Context) (bool, error)
	// Free releases the local handle and temporary storage using context.Background.
	Free() error
	// FreeContext releases the local handle and temporary storage using ctx.
	FreeContext(context.Context) error
}

// Blob is a type-safe, connection-scoped BLOB value. Scan adopts a persistent
// query locator from the row source into its owning physical connection lease.
// Size and Trim use bytes. To use Blob after Rows.Close, run the query through a
// dedicated *sql.Conn and keep it open through the final LOB operation. If the
// query uses *sql.DB, Rows.Close may return the physical connection to the pool
// and expire Blob's session capability, so its locator-backed operations are
// rejected. This protects against use on a reused session; it does not delete
// the persistent LOB in Oracle. If only the bytes are needed, call ReadAll or
// ReadAllContext after Scan and before consuming the LOB or advancing/closing
// Rows; keep the returned []byte after closing Rows. While its connection stays
// open, Blob supports direct operations such as Write, Trim, Open, and Free.
// Blob is not safe for concurrent use; callers must serialize all operations
// on it, including Scan, reads, writes, Close, and Free. Scan may replace the
// current locator; Close and Free may invalidate it. Blob contains
// synchronization state and must not be copied after its first use.
type Blob struct{ lobValue }

// CreateTemporaryBlob creates a connection-scoped temporary BLOB. Call Free
// before closing conn to release Oracle temporary storage.
func CreateTemporaryBlob(ctx context.Context, conn *sql.Conn) (*Blob, error) {
	handle, err := newTemporaryTypedLOB(ctx, conn, internallob.BLOB)
	if err != nil {
		return nil, err
	}
	return &Blob{lobValue: lobValue{kind: internallob.BLOB, handle: handle}}, nil
}

// Scan implements sql.Scanner for BLOB values returned by go-oracledb and
// accepts only its locator-backed source or SQL NULL. This direct locator scan
// is a go-oracledb-specific database/sql extension, not a standard driver.Value
// result or portable Scanner source type. Use Rows from a dedicated *sql.Conn
// if the scanned value must remain usable after Rows.Close; Rows from *sql.DB
// may expire the handle's session capability on close. Materialize with
// ReadAll after Scan and before consuming the LOB or advancing/closing Rows if
// only the data is needed.
func (blob *Blob) Scan(input any) error { return blob.lobValue.scan(input, internallob.BLOB) }

// Kind identifies this value as a BLOB, including before a scan and after a
// NULL scan.
func (*Blob) Kind() Kind { return internallob.BLOB }

// ReadAt reads BLOB bytes starting at the zero-based byte offset. It follows
// io.ReaderAt semantics: if fewer than len(p) bytes are available, it returns
// the bytes read and io.EOF. It does not change the sequential read cursor.
func (blob *Blob) ReadAt(p []byte, offset int64) (int, error) {
	return blob.ReadAtContext(context.Background(), p, offset)
}

// ReadAtContext reads BLOB bytes starting at the zero-based byte offset using
// ctx. It follows io.ReaderAt short-read behavior and does not change the
// sequential read cursor or its buffered data.
func (blob *Blob) ReadAtContext(ctx context.Context, p []byte, offset int64) (int, error) {
	handle, err := blob.lobValue.activeHandle()
	if err != nil {
		return 0, err
	}
	return handle.ReadAtContext(ctx, p, offset)
}

// ReadAll reads and returns the remaining BLOB bytes using
// context.Background. It does not close the value. On a read error, it returns
// the successfully read prefix with the error; use ReadAllContext to provide a
// deadline or cancellation.
func (blob *Blob) ReadAll() ([]byte, error) {
	return blob.ReadAllContext(context.Background())
}

// ReadAllContext materializes the remaining BLOB bytes using ctx. If a read
// fails after data has been read, it returns the bytes read so far with the
// error. If more than MaxInt32 bytes remain, it returns the first MaxInt32
// bytes and an error; the next byte remains available to a later Read.
func (blob *Blob) ReadAllContext(ctx context.Context) ([]byte, error) {
	return readRemainingContext(ctx, blob, "BLOB")
}

// Clob is a type-safe, connection-scoped CLOB value. Its Read method returns
// UTF-8 bytes; Size and Trim use UTF-16 code units. The validated character-set
// profile uses database character set AL32UTF8; other database character sets
// are not yet validated. To use Clob after Rows.Close, run the query through a
// dedicated *sql.Conn and keep it open through the final LOB operation. If the
// query uses *sql.DB, Rows.Close may return the physical connection to the pool
// and expire Clob's session capability, so its locator-backed operations are
// rejected. This protects against use on a reused session; it does not delete
// the persistent LOB in Oracle. If only the text is needed, call ReadAll or
// ReadAllContext after Scan and before consuming the LOB or advancing/closing
// Rows; keep the returned string after closing Rows. Clob is not safe for
// concurrent use; callers must serialize all operations on it, including
// Scan, reads, writes, Close, and Free. Scan may replace the current locator;
// Close and Free may invalidate it. Clob contains synchronization state and
// must not be copied after first use.
type Clob struct{ lobValue }

// CreateTemporaryClob creates a connection-scoped temporary CLOB. Call Free
// before closing conn to release Oracle temporary storage.
func CreateTemporaryClob(ctx context.Context, conn *sql.Conn) (*Clob, error) {
	handle, err := newTemporaryTypedLOB(ctx, conn, internallob.CLOB)
	if err != nil {
		return nil, err
	}
	return &Clob{lobValue: lobValue{kind: internallob.CLOB, handle: handle}}, nil
}

// Scan implements sql.Scanner for CLOB values returned by go-oracledb and
// accepts only its locator-backed source or SQL NULL. This direct locator scan
// is a go-oracledb-specific database/sql extension, not a standard driver.Value
// result or portable Scanner source type. Use Rows from a dedicated *sql.Conn
// if the scanned value must remain usable after Rows.Close; Rows from *sql.DB
// may expire the handle's session capability on close. Materialize with
// ReadAll after Scan and before consuming the LOB or advancing/closing Rows if
// only the data is needed.
func (clob *Clob) Scan(input any) error { return clob.lobValue.scan(input, internallob.CLOB) }

// Kind identifies this value as a CLOB, including before a scan and after a
// NULL scan.
func (*Clob) Kind() Kind { return internallob.CLOB }

// ReadTextAt reads complete UTF-8 characters from the zero-based CLOB logical
// unit offset. The offset and returned nUnits use Oracle UTF-16 code units;
// a supplementary character uses two units. It never implements io.ReaderAt.
// If a character does not fit while dst still has space, it returns
// io.ErrShortBuffer with the complete-character prefix. If dst is exactly
// filled, it returns that prefix with nil; advance by nUnits to continue.
func (clob *Clob) ReadTextAt(dst []byte, offset int64) (nBytes int, nUnits int64, err error) {
	return clob.ReadTextAtContext(context.Background(), dst, offset)
}

// ReadTextAtContext reads complete UTF-8 characters from the zero-based CLOB
// logical unit offset using ctx. Oracle locator positions are one-based, so
// the offset is translated internally. An offset inside a surrogate pair
// returns an error. If the next complete character cannot fit while dst still
// has space, it returns io.ErrShortBuffer without copying part of that
// character. If dst is exactly filled, it returns nil. The byte and unit
// counts describe the returned complete-character prefix.
func (clob *Clob) ReadTextAtContext(ctx context.Context, dst []byte, offset int64) (nBytes int, nUnits int64, err error) {
	handle, err := clob.lobValue.activeHandle()
	if err != nil {
		return 0, 0, err
	}
	return handle.ReadTextAtContext(ctx, dst, offset)
}

// WriteString writes complete, valid UTF-8 text at the current CLOB cursor. It
// implements io.StringWriter. Repeated calls must each contain complete
// characters. Large strings are sent in bounded Oracle requests at character
// boundaries; the method preserves any existing suffix after the written text.
// The returned count is the number of input bytes Oracle acknowledged,
// including when a later request returns an error. Use WriteStringContext to
// set a deadline or cancel a multi-request write.
func (clob *Clob) WriteString(text string) (int, error) {
	return clob.WriteStringContext(context.Background(), text)
}

// WriteStringContext writes complete, valid UTF-8 text at the current CLOB
// cursor using ctx. Each call must contain complete characters; repeated calls
// can be used for text chunks that end at character boundaries. Large writes
// are split internally at character boundaries. It preserves any existing
// suffix after the written text. A large write may be split into multiple
// Oracle requests, so an error can follow partial modification; the returned
// count reports bytes acknowledged before the error. If a request errors,
// Oracle may have applied that request; its bytes are not counted and the LOB
// handle is invalidated. Text is validated before the first request is sent.
func (clob *Clob) WriteStringContext(ctx context.Context, text string) (int, error) {
	return clob.lobValue.writeStringContext(ctx, text)
}

// ReadAll reads and returns the remaining CLOB text as a UTF-8 string using
// context.Background. It does not close the value. On a read error, it returns
// the successfully read prefix with the error; that partial text may end
// inside a UTF-8 character. Use ReadAllContext to provide a deadline or
// cancellation.
func (clob *Clob) ReadAll() (string, error) {
	return clob.ReadAllContext(context.Background())
}

// ReadAllContext materializes the remaining CLOB text as a UTF-8 string using
// ctx. If a read fails after data has been read, it returns the text read so
// far with the error. The partial string may end inside a UTF-8 character. If
// the value exceeds the materialization limit, the first byte beyond the
// returned prefix remains available to a later Read.
func (clob *Clob) ReadAllContext(ctx context.Context) (string, error) {
	data, err := readRemainingContext(ctx, clob, "CLOB")
	return string(data), err
}

// NClob is a type-safe, connection-scoped NCLOB value. Its Read method returns
// UTF-8 bytes; Size and Trim use UTF-16 code units. The validated national
// character-set profile is AL16UTF16; other national character sets are not
// yet validated. To use NClob after Rows.Close, run the query through a
// dedicated *sql.Conn and keep it open through the final LOB operation. If the
// query uses *sql.DB, Rows.Close may return the physical connection to the pool
// and expire NClob's session capability, so its locator-backed operations are
// rejected. This protects against use on a reused session; it does not delete
// the persistent LOB in Oracle. If only the text is needed, call ReadAll or
// ReadAllContext after Scan and before consuming the LOB or advancing/closing
// Rows; keep the returned string after closing Rows. NClob is not safe for
// concurrent use; callers must serialize all operations on it, including
// Scan, reads, writes, Close, and Free. Scan may replace the current locator;
// Close and Free may invalidate it. NClob contains synchronization state and
// must not be copied after first use.
type NClob struct{ lobValue }

var (
	_ io.ReaderAt     = (*Blob)(nil)
	_ io.StringWriter = (*Clob)(nil)
	_ io.StringWriter = (*NClob)(nil)
)

// CreateTemporaryNClob creates a connection-scoped temporary NCLOB. Call Free
// before closing conn to release Oracle temporary storage.
func CreateTemporaryNClob(ctx context.Context, conn *sql.Conn) (*NClob, error) {
	handle, err := newTemporaryTypedLOB(ctx, conn, internallob.NCLOB)
	if err != nil {
		return nil, err
	}
	return &NClob{lobValue: lobValue{kind: internallob.NCLOB, handle: handle}}, nil
}

// Scan implements sql.Scanner for NCLOB values returned by go-oracledb and
// accepts only its locator-backed source or SQL NULL. This direct locator scan
// is a go-oracledb-specific database/sql extension, not a standard driver.Value
// result or portable Scanner source type. Use Rows from a dedicated *sql.Conn
// if the scanned value must remain usable after Rows.Close; Rows from *sql.DB
// may expire the handle's session capability on close. Materialize with
// ReadAll after Scan and before consuming the LOB or advancing/closing Rows if
// only the data is needed.
func (nclob *NClob) Scan(input any) error { return nclob.lobValue.scan(input, internallob.NCLOB) }

// Kind identifies this value as an NCLOB, including before a scan and after a
// NULL scan.
func (*NClob) Kind() Kind { return internallob.NCLOB }

// ReadTextAt reads complete UTF-8 characters from the zero-based NCLOB logical
// unit offset. The offset and returned nUnits use Oracle UTF-16 code units;
// a supplementary character uses two units. It never implements io.ReaderAt.
// If a character does not fit while dst still has space, it returns
// io.ErrShortBuffer with the complete-character prefix. If dst is exactly
// filled, it returns that prefix with nil; advance by nUnits to continue.
func (nclob *NClob) ReadTextAt(dst []byte, offset int64) (nBytes int, nUnits int64, err error) {
	return nclob.ReadTextAtContext(context.Background(), dst, offset)
}

// ReadTextAtContext reads complete UTF-8 characters from the zero-based NCLOB
// logical unit offset using ctx. Oracle locator positions are one-based, so
// the offset is translated internally. An offset inside a surrogate pair
// returns an error. If the next complete character cannot fit while dst still
// has space, it returns io.ErrShortBuffer without copying part of that
// character. If dst is exactly filled, it returns nil. The byte and unit
// counts describe the returned complete-character prefix.
func (nclob *NClob) ReadTextAtContext(ctx context.Context, dst []byte, offset int64) (nBytes int, nUnits int64, err error) {
	handle, err := nclob.lobValue.activeHandle()
	if err != nil {
		return 0, 0, err
	}
	return handle.ReadTextAtContext(ctx, dst, offset)
}

// WriteString writes complete, valid UTF-8 text at the current NCLOB cursor.
// It implements io.StringWriter. Repeated calls must each contain complete
// characters. Large strings are sent in bounded Oracle requests at character
// boundaries; the method preserves any existing suffix after the written text.
// The returned count is the number of input bytes Oracle acknowledged,
// including when a later request returns an error. Use WriteStringContext to
// set a deadline or cancel a multi-request write.
func (nclob *NClob) WriteString(text string) (int, error) {
	return nclob.WriteStringContext(context.Background(), text)
}

// WriteStringContext writes complete, valid UTF-8 text at the current NCLOB
// cursor using ctx. Each call must contain complete characters; repeated calls
// can be used for text chunks that end at character boundaries. Large writes
// are split internally at character boundaries. It preserves any existing
// suffix after the written text. A large write may be split into multiple
// Oracle requests, so an error can follow partial modification; the returned
// count reports bytes acknowledged before the error. If a request errors,
// Oracle may have applied that request; its bytes are not counted and the LOB
// handle is invalidated. Text is validated before the first request is sent.
func (nclob *NClob) WriteStringContext(ctx context.Context, text string) (int, error) {
	return nclob.lobValue.writeStringContext(ctx, text)
}

// ReadAll reads and returns the remaining NCLOB text as a UTF-8 string using
// context.Background. It does not close the value. On a read error, it returns
// the successfully read prefix with the error; that partial text may end
// inside a UTF-8 character. Use ReadAllContext to provide a deadline or
// cancellation.
func (nclob *NClob) ReadAll() (string, error) {
	return nclob.ReadAllContext(context.Background())
}

// ReadAllContext materializes the remaining NCLOB text as a UTF-8 string using
// ctx. If a read fails after data has been read, it returns the text read so
// far with the error. The partial string may end inside a UTF-8 character. If
// the value exceeds the materialization limit, the first byte beyond the
// returned prefix remains available to a later Read.
func (nclob *NClob) ReadAllContext(ctx context.Context) (string, error) {
	data, err := readRemainingContext(ctx, nclob, "NCLOB")
	return string(data), err
}

// lobValue owns a connection-scoped locator. Query locators are adopted from
// their row source during Scan; temporary locators are created directly on the
// supplied dedicated connection.
type lobValue struct {
	// mu protects the current handle and declared LOB kind.
	mu sync.Mutex
	// kind records the scanner's fixed LOB family, including after SQL NULL.
	kind Kind
	// handle is nil for a SQL NULL or not-yet-scanned value.
	handle *lobHandle
}

func invalidLOBConnection(operation, detail string) error {
	return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, operation, "typed LOB", detail)
}

func (value *lobValue) scan(input any, expectedKind Kind) error {
	if !internallob.ValidKind(expectedKind) {
		return common.NewOracleError(oracleErrors.InvalidLobSource, nil, "LOB kind")
	}

	var source internallob.LOBSource
	if input != nil {
		var ok bool
		source, ok = input.(internallob.LOBSource)
		if !ok || internallob.IsNilSource(source) {
			return common.NewOracleError(oracleErrors.InvalidLobSource, nil, "source type")
		}
		if !internallob.ValidKind(source.Kind()) {
			_ = source.Close()
			return common.NewOracleError(oracleErrors.InvalidLobSource, nil, "LOB kind")
		}
		if source.Kind() != expectedKind {
			_ = source.Close()
			return common.NewOracleError(oracleErrors.InvalidLobSource, nil, "LOB kind")
		}
	}
	var handle *lobHandle
	if source != nil {
		var err error
		adopter, ok := source.(internallob.PersistentLocatorAdopter)
		if !ok {
			_ = source.Close()
			return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "adopt-persistent", "LOB", "source cannot transfer connection ownership")
		}
		handle, err = adoptPersistentLOB(adopter, expectedKind)
		if err != nil {
			_ = source.Close()
			return err
		}
	}

	value.mu.Lock()
	previous := value.handle
	value.mu.Unlock()
	if previous != nil {
		if freeErr := previous.Free(context.Background()); freeErr != nil {
			if handle != nil {
				freeErr = errors.Join(freeErr, handle.Free(context.Background()))
			}
			return freeErr
		}
	}
	value.mu.Lock()
	value.handle = handle
	value.kind = expectedKind
	value.mu.Unlock()
	return nil
}

func newTemporaryTypedLOB(ctx context.Context, conn *sql.Conn, kind Kind) (*lobHandle, error) {
	if conn == nil {
		return nil, invalidLOBConnection("create temporary", "nil *sql.Conn")
	}
	return createTemporaryLOB(ctx, conn, kind)
}

func (value *lobValue) current() *lobHandle {
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.handle
}

func (value *lobValue) activeHandle() (*lobHandle, error) {
	handle := value.current()
	if handle != nil {
		return handle, nil
	}
	return nil, common.NewOracleError(oracleErrors.NullLobValue, nil, "unscanned value")
}

// Read implements io.Reader. It uses context.Background; use ReadContext when
// cancellation or a deadline is required.
func (value *lobValue) Read(buffer []byte) (int, error) {
	return value.ReadContext(context.Background(), buffer)
}

// ReadContext reads from the current logical offset using ctx.
func (value *lobValue) ReadContext(ctx context.Context, buffer []byte) (int, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return 0, err
	}
	return handle.ReadContext(ctx, buffer)
}

func (value *lobValue) hasUnreadDataContext(ctx context.Context) (bool, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return false, err
	}
	return handle.hasUnreadDataContext(ctx)
}

// Write writes BLOB bytes or UTF-8 text at the current logical offset using
// context.Background. Each CLOB/NCLOB call must contain complete, valid UTF-8
// characters. For io.Copy, use a reader or adapter that returns chunks ending
// at character boundaries. A large call is split internally at those
// boundaries. It returns the bytes acknowledged by Oracle, so an error after
// partial progress may return a non-zero count. Use WriteContext when
// cancellation or a deadline is required.
func (value *lobValue) Write(data []byte) (int, error) {
	return value.WriteContext(context.Background(), data)
}

// WriteContext writes at the caller-visible logical offset using ctx. Each
// CLOB/NCLOB call must contain complete, valid UTF-8 characters. For
// io.Copy, use a reader or adapter that returns chunks ending at character
// boundaries. A large call is split internally at those boundaries. If a
// preceding read stopped inside a UTF-8 character, the write returns
// InvalidLOBBuffer because Oracle has no character offset for that position.
// Large inputs are sent in bounded requests, with text requests split at
// character boundaries. The returned count is the number of bytes acknowledged
// by Oracle, including an acknowledged prefix if a later request fails. A
// successful or short write discards buffered read-ahead. An error after a
// request is attempted invalidates the handle because Oracle may have applied
// that request despite the error; later operations return LobValueInvalidated.
func (value *lobValue) WriteContext(ctx context.Context, data []byte) (int, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return 0, err
	}
	return handle.WriteContext(ctx, data)
}

// writeStringContext is the shared implementation for the public CLOB and
// NCLOB string writers. BLOB intentionally has no string-writing API.
func (value *lobValue) writeStringContext(ctx context.Context, text string) (int, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return 0, err
	}
	if handle.kind != CLOB && handle.kind != NCLOB {
		return 0, invalidLOBConnection("write", "WriteString requires CLOB or NCLOB")
	}
	return handle.WriteStringContext(ctx, text)
}

// WriteTo copies the remaining LOB to writer using context.Background.
func (value *lobValue) WriteTo(writer io.Writer) (int64, error) {
	return value.WriteToContext(context.Background(), writer)
}

// WriteToContext copies the remaining LOB to writer using ctx.
func (value *lobValue) WriteToContext(ctx context.Context, writer io.Writer) (int64, error) {
	if writer == nil {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write-to", "LOB", "nil writer")
	}
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		n, readErr := value.ReadContext(ctx, buffer)
		if n > 0 {
			written, writeErr := writer.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
		if n == 0 {
			return total, io.ErrNoProgress
		}
	}
}

// Size returns the logical LOB length using context.Background.
func (value *lobValue) Size() (int64, error) {
	return value.SizeContext(context.Background())
}

// SizeContext returns the logical LOB length using ctx.
func (value *lobValue) SizeContext(ctx context.Context) (int64, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return 0, err
	}
	return handle.Size(ctx)
}

// ChunkSize reports Oracle's storage chunk size in bytes.
func (value *lobValue) ChunkSize() (int64, error) {
	return value.ChunkSizeContext(context.Background())
}

// ChunkSizeContext reports Oracle's storage chunk size in bytes using ctx.
func (value *lobValue) ChunkSizeContext(ctx context.Context) (int64, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return 0, err
	}
	return handle.ChunkSize(ctx)
}

// Trim changes the server LOB length using context.Background.
func (value *lobValue) Trim(length int64) error {
	return value.TrimContext(context.Background(), length)
}

// TrimContext changes the server LOB length using ctx. The length uses bytes
// for BLOBs and UTF-16 code units for CLOBs and NCLOBs. On success, trimming to
// zero resets the sequential cursor to the beginning. A nonzero text trim is
// rejected if a read has returned only part of a UTF-8 character; finish
// reading that character first. Other nonzero trims preserve the cursor unless
// it is beyond the new end, when it is clamped to the new EOF position.
// Buffered read-ahead is discarded after a successful trim. An error after
// the trim operation is attempted invalidates the handle because Oracle may
// have applied the trim despite the error; later operations return
// LobValueInvalidated.
func (value *lobValue) TrimContext(ctx context.Context, length int64) error {
	handle, err := value.activeHandle()
	if err != nil {
		return err
	}
	return handle.Trim(ctx, length)
}

// Open opens the LOB using context.Background.
func (value *lobValue) Open(mode OpenMode) (bool, error) {
	return value.OpenContext(context.Background(), mode)
}

// OpenContext opens the LOB using ctx. If an attempted persistent open returns
// an error, the cached server state becomes unknown; use IsOpenContext to
// reconcile before retrying Open, CloseServer, Close, or Free.
func (value *lobValue) OpenContext(ctx context.Context, mode OpenMode) (bool, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return false, err
	}
	return handle.Open(ctx, mode)
}

// CloseServer closes an explicit server-side open using context.Background.
func (value *lobValue) CloseServer() error {
	return value.CloseServerContext(context.Background())
}

// CloseServerContext closes an explicit server-side open using ctx. If an
// attempted persistent close returns an error, use IsOpenContext to reconcile
// before retrying CloseServer or releasing the local handle.
func (value *lobValue) CloseServerContext(ctx context.Context) error {
	handle, err := value.activeHandle()
	if err != nil {
		return err
	}
	return handle.CloseServer(ctx)
}

// IsOpen reports server-side open state using context.Background.
func (value *lobValue) IsOpen() (bool, error) {
	return value.IsOpenContext(context.Background())
}

// IsOpenContext reports server-side open state using ctx and reconciles the
// cached state after a successful query. A failed query leaves an unknown
// state unresolved.
func (value *lobValue) IsOpenContext(ctx context.Context) (bool, error) {
	handle, err := value.activeHandle()
	if err != nil {
		return false, err
	}
	return handle.IsOpen(ctx)
}

// Close releases local client state. Call Free to release temporary storage.
// It returns LobOpenStateUnknown if a persistent open/close operation failed
// ambiguously; call IsOpenContext to reconcile the state first.
func (value *lobValue) Close() error {
	handle := value.current()
	if handle == nil {
		return nil
	}
	return handle.Close()
}

// Free releases the LOB handle and temporary storage using context.Background.
// A successful call invalidates the value; repeated calls are no-ops.
func (value *lobValue) Free() error {
	return value.FreeContext(context.Background())
}

// FreeContext releases the LOB handle and temporary storage using ctx. If a
// persistent open/close operation left the state unknown, call IsOpenContext
// to reconcile it first; FreeContext does not perform an implicit probe. A
// successful call invalidates the value and repeated calls are no-ops.
func (value *lobValue) FreeContext(ctx context.Context) error {
	handle := value.current()
	if handle == nil {
		return nil
	}
	return handle.Free(ctx)
}

// Kind returns the typed LOB kind, including when the scanned value is NULL.
func (value *lobValue) Kind() Kind { return value.kind }

// Valid reports whether the value is non-NULL and has not been successfully
// freed. A zero-length LOB is valid. It remains true after local Close or
// handle invalidation, so it does not guarantee that locator operations can
// still succeed.
func (value *lobValue) Valid() bool {
	handle := value.current()
	return handle != nil && handle.Valid()
}

// IsTemporary reports whether the value is backed by an Oracle temporary LOB.
func (value *lobValue) IsTemporary() bool {
	handle := value.current()
	return handle != nil && handle.IsTemporary()
}

type contextReader interface {
	ReadContext(context.Context, []byte) (int, error)
	hasUnreadDataContext(context.Context) (bool, error)
}

func readRemainingContext(ctx context.Context, reader contextReader, kind string) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, 32*1024)
	for {
		remaining := int64(math.MaxInt32) - int64(output.Len())
		if remaining == 0 {
			more, err := reader.hasUnreadDataContext(ctx)
			if err != nil {
				return output.Bytes(), err
			}
			if more {
				return output.Bytes(), common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "materialize", kind, "value exceeds MaxInt32")
			}
			return output.Bytes(), nil
		}
		readBuffer := buffer
		if int64(len(readBuffer)) > remaining {
			readBuffer = readBuffer[:int(remaining)]
		}
		n, err := reader.ReadContext(ctx, readBuffer)
		if n > 0 {
			_, _ = output.Write(readBuffer[:n])
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return output.Bytes(), err
		}
		if n == 0 {
			return output.Bytes(), io.ErrNoProgress
		}
	}
}

var (
	_ LOB = (*Blob)(nil)
	_ LOB = (*Clob)(nil)
	_ LOB = (*NClob)(nil)
)
