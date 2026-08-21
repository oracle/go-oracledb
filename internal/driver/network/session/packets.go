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
	"encoding/binary"
	"fmt"
	"io"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

const (
	MAX_CDATA_LEN = 230
	NSPCNL        = 74
)

type packet interface {
	Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error
	Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error
}

// header represents a packet header
type header struct {
	PacketLength   uint32
	PacketChecksum uint16
	Type           int
	Flags          byte
	HeaderChecksum uint16
}

// Marshal serializes the header into a buffer
func (h *header) Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	if sAtts.LargeSDU {
		binary.BigEndian.PutUint32(buffer[0:], h.PacketLength)
	} else {
		binary.BigEndian.PutUint16(buffer[0:], uint16(h.PacketLength))
		binary.BigEndian.PutUint16(buffer[2:], 0) // packet checksum
	}
	buffer[4] = uint8(h.Type)
	buffer[5] = flags
	binary.BigEndian.PutUint16(buffer[6:], h.HeaderChecksum)
	return nil
}

// Unmarshal deserializes the header from a buffer
func (h *header) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	if len(buffer) < 8 {
		return fmt.Errorf("buffer too short for header")
	}
	if sAtts.LargeSDU {
		h.PacketLength = binary.BigEndian.Uint32(buffer[0:])
	} else {
		h.PacketLength = uint32(binary.BigEndian.Uint16(buffer[0:]))
		h.PacketChecksum = binary.BigEndian.Uint16(buffer[2:])
	}
	h.Type = int(buffer[4])
	h.Flags = buffer[5]
	h.HeaderChecksum = binary.BigEndian.Uint16(buffer[6:])
	return nil
}

// connectPacket represents an NSPTCN connect packet
type connectPacket struct {
	hdr            header
	ConnectData    []byte
	ConnectDataLen int
	Overflow       bool
	Buf            []byte
}

// Marshal creates a new connectPacket
func (cp *connectPacket) Marshal(connectData []byte, sAtts *sessionAtts, flags uint8) error {
	cp.ConnectData = connectData
	cp.ConnectDataLen = len(connectData)

	var size int
	//The Overflow field in connectPacket is a boolean flag that indicates whether the connect data exceeds the maximum
	// allowed length (MAX_CDATA_LEN = 230 bytes) for inclusion in a single connect packet.
	//If the length of ConnectData is greater than 230 bytes, Overflow is set to true, and the packet is created with a
	//fixed size (NSPCNL = 74 bytes) without copying the full data into the buffer. This suggests that the data will overflow
	//and require additional handling, such as sending in subsequent data packets.
	//If the length is 230 bytes or less, Overflow remains false, and the full ConnectData is copied into the packet buffer.
	//This flag helps manage cases where connect data is too large for the initial connect packet structure.
	if cp.ConnectDataLen <= MAX_CDATA_LEN {
		size = NSPCNL + cp.ConnectDataLen
	} else {
		size = NSPCNL
		cp.Overflow = true
	}
	cp.Buf = make([]byte, size)
	cp.hdr = header{
		PacketLength: uint32(size),
		Type:         NSPTCN,
		Flags:        flags,
	}
	if err := cp.hdr.Marshal(cp.Buf, sAtts, 0); err != nil {
		return err
	}

	binary.BigEndian.PutUint16(cp.Buf[NSPCNVSN:], TNS_VERSION_DESIRED)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNLOV:], TNS_VERSION_MINIMUM)

	options := NSGDONTCARE
	binary.BigEndian.PutUint16(cp.Buf[NSPCNOPT:], uint16(options))

	sdu := sAtts.SDU
	if sdu > NSPMXSDULN {
		sdu = NSPMXSDULN
	}
	binary.BigEndian.PutUint16(cp.Buf[NSPCNSDU:], uint16(sdu))

	tdu := sAtts.TDU
	if tdu > NSPMXSDULN {
		tdu = NSPMXSDULN
	}
	binary.BigEndian.PutUint16(cp.Buf[NSPCNTDU:], uint16(tdu))

	binary.BigEndian.PutUint16(cp.Buf[NSPCNNTC:], 0) // NT characteristics
	binary.BigEndian.PutUint16(cp.Buf[NSPCNTNA:], 0)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNONE:], 1)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNLEN:], uint16(cp.ConnectDataLen))
	binary.BigEndian.PutUint16(cp.Buf[NSPCNOFF:], NSPCNDAT)
	cp.Buf[NSPCNFL0] = uint8(sAtts.NAFlags)
	cp.Buf[NSPCNFL1] = uint8(sAtts.NAFlags)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNTMO:], 0)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNTCK:], 0)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNADL:], 0)
	binary.BigEndian.PutUint16(cp.Buf[NSPCNAOF:], 0)
	binary.BigEndian.PutUint32(cp.Buf[NSPCNLSD:], uint32(sAtts.SDU))
	binary.BigEndian.PutUint32(cp.Buf[NSPCNLTD:], uint32(sAtts.TDU))

	var compressionField uint16
	if sAtts.NetworkCompression {
		compressionField = NSPACCFON << 8
		if contains(sAtts.NetworkCompressionLevels, "high") {
			compressionField |= NETWORK_COMPRESSION_ZLIB << 10
		}
	}
	binary.BigEndian.PutUint16(cp.Buf[NSPCNCFL:], compressionField)
	binary.BigEndian.PutUint32(cp.Buf[NSPCNCFL2:], 0)

	if !cp.Overflow && cp.ConnectDataLen > 0 {
		copy(cp.Buf[NSPCNDAT:], connectData)
	}
	return nil
}

func (cp *connectPacket) Unmarshal(buffer []byte, sAtts sessionAtts, hdr *header) error {
	return fmt.Errorf("not implemented")
}

// dataPacket represents an NSPTDA data packet
type dataPacket struct {
	hdr    *header
	Offset int
	Len    int
	BufLen int
	Buf    []byte
}

// Remaining calculates and returns the number of bytes left to read in the current data packet
func (dp *dataPacket) Remaining() int {
	return dp.Len - dp.Offset
}

func (dp *dataPacket) ReadByte() (byte, error) {
	if dp.Offset < dp.Len {
		retByte := dp.Buf[dp.Offset]
		dp.Offset++
		return retByte, nil
	}
	common.Odl.Debug("dataPacket.ReadByte EOF")
	return 0, io.EOF
}

func (dp *dataPacket) Read(numBytes int) ([]byte, error) {
	if (dp.Offset + numBytes) <= dp.Len {
		retBytes := dp.Buf[dp.Offset : dp.Offset+numBytes]
		dp.Offset += numBytes
		return retBytes, nil
	}
	return nil, io.EOF
}

// MarshalPacket initializes the data packet
func (dp *dataPacket) Marshal(buf []byte, sAtts *sessionAtts, flags uint8) error {
	dp.Buf = buf
	dp.BufLen = len(buf)
	dp.Len = dp.BufLen
	dp.hdr = &header{
		Type:  NSPTDA,
		Flags: 0,
	}
	dp.Offset = NSPDADAT
	return nil
}

// FillBuf populates the data packet
func (dp *dataPacket) FillBuf(userBuf []byte, offset, len int, flags uint16, isLargeSDU bool) int {
	bytes2Copy := len
	//limit the bytes to copy to the remaining buffer space to avoid overflow;
	if len > dp.BufLen-dp.Offset {
		bytes2Copy = dp.BufLen - dp.Offset
	}
	if bytes2Copy > 0 {
		copy(dp.Buf[dp.Offset:], userBuf[offset:offset+bytes2Copy])
	}
	dp.Offset += bytes2Copy
	return bytes2Copy
}

// Prepare2Send prepares the data packet for sending
func (dp *dataPacket) Prepare2Send(flags uint16, sAtts *sessionAtts) error {
	dp.hdr.PacketLength = uint32(dp.Offset)
	if err := dp.hdr.Marshal(dp.Buf, sAtts, 0); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(dp.Buf[NSPDAFLG:], flags)
	return nil
}

// Reset the send Packet
func (dp *dataPacket) Reset() {
	dp.Offset = NSPDADAT
}

// Unmarshal constructs a data packet from received data
func (dp *dataPacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	if int(hdr.PacketLength) < NSPDADAT {
		return fmt.Errorf("data packet too short: got %d, need >= %d", hdr.PacketLength, NSPDADAT)
	}
	dp.hdr = hdr
	dp.Buf = buffer
	dp.Offset = NSPDADAT
	dp.Len = int(hdr.PacketLength)
	return nil
}

// acceptPacket represents an NSPTAC accept packet
type acceptPacket struct {
	hdr   *header
	Buf   []byte
	Len   int
	Cflag uint8
	Flag0 uint8
	Flag1 uint8
}

// Unmarshal creates an acceptPacket from a buffer
func (ap *acceptPacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	const (
		minAcceptBaseLen     = NSPACFL1 + 1
		minAcceptLargeSDULen = NSPACLTD + 4
		minAcceptCflagLen    = NSPACCFL + 1
	)

	if len(buffer) < minAcceptBaseLen {
		return fmt.Errorf("accept packet too short: got %d, need >= %d", len(buffer), minAcceptBaseLen)
	}
	ap.hdr = hdr
	ap.Buf = buffer
	ap.Len = len(buffer)
	sAtts.Version = int(binary.BigEndian.Uint16(buffer[NSPACVSN:]))
	sAtts.Options = int(binary.BigEndian.Uint16(buffer[NSPACOPT:]))
	sdu := int(binary.BigEndian.Uint16(buffer[NSPACSDU:]))
	tdu := int(binary.BigEndian.Uint16(buffer[NSPACTDU:]))
	sdu = clamp(sdu, NSPMNSDULN, NSPMXSDULN)
	tdu = clamp(tdu, NSPMNTDULN, NSPMXTDULN)

	if sAtts.Version >= 315 {
		if len(buffer) < minAcceptLargeSDULen {
			return fmt.Errorf("accept packet too short for large SDU/TDU: got %d, need >= %d", len(buffer), minAcceptLargeSDULen)
		}
		sdu = int(binary.BigEndian.Uint32(buffer[NSPACLSD:]))
		tdu = int(binary.BigEndian.Uint32(buffer[NSPACLTD:]))
		// ensure the listener-supplied SDU/TDU values never drop below the protocol minimums
		// or exceed the driver’s safe maxima before we store them or size any buffers.
		sdu = clamp(sdu, NSPMNSDULN, NSPABSSDULN)
		tdu = clamp(tdu, NSPMNTDULN, NSPMXTDULN)
		sAtts.LargeSDU = true
		if len(buffer) < minAcceptCflagLen {
			return fmt.Errorf("accept packet too short for compression flag: got %d, need >= %d", len(buffer), minAcceptCflagLen)
		}
		ap.Cflag = buffer[NSPACCFL]
		if ap.Cflag&NSPACCFON != 0 {
			sAtts.NegotiatedNetworkCompressionScheme = int((ap.Cflag & 0x3c) >> 2)
			sAtts.NetworkCompressionEnabled = true
			sAtts.FirstRecvCompressedPacket = true
			sAtts.FirstSendCompressedPacket = true
		} else {
			sAtts.NetworkCompressionEnabled = false
		}
	}
	sAtts.SDU = sdu
	sAtts.TDU = tdu
	ap.Flag0 = buffer[NSPACFL0]
	ap.Flag1 = buffer[NSPACFL1]
	return nil
}

func (ap *acceptPacket) Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	return fmt.Errorf("not implemented")
}

// refusePacket represents an NSPTRF refuse packet
type refusePacket struct {
	hdr          *header
	Buf          []byte
	UserReason   uint8
	SystemReason uint8
	DataLen      int
	DataOff      int
	DataBuf      string
	Overflow     bool
}

// Unmarshal creates a refusePacket from a buffer
func (rp *refusePacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	const minRefuseLen = NSPRFDAT
	if len(buffer) < minRefuseLen {
		return fmt.Errorf("refuse packet too short: got %d, need >= %d", len(buffer), minRefuseLen)
	}
	rp.hdr = hdr
	rp.Buf = buffer
	rp.UserReason = buffer[NSPRFURS]
	rp.SystemReason = buffer[NSPRFSRS]
	rp.DataLen = int(binary.BigEndian.Uint16(buffer[NSPRFLEN:]))
	rp.DataOff = NSPRFDAT
	if int(rp.hdr.PacketLength) > rp.DataOff {
		rp.DataBuf = string(buffer[rp.DataOff:rp.hdr.PacketLength])
		rp.Overflow = false
	} else {
		rp.Overflow = true
	}
	return nil
}

func (ap *refusePacket) Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	return fmt.Errorf("not implemented")
}

// redirectPacket represents an NSPTRD redirect packet
type redirectPacket struct {
	hdr      *header
	Buf      []byte
	DataLen  int
	DataOff  int
	DataBuf  []byte
	Overflow bool
}

// Unmarshal creates a redirectPacket from a buffer
func (rp *redirectPacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	const minRedirectLen = NSPRDDAT
	if len(buffer) < minRedirectLen {
		return fmt.Errorf("redirect packet too short: got %d, need >= %d", len(buffer), minRedirectLen)
	}
	rp.hdr = hdr
	rp.Buf = buffer
	rp.DataLen = int(binary.BigEndian.Uint16(buffer[NSPRDLEN:]))
	rp.DataOff = NSPRDDAT
	if int(rp.hdr.PacketLength) > rp.DataOff {
		rp.DataBuf = buffer[rp.DataOff:rp.hdr.PacketLength]
		rp.Overflow = false
	} else {
		rp.Overflow = true
	}
	return nil
}

func (ap *redirectPacket) Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	return fmt.Errorf("not implemented")
}

// markerPacket represents an NSPTMK marker packet
type markerPacket struct {
	hdr        *header
	Buf        []byte
	MarkerType uint8
	MarkerODT  uint8
	Data       uint8
}

// MarshalPrepare prepares the marker packet for sending
func (mp *markerPacket) Marshal(buf []byte, sAtts *sessionAtts, data uint8) error {
	mp.Buf = make([]byte, NSPMKDAT+1)
	mp.hdr = &header{
		PacketLength: uint32(NSPMKDAT + 1),
		Type:         NSPTMK,
		Flags:        0,
	}
	if err := mp.hdr.Marshal(mp.Buf, sAtts, 0); err != nil {

		return err
	}
	mp.Buf[NSPMKTYP] = NSPMKTD1
	mp.Buf[NSPMKDAT] = data
	mp.MarkerType = NSPMKTD1
	mp.Data = data
	return nil
}

// Unmarshal constructs a marker packet from received data
func (mp *markerPacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	const minMarkerLen = NSPMKDAT + 1
	if len(buffer) < minMarkerLen {
		return fmt.Errorf("marker packet too short: got %d, need >= %d", len(buffer), minMarkerLen)
	}
	mp.hdr = hdr
	mp.Buf = buffer
	mp.MarkerType = buffer[NSPMKTYP]
	mp.Data = buffer[NSPMKDAT]
	return nil
}

// controlPacket represents an NSPTCTL control packet
type controlPacket struct {
	hdr            *header
	Buf            []byte
	Errno          uint32
	Notif          []byte
	NotifLen       uint32
	Cmd            uint16
	IsNotification bool
}

// Marshal creates a new controlPacket
func (ap *controlPacket) Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	return fmt.Errorf("not implemented")
}

// Clear resets the control packet
func (cp *controlPacket) Clear() {
	cp.Errno = 0
	cp.Notif = nil
	cp.NotifLen = 0
	cp.Cmd = 0
	cp.IsNotification = false
}

// Unmarshal constructs a control packet from received data
func (cp *controlPacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	const (
		NSECMANSHUT           = 12572
		NSESENDMESG           = 12573
		ORA_ERROR_EMFI_NUMBER = 22
	)
	if len(buffer) < NSPCTLCMD+2 {
		return fmt.Errorf("control packet too short: got %d, need >= %d", len(buffer), NSPCTLCMD+2)
	}
	cp.hdr = hdr
	cp.Cmd = binary.BigEndian.Uint16(buffer[NSPCTLCMD:])
	switch cp.Cmd {
	case NSPCTL_SERR:
		if len(buffer) < NSPCTLDAT+12 {
			return fmt.Errorf("buffer too short for control packet data")
		}
		emfi := binary.BigEndian.Uint32(buffer[NSPCTLDAT:])
		err1 := binary.BigEndian.Uint32(buffer[NSPCTLDAT+4:])
		err2 := binary.BigEndian.Uint32(buffer[NSPCTLDAT+8:])
		if err1 == NSECMANSHUT {
			cp.Errno = err1
			cp.IsNotification = true
		} else if err1 == NSESENDMESG {
			cp.IsNotification = true
			cp.Errno = err1
			cp.NotifLen = err2
			if len(buffer) < NSPCTLDAT+12+int(cp.NotifLen) {
				return fmt.Errorf("buffer too short for notification data")
			}
			cp.Notif = make([]byte, cp.NotifLen)
			copy(cp.Notif, buffer[NSPCTLDAT+12:NSPCTLDAT+12+int(cp.NotifLen)])
		} else {
			cp.Errno = err1
			cp.IsNotification = false
			if emfi == ORA_ERROR_EMFI_NUMBER {
				return fmt.Errorf("%w: ORA-%d", ErrConnectionInband, err1)
			} else {
				return fmt.Errorf("%w: TNS-%d", ErrConnectionInband, err1)
			}
		}
	default:
		return ErrInvalidPacket
	}
	return nil
}

// resendPacket represents an NSPTRS resend packet
type resendPacket struct {
	hdr *header
	Buf []byte
}

// Marshal prepares the resend packet for sending
func (rp *resendPacket) Marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	return nil
}

// Unmarshal constructs a resend packet from received data
func (rp *resendPacket) Unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	if len(buffer) < NSPSIZHD {
		return fmt.Errorf("buffer too short for resend packet")
	}
	rp.hdr = hdr
	rp.Buf = buffer
	// No additional data to unmarshal for resend packet
	return nil
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Helper that takes a value plus lower/upper bounds and forces the result into that range:
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
