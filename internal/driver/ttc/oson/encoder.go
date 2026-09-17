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
** either these or other term.
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
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// Compact Oracle NUMBER payloads use the 0x20 family when the payload fits
	// in 8 bytes; the low nibble stores payload length minus one.
	_compactOracleNumberMaxPayloadLen = 8
)

// Encode converts a supported Go value to an OSON document.
//
// Input:
//   - nil, bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
//     float32, float64, []byte, time.Time, drvCommon.JSONNumber, json.Number, map[string]any, []any.
//
// Output:
// - drvCommon.B1Array containing the encoded OSON document.
//
// Errors:
// - common.OsonEncodingError for an unsupported value or an OSON encoding/size limit failure.
func Encode(value any) (drvCommon.B1Array, error) {
	inputType := fmt.Sprintf("%T", value)
	common.Odl.Debug("oson.Encode: begin", "inputType", inputType)

	enc := newOsonEncoder()
	doc, err := enc.encode(value)
	if err != nil {
		common.Odl.Debug("oson.Encode: failed", "error", err, "inputType", inputType)
		return nil, err
	}

	common.Odl.Debug("oson.Encode: completed",
		"inputType", inputType,
		"documentBytes", len(doc),
		"version", enc.version,
		"flags", enc.flags,
		"primaryFields", len(enc.dict.primary),
		"secondaryFields", len(enc.dict.secondary),
		"treeBytes", len(enc.treeSegmentBytes))
	return doc, nil
}

// osonEncoder keeps the state needed to build one OSON document.
type osonEncoder struct {
	// Field-name dictionary for this document.
	dict fieldNameDictionary

	// OSON format version for the document being built. Most documents use
	// version 1. Documents with field names longer than 255 bytes use version 3
	// because they require a secondary dictionary.
	version drvCommon.UB1
	// Header flags for the document.
	flags drvCommon.UB2

	// Size of the primary dictionary heap in bytes.
	primaryHeapSize int
	// Size of the secondary dictionary heap in bytes.
	secondaryHeapSize int
	// Final segement tree bytes.
	treeSegmentBytes drvCommon.B1Array
}

// fieldNameDictionary is the per-document object-key dictionary state.
type fieldNameDictionary struct {
	// entriesByName resolves an object key to its planned dictionary entry.
	entriesByName map[string]*fieldNameEntry

	// primary stores planned dictionary entries for keys up to 255 UTF-8 bytes.
	primary []*fieldNameEntry
	// secondary stores planned dictionary entries for keys over 255 UTF-8 bytes.
	secondary []*fieldNameEntry

	// fieldIDWidth is the encoded object field-ID width in bytes for this document.
	fieldIDWidth int
}

// fieldNameEntry is the on-wire dictionary row for one object key.
type fieldNameEntry struct {
	// name is the decoded Go string form of the object key.
	name string
	// raw is the UTF-8 byte form written into the dictionary heap.
	raw drvCommon.B1Array

	// hash is the compact dictionary hash written into the tier's hash array.
	hash uint32
	// heapOffset is the byte offset of this key within its packed dictionary heap.
	heapOffset int
	// fieldID is the finalized 1-based dictionary ID used by encoded object nodes.
	fieldID int
}

// newOsonEncoder returns a fresh encoder for one document.
//
// Encoder state must not be reused across calls because dictionaries are scoped
// to a single OSON document.
func newOsonEncoder() *osonEncoder {
	return &osonEncoder{
		version: osonFormatMinVersion,
		dict: fieldNameDictionary{
			entriesByName: make(map[string]*fieldNameEntry),
		},
	}
}

// encode converts a supported Go value into an OSON document.
func (enc *osonEncoder) encode(value any) (drvCommon.B1Array, error) {
	kind, err := classifyJSONValue(value)
	if err != nil {
		common.Odl.Debug("osonEncoder.encode: failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.OsonEncodingError, err)
	}
	common.Odl.Debug("osonEncoder.encode: classified", "kind", kind)
	switch kind {
	case drvCommon.KindScalar:
		return enc.encodeScalarDocument(value)
	}
	return enc.encodeContainer(value)
}

// encodeScalarDocument converts a supported scalar value into an OSON document.
func (enc *osonEncoder) encodeScalarDocument(value any) (drvCommon.B1Array, error) {
	var tree osonWriteBuffer
	err := enc.writeScalarNode(&tree, value)
	if err != nil {
		return nil, err
	}

	enc.treeSegmentBytes = tree.bytes()
	enc.prepareScalarHeader()

	doc := enc.emitScalarDocument()
	common.Odl.Debug("osonEncoder.encodeScalarDocument: completed",
		"opcode", enc.treeSegmentBytes[0],
		"treeBytes", len(enc.treeSegmentBytes),
		"version", enc.version,
		"flags", enc.flags,
		"documentBytes", len(doc))
	return doc, nil
}

// prepareScalarHeader sets the header for a scalar OSON document.
func (enc *osonEncoder) prepareScalarHeader() {
	enc.version = osonFormatMinVersion
	enc.flags = osonFlagInlineLeafMask | osonFlagStringLengthInOpcodeMask | osonFlagScalarDocumentMask
	if len(enc.treeSegmentBytes) > math.MaxUint16 {
		enc.flags |= osonFlagTreeSegmentSizeUB4Mask
	}
}

// emitScalarDocument writes a scalar OSON document with the tree segment after the header.
func (enc *osonEncoder) emitScalarDocument() drvCommon.B1Array {
	var out osonWriteBuffer
	out.writeUB4(drvCommon.UB4(osonMagicPrefix | int(enc.version)))
	out.writeUB2(enc.flags)
	if enc.flags&osonFlagTreeSegmentSizeUB4Mask != 0 {
		out.writeUB4(drvCommon.UB4(len(enc.treeSegmentBytes)))
	} else {
		out.writeUB2(drvCommon.UB2(len(enc.treeSegmentBytes)))
	}
	out.writeBytes(enc.treeSegmentBytes)
	return out.bytes()
}

// encodeContainer converts a supported object or array into an OSON document.
func (enc *osonEncoder) encodeContainer(value any) (drvCommon.B1Array, error) {
	if err := enc.prepareDictionary(value); err != nil {
		common.Odl.Debug("osonEncoder.encodeContainer: failed", "error", err, "stage", "dictionary")
		return nil, err
	}
	if err := enc.prepareTreeSegment(value); err != nil {
		common.Odl.Debug("osonEncoder.encodeContainer: failed", "error", err, "stage", "tree")
		return nil, err
	}

	enc.prepareContainerHeader()
	doc := enc.emitContainerDocument()
	common.Odl.Debug("osonEncoder.encodeContainer: completed",
		"primaryFields", len(enc.dict.primary),
		"secondaryFields", len(enc.dict.secondary),
		"fieldIDWidth", enc.dict.fieldIDWidth,
		"primaryHeapBytes", enc.primaryHeapSize,
		"secondaryHeapBytes", enc.secondaryHeapSize,
		"treeBytes", len(enc.treeSegmentBytes),
		"version", enc.version,
		"flags", enc.flags,
		"documentBytes", len(doc))
	return doc, nil
}

// prepareDictionary collects object keys and assigns their final dictionary ids.
func (enc *osonEncoder) prepareDictionary(value any) error {
	if err := enc.collectFieldNames(value); err != nil {
		return err
	}

	enc.processFieldNames()
	return nil
}

// prepareTreeSegment emits the container tree and stores the final segment.
func (enc *osonEncoder) prepareTreeSegment(value any) error {
	var tree osonWriteBuffer
	if err := enc.writeNode(&tree, value, osonUB4Size); err != nil {
		return err
	}

	enc.treeSegmentBytes = tree.bytes()
	return nil
}

// prepareContainerHeader derives container header fields from dictionary and
// tree-segment state. It must run after field names and tree bytes are ready.
func (enc *osonEncoder) prepareContainerHeader() {
	enc.version = osonFormatMinVersion
	enc.flags = osonFlagInlineLeafMask | osonFlagStringLengthInOpcodeMask
	enc.primaryHeapSize = enc.dictionaryHeapSize(enc.dict.primary, osonUB1Size)
	enc.secondaryHeapSize = enc.dictionaryHeapSize(enc.dict.secondary, osonUB2Size)

	if len(enc.dict.primary) > 0 {
		enc.flags |= osonFlagPrimaryHashIDsUseUB1Mask
	}
	switch enc.dict.fieldIDWidth {
	case osonUB4Size:
		enc.flags |= osonFlagDistinctFieldCountUB4Mask
	case osonUB2Size:
		enc.flags |= osonFlagDistinctFieldCountUB2Mask
	}
	if enc.primaryHeapSize > math.MaxUint16 {
		enc.flags |= osonFlagFieldHeapSizeUB4Mask
	}
	if len(enc.treeSegmentBytes) > math.MaxUint16 {
		enc.flags |= osonFlagTreeSegmentSizeUB4Mask
	}
	if len(enc.dict.secondary) > 0 {
		enc.version = 3
	}
}

// emitContainerDocument writes the non-scalar OSON header, dictionaries, and
// tree segment.
//
// Containers always use inline leaf values in this encoder. Child offsets are
// stored as tree-relative entries using the narrowest width selected while
// building the tree, so the document does not need the relative-offset header
// flag.
func (enc *osonEncoder) emitContainerDocument() drvCommon.B1Array {
	var out osonWriteBuffer
	out.writeUB4(drvCommon.UB4(osonMagicPrefix | int(enc.version)))
	out.writeUB2(enc.flags)
	enc.writePrimaryDictionaryHeader(&out, enc.primaryHeapSize)
	if len(enc.dict.secondary) > 0 {
		enc.writeSecondaryDictionaryHeader(&out, enc.secondaryHeapSize)
	}
	enc.writeTreeSegmentSize(&out, enc.flags, len(enc.treeSegmentBytes))
	// Tiny-node statistics are metadata. The tree is self-describing
	// without this value, so the encoder writes zero until it collects these
	// statistics as part of tree construction.
	out.writeUB2(0)

	enc.writeDictionary(&out, enc.dict.primary, osonUB1Size)
	if len(enc.dict.secondary) > 0 {
		enc.writeDictionary(&out, enc.dict.secondary, osonUB2Size)
	}
	out.writeBytes(enc.treeSegmentBytes)
	return out.bytes()
}

// collectFieldNames gathers object keys for the OSON field-name dictionary.
func (enc *osonEncoder) collectFieldNames(value any) error {
	kind, err := classifyJSONValue(value)
	if err != nil {
		common.Odl.Debug("osonEncoder.collectFieldNames: failed", "error", err)
		return common.NewOracleError(oracleErrors.OsonEncodingError, err)
	}
	if kind == drvCommon.KindScalar {
		return nil
	}
	if kind == drvCommon.KindArray {
		for _, child := range value.([]any) {
			if err := enc.collectFieldNames(child); err != nil {
				return err
			}
		}
		return nil
	}
	for key, child := range value.(map[string]any) {
		if err := enc.addFieldName(key); err != nil {
			return err
		}
		if err := enc.collectFieldNames(child); err != nil {
			return err
		}
	}
	return nil
}

// addFieldName adds one object key to the correct OSON dictionary tier.
// Key length is measured in UTF-8 bytes, not runes.
func (enc *osonEncoder) addFieldName(name string) error {
	if _, ok := enc.dict.entriesByName[name]; ok {
		return nil
	}

	hash, byteLen := osonHash(name)
	if byteLen > osonMaxSecondaryDictKeyLength {
		cause := fmt.Errorf("field name length %d exceeds OSON limit %d", byteLen, osonMaxSecondaryDictKeyLength)
		common.Odl.Debug("osonEncoder.addFieldName: failed", "error", cause, "length", byteLen)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}

	field := &fieldNameEntry{
		name: name,
		raw:  drvCommon.B1Array([]byte(name)),
	}

	if byteLen <= osonMaxPrimaryDictKeyLength {
		field.hash = compactPrimaryHash(hash, osonPrimaryDictHashIDSizeUB1)
		enc.dict.primary = append(enc.dict.primary, field)
	} else {
		field.hash = compactSecondaryHash(hash)
		enc.dict.secondary = append(enc.dict.secondary, field)
	}

	enc.dict.entriesByName[name] = field
	return nil
}

// processFieldNames sorts the dictionary and assigns field IDs, heap offsets, and field-ID width.
func (enc *osonEncoder) processFieldNames() {
	sortFieldNames(enc.dict.primary)
	sortFieldNames(enc.dict.secondary)

	offset := 0
	for i, field := range enc.dict.primary {
		field.heapOffset = offset
		field.fieldID = i + 1
		offset += osonUB1Size + len(field.raw)
	}

	offset = 0
	for i, field := range enc.dict.secondary {
		field.heapOffset = offset
		field.fieldID = len(enc.dict.primary) + i + 1
		offset += osonUB2Size + len(field.raw)
	}

	switch total := len(enc.dict.primary) + len(enc.dict.secondary); {
	case total > math.MaxUint16:
		enc.dict.fieldIDWidth = osonUB4Size
	case total > math.MaxUint8:
		enc.dict.fieldIDWidth = osonUB2Size
	default:
		enc.dict.fieldIDWidth = osonUB1Size
	}
	common.Odl.Debug("osonEncoder.processFieldNames: completed",
		"primaryFields", len(enc.dict.primary),
		"secondaryFields", len(enc.dict.secondary),
		"fieldIDWidth", enc.dict.fieldIDWidth)
}

// sortFieldNames sorts one dictionary tier by hash, length, then bytes.
func sortFieldNames(fields []*fieldNameEntry) {
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].hash != fields[j].hash {
			return fields[i].hash < fields[j].hash
		}
		if len(fields[i].raw) != len(fields[j].raw) {
			return len(fields[i].raw) < len(fields[j].raw)
		}
		return string(fields[i].raw) < string(fields[j].raw)
	})
}

// writeNode writes one OSON tree node for a supported Go value.
func (enc *osonEncoder) writeNode(tree *osonWriteBuffer, value any, childOffsetSize int) error {
	kind, err := classifyJSONValue(value)
	if err != nil {
		common.Odl.Debug("osonEncoder.writeNode: failed", "error", err)
		return common.NewOracleError(oracleErrors.OsonEncodingError, err)
	}
	switch kind {
	case drvCommon.KindScalar:
		return enc.writeScalarNode(tree, value)
	case drvCommon.KindArray:
		return enc.writeArrayNode(tree, value.([]any), childOffsetSize)
	}
	return enc.writeObjectNode(tree, value.(map[string]any), childOffsetSize)
}

// bufferPatchError wraps an internal offset-table patching failure as an OSON
// encoding error.
func (enc *osonEncoder) bufferPatchError(operation string, err error) error {
	common.Odl.Debug("osonEncoder."+operation+": failed", "error", err)
	return common.NewOracleError(oracleErrors.OsonEncodingError, err)
}

// writeArrayNode writes one array node and its elements.
func (enc *osonEncoder) writeArrayNode(tree *osonWriteBuffer, value []any, elementOffsetSize int) error {
	nodeOffset := tree.position()
	count := len(value)
	common.Odl.Debug("osonEncoder.writeArrayNode: begin",
		"treeOffset", nodeOffset,
		"elements", count,
		"childOffsetWidth", elementOffsetSize)
	tree.writeUB1(containerOpcode(osonOpArrayType, count, elementOffsetSize))
	tree.writeContainerCount(count)

	offsetStart := tree.reserve(count * elementOffsetSize)
	for i, child := range value {
		if err := tree.patchUint(offsetStart+i*elementOffsetSize, elementOffsetSize, tree.position()); err != nil {
			return enc.bufferPatchError("writeArrayNode", err)
		}
		if err := enc.writeNode(tree, child, elementOffsetSize); err != nil {
			return err
		}
	}
	common.Odl.Debug("osonEncoder.writeArrayNode: completed",
		"treeOffset", nodeOffset,
		"elements", count,
		"encodedBytes", tree.position()-nodeOffset)
	return nil
}

// writeObjectNode writes one object node and its member values.
func (enc *osonEncoder) writeObjectNode(tree *osonWriteBuffer, value map[string]any, childOffsetSize int) error {
	nodeOffset := tree.position()
	members, err := enc.sortedObjectMembers(value)
	if err != nil {
		return err
	}
	common.Odl.Debug("osonEncoder.writeObjectNode: begin",
		"treeOffset", nodeOffset,
		"members", len(members),
		"fieldIDWidth", enc.dict.fieldIDWidth,
		"childOffsetWidth", childOffsetSize)
	tree.writeUB1(containerOpcode(osonOpObjectType, len(members), childOffsetSize))
	tree.writeContainerCount(len(members))

	fieldIDStart := tree.reserve(len(members) * enc.dict.fieldIDWidth)
	offsetStart := tree.reserve(len(members) * childOffsetSize)
	for i, member := range members {
		if err := tree.patchUint(fieldIDStart+i*enc.dict.fieldIDWidth, enc.dict.fieldIDWidth, member.fieldID); err != nil {
			return enc.bufferPatchError("writeObjectNode", err)
		}
		if err := tree.patchUint(offsetStart+i*childOffsetSize, childOffsetSize, tree.position()); err != nil {
			return enc.bufferPatchError("writeObjectNode", err)
		}
		if err := enc.writeNode(tree, member.value, childOffsetSize); err != nil {
			return err
		}
	}
	common.Odl.Debug("osonEncoder.writeObjectNode: completed",
		"treeOffset", nodeOffset,
		"members", len(members),
		"encodedBytes", tree.position()-nodeOffset)
	return nil
}

// objectMember pairs an object value with its finalized dictionary field id.
type objectMember struct {
	// fieldID is the finalized dictionary field ID for this object key.
	fieldID int
	// value is the child value encoded at the matching object-member position.
	value any
}

// sortedObjectMembers returns object members in ascending field-id order.
func (enc *osonEncoder) sortedObjectMembers(value map[string]any) ([]objectMember, error) {
	members := make([]objectMember, 0, len(value))
	for key, child := range value {
		field, ok := enc.dict.entriesByName[key]
		if !ok {
			cause := fmt.Errorf("field %q missing from finalized OSON dictionary", key)
			common.Odl.Debug("osonEncoder.sortedObjectMembers: failed", "error", cause)
			return nil, common.NewOracleError(oracleErrors.OsonEncodingError, cause)
		}
		members = append(members, objectMember{
			fieldID: field.fieldID,
			value:   child,
		})
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].fieldID < members[j].fieldID
	})
	return members, nil
}

// writeScalarNode writes one supported scalar value into the OSON tree.
func (enc *osonEncoder) writeScalarNode(tree *osonWriteBuffer, value any) error {
	switch v := value.(type) {
	case nil:
		tree.writeUB1(osonOpNull)
		common.Odl.Debug("osonEncoder.writeScalarNode: null", "opcode", osonOpNull)
		return nil
	case bool:
		if v {
			tree.writeUB1(osonOpTrue)
			common.Odl.Debug("osonEncoder.writeScalarNode: boolean", "opcode", osonOpTrue)
		} else {
			tree.writeUB1(osonOpFalse)
			common.Odl.Debug("osonEncoder.writeScalarNode: boolean", "opcode", osonOpFalse)
		}
		return nil
	case string:
		return enc.writeString(tree, v)
	case int:
		return enc.writeInt(tree, int64(v))
	case int8:
		return enc.writeInt(tree, int64(v))
	case int16:
		return enc.writeInt(tree, int64(v))
	case int32:
		return enc.writeInt(tree, int64(v))
	case int64:
		return enc.writeInt(tree, v)
	case uint:
		return enc.writeUInt(tree, uint64(v))
	case uint8:
		return enc.writeUInt(tree, uint64(v))
	case uint16:
		return enc.writeUInt(tree, uint64(v))
	case uint32:
		return enc.writeUInt(tree, uint64(v))
	case uint64:
		return enc.writeUInt(tree, v)
	case float32:
		return enc.writeBinaryFloat(tree, v)
	case float64:
		return enc.writeBinaryDouble(tree, v)
	case []byte:
		return enc.writeBinary(tree, drvCommon.B1Array(v))
	case time.Time:
		return enc.writeTimestamp(tree, v)
	case drvCommon.JSONNumber:
		return enc.writeStringNumber(tree, string(v))
	case stdjson.Number:
		return enc.writeStringNumber(tree, v.String())
	default:
		cause := fmt.Errorf("invalid OSON scalar value type %T", value)
		common.Odl.Debug("osonEncoder.writeScalarNode: failed", "error", cause)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}
}

// writeString writes one UTF-8 string scalar node.
//
// The opcode family is selected from the encoded byte length:
//
//	0..31       => [length opcode][bytes]
//	32..255     => [0x33][UB1 length][bytes]
//	256..65535  => [0x37][UB2 length][bytes]
//	65536..UB4  => [0x38][UB4 length][bytes]
func (enc *osonEncoder) writeString(tree *osonWriteBuffer, value string) error {
	nodeOffset := tree.position()
	raw := []byte(value)
	if len(raw) > math.MaxUint32 {
		cause := fmt.Errorf("string scalar length %d exceeds OSON UB4 length limit %d", len(raw), math.MaxUint32)
		common.Odl.Debug("osonEncoder.writeString: failed", "error", cause, "length", len(raw))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}

	switch {
	case len(raw) <= int(osonOpShortStringMax):
		tree.writeUB1(drvCommon.UB1(len(raw)))
	case len(raw) <= math.MaxUint8:
		tree.writeUB1(osonOpStringUB1)
		tree.writeUB1(drvCommon.UB1(len(raw)))
	case len(raw) <= math.MaxUint16:
		tree.writeUB1(osonOpStringUB2)
		tree.writeUB2(drvCommon.UB2(len(raw)))
	default:
		tree.writeUB1(osonOpStringUB4)
		tree.writeUB4(drvCommon.UB4(len(raw)))
	}
	tree.writeBytes(drvCommon.B1Array(raw))
	common.Odl.Debug("osonEncoder.writeString: completed",
		"opcode", tree.data[nodeOffset],
		"payloadLength", len(raw))
	return nil
}

// writeInt encodes a signed integer and selects the smallest compatible OSON
// Oracle NUMBER opcode from the encoded payload length.
func (enc *osonEncoder) writeInt(tree *osonWriteBuffer, value int64) error {
	payload, err := converters.EncodeInt(value)
	if err != nil {
		return _wrapScalarEncodingError("writeInt", err)
	}
	if len(payload) == 0 {
		cause := fmt.Errorf("encoding signed integer %d produced an empty Oracle NUMBER payload", value)
		common.Odl.Debug("osonEncoder.writeInt: failed", "error", cause)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}
	if len(payload) <= _compactSigned32LengthMask {
		opcode := osonOpCompactSigned32Prefix | drvCommon.UB1(len(payload))
		common.Odl.Debug("osonEncoder.writeInt: compact SB4", "opcode", opcode, "payloadLength", len(payload))
		tree.writeUB1(opcode)
		tree.writeBytes(payload)
		return nil
	}
	if len(payload) > _compactSigned64LengthMask {
		return writeOracleNumberPayload(tree, payload, value)
	}
	opcode := osonOpCompactSigned64Prefix | drvCommon.UB1(len(payload))
	common.Odl.Debug("osonEncoder.writeInt: compact SB8", "opcode", opcode, "payloadLength", len(payload))
	tree.writeUB1(opcode)
	tree.writeBytes(payload)
	return nil
}

// writeUInt writes an unsigned integer as a generic Oracle NUMBER.
func (enc *osonEncoder) writeUInt(tree *osonWriteBuffer, value uint64) error {
	payload, err := converters.EncodeUInt(value)
	if err != nil {
		return _wrapScalarEncodingError("writeUInt", err)
	}
	return writeOracleNumberPayload(tree, payload, value)
}

// writeOracleNumberPayload selects the generic Oracle NUMBER opcode after the
// payload has been encoded. Compact NUMBER stores payload length minus one in
// the opcode; larger payloads use an explicit UB1 length.
func writeOracleNumberPayload(tree *osonWriteBuffer, payload drvCommon.B1Array, value any) error {

	if len(payload) == 0 {
		cause := fmt.Errorf("encoding integer %v produced an empty Oracle NUMBER payload", value)
		common.Odl.Debug("writeOracleNumberPayload: failed", "error", cause)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}
	if len(payload) <= _compactOracleNumberMaxPayloadLen {
		opcode := osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(payload)-1)
		common.Odl.Debug("writeOracleNumberPayload: compact oracle number", "opcode", opcode, "payloadLength", len(payload))
		tree.writeUB1(opcode)
		tree.writeBytes(payload)
		return nil
	}

	common.Odl.Debug("writeOracleNumberPayload: explicit oracle number", "opcode", osonOpOracleNumber, "payloadLength", len(payload))
	tree.writeUB1(osonOpOracleNumber)
	tree.writeUB1(drvCommon.UB1(len(payload)))
	tree.writeBytes(payload)
	return nil
}

// writeStringNumber writes a JSON number as string.
func (enc *osonEncoder) writeStringNumber(tree *osonWriteBuffer, value string) error {
	if !isJSONNumber(value) {
		cause := fmt.Errorf("value %q is not valid JSON number text", value)
		common.Odl.Debug("osonEncoder.writeStringNumber: failed", "error", cause, "length", len(value))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}

	raw := []byte(value)
	if len(raw) > math.MaxUint8 {
		cause := fmt.Errorf("string number length %d exceeds OSON UB1 length limit %d", len(raw), math.MaxUint8)
		common.Odl.Debug("osonEncoder.writeStringNumber: failed", "error", cause, "length", len(raw))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}

	tree.writeUB1(osonOpStringNumber)
	tree.writeUB1(drvCommon.UB1(len(raw)))
	tree.writeBytes(raw)

	common.Odl.Debug("osonEncoder.writeStringNumber: string number", "payloadLength", len(raw))
	return nil
}

// isJSONNumber reports whether value is exactly one RFC 8259 number token.
func isJSONNumber(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}

	decoder := stdjson.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return false
	}
	if _, ok := decoded.(stdjson.Number); !ok {
		return false
	}
	// A second decode must reach EOF; otherwise the input contained another
	// token after the number.
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

// writeBinaryFloat writes a fixed-width binary float scalar.
func (enc *osonEncoder) writeBinaryFloat(tree *osonWriteBuffer, value float32) error {
	payload, err := converters.EncodeBinaryFloat(value)
	if err != nil {
		return _wrapScalarEncodingError("writeBinaryFloat", err)
	}

	tree.writeUB1(osonOpBinaryFloat)
	tree.writeBytes(payload)

	common.Odl.Debug("osonEncoder.writeBinaryFloat: binary float", "opcode", osonOpBinaryFloat, "payloadLength", len(payload))
	return nil
}

// writeBinaryDouble writes a fixed-width binary double scalar.
func (enc *osonEncoder) writeBinaryDouble(tree *osonWriteBuffer, value float64) error {
	payload, err := converters.EncodeBinaryDouble(value)
	if err != nil {
		return _wrapScalarEncodingError("writeBinaryDouble", err)
	}

	tree.writeUB1(osonOpBinaryDouble)
	tree.writeBytes(payload)

	common.Odl.Debug("osonEncoder.writeBinaryDouble: binary double", "opcode", osonOpBinaryDouble, "payloadLength", len(payload))
	return nil
}

// writeBinary writes a variable-length binary scalar.
func (enc *osonEncoder) writeBinary(tree *osonWriteBuffer, value drvCommon.B1Array) error {
	nodeOffset := tree.position()
	if len(value) > math.MaxUint32 {
		cause := fmt.Errorf("binary scalar length %d exceeds OSON UB4 length limit %d", len(value), math.MaxUint32)
		common.Odl.Debug("osonEncoder.writeBinary: failed", "error", cause, "length", len(value))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}

	if len(value) <= math.MaxUint16 {
		tree.writeUB1(osonOpBinaryUB2)
		tree.writeUB2(drvCommon.UB2(len(value)))
	} else {
		tree.writeUB1(osonOpBinaryUB4)
		tree.writeUB4(drvCommon.UB4(len(value)))
	}

	tree.writeBytes(value)
	common.Odl.Debug("osonEncoder.writeBinary: completed",
		"opcode", tree.data[nodeOffset],
		"payloadLength", len(value))
	return nil
}

// writeTimestamp writes an Oracle TIMESTAMP scalar.
func (enc *osonEncoder) writeTimestamp(tree *osonWriteBuffer, value time.Time) error {
	payload, err := converters.EncodeTimestamp(value)
	if err != nil {
		return _wrapScalarEncodingError("writeTimestamp", err)
	}

	tree.writeUB1(osonOpTimestamp)
	tree.writeBytes(payload)

	common.Odl.Debug("osonEncoder.writeTimestamp: completed",
		"opcode", osonOpTimestamp,
		"payloadLength", len(payload))
	return nil
}

// _wrapScalarEncodingError logs converter failures and returns the public OSON
// encoding error used by scalar writers.
func _wrapScalarEncodingError(operation string, cause error) error {
	common.Odl.Debug("osonEncoder."+operation+": failed", "error", cause)
	return common.NewOracleError(oracleErrors.OsonEncodingError, cause)
}

// containerOpcode builds the container opcode from the base type, child count, and child-offset size.
func containerOpcode(base drvCommon.UB1, count, childOffsetSize int) drvCommon.UB1 {
	opcode := base
	if childOffsetSize == osonUB4Size {
		opcode |= osonOpChildOffsetUB4Bit
	}
	switch {
	case count > math.MaxUint16:
		opcode |= osonOpChildCountUB4
	case count > math.MaxUint8:
		opcode |= osonOpChildCountUB2
	default:
		opcode |= osonOpChildCountUB1
	}
	return opcode
}

// writePrimaryDictionaryHeader writes the primary dictionary count and heap size.
func (enc *osonEncoder) writePrimaryDictionaryHeader(out *osonWriteBuffer, heapSize int) {
	enc.writeUint(out, enc.dict.fieldIDWidth, len(enc.dict.primary))
	if heapSize > math.MaxUint16 {
		out.writeUB4(drvCommon.UB4(heapSize))
	} else {
		out.writeUB2(drvCommon.UB2(heapSize))
	}
}

// writeSecondaryDictionaryHeader writes the secondary dictionary header for long field names.
func (enc *osonEncoder) writeSecondaryDictionaryHeader(out *osonWriteBuffer, heapSize int) {
	var secondaryFlags drvCommon.UB2
	if heapSize <= math.MaxUint16 {
		secondaryFlags |= osonFlagSecondaryFieldOffsetsUB2Mask
	}

	out.writeUB2(secondaryFlags)
	out.writeUB4(drvCommon.UB4(len(enc.dict.secondary)))
	out.writeUB4(drvCommon.UB4(heapSize))
}

// writeTreeSegmentSize writes the tree byte length using the width selected in
// the primary header flags.
func (enc *osonEncoder) writeTreeSegmentSize(out *osonWriteBuffer, flags drvCommon.UB2, size int) {
	if flags&osonFlagTreeSegmentSizeUB4Mask != 0 {
		out.writeUB4(drvCommon.UB4(size))
		return
	}
	out.writeUB2(drvCommon.UB2(size))
}

// writeDictionary writes one complete dictionary tier.
func (enc *osonEncoder) writeDictionary(out *osonWriteBuffer, fields []*fieldNameEntry, lengthSize int) {
	for _, field := range fields {
		enc.writeUint(out, lengthSize, int(field.hash))
	}
	offsetSize := osonUB2Size
	if enc.dictionaryHeapSize(fields, lengthSize) > math.MaxUint16 {
		offsetSize = osonUB4Size
	}
	for _, field := range fields {
		enc.writeUint(out, offsetSize, field.heapOffset)
	}
	for _, field := range fields {
		enc.writeUint(out, lengthSize, len(field.raw))
		out.writeBytes(field.raw)
	}
}

// dictionaryHeapSize returns the byte count of the packed dictionary heap.
func (enc *osonEncoder) dictionaryHeapSize(fields []*fieldNameEntry, lengthSize int) int {
	size := 0
	for _, field := range fields {
		size += lengthSize + len(field.raw)
	}
	return size
}

// writeUint writes value using one of the OSON unsigned integer size.
func (enc *osonEncoder) writeUint(out *osonWriteBuffer, size, value int) {
	switch size {
	case osonUB1Size:
		out.writeUB1(drvCommon.UB1(value))
	case osonUB2Size:
		out.writeUB2(drvCommon.UB2(value))
	default:
		out.writeUB4(drvCommon.UB4(value))
	}
}

// osonWriteBuffer accumulates encoder output and supports patching reserved
// integer slots after their values become known.
type osonWriteBuffer struct {
	// data holds the encoded bytes accumulated so far.
	data drvCommon.B1Array
}

// writeUB1 appends one unsigned byte.
func (b *osonWriteBuffer) writeUB1(value drvCommon.UB1) {
	b.data = append(b.data, byte(value))
}

// writeUB2 appends one big-endian unsigned 2-byte integer.
func (b *osonWriteBuffer) writeUB2(value drvCommon.UB2) {
	raw := make([]byte, osonUB2Size)
	binary.BigEndian.PutUint16(raw, uint16(value))
	b.data = append(b.data, raw...)
}

// writeUB4 appends one big-endian unsigned 4-byte integer.
func (b *osonWriteBuffer) writeUB4(value drvCommon.UB4) {
	raw := make([]byte, osonUB4Size)
	binary.BigEndian.PutUint32(raw, uint32(value))
	b.data = append(b.data, raw...)
}

// writeBytes appends raw bytes without adding a length prefix.
func (b *osonWriteBuffer) writeBytes(value drvCommon.B1Array) {
	b.data = append(b.data, value...)
}

// position returns the current tree-relative write position.
func (b *osonWriteBuffer) position() int {
	return len(b.data)
}

// reserve appends length zero bytes and returns the start offset to patch later.
//
// Containers use this for field-id and child-offset tables. Those table bytes
// must physically appear before the children, but their values are only known
// during recursive child emission.
func (b *osonWriteBuffer) reserve(length int) int {
	start := len(b.data)
	b.data = append(b.data, make(drvCommon.B1Array, length)...)
	return start
}

// writeContainerCount appends a container child/member count.
func (b *osonWriteBuffer) writeContainerCount(count int) {
	switch {
	case count > math.MaxUint16:
		b.writeUB4(drvCommon.UB4(count))
	case count > math.MaxUint8:
		b.writeUB2(drvCommon.UB2(count))
	default:
		b.writeUB1(drvCommon.UB1(count))
	}
}

// patchUint overwrites a previously reserved unsigned integer slot.
func (b *osonWriteBuffer) patchUint(offset, width, value int) error {
	if offset < 0 {
		return fmt.Errorf("patch offset %d is negative", offset)
	}
	if value < 0 {
		return fmt.Errorf("patch value %d is negative", value)
	}
	var maxValue uint64
	switch width {
	case osonUB1Size:
		maxValue = math.MaxUint8
	case osonUB2Size:
		maxValue = math.MaxUint16
	case osonUB4Size:
		maxValue = math.MaxUint32
	default:
		return fmt.Errorf("unsupported patch width %d", width)
	}

	if uint64(value) > maxValue {
		return fmt.Errorf("patch value %d overflows UB%d", value, width)
	}
	if offset > len(b.data)-width {
		return fmt.Errorf("patch range [%d:%d] exceeds buffer length %d", offset, offset+width, len(b.data))
	}

	switch width {
	case osonUB1Size:
		b.data[offset] = byte(drvCommon.UB1(value))
	case osonUB2Size:
		binary.BigEndian.PutUint16(b.data[offset:], uint16(value))
	case osonUB4Size:
		binary.BigEndian.PutUint32(b.data[offset:], uint32(value))
	}
	return nil
}

// bytes returns a defensive copy of the bytes written so far.
func (b *osonWriteBuffer) bytes() drvCommon.B1Array {
	out := make(drvCommon.B1Array, len(b.data))
	copy(out, b.data)
	return out
}
