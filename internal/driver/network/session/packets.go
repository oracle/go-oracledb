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
	"io"
	"strconv"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	MAX_CDATA_LEN = 230
	NSPCNL        = 74
)

type packetUnmarshaller interface {
	unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error
}

type packetMarshaller interface {
	marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error
}

// header represents a packet header
type header struct {
	packetLength   uint32
	packetChecksum uint16
	typ            int
	flags          byte
	headerChecksum uint16
}

// Marshal serializes the header into a buffer
func (h *header) marshal(buffer []byte, sAtts *sessionAtts, flags uint8) error {
	if sAtts.largeSDU {
		binary.BigEndian.PutUint32(buffer[0:], h.packetLength)
	} else {
		binary.BigEndian.PutUint16(buffer[0:], uint16(h.packetLength))
		binary.BigEndian.PutUint16(buffer[2:], 0) // packet checksum
	}
	buffer[4] = uint8(h.typ)
	buffer[5] = flags
	binary.BigEndian.PutUint16(buffer[6:], h.headerChecksum)
	return nil
}

// Unmarshal deserializes the header from a buffer
func (h *header) unmarshal(buffer []byte, sAtts *sessionAtts, _ *header) error {
	if len(buffer) < 8 {
		return common.NewOracleError(oracleErrors.InvalidNetworkExpectedLength, nil, "packet header", len(buffer), 8)
	}
	if sAtts.largeSDU {
		h.packetLength = binary.BigEndian.Uint32(buffer[0:])
	} else {
		h.packetLength = uint32(binary.BigEndian.Uint16(buffer[0:]))
		h.packetChecksum = binary.BigEndian.Uint16(buffer[2:])
	}
	h.typ = int(buffer[4])
	h.flags = buffer[5]
	h.headerChecksum = binary.BigEndian.Uint16(buffer[6:])
	return nil
}

// connectPacket represents an NSPTCN connect packet
type connectPacket struct {
	hdr            header
	connectData    []byte
	connectDataLen int
	overflow       bool
	buf            []byte
}

// Marshal creates a new connectPacket
func (cp *connectPacket) marshal(connectData []byte, sAtts *sessionAtts, flags uint8) error {
	cp.connectData = connectData
	cp.connectDataLen = len(connectData)

	var size int
	//The Overflow field in connectPacket is a boolean flag that indicates whether the connect data exceeds the maximum
	// allowed length (MAX_CDATA_LEN = 230 bytes) for inclusion in a single connect packet.
	//If the length of ConnectData is greater than 230 bytes, Overflow is set to true, and the packet is created with a
	//fixed size (NSPCNL = 74 bytes) without copying the full data into the buffer. This suggests that the data will overflow
	//and require additional handling, such as sending in subsequent data packets.
	//If the length is 230 bytes or less, Overflow remains false, and the full ConnectData is copied into the packet buffer.
	//This flag helps manage cases where connect data is too large for the initial connect packet structure.
	if cp.connectDataLen <= MAX_CDATA_LEN {
		size = NSPCNL + cp.connectDataLen
	} else {
		size = NSPCNL
		cp.overflow = true
	}
	cp.buf = make([]byte, size)
	cp.hdr = header{
		packetLength: uint32(size),
		typ:          NSPTCN,
		flags:        flags,
	}
	if err := cp.hdr.marshal(cp.buf, sAtts, 0); err != nil {
		return err
	}

	binary.BigEndian.PutUint16(cp.buf[NSPCNVSN:], TNS_VERSION_DESIRED)
	binary.BigEndian.PutUint16(cp.buf[NSPCNLOV:], TNS_VERSION_MINIMUM)

	options := NSGDONTCARE
	binary.BigEndian.PutUint16(cp.buf[NSPCNOPT:], uint16(options))

	sdu := sAtts.sdu
	if sdu > NSPMXSDULN {
		sdu = NSPMXSDULN
	}
	binary.BigEndian.PutUint16(cp.buf[NSPCNSDU:], uint16(sdu))

	tdu := sAtts.tdu
	if tdu > NSPMXSDULN {
		tdu = NSPMXSDULN
	}
	binary.BigEndian.PutUint16(cp.buf[NSPCNTDU:], uint16(tdu))

	binary.BigEndian.PutUint16(cp.buf[NSPCNNTC:], 0) // NT characteristics
	binary.BigEndian.PutUint16(cp.buf[NSPCNTNA:], 0)
	binary.BigEndian.PutUint16(cp.buf[NSPCNONE:], 1)
	binary.BigEndian.PutUint16(cp.buf[NSPCNLEN:], uint16(cp.connectDataLen))
	binary.BigEndian.PutUint16(cp.buf[NSPCNOFF:], NSPCNDAT)
	cp.buf[NSPCNFL0] = uint8(sAtts.naFlags)
	cp.buf[NSPCNFL1] = uint8(sAtts.naFlags)
	binary.BigEndian.PutUint16(cp.buf[NSPCNTMO:], 0)
	binary.BigEndian.PutUint16(cp.buf[NSPCNTCK:], 0)
	binary.BigEndian.PutUint16(cp.buf[NSPCNADL:], 0)
	binary.BigEndian.PutUint16(cp.buf[NSPCNAOF:], 0)
	binary.BigEndian.PutUint32(cp.buf[NSPCNLSD:], uint32(sAtts.sdu))
	binary.BigEndian.PutUint32(cp.buf[NSPCNLTD:], uint32(sAtts.tdu))

	var compressionField uint16
	if sAtts.networkCompression {
		compressionField = NSPACCFON << 8
		if contains(sAtts.networkCompressionLevels, "high") {
			compressionField |= NETWORK_COMPRESSION_ZLIB << 10
		}
	}
	binary.BigEndian.PutUint16(cp.buf[NSPCNCFL:], compressionField)
	binary.BigEndian.PutUint32(cp.buf[NSPCNCFL2:], 0)

	if !cp.overflow && cp.connectDataLen > 0 {
		copy(cp.buf[NSPCNDAT:], connectData)
	}
	return nil
}

// dataPacket represents an NSPTDA data packet
type dataPacket struct {
	hdr    *header
	offset int
	len    int
	bufLen int
	buf    []byte
}

// Remaining calculates and returns the number of bytes left to read in the current data packet
func (dp *dataPacket) Remaining() int {
	return dp.len - dp.offset
}

func (dp *dataPacket) ReadByte() (byte, error) {
	if dp.offset < dp.len {
		retByte := dp.buf[dp.offset]
		dp.offset++
		return retByte, nil
	}
	common.Odl.Debug("dataPacket.ReadByte EOF")
	return 0, io.EOF
}

func (dp *dataPacket) Read(numBytes int) ([]byte, error) {
	if (dp.offset + numBytes) <= dp.len {
		retBytes := dp.buf[dp.offset : dp.offset+numBytes]
		dp.offset += numBytes
		return retBytes, nil
	}
	return nil, io.EOF
}

// MarshalPacket initializes the data packet
func (dp *dataPacket) marshal(buf []byte, _ *sessionAtts, _ uint8) error {
	dp.buf = buf
	dp.bufLen = len(buf)
	dp.len = dp.bufLen
	dp.hdr = &header{
		typ:   NSPTDA,
		flags: 0,
	}
	dp.offset = NSPDADAT
	return nil
}

// FillBuf populates the data packet
func (dp *dataPacket) FillBuf(userBuf []byte, offset, len int) int {
	bytes2Copy := len
	//limit the bytes to copy to the remaining buffer space to avoid overflow;
	if len > dp.bufLen-dp.offset {
		bytes2Copy = dp.bufLen - dp.offset
	}
	if bytes2Copy > 0 {
		copy(dp.buf[dp.offset:], userBuf[offset:offset+bytes2Copy])
	}
	dp.offset += bytes2Copy
	return bytes2Copy
}

// Prepare2Send prepares the data packet for sending
func (dp *dataPacket) Prepare2Send(flags uint16, sAtts *sessionAtts) error {
	dp.hdr.packetLength = uint32(dp.offset)
	if err := dp.hdr.marshal(dp.buf, sAtts, 0); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(dp.buf[NSPDAFLG:], flags)
	return nil
}

// Reset the send Packet
func (dp *dataPacket) Reset() {
	dp.offset = NSPDADAT
}

// Unmarshal constructs a data packet from received data
func (dp *dataPacket) unmarshal(buffer []byte, _ *sessionAtts, hdr *header) error {
	if int(hdr.packetLength) < NSPDADAT {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTDA", hdr.packetLength, NSPDADAT)
	}
	dp.hdr = hdr
	dp.buf = buffer
	dp.offset = NSPDADAT
	dp.len = int(hdr.packetLength)
	return nil
}

// acceptPacket represents an NSPTAC accept packet
type acceptPacket struct {
	hdr   *header
	buf   []byte
	len   int
	cflag uint8
	flag0 uint8
	flag1 uint8
}

// Unmarshal creates an acceptPacket from a buffer
func (ap *acceptPacket) unmarshal(buffer []byte, sAtts *sessionAtts, hdr *header) error {
	const (
		minAcceptBaseLen     = NSPACFL1 + 1
		minAcceptLargeSDULen = NSPACLTD + 4
		minAcceptCflagLen    = NSPACCFL + 1
	)

	if len(buffer) < minAcceptBaseLen {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTAC", len(buffer), minAcceptBaseLen)
	}
	ap.hdr = hdr
	ap.buf = buffer
	ap.len = len(buffer)
	sAtts.version = int(binary.BigEndian.Uint16(buffer[NSPACVSN:]))
	sAtts.options = int(binary.BigEndian.Uint16(buffer[NSPACOPT:]))
	sdu := int(binary.BigEndian.Uint16(buffer[NSPACSDU:]))
	tdu := int(binary.BigEndian.Uint16(buffer[NSPACTDU:]))
	sdu = clamp(sdu, NSPMNSDULN, NSPMXSDULN)
	tdu = clamp(tdu, NSPMNTDULN, NSPMXTDULN)

	if sAtts.version >= 315 {
		if len(buffer) < minAcceptLargeSDULen {
			return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTAC", len(buffer), minAcceptLargeSDULen)
		}
		sdu = int(binary.BigEndian.Uint32(buffer[NSPACLSD:]))
		tdu = int(binary.BigEndian.Uint32(buffer[NSPACLTD:]))
		// ensure the listener-supplied SDU/TDU values never drop below the protocol minimums
		// or exceed the driver’s safe maxima before we store them or size any buffers.
		sdu = clamp(sdu, NSPMNSDULN, NSPABSSDULN)
		tdu = clamp(tdu, NSPMNTDULN, NSPMXTDULN)
		sAtts.largeSDU = true
		if len(buffer) < minAcceptCflagLen {
			return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTAC", len(buffer), minAcceptCflagLen)
		}
		ap.cflag = buffer[NSPACCFL]
		if ap.cflag&NSPACCFON != 0 {
			sAtts.negotiatedNetworkCompressionScheme = int((ap.cflag & 0x3c) >> 2)
			sAtts.networkCompressionEnabled = true
			sAtts.firstRecvCompressedPacket = true
			sAtts.firstSendCompressedPacket = true
		} else {
			sAtts.networkCompressionEnabled = false
		}
	}
	sAtts.sdu = sdu
	sAtts.tdu = tdu
	ap.flag0 = buffer[NSPACFL0]
	ap.flag1 = buffer[NSPACFL1]
	return nil
}

// refusePacket represents an NSPTRF refuse packet
type refusePacket struct {
	hdr          *header
	buf          []byte
	userReason   uint8
	systemReason uint8
	dataLen      int
	dataOff      int
	dataBuf      string
	overflow     bool
}

// Unmarshal creates a refusePacket from a buffer
func (rp *refusePacket) unmarshal(buffer []byte, _ *sessionAtts, hdr *header) error {
	const minRefuseLen = NSPRFDAT
	if len(buffer) < minRefuseLen {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTRF", len(buffer), minRefuseLen)
	}
	rp.hdr = hdr
	rp.buf = buffer
	rp.userReason = buffer[NSPRFURS]
	rp.systemReason = buffer[NSPRFSRS]
	rp.dataLen = int(binary.BigEndian.Uint16(buffer[NSPRFLEN:]))
	rp.dataOff = NSPRFDAT
	if int(rp.hdr.packetLength) > rp.dataOff {
		rp.dataBuf = string(buffer[rp.dataOff:rp.hdr.packetLength])
		rp.overflow = false
	} else {
		rp.overflow = true
	}
	return nil
}

// redirectPacket represents an NSPTRD redirect packet
type redirectPacket struct {
	hdr      *header
	buf      []byte
	dataLen  int
	dataOff  int
	dataBuf  []byte
	overflow bool
}

// Unmarshal creates a redirectPacket from a buffer
func (rp *redirectPacket) unmarshal(buffer []byte, _ *sessionAtts, hdr *header) error {
	const minRedirectLen = NSPRDDAT
	if len(buffer) < minRedirectLen {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTRD", len(buffer), minRedirectLen)
	}
	rp.hdr = hdr
	rp.buf = buffer
	rp.dataLen = int(binary.BigEndian.Uint16(buffer[NSPRDLEN:]))
	rp.dataOff = NSPRDDAT
	if int(rp.hdr.packetLength) > rp.dataOff {
		rp.dataBuf = buffer[rp.dataOff:rp.hdr.packetLength]
		rp.overflow = false
	} else {
		rp.overflow = true
	}
	return nil
}

// markerPacket represents an NSPTMK marker packet
type markerPacket struct {
	hdr        *header
	buf        []byte
	markerType uint8
	markerODT  uint8
	data       uint8
}

// MarshalPrepare prepares the marker packet for sending
func (mp *markerPacket) marshal(_ []byte, sAtts *sessionAtts, data uint8) error {
	mp.buf = make([]byte, NSPMKDAT+1)
	mp.hdr = &header{
		packetLength: uint32(NSPMKDAT + 1),
		typ:          NSPTMK,
		flags:        0,
	}
	if err := mp.hdr.marshal(mp.buf, sAtts, 0); err != nil {

		return err
	}
	mp.buf[NSPMKTYP] = NSPMKTD1
	mp.buf[NSPMKDAT] = data
	mp.markerType = NSPMKTD1
	mp.data = data
	return nil
}

// Unmarshal constructs a marker packet from received data
func (mp *markerPacket) unmarshal(buffer []byte, _ *sessionAtts, hdr *header) error {
	const minMarkerLen = NSPMKDAT + 1
	if len(buffer) < minMarkerLen {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTMK", len(buffer), minMarkerLen)
	}
	mp.hdr = hdr
	mp.buf = buffer
	mp.markerType = buffer[NSPMKTYP]
	mp.data = buffer[NSPMKDAT]
	return nil
}

// controlPacket represents an NSPTCTL control packet
type controlPacket struct {
	hdr            *header
	buf            []byte
	errno          uint32
	notif          []byte
	notifLen       uint32
	cmd            uint16
	isNotification bool
}

// Clear resets the control packet
func (cp *controlPacket) Clear() {
	cp.errno = 0
	cp.notif = nil
	cp.notifLen = 0
	cp.cmd = 0
	cp.isNotification = false
}

// Unmarshal constructs a control packet from received data
func (cp *controlPacket) unmarshal(buffer []byte, _ *sessionAtts, hdr *header) error {
	const (
		NSECMANSHUT           = 12572
		NSESENDMESG           = 12573
		ORA_ERROR_EMFI_NUMBER = 22
	)
	if len(buffer) < NSPCTLCMD+2 {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTCNL", len(buffer), NSPCTLCMD+2)
	}
	cp.hdr = hdr
	cp.cmd = binary.BigEndian.Uint16(buffer[NSPCTLCMD:])
	switch cp.cmd {
	case NSPCTL_SERR:
		if len(buffer) < NSPCTLDAT+12 {
			return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTCNL", len(buffer), NSPCTLDAT+12)
		}
		emfi := binary.BigEndian.Uint32(buffer[NSPCTLDAT:])
		err1 := binary.BigEndian.Uint32(buffer[NSPCTLDAT+4:])
		err2 := binary.BigEndian.Uint32(buffer[NSPCTLDAT+8:])
		if err1 == NSECMANSHUT {
			cp.errno = err1
			cp.isNotification = true
		} else if err1 == NSESENDMESG {
			cp.isNotification = true
			cp.errno = err1
			cp.notifLen = err2
			if len(buffer) < NSPCTLDAT+12+int(cp.notifLen) {
				return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTCNL", len(buffer), NSPCTLDAT+12+int(cp.notifLen))
			}
			cp.notif = make([]byte, cp.notifLen)
			copy(cp.notif, buffer[NSPCTLDAT+12:NSPCTLDAT+12+int(cp.notifLen)])
		} else {
			cp.errno = err1
			cp.isNotification = false
			if emfi == ORA_ERROR_EMFI_NUMBER {
				return common.NewOracleError(oracleErrors.ErrConnectionInband,
					common.NewOracleError(oracleErrors.NetworkServerErrorCode, nil, "ORA", strconv.FormatInt(int64(err1), 10)))
			} else {
				return common.NewOracleError(oracleErrors.ErrConnectionInband,
					common.NewOracleError(oracleErrors.NetworkServerErrorCode, nil, "TNS", strconv.FormatInt(int64(err1), 10)))
			}
		}
	default:
		return common.NewOracleError(oracleErrors.InvalidNetworkValue, nil, "control command", cp.cmd)
	}
	return nil
}

// resendPacket represents an NSPTRS resend packet
type resendPacket struct {
	hdr *header
	buf []byte
}

// Unmarshal constructs a resend packet from received data
func (rp *resendPacket) unmarshal(buffer []byte, _ *sessionAtts, hdr *header) error {
	if len(buffer) < NSPSIZHD {
		return common.NewOracleError(oracleErrors.InvalidNetworkContextExpectedLength, nil, "packet", "NSPTRS", len(buffer), NSPSIZHD)
	}
	rp.hdr = hdr
	rp.buf = buffer
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
