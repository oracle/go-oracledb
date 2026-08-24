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
and exposes the column data in raw byte-array form.
*/
type tTIrxd struct {
	// numberOfColumns is the expected number of columns for each incoming RXD row.
	numberOfColumns driverCommon.UB4
	// rowCount is the zero-based index of the row currently being processed/unmarshalled.
	rowCount driverCommon.UB4
	// row holds, for the current row, a common.B1Array (wire-format bytes) for each column.
	// row[i] gives the raw data for column i for this row.
	row []driverCommon.B1Array
	// prevRow contains each column's data from the previous unmarshalled row, used for column-carry (BVC logic).
	prevRow []driverCommon.B1Array
	// prevLobColContext contains the LOB metadata aligned with prevRow for BVC
	// column carry.
	prevLobColContext []*LobColumnContext
	// bvcColSent is a BitSet indicating which columns are present in this row (per BVC protocol column-carry rules).
	bvcColSent *driverCommon.BitSet
	// bvcFound is true if BVC/column-carry logic applies to the current row (if bvcColSent has any set columns).
	bvcFound bool

	// Outgoing bind payload for TTIRXD when marshalling bind values (Phase 1: single row binds).
	bindRow        []driverCommon.B1Array
	bindOACs       []driverCommon.Marshallable
	columnContexts []ColumnContext
	lobColContext  []*LobColumnContext

	numberOfReturningPositions int
	isDmlReturning             bool

	// session character set
	sessCharSet  driverCommon.UB2
	sessNCharSet driverCommon.UB2
}

// newTTIrxd instantiates a TTIrxd struct configured to decode plain RXD resultset messages from Oracle's TTC protocol.
func newTTIrxd() driverCommon.Message[driverCommon.MessageType] {
	return &tTIrxd{}
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC RXD message code.
func (rxd *tTIrxd) GetMsgCode() driverCommon.MessageType {
	return TTIRXD
}

// SetNumberOfColumns sets the expected column count for RXD row unmarshalling and propagates this to the embedded BVC logic.
func (rxd *tTIrxd) SetNumberOfColumns(noOfCols driverCommon.UB4) {
	rxd.numberOfColumns = noOfCols
}

// SetRowCount sets the index of the row currently being processed in this RXD unmarshal session.
func (rxd *tTIrxd) SetRowCount(rowCount driverCommon.UB4) {
	rxd.rowCount = rowCount
}

func (rxd *tTIrxd) SetNumberofReturningArgs(numberofArgs int) {
	rxd.numberOfReturningPositions = numberofArgs
}

// SetPrevRow assigns the previous row's column data for BVC carry. The matching
// LOB metadata is supplied separately by setPrevLobColumnContext.
func (rxd *tTIrxd) SetPrevRow(row []driverCommon.B1Array) {
	rxd.prevRow = row
}

// setPrevLobColumnContext assigns the per-column LOB metadata for the previous
// row. BVC decoding carries this metadata together with omitted column data.
func (rxd *tTIrxd) setPrevLobColumnContext(lobColContext []*LobColumnContext) {
	rxd.prevLobColContext = lobColContext
}

// SetBvcState sets both the BVC protocol state indicator and the column-sent bitset for BVC column-carry logic.
func (rxd *tTIrxd) SetBvcState(colSent *driverCommon.BitSet, found bool) {
	rxd.bvcColSent = colSent
	rxd.bvcFound = found
}

func (rxd *tTIrxd) setColumnContexts(columnContexts []ColumnContext) {
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

func (rxd *tTIrxd) getLobColumnContext() []*LobColumnContext {
	return rxd.lobColContext
}

/*

setBindValues sets the outgoing bind payload for marshalling as an RXD message.

Parameters:
- row: row is the encoded data for one row: a slice of CLR-ready byte arrays, one per column.
       Each element should contain the wire-format bytes for that column (nil indicates SQL NULL).
*/

func (rxd *tTIrxd) setBindValues(row []driverCommon.B1Array) {
	rxd.bindRow = row
}

func (rxd *tTIrxd) setBindOACs(oacs []driverCommon.Marshallable) {
	rxd.bindOACs = oacs
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
		val := rxd.bindRow[i]
		if val == nil {
			// Write CLR null indicator
			if err := engine.MarshalUB1(ctx, driverCommon.UB1(0)); err != nil {
				common.Odl.Error("tTIrxd.MarshalTo: failed to write null length indicator",
					"error", err, "stage", "null-indicator", "index", i)
				return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
			}
			continue
		}
		// LOB locator binds carry an explicit UB4 locator length before the CLR.
		// Scalar binds start directly with CLR framing.
		if i < len(rxd.bindOACs) {
			if oac, ok := rxd.bindOACs[i].(*tTIoac); ok && oac.requestedtype == DtyBlob {
				if err := engine.MarshalUB4(ctx, driverCommon.UB4(len(val))); err != nil {
					return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
				}
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

// UnMarshalFrom reads and decodes either a regular RXD result-set row or a DML/PLSQL
// RETURNING-bind payload from the provided Marshaller per TTC specifications.
// It uses column/row information previously set on the struct and applies BVC
// column-carry to both raw data and LOB metadata when required. After a
// successful call, BVC state is reset and rxd.row contains the unmarshalled row.
func (rxd *tTIrxd) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	// DML returning case
	if rxd.numberOfReturningPositions > 0 && rxd.isDmlReturning {
		rxd.row = make([]driverCommon.B1Array, rxd.numberOfReturningPositions)
		for col := 0; col < rxd.numberOfReturningPositions; col++ {
			numberOfRows, err := mar.UnmarshalUB4(ctx)
			if err != nil {
				common.Odl.Error("tTIrxd.UnMarshalFrom: failed to read DML RETURNING row count",
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

	common.Odl.Debug("RXD Unmarshal: starting row",
		"rowNum", rxd.rowCount,
		"bvcFound", rxd.bvcFound)

	// BVC rows carry both raw column data and its aligned LOB metadata from the
	// previous row. Columns marked present are unmarshalled as fresh values.
	// For non-BVC rows, unmarshal all columns as fresh.
	rxd.lobColContext = make([]*LobColumnContext, 0, int(rxd.numberOfColumns))
	if rxd.bvcFound {
		// BVC carry requires a previous row from the current result set.
		if rxd.prevRow == nil {
			common.Odl.Debug("BVC/column-carry protocol unsupported for first result row")
			return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
		// The carried row must remain aligned with the result metadata.
		if len(rxd.prevRow) != int(rxd.numberOfColumns) {
			common.Odl.Warn("Previous row column count mismatch",
				"columns", len(rxd.prevRow), "expected", rxd.numberOfColumns)
			return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
		if rxd.prevLobColContext != nil && len(rxd.prevLobColContext) != int(rxd.numberOfColumns) {
			common.Odl.Warn("Previous LOB column context count mismatch",
				"contexts", len(rxd.prevLobColContext), "cols", rxd.numberOfColumns)
			return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
		rxd.row = make([]driverCommon.B1Array, rxd.numberOfColumns)
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
				if rxd.prevRow[col] != nil {
					tmp := make(driverCommon.B1Array, len(rxd.prevRow[col]))
					copy(tmp, rxd.prevRow[col])
					rxd.row[col] = tmp
				}
				var lobContext *LobColumnContext
				if rxd.prevLobColContext != nil {
					lobContext = rxd.prevLobColContext[col]
				}
				rxd.lobColContext = append(rxd.lobColContext, lobContext)
			}
		}
	} else {
		// Non-BVC: all columns present, unmarshal each as fresh.
		rxd.row = make([]driverCommon.B1Array, rxd.numberOfColumns)
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
	if len(rxd.lobColContext) != int(rxd.numberOfColumns) {
		common.Odl.Warn("RXD LOB column context count mismatch",
			"contexts", len(rxd.lobColContext), "cols", rxd.numberOfColumns)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	rxd.bvcFound = false
	common.Odl.Debug("RXD Unmarshal: success",
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
	For non-LOB columns it also appends a nil LOB context entry so the row's LOB metadata
	slice remains aligned with the row data by column position.

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
	switch dtyType {
	case DtyClob:
		if err := rxd._unmarshalClobColumn(ctx, mar, col); err != nil {
			return err
		}
	case DtyBlob, DtyJSON:
		if err := rxd._unmarshalBlobColumn(ctx, mar, col, dtyType); err != nil {
			return err
		}
	default:
		if err := rxd._unmarshalScalarColumn(ctx, mar, col); err != nil {
			return err
		}
		rxd.lobColContext = append(rxd.lobColContext, nil)
	}
	return nil
}

// _unmarshalScalarColumn decodes a single column's value into rxd.row[col].
// Reads length and value per TTC wire format. Returns error on failure.
func (rxd *tTIrxd) _unmarshalScalarColumn(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal column data column", "index", col, "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	common.Odl.Debug("RXD Unmarshal: column data decoded",
		"col", col,
		"length", length,
		"data", colData)
	rxd.row[col] = colData
	return nil
}

/*
_unmarshalClobColumn unmarshals a prefetched CLOB/NCLOB column and its LOB metadata.

Description:

	Reads the TTC LOB header for text LOBs, including logical length, prefetch metadata,
	character set information, inline prefetched data, and the server-side LOB locator.
	The decoded raw bytes are stored in rxd.row[col] and the associated LOB metadata is
	appended to rxd.lobColContext.

Parameters:
  - ctx: Request context used by the marshaller while reading from the wire.
  - mar: Marshaller used to decode bytes from the RXD payload.
  - col: Zero-based column index within the current row.

Returns:
  - error: Non-nil if the prefetched CLOB payload cannot be unmarshalled.

Errors:
  - Returns an error when the prefetched column payload cannot be read from the wire.
*/
func (rxd *tTIrxd) _unmarshalClobColumn(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	// length
	lob := &LobColumnContext{}
	var err error
	if lob.LobLength, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read LOB length",
			"error", err, "stage", "lob-length", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	if lob.LobLength == 0 {
		rxd.row[col] = nil
		rxd.lobColContext = append(rxd.lobColContext, lob)
		return nil
	}

	// prefetched: always for V1
	// ------------------------------------------
	// prefetched length
	if lob.PrefetchLength, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read prefetch length",
			"error", err, "stage", "prefetch-length", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	// prefetched chunk size
	if lob.PrefetchChunkSize, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read prefetch chunk size",
			"error", err, "stage", "prefetch-chunk-size", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	var dbVary bool
	var ub1 driverCommon.UB1
	if ub1, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read dbVary flag",
			"error", err, "stage", "db-vary", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	dbVary = byte(ub1) == 0x1
	if dbVary {
		// characterset
		if lob.CharsetID, err = mar.UnmarshalUB2(ctx); err != nil {
			common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read charset ID",
				"error", err, "stage", "charset-id", "index", col)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
		}
	}

	// the form of use
	if lob.CharsetForm, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read charset form",
			"error", err, "stage", "charset-form", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}

	// If the character set ID was not returned by the server, use the session
	// character set
	if lob.CharsetID == 0 {
		if lob.CharsetForm == 2 {
			lob.CharsetID = rxd.sessNCharSet
		} else {
			lob.CharsetID = rxd.sessCharSet
		}
	}

	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read prefetched column data",
			"error", err, "stage", "prefetched-data", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	common.Odl.Debug("RXD Unmarshal: column data decoded",
		"col", col,
		"length", length,
		"data", colData)
	rxd.row[col] = colData
	// ------------------------------------------

	// locator??
	if lob.LobLocator, _, err = mar.UnmarshalCLRColumnData(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalClobColumn: failed to read LOB locator",
			"error", err, "stage", "lob-locator", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}

	rxd.lobColContext = append(rxd.lobColContext, lob)

	return nil

}

/*
_processIndicator unmarshals the trailing JSON indicator fields for a JSON LOB column.

Description:

	Consumes the two UB2 indicator values emitted after JSON LOB payloads in the TTC
	wire format. Both values are required for proper stream alignment even when the
	column value is NULL.

Parameters:
  - ctx: Request context used by the marshaller while reading from the wire.
  - mar: Marshaller used to decode bytes from the RXD payload.
  - col: Zero-based column index within the current row.

Returns:
  - error: Non-nil if either JSON indicator field cannot be unmarshalled.

Errors:
  - Returns an error when either trailing JSON indicator cannot be read from the wire.
*/
func (rxd *tTIrxd) _processIndicator(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	var err error
	if _, err = mar.UnmarshalSB2(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read JSON indicator 1",
			"error", err, "stage", "json-indicator-1", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	if _, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read JSON indicator 2",
			"error", err, "stage", "json-indicator-2", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	return nil
}

func (rxd *tTIrxd) _processDMLPlSqlIndicator(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	var err error
	if _, err = mar.UnmarshalSB2(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read JSON indicator 1",
			"error", err, "stage", "json-indicator-1", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	return nil
}

/*
_unmarshalBlobColumn unmarshals a prefetched BLOB or JSON column and its LOB metadata.

Description:

	Reads the TTC LOB header for binary LOB payloads, including prefetch metadata, inline
	prefetched bytes, and the server-side locator. When the datatype is JSON, the function
	also consumes the trailing JSON indicator fields emitted by the protocol. The decoded
	raw bytes are stored in rxd.row[col] and the associated LOB metadata is appended to
	rxd.lobColContext.

Parameters:
  - ctx: Request context used by the marshaller while reading from the wire.
  - mar: Marshaller used to decode bytes from the RXD payload.
  - col: Zero-based column index within the current row.
  - dtyType: Oracle TTC datatype, used to distinguish BLOB from JSON handling.

Returns:
  - error: Non-nil if the prefetched BLOB/JSON payload cannot be unmarshalled.

Errors:
  - Returns an error when the prefetched column payload cannot be read from the wire.
*/
func (rxd *tTIrxd) _unmarshalBlobColumn(ctx context.Context, mar driverCommon.Marshaller, col int, dtyType DtyType) error {
	// length
	lob := &LobColumnContext{}
	var err error
	if lob.LobLength, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read LOB length",
			"error", err, "stage", "lob-length", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	if lob.LobLength == 0 {
		rxd.row[col] = nil
		rxd.lobColContext = append(rxd.lobColContext, lob)
		if dtyType == DtyJSON {
			if err = rxd._processIndicator(ctx, mar, col); err != nil {
				return err
			}
		}
		return nil
	}

	// prefetched: always for V1
	// ------------------------------------------
	// prefetched length
	if lob.PrefetchLength, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read prefetch length",
			"error", err, "stage", "prefetch-length", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	// prefetched chunk size
	if lob.PrefetchChunkSize, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read prefetch chunk size",
			"error", err, "stage", "prefetch-chunk-size", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}

	colData, length, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read prefetched column data",
			"error", err, "stage", "prefetched-data", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}
	common.Odl.Debug("RXD Unmarshal: column data decoded",
		"col", col,
		"length", length,
		"data", colData)
	rxd.row[col] = colData
	// ------------------------------------------

	// locator
	if lob.LobLocator, _, err = mar.UnmarshalCLRColumnData(ctx); err != nil {
		common.Odl.Error("tTIrxd._unmarshalBlobColumn: failed to read LOB locator",
			"error", err, "stage", "lob-locator", "index", col)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[rxd.GetMsgCode()])
	}

	// indicator
	if dtyType == DtyJSON {
		if err = rxd._processIndicator(ctx, mar, col); err != nil {
			return err
		}
	}

	rxd.lobColContext = append(rxd.lobColContext, lob)

	return nil

}
