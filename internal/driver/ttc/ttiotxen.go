/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all
** copyright and patent rights owned or freely licensable by each licensor
** covering either (i) the unmodified Software as contributed to or provided by
** such licensors, or (ii) the Larger Works (as defined below), to deal in both
** (a) the Software, and (b) any piece of software and/or hardware listed in the
** lrgrwrks.txt file if one is included with the Software (each a "Larger Work"
** to which the Software is contributed by such licensors), without restriction,
** including without limitation the rights to copy, create derivative works of,
** display, perform, and distribute the Software, and to sublicense the foregoing
** rights on either these or other terms.
**
** This license is subject to the condition that the above copyright notice and
** either this complete permission notice or at a minimum a reference to the UPL
** must be included in all copies or substantial portions of the Software.
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

type txStateChangeOperation driverCommon.SB4

// OTXEN transaction state-change operations.
const (
	otxenCommit  txStateChangeOperation = 0x01
	otxenAbort   txStateChangeOperation = 0x02
	otxenPrepare txStateChangeOperation = 0x03
	otxenForget  txStateChangeOperation = 0x04
	otxenRecover txStateChangeOperation = 0x05
	otxmlPrepare txStateChangeOperation = 0x06

	// K2 commands supplied as the OTXEN in-state value.
	k2cmdPrepare       driverCommon.UB4 = 0
	k2cmdRequestCommit driverCommon.UB4 = 1
	k2cmdCommit        driverCommon.UB4 = 2
	k2cmdAbort         driverCommon.UB4 = 3
	k2cmdReadOnly      driverCommon.UB4 = 4
	k2cmdForget        driverCommon.UB4 = 5
	k2cmdRecovered     driverCommon.UB4 = 7
	k2cmdTimeout       driverCommon.UB4 = 8
)

// tTIOtxen represents the OTXEN TTC function used to end, prepare, forget, or
// recover an X/Open transaction.
//
// Its payload follows the server-side definition:
//
//   - opcode (SWORD)
//   - transaction context pointer and length
//   - XID format id, global transaction ID length, and BQUAL length
//   - XID pointer and length
//   - timeout (UWORD)
//   - in-state/K2 command (UB4)
//   - out-state pointer
//   - transaction state change flags (UB4)
//   - transaction context and XID variable data
type tTIOtxen struct {
	headerMarshaller driverCommon.Marshallable

	operation                 driverCommon.SB4
	transactionContext        driverCommon.B1Array
	formatID                  driverCommon.UB4
	globalTransactionIDLength driverCommon.UB4
	bqualLength               driverCommon.UB4
	xid                       driverCommon.B1Array
	timeout                   driverCommon.UB2
	inState                   driverCommon.UB4 // OTXEN in-state/K2 command
	flags                     driverCommon.UB4 // OTXEN transaction state change flags
}

// newOTxEn creates an OTXEN function message using the standard TTIFUN header.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New OTXEN message.
func newOTxEn() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOtxen{
		headerMarshaller: &ttiFunHeader{_funcType: oTxEn},
	}
}

// newOTxEn18 creates an OTXEN function message using the TTC 18+ TTIFUN header.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New TTC 18+ OTXEN message.
func newOTxEn18() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOtxen{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oTxEn}},
	}
}

// GetMsgCode returns the TTC message category used to send OTXEN.
//
// Returns:
//   - driverCommon.MessageType: TTIFUN.
func (m *tTIOtxen) GetMsgCode() driverCommon.MessageType { return TTIFUN }

// GetFuncCode returns the TTC function code for OTXEN.
//
// Returns:
//   - driverCommon.FunctionType: OTXEN function code.
func (m *tTIOtxen) GetFuncCode() driverCommon.FunctionType { return oTxEn }

// configureForCommit configures m for a transaction commit operation.
//
// Parameters:
//   - transaction: Transaction whose XID and timeout should be sent.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxen) configureForCommit(transaction oracleTx) {
	m._configureForOperation(transaction, otxenCommit, k2cmdCommit)
}

// configureForAbort configures m for a transaction rollback operation.
//
// Parameters:
//   - transaction: Transaction whose XID and timeout should be sent.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxen) configureForAbort(transaction oracleTx) {
	m._configureForOperation(transaction, otxenAbort, k2cmdAbort)
}

// _configureForOperation sets the OTXEN operation and transaction identifiers.
//
// Parameters:
//   - transaction: Transaction whose XID and timeout should be sent.
//   - operation: OTXEN state-change operation.
//   - inState: K2 transaction state command.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxen) _configureForOperation(transaction oracleTx, operation txStateChangeOperation, inState driverCommon.UB4) {
	m.operation = driverCommon.SB4(operation)
	m.inState = inState

	if sessionlessTx, ok := transaction.(*sessionlessTransaction); ok {
		m.formatID = k2gSessionless
		m.xid = sessionlessTx.xid
		m.globalTransactionIDLength = sessionlessTx.globalTransactionIDLength
		m.bqualLength = sessionlessTx.bqualLength
		m.timeout = driverCommon.UB2(sessionlessTx.timeout)
	}
}

// MarshalTo serializes OTXEN according to the transaction-end layout.
//
// Parameters:
//   - ctx: Context used during serialization.
//   - engine: Marshaller receiving the encoded message.
//
// Returns:
//   - error: Error if any part of the message cannot be serialized.
func (m *tTIOtxen) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OTXEN header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalSB4(ctx, m.operation); err != nil {
		common.Odl.Warn("Error marshalling OTXEN operation", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if len(m.transactionContext) > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXEN transaction context ptr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXEN null transaction context ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(len(m.transactionContext))); err != nil {
		common.Odl.Warn("Error marshalling OTXEN transaction context length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalUB4(ctx, m.formatID); err != nil {
		common.Odl.Warn("Error marshalling OTXEN format id", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.globalTransactionIDLength); err != nil {
		common.Odl.Warn("Error marshalling OTXEN global transaction ID length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.bqualLength); err != nil {
		common.Odl.Warn("Error marshalling OTXEN BQUAL length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if len(m.xid) > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXEN xid ptr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXEN null xid ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(len(m.xid))); err != nil {
		common.Odl.Warn("Error marshalling OTXEN xid length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB2(ctx, m.timeout); err != nil {
		common.Odl.Warn("Error marshalling OTXEN timeout", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.inState); err != nil {
		common.Odl.Warn("Error marshalling OTXEN in state", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXEN out-state ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.flags); err != nil {
		common.Odl.Warn("Error marshalling OTXEN transaction state change flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if len(m.transactionContext) > 0 {
		if err := engine.MarshalB1Array(ctx, m.transactionContext); err != nil {
			common.Odl.Warn("Error marshalling OTXEN transaction context", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}
	if len(m.xid) > 0 {
		if err := engine.MarshalB1Array(ctx, m.xid); err != nil {
			common.Odl.Warn("Error marshalling OTXEN xid", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}

	return nil
}

// ttiOTxEnRPA carries the OTXEN transaction state returned in TTIRPA.
type ttiOTxEnRPA struct {
	outState driverCommon.UB4
}

// newOTxEnRPA creates the OTXEN TTIRPA decoder.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New OTXEN return-parameter decoder.
func newOTxEnRPA() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOTxEnRPA{}
}

// GetMsgCode returns the TTC message category used for OTXEN return parameters.
//
// Returns:
//   - driverCommon.MessageType: TTIRPA.
func (m *ttiOTxEnRPA) GetMsgCode() driverCommon.MessageType { return TTIRPA }

// GetOutState returns the transaction state returned by OTXEN.
//
// Returns:
//   - driverCommon.UB4: Transaction state returned by the server.
func (m *ttiOTxEnRPA) GetOutState() driverCommon.UB4 { return m.outState }

// UnMarshalFrom decodes the OTXEN TTIRPA payload, which contains one UB4 out
// state value.
//
// Parameters:
//   - ctx: Context used during deserialization.
//   - engine: Marshaller supplying the encoded payload.
//
// Returns:
//   - error: Error if the out-state value cannot be deserialized.
func (m *ttiOTxEnRPA) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	outState, err := engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling OTXEN RPA out state", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	m.outState = outState
	return nil
}
