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

/*
tTIrxd provides unmarshalling support for TTC RXD protocol messages, which represent plain result set rows
delivered by the network. This struct receives RXD packets, parses columns per protocol (including column-carry logic for BVC),
and retains raw wire bytes or constructed values together with their LOB metadata.
*/
type tTIrxd struct {
	// numberOfColumns is the expected number of columns for each incoming RXD row.
	numberOfColumns driverCommon.UB4
	// codecFactory supplies protocol-matched decoders during unmarshalling.
	codecFactory codecFactory
	// rowCount is the zero-based index of the row currently being processed/unmarshalled.
	rowCount driverCommon.UB4
	// row holds each value and its optional LOB metadata at the column position.
	row []columnPayload
	// prevRow contains each column's data from the previous unmarshalled row, used for column-carry (BVC logic).
	prevRow []columnPayload
	// bvcColSent is a BitSet indicating which columns are present in this row (per BVC protocol column-carry rules).
	bvcColSent *driverCommon.BitSet
	// bvcFound is true if BVC/column-carry logic applies to the current row (if bvcColSent has any set columns).
	bvcFound bool

	// bindRow holds outgoing bind payloads for one TTIRXD row.
	bindRow        []encodedBind
	columnContexts []columnContext

	numberOfReturningPositions int
	isDmlReturning             bool

	// session character set
	sessCharSet  driverCommon.UB2
	sessNCharSet driverCommon.UB2
}

// encodedBind is one TTC-ready SQL bind value. A temporary LOB locator uses a
// distinct RXD representation: its TTC UB4 byte length precedes its CLR bytes.
type encodedBind struct {
	data       driverCommon.B1Array
	lobLocator bool
}

// newTTIrxd instantiates a TTIrxd struct configured to decode plain RXD resultset messages from Oracle's TTC protocol.
func newTTIrxd() driverCommon.Message[driverCommon.MessageType] {
	return &tTIrxd{}
}

// setCodecFactory implements codecFactoryUser by storing the factory used to
// resolve column wire readers. The message factory supplies this dependency
// before RXD unmarshalling begins.
func (rxd *tTIrxd) setCodecFactory(factory codecFactory) {
	rxd.codecFactory = factory
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC RXD message code.
func (rxd *tTIrxd) GetMsgCode() driverCommon.MessageType {
	return TTIRXD
}

// setNumberOfColumns sets the expected column count for RXD row unmarshalling and propagates this to the embedded BVC logic.
func (rxd *tTIrxd) setNumberOfColumns(noOfCols driverCommon.UB4) {
	rxd.numberOfColumns = noOfCols
}

// setRowCount sets the index of the row currently being processed in this RXD unmarshal session.
func (rxd *tTIrxd) setRowCount(rowCount driverCommon.UB4) {
	rxd.rowCount = rowCount
}

func (rxd *tTIrxd) setNumberofReturningArgs(numberofArgs int) {
	rxd.numberOfReturningPositions = numberofArgs
}

// setPrevRow assigns previous column payloads, including LOB metadata, for BVC carry.
func (rxd *tTIrxd) setPrevRow(row []columnPayload) {
	rxd.prevRow = row
}

// setBvcState sets both the BVC protocol state indicator and the column-sent bitset for BVC column-carry logic.
func (rxd *tTIrxd) setBvcState(colSent *driverCommon.BitSet, found bool) {
	rxd.bvcColSent = colSent
	rxd.bvcFound = found
}

func (rxd *tTIrxd) setColumnContexts(columnContexts []columnContext) {
	rxd.columnContexts = columnContexts
}

func (rxd *tTIrxd) setDmlReturning() {
	rxd.isDmlReturning = true
}

// SetSessionNCharacterSet sets session NChar character set
func (rxd *tTIrxd) setSessionNCharacterSet(sessNCharSet driverCommon.UB2) {
	rxd.sessNCharSet = sessNCharSet
}

// SetSessionNCharacterSet sets session character set
func (rxd *tTIrxd) setSessionCharacterSet(sessCharSet driverCommon.UB2) {
	rxd.sessCharSet = sessCharSet
}

/*

setBindValues sets the outgoing bind payload for marshalling as an RXD message.

Parameters:
- row: row is the encoded data for one row: a slice of CLR-ready byte arrays, one per column.
       Each element should contain the wire-format bytes for that column (nil indicates SQL NULL).
*/

func (rxd *tTIrxd) setBindValues(row []driverCommon.B1Array) {
	rxd.bindRow = make([]encodedBind, len(row))
	for index, value := range row {
		rxd.bindRow[index] = encodedBind{data: value}
	}
}

// setEncodedBinds assigns one fully described outgoing bind row, including
// locator framing for temporary LOB bind values.
func (rxd *tTIrxd) setEncodedBinds(row []encodedBind) {
	rxd.bindRow = row
}

/*
MarshalTo writes the RXD bind payload for outgoing messages (single row).

When used as an outgoing message, tTIrxd marshals the bind row values as a CLR sequence,
using the TTC null length indicator for nil values. The call to MarshalTo marshals the
encoded bytes for a single row.
*/
func (rxd *tTIrxd) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	// Nothing to marshal if no bind payload was configured.
	if rxd.bindRow == nil {
		return nil
	}
	bindCount := len(rxd.bindRow)
	for i := 0; i < bindCount; i++ {
		bind := rxd.bindRow[i]
		val := bind.data
		if val == nil {
			// Write CLR null indicator
			if err := engine.MarshalUB1(ctx, driverCommon.UB1(0)); err != nil {
				common.Odl.Error("tTIrxd.MarshalTo: failed to write null length indicator",
					"error", err, "stage", "null-indicator", "index", i)
				return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
			}
			continue
		}
		if bind.lobLocator {
			// A LOB bind carries its locator byte length before the CLR value.
			if err := engine.MarshalUB4(ctx, driverCommon.UB4(len(val))); err != nil {
				common.Odl.Error("tTIrxd.MarshalTo: failed to write LOB locator length",
					"error", err, "stage", "lob-locator-length", "index", i)
				return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
			}
		}
		// Write as CLR (short or long form depending on size)
		if err := engine.MarshalCLR(ctx, val, 0, len(val)); err != nil {
			common.Odl.Error("tTIrxd.MarshalTo: failed to write CLR",
				"error", err, "stage", "clr", "index", i)
			return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
	}
	return nil
}

// UnmarshalFrom reads and decodes either a regular RXD result-set row or a DML/PLSQL
// RETURNING-bind payload from the provided Marshaller per TTC specifications.
// It uses column/row information previously set on the struct and applies BVC
// column-carry to both raw data and LOB metadata when required. After a
// successful call, BVC state is reset and rxd.row contains the unmarshalled row.
func (rxd *tTIrxd) UnmarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	// DML returning case
	if rxd.numberOfReturningPositions > 0 && rxd.isDmlReturning {
		rxd.row = make([]columnPayload, rxd.numberOfReturningPositions)
		for col := 0; col < rxd.numberOfReturningPositions; col++ {
			numberOfRows, err := mar.UnmarshalUB4(ctx)
			if err != nil {
				common.Odl.Error("tTIrxd.UnmarshalFrom: failed to read DML RETURNING row count",
					"error", err, "stage", "dml-returning-row-count", "index", col)
				return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
			}
			for row := 0; row < int(numberOfRows); row++ {
				if err := rxd._unmarshalScalarColumn(ctx, mar, col); err != nil {
					return err
				}
				if err := rxd._processDMLPlSqlIndicator(ctx, mar, col); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// PL/SQL case
	if rxd.numberOfReturningPositions > 0 {
		rxd.numberOfColumns = driverCommon.UB4(rxd.numberOfReturningPositions)
	}

	if rxd.numberOfColumns <= 0 {
		common.Odl.Warn("Invalid RXD: column count (DCB.ColCount) must be > 0")
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	if len(rxd.columnContexts) != int(rxd.numberOfColumns) {
		common.Odl.Warn("Invalid RXD: column context count mismatch", "contexts", len(rxd.columnContexts), "cols", rxd.numberOfColumns)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}

	common.Odl.Finest("RXD Unmarshal: starting row",
		"rowNum", rxd.rowCount,
		"bvcFound", rxd.bvcFound)

	// BVC rows carry both raw column data and its aligned LOB metadata from the
	// previous row. Columns marked present are unmarshalled as fresh values.
	// For non-BVC rows, unmarshal all columns as fresh.
	if rxd.bvcFound {
		// BVC carry requires a previous row from the current result set.
		if rxd.prevRow == nil {
			common.Odl.Finest("BVC/column-carry protocol unsupported for first result row")
			return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
		// The carried row must remain aligned with the result metadata.
		if len(rxd.prevRow) != int(rxd.numberOfColumns) {
			common.Odl.Warn("Previous row column count mismatch",
				"columns", len(rxd.prevRow), "expected", rxd.numberOfColumns)
			return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
		rxd.row = make([]columnPayload, rxd.numberOfColumns)
		for col := 0; col < int(rxd.numberOfColumns); col++ {
			if rxd.bvcColSent != nil && rxd.bvcColSent.Get(col) {
				err := rxd._unmarshalColumn(ctx, rxd.getColumnDataType(col), mar, col)
				if err != nil {
					return err
				}
				if rxd.numberOfReturningPositions > 0 {
					if err := rxd._processDMLPlSqlIndicator(ctx, mar, col); err != nil {
						return err
					}
				}
			} else {
				// Not present: carry the previous value and its matching LOB metadata.
				rxd.row[col] = rxd.prevRow[col].clone()
			}
		}
	} else {
		// Non-BVC: all columns present, unmarshal each as fresh.
		rxd.row = make([]columnPayload, rxd.numberOfColumns)
		for col := 0; col < int(rxd.numberOfColumns); col++ {
			err := rxd._unmarshalColumn(ctx, rxd.getColumnDataType(col), mar, col)
			if err != nil {
				return err
			}
			if rxd.numberOfReturningPositions > 0 {
				if err := rxd._processDMLPlSqlIndicator(ctx, mar, col); err != nil {
					return err
				}
			}
		}
	}
	rxd.bvcFound = false
	common.Odl.Finest("RXD Unmarshal: success",
		"rowNum", rxd.rowCount,
		"rowData", rxd.row)
	return nil
}

func (rxd *tTIrxd) getColumnDataType(col int) DtyType {
	return rxd.columnContexts[col].DataType
}

/*
_unmarshalColumn unmarshals a single RXD column according to its Oracle wire type.

Description:

	Dispatches to the LOB-specific or scalar unmarshalling helper based on the TTC datatype.
	Each payload retains its value and optional LOB metadata together.

Parameters:
  - ctx: Request context used by the marshaller while reading from the wire.
  - dtyType: Oracle TTC datatype for the target column.
  - mar: Marshaller used to decode bytes from the RXD payload.
  - col: Zero-based column index within the current row.

Returns:
  - error: Non-nil if unmarshalling the selected column fails.

Errors:
  - Propagates errors returned by the delegated column unmarshalling helper.
*/
func (rxd *tTIrxd) _unmarshalColumn(ctx context.Context, dtyType DtyType, mar driverCommon.Marshaller, col int) error {
	// Preserve CLR framing when the existing decoder lookup has no usable entry.
	handler := unwrapCLRColumn
	if decoder, err := rxd.codecFactory.getDecoder(dtyType); err == nil && decoder != nil && decoder.unwrapColumn != nil {
		handler = decoder.unwrapColumn
	}
	payload, err := handler(ctx, mar, columnUnwrapContext{index: col, sessCharSet: rxd.sessCharSet, sessNCharSet: rxd.sessNCharSet})
	if err != nil {
		return err
	}
	rxd.row[col] = payload
	return nil
}

// _unmarshalScalarColumn decodes a single column's value into rxd.row[col].
// Reads length and value per TTC wire format. Returns error on failure.
func (rxd *tTIrxd) _unmarshalScalarColumn(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	payload, err := unwrapCLRColumn(ctx, mar, columnUnwrapContext{index: col})
	if err != nil {
		return err
	}
	rxd.row[col] = payload
	return nil
}

func (rxd *tTIrxd) _processDMLPlSqlIndicator(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	var err error
	if _, err = mar.UnmarshalSB2(ctx); err != nil {
		common.Odl.Error("tTIrxd._unwrapBlobColumn: failed to read JSON indicator 1",
			"error", err, "stage", "json-indicator-1", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	return nil
}
