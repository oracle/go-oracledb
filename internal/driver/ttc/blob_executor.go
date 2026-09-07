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
	"log/slog"
	"math"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// blobFormOfUse identifies the form-of-use value used for binary LOB locators.
	// Binary LOBs do not carry character semantics, so the value remains zero.
	blobFormOfUse driverCommon.UB2 = 0

	// blobCharsetIDPlaceholder supplies the non-zero charset value required by
	// the OLOBOPS temporary-create protocol. It makes the charset pointer
	// non-null but does not apply character conversion to BLOB data.
	blobCharsetIDPlaceholder driverCommon.UB2 = 1
)

// blobExecutor orchestrates BLOB operations on top of the shared lobExecutor.
// The executor intentionally keeps the API surface byte-oriented so higher
// layers do not perform redundant conversions when streaming binary payloads.
type blobExecutor struct {
	*lobExecutor
}

// newBlobExecutor constructs a blobExecutor using the supplied message shelf.
//
// Parameters:
//   - shelf: message shelf shared across TTC executors.
//
// Returns:
//   - *blobExecutor: executor backed by the shared lobExecutor.
//
// Errors:
//   - None.
func newBlobExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType]) *blobExecutor {
	return &blobExecutor{lobExecutor: newLobExecutor(shelf)}
}

// open delegates to the shared lobExecutor while preventing unsupported BFILE
// open modes from slipping through the binary LOB path.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: BLOB locator to open.
//   - mode: desired access mode, either lobOpenModeReadOnly or
//     lobOpenModeReadWrite.
//
// Returns:
//   - bool: true when the locator was opened, or false when no state change was
//     required.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Returns UnsupportedLobOperation for the BFILE-only read mode.
//   - Propagates validation, TTC, protocol, and network failures from the
//     shared lobExecutor.
//
// Behaviour:
//   - Temporary and abstract locators are opened by updating their local
//     locator flags.
//   - Persistent locators are opened through an OLOBOPS round trip.
func (b *blobExecutor) open(ctx context.Context, lobLocator *locator, mode lobOpenMode) (bool, error) {
	if mode == bfileOpenModeReadOnly {
		err := common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "BFILE open")
		common.Odl.Error("blobExecutor.open: invalid open mode", "mode", mode, "error", err)
		return false, err
	}

	return b.lobExecutor.open(ctx, lobLocator, mode)
}

// close closes the locator and clears client side flags via the base executor.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: BLOB locator to close.
//
// Returns:
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates locator-state, TTC, protocol, and network failures from the
//     shared lobExecutor.
func (b *blobExecutor) close(ctx context.Context, lobLocator *locator) error {
	return b.lobExecutor.close(ctx, lobLocator)
}

// getChunkSize retrieves the server reported chunk (page) size for the locator.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: BLOB locator whose storage chunk size is requested.
//
// Returns:
//   - driverCommon.UB8: chunk size in bytes reported by the server.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates TTC, protocol, and network failures from the shared
//     lobExecutor.
func (b *blobExecutor) getChunkSize(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	return b.lobExecutor.getChunkSize(ctx, lobLocator)
}

// getLength retrieves the server reported length for the supplied locator.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: BLOB locator whose length is requested.
//
// Returns:
//   - driverCommon.UB8: BLOB length in bytes reported by the server.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates TTC, protocol, and network failures from the shared
//     lobExecutor.
func (b *blobExecutor) getLength(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	return b.lobExecutor.getLength(ctx, lobLocator)
}

// trim truncates or extends the LOB to the provided length.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: BLOB locator whose length should be changed.
//   - newLength: desired BLOB length in bytes.
//
// Returns:
//   - driverCommon.UB8: resulting length in bytes reported by the server.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Returns InvalidLOBBuffer for value-based or read-only locators.
//   - Propagates TTC, protocol, and network failures from the shared
//     lobExecutor.
func (b *blobExecutor) trim(ctx context.Context, lobLocator *locator, newLength driverCommon.UB8) (driverCommon.UB8, error) {
	return b.lobExecutor.trim(ctx, lobLocator, newLength)
}

// isOpen interrogates the locator open state using the base executor helper.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: BLOB locator whose open state is requested.
//
// Returns:
//   - bool: true when the locator is open.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates TTC, protocol, and network failures from the shared
//     lobExecutor.
//
// Behaviour:
//   - Temporary and abstract locator state is read locally.
//   - Persistent locator state is queried through an OLOBOPS round trip.
func (b *blobExecutor) isOpen(ctx context.Context, lobLocator *locator) (bool, error) {
	return b.lobExecutor.isOpen(ctx, lobLocator)
}

// createTemporaryLob provisions a temporary BLOB locator.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - cache: whether Oracle should place temporary BLOB data in the buffer
//     cache.
//   - duration: server duration hint for the temporary BLOB lifecycle.
//
// Returns:
//   - driverCommon.B1Array: locator bytes for the newly created temporary BLOB.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates TTC, protocol, and network failures from the shared
//     lobExecutor.
//
// Behaviour:
//   - Temporary BLOBs use binary form-of-use and a non-zero charset placeholder
//     required by OLOBOPS. The placeholder does not give binary data character
//     semantics or trigger character conversion.
func (b *blobExecutor) createTemporaryLob(ctx context.Context, cache bool, duration driverCommon.UB4) (driverCommon.B1Array, error) {
	tempSize := kolllTempWithSignature
	def := newLobDefinitionForTemporaryCreate(
		tempSize,
		blobFormOfUse,
		driverCommon.UB8(DtyBlob),
		duration,
		cache,
		blobCharsetIDPlaceholder,
	)

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"blobExecutor.createTemporaryLob: before execute",
			"cache", cache,
			"duration", duration,
			"tempSize", tempSize,
			"operation", def.operation,
		)
	}

	if err := b.lobExecutor.execute(ctx, def); err != nil {
		common.Odl.Error("blobExecutor.createTemporaryLob: execute failed",
			"error", err,
			"operation", def.operation,
		)
		return nil, err
	}

	return def.sourceLocator.locatorBytes, nil
}

// write streams the supplied binary payload into the target locator using the
// shared lobExecutor.write helper.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - lobLocator: writable BLOB locator and byte offset for the operation.
//   - data: raw bytes to write without character-set conversion.
//
// Returns:
//   - driverCommon.UB8: number of bytes reported written by the server.
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Returns InvalidLOBBuffer for value-based or read-only locators.
//   - Propagates TTC, protocol, and network failures from the shared
//     lobExecutor.
//
// Buffer requirements:
//   - The entire data slice is sent in this operation. Its length determines
//     the OLOBOPS byte amount.
func (b *blobExecutor) write(
	ctx context.Context,
	lobLocator *locator,
	data driverCommon.B1Array,
) (driverCommon.UB8, error) {
	return b.lobExecutor.write(ctx, lobLocator, data, driverCommon.UB8(len(data)))
}

// read performs one BLOB locator read into a newly allocated, bounded
// result buffer.
//
// Parameters:
//   - ctx: request context used for the TTC exchange.
//   - lobLocator: locator and 1-based offset identifying the BLOB data to read.
//   - amount: maximum number of bytes to request and destination-buffer capacity.
//
// Returns:
//   - []byte: bytes read from the locator. The slice owns the allocation made
//     for this call; the executor does not retain, pool, or reuse it, so callers
//     may retain or modify it.
//   - driverCommon.UB8: number of bytes in the returned slice.
//   - error: nil on success.
//
// Errors:
//   - Returns InvalidLOBBuffer when amount exceeds math.MaxInt, the largest Go
//     int for the target architecture.
//   - Returns LobExecError when a TTC protocol response exceeds the destination
//     buffer or another underlying read-exchange error occurs.
//
// Returning the allocated destination directly avoids a second allocation and
// copy for every BLOB refill. The lower TTC callback verifies that protocol
// frames cannot exceed the destination buffer.
func (b *blobExecutor) read(ctx context.Context, lobLocator *locator, amount driverCommon.UB8) ([]byte, driverCommon.UB8, error) {
	if amount > driverCommon.UB8(math.MaxInt) {
		return nil, 0, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "read", "blob", "amount overflow")
	}
	buffer := make(driverCommon.B1Array, int(amount))
	read, logical, err := b.lobExecutor.read(ctx, lobLocator, amount, buffer)
	if err != nil {
		return nil, 0, err
	}
	if read != logical {
		return nil, 0, &completedLobResponseError{err: common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"blob",
			"lobAmt mismatch",
		)}
	}
	return buffer[:int(read)], logical, nil
}
