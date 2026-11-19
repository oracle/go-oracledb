/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights in the Software, and under any and all patent rights owned or freely
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
** Software and the Larger Work(s), and to sublicense the foregoing Software and
** the Larger Work(s) on either these or other terms.
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
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	common "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

func TestGenUUID(t *testing.T) {
	t.Parallel()
	uuid, err := common.GenUUID()
	if err != nil {
		t.Fatalf("GenUUID returned error: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(uuid); err != nil {
		t.Fatalf("GenUUID did not return base64 string: %v", err)
	}
}

func TestNewSessionAttsDefaults(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts()
	if satt.sessionSDU() != NSPDFSDULN || satt.tdu != NSPDFTDULN {
		t.Fatalf("unexpected defaults: SDU %d TDU %d", satt.sessionSDU(), satt.tdu)
	}
	if satt.networkCompressionThreshold != 1024 {
		t.Fatalf("unexpected networkCompressionThreshold: %d", satt.networkCompressionThreshold)
	}
	if satt.naFlags != NSINANOSERVICES {
		t.Fatalf("unexpected naFlags: %d", satt.naFlags)
	}
	attributes, err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if !attributes.TCPNODelay {
		t.Fatal("expected TCP_NODELAY default to be enabled")
	}
}

func TestSessionAttsKeepsTransportOptionsAndProjectsAdapterSettings(t *testing.T) {
	t.Parallel()
	options := oracleconfig.TransportOptions{
		SDU:                     4096,
		ConnectTimeout:          3 * time.Millisecond,
		TransportConnectTimeout: 10 * time.Millisecond,
		ReceiveTimeout:          4 * time.Millisecond,
		SendTimeout:             5 * time.Millisecond,
		ExpireTime:              2 * time.Minute,
		ReceiveBufferSize:       8192,
		SendBufferSize:          16384,
		UseSNI:                  true,
		Compression:             true,
		CompressionLevels:       []string{"low", "high"},
		Enable:                  "broken",
		HTTPSProxy:              "proxy.example",
		HTTPSProxyPort:          8080,
		Security: oracleconfig.Security{
			SSLAllowWeakDNMatch: true,
			SSLServerDNMatch:    true,
			SSLServerCertDN:     "CN=example",
			WalletLocation:      "/tmp/wallet",
		},
	}
	satt := newSessionAtts()
	satt.setFrom(options)

	if satt.sessionSDU() != 4096 {
		t.Fatalf("expected session SDU 4096, got %d", satt.sessionSDU())
	}
	if satt.transportOptions.CompressionLevels[0] != "low" {
		t.Fatalf("session options were not retained: %v", satt.transportOptions.CompressionLevels)
	}
	attributes, err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if attributes.ExpireTime != 120000 || attributes.Transportconnecttimeout != 10 {
		t.Fatalf("unexpected timeout projection: expire=%d transport-connect=%d", attributes.ExpireTime, attributes.Transportconnecttimeout)
	}
	if attributes.RecvTimeout != 4 || attributes.SendTimeout != 5 {
		t.Fatalf("unexpected I/O timeouts: receive=%d send=%d", attributes.RecvTimeout, attributes.SendTimeout)
	}
	if attributes.RecvBufSize != 8192 || attributes.SendBufSize != 16384 {
		t.Fatalf("unexpected socket buffers: receive=%d send=%d", attributes.RecvBufSize, attributes.SendBufSize)
	}
	if !attributes.EnabledDCD {
		t.Fatalf("expected DCD to be enabled: %+v", attributes)
	}
	if satt.transportOptions.HTTPSProxy != "proxy.example" || satt.transportOptions.HTTPSProxyPort != 8080 {
		t.Fatalf("proxy settings were not retained in transport options: %+v", satt.transportOptions)
	}
	if attributes.WalletLocation != "/tmp/wallet" || attributes.SSLServerCertDN != "CN=example" || !attributes.SSLAllowWeakDNMatch || !attributes.SSLServerDNMatch || !attributes.UseSNI {
		t.Fatalf("unexpected TLS projection: %+v", attributes)
	}
}

func TestPrepareDefaultCompressionLevel(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts()
	satt.setFrom(oracleconfig.TransportOptions{Compression: true})
	packet := &connectPacket{}
	packet.marshal(nil, satt, 0)
	want := uint16((NSPACCFON << 8) | (NETWORK_COMPRESSION_ZLIB << 10))
	if got := binary.BigEndian.Uint16(packet.buf[NSPCNCFL:]); got != want {
		t.Fatalf("unexpected compression flags: got %x, want %x", got, want)
	}
}

func TestPrepareCompressionThresholdDefault(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts()
	satt.setFrom(oracleconfig.TransportOptions{Compression: true, CompressionLevels: []string{"low"}})
	if satt.networkCompressionThreshold != 1024 {
		t.Fatalf("expected default threshold 1024, got %d", satt.networkCompressionThreshold)
	}
}

func TestReadWalletFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, PEM_WALLET_FILE_NAME)
	if err := os.WriteFile(path, []byte("wallet-data"), 0600); err != nil {
		t.Fatalf("failed to write wallet file: %v", err)
	}
	data, err := readWalletFile(dir)
	if err != nil {
		t.Fatalf("readWalletFile error: %v", err)
	}
	if string(data) != "wallet-data" {
		t.Fatalf("unexpected wallet content: %s", data)
	}
}

func TestReadWalletFileError(t *testing.T) {
	t.Parallel()
	_, err := readWalletFile("/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent wallet")
	}
	if !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareWithWallet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, PEM_WALLET_FILE_NAME), []byte("wallet"), 0600); err != nil {
		t.Fatalf("write wallet error: %v", err)
	}
	satt := newSessionAtts()
	satt.setFrom(oracleconfig.TransportOptions{Security: oracleconfig.Security{WalletLocation: dir}})
	attributes, err := satt.prepare(common.ProtocolTCPS)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if string(attributes.WalletContent) != "wallet" || attributes.UseSystemTrust {
		t.Fatalf("unexpected wallet attributes: %+v", attributes)
	}
	if attributes.Transportconnecttimeout != DEFAULT_TRANSPORT_CONNECT_TIMEOUT {
		t.Fatalf("expected default transport timeout, got %d", attributes.Transportconnecttimeout)
	}
}

func TestPrepareWithSystemTrust(t *testing.T) {
	t.Parallel()
	for _, walletLocation := range []string{"", systemWalletLocation} {
		satt := newSessionAtts()
		satt.setFrom(oracleconfig.TransportOptions{Security: oracleconfig.Security{WalletLocation: walletLocation}})
		attributes, err := satt.prepare(common.ProtocolTCPS)
		if err != nil {
			t.Fatalf("prepare should use system trust for wallet location %q: %v", walletLocation, err)
		}
		if !attributes.UseSystemTrust || attributes.WalletContent != nil {
			t.Fatalf("expected system trust without wallet content for location %q: %+v", walletLocation, attributes)
		}
	}
}

func TestPrepareDefaultTimeout(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts()
	attributes, err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if attributes.Transportconnecttimeout != DEFAULT_TRANSPORT_CONNECT_TIMEOUT {
		t.Fatalf("expected default transport timeout, got %d", attributes.Transportconnecttimeout)
	}
	if attributes.UseSystemTrust {
		t.Fatal("expected system TLS trust to remain disabled for TCP")
	}
}

func TestPrepareClampsSDU(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		sdu  int
		want int
	}{
		{name: "TooSmall", sdu: NSPDADAT, want: NSPMNSDULN},
		{name: "TooLarge", sdu: NSPABSSDULN + 1, want: NSPABSSDULN},
	} {
		t.Run(tt.name, func(t *testing.T) {
			satt := newSessionAtts()
			satt.setFrom(oracleconfig.TransportOptions{SDU: tt.sdu})
			if _, err := satt.prepare(common.ProtocolTCP); err != nil {
				t.Fatalf("prepare failed: %v", err)
			}
			if satt.sessionSDU() != tt.want {
				t.Fatalf("expected SDU %d, got %d", tt.want, satt.sessionSDU())
			}
		})
	}
}

func TestPrepareWalletError(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts()
	satt.setFrom(oracleconfig.TransportOptions{Security: oracleconfig.Security{WalletLocation: "/nonexistent"}})
	if _, err := satt.prepare(common.ProtocolTCPS); err == nil {
		t.Fatal("expected prepare error for bad wallet path")
	}
}

func TestPrepareNonTCPSDoesNotLoadWallet(t *testing.T) {
	t.Parallel()
	satt := newSessionAtts()
	satt.setFrom(oracleconfig.TransportOptions{Security: oracleconfig.Security{WalletLocation: "/some/path"}})
	attributes, err := satt.prepare(common.ProtocolTCP)
	if err != nil {
		t.Fatalf("prepare failed for TCP: %v", err)
	}
	if attributes.WalletContent != nil {
		t.Fatal("wallet should not be loaded for non-TCPS")
	}
}
