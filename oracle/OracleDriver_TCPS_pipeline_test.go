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
	"path/filepath"
	"strings"
	"testing"
)

const pipelineTCPSConfigName = "pipeline_test_tcps"

// TestDriver_TCPS_Pipeline_SelectDual validates that the driver can establish a
// TCPS connection using a dedicated CI configuration and execute a trivial query.
//
// Expected configuration:
//   - config_name = pipeline_tcps
//   - protocol = tcps
//   - wallet_location points to a directory containing the PEM wallet file
//   - ssl_server_cert_dn contains the expected server certificate DN
func TestDriver_TCPS_Pipeline_SelectDual(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig(pipelineTCPSConfigName)
	if err != nil || config == nil {
		t.Skipf("TCPS pipeline configuration %q not available: %v", pipelineTCPSConfigName, err)
	}
	/*if !config.Enabled {
		t.Skipf("TCPS pipeline configuration %q is disabled", pipelineTCPSConfigName)
	}*/

	if !strings.EqualFold(config.Database.Protocol, "tcps") {
		t.Fatalf("configuration %q must use protocol tcps, got %q", pipelineTCPSConfigName, config.Database.Protocol)
	}
	if strings.TrimSpace(config.Security.WalletLocation) == "" {
		t.Fatalf("configuration %q must define security.wallet_location", pipelineTCPSConfigName)
	}
	if strings.TrimSpace(config.Security.SslServerCertDn) == "" {
		t.Fatalf("configuration %q must define security.ssl_server_cert_dn", pipelineTCPSConfigName)
	}

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open TCPS test DB: %v", err)
	}
	defer db.Close()

	var got int
	if err := db.QueryRowContext(context.Background(), "SELECT 1 FROM DUAL").Scan(&got); err != nil {
		t.Fatalf("TCPS SELECT 1 FROM DUAL failed: %v", err)
	}
	if got != 1 {
		t.Fatalf("unexpected value from TCPS DUAL query: got %d, want 1", got)
	}
}

// TestDriver_TCPS_Pipeline_InvalidCertDn verifies that the dedicated pipeline
// TCPS configuration rejects connections when ssl_server_cert_dn does not match
// the server certificate DN.
func TestDriver_TCPS_Pipeline_InvalidCertDn(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig(pipelineTCPSConfigName)
	if err != nil || config == nil {
		t.Skipf("TCPS pipeline configuration %q not available: %v", pipelineTCPSConfigName, err)
	}

	if !strings.EqualFold(config.Database.Protocol, "tcps") {
		t.Skipf("configuration %q is not TCPS: %q", pipelineTCPSConfigName, config.Database.Protocol)
	}
	if strings.TrimSpace(config.Security.SslServerCertDn) == "" {
		t.Skipf("configuration %q does not define security.ssl_server_cert_dn", pipelineTCPSConfigName)
	}

	invalidConfig := config.Clone()
	invalidConfig.Security.SslServerCertDn = "CN=wrong"

	if _, err := openTestDBWithConfig(invalidConfig); err == nil {
		t.Fatalf("expected TCPS connection failure with invalid ssl_server_cert_dn, but succeeded")
	} else if !strings.Contains(err.Error(), "DN match failed: DN mismatch") {
		t.Fatalf("unexpected error for invalid ssl_server_cert_dn: %v", err)
	}
}

// TestDriver_TCPS_Pipeline_InvalidWalletLocation verifies that the dedicated
// pipeline TCPS configuration rejects connections when wallet_location points
// to a non-existent path.
func TestDriver_TCPS_Pipeline_InvalidWalletLocation(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig(pipelineTCPSConfigName)
	if err != nil || config == nil {
		t.Skipf("TCPS pipeline configuration %q not available: %v", pipelineTCPSConfigName, err)
	}

	if !strings.EqualFold(config.Database.Protocol, "tcps") {
		t.Skipf("configuration %q is not TCPS: %q", pipelineTCPSConfigName, config.Database.Protocol)
	}
	if strings.TrimSpace(config.Security.WalletLocation) == "" {
		t.Skipf("configuration %q does not define security.wallet_location", pipelineTCPSConfigName)
	}

	invalidConfig := config.Clone()
	invalidConfig.Security.WalletLocation = filepath.Join(config.Security.WalletLocation, "does-not-exist")

	if _, err := openTestDBWithConfig(invalidConfig); err == nil {
		t.Fatalf("expected TCPS connection failure with invalid wallet_location, but succeeded")
	}
}

// TestDriver_TCPS_Pipeline_DNMatchOff_AllowsInvalidDN verifies that the
// dedicated pipeline TCPS configuration can still connect when DN matching is
// explicitly disabled, even if ssl_server_cert_dn is incorrect.
func TestDriver_TCPS_Pipeline_DNMatchOff_AllowsInvalidDN(t *testing.T) {
	t.Parallel()
	config, err := TestEnvironement.GetConfig(pipelineTCPSConfigName)
	if err != nil || config == nil {
		t.Skipf("TCPS pipeline configuration %q not available: %v", pipelineTCPSConfigName, err)
	}

	if !strings.EqualFold(config.Database.Protocol, "tcps") {
		t.Skipf("configuration %q is not TCPS: %q", pipelineTCPSConfigName, config.Database.Protocol)
	}
	if strings.TrimSpace(config.Security.WalletLocation) == "" {
		t.Skipf("configuration %q does not define security.wallet_location", pipelineTCPSConfigName)
	}

	invalidConfig := config.Clone()
	invalidConfig.Security.SslServerDnMatch = "off"
	invalidConfig.Security.SslServerCertDn = "CN=wrong"

	db, err := openTestDBWithConfig(invalidConfig)
	if err != nil {
		t.Fatalf("expected TCPS connection success with ssl_server_dn_match=off, but got: %v", err)
	}
	defer db.Close()
}
