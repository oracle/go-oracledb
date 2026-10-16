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
	"fmt"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// _parsedDictionaryLayout holds the dictionary counts and heap sizes needed to decode the wire format.
type _parsedDictionaryLayout struct {
	// primaryCount is the number of primary-tier field names.
	primaryCount uint
	// primaryHeapSize is the byte size of the primary dictionary heap.
	primaryHeapSize uint
	// secondaryCount is the number of secondary-tier field names.
	secondaryCount uint
	// secondaryHeapSize is the byte size of the secondary dictionary heap.
	secondaryHeapSize uint
}

// dictionary stores the decoded OSON field-name dictionary.
//
// OSON stores object keys separately from the tree payload:
//   - hashIDs contains the compact hashes used for lookup
//   - fieldNames contains the UTF-8 names at the same indexes
//
// The two slices stay parallel and preserve wire order.
type dictionary struct {
	// hashIDs stores sorted compact hashes parallel to fieldNames.
	hashIDs []uint32
	// fieldNames stores decoded UTF-8 names parallel to hashIDs.
	fieldNames []string
}

// fieldNameAt resolves a zero-based field index from the merged dictionary.
func (d dictionary) fieldNameAt(fid int) (string, bool) {
	// Negative ids are always invalid.
	if fid < 0 {
		return "", false
	}
	// The merged dictionary is indexed directly by zero-based field id.
	if fid < len(d.fieldNames) {
		return d.fieldNames[fid], true
	}
	return "", false
}

// osonHeader stores the parsed metadata needed to read one OSON document.
//
// The header describes the document in three parts:
//  1. Fixed header fields such as version, flags, and segment sizes.
//  2. The field-name dictionary used to resolve object member IDs.
//  3. The encoded tree segment that holds the document values.
type osonHeader struct {
	// formatVersion is the low byte of the OSON magic/version word.
	formatVersion drvCommon.UB1

	// flags are the main OSON header flags that control layout choices.
	flags drvCommon.UB2

	// secondaryFlags are the optional v3+ extension flags.
	secondaryFlags drvCommon.UB2

	// fieldDictionary stores all decoded field names in wire order.
	fieldDictionary dictionary

	// primaryFieldsCount is the number of primary-dictionary entries.
	primaryFieldsCount int

	// treeSegmentByteLength is the declared size of the encoded tree segment.
	treeSegmentByteLength uint

	// treeSegmentStartOffset is the absolute offset where the tree segment begins.
	treeSegmentStartOffset int

	// tinyNodeStatCount is the tiny-node count recorded for non-scalar documents.
	tinyNodeStatCount drvCommon.UB2

	// updateHeaderFlags are the optional update-header flags.
	updateHeaderFlags drvCommon.UB2

	// extendedTreeSegmentStartOffset is the absolute offset of the extended tree segment.
	extendedTreeSegmentStartOffset int
	// extendedTreeSegmentByteLength is the declared length of the extended tree segment.
	extendedTreeSegmentByteLength uint

	// forwardingAddresses maps tree-relative offsets from the base tree to the extended tree.
	forwardingAddresses map[int]int
}

// newOsonHeader parses the metadata at the start of an OSON document.
//
// Parameters:
//   - buffer: document buffer positioned at the OSON fixed header.
//
// Returns:
//   - A populated header and buffer positioned at the primary tree segment.
//
// Errors:
//   - Returns an internal error when the document is truncated, malformed,
//     unsupported, or contains an invalid dictionary or tree segment.
func newOsonHeader(buffer *osonBuffer) (*osonHeader, error) {

	header := &osonHeader{}
	if err := header.initialize(buffer); err != nil {
		return nil, err
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("newOsonHeader: parsed", "version", header.formatVersion,
			"uniqueFields", header.primaryFieldsCount, "uniqueFields2", len(header.fieldDictionary.fieldNames)-header.primaryFieldsCount,
			"treeSegmentOffset", header.treeSegmentStartOffset, "treeSegmentSize", header.treeSegmentByteLength)
	}
	return header, nil
}

// initialize performs the full header parse on buf.
//
// On success:
//   - all header fields are populated
//   - any dictionaries have been decoded
//   - the buffer is rewound to the start of the main tree segment so callers
//     can begin decoding nodes immediately
//
// Errors:
//   - Returns an internal error for an invalid OSON header,
//     dictionary, tree segment, or optional update-header tail.
func (h *osonHeader) initialize(buf *osonBuffer) error {
	// parse the fixed header first so dictionary sizes and counts are known.
	layout, err := h.readHeader(buf)
	if err != nil {
		return err
	}

	// For scalar payloads the tree starts immediately after the fixed header.
	// For object/array payloads the dictionary readers advance the buffer and
	// overwrite this value with the post-dictionary tree start.
	h.treeSegmentStartOffset = buf.position()

	if !h.isScalar() {
		// decode the <=255-byte key dictionary first
		if layout.primaryCount > 0 {
			if err := h.readPrimaryDictionary(buf, layout); err != nil {
				return err
			}
		}

		// then decode the v3+ >255-byte key dictionary, if present.
		if layout.secondaryCount > 0 {
			if err := h.readSecondaryDictionary(buf, layout); err != nil {
				return err
			}
		}

		// this flag should be always on
		if !h.isSet(osonFlagInlineLeafMask) {
			details := fmt.Sprintf("non-scalar header flags 0x%04x do not set required inline-leaf flag 0x%04x", h.flags, osonFlagInlineLeafMask)
			common.Odl.Debug("osonHeader.initialize: failed", "reason", details, "flags", h.flags)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
	}

	if err := buf.ensureRange(h.treeSegmentStartOffset, int(h.treeSegmentByteLength), "osonHeader.initialize"); err != nil {
		common.Odl.Debug("osonHeader.initialize: failed",
			"error", err,
			"treeOffset", h.treeSegmentStartOffset,
			"treeBytes", h.treeSegmentByteLength,
			"documentBytes", buf.size())
		return err
	}

	// some OSON documents carry an update header, overflow-address mapping, and an
	// extended tree segment after the primary tree segment.
	if err := h.readOptionalUpdateHeader(buf); err != nil {
		return err
	}

	// consumers expect to start decoding from the main tree segment
	if err := buf.setPosition(h.treeSegmentStartOffset); err != nil {
		common.Odl.Debug("osonHeader.initialize: failed", "error", err, "treeOffset", h.treeSegmentStartOffset)
		return err
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("osonHeader.initialize: completed",
			"treeSegmentOffset", h.treeSegmentStartOffset,
			"treeSegmentSize", h.treeSegmentByteLength,
			"tinyNodeCount", h.tinyNodeStatCount)
	}
	return nil
}

// readOptionalUpdateHeader parses the update-header tail when bytes remain after
// the primary tree segment.
//
// Parameters:
//   - buf: buffer containing the primary tree followed, optionally, by update metadata.
//
// Errors:
//   - Returns an internal error when an update header, mapping segment, or
//     extended tree segment is truncated or structurally inconsistent.
func (h *osonHeader) readOptionalUpdateHeader(buf *osonBuffer) error {
	// some OSON documents append partial-update metadata after the primary tree:
	//   [update header][overflow mapping segment][extended tree segment]
	//
	// Ordinary documents stop exactly at the end of the primary tree segment, so the
	// absence of trailing bytes simply means "no update metadata present".
	updateHeaderOffset := h.treeSegmentStartOffset + int(h.treeSegmentByteLength)
	if updateHeaderOffset >= buf.size() {
		return nil
	}
	if h.formatVersion != 2 && h.formatVersion != 4 {
		details := fmt.Sprintf("update metadata is not valid for OSON version %d", h.formatVersion)
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "reason", details)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	if err := buf.setPosition(updateHeaderOffset); err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "updateOffset", updateHeaderOffset)
		return err
	}

	flags, err := buf.readUB2()
	if err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err)
		return err
	}
	numMappings, err := buf.readUB2()
	if err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err)
		return err
	}

	reserved, err := buf.readUB4()
	if err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err)
		return err
	}
	mappingSegmentSize, err := buf.readUB4()
	if err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err)
		return err
	}
	extendedTreeSize, err := buf.readUB4()
	if err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err)
		return err
	}

	// The update metadata entry count and map segment size have fixed limits;
	// reject oversized values before trusting any derived segment width.
	if numMappings > osonUpdateMaxOverflowEntries || mappingSegmentSize > osonUpdateMaxOverflowMapSize {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "reason", "update metadata exceeds OSON limits",
			"mappingCount", numMappings, "maxMappingCount", osonUpdateMaxOverflowEntries,
			"mappingBytes", mappingSegmentSize, "maxMappingBytes", osonUpdateMaxOverflowMapSize)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	mappingBytes := int(mappingSegmentSize)
	extendedBytes := int(extendedTreeSize)
	if flags&^osonFlagUpdateOverflowSegmentUB2Mask != 0 || reserved != 0 {
		details := fmt.Sprintf("update header flags 0x%04x contain unsupported bits 0x%04x or reserved field is 0x%08x", flags, flags&^osonFlagUpdateOverflowSegmentUB2Mask, reserved)
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "reason", details, "flags", flags, "reserved", reserved)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	mappingOffset := buf.position()
	if err := buf.ensureRange(mappingOffset, mappingBytes, "osonHeader.readOptionalUpdateHeader"); err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "mappingOffset", mappingOffset, "mappingBytes", mappingBytes)
		return err
	}

	h.updateHeaderFlags = flags
	// The overflow mapping segment is padded independently from the actual number
	// of entries. The extended tree therefore starts after the full declared
	// mapping-segment byte size, not immediately after the last parsed entry.
	h.extendedTreeSegmentStartOffset = mappingOffset + mappingBytes
	if err := buf.ensureRange(h.extendedTreeSegmentStartOffset, extendedBytes, "osonHeader.readOptionalUpdateHeader"); err != nil {
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "extendedTreeOffset", h.extendedTreeSegmentStartOffset, "extendedTreeBytes", extendedBytes)
		return err
	}
	if h.extendedTreeSegmentStartOffset+extendedBytes != buf.size() {
		details := fmt.Sprintf("extended tree ends at byte %d but OSON document size is %d", h.extendedTreeSegmentStartOffset+extendedBytes, buf.size())
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "reason", details)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}
	h.extendedTreeSegmentByteLength = uint(extendedTreeSize)

	entrySize := osonUpdateMappingEntrySizeUB4
	if h.isSetUpdate(osonFlagUpdateOverflowSegmentUB2Mask) {
		// Compact update headers use UB2 pairs: [primary-tree-relative][extended-tree-relative].
		entrySize = osonUpdateMappingEntrySizeUB2
	}
	if int(numMappings) > 0 && int(numMappings) > mappingBytes/entrySize {
		details := fmt.Sprintf("update header mapping count %d exceeds mapping segment capacity %d entries", numMappings, mappingBytes/entrySize)
		common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "reason", details, "mappingBytes", mappingBytes, "entryWidth", entrySize)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}
	h.forwardingAddresses = make(map[int]int, int(numMappings))
	for i := 0; i < int(numMappings); i++ {
		if h.isSetUpdate(osonFlagUpdateOverflowSegmentUB2Mask) {
			from, err := buf.readUB2()
			if err != nil {
				common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB2Size)
				return err
			}
			to, err := buf.readUB2()
			if err != nil {
				common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB2Size)
				return err
			}
			if err := h.addForwardingAddress(int(from), int(to), extendedBytes); err != nil {
				return err
			}
			continue
		}

		// Wider update headers store the same mapping in UB4 form so large trees
		// can still redirect nodes into the extended tree segment.
		from, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB4Size)
			return err
		}
		to, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB4Size)
			return err
		}

		if err := h.addForwardingAddress(int(from), int(to), extendedBytes); err != nil {
			return err
		}
	}

	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("osonHeader.readOptionalUpdateHeader: parsed",
			"flags", h.updateHeaderFlags,
			"mappingCount", numMappings,
			"mappingBytes", mappingBytes,
			"extendedTreeOffset", h.extendedTreeSegmentStartOffset,
			"extendedTreeBytes", extendedBytes)
	}
	return nil
}

// addForwardingAddress validates one update-map entry before retaining it.
func (h *osonHeader) addForwardingAddress(from, to, extendedTreeSize int) error {
	if from < 0 || from >= int(h.treeSegmentByteLength) {
		details := fmt.Sprintf("update mapping source offset %d is outside primary tree range [0,%d)", from, h.treeSegmentByteLength)
		common.Odl.Debug("osonHeader.addForwardingAddress: failed", "reason", details, "from", from)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}
	if to < 0 || to >= extendedTreeSize {
		details := fmt.Sprintf("update mapping target offset %d is outside extended tree range [0,%d)", to, extendedTreeSize)
		common.Odl.Debug("osonHeader.addForwardingAddress: failed", "reason", details, "to", to)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}
	if _, exists := h.forwardingAddresses[from]; exists {
		details := fmt.Sprintf("duplicate update mapping source offset %d", from)
		common.Odl.Debug("osonHeader.addForwardingAddress: failed", "reason", details, "from", from)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}
	h.forwardingAddresses[from] = to
	return nil
}

// readHeader decodes only the fixed-width and flag-controlled header prefix.
//
// Parameters:
//   - buf: buffer positioned at the OSON magic and version word.
//
// Returns:
//   - Layout metadata used by dictionary readers.
//
// Errors:
//   - Returns an internal error for truncated, malformed, or unsupported headers.
func (h *osonHeader) readHeader(buf *osonBuffer) (_parsedDictionaryLayout, error) {
	layout := _parsedDictionaryLayout{}

	if buf.remaining() < osonHeaderMinSize {
		details := fmt.Sprintf("truncated OSON header, expected at least %d bytes, have %d", osonHeaderMinSize, buf.remaining())
		common.Odl.Debug("osonHeader.readHeader: failed", "reason", details, "remainingBytes", buf.remaining())
		return layout, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// Read and validate the fixed OSON magic/version word.
	magicAndVersion, err := buf.readUB4()
	if err != nil {
		common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
		return layout, err
	}
	mv := uint32(magicAndVersion)
	if mv&osonMagicPrefixMask != osonMagicPrefix {
		details := fmt.Sprintf("invalid OSON magic 0x%08x, expected 0x%08x", mv&osonMagicPrefixMask, osonMagicPrefix)
		common.Odl.Debug("osonHeader.readHeader: failed", "reason", details, "magic", mv&osonMagicPrefixMask)
		return layout, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	h.formatVersion = drvCommon.UB1(mv & osonVersionByteMask)

	// The current reader accepts only the supported on-wire version range.
	if h.formatVersion < osonFormatMinVersion || h.formatVersion > osonFormatMaxVersion {
		details := fmt.Sprintf("unsupported OSON version %d", h.formatVersion)
		common.Odl.Debug("osonHeader.readHeader: failed", "reason", details, "version", h.formatVersion)
		return layout, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	if h.flags, err = buf.readUB2(); err != nil {
		common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
		return layout, err
	}
	if h.flags&osonFlagReservedMask != 0 {
		details := fmt.Sprintf("root-header flags 0x%04x set reserved bits 0x%04x", h.flags, h.flags&osonFlagReservedMask)
		common.Odl.Debug("osonHeader.readHeader: failed", "reason", details, "flags", h.flags)
		return layout, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// scalar documents do not carry dictionaries or tiny-node statistics.
	if h.isScalar() {
		// For scalars, the fixed header ends immediately before the tree payload.
		size, err := h.readTreeSegmentSize(buf)
		if err != nil {
			return layout, err
		}
		h.treeSegmentByteLength = size
		if err := buf.ensureAvailable(int(size)); err != nil {
			return layout, err
		}
		return layout, nil
	}

	switch {
	case h.isSet(osonFlagDistinctFieldCountUB4Mask):
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.primaryCount = uint(val)
	case h.isSet(osonFlagDistinctFieldCountUB2Mask):
		val, err := buf.readUB2()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.primaryCount = uint(val)
	default:
		val, err := buf.readUB1()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.primaryCount = uint(val)
	}

	// The primary dictionary heap stores the actual field-name bytes. The flag
	// decides whether its total size is encoded in 2 or 4 bytes.
	if h.isSet(osonFlagFieldHeapSizeUB4Mask) {
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.primaryHeapSize = uint(val)
	} else {
		val, err := buf.readUB2()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.primaryHeapSize = uint(val)
	}

	// OSON v3+ can carry a secondary dictionary for keys longer than 255 bytes.
	// Its counts and heap sizes are always encoded as UB4 values.
	if h.formatVersion >= 3 {
		// The secondary flag word controls only the long-key dictionary layout.
		if h.secondaryFlags, err = buf.readUB2(); err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		if h.secondaryFlags&osonFlagSecondaryReservedMask != 0 {
			details := fmt.Sprintf("secondary-header flags 0x%04x set reserved bits 0x%04x", h.secondaryFlags, h.secondaryFlags&osonFlagSecondaryReservedMask)
			common.Odl.Debug("osonHeader.readHeader: failed", "reason", details, "flags", h.secondaryFlags)
			return layout, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.secondaryCount = uint(val)
		val, err = buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
			return layout, err
		}
		layout.secondaryHeapSize = uint(val)
	}

	// The tree segment follows the header (for scalars) or the dictionaries (for
	// containers). Its encoded size is always present in the fixed header.
	size, err := h.readTreeSegmentSize(buf)
	if err != nil {
		return layout, err
	}
	h.treeSegmentByteLength = size

	// Non-scalar documents store a tiny-node statistic after the tree size.
	if h.tinyNodeStatCount, err = buf.readUB2(); err != nil {
		common.Odl.Debug("osonHeader.readHeader: failed", "error", err)
		return layout, err
	}

	// Validate the entire declared layout before any dictionary reader allocates.
	primaryOffsetSize := osonUB2Size
	if h.isSet(osonFlagFieldHeapSizeUB4Mask) {
		primaryOffsetSize = osonUB4Size
	}
	secondaryOffsetSize := osonUB4Size
	if h.secondaryFlags&osonFlagSecondaryFieldOffsetsUB2Mask != 0 {
		secondaryOffsetSize = osonUB2Size
	}
	requiredBytes := uint64(layout.primaryCount)*uint64(osonPrimaryDictHashIDSizeUB1+primaryOffsetSize) +
		uint64(layout.primaryHeapSize) +
		uint64(layout.secondaryCount)*uint64(osonSecondaryDictHashIDSizeUB2+secondaryOffsetSize) +
		uint64(layout.secondaryHeapSize) + uint64(h.treeSegmentByteLength)
	if requiredBytes > uint64(buf.remaining()) {
		common.Odl.Debug("osonHeader.readHeader: failed", "reason", "declared layout exceeds available bytes",
			"requiredBytes", requiredBytes, "remainingBytes", buf.remaining(),
			"primaryCount", layout.primaryCount, "secondaryCount", layout.secondaryCount,
			"primaryHeapSize", layout.primaryHeapSize, "secondaryHeapSize", layout.secondaryHeapSize,
			"treeSegmentSize", h.treeSegmentByteLength)
		return layout, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("osonHeader.readHeader: parsed",
			"version", h.formatVersion,
			"flags", h.flags,
			"flags2", h.secondaryFlags,
			"uniqueFields", layout.primaryCount,
			"uniqueFields2", layout.secondaryCount,
			"fieldHeapSize", layout.primaryHeapSize,
			"fieldHeapSize2", layout.secondaryHeapSize,
			"treeSegmentSize", h.treeSegmentByteLength,
			"tinyNodeCount", h.tinyNodeStatCount)
	}

	return layout, nil
}

// readTreeSegmentSize reads the declared tree byte length using the width
// selected by the receiver's header flags.
//
// Errors:
//   - Returns an internal error if the selected UB2 or UB4 size field is truncated.
func (h *osonHeader) readTreeSegmentSize(buf *osonBuffer) (uint, error) {
	// Tree size width is selected by a primary header flag.
	if h.isSet(osonFlagTreeSegmentSizeUB4Mask) {
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readTreeSegmentSize: failed", "error", err, "width", osonUB4Size)
			return 0, err
		}
		return uint(val), nil
	}
	val, err := buf.readUB2()
	if err != nil {
		common.Odl.Debug("osonHeader.readTreeSegmentSize: failed", "error", err, "width", osonUB2Size)
		return 0, err
	}
	return uint(val), nil
}

// readPrimaryDictionary decodes the short-key dictionary.
//
// Layout:
//  1. Sorted hash array
//  2. Heap-offset array
//  3. Heap bytes containing 1-byte-length-prefixed UTF-8 names
//
// Parameters:
//   - buf: buffer positioned at the primary dictionary hash array.
//   - layout: counts and heap sizes read from the fixed header; primaryCount is positive.
//
// Errors:
//   - Returns an internal error for truncated arrays, invalid offsets, or malformed heap entries.
func (h *osonHeader) readPrimaryDictionary(buf *osonBuffer, layout _parsedDictionaryLayout) error {
	count := int(layout.primaryCount)

	hashIDs, err := h.readPrimaryHashes(buf, count)
	if err != nil {
		return err
	}

	offsets, err := h.readPrimaryOffsets(buf, count)
	if err != nil {
		return err
	}

	// The heap contains length-prefixed UTF-8 names packed back-to-back.
	heapSize := int(layout.primaryHeapSize)
	heap, err := buf.readSlice(heapSize)
	if err != nil {
		common.Odl.Debug("osonHeader.readPrimaryDictionary: failed", "error", err, "heapBytes", heapSize)
		return err
	}

	treeOffset := buf.position()

	// each primary-dictionary heap entry is encoded as:
	//   1 byte length
	//   N bytes UTF-8 field name
	names := make([]string, count)
	for i := 0; i < count; i++ {
		// Heap offsets are untrusted wire values. Each entry must start
		// inside the heap and keep its 1-byte length prefix addressable,
		// so the unchecked heap[offset:] below can never panic.
		offset := offsets[i]
		if offset < 0 || offset+osonUB1Size > len(heap) {
			details := fmt.Sprintf("primary dictionary entry %d has heap offset %d outside heap size %d", i, offset, len(heap))
			common.Odl.Debug("osonHeader.readPrimaryDictionary: failed", "reason", details, "entry", i, "heapOffset", offset, "heapBytes", len(heap))
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
		entry := heap[offset:]

		// Primary dictionary entries use a single-byte length prefix.
		// The declared length must also fit the remaining heap bytes;
		// entries may overlap but may not run past the heap end.
		length := int(entry[0])
		entry = entry[1:]
		if length > len(entry) {
			details := fmt.Sprintf("primary dictionary entry at offset %d declares name length %d exceeding remaining heap %d", offset, length, len(entry))
			common.Odl.Debug("osonHeader.readPrimaryDictionary: failed", "reason", details, "entry", i, "heapOffset", offset, "nameBytes", length)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
		nameBytes := entry[:length]
		if !utf8.Valid(nameBytes) {
			details := fmt.Sprintf("primary dictionary entry at offset %d contains invalid UTF-8 in its %d-byte name", offset, length)
			common.Odl.Debug("osonHeader.readPrimaryDictionary: failed", "reason", details, "entry", i, "heapOffset", offset)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
		names[i] = string(nameBytes)
	}

	h.fieldDictionary.hashIDs = append(h.fieldDictionary.hashIDs, hashIDs...)
	h.fieldDictionary.fieldNames = append(h.fieldDictionary.fieldNames, names...)
	h.primaryFieldsCount = len(h.fieldDictionary.fieldNames)
	// Once the heap is consumed, the next unread byte is the start of the tree.
	h.treeSegmentStartOffset = treeOffset
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("osonHeader.readPrimaryDictionary: completed", "count", count, "heapSize", heapSize)
	}
	return nil
}

// readSecondaryDictionary decodes the long-key dictionary used by OSON v3+.
//
// Layout:
//  1. Sorted 2-byte hash array
//  2. Heap-offset array (2 or 4 bytes per entry depending on flags)
//  3. Heap bytes containing 2-byte-length-prefixed UTF-8 names
//
// Parameters:
//   - buf: buffer positioned at the secondary dictionary hash array.
//   - layout: metadata from a v3+ header with a positive secondaryCount.
//
// Errors:
//   - Returns an internal error for truncated arrays,
//     invalid offsets, or malformed long-key heap entries.
func (h *osonHeader) readSecondaryDictionary(buf *osonBuffer, layout _parsedDictionaryLayout) error {
	count := int(layout.secondaryCount)

	hashIDs, err := h.readSecondaryHashes(buf, count)
	if err != nil {
		return err
	}

	offsets, err := h.readSecondaryOffsets(buf, count)
	if err != nil {
		return err
	}

	// The secondary heap is also packed, but each name uses a UB2 length prefix.
	heapSize := int(layout.secondaryHeapSize)
	heap, err := buf.readSlice(heapSize)
	if err != nil {
		common.Odl.Debug("osonHeader.readSecondaryDictionary: failed", "error", err, "heapBytes", heapSize)
		return err
	}

	treeOffset := buf.position()

	// each secondary-dictionary heap entry is encoded as:
	//   2 byte big-endian length
	//   N bytes UTF-8 field name
	names := make([]string, count)
	for i := 0; i < count; i++ {
		// Heap offsets are untrusted wire values. Each entry must start
		// inside the heap and keep its 2-byte length prefix addressable,
		// so the unchecked heap[offset:] below can never panic.
		offset := offsets[i]
		if offset < 0 || offset+osonUB2Size > len(heap) {
			details := fmt.Sprintf("secondary dictionary entry %d has heap offset %d outside heap size %d", i, offset, len(heap))
			common.Odl.Debug("osonHeader.readSecondaryDictionary: failed", "reason", details, "entry", i, "heapOffset", offset, "heapBytes", len(heap))
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
		entry := heap[offset:]
		// Secondary dictionary entries use a big-endian 2-byte length prefix.
		// The declared length must also fit the remaining heap bytes.
		length := int(binary.BigEndian.Uint16(entry[:osonUB2Size]))
		entry = entry[osonUB2Size:]
		if length > len(entry) {
			details := fmt.Sprintf("secondary dictionary entry at offset %d declares name length %d exceeding remaining heap %d", offset, length, len(entry))
			common.Odl.Debug("osonHeader.readSecondaryDictionary: failed", "reason", details, "entry", i, "heapOffset", offset, "nameBytes", length)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
		nameBytes := entry[:length]
		if !utf8.Valid(nameBytes) {
			details := fmt.Sprintf("secondary dictionary entry at offset %d contains invalid UTF-8 in its %d-byte name", offset, length)
			common.Odl.Debug("osonHeader.readSecondaryDictionary: failed", "reason", details, "entry", i, "heapOffset", offset)
			return common.NewOracleError(oracleErrors.InternalError, nil)
		}
		names[i] = string(nameBytes)
	}

	h.fieldDictionary.hashIDs = append(h.fieldDictionary.hashIDs, hashIDs...)
	h.fieldDictionary.fieldNames = append(h.fieldDictionary.fieldNames, names...)
	// If the secondary dictionary is present it is always the last dictionary
	// before the tree segment.
	h.treeSegmentStartOffset = treeOffset
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("osonHeader.readSecondaryDictionary: completed", "count", count, "heapBytes", heapSize)
	}
	return nil
}

// readPrimaryHashes reads count sorted UB1 hashes for the short-key dictionary.
//
// Returns:
//   - Compact on-wire hash values widened to uint32 for uniform lookup.
//
// Errors:
//   - Returns an internal error if the hash array is truncated.
func (h *osonHeader) readPrimaryHashes(buf *osonBuffer, count int) ([]uint32, error) {
	hashIDs := make([]uint32, count)
	for i := range count {
		// Preserve the compact on-wire UB1 values as uint32 for uniform searches.
		val, err := buf.readUB1()
		if err != nil {
			common.Odl.Debug("osonHeader.readPrimaryHashes: failed", "error", err, "index", i, "width", osonPrimaryDictHashIDSizeUB1)
			return nil, err
		}
		hashIDs[i] = uint32(val)
	}
	return hashIDs, nil
}

// readPrimaryOffsets reads heap offsets for the short-key dictionary entries.
// Offsets are relative to the start of the primary dictionary heap.
//
// Errors:
//   - Returns an internal error if the flag-selected UB2 or UB4 offset array is truncated.
func (h *osonHeader) readPrimaryOffsets(buf *osonBuffer, count int) ([]int, error) {
	offsets := make([]int, count)
	for i := range count {
		// Primary dictionary offsets widen with the declared heap-size width.
		if h.isSet(osonFlagFieldHeapSizeUB4Mask) {
			val, err := buf.readUB4()
			if err != nil {
				common.Odl.Debug("osonHeader.readPrimaryOffsets: failed", "error", err, "index", i, "width", osonUB4Size)
				return nil, err
			}

			offsets[i] = int(val)
		} else {
			// Small heaps keep offsets compact as UB2 entries.
			val, err := buf.readUB2()
			if err != nil {
				common.Odl.Debug("osonHeader.readPrimaryOffsets: failed", "error", err, "index", i, "width", osonUB2Size)
				return nil, err
			}
			offsets[i] = int(val)
		}
	}
	return offsets, nil
}

// readSecondaryHashes reads count sorted UB2 hashes for the long-key dictionary.
//
// Returns:
//   - Compact on-wire hash values widened to uint32 for uniform lookup.
//
// Errors:
//   - Returns an internal error if the hash array is truncated.
func (h *osonHeader) readSecondaryHashes(buf *osonBuffer, count int) ([]uint32, error) {
	hashIDs := make([]uint32, count)
	for i := range count {
		// Preserve the compact on-wire UB2 values as uint32 for uniform searches.
		val, err := buf.readUB2()
		if err != nil {
			common.Odl.Debug("osonHeader.readSecondaryHashes: failed", "error", err, "index", i, "width", osonSecondaryDictHashIDSizeUB2)
			return nil, err
		}
		hashIDs[i] = uint32(val)
	}
	return hashIDs, nil
}

// readSecondaryOffsets reads heap offsets for long-key dictionary entries.
// The v3+ secondary flags choose whether each offset is UB2 or UB4.
//
// Errors:
//   - Returns an internal error if the flag-selected offset array is truncated.
func (h *osonHeader) readSecondaryOffsets(buf *osonBuffer, count int) ([]int, error) {
	offsets := make([]int, count)
	if h.secondaryFlags&osonFlagSecondaryFieldOffsetsUB2Mask != 0 {
		// Small long-key heaps can keep the offset array compact as UB2 entries.
		for i := 0; i < count; i++ {
			val, err := buf.readUB2()
			if err != nil {
				common.Odl.Debug("osonHeader.readSecondaryOffsets: failed", "error", err, "index", i, "width", osonUB2Size)
				return nil, err
			}
			offsets[i] = int(val)
		}
		return offsets, nil
	}
	// Otherwise offsets are stored in full UB4 width.
	for i := 0; i < count; i++ {
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Debug("osonHeader.readSecondaryOffsets: failed", "error", err, "index", i, "width", osonUB4Size)
			return nil, err
		}
		offsets[i] = int(val)
	}
	return offsets, nil
}

// isScalar reports whether the OSON document is a scalar value.
func (h *osonHeader) isScalar() bool {
	return h.isSet(osonFlagScalarDocumentMask)
}

// relativeOffsets reports whether container child-offset arrays are encoded as
// signed deltas from the containing node's tree-relative position.
func (h *osonHeader) relativeOffsets() bool {
	return h.isSet(osonFlagRelativeOffsetsMask)
}

// numFieldIDBytes returns the size used by encoded object field-ID entries.
//
// Returns:
//   - The field-ID width in bytes: UB1, UB2, or UB4.
func (h *osonHeader) numFieldIDBytes() int {
	switch {
	case h.isSet(osonFlagDistinctFieldCountUB4Mask):
		return osonUB4Size
	case h.isSet(osonFlagDistinctFieldCountUB2Mask):
		return osonUB2Size
	default:
		return osonUB1Size
	}
}

// isSet reports whether a primary-header flag bit is enabled.
func (h *osonHeader) isSet(flag drvCommon.UB2) bool {
	return h.flags&flag != 0
}

// isSetUpdate reports whether an update-header flag bit is enabled.
func (h *osonHeader) isSetUpdate(flag drvCommon.UB2) bool {
	return h.updateHeaderFlags&flag != 0
}

// version returns the encoded OSON version.
func (h *osonHeader) version() drvCommon.UB1 {
	return h.formatVersion
}

// fieldName returns a field name by zero-based dictionary index.
//
// Parameters:
//   - fid: zero-based index into the merged primary-then-secondary dictionary.
//
// Returns:
//   - The matching name and true, or an empty string and false when fid is invalid.
func (h *osonHeader) fieldName(fid int) (string, bool) {
	return h.fieldDictionary.fieldNameAt(fid)
}

// treeSegmentOffset returns the absolute document offset of the primary tree segment.
//
// Returns:
//   - Zero-based byte offset from the start of the OSON document.
func (h *osonHeader) treeSegmentOffset() int {
	return h.treeSegmentStartOffset
}

// segmentOffsetForNode returns the tree-segment base that should be used when
// interpreting container-local offsets for the node at absoluteOffset.
//
// Primary-tree nodes encode child references relative to the primary tree
// segment. Nodes redirected into the extended tree use the extended tree as
// their local address space instead.
//
// Parameters:
//   - absoluteOffset: zero-based byte offset of the node in the OSON document.
//
// Returns:
//   - Absolute start offset of the node's containing tree segment.
func (h *osonHeader) segmentOffsetForNode(absoluteOffset int) int {
	if h.extendedTreeSegmentStartOffset != 0 && absoluteOffset >= h.extendedTreeSegmentStartOffset {
		return h.extendedTreeSegmentStartOffset
	}
	return h.treeSegmentStartOffset
}

// containsNodeOffset rejects forwarding targets outside the declared tree
// segments before node parsing reads an opcode from them.
func (h *osonHeader) containsNodeOffset(absoluteOffset int) bool {
	primaryEnd := h.treeSegmentStartOffset + int(h.treeSegmentByteLength)
	if absoluteOffset >= h.treeSegmentStartOffset && absoluteOffset < primaryEnd {
		return true
	}
	if h.extendedTreeSegmentStartOffset == 0 {
		return false
	}
	extendedEnd := h.extendedTreeSegmentStartOffset + int(h.extendedTreeSegmentByteLength)
	return absoluteOffset >= h.extendedTreeSegmentStartOffset && absoluteOffset < extendedEnd
}

// resolveForwardedOffset maps one extended-tree-relative offset to an absolute document offset.
//
// Parameters:
//   - relativeOffset: byte offset relative to the extended tree segment start.
//
// Returns:
//   - Absolute document offset of the forwarded node.
//
// Errors:
//   - Returns an internal error when the document has no extended tree segment.
func (h *osonHeader) resolveForwardedOffset(relativeOffset int) (int, error) {
	if h.extendedTreeSegmentStartOffset == 0 || relativeOffset < 0 || relativeOffset >= int(h.extendedTreeSegmentByteLength) {
		details := fmt.Sprintf("forwarded offset %d outside tree", relativeOffset)
		common.Odl.Debug("osonHeader.resolveForwardedOffset: failed", "reason", details, "relativeOffset", relativeOffset, "extendedTreeBytes", h.extendedTreeSegmentByteLength)
		return 0, common.NewOracleError(oracleErrors.InternalError, nil)
	}
	return h.extendedTreeSegmentStartOffset + relativeOffset, nil
}

// resolveOverflowOffset maps one primary-tree absolute node offset through the
// overflow-address mapping into the extended tree segment.
//
// Parameters:
//   - absoluteOffset: byte offset of an overflow node in the primary tree segment.
//
// Returns:
//   - Absolute document offset of the mapped node in the extended tree segment.
//
// Errors:
//   - Returns an internal error when no mapping exists or the document has
//     no extended tree segment.
func (h *osonHeader) resolveOverflowOffset(absoluteOffset int) (int, error) {
	// Overflow mappings are keyed by the original node's tree-relative position in
	// the primary tree segment, not by absolute document offset.
	relativeOffset := absoluteOffset - h.treeSegmentStartOffset
	forwarded, ok := h.forwardingAddresses[relativeOffset]
	if !ok {
		details := fmt.Sprintf("no overflow mapping for offset %d", relativeOffset)
		common.Odl.Debug("osonHeader.resolveOverflowOffset: failed", "reason", details, "absoluteOffset", absoluteOffset, "relativeOffset", relativeOffset)
		return 0, common.NewOracleError(oracleErrors.InternalError, nil)
	}
	return h.resolveForwardedOffset(forwarded)
}

// osonHash computes the full 32-bit FNV-derived field-name hash.
//
// Parameters:
//   - key: UTF-8 field name to hash.
//
// Returns:
//   - Full hash before tier compaction and the key's UTF-8 byte length.
func osonHash(key string) (uint32, int) {
	// Hash the UTF-8 bytes directly so field lookup matches the on-wire format.
	hash := uint32(osonFNVOffsetBasis32)
	bytes := []byte(key)
	for i := 0; i < len(bytes); i++ {
		hash = (hash ^ uint32(bytes[i])) * osonFNVPrime32
	}
	return hash, len(bytes)
}

// compactPrimaryHash truncates a full field-name hash to a primary-tier width.
//
// Parameters:
//   - hash: full 32-bit OSON field-name hash.
//   - width: requested compact width in bytes.
//
// Returns:
//   - Low width bytes of hash for UB1 or UB2; otherwise the original hash.
func compactPrimaryHash(hash uint32, size int) uint32 {
	switch size {
	case osonUB1Size:
		return hash & 0xff
	case osonUB2Size:
		return hash & 0xffff
	default:
		return hash
	}
}

// compactSecondaryHash produces the secondary-tier UB2 hash layout used on the wire.
//
// Parameters:
//   - hash: full 32-bit OSON field-name hash.
//
// Returns:
//   - Low two bytes in the byte-swapped order required by the secondary dictionary.
func compactSecondaryHash(hash uint32) uint32 {
	return ((hash & 0xff) << 8) | ((hash & 0xff00) >> 8)
}
