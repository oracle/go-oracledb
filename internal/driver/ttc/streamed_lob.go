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
	"context"
	"errors"
	"io"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// streamedLob is the private buffered read and lifecycle state for one
// locator-backed LOB decoded from a query row. It owns a private TTC locator
// copy and retains stable row-owned prefix storage, but it does not own the
// physical session or query lifetime. It is usable only while its owning Rows
// and Statement remain open. Its mutex serializes Read, Size, Close, and owner
// invalidation, including complete TTC refills, so a locator cannot be freed
// or invalidated midway through an operation.
//
// streamedLob deliberately owns no public API policy. oracle/lob.LOB exposes
// the supported application surface; blobExecutor and clobExecutor perform the
// type-specific TTC exchanges. This type coordinates their result streaming
// with row lifetime, buffered data, and locator ownership cleanup.
type streamedLob struct {
	// mu serializes state changes and the complete duration of one locator RPC.
	mu sync.Mutex

	// owner controls query lifetime, validity, operation context, and the
	// ownership registry that keeps this value alive while it can be read.
	owner *ttcRows

	// kind identifies the LOB family and selects the protocol dispatch and
	// character-unit semantics. It is fixed when the value is constructed.
	kind internallob.Kind
	// manager owns kind dispatch and unsafe-session policy. It does not own this
	// value's lifecycle; Rows supplies operation admission and cursor lifetime.
	manager *lobManager

	// locator owns an independent opaque byte copy and the mutable next 1-based
	// server offset used by the next locator operation.
	locator *locator

	// totalLength caches the logical size in the same units used by locator
	// offsets: bytes for BLOBs and UTF-16 code units for CLOBs/NCLOBs.
	totalLength driverCommon.UB8
	// prefix is the inline row payload retained from stable row-owned storage in
	// public byte representation. It is not copied again by this value.
	// For CLOBs and NCLOBs, the encoded row payload is converted to UTF-8 before
	// it is stored here.
	prefix []byte
	// pending contains one converted bounded refill that was larger than the
	// caller's destination. It is consumed before another TTC refill.
	pending []byte
	// pendingHighSurrogate is a UTF-16 high surrogate retained when an inline
	// CLOB/NCLOB prefix ends halfway through a supplementary character. It is
	// joined with the first low surrogate returned by the locator read.
	pendingHighSurrogate uint16
	// nextOffset is the next 1-based Oracle byte or UTF-16-unit position.
	nextOffset driverCommon.UB8
	// readStarted prevents promotion after any data has been returned to the
	// caller, even after all buffered data has been consumed.
	readStarted bool

	// closed records an explicit application Close.
	closed bool
	// invalidated records owner failure/close and takes precedence over closed.
	invalidated bool
}

// newStreamedLob builds a locator-backed source from validated row-owned data.
// The private caller supplies a non-nil Rows owner and validates the row's
// metadata before calling this constructor. It does not perform network I/O;
// the caller registers the result before exposing it.
//
// Parameters:
//   - owner: Rows that controls validity, context, and LOB ownership.
//   - dtype: TTC BLOB or CLOB datatype.
//   - prefix: stable inline row payload to retain or decode.
//   - metadata: locator, length, and character-set metadata from the row.
//
// Returns:
//   - *streamedLob: initialized locator-backed source.
//   - error: invalid source metadata or prefix-decoding failure.
func newStreamedLob(owner *ttcRows, dtype DtyType, prefix driverCommon.B1Array, metadata lobColumnContext) (*streamedLob, error) {
	if len(metadata.lobLocator) == 0 {
		return nil, common.NewOracleError(oracleErrors.InvalidLobSource, nil, "locator metadata")
	}
	// RXD decoding has already copied this locator into row-owned metadata. Keep
	// a separate locator copy because locator state belongs to this value and
	// must not be coupled to row metadata or future locator operations.
	locatorBytes := append(driverCommon.B1Array(nil), metadata.lobLocator...)
	loc := newLocator(locatorBytes, 1)
	// The minimal query API supports persistent table locators only. Temporary
	// and abstract locators require separate physical-session ownership and
	// cleanup semantics, so accepting them would reintroduce the escaped-LOB
	// lifecycle this implementation deliberately avoids.
	if loc.isTemporaryLocator() || loc.isAbstractLocator() {
		return nil, common.NewOracleError(
			oracleErrors.InvalidLobSource,
			nil,
			"temporary or abstract query LOB",
		)
	}
	// Manager construction only wires session-bound executors; the first actual
	// locator operation is performed later by Read, Size, or ChunkSize.
	manager, err := newLobManager(owner.shelf, owner.sessionContext)
	if err != nil {
		return nil, err
	}
	lob := &streamedLob{
		owner:       owner,
		manager:     manager,
		locator:     loc,
		totalLength: metadata.totalLobLength,
		nextOffset:  1,
	}

	switch dtype {
	case DtyBlob:
		// BLOB prefix bytes and locator offsets use the same logical units.
		lob.kind = internallob.BLOB
		// The caller supplies a row-owned copy that is stable for the value's
		// lifetime, so avoid allocating another copy of a potentially large
		// prefetched BLOB.
		lob.prefix = []byte(prefix)
		logical := driverCommon.UB8(len(lob.prefix))
		if err := validateStreamedLobPrefix(logical, lob.totalLength); err != nil {
			return nil, err
		}
		lob.nextOffset += logical
	case DtyClob:
		// CLOB/NCLOB prefix bytes are exposed as UTF-8, while logical length and
		// the next locator offset remain in Oracle character units.
		lob.kind = internallob.CLOB
		if metadata.charsetForm == FormNChar {
			lob.kind = internallob.NCLOB
		}
		clobExecutor := manager.getClobExecutor()
		decodedPrefix, logical, pendingHighSurrogate, err := clobExecutor.decodeReadPayload(
			loc,
			lob.kind == internallob.NCLOB,
			prefix,
			0,
			true,
		)
		if err != nil {
			return nil, err
		}
		if err := validateStreamedLobPrefix(logical, lob.totalLength); err != nil {
			return nil, err
		}
		if pendingHighSurrogate != 0 && logical >= lob.totalLength {
			return nil, common.NewOracleError(
				oracleErrors.InvalidLOBBuffer,
				nil,
				"decode",
				"clob",
				"invalid UTF-16 surrogate pair",
			)
		}
		lob.prefix = decodedPrefix
		lob.pendingHighSurrogate = pendingHighSurrogate
		lob.nextOffset += logical
	default:
		return nil, common.NewOracleError(oracleErrors.InvalidLobSource, nil, "TTC datatype")
	}
	return lob, nil
}

// Kind implements internal/lob.LOBSource.
//
// Returns:
//   - internallob.Kind: BLOB, CLOB, or NCLOB selected at construction.
func (lob *streamedLob) Kind() internallob.Kind {
	return lob.kind
}

// DetachPersistentLocator verifies sessionKey owns this unread persistent locator
// and transfers it out of Rows. Detaching releases Rows ownership.
//
// Returns:
//   - []byte: independent locator-byte copy for a direct LOB handle.
//   - error: lifecycle, session-mismatch, or InvalidLOBBuffer after any read.
func (lob *streamedLob) DetachPersistentLocator(sessionKey any) ([]byte, error) {
	lob.mu.Lock()
	if err := lob.stateErrorLocked(); err != nil {
		lob.mu.Unlock()
		return nil, err
	}
	owner := lob.owner
	if sessionKey == nil || sessionKey != owner.shelf {
		lob.mu.Unlock()
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open-persistent", "LOB", "source belongs to another connection")
	}
	// Promotion is allowed only before any bytes are returned. Prefetched data
	// may still be present at this point, so use the read-history marker rather
	// than the remaining buffer lengths.
	if lob.readStarted {
		lob.mu.Unlock()
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open-persistent", "LOB", "LOB has already been read")
	}
	locatorBytes := append([]byte(nil), lob.locator.locatorBytes...)
	lob.closed = true
	lob.prefix = nil
	lob.pending = nil
	lob.pendingHighSurrogate = 0
	lob.mu.Unlock()
	owner.releaseLob(lob)
	return locatorBytes, nil
}

// Read implements io.Reader. It returns inline row data before issuing bounded
// locator RPCs and releases Rows ownership once the stream reaches EOF.
//
// Parameters:
//   - dst: destination for the next public BLOB or UTF-8 character bytes.
//
// Returns:
//   - int: bytes copied to dst.
//   - error: io.EOF at end of stream or a lifecycle, decode, or RPC error.
func (lob *streamedLob) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	lob.mu.Lock()
	if err := lob.stateErrorLocked(); err != nil {
		lob.mu.Unlock()
		return 0, err
	}
	owner := lob.owner
	// Inline data is already part of the row result and must be returned before
	// any locator refill. A completed prefix may also reach EOF without RPC.
	if n, finished := lob.copyBufferedLocked(dst); n > 0 {
		lob.mu.Unlock()
		if finished {
			owner.releaseLob(lob)
		}
		return n, nil
	}

	request := lob.readChunkSize()
	// totalLength and nextOffset use the same logical unit space for each LOB
	// kind, so every locator refill can be bounded without an end-of-value RPC.
	consumed := lob.nextOffset - 1
	if consumed >= lob.totalLength {
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, io.EOF
	}
	remaining := lob.totalLength - consumed
	if driverCommon.UB8(request) > remaining {
		request = int(remaining)
	}
	// Keep the LOB mutex held through admission and the complete exchange. This
	// serializes this value's offset/buffer state and lets Rows.Close invalidate
	// the value without racing its locator operation.
	ctx, release, err := lob.beginOperation()
	if err != nil {
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, common.NewOracleError(oracleErrors.LobValueInvalidated, err, "Rows owner")
	}
	lob.locator.offset = lob.nextOffset
	payload, logical, readErr := lob.manager.read(
		ctx,
		lob.Kind(),
		lob.locator,
		driverCommon.UB8(request),
		lob.pendingHighSurrogate,
	)
	unsafeStream := readErr != nil && !isCompletedLobResponseError(readErr)
	release()
	if readErr != nil {
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		if unsafeStream {
			owner.invalidateAfterUnsafeLobRPC()
		}
		return 0, readErr
	}
	if err := lob.stateErrorLocked(); err != nil {
		// Rows.Close or session invalidation may win while the RPC is completing;
		// never publish a successful response after that terminal transition.
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, err
	}
	if logical > driverCommon.UB8(request) {
		err := common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"lob",
			"logical amount exceeds request",
		)
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, err
	}
	if len(payload) == 0 || logical == 0 {
		// A refill before totalLength must make progress.
		err := common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"LOB",
			"invalid response",
		)
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, err
	}
	lob.pendingHighSurrogate = 0
	// The executor returns a fresh converted payload. Retain it only when the
	// caller's buffer cannot consume the complete response in this call.
	lob.nextOffset += logical
	lob.pending = payload
	n := copyBuffered(dst, &lob.pending)
	if n > 0 {
		lob.readStarted = true
	}
	finished := lob.finishIfCompleteLocked()
	lob.mu.Unlock()
	if finished {
		owner.releaseLob(lob)
	}
	return n, nil
}

// Size returns the server-reported LOB length. BLOB lengths are bytes; CLOB
// and NCLOB lengths are UTF-16 units. The cached length bounds all reads.
//
// Returns:
//   - int64: logical server length.
//   - error: lifecycle, range, or locator-RPC error.
func (lob *streamedLob) Size() (int64, error) {
	lob.mu.Lock()
	if err := lob.stateErrorLocked(); err != nil {
		lob.mu.Unlock()
		return 0, err
	}
	owner := lob.owner
	if lob.kind == internallob.BLOB {
		// BLOB length is already present in row metadata, so no TTC operation is
		// needed. CLOB and NCLOB lengths require the character-aware executor.
		length, err := checkedLobLength(lob.totalLength)
		lob.mu.Unlock()
		return length, err
	}
	ctx, release, err := lob.beginOperation()
	if err != nil {
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, common.NewOracleError(oracleErrors.LobValueInvalidated, err, "Rows owner")
	}
	var length driverCommon.UB8
	length, err = lob.manager.length(ctx, lob.Kind(), lob.locator)
	unsafeStream := err != nil && !isCompletedLobResponseError(err)
	release()
	if err != nil {
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		if unsafeStream {
			owner.invalidateAfterUnsafeLobRPC()
		}
		return 0, err
	}
	if err := lob.stateErrorLocked(); err != nil {
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, err
	}
	checked, err := checkedLobLength(length)
	lob.mu.Unlock()
	return checked, err
}

// ChunkSize reports the server storage chunk size. It does not change the
// stream's network refill size.
//
// Returns:
//   - int64: server storage chunk size in bytes.
//   - error: lifecycle, range, or locator-RPC error.
func (lob *streamedLob) ChunkSize() (int64, error) {
	lob.mu.Lock()
	if err := lob.stateErrorLocked(); err != nil {
		lob.mu.Unlock()
		return 0, err
	}
	owner := lob.owner
	ctx, release, err := lob.beginOperation()
	if err != nil {
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, common.NewOracleError(oracleErrors.LobValueInvalidated, err, "Rows owner")
	}
	var size driverCommon.UB8
	size, err = lob.manager.chunkSize(ctx, lob.Kind(), lob.locator)
	unsafeStream := err != nil && !isCompletedLobResponseError(err)
	release()
	if err != nil {
		lob.invalidated = true
		lob.mu.Unlock()
		owner.releaseLob(lob)
		if unsafeStream {
			owner.invalidateAfterUnsafeLobRPC()
		}
		return 0, err
	}
	if err := lob.stateErrorLocked(); err != nil {
		lob.mu.Unlock()
		owner.releaseLob(lob)
		return 0, err
	}
	lob.mu.Unlock()
	return checkedLobLength(size)
}

// WriteTo implements io.WriterTo by repeatedly reading into one reusable
// bounded buffer. It leaves the stream at EOF on success.
//
// Parameters:
//   - writer: destination for remaining public LOB bytes.
//
// Returns:
//   - int64: bytes written to writer.
//   - error: writer, read, or lifecycle error.
func (lob *streamedLob) WriteTo(writer io.Writer) (int64, error) {
	if writer == nil {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "write-to", "lob", "nil writer")
	}
	buffer := make([]byte, lob.readChunkSize())
	var total int64
	for {
		n, readErr := lob.Read(buffer)
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
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

// Close releases Rows ownership and discards buffered data.
//
// Returns:
//   - error: always nil.
func (lob *streamedLob) Close() error {
	lob.mu.Lock()
	if lob.closed {
		lob.mu.Unlock()
		return nil
	}
	owner := lob.owner
	// This is local query-value cleanup; it does not send a server-side LOB
	// close operation. Direct persistent LOBs own that separate lifecycle.
	lob.closed = true
	lob.prefix = nil
	lob.pending = nil
	lob.pendingHighSurrogate = 0
	lob.mu.Unlock()
	if owner != nil {
		owner.releaseLob(lob)
	}
	return nil
}

// invalidate prevents future RPCs and drops buffered data.
func (lob *streamedLob) invalidate() {
	lob.mu.Lock()
	lob.invalidated = true
	lob.prefix = nil
	lob.pending = nil
	lob.pendingHighSurrogate = 0
	lob.mu.Unlock()
}

// beginOperation serializes a locator RPC with the physical session.
//
// Returns:
//   - context.Context: exchange-scoped cancelable context.
//   - func(): required release function after the RPC.
//   - error: owner-closure or context error before an RPC begins.
func (lob *streamedLob) beginOperation() (context.Context, func(), error) {
	return lob.owner.beginLobOperation()
}

// readChunkSize returns the protocol-safe application and locator refill size.
//
// Returns:
//   - int: byte chunk for BLOB or character-unit chunk for CLOB and NCLOB.
func (lob *streamedLob) readChunkSize() int {
	if lob.kind == internallob.BLOB {
		return internallob.DefaultBlobLobChunkBytes
	}
	return internallob.DefaultCharacterLobChunkChars
}

// stateErrorLocked reports terminal state without network activity. lob.mu
// must be held.
//
// Returns:
//   - error: lifecycle error, or nil when the source remains usable.
func (lob *streamedLob) stateErrorLocked() error {
	// Owner/session invalidation takes precedence over an explicit local Close:
	// a locator must never report itself as usable after its query or session is
	// gone.
	if lob.invalidated {
		lob.invalidated = true
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "Rows owner")
	}
	if lob.owner.shelf.lobState.isInvalidated() {
		lob.invalidated = true
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "LOB session")
	}
	if lob.owner.isClosed() {
		lob.invalidated = true
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "Rows owner")
	}
	if err := lob.owner.contextErr(); err != nil {
		lob.invalidated = true
		return common.NewOracleError(oracleErrors.LobValueInvalidated, err, "query context")
	}
	if lob.closed {
		return common.NewOracleError(oracleErrors.LobValueClosed, nil, "after Close")
	}
	return nil
}

// finishIfCompleteLocked reports whether all buffered bytes and the known
// logical length are consumed. lob.mu must be held.
//
// Returns:
//   - bool: true when the caller must release Rows ownership.
func (lob *streamedLob) finishIfCompleteLocked() bool {
	if len(lob.prefix) != 0 || len(lob.pending) != 0 {
		return false
	}
	return lob.nextOffset-1 >= lob.totalLength
}

// copyBufferedLocked copies unread prefix or refill data into dst. lob.mu
// must be held.
//
// Parameters:
//   - dst: destination for unread buffered bytes.
//
// Returns:
//   - int: bytes copied from prefix or pending data.
//   - bool: true when copying completed a LOB and releases Rows ownership.
func (lob *streamedLob) copyBufferedLocked(dst []byte) (int, bool) {
	if n := copyBuffered(dst, &lob.prefix); n > 0 {
		lob.readStarted = true
		return n, lob.finishIfCompleteLocked()
	}
	if n := copyBuffered(dst, &lob.pending); n > 0 {
		lob.readStarted = true
		return n, lob.finishIfCompleteLocked()
	}
	return 0, false
}

// validateStreamedLobPrefix verifies that an inline row payload fits within
// the locator's declared logical length. The caller supplies bytes for BLOBs
// or UTF-16 code units for CLOBs and NCLOBs.
//
// Returns:
//   - error: InvalidLOBBuffer when the prefix exceeds the declared length.
func validateStreamedLobPrefix(logical, declared driverCommon.UB8) error {
	if logical > declared {
		return common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"decode",
			"lob",
			"inline prefix exceeds declared length",
		)
	}
	return nil
}

// copyBuffered copies from a buffered segment and removes the bytes returned
// to the caller. It also drops the backing reference once the segment is
// exhausted so completed buffers can be reclaimed promptly.
//
// Parameters:
//   - dst: destination buffer.
//   - source: buffered source slice to update.
//
// Returns:
//   - int: bytes copied.
func copyBuffered(dst []byte, source *[]byte) int {
	if len(*source) == 0 {
		return 0
	}
	n := copy(dst, *source)
	*source = (*source)[n:]
	if len(*source) == 0 {
		*source = nil
	}
	return n
}

// checkedLobLength converts the protocol's unsigned size to the public int64.
//
// Parameters:
//   - length: unsigned protocol size.
//
// Returns:
//   - int64: representable public size.
//   - error: InvalidLOBBuffer when length exceeds int64.
func checkedLobLength(length driverCommon.UB8) (int64, error) {
	const maxInt64 = int64(^uint64(0) >> 1)
	if uint64(length) > uint64(maxInt64) {
		return 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "size", "lob", "length exceeds int64")
	}
	return int64(length), nil
}
