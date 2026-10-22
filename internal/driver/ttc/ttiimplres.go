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

// tTIimplres decodes the implicit result sets returned by
// DBMS_SQL.RETURN_RESULT. The message starts with the number of result sets,
// followed by one DCB/cursor pair for each result set. Its optional nested
// RXH/RXD/BVC stream is emitted only when the client and server negotiated
// implicit-result prefetch.
type tTIimplres struct {
	// messageFactory creates protocol-version-matched DCB, RXD, and nested
	// prefetch messages without retaining connection state.
	messageFactory Factory
	// prefetch indicates that RXH/RXD/BVC data follows each cursor descriptor.
	prefetch bool
	// rows preserves the server order of implicit cursors.
	rows []*ttcRowsRefCursor
}

// newTTIimplres creates an implicit-result decoder without prefetch support.
func newTTIimplres() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplres{}
}

// newTTIimplresWithPrefetchSupport creates a decoder for prefetched implicit results.
func newTTIimplresWithPrefetchSupport() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplres{prefetch: true}
}

// GetMsgCode identifies this decoder as TTIIMPLRES.
func (p *tTIimplres) GetMsgCode() driverCommon.MessageType { return TTIIMPLRES }

// setMessageFactory implements messageFactoryUser for nested TTC messages.
func (p *tTIimplres) setMessageFactory(factory Factory) {
	p.messageFactory = factory
}

/*
UnmarshalFrom decodes all implicit cursor descriptors and their optional
first-round-trip prefetched rows.

Description:

  - Reads the result-set count and each DCB/cursor-ID descriptor emitted by
    DBMS_SQL.RETURN_RESULT.
  - Initializes one RefCursor rows object per server cursor.
  - Delegates the optional RXH/BVC/RXD/OER prefetch stream to unmarshalPrefetch
    when the negotiated capability is enabled.
  - Preserves server result-set order in rows for RowsNextResultSet traversal.

Parameters:

- ctx: context governing protocol unmarshalling.
- mar: TTC marshaller positioned at the TTIIMPLRES payload.

Returns:

  - error: a protocol, descriptor, or prefetch-decoding error; nil after all
    implicit cursors have been decoded.

Notes:

  - The injected message factory allocates version-specific DCB, RXD, and OER
    messages for the negotiated TTC protocol.
*/
func (p *tTIimplres) UnmarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	resultSetCount, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.rows = make([]*ttcRowsRefCursor, 0, resultSetCount)
	common.Odl.Fine("Decoding implicit result sets", "count", resultSetCount, "prefetch", p.prefetch)
	for range resultSetCount {
		msg, err := p.messageFactory.GetMessage(TTIDCB)
		if err != nil {
			return common.NewOracleError(oracleErrors.CallbackFactoryError, err, "unmarshal implicit-result DCB")
		}
		dcb := msg.(*tTIdcb)
		if err = dcb.UnmarshalFrom(ctx, mar); err != nil {
			return err
		}
		columns, err := dcb.getColumnContexts()
		if err != nil {
			return err
		}
		cursorID, err := mar.UnmarshalUB4(ctx)
		if err != nil {
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		common.Odl.Fine("Decoded implicit result cursor descriptor", "cursorID", cursorID, "columns", len(columns), "prefetch", p.prefetch)
		rows := newRefCursorRows(driverCommon.SB4(cursorID), columns)
		if p.prefetch {
			if err = p.unmarshalPrefetch(ctx, mar, rows, columns); err != nil {
				return err
			}
		}
		p.rows = append(p.rows, rows)
	}
	return nil
}

// getRefCursorRows returns the top-level implicit cursors that must be attached
// to the connection after TTIIMPLRES unmarshalling.
func (p *tTIimplres) getRefCursorRows() []*ttcRowsRefCursor {
	return p.rows
}

/*
unmarshalPrefetch consumes the nested RXH/BVC/RXD stream for one implicit
cursor until its implicit-result OER terminator is received.

Description:

  - Reads and dispatches RXH, BVC, and RXD messages returned for a single
    implicit cursor in the initial round trip.
  - Reuses queryRunState to retain BVC column-carry state, prior row data, LOB
    context, and nested RefCursor values across RXD messages.
  - Finalizes the supplied rows when the implicit-result OER terminator reports
    success or the expected no-data status.

Parameters:

- ctx: context governing protocol unmarshalling.
- mar: TTC marshaller positioned at the first nested prefetch message.
- rows: initialized rows object that receives the prefetched data.
- columns: cursor descriptor metadata used to configure each RXD message.

Returns:

  - error: a nested message, Oracle error, or unexpected-message error; nil when
    the cursor's prefetch stream terminates successfully.

Notes:

  - A non-prefetched implicit cursor keeps its deferred RefCursor fetch function
    and never enters this method.
*/
func (p *tTIimplres) unmarshalPrefetch(ctx context.Context, mar driverCommon.Marshaller, rows *ttcRowsRefCursor, columns []columnContext) error {
	common.Odl.Fine("Decoding implicit result prefetch", "cursorID", rows.cursorID, "columns", len(columns))
	state := &queryRunState{rows: rows.ttcRows}
	for {
		code, err := mar.UnmarshalUB1(ctx)
		if err != nil {
			common.Odl.Fine("Failed to read implicit result prefetch message type", "cursorID", rows.cursorID, "rows", state.rowCount, "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		common.Odl.Fine("Decoding implicit result prefetch message", "cursorID", rows.cursorID, "messageType", TTCMsgTypeName[driverCommon.MessageType(code)], "rows", state.rowCount)
		switch driverCommon.MessageType(code) {
		case TTIRXH:
			msg, err := p.messageFactory.GetMessage(TTIRXH)
			if err != nil {
				common.Odl.Error("unmarshalPrefetch: GetMessage(TTIRXH) failed", "error", err, "stage", "get-rxh")
				return common.NewOracleError(oracleErrors.CallbackFactoryError, err, "unmarshalPrefetch failed")
			}
			rxh := msg.(*tTIrxh)
			if err = rxh.UnmarshalFrom(ctx, mar); err != nil {
				return err
			}
			common.Odl.Fine("Decoded implicit result RXH", "cursorID", rows.cursorID, "iteration", rxh.iterationNum, "iterations", rxh.numItersThisTime, "requests", rxh.numRequest)
		case TTIBVC:
			msg, err := p.messageFactory.GetMessage(TTIBVC)
			if err != nil {
				common.Odl.Error("unmarshalPrefetch: GetMessage(TTIBVC) failed", "error", err, "stage", "get-bvc")
				return common.NewOracleError(oracleErrors.CallbackFactoryError, err, "unmarshalPrefetch failed")
			}
			bvc := msg.(*tTIbvc)
			bvc.SetNumberOfColumns(driverCommon.UB4(len(columns)))
			if err = bvc.UnmarshalFrom(ctx, mar); err != nil {
				return err
			}
			state.handleBVC(bvc)
			common.Odl.Fine("Decoded implicit result BVC", "cursorID", rows.cursorID, "columns", len(columns), "presentColumns", bvc.bvcColSent.Cardinality())
		case TTIRXD:
			common.Odl.Fine("Decoding implicit result RXD", "cursorID", rows.cursorID, "row", state.rowCount, "hasBVC", state.bvcFound, "hasPreviousRow", state.prevRow != nil)
			// Configure the version-specific RXD decoder with the preceding row
			// state so BVC-omitted columns can be carried into this row.
			rxd, err := state.createRXD(columns, p.messageFactory)
			if err != nil {
				return err
			}
			// Decode the wire image before retaining it: REF CURSOR columns may
			// include nested DCB metadata that is consumed by RXD decoding.
			if err = rxd.UnmarshalFrom(ctx, mar); err != nil {
				return err
			}
			// Retain decoded values and make them the source for the next BVC row.
			state.handleRXDRow(rxd)
			common.Odl.Fine("Decoded implicit result RXD", "cursorID", rows.cursorID, "row", state.rowCount-1, "columns", len(rxd.row), "totalRows", len(rows.rowData))
		case TTIIMPLOER, TTIOER:
			msg, err := p.messageFactory.GetMessage(TTIIMPLOER)
			if err != nil {
				common.Odl.Error("unmarshalPrefetch: GetMessage(TTIIMPLOER) failed", "error", err, "stage", "get-implicit-oer")
				return common.NewOracleError(oracleErrors.CallbackFactoryError, err, "unmarshalPrefetch failed")
			}
			oer := msg.(tTIOerIface)
			if err = oer.(driverCommon.Unmarshallable).UnmarshalFrom(ctx, mar); err != nil {
				return err
			}
			oerErr := oer.getError()
			common.Odl.Fine("Decoded implicit result OER", "cursorID", rows.cursorID, "error", oerErr, "rows", state.rowCount)
			if oerErr != nil {
				sqlErr, ok := oerErr.(oracleErrors.SQLError)
				if !ok || sqlErr.ErrorCode() != string(oracleErrors.NoDataFound) {
					return oerErr
				}
			}
			rows.numOfRows = len(rows.rowData)
			rows.fetched = true
			common.Odl.Fine("Completed implicit result prefetch", "cursorID", rows.cursorID, "rows", rows.numOfRows)
			return nil
		default:
			common.Odl.Error("Unexpected implicit result prefetch message", "cursorID", rows.cursorID, "messageType", code, "rows", state.rowCount)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
	}
}
