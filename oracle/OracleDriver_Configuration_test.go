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
	"flag"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

// TestDriver_ConfigurationConnectsWithDifferentDSNs verifies remote
// connectivity through both EZConnect and TNS descriptor input while using
// the same credentials and connection configuration.
func TestDriver_ConfigurationConnectsWithDifferentDSNs(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No database configuration available")
	}

	protocol := strings.ToLower(strings.TrimSpace(TestingConfig.Database.Protocol))
	if protocol == "" {
		protocol = "tcp"
	}
	ezConnectDSN := ""
	if serviceName := strings.TrimSpace(TestingConfig.Database.ServiceName); serviceName != "" {
		ezConnectDSN = fmt.Sprintf("%s://%s:%d/%s", protocol, TestingConfig.Database.Host, TestingConfig.Database.Port, serviceName)
		if instanceName := strings.TrimSpace(TestingConfig.Database.InstanceName); instanceName != "" {
			ezConnectDSN += "/" + instanceName
		}
	}

	TNS_complex_dsn := fmt.Sprintf(`(DESCRIPTION_LIST=
		          (LOAD_BALANCE=off)
	             (FAILOVER=on)
	             (DESCRIPTION=
		               (CONNECT_TIMEOUT=50)
	                  (RETRY_COUNT=2)
	                  (ADDRESS_LIST=
		                   (LOAD_BALANCE=on)
	                      (ADDRESS=(PROTOCOL=%[1]s)(HOST=%[2]s)(PORT=%[6]d))
	                      (ADDRESS=(PROTOCOL=%[1]s)(HOST=%[2]s)(PORT=%[5]d))
                      )
	                  (CONNECT_DATA=(SERVICE_NAME=orders.example.com))
                      (SECURITY=(SSL_SERVER_DN_MATCH=yes)(SSL_SERVER_CERT_DN="CN=orders-primary, OU=DB"))
                 )
                 (DESCRIPTION=
                      (CONNECT_TIMEOUT=300)
	                  (RETRY_COUNT=0)
                      (ADDRESS=(PROTOCOL=%[1]s)(HOST=%[2]s)(PORT=%[4]d))
	                  (CONNECT_DATA=(SERVICE_NAME=%[3]s))
                 )
               )`, TestingConfig.Database.Protocol,
		           TestingConfig.Database.Host, TestingConfig.Database.ServiceName,
				   TestingConfig.Database.Port, TestingConfig.Database.Port+1, TestingConfig.Database.Port+2)

	EZ_complex_dsn := fmt.Sprintf("%[1]s://%[2]s:%[4]d,%[2]s:%[5]d/%[3]s?connect_timeout=50&retry_count=2",
		TestingConfig.Database.Protocol, TestingConfig.Database.Host, TestingConfig.Database.ServiceName,
		TestingConfig.Database.Port, TestingConfig.Database.Port + 1)
	
    dsns := []struct {
		name string
		dsn  string
	}{
		{name: "EZConnect", dsn: ezConnectDSN},
		{name: "TNS", dsn: TestingConfig.GetConnectionDSN()},
		{name: "complexTNS", dsn: TNS_complex_dsn},
		{name: "complexEZ", dsn: EZ_complex_dsn},
	}
	for _, dsn := range dsns {
		t.Run(dsn.name, func(t *testing.T) {
			if dsn.dsn == "" {
				if dsn.name == "EZConnect" {
					t.Skip("the test database configuration does not provide a service name for EZConnect")
				}
				t.Fatal("the test database configuration does not provide a TNS descriptor")
			}

			config := NewOracleDriverConfig()
			config.ConnectDescriptor = dsn.dsn
			config.Credentials.User = TestingConfig.Credentials.Username
			config.Credentials.Password = TestingConfig.Credentials.Password
			config.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
			config.ConnectionProperties.HttpsProxy = TestingConfig.Database.HTTPSProxy
			if TestingConfig.Database.HTTPSProxyPort > 0 {
				config.ConnectionProperties.HttpsProxyPort = int(TestingConfig.Database.HTTPSProxyPort)
			}

			connector, err := NewOracleConnector(config)
			if err != nil {
				t.Fatalf("creating connector: %v", err)
			}
			db := sql.OpenDB(connector)
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Errorf("closing database: %v", err)
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			err = db.PingContext(ctx)
			if err != nil {
				t.Fatalf("connection using %s DSN failed: %v", dsn.name, err)
			}
		})
	}
}

// TestDriver_ConfigurationWithConnectorBasic verifies that a connector can be
// created from an explicit configuration object.
func TestDriver_ConfigurationWithConnectorBasic(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	c := NewOracleDriverConfig()
	c.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
	c.Credentials.User = TestingConfig.Credentials.Username
	c.Credentials.Password = TestingConfig.Credentials.Password
	c.ConnectDescriptor = TestingConfig.GetConnectionDSN()

	t.Logf("connecting to %v\n", c)
	connector, err := NewOracleConnector(c)
	if err != nil {
		t.Fatalf("failed to create connector  : %v", err)
	}
	db := sql.OpenDB(connector)
	err = db.Ping()
	if err != nil {
		t.Fatalf("failed to connect connector  : %v", err)
	}

	rows, err := db.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
	if err != nil {
		t.Fatalf("select from DUAL failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("no row returned from DUAL")
	}
}

// TestDriver_ConfigurationWithConnectorWithEnvOverwrite verifies that
// credentials from environment variables are applied to connector configuration.
func TestDriver_ConfigurationWithConnectorWithEnvOverwrite(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	c := NewOracleDriverConfig()
	c.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
	c.ConnectDescriptor = TestingConfig.GetConnectionDSN()

	// that should make the connection possible
	t.Setenv("ORACLE_GO_CREDENTIALS_USER", TestingConfig.Credentials.Username)
	t.Setenv("ORACLE_GO_CREDENTIALS_PASSWORD", TestingConfig.Credentials.Password)

	t.Logf("connecting to %v\n", c)
	connector, err := NewOracleConnector(c)
	if err != nil {
		t.Fatalf("failed to create connector  : %v", err)
	}
	db := sql.OpenDB(connector)
	defer func() {
		db.Close()
	}()
	err = db.Ping()
	if err != nil {
		t.Fatalf("failed to connect connector  : %v", err)
	}

}

// TestDriver_ConfigurationWithConnectorWithFlagOverwrite verifies that
// credentials from flags are applied to connector configuration.
func TestDriver_ConfigurationWithConnectorWithFlagOverwrite(t *testing.T) {

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	c := NewOracleDriverConfig()
	// password is sensitive and cannot be used as a flag
	c.Credentials.Password = TestingConfig.Credentials.Password
	c.ConnectDescriptor = TestingConfig.GetConnectionDSN()

	// that should make the connection possible
	flag.Set("oracle.go.Credentials.User", TestingConfig.Credentials.Username)
	flag.Set("oracle.go.Credentials.LogonMode", TestingConfig.Credentials.LogonMode)

	defer func() {
		flag.Set("oracle.go.Credentials.User", "")
		flag.Set("oracle.go.Credentials.Password", "")
		flag.Set("oracle.go.Credentials.LogonMode", "")
	}()

	t.Logf("connecting to %v\n", c)
	connector, err := NewOracleConnector(c)
	if err != nil {
		t.Fatalf("failed to create connector  : %v", err)
	}
	db := sql.OpenDB(connector)
	defer func() {
		db.Close()
	}()
	err = db.Ping()
	if err != nil {
		t.Fatalf("failed to connect connector  : %v", err)
	}

}

// TestDriver_ConfigurationWithConnectorWithDsnNegative
// verifies that dsn have precedence over configuration
// when credentials are specified in dsn, they cannot be present in configuration
func TestDriver_ConfigurationWithCredentialsWithDsnNegative(t *testing.T) {
	t.Parallel()
	c := NewOracleDriverConfig()
	c.Credentials.User = "foo"
	c.Credentials.Password = "bar"
	c.ConnectDescriptor = "foo/@localhost:1521/srv"

	_, err := NewOracleConnector(c)
	if err == nil {
		t.Fatalf("Should have receive an error")
	}
	t.Log("received expected  error", err.Error())
}

// TestDriver_ConfigurationLogging dummy test to activate logging.
func TestDriver_ConfigurationLogging(t *testing.T) {
	// this must be exclusive as this is change the global reference to logger
	loggingConfig := NewOracleLoggingConfig()
	loggingConfig.Destination = "STDOUT"
	loggingConfig.Level = "DEBUG"
	GetDefaultDriver().ApplyDriverLoggingConfig(loggingConfig)

	defer func() {
		GetDefaultDriver().ApplyDriverLoggingConfig(NewOracleLoggingConfig())
	}()
}

// TestDriver_ApplyDriverLoggingConfigPreservedByOpenConnector verifies that
// opening a connector does not replace logging configured by the application.
func TestDriver_ApplyDriverLoggingConfigPreservedByOpenConnector(t *testing.T) {
	driver := NewDriver()
	loggingConfig := NewOracleLoggingConfig()
	loggingConfig.Destination = filepath.Join(t.TempDir(), "driver.log")
	loggingConfig.Level = "DEBUG"
	driver.ApplyDriverLoggingConfig(loggingConfig)
	t.Cleanup(func() {
		driver.ApplyDriverLoggingConfig(NewOracleLoggingConfig())
	})

	configuredLogger := common.Odl
	if !configuredLogger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected debug logging to be enabled after applying the configuration")
	}

	if _, err := driver.openConnector("localhost:1521/freepdb1"); err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	if common.Odl != configuredLogger {
		t.Fatal("openConnector replaced the logging configuration")
	}
	if !common.Odl.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected debug logging configuration to remain enabled after openConnector")
	}
}

type testingConnector struct {
	oracleConnector
}

// TestDriver_OpenConnectorUsesFallbackConnectDescriptor verifies that an empty
// DSN falls back to the connect descriptor from the driver configuration.
func TestDriver_OpenConnectorUsesFallbackConnectDescriptor(t *testing.T) {
	t.Parallel()
	config := NewOracleDriverConfig()
	config.ConnectDescriptor = "localhost:1521/freepdb1?oracle.go.ConnectionProperties.ConnectTimeout=12"

	connector, err := NewDriverWithConfig(config).openConnector("")
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}
	oracleConnector := connector.(*oracleConnector)
	if oracleConnector.connectionPlans.BaseConfig.ConnectDescriptor != "localhost:1521/freepdb1" {
		t.Fatalf("expected effective connect descriptor, got %q", oracleConnector.connectionPlans.BaseConfig.ConnectDescriptor)
	}
	if got := oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.ConnectTimeout; got != 12 {
		t.Fatalf("expected resolved connect timeout 12, got %d", got)
	}
	if len(oracleConnector.connectionPlans.Plan.Descriptions) == 0 {
		t.Fatal("expected a parsed connection description")
	}
}

// TestDriver_OpenConnectorStoresConnectDescriptorFromDSN verifies that
// credentials and the connect descriptor are stored from the DSN.
func TestDriver_OpenConnectorStoresConnectDescriptorFromDSN(t *testing.T) {
	connector, err := NewDriver().openConnector("user/pass@localhost:1521/freepdb1")
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if oracleConnector.connectionPlans.BaseConfig.ConnectDescriptor != "localhost:1521/freepdb1" {
		t.Fatalf("expected DSN connect descriptor, got %q", oracleConnector.connectionPlans.BaseConfig.ConnectDescriptor)
	}
	if oracleConnector.connectionPlans.BaseConfig.Credentials.User != "user" {
		t.Fatalf("expected DSN user, got %q", oracleConnector.connectionPlans.BaseConfig.Credentials.User)
	}
	if oracleConnector.connectionPlans.BaseConfig.Credentials.Password != "pass" {
		t.Fatalf("expected DSN password, got %q", oracleConnector.connectionPlans.BaseConfig.Credentials.Password)
	}
}

// TestDriver_OpenConnectorUsesNSProperty verifies that Oracle Net properties
// from the configuration object are retained in the connection plans' base config.
func TestDriver_OpenConnectorUsesNSProperty(t *testing.T) {
	t.Parallel()
	const walletLocation = "/tmp/wallet location"

	config := NewOracleDriverConfig()
	config.ConnectDescriptor = "tcps://127.0.0.1:2484/freepdb1"
	config.ConnectionProperties.WalletLocation = walletLocation

	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if got := oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.WalletLocation; got != walletLocation {
		t.Fatalf("expected wallet location %q in base configuration, got %q", walletLocation, got)
	}
}

// TestDriver_OpenConnectorUsesNSParamOverConfig verifies that an Oracle Net
// query setting is retained separately from the base configuration.
func TestDriver_OpenConnectorUsesNSParamOverConfig(t *testing.T) {
	t.Parallel()
	const configTransportConnectTimeout = 2
	const dsnTransportConnectTimeout = 7

	config := NewOracleDriverConfig()
	config.ConnectDescriptor = "tcps://127.0.0.1:2484/freepdb1?transport_connect_timeout=7"
	config.ConnectionProperties.TransportConnectTimeout = configTransportConnectTimeout

	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.TransportConnectTimeout != dsnTransportConnectTimeout {
		t.Fatalf("expected merged base transport connect timeout %d, got %d", dsnTransportConnectTimeout, oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.TransportConnectTimeout)
	}
	if config.ConnectionProperties.TransportConnectTimeout != configTransportConnectTimeout {
		t.Fatalf("openConnector changed the caller's transport connect timeout to %d", config.ConnectionProperties.TransportConnectTimeout)
	}
}

// TestDriver_OpenConnectorUsesParam verifies that fully-qualified DSN query
// parameters are resolved into the connection plans' base configuration.
func TestDriver_OpenConnectorUsesParam(t *testing.T) {
	t.Parallel()
	const walletLocation = "/tmp/foo"

	connector, err := GetDefaultDriver().openConnector("tcps://127.0.0.1:2484/freepdb1?oracle.go.ConnectionProperties.WalletLocation=/tmp/foo")
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if got := oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.WalletLocation; got != walletLocation {
		t.Fatalf("expected resolved wallet location %q, got %q", walletLocation, got)
	}
}

// TestDriver_OpenConnectorUsesNSParam verifies that Oracle Net DSN query
// parameters are resolved into the connection plans' base configuration.
func TestDriver_OpenConnectorUsesNSParam(t *testing.T) {
	t.Parallel()
	const walletLocation = "/tmp/foo"
	const logonMode = "SYSOPER"

	connector, err := GetDefaultDriver().openConnector("tcps://127.0.0.1:2484/freepdb1?wallet_location=" + walletLocation + "&logon_mode=" + logonMode)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)

	if got := oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.WalletLocation; got != walletLocation {
		t.Fatalf("expected resolved wallet location %q, got %q", walletLocation, got)
	}

	if got := oracleConnector.connectionPlans.BaseConfig.Credentials.LogonMode; got != logonMode {
		t.Fatalf("expected resolved logon mode %q, got %q", logonMode, got)
	}
}

// TestDriver_OpenConnectorPreservesQuotedInParameters verifies that a
// quoted configuration value with space retains its spaces
// expectations : quoting remain correct
func TestDriver_OpenConnectorPreservesQuotedInParameters(t *testing.T) {
	t.Parallel()
	const expected = "CN=My Server, OU=Org"
	const dsn = `tcps://127.0.0.1:1521/service?ssl_server_cert_dn="CN=My Server, OU=Org"`

	connector, err := NewDriver().openConnector(dsn)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if got := oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.SSLServerCertDN; got != expected {
		t.Fatalf("expected resolved SSL server certificate DN %q, got %q", expected, got)
	}
}

// TestDriver_OpenConnectorReturnsInvalidDSNParameterError verifies that invalid
// DSN query parameters fail connector creation.
func TestDriver_OpenConnectorReturnsInvalidDSNParameterError(t *testing.T) {
	t.Parallel()
	_, err := NewDriver().openConnector("user/pass@localhost:1521/freepdb1?oracle.go.ConnectionProperties.ConnectTimeout=-1")
	if err == nil {
		t.Fatal("expected invalid DSN parameter error")
	}
}
// TestDriver_OpenConnectorCommonParams verifies that parameters from configuration.
// that must be sent as connect data are actually present
func TestDriver_OpenConnectorCommonParamsFromConf(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := NewOracleDriverConfig()
	config.ConnectionProperties.ServerType = "DEDICATEd"
 	config.ConnectDescriptor = TestingConfig.GetConnectionString()


	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("creating connector: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if !strings.EqualFold(oracleConnector.connectionPlans.BaseConfig.ConnectionProperties.ServerType,"DEDICATEd") {
		t.Fatalf("serverType discard from base configuration")
	}
	iterator, err := oracleConnector.connectionPlans.NewConnectionAttemptIterator(context.Background(), "server-type-test")
	if err != nil {
		t.Fatalf("creating connection attempt iterator: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("resolving connection attempt: %v", err)
	}
	connectData := attempt.GetConnectData()
	serverType := attempt.GetEffectiveConfig().ConnectionProperties.ServerType
	if !strings.EqualFold(connectData.Server, serverType) {
		t.Fatalf("resolved config ServerType %q does not match CONNECT_DATA SERVER %q", serverType, connectData.Server)
	}
	if !strings.Contains(strings.ToUpper(attempt.GetConnectDescriptor()), "(SERVER="+strings.ToUpper(serverType)+")") {
		t.Fatalf("connect descriptor does not contain resolved server type %q: %s", serverType, attempt.GetConnectDescriptor())
	}

	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	err = db.PingContext(ctx)

	if err != nil {
		t.Fatalf("connection using server type failed: %v",err)
	}

}

// TestDriver_OpenConnectorCommonParams verifies that parameters from TNS DSN.
// that must be sent as connect data are actually present and configuration is updated
func TestDriver_OpenConnectorCommonParamsFromTSNDSN(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := NewOracleDriverConfig()
	config.ConnectDescriptor = fmt.Sprintf("%s/%s@(description=(address=(protocol=%s)(host=%s)(port=%d))(connect_data=(SERVER=DEDICATED)(service_name=%s)))",
	TestingConfig.Credentials.Username, TestingConfig.Credentials.Password ,
	TestingConfig.Database.Protocol,TestingConfig.Database.Host,TestingConfig.Database.Port,
	TestingConfig.Database.ServiceName)


	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("creating connector: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)

	iterator, err := oracleConnector.connectionPlans.NewConnectionAttemptIterator(context.Background(), "server-type-test")
	if err != nil {
		t.Fatalf("creating connection attempt iterator: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("resolving connection attempt: %v", err)
	}
	connectData := attempt.GetConnectData()
	serverType := attempt.GetEffectiveConfig().ConnectionProperties.ServerType
	if !strings.EqualFold(connectData.Server, "dedicated") {
		t.Fatalf("resolved config ServerType %q does not match CONNECT_DATA SERVER %q", serverType, connectData.Server)
	}
	if !strings.EqualFold(serverType,"dedicated") {
		t.Fatalf("resolved config ServerType %q does not match CONNECT_DATA SERVER %q", serverType, connectData.Server)
	}
	if !strings.Contains(strings.ToUpper(attempt.GetConnectDescriptor()), "(SERVER="+strings.ToUpper(serverType)+")") {
		t.Fatalf("connect descriptor does not contain resolved server type %q: %s", serverType, attempt.GetConnectDescriptor())
	}

	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	err = db.PingContext(ctx)

	if err != nil {
		t.Fatalf("connection using server type failed: %v",err)
	}

}

// TestDriver_OpenConnectorCommonParams verifies that parameters from EZ DSN.
// that must be sent as connect data are actually present and configuration is updated
func TestDriver_OpenConnectorCommonParamsFromEZCDSN(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := NewOracleDriverConfig()
	config.ConnectDescriptor = fmt.Sprintf("%s/%s@%s:%d/%s:dedicated",
		TestingConfig.Credentials.Username, TestingConfig.Credentials.Password ,
		TestingConfig.Database.Host,TestingConfig.Database.Port,
		TestingConfig.Database.ServiceName)


	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("creating connector: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)

	iterator, err := oracleConnector.connectionPlans.NewConnectionAttemptIterator(context.Background(), "server-type-test")
	if err != nil {
		t.Fatalf("creating connection attempt iterator: %v", err)
	}
	attempt, err := iterator.Next()
	if err != nil {
		t.Fatalf("resolving connection attempt: %v", err)
	}
	connectData := attempt.GetConnectData()
	serverType := attempt.GetEffectiveConfig().ConnectionProperties.ServerType
	if !strings.EqualFold(connectData.Server, "dedicated") {
		t.Fatalf("resolved config ServerType %q does not match CONNECT_DATA SERVER %q", serverType, connectData.Server)
	}
	if !strings.EqualFold(serverType,"dedicated") {
		t.Fatalf("resolved config ServerType %q does not match CONNECT_DATA SERVER %q", serverType, connectData.Server)
	}
	if !strings.Contains(strings.ToUpper(attempt.GetConnectDescriptor()), "(SERVER="+strings.ToUpper(serverType)+")") {
		t.Fatalf("connect descriptor does not contain resolved server type %q: %s", serverType, attempt.GetConnectDescriptor())
	}

	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	err = db.PingContext(ctx)

	if err != nil {
		t.Fatalf("connection using server type failed: %v",err)
	}
}
