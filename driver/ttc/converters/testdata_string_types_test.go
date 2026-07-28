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

package converters

import (
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

// assertEqBytes is a small helper to compare two B1Array values with a
// helpful failure message showing both buffers.
func assertEqBytes(t *testing.T, got, exp common.B1Array) {
	t.Helper()
	if len(got) != len(exp) {
		t.Fatalf("length mismatch: got %d, want %d (got=% X exp=% X)", len(got), len(exp), []byte(got), []byte(exp))
	}
	for i := range got {
		if got[i] != exp[i] {
			t.Fatalf("byte mismatch at %d: got=% X exp=% X", i, []byte(got), []byte(exp))
		}
	}
}

// Shared test data rows for the different Oracle string types we support.
// Keeping them in one place avoids duplication across type-specific test files.

// CHAR(5)
var charRows = []struct {
	val    string
	expEnc common.B1Array
	expDec common.B1Array // padded to width 5
}{
	{"A1", common.B1Array{0x41, 0x31}, common.B1Array{0x41, 0x31, 0x20, 0x20, 0x20}},
	{"B2", common.B1Array{0x42, 0x32}, common.B1Array{0x42, 0x32, 0x20, 0x20, 0x20}},
	{"C3", common.B1Array{0x43, 0x33}, common.B1Array{0x43, 0x33, 0x20, 0x20, 0x20}},
	{"D4", common.B1Array{0x44, 0x34}, common.B1Array{0x44, 0x34, 0x20, 0x20, 0x20}},
	{"E5", common.B1Array{0x45, 0x35}, common.B1Array{0x45, 0x35, 0x20, 0x20, 0x20}},
	{"F6", common.B1Array{0x46, 0x36}, common.B1Array{0x46, 0x36, 0x20, 0x20, 0x20}},
	{"G7", common.B1Array{0x47, 0x37}, common.B1Array{0x47, 0x37, 0x20, 0x20, 0x20}},
}

// VARCHAR2(20)
var varcharRows = []struct {
	val    string
	expEnc common.B1Array
}{
	{"John", common.B1Array{0x4A, 0x6F, 0x68, 0x6E}},
	{"Alexander", common.B1Array{0x41, 0x6C, 0x65, 0x78, 0x61, 0x6E, 0x64, 0x65, 0x72}},
	{"Maria", common.B1Array{0x4D, 0x61, 0x72, 0x69, 0x61}},
	{"Al", common.B1Array{0x41, 0x6C}},
	{"Takashi", common.B1Array{0x54, 0x61, 0x6B, 0x61, 0x73, 0x68, 0x69}},
	{"👍👋", common.B1Array{0xF0, 0x9F, 0x91, 0x8D, 0xF0, 0x9F, 0x91, 0x8B}},
	{"Tommy", common.B1Array{0x54, 0x6F, 0x6D, 0x6D, 0x79}},
}

// NVARCHAR2(20)
var nvarcharRows = []struct {
	val    string
	expEnc common.B1Array
	expDec common.B1Array
}{
	{"山田太郎", common.B1Array{0xE5, 0xB1, 0xB1, 0xE7, 0x94, 0xB0, 0xE5, 0xA4, 0xAA, 0xE9, 0x83, 0x8E}, common.B1Array{0x5C, 0x71, 0x75, 0x30, 0x59, 0x2A, 0x90, 0xCE}},
	{"李小龙", common.B1Array{0xE6, 0x9D, 0x8E, 0xE5, 0xB0, 0x8F, 0xE9, 0xBE, 0x99}, common.B1Array{0x67, 0x4E, 0x5C, 0x0F, 0x9F, 0x99}},
	{"北京市", common.B1Array{0xE5, 0x8C, 0x97, 0xE4, 0xBA, 0xAC, 0xE5, 0xB8, 0x82}, common.B1Array{0x53, 0x17, 0x4E, 0xAC, 0x5E, 0x02}},
	{"خالد", common.B1Array{0xD8, 0xAE, 0xD8, 0xA7, 0xD9, 0x84, 0xD8, 0xAF}, common.B1Array{0x06, 0x2E, 0x06, 0x27, 0x06, 0x44, 0x06, 0x2F}},
	{"日本", common.B1Array{0xE6, 0x97, 0xA5, 0xE6, 0x9C, 0xAC}, common.B1Array{0x65, 0xE5, 0x67, 0x2C}},
	{"👨‍💻🖥️", common.B1Array{0xF0, 0x9F, 0x91, 0xA8, 0xE2, 0x80, 0x8D, 0xF0, 0x9F, 0x92, 0xBB, 0xF0, 0x9F, 0x96, 0xA5, 0xEF, 0xB8, 0x8F}, common.B1Array{0xD8, 0x3D, 0xDC, 0x68, 0x20, 0x0D, 0xD8, 0x3D, 0xDC, 0xBB, 0xD8, 0x3D, 0xDD, 0xA5, 0xFE, 0x0F}},
	{"محمد", common.B1Array{0xD9, 0x85, 0xD8, 0xAD, 0xD9, 0x85, 0xD8, 0xAF}, common.B1Array{0x06, 0x45, 0x06, 0x2D, 0x06, 0x45, 0x06, 0x2F}},
}

// NCHAR(3)
var ncharRows = []struct {
	val          string
	expEnc       common.B1Array
	expDecPadded common.B1Array
}{
	{"US", common.B1Array{0x55, 0x53}, common.B1Array{0x00, 0x55, 0x00, 0x53, 0x00, 0x20}},
	{"IN", common.B1Array{0x49, 0x4E}, common.B1Array{0x00, 0x49, 0x00, 0x4E, 0x00, 0x20}},
	{"CN", common.B1Array{0x43, 0x4E}, common.B1Array{0x00, 0x43, 0x00, 0x4E, 0x00, 0x20}},
	{"AE", common.B1Array{0x41, 0x45}, common.B1Array{0x00, 0x41, 0x00, 0x45, 0x00, 0x20}},
	{"JP", common.B1Array{0x4A, 0x50}, common.B1Array{0x00, 0x4A, 0x00, 0x50, 0x00, 0x20}},
	{"FR", common.B1Array{0x46, 0x52}, common.B1Array{0x00, 0x46, 0x00, 0x52, 0x00, 0x20}},
	{"EG", common.B1Array{0x45, 0x47}, common.B1Array{0x00, 0x45, 0x00, 0x47, 0x00, 0x20}},
}
