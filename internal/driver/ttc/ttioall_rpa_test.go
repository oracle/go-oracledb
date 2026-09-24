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

// helper: slice off TTC header and return payload
func ttirpaPayloadFromDump(lines []string) []byte {
	buf, _ := ExtractBytesFromDump(lines)
	if len(buf) <= 11 {
		return []byte{}
	}
	return buf[11:]
}

// Basic sanity check: decodes without error and produces expected baseline fields
func runTTIRPATest(t *testing.T, name string, dump []string) {
	t.Helper()

	payload := ttirpaPayloadFromDump(dump)
	if len(payload) == 0 {
		t.Fatalf("%s: empty payload from dump", name)
	}

	// Feed payload to marshaller
	buf := NewArrayDataBuffer(len(payload))
	if err := buf.WriteBytesWithContext(context.Background(), payload); err != nil {
		t.Fatalf("%s: failed to load payload into buffer: %v", name, err)
	}
	mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	// Use concrete type directly (same package) so we can assert fields
	rpa := &ttioallrpa{}

	if rpa.GetMsgCode() != TTIRPA {
		t.Fatalf("%s: GetMsgCode mismatch, expected TTIRPA", name)
	}

	if err := rpa.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("%s: UnMarshalFrom failed: %v", name, err)
	}
	// for DMLS
	if strings.Contains(name, "INSERT") {
		if err := rpa.UnMarshalDMLRows(context.Background(), mar); err != nil {
			t.Fatalf("%s: UnMarshalDMLRows failed: %v", name, err)
		}
	}

	// Minimal invariants from decoder:
	// - al8o4 must be present and contain at least [0]=SCN low, [1]=SCN high, [2]=cursorId
	if len(rpa.al8o4) < 3 {
		t.Fatalf("%s: al8o4 too short, got %d", name, len(rpa.al8o4))
	}

	// The computed cursorId is derived from al8o4[2]; ensure it matches
	if rpa.cursorId != common.SB4(rpa.al8o4[2]) {
		t.Fatalf("%s: cursorId mismatch: got %d want %d", name, rpa.cursorId, common.SB4(rpa.al8o4[2]))
	}

	// Recompute SCN as the implementation does and compare
	const KSCNFVB common.UB4 = 0x8000
	least := rpa.al8o4[0]
	most := rpa.al8o4[1] &^ KSCNFVB
	wantSCN := (uint64(most) << 32) | uint64(least)
	if rpa.scn != wantSCN {
		t.Fatalf("%s: scn mismatch: got %d want %d", name, rpa.scn, wantSCN)
	}

	// Reaching here means the payload parsed and core invariants hold.
}

// TestTTIOallRPA_Unmarshal_Drop verifies DROP TTIRPA payload decodes without error and core invariants hold.
// Expectation: UnMarshalFrom succeeds and derived fields (cursorId, SCN, al8o4) are consistent.
func TestTTIOallRPA_Unmarshal_Drop(t *testing.T) {
	t.Parallel()
	runTTIRPATest(t, "DROP", validTTIRPADropDump)
}

// TestTTIOallRPA_Unmarshal_Create verifies CREATE TTIRPA payload decodes without error and core invariants hold.
// Expectation: UnMarshalFrom succeeds and computed values (cursorId, SCN) match expected derivations.
func TestTTIOallRPA_Unmarshal_Create(t *testing.T) {
	t.Parallel()
	runTTIRPATest(t, "CREATE", validTTIRPACreateDump)
}

// TestTTIOallRPA_Unmarshal_Insert verifies INSERT TTIRPA payload decodes and DML rows can be unmarshalled.
// Expectation: UnMarshalFrom and UnMarshalDMLRows succeed with consistent decoder state.
func TestTTIOallRPA_Unmarshal_Insert(t *testing.T) {
	t.Parallel()
	runTTIRPATest(t, "INSERT", validTTIRPAInsertDump)
}

// TestTTIOallRPA_Unmarshal_AlterSessionDrop verifies ALTER SESSION/DROP style TTIRPA payload decodes correctly.
// Expectation: UnMarshalFrom succeeds and invariants (cursorId, SCN) hold true.
func TestTTIOallRPA_Unmarshal_AlterSessionDrop(t *testing.T) {
	t.Parallel()
	runTTIRPATest(t, "ALTER_SESSION_DROP", validTTIRPAAlterSessionDrop)
}

// Truncated payload should fail during UnMarshalFrom (e.g., not enough bytes for al8o4 or subsequent fields)
// TestTTIOallRPA_UnmarshalFrom_Fail_Truncated verifies truncated payloads fail during UnMarshalFrom.
// Expectation: decoder returns an error due to insufficient bytes for required fields (e.g., al8o4).
func TestTTIOallRPA_UnmarshalFrom_Fail_Truncated(t *testing.T) {
	t.Parallel()
	full := ttirpaPayloadFromDump(validTTIRPACreateDump)
	if len(full) < 2 {
		t.Skip("validTTIRPACreateDump too small to truncate meaningfully")
	}
	truncated := full[:2] // only UB2 length, no UB4s to follow

	buf := NewArrayDataBuffer(len(truncated))
	if err := buf.WriteBytesWithContext(context.Background(), truncated); err != nil {
		t.Fatalf("failed to load truncated payload: %v", err)
	}
	mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	rpa := &ttioallrpa{}
	if err := rpa.UnMarshalFrom(context.Background(), mar); err == nil {
		t.Fatalf("expected UnMarshalFrom to fail on truncated payload, but got nil error")
	}
}

// Fault-injection using FaultyArrayBasedDataBuffer
// TestTTIOallRPA_UnmarshalFrom_FaultyBuffer verifies injected read faults cause UnMarshalFrom to fail.
// Expectation: each subtest triggers an error for the configured failOn/callN scenario.
func TestTTIOallRPA_UnmarshalFrom_FaultyBuffer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		dump   []string
		failOn FailOn
		callN  int
	}{
		{"Create_ReadBytes_Fail1", validTTIRPACreateDump, failOnReadBytes, 1},
		{"Create_ReadByte_Fail1", validTTIRPACreateDump, failOnReadByte, 1},
		{"Drop_ReadBytes_Fail1", validTTIRPADropDump, failOnReadBytes, 1},
		{"Drop_ReadBytes_Fail2", validTTIRPADropDump, failOnReadByte, 8},
		{"Drop_ReadBytes_Fail3", validTTIRPADropDump, failOnReadByte, 9},
		{"Drop_ReadBytes_Fail4", validTTIRPADropDump, failOnReadByte, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := ttirpaPayloadFromDump(tc.dump)
			if len(payload) == 0 {
				t.Skip("empty payload from dump")
			}
			mar := createMarshaller(payload, tc.failOn, tc.callN)

			rpa := &ttioallrpa{}
			if err := rpa.UnMarshalFrom(context.Background(), mar); err == nil {
				t.Fatalf("expected UnMarshalFrom to fail with %v (call %d)", tc.failOn, tc.callN)
			}
		})
	}
}

// TestTTIOallRPA_UnmarshalFrom_UnsupportedNonZeroValues verifies unsupported optional RPA sections fail when present.
// Expectation: non-zero keyword-value count and registration-feedback length are rejected.
func TestTTIOallRPA_UnmarshalFrom_UnsupportedNonZeroValues(t *testing.T) {
	t.Parallel()
	payload := ttirpaPayloadFromDump(validTTIRPADropDump)
	if len(payload) < 16 {
		t.Skip("validTTIRPADropDump too small to modify unsupported RPA fields")
	}

	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "keyword_value_count",
			payload: append(append(append([]byte{}, payload[:14]...), 0x01, 0x01), payload[15:]...),
		},
		{
			name:    "registration_feedback_length",
			payload: append(append(append([]byte{}, payload[:15]...), 0x01, 0x01), payload[16:]...),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := NewArrayDataBuffer(len(tc.payload))
			if err := buf.WriteBytesWithContext(context.Background(), tc.payload); err != nil {
				t.Fatalf("failed to load modified payload into buffer: %v", err)
			}
			mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

			rpa := &ttioallrpa{}
			if err := rpa.UnMarshalFrom(context.Background(), mar); err == nil || !strings.Contains(err.Error(), "Failed to unmarshal message") {
				t.Fatalf("expected failure for unsupported non-zero %s, got err=%v", tc.name, err)
			}
		})
	}
}

// TestTTIOallRPA_UnmarshalDMLRows_FaultyBuffer verifies failures during DML row count unmarshalling are reported.
// Expectation: UnMarshalDMLRows returns error when marshaller read operations fail for the rows section.
func TestTTIOallRPA_UnmarshalDMLRows_FaultyBuffer(t *testing.T) {
	t.Parallel()
	// Build a minimal DML rows buffer: UB4 count (2) + 16 bytes for two UB8s
	rowsBuf := append([]byte{0x01, 0x02, 0x00, 0x02}, make([]byte, 16)...)

	sub := func(name string, failOn FailOn, mar2 common.Marshaller) {
		t.Run(name, func(t *testing.T) {
			// Parse a valid RPA first (INSERT) so rpa is initialized
			payload := ttirpaPayloadFromDump(validTTIRPAInsertDump)
			if len(payload) == 0 {
				t.Skip("empty insert payload")
			}
			buf := NewArrayDataBuffer(len(payload))
			_ = buf.WriteBytesWithContext(context.Background(), payload)
			mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			rpa := &ttioallrpa{}
			if err := rpa.UnMarshalFrom(context.Background(), mar); err != nil {
				t.Fatalf("unexpected UnMarshalFrom error: %v", err)
			}

			// Now inject a faulty reader for DML rows phase
			if err := rpa.UnMarshalDMLRows(context.Background(), mar2); err == nil {
				t.Fatalf("expected UnMarshalDMLRows to fail with %v", failOn)
			}
		})
	}

	sub("DMLRows_ReadBytes_Fail1", failOnReadBytes, createMarshaller(rowsBuf, failOnReadBytes, 2))
	sub("DMLRows_ReadByte_Fail1", failOnReadByte, createMarshaller(rowsBuf, failOnReadByte, 1))
}

// Exercise transaction context path: set al8txl > 0 and supply 3 bytes (A,B,C).
// TestTTIOallRPA_Unmarshal_TransactionContext verifies parsing succeeds when transaction context bytes are present.
// Expectation: UnMarshalFrom completes and invariants (cursorId and SCN derived from al8o4) match expected values.
func TestTTIOallRPA_Unmarshal_TransactionContext(t *testing.T) {
	t.Parallel()
	// Start from DROP dump payload
	payload := ttirpaPayloadFromDump(validTTIRPADropDump)
	if len(payload) < 15 {
		t.Skip("payload too short to modify al8txl at index 13/14")
	}
	// Set al8txl (UB2) to 3. With BIG_ENDIAN, high byte at index 13, low byte at 14.
	finalPayload := make([]byte, 0)
	payload[13] = 0x01
	finalPayload = append(finalPayload, payload[:14]...)
	finalPayload = append(finalPayload, 0x03, 'A', 'B', 'C')
	finalPayload = append(finalPayload, payload[14:]...)

	// Feed modified payload to marshaller
	buf := NewArrayDataBuffer(len(finalPayload))
	if err := buf.WriteBytesWithContext(context.Background(), finalPayload); err != nil {
		t.Fatalf("failed to load modified payload into buffer: %v", err)
	}
	mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	rpa := &ttioallrpa{}
	if rpa.GetMsgCode() != TTIRPA {
		t.Fatalf("GetMsgCode mismatch, expected TTIRPA")
	}
	// Should parse including transaction context without error
	if err := rpa.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom failed with transaction context: %v", err)
	}

	// Minimal invariants as in runTTIRPATest
	if len(rpa.al8o4) < 3 {
		t.Fatalf("al8o4 too short, got %d", len(rpa.al8o4))
	}
	if rpa.cursorId != common.SB4(rpa.al8o4[2]) {
		t.Fatalf("cursorId mismatch: got %d want %d", rpa.cursorId, common.SB4(rpa.al8o4[2]))
	}
	const KSCNFVB common.UB4 = 0x8000
	least := rpa.al8o4[0]
	most := rpa.al8o4[1] &^ KSCNFVB
	wantSCN := (uint64(most) << 32) | uint64(least)
	if rpa.scn != wantSCN {
		t.Fatalf("scn mismatch: got %d want %d", rpa.scn, wantSCN)
	}
}

// Negative: exercise failure when transaction context bytes are insufficient.
// Sets al8txl=3 but appends fewer than 3 bytes to force UnmarshalB1Array error.

// TestTTIOallRPA_Unmarshal_TransactionContext_Fail_Bytes verifies insufficient txn context bytes cause failure.
// Expectation: UnMarshalFrom returns an error indicating failure while reading [txn-bytes].
func TestTTIOallRPA_Unmarshal_TransactionContext_Fail_Bytes(t *testing.T) {
	t.Parallel()
	// Start from DROP dump payload
	payload := ttirpaPayloadFromDump(validTTIRPADropDump)
	if len(payload) < 15 {
		t.Skip("payload too short to modify al8txl at index 13/14")
	}
	// Set al8txl (UB2) to 3 and craft a truncated final payload so only 1 byte is available.
	// With BIG_ENDIAN, high byte at index 13, low byte at 14; keep consistency with other test by setting high=0x01.
	finalPayload := make([]byte, 0)
	payload[13] = 0x01
	finalPayload = append(finalPayload, payload[:14]...)
	// Insert low byte (0x03) and only 1 data byte 'A' (less than required 3)
	finalPayload = append(finalPayload, 0x03, 'A')
	// DO NOT append payload[14:] to force insufficient bytes for UnmarshalB1Array(ctx, 3)

	buf := NewArrayDataBuffer(len(finalPayload))
	if err := buf.WriteBytesWithContext(context.Background(), finalPayload); err != nil {
		t.Fatalf("failed to load modified payload into buffer: %v", err)
	}
	mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	rpa := &ttioallrpa{}
	if err := rpa.UnMarshalFrom(context.Background(), mar); err == nil || !strings.Contains(err.Error(), "Failed to unmarshal message") {
		t.Fatalf("expected failure for transaction context bytes, got err=%v", err)
	}
}
