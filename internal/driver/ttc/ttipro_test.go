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
	"bytes"
	"context"
	"encoding/binary"
	"regexp"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestTTIproNew asserts that newTTIpro returns a valid, non-nil object.
func TestTTIproNew(t *testing.T) {
	t.Parallel()
	pro := newTTIpro()
	if pro == nil {
		t.Fatal("newTTIpro returned nil")
	}
}

// TestTTIproMarshalTo_Success checks success path marshaling.
func TestTTIproMarshalTo_Success(t *testing.T) {
	t.Parallel()
	pro := newTTIpro()
	buf := NewArrayDataBuffer(1500)
	expected := append(append([]byte{}, 6, 5, 4, 3, 2, 1, 0), []byte("GO_TTC-8.2.0\x00")...)

	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	marshallable, _ := pro.(common.Marshallable)
	err := marshallable.MarshalTo(context.Background(), engine)

	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	written := buf.bytes[:buf.currentWritePosition]
	if !bytes.Equal(written, expected) {
		t.Errorf("got %v, want %v", written, expected)
	}
}

// TestTTIproMarshalTo_Fail checks error marshaling.
func TestTTIproMarshalTo_Fail(t *testing.T) {
	t.Parallel()
	pro := newTTIpro()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
		wantErr   string
	}{
		{"Error Writing supported protocol version signature", 0, 1, "simulated write error"},
		{"Error Writing protocol client identity string", 0, 2, "simulated write error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var adb *ArrayBasedDataBuffer
			if tc.failByte == 1 {
				adb = nil
			} else {
				adb = NewArrayDataBuffer(100)
			}
			buf := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: adb,
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			marshallable, _ := pro.(common.Marshallable)
			err := marshallable.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			matched, matchErr := regexp.MatchString(tc.wantErr, err.Error())
			if matchErr != nil {
				t.Errorf("failed to compile regex pattern: %v", matchErr)
			} else if !matched {
				t.Errorf("expected error to match pattern %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestTTIproGetters validates tTIpro's getter API for capabilities, version, charset, and flags.
func TestTTIproGetters(t *testing.T) {
	t.Parallel()
	runtime := []byte{1, 2, 3}
	compile := []byte{4, 5, 6, 7, 8, 9, 10, 123}
	desc := common.B1Array("test")
	p := &tTIpro{
		oVersion:           123,
		proSvrVer:          456,
		clientCaps:         &capability{runTimeCapabilities: runtime, compileTimeCapabilities: compile},
		serverCaps:         &capability{runTimeCapabilities: runtime, compileTimeCapabilities: compile},
		svrCharSet:         871,
		nCharCharset:       775,
		svrFlags:           1,
		svrPortDescription: (desc),
	}
	if p.getOracleVersion() != 123 {
		t.Errorf("getOracleVersion = %d, want 123", p.getOracleVersion())
	}
	if p.getProtocolVersion() != 456 {
		t.Errorf("getProtocolVersion = %d, want 456", p.getProtocolVersion())
	}
	if p.GetMsgCode() != TTIPRO {
		t.Errorf("GetMsgCode = %v, want %v", p.GetMsgCode(), TTIPRO)
	}
	if !bytes.Equal(*p.getServerRuntimeCapabilities(), runtime) {
		t.Error("getServerRuntimeCapabilities mismatch")
	}
	if !bytes.Equal(*p.getServerCompileTimeCapabilities(), compile) {
		t.Error("getServerCompileTimeCapabilities mismatch")
	}
	if p.getCharacterSet() != 871 {
		t.Errorf("getCharacterSet = %d, want 871", p.getCharacterSet())
	}
	if p.getNCharCharacterSet() != 775 {
		t.Errorf("getNCharCharacterSet = %d, want 775", p.getNCharCharacterSet())
	}
	if p.getFlags() != 1 {
		t.Errorf("getFlags = %d, want 1", p.getFlags())
	}
}

func makeTTIproSuccessPayload(protoVer byte, includeFDO bool, addArrays bool) []byte {
	var buf []byte
	// Append TTC message type, protocol version, reserved byte.
	buf = append(buf, protoVer, 0)
	// Append server port description string ("test"), followed by null-terminator.
	desc := []byte("test")
	buf = append(buf, desc...)
	buf = append(buf, 0)
	// Append server character set (UB2) and server flags (UB1).
	buf = binary.LittleEndian.AppendUint16(buf, 871)
	buf = append(buf, 1)
	// Append svrCharSetElem (UB2): number of charset elements.
	buf = binary.LittleEndian.AppendUint16(buf, 0)
	// buf =  append(buf, []byte{1, 2, 3, 4, 5}...)
	if includeFDO {
		// Append FDO length (UB2): feature data object.
		buf = binary.BigEndian.AppendUint16(buf, 11)
		// Append FDO bytes (dummy/test values).
		fdo := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 7}
		buf = append(buf, fdo...)
		if addArrays {
			// Append compile time capabilities array length and data.
			buf = append(buf, 5)
			buf = append(buf, []byte{1, 2, 3, 4, 5}...)
			// Append run time capabilities array length and data.
			buf = append(buf, 3)
			buf = append(buf, []byte{6, 7, 8}...)
		}
	}
	return buf
}

// TestTTIproUnmarshalFrom_Success checks success path unmarshal.
func TestTTIproUnmarshalFrom_Success(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		payload              []byte
		expectedSvrCharSet   common.UB2
		expectedNCharCharset common.UB2
		expectedSvrFlags     byte
		expectedCompile      []byte
		expectedRuntime      []byte
		expectedSvrInfoAvail bool
		expectedCharSetElem  common.UB2
	}{
		{
			name:                 "Success",
			payload:              makeTTIproSuccessPayload(6, true, true),
			expectedSvrCharSet:   871,
			expectedNCharCharset: 775,
			expectedSvrFlags:     1,
			expectedCompile:      []byte{1, 2, 3, 4, 5},
			expectedRuntime:      []byte{6, 7, 8},
			expectedSvrInfoAvail: true,
		},
		{
			name:                 "VerLessThan5",
			payload:              append(append(append([]byte{4, 0}, 0), binary.LittleEndian.AppendUint16([]byte{}, 871)...), 1, 0, 0),
			expectedSvrCharSet:   871,
			expectedSvrFlags:     1,
			expectedCompile:      nil,
			expectedSvrInfoAvail: true,
			expectedNCharCharset: 0,
		},
		{
			name:                 "VerLessThan6",
			payload:              makeTTIproSuccessPayload(5, true, false),
			expectedSvrCharSet:   871,
			expectedNCharCharset: 775,
			expectedSvrFlags:     1,
			expectedCompile:      nil,
			expectedSvrInfoAvail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTTIpro()
			buf := NewArrayDataBuffer(1024)
			buf.WriteBytesWithContext(context.Background(), tt.payload)
			engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
			unmarshallable, _ := p.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(context.Background(), engine)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			ttipro := p.(*tTIpro)

			if ttipro.svrCharSet != tt.expectedSvrCharSet {
				t.Errorf("svrCharSet = %d, want %d", ttipro.svrCharSet, tt.expectedSvrCharSet)
			}
			if ttipro.nCharCharset != tt.expectedNCharCharset {
				t.Errorf("nCharCharset = %d, want %d", ttipro.nCharCharset, tt.expectedNCharCharset)
			}
			if ttipro.svrFlags != tt.expectedSvrFlags {
				t.Errorf("svrFlags = %d, want %d", ttipro.svrFlags, tt.expectedSvrFlags)
			}
			if tt.expectedCompile != nil && (ttipro.serverCaps == nil || !bytes.Equal(ttipro.serverCaps.compileTimeCapabilities, tt.expectedCompile)) {
				t.Errorf("compile = %v, want %v", ttipro.serverCaps.compileTimeCapabilities, tt.expectedCompile)
			}
			if tt.expectedRuntime != nil && (ttipro.serverCaps == nil || !bytes.Equal(ttipro.serverCaps.runTimeCapabilities, tt.expectedRuntime)) {
				t.Errorf("runtime = %v, want %v", ttipro.serverCaps.runTimeCapabilities, tt.expectedRuntime)
			}
			if tt.expectedCharSetElem != 0 && ttipro.svrCharSetElem != tt.expectedCharSetElem {
				t.Errorf("svrCharSetElem = %d, want %d", ttipro.svrCharSetElem, tt.expectedCharSetElem)
			}
		})
	}
}

func ttiProFailurePayload(tc string) []byte {
	switch tc {
	case "InvalidVersion":
		return append([]byte{}, 3)
	case "CharSetElemPresentButTooShort":
		buf := []byte{6, 0}
		buf = append(buf, 0)
		buf = binary.BigEndian.AppendUint16(buf, 871)
		buf = append(buf, 1)
		buf = binary.BigEndian.AppendUint16(buf, 2)
		// not enough bytes for 2*5=10 char set elements
		buf = append(buf, []byte{1, 2, 3, 4, 5}...)
		return buf
	case "FDOZeroLength":
		buf := []byte{6, 0}
		buf = append(buf, make([]byte, 50)...)
		buf = binary.BigEndian.AppendUint16(buf, 871)
		buf = append(buf, 1)
		buf = binary.BigEndian.AppendUint16(buf, 0)
		// Protocol >= 5, so FDO length is next, set to 0
		buf = binary.BigEndian.AppendUint16(buf, 0)
		return buf
	case "FDOInvalidOffset":
		buf := []byte{6, 0}
		desc := []byte("test")
		buf = append(buf, desc...)
		buf = append(buf, 0)
		buf = binary.BigEndian.AppendUint16(buf, 871) // svrCharSet
		buf = append(buf, 1)                          // svrFlags
		buf = binary.BigEndian.AppendUint16(buf, 0)   // svrCharSetElem
		buf = binary.BigEndian.AppendUint16(buf, 11)  // FDO Length (11 bytes)
		fdo := make([]byte, 11)
		fdo[5] = 6
		fdo[6] = 1
		// i = 6 + 6 + 1 = 13. i+4 = 17 > 11 (len(fdo)), triggers the error
		buf = append(buf, fdo...)
		return buf
	default:
		return nil
	}
}

func TestTTIproUnmarshalFrom_FailInvalidData(t *testing.T) {
	t.Parallel()
	failures := []struct {
		name        string
		payload     []byte
		expectedErr string
	}{
		{name: "InvalidVersion", expectedErr: "Failed to unmarshal message: Protocol message"},
		{name: "CharSetElemPresentButTooShort", expectedErr: "EOF"},
		{name: "FDOZeroLength", expectedErr: "Failed to unmarshal message: Protocol message"},
		{name: "FDOInvalidOffset", expectedErr: "Failed to unmarshal message: Protocol message"},
	}

	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			p := newTTIpro()
			payload := ttiProFailurePayload(tt.name)
			buf := NewArrayDataBuffer(1024)
			buf.WriteBytesWithContext(context.Background(), payload)
			engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
			unmarshallable, _ := p.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(context.Background(), engine)
			if err == nil {
				t.Errorf("expected error but got none")
				return
			}
			if !regexp.MustCompile(tt.expectedErr).MatchString(err.Error()) {
				t.Errorf("expected error to match pattern %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestTTIproUnmarshalFrom_FailUnmarshal(t *testing.T) {
	t.Parallel()
	readErrs := []struct {
		name           string
		failByteCount  int
		failBytesCount int
		wantErr        string
	}{
		{"Error Reading Protocol Version", 1, 0, "simulated read error"},
		{"Error Reading reserved skip byte", 2, 0, "simulated read error"},
		{"Error Reading server port description", 3, 0, "simulated read error"},
		{"Error Reading server character set", 0, 1, "simulated read error"},
		{"Error Reading server flags", 8, 0, "simulated read error"},
		{"Error Reading server character set element", 0, 2, "simulated read error"},
		{"Error Reading fdo length", 0, 3, "simulated read error"},
		{"Error Reading fdo data", 0, 4, "simulated read error"},
		{"Error Reading server caps", 9, 0, "simulated read error"},
	}

	for _, rerr := range readErrs {
		t.Run(rerr.name, func(t *testing.T) {
			normalPayload := makeTTIproSuccessPayload(6, true, true)
			p := newTTIpro()
			buf := NewArrayDataBuffer(1024)
			buf.WriteBytesWithContext(context.Background(), normalPayload)
			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: buf,
				FailOnReadByteCall:   rerr.failByteCount,
				FailOnReadBytesCall:  rerr.failBytesCount,
			}
			engine := NewNativeMarshalEngine(faulty, common.BIG_ENDIAN)
			unmarshallable, _ := p.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(context.Background(), engine)
			if err == nil || !regexp.MustCompile(rerr.wantErr).MatchString(err.Error()) {
				t.Errorf("expected error %q but got %v", rerr.wantErr, err)
			}
		})
	}
}
