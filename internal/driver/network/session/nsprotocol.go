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
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/transport"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// NetworkSession represents a network session for communication with the server
type NetworkSession struct {
	Connected           bool
	IsBreak             bool
	IsReset             bool
	BreakPosted         bool
	CompressionEnabled  bool
	EndOfRequestSupport bool
	SupportsFastAuth    bool
	RedirectCount       int
	ResendCount         int
	SAtts               *SessionAtts
	NTAdapter           transport.NTAdapter
	CData               []byte
	CDataNVPair         interface{}
	SndDatapkt          *DataPacket
	RcvDatapkt          *DataPacket
	ControlPkt          *ControlPacket
	byteOrder           ByteOrder
	rcvBuf              []byte
	sndBuf              []byte
	pendingPacket       []byte // to store pushed back packet from CheckinbandNotification
	resetInProgress     bool
}

const (
	maxRedirectCount = 4
	maxResendCount   = 4
)

// NewNetworkSession creates a new NetworkSession instance
func NewNetworkSession() *NetworkSession {
	return &NetworkSession{
		Connected:          false,
		IsBreak:            false,
		IsReset:            false,
		BreakPosted:        false,
		CompressionEnabled: false,
		SndDatapkt:         &DataPacket{},
		RcvDatapkt:         &DataPacket{},
		ControlPkt:         &ControlPacket{},
		byteOrder:          BIG_ENDIAN,
	}
}

// GetRemoteAddress returns the connected remote TCP endpoint as "ip:port"
// when the session is using a TCPS transport, or an empty string if the
// remote address is not available.
func (ns *NetworkSession) GetRemoteAddress() string {
	tlsAdapter, ok := ns.NTAdapter.(*transport.NTTCPS)
	if !ok || tlsAdapter.Stream == nil {
		return ""
	}
	remoteAddr, ok := tlsAdapter.Stream.RemoteAddr().(*net.TCPAddr)
	if !ok || remoteAddr == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", remoteAddr.IP, remoteAddr.Port)
}

// transportConnect establishes the transport-level connection
func (ns *NetworkSession) transportConnect(ctx context.Context, address transport.Address) error {
	if address.Protocol == driverCommon.ProtocolTCP && address.HTTPSProxy != "" {
		return fmt.Errorf("https proxy requires protocol as tcps")
	}
	if ns.NTAdapter == nil {
		if address.Protocol == driverCommon.ProtocolTCP {
			ns.NTAdapter = transport.NewNTTCP(ns.SAtts.NT, TCP_DEFAULT_PORT)
		} else if address.Protocol == driverCommon.ProtocolTCPS {
			ns.NTAdapter = transport.NewNTTCPS(ns.SAtts.NT)
		}
	}
	err := ns.NTAdapter.Connect(ctx, address)
	if err != nil {
		return err
	}
	//initializes SndDatapkt with SDU size
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)

	err = ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
	if err != nil {
		return err
	}
	ns.RcvDatapkt = &DataPacket{}
	return nil
}
func (ns *NetworkSession) handleAccept(ctx context.Context, p *AcceptPacket) error {
	if ns.SAtts.Version < TNS_VERSION_MINIMUM {
		err := ns.Disconnect(ctx, 0)
		if err != nil {
			return err
		}
		return fmt.Errorf("unsupported TNS version: %d (minimum required: %d)", ns.SAtts.Version, TNS_VERSION_MINIMUM)
	}

	if ns.SAtts.Version >= TNS_VERSION_MIN_DATA_FLAGS {
		// sanity
		if len(p.Buf) < NSPACFL2+4 { // we gonna read an Uint32
			msg := fmt.Sprintf("Unexpected buffer length (%d) in accept packet", len(p.Buf))
			common.Odl.Warn(msg)
			return common.NewOracleError(oracleErrors.InternalError, nil, msg)
		}
		acceptFlag2 := binary.BigEndian.Uint32(p.Buf[NSPACFL2:])
		ns.EndOfRequestSupport = (acceptFlag2&TNS_ACCEPT_FLAG_HAS_END_OF_REQUEST != 0)
		ns.SupportsFastAuth = (acceptFlag2&TNS_ACCEPT_FLAG_FAST_AUTH != 0)
	}

	tlsadapter, ok := ns.NTAdapter.(*transport.NTTCPS)
	if ok {
		if err := tlsadapter.VerifyPostAcceptDNMatch(); err != nil {
			return err
		}
		tlsadapter.Clear()
	}
	ns.Connected = true
	ns.CData = nil
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.ControlPkt = &ControlPacket{}
	err := ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
	if err != nil {
		ns.Disconnect(ctx, 0)
		return err
	}
	return nil
}

// refuseArgs collects the placeholder values needed to format the
// localized message for a specific ORA code.
func (ns *NetworkSession) refuseArgs(errCode string, address transport.Address) ([]any, error) {
	// Parse ns.CData to extract service_name, host, port
	cDataStr := string(ns.CData)
	cDataNode, err := naming.Parse(cDataStr)
	if err != nil {
		return nil, err
	}
	switch errCode {
	case "12514":
		serviceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
		if err != nil {
			return nil, err
		}
		return []any{serviceName, address.Host, address.Port, ns.SAtts.NT.Connectionid}, nil
	case "12520":
		serverType, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVER")
		if err != nil {
			return nil, err
		}
		serviceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
		if err != nil {
			return nil, err
		}
		return []any{serverType, serviceName, address.Host, address.Port, ns.SAtts.NT.Connectionid}, nil
	case "12521":
		instanceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		serviceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
		if err != nil {
			return nil, err
		}
		return []any{instanceName, serviceName, address.Host, address.Port, ns.SAtts.NT.Connectionid}, nil
	case "12505":
		sid, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SID")
		if err != nil {
			return nil, err
		}
		return []any{sid, address.Host, address.Port, ns.SAtts.NT.Connectionid}, nil
	default:
		return nil, nil
	}
}
func (ns *NetworkSession) handleRefuse(ctx context.Context, p *RefusePacket, address transport.Address) error {
	if p.Overflow {
		_, err := ns.recvPacket(ctx)
		if err != nil {
			common.Odl.Error("An error occurred while receiving packet", "error", err)
			return err
		}
		p.DataBuf = string(ns.RcvDatapkt.Buf[ns.RcvDatapkt.Offset:ns.RcvDatapkt.Len])
	}
	refuseNode, err := naming.Parse(p.DataBuf)
	if err != nil {
		return fmt.Errorf("parse error in refuse data: %w", err)
	}
	errCode, err := refuseNode.GetValue("DESCRIPTION/ERR")
	if err != nil {
		return err
	}
	mappedCode, ok := oracleErrors.OracleRefuseErrorCodes[errCode]
	if !ok {
		return fmt.Errorf("connection refused: ERR code ORA-%s, user reason %d, system reason %d", errCode, p.UserReason, p.SystemReason)
	}

	args, err := ns.refuseArgs(errCode, address)
	if err != nil {
		return err
	}
	return common.NewOracleError(mappedCode, nil, args...)
}

func (ns *NetworkSession) handleRedirect(ctx context.Context, p *RedirectPacket, address transport.Address) error {
	ns.RedirectCount++
	if ns.RedirectCount > maxRedirectCount {
		return fmt.Errorf("too many redirects: exceeded maximum of %d", maxRedirectCount)
	}
	if p.Overflow {
		if _, err := ns.recvPacket(ctx); err != nil {
			return err
		}
		p.DataBuf = ns.RcvDatapkt.Buf[ns.RcvDatapkt.Offset:ns.RcvDatapkt.Len]
	}
	var addrStr string
	var redirectConnectData []byte
	if (p.hdr.Flags & byte(NSPFRDS)) != 0 {
		parts := bytes.Split(p.DataBuf, []byte{0})
		if len(parts) > 0 {
			addrStr = string(parts[0])
		}
		if len(parts) > 1 {
			redirectConnectData = parts[1]
		}
	} else {
		addrStr = string(p.DataBuf)
		redirectConnectData = ns.CData
	}
	redirAddressNode, err := naming.Parse(addrStr)
	if err != nil {
		return err
	}
	oldhostname := address.Hostname
	redirCtx, err := naming.ExtractConnectionContext(redirAddressNode)
	if err != nil {
		return err
	}
	iter := naming.NewConnectionIterator(ctx, redirAddressNode, redirCtx)

	if iter.HasNext() {
		redirOption := iter.Next()
		redirOption.Address.OriginHost = oldhostname
		hostToBeUsed := redirOption.Address.Host
		if redirOption.Address.ResolvedIP != "" {
			hostToBeUsed = redirOption.Address.ResolvedIP
		}
		newAddress := transport.Address{
			Address: naming.Address{
				Host:       hostToBeUsed,
				Port:       redirOption.Address.Port,
				Protocol:   redirOption.Address.Protocol,
				OriginHost: oldhostname,
				ResolvedIP: redirOption.Address.ResolvedIP,
			},
			Hostname: redirOption.Address.Host,
		}
		ns.NTAdapter.Disconnect()
		ns.Connected = false
		err = ns.transportConnect(ctx, newAddress)
		if err != nil {
			return err
		}
		ns.Connected = true
		connectPkt := &ConnectPacket{}
		//initializes SndDatapkt with SDU size
		ns.sndBuf = make([]byte, ns.SAtts.SDU)
		ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
		connectPkt.Marshal(redirectConnectData, ns.SAtts, NSPFRDR)
		err = ns.sendConnect(ctx, connectPkt)
		if err != nil {
			return err
		}
		return nil // continue the loop
	}
	return fmt.Errorf("no redirect option available")
}

func (ns *NetworkSession) handleResend(ctx context.Context, p *ResendPacket, connectPkt *ConnectPacket) error {
	if p.hdr.Flags&NSPFSRN != 0 {
		tlsAdapter, ok := ns.NTAdapter.(interface{ TLSReneg() })
		/*
			Oracle uses that flag on resend packets (NSPTRS) to tell the client
			"please renegotiate the TLS session" essentially a server-side TLS renegotiation request.
			In normal operation it should only come back when you're connected over TCPS, because there's
			nothing to renegotiate on plain TCP. However, since the packet header is remote-controlled,
			a hostile listener could flip the bit even on a TCP session, which is why we now treat it as
			an error when there's no TLS-capable adapter behind the session.
		*/
		if !ok {
			return fmt.Errorf("invalid resend flag for non-TCPS connection")
		}
		tlsAdapter.TLSReneg()
	}
	err := ns.sendConnect(ctx, connectPkt)
	if err != nil {
		ns.Disconnect(ctx, 0)
		return err
	}
	return nil // continue the loop
}

func (ns *NetworkSession) connect(ctx context.Context, address transport.Address) error {
	var err error
	err = ns.transportConnect(ctx, address)
	if err != nil {
		return err
	}
	connectPkt := &ConnectPacket{}
	connectPkt.Marshal(ns.CData, ns.SAtts, NO_HEADER_FLAGS)
	err = ns.sendConnect(ctx, connectPkt)
	if err != nil {
		if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
			return disconnectErr
		}
		return err
	}
	for {
		pkt, err := ns.recvPacket(ctx)
		if err != nil {
			if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
				return disconnectErr
			}
			return err
		}
		switch p := pkt.(type) {
		case *AcceptPacket:
			err = ns.handleAccept(ctx, p)
			if err != nil {
				if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
					return disconnectErr
				}
				return err
			}
			return nil
		case *RefusePacket:
			err = ns.handleRefuse(ctx, p, address)
			if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
				return disconnectErr
			}
			return err
		case *RedirectPacket:
			err = ns.handleRedirect(ctx, p, address)
			if err != nil {
				if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
					return disconnectErr
				}
				return err
			}
		case *ResendPacket:
			ns.ResendCount++
			if ns.ResendCount > maxResendCount {
				if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
					return disconnectErr
				}
				return fmt.Errorf("too many resends: exceeded maximum of %d", maxResendCount)
			}
			err = ns.handleResend(ctx, p, connectPkt)
			if err != nil {
				if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
					return disconnectErr
				}
				return err
			}
		default:
			if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
				return disconnectErr
			}
			return fmt.Errorf("unexpected packet type during connect")
		}
	}
}
func ConnectToOption(ctx context.Context, option *naming.ConnectionOption) (*NetworkSession, error) {
	return ConnectToOptionWithConnectionID(ctx, option, "")
}
func ConnectToOptionWithConnectionID(ctx context.Context, option *naming.ConnectionOption, connectionID string) (*NetworkSession, error) {
	ns := NewNetworkSession()

	portToBeUsed := option.Address.Port
	if option.Address.Port == 0 {
		common.Odl.Debug("no port specified, fall-back to default", "port", TCP_DEFAULT_PORT)
		portToBeUsed = TCP_DEFAULT_PORT
	}

	ns.SAtts = NewSessionAtts(connectionID)
	if option.Description != nil {
		ns.SAtts.SetFrom(option.Description)
	}
	err := ns.SAtts.Prepare(option.Address.Protocol)
	if err != nil {
		return nil, err
	}
	connectStr := option.ConnectString
	root, err := naming.Parse(connectStr)
	if err != nil {
		return nil, err
	}
	connectData, err := root.GetNode("DESCRIPTION/CONNECT_DATA")
	if err != nil {
		connectData = &naming.Node{Name: "CONNECT_DATA"}
		root.Children = append(root.Children, *connectData)
	}
	connIDNode := naming.Node{Name: "CONNECTION_ID", Value: ns.SAtts.NT.Connectionid}
	connectData.Children = append(connectData.Children, connIDNode)
	newConnectStr := root.ToString()
	ns.CData = []byte(newConnectStr)

	hostToBeUsed := option.Address.Host
	if option.Address.ResolvedIP != "" {
		hostToBeUsed = option.Address.ResolvedIP
	}
	address := transport.Address{
		Address: naming.Address{
			Host:       hostToBeUsed,
			Port:       portToBeUsed,
			Protocol:   option.Address.Protocol,
			ResolvedIP: option.Address.ResolvedIP,
		},
		Hostname: option.Address.Host,
	}

	err = ns.connect(ctx, address)
	if err != nil {
		return nil, err
	}
	return ns, nil

}

// SendConnect sends the NSPTCN connect packet
func (ns *NetworkSession) sendConnect(ctx context.Context, connectPkt *ConnectPacket) error {

	err := ns.SendPacket(ctx, connectPkt.Buf)
	if err != nil {
		return fmt.Errorf("NS send connect packet failed: %w", err)
	}
	if connectPkt.Overflow {
		err = ns.Send(ctx, connectPkt.ConnectData, 0, connectPkt.ConnectDataLen)
		if err != nil {
			return fmt.Errorf("NS send connect packet failed: %w", err)
		}
	}
	return nil
}

// recvPacket receives a packet from the network and returns its unmarshaled struct
func (ns *NetworkSession) recvPacket(ctx context.Context) (any, error) {
	// Handle pending packet from inband notification check
	if ns.pendingPacket != nil {
		buf := ns.pendingPacket
		ns.pendingPacket = nil
		hdr := &Header{}
		err := hdr.Unmarshal(buf, ns.SAtts, nil)
		if err != nil {
			return nil, err
		}
		return ns.processPacket(buf, hdr)
	}

	if ns.SndDatapkt.Offset > NSPDADAT { /* Flush any data left in send buffer */
		err := ns.SndDatapkt.Prepare2Send(0, ns.SAtts)
		if err != nil {
			return nil, err
		}
		err = ns.SendPacket(ctx, ns.SndDatapkt.Buf[:ns.SndDatapkt.Offset])
		if err != nil {
			return nil, err
		}
		ns.SndDatapkt.Reset()
	}
	const PACKET_HEADER_SIZE = 8
	var packetLen int
	//read packet header first
	n, err := ns.NTAdapter.Receive(ctx, ns.rcvBuf, PACKET_HEADER_SIZE)
	if err != nil {
		return nil, err
	}
	// get packetLen
	if ns.SAtts.LargeSDU {
		packetLen = int(binary.BigEndian.Uint32(ns.rcvBuf[0:4]))
	} else {
		packetLen = int(binary.BigEndian.Uint16(ns.rcvBuf[0:2]))
	}

	if packetLen < PACKET_HEADER_SIZE || packetLen > len(ns.rcvBuf) {
		return nil, fmt.Errorf("invalid packet length: %d", packetLen)
	}
	bodyLen := packetLen - PACKET_HEADER_SIZE
	if bodyLen > 0 {
		n, err = ns.NTAdapter.Receive(ctx, ns.rcvBuf[PACKET_HEADER_SIZE:packetLen], bodyLen)
		if err != nil {
			return nil, err
		}
		if n != bodyLen {
			return nil, fmt.Errorf("incomplete body read: got %d, expected %d", n, bodyLen)
		}
	}
	buf := ns.rcvBuf[:packetLen]
	PrintPacket(buf, 0, packetLen)

	hdr := &Header{}
	err = hdr.Unmarshal(buf, ns.SAtts, nil)
	if err != nil {
		return nil, err
	}
	packet, err := ns.processPacket(buf, hdr)
	if err != nil {
		return nil, err
	}

	// Handle reset when break is received and reset has not yet been received.
	if hdr.Type == NSPTMK && ns.IsBreak && !ns.IsReset {
		common.Odl.Debug("Received break packet from server")
		if ns.resetInProgress {
			// Reset is already draining packets until NIQRMARK is received.
			// Return this marker to the Reset loop instead of recursively
			// starting another Reset.
			return packet, nil
		}
		if err := ns.Reset(ctx); err != nil {
			return nil, err
		}
		return ns.recvPacket(ctx)
	}

	return packet, nil
}

// processPacket processes a packet and returns its unmarshaled struct
func (ns *NetworkSession) processPacket(buf []byte, hdr *Header) (any, error) {
	var packet Packet
	switch hdr.Type {
	case NSPTAC:
		packet = &AcceptPacket{}
	case NSPTRF:
		packet = &RefusePacket{}
	case NSPTRD:
		packet = &RedirectPacket{}
	case NSPTRS:
		packet = &ResendPacket{}
	case NSPTMK:
		packet = &MarkerPacket{}
	case NSPTCNL:
		packet = ns.ControlPkt
	case NSPTDA:
		packet = ns.RcvDatapkt
	default:
		return nil, fmt.Errorf("unsupported packet type: %d", hdr.Type)
	}
	err := packet.Unmarshal(buf, ns.SAtts, hdr)
	if err != nil {
		return nil, err
	}
	if hdr.Type == NSPTMK {
		p := packet.(*MarkerPacket)
		common.Odl.Debug("marker packet received", "marker-type", p.MarkerType, "data", p.Data)
		switch p.MarkerType {
		case NSPMKTD0:
			ns.IsBreak = true
		case NSPMKTD1:
			ns.IsBreak = true
			if p.Data == NIQRMARK {
				ns.IsReset = true
			}
		}
	}
	return packet, nil
}

// SendPacket sends a packet with optional compression
func (ns *NetworkSession) SendPacket(ctx context.Context, buf []byte) error {
	PrintPacket(buf, 0, len(buf))
	if len(buf) < PACKET_HEADER_SIZE {
		return fmt.Errorf("buffer too short: %d bytes, need at least %d", len(buf), PACKET_HEADER_SIZE)
	}
	return ns.NTAdapter.Send(ctx, buf)
}

// Send transmits the provided user data (userBuf) starting from the given offset for the specified length.
// It handles breaking the data into packets if necessary, using the send data packet (SndDatapkt) for buffering.
func (ns *NetworkSession) Send(ctx context.Context, userBuf []byte, offset, len int) error {
	if ns.IsBreak {
		return nil
	}
	if len <= 0 {
		return nil
	}
	bytesCopied := 0
	// Check if the current send packet has available space (offset < BufLen) to accommodate more data
	// without needing to send the packet immediately. If true, fill the buffer with as much user data
	// as possible, update lengths and offsets, and continue processing any remaining data in subsequent iterations.
	if ns.SndDatapkt.Offset < ns.SndDatapkt.BufLen {
		bytesCopied = ns.SndDatapkt.FillBuf(userBuf, offset, len, 0, ns.SAtts.LargeSDU)
		len -= bytesCopied
		offset += bytesCopied
	}
	for len > 0 {
		err := ns.SndDatapkt.Prepare2Send(0, ns.SAtts)
		if err != nil {
			return err
		}
		err = ns.SendPacket(ctx, ns.SndDatapkt.Buf)
		ns.SndDatapkt.Reset()
		if err != nil {
			return err
		}
		if ns.IsBreak {
			return nil
		}
		bytesCopied = ns.SndDatapkt.FillBuf(userBuf, offset, len, 0, ns.SAtts.LargeSDU)
		len -= bytesCopied
		offset += bytesCopied
	}
	return nil
}

// Reset resets the connection
func (ns *NetworkSession) Reset(ctx context.Context) error {
	if ns.resetInProgress {
		return nil
	}
	ns.resetInProgress = true
	defer func() {
		ns.resetInProgress = false
	}()

	var markerPkt = &MarkerPacket{}
	if ns.BreakPosted {
		err := markerPkt.Marshal(nil, ns.SAtts, NIQBMARK)
		if err != nil {
			return err
		}
		err = ns.SendPacket(ctx, markerPkt.Buf)
		if err != nil {
			return err
		}
		ns.BreakPosted = false
	}
	err := markerPkt.Marshal(nil, ns.SAtts, NIQRMARK)
	if err != nil {
		return err
	}
	err = ns.SendPacket(ctx, markerPkt.Buf)
	common.Odl.Debug("Reset packet sent")
	if err != nil {
		common.Odl.Error("An error occurred while sending reset", "error", err)
		return err
	}
	for !ns.IsReset {
		_, err := ns.recvPacket(ctx)
		if err != nil {
			common.Odl.Error("An error occurred while receiving packet", "error", err)
			return err
		}
		common.Odl.Debug("Packet received", "IsReset", ns.IsReset)
	}
	//reset sndDatapkt
	ns.SndDatapkt.Reset()
	ns.RcvDatapkt.Offset = NSPDADAT
	ns.RcvDatapkt.Len = ns.RcvDatapkt.Offset
	//set break/reset as false
	ns.IsBreak = false
	ns.IsReset = false
	common.Odl.Debug("End of break-reset")
	return nil
}

// Disconnect
func (ns *NetworkSession) Disconnect(ctx context.Context, flags int) error {
	if !ns.Connected {
		if ns.NTAdapter != nil {
			if cleaner, ok := ns.NTAdapter.(interface{ Clear() }); ok {
				cleaner.Clear()
			}
			disconnectErr := ns.NTAdapter.Disconnect()
			ns.NTAdapter = nil
			return disconnectErr
		}
		return nil
	}
	ns.Connected = false
	var err error
	if flags&driverCommon.NSFIMM == 0 {
		if prepareErr := ns.SndDatapkt.Prepare2Send(NSPDAFEOF, ns.SAtts); prepareErr != nil {
			err = prepareErr
		} else if sendErr := ns.SendPacket(ctx, ns.SndDatapkt.Buf); sendErr != nil {
			err = sendErr
		}
	}
	if cleaner, ok := ns.NTAdapter.(interface{ Clear() }); ok {
		cleaner.Clear()
	}
	disconnectErr := ns.NTAdapter.Disconnect()
	ns.NTAdapter = nil
	if err != nil {
		return err
	}
	return disconnectErr
}
func (ns *NetworkSession) IsLittleEndian() bool {
	return ns.byteOrder == LITTLE_ENDIAN
}

func (ns *NetworkSession) CheckInbandNotification() bool {
	// Control packet already read
	if ns.ControlPkt.Errno != 0 {
		if ns.ControlPkt.IsNotification {
			ns.ControlPkt.Clear() //reset
			return true
		}
		ns.ControlPkt.Clear() //reset
		return false

	}
	// Short timeout for header
	ctxHeader, cancelHeader := context.WithTimeout(common.BackgroundContext, 50*time.Microsecond)
	defer cancelHeader()
	// Read header
	n, err := ns.NTAdapter.Receive(ctxHeader, ns.rcvBuf, PACKET_HEADER_SIZE)
	if err != nil || n != PACKET_HEADER_SIZE {
		return false // Timeout
	}
	// Get packet length
	var packetLen int
	if ns.SAtts.LargeSDU {
		packetLen = int(binary.BigEndian.Uint32(ns.rcvBuf[0:4]))
	} else {
		packetLen = int(binary.BigEndian.Uint16(ns.rcvBuf[0:2]))
	}

	// Longer timeout for body to avoid inconsistent state
	bodyLen := packetLen - PACKET_HEADER_SIZE
	if bodyLen > 0 {
		ctxBody, cancelBody := context.WithTimeout(common.BackgroundContext, 10*time.Second)
		defer cancelBody()
		n, errBody := ns.NTAdapter.Receive(ctxBody, ns.rcvBuf[PACKET_HEADER_SIZE:], bodyLen)
		if errBody != nil || n < bodyLen {
			return false
		}
	}

	// Full packet; unmarshal header
	buf := ns.rcvBuf[:packetLen]
	hdr := &Header{}
	if err := hdr.Unmarshal(buf, ns.SAtts, nil); err != nil {
		return false
	}

	if hdr.Type == NSPTCNL {
		ns.ControlPkt.Unmarshal(buf, ns.SAtts, hdr)
		if ns.ControlPkt.IsNotification {
			ns.ControlPkt.Clear() //reset
			return true
		}
		ns.ControlPkt.Clear()
		return false
	} else {
		// Push back
		ns.pendingPacket = make([]byte, packetLen)
		copy(ns.pendingPacket, buf)
		return false
	}
}
func PrintPacket(buf []byte, offset, length int) {

	if !common.Opl.Enabled(nil, slog.LevelInfo) {
		return
	}

	var line bytes.Buffer
	var lineL bytes.Buffer

	common.Opl.Info("PrintPacket", "Capacity", len(buf),
		"Offset", offset, "Data Length", length)

	for i, b := range buf[offset : offset+length] {
		// Format byte as 2-digit hex with leading 0
		hexByte := fmt.Sprintf("%02X", b)
		if line.Len() != 0 {
			line.WriteString(" ")
		}
		line.WriteString(hexByte)
		if b >= 33 && b <= 126 {
			// Printable ASCII range
			lineL.WriteString(fmt.Sprintf("%c", b))
		} else {
			// Non-printable, replace with dot
			lineL.WriteString(".")
		}

		// If 4 bytes written or it's the last byte, print the line
		if (i+1)%8 == 0 || i == len(buf)-1 {
			common.Opl.Info(fmt.Sprintf("%-8s %s", lineL.String(), line.String()))
			lineL.Reset()
			line.Reset()
		}
	}
}
