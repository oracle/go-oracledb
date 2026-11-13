package ttc

import (
	"context"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

type cookieTokenAuthenticator struct {
	*tokenAuthenticatorCommon
}

// newCookieTokenAuthenticator prepares the cookie path using the supplied token provider.
func newCookieTokenAuthenticator(tokenProvider oracleProviders.TokenAuthenticationProvider) *cookieTokenAuthenticator {
	shared := &tokenAuthenticatorCommon{
		tokenProvider: tokenProvider,
	}
	return &cookieTokenAuthenticator{
		tokenAuthenticatorCommon: shared,
	}
}

// Authenticate combines cookie negotiation with token OAUTH.
// It returns authentication errors or ErrTTIRENEG when normal fallback is required.
func (ta *cookieTokenAuthenticator) Authenticate(ctx context.Context) error {
	common.Odl.Debug("Start TOKEN authentication")

	msg, err := ta.prepareOAuth(ctx)
	if err != nil {
		return err
	}

	shelf := ta.shelf
	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)
	// send oauth message
	if err := streamer.Push(ctx, msg); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err := streamer.Flush(ctx); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	response, err := readCookieAuthenticationResponse(ctx, &shelf, streamer, oauth)
	if err != nil {
		return err
	}
	oauthrpa := response.(*OAuthRPA)
	ta.sessionContext.UpdateSessionProperties(oauthrpa.connectionValues)

	return nil
}
