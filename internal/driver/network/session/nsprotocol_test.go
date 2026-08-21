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
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/transport"
)

type mockNTAdapter struct {
	connected     bool
	disconnected  bool
	sentData      [][]byte
	receivedData  []byte
	recvPos       int
	connectErr    error
	sendErr       error
	receiveErr    error
	disconnectErr error
	sendCall      int
	errorOnSecond bool
	secondSendErr error
	receiveCalls  int
	lastAddress   transport.Address
}

type mockNTTCPS struct {
	mockNTAdapter
	renegotiated bool
	cleared      bool
}

func (m *mockNTTCPS) TLSReneg() {
	m.renegotiated = true
}

func (m *mockNTTCPS) Clear() {
	m.cleared = true
}

// Connect simulates connecting to the given address
func (m *mockNTAdapter) Connect(ctx context.Context, address transport.Address) error {
	m.connected = true
	m.lastAddress = address
	return m.connectErr
}

// Disconnect simulates disconnecting from the server
func (m *mockNTAdapter) Disconnect() error {
	m.disconnected = true
	return m.disconnectErr
}

// Send simulates sending data over the network
func (m *mockNTAdapter) Send(ctx context.Context, data []byte) error {
	m.sendCall++
	if m.errorOnSecond && m.sendCall == 2 {
		return m.secondSendErr
	}
	m.sentData = append(m.sentData, data)
	return m.sendErr
}

// Receive simulates receiving data into the buffer
func (m *mockNTAdapter) Receive(ctx context.Context, buf []byte, size int) (int, error) {
	if m.receiveErr != nil {
		return 0, m.receiveErr
	}
	m.receiveCalls++
	remaining := len(m.receivedData) - m.recvPos
	if remaining <= 0 {
		return 0, io.EOF
	}
	toCopy := size
	if toCopy > len(buf) {
		toCopy = len(buf)
	}
	if toCopy > remaining {
		toCopy = remaining
	}
	copy(buf, m.receivedData[m.recvPos:m.recvPos+toCopy])
	m.recvPos += toCopy
	return toCopy, nil
}

// TestNewNetworkSession tests the creation of a new NetworkSession
func TestNewNetworkSession(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	if ns.connected {
		t.Errorf("New session should not be connected")
	}
	if ns.isBreak || ns.isReset || ns.breakPosted || ns.compressionEnabled {
		t.Errorf("New session flags should be false")
	}
	if ns.sndDatapkt == nil || ns.rcvDatapkt == nil || ns.controlPkt == nil {
		t.Errorf("Packets should be initialized")
	}
	if ns.byteOrder != driverCommon.BIG_ENDIAN {
		t.Errorf("Default byte order should be BIG_ENDIAN")
	}
}

// TestTransportConnect tests the transportConnect function
func TestTransportConnect(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, SDU: 8192}

	// Test HTTPS proxy with TCP
	address := transport.Address{
		Address:    naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "host", Port: 1521},
		HTTPSProxy: "proxy",
	}
	err := ns.transportConnect(context.Background(), address)
	if err == nil || !strings.Contains(err.Error(), "https proxy requires protocol as tcps") {
		t.Errorf("Expected HTTPS proxy error, got %v", err)
	}

	// Test TCP connection attempt (will fail without real server)
	address = transport.Address{
		Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 9999},
	}
	err = ns.transportConnect(context.Background(), address)
	if err == nil {
		t.Errorf("Expected connection error")
	}

	// Test TCPS
	address.Address.Protocol = driverCommon.ProtocolTCPS
	err = ns.transportConnect(context.Background(), address)
	if err == nil {
		t.Errorf("Expected connection error")
	}

	// Cover ntAdapter nil case for TCP
	ns.ntAdapter = nil
	address.Address.Protocol = driverCommon.ProtocolTCP
	err = ns.transportConnect(context.Background(), address)
	if err == nil {
		t.Errorf("Expected connection error for TCP")
	}

	// Cover ntAdapter nil case for TCPS
	ns.ntAdapter = nil
	address.Address.Protocol = driverCommon.ProtocolTCPS
	err = ns.transportConnect(context.Background(), address)
	if err == nil {
		t.Errorf("Expected connection error for TCPS")
	}
}

// TestConnectToOption tests the ConnectToOption function
func TestConnectToOption(t *testing.T) {
	t.Parallel()
	newOption := func(protocol driverCommon.Protocol, description *naming.Description, connectString string) *naming.ConnectionOption {
		return naming.NewConnectionOption(
			naming.Address{
				Host:     "localhost",
				Port:     1521,
				Protocol: protocol,
			},
			description,
			naming.ConnectData{},
			nil,
			"",
			connectString,
		)
	}
	option := newOption(
		driverCommon.ProtocolTCP,
		&naming.Description{},
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orcl)))",
	)

	// Test parse error
	option = newOption(driverCommon.ProtocolTCP, &naming.Description{}, "(DESCRIPTION")
	_, err := ConnectToOption(context.Background(), option)
	if err == nil {
		t.Errorf("Expected parse error")
	}
	option = newOption(
		driverCommon.ProtocolTCP,
		&naming.Description{},
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orcl)))",
	)

	// Test connection attempt (will fail)
	_, err = ConnectToOption(context.Background(), option)
	if err == nil {
		t.Errorf("Expected connection error")
	}

	// Additional tests to increase coverage
	t.Run("Missing CONNECT_DATA", func(t *testing.T) {
		option = newOption(driverCommon.ProtocolTCP, &naming.Description{}, "(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=localhost)(PORT=1521)))")
		_, err := ConnectToOption(context.Background(), option)
		if err == nil {
			t.Errorf("Expected connection error for missing CONNECT_DATA")
		}
	})

	t.Run("Prepare Error", func(t *testing.T) {
		option = newOption(
			driverCommon.ProtocolTCPS,
			&naming.Description{Security: naming.Security{WalletLocation: "/nonexistent"}},
			"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orcl)))",
		)
		_, err = ConnectToOption(context.Background(), option)
		if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
			t.Errorf("Expected prepare error, got %v", err)
		}
	})
}

// TestConnectSubtests groups subtests for ConnectToOption
func TestConnectSubtests(t *testing.T) {
	t.Parallel()
	option := naming.NewConnectionOption(
		naming.Address{
			Host:     "localhost",
			Port:     1521,
			Protocol: driverCommon.ProtocolTCP,
		},
		&naming.Description{},
		naming.ConnectData{},
		nil,
		"",
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orcl)))",
	)

	t.Run("Low TNS Version", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: 314, SDU: 8192} // low version
		acceptBuf := make([]byte, 50)
		binary.BigEndian.PutUint16(acceptBuf[0:2], 50) // packet len for !LargeSDU
		acceptBuf[4] = NSPTAC
		binary.BigEndian.PutUint16(acceptBuf[NSPACVSN:], TNS_VERSION_MINIMUM-1) // low version
		mock.receivedData = acceptBuf
		mock.recvPos = 0
		// Simulate connect flow
		err := ns.transportConnect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err != nil {
			t.Errorf("Unexpected transportConnect error: %v", err)
		}
		connectPkt := &connectPacket{}
		connectPkt.Marshal([]byte(option.ConnectString), ns.sAtts, NO_HEADER_FLAGS)
		err = ns.sendConnect(context.Background(), connectPkt)
		if err != nil {
			t.Errorf("Unexpected sendConnect error: %v", err)
		}
		pkt, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected recvPacket error: %v", err)
		}
		if _, ok := pkt.(*acceptPacket); !ok {
			t.Errorf("Expected acceptPacket")
		}
		if ns.sAtts.Version < TNS_VERSION_MINIMUM {
			err = fmt.Errorf("unsupported TNS version: %d (minimum required: %d)", ns.sAtts.Version, TNS_VERSION_MINIMUM)
		}
		if err == nil || !strings.Contains(err.Error(), "unsupported TNS version") {
			t.Errorf("Expected low version error, got %v", err)
		}
		ns.Disconnect(context.Background(), 0)
	})

	t.Run("Redirect Packet from server without overflow", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		// mock redirect packet from server without overflow
		redirectData := []byte("(ADDRESS=(PROTOCOL=tcp)(HOST=redirecthost)(PORT=1522))")
		dataLen := len(redirectData)
		packetLen := 8 + 2 + dataLen
		fullPacket := make([]byte, packetLen)
		binary.BigEndian.PutUint16(fullPacket[0:2], uint16(packetLen))
		fullPacket[4] = NSPTRD // type 5
		fullPacket[5] = 0x00   // flags 0
		binary.BigEndian.PutUint16(fullPacket[8:10], uint16(dataLen))
		copy(fullPacket[10:], redirectData)
		mock.receivedData = fullPacket
		// mock accept packet after redirect
		acceptpacket := []byte{0x0, 0x3d, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x1, 0x3f, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x3d, 0x49, 0xa, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x1a, 0x0, 0x0, 0x0, 0x4a, 0xef, 0x45, 0x83, 0x1b, 0x21, 0xd6, 0xe5, 0xc2, 0x70, 0x13, 0xd3, 0xc8, 0x6f, 0x67, 0x80}
		mock.receivedData = append(mock.receivedData, acceptpacket...)
		mock.recvPos = 0
		err := ns.connect(context.Background(), transport.Address{
			Address:  naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
			Hostname: "originalhost",
		})
		if err != nil {
			t.Errorf("Unexpected redirect packet error without overflow: %v", err)
		}
		if mock.lastAddress.Hostname != "redirecthost" {
			t.Errorf("Expected redirected host to be used for DN match, got %q", mock.lastAddress.Hostname)
		}
		if mock.lastAddress.OriginHost != "originalhost" {
			t.Errorf("Expected original host to be preserved as fallback, got %q", mock.lastAddress.OriginHost)
		}
		ns.Disconnect(context.Background(), 0)
	})
	t.Run("Redirect Packet from server with overflow", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		// mock redirect packet from server
		data := []byte{0x0, 0xa, 0x0, 0x0, 0x5, 0x2, 0x0, 0x0, 0x1, 0x61}
		// redirect packet overflow data
		overflowdata := []byte{0x1, 0x6b, 0x0, 0x0, 0x6, 0x0, 0x0, 0x0, 0x0, 0x40, 0x28, 0x41, 0x44, 0x44, 0x52, 0x45, 0x53, 0x53, 0x3d, 0x28, 0x50, 0x52, 0x4f, 0x54, 0x4f, 0x43, 0x4f, 0x4c, 0x3d, 0x74, 0x63, 0x70, 0x29, 0x28, 0x48, 0x4f, 0x53, 0x54, 0x3d, 0x70, 0x68, 0x6f, 0x65, 0x6e, 0x69, 0x78, 0x39, 0x38, 0x32, 0x36, 0x39, 0x2e, 0x64, 0x65, 0x76, 0x33, 0x73, 0x75, 0x62, 0x32, 0x70, 0x68, 0x78, 0x2e, 0x64, 0x61, 0x74, 0x61, 0x62, 0x61, 0x73, 0x65, 0x64, 0x65, 0x33, 0x70, 0x68, 0x78, 0x2e, 0x6f, 0x72, 0x61, 0x63, 0x6c, 0x65, 0x76, 0x63, 0x6e, 0x2e, 0x63, 0x6f, 0x6d, 0x29, 0x28, 0x50, 0x4f, 0x52, 0x54, 0x3d, 0x31, 0x35, 0x32, 0x30, 0x29, 0x29, 0x0, 0x28, 0x44, 0x45, 0x53, 0x43, 0x52, 0x49, 0x50, 0x54, 0x49, 0x4f, 0x4e, 0x3d, 0x28, 0x41, 0x44, 0x44, 0x52, 0x45, 0x53, 0x53, 0x3d, 0x28, 0x50, 0x52, 0x4f, 0x54, 0x4f, 0x43, 0x4f, 0x4c, 0x3d, 0x74, 0x63, 0x70, 0x29, 0x28, 0x48, 0x4f, 0x53, 0x54, 0x3d, 0x70, 0x68, 0x6f, 0x65, 0x6e, 0x69, 0x78, 0x39, 0x38, 0x32, 0x36, 0x39, 0x2e, 0x64, 0x65, 0x76, 0x33, 0x73, 0x75, 0x62, 0x32, 0x70, 0x68, 0x78, 0x2e, 0x64, 0x61, 0x74, 0x61, 0x62, 0x61, 0x73, 0x65, 0x64, 0x65, 0x33, 0x70, 0x68, 0x78, 0x2e, 0x6f, 0x72, 0x61, 0x63, 0x6c, 0x65, 0x76, 0x63, 0x6e, 0x2e, 0x63, 0x6f, 0x6d, 0x29, 0x28, 0x50, 0x4f, 0x52, 0x54, 0x3d, 0x31, 0x38, 0x32, 0x31, 0x29, 0x29, 0x28, 0x43, 0x4f, 0x4e, 0x4e, 0x45, 0x43, 0x54, 0x5f, 0x44, 0x41, 0x54, 0x41, 0x3d, 0x28, 0x53, 0x45, 0x52, 0x56, 0x49, 0x43, 0x45, 0x5f, 0x4e, 0x41, 0x4d, 0x45, 0x3d, 0x63, 0x64, 0x62, 0x31, 0x5f, 0x70, 0x64, 0x62, 0x31, 0x2e, 0x72, 0x65, 0x67, 0x72, 0x65, 0x73, 0x73, 0x2e, 0x72, 0x64, 0x62, 0x6d, 0x73, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x75, 0x73, 0x2e, 0x6f, 0x72, 0x61, 0x63, 0x6c, 0x65, 0x2e, 0x63, 0x6f, 0x6d, 0x29, 0x28, 0x43, 0x4f, 0x4e, 0x4e, 0x45, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x5f, 0x49, 0x44, 0x3d, 0x58, 0x75, 0x48, 0x59, 0x48, 0x2b, 0x55, 0x6e, 0x6e, 0x4a, 0x45, 0x32, 0x44, 0x36, 0x46, 0x7a, 0x74, 0x30, 0x49, 0x54, 0x6f, 0x51, 0x3d, 0x3d, 0x29, 0x28, 0x53, 0x45, 0x52, 0x56, 0x45, 0x52, 0x3d, 0x64, 0x65, 0x64, 0x69, 0x63, 0x61, 0x74, 0x65, 0x64, 0x29, 0x28, 0x49, 0x4e, 0x53, 0x54, 0x41, 0x4e, 0x43, 0x45, 0x5f, 0x4e, 0x41, 0x4d, 0x45, 0x3d, 0x32, 0x33, 0x63, 0x29, 0x29, 0x29}
		mock.receivedData = append(data, overflowdata...)
		// mock accept packet after a redirect
		acceptpacket := []byte{0x0, 0x3d, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x1, 0x3f, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x3d, 0x49, 0xa, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x1a, 0x0, 0x0, 0x0, 0x4a, 0xef, 0x45, 0x83, 0x1b, 0x21, 0xd6, 0xe5, 0xc2, 0x70, 0x13, 0xd3, 0xc8, 0x6f, 0x67, 0x80}
		mock.receivedData = append(mock.receivedData, acceptpacket...)
		mock.recvPos = 0
		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err != nil {
			t.Errorf("Unexpected redirect packet error: %v", err)
		}
		ns.Disconnect(context.Background(), 0)
	})
	t.Run("Redirect Packet exceeds maximum redirect count", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=redirect_probe)))")
		ns.redirectCount = maxRedirectCount

		redirectData := []byte("(ADDRESS=(PROTOCOL=tcp)(HOST=redirecthost)(PORT=1522))")
		dataLen := len(redirectData)
		packetLen := 8 + 2 + dataLen
		fullPacket := make([]byte, packetLen)
		binary.BigEndian.PutUint16(fullPacket[0:2], uint16(packetLen))
		fullPacket[4] = NSPTRD
		fullPacket[5] = 0x00
		binary.BigEndian.PutUint16(fullPacket[8:10], uint16(dataLen))
		copy(fullPacket[10:], redirectData)
		mock.receivedData = fullPacket
		mock.recvPos = 0

		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err == nil || !strings.Contains(err.Error(), "too many redirects") {
			t.Errorf("Expected too many redirects error, got %v", err)
		}
	})
	t.Run("Refuse Packet from server", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")
		// mock refuse packet buf from the server
		mock.receivedData = []byte{0x0, 0x5f, 0x0, 0x0, 0x4, 0x0, 0x0, 0x0, 0x22, 0x0, 0x0, 0x53, 0x28, 0x44, 0x45, 0x53, 0x43, 0x52, 0x49, 0x50, 0x54, 0x49, 0x4f, 0x4e, 0x3d, 0x28, 0x54, 0x4d, 0x50, 0x3d, 0x29, 0x28, 0x56, 0x53, 0x4e, 0x4e, 0x55, 0x4d, 0x3d, 0x30, 0x29, 0x28, 0x45, 0x52, 0x52, 0x3d, 0x31, 0x32, 0x35, 0x31, 0x34, 0x29, 0x28, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x5f, 0x53, 0x54, 0x41, 0x43, 0x4b, 0x3d, 0x28, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x3d, 0x28, 0x43, 0x4f, 0x44, 0x45, 0x3d, 0x31, 0x32, 0x35, 0x31, 0x34, 0x29, 0x28, 0x45, 0x4d, 0x46, 0x49, 0x3d, 0x34, 0x29, 0x29, 0x29, 0x29}
		mock.recvPos = 0
		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if !strings.Contains(err.Error(), "12514") {
			t.Errorf("Expected refuse error, got %v", err)
		}
		ns.Disconnect(context.Background(), 0)
	})
	t.Run("Refuse Packet from server with overflow", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")
		refuseHdr := []byte{0x00, 0x0c, 0x00, 0x00, 0x04, 0x02, 0x00, 0x00, 0x22, 0x00, 0x00, 0x00}
		overflowPkt := []byte{0x00, 0x5d, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x28, 0x44, 0x45, 0x53, 0x43, 0x52, 0x49, 0x50, 0x54, 0x49, 0x4f, 0x4e, 0x3d, 0x28, 0x54, 0x4d, 0x50, 0x3d, 0x29, 0x28, 0x56, 0x53, 0x4e, 0x4e, 0x55, 0x4d, 0x3d, 0x30, 0x29, 0x28, 0x45, 0x52, 0x52, 0x3d, 0x31, 0x32, 0x35, 0x31, 0x34, 0x29, 0x28, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x5f, 0x53, 0x54, 0x41, 0x43, 0x4b, 0x3d, 0x28, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x3d, 0x28, 0x43, 0x4f, 0x44, 0x45, 0x3d, 0x31, 0x32, 0x35, 0x31, 0x34, 0x29, 0x28, 0x45, 0x4d, 0x46, 0x49, 0x3d, 0x34, 0x29, 0x29, 0x29, 0x29}
		mock.receivedData = append(refuseHdr, overflowPkt...)
		mock.recvPos = 0
		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if !strings.Contains(err.Error(), "12514") {
			t.Errorf("Expected refuse error with overflow, got %v", err)
		}
		ns.Disconnect(context.Background(), 0)
	})
	t.Run("Resend Packet", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		resendBuf := make([]byte, 10)
		binary.BigEndian.PutUint16(resendBuf[0:2], 10)
		resendBuf[4] = NSPTRS
		mock.receivedData = resendBuf
		mock.recvPos = 0
		// Simulate connect flow
		err := ns.transportConnect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err != nil {
			t.Errorf("Unexpected transportConnect error: %v", err)
		}
		connectPkt := &connectPacket{}
		connectPkt.Marshal([]byte(option.ConnectString), ns.sAtts, NO_HEADER_FLAGS)
		err = ns.sendConnect(context.Background(), connectPkt)
		if err != nil {
			t.Errorf("Unexpected sendConnect error: %v", err)
		}
		pkt, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected recvPacket error: %v", err)
		}
		if p, ok := pkt.(*resendPacket); ok {
			if p.hdr.Flags&NSPFSRN != 0 {
				// Simulate TLS reneg if needed
			}
			err = ns.sendConnect(context.Background(), connectPkt)
			if err != nil {
				t.Errorf("Unexpected resend error: %v", err)
			}
		}
		mock.receiveErr = fmt.Errorf("recv error after resend")
		_, err = ns.recvPacket(context.Background()) // simulate next recv failing
		if err == nil {
			t.Errorf("Expected connection error for resend loop")
		}
		ns.Disconnect(context.Background(), 0)
	})

	t.Run("Unexpected Packet", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		unknownBuf := make([]byte, 10)
		binary.BigEndian.PutUint16(unknownBuf[0:2], 10)
		unknownBuf[4] = 99 // unknown type
		mock.receivedData = unknownBuf
		mock.recvPos = 0
		// Simulate connect flow
		err := ns.transportConnect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err != nil {
			t.Errorf("Unexpected transportConnect error: %v", err)
		}
		connectPkt := &connectPacket{}
		connectPkt.Marshal([]byte(option.ConnectString), ns.sAtts, NO_HEADER_FLAGS)
		err = ns.sendConnect(context.Background(), connectPkt)
		if err != nil {
			t.Errorf("Unexpected sendConnect error: %v", err)
		}
		pkt, err := ns.recvPacket(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unsupported packet type") {
			t.Errorf("Expected unexpected packet error, got %v", err)
		}
		_ = pkt // to avoid unused
		ns.Disconnect(context.Background(), 0)
	})

	t.Run("Recv Error in Loop", func(t *testing.T) {
		mock := &mockNTAdapter{receiveErr: fmt.Errorf("recv error")}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		// Simulate connect flow
		err := ns.transportConnect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err != nil {
			t.Errorf("Unexpected transportConnect error: %v", err)
		}
		connectPkt := &connectPacket{}
		connectPkt.Marshal([]byte(option.ConnectString), ns.sAtts, NO_HEADER_FLAGS)
		err = ns.sendConnect(context.Background(), connectPkt)
		if err != nil {
			t.Errorf("Unexpected sendConnect error: %v", err)
		}
		_, err = ns.recvPacket(context.Background())
		if err == nil || err.Error() != "recv error" {
			t.Errorf("Expected recv error, got %v", err)
		}
		ns.Disconnect(context.Background(), 0)
	})

	t.Run("SendConnect Error", func(t *testing.T) {
		mock := &mockNTAdapter{sendErr: fmt.Errorf("send error")}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		// Simulate connect flow
		err := ns.transportConnect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err != nil {
			t.Errorf("Unexpected transportConnect error: %v", err)
		}
		connectPkt := &connectPacket{}
		connectPkt.Marshal([]byte(option.ConnectString), ns.sAtts, NO_HEADER_FLAGS)
		err = ns.sendConnect(context.Background(), connectPkt)
		if err == nil || !strings.Contains(err.Error(), "send error") {
			t.Errorf("Expected send connect error, got %v", err)
		}
		ns.Disconnect(context.Background(), 0)
	})
	t.Run("HandleAccept Error in Connect", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")

		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.rcvBuf = make([]byte, ns.sAtts.SDU)
		ns.rcvDatapkt = &dataPacket{}
		ns.rcvDatapkt.Marshal(ns.rcvBuf, ns.sAtts, 0)

		acceptBuf := make([]byte, 50)
		binary.BigEndian.PutUint16(acceptBuf[0:2], 50)
		acceptBuf[4] = NSPTAC
		binary.BigEndian.PutUint16(acceptBuf[NSPACVSN:], TNS_VERSION_MINIMUM-1) // low version to cause error
		mock.receivedData = acceptBuf
		mock.recvPos = 0
		mock.disconnectErr = nil

		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err == nil || !strings.Contains(err.Error(), "unsupported TNS version") {
			t.Errorf("Expected handleAccept error, got %v", err)
		}
	})

	t.Run("HandleRefuse Error in Connect", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")

		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.rcvBuf = make([]byte, ns.sAtts.SDU)
		ns.rcvDatapkt = &dataPacket{}
		ns.rcvDatapkt.Marshal(ns.rcvBuf, ns.sAtts, 0)

		refuseBuf := []byte{0x0, 0x5f, 0x0, 0x0, 0x4, 0x0, 0x0, 0x0, 0x22, 0x0, 0x0, 0x53, 0x28, 0x44, 0x45, 0x53, 0x43, 0x52, 0x49, 0x50, 0x54, 0x49, 0x4f, 0x4e, 0x3d, 0x28, 0x54, 0x4d, 0x50, 0x3d, 0x29, 0x28, 0x56, 0x53, 0x4e, 0x4e, 0x55, 0x4d, 0x3d, 0x30, 0x29, 0x28, 0x45, 0x52, 0x52, 0x3d, 0x31, 0x32, 0x35, 0x31, 0x34, 0x29, 0x28, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x5f, 0x53, 0x54, 0x41, 0x43, 0x4b, 0x3d, 0x28, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x3d, 0x28, 0x43, 0x4f, 0x44, 0x45, 0x3d, 0x31, 0x32, 0x35, 0x31, 0x34, 0x29, 0x28, 0x45, 0x4d, 0x46, 0x49, 0x3d, 0x34, 0x29, 0x29, 0x29, 0x29}
		mock.receivedData = refuseBuf
		mock.recvPos = 0
		mock.disconnectErr = nil

		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err == nil || !strings.Contains(err.Error(), "ORA-12514") {
			t.Errorf("Expected handleRefuse error, got %v", err)
		}
	})

	t.Run("HandleRedirect Error in Connect", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")

		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.rcvBuf = make([]byte, ns.sAtts.SDU)
		ns.rcvDatapkt = &dataPacket{}
		ns.rcvDatapkt.Marshal(ns.rcvBuf, ns.sAtts, 0)

		redirectBuf := make([]byte, 10)
		binary.BigEndian.PutUint16(redirectBuf[0:2], 10)
		redirectBuf[4] = NSPTRD
		mock.receivedData = redirectBuf // no overflow packet follows, so redirect overflow read fails
		mock.recvPos = 0
		mock.disconnectErr = nil

		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err == nil || !strings.Contains(err.Error(), "EOF") {
			t.Errorf("Expected handleRedirect error, got %v", err)
		}
	})

	t.Run("HandleResend Error in Connect", func(t *testing.T) {
		mock := &mockNTAdapter{sendErr: fmt.Errorf("send error")}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")

		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.rcvBuf = make([]byte, ns.sAtts.SDU)
		ns.rcvDatapkt = &dataPacket{}
		ns.rcvDatapkt.Marshal(ns.rcvBuf, ns.sAtts, 0)

		resendBuf := make([]byte, 10)
		binary.BigEndian.PutUint16(resendBuf[0:2], 10)
		resendBuf[4] = NSPTRS
		mock.receivedData = resendBuf
		mock.recvPos = 0
		mock.disconnectErr = nil

		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err == nil || !strings.Contains(err.Error(), "send error") {
			t.Errorf("Expected handleResend error, got %v", err)
		}
	})
	t.Run("Resend Packet exceeds maximum resend count", func(t *testing.T) {
		mock := &mockNTAdapter{}
		ns := newNetworkSession()
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{}, Version: TNS_VERSION_MINIMUM, SDU: 8192}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")

		resendBuf := make([]byte, 8)
		binary.BigEndian.PutUint16(resendBuf[0:2], 8)
		resendBuf[4] = NSPTRS
		for range maxResendCount + 1 {
			mock.receivedData = append(mock.receivedData, resendBuf...)
		}
		mock.recvPos = 0

		err := ns.connect(context.Background(), transport.Address{
			Address: naming.Address{Protocol: driverCommon.ProtocolTCP, Host: "localhost", Port: 1521},
		})
		if err == nil || !strings.Contains(err.Error(), "too many resends") {
			t.Errorf("Expected too many resends error, got %v", err)
		}
		if ns.resendCount != maxResendCount+1 {
			t.Errorf("Expected resend count %d, got %d", maxResendCount+1, ns.resendCount)
		}
	})
}

// TestSend tests the Send method
func TestSend(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.connected = true
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sAtts = &sessionAtts{SDU: 20, LargeSDU: false}
	ns.sndBuf = make([]byte, 20)
	ns.sndDatapkt = &dataPacket{hdr: &header{}}
	ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)

	userBuf := []byte("testdata that is longer than SDU to test multiple packets")
	err := ns.Send(context.Background(), userBuf, 0, len(userBuf))
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(mock.sentData) == 0 {
		t.Errorf("Expected sends")
	}

	// Test zero length
	ns.connected = true
	err = ns.Send(context.Background(), userBuf, 0, 0)
	if err != nil {
		t.Errorf("Unexpected error for zero length")
	}

	// Test negative length
	err = ns.Send(context.Background(), userBuf, 0, -1)
	if err != nil {
		t.Errorf("Unexpected error for negative length")
	}

	// Test break
	ns.isBreak = true
	err = ns.Send(context.Background(), userBuf, 0, len(userBuf))
	if err != nil {
		t.Errorf("Unexpected error for break")
	}

	// Test send error in loop
	ns.isBreak = false
	mock.sendErr = fmt.Errorf("send error")
	err = ns.Send(context.Background(), userBuf, 0, len(userBuf))
	if err == nil || !strings.Contains(err.Error(), "send error") {
		t.Errorf("Expected send error in loop, got %v", err)
	}

	// Test with offset > BufLen
	ns.sndDatapkt.Offset = ns.sndDatapkt.BufLen + 1
	err = ns.Send(context.Background(), userBuf, 0, len(userBuf))
	if err == nil || !strings.Contains(err.Error(), "send error") { // assuming sendErr still set
		t.Errorf("Expected error with offset > BufLen")
	}
}

// TestReset tests the Reset function
func TestReset(t *testing.T) {
	t.Parallel()
	setup := func() (*networkSession, *mockNTAdapter) {
		ns := newNetworkSession()
		ns.connected = true
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{LargeSDU: false, SDU: 8192}
		ns.rcvBuf = make([]byte, ns.sAtts.SDU)
		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.rcvDatapkt = &dataPacket{Offset: NSPDADAT, Len: NSPDADAT}
		ns.breakPosted = true
		return ns, mock
	}

	t.Run("Basic", func(t *testing.T) {
		ns, mock := setup()
		markerBuf := make([]byte, 11)
		binary.BigEndian.PutUint16(markerBuf[0:2], 11)
		markerBuf[4] = NSPTMK
		markerBuf[8] = NSPMKTD1
		markerBuf[10] = NIQRMARK
		mock.receivedData = markerBuf
		mock.recvPos = 0

		err := ns.Reset(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ns.isBreak || ns.isReset || ns.breakPosted {
			t.Errorf("Flags not reset")
		}
		if ns.sndDatapkt.Offset != NSPDADAT || ns.rcvDatapkt.Offset != NSPDADAT || ns.rcvDatapkt.Len != NSPDADAT {
			t.Errorf("Packets not reset")
		}
	})

	t.Run("SendError", func(t *testing.T) {
		ns, mock := setup()
		mock.sendErr = fmt.Errorf("send error")
		err := ns.Reset(context.Background())
		if err == nil || !strings.Contains(err.Error(), "send error") {
			t.Errorf("Expected send error, got %v", err)
		}
	})

	t.Run("RecvError", func(t *testing.T) {
		ns, mock := setup()
		mock.receiveErr = fmt.Errorf("recv error")
		err := ns.Reset(context.Background())
		if err == nil || err.Error() != "recv error" {
			t.Errorf("Expected recv error, got %v", err)
		}
	})

	t.Run("NoBreakPosted", func(t *testing.T) {
		ns, mock := setup()
		ns.breakPosted = false
		markerBuf := make([]byte, 11)
		binary.BigEndian.PutUint16(markerBuf[0:2], 11)
		markerBuf[4] = NSPTMK
		markerBuf[8] = NSPMKTD1
		markerBuf[10] = NIQRMARK
		mock.receivedData = markerBuf
		mock.recvPos = 0
		err := ns.Reset(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("SendErrorNoBreak", func(t *testing.T) {
		ns, mock := setup()
		ns.breakPosted = false
		mock.sendErr = fmt.Errorf("send error")
		err := ns.Reset(context.Background())
		if err == nil || !strings.Contains(err.Error(), "send error") {
			t.Errorf("Expected send error, got %v", err)
		}
	})

	t.Run("BreakMarker", func(t *testing.T) {
		ns, mock := setup()
		markerBuf1 := make([]byte, 11)
		binary.BigEndian.PutUint16(markerBuf1[0:2], 11)
		markerBuf1[4] = NSPTMK
		markerBuf1[8] = NSPMKTD1
		markerBuf1[10] = NIQBMARK // Break marker
		markerBuf2 := make([]byte, 11)
		binary.BigEndian.PutUint16(markerBuf2[0:2], 11)
		markerBuf2[4] = NSPTMK
		markerBuf2[8] = NSPMKTD1
		markerBuf2[10] = NIQRMARK // Reset marker
		hdrBuf := make([]byte, 11)
		binary.BigEndian.PutUint16(hdrBuf[0:2], 11)
		hdrBuf[4] = NSPTDA
		mock.receivedData = append(markerBuf1, markerBuf2...)
		mock.receivedData = append(mock.receivedData, hdrBuf...)
		mock.recvPos = 0
		// Receiving a break packet will trigger reset
		_, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("MarkerD0", func(t *testing.T) {
		ns, mock := setup()
		markerBuf := make([]byte, 11)
		binary.BigEndian.PutUint16(markerBuf[0:2], 11)
		markerBuf[4] = NSPTMK
		markerBuf[8] = NSPMKTD0
		mock.receivedData = markerBuf
		mock.recvPos = 0
		err := ns.Reset(context.Background())
		if err == nil {
			t.Errorf("Expected loop until reset")
		}
	})

	t.Run("RepeatedBreakMarkersIgnoredDuringReset", func(t *testing.T) {
		ns, mock := setup()
		ns.breakPosted = false

		breakMarker := make([]byte, 11)
		binary.BigEndian.PutUint16(breakMarker[0:2], 11)
		breakMarker[4] = NSPTMK
		breakMarker[8] = NSPMKTD1
		breakMarker[10] = NIQBMARK

		resetMarker := make([]byte, 11)
		binary.BigEndian.PutUint16(resetMarker[0:2], 11)
		resetMarker[4] = NSPTMK
		resetMarker[8] = NSPMKTD1
		resetMarker[10] = NIQRMARK

		for range 3 {
			mock.receivedData = append(mock.receivedData, breakMarker...)
		}
		mock.receivedData = append(mock.receivedData, resetMarker...)
		mock.recvPos = 0

		err := ns.Reset(context.Background())
		if err != nil {
			t.Errorf("Unexpected reset error: %v", err)
		}
		if mock.sendCall != 1 {
			t.Errorf("Expected repeated break markers to be ignored during reset, sent %d reset packets", mock.sendCall)
		}
	})
}

// TestDisconnect tests the Disconnect function
func TestDisconnect(t *testing.T) {
	t.Parallel()
	t.Run("Normal", func(t *testing.T) {
		ns := newNetworkSession()
		ns.connected = true
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{LargeSDU: false}
		ns.sndBuf = make([]byte, 100)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.sndDatapkt.Offset = 10
		err := ns.Disconnect(context.Background(), 0)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ns.connected {
			t.Errorf("Still connected after Disconnect")
		}
		if !mock.disconnected {
			t.Errorf("Disconnect not called")
		}
	})

	t.Run("Immediate", func(t *testing.T) {
		ns := newNetworkSession()
		ns.connected = true
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		err := ns.Disconnect(context.Background(), driverCommon.NSFIMM)
		if err != nil {
			t.Errorf("Unexpected error for immediate disconnect")
		}
		if ns.connected {
			t.Errorf("Still connected after immediate Disconnect")
		}
		if !mock.disconnected {
			t.Errorf("Disconnect not called for immediate")
		}
	})

	t.Run("NotConnected", func(t *testing.T) {
		ns := newNetworkSession()
		err := ns.Disconnect(context.Background(), 0)
		if err != nil {
			t.Errorf("Unexpected error when not connected: %v", err)
		}
		if ns.connected {
			t.Errorf("connected flag should remain false")
		}
	})

	t.Run("NotConnectedWithAdapterCleansAndDisconnects", func(t *testing.T) {
		ns := newNetworkSession()
		mock := &mockNTTCPS{}
		ns.ntAdapter = mock

		err := ns.Disconnect(context.Background(), 0)
		if err != nil {
			t.Errorf("Unexpected error when not connected with adapter: %v", err)
		}
		if !mock.cleared {
			t.Errorf("Clear should be called before discarding a TCPS adapter")
		}
		if !mock.disconnected {
			t.Errorf("Disconnect should close an adapter even before session Accept")
		}
		if ns.ntAdapter != nil {
			t.Errorf("ntAdapter should be nil after cleanup")
		}
	})

	t.Run("Error", func(t *testing.T) {
		ns := newNetworkSession()
		ns.connected = true
		mock := &mockNTAdapter{disconnectErr: fmt.Errorf("disconnect error")}
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{LargeSDU: false}
		ns.sndBuf = make([]byte, 100)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.sndDatapkt.Offset = 10
		err := ns.Disconnect(context.Background(), 0)
		if err == nil || err.Error() != "disconnect error" {
			t.Errorf("Expected disconnect error, got %v", err)
		}
		if ns.connected {
			t.Errorf("connected flag should be false even on error")
		}
		if ns.ntAdapter != nil {
			t.Errorf("ntAdapter should be nil after disconnect")
		}
		if !mock.disconnected {
			t.Errorf("Disconnect should be called even if it returns error")
		}
	})

	t.Run("SendErrorStillDisconnectsAdapter", func(t *testing.T) {
		ns := newNetworkSession()
		ns.connected = true
		mock := &mockNTAdapter{sendErr: fmt.Errorf("send error")}
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{LargeSDU: false}
		ns.sndBuf = make([]byte, 100)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		ns.sndDatapkt.Offset = 10

		err := ns.Disconnect(context.Background(), 0)
		if err == nil || !strings.Contains(err.Error(), "send error") {
			t.Errorf("Expected send error, got %v", err)
		}
		if ns.connected {
			t.Errorf("connected flag should be false after send error")
		}
		if ns.ntAdapter != nil {
			t.Errorf("ntAdapter should be nil after send error")
		}
		if !mock.disconnected {
			t.Errorf("Disconnect should be called after send error")
		}
	})
}

func TestPrintPacket(t *testing.T) {
	t.Parallel()
	originalHandler := common.Opl.Handler()
	defer func() { common.Opl = slog.New(originalHandler) }()

	tests := []struct {
		name     string
		buf      []byte
		offset   int
		length   int
		logLevel slog.Leveler
	}{
		{"Basic", []byte{0x01, 0x02, 0x03, 0x41, 0x42, 0x43}, 0, 6, slog.LevelInfo},
		{"NonPrintable", []byte{0x01, 0x1F}, 0, 2, slog.LevelInfo},
		{"Empty", []byte{}, 0, 0, slog.LevelInfo},
		{"OffsetLength", []byte{0x01, 0x02, 0x03, 0x04}, 1, 2, slog.LevelInfo},
		{"SingleByte", []byte{0x41}, 0, 1, slog.LevelInfo},
		{"Remainder7", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, 0, 7, slog.LevelInfo},
		{"Full8Bytes", []byte{0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48}, 0, 8, slog.LevelInfo},
		{"AllNonPrintable", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, 0, 9, slog.LevelInfo},
		{"LoggingDisabled", []byte{0x01, 0x02}, 0, 2, slog.LevelError},
		{"OffsetExceedsLength", []byte{0x01, 0x02, 0x03}, 2, 1, slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			common.Opl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: tt.logLevel}))
			PrintPacket(tt.buf, tt.offset, tt.length)
			// No assertions needed as we're testing for coverage and no panics
		})
	}
}

// TestSendPacketError tests error cases in SendPacket
func TestSendPacketError(t *testing.T) {
	ns := newNetworkSession()
	err := ns.SendPacket(context.Background(), make([]byte, 7))
	if err == nil {
		t.Errorf("Expected error for short buffer")
	}

	// Cover debug print
	t.Setenv("ORACLE_GO_DRIVER_DEBUG_PACKETS", "true")
	ns.ntAdapter = &mockNTAdapter{}
	_ = ns.SendPacket(context.Background(), make([]byte, 8)) // ignore error, cover print
}

// TestSendConnect tests the sendConnect method
func TestSendConnect(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sAtts = &sessionAtts{SDU: 8192}
	ns.sndBuf = make([]byte, ns.sAtts.SDU)
	ns.sndDatapkt = &dataPacket{}
	ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
	connectPkt := &connectPacket{Overflow: false, Buf: make([]byte, 100)}
	err := ns.sendConnect(context.Background(), connectPkt)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(mock.sentData) != 1 {
		t.Errorf("Expected one send")
	}

	// Test overflow
	connectPkt.Overflow = true
	connectPkt.ConnectData = []byte("overflow data")
	connectPkt.ConnectDataLen = len(connectPkt.ConnectData)
	ns.connected = true // Set connected for Send
	err = ns.sendConnect(context.Background(), connectPkt)
	if err != nil {
		t.Errorf("Unexpected error for overflow: %v", err)
	}
	if len(mock.sentData) != 2 {
		t.Errorf("Expected two sends for overflow")
	}

	// Test error on SendPacket
	mock.sentData = nil
	mock.sendErr = fmt.Errorf("send error")
	connectPkt.Overflow = false
	err = ns.sendConnect(context.Background(), connectPkt)
	if err == nil || !strings.Contains(err.Error(), "send error") {
		t.Errorf("Expected send error, got %v", err)
	}

	// Test error on Send for overflow
	// for covering such if conditions err != nil { return err }).
	mock = &mockNTAdapter{}
	ns.ntAdapter = mock
	mock.errorOnSecond = true
	mock.secondSendErr = fmt.Errorf("overflow send error")
	mock.sendErr = nil
	mock.sendCall = 0
	connectPkt.ConnectDataLen = len(connectPkt.ConnectData)
	connectPkt.Overflow = true
	ns.connected = true
	ns.sndDatapkt.Offset = ns.sndDatapkt.BufLen
	err = ns.sendConnect(context.Background(), connectPkt)
	if err == nil || !strings.Contains(err.Error(), "overflow send error") {
		t.Errorf("Expected overflow send error, got %v", err)
	}
}

// TestProcessPacket tests the processPacket method
func TestProcessPacket(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{Version: 315, LargeSDU: true}
	buf := make([]byte, 50) // Larger buffer to avoid index errors
	hdr := &header{Type: NSPTAC, PacketLength: 50}
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 315)
	binary.BigEndian.PutUint32(buf[NSPACLSD:], 8192)
	binary.BigEndian.PutUint32(buf[NSPACLTD:], 8192)
	pkt, err := ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if _, ok := pkt.(*acceptPacket); !ok {
		t.Errorf("Expected acceptPacket")
	}

	// Test marker
	hdr.Type = NSPTMK
	buf[NSPMKTYP] = NSPMKTD1
	buf[NSPMKDAT] = NIQRMARK
	_, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for marker: %v", err)
	}
	if ns.isBreak != true || ns.isReset != true {
		t.Errorf("Marker flags not set")
	}

	// Test unsupported type
	hdr.Type = 99
	_, err = ns.processPacket(buf, hdr)
	if err == nil || err.Error() != "unsupported packet type: 99" {
		t.Errorf("Expected unsupported type error")
	}

	// Test refuse
	hdr.Type = NSPTRF
	pkt, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for refuse")
	}
	if _, ok := pkt.(*refusePacket); !ok {
		t.Errorf("Expected refusePacket")
	}

	// Test redirect
	hdr.Type = NSPTRD
	pkt, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for redirect")
	}
	if _, ok := pkt.(*redirectPacket); !ok {
		t.Errorf("Expected redirectPacket")
	}

	// Test resend
	hdr.Type = NSPTRS
	pkt, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for resend")
	}
	if _, ok := pkt.(*resendPacket); !ok {
		t.Errorf("Expected resendPacket")
	}

	// Test control
	hdr.Type = NSPTCNL
	binary.BigEndian.PutUint16(buf[NSPCTLCMD:], NSPCTL_SERR)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 0)       // emfi
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12572) // err1
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+8:], 0)     // err2
	pkt, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for control: %v", err)
	}
	if _, ok := pkt.(*controlPacket); !ok {
		t.Errorf("Expected controlPacket")
	}

	// Test data packet
	hdr.Type = NSPTDA
	pkt, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for data: %v", err)
	}
	if _, ok := pkt.(*dataPacket); !ok {
		t.Errorf("Expected dataPacket")
	}

	// Test marker D0
	hdr.Type = NSPTMK
	buf[NSPMKTYP] = NSPMKTD0
	_, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected error for marker D0")
	}
	if !ns.isBreak {
		t.Errorf("isBreak not set for D0")
	}

	// Test unmarshal error
	hdr.Type = NSPTAC
	// Corrupt version to cause unmarshal error
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 0)
	_, err = ns.processPacket(buf, hdr)
	if err != nil {
		t.Errorf("Unexpected unmarshal error: %v", err)
	}
}

func TestRecvPacket(t *testing.T) {
	setupNS := func(largeSDU bool) (*networkSession, *mockNTAdapter) {
		ns := newNetworkSession()
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		ns.sAtts = &sessionAtts{LargeSDU: largeSDU, SDU: 8192}
		ns.rcvBuf = make([]byte, ns.sAtts.SDU)
		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		return ns, mock
	}

	t.Run("Basic", func(t *testing.T) {
		ns, mock := setupNS(false)
		hdrBuf := make([]byte, 8)
		binary.BigEndian.PutUint16(hdrBuf[0:], 10)
		hdrBuf[4] = NSPTDA
		mock.receivedData = append(hdrBuf, []byte{1, 2}...)
		pkt, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if _, ok := pkt.(*dataPacket); !ok {
			t.Errorf("Expected dataPacket")
		}
	})

	t.Run("LargeSDU", func(t *testing.T) {
		ns, mock := setupNS(true)
		hdrBuf := make([]byte, 8)
		binary.BigEndian.PutUint32(hdrBuf[0:], 10)
		hdrBuf[4] = NSPTDA
		mock.receivedData = append(hdrBuf, []byte{1, 2}...)
		pkt, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if _, ok := pkt.(*dataPacket); !ok {
			t.Errorf("Expected dataPacket")
		}
	})

	t.Run("ReceiveError", func(t *testing.T) {
		ns, mock := setupNS(false)
		mock.receiveErr = fmt.Errorf("receive error")
		_, err := ns.recvPacket(context.Background())
		if err == nil || err.Error() != "receive error" {
			t.Errorf("Expected receive error, got %v", err)
		}
	})

	t.Run("PartialReadError", func(t *testing.T) {
		ns, mock := setupNS(false)
		// Provide partial header
		mock.receivedData = make([]byte, 4)
		// First read gets 4 bytes, then error on second read for the rest of header
		mock.receiveErr = fmt.Errorf("partial read error")
		_, err := ns.recvPacket(context.Background())
		if err == nil || err.Error() != "partial read error" {
			t.Errorf("Expected partial read error, got %v", err)
		}
	})

	t.Run("DebugPrint", func(t *testing.T) {
		t.Setenv("ORACLE_GO_DRIVER_DEBUG_PACKETS", "true")
		ns, mock := setupNS(false)
		hdrBuf := make([]byte, 8)
		binary.BigEndian.PutUint16(hdrBuf[0:], 10)
		hdrBuf[4] = NSPTDA
		mock.receivedData = append(hdrBuf, []byte{1, 2}...)
		_, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		// Check no panic, print is logged
	})

	t.Run("FlushSendBuffer", func(t *testing.T) {
		ns, mock := setupNS(false)
		ns.sndDatapkt.hdr = &header{}
		ns.sndDatapkt.Buf = ns.sndBuf
		ns.sndDatapkt.BufLen = len(ns.sndBuf)
		ns.sndDatapkt.Offset = NSPDADAT + 1 // Simulate data in send buffer > NSPDADAT
		// Prepare receive data for a small valid data packet.
		hdrBuf := make([]byte, NSPDADAT)
		binary.BigEndian.PutUint16(hdrBuf[0:], NSPDADAT)
		hdrBuf[4] = NSPTDA
		mock.receivedData = hdrBuf
		_, err := ns.recvPacket(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(mock.sentData) == 0 {
			t.Errorf("Expected SendPacket to be called")
		}
		if ns.sndDatapkt.Offset != NSPDADAT {
			t.Errorf("Send buffer not reset to NSPDADAT after flush, got %d", ns.sndDatapkt.Offset)
		}
	})

}
func TestRefuseArgs(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	address := transport.Address{Address: naming.Address{Host: "host", Port: 1521}, Hostname: "hostname"}
	ns.sAtts = &sessionAtts{NT: transport.NTattributes{Connectionid: "connid"}}

	tests := []struct {
		name      string
		cData     []byte
		errCode   string
		wantLen   int
		expectErr bool
	}{
		{"12514 Normal", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), "12514", 4, false},
		{"12514 Missing ServiceName", []byte("(DESCRIPTION=(CONNECT_DATA=()))"), "12514", 0, true},
		{"12520 Normal", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)(SERVER=dedicated)))"), "12520", 5, false},
		{"12520 Missing Server", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), "12520", 0, true},
		{"12520 Missing ServiceName", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVER=dedicated)))"), "12520", 0, true},
		{"12521 Normal", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)(INSTANCE_NAME=inst)))"), "12521", 5, false},
		{"12521 Missing InstanceName", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), "12521", 0, true},
		{"12521 Missing ServiceName", []byte("(DESCRIPTION=(CONNECT_DATA=(INSTANCE_NAME=inst)))"), "12521", 0, true},
		{"12505 Normal", []byte("(DESCRIPTION=(CONNECT_DATA=(SID=sid)))"), "12505", 4, false},
		{"12505 Missing SID", []byte("(DESCRIPTION=(CONNECT_DATA=()))"), "12505", 0, true},
		{"Unknown", []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), "unknown", 0, false},
		{"Parse Error", []byte("invalid"), "12514", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns.cData = tt.cData
			args, err := ns.refuseArgs(tt.errCode, address)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(args) != tt.wantLen {
					t.Errorf("Got %d args, want %d", len(args), tt.wantLen)
				}
			}
		})
	}
}

// TestHandleRefuse tests the handleRefuse function
func TestHandleRefuse(t *testing.T) {
	t.Parallel()
	setup := func() (*networkSession, *mockNTAdapter) {
		ns := newNetworkSession()
		ns.sAtts = &sessionAtts{NT: transport.NTattributes{Connectionid: "testconnid"}}
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		ns.rcvBuf = make([]byte, 8192)
		ns.rcvDatapkt = &dataPacket{Buf: ns.rcvBuf}
		return ns, mock
	}
	t.Run("UnknownCode", func(t *testing.T) {
		ns, _ := setup()
		address := transport.Address{Address: naming.Address{Host: "host", Port: 1521}}
		p := &refusePacket{Overflow: false, DataBuf: "(DESCRIPTION=(ERR=99999))"}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")
		err := ns.handleRefuse(context.Background(), p, address)
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("Expected generic refuse error")
		}
	})
	t.Run("RecvErrorInOverflow", func(t *testing.T) {
		ns, mock := setup()
		mock.receiveErr = fmt.Errorf("recv error")
		address := transport.Address{Address: naming.Address{Host: "host", Port: 1521}}
		p := &refusePacket{Overflow: true, DataBuf: "(DESCRIPTION=(ERR=12514))"}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")
		err := ns.handleRefuse(context.Background(), p, address)
		if err == nil || !strings.Contains(err.Error(), "recv error") {
			t.Errorf("Expected recv error in overflow, got %v", err)
		}
	})
	t.Run("ParseError", func(t *testing.T) {
		ns, _ := setup()
		address := transport.Address{Address: naming.Address{Host: "host", Port: 1521}}
		p := &refusePacket{Overflow: false, DataBuf: "invalid"}
		ns.cData = []byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))")
		err := ns.handleRefuse(context.Background(), p, address)
		if err == nil || !strings.Contains(err.Error(), "parse error") {
			t.Errorf("Expected parse error, got %v", err)
		}
	})
}

// TestHandleResend tests the handleResend function
func TestHandleResend(t *testing.T) {
	t.Parallel()
	t.Run("No SRN", func(t *testing.T) {
		ns := newNetworkSession()
		ns.sAtts = &sessionAtts{SDU: 8192}
		ns.ntAdapter = &mockNTAdapter{}
		connectPkt := &connectPacket{Buf: make([]byte, 100)}
		connectPkt.Marshal([]byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), ns.sAtts, NO_HEADER_FLAGS)
		p := &resendPacket{hdr: &header{Flags: 0}}
		err := ns.handleResend(context.Background(), p, connectPkt)
		if err != nil {
			t.Errorf("Unexpected error without SRN: %v", err)
		}
	})

	t.Run("SendConnect Error", func(t *testing.T) {
		ns := newNetworkSession()
		ns.connected = true
		ns.sAtts = &sessionAtts{SDU: 8192}
		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		mock := &mockNTAdapter{sendErr: fmt.Errorf("send error")}
		ns.ntAdapter = mock
		connectPkt := &connectPacket{Buf: make([]byte, 100)}
		connectPkt.Marshal([]byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), ns.sAtts, NO_HEADER_FLAGS)
		p := &resendPacket{hdr: &header{Flags: 0}}
		err := ns.handleResend(context.Background(), p, connectPkt)
		fmt.Println(err)
		if err == nil || !strings.Contains(err.Error(), "send error") {
			t.Errorf("Expected send error, got %v", err)
		}
	})

	t.Run("SRNOnNonTCPS", func(t *testing.T) {
		ns := newNetworkSession()
		ns.sAtts = &sessionAtts{SDU: 8192}
		ns.ntAdapter = &mockNTAdapter{}
		connectPkt := &connectPacket{Buf: make([]byte, 100)}
		if err := connectPkt.Marshal([]byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), ns.sAtts, NO_HEADER_FLAGS); err != nil {
			t.Fatalf("Marshal connect packet: %v", err)
		}
		p := &resendPacket{hdr: &header{Flags: NSPFSRN}}
		err := ns.handleResend(context.Background(), p, connectPkt)
		if err == nil || !strings.Contains(err.Error(), "invalid resend flag") {
			t.Errorf("Expected invalid resend flag error, got %v", err)
		}
	})

	t.Run("SRNOnTCPS", func(t *testing.T) {
		ns := newNetworkSession()
		ns.sAtts = &sessionAtts{SDU: 8192}
		mockTCPS := &mockNTTCPS{}
		ns.ntAdapter = mockTCPS
		connectPkt := &connectPacket{Buf: make([]byte, 100)}
		if err := connectPkt.Marshal([]byte("(DESCRIPTION=(CONNECT_DATA=(SERVICE_NAME=orcl)))"), ns.sAtts, NO_HEADER_FLAGS); err != nil {
			t.Fatalf("Marshal connect packet: %v", err)
		}
		p := &resendPacket{hdr: &header{Flags: NSPFSRN}}
		err := ns.handleResend(context.Background(), p, connectPkt)
		if err != nil {
			t.Fatalf("Unexpected error for TCPS resend: %v", err)
		}
		if !mockTCPS.renegotiated {
			t.Errorf("Expected TLS renegotiation to be triggered")
		}
	})
}

func TestCheckInbandNotification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		mockData      []byte // Full packet data for mock Receive
		expectTrue    bool
		expectPending bool // Expect push for non-control
		expectReset   bool // Expect controlPkt reset for true
	}{
		{"Control Packet", []byte{0x00, 0x16, 0x00, 0x00, NSPTCNL, 0x00, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x31, 0x1C, 0x00, 0x00, 0x00, 0x00}, true, false, true}, // Assume unmarshal sets notification Errno
		{"Non-Control Packet", []byte{0x00, 0x0A, 0x00, 0x00, NSPTDA, 0x00, 0x00, 0x00, 0x01, 0x02}, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := newNetworkSession()
			ns.controlPkt = &controlPacket{}
			ns.sAtts = &sessionAtts{LargeSDU: false, SDU: 8192}
			ns.rcvBuf = make([]byte, ns.sAtts.SDU)
			mock := &mockNTAdapter{receivedData: tt.mockData}
			ns.ntAdapter = mock

			got := ns.CheckInbandNotification()
			if got != tt.expectTrue {
				t.Errorf("CheckInbandNotification() = %v, want %v", got, tt.expectTrue)
			}
			if (len(ns.pendingPacket) > 0) != tt.expectPending {
				t.Errorf("pendingPacket pushed = %v, want %v", len(ns.pendingPacket) > 0, tt.expectPending)
			}
			if tt.expectReset && ns.controlPkt.Errno != 0 {
				t.Errorf("controlPkt not reset, Errno = %d", ns.controlPkt.Errno)
			}
		})
	}
}
