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

package lob

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// OpenMode selects the access mode used by DirectLOB.Open. Persistent locators
// send this mode to Oracle; temporary locators record it in local flags.
type OpenMode uint8

const (
	// ReadOnly opens the LOB for server-side read operations only.
	ReadOnly OpenMode = iota + 1
	// ReadWrite opens the LOB for server-side reads and writes.
	ReadWrite
)

// DirectLOB is a connection-bound persistent or temporary Oracle LOB.
//
// CreateTemporary creates a DirectLOB on a dedicated *sql.Conn. The locator
// belongs to that physical Oracle session, so conn must remain open until Free
// succeeds. DirectLOB serializes its operations and must not be copied after
// first use. Concurrent callers need their own coordination when the ordering
// of reads and writes matters. Read and Write use bytes for a BLOB and UTF-8
// for a CLOB or NCLOB; Size, Trim, and locator offsets use Oracle logical units
// (bytes for a BLOB and UTF-16 code units for a CLOB or NCLOB). Close releases
// only client state unless an explicit persistent Open still requires
// CloseServer; Free is the unified cleanup method for both locator kinds.
//
// CLOB and NCLOB support is currently validated for a database character set
// of AL32UTF8 and a national character set of AL16UTF16. Text is exchanged
// with Go as UTF-8, while Oracle logical offsets, lengths, and acknowledgements
// use UTF-16 code units. Other server or national character-set combinations
// are not currently validated by the driver.
type DirectLOB struct {
	// mu serializes state changes and complete database/sql.Raw operations.
	mu sync.Mutex
	// conn is the dedicated physical-session owner of locator.
	conn *sql.Conn
	// kind selects BLOB, CLOB, or NCLOB executor and unit semantics.
	kind Kind
	// locator is the mutable TTC locator byte copy owned by this handle.
	locator []byte
	// offset is the next one-based Oracle logical read or write position.
	offset uint64
	// pending holds UTF-8 bytes that did not fit in the caller read buffer.
	pending []byte
	// closed prevents all later operations after local Close.
	closed bool
	// freed records a successful unified Free operation.
	freed bool
	// invalidated records a protocol acknowledgement that cannot be represented
	// as a valid public write result. The terminal TTC response was consumed, so
	// the session remains usable, but this handle's cursor is no longer trusted.
	invalidated bool
	// temporary distinguishes locators created by CreateTemporary from persistent
	// query locators promoted by OpenPersistent.
	temporary bool
	// serverOpen records a successful explicit Open on a persistent locator.
	// CloseServer must succeed before local Close or Free can release the handle.
	serverOpen bool
}

// CreateTemporary creates a session-duration temporary BLOB, CLOB, or NCLOB
// on conn. The caller must use a dedicated *sql.Conn rather than *sql.DB so
// every operation reaches the physical session that owns the locator. kind
// must be BLOB, CLOB, or NCLOB. It returns an InvalidLOBBuffer error for an
// invalid argument and propagates database/sql and Oracle errors unchanged.
//
// Parameters:
//   - ctx: context for temporary-LOB creation.
//   - conn: dedicated connection that owns the new locator.
//   - kind: BLOB, CLOB, or NCLOB.
//
// Returns:
//   - *DirectLOB: newly created session-duration locator handle.
//   - error: nil on success. CLOB and NCLOB creation follows the package's
//     validated character-set scope.
func CreateTemporary(ctx context.Context, conn *sql.Conn, kind Kind) (*DirectLOB, error) {
	// Reject invalid inputs before entering Raw; no session operation is needed
	// when the requested LOB kind or connection is unusable.
	if conn == nil || !internallob.ValidKind(kind) {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "create", "LOB", "invalid argument")
	}
	var locator []byte
	// Raw keeps the concrete driver connection inside database/sql while the
	// driver creates the session-owned locator.
	err := conn.Raw(func(raw any) error {
		driver, ok := raw.(directLOBDriver)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "direct LOB")
		}
		var err error
		locator, err = driver.LobCreate(ctx, uint8(kind))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &DirectLOB{conn: conn, kind: kind, locator: locator, offset: 1, temporary: true}, nil
}

// OpenPersistent promotes an unread persistent query LOB to a direct handle.
// The query that populated source must have run through the same dedicated
// *sql.Conn. On success the source is closed and the returned handle remains
// valid after the producing Rows and Statement close.
//
// It returns InvalidLOBBuffer when source is nil, NULL, already read, or bound
// to a different physical connection. If the producing Rows was closed or its
// query context was canceled, the source has already been invalidated and the
// corresponding lifecycle error is returned. Temporary and abstract query
// locators are rejected during query decoding and cannot be promoted.
//
// Parameters:
//   - ctx: context for the promotion operation.
//   - conn: dedicated connection that owns source's locator.
//   - source: unread persistent query LOB to promote.
//
// Returns:
//   - *DirectLOB: connection-bound persistent locator handle.
//   - error: nil on success.
func OpenPersistent(ctx context.Context, conn *sql.Conn, source *LOB) (*DirectLOB, error) {
	if conn == nil || source == nil {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open-persistent", "LOB", "invalid argument")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Serialize promotion with Scan so the source cannot be replaced while its
	// locator is being transferred.
	source.scanMu.Lock()
	defer source.scanMu.Unlock()
	// Keep the source attached until DetachPersistentLocator succeeds. Close
	// therefore waits for promotion and cannot invalidate a successful handoff.
	source.mu.Lock()
	defer source.mu.Unlock()
	if !source.valid || source.closed || source.source == nil {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open-persistent", "LOB", "closed or NULL")
	}
	locatorSource, ok := source.source.(internallob.PersistentLocatorSource)
	kind := source.kind
	if !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open-persistent", "LOB", "not persistent")
	}
	var locator []byte
	// The driver callback verifies physical-session affinity and performs the
	// source-side detach while both wrapper locks are held.
	err := conn.Raw(func(raw any) error {
		driver, ok := raw.(directLOBDriver)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "direct LOB")
		}
		var detachErr error
		locator, detachErr = locatorSource.DetachPersistentLocator(driver.LobSessionKey())
		return detachErr
	})
	if err != nil {
		return nil, err
	}
	// Publish the terminal source transition only after the locator has been
	// copied successfully into the new connection-bound handle.
	source.source = nil
	source.closed = true
	return &DirectLOB{conn: conn, kind: kind, locator: locator, offset: 1}, nil
}

// Read reads from the current logical offset using context.Background. CLOB
// and NCLOB text is UTF-8. It returns io.EOF after all data is consumed.
// Use ReadContext when cancellation or a deadline is required.
//
// Parameters:
//   - dst: destination for the next LOB bytes.
//
// Returns:
//   - int: bytes copied into dst.
//   - error: io.EOF at end of the LOB or an operation error.
func (dlob *DirectLOB) Read(dst []byte) (int, error) {
	return dlob.ReadContext(context.Background(), dst)
}

// ReadContext reads from the current logical offset using ctx. CLOB and NCLOB
// text is UTF-8 within the package's validated character-set scope. It returns
// io.EOF after all data is consumed and propagates ctx cancellation and Oracle
// read errors unchanged.
//
// Parameters:
//   - ctx: context for the read RPC.
//   - dst: destination for the next LOB bytes.
//
// Returns:
//   - int: bytes copied into dst.
//   - error: io.EOF at end of the LOB or an operation error.
func (dlob *DirectLOB) ReadContext(ctx context.Context, dst []byte) (int, error) {
	// Keep the cursor, pending bytes, and one raw-driver exchange under the
	// same lock so concurrent calls cannot reorder or duplicate reads.
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return 0, err
	}
	if len(dst) == 0 {
		return 0, nil
	}
	// A previous refill may contain more UTF-8 bytes than the caller requested.
	// Serve those bytes before asking Oracle for another logical-unit range.
	if len(dlob.pending) != 0 {
		n := copy(dst, dlob.pending)
		dlob.pending = dlob.pending[n:]
		return n, nil
	}
	readSize := internallob.DefaultBlobLobChunkBytes
	if dlob.kind != BLOB {
		// dst is measured in UTF-8 bytes, whereas Oracle character LOB reads
		// are measured in UTF-16 units. Refill a bounded character chunk and
		// retain any excess UTF-8 bytes in pending, so a 1-3 byte caller buffer
		// cannot make the TTC request end halfway through a surrogate pair.
		readSize = internallob.DefaultCharacterLobChunkChars
	}
	var data []byte
	for len(data) == 0 {
		read, logical, err := dlob._readLocked(ctx, readSize)
		if err != nil {
			return 0, err
		}
		if logical == 0 {
			return 0, io.EOF
		}
		dlob.offset += logical
		data = read
	}
	n := copy(dst, data)
	dlob.pending = append(dlob.pending[:0], data[n:]...)
	return n, nil
}

// Write writes at the current logical offset and advances it by the data the
// server accepted. CLOB and NCLOB input must be valid UTF-8. Use WriteContext
// when cancellation or a deadline is required.
//
// Parameters:
//   - data: BLOB bytes or UTF-8 CLOB/NCLOB text.
//
// Returns:
//   - int: bytes accepted from data.
//   - error: validation or operation error.
func (dlob *DirectLOB) Write(data []byte) (int, error) {
	return dlob.WriteContext(context.Background(), data)
}

// WriteContext writes at the current logical offset using ctx and advances it
// by the data the server accepted. It returns io.ErrShortWrite when Oracle
// acknowledges fewer than the requested logical units, InvalidLOBBuffer for
// malformed CLOB/NCLOB UTF-8 or an invalid acknowledgement, and otherwise
// propagates ctx and Oracle errors.
//
// Parameters:
//   - ctx: context for the write RPC.
//   - data: BLOB bytes or UTF-8 CLOB/NCLOB text.
//
// Returns:
//   - int: bytes accepted from data.
//   - error: validation or operation error.
func (dlob *DirectLOB) WriteContext(ctx context.Context, data []byte) (int, error) {
	// Serialize writes with reads because both advance the same logical offset.
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}
	if dlob.kind != BLOB && !utf8.Valid(data) {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "invalid UTF-8")
	}
	var logical uint64
	err := dlob._executeOnRawDriver(func(driver directLOBDriver) error {
		var err error
		logical, err = driver.LobWrite(ctx, uint8(dlob.kind), dlob.locator, dlob.offset, data)
		return err
	})
	if err != nil {
		return 0, err
	}
	accepted, acknowledgeErr := _acceptedWriteBytes(dlob.kind, data, logical)
	if acknowledgeErr == nil || errors.Is(acknowledgeErr, io.ErrShortWrite) {
		// A short write is a valid, fully-delimited server result. Advance to the
		// acknowledged logical position so callers may retry the remaining bytes.
		dlob.offset += logical
		return accepted, acknowledgeErr
	}
	// The response was consumed but its amount cannot map to a valid BLOB byte
	// count or UTF-8 character boundary. Do not commit an untrustworthy cursor.
	dlob.invalidated = true
	dlob.pending = nil
	return accepted, acknowledgeErr
}

// WriteTo copies the remaining LOB to writer using bounded reads. It stops at
// EOF and returns the bytes written. Writer errors and short writes are
// returned directly; a nil writer returns InvalidLOBBuffer.
//
// Parameters:
//   - writer: destination for the remaining LOB bytes.
//
// Returns:
//   - int64: bytes written.
//   - error: write, read, or lifecycle error.
func (dlob *DirectLOB) WriteTo(writer io.Writer) (int64, error) {
	if writer == nil {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write-to", "LOB", "nil writer")
	}
	buffer := make([]byte, internallob.DefaultBlobLobChunkBytes)
	var total int64
	for {
		n, readErr := dlob.Read(buffer)
		if n != 0 {
			written, writeErr := writer.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

// Size returns the current server length: bytes for BLOB and UTF-16 code units
// for CLOB and NCLOB. It performs a locator RPC and honors ctx.
//
// Parameters:
//   - ctx: context for the length RPC.
//
// Returns:
//   - int64: logical LOB length.
//   - error: locator RPC or lifecycle error.
func (dlob *DirectLOB) Size(ctx context.Context) (int64, error) {
	return dlob._uintOperation(ctx, "size", func(d directLOBDriver) (uint64, error) {
		return d.LobLength(ctx, uint8(dlob.kind), dlob.locator)
	})
}

// ChunkSize returns Oracle's server storage chunk size in bytes. It performs a
// locator RPC and does not change this handle's bounded read-buffer size.
//
// Parameters:
//   - ctx: context for the chunk-size RPC.
//
// Returns:
//   - int64: server storage chunk size.
//   - error: locator RPC or lifecycle error.
func (dlob *DirectLOB) ChunkSize(ctx context.Context) (int64, error) {
	return dlob._uintOperation(ctx, "chunk-size", func(d directLOBDriver) (uint64, error) {
		return d.LobChunkSize(ctx, uint8(dlob.kind), dlob.locator)
	})
}

// Trim changes the server LOB length in the units reported by Size. The LOB
// must be writable. A negative length returns InvalidLOBBuffer.
//
// Parameters:
//   - ctx: context for the trim RPC.
//   - length: target logical length.
//
// Returns:
//   - error: nil on success.
func (dlob *DirectLOB) Trim(ctx context.Context, length int64) error {
	if length < 0 {
		return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "trim", "LOB", "negative length")
	}
	// A successful trim changes the server contents, so prefetched bytes from
	// before the trim can no longer be returned to the caller.
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return err
	}
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error {
		_, err := d.LobTrim(ctx, uint8(dlob.kind), dlob.locator, uint64(length))
		return err
	})
	if err == nil {
		dlob.pending = nil
	}
	return err
}

// Open opens a persistent LOB server-side with the selected mode. For a
// temporary LOB it updates the locator's local open/access flags and sends no
// TTC request. It returns true only if it changed the locator state. The
// caller should call CloseServer before Free when a persistent Open completed
// successfully.
//
// Parameters:
//   - ctx: context for the open RPC.
//   - mode: ReadOnly or ReadWrite.
//
// Returns:
//   - bool: true when this call changed server state.
//   - error: validation, RPC, or lifecycle error.
func (dlob *DirectLOB) Open(ctx context.Context, mode OpenMode) (bool, error) {
	if mode != ReadOnly && mode != ReadWrite {
		return false, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open", "LOB", "invalid mode")
	}
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return false, err
	}
	var opened bool
	var locator []byte
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error {
		var err error
		opened, locator, err = d.LobOpen(ctx, uint8(dlob.kind), dlob.locator, uint8(mode))
		return err
	})
	if err == nil {
		// The bridge returns a fresh locator because temporary opens modify its
		// local flags; persistent opens return the server-updated locator copy.
		dlob.locator = locator
		if opened && !dlob.temporary {
			dlob.serverOpen = true
		}
	}
	return opened, err
}

// Close closes the local Go handle and discards buffered data. It is idempotent
// and does not send Oracle's server-side close or free a temporary LOB. For a
// persistent LOB opened with Open, Close returns LobOpen until CloseServer
// succeeds, leaving the handle usable for the required server transition.
//
// Returns:
//   - error: nil on success, or LobOpen when an explicit persistent Open still
//     requires CloseServer.
func (dlob *DirectLOB) Close() error {
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if dlob.serverOpen {
		return common.NewOracleError(oracleErrors.LobOpen, nil)
	}
	// Close only ends the local handle. Explicit persistent server state must be
	// closed through CloseServer before this transition is allowed.
	dlob.closed = true
	dlob.pending = nil
	return nil
}

// CloseServer closes a persistent LOB on the server after Open. For a
// temporary LOB it clears the local open/access flags and sends no TTC
// request. It leaves the Go handle usable for later operations. A failed
// persistent close preserves the open state so the caller can retry before
// local Close or Free.
//
// Parameters:
//   - ctx: context for the server-close RPC.
//
// Returns:
//   - error: RPC or lifecycle error.
func (dlob *DirectLOB) CloseServer(ctx context.Context) error {
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return err
	}
	var locator []byte
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error {
		var err error
		locator, err = d.LobClose(ctx, uint8(dlob.kind), dlob.locator)
		return err
	})
	if err == nil {
		// Do not clear serverOpen until Oracle confirms the close. This leaves a
		// failed close retryable and keeps local Close/Free from stranding it.
		dlob.locator = locator
		dlob.serverOpen = false
	}
	return err
}

// IsOpen reports the current open state. Persistent locators query Oracle and
// temporary locators read their local open flag; temporary checks send no TTC
// request. The operation honors ctx.
//
// Parameters:
//   - ctx: context for the state RPC.
//
// Returns:
//   - bool: current server-side open state.
//   - error: RPC or lifecycle error.
func (dlob *DirectLOB) IsOpen(ctx context.Context) (bool, error) {
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return false, err
	}
	var open bool
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error {
		var err error
		open, err = d.LobIsOpen(ctx, uint8(dlob.kind), dlob.locator)
		return err
	})
	return open, err
}

// Free releases this handle's resources. For a persistent LOB it closes local
// client state once any explicit server open has been closed. For a temporary
// LOB it queues an Oracle temporary-LOB free on the owning session's next
// ordinary TTC request and then closes local state. It is idempotent and must
// be called before closing the owning *sql.Conn when the LOB is temporary.
//
// Parameters:
//   - ctx: context for the driver operation that queues the free.
//
// Returns:
//   - error: nil on success, LobOpen when an explicit persistent Open still
//     requires CloseServer, or a context/temporary-free error.
func (dlob *DirectLOB) Free(ctx context.Context) error {
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if dlob.freed {
		return nil
	}
	if dlob.serverOpen {
		return common.NewOracleError(oracleErrors.LobOpen, nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !dlob.temporary {
		// Persistent locators are owned by Oracle storage; Free only releases
		// this Go handle after any required server close has completed.
		dlob.closed = true
		dlob.pending = nil
		dlob.freed = true
		return nil
	}
	// Temporary storage is released through the session registry. The handle
	// becomes terminal only after the registry accepts the release.
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error { return d.LobFree(ctx, dlob.locator) })
	if err == nil {
		dlob.freed = true
		dlob.closed = true
		dlob.pending = nil
	}
	return err
}

// Kind identifies this LOB's Oracle family and never performs a locator RPC.
//
// Returns:
//   - Kind: BLOB, CLOB, NCLOB, or Unknown.
func (dlob *DirectLOB) Kind() Kind { return dlob.kind }

// IsTemporary reports whether this LOB is backed by an Oracle temporary
// locator. It is a local property and never performs a locator RPC. The value
// remains the same after the handle is closed or freed.
func (dlob *DirectLOB) IsTemporary() bool { return dlob.temporary }

// directLOBDriver is the private capability contract implemented by the driver's
// raw connection. It isolates DirectLOB from TTC types while preserving the
// physical-session affinity required by Oracle LOB locators.
//
// Each method accepts copied locator bytes and returns only Go values, so a
// database/sql.Raw callback never exposes the underlying driver connection.
// Logical offsets and lengths are bytes for BLOBs and UTF-16 code units for
// CLOBs and NCLOBs.
type directLOBDriver interface {
	LobCreate(context.Context, uint8) ([]byte, error)
	LobRead(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error)
	LobWrite(context.Context, uint8, []byte, uint64, []byte) (uint64, error)
	LobLength(context.Context, uint8, []byte) (uint64, error)
	LobChunkSize(context.Context, uint8, []byte) (uint64, error)
	LobTrim(context.Context, uint8, []byte, uint64) (uint64, error)
	LobOpen(context.Context, uint8, []byte, uint8) (bool, []byte, error)
	LobClose(context.Context, uint8, []byte) ([]byte, error)
	LobIsOpen(context.Context, uint8, []byte) (bool, error)
	LobFree(context.Context, []byte) error
	LobSessionKey() any
}

// _readLocked performs one bounded locator read while dlob.mu is held. The caller
// owns locking so the returned data and logical amount can be applied to the
// same cursor without another DirectLOB operation interleaving.
//
// Parameters:
//   - ctx: context for the read RPC.
//   - size: maximum logical amount to read.
//
// Returns:
//   - []byte: BLOB bytes or UTF-8 CLOB/NCLOB data.
//   - uint64: logical units consumed from the current offset.
//   - error: nil on success.
func (dlob *DirectLOB) _readLocked(ctx context.Context, size int) ([]byte, uint64, error) {
	var data []byte
	var logical uint64
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error {
		var err error
		data, logical, err = d.LobRead(ctx, uint8(dlob.kind), dlob.locator, dlob.offset, uint64(size))
		return err
	})
	return data, logical, err
}

// _acceptedWriteBytes translates a server acknowledgement in Oracle logical
// units into an io.Writer byte count. Oracle must acknowledge whole Unicode
// scalar values for CLOB/NCLOB writes; any partial character unit is a protocol
// error because it cannot be represented by a valid UTF-8 prefix.
func _acceptedWriteBytes(kind Kind, data []byte, logical uint64) (int, error) {
	if kind == BLOB {
		if logical > uint64(len(data)) {
			return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "ack exceeds payload")
		}
		if logical != uint64(len(data)) {
			return int(logical), io.ErrShortWrite
		}
		return len(data), nil
	}

	var units uint64
	var bytes int
	for _, r := range string(data) {
		runeUnits := uint64(1)
		if r >= 0x10000 {
			runeUnits = 2
		}
		if units+runeUnits > logical {
			break
		}
		units += runeUnits
		bytes += utf8.RuneLen(r)
	}
	if units != logical {
		return 0, common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"write",
			"LOB",
			"split UTF-16 ack",
		)
	}
	if bytes != len(data) {
		return bytes, io.ErrShortWrite
	}
	return bytes, nil
}

// _uintOperation invokes one scalar locator operation while holding dlob.mu and
// checks that its unsigned result fits the public int64 return type.
//
// Parameters:
//   - ctx: context for the locator RPC.
//   - operationName: short operation label used in range errors.
//   - operation: raw-connection operation returning an unsigned scalar.
//
// Returns:
//   - int64: scalar result when representable by the public API.
//   - error: operation, lifecycle, or range error.
func (dlob *DirectLOB) _uintOperation(ctx context.Context, operationName string, operation func(directLOBDriver) (uint64, error)) (int64, error) {
	dlob.mu.Lock()
	defer dlob.mu.Unlock()
	if err := dlob._stateErrorLocked(); err != nil {
		return 0, err
	}
	var n uint64
	err := dlob._executeOnRawDriver(func(d directLOBDriver) error { var err error; n, err = operation(d); return err })
	if err != nil {
		return 0, err
	}
	if n > uint64(^uint64(0)>>1) {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, operationName, "LOB", "exceeds int64")
	}
	return int64(n), nil
}

// _executeOnRawDriver invokes operation through database/sql.Raw while dlob.mu
// is held. It does not acquire dlob.mu itself; callers must hold it.
// Keeping the callback inside Raw prevents the concrete driver connection and
// its mutable TTC state from escaping the database/sql connection boundary.
//
// Parameters:
//   - operation: raw-driver operation performed during the callback.
//
// Returns:
//   - error: Raw callback, bridge-capability, or operation error.
func (dlob *DirectLOB) _executeOnRawDriver(operation func(directLOBDriver) error) error {
	return dlob.conn.Raw(func(raw any) error {
		driver, ok := raw.(directLOBDriver)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "direct LOB")
		}
		return operation(driver)
	})
}

// _stateErrorLocked reports whether dlob can accept another operation. It must
// be called with dlob.mu held. Free is intentionally handled separately because it
// must remain available after local Close to release a temporary locator.
//
// Returns:
//   - error: LobValueClosed when the handle cannot be used, otherwise nil.
func (dlob *DirectLOB) _stateErrorLocked() error {
	if dlob.freed {
		return common.NewOracleError(oracleErrors.LobValueClosed, nil, "after Free")
	}
	if dlob.invalidated {
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "invalid write ack")
	}
	if dlob.closed || dlob.conn == nil {
		return common.NewOracleError(oracleErrors.LobValueClosed, nil, "after Close")
	}
	return nil
}
