package ttc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

type mockTokenAuthenticationProvider struct {
	token string
	err   error
}

func (m mockTokenAuthenticationProvider) Token(context.Context) (string, error) {
	return m.token, m.err
}

type mockOCITokenAuthenticationProvider struct {
	mockTokenAuthenticationProvider
	privateKey    []byte
	privateKeyErr error
}

func (m mockOCITokenAuthenticationProvider) PrivateKey(context.Context) ([]byte, error) {
	return m.privateKey, m.privateKeyErr
}

type mockNonTokenProvider struct{}

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

func TestGetAuthenticator_UsesTokenAuthenticatorForOCIToken(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockOCITokenAuthenticationProvider{
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

func TestGetAuthenticator_RejectsPasswordWithoutUsername(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.Credentials.Password = "password"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockTokenAuthenticationProvider{token: "token-value"},
	))
	if err == nil || !strings.Contains(err.Error(), "empty username") {
		t.Fatalf("expected empty username error, got authenticator %T and error %v", authenticator, err)
	}
}

func TestGetAuthenticator_PrefersPasswordCredentialsWhenUsernameIsPresent(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.Credentials.User = "user"
	cfg.Credentials.Password = "password"

	authenticator, err := GetAuthenticator(cfg, newTestProviderRegistry(
		mockTokenAuthenticationProvider{token: "token-value"},
	))
	if err != nil {
		t.Fatalf("GetAuthenticator returned error: %v", err)
	}
	if _, ok := authenticator.(*PasswordAuthenticator); !ok {
		t.Fatalf("expected PasswordAuthenticator, got %T", authenticator)
	}
}

func TestFindFirstTokenAuthenticatorProviderReturnsFirstMatch(t *testing.T) {
	t.Parallel()

	provider := findFirstTokenAuthenticatorProvider([]oracleProviders.Provider{
		mockNonTokenProvider{},
		mockTokenAuthenticationProvider{token: "first-token"},
		mockTokenAuthenticationProvider{token: "second-token"},
	})
	if provider == nil {
		t.Fatal("expected token authentication provider, got nil")
	}

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
	oauth.keyValList = NewKeyValueList()
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

func TestOCITokenProviderGenerateTokenHeader(t *testing.T) {
	t.Parallel()

	sessContext := driverCommon.NewSessionContext()
	sessionProperties := driverCommon.NewProperties[string]()
	sessionProperties.SetProperty("REMOTE_ADDRESS", "192.0.2.10:1522")
	sessContext.UpdateSessionProperties(sessionProperties)

	authenticator := &tokenAuthenticator{
		tokenProvider:  mockOCITokenAuthenticationProvider{},
		connectString:  "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))",
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

func TestFindFirstTokenAuthenticatorProviderReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	provider := findFirstTokenAuthenticatorProvider([]oracleProviders.Provider{
		mockNonTokenProvider{},
		struct{}{},
	})
	if provider != nil {
		t.Fatalf("expected nil token authentication provider, got %T", provider)
	}
}

func TestOAuthSetTokenKeyValsForOAUTHAddsTokenOnlyWithoutHeader(t *testing.T) {
	t.Parallel()

	oauth := NewOAuth().(*oAuth)
	oauth.keyValList = NewKeyValueList()

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

func TestTokenAuthenticatorSignHeaderForOCIProvider(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	authenticator := &tokenAuthenticator{
		tokenProvider: mockOCITokenAuthenticationProvider{
			privateKey: encodePrivateKeyPEM(t, privateKey),
		},
	}

	got, err := authenticator.signHeader(context.Background(), "date: Mon, 10 Aug 2026 10:00:00 GMT")
	if err != nil {
		t.Fatalf("signHeader returned error: %v", err)
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

	got, err := authenticator.signHeader(context.Background(), "date: Mon, 10 Aug 2026 10:00:00 GMT")
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
