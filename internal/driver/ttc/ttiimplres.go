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
	newDCB           func() (*tTIdcb, error)
	newRows          func([]columnContext, driverCommon.SB4) *ttcRows
	newRefCursorRows func([]columnContext, driverCommon.SB4) *ttcRows
	prefetch         bool
	sessCharSet      driverCommon.UB2
	sessNCharSet     driverCommon.UB2
	rows             []*ttcRows
}

func newTTIimplres() driverCommon.Message[driverCommon.MessageType] { return &tTIimplres{} }

func (p *tTIimplres) GetMsgCode() driverCommon.MessageType { return TTIIMPLRES }

func (p *tTIimplres) configure(newDCB func() (*tTIdcb, error), newRows func([]columnContext, driverCommon.SB4) *ttcRows, prefetch bool) {
	p.newDCB = newDCB
	p.newRows = newRows
	p.prefetch = prefetch
}

func (p *tTIimplres) setSessionCharacterSets(charSet, ncharSet driverCommon.UB2) {
	p.sessCharSet = charSet
	p.sessNCharSet = ncharSet
}

func (p *tTIimplres) setRefCursorRowsFactory(newRows func([]columnContext, driverCommon.SB4) *ttcRows) {
	p.newRefCursorRows = newRows
}

func (p *tTIimplres) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	if p.newDCB == nil || p.newRows == nil {
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, "implicit result factories are not configured")
	}

	resultSetCount, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.rows = make([]*ttcRows, 0, resultSetCount)
	for range resultSetCount {
		dcb, err := p.newDCB()
		if err != nil {
			return err
		}
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

func (p *tTIimplres) unmarshalPrefetch(ctx context.Context, mar driverCommon.Marshaller, rows *ttcRows, columns []columnContext) error {
	state := &queryRunState{rows: rows}
	for {
		code, err := mar.UnmarshalUB1(ctx)
		if err != nil {
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		switch driverCommon.MessageType(code) {
		case TTIRXH:
			if err = (&tTIrxh{}).UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
		case TTIBVC:
			bvc := &tTIbvc{}
			bvc.SetNumberOfColumns(driverCommon.UB4(len(columns)))
			if err = bvc.UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
			state.handleBVC(bvc)
		case TTIRXD:
			rxd := &tTIrxd{}
			rxd.setNumberOfColumns(driverCommon.UB4(len(columns)))
			rxd.setColumnContexts(columns)
			rxd.setSessionCharacterSet(p.sessCharSet)
			rxd.setSessionNCharacterSet(p.sessNCharSet)
			if p.newRefCursorRows != nil {
				rxd.setRefCursorFactories(p.newDCB, p.newRefCursorRows)
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
		case TTIIMPLOER:
			oer := &tTIoer{}
			if err = oer.UnMarshalFrom(ctx, mar); err != nil {
				return err
			}
			if err = oer.getError(); err != nil && oer.retCode != 1403 && oer.oerrcd2 != 1403 {
				return err
			}
			rows.numOfRows = len(rows.rowData)
			rows.fetch = nil
			return nil
		default:
			return common.NewOracleError(oracleErrors.ProtocolViolation, nil, "unexpected implicit result prefetch message", code)
		}
	}
}
