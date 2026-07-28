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
	"io"
	"testing"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/network/naming"
	"github.com/oracle/go-driver/driver/network/session"
	"github.com/oracle/go-driver/driver/network/transport"
)

type mockConnectorNTAdapter struct {
	disconnected bool
	sentData     [][]byte
}

func (m *mockConnectorNTAdapter) Connect(ctx context.Context, address transport.Address) error {
	return nil
}

func (m *mockConnectorNTAdapter) Send(ctx context.Context, buf []byte) error {
	m.sentData = append(m.sentData, append([]byte(nil), buf...))
	return nil
}

func (m *mockConnectorNTAdapter) Receive(ctx context.Context, buf []byte, bytes2Read int) (int, error) {
	return 0, io.EOF
}

func (m *mockConnectorNTAdapter) Disconnect() error {
	m.disconnected = true
	return nil
}

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

func newConnectorTestSession(t *testing.T, adapter transport.NTAdapter) *session.NetworkSession {
	t.Helper()

	ns := session.NewNetworkSession()
	ns.Connected = true
	ns.SAtts = session.NewSessionAtts("test-connection-id")
	ns.NTAdapter = adapter
	if err := ns.SndDatapkt.Marshal(make([]byte, ns.SAtts.SDU), ns.SAtts, 0); err != nil {
		t.Fatalf("failed to initialize network session packet: %v", err)
	}
	return ns
}

// TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails verifies
// that Connector.Connect closes an already-open network session if TTC
// connection instantiator creation fails.
func TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails(t *testing.T) {
	t.Parallel()
	adapter := &mockConnectorNTAdapter{}
	ns := newConnectorTestSession(t, adapter)
	connector, err := newOracleConnector(
		newConnectorTestConfig(t),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (*session.NetworkSession, error) {
			return ns, nil
		},
		func(drvConfig *common.OracleDriverConfig, connectedNS *session.NetworkSession) (common.ConnectionInstantiator, error) {
			if connectedNS != ns {
				t.Fatalf("got network session %p, want %p", connectedNS, ns)
			}
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
	if !adapter.disconnected {
		t.Fatal("expected network session to disconnect after instantiator error")
	}
	if len(adapter.sentData) == 0 {
		t.Fatal("expected graceful disconnect packet before transport close")
	}
}

// TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails verifies
// that Connector.Connect closes an already-open network session if TTC
// negotiation or authentication fails while creating the driver connection.
func TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails(t *testing.T) {
	t.Parallel()
	adapter := &mockConnectorNTAdapter{}
	ns := newConnectorTestSession(t, adapter)
	connector, err := newOracleConnector(
		newConnectorTestConfig(t),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (*session.NetworkSession, error) {
			return ns, nil
		},
		func(drvConfig *common.OracleDriverConfig, connectedNS *session.NetworkSession) (common.ConnectionInstantiator, error) {
			if connectedNS != ns {
				t.Fatalf("got network session %p, want %p", connectedNS, ns)
			}
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
	if !adapter.disconnected {
		t.Fatal("expected network session to disconnect after GetConnection error")
	}
	if len(adapter.sentData) == 0 {
		t.Fatal("expected graceful disconnect packet before transport close")
	}
}

// TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess verifies that
// Connector.Connect transfers network session ownership to the returned
// driver connection after a successful setup.
func TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess(t *testing.T) {
	t.Parallel()
	adapter := &mockConnectorNTAdapter{}
	ns := newConnectorTestSession(t, adapter)
	connector, err := newOracleConnector(
		newConnectorTestConfig(t),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (*session.NetworkSession, error) {
			return ns, nil
		},
		func(drvConfig *common.OracleDriverConfig, connectedNS *session.NetworkSession) (common.ConnectionInstantiator, error) {
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
	if adapter.disconnected {
		t.Fatal("did not expect connector to disconnect session after connection ownership transfer")
	}
}

// TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess verifies
// that a failed earlier connection attempt does not cause Connector.Connect to
// return a stale error after a later attempt succeeds.
func TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess(t *testing.T) {
	t.Parallel()
	adapter := &mockConnectorNTAdapter{}
	ns := newConnectorTestSession(t, adapter)
	attempts := 0
	connector, err := newOracleConnector(
		newConnectorTestConfigWithConnectString(t, "(DESCRIPTION=(FAILOVER=ON)(LOAD_BALANCE=OFF)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.2)(PORT=1522)))(CONNECT_DATA=(SERVICE_NAME=test)))"),
		NewOracleDriverConfig(),
		func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (*session.NetworkSession, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("first address failed")
			}
			return ns, nil
		},
		func(drvConfig *common.OracleDriverConfig, connectedNS *session.NetworkSession) (common.ConnectionInstantiator, error) {
			if connectedNS != ns {
				t.Fatalf("got network session %p, want %p", connectedNS, ns)
			}
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
	if adapter.disconnected {
		t.Fatal("did not expect connector to disconnect session after later successful attempt")
	}
}
