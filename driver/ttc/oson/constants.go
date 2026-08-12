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

import "github.com/oracle/go-driver/driver/common"

// Internal wire-format widths, fixed payload sizes, and header bounds used across the OSON parser.
const (
	// osonUB1Size is the byte size of one OSON UB1 field.
	osonUB1Size = 1
	// osonUB2Size is the byte size of one OSON UB2 field.
	osonUB2Size = 2
	// osonUB4Size is the byte size of one OSON UB4 field.
	osonUB4Size = 4

	// osonMagicPrefix is the fixed `FF 4A 5A` OSON magic prefix with a zeroed version byte.
	osonMagicPrefix = 0xff_4a_5a_00
	// osonFormatMinVersion is the oldest OSON format version accepted by this parser.
	osonFormatMinVersion = 1
	// osonFormatMaxVersion is the newest OSON format version accepted by this parser.
	osonFormatMaxVersion = 3

	// osonHeaderMinSize is the minimum size of the fixed OSON header prefix.
	osonHeaderMinSize = 6
	// osonMagicPrefixMask selects the magic-prefix bytes from the first header word.
	osonMagicPrefixMask = 0xff_ff_ff_00
	// osonVersionByteMask selects the format-version byte from the first header word.
	osonVersionByteMask = 0x00_00_00_ff

	// osonUpdateMappingEntrySizeUB2 is the byte size of one UB2 overflow mapping pair.
	osonUpdateMappingEntrySizeUB2 = 2 * 2
	// osonUpdateMappingEntrySizeUB4 is the byte size of one UB4 overflow mapping pair.
	osonUpdateMappingEntrySizeUB4 = 4 * 2

	// osonScalarOpcodeSize is the size in bytes of a scalar encoded only by its opcode byte.
	osonScalarOpcodeSize = 1
	// osonScalarHeaderSizeUB1 is the size in bytes of [opcode][UB1 length].
	osonScalarHeaderSizeUB1 = 2
	// osonScalarHeaderSizeUB2 is the size in bytes of [opcode][UB2 length].
	osonScalarHeaderSizeUB2 = 3
	// osonScalarHeaderSizeUB4 is the size in bytes of [opcode][UB4 length].
	osonScalarHeaderSizeUB4 = 5

	// osonBinaryFloatPayloadSize is the fixed payload size of one binary float scalar.
	osonBinaryFloatPayloadSize = 4
	// osonBinaryDoublePayloadSize is the fixed payload size of one binary double scalar.
	osonBinaryDoublePayloadSize = 8
	// osonDatePayloadSize is the fixed payload size of one Oracle DATE scalar.
	osonDatePayloadSize = 7
	// osonIntervalYMPayloadSize is the fixed payload size of one INTERVAL YEAR TO MONTH scalar.
	osonIntervalYMPayloadSize = 5
	// osonIntervalDSPayloadSize is the fixed payload size of one INTERVAL DAY TO SECOND scalar.
	osonIntervalDSPayloadSize = 11
	// osonTimestampPayloadSize is the fixed payload size of one TIMESTAMP scalar.
	osonTimestampPayloadSize = 11
	// osonTimestamp7PayloadSize is the fixed payload size of one 7-byte TIMESTAMP scalar.
	osonTimestamp7PayloadSize = 7
	// osonTimestampTZPayloadSize is the fixed payload size of one TIMESTAMP WITH TIME ZONE scalar.
	osonTimestampTZPayloadSize = 13

	// osonIDMaxPayloadSize is the largest payload that fits the UB1-length OSON ID encoding.
	osonIDMaxPayloadSize = 127

	osonObjectOrArrayOpMask = 0xC0
	osonCompactSigned32Mask = 0xF8
	osonCompactSigned64Mask = 0xF0
	osonCompactNumberMask   = 0xF0
)

// FNV hash constants used to reproduce OSON field-name hash compatibility.
const (
	// osonFNVOffsetBasis32 is the 32-bit FNV offset basis for OSON field-name hashes.
	osonFNVOffsetBasis32 = 0x811C9DC5
	// osonFNVPrime32 is the 32-bit FNV prime for OSON field-name hashes.
	osonFNVPrime32 = 16777619
)

// Flag masks in the 2-byte secondary flag.
const (
	// UB2 entries for field-name offsets when the field-name heap fits within UB2 limit.
	osonFlagSecondaryFieldOffsetsUB2Mask = 0x0100
	// osonFlagUpdateOverflowSegmentUB2Mask indicates the overflow-address mapping
	// segment stores UB2 key/value entries; otherwise entries are UB4.
	osonFlagUpdateOverflowSegmentUB2Mask = 0x0100
)

// Flag masks in the 2-byte primary flag.
const (
	// osonFlagRelativeOffsetsMask indicates child offsets are stored as signed
	// deltas relative to the containing node's tree-relative offset.
	osonFlagRelativeOffsetsMask = 0x0001

	// osonFlagInlineLeafMask indicates JSON scalar leaf values are inlined in the
	// tree node instead of stored in a separate leaf-value segment.
	osonFlagInlineLeafMask = 0x0002

	// osonFlagStringLengthInOpcodeMask indicates short string lengths are encoded
	// directly in the scalar opcode when possible.
	osonFlagStringLengthInOpcodeMask = 0x0004

	// osonFlagDistinctFieldCountUB4Mask indicates the distinct-field count for
	// field names with length <= 255 and the encoded object field-id width use UB4.
	osonFlagDistinctFieldCountUB4Mask = 0x0008

	// osonFlagScalarDocumentMask marks a top-level scalar OSON document.
	osonFlagScalarDocumentMask = 0x0010

	// osonFlagPrimaryHashIDsUseUB1Mask indicates primary dictionary hash IDs are
	// stored as UB1 values.
	osonFlagPrimaryHashIDsUseUB1Mask = 0x0100

	// osonFlagDistinctFieldCountUB2Mask indicates the distinct-field count for
	// field names with length <= 255 and the encoded object field-id width use UB2.
	osonFlagDistinctFieldCountUB2Mask = 0x0400

	// osonFlagFieldHeapSizeUB4Mask indicates the field-name heap size and
	// offset-array entry width for field names with length <= 255 use UB4.
	osonFlagFieldHeapSizeUB4Mask = 0x0800

	// osonFlagTreeSegmentSizeUB4Mask indicates the tree segment size is stored as UB4.
	osonFlagTreeSegmentSizeUB4Mask = 0x1000

	// osonFlagObjectFieldsUnsortedMask indicates object field ids may be unsorted.
	osonFlagObjectFieldsUnsortedMask = 0x8000
)

// Dictionary limits and size bounds derived from OSON field-name encoding.
const (
	// osonMaxPrimaryDictKeyLength limits primary dictionary field names.
	osonMaxPrimaryDictKeyLength = 0xff
	// osonMaxSecondaryDictKeyLength limits secondary dictionary field names.
	osonMaxSecondaryDictKeyLength = 0xffff
	// osonPrimaryDictHashIDSizeUB1 is the primary hash-id size.
	osonPrimaryDictHashIDSizeUB1 = 1
	// osonSecondaryDictHashIDSizeUB2 is the secondary hash-id size.
	osonSecondaryDictHashIDSizeUB2 = 2
)

// OSON op constants.
const (
	// Object container prefix.
	osonOpObjectType = 0x80
	// Array container prefix.
	osonOpArrayType = 0xC0
	// Uses UB4 child offsets instead of UB2.
	osonOpChildOffsetUB4Bit = 0x20
	// Child-size selector bits.
	osonOpChildSizeBits = 0x18
	// Direct child count uses UB1.
	osonOpChildCountUB1 = 0x00
	// Direct child count uses UB2.
	osonOpChildCountUB2 = 0x08
	// Direct child count uses UB4.
	osonOpChildCountUB4 = 0x10
	// Child header uses delegate/shared-FID form.
	osonOpChildDelegateForm = 0x18
	// Child field IDs are not sorted.
	osonOpChildNoSortBit = 0x04
	// Object shares its field-ID array.
	osonOpObjectSharedFieldIDsBit = 0x02
	// Object content is in the extended tree segment.
	osonOpObjectUpdateOverflowBit = 0x01
	// Object prefix with shared field IDs and overflow bits set.
	osonOpUpdatedObjectReferencePattern = osonOpObjectType | osonOpObjectSharedFieldIDsBit | osonOpObjectUpdateOverflowBit

	// Short UTF-8 string prefix.
	osonOpShortStringMax = 0x1f
	// Compact Oracle NUMBER prefix.
	osonOpCompactOracleNumberPrefix = 0x20

	// JSON null.
	osonOpNull = 0x30
	// JSON true.
	osonOpTrue = 0x31
	// JSON false.
	osonOpFalse = 0x32
	// String with UB1 length.
	osonOpStringUB1 = 0x33
	// Explicit-length Oracle NUMBER.
	osonOpOracleNumber = 0x34
	// Numeric text with UB1-style length.
	osonOpStringNumber = 0x35
	// Binary double.
	osonOpBinaryDouble = 0x36
	// String with UB2 length.
	osonOpStringUB2 = 0x37
	// String with UB4 length.
	osonOpStringUB4 = 0x38
	// 11-byte TIMESTAMP.
	osonOpTimestamp = 0x39
	// Binary with UB2 length.
	osonOpBinaryUB2 = 0x3a
	// Binary with UB4 length.
	osonOpBinaryUB4 = 0x3b
	// 7-byte Oracle DATE.
	osonOpDate = 0x3c
	// INTERVAL YEAR TO MONTH.
	osonOpIntervalYM = 0x3d
	// INTERVAL DAY TO SECOND.
	osonOpIntervalDS = 0x3e

	// Compact signed NUMBER prefix for up to 32-bit payloads.
	osonOpCompactSigned32Prefix = 0x40
	// Compact signed NUMBER prefix for up to 64-bit payloads.
	osonOpCompactSigned64Prefix = 0x50
	// Compact decimal NUMBER prefix.
	osonOpCompactDecimalPrefix = 0x60

	// Explicit-length decimal NUMBER.
	osonOpOracleDecimal = 0x74
	// Redirects to the overflow mapping segment.
	osonOpUpdateOverflow = 0x75
	// Redirects to the extended tree using UB2.
	osonOpUpdateForwardUB2 = 0x76
	// Redirects to the extended tree using UB4.
	osonOpUpdateForwardUB4 = 0x77
	// Reserved update value.
	osonOpUpdateOversizeReserved = 0x78
	// TIMESTAMP WITH TIME ZONE.
	osonOpTimestampTZ = 0x7c
	// Compact 7-byte TIMESTAMP.
	osonOpTimestamp7 = 0x7d
	// UB1-length binary ID.
	osonOpID = 0x7e
	// Binary float.
	osonOpBinaryFloat = 0x7f
)

// int platform size
const (
	int32Size = 32
	int64Size = 64
)

// isShortStringOpcode reports whether opcode matches `0b000xxxxx`.
func isShortStringOpcode(opcode common.UB1) bool {
	return opcode <= osonOpShortStringMax
}

// isObjectOpcode reports whether the top two bits classify opcode as an object
// container.
func isObjectOpcode(opcode common.UB1) bool {
	return opcode&osonObjectOrArrayOpMask == osonOpObjectType
}

// isArrayOpcode reports whether the top two bits classify opcode as an array
// container.
func isArrayOpcode(opcode common.UB1) bool {
	return opcode&osonObjectOrArrayOpMask == osonOpArrayType
}

// isCompactSigned32Opcode reports whether opcode matches `0b01000xxx`.
func isCompactSigned32Opcode(opcode common.UB1) bool {
	return opcode&osonCompactSigned32Mask == osonOpCompactSigned32Prefix
}

// isCompactSigned64Opcode reports whether opcode matches `0b0101xxxx`.
func isCompactSigned64Opcode(opcode common.UB1) bool {
	return opcode&osonCompactSigned64Mask == osonOpCompactSigned64Prefix
}

// isCompactOracleNumberOpcode reports whether opcode matches `0b0010xxxx`.
func isCompactOracleNumberOpcode(opcode common.UB1) bool {
	return opcode&osonCompactNumberMask == osonOpCompactOracleNumberPrefix
}

// isCompactDecimalOpcode reports whether opcode matches `0b0110xxxx`.
func isCompactDecimalOpcode(opcode common.UB1) bool {
	return opcode&osonCompactNumberMask == osonOpCompactDecimalPrefix
}
