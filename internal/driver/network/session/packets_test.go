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
	"strings"
	"testing"
)

func TestHeaderMarshalUnmarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{LargeSDU: false}
	h := &header{
		PacketLength:   100,
		PacketChecksum: 1234,
		Type:           NSPTDA,
		Flags:          1,
		HeaderChecksum: 5678,
	}
	buf := make([]byte, NSPSIZHD)
	err := h.Marshal(buf, sAtts, 0)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}

	var h2 header
	err = h2.Unmarshal(buf, sAtts, nil)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if h2.PacketLength != 100 || h2.Type != NSPTDA || h2.Flags != 0 || h2.HeaderChecksum != 5678 || h2.PacketChecksum != 0 {
		t.Errorf("Unmarshaled header mismatch: got %+v, want %+v", h2, *h)
	}

	// Test large SDU
	sAtts.LargeSDU = true
	bufLarge := make([]byte, NSPSIZHD)
	err = h.Marshal(bufLarge, sAtts, NSPFLSD)
	if err != nil {
		t.Errorf("Marshal large SDU failed: %v", err)
	}

	var h3 header
	err = h3.Unmarshal(bufLarge, sAtts, nil)
	if err != nil {
		t.Errorf("Unmarshal large SDU failed: %v", err)
	}
	if h3.PacketLength != 100 || h3.Flags != NSPFLSD {
		t.Errorf("Large SDU mismatch: got %+v, want %+v", h3, *h)
	}

	// Short buffer error
	shortBuf := make([]byte, NSPSIZHD-1)
	err = h3.Unmarshal(shortBuf, sAtts, nil)
	if err == nil {
		t.Errorf("Expected error for short buffer")
	}
}

func TestConnectPacketMarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{
		Version:                     TNS_VERSION_DESIRED,
		SDU:                         512,
		TDU:                         512,
		NAFlags:                     NSINANOSERVICES,
		NetworkCompression:          true,
		NetworkCompressionLevels:    []string{"high"},
		NetworkCompressionThreshold: 1024,
	}
	data := []byte("short data")
	cp := &connectPacket{}
	cp.Marshal(data, sAtts, 0)
	if cp.Overflow {
		t.Errorf("Expected no overflow for short data")
	}
	if binary.BigEndian.Uint16(cp.Buf[NSPCNVSN:]) != TNS_VERSION_DESIRED {
		t.Errorf("Version mismatch")
	}
	if binary.BigEndian.Uint16(cp.Buf[NSPCNSDU:]) != 512 {
		t.Errorf("SDU mismatch")
	}
	if binary.BigEndian.Uint16(cp.Buf[NSPCNOPT:]) != NSGDONTCARE {
		t.Errorf("Options mismatch")
	}
	if !bytes.Contains(cp.Buf[NSPCNDAT:], data) {
		t.Errorf("Data not in buffer")
	}
	compressionField := binary.BigEndian.Uint16(cp.Buf[NSPCNCFL:])
	expectedCompression := uint16((NSPACCFON << 8) | (NETWORK_COMPRESSION_ZLIB << 10))
	if compressionField != expectedCompression {
		t.Errorf("Compression flags mismatch: got %x, want %x", compressionField, expectedCompression)
	}

	// Long data
	longData := make([]byte, MAX_CDATA_LEN+1)
	cp.Marshal(longData, sAtts, 0)
	if !cp.Overflow {
		t.Errorf("Expected overflow for long data")
	}
	if len(cp.Buf) != NSPCNL {
		t.Errorf("Buffer size mismatch for overflow")
	}

	// No compression
	sAtts.NetworkCompression = false
	cp.Marshal(data, sAtts, 0)
	compressionField = binary.BigEndian.Uint16(cp.Buf[NSPCNCFL:])
	if compressionField != 0 {
		t.Errorf("Expected no compression flags")
	}

	// SDU capping
	sAtts.SDU = NSPMXSDULN + 1
	cp.Marshal(data, sAtts, 0)
	if binary.BigEndian.Uint16(cp.Buf[NSPCNSDU:]) != NSPMXSDULN {
		t.Errorf("SDU not capped")
	}

}

func TestConnectPacketUnmarshal(t *testing.T) {
	t.Parallel()
	cp := &connectPacket{}
	err := cp.Unmarshal([]byte{}, sessionAtts{}, &header{})
	if err == nil || err.Error() != "not implemented" {
		t.Errorf("Expected 'not implemented' error, got %v", err)
	}
}
func TestDataPacketMarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{LargeSDU: false}
	buf := make([]byte, 100)
	dp := &dataPacket{}
	err := dp.Marshal(buf, sAtts, 0)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if dp.Offset != NSPDADAT || dp.Len != 100 || dp.BufLen != 100 {
		t.Errorf("Marshal state mismatch")
	}
	if dp.hdr.Type != NSPTDA {
		t.Errorf("header type mismatch")
	}
}
func TestDataPacketFillBuf(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{Offset: NSPDADAT, BufLen: 20, Buf: make([]byte, 20)}
	userBuf := []byte("test")
	copied := dp.FillBuf(userBuf, 0, len(userBuf), 0, false)
	if copied != 4 || dp.Offset != NSPDADAT+4 || string(dp.Buf[NSPDADAT:NSPDADAT+4]) != "test" {
		t.Errorf("FillBuf failed")
	}

	// Overflow
	longUserBuf := make([]byte, 20)
	copy(longUserBuf, "testtesttesttesttest") // Ensure it's 20 bytes
	expected := 20 - dp.Offset
	copied = dp.FillBuf(longUserBuf, 0, 20, 0, false)
	if copied != expected || dp.Offset != 20 {
		t.Errorf("FillBuf overflow failed")
	}

	// Large SDU
	dp = &dataPacket{Offset: NSPDADAT, BufLen: 20, Buf: make([]byte, 20)}
	copied = dp.FillBuf(userBuf, 0, len(userBuf), 0, true)
	if copied != 4 {
		t.Errorf("FillBuf large SDU failed")
	}
}
func TestDataPacketPrepare2Send(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{LargeSDU: false}
	dp := &dataPacket{Offset: 20, Buf: make([]byte, 20), BufLen: 20}
	dp.hdr = &header{}
	err := dp.Prepare2Send(NSPDAFEOF, sAtts)
	if err != nil {
		t.Errorf("Prepare2Send failed")
	}
	if dp.hdr.PacketLength != 20 || binary.BigEndian.Uint16(dp.Buf[NSPDAFLG:]) != NSPDAFEOF {
		t.Errorf("Prepare2Send unexpected data")
	}

	// Large SDU
	sAtts.LargeSDU = true
	err = dp.Prepare2Send(0, sAtts)
	if err != nil {
		t.Errorf("Prepare2Send failed")
	}
	if binary.BigEndian.Uint32(dp.Buf[0:]) != 20 {
		t.Errorf("Large SDU length not set")
	}
}

func TestDataPacketReset(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{Offset: 50}
	dp.Reset()
	if dp.Offset != NSPDADAT {
		t.Errorf("Reset failed")
	}
}

func TestDataPacketUnmarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{LargeSDU: false}
	buf := make([]byte, 50)
	hdr := &header{PacketLength: 50, Type: NSPTDA}
	dp := &dataPacket{}
	err := dp.Unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if dp.Len != 50 || dp.Offset != NSPDADAT || dp.hdr != hdr || !bytes.Equal(dp.Buf, buf) {
		t.Errorf("Unmarshal state mismatch")
	}

	// Large SDU
	sAtts.LargeSDU = true
	err = dp.Unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Large SDU unmarshal failed: %v", err)
	}

	for _, packetLen := range []uint32{8, 9} {
		hdr := &header{PacketLength: packetLen, Type: NSPTDA}
		err = dp.Unmarshal(buf[:packetLen], sAtts, hdr)
		if err == nil || !strings.Contains(err.Error(), "data packet too short") {
			t.Errorf("Expected data packet too short error for length %d, got %v", packetLen, err)
		}
	}
}

func TestDataPacketReadByte(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{Offset: 0, Len: 5, Buf: []byte{1, 2, 3, 4, 5}}
	b, err := dp.ReadByte()
	if err != nil || b != 1 || dp.Offset != 1 {
		t.Errorf("ReadByte failed")
	}

	dp.Offset = 5
	_, err = dp.ReadByte()
	if err == nil {
		t.Errorf("Expected EOF")
	}
}

func TestDataPacketRead(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{Offset: 0, Len: 5, Buf: []byte{1, 2, 3, 4, 5}}
	data, err := dp.Read(3)
	if err != nil || !bytes.Equal(data, []byte{1, 2, 3}) || dp.Offset != 3 {
		t.Errorf("Read failed")
	}

	_, err = dp.Read(3)
	if err == nil {
		t.Errorf("Expected EOF")
	}
}

func TestDataPacketRemaining(t *testing.T) {
	t.Parallel()
	dp := &dataPacket{Offset: 2, Len: 5}
	if dp.Remaining() != 3 {
		t.Errorf("Remaining failed")
	}
}

func TestAcceptPacketUnmarshal(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{Version: 315, LargeSDU: true}
	buf := make([]byte, 100)
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 315)
	binary.BigEndian.PutUint16(buf[NSPACOPT:], 1)
	binary.BigEndian.PutUint32(buf[NSPACLSD:], 8192)
	binary.BigEndian.PutUint32(buf[NSPACLTD:], 8192)
	buf[NSPACCFL] = NSPACCFON | (NETWORK_COMPRESSION_ZLIB << 2)
	binary.BigEndian.PutUint32(buf[NSPACFL2:], TNS_ACCEPT_FLAG_HAS_END_OF_REQUEST|TNS_ACCEPT_FLAG_FAST_AUTH)
	buf[NSPACFL0] = 10
	buf[NSPACFL1] = 20
	hdr := &header{PacketLength: 100, Type: NSPTAC}

	ap := &acceptPacket{}
	err := ap.Unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if sAtts.Version != 315 || sAtts.Options != 1 || sAtts.SDU != 8192 || sAtts.TDU != 8192 || !sAtts.LargeSDU {
		t.Errorf("Basic fields mismatch")
	}
	if !sAtts.NetworkCompressionEnabled || sAtts.NegotiatedNetworkCompressionScheme != NETWORK_COMPRESSION_ZLIB || !sAtts.FirstRecvCompressedPacket || !sAtts.FirstSendCompressedPacket {
		t.Errorf("Compression fields mismatch")
	}
	if ap.Flag0 != 10 || ap.Flag1 != 20 || ap.Cflag != buf[NSPACCFL] {
		t.Errorf("Flags mismatch")
	}

	// Older version
	sAtts.Version = 314
	sAtts.LargeSDU = false
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 314)
	binary.BigEndian.PutUint16(buf[NSPACSDU:], 512)
	binary.BigEndian.PutUint16(buf[NSPACTDU:], 512)
	err = ap.Unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal old version failed: %v", err)
	}
	if sAtts.SDU != 512 || sAtts.TDU != 512 {
		t.Errorf("SDU/TDU not updated for old version")
	}

	sAtts.Version = 315
	binary.BigEndian.PutUint16(buf[NSPACVSN:], 315)

	// No compression
	buf[NSPACCFL] = 0
	err = ap.Unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal no compression failed: %v", err)
	}
	if sAtts.NetworkCompressionEnabled {
		t.Errorf("Compression enabled unexpectedly")
	}

	// Test version below min data flags
	sAtts.Version = TNS_VERSION_MIN_DATA_FLAGS - 1
	err = ap.Unmarshal(buf, sAtts, hdr)
	if err != nil {
		t.Errorf("Unmarshal below min data flags failed: %v", err)
	}

	// Test Marshal not implemented
	err = ap.Marshal(buf, sAtts, 0)
	if err == nil {
		t.Errorf("Expected not implemented error for Marshal")
	}

	// Short buffer should error
	err = ap.Unmarshal(buf[:NSPACFL1], sAtts, hdr)
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
		hdr := &header{PacketLength: uint32(len(buf)), Type: NSPTAC}
		ap := &acceptPacket{}
		if err := ap.Unmarshal(buf, sAtts, hdr); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if sAtts.SDU != NSPMXSDULN {
			t.Fatalf("expected SDU capped to %d, got %d", NSPMXSDULN, sAtts.SDU)
		}
		if sAtts.TDU != int(sixteenBitMax) {
			t.Fatalf("expected TDU to remain %d, got %d", sixteenBitMax, sAtts.TDU)
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
		hdr := &header{PacketLength: uint32(len(buf)), Type: NSPTAC}
		ap := &acceptPacket{}
		if err := ap.Unmarshal(buf, sAtts, hdr); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if sAtts.SDU != NSPABSSDULN {
			t.Fatalf("expected SDU capped to %d, got %d", NSPABSSDULN, sAtts.SDU)
		}
		if sAtts.TDU != NSPMXTDULN {
			t.Fatalf("expected TDU capped to %d, got %d", NSPMXTDULN, sAtts.TDU)
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
	hdr := &header{PacketLength: 17, Type: NSPTRF}

	rp := &refusePacket{}
	err := rp.Unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if rp.UserReason != 1 || rp.SystemReason != 2 || rp.DataLen != 5 || rp.DataBuf != "data!" || rp.Overflow {
		t.Errorf("Refuse data mismatch")
	}

	// Overflow case
	hdr.PacketLength = NSPRFDAT
	err = rp.Unmarshal(buf[:NSPRFDAT], nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal overflow failed: %v", err)
	}
	if !rp.Overflow {
		t.Errorf("Overflow not set")
	}

	// Short buffer should error
	err = rp.Unmarshal(buf[:NSPRFDAT-1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short refuse packet")
	}

	// Test Marshal not implemented
	err = rp.Marshal(buf, nil, 0)
	if err == nil {
		t.Errorf("Expected not implemented error for Marshal")
	}
}

func TestRedirectPacketUnmarshal(t *testing.T) {
	t.Parallel()
	buf := make([]byte, NSPRDDAT+4)
	binary.BigEndian.PutUint16(buf[NSPRDLEN:], 4)
	copy(buf[NSPRDDAT:], "data")
	hdr := &header{PacketLength: uint32(NSPRDDAT + 4), Type: NSPTRD}

	rp := &redirectPacket{}
	err := rp.Unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if rp.DataLen != 4 || !bytes.Equal(rp.DataBuf, []byte("data")) || rp.Overflow {
		t.Errorf("Redirect data mismatch")
	}

	// Overflow case
	hdr.PacketLength = NSPRDDAT
	err = rp.Unmarshal(buf[:NSPRDDAT], nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal overflow failed: %v", err)
	}
	if !rp.Overflow {
		t.Errorf("Overflow not set")
	}

	// Short buffer should error
	err = rp.Unmarshal(buf[:NSPRDDAT-1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short redirect packet")
	}

	// Test Marshal not implemented
	err = rp.Marshal(buf, nil, 0)
	if err == nil {
		t.Errorf("Expected not implemented error for Marshal")
	}
}

func TestMarkerPacket(t *testing.T) {
	t.Parallel()
	sAtts := &sessionAtts{}
	mp := &markerPacket{}
	err := mp.Marshal(nil, sAtts, NIQRMARK)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}
	if mp.MarkerType != NSPMKTD1 || mp.Data != NIQRMARK {
		t.Errorf("Marker data mismatch")
	}

	// Unmarshal
	var hdr header
	hdr.Type = NSPTMK
	hdr.PacketLength = uint32(len(mp.Buf))
	err = mp.Unmarshal(mp.Buf, sAtts, &hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if mp.MarkerType != NSPMKTD1 || mp.Data != NIQRMARK {
		t.Errorf("Unmarshaled marker mismatch")
	}

	// Test different marker type
	mp = &markerPacket{}
	buf := make([]byte, NSPMKDAT+1)
	buf[NSPMKTYP] = NSPMKTD0
	hdr.PacketLength = uint32(len(buf))
	err = mp.Unmarshal(buf, sAtts, &hdr)
	if err != nil {
		t.Errorf("Unmarshal NSPMKTD0 failed: %v", err)
	}
	if mp.MarkerType != NSPMKTD0 {
		t.Errorf("Marker type mismatch")
	}

	// Test attention marker
	buf[NSPMKTYP] = NSPMKTAT
	err = mp.Unmarshal(buf, sAtts, &hdr)
	if err != nil {
		t.Errorf("Unmarshal NSPMKTAT failed: %v", err)
	}
	if mp.MarkerType != NSPMKTAT {
		t.Errorf("Attention marker type mismatch")
	}

	// Short buffer should error
	err = mp.Unmarshal(buf[:NSPMKDAT], sAtts, &hdr)
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
	hdr := &header{PacketLength: 30, Type: NSPTCNL}

	cp := &controlPacket{}
	err := cp.Unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}
	if cp.Cmd != NSPCTL_SERR || cp.Errno != 12573 || cp.NotifLen != 5 || string(cp.Notif) != "notif" {
		t.Errorf("Control data mismatch")
	}

	// Test NSECMANSHUT
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12572)
	err = cp.Unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal 12572 failed: %v", err)
	}
	if cp.Errno != 12572 {
		t.Errorf("Errno mismatch for 12572")
	}

	// Test other error with EMFI
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 22)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12345)
	err = cp.Unmarshal(buf, nil, hdr)
	if err == nil || err.Error() != "inband connection error: ORA-12345" {
		t.Errorf("Unexpected error for ORA error: %v", err)
	}

	// Test other error without EMFI
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 0)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12345)
	err = cp.Unmarshal(buf, nil, hdr)
	if err == nil || err.Error() != "inband connection error: TNS-12345" {
		t.Errorf("Unexpected error for TNS error: %v", err)
	}

	cp.Clear()
	if cp.Errno != 0 || cp.NotifLen != 0 || cp.Cmd != 0 {
		t.Errorf("Clear failed")
	}

	// Short buffer for data
	err = cp.Unmarshal(buf[:NSPCTLDAT+11], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short buffer data")
	}

	// header shorter than command field
	err = cp.Unmarshal(buf[:NSPCTLCMD+1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short control packet header")
	}

	// Short buffer for notification
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+8:], 10)
	err = cp.Unmarshal(buf, nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short notification buffer")
	}

	// Invalid cmd
	binary.BigEndian.PutUint16(buf[NSPCTLCMD:], 999)
	err = cp.Unmarshal(buf, nil, hdr)
	if err == nil {
		t.Errorf("Expected error for invalid cmd")
	}

	// Test Marshal not implemented
	err = cp.Marshal(buf, nil, 0)
	if err == nil {
		t.Errorf("Expected not implemented error for Marshal")
	}

	// Reset cmd for invalid EMFI test
	binary.BigEndian.PutUint16(buf[NSPCTLCMD:], NSPCTL_SERR)

	// Test invalid EMFI
	binary.BigEndian.PutUint32(buf[NSPCTLDAT:], 999)
	binary.BigEndian.PutUint32(buf[NSPCTLDAT+4:], 12345)
	err = cp.Unmarshal(buf, nil, hdr)
	if err == nil || err.Error() != "inband connection error: TNS-12345" {
		t.Errorf("Expected inband connection error, got %v", err)
	}
}

func TestResendPacketUnmarshal(t *testing.T) {
	t.Parallel()
	buf := make([]byte, NSPSIZHD)
	hdr := &header{PacketLength: NSPSIZHD, Type: NSPTRS}

	rp := &resendPacket{}
	err := rp.Unmarshal(buf, nil, hdr)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}

	// Short buffer
	err = rp.Unmarshal(buf[:NSPSIZHD-1], nil, hdr)
	if err == nil {
		t.Errorf("Expected error for short buffer")
	}

	// Test Marshal
	err = rp.Marshal(buf, nil, 0)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
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
