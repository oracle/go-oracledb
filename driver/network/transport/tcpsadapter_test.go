/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package transport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNTTCPSProcessWalletReusesParsedWalletAfterRawContentCleared(t *testing.T) {
	t.Parallel()
	walletContent := testWalletWithRootCert(t)
	nt := NewNTTCPS(NTattributes{WalletContent: walletContent})

	if err := nt.processWallet(); err != nil {
		t.Fatalf("first wallet processing failed: %v", err)
	}

	if !nt.walletProcessed {
		t.Fatal("expected wallet to be marked processed")
	}
	if nt.Atts.WalletContent != nil {
		t.Fatal("expected raw wallet content to be cleared after processing")
	}
	if nt.rootCAs == nil {
		t.Fatal("expected parsed root CAs to be retained for reconnect")
	}
	if len(nt.rootCAs.Subjects()) == 0 {
		t.Fatal("expected parsed root CA pool to contain wallet certificate")
	}

	rootCAs := nt.rootCAs
	if err := nt.processWallet(); err != nil {
		t.Fatalf("second wallet processing should reuse cached material: %v", err)
	}
	if nt.rootCAs != rootCAs {
		t.Fatal("expected cached root CAs to be reused after raw wallet content was cleared")
	}
}

func TestNTTCPSProcessWalletRejectsWalletWithoutCertificatesEvenWithSystemTrust(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	walletContent := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	nt := NewNTTCPS(NTattributes{
		WalletContent:  walletContent,
		UseSystemTrust: true,
	})
	err = nt.processWallet()
	if err == nil {
		t.Fatal("expected wallet without certificates to fail")
	}
	if !strings.Contains(err.Error(), "wallet contains no CA certificates") {
		t.Fatalf("unexpected error: %v", err)
	}
	if nt.walletProcessed {
		t.Fatal("wallet without certificates should not be marked processed")
	}
	if nt.rootCAs != nil {
		t.Fatal("wallet without certificates should not configure root CAs")
	}
}

func TestNTTCPSVerifyPostAcceptDNMatchUsesFinalTLSCertificate(t *testing.T) {
	t.Parallel()
	tlsConn, cleanup := testTLSClientConnWithServerName(t, "db.example.com")
	defer cleanup()

	nt := NewNTTCPS(NTattributes{
		SSLServerDNMatch:    true,
		SSLAllowWeakDNMatch: true,
	})
	nt.Stream = tlsConn
	nt.Hostname = "db.example.com"

	if err := nt.VerifyPostAcceptDNMatch(); err != nil {
		t.Fatalf("post-accept DN match failed: %v", err)
	}
}

func TestNTTCPSVerifyPostAcceptDNMatchRejectsMismatchedFinalCertificate(t *testing.T) {
	t.Parallel()
	tlsConn, cleanup := testTLSClientConnWithServerName(t, "db.example.com")
	defer cleanup()

	nt := NewNTTCPS(NTattributes{
		SSLServerDNMatch:    true,
		SSLAllowWeakDNMatch: true,
	})
	nt.Stream = tlsConn
	nt.Hostname = "other.example.com"

	err := nt.VerifyPostAcceptDNMatch()
	if err == nil {
		t.Fatal("expected post-accept DN match to fail")
	}
	if !strings.Contains(err.Error(), "post-accept DN match failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNTTCPSVerifyPostAcceptDNMatchUsesWeakServiceNameFallback(t *testing.T) {
	t.Parallel()
	tlsConn, cleanup := testTLSClientConn(t, "service-name", "db.example.com")
	defer cleanup()

	nt := NewNTTCPS(NTattributes{
		SSLServerDNMatch:    true,
		SSLAllowWeakDNMatch: true,
		Servicename:         "service-name",
	})
	nt.Stream = tlsConn
	nt.Hostname = "other.example.com"

	if err := nt.VerifyPostAcceptDNMatch(); err != nil {
		t.Fatalf("expected post-accept weak service-name fallback to pass, got %v", err)
	}
}

func TestNTTCPSVerifyPostAcceptDNMatchSkipsWhenStrictMatchAlreadyEnabled(t *testing.T) {
	t.Parallel()
	nt := NewNTTCPS(NTattributes{
		SSLServerDNMatch:    true,
		SSLAllowWeakDNMatch: true,
	})
	nt.doDNMatch = true

	if err := nt.VerifyPostAcceptDNMatch(); err != nil {
		t.Fatalf("expected post-accept DN match to skip when strict matching is already enabled, got %v", err)
	}
}

// TestVerifyDNWithMultiValuedRDN accepts reordered attributes within one
// multi-valued RDN, but rejects the same attributes when their RDN grouping differs.
func TestVerifyDNWithMultiValuedRDN(t *testing.T) {
	t.Parallel()
	// The certificate keeps OU and CN in the same RDN, with OU listed first.
	cert := testCertificateWithSubject(t, pkix.RDNSequence{
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 6}, Value: "US"}},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 10}, Value: "Oracle"}},
		{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 11}, Value: "Production"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "db, primary"},
		},
	})

	tests := []struct {
		name    string
		dn      string
		wantErr bool
	}{
		{
			name:    "matches escaped comma, space, and unordered attributes in one RDN",
			dn:      `CN=db\, primary+OU=Production,O=Oracle,C=US`,
			wantErr: false,
		},
		{
			name:    "rejects the same attributes in different RDN groups",
			dn:      `CN=db\, primary,OU=Production,O=Oracle,C=US`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyDN(cert, tt.dn)
			if (err != nil) != tt.wantErr {
				t.Fatalf("verifyDN(%q) error = %v, want error = %t", tt.dn, err, tt.wantErr)
			}
		})
	}
}

// TestVerifyDNAllowsAliases verifies that S and E use the same OIDs as ST and emailAddress.
func TestVerifyDNAllowsAliases(t *testing.T) {
	t.Parallel()
	cert := testCertificateWithSubject(t, pkix.RDNSequence{
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 6}, Value: "US"}},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 8}, Value: "California"}},
		{{Type: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}, Value: "admin@example.com"}},
	})

	if err := verifyDN(cert, `E=admin@example.com,S=California,C=US`); err != nil {
		t.Fatalf("verifyDN with S and E aliases failed: %v", err)
	}
}

// TestVerifyDNAllowsRepeatedAttributeAcrossRDNs verifies that the same OID is
// allowed in separate comma-delimited RDN groups.
func TestVerifyDNAllowsRepeatedAttributeAcrossRDNs(t *testing.T) {
	t.Parallel()
	cert := testCertificateWithSubject(t, pkix.RDNSequence{
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 6}, Value: "US"}},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "parent.example.com"}},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "db.example.com"}},
	})

	if err := verifyDN(cert, `CN=db.example.com,CN=parent.example.com,C=US`); err != nil {
		t.Fatalf("verifyDN with repeated CN across RDNs failed: %v", err)
	}
}

func TestNTTCPSDisconnectPreservesProcessedWalletForRedirectReuse(t *testing.T) {
	t.Parallel()
	walletContent := testWalletWithRootCert(t)
	nt := NewNTTCPS(NTattributes{WalletContent: walletContent})

	if err := nt.processWallet(); err != nil {
		t.Fatalf("wallet processing failed: %v", err)
	}
	rootCAs := nt.rootCAs
	nt.config = &tls.Config{RootCAs: rootCAs}

	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	nt.Stream = clientConn
	nt.tcpStream = clientConn
	nt.Connected = true

	if err := nt.Disconnect(); err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}

	if nt.Stream != nil {
		t.Fatal("expected stream to be cleared after disconnect.")
	}
	if nt.tcpStream != nil {
		t.Fatal("expected tcpStream to be cleared after disconnect")
	}
	if !nt.walletProcessed {
		t.Fatal("expected processed-wallet marker to remain for redirect reuse")
	}
	if nt.rootCAs != rootCAs {
		t.Fatal("expected parsed wallet root CAs to remain available for redirect reuse")
	}
	if nt.config == nil {
		t.Fatal("expected TLS config to remain available for redirect reuse")
	}
}

func TestNTTCPDisconnectClosesStreamWhenConnectedFlagFalse(t *testing.T) {
	t.Parallel()
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	nt := &NTTCP{Stream: clientConn}

	if err := nt.Disconnect(); err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}

	if nt.Stream != nil {
		t.Fatal("expected stream to be cleared")
	}
	if nt.Connected {
		t.Fatal("expected connected flag to be false")
	}
}

// testCertificateWithSubject builds the minimal certificate data needed by verifyDN.
func testCertificateWithSubject(t *testing.T, subject pkix.RDNSequence) *x509.Certificate {
	t.Helper()

	rawSubject, err := asn1.Marshal(subject)
	if err != nil {
		t.Fatalf("marshal certificate subject: %v", err)
	}
	return &x509.Certificate{RawSubject: rawSubject}
}

func testWalletWithRootCert(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "test-root",
			Organization: []string{"Oracle"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

func testTLSClientConnWithServerName(t *testing.T, serverName string) (*tls.Conn, func()) {
	t.Helper()
	return testTLSClientConn(t, serverName, serverName)
}

func testTLSClientConn(t *testing.T, commonName, dnsName string) (*tls.Conn, func()) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		DNSNames:              []string{dnsName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	clientRaw, serverRaw := net.Pipe()
	serverTLS := tls.Server(serverRaw, &tls.Config{Certificates: []tls.Certificate{cert}})
	clientTLS := tls.Client(clientRaw, &tls.Config{InsecureSkipVerify: true})

	errCh := make(chan error, 1)
	go func() {
		errCh <- serverTLS.Handshake()
	}()
	if err := clientTLS.Handshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server handshake: %v", err)
	}

	cleanup := func() {
		_ = clientTLS.Close()
		_ = serverTLS.Close()
	}
	return clientTLS, cleanup
}
