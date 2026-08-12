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

package oson

import (
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/ttc/converters"
)

// TestScalarNode_ValueCoversSupportedDecodeUseCases verifies that scalar Value
// decodes each supported inline OSON scalar family into the expected Go value.
func TestScalarNode_ValueCoversSupportedDecodeUseCases(t *testing.T) {

	timestampTZ := time.Date(2024, time.March, 1, 12, 34, 56, 789000000, time.FixedZone("UTC+02", 2*3600))
	datePayload, _ := converters.EncodeDate(time.Date(2024, time.January, 2, 3, 4, 5, 0, time.Local))
	timestampPayload, _ := converters.EncodeTimestamp(time.Date(2024, time.January, 2, 3, 4, 5, 123000000, time.Local))
	timestamp7Payload, _ := converters.EncodeTimestamp(time.Date(2024, time.January, 2, 3, 4, 5, 0, time.Local))
	timestampTZPayload, _ := converters.EncodeTimestampWithTimeZone(timestampTZ)
	binaryFloatPayload, _ := converters.EncodeBinaryFloat(float32(12.5))
	binaryDoublePayload, _ := converters.EncodeBinaryDouble(float64(42.25))
	integerNumberPayload, _ := converters.EncodeInt(int64(42))
	decimalNumberPayload, _ := converters.EncodeFloat(12.75)
	largeNumberPayload, _ := converters.EncodeInt(int64(1234567890123456))
	intervalYMPayload, _ := converters.EncodeIntervalYearToMonth("02-03")
	intervalDSPayload, _ := converters.EncodeIntervalDayToSecond("10 05:30:02.123")
	const (
		shortStringText  = "ok"
		stringNumberText = "12.75"
		helloText        = "hello"
		worldText        = "world"
	)

	tests := []struct {
		name    string
		payload common.B1Array
		assert  func(t *testing.T, got any)
	}{
		{
			name:    "compact signed32",
			payload: append(common.B1Array{byte(osonOpCompactSigned32Prefix | common.UB1(len(integerNumberPayload)))}, integerNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(42) {
					t.Fatalf("got %#v, want 42", got)
				}
			},
		},
		{
			name:    "compact signed64",
			payload: append(common.B1Array{byte(osonOpCompactSigned64Prefix | common.UB1(len(integerNumberPayload)))}, integerNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(42) {
					t.Fatalf("got %#v, want 42", got)
				}
			},
		},
		{
			name:    "compact oracle number",
			payload: append(common.B1Array{byte(osonOpCompactOracleNumberPrefix | common.UB1(len(decimalNumberPayload)-_compactNumberLengthBias))}, decimalNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("got %#v, want 12.75", got)
				}
			},
		},
		{
			name:    "compact decimal",
			payload: append(common.B1Array{byte(osonOpCompactDecimalPrefix | common.UB1(len(decimalNumberPayload)-_compactNumberLengthBias))}, decimalNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("got %#v, want 12.75", got)
				}
			},
		},
		{
			name:    "explicit oracle number",
			payload: append(common.B1Array{osonOpOracleNumber, byte(common.UB1(len(largeNumberPayload)))}, largeNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(1234567890123456) {
					t.Fatalf("got %#v, want 1234567890123456", got)
				}
			},
		},
		{
			name:    "explicit oracle decimal",
			payload: append(common.B1Array{osonOpOracleDecimal, byte(common.UB1(len(decimalNumberPayload)))}, decimalNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("got %#v, want 12.75", got)
				}
			},
		},
		{
			name:    "string number",
			payload: append(common.B1Array{osonOpStringNumber, byte(common.UB1(len(stringNumberText)))}, common.B1Array(stringNumberText)...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("got %#v, want 12.75", got)
				}
			},
		},
		{
			name:    "compact short string",
			payload: append(common.B1Array{byte(common.UB1(len(shortStringText)))}, common.B1Array(shortStringText)...),
			assert: func(t *testing.T, got any) {
				if got != "ok" {
					t.Fatalf("got %#v, want ok", got)
				}
			},
		},
		{
			name:    "string ub1",
			payload: append(common.B1Array{osonOpStringUB1, byte(common.UB1(len(helloText)))}, common.B1Array(helloText)...),
			assert: func(t *testing.T, got any) {
				if got != "hello" {
					t.Fatalf("got %#v, want hello", got)
				}
			},
		},
		{
			name:    "string ub2",
			payload: append(common.B1Array{osonOpStringUB2, 0x00, byte(common.UB1(len(helloText)))}, common.B1Array(helloText)...),
			assert: func(t *testing.T, got any) {
				if got != "hello" {
					t.Fatalf("got %#v, want hello", got)
				}
			},
		},
		{
			name:    "string ub4",
			payload: append(common.B1Array{osonOpStringUB4, 0x00, 0x00, 0x00, byte(common.UB1(len(worldText)))}, common.B1Array(worldText)...),
			assert: func(t *testing.T, got any) {
				if got != "world" {
					t.Fatalf("got %#v, want world", got)
				}
			},
		},
		{
			name:    "null",
			payload: common.B1Array{osonOpNull},
			assert: func(t *testing.T, got any) {
				if got != nil {
					t.Fatalf("got %#v, want nil", got)
				}
			},
		},
		{
			name:    "true",
			payload: common.B1Array{osonOpTrue},
			assert: func(t *testing.T, got any) {
				if got != true {
					t.Fatalf("got %#v, want true", got)
				}
			},
		},
		{
			name:    "false",
			payload: common.B1Array{osonOpFalse},
			assert: func(t *testing.T, got any) {
				if got != false {
					t.Fatalf("got %#v, want false", got)
				}
			},
		},
		{
			name:    "binary float",
			payload: append(common.B1Array{osonOpBinaryFloat}, binaryFloatPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.5) {
					t.Fatalf("got %#v, want 12.5", got)
				}
			},
		},
		{
			name:    "binary double",
			payload: append(common.B1Array{osonOpBinaryDouble}, binaryDoublePayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(42.25) {
					t.Fatalf("got %#v, want 42.25", got)
				}
			},
		},
		{
			name:    "date",
			payload: append(common.B1Array{osonOpDate}, datePayload...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("got %T, want time.Time", got)
				}
				if tm.Year() != 2024 || tm.Month() != time.January || tm.Day() != 2 {
					t.Fatalf("got %v, want 2024-01-02", tm)
				}
			},
		},
		{
			name:    "timestamp",
			payload: append(common.B1Array{osonOpTimestamp}, timestampPayload...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("got %T, want time.Time", got)
				}
				if tm.Nanosecond() != 123000000 {
					t.Fatalf("got %v, want nanoseconds 123000000", tm)
				}
			},
		},
		{
			name:    "timestamp7",
			payload: append(common.B1Array{osonOpTimestamp7}, timestamp7Payload[:7]...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("got %T, want time.Time", got)
				}
				if tm.Nanosecond() != 0 {
					t.Fatalf("got %v, want zero fractional seconds", tm)
				}
			},
		},
		{
			name:    "timestamp with timezone",
			payload: append(common.B1Array{osonOpTimestampTZ}, timestampTZPayload...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("got %T, want time.Time", got)
				}
				_, off := tm.Zone()
				if off != 2*3600 {
					t.Fatalf("got offset %d, want 7200", off)
				}
			},
		},
		{
			name:    "interval year to month",
			payload: append(common.B1Array{osonOpIntervalYM}, intervalYMPayload...),
			assert: func(t *testing.T, got any) {
				if got != "02-03" {
					t.Fatalf("got %#v, want 02-03", got)
				}
			},
		},
		{
			name:    "interval day to second",
			payload: append(common.B1Array{osonOpIntervalDS}, intervalDSPayload...),
			assert: func(t *testing.T, got any) {
				if got != "10 05:30:02.123" {
					t.Fatalf("got %#v, want 10 05:30:02.123", got)
				}
			},
		},
		{
			name:    "binary ub2",
			payload: common.B1Array{osonOpBinaryUB2, 0x00, 0x03, 0xaa, 0xbb, 0xcc},
			assert: func(t *testing.T, got any) {
				if !reflect.DeepEqual(got, []byte{0xaa, 0xbb, 0xcc}) {
					t.Fatalf("got %#v, want raw bytes", got)
				}
			},
		},
		{
			name:    "binary ub4",
			payload: common.B1Array{osonOpBinaryUB4, 0x00, 0x00, 0x00, 0x03, 0xde, 0xad, 0xbe},
			assert: func(t *testing.T, got any) {
				if !reflect.DeepEqual(got, []byte{0xde, 0xad, 0xbe}) {
					t.Fatalf("got %#v, want raw bytes", got)
				}
			},
		},
		{
			name:    "binary id",
			payload: common.B1Array{osonOpID, 0x03, 0x01, 0x02, 0x03},
			assert: func(t *testing.T, got any) {
				if !reflect.DeepEqual(got, []byte{0x01, 0x02, 0x03}) {
					t.Fatalf("got %#v, want id bytes", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node, err := newScalarNodeAt(newOsonBuffer(tc.payload), &osonHeader{}, 0)
			if err != nil {
				t.Fatalf("newScalarNodeAt() error = %v", err)
			}
			got, err := node.Value(common.JSONOptDefault)
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			tc.assert(t, got)
		})
	}
}

// TestScalarNode_NumberAsStringOption verifies that numeric scalar opcodes honor
// JSONOptNumberAsString by preserving their textual numeric representation.
func TestScalarNode_NumberAsStringOption(t *testing.T) {
	decimalNumberPayload, _ := converters.EncodeFloat(12.75)
	arrayDecimalPayload, _ := converters.EncodeFloat(8.25)
	decimalScalePayload, _ := converters.EncodeFloat(12345.6789)
	float64ScalePayload, _ := converters.EncodeFloat(98765.125)
	largeIntegerPayload, _ := converters.EncodeInt(int64(1234567890123456))
	compactIntegerPayload, _ := converters.EncodeInt(int64(7))
	binaryFloatPayload, _ := converters.EncodeBinaryFloat(float32(12.5))
	const stringNumberText = "12.75"

	tests := []struct {
		name    string
		payload common.B1Array
		want    common.JSONNumber
	}{
		{
			name:    "compact oracle number",
			payload: append(common.B1Array{byte(osonOpCompactOracleNumberPrefix | common.UB1(len(decimalNumberPayload)-_compactNumberLengthBias))}, decimalNumberPayload...),
			want:    common.JSONNumber("12.75"),
		},
		{
			name:    "compact oracle array decimal",
			payload: append(common.B1Array{byte(osonOpCompactOracleNumberPrefix | common.UB1(len(arrayDecimalPayload)-_compactNumberLengthBias))}, arrayDecimalPayload...),
			want:    common.JSONNumber("8.25"),
		},
		{
			name:    "compact oracle decimal scale",
			payload: append(common.B1Array{byte(osonOpCompactOracleNumberPrefix | common.UB1(len(decimalScalePayload)-_compactNumberLengthBias))}, decimalScalePayload...),
			want:    common.JSONNumber("12345.6789"),
		},
		{
			name:    "compact oracle float64 scale",
			payload: append(common.B1Array{byte(osonOpCompactOracleNumberPrefix | common.UB1(len(float64ScalePayload)-_compactNumberLengthBias))}, float64ScalePayload...),
			want:    common.JSONNumber("98765.125"),
		},
		{
			name:    "explicit oracle number",
			payload: append(common.B1Array{osonOpOracleNumber, byte(common.UB1(len(largeIntegerPayload)))}, largeIntegerPayload...),
			want:    common.JSONNumber("1234567890123456"),
		},
		{
			name:    "compact signed integer",
			payload: append(common.B1Array{byte(osonOpCompactSigned32Prefix | common.UB1(len(compactIntegerPayload)))}, compactIntegerPayload...),
			want:    common.JSONNumber("7"),
		},
		{
			name:    "binary float",
			payload: append(common.B1Array{osonOpBinaryFloat}, binaryFloatPayload...),
			want:    common.JSONNumber("12.5"),
		},
		{
			name:    "string number",
			payload: append(common.B1Array{osonOpStringNumber, byte(common.UB1(len(stringNumberText)))}, common.B1Array(stringNumberText)...),
			want:    common.JSONNumber("12.75"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node, err := newScalarNodeAt(newOsonBuffer(tc.payload), &osonHeader{}, 0)
			if err != nil {
				t.Fatalf("newScalarNodeAt() error = %v", err)
			}
			got, err := node.Value(common.JSONOptNumberAsString)
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestScalarNode_DefaultOracleNumberAllowsLargePrecisionFloat verifies that
// large Oracle NUMBER payloads still decode to float64 by default while the string option preserves exact text.
func TestScalarNode_DefaultOracleNumberAllowsLargePrecisionFloat(t *testing.T) {
	const text = "123456789012345678901234567890.12345"

	payload := converters.ToNumber([]byte("12345678901234567890123456789012345"), false, 29)
	node, err := newScalarNodeAt(
		newOsonBuffer(append(common.B1Array{osonOpOracleNumber, byte(common.UB1(len(payload)))}, payload...)),
		&osonHeader{},
		0,
	)
	if err != nil {
		t.Fatalf("newScalarNodeAt() error = %v", err)
	}

	got, err := node.Value(common.JSONOptDefault)
	if err != nil {
		t.Fatalf("Value(JSONOptDefault) error = %v", err)
	}
	want, err := strconv.ParseFloat(text, 64)
	if err != nil {
		t.Fatalf("ParseFloat() error = %v", err)
	}
	if got != want {
		t.Fatalf("Value(JSONOptDefault) = %#v, want %#v", got, want)
	}

	gotString, err := node.Value(common.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("Value(JSONOptNumberAsString) error = %v", err)
	}
	if gotString != common.JSONNumber(text) {
		t.Fatalf("Value(JSONOptNumberAsString) = %#v, want JSONNumber(%q)", gotString, text)
	}

	gotText, err := node.StringWithOption(common.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("StringWithOption(JSONOptNumberAsString) error = %v", err)
	}
	if gotText != text {
		t.Fatalf("StringWithOption(JSONOptNumberAsString) = %q, want %q", gotText, text)
	}
}

// TestScalarNode_KindAndStringWithOption verifies scalar kind classification
// and JSON string rendering for a decoded scalar value.
func TestScalarNode_KindAndStringWithOption(t *testing.T) {
	node, err := newScalarNodeAt(newOsonBuffer(common.B1Array{0x02, 'o', 'k'}), &osonHeader{}, 0)
	if err != nil {
		t.Fatalf("newScalarNodeAt() error = %v", err)
	}

	if got, want := node.Kind(), common.KindScalar; got != want {
		t.Fatalf("Kind() = %v, want %v", got, want)
	}

	text, err := node.StringWithOption(common.JSONOptDefault)
	if err != nil {
		t.Fatalf("StringWithOption() error = %v", err)
	}
	if text != `"ok"` {
		t.Fatalf("StringWithOption() = %q, want %q", text, `"ok"`)
	}
}

// TestScalarNode_MalformedScalarPayloads verifies that malformed scalar
// payloads fail with the expected Oracle error codes.
func TestScalarNode_MalformedScalarPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload common.B1Array
		want    common.ErrorCode
	}{
		{
			name:    "truncated string ub1 payload",
			payload: common.B1Array{osonOpStringUB1, 0x05, 'h', 'e'},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated string ub2 payload",
			payload: common.B1Array{osonOpStringUB2, 0x00, 0x05, 'h', 'e'},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated string ub4 payload",
			payload: common.B1Array{osonOpStringUB4, 0x00, 0x00, 0x00, 0x05, 'h', 'e'},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated explicit oracle number payload",
			payload: common.B1Array{osonOpOracleNumber, 0x03, 0x01},
			want:    common.OsonBufferError,
		},
		{
			name:    "invalid string number payload",
			payload: common.B1Array{osonOpStringNumber, 0x03, 'x', 'y', 'z'},
			want:    common.OsonParsingError,
		},
		{
			name:    "truncated binary float payload",
			payload: common.B1Array{osonOpBinaryFloat, 0x01},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated binary double payload",
			payload: common.B1Array{osonOpBinaryDouble, 0x01, 0x02},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated timestamp payload",
			payload: common.B1Array{osonOpTimestamp, 120, 124, 1, 2},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated timestamptz payload",
			payload: common.B1Array{osonOpTimestampTZ, 120, 124, 1, 2},
			want:    common.OsonBufferError,
		},
		{
			name:    "truncated interval ds payload",
			payload: common.B1Array{osonOpIntervalDS, 0x80, 0x00},
			want:    common.OsonBufferError,
		},
		{
			name:    "oversized id payload length",
			payload: common.B1Array{osonOpID, 0x80},
			want:    common.OsonBufferError,
		},
		{
			name:    "reserved update opcode",
			payload: common.B1Array{osonOpUpdateOversizeReserved},
			want:    common.OsonUnsupportedScalarError,
		},
		{
			name:    "unknown scalar opcode",
			payload: common.B1Array{0x79},
			want:    common.OsonUnsupportedScalarError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node, err := newScalarNodeAt(newOsonBuffer(tc.payload), &osonHeader{}, 0)
			if err != nil {
				t.Fatalf("newScalarNodeAt() error = %v", err)
			}

			_, err = node.Value(common.JSONOptDefault)
			if err == nil {
				t.Fatal("Value() error = nil, want failure")
			}
			assertOracleErrorCode(t, err, tc.want)
		})
	}
}

func assertOracleErrorCode(t *testing.T, err error, want common.ErrorCode) {
	t.Helper()

	oraErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("error type = %T, want common.SQLError", err)
	}
	if got := oraErr.ErrorCode(); got != string(want) {
		t.Fatalf("error code = %v, want %v", got, want)
	}
}

// TestScalarNode_BinaryFloatSpecialValue verifies that binary float decoding
// preserves special IEEE values such as positive infinity.
func TestScalarNode_BinaryFloatSpecialValue(t *testing.T) {
	payload, _ := converters.EncodeBinaryFloat(float32(math.Inf(1)))
	node, err := newScalarNodeAt(
		newOsonBuffer(append(common.B1Array{osonOpBinaryFloat}, payload...)),
		&osonHeader{},
		0,
	)
	if err != nil {
		t.Fatalf("newScalarNodeAt() error = %v", err)
	}

	got, err := node.Value(common.JSONOptDefault)
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if !math.IsInf(got.(float64), 1) {
		t.Fatalf("got %#v, want +Inf", got)
	}
}
