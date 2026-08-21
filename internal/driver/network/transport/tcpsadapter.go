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

package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"unicode/utf8"
)

// nttcps represents a TCPS network transport adapter
type nttcps struct {
	nttcp
	config          *tls.Config
	tcpStream       net.Conn
	doDNMatch       bool
	walletProcessed bool
	clientCert      *tls.Certificate
	rootCAs         *x509.CertPool
}

func NewNTTCPS(atts NTattributes) *nttcps {
	return &nttcps{
		nttcp: nttcp{atts: atts},
	}
}

// Connect establishes a TLS connection
func (nt *nttcps) Connect(ctx context.Context, address Address) error {
	nt.originHost = address.OriginHost
	if err := nt.nttcp.Connect(ctx, address); err != nil { // Establish a TCP connection
		return err
	}
	nt.tcpStream = nt.stream
	cleanupOnError := func() {
		nt.Clear()
		_ = nt.Disconnect()
	}

	if err := nt.processWallet(); err != nil {
		cleanupOnError()
		return err
	}

	var certs []tls.Certificate
	if nt.clientCert != nil {
		certs = []tls.Certificate{*nt.clientCert}
	}

	nt.config = &tls.Config{
		Certificates:       certs,
		RootCAs:            nt.rootCAs,
		InsecureSkipVerify: true,
	}

	// Verify server certificate and do DN match
	nt.config.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		certs := make([]*x509.Certificate, len(rawCerts))
		for i, data := range rawCerts {
			cert, err := x509.ParseCertificate(data)
			if err != nil {
				return err
			}
			certs[i] = cert
		}

		rootCAs := nt.rootCAs
		if rootCAs == nil {
			if !nt.atts.UseSystemTrust {
				return fmt.Errorf("no trusted CA certificates configured")
			}

			var err error
			rootCAs, err = x509.SystemCertPool()
			if err != nil {
				return err
			}
		}

		opts := x509.VerifyOptions{
			Roots:         rootCAs,
			Intermediates: x509.NewCertPool(),
			// DNSName omitted → skips hostname check
		}

		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}

		if _, err := certs[0].Verify(opts); err != nil {
			return fmt.Errorf("unauthorized server certificate: %w", err)
		}

		if nt.atts.SSLServerDNMatch && nt.doDNMatch {
			if err := nt.verifyServerDN(certs[0]); err != nil {
				return fmt.Errorf("DN match failed: %w", err)
			}
		}

		return nil
	}

	if nt.atts.SSLAllowWeakDNMatch {
		nt.doDNMatch = false
	} else {
		nt.doDNMatch = true
	}

	nt.stream = tls.Client(nt.tcpStream, nt.config)

	return nil
}

func (nt *nttcps) VerifyPostAcceptDNMatch() error {
	if !nt.atts.SSLServerDNMatch || !nt.atts.SSLAllowWeakDNMatch || nt.doDNMatch {
		return nil
	}
	tlsConn, ok := nt.stream.(*tls.Conn)
	if !ok {
		return fmt.Errorf("TCPS stream is not TLS")
	}
	//ensure TLS handshake is complete.
	if err := tlsConn.Handshake(); err != nil {
		return err
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("missing server certificate")
	}

	cert := state.PeerCertificates[0]
	if err := nt.verifyServerDN(cert); err != nil {
		return fmt.Errorf("post-accept DN match failed: %w", err)
	}
	return nil
}

func (nt *nttcps) verifyServerDN(cert *x509.Certificate) error {
	if nt.atts.SSLServerCertDN != "" {
		return verifyDN(cert, nt.atts.SSLServerCertDN)
	}

	err := cert.VerifyHostname(nt.hostname)
	if err != nil && nt.originHost != "" {
		err = cert.VerifyHostname(nt.originHost)
	}
	if err != nil && nt.atts.SSLAllowWeakDNMatch && nt.atts.Servicename == cert.Subject.CommonName {
		return nil
	}
	return err
}

// Perform TLS Handshake to establish a TLS connection
func (nt *nttcps) TLSReneg() {
	nt.stream = tls.Client(nt.tcpStream, nt.config)
}

// Clear out sensitive data
func (nt *nttcps) Clear() {
	if nt.config != nil && len(nt.config.Certificates) > 0 {
		nt.config.Certificates[0].PrivateKey = nil
		nt.config.Certificates = nil
	}
	nt.config = nil
	if nt.clientCert != nil {
		nt.clientCert.PrivateKey = nil
		nt.clientCert.Certificate = nil
		nt.clientCert = nil
	}
	nt.rootCAs = nil
}

func (nt *nttcps) Disconnect() error {
	err := nt.nttcp.Disconnect()
	nt.tcpStream = nil
	return err
}

// Process the PEM wallet
func (nt *nttcps) processWallet() error {
	if nt.walletProcessed {
		return nil
	}

	var keyPEM, rootCAPEM []byte
	var clientCert tls.Certificate
	var rootCAs *x509.CertPool
	var err error
	walletContent := nt.atts.WalletContent
	password := nt.atts.WalletPassword

	rest := walletContent
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		var decryptedBytes []byte

		if x509.IsEncryptedPEMBlock(block) { //nolint
			decryptedBytes, err = x509.DecryptPEMBlock(block, []byte(password)) //nolint
			if err != nil {
				return fmt.Errorf("decrypt PEM block: %w", err)
			}
		} else {
			decryptedBytes = block.Bytes
		}

		switch block.Type {
		case "CERTIFICATE":
			// Add decrypted cert back into PEM format
			rootCAPEM = append(rootCAPEM, pem.EncodeToMemory(&pem.Block{
				Type:  block.Type,
				Bytes: decryptedBytes,
			})...)
		case "RSA PRIVATE KEY", "EC PRIVATE KEY", "PRIVATE KEY":
			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  block.Type,
				Bytes: decryptedBytes,
			})
		case "ENCRYPTED PRIVATE KEY":
			if password == "" {
				return fmt.Errorf("missing wallet password for encrypted private key; set oracle.go.wallet_password")
			}
			// Pass the *pem.Block directly
			privateKey, err := parsePKCS8EncryptedPrivateKey(block, []byte(password))
			if err != nil {
				return fmt.Errorf("decrypt encrypted private key: %w", err)
			}

			privateKeyBytes, marshalErr := x509.MarshalPKCS8PrivateKey(privateKey)
			if marshalErr != nil {
				return fmt.Errorf("marshal encrypted private key: %w", marshalErr)
			}

			keyPEM = pem.EncodeToMemory(&pem.Block{
				Type:  "PRIVATE KEY",
				Bytes: privateKeyBytes,
			})
		default:
			return fmt.Errorf("unknown PEM block type: %s", block.Type)
		}
	}

	walletProvided := walletContent != nil
	if walletProvided && rootCAPEM == nil {
		return fmt.Errorf("wallet contains no CA certificates; add the server CA certificate to the wallet or use WALLET_LOCATION=SYSTEM")
	}

	// Create client certificate
	if keyPEM != nil {
		clientCert, err = tls.X509KeyPair(rootCAPEM, keyPEM)
		if err != nil {
			return fmt.Errorf("load key pair: %w", err)
		}
		nt.clientCert = &clientCert
	}

	// Load root CAs
	if rootCAPEM != nil {
		rootCAs = x509.NewCertPool()
		if !rootCAs.AppendCertsFromPEM(rootCAPEM) {
			return fmt.Errorf("failed to parse root CA certs")
		}
	}

	for i := range walletContent {
		walletContent[i] = 0
	}
	rootCAPEM = nil
	nt.rootCAs = rootCAs
	nt.walletProcessed = true
	nt.atts.WalletContent = nil
	return nil
}

// dnNameToOID maps supported configured-DN attribute names to their OIDs.
// Keys are uppercase; aliases map to the same OID and unknown names fail closed.
var dnNameToOID = map[string]asn1.ObjectIdentifier{
	"CN":              {2, 5, 4, 3},
	"O":               {2, 5, 4, 10},
	"OU":              {2, 5, 4, 11},
	"L":               {2, 5, 4, 7},
	"ST":              {2, 5, 4, 8},
	"S":               {2, 5, 4, 8},
	"C":               {2, 5, 4, 6},
	"STREET":          {2, 5, 4, 9},
	"SERIALNUMBER":    {2, 5, 4, 5},
	"POSTALCODE":      {2, 5, 4, 17},
	"DC":              {0, 9, 2342, 19200300, 100, 1, 25},
	"UID":             {0, 9, 2342, 19200300, 100, 1, 1},
	"T":               {2, 5, 4, 12},
	"TITLE":           {2, 5, 4, 12},
	"SN":              {2, 5, 4, 4},
	"G":               {2, 5, 4, 42},
	"GN":              {2, 5, 4, 42},
	"E":               {1, 2, 840, 113549, 1, 9, 1},
	"EMAIL":           {1, 2, 840, 113549, 1, 9, 1},
	"MAIL":            {1, 2, 840, 113549, 1, 9, 1},
	"TELEPHONENUMBER": {2, 5, 4, 20},
	"DNQUALIFIER":     {2, 5, 4, 46},
	"DNQ":             {2, 5, 4, 46},
}

var supportedDNOIDs = buildSupportedDNOIDs()

func buildSupportedDNOIDs() map[string]struct{} {
	result := make(map[string]struct{}, len(dnNameToOID))
	for _, oid := range dnNameToOID {
		result[oid.String()] = struct{}{}
	}
	return result
}

// certificateRDNSequence decodes the raw Subject DN rather than using the
// flattened cert.Subject.Names, preserving RDN grouping and DER order.
func certificateRDNSequence(cert *x509.Certificate) (pkix.RDNSequence, error) {
	var rdns pkix.RDNSequence
	rest, err := asn1.Unmarshal(cert.RawSubject, &rdns)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("trailing data in certificate subject")
	}
	return rdns, nil
}

// parseConfiguredDN parses an RFC 4514-style DN string into DER RDN order.
// Commas create RDN groups, while plus signs create multiple attributes in one group.
func parseConfiguredDN(str string) (pkix.RDNSequence, error) {
	str = strings.TrimSpace(str)
	if str == "" {
		return nil, fmt.Errorf("empty DN")
	}

	rdnParts, err := splitUnescaped(str, ',')
	if err != nil {
		return nil, err
	}

	rdns := make(pkix.RDNSequence, 0, len(rdnParts))
	for _, rdnPart := range rdnParts {
		rdnPart = strings.TrimSpace(rdnPart)
		if rdnPart == "" {
			return nil, fmt.Errorf("empty RDN")
		}

		attributeParts, err := splitUnescaped(rdnPart, '+')
		if err != nil {
			return nil, err
		}

		rdn := make(pkix.RelativeDistinguishedNameSET, 0, len(attributeParts))
		for _, attributePart := range attributeParts {
			atv, err := parseDNAttribute(attributePart)
			if err != nil {
				return nil, err
			}
			rdn = append(rdn, atv)
		}
		rdns = append(rdns, rdn)
	}

	// RFC 4514 strings are leaf-first. DER RDNSequence is root-first.
	for i, j := 0, len(rdns)-1; i < j; i, j = i+1, j-1 {
		rdns[i], rdns[j] = rdns[j], rdns[i]
	}
	return rdns, nil
}

// parseDNAttribute converts one configured attribute, such as "CN=db.example.com",
// into its OID and decoded string value.
func parseDNAttribute(str string) (pkix.AttributeTypeAndValue, error) {
	str = strings.TrimSpace(str)
	if str == "" {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("empty DN attribute")
	}

	equalIndex, err := indexUnescaped(str, '=')
	if err != nil {
		return pkix.AttributeTypeAndValue{}, err
	}
	if equalIndex < 0 {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("malformed DN attribute %q", str)
	}

	name := strings.ToUpper(strings.TrimSpace(str[:equalIndex]))
	if name == "" {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("empty DN attribute name")
	}

	oid, supported := dnNameToOID[name]
	if !supported {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("unsupported DN attribute %q", name)
	}

	rawValue := trimDNValueSpace(str[equalIndex+1:])
	if rawValue == "" {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("empty DN attribute value for %q", name)
	}
	if rawValue[0] == '#' {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("hex-encoded DN values are not supported")
	}

	value, err := decodeDNValue(rawValue)
	if err != nil {
		return pkix.AttributeTypeAndValue{}, err
	}
	if value == "" {
		return pkix.AttributeTypeAndValue{}, fmt.Errorf("empty DN attribute value for %q", name)
	}

	return pkix.AttributeTypeAndValue{Type: oid, Value: value}, nil
}

// splitUnescaped splits only on separators not escaped with a backslash, so
// values such as "CN=db\,primary" keep their literal comma.
func splitUnescaped(str string, separator byte) ([]string, error) {
	var result []string
	start := 0
	for i := 0; i < len(str); i++ {
		if str[i] == '\\' {
			if i+1 >= len(str) {
				return nil, fmt.Errorf("malformed DN escape: trailing backslash")
			}
			i++
			continue
		}
		if str[i] == separator {
			result = append(result, str[start:i])
			start = i + 1
		}
	}
	return append(result, str[start:]), nil
}

func indexUnescaped(str string, target byte) (int, error) {
	for i := 0; i < len(str); i++ {
		if str[i] == '\\' {
			if i+1 >= len(str) {
				return -1, fmt.Errorf("malformed DN escape: trailing backslash")
			}
			i++
			continue
		}
		if str[i] == target {
			return i, nil
		}
	}
	return -1, nil
}

func trimDNValueSpace(str string) string {
	start := 0
	for start < len(str) && isDNValueSpace(str[start]) {
		start++
	}

	end := len(str)
	for end > start && isDNValueSpace(str[end-1]) && !isEscaped(str, end-1) {
		end--
	}
	return str[start:end]
}

func isDNValueSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

func isEscaped(str string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && str[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

// decodeDNValue converts RFC 4514 escapes, including "\," and hex byte
// escapes such as "\2C", to their literal UTF-8 value.
func decodeDNValue(str string) (string, error) {
	out := make([]byte, 0, len(str))
	for i := 0; i < len(str); i++ {
		if str[i] != '\\' {
			out = append(out, str[i])
			continue
		}
		if i+1 >= len(str) {
			return "", fmt.Errorf("malformed DN escape: trailing backslash")
		}

		if i+2 < len(str) && isHexDigit(str[i+1]) && isHexDigit(str[i+2]) {
			out = append(out, hexValue(str[i+1])<<4|hexValue(str[i+2]))
			i += 2
			continue
		}

		next := str[i+1]
		if !isEscapableDNChar(next) {
			return "", fmt.Errorf("malformed DN escape %q", str[i:i+2])
		}
		out = append(out, next)
		i++
	}

	if !utf8.Valid(out) {
		return "", fmt.Errorf("DN attribute value is not valid UTF-8")
	}
	return string(out), nil
}

func isHexDigit(b byte) bool {
	return ('0' <= b && b <= '9') || ('a' <= b && b <= 'f') || ('A' <= b && b <= 'F')
}

func hexValue(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10
	default:
		return b - 'A' + 10
	}
}

func isEscapableDNChar(b byte) bool {
	switch b {
	case ' ', '"', '#', '+', ',', ';', '<', '=', '>', '\\':
		return true
	default:
		return false
	}
}

// verifyDN checks that the server certificate subject matches the configured
// DN string exactly: same ordered RDNs, same OIDs, same string values.
func verifyDN(cert *x509.Certificate, dnString string) error {
	configured, err := parseConfiguredDN(dnString)
	if err != nil {
		return fmt.Errorf("invalid configured DN: %w", err)
	}

	certSubject, err := certificateRDNSequence(cert)
	if err != nil {
		return fmt.Errorf("parse server certificate subject: %w", err)
	}

	if len(configured) != len(certSubject) {
		return fmt.Errorf("DN mismatch: configured DN has %d RDNs but server certificate subject has %d",
			len(configured), len(certSubject))
	}

	for i := range configured {
		if err := compareRDNs(configured[i], certSubject[i]); err != nil {
			return fmt.Errorf("DN mismatch at RDN %d: %w", i, err)
		}
	}
	return nil
}

// compareRDNs compares attributes within one RDN as a set: their order does
// not matter, but each supported OID and value must match exactly.
func compareRDNs(configured, certSubject pkix.RelativeDistinguishedNameSET) error {
	if len(configured) != len(certSubject) {
		return fmt.Errorf("configured RDN has %d attributes but server certificate RDN has %d",
			len(configured), len(certSubject))
	}

	if err := validateRDN(configured, "configured DN"); err != nil {
		return err
	}
	if err := validateRDN(certSubject, "server certificate subject"); err != nil {
		return err
	}

	for _, configuredAttr := range configured {
		configuredValue, ok := configuredAttr.Value.(string)
		if !ok {
			return fmt.Errorf("configured DN attribute OID %s has non-string value", configuredAttr.Type)
		}

		found := false
		for _, certAttr := range certSubject {
			if !configuredAttr.Type.Equal(certAttr.Type) {
				continue
			}
			found = true
			certValue, ok := certAttr.Value.(string)
			if !ok {
				return fmt.Errorf("server certificate subject attribute OID %s has non-string value", certAttr.Type)
			}
			if certValue != configuredValue {
				return fmt.Errorf("attribute OID %s: expected %q but got %q",
					configuredAttr.Type, configuredValue, certValue)
			}
			break
		}
		if !found {
			return fmt.Errorf("configured attribute OID %s not found in server certificate subject", configuredAttr.Type)
		}
	}
	return nil
}

// validateRDN ensures an RDN contains only supported string attributes and
// has no duplicate OIDs, keeping later set comparison unambiguous.
func validateRDN(rdn pkix.RelativeDistinguishedNameSET, source string) error {
	for i, atv := range rdn {
		oid := atv.Type.String()
		if _, supported := supportedDNOIDs[oid]; !supported {
			return fmt.Errorf("%s contains unsupported attribute OID %s", source, atv.Type)
		}

		if _, ok := atv.Value.(string); !ok {
			return fmt.Errorf("%s attribute OID %s has non-string value", source, atv.Type)
		}
		for j := 0; j < i; j++ {
			if rdn[j].Type.Equal(atv.Type) {
				return fmt.Errorf("%s contains duplicate attribute OID %s within RDN", source, atv.Type)
			}
		}
	}
	return nil
}
