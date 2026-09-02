/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"net"
	"testing"
)

func TestNewNTTCP(t *testing.T) {
	nt := NewNTTCP(NTattributes{Connectionid: "cid"}, 1521)
	if nt.cha != TCPCHA {
		t.Fatalf("cha = %d, want %d", nt.cha, TCPCHA)
	}
	if nt.connected {
		t.Fatal("new NTTCP should not be connected")
	}
	if nt.secure {
		t.Fatal("new NTTCP should not be secure")
	}
	if nt.port != 1521 {
		t.Fatalf("port = %d, want 1521", nt.port)
	}
	if nt.atts.Connectionid != "cid" {
		t.Fatalf("Connectionid = %q, want cid", nt.atts.Connectionid)
	}
}

func TestNTTCPSendReceiveAndDisconnect(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("cleanup close peer connection failed: %v", err)
		}
	})

	nt := NewNTTCP(NTattributes{}, 1521)
	nt.stream = client
	nt.connected = true

	readDone := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		buf := make([]byte, 5)
		_, err := io.ReadFull(server, buf)
		readDone <- struct {
			data []byte
			err  error
		}{data: buf, err: err}
	}()

	if err := nt.Send(context.Background(), []byte("hello")); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	read := <-readDone
	if read.err != nil {
		t.Fatalf("peer read returned error: %v", read.err)
	}
	if got := string(read.data); got != "hello" {
		t.Fatalf("peer read %q, want hello", got)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := server.Write([]byte("world"))
		writeDone <- err
	}()

	buf := make([]byte, 5)
	n, err := nt.Receive(context.Background(), buf, len(buf))
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if n != 5 || string(buf) != "world" {
		t.Fatalf("Receive = (%d, %q), want (5, world)", n, string(buf))
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("peer write returned error: %v", err)
	}

	if err := nt.Disconnect(); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	if nt.connected {
		t.Fatal("Disconnect should clear connected")
	}
}

func TestNTTCPReceiveBufferTooSmall(t *testing.T) {
	nt := NewNTTCP(NTattributes{}, 1521)
	n, err := nt.Receive(context.Background(), make([]byte, 1), 2)
	if err == nil {
		t.Fatal("expected buffer-too-small error")
	}
	if n != 0 {
		t.Fatalf("bytes read = %d, want 0", n)
	}
}

func TestNTTCPDisconnectWhenNotConnected(t *testing.T) {
	nt := NewNTTCP(NTattributes{}, 1521)
	if err := nt.Disconnect(); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
}

func TestNewNTTCPSAndClear(t *testing.T) {
	nt := NewNTTCPS(NTattributes{WalletPassword: "secret"})
	if nt.atts.WalletPassword != "secret" {
		t.Fatalf("WalletPassword = %q, want secret", nt.atts.WalletPassword)
	}

	nt.config = &tls.Config{Certificates: []tls.Certificate{{PrivateKey: struct{}{}}}}
	nt.Clear()
	if nt.config != nil {
		t.Fatal("Clear should nil TLS config")
	}
}

func TestProcessWalletEmptyAndUnknownBlock(t *testing.T) {
	nt := NewNTTCPS(NTattributes{})
	if err := nt.processWallet(); err != nil {
		t.Fatalf("processWallet with empty wallet returned error: %v", err)
	}
	if nt.clientCert != nil {
		t.Fatal("processWallet with empty wallet should not configure a client certificate")
	}
	if nt.rootCAs != nil {
		t.Fatal("processWallet with empty wallet should not return root CAs")
	}

	content := pem.EncodeToMemory(&pem.Block{Type: "BOGUS", Bytes: []byte("x")})
	nt = NewNTTCPS(NTattributes{WalletContent: content})
	if err := nt.processWallet(); err == nil {
		t.Fatal("expected unknown PEM block error")
	}
}

func TestParseAndVerifyDN(t *testing.T) {
	got, err := parseConfiguredDN("CN=db.example.com, O=Oracle, OU=Drivers")
	if err != nil {
		t.Fatalf("parseConfiguredDN returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed unexpected values: %#v", got)
	}

	rawSubject, err := asn1.Marshal(got)
	if err != nil {
		t.Fatalf("marshal subject: %v", err)
	}
	cert := &x509.Certificate{RawSubject: rawSubject}
	if err := verifyDN(cert, "CN=db.example.com,O=Oracle,OU=Drivers"); err != nil {
		t.Fatalf("verifyDN returned error: %v", err)
	}
	if err := verifyDN(cert, "CN=other,O=Oracle,OU=Drivers"); err == nil {
		t.Fatal("expected DN mismatch")
	}
}
