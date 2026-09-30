/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package transport

import (
	"context"
	"errors"
	"net"
	"testing"
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

// TestNTTCPTransportSendReceiveRoundTrip verifies successful Send and Receive
// operations over a connected TCP adapter and the documented RemoteAddr
// behavior before and after a stream is assigned.
func TestNTTCPTransportSendReceiveRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	nt := NewNTTCP(NTattributes{}, 1521)
	if nt.RemoteAddr() != nil {
		t.Fatal("RemoteAddr should be nil before a stream is assigned")
	}
	nt.stream = client
	if nt.RemoteAddr() == nil {
		t.Fatal("RemoteAddr should expose the assigned stream address")
	}

	serverRead := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		n, err := server.Read(buf)
		if err == nil && string(buf[:n]) != "ping" {
			serverRead <- errors.New("server received unexpected payload")
			return
		}
		serverRead <- err
	}()
	if err := nt.Send(context.Background(), []byte("ping")); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if err := <-serverRead; err != nil {
		t.Fatalf("server read failed: %v", err)
	}

	serverWrite := make(chan error, 1)
	go func() {
		_, err := server.Write([]byte("pong"))
		serverWrite <- err
	}()
	buf := make([]byte, 4)
	n, err := nt.Receive(context.Background(), buf, len(buf))
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if n != len(buf) || string(buf) != "pong" {
		t.Fatalf("Receive returned (%d, %q), want (4, pong)", n, string(buf))
	}
	if err := <-serverWrite; err != nil {
		t.Fatalf("server write failed: %v", err)
	}
}
