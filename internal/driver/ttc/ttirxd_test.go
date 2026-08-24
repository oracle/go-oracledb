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
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

var validRow1Dump = []string{
	`"07 02 C1 02 03 61 62 63"`,
}

var validBvcRxdDump = []string{
	`".....abc 07 02 C1 02 03 61 62 63"`,
	`".....xyz 07 02 C1 03 03 78 79 7A"`,
	`"........ 15 01 01 01 07 02 C1 04"`,
	`"........ 15 01 01 01 07 02 C1 05"`,
	`".....xyz 07 02 C1 06 03 61 62 63"`,
	`"....-... 08"`,
}

// ID	Name	Address
// 1	abc		street 1
// 2	xyz		street 1
// 3	xyz		street 1
// 4	xyz		street 2
// 5	abc		street 2
var validBvcRxd3ColDump = []string{
	`"........ 07 02"`,
	`"...abc.s C1 02 03 61 62 63 08 73"`,
	`"treet.1. 74 72 65 65 74 20 31 07"`,
	`"....xyz. 02 C1 03 03 78 79 7A 08"`,
	`"street.1 73 74 72 65 65 74 20 31"`,
	`"........ 15 01 01 01 07 02 C1 04"`,
	`"........ 15 01 02 05 07 02 C1 05"`,
	`".street. 08 73 74 72 65 65 74 20"`,
	`"2....... 32 15 01 02 03 07 02 C1"`,
	`"..abc... 06 03 61 62 63 08"`,
}

// makeRowPayload converts a string dump representing row data to a byte slice payload suitable for unmarshalling tests.
// It uses ExtractBytesFromDump to generate the byte array, skipping the header byte (buf[1:] as data starts from index 1).
func makeRowPayload(dump []string) []byte {
	buf, _ := ExtractBytesFromDump(dump)
	return buf[1:]
}

// rowToBytes converts a slice of common.B1Array (used for row data) to a slice of byte slices.
// Primarily used to compare row results in tests.
func rowToBytes(row []common.B1Array) [][]byte {
	out := make([][]byte, len(row))
	for i, arr := range row {
		out[i] = []byte(arr)
	}
	return out
}

// TestTTIrxd_GetMsgCode verifies that GetMsgCode returns TTIRXD for tTIrxd message.
func TestTTIrxd_GetMsgCode(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)
	if got := rxd.GetMsgCode(); got != TTIRXD {
		t.Errorf("GetMsgCode() = %v, want %v", got, TTIRXD)
	}
}

// TestTTIrxd_MarshalTo_Success verifies that MarshalTo writes the expected bytes:
// - For nil bind value: writes 0 (byte)
// - For non-nil bind value: writes CLR short form (len + bytes)
func TestTTIrxd_MarshalTo_Success(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)

	// Encoded bind data set before calling MarshalTo (nil, then 3-byte value)
	val := common.B1Array{0x01, 0x02, 0x03}
	rxd.setBindValues([]common.B1Array{nil, val})

	// Use ArrayBasedDataBuffer via NewMarshalEngineTest so we can inspect bytes directly
	buf, eng := NewMarshalEngineTest(session.BIG_ENDIAN, B2, Universal, 64)

	err := rxd.MarshalTo(context.Background(), eng)
	if err != nil {
		t.Fatalf("MarshalTo returned error: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	want := []byte{0, 0x03, 0x01, 0x02, 0x03}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MarshalTo wrote unexpected bytes.\nGot:  %v\nWant: %v", got, want)
	}
}

func TestTTIrxd_MarshalTo_BlobLocatorPrefix(t *testing.T) {
	t.Parallel()

	rxd := newTTIrxd().(*tTIrxd)
	locator := common.B1Array{0x01, 0x02, 0x03}
	rxd.setBindValues([]common.B1Array{locator})
	rxd.setBindOACs([]common.Marshallable{newTTIOacBlobBind(common.UB4(len(locator)))})
	buf, engine := NewMarshalEngineTest(session.BIG_ENDIAN, B2, Universal, 64)

	if err := rxd.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo returned error: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	want := []byte{1, 3, 3, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MarshalTo BLOB bytes = %v, want %v", got, want)
	}
}

// TestTTIrxd_MarshalTo_FailOnNullIndicator simulates a failure when writing the null indicator byte.
// Uses FaultyArrayBasedDataBuffer (via createMarshaller) to fail WriteByte on first call.
func TestTTIrxd_MarshalTo_FailOnNullIndicator(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setBindValues([]common.B1Array{nil})

	// Fail the first WriteByteWithContext call (which writes the null indicator)
	mar := createMarshaller(make([]byte, 8), failOnWriteByte, 1)

	err := rxd.MarshalTo(context.Background(), mar)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var se oracleErrors.SQLError
	if !errors.As(err, &se) {
		t.Fatalf("expected error implementing SQLError, got: %T: %v", err, err)
	}
	if se.ErrorCode() != string(oracleErrors.FailMarshal) {
		t.Errorf("unexpected error code. Got %s, want %s", se.ErrorCode(), oracleErrors.FailMarshal)
	}
}

// TestTTIrxd_MarshalTo_FailOnCLRDataWrite simulates a failure when writing CLR data bytes.
// Uses FaultyArrayBasedDataBuffer to fail WriteBytes on first call, after the length byte is written.
func TestTTIrxd_MarshalTo_FailOnCLRDataWrite(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setBindValues([]common.B1Array{{0xAA, 0xBB, 0xCC}})

	// First WriteBytesWithContext call should fail (after successful length byte write)
	mar := createMarshaller(make([]byte, 8), failOnWriteBytes, 1)

	err := rxd.MarshalTo(context.Background(), mar)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var se oracleErrors.SQLError
	if !errors.As(err, &se) {
		t.Fatalf("expected error implementing SQLError, got: %T: %v", err, err)
	}
	if se.ErrorCode() != string(oracleErrors.FailMarshal) {
		t.Errorf("unexpected error code. Got %s, want %s", se.ErrorCode(), oracleErrors.FailMarshal)
	}
}

/*
TestTTIrxd_BvcOnFirstRow_ReturnsError checks that an error is returned when a Bit Vector for Columns (BVC)
is present on the first row, which is prohibited by the RXD protocol. The test asserts that the error is detected
and has the expected message content.
*/
func TestTTIrxd_BvcOnFirstRow_ReturnsError(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)
	rxd.SetNumberOfColumns(2)
	rxd.SetRowCount(0)
	rxd.setColumnContexts([]ColumnContext{{DataType: DtyVCS}, {DataType: DtyVCS}})
	bitset := common.NewBitSet(2)
	bitset.Set(0, true)
	bitset.Set(1, true)
	rxd.SetBvcState(bitset, true)
	// This is a valid two-column payload, so only the missing previous row can reject it.
	mar := createMarshaller([]byte{1, 'a', 1, 'b'}, 0, 0)
	err := rxd.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatalf("Expected error for BVC on first row, got <nil>")
	}
	expError := "Failed to unmarshal message: Row transfer data message"
	if got := err.Error(); got == "" || !contains(got, expError) {
		t.Errorf("Unexpected error. Got: %v, Want substring: %v", got, expError)
	}
}

// TestTTIrxd_Setters verifies the setter methods for tTIrxd.
// It ensures proper assignment for number of columns, row count, prevRow, and BVC state.
// Each setter is validated by checking the field post-invocation.
func TestTTIrxd_Setters(t *testing.T) {
	t.Parallel()
	// Initialize tTIrxd struct
	rxd := newTTIrxd().(*tTIrxd)
	cols := common.UB4(2)
	rowNum := common.UB4(5)
	prev := []common.B1Array{
		{0x01},
		{0x02},
	}

	// Set and verify number of columns
	rxd.SetNumberOfColumns(cols)
	if rxd.numberOfColumns != cols {
		t.Errorf("SetNumberOfColumns: expected %d, got %d", cols, rxd.numberOfColumns)
	}
	// Set and verify row count
	rxd.SetRowCount(rowNum)
	if rxd.rowCount != rowNum {
		t.Errorf("SetRowCount: expected %d, got %d", rowNum, rxd.rowCount)
	}
	// Set prevRow to nil and verify
	rxd.SetPrevRow(nil)
	if rxd.prevRow != nil {
		t.Errorf("SetPrevRow(nil): expected nil, got %v", rxd.prevRow)
	}
	// Set prevRow to previous data and verify
	rxd.SetPrevRow(prev)
	if len(rxd.prevRow) != 2 {
		t.Errorf("SetPrevRow: expected len 2, got %d", len(rxd.prevRow))
	}
	// Set and verify BVC state
	rxd.SetBvcState(nil, true)
	if !rxd.bvcFound {
		t.Errorf("SetBvcState: expected bvcFound true")
	}
}

// TestTTIrxd_UnmarshalFrom_ErrorCases exercises error handling for TTIrxd's UnMarshalFrom method.
// It checks for various misconfigurations and insufficient payload scenarios, ensuring correct error responses.
// Each test case covers a different error trigger.
func TestTTIrxd_UnmarshalFrom_ErrorCases(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name       string
		setup      func(*tTIrxd)
		payload    []byte
		wantErrSub string
	}
	cases := []testCase{
		{
			name: "zero columns",
			setup: func(rxd *tTIrxd) {
				// No columns are set, simulating misconfigured state.
				rxd.SetNumberOfColumns(0)
				rxd.SetRowCount(1)
				rxd.SetPrevRow(nil)
				rxd.SetBvcState(nil, false)
			},
			payload:    []byte{10, 20, 30}, // arbitrary data shouldn't matter, as zero cols should error early
			wantErrSub: "Failed to unmarshal message: Row transfer data message",
		},
		{
			name: "payload too short for column header",
			setup: func(rxd *tTIrxd) {
				// Valid columns, no payload; triggers too-short error at col 0
				rxd.SetNumberOfColumns(2)
				rxd.SetRowCount(1)
				rxd.SetPrevRow(nil)
				rxd.SetBvcState(nil, false)
			},
			payload:    []byte{}, // empty payload
			wantErrSub: "Failed to unmarshal message: Row transfer data message",
		},
		{
			name: "payload too short for column data",
			setup: func(rxd *tTIrxd) {
				// Payload advertises length for data, but not enough bytes after
				rxd.SetNumberOfColumns(1)
				rxd.SetRowCount(1)
				rxd.SetPrevRow(nil)
				rxd.SetBvcState(nil, false)
			},
			payload:    []byte{5}, // column claims 5 bytes, but only length present
			wantErrSub: "Failed to unmarshal message: Row transfer data message",
		},
		{
			name: "bvc found, prevRow is nil",
			setup: func(rxd *tTIrxd) {
				rxd.SetNumberOfColumns(2)
				rxd.SetRowCount(2) // not first row
				rxd.SetPrevRow(nil)
				bitset := &common.BitSet{}
				rxd.SetBvcState(bitset, true)
			},
			payload:    []byte{42, 43}, // won't be used, error triggers before unmarshalling
			wantErrSub: "Failed to unmarshal message: Row transfer data message",
		},
		{
			name: "bvc found, prevRow wrong length",
			setup: func(rxd *tTIrxd) {
				rxd.SetNumberOfColumns(3)
				rxd.SetRowCount(2)                         // not first row
				rxd.SetPrevRow([]common.B1Array{{1}, {2}}) // length 2, should be 3
				bitset := &common.BitSet{}
				rxd.SetBvcState(bitset, true)
			},
			payload:    []byte{42, 43}, // won't be used, error triggers before unmarshalling
			wantErrSub: "Failed to unmarshal message: Row transfer data message",
		},
	}
	// Iterate all error case scenarios
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new tTIrxd & configure it for this scenario
			rxd := newTTIrxd().(*tTIrxd)
			tc.setup(rxd)
			columnContexts := make([]ColumnContext, int(rxd.numberOfColumns))
			for i := range columnContexts {
				columnContexts[i].DataType = DtyVCS
			}
			rxd.setColumnContexts(columnContexts)
			// Create the test marshaller with provided payload
			mar := createMarshaller(tc.payload, 0, 0)
			// Attempt to unmarshal; should error in each test case
			err := rxd.UnMarshalFrom(context.Background(), mar)
			if err == nil {
				t.Fatalf("Expected error but got <nil>")
			}
			// Error message must contain expected substring
			if !contains(err.Error(), tc.wantErrSub) {
				t.Errorf("Error mismatch. Got: %v, Want substring: %v", err.Error(), tc.wantErrSub)
			}
		})
	}
}

// TestTTIrxd_UnmarshalFrom verifies correct unmarshalling for a valid row payload.
// It ensures number of columns, row data populations, and byte comparisons are correct.
func TestTTIrxd_UnmarshalFrom(t *testing.T) {
	t.Parallel()
	const colCount = 2
	const rowCount = 1

	// Prepare payload and marshaller
	payload := makeRowPayload(validRow1Dump)
	mar := createMarshaller(payload, 0, 0)

	// Configure tTIrxd for straightforward unmarshalling
	rxd := newTTIrxd().(*tTIrxd)
	rxd.SetNumberOfColumns(colCount)
	rxd.SetRowCount(rowCount)
	rxd.SetPrevRow(nil)
	rxd.SetBvcState(nil, false)
	rxd.setColumnContexts([]ColumnContext{{DataType: DtyVCS}, {DataType: DtyVCS}})

	// Attempt to unmarshal: should succeed with valid data
	err := rxd.UnMarshalFrom(context.Background(), mar)
	if err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	// The row must contain exactly the expected number of columns
	if len(rxd.row) != colCount {
		t.Fatalf("Row should have %d columns, got %d", colCount, len(rxd.row))
	}

	// Each column should have non-empty data
	for i, col := range rxd.row {
		if col == nil || len(col) == 0 {
			t.Errorf("Expected non-empty data in col %d; got %v", i, col)
		}
	}

	// Compare row data to expected byte arrays
	want := [][]byte{{193, 2}, {97, 98, 99}}
	if !reflect.DeepEqual(rowToBytes(rxd.row), want) {
		t.Errorf("row data mismatch.\nGot:  %v\nWant: %v", rowToBytes(rxd.row), want)
	}
}

// TestTTIrxd_bvc_IntegrationTest runs integration scenarios for TTIrxd using Actual dumps.
// It validates unmarshalling of both two- and three-column encoded dumps, and checks reconstructed rows.
// Each test case describes its scenario with expected results.
func TestTTIrxd_bvc_IntegrationTest(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name    string
		dump    []string
		expRows [][][]byte
		noCols  common.UB4
	}
	cases := []testCase{
		{
			name:   "2-columns",
			dump:   validBvcRxdDump,
			noCols: 2,
			expRows: [][][]byte{
				{{193, 2}, {97, 98, 99}},
				{{193, 3}, {120, 121, 122}},
				{{193, 4}, {120, 121, 122}},
				{{193, 5}, {120, 121, 122}},
				{{193, 6}, {97, 98, 99}},
			},
		},
		{
			name:   "3-columns",
			dump:   validBvcRxd3ColDump,
			noCols: 3,
			expRows: [][][]byte{
				{{193, 2}, {97, 98, 99}, {115, 116, 114, 101, 101, 116, 32, 49}},
				{{193, 3}, {120, 121, 122}, {115, 116, 114, 101, 101, 116, 32, 49}},
				{{193, 4}, {120, 121, 122}, {115, 116, 114, 101, 101, 116, 32, 49}},
				{{193, 5}, {120, 121, 122}, {115, 116, 114, 101, 101, 116, 32, 50}},
				{{193, 6}, {97, 98, 99}, {115, 116, 114, 101, 101, 116, 32, 50}},
			},
		},
	}

	// Run each test scenario using the shared runBvcIntegration logic
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runBvcIntegration(t, tc.dump, tc.expRows, tc.noCols)
		})
	}
}

// runBvcIntegration unmarshals BVC-integrated row data and compares to expected byte results.
// It simulates marshaller-driven stream consumption: steps through RXD and BVC codes, performs row extraction,
// and validates both row contents and sequence.
func runBvcIntegration(t *testing.T, dump []string, expRows [][][]byte, noCols common.UB4) {
	ctx := context.Background()
	// Prepare marshaller with given binary dump
	payload, _ := ExtractBytesFromDump(dump)
	mar := createMarshaller(payload, 0, 0)
	bvc := &tTIbvc{}
	var prevRow []common.B1Array
	rowCount := common.UB4(0)
	rxdIndex := 0

	for {
		// Step: read message code from stream, abort if finished
		code, err := mar.UnmarshalUB1(ctx)
		if err != nil {
			t.Fatalf("UnMarshal Msg Code failed: %v", err)
		}

		// Stop when the TTIRPA code is encountered (end of rows)
		if code == common.UB1(TTIRPA) {
			break
		}

		switch code {
		case common.UB1(TTIRXD):
			// Step: RXD - Prepare to read a new row
			rowCount++
			rxd := newTTIrxd().(*tTIrxd)
			rxd.SetBvcState(bvc.bvcColSent, bvc.bvcFound)
			rxd.SetRowCount(rowCount)
			rxd.SetNumberOfColumns(noCols)
			rxd.SetPrevRow(prevRow)
			columnContexts := make([]ColumnContext, int(noCols))
			for i := range columnContexts {
				columnContexts[i].DataType = DtyVCS
			}
			rxd.setColumnContexts(columnContexts)
			// Step: Attempt to unmarshal row data
			err = rxd.UnMarshalFrom(ctx, mar)
			if err != nil {
				t.Fatalf("UnMarshalFrom RXD failed: %v", err)
			}

			// Compare this row's bytes to expected output
			got := rowToBytes(rxd.row)
			if rxdIndex < len(expRows) {
				want := expRows[rxdIndex]
				if !reflect.DeepEqual(got, want) {
					t.Errorf("Row %d mismatch.\nGot:  %v\nWant: %v", rxdIndex+1, got, want)
				}
			}
			// Step: reset state for loop
			rxdIndex++
			prevRow = rxd.row
			bvc.bvcFound = false

		case common.UB1(TTIBVC):
			// Step: BVC - Unmarshal BVC state for next rounds
			bvc = &tTIbvc{}
			bvc.SetNumberOfColumns(noCols)
			err = bvc.UnMarshalFrom(ctx, mar)
			if err != nil {
				t.Fatalf("UnMarshalFrom BVC failed: %v", err)
			}
			// print state to terminal (debug/test visibility)
			fmt.Printf("Row %v: BVC %v\n", rowCount, bvc.bvcColSent.String())

		default:
			// Unknown code - skip (defensive)
			break
		}
	}
}

// TestTTIrxd_BvcPresentColumn_UnmarshalError verifies error propagation in UnMarshalFrom when a BVC bitset
// marks a column as present (so unmarshalColumn is called) but marshalling fails (e.g. not enough data).
func TestTTIrxd_BvcPresentColumn_UnmarshalError(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)
	const numCols = 2
	rxd.SetNumberOfColumns(numCols)
	rxd.SetRowCount(2) // not first row
	rxd.SetPrevRow([]common.B1Array{{0x11}, {0x22}})
	bitset := common.NewBitSet(numCols)
	bitset.Set(0, true) // Only column 0 marked present
	rxd.SetBvcState(bitset, true)
	rxd.setColumnContexts([]ColumnContext{{DataType: DtyVCS}, {DataType: DtyVCS}})

	// Payload with only length byte, not enough data (forces error in _unmarshalScalarColumn)
	payload := []byte{5}
	mar := createMarshaller(payload, 0, 0)
	err := rxd.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatalf("Expected error for insufficient data on present column, got <nil>")
	}
	wantSub := "Failed to unmarshal message: Row transfer data message"
	if got := err.Error(); !contains(got, wantSub) {
		t.Errorf("Expected error to contain %q, got: %v", wantSub, got)
	}
}

// TestTTIrxd_BvcCarriedNullKeepsLobContextAligned verifies that a BVC-omitted
// NULL column still contributes one entry to the row's parallel LOB-context
// slice. Without that placeholder, row decoding later indexes past the end of
// lobColContext before it can recognize the column as NULL.
func TestTTIrxd_BvcCarriedNullKeepsLobContextAligned(t *testing.T) {
	t.Parallel()
	const numCols = 2

	rxd := newTTIrxd().(*tTIrxd)
	rxd.SetNumberOfColumns(numCols)
	rxd.SetRowCount(2)
	rxd.SetPrevRow([]common.B1Array{{0x11}, nil})
	rxd.setColumnContexts([]ColumnContext{
		{DataType: DtyVCS},
		{DataType: DtyClob},
	})

	// Only column 0 is present in this row. Column 1 is SQL NULL and must be
	// carried from the previous row without consuming any bytes from the wire.
	bitset := common.NewBitSet(numCols)
	bitset.Set(0, true)
	rxd.SetBvcState(bitset, true)

	mar := createMarshaller([]byte{1, 0x22}, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	if got := len(rxd.row); got != numCols {
		t.Fatalf("row column count = %d, want %d", got, numCols)
	}
	if rxd.row[1] != nil {
		t.Fatalf("carried NULL column = %v, want nil", rxd.row[1])
	}
	if got := len(rxd.getLobColumnContext()); got != numCols {
		t.Fatalf("LOB context count = %d, want %d to remain aligned with the row", got, numCols)
	}
}

// TestTTIrxd_BvcCarriedClobPreservesLobContext verifies that BVC carry moves a
// non-NULL CLOB's metadata together with its raw bytes. Losing the context leaves
// DecodeClob with a nil LobContext and causes a nil-pointer panic during Rows.Next.
func TestTTIrxd_BvcCarriedClobPreservesLobContext(t *testing.T) {
	t.Parallel()
	const numCols = 2

	shelf, _, _ := newExecTestShelf(1024)
	exec := &statementExecutorSelect{
		statementProcessor: statementProcessor{
			shelf:   shelf,
			sessCtx: &common.SessionContext{},
		},
		resultMetadata: selectResultMetadata{columns: []ColumnContext{
			{DataType: DtyVCS},
			{DataType: DtyClob},
		}},
	}
	state := &queryRunState{rows: exec.resultMetadata.newRows(shelf)}

	clobData := common.B1Array("hello")
	previousLobContext := &LobColumnContext{CharsetID: al16Utf16CharSet}
	state.handleRXDRow(&tTIrxd{
		row:           []common.B1Array{{0x11}, clobData},
		lobColContext: []*LobColumnContext{nil, previousLobContext},
	})

	// Only column 0 is sent for the next row. Column 1 must carry both its CLOB
	// bytes and the previous row's LOB metadata.
	bitset := common.NewBitSet(numCols)
	bitset.Set(0, true)
	state.bvcColSent = bitset
	state.bvcFound = true

	msg, err := exec.createRXD(state, nil)
	if err != nil {
		t.Fatalf("createRXD failed: %v", err)
	}
	rxd := msg.(*tTIrxd)
	mar := createMarshaller([]byte{1, 0x22}, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	if !reflect.DeepEqual(rxd.row[1], clobData) {
		t.Fatalf("carried CLOB data = %v, want %v", rxd.row[1], clobData)
	}
	if got := len(rxd.getLobColumnContext()); got != numCols {
		t.Fatalf("LOB context count = %d, want %d", got, numCols)
	}
	if got := rxd.getLobColumnContext()[1]; !reflect.DeepEqual(got, previousLobContext) {
		t.Fatalf("carried CLOB context = %#v, want %#v", got, previousLobContext)
	}
}
