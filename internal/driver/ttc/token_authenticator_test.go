package ttc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

func TestGetAuthenticator_UsesTokenAuthenticatorForOCIToken(t *testing.T) {
	t.Parallel()

	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.Credentials.TokenAuthentication = oracleconfig.TokenAuthenticationOCI
	cfg.Credentials.TokenLocation = t.TempDir()
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg)
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
	cfg.Credentials.TokenAuthentication = oracleconfig.TokenAuthenticationOAuth
	cfg.Credentials.TokenLocation = filepath.Join(t.TempDir(), "token")
	cfg.ConnectDescriptor = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))"

	authenticator, err := GetAuthenticator(cfg)
	if err != nil {
		t.Fatalf("GetAuthenticator returned error: %v", err)
	}
	if _, ok := authenticator.(*tokenAuthenticator); !ok {
		t.Fatalf("expected tokenAuthenticator, got %T", authenticator)
	}
}

func TestOCITokenProviderResolveTokenPathDefault(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot retrieve user home directory")
	}

	got, err := (ociTokenProvider{}).resolveTokenPath("")
	if err != nil {
		t.Fatalf("resolveTokenPath returned error: %v", err)
	}

	want := filepath.Join(homeDir, ".oci", "db-token", tokenFileName)
	if got != want {
		t.Fatalf("resolveTokenPath = %q, want %q", got, want)
	}
}

func TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	oauth := NewOAuth().(*oAuth)
	oauth.keyValList = NewKeyValueList()
	header := "date: Mon, 10 Aug 2026 10:00:00 GMT\n(request-target): freepdb1\nhost: 127.0.0.1:1521"
	if err := oauth.setTokenKeyValsForOAUTH("token-value", header, privateKey); err != nil {
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

	header, err := (ociTokenProvider{}).generateTokenHeader(tokenProviderContext{
		connectString:  "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))",
		sessionContext: sessContext,
	})
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

func TestOAuthTokenProviderResolveTokenPathFile(t *testing.T) {
	t.Parallel()

	tokenFile := filepath.Join(t.TempDir(), "jwtbearertoken")
	if err := os.WriteFile(tokenFile, []byte("token-value"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := (oauthTokenProvider{}).resolveTokenPath(tokenFile)
	if err != nil {
		t.Fatalf("resolveTokenPath returned error: %v", err)
	}
	if got != tokenFile {
		t.Fatalf("resolveTokenPath = %q, want %q", got, tokenFile)
	}
}

func TestOAuthTokenProviderApplyAuthDataAddsTokenOnly(t *testing.T) {
	t.Parallel()

	oauth := NewOAuth().(*oAuth)
	oauth.keyValList = NewKeyValueList()

	if err := (oauthTokenProvider{}).applyAuthData(oauth, "token-value", tokenProviderContext{}); err != nil {
		t.Fatalf("applyAuthData returned error: %v", err)
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

func TestTokenAuthenticatorResolveAccessTokenUsesConfiguredAccessToken(t *testing.T) {
	t.Parallel()

	authenticator := NewTokenAuthenticator(oracleconfig.TokenAuthenticationOAuth, " direct-token ", "", "")

	got, err := authenticator.resolveAccessToken()
	if err != nil {
		t.Fatalf("resolveAccessToken returned error: %v", err)
	}
	if got != "direct-token" {
		t.Fatalf("resolveAccessToken = %q, want %q", got, "direct-token")
	}
}

func TestTokenAuthenticatorResolveAccessTokenPrefersAccessTokenOverLocation(t *testing.T) {
	t.Parallel()

	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	authenticator := NewTokenAuthenticator(oracleconfig.TokenAuthenticationOAuth, "inline-token", tokenFile, "")

	got, err := authenticator.resolveAccessToken()
	if err != nil {
		t.Fatalf("resolveAccessToken returned error: %v", err)
	}
	if got != "inline-token" {
		t.Fatalf("resolveAccessToken = %q, want %q", got, "inline-token")
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
