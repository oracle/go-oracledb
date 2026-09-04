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
	"slices"
	"strings"
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestOsonHeader_SampleOson verifies parsing metadata and primary dictionary entries from the basic object fixture.
func TestOsonHeader_SampleOson(t *testing.T) {
	// Basic v1 object with a primary dictionary.
	buf := newOsonBuffer(sampleSimpleObject.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}

	if got, want := header.version(), drvCommon.UB1(1); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if header.isScalar() {
		t.Fatalf("isScalar = true, want false")
	}
	if got, want := int(len(header.fieldDictionary.fieldNames)), 3; got != want {
		t.Fatalf("uniqueFields = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentByteLength, drvCommon.UB4(0x001C); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentOffset(), 39; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}

	wantNames := []string{"role", "active", "name"}
	if got := header.fieldDictionary.fieldNames; !slices.Equal(got, wantNames) {
		t.Fatalf("fieldNames = %v, want %v", got, wantNames)
	}

	for i, wantName := range wantNames {
		if got, ok := header.fieldName(i); !ok || got != wantName {
			t.Fatalf("fieldName(%d) = (%q, %v), want (%q, true)", i, got, ok, wantName)
		}
	}
}

// TestIsOson verifies that valid OSON documents are identified and non-OSON byte sequences are rejected.
func TestIsOson(t *testing.T) {
	if !IsOson(sampleSimpleObject.oson) {
		t.Fatal("IsOson(sampleSimpleObject.oson) = false, want true")
	}
	if !IsOson(sampleScalarTrue.oson) {
		t.Fatal("IsOson(sampleScalarTrue.oson) = false, want true")
	}
	if IsOson(drvCommon.B1Array(`{"name":"Alice"}`)) {
		t.Fatal("IsOson(text JSON) = true, want false")
	}
	if IsOson(drvCommon.B1Array{0xFF, 0x4A, 0x5A}) {
		t.Fatal("IsOson(short prefix) = true, want false")
	}
}

// TestOsonHeader_ScalarDocumentUsesPostHeaderTreeOffset verifies a scalar tree begins immediately after its fixed header.
func TestOsonHeader_ScalarDocumentUsesPostHeaderTreeOffset(t *testing.T) {
	// Scalars start the tree right after the fixed header.
	buf := newOsonBuffer(sampleScalarTrue.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}

	if !header.isScalar() {
		t.Fatal("isScalar = false, want true")
	}
	if got, want := header.treeSegmentOffset(), 8; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentByteLength, drvCommon.UB4(1); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := buf.position(), header.treeSegmentOffset(); got != want {
		t.Fatalf("buffer position = %d, want tree segment offset %d", got, want)
	}
}

// TestOsonHeader_UpdatedTinyScalarFixture verifies the V2 update metadata of
// the database-produced tiny scalar replacement fixture.
func TestOsonHeader_UpdatedTinyScalarFixture(t *testing.T) {
	buffer := newOsonBuffer(sampleUpdatedTinyScalar.oson)
	header, err := newOsonHeader(buffer)
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if got, want := header.version(), drvCommon.UB1(2); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if got, want := header.updateHeaderFlags, drvCommon.UB2(osonFlagUpdateOverflowSegmentUB2Mask); got != want {
		t.Fatalf("updateHeaderFlags = 0x%04x, want 0x%04x", got, want)
	}
	if got, want := header.extendedTreeSegmentStartOffset, 51; got != want {
		t.Fatalf("extendedTreeSegmentStartOffset = %d, want %d", got, want)
	}
	if got := len(header.forwardingAddresses); got != 0 {
		t.Fatalf("forwarding address count = %d, want 0", got)
	}
}

// TestOsonHeader_RejectsMalformedUpdateMetadata verifies update-header
// reserved fields and segment boundaries are validated.
func TestOsonHeader_RejectsMalformedUpdateMetadata(t *testing.T) {
	validHeader, err := newOsonHeader(newOsonBuffer(sampleUpdatedTinyScalar.oson))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	updateOffset := validHeader.treeSegmentOffset() + int(validHeader.treeSegmentByteLength)

	tests := []struct {
		name       string
		appendByte bool
		mutate     func(drvCommon.B1Array)
	}{
		{
			name: "reserved update flag",
			mutate: func(doc drvCommon.B1Array) {
				doc[updateOffset] = 0x02
			},
		},
		{
			name: "reserved update bytes",
			mutate: func(doc drvCommon.B1Array) {
				doc[updateOffset+4] = 0x01
			},
		},
		{
			name: "mapping count exceeds map capacity",
			mutate: func(doc drvCommon.B1Array) {
				doc[updateOffset+3] = 0x01
				binary.BigEndian.PutUint32(doc[updateOffset+8:], 0)
			},
		},
		{
			name:       "trailing byte after extended tree",
			appendByte: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := sampleUpdatedTinyScalar.cloneOSON()
			if test.appendByte {
				doc = append(doc, 0x00)
			}
			if test.mutate != nil {
				test.mutate(doc)
			}
			_, err := newOsonHeader(newOsonBuffer(doc))
			if err == nil {
				t.Fatal("newOsonHeader() error = nil, want failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
		})
	}
}

// TestOsonHeader_RejectsV1UpdateMetadata verifies partial-update segments are
// accepted only in their V2/V4 document forms.
func TestOsonHeader_RejectsV1UpdateMetadata(t *testing.T) {
	doc := append(sampleSimpleObject.cloneOSON(), make(drvCommon.B1Array, 16)...)
	_, err := newOsonHeader(newOsonBuffer(doc))
	if err == nil {
		t.Fatal("newOsonHeader() error = nil, want V1 update-metadata failure")
	}
	assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
}

// TestOsonHeader_RejectsOutOfRangeUpdateMappings verifies update-map entries
// are validated even when no decoded node has reached the mapping yet.
func TestOsonHeader_RejectsOutOfRangeUpdateMappings(t *testing.T) {
	tests := []struct {
		name   string
		sample osonSample
		mutate func(drvCommon.B1Array, int)
	}{
		{
			name:   "UB2 source outside primary tree",
			sample: sampleUpdatedOverflow,
			mutate: func(doc drvCommon.B1Array, mapOffset int) {
				binary.BigEndian.PutUint16(doc[mapOffset:], 0xffff)
			},
		},
		{
			name:   "UB2 target outside extended tree",
			sample: sampleUpdatedOverflow,
			mutate: func(doc drvCommon.B1Array, mapOffset int) {
				binary.BigEndian.PutUint16(doc[mapOffset+2:], 0xffff)
			},
		},
		{
			name:   "UB4 source outside primary tree",
			sample: sampleUpdatedOverflowUB4,
			mutate: func(doc drvCommon.B1Array, mapOffset int) {
				binary.BigEndian.PutUint32(doc[mapOffset:], 0xffffffff)
			},
		},
		{
			name:   "UB4 target outside extended tree",
			sample: sampleUpdatedOverflowUB4,
			mutate: func(doc drvCommon.B1Array, mapOffset int) {
				binary.BigEndian.PutUint32(doc[mapOffset+4:], 0xffffffff)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := test.sample.cloneOSON()
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatal(err)
			}
			mapOffset := header.treeSegmentOffset() + int(header.treeSegmentByteLength) + 16
			test.mutate(doc, mapOffset)
			if _, err := newOsonHeader(newOsonBuffer(doc)); err == nil {
				t.Fatal("newOsonHeader() error = nil, want invalid-map failure")
			} else {
				assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
			}
		})
	}
}

// TestOsonHeader_NestedObjectArrayFixture verifies metadata, dictionary ordering, and buffer positioning for a nested fixture.
func TestOsonHeader_NestedObjectArrayFixture(t *testing.T) {
	// Larger primary dictionary with tiny-node stats.
	buf := newOsonBuffer(sampleNestedObjectArray.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}

	if got, want := header.version(), drvCommon.UB1(1); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if header.isScalar() {
		t.Fatal("isScalar = true, want false")
	}
	if got, want := int(len(header.fieldDictionary.fieldNames)), 7; got != want {
		t.Fatalf("uniqueFields = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentByteLength, drvCommon.UB4(0x003A); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentOffset(), 60; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}
	if got, want := header.tinyNodeStatCount, drvCommon.UB2(1); got != want {
		t.Fatalf("tinyNodeCount = %d, want %d", got, want)
	}

	want := []string{"x", "items", "ok", "id", "name", "user", "y"}
	if got := header.fieldDictionary.fieldNames; !slices.Equal(got, want) {
		t.Fatalf("fieldNames = %v, want %v", got, want)
	}
	for i, name := range want {
		if got, ok := header.fieldName(i); !ok || got != name {
			t.Fatalf("fieldName(%d) = (%q, %v), want (%q, true)", i, got, ok, name)
		}

	}
	if got, want := buf.position(), header.treeSegmentOffset(); got != want {
		t.Fatalf("buffer position = %d, want reset to tree segment offset %d", got, want)
	}
}

// TestOsonHeader_SecondaryDictionaryFixture verifies parsing and lookup across primary and secondary dictionaries.
func TestOsonHeader_SecondaryDictionaryFixture(t *testing.T) {
	// v3 split dictionary with one short and one long key.
	buf := newOsonBuffer(sampleSecondaryDictionary.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}

	longKey := osonLongKey

	if got, want := header.version(), drvCommon.UB1(3); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if header.isScalar() {
		t.Fatal("isScalar = true, want false")
	}
	if got, want := int(len(header.fieldDictionary.fieldNames)), 2; got != want {
		t.Fatalf("uniqueFields2 = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentByteLength, drvCommon.UB4(0x000E); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentOffset(), 294; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}

	want := []string{"short", longKey}
	if got := header.fieldDictionary.fieldNames; !slices.Equal(got, want) {
		t.Fatalf("fieldNames length/order mismatch: got %v entries, want %v", len(got), len(want))
	}
	for i, name := range want {
		if got, ok := header.fieldName(i); !ok || got != name {
			t.Fatalf("fieldName(%d) = (%q, %v), want (%q, true)", i, got, ok, name)
		}

	}
	if got, want := buf.position(), header.treeSegmentOffset(); got != want {
		t.Fatalf("buffer position = %d, want reset to tree segment offset %d", got, want)
	}
}

// TestOsonHeader_RejectsMissingInlineLeafFlag verifies headers missing the required inline-leaf flag are rejected.
func TestOsonHeader_RejectsMissingInlineLeafFlag(t *testing.T) {
	// Inline-leaf is required here.
	buf := newOsonBuffer(sampleMissingInlineLeafFlag.oson)
	if _, err := newOsonHeader(buf); err == nil {
		t.Fatal("newOsonHeader succeeded, want error for missing inline-leaf flag")
	}
}

// TestOsonHeader_RejectsTruncatedTreeSegment verifies truncated object and scalar tree segments are rejected.
func TestOsonHeader_RejectsTruncatedTreeSegment(t *testing.T) {
	// Keep the header and dictionary bytes intact but truncate the declared tree.
	truncated := sampleSimpleObject.cloneOSON()
	truncated = truncated[:len(truncated)-1]
	buf := newOsonBuffer(truncated)

	if _, err := newOsonHeader(buf); err == nil {
		t.Fatal("newOsonHeader succeeded, want error for truncated tree segment")
	}

	truncated = sampleScalarTrue.cloneOSON()
	truncated = truncated[:len(truncated)-1]
	buf = newOsonBuffer(truncated)

	if _, err := newOsonHeader(buf); err == nil {
		t.Fatal("newOsonHeader succeeded, want error for truncated scalar tree segment")
	}
}

// TestOsonHeader_RejectsCorruptPrimaryDictionaryOffset verifies out-of-heap primary offsets return contextual errors.
func TestOsonHeader_RejectsCorruptPrimaryDictionaryOffset(t *testing.T) {
	corrupt := sampleSimpleObject.cloneOSON()
	// sampleSimpleObject stores:
	//   header[0:13]
	//   3 primary hash bytes
	//   3 UB2 dictionary offsets
	// The first offset starts at byte 16.
	binary.BigEndian.PutUint16(corrupt[16:], 0xffff)

	_, err := newOsonHeader(newOsonBuffer(corrupt))
	if err == nil {
		t.Fatal("newOsonHeader succeeded, want error for corrupt dictionary offset")
	}

	if !strings.Contains(err.Error(), "outside heap") {
		t.Fatalf("error = %v, want heap-offset context", err)
	}
}

// TestOsonHeader_RejectsMalformedFixedHeader verifies malformed fixed-header variants return OSON header errors.
func TestOsonHeader_RejectsMalformedFixedHeader(t *testing.T) {
	tests := []struct {
		name string
		doc  drvCommon.B1Array
	}{
		{
			name: "truncated magic",
			doc:  drvCommon.B1Array{0xff, 0x4a, 0x5a},
		},
		{
			name: "invalid magic",
			doc:  drvCommon.B1Array{0xff, 0x4a, 0x00, 0x01, 0x00, 0x16},
		},
		{
			name: "unsupported version low",
			doc:  drvCommon.B1Array{0xff, 0x4a, 0x5a, 0x00, 0x00, 0x16},
		},
		{
			name: "unsupported version high",
			doc:  drvCommon.B1Array{0xff, 0x4a, 0x5a, 0x05, 0x00, 0x16},
		},
		{
			name: "truncated scalar tree size",
			doc:  drvCommon.B1Array{0xff, 0x4a, 0x5a, 0x01, 0x00, 0x16, 0x00},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newOsonHeader(newOsonBuffer(tc.doc))
			if err == nil {
				t.Fatal("newOsonHeader() error = nil, want failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
		})
	}
}

// TestOsonHeader_RejectsReservedFlags verifies persistent reserved flag bits
// cannot silently change the interpreted wire layout.
func TestOsonHeader_RejectsReservedFlags(t *testing.T) {
	tests := []struct {
		name string
		doc  drvCommon.B1Array
	}{
		{
			name: "root flag one",
			doc: func() drvCommon.B1Array {
				doc := sampleScalarTrue.cloneOSON()
				doc[4] |= 0x02
				return doc
			}(),
		},
		{
			name: "root flag two",
			doc: func() drvCommon.B1Array {
				doc := sampleScalarTrue.cloneOSON()
				doc[5] |= 0x80
				return doc
			}(),
		},
		{
			name: "secondary flag three",
			doc: func() drvCommon.B1Array {
				doc := sampleSecondaryDictionary.cloneOSON()
				doc[9] |= 0x02
				return doc
			}(),
		},
		{
			name: "secondary flag four",
			doc: func() drvCommon.B1Array {
				doc := sampleSecondaryDictionary.cloneOSON()
				doc[10] = 0x01
				return doc
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newOsonHeader(newOsonBuffer(test.doc))
			if err == nil {
				t.Fatal("newOsonHeader() error = nil, want reserved-flag failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
		})
	}
}

// TestOsonHeader_ScalarTreeSizeUB4 verifies scalar documents honor a UB4-encoded tree segment size.
func TestOsonHeader_ScalarTreeSizeUB4(t *testing.T) {
	doc := buildScalarOsonForTest(drvCommon.B1Array{osonOpFalse}, osonFlagTreeSegmentSizeUB4Mask, nil)
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if got, want := header.treeSegmentOffset(), 10; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentByteLength, drvCommon.UB4(1); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
}

// TestOsonHeader_ReadHeaderWidthVariants verifies flag-controlled header field widths for v1 and v3 documents.
func TestOsonHeader_ReadHeaderWidthVariants(t *testing.T) {
	v3Flags := drvCommon.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB2Mask | osonFlagFieldHeapSizeUB4Mask | osonFlagTreeSegmentSizeUB4Mask)
	v3Doc := drvCommon.B1Array{
		0xff, 0x4a, 0x5a, 0x03,
		byte(v3Flags >> 8), byte(v3Flags),
		0x01, 0x00, // primary field count UB2
		0x00, 0x00, 0x01, 0x23, // primary heap size UB4
		0x01, 0x00, // secondary flags
		0x00, 0x00, 0x00, 0x01, // secondary count
		0x00, 0x00, 0x01, 0x02, // secondary heap size
		0x00, 0x00, 0x00, 0x40, // tree size UB4
		0x00, 0x02, // tiny node count
	}
	header := &osonHeader{}
	layout, err := header.readHeader(newOsonBuffer(v3Doc))
	if err != nil {
		t.Fatalf("readHeader(v3 widths) error = %v", err)
	}
	if layout.primaryCount != 0x0100 || layout.primaryHeapSize != 0x0123 {
		t.Fatalf("primary layout = (%d, %d), want (256, 291)", layout.primaryCount, layout.primaryHeapSize)
	}
	if layout.secondaryCount != 1 || layout.secondaryHeapSize != 0x0102 {
		t.Fatalf("secondary layout = (%d, %d), want (1, 258)", layout.secondaryCount, layout.secondaryHeapSize)
	}
	if got, want := header.treeSegmentByteLength, drvCommon.UB4(0x40); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.tinyNodeStatCount, drvCommon.UB2(2); got != want {
		t.Fatalf("tinyNodeCount = %d, want %d", got, want)
	}

	v1Flags := drvCommon.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB4Mask)
	v1Doc := drvCommon.B1Array{
		0xff, 0x4a, 0x5a, 0x01,
		byte(v1Flags >> 8), byte(v1Flags),
		0x00, 0x00, 0x01, 0x00, // primary field count UB4
		0x00, 0x08, // primary heap size UB2
		0x00, 0x04, // tree size UB2
		0x00, 0x01, // tiny node count
	}
	header = &osonHeader{}
	layout, err = header.readHeader(newOsonBuffer(v1Doc))
	if err != nil {
		t.Fatalf("readHeader(v1 UB4 count) error = %v", err)
	}
	if layout.primaryCount != 0x0100 || layout.primaryHeapSize != 8 {
		t.Fatalf("v1 primary layout = (%d, %d), want (256, 8)", layout.primaryCount, layout.primaryHeapSize)
	}
}

// TestOsonHeader_MetadataHelpersReflectFlagsAndBounds verifies helper behavior for flags, segment selection, and invalid field indexes.
func TestOsonHeader_MetadataHelpersReflectFlagsAndBounds(t *testing.T) {
	header := &osonHeader{
		flags: osonFlagInlineLeafMask |
			osonFlagObjectFIDsUnsortedMask |
			osonFlagDistinctFieldCountUB2Mask,
		treeSegmentStartOffset:         10,
		extendedTreeSegmentStartOffset: 20,
	}
	if !header.isInlineLeaf() {
		t.Fatal("isInlineLeaf() = false, want true")
	}
	if header.fieldsSorted() {
		t.Fatal("fieldsSorted() = true, want false for unsorted flag")
	}
	if got, want := header.numFieldIDBytes(), osonUB2Size; got != want {
		t.Fatalf("numFieldIDBytes() = %d, want %d", got, want)
	}
	header.flags = osonFlagDistinctFieldCountUB4Mask
	if got, want := header.numFieldIDBytes(), osonUB4Size; got != want {
		t.Fatalf("numFieldIDBytes() = %d, want %d", got, want)
	}
	if got, want := header.segmentOffsetForNode(25), 20; got != want {
		t.Fatalf("segmentOffsetForNode(extended) = %d, want %d", got, want)
	}

	if name, ok := header.fieldName(-1); ok || name != "" {
		t.Fatalf("fieldName(-1) = (%q, %v), want empty false", name, ok)
	}
	if got := compactPrimaryHash(0x01020304, osonUB2Size); got != 0x0304 {
		t.Fatalf("compactPrimaryHash(UB2) = %#x, want 0x0304", got)
	}
	if got := compactPrimaryHash(0x01020304, osonUB4Size); got != 0x01020304 {
		t.Fatalf("compactPrimaryHash(UB4) = %#x, want original hash", got)
	}
}

// TestOsonHeader_ForwardingHelpers verifies extended-tree offset resolution
// without constructing an OSON update document.
func TestOsonHeader_ForwardingHelpers(t *testing.T) {
	header := &osonHeader{
		treeSegmentStartOffset:         20,
		extendedTreeSegmentStartOffset: 100,
		extendedTreeSegmentByteLength:  100,
		forwardingAddresses:            map[int]int{10: 30},
	}

	if got, err := header.resolveForwardedOffset(30); err != nil || got != 130 {
		t.Fatalf("resolveForwardedOffset(30) = (%d, %v), want (130, nil)", got, err)
	}
	if got, err := header.resolveOverflowOffset(30); err != nil || got != 130 {
		t.Fatalf("resolveOverflowOffset(30) = (%d, %v), want (130, nil)", got, err)
	}
	if _, err := header.resolveForwardedOffset(100); err == nil {
		t.Fatal("resolveForwardedOffset(100) error = nil, want bounds failure")
	}

	for _, test := range []struct {
		name string
		call func(*osonHeader) error
	}{
		{
			name: "forwarded offset without extended tree",
			call: func(header *osonHeader) error {
				_, err := header.resolveForwardedOffset(0)
				return err
			},
		},
		{
			name: "overflow offset without mapping",
			call: func(header *osonHeader) error {
				_, err := header.resolveOverflowOffset(20)
				return err
			},
		},
		{
			name: "overflow offset absent from mapping",
			call: func(header *osonHeader) error {
				_, err := header.resolveOverflowOffset(21)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := &osonHeader{}
			if test.name == "overflow offset absent from mapping" {
				candidate = header
			}
			err := test.call(candidate)
			if err == nil {
				t.Fatal("offset resolution error = nil, want failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
		})
	}

	for _, sample := range []osonSample{sampleUpdatedOverflow, sampleUpdatedOverflowUB4} {
		doc := sample.cloneOSON()
		for length := 0; length < len(doc); length++ {
			_, _ = newOsonHeader(newOsonBuffer(doc[:length]))
		}
	}

	if err := (&osonHeader{formatVersion: 2}).readSecondaryDictionary(newOsonBuffer(nil), _parsedDictionaryLayout{secondaryCount: 1, secondaryHeapSize: 1}); err == nil {
		t.Fatal("readSecondaryDictionary(v2) error = nil, want version failure")
	}
	if err := (&osonHeader{formatVersion: 3}).readSecondaryDictionary(newOsonBuffer(nil), _parsedDictionaryLayout{}); err != nil {
		t.Fatalf("readSecondaryDictionary(empty) error = %v", err)
	}
	if err := (&osonHeader{formatVersion: 3}).readSecondaryDictionary(newOsonBuffer(nil), _parsedDictionaryLayout{secondaryCount: 1}); err == nil {
		t.Fatal("readSecondaryDictionary(empty heap) error = nil, want failure")
	}
}

// TestOsonHeader_AddForwardingAddressValidatesMappings verifies update-map
// entries stay within their respective tree segments and have unique sources.
func TestOsonHeader_AddForwardingAddressValidatesMappings(t *testing.T) {
	header := &osonHeader{
		treeSegmentByteLength: 10,
		forwardingAddresses:   make(map[int]int),
	}
	if err := header.addForwardingAddress(3, 7, 8); err != nil {
		t.Fatalf("addForwardingAddress(valid) error = %v", err)
	}
	if got := header.forwardingAddresses[3]; got != 7 {
		t.Fatalf("forwarding address = %d, want 7", got)
	}

	for _, test := range []struct {
		name string
		from int
		to   int
	}{
		{name: "source before primary tree", from: -1, to: 0},
		{name: "source after primary tree", from: 10, to: 0},
		{name: "target before extended tree", from: 4, to: -1},
		{name: "target after extended tree", from: 4, to: 8},
		{name: "duplicate source", from: 3, to: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := header.addForwardingAddress(test.from, test.to, 8)
			if err == nil {
				t.Fatal("addForwardingAddress() error = nil, want validation failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
		})
	}
}

// TestOsonHeader_RejectsNilBuffer verifies header initialization reports a
// parser error instead of dereferencing a nil buffer.
func TestOsonHeader_RejectsNilBuffer(t *testing.T) {
	err := (&osonHeader{}).initialize(nil)
	if err == nil {
		t.Fatal("initialize(nil) error = nil, want failure")
	}
	assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
}

// TestOsonHeader_RejectsCorruptDictionaryHeaps verifies malformed primary and secondary dictionary heaps are rejected.
func TestOsonHeader_RejectsCorruptDictionaryHeaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(drvCommon.B1Array)
	}{
		{
			name: "primary heap size zero with nonzero field count",
			mutate: func(doc drvCommon.B1Array) {
				binary.BigEndian.PutUint16(doc[7:], 0)
			},
		},
		{
			name: "primary entry length exceeds heap",
			mutate: func(doc drvCommon.B1Array) {
				// First primary heap byte is the length of the "name" field.
				doc[22] = 0x20
			},
		},
		{
			name: "secondary entry too short for long-key tier",
			mutate: func(doc drvCommon.B1Array) {
				// In sampleSecondaryDictionary the secondary heap starts at byte 36.
				binary.BigEndian.PutUint16(doc[36:], osonMaxPrimaryDictKeyLength)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := sampleSimpleObject
			if strings.HasPrefix(tc.name, "secondary") {
				source = sampleSecondaryDictionary
			}
			corrupt := source.cloneOSON()
			tc.mutate(corrupt)
			_, err := newOsonHeader(newOsonBuffer(corrupt))
			if err == nil {
				t.Fatal("newOsonHeader() error = nil, want dictionary failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
		})
	}
}

// TestOsonHeader_DictionaryReadersHandleBothOffsetWidths verifies the compact
// and wide offset encodings used by the dictionary parser components.
func TestOsonHeader_DictionaryReadersHandleBothOffsetWidths(t *testing.T) {
	tests := []struct {
		name   string
		header *osonHeader
		data   drvCommon.B1Array
		read   func(*osonHeader, *osonBuffer) ([]int, error)
		want   []int
	}{
		{
			name:   "primary ub2 offsets",
			header: &osonHeader{},
			data:   drvCommon.B1Array{0x00, 0x03, 0x00, 0x07},
			read: func(header *osonHeader, buffer *osonBuffer) ([]int, error) {
				return header.readPrimaryOffsets(buffer, 2)
			},
			want: []int{3, 7},
		},
		{
			name:   "primary ub4 offsets",
			header: &osonHeader{flags: osonFlagFieldHeapSizeUB4Mask},
			data:   drvCommon.B1Array{0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x01, 0x00},
			read: func(header *osonHeader, buffer *osonBuffer) ([]int, error) {
				return header.readPrimaryOffsets(buffer, 2)
			},
			want: []int{3, 256},
		},
		{
			name:   "secondary ub2 offsets",
			header: &osonHeader{secondaryFlags: osonFlagSecondaryFieldOffsetsUB2Mask},
			data:   drvCommon.B1Array{0x00, 0x03, 0x00, 0x07},
			read: func(header *osonHeader, buffer *osonBuffer) ([]int, error) {
				return header.readSecondaryOffsets(buffer, 2)
			},
			want: []int{3, 7},
		},
		{
			name:   "secondary ub4 offsets",
			header: &osonHeader{},
			data:   drvCommon.B1Array{0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x01, 0x00},
			read: func(header *osonHeader, buffer *osonBuffer) ([]int, error) {
				return header.readSecondaryOffsets(buffer, 2)
			},
			want: []int{3, 256},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.read(test.header, newOsonBuffer(test.data))
			if err != nil {
				t.Fatalf("dictionary read error = %v", err)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("offsets = %v, want %v", got, test.want)
			}
		})
	}
}

// TestOsonHeader_DictionaryReadersRejectTruncation verifies dictionary reader
// failures return parser errors rather than indexing past the input buffer.
func TestOsonHeader_DictionaryReadersRejectTruncation(t *testing.T) {
	tests := []struct {
		name string
		read func() error
	}{
		{
			name: "primary hashes",
			read: func() error {
				_, err := (&osonHeader{}).readPrimaryHashes(newOsonBuffer(nil), 1)
				return err
			},
		},
		{
			name: "primary ub4 offsets",
			read: func() error {
				_, err := (&osonHeader{flags: osonFlagFieldHeapSizeUB4Mask}).readPrimaryOffsets(newOsonBuffer(drvCommon.B1Array{0, 0, 0}), 1)
				return err
			},
		},
		{
			name: "secondary hashes",
			read: func() error {
				_, err := (&osonHeader{}).readSecondaryHashes(newOsonBuffer(drvCommon.B1Array{0}), 1)
				return err
			},
		},
		{
			name: "secondary ub2 offsets",
			read: func() error {
				_, err := (&osonHeader{secondaryFlags: osonFlagSecondaryFieldOffsetsUB2Mask}).readSecondaryOffsets(newOsonBuffer(drvCommon.B1Array{0}), 1)
				return err
			},
		},
		{
			name: "secondary ub4 offsets",
			read: func() error {
				_, err := (&osonHeader{}).readSecondaryOffsets(newOsonBuffer(drvCommon.B1Array{0, 0, 0}), 1)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.read()
			if err == nil {
				t.Fatal("dictionary read error = nil, want failure")
			}
			assertOracleErrorCode(t, err, oracleErrors.OsonHeaderError)
		})
	}
}

// buildScalarOsonForTest creates a minimal v1 scalar OSON document with optional header flags and trailing bytes.
func buildScalarOsonForTest(tree drvCommon.B1Array, extraFlags drvCommon.UB2, tail drvCommon.B1Array) drvCommon.B1Array {
	flags := drvCommon.UB2(0x0016) | extraFlags
	doc := drvCommon.B1Array{0xff, 0x4a, 0x5a, 0x01, byte(flags >> 8), byte(flags)}
	if flags&osonFlagTreeSegmentSizeUB4Mask != 0 {
		doc = append(doc, 0, 0, 0, byte(len(tree)))
	} else {
		doc = append(doc, 0, byte(len(tree)))
	}
	doc = append(doc, tree...)
	doc = append(doc, tail...)
	return doc
}

// TestOsonHeader_RejectsTruncatedInput exercises each length-sensitive header
// reader with a progressively truncated document.
func TestOsonHeader_RejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		doc  drvCommon.B1Array
	}{
		{"fixed header", sampleSimpleObject.cloneOSON()},
		{"scalar header", sampleScalarTrue.cloneOSON()},
		{"update header", sampleUpdatedTinyScalar.cloneOSON()},
	} {
		t.Run(test.name, func(t *testing.T) {
			limit := len(test.doc)
			if test.name == "update header" {
				limit--
			}
			for length := 0; length <= limit; length++ {
				_, _ = newOsonHeader(newOsonBuffer(test.doc[:length]))
			}
		})
	}
}

// TestOsonHeader_RejectsInvalidSecondaryDictionary covers the independent
// validation stages of a long-key dictionary entry.
func TestOsonHeader_RejectsInvalidSecondaryDictionary(t *testing.T) {
	t.Parallel()

	makeDocument := func(heap drvCommon.B1Array, offset uint32) drvCommon.B1Array {
		doc := drvCommon.B1Array{0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(doc[2:], offset)
		doc = append(doc, heap...)
		return doc
	}
	cases := []struct {
		name string
		doc  drvCommon.B1Array
		size int
	}{
		{"truncated hash", nil, 1},
		{"truncated offset", drvCommon.B1Array{0, 0}, 1},
		{"truncated heap", drvCommon.B1Array{0, 0, 0, 0, 0, 0}, 1},
		{"offset outside heap", makeDocument(drvCommon.B1Array{0}, 1), 1},
		{"entry missing length", makeDocument(drvCommon.B1Array{0}, 0), 1},
		{"entry too short for primary tier", makeDocument(drvCommon.B1Array{0, 0}, 0), 2},
		{"entry exceeds heap", makeDocument(drvCommon.B1Array{1, 0, 0, 0}, 0), 4},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			header := &osonHeader{formatVersion: 3}
			if err := header.readSecondaryDictionary(newOsonBuffer(test.doc), _parsedDictionaryLayout{secondaryCount: 1, secondaryHeapSize: test.size}); err == nil {
				t.Fatal("readSecondaryDictionary() error = nil, want malformed-entry failure")
			}
		})
	}

	heap := append(drvCommon.B1Array{1, 0}, drvCommon.B1Array{0xff}...)
	heap = append(heap, make(drvCommon.B1Array, 255)...)
	header := &osonHeader{formatVersion: 3}
	if err := header.readSecondaryDictionary(newOsonBuffer(makeDocument(heap, 0)), _parsedDictionaryLayout{secondaryCount: 1, secondaryHeapSize: len(heap)}); err == nil {
		t.Fatal("readSecondaryDictionary(invalid UTF-8) error = nil, want failure")
	}
}
