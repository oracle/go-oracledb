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
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"strconv"
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
}

// newTokenAuthenticator creates a token authenticator backed by the first
// token authentication provider found in the supplied provider registry.
//
// Parameters:
//   - providerRegistry: the providers available for this connection attempt.
//   - connectString: the connect descriptor used to derive token header fields.
//
// Returns:
//   - the configured token authenticator.
func newTokenAuthenticator(tokenProvider oracleProviders.TokenAuthenticationProvider) *tokenAuthenticator {
	return &tokenAuthenticator{
		tokenProvider: tokenProvider,
	}
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
	if len(token) == 0 {
		return common.NewOracleError(oracleErrors.EmptyTokenError, nil)
	}
	if err := validateJWTExpiration(token); err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	// Generate header and sign it if expected
	tokenHeader, err := ta.generateTokenHeader()
	if err != nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	signature, err := ta.signHeader(ctx, tokenHeader, token)
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

	oauthrpa, err := handleOAuthResponse(ctx, streamer, &shelf)
	if err != nil {
		return err
	}

	ta.sessionContext.UpdateSessionProperties(oauthrpa.connectionValues)

	return nil
}

// expectsHeader reports whether the provider requires signed token header fields.
//
// Parameters:
//   - tokenProvider: the provider being used for authentication.
//
// Returns:
//   - true when tokenProvider implements SignedTokenAuthenticationProvider.
//   - false otherwise.
func expectsHeader(tokenProvider oracleProviders.TokenAuthenticationProvider) bool {
	_, ok := tokenProvider.(oracleProviders.SignedTokenAuthenticationProvider)
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

// generateTokenHeader builds the signed token header when the active provider
// requires one.
//
// Parameters:
//   - none.
//
// Returns:
//   - the generated token header, or an empty string when no header is required.
//   - an error if required session or connect descriptor values cannot be derived.
func (ta *tokenAuthenticator) generateTokenHeader() (string, error) {
	if expectsHeader(ta.tokenProvider) {
		connectDescriptor, err := getRequiredClientProperty(ta.sessionContext, driverCommon.ConnectDescriptor)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		serviceName, err := extractServiceName(connectDescriptor)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		remoteAddr, err := getRequiredClientProperty(ta.sessionContext, driverCommon.RemoteAddress)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		remotePort, err := getRequiredClientIntProperty(ta.sessionContext, driverCommon.RemotePort)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		return fmt.Sprintf(
			"date: %s\n(request-target): %s\nhost: %s",
			time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"),
			serviceName,
			net.JoinHostPort(remoteAddr, strconv.Itoa(remotePort)),
		), nil
	}
	return "", nil
}

// signHeader signs the token header when the active provider supports
// private-key signing.
//
// Parameters:
//   - ctx: the context controlling private key retrieval.
//   - header: the token header payload to sign.
//   - token: the token whose matching private key is required.
//
// Returns:
//   - the base64-encoded signature, or an empty string when signing is not required.
//   - an error if key retrieval, parsing, or signing fails.
func (ta *tokenAuthenticator) signHeader(ctx context.Context, header string, token string) (string, error) {
	if provider, ok := ta.tokenProvider.(oracleProviders.SignedTokenAuthenticationProvider); ok && len(header) > 0 {
		keyPEM, err := provider.PrivateKeyForToken(ctx, token)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		signer, err := getSigner(keyPEM)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		signature, err := signTokenHeader(header, signer)
		if err != nil {
			return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
		}
		return signature, nil
	}
	return "", nil
}

// getRequiredClientProperty returns a non-empty string property from the
// session context.
//
// Parameters:
//   - sessionContext: the session context holding the property snapshot.
//   - key: the property name to retrieve.
//
// Returns:
//   - the trimmed string property value.
//   - an error if the property is missing, not a string, or empty.
func getRequiredClientProperty(sessionContext *driverCommon.SessionContext, key string) (string, error) {
	value, ok := sessionContext.GetClientProperties().GetProperty(key).(string)
	if !ok {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
	}
	return value, nil
}

// getRequiredClientIntProperty returns a non-zero int property from the
// session context.
//
// Parameters:
//   - sessionContext: the session context holding the property snapshot.
//   - key: the property name to retrieve.
//
// Returns:
//   - the int property value.
//   - an error if the property is missing, not an int, or zero.
func getRequiredClientIntProperty(sessionContext *driverCommon.SessionContext, key string) (int, error) {
	value, ok := sessionContext.GetClientProperties().GetProperty(key).(int)
	if !ok || value == 0 {
		return 0, common.NewOracleError(oracleErrors.ValueRetrievalError, nil, key)
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
		return nil, common.NewOracleError(oracleErrors.InvalidSignedTokenPrivateKey, nil)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidSignedTokenPrivateKey, nil)
	}
	if _, ok := signer.(*rsa.PrivateKey); !ok {
		return nil, common.NewOracleError(oracleErrors.InvalidSignedTokenPrivateKey, nil)
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
		return "", common.NewOracleError(oracleErrors.TokenAuthenticationError, err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}
