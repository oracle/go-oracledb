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
)

// Helper functions for capabilities test data
func testCompileCaps() []byte {
	compile := make([]byte, 38)
	compile[27] = 0
	compile[37] = 0
	return compile
}
func testRunCaps() []byte {
	return []byte{2, 1, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0}
}
func testErrorCaps() []byte {
	return []byte{1, 2, 3}
}
func makeTestPayload(compile, run []byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(byte(len(compile)))
	buf.Write(compile)
	if run != nil {
		buf.WriteByte(byte(len(run)))
		buf.Write(run)
	}
	return buf.Bytes()
}

// TestCapabilityNew verifies that newCapability returns a non-nil struct and both caps are nil.
func TestCapabilityNew(t *testing.T) {
	t.Parallel()
	cap := newCapability()
	if cap == nil {
		t.Fatal("Expected NewCapability to return non-nil Capability")
	}
	if !bytes.Equal(cap.runTimeCapabilities, assumedSrvRtCaps) {
		t.Errorf("runTimeCapabilities: got %v, want %v", cap.runTimeCapabilities, assumedSrvRtCaps)
	}
	if !bytes.Equal(cap.compileTimeCapabilities, assumedSrvCtCaps) {
		t.Errorf("compileTimeCapabilities: got %v, want %v", cap.compileTimeCapabilities, assumedSrvCtCaps)
	}
}

// TestCapabilityNewDefault verifies that newDefaultCapability returns a non-nil, initialized capability object.
func TestCapabilityNewDefault(t *testing.T) {
	t.Parallel()
	cap := newDefaultCapability()
	if cap == nil {
		t.Fatal("Expected NewCapability to return non-nil Capability")
	}
	if cap.compileTimeCapabilities == nil || cap.runTimeCapabilities == nil {
		t.Fatal("Client caps must be initialized")
	}
	if len(cap.compileTimeCapabilities) == 0 || len(cap.runTimeCapabilities) == 0 {
		t.Fatal("Client caps arrays should not be empty")
	}
}

// TestCapabilityMarshalTo_Success checks success path marshaling.
func TestCapabilityMarshalTo_Success(t *testing.T) {
	t.Parallel()
	c := &capability{
		compileTimeCapabilities: []byte{4, 5, 6},
		runTimeCapabilities:     []byte{1, 2, 3},
	}
	expected := []byte{3, 4, 5, 6, 3, 1, 2, 3}
	buf := NewArrayDataBuffer(1024)
	engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)

	err := c.MarshalTo(context.Background(), engine)
	if err != nil {
		t.Fatalf("marshalTo failed: %v", err)
	}
	written := buf.bytes[:buf.currentWritePosition]
	if !bytes.Equal(written, expected) {
		t.Errorf("got %v, want %v", written, expected)
	}
}

// TestCapabilityMarshalTo_Fail checks error marshaling.
func TestCapabilityMarshalTo_Fail(t *testing.T) {
	t.Parallel()
	cap := newDefaultCapability()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
		wantErr   string
	}{
		{"Error on compile time caps length write", 1, 0, "simulated write error"},
		{"Error on compile time caps write", 0, 1, "simulated write error"},
		{"Error on run time caps length write", 2, 0, "simulated write error"},
		{"Error on run time caps write", 0, 2, "simulated write error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(1024),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
			err := cap.MarshalTo(context.Background(), engine)
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

// TestCapabilityUnMarshalFrom_Success checks success path unmarshal.
func TestCapabilityUnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	payload := makeTestPayload(testCompileCaps(), testRunCaps())
	buf := NewArrayDataBuffer(1024)
	buf.WriteBytesWithContext(context.Background(), payload)

	cap := newCapability()
	engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
	err := cap.UnMarshalFrom(context.Background(), engine)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !bytes.Equal(cap.compileTimeCapabilities, testCompileCaps()) {
		t.Errorf("server compile time capabilities mismatch: got %v, want %v", cap.compileTimeCapabilities, testCompileCaps())
	}
	if !bytes.Equal(cap.runTimeCapabilities, testRunCaps()) {
		t.Errorf("server run time capabilities mismatch: got %v, want %v", cap.runTimeCapabilities, testRunCaps())
	}
}

// TestCapabilityUnMarshalFrom_Fail checks error unmarshal.
func TestCapabilityUnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	errorCases := []struct {
		name    string
		failPos int
	}{
		{"Error Reading Compile time length", 1},
		{"Error Reading Compile time Caps", 2},
		{"Error Reading Runtime length", 5},
		{"Error Reading Runtime Caps", 6},
	}
	for _, ec := range errorCases {
		t.Run(ec.name, func(t *testing.T) {
			var buf common.DataBuffer
			payload := makeTestPayload(testErrorCaps(), testErrorCaps())
			dbuf := NewArrayDataBuffer(10)
			dbuf.WriteBytesWithContext(context.Background(), payload)
			buf = &FaultyArrayBasedDataBuffer{ArrayBasedDataBuffer: dbuf, FailOnReadByteCall: ec.failPos}
			cap := newCapability()
			engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
			err := cap.UnMarshalFrom(context.Background(), engine)
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

// TestCapability_AdjustCapabilityFrom verifies that AdjustCapabilityFrom mutates client caps based on server caps as specified.
func TestCapability_AdjustCapabilityFrom(t *testing.T) {
	t.Parallel()
	// Set up minimal lengths for relevant indices
	client := &capability{
		compileTimeCapabilities: make([]byte, 50),
		runTimeCapabilities:     make([]byte, 10),
	}
	server := &capability{
		compileTimeCapabilities: make([]byte, 50),
		runTimeCapabilities:     make([]byte, 10),
	}

	// Case 1: UB2Dty zero disables client UB2Dty
	index := client.knownUsedCompileTimeCapabilities[kpccapCtUb2dty].index
	server.compileTimeCapabilities[index] = 0x00
	client.compileTimeCapabilities[index] = 0xFF // pre-set nonzero

	// Case 2: RunTimeCaps Zcpy bit gets enabled
	index = client.knownUsedRuntimeCapabilities[kpccapRtbTtc1Iovoff].index
	client.runTimeCapabilities[index] = 0x00

	// Case 3: RunTimeCaps TzEx gets disabled if server does not have TzEx
	kpccapRtTz := client.knownUsedRuntimeCapabilities[kpccapRtTzEx]
	server.runTimeCapabilities = nil
	client.runTimeCapabilities[kpccapRtTz.index] = kpccapRtTz.value

	// Case 4: 32K bit is always enabled
	kpccapRtbTtc := client.knownUsedRuntimeCapabilities[kpccapRtbTtc32k]
	client.runTimeCapabilities[kpccapRtbTtc.index] &^= kpccapRtbTtc.value // ensure 32K bit is off before

	// Case 5: compileTimeCapabilities[kpccapCtbTtc3] and runTimeCapabilities[kpccapRtTz] are masked if mismatch
	kpccapCtbTtc3 := client.knownUsedCompileTimeCapabilities[kpccapCtbTtc3Tzver]
	server.compileTimeCapabilities[index] = 0x00
	client.compileTimeCapabilities[index] = kpccapCtbTtc3.value
	client.runTimeCapabilities[kpccapRtTz.index] = kpccapRtTz.value

	client.adjustCapabilityFrom(server)

	kpccapCtUb2dty := client.knownUsedCompileTimeCapabilities[kpccapCtUb2dty]
	if client.compileTimeCapabilities[index] != 0 {
		t.Errorf("compileTimeCapabilities[kpccapCtUb2Dty]: got %d, want 0", client.compileTimeCapabilities[kpccapCtUb2dty.index])
	}

	if (client.runTimeCapabilities[kpccapRtbTtc.index] & kpccapRtbTtc.value) != kpccapRtbTtc.value {
		t.Errorf("runTimeCapabilities[kpccapRtbTtc] 32K bit not set")
	}

	if (client.runTimeCapabilities[kpccapRtTz.index] & kpccapRtTz.value) != 0 {
		t.Errorf("runTimeCapabilities[kpccapRtTz] TzEx bit should be cleared")
	}

	if (client.compileTimeCapabilities[kpccapCtbTtc3.index] & client.knownUsedCompileTimeCapabilities[kpccapCtbTtc3Tzver].value) != 0 {
		t.Errorf("compileTimeCapabilities[kpccapCtbTtc3] Tzver bit should be cleared")
	}
}

func TestCapability_toMap(t *testing.T) {
	t.Parallel()
	capabilities := newDefaultCapability()
	capabilitiesMap := capabilities.toMap()
	for key, value := range capabilities.knownUsedCompileTimeCapabilities {
		if value.isDefault {
			if capabilitiesMap[key].Value != value.value {
				t.Fatalf("Invalid capability value in Map for %s, expected %x but was %x", key, value.value, capabilitiesMap[key].Value)
			}
			if value.value != 0 && !capabilitiesMap[key].IsSet {
				t.Fatalf("Expected capability to be set in Map for %s", key)
			}
		} else {
			if capabilitiesMap[key].Value != 0 {
				t.Fatalf("Invalid capability value in Map for %s, expected %x but was %x", key, 0, capabilitiesMap[key].Value)
			}
			if value.value != 0 && capabilitiesMap[key].IsSet {
				t.Fatalf("Expected capability not to be set in Map for %s", key)
			}
		}
	}

	for key, value := range capabilities.knownUsedRuntimeCapabilities {
		if value.isDefault {
			if capabilitiesMap[key].Value != value.value {
				t.Fatalf("Invalid capability value in Map for %s, expected %x but was %x", key, value.value, capabilitiesMap[key].Value)
			}
			if value.value != 0 && !capabilitiesMap[key].IsSet {
				t.Fatalf("Expected capability to be set in Map for %s", key)
			}
		} else {
			if capabilitiesMap[key].Value != 0 {
				t.Fatalf("Invalid capability value in Map for %s, expected %x but was %x", key, 0, capabilitiesMap[key].Value)
			}
			if value.value != 0 && capabilitiesMap[key].IsSet {
				t.Fatalf("Expected capability not to be set in Map for %s", key)
			}
		}
	}
}
