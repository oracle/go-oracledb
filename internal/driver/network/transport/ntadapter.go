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
	"fmt"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

type NTAdapter interface {
	Connect(ctx context.Context, address Address) error
	Send(ctx context.Context, buf []byte) error
	Receive(ctx context.Context, buf []byte, bytes2Read int) (int, error)
	Disconnect() error
}

// Address contains the resolved transport destination and the hostname
// identity needed for TLS and proxy CONNECT handling.
type Address struct {
	// DialHost is the resolved IP or original hostname used to open the socket.
	DialHost string
	// Port is the listener port.
	Port uint16
	// Protocol is the transport protocol used for this attempt.
	Protocol driverCommon.Protocol
	// TLSHostname is the selected DSN hostname used for TLS verification and as
	// the target hostname in an HTTP CONNECT request.
	TLSHostname string
	// PreviousHostname is the preceding endpoint hostname used as a certificate
	// verification fallback after a listener redirect.
	PreviousHostname string
	// HTTPSProxy is the optional HTTP CONNECT proxy hostname.
	HTTPSProxy string
	// HTTPSProxyPort is the HTTP CONNECT proxy port.
	HTTPSProxyPort int
	// ConnectionID is the resolved identifier used to correlate transport logs
	// and errors with the Oracle Net connection attempt.
	ConnectionID string
}

// String formats the dial destination as host:port for network errors and logs.
func (address Address) String() string {
	return fmt.Sprintf("%s:%d", address.DialHost, address.Port)
}

// NTattributes contains adapter-specific settings projected from the resolved
// transport options, plus runtime TLS material such as loaded wallet contents.
// ConnectionID is carried on Address because it identifies the attempt rather
// than configuring the transport.
type NTattributes struct {
	TCPNODelay              bool
	SendTimeout             int
	RecvTimeout             int
	SendBufSize             int
	RecvBufSize             int
	ExpireTime              int
	SSLServerDNMatch        bool
	SSLAllowWeakDNMatch     bool
	EnabledDCD              bool
	UseSNI                  bool
	Transportconnecttimeout int
	WalletContent           []byte
	WalletPassword          string
	SSLServerCertDN         string
	Servicename             string
	WalletLocation          string
	UseSystemTrust          bool
}
