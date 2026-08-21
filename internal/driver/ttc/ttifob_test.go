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

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestTTIFOB_MessageStreamerFlushesOnlyMessageCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buf := NewArrayDataBuffer(8)
	mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(mar)
	streamer := NewMessageStreamer(shelf)

	msg := newTTIfob()
	if msg.GetMsgCode() != TTIFOB {
		t.Fatalf("GetMsgCode mismatch: got %v want %v", msg.GetMsgCode(), TTIFOB)
	}
	if err := streamer.Push(ctx, msg); err != nil {
		t.Fatalf("Push TTIFOB failed: %v", err)
	}
	if err := streamer.Flush(ctx); err != nil {
		t.Fatalf("Flush TTIFOB failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	want := []byte{byte(TTIFOB)}
	if !bytes.Equal(got, want) {
		t.Fatalf("TTIFOB wire mismatch: got % X want % X", got, want)
	}
}
