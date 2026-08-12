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
	"slices"

	"github.com/oracle/go-driver/driver/common"
)

// _parsedDictionaryLayout holds the dictionary counts and heap sizes needed to decode the wire format.
type _parsedDictionaryLayout struct {
	// primaryCount is the number of primary-tier field names.
	primaryCount int
	// primaryHeapSize is the byte size of the primary dictionary heap.
	primaryHeapSize int
	// secondaryCount is the number of secondary-tier field names.
	secondaryCount int
	// secondaryHeapSize is the byte size of the secondary dictionary heap.
	secondaryHeapSize int
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

// allFieldNames returns a copy of the ordered field-name list.
//
// Output:
//   - A copy that callers may modify without changing the dictionary.
func (d dictionary) allFieldNames() []string {
	// Return a defensive copy so callers cannot mutate header state.
	names := make([]string, len(d.fieldNames))
	copy(names, d.fieldNames)
	return names
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

// fieldIDForKey resolves a key to its 1-based field ID from the merged dictionary.
//
// Input:
//   - key: UTF-8 field name to locate.
//   - primaryCount: number of leading entries in the primary dictionary tier.
//
// Output:
//   - The 1-based field ID, or -1 if key is absent or primaryCount is inconsistent.
func (d dictionary) fieldIDForKey(key string, primaryCount int) int {
	fullHash, utfLen := osonHash(key)

	// reject inconsistent dictionary metadata
	if primaryCount < 0 || primaryCount > len(d.fieldNames) {
		return -1
	}

	// field-names inside primary dict use compact UB1 hashes
	if utfLen <= osonMaxPrimaryDictKeyLength && primaryCount > 0 {
		hash := compactPrimaryHash(fullHash, osonPrimaryDictHashIDSizeUB1)
		return locateFieldId(hash, key, dictionary{
			hashIDs:    d.hashIDs[:primaryCount],
			fieldNames: d.fieldNames[:primaryCount],
		})
	}

	// field-names inside secondary dict use compact UB2 hashes
	if utfLen <= osonMaxSecondaryDictKeyLength && primaryCount < len(d.fieldNames) {
		hash := compactSecondaryHash(fullHash)
		if idx := locateFieldId(hash, key, dictionary{
			hashIDs:    d.hashIDs[primaryCount:],
			fieldNames: d.fieldNames[primaryCount:],
		}); idx > 0 {
			return primaryCount + idx
		}
	}

	return -1
}

// osonHeader stores the parsed metadata needed to read one OSON document.
//
// The header describes the document in three parts:
//  1. Fixed header fields such as version, flags, and segment sizes.
//  2. The field-name dictionary used to resolve object member IDs.
//  3. The encoded tree segment that holds the document values.
type osonHeader struct {
	// formatVersion is the low byte of the OSON magic/version word.
	formatVersion common.UB1

	// flags are the main OSON header flags that control layout choices.
	flags common.UB2

	// secondaryFlags are the optional v3+ extension flags.
	secondaryFlags common.UB2

	// fieldDictionary stores all decoded field names in wire order.
	fieldDictionary dictionary

	// primaryFieldsCount is the number of primary-dictionary entries.
	primaryFieldsCount int

	// treeSegmentByteLength is the declared size of the encoded tree segment.
	treeSegmentByteLength common.UB4

	// treeSegmentStartOffset is the absolute offset where the tree segment begins.
	treeSegmentStartOffset int

	// tinyNodeStatCount is the tiny-node count recorded for non-scalar documents.
	tinyNodeStatCount common.UB2

	// updateHeaderFlags are the optional update-header flags.
	updateHeaderFlags common.UB2

	// extendedTreeSegmentStartOffset is the absolute offset of the extended tree segment.
	extendedTreeSegmentStartOffset int

	// forwardingAddresses maps tree-relative offsets from the base tree to the extended tree.
	forwardingAddresses map[int]int
}

// newOsonHeader parses the metadata at the start of an OSON document.
//
// Input:
//   - buffer: document buffer positioned at the OSON fixed header.
//
// Output:
//   - A populated header and buffer positioned at the primary tree segment.
//
// Errors:
//   - Returns common.OsonHeaderError when the document is truncated, malformed,
//     unsupported, or contains an invalid dictionary or tree segment.
func newOsonHeader(buffer *osonBuffer) (*osonHeader, error) {
	common.Odl.Debug("newOsonHeader: begin")

	header := &osonHeader{}
	if err := header.initialize(buffer); err != nil {
		return nil, err
	}
	common.Odl.Debug("newOsonHeader: parsed", "version", header.formatVersion,
		"uniqueFields", header.primaryFieldsCount, "uniqueFields2", len(header.fieldDictionary.fieldNames)-header.primaryFieldsCount,
		"treeSegmentOffset", header.treeSegmentStartOffset, "treeSegmentSize", header.treeSegmentByteLength)
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
//   - Returns common.OsonHeaderError for a nil buffer or invalid OSON header,
//     dictionary, tree segment, or optional update-header tail.
func (h *osonHeader) initialize(buf *osonBuffer) error {
	// A nil buffer is a programming error; fail before any parsing.
	if buf == nil {
		cause := fmt.Errorf("header initialization requires a buffer")
		common.Odl.Error("osonHeader.initialize: failed", "error", cause)
		return common.NewOracleError(common.OsonHeaderError, cause)
	}

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
			cause := fmt.Errorf("unsupported OSON header: inline-leaf flag must always be on for non-scalar documents")
			common.Odl.Error("osonHeader.initialize: failed", "error", cause, "flags", h.flags)
			return common.NewOracleError(common.OsonHeaderError, nil)
		}
	}

	if err := buf.ensureRange(h.treeSegmentStartOffset, int(h.treeSegmentByteLength), "osonHeader.initialize"); err != nil {
		common.Odl.Error("osonHeader.initialize: failed",
			"error", err,
			"treeSegmentOffset", h.treeSegmentStartOffset,
			"treeSegmentSize", h.treeSegmentByteLength,
			"documentSize", buf.size())
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	// some OSON documents carry an update header, overflow-address mapping, and an
	// extended tree segment after the primary tree segment.
	if err := h.readOptionalUpdateHeader(buf); err != nil {
		return err
	}

	// consumers expect to start decoding from the main tree segment
	if err := buf.setPosition(h.treeSegmentStartOffset); err != nil {
		common.Odl.Error("osonHeader.initialize: failed", "error", err, "treeSegmentOffset", h.treeSegmentStartOffset)
		return common.NewOracleError(common.OsonHeaderError, err)
	}
	common.Odl.Debug("osonHeader.initialize: completed",
		"treeSegmentOffset", h.treeSegmentStartOffset,
		"treeSegmentSize", h.treeSegmentByteLength,
		"tinyNodeCount", h.tinyNodeStatCount)
	return nil
}

// readOptionalUpdateHeader parses the update-header tail when bytes remain after
// the primary tree segment.
//
// Input:
//   - buf: buffer containing the primary tree followed, optionally, by update metadata.
//
// Errors:
//   - Returns common.OsonHeaderError when an update header, mapping segment, or
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

	if err := buf.setPosition(updateHeaderOffset); err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "updateHeaderOffset", updateHeaderOffset)
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	flags, err := buf.readUB2()
	if err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "step", "update-flags")
		return common.NewOracleError(common.OsonHeaderError, err)
	}
	numMappings, err := buf.readUB2()
	if err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "step", "mapping-count")
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	// reserved
	if _, err := buf.readUB4(); err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "step", "reserved")
		return common.NewOracleError(common.OsonHeaderError, err)
	}
	mappingSegmentSize, err := buf.readUB4()
	if err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "step", "mapping-segment-size")
		return common.NewOracleError(common.OsonHeaderError, err)
	}
	extendedTreeSize, err := buf.readUB4()
	if err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "step", "extended-tree-size")
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	mappingBytes := int(mappingSegmentSize)
	extendedBytes := int(extendedTreeSize)

	mappingOffset := buf.position()
	if err := buf.ensureRange(mappingOffset, mappingBytes, "osonHeader.readOptionalUpdateHeader"); err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "mappingOffset", mappingOffset, "mappingBytes", mappingBytes)
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	h.updateHeaderFlags = flags
	// The overflow mapping segment is padded independently from the actual number
	// of entries. The extended tree therefore starts after the full declared
	// mapping-segment byte size, not immediately after the last parsed entry.
	h.extendedTreeSegmentStartOffset = mappingOffset + mappingBytes
	if err := buf.ensureRange(h.extendedTreeSegmentStartOffset, extendedBytes, "osonHeader.readOptionalUpdateHeader"); err != nil {
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "extendedTreeOffset", h.extendedTreeSegmentStartOffset, "extendedTreeBytes", extendedBytes)
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	h.forwardingAddresses = make(map[int]int, int(numMappings))
	entrySize := osonUpdateMappingEntrySizeUB4
	if h.isSetUpdate(osonFlagUpdateOverflowSegmentUB2Mask) {
		// Compact update headers use UB2 pairs: [primary-tree-relative][extended-tree-relative].
		entrySize = osonUpdateMappingEntrySizeUB2
	}
	if int(numMappings) > 0 && int(numMappings) > mappingBytes/entrySize {
		cause := fmt.Errorf("update header mapping count %d exceeds mapping segment capacity %d", numMappings, mappingBytes)
		common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", cause, "mappingBytes", mappingBytes, "entryWidth", entrySize)
		return common.NewOracleError(common.OsonHeaderError, cause)
	}
	for i := 0; i < int(numMappings); i++ {
		if h.isSetUpdate(osonFlagUpdateOverflowSegmentUB2Mask) {
			from, err := buf.readUB2()
			if err != nil {
				common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB2Size)
				return common.NewOracleError(common.OsonHeaderError, err)
			}
			to, err := buf.readUB2()
			if err != nil {
				common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB2Size)
				return common.NewOracleError(common.OsonHeaderError, err)
			}
			h.forwardingAddresses[int(from)] = int(to)
			continue
		}

		// Wider update headers store the same mapping in UB4 form so large trees
		// can still redirect nodes into the extended tree segment.
		from, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB4Size)
			return common.NewOracleError(common.OsonHeaderError, err)
		}
		to, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readOptionalUpdateHeader: failed", "error", err, "index", i, "width", osonUB4Size)
			return common.NewOracleError(common.OsonHeaderError, err)
		}

		h.forwardingAddresses[int(from)] = int(to)
	}

	common.Odl.Debug("osonHeader.readOptionalUpdateHeader: parsed",
		"flags", h.updateHeaderFlags,
		"mappingCount", numMappings,
		"mappingBytes", mappingBytes,
		"extendedTreeOffset", h.extendedTreeSegmentStartOffset,
		"extendedTreeBytes", extendedBytes)
	return nil
}

// readHeader decodes only the fixed-width and flag-controlled header prefix.
//
// Input:
//   - buf: buffer positioned at the OSON magic and version word.
//
// Output:
//   - Layout metadata used by dictionary readers.
//
// Errors:
//   - Returns common.OsonHeaderError for truncated, malformed, or unsupported headers.
func (h *osonHeader) readHeader(buf *osonBuffer) (_parsedDictionaryLayout, error) {
	layout := _parsedDictionaryLayout{}

	if buf.remaining() < osonHeaderMinSize {
		cause := fmt.Errorf("truncated OSON header, expected at least %d bytes, have %d", osonHeaderMinSize, buf.remaining())
		common.Odl.Error("osonHeader.readHeader: failed", "error", cause, "remaining", buf.remaining())
		return layout, common.NewOracleError(common.OsonHeaderError, cause)
	}

	// Read and validate the fixed OSON magic/version word.
	magicAndVersion, err := buf.readUB4()
	if err != nil {
		common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "magic-version")
		return layout, common.NewOracleError(common.OsonHeaderError, err)
	}
	mv := uint32(magicAndVersion)
	if mv&osonMagicPrefixMask != osonMagicPrefix {
		cause := fmt.Errorf("invalid magic 0x%08x", mv&osonMagicPrefixMask)
		common.Odl.Error("osonHeader.readHeader: failed", "error", cause)
		return layout, common.NewOracleError(common.OsonHeaderError, cause)
	}

	h.formatVersion = common.UB1(mv & osonVersionByteMask)

	// The current reader accepts only the supported on-wire version range.
	if h.formatVersion < osonFormatMinVersion || h.formatVersion > osonFormatMaxVersion {
		cause := fmt.Errorf("unsupported OSON version %d", h.formatVersion)
		common.Odl.Error("osonHeader.readHeader: failed", "error", cause, "version", h.formatVersion)
		return layout, common.NewOracleError(common.OsonHeaderError, cause)
	}

	if h.flags, err = buf.readUB2(); err != nil {
		common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "flags")
		return layout, common.NewOracleError(common.OsonHeaderError, err)
	}

	// scalar documents do not carry dictionaries or tiny-node statistics.
	if h.isScalar() {
		// For scalars, the fixed header ends immediately before the tree payload.
		size, err := h.readTreeSegmentSize(buf)
		if err != nil {
			return layout, err
		}
		h.treeSegmentByteLength = size
		return layout, nil
	}

	switch {
	case h.isSet(osonFlagDistinctFieldCountUB4Mask):
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "distinct-field-count-ub4")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.primaryCount = int(val)
	case h.isSet(osonFlagDistinctFieldCountUB2Mask):
		val, err := buf.readUB2()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "distinct-field-count-ub2")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.primaryCount = int(val)
	default:
		val, err := buf.readUB1()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "distinct-field-count-ub1")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.primaryCount = int(val)
	}

	// The primary dictionary heap stores the actual field-name bytes. The flag
	// decides whether its total size is encoded in 2 or 4 bytes.
	if h.isSet(osonFlagFieldHeapSizeUB4Mask) {
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "field-heap-size-ub4")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.primaryHeapSize = int(val)
	} else {
		val, err := buf.readUB2()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "field-heap-size-ub2")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.primaryHeapSize = int(val)
	}

	// OSON v3+ can carry a secondary dictionary for keys longer than 255 bytes.
	// Its counts and heap sizes are always encoded as UB4 values.
	if h.formatVersion >= 3 {
		// The secondary flag word controls only the long-key dictionary layout.
		if h.secondaryFlags, err = buf.readUB2(); err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "flags2")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "unique-fields2")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.secondaryCount = int(val)
		val, err = buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "field-heap-size2")
			return layout, common.NewOracleError(common.OsonHeaderError, nil)
		}
		layout.secondaryHeapSize = int(val)
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
		common.Odl.Error("osonHeader.readHeader: failed", "error", err, "step", "tiny-node-count")
		return layout, common.NewOracleError(common.OsonHeaderError, nil)
	}

	common.Odl.Debug("osonHeader.readHeader: parsed",
		"version", h.formatVersion,
		"flags", h.flags,
		"flags2", h.secondaryFlags,
		"uniqueFields", layout.primaryCount,
		"uniqueFields2", layout.secondaryCount,
		"fieldHeapSize", layout.primaryHeapSize,
		"fieldHeapSize2", layout.secondaryHeapSize,
		"treeSegmentSize", h.treeSegmentByteLength,
		"tinyNodeCount", h.tinyNodeStatCount)

	return layout, nil
}

// readTreeSegmentSize reads the declared tree byte length using the width
// selected by the receiver's header flags.
//
// Errors:
//   - Returns common.OsonHeaderError if the selected UB2 or UB4 size field is truncated.
func (h *osonHeader) readTreeSegmentSize(buf *osonBuffer) (common.UB4, error) {
	// Tree size width is selected by a primary header flag.
	if h.isSet(osonFlagTreeSegmentSizeUB4Mask) {
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readTreeSegmentSize: failed", "error", err, "encoding", "UB4")
			return 0, common.NewOracleError(common.OsonHeaderError, nil)
		}
		return val, nil
	}
	val, err := buf.readUB2()
	if err != nil {
		common.Odl.Error("osonHeader.readTreeSegmentSize: failed", "error", err, "encoding", "UB2")
		return 0, common.NewOracleError(common.OsonHeaderError, nil)
	}
	return common.UB4(val), nil
}

// readPrimaryDictionary decodes the short-key dictionary.
//
// Layout:
//  1. Sorted hash array
//  2. Heap-offset array
//  3. Heap bytes containing 1-byte-length-prefixed UTF-8 names
//
// Input:
//   - buf: buffer positioned at the primary dictionary hash array.
//   - layout: counts and heap sizes read from the fixed header.
//
// Errors:
//   - Returns common.OsonHeaderError for truncated arrays, invalid offsets, or malformed heap entries.
func (h *osonHeader) readPrimaryDictionary(buf *osonBuffer, layout _parsedDictionaryLayout) error {
	count := layout.primaryCount
	if count == 0 {
		return nil
	}
	// A non-empty dictionary with an empty heap is structurally invalid.
	if layout.primaryHeapSize == 0 {
		cause := fmt.Errorf("primary dictionary missing heap data")
		common.Odl.Error("osonHeader.readPrimaryDictionary: failed", "error", cause)
		return common.NewOracleError(common.OsonHeaderError, cause)
	}

	hashIDs, err := h.readPrimaryHashes(buf, count)
	if err != nil {
		return err
	}

	offsets, err := h.readPrimaryOffsets(buf, count)
	if err != nil {
		return err
	}

	// The heap contains length-prefixed UTF-8 names packed back-to-back.
	heapSize := layout.primaryHeapSize
	heap, err := buf.readSlice(heapSize)
	if err != nil {
		common.Odl.Error("osonHeader.readPrimaryDictionary: failed", "error", err, "step", "read-heap", "heapSize", heapSize)
		return common.NewOracleError(common.OsonHeaderError, err)
	}

	common.Odl.Debug("osonHeader.readPrimaryDictionary: begin", "count", count, "heapSize", heapSize)
	treeOffset := buf.position()

	// each primary-dictionary heap entry is encoded as:
	//   1 byte length
	//   N bytes UTF-8 field name
	names := make([]string, count)
	for i := 0; i < count; i++ {
		offset := offsets[i]
		// Offsets are relative to the start of the heap, not the document.
		if offset < 0 || offset >= len(heap) {
			cause := fmt.Errorf("offset %d outside heap (%d)", offset, len(heap))
			common.Odl.Error("osonHeader.readPrimaryDictionary: failed", "error", cause, "offset", offset, "heapSize", len(heap))
			return common.NewOracleError(common.OsonHeaderError, cause)
		}
		entry := heap[offset:]
		if len(entry) == 0 {
			cause := fmt.Errorf("entry at %d is empty", offset)
			common.Odl.Error("osonHeader.readPrimaryDictionary: failed", "error", cause, "offset", offset)
			return common.NewOracleError(common.OsonHeaderError, cause)
		}

		// Primary dictionary entries use a single-byte length prefix.
		length := int(entry[0])
		entry = entry[1:]
		if length > len(entry) {
			cause := fmt.Errorf("entry at %d exceeds heap bounds", offset)
			common.Odl.Error("osonHeader.readPrimaryDictionary: failed", "error", cause, "offset", offset, "length", length, "remaining", len(entry))
			return common.NewOracleError(common.OsonHeaderError, cause)
		}
		names[i] = string(entry[:length])
	}

	h.fieldDictionary.hashIDs = append(h.fieldDictionary.hashIDs, hashIDs...)
	h.fieldDictionary.fieldNames = append(h.fieldDictionary.fieldNames, names...)
	h.primaryFieldsCount = len(h.fieldDictionary.fieldNames)
	// Once the heap is consumed, the next unread byte is the start of the tree.
	h.treeSegmentStartOffset = treeOffset
	common.Odl.Debug("osonHeader.readPrimaryDictionary: completed", "count", count, "heapSize", heapSize)
	return nil
}

// readSecondaryDictionary decodes the long-key dictionary used by OSON v3+.
//
// Layout:
//  1. Sorted 2-byte hash array
//  2. Heap-offset array (2 or 4 bytes per entry depending on flags)
//  3. Heap bytes containing 2-byte-length-prefixed UTF-8 names
//
// Input:
//   - buf: buffer positioned at the secondary dictionary hash array.
//   - layout: counts and heap sizes read from the fixed header.
//
// Errors:
//   - Returns common.OsonHeaderError for unsupported versions, truncated arrays,
//     invalid offsets, or malformed long-key heap entries.
func (h *osonHeader) readSecondaryDictionary(buf *osonBuffer, layout _parsedDictionaryLayout) error {
	// Long-key dictionaries are legal only in OSON v3+.
	if h.formatVersion < 3 {
		cause := fmt.Errorf("secondary dictionary present but version is %d", h.formatVersion)
		common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", cause, "version", h.formatVersion)
		return common.NewOracleError(common.OsonHeaderError, nil)
	}

	count := layout.secondaryCount
	if count == 0 {
		return nil
	}
	// A declared long-key dictionary must have heap bytes to decode names from.
	if layout.secondaryHeapSize == 0 {
		cause := fmt.Errorf("secondary dictionary missing heap data")
		common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", cause)
		return common.NewOracleError(common.OsonHeaderError, nil)
	}

	hashIDs, err := h.readSecondaryHashes(buf, count)
	if err != nil {
		return err
	}

	offsets, err := h.readSecondaryOffsets(buf, count)
	if err != nil {
		return err
	}

	// The secondary heap is also packed, but each name uses a UB2 length prefix.
	heapSize := layout.secondaryHeapSize
	heap, err := buf.readSlice(heapSize)
	if err != nil {
		common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", err, "step", "read-heap", "heapSize2", heapSize)
		return common.NewOracleError(common.OsonHeaderError, nil)
	}

	common.Odl.Debug("osonHeader.readSecondaryDictionary: begin", "count", count, "heapSize2", heapSize)
	treeOffset := buf.position()

	// each secondary-dictionary heap entry is encoded as:
	//   2 byte big-endian length
	//   N bytes UTF-8 field name
	names := make([]string, count)
	for i := 0; i < count; i++ {
		offset := offsets[i]
		// Offsets are relative to the start of the secondary heap.
		if offset < 0 || offset >= len(heap) {
			cause := fmt.Errorf("offset %d outside heap (%d)", offset, len(heap))
			common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", cause, "offset", offset, "heapSize", len(heap))
			return common.NewOracleError(common.OsonHeaderError, cause)
		}
		entry := heap[offset:]
		if len(entry) < osonUB2Size {
			cause := fmt.Errorf("entry at %d is truncated", offset)
			common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", cause, "offset", offset)
			return common.NewOracleError(common.OsonHeaderError, cause)
		}
		// Secondary dictionary entries use a big-endian 2-byte length prefix.
		length := int(binary.BigEndian.Uint16(entry[:osonUB2Size]))
		entry = entry[osonUB2Size:]

		// Secondary entries must be strictly longer than the primary-tier limit.
		if length <= osonMaxPrimaryDictKeyLength {
			cause := fmt.Errorf("secondary dictionary entry at %d has invalid length %d", offset, length)
			common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", cause, "offset", offset, "length", length)
			return common.NewOracleError(common.OsonHeaderError, cause)
		}
		if length > len(entry) {
			cause := fmt.Errorf("entry at %d exceeds heap bounds", offset)
			common.Odl.Error("osonHeader.readSecondaryDictionary: failed", "error", cause, "offset", offset, "length", length, "remaining", len(entry))
			return common.NewOracleError(common.OsonHeaderError, cause)
		}
		names[i] = string(entry[:length])
	}

	h.fieldDictionary.hashIDs = append(h.fieldDictionary.hashIDs, hashIDs...)
	h.fieldDictionary.fieldNames = append(h.fieldDictionary.fieldNames, names...)
	// If the secondary dictionary is present it is always the last dictionary
	// before the tree segment.
	h.treeSegmentStartOffset = treeOffset
	common.Odl.Debug("osonHeader.readSecondaryDictionary: completed", "count", count, "heapSize2", heapSize)
	return nil
}

// readPrimaryHashes reads count sorted UB1 hashes for the short-key dictionary.
//
// Output:
//   - Compact on-wire hash values widened to uint32 for uniform lookup.
//
// Errors:
//   - Returns common.OsonHeaderError if the hash array is truncated.
func (h *osonHeader) readPrimaryHashes(buf *osonBuffer, count int) ([]uint32, error) {
	hashIDs := make([]uint32, count)
	for i := range count {
		// Preserve the compact on-wire UB1 values as uint32 for uniform searches.
		val, err := buf.readUB1()
		if err != nil {
			common.Odl.Error("osonHeader.readPrimaryHashes: failed", "error", err, "index", i, "width", osonPrimaryDictHashIDSizeUB1)
			return nil, common.NewOracleError(common.OsonHeaderError, err)
		}
		hashIDs[i] = uint32(val)
	}
	return hashIDs, nil
}

// readPrimaryOffsets reads heap offsets for the short-key dictionary entries.
// Offsets are relative to the start of the primary dictionary heap.
//
// Errors:
//   - Returns common.OsonHeaderError if the flag-selected UB2 or UB4 offset array is truncated.
func (h *osonHeader) readPrimaryOffsets(buf *osonBuffer, count int) ([]int, error) {
	offsets := make([]int, count)
	for i := range count {
		// Primary dictionary offsets widen with the declared heap-size width.
		if h.isSet(osonFlagFieldHeapSizeUB4Mask) {
			val, err := buf.readUB4()
			if err != nil {
				common.Odl.Error("osonHeader.readPrimaryOffsets: failed", "error", err, "index", i, "width", osonUB4Size)
				return nil, common.NewOracleError(common.OsonHeaderError, err)
			}

			offsets[i] = int(val)
		} else {
			// Small heaps keep offsets compact as UB2 entries.
			val, err := buf.readUB2()
			if err != nil {
				common.Odl.Error("osonHeader.readPrimaryOffsets: failed", "error", err, "index", i, "width", osonUB2Size)
				return nil, common.NewOracleError(common.OsonHeaderError, nil)
			}
			offsets[i] = int(val)
		}
	}
	return offsets, nil
}

// readSecondaryHashes reads count sorted UB2 hashes for the long-key dictionary.
//
// Output:
//   - Compact on-wire hash values widened to uint32 for uniform lookup.
//
// Errors:
//   - Returns common.OsonHeaderError if the hash array is truncated.
func (h *osonHeader) readSecondaryHashes(buf *osonBuffer, count int) ([]uint32, error) {
	hashIDs := make([]uint32, count)
	for i := range count {
		// Preserve the compact on-wire UB2 values as uint32 for uniform searches.
		val, err := buf.readUB2()
		if err != nil {
			common.Odl.Error("osonHeader.readSecondaryHashes: failed", "error", err, "index", i, "width", osonSecondaryDictHashIDSizeUB2)
			return nil, common.NewOracleError(common.OsonHeaderError, err)
		}
		hashIDs[i] = uint32(val)
	}
	return hashIDs, nil
}

// readSecondaryOffsets reads heap offsets for long-key dictionary entries.
// The v3+ secondary flags choose whether each offset is UB2 or UB4.
//
// Errors:
//   - Returns common.OsonHeaderError if the flag-selected offset array is truncated.
func (h *osonHeader) readSecondaryOffsets(buf *osonBuffer, count int) ([]int, error) {
	offsets := make([]int, count)
	if h.secondaryFlags&osonFlagSecondaryFieldOffsetsUB2Mask != 0 {
		// Small long-key heaps can keep the offset array compact as UB2 entries.
		for i := 0; i < count; i++ {
			val, err := buf.readUB2()
			if err != nil {
				common.Odl.Error("osonHeader.readSecondaryOffsets: failed", "error", err, "index", i, "width", osonUB2Size)
				return nil, common.NewOracleError(common.OsonHeaderError, err)
			}
			offsets[i] = int(val)
		}
		return offsets, nil
	}
	// Otherwise offsets are stored in full UB4 width.
	for i := 0; i < count; i++ {
		val, err := buf.readUB4()
		if err != nil {
			common.Odl.Error("osonHeader.readSecondaryOffsets: failed", "error", err, "index", i, "width", osonUB4Size)
			return nil, common.NewOracleError(common.OsonHeaderError, err)
		}
		offsets[i] = int(val)
	}
	return offsets, nil
}

// isScalar reports whether the OSON document is a scalar value.
func (h *osonHeader) isScalar() bool {
	return h.isSet(osonFlagScalarDocumentMask)
}

// isInlineLeaf reports whether the document uses the inline leaf encoding required by the current parser.
func (h *osonHeader) isInlineLeaf() bool {
	return h.isSet(osonFlagInlineLeafMask)
}

// relativeOffsets reports whether container child-offset arrays are encoded as
// signed deltas from the containing node's tree-relative position.
func (h *osonHeader) relativeOffsets() bool {
	return h.isSet(osonFlagRelativeOffsetsMask)
}

// fieldsSorted reports whether object field IDs are globally declared sorted.
func (h *osonHeader) fieldsSorted() bool {
	return !h.isSet(osonFlagObjectFieldsUnsortedMask)
}

// numFieldIDBytes returns the size used by encoded object field-ID entries.
//
// Output:
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
func (h *osonHeader) isSet(flag common.UB2) bool {
	return h.flags&flag != 0
}

// isSetUpdate reports whether an update-header flag bit is enabled.
func (h *osonHeader) isSetUpdate(flag common.UB2) bool {
	return h.updateHeaderFlags&flag != 0
}

// version returns the encoded OSON version.
func (h *osonHeader) version() common.UB1 {
	return h.formatVersion
}

// uniqueFields returns the number of distinct field names in the dictionary.
func (h *osonHeader) uniqueFields() int {
	return len(h.fieldDictionary.fieldNames)
}

// fieldNames returns the ordered field names across both dictionaries.
//
// Output:
//   - A copy in primary-then-secondary dictionary order.
func (h *osonHeader) fieldNames() []string {
	return h.fieldDictionary.allFieldNames()
}

// fieldName returns a field name by zero-based dictionary index.
//
// Input:
//   - fid: zero-based index into the merged primary-then-secondary dictionary.
//
// Output:
//   - The matching name and true, or an empty string and false when fid is invalid.
func (h *osonHeader) fieldName(fid int) (string, bool) {
	return h.fieldDictionary.fieldNameAt(fid)
}

// fieldID resolves a field name to the 1-based field id used inside object
// member entries.
//
// The lookup first chooses the correct dictionary tier from the UTF-8 key
// length, then binary-searches the sorted hash array, and finally scans across
// any hash collisions to confirm the exact string.
//
// Input:
//   - key: UTF-8 field name to resolve.
//
// Output:
//   - The 1-based object field ID, or -1 if key is absent or unsupported by the dictionary tiers.
func (h *osonHeader) fieldID(key string) int {
	return h.fieldDictionary.fieldIDForKey(key, h.primaryFieldsCount)
}

// treeSegmentSize returns the declared primary tree-segment size.
//
// Output:
//   - Encoded primary tree length in bytes.
func (h *osonHeader) treeSegmentSize() common.UB4 {
	// This is the encoded byte length, not the current buffer remaining count.
	return h.treeSegmentByteLength
}

// treeSegmentOffset returns the absolute document offset of the primary tree segment.
//
// Output:
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
// Input:
//   - absoluteOffset: zero-based byte offset of the node in the OSON document.
//
// Output:
//   - Absolute start offset of the node's containing tree segment.
func (h *osonHeader) segmentOffsetForNode(absoluteOffset int) int {
	if h.extendedTreeSegmentStartOffset != 0 && absoluteOffset >= h.extendedTreeSegmentStartOffset {
		return h.extendedTreeSegmentStartOffset
	}
	return h.treeSegmentStartOffset
}

// tinyNodeCount returns the number of tiny nodes recorded in the header statistics.
func (h *osonHeader) tinyNodeCount() common.UB2 {
	return h.tinyNodeStatCount
}

// resolveForwardedOffset maps one extended-tree-relative offset to an absolute document offset.
//
// Input:
//   - relativeOffset: byte offset relative to the extended tree segment start.
//
// Output:
//   - Absolute document offset of the forwarded node.
//
// Errors:
//   - Returns common.OsonParsingError when the document has no extended tree segment.
func (h *osonHeader) resolveForwardedOffset(relativeOffset int) (int, error) {
	if h.extendedTreeSegmentStartOffset == 0 {
		cause := fmt.Errorf("forwarded node requires an extended tree segment")
		common.Odl.Error("osonHeader.resolveForwardedOffset: failed", "error", cause, "relativeOffset", relativeOffset)
		return 0, common.NewOracleError(common.OsonParsingError, cause)
	}
	return h.extendedTreeSegmentStartOffset + relativeOffset, nil
}

// resolveOverflowOffset maps one primary-tree absolute node offset through the
// overflow-address mapping into the extended tree segment.
//
// Input:
//   - absoluteOffset: byte offset of an overflow node in the primary tree segment.
//
// Output:
//   - Absolute document offset of the mapped node in the extended tree segment.
//
// Errors:
//   - Returns common.OsonParsingError when no mapping exists or the document has
//     no extended tree segment.
func (h *osonHeader) resolveOverflowOffset(absoluteOffset int) (int, error) {
	if h.forwardingAddresses == nil {
		cause := fmt.Errorf("overflow node requires an overflow-address mapping segment")
		common.Odl.Error("osonHeader.resolveOverflowOffset: failed", "error", cause, "absoluteOffset", absoluteOffset)
		return 0, common.NewOracleError(common.OsonParsingError, cause)
	}
	// Overflow mappings are keyed by the original node's tree-relative position in
	// the primary tree segment, not by absolute document offset.
	relativeOffset := absoluteOffset - h.treeSegmentStartOffset
	forwarded, ok := h.forwardingAddresses[relativeOffset]
	if !ok {
		cause := fmt.Errorf("overflow mapping missing for tree-relative offset %d", relativeOffset)
		common.Odl.Error("osonHeader.resolveOverflowOffset: failed", "error", cause, "absoluteOffset", absoluteOffset, "relativeOffset", relativeOffset)
		return 0, common.NewOracleError(common.OsonParsingError, cause)
	}
	return h.resolveForwardedOffset(forwarded)
}

// locateFieldId performs a binary search on dictionary hash IDs and resolves
// same-hash collisions by comparing the actual key text.
//
// Input:
//   - hash: compact hash appropriate for dict's tier.
//   - key: original UTF-8 field name used to disambiguate collisions.
//   - dict: dictionary with parallel, hash-sorted hashIDs and fieldNames slices.
//
// Output:
//   - A 1-based index within dict, or -1 when key is absent.
func locateFieldId(hash uint32, key string, dict dictionary) int {
	// Hash arrays are sorted on the wire, so use binary search for the first hit.
	idx, found := slices.BinarySearch(dict.hashIDs, hash)
	if !found {
		return -1
	}

	// walk back to the first matching hash so collisions can be scanned linearly.
	for idx > 0 && dict.hashIDs[idx-1] == hash {
		idx--
	}

	// then walk forward until the exact key is found or the hash range ends.
	for idx < len(dict.hashIDs) && dict.hashIDs[idx] == hash {
		if dict.fieldNames[idx] == key {
			return idx + 1
		}
		idx++
	}
	return -1
}

// ohash computes the tier-specific compact hash used by dictionary fixtures.
//
// Input:
//   - key: UTF-8 field name to hash.
//
// Output:
//   - Compact primary or secondary hash and the key's UTF-8 byte length.
func ohash(key string) (uint32, int) {
	// Build the full hash first, then compact it according to the dictionary tier.
	hash, length := osonHash(key)

	if length <= osonMaxPrimaryDictKeyLength {
		return compactPrimaryHash(hash, osonPrimaryDictHashIDSizeUB1), length
	}

	return compactSecondaryHash(hash), length
}

// osonHash computes the full 32-bit FNV-derived field-name hash.
//
// Input:
//   - key: UTF-8 field name to hash.
//
// Output:
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
// Input:
//   - hash: full 32-bit OSON field-name hash.
//   - width: requested compact width in bytes.
//
// Output:
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
// Input:
//   - hash: full 32-bit OSON field-name hash.
//
// Output:
//   - Low two bytes in the byte-swapped order required by the secondary dictionary.
func compactSecondaryHash(hash uint32) uint32 {
	return ((hash & 0xff) << 8) | ((hash & 0xff00) >> 8)
}
