package ttc

import (
	"context"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type cookiePasswordAuthenticator struct {
	*passwordAuthenticatorCommon
}

// newCookiePasswordAuthenticator prepares password credentials for the cookie path.
func newCookiePasswordAuthenticator(username, password string, connectString string) *cookiePasswordAuthenticator {
	username, proxyClient := parseUsername(username)
	shared := &passwordAuthenticatorCommon{
		_connectString: connectString,
		_username:      driverCommon.StringToB1Array(username),
		_proxyClient:   driverCommon.StringToB1Array(proxyClient),
		_password:      driverCommon.StringToB1Array(sanitizeInputCredential(password)),
		_logonMode:     0,
	}

	return &cookiePasswordAuthenticator{
		passwordAuthenticatorCommon: shared,
	}
}

// newCookiePasswordAuthenticatorWithLogonMode creates a cookie password authenticator with a logon mode.
// Parameters:
//   - username : the username
//   - password : user password
//   - connectString : connectionString
//   - mode : logonMode
func newCookiePasswordAuthenticatorWithLogonMode(username, password string, mode common.LogonMode, connectString string) *cookiePasswordAuthenticator {
	pa := newCookiePasswordAuthenticator(username, password, connectString)
	pa._logonMode = mode.Value()
	return pa
}

// Authenticate combines cookie negotiation with OSESSKEY, then completes password OAUTH.
// It returns authentication errors or ErrTTIRENEG when normal fallback is required.
func (pa *cookiePasswordAuthenticator) Authenticate(ctx context.Context) error {
	common.Odl.Fine("Authenticate start for user", "userName", pa._username)
	shelf := pa._shelf

	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)

	if err := pa.validatePasswordLength(); err != nil {
		return err
	}

	osesskey, err := pa.prepareOSESSKEY()
	if err != nil {
		return err
	}

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

	response, err := readCookieAuthenticationResponse(ctx, &shelf, streamer, oSesskey)
	if err != nil {
		return err
	}

	osesskeyrpa := response.(*oSesskeyRPA)
	pa._sessionContext.UpdateSessionProperties(osesskeyrpa.connectionValues)

	err = pa._doOAuth(ctx)
	if err != nil {
		common.Odl.Warn("Authentication failed, oauth keys exchange failed", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	common.Odl.Fine("cookiePasswordAuthenticator: Authentication complete")
	return nil
}
