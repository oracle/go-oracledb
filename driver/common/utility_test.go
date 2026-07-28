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
)

func TestStripSpacesOutsideQuotes(t *testing.T) {
	t.Parallel()
	input := ` host = "my host"  : 1521 `
	expected := `host="my host":1521`

	if got := StripSpacesOutsideQuotes(input); got != expected {
		t.Fatalf("StripSpacesOutsideQuotes(%q) = %q, want %q", input, got, expected)
	}
}

func TestUtility_SimpleStringToB1Array(t *testing.T) {
	t.Parallel()
	res := StringToB1Array("foo")
	if len(res) != len("foo") {
		t.Fatalf("unexpected b1 array length [%d] should be [%d]", len(res), len("foo"))
	}
	for i, c := range "foo" {
		if "foo"[i] != uint8(c) {
			t.Fatalf("unexpected b1 array fields [%c] should be [%c]", uint8(c), "foo"[i])
		}
	}
}

func TestUtility_EmptyStringToB1Array(t *testing.T) {
	t.Parallel()
	res := StringToB1Array("")
	if len(res) != len("") {
		t.Fatalf("unexpected b1 array length [%d] should be [%d]", len(res), len(""))
	}
}

func TestUtility_GetTimeZoneBytes(t *testing.T) {
	t.Parallel()
	res := GetTimeZoneBytes()
	if len(res) == 0 {
		t.Fatalf("can't get timezone ")
	}
	t.Logf("tz bytes = %s", res)
}

func TestNibbleToHex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    byte
		expected byte
	}{
		{0, '0'},
		{9, '9'},
		{10, 'A'},
		{15, 'F'},
		{16, '0'}, // high bits filtered
	}
	for _, test := range tests {
		result := NibbleToHex(test.input)
		if result != test.expected {
			t.Errorf("nibbleToHex(%d) = %c, want %c", test.input, result, test.expected)
		}
	}
}

func TestBArray2Nibbles(t *testing.T) {
	t.Parallel()
	array := []byte{0xAB, 0xCD}
	nibbles := make([]byte, 4)
	length := BArray2Nibbles(array, nibbles)
	if length != 4 {
		t.Errorf("bArray2Nibbles length = %d, want 4", length)
	}
	expected := []byte{'A', 'B', 'C', 'D'}
	for i, b := range expected {
		if nibbles[i] != b {
			t.Errorf("nibbles[%d] = %c, want %c", i, nibbles[i], b)
		}
	}
}

func TestToBinArray(t *testing.T) {
	t.Parallel()

	hexStr := "ABCD"
	result := ToBinArray(hexStr)
	expected := []byte{0xAB, 0xCD}
	if len(result) != len(expected) {
		t.Fatalf("toBinArray length = %d, want %d", len(result), len(expected))
	}
	for i, b := range expected {
		if result[i] != b {
			t.Errorf("result[%d] = %x, want %x", i, result[i], b)
		}
	}
}

// TestUtility_NonBMPStringToB1Array verifies UTF-8 encoding of the non-BMP
// grinning face emoji (😀), represented by Unicode code point U+1F600.
func TestUtility_NonBMPStringToB1Array(t *testing.T) {
	t.Parallel()
	input := "\U0001F600"
	expected := []byte(input)

	res := StringToB1Array(input)
	if len(res) != len(expected) {
		t.Fatalf("unexpected b1 array length [%d] should be [%d]", len(res), len(expected))
	}
	for i, b := range expected {
		if res[i] != b {
			t.Fatalf("unexpected b1 array field at index [%d]: [%d] should be [%d]", i, res[i], b)
		}
	}
}
