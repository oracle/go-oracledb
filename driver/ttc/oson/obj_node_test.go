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
	"encoding/binary"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

// TestObjectNode_KindReportsObject verifies object nodes report the object JSON kind.
func TestObjectNode_KindReportsObject(t *testing.T) {
	obj := &objectNode{}
	if got := obj.Kind(); got != common.KindObject {
		t.Fatalf("Kind() = %v, want %v", got, common.KindObject)
	}
}

// TestObjectNode_SimpleObjectTraversal verifies key lookup, full materialization, and JSON rendering for the basic object fixture.
func TestObjectNode_SimpleObjectTraversal(t *testing.T) {
	t.Parallel()

	buf := newOsonBuffer(sampleSimpleObject.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}

	root, err := newNodeAt(buf, header, header.treeSegmentOffset())
	if err != nil {
		t.Fatalf("newNodeAt() error = %v", err)
	}

	obj, ok := root.(common.JSONObjectNode)
	if !ok {
		t.Fatalf("root type = %T, want *objectNode", root)
	}

	got, want := obj.Keys(), []string{"name", "role", "active"}

	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}

	nameNode, found := obj.Get("name")
	if !found || nameNode == nil {
		t.Fatalf("Get(name) = (%v, %v), want child", nameNode, found)
	}
	name, err := nameNode.GetValue(common.JSONOptDefault)
	if err != nil {
		t.Fatalf("Get(name).GetValue() error = %v", err)
	}
	if name != "Alice" {
		t.Fatalf("Get(name).GetValue() = %v, want Alice", name)
	}

	gotValue, err := obj.Value(common.JSONOptDefault)
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	wantValue := map[string]any{"name": "Alice", "role": "Developer", "active": true}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("Value() = %#v, want %#v", gotValue, wantValue)
	}

	text, err := obj.StringWithOption(common.JSONOptDefault)
	if err != nil {
		t.Fatalf("StringWithOption() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("StringWithOption() produced invalid JSON %q: %v", text, err)
	}
	if decoded["name"] != "Alice" || decoded["role"] != "Developer" || decoded["active"] != true {
		t.Fatalf("StringWithOption() decoded = %#v, want simple object fields", decoded)
	}
}

// TestObjectNode_SecondaryDictionaryTraversal verifies object lookup works across primary and secondary dictionary tiers.
func TestObjectNode_SecondaryDictionaryTraversal(t *testing.T) {
	t.Parallel()

	root, err := Parse(sampleSecondaryDictionary.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj, ok := root.(common.JSONObjectNode)
	if !ok {
		t.Fatalf("root type = %T, want object", root)
	}
	if got, want := obj.Len(), 2; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	longNode, found := obj.Get(osonLongKey)
	if !found {
		t.Fatal("Get(long key) found = false, want true")
	}
	longValue, err := longNode.GetValue(common.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("long key GetValue() error = %v", err)
	}
	if longValue != common.JSONNumber("1") {
		t.Fatalf("long key value = %#v, want JSONNumber(\"1\")", longValue)
	}

	shortNode, found := obj.Get("short")
	if !found {
		t.Fatal("Get(short) found = false, want true")
	}
	shortValue, err := shortNode.GetValue(common.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("short GetValue() error = %v", err)
	}
	if shortValue != common.JSONNumber("2") {
		t.Fatalf("short value = %#v, want JSONNumber(\"2\")", shortValue)
	}

	if _, found := obj.Get("missing"); found {
		t.Fatal("Get(missing) found = true, want false")
	}
}

// TestObjectNode_RejectsMalformedFieldIDAndChildOffset verifies invalid field IDs and out-of-tree child offsets are rejected.
func TestObjectNode_RejectsMalformedFieldIDAndChildOffset(t *testing.T) {
	t.Parallel()
	buf := newOsonBuffer(sampleSimpleObject.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	treeOffset := header.treeSegmentOffset()

	badFieldID := sampleSimpleObject.cloneOSON()
	badFieldID[treeOffset+2] = 0x04
	if _, err := newObjectNodeAt(newOsonBuffer(badFieldID), header, treeOffset); err == nil {
		t.Fatal("newObjectNodeAt() with missing field id error = nil, want error")
	}

	badChildOffset := sampleSimpleObject.cloneOSON()
	binary.BigEndian.PutUint16(badChildOffset[treeOffset+5:], 0x7fff)
	badRoot, err := newObjectNodeAt(newOsonBuffer(badChildOffset), header, treeOffset)
	if err != nil {
		return
	}
	if _, err := badRoot.Value(common.JSONOptDefault); err == nil {
		t.Fatal("Value() with out-of-tree child offset error = nil, want error")
	}
}

// TestObjectNode_RejectsMalformedLayout verifies object construction fails for non-object opcodes and truncated field-ID tables.
func TestObjectNode_RejectsMalformedLayout(t *testing.T) {
	t.Parallel()

	if _, err := newObjectNodeAt(newOsonBuffer(common.B1Array{osonOpTrue}), &osonHeader{}, 0); err == nil {
		t.Fatal("newObjectNodeAt(non-object) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, common.OsonParsingError)
	}

	header := &osonHeader{
		treeSegmentStartOffset: 0,
		fieldDictionary:        dictionary{fieldNames: []string{"alpha"}},
		primaryFieldsCount:     1,
	}
	if _, err := newObjectNodeAt(newOsonBuffer(common.B1Array{0x84, 0x01}), header, 0); err == nil {
		t.Fatal("newObjectNodeAt(truncated field ids) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, common.OsonBufferError)
	}
}
