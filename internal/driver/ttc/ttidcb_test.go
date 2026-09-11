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

// TestTTIdcb24Constructor verifies that the newTTIdcb24 constructor properly initializes
// the tTIdcb24 structure and its embedded UDS field.
func TestTTIdcb24Constructor(t *testing.T) {
	t.Parallel()
	msg := newTTIdcb24()
	if msg == nil {
		t.Fatal("newTTIdcb returned nil")
	}

	ttidcb24, ok := msg.(*tTIdcb)
	if !ok {
		t.Fatalf("newTTIdcb did not return *tTIdcb, got %T", msg)
	}

	if ttidcb24.newUDS == nil {
		t.Error("newUDS factory is not initialized")
	}
}

// TestTTIdcb_GetMsgCode ensures the GetMsgCode method of tTIdcb
// returns the correct message type code (TTIDCB).
func TestTTIdcb_GetMsgCode(t *testing.T) {
	t.Parallel()
	ttidcb := newTTIdcb().(*tTIdcb)
	got := ttidcb.GetMsgCode()
	if got != TTIDCB {
		t.Errorf("GetMsgCode() = %v, want %v", got, TTIDCB)
	}
}

// TestTTIdcb24UnMarshalFrom_Success tests the UnMarshalFrom method for a tTIdcb24 instance
// using a valid encoded payload. The test asserts successful unmarshalling with no error,
// and checks that numUDS and the first column name match expected values.
func TestTTIdcb24UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	validBuf := makeTtidcbPayload(validTtidcbDump)

	mar := createMarshaller(validBuf, 0, 0)
	ttidcb := newTTIdcb24().(*tTIdcb)
	err := ttidcb.UnMarshalFrom(ctx, mar)
	if err != nil {
		t.Fatalf("UnMarshalFrom (valid path) failed: %v", err)
	}
	expectedNumUDS := common.UB4(2)
	if ttidcb.numUDS != expectedNumUDS {
		t.Errorf("Expected numUDS = %d, got %d", expectedNumUDS, ttidcb.numUDS)
	}
	// Sanity: ensure newUDS factory is initialized
	if ttidcb.newUDS == nil {
		t.Fatal("newUDS factory not initialized")
	}
	// New check: validate colNames slice
	expectedColNames := []common.B1Array{common.StringToB1Array("ID"),
		common.StringToB1Array("NAME")}
	if len(ttidcb.colNames) != len(expectedColNames) {
		t.Errorf("colNames length = %d, want %d", len(ttidcb.colNames), len(expectedColNames))
	} else {
		for i, want := range expectedColNames {
			if !ttidcb.colNames[i].Equals(want) {
				t.Errorf("colNames[%d] = %q, want %q", i, ttidcb.colNames[i], want)
			}
		}
	}
}

// TestTTIdcb24UnMarshalFrom_Fail exercises error paths in tTIdcb24.UnMarshalFrom using injection of
// simulated read errors at various positions in the marshalling sequence.
func TestTTIdcb24UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type faultyTest struct {
		name      string
		failOn    FailOn
		failCount int
	}
	faults := []faultyTest{
		{"Fail on length", failOnReadByte, 1},
		{"Fail on ignore buffer", failOnReadBytes, 1},
		{"Fail on maxSizeOfOneRow", failOnReadByte, 2},
		{"Fail on numUds", failOnReadByte, 3},
		{"Fail on after numUds", failOnReadByte, 4},
		{"Fail on uds unmarshal", failOnReadByte, 5},
		{"Fail on date", failOnReadByte, 77},
		{"Fail on dcb flag", failOnReadByte, 79},
		{"Fail on dcbmdbz", failOnReadByte, 80},
		{"Fail on dcbmnpr", failOnReadByte, 81},
		{"Fail on dcbmxpr", failOnReadByte, 82},
		{"Fail on query compile key", failOnReadByte, 83},
	}
	payload := makeTtidcbPayload(validTtidcbDump)

	for _, tc := range faults {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mar := createMarshaller(payload, tc.failOn, tc.failCount)
			ttidcb := newTTIdcb24().(*tTIdcb)
			err := ttidcb.UnMarshalFrom(ctx, mar)
			if err == nil {
				t.Errorf("expected error, got nil for case: %s", tc.name)
			} else {
				// Confirm the error was injected on purpose
				if !strings.Contains(err.Error(), "simulated read error") {
					t.Errorf("expected error message to contain 'simulated read error', got: %v", err)
				}
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}

// TestTTIdcb_receiveCommon_FromOdny_Fail checks that the fromOdny code path of receiveCommon properly
// propagates simulated read errors for both first and second reads of UB2. Table-driven for easy extension.
func TestTTIdcb_receiveCommon_FromOdny_Fail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ttidcb := newTTIdcb().(*tTIdcb)
	payload := []byte{0x01, 0xAA, 0xBB, 0x00, 0xBB}

	tests := []struct {
		name      string
		failCount int
	}{
		{"fail on fromOdny true : num uds", 1},
		{"fail on fromOdny true : uds unmarshal from", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mar := createMarshaller(payload, failOnReadByte, tc.failCount)
			err := ttidcb.receiveCommon(ctx, mar, true)
			if err == nil {
				t.Fatalf("expected error, got nil (failCount=%d)", tc.failCount)
			}
			// Ensure injected error is found in the returned message
			if !strings.Contains(err.Error(), "simulated read error") {
				t.Errorf("error does not match expected error pattern, got: %v", err)
			}
		})
	}
}

/*
TestTTIdcb_PerColumnUDS_NoAliasing verifies that receiveCommon allocates a fresh UDS
per column (no aliasing). It ensures:
- udsArr holds distinct pointers for each column
- Mutating the first column's base columnName does not affect the second
*/
func TestTTIdcb_PerColumnUDS_NoAliasing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	payload := makeTtidcbPayload(validTtidcbDump)
	mar := createMarshaller(payload, 0, 0)
	ttidcb := newTTIdcb24().(*tTIdcb)

	if err := ttidcb.UnMarshalFrom(ctx, mar); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	if int(ttidcb.numUDS) != len(ttidcb.udsArr) || len(ttidcb.udsArr) != 2 {
		t.Fatalf("udsArr length mismatch: numUDS=%d, len(udsArr)=%d", ttidcb.numUDS, len(ttidcb.udsArr))
	}
	// Distinct objects
	if ttidcb.udsArr[0] == ttidcb.udsArr[1] {
		t.Fatalf("udsArr[0] and udsArr[1] alias the same object")
	}

	// Snapshot names
	name0Before := getColumnNameBytes(ttidcb.udsArr[0])
	name1Before := getColumnNameBytes(ttidcb.udsArr[1])
	if len(name0Before) == 0 || len(name1Before) == 0 {
		t.Fatalf("empty column names: name0=%q name1=%q", string(name0Before), string(name1Before))
	}

	// Mutate first column's base columnName first byte
	mutateBaseColumnNameFirstByte(ttidcb.udsArr[0], 'X')

	// Ensure second column unaffected
	name1After := getColumnNameBytes(ttidcb.udsArr[1])
	if len(name1After) == 0 {
		t.Fatalf("second column name became empty after mutation")
	}
	if name1After[0] == 'X' {
		t.Fatalf("aliasing detected: modifying first column mutated second column's name (got %q)", string(name1After))
	}

	// Sanity: first column did change
	name0After := getColumnNameBytes(ttidcb.udsArr[0])
	if name0After[0] != 'X' {
		t.Fatalf("expected first column name to start with 'X', got %q", string(name0After))
	}
}

// getColumnNameBytes obtains the base columnName slice from any supported UDS.
func getColumnNameBytes(u common.UnMarshallable) []byte {
	switch v := u.(type) {
	case *tTIuds:
		return v.columnName
	case *tTIuds17:
		if v.tTIuds != nil {
			return v.tTIuds.columnName
		}
	case *tTIuds20:
		if v.tTIuds17 != nil && v.tTIuds17.tTIuds != nil {
			return v.tTIuds17.tTIuds.columnName
		}
	case *tTIuds24:
		if v.tTIuds20 != nil && v.tTIuds20.tTIuds17 != nil && v.tTIuds20.tTIuds17.tTIuds != nil {
			return v.tTIuds20.tTIuds17.tTIuds.columnName
		}
	}
	return nil
}

// mutateBaseColumnNameFirstByte mutates the first byte of the base columnName if present.
func mutateBaseColumnNameFirstByte(u common.UnMarshallable, b byte) {
	switch v := u.(type) {
	case *tTIuds:
		if len(v.columnName) > 0 {
			v.columnName[0] = b
		}
	case *tTIuds17:
		if v.tTIuds != nil && len(v.tTIuds.columnName) > 0 {
			v.tTIuds.columnName[0] = b
		}
	case *tTIuds20:
		if v.tTIuds17 != nil && v.tTIuds17.tTIuds != nil && len(v.tTIuds17.tTIuds.columnName) > 0 {
			v.tTIuds17.tTIuds.columnName[0] = b
		}
	case *tTIuds24:
		if v.tTIuds20 != nil && v.tTIuds20.tTIuds17 != nil && v.tTIuds20.tTIuds17.tTIuds != nil && len(v.tTIuds20.tTIuds17.tTIuds.columnName) > 0 {
			v.tTIuds20.tTIuds17.tTIuds.columnName[0] = b
		}
	}
}
