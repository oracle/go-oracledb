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
	"errors"
	"testing"
)

func TestPrepareReadBuffer(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT, buf: ns.rcvBuf} // no data

	// Test with no data, triggers recv
	dataPkt := make([]byte, 20)
	binary.BigEndian.PutUint16(dataPkt[0:2], 20)
	dataPkt[4] = NSPTDA
	copy(dataPkt[NSPDADAT:], []byte("testdata"))
	mock.receivedData = dataPkt
	mock.recvPos = 0

	err := ns.PrepareReadBuffer(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.rcvDatapkt.len != 20 {
		t.Errorf("Buffer not filled")
	}
}

func TestPrepareReadBufferWithError(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{receiveErr: errors.New("recv error")}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT, buf: ns.rcvBuf}

	err := ns.PrepareReadBuffer(context.Background())
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestReadUI8(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 1, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	ns.rcvDatapkt.buf[NSPDADAT] = 42

	val, err := ns.ReadUI8(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
}

func TestReadUI8WithError(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{receiveErr: errors.New("recv error")}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT, buf: ns.rcvBuf}

	_, err := ns.ReadUI8(context.Background())
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestReadUI16(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 2, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	binary.BigEndian.PutUint16(ns.rcvDatapkt.buf[NSPDADAT:], 1234)

	val, err := ns.ReadUI16(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 1234 {
		t.Errorf("Expected 1234, got %d", val)
	}
}

func TestReadUI16MultiPacket(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 20}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT + 1, len: NSPDADAT + 2, buf: ns.rcvBuf}
	ns.rcvDatapkt.buf[ns.rcvDatapkt.offset] = 0x04 // first byte of 1234 (0x04D2)

	// Next packet with second byte
	nextPkt := make([]byte, 20)
	binary.BigEndian.PutUint16(nextPkt[0:2], 20)
	nextPkt[4] = NSPTDA
	nextPkt[NSPDADAT] = 0xD2
	mock.receivedData = nextPkt
	mock.recvPos = 0

	val, err := ns.ReadUI16(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 1234 {
		t.Errorf("Expected 1234, got %d", val)
	}
}
func TestReadNativeUI16(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 2, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	binary.LittleEndian.PutUint16(ns.rcvDatapkt.buf[NSPDADAT:], 1234)

	val, err := ns.ReadNativeUI16(context.Background(), true)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 1234 {
		t.Errorf("Expected 1234, got %d", val)
	}
}

func TestReadUI32(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 4, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	binary.BigEndian.PutUint32(ns.rcvDatapkt.buf[NSPDADAT:], 12345678)

	val, err := ns.ReadUI32(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 12345678 {
		t.Errorf("Expected 12345678, got %d", val)
	}
}

func TestReadText(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 5, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	copy(ns.rcvDatapkt.buf[NSPDADAT:], []byte("test\x00"))

	buf, err := ns.ReadText(context.Background(), 10)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(*buf) != "test" {
		t.Errorf("Expected 'test', got '%s'", *buf)
	}
}

func TestReadBA(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 4, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	copy(ns.rcvDatapkt.buf[NSPDADAT:], []byte{1, 2, 3, 4})

	buf, err := ns.ReadBA(context.Background(), 4)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(buf, []byte{1, 2, 3, 4}) {
		t.Errorf("Expected [1 2 3 4], got %v", buf)
	}
}

func TestWriteUI8(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sndBuf = make([]byte, ns.sAtts.sdu)
	ns.sndDatapkt = &dataPacket{offset: NSPDADAT, buf: ns.sndBuf, bufLen: len(ns.sndBuf)}
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)

	err := ns.WriteUI8(context.Background(), 42)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.sndDatapkt.buf[NSPDADAT] != 42 {
		t.Errorf("Expected 42, got %d", ns.sndDatapkt.buf[NSPDADAT])
	}
}

func TestWriteI32(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sndBuf = make([]byte, ns.sAtts.sdu)
	ns.sndDatapkt = &dataPacket{offset: NSPDADAT, buf: ns.sndBuf, bufLen: len(ns.sndBuf)}
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)

	err := ns.WriteI32(context.Background(), 12345678, false)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if binary.BigEndian.Uint32(ns.sndDatapkt.buf[NSPDADAT:]) != 12345678 {
		t.Errorf("Value not written")
	}
}

func TestWriteBA(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sndBuf = make([]byte, ns.sAtts.sdu)
	ns.sndDatapkt = &dataPacket{offset: NSPDADAT, buf: ns.sndBuf, bufLen: len(ns.sndBuf)}
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)

	ba := []byte{1, 2, 3, 4}
	err := ns.WriteBA(context.Background(), &ba)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(ns.sndDatapkt.buf[NSPDADAT:ns.sndDatapkt.offset], ba) {
		t.Errorf("Expected [1 2 3 4], got %v", ns.sndDatapkt.buf[NSPDADAT:ns.sndDatapkt.offset])
	}
}

func TestSkipNBytes(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 10, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	copy(ns.rcvDatapkt.buf[NSPDADAT:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	err := ns.SkipNBytes(context.Background(), 5)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.rcvDatapkt.offset != NSPDADAT+5 {
		t.Errorf("Expected offset %d, got %d", NSPDADAT+5, ns.rcvDatapkt.offset)
	}
}
func TestCancelOperation(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.connected = true
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.sndBuf = make([]byte, ns.sAtts.sdu)
	ns.sndDatapkt = &dataPacket{offset: NSPDADAT, buf: ns.sndBuf, bufLen: len(ns.sndBuf)}
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT, buf: ns.rcvBuf}

	// Mock reset marker
	resetMarker := make([]byte, 11)
	binary.BigEndian.PutUint16(resetMarker[0:2], 11)
	resetMarker[4] = NSPTMK
	resetMarker[8] = NSPMKTD1
	resetMarker[10] = NIQRMARK

	mock.receivedData = resetMarker
	mock.recvPos = 0

	err := ns.CancelOperation(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.isBreak || ns.isReset {
		t.Errorf("Expected isBreak and isReset to be false after reset, got %v %v", ns.isBreak, ns.isReset)
	}
}
func TestIsInBreakReset(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.isBreak = true
	if !ns.IsInBreakReset() {
		t.Errorf("Expected true, got false")
	}
	ns.isBreak = false
	ns.isReset = true
	if !ns.IsInBreakReset() {
		t.Errorf("Expected true, got false")
	}
	ns.isReset = false
	if ns.IsInBreakReset() {
		t.Errorf("Expected false, got true")
	}
}

// Add more tests for other functions to increase coverage...

func TestReadMultiPacket(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 20}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT + 5, len: NSPDADAT + 10, buf: ns.rcvBuf} // 5 bytes remaining

	copy(ns.rcvDatapkt.buf[ns.rcvDatapkt.offset:], []byte{1, 2, 3, 4, 5})

	// Next packet
	nextPkt := make([]byte, 20)
	binary.BigEndian.PutUint16(nextPkt[0:2], 20)
	nextPkt[4] = NSPTDA
	copy(nextPkt[NSPDADAT:], []byte{6, 7, 8, 9, 10})
	mock.receivedData = nextPkt
	mock.recvPos = 0

	buf := make([]byte, 10)
	err := ns.readMultiPacket(context.Background(), buf, 10)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	for i := 0; i < 10; i++ {
		if buf[i] != byte(i+1) {
			t.Errorf("Mismatch at %d", i)
		}
	}
}

func TestFlush(t *testing.T) {
	t.Parallel()
	t.Run("WithBufferedData", func(t *testing.T) {
		ns := newNetworkSession()
		ns.sAtts = &sessionAtts{sdu: 8192}
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		ns.sndBuf = make([]byte, ns.sAtts.sdu)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)
		ns.sndDatapkt.FillBuf([]byte("test data"), 0, 9)

		err := ns.Flush(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(mock.sentData) != 1 {
			t.Errorf("Flush did not send")
		}
		if ns.sndDatapkt.offset != NSPDADAT {
			t.Errorf("Offset not reset")
		}
	})

	t.Run("WithoutBufferedData", func(t *testing.T) {
		ns := newNetworkSession()
		ns.sAtts = &sessionAtts{sdu: 8192}
		mock := &mockNTAdapter{}
		ns.ntAdapter = mock
		ns.sndBuf = make([]byte, ns.sAtts.sdu)
		ns.sndDatapkt = &dataPacket{}
		ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)

		err := ns.Flush(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(mock.sentData) != 0 {
			t.Errorf("Flush sent empty packet")
		}
		if ns.sndDatapkt.offset != NSPDADAT {
			t.Errorf("Offset changed without buffered data")
		}
	})
}

func TestSendReset(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.connected = true
	ns.sndBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.sndDatapkt = &dataPacket{}
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)
	ns.rcvDatapkt = &dataPacket{buf: ns.rcvBuf}

	// Mock reset marker
	resetMarker := make([]byte, 11)
	binary.BigEndian.PutUint16(resetMarker[0:2], 11)
	resetMarker[4] = NSPTMK
	resetMarker[8] = NSPMKTD1
	resetMarker[10] = NIQRMARK

	mock.receivedData = resetMarker
	mock.recvPos = 0

	err := ns.SendReset(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.isReset != false || ns.isBreak != false {
		t.Errorf("Expected isReset and isBreak to be false, got %v %v", ns.isReset, ns.isBreak)
	}
}
func TestPrepareReadBufferWithData(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 10, buf: ns.rcvBuf}

	err := ns.PrepareReadBuffer(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.rcvDatapkt.offset != NSPDADAT {
		t.Errorf("Offset not reset")
	}
}
func TestReadUI32MultiPacket(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 20}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 2, buf: ns.rcvBuf}
	binary.BigEndian.PutUint16(ns.rcvDatapkt.buf[ns.rcvDatapkt.offset:], 0x1234) // partial

	nextPkt := make([]byte, 20)
	binary.BigEndian.PutUint16(nextPkt[0:2], 20)
	nextPkt[4] = NSPTDA
	binary.BigEndian.PutUint16(nextPkt[NSPDADAT:], 0x5678)
	mock.receivedData = nextPkt
	mock.recvPos = 0

	val, err := ns.ReadUI32(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 0x12345678 {
		t.Errorf("Expected 0x12345678, got %x", val)
	}
}
func TestWriteUI16WithFlush(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 10}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sndBuf = make([]byte, ns.sAtts.sdu+NSPDADAT)
	ns.sndDatapkt = &dataPacket{buf: ns.sndBuf, bufLen: len(ns.sndBuf)} // almost full
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)
	ns.sndDatapkt.offset = len(ns.sndBuf) - 1

	err := ns.WriteUI16(context.Background(), 1234, true)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(mock.sentData) != 1 {
		t.Errorf("Expected flush")
	}
}
func TestSendWithBreak(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.isBreak = true
	err := ns.Send(context.Background(), []byte{1, 2, 3}, 0, 3)
	if err != nil {
		t.Errorf("Unexpected error with break")
	}
}

func TestWriteBytesWithContext(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.sndBuf = make([]byte, ns.sAtts.sdu)
	ns.sndDatapkt = &dataPacket{offset: NSPDADAT, buf: ns.sndBuf, bufLen: len(ns.sndBuf)}
	ns.sndDatapkt.marshal(ns.sndBuf, ns.sAtts, 0)

	src := []byte{1, 2, 3}
	err := ns.WriteBytesWithContext(context.Background(), src)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(ns.sndDatapkt.buf[NSPDADAT:ns.sndDatapkt.offset], src) {
		t.Errorf("Expected [1 2 3], got %v", ns.sndDatapkt.buf[NSPDADAT:ns.sndDatapkt.offset])
	}
}

func TestReadByteWithContext(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 1, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	ns.rcvDatapkt.buf[NSPDADAT] = 42

	val, err := ns.ReadByteWithContext(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
}

func TestReadBytesWithContext(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: 8192}
	mock := &mockNTAdapter{}
	ns.ntAdapter = mock
	ns.rcvBuf = make([]byte, ns.sAtts.sdu)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + 4, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	copy(ns.rcvDatapkt.buf[NSPDADAT:], []byte{1, 2, 3, 4})

	buf, err := ns.ReadBytesWithContext(context.Background(), 4)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(*buf, []byte{1, 2, 3, 4}) {
		t.Errorf("Expected [1 2 3 4], got %v", *buf)
	}
}

func TestReadBytesWithContextDoesNotTruncateLargeLength(t *testing.T) {
	t.Parallel()
	ns := newNetworkSession()
	ns.sAtts = &sessionAtts{sdu: NSPABSSDULN}
	length := 65536
	ns.rcvBuf = make([]byte, NSPDADAT+length)
	ns.rcvDatapkt = &dataPacket{offset: NSPDADAT, len: NSPDADAT + length, buf: ns.rcvBuf}
	ns.rcvDatapkt.marshal(ns.rcvBuf, ns.sAtts, 0)
	for i := 0; i < length; i++ {
		ns.rcvDatapkt.buf[NSPDADAT+i] = byte(i)
	}

	buf, err := ns.ReadBytesWithContext(context.Background(), int32(length))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(*buf) != length {
		t.Fatalf("Expected %d bytes, got %d", length, len(*buf))
	}
	if ns.rcvDatapkt.Remaining() != 0 {
		t.Fatalf("Expected buffer to be fully drained, %d bytes remain", ns.rcvDatapkt.Remaining())
	}
}
