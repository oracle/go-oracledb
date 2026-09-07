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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// The Lob* methods are the private database/sql.Raw bridge used by
// oracle/lob.DirectLOB. They accept only copied locator bytes so a raw driver
// connection never escapes database/sql's callback.

// LobCreate creates a session-duration temporary locator for kind.
//
// Parameters:
//   - ctx: context for the create RPC.
//   - kind: internal BLOB, CLOB, or NCLOB discriminator.
//
// Returns:
//   - []byte: copied locator bytes.
//   - error: nil on success.
func (c *connection) LobCreate(ctx context.Context, kind uint8) ([]byte, error) {
	var result driverCommon.B1Array
	err := c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		var err error
		result, err = manager.createTemporary(ctx, internallob.Kind(kind))
		return err
	})
	return append([]byte(nil), result...), err
}

// LobSessionKey returns the opaque physical-session identity used to reject
// a persistent query locator promoted through a different *sql.Conn.
//
// Returns:
//   - any: identity of the underlying physical session.
func (c *connection) LobSessionKey() any { return c.shelf }

// LobRead performs one bounded read at offset and returns BLOB bytes or UTF-8
// CLOB/NCLOB data.
// Oracle locator reads are character-boundary aligned, so CLOB and NCLOB reads
// do not retain conversion state across calls.
//
// Parameters:
//   - ctx: context for the read RPC.
//   - kind: internal BLOB, CLOB, or NCLOB discriminator.
//   - bytes: copied locator bytes.
//   - offset: 1-based Oracle logical read position.
//   - amount: maximum logical amount to read.
//
// Returns:
//   - []byte: BLOB bytes or UTF-8 CLOB/NCLOB data.
//   - uint64: logical units consumed from offset.
//   - error: nil on success.
func (c *connection) LobRead(ctx context.Context, kind uint8, bytes []byte, offset, amount uint64) ([]byte, uint64, error) {
	loc, err := _lobLocator(bytes, offset)
	if err != nil {
		return nil, 0, err
	}
	var data []byte
	var logical driverCommon.UB8
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		var err error
		data, logical, err = manager.read(ctx, internallob.Kind(kind), loc, driverCommon.UB8(amount), 0)
		return err
	})
	return data, uint64(logical), err
}

// LobWrite writes BLOB bytes or UTF-8 CLOB text at offset.
//
// Parameters:
//   - ctx: context for the write RPC.
//   - kind: internal BLOB, CLOB, or NCLOB discriminator.
//   - bytes: copied locator bytes.
//   - offset: 1-based Oracle logical write position.
//   - data: BLOB bytes or UTF-8 CLOB/NCLOB text.
//
// Returns:
//   - uint64: logical units written.
//   - error: nil on success.
func (c *connection) LobWrite(ctx context.Context, kind uint8, bytes []byte, offset uint64, data []byte) (uint64, error) {
	loc, err := _lobLocator(bytes, offset)
	if err != nil {
		return 0, err
	}
	var written driverCommon.UB8
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		var err error
		written, err = manager.write(ctx, internallob.Kind(kind), loc, data)
		return err
	})
	return uint64(written), err
}

// LobLength returns the logical LOB content length for kind.
//
// Parameters:
//   - ctx: context for the length RPC.
//   - kind: internal BLOB, CLOB, or NCLOB discriminator.
//   - bytes: copied locator bytes.
//
// Returns:
//   - uint64: bytes for BLOBs or UTF-16 code units for CLOBs and NCLOBs.
//   - error: nil on success.
func (c *connection) LobLength(ctx context.Context, kind uint8, bytes []byte) (uint64, error) {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return 0, err
	}
	var length driverCommon.UB8
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		length, err = manager.length(ctx, internallob.Kind(kind), loc)
		return err
	})
	return uint64(length), err
}

// LobChunkSize returns the server storage chunk size for kind.
//
// Parameters:
//   - ctx: context for the chunk-size RPC.
//   - kind: internal BLOB, CLOB, or NCLOB discriminator.
//   - bytes: copied locator bytes.
//
// Returns:
//   - uint64: server storage chunk size in bytes.
//   - error: nil on success.
func (c *connection) LobChunkSize(ctx context.Context, kind uint8, bytes []byte) (uint64, error) {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return 0, err
	}
	var chunkSize driverCommon.UB8
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		chunkSize, err = manager.chunkSize(ctx, internallob.Kind(kind), loc)
		return err
	})
	return uint64(chunkSize), err
}

// LobTrim changes the locator length and returns the server-reported result.
//
// Parameters:
//   - ctx: context for the trim RPC.
//   - kind: internal LOB discriminator.
//   - bytes: copied locator bytes.
//   - length: target logical length.
//
// Returns:
//   - uint64: resulting logical length.
//   - error: nil on success.
func (c *connection) LobTrim(ctx context.Context, kind uint8, bytes []byte, length uint64) (uint64, error) {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return 0, err
	}
	var result driverCommon.UB8
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		result, err = manager.trim(ctx, internallob.Kind(kind), loc, driverCommon.UB8(length))
		return err
	})
	return uint64(result), err
}

// LobOpen opens the locator with the requested TTC access mode.
//
// Parameters:
//   - ctx: context for the open RPC.
//   - kind: internal LOB discriminator.
//   - bytes: copied locator bytes.
//   - mode: TTC read-only or read-write open mode.
//
// Returns:
//   - bool: true when this call changed server state.
//   - []byte: copied locator bytes after any local state transition.
//   - error: nil on success.
func (c *connection) LobOpen(ctx context.Context, kind uint8, bytes []byte, mode uint8) (bool, []byte, error) {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return false, nil, err
	}
	var opened bool
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		opened, err = manager.open(ctx, internallob.Kind(kind), loc, lobOpenMode(mode))
		return err
	})
	return opened, append([]byte(nil), loc.locatorBytes...), err
}

// LobClose sends the TTC server-side close operation for an opened locator.
//
// Parameters:
//   - ctx: context for the close RPC.
//   - kind: internal LOB discriminator.
//   - bytes: copied locator bytes.
//
// Returns:
//   - []byte: copied locator bytes after any local state transition.
//   - error: nil on success.
func (c *connection) LobClose(ctx context.Context, kind uint8, bytes []byte) ([]byte, error) {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return nil, err
	}
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		return manager.close(ctx, internallob.Kind(kind), loc)
	})
	return append([]byte(nil), loc.locatorBytes...), err
}

// LobIsOpen returns the server-side open state for a locator.
//
// Parameters:
//   - ctx: context for the state RPC.
//   - kind: internal LOB discriminator.
//   - bytes: copied locator bytes.
//
// Returns:
//   - bool: current server-side open state.
//   - error: nil on success.
func (c *connection) LobIsOpen(ctx context.Context, kind uint8, bytes []byte) (bool, error) {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return false, err
	}
	var open bool
	err = c._executeLobOperation(ctx, func(ctx context.Context, manager *lobManager) error {
		open, err = manager.isOpen(ctx, internallob.Kind(kind), loc)
		return err
	})
	return open, err
}

// LobFree queues a temporary or abstract locator for release by the next
// ordinary TTC request on the owning session.
//
// Parameters:
//   - ctx: reserved for the directLOBDriver interface; no RPC is performed.
//   - bytes: copied locator bytes.
//
// Returns:
//   - error: nil when the locator was queued and locally invalidated.
func (c *connection) LobFree(_ context.Context, bytes []byte) error {
	loc, err := _lobLocator(bytes, 1)
	if err != nil {
		return err
	}
	manager, err := newLobManager(c.shelf, c.sessCtx)
	if err != nil {
		return err
	}
	return manager.freeLobReference(loc)
}

// _executeLobOperation creates the LOB manager before admitting one cancelable TTC
// exchange on this connection. The callback receives the operation context so
// all manager work remains inside the physical-session admission boundary.
//
// Parameters:
//   - ctx: parent context for the exchange.
//   - operation: manager operation executed while the physical connection is reserved.
//
// Returns:
//   - error: manager construction, lifecycle, cancellation, or operation error.
func (c *connection) _executeLobOperation(ctx context.Context, operation func(context.Context, *lobManager) error) error {
	manager, err := newLobManager(c.shelf, c.sessCtx)
	if err != nil {
		return err
	}
	return c._lobOperation(ctx, func(opCtx context.Context) error {
		return operation(opCtx, manager)
	})
}

// _lobOperation serializes a cancelable TTC LOB exchange on this connection.
//
// Parameters:
//   - ctx: parent context for the exchange.
//   - operation: exchange executed while the physical connection is reserved.
//
// Returns:
//   - error: lifecycle, cancellation, or operation error.
func (c *connection) _lobOperation(ctx context.Context, operation func(context.Context) error) error {
	c.stateMu.RLock()
	available := !c._isClosed && c._isValid
	c.stateMu.RUnlock()
	if !available || c.shelf == nil {
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "direct LOB connection")
	}
	release, err := c.shelf.synchronizer.begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	closed, valid := c.connectionState()
	if closed || !valid || c.shelf.lobState.isInvalidated() {
		return common.NewOracleError(oracleErrors.LobValueInvalidated, nil, "direct LOB connection")
	}
	opCtx, _, cleanup := c.shelf.cancellation.newCancelableOperationContext(ctx, c.shelf.cancelExecution)
	defer cleanup()
	err = operation(opCtx)
	if _shouldDiscardLobSession(err) {
		// A direct handle runs outside database/sql's normal statement path. If
		// its terminal response was not consumed, no later locator operation may
		// safely reuse this TTC stream.
		abandonLobSession(c.shelf)
	}
	return err
}

// _shouldDiscardLobSession reports errors from an attempted TTC exchange
// whose terminal response is not known to have been consumed. Validation and
// local locator-state errors never enter this path and must not poison a sound
// physical session.
func _shouldDiscardLobSession(err error) bool {
	if err == nil || isCompletedLobResponseError(err) {
		return false
	}
	var coded oracleErrors.SQLError
	return errors.As(err, &coded) && coded.ErrorCode() == string(oracleErrors.LobExecError)
}

// _lobLocator validates locator bytes and builds an offset-specific wrapper.
//
// Parameters:
//   - bytes: non-empty locator byte representation.
//   - offset: 1-based Oracle logical position.
//
// Returns:
//   - *locator: locator wrapper for the requested offset.
//   - error: InvalidLOBBuffer when bytes is empty.
func _lobLocator(bytes []byte, offset uint64) (*locator, error) {
	if len(bytes) == 0 {
		return nil, common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"direct-lob",
			"lob",
			"empty locator",
		)
	}
	return newLocator(append(driverCommon.B1Array(nil), bytes...), driverCommon.UB8(offset)), nil
}
