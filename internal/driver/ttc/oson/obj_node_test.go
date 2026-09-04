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

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestObjectNode_KindReportsObject verifies object nodes report the object JSON kind.
func TestObjectNode_KindReportsObject(t *testing.T) {
	obj := &objectNode{}
	if got := obj.Kind(); got != drvCommon.KindObject {
		t.Fatalf("Kind() = %v, want %v", got, drvCommon.KindObject)
	}
}

// TestObjectNode_ReadersRejectTruncatedLayouts exercises object
// metadata readers directly for each supported field-id width and delegate
// reference width.
func TestObjectNode_ReadersRejectTruncatedLayouts(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		flags drvCommon.UB2
		bytes drvCommon.B1Array
	}{
		{"field ID UB1", 0, nil},
		{"field ID UB2", osonFlagDistinctFieldCountUB2Mask, drvCommon.B1Array{0}},
		{"field ID UB4", osonFlagDistinctFieldCountUB4Mask, drvCommon.B1Array{0, 0, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := &osonHeader{flags: test.flags}
			if _, err := readFieldIDEntriesAt(newOsonBuffer(test.bytes), header, 0, 1); err == nil {
				t.Fatal("readFieldIDEntriesAt() error = nil, want truncation")
			}
		})
	}

	for _, test := range []struct {
		name   string
		opcode drvCommon.UB1
		bytes  drvCommon.B1Array
	}{
		{"direct count", osonOpObjectType | osonOpChildCountUB2, drvCommon.B1Array{osonOpObjectType | osonOpChildCountUB2}},
		{"delegate UB2", osonOpObjectType | osonOpChildDelegateForm, drvCommon.B1Array{osonOpObjectType | osonOpChildDelegateForm}},
		{"delegate UB4", osonOpObjectType | osonOpChildDelegateForm | osonOpChildOffsetUB4Bit, drvCommon.B1Array{osonOpObjectType | osonOpChildDelegateForm | osonOpChildOffsetUB4Bit}},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := &osonHeader{treeSegmentStartOffset: 0}
			if _, _, _, err := readObjectLayout(newOsonBuffer(test.bytes), header, 0, test.opcode); err == nil {
				t.Fatal("readObjectLayout() error = nil, want malformed-layout failure")
			}
		})
	}

	if _, err := newObjectNodeAt(newOsonBuffer(nil), &osonHeader{}, 0); err == nil {
		t.Fatal("newObjectNodeAt() error = nil, want out-of-range failure")
	}
	if _, _, _, err := readObjectLayout(newOsonBuffer(drvCommon.B1Array{osonOpObjectType | 0x03}), &osonHeader{}, 0, osonOpObjectType|0x03); err == nil {
		t.Fatal("readObjectLayout(unsupported count) error = nil, want failure")
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

	obj, ok := root.(drvCommon.JSONObjectNode)
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
	name, err := nameNode.GetValue(drvCommon.JSONOptDefault)
	if err != nil {
		t.Fatalf("Get(name).GetValue() error = %v", err)
	}
	if name != "Alice" {
		t.Fatalf("Get(name).GetValue() = %v, want Alice", name)
	}

	gotValue, err := obj.Value(drvCommon.JSONOptDefault)
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	wantValue := map[string]any{"name": "Alice", "role": "Developer", "active": true}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("Value() = %#v, want %#v", gotValue, wantValue)
	}

	text, err := obj.StringWithOption(drvCommon.JSONOptDefault)
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

// TestObjectNode_GetRejectsMalformedChild verifies keyed lazy access returns
// false when a database-produced object is modified to contain an invalid
// child redirect.
func TestObjectNode_GetRejectsMalformedChild(t *testing.T) {
	doc := sampleSimpleObject.cloneOSON()
	root, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj, ok := root.(*objectNode)
	if !ok {
		t.Fatalf("root type = %T, want *objectNode", root)
	}
	childOffset, ok := obj.childrenOffsets["name"]
	if !ok {
		t.Fatal("name child offset is missing")
	}
	doc[childOffset] = byte(osonOpUpdateForwardUB2)
	binary.BigEndian.PutUint16(doc[childOffset+1:], 0xffff)

	if _, ok := obj.Get("name"); ok {
		t.Fatal("Get(name) found = true, want false for malformed child")
	}
}

// TestObjectNode_SecondaryDictionaryTraversal verifies object lookup works across primary and secondary dictionary tiers.
func TestObjectNode_SecondaryDictionaryTraversal(t *testing.T) {
	t.Parallel()

	root, err := Parse(sampleSecondaryDictionary.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj, ok := root.(drvCommon.JSONObjectNode)
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
	longValue, err := longNode.GetValue(drvCommon.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("long key GetValue() error = %v", err)
	}
	if longValue != drvCommon.JSONNumber("1") {
		t.Fatalf("long key value = %#v, want JSONNumber(\"1\")", longValue)
	}

	shortNode, found := obj.Get("short")
	if !found {
		t.Fatal("Get(short) found = false, want true")
	}
	shortValue, err := shortNode.GetValue(drvCommon.JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("short GetValue() error = %v", err)
	}
	if shortValue != drvCommon.JSONNumber("2") {
		t.Fatalf("short value = %#v, want JSONNumber(\"2\")", shortValue)
	}

	if _, found := obj.Get("missing"); found {
		t.Fatal("Get(missing) found = true, want false")
	}
}

// TestObjectNode_RejectsMalformedLayouts groups object-layout failures while
// keeping each malformed wire case visible as a named subtest.
func TestObjectNode_RejectsMalformedLayouts(t *testing.T) {
	t.Parallel()
	t.Run("duplicate field IDs", testObjectNodeRejectsDuplicateFieldIDs)
	t.Run("invalid field ID and child offset", testObjectNodeRejectsMalformedFieldIDAndChildOffset)
	t.Run("invalid opcode and truncated field IDs", testObjectNodeRejectsMalformedLayout)
	t.Run("impossible field-ID tables", testObjectNodeRejectsImpossibleFieldIDTables)
}

// testObjectNodeRejectsDuplicateFieldIDs verifies malformed objects cannot
// silently overwrite a member while materializing into a Go map.
func testObjectNodeRejectsDuplicateFieldIDs(t *testing.T) {
	doc := sampleSimpleObject.cloneOSON()
	buf := newOsonBuffer(doc)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatal(err)
	}
	// The simple object has a UB1 FID array directly after [opcode][count].
	fidStart := header.treeSegmentOffset() + 2
	doc[fidStart+1] = doc[fidStart]
	if _, err := Parse(doc); err == nil {
		t.Fatal("Parse() error = nil, want duplicate-field-id failure")
	} else {
		assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
	}
}

// testObjectNodeRejectsMalformedFieldIDAndChildOffset verifies invalid field IDs and out-of-tree child offsets are rejected.
func testObjectNodeRejectsMalformedFieldIDAndChildOffset(t *testing.T) {
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
	if _, err := badRoot.Value(drvCommon.JSONOptDefault); err == nil {
		t.Fatal("Value() with out-of-tree child offset error = nil, want error")
	}
}

// testObjectNodeRejectsMalformedLayout verifies object construction fails for non-object opcodes and truncated field-ID tables.
func testObjectNodeRejectsMalformedLayout(t *testing.T) {
	t.Parallel()

	if _, err := newObjectNodeAt(newOsonBuffer(drvCommon.B1Array{osonOpTrue}), &osonHeader{}, 0); err == nil {
		t.Fatal("newObjectNodeAt(non-object) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
	}

	header := &osonHeader{
		treeSegmentStartOffset: 0,
		fieldDictionary:        dictionary{fieldNames: []string{"alpha"}},
		primaryFieldsCount:     1,
	}
	if _, err := newObjectNodeAt(newOsonBuffer(drvCommon.B1Array{0x84, 0x01}), header, 0); err == nil {
		t.Fatal("newObjectNodeAt(truncated field ids) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
	}
}

// testObjectNodeRejectsImpossibleFieldIDTables verifies corrupted child
// counts cannot force field-ID table allocation beyond the OSON document.
func testObjectNodeRejectsImpossibleFieldIDTables(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(drvCommon.B1Array, int)
	}{
		{
			name: "UB1 count exceeds tree",
			mutate: func(doc drvCommon.B1Array, treeStart int) {
				doc[treeStart+1] = 0xff
			},
		},
		{
			name: "UB4 count exceeds tree",
			mutate: func(doc drvCommon.B1Array, treeStart int) {
				doc[treeStart] = (doc[treeStart] &^ byte(osonOpChildSizeBits)) | byte(osonOpChildCountUB4)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := sampleSimpleObject.cloneOSON()
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(doc, header.treeSegmentOffset())
			if _, err := Parse(doc); err == nil {
				t.Fatal("Parse() error = nil, want impossible-table failure")
			} else {
				assertOracleErrorCode(t, err, oracleErrors.OsonBufferError)
			}
		})
	}
}

// TestObjectNode_SharedOverflowUsesPrimaryTreeOffsets verifies delegated
// objects resolve their child offsets from the primary-tree address origin.
func TestObjectNode_SharedOverflowUsesPrimaryTreeOffsets(t *testing.T) {
	candidate := sampleSharedObjects.cloneOSON()
	buf := newOsonBuffer(candidate)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatal(err)
	}
	candidate[3] = 0x02
	candidate[header.treeSegmentStartOffset+0x56] = 0x87
	candidate = append(candidate,
		0x01, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x04,
		0x00, 0x00, 0x00, 0x08,
		0x00, 0x56, 0x00, 0x00,
		0x84, 0x02, 0x01, 0x02, 0x00, 0x0C, 0x00, 0x08,
	)
	root, err := Parse(candidate)
	if err != nil {
		t.Fatal(err)
	}
	value, err := root.GetValue(drvCommon.JSONOptNumberAsString)
	if err != nil {
		t.Fatal(err)
	}
	records := value.(map[string]any)["identical_records"].([]any)
	if len(records) != 32 {
		t.Fatalf("record count = %d, want 32", len(records))
	}
	if _, exists := records[0].(map[string]any)["delegate_name"]; exists {
		t.Fatal("updated primary record retains deleted field")
	}
	if got := records[1].(map[string]any)["delegate_name"]; got != "same" {
		t.Fatalf("delegated record name = %v, want same", got)
	}
}

// TestObjectNode_RejectsInvalidDelegateReferences verifies shared-FID objects
// cannot point at another delegate object or a non-object tree node.
func TestObjectNode_RejectsInvalidDelegateReferences(t *testing.T) {
	for _, test := range []struct {
		name string
		find func(drvCommon.B1Array, int) int
		want oracleErrors.ErrorCode
	}{
		{
			name: "delegate object points to itself",
			want: oracleErrors.OsonParsingError,
			find: func(doc drvCommon.B1Array, treeStart int) int {
				for offset := treeStart; offset < len(doc); offset++ {
					if doc[offset] == byte(0x9c) {
						return offset
					}
				}
				return -1
			},
		},
		{
			name: "delegate object points to scalar",
			want: oracleErrors.OsonParsingError,
			find: func(doc drvCommon.B1Array, treeStart int) int {
				for offset := treeStart; offset < len(doc); offset++ {
					if doc[offset] == byte(osonOpTrue) {
						return offset
					}
				}
				return -1
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := sampleSharedObjects.cloneOSON()
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatal(err)
			}
			delegateOffset := -1
			for offset := header.treeSegmentOffset(); offset < len(doc); offset++ {
				if doc[offset] == byte(0x9c) {
					delegateOffset = offset
					break
				}
			}
			if delegateOffset < 0 {
				t.Fatal("fixture does not contain a UB2 delegated object")
			}
			targetOffset := test.find(doc, header.treeSegmentOffset())
			if targetOffset < 0 {
				t.Fatal("fixture does not contain the requested delegate target")
			}
			binary.BigEndian.PutUint16(doc[delegateOffset+1:], uint16(targetOffset-header.treeSegmentOffset()))

			root, err := Parse(doc)
			if err == nil {
				_, err = root.GetValue(drvCommon.JSONOptDefault)
			}
			if err == nil {
				t.Fatal("GetValue() error = nil, want invalid-delegate failure")
			}
			assertOracleErrorCode(t, err, test.want)
		})
	}
}

func TestReadFieldIDEntriesAt_ReadsAllSupportedWidths(t *testing.T) {
	tests := []struct {
		name   string
		header osonHeader
		data   drvCommon.B1Array
		want   []int
	}{
		{
			name: "ub1",
			data: drvCommon.B1Array{0x01, 0xFE},
			want: []int{1, 254},
		},
		{
			name:   "ub2",
			header: osonHeader{flags: osonFlagDistinctFieldCountUB2Mask},
			data:   drvCommon.B1Array{0x00, 0x01, 0x01, 0x00},
			want:   []int{1, 256},
		},
		{
			name:   "ub4",
			header: osonHeader{flags: osonFlagDistinctFieldCountUB4Mask},
			data:   drvCommon.B1Array{0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00},
			want:   []int{1, 65536},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readFieldIDEntriesAt(newOsonBuffer(tt.data), &tt.header, 0, len(tt.want))
			if err != nil {
				t.Fatalf("readFieldIDEntriesAt() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("field IDs = %v, want %v", got, tt.want)
			}
		})
	}
}
