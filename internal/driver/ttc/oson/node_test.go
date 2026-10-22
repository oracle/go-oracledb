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
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"testing"
)

// TestDecodeOracleSamples verifies independently produced wire samples
// parse, materialize, and render as expected.
func TestDecodeOracleSamples(t *testing.T) {
	samples := []osonSample{
		sampleScalarNull, sampleScalarTrue, sampleScalarFalse, sampleScalarShortString,
		sampleNumberSmallPositive, sampleNumberSmallNegative, sampleNumberLarge, sampleNumberDecimal,
		sampleString32, sampleString255, sampleString256, sampleBinaryFloat, sampleBinaryDouble,
		sampleDate, sampleTimestamp, sampleTimestamp7, sampleTimestampTZ,
		sampleIntervalYM, sampleIntervalDS, sampleRawBinary,
		sampleEmptyArray, sampleNestedArray, sampleEmptyObject, sampleRepeatedKey, sampleUTF8Key,
		sampleSimpleObject, sampleNestedObjectArray, sampleSecondaryDictionary, sampleUpdatedSecondaryDictionary,
		sampleUpdatedTinyScalar, sampleUpdatedOverflow, sampleUpdatedForwardUB2, sampleUpdatedForwardUB4,
		sampleRelativeOffsets, sampleUpdatedOverflowUB4, sampleSharedObjects, sampleSharedObjectsUpdate,
	}
	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			root, err := Parse(sample.oson)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if _, err := root.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber}); err != nil {
				t.Fatalf("GetValue() error = %v", err)
			}
			if sample.name != sampleTimestampTZ.name && sample.name != sampleIntervalYM.name && sample.name != sampleIntervalDS.name {
				text, err := root.String()
				if err != nil {
					t.Fatalf("String() error = %v", err)
				}
				assertJSONEqual(t, text, sample.json)
			}
		})
	}
}

// TestParseRejectsForwardingCycle verifies node resolution rejects forwarding
// cycles in update redirects.
func TestParseRejectsForwardingCycle(t *testing.T) {
	buf := newOsonBuffer(drvCommon.B1Array{
		osonOpUpdateForwardUB2, 0, 0,
		osonOpUpdateForwardUB2, 0, 0,
	})
	header := &osonHeader{
		treeSegmentByteLength:          3,
		extendedTreeSegmentStartOffset: 3,
		extendedTreeSegmentByteLength:  3,
	}
	if _, err := newNodeAt(buf, header, 0); err == nil {
		t.Fatal("newNodeAt() error = nil, want forwarding-cycle error")
	}
}

// TestParseRecognizesOSON verifies IsOson recognizes valid OSON documents and
// rejects anything else.
func TestParseRecognizesOSON(t *testing.T) {
	if !IsOson(sampleScalarTrue.oson) || !IsOson(sampleSimpleObject.oson) {
		t.Fatal("IsOson() rejected a valid scalar or object document")
	}
	if IsOson(drvCommon.B1Array(`{"value":1}`)) || IsOson(sampleScalarTrue.oson[:3]) {
		t.Fatal("IsOson() accepted JSON text or a truncated OSON prefix")
	}
}
