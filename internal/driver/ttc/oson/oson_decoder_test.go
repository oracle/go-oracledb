/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
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
			if _, err := root.GetValue(drvCommon.JSONOptNumberAsString); err != nil {
				t.Fatalf("GetValue() error = %v", err)
			}
			if !test.checkJSON {
				return
			}
			text, err := root.StringWithOption(drvCommon.JSONOptNumberAsString)
			if err != nil {
				t.Fatalf("StringWithOption() error = %v", err)
			}
			assertSameJSON(t, text, test.sample.json)
		})
	}
}

// TestOsonDecoder_RejectsNonJSONBinaryFloatText verifies binary floating
// values remain decodable while StringWithOption rejects values JSON cannot
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
			if _, err := root.StringWithOption(drvCommon.JSONOptDefault); err == nil {
				t.Fatal("StringWithOption() error = nil, want JSON encoding failure")
			} else {
				assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
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
