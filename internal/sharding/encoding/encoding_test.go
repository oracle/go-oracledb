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

	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
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
		{"varchar", []publicSharding.Subkey{{Type: publicSharding.VARCHAR, Value: "abc"}}, "1 0 1,YWJj"},
		{"unicode", []publicSharding.Subkey{{Type: publicSharding.VARCHAR, Value: "é"}}, "1 0 1,w6k="},
		{"national", []publicSharding.Subkey{{Type: publicSharding.NVARCHAR, Value: "é"}}, "1 0 1,AOk="},
		{"surrogate", []publicSharding.Subkey{{Type: publicSharding.NCHAR, Value: "😀"}}, "1 0 96,2D3eAA=="},
		{"zero", []publicSharding.Subkey{{Type: publicSharding.NUMBER, Value: 0}}, "1 0 2,gA=="},
		{"positive", []publicSharding.Subkey{{Type: publicSharding.NUMBER, Value: 120}}, "1 0 2,wgIV"},
		{"negative", []publicSharding.Subkey{{Type: publicSharding.NUMBER, Value: -1}}, "1 0 2,PmRm"},
		{"fraction", []publicSharding.Subkey{{Type: publicSharding.NUMBER, Value: 1.5}}, "1 0 2,wQIz"},
		{"char", []publicSharding.Subkey{{Type: publicSharding.CHAR, Value: "US"}}, "1 0 96,VVM="},
		{"raw", []publicSharding.Subkey{{Type: publicSharding.RAW, Value: []byte{0, 255}}}, "1 0 23,AP8="},
		{"compound", []publicSharding.Subkey{{Type: publicSharding.VARCHAR, Value: "abc"}, {Type: publicSharding.NUMBER, Value: 120}}, "1 0 1 2,YWJj,wgIV"},
		{"date", []publicSharding.Subkey{{Type: publicSharding.DATE, Value: time.Date(2026, 11, 11, 12, 30, 45, 0, time.UTC)}}, "1 0 12,eH4LCw0fLg=="},
		{"timestamp", []publicSharding.Subkey{{Type: publicSharding.TIMESTAMP, Value: time.Date(2026, 11, 11, 12, 30, 45, 1, time.UTC)}}, "1 0 180,eH4LCw0fLgAAAAE="},
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
	cases := [][]publicSharding.Subkey{nil, {{Type: publicSharding.VARCHAR, Value: ""}}, {{Type: publicSharding.VARCHAR, Value: "\xff"}}, {{Type: publicSharding.NUMBER, Value: nil}}, {{Type: publicSharding.NUMBER, Value: "not-a-number"}}, {{Type: publicSharding.NUMBER, Value: true}}, {{Type: publicSharding.NUMBER, Value: math.NaN()}}, {{Type: publicSharding.NUMBER, Value: math.Inf(1)}}, {{Type: publicSharding.RAW, Value: []byte(nil)}}, {{Type: publicSharding.DATE, Value: "today"}}, {{Type: publicSharding.KeyType(99), Value: 1}}, {{Type: publicSharding.VARCHAR, Value: strings.Repeat("a", 65536)}}}
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

// TestStringSubkeyEncoding verifies conversion according to the declared type,
// exact decimal handling, and equivalent wire bytes for typed/string inputs.
func TestStringSubkeyEncoding(t *testing.T) {
	cases := []struct {
		name string
		kind publicSharding.KeyType
		text string
		want string
	}{
		{"number", publicSharding.NUMBER, "120", "1 0 2,wgIV"},
		{"exponent", publicSharding.NUMBER, " +1.20e2 ", "1 0 2,wgIV"},
		{"fraction", publicSharding.NUMBER, "1.5", "1 0 2,wQIz"},
		{"negative", publicSharding.NUMBER, "-1", "1 0 2,PmRm"},
		{"zero", publicSharding.NUMBER, "-0.000", "1 0 2,gA=="},
		{"raw", publicSharding.RAW, "00fF", "1 0 23,AP8="},
		{"date", publicSharding.DATE, "2026-11-11 12:30:45", "1 0 12,eH4LCw0fLg=="},
		{"timestamp", publicSharding.TIMESTAMP, "2026-11-11T12:30:45.000000001Z", "1 0 180,eH4LCw0fLgAAAAE="},
		{"local timestamp", publicSharding.TIMESTAMP, "2026-11-11 12:30:45.000000001", "1 0 180,eH4LCw0fLgAAAAE="},
		{"character remains text", publicSharding.VARCHAR, "120", "1 0 1,MTIw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := publicSharding.NewKey(publicSharding.Subkey{Type: tc.kind, Value: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			got, err := EncodeKey(key)
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
			if key.Subkeys()[0].Value != tc.text {
				t.Fatal("encoding modified stored subkey")
			}
		})
	}
	// Decoding exact decimal bytes catches precision loss beyond float64/int64.
	for _, value := range []string{"9007199254740993", "18446744073709551616", "12345678901234567890.123456789012345678", "0.0012", "-0.0012"} {
		data, ok := encodeNumberString(value)
		if !ok {
			t.Fatalf("rejected %s", value)
		}
		got, err := converters.DecodeExactDecimalAsString(data)
		if err != nil || got != value {
			t.Fatalf("round trip: got %s, %v; want %s", got, err, value)
		}
	}
	for _, value := range []string{"1e-130", "9.99e125"} {
		if _, ok := encodeNumberString(value); !ok {
			t.Fatalf("rejected boundary %s", value)
		}
	}
	for _, value := range []string{"2026-11-11", "2026-11-11T12:30:45", "2026-11-11T12:30:45+05:30"} {
		if _, ok := parseTimeString(value); !ok {
			t.Fatalf("rejected time %s", value)
		}
	}
}

// TestStringSubkeyEncodingRejectsInvalidValues verifies malformed, ambiguous, and
// out-of-range strings fail during encoding with redacted registered errors.
func TestStringSubkeyEncodingRejectsInvalidValues(t *testing.T) {
	cases := map[publicSharding.KeyType][]string{
		publicSharding.NUMBER:    {"", ".", "1.2.3", "1e", "NaN", "Inf", "0x12", "1_000", "1e126", "1e-131", "1e999999999999", "123456789012345678901234567890123456789"},
		publicSharding.RAW:       {"", "abc", "zz", "0x12"},
		publicSharding.DATE:      {"today", "11/12/2026", "2026-02-30", "0000-01-01"},
		publicSharding.TIMESTAMP: {"invalid", "2026-11-11T25:00:00Z", "2026-11-11T12:30:45.1234567891Z"},
	}
	for kind, values := range cases {
		for _, value := range values {
			key, err := publicSharding.NewKey(publicSharding.Subkey{Type: kind, Value: value})
			if err != nil {
				t.Fatal(err)
			}
			_, err = EncodeKey(key)
			var sqlErr oracleErrors.SQLError
			if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(oracleErrors.ShardingSubkeyEncoding) {
				t.Fatalf("expected encoding error for type %v, got %v", kind, err)
			}
			var cause oracleErrors.SQLError
			if !errors.As(errors.Unwrap(err), &cause) || cause.ErrorCode() != string(oracleErrors.ShardingSubkeyInvalid) {
				t.Fatalf("expected invalid-subkey cause, got %v", err)
			}
		}
	}
	key, _ := publicSharding.NewKey(publicSharding.Subkey{Type: publicSharding.NUMBER, Value: "private-invalid-value"})
	_, err := EncodeKey(key)
	if err == nil || strings.Contains(err.Error(), "private-invalid-value") {
		t.Fatal("missing or unredacted conversion error")
	}
}
