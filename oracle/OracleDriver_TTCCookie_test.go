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

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

// TestDriver_TTCCookieTTIRENEG verifies cookie caching, server-requested
// renegotiation, and reuse of the replacement cookie with password authentication.
func TestDriver_TTCCookieTTIRENEG(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	config := NewOracleDriverConfig()
	config.ConnectDescriptor = TestingConfig.GetConnectionString()
	config.DriverProperties.TTCCookieOptimizationEnabled = true
	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	ttc.ClearTTCCookieCacheForTest()
	t.Cleanup(ttc.ClearTTCCookieCacheForTest)

	// The first connection negotiates normally and creates the cached cookie.
	first := connectAndCloseCookieTest(t, connector)
	if !first.eligible {
		t.Skip("Server did not provide a valid database UUID")
	}
	if first.cacheHit || first.usedFallback {
		t.Fatal("first connection did not use normal negotiation")
	}
	originalVersion, found := ttc.TTCCookieProtocolVersionForTest(first.databaseUUID)
	if !found || originalVersion == 0 {
		t.Fatal("first connection did not cache a valid cookie")
	}

	// A stale cookie must trigger TTIRENEG and recover through normal negotiation.
	if _, found := ttc.CorruptTTCCookieForTest(first.databaseUUID); !found {
		t.Fatal("no cached cookie to make stale")
	}
	recovered := connectAndCloseCookieTest(t, connector)
	if recovered.databaseUUID != first.databaseUUID || !recovered.cacheHit || !recovered.usedFallback {
		t.Fatal("stale cookie did not cause TTIRENEG and normal fallback for the same database")
	}
	version, found := ttc.TTCCookieProtocolVersionForTest(first.databaseUUID)
	if !found || version != originalVersion {
		t.Fatalf("cached protocol version = %d, found=%t; want %d after renegotiation", version, found, originalVersion)
	}

	// The next connection must use the replacement cookie without falling back.
	reused := connectAndCloseCookieTest(t, connector)
	if reused.databaseUUID != first.databaseUUID || !reused.cacheHit || reused.usedFallback {
		t.Fatal("replacement cookie was not accepted for the same database")
	}
}

// TestDriver_TTCCookieOCIToken exercises cookie acceptance and server-driven
// re-negotiation with a real OCI IAM token connection.
func TestDriver_TTCCookieOCIToken(t *testing.T) {
	connectDescriptor := os.Getenv("ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR")
	tokenLocation := os.Getenv("ORACLE_GO_OCI_TOKEN_LOCATION")
	if connectDescriptor == "" || tokenLocation == "" {
		t.Skip("OCI token test requires ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR and ORACLE_GO_OCI_TOKEN_LOCATION")
	}

	cfg := NewOracleDriverConfig()
	cfg.ConnectDescriptor = connectDescriptor
	cfg.DriverProperties.TTCCookieOptimizationEnabled = true
	connector, err := NewOracleConnector(cfg)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	registrar, ok := connector.(oracleProviders.ProviderRegistrar)
	if !ok {
		t.Fatal("connector does not support provider registration")
	}
	registrar.RegisterProvider(testOCITokenProvider{
		tokenPath:      filepath.Join(tokenLocation, testOCITokenFileName),
		privateKeyPath: filepath.Join(tokenLocation, testOCIPrivateKeyFileName),
	})
	ttc.ClearTTCCookieCacheForTest()
	t.Cleanup(ttc.ClearTTCCookieCacheForTest)

	// The first connection negotiates normally and creates the cached cookie.
	first := connectAndCloseCookieTest(t, connector)
	if !first.eligible {
		t.Skip("Server did not provide a valid database UUID")
	}
	if first.cacheHit || first.usedFallback {
		t.Fatal("first connection did not use normal negotiation")
	}
	originalVersion, found := ttc.TTCCookieProtocolVersionForTest(first.databaseUUID)
	if !found || originalVersion == 0 {
		t.Fatal("first connection did not cache a valid cookie")
	}

	// A stale cookie must trigger TTIRENEG and recover through normal negotiation.
	if _, found := ttc.CorruptTTCCookieForTest(first.databaseUUID); !found {
		t.Fatal("no cached cookie to make stale")
	}
	recovered := connectAndCloseCookieTest(t, connector)
	if recovered.databaseUUID != first.databaseUUID || !recovered.cacheHit || !recovered.usedFallback {
		t.Fatal("stale cookie did not cause TTIRENEG and normal fallback for the same database")
	}
	version, found := ttc.TTCCookieProtocolVersionForTest(first.databaseUUID)
	if !found || version != originalVersion {
		t.Fatalf("cached protocol version = %d, found=%t; want %d after renegotiation", version, found, originalVersion)
	}

	// The next connection must use the replacement cookie without falling back.
	reused := connectAndCloseCookieTest(t, connector)
	if reused.databaseUUID != first.databaseUUID || !reused.cacheHit || reused.usedFallback {
		t.Fatal("replacement cookie was not accepted for the same database")
	}
}

// cookieConnectionResult identifies the negotiation path used by one connection.
type cookieConnectionResult struct {
	databaseUUID string
	eligible     bool
	cacheHit     bool
	usedFallback bool
}

// connectAndCloseCookieTest opens a fresh pool, connects, and closes it so every
// call establishes a new physical connection. It observes the real instantiator
// factory to distinguish cookie use from normal negotiation; no responses are mocked.
func connectAndCloseCookieTest(t *testing.T, c driver.Connector) cookieConnectionResult {
	t.Helper()
	connector, ok := c.(*connector)
	if !ok {
		t.Fatalf("unexpected connector type: %T", c)
	}
	if !connector.connectorConfig.DriverProperties.TTCCookieOptimizationEnabled {
		t.Fatal("cookie optimization was disabled by external configuration")
	}
	var result cookieConnectionResult
	factory := connector.connInstantiatorFactory
	defer func() { connector.connInstantiatorFactory = factory }()
	connector.connInstantiatorFactory = func(
		config *oracleconfig.OracleDriverConfig,
		ns driverCommon.NetworkSession,
		providers common.Registry[oracleProviders.Provider],
		renegotiating bool,
	) (driverCommon.ConnectionInstantiator, error) {
		if renegotiating {
			result.usedFallback = true
		} else {
			result.databaseUUID = ns.GetDatabaseUUID()
			result.eligible = ttc.IsTTCCookieEligible(config.DriverProperties, result.databaseUUID)
			if result.eligible {
				_, result.cacheHit = ttc.GetCachedTTCCookie(result.databaseUUID)
			}
		}
		return factory(config, ns, providers, renegotiating)
	}

	db := sql.OpenDB(connector)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("connect: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return result
}
