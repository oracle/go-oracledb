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
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// The max password bytes length supported is 1024 bytes as per
// server-side code.
const maxPasswordBytes = 1024

// passwordAuthenticator authenticates using user/password O5 logon.
type passwordAuthenticator struct {
	_username                      driverCommon.B1Array
	_password                      driverCommon.B1Array
	_logonMode                     int64
	_shelf                         ttiShelf[driverCommon.MessageType]
	_serverCompileTimeVerifierType byte
	_sessionContext                *driverCommon.SessionContext
	_connectString                 string
	_logonHelper                   *o5Logon
}

// newPasswordAuthenticator instantiates a new password authenticator.
// Parameters:
//   - username : the username
//   - password : user password
//   - connectString : connectionString
func newPasswordAuthenticator(username, password string, connectString string) *passwordAuthenticator {
	pa := passwordAuthenticator{
		_connectString: connectString,
		_username:      driverCommon.StringToB1Array(sanitizeInputCredential(username)),
		_password:      driverCommon.StringToB1Array(sanitizeInputCredential(password)),
		_logonMode:     0,
	}
	return &pa
}

// newPasswordAuthenticatorWithLogonMode instantiates a new password authenticator.
// Parameters:
//   - username : the username
//   - password : user password
//   - connectString : connectionString
//   - mode : logonMode
func newPasswordAuthenticatorWithLogonMode(username, password string, mode common.LogonMode, connectString string) *passwordAuthenticator {
	pa := newPasswordAuthenticator(username, password, connectString)
	pa._logonMode = mode.Value()
	return pa
}

// Authenticate performs authentication.
func (pa *passwordAuthenticator) Authenticate(ctx context.Context) error {
	common.Odl.Debug("Authenticate start for user", "userName", pa._username)

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
	common.Odl.Debug("Authenticate ended")
	return nil
}

func (pa *passwordAuthenticator) validatePasswordLength() error {
	if len(pa._password) > maxPasswordBytes {
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, "password length")
	}
	return nil
}

// SetShelf sets the shelf to use within the authenticator.
func (pa *passwordAuthenticator) SetShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	pa._shelf = *shelf
}

// SetSessionContext sets the session context to use within the authenticator.
func (pa *passwordAuthenticator) SetSessionContext(sessCtx *driverCommon.SessionContext) {
	pa._sessionContext = sessCtx
}

func (pa *passwordAuthenticator) _doOSESSKEY(ctx context.Context) error {
	common.Odl.Debug("_doOSESSKEY start")
	shelf := pa._shelf
	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)
	osesskey, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIFUN, oSesskey)
	if err != nil {
		common.Odl.Warn("Can't get osesskey message from factory", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	osesskey.(*oSessionKey).buildKeyValueList()
	osesskey.(*oSessionKey).setUser(pa._username)

	osesskeyRPACallBack := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		common.Odl.Debug("_doOSESSKEY inside osesskeyRPACallBack")
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

	common.Odl.Debug("_doOSESSKEY message streamer flushed")

	var osesskeyrpa *oSesskeyRPA
	oerFound := false
	for {
		common.Odl.Debug("_doOSESSKEY pulling message streamer flushed")
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN)
		if err != nil {
			common.Odl.Warn("Failed to pull authentication response from streamer", "error", err)
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			osesskeyrpa = msg.(*oSesskeyRPA)
			common.Odl.Debug("_doOSESSKEY RPA received", "OSESSKEY_RPA", osesskeyrpa)
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			err := ttioer.getError()
			if err != nil {
				return err
			}
			common.Odl.Debug("_doOSESSKEY TTIOER message received", "ErrorMessage", msg)
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

	// no message should be left at this point
	msgIn, _ := streamer.Drain(ctx, driverCommon.IN)
	// no message should be left at this point
	if msgIn != 0 {
		// should drop connection.
		common.Odl.Warn("messaging service not drained after RPA")
		shelf.getEventService().post(streamerStaleEvent)
		return shelf.LocalizeError(common.NewOracleError(oracleErrors.InternalError, nil))
	}

	if osesskeyrpa == nil {
		// we did not receive any RPA. defend ourselves against avoid malicious
		// attack that would send us an OER but no RPA.
		common.Odl.Warn("Did not received expected RPA yet")
		return common.NewOracleError(oracleErrors.InternalError, nil, nil)
	}

	pa._sessionContext.UpdateSessionProperties(osesskeyrpa.connectionValues)
	common.Odl.Debug("_doOSESSKEY ended")
	return nil

}

func (pa *passwordAuthenticator) _doOAuth(ctx context.Context) error {
	common.Odl.Debug("Authenticator, starting oauthMsg")
	shelf := pa._shelf
	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)

	oauthMsg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIFUN, oauth)
	if err != nil {
		common.Odl.Warn("Can't get oauthMsg message from factory", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	sessionProperties := pa._sessionContext.GetSessionProperties()

	oauthMsg.(*oAuth).setEncryptedSK(sessionProperties.GetProperty(authSesskey).(*driverCommon.KeyValue).Value)
	oauthMsg.(*oAuth).setSalt(sessionProperties.GetProperty(authVFRData).(*driverCommon.KeyValue).Value)

	err = oauthMsg.(*oAuth).setVerifierType(int(sessionProperties.GetProperty(authVFRData).(*driverCommon.KeyValue).Flag))
	if err != nil {
		common.Odl.Warn("OauthMsg preparation failed", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	PBKDF2VgenCount, err := strconv.Atoi(string(sessionProperties.GetProperty(authPbkdf2VgenCount).(*driverCommon.KeyValue).Value))
	if err != nil {
		return err
	}
	if !isValidPBKDF2VgenCount(PBKDF2VgenCount) {
		common.Odl.Warn("PBKDF2VgenCount outside allowed range",
			"min", minimumPBKDF2VgenCount,
			"max", maximumPBKDF2VgenCount,
			"count", PBKDF2VgenCount)
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, "invalid PBKDF2VgenCount")
	}

	PBKDF2SderCount, err := strconv.Atoi(driverCommon.B1ArrayToString(sessionProperties.GetProperty(authPbkdf2SderCount).(*driverCommon.KeyValue).Value))
	if err != nil {
		return err
	}
	if !isValidPBKDF2SderCount(PBKDF2SderCount) {
		common.Odl.Warn("PBKDF2SderCount outside allowed range",
			"min", minimumPBKDF2SderCount,
			"max", maximumPBKDF2SderCount,
			"count", PBKDF2SderCount)
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, "invalid PBKDF2SderCount")
	}

	oauthMsg.(*oAuth).setPBKDF2Salt(sessionProperties.GetProperty(authPDKDF2CSKSalt).(*driverCommon.KeyValue).Value)
	oauthMsg.(*oAuth).setUser(pa._username)
	oauthMsg.(*oAuth).setConnectString(pa._connectString)
	oauthMsg.(*oAuth).setPBKDF2VgenCount(PBKDF2VgenCount)
	oauthMsg.(*oAuth).setPBKDF2SderCount(PBKDF2SderCount)
	oauthMsg.(*oAuth).setHasO5LNPSupport(shelf.GetCapabilities()[kztvovKpclogO5lNp].IsSet)
	oauthMsg.(*oAuth).setHasO7LMRSupport(shelf.GetCapabilities()[kztvovKpclogO7lMr].IsSet)
	oauthMsg.(*oAuth).setBUseO5Logon(sessionProperties.ContainsKey(authVFRData))
	oauthMsg.(*oAuth).setLogonMode(pa._logonMode)
	if common.KpzLogonSysdba.Enabled(pa._logonMode) && len(pa._password) == 0 {
		common.Odl.Debug("KpzLogonSysdba mode, skip _oSessionKeyInit of o response")
	} else {
		err = oauthMsg.(*oAuth)._initializeOAuthResponse(pa._password)
		if err != nil {
			common.Odl.Debug("Can't initialize oauthMsg response", "error", err)
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
	}

	pa._logonHelper = oauthMsg.(*oAuth).getLogonHelper()

	err = streamer.Push(ctx, oauthMsg)
	if err != nil {
		common.Odl.Warn("Authenticator, failed to push oauthMsg message", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	err = streamer.Flush(ctx)
	if err != nil {
		common.Odl.Warn("Authenticator, failed to flush streamer", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	common.Odl.Debug("_doOAuth oatuh message pushed and flushed to the streamer")

	oauthRPACallBack := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		common.Odl.Debug("Inside oauthRPACallBack hdr", "msg", t.GetType())
		msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oauth)
		if err != nil {
			return nil, err
		}
		return msg, nil
	}
	streamer.RegisterPreUnmarshallCallback(TTIRPA, oauthRPACallBack)
	defer streamer.UnRegisterPreUnmarshallCallback(TTIRPA)

	oerCallback := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		common.Odl.Debug("Inside oerCallback hdr", "msg", t.GetType())
		msg, err := shelf.GetMessageFactory().(Factory).GetMessage(TTIOER)
		if err != nil {
			return nil, err
		}
		return msg, nil
	}
	streamer.RegisterPreUnmarshallCallback(TTIOER, oerCallback)
	defer streamer.UnRegisterPreUnmarshallCallback(TTIOER)

	var oauthrpa *OAuthRPA = nil
	goOn := true
	for goOn == true {
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN)
		if err != nil {
			common.Odl.Debug("Authenticator failed to pull server messages", "error", err)
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			oauthrpa = msg.(*OAuthRPA)
			common.Odl.Debug("Authenticator:", "oAuth-RPA", oauthrpa)
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			err := ttioer.getError()
			if err != nil {
				return err
			}
			common.Odl.Debug("Authenticator: TTIOER message received", "ErrorMessage", ttioer)
			goOn = false
		case TTIWRN:
			logAuthenticationWarning(msg.(*tTIwrn))
		default:
			common.Odl.Warn("Unexpected message type during oauthMsg", "message", msg)
			return common.NewOracleError(oracleErrors.InternalError, err, nil)
		}
	}

	msgIn, _ := streamer.Drain(ctx, driverCommon.IN)
	// no message should be left at this point
	if msgIn != 0 {
		// should drop connection.
		common.Odl.Warn("messaging service not drained after query")
		shelf.getEventService().post(streamerStaleEvent)
		return shelf.LocalizeError(common.NewOracleError(oracleErrors.InternalError, nil))
	}

	if oauthrpa == nil {
		// we did not receive any RPA. defend ourselves against avoid malicious
		// attack that would send us an OER but no RPA.
		common.Odl.Warn("Did not received expected RPA yet")
		return common.NewOracleError(oracleErrors.InternalError, nil, nil)
	}

	common.Odl.Debug("Authenticator: processing RPA")

	pa._sessionContext.UpdateSessionProperties(oauthrpa.connectionValues)
	serverResponse := driverCommon.B1ArrayToString(pa._sessionContext.GetSessionProperties().
		GetProperty(authServerResponse).(driverCommon.B1Array))
	err = pa._logonHelper.ValidateServerIdentity(serverResponse)
	if err != nil {
		common.Odl.Warn("Error occurred while validating server response", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	common.Odl.Debug("Authenticator: oauthMsg ok")
	return nil
}

func logAuthenticationWarning(warning *tTIwrn) {
	common.Odl.Warn("Authentication warning received",
		"warningNumber", warning.warningNumber,
		"warningMessage", warning.warningMessage,
		"warningFlags", warning.warningFlags)
}

// sanitizeInputCredential removes spaces and leading and trailing double quotes from a string
//
// All the user names are converted to upper case except for those within double quotes.
// Returns:
//   - sanitized version of given credential
func sanitizeInputCredential(credential string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(credential, "\""), "\""))
}
