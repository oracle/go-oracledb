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
	"reflect"
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestArrayNode_NestedObjectArrayTraversal verifies indexed traversal and JSON materialization of a nested OSON array.
func TestArrayNode_NestedObjectArrayTraversal(t *testing.T) {
	t.Parallel()

	root, err := Parse(sampleNestedObjectArray.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	rootObject, ok := root.(drvCommon.JSONObjectNode)
	if !ok {
		t.Fatalf("root type = %T, want object", root)
	}
	if got, want := rootObject.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	itemsNode, found := rootObject.Get("items")
	if !found {
		t.Fatal("Get(items) found = false, want true")
	}
	items, ok := itemsNode.(drvCommon.JSONArrayNode)
	if !ok {
		t.Fatalf("items type = %T, want array", itemsNode)
	}
	if got, want := items.Kind(), drvCommon.KindArray; got != want {
		t.Fatalf("items.Kind() = %v, want %v", got, want)
	}
	if got, want := items.Len(), 4; got != want {
		t.Fatalf("items.Len() = %d, want %d", got, want)
	}
	if _, found := items.Get(-1); found {
		t.Fatal("items.Get(-1) found = true, want false")
	}
	if _, found := items.Get(items.Len()); found {
		t.Fatal("items.Get(len) found = true, want false")
	}

	firstItemNode, found := items.Get(0)
	if !found {
		t.Fatal("items.Get(0) found = false, want true")
	}
	firstItemValue, err := firstItemNode.GetValue(drvCommon.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("items[0].GetValue(NumberAsString) error = %v", err)
	}
	if firstItemValue != drvCommon.JSONNumber("1") {
		t.Fatalf("items[0] = %#v, want JSONNumber(\"1\")", firstItemValue)
	}

	itemsSlice, err := itemsNode.GetValue(drvCommon.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("Value(NumberAsString) error = %v", err)
	}
	want := []any{
		drvCommon.JSONNumber("1"),
		true,
		nil,
		map[string]any{"x": []any{map[string]any{"y": "z"}}},
	}

	if !reflect.DeepEqual(itemsSlice, want) {
		t.Fatalf("Value(NumberAsString) = %#v, want %#v", itemsSlice, want)
	}
}

// TestArrayNode_RejectsMalformedLayouts verifies malformed array layouts and
// invalid array-only opcode combinations.
func TestArrayNode_RejectsMalformedLayouts(t *testing.T) {
	t.Parallel()

	t.Run("non-array opcode", func(t *testing.T) {
		if _, err := newArrayNodeAt(newOsonBuffer(drvCommon.B1Array{osonOpTrue}), &osonHeader{}, 0); err == nil {
			t.Fatal("newArrayNodeAt(non-array) error = nil, want failure")
		} else {
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
		}
	})

	t.Run("truncated child count", func(t *testing.T) {
		zeroOffsetHeader := &osonHeader{treeSegmentStartOffset: 0}
		if _, err := newArrayNodeAt(newOsonBuffer(drvCommon.B1Array{osonOpArrayType | osonOpChildCountUB2, 0x00}), zeroOffsetHeader, 0); err == nil {
			t.Fatal("newArrayNodeAt(truncated count) error = nil, want failure")
		} else {
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
		}
	})

	t.Run("invalid child offset", func(t *testing.T) {
		zeroOffsetHeader := &osonHeader{treeSegmentStartOffset: 0}
		arrayWithInvalidChildOffset := &arrayNode{
			nodeBase:     nodeBase{buf: newOsonBuffer(drvCommon.B1Array{osonOpTrue}), header: zeroOffsetHeader},
			childOffsets: []int{9},
		}
		if _, ok := arrayWithInvalidChildOffset.Get(0); ok {
			t.Fatal("Get(0) with invalid child offset found = true, want false")
		}
		if _, err := arrayWithInvalidChildOffset.Value(drvCommon.JSONOptDefault); err == nil {
			t.Fatal("Value() with invalid child offset error = nil, want failure")
		} else {
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
		}
	})

	t.Run("impossible child-offset table", func(t *testing.T) {
		doc := sampleNestedArray.cloneOSON()
		header, err := newOsonHeader(newOsonBuffer(doc))
		if err != nil {
			t.Fatal(err)
		}
		doc[header.treeSegmentOffset()+1] = 0xff
		if _, err := Parse(doc); err == nil {
			t.Fatal("Parse() error = nil, want impossible-offset-table failure")
		} else {
			assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
		}
	})

	t.Run("object-only opcode flags", func(t *testing.T) {
		header := &osonHeader{treeSegmentStartOffset: 0, treeSegmentByteLength: 2}
		for _, test := range []struct {
			name   string
			opcode drvCommon.UB1
		}{
			{name: "unsorted field IDs", opcode: osonOpArrayType | osonOpChildNoSortBit},
			{name: "shared field IDs", opcode: osonOpArrayType | osonOpObjectSharedFieldIDsBit},
			{name: "overflow object", opcode: osonOpArrayType | osonOpObjectUpdateOverflowBit},
			{name: "delegate child header", opcode: osonOpArrayType | osonOpChildDelegateForm},
		} {
			t.Run(test.name, func(t *testing.T) {
				_, err := newArrayNodeAt(newOsonBuffer(drvCommon.B1Array{byte(test.opcode), 0x00}), header, 0)
				if err == nil {
					t.Fatalf("newArrayNodeAt(opcode=%#02x) error = nil, want failure", test.opcode)
				}
				assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
			})
		}
	})

	if _, err := newArrayNodeAt(newOsonBuffer(nil), &osonHeader{}, 0); err == nil {
		t.Fatal("newArrayNodeAt(empty) error = nil, want out-of-range failure")
	}
}
