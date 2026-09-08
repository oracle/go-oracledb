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
	// newDCB creates a DCB for the negotiated TTC protocol version.
	newDCB func() driverCommon.Message[driverCommon.MessageType]
	// newRows builds rows for the top-level implicit cursor descriptor.
	newRows func([]columnContext, driverCommon.SB4) *ttcRowsRefCursor
	// newRefCursorRows builds child rows for REF CURSOR columns in prefetched data.
	newRefCursorRows func([]columnContext, driverCommon.SB4) *ttcRowsRefCursor
	// prefetch indicates that RXH/RXD/BVC data follows each cursor descriptor.
	prefetch bool
	// sessCharSet and sessNCharSet decode character data in nested RXD messages.
	sessCharSet  driverCommon.UB2
	sessNCharSet driverCommon.UB2
	// rows preserves the server order of implicit cursors.
	rows []*ttcRowsRefCursor
}

// newTTIimplres creates an implicit-result decoder for TTC versions 12-16.
func newTTIimplres() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplres{newDCB: newTTIdcb}
}

// newTTIimplres17 creates an implicit-result decoder for TTC versions 17-19.
func newTTIimplres17() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplres{newDCB: newTTIdcb17}
}

// newTTIimplres20 creates an implicit-result decoder for TTC versions 20-23.
func newTTIimplres20() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplres{newDCB: newTTIdcb20}
}

// newTTIimplres24 creates an implicit-result decoder for TTC version 24 and later.
func newTTIimplres24() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplres{newDCB: newTTIdcb24}
}

// GetMsgCode identifies this decoder as TTIIMPLRES.
func (p *tTIimplres) GetMsgCode() driverCommon.MessageType { return TTIIMPLRES }

// configure installs the row factory and negotiated prefetch mode needed to
// decode a TTIIMPLRES payload.
func (p *tTIimplres) configure(newRows func([]columnContext, driverCommon.SB4) *ttcRowsRefCursor, prefetch bool) {
	p.newRows = newRows
	p.prefetch = prefetch
}

// setSessionCharacterSets supplies the character sets used by prefetched RXD rows.
func (p *tTIimplres) setSessionCharacterSets(charSet, ncharSet driverCommon.UB2) {
	p.sessCharSet = charSet
	p.sessNCharSet = ncharSet
}

// setRefCursorRowsFactory installs the factory for nested REF CURSOR columns.
func (p *tTIimplres) setRefCursorRowsFactory(newRows func([]columnContext, driverCommon.SB4) *ttcRowsRefCursor) {
	p.newRefCursorRows = newRows
}

// UnMarshalFrom decodes all implicit cursor descriptors and their optional
// first-round-trip prefetched rows.
func (p *tTIimplres) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	if p.newDCB == nil || p.newRows == nil {
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "implicit result factories are not configured")
	}

	resultSetCount, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.rows = make([]*ttcRowsRefCursor, 0, resultSetCount)
	common.Odl.Debug("Decoding implicit result sets", "count", resultSetCount, "prefetch", p.prefetch)
	for range resultSetCount {
		dcb := p.newDCB().(*tTIdcb)
		if err = dcb.UnMarshalFrom(ctx, mar); err != nil {
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
		common.Odl.Debug("Decoded implicit result cursor descriptor", "cursorID", cursorID, "columns", len(columns), "prefetch", p.prefetch)
		rows := p.newRows(columns, driverCommon.SB4(cursorID))
		if p.prefetch {
			if err = p.unmarshalPrefetch(ctx, mar, rows, columns); err != nil {
				return err
			}
		}
		p.rows = append(p.rows, rows)
	}
	return nil
}

// unmarshalPrefetch consumes the nested RXH/BVC/RXD stream for one implicit
// cursor until its implicit-result OER terminator is received.
func (p *tTIimplres) unmarshalPrefetch(ctx context.Context, mar driverCommon.Marshaller, rows *ttcRowsRefCursor, columns []columnContext) error {
	common.Odl.Debug("Decoding implicit result prefetch", "cursorID", rows.cursorID, "columns", len(columns))
	state := &queryRunState{rows: rows}
	for {
		code, err := mar.UnmarshalUB1(ctx)
		if err != nil {
			common.Odl.Debug("Failed to read implicit result prefetch message type", "cursorID", rows.cursorID, "rows", state.rowCount, "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		common.Odl.Debug("Decoding implicit result prefetch message", "cursorID", rows.cursorID, "messageType", TTCMsgTypeName[driverCommon.MessageType(code)], "rows", state.rowCount)
		switch driverCommon.MessageType(code) {
		case TTIRXH:
			rxh := &tTIrxh{}
			if err = rxh.UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
			common.Odl.Debug("Decoded implicit result RXH", "cursorID", rows.cursorID, "iteration", rxh.iterationNum, "iterations", rxh.numItersThisTime, "requests", rxh.numRequest)
		case TTIBVC:
			bvc := &tTIbvc{}
			bvc.SetNumberOfColumns(driverCommon.UB4(len(columns)))
			if err = bvc.UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
			state.handleBVC(bvc)
			common.Odl.Debug("Decoded implicit result BVC", "cursorID", rows.cursorID, "columns", len(columns), "presentColumns", bvc.bvcColSent.Cardinality())
		case TTIRXD:
			common.Odl.Debug("Decoding implicit result RXD", "cursorID", rows.cursorID, "row", state.rowCount, "hasBVC", state.bvcFound, "hasPreviousRow", state.prevRow != nil)
			rxd := newTTIrxd().(*tTIrxd)
			rxd.refCursorDCB = p.newDCB().(*tTIdcb)
			rxd.setNumberOfColumns(driverCommon.UB4(len(columns)))
			rxd.setColumnContexts(columns)
			rxd.setSessionCharacterSet(p.sessCharSet)
			rxd.setSessionNCharacterSet(p.sessNCharSet)
			if p.newRefCursorRows != nil {
				rxd.setRefCursorRowsFactory(p.newRefCursorRows)
			}
			rxd.setRowCount(state.rowCount)
			rxd.setBvcState(state.bvcColSent, state.bvcFound)
			if state.prevRow != nil {
				rxd.setPrevRow(state.prevRow)
				rxd.setPrevRefCursorRows(state.prevRefCursorRows)
				rxd.setPrevLobColumnContext(state.prevLobColContext)
			}
			if err = rxd.UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
			state.rowCount++
			state.handleRXDRow(rxd)
			common.Odl.Debug("Decoded implicit result RXD", "cursorID", rows.cursorID, "row", state.rowCount-1, "columns", len(rxd.row), "totalRows", len(rows.rowData))
		case TTIIMPLOER:
			oer := &tTIoer{}
			if err = oer.UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
			common.Odl.Debug("Decoded implicit result OER", "cursorID", rows.cursorID, "returnCode", oer.retCode, "errorCode", oer.oerrcd2, "rows", state.rowCount)
			if err = oer.getError(); err != nil && oer.retCode != 1403 && oer.oerrcd2 != 1403 {
				return err
			}
			rows.numOfRows = len(rows.rowData)
			rows.fetch = nil
			common.Odl.Debug("Completed implicit result prefetch", "cursorID", rows.cursorID, "rows", rows.numOfRows)
			return nil
		default:
			common.Odl.Debug("Unexpected implicit result prefetch message", "cursorID", rows.cursorID, "messageType", code, "rows", state.rowCount)
			return common.NewOracleError(oracleErrors.ProtocolViolation, nil, "unexpected implicit result prefetch message", code)
		}
	}
}
