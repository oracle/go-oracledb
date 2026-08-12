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

	"github.com/oracle/go-driver/driver/common"
)

// hashOrderedField pairs a dictionary field name with its compact hash for ordering assertions.
type hashOrderedField struct {
	name string
	hash int
}

// equalHashOrderedFieldNames reports whether names match the expected compact-hash ordering.
func equalHashOrderedFieldNames(names []string, expected []hashOrderedField) bool {
	if len(names) != len(expected) {
		return false
	}
	for i := range names {
		if names[i] != expected[i].name {
			return false
		}
	}
	return true
}

// TestOsonHeader_SampleOson verifies parsing metadata and primary dictionary entries from the basic object fixture.
func TestOsonHeader_SampleOson(t *testing.T) {
	// Basic v1 object with a primary dictionary.
	buf := newOsonBuffer(sampleSimpleObject.oson)
	header, err := newOsonHeader(buf)
	if err != nil {
		t.Fatalf("newOsonHeader returned error: %v", err)
	}

	if got, want := header.version(), common.UB1(1); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if header.isScalar() {
		t.Fatalf("isScalar = true, want false")
	}
	if got, want := int(header.uniqueFields()), 3; got != want {
		t.Fatalf("uniqueFields = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentSize(), common.UB4(0x001C); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentOffset(), 39; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}

	expectedNames := []string{"role", "name", "active"}
	expectedFields := make([]hashOrderedField, len(expectedNames))
	for i, name := range expectedNames {
		hash, _ := ohash(name)
		expectedFields[i].name = name
		expectedFields[i].hash = int(hash)
	}
	slices.SortFunc(expectedFields, func(a, b hashOrderedField) int {
		return a.hash - b.hash
	})
	if got := header.fieldNames(); !equalHashOrderedFieldNames(got, expectedFields) {
		t.Fatalf("fieldNames = %v, want %v", got, expectedNames)
	}

	for i, expected := range expectedFields {
		if got, ok := header.fieldName(i); !ok || got != expected.name {
			t.Fatalf("fieldName(%d) = (%q, %v), want (%q, true)", i, got, ok, expected.name)
		}
		if fid := header.fieldID(expected.name); fid != i+1 {
			t.Fatalf("fieldID(%q) = %d, want %d", expected.name, fid, i+1)
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
	if IsOson(common.B1Array(`{"name":"Alice"}`)) {
		t.Fatal("IsOson(text JSON) = true, want false")
	}
	if IsOson(common.B1Array{0xFF, 0x4A, 0x5A}) {
		t.Fatal("IsOson(short prefix) = true, want false")
	}
}

// TestOsonHeader_GetFieldId_UsesPrimaryHashWidthForUB1 verifies primary dictionary lookup with a one-byte hash.
func TestOsonHeader_GetFieldId_UsesPrimaryHashWidthForUB1(t *testing.T) {
	// Primary lookups use compact UB1 hashes.
	full, _ := osonHash("alpha")
	header := &osonHeader{
		primaryFieldsCount: 1,
		fieldDictionary: dictionary{
			hashIDs:    []uint32{compactPrimaryHash(full, 1)},
			fieldNames: []string{"alpha"},
		},
	}

	if got := header.fieldID("alpha"); got != 1 {
		t.Fatalf("fieldID(alpha) = %d, want 1", got)
	}
}

// TestOsonHeader_GetFieldId_UsesUTF8BytesForPrimaryKey verifies primary-key tier selection uses UTF-8 byte length.
func TestOsonHeader_GetFieldId_UsesUTF8BytesForPrimaryKey(t *testing.T) {
	// Tier selection uses UTF-8 byte length.
	key := "cafeé"
	h, n := ohash(key)
	if want := len([]byte(key)); n != want {
		t.Fatalf("ohash(%q) length = %d, want UTF-8 length %d", key, n, want)
	}

	header := &osonHeader{
		primaryFieldsCount: 1,
		fieldDictionary: dictionary{
			hashIDs:    []uint32{h},
			fieldNames: []string{key},
		},
	}

	if got := header.fieldID(key); got != 1 {
		t.Fatalf("fieldID(%q) = %d, want 1", key, got)
	}
}

// TestOsonHeader_GetFieldId_UsesUTF8LengthForSecondaryKey verifies a 256-byte UTF-8 key is looked up in the secondary dictionary.
func TestOsonHeader_GetFieldId_UsesUTF8LengthForSecondaryKey(t *testing.T) {
	// 256 UTF-8 bytes moves the key to the long-key tier.
	key := strings.Repeat("é", 128) // 256 UTF-8 bytes, so this must use the secondary dictionary.
	h, n := ohash(key)
	if want := len([]byte(key)); n != want {
		t.Fatalf("ohash(long UTF-8 key) length = %d, want %d", n, want)
	}

	header := &osonHeader{
		primaryFieldsCount: 0,
		fieldDictionary: dictionary{
			hashIDs:    []uint32{h},
			fieldNames: []string{key},
		},
	}

	if got := header.fieldID(key); got != 1 {
		t.Fatalf("fieldID(long UTF-8 key) = %d, want 1", got)
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
	if got, want := header.treeSegmentSize(), common.UB4(1); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := buf.position(), header.treeSegmentOffset(); got != want {
		t.Fatalf("buffer position = %d, want tree segment offset %d", got, want)
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

	if got, want := header.version(), common.UB1(1); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if header.isScalar() {
		t.Fatal("isScalar = true, want false")
	}
	if got, want := int(header.uniqueFields()), 7; got != want {
		t.Fatalf("uniqueFields = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentSize(), common.UB4(0x003A); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentOffset(), 60; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}
	if got, want := header.tinyNodeCount(), common.UB2(1); got != want {
		t.Fatalf("tinyNodeCount = %d, want %d", got, want)
	}

	want := []string{"x", "items", "ok", "id", "name", "user", "y"}
	if got := header.fieldNames(); !slices.Equal(got, want) {
		t.Fatalf("fieldNames = %v, want %v", got, want)
	}
	for i, name := range want {
		if got, ok := header.fieldName(i); !ok || got != name {
			t.Fatalf("fieldName(%d) = (%q, %v), want (%q, true)", i, got, ok, name)
		}
		if fid := header.fieldID(name); fid != i+1 {
			t.Fatalf("fieldID(%q) = %d, want %d", name, fid, i+1)
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

	if got, want := header.version(), common.UB1(3); got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
	if header.isScalar() {
		t.Fatal("isScalar = true, want false")
	}
	if got, want := int(header.uniqueFields()), 2; got != want {
		t.Fatalf("uniqueFields2 = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentSize(), common.UB4(0x000E); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentOffset(), 294; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}

	want := []string{"short", longKey}
	if got := header.fieldNames(); !slices.Equal(got, want) {
		t.Fatalf("fieldNames length/order mismatch: got %v entries, want %v", len(got), len(want))
	}
	for i, name := range want {
		if got, ok := header.fieldName(i); !ok || got != name {
			t.Fatalf("fieldName(%d) = (%q, %v), want (%q, true)", i, got, ok, name)
		}
		if fid := header.fieldID(name); fid != i+1 {
			t.Fatalf("fieldID(%q) = %d, want %d", name, fid, i+1)
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
		doc  common.B1Array
	}{
		{
			name: "truncated magic",
			doc:  common.B1Array{0xff, 0x4a, 0x5a},
		},
		{
			name: "invalid magic",
			doc:  common.B1Array{0xff, 0x4a, 0x00, 0x01, 0x00, 0x16},
		},
		{
			name: "unsupported version low",
			doc:  common.B1Array{0xff, 0x4a, 0x5a, 0x00, 0x00, 0x16},
		},
		{
			name: "unsupported version high",
			doc:  common.B1Array{0xff, 0x4a, 0x5a, 0x05, 0x00, 0x16},
		},
		{
			name: "truncated scalar tree size",
			doc:  common.B1Array{0xff, 0x4a, 0x5a, 0x01, 0x00, 0x16, 0x00},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newOsonHeader(newOsonBuffer(tc.doc))
			if err == nil {
				t.Fatal("newOsonHeader() error = nil, want failure")
			}
			assertOracleErrorCode(t, err, common.OsonHeaderError)
		})
	}
}

// TestOsonHeader_ScalarTreeSizeUB4 verifies scalar documents honor a UB4-encoded tree segment size.
func TestOsonHeader_ScalarTreeSizeUB4(t *testing.T) {
	doc := buildScalarOsonForTest(common.B1Array{osonOpFalse}, osonFlagTreeSegmentSizeUB4Mask, nil)
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	if got, want := header.treeSegmentOffset(), 10; got != want {
		t.Fatalf("treeSegmentOffset = %d, want %d", got, want)
	}
	if got, want := header.treeSegmentSize(), common.UB4(1); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
}

// TestOsonHeader_ReadHeaderWidthVariants verifies flag-controlled header field widths for v1 and v3 documents.
func TestOsonHeader_ReadHeaderWidthVariants(t *testing.T) {
	v3Flags := common.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB2Mask | osonFlagFieldHeapSizeUB4Mask | osonFlagTreeSegmentSizeUB4Mask)
	v3Doc := common.B1Array{
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
	if got, want := header.treeSegmentSize(), common.UB4(0x40); got != want {
		t.Fatalf("treeSegmentSize = %d, want %d", got, want)
	}
	if got, want := header.tinyNodeCount(), common.UB2(2); got != want {
		t.Fatalf("tinyNodeCount = %d, want %d", got, want)
	}

	v1Flags := common.UB2(osonFlagInlineLeafMask | osonFlagDistinctFieldCountUB4Mask)
	v1Doc := common.B1Array{
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
			osonFlagObjectFieldsUnsortedMask |
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

// TestOsonHeader_RejectsCorruptDictionaryHeaps verifies malformed primary and secondary dictionary heaps are rejected.
func TestOsonHeader_RejectsCorruptDictionaryHeaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(common.B1Array)
	}{
		{
			name: "primary heap size zero with nonzero field count",
			mutate: func(doc common.B1Array) {
				binary.BigEndian.PutUint16(doc[7:], 0)
			},
		},
		{
			name: "primary entry length exceeds heap",
			mutate: func(doc common.B1Array) {
				// First primary heap byte is the length of the "name" field.
				doc[22] = 0x20
			},
		},
		{
			name: "secondary entry too short for long-key tier",
			mutate: func(doc common.B1Array) {
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
			assertOracleErrorCode(t, err, common.OsonHeaderError)
		})
	}
}

// buildScalarOsonForTest creates a minimal v1 scalar OSON document with optional header flags and trailing bytes.
func buildScalarOsonForTest(tree common.B1Array, extraFlags common.UB2, tail common.B1Array) common.B1Array {
	flags := common.UB2(0x0016) | extraFlags
	doc := common.B1Array{0xff, 0x4a, 0x5a, 0x01, byte(flags >> 8), byte(flags)}
	if flags&osonFlagTreeSegmentSizeUB4Mask != 0 {
		doc = append(doc, 0, 0, 0, byte(len(tree)))
	} else {
		doc = append(doc, 0, byte(len(tree)))
	}
	doc = append(doc, tree...)
	doc = append(doc, tail...)
	return doc
}
