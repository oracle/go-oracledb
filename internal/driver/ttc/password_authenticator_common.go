package ttc

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// A proxy login consists of two identifiers, either unquoted or double quoted.
// Brackets within a quoted identifier are part of that identifier.
var proxyUsernamePattern = regexp.MustCompile(`^("[^"\x00]+"|[\p{L}\p{M}\p{N}\p{Pc}$#]+)\[("[^"\x00]+"|[\p{L}\p{M}\p{N}\p{Pc}$#]+)\]$`)

// parseUsername extracts the proxy client and sanitizes the authenticating username.
// Unrecognized syntax is treated as an ordinary username.
func parseUsername(username string) (string, string) {
	var proxyClient string
	// Parse before sanitizing to preserve quoted identifiers.
	if parts := proxyUsernamePattern.FindStringSubmatch(strings.TrimSpace(username)); parts != nil {
		username = parts[1]
		// Preserve quotes on the proxy client.
		proxyClient = parts[2]
	}
	return sanitizeInputCredential(username), proxyClient
}

// passwordAuthenticatorCommon holds credentials and authentication steps used
// by the normal and cookie password authenticators for one connection.
type passwordAuthenticatorCommon struct {
	_username                      driverCommon.B1Array
	_proxyClient                   driverCommon.B1Array
	_password                      driverCommon.B1Array
	_logonMode                     int64
	_shelf                         ttiShelf[driverCommon.MessageType]
	_serverCompileTimeVerifierType byte
	_sessionContext                *driverCommon.SessionContext
	_connectString                 string
	_logonHelper                   *o5Logon
}

// SetSessionContext sets the session context to use within the authenticator.
func (pa *passwordAuthenticatorCommon) SetSessionContext(sessCtx *driverCommon.SessionContext) {
	pa._sessionContext = sessCtx
}

// SetShelf sets the shelf to use within the authenticator.
func (pa *passwordAuthenticatorCommon) SetShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	pa._shelf = *shelf
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

func (pa *passwordAuthenticatorCommon) validatePasswordLength() error {
	if len(pa._password) > maxPasswordBytes {
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, "password length")
	}
	return nil
}

// prepareOSESSKEY builds the first password-authentication request without sending it.
// It returns an error if the message factory cannot create the request.
func (pa *passwordAuthenticatorCommon) prepareOSESSKEY() (driverCommon.Message[driverCommon.MessageType], error) {
	shelf := pa._shelf

	osesskey, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIFUN, oSesskey)
	if err != nil {
		common.Odl.Warn("Can't get osesskey message from factory", "error", err)
		return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	osesskey.(*oSessionKey).buildKeyValueList()
	osesskey.(*oSessionKey).setUser(pa._username)

	return osesskey, nil
}

func (pa *passwordAuthenticatorCommon) _doOAuth(ctx context.Context) error {
	common.Odl.Fine("Authenticator, starting oauthMsg")
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
	oauthMsg.(*oAuth).setProxyClient(pa._proxyClient)
	oauthMsg.(*oAuth).setConnectString(pa._connectString)
	oauthMsg.(*oAuth).setPBKDF2VgenCount(PBKDF2VgenCount)
	oauthMsg.(*oAuth).setPBKDF2SderCount(PBKDF2SderCount)
	oauthMsg.(*oAuth).setHasO5LNPSupport(shelf.GetCapabilities()[kztvovKpclogO5lNp].IsSet)
	oauthMsg.(*oAuth).setHasO7LMRSupport(shelf.GetCapabilities()[kztvovKpclogO7lMr].IsSet)
	oauthMsg.(*oAuth).setBUseO5Logon(sessionProperties.ContainsKey(authVFRData))
	oauthMsg.(*oAuth).setLogonMode(pa._logonMode)
	if common.KpzLogonSysdba.Enabled(pa._logonMode) && len(pa._password) == 0 {
		common.Odl.Fine("KpzLogonSysdba mode, skip _oSessionKeyInit of o response")
	} else {
		err = oauthMsg.(*oAuth)._initializeOAuthResponse(pa._password)
		if err != nil {
			common.Odl.Fine("Can't initialize oauthMsg response", "error", err)
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

	common.Odl.Fine("_doOAuth oatuh message pushed and flushed to the streamer")

	oauthrpa, err := handleOAuthResponse(ctx, streamer, &shelf)
	if err != nil {
		return err
	}

	pa._sessionContext.UpdateSessionProperties(oauthrpa.connectionValues)
	serverResponse := driverCommon.B1ArrayToString(pa._sessionContext.GetSessionProperties().
		GetProperty(authServerResponse).(driverCommon.B1Array))
	err = pa._logonHelper.ValidateServerIdentity(serverResponse)
	if err != nil {
		common.Odl.Warn("Error occurred while validating server response", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	common.Odl.Fine("Authenticator: oauthMsg ok")
	return nil
}

// sanitizeInputCredential removes spaces and leading and trailing double quotes from a string
//
// All the user names are converted to upper case except for those within double quotes.
// Returns:
//   - sanitized version of given credential
func sanitizeInputCredential(credential string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(credential, "\""), "\""))
}

func logAuthenticationWarning(warning *tTIwrn) {
	common.Odl.Warn("Authentication warning received",
		"warningNumber", warning.warningNumber,
		"warningMessage", warning.warningMessage,
		"warningFlags", warning.warningFlags)
}
