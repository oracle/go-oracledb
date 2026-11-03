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
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"math"
	"reflect"
	"testing"
)

// TestDecodeNumberModes verifies NUMBER decoding honors the configured number
// mode.
func TestDecodeNumberModes(t *testing.T) {
	root, err := Parse(sampleNumberLarge.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, err := root.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
	if err != nil {
		t.Fatalf("GetValue(JSONNumber) error = %v", err)
	}
	if got != json.Number("9007199254740993") {
		t.Fatalf("JSON number = %#v, want exact integer text", got)
	}
	got, err = root.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsFloat64})
	if err != nil {
		t.Fatalf("GetValue(float64) error = %v", err)
	}
	if _, ok := got.(float64); !ok {
		t.Fatalf("float mode returned %T, want float64", got)
	}
}

// TestDecodeNativeIntegerOpcode verifies unsupported scalar opcodes fail with
// a clear error instead of a guessed value.
func TestDecodeNativeIntegerOpcode(t *testing.T) {
	value := newScalarNodeAt(newOsonBuffer(drvCommon.B1Array{byte(osonOpNativeInteger)}), &osonHeader{}, 0, osonOpNativeInteger)
	_, err := value.GetValue(drvCommon.JSONConversionOptions{})
	if err == nil {
		t.Fatal("GetValue(native integer) error = nil, want unsupported-scalar error")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok || sqlErr.ErrorCode() != string(oracleErrors.OsonUnsupportedScalarError) {
		t.Fatalf("GetValue(native integer) error = %T %v, want OsonUnsupportedScalarError", err, err)
	}
}

// TestDecodeScalarJSONRendering verifies scalars render as valid JSON text or
// fail when they cannot be represented.
func TestDecodeScalarJSONRendering(t *testing.T) {
	for _, sample := range []osonSample{sampleNumberLarge, sampleRawBinary} {
		t.Run(sample.name, func(t *testing.T) {
			root, err := Parse(sample.oson)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			text, err := root.String()
			if err != nil {
				t.Fatalf("String() error = %v", err)
			}
			assertJSONEqual(t, text, sample.json)
		})
	}
	for _, test := range []struct {
		value any
		want  string
	}{
		{math.NaN(), `"Nan"`},
		{math.Inf(1), `"Inf"`},
		{math.Inf(-1), `"-Inf"`},
		{float32(math.NaN()), `"Nan"`},
		{float32(math.Inf(1)), `"Inf"`},
		{float32(math.Inf(-1)), `"-Inf"`},
	} {
		doc, err := Encode(test.value)
		if err != nil {
			t.Fatalf("Encode(%v) error = %v", test.value, err)
		}
		root, err := Parse(doc)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		text, err := root.MarshalJSON()
		if err != nil || string(text) != test.want {
			t.Fatalf("MarshalJSON(%v) = (%s, %v), want %s", test.value, text, err, test.want)
		}
		rendered, err := root.String()
		if err != nil || rendered != test.want {
			t.Fatalf("String(%v) = (%s, %v), want %s", test.value, rendered, err, test.want)
		}
		for _, mode := range []drvCommon.JSONNumberMode{drvCommon.JSONNumberDefault, drvCommon.JSONNumberAsJSONNumber, drvCommon.JSONNumberAsFloat64} {
			value, err := root.GetValue(drvCommon.JSONConversionOptions{NumberMode: mode})
			if err != nil {
				t.Fatalf("GetValue() error = %v", err)
			}
			got, ok := value.(float64)
			if !ok {
				t.Fatalf("GetValue(mode=%v) = %T, want float64", mode, value)
			}
			if test.want == `"Nan"` && !math.IsNaN(got) ||
				test.want == `"Inf"` && !math.IsInf(got, 1) ||
				test.want == `"-Inf"` && !math.IsInf(got, -1) {
				t.Fatalf("GetValue(%v) = %v, want original nonfinite value", test.value, got)
			}
		}
	}
}

// TestDecodeOtherScalarWireForms verifies scalar wire forms emitted by the
// database decode to the expected values.
func TestDecodeOtherScalarWireForms(t *testing.T) {
	payload, err := converters.EncodeFloat(12.75)
	if err != nil {
		t.Fatalf("EncodeFloat() error = %v", err)
	}
	compactDecimal := append(drvCommon.B1Array{byte(osonOpCompactDecimalPrefix | drvCommon.UB1(len(payload)-1))}, payload...)
	explicitDecimal := append(drvCommon.B1Array{osonOpOracleDecimal, byte(len(payload))}, payload...)
	tests := []struct {
		name string
		doc  drvCommon.B1Array
		want any
	}{
		{"compact DECIMAL", compactDecimal, json.Number("12.75")},
		{"explicit DECIMAL", explicitDecimal, json.Number("12.75")},
		{"ID bytes", drvCommon.B1Array{osonOpID, 3, 0xaa, 0xbb, 0xcc}, []byte{0xaa, 0xbb, 0xcc}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := newScalarNodeAt(newOsonBuffer(test.doc), &osonHeader{}, 0, drvCommon.UB1(test.doc[0]))
			got, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Value() = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestDecodeMalformedScalarPayload verifies malformed forms of scalar
// payloads fail with the appropriate error.
func TestDecodeMalformedScalarPayload(t *testing.T) {
	tests := []struct {
		name string
		doc  drvCommon.B1Array
		code oracleErrors.ErrorCode
	}{
		{"truncated compact string", drvCommon.B1Array{1}, oracleErrors.InternalError},
		{"truncated string UB1", drvCommon.B1Array{osonOpStringUB1, 3, 'a'}, oracleErrors.InternalError},
		{"truncated string UB2", drvCommon.B1Array{osonOpStringUB2, 0, 3, 'a'}, oracleErrors.InternalError},
		{"truncated string UB4", drvCommon.B1Array{osonOpStringUB4, 0, 0, 0, 3, 'a'}, oracleErrors.InternalError},
		{"truncated NUMBER", drvCommon.B1Array{osonOpOracleNumber, 3, 1}, oracleErrors.InternalError},
		{"truncated DECIMAL", drvCommon.B1Array{osonOpOracleDecimal, 3, 1}, oracleErrors.InternalError},
		{"truncated string number", drvCommon.B1Array{osonOpStringNumber, 3, '1'}, oracleErrors.InternalError},
		{"invalid string number", drvCommon.B1Array{osonOpStringNumber, 3, 'x', 'y', 'z'}, oracleErrors.InternalError},
		{"empty string number", drvCommon.B1Array{osonOpStringNumber, 0}, oracleErrors.InternalError},
		{"quoted string number", drvCommon.B1Array{osonOpStringNumber, 3, '"', '1', '"'}, oracleErrors.InternalError},
		{"leading-zero string number", drvCommon.B1Array{osonOpStringNumber, 2, '0', '1'}, oracleErrors.InternalError},
		{"truncated binary float", drvCommon.B1Array{osonOpBinaryFloat, 0, 0}, oracleErrors.InternalError},
		{"truncated binary double", drvCommon.B1Array{osonOpBinaryDouble, 0, 0}, oracleErrors.InternalError},
		{"truncated DATE", drvCommon.B1Array{osonOpDate, 1, 2}, oracleErrors.InternalError},
		{"truncated TIMESTAMP", drvCommon.B1Array{osonOpTimestamp, 1, 2}, oracleErrors.InternalError},
		{"truncated TIMESTAMP7", drvCommon.B1Array{osonOpTimestamp7, 1, 2}, oracleErrors.InternalError},
		{"truncated TIMESTAMP WITH TIME ZONE", drvCommon.B1Array{osonOpTimestampTZ, 1, 2}, oracleErrors.InternalError},
		{"truncated INTERVAL YEAR TO MONTH", drvCommon.B1Array{osonOpIntervalYM, 1, 2}, oracleErrors.InternalError},
		{"truncated INTERVAL DAY TO SECOND", drvCommon.B1Array{osonOpIntervalDS, 1, 2}, oracleErrors.InternalError},
		{"truncated binary UB2", drvCommon.B1Array{osonOpBinaryUB2, 0, 3, 1}, oracleErrors.InternalError},
		{"truncated binary UB4", drvCommon.B1Array{osonOpBinaryUB4, 0, 0, 0, 3, 1}, oracleErrors.InternalError},
		{"truncated ID", drvCommon.B1Array{osonOpID, 3, 1}, oracleErrors.InternalError},
		{"zero-length compact signed32", drvCommon.B1Array{osonOpCompactSigned32Prefix}, oracleErrors.InternalError},
		{"zero-length compact signed64", drvCommon.B1Array{osonOpCompactSigned64Prefix}, oracleErrors.InternalError},
		{"over-long compact signed64", drvCommon.B1Array{osonOpCompactSigned64Prefix | 13, 0}, oracleErrors.InternalError},
		{"unsupported native integer", drvCommon.B1Array{osonOpNativeInteger}, oracleErrors.OsonUnsupportedScalarError},
		{"unsupported extended binary", drvCommon.B1Array{osonOpExtendedBinary}, oracleErrors.OsonUnsupportedScalarError},
		{"unknown scalar opcode", drvCommon.B1Array{0x7a}, oracleErrors.InternalError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := newScalarNodeAt(newOsonBuffer(test.doc), &osonHeader{}, 0, drvCommon.UB1(test.doc[0]))
			for _, mode := range []drvCommon.JSONNumberMode{drvCommon.JSONNumberDefault, drvCommon.JSONNumberAsJSONNumber, drvCommon.JSONNumberAsFloat64} {
				_, err := node.Value(drvCommon.JSONConversionOptions{NumberMode: mode})
				assertOracleErrorCode(t, err, test.code)
			}
			_, err := node.MarshalJSON()
			assertOracleErrorCode(t, err, test.code)
		})
	}
}
