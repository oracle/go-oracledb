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

package common

import (
	"testing"

	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

func TestShelf_ConnectionProperties(t *testing.T) {
	shelf := NewShelf[int]()

	props := &oracleconfig.OracleDriverProperties{StrictNullValueHandling: true, DefaultLobPrefetchSize: 1024}
	if returned := shelf.UpdateConnectionProperties(props); returned != shelf {
		t.Fatal("UpdateConnectionProperties should return the shelf")
	}

	if got := shelf.GetConnectionProperties(); got != props {
		t.Fatalf("GetConnectionProperties = %p, want %p", got, props)
	}
}

func TestUtility_B1ArrayToString(t *testing.T) {
	tests := []struct {
		name string
		in   B1Array
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "empty", in: B1Array{}, want: ""},
		{name: "ascii", in: B1Array("hello"), want: "hello"},
		{name: "skips trailing null", in: B1Array{'a', 0}, want: "a"},
		{name: "invalid utf8 replacement", in: B1Array{0xff}, want: "�"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := B1ArrayToString(tt.in); got != tt.want {
				t.Fatalf("B1ArrayToString(%v) = %q, want %q", []byte(tt.in), got, tt.want)
			}
			if got := tt.in.String(); got != tt.want {
				t.Fatalf("B1Array.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUtility_GenerateRandomBytes(t *testing.T) {
	buf := make([]byte, 8)
	if err := GenerateRandomBytes(buf); err != nil {
		t.Fatalf("GenerateRandomBytes returned error: %v", err)
	}
	if len(buf) != 8 {
		t.Fatalf("buffer length changed: got %d, want 8", len(buf))
	}
}
