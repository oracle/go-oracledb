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
	_dataBuffer driverCommon.DataBuffer
}

// newConnectionNegotiator creates a new instance of connectionNegotiator.
func newConnectionNegotiator() *connectionNegotiator {
	return &connectionNegotiator{}
}

// Negotiate performs the connection negotiation process over the provided DataBuffer.
func (cn *connectionNegotiator) Negotiate(ctx context.Context) (*driverCommon.SessionContext, *ttiShelf[driverCommon.MessageType], error) {
	var pro *tTIpro
	var dty *tTIdty

	common.Odl.Debug("connectionNegotiator: Negotiation start")
	// 1. Create Shelf & Session Context
	shelf, sessCtx := _createShelfAndSessionContext()

	// 2. Fetch new Marshaller & TTC Msg Streamer
	_createAndRegisterMarshaller(cn._dataBuffer, shelf, true)
	msgStmr := _createAndRegisterMessageStreamer(shelf)

	// 3. Create and register Initial MessageFactory
	msgfactory := _createAndRegisterMessageFactory(shelf, -1)

	// 4. Negotiate Protocol
	var err error
	pro, err = cn._negotiateProtocol(ctx, msgStmr, msgfactory)
	if err != nil {
		return nil, nil, err
	}

	// 5. Register negotiated server capabilities
	shelf.RegisterCapabilities(pro.serverCaps.toMap())

	// 6. Negotiate Datatype
	dty, err = cn._negotiateDatatype(ctx, msgStmr, msgfactory, pro)
	if err != nil {
		return nil, nil, err
	}

	// The driver character set is hard-coded to AL32UTF8 because the Go client currently negotiates
	// AL32UTF8 for cliRIN/cliROUT in TTIDTY and only supports that pairing.
	sessCtx.SetSessionCharacterSets(
		al32Utf8CharSet,
		pro.getNCharCharacterSet(),
	)

	// 7. Update Marshaller with Datatype Reps
	_createAndRegisterMarshaller(cn._dataBuffer, shelf, false)

	// Determine negotiated TTC version (minimum of client and server)
	clientVersion := dty.GetClientTTCVersion()
	serverVersion := shelf.GetCapabilities()[kpccapCtTtcFldVsn].Value
	negotiatedTTCVersion := serverVersion
	if clientVersion < serverVersion {
		negotiatedTTCVersion = clientVersion
	}
	common.Odl.Debug("Negotiated TTC version", "client version", clientVersion, "server version", serverVersion, "negotiated version", negotiatedTTCVersion)

	// set timezone version number in session context
	sessCtx.SetTimeZoneVersionNumber(dty.GetTimeZoneVersionNumber())

	// 7. Update Version based MessageFactory
	_ = _createAndRegisterMessageFactory(shelf, int8(negotiatedTTCVersion))

	common.Odl.Debug("connectionNegotiator: Negotiation complete", "SessionContext", sessCtx, "Shelf", shelf)

	_createAndRegisterCodecFactory(shelf, int8(negotiatedTTCVersion))

	// 8. Re-register messages with negotiated capabilities
	if shelf.GetCapabilities()[kpccapCtbTtc1Eocs].IsSet {
		RegisterOerWithCapability()
		RegisterSTAWithCapability()
		common.Odl.Debug("Replaced OER messages by messages with EOCS support")
	}

	common.Odl.Debug("connectionNegotiator: Negotiation complete", "SessionContext", sessCtx, "Shelf", shelf)
	return sessCtx, shelf, nil
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
	common.Odl.Debug("Start datatype negotiation")
	dtyMsg, err := msgfactory.GetMessage(TTIDTY)
	if err != nil {
		common.Odl.Warn("msgfactory.GetMessage(TTIDTY) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)

	}

	// If the server character set is not multibyte, set the TTCLXMCONV flag to indicate that character data should be treated as length-prefixed.
	if !isCharSetMultibyte(pro.svrCharSet) {
		typeRepresentationTable.SetFlags(TTCLXMCONV)
	}

	common.Odl.Debug("connectionNegotiator: TTIDTY message created and sending")
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
	common.Odl.Debug("connectionNegotiator: TTIDTY negotiation message received", "msg_type", fmt.Sprintf("%T", msg))

	switch m := msg.(type) {
	case *tTIdty:
		common.Odl.Debug("connectionNegotiator: Negotiated datatype", "DTY", m)
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
	common.Odl.Debug("connectionNegotiator: TTIPRO message created and sending")
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

	msg, err := msgStmr.Pull(ctx, TTIPRO)
	if err != nil {
		common.Odl.Warn("msgStmr.Pull(TTIPRO) failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.NegotiatorError, err, nil)
	}
	common.Odl.Debug("connectionNegotiator: TTIPRO negotiation message received", "msg_type", fmt.Sprintf("%T", msg))

	switch m := msg.(type) {
	case *tTIpro:
		common.Odl.Debug("connectionNegotiator: Negotiated capabilities", "Caps", m.clientCaps)
		return m, nil
	default:
		common.Odl.Warn("Unexpected message type", "message", m)
		return nil, common.NewOracleError(oracleErrors.InternalError, err, nil)
	}
}

// _createShelfAndSessionContext manages creation of Shelf and SessionContext.
func _createShelfAndSessionContext() (*ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext) {
	shelf := newShelf[driverCommon.MessageType]()
	common.Odl.Debug("connectionNegotiator: Shelf created")

	sessCtx := driverCommon.NewSessionContext()
	common.Odl.Debug("connectionNegotiator: SessionContext created")
	return shelf, sessCtx
}

// _createAndRegisterMessageStreamer creates a new message streamer and registers it to the shelf.
func _createAndRegisterMessageStreamer(
	shelf *ttiShelf[driverCommon.MessageType],
) *MessageStreamer {
	msgStmr := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(msgStmr)
	common.Odl.Debug("connectionNegotiator: MessageStreamer created and registered")
	return msgStmr
}

// _createAndRegisterMessageFactory creates a new message factory and registers it to the shelf.
func _createAndRegisterMessageFactory(
	shelf *ttiShelf[driverCommon.MessageType], version int8) driverCommon.Factory {
	msgfactory := NewMessageFactoryForProtocol(version)
	shelf.RegisterMessageFactory(msgfactory)
	common.Odl.Debug("connectionNegotiator: MessageFactory created and registered")
	return msgfactory
}

// _createAndRegisterCodecFactory creates a new codec factory and registers it to the shelf.
func _createAndRegisterCodecFactory(
	shelf *ttiShelf[driverCommon.MessageType], version int8) {
	codecFactory := NewCodecFactoryForProtocol(version)
	shelf.RegisterCodecFactory(codecFactory)
	common.Odl.Debug("connectionNegotiator: codecFactory created and registered")
}

// _createAndRegisterMarshaller creates a new marshaller and registers it to the shelf.
func _createAndRegisterMarshaller(
	buf driverCommon.DataBuffer,
	shelf *ttiShelf[driverCommon.MessageType],
	isNative bool,
) {
	var mar driverCommon.Marshaller
	if isNative {
		mar = NewNativeMarshalEngine(buf, driverCommon.BIG_ENDIAN)
		common.Odl.Debug("connectionNegotiator: Creating Native Marshaller")
	} else {
		mar = NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		common.Odl.Debug("connectionNegotiator: Creating TTC Marshaller")
	}
	if props := shelf.GetConnectionProperties(); props != nil {
		if marshalEngine, ok := mar.(*MarshalEngine); ok {
			marshalEngine.setDefaultLobPrefetchSize(int64(props.GetDefaultLobPrefetchSize()))
		}
	}
	shelf.RegisterMarshaller(mar)
	common.Odl.Debug("connectionNegotiator: Marshaller created/updated and registered")
}
