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
** either these or other term.
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
	"math"
	"reflect"
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestOsonDecoderFixtures verifies that every valid database fixture parses and
// materializes. Typed scalar rendering has separate coverage because its JSON
// serialization contract differs from the decoded Go representation.
func TestOsonDecoderFixtures(t *testing.T) {
	tests := []struct {
		sample    osonSample
		checkJSON bool
	}{
		{sampleScalarNull, true},
		{sampleScalarTrue, true},
		{sampleScalarFalse, true},
		{sampleScalarShortString, true},
		{sampleNumberSmallPositive, true},
		{sampleNumberSmallNegative, true},
		{sampleNumberLarge, true},
		{sampleNumberDecimal, true},
		{sampleString32, true},
		{sampleString255, true},
		{sampleString256, true},
		{sampleEmptyArray, true},
		{sampleNestedArray, true},
		{sampleEmptyObject, true},
		{sampleRepeatedKey, true},
		{sampleUTF8Key, true},
		{sampleSimpleObject, true},
		{sampleNestedObjectArray, true},
		{sampleSecondaryDictionary, true},
		{sampleUpdatedSecondaryDictionary, true},
		{sampleUpdatedTinyScalar, true},
		{sampleUpdatedOverflow, true},
		{sampleUpdatedForwardUB2, true},
		{sampleUpdatedForwardUB4, true},
		{sampleRelativeOffsets, true},
		{sampleUpdatedOverflowUB4, true},
		{sampleBinaryFloat, true},
		{sampleBinaryDouble, true},
		{sampleDate, true},
		{sampleTimestamp, true},
		{sampleTimestamp7, true},
		{sampleTimestampTZ, false},
		{sampleIntervalYM, false},
		{sampleIntervalDS, false},
		{sampleRawBinary, true},
		{sampleSharedObjects, true},
		{sampleSharedObjectsUpdate, true},
	}

	for _, test := range tests {
		t.Run(test.sample.name, func(t *testing.T) {
			root, err := Parse(test.sample.oson)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if _, err := root.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber}); err != nil {
				t.Fatalf("GetValue() error = %v", err)
			}
			if !test.checkJSON {
				return
			}
			text, err := root.String()
			if err != nil {
				t.Fatalf("String() error = %v", err)
			}
			assertSameJSON(t, text, test.sample.json)
		})
	}
}

// TestOsonDecoder_RejectsNonJSONBinaryFloatText verifies binary floating
// values remain decodable while String rejects values JSON cannot
// represent, including when nested inside containers.
func TestOsonDecoder_RejectsNonJSONBinaryFloatText(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "scalar", value: math.NaN()},
		{name: "array", value: []any{math.NaN()}},
		{name: "object", value: map[string]any{"value": math.NaN()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Encode(test.value)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			root, err := Parse(doc)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if _, err := root.String(); err == nil {
				t.Fatal("String() error = nil, want JSON encoding failure")
			} else {
				assertOracleErrorCode(t, err, oracleErrors.JSONRenderingError)
			}
		})
	}
}

func assertSameJSON(t *testing.T, got, want string) {
	t.Helper()
	// Compare parsed JSON values so the test does not depend on map key order
	// or on insignificant formatting differences in the serialized document.
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("decoded text is invalid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("fixture JSON is invalid JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %s, want semantic match for %s", got, want)
	}
}
