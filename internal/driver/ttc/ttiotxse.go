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
	"database/sql/driver"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type txOperation uint8

const (
	// OTXSE transaction switching opcodes.
	otxseStart  driverCommon.SB4 = 0x01
	otxseDetach driverCommon.SB4 = 0x02
	otxsePost   driverCommon.SB4 = 0x04

	// OTXSE flags used for sessionless transaction control.
	otxseTransNew          driverCommon.UB4 = 0x00000001
	otxseTransResume       driverCommon.UB4 = 0x00000004
	otxseTransSessionless  driverCommon.UB4 = 0x00000010
	otxseTransReadOnly     driverCommon.UB4 = 0x00000100
	otxseTransReadWrite    driverCommon.UB4 = 0x00000200
	otxseTransSerializable driverCommon.UB4 = 0x00000400

	// Oracle format identifier used by JDBC for sessionless transactions.
	k2gSessionless driverCommon.UB4 = 0x004e5c3e
)

// tTIOtxse represents the OTXSE TTC function request used for transaction
// start, resume and detach operations.
type tTIOtxse struct {
	headerMarshaller driverCommon.Marshallable
	msgCode          driverCommon.MessageType

	operation                 driverCommon.SB4
	xid                       driverCommon.B1Array
	formatID                  driverCommon.UB4
	globalTransactionIDLength driverCommon.UB4
	bqualLength               driverCommon.UB4
	flags                     driverCommon.UB4
	timeout                   driverCommon.UB2
	applicationValue          *driverCommon.UB4
}

// newOTxSe creates an OTXSE function message using the standard TTIFUN header.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New OTXSE message.
func newOTxSe() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader{_funcType: oTxSe},
		msgCode:          TTIFUN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// newOTxSe18 creates an OTXSE function message using the TTC 18+ TTIFUN header.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New TTC 18+ OTXSE message.
func newOTxSe18() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oTxSe}},
		msgCode:          TTIFUN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// newOTxSePfn creates an OTXSE piggyback message using the standard TTIPFN header.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New OTXSE piggyback message.
func newOTxSePfn() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader{_funcType: oTxSe},
		msgCode:          TTIPFN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// newOTxSePfn18 creates an OTXSE piggyback message using the TTC 18+ TTIPFN header.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New TTC 18+ OTXSE piggyback message.
func newOTxSePfn18() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oTxSe}},
		msgCode:          TTIPFN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// GetMsgCode returns the TTC message category used to send this OTXSE request.
//
// Returns:
//   - driverCommon.MessageType: Message category configured for this request.
func (m *tTIOtxse) GetMsgCode() driverCommon.MessageType { return m.msgCode }

// GetFuncCode returns the TTC function code for transaction switching operations.
//
// Returns:
//   - driverCommon.FunctionType: OTXSE function code.
func (m *tTIOtxse) GetFuncCode() driverCommon.FunctionType { return oTxSe }

// configureForStart configures m for a new transaction start operation.
//
// Parameters:
//   - tx: Transaction to start.
//   - opts: Transaction isolation and read-only options.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxse) configureForStart(tx oracleTx, opts driver.TxOptions) {
	m.operation = otxseStart
	applicationValue := driverCommon.UB4(0)
	m.applicationValue = &applicationValue
	m.flags = convertTxOptionsToFlags(opts)
	if sessionlessTx, ok := tx.(*sessionlessTransaction); ok {
		m._configureSessionless(sessionlessTx)
	}
}

// configureForResume configures m for resuming a sessionless transaction.
//
// Parameters:
//   - sessionlessTx: Sessionless transaction to resume.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxse) configureForResume(sessionlessTx *sessionlessTransaction) {
	m.operation = otxseStart
	m.flags = otxseTransResume
	applicationValue := driverCommon.UB4(0)
	m.applicationValue = &applicationValue
	m._configureSessionless(sessionlessTx)
}

// configureForSuspend configures m for detaching a sessionless transaction.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxse) configureForSuspend() {
	m.operation = otxseDetach
	m.flags = otxseTransSessionless
	m.formatID = k2gSessionless
	applicationValue := driverCommon.UB4(0)
	m.applicationValue = &applicationValue
}

// _configureSessionless copies sessionless transaction identifiers into m.
//
// Parameters:
//   - sessionlessTx: Sessionless transaction whose identifiers should be sent.
//
// Returns:
//   - None. The message is updated in place.
func (m *tTIOtxse) _configureSessionless(sessionlessTx *sessionlessTransaction) {
	m.formatID = k2gSessionless
	m.xid = sessionlessTx.xid
	m.globalTransactionIDLength = sessionlessTx.globalTransactionIDLength
	m.bqualLength = sessionlessTx.bqualLength
	m.timeout = driverCommon.UB2(sessionlessTx.timeout)
	m.flags |= otxseTransSessionless
}

// MarshalTo serializes the OTXSE request using the TTC wire layout for transaction switching.
//
// Parameters:
//   - ctx: Context used during serialization.
//   - engine: Marshaller receiving the encoded message.
//
// Returns:
//   - error: Error if any part of the message cannot be serialized.
func (m *tTIOtxse) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OTXSE header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalSB4(ctx, m.operation); err != nil {
		common.Odl.Warn("Error marshalling OTXSE operation", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// transaction context not used for sessionless transactions
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null transaction context ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// transaction context length is always 0
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OTXSE transaction context length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.formatID); err != nil {
		common.Odl.Warn("Error marshalling OTXSE format id", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.globalTransactionIDLength); err != nil {
		common.Odl.Warn("Error marshalling OTXSE global transaction ID length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.bqualLength); err != nil {
		common.Odl.Warn("Error marshalling OTXSE bqual length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if len(m.xid) > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE xid ptr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null xid ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalUB4(ctx, driverCommon.UB4(len(m.xid))); err != nil {
		common.Odl.Warn("Error marshalling OTXSE xid length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.flags); err != nil {
		common.Odl.Warn("Error marshalling OTXSE flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB2(ctx, m.timeout); err != nil {
		common.Odl.Warn("Error marshalling OTXSE timeout", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if m.applicationValue != nil {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE application value ptr", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null application value ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE return application value ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE return context ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// internal name is always NULL PTR
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null internal name ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OTXSE zero internal name length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// external name is always NULL PTR
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null external name ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Warn("Error marshalling OTXSE zero external name length", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	if len(m.xid) > 0 {
		if err := engine.MarshalB1Array(ctx, m.xid); err != nil {
			common.Odl.Warn("Error marshalling OTXSE xid", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}
	if m.applicationValue != nil {
		if err := engine.MarshalUB4(ctx, *m.applicationValue); err != nil {
			common.Odl.Warn("Error marshalling OTXSE application value", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}

	return nil
}

// ttiOTxSeRPA carries the OTXSE return parameters decoded from a TTIRPA reply.
// JDBC's T4CTTIOtxse.readRPA() unmarshals the same UB4 application value
// followed by a UB2 context length and the raw transaction context bytes.
type ttiOTxSeRPA struct {
	applicationValue driverCommon.UB4
	context          driverCommon.B1Array
}

// newOTxSeRPA creates an OTXSE TTIRPA decoder.
//
// Returns:
//   - driverCommon.Message[driverCommon.MessageType]: New OTXSE return-parameter decoder.
func newOTxSeRPA() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOTxSeRPA{}
}

// GetMsgCode returns the TTC message category used for OTXSE return parameters.
//
// Returns:
//   - driverCommon.MessageType: TTIRPA.
func (m *ttiOTxSeRPA) GetMsgCode() driverCommon.MessageType {
	return TTIRPA
}

// GetApplicationValue returns the O2U application value returned by the server.
//
// Returns:
//   - driverCommon.UB4: Application value returned by the server.
func (m *ttiOTxSeRPA) GetApplicationValue() driverCommon.UB4 {
	return m.applicationValue
}

// GetContext returns the transaction context returned by the server.
//
// Returns:
//   - driverCommon.B1Array: Transaction context returned by the server.
func (m *ttiOTxSeRPA) GetContext() driverCommon.B1Array {
	return m.context
}

// UnMarshalFrom decodes the OTXSE reply payload returned in TTIRPA.
//
// Parameters:
//   - ctx: Context used during deserialization.
//   - engine: Marshaller supplying the encoded payload.
//
// Returns:
//   - error: Error if the reply payload cannot be deserialized.
func (m *ttiOTxSeRPA) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	applicationValue, err := engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling OTXSE RPA application value", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	m.applicationValue = applicationValue

	contextLength, err := engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling OTXSE RPA context length", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	if contextLength == 0 {
		m.context = nil
		return nil
	}

	contextBytes, err := engine.UnmarshalB1Array(ctx, int(contextLength))
	if err != nil {
		common.Odl.Warn("Error unmarshalling OTXSE RPA context bytes", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	m.context = contextBytes
	return nil
}
