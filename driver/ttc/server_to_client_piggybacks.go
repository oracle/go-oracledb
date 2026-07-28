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
	"github.com/oracle/go-driver/driver/common"
)

// registerServerToClientPiggybacks registers server to client piggyback messages
// callback functions
func registerServerToClientPiggybacks(shelf *ttiShelf[common.MessageType], sessionContext *common.SessionContext) {
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
	shelf      *ttiShelf[common.MessageType]
	sessionCtx *common.SessionContext
}

// handleServerToClientPiggyback handles server to client piggyback messages.
func (sessionUpdater serverToClientPiggybackUpdater) handleServerToClientPiggyback(msg common.Message[common.MessageType], e error) (bool, error) {
	function, ok := msg.(common.Function)
	if !ok {
		return false, common.NewOracleError(common.SPFNotFunction, nil)
	}
	functionCode := function.GetFuncCode()
	switch functionCode {
	case common.FunctionType(ocssync):
		return sessionUpdater.updateSessionProperties(msg, e)
	default:
		return false, common.NewOracleError(common.UnknownSPFFunction, nil, functionCode)
	}

}

// updateSessionProperties handles OCSSYNC message. Updates session properties.
func (sessionUpdater serverToClientPiggybackUpdater) updateSessionProperties(msg common.Message[common.MessageType], e error) (bool, error) {
	if e != nil {
		return false, e
	}

	ttiSPFOCSSync, _ := msg.(*ttiSPFOCSSync)

	sessionUpdater.sessionCtx.UpdateSessionProperties(ttiSPFOCSSync.getKeyValueArr())

	return false, nil
}
