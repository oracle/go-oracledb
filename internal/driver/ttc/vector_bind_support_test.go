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
	"database/sql/driver"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestCheckNamedValueAcceptsVectorTypes verifies each named VECTOR type bypasses
// database/sql's default conversion while unrelated slice values do not.
func TestCheckNamedValueAcceptsVectorTypes(t *testing.T) {
	if err := checkNamedValue(nil); err != driver.ErrSkip {
		t.Fatalf("nil NamedValue: got %v want driver.ErrSkip", err)
	}

	cases := []struct {
		name  string
		value any
		ok    bool
	}{
		{"float64", datatype.VectorFloat64{1, 2}, true},
		{"float32", datatype.VectorFloat32{1, 2}, true},
		{"int8", datatype.VectorInt8{1, 2}, true},
		{"binary", datatype.VectorBinary{0xAA}, true},
		{"plain float64", []float64{1, 2}, false},
		{"raw bytes", []byte{1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkNamedValue(&driver.NamedValue{Ordinal: 1, Value: tc.value})
			if tc.ok && err != nil {
				t.Fatalf("expected accepted VECTOR type, got %v", err)
			}
			if !tc.ok && err != driver.ErrSkip {
				t.Fatalf("expected driver.ErrSkip, got %v", err)
			}
		})
	}
}
