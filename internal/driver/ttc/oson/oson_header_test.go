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
	"math"
	"strconv"
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestParseRejectsMalformedHeaders verifies Parse rejects malformed forms of
// OSON headers.
func TestParseRejectsMalformedHeaders(t *testing.T) {
	for _, length := range []int{0, 1, 5, 8} {
		t.Run("truncated/"+strconv.Itoa(length), func(t *testing.T) {
			if _, err := Parse(sampleScalarTrue.oson[:length]); err == nil {
				t.Fatalf("Parse(%d bytes) error = nil, want malformed-header error", length)
			}
		})
	}
	badMagic := sampleScalarTrue.cloneOSON()
	badMagic[1] = 0
	if _, err := Parse(badMagic); err == nil {
		t.Fatal("Parse(bad magic) error = nil, want malformed-header error")
	}
	for _, version := range []byte{0, 5} {
		badVersion := sampleScalarTrue.cloneOSON()
		badVersion[3] = version
		if _, err := Parse(badVersion); err == nil {
			t.Errorf("Parse(version %d) error = nil, want unsupported-version error", version)
		}
	}
	reservedFlag := sampleScalarTrue.cloneOSON()
	reservedFlag[5] |= 0x80
	if _, err := Parse(reservedFlag); err == nil {
		t.Fatal("Parse(reserved root flag) error = nil, want malformed-header error")
	}
	truncatedTree := sampleScalarTrue.cloneOSON()[:len(sampleScalarTrue.oson)-1]
	if _, err := Parse(truncatedTree); err == nil {
		t.Fatal("Parse(truncated scalar tree) error = nil, want malformed-header error")
	}
	reservedSecondaryFlag := sampleSecondaryDictionary.cloneOSON()
	reservedSecondaryFlag[10] |= 0x02
	if _, err := Parse(reservedSecondaryFlag); err == nil {
		t.Fatal("Parse(reserved secondary flag) error = nil, want malformed-header error")
	}
	if _, err := Parse(sampleMissingInlineLeafFlag.oson); err == nil {
		t.Fatal("Parse(missing inline-leaf flag) error = nil, want malformed-header error")
	}
}

// TestHeaderRejectsInvalidUpdateMetadata verifies the header rejects invalid
// update metadata.
func TestHeaderRejectsInvalidUpdateMetadata(t *testing.T) {
	valid, err := newOsonHeader(newOsonBuffer(sampleUpdatedTinyScalar.oson))
	if err != nil {
		t.Fatalf("newOsonHeader(valid update) error = %v", err)
	}
	updateStart := valid.treeSegmentOffset() + int(valid.treeSegmentByteLength)
	tests := []struct {
		name       string
		appendByte bool
		mutate     func(drvCommon.B1Array)
	}{
		{"reserved flag", false, func(doc drvCommon.B1Array) { doc[updateStart+1] |= 0x02 }},
		{"reserved metadata", false, func(doc drvCommon.B1Array) { doc[updateStart+7] = 0x01 }},
		{"trailing document byte", true, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := sampleUpdatedTinyScalar.cloneOSON()
			if test.appendByte {
				doc = append(doc, 0)
			}
			if test.mutate != nil {
				test.mutate(doc)
			}
			_, err := newOsonHeader(newOsonBuffer(doc))
			if err == nil {
				t.Fatal("newOsonHeader() error = nil, want malformed-update error")
			}
			assertOracleErrorCode(t, err, oracleErrors.InternalError)
		})
	}
}

// TestHeaderRejectsUpdateCaps verifies the header enforces the partial-update
// caps: at most 1024 overflow-address map entries and an 8192-byte map segment,
// for both mapping widths.
func TestHeaderRejectsUpdateCaps(t *testing.T) {
	// Update header layout: [flags UB2][entry count UB2][reserved UB4][map size UB4][extended tree size UB4].
	tests := []struct {
		name   string
		sample osonSample
	}{
		{"UB2", sampleUpdatedOverflow},
		{"UB4", sampleUpdatedOverflowUB4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header, err := newOsonHeader(newOsonBuffer(test.sample.oson))
			if err != nil {
				t.Fatalf("newOsonHeader(valid update) error = %v", err)
			}
			updateStart := header.treeSegmentOffset() + int(header.treeSegmentByteLength)
			for _, limit := range []struct {
				name   string
				offset int
				width  int
				value  int
			}{
				{"entry count", updateStart + osonUB2Size, osonUB2Size, osonUpdateMaxOverflowEntries + 1},
				{"map size", updateStart + 2*osonUB2Size + osonUB4Size, osonUB4Size, osonUpdateMaxOverflowMapSize + 1},
			} {
				t.Run(limit.name, func(t *testing.T) {
					doc := test.sample.cloneOSON()
					patch := &osonWriteBuffer{data: doc}
					if err := patch.patchUint(limit.offset, limit.width, limit.value); err != nil {
						t.Fatalf("patchUint(%s) error = %v", limit.name, err)
					}
					_, err := Parse(doc)
					if err == nil {
						t.Fatalf("Parse accepted %s %d above the OSON limit", limit.name, limit.value)
					}
					assertOracleErrorCode(t, err, oracleErrors.InternalError)
				})
			}
		})
	}
}

// TestHeaderRejectsOutOfRangeUpdateMappings verifies the header rejects update
// mappings pointing outside the tree segment and missing overflow mappings.
func TestHeaderRejectsOutOfRangeUpdateMappings(t *testing.T) {
	t.Run("missing overflow mapping", func(t *testing.T) {
		header := &osonHeader{
			treeSegmentStartOffset: 20,
		}
		_, err := header.resolveOverflowOffset(25)
		assertOracleErrorCode(t, err, oracleErrors.InternalError)
	})

	tests := []struct {
		name   string
		sample osonSample
		mutate func(drvCommon.B1Array, int)
	}{
		{"UB2 source", sampleUpdatedOverflow, func(doc drvCommon.B1Array, offset int) {
			binary.BigEndian.PutUint16(doc[offset:], math.MaxUint16)
		}},
		{"UB2 target", sampleUpdatedOverflow, func(doc drvCommon.B1Array, offset int) {
			binary.BigEndian.PutUint16(doc[offset+2:], math.MaxUint16)
		}},
		{"UB4 source", sampleUpdatedOverflowUB4, func(doc drvCommon.B1Array, offset int) {
			binary.BigEndian.PutUint32(doc[offset:], math.MaxUint32)
		}},
		{"UB4 target", sampleUpdatedOverflowUB4, func(doc drvCommon.B1Array, offset int) {
			binary.BigEndian.PutUint32(doc[offset+4:], math.MaxUint32)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := test.sample.cloneOSON()
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatalf("newOsonHeader(valid fixture) error = %v", err)
			}
			mapOffset := header.treeSegmentOffset() + int(header.treeSegmentByteLength) + 16
			test.mutate(doc, mapOffset)
			_, err = newOsonHeader(newOsonBuffer(doc))
			if err == nil {
				t.Fatal("newOsonHeader(invalid mapping) error = nil, want validation error")
			}
			assertOracleErrorCode(t, err, oracleErrors.InternalError)
		})
	}
}

// TestHeaderRejectsMalformedDictionaryHeap verifies the header bounds-checks
// dictionary heap offsets and name lengths before slicing the heap.
func TestHeaderRejectsMalformedDictionaryHeap(t *testing.T) {
	// Each case corrupts one real OSON fixture: an offset entry pointing
	// past its tier's heap, or the first heap record's length prefix claiming
	// more bytes than the heap holds. Tiers are laid out as hash array,
	// offset array, then heap, directly before the tree segment, so a heap
	// starts at the tree offset minus its packed length-prefixed records.
	tests := []struct {
		name      string
		sample    osonSample
		secondary bool // corrupt the long-key tier instead of the primary
		offset    bool // corrupt an offset entry instead of a length prefix
	}{
		{"primary offset", sampleSimpleObject, false, true},
		{"primary length", sampleSimpleObject, false, false},
		{"secondary offset", sampleSecondaryDictionary, true, true},
		{"secondary length", sampleSecondaryDictionary, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Parse the untouched fixture first: this control proves the
			// sample itself is valid, so any later failure can only come
			// from the mutation below, and it decodes the field names and
			// tree start needed to locate the dictionary bytes.
			doc := test.sample.cloneOSON()
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatalf("newOsonHeader(valid fixture) error = %v", err)
			}

			// Pick the tier under test: primary records are [1-byte length]
			// [name], secondary records are [UB2 length][name]. Summing the
			// packed record sizes gives the heap size, and because the tier
			// ends exactly where the tree begins, treeStart - heapSize is the
			// heap's first byte.
			prefixSize := osonUB1Size
			names := header.fieldDictionary.fieldNames[:header.primaryFieldsCount]
			if test.secondary {
				prefixSize = osonUB2Size
				names = header.fieldDictionary.fieldNames[header.primaryFieldsCount:]
			}
			heapSize := 0
			for _, name := range names {
				heapSize += prefixSize + len(name)
			}
			heapStart := header.treeSegmentStartOffset - heapSize

			if test.offset {
				// The offset array ends where the heap begins, so the last
				// UB2 entry sits at heapStart-2. Rewriting it to 0xFFFF makes
				// the decoder resolve an entry far past the small heap
				// (17 bytes primary, 258 secondary); the offset bounds check
				// must reject it instead of slicing heap[65535:].
				binary.BigEndian.PutUint16(doc[heapStart-osonUB2Size:], math.MaxUint16)
			} else {
				// The first heap record's length prefix starts at heapStart.
				// Setting its first byte to 0xFF declares a name of 255 bytes
				// in the primary tier (16 remain) or 0xFF00 bytes in the
				// secondary tier (256 remain); the length bounds check must
				// reject it instead of slicing past the heap.
				doc[heapStart] = 0xff
			}

			_, err = newOsonHeader(newOsonBuffer(doc))
			if err == nil {
				t.Fatal("newOsonHeader(malformed heap) error = nil, want malformed-dictionary error")
			}
			assertOracleErrorCode(t, err, oracleErrors.InternalError)
		})
	}
}

// TestHeaderRejectsImpossibleDictionaryCounts verifies tiny documents cannot
// declare billions of dictionary entries, without allocating those dictionaries.
func TestHeaderRejectsImpossibleDictionaryCounts(t *testing.T) {
	primaryFlags := drvCommon.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB4Mask)
	primaryCountDoc := drvCommon.B1Array{
		0xff, 0x4a, 0x5a, 0x01, // magic and version
		byte(primaryFlags >> 8), byte(primaryFlags),
		0xff, 0xff, 0xff, 0xff, // primary count: 2^32 - 1
		0x00, 0x00, // primary heap size
		0x00, 0x00, // tree size
		0x00, 0x00, // tiny-node count
	}

	secondaryFlags := drvCommon.UB2(osonFlagInlineLeafMask)
	secondaryCountDoc := drvCommon.B1Array{
		0xff, 0x4a, 0x5a, 0x03, // magic and version
		byte(secondaryFlags >> 8), byte(secondaryFlags),
		0x00,       // primary count
		0x00, 0x00, // primary heap size
		0x00, 0x00, // secondary flags
		0xff, 0xff, 0xff, 0xff, // secondary count: 2^32 - 1
		0x00, 0x00, 0x00, 0x00, // secondary heap size
		0x00, 0x00, // tree size
		0x00, 0x00, // tiny-node count
	}

	tests := []struct {
		name string
		doc  drvCommon.B1Array
	}{
		{name: "primary", doc: primaryCountDoc},
		{name: "secondary", doc: secondaryCountDoc},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&osonHeader{}).readHeader(newOsonBuffer(test.doc))
			if err == nil {
				t.Fatalf("readHeader accepted an impossible %s dictionary count in a %d-byte document; want malformed-header error", test.name, len(test.doc))
			}
		})
	}
}

// TestHeaderUsesFlagSelectedWidths verifies the header reads its fields with
// the widths selected by its flags.
func TestHeaderUsesFlagSelectedWidths(t *testing.T) {
	v3Flags := drvCommon.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB2Mask | osonFlagFieldHeapSizeUB4Mask | osonFlagTreeSegmentSizeUB4Mask)
	v3 := drvCommon.B1Array{
		0xff, 0x4a, 0x5a, 0x03,
		byte(v3Flags >> 8), byte(v3Flags),
		0x01, 0x00,
		0x00, 0x00, 0x01, 0x23,
		0x01, 0x00,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x01, 0x02,
		0x00, 0x00, 0x00, 0x40,
		0x00, 0x02,
	}
	// Supply the declared payload; this test isolates header width decoding.
	v3 = append(v3, make([]byte, 256*5+0x123+4+0x102+0x40)...)
	header := &osonHeader{}
	layout, err := header.readHeader(newOsonBuffer(v3))
	if err != nil {
		t.Fatalf("readHeader(v3) error = %v", err)
	}
	if layout.primaryCount != 256 || layout.primaryHeapSize != 0x123 || layout.secondaryCount != 1 || layout.secondaryHeapSize != 0x102 {
		t.Fatalf("v3 dictionary layout = %+v, want counts 256/1 and heaps 0x123/0x102", layout)
	}
	if header.treeSegmentByteLength != 0x40 || header.tinyNodeStatCount != 2 {
		t.Fatalf("v3 tree metadata = size %d, tiny nodes %d; want 64, 2", header.treeSegmentByteLength, header.tinyNodeStatCount)
	}

	v1Flags := drvCommon.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB4Mask)
	v1 := drvCommon.B1Array{
		0xff, 0x4a, 0x5a, 0x01,
		byte(v1Flags >> 8), byte(v1Flags),
		0x00, 0x00, 0x01, 0x00,
		0x00, 0x08,
		0x00, 0x04,
		0x00, 0x01,
	}
	v1 = append(v1, make([]byte, 256*3+8+4)...)
	layout, err = (&osonHeader{}).readHeader(newOsonBuffer(v1))
	if err != nil {
		t.Fatalf("readHeader(v1) error = %v", err)
	}
	if layout.primaryCount != 256 || layout.primaryHeapSize != 8 {
		t.Fatalf("v1 dictionary layout = %+v, want count 256 and heap 8", layout)
	}
}
