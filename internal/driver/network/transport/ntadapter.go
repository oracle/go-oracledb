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

	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
)

type NTAdapter interface {
	// Connect opens a network connection to address. It returns an error if the
	// connection cannot be established, the context ends, or transport setup
	// fails.
	Connect(ctx context.Context, address Address) error
	// Send writes all bytes in buf to the network connection. It returns an
	// error if a write fails or the context is cancelled or reaches its deadline.
	Send(ctx context.Context, buf []byte) error
	// Receive reads bytes2Read bytes into buf and returns the number of bytes
	// read. It returns an error if bytes2Read exceeds len(buf), a read fails, or
	// the context is cancelled or reaches its deadline. On a read error, the
	// returned byte count may be less than bytes2Read.
	Receive(ctx context.Context, buf []byte, bytes2Read int) (int, error)
	// Disconnect closes the network connection, if one is open. It returns any
	// error from closing the connection; if no connection is open, it succeeds.
	Disconnect() error
}

type Address struct {
	naming.Address
	Hostname       string
	HTTPSProxy     string
	HTTPSProxyPort int
}

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
	HttpsProxy              string
	HttpsProxyPort          int
	UseSNI                  bool
	Transportconnecttimeout int
	Connectionidprefix      string
	Connectionid            string
	WalletContent           []byte
	WalletPassword          string
	SSLServerCertDN         string
	Servicename             string
	WalletLocation          string
	UseSystemTrust          bool
}
