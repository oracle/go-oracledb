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
	"testing"

	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestNewNodeAt_RejectsMissingContext verifies node construction fails when either the buffer or parsed header is missing.
func TestNewNodeAt_RejectsMissingContext(t *testing.T) {
	if _, err := newNodeAt(nil, &osonHeader{}, 0); err == nil {
		t.Fatal("newNodeAt(nil buffer) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
	}
	if _, err := newNodeAt(newOsonBuffer(drvCommon.B1Array{osonOpTrue}), nil, 0); err == nil {
		t.Fatal("newNodeAt(nil header) error = nil, want failure")
	} else {
		assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
	}
}

// TestNewNodeAt_ResolvesRedirectChainsAndRejectsCycles verifies forwarding is
// resolved until a concrete node is reached and cannot loop indefinitely.
func TestNewNodeAt_ResolvesRedirectChainsAndRejectsCycles(t *testing.T) {
	t.Run("redirect chain", func(t *testing.T) {
		// Low-level forwarding layout, not an OSON source fixture.
		buffer := newOsonBuffer(drvCommon.B1Array{
			osonOpUpdateForwardUB2, 0x00, 0x00,
			osonOpUpdateForwardUB2, 0x00, 0x03,
			osonOpTrue,
		})
		header := &osonHeader{
			treeSegmentByteLength:          3,
			extendedTreeSegmentStartOffset: 3,
			extendedTreeSegmentByteLength:  4,
		}

		node, err := newNodeAt(buffer, header, 0)
		if err != nil {
			t.Fatalf("newNodeAt() error = %v", err)
		}
		value, err := node.GetValue(drvCommon.JSONOptDefault)
		if err != nil {
			t.Fatalf("GetValue() error = %v", err)
		}
		if got, want := value, true; got != want {
			t.Fatalf("GetValue() = %v, want %v", got, want)
		}
	})

	t.Run("redirect cycle", func(t *testing.T) {
		// Low-level forwarding layout, not an OSON source fixture.
		buffer := newOsonBuffer(drvCommon.B1Array{
			osonOpUpdateForwardUB2, 0x00, 0x00,
			osonOpUpdateForwardUB2, 0x00, 0x00,
		})
		header := &osonHeader{
			treeSegmentByteLength:          3,
			extendedTreeSegmentStartOffset: 3,
			extendedTreeSegmentByteLength:  3,
		}

		_, err := newNodeAt(buffer, header, 0)
		if err == nil {
			t.Fatal("newNodeAt() error = nil, want forwarding-cycle failure")
		}
		assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
	})
}

// TestNewNodeAt_RejectsUpdateHeaderOffset verifies a node offset cannot point
// into the V2 update header rather than either tree segment.
func TestNewNodeAt_RejectsUpdateHeaderOffset(t *testing.T) {
	buffer := newOsonBuffer(sampleUpdatedTinyScalar.oson)
	header, err := newOsonHeader(buffer)
	if err != nil {
		t.Fatalf("newOsonHeader() error = %v", err)
	}
	updateOffset := header.treeSegmentOffset() + int(header.treeSegmentByteLength)

	_, err = newNodeAt(buffer, header, updateOffset)
	if err == nil {
		t.Fatal("newNodeAt() error = nil, want update-header offset failure")
	}
	assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
}

// TestNodeOffsets_CoverAddressWidths verifies child offsets use the primary
// tree address space and relative offsets preserve signed deltas.
func TestNodeOffsets_CoverAddressWidths(t *testing.T) {
	buffer := newOsonBuffer(drvCommon.B1Array{
		0x00, 0x02,
		0x00, 0x00, 0x00, 0x03,
		0xFF, 0xF0,
		0xFF, 0xFF, 0xFF, 0xF0,
	})
	header := &osonHeader{
		treeSegmentStartOffset:         10,
		treeSegmentByteLength:          64,
		extendedTreeSegmentStartOffset: 90,
		extendedTreeSegmentByteLength:  64,
	}

	if got, err := readChildOffsetAt(buffer, header, 95, 0, osonUB2Size); err != nil || got != 12 {
		t.Fatalf("UB2 primary offset = %d, %v; want 12, nil", got, err)
	}
	if got, err := readChildOffsetAt(buffer, header, 95, 2, osonUB4Size); err != nil || got != 13 {
		t.Fatalf("UB4 primary offset = %d, %v; want 13, nil", got, err)
	}

	header.flags = osonFlagRelativeOffsetsMask
	if got, err := readChildOffsetAt(buffer, header, 30, 6, osonUB2Size); err != nil || got != 14 {
		t.Fatalf("relative UB2 offset = %d, %v; want 14, nil", got, err)
	}
	if got, err := readChildOffsetAt(buffer, header, 30, 8, osonUB4Size); err != nil || got != 14 {
		t.Fatalf("relative UB4 offset = %d, %v; want 14, nil", got, err)
	}
	if _, err := readRelativeChildOffset(buffer, 0, 3); err == nil {
		t.Fatal("readRelativeChildOffset() error = nil, want unsupported-width failure")
	}
}

// TestParse_RejectsOutOfRangeRelativeChildOffset verifies a relative offset
// derived from an Oracle-produced fixture cannot escape the primary tree.
func TestParse_RejectsOutOfRangeRelativeChildOffset(t *testing.T) {
	doc := sampleRelativeOffsets.cloneOSON()
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatal(err)
	}
	rootOffset := header.treeSegmentOffset()
	// The root object stores [opcode][count][three UB1 FIDs][UB2 offsets...].
	binary.BigEndian.PutUint16(doc[rootOffset+5:], 0x7fff)

	root, err := Parse(doc)
	if err == nil {
		_, err = root.GetValue(drvCommon.JSONOptDefault)
	}
	if err == nil {
		t.Fatal("GetValue() error = nil, want out-of-range relative-offset failure")
	}
	assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
}

// TestRedirectedNodeOffset_ValidatesEveryMarker verifies redirect forms either
// resolve within the extended tree or fail before decoding an invalid address.
func TestRedirectedNodeOffset_ValidatesEveryMarker(t *testing.T) {
	buffer := newOsonBuffer(drvCommon.B1Array{
		osonOpUpdateForwardUB4, 0x00, 0x00, 0x00, 0x00,
		osonOpTrue,
	})
	header := &osonHeader{
		treeSegmentStartOffset:         0,
		treeSegmentByteLength:          5,
		extendedTreeSegmentStartOffset: 5,
		extendedTreeSegmentByteLength:  1,
	}
	if got, redirected, err := redirectedNodeOffset(buffer, header, 0, osonOpUpdateForwardUB4); err != nil || !redirected || got != 5 {
		t.Fatalf("UB4 redirect = %d, %t, %v; want 5, true, nil", got, redirected, err)
	}
	if _, _, err := redirectedNodeOffset(buffer, header, 0, osonOpUpdateOverflow); err == nil {
		t.Fatal("overflow redirect error = nil, want missing-map failure")
	}
	if _, _, err := redirectedNodeOffset(buffer, header, 0, osonOpUpdateOversizeReserved); err == nil {
		t.Fatal("reserved-growth redirect error = nil, want unsupported-opcode failure")
	}
}

// TestParse_RejectsInvalidUpdateTargets verifies update redirects remain
// inside the declared extended tree segment.
func TestParse_RejectsInvalidUpdateTargets(t *testing.T) {
	tests := []struct {
		name   string
		sample osonSample
		mutate func(drvCommon.B1Array, *osonHeader)
		want   oracleErrors.ErrorCode
	}{
		{"overflow map target", sampleUpdatedOverflow, func(doc drvCommon.B1Array, header *osonHeader) {
			updateOffset := header.treeSegmentOffset() + int(header.treeSegmentByteLength)
			binary.BigEndian.PutUint16(doc[updateOffset+18:], 0xffff)
		}, oracleErrors.OsonHeaderError},
		{"inline UB2 target", sampleUpdatedForwardUB2, func(doc drvCommon.B1Array, header *osonHeader) {
			for offset := header.treeSegmentOffset(); offset < header.treeSegmentOffset()+int(header.treeSegmentByteLength); offset++ {
				if doc[offset] == byte(osonOpUpdateForwardUB2) {
					binary.BigEndian.PutUint16(doc[offset+1:], 0xffff)
					return
				}
			}
			t.Fatal("fixture does not contain an UB2 forwarding opcode")
		}, oracleErrors.OsonParsingError},
		{"inline UB4 target", sampleUpdatedForwardUB4, func(doc drvCommon.B1Array, header *osonHeader) {
			for offset := header.treeSegmentOffset(); offset < header.treeSegmentOffset()+int(header.treeSegmentByteLength); offset++ {
				if doc[offset] == byte(osonOpUpdateForwardUB4) {
					binary.BigEndian.PutUint32(doc[offset+1:], 0xffffffff)
					return
				}
			}
			t.Fatal("fixture does not contain an UB4 forwarding opcode")
		}, oracleErrors.OsonParsingError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := test.sample.cloneOSON()
			header, err := newOsonHeader(newOsonBuffer(doc))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(doc, header)
			root, err := Parse(doc)
			if err == nil {
				_, err = root.GetValue(drvCommon.JSONOptNumberAsString)
			}
			if err == nil {
				t.Fatal("GetValue() error = nil, want invalid-forward-target failure")
			} else {
				assertOracleErrorCode(t, err, test.want)
			}
		})
	}
}

// TestParse_RejectsForwardingCycle verifies public materialization rejects a
// cycle introduced into the extended tree of an Oracle-produced update image.
func TestParse_RejectsForwardingCycle(t *testing.T) {
	doc := sampleUpdatedForwardUB2.cloneOSON()
	header, err := newOsonHeader(newOsonBuffer(doc))
	if err != nil {
		t.Fatal(err)
	}
	forwardOffset := -1
	for offset := header.treeSegmentOffset(); offset < header.treeSegmentOffset()+int(header.treeSegmentByteLength); offset++ {
		if doc[offset] == byte(osonOpUpdateForwardUB2) {
			forwardOffset = int(binary.BigEndian.Uint16(doc[offset+1:]))
			break
		}
	}
	if forwardOffset < 0 {
		t.Fatal("fixture does not contain an UB2 forwarding opcode")
	}
	extendedOffset := header.extendedTreeSegmentStartOffset + forwardOffset
	doc[extendedOffset] = byte(osonOpUpdateForwardUB2)
	binary.BigEndian.PutUint16(doc[extendedOffset+1:], uint16(forwardOffset))

	root, err := Parse(doc)
	if err == nil {
		_, err = root.GetValue(drvCommon.JSONOptDefault)
	}
	if err == nil {
		t.Fatal("GetValue() error = nil, want forwarding-cycle failure")
	}
	assertOracleErrorCode(t, err, oracleErrors.OsonParsingError)
}

// TestNode_ReadHelpersRejectMalformedInput covers error exits that
// are difficult to reach through a complete document.
func TestNode_ReadHelpersRejectMalformedInput(t *testing.T) {
	t.Parallel()

	buf := newOsonBuffer(drvCommon.B1Array{0x00, 0x00, 0x00, 0x00})
	for _, opcode := range []drvCommon.UB1{
		osonOpArrayType | osonOpChildCountUB1,
		osonOpArrayType | osonOpChildCountUB2,
		osonOpArrayType | osonOpChildCountUB4,
	} {
		if _, _, err := readContainerCountAt(buf, len(buf.data), opcode); err == nil {
			t.Fatalf("readContainerCountAt(%#x) error = nil, want truncation", opcode)
		}
	}
	if _, _, err := readContainerCountAt(buf, 0, osonOpArrayType|osonOpChildDelegateForm); err == nil {
		t.Fatal("readContainerCountAt(delegate) error = nil, want unsupported encoding")
	}

	header := &osonHeader{treeSegmentStartOffset: 0, treeSegmentByteLength: 4}
	for _, width := range []int{osonUB2Size, osonUB4Size} {
		if _, err := readChildOffsetAt(buf, header, 0, len(buf.data), width); err == nil {
			t.Fatalf("readChildOffsetAt(width=%d) error = nil, want truncation", width)
		}
	}
	if _, err := readChildOffsetAt(buf, header, 0, 0, 1); err == nil {
		t.Fatal("readChildOffsetAt(width=1) error = nil, want unsupported width")
	}
	if _, err := readRelativeChildOffset(buf, 0, 1); err == nil {
		t.Fatal("readRelativeChildOffset(width=1) error = nil, want unsupported width")
	}
}

// TestNode_RedirectReadsRejectTruncatedPayloads covers both inline
// forwarding address widths when their payload is incomplete.
func TestNode_RedirectReadsRejectTruncatedPayloads(t *testing.T) {
	t.Parallel()
	header := &osonHeader{}
	for _, test := range []struct {
		name string
		op   drvCommon.UB1
		data drvCommon.B1Array
	}{
		{"UB2", osonOpUpdateForwardUB2, drvCommon.B1Array{osonOpUpdateForwardUB2, 0}},
		{"UB4", osonOpUpdateForwardUB4, drvCommon.B1Array{osonOpUpdateForwardUB4, 0, 0, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := redirectedNodeOffset(newOsonBuffer(test.data), header, 0, test.op); err == nil {
				t.Fatal("redirectedNodeOffset() error = nil, want truncated-payload failure")
			}
		})
	}
}
