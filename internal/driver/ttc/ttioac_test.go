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
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestTTIoac_NewTTIoac checks correct field setup for different input types.
func TestTTIoac_NewTTIoac(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		inputType        int16
		inputMaxlen      common.UB4
		expectedDataType common.UB1
		expectedMaxlen   common.UB4
	}{
		{"Varchar", DtyChr, 128, common.UB1(DtyChr), 128},
		{"Rowid", DtyRdd, 4, common.UB1(DtyRiD), 4},
		{"Number", DtyNum, 32, common.UB1(DtyNum), 32},
		{"Binary", DtyVbi, 255, common.UB1(DtyBin), 255},
		{"Cursor", DtyRSet, 0, common.UB1(DtyCur), 4},
		{"Unhandled", 111, 44, common.UB1(111), 44},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := newTTIoac(tt.inputType, tt.inputMaxlen)

			if obj.dataType != tt.expectedDataType {
				t.Errorf("Expected dataType %v, got %v", tt.expectedDataType, obj.dataType)
			}
			if obj.maxLength != tt.expectedMaxlen {
				t.Errorf("Expected maxLength %v, got %v", tt.expectedMaxlen, obj.maxLength)
			}
		})
	}
}

// TestTTIoac_UnMarshalFrom_Success tests that UnMarshalFrom succeeds on a valid TTC payload.
func TestTTIoac_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dump []string
	}{
		{"valid chr unmarshal", validTtioacChrUnmarshalDump},
		{"valid num unmarshal", validTtioacNumUnmarshalDump},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := makeTtioacUnmarshalPayload(tc.dump)
			mar := createMarshaller(payload, 0, 0)
			obj := &tTIoac{}
			err := obj.UnMarshalFrom(context.Background(), mar)
			if err != nil {
				t.Fatalf("UnMarshalFrom failed: %v", err)
			}
			switch tc.name {
			case "valid chr unmarshal":
				// Check selected fields for validTtioacChrUnmarshalDump (should decode to DtyChr)
				if obj.dataType != common.UB1(DtyChr) {
					t.Errorf("expected dataType DtyChr (%d), got %d", DtyChr, obj.dataType)
				}
				if obj.maxLength != 4000 {
					t.Errorf("expected maxLength 40000, got %d", obj.maxLength)
				}
			case "valid num unmarshal":
				// Check selected fields for validTtioacNumUnmarshalDump (should decode to DtyNum)
				if obj.dataType != common.UB1(DtyNum) {
					t.Errorf("expected dataType DtyNum (%d), got %d", DtyNum, obj.dataType)
				}
				if obj.scale != common.SB1(NumberScaleFloatSentinel) {
					t.Errorf("expected scale %d, got %d", NumberScaleFloatSentinel, obj.scale)
				}
				if obj.maxLength != 22 {
					t.Errorf("expected maxLength 127, got %d", obj.maxLength)
				}
			}
		})
	}
}

// TestTTIoac_UnMarshalFrom_Fail covers error scenarios using a faulty data buffer
func TestTTIoac_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type faultyTest struct {
		name      string
		failCount int
	}
	tests := []faultyTest{
		{"Fail on dataType read", 1},
		{"Fail on flags read", 2},
		{"Fail on precision read", 3},
		{"Fail on scale read", 4},
		{"Fail on maxLength read", 5},
		{"Fail on nbArrayElements read", 6},
		{"Fail on flagsContinuation read", 7},
		{"Fail on toid read", 8},
		{"Fail on versionNumber read", 9},
		{"Fail on characterSetID read", 10},
		{"Fail on characterSetForm read", 11},
		{"Fail on codepointLengthLimit read", 12},
		{"Fail on collationId read", 13},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			payload := makeTtioacUnmarshalPayload(validTtioacChrUnmarshalDump)
			mar := createMarshaller(payload, failOnReadByte, tc.failCount)
			obj := &tTIoac{}
			err := obj.UnMarshalFrom(ctx, mar)
			if err == nil {
				t.Errorf("expected error, got nil for case: %s", tc.name)
			} else {
				if !strings.Contains(err.Error(), "simulated read error") {
					t.Errorf("expected error message to contain 'simulated read error', got: %v", err)
				}
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}

// TestTTIoac_MarshalTo_Success tests marshalling of TTIoac.
func TestTTIoac_MarshalTo_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	type testCase struct {
		name         string
		accessorType int16
		maxlen       common.UB4
		charsetID    common.UB2
		charsetForm  common.UB1
		precision    common.UB1
		scale        common.SB1
		maxLength    common.UB4
		codepointLen common.UB4
		collationID  common.UB4
		nbArrayElts  common.UB4
		versionNum   common.UB2
		flagsCont    common.UB8
		toid         []byte
	}
	testCases := []testCase{
		{
			name:         "varchar",
			accessorType: DtyChr,
			maxlen:       42,
			charsetID:    common.UB2(1789),
			charsetForm:  common.UB1(2),
			precision:    common.UB1(5),
			scale:        common.SB1(10),
			maxLength:    common.UB4(42),
			codepointLen: common.UB4(100),
			collationID:  common.UB4(56),
			nbArrayElts:  common.UB4(7),
			versionNum:   common.UB2(2),
			flagsCont:    common.UB8(9),
			toid:         []byte{1, 2, 3},
		},
		{
			name:         "dtynum",
			accessorType: DtyNum,
			maxlen:       22,
			charsetID:    common.UB2(0),
			charsetForm:  common.UB1(0),
			precision:    common.UB1(2),
			scale:        common.SB1(7),
			maxLength:    common.UB4(22),
			codepointLen: common.UB4(88),
			collationID:  common.UB4(9),
			nbArrayElts:  common.UB4(3),
			versionNum:   common.UB2(1),
			flagsCont:    common.UB8(0),
			toid:         []byte{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obj := newTTIoac(tc.accessorType, tc.maxlen)
			obj.characterSetID = tc.charsetID
			obj.characterSetForm = tc.charsetForm
			obj.precision = tc.precision
			obj.scale = tc.scale
			obj.maxLength = tc.maxLength
			obj.codepointLengthLimit = tc.codepointLen
			obj.collationID = tc.collationID
			obj.nbArrayElements = tc.nbArrayElts
			obj.versionNumber = tc.versionNum
			obj.flagsContinuation = tc.flagsCont
			if tc.toid != nil {
				obj.toid = (*common.B1Array)(&tc.toid)
			}
			_, mar := NewMarshalEngineTest(common.BIG_ENDIAN, Universal, Universal, 1024)
			err := obj.MarshalTo(ctx, mar)
			if err != nil {
				t.Fatalf("MarshalTo failed for %s case: %v", tc.name, err)
			}
		})
	}
}

// TestTTIoac_MarshalTo_Fail tests failure scenarios when marshalling TTIoac due to write errors.
func TestTTIoac_MarshalTo_Fail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	orig := newTTIoac(DtyChr, 42)
	orig.characterSetID = 1789
	orig.characterSetForm = 2
	orig.precision = 5
	orig.scale = 10
	orig.maxLength = 42
	orig.codepointLengthLimit = 100
	orig.collationID = 56
	orig.nbArrayElements = 7
	orig.versionNumber = 2
	orig.flagsContinuation = 9
	b := []byte{1, 2, 3}
	orig.toid = (*common.B1Array)(&b)

	type failCase struct {
		name      string
		failCount int
		failType  FailOn
	}
	cases := []failCase{
		{"Fail on Write dataType", 1, failOnWriteByte},
		{"Fail on Write flag", 2, failOnWriteByte},
		{"Fail on Write precision", 3, failOnWriteByte},
		{"Fail on Write scale", 4, failOnWriteByte},
		{"Fail on Write maxLength", 1, failOnWriteBytes},
		{"Fail on Write nbArrayElements", 2, failOnWriteBytes},
		{"Fail on Write flagsContinuation", 3, failOnWriteBytes},
		{"Fail on Write toid", 4, failOnWriteBytes},
		{"Fail on Write versionNumber", 6, failOnWriteBytes},
		{"Fail on Write characterSetID", 7, failOnWriteBytes},
		{"Fail on Write characterSetForm", 6, failOnWriteByte},
		{"Fail on Write codepointLengthLimit", 8, failOnWriteBytes},
		{"Fail on Write collationId", 9, failOnWriteBytes},
		// Explicitly fail on UB2(p.scale) for DtyNum
		{"Fail on MarshalUB2 scale DtyNum", 1, failOnWriteBytes},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := make([]byte, 2048)
			mar := createMarshaller(payload, tc.failType, tc.failCount)
			var err error
			if tc.name == "Fail on MarshalUB2 scale DtyNum" {
				// DtyNum config to enter MarshalUB2 scale block
				dtyNum := newTTIoac(DtyNum, 22)
				dtyNum.scale = common.SB1(7)
				err = dtyNum.MarshalTo(ctx, mar)
			} else {
				err = orig.MarshalTo(ctx, mar)
			}
			if err == nil {
				t.Errorf("expected error, got nil for case: %s", tc.name)
			} else {
				if !strings.Contains(err.Error(), "simulated write error") {
					t.Errorf("expected error message to contain 'simulated write error', got: %v", err)
				}
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}

// TestTTIoac_Setters tests the setter methods of TTIoac.
func TestTTIoac_Setters(t *testing.T) {
	t.Parallel()
	obj := &tTIoac{}
	obj.flagsContinuation = 2
	obj.addFlagsContinuation(8)
	if obj.flagsContinuation != 10 {
		t.Errorf("addFlagsContinuation failed: expected flagsContinuation=10, got %d", obj.flagsContinuation)
	}
}
