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
	"fmt"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// connectionNegotiator implements the Negotiator interface for TTC connections.
type connectionNegotiator struct {
	*connectionNegotiatorCommon
	serverProtocolPending bool
}

// newConnectionNegotiator creates a new instance of connectionNegotiator.
func newConnectionNegotiator(buf driverCommon.DataBuffer, serverProtocolPending bool) *connectionNegotiator {
	shared := &connectionNegotiatorCommon{
		_dataBuffer: buf,
	}

	return &connectionNegotiator{
		connectionNegotiatorCommon: shared,
		serverProtocolPending:      serverProtocolPending,
	}
}

// Negotiate performs the connection negotiation process over the provided DataBuffer.
func (cn *connectionNegotiator) Negotiate(ctx context.Context) (*driverCommon.SessionContext, *ttiShelf[driverCommon.MessageType], *ttiCookie, error) {
	var pro *tTIpro
	var dty *tTIdty
	shelf, sessCtx, msgStmr, msgfactory := cn.initializeNegotiationResources()

	// 4. Negotiate Protocol
	var err error
	if cn.serverProtocolPending {
		// After TTIRENEG the server's replacement TTIPRO is already pending;
		// consume it without sending another client protocol request.
		pro, err = cn.receiveProtocolResponse(ctx, msgStmr)
	} else {
		pro, err = cn._negotiateProtocol(ctx, msgStmr, msgfactory)
	}

	if err != nil {
		return nil, nil, nil, err
	}

	// 5. Register negotiated server capabilities
	shelf.RegisterCapabilities(pro.serverCaps.toMap())

	// 6. Negotiate Datatype
	dty, err = cn._negotiateDatatype(ctx, msgStmr, msgfactory, pro)
	if err != nil {
		return nil, nil, nil, err
	}

	cn.applyNegotiatedSettings(shelf, sessCtx, pro, dty)
	// set timezone version number in session context
	sessCtx.SetTimeZoneVersionNumber(dty.GetTimeZoneVersionNumber())
	common.Odl.Fine("connectionNegotiator: Negotiation complete", "SessionContext", sessCtx, "Shelf", shelf)
	cookie := newTTICookieFromTTIpro(pro)
	return sessCtx, shelf, cookie, nil
}

// SetDataBuffer Sets the data buffer used to create the marshaler within the negotiator
func (cn *connectionNegotiator) SetDataBuffer(buf driverCommon.DataBuffer) {
	cn._dataBuffer = buf
}

// _negotiateDatatype negotiates the datatype (TTIDTY) and returns the negotiated *ttc.tTIdty.
func (cn *connectionNegotiator) _negotiateDatatype(
	ctx context.Context,
	msgStmr driverCommon.Streamer[driverCommon.MessageType],
	msgfactory driverCommon.Factory,
	pro *tTIpro,
) (*tTIdty, error) {
	common.Odl.Fine("Start datatype negotiation")
	dtyMsg, err := msgfactory.GetMessage(TTIDTY)
	if err != nil {
		common.Odl.Warn("msgfactory.GetMessage(TTIDTY) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)

	}

	// If the server character set is not multibyte, set the TTCLXMCONV flag to indicate that character data should be treated as length-prefixed.
	if !isCharSetMultibyte(pro.svrCharSet) {
		typeRepresentationTable.SetFlags(TTCLXMCONV)
	}

	common.Odl.Fine("connectionNegotiator: TTIDTY message created and sending")
	if pro == nil || pro.clientCaps == nil {
		common.Odl.Warn("Client Caps is nil) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}
	dtyMsg.(*tTIdty).SetNegotiatedCapabilities(pro.clientCaps)

	err = msgStmr.Push(ctx, dtyMsg)
	if err != nil {
		common.Odl.Warn("Push TTIDTY failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}

	err = msgStmr.Flush(ctx)
	if err != nil {
		common.Odl.Warn("Flush TTIDTY failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}

	// ttydty client caps is needed for typerep unmarshaling thus registering callback
	dtyCallBack := func(header *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		msg, err := msgfactory.GetMessage(TTIDTY)
		if err != nil {
			return nil, err
		}
		msg.(*tTIdty).SetNegotiatedCapabilities(pro.clientCaps)
		return msg, nil
	}
	msgStmr.(MessageStreamerInterface).RegisterPreUnmarshallCallback(TTIDTY, dtyCallBack)
	defer msgStmr.(MessageStreamerInterface).UnRegisterPreUnmarshallCallback(TTIDTY)
	msg, err := msgStmr.Pull(ctx, TTIDTY)
	if err != nil {
		common.Odl.Warn("msgStmr.Pull(TTIDTY) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err)
	}
	common.Odl.Fine("connectionNegotiator: TTIDTY negotiation message received", "msg_type", fmt.Sprintf("%T", msg))

	switch m := msg.(type) {
	case *tTIdty:
		common.Odl.Fine("connectionNegotiator: Negotiated datatype", "DTY", m)
		return m, nil
	default:
		common.Odl.Warn("Unexpected message type during TTIDTY", "message", m)
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}
}

// _negotiateProtocol negotiates the protocol (TTIPRO) and returbuf the negotiated *ttc.tTIpro.
func (cn *connectionNegotiator) _negotiateProtocol(
	ctx context.Context,
	msgStmr driverCommon.Streamer[driverCommon.MessageType],
	msgfactory driverCommon.Factory,
) (*tTIpro, error) {
	proMsg, err := msgfactory.GetMessage(TTIPRO)
	if err != nil {
		common.Odl.Warn("msgfactory.GetMessage(TTIPRO) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}
	common.Odl.Fine("connectionNegotiator: TTIPRO message created and sending")
	err = msgStmr.Push(ctx, proMsg.(driverCommon.Message[driverCommon.MessageType]))
	if err != nil {
		common.Odl.Warn("Push TTIPRO failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}

	err = msgStmr.Flush(ctx)
	if err != nil {
		common.Odl.Warn("Flush TTIPRO failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}

	return cn.receiveProtocolResponse(ctx, msgStmr)
}

// receiveProtocolResponse reads TTIPRO without sending a new protocol request.
// It returns an error if reading fails or the response has an unexpected type.
func (cn *connectionNegotiator) receiveProtocolResponse(ctx context.Context, msgStmr driverCommon.Streamer[driverCommon.MessageType]) (*tTIpro, error) {
	msg, err := msgStmr.Pull(ctx, TTIPRO)
	if err != nil {
		common.Odl.Warn("msgStmr.Pull(TTIPRO) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}
	common.Odl.Fine("connectionNegotiator: TTIPRO negotiation message received", "msg_type", fmt.Sprintf("%T", msg))

	switch m := msg.(type) {
	case *tTIpro:
		common.Odl.Fine("connectionNegotiator: Negotiated capabilities", "Caps", m.clientCaps)
		return m, nil
	default:
		common.Odl.Warn("Unexpected message type", "message", m)
		return nil, common.NewOracleError(oracleErrors.InternalError, err, nil)
	}
}
