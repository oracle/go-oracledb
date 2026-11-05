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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDecodeUnsupportedScalarAccess retains the #132 capability boundary and
// verifies lazy access to supported siblings after unsupported scalar errors.
// The synthetic scalar payloads are deliberately uninterpreted for these opcodes.
func TestDecodeUnsupportedScalarAccess(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		opcode byte
	}{{"native_integer", 0x79}, {"extended_binary", 0x7b}} {
		t.Run(fixture.name, func(t *testing.T) {
			const marker = "UNSUPPORTED_SCALAR_MARKER"
			root := osonWithUnsupportedScalar(t, map[string]any{"bad": marker, "good": "survives"}, marker, fixture.opcode)
			object, ok := root.(drvCommon.JSONObjectNode)
			if !ok {
				t.Fatalf("root type = %T, want JSONObjectNode", root)
			}
			bad, err := object.Get("bad")
			if err != nil {
				t.Fatalf("construct lazy scalar: %v", err)
			}
			if got := bad.Kind(); got != drvCommon.KindScalar {
				t.Fatalf("unsupported scalar Kind() = %v, want scalar", got)
			}
			scalar, ok := bad.(drvCommon.JSONScalarNode)
			if !ok {
				t.Fatalf("child type = %T, want JSONScalarNode", bad)
			}
			for _, mode := range []drvCommon.JSONNumberMode{drvCommon.JSONNumberDefault, drvCommon.JSONNumberAsJSONNumber, drvCommon.JSONNumberAsFloat64} {
				opts := drvCommon.JSONConversionOptions{NumberMode: mode}
				_, err := bad.GetValue(opts)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
				_, err = scalar.Value(opts)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
				_, err = root.GetValue(opts)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
				_, err = object.Value(opts)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
			}
			for _, node := range []drvCommon.JSONNode{root, object, bad, scalar} {
				_, err := json.Marshal(node)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
				_, err = node.String()
				assertOracleErrorCode(t, err, oracleErrors.JSONRenderingError)
			}
			good, err := object.Get("good")
			if err != nil {
				t.Fatal(err)
			}
			if value, err := good.GetValue(drvCommon.JSONConversionOptions{}); err != nil || value != "survives" {
				t.Fatalf("supported object sibling after errors = (%v, %v)", value, err)
			}

			arrayRoot := osonWithUnsupportedScalar(t, []any{marker, "survives"}, marker, fixture.opcode)
			array, ok := arrayRoot.(drvCommon.JSONArrayNode)
			if !ok {
				t.Fatalf("root type = %T, want JSONArrayNode", arrayRoot)
			}
			if got := array.Len(); got != 2 {
				t.Fatalf("array Len() = %d, want 2", got)
			}
			bad, err = array.Get(0)
			if err != nil {
				t.Fatalf("construct lazy array scalar: %v", err)
			}
			for _, mode := range []drvCommon.JSONNumberMode{drvCommon.JSONNumberDefault, drvCommon.JSONNumberAsJSONNumber, drvCommon.JSONNumberAsFloat64} {
				opts := drvCommon.JSONConversionOptions{NumberMode: mode}
				_, err := bad.GetValue(opts)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
				_, err = array.Value(opts)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
			}
			for _, node := range []drvCommon.JSONNode{arrayRoot, array, bad} {
				_, err := json.Marshal(node)
				assertOracleErrorCode(t, err, oracleErrors.OsonUnsupportedScalarError)
				_, err = node.String()
				assertOracleErrorCode(t, err, oracleErrors.JSONRenderingError)
			}
			good, err = array.Get(1)
			if err != nil {
				t.Fatal(err)
			}
			if value, err := good.GetValue(drvCommon.JSONConversionOptions{}); err != nil || value != "survives" {
				t.Fatalf("supported array sibling after errors = (%v, %v)", value, err)
			}
			if text, err := json.Marshal(good); err != nil || string(text) != "\"survives\"" {
				t.Fatalf("supported array sibling rendering = (%s, %v)", text, err)
			}
		})
	}
}

func osonWithUnsupportedScalar(t *testing.T, input any, marker string, opcode byte) drvCommon.JSONNode {
	t.Helper()
	data, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode(fixture): %v", err)
	}
	at := bytes.Index(data, []byte(marker))
	if at < 1 || bytes.Count(data, []byte(marker)) != 1 || data[at-1] != byte(len(marker)) {
		t.Fatal("unexpected compact-string fixture layout")
	}
	data[at-1] = opcode
	root, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse must preserve lazy access to supported sibling: %v", err)
	}
	return root
}

// TestDecodeObjectAPI verifies object node access API, including lookup and full
// materialization.
func TestDecodeObjectAPI(t *testing.T) {
	root, err := Parse(sampleSimpleObject.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj, ok := root.(drvCommon.JSONObjectNode)
	if !ok {
		t.Fatalf("root type = %T, want JSONObjectNode", root)
	}
	if got, want := obj.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	keys := obj.Keys()
	slices.Sort(keys)
	if want := []string{"active", "name", "role"}; !slices.Equal(keys, want) {
		t.Fatalf("Keys() = %v, want %v", keys, want)
	}
	for _, key := range []string{"active", "name", "role"} {
		if _, err := obj.Get(key); err != nil {
			t.Errorf("Get(%q) error = %v", key, err)
		}
	}
	_, err = obj.Get("missing")
	assertOracleErrorCode(t, err, oracleErrors.JSONObjectKeyNotFoundError)
	got, err := obj.Value(drvCommon.JSONConversionOptions{})
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	want := map[string]any{"active": true, "name": "Alice", "role": "Developer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Value() = %#v, want %#v", got, want)
	}
}

// TestDecodeSecondaryDictionary verifies object members resolve through both
// name dictionaries.
func TestDecodeSecondaryDictionary(t *testing.T) {
	root, err := Parse(sampleSecondaryDictionary.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj, ok := root.(drvCommon.JSONObjectNode)
	if !ok {
		t.Fatalf("root type = %T, want JSONObjectNode", root)
	}
	for key, want := range map[string]json.Number{osonLongKey: "1", "short": "2"} {
		node, err := obj.Get(key)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", key, err)
		}
		got, err := node.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
		if err != nil {
			t.Fatalf("GetValue(%q) error = %v", key, err)
		}
		if got != want {
			t.Fatalf("GetValue(%q) = %#v, want %s", key, got, want)
		}
	}
}

// TestDecodeContainerNavigation verifies navigation and materialization of
// nested objects and arrays, including their child-count prefixes.
func TestDecodeContainerNavigation(t *testing.T) {
	for _, test := range []struct {
		name   string
		data   drvCommon.B1Array
		opcode drvCommon.UB1
	}{
		{"empty", drvCommon.B1Array{0}, osonOpChildCountUB1},
		{"one-byte count", drvCommon.B1Array{0xff}, osonOpChildCountUB1},
		{"two-byte count", drvCommon.B1Array{0x01, 0x00}, osonOpChildCountUB2},
		{"four-byte count", drvCommon.B1Array{0x00, 0x01, 0x00, 0x00}, osonOpChildCountUB4},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, kind := range []drvCommon.UB1{osonOpArrayType, osonOpObjectType} {
				// Read past an opcode byte to verify nonzero-offset handling.
				opcode := kind | test.opcode
				buf := newOsonBuffer(append(drvCommon.B1Array{byte(opcode)}, test.data...))
				_, _, err := readContainerCountAt(buf, 1, opcode)
				if err != nil {
					t.Fatalf("readContainerCountAt() error = %v", err)
				}
			}
		})
	}

	root, err := Parse(sampleNestedObjectArray.oson)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj, ok := root.(drvCommon.JSONObjectNode)
	if !ok {
		t.Fatalf("root type = %T, want JSONObjectNode", root)
	}
	itemsNode, err := obj.Get("items")
	if err != nil {
		t.Fatalf("Get(items) error = %v", err)
	}
	items, ok := itemsNode.(drvCommon.JSONArrayNode)
	if !ok {
		t.Fatalf("items type = %T, want JSONArrayNode", itemsNode)
	}
	if got, want := items.Len(), 4; got != want {
		t.Fatalf("items.Len() = %d, want %d", got, want)
	}
	for _, index := range []int{-1, items.Len()} {
		_, err := items.Get(index)
		assertOracleErrorCode(t, err, oracleErrors.JSONArrayIndexOutOfRangeError)
	}
	first, err := items.Get(0)
	if err != nil {
		t.Fatalf("Get(0) error = %v", err)
	}
	if got, err := first.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber}); err != nil || got != json.Number("1") {
		t.Fatalf("Get(0).GetValue() = %#v, %v; want json.Number(1)", got, err)
	}
	got, err := itemsNode.GetValue(drvCommon.JSONConversionOptions{NumberMode: drvCommon.JSONNumberAsJSONNumber})
	if err != nil {
		t.Fatalf("items.GetValue() error = %v", err)
	}
	want := []any{
		json.Number("1"), true, nil,
		map[string]any{"x": []any{map[string]any{"y": "z"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items value = %#v, want %#v", got, want)
	}
	text, err := items.String()
	if err != nil {
		t.Fatalf("items.String() error = %v", err)
	}
	assertJSONEqual(t, text, `[1,true,null,{"x":[{"y":"z"}]}]`)
}

// TestParseRejectsMalformedContainerTables verifies Parse rejects malformed
// forms of container tables.
func TestParseRejectsMalformedContainerTables(t *testing.T) {
	for _, test := range []struct {
		name   string
		data   drvCommon.B1Array
		opcode drvCommon.UB1
	}{
		{"truncated one-byte count", nil, osonOpChildCountUB1},
		{"truncated two-byte count", drvCommon.B1Array{0x01}, osonOpChildCountUB2},
		{"truncated four-byte count", drvCommon.B1Array{0x00, 0x01, 0x00}, osonOpChildCountUB4},
		{"invalid count width", nil, osonOpChildSizeBits},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, kind := range []drvCommon.UB1{osonOpArrayType, osonOpObjectType} {
				opcode := kind | test.opcode
				buf := newOsonBuffer(append(drvCommon.B1Array{byte(opcode)}, test.data...))
				_, _, err := readContainerCountAt(buf, 1, opcode)
				assertOracleErrorCode(t, err, oracleErrors.InternalError)
			}
		})
	}

	badArray := sampleNestedArray.cloneOSON()
	arrayHeader, err := newOsonHeader(newOsonBuffer(badArray))
	if err != nil {
		t.Fatalf("newOsonHeader(array fixture) error = %v", err)
	}
	badArray[arrayHeader.treeSegmentOffset()+1] = 0xff
	if _, err := Parse(badArray); err == nil {
		t.Fatal("Parse(malformed array count) error = nil, want layout error")
	}

	badObject := sampleSimpleObject.cloneOSON()
	objectHeader, err := newOsonHeader(newOsonBuffer(badObject))
	if err != nil {
		t.Fatalf("newOsonHeader(object fixture) error = %v", err)
	}
	firstFieldID := objectHeader.treeSegmentOffset() + 2
	badObject[firstFieldID] = 0
	if _, err := Parse(badObject); err == nil {
		t.Fatal("Parse(invalid object field ID) error = nil, want parsing error")
	}

	for _, objectFlag := range []drvCommon.UB1{osonOpChildNoSortBit, osonOpObjectSharedFieldIDsBit, osonOpObjectUpdateOverflowBit} {
		badArray := sampleNestedArray.cloneOSON()
		badArray[arrayHeader.treeSegmentOffset()] = byte(osonOpArrayType | objectFlag)
		if _, err := Parse(badArray); err == nil {
			t.Errorf("Parse(array opcode 0x%02x) error = nil, want object-only-flag rejection", osonOpArrayType|objectFlag)
		}
	}
}

// TestParseRejectsDelegateWithoutReferredBit verifies a field-ID reference
// object rejects delegate objects that do not own a shared field-ID array.
func TestParseRejectsDelegateWithoutReferredBit(t *testing.T) {
	// Tree layout:
	//   offset 0: referring object [0x98][delegate ref=5][child offset=9]
	//   offset 5: delegate object  [0x80][count=1][fid=1][child offset=9]
	// The delegate opcode 0x80 omits the shared-field-IDs bit (0x02), so it
	// cannot own the field-ID array the referring object tries to reuse.
	buf := newOsonBuffer(drvCommon.B1Array{
		osonOpObjectType | osonOpChildDelegateForm, 0x00, 0x05, 0x00, 0x09,
		osonOpObjectType, 1, 1, 0x00, 0x09,
	})
	// One-entry dictionary lets fid=1 resolve, so the only failure left is
	// the delegate ownership check itself.
	header := &osonHeader{
		treeSegmentByteLength: 10,
		fieldDictionary:       dictionary{fieldNames: []string{"a"}},
	}
	_, err := newNodeAt(buf, header, 0)
	if err == nil {
		t.Fatal("newNodeAt() error = nil, want delegate-ownership error")
	}
	assertOracleErrorCode(t, err, oracleErrors.InternalError)
}

// TestParseRejectsDuplicateFieldID verifies an object whose field-ID array
// repeats a field ID is rejected.
func TestParseRejectsDuplicateFieldID(t *testing.T) {
	// Tree layout: [object opcode][count=2][fid=1][fid=1][child offset=6][child offset=7][scalar 0x31][scalar 0x30]
	// Both members use field ID 1, so the second would overwrite the first.
	buf := newOsonBuffer(drvCommon.B1Array{
		osonOpObjectType, 2, 1, 1, 0x00, 0x06, 0x00, 0x07,
		osonOpTrue,
		osonOpNull,
	})
	header := &osonHeader{
		treeSegmentByteLength: 10,
		fieldDictionary:       dictionary{fieldNames: []string{"a"}},
	}
	_, err := newNodeAt(buf, header, 0)
	if err == nil {
		t.Fatal("newNodeAt() error = nil, want duplicate-field-ID error")
	}
	assertOracleErrorCode(t, err, oracleErrors.InternalError)
}

// TestParseRejectsInvalidChildOffset verifies child offsets pointing outside
// the tree segment fail cleanly.
func TestParseRejectsInvalidChildOffset(t *testing.T) {
	doc := sampleSimpleObject.cloneOSON()
	root, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	obj := root.(*objectNode)
	childOffset := obj.childrenOffsets["name"]
	doc[childOffset] = byte(osonOpUpdateForwardUB2)
	binary.BigEndian.PutUint16(doc[childOffset+1:], math.MaxUint16)
	_, err = obj.Get("name")
	assertOracleErrorCode(t, err, oracleErrors.InternalError)
	if _, err := obj.Value(drvCommon.JSONConversionOptions{}); err == nil {
		t.Fatal("Value() error = nil, want failure when an object child redirect is invalid")
	}
}
