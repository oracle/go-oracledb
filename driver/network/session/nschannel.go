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

	"github.com/oracle/go-driver/driver/common"
)

// NSChannel interface defines methods for network communication
type NSChannel interface {
	PrepareReadBuffer(ctx context.Context) error
	Write(ctx context.Context, buf []byte) error
	CancelOperation(ctx context.Context) error
	SendInterrupt(ctx context.Context) error
	SendReset(ctx context.Context) error
	SendZDP(ctx context.Context) error
	SendZeroCopy(ctx context.Context, buf []byte)
	ReceiveZeroCopy(ctx context.Context, buf []byte)
	Flush(ctx context.Context) error
	IsInBreakReset() bool
}

// ByteOrder defines endianness for multi-byte reads/writes
type ByteOrder struct {
	name string
}

var (
	BIG_ENDIAN    = ByteOrder{name: "BIG_ENDIAN"}
	LITTLE_ENDIAN = ByteOrder{name: "LITTLE_ENDIAN"}
)

// PrepareReadBuffer ensures that the receive buffer has data available for reading
func (ns *NetworkSession) PrepareReadBuffer(ctx context.Context) error {
	//If the current receive packet has no remaining data, it resets
	if ns.RcvDatapkt.Len-ns.RcvDatapkt.Offset == 0 {
		ns.RcvDatapkt.Offset = NSPDADAT
		ns.RcvDatapkt.Len = ns.RcvDatapkt.BufLen
		// fillReadBuffer to populate the buffer with new data from the network.
		return ns.fillReadBuffer(ctx)
	}
	return nil
}

// fillReadBuffer to populate the buffer with new data from the network.
func (ns *NetworkSession) fillReadBuffer(ctx context.Context) error {
	_, err := ns.recvPacket(ctx)
	if ns.IsBreak {
		return fmt.Errorf("break-packet received")
	}
	if err != nil {
		return err
	}
	return nil

}

func (ns *NetworkSession) ReadByteWithContext(ctx context.Context) (byte, error) {
	return ns.ReadUI8(ctx)
}

func (ns *NetworkSession) ReadBytesWithContext(ctx context.Context, length int32) (*[]byte, error) {
	if length < 0 {
		return nil, fmt.Errorf("invalid byte length: %d", length)
	}
	return ns.readNBytes(ctx, int(length))
}

func (ns *NetworkSession) ReadNBytes(ctx context.Context, n uint16) (*[]byte, error) {
	return ns.readNBytes(ctx, int(n))
}

func (ns *NetworkSession) readNBytes(ctx context.Context, n int) (*[]byte, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return nil, err
	}
	var b []byte
	// Remaining  returns the number of bytes left to read in the current data packet
	if ns.RcvDatapkt.Remaining() >= n {
		b, err = ns.RcvDatapkt.Read(n)
	} else {
		b = make([]byte, n)
		err = ns.readMultiPacket(ctx, b, n)
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (ns *NetworkSession) ReadUI8(ctx context.Context) (uint8, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return 0, err
	}
	rByte, err := ns.RcvDatapkt.ReadByte()
	return rByte, err
}

func (ns *NetworkSession) ReadNativeUI16(ctx context.Context, isLSB bool) (uint16, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return 0, err
	}
	var b []byte
	if ns.RcvDatapkt.Remaining() >= 2 {
		b, err = ns.RcvDatapkt.Read(2)
	} else {
		b = make([]byte, 2)
		err = ns.readMultiPacket(ctx, b, 2)
	}
	if err != nil {
		return 0, err
	}
	if isLSB {
		return binary.LittleEndian.Uint16(b), nil
	} else {
		return binary.BigEndian.Uint16(b), nil
	}
}

func (ns *NetworkSession) ReadUI16(ctx context.Context) (uint16, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return 0, err
	}
	var b []byte
	if ns.RcvDatapkt.Remaining() >= 2 {
		b, err = ns.RcvDatapkt.Read(2)
	} else {
		b = make([]byte, 2)
		err = ns.readMultiPacket(ctx, b, 2)
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

// readMultiPacket reads the specified number of bytes (numBytes) into the provided buffer (buf),
// spanning multiple packets if necessary. It continues reading from the current receive packet until
// the requested amount is fulfilled, fetching new packets via recvPacket when the current packet's
// remaining data is exhausted
func (ns *NetworkSession) readMultiPacket(ctx context.Context, buf []byte, numBytes int) error {
	bytesRead := 0
	for bytesRead < numBytes {
		remaining := ns.RcvDatapkt.Remaining()
		if remaining == 0 {
			_, err := ns.recvPacket(ctx)
			if ns.IsBreak {
				return fmt.Errorf("break-packet received")
			}
			if err != nil {
				return fmt.Errorf("failed to receive next packet: %w", err)
			}
			continue
		}

		bytesToRead := numBytes - bytesRead
		if bytesToRead > remaining {
			bytesToRead = remaining
		}
		data, err := ns.RcvDatapkt.Read(bytesToRead)
		if err != nil {
			return fmt.Errorf("failed to read from current packet: %w", err)
		}

		copy(buf[bytesRead:], data)
		bytesRead += len(data)
	}

	return nil

}

func (ns *NetworkSession) GetRemainingByteCount(ctx context.Context) int {
	return ns.RcvDatapkt.Remaining()
}

func (ns *NetworkSession) ReadUI32(ctx context.Context) (uint32, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return 0, err
	}
	var b []byte
	if ns.RcvDatapkt.Remaining() >= 4 {
		b, err = ns.RcvDatapkt.Read(4)
	} else {
		b = make([]byte, 4)
		err = ns.readMultiPacket(ctx, b, 4)
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (ns *NetworkSession) ReadI64(ctx context.Context) (int64, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return 0, err
	}
	var b []byte
	if ns.RcvDatapkt.Remaining() >= 8 {
		b, err = ns.RcvDatapkt.Read(8)
	} else {
		b = make([]byte, 8)
		err = ns.readMultiPacket(ctx, b, 8)
	}
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), nil
}
func (ns *NetworkSession) ReadText(ctx context.Context, length int) (*[]byte, error) {

	offset := 0
	tmpBuffer := make([]byte, length)
	for offset < length {
		//It reads byte by byte,
		// stopping early if a null byte (0) is encountered
		b, err := ns.RcvDatapkt.ReadByte()
		if err == io.EOF {
			err = ns.PrepareReadBuffer(ctx)
			if err != nil {
				return nil, err
			}
			b, err = ns.RcvDatapkt.ReadByte()
			if err != nil {
				return nil, err
			}
		}
		tmpBuffer[offset] = b
		offset++
		if b == 0 {
			break
		}
	}
	offset--
	return new(tmpBuffer[:offset]), nil
}
func (ns *NetworkSession) ReadBA(ctx context.Context, n uint16) ([]byte, error) {
	err := ns.PrepareReadBuffer(ctx)
	if err != nil {
		return nil, err
	}
	buf, err := ns.ReadNBytes(ctx, n)
	if err != nil {
		return nil, err
	}
	return *buf, nil
}

func (ns *NetworkSession) ReadBA2(ctx context.Context, n1, n2 uint16) ([]byte, []byte, error) {
	ba1, err := ns.ReadNBytes(ctx, n1)
	if err != nil {
		return nil, nil, err
	}
	ba2, err := ns.ReadNBytes(ctx, n2)
	if err != nil {
		return nil, nil, err
	}
	return *ba1, *ba2, nil
}

// Write helper methods for writing multi-byte values
func (ns *NetworkSession) WriteUI8(ctx context.Context, value uint8) error {
	return ns.WriteByteWithContext(ctx, byte(value))
}

func (ns *NetworkSession) WriteBytesWithContext(ctx context.Context, bytes []byte) error {
	return ns.Write(ctx, bytes)
}

func (ns *NetworkSession) Write(ctx context.Context, bytes []byte) error {
	if len(bytes) <= 0 {
		return nil
	}
	return ns.Send(ctx, bytes, 0, len(bytes))
}
func (ns *NetworkSession) WriteUI16(ctx context.Context, value int16, isLSB bool) error {
	if ns.SndDatapkt.Len-ns.SndDatapkt.Offset < 2 {
		if err := ns.Flush(ctx); err != nil {
			return err
		}
	}
	buf := ns.SndDatapkt.Buf[ns.SndDatapkt.Offset:]

	if isLSB {
		binary.LittleEndian.PutUint16(buf, uint16(value))
	} else {
		binary.BigEndian.PutUint16(buf, uint16(value))
	}
	ns.SndDatapkt.Offset += 2
	return nil
}

func (ns *NetworkSession) WriteI64(ctx context.Context, value int64, isLSB bool) error {
	// Check if there's enough space in the current send packet buffer for writing an int64 (8 bytes).
	// If not, flush the current packet to send it and prepare a new one for the write operation.
	if ns.SndDatapkt.Len-ns.SndDatapkt.Offset < 8 {
		if err := ns.Flush(ctx); err != nil {
			return err
		}
	}
	buf := ns.SndDatapkt.Buf[ns.SndDatapkt.Offset:]
	if isLSB {
		binary.LittleEndian.PutUint64(buf, uint64(value))
	} else {
		binary.BigEndian.PutUint64(buf, uint64(value))
	}
	ns.SndDatapkt.Offset += 8
	return nil
}

func (ns *NetworkSession) WriteText(ctx context.Context, text string) error {
	bytes := []byte(text)
	return ns.Send(ctx, bytes, 0, len(bytes))
}

func (ns *NetworkSession) WriteB(ctx context.Context, b byte) error {
	panic("same as writeByte")
}

func (ns *NetworkSession) WriteBA(ctx context.Context, ba *[]byte) error {
	return ns.Send(ctx, *ba, 0, len(*ba))
}

func (ns *NetworkSession) WriteBA2(ctx context.Context, ba *[]byte, n1, n2 int) error {
	panic("implement me")
}

func (ns *NetworkSession) SetByteOrder(ctx context.Context, bo ByteOrder) {
	ns.byteOrder = bo
}

func (ns *NetworkSession) GetByteOrder(ctx context.Context) ByteOrder {
	return ns.byteOrder
}

func (ns *NetworkSession) WriteI32(ctx context.Context, value int32, isLSB bool) error {

	if ns.SndDatapkt.Len-ns.SndDatapkt.Offset < 4 {
		if err := ns.Flush(ctx); err != nil {
			return err
		}
	}
	buf := ns.SndDatapkt.Buf[ns.SndDatapkt.Offset:]
	if isLSB {
		binary.LittleEndian.PutUint32(buf, uint32(value))

	} else {
		binary.BigEndian.PutUint32(buf, uint32(value))

	}
	ns.SndDatapkt.Offset += 4
	return nil

}

func (ns *NetworkSession) WriteByteWithContext(ctx context.Context, value byte) error {

	if ns.SndDatapkt.Len-ns.SndDatapkt.Offset < 1 {
		if err := ns.Flush(ctx); err != nil {
			return err
		}
	}
	b := ns.SndDatapkt.Buf[ns.SndDatapkt.Offset:]
	b[0] = value
	ns.SndDatapkt.Offset++
	return nil
}

func (ns *NetworkSession) WritePartial(ctx context.Context, src []byte, offset int, length int) error {
	if offset < 0 || length < 0 || offset+length > len(src) {
		return fmt.Errorf("invalid offset or length")
	}
	if ns.SndDatapkt.Remaining() < length {
		if err := ns.Flush(ctx); err != nil {
			return err
		}
		if ns.SndDatapkt.Remaining() < length {
			return fmt.Errorf("buffer overflow: insufficient space after flush")
		}
	}
	return ns.Write(ctx, src[offset:offset+length])
}

func (ns *NetworkSession) SkipNBytes(ctx context.Context, n int) error {
	bytesSkipped := 0

	for bytesSkipped < n {
		err := ns.PrepareReadBuffer(ctx)
		if err != nil {
			return err
		}

		remaining := ns.RcvDatapkt.Remaining()
		needToSkip := n - bytesSkipped

		if remaining >= needToSkip {
			// Skip what we need and exit
			_, err := ns.RcvDatapkt.Read(needToSkip)
			if err != nil {
				return err
			}
			bytesSkipped = n
		} else {
			// Skip all available and loop again
			if remaining > 0 {
				_, err := ns.RcvDatapkt.Read(remaining)
				if err != nil {
					return err
				}
				bytesSkipped += remaining
			}
			// refill buffer on next iteration
		}
	}

	return nil
}

// CancelOperation cancels the current operation in the server.
//
// Sends Break marker packet followed by reset and ignores all packets received
// from the server until a reset packet is received.
func (ns *NetworkSession) CancelOperation(ctx context.Context) error {
	if ns.IsBreak {
		return nil
	}
	if !ns.Connected {
		ns.IsBreak = true
		ns.BreakPosted = true
		return nil
	}

	// Build and send break packet
	ns.IsBreak = true
	var markerPkt = &MarkerPacket{}
	err := markerPkt.Marshal(nil, ns.SAtts, NIQIMARK)
	if err != nil {
		return err
	}
	err = ns.SendPacket(ctx, markerPkt.Buf)
	common.Odl.Debug("Break packet sent")
	if err != nil {
		common.Odl.Info("An error occurred while sending the break packet", "error", err)
		return err
	}
	// Call reset
	err = ns.Reset(ctx)
	if err != nil {
		common.Odl.Info("An error occurred on reset", "error", err)
		return err
	}
	return nil
}

func (ns *NetworkSession) SendInterrupt(ctx context.Context) error {
	// Placeholder: Implement interrupt logic if needed
	return fmt.Errorf("SendInterrupt not implemented")
}

func (ns *NetworkSession) SendReset(ctx context.Context) error {
	return ns.Reset(ctx) // Reuse existing Reset method
}

func (ns *NetworkSession) SendZDP(ctx context.Context) error {
	// Placeholder: Zero Data Packet logic
	return fmt.Errorf("SendZDP not implemented")
}

func (ns *NetworkSession) SendZeroCopy(ctx context.Context, bytes []byte) {
	// Placeholder: Zero-copy send logic
	panic("SendZeroCopy not implemented")
}

func (ns *NetworkSession) ReceiveZeroCopy(ctx context.Context, bytes []byte) {
	// Placeholder: Zero-copy receive logic
	panic("ReceiveZeroCopy not implemented")
}

func (ns *NetworkSession) Flush(ctx context.Context) error {
	if ns.IsBreak {
		return nil
	}
	if ns.SndDatapkt.Offset <= NSPDADAT {
		return nil
	}
	err := ns.SndDatapkt.Prepare2Send(0, ns.SAtts)
	if err != nil {
		return err
	}
	err = ns.SendPacket(ctx, ns.SndDatapkt.Buf[:ns.SndDatapkt.Offset])
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("connection closed during flush")
		}
		return err
	}
	ns.SndDatapkt.Reset()
	return nil
}

func (ns *NetworkSession) IsInBreakReset() bool {
	return ns.IsBreak || ns.IsReset
}
