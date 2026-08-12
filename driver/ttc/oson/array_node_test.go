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

	"github.com/oracle/go-driver/driver/common"
)

// TestArrayNode_NestedObjectArrayTraversal verifies indexed traversal and JSON materialization of a nested OSON array.
func TestArrayNode_NestedObjectArrayTraversal(t *testing.T) {
	t.Parallel()

	root, err := Parse(sampleNestedObjectArray.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	rootObject, ok := root.(common.JSONObjectNode)
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
	items, ok := itemsNode.(common.JSONArrayNode)
	if !ok {
		t.Fatalf("items type = %T, want array", itemsNode)
	}
	if got, want := items.Kind(), common.KindArray; got != want {
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
	firstItemValue, err := firstItemNode.GetValue(common.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("items[0].GetValue(NumberAsString) error = %v", err)
	}
	if firstItemValue != common.JSONNumber("1") {
		t.Fatalf("items[0] = %#v, want JSONNumber(\"1\")", firstItemValue)
	}

	itemsSlice, err := itemsNode.GetValue(common.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("Value(NumberAsString) error = %v", err)
	}
	want := []any{
		common.JSONNumber("1"),
		true,
		nil,
		map[string]any{"x": []any{map[string]any{"y": "z"}}},
	}

	if !reflect.DeepEqual(itemsSlice, want) {
		t.Fatalf("Value(NumberAsString) = %#v, want %#v", itemsSlice, want)
	}
}

// TestArrayNode_RejectsMalformedArrayWireFormat verifies malformed OSON array layout rejection.
func TestArrayNode_RejectsMalformedArrayWireFormat(t *testing.T) {
	t.Parallel()

	// A scalar true opcode cannot start an OSON array node.
	if _, err := newArrayNodeAt(newOsonBuffer(common.B1Array{osonOpTrue}), &osonHeader{}, 0); err == nil {
		t.Fatal("newArrayNodeAt(non-array) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, common.OsonBufferError)
	}

	zeroOffsetHeader := &osonHeader{treeSegmentStartOffset: 0}
	// The child-count selector requires a two-byte UB2 count, but the fixture supplies only one byte.
	if _, err := newArrayNodeAt(newOsonBuffer(common.B1Array{osonOpArrayType | osonOpChildCountUB2, 0x00}), zeroOffsetHeader, 0); err == nil {
		t.Fatal("newArrayNodeAt(truncated count) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, common.OsonBufferError)
	}

	// Child offsets are absolute document offsets; 9 lies beyond this one-byte test buffer.
	invalidChildOffset := 9
	arrayWithInvalidChildOffset := &arrayNode{
		nodeBase:     nodeBase{buf: newOsonBuffer(common.B1Array{osonOpTrue}), header: zeroOffsetHeader},
		childOffsets: []int{invalidChildOffset},
	}
	if _, ok := arrayWithInvalidChildOffset.Get(0); ok {
		t.Fatal("Get(0) with invalid child offset found = true, want false")
	}
	if _, err := arrayWithInvalidChildOffset.Value(common.JSONOptDefault); err == nil {
		t.Fatal("Value() with invalid child offset error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, common.OsonBufferError)
	}
}
