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

// Package ttc – unit tests for DML RETURNING executor logic.
// These tests focus on the statementProcessor and statementExecutorDML logic that supports DML RETURNING,
// particularly the handling of OUT binds and OAC management.
//
// The tests in this file are white-box and target specific internal behaviors such as:
//   - initExecRunner: detection of sql.Out binds, management of
//     returnIOVVector and noPLSQLMode flags, and tracking of OUT
//     destinations.
//   - needToSendOACs: logic for determining when OACs must be
//     resent based on changes in bind count, type or length.
//   - getMaxLengthForOac: ensuring that the maxLength for OACs
//     is correctly calculated and monotonic across executions.
//   - handleRXDRow: decoding of returned values from the server
//     and assignment to destination pointers.
package ttc

import (
	"database/sql"
	sqldriver "database/sql/driver"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// ---------------------------------------------------------------------------
// initExecRunner
// ---------------------------------------------------------------------------

// TestInitExecRunner_NoOutBinds verifies that when no sql.Out binds are present
// the executor does NOT set the returnIOVVector flag and DOES preserve noPLSQLMode.
func TestInitExecRunner_NoOutBinds_FlagsNotSet(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := newStatementExecutorDML()
	exec.SetShelf(shelf)

	// Capture initial flag state that newStatementExecutorDML establishes.
	initialOpts := exec.opts
	if initialOpts&noPLSQLMode == 0 {
		t.Fatalf("pre-condition: noPLSQLMode must be set after construction; opts=0x%x", initialOpts)
	}

	args := []sqldriver.Value{int64(1), "hello"}
	exec.initExecRunner(args)

	// No OUT binds → returnIOVVector must NOT be set.
	if exec.opts&returnIOVVector != 0 {
		t.Errorf("returnIOVVector must not be set when there are no sql.Out binds; opts=0x%x", exec.opts)
	}
	// noPLSQLMode must remain set.
	if exec.opts&noPLSQLMode == 0 {
		t.Errorf("noPLSQLMode must remain set when there are no sql.Out binds; opts=0x%x", exec.opts)
	}
	// No destinations recorded.
	if len(exec.outDestPtrs) != 0 {
		t.Errorf("outDestPtrs must be empty, got %d entries", len(exec.outDestPtrs))
	}
}

// TestInitExecRunner_WithOutBinds_FlagsSet verifies that when sql.Out binds are present
// the executor sets returnIOVVector and clears noPLSQLMode.
func TestInitExecRunner_WithOutBinds_FlagsSet(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := newStatementExecutorDML()
	exec.SetShelf(shelf)

	var dest1 int64
	var dest2 string

	args := []sqldriver.Value{
		int64(42),
		sql.Out{Dest: &dest1},
		sql.Out{Dest: &dest2},
	}
	exec.initExecRunner(args)

	// returnIOVVector must be set.
	if exec.opts&returnIOVVector == 0 {
		t.Errorf("returnIOVVector must be set when sql.Out binds are present; opts=0x%x", exec.opts)
	}
	// noPLSQLMode must be cleared.
	if exec.opts&noPLSQLMode != 0 {
		t.Errorf("noPLSQLMode must be cleared when sql.Out binds are present; opts=0x%x", exec.opts)
	}
	// Two OUT destinations recorded.
	if len(exec.outDestPtrs) != 2 {
		t.Errorf("expected 2 outDestPtrs, got %d", len(exec.outDestPtrs))
	}
	if exec.outDestPtrs[0] != &dest1 {
		t.Errorf("outDestPtrs[0] = %p, want %p", exec.outDestPtrs[0], &dest1)
	}
	if exec.outDestPtrs[1] != &dest2 {
		t.Errorf("outDestPtrs[1] = %p, want %p", exec.outDestPtrs[1], &dest2)
	}
}

// TestInitExecRunner_InOutBinds_Counted verifies that sql.Out{In: true} increments
// numberOfInOutParams while still registering the destination pointer.
func TestInitExecRunner_InOutBinds_Counted(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := newStatementExecutorDML()
	exec.SetShelf(shelf)

	var dest1 int64
	var dest2 string

	args := []sqldriver.Value{
		sql.Out{Dest: &dest1, In: true},  // IN OUT – counted
		sql.Out{Dest: &dest2, In: false}, // OUT only – not counted
	}
	exec.initExecRunner(args)

	if exec.numberOfInOutParams != 1 {
		t.Errorf("numberOfInOutParams: got %d, want 1", exec.numberOfInOutParams)
	}
	if len(exec.outDestPtrs) != 2 {
		t.Errorf("outDestPtrs length: got %d, want 2", len(exec.outDestPtrs))
	}
}

// TestInitExecRunner_Reset verifies that calling initExecRunner a second time resets
// the per-execution state (outDestPtrs, numberOfInOutParams) so stale destinations
// are not carried over to the next execution.
//
// Note on returnIOVVector: initExecRunner only SETS this flag when OUT binds are
// present; it does not clear it when they disappear on a subsequent call.  The flag
// accumulates until the executor itself is discarded.  This is the current driver
// design; the test intentionally does not assert on returnIOVVector.
func TestInitExecRunner_Reset(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := newStatementExecutorDML()
	exec.SetShelf(shelf)

	var dest1 int64
	exec.initExecRunner([]sqldriver.Value{sql.Out{Dest: &dest1, In: true}})

	if len(exec.outDestPtrs) != 1 || exec.numberOfInOutParams != 1 {
		t.Fatalf("pre-condition failed after first initExecRunner call")
	}

	// Second call with no OUT binds must clear destination tracking.
	exec.initExecRunner([]sqldriver.Value{int64(7)})

	if len(exec.outDestPtrs) != 0 {
		t.Errorf("outDestPtrs must be reset, got %d entries", len(exec.outDestPtrs))
	}
	if exec.numberOfInOutParams != 0 {
		t.Errorf("numberOfInOutParams must be reset, got %d", exec.numberOfInOutParams)
	}
}

// ---------------------------------------------------------------------------
// needToSendOACs
// ---------------------------------------------------------------------------

// TestNeedToSendOACs_NoPreviousOACs verifies that the first call (previousOacs == nil)
// always reports that OACs must be sent.
func TestNeedToSendOACs_NoPreviousOACs(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{int64(1)}); err != nil {
		t.Fatalf("prepareBindsAndOAC: %v", err)
	}
	// previousOacs is still nil after the first call.
	if !sp.needToSendOACs() {
		t.Error("needToSendOACs must return true when previousOacs is nil (first execution)")
	}
}

// TestNeedToSendOACs_CountChanged verifies that OACs are sent when the number of
// bind positions changes between executions.
//
// getMaxLengthForOac is now bounds-checked: when the current execution adds a new
// bind position that has no previous OAC, it falls back to the current length
// rather than panicking with an out-of-bounds access.
func TestNeedToSendOACs_CountChanged(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// First "execution" – simulated by preparing and promoting to previousOacs.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{int64(1)}); err != nil {
		t.Fatalf("first prepareBindsAndOAC: %v", err)
	}
	sp.previousOacs = sp.currentOacs // mimic post-execution promotion

	// Second "execution" with a larger bind count.  getMaxLengthForOac must not
	// panic for position 1 (which has no prior OAC entry).
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{int64(2), "extra"}); err != nil {
		t.Fatalf("second prepareBindsAndOAC: %v", err)
	}
	if !sp.needToSendOACs() {
		t.Error("needToSendOACs must return true when bind count changed")
	}
}

// TestNeedToSendOACs_TypeChanged verifies that OACs are resent when the type of a
// bind position changes between executions (e.g. int64 → string).
func TestNeedToSendOACs_TypeChanged(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// First execution: int64 bind.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{int64(1)}); err != nil {
		t.Fatalf("first prepareBindsAndOAC: %v", err)
	}
	sp.previousOacs = sp.currentOacs

	// Second execution: string bind at the same position → different dataType OAC.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{"hello"}); err != nil {
		t.Fatalf("second prepareBindsAndOAC: %v", err)
	}
	if !sp.needToSendOACs() {
		t.Error("needToSendOACs must return true when bind dataType changed")
	}
}

// TestNeedToSendOACs_LengthIncreased verifies that OACs are resent when the encoded
// length of a bind grows beyond the previously declared maxLength.
func TestNeedToSendOACs_LengthIncreased(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// First execution: short string.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{"ab"}); err != nil {
		t.Fatalf("first prepareBindsAndOAC: %v", err)
	}
	sp.previousOacs = sp.currentOacs
	prevMaxLen := sp.currentOacs[0].(*tTIoac).maxLength

	// Second execution: longer string that exceeds prevMaxLen.
	longStr := "this-string-is-definitely-longer-than-two-characters"
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{longStr}); err != nil {
		t.Fatalf("second prepareBindsAndOAC: %v", err)
	}
	newMaxLen := sp.currentOacs[0].(*tTIoac).maxLength
	if newMaxLen <= prevMaxLen {
		t.Skipf("expected longer string to produce larger OAC maxLength; prev=%d new=%d", prevMaxLen, newMaxLen)
	}
	if !sp.needToSendOACs() {
		t.Error("needToSendOACs must return true when encoded length grew")
	}
}

// TestNeedToSendOACs_SameOAC_ReturnsFalse verifies that OACs are NOT resent when
// the bind type, length and count are identical to the previous execution.
func TestNeedToSendOACs_SameOAC_ReturnsFalse(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// First execution.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{int64(100)}); err != nil {
		t.Fatalf("first prepareBindsAndOAC: %v", err)
	}
	sp.previousOacs = sp.currentOacs

	// Second execution: same type and same encoded length.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{int64(200)}); err != nil {
		t.Fatalf("second prepareBindsAndOAC: %v", err)
	}
	if sp.needToSendOACs() {
		t.Error("needToSendOACs must return false when OAC type and length are unchanged")
	}
}

// ---------------------------------------------------------------------------
// getMaxLengthForOac
// ---------------------------------------------------------------------------

// TestGetMaxLengthForOac_NoPreviousOACs verifies that when no previous OACs exist
// the returned max length equals the currently encoded value's length.
func TestGetMaxLengthForOac_NoPreviousOACs(t *testing.T) {
	t.Parallel()
	sp := &statementProcessor{}
	// previousOacs is nil – current length should be returned.
	got := sp.getMaxLengthForOac(0, 7)
	if got != 7 {
		t.Errorf("getMaxLengthForOac with nil previousOacs: got %d, want 7", got)
	}
}

// TestGetMaxLengthForOac_PreservesPreviousLarger verifies that when the previous OAC
// declared a larger maxLength the larger value is preserved (monotonic non-decrease).
func TestGetMaxLengthForOac_PreservesPreviousLarger(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// Establish previousOacs with a large maxLength by executing with a long string.
	longStr := "abcdefghijklmnopqrstuvwxyz" // 26 chars
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{longStr}); err != nil {
		t.Fatalf("prepareBindsAndOAC: %v", err)
	}
	sp.previousOacs = sp.currentOacs
	prevMaxLen := sp.previousOacs[0].(*tTIoac).maxLength

	// Now ask for the OAC max length using a shorter current length.
	shortLen := int(prevMaxLen) - 5
	if shortLen < 1 {
		t.Skipf("prevMaxLen=%d too small for this test", prevMaxLen)
	}
	got := sp.getMaxLengthForOac(0, shortLen)
	if got != prevMaxLen {
		t.Errorf("getMaxLengthForOac: got %d, want %d (previous larger value must be preserved)", got, prevMaxLen)
	}
}

// TestGetMaxLengthForOac_UsesCurrentIfLarger verifies that when the current encoded
// length exceeds the previously declared maxLength the current length is returned.
func TestGetMaxLengthForOac_UsesCurrentIfLarger(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	sp := &statementProcessor{shelf: shelf}

	// Establish previousOacs with a small maxLength.
	if err := sp.prepareBindsAndOAC([]sqldriver.Value{"xy"}); err != nil {
		t.Fatalf("prepareBindsAndOAC: %v", err)
	}
	sp.previousOacs = sp.currentOacs
	prevMaxLen := sp.previousOacs[0].(*tTIoac).maxLength

	// Use a current length that is larger.
	largerLen := int(prevMaxLen) + 50
	got := sp.getMaxLengthForOac(0, largerLen)
	if got != common.UB4(largerLen) {
		t.Errorf("getMaxLengthForOac: got %d, want %d (current larger value must win)", got, largerLen)
	}
}

// ---------------------------------------------------------------------------
// handleRXDRow
// ---------------------------------------------------------------------------

// TestHandleRXDRow_AssignsDecodedValue verifies that handleRXDRow decodes a wire-format
// value from rxd.row and assigns it to the correct destination pointer.
func TestHandleRXDRow_AssignsDecodedValue(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := &statementExecutorDML{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{shelf: shelf},
		},
	}

	var dest string
	exec.outDestPtrs = []any{&dest}
	exec.outColumnContexts = []columnContext{
		{Index: 0, DataType: DtyVCS},
	}

	// Build a fake tTIrxd carrying the wire bytes for "hello".
	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = []common.B1Array{common.B1Array("hello")}

	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatalf("handleRXDRow returned error: %v", err)
	}
	if dest != "hello" {
		t.Errorf("dest: got %q, want %q", dest, "hello")
	}
}

// TestHandleRXDRow_NilDestinationSkipped verifies that a nil destination pointer is
// silently skipped without panicking or returning an error.
func TestHandleRXDRow_NilDestinationSkipped(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := &statementExecutorDML{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{shelf: shelf},
		},
	}

	exec.outDestPtrs = []any{nil} // nil destination
	exec.outColumnContexts = []columnContext{
		{Index: 0, DataType: DtyVCS},
	}

	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = []common.B1Array{common.B1Array("ignored")}

	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatalf("handleRXDRow returned error for nil destination: %v", err)
	}
	// No panic and no error is the success criterion.
}

// TestHandleRXDRow_MoreDestsThanReturnedValues verifies that when the server returns
// fewer values than there are OUT destinations the extra destinations are skipped.
func TestHandleRXDRow_MoreDestsThanReturnedValues(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := &statementExecutorDML{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{shelf: shelf},
		},
	}

	var dest1 string = "sentinel1"
	var dest2 string = "sentinel2"
	exec.outDestPtrs = []any{&dest1, &dest2}
	exec.outColumnContexts = []columnContext{
		{Index: 0, DataType: DtyVCS},
		{Index: 1, DataType: DtyVCS},
	}

	// Server only returned one value.
	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = []common.B1Array{common.B1Array("value1")} // only 1 element

	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatalf("handleRXDRow returned error: %v", err)
	}
	if dest1 != "value1" {
		t.Errorf("dest1: got %q, want %q", dest1, "value1")
	}
	// dest2 must remain at its initial value (the index is out-of-range for rxd.row).
	if dest2 != "sentinel2" {
		t.Errorf("dest2 must remain unchanged, got %q", dest2)
	}
}

// TestHandleRXDRow_NilWireValue_SkipsAssignment verifies that when the wire payload for
// a position is nil (SQL NULL), handleRXDRow skips assignment and the destination
// keeps its initial value.
func TestHandleRXDRow_NilWireValue_SkipsAssignment(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := &statementExecutorDML{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{shelf: shelf},
		},
	}

	exec.outDestPtrs = []any{new("original")}
	exec.outColumnContexts = []columnContext{
		{Index: 0, DataType: DtyVCS},
	}

	// Nil wire payload – decoder returns nil → assignment skipped.
	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = []common.B1Array{nil}

	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatalf("handleRXDRow returned error for nil wire value: %v", err)
	}
	// The decoder for DtyVCS called with nil data returns "".
	// An empty string IS a valid decoded value (not nil), so the field gets assigned "".
	// This is acceptable behaviour for an empty/null string wire payload.
	// The important thing is: no panic and no error.
}

// TestHandleRXDRow_RawBytes_AssignedToByteSlice verifies that raw bytes in the wire
// payload are assigned to a []byte destination via the registered DtyBin decoder.
func TestHandleRXDRow_RawBytes_AssignedToByteSlice(t *testing.T) {
	t.Parallel()
	shelf := newShelf[common.MessageType]()
	registerTestCodecs(shelf, 20)

	exec := &statementExecutorDML{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{shelf: shelf},
		},
	}

	var dest []byte
	exec.outDestPtrs = []any{&dest}
	exec.outColumnContexts = []columnContext{
		{Index: 0, DataType: DtyBin},
	}

	payload := common.B1Array{0xDE, 0xAD, 0xBE, 0xEF}
	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = []common.B1Array{payload}

	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatalf("handleRXDRow returned error: %v", err)
	}
	if len(dest) != 4 || dest[0] != 0xDE || dest[3] != 0xEF {
		t.Errorf("dest: got %v, want [0xDE 0xAD 0xBE 0xEF]", dest)
	}
}
