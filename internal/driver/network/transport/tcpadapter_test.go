/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

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

// TestNTTCPTransportSendReceiveRoundTrip connects the driver adapter to a local
// TCP listener. Send must deliver the whole request, and Receive must read the
// requested reply without changing bytes beyond the requested length.
func TestNTTCPTransportSendReceiveRoundTrip(t *testing.T) {
	t.Parallel()
	nt, server := testNTTCPWithListener(t, NTattributes{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverRead := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		n, err := io.ReadFull(server, buf)
		if err == nil && string(buf[:n]) != "ping" {
			serverRead <- errors.New("server received unexpected payload")
			return
		}
		serverRead <- err
	}()
	if err := nt.Send(ctx, []byte("ping")); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if err := <-serverRead; err != nil {
		t.Fatalf("server read failed: %v", err)
	}

	if _, err := server.Write([]byte("po")); err != nil {
		t.Fatalf("server write failed: %v", err)
	}
	if _, err := server.Write([]byte("ng")); err != nil {
		t.Fatalf("server write failed: %v", err)
	}
	buf := []byte("------")
	n, err := nt.Receive(ctx, buf, 4)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if n != 4 || string(buf) != "pong--" {
		t.Fatalf("Receive returned (%d, %q), want (4, pong--)", n, string(buf))
	}
}

// TestNTTCPTransportRejectsInvalidReceiveLength checks that a read larger than
// the buffer returns InvalidNetworkExpectedLength, zero bytes, and an unchanged buffer.
func TestNTTCPTransportRejectsInvalidReceiveLength(t *testing.T) {
	t.Parallel()
	nt, _ := testNTTCPWithListener(t, NTattributes{})
	buf := []byte("-")
	n, err := nt.Receive(context.Background(), buf, 2)
	var sqlErr oracleErrors.SQLError
	if n != 0 || !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.InvalidNetworkExpectedLength) || string(buf) != "-" {
		t.Fatalf("Receive returned (%d, %v, %q), want zero bytes, %s, and unchanged buffer", n, err, buf, oracleErrors.InvalidNetworkExpectedLength)
	}
}

// TestNTTCPTransportReceiveTimeouts checks that a read waiting on a real TCP
// peer returns the caller's deadline or the configured receive-timeout cause.
// A later read must succeed, showing that the adapter reset the socket deadline.
func TestNTTCPTransportReceiveTimeouts(t *testing.T) {
	t.Parallel()
	for _, configured := range []bool{false, true} {
		name := "caller deadline"
		if configured {
			name = "receive timeout"
		}
		t.Run(name, func(t *testing.T) {
			atts := NTattributes{Connectionid: "receive-timeout-test"}
			if configured {
				atts.RecvTimeout = 50
			}
			nt, server := testNTTCPWithListener(t, atts)
			ctx := context.Background()
			if !configured {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			n, err := nt.Receive(ctx, make([]byte, 4), 4)
			if n != 0 {
				t.Fatalf("timed-out Receive returned %d bytes, want 0", n)
			}
			if configured {
				var cause common.CtxTimeoutCauseError
				if !errors.As(err, &cause) || cause.GetSource() != "recv-timeout" || cause.GetValue() != 50 || cause.GetEmitterID() != atts.Connectionid {
					t.Fatalf("Receive error = %v, want the configured receive-timeout cause", err)
				}
			} else if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Receive error = %v, want context.DeadlineExceeded", err)
			}

			nt.atts.RecvTimeout = 0
			if _, err := server.Write([]byte("pong")); err != nil {
				t.Fatalf("server write after timeout failed: %v", err)
			}
			retryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			buf := make([]byte, 4)
			n, err = nt.Receive(retryCtx, buf, len(buf))
			if err != nil || n != len(buf) || string(buf) != "pong" {
				t.Fatalf("Receive after timeout returned (%d, %q, %v), want (4, pong, nil)", n, buf, err)
			}
		})
	}
}

// TestNTTCPTransportReturnsReadErrors checks that a peer closing after a short
// reply returns ChannelReadFailed with the EOF cause and the bytes already read.
func TestNTTCPTransportReturnsReadErrors(t *testing.T) {
	t.Parallel()
	nt, server := testNTTCPWithListener(t, NTattributes{})
	if _, err := server.Write([]byte("ab")); err != nil {
		t.Fatalf("server write failed: %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("server close failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	buf := []byte("----")
	n, err := nt.Receive(ctx, buf, len(buf))
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.ChannelReadFailed) || !errors.Is(err, io.EOF) {
		t.Fatalf("Receive error = %v, want %s", err, oracleErrors.ChannelReadFailed)
	}
	if n != 2 || string(buf) != "ab--" {
		t.Fatalf("short Receive returned (%d, %q), want (2, ab--)", n, buf)
	}
}

// TestNTTCPConnectHonorsCanceledContext checks that Connect returns a caller's
// cancellation without retaining a socket or reporting a successful connection.
func TestNTTCPConnectHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	nt := NewNTTCP(NTattributes{}, port)
	defer nt.Disconnect()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = nt.Connect(ctx, Address{Address: naming.Address{Host: "127.0.0.1", Port: port}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect error = %v, want context.Canceled", err)
	}
	if nt.connected || nt.stream != nil {
		t.Fatal("canceled Connect retained an active connection")
	}
}

// testNTTCPWithListener connects the driver adapter through its Connect method
// and accepts the peer socket. All sockets and I/O are bounded and cleaned up.
func testNTTCPWithListener(t *testing.T, atts NTattributes) (*nttcp, *net.TCPConn) {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("listener deadline failed: %v", err)
	}
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	nt := NewNTTCP(atts, port)
	t.Cleanup(func() { nt.Disconnect() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := nt.Connect(ctx, Address{Address: naming.Address{Host: "127.0.0.1", Port: port}}); err != nil {
		t.Fatalf("driver Connect failed: %v", err)
	}
	server, err := listener.AcceptTCP()
	if err != nil {
		t.Fatalf("accept failed: %v", err)
	}
	t.Cleanup(func() { server.Close() })
	if err := server.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("server deadline failed: %v", err)
	}
	return nt, server
}
