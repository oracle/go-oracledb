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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// Local UB bounds keep length-width decisions tied to OSON field widths
	_maxUB1 = 1<<8 - 1
	_maxUB2 = 1<<16 - 1
	_maxUB4 = 1<<32 - 1

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
	enc := newOsonEncoder()
	return enc.encode(value)
}

// osonEncoder keeps the state needed to build one OSON document.
type osonEncoder struct {
	// Field-name dictionary for this document.
	dict fieldNameDictionary

	// OSON document version.
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
		return nil, common.NewOracleError(oracleErrors.OsonEncodingError, err, fmt.Sprintf("%T", value))
	}
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

	return enc.emitScalarDocument(), nil
}

// prepareScalarHeader sets the header for a scalar OSON document.
func (enc *osonEncoder) prepareScalarHeader() {
	enc.version = osonFormatMinVersion
	enc.flags = osonFlagInlineLeafMask | osonFlagStringLengthInOpcodeMask | osonFlagScalarDocumentMask
	if len(enc.treeSegmentBytes) > _maxUB2 {
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
		return nil, err
	}
	if err := enc.prepareTreeSegment(value); err != nil {
		return nil, err
	}

	enc.prepareContainerHeader()
	return enc.emitContainerDocument(), nil
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
	if enc.primaryHeapSize > _maxUB2 {
		enc.flags |= osonFlagFieldHeapSizeUB4Mask
	}
	if len(enc.treeSegmentBytes) > _maxUB2 {
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
		return common.NewOracleError(oracleErrors.OsonEncodingError, err, fmt.Sprintf("%T", value))
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
		common.Odl.Error("osonEncoder.addFieldName: failed", "error", cause, "field", name, "length", byteLen)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, byteLen)
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
	case total > _maxUB2:
		enc.dict.fieldIDWidth = osonUB4Size
	case total > _maxUB1:
		enc.dict.fieldIDWidth = osonUB2Size
	default:
		enc.dict.fieldIDWidth = osonUB1Size
	}
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
		return common.NewOracleError(oracleErrors.OsonEncodingError, err, fmt.Sprintf("%T", value))
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
	common.Odl.Error("osonEncoder."+operation+": failed", "error", err)
	return common.NewOracleError(oracleErrors.OsonEncodingError, err, nil)
}

// writeArrayNode writes one array node and its elements.
func (enc *osonEncoder) writeArrayNode(tree *osonWriteBuffer, value []any, elementOffsetSize int) error {
	count := len(value)
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
	return nil
}

// writeObjectNode writes one object node and its member values.
func (enc *osonEncoder) writeObjectNode(tree *osonWriteBuffer, value map[string]any, childOffsetSize int) error {
	members, err := enc.sortedObjectMembers(value)
	if err != nil {
		return err
	}
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
			common.Odl.Error("osonEncoder.sortedObjectMembers: failed", "error", cause)
			return nil, common.NewOracleError(oracleErrors.OsonEncodingError, cause, key)
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
		return nil
	case bool:
		if v {
			tree.writeUB1(osonOpTrue)
		} else {
			tree.writeUB1(osonOpFalse)
		}
		return nil
	case string:
		return enc.writeStringScalar(tree, v)
	case int:
		if strconv.IntSize <= 32 {
			return enc.writeSignedIntScalar(tree, int64(v), osonOpCompactSigned32Prefix, int32Size)
		}
		return enc.writeSignedIntScalar(tree, int64(v), osonOpCompactSigned64Prefix, int64Size)
	case int8:
		return enc.writeSignedIntScalar(tree, int64(v), osonOpCompactSigned32Prefix, int32Size)
	case int16:
		return enc.writeSignedIntScalar(tree, int64(v), osonOpCompactSigned32Prefix, int32Size)
	case int32:
		return enc.writeSignedIntScalar(tree, int64(v), osonOpCompactSigned32Prefix, int32Size)
	case int64:
		return enc.writeSignedIntScalar(tree, v, osonOpCompactSigned64Prefix, int64Size)
	case uint:
		return enc.writeUnsignedIntScalar(tree, uint64(v))
	case uint8:
		return enc.writeUnsignedIntScalar(tree, uint64(v))
	case uint16:
		return enc.writeUnsignedIntScalar(tree, uint64(v))
	case uint32:
		return enc.writeUnsignedIntScalar(tree, uint64(v))
	case uint64:
		return enc.writeUnsignedIntScalar(tree, v)
	case float32:
		return enc.writeBinaryFloatScalar(tree, v)
	case float64:
		return enc.writeBinaryDoubleScalar(tree, v)
	case []byte:
		return enc.writeBinaryScalar(tree, drvCommon.B1Array(v))
	case time.Time:
		return enc.writeTimestampScalar(tree, v)
	case drvCommon.JSONNumber:
		return enc.writeStringNumberScalar(tree, string(v))
	case stdjson.Number:
		return enc.writeStringNumberScalar(tree, v.String())
	default:
		cause := fmt.Errorf("unsupported OSON scalar value type %T", value)
		common.Odl.Debug("osonEncoder.writeScalarNode: failed", "error", cause)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, fmt.Sprintf("%T", value))
	}
}

// writeStringScalar writes one UTF-8 string scalar node.
//
// The opcode family is selected from the encoded byte length:
//
//	0..31       => [length opcode][bytes]
//	32..255     => [0x33][UB1 length][bytes]
//	256..65535  => [0x37][UB2 length][bytes]
//	65536..UB4  => [0x38][UB4 length][bytes]
func (enc *osonEncoder) writeStringScalar(tree *osonWriteBuffer, value string) error {
	raw := []byte(value)
	if len(raw) > _maxUB4 {
		cause := fmt.Errorf("string scalar length %d exceeds OSON UB4 length limit", len(raw))
		common.Odl.Error("osonEncoder.writeStringScalar: failed", "error", cause, "length", len(raw))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, len(raw))
	}

	switch {
	case len(raw) <= int(osonOpShortStringMax):
		tree.writeUB1(drvCommon.UB1(len(raw)))
	case len(raw) <= _maxUB1:
		tree.writeUB1(osonOpStringUB1)
		tree.writeUB1(drvCommon.UB1(len(raw)))
	case len(raw) <= _maxUB2:
		tree.writeUB1(osonOpStringUB2)
		tree.writeUB2(drvCommon.UB2(len(raw)))
	default:
		tree.writeUB1(osonOpStringUB4)
		tree.writeUB4(drvCommon.UB4(len(raw)))
	}
	tree.writeBytes(drvCommon.B1Array(raw))
	return nil
}

// writeSignedIntScalar writes a signed integer scalar.
func (enc *osonEncoder) writeSignedIntScalar(tree *osonWriteBuffer, value int64, opcodeMask drvCommon.UB1, platformSize drvCommon.UB1) error {
	payload, err := converters.EncodeInt(value)
	if err != nil {
		return enc.scalarEncodingError("writeSignedIntScalar", err, value)
	}
	if len(payload) == 0 {
		cause := fmt.Errorf("empty compact signed integer payload")
		common.Odl.Error("osonEncoder.writeSignedIntScalar: failed", "error", cause, "opcodeMask", opcodeMask)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, fmt.Sprintf("%T", value))
	}
	if len(payload) > int(platformSize) {
		cause := fmt.Errorf("compact signed integer payload length %d exceeds OSON opcode capacity", len(payload))
		common.Odl.Error("osonEncoder.writeSignedIntScalar: failed", "error", cause, "payloadLength", len(payload), "lengthMask", platformSize)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, fmt.Sprintf("%T", value))
	}
	opcode := opcodeMask | drvCommon.UB1(len(payload))
	common.Odl.Debug("osonEncoder.writeSignedIntScalar: compact signed number", "opcode", opcode, "payloadLength", len(payload))
	tree.writeUB1(opcode)
	tree.writeBytes(payload)
	return nil
}

// writeUnsignedIntScalar writes an unsigned integer as an Oracle NUMBER scalar.
func (enc *osonEncoder) writeUnsignedIntScalar(tree *osonWriteBuffer, value uint64) error {
	payload, err := converters.EncodeUInt(value)
	if err != nil {
		return enc.scalarEncodingError("writeUnsignedIntScalar", err, value)
	}

	if len(payload) == 0 {
		cause := fmt.Errorf("empty Oracle NUMBER payload")
		common.Odl.Error("osonEncoder.writeUnsignedIntScalar: failed", "error", cause)
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, nil)
	}
	if len(payload) <= _compactOracleNumberMaxPayloadLen {
		opcode := osonOpCompactOracleNumberPrefix | drvCommon.UB1(len(payload)-1)
		common.Odl.Debug("osonEncoder.writeUnsignedIntScalar: compact oracle number", "opcode", opcode, "payloadLength", len(payload))
		tree.writeUB1(opcode)
		tree.writeBytes(payload)
		return nil
	}

	if len(payload) > _maxUB1 {
		cause := fmt.Errorf("number payload length %d exceeds OSON UB1 length limit", len(payload))
		common.Odl.Error("osonEncoder.writeUnsignedIntScalar: failed", "error", cause, "length", len(payload))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, len(payload))
	}
	common.Odl.Debug("osonEncoder.writeUnsignedIntScalar: explicit oracle number", "opcode", osonOpOracleNumber, "payloadLength", len(payload))
	tree.writeUB1(osonOpOracleNumber)
	tree.writeUB1(drvCommon.UB1(len(payload)))
	tree.writeBytes(payload)
	return nil
}

// writeStringNumberScalar writes a JSON number as string.
func (enc *osonEncoder) writeStringNumberScalar(tree *osonWriteBuffer, value string) error {
	if !isJSONNumber(value) {
		cause := fmt.Errorf("invalid JSON number text")
		common.Odl.Error("osonEncoder.writeStringNumberScalar: failed", "error", cause, "length", len(value))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, value)
	}

	raw := []byte(value)
	if len(raw) > _maxUB1 {
		cause := fmt.Errorf("string number length %d exceeds OSON UB1 length limit", len(raw))
		common.Odl.Error("osonEncoder.writeStringNumberScalar: failed", "error", cause, "length", len(raw))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, len(raw))
	}

	tree.writeUB1(osonOpStringNumber)
	tree.writeUB1(drvCommon.UB1(len(raw)))
	tree.writeBytes(raw)

	common.Odl.Debug("osonEncoder.writeStringNumberScalar: string number", "payloadLength", len(raw))
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

// writeBinaryFloatScalar writes a fixed-width binary float scalar.
func (enc *osonEncoder) writeBinaryFloatScalar(tree *osonWriteBuffer, value float32) error {
	payload, err := converters.EncodeBinaryFloat(value)
	if err != nil {
		return enc.scalarEncodingError("writeBinaryFloatScalar", err, value)
	}

	tree.writeUB1(osonOpBinaryFloat)
	tree.writeBytes(payload)

	common.Odl.Debug("osonEncoder.writeBinaryFloatScalar: binary float", "opcode", osonOpBinaryFloat, "payloadLength", len(payload))
	return nil
}

// writeBinaryDoubleScalar writes a fixed-width binary double scalar.
func (enc *osonEncoder) writeBinaryDoubleScalar(tree *osonWriteBuffer, value float64) error {
	payload, err := converters.EncodeBinaryDouble(value)
	if err != nil {
		return enc.scalarEncodingError("writeBinaryDoubleScalar", err, value)
	}

	tree.writeUB1(osonOpBinaryDouble)
	tree.writeBytes(payload)

	common.Odl.Debug("osonEncoder.writeBinaryDoubleScalar: binary double", "opcode", osonOpBinaryDouble, "payloadLength", len(payload))
	return nil
}

// writeBinaryScalar writes a variable-length binary scalar.
func (enc *osonEncoder) writeBinaryScalar(tree *osonWriteBuffer, value drvCommon.B1Array) error {
	if len(value) > _maxUB4 {
		cause := fmt.Errorf("binary scalar length %d exceeds OSON UB4 length limit", len(value))
		common.Odl.Error("osonEncoder.writeBinaryScalar: failed", "error", cause, "length", len(value))
		return common.NewOracleError(oracleErrors.OsonEncodingError, cause, len(value))
	}

	if len(value) <= _maxUB2 {
		tree.writeUB1(osonOpBinaryUB2)
		tree.writeUB2(drvCommon.UB2(len(value)))
	} else {
		tree.writeUB1(osonOpBinaryUB4)
		tree.writeUB4(drvCommon.UB4(len(value)))
	}

	tree.writeBytes(value)
	return nil
}

// writeTimestampScalar writes an Oracle TIMESTAMP scalar.
func (enc *osonEncoder) writeTimestampScalar(tree *osonWriteBuffer, value time.Time) error {
	payload, err := converters.EncodeTimestamp(value)
	if err != nil {
		return enc.scalarEncodingError("writeTimestampScalar", err, value)
	}

	tree.writeUB1(osonOpTimestamp)
	tree.writeBytes(payload)

	return nil
}

// scalarEncodingError logs converter failures and returns the public OSON
// encoding error used by scalar writers.
func (enc *osonEncoder) scalarEncodingError(operation string, cause error, value any) error {
	common.Odl.Error("osonEncoder."+operation+": failed", "error", cause)
	return common.NewOracleError(oracleErrors.OsonEncodingError, cause, fmt.Sprintf("%T", value))
}

// containerOpcode builds the container opcode from the base type, child count, and child-offset size.
func containerOpcode(base drvCommon.UB1, count, childOffsetSize int) drvCommon.UB1 {
	opcode := base
	if childOffsetSize == osonUB4Size {
		opcode |= osonOpChildOffsetUB4Bit
	}
	switch {
	case count > _maxUB2:
		opcode |= osonOpChildCountUB4
	case count > _maxUB1:
		opcode |= osonOpChildCountUB2
	default:
		opcode |= osonOpChildCountUB1
	}
	return opcode
}

// writePrimaryDictionaryHeader writes the primary dictionary count and heap size.
func (enc *osonEncoder) writePrimaryDictionaryHeader(out *osonWriteBuffer, heapSize int) {
	enc.writeUint(out, enc.dict.fieldIDWidth, len(enc.dict.primary))
	if heapSize > _maxUB2 {
		out.writeUB4(drvCommon.UB4(heapSize))
	} else {
		out.writeUB2(drvCommon.UB2(heapSize))
	}
}

// writeSecondaryDictionaryHeader writes the secondary dictionary header for long field names.
func (enc *osonEncoder) writeSecondaryDictionaryHeader(out *osonWriteBuffer, heapSize int) {
	var secondaryFlags drvCommon.UB2
	if heapSize <= _maxUB2 {
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
	if enc.dictionaryHeapSize(fields, lengthSize) > _maxUB2 {
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
	case count > _maxUB2:
		b.writeUB4(drvCommon.UB4(count))
	case count > _maxUB1:
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
		maxValue = _maxUB1
	case osonUB2Size:
		maxValue = _maxUB2
	case osonUB4Size:
		maxValue = _maxUB4
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
