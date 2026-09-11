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

// Package oci provides OCI IAM scoped-access tokens for Oracle Database token
// authentication.
package oci

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/identitydataplane"
)

const defaultRefreshBeforeExpiry = 5 * time.Minute

// Principal identifies the OCI IAM principal used to obtain scoped database
// access tokens.
type Principal string

const (
	// InstancePrincipal authenticates with the OCI instance principal.
	InstancePrincipal Principal = "instance_principal"
	// ResourcePrincipal authenticates with the OCI resource principal.
	ResourcePrincipal Principal = "resource_principal"
	// OKEWorkloadIdentity authenticates with the OKE workload identity.
	OKEWorkloadIdentity Principal = "oke_workload_identity"
	// ConfigProfile authenticates with a profile in an OCI configuration file.
	ConfigProfile Principal = "config_profile"
)

// Config configures an OCI IAM database token provider.
type Config struct {
	// Principal selects the OCI IAM principal used to obtain database tokens.
	Principal Principal
	// CompartmentOCID identifies the OCI compartment used to derive Scope when
	// Scope is empty.
	CompartmentOCID string
	// DatabaseOCID identifies the target database used to derive Scope when
	// Scope is empty.
	DatabaseOCID string
	// Scope explicitly selects the scoped-access-token scope. It takes
	// precedence over CompartmentOCID and DatabaseOCID.
	Scope string
	// RefreshBeforeExpiry controls how early Token refreshes the current token.
	// Zero uses the default five-minute window. Negative values are rejected.
	RefreshBeforeExpiry time.Duration
	// Region overrides the region reported by the selected principal.
	Region string
	// ConfigFile selects an OCI configuration file when Principal is
	// ConfigProfile. An empty value uses the OCI SDK default location.
	ConfigFile string
	// ConfigProfile names the profile in ConfigFile when Principal is
	// ConfigProfile.
	ConfigProfile string
}

// Provider obtains scoped OCI IAM database tokens and retains the private key
// associated with every returned token until that token expires.
type Provider struct {
	// mu protects token, expires, generations, refreshing, and refreshDone.
	// client, scope, refreshBeforeExpiry, now, and newKey are immutable after construction.
	mu sync.Mutex
	// client issues signed OCI IAM scoped-access-token requests.
	client scopedTokenClient
	// scope is the normalized scope included in every token request.
	scope string
	// refreshBeforeExpiry controls how early a cached token is replaced.
	refreshBeforeExpiry time.Duration
	// token is the newest successfully issued token.
	token string
	// expires is token's expiration time.
	expires time.Time
	// generations maps token hashes to immutable retained key generations.
	generations map[[sha256.Size]byte]tokenGeneration
	// refreshing reports whether one caller is currently obtaining a token.
	refreshing bool
	// refreshDone is closed when the active refresh finishes, waking waiters.
	refreshDone chan struct{}
	// now supplies wall-clock time; production uses time.Now and tests inject it.
	now func() time.Time
	// newKey creates the RSA key associated with the next token generation.
	newKey func() (*rsa.PrivateKey, error)
}

// scopedTokenClient is the private subset of the OCI Identity Data Plane
// client used by Provider. The OCI SDK client implements it; tests provide an
// offline implementation.
type scopedTokenClient interface {
	// GenerateScopedAccessToken obtains a token for the request's scope and
	// public key.
	//
	// Parameters:
	//   - context.Context: controls cancellation and request deadlines.
	//   - identitydataplane.GenerateScopedAccessTokenRequest: token scope and
	//     public key.
	//
	// Returns:
	//   - identitydataplane.GenerateScopedAccessTokenResponse: issued token.
	//   - error: OCI request or authentication failure.
	GenerateScopedAccessToken(context.Context, identitydataplane.GenerateScopedAccessTokenRequest) (identitydataplane.GenerateScopedAccessTokenResponse, error)
}

// tokenGeneration retains immutable private key material for one token.
type tokenGeneration struct {
	// privateKeyPEM is the PKCS#8 key returned for the matching token.
	privateKeyPEM []byte
	// expires controls when the generation is removed.
	expires time.Time
}

// dependencies is a private dependency-injection bundle used only by
// newProvider. New supplies production implementations; tests replace them
// with deterministic clocks, keys, clients, and principal factories so every
// lifecycle and failure path runs offline without sleeping or contacting OCI.
type dependencies struct {
	// now supplies the current time.
	now func() time.Time
	// newKey creates a token-generation RSA key.
	newKey func() (*rsa.PrivateKey, error)
	// newClient creates the OCI Identity Data Plane client.
	newClient func(common.ConfigurationProvider, string) (scopedTokenClient, error)
	// instancePrincipal creates an instance-principal configuration provider.
	instancePrincipal func() (common.ConfigurationProvider, error)
	// resourcePrincipal creates a resource-principal configuration provider.
	resourcePrincipal func() (common.ConfigurationProvider, error)
	// workloadIdentity creates an OKE workload-identity configuration provider.
	workloadIdentity func() (common.ConfigurationProvider, error)
	// configProfile creates a file-backed configuration provider.
	configProfile func(string, string) common.ConfigurationProvider
}

// New constructs an OCI IAM database token provider.
//
// Parameters:
//   - config: principal, scope, and optional region/profile configuration.
//
// Returns:
//   - *Provider: concurrency-safe signed token provider.
//   - error: configuration, principal, or client construction failure.
func New(config Config) (*Provider, error) {
	return newProvider(config, defaultDependencies())
}

// defaultDependencies returns production implementations for external
// dependencies.
//
// Returns:
//   - dependencies: wall clock, RSA generator, OCI client, and principal factories.
func defaultDependencies() dependencies {
	return dependencies{
		now:               time.Now,
		newKey:            generatePrivateKey,
		newClient:         newDataplaneClient,
		instancePrincipal: instancePrincipalConfigurationProvider,
		resourcePrincipal: resourcePrincipalConfigurationProvider,
		workloadIdentity:  workloadIdentityConfigurationProvider,
		configProfile:     common.CustomProfileConfigProvider,
	}
}

// generatePrivateKey creates a 2048-bit RSA key for one token generation.
//
// Returns:
//   - *rsa.PrivateKey: generated private key.
//   - error: cryptographic randomness or key-generation failure.
func generatePrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// instancePrincipalConfigurationProvider widens the OCI SDK factory result to
// the common configuration-provider interface used by dependencies.
//
// Returns:
//   - common.ConfigurationProvider: instance-principal provider.
//   - error: OCI instance-principal initialization failure.
func instancePrincipalConfigurationProvider() (common.ConfigurationProvider, error) {
	return auth.InstancePrincipalConfigurationProvider()
}

// resourcePrincipalConfigurationProvider widens the OCI SDK factory result to
// the common configuration-provider interface used by dependencies.
//
// Returns:
//   - common.ConfigurationProvider: resource-principal provider.
//   - error: OCI resource-principal initialization failure.
func resourcePrincipalConfigurationProvider() (common.ConfigurationProvider, error) {
	return auth.ResourcePrincipalConfigurationProvider()
}

// workloadIdentityConfigurationProvider widens the OCI SDK factory result to
// the common configuration-provider interface used by dependencies.
//
// Returns:
//   - common.ConfigurationProvider: OKE workload-identity provider.
//   - error: OCI workload-identity initialization failure.
func workloadIdentityConfigurationProvider() (common.ConfigurationProvider, error) {
	return auth.OkeWorkloadIdentityConfigurationProvider()
}

// newProvider validates config and builds a provider with injected dependencies.
// Nil provider and client checks guard the private test/configuration seams from
// returning unusable values without errors.
//
// Parameters:
//   - config: provider configuration to normalize.
//   - deps: external factories and deterministic lifecycle dependencies.
//
// Returns:
//   - *Provider: initialized provider.
//   - error: validation, principal, or client construction failure.
func newProvider(config Config, deps dependencies) (*Provider, error) {
	config, err := config.normalized()
	if err != nil {
		return nil, err
	}
	baseProvider, err := configurationProvider(config, deps)
	if err != nil {
		return nil, err
	}
	if baseProvider == nil {
		return nil, fmt.Errorf("create OCI configuration provider: provider is nil")
	}
	client, err := deps.newClient(baseProvider, config.Region)
	if err != nil {
		return nil, fmt.Errorf("create OCI identity data plane client: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("create OCI identity data plane client: client is nil")
	}
	return &Provider{
		client:              client,
		scope:               config.Scope,
		refreshBeforeExpiry: config.RefreshBeforeExpiry,
		now:                 deps.now,
		newKey:              deps.newKey,
	}, nil
}

// normalized trims and validates configuration and derives Scope when needed.
//
// Returns:
//   - Config: normalized configuration.
//   - error: missing or unsupported principal and scope configuration.
func (config Config) normalized() (Config, error) {
	config.Principal = Principal(strings.ToLower(strings.TrimSpace(string(config.Principal))))
	config.CompartmentOCID = strings.TrimSpace(config.CompartmentOCID)
	config.DatabaseOCID = strings.TrimSpace(config.DatabaseOCID)
	config.Scope = strings.TrimSpace(config.Scope)
	config.Region = strings.TrimSpace(config.Region)
	config.ConfigFile = strings.TrimSpace(config.ConfigFile)
	config.ConfigProfile = strings.TrimSpace(config.ConfigProfile)
	if config.RefreshBeforeExpiry < 0 {
		return Config{}, fmt.Errorf("OCI IAM database token provider requires non-negative RefreshBeforeExpiry")
	}
	if config.RefreshBeforeExpiry == 0 {
		config.RefreshBeforeExpiry = defaultRefreshBeforeExpiry
	}

	switch config.Principal {
	case InstancePrincipal, ResourcePrincipal, OKEWorkloadIdentity, ConfigProfile:
	case "":
		return Config{}, fmt.Errorf("OCI IAM database token provider requires Principal")
	default:
		return Config{}, fmt.Errorf("unsupported OCI IAM principal %q", config.Principal)
	}
	if config.Scope == "" {
		if config.CompartmentOCID == "" {
			return Config{}, fmt.Errorf("OCI IAM database token provider requires CompartmentOCID when Scope is not set")
		}
		if config.DatabaseOCID == "" {
			return Config{}, fmt.Errorf("OCI IAM database token provider requires DatabaseOCID when Scope is not set")
		}
		config.Scope = fmt.Sprintf("urn:oracle:db::id::%s::%s", config.CompartmentOCID, config.DatabaseOCID)
	}
	if err := validateDatabaseScope(config.Scope); err != nil {
		return Config{}, err
	}
	if config.Principal == ConfigProfile && config.ConfigProfile == "" {
		return Config{}, fmt.Errorf("OCI IAM config_profile principal requires ConfigProfile")
	}
	return config, nil
}

// validateDatabaseScope accepts the documented OCI database-token ID and path
// URN families while leaving resource-specific authorization to OCI.
//
// Parameters:
//   - scope: normalized explicit or derived token scope.
//
// Returns:
//   - error: unsupported prefix, empty target, whitespace, or control character.
func validateDatabaseScope(scope string) error {
	const (
		idPrefix   = "urn:oracle:db::id::"
		pathPrefix = "urn:oracle:db::path::"
	)
	target := ""
	switch {
	case strings.HasPrefix(scope, idPrefix):
		target = strings.TrimPrefix(scope, idPrefix)
	case strings.HasPrefix(scope, pathPrefix):
		target = strings.TrimPrefix(scope, pathPrefix)
	default:
		return fmt.Errorf("OCI IAM database token scope must use %q or %q", idPrefix, pathPrefix)
	}
	if target == "" {
		return fmt.Errorf("OCI IAM database token scope requires a target")
	}
	if strings.IndexFunc(scope, func(value rune) bool {
		return unicode.IsSpace(value) || unicode.IsControl(value)
	}) >= 0 {
		return fmt.Errorf("OCI IAM database token scope must not contain whitespace or control characters")
	}
	return nil
}

// configurationProvider selects the configured OCI principal factory. The
// default branch protects direct internal calls that bypass normalized.
//
// Parameters:
//   - config: normalized principal and profile configuration.
//   - deps: injected OCI principal factories.
//
// Returns:
//   - common.ConfigurationProvider: selected OCI credential provider.
//   - error: principal initialization or unsupported-principal failure.
func configurationProvider(config Config, deps dependencies) (common.ConfigurationProvider, error) {
	switch config.Principal {
	case InstancePrincipal:
		provider, err := deps.instancePrincipal()
		if err != nil {
			return nil, fmt.Errorf("create OCI instance principal configuration provider: %w", err)
		}
		return provider, nil
	case ResourcePrincipal:
		provider, err := deps.resourcePrincipal()
		if err != nil {
			return nil, fmt.Errorf("create OCI resource principal configuration provider: %w", err)
		}
		return provider, nil
	case OKEWorkloadIdentity:
		provider, err := deps.workloadIdentity()
		if err != nil {
			return nil, fmt.Errorf("create OCI OKE workload identity configuration provider: %w", err)
		}
		return provider, nil
	case ConfigProfile:
		return deps.configProfile(config.ConfigFile, config.ConfigProfile), nil
	default:
		return nil, fmt.Errorf("unsupported OCI IAM principal %q", config.Principal)
	}
}

// newDataplaneClient creates the OCI Identity Data Plane client. Region
// overrides are applied through the configuration provider before construction
// so the SDK initializes the client consistently.
//
// Parameters:
//   - provider: OCI request-signing configuration.
//   - region: optional endpoint region override.
//
// Returns:
//   - scopedTokenClient: initialized client pointer.
//   - error: OCI client construction failure; no client is returned on error.
func newDataplaneClient(provider common.ConfigurationProvider, region string) (scopedTokenClient, error) {
	if region != "" {
		provider = regionalConfigurationProvider{ConfigurationProvider: provider, region: region}
	}
	client, err := identitydataplane.NewDataplaneClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// regionalConfigurationProvider overrides Region while preserving optional
// refreshable-auth behavior from the wrapped provider.
type regionalConfigurationProvider struct {
	// ConfigurationProvider supplies all credentials except the region override.
	common.ConfigurationProvider
	// region is returned to OCI SDK client construction.
	region string
}

// Region returns the explicit region override.
//
// Returns:
//   - string: configured OCI region.
//   - error: always nil.
func (provider regionalConfigurationProvider) Region() (string, error) {
	return provider.region, nil
}

// Refreshable preserves the wrapped provider's credential-refresh capability.
//
// Returns:
//   - bool: true when the wrapped provider supports refreshable authentication.
func (provider regionalConfigurationProvider) Refreshable() bool {
	refreshable, ok := provider.ConfigurationProvider.(common.RefreshableConfigurationProvider)
	return ok && refreshable.Refreshable()
}

// Token returns the cached token if it is still fresh; otherwise, it obtains a
// replacement. One caller performs refresh work while other callers wait on a
// completion channel without holding mu. Waiters loop to recheck shared state
// because the active refresh may fail or return a token near its refresh window.
//
// Parameters:
//   - ctx: controls OCI token issuance and waiting for another refresh.
//
// Returns:
//   - string: fresh OCI IAM database token.
//   - error: cancellation, key generation, OCI request, or token validation failure.
func (provider *Provider) Token(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		provider.mu.Lock()
		now := provider.now()
		if provider.token != "" && now.Add(provider.refreshBeforeExpiry).Before(provider.expires) {
			token := provider.token
			provider.mu.Unlock()
			return token, nil
		}
		provider.pruneExpiredGenerationsLocked(now)
		// A waiter snapshots the active completion channel before releasing mu.
		// Closing the channel wakes every waiter without serializing them on I/O.
		if provider.refreshing {
			done := provider.refreshDone
			provider.mu.Unlock()
			slog.DebugContext(ctx, "waiting for OCI IAM database token refresh")
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-done:
				continue
			}
		}
		provider.refreshing = true
		provider.refreshDone = make(chan struct{})
		done := provider.refreshDone
		provider.mu.Unlock()
		slog.DebugContext(ctx, "starting OCI IAM database token refresh")

		// RSA generation and the OCI request run without mu so retained keys
		// remain available and waiters can honor their own cancellation.

		token, generation, err := provider.requestToken(ctx, now)
		if err == nil {
			slog.DebugContext(ctx, "completed OCI IAM database token refresh", "expires", generation.expires)
		}

		// Publish the result and close the leader's channel atomically under mu.
		provider.mu.Lock()
		if err == nil {
			if provider.generations == nil {
				provider.generations = make(map[[sha256.Size]byte]tokenGeneration)
			}
			provider.token = token
			provider.expires = generation.expires
			provider.generations[sha256.Sum256([]byte(token))] = generation
		}
		provider.refreshing = false
		provider.refreshDone = nil
		close(done)
		provider.mu.Unlock()
		if err != nil {
			return "", err
		}
		return token, nil
	}
}

// PrivateKeyForToken returns a copy of the PEM-encoded private key retained for
// token. Unknown and expired tokens are rejected.
//
// Parameters:
//   - ctx: controls cancellation while entering the generation store.
//   - token: exact token previously returned by Token.
//
// Returns:
//   - []byte: PKCS#8 PEM private key associated with token.
//   - error: cancellation or unknown/expired-token failure.
func (provider *Provider) PrivateKeyForToken(ctx context.Context, token string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	provider.pruneExpiredGenerationsLocked(provider.now())
	generation, ok := provider.generations[sha256.Sum256([]byte(token))]
	if !ok {
		return nil, fmt.Errorf("no OCI IAM database private key for token")
	}
	return bytes.Clone(generation.privateKeyPEM), nil
}

// requestToken generates a key pair and asks OCI for a token scoped to its
// public key. The private key is marshalled once and retained with the result.
//
// Parameters:
//   - ctx: controls OCI request cancellation.
//   - now: reference time used to reject an already-expired response.
//
// Returns:
//   - string: issued token.
//   - tokenGeneration: matching private key and expiry.
//   - error: key generation, encoding, OCI request, or JWT validation failure.
func (provider *Provider) requestToken(ctx context.Context, now time.Time) (string, tokenGeneration, error) {
	var generation tokenGeneration
	if err := ctx.Err(); err != nil {
		return "", generation, err
	}
	key, err := provider.newKey()
	if err != nil {
		return "", generation, fmt.Errorf("generate OCI IAM database token key: %w", err)
	}
	if key == nil {
		return "", generation, fmt.Errorf("generate OCI IAM database token key: key is nil")
	}
	privateKeyPEM, err := marshalPrivateKey(key)
	if err != nil {
		return "", generation, err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", generation, fmt.Errorf("encode OCI IAM database token public key: %w", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	response, err := provider.client.GenerateScopedAccessToken(ctx, identitydataplane.GenerateScopedAccessTokenRequest{
		GenerateScopedAccessTokenDetails: identitydataplane.GenerateScopedAccessTokenDetails{
			Scope:     common.String(provider.scope),
			PublicKey: common.String(string(publicPEM)),
		},
	})
	if err != nil {
		return "", generation, fmt.Errorf("get OCI IAM database token: %w", err)
	}
	// Token is optional in the generated SDK response model, so validate it
	// before parsing claims or publishing a generation.
	if response.Token == nil || *response.Token == "" {
		return "", generation, fmt.Errorf("identity data plane returned an empty OCI IAM database token")
	}
	expires, err := jwtExpiration(*response.Token)
	if err != nil {
		return "", generation, fmt.Errorf("read OCI IAM database token expiration: %w", err)
	}
	if !expires.After(now) {
		return "", generation, fmt.Errorf("identity data plane returned an expired OCI IAM database token")
	}
	return *response.Token, tokenGeneration{privateKeyPEM: privateKeyPEM, expires: expires}, nil
}

// pruneExpiredGenerationsLocked removes key material whose token has expired.
// The caller must hold provider.mu.
//
// Parameters:
//   - now: generations expiring at or before this time are removed.
func (provider *Provider) pruneExpiredGenerationsLocked(now time.Time) {
	pruned := 0
	for id, generation := range provider.generations {
		if !generation.expires.After(now) {
			delete(provider.generations, id)
			pruned++
		}
	}
	if pruned != 0 {
		slog.Debug("pruned expired OCI IAM database token keys", "count", pruned)
	}
}

// marshalPrivateKey encodes an RSA private key as PKCS#8 PEM. The nil check
// protects the injected key-generation seam from returning nil without error.
//
// Parameters:
//   - key: RSA private key to encode.
//
// Returns:
//   - []byte: PKCS#8 PEM bytes.
//   - error: nil key or PKCS#8 encoding failure.
func marshalPrivateKey(key *rsa.PrivateKey) ([]byte, error) {
	if key == nil {
		return nil, fmt.Errorf("encode OCI IAM database token private key: key is nil")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode OCI IAM database token private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// jwtExpiration extracts the exp claim from a compact JWT without validating
// its signature; OCI has already authenticated the response transport.
//
// Parameters:
//   - token: compact JWT returned by OCI.
//
// Returns:
//   - time.Time: token expiration.
//   - error: malformed encoding, claims, or missing exp.
func jwtExpiration(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("database token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims struct {
		ExpiresAt int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("decode JWT claims: %w", err)
	}
	if claims.ExpiresAt == 0 {
		return time.Time{}, fmt.Errorf("JWT has no exp claim")
	}
	return time.Unix(claims.ExpiresAt, 0), nil
}
