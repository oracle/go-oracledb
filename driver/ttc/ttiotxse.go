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

	"github.com/oracle/go-driver/driver/common"
)

const (
	// OTXSE transaction switching opcodes.
	otxseStart  common.SB4 = 0x01
	otxseDetach common.SB4 = 0x02
	otxsePost   common.SB4 = 0x04

	// OTXSE flags used for sessionless transaction control.
	otxseTransNew         common.UB4 = 0x00000001
	otxseTransResume      common.UB4 = 0x00000004
	otxseTransSessionless common.UB4 = 0x00000010

	// Oracle format identifier used by JDBC for sessionless transactions.
	k2gSessionless common.UB4 = 0x004e5c3e
)

// tTIOtxse represents the OTXSE TTC function request used for transaction
// start, resume and detach operations.
type tTIOtxse struct {
	headerMarshaller common.Marshallable
	msgCode          common.MessageType

	operation          common.SB4
	transactionContext common.B1Array
	xid                common.B1Array
	formatID           common.UB4
	gtridLength        common.UB4
	bqualLength        common.UB4
	flags              common.UB4
	timeout            common.UB2
	applicationValue   *common.UB4
	internalName       common.B1Array
	externalName       common.B1Array
}

// newOTxSe creates an OTXSE function message using the standard TTIFUN header.
func newOTxSe() common.Message[common.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader{_funcType: oTxSe},
		msgCode:          TTIFUN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// newOTxSe18 creates an OTXSE function message using the TTC 18+ TTIFUN header.
func newOTxSe18() common.Message[common.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oTxSe}},
		msgCode:          TTIFUN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// newOTxSePfn creates an OTXSE piggyback message using the standard TTIPFN header.
func newOTxSePfn() common.Message[common.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader{_funcType: oTxSe},
		msgCode:          TTIPFN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// newOTxSePfn18 creates an OTXSE piggyback message using the TTC 18+ TTIPFN header.
func newOTxSePfn18() common.Message[common.MessageType] {
	return &tTIOtxse{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oTxSe}},
		msgCode:          TTIPFN,
		formatID:         k2gSessionless,
		flags:            otxseTransSessionless,
	}
}

// GetMsgCode returns the TTC message category used to send this OTXSE request.
func (m *tTIOtxse) GetMsgCode() common.MessageType { return m.msgCode }

// GetFuncCode returns the TTC function code for transaction switching operations.
func (m *tTIOtxse) GetFuncCode() common.FunctionType { return oTxSe }

// setOperation selects the OTXSE opcode, such as start, detach, or post-call detach.
func (m *tTIOtxse) setOperation(op common.SB4) { m.operation = op }

// setTransactionContext stores the transaction context returned by prior transaction calls.
func (m *tTIOtxse) setTransactionContext(ctxBytes common.B1Array) { m.transactionContext = ctxBytes }

// setXID sets the serialized XID bytes together with the logical GTRID and BQUAL lengths.
func (m *tTIOtxse) setXID(xid common.B1Array, gtridLength, bqualLength common.UB4) {
	m.xid = xid
	m.gtridLength = gtridLength
	m.bqualLength = bqualLength
}

// setTimeout sets the server-side sessionless transaction timeout in seconds.
func (m *tTIOtxse) setTimeout(timeout common.UB2) { m.timeout = timeout }

// setFlags sets the OCI transaction flags marshalled in the OTXSE payload.
func (m *tTIOtxse) setFlags(flags common.UB4) { m.flags = flags }

// setFormatID sets the XID format identifier sent with the OTXSE request.
func (m *tTIOtxse) setFormatID(formatID common.UB4) { m.formatID = formatID }

// setApplicationValue sets the optional application value field sent with the request.
func (m *tTIOtxse) setApplicationValue(value common.UB4) { m.applicationValue = &value }

// setInternalName sets the optional client internal database name field.
func (m *tTIOtxse) setInternalName(name common.B1Array) { m.internalName = name }

// setExternalName sets the optional client external database name field.
func (m *tTIOtxse) setExternalName(name common.B1Array) { m.externalName = name }

// MarshalTo serializes the OTXSE request using the TTC wire layout for transaction switching.
func (m *tTIOtxse) MarshalTo(ctx context.Context, engine common.Marshaller) error {
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OTXSE header", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if err := engine.MarshalSB4(ctx, m.operation); err != nil {
		common.Odl.Warn("Error marshalling OTXSE operation", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	sendTxnContext := len(m.transactionContext) > 0 && m.operation == otxseDetach && m.formatID != k2gSessionless
	if sendTxnContext {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE transaction context ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null transaction context ptr", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if err := engine.MarshalUB4(ctx, common.UB4(len(m.transactionContext))); err != nil {
		common.Odl.Warn("Error marshalling OTXSE transaction context length", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.formatID); err != nil {
		common.Odl.Warn("Error marshalling OTXSE format id", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.gtridLength); err != nil {
		common.Odl.Warn("Error marshalling OTXSE gtrid length", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.bqualLength); err != nil {
		common.Odl.Warn("Error marshalling OTXSE bqual length", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if len(m.xid) > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE xid ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null xid ptr", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if err := engine.MarshalUB4(ctx, common.UB4(len(m.xid))); err != nil {
		common.Odl.Warn("Error marshalling OTXSE xid length", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB4(ctx, m.flags); err != nil {
		common.Odl.Warn("Error marshalling OTXSE flags", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}
	if err := engine.MarshalUB2(ctx, m.timeout); err != nil {
		common.Odl.Warn("Error marshalling OTXSE timeout", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if m.applicationValue != nil {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE application value ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	} else if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE null application value ptr", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE return application value ptr", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}
	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OTXSE return context ptr", "error", err)
		return common.NewOracleError(common.FailMarshal, err, nil)
	}

	if len(m.internalName) > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE internal name ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
		if err := engine.MarshalUB4(ctx, common.UB4(len(m.internalName))); err != nil {
			common.Odl.Warn("Error marshalling OTXSE internal name length", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	} else {
		if err := engine.MarshalNullPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE null internal name ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
		if err := engine.MarshalUB4(ctx, 0); err != nil {
			common.Odl.Warn("Error marshalling OTXSE zero internal name length", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}

	if len(m.externalName) > 0 {
		if err := engine.MarshalPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE external name ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
		if err := engine.MarshalUB4(ctx, common.UB4(len(m.externalName))); err != nil {
			common.Odl.Warn("Error marshalling OTXSE external name length", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	} else {
		if err := engine.MarshalNullPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling OTXSE null external name ptr", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
		if err := engine.MarshalUB4(ctx, 0); err != nil {
			common.Odl.Warn("Error marshalling OTXSE zero external name length", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}

	if sendTxnContext {
		if err := engine.MarshalB1Array(ctx, m.transactionContext); err != nil {
			common.Odl.Warn("Error marshalling OTXSE transaction context", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}
	if len(m.xid) > 0 {
		if err := engine.MarshalB1Array(ctx, m.xid); err != nil {
			common.Odl.Warn("Error marshalling OTXSE xid", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}
	if m.applicationValue != nil {
		if err := engine.MarshalUB4(ctx, *m.applicationValue); err != nil {
			common.Odl.Warn("Error marshalling OTXSE application value", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}
	if len(m.internalName) > 0 {
		if err := engine.MarshalChar(ctx, m.internalName); err != nil {
			common.Odl.Warn("Error marshalling OTXSE internal name", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}
	if len(m.externalName) > 0 {
		if err := engine.MarshalChar(ctx, m.externalName); err != nil {
			common.Odl.Warn("Error marshalling OTXSE external name", "error", err)
			return common.NewOracleError(common.FailMarshal, err, nil)
		}
	}

	return nil
}
