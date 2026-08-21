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
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestTTIdtyNew(t *testing.T) {
	t.Parallel()
	msg := NewTTIdty()
	dty, ok := msg.(*tTIdty)
	if !ok {
		t.Fatalf("expected *tTIdty, got %T", msg)
	}
	expectedCliRIN := al32Utf8CharSet
	expectedCliROUT := al32Utf8CharSet
	expectedCliFlags := common.UB1(0)
	if dty.cliRIN != expectedCliRIN || dty.cliROUT != expectedCliROUT {
		t.Errorf("unexpected tTIdty default field values: got cliRIN=%v, cliROUT=%v, want cliRIN=%v, cliROUT=%v, cliFlags=%v",
			dty.cliRIN, dty.cliROUT, expectedCliRIN, expectedCliROUT, expectedCliFlags)
	}
}

func TestTTIdtyMarshalTo_Success(t *testing.T) {
	t.Parallel()
	const (
		cliRIN  = 0x1234
		cliROUT = 0x5678
	)
	// Initialize with the correct sizes
	caps := newDefaultCapability()
	// Overwrite a few known bytes (at the front of the slices) for test
	caps.compileTimeCapabilities[0] = 0xDE
	caps.runTimeCapabilities[0] = 0xAC

	dty := &tTIdty{
		cliRIN:         common.UB2(cliRIN),
		cliROUT:        common.UB2(cliROUT),
		negotiatedCaps: caps,
	}

	tdb := NewArrayDataBuffer(16384)
	mar := NewNativeMarshalEngine(tdb, common.BIG_ENDIAN)

	err := dty.MarshalTo(context.Background(), mar)
	written := tdb.bytes[:tdb.currentWritePosition]
	if err != nil {
		t.Fatalf("MarshalTo unexpected error: %v", err)
	}

	// Build expected output up to the cap slices
	expected := []byte{
		0x34, 0x12, // cliRIN swapped
		0x78, 0x56, // cliROUT swapped
		0x01,                                    // cflag
		byte(len(caps.compileTimeCapabilities)), // length
	}
	expected = append(expected, caps.compileTimeCapabilities...)
	expected = append(expected, byte(len(caps.runTimeCapabilities)))
	expected = append(expected, caps.runTimeCapabilities...)

	// Only compare the capability preamble and not trailing typeRepresentationTable output (which is out of our direct control here)
	cmpLen := len(expected)
	if len(written) < cmpLen {
		t.Fatalf("Too few bytes marshaled: got %d, want at least %d", len(written), cmpLen)
	}
	if !bytes.Equal(written[:cmpLen], expected) {
		t.Errorf("MarshalTo mismatch\n got: % X\nwant: % X", written[:cmpLen], expected)
	}
}

func newTTIdtyWithCaps() *tTIdty {
	caps := newCapability()
	ttidty := NewTTIdty().(*tTIdty)
	ttidty.SetNegotiatedCapabilities(caps)
	ttidty.cliRIN = al32Utf8CharSet
	ttidty.cliROUT = al32Utf8CharSet
	return ttidty
}

func TestTTIdtyMarshalTo_Fail(t *testing.T) {
	t.Parallel()
	type FaultyConfig struct {
		failWriteOn      int
		failWriteBytesOn int
	}
	tests := []struct {
		name        string
		expectedErr string
		faulty      *FaultyConfig
	}{
		{"NoCapabilities", "Data type message", nil},
		{"FailMessageTypeWriteByte", "simulated write error (WriteByte)", &FaultyConfig{failWriteOn: 1}},
		{"FailCliRINWriteUB2", "simulated write error (WriteBytes)", &FaultyConfig{failWriteBytesOn: 1}},
		{"FailCliROUTWriteUB2", "simulated write error (WriteBytes)", &FaultyConfig{failWriteBytesOn: 2}},
		{"FailCliFlagsWriteByte", "simulated write error (WriteByte)", &FaultyConfig{failWriteOn: 2}},
		{"FailClientCapsWriteByte", "simulated write error (WriteBytes)", &FaultyConfig{failWriteBytesOn: 4}},
		{"FailTZWriteBytes", "simulated write error (WriteBytes)", &FaultyConfig{failWriteBytesOn: 5}},
		{"FailTT3CapsWriteByte", "simulated write error (WriteBytes)", &FaultyConfig{failWriteBytesOn: 6}},
	}

	newFaultyBuffer := func(f *FaultyConfig) *FaultyArrayBasedDataBuffer {
		return &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(1024),
			FailOnWriteByteCall:  f.failWriteOn,
			FailOnWriteBytesCall: f.failWriteBytesOn,
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				mar *MarshalEngine
				obj *tTIdty
			)
			if tt.name == "NoCapabilities" {
				obj = NewTTIdty().(*tTIdty)
			} else {
				obj = newTTIdtyWithCaps()
			}

			if tt.faulty != nil {
				mar = NewNativeMarshalEngine(newFaultyBuffer(tt.faulty), common.BIG_ENDIAN)
			} else {
				tdb := NewArrayDataBuffer(8192)
				mar = NewNativeMarshalEngine(tdb, common.BIG_ENDIAN)
			}
			err := obj.MarshalTo(context.Background(), mar)
			if tt.expectedErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("expected error containing %q, got %v", tt.expectedErr, err)
					if err != nil {
						t.Logf("Actual error string: %#v", err.Error())
					}
				}
			} else if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.name, err)
			}
		})
	}
}

func TestTTIdtyGetters(t *testing.T) {
	t.Parallel()
	// Use real capability initialization to prevent slice out of range errors
	caps := newCapability()
	const ttcVer byte = 42
	index := caps.knownUsedCompileTimeCapabilities[kpccapCtTtcFldVsn].index
	caps.compileTimeCapabilities[index] = ttcVer
	// Optional: set runtimeCapabilities if you want, but not strictly needed here
	dty := &tTIdty{
		cliRIN:                0x1111,
		cliROUT:               0x2222,
		negotiatedCaps:        caps,
		negotiatedCapsMap:     caps.toMap(),
		timeZoneVersionNumber: 77,
	}

	if got := dty.GetMsgCode(); got != TTIDTY {
		t.Errorf("GetMsgCode = %v, want %v", got, TTIDTY)
	}
	if got := dty.GetClientTTCVersion(); got != ttcVer {
		t.Errorf("GetClientTTCVersion = %v, want %v", got, ttcVer)
	}
	if got := dty.GetTimeZoneVersionNumber(); got != 77 {
		t.Errorf("GetTimeZoneVersionNumber = %v, want 77", got)
	}
}

func TestTTIdtyUnmarshalFrom_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buf := NewArrayDataBuffer(16)
	u := &tTIdty{}
	cap := newCapability()
	// This test only works with kpccapRtTzEx not set
	cap.runTimeCapabilities[cap.knownUsedRuntimeCapabilities[kpccapRtTzEx].index] = 0
	u.SetNegotiatedCapabilities(cap) // Ensure capabilities are set before unmarshalling
	// Provide a minimal valid typerep section: UB2(0x0001), UB2(0x0000), UB2(0x0000)
	// (type block start, end block, end struct)
	testBytes := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	_ = buf.WriteBytesWithContext(ctx, testBytes)

	mar := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)

	err := u.UnMarshalFrom(ctx, mar)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
}

func TestTTIdtyUnmarshalFrom_Failure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name        string
		setCaps     bool
		expectedErr string
	}{
		{
			name:        "capabilities not set",
			setCaps:     false,
			expectedErr: "Failed to unmarshal message: Data type message",
		},
		{
			name:        "typerep unmarshal fail",
			setCaps:     true,
			expectedErr: "Failed to unmarshal message: Data type message",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &tTIdty{}
			if tc.setCaps {
				u.SetNegotiatedCapabilities(newCapability())
			}

			var buf common.DataBuffer

			if tc.name == "typerep unmarshal fail" {
				payload := makeDtyValidPayload()
				baseBuf := NewArrayDataBuffer(len(payload))
				_ = baseBuf.WriteBytesWithContext(ctx, payload)
				// Inject failure on first read bytes call (or tune as needed)
				buf = &FaultyArrayBasedDataBuffer{
					ArrayBasedDataBuffer: baseBuf,
					FailOnReadBytesCall:  3,
				}
			} else {
				// Default behavior (capabilities not set)
				buf = NewArrayDataBuffer(4)
				_ = buf.WriteBytesWithContext(ctx, []byte{0x02, 0x00})
			}

			mar := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)

			err := u.UnMarshalFrom(ctx, mar)
			if err == nil {
				t.Fatalf("[%s] Expected error matching %q, got nil", tc.name, tc.expectedErr)
			}
			matched, matchErr := regexp.MatchString(tc.expectedErr, err.Error())
			if matchErr != nil {
				t.Fatalf("[%s] Failed to compile regexp: %v", tc.name, matchErr)
			}
			if !matched {
				t.Fatalf("[%s] Expected error matching %q, got %v", tc.name, tc.expectedErr, err)
			}
		})
	}
}
