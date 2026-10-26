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

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// objectNode implements JSONObjectNode for an OSON object.
//
// The concrete type is scoped to package oson. Callers obtain an object node
// through Parse, which exposes it as JSONNode and keeps the OSON wire
// representation inside this package. Because JSONObjectNode embeds JSONNode,
// *objectNode satisfies both interfaces. After checking Kind, a caller can type
// assert the returned JSONNode to JSONObjectNode for keyed access.
//
// Creating an objectNode reads only the object metadata: its opcode, member
// count, field IDs, and child-offset table. The field IDs are resolved through
// the OSON dictionary, and childrenOffsets records each field name with the
// offset of its value opcode. Child payloads remain encoded until Get creates a
// node for one member or Value materializes the complete object.
type objectNode struct {
	nodeBase
	// childrenOffsets maps each decoded member name to its absolute value-opcode offset.
	childrenOffsets map[string]int
}

// newObjectNodeAt parses one object node at offset and returns *objectNode.
//
// Parameters:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - offset: absolute document offset of the object node.
//   - opcode: object opcode already read and classified by newNodeAt.
//
// Returns:
//   - *objectNode: OSON object node.
//
// Errors:
//   - malformed object layout, field IDs, or child offsets.
func newObjectNodeAt(buf *osonBuffer, header *osonHeader, offset int, opcode drvCommon.UB1) (*objectNode, error) {
	memberCount, fieldIDArrayStart, childOffsetArrayStart, err := readObjectLayout(buf, header, offset, opcode)
	if err != nil {
		common.Odl.Debug("newObjectNodeAt: failed", "error", err, "offset", offset, "opcode", opcode)
		return nil, err
	}

	fieldIDValues, err := readFieldIDEntriesAt(buf, header, fieldIDArrayStart, memberCount)
	if err != nil {
		common.Odl.Debug("newObjectNodeAt: failed", "error", err, "offset", offset, "members", memberCount, "fieldIDTable", fieldIDArrayStart)
		return nil, err
	}
	memberOffsets, err := readChildOffsetsAt(buf, header, offset, childOffsetArrayStart, memberCount, opcode)
	if err != nil {
		common.Odl.Debug("newObjectNodeAt: failed", "error", err, "offset", offset, "members", memberCount, "childOffsetTable", childOffsetArrayStart)
		return nil, err
	}

	members := make(map[string]int, memberCount)
	for i := 0; i < memberCount; i++ {
		// Field ids are 1-based on the wire and the merged dictionary is zero-based.
		fieldName, ok := header.fieldName(fieldIDValues[i] - 1)
		if !ok {
			details := fmt.Sprintf("field ID %d not found", fieldIDValues[i])
			common.Odl.Debug("newObjectNodeAt: failed", "reason", details, "offset", offset, "member", i, "fieldID", fieldIDValues[i])
			return nil, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		if _, dup := members[fieldName]; dup {
			details := fmt.Sprintf("field ID %d duplicates an earlier member", fieldIDValues[i])
			common.Odl.Debug("newObjectNodeAt: failed", "reason", details, "offset", offset, "member", i, "fieldID", fieldIDValues[i])
			return nil, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		members[fieldName] = memberOffsets[i]
	}

	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("newObjectNodeAt: parsed",
			"offset", offset,
			"opcode", opcode,
			"members", memberCount,
			"fieldIDWidth", header.numFieldIDBytes(),
			"childOffsetWidth", childOffsetSize(opcode),
			"fieldIDTable", fieldIDArrayStart,
			"childOffsetTable", childOffsetArrayStart)
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
// Parameters:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - start: absolute offset of the field-id array.
//   - count: number of field IDs to read.
//
// Returns:
//   - decoded field IDs in member order.
//
// Errors:
//   - buffer-read failure.
func readFieldIDEntriesAt(buf *osonBuffer, header *osonHeader, start, count int) ([]int, error) {
	size := header.numFieldIDBytes()
	if err := ensureNodeTableRange(buf, start, count, size, "readFieldIDEntriesAt"); err != nil {
		return nil, err
	}
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
		default: // numFieldIDBytes returns only UB1, UB2, or UB4.
			val, err := buf.readUB4At(entryOffset)
			if err != nil {
				return nil, err
			}
			entries[i] = int(val)
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
// Parameters:
//   - `buf`: OSON document reader
//   - `header`: parsed OSON header metadata
//   - `offset`: absolute offset of the object opcode
//   - `opcode`: object opcode already read at `offset`
//
// Returns:
//   - `count`: number of logical object members
//   - `fidArrayStart`: absolute offset of the field-id array to read
//   - `childArrayStart`: absolute offset of this object's child-offset array
//
// Errors:
//   - malformed direct or delegated object layout
//   - bad delegate reference
//   - buffer-read failure
func readObjectLayout(buf *osonBuffer, header *osonHeader, offset int, opcode drvCommon.UB1) (count, fidArrayStart, childArrayStart int, err error) {
	if opcode&osonOpChildSizeBits != osonOpChildDelegateForm {
		// Direct object layout:
		//   [opcode][count][field ids][child offsets][children...]
		count, nextOffset, err := readContainerCountAt(buf, offset+osonUB1Size, opcode)
		if err != nil {
			return 0, 0, 0, err
		}
		fidArrayStart = nextOffset
		childArrayStart = fidArrayStart + (count * header.numFieldIDBytes())
		return count, fidArrayStart, childArrayStart, nil
	}

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
	// A valid delegate must own a shared field-ID array, which requires all
	// three of: object family opcode, a direct count+FID layout (not another
	// 0x18 reference), and the referred bit (0x02) that advertises the FID
	// array as shareable. Anything else is a malformed reference.
	if !isObjectOpcode(delegateOpcode) ||
		delegateOpcode&osonOpChildSizeBits == osonOpChildDelegateForm ||
		delegateOpcode&osonOpObjectSharedFieldIDsBit == 0 {
		details := fmt.Sprintf("delegate object %d is not a field-ID owner", delegateOffset)
		common.Odl.Debug("readObjectLayout: failed", "reason", details, "offset", offset, "delegateOffset", delegateOffset, "delegateOpcode", delegateOpcode)
		return 0, 0, 0, common.NewOracleError(oracleErrors.InternalError, nil)
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
}

// Kind implements the JSONNode interface.
//
// Parameters:
//   - none.
//
// Returns:
//   - drvCommon.KindObject.
//
// Errors:
//   - none.
func (obj *objectNode) Kind() drvCommon.Kind {
	return drvCommon.KindObject
}

// GetValue implements the JSONNode interface.
//
// Parameters:
//   - opts: JSON materialization options.
//
// Returns:
//   - the fully materialized object.
//
// Errors:
//   - child node construction or value decoding failure.
func (obj *objectNode) GetValue(opts drvCommon.JSONConversionOptions) (any, error) {
	return obj.Value(opts)
}

// String implements the JSONNode interface.
//
// Returns:
//   - JSON text for the object.
//
// Errors:
//   - child node construction, value decoding, or JSON encoding failure.
func (obj *objectNode) String() (string, error) {
	text, err := json.Marshal(obj)
	if err != nil {
		common.Odl.Debug("objectNode.String: failed", "error", err, "offset", obj.offset)
		return "", common.NewOracleError(oracleErrors.JSONRenderingError, err)
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("objectNode.String: completed", "offset", obj.offset, "textBytes", len(text))
	}
	return string(text), nil
}

// MarshalJSON implements encoding/json.Marshaler.
//
// It marshals each child node directly so objects retain their existing shape
// while scalar nodes own the scalar-specific marshaling policy.
func (obj *objectNode) MarshalJSON() ([]byte, error) {
	values := make(map[string]json.RawMessage, len(obj.childrenOffsets))
	for name, offset := range obj.childrenOffsets {
		child, err := newNodeAt(obj.buf, obj.header, offset)
		if err != nil {
			return nil, err
		}

		values[name], err = child.MarshalJSON()
		if err != nil {
			return nil, err
		}
	}

	text, err := json.Marshal(values)
	if err != nil {
		common.Odl.Debug("objNode.MarshalJSON: failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.JSONRenderingError, err)
	}
	return text, nil
}

// Get implements the JSONObjectNode interface.
//
// Parameters:
//   - key: object field name.
//
// Returns:
//   - child node or nil.
//   - error for a missing key or child node construction failure.
//
// Errors:
//   - missing key or child node construction failure.
func (obj *objectNode) Get(key string) (drvCommon.JSONNode, error) {
	childOffset, ok := obj.childrenOffsets[key]
	if !ok {
		common.Odl.Debug("objectNode.Get: failed", "reason", "key not found", "offset", obj.offset)
		return nil, common.NewOracleError(oracleErrors.JSONObjectKeyNotFoundError, nil)
	}
	child, err := newNodeAt(obj.buf, obj.header, childOffset)
	if err != nil {
		common.Odl.Debug("objectNode.Get: failed", "error", err, "offset", obj.offset, "childOffset", childOffset)
		return nil, err
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("objectNode.Get: completed",
			"offset", obj.offset,
			"childOffset", childOffset,
			"childKind", child.Kind())
	}
	return child, nil
}

// Len implements the JSONObjectNode interface.
//
// Parameters:
//   - none.
//
// Returns:
//   - number of object members.
//
// Errors:
//   - none.
func (obj *objectNode) Len() int {
	return len(obj.childrenOffsets)
}

// Keys implements the JSONObjectNode interface.
//
// Parameters:
//   - none.
//
// Returns:
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

// Value implements the JSONObjectNode interface.
//
// Parameters:
//   - opts: JSON materialization options.
//
// Returns:
//   - fully materialized object as map[string]any.
//
// Errors:
//   - child node construction or value decoding failure.
func (obj *objectNode) Value(opts drvCommon.JSONConversionOptions) (map[string]any, error) {
	values := make(map[string]any, len(obj.childrenOffsets))
	for fieldName, offset := range obj.childrenOffsets {
		child, err := newNodeAt(obj.buf, obj.header, offset)
		if err != nil {
			common.Odl.Debug("objectNode.Value: failed", "error", err, "offset", obj.offset, "childOffset", offset)
			return nil, err
		}
		value, err := child.GetValue(opts)
		if err != nil {
			common.Odl.Debug("objectNode.Value: failed", "error", err, "offset", obj.offset, "childOffset", offset)
			return nil, err
		}
		values[fieldName] = value
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("objectNode.Value: completed", "offset", obj.offset, "members", len(values), "options", opts)
	}
	return values, nil
}
