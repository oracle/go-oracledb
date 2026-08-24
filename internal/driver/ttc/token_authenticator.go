/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

type tokenAuthenticator struct {
	shelf          ttiShelf[driverCommon.MessageType]
	sessionContext *driverCommon.SessionContext
	tokenProvider  oracleProviders.TokenAuthenticationProvider
	connectString  string
}

// newTokenAuthenticator creates a token authenticator backed by tokenProvider.
//
// Parameters:
//   - tokenProvider: the provider selected for this connection attempt.
//   - connectString: the connect descriptor used to derive token header fields.
//
// Returns:
//   - the configured token authenticator.
func newTokenAuthenticator(tokenProvider oracleProviders.TokenAuthenticationProvider, connectString string) *tokenAuthenticator {
	return &tokenAuthenticator{
		tokenProvider: tokenProvider,
		connectString: connectString,
	}
}

// findFirstTokenAuthenticatorProvider returns the first provider in the
// registry that implements token authentication.
//
// Parameters:
//   - providerRegistry: the providers available for the connection attempt.
//
// Returns:
//   - the first token authentication provider found in the registry.
//   - nil when no token authentication provider is registered.
func findFirstTokenAuthenticatorProvider(providerRegistry []oracleProviders.Provider) oracleProviders.TokenAuthenticationProvider {
	for _, provider := range providerRegistry {
		if tokenProvider, ok := provider.(oracleProviders.TokenAuthenticationProvider); ok {
			return tokenProvider
		}
	}
	return nil
}

// SetShelf stores the TTC shelf used to create and exchange authentication
// messages.
//
// Parameters:
//   - shelf: the TTC shelf bound to the current connection attempt.
func (ta *tokenAuthenticator) SetShelf(shelf *ttiShelf[driverCommon.MessageType]) {
	ta.shelf = *shelf
}

// SetSessionContext stores the session context used during token
// authentication.
//
// Parameters:
//   - sessCtx: the session context associated with the current connection attempt.
func (ta *tokenAuthenticator) SetSessionContext(sessCtx *driverCommon.SessionContext) {
	ta.sessionContext = sessCtx
}

// Authenticate performs token-based authentication over TTC and updates the
// session context with the server response.
//
// Parameters:
//   - ctx: the context controlling message exchange, cancellation, and timeout.
//
// Returns:
//   - nil when authentication succeeds.
//   - an error if token resolution, signing, message exchange, or server validation fails.
func (ta *tokenAuthenticator) Authenticate(ctx context.Context) error {
	common.Odl.Debug("Start TOKEN authentication")
	if ta.tokenProvider == nil {
		return common.NewOracleError(oracleErrors.NoAuthenticatorError, nil)
	}
	if ta.sessionContext == nil {
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// Validate token
	token, err := ta.tokenProvider.Token(ctx)
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err := validateJWTExpiration(token); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	// Generate header and sign it if expected
	tokenHeader, err := ta.generateTokenHeader()
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	signature, err := ta.signHeader(ctx, tokenHeader)
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	shelf := ta.shelf
	streamer := shelf.GetMessageStreamer().(MessageStreamerInterface)

	msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIFUN, oauth)
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	// build oauth message
	oauthMsg := msg.(*oAuth)
	oauthMsg.setConnectString(ta.connectString)
	oauthMsg.setLogonMode(logonMode(ta.tokenProvider))
	oauthMsg.prepareForTokenOAUTH(driverCommon.B1Array{})
	oauthMsg.setTokenKeyValsForOAUTH(token, tokenHeader, signature)

	// send oauth message
	if err := streamer.Push(ctx, msg); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err := streamer.Flush(ctx); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	// prepare to receive response
	oauthRPACallBack := func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oauth)
	}
	streamer.RegisterPreUnmarshallCallback(TTIRPA, oauthRPACallBack)
	defer streamer.UnRegisterPreUnmarshallCallback(TTIRPA)

	// fetch response
	var oauthrpa *OAuthRPA
	for {
		msg, err := streamer.Pull(ctx, TTIRPA, TTIOER, TTIWRN)
		if err != nil {
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}
		switch msg.GetMsgCode() {
		case TTIRPA:
			oauthrpa = msg.(*OAuthRPA)
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			if err := ttioer.getError(); err != nil {
				return err
			}
			if oauthrpa == nil {
				return common.NewOracleError(oracleErrors.InternalError, nil, nil)
			}
			ta.sessionContext.UpdateSessionProperties(oauthrpa.connectionValues)
			return nil
		case TTIWRN:
			logAuthenticationWarning(msg.(*tTIwrn))
		default:
			return common.NewOracleError(oracleErrors.InternalError, nil, nil)
		}
	}
}

// expectsHeader reports whether the provider requires OCI-style signed token
// header fields.
//
// Parameters:
//   - tokenProvider: the token provider being used for authentication.
//
// Returns:
//   - true when tokenProvider implements OCITokenAuthenticationProvider.
//   - false otherwise.
func expectsHeader(tokenProvider oracleProviders.TokenAuthenticationProvider) bool {
	_, ok := tokenProvider.(oracleProviders.OCITokenAuthenticationProvider)
	return ok
}

// logonMode returns the OAUTH logon mode bits for the supplied token provider.
//
// Parameters:
//   - tokenProvider: the token provider being used for authentication.
//
// Returns:
//   - the logon mode flags required for the provider type.
func logonMode(tokenProvider oracleProviders.TokenAuthenticationProvider) int64 {
	if expectsHeader(tokenProvider) {
		return common.KpzLogon.Value() | common.KpzLogonToken.Value()
	} else {
		return common.KpzLogon.Value()
	}
}

// generateTokenHeader builds the signed OCI token header when the active
// provider requires one.
//
// Parameters:
//   - none.
//
// Returns:
//   - the generated token header, or an empty string when no header is required.
//   - an error if required session or connect descriptor values cannot be derived.
func (ta *tokenAuthenticator) generateTokenHeader() (string, error) {
	if expectsHeader(ta.tokenProvider) {
		serviceName, err := extractServiceName(ta.connectString)
		if err != nil {
			return "", err
		}
		remoteAddr, err := getRequiredSessionProperty(ta.sessionContext, "REMOTE_ADDRESS")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"date: %s\n(request-target): %s\nhost: %s",
			time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"),
			serviceName,
			remoteAddr,
		), nil
	}
	return "", nil
}

// signHeader signs the OCI token header when the active provider supports
// private-key signing.
//
// Parameters:
//   - ctx: the context controlling private key retrieval.
//   - header: the token header payload to sign.
//
// Returns:
//   - the base64-encoded signature, or an empty string when signing is not required.
//   - an error if key retrieval, parsing, or signing fails.
func (ta *tokenAuthenticator) signHeader(ctx context.Context, header string) (string, error) {
	if expectsHeader(ta.tokenProvider) && len(header) > 0 {
		keyPEM, err := ta.tokenProvider.(oracleProviders.OCITokenAuthenticationProvider).PrivateKey(ctx)
		if err != nil {
			return "", err
		}
		signer, err := getSigner(keyPEM)
		if err != nil {
			return "", err
		}
		signature, err := signTokenHeader(header, signer)
		if err != nil {
			return "", err
		}
		return signature, nil
	}
	return "", nil
}

// getRequiredSessionProperty returns a non-empty string property from the
// session context.
//
// Parameters:
//   - sessionContext: the session context holding the property snapshot.
//   - key: the property name to retrieve.
//
// Returns:
//   - the trimmed string property value.
//   - an error if the property is missing, not a string, or empty.
func getRequiredSessionProperty(sessionContext *driverCommon.SessionContext, key string) (string, error) {
	value, ok := sessionContext.GetSessionProperties().GetProperty(key).(string)
	if !ok {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	return value, nil
}

// extractServiceName extracts the SERVICE_NAME value from a connect descriptor.
//
// Parameters:
//   - connectString: the connect descriptor to inspect.
//
// Returns:
//   - the extracted service name.
//   - an error if SERVICE_NAME cannot be found or parsed.
func extractServiceName(connectString string) (string, error) {
	common.Odl.Debug("ConnectionString", "connectString", connectString)
	return extractAddressValue(connectString, "SERVICE_NAME")
}

// extractAddressValue extracts a descriptor value identified by key from the
// supplied connect descriptor.
//
// Parameters:
//   - connectString: the connect descriptor to inspect.
//   - key: the descriptor key to locate.
//
// Returns:
//   - the trimmed value associated with key.
//   - an error if key cannot be found or its value cannot be parsed.
func extractAddressValue(connectString, key string) (string, error) {
	upper := strings.ToUpper(connectString)
	idx := strings.Index(upper, key+"=")
	if idx == -1 {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	start := idx + len(key) + 1
	end := strings.IndexAny(connectString[start:], ")")
	if end == -1 {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	return strings.TrimSpace(connectString[start : start+end]), nil
}

// getSigner parses a PEM-encoded PKCS#8 RSA private key into a crypto.Signer.
//
// Parameters:
//   - keyPEM: the PEM-encoded private key bytes.
//
// Returns:
//   - the parsed crypto.Signer implementation.
//   - an error if keyPEM is missing, malformed, or not an RSA signing key.
func getSigner(keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, common.NewOracleError(oracleErrors.InvalidPrivateKey, nil)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidPrivateKey, nil)
	}
	if _, ok := signer.(*rsa.PrivateKey); !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidPrivateKey, nil)
	}
	return signer, nil
}

// validateJWTExpiration checks the JWT exp claim when the token payload is a
// decodable JWT.
//
// Parameters:
//   - token: the token string to inspect.
//
// Returns:
//   - nil when the token is not a decodable JWT or when it is not expired.
//   - an error if the JWT contains an exp claim that is already in the past.
func validateJWTExpiration(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	// The token and decoded claims are credentials and must not be logged.
	var claims struct {
		Exp *int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
		return nil
	}
	if time.Unix(*claims.Exp, 0).Before(time.Now()) {
		return common.NewOracleError(oracleErrors.ExpiredToken, nil)
	}
	return nil
}

// signTokenHeader signs the supplied header using SHA-256 and returns the
// base64-encoded signature.
//
// Parameters:
//   - header: the header payload to sign.
//   - signer: the private key signer used to produce the signature.
//
// Returns:
//   - the base64-encoded signature for header.
//   - an error if the signing operation fails.
func signTokenHeader(header string, signer crypto.Signer) (string, error) {
	sum := sha256.Sum256([]byte(header))
	signature, err := signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}
