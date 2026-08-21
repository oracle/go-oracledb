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

// Package ttc – unit tests for the DML RETURNING path of tTIrxd.UnMarshalFrom
package ttc

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// ---------------------------------------------------------------------------
// Wire-payload helpers
// ---------------------------------------------------------------------------

// encodeUniversalUB4 encodes v using Oracle's Universal (compact) integer format,
// which is what createMarshaller's UnmarshalUB4 expects (B4 = Universal in rep[2]).
//
//   - 0              → {0x00}
//   - 1-255          → {0x01, v}
//   - 256-65535      → {0x02, high, low}
//   - etc.
func encodeUniversalUB4(v uint32) []byte {
	if v == 0 {
		return []byte{0x00}
	}
	b := [4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	i := 0
	for i < 3 && b[i] == 0 {
		i++
	}
	result := make([]byte, 1+4-i)
	result[0] = byte(4 - i)
	copy(result[1:], b[i:])
	return result
}

// encodeUniversalUB2 encodes v using Oracle's Universal (compact) integer format,
// which is what createMarshaller's UnmarshalUB2 expects (B2 = Universal in rep[1]).
func encodeUniversalUB2(v uint16) []byte {
	if v == 0 {
		return []byte{0x00}
	}
	if v <= 0xFF {
		return []byte{0x01, byte(v)}
	}
	return []byte{0x02, byte(v >> 8), byte(v)}
}

// returningPayload builds a RETURNING wire payload for a single position using the
// encoding conventions of createMarshaller (Universal UB4/UB2, Native UB1/CLR).
//
//	rows: slice of CLR-encoded data items (nil = SQL NULL, non-nil = value bytes).
//
// Format per position:
//
//	UB4 (Universal) – row count
//	For each row: CLR (UB1 length byte + data bytes, or 0xFF for NULL) + UB2 (Universal) indicator
func returningPayload(rows [][]byte) []byte {
	out := []byte{}
	// UB4 row count in Universal encoding.
	out = append(out, encodeUniversalUB4(uint32(len(rows)))...)
	for _, row := range rows {
		if row == nil {
			// NULL: CLR null length indicator = 0xFF (not 0x00 which means zero-length).
			out = append(out, 0xFF)
		} else {
			// Short CLR form: UB1 length byte (Native) + data bytes.
			out = append(out, byte(len(row)))
			out = append(out, row...)
		}
		// DML/PL/SQL trailing indicator UB2 = 0 in Universal encoding.
		out = append(out, encodeUniversalUB2(0)...)
	}
	return out
}

// buildReturningWire concatenates payloads for multiple RETURNING positions.
func buildReturningWire(positions ...[][]byte) []byte {
	out := []byte{}
	for _, rows := range positions {
		out = append(out, returningPayload(rows)...)
	}
	return out
}

func newDMLReturningRXD(numberOfReturningPositions int) *tTIrxd {
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setNumberofReturningArgs(numberOfReturningPositions)
	rxd.setDmlReturning()
	return rxd
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestTTIrxd_UnMarshalFrom_Returning_SinglePosition_SingleRow verifies that
// UnMarshalFrom with numberOfReturningPositions=1 correctly reads a single
// returned row for that position.
func TestTTIrxd_UnMarshalFrom_Returning_SinglePosition_SingleRow(t *testing.T) {
	t.Parallel()
	data := []byte("hello")
	wire := buildReturningWire([][]byte{data})

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error: %v", err)
	}

	if len(rxd.row) != 1 {
		t.Fatalf("row length: got %d, want 1", len(rxd.row))
	}
	if !reflect.DeepEqual([]byte(rxd.row[0]), data) {
		t.Errorf("row[0]: got %v, want %v", rxd.row[0], data)
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_TwoPositions_SingleRowEach verifies that
// UnMarshalFrom reads two RETURNING positions each containing one row.
func TestTTIrxd_UnMarshalFrom_Returning_TwoPositions_SingleRowEach(t *testing.T) {
	t.Parallel()
	data0 := []byte("alpha")
	data1 := []byte{0xDE, 0xAD, 0xBE}
	wire := buildReturningWire([][]byte{data0}, [][]byte{data1})

	rxd := newDMLReturningRXD(2)

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error: %v", err)
	}

	if len(rxd.row) != 2 {
		t.Fatalf("row length: got %d, want 2", len(rxd.row))
	}
	if !reflect.DeepEqual([]byte(rxd.row[0]), data0) {
		t.Errorf("row[0]: got %v, want %v", rxd.row[0], data0)
	}
	if !reflect.DeepEqual([]byte(rxd.row[1]), data1) {
		t.Errorf("row[1]: got %v, want %v", rxd.row[1], data1)
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_ZeroRowsForPosition verifies that when a
// RETURNING position has a row count of zero (the DML matched no rows for that
// position) no data is read and the row slot remains nil.
func TestTTIrxd_UnMarshalFrom_Returning_ZeroRowsForPosition(t *testing.T) {
	t.Parallel()
	// Position 0: 0 rows (no data or indicators follow).
	wire := buildReturningWire([][]byte{}) // empty slice → row count = 0

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error for zero-row position: %v", err)
	}

	if len(rxd.row) != 1 {
		t.Fatalf("row length: got %d, want 1", len(rxd.row))
	}
	// With 0 rows the inner loop never executes, so row[0] stays nil.
	if rxd.row[0] != nil {
		t.Errorf("row[0] should be nil for zero-row position, got %v", rxd.row[0])
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_NullValue verifies that a NULL wire value
// (CLR length byte = 0x00) is stored as a nil B1Array in rxd.row.
func TestTTIrxd_UnMarshalFrom_Returning_NullValue(t *testing.T) {
	t.Parallel()
	// Pass nil to returningPayload to produce a NULL CLR (0x00 length byte).
	wire := buildReturningWire([][]byte{nil})

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error for NULL value: %v", err)
	}

	if len(rxd.row) != 1 {
		t.Fatalf("row length: got %d, want 1", len(rxd.row))
	}
	if rxd.row[0] != nil {
		t.Errorf("row[0] should be nil (SQL NULL), got %v", rxd.row[0])
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_MultipleRows verifies that when multiple rows
// are returned for one position the final row's data is stored in rxd.row[col]
// (each iteration overwrites the previous one via _unmarshalScalarColumn).
func TestTTIrxd_UnMarshalFrom_Returning_MultipleRows(t *testing.T) {
	t.Parallel()
	first := []byte("first")
	last := []byte("last-value")

	// Two rows for position 0.
	wire := buildReturningWire([][]byte{first, last})

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error: %v", err)
	}

	if len(rxd.row) != 1 {
		t.Fatalf("row length: got %d, want 1", len(rxd.row))
	}
	// The second (last) iteration overwrites the first – verify the last value won.
	if !reflect.DeepEqual([]byte(rxd.row[0]), last) {
		t.Errorf("row[0]: got %v, want last value %v", rxd.row[0], last)
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_RowCountReadError verifies that an error is
// returned when the wire stream is truncated before the UB4 row-count field can be
// read for a position.
func TestTTIrxd_UnMarshalFrom_Returning_RowCountReadError(t *testing.T) {
	t.Parallel()
	// Universal UB4 length says 4 bytes follow, but only 2 are present.
	wire := []byte{0x04, 0x00, 0x00}

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	err := rxd.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatal("expected error for truncated DML RETURNING row count, got nil")
	}
	var se oracleErrors.SQLError
	if !errors.As(err, &se) {
		t.Fatalf("expected SQLError, got %T: %v", err, err)
	}
	if se.ErrorCode() != string(oracleErrors.FailUnmarshal) {
		t.Errorf("error code: got %s, want %s", se.ErrorCode(), oracleErrors.FailUnmarshal)
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_DataReadError verifies that an error is
// returned when the wire stream is truncated in the middle of a CLR data payload
// for a returned row.
func TestTTIrxd_UnMarshalFrom_Returning_DataReadError(t *testing.T) {
	t.Parallel()
	// UB4 row count = 1 in Universal encoding, then CLR length = 5 but no data.
	wire := []byte{
		0x01, 0x01, // UB4 = 1 (Universal: 1 byte follows, value = 1)
		0x05, // CLR length = 5 bytes... but no data follows (stream truncated).
	}

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	err := rxd.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatal("expected error for truncated CLR data, got nil")
	}
	wantSub := "Failed to unmarshal message: Row transfer data message"
	if !contains(err.Error(), wantSub) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSub)
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_IndicatorReadError verifies that an error is
// returned when the wire stream is truncated before the UB2 indicator that follows
// each returned row's CLR data.
func TestTTIrxd_UnMarshalFrom_Returning_IndicatorReadError(t *testing.T) {
	t.Parallel()
	// UB4 row count = 1 (Universal), valid CLR "ab", but no trailing UB2 indicator.
	wire := []byte{
		0x01, 0x01, // UB4 = 1 (Universal: 1 byte follows, value = 1)
		0x02, 0x61, 0x62, // CLR: length=2, "ab"
		// Missing UB2 indicator (should be 0x00 in Universal encoding).
	}

	rxd := newDMLReturningRXD(1)

	mar := createMarshaller(wire, 0, 0)
	err := rxd.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatal("expected error for missing UB2 indicator, got nil")
	}
	wantSub := "Failed to unmarshal message: Row transfer data message"
	if !contains(err.Error(), wantSub) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSub)
	}
}

// TestTTIrxd_UnMarshalFrom_Returning_ThreePositionsMixed verifies a realistic scenario
// with three RETURNING positions: the first has one row, the second has zero rows
// (no match), and the third has one NULL value.
func TestTTIrxd_UnMarshalFrom_Returning_ThreePositionsMixed(t *testing.T) {
	t.Parallel()
	data0 := []byte{0xC1, 0x02} // NUMBER wire encoding for "1"
	// position 1: zero rows
	// position 2: NULL
	wire := buildReturningWire(
		[][]byte{data0}, // position 0 – one non-null row
		[][]byte{},      // position 1 – zero rows
		[][]byte{nil},   // position 2 – one NULL row
	)

	rxd := newDMLReturningRXD(3)

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error: %v", err)
	}

	if len(rxd.row) != 3 {
		t.Fatalf("row length: got %d, want 3", len(rxd.row))
	}
	if !reflect.DeepEqual([]byte(rxd.row[0]), data0) {
		t.Errorf("row[0]: got %v, want %v", rxd.row[0], data0)
	}
	if rxd.row[1] != nil {
		t.Errorf("row[1] should be nil (zero rows), got %v", rxd.row[1])
	}
	if rxd.row[2] != nil {
		t.Errorf("row[2] should be nil (NULL value), got %v", rxd.row[2])
	}
}

// TestTTIrxd_SetNumberofReturningArgs_SwitchesMode verifies that setting
// numberOfReturningPositions to a positive value correctly switches UnMarshalFrom
// away from the regular column-count path (which requires numberOfColumns > 0).
func TestTTIrxd_SetNumberofReturningArgs_SwitchesMode(t *testing.T) {
	t.Parallel()
	data := []byte("abc")
	wire := buildReturningWire([][]byte{data})

	rxd := newDMLReturningRXD(1)
	// numberOfColumns is deliberately left at 0 – if the non-RETURNING path were taken
	// it would fail with "column count must be > 0".  This test confirms the RETURNING
	// path is taken instead.

	mar := createMarshaller(wire, 0, 0)
	if err := rxd.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("RETURNING mode must not require numberOfColumns; got error: %v", err)
	}
	if len(rxd.row) != 1 || !reflect.DeepEqual([]byte(rxd.row[0]), data) {
		t.Errorf("unexpected row data: got %v, want %v", rxd.row, data)
	}
}

// TestTTIrxd_MarshalTo_LargeCLR verifies that MarshalTo correctly writes a large
// value (> 250 bytes) using the CLR long form encoding.
func TestTTIrxd_MarshalTo_LargeCLR(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)

	// 260 bytes of payload – exceeds the CLR short-form limit (0xFE = 254).
	large := make(common.B1Array, 260)
	for i := range large {
		large[i] = byte(i & 0xFF)
	}
	rxd.setBindValues([]common.B1Array{large})

	buf, eng := NewMarshalEngineTest(common.BIG_ENDIAN, B2, Universal, 1024)

	if err := rxd.MarshalTo(context.Background(), eng); err != nil {
		t.Fatalf("MarshalTo returned error for large CLR: %v", err)
	}

	written := buf.bytes[:buf.currentWritePosition]
	if len(written) == 0 {
		t.Fatal("MarshalTo wrote no bytes for large CLR value")
	}
	// The first byte of the CLR encoding for lengths > 253 is 0xFE (long form indicator).
	if written[0] != 0xFE {
		t.Errorf("expected CLR long form indicator 0xFE, got 0x%02X", written[0])
	}
}

// TestTTIrxd_ProcessDMLPlSqlIndicator_ReadError verifies that _processDMLPlSqlIndicator
// returns a FailUnmarshal error when the UB2 indicator cannot be read from the stream.
func TestTTIrxd_ProcessDMLPlSqlIndicator_ReadError(t *testing.T) {
	t.Parallel()
	rxd := newTTIrxd().(*tTIrxd)

	// Empty marshaller – no bytes available for UB2.
	mar := createMarshaller([]byte{}, 0, 0)

	err := rxd._processDMLPlSqlIndicator(context.Background(), mar, 0)
	if err == nil {
		t.Fatal("expected error from _processDMLPlSqlIndicator with empty stream, got nil")
	}

	var se oracleErrors.SQLError
	if !errors.As(err, &se) {
		t.Fatalf("expected SQLError, got %T: %v", err, err)
	}
	if se.ErrorCode() != string(oracleErrors.FailUnmarshal) {
		t.Errorf("error code: got %s, want %s", se.ErrorCode(), oracleErrors.FailUnmarshal)
	}
}
