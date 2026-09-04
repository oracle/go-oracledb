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

// arrayNode implements drvCommon.JSONArrayNode.
type arrayNode struct {
	nodeBase
	// childOffsets contains the absolute document offset of each array element opcode.
	childOffsets []int
}

// newArrayNodeAt parses OSON array metadata at an absolute document offset.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - arrayNodeOffset: absolute document offset of the array opcode.
//
// Errors:
//   - invalid array opcode.
//   - malformed child-count or child-offset layout.
func newArrayNodeAt(buf *osonBuffer, header *osonHeader, arrayNodeOffset int) (*arrayNode, error) {
	opcode, err := buf.readUB1At(arrayNodeOffset)
	if err != nil {
		common.Odl.Error("newArrayNodeAt: failed", "error", err, "offset", arrayNodeOffset)
		return nil, err
	}
	if !isArrayOpcode(opcode) {
		cause := fmt.Errorf("opcode 0x%02x is not an array", opcode)
		common.Odl.Error("newArrayNodeAt: failed", "error", cause, "offset", arrayNodeOffset, "opcode", opcode)
		return nil, common.NewOracleError(oracleErrors.OsonBufferError, nil)
	}
	if opcode&(osonOpChildNoSortBit|osonOpObjectSharedFieldIDsBit|osonOpObjectUpdateOverflowBit) != 0 {
		cause := fmt.Errorf("array opcode 0x%02x contains object-only flags", opcode)
		common.Odl.Error("newArrayNodeAt: failed", "error", cause, "offset", arrayNodeOffset, "opcode", opcode)
		return nil, common.NewOracleError(oracleErrors.OsonParsingError, cause)
	}
	if opcode&osonOpChildSizeBits == osonOpChildDelegateForm {
		cause := fmt.Errorf("array opcode 0x%02x uses the delegate child-header form", opcode)
		common.Odl.Error("newArrayNodeAt: failed", "error", cause, "offset", arrayNodeOffset, "opcode", opcode)
		return nil, common.NewOracleError(oracleErrors.OsonParsingError, cause)
	}

	elementCount, childOffsetArrayStart, err := readContainerCountAt(buf, arrayNodeOffset+1, opcode)
	if err != nil {
		common.Odl.Error("newArrayNodeAt: failed", "error", err, "offset", arrayNodeOffset, "opcode", opcode)
		return nil, err
	}

	childOffsets, err := readChildOffsetsAt(buf, header, arrayNodeOffset, childOffsetArrayStart, elementCount, opcode)
	if err != nil {
		common.Odl.Error("newArrayNodeAt: failed", "error", err, "offset", arrayNodeOffset, "count", elementCount)
		return nil, err
	}

	return &arrayNode{
		nodeBase: nodeBase{
			buf:    buf,
			header: header,
			offset: arrayNodeOffset,
		},
		childOffsets: childOffsets,
	}, nil
}

// Kind implements drvCommon.JSONNode.Kind.
//
// Input:
//   - none.
//
// Output:
//   - drvCommon.KindArray.
//
// Errors:
//   - none.
func (array *arrayNode) Kind() drvCommon.Kind {
	return drvCommon.KindArray
}

// GetValue implements drvCommon.JSONNode.GetValue.
//
// Input:
//   - opts: JSON materialization options.
//
// Output:
//   - fully materialized array.
//
// Errors:
//   - child node construction or value decoding failure.
func (array *arrayNode) GetValue(opts drvCommon.JSONOption) (any, error) {
	return array.Value(opts)
}

// StringWithOption implements drvCommon.JSONNode.StringWithOption.
//
// Input:
//   - opts: JSON materialization options.
//
// Output:
//   - JSON text for the array.
//
// Errors:
//   - child node construction, value decoding, or JSON encoding failure.
func (array *arrayNode) StringWithOption(opts drvCommon.JSONOption) (string, error) {
	materializedArray, err := array.Value(opts)
	if err != nil {
		return "", err
	}

	jsonBytes, err := json.Marshal(jsonCompatibleValue(materializedArray))
	if err != nil {
		common.Odl.Error("arrayNode.StringWithOption: failed", "error", err, "offset", array.offset)
		return "", common.NewOracleError(oracleErrors.OsonBufferError, nil)
	}
	return string(jsonBytes), nil
}

// Get implements drvCommon.JSONArrayNode.Get.
//
// Input:
//   - index: zero-based array element index.
//
// Output:
//   - child node and true when index resolves successfully.
//
// Errors:
//   - encoded as false for an invalid index or malformed child node.
func (array *arrayNode) Get(index int) (drvCommon.JSONNode, bool) {
	if index < 0 || index >= len(array.childOffsets) {
		return nil, false
	}

	childNode, err := newNodeAt(array.buf, array.header, array.childOffsets[index])
	if err != nil {
		common.Odl.Error("arrayNode.Get: failed", "error", err, "offset", array.offset, "index", index, "childOffset", array.childOffsets[index])
		return nil, false
	}
	return childNode, true
}

// Len implements drvCommon.JSONArrayNode.Len.
//
// Input:
//   - none.
//
// Output:
//   - Length of the array.
//
// Errors:
//   - none.
func (array *arrayNode) Len() int {
	return len(array.childOffsets)
}

// Value implements drvCommon.JSONArrayNode.Value.
//
// Input:
//   - opts: JSON materialization options.
//
// Output:
//   - fully materialized array.
//
// Errors:
//   - child node construction or value decoding failure.
func (array *arrayNode) Value(opts drvCommon.JSONOption) ([]any, error) {
	elementValues := make([]any, len(array.childOffsets))
	for elementIndex := range array.childOffsets {
		// Get intentionally returns only a boolean for the public lazy-node API;
		// materialization must retain the parsing error for its caller.
		childNode, err := newNodeAt(array.buf, array.header, array.childOffsets[elementIndex])
		if err != nil {
			common.Odl.Error("arrayNode.Value: failed", "error", err, "offset", array.offset, "index", elementIndex)
			return nil, err
		}

		elementValue, err := childNode.GetValue(opts)
		if err != nil {
			common.Odl.Error("arrayNode.Value: failed", "error", err, "offset", array.offset, "index", elementIndex)
			return nil, err
		}

		elementValues[elementIndex] = elementValue
	}

	return elementValues, nil
}
