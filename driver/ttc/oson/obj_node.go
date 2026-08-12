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
	"fmt"

	"github.com/oracle/go-driver/driver/common"
)

// objectNode implements common.JSONObjectNode.
type objectNode struct {
	nodeBase
	// childrenOffsets maps each decoded member name to its absolute value-opcode offset.
	childrenOffsets map[string]int
}

// newObjectNodeAt parses one object node at offset and returns *objectNode.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - offset: absolute document offset of the object node.
//
// Output:
//   - *objectNode: OSON object node.
//
// Errors:
//   - invalid object opcode.
//   - malformed object layout, field IDs, or child offsets.
func newObjectNodeAt(buf *osonBuffer, header *osonHeader, offset int) (*objectNode, error) {
	opcode, err := buf.readUB1At(offset)
	if err != nil {
		common.Odl.Error("newObjectNodeAt: failed", "error", err, "offset", offset)
		return nil, err
	}
	if !isObjectOpcode(opcode) {
		cause := fmt.Errorf("opcode 0x%02x is not an object", opcode)
		common.Odl.Error("newObjectNodeAt: failed", "error", cause, "offset", offset, "opcode", opcode)
		return nil, common.NewOracleError(common.OsonParsingError, cause)
	}

	memberCount, fieldIDArrayStart, childOffsetArrayStart, err := readObjectLayout(buf, header, offset, opcode)
	if err != nil {
		common.Odl.Error("newObjectNodeAt: failed", "error", err, "offset", offset, "opcode", opcode)
		return nil, err
	}

	fieldIDValues, err := readFieldIDEntriesAt(buf, header, fieldIDArrayStart, memberCount)
	if err != nil {
		common.Odl.Error("newObjectNodeAt: failed", "error", err, "offset", offset, "count", memberCount)
		return nil, err
	}
	memberOffsets, err := readChildOffsetsAt(buf, header, offset, childOffsetArrayStart, memberCount, opcode)
	if err != nil {
		common.Odl.Error("newObjectNodeAt: failed", "error", err, "offset", offset, "count", memberCount)
		return nil, err
	}

	members := make(map[string]int, memberCount)
	for i := 0; i < memberCount; i++ {
		// Field ids are 1-based on the wire and the merged dictionary is zero-based.
		fieldName, ok := header.fieldName(fieldIDValues[i] - 1)
		if !ok {
			cause := fmt.Errorf("field id %d not found in dictionary", fieldIDValues[i])
			common.Odl.Error("newObjectNodeAt: failed", "error", cause, "offset", offset, "index", i, "fieldID", fieldIDValues[i])
			return nil, common.NewOracleError(common.OsonParsingError, cause)
		}
		members[fieldName] = memberOffsets[i]
	}

	return &objectNode{
		nodeBase: nodeBase{
			buf:    buf,
			header: header,
			offset: offset,
		},
		childrenOffsets: members,
	}, nil
}

// readFieldIDEntriesAt expands one encoded field-id array into []int.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - start: absolute offset of the field-id array.
//   - count: number of field IDs to read.
//
// Output:
//   - decoded field IDs in member order.
//
// Errors:
//   - unsupported field-id width.
//   - buffer-read failure or UB4 conversion failure.
func readFieldIDEntriesAt(buf *osonBuffer, header *osonHeader, start, count int) ([]int, error) {
	size := header.numFieldIDBytes()
	entries := make([]int, count)
	for i := 0; i < count; i++ {
		entryOffset := start + (i * size)
		switch size {
		case osonUB1Size:
			val, err := buf.readUB1At(entryOffset)
			if err != nil {
				return nil, err
			}
			entries[i] = int(val)
		case osonUB2Size:
			val, err := buf.readUB2At(entryOffset)
			if err != nil {
				return nil, err
			}
			entries[i] = int(val)
		case osonUB4Size:
			val, err := buf.readUB4At(entryOffset)
			if err != nil {
				return nil, err
			}
			entries[i] = int(val)
		default:
			cause := fmt.Errorf("unsupported field id width %d", size)
			common.Odl.Error("readFieldIDEntriesAt: failed", "error", cause, "width", size)
			return nil, common.NewOracleError(common.OsonParsingError, cause)
		}
	}
	return entries, nil
}

// readObjectLayout decodes the object-local structural metadata.
//
// Two object forms exist:
//
// Direct form:
//
//	[opcode][count][field ids][child offsets][children...]
//
// Delegated/shared-FID form:
//
//	[opcode][delegate object ref][child offsets][children...]
//	                       |
//	                       +--> other object contains [count][field ids]
//
// More explicitly, the delegated layout splits ownership like this:
//
//	current object:
//	  [opcode][delegate ref][its own child offsets][its own children...]
//	delegate object:
//	  [opcode][count][shared field ids][delegate child offsets][delegate children...]
//
// So the current object reuses only:
//
//	[count][shared field ids]
//
// and still keeps its own:
//
//	[child offsets][children...]
//
// The goal of this helper is to normalize both forms into the same runtime
// description:
//   - member count
//   - absolute location of the field-id array
//   - absolute location of this object's child-offset array
//
// Input:
//   - `buf`: OSON document reader
//   - `header`: parsed OSON header metadata
//   - `offset`: absolute offset of the object opcode
//   - `opcode`: object opcode already read at `offset`
//
// Output:
//   - `count`: number of logical object members
//   - `fidArrayStart`: absolute offset of the field-id array to read
//   - `childArrayStart`: absolute offset of this object's child-offset array
//
// Errors:
//   - malformed direct or delegated object layout
//   - bad delegate reference
//   - buffer-read failure
func readObjectLayout(buf *osonBuffer, header *osonHeader, offset int, opcode common.UB1) (count, fidArrayStart, childArrayStart int, err error) {
	switch opcode & osonOpChildSizeBits {
	case osonOpChildCountUB1, osonOpChildCountUB2, osonOpChildCountUB4:
		// Direct object layout:
		//   [opcode][count][field ids][child offsets][children...]
		count, nextOffset, err := readContainerCountAt(buf, offset+osonUB1Size, opcode)
		if err != nil {
			return 0, 0, 0, err
		}
		fidArrayStart = nextOffset
		childArrayStart = fidArrayStart + (count * header.numFieldIDBytes())
		return count, fidArrayStart, childArrayStart, nil
	case osonOpChildDelegateForm:
		// Delegated/shared-FID object layout:
		//   current object:
		//     [opcode][delegate ref][its own child offsets][its own children...]
		//   delegate object:
		//     [opcode][count][shared field ids][delegate child offsets][delegate children...]
		//
		// So this branch resolves the delegate object first, reads `count` and the
		// shared field-id array from that delegate, and then sets
		// `childArrayStart` back into the current object immediately after the
		// delegate reference.
		delegateWidth := childOffsetSize(opcode)
		// Delegate references remain relative to the primary tree segment, even
		// when this referring object was reached through a V2 update redirect
		// into the extended tree segment. This matches the OSON Java decoder.
		primaryTreeStart := header.treeSegmentOffset()

		var delegateOffset int
		if delegateWidth == osonUB2Size {
			val, readErr := buf.readUB2At(offset + osonUB1Size)
			if readErr != nil {
				return 0, 0, 0, readErr
			}
			delegateOffset = primaryTreeStart + int(val)
		} else {
			val, readErr := buf.readUB4At(offset + osonUB1Size)
			if readErr != nil {
				return 0, 0, 0, readErr
			}

			delegateOffset = primaryTreeStart + int(val)
		}

		delegateOpcode, readErr := buf.readUB1At(delegateOffset)
		if readErr != nil {
			return 0, 0, 0, readErr
		}
		if !isObjectOpcode(delegateOpcode) || delegateOpcode&osonOpChildSizeBits == osonOpChildDelegateForm {
			cause := fmt.Errorf("delegate object at %d does not carry a direct field-id array", delegateOffset)
			common.Odl.Error("readObjectLayout: failed", "error", cause, "offset", offset, "delegateOffset", delegateOffset, "delegateOpcode", delegateOpcode)
			return 0, 0, 0, common.NewOracleError(common.OsonParsingError, cause)
		}

		count, nextOffset, readErr := readContainerCountAt(buf, delegateOffset+osonUB1Size, delegateOpcode)
		if readErr != nil {
			return 0, 0, 0, readErr
		}
		fidArrayStart = nextOffset
		// In the delegate form, this object's own child-offset array begins
		// immediately after the delegate reference.
		childArrayStart = offset + osonUB1Size + delegateWidth
		return count, fidArrayStart, childArrayStart, nil
	default:
		cause := fmt.Errorf("unsupported object count encoding 0x%02x", opcode&osonOpChildSizeBits)
		common.Odl.Error("readObjectLayout: failed", "error", cause, "offset", offset, "opcode", opcode)
		return 0, 0, 0, common.NewOracleError(common.OsonParsingError, cause)
	}
}

// Kind implements common.JSONNode.Kind.
//
// Input:
//   - none.
//
// Output:
//   - common.KindObject.
//
// Errors:
//   - none.
func (obj *objectNode) Kind() common.Kind {
	return common.KindObject
}

// GetValue implements common.JSONNode.GetValue.
//
// Input:
//   - opts: JSON materialization options.
//
// Output:
//   - the fully materialized object.
//
// Errors:
//   - child node construction or value decoding failure.
func (obj *objectNode) GetValue(opts common.JSONOption) (any, error) {
	return obj.Value(opts)
}

// StringWithOption materializes the object as JSON text.
//
// Input:
//   - opts: JSON materialization options.
//
// Output:
//   - JSON text for the object.
//
// Errors:
//   - child node construction, value decoding, or JSON encoding failure.
func (obj *objectNode) StringWithOption(opts common.JSONOption) (string, error) {
	value, err := obj.Value(opts)
	if err != nil {
		return "", err
	}

	text, err := json.Marshal(value)
	if err != nil {
		common.Odl.Error("objectNode.StringWithOption: failed", "error", err, "offset", obj.offset)
		return "", common.NewOracleError(common.OsonBufferError, nil)
	}
	return string(text), nil
}

// Get creates a node for key without decoding its value.
//
// Input:
//   - key: object field name.
//
// Output:
//   - child node or nil.
//   - key resolving success
//
// Errors:
//   - none.
func (obj *objectNode) Get(key string) (common.JSONNode, bool) {
	childOffset, ok := obj.childrenOffsets[key]
	if !ok {
		return nil, false
	}
	child, err := newNodeAt(obj.buf, obj.header, childOffset)
	if err != nil {
		common.Odl.Error("objectNode.Get: failed", "error", err, "offset", obj.offset, "key", key)
		return nil, false
	}
	return child, true
}

// Len returns the number of members addressable through keyed access.
//
// Input:
//   - none.
//
// Output:
//   - number of object members.
//
// Errors:
//   - none.
func (obj *objectNode) Len() int {
	return len(obj.childrenOffsets)
}

// Keys returns the object member names.
//
// Input:
//   - none.
//
// Output:
//   - member names in unspecified order.
//
// Errors:
//   - none.
func (obj *objectNode) Keys() []string {
	fieldNames := make([]string, 0, len(obj.childrenOffsets))
	for fieldName := range obj.childrenOffsets {
		fieldNames = append(fieldNames, fieldName)
	}
	return fieldNames
}

// Value materializes the object into a Go map.
//
// Input:
//   - opts: JSON materialization options.
//
// Output:
//   - fully materialized object as map[string]any.
//
// Errors:
//   - child node construction or value decoding failure.
func (obj *objectNode) Value(opts common.JSONOption) (map[string]any, error) {
	values := make(map[string]any, len(obj.childrenOffsets))
	for fieldName, offset := range obj.childrenOffsets {
		child, err := newNodeAt(obj.buf, obj.header, offset)
		if err != nil {
			common.Odl.Error("objectNode.Value: failed", "error", err, "offset", obj.offset, "key", fieldName)
			return nil, err
		}
		value, err := child.GetValue(opts)
		if err != nil {
			common.Odl.Error("objectNode.Value: failed", "error", err, "offset", obj.offset, "key", fieldName)
			return nil, err
		}
		values[fieldName] = value
	}
	return values, nil
}
