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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/transport"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

// TODO: The first can be deleted, underscore is a better standard
const walletPasswordEnvVar = "oracle.go.wallet_password"
const walletPasswordEnvVarAlt = "ORACLE_GO_WALLET_PASSWORD"
const systemWalletLocation = "SYSTEM"

// sessionAtts represents network session attributes
type sessionAtts struct {
	largeSDU bool
	// negotiatedSDU is populated after ACCEPT. Before then, sessionSDU reads the
	// requested SDU directly from transportOptions.
	negotiatedSDU int
	tdu           int
	// transportOptions is the attempt's effective options and remains the source
	// for requested settings that the session protocol reads.
	transportOptions                   oracleconfig.TransportOptions
	networkCompressionThreshold        int
	naFlags                            int
	negotiatedNetworkCompressionScheme int
	networkCompressionEnabled          bool
	firstRecvCompressedPacket          bool
	firstSendCompressedPacket          bool
	version                            int
	options                            int
}

// newSessionAtts creates a new sessionAtts instance
func newSessionAtts() *sessionAtts {
	return &sessionAtts{
		largeSDU:                    false,
		tdu:                         NSPDFTDULN,
		networkCompressionThreshold: 1024,
		naFlags:                     NSINANOSERVICES,
	}
}

// setFrom stores the resolved options for this connection attempt. The
// session reads requested values from this snapshot until Oracle Net returns
// negotiated values from the server.
//
// Input:
//   - src: effective options for the selected access point. The caller owns the
//     attempt configuration; this session keeps the supplied snapshot for its
//     lifetime and does not modify its transport settings.
//
// Output: none.
func (sa *sessionAtts) setFrom(src oracleconfig.TransportOptions) {
	sa.transportOptions = src
}

// sessionSDU returns the server-negotiated SDU when available, otherwise the
// clamped SDU requested by the selected connection attempt.
//
// Input: none.
//
// Output:
//   - int: active session data unit size in bytes.
func (sa *sessionAtts) sessionSDU() int {
	if sa.negotiatedSDU > 0 {
		return sa.negotiatedSDU
	}
	sdu := sa.transportOptions.SDU
	if sdu == 0 {
		sdu = NSPDFSDULN
	}
	return clamp(sdu, NSPMNSDULN, NSPABSSDULN)
}

// readWalletFile reads the PEM wallet from the configured wallet location.
//
// Input:
//   - walletLocation: directory containing the PEM wallet file.
//
// Output:
//   - []byte: wallet contents for the TLS adapter.
//   - error: file access error, if the wallet cannot be read.
func readWalletFile(walletLocation string) ([]byte, error) {
	path := filepath.Join(walletLocation, PEM_WALLET_FILE_NAME)
	return os.ReadFile(path)
}

// prepare builds transport adapter attributes for this session from its
// effective options.
//
// Input:
//   - protocol: transport protocol used to configure TLS and wallet behavior.
//
// Output:
//   - transport.NTattributes: adapter-specific projection of the effective
//     options, including wallet data loaded for TCPS.
//   - error: wallet loading error encountered during setup.
func (sa *sessionAtts) prepare(protocol driverCommon.Protocol) (transport.NTattributes, error) {
	options := sa.transportOptions

	attributes := transport.NTattributes{
		TCPNODelay:              true,
		SendTimeout:             int(options.SendTimeout / time.Millisecond),
		RecvTimeout:             int(options.ReceiveTimeout / time.Millisecond),
		SendBufSize:             options.SendBufferSize,
		RecvBufSize:             options.ReceiveBufferSize,
		ExpireTime:              int(options.ExpireTime / time.Millisecond),
		SSLServerDNMatch:        options.Security.SSLServerDNMatch,
		SSLAllowWeakDNMatch:     options.Security.SSLAllowWeakDNMatch,
		EnabledDCD:              strings.EqualFold(options.Enable, "BROKEN"),
		UseSNI:                  options.UseSNI,
		SSLServerCertDN:         options.Security.SSLServerCertDN,
		WalletLocation:          options.Security.WalletLocation,
		Transportconnecttimeout: int(options.TransportConnectTimeout / time.Millisecond),
	}
	if attributes.Transportconnecttimeout == 0 && options.ConnectTimeout == 0 {
		attributes.Transportconnecttimeout = DEFAULT_TRANSPORT_CONNECT_TIMEOUT
	}

	// TODO : do we support this ?
	attributes.WalletPassword = os.Getenv(walletPasswordEnvVar)
	if attributes.WalletPassword == "" {
		attributes.WalletPassword = os.Getenv(walletPasswordEnvVarAlt)
	}

	walletLocation := strings.TrimSpace(attributes.WalletLocation)
	attributes.UseSystemTrust = protocol == driverCommon.ProtocolTCPS &&
		(walletLocation == "" || strings.EqualFold(walletLocation, systemWalletLocation))
	if protocol == driverCommon.ProtocolTCPS && !attributes.UseSystemTrust {
		data, err := readWalletFile(attributes.WalletLocation)
		if err != nil {
			common.Odl.Debug("Error reading wallet file", "error", err)
			return transport.NTattributes{}, err
		}
		attributes.WalletContent = data
	}

	return attributes, nil
}
