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
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/transport"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// networkSession represents a network session for communication with the server
type networkSession struct {
	connected           bool
	isBreak             bool
	isReset             bool
	breakPosted         bool
	compressionEnabled  bool
	endOfRequestSupport bool
	supportsFastAuth    bool
	redirectCount       int
	resendCount         int
	sAtts               *sessionAtts
	ntAdapter           transport.NTAdapter
	cData               []byte
	cDataNVPair         interface{}
	sndDatapkt          *dataPacket
	rcvDatapkt          *dataPacket
	controlPkt          *controlPacket
	byteOrder           driverCommon.ByteOrder
	rcvBuf              []byte
	sndBuf              []byte
	pendingPacket       []byte // to store pushed back packet from CheckinbandNotification
	resetInProgress     bool
}

const (
	maxRedirectCount = 4
	maxResendCount   = 4
)

// newNetworkSession creates a new networkSession instance
func newNetworkSession() *networkSession {
	return &networkSession{
		connected:          false,
		isBreak:            false,
		isReset:            false,
		breakPosted:        false,
		compressionEnabled: false,
		sndDatapkt:         &dataPacket{},
		rcvDatapkt:         &dataPacket{},
		controlPkt:         &controlPacket{},
		byteOrder:          driverCommon.BIG_ENDIAN,
	}
}

// transportConnect establishes the transport-level connection
func (ns *networkSession) transportConnect(ctx context.Context, address transport.Address) error {
	if address.Protocol == driverCommon.ProtocolTCP && address.HTTPSProxy != "" {
		return fmt.Errorf("https proxy requires protocol as tcps")
	}
	if ns.ntAdapter == nil {
		if address.Protocol == driverCommon.ProtocolTCP {
			ns.ntAdapter = transport.NewNTTCP(ns.sAtts.NT, TCP_DEFAULT_PORT)
		} else if address.Protocol == driverCommon.ProtocolTCPS {
			ns.ntAdapter = transport.NewNTTCPS(ns.sAtts.NT)
		}
	}
	err := ns.ntAdapter.Connect(ctx, address)
	if err != nil {
		return err
	}
	//initializes sndDatapkt with SDU size
	ns.sndBuf = make([]byte, ns.sAtts.SDU)
	ns.rcvBuf = make([]byte, ns.sAtts.SDU)

	err = ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
	if err != nil {
		return err
	}
	ns.rcvDatapkt = &dataPacket{}
	return nil
}
func (ns *networkSession) handleAccept(ctx context.Context, p *acceptPacket) error {
	if ns.sAtts.Version < TNS_VERSION_MINIMUM {
		err := ns.Disconnect(ctx, 0)
		if err != nil {
			return err
		}
		return fmt.Errorf("unsupported TNS version: %d (minimum required: %d)", ns.sAtts.Version, TNS_VERSION_MINIMUM)
	}

	if ns.sAtts.Version >= TNS_VERSION_MIN_DATA_FLAGS {
		// sanity
		if len(p.Buf) < NSPACFL2+4 { // we gonna read an Uint32
			msg := fmt.Sprintf("Unexpected buffer length (%d) in accept packet", len(p.Buf))
			common.Odl.Warn(msg)
			return common.NewOracleError(oracleErrors.InternalError, nil, msg)
		}
		acceptFlag2 := binary.BigEndian.Uint32(p.Buf[NSPACFL2:])
		ns.endOfRequestSupport = (acceptFlag2&TNS_ACCEPT_FLAG_HAS_END_OF_REQUEST != 0)
		ns.supportsFastAuth = (acceptFlag2&TNS_ACCEPT_FLAG_FAST_AUTH != 0)
	}

	tlsadapter, ok := ns.ntAdapter.(interface {
		VerifyPostAcceptDNMatch() error
		Clear()
	})
	if ok {
		if err := tlsadapter.VerifyPostAcceptDNMatch(); err != nil {
			return err
		}
		tlsadapter.Clear()
	}
	ns.connected = true
	ns.cData = nil
	ns.sndBuf = make([]byte, ns.sAtts.SDU)
	ns.rcvBuf = make([]byte, ns.sAtts.SDU)
	ns.controlPkt = &controlPacket{}
	err := ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
	if err != nil {
		ns.Disconnect(ctx, 0)
		return err
	}
	return nil
}

// refuseArgs collects the placeholder values needed to format the
// localized message for a specific ORA code.
func (ns *networkSession) refuseArgs(errCode string, address transport.Address) ([]any, error) {
	// Parse ns.cData to extract service_name, host, port
	cDataStr := string(ns.cData)
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
		return []any{serviceName, address.Host, address.Port, ns.sAtts.NT.Connectionid}, nil
	case "12520":
		serverType, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVER")
		if err != nil {
			return nil, err
		}
		serviceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
		if err != nil {
			return nil, err
		}
		return []any{serverType, serviceName, address.Host, address.Port, ns.sAtts.NT.Connectionid}, nil
	case "12521":
		instanceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		serviceName, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SERVICE_NAME")
		if err != nil {
			return nil, err
		}
		return []any{instanceName, serviceName, address.Host, address.Port, ns.sAtts.NT.Connectionid}, nil
	case "12505":
		sid, err := cDataNode.GetValue("DESCRIPTION/CONNECT_DATA/SID")
		if err != nil {
			return nil, err
		}
		return []any{sid, address.Host, address.Port, ns.sAtts.NT.Connectionid}, nil
	default:
		return nil, nil
	}
}
func (ns *networkSession) handleRefuse(ctx context.Context, p *refusePacket, address transport.Address) error {
	if p.Overflow {
		_, err := ns.recvPacket(ctx)
		if err != nil {
			common.Odl.Error("An error occurred while receiving packet", "error", err)
			return err
		}
		p.DataBuf = string(ns.rcvDatapkt.Buf[ns.rcvDatapkt.Offset:ns.rcvDatapkt.Len])
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

func (ns *networkSession) handleRedirect(ctx context.Context, p *redirectPacket, address transport.Address) error {
	ns.redirectCount++
	if ns.redirectCount > maxRedirectCount {
		return fmt.Errorf("too many redirects: exceeded maximum of %d", maxRedirectCount)
	}
	if p.Overflow {
		if _, err := ns.recvPacket(ctx); err != nil {
			return err
		}
		p.DataBuf = ns.rcvDatapkt.Buf[ns.rcvDatapkt.Offset:ns.rcvDatapkt.Len]
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
		redirectConnectData = ns.cData
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
		ns.ntAdapter.Disconnect()
		ns.connected = false
		err = ns.transportConnect(ctx, newAddress)
		if err != nil {
			return err
		}
		ns.connected = true
		connectPkt := &connectPacket{}
		//initializes sndDatapkt with SDU size
		ns.sndBuf = make([]byte, ns.sAtts.SDU)
		ns.sndDatapkt.Marshal(ns.sndBuf, ns.sAtts, 0)
		connectPkt.Marshal(redirectConnectData, ns.sAtts, NSPFRDR)
		err = ns.sendConnect(ctx, connectPkt)
		if err != nil {
			return err
		}
		return nil // continue the loop
	}
	return fmt.Errorf("no redirect option available")
}

func (ns *networkSession) handleResend(ctx context.Context, p *resendPacket, connectPkt *connectPacket) error {
	if p.hdr.Flags&NSPFSRN != 0 {
		tlsAdapter, ok := ns.ntAdapter.(interface{ TLSReneg() })
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

func (ns *networkSession) connect(ctx context.Context, address transport.Address) error {
	var err error
	err = ns.transportConnect(ctx, address)
	if err != nil {
		return err
	}
	connectPkt := &connectPacket{}
	connectPkt.Marshal(ns.cData, ns.sAtts, NO_HEADER_FLAGS)
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
		case *acceptPacket:
			err = ns.handleAccept(ctx, p)
			if err != nil {
				if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
					return disconnectErr
				}
				return err
			}
			return nil
		case *refusePacket:
			err = ns.handleRefuse(ctx, p, address)
			if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
				return disconnectErr
			}
			return err
		case *redirectPacket:
			err = ns.handleRedirect(ctx, p, address)
			if err != nil {
				if disconnectErr := ns.Disconnect(ctx, 0); disconnectErr != nil {
					return disconnectErr
				}
				return err
			}
		case *resendPacket:
			ns.resendCount++
			if ns.resendCount > maxResendCount {
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
func ConnectToOption(ctx context.Context, option *naming.ConnectionOption) (driverCommon.NetworkSession, error) {
	return ConnectToOptionWithConnectionID(ctx, option, "")
}
func ConnectToOptionWithConnectionID(ctx context.Context, option *naming.ConnectionOption, connectionID string) (driverCommon.NetworkSession, error) {
	ns := newNetworkSession()
	addressOption := option.Address
	description := option.Description

	portToBeUsed := addressOption.Port
	if addressOption.Port == 0 {
		common.Odl.Debug("no port specified, fall-back to default", "port", TCP_DEFAULT_PORT)
		portToBeUsed = TCP_DEFAULT_PORT
	}

	ns.sAtts = newSessionAtts(connectionID)
	if description != nil {
		ns.sAtts.setFrom(description)
	}
	err := ns.sAtts.prepare(addressOption.Protocol)
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
	connIDNode := naming.Node{Name: "CONNECTION_ID", Value: ns.sAtts.NT.Connectionid}
	connectData.Children = append(connectData.Children, connIDNode)
	newConnectStr := root.ToString()
	ns.cData = []byte(newConnectStr)

	hostToBeUsed := addressOption.Host
	if addressOption.ResolvedIP != "" {
		hostToBeUsed = addressOption.ResolvedIP
	}
	address := transport.Address{
		Address: naming.Address{
			Host:       hostToBeUsed,
			Port:       portToBeUsed,
			Protocol:   addressOption.Protocol,
			ResolvedIP: addressOption.ResolvedIP,
		},
		Hostname: addressOption.Host,
	}

	err = ns.connect(ctx, address)
	if err != nil {
		return nil, err
	}
	return ns, nil

}

// SendConnect sends the NSPTCN connect packet
func (ns *networkSession) sendConnect(ctx context.Context, connectPkt *connectPacket) error {

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
func (ns *networkSession) recvPacket(ctx context.Context) (any, error) {
	// Handle pending packet from inband notification check
	if ns.pendingPacket != nil {
		buf := ns.pendingPacket
		ns.pendingPacket = nil
		hdr := &header{}
		err := hdr.Unmarshal(buf, ns.sAtts, nil)
		if err != nil {
			return nil, err
		}
		return ns.processPacket(buf, hdr)
	}

	if ns.sndDatapkt.Offset > NSPDADAT { /* Flush any data left in send buffer */
		err := ns.sndDatapkt.Prepare2Send(0, ns.sAtts)
		if err != nil {
			return nil, err
		}
		err = ns.SendPacket(ctx, ns.sndDatapkt.Buf[:ns.sndDatapkt.Offset])
		if err != nil {
			return nil, err
		}
		ns.sndDatapkt.Reset()
	}
	const PACKET_HEADER_SIZE = 8
	var packetLen int
	//read packet header first
	n, err := ns.ntAdapter.Receive(ctx, ns.rcvBuf, PACKET_HEADER_SIZE)
	if err != nil {
		return nil, err
	}
	// get packetLen
	if ns.sAtts.LargeSDU {
		packetLen = int(binary.BigEndian.Uint32(ns.rcvBuf[0:4]))
	} else {
		packetLen = int(binary.BigEndian.Uint16(ns.rcvBuf[0:2]))
	}

	if packetLen < PACKET_HEADER_SIZE || packetLen > len(ns.rcvBuf) {
		return nil, fmt.Errorf("invalid packet length: %d", packetLen)
	}
	bodyLen := packetLen - PACKET_HEADER_SIZE
	if bodyLen > 0 {
		n, err = ns.ntAdapter.Receive(ctx, ns.rcvBuf[PACKET_HEADER_SIZE:packetLen], bodyLen)
		if err != nil {
			return nil, err
		}
		if n != bodyLen {
			return nil, fmt.Errorf("incomplete body read: got %d, expected %d", n, bodyLen)
		}
	}
	buf := ns.rcvBuf[:packetLen]
	PrintPacket(buf, 0, packetLen)

	hdr := &header{}
	err = hdr.Unmarshal(buf, ns.sAtts, nil)
	if err != nil {
		return nil, err
	}
	packet, err := ns.processPacket(buf, hdr)
	if err != nil {
		return nil, err
	}

	// Handle reset when break is received and reset has not yet been received.
	if hdr.Type == NSPTMK && ns.isBreak && !ns.isReset {
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
func (ns *networkSession) processPacket(buf []byte, hdr *header) (any, error) {
	var packet packet
	switch hdr.Type {
	case NSPTAC:
		packet = &acceptPacket{}
	case NSPTRF:
		packet = &refusePacket{}
	case NSPTRD:
		packet = &redirectPacket{}
	case NSPTRS:
		packet = &resendPacket{}
	case NSPTMK:
		packet = &markerPacket{}
	case NSPTCNL:
		packet = ns.controlPkt
	case NSPTDA:
		packet = ns.rcvDatapkt
	default:
		return nil, fmt.Errorf("unsupported packet type: %d", hdr.Type)
	}
	err := packet.Unmarshal(buf, ns.sAtts, hdr)
	if err != nil {
		return nil, err
	}
	if hdr.Type == NSPTMK {
		p := packet.(*markerPacket)
		common.Odl.Debug("marker packet received", "marker-type", p.MarkerType, "data", p.Data)
		switch p.MarkerType {
		case NSPMKTD0:
			ns.isBreak = true
		case NSPMKTD1:
			ns.isBreak = true
			if p.Data == NIQRMARK {
				ns.isReset = true
			}
		}
	}
	return packet, nil
}

// SendPacket sends a packet with optional compression
func (ns *networkSession) SendPacket(ctx context.Context, buf []byte) error {
	PrintPacket(buf, 0, len(buf))
	if len(buf) < PACKET_HEADER_SIZE {
		return fmt.Errorf("buffer too short: %d bytes, need at least %d", len(buf), PACKET_HEADER_SIZE)
	}
	return ns.ntAdapter.Send(ctx, buf)
}

// Send transmits the provided user data (userBuf) starting from the given offset for the specified length.
// It handles breaking the data into packets if necessary, using the send data packet (sndDatapkt) for buffering.
func (ns *networkSession) Send(ctx context.Context, userBuf []byte, offset, len int) error {
	if ns.isBreak {
		return nil
	}
	if len <= 0 {
		return nil
	}
	bytesCopied := 0
	// Check if the current send packet has available space (offset < BufLen) to accommodate more data
	// without needing to send the packet immediately. If true, fill the buffer with as much user data
	// as possible, update lengths and offsets, and continue processing any remaining data in subsequent iterations.
	if ns.sndDatapkt.Offset < ns.sndDatapkt.BufLen {
		bytesCopied = ns.sndDatapkt.FillBuf(userBuf, offset, len, 0, ns.sAtts.LargeSDU)
		len -= bytesCopied
		offset += bytesCopied
	}
	for len > 0 {
		err := ns.sndDatapkt.Prepare2Send(0, ns.sAtts)
		if err != nil {
			return err
		}
		err = ns.SendPacket(ctx, ns.sndDatapkt.Buf)
		ns.sndDatapkt.Reset()
		if err != nil {
			return err
		}
		if ns.isBreak {
			return nil
		}
		bytesCopied = ns.sndDatapkt.FillBuf(userBuf, offset, len, 0, ns.sAtts.LargeSDU)
		len -= bytesCopied
		offset += bytesCopied
	}
	return nil
}

// Reset resets the connection
func (ns *networkSession) Reset(ctx context.Context) error {
	if ns.resetInProgress {
		return nil
	}
	ns.resetInProgress = true
	defer func() {
		ns.resetInProgress = false
	}()

	var markerPkt = &markerPacket{}
	if ns.breakPosted {
		err := markerPkt.Marshal(nil, ns.sAtts, NIQBMARK)
		if err != nil {
			return err
		}
		err = ns.SendPacket(ctx, markerPkt.Buf)
		if err != nil {
			return err
		}
		ns.breakPosted = false
	}
	err := markerPkt.Marshal(nil, ns.sAtts, NIQRMARK)
	if err != nil {
		return err
	}
	err = ns.SendPacket(ctx, markerPkt.Buf)
	common.Odl.Debug("Reset packet sent")
	if err != nil {
		common.Odl.Error("An error occurred while sending reset", "error", err)
		return err
	}
	for !ns.isReset {
		_, err := ns.recvPacket(ctx)
		if err != nil {
			common.Odl.Error("An error occurred while receiving packet", "error", err)
			return err
		}
		common.Odl.Debug("Packet received", "isReset", ns.isReset)
	}
	//reset sndDatapkt
	ns.sndDatapkt.Reset()
	ns.rcvDatapkt.Offset = NSPDADAT
	ns.rcvDatapkt.Len = ns.rcvDatapkt.Offset
	//set break/reset as false
	ns.isBreak = false
	ns.isReset = false
	common.Odl.Debug("End of break-reset")
	return nil
}

// Disconnect
func (ns *networkSession) Disconnect(ctx context.Context, flags int) error {
	if !ns.connected {
		if ns.ntAdapter != nil {
			if cleaner, ok := ns.ntAdapter.(interface{ Clear() }); ok {
				cleaner.Clear()
			}
			disconnectErr := ns.ntAdapter.Disconnect()
			ns.ntAdapter = nil
			return disconnectErr
		}
		return nil
	}
	ns.connected = false
	var err error
	if flags&driverCommon.NSFIMM == 0 {
		if prepareErr := ns.sndDatapkt.Prepare2Send(NSPDAFEOF, ns.sAtts); prepareErr != nil {
			err = prepareErr
		} else if sendErr := ns.SendPacket(ctx, ns.sndDatapkt.Buf); sendErr != nil {
			err = sendErr
		}
	}
	if cleaner, ok := ns.ntAdapter.(interface{ Clear() }); ok {
		cleaner.Clear()
	}
	disconnectErr := ns.ntAdapter.Disconnect()
	ns.ntAdapter = nil
	if err != nil {
		return err
	}
	return disconnectErr
}

func (ns *networkSession) CheckInbandNotification() bool {
	// Control packet already read
	if ns.controlPkt.Errno != 0 {
		if ns.controlPkt.IsNotification {
			ns.controlPkt.Clear() //reset
			return true
		}
		ns.controlPkt.Clear() //reset
		return false

	}
	// Short timeout for header
	ctxHeader, cancelHeader := context.WithTimeout(common.BackgroundContext, 50*time.Microsecond)
	defer cancelHeader()
	// Read header
	n, err := ns.ntAdapter.Receive(ctxHeader, ns.rcvBuf, PACKET_HEADER_SIZE)
	if err != nil || n != PACKET_HEADER_SIZE {
		return false // Timeout
	}
	// Get packet length
	var packetLen int
	if ns.sAtts.LargeSDU {
		packetLen = int(binary.BigEndian.Uint32(ns.rcvBuf[0:4]))
	} else {
		packetLen = int(binary.BigEndian.Uint16(ns.rcvBuf[0:2]))
	}

	// Longer timeout for body to avoid inconsistent state
	bodyLen := packetLen - PACKET_HEADER_SIZE
	if bodyLen > 0 {
		ctxBody, cancelBody := context.WithTimeout(common.BackgroundContext, 10*time.Second)
		defer cancelBody()
		n, errBody := ns.ntAdapter.Receive(ctxBody, ns.rcvBuf[PACKET_HEADER_SIZE:], bodyLen)
		if errBody != nil || n < bodyLen {
			return false
		}
	}

	// Full packet; unmarshal header
	buf := ns.rcvBuf[:packetLen]
	hdr := &header{}
	if err := hdr.Unmarshal(buf, ns.sAtts, nil); err != nil {
		return false
	}

	if hdr.Type == NSPTCNL {
		ns.controlPkt.Unmarshal(buf, ns.sAtts, hdr)
		if ns.controlPkt.IsNotification {
			ns.controlPkt.Clear() //reset
			return true
		}
		ns.controlPkt.Clear()
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
