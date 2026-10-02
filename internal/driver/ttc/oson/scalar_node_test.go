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
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestScalarNode_ValueCoversSupportedDecodeUseCases verifies that scalar Value
// decodes each supported inline OSON scalar family into the expected Go value.
func TestScalarNode_ValueCoversSupportedDecodeUseCases(t *testing.T) {

	timestampTZ := time.Date(2024, time.March, 1, 12, 34, 56, 789000000, time.FixedZone("UTC+02", 2*3600))
	datePayload, _ := converters.EncodeDate(time.Date(2024, time.January, 2, 3, 4, 5, 0, time.Local))
	timestampPayload, _ := converters.EncodeTimestamp(time.Date(2024, time.January, 2, 3, 4, 5, 123000000, time.Local))
	timestamp7Payload, _ := converters.EncodeTimestamp(time.Date(2024, time.January, 2, 3, 4, 5, 0, time.Local))
	timestampTZPayload := encodeTimestampTZ(timestampTZ)
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
		payload drvCommon.B1Array
		assert  func(t *testing.T, got any)
	}{
		{
			name:    "compact signed32",
			payload: append(drvCommon.B1Array{byte(osonOpCompactSigned32Prefix | drvCommon.UB1(len(integerNumberPayload)))}, integerNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(42) {
					t.Fatalf("expected Value() to return float64(42), got %#v", got)
				}
			},
		},
		{
			name:    "compact signed64",
			payload: append(drvCommon.B1Array{byte(osonOpCompactSigned64Prefix | drvCommon.UB1(len(integerNumberPayload)))}, integerNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(42) {
					t.Fatalf("expected Value() to return float64(42), got %#v", got)
				}
			},
		},
		{
			name:    "compact oracle number",
			payload: append(drvCommon.B1Array{byte(osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(decimalNumberPayload)-_compactNumberLengthBias))}, decimalNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("expected Value() to return float64(12.75), got %#v", got)
				}
			},
		},
		{
			name:    "compact decimal",
			payload: append(drvCommon.B1Array{byte(osonOpCompactDecimalPrefix | drvCommon.UB1(len(decimalNumberPayload)-_compactNumberLengthBias))}, decimalNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("expected Value() to return float64(12.75), got %#v", got)
				}
			},
		},
		{
			name:    "explicit oracle number",
			payload: append(drvCommon.B1Array{osonOpOracleNumber, byte(drvCommon.UB1(len(largeNumberPayload)))}, largeNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(1234567890123456) {
					t.Fatalf("expected Value() to return float64(1234567890123456), got %#v", got)
				}
			},
		},
		{
			name:    "explicit oracle decimal",
			payload: append(drvCommon.B1Array{osonOpOracleDecimal, byte(drvCommon.UB1(len(decimalNumberPayload)))}, decimalNumberPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("expected Value() to return float64(12.75), got %#v", got)
				}
			},
		},
		{
			name:    "string number",
			payload: append(drvCommon.B1Array{osonOpStringNumber, byte(drvCommon.UB1(len(stringNumberText)))}, drvCommon.B1Array(stringNumberText)...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.75) {
					t.Fatalf("expected Value() to return float64(12.75), got %#v", got)
				}
			},
		},
		{
			name:    "compact short string",
			payload: append(drvCommon.B1Array{byte(drvCommon.UB1(len(shortStringText)))}, drvCommon.B1Array(shortStringText)...),
			assert: func(t *testing.T, got any) {
				if got != "ok" {
					t.Fatalf("expected Value() to return %q, got %#v", "ok", got)
				}
			},
		},
		{
			name:    "string ub1",
			payload: append(drvCommon.B1Array{osonOpStringUB1, byte(drvCommon.UB1(len(helloText)))}, drvCommon.B1Array(helloText)...),
			assert: func(t *testing.T, got any) {
				if got != "hello" {
					t.Fatalf("expected Value() to return %q, got %#v", "hello", got)
				}
			},
		},
		{
			name:    "string ub2",
			payload: append(drvCommon.B1Array{osonOpStringUB2, 0x00, byte(drvCommon.UB1(len(helloText)))}, drvCommon.B1Array(helloText)...),
			assert: func(t *testing.T, got any) {
				if got != "hello" {
					t.Fatalf("expected Value() to return %q, got %#v", "hello", got)
				}
			},
		},
		{
			name:    "string ub4",
			payload: append(drvCommon.B1Array{osonOpStringUB4, 0x00, 0x00, 0x00, byte(drvCommon.UB1(len(worldText)))}, drvCommon.B1Array(worldText)...),
			assert: func(t *testing.T, got any) {
				if got != "world" {
					t.Fatalf("expected Value() to return %q, got %#v", "world", got)
				}
			},
		},
		{
			name:    "null",
			payload: drvCommon.B1Array{osonOpNull},
			assert: func(t *testing.T, got any) {
				if got != nil {
					t.Fatalf("expected Value() to return nil, got %#v", got)
				}
			},
		},
		{
			name:    "true",
			payload: drvCommon.B1Array{osonOpTrue},
			assert: func(t *testing.T, got any) {
				if got != true {
					t.Fatalf("expected Value() to return true, got %#v", got)
				}
			},
		},
		{
			name:    "false",
			payload: drvCommon.B1Array{osonOpFalse},
			assert: func(t *testing.T, got any) {
				if got != false {
					t.Fatalf("expected Value() to return false, got %#v", got)
				}
			},
		},
		{
			name:    "binary float",
			payload: append(drvCommon.B1Array{osonOpBinaryFloat}, binaryFloatPayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(12.5) {
					t.Fatalf("expected Value() to return float64(12.5), got %#v", got)
				}
			},
		},
		{
			name:    "binary double",
			payload: append(drvCommon.B1Array{osonOpBinaryDouble}, binaryDoublePayload...),
			assert: func(t *testing.T, got any) {
				if got != float64(42.25) {
					t.Fatalf("expected Value() to return float64(42.25), got %#v", got)
				}
			},
		},
		{
			name:    "date",
			payload: append(drvCommon.B1Array{osonOpDate}, datePayload...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("expected Value() to return time.Time, got %T", got)
				}
				if tm.Year() != 2024 || tm.Month() != time.January || tm.Day() != 2 {
					t.Fatalf("expected the decoded date to be 2024-01-02, got %v", tm)
				}
			},
		},
		{
			name:    "timestamp",
			payload: append(drvCommon.B1Array{osonOpTimestamp}, timestampPayload...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("expected Value() to return time.Time, got %T", got)
				}
				if tm.Nanosecond() != 123000000 {
					t.Fatalf("expected the decoded timestamp to contain 123000000 nanoseconds, got %v", tm)
				}
			},
		},
		{
			name:    "timestamp7",
			payload: append(drvCommon.B1Array{osonOpTimestamp7}, timestamp7Payload[:7]...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("expected Value() to return time.Time, got %T", got)
				}
				if tm.Nanosecond() != 0 {
					t.Fatalf("expected the seven-byte timestamp to have zero fractional seconds, got %v", tm)
				}
			},
		},
		{
			name:    "timestamp with timezone",
			payload: append(drvCommon.B1Array{osonOpTimestampTZ}, timestampTZPayload...),
			assert: func(t *testing.T, got any) {
				tm, ok := got.(time.Time)
				if !ok {
					t.Fatalf("expected Value() to return time.Time, got %T", got)
				}
				_, off := tm.Zone()
				if off != 2*3600 {
					t.Fatalf("expected the decoded timezone offset to be 7200 seconds, got %d", off)
				}
				if !tm.Equal(timestampTZ) {
					t.Fatalf("decoded timestamp = %s, want %s", tm.Format(time.RFC3339Nano), timestampTZ.Format(time.RFC3339Nano))
				}
			},
		},
		{
			name:    "interval year to month",
			payload: append(drvCommon.B1Array{osonOpIntervalYM}, intervalYMPayload...),
			assert: func(t *testing.T, got any) {
				if got != "02-03" {
					t.Fatalf("expected Value() to return the interval %q, got %#v", "02-03", got)
				}
			},
		},
		{
			name:    "interval day to second",
			payload: append(drvCommon.B1Array{osonOpIntervalDS}, intervalDSPayload...),
			assert: func(t *testing.T, got any) {
				if got != "10 05:30:02.123" {
					t.Fatalf("expected Value() to return the interval %q, got %#v", "10 05:30:02.123", got)
				}
			},
		},
		{
			name:    "binary ub2",
			payload: drvCommon.B1Array{osonOpBinaryUB2, 0x00, 0x03, 0xaa, 0xbb, 0xcc},
			assert: func(t *testing.T, got any) {
				if !reflect.DeepEqual(got, []byte{0xaa, 0xbb, 0xcc}) {
					t.Fatalf("expected Value() to return []byte{0xaa, 0xbb, 0xcc}, got %#v", got)
				}
			},
		},
		{
			name:    "binary ub4",
			payload: drvCommon.B1Array{osonOpBinaryUB4, 0x00, 0x00, 0x00, 0x03, 0xde, 0xad, 0xbe},
			assert: func(t *testing.T, got any) {
				if !reflect.DeepEqual(got, []byte{0xde, 0xad, 0xbe}) {
					t.Fatalf("expected Value() to return []byte{0xde, 0xad, 0xbe}, got %#v", got)
				}
			},
		},
		{
			name:    "binary id",
			payload: drvCommon.B1Array{osonOpID, 0x03, 0x01, 0x02, 0x03},
			assert: func(t *testing.T, got any) {
				if !reflect.DeepEqual(got, []byte{0x01, 0x02, 0x03}) {
					t.Fatalf("expected Value() to return ID bytes []byte{0x01, 0x02, 0x03}, got %#v", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := newScalarNodeAt(newOsonBuffer(tc.payload), &osonHeader{}, 0, drvCommon.UB1(tc.payload[0]))
			got, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsFloat64})
			if err != nil {
				t.Fatalf("failed to decode the valid scalar payload: %v", err)
			}
			tc.assert(t, got)
		})
	}
}

// TestScalarNode_NumberAsStringOption verifies that numeric scalar opcodes honor
// JSONNumberAsJSONNumber by preserving their textual numeric representation.
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
		payload drvCommon.B1Array
		want    json.Number
	}{
		{
			name:    "compact oracle number",
			payload: append(drvCommon.B1Array{byte(osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(decimalNumberPayload)-_compactNumberLengthBias))}, decimalNumberPayload...),
			want:    json.Number("12.75"),
		},
		{
			name:    "compact oracle array decimal",
			payload: append(drvCommon.B1Array{byte(osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(arrayDecimalPayload)-_compactNumberLengthBias))}, arrayDecimalPayload...),
			want:    json.Number("8.25"),
		},
		{
			name:    "compact oracle decimal scale",
			payload: append(drvCommon.B1Array{byte(osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(decimalScalePayload)-_compactNumberLengthBias))}, decimalScalePayload...),
			want:    json.Number("12345.6789"),
		},
		{
			name:    "compact oracle float64 scale",
			payload: append(drvCommon.B1Array{byte(osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(float64ScalePayload)-_compactNumberLengthBias))}, float64ScalePayload...),
			want:    json.Number("98765.125"),
		},
		{
			name:    "explicit oracle number",
			payload: append(drvCommon.B1Array{osonOpOracleNumber, byte(drvCommon.UB1(len(largeIntegerPayload)))}, largeIntegerPayload...),
			want:    json.Number("1234567890123456"),
		},
		{
			name:    "compact signed integer",
			payload: append(drvCommon.B1Array{byte(osonOpCompactSigned32Prefix | drvCommon.UB1(len(compactIntegerPayload)))}, compactIntegerPayload...),
			want:    json.Number("7"),
		},
		{
			name:    "binary float",
			payload: append(drvCommon.B1Array{osonOpBinaryFloat}, binaryFloatPayload...),
			want:    json.Number("12.5"),
		},
		{
			name:    "string number",
			payload: append(drvCommon.B1Array{osonOpStringNumber, byte(drvCommon.UB1(len(stringNumberText)))}, drvCommon.B1Array(stringNumberText)...),
			want:    json.Number("12.75"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := newScalarNodeAt(newOsonBuffer(tc.payload), &osonHeader{}, 0, drvCommon.UB1(tc.payload[0]))
			got, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
			if err != nil {
				t.Fatalf("failed to decode the numeric payload with JSONNumberAsJSONNumber: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected Value(JSONNumberAsJSONNumber) to return %#v, got %#v", tc.want, got)
			}
		})
	}
}

// TestScalarNode_StringQuotesStringValue expects a decoded OSON string scalar
// to render as a quoted JSON string.
func TestScalarNode_StringQuotesStringValue(t *testing.T) {
	node := newScalarNodeAt(newOsonBuffer(drvCommon.B1Array{0x02, 'o', 'k'}), &osonHeader{}, 0, 0x02)

	text, err := node.String()
	if err != nil {
		t.Fatalf("failed to render the valid short-string scalar as JSON: %v", err)
	}
	if text != `"ok"` {
		t.Fatalf("expected String() to return %q, got %q", `"ok"`, text)
	}
}

// TestScalarNode_MalformedScalarPayloads verifies that malformed scalar
// payloads fail with the expected Oracle error codes.
func TestScalarNode_MalformedScalarPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload drvCommon.B1Array
		want    oracleErrors.ErrorCode
	}{
		{
			name:    "truncated string ub1 payload",
			payload: drvCommon.B1Array{osonOpStringUB1, 0x05, 'h', 'e'},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated string ub2 payload",
			payload: drvCommon.B1Array{osonOpStringUB2, 0x00, 0x05, 'h', 'e'},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated string ub4 payload",
			payload: drvCommon.B1Array{osonOpStringUB4, 0x00, 0x00, 0x00, 0x05, 'h', 'e'},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated explicit oracle number payload",
			payload: drvCommon.B1Array{osonOpOracleNumber, 0x03, 0x01},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "invalid string number payload",
			payload: drvCommon.B1Array{osonOpStringNumber, 0x03, 'x', 'y', 'z'},
			want:    oracleErrors.OsonParsingError,
		},
		{
			name:    "truncated binary float payload",
			payload: drvCommon.B1Array{osonOpBinaryFloat, 0x01},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated binary double payload",
			payload: drvCommon.B1Array{osonOpBinaryDouble, 0x01, 0x02},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated timestamp payload",
			payload: drvCommon.B1Array{osonOpTimestamp, 120, 124, 1, 2},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated date payload",
			payload: drvCommon.B1Array{osonOpDate, 120, 124, 1, 2},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated timestamp7 payload",
			payload: drvCommon.B1Array{osonOpTimestamp7, 120, 124, 1, 2},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated timestamptz payload",
			payload: drvCommon.B1Array{osonOpTimestampTZ, 120, 124, 1, 2},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated interval ds payload",
			payload: drvCommon.B1Array{osonOpIntervalDS, 0x80, 0x00},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated interval ym payload",
			payload: drvCommon.B1Array{osonOpIntervalYM, 0x80, 0x00},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated binary ub2 payload",
			payload: drvCommon.B1Array{osonOpBinaryUB2, 0x00, 0x03, 0x01},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "truncated binary ub4 payload",
			payload: drvCommon.B1Array{osonOpBinaryUB4, 0x00, 0x00, 0x00, 0x03, 0x01},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "empty compact signed32 payload",
			payload: drvCommon.B1Array{osonOpCompactSigned32Prefix},
			want:    oracleErrors.ConverterEmptyInput,
		},
		{
			name:    "empty compact signed64 payload",
			payload: drvCommon.B1Array{osonOpCompactSigned64Prefix},
			want:    oracleErrors.ConverterEmptyInput,
		},
		{
			name:    "truncated id payload",
			payload: drvCommon.B1Array{osonOpID, 0x80},
			want:    oracleErrors.OsonBufferError,
		},
		{
			name:    "reserved update opcode",
			payload: drvCommon.B1Array{osonOpUpdateOversizeReserved},
			want:    oracleErrors.OsonParsingError,
		},
		{
			name:    "known native integer opcode not implemented",
			payload: drvCommon.B1Array{osonOpNativeInteger, 0x01, 0x01},
			want:    oracleErrors.OsonUnsupportedScalarError,
		},
		{
			name:    "known extended binary opcode not implemented",
			payload: drvCommon.B1Array{osonOpExtendedBinary, 0x01, 0x00, 0x00, 0x00, 0x00},
			want:    oracleErrors.OsonUnsupportedScalarError,
		},
		{
			name:    "reserved scalar opcode",
			payload: drvCommon.B1Array{0x7a},
			want:    oracleErrors.OsonParsingError,
		},
		{name: "truncated short string payload", payload: drvCommon.B1Array{1}, want: oracleErrors.OsonBufferError},
		{name: "truncated compact signed32 payload", payload: drvCommon.B1Array{osonOpCompactSigned32Prefix | 1}, want: oracleErrors.OsonBufferError},
		{name: "truncated compact signed64 payload", payload: drvCommon.B1Array{osonOpCompactSigned64Prefix | 1}, want: oracleErrors.OsonBufferError},
		{name: "truncated compact number payload", payload: drvCommon.B1Array{osonOpCompactOracleNumberPrefix}, want: oracleErrors.OsonBufferError},
		{name: "truncated compact decimal payload", payload: drvCommon.B1Array{osonOpCompactDecimalPrefix}, want: oracleErrors.OsonBufferError},
		{name: "truncated string number payload", payload: drvCommon.B1Array{osonOpStringNumber, 1}, want: oracleErrors.OsonBufferError},
	}

	for _, opcode := range []drvCommon.UB1{
		osonOpStringUB1, osonOpStringUB2, osonOpStringUB4,
		osonOpOracleNumber, osonOpStringNumber, osonOpID,
		osonOpBinaryUB2, osonOpBinaryUB4,
	} {
		tests = append(tests, struct {
			name    string
			payload drvCommon.B1Array
			want    oracleErrors.ErrorCode
		}{
			name:    "truncated length prefix " + strconv.Itoa(int(opcode)),
			payload: drvCommon.B1Array{byte(opcode)},
			want:    oracleErrors.OsonBufferError,
		})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := newScalarNodeAt(newOsonBuffer(tc.payload), &osonHeader{}, 0, drvCommon.UB1(tc.payload[0]))

			_, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsFloat64})
			if err == nil {
				t.Fatal("Value() error = nil, want failure")
			}
			assertOracleErrorCode(t, err, tc.want)
		})
	}
}

// TestScalarNode_IDReadsFullUB1Length expects the ID reader to accept a payload at the
// maximum length supported by its length field.
func TestScalarNode_IDReadsFullUB1Length(t *testing.T) {
	const payloadLength = math.MaxUint8
	payload := append(drvCommon.B1Array{osonOpID, byte(payloadLength)}, make([]byte, payloadLength)...)

	node := newScalarNodeAt(newOsonBuffer(payload), &osonHeader{}, 0, drvCommon.UB1(payload[0]))
	got, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsFloat64})
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if raw, ok := got.([]byte); !ok || len(raw) != payloadLength {
		t.Fatalf("Value() = %#v, want []byte of length %d", got, payloadLength)
	}
}

func assertOracleErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()

	oraErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("error type = %T, want oracleErrors.SQLError", err)
	}
	if got := oraErr.ErrorCode(); got != string(want) {
		t.Fatalf("error code = %v, want %v", got, want)
	}
	switch want {
	case oracleErrors.OsonHeaderError,
		oracleErrors.OsonEncodingError,
		oracleErrors.OsonUnsupportedScalarError,
		oracleErrors.JSONRenderingError:
		if errors.Unwrap(err) == nil {
			t.Fatalf("OSON error %s has no cause", want)
		}
	}
}

// TestScalarNode_BinaryFloatSpecialValue verifies that binary float decoding
// preserves special IEEE values such as positive infinity.
func TestScalarNode_BinaryFloatSpecialValue(t *testing.T) {
	payload, _ := converters.EncodeBinaryFloat(float32(math.Inf(1)))
	node := newScalarNodeAt(
		newOsonBuffer(append(drvCommon.B1Array{osonOpBinaryFloat}, payload...)),
		&osonHeader{},
		0,
		osonOpBinaryFloat,
	)

	got, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsFloat64})
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if !math.IsInf(got.(float64), 1) {
		t.Fatalf("expected Value() to preserve positive infinity, got %#v", got)
	}
}

// TestScalarNode_RejectsUnknownOpcode verifies unknown scalar opcodes return a
// parsing error without attempting to interpret their payload.
func TestScalarNode_RejectsUnknownOpcode(t *testing.T) {
	node := &scalarNode{
		nodeBase: nodeBase{buf: newOsonBuffer(drvCommon.B1Array{0x7a})},
		opcode:   0x7a,
	}
	_, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsFloat64})
	if err == nil {
		t.Fatal("Value() error = nil, want unknown-opcode failure")
	}
	assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
}
