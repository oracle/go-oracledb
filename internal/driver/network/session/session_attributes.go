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
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/transport"
)

// TODO: The first can be deleted, underscore is a better standard
const walletPasswordEnvVar = "oracle.go.wallet_password"
const walletPasswordEnvVarAlt = "ORACLE_GO_WALLET_PASSWORD"
const systemWalletLocation = "SYSTEM"

// sessionAtts represents network session attributes
type sessionAtts struct {
	LargeSDU                           bool
	SDU                                int
	TDU                                int
	NT                                 transport.NTattributes
	UUID                               string
	NetworkCompressionThreshold        int
	NetworkCompression                 bool
	NetworkCompressionLevels           []string
	ConnectTimeout                     int
	RecvTimeout                        int
	SendTimeout                        int
	NAFlags                            int
	cDataNVPair                        interface{} // Placeholder for nvStrToNvPair data
	NegotiatedNetworkCompressionScheme int
	NetworkCompressionEnabled          bool
	FirstRecvCompressedPacket          bool
	FirstSendCompressedPacket          bool
	Version                            int
	Options                            int
}

// GenUUID generates a UUID for connection ID
func GenUUID() (string, error) {
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// newSessionAtts creates a new sessionAtts instance
func newSessionAtts(uuid string) *sessionAtts {
	return &sessionAtts{
		LargeSDU:                    false,
		SDU:                         NSPDFSDULN,
		TDU:                         NSPDFTDULN,
		NT:                          transport.NTattributes{Connectionidprefix: "", TCPNODelay: true, SSLServerDNMatch: true},
		UUID:                        uuid,
		NetworkCompressionThreshold: 1024,
		NAFlags:                     NSINANOSERVICES,
	}
}

func (sa *sessionAtts) setFrom(source interface{}) {
	if source == nil {
		return
	}

	var sdu float64
	var networkCompression bool
	var networkCompressionLevels []string
	var networkCompressionThreshold float64
	var expireTime float64
	var connectTimeout float64
	var transportConnectTimeout float64
	var recvTimeout float64
	var sendTimeout float64
	var connectionIDPrefix string
	var tcpNoDelay bool
	var sslAllowWeakDNMatch bool
	var sslServerDNMatch bool
	var sslServerCertDN string
	var enable string
	var httpsProxy string
	var httpsProxyPort float64
	var walletLocation string
	var walletPassword string
	var useSNI bool

	switch src := source.(type) {
	case *naming.Description:
		sdu = float64(src.SDU)
		networkCompression = src.IsCompressionEnabled()
		networkCompressionLevels = src.CompressionLevels
		expireTime = float64(src.ExpireTime)
		connectTimeout = float64(src.ConnectTimeout)
		transportConnectTimeout = float64(src.TransportConnectTimeout)
		recvTimeout = float64(src.RecvTimeout)
		sendTimeout = float64(src.SendTimeout)
		connectionIDPrefix = src.ConnectionIDPrefix
		tcpNoDelay = true
		sslAllowWeakDNMatch = src.Security.IsSSLAllowWeakDNMatchEnabled()
		sslServerDNMatch = src.Security.IsSSLServerDNMatchEnabled()
		sslServerCertDN = src.Security.SSLServerCertDN
		enable = src.Enable
		walletLocation = src.Security.WalletLocation
		useSNI = src.IsUseSNIEnabled()

	default:
		return
	}

	// Assuming we don't have the password read until now by any means
	// Hence reading using environment variable
	if walletPassword == "" {
		walletPassword = os.Getenv(walletPasswordEnvVar)
	}
	if walletPassword == "" {
		walletPassword = os.Getenv(walletPasswordEnvVarAlt)
	}

	// Set from extracted values
	if sdu > 0 {
		sa.SDU = int(sdu)
	}
	if networkCompression {
		sa.NetworkCompression = true
		sa.NetworkCompressionLevels = networkCompressionLevels

		if len(sa.NetworkCompressionLevels) == 0 {
			sa.NetworkCompressionLevels = []string{"high"}
		}
	}
	if networkCompressionThreshold >= 200 {
		sa.NetworkCompressionThreshold = int(networkCompressionThreshold)
	}
	if expireTime > 0 {
		sa.NT.ExpireTime = int(expireTime * 1000 * 60)
	}
	if connectTimeout > 0 {
		sa.ConnectTimeout = int(connectTimeout)
	}
	if transportConnectTimeout > 0 {
		sa.NT.Transportconnecttimeout = int(transportConnectTimeout)
	}
	if recvTimeout > 0 {
		sa.NT.RecvTimeout = int(recvTimeout)
	}
	if sendTimeout > 0 {
		sa.NT.SendTimeout = int(sendTimeout)
	}
	if connectionIDPrefix != "" {
		sa.NT.Connectionidprefix = connectionIDPrefix
	}
	sa.NT.TCPNODelay = tcpNoDelay

	if strings.ToUpper(enable) == "BROKEN" {
		sa.NT.EnabledDCD = true
	}
	if httpsProxy != "" {
		sa.NT.HttpsProxy = httpsProxy
	}
	if httpsProxyPort >= 0 {
		sa.NT.HttpsProxyPort = int(httpsProxyPort)
	}
	sa.NT.WalletLocation = walletLocation
	sa.NT.SSLAllowWeakDNMatch = sslAllowWeakDNMatch
	sa.NT.SSLServerDNMatch = sslServerDNMatch
	sa.NT.SSLServerCertDN = sslServerCertDN
	sa.NT.WalletPassword = walletPassword
	sa.NT.UseSNI = useSNI // Assuming NT has UseSNI; add if not

}

// readWalletFile reads the wallet file
func (sa *sessionAtts) readWalletFile() ([]byte, error) {
	path := filepath.Join(sa.NT.WalletLocation, PEM_WALLET_FILE_NAME)
	return os.ReadFile(path)
}

// prepare prepares attributes for connection
func (sa *sessionAtts) prepare(protocol driverCommon.Protocol) error {
	sa.SDU = clamp(sa.SDU, NSPMNSDULN, NSPABSSDULN)

	if sa.UUID == "" {
		uuid, err := GenUUID()
		if err != nil {
			return err
		}
		sa.UUID = uuid
	}
	if sa.NT.Connectionidprefix != "" {
		sa.NT.Connectionid = sa.NT.Connectionidprefix + sa.UUID
	} else {
		sa.NT.Connectionid = sa.UUID
	}

	walletLocation := strings.TrimSpace(sa.NT.WalletLocation)
	sa.NT.UseSystemTrust = protocol == driverCommon.ProtocolTCPS &&
		(walletLocation == "" || strings.EqualFold(walletLocation, systemWalletLocation))
	if protocol == driverCommon.ProtocolTCPS && sa.NT.WalletContent == nil && !sa.NT.UseSystemTrust {
		data, err := sa.readWalletFile()
		if err != nil {
			return err
		}
		sa.NT.WalletContent = data
	}

	if sa.ConnectTimeout == 0 && sa.NT.Transportconnecttimeout == 0 {
		sa.NT.Transportconnecttimeout = DEFAULT_TRANSPORT_CONNECT_TIMEOUT
	}
	common.Odl.Debug("connection timeout set", "tm",
		sa.NT.Transportconnecttimeout)

	//sa.NT.Atts = sa // Set the reference back to sessionAtts
	return nil
}
