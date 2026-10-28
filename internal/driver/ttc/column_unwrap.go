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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// columnPayload holds raw wire bytes or an already constructed value (for example
// driver.Rows), with its optional LOB metadata. Byte payloads still use the type
// decoder; constructed values pass through unchanged. Handlers normalize NULL to nil.
type columnPayload struct {
	data any               // Raw B1Array bytes or a constructed value; nil represents SQL NULL.
	lob  *lobColumnContext // Optional LOB metadata accompanying the value.
}

/*
clone copies a column payload for row buffering or BVC carry.

Description:

	Copies raw B1Array bytes and LOB locator bytes into independent buffers while
	retaining the identity of constructed values such as cursor rows. A row may
	outlive the RXD message and its marshaller buffer, so locator bytes must be
	owned by the copy as well.

Returns:
  - columnPayload: A payload with detached raw and locator bytes and shared constructed values.
*/
func (p columnPayload) clone() columnPayload {
	if data, ok := p.data.(driverCommon.B1Array); ok {
		p.data = append(driverCommon.B1Array(nil), data...)
	}
	if p.lob != nil {
		lob := *p.lob
		lob.LobLocator = append(driverCommon.B1Array(nil), lob.LobLocator...)
		p.lob = &lob
	}
	return p
}

// columnUnwrapContext carries the column position, dependencies, and session
// character sets required to unwrap one RXD column.
// Handlers must consume exactly one column, and must not modify row storage.
type columnUnwrapContext struct {
	index        int              // Zero-based column position used in diagnostics.
	sessCharSet  driverCommon.UB2 // Session database character set used when the LOB omits its charset ID.
	sessNCharSet driverCommon.UB2 // Session national character set used for the NCLOB charset fallback.
}

/*
columnUnwrapFunc defines the wire reader for one RXD column.

Description:

	Implementations consume exactly one column's wire representation and return its
	raw bytes or constructed value together with optional LOB metadata. They normalize
	SQL NULL data to nil and leave row storage to the caller. Type-specific framing,
	such as JSON indicators, belongs to the handler; outer PL/SQL indicators do not.

Parameters:
  - ctx: Request context used while reading from the wire.
  - mar: Marshaller positioned at the beginning of the column payload.
  - env: Column metadata, zero-based position, and session character sets.

Returns:
  - columnPayload: Column data and optional LOB metadata on success.
  - error: Non-nil if the column cannot be unwrapped.

Errors:
  - Implementations propagate or wrap wire-reading errors and return an empty payload.
*/
type columnUnwrapFunc func(context.Context, driverCommon.Marshaller, columnUnwrapContext) (columnPayload, error)

// unwrapRefCursorColumn decodes a REF CURSOR using the protocol-specific DCB
// supplied by its registered versioned unwrap handler.
func unwrapRefCursorColumn(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext, dcb *tTIdcb) (columnPayload, error) {
	if err := dcb.unmarshalFromRefCursor(ctx, mar); err != nil {
		return columnPayload{}, err
	}
	cursorID, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	if dcb.numUDS == 0 {
		if cursorID != 0 {
			return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[TTIRXD])
		}
		return columnPayload{}, nil
	}
	if cursorID == 0 {
		return columnPayload{}, nil
	}
	columns, err := dcb.getColumnContexts()
	if err != nil {
		return columnPayload{}, err
	}
	return columnPayload{data: newRefCursorRows(driverCommon.SB4(cursorID), columns)}, nil
}

// unwrapRefCursorColumn12 decodes REF CURSOR metadata for TTC versions 12-16.
func unwrapRefCursorColumn12(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	return unwrapRefCursorColumn(ctx, mar, env, newTTIdcb().(*tTIdcb))
}

// unwrapRefCursorColumn17 decodes REF CURSOR metadata for TTC versions 17-19.
func unwrapRefCursorColumn17(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	return unwrapRefCursorColumn(ctx, mar, env, newTTIdcb17().(*tTIdcb))
}

// unwrapRefCursorColumn20 decodes REF CURSOR metadata for TTC versions 20-23.
func unwrapRefCursorColumn20(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	return unwrapRefCursorColumn(ctx, mar, env, newTTIdcb20().(*tTIdcb))
}

// unwrapRefCursorColumn24 decodes REF CURSOR metadata for TTC version 24 and later.
func unwrapRefCursorColumn24(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	return unwrapRefCursorColumn(ctx, mar, env, newTTIdcb24().(*tTIdcb))
}

/*
unwrapCLRColumn reads a column encoded using TTC CLR framing.

Description:

	Reads the length-prefixed bytes and normalizes an empty value to SQL NULL.
	The payload contains no LOB metadata. Conversion of non-NULL bytes to a Go
	value remains the responsibility of the datatype decoder.

Parameters:
  - ctx: Request context used while reading from the wire.
  - mar: Marshaller positioned at the column's CLR length indicator.
  - env: Column context providing the position used in diagnostics.

Returns:
  - columnPayload: Raw bytes, or nil data for SQL NULL, without LOB metadata.
  - error: Non-nil if reading the CLR value fails.

Errors:
  - Returns FailUnmarshal with the underlying CLR read error.
*/
func unwrapCLRColumn(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	var payload columnPayload

	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unwrap column data column", "index", env.index, "error", err)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	common.Odl.Fine("RXD Unwrap: column data decoded",
		"col", env.index,
		"length", length,
		"data", colData)
	payload = byteColumnPayload(colData)
	return payload, nil
}

/*
unwrapClobColumn reads a prefetched CLOB or NCLOB column.

Description:

	Reads the locator length, 64-bit logical LOB length, prefetched chunk size, character
	set information, prefetched bytes, and locator. When the server omits the
	charset ID, charset form selects the session database or national character
	set. A zero locator length returns NULL data with its LOB context.

Parameters:
  - ctx: Request context used while reading from the wire.
  - mar: Marshaller positioned at the text LOB length field.
  - env: Column position and session character sets used for diagnostics and charset fallback.

Returns:
  - columnPayload: Prefetched bytes and LOB metadata, with nil data for a NULL value.
  - error: Non-nil if any required LOB field cannot be read.

Errors:
  - Returns FailUnmarshal with the underlying read error and an empty payload.
*/
func unwrapClobColumn(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	var payload columnPayload

	// length
	lob := &lobColumnContext{}
	var err error
	if lob.LocatorByteLength, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read LOB length",
			"error", err, "stage", "lob-length", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	if lob.LocatorByteLength == 0 {
		payload.data = nil
		payload.lob = lob
		return payload, nil
	}

	// prefetched: always for V1
	// ------------------------------------------
	// total logical LOB length
	if lob.TotalLobLength, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read total LOB length",
			"error", err, "stage", "total-lob-length", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	// prefetched chunk size
	if _, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read prefetch chunk size",
			"error", err, "stage", "prefetch-chunk-size", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	dbVary, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read dbVary flag",
			"error", err, "stage", "db-vary", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	if dbVary == 1 {
		// characterset
		if lob.CharsetID, err = mar.UnmarshalUB2(ctx); err != nil {
			common.Odl.Error("unwrapClobColumn: failed to read charset ID",
				"error", err, "stage", "charset-id", "index", env.index)
			return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
		}
	}

	// the form of use
	if lob.CharsetForm, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read charset form",
			"error", err, "stage", "charset-form", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}

	// If the character set ID was not returned by the server, use the session
	// character set
	if lob.CharsetID == 0 {
		if lob.CharsetForm == 2 {
			lob.CharsetID = env.sessNCharSet
		} else {
			lob.CharsetID = env.sessCharSet
		}
	}

	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read prefetched column data",
			"error", err, "stage", "prefetched-data", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	common.Odl.Fine("RXD Unwrap: prefetched CLOB data decoded",
		"col", env.index,
		"length", length)
	payload = byteColumnPayload(colData)
	// ------------------------------------------

	// locator
	if lob.LobLocator, _, err = mar.UnmarshalCLRColumnData(ctx); err != nil {
		common.Odl.Error("unwrapClobColumn: failed to read LOB locator",
			"error", err, "stage", "lob-locator", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}

	payload.lob = lob

	return payload, nil

}

/*
unwrapBlobColumn reads a prefetched binary LOB payload.

Description:

	Reads the locator length, 64-bit logical LOB length, prefetch metadata,
	prefetched bytes, and locator. A zero locator length returns NULL data with
	its LOB context. This reader also supplies the binary LOB portion of JSON
	values; unwrapJSONColumn consumes their additional trailing indicators.

Parameters:
  - ctx: Request context used while reading from the wire.
  - mar: Marshaller positioned at the binary LOB length field.
  - env: Column context providing the position used in diagnostics.

Returns:
  - columnPayload: Prefetched bytes and LOB metadata, with nil data for a NULL value.
  - error: Non-nil if any required LOB field cannot be read.

Errors:
  - Returns FailUnmarshal with the underlying read error and an empty payload.
*/
func unwrapBlobColumn(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	var payload columnPayload

	// length
	lob := &lobColumnContext{}
	var err error
	if lob.LocatorByteLength, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unwrapBlobColumn: failed to read LOB length",
			"error", err, "stage", "lob-length", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	if lob.LocatorByteLength == 0 {
		payload.data = nil
		payload.lob = lob
		return payload, nil
	}

	// prefetched: always for V1
	// ------------------------------------------
	// total logical LOB length
	if lob.TotalLobLength, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Error("unwrapBlobColumn: failed to read total LOB length",
			"error", err, "stage", "total-lob-length", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	// prefetched chunk size
	if _, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unwrapBlobColumn: failed to read prefetch chunk size",
			"error", err, "stage", "prefetch-chunk-size", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}

	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Error("unwrapBlobColumn: failed to read prefetched column data",
			"error", err, "stage", "prefetched-data", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	common.Odl.Fine("RXD Unwrap: prefetched BLOB data decoded",
		"col", env.index,
		"length", length)
	payload = byteColumnPayload(colData)
	// ------------------------------------------

	// locator
	if lob.LobLocator, _, err = mar.UnmarshalCLRColumnData(ctx); err != nil {
		common.Odl.Error("unwrapBlobColumn: failed to read LOB locator",
			"error", err, "stage", "lob-locator", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}

	payload.lob = lob

	return payload, nil

}

/*
unwrapJSONColumn reads a prefetched OSON payload and its LOB metadata.

Description:

	Reads the LOB length, prefetch metadata, OSON bytes, and locator independently
	of the BLOB reader. A zero LOB length returns NULL data with its LOB context.
	The native JSON fetch path has no trailing JSON indicators.

Parameters:
  - ctx: Request context used while reading from the wire.
  - mar: Marshaller positioned at the JSON LOB length field.
  - env: Column context providing the position used in diagnostics.

Returns:
  - columnPayload: Prefetched JSON bytes and LOB metadata, or nil data for SQL NULL.
  - error: Non-nil if any required JSON field cannot be read.

Errors:
  - Returns FailUnmarshal with the underlying read error and an empty payload.
*/
func unwrapJSONColumn(ctx context.Context, mar driverCommon.Marshaller, env columnUnwrapContext) (columnPayload, error) {
	var payload columnPayload
	lob := &lobColumnContext{}
	var err error

	if lob.LobLength, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unwrapJSONColumn: failed to read LOB length",
			"error", err, "stage", "lob-length", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	if lob.LobLength == 0 {
		payload.lob = lob
		return payload, nil
	}

	if lob.PrefetchLength, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Error("unwrapJSONColumn: failed to read prefetch length",
			"error", err, "stage", "prefetch-length", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	if lob.PrefetchChunkSize, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unwrapJSONColumn: failed to read prefetch chunk size",
			"error", err, "stage", "prefetch-chunk-size", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}

	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Error("unwrapJSONColumn: failed to read prefetched column data",
			"error", err, "stage", "prefetched-data", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}
	common.Odl.Fine("RXD Unwrap: JSON column data decoded",
		"col", env.index,
		"length", length,
		"data", colData)
	payload = byteColumnPayload(colData)

	if lob.LobLocator, _, err = mar.UnmarshalCLRColumnData(ctx); err != nil {
		common.Odl.Error("unwrapJSONColumn: failed to read LOB locator",
			"error", err, "stage", "lob-locator", "index", env.index)
		return columnPayload{}, common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[TTIRXD])
	}

	payload.lob = lob
	return payload, nil
}

/*
byteColumnPayload wraps raw column bytes and normalizes SQL NULL.

Description:

	Returns an empty payload for nil or zero-length input. Non-empty bytes are
	retained without copying and carry no LOB metadata. Call clone when an
	independent byte buffer is required for row storage or BVC carry.

Parameters:
  - data: Raw bytes extracted from the column's wire representation.

Returns:
  - columnPayload: Raw bytes for non-empty input, or nil data for SQL NULL.
*/
func byteColumnPayload(data driverCommon.B1Array) columnPayload {
	if len(data) == 0 {
		return columnPayload{}
	}
	return columnPayload{data: data}
}
