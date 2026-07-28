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

package ttc

import (
	"context"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

var validBvcDump1 = []string{
	`"15 01 01 01"`,
}

var validBvcDump2 = []string{
	`"15 01 02 05"`,
}

var validBvcDump3 = []string{
	`"15 01 02 03"`,
}

var invalidBvcDump = []string{
	`"15 01 02 01"`,
}

func makeBvcPayload(dump []string) []byte {
	buf, _ := ExtractBytesFromDump(dump)
	return buf[1:]
}

// TestTTIbvc_GetMsgCode verifies that GetMsgCode returns TTIBVC for tTIbvc message.
func TestTTIbvc_GetMsgCode(t *testing.T) {
	t.Parallel()
	bvc := newTTIbvc().(*tTIbvc)
	if got := bvc.GetMsgCode(); got != TTIBVC {
		t.Errorf("GetMsgCode() = %v, want %v", got, TTIBVC)
	}
}

/*
TestTTIbvc_SetNumberOfColumns ensures that SetNumberOfColumns properly initializes:
- numberOfColumns with the requested column count
- allocates a BitSet of proper size for bitvector tracking
- resets the bvcFound flag, indicating that no valid vector is present yet
*/
func TestTTIbvc_SetNumberOfColumns(t *testing.T) {
	t.Parallel()
	var bvc tTIbvc
	bvc.SetNumberOfColumns(20)
	// Should store the number of columns
	if bvc.numberOfColumns != 20 {
		t.Errorf("SetNumberOfColumns did not set noOfCols, got=%d", bvc.numberOfColumns)
	}
	// Should allocate a bit vector of proper size
	if bvc.bvcColSent == nil || bvc.bvcColSent.Length() < 20 {
		t.Errorf("SetNumberOfColumns did not allocate bitset")
	}
	// Should mark as not found/initialized
	if bvc.bvcFound {
		t.Error("SetNumberOfColumns should reset bvcFound to false")
	}
}

/*
TestTTIbvc_UnMarshalFrom covers the protocol logic for unmarshalling a received BVC (bitvector column) message:
- Validates that valid presence vectors activate the correct bits/columns.
- Asserts the column count matches the received protocol value.
- Verifies that errors are raised on mismatch or on unexpected stream input.
*/
func TestTTIbvc_UnMarshalFrom(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		payload     []byte
		noOfCols    common.UB4
		expectSet   []int
		wantErr     bool
		expectFound bool
	}
	cases := []testCase{
		{
			// Each test sets up the message to marshal, expected number of present columns,
			// and what error/status/columns should result.
			name:        "Valid three columns",
			payload:     makeBvcPayload(validBvcDump1),
			noOfCols:    3,
			expectSet:   []int{0},
			wantErr:     false,
			expectFound: true,
		},
		{
			name:        "Valid three columns",
			payload:     makeBvcPayload(validBvcDump2),
			noOfCols:    3,
			expectSet:   []int{0, 2},
			wantErr:     false,
			expectFound: true,
		},
		{
			name:        "Error on UnmarshalUB2 (empty payload)",
			payload:     []byte{},
			noOfCols:    1,
			expectSet:   nil,
			wantErr:     true,
			expectFound: false,
		},
		{
			name: "Column count mismatch triggers error",
			// UB2 (2 bytes) = 2 (numColsSent), next byte only sets 1 bit
			payload:     makeBvcPayload(invalidBvcDump), // Says 2 set, but only 1 bit set (bit 0)
			noOfCols:    3,
			expectSet:   nil,
			wantErr:     true,
			expectFound: true,
		},
		{
			name:        "Error on UnmarshalUB1 (payload too short)",
			payload:     []byte{1, 1},
			noOfCols:    8,
			expectSet:   nil,
			wantErr:     true,
			expectFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mar := createMarshaller(tc.payload, 0, 0)
			bvc := newTTIbvc().(*tTIbvc)
			// Setup: initialize for N columns, then try to unmarshal the bitvector payload.
			bvc.SetNumberOfColumns(tc.noOfCols)
			err := bvc.UnMarshalFrom(context.Background(), mar)
			if tc.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Log bit state after reading the message.
			t.Logf("Bits after UnMarshalFrom: %s", bvc.bvcColSent.String())
			if len(tc.expectSet) > 0 {
				// Check all expected set bits are present in the vector.
				for _, idx := range tc.expectSet {
					if !bvc.bvcColSent.Get(idx) {
						t.Errorf("expected bit %d to be set", idx)
					}
				}
			}
			// bvcFound should reflect whether a valid/full vector was received.
			if bvc.bvcFound != tc.expectFound {
				t.Errorf("expected bvcFound=%v, got=%v", tc.expectFound, bvc.bvcFound)
			}
		})
	}
}

/*
TestTTIbvc_SetBitVector directly exercises bit vector initialization from server-provided arrays:
- It checks that the columns specified as present set the appropriate bits in bvcColSent.
- It also verifies the logic for handling found/not-found status.
*/
func TestTTIbvc_SetBitVector(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		vec         []byte
		length      int
		noOfCols    common.UB4
		expectSet   []int
		expectFound bool
	}
	cases := []testCase{
		{
			name:        "Set bits from 1 byte vector",
			vec:         []byte{0b10111001},
			length:      1,
			noOfCols:    8,
			expectSet:   []int{0, 3, 4, 5, 7},
			expectFound: true,
		},
		{
			name:        "Zero length disables found",
			vec:         nil,
			length:      0,
			noOfCols:    8,
			expectSet:   []int{},
			expectFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup the bitvector column tracker, and read the vector input for this test case.
			bvc := newTTIbvc().(*tTIbvc)
			bvc.SetNumberOfColumns(tc.noOfCols)
			bvc.SetBitVector(tc.vec, tc.length)
			// Check each expected bit is set in the output.
			for _, idx := range tc.expectSet {
				if !bvc.bvcColSent.Get(idx) {
					t.Errorf("expected bit %d to be set", idx)
				}
			}
			// Should correctly set the found status if data was present.
			if bvc.bvcFound != tc.expectFound {
				t.Errorf("expected bvcFound=%v, got=%v", tc.expectFound, bvc.bvcFound)
			}
		})
	}
}
