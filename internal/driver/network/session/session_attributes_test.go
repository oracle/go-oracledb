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

package session

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	common "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
)

func TestNewSessionAttsDefaults(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("custom")
	if satt.uuid != "custom" {
		t.Fatalf("expected custom UUID, got %s", satt.uuid)
	}
	if satt.sdu != NSPDFSDULN || satt.tdu != NSPDFTDULN {
		t.Fatalf("unexpected defaults: SDU %d TDU %d", satt.sdu, satt.tdu)
	}
	if satt.nt.TCPNODelay != true || satt.nt.SSLServerDNMatch != true {
		t.Fatalf("unexpected NT defaults: %+v", satt.nt)
	}
	if satt.networkCompressionThreshold != 1024 {
		t.Fatalf("unexpected networkCompressionThreshold: %d", satt.networkCompressionThreshold)
	}
	if satt.naFlags != NSINANOSERVICES {
		t.Fatalf("unexpected naFlags: %d", satt.naFlags)
	}
}

func TestSessionAttsSetFromDescription(t *testing.T) {
	t.Parallel()
	desc := &naming.Description{
		SDU:                     4096,
		Compression:             true,
		CompressionLevels:       []string{"low", "high"},
		TransportConnectTimeout: 10,
		ExpireTime:              2,
		ConnectTimeout:          3,
		RecvTimeout:             4,
		ConnectionIDPrefix:      "prefix",
		Enable:                  "broken",
		Security: naming.Security{
			SSLAllowWeakDNMatch: true,
			SSLServerCertDN:     "CN=example",
			WalletLocation:      "/tmp/wallet",
		},
		UseSNI: true,
	}

	satt := newSessionAtts("uuid")
	satt.setFrom(desc)

	if satt.sdu != 4096 {
		t.Fatalf("expected sdu 4096 got %d", satt.sdu)
	}
	if !satt.networkCompression || len(satt.networkCompressionLevels) != 2 || satt.networkCompressionLevels[0] != "low" {
		t.Fatalf("unexpected compression fields: %+v", satt)
	}
	if satt.nt.ExpireTime != 120000 {
		t.Fatalf("unexpected expire time: %d", satt.nt.ExpireTime)
	}
	if satt.connectTimeout != 3 {
		t.Fatalf("unexpected connect timeout: %d", satt.connectTimeout)
	}
	if satt.nt.Transportconnecttimeout != 10 {
		t.Fatalf("unexpected transport connect timeout: %d", satt.nt.Transportconnecttimeout)
	}
	if satt.nt.RecvTimeout != 4 {
		t.Fatalf("unexpected recv timeout: %d", satt.recvTimeout)
	}
	if satt.nt.Connectionidprefix != "prefix" {
		t.Fatalf("unexpected Connectionidprefix: %s", satt.nt.Connectionidprefix)
	}
	if !satt.nt.EnabledDCD {
		t.Fatalf("expected EnabledDCD to be true")
	}
	if satt.nt.WalletLocation != "/tmp/wallet" {
		t.Fatalf("wallet location not set: %s", satt.nt.WalletLocation)
	}
	if satt.nt.SSLServerCertDN != "CN=example" {
		t.Fatalf("SSLServerCertDN unexpected: %s", satt.nt.SSLServerCertDN)
	}
	if !satt.nt.SSLAllowWeakDNMatch {
		t.Fatalf("expected weak dn match true")
	}
	if !satt.nt.UseSNI {
		t.Fatalf("expected UseSNI true")
	}
}

func TestSessionAttsSetFromParsedDescriptionKeepsDefaultDNMatch(t *testing.T) {
	t.Parallel()
	root, err := naming.Parse("(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=service)))")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	ctx, err := naming.ExtractConnectionContext(root)
	if err != nil {
		t.Fatalf("ExtractConnectionContext failed: %v", err)
	}
	if ctx.Description == nil {
		t.Fatal("expected parsed description")
	}

	satt := newSessionAtts("uuid")
	satt.setFrom(ctx.Description)
	if !satt.nt.SSLServerDNMatch {
		t.Fatal("expected SSLServerDNMatch to remain true when SECURITY node is absent")
	}
}

func TestSessionAttsSetFromNil(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("")
	original := *satt
	satt.setFrom(nil)
	if !reflect.DeepEqual(*satt, original) {
		t.Fatalf("SetFrom(nil) modified fields")
	}
	satt.setFrom("unsupported")
	if !reflect.DeepEqual(*satt, original) {
		t.Fatalf("SetFrom(unsupported) modified fields")
	}
}

func TestSessionAttsSetFromCompressionDefaultLevel(t *testing.T) {
	t.Parallel()
	desc := &naming.Description{Compression: true, CompressionLevels: nil}
	satt := newSessionAtts("uuid")
	satt.setFrom(desc)
	if !satt.networkCompression || len(satt.networkCompressionLevels) != 1 || satt.networkCompressionLevels[0] != "high" {
		t.Fatalf("expected default compression level, got %+v", satt.networkCompressionLevels)
	}
}

func TestSessionAttsSetFromCompressionThreshold(t *testing.T) {
	t.Parallel()
	desc := &naming.Description{Compression: true, CompressionLevels: []string{"low"}}
	satt := newSessionAtts("uuid")
	satt.setFrom(desc)
	if satt.networkCompressionThreshold != 1024 {
		t.Fatalf("expected default threshold 1024, got %d", satt.networkCompressionThreshold)
	}
}

func TestSessionAttsSetFromEnableNotBroken(t *testing.T) {
	t.Parallel()
	desc := &naming.Description{Enable: "notbroken"}
	satt := newSessionAtts("")
	satt.setFrom(desc)
	if satt.nt.EnabledDCD {
		t.Fatalf("expected EnabledDCD false for enable not 'broken'")
	}
}

func TestSessionAttsSetFromSSLAllowWeakDNMatchNo(t *testing.T) {
	t.Parallel()
	desc := &naming.Description{Security: naming.Security{SSLAllowWeakDNMatch: false}}
	satt := newSessionAtts("")
	satt.setFrom(desc)
	if satt.nt.SSLAllowWeakDNMatch {
		t.Fatalf("expected SSLAllowWeakDNMatch false")
	}
}

func TestSessionAttsSetFromUseSNINo(t *testing.T) {
	t.Parallel()
	desc := &naming.Description{UseSNI: false}
	satt := newSessionAtts("")
	satt.setFrom(desc)
	if satt.nt.UseSNI {
		t.Fatalf("expected UseSNI false")
	}
}

func TestReadWalletFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, PEM_WALLET_FILE_NAME)
	content := []byte("wallet-data")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("failed to write wallet file: %v", err)
	}
	satt := newSessionAtts("uuid")
	satt.nt.WalletLocation = dir
	data, err := satt.readWalletFile()
	if err != nil {
		t.Fatalf("ReadWalletFile error: %v", err)
	}
	if string(data) != "wallet-data" {
		t.Fatalf("unexpected wallet content: %s", data)
	}
}

func TestReadWalletFileError(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("uuid")
	satt.nt.WalletLocation = "/nonexistent"
	_, err := satt.readWalletFile()
	if err == nil {
		t.Fatalf("expected error for nonexistent wallet")
	}
	if !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("")
	satt.nt.Connectionidprefix = "pref-"
	satt.nt.WalletLocation = ""
	satt.nt.WalletContent = []byte("existing")
	satt.nt.Transportconnecttimeout = 1000
	satt.connectTimeout = 0
	err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if satt.uuid == "" {
		t.Fatalf("Prepare should set UUID")
	}
	if !strings.HasPrefix(satt.nt.Connectionid, "pref-") {
		t.Fatalf("Connectionid not prefixed: %s", satt.nt.Connectionid)
	}
	if string(satt.nt.WalletContent) != "existing" {
		t.Fatalf("wallet content changed: %s", satt.nt.WalletContent)
	}
	if satt.nt.Transportconnecttimeout != 1000 {
		t.Fatalf("expected transport timeout unchanged, got %d", satt.nt.Transportconnecttimeout)
	}
}

func TestPrepareWithWallet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	walletPath := filepath.Join(dir, PEM_WALLET_FILE_NAME)
	if err := os.WriteFile(walletPath, []byte("wallet"), 0600); err != nil {
		t.Fatalf("write wallet error: %v", err)
	}
	satt := newSessionAtts("")
	satt.nt.WalletLocation = dir
	satt.nt.Transportconnecttimeout = 0
	satt.connectTimeout = 0
	err := satt.prepare(common.ProtocolTCPS)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if satt.uuid == "" {
		t.Fatalf("UUID not set")
	}
	if satt.nt.Connectionid != satt.uuid {
		t.Fatalf("unexpected Connectionid: %s", satt.nt.Connectionid)
	}
	if string(satt.nt.WalletContent) != "wallet" {
		t.Fatalf("wallet not loaded: %s", satt.nt.WalletContent)
	}
	if satt.nt.UseSystemTrust {
		t.Fatal("expected a configured wallet to provide its own trust anchors")
	}
	if satt.nt.Transportconnecttimeout != DEFAULT_TRANSPORT_CONNECT_TIMEOUT {
		t.Fatalf("expected default transport timeout, got %d", satt.nt.Transportconnecttimeout)
	}
}

func TestPrepareWithSystemTrust(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		dsn            string
		walletLocation string
	}{
		{
			name:           "ConnectDescriptorExplicitSystem",
			dsn:            "user/pass@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=db))(SECURITY=(WALLET_LOCATION=SYSTEM)))",
			walletLocation: systemWalletLocation,
		},
		{
			name:           "EZConnectExplicitSystem",
			dsn:            "user/pass@tcps://host:1521/db?wallet_location=SYSTEM",
			walletLocation: systemWalletLocation,
		},
		{
			name: "ConnectDescriptorWithoutWallet",
			dsn:  "user/pass@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=db)))",
		},
		{
			name: "EZConnectWithoutWallet",
			dsn:  "user/pass@tcps://host:1521/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := naming.ParseDSNString(tt.dsn)
			if err != nil {
				t.Fatalf("ParseDSNString error: %v", err)
			}
			iter := parsed.NewConnectionAttemptIterator(context.Background())
			if !iter.HasNext() {
				t.Fatal("expected connection option")
			}
			option := iter.Next()
			desc := option.Description
			if desc == nil {
				t.Fatal("expected description")
			}

			satt := newSessionAtts("")
			satt.setFrom(desc)
			if satt.nt.WalletLocation != tt.walletLocation {
				t.Fatalf("expected wallet location %q, got %q", tt.walletLocation, satt.nt.WalletLocation)
			}
			if err := satt.prepare(common.ProtocolTCPS); err != nil {
				t.Fatalf("Prepare should use system trust: %v", err)
			}
			if !satt.nt.UseSystemTrust {
				t.Fatal("expected system trust to be enabled")
			}
			if satt.nt.WalletContent != nil {
				t.Fatalf("expected no wallet content, got %q", string(satt.nt.WalletContent))
			}
		})
	}
}

func TestPrepareDefaultTimeout(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("")
	satt.nt.Transportconnecttimeout = 0
	satt.connectTimeout = 0
	err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if satt.nt.Transportconnecttimeout != DEFAULT_TRANSPORT_CONNECT_TIMEOUT {
		t.Fatalf("expected default transport timeout, got %d", satt.nt.Transportconnecttimeout)
	}
	if satt.nt.UseSystemTrust {
		t.Fatal("expected system TLS trust to remain disabled for TCP")
	}
}

func TestPrepareClampsSDU(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sdu  int
		want int
	}{
		{name: "TooSmall", sdu: NSPDADAT, want: NSPMNSDULN},
		{name: "TooLarge", sdu: NSPABSSDULN + 1, want: NSPABSSDULN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			satt := newSessionAtts("")
			satt.sdu = tt.sdu
			if err := satt.prepare(common.ProtocolTCP); err != nil {
				t.Fatalf("Prepare error: %v", err)
			}
			if satt.sdu != tt.want {
				t.Fatalf("expected SDU %d, got %d", tt.want, satt.sdu)
			}
		})
	}
}

func TestPrepareWalletError(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("")
	satt.nt.WalletLocation = "/nonexistent"
	err := satt.prepare(common.ProtocolTCPS)
	if err == nil {
		t.Fatalf("expected Prepare error for bad wallet path")
	}
}

func TestPrepareUUIDError(t *testing.T) {
	t.Parallel()
	// Hard to force rand.Read error, but for coverage note it returns err
}

func TestPrepareNoPrefix(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("")
	err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if satt.nt.Connectionid != satt.uuid {
		t.Fatalf("Connectionid should be UUID without prefix: %s", satt.nt.Connectionid)
	}
}

func TestPrepareNonTCPSNoWalletLoad(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts("")
	satt.nt.WalletLocation = "/some/path"
	err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	if satt.nt.WalletContent != nil {
		t.Fatalf("wallet should not be loaded for non-tcps")
	}
}
