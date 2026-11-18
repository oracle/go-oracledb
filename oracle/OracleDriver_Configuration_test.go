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
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

// withIsolatedOracleFlags is for nonparallel tests registered as Exclusive.
// Fresh values and a fresh visited set keep explicit overrides local to fn.
func withIsolatedOracleFlags(t *testing.T, fn func(*flag.FlagSet)) {
	t.Helper()
	original := flag.CommandLine
	isolated := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	isolated.SetOutput(io.Discard)
	for _, name := range []string{
		"oracle.go.Credentials.User",
		"oracle.go.Credentials.logon_mode",
		"oracle.go.Locale.Territory",
	} {
		registered := original.Lookup(name)
		if registered == nil {
			t.Fatalf("required Oracle flag %q is not registered", name)
		}
		getter, ok := registered.Value.(flag.Getter)
		if !ok {
			t.Fatalf("Oracle flag %q has no typed getter", name)
		}
		if _, ok := getter.Get().(string); !ok {
			t.Fatalf("Oracle flag %q is not a string flag", name)
		}
		isolated.String(name, registered.DefValue, registered.Usage)
	}
	if err := isolated.Parse(nil); err != nil {
		t.Fatalf("parse isolated Oracle flags: %v", err)
	}
	flag.CommandLine = isolated
	defer func() { flag.CommandLine = original }()
	fn(isolated)
}

// TestConfiguration_FlagIsolation verifies flag overrides work in isolated scopes
// without changing the original values, visited flags or later configurations.
func TestConfiguration_FlagIsolation(t *testing.T) {
	type flagState struct {
		values  map[string]string
		visited map[string]string
	}
	snapshot := func(fs *flag.FlagSet) flagState {
		state := flagState{values: map[string]string{}, visited: map[string]string{}}
		fs.VisitAll(func(f *flag.Flag) { state.values[f.Name] = f.Value.String() })
		fs.Visit(func(f *flag.Flag) { state.visited[f.Name] = f.Value.String() })
		return state
	}
	set := func(t *testing.T, fs *flag.FlagSet, name, value string) {
		t.Helper()
		if err := fs.Set(name, value); err != nil {
			t.Fatalf("set isolated flag %q: %v", name, err)
		}
	}
	original := flag.CommandLine
	before := snapshot(original)
	assertOriginal := func(t *testing.T) {
		t.Helper()
		if flag.CommandLine != original || !reflect.DeepEqual(snapshot(original), before) {
			t.Fatal("isolated test changed the original flag set, values or visited flags")
		}
	}
	t.Cleanup(func() { assertOriginal(t) })

	t.Run("unset_flags_preserve_configuration", func(t *testing.T) {
		withIsolatedOracleFlags(t, func(fs *flag.FlagSet) {
			config := NewOracleDriverConfig()
			config.Credentials.User = "configured_user"
			config.Locale.Territory = "AMERICA"
			if err := config.AssignFromFlags(); err != nil {
				t.Fatalf("assign unvisited flags: %v", err)
			}
			if fs.NFlag() != 0 || config.Credentials.User != "configured_user" || config.Locale.Territory != "AMERICA" {
				t.Fatal("unvisited flags changed supplied configuration")
			}
			set(t, fs, "oracle.go.Credentials.User", "flag_user")
			set(t, fs, "oracle.go.Credentials.logon_mode", "SYSDBA")
			set(t, fs, "oracle.go.Locale.Territory", "FOO")
			if err := config.AssignFromFlags(); err != nil {
				t.Fatalf("assign explicit flags: %v", err)
			}
			if config.Credentials.User != "flag_user" || config.Credentials.LogonMode != "SYSDBA" || config.Locale.Territory != "FOO" {
				t.Fatal("explicit flags did not override the isolated configuration")
			}
		})
		assertOriginal(t)
	})
	t.Run("empty_override_does_not_leak_to_next_configuration", func(t *testing.T) {
		withIsolatedOracleFlags(t, func(outer *flag.FlagSet) {
			withIsolatedOracleFlags(t, func(inner *flag.FlagSet) {
				set(t, inner, "oracle.go.Credentials.User", "old_user")
				set(t, inner, "oracle.go.Credentials.User", "")
				config := NewOracleDriverConfig()
				config.Credentials.User = "supplied_user"
				if err := config.AssignFromFlags(); err != nil {
					t.Fatalf("assign explicit empty override: %v", err)
				}
				if inner.NFlag() != 1 || config.Credentials.User != "" {
					t.Fatal("control did not reproduce the explicitly visited empty override")
				}
			})
			config := NewOracleDriverConfig()
			config.Credentials.User = "supplied_user"
			if err := config.AssignFromFlags(); err != nil {
				t.Fatalf("assign flags after isolated cleanup: %v", err)
			}
			if flag.CommandLine != outer || outer.NFlag() != 0 || config.Credentials.User != "supplied_user" {
				t.Fatal("empty override or visited state leaked into the next configuration")
			}
		})
		assertOriginal(t)
	})
	t.Run("nested_scope_preserves_existing_explicit_flags", func(t *testing.T) {
		withIsolatedOracleFlags(t, func(outer *flag.FlagSet) {
			set(t, outer, "oracle.go.Credentials.User", "outer_user")
			outerBefore := snapshot(outer)
			withIsolatedOracleFlags(t, func(inner *flag.FlagSet) {
				set(t, inner, "oracle.go.Credentials.User", "inner_user")
			})
			if flag.CommandLine != outer || !reflect.DeepEqual(snapshot(outer), outerBefore) {
				t.Fatal("nested scope replaced the outer explicit override")
			}
			config := NewOracleDriverConfig()
			if err := config.AssignFromFlags(); err != nil || config.Credentials.User != "outer_user" {
				t.Fatalf("outer flag no longer applies: user=%q error=%v", config.Credentials.User, err)
			}
		})
		assertOriginal(t)
	})
	t.Run("panic_restores_original", func(t *testing.T) {
		marker := new(int)
		func() {
			defer func() {
				if got := recover(); got != marker {
					t.Fatalf("unexpected panic value: %v", got)
				}
			}()
			withIsolatedOracleFlags(t, func(fs *flag.FlagSet) {
				set(t, fs, "oracle.go.Credentials.User", "panic_user")
				panic(marker)
			})
		}()
		assertOriginal(t)
	})
	t.Run("repeated_scopes_do_not_accumulate_flags", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			withIsolatedOracleFlags(t, func(fs *flag.FlagSet) {
				if fs.NFlag() != 0 {
					t.Fatal("new scope inherited visited flags")
				}
				set(t, fs, "oracle.go.Credentials.User", "repeat_user")
			})
			assertOriginal(t)
		}
	})
	t.Log("Original flag set, values and visited names preserved; explicit and empty overrides remain scoped.")
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

	withIsolatedOracleFlags(t, func(fs *flag.FlagSet) {
		for name, value := range map[string]string{
			"oracle.go.Credentials.User":       TestingConfig.Credentials.Username,
			"oracle.go.Credentials.logon_mode": TestingConfig.Credentials.LogonMode,
		} {
			if err := fs.Set(name, value); err != nil {
				t.Fatalf("set connection flag %q: %v", name, err)
			}
		}
		connector, err := NewOracleConnector(c)
		if err != nil {
			t.Fatalf("failed to create connector: %v", err)
		}
		db := sql.OpenDB(connector)
		defer func() {
			if err := db.Close(); err != nil {
				t.Errorf("close flag-override connection: %v", err)
			}
		}()
		if err := db.Ping(); err != nil {
			t.Fatalf("failed to connect using flag overrides: %v", err)
		}
	})

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

	if _, err := driver.OpenConnector("localhost:1521/freepdb1"); err != nil {
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

	connector, err := NewDriverWithConfig(config).OpenConnector("")
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
	connector, err := NewDriver().OpenConnector("user/pass@localhost:1521/freepdb1")
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

	connector, err := GetDefaultDriver().OpenConnector("tcps://127.0.0.1:2484/freepdb1?oracle.go.ConnectionProperties.WalletLocation=/tmp/foo")
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

	connector, err := GetDefaultDriver().OpenConnector("tcps://127.0.0.1:2484/freepdb1?wallet_location=" + walletLocation + "&logon_mode=" + logonMode)
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

// TestDriver_OpenConnectorPreservesQuotedInParameters verifies that a
// quoted configuration value with space retains its spaces
// expectations : quoting remain correct
func TestDriver_OpenConnectorPreservesQuotedInParameters(t *testing.T) {
	t.Parallel()
	const expected = "CN=My Server, OU=Org"
	const dsn = `tcps://127.0.0.1:1521/service?ssl_server_cert_dn="CN=My Server, OU=Org"`

	connector, err := NewDriver().OpenConnector(dsn)
	if err != nil {
		t.Fatalf("openConnector failed: %v", err)
	}

	oracleConnector := connector.(*oracleConnector)
	iterator := oracleConnector.config.NewConnectionAttemptIterator(context.Background())
	if !iterator.HasNext() {
		t.Fatal("expected a connection attempt")
	}

	option := iterator.Next()
	if option.Description == nil {
		t.Fatal("expected parsed description")
	}
	if got := option.Description.Security.SSLServerCertDN; got != expected {
		t.Fatalf("expected SSL server certificate DN %q, got %q", expected, got)
	}
}

// TestDriver_OpenConnectorReturnsInvalidDSNParameterError verifies that invalid
// DSN query parameters fail connector creation.
func TestDriver_OpenConnectorReturnsInvalidDSNParameterError(t *testing.T) {
	t.Parallel()
	_, err := NewDriver().OpenConnector("user/pass@localhost:1521/freepdb1?oracle.go.ConnectionProperties.ConnectTimeout=-1")
	if err == nil {
		t.Fatal("expected invalid DSN parameter error")
	}
}
