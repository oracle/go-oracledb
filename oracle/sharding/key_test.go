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

package sharding_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	publicSharding "github.com/oracle/go-oracledb/v26/oracle/sharding"
)

// TestKeyStoresOrderedSubkeys verifies that construction and access preserve subkey order,
// copy caller-owned RAW bytes, and redact values during formatting.
func TestKeyStoresOrderedSubkeys(t *testing.T) {
	raw := []byte{1, 2, 3}
	input := []publicSharding.Subkey{{publicSharding.RAW, raw}, {publicSharding.NUMBER, 120}}
	key, err := publicSharding.NewKey(input...)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 99
	input[1].Value = 200
	want := []publicSharding.Subkey{{publicSharding.RAW, []byte{1, 2, 3}}, {publicSharding.NUMBER, 120}}
	if !reflect.DeepEqual(key.Subkeys(), want) {
		t.Fatal("input mutation changed key or order")
	}
	copy := key.Subkeys()
	copy[0].Value.([]byte)[0] = 88
	copy[1].Value = 300
	if !reflect.DeepEqual(key.Subkeys(), want) {
		t.Fatal("accessor exposed key storage")
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, key), "120") {
			t.Fatal("key disclosed value")
		}
	}
}

// TestKeyDefersEncoding verifies that construction stores values without encoding
// them, while still rejecting a key with no subkeys.
func TestKeyDefersEncoding(t *testing.T) {
	if _, err := publicSharding.NewKey(publicSharding.Subkey{publicSharding.NUMBER, "not a number"}); err != nil {
		t.Fatal("constructor attempted encoding", err)
	}
	if _, err := publicSharding.NewKey(); err == nil {
		t.Fatal("empty key accepted")
	}
}
