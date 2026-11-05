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
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestMain initializes the shared repository test configuration before OSON tests run.
func TestMain(m *testing.M) {
	if err := oracleTest.InitConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "InitConfig failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

var testCases = []oracleTest.CategorizedTestCase{
	// Check the bounded, big-endian reads used by the OSON parser.
	{Name: "TestBufferReadsAndBounds", Categories: "unitary", Fn: TestBufferReadsAndBounds},

	// Parse document headers, selected field widths, and header update metadata.
	{Name: "TestParseRecognizesOSON", Categories: "unitary", Fn: TestParseRecognizesOSON},
	{Name: "TestHeaderUsesFlagSelectedWidths", Categories: "unitary", Fn: TestHeaderUsesFlagSelectedWidths},
	{Name: "TestParseRejectsMalformedHeaders", Categories: "unitary", Fn: TestParseRejectsMalformedHeaders},
	{Name: "TestHeaderRejectsInvalidUpdateMetadata", Categories: "unitary", Fn: TestHeaderRejectsInvalidUpdateMetadata},
	{Name: "TestHeaderRejectsUpdateCaps", Categories: "unitary", Fn: TestHeaderRejectsUpdateCaps},
	{Name: "TestHeaderRejectsOutOfRangeUpdateMappings", Categories: "unitary", Fn: TestHeaderRejectsOutOfRangeUpdateMappings},
	{Name: "TestHeaderRejectsMalformedDictionaryHeap", Categories: "unitary", Fn: TestHeaderRejectsMalformedDictionaryHeap},
	{Name: "TestHeaderRejectsImpossibleDictionaryCounts", Categories: "unitary", Fn: TestHeaderRejectsImpossibleDictionaryCounts},

	// Validate container tables and node references before exposing document values.
	{Name: "TestParseRejectsMalformedContainerTables", Categories: "unitary", Fn: TestParseRejectsMalformedContainerTables},
	{Name: "TestParseRejectsInvalidChildOffset", Categories: "unitary", Fn: TestParseRejectsInvalidChildOffset},
	{Name: "TestParseRejectsForwardingCycle", Categories: "unitary", Fn: TestParseRejectsForwardingCycle},
	{Name: "TestParseRejectsDelegateWithoutReferredBit", Categories: "unitary", Fn: TestParseRejectsDelegateWithoutReferredBit},
	{Name: "TestParseRejectsDuplicateFieldID", Categories: "unitary", Fn: TestParseRejectsDuplicateFieldID},

	// Decode valid documents and verify scalar values, object and array access,
	// independent Oracle samples, and JSON rendering.
	{Name: "TestDecodeOracleSamples", Categories: "unitary", Fn: TestDecodeOracleSamples},
	{Name: "TestDecodeNumberModes", Categories: "unitary", Fn: TestDecodeNumberModes},
	{Name: "TestDecodeObjectAPI", Categories: "unitary", Fn: TestDecodeObjectAPI},
	{Name: "TestDecodeContainerNavigation", Categories: "unitary", Fn: TestDecodeContainerNavigation},
	{Name: "TestDecodeSecondaryDictionary", Categories: "unitary", Fn: TestDecodeSecondaryDictionary},
	{Name: "TestDecodeScalarJSONRendering", Categories: "unitary", Fn: TestDecodeScalarJSONRendering},
	{Name: "TestDecodeOtherScalarWireForms", Categories: "unitary", Fn: TestDecodeOtherScalarWireForms},
	{Name: "TestDecodeIDScalarFixture", Categories: "unitary", Fn: TestDecodeIDScalarFixture},

	// Check unsupported scalar opcodes and malformed scalar payloads fail cleanly.
	{Name: "TestDecodeNativeIntegerOpcode", Categories: "unitary", Fn: TestDecodeNativeIntegerOpcode},
	{Name: "TestDecodeMalformedScalarPayload", Categories: "unitary", Fn: TestDecodeMalformedScalarPayload},
	{Name: "TestDecodeMalformedCompactSignedPayloads", Categories: "unitary", Exclusive: true, Fn: TestDecodeMalformedCompactSignedPayloads},
	{Name: "TestDecodeUnsupportedScalarAccess", Categories: "unitary", Exclusive: true, Fn: TestDecodeUnsupportedScalarAccess},

	// Encode scalar values with the expected opcode families and length forms.
	{Name: "TestEncodeSignedIntegerWidths", Categories: "unitary", Fn: TestEncodeSignedIntegerWidths},
	{Name: "TestEncodeOtherScalarTypes", Categories: "unitary", Fn: TestEncodeOtherScalarTypes},
	{Name: "TestEncodeStringLengthForms", Categories: "unitary", Fn: TestEncodeStringLengthForms},
	{Name: "TestEncodeBinaryLengthForms", Categories: "unitary", Fn: TestEncodeBinaryLengthForms},
	{Name: "TestEncodeTimeOptions", Categories: "unitary", Fn: TestEncodeTimeOptions},

	// Encode objects and arrays across child-count, field-ID, dictionary, and
	// child-offset width boundaries, then verify nested values round trip.
	{Name: "TestEncodeContainerCountWidths", Categories: "unitary", Fn: TestEncodeContainerCountWidths},
	{Name: "TestEncodeContainerRoundTrip", Categories: "unitary", Fn: TestEncodeContainerRoundTrip},
	{Name: "TestEncodeLongFieldNames", Categories: "unitary", Fn: TestEncodeLongFieldNames},
	{Name: "TestEncodeWideObjectFieldIDs", Categories: "unitary", Fn: TestEncodeWideObjectFieldIDs},
	{Name: "TestEncodeWidePrimaryDictionaryOffsets", Categories: "unitary", Fn: TestEncodeWidePrimaryDictionaryOffsets},
	{Name: "TestEncodeWideSecondaryDictionaryOffsets", Categories: "unitary", Fn: TestEncodeWideSecondaryDictionaryOffsets},
	{Name: "TestEncodeLargeContainerOffsets", Categories: "unitary", Fn: TestEncodeLargeContainerOffsets},

	// Keep unsupported inputs and documents beyond the format limit out of output.
	{Name: "TestEncodeRejectsUnsupportedValues", Categories: "unitary", Fn: TestEncodeRejectsUnsupportedValues},
	{Name: "TestEncodeRejectsOversizedDocument", Categories: "unitary", Fn: TestEncodeRejectsOversizedDocument},
	{Name: "TestOSONScalarBoundaryContent", Categories: "unitary", Exclusive: true, Fn: TestOSONScalarBoundaryContent},
	{Name: "TestOSONContainerBoundaryContent", Categories: "unitary", Exclusive: true, Fn: TestOSONContainerBoundaryContent},
	{Name: "TestOSONDictionaryHeapBoundaryContent", Categories: "unitary", Exclusive: true, Fn: TestOSONDictionaryHeapBoundaryContent},
	{Name: "TestOSONMixedDictionaryFieldIDBoundary", Categories: "unitary", Exclusive: true, Fn: TestOSONMixedDictionaryFieldIDBoundary},
	{Name: "TestOSONFieldNameMaximum", Categories: "unitary", Exclusive: true, Fn: TestOSONFieldNameMaximum},
	{Name: "TestOSONExactDocumentSizeLimit", Categories: "unitary", Exclusive: true, Fn: TestOSONExactDocumentSizeLimit},
	{Name: "TestOSONNumberTextLengthLimit", Categories: "unitary", Exclusive: true, Fn: TestOSONNumberTextLengthLimit},
}

// main test
func TestCategoryExecutor(t *testing.T) {
	oracleTest.RunCategoryExecutor(t, oracleTest.TestCategories, testCases)
}

// assertJSONEqual compares decoded JSON values so object member order does not matter.
func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	if gotValue, wantValue := decodeJSONForTest(t, got), decodeJSONForTest(t, want); !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON value = %#v, want %#v", gotValue, wantValue)
	}
}

// decodeJSONForTest parses JSON while preserving number text for semantic comparisons.
func decodeJSONForTest(t *testing.T, text string) any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode JSON %q: %v", text, err)
	}
	return value
}

// assertOracleErrorCode checks an OSON failure uses the expected public Oracle error code.
func assertOracleErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) {
		t.Fatalf("error = %v (%T), want oracleErrors.SQLError", err, err)
	}
	if got := sqlErr.ErrorCode(); got != string(want) {
		t.Fatalf("error code = %s, want %s", got, want)
	}
}
