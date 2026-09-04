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
	stdjson "encoding/json"
	"fmt"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Parse creates the lazy root OSON node for a document.
//
// Input:
//   - data: complete OSON document bytes.
//
// Output:
//   - root JSONNode for the document.
//
// Errors:
//   - malformed header, dictionary, or root node.
func Parse(data drvCommon.B1Array) (drvCommon.JSONNode, error) {
	buf := newOsonBuffer(data)
	header, err := newOsonHeader(buf)
	if err != nil {
		return nil, err
	}

	return newNodeAt(buf, header, header.treeSegmentStartOffset)
}

// classifyJSONValue determines the OSON node kind from a Go value's type.
//
// It returns KindScalar for these supported scalar types:
//
//   - nil, bool, and string
//   - int, int8, int16, int32, and int64
//   - uint, uint8, uint16, uint32, and uint64
//   - float32 and float64
//   - []byte, time.Time, JSONNumber, and json.Number
//
// It returns KindArray for []any and KindObject for map[string]any.
//
// It returns an error when the value's type is not supported by this driver.
func classifyJSONValue(value any) (drvCommon.Kind, error) {
	switch value.(type) {
	case nil,
		bool,
		string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		[]byte,
		time.Time,
		drvCommon.JSONNumber,
		stdjson.Number:
		return drvCommon.KindScalar, nil
	case []any:
		return drvCommon.KindArray, nil
	case map[string]any:
		return drvCommon.KindObject, nil
	default:
		return 0, fmt.Errorf("unsupported OSON value type %T", value)
	}
}

// nodeBase provides the shared encoded-document context for OSON nodes.
type nodeBase struct {
	// buffer for the current OSON document.
	buf *osonBuffer
	// OSON header for the current document.
	header *osonHeader
	// offset is the absolute document offset of this node's effective opcode.
	offset int
}

// IsOson reports whether data starts with the OSON magic/version prefix.
//
// Input:
//   - data: candidate OSON document bytes.
//
// Output:
//   - true when the prefix matches an OSON document.
//
// Errors:
//   - none; short input returns false.
func IsOson(data drvCommon.B1Array) bool {
	if len(data) < osonMinSize {
		return false
	}
	return binary.BigEndian.Uint32(data[:osonUB4Size])&osonMagicPrefixMask == osonMagicPrefix
}

// jsonCompatibleValue converts raw binary values to the hexadecimal string
// representation used by OSON JSON rendering. Container values are traversed
// so binary children do not fall back to encoding/json's base64 representation.
func jsonCompatibleValue(value any) any {
	switch value := value.(type) {
	case []byte:
		return fmt.Sprintf("%X", value)
	case []any:
		converted := make([]any, len(value))
		for i, item := range value {
			converted[i] = jsonCompatibleValue(item)
		}
		return converted
	case map[string]any:
		converted := make(map[string]any, len(value))
		for key, item := range value {
			converted[key] = jsonCompatibleValue(item)
		}
		return converted
	default:
		return value
	}
}

// newNodeAt resolves one node at offset and returns the matching OSON node.
//
// It reads the node opcode, resolves forwarding records, then dispatches to
// the object, array, or scalar constructor.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - offset: absolute document offset of the node.
//
// Output:
//   - JSONNode for the resolved node.
//
// Errors:
//   - nil buffer/header.
//   - buffer-read failure.
//   - malformed redirect or unsupported node layout.
func newNodeAt(buf *osonBuffer, header *osonHeader, offset int) (drvCommon.JSONNode, error) {
	if buf == nil || header == nil {
		cause := fmt.Errorf("node construction requires both buffer and header")
		common.Odl.Error("newNodeAt: failed", "error", cause, "offset", offset)
		return nil, common.NewOracleError(oracleErrors.OsonParsingError, cause)
	}

	resolvedOffset := offset
	seen := make(map[int]struct{})
	var opcode drvCommon.UB1
	for {
		// Update records can forward through more than one tree entry. Resolve
		// the chain here so each node constructor sees the final node opcode.
		if !header.containsNodeOffset(resolvedOffset) {
			cause := fmt.Errorf("node offset %d is outside the tree segments", resolvedOffset)
			common.Odl.Error("newNodeAt: failed", "error", cause, "offset", resolvedOffset)
			return nil, common.NewOracleError(oracleErrors.OsonParsingError, cause)
		}
		if _, exists := seen[resolvedOffset]; exists {
			cause := fmt.Errorf("cyclic OSON forwarding at node offset %d", resolvedOffset)
			common.Odl.Error("newNodeAt: failed", "error", cause, "offset", resolvedOffset)
			return nil, common.NewOracleError(oracleErrors.OsonParsingError, cause)
		}
		seen[resolvedOffset] = struct{}{}

		var err error
		opcode, err = buf.readUB1At(resolvedOffset)
		if err != nil {
			common.Odl.Error("newNodeAt: failed", "error", err, "offset", resolvedOffset)
			return nil, err
		}
		nextOffset, redirected, err := redirectedNodeOffset(buf, header, resolvedOffset, opcode)
		if err != nil {
			common.Odl.Error("newNodeAt: failed", "error", err, "offset", resolvedOffset, "opcode", opcode)
			return nil, err
		}
		if !redirected {
			break
		}
		resolvedOffset = nextOffset
	}

	switch {
	case isObjectOpcode(opcode):
		return newObjectNodeAt(buf, header, resolvedOffset)
	case isArrayOpcode(opcode):
		return newArrayNodeAt(buf, header, resolvedOffset)
	default:
		return newScalarNodeAt(buf, header, resolvedOffset)
	}
}

// redirectedNodeOffset resolves one redirect opcode to the target node offset.
func redirectedNodeOffset(buf *osonBuffer, header *osonHeader, offset int, opcode drvCommon.UB1) (int, bool, error) {
	switch {

	case isUpdatedOverflowObjectOpcode(opcode):
		nextOffset, err := header.resolveOverflowOffset(offset)
		return nextOffset, true, err
	case opcode == osonOpUpdateOverflow:
		nextOffset, err := header.resolveOverflowOffset(offset)
		return nextOffset, true, err
	case opcode == osonOpUpdateForwardUB2:
		relativeOffset, err := buf.readUB2At(offset + osonUB1Size)
		if err != nil {
			return 0, false, err
		}
		nextOffset, err := header.resolveForwardedOffset(int(relativeOffset))
		return nextOffset, true, err
	case opcode == osonOpUpdateForwardUB4:
		relativeOffset, err := buf.readUB4At(offset + osonUB1Size)
		if err != nil {
			return 0, false, err
		}

		nextOffset, err := header.resolveForwardedOffset(int(relativeOffset))
		return nextOffset, true, err
	case opcode == osonOpUpdateOversizeReserved:
		cause := fmt.Errorf("reserved update opcode 0x%02x is not supported", opcode)
		common.Odl.Error("redirectedNodeOffset: failed", "error", cause, "offset", offset, "opcode", opcode)
		return 0, false, common.NewOracleError(oracleErrors.OsonParsingError, cause)
	default:
		return 0, false, nil
	}
}

// isUpdatedOverflowObjectOpcode reports whether an object opcode is the special
// "shared field ids + overflow body" form.
//
// Bit shape:
//
//	[10xxxxxx] object family
//	[... ...1] overflow flag set
//	[... ..1.] referred/shared-FID flag set
func isUpdatedOverflowObjectOpcode(opcode drvCommon.UB1) bool {
	return isObjectOpcode(opcode) &&
		(opcode&osonOpUpdatedObjectReferencePattern) == osonOpUpdatedObjectReferencePattern
}

// childOffsetSize returns the encoded width of one child-offset entry.
func childOffsetSize(opcode drvCommon.UB1) int {
	if opcode&osonOpChildOffsetUB4Bit != 0 {
		return osonUB4Size
	}
	return osonUB2Size
}

// readContainerCountAt decodes the direct child-count prefix used by ordinary
// array/object containers.
func readContainerCountAt(buf *osonBuffer, offset int, opcode drvCommon.UB1) (count, nextOffset int, err error) {
	switch opcode & osonOpChildSizeBits {
	case osonOpChildCountUB1:
		val, readErr := buf.readUB1At(offset)
		if readErr != nil {
			return 0, 0, readErr
		}
		return int(val), offset + osonUB1Size, nil
	case osonOpChildCountUB2:
		val, readErr := buf.readUB2At(offset)
		if readErr != nil {
			return 0, 0, readErr
		}
		return int(val), offset + osonUB2Size, nil
	case osonOpChildCountUB4:
		val, readErr := buf.readUB4At(offset)
		if readErr != nil {
			return 0, 0, readErr
		}

		return int(val), offset + osonUB4Size, nil
	default:
		cause := fmt.Errorf("opcode 0x%02x does not encode a direct child count", opcode)
		common.Odl.Error("readContainerCountAt: failed", "error", cause, "offset", offset-1, "opcode", opcode)
		return 0, 0, common.NewOracleError(oracleErrors.OsonParsingError, cause)
	}
}

// readChildOffsetsAt decodes the child-offset table into absolute document offsets.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - containerOffset: absolute offset of the containing node.
//   - start: absolute offset of the child-offset table.
//   - count: number of child offsets to decode.
//   - opcode: containing array or object opcode.
//
// Output:
//   - absolute child-node offsets in wire order.
//
// Errors:
//   - child-offset decode failure.
func readChildOffsetsAt(buf *osonBuffer, header *osonHeader, containerOffset, start, count int, opcode drvCommon.UB1) ([]int, error) {
	size := childOffsetSize(opcode)
	if err := ensureNodeTableRange(buf, start, count, size, "readChildOffsetsAt"); err != nil {
		return nil, err
	}
	offsets := make([]int, count)
	for i := 0; i < count; i++ {
		entryOffset := start + (i * size)
		childOffset, err := readChildOffsetAt(buf, header, containerOffset, entryOffset, size)
		if err != nil {
			return nil, err
		}
		offsets[i] = childOffset
	}
	return offsets, nil
}

// ensureNodeTableRange validates an untrusted node table before allocation.
func ensureNodeTableRange(buf *osonBuffer, start, count, width int, stage string) error {
	if buf == nil || start < 0 || count < 0 || width <= 0 || start > buf.size() || count > (buf.size()-start)/width {
		cause := fmt.Errorf("node table start %d count %d width %d exceeds document size", start, count, width)
		common.Odl.Error(stage+": failed", "error", cause, "start", start, "count", count, "width", width)
		return common.NewOracleError(oracleErrors.OsonBufferError, cause)
	}
	return nil
}

// readChildOffsetAt decodes one child-offset entry into an absolute document offset.
//
// Input:
//   - buf: OSON document reader.
//   - header: parsed OSON header metadata.
//   - containerOffset: absolute offset of the containing node.
//   - entryOffset: absolute offset of the child-offset entry.
//   - width: encoded width of the child-offset entry.
//
// Output:
//   - absolute document offset of the referenced child node.
//
// Errors:
//   - unsupported entry width.
//   - buffer-read failure.
func readChildOffsetAt(buf *osonBuffer, header *osonHeader, containerOffset, entryOffset, width int) (int, error) {
	treeStart := header.segmentOffsetForNode(containerOffset)
	containerTreeOffset := containerOffset - treeStart

	if header.relativeOffsets() {
		delta, err := readRelativeChildOffset(buf, entryOffset, width)
		if err != nil {
			return 0, err
		}
		return treeStart + containerTreeOffset + delta, nil
	}

	switch width {
	case osonUB2Size:
		val, err := buf.readUB2At(entryOffset)
		if err != nil {
			return 0, err
		}
		// The stored value is relative to the beginning of the primary tree.
		// That beginning is the common address-space origin for both the primary
		// and extended tree segments, so do not add the current node's segment
		// offset here.
		return header.treeSegmentOffset() + int(val), nil
	case osonUB4Size:
		val, err := buf.readUB4At(entryOffset)
		if err != nil {
			return 0, err
		}

		return header.treeSegmentOffset() + int(val), nil
	}

	cause := fmt.Errorf("unsupported child offset width %d", width)
	common.Odl.Error("readChildOffsetAt: failed", "error", cause, "width", width, "entryOffset", entryOffset)
	return 0, common.NewOracleError(oracleErrors.OsonParsingError, cause)
}

// readRelativeChildOffset decodes one signed child-offset delta from a relative table.
//
// Input:
//   - buf: OSON document reader.
//   - entryOffset: absolute offset of the delta entry.
//   - width: encoded width of the delta entry.
//
// Output:
//   - signed tree-relative delta stored at entryOffset.
//
// Errors:
//   - unsupported delta width.
//   - buffer-read failure.
func readRelativeChildOffset(buf *osonBuffer, entryOffset, width int) (int, error) {
	switch width {
	case osonUB2Size:
		val, err := buf.readSB2At(entryOffset)
		if err != nil {
			return 0, err
		}
		return int(val), nil
	case osonUB4Size:
		val, err := buf.readSB4At(entryOffset)
		if err != nil {
			return 0, err
		}
		return int(val), nil
	default:
		cause := fmt.Errorf("unsupported child offset width %d", width)
		common.Odl.Error("readRelativeChildOffset: failed", "error", cause, "width", width, "entryOffset", entryOffset)
		return 0, common.NewOracleError(oracleErrors.OsonParsingError, cause)
	}
}
