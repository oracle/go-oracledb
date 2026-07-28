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
	"database/sql"
	"strings"
	"testing"
)

// TestDriver_TCPS_DN_Components_WhiteSpaces tests connection with a full DN containing all
// components (CN, OU, O, L, S, C) including whitespaces in values
// ensuring the full Distinguished Name is correctly extracted and matched against the server's certificate.
// Components:
// - CN: Common Name (e.g., hostname or entity name)
// - OU: Organizational Unit (department/division)
// - O: Organization (company name)
// - L: Locality (city)
// - S: State or Province
// - C: Country
func TestDriver_TCPS_DN_Components_WhiteSpaces(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	config, err := TestEnvironement.GetConfig("tcps_phoenix98269")
	if config == nil || err != nil {
		t.Skip("Can't find required configuration")
	}
	if !config.Enabled {
		t.Skip("Config not enabled")
	}
	dsn := config.GetConnectionString()
	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		t.Fatalf("expected successful connection with full DN components and whitespaces, but failed: %v", err)
	}
}

// TestDriver_TCPS_Handshake_EnforcesDNMatching_WhitespaceMismatchRejection verifies that removing the
// whitespace delimiters from ssl_server_cert_dn breaks DN matching and the TCPS
// handshake fails
func TestDriver_TCPS_Handshake_EnforcesDNMatching_WhitespaceMismatchRejection(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil || strings.ToLower(TestingConfig.Database.Protocol) != "tcps" {
		t.Skip("Skipping TCPS test as configuration is not TCPS-enabled")
	}
	config := TestingConfig.Clone()
	serverDN := config.Security.SslServerCertDn
	if serverDN == "" {
		t.Skip("Skipping because serverDN is empty")
	}
	joined := strings.Join(strings.Fields(serverDN), "")
	if joined == serverDN {
		t.Skip("Skipping because serverDN has no whitespaces")
	}
	// Simulate an admin copying the DN without commas/spaces; this should no longer match.
	serverDN = joined
	config.Security.SslServerCertDn = serverDN
	dsn := config.GetConnectionString()
	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err == nil {
		t.Fatalf("expected connection failure with invalid ssl_server_cert_dn, but succeeded")
	} else {
		if !strings.Contains(err.Error(), "DN match failed: DN mismatch") {
			t.Fatalf("unexpected error type: %v", err)
		}
	}
}

// SSL_SERVER_DN_MATCH set to off
func TestDriver_TCPS_SSL_SERVER_DN_MATCH_OFF(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil || strings.ToLower(TestingConfig.Database.Protocol) != "tcps" {
		t.Skip("Skipping TCPS test as configuration is not TCPS-enabled")
	}
	invalidConfig := TestingConfig.Clone()
	invalidConfig.Security.SslServerDnMatch = "off" // Assuming "on" is correct, change to invalid
	invalidConfig.Security.SslServerCertDn = "CN=invalid"
	dsn := invalidConfig.GetConnectionString()
	db, err := sql.Open(invalidConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		t.Fatalf("Expected no error but got %s", err)
	}
}

// TestDriver_TCPS_InvalidCertDn tests connection failure with invalid ssl_server_cert_dn.
func TestDriver_TCPS_InvalidCertDn(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil || strings.ToLower(TestingConfig.Database.Protocol) != "tcps" {
		t.Skip("Skipping TCPS test as configuration is not TCPS-enabled")
	}
	invalidConfig := TestingConfig.Clone()
	invalidConfig.Security.SslServerCertDn = "CN=wrong" // Change to invalid DN

	dsn := invalidConfig.GetConnectionString()
	db, err := sql.Open(invalidConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err == nil {
		t.Fatalf("expected connection failure with invalid ssl_server_cert_dn, but succeeded")
	} else {
		if !strings.Contains(err.Error(), "DN match failed: DN mismatch") {
			t.Fatalf("unexpected error type: %v", err)
		}
	}
}

// Confirms that, without explicitly setting ssl_server_dn_match, the driver turns DN checking on by default.
func TestDriver_TCPS_SSL_SERVER_DN_MATCH_DEFAULT(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil || strings.ToLower(TestingConfig.Database.Protocol) != "tcps" {
		t.Skip("Skipping TCPS test as configuration is not TCPS-enabled")
	}

	config := TestingConfig.Clone()
	config.Security.SslServerDnMatch = ""
	config.Security.SslServerCertDn = "CN=test"
	dsn := config.GetConnectionString()
	db, err := sql.Open(config.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err == nil {
		t.Fatalf("Expected error but got %s", err)
	} else {
		if !strings.Contains(err.Error(), "DN match failed: DN mismatch") {
			t.Fatalf("unexpected error type: %v", err)
		}
	}
}
