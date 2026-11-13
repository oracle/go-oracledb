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

// The max password bytes length supported is 1024 bytes as per
// server-side code.
const maxPasswordBytes = 1024

// passwordAuthenticator authenticates using user/password O5 logon.
type passwordAuthenticator struct {
	*passwordAuthenticatorCommon
}

// newPasswordAuthenticator instantiates a new password authenticator.
// Parameters:
//   - username : the username
//   - password : user password
//   - connectString : connectionString
func newPasswordAuthenticator(username, password string, connectString string) *passwordAuthenticator {
	username, proxyClient := parseUsername(username)
	shared := &passwordAuthenticatorCommon{
		_connectString: connectString,
		_username:      driverCommon.StringToB1Array(username),
		_proxyClient:   driverCommon.StringToB1Array(proxyClient),
		_password:      driverCommon.StringToB1Array(sanitizeInputCredential(password)),
		_logonMode:     0,
	}

	return &passwordAuthenticator{
		passwordAuthenticatorCommon: shared,
	}
}

// Authenticate performs authentication.
func (pa *passwordAuthenticator) Authenticate(ctx context.Context) error {
	common.Odl.Fine("Authenticate start for user", "userName", pa._username)

	if err := pa.validatePasswordLength(); err != nil {
		return err
	}

	err := pa._doOSESSKEY(ctx)
	if err != nil {
		common.Odl.Warn("Authentication failed, osession keys exchange failed", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	err = pa._doOAuth(ctx)
	if err != nil {
		common.Odl.Warn("Authentication failed, oauth keys exchange failed", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	common.Odl.Fine("Authenticate ended")
	return nil
}

func (pa *passwordAuthenticator) _doOSESSKEY(ctx context.Context) error {
	common.Odl.Fine("_doOSESSKEY start")
	shelf := pa._shelf
	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)

	osesskey, err := pa.prepareOSESSKEY()
	if err != nil {
		return err
	}

	osesskeyRPACallBack := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		common.Odl.Fine("_doOSESSKEY inside osesskeyRPACallBack")
		msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oSesskey)
		if err != nil {
			common.Odl.Warn("Can't get osesskey RPA frnom factory", "error", err)
			return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		return msg, nil
	}
	streamer.RegisterPreUnmarshallCallback(TTIRPA, osesskeyRPACallBack)

	defer streamer.UnRegisterPreUnmarshallCallback(TTIRPA)

	err = streamer.Push(ctx, osesskey)
	if err != nil {
		common.Odl.Warn("Failed to push osesskey message to streamer", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	err = streamer.Flush(ctx)
	if err != nil {
		common.Odl.Warn("failed to flush streamer", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	common.Odl.Fine("_doOSESSKEY message streamer flushed")

	var osesskeyrpa *oSesskeyRPA
	oerFound := false
	for {
		common.Odl.Fine("_doOSESSKEY pulling message streamer flushed")
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN)
		if err != nil {
			common.Odl.Warn("Failed to pull authentication response from streamer", "error", err)
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			osesskeyrpa = msg.(*oSesskeyRPA)
			common.Odl.Fine("_doOSESSKEY RPA received", "OSESSKEY_RPA", osesskeyrpa)
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			err := ttioer.getError()
			if err != nil {
				return err
			}
			common.Odl.Fine("_doOSESSKEY TTIOER message received", "ErrorMessage", msg)
			oerFound = true
		case TTIWRN:
			logAuthenticationWarning(msg.(*tTIwrn))
		default:
			common.Odl.Warn("Unexpected message type during oSessionKey", "message", msg)
			return common.NewOracleError(oracleErrors.InternalError, err, nil)

		}
		if oerFound {
			break
		}
	}

	if err := shelf.checkCurrentState(ctx); err != nil {
		return err
	}

	if osesskeyrpa == nil {
		// we did not receive any RPA. defend ourselves against avoid malicious
		// attack that would send us an OER but no RPA.
		common.Odl.Warn("Did not received expected RPA yet")
		return common.NewOracleError(oracleErrors.InternalError, nil, nil)
	}

	pa._sessionContext.UpdateSessionProperties(osesskeyrpa.connectionValues)
	common.Odl.Fine("_doOSESSKEY ended")
	return nil

}

// handleOAuthResponse shared function among authenticators that handles
// sending the TTC OAUTH message and receiving the response.
func handleOAuthResponse(ctx context.Context, streamer MessageStreamerInterface, shelf *ttiShelf[driverCommon.MessageType]) (*OAuthRPA, error) {
	var oauthrpa *OAuthRPA = nil

	oauthRPACallBack := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		common.Odl.Fine("Inside oauthRPACallBack hdr", "msg", t.GetType())
		msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oauth)
		if err != nil {
			return nil, err
		}
		return msg, nil
	}
	streamer.RegisterPreUnmarshallCallback(TTIRPA, oauthRPACallBack)
	defer streamer.UnRegisterPreUnmarshallCallback(TTIRPA)

	goOn := true
	for goOn == true {
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN)
		if err != nil {
			common.Odl.Fine("Authenticator failed to pull server messages", "error", err)
			return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			oauthrpa = msg.(*OAuthRPA)
			common.Odl.Fine("Authenticator:", "oAuth-RPA", oauthrpa)
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
		default:
			common.Odl.Warn("Unexpected message type during oauthMsg", "message", msg)
			return nil, common.NewOracleError(oracleErrors.InternalError, err, nil)
		}
	}

	if err := shelf.checkCurrentState(ctx); err != nil {
		return nil, err
	}

	if oauthrpa == nil {
		// we did not receive any RPA. defend ourselves against avoid malicious
		// attack that would send us an OER but no RPA.
		common.Odl.Warn("Did not received expected RPA yet")
		return nil, common.NewOracleError(oracleErrors.InternalError, nil, nil)
	}

	common.Odl.Fine("Authenticator: processing RPA")
	return oauthrpa, nil
}
