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

package oci

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identitydataplane"
)

const testScope = "urn:oracle:db::id::scope"

var _ interface {
	Token(context.Context) (string, error)
	PrivateKeyForToken(context.Context, string) ([]byte, error)
} = (*Provider)(nil)

type tokenResult struct {
	token    string
	tokenSet bool
	err      error
}

type recordingClient struct {
	mu       sync.Mutex
	results  []tokenResult
	requests []identitydataplane.GenerateScopedAccessTokenRequest
	calls    int
}

func (client *recordingClient) GenerateScopedAccessToken(_ context.Context, request identitydataplane.GenerateScopedAccessTokenRequest) (identitydataplane.GenerateScopedAccessTokenResponse, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.requests = append(client.requests, request)
	index := client.calls
	client.calls++
	if len(client.results) == 0 {
		return identitydataplane.GenerateScopedAccessTokenResponse{}, nil
	}
	if index >= len(client.results) {
		index = len(client.results) - 1
	}
	result := client.results[index]
	if result.err != nil {
		return identitydataplane.GenerateScopedAccessTokenResponse{}, result.err
	}
	if !result.tokenSet {
		return identitydataplane.GenerateScopedAccessTokenResponse{}, nil
	}
	return identitydataplane.GenerateScopedAccessTokenResponse{
		SecurityToken: identitydataplane.SecurityToken{Token: &result.token},
	}, nil
}

func (client *recordingClient) request(index int) identitydataplane.GenerateScopedAccessTokenRequest {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requests[index]
}

func (client *recordingClient) callCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls
}

type blockingClient struct {
	started chan struct{}
	release chan struct{}
	token   string
	once    sync.Once
}

func (client *blockingClient) GenerateScopedAccessToken(ctx context.Context, _ identitydataplane.GenerateScopedAccessTokenRequest) (identitydataplane.GenerateScopedAccessTokenResponse, error) {
	client.once.Do(func() { close(client.started) })
	select {
	case <-ctx.Done():
		return identitydataplane.GenerateScopedAccessTokenResponse{}, ctx.Err()
	case <-client.release:
		return identitydataplane.GenerateScopedAccessTokenResponse{
			SecurityToken: identitydataplane.SecurityToken{Token: &client.token},
		}, nil
	}
}

type testConfigurationProvider struct {
	key    *rsa.PrivateKey
	region string
}

func (provider testConfigurationProvider) PrivateRSAKey() (*rsa.PrivateKey, error) {
	return provider.key, nil
}

func (provider testConfigurationProvider) KeyID() (string, error) {
	return "tenancy/user/fingerprint", nil
}

func (provider testConfigurationProvider) TenancyOCID() (string, error) {
	return "tenancy", nil
}

func (provider testConfigurationProvider) UserOCID() (string, error) {
	return "user", nil
}

func (provider testConfigurationProvider) KeyFingerprint() (string, error) {
	return "fingerprint", nil
}

func (provider testConfigurationProvider) Region() (string, error) {
	return provider.region, nil
}

func (provider testConfigurationProvider) AuthType() (common.AuthConfig, error) {
	return common.AuthConfig{AuthType: common.UnknownAuthenticationType}, nil
}

type refreshableTestConfigurationProvider struct {
	testConfigurationProvider
	refreshable bool
}

func (provider refreshableTestConfigurationProvider) Refreshable() bool {
	return provider.refreshable
}

func newTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testDependencies(now *time.Time, client scopedTokenClient, t *testing.T) dependencies {
	t.Helper()
	configuration := testConfigurationProvider{key: newTestKey(t), region: "us-phoenix-1"}
	return dependencies{
		now:    func() time.Time { return *now },
		newKey: func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 1024) },
		newClient: func(common.ConfigurationProvider, string) (scopedTokenClient, error) {
			return client, nil
		},
		instancePrincipal: func() (common.ConfigurationProvider, error) { return configuration, nil },
		resourcePrincipal: func() (common.ConfigurationProvider, error) { return configuration, nil },
		workloadIdentity:  func() (common.ConfigurationProvider, error) { return configuration, nil },
		configProfile:     func(string, string) common.ConfigurationProvider { return configuration },
	}
}

func newTestProvider(t *testing.T, now *time.Time, client scopedTokenClient) *Provider {
	t.Helper()
	provider, err := newProvider(Config{Principal: InstancePrincipal, Scope: testScope}, testDependencies(now, client, t))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func jwtToken(t *testing.T, expires time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]int64{"exp": expires.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

// TestConfigNormalization verifies trimming, scope derivation, explicit scope
// precedence, and required principal/profile fields.
func TestConfigNormalization(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing principal", config: Config{Scope: testScope}, want: "requires Principal"},
		{name: "unsupported principal", config: Config{Principal: "other", Scope: testScope}, want: "unsupported OCI IAM principal"},
		{name: "missing compartment", config: Config{Principal: InstancePrincipal}, want: "requires CompartmentOCID"},
		{name: "missing database", config: Config{Principal: InstancePrincipal, CompartmentOCID: "compartment"}, want: "requires DatabaseOCID"},
		{name: "profile missing profile", config: Config{Principal: ConfigProfile, Scope: testScope}, want: "requires ConfigProfile"},
		{name: "negative refresh window", config: Config{Principal: InstancePrincipal, Scope: testScope, RefreshBeforeExpiry: -time.Second}, want: "non-negative RefreshBeforeExpiry"},
		{name: "invalid scope family", config: Config{Principal: InstancePrincipal, Scope: "scope"}, want: "must use"},
		{name: "empty scope target", config: Config{Principal: InstancePrincipal, Scope: "urn:oracle:db::id::"}, want: "requires a target"},
		{name: "scope whitespace", config: Config{Principal: InstancePrincipal, Scope: "urn:oracle:db::id::bad scope"}, want: "must not contain whitespace"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.config.normalized()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("normalized() error = %v, want %q", err, test.want)
			}
		})
	}

	config, err := (Config{
		Principal:       " INSTANCE_PRINCIPAL ",
		CompartmentOCID: " compartment ",
		DatabaseOCID:    " database ",
		Region:          " us-ashburn-1 ",
	}).normalized()
	if err != nil {
		t.Fatal(err)
	}
	if config.Principal != InstancePrincipal || config.Region != "us-ashburn-1" {
		t.Fatalf("normalized config = %#v", config)
	}
	if config.Scope != "urn:oracle:db::id::compartment::database" {
		t.Fatalf("derived scope = %q", config.Scope)
	}
	if config.RefreshBeforeExpiry != defaultRefreshBeforeExpiry {
		t.Fatalf("default refresh window = %v", config.RefreshBeforeExpiry)
	}

	config, err = (Config{
		Principal:           ResourcePrincipal,
		Scope:               " urn:oracle:db::path::tenancy:compartment ",
		RefreshBeforeExpiry: time.Minute,
	}).normalized()
	if err != nil {
		t.Fatal(err)
	}
	if config.Scope != "urn:oracle:db::path::tenancy:compartment" || config.RefreshBeforeExpiry != time.Minute {
		t.Fatalf("explicit config = %#v", config)
	}

	if _, err := New(Config{}); err == nil {
		t.Fatal("New accepted invalid configuration")
	}
}

// TestProviderConfigurationModes verifies all supported principal factories,
// profile arguments, derived scope, and region forwarding.
func TestProviderConfigurationModes(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	configuration := testConfigurationProvider{key: newTestKey(t), region: "us-phoenix-1"}
	cases := []struct {
		name        string
		config      Config
		wantFactory string
	}{
		{name: "instance", config: Config{Principal: InstancePrincipal, Scope: testScope}, wantFactory: "instance"},
		{name: "resource", config: Config{Principal: ResourcePrincipal, Scope: testScope}, wantFactory: "resource"},
		{name: "workload", config: Config{Principal: OKEWorkloadIdentity, Scope: testScope}, wantFactory: "workload"},
		{name: "profile", config: Config{Principal: ConfigProfile, Scope: testScope, ConfigFile: " file ", ConfigProfile: " profile "}, wantFactory: "profile"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var factory string
			var gotRegion string
			client := &recordingClient{}
			deps := dependencies{
				now:    func() time.Time { return now },
				newKey: func() (*rsa.PrivateKey, error) { return newTestKey(t), nil },
				newClient: func(provider common.ConfigurationProvider, region string) (scopedTokenClient, error) {
					if provider == nil {
						t.Fatal("newClient received nil configuration provider")
					}
					gotRegion = region
					return client, nil
				},
				instancePrincipal: func() (common.ConfigurationProvider, error) { factory = "instance"; return configuration, nil },
				resourcePrincipal: func() (common.ConfigurationProvider, error) { factory = "resource"; return configuration, nil },
				workloadIdentity:  func() (common.ConfigurationProvider, error) { factory = "workload"; return configuration, nil },
				configProfile: func(file, profile string) common.ConfigurationProvider {
					factory = "profile:" + file + ":" + profile
					return configuration
				},
			}
			test.config.Region = " us-ashburn-1 "
			provider, err := newProvider(test.config, deps)
			if err != nil {
				t.Fatal(err)
			}
			if gotRegion != "us-ashburn-1" {
				t.Fatalf("region = %q", gotRegion)
			}
			if test.wantFactory == "profile" {
				if factory != "profile:file:profile" {
					t.Fatalf("profile factory = %q", factory)
				}
			} else if factory != test.wantFactory {
				t.Fatalf("factory = %q, want %q", factory, test.wantFactory)
			}
			if provider.scope != testScope {
				t.Fatalf("scope = %q", provider.scope)
			}
		})
	}
}

// TestProviderConstructionErrors verifies principal and client factory
// failures, including invalid nil results from injected factories.
func TestProviderConstructionErrors(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	configuration := testConfigurationProvider{key: newTestKey(t), region: "us-phoenix-1"}
	factoryError := errors.New("principal unavailable")
	cases := []struct {
		name   string
		config Config
		deps   func(dependencies) dependencies
		want   error
	}{
		{
			name:   "instance principal",
			config: Config{Principal: InstancePrincipal, Scope: testScope},
			deps: func(deps dependencies) dependencies {
				deps.instancePrincipal = func() (common.ConfigurationProvider, error) { return nil, factoryError }
				return deps
			},
			want: factoryError,
		},
		{
			name:   "resource principal",
			config: Config{Principal: ResourcePrincipal, Scope: testScope},
			deps: func(deps dependencies) dependencies {
				deps.resourcePrincipal = func() (common.ConfigurationProvider, error) { return nil, factoryError }
				return deps
			},
			want: factoryError,
		},
		{
			name:   "workload identity",
			config: Config{Principal: OKEWorkloadIdentity, Scope: testScope},
			deps: func(deps dependencies) dependencies {
				deps.workloadIdentity = func() (common.ConfigurationProvider, error) { return nil, factoryError }
				return deps
			},
			want: factoryError,
		},
		{
			name:   "client",
			config: Config{Principal: ConfigProfile, Scope: testScope, ConfigProfile: "DEFAULT"},
			deps: func(deps dependencies) dependencies {
				deps.configProfile = func(string, string) common.ConfigurationProvider { return configuration }
				deps.newClient = func(common.ConfigurationProvider, string) (scopedTokenClient, error) { return nil, factoryError }
				return deps
			},
			want: factoryError,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			deps := testDependencies(&now, &recordingClient{}, t)
			_, err := newProvider(test.config, test.deps(deps))
			if !errors.Is(err, test.want) {
				t.Fatalf("newProvider() error = %v, want wrapped %v", err, test.want)
			}
		})
	}

	deps := testDependencies(&now, &recordingClient{}, t)
	deps.newClient = func(common.ConfigurationProvider, string) (scopedTokenClient, error) { return nil, nil }
	if _, err := newProvider(Config{Principal: InstancePrincipal, Scope: testScope}, deps); err == nil || !strings.Contains(err.Error(), "client is nil") {
		t.Fatalf("newProvider() error = %v, want nil client error", err)
	}

	deps = testDependencies(&now, &recordingClient{}, t)
	deps.instancePrincipal = func() (common.ConfigurationProvider, error) { return nil, nil }
	if _, err := newProvider(Config{Principal: InstancePrincipal, Scope: testScope}, deps); err == nil || !strings.Contains(err.Error(), "provider is nil") {
		t.Fatalf("newProvider() error = %v, want nil configuration provider error", err)
	}
	if _, err := configurationProvider(Config{Principal: "unsupported"}, testDependencies(&now, &recordingClient{}, t)); err == nil {
		t.Fatal("configurationProvider accepted unsupported principal")
	}
}

// TestRefreshBeforeExpiry verifies a custom refresh lead time controls cache
// reuse without changing the default.
func TestRefreshBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	first := jwtToken(t, now.Add(10*time.Minute))
	second := jwtToken(t, now.Add(20*time.Minute))
	client := &recordingClient{results: []tokenResult{
		{token: first, tokenSet: true},
		{token: second, tokenSet: true},
	}}
	provider, err := newProvider(
		Config{Principal: InstancePrincipal, Scope: testScope, RefreshBeforeExpiry: time.Minute},
		testDependencies(&now, client, t),
	)
	if err != nil {
		t.Fatal(err)
	}
	if token, err := provider.Token(context.Background()); err != nil || token != first {
		t.Fatalf("first Token() = %q, %v", token, err)
	}
	now = now.Add(8 * time.Minute)
	if token, err := provider.Token(context.Background()); err != nil || token != first {
		t.Fatalf("cached Token() = %q, %v", token, err)
	}
	now = now.Add(time.Minute)
	if token, err := provider.Token(context.Background()); err != nil || token != second {
		t.Fatalf("refreshed Token() = %q, %v", token, err)
	}
	if client.callCount() != 2 {
		t.Fatalf("token requests = %d, want 2", client.callCount())
	}
}

// TestTokenCacheRefreshRetentionAndPruning verifies cache reuse, refresh,
// exact key retention across generations, and expiry-based pruning.
func TestTokenCacheRefreshRetentionAndPruning(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	firstExpiration := now.Add(10 * time.Minute)
	secondExpiration := now.Add(20 * time.Minute)
	first := jwtToken(t, firstExpiration)
	second := jwtToken(t, secondExpiration)
	client := &recordingClient{results: []tokenResult{{token: first, tokenSet: true}, {token: second, tokenSet: true}}}
	provider := newTestProvider(t, &now, client)

	token, err := provider.Token(context.Background())
	if err != nil || token != first {
		t.Fatalf("first Token() = %q, %v", token, err)
	}
	if _, err := provider.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.callCount() != 1 {
		t.Fatalf("calls after cache hit = %d", client.callCount())
	}
	firstKey, err := provider.PrivateKeyForToken(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivateKeyMatchesRequest(t, firstKey, client.request(0))
	firstByte := firstKey[0]
	firstKey[0] ^= 0xFF
	retainedKey, err := provider.PrivateKeyForToken(context.Background(), first)
	if err != nil || retainedKey[0] != firstByte {
		t.Fatalf("retained key was changed through a returned slice: %v", err)
	}
	firstKey[0] = firstByte

	now = now.Add(5 * time.Minute)
	token, err = provider.Token(context.Background())
	if err != nil || token != second {
		t.Fatalf("refreshed Token() = %q, %v", token, err)
	}
	if client.callCount() != 2 {
		t.Fatalf("calls after refresh = %d", client.callCount())
	}
	secondKey, err := provider.PrivateKeyForToken(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstKey) == string(secondKey) {
		t.Fatal("refresh reused the private key")
	}
	if retainedFirst, err := provider.PrivateKeyForToken(context.Background(), first); err != nil || string(retainedFirst) != string(firstKey) {
		t.Fatalf("retained first key = %q, %v", retainedFirst, err)
	}
	if len(provider.generations) != 2 {
		t.Fatalf("retained generations = %d", len(provider.generations))
	}

	now = firstExpiration
	if _, err := provider.PrivateKeyForToken(context.Background(), first); err == nil || !strings.Contains(err.Error(), "no OCI IAM database private key") {
		t.Fatalf("expired token key error = %v", err)
	}
	if len(provider.generations) != 1 {
		t.Fatalf("generations after pruning = %d", len(provider.generations))
	}
}

func assertPrivateKeyMatchesRequest(t *testing.T, privatePEM []byte, request identitydataplane.GenerateScopedAccessTokenRequest) {
	t.Helper()
	block, _ := pem.Decode(privatePEM)
	if block == nil {
		t.Fatal("private key is not PEM")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("private key type = %T", privateKey)
	}
	if request.GenerateScopedAccessTokenDetails.Scope == nil || *request.GenerateScopedAccessTokenDetails.Scope != testScope {
		t.Fatalf("request scope = %v", request.GenerateScopedAccessTokenDetails.Scope)
	}
	publicBlock, _ := pem.Decode([]byte(*request.GenerateScopedAccessTokenDetails.PublicKey))
	if publicBlock == nil {
		t.Fatal("public key is not PEM")
	}
	publicKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	publicRSAKey, ok := publicKey.(*rsa.PublicKey)
	if !ok || rsaKey.PublicKey.N.Cmp(publicRSAKey.N) != 0 || rsaKey.PublicKey.E != publicRSAKey.E {
		t.Fatal("private key does not match request public key")
	}
}

// TestTokenErrors verifies OCI request failures and empty, malformed, missing-
// expiry, and already-expired token responses.
func TestTokenErrors(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	clientError := errors.New("OCI unavailable")
	cases := []struct {
		name   string
		result tokenResult
		want   string
		cause  error
	}{
		{name: "client", result: tokenResult{err: clientError}, want: "get OCI IAM database token", cause: clientError},
		{name: "nil token", result: tokenResult{}, want: "empty OCI IAM database token"},
		{name: "empty token", result: tokenResult{token: "", tokenSet: true}, want: "empty OCI IAM database token"},
		{name: "not jwt", result: tokenResult{token: "not-a-jwt", tokenSet: true}, want: "database token is not a JWT"},
		{name: "bad payload", result: tokenResult{token: "header.%%%.signature", tokenSet: true}, want: "decode JWT payload"},
		{name: "bad claims", result: tokenResult{token: "header." + base64.RawURLEncoding.EncodeToString([]byte("{")) + ".signature", tokenSet: true}, want: "decode JWT claims"},
		{name: "missing expiration", result: tokenResult{token: "header." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".signature", tokenSet: true}, want: "JWT has no exp claim"},
		{name: "expired", result: tokenResult{token: jwtToken(t, now), tokenSet: true}, want: "expired OCI IAM database token"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := &recordingClient{results: []tokenResult{test.result}}
			provider := newTestProvider(t, &now, client)
			_, err := provider.Token(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Token() error = %v, want %q", err, test.want)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("Token() error = %v, want wrapped %v", err, test.cause)
			}
			if len(provider.generations) != 0 {
				t.Fatalf("failed token retained %d generations", len(provider.generations))
			}
		})
	}
}

// TestTokenKeyAndContextErrors verifies key-generation failures, cancellation,
// unknown token rejection, and invalid private-key inputs.
func TestTokenKeyAndContextErrors(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	keyError := errors.New("randomness unavailable")
	deps := testDependencies(&now, &recordingClient{}, t)
	deps.newKey = func() (*rsa.PrivateKey, error) { return nil, keyError }
	provider, err := newProvider(Config{Principal: InstancePrincipal, Scope: testScope}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Token(context.Background()); !errors.Is(err, keyError) {
		t.Fatalf("key error = %v", err)
	}

	deps = testDependencies(&now, &recordingClient{}, t)
	deps.newKey = func() (*rsa.PrivateKey, error) { return nil, nil }
	provider, err = newProvider(Config{Principal: InstancePrincipal, Scope: testScope}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Token(context.Background()); err == nil || !strings.Contains(err.Error(), "key is nil") {
		t.Fatalf("nil key error = %v", err)
	}

	client := &recordingClient{results: []tokenResult{{token: jwtToken(t, now.Add(time.Hour)), tokenSet: true}}}
	provider = newTestProvider(t, &now, client)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Token(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Token() error = %v", err)
	}
	if client.callCount() != 0 {
		t.Fatalf("cancelled Token() made %d client calls", client.callCount())
	}
	if _, err := provider.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.PrivateKeyForToken(cancelled, provider.token); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled PrivateKeyForToken() error = %v", err)
	}
	if _, err := provider.PrivateKeyForToken(context.Background(), "not-returned"); err == nil || strings.Contains(err.Error(), "not-returned") {
		t.Fatalf("unknown token error = %v", err)
	}
	if _, err := marshalPrivateKey(nil); err == nil || !strings.Contains(err.Error(), "key is nil") {
		t.Fatalf("nil marshal error = %v", err)
	}
}

// TestProviderConcurrency verifies concurrent callers share one token refresh
// and receive the same cached generation.
func TestProviderConcurrency(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	token := jwtToken(t, now.Add(time.Hour))
	client := &recordingClient{results: []tokenResult{{token: token, tokenSet: true}}}
	provider := newTestProvider(t, &now, client)

	const callers = 32
	var group sync.WaitGroup
	errors := make(chan error, callers)
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := provider.Token(context.Background())
			if err != nil {
				errors <- err
				return
			}
			if got != token {
				errors <- fmt.Errorf("token = %q", got)
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if client.callCount() != 1 {
		t.Fatalf("concurrent calls = %d", client.callCount())
	}
}

// TestRefreshDoesNotBlockRetainedKeyLookupOrCancellation verifies a slow
// refresh does not block valid key lookup or prevent waiter cancellation.
func TestRefreshDoesNotBlockRetainedKeyLookupOrCancellation(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	refreshedToken := jwtToken(t, now.Add(time.Hour))
	client := &blockingClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
		token:   refreshedToken,
	}
	provider := newTestProvider(t, &now, client)
	retainedToken := jwtToken(t, now.Add(10*time.Minute))
	retainedKey := newTestKey(t)
	provider.token = retainedToken
	retainedKeyPEM, err := marshalPrivateKey(retainedKey)
	if err != nil {
		t.Fatalf("marshal retained key: %v", err)
	}
	provider.expires = now.Add(time.Minute)
	provider.generations = map[[sha256.Size]byte]tokenGeneration{
		sha256.Sum256([]byte(retainedToken)): {privateKeyPEM: retainedKeyPEM, expires: now.Add(10 * time.Minute)},
	}

	refreshResult := make(chan error, 1)
	go func() {
		_, err := provider.Token(context.Background())
		refreshResult <- err
	}()
	<-client.started

	if _, err := provider.PrivateKeyForToken(context.Background(), retainedToken); err != nil {
		t.Fatalf("PrivateKeyForToken during refresh: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := provider.Token(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting Token error = %v, want deadline exceeded", err)
	}

	close(client.release)
	if err := <-refreshResult; err != nil {
		t.Fatalf("refresh Token: %v", err)
	}
}

// TestJWTExpirationAndDataplaneRegion verifies JWT expiration parsing, RSA key
// generation, principal wrappers, region override, and refreshability.
func TestJWTExpirationAndDataplaneRegion(t *testing.T) {
	expires := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)
	got, err := jwtExpiration(jwtToken(t, expires))
	if err != nil || !got.Equal(expires) {
		t.Fatalf("jwtExpiration() = %v, %v", got, err)
	}

	generatedKey, err := generatePrivateKey()
	if err != nil {
		t.Fatalf("generatePrivateKey(): %v", err)
	}
	if generatedKey.N.BitLen() != 2048 {
		t.Fatalf("generatePrivateKey() bits = %d, want 2048", generatedKey.N.BitLen())
	}

	t.Setenv("OCI_RESOURCE_PRINCIPAL_VERSION", "")
	if _, err := resourcePrincipalConfigurationProvider(); err == nil {
		t.Fatal("resource principal accepted an invalid environment")
	}
	if _, err := workloadIdentityConfigurationProvider(); err == nil {
		t.Fatal("OKE workload identity accepted an invalid environment")
	}

	key := newTestKey(t)
	base := testConfigurationProvider{key: key, region: "us-phoenix-1"}
	client, err := newDataplaneClient(base, "")
	if err != nil {
		t.Fatal(err)
	}
	dataPlane, ok := client.(*identitydataplane.DataplaneClient)
	if !ok || !strings.Contains(dataPlane.Host, "us-phoenix-1") {
		t.Fatalf("default regional client = %#v", client)
	}
	refreshableBase := refreshableTestConfigurationProvider{testConfigurationProvider: base, refreshable: true}
	client, err = newDataplaneClient(refreshableBase, "us-ashburn-1")
	if err != nil {
		t.Fatal(err)
	}
	dataPlane, ok = client.(*identitydataplane.DataplaneClient)
	if !ok || !strings.Contains(dataPlane.Host, "us-ashburn-1") {
		t.Fatalf("overridden regional client = %#v", client)
	}
	if !dataPlane.BaseClient.IsRefreshableAuthType() {
		t.Fatal("region override hid refreshable authentication")
	}
	regional := regionalConfigurationProvider{ConfigurationProvider: refreshableBase, region: "us-chicago-1"}
	if region, err := regional.Region(); err != nil || region != "us-chicago-1" {
		t.Fatalf("regional provider Region() = %q, %v", region, err)
	}
	if !regional.Refreshable() {
		t.Fatal("regional provider did not forward Refreshable")
	}
	if (regionalConfigurationProvider{ConfigurationProvider: base}).Refreshable() {
		t.Fatal("regional provider reported a non-refreshable provider as refreshable")
	}
}
