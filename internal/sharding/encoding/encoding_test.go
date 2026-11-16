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

package encoding

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	publicSharding "github.com/oracle/go-oracledb/v26/oracle/sharding"
)

// TestListenerEncoding checks literal listener-format fixtures for supported
// types and compound keys, including type headers, Base64 values, and subkey order.
func TestListenerEncoding(t *testing.T) {
	// Literal fixtures follow the version-1/type-0 format in the JDBC Encoder:
	// internal type codes followed by Base64 of Oracle datum bytes.
	tests := []struct {
		name    string
		subkeys []publicSharding.Subkey
		want    string
	}{
		{"varchar", []publicSharding.Subkey{{publicSharding.VARCHAR, "abc"}}, "1 0 1,YWJj"},
		{"unicode", []publicSharding.Subkey{{publicSharding.VARCHAR, "é"}}, "1 0 1,w6k="},
		{"national", []publicSharding.Subkey{{publicSharding.NVARCHAR, "é"}}, "1 0 1,AOk="},
		{"surrogate", []publicSharding.Subkey{{publicSharding.NCHAR, "😀"}}, "1 0 96,2D3eAA=="},
		{"zero", []publicSharding.Subkey{{publicSharding.NUMBER, 0}}, "1 0 2,gA=="},
		{"positive", []publicSharding.Subkey{{publicSharding.NUMBER, 120}}, "1 0 2,wgIV"},
		{"negative", []publicSharding.Subkey{{publicSharding.NUMBER, -1}}, "1 0 2,PmRm"},
		{"fraction", []publicSharding.Subkey{{publicSharding.NUMBER, 1.5}}, "1 0 2,wQIz"},
		{"char", []publicSharding.Subkey{{publicSharding.CHAR, "US"}}, "1 0 96,VVM="},
		{"raw", []publicSharding.Subkey{{publicSharding.RAW, []byte{0, 255}}}, "1 0 23,AP8="},
		{"compound", []publicSharding.Subkey{{publicSharding.VARCHAR, "abc"}, {publicSharding.NUMBER, 120}}, "1 0 1 2,YWJj,wgIV"},
		{"date", []publicSharding.Subkey{{publicSharding.DATE, time.Date(2026, 11, 11, 12, 30, 45, 0, time.UTC)}}, "1 0 12,eH4LCw0fLg=="},
		{"timestamp", []publicSharding.Subkey{{publicSharding.TIMESTAMP, time.Date(2026, 11, 11, 12, 30, 45, 1, time.UTC)}}, "1 0 180,eH4LCw0fLgAAAAE="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := publicSharding.NewKey(tt.subkeys...)
			if err != nil {
				t.Fatal(err)
			}
			got, err := EncodeKey(key)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

// TestKeyRejectsInvalidValues verifies rejection of empty keys, unsupported
// types or values, non-finite numbers, and keys exceeding the encoded size limit.
func TestKeyRejectsInvalidValues(t *testing.T) {
	cases := [][]publicSharding.Subkey{nil, {{publicSharding.VARCHAR, ""}}, {{publicSharding.VARCHAR, "\xff"}}, {{publicSharding.NUMBER, nil}}, {{publicSharding.NUMBER, "123"}}, {{publicSharding.NUMBER, true}}, {{publicSharding.NUMBER, math.NaN()}}, {{publicSharding.NUMBER, math.Inf(1)}}, {{publicSharding.RAW, []byte(nil)}}, {{publicSharding.DATE, "today"}}, {{publicSharding.KeyType(99), 1}}, {{publicSharding.VARCHAR, strings.Repeat("a", 65536)}}}
	for i, c := range cases {
		key, err := publicSharding.NewKey(c...)
		if err == nil {
			_, err = EncodeKey(key)
		}
		if err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

// TestKeyErrorsUseDriverCodes checks registered error codes, wrapped subkey
// errors, and length errors without exposing private key values.
func TestKeyErrorsUseDriverCodes(t *testing.T) {
	_, err := publicSharding.NewKey()
	assertCode := func(err error, code oracleErrors.ErrorCode) {
		t.Helper()
		var sqlErr oracleErrors.SQLError
		if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(code) {
			t.Fatalf("want %s, got %v", code, err)
		}
	}
	assertCode(err, oracleErrors.ShardingKeyEmpty)
	key, err := publicSharding.NewKey(publicSharding.Subkey{Type: publicSharding.NUMBER, Value: "private-customer-value"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = EncodeKey(key)
	assertCode(err, oracleErrors.ShardingSubkeyEncoding)
	assertCode(errors.Unwrap(err), oracleErrors.ShardingSubkeyInvalid)
	if strings.Contains(err.Error(), "private-customer-value") {
		t.Fatal("error disclosed key value")
	}
	key, err = publicSharding.NewKey(publicSharding.Subkey{Type: publicSharding.VARCHAR, Value: strings.Repeat("x", 65536)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = EncodeKey(key)
	assertCode(err, oracleErrors.ShardingKeyTooLong)
}
