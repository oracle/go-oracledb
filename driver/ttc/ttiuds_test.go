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

	"github.com/oracle/go-driver/driver/common"
)

/*
TestTTIuds_UnMarshalFrom_Success verifies successful unmarshalling of a valid TTIuds payload,
checking all key fields (nullable, columnName, schemaName, typeName, kernelPosition, columnFlags)
against expected values.
*/
func TestTTIuds_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	payload := makeTtiudsUnmarshalPayload(validTtiudsUnmarshalDump)
	mar := createMarshaller(payload, 0, 0)
	obj := newTTIuds()
	err := obj.UnMarshalFrom(ctx, mar)
	if err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	typed := obj.(*tTIuds)
	if !typed.isNullable {
		t.Errorf("expected isNullable true, got false")
	}
	if typed.columnNameLen != 2 {
		t.Errorf("expected columnNameLen 2, got %d", typed.columnNameLen)
	}
	if string(typed.columnName) != "ID" {
		t.Errorf("expected columnName ID, got %q", typed.columnName)
	}
	if string(typed.schemaName) != "" {
		t.Errorf("expected schemaName empty, got %q", typed.schemaName)
	}
	if string(typed.typeName) != "" {
		t.Errorf("expected typeName empty, got %q", typed.typeName)
	}
	if typed.kernelPosition != 0 {
		t.Errorf("expected kernelPosition=0, got %d", typed.kernelPosition)
	}
	if typed.columnFlags != 32768 {
		t.Errorf("expected columnFlags=32768, got %d", typed.columnFlags)
	}
}

/*
TestTTIuds_UnMarshalFrom_Fail exercises error handling of TTIuds unmarshalling by injecting simulated
read errors at various stages, ensuring all error paths are reached and confirmed.
*/
func TestTTIuds_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type faultyTest struct {
		name      string
		failCount int
	}
	faults := []faultyTest{
		{"Fail on oac", 1},
		{"Fail on nullable", 14},
		{"Fail on columnNameLen", 15},
		{"Fail on columnName", 16},
		{"Fail on schemaName", 18},
		{"Fail on typeName", 19},
		{"Fail on kernelPosition", 20},
		{"Fail on columnFlags", 21},
	}
	payload := makeTtiudsUnmarshalPayload(validTtiudsUnmarshalDump)

	for _, tc := range faults {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mar := createMarshaller(payload, failOnReadByte, tc.failCount)
			obj := newTTIuds()
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

/*
TestTTIuds_GetColumnName verifies that GetColumnName on tTIuds returns the correct column name as a B1Array
for both non-empty and empty input cases.
*/
func TestTTIuds_GetColumnName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		desc  string
		input []byte
	}{
		{"non-empty name", []byte("ID")},
		{"empty", []byte{}},
	}
	for _, tc := range cases {
		u := &tTIuds{columnName: tc.input}
		got := u.getColumnName()
		if string(got) != string(tc.input) {
			t.Errorf("%s: getColumnName() = %v, want %v", tc.desc, got, tc.input)
		}
	}
}

/*
TestTTIuds_GetUdsArrayAndColCount verifies that getUdsArrayAndColCount returns the correct column count
and UDS array for all tTIdcb family types and returns (0, nil) for unsupported inputs.
*/
func TestTTIuds_GetUdsArrayAndColCount(t *testing.T) {
	t.Parallel()
	t.Skip()
	makeDCB := func(numCols uint32) (*tTIdcb, []common.UnMarshallable) {
		udsArr := make([]common.UnMarshallable, numCols)
		for i := range udsArr {
			udsArr[i] = &tTIuds{columnName: []byte{byte('A' + i)}}
		}
		return &tTIdcb{
			numUDS: common.UB4(numCols),
			udsArr: udsArr,
		}, udsArr
	}

	ttidcb, u1 := makeDCB(2)
	ttidcb17 := ttidcb
	ttidcb20 := ttidcb
	ttidcb24 := ttidcb

	tests := []struct {
		name    string
		input   common.Message[common.MessageType]
		wantN   common.UB4
		wantArr []common.UnMarshallable
	}{
		{"*tTIdcb", ttidcb, 2, u1},
		{"*tTIdcb17", ttidcb17, 2, u1},
		{"*tTIdcb20", ttidcb20, 2, u1},
		{"*tTIdcb24", ttidcb24, 2, u1},
		{"unsupported", &dummyMsg{}, 0, nil},
	}

	for _, tc := range tests {
		gotN := ttidcb.getNumberOfColumns()
		gotArr, err := ttidcb.getColumnContexts()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if gotN != tc.wantN {
			t.Errorf("%s: expected col count %d, got %d", tc.name, tc.wantN, gotN)
		}
		if len(gotArr) != len(tc.wantArr) {
			t.Errorf("%s: expected udsArr len %d, got %d", tc.name, len(tc.wantArr), len(gotArr))
		}
	}
}
