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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

type mockTokenAuthenticationProvider struct {
	token string
	err   error
}

func (m mockTokenAuthenticationProvider) Token(context.Context) (string, error) {
	return m.token, m.err
}

type mockSignedTokenAuthenticationProvider struct {
	mockTokenAuthenticationProvider
	privateKey    []byte
	privateKeyErr error
	tokenSeen     *string
}

func (m mockSignedTokenAuthenticationProvider) PrivateKeyForToken(
	_ context.Context,
	token string,
) ([]byte, error) {
	if m.tokenSeen != nil {
		*m.tokenSeen = token
	}
	return m.privateKey, m.privateKeyErr
}

func encodePrivateKeyPEM(t *testing.T, privateKey *rsa.PrivateKey) []byte {
	t.Helper()

	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey failed: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
}

func newTestProviderRegistry(providersToRegister ...oracleProviders.Provider) common.ProviderRegistry {
	registry := common.NewProviderRegistry()
	for _, provider := range providersToRegister {
		registry.RegisterProvider(provider)
	}
	return registry
}

type stubProviderRegistry struct {
	provider oracleProviders.Provider
	err      error
}

func (s stubProviderRegistry) RegisterProvider(provider oracleProviders.Provider) {
	s.provider = provider
}

func (s stubProviderRegistry) Provider(providerType reflect.Type) (oracleProviders.Provider, error) {
	return s.provider, s.err
}

// TestGetAuthenticator_SelectionLogic verifies the current authenticator
// selection rules for combinations of username, password, and token providers.
func TestGetAuthenticator_SelectionLogic(t *testing.T) {
	t.Parallel()

	tokenProvider := mockTokenAuthenticationProvider{token: "token-value"}
	providerLookupErr := common.NewOracleError(oracleErrors.ProviderNotFound, nil, "token provider")

	tests := []struct {
		name          string
		user          string
		password      string
		provider      common.ProviderRegistry
		wantType      string
		wantErrorCode oracleErrors.ErrorCode
	}{
		{
			name:     "username and password use password authenticator",
			user:     "scott",
			password: "tiger",
			wantType: "*ttc.passwordAuthenticator",
		},
		{
			name:     "username and password ignore token provider",
			user:     "scott",
			password: "tiger",
			provider: newTestProviderRegistry(tokenProvider),
			wantType: "*ttc.passwordAuthenticator",
		},
		{
			name:          "username without password does not use token provider",
			user:          "scott",
			provider:      newTestProviderRegistry(tokenProvider),
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
		{
			name:          "username without password and without provider returns no authenticator",
			user:          "scott",
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
		{
			name:     "missing username with token provider uses token authenticator",
			provider: newTestProviderRegistry(tokenProvider),
			wantType: "*ttc.tokenAuthenticator",
		},
		{
			name:          "missing username and password without provider returns no authenticator",
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
		{
			name:          "missing username with password and without provider returns empty username",
			password:      "tiger",
			wantErrorCode: oracleErrors.EmptyUsernameError,
		},
		{
			name:          "missing username with password and token provider still returns no password error",
			password:      "tiger",
			provider:      newTestProviderRegistry(tokenProvider),
			wantErrorCode: oracleErrors.EmptyUsernameError,
		},
		{
			name:          "missing username with provider lookup error returns no authenticator",
			provider:      stubProviderRegistry{err: providerLookupErr},
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
		{
			name:          "missing username with password and provider lookup error returns no password error",
			password:      "tiger",
			provider:      stubProviderRegistry{err: providerLookupErr},
			wantErrorCode: oracleErrors.EmptyUsernameError,
		},
		{
			name:          "missing username with nil provider result and password returns empty username",
			password:      "tiger",
			provider:      stubProviderRegistry{},
			wantErrorCode: oracleErrors.EmptyUsernameError,
		},
		{
			name:          "missing username with nil provider result and no password returns no authenticator",
			provider:      stubProviderRegistry{},
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := oracleconfig.NewOracleDriverConfig()
			cfg.Credentials.User = tt.user
			cfg.Credentials.Password = tt.password

			authenticator, err := GetAuthenticator(cfg, tt.provider)

			if tt.wantErrorCode != "" {
				if err == nil {
					t.Fatalf("expected error %s, got nil", tt.wantErrorCode)
				}
				sqle, ok := err.(oracleErrors.SQLError)
				if !ok {
					t.Fatalf("expected SQLError, got %T", err)
				}
				if got := oracleErrors.ErrorCode(sqle.ErrorCode()); got != tt.wantErrorCode {
					t.Fatalf("error code = %s, want %s", got, tt.wantErrorCode)
				}
				if authenticator != nil {
					t.Fatalf("expected nil authenticator on error, got %T", authenticator)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetAuthenticator returned error: %v", err)
			}
			if authenticator == nil {
				t.Fatal("expected authenticator, got nil")
			}
			if got := reflect.TypeOf(authenticator).String(); got != tt.wantType {
				t.Fatalf("authenticator type = %s, want %s", got, tt.wantType)
			}
		})
	}
}

func TestGetAuthenticator_UsesTokenAuthenticatorForSignedToken(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockSignedTokenAuthenticationProvider{
			mockTokenAuthenticationProvider: mockTokenAuthenticationProvider{token: "token-value"},
		},
	))
	if err != nil {
		t.Fatalf("GetAuthenticator returned error: %v", err)
	}
	if _, ok := authenticator.(*tokenAuthenticator); !ok {
		t.Fatalf("expected tokenAuthenticator, got %T", authenticator)
	}
}

func TestGetAuthenticator_UsesTokenAuthenticatorForOAuth(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockTokenAuthenticationProvider{token: "token-value"},
	))
	if err != nil {
		t.Fatalf("GetAuthenticator returned error: %v", err)
	}
	if _, ok := authenticator.(*tokenAuthenticator); !ok {
		t.Fatalf("expected tokenAuthenticator, got %T", authenticator)
	}
}

func TestProviderRegistryReturnsFirstRegisteredTokenProvider(t *testing.T) {
	t.Parallel()

	registry := newTestProviderRegistry(
		struct{}{},
		mockTokenAuthenticationProvider{token: "first-token"},
		mockTokenAuthenticationProvider{token: "second-token"},
	)
	gotProvider, err := registry.Provider(reflect.TypeOf((*oracleProviders.TokenAuthenticationProvider)(nil)).Elem())
	if err != nil {
		t.Fatalf("GetProvider returned error: %v", err)
	}
	provider := gotProvider.(oracleProviders.TokenAuthenticationProvider)

	got, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token returned error: %v", err)
	}
	if got != "first-token" {
		t.Fatalf("Token = %q, want %q", got, "first-token")
	}
}

func TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature(t *testing.T) {
	t.Parallel()

	oauth := NewOAuth().(*oAuth)
	oauth.keyValList = newKeyValueList()
	header := "date: Mon, 10 Aug 2026 10:00:00 GMT\n(request-target): freepdb1\nhost: 127.0.0.1:1521"
	signature := base64.StdEncoding.EncodeToString([]byte("signature"))
	if err := oauth.setTokenKeyValsForOAUTH("token-value", header, signature); err != nil {
		t.Fatalf("setTokenKeyValsForOAUTH returned error: %v", err)
	}

	if oauth.keyValList.Len() != 3 {
		t.Fatalf("expected 3 key/value pairs, got %d", oauth.keyValList.Len())
	}

	got := map[string]string{}
	for e := oauth.keyValList.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*driverCommon.KeyValue)
		got[driverCommon.B1ArrayToString(kv.Key)] = driverCommon.B1ArrayToString(kv.Value)
	}

	if got[authToken] != "token-value" {
		t.Fatalf("AUTH_TOKEN = %q, want token-value", got[authToken])
	}
	if got[authHeader] != header {
		t.Fatalf("AUTH_HEADER = %q, want %q", got[authHeader], header)
	}
	if got[authSignature] == "" {
		t.Fatal("AUTH_SIGNATURE should not be empty")
	}
	if _, err := base64.StdEncoding.DecodeString(got[authSignature]); err != nil {
		t.Fatalf("AUTH_SIGNATURE is not valid base64: %v", err)
	}
}

func TestSignedTokenProviderGenerateTokenHeader(t *testing.T) {
	t.Parallel()

	sessContext := driverCommon.NewSessionContext()
	sessContext.GetClientProperties().SetProperty(driverCommon.RemoteAddress, "192.0.2.10")
	sessContext.GetClientProperties().SetProperty(driverCommon.RemotePort, 1522)
	sessContext.GetClientProperties().SetProperty(driverCommon.ConnectDescriptor, "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")

	authenticator := &tokenAuthenticator{
		tokenProvider:  mockSignedTokenAuthenticationProvider{},
		sessionContext: sessContext,
	}

	header, err := authenticator.generateTokenHeader()
	if err != nil {
		t.Fatalf("generateTokenHeader returned error: %v", err)
	}
	if !strings.Contains(header, "(request-target): freepdb1") {
		t.Fatalf("header missing service name: %q", header)
	}
	if !strings.Contains(header, "host: 192.0.2.10:1522") {
		t.Fatalf("header missing remote ip:port: %q", header)
	}
	if !strings.Contains(header, " GMT\n(request-target): ") {
		t.Fatalf("header date should use GMT format: %q", header)
	}
}

func TestProviderRegistryReturnsNilWhenTokenProviderMissing(t *testing.T) {
	t.Parallel()

	registry := newTestProviderRegistry(
		struct{}{},
		struct{}{},
	)
	provider, err := registry.Provider(reflect.TypeOf((*oracleProviders.TokenAuthenticationProvider)(nil)).Elem())
	if err != nil {
		t.Fatalf("expected nil error when token provider is missing, got %v", err)
	}
	if provider != nil {
		t.Fatalf("expected nil provider when token provider is missing, got %T", provider)
	}
}

func TestOAuthSetTokenKeyValsForOAUTHAddsTokenOnlyWithoutHeader(t *testing.T) {
	t.Parallel()

	oauth := NewOAuth().(*oAuth)
	oauth.keyValList = newKeyValueList()

	if err := oauth.setTokenKeyValsForOAUTH("token-value", "", ""); err != nil {
		t.Fatalf("setTokenKeyValsForOAUTH returned error: %v", err)
	}

	if oauth.keyValList.Len() != 1 {
		t.Fatalf("expected 1 key/value pair, got %d", oauth.keyValList.Len())
	}

	kv := oauth.keyValList.Front().Value.(*driverCommon.KeyValue)
	if got := driverCommon.B1ArrayToString(kv.Key); got != authToken {
		t.Fatalf("unexpected key %q, want %q", got, authToken)
	}
	if got := driverCommon.B1ArrayToString(kv.Value); got != "token-value" {
		t.Fatalf("unexpected value %q, want %q", got, "token-value")
	}
}

func TestTokenAuthenticatorSignHeaderForSignedProvider(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	keyPEM := encodePrivateKeyPEM(t, privateKey)
	var tokenSeen string
	authenticator := &tokenAuthenticator{
		tokenProvider: mockSignedTokenAuthenticationProvider{
			privateKey: keyPEM,
			tokenSeen:  &tokenSeen,
		},
	}

	got, err := authenticator.signHeader(
		context.Background(),
		"date: Mon, 10 Aug 2026 10:00:00 GMT",
		"token-value",
	)
	if err != nil {
		t.Fatalf("signHeader returned error: %v", err)
	}
	if tokenSeen != "token-value" {
		t.Fatalf("PrivateKeyForToken token = %q, want token-value", tokenSeen)
	}
	if got == "" {
		t.Fatal("expected non-empty signature")
	}
	if _, err := base64.StdEncoding.DecodeString(got); err != nil {
		t.Fatalf("signature is not valid base64: %v", err)
	}
}

func TestTokenAuthenticatorSignHeaderForOAuthProviderReturnsEmpty(t *testing.T) {
	t.Parallel()

	authenticator := &tokenAuthenticator{
		tokenProvider: mockTokenAuthenticationProvider{token: "token-value"},
	}

	got, err := authenticator.signHeader(
		context.Background(),
		"date: Mon, 10 Aug 2026 10:00:00 GMT",
		"token-value",
	)
	if err != nil {
		t.Fatalf("signHeader returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("signHeader = %q, want empty signature", got)
	}
}

func TestValidateJWTExpirationExpired(t *testing.T) {
	t.Parallel()

	token := "eyJhbGciOiJub25lIn0.eyJleHAiOjF9."
	err := validateJWTExpiration(token)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

func tokenAuthErrorCode(t *testing.T, err error) oracleErrors.ErrorCode {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T", err)
	}
	return oracleErrors.ErrorCode(sqlErr.ErrorCode())
}

func TestTokenAuthenticatorAuthenticateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		auth     *tokenAuthenticator
		wantCode oracleErrors.ErrorCode
	}{
		{name: "nil provider", auth: &tokenAuthenticator{}, wantCode: oracleErrors.NoAuthenticatorError},
		{name: "nil session context", auth: &tokenAuthenticator{tokenProvider: mockTokenAuthenticationProvider{token: "token"}}, wantCode: oracleErrors.InternalError},
		{name: "provider error", auth: &tokenAuthenticator{tokenProvider: mockTokenAuthenticationProvider{err: errors.New("token unavailable")}, sessionContext: driverCommon.NewSessionContext()}, wantCode: oracleErrors.AuthenticatorError},
		{name: "empty token", auth: &tokenAuthenticator{tokenProvider: mockTokenAuthenticationProvider{}, sessionContext: driverCommon.NewSessionContext()}, wantCode: oracleErrors.EmptyTokenError},
		{name: "expired jwt", auth: &tokenAuthenticator{tokenProvider: mockTokenAuthenticationProvider{token: "eyJhbGciOiJub25lIn0.eyJleHAiOjF9."}, sessionContext: driverCommon.NewSessionContext()}, wantCode: oracleErrors.AuthenticatorError},
		{name: "signed token missing descriptor", auth: &tokenAuthenticator{tokenProvider: mockSignedTokenAuthenticationProvider{mockTokenAuthenticationProvider: mockTokenAuthenticationProvider{token: "token"}}, sessionContext: driverCommon.NewSessionContext()}, wantCode: oracleErrors.AuthenticatorError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenAuthErrorCode(t, tt.auth.Authenticate(context.Background())); got != tt.wantCode {
				t.Fatalf("error code = %s, want %s", got, tt.wantCode)
			}
		})
	}

}

func TestTokenAuthenticatorAuthenticateBuildsOAuthMessage(t *testing.T) {
	t.Parallel()
	shelf, _, _ := newAuthTestShelf(1 << 16)
	streamer := &mockStreamer{
		pullMsgs: []driverCommon.Message[driverCommon.MessageType]{
			&mockOer{err: errors.New("authentication failed")},
		},
	}
	shelf.RegisterMessageStreamer(streamer)

	authenticator := newTokenAuthenticator(mockTokenAuthenticationProvider{token: "token-value"})
	authenticator.SetShelf(shelf)
	authenticator.SetSessionContext(driverCommon.NewSessionContext())
	if err := authenticator.Authenticate(context.Background()); err == nil {
		t.Fatal("Authenticate should return the OER error")
	}

	if !streamer.pushCalled || streamer.pushedMsg.Len() != 1 {
		t.Fatal("Authenticate should push exactly one OAuth message")
	}
	msg := streamer.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
	oauth, ok := (*msg).(*oAuth)
	if !ok {
		t.Fatalf("pushed message = %T, want *oAuth", *msg)
	}
	if oauth.logonMode != KpzLogon {
		t.Fatalf("OAuth logon mode = %#x, want %#x", oauth.logonMode, KpzLogon)
	}
	values := map[string]string{}
	for e := oauth.keyValList.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*driverCommon.KeyValue)
		values[driverCommon.B1ArrayToString(kv.Key)] = driverCommon.B1ArrayToString(kv.Value)
	}
	if values[authToken] != "token-value" {
		t.Fatalf("AUTH_TOKEN = %q, want token-value", values[authToken])
	}
	if _, ok := values[authHeader]; ok {
		t.Fatal("unsigned OAuth message should not contain AUTH_HEADER")
	}
	if _, ok := values[authSignature]; ok {
		t.Fatal("unsigned OAuth message should not contain AUTH_SIGNATURE")
	}
}

func TestTokenAuthenticatorAuthenticateRequiresRPAForSuccessfulOER(t *testing.T) {
	t.Parallel()
	newAuthenticator := func(responses ...driverCommon.Message[driverCommon.MessageType]) (*tokenAuthenticator, *mockStreamer) {
		shelf, _, _ := newAuthTestShelf(1 << 16)
		streamer := &mockStreamer{pullMsgs: responses}
		shelf.RegisterMessageStreamer(streamer)
		authenticator := newTokenAuthenticator(mockTokenAuthenticationProvider{token: "token-value"})
		authenticator.SetShelf(shelf)
		authenticator.SetSessionContext(driverCommon.NewSessionContext())
		return authenticator, streamer
	}

	authenticator, streamer := newAuthenticator(&mockOer{})
	if got := tokenAuthErrorCode(t, authenticator.Authenticate(context.Background())); got != oracleErrors.InternalError {
		t.Fatalf("successful OER before RPA error code = %s, want %s", got, oracleErrors.InternalError)
	}

	rpa := &OAuthRPA{connectionValues: driverCommon.NewProperties[string]()}
	authenticator, streamer = newAuthenticator(rpa, &mockOer{})
	if err := authenticator.Authenticate(context.Background()); err != nil {
		t.Fatalf("RPA followed by successful OER returned error: %v", err)
	}
	if len(streamer.pullTypes) != 3 || streamer.pullTypes[0] != TTIRPA || streamer.pullTypes[1] != TTIOER || streamer.pullTypes[2] != TTIWRN {
		t.Fatalf("authentication Pull types = %v, want [TTIRPA TTIOER TTIWRN]", streamer.pullTypes)
	}
}

func TestTokenAuthenticatorHelpers(t *testing.T) {
	t.Parallel()

	oauthProvider := mockTokenAuthenticationProvider{}
	signedProvider := mockSignedTokenAuthenticationProvider{}
	if expectsHeader(oauthProvider) || !expectsHeader(signedProvider) {
		t.Fatal("expectsHeader returned an unexpected result")
	}
	if got := logonMode(oauthProvider); got != common.KpzLogon.Value() {
		t.Fatalf("OAuth logon mode = %d, want %d", got, common.KpzLogon.Value())
	}
	if got := logonMode(signedProvider); got != common.KpzLogon.Value()|common.KpzLogonToken.Value() {
		t.Fatalf("signed logon mode = %d, want token mode", got)
	}

	if got, err := extractAddressValue("(SERVICE_NAME=freepdb1)", "SERVICE_NAME"); err != nil || got != "freepdb1" {
		t.Fatalf("extractAddressValue = %q, %v", got, err)
	}
	for _, descriptor := range []string{"", "(HOST=localhost)", "(SERVICE_NAME=freepdb1"} {
		if _, err := extractServiceName(descriptor); err == nil {
			t.Fatalf("extractServiceName(%q) should fail", descriptor)
		}
	}

	ctx := driverCommon.NewSessionContext()
	ctx.GetClientProperties().SetProperty("property", " value ")
	if got, err := getRequiredClientProperty(ctx, "property"); err != nil || got != "value" {
		t.Fatalf("getRequiredClientProperty = %q, %v", got, err)
	}
	for _, key := range []string{"missing", "empty"} {
		if key == "empty" {
			ctx.GetClientProperties().SetProperty(key, " ")
		}
		if _, err := getRequiredClientProperty(ctx, key); err == nil {
			t.Fatalf("property %q should fail", key)
		}
	}
	ctx.GetClientProperties().SetProperty("port", 1521)
	if got, err := getRequiredClientIntProperty(ctx, "port"); err != nil || got != 1521 {
		t.Fatalf("int property = %d, %v", got, err)
	}
	if _, err := getRequiredClientIntProperty(ctx, "missing"); err == nil {
		t.Fatal("missing int property should fail")
	}
}

func TestTokenAuthenticatorSettersAndHeaderValidation(t *testing.T) {
	t.Parallel()
	shelf, _, _ := newAuthTestShelf(1 << 12)
	authenticator := newTokenAuthenticator(mockTokenAuthenticationProvider{})
	authenticator.SetShelf(shelf)
	if authenticator.shelf.GetMessageFactory() == nil {
		t.Fatal("SetShelf did not store the shelf")
	}
	sessionContext := driverCommon.NewSessionContext()
	authenticator.SetSessionContext(sessionContext)
	if authenticator.sessionContext != sessionContext {
		t.Fatal("SetSessionContext did not store the session context")
	}

	signed := &tokenAuthenticator{tokenProvider: mockSignedTokenAuthenticationProvider{}, sessionContext: sessionContext}
	tests := []struct {
		name string
		set  func()
	}{
		{name: "missing descriptor", set: func() {}},
		{name: "missing service name", set: func() {
			sessionContext.GetClientProperties().SetProperty(driverCommon.ConnectDescriptor, "(HOST=localhost)")
		}},
		{name: "missing remote address", set: func() {
			sessionContext.GetClientProperties().SetProperty(driverCommon.ConnectDescriptor, "(SERVICE_NAME=freepdb1)")
		}},
		{name: "missing remote port", set: func() {
			sessionContext.GetClientProperties().SetProperty(driverCommon.RemoteAddress, "127.0.0.1")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionContext = driverCommon.NewSessionContext()
			signed.sessionContext = sessionContext
			tt.set()
			if _, err := signed.generateTokenHeader(); err == nil {
				t.Fatal("generateTokenHeader should fail")
			}
		})
	}
}

func TestTokenAuthenticatorSignHeaderErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider mockSignedTokenAuthenticationProvider
		wantCode oracleErrors.ErrorCode
	}{
		{name: "private key provider error", provider: mockSignedTokenAuthenticationProvider{privateKeyErr: errors.New("key unavailable")}, wantCode: oracleErrors.TokenAuthenticationError},
		{name: "malformed pem", provider: mockSignedTokenAuthenticationProvider{privateKey: []byte("not pem")}, wantCode: oracleErrors.TokenAuthenticationError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator := &tokenAuthenticator{tokenProvider: tt.provider}
			if got := tokenAuthErrorCode(t, func() error {
				_, err := authenticator.signHeader(context.Background(), "header", "token")
				return err
			}()); got != tt.wantCode {
				t.Fatalf("error code = %s, want %s", got, tt.wantCode)
			}
		})
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = publicKey
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	authenticator := &tokenAuthenticator{tokenProvider: mockSignedTokenAuthenticationProvider{privateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})}}
	if got := tokenAuthErrorCode(t, func() error {
		_, err := authenticator.signHeader(context.Background(), "header", "token")
		return err
	}()); got != oracleErrors.TokenAuthenticationError {
		t.Fatalf("non-RSA key error code = %s", got)
	}
}

func TestTokenAuthenticatorAuthenticateStreamerErrors(t *testing.T) {
	t.Parallel()
	newAuth := func() (*tokenAuthenticator, *ttiShelf[driverCommon.MessageType], *mockStreamer) {
		shelf, _, _ := newAuthTestShelf(1 << 12)
		streamer := &mockStreamer{}
		shelf.RegisterMessageStreamer(streamer)
		authenticator := newTokenAuthenticator(mockTokenAuthenticationProvider{token: "token"})
		authenticator.SetShelf(shelf)
		authenticator.SetSessionContext(driverCommon.NewSessionContext())
		return authenticator, shelf, streamer
	}

	tests := []struct {
		name  string
		setup func(*tokenAuthenticator, *ttiShelf[driverCommon.MessageType], *mockStreamer)
	}{
		{name: "factory error", setup: func(_ *tokenAuthenticator, shelf *ttiShelf[driverCommon.MessageType], _ *mockStreamer) {
			shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
		}},
		{name: "push error", setup: func(_ *tokenAuthenticator, _ *ttiShelf[driverCommon.MessageType], streamer *mockStreamer) {
			streamer.pushErr = errors.New("push failed")
		}},
		{name: "flush error", setup: func(_ *tokenAuthenticator, _ *ttiShelf[driverCommon.MessageType], streamer *mockStreamer) {
			streamer.flushErr = errors.New("flush failed")
		}},
		{name: "pull error", setup: func(_ *tokenAuthenticator, _ *ttiShelf[driverCommon.MessageType], streamer *mockStreamer) {
			streamer.pullErr = errors.New("pull failed")
		}},
		{name: "unexpected response", setup: func(_ *tokenAuthenticator, _ *ttiShelf[driverCommon.MessageType], streamer *mockStreamer) {
			streamer.pullMsgs = []driverCommon.Message[driverCommon.MessageType]{&dummyMsg{}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator, shelf, streamer := newAuth()
			tt.setup(authenticator, shelf, streamer)
			if err := authenticator.Authenticate(context.Background()); err == nil {
				t.Fatal("Authenticate should fail")
			}
		})
	}
}

func TestValidateJWTExpirationNonExpiredAndMalformed(t *testing.T) {
	t.Parallel()
	for _, token := range []string{
		"plain-token",
		"a.not-base64.",
		"a." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user"}`)) + ".",
		"a." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":9999999999}`)) + ".",
	} {
		if err := validateJWTExpiration(token); err != nil {
			t.Errorf("validateJWTExpiration(%q) = %v", token, err)
		}
	}
}
