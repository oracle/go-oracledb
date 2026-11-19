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

const ttiInitAssumedFieldVersion byte = 23

// newTTIInitProtocol prepares the provisional settings used before server PRO arrives.
func newTTIInitProtocol() *tTIpro {
	pro := newTTIpro().(*tTIpro)
	fieldVersion := pro.serverCaps.knownUsedCompileTimeCapabilities[kpccapCtTtcFldVsn]
	pro.serverCaps.compileTimeCapabilities[fieldVersion.index] = ttiInitAssumedFieldVersion
	pro.clientCaps.adjustCapabilityFrom(pro.serverCaps)
	return pro
}

// Negotiation responses are consumed later, during authentication.
type ttiInitConnectionNegotiator struct {
	*connectionNegotiatorCommon
	_negotiationComplete bool
}

func newTTIInitConnectionNegotiator(buffer driverCommon.DataBuffer) *ttiInitConnectionNegotiator {
	return &ttiInitConnectionNegotiator{
		connectionNegotiatorCommon: &connectionNegotiatorCommon{_dataBuffer: buffer},
	}
}

// Negotiate queues TTIINIT and TTIDTY without flushing. Authentication appends
// its first request and sends the combined batch.
func (n *ttiInitConnectionNegotiator) Negotiate(ctx context.Context) (*driverCommon.SessionContext, *ttiShelf[driverCommon.MessageType], error) {
	n._negotiationComplete = false
	shelf, sessCtx, streamer, _ := n.initializeNegotiationResources()
	pro := newTTIInitProtocol()
	shelf.RegisterCapabilities(pro.serverCaps.toMap())
	dty := NewTTIdty().(*tTIdty)
	dty.SetNegotiatedCapabilities(pro.clientCaps)
	n.applyNegotiatedSettings(shelf, sessCtx, pro, dty)

	initMsg := &ttiInitMessage{
		_protocolMessage: pro,
		_fieldVersion:    ttiInitAssumedFieldVersion,
	}
	if err := streamer.Push(ctx, initMsg); err != nil {
		return nil, nil, err
	}
	if err := streamer.Push(ctx, dty); err != nil {
		return nil, nil, err
	}
	common.Odl.Fine("ttiInitConnectionNegotiator: TTIINIT and TTIDTY queued for the first authentication request")

	// Responses arrive during authentication. The instantiator removes these
	// callbacks when connection setup ends, including errors and fallback.
	n.registerTTIInitNegotiationCallbacks(streamer, shelf, sessCtx, dty)
	return sessCtx, shelf, nil
}

func (n *ttiInitConnectionNegotiator) registerTTIInitNegotiationCallbacks(
	streamer MessageStreamerInterface,
	shelf *ttiShelf[driverCommon.MessageType],
	sessCtx *driverCommon.SessionContext,
	dty *tTIdty,
) {
	var pro *tTIpro
	receiveProtocolResponse := func(msg driverCommon.Message[driverCommon.MessageType], decodeErr error) (bool, error) {
		if decodeErr != nil {
			return true, decodeErr
		}
		if pro != nil {
			return true, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		pro = msg.(*tTIpro)
		shelf.RegisterCapabilities(pro.serverCaps.toMap())
		return true, nil
	}
	prepareDatatypeResponse := func(_ *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		if pro == nil {
			return nil, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		// Reuse the request's DTY: its capabilities determine which response fields to read.
		return dty, nil
	}
	completeNegotiation := func(msg driverCommon.Message[driverCommon.MessageType], decodeErr error) (bool, error) {
		if decodeErr != nil {
			return true, decodeErr
		}
		if pro == nil || n._negotiationComplete {
			return true, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		response := msg.(*tTIdty)
		n.applyNegotiatedSettings(shelf, sessCtx, pro, response)
		sessCtx.SetTimeZoneVersionNumber(response.GetTimeZoneVersionNumber())
		n._negotiationComplete = true
		common.Odl.Fine("ttiInitConnectionNegotiator: Negotiation complete", "SessionContext", sessCtx, "Shelf", shelf)
		return true, nil
	}
	streamer.RegisterPostUnmarshallCallback(TTIPRO, receiveProtocolResponse)
	streamer.RegisterPreUnmarshallCallback(TTIDTY, prepareDatatypeResponse)
	streamer.RegisterPostUnmarshallCallback(TTIDTY, completeNegotiation)
}

// checkNegotiationComplete prevents returning a connection with provisional settings.
func (n *ttiInitConnectionNegotiator) checkNegotiationComplete() error {
	if !n._negotiationComplete {
		return common.NewOracleError(oracleErrors.NegotiatorError, nil)
	}
	return nil
}

func unregisterTTIInitNegotiationCallbacks(streamer MessageStreamerInterface) {
	streamer.UnRegisterPostUnmarshallCallback(TTIPRO)
	streamer.UnRegisterPreUnmarshallCallback(TTIDTY)
	streamer.UnRegisterPostUnmarshallCallback(TTIDTY)
}
