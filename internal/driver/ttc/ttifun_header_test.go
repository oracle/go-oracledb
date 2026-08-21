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

package ttc

import (
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestTTIFunNoPayload_MarshalTo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		funcType  common.FunctionType
		wantBytes []byte
		wantErr   bool
		bufSize   int
	}{
		{
			name:      "logOff - normal case",
			funcType:  logOff,
			wantBytes: []byte{byte(logOff), 1},
			wantErr:   false,
			bufSize:   2,
		},
		{
			name:      "Other function type - normal case",
			funcType:  common.FunctionType(0xFF), // test that it will marshal the given FunctionType.
			wantBytes: []byte{byte(0xFF), 1},
			wantErr:   false,
			bufSize:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ttiFunHeader{_funcType: tt.funcType}

			dataBuffer := NewArrayDataBuffer(tt.bufSize)
			engine := NewMarshalEngine(dataBuffer, common.BIG_ENDIAN, [5]byte{Native, Native, Native, Native, Native})

			err := msg.MarshalTo(context.Background(), engine)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalTo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if dataBuffer.currentWritePosition != len(tt.wantBytes) {
					t.Errorf("MarshalTo() wrote %d bytes, want %d", dataBuffer.currentWritePosition, len(tt.wantBytes))
				}
				for i, b := range tt.wantBytes {
					if dataBuffer.bytes[i] != b {
						t.Errorf("MarshalTo() byte at position %d = %v, want %v", i, dataBuffer.bytes[i], b)
					}
				}
			}
		})
	}
}

func TestNewLogOff(t *testing.T) {
	t.Parallel()
	msg := newLogOff()
	if msg.GetMsgCode() != TTIFUN {
		t.Errorf("newLogOff() GetMsgCode = %v, want %v", msg.GetMsgCode(), TTIFUN)
	}
	noPayloadMsg, ok := msg.(*ttiFunHeader)
	if !ok {
		t.Errorf("newLogOff() did not return *ttiFunNoPayload")
	}
	dataBuffer := NewArrayDataBuffer(2)
	engine := NewMarshalEngine(dataBuffer, common.BIG_ENDIAN, [5]byte{Native, Native, Native, Native, Native})
	err := noPayloadMsg.MarshalTo(context.Background(), engine)
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if dataBuffer.bytes[0] != byte(logOff) {
		t.Errorf("Marshalled function code should be logOff but was %d", dataBuffer.bytes[0])
	}
}

func TestNewLogOff18(t *testing.T) {
	t.Parallel()
	msg := newLogOff18()
	if msg.GetMsgCode() != TTIFUN {
		t.Errorf("newLogOff() GetMsgCode = %v, want %v", msg.GetMsgCode(), TTIFUN)
	}
	noPayloadMsg, ok := msg.(*ttiFunHeader18)
	if !ok {
		t.Errorf("newLogOff() did not return *ttiFunNoPayload18")
	}
	dataBuffer := NewArrayDataBuffer(3)
	engine := NewMarshalEngine(dataBuffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	err := noPayloadMsg.MarshalTo(context.Background(), engine)
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	// firsrt byte contains function code
	if dataBuffer.bytes[0] != byte(logOff) {
		t.Errorf("Marshalled function code should be logOff but was %d", dataBuffer.bytes[0])
	}
	// second byte contains sequence number
	if dataBuffer.bytes[1] != 1 {
		t.Errorf("Marshalled sequence number should be 1 but was %d", dataBuffer.bytes[1])
	}
	// third byte contains token -> always 0
	if dataBuffer.bytes[2] != 0 {
		t.Errorf("Marshalled token should be 0 but was %d", dataBuffer.bytes[2])
	}
}
