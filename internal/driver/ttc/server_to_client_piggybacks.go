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
	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// registerServerToClientPiggybacks registers server to client piggyback messages
// callback functions
func registerServerToClientPiggybacks(shelf *ttiShelf[driverCommon.MessageType], sessionContext *driverCommon.SessionContext) {
	messageStreamer := shelf.GetMessageStreamer().(MessageStreamerInterface)
	sessionUpdater := &serverToClientPiggybackUpdater{
		shelf:      shelf,
		sessionCtx: sessionContext,
	}
	messageStreamer.RegisterPostUnmarshallCallback(TTISPF, sessionUpdater.handleServerToClientPiggyback)
}

// sessionUpdater used for server to client piggyback message callback that
// updates session context
type serverToClientPiggybackUpdater struct {
	shelf      *ttiShelf[driverCommon.MessageType]
	sessionCtx *driverCommon.SessionContext
}

// handleServerToClientPiggyback handles server to client piggyback messages.
func (sessionUpdater serverToClientPiggybackUpdater) handleServerToClientPiggyback(msg driverCommon.Message[driverCommon.MessageType], e error) (bool, error) {
	function, ok := msg.(driverCommon.Function)
	if !ok {
		return false, common.NewOracleError(oracleErrors.SPFNotFunction, nil)
	}
	if e != nil {
		return false, e
	}
	functionCode := function.GetFuncCode()
	switch functionCode {
	case ocssync:
		return sessionUpdater.updateSessionSyncProperties(msg)
	case ocsessret:
		return sessionUpdater.updateSessionRetProperties(msg)
	case ocospid:
		return sessionUpdater.updateOsPID(msg)
	default:
		return false, common.NewOracleError(oracleErrors.UnknownSPFFunction, nil, functionCode)
	}

}

func (sessionUpdater serverToClientPiggybackUpdater) updateOsPID(msg driverCommon.Message[driverCommon.MessageType]) (bool, error) {
	common.Odl.Debug("got updateOsPID")
	//ttiSPFOSPID, _ := msg.(*ttiSPFOCOspid)
	return false, nil
}

// updateSessionSyncProperties handles OCSSYNC message. Updates session properties.
func (sessionUpdater serverToClientPiggybackUpdater) updateSessionSyncProperties(msg driverCommon.Message[driverCommon.MessageType]) (bool, error) {

	common.Odl.Debug("Update session sync properties")

	ttiSPFOCSSync, _ := msg.(*ttiSPFOCSSync)

	sessionUpdater.sessionCtx.UpdateSessionProperties(ttiSPFOCSSync.getKeyValueArr())

	return false, nil
}

// updateSessionSyncProperties handles OCSSYNC message. Updates session properties.
func (sessionUpdater serverToClientPiggybackUpdater) updateSessionRetProperties(msg driverCommon.Message[driverCommon.MessageType]) (bool, error) {
	ttiSPFOCSessret, _ := msg.(*ttiSPFOCSessret)
	props := driverCommon.NewProperties[string]()
	props.SetProperty(authSessionId, ttiSPFOCSessret.getSessretidx())
	props.SetProperty(authSessionSerial, ttiSPFOCSessret.getSessretser())
	common.Odl.Debug("ocsessret / session updated", authSessionId, ttiSPFOCSessret.getSessretidx(), authSessionSerial, ttiSPFOCSessret.getSessretser())
	sessionUpdater.sessionCtx.UpdateSessionProperties(props)

	return false, nil
}
