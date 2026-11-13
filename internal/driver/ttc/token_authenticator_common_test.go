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
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestSignedTokenProviderGenerateTokenHeader verifies that a signed provider's
// header contains the service name, host, port, and GMT timestamp.
func TestSignedTokenProviderGenerateTokenHeader(t *testing.T) {
	t.Parallel()

	sessContext := driverCommon.NewSessionContext()
	sessContext.GetClientProperties().SetProperty(driverCommon.RemoteAddress, "192.0.2.10")
	sessContext.GetClientProperties().SetProperty(driverCommon.RemotePort, 1522)
	sessContext.GetClientProperties().SetProperty(driverCommon.ConnectDescriptor, "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")

	setup := &tokenAuthenticatorCommon{
		tokenProvider:  mockSignedTokenAuthenticationProvider{},
		sessionContext: sessContext,
	}

	header, err := setup.generateTokenHeader()
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

// TestTokenAuthenticatorSignHeaderForSignedProvider verifies that signed
// providers produce a base64-encoded signature and receive the token.
func TestTokenAuthenticatorSignHeaderForSignedProvider(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	keyPEM := encodePrivateKeyPEM(t, privateKey)
	var tokenSeen string
	setup := &tokenAuthenticatorCommon{
		tokenProvider: mockSignedTokenAuthenticationProvider{
			privateKey: keyPEM,
			tokenSeen:  &tokenSeen,
		},
	}

	got, err := setup.signHeader(
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

func encodePrivateKeyPEM(t *testing.T, privateKey *rsa.PrivateKey) []byte {
	t.Helper()

	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey failed: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
}

// TestTokenAuthenticatorSignHeaderForOAuthProviderReturnsEmpty verifies that
// OAuth providers do not produce a separate signature header.
func TestTokenAuthenticatorSignHeaderForOAuthProviderReturnsEmpty(t *testing.T) {
	t.Parallel()

	setup := &tokenAuthenticatorCommon{
		tokenProvider: mockTokenAuthenticationProvider{token: "token-value"},
	}

	got, err := setup.signHeader(
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

// TestValidateJWTExpirationExpired verifies that an expired JWT is rejected.
func TestValidateJWTExpirationExpired(t *testing.T) {
	t.Parallel()

	token := "eyJhbGciOiJub25lIn0.eyJleHAiOjF9."
	err := validateJWTExpiration(token)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

// TestTokenAuthenticatorHelpers verifies token-provider modes and helper
// functions for service names and required session properties.
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

// TestTokenAuthenticatorSignHeaderErrors verifies signing failures for invalid
// keys, malformed PEM data, and unsupported key types.
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
			setup := &tokenAuthenticatorCommon{tokenProvider: tt.provider}
			if got := tokenAuthErrorCode(t, func() error {
				_, err := setup.signHeader(context.Background(), "header", "token")
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
	setup := &tokenAuthenticatorCommon{tokenProvider: mockSignedTokenAuthenticationProvider{privateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})}}
	if got := tokenAuthErrorCode(t, func() error {
		_, err := setup.signHeader(context.Background(), "header", "token")
		return err
	}()); got != oracleErrors.TokenAuthenticationError {
		t.Fatalf("non-RSA key error code = %s", got)
	}
}

// TestValidateJWTExpirationNonExpiredAndMalformed verifies that non-expired and
// malformed-but-non-expiring tokens are accepted.
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

// TestTokenAuthenticatorCommonHeaderValidation verifies validation of the
// session properties required to build a signed-token header.
func TestTokenAuthenticatorCommonHeaderValidation(t *testing.T) {
	t.Parallel()
	sessionContext := driverCommon.NewSessionContext()

	signed := &tokenAuthenticatorCommon{tokenProvider: mockSignedTokenAuthenticationProvider{}, sessionContext: sessionContext}
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
