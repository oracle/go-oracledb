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
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// persistentOpenState records the driver's knowledge of a persistent
// locator's server-side open state. The zero value is closed so newly scanned
// and temporary handles retain their existing initialization behavior.
type persistentOpenState uint8

const (
	persistentOpenClosed persistentOpenState = iota
	persistentOpenOpen
	persistentOpenUnknown
)

// lobHandle is a connection-bound persistent or temporary Oracle LOB.
//
// createTemporaryLOB creates a lobHandle on a dedicated *sql.Conn. The locator
// belongs to that physical Oracle session, so conn must remain open until Free
// succeeds. lobHandle serializes its operations and must not be copied after
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
type lobHandle struct {
	// mu serializes state changes and complete database/sql.Raw operations.
	mu sync.Mutex
	// conn is the dedicated physical-session owner of locator.
	conn *sql.Conn
	// sessionDriver is a lease-checked driver capability for a query locator
	// adopted from Rows. It avoids retaining the outer *sql.Conn while still
	// expiring the handle before database/sql returns its physical connection to
	// the pool.
	sessionDriver internallob.LOBSession
	// kind selects BLOB, CLOB, or NCLOB executor and unit semantics.
	kind Kind
	// locator is the mutable TTC locator byte copy owned by this handle.
	locator []byte
	// offset is the next one-based logical position visible to the caller. For
	// CLOB/NCLOB it advances only when a complete UTF-8 character is delivered.
	offset uint64
	// fetchOffset is the next Oracle position to fetch. It may be ahead of
	// offset while pending contains read-ahead bytes. Zero means the first fetch
	// starts at offset.
	fetchOffset uint64
	// pending holds unread public bytes adopted from a row prefix or retained
	// after a locator read. It contains BLOB bytes or UTF-8 CLOB/NCLOB text.
	pending []byte
	// readComplete means the buffered prefix covers all remaining data, so reads
	// can return EOF after it is consumed without another locator RPC.
	readComplete bool
	// utf8Carry holds bytes of a CLOB/NCLOB character already delivered across
	// Read calls but not yet complete. Mutations cannot use a cursor inside it.
	utf8Carry []byte
	// closed prevents all later operations after local Close.
	closed bool
	// freed records a successful unified Free operation.
	freed bool
	// invalidated records a handle state that can no longer be trusted, such as
	// an invalid write acknowledgement or an error after a mutation was
	// attempted. The physical session may remain usable when the TTC response
	// was consumed.
	invalidated bool
	// ownerLost records that the physical session lease is gone, so no locator
	// operation, including open-state reconciliation, can succeed.
	ownerLost bool
	// temporary distinguishes locators created by createTemporaryLOB from persistent
	// query locators adopted from Rows.
	temporary bool
	// serverOpenState records the last known server-side open state for a
	// persistent locator. An attempted Open or CloseServer that returns an error
	// makes the state unknown until IsOpen succeeds.
	serverOpenState persistentOpenState
}

// createTemporaryLOB creates a session-duration temporary BLOB, CLOB, or NCLOB
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
//   - *lobHandle: newly created session-duration locator handle.
//   - error: nil on success. CLOB and NCLOB creation follows the package's
//     validated character-set scope.
func createTemporaryLOB(ctx context.Context, conn *sql.Conn, kind Kind) (*lobHandle, error) {
	// Reject invalid inputs before entering Raw; no session operation is needed
	// when the requested LOB kind or connection is unusable.
	if conn == nil || !internallob.ValidKind(kind) {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "create", "LOB", "invalid argument")
	}
	var locator []byte
	// Raw keeps the concrete driver connection inside database/sql while the
	// driver creates the session-owned locator.
	err := conn.Raw(func(raw any) error {
		driver, ok := raw.(lobHandleDriver)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "typed LOB")
		}
		var err error
		locator, err = driver.LobCreate(ctx, uint8(kind))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &lobHandle{conn: conn, kind: kind, locator: locator, offset: 1, fetchOffset: 1, temporary: true}, nil
}

// adoptPersistentLOB transfers an unread persistent locator and its prefetched
// row bytes into a lease-checked driver handle. The source supplies the
// operation capability for the physical session that produced the locator.
func adoptPersistentLOB(source internallob.PersistentLocatorAdopter, kind Kind) (*lobHandle, error) {
	if source == nil || !internallob.ValidKind(kind) {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "adopt-persistent", "LOB", "invalid argument")
	}
	state, driver, err := source.AdoptPersistentLocator(kind)
	if err != nil {
		return nil, err
	}
	if len(state.Locator) == 0 || state.FetchOffset == 0 || driver == nil {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "adopt-persistent", "LOB", "invalid source capability")
	}
	return &lobHandle{
		sessionDriver: driver,
		kind:          kind,
		locator:       append([]byte(nil), state.Locator...),
		offset:        1,
		fetchOffset:   state.FetchOffset,
		pending:       state.Prefix,
		readComplete:  state.PrefixComplete,
	}, nil
}

type lobHandleDriver = internallob.LOBHandleDriver

// acquireSessionLease keeps a query LOB's owning *sql.Conn lease active for a
// complete handle operation. Temporary LOBs and explicitly connection-bound
// scanner handles use database/sql.Raw instead and need no extra lock.
func (handle *lobHandle) acquireSessionLease() (func(), error) {
	if handle.sessionDriver == nil {
		return func() {}, nil
	}
	release, err := handle.sessionDriver.AcquireLOBLease()
	if handle.ownerSessionLost(err) {
		handle.invalidateOwnerLocked()
	}
	return release, err
}

// executeOnRawDriver invokes operation through the source's lease capability
// or database/sql.Raw while handle.mu is held. It does not acquire handle.mu
// itself; callers must hold it. The lease capability serializes against TTC
// work and rejects calls after the owning connection is returned to the pool.
//
// Parameters:
//   - ctx: context for the operation.
//   - operation: raw-driver operation performed during the callback.
//
// Returns:
//   - error: Raw callback, bridge-capability, or operation error.

func (handle *lobHandle) executeOnRawDriver(ctx context.Context, operation func(lobHandleDriver) error) error {
	if handle.sessionDriver != nil {
		err := operation(handle.sessionDriver)
		if handle.ownerSessionLost(err) {
			handle.invalidateOwnerLocked()
		}
		return err
	}
	if handle.conn == nil {
		return common.NewOracleError(oracleErrors.LobValueClosed, nil, "typed LOB connection")
	}
	callbackEntered := false
	err := handle.conn.Raw(func(raw any) error {
		callbackEntered = true
		driver, ok := raw.(lobHandleDriver)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "typed LOB")
		}
		return operation(driver)
	})
	if handle.ownerSessionLost(err) || (!callbackEntered && err != nil) {
		handle.invalidateOwnerLocked()
	}
	return err
}

// ownerSessionLost reports errors proving that a locator's owning physical
// connection can no longer be used. It must be called with handle.mu held when
// it may invalidate the handle.
func (handle *lobHandle) ownerSessionLost(err error) bool {
	if err == nil || errors.Is(err, sql.ErrConnDone) || errors.Is(err, driver.ErrBadConn) {
		return err != nil
	}
	var sqlErr oracleErrors.SQLError
	return errors.As(err, &sqlErr) && sqlErr.ErrorCode() == string(oracleErrors.LobValueInvalidated)
}

// invalidateOwnerLocked retires a handle whose owning physical session is no
// longer usable. handle.mu must be held.
func (handle *lobHandle) invalidateOwnerLocked() {
	handle.ownerLost = true
	handle.invalidateLocked()
}

// stateErrorLocked reports whether dlob can accept another operation. It must
// be called with handle.mu held. Free is intentionally handled separately because it
// must remain available after local Close to release a temporary locator.
//
// Returns:
//   - error: lifecycle error, including LobValueClosed or LobValueInvalidated.
func (handle *lobHandle) stateErrorLocked() error {
	if handle.freed {
		return common.NewOracleError(oracleErrors.LobValueClosed, nil, "after Free")
	}
	if handle.invalidated {
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "LOB handle state cannot be trusted")
	}
	if handle.closed || (handle.conn == nil && handle.sessionDriver == nil) {
		return common.NewOracleError(oracleErrors.LobValueClosed, nil, "after Close")
	}
	if handle.sessionDriver != nil {
		if err := handle.sessionDriver.ValidateLOBLease(); err != nil {
			if handle.ownerSessionLost(err) {
				handle.invalidateOwnerLocked()
			}
			return err
		}
	}
	return nil
}

// invalidateLocked makes this handle unusable after an operation leaves its
// locator or sequential cursor state uncertain. handle.mu must be held.
func (handle *lobHandle) invalidateLocked() {
	handle.invalidated = true
	handle.pending = nil
	handle.utf8Carry = nil
}

// markFreedLocked records terminal cleanup and drops session and locator
// references. The freed bit remains so later Free calls are idempotent.
// handle.mu must be held.
func (handle *lobHandle) markFreedLocked() {
	handle.freed = true
	handle.closed = true
	handle.conn = nil
	handle.sessionDriver = nil
	handle.locator = nil
	handle.offset = 0
	handle.fetchOffset = 0
	handle.pending = nil
	handle.readComplete = false
	handle.utf8Carry = nil
	handle.serverOpenState = persistentOpenClosed
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
func (handle *lobHandle) Read(dst []byte) (int, error) {
	return handle.ReadContext(context.Background(), dst)
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
func (handle *lobHandle) ReadContext(ctx context.Context, dst []byte) (int, error) {
	// Keep the cursor, pending bytes, and one raw-driver exchange under the
	// same lock so concurrent calls cannot reorder or duplicate reads.
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return 0, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return 0, err
	}
	if len(dst) == 0 {
		return 0, nil
	}
	// A previous refill may contain more UTF-8 bytes than the caller requested.
	// Serve those bytes before asking Oracle for another logical-unit range.
	if len(handle.pending) != 0 {
		n := copy(dst, handle.pending)
		handle.pending = handle.pending[n:]
		if err := handle.advanceCallerOffsetLocked(dst[:n]); err != nil {
			handle.invalidated = true
			handle.pending = nil
			return n, err
		}
		return n, nil
	}
	if handle.readComplete {
		return 0, io.EOF
	}
	readSize := internallob.DefaultBlobLobChunkBytes
	if handle.kind != BLOB {
		// dst is measured in UTF-8 bytes, whereas Oracle character LOB reads
		// are measured in UTF-16 units. Refill a bounded character chunk and
		// retain any excess UTF-8 bytes in pending, so a 1-3 byte caller buffer
		// cannot make the TTC request end halfway through a surrogate pair.
		readSize = internallob.DefaultCharacterLobChunkChars
	}
	var data []byte
	for len(data) == 0 {
		read, logical, err := handle.readLocked(ctx, readSize)
		if err != nil {
			return 0, err
		}
		if logical == 0 {
			return 0, io.EOF
		}
		fetchOffset := handle.fetchOffset
		if fetchOffset == 0 {
			fetchOffset = handle.offset
		}
		handle.fetchOffset = fetchOffset + logical
		data = read
	}
	n := copy(dst, data)
	handle.pending = append(handle.pending[:0], data[n:]...)
	if err := handle.advanceCallerOffsetLocked(dst[:n]); err != nil {
		handle.invalidated = true
		handle.pending = nil
		return n, err
	}
	return n, nil
}

// ReadAt reads BLOB bytes from a zero-based byte offset without changing the
// sequential cursor. It follows io.ReaderAt short-read behavior.
func (handle *lobHandle) ReadAt(dst []byte, offset int64) (int, error) {
	return handle.ReadAtContext(context.Background(), dst, offset)
}

// ReadAtContext reads BLOB bytes from a zero-based byte offset using ctx. It
// reads directly from the owning locator and leaves sequential read-ahead and
// cursor state untouched.
func (handle *lobHandle) ReadAtContext(ctx context.Context, dst []byte, offset int64) (int, error) {
	if handle.kind != BLOB {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read-at", "LOB", "BLOB required")
	}
	if _, err := oneBasedLOBPosition(offset, 0); err != nil {
		return 0, err
	}

	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return 0, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(dst) == 0 {
		return 0, nil
	}

	total := 0
	err = handle.executeOnRawDriver(ctx, func(driver lobHandleDriver) error {
		length, err := driver.LobLength(ctx, uint8(handle.kind), handle.locator)
		if err != nil {
			return err
		}
		zeroBased := uint64(offset)
		if zeroBased >= length {
			return nil
		}

		requestLength := uint64(len(dst))
		if available := length - zeroBased; requestLength > available {
			requestLength = available
		}
		for uint64(total) < requestLength {
			if err := ctx.Err(); err != nil {
				return err
			}
			position, err := oneBasedLOBPosition(offset, uint64(total))
			if err != nil {
				return err
			}
			amount := requestLength - uint64(total)
			if amount > internallob.DefaultBlobLobChunkBytes {
				amount = internallob.DefaultBlobLobChunkBytes
			}
			data, logical, err := driver.LobRead(ctx, uint8(handle.kind), handle.locator, position, amount)
			if err != nil {
				return err
			}
			if logical > amount || logical != uint64(len(data)) {
				return invalidLOBReadResponse("BLOB read returned inconsistent bytes and logical length")
			}
			if logical == 0 {
				break
			}
			total += copy(dst[total:], data)
			if logical < amount {
				break
			}
		}
		return nil
	})
	if err != nil {
		return total, err
	}
	if total < len(dst) {
		return total, io.EOF
	}
	return total, nil
}

// ReadTextAt reads complete CLOB/NCLOB characters from a zero-based Oracle
// logical-unit offset. It returns UTF-8 bytes and the corresponding UTF-16
// units without changing sequential read state.
func (handle *lobHandle) ReadTextAt(dst []byte, offset int64) (int, int64, error) {
	return handle.ReadTextAtContext(context.Background(), dst, offset)
}

// ReadTextAtContext reads complete CLOB/NCLOB characters from a zero-based
// Oracle logical-unit offset using ctx. If the next complete character cannot
// fit while dst still has space, it returns io.ErrShortBuffer and never copies
// part of that character. If dst is exactly filled, it returns nil. The
// returned units count matches the copied UTF-8 prefix.
func (handle *lobHandle) ReadTextAtContext(ctx context.Context, dst []byte, offset int64) (int, int64, error) {
	if handle.kind != CLOB && handle.kind != NCLOB {
		return 0, 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read-text-at", "LOB", "CLOB or NCLOB required")
	}
	if _, err := oneBasedLOBPosition(offset, 0); err != nil {
		return 0, 0, err
	}

	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return 0, 0, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return 0, 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if len(dst) == 0 {
		return 0, 0, nil
	}

	totalBytes := 0
	var totalUnits uint64
	var availableUnits uint64
	eof := false
	err = handle.executeOnRawDriver(ctx, func(driver lobHandleDriver) error {
		length, err := driver.LobLength(ctx, uint8(handle.kind), handle.locator)
		if err != nil {
			return err
		}
		zeroBased := uint64(offset)
		if zeroBased >= length {
			eof = true
			return nil
		}
		availableUnits = length - zeroBased

		for totalBytes < len(dst) && totalUnits < availableUnits {
			if err := ctx.Err(); err != nil {
				return err
			}
			remainingBytes := len(dst) - totalBytes
			amount := uint64(remainingBytes)
			if amount < 2 {
				amount = 2
			}
			if amount > internallob.DefaultCharacterLobChunkChars {
				amount = internallob.DefaultCharacterLobChunkChars
			}
			remainingUnits := availableUnits - totalUnits
			if amount > remainingUnits {
				amount = remainingUnits
			}
			position, err := oneBasedLOBPosition(offset, totalUnits)
			if err != nil {
				return err
			}
			data, logical, err := driver.LobRead(ctx, uint8(handle.kind), handle.locator, position, amount)
			if err != nil {
				return err
			}
			dataUnits, err := utf16UnitsForLOBText(data)
			if err != nil {
				return err
			}
			if logical != dataUnits || logical > remainingUnits || (logical > amount && (logical != amount+1 || !endsWithSupplementaryCharacter(data))) {
				return invalidLOBReadResponse("CLOB/NCLOB read returned inconsistent text and logical length")
			}
			if logical == 0 {
				if len(data) != 0 {
					return invalidLOBReadResponse("CLOB/NCLOB read returned text with zero logical length")
				}
				eof = true
				return nil
			}

			for len(data) != 0 {
				r, size := utf8.DecodeRune(data)
				if size > len(dst)-totalBytes {
					return io.ErrShortBuffer
				}
				totalBytes += copy(dst[totalBytes:], data[:size])
				if r > 0xFFFF {
					totalUnits += 2
				} else {
					totalUnits++
				}
				data = data[size:]
				if totalBytes == len(dst) {
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		return totalBytes, int64(totalUnits), err
	}
	if totalBytes < len(dst) && (eof || totalUnits >= availableUnits) {
		return totalBytes, int64(totalUnits), io.EOF
	}
	return totalBytes, int64(totalUnits), nil
}

func oneBasedLOBPosition(offset int64, advance uint64) (uint64, error) {
	if offset < 0 {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read-at", "LOB", "negative offset")
	}
	position := uint64(offset) + 1
	if advance > ^uint64(0)-position {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read-at", "LOB", "offset overflow")
	}
	return position + advance, nil
}

func utf16UnitsForLOBText(data []byte) (uint64, error) {
	if !utf8.Valid(data) {
		return 0, invalidLOBReadResponse("CLOB/NCLOB read returned invalid UTF-8")
	}
	var units uint64
	for len(data) != 0 {
		r, size := utf8.DecodeRune(data)
		units++
		if r > 0xFFFF {
			units++
		}
		data = data[size:]
	}
	return units, nil
}

func endsWithSupplementaryCharacter(data []byte) bool {
	if !utf8.Valid(data) || len(data) == 0 {
		return false
	}
	r, _ := utf8.DecodeLastRune(data)
	return r > 0xFFFF
}

func invalidLOBReadResponse(detail string) error {
	return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read", "LOB", detail)
}

// hasUnreadDataContext checks whether another read can return data without
// advancing the caller-visible cursor. If it fetches data from Oracle, it
// retains that read-ahead in pending for the next ReadContext call. handle.mu
// protects both the cursor and this probe.
func (handle *lobHandle) hasUnreadDataContext(ctx context.Context) (bool, error) {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return false, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return false, err
	}
	if len(handle.pending) != 0 {
		return true, nil
	}
	if handle.readComplete {
		return false, nil
	}

	readSize := internallob.DefaultBlobLobChunkBytes
	if handle.kind != BLOB {
		readSize = internallob.DefaultCharacterLobChunkChars
	}
	for {
		data, logical, err := handle.readLocked(ctx, readSize)
		if err != nil {
			return false, err
		}
		if logical == 0 {
			if len(data) != 0 {
				return false, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read", "LOB", "data returned with zero logical length")
			}
			return false, nil
		}
		fetchOffset := handle.fetchOffset
		if fetchOffset == 0 {
			fetchOffset = handle.offset
		}
		handle.fetchOffset = fetchOffset + logical
		if len(data) != 0 {
			handle.pending = append(handle.pending[:0], data...)
			return true, nil
		}
	}
}

// readLocked performs one bounded locator read while handle.mu is held. The caller
// owns locking so the returned data and logical amount can be applied to the
// fetch position without another lobHandle operation interleaving.
//
// Parameters:
//   - ctx: context for the read RPC.
//   - size: maximum logical amount to read.
//
// Returns:
//   - []byte: BLOB bytes or UTF-8 CLOB/NCLOB data.
//   - uint64: logical units consumed from the current fetch offset.
//   - error: nil on success.
func (handle *lobHandle) readLocked(ctx context.Context, size int) ([]byte, uint64, error) {
	var data []byte
	var logical uint64
	fetchOffset := handle.fetchOffset
	if fetchOffset == 0 {
		fetchOffset = handle.offset
	}
	err := handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error {
		var err error
		data, logical, err = d.LobRead(ctx, uint8(handle.kind), handle.locator, fetchOffset, uint64(size))
		return err
	})
	return data, logical, err
}

// advanceCallerOffsetLocked accounts for bytes returned to the caller. BLOB
// positions are bytes. Character-LOB positions advance by UTF-16 units only
// when a complete UTF-8 scalar has been returned; utf8Carry remembers a scalar
// split across Read calls. handle.mu must be held.
func (handle *lobHandle) advanceCallerOffsetLocked(data []byte) error {
	if handle.kind == BLOB {
		handle.offset += uint64(len(data))
		return nil
	}

	combined := data
	if len(handle.utf8Carry) != 0 {
		combined = make([]byte, 0, len(handle.utf8Carry)+len(data))
		combined = append(combined, handle.utf8Carry...)
		combined = append(combined, data...)
	}
	handle.utf8Carry = nil
	for len(combined) != 0 {
		if !utf8.FullRune(combined) {
			handle.utf8Carry = append(handle.utf8Carry[:0], combined...)
			return nil
		}
		r, size := utf8.DecodeRune(combined)
		if r == utf8.RuneError && size == 1 {
			return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read", "LOB", "invalid UTF-8")
		}
		units := uint64(1)
		if r > 0xFFFF {
			units = 2
		}
		handle.offset += units
		combined = combined[size:]
	}
	return nil
}

// Write writes at the caller-visible logical offset and advances it by the
// data the server accepted. Large inputs are split into bounded requests.
// CLOB and NCLOB input must be valid UTF-8; text request boundaries preserve
// complete characters. It fails if preceding reads stopped inside a UTF-8
// character. The returned count includes the acknowledged prefix if a later
// request fails. Use WriteContext when cancellation or a deadline is required.
//
// Parameters:
//   - data: BLOB bytes or UTF-8 CLOB/NCLOB text.
//
// Returns:
//   - int: bytes accepted from data.
//   - error: validation or operation error.
func (handle *lobHandle) Write(data []byte) (int, error) {
	return handle.WriteContext(context.Background(), data)
}

// WriteContext writes at the caller-visible logical offset using ctx and
// advances it by the data the server accepted. Large inputs are split into
// bounded requests; CLOB/NCLOB requests end at UTF-8 character boundaries. It
// discards read-ahead after an acknowledged write. For CLOB/NCLOB, it returns
// InvalidLOBBuffer if preceding reads stopped inside a UTF-8 character, since
// that position has no Oracle character offset. It returns io.ErrShortWrite
// when Oracle acknowledges fewer than the requested logical units,
// InvalidLOBBuffer for malformed input or an invalid acknowledgement, and
// otherwise propagates ctx and Oracle errors. If a later request fails, the
// returned byte count includes the acknowledged prefix. An error after a
// request is attempted invalidates this handle because the server may have
// applied that request despite the error.
//
// Parameters:
//   - ctx: context for the write RPC.
//   - data: BLOB bytes or UTF-8 CLOB/NCLOB text.
//
// Returns:
//   - int: bytes accepted from data.
//   - error: validation or operation error.
func (handle *lobHandle) WriteContext(ctx context.Context, data []byte) (int, error) {
	return handle.writeChunksContext(ctx, len(data), func(start int, kind Kind) ([]byte, int) {
		if kind == BLOB {
			end := len(data)
			if remaining := len(data) - start; remaining > internallob.DefaultBlobLobChunkBytes {
				end = start + internallob.DefaultBlobLobChunkBytes
			}
			return data[start:end], end
		}

		end := start
		for characters := 0; end < len(data) && characters < internallob.DefaultCharacterLobChunkChars; characters++ {
			_, size := utf8.DecodeRune(data[end:])
			end += size
		}
		return data[start:end], end
	}, func(kind Kind) error {
		if kind != BLOB && !utf8.Valid(data) {
			return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "invalid UTF-8")
		}
		return nil
	})
}

// WriteStringContext writes UTF-8 text at the caller-visible logical offset
// using ctx. It is the string-input counterpart to WriteContext for CLOB and
// NCLOB values. The text is split into bounded requests without splitting UTF-8
// characters. It must only be called for a CLOB or NCLOB handle.
func (handle *lobHandle) WriteStringContext(ctx context.Context, text string) (int, error) {
	return handle.writeChunksContext(ctx, len(text), func(start int, _ Kind) ([]byte, int) {
		end := start
		for characters := 0; end < len(text) && characters < internallob.DefaultCharacterLobChunkChars; characters++ {
			_, size := utf8.DecodeRuneInString(text[end:])
			end += size
		}
		return []byte(text[start:end]), end
	}, func(kind Kind) error {
		if kind != CLOB && kind != NCLOB {
			return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "WriteString requires CLOB or NCLOB")
		}
		if !utf8.ValidString(text) {
			return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "invalid UTF-8")
		}
		return nil
	})
}

// writeChunksContext performs a sequential write while holding the handle
// lock. nextChunk returns the bounded, valid UTF-8 request starting at the
// supplied input byte offset and the next input byte offset. validate runs
// before any request is sent.
func (handle *lobHandle) writeChunksContext(
	ctx context.Context,
	inputBytes int,
	nextChunk func(int, Kind) ([]byte, int),
	validate func(Kind) error,
) (int, error) {
	// Serialize writes with reads because both advance the same logical offset.
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return 0, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return 0, err
	}
	if inputBytes == 0 {
		return 0, nil
	}
	if handle.kind != BLOB && len(handle.utf8Carry) != 0 {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "read cursor is inside a UTF-8 character")
	}
	if err := validate(handle.kind); err != nil {
		return 0, err
	}

	var totalAccepted int
	for totalAccepted < inputBytes {
		// A cancellation between acknowledged chunks leaves a known cursor and
		// returns the accepted prefix without invalidating the handle.
		if err := ctx.Err(); err != nil {
			return totalAccepted, err
		}
		chunk, next := nextChunk(totalAccepted, handle.kind)
		if len(chunk) == 0 || next <= totalAccepted || next > inputBytes {
			return totalAccepted, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write", "LOB", "invalid internal write chunk")
		}

		var logical uint64
		operationAttempted := false
		err = handle.executeOnRawDriver(ctx, func(driver lobHandleDriver) error {
			operationAttempted = true
			var err error
			logical, err = driver.LobWrite(ctx, uint8(handle.kind), handle.locator, handle.offset, chunk)
			return err
		})
		if err != nil {
			if operationAttempted {
				// An operation error does not prove that Oracle left the LOB unchanged.
				// Retire this handle so later reads cannot serve stale read-ahead.
				handle.invalidateLocked()
			}
			return totalAccepted, err
		}
		accepted, acknowledgeErr := acceptedWriteBytes(handle.kind, chunk, logical)
		if acknowledgeErr != nil && !errors.Is(acknowledgeErr, io.ErrShortWrite) {
			// The response was consumed but its amount cannot map to a valid BLOB
			// byte count or UTF-8 character boundary. Do not commit an untrustworthy
			// cursor.
			handle.invalidateLocked()
			return totalAccepted + accepted, acknowledgeErr
		}

		// A complete or short acknowledgement is a valid, fully-delimited server
		// result. Advance to the acknowledged logical position so callers may
		// retry the remaining bytes. The old read-ahead reflects pre-write data.
		handle.offset += logical
		handle.pending = nil
		handle.readComplete = false
		handle.fetchOffset = handle.offset
		totalAccepted += accepted
		if acknowledgeErr != nil {
			return totalAccepted, acknowledgeErr
		}
	}
	return totalAccepted, nil
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
func (handle *lobHandle) WriteTo(writer io.Writer) (int64, error) {
	if writer == nil {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write-to", "LOB", "nil writer")
	}
	buffer := make([]byte, internallob.DefaultBlobLobChunkBytes)
	var total int64
	for {
		n, readErr := handle.Read(buffer)
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

// acceptedWriteBytes translates a server acknowledgement in Oracle logical
// units into an io.Writer byte count. Oracle must acknowledge whole Unicode
// scalar values for CLOB/NCLOB writes; any partial character unit is a protocol
// error because it cannot be represented by a valid UTF-8 prefix.
func acceptedWriteBytes(kind Kind, data []byte, logical uint64) (int, error) {
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

// Size returns the current server length: bytes for BLOB and UTF-16 code units
// for CLOB and NCLOB. It performs a locator RPC and honors ctx.
//
// Parameters:
//   - ctx: context for the length RPC.
//
// Returns:
//   - int64: logical LOB length.
//   - error: locator RPC or lifecycle error.
func (handle *lobHandle) Size(ctx context.Context) (int64, error) {
	return handle.uintOperation(ctx, "size", func(d lobHandleDriver) (uint64, error) {
		return d.LobLength(ctx, uint8(handle.kind), handle.locator)
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
func (handle *lobHandle) ChunkSize(ctx context.Context) (int64, error) {
	return handle.uintOperation(ctx, "chunk-size", func(d lobHandleDriver) (uint64, error) {
		return d.LobChunkSize(ctx, uint8(handle.kind), handle.locator)
	})
}

// Trim changes the server LOB length in the units reported by Size. The LOB
// must be writable. A negative length returns InvalidLOBBuffer. On success,
// Trim(0) resets the sequential cursor to the beginning; a nonzero trim
// preserves the cursor unless it is beyond the new end, when it is clamped to
// the new EOF position. For CLOB/NCLOB, a nonzero trim is rejected while a
// read has returned only part of a UTF-8 character; finish reading that
// character before trimming. Read-ahead is discarded after a successful trim.
// An error after the trim operation is attempted invalidates this handle
// because the server may have applied the trim despite the error.
//
// Parameters:
//   - ctx: context for the trim RPC.
//   - length: target logical length.
//
// Returns:
//   - error: nil on success.
func (handle *lobHandle) Trim(ctx context.Context, length int64) error {
	if length < 0 {
		return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "trim", "LOB", "negative length")
	}
	// A successful trim changes the server contents, so prefetched bytes from
	// before the trim can no longer be returned to the caller.
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return err
	}
	// A pre-canceled trim has not reached Oracle, so leave the handle and its
	// buffered state untouched.
	if err := ctx.Err(); err != nil {
		return err
	}
	if length != 0 && handle.kind != BLOB && len(handle.utf8Carry) != 0 {
		return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "trim", "LOB", "read cursor is inside a UTF-8 character")
	}
	operationAttempted := false
	err = handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error {
		operationAttempted = true
		_, err := d.LobTrim(ctx, uint8(handle.kind), handle.locator, uint64(length))
		return err
	})
	if err != nil {
		if operationAttempted {
			// A failed trim can have an uncertain server outcome. Discard cached
			// bytes and reject later operations on this handle.
			handle.invalidateLocked()
		}
	} else {
		handle.pending = nil
		handle.utf8Carry = nil
		// Truncating to zero is the documented replace-content path, so restart
		// at the first logical unit. Other trims preserve the cursor unless it is
		// now past the end, in which case clamp it to the new EOF position.
		if length == 0 {
			handle.offset = 1
		} else if end := uint64(length) + 1; handle.offset > end {
			handle.offset = end
		}
		handle.fetchOffset = handle.offset
		// Trim(0) establishes a known empty LOB. Other trims can leave content
		// beyond the row prefix, so the next read must consult Oracle.
		handle.readComplete = length == 0
	}
	return err
}

// Open opens a persistent LOB server-side with the selected mode. For a
// temporary LOB it updates the locator's local open/access flags and sends no
// TTC request. It returns true only if it changed the locator state. The
// caller should call CloseServer before Free when a persistent Open completed
// successfully. If an attempted Open returns an error, the server state is
// unknown until IsOpen successfully reconciles it.
//
// Parameters:
//   - ctx: context for the open RPC.
//   - mode: ReadOnly or ReadWrite.
//
// Returns:
//   - bool: true when this call changed server state.
//   - error: validation, RPC, or lifecycle error.
func (handle *lobHandle) Open(ctx context.Context, mode OpenMode) (bool, error) {
	if mode != ReadOnly && mode != ReadWrite {
		return false, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open", "LOB", "invalid mode")
	}
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return false, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return false, err
	}
	if !handle.temporary && handle.serverOpenState == persistentOpenUnknown {
		return false, lobOpenStateUnknownError()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var opened bool
	var locator []byte
	operationAttempted := false
	err = handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error {
		operationAttempted = true
		var err error
		opened, locator, err = d.LobOpen(ctx, uint8(handle.kind), handle.locator, uint8(mode))
		return err
	})
	if err != nil {
		if operationAttempted && !handle.temporary {
			// The server may have applied Open before the error reached this
			// handle. Do not preserve an assumption based on the pre-call state.
			handle.serverOpenState = persistentOpenUnknown
		}
		return false, err
	}
	// The bridge returns a fresh locator because temporary opens modify its
	// local flags; persistent opens return the server-updated locator copy.
	handle.locator = locator
	if opened && !handle.temporary {
		handle.serverOpenState = persistentOpenOpen
	}
	return opened, err
}

// Close closes the local Go handle and discards buffered data. It is idempotent
// and does not send Oracle's server-side close or free a temporary LOB. For a
// persistent LOB opened with Open, Close returns LobOpen until CloseServer
// succeeds, leaving the handle usable for the required server transition. If
// the persistent open state is unknown, it returns LobOpenStateUnknown; call
// IsOpenContext to reconcile it first.
//
// Returns:
//   - error: nil on success, LobOpen when an explicit persistent Open still
//     requires CloseServer, or LobOpenStateUnknown until IsOpenContext
//     reconciles the state.
func (handle *lobHandle) Close() error {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.ownerLost {
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "LOB owner session is no longer usable")
	}
	if handle.serverOpenState == persistentOpenUnknown {
		return lobOpenStateUnknownError()
	}
	if handle.serverOpenState == persistentOpenOpen {
		return common.NewOracleError(oracleErrors.LobOpen, nil)
	}
	// Close only ends the local handle. Explicit persistent server state must be
	// closed through CloseServer before this transition is allowed.
	handle.closed = true
	handle.pending = nil
	handle.utf8Carry = nil
	return nil
}

// CloseServer closes a persistent LOB on the server after Open. For a
// temporary LOB it clears the local open/access flags and sends no TTC
// request. It leaves the Go handle usable for later operations. If an
// attempted persistent close returns an error, the server state becomes
// unknown; call IsOpenContext to reconcile it before retrying or releasing the
// local handle.
//
// Parameters:
//   - ctx: context for the server-close RPC.
//
// Returns:
//   - error: RPC or lifecycle error, including LobOpenStateUnknown when a
//     previous transition must be reconciled first.
func (handle *lobHandle) CloseServer(ctx context.Context) error {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return err
	}
	if !handle.temporary && handle.serverOpenState == persistentOpenUnknown {
		return lobOpenStateUnknownError()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var locator []byte
	operationAttempted := false
	err = handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error {
		operationAttempted = true
		var err error
		locator, err = d.LobClose(ctx, uint8(handle.kind), handle.locator)
		return err
	})
	if err != nil {
		if operationAttempted && !handle.temporary {
			// The server may have applied CloseServer before returning an error.
			handle.serverOpenState = persistentOpenUnknown
		}
		return err
	}
	handle.locator = locator
	handle.serverOpenState = persistentOpenClosed
	return err
}

// IsOpen reports the current open state. Persistent locators query Oracle and
// temporary locators read their local open flag; temporary checks send no TTC
// request. A successful persistent query also reconciles the handle's cached
// open state. A failed query leaves an unknown state unreconciled. The
// operation honors ctx.
//
// Parameters:
//   - ctx: context for the state RPC.
//
// Returns:
//   - bool: current server-side open state.
//   - error: RPC or lifecycle error.
func (handle *lobHandle) IsOpen(ctx context.Context) (bool, error) {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return false, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return false, err
	}
	var open bool
	err = handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error {
		var err error
		open, err = d.LobIsOpen(ctx, uint8(handle.kind), handle.locator)
		return err
	})
	if err == nil && !handle.temporary {
		if open {
			handle.serverOpenState = persistentOpenOpen
		} else {
			handle.serverOpenState = persistentOpenClosed
		}
	}
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
//     requires CloseServer, LobOpenStateUnknown until IsOpenContext reconciles
//     the state, or a context/temporary-free error.
func (handle *lobHandle) Free(ctx context.Context) error {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.freed {
		return nil
	}
	if handle.ownerLost {
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "LOB owner session is no longer usable")
	}
	if handle.serverOpenState == persistentOpenUnknown {
		return lobOpenStateUnknownError()
	}
	if handle.serverOpenState == persistentOpenOpen {
		return common.NewOracleError(oracleErrors.LobOpen, nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !handle.temporary {
		// Persistent locators are owned by Oracle storage; Free only releases
		// this Go handle after any required server close has completed.
		handle.markFreedLocked()
		return nil
	}
	// Temporary storage is released through the session registry. The handle
	// becomes terminal only after the registry accepts the release.
	err := handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error { return d.LobFree(ctx, handle.locator) })
	if err == nil {
		handle.markFreedLocked()
	}
	return err
}

// Kind identifies this LOB's Oracle family and never performs a locator RPC.
//
// Returns:
//   - Kind: BLOB, CLOB, NCLOB, or Unknown.
func (handle *lobHandle) Kind() Kind { return handle.kind }

// Valid reports whether this handle still represents an allocated LOB value.
// It becomes false after a successful Free.
func (handle *lobHandle) Valid() bool {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	return !handle.freed
}

// IsTemporary reports whether this LOB is backed by an Oracle temporary
// locator. It is a local property and never performs a locator RPC. The value
// remains the same after the handle is closed or freed.
func (handle *lobHandle) IsTemporary() bool { return handle.temporary }

// lobOpenStateUnknownError tells callers to query IsOpenContext before
// repeating an open-state transition or releasing the local handle.
func lobOpenStateUnknownError() error {
	return common.NewOracleError(oracleErrors.LobOpenStateUnknown, nil)
}

// uintOperation invokes one scalar locator operation while holding handle.mu and
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
func (handle *lobHandle) uintOperation(ctx context.Context, operationName string, operation func(lobHandleDriver) (uint64, error)) (int64, error) {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	releaseLease, err := handle.acquireSessionLease()
	if err != nil {
		return 0, err
	}
	defer releaseLease()
	if err := handle.stateErrorLocked(); err != nil {
		return 0, err
	}
	var n uint64
	err = handle.executeOnRawDriver(ctx, func(d lobHandleDriver) error { var err error; n, err = operation(d); return err })
	if err != nil {
		return 0, err
	}
	if n > uint64(^uint64(0)>>1) {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, operationName, "LOB", "exceeds int64")
	}
	return int64(n), nil
}
