/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type fakeSocketBufferSetter struct {
	recvBufSize int
	sendBufSize int
	recvErr     error
	sendErr     error
}

func (s *fakeSocketBufferSetter) SetReadBuffer(bytes int) error {
	s.recvBufSize = bytes
	return s.recvErr
}

func (s *fakeSocketBufferSetter) SetWriteBuffer(bytes int) error {
	s.sendBufSize = bytes
	return s.sendErr
}

func TestConfigureSocketBuffers(t *testing.T) {
	t.Parallel()

	setter := &fakeSocketBufferSetter{}
	if err := configureSocketBuffers(setter, 8192, 16384); err != nil {
		t.Fatalf("configureSocketBuffers failed: %v", err)
	}
	if setter.recvBufSize != 8192 || setter.sendBufSize != 16384 {
		t.Fatalf("unexpected buffer sizes: recv=%d send=%d", setter.recvBufSize, setter.sendBufSize)
	}
}

func TestConfigureSocketBuffersErrors(t *testing.T) {
	t.Parallel()

	recvErr := errors.New("receive buffer failed")
	setter := &fakeSocketBufferSetter{recvErr: recvErr}
	if err := configureSocketBuffers(setter, 8192, 16384); !errors.Is(err, recvErr) {
		t.Fatalf("expected receive buffer error, got %v", err)
	}
	if setter.sendBufSize != 0 {
		t.Fatalf("send buffer must not be configured after receive failure, got %d", setter.sendBufSize)
	}

	sendErr := errors.New("send buffer failed")
	setter = &fakeSocketBufferSetter{sendErr: sendErr}
	if err := configureSocketBuffers(setter, 0, 16384); !errors.Is(err, sendErr) {
		t.Fatalf("expected send buffer error, got %v", err)
	}
}

// TestNormalizeDialError_PreservesDNSTimeout verifies that a timed-out DNS
// lookup is not converted into a generic connection timeout.
func TestNormalizeDialError_PreservesDNSTimeout(t *testing.T) {
	dnsErr := &net.DNSError{Err: "i/o timeout", Name: "db.example.com", IsTimeout: true}
	err := &net.OpError{Op: "dial", Net: "tcp", Err: dnsErr}

	got := normalizeDialError(context.Background(), err, Address{}, "test-id")
	var gotDNSErr *net.DNSError
	if !errors.As(got, &gotDNSErr) {
		t.Fatalf("normalized error = %T, want an error wrapping *net.DNSError", got)
	}
}

// TestNTTCPConnectThroughHTTPSProxy verifies the successful proxy flow: the
// driver connects to the proxy, sends CONNECT for the database target, and
// keeps the connection open after a 200 response.
func TestNTTCPConnectThroughHTTPSProxy(t *testing.T) {
	address, proxyResult := startTestProxy(t, func(proxyConn net.Conn) error {
		request, readErr := http.ReadRequest(bufio.NewReader(proxyConn))
		if readErr != nil {
			return readErr
		}
		if request.Method != http.MethodConnect {
			return fmt.Errorf("expected CONNECT method, got %s", request.Method)
		}
		if request.Host != "dbhost:1522" {
			return fmt.Errorf("expected CONNECT target dbhost:1522, got %s", request.Host)
		}
		_, writeErr := fmt.Fprint(proxyConn,
			"HTTP/1.1 200 Connection Established\r\nContent-Length: 0\r\n\r\n")
		return writeErr
	})

	nt := NewNTTCP(NTattributes{}, 1522)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := nt.nTConnect(ctx, address); err != nil {
		t.Fatalf("nTConnect failed: %v", err)
	}
	defer nt.Disconnect()

	if err := <-proxyResult; err != nil {
		t.Fatalf("proxy validation failed: %v", err)
	}
	if !nt.connected || nt.stream == nil {
		t.Fatal("expected proxy tunnel connection to remain open")
	}
}

// TestNTTCPConnectThroughHTTPSProxyRejectsNonSuccess verifies that a proxy
// response outside the 2xx range is reported as a proxy-connect error.
func TestNTTCPConnectThroughHTTPSProxyRejectsNonSuccess(t *testing.T) {
	address, proxyResult := startTestProxy(t, func(conn net.Conn) error {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return err
		}
		_, err := fmt.Fprint(conn, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
		return err
	})

	nt := NewNTTCP(NTattributes{}, 1522)
	err := nt.nTConnect(context.Background(), address)
	if err == nil {
		t.Fatal("expected proxy rejection error")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok || sqlErr.ErrorCode() != string(oracleErrors.HTTPSProxyConnectFailed) {
		t.Fatalf("expected HTTPS proxy error, got %T (%v)", err, err)
	}
	if err := <-proxyResult; err != nil {
		t.Fatalf("proxy validation failed: %v", err)
	}
}

// TestNTTCPConnectThroughHTTPSProxyMalformedResponse verifies that an invalid
// HTTP response from the proxy is reported as a proxy-connect error.
func TestNTTCPConnectThroughHTTPSProxyMalformedResponse(t *testing.T) {
	address, proxyResult := startTestProxy(t, func(conn net.Conn) error {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return err
		}
		_, err := fmt.Fprint(conn, "not an HTTP response\r\n\r\n")
		return err
	})

	nt := NewNTTCP(NTattributes{}, 1522)
	err := nt.nTConnect(context.Background(), address)
	if err == nil {
		t.Fatal("expected malformed proxy response error")
	}
	if sqlErr, ok := err.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.HTTPSProxyConnectFailed) {
		t.Fatalf("expected HTTPS proxy error, got %T (%v)", err, err)
	}
	if err := <-proxyResult; err != nil {
		t.Fatalf("proxy validation failed: %v", err)
	}
}

// TestNTTCPConnectThroughHTTPSProxyConnectionClosed verifies that closing the
// proxy connection after receiving CONNECT produces a proxy-connect error.
func TestNTTCPConnectThroughHTTPSProxyConnectionClosed(t *testing.T) {
	address, proxyResult := startTestProxy(t, func(conn net.Conn) error {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return err
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetLinger(0)
		}
		return conn.Close()
	})

	nt := NewNTTCP(NTattributes{}, 1522)
	err := nt.nTConnect(context.Background(), address)
	if err == nil {
		t.Fatal("expected proxy request failure")
	}
	if sqlErr, ok := err.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.HTTPSProxyConnectFailed) {
		t.Fatalf("expected HTTPS proxy error, got %T (%v)", err, err)
	}
	if err := <-proxyResult; err != nil {
		t.Fatalf("proxy validation failed: %v", err)
	}
}

// TestNTTCPConnectThroughHTTPSProxyTimeout verifies that a proxy that accepts
// the TCP connection but never completes CONNECT is stopped by the transport
// connect timeout.
func TestNTTCPConnectThroughHTTPSProxyTimeout(t *testing.T) {
	address, proxyResult := startTestProxy(t, func(conn net.Conn) error {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return err
		}
		buf := make([]byte, 1)
		_, err := conn.Read(buf)
		return err
	})

	nt := NewNTTCP(NTattributes{Transportconnecttimeout: 50}, 1522)
	done := make(chan error, 1)
	go func() {
		done <- nt.nTConnect(context.Background(), address)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected proxy handshake timeout")
		}
		var timeoutCause common.CtxTimeoutCauseError
		if !errors.As(err, &timeoutCause) {
			t.Fatalf("expected transport timeout cause, got %T (%v)", err, err)
		}
		if timeoutCause.GetSource() != "TransportConnectTimeout" || timeoutCause.GetValue() != 50 {
			t.Fatalf("unexpected timeout cause: source=%s value=%d", timeoutCause.GetSource(), timeoutCause.GetValue())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("proxy handshake did not honor TransportConnectTimeout")
	}

	if err := <-proxyResult; err == nil {
		t.Fatal("expected proxy connection to close after timeout")
	}
}

// TestHTTPSProxyPortOrDefault verifies that an omitted proxy port defaults to
// the standard HTTP proxy port while an explicit port is preserved.
func TestHTTPSProxyPortOrDefault(t *testing.T) {
	if got := httpsProxyPortOrDefault(0); got != DEFAULT_HTTPS_PROXY_PORT {
		t.Fatalf("expected default proxy port %d, got %d", DEFAULT_HTTPS_PROXY_PORT, got)
	}
	if got := httpsProxyPortOrDefault(8080); got != 8080 {
		t.Fatalf("expected configured proxy port 8080, got %d", got)
	}
}

// TestNTTCPConnectThroughHTTPSProxyDialFailure verifies that failure to open
// the TCP connection to the proxy is returned to the caller.
func TestNTTCPConnectThroughHTTPSProxyDialFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve proxy address: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	nt := NewNTTCP(NTattributes{}, 1522)
	err = nt.nTConnect(context.Background(), Address{
		Address: naming.Address{
			Protocol: driverCommon.ProtocolTCPS,
			Host:     "dbhost",
			Port:     1522,
		},
		HTTPSProxy:     "127.0.0.1",
		HTTPSProxyPort: port,
	})
	if err == nil {
		t.Fatal("expected proxy dial error")
	}
}

func startTestProxy(t *testing.T, handler func(net.Conn) error) (Address, <-chan error) {
	// Use a local listener so proxy tests do not depend on Oracle network access
	// or on the availability of a corporate proxy.
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			result <- acceptErr
			return
		}
		defer conn.Close()
		result <- handler(conn)
	}()
	t.Cleanup(func() { _ = listener.Close() })

	return Address{
		Address: naming.Address{
			Protocol: driverCommon.ProtocolTCPS,
			Host:     "dbhost",
			Port:     1522,
		},
		HTTPSProxy:     "127.0.0.1",
		HTTPSProxyPort: listener.Addr().(*net.TCPAddr).Port,
	}, result
}
