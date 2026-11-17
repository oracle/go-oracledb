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
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestTTIInitMarshalTo verifies the server-conversion flags, embedded PRO,
// unused charset fields, and initial field version in the outgoing message.
func TestTTIInitMarshalTo(t *testing.T) {
	t.Parallel()
	buffer := NewArrayDataBuffer(256)
	marshaller := NewNativeMarshalEngine(buffer, driverCommon.BIG_ENDIAN)
	message := &ttiInitMessage{
		_protocolMessage: newTTIpro().(*tTIpro),
		_fieldVersion:    ttcFldVsn234,
	}
	want := []byte{1, 1, 0, byte(TTIPRO)}    // Envelope version, flags, embedded PRO code.
	want = append(want, 6, 5, 4, 3, 2, 1, 0) // Supported protocol versions.
	want = append(want, []byte("GO_TTC-8.2.0\x00")...)
	want = append(want, 0, 0, 0, 0, 0)
	want = append(want, ttcFldVsn234)

	err := message.MarshalTo(context.Background(), marshaller)

	if err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}
	if message.GetMsgCode() != TTIINIT {
		t.Fatalf("message code = %v, want %v", message.GetMsgCode(), TTIINIT)
	}
	if got := buffer.bytes[:buffer.currentWritePosition]; !bytes.Equal(got, want) {
		t.Fatalf("TTIINIT body = % X, want % X", got, want)
	}
}

// TestTTIInitMarshalToFail verifies that envelope and embedded PRO write failures are returned.
func TestTTIInitMarshalToFail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		failByte  int
		failBytes int
	}{
		{name: "envelope version", failByte: 1},
		{name: "embedded protocol", failBytes: 1},
		{name: "database charset", failBytes: 3},
		{name: "initial field version", failByte: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buffer := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(256),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			marshaller := NewNativeMarshalEngine(buffer, driverCommon.BIG_ENDIAN)
			message := &ttiInitMessage{
				_protocolMessage: newTTIpro().(*tTIpro),
				_fieldVersion:    ttcFldVsn234,
			}

			err := message.MarshalTo(context.Background(), marshaller)

			if err == nil {
				t.Fatal("MarshalTo succeeded after a simulated write failure")
			}
		})
	}
}
