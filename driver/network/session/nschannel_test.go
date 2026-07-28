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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT, Buf: ns.rcvBuf} // no data

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
	if ns.RcvDatapkt.Len != 20 {
		t.Errorf("Buffer not filled")
	}
}

func TestPrepareReadBufferWithError(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{receiveErr: errors.New("recv error")}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT, Buf: ns.rcvBuf}

	err := ns.PrepareReadBuffer(context.Background())
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestReadUI8(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 1, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	ns.RcvDatapkt.Buf[NSPDADAT] = 42

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{receiveErr: errors.New("recv error")}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT, Buf: ns.rcvBuf}

	_, err := ns.ReadUI8(context.Background())
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestReadUI16(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 2, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	binary.BigEndian.PutUint16(ns.RcvDatapkt.Buf[NSPDADAT:], 1234)

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 20}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT + 1, Len: NSPDADAT + 2, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Buf[ns.RcvDatapkt.Offset] = 0x04 // first byte of 1234 (0x04D2)

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 2, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	binary.LittleEndian.PutUint16(ns.RcvDatapkt.Buf[NSPDADAT:], 1234)

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 4, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	binary.BigEndian.PutUint32(ns.RcvDatapkt.Buf[NSPDADAT:], 12345678)

	val, err := ns.ReadUI32(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 12345678 {
		t.Errorf("Expected 12345678, got %d", val)
	}
}

func TestReadI64(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 8, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	binary.BigEndian.PutUint64(ns.RcvDatapkt.Buf[NSPDADAT:], 1234567890123456)

	val, err := ns.ReadI64(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 1234567890123456 {
		t.Errorf("Expected 1234567890123456, got %d", val)
	}
}

func TestReadText(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 5, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	copy(ns.RcvDatapkt.Buf[NSPDADAT:], []byte("test\x00"))

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 4, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	copy(ns.RcvDatapkt.Buf[NSPDADAT:], []byte{1, 2, 3, 4})

	buf, err := ns.ReadBA(context.Background(), 4)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(buf, []byte{1, 2, 3, 4}) {
		t.Errorf("Expected [1 2 3 4], got %v", buf)
	}
}

func TestReadBA2(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 6, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	copy(ns.RcvDatapkt.Buf[NSPDADAT:], []byte{1, 2, 3, 4, 5, 6})

	ba1, ba2, err := ns.ReadBA2(context.Background(), 3, 3)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(ba1, []byte{1, 2, 3}) || !bytes.Equal(ba2, []byte{4, 5, 6}) {
		t.Errorf("Expected [1 2 3] [4 5 6], got %v %v", ba1, ba2)
	}
}

func TestWriteUI8(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	err := ns.WriteUI8(context.Background(), 42)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.SndDatapkt.Buf[NSPDADAT] != 42 {
		t.Errorf("Expected 42, got %d", ns.SndDatapkt.Buf[NSPDADAT])
	}
}

func TestWriteI32(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	err := ns.WriteI32(context.Background(), 12345678, false)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if binary.BigEndian.Uint32(ns.SndDatapkt.Buf[NSPDADAT:]) != 12345678 {
		t.Errorf("Value not written")
	}
}

func TestWriteI64(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	err := ns.WriteI64(context.Background(), 1234567890123456, false)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if binary.BigEndian.Uint64(ns.SndDatapkt.Buf[NSPDADAT:]) != 1234567890123456 {
		t.Errorf("Value not written")
	}
}

func TestWriteText(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	err := ns.WriteText(context.Background(), "test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset]) != "test" {
		t.Errorf("Expected 'test', got '%s'", string(ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset]))
	}
}

func TestWriteBA(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	ba := []byte{1, 2, 3, 4}
	err := ns.WriteBA(context.Background(), &ba)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset], ba) {
		t.Errorf("Expected [1 2 3 4], got %v", ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset])
	}
}

func TestSkipNBytes(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 10, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	copy(ns.RcvDatapkt.Buf[NSPDADAT:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	err := ns.SkipNBytes(context.Background(), 5)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.RcvDatapkt.Offset != NSPDADAT+5 {
		t.Errorf("Expected offset %d, got %d", NSPDADAT+5, ns.RcvDatapkt.Offset)
	}
}
func TestCancelOperation(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.Connected = true
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT, Buf: ns.rcvBuf}

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
	if ns.IsBreak || ns.IsReset {
		t.Errorf("Expected IsBreak and IsReset to be false after reset, got %v %v", ns.IsBreak, ns.IsReset)
	}
}
func TestIsInBreakReset(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.IsBreak = true
	if !ns.IsInBreakReset() {
		t.Errorf("Expected true, got false")
	}
	ns.IsBreak = false
	ns.IsReset = true
	if !ns.IsInBreakReset() {
		t.Errorf("Expected true, got false")
	}
	ns.IsReset = false
	if ns.IsInBreakReset() {
		t.Errorf("Expected false, got true")
	}
}

// Add more tests for other functions to increase coverage...

func TestReadMultiPacket(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 20}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT + 5, Len: NSPDADAT + 10, Buf: ns.rcvBuf} // 5 bytes remaining

	copy(ns.RcvDatapkt.Buf[ns.RcvDatapkt.Offset:], []byte{1, 2, 3, 4, 5})

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
		ns := NewNetworkSession()
		ns.SAtts = &SessionAtts{SDU: 8192}
		mock := &mockNTAdapter{}
		ns.NTAdapter = mock
		ns.sndBuf = make([]byte, ns.SAtts.SDU)
		ns.SndDatapkt = &DataPacket{}
		ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
		ns.SndDatapkt.FillBuf([]byte("test data"), 0, 9, 0, false)

		err := ns.Flush(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(mock.sentData) != 1 {
			t.Errorf("Flush did not send")
		}
		if ns.SndDatapkt.Offset != NSPDADAT {
			t.Errorf("Offset not reset")
		}
	})

	t.Run("WithoutBufferedData", func(t *testing.T) {
		ns := NewNetworkSession()
		ns.SAtts = &SessionAtts{SDU: 8192}
		mock := &mockNTAdapter{}
		ns.NTAdapter = mock
		ns.sndBuf = make([]byte, ns.SAtts.SDU)
		ns.SndDatapkt = &DataPacket{}
		ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

		err := ns.Flush(context.Background())
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(mock.sentData) != 0 {
			t.Errorf("Flush sent empty packet")
		}
		if ns.SndDatapkt.Offset != NSPDADAT {
			t.Errorf("Offset changed without buffered data")
		}
	})
}

func TestWritePartial(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 20}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	src := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	err := ns.WritePartial(context.Background(), src, 2, 5)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset], []byte{3, 4, 5, 6, 7}) {
		t.Errorf("Expected [3 4 5 6 7], got %v", ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset])
	}
}

func TestSendInterrupt(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	err := ns.SendInterrupt(context.Background())
	if err == nil || err.Error() != "SendInterrupt not implemented" {
		t.Errorf("Expected 'SendInterrupt not implemented', got %v", err)
	}
}

func TestSendReset(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.Connected = true
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
	ns.RcvDatapkt = &DataPacket{Buf: ns.rcvBuf}

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
	if ns.IsReset != false || ns.IsBreak != false {
		t.Errorf("Expected IsReset and IsBreak to be false, got %v %v", ns.IsReset, ns.IsBreak)
	}
}
func TestSendZDP(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	err := ns.SendZDP(context.Background())
	if err == nil || err.Error() != "SendZDP not implemented" {
		t.Errorf("Expected 'SendZDP not implemented', got %v", err)
	}
}

func TestSendZeroCopy(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic, got none")
		} else if r != "SendZeroCopy not implemented" {
			t.Errorf("Expected 'SendZeroCopy not implemented', got %v", r)
		}
	}()
	ns.SendZeroCopy(context.Background(), []byte{})
}

func TestReceiveZeroCopy(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic, got none")
		} else if r != "ReceiveZeroCopy not implemented" {
			t.Errorf("Expected 'ReceiveZeroCopy not implemented', got %v", r)
		}
	}()
	ns.ReceiveZeroCopy(context.Background(), []byte{})
}

func TestPrepareReadBufferWithData(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 10, Buf: ns.rcvBuf}

	err := ns.PrepareReadBuffer(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if ns.RcvDatapkt.Offset != NSPDADAT {
		t.Errorf("Offset not reset")
	}
}
func TestReadUI32MultiPacket(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 20}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 2, Buf: ns.rcvBuf}
	binary.BigEndian.PutUint16(ns.RcvDatapkt.Buf[ns.RcvDatapkt.Offset:], 0x1234) // partial

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 10}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU+NSPDADAT)
	ns.SndDatapkt = &DataPacket{Buf: ns.sndBuf, BufLen: len(ns.sndBuf)} // almost full
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)
	ns.SndDatapkt.Offset = len(ns.sndBuf) - 1

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
	ns := NewNetworkSession()
	ns.IsBreak = true
	err := ns.Send(context.Background(), []byte{1, 2, 3}, 0, 3)
	if err != nil {
		t.Errorf("Unexpected error with break")
	}
}

func TestWriteBytesWithContext(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.sndBuf = make([]byte, ns.SAtts.SDU)
	ns.SndDatapkt = &DataPacket{Offset: NSPDADAT, Buf: ns.sndBuf, BufLen: len(ns.sndBuf)}
	ns.SndDatapkt.Marshal(ns.sndBuf, ns.SAtts, 0)

	src := []byte{1, 2, 3}
	err := ns.WriteBytesWithContext(context.Background(), src)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !bytes.Equal(ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset], src) {
		t.Errorf("Expected [1 2 3], got %v", ns.SndDatapkt.Buf[NSPDADAT:ns.SndDatapkt.Offset])
	}
}

func TestSetByteOrder(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SetByteOrder(context.Background(), LITTLE_ENDIAN)
	if ns.byteOrder != LITTLE_ENDIAN {
		t.Errorf("Expected LITTLE_ENDIAN, got %v", ns.byteOrder)
	}
}

func TestGetByteOrder(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.byteOrder = LITTLE_ENDIAN
	order := ns.GetByteOrder(context.Background())
	if order != LITTLE_ENDIAN {
		t.Errorf("Expected LITTLE_ENDIAN, got %v", order)
	}
}

func TestWriteB(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic")
		}
	}()
	ns.WriteB(context.Background(), 42)
}

func TestReadByteWithContext(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 1, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	ns.RcvDatapkt.Buf[NSPDADAT] = 42

	val, err := ns.ReadByteWithContext(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
}

func TestGetRemainingByteCount(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.RcvDatapkt = &DataPacket{Offset: 5, Len: 10}
	count := ns.GetRemainingByteCount(context.Background())
	if count != 5 {
		t.Errorf("Expected 5, got %d", count)
	}
}

func TestReadBytesWithContext(t *testing.T) {
	t.Parallel()
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: 8192}
	mock := &mockNTAdapter{}
	ns.NTAdapter = mock
	ns.rcvBuf = make([]byte, ns.SAtts.SDU)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + 4, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	copy(ns.RcvDatapkt.Buf[NSPDADAT:], []byte{1, 2, 3, 4})

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
	ns := NewNetworkSession()
	ns.SAtts = &SessionAtts{SDU: NSPABSSDULN}
	length := 65536
	ns.rcvBuf = make([]byte, NSPDADAT+length)
	ns.RcvDatapkt = &DataPacket{Offset: NSPDADAT, Len: NSPDADAT + length, Buf: ns.rcvBuf}
	ns.RcvDatapkt.Marshal(ns.rcvBuf, ns.SAtts, 0)
	for i := 0; i < length; i++ {
		ns.RcvDatapkt.Buf[NSPDADAT+i] = byte(i)
	}

	buf, err := ns.ReadBytesWithContext(context.Background(), int32(length))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(*buf) != length {
		t.Fatalf("Expected %d bytes, got %d", length, len(*buf))
	}
	if ns.RcvDatapkt.Remaining() != 0 {
		t.Fatalf("Expected buffer to be fully drained, %d bytes remain", ns.RcvDatapkt.Remaining())
	}
}
