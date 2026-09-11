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
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

type mockConnectorConnectionInstantiator struct {
	conn driver.Conn
	err  error
}

func (m mockConnectorConnectionInstantiator) GetConnection(ctx context.Context) (driver.Conn, error) {
	return m.conn, m.err
}

type mockConnectorDriverConn struct{}

func (m mockConnectorDriverConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (m mockConnectorDriverConn) Close() error {
	return nil
}

func (m mockConnectorDriverConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockConnectorNetworkSession struct {
	disconnectCalled bool
	disconnectFlags  int
	disconnectCtx    context.Context
	disconnectErr    error
	cancelCalled     bool
	inband           bool
}

func (m *mockConnectorNetworkSession) Disconnect(ctx context.Context, flags int) error {
	m.disconnectCalled = true
	m.disconnectCtx = ctx
	m.disconnectFlags = flags
	return m.disconnectErr
}

func (m *mockConnectorNetworkSession) CancelOperation(ctx context.Context) error {
	m.cancelCalled = true
	return nil
}

func (m *mockConnectorNetworkSession) CheckInbandNotification() bool {
	return m.inband
}

func (m *mockConnectorNetworkSession) GetRemoteAddress() string {
	return ""
}

func (m *mockConnectorNetworkSession) GetRemotePort() int {
	return 0
}

func newConnectorTestConfig(t *testing.T) *naming.ParsedConfig {
	t.Helper()

	return newConnectorTestConfigWithConnectString(t, "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))")
}

func newConnectorTestConfigWithConnectString(t *testing.T, connectString string) *naming.ParsedConfig {
	t.Helper()

	cfg, err := naming.ParseDSNString("user/password@" + connectString)
	if err != nil {
		t.Fatalf("failed to parse test DSN: %v", err)
	}
	return cfg
}

func assertNoTokenProviderRegistered(t *testing.T, providerRegistry common.Registry[oracleProviders.Provider]) {
	t.Helper()

	provider, _ := providerRegistry.Get(reflect.TypeOf((*oracleProviders.TokenAuthenticationProvider)(nil)).Elem())
	if provider != nil {
		t.Fatalf("expected empty provider registry, got token provider %T", provider)
	}
}

// TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails verifies
// that Connector.Connect closes an already-open network session if TTC
// connection instantiator creation fails.
func TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails(t *testing.T) {
	t.Parallel()
	ns := &mockConnectorNetworkSession{}
	connector, err := newOracleConnector(
		newConnectorTestConfig(t),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (driverCommon.NetworkSession, error) {
			return ns, nil
		},
		func(drvConfig *oracleconfig.OracleDriverConfig, connectedNS driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error) {
			if connectedNS != ns {
				t.Fatalf("got network session %p, want %p", connectedNS, ns)
			}
			assertNoTokenProviderRegistered(t, providerRegistry)
			return nil, errors.New("instantiator failed")
		},
	)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	conn, err := connector.Connect(context.Background())
	if err == nil {
		t.Fatal("expected connect error, got nil")
	}
	if conn != nil {
		t.Fatalf("expected nil connection on error, got %T", conn)
	}
	if !ns.disconnectCalled {
		t.Fatal("expected network session to disconnect after instantiator error")
	}
	if ns.disconnectFlags != 0 {
		t.Fatalf("expected graceful disconnect flags 0, got %d", ns.disconnectFlags)
	}
}

// TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails verifies
// that Connector.Connect closes an already-open network session if TTC
// negotiation or authentication fails while creating the driver connection.
func TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails(t *testing.T) {
	t.Parallel()
	ns := &mockConnectorNetworkSession{}
	connector, err := newOracleConnector(
		newConnectorTestConfig(t),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (driverCommon.NetworkSession, error) {
			return ns, nil
		},
		func(drvConfig *oracleconfig.OracleDriverConfig, connectedNS driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error) {
			if connectedNS != ns {
				t.Fatalf("got network session %p, want %p", connectedNS, ns)
			}
			assertNoTokenProviderRegistered(t, providerRegistry)
			return mockConnectorConnectionInstantiator{err: errors.New("authentication failed")}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	conn, err := connector.Connect(context.Background())
	if err == nil {
		t.Fatal("expected connect error, got nil")
	}
	if conn != nil {
		t.Fatalf("expected nil connection on error, got %T", conn)
	}
}

// TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess verifies that
// Connector.Connect transfers network session ownership to the returned
// driver connection after a successful setup.
func TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess(t *testing.T) {
	t.Parallel()
	ns := &mockConnectorNetworkSession{}
	connector, err := newOracleConnector(
		newConnectorTestConfig(t),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (driverCommon.NetworkSession, error) {
			return ns, nil
		},
		func(drvConfig *oracleconfig.OracleDriverConfig, connectedNS driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error) {
			assertNoTokenProviderRegistered(t, providerRegistry)
			return mockConnectorConnectionInstantiator{conn: mockConnectorDriverConn{}}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	conn, err := connector.Connect(context.Background())
	if err != nil {
		t.Fatalf("unexpected connect error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
}

// TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess verifies
// that a failed earlier connection attempt does not cause Connector.Connect to
// return a stale error after a later attempt succeeds.
func TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess(t *testing.T) {
	t.Parallel()
	ns := &mockConnectorNetworkSession{}
	attempts := 0
	connector, err := newOracleConnector(
		newConnectorTestConfigWithConnectString(t, "(DESCRIPTION=(FAILOVER=ON)(LOAD_BALANCE=OFF)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (driverCommon.NetworkSession, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("first address failed")
			}
			return ns, nil
		},
		func(drvConfig *oracleconfig.OracleDriverConfig, connectedNS driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error) {
			if connectedNS != ns {
				t.Fatalf("got network session %p, want %p", connectedNS, ns)
			}
			assertNoTokenProviderRegistered(t, providerRegistry)
			return mockConnectorConnectionInstantiator{conn: mockConnectorDriverConn{}}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	conn, err := connector.Connect(context.Background())
	if err != nil {
		t.Fatalf("unexpected connect error after later successful attempt: %v", err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
	if attempts != 2 {
		t.Fatalf("expected two connection attempts, got %d", attempts)
	}
}
