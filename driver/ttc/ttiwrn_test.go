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
)

func TestTTIwrnUnMarshalFrom(t *testing.T) {
	t.Parallel()
	warning := newTTIwrn().(*tTIwrn)
	payload := []byte{
		0x02, 0x6d, 0x62, // wrnnum = 28002
		0x01, 0x05, // wrnlen = 5
		0x01, 0x05, // wrnflg = KPWRN_FIRST | KPWRN_LAST
		'h', 'e', 'l', 'l', 'o',
	}

	if err := warning.UnMarshalFrom(context.Background(), createMarshaller(payload, 0, 0)); err != nil {
		t.Fatalf("UnMarshalFrom returned error: %v", err)
	}
	if warning.GetMsgCode() != TTIWRN {
		t.Errorf("GetMsgCode = %v, want %v", warning.GetMsgCode(), TTIWRN)
	}
	if warning.warningNumber != 28002 {
		t.Errorf("warningNumber = %d, want 28002", warning.warningNumber)
	}
	if got := warning.warningMessage; got != "hello" {
		t.Errorf("warningMessage = %q, want %q", got, "hello")
	}
	wantFlags := kpwrnFirst | kpwrnLast
	if warning.warningFlags != wantFlags {
		t.Errorf("warningFlags = %#x, want %#x", warning.warningFlags, wantFlags)
	}
}

func TestTTIwrnUnMarshalFromRejectsTruncatedMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "warning number", payload: nil},
		{name: "warning length", payload: []byte{0x01, 0x01}},
		{name: "warning flags", payload: []byte{0x01, 0x01, 0x01, 0x01}},
		{name: "warning message", payload: []byte{0x01, 0x01, 0x01, 0x05, 0x01, 0x01, 'x'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning := newTTIwrn().(*tTIwrn)
			if err := warning.UnMarshalFrom(context.Background(), createMarshaller(tt.payload, 0, 0)); err == nil {
				t.Fatal("UnMarshalFrom returned nil error for truncated message")
			}
		})
	}
}
