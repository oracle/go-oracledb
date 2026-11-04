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
	"encoding/base64"
	"errors"
	"reflect"
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

func newTestProviderRegistry(providersToRegister ...oracleProviders.Provider) common.Registry[oracleProviders.Provider] {
	registry := common.NewSafeRegistry[oracleProviders.Provider]()
	for _, provider := range providersToRegister {
		registry.Register(provider)
	}
	return registry
}

type stubProviderRegistry struct {
	provider oracleProviders.Provider
	err      error
}

func (s stubProviderRegistry) Register(provider oracleProviders.Provider) {
	s.provider = provider
}

func (s stubProviderRegistry) GetAll() []oracleProviders.Provider {
	if s.provider == nil {
		return nil
	}
	return []oracleProviders.Provider{s.provider}
}

func (s stubProviderRegistry) Get(providerType reflect.Type) (oracleProviders.Provider, error) {
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
		provider      common.Registry[oracleProviders.Provider]
		wantType      string
		wantErrorCode oracleErrors.ErrorCode
	}{
		{
			name:     "username and password use password authenticator",
			user:     "scott",
			password: "tiger",
			provider: newTestProviderRegistry(),
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
			provider:      newTestProviderRegistry(),
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
		{
			name:     "missing username with token provider uses token authenticator",
			provider: newTestProviderRegistry(tokenProvider),
			wantType: "*ttc.tokenAuthenticator",
		},
		{
			name:          "missing username and password without provider returns no authenticator",
			provider:      newTestProviderRegistry(),
			wantErrorCode: oracleErrors.NoAuthenticatorError,
		},
		{
			name:          "missing username with password and without provider returns empty username",
			password:      "tiger",
			provider:      newTestProviderRegistry(),
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

			authenticator, err := GetAuthenticator(cfg, tt.provider, driverCommon.ConnectionNormal)

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

// TestGetAuthenticator_UsesTokenAuthenticatorForSignedToken verifies that when
// a signed-token provider is registered and that no username or password as
// passed the authenticator selected in a the token authenticator.
func TestGetAuthenticator_UsesTokenAuthenticatorForSignedToken(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockSignedTokenAuthenticationProvider{
			mockTokenAuthenticationProvider: mockTokenAuthenticationProvider{token: "token-value"},
		},
	),
		driverCommon.ConnectionNormal,
	)
	if err != nil {
		t.Fatalf("GetAuthenticator returned error: %v", err)
	}
	if _, ok := authenticator.(*tokenAuthenticator); !ok {
		t.Fatalf("expected tokenAuthenticator, got %T", authenticator)
	}
}

// TestGetAuthenticator_UsesTokenAuthenticatorForSignedToken verifies that when
// a OAuth provider is registered and that no username or password as passed
// the authenticator selected in a the token authenticator.
func TestGetAuthenticator_UsesTokenAuthenticatorForOAuth(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockTokenAuthenticationProvider{token: "token-value"},
	),
		driverCommon.ConnectionNormal,
	)
	if err != nil {
		t.Fatalf("GetAuthenticator returned error: %v", err)
	}
	if _, ok := authenticator.(*tokenAuthenticator); !ok {
		t.Fatalf("expected tokenAuthenticator, got %T", authenticator)
	}
}

// TestProviderRegistryReturnsFirstRegisteredTokenProvider verifies that the
// first matching token provider is returned from the registry.
func TestProviderRegistryReturnsFirstRegisteredTokenProvider(t *testing.T) {
	t.Parallel()

	registry := newTestProviderRegistry(
		struct{}{},
		mockTokenAuthenticationProvider{token: "first-token"},
		mockTokenAuthenticationProvider{token: "second-token"},
	)
	gotProvider, err := registry.Get(reflect.TypeOf((*oracleProviders.TokenAuthenticationProvider)(nil)).Elem())
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
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

// TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature verifies that
// token, header, and signature values are encoded in the message.
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

// TestProviderRegistryReturnsNilWhenTokenProviderMissing verifies that lookup
// returns no provider when the registry has no token provider.
func TestProviderRegistryReturnsNilWhenTokenProviderMissing(t *testing.T) {
	t.Parallel()

	registry := newTestProviderRegistry(
		struct{}{},
		struct{}{},
	)
	provider, err := registry.Get(reflect.TypeOf((*oracleProviders.TokenAuthenticationProvider)(nil)).Elem())
	if err != nil {
		t.Fatalf("expected nil error when token provider is missing, got %v", err)
	}
	if provider != nil {
		t.Fatalf("expected nil provider when token provider is missing, got %T", provider)
	}
}

// TestOAuthSetTokenKeyValsForOAUTHAddsTokenOnlyWithoutHeader verifies that an
// unsigned OAuth message contains only the token value.
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

// tokenAuthErrorCode if the error is not nil and a SQLError, it returns the
// error code otherwise nil.
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

// TestTokenAuthenticatorAuthenticateValidation verifies Authenticate's input,
// provider, token, JWT, and signed-provider validation errors.
func TestTokenAuthenticatorAuthenticateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		provider       oracleProviders.TokenAuthenticationProvider
		sessionContext *driverCommon.SessionContext
		wantCode       oracleErrors.ErrorCode
	}{
		{name: "nil provider", wantCode: oracleErrors.NoAuthenticatorError},
		{name: "nil session context", provider: mockTokenAuthenticationProvider{token: "token"}, wantCode: oracleErrors.InternalError},
		{name: "provider error", provider: mockTokenAuthenticationProvider{err: errors.New("token unavailable")}, sessionContext: driverCommon.NewSessionContext(), wantCode: oracleErrors.AuthenticatorError},
		{name: "empty token", provider: mockTokenAuthenticationProvider{}, sessionContext: driverCommon.NewSessionContext(), wantCode: oracleErrors.EmptyTokenError},
		{name: "expired jwt", provider: mockTokenAuthenticationProvider{token: "eyJhbGciOiJub25lIn0.eyJleHAiOjF9."}, sessionContext: driverCommon.NewSessionContext(), wantCode: oracleErrors.AuthenticatorError},
		{name: "signed token missing descriptor", provider: mockSignedTokenAuthenticationProvider{mockTokenAuthenticationProvider: mockTokenAuthenticationProvider{token: "token"}}, sessionContext: driverCommon.NewSessionContext(), wantCode: oracleErrors.AuthenticatorError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator := newTokenAuthenticator(tt.provider)
			authenticator.SetSessionContext(tt.sessionContext)
			if got := tokenAuthErrorCode(t, authenticator.Authenticate(context.Background())); got != tt.wantCode {
				t.Fatalf("error code = %s, want %s", got, tt.wantCode)
			}
		})
	}

}

// TestTokenAuthenticatorAuthenticateBuildsOAuthMessage verifies that
// Authenticate sends the expected OAuth message and token fields.
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

// TestTokenAuthenticatorAuthenticateRequiresRPAForSuccessfulOER verifies that
// a successful OER response is valid only after an RPA response.
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

// TestTokenAuthenticatorAuthenticateStreamerErrors verifies that Authenticate
// reports message factory, streamer, and response errors.
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

// TestTokenAuthenticatorSetters verifies shelf and session-context setters.
func TestTokenAuthenticatorSetters(t *testing.T) {
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
}
