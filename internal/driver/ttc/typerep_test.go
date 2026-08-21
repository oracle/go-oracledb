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
	"regexp"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func TestTypeRep_NewTypeRep(t *testing.T) {
	t.Parallel()
	tr := newTypeRep()
	if tr == nil {
		t.Fatal("newTypeRep returned nil")
	}
	if tr.conversionFlags != 0 || tr.serverConversion {
		t.Errorf("Expected zero conversionFlags and serverConversion false, got %d, %v", tr.conversionFlags, tr.serverConversion)
	}
	expected := []byte{Native, Universal, Universal, Universal, Universal}
	for i, exp := range expected {
		if (tr.nativeTypesRepresentation)[i] != exp {
			t.Errorf("nativeTypesRepresentation[%d] expected %d, got %d", i, exp, (tr.representations)[i])
		}
	}
}

// TestTypeRep_MarshalTo_Fail uses FaultyArrayBasedDataBuffer to simulate write errors
func TestTypeRep_MarshalTo_Fail(t *testing.T) {
	t.Parallel()
	failCases := []struct {
		name       string
		failCall   int
		wantErrMsg string
	}{
		{"UB2_WriteError1", 1, "simulated write error"},
		{"UB2_WriteError2", 3, "simulated write error"},
	}

	for _, fp := range failCases {
		t.Run(fp.name, func(t *testing.T) {
			tr := newTypeRep()
			tr.addTypeRepToTable(0x22, 0x00, 0x00) // add one entry

			buf := NewArrayDataBuffer(16)
			faultyBuf := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: buf,
				FailOnWriteByteCall:  0,
				FailOnWriteBytesCall: 0,
			}
			faultyBuf.FailOnWriteBytesCall = fp.failCall

			engine := NewMarshalEngine(faultyBuf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := tr.MarshalTo(context.Background(), engine)
			assertErrorContains(t, err, fp.wantErrMsg)
		})
	}
}

func TestTypeRep_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		makePayload func() []byte
	}{
		{"Success", makeDtyValidPayload},
	}

	for _, tc := range tests {
		tr := newTypeRep()
		payload := tc.makePayload()
		buf := NewArrayDataBuffer(len(payload))
		_ = buf.WriteBytesWithContext(context.Background(), payload)
		buf.currentReadPosition = 0
		engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
		err := tr.UnMarshalFrom(context.Background(), engine)
		if err != nil {
			t.Fatalf("[%s] UnMarshalFrom returned error: %v", tc.name, err)
		}
	}
}

func TestTypeRep_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		payloadFunc         func() []byte
		failOnReadBytesCall int
	}{
		{
			name:                "TypeBlock_Fail",
			payloadFunc:         makeDtyValidPayload,
			failOnReadBytesCall: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newTypeRep()
			payload := tt.payloadFunc()
			baseBuf := NewArrayDataBuffer(len(payload))
			_ = baseBuf.WriteBytesWithContext(context.Background(), payload)
			baseBuf.currentReadPosition = 0
			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: baseBuf,
				FailOnReadByteCall:   0,
				FailOnReadBytesCall:  tt.failOnReadBytesCall,
			}
			engine := NewNativeMarshalEngine(faulty, common.BIG_ENDIAN)
			err := tr.UnMarshalFrom(context.Background(), engine)
			if err == nil || !regexp.MustCompile("simulated read error").MatchString(err.Error()) {
				t.Errorf("expected 'simulated read error' but got %v", err)
			}
		})
	}
}

func TestTypeRep_UnMarshalFrom_TooManyTypeRepresentations(t *testing.T) {
	t.Parallel()
	tr := newTypeRep()
	payload := make([]byte, 0, int(_maxReceivedReps+1)*4+2)
	for i := int16(0); i < _maxReceivedReps+1; i++ {
		payload = append(payload, 0x00, 0x01) // start of a type block
		payload = append(payload, 0x00, 0x00) // end of the current type block
	}
	payload = append(payload, 0x00, 0x00) // terminal zero

	buf := NewArrayDataBuffer(len(payload))
	_ = buf.WriteBytesWithContext(context.Background(), payload)
	buf.currentReadPosition = 0
	engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)

	err := tr.UnMarshalFrom(context.Background(), engine)
	if err == nil {
		t.Fatal("expected protocol violation error, got nil")
	}

	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected common.SQLError, got %T", err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ProtocolViolationLimitExceeded) {
		t.Fatalf("error code: got %s, want %s", sqlErr.ErrorCode(), oracleErrors.ProtocolViolationLimitExceeded)
	}
}

func TestTypeRep_MarshalTo_Success(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		addReps   []int16 // representations to add
		wantBytes func(reps []int16) []byte
	}{
		{
			name:    "UB2",
			addReps: []int16{0x22, 0x33, 0x44},
			wantBytes: func(reps []int16) []byte {
				// UB2, little-endian, plus UB2(0) terminator
				expected := []byte{}
				for _, val := range reps {
					if val > 0 {
						expected = append(expected, 1, byte(val&0xFF), 0x00)
					}
				}
				expected = append(expected, 0x00)
				return expected
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTypeRep()
			buf := NewArrayDataBuffer(64)
			engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			for _, rep := range tc.addReps {
				tr.addTypeRepToTable(rep, 0x00, 0x00) // use dummy ndty/rep (not used in MarshalTo)
			}
			err := tr.MarshalTo(context.Background(), engine)
			if err != nil {
				t.Fatalf("MarshalTo returned error: %v", err)
			}
			got := buf.bytes[:buf.currentWritePosition]
			expected := tc.wantBytes(tc.addReps)
			if !bytes.Equal(got, expected) {
				t.Errorf("MarshalTo (%s) output does not match expected: got %v want %v", tc.name, got, expected)
			}
		})
	}
}

func TestTypeRep_AddTypeRepToTable_Resize(t *testing.T) {
	t.Parallel()
	tr := newTypeRep()
	origCap := len(tr.representations)

	// Add maximum possible entries until just before resize trigger
	for i := int(tr.representations[0]); len(tr.representations) >= int(tr.representations[0])+4; i++ {
		tr.addTypeRepToTable(int16(i), 0, 0)
	}

	// Capture a copy of representations values before resize (should not have triggered yet)
	origCopy := make([]int16, len(tr.representations))
	copy(origCopy, tr.representations)

	// Now, add one more which SHOULD trigger resize
	tr.addTypeRepToTable(999, 0, 101)

	if len(tr.representations) != origCap*2 {
		t.Errorf("Expected resized representations to double: got %d, want %d", len(tr.representations), origCap*2)
	}
	// Only contents up to old logical length should be identical (rest may be changed by the triggering add)
	preResizeLen := int(origCopy[0])
	for i := 1; i < preResizeLen; i++ {
		if tr.representations[i] != origCopy[i] {
			t.Errorf("Content mismatch after resize at idx %d: got %d, want %d", i, tr.representations[i], origCopy[i])
			break
		}
	}
}

func TestTypeRep_SetRepAndGetRep(t *testing.T) {
	t.Parallel()
	tr := newTypeRep()

	// Valid set
	tr.setRep(B4, Lsb)
	rep := tr.getRep(B4)
	if rep != Lsb {
		t.Errorf("Expected LSB (%d), got %d", Lsb, rep)
	}

}

func TestTypeRep_SetFlagsAndGetFlags(t *testing.T) {
	t.Parallel()
	tr := newTypeRep()

	tr.SetFlags(42)
	if tr.getFlags() != 42 {
		t.Errorf("Expected flags 42, got %d", tr.getFlags())
	}
}

func TestTypeRep_Setters_Getters(t *testing.T) {
	t.Parallel()
	tr := newTypeRep()
	// B2, B4, B8, PTR are Universal by default, B1 is Native by default
	if !tr.isNativeTypeAsUniversal(B2) {
		t.Error("Expected B2 to be Universal (true)")
	}
	if tr.isNativeTypeAsUniversal(B1) {
		t.Error("Expected B1 to NOT be Universal (false)")
	}
	// set B1 to Universal and check again
	tr.setRep(B1, Universal)
	if !tr.isNativeTypeAsUniversal(B1) {
		t.Error("After setRep, expected B1 to be Universal (true)")
	}
}
