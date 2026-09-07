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
	"encoding/binary"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func TestHeaderMarshalUnmarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{largeSDU: false}
	h := &header{
		packetLength:   100,
		packetChecksum: 1234,
		typ:            NSPTDA,
		flags:          1,
		headerChecksum: 5678,
	}
	buf := make([]byte, NSPSIZHD)
	err := h.marshal(buf, sAtts, 0)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}

	var h2 header
	err = h2.unmarshal(buf, sAtts, nil)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if h2.packetLength != 100 || h2.typ != NSPTDA || h2.flags != 0 || h2.headerChecksum != 5678 || h2.packetChecksum != 0 {
		t.Errorf("Unmarshaled header mismatch: got %+v, want %+v", h2, *h)
	}

	// Test large SDU
	sAtts.largeSDU = true
	bufLarge := make([]byte, NSPSIZHD)
	err = h.marshal(bufLarge, sAtts, NSPFLSD)
	if err != nil {
		t.Errorf("Marshal large SDU failed: %v", err)
	}

	var h3 header
	err = h3.unmarshal(bufLarge, sAtts, nil)
	if err != nil {
		t.Errorf("Unmarshal large SDU failed: %v", err)
	}
	if h3.packetLength != 100 || h3.flags != NSPFLSD {
		t.Errorf("Large SDU mismatch: got %+v, want %+v", h3, *h)
	}

	// Short buffer error
	shortBuf := make([]byte, NSPSIZHD-1)
	err = h3.unmarshal(shortBuf, sAtts, nil)
	if err == nil {
		t.Errorf("Expected error for short buffer")
	}
}

func TestConnectPacketMarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{
		version:                     TNS_VERSION_DESIRED,
		sdu:                         512,
		tdu:                         512,
		naFlags:                     NSINANOSERVICES,
		networkCompression:          true,
		networkCompressionLevels:    []string{"high"},
		networkCompressionThreshold: 1024,
	}
	data := []byte("short data")
	cp := &connectPacket{}
	cp.marshal(data, sAtts, 0)
	if cp.overflow {
		t.Errorf("Expected no overflow for short data")
	}
	if binary.BigEndian.Uint16(cp.buf[NSPCNVSN:]) != TNS_VERSION_DESIRED {
		t.Errorf("Version mismatch")
	}
	if binary.BigEndian.Uint16(cp.buf[NSPCNSDU:]) != 512 {
		t.Errorf("SDU mismatch")
	}
	if binary.BigEndian.Uint16(cp.buf[NSPCNOPT:]) != NSGDONTCARE {
		t.Errorf("Options mismatch")
	}
	if !bytes.Contains(cp.buf[NSPCNDAT:], data) {
		t.Errorf("Data not in buffer")
	}
	compressionField := binary.BigEndian.Uint16(cp.buf[NSPCNCFL:])
	expectedCompression := uint16((NSPACCFON << 8) | (NETWORK_COMPRESSION_ZLIB << 10))
	if compressionField != expectedCompression {
		t.Errorf("Compression flags mismatch: got %x, want %x", compressionField, expectedCompression)
	}

	// Long data
	longData := make([]byte, MAX_CDATA_LEN+1)
	cp.marshal(longData, sAtts, 0)
	if !cp.overflow {
		t.Errorf("Expected overflow for long data")
	}
	if len(cp.buf) != NSPCNL {
		t.Errorf("Buffer size mismatch for overflow")
	}

	// No compression
	sAtts.networkCompression = false
	cp.marshal(data, sAtts, 0)
	compressionField = binary.BigEndian.Uint16(cp.buf[NSPCNCFL:])
	if compressionField != 0 {
		t.Errorf("Expected no compression flags")
	}

	// SDU capping
	sAtts.sdu = NSPMXSDULN + 1
	cp.marshal(data, sAtts, 0)
	if binary.BigEndian.Uint16(cp.buf[NSPCNSDU:]) != NSPMXSDULN {
		t.Errorf("SDU not capped")
	}

}

func TestDataPacketMarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{largeSDU: false}
	buf := make([]byte, 100)
	dp := &dataPacket{}
	err := dp.marshal(buf, sAtts, 0)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if dp.offset != NSPDADAT || dp.len != 100 || dp.bufLen != 100 {
		t.Errorf("Marshal state mismatch")
	}
	if dp.hdr.typ != NSPTDA {
		t.Errorf("header type mismatch")
	}
}
func TestDataPacketFillBuf(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{offset: NSPDADAT, bufLen: 20, buf: make([]byte, 20)}
	userBuf := []byte("test")
	copied := dp.FillBuf(userBuf, 0, len(userBuf))
	if copied != 4 || dp.offset != NSPDADAT+4 || string(dp.buf[NSPDADAT:NSPDADAT+4]) != "test" {
		t.Errorf("FillBuf failed")
	}

	// Overflow
	longUserBuf := make([]byte, 20)
	copy(longUserBuf, "testtesttesttesttest") // Ensure it's 20 bytes
	expected := 20 - dp.offset
	copied = dp.FillBuf(longUserBuf, 0, 20)
	if copied != expected || dp.offset != 20 {
		t.Errorf("FillBuf overflow failed")
	}

	// No protocol-specific branch remains; behavior is just bounded copy.
	dp = &dataPacket{offset: NSPDADAT, bufLen: 20, buf: make([]byte, 20)}
	copied = dp.FillBuf(userBuf, 0, len(userBuf))
	if copied != 4 {
		t.Errorf("FillBuf bounded copy failed")
	}
}
func TestDataPacketPrepare2Send(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{largeSDU: false}
	dp := &dataPacket{offset: 20, buf: make([]byte, 20), bufLen: 20}
	dp.hdr = &header{}
	err := dp.Prepare2Send(NSPDAFEOF, sAtts)
	if err != nil {
		t.Errorf("Prepare2Send failed")
	}
	if dp.hdr.packetLength != 20 || binary.BigEndian.Uint16(dp.buf[NSPDAFLG:]) != NSPDAFEOF {
		t.Errorf("Prepare2Send unexpected data")
	}

	// Large SDU
	sAtts.largeSDU = true
	err = dp.Prepare2Send(0, sAtts)
	if err != nil {
		t.Errorf("Prepare2Send failed")
	}
	if binary.BigEndian.Uint32(dp.buf[0:]) != 20 {
		t.Errorf("Large SDU length not set")
	}
}

func TestDataPacketReset(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{offset: 50}
	dp.Reset()
	if dp.offset != NSPDADAT {
		t.Errorf("Reset failed")
	}
}

func TestDataPacketUnmarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{largeSDU: false}
	buf := make([]byte, 50)
	hdr := &header{packetLength: 50, typ: NSPTDA}
	dp := &dataPacket{}
	err := dp.unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if dp.len != 50 || dp.offset != NSPDADAT || dp.hdr != hdr || !bytes.Equal(dp.buf, buf) {
		t.Errorf("Unmarshal state mismatch")
	}

	// Large SDU
	sAtts.largeSDU = true
	err = dp.unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Large SDU unmarshal failed: %v", err)
	}

	for _, packetLen := range []uint32{8, 9} {
		hdr := &header{packetLength: packetLen, typ: NSPTDA}
		err = dp.unmarshal(buf[:packetLen], sAtts, hdr)
		expectOracleErrorCode(t, err, oracleErrors.InvalidNetworkContextExpectedLength)
	}
}

func TestDataPacketReadByte(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{offset: 0, len: 5, buf: []byte{1, 2, 3, 4, 5}}
	b, err := dp.ReadByte()
	if err != nil || b != 1 || dp.offset != 1 {
		t.Errorf("ReadByte failed")
	}

	dp.offset = 5
	_, err = dp.ReadByte()
	if err == nil {
		t.Errorf("Expected EOF")
	}
}

func TestDataPacketRead(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{offset: 0, len: 5, buf: []byte{1, 2, 3, 4, 5}}
	data, err := dp.Read(3)
	if err != nil || !bytes.Equal(data, []byte{1, 2, 3}) || dp.offset != 3 {
		t.Errorf("Read failed")
	}

	_, err = dp.Read(3)
	if err == nil {
		t.Errorf("Expected EOF")
	}
}

func TestDataPacketRemaining(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{offset: 2, len: 5}
	if dp.Remaining() != 3 {
		t.Errorf("Remaining failed")
	}
}

func TestAcceptPacketUnmarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{version: 315, largeSDU: true}
	buf := make([]byte, 100)
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 315)
	binary.BigEndian.PutUint16(buf[NSPACOPT:], 1)
	binary.BigEndian.PutUint32(buf[NSPACLSD:], 8192)
	binary.BigEndian.PutUint32(buf[NSPACLTD:], 8192)
	buf[NSPACCFL] = NSPACCFON | (NETWORK_COMPRESSION_ZLIB << 2)
	binary.BigEndian.PutUint32(buf[NSPACFL2:], TNS_ACCEPT_FLAG_HAS_END_OF_REQUEST|TNS_ACCEPT_FLAG_FAST_AUTH)
	buf[NSPACFL0] = 10
	buf[NSPACFL1] = 20
	hdr := &header{packetLength: 100, typ: NSPTAC}

	ap := &acceptPacket{}
	err := ap.unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if sAtts.version != 315 || sAtts.options != 1 || sAtts.sdu != 8192 || sAtts.tdu != 8192 || !sAtts.largeSDU {
		t.Errorf("Basic fields mismatch")
	}
	if !sAtts.networkCompressionEnabled || sAtts.negotiatedNetworkCompressionScheme != NETWORK_COMPRESSION_ZLIB || !sAtts.firstRecvCompressedPacket || !sAtts.firstSendCompressedPacket {
		t.Errorf("Compression fields mismatch")
	}
	if ap.flag0 != 10 || ap.flag1 != 20 || ap.cflag != buf[NSPACCFL] {
		t.Errorf("Flags mismatch")
	}

	// Older version
	sAtts.version = 314
	sAtts.largeSDU = false
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 314)
	binary.BigEndian.PutUint16(buf[NSPACSDU:], 512)
	binary.BigEndian.PutUint16(buf[NSPACTDU:], 512)
	err = ap.unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal old version failed: %v", err)
	}
	if sAtts.sdu != 512 || sAtts.tdu != 512 {
		t.Errorf("SDU/TDU not updated for old version")
	}

	sAtts.version = 315
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 315)

	// No compression
	buf[NSPACCFL] = 0
	err = ap.unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal no compression failed: %v", err)
	}
	if sAtts.networkCompressionEnabled {
		t.Errorf("Compression enabled unexpectedly")
	}

	// Test version below min data flags
	sAtts.version = TNS_VERSION_MIN_DATA_FLAGS - 1
	err = ap.unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal below min data flags failed: %v", err)
	}

	// Short buffer should error
	err = ap.unmarshal(buf[:NSPACFL1], sAtts, hdr)
	if err == nil {
		t.Errorf("Expected error for short accept packet")
	}
}

func TestAcceptPacketClampsOversizedValues(t *testing.T) {
	t.Parallel()
	const sixteenBitMax = 0xFFFF

	t.Run("16bit fields", func(t *testing.T) {
		buf := make([]byte, 100)
		sAtts := &sessionAtts{}
		binary.BigEndian.PutUint16(buf[NSPACVSN:], 314)
		binary.BigEndian.PutUint16(buf[NSPACOPT:], 1)
		binary.BigEndian.PutUint16(buf[NSPACSDU:], sixteenBitMax)
		binary.BigEndian.PutUint16(buf[NSPACTDU:], sixteenBitMax)
		hdr := &header{packetLength: uint32(len(buf)), typ: NSPTAC}
		ap := &acceptPacket{}
		if err := ap.unmarshal(buf, sAtts, hdr); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if sAtts.sdu != NSPMXSDULN {
			t.Fatalf("expected SDU capped to %d, got %d", NSPMXSDULN, sAtts.sdu)
		}
		if sAtts.tdu != int(sixteenBitMax) {
			t.Fatalf("expected TDU to remain %d, got %d", sixteenBitMax, sAtts.tdu)
		}
	})

	t.Run("32bit fields", func(t *testing.T) {
		buf := make([]byte, 100)
		sAtts := &sessionAtts{}
		binary.BigEndian.PutUint16(buf[NSPACVSN:], 315)
		binary.BigEndian.PutUint16(buf[NSPACOPT:], 1)
		binary.BigEndian.PutUint16(buf[NSPACSDU:], uint16(NSPMXSDULN))
		binary.BigEndian.PutUint16(buf[NSPACTDU:], 0xFFFF)
		binary.BigEndian.PutUint32(buf[NSPACLSD:], uint32(NSPABSSDULN*4))
		binary.BigEndian.PutUint32(buf[NSPACLTD:], uint32(NSPMXTDULN*4))
		hdr := &header{packetLength: uint32(len(buf)), typ: NSPTAC}
		ap := &acceptPacket{}
		if err := ap.unmarshal(buf, sAtts, hdr); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if sAtts.sdu != NSPABSSDULN {
			t.Fatalf("expected SDU capped to %d, got %d", NSPABSSDULN, sAtts.sdu)
		}
		if sAtts.tdu != NSPMXTDULN {
			t.Fatalf("expected TDU capped to %d, got %d", NSPMXTDULN, sAtts.tdu)
		}
	})
}

func TestRefusePacketUnmarshal(t *testing.T) {
	t.Parallel()
	buf := make([]byte, 17)
	buf[NSPRFURS] = 1
	buf[NSPRFSRS] = 2
	binary.BigEndian.PutUint16(buf[NSPRFLEN:], 5)
	copy(buf[NSPRFDAT:], "data!")
	hdr := &header{packetLength: 17, typ: NSPTRF}

	rp := &refusePacket{}
	err := rp.unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if rp.userReason != 1 || rp.systemReason != 2 || rp.dataLen != 5 || rp.dataBuf != "data!" || rp.overflow {
		t.Errorf("Refuse data mismatch")
	}

	// Overflow case
	hdr.packetLength = NSPRFDAT
	err = rp.unmarshal(buf[:NSPRFDAT], nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal overflow failed: %v", err)
	}
	if !rp.overflow {
		t.Errorf("Overflow not set")
	}

	// Short buffer should error
	err = rp.unmarshal(buf[:NSPRFDAT-1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short refuse packet")
	}

}

func TestRedirectPacketUnmarshal(t *testing.T) {
	t.Parallel()
	buf := make([]byte, NSPRDDAT+4)
	binary.BigEndian.PutUint16(buf[NSPRDLEN:], 4)
	copy(buf[NSPRDDAT:], "data")
	hdr := &header{packetLength: uint32(NSPRDDAT + 4), typ: NSPTRD}

	rp := &redirectPacket{}
	err := rp.unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if rp.dataLen != 4 || !bytes.Equal(rp.dataBuf, []byte("data")) || rp.overflow {
		t.Errorf("Redirect data mismatch")
	}

	// Overflow case
	hdr.packetLength = NSPRDDAT
	err = rp.unmarshal(buf[:NSPRDDAT], nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal overflow failed: %v", err)
	}
	if !rp.overflow {
		t.Errorf("Overflow not set")
	}

	// Short buffer should error
	err = rp.unmarshal(buf[:NSPRDDAT-1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short redirect packet")
	}

}

func TestMarkerPacket(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{}
	mp := &markerPacket{}
	err := mp.marshal(nil, sAtts, NIQRMARK)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if mp.markerType != NSPMKTD1 || mp.data != NIQRMARK {
		t.Errorf("Marker data mismatch")
	}

	// Unmarshal
	var hdr header
	hdr.typ = NSPTMK
	hdr.packetLength = uint32(len(mp.buf))
	err = mp.unmarshal(mp.buf, sAtts, &hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if mp.markerType != NSPMKTD1 || mp.data != NIQRMARK {
		t.Errorf("Unmarshaled marker mismatch")
	}

	// Test different marker type
	mp = &markerPacket{}
	buf := make([]byte, NSPMKDAT+1)
	buf[NSPMKTYP] = NSPMKTD0
	hdr.packetLength = uint32(len(buf))
	err = mp.unmarshal(buf, sAtts, &hdr)
	if err != nil {
		t.Errorf("Unmarshal NSPMKTD0 failed: %v", err)
	}
	if mp.markerType != NSPMKTD0 {
		t.Errorf("Marker type mismatch")
	}

	// Test attention marker
	buf[NSPMKTYP] = NSPMKTAT
	err = mp.unmarshal(buf, sAtts, &hdr)
	if err != nil {
		t.Errorf("Unmarshal NSPMKTAT failed: %v", err)
	}
	if mp.markerType != NSPMKTAT {
		t.Errorf("Attention marker type mismatch")
	}

	// Short buffer should error
	err = mp.unmarshal(buf[:NSPMKDAT], sAtts, &hdr)
	if err == nil {
		t.Errorf("Expected error for short marker packet")
	}
}

func TestControlPacketUnmarshal(t *testing.T) {
	t.Parallel()
	buf := make([]byte, 30)
	binary.BigEndian.PutUint16(buf[NSPCTLCMD:], NSPCTL_SERR)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 22)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12573)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+8:], 5)
	copy(buf[NSPCTLDAT+12:], "notif")
	hdr := &header{packetLength: 30, typ: NSPTCNL}

	cp := &controlPacket{}
	err := cp.unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if cp.cmd != NSPCTL_SERR || cp.errno != 12573 || cp.notifLen != 5 || string(cp.notif) != "notif" {
		t.Errorf("Control data mismatch")
	}

	// Test NSECMANSHUT
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12572)
	err = cp.unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal 12572 failed: %v", err)
	}
	if cp.errno != 12572 {
		t.Errorf("Errno mismatch for 12572")
	}

	// Test other error with EMFI
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 22)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12345)
	err = cp.unmarshal(buf, nil, hdr)
	if err == nil || err.(oracleErrors.SQLError).ErrorCode() != string(oracleErrors.ErrConnectionInband) {
		t.Errorf("expected in-band ORA error, got %v", err)
	}

	// Test other error without EMFI
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 0)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12345)
	err = cp.unmarshal(buf, nil, hdr)
	if err == nil || err.(oracleErrors.SQLError).ErrorCode() != string(oracleErrors.ErrConnectionInband) {
		t.Errorf("expected in-band TNS error, got %v", err)
	}

	cp.Clear()
	if cp.errno != 0 || cp.notifLen != 0 || cp.cmd != 0 {
		t.Errorf("Clear failed")
	}

	// Short buffer for data
	err = cp.unmarshal(buf[:NSPCTLDAT+11], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short buffer data")
	}

	// header shorter than command field
	err = cp.unmarshal(buf[:NSPCTLCMD+1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short control packet header")
	}

	// Short buffer for notification
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+8:], 10)
	err = cp.unmarshal(buf, nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short notification buffer")
	}

	// Invalid cmd
	binary.BigEndian.PutUint16(buf[NSPCTLCMD:], 999)
	err = cp.unmarshal(buf, nil, hdr)
	if err == nil {
		t.Errorf("Expected error for invalid cmd")
	}

	// Reset cmd for invalid EMFI test
	binary.BigEndian.PutUint16(buf[NSPCTLCMD:], NSPCTL_SERR)

	// Test invalid EMFI
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 999)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12345)
	err = cp.unmarshal(buf, nil, hdr)
	if err == nil || err.(oracleErrors.SQLError).ErrorCode() != string(oracleErrors.ErrConnectionInband) {
		t.Errorf("expected in-band TNS error, got %v", err)
	}
}

func TestResendPacketUnmarshal(t *testing.T) {
	t.Parallel()
	buf := make([]byte, NSPSIZHD)
	hdr := &header{packetLength: NSPSIZHD, typ: NSPTRS}

	rp := &resendPacket{}
	err := rp.unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}

	// Short buffer
	err = rp.unmarshal(buf[:NSPSIZHD-1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short buffer")
	}

}

func TestContains(t *testing.T) {
	t.Parallel()
	slice := []string{"low", "high"}
	if !contains(slice, "high") {
		t.Errorf("Contains failed for existing item")
	}
	if contains(slice, "medium") {
		t.Errorf("Contains failed for non-existing item")
	}
	if contains([]string{}, "test") {
		t.Errorf("Contains on empty slice failed")
	}
}
