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

const (
	// statementParsedByServer indicates that the SQL should be parsed by the server during this OALL8 operation.
	statementParsedByServer driverCommon.UB4 = 0x00000001
	// bindValuesPresent indicates that bindValuesPresent values/definitions are included for this OALL8 operation.
	bindValuesPresent driverCommon.UB4 = 0x00000008
	// executeStatement indicates that the statement should be executed as part of this OALL8 operation.
	executeStatement driverCommon.UB4 = 0x00000020
	// fetchRows indicates that rows should be fetched in this OALL8 operation.
	fetchRows driverCommon.UB4 = 0x00000040
	// commitAfterExecution indicates that the server should commit after execution.
	// Should not be set when running within transaction.
	commitAfterExecution driverCommon.UB4 = 0x00000100
	// noPLSQLMode disables PL/SQL mode; marshal as SQL (non-PL/SQL) semantics.
	noPLSQLMode driverCommon.UB4 = 0x00008000
	// defineColumnsProvided indicates that defineColumnsProvided (output) columns are being provided by the client.
	defineColumnsProvided driverCommon.UB4 = 0x00000010

	returnIOVVector driverCommon.UB4 = 0x000400

	// maxLength is the default maximum length (UB4) the server should consider for this OALL8 request.
	maxLength = 2147483647
	// plsqlMaxLength is the maximum length when marshalling PL/SQL.
	plsqlMaxLength = 32760
	// maxValue is a sentinel UB4 used for prefetch size to indicate "use server maximum".
	maxValue = 0xFFFFFFFF
)

// tTIOall represents the OALL8 execute/query TTC function request.
// This is a TTIFUN message carrying an execution payload (SQL text/binds).
type tTIOall struct {
	headerMarshaller driverCommon.Marshallable // headerMarshaller marshals the function code w/wo seq and token numbers
	sql              driverCommon.B1Array      // sql represents SQL text payload encoded in network character set
	options          driverCommon.UB4          // options represents operation flags (statementParsedByServer / bindValuesPresent / executeStatement / defineColumnsProvided /...)
	oall8Options     []driverCommon.UB4        // oall8Options represents variable-length vector (we use len=13)
	cursorId         driverCommon.SB4          // cursorId id of the cursorId
	maxLength        driverCommon.UB4          // maxLength represents max bytes for marshalling
	numberOfBinds    driverCommon.SB4          // numberOfBinds represents number of bindValuesPresent positions present in this request
	defineColumns    driverCommon.SB4          // defineColumns represents number of defineColumnsProvided columns provided by the client
	currentRank      driverCommon.UB4          // currentRank represents current row in processing, incremented at each execution
	maxClientBuffer  driverCommon.UB4          // maxClientBuffer represents max client buffer size for prefetch.
	rowsToFetch      driverCommon.UB4          // rowsToFetch represents number of rows to prefetch when SELECT

	// Phase 1: per-bind OAC definitions to be marshalled in OALL8; bind values are sent via TTIRXD.
	bindOACs []driverCommon.Marshallable

	// Phase 1: per-define OAC definitions to be marshalled in OALL8 for result-set columns.
	defineOACs []driverCommon.Marshallable
}

// NewOall18 constructs a new OALL8 TTC function message with default limits.
func NewOall18() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOall{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oAll8}},
		maxLength:        maxLength,
	}
}

// NewOall constructs a new OALL8 TTC function message with default limits.
func NewOall() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOall{
		headerMarshaller: &ttiFunHeader{_funcType: oAll8},
		maxLength:        maxLength,
	}
}

// GetMsgCode implements common.Message.
func (m *tTIOall) GetMsgCode() driverCommon.MessageType { return TTIFUN }

// getFuncCode returns the TTC function code associated with OALL8.
func (m *tTIOall) GetFuncCode() driverCommon.FunctionType { return oAll8 }

// setSQL sets the SQL text payload (as network charset bytes).
func (m *tTIOall) setSQL(sql driverCommon.B1Array) { m.sql = sql }

// setOptions sets operation flags for OALL8.
func (m *tTIOall) setOptions(opts driverCommon.UB4) { m.options = opts }

// setOall8Options sets the i4 vector (length, iterations, flags, etc.).
func (m *tTIOall) setOall8Options(v []driverCommon.UB4) { m.oall8Options = v }

// setCursorId sets the cursor id.
func (m *tTIOall) setCursorId(c driverCommon.SB4) { m.cursorId = c }

// setMaxLength sets the maximum length used for marshalling.
func (m *tTIOall) setMaxLength(n driverCommon.UB4) { m.maxLength = n }

// setRowsToFetch sets the number of rows to prefetch to maxValue for select.
func (m *tTIOall) setRowsToFetch() {
	m.maxClientBuffer = maxValue
	m.rowsToFetch = maxValue

}

func (m *tTIOall) resetRowsToFetch() {
	m.maxClientBuffer = 0
	m.rowsToFetch = 0

}

// setNumberOfBindPositions sets the number of bind positions expected in the request.
func (m *tTIOall) setNumberOfBindPositions(n driverCommon.SB4) { m.numberOfBinds = n }

// setDefCols sets the number of define columns provided by the client.
func (m *tTIOall) setDefCols(n driverCommon.SB4) { m.defineColumns = n }

// setCurrentRank sets the DML currentRank value used in TTC >=1200 branch.
func (m *tTIOall) setCurrentRank(r driverCommon.UB4) { m.currentRank = r }

/*
setBindOACs attaches per-bind OAC (Oracle Access Descriptor) definitions to this OALL8 request.

Parameter:
  - currRowOac: slice of OAC descriptors, one per bind position, in bind order (SQL 1-based -> slice 0-based).
    Each tTIoac advertises the TTC datatype (e.g., DtyNum, DtyChr via DtyVCS, DtyBin via DtyVbi, DtyBol, DtyStz)
    and the maximum length for the associated bind value. The slice length should be greater than or equal to
    numberOfBinds; only the first numberOfBinds entries are marshalled.

Notes:
  - OACs are marshalled as part of the OALL8 message when options includes bindValuesPresent and numberOfBinds > 0.
  - The actual bind values are sent separately via subsequent TTIRXD messages; OALL8 carries only the OAC metadata.
*/
func (m *tTIOall) setBindOACs(currRowOac []driverCommon.Marshallable) { m.bindOACs = currRowOac }

// setDefineOACs attaches per-define OAC (Oracle Access Descriptor) definitions to this OALL8 request.
func (m *tTIOall) setDefineOACs(oacs []driverCommon.Marshallable) { m.defineOACs = oacs }

// MarshalTo serializes the OALL8 function message.
func (m *tTIOall) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	// marshal function code, sequence number w/wo token-number
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OALL8 header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8 header")
	}

	// ---- Step#1: marshal PISDEF ----
	// options
	if err := engine.MarshalUB4(ctx, m.options); err != nil {
		common.Odl.Warn("Error marshalling OALL8 options", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}

	// cursorId
	if err := engine.MarshalSB4(ctx, m.cursorId); err != nil {
		common.Odl.Warn("Error marshalling OALL8 cursorId", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")

	}

	// sql pointer/length
	if err := m._marshalPtr(ctx, engine, driverCommon.SB4(len(m.sql)), "Oall8-sql-ptr"); err != nil {
		common.Odl.Warn("Error marshalling OALL8 sql ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}
	if err := engine.MarshalSB4(ctx, driverCommon.SB4(len(m.sql))); err != nil {
		common.Odl.Warn("Error marshalling OALL8 sql len", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}

	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 oall8Options", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}
	if err := engine.MarshalSB4(ctx, driverCommon.SB4(len(m.oall8Options))); err != nil {
		common.Odl.Warn("Error marshalling OALL8 oall8Options len", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}

	// al8o4 / al8o4l (out vector)
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 out vector ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 out vector length ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}

	// Prefetch (client buffer size, prefetch size)
	if err := engine.MarshalUB4(ctx, m.maxClientBuffer); err != nil {
		common.Odl.Warn("Error marshalling OALL8 prefetch size", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}
	if err := engine.MarshalUB4(ctx, m.rowsToFetch); err != nil {
		common.Odl.Warn("Error marshalling OALL8 prefetch rows", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}

	// Max length (configured based on pl-sql/ non-pl-sql)
	if err := engine.MarshalUB4(ctx, m.maxLength); err != nil {
		common.Odl.Warn("Error marshalling OALL8 maxLength", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL8")
	}

	// al8bno (bind input parameters)
	if m.options&bindValuesPresent != 0 {
		if err := m._marshalPtr(ctx, engine, m.numberOfBinds, "bind ptr"); err != nil {
			return err
		}

		if err := engine.MarshalSB4(ctx, m.numberOfBinds); err != nil {
			common.Odl.Warn("Error marshalling OALL8 bind count", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "OALL")
		}
	} else {
		if err := m._marshalPtr(ctx, engine, 0, "bind ptr"); err != nil {
			return err
		}

		if err := engine.MarshalSB4(ctx, 0); err != nil {
			common.Odl.Warn("Error marshalling OALL8 bind count", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}

	// app/txn/txnLen/kv/kvLen (unused)
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 app", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 txn", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 txnLen", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 kv", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 kvLen", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// define ptr/count
	if err := m._marshalPtr(ctx, engine, m.defineColumns, "define ptr"); err != nil {
		return err
	}
	if err := engine.MarshalSB4(ctx, m.defineColumns); err != nil {
		common.Odl.Warn("Error marshalling OALL8 define count", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	regidLb, regidMb := 0, 0
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(regidLb)); err != nil {
		common.Odl.Warn("Error marshalling OALL8 regidLb", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 reg ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 reg ptr2", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 null ptr1", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OALL8 al8blvl", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 null ptr2", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OALL8 al8dnaml", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalUB4(ctx, driverCommon.UB4(regidMb)); err != nil {
		common.Odl.Warn("Error marshalling OALL8 regidMb", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := m._marshalPtr(ctx, engine, driverCommon.SB4(m.currentRank), "1200 ptr1"); err != nil {
		return err
	}
	// currentRank as UB4; zero when not set
	if err := engine.MarshalUB4(ctx, m.currentRank); err != nil {
		common.Odl.Warn("Error marshalling OALL8 currentRank", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := m._marshalPtr(ctx, engine, driverCommon.SB4(m.currentRank), "1200 ptr2"); err != nil {
		return err
	}

	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 null ptr1", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OALL8 ub4_1", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 null ptr2", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OALL8 ub4_2", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 1220 null ptr3", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OALL8 null ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OALL8 ub4", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// ---- Step#2: Send data ----
	// SQL char bytes
	if err := engine.MarshalChar(ctx, m.sql); err != nil {
		common.Odl.Warn("Error marshalling OALL8 sql data", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	// oall8Options vector as UB4 sequence
	for i := 0; i < len(m.oall8Options); i++ {
		if err := engine.MarshalUB4(ctx, m.oall8Options[i]); err != nil {
			common.Odl.Warn("Error marshalling OALL8 oall8Options data", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}

	// bindValuesPresent option set and numberOfBinds > 0: marshal per-bind OAC definitions.
	if (m.options&bindValuesPresent) != 0 && m.numberOfBinds > 0 && len(m.bindOACs) > 0 {
		for i := 0; i < int(m.numberOfBinds); i++ {
			if err := m.bindOACs[i].MarshalTo(ctx, engine); err != nil {
				common.Odl.Error("OALL8: marshal bind OAC failed", "error", err, "index", i)
				return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
			}
		}
	}

	// defineColumnsProvided option set and defineColumns > 0: marshal per-define OAC definitions.
	if (m.options&defineColumnsProvided) != 0 && m.defineColumns > 0 && len(m.defineOACs) > 0 {
		for i := 0; i < int(m.defineColumns); i++ {
			if err := m.defineOACs[i].MarshalTo(ctx, engine); err != nil {
				common.Odl.Error("OALL8: marshal define OAC failed", "error", err, "index", i)
				return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
			}
		}
	}

	return nil
}

func (m *tTIOall) _marshalPtr(ctx context.Context, engine driverCommon.Marshaller, n driverCommon.SB4, fieldName string) error {
	if n > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OALL8", "field name", fieldName, "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else {
		if err := engine.MarshalNullPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OALL8", "field name", fieldName, "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}
	return nil
}
