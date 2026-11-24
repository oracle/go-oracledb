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

// readTTIInitAuthenticationResponse reads the combined negotiation and first
// authentication response. TTIRENEG requests fallback; it does not retry here.
func readTTIInitAuthenticationResponse(
	ctx context.Context,
	shelf *ttiShelf[driverCommon.MessageType],
	streamer MessageStreamerInterface,
	funcType driverCommon.FunctionType,
) (driverCommon.Message[driverCommon.MessageType], error) {
	var rpa driverCommon.Message[driverCommon.MessageType]

	createAuthenticationRPA := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		common.Odl.Fine("TTIINIT authentication: Creating RPA response", "function", funcType, "messageType", t.GetType())
		msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, funcType)
		if err != nil {
			return nil, err
		}
		return msg, nil
	}

	streamer.RegisterPreUnmarshallCallback(TTIRPA, createAuthenticationRPA)
	defer streamer.UnRegisterPreUnmarshallCallback(TTIRPA)

	goOn := true
	for goOn {
		// The callbacks discard successful PRO and DTY replies. Keep both types
		// in the list so Pull returns their decode errors immediately.
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN, TTIPRO, TTIRENEG, TTIDTY)
		if err != nil {
			common.Odl.Fine("Authenticator failed to pull server messages", "error", err)
			return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			rpa = msg
			common.Odl.Fine("TTIINIT authentication: RPA received", "function", funcType)
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			err := ttioer.getError()
			if err != nil {
				return nil, err
			}
			common.Odl.Fine("Authenticator: TTIOER message received", "ErrorMessage", ttioer)
			goOn = false
		case TTIWRN:
			logAuthenticationWarning(msg.(*tTIwrn))
		case TTIRENEG:
			common.Odl.Fine("TTIINIT authentication: Server requested TTC renegotiation", "function", funcType)
			return nil, driverCommon.ErrTTIRENEG
		default:
			common.Odl.Warn("Unexpected message type during TTIINIT authentication", "function", funcType, "messageType", msg.GetMsgCode())
			return nil, common.NewOracleError(oracleErrors.InternalError, err, nil)
		}
	}

	if err := shelf.checkCurrentState(ctx); err != nil {
		return nil, err
	}

	if rpa == nil {
		// we did not receive any RPA. defend ourselves against avoid malicious
		// attack that would send us an OER but no RPA.
		common.Odl.Warn("Did not received expected RPA yet")
		return nil, common.NewOracleError(oracleErrors.InternalError, nil, nil)
	}

	common.Odl.Fine("Authenticator: processing RPA")
	return rpa, nil
}
