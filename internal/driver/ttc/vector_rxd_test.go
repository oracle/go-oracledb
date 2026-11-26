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
** is contributed to or provided by such licensors),
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

// TestUnmarshalVectorColumn verifies VECTOR prefetch framing leaves the full payload available.
func TestUnmarshalVectorColumn(t *testing.T) {
	ctx := context.Background()
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 256)
	locator := driverCommon.B1Array(bytes.Repeat([]byte{0x5A}, 9))
	prefetched := driverCommon.B1Array{1, 2, 3}
	if err := writer.MarshalUB4(ctx, driverCommon.UB4(len(locator))); err != nil {
		t.Fatalf("marshal locator length: %v", err)
	}
	if err := writer.MarshalUB8(ctx, driverCommon.UB8(len(prefetched))); err != nil {
		t.Fatalf("marshal vector length: %v", err)
	}
	if err := writer.MarshalUB4(ctx, driverCommon.UB4(len(prefetched))); err != nil {
		t.Fatalf("marshal prefetch chunk size: %v", err)
	}
	if err := writer.MarshalCLR(ctx, prefetched, 0, len(prefetched)); err != nil {
		t.Fatalf("marshal prefetched data: %v", err)
	}
	if err := writer.MarshalCLR(ctx, locator, 0, len(locator)); err != nil {
		t.Fatalf("marshal locator: %v", err)
	}

	reader := createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0)
	payload, err := unwrapVectorColumn(ctx, reader, columnUnwrapContext{})
	if err != nil {
		t.Fatalf("unmarshal vector column: %v", err)
	}
	got, ok := payload.data.(driverCommon.B1Array)
	if !ok || !bytes.Equal(got, prefetched) {
		t.Fatalf("prefetched data mismatch: got %v want %v", payload.data, prefetched)
	}
}
