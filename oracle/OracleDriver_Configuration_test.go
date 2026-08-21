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
	"testing"
)

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

	c := NewOracleDriverConfig()
	c.Credentials.User = "foo"
	c.Credentials.Password = "bar"
	c.ConnectDescriptor = "foo/@localhost:1521/srv"

	_, err := NewOracleConnector(c)
	if err == nil {
		t.Fatalf("Should have receive an error")
	}
	t.Log("received an error", err.Error())

}

// TestDriver_ConfigurationLogging dummy test to activate logging.
func TestDriver_ConfigurationLogging(t *testing.T) {
	t.Parallel()
	loggingConfig := NewOracleLoggingConfig()
	loggingConfig.Destination = "STDOUT"
	loggingConfig.Level = "DEBUG"
	GetDefaultDriver().ApplyDriverLoggingConfig(loggingConfig)

	defer func() {
		GetDefaultDriver().ApplyDriverLoggingConfig(NewOracleLoggingConfig())
	}()
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
	if oracleConnector.connectorConfig.ConnectDescriptor != "localhost:1521/freepdb1" {
		t.Fatalf("expected effective connect descriptor, got %q", oracleConnector.connectorConfig.ConnectDescriptor)
	}
	if oracleConnector.connectorConfig.ConnectionProperties.ConnectTimeout != 12 {
		t.Fatalf("expected connect timeout from fallback DSN query, got %d", oracleConnector.connectorConfig.ConnectionProperties.ConnectTimeout)
	}
	if oracleConnector.config.ConnectString == "" {
		t.Fatal("expected parsed network connect string")
	}
}

// TestDriver_OpenConnectorStoresConnectDescriptorFromDSN verifies that
// credentials and the connect descriptor are stored from the DSN.
func TestDriver_OpenConnectorStoresConnectDescriptorFromDSN(t *testing.T) {
	t.Parallel()
	connector, err := NewDriver().openConnector("user/pass@localhost:1521/freepdb1")
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if oracleConnector.connectorConfig.ConnectDescriptor != "localhost:1521/freepdb1" {
		t.Fatalf("expected DSN connect descriptor, got %q", oracleConnector.connectorConfig.ConnectDescriptor)
	}
	if oracleConnector.connectorConfig.Credentials.User != "user" {
		t.Fatalf("expected DSN user, got %q", oracleConnector.connectorConfig.Credentials.User)
	}
	if oracleConnector.connectorConfig.Credentials.Password != "pass" {
		t.Fatalf("expected DSN password, got %q", oracleConnector.connectorConfig.Credentials.Password)
	}
}

// TestDriver_OpenConnectorUsesNSProperty verifies that Oracle Net properties
// from the configuration object are applied to parsed connection options.
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
	iterator := oracleConnector.config.NewConnectionAttemptIterator(context.Background())
	if !iterator.HasNext() {
		t.Fatal("expected a connection attempt")
	}
	option := iterator.Next()
	desc := option.Description
	if desc == nil {
		t.Fatal("expected parsed description")
	}
	if desc.Security.WalletLocation != walletLocation {
		t.Fatalf("expected wallet location %q, got %q", walletLocation, desc.Security.WalletLocation)
	}
}

// TestDriver_OpenConnectorUsesNSParamOverConfig verifies
// that a ns property takes precedence
// over the configuration object.
func TestDriver_OpenConnectorUsesNSParamOverConfig(t *testing.T) {
	t.Parallel()
	const configTransportConnectTimeout = 2
	const dsnTransportConnectTimeout = 7
	const expectedTransportConnectTimeoutMS = dsnTransportConnectTimeout * 1000

	config := NewOracleDriverConfig()
	config.ConnectDescriptor = "tcps://127.0.0.1:2484/freepdb1?transport_connect_timeout=7"
	config.ConnectionProperties.TransportConnectTimeout = configTransportConnectTimeout

	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	if oracleConnector.connectorConfig.ConnectionProperties.TransportConnectTimeout != dsnTransportConnectTimeout {
		t.Fatalf(
			"expected connector config transport connect timeout %d, got %d",
			dsnTransportConnectTimeout,
			oracleConnector.connectorConfig.ConnectionProperties.TransportConnectTimeout,
		)
	}

	iterator := oracleConnector.config.NewConnectionAttemptIterator(context.Background())
	if !iterator.HasNext() {
		t.Fatal("expected a connection attempt")
	}
	option := iterator.Next()
	desc := option.Description
	if desc == nil {
		t.Fatal("expected parsed description")
	}
	if desc.TransportConnectTimeout != expectedTransportConnectTimeoutMS {
		t.Fatalf(
			"expected transport connect timeout %dms, got %dms",
			expectedTransportConnectTimeoutMS,
			desc.TransportConnectTimeout,
		)
	}
}

// TestDriver_OpenConnectorUsesParam verifies that fully-qualified DSN query
// parameters are applied to parsed connection options.
func TestDriver_OpenConnectorUsesParam(t *testing.T) {
	t.Parallel()
	const walletLocation = "/tmp/foo"

	connector, err := GetDefaultDriver().openConnector("tcps://127.0.0.1:2484/freepdb1?oracle.go.ConnectionProperties.WalletLocation=/tmp/foo")
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	iterator := oracleConnector.config.NewConnectionAttemptIterator(context.Background())
	if !iterator.HasNext() {
		t.Fatal("expected a connection attempt")
	}
	option := iterator.Next()
	desc := option.Description
	if desc == nil {
		t.Fatal("expected parsed description")
	}
	if desc.Security.WalletLocation != walletLocation {
		t.Fatalf("expected wallet location %q, got %q", walletLocation, desc.Security.WalletLocation)
	}
}

// TestDriver_OpenConnectorUsesNSParam verifies that Oracle Net DSN query
// parameters are applied to parsed connection options and credentials.
func TestDriver_OpenConnectorUsesNSParam(t *testing.T) {
	t.Parallel()
	const walletLocation = "/tmp/foo"
	const logonMode = "SYSOPER"

	connector, err := GetDefaultDriver().openConnector("tcps://127.0.0.1:2484/freepdb1?wallet_location=" + walletLocation + "&logon_mode=" + logonMode)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)

	iterator := oracleConnector.config.NewConnectionAttemptIterator(context.Background())
	if !iterator.HasNext() {
		t.Fatal("expected a connection attempt")
	}
	option := iterator.Next()
	desc := option.Description
	if desc == nil {
		t.Fatal("expected parsed description")
	}
	if desc.Security.WalletLocation != walletLocation {
		t.Fatalf("expected wallet location %q, got %q", walletLocation, desc.Security.WalletLocation)
	}

	if oracleConnector.connectorConfig.Credentials.LogonMode != logonMode {
		t.Fatalf("expected logon mode  %q, got %q", logonMode, oracleConnector.connectorConfig.Credentials.LogonMode)
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
