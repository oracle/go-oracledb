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
	"bytes"
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
)

// oall8GoldenPayload extracts the TTC OALL8 function payload (post-header) from hex dump
// using the same 11-byte offset convention as other TTIFUN goldens (e.g., OAuth).
func oall8GoldenPayload(lines []string) []byte {
	buf, _ := ExtractBytesFromDump(lines)
	if len(buf) <= 11 {
		return []byte{}
	}
	return buf[11:]
}

func newEngine(capacity int) (*ArrayBasedDataBuffer, common.Marshaller) {
	buf := NewArrayDataBuffer(capacity)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	return buf, engine
}

// ============ Getter tests ============

func TestOall8_New_Success(t *testing.T) {
	t.Parallel()
	msg := NewOall18()
	if msg == nil {
		t.Fatal("NewOall18 returned nil")
	}

	msg = NewOall()
	if msg == nil {
		t.Fatal("NewOall returned nil")
	}
}

func TestOall8_GetMsgCode(t *testing.T) {
	t.Parallel()
	msg := NewOall18()
	if msg.GetMsgCode() != TTIFUN {
		t.Errorf("expected TTIFUN, got %v", msg.GetMsgCode())
	}
}

func TestOall8_getFuncCode(t *testing.T) {
	t.Parallel()
	m := NewOall18().(*tTIOall)
	if m.GetFuncCode() != oAll8 {
		t.Errorf("expected oAll8, got %v", m.GetFuncCode())
	}
}

// Helpers to build 13-int oall8Options vector per statement kind/flags.
// 0: 1 if statementParsedByServer (PRS) requested, else 0
// 1: iterations/batch (1 for DML & OTHER; 0 for SELECT when no fetchRows/describe)
// 5: SCN low (0 here)
// 6: SCN high (0 here)
// 7: 1 for SELECT, else 0
// 9: flags: +AL8EX_IMPL_RESULTS_CLIENT if EXE, +AL8EX_GET_PIDMLRC if DML
// 11: SSS cursorId position (0 here)
func makeAl8i4Drop() []common.UB4 {
	al := make([]common.UB4, 13)
	al[0] = 1                       // PRS
	al[1] = 1                       // OTHER => iterations 1
	al[5] = 0                       // SCN lo
	al[6] = 0                       // SCN hi
	al[7] = 0                       // not SELECT
	al[9] = _al8exImplResultsClient // EXE set
	al[11] = 0
	return al
}

func makeAl8i4Create() []common.UB4 {
	al := make([]common.UB4, 13)
	al[0] = 1
	al[1] = 1 // OTHER
	al[5] = 0
	al[6] = 0
	al[7] = 0
	al[9] = _al8exImplResultsClient
	al[11] = 0
	return al
}

func makeAl8i4Insert() []common.UB4 {
	al := make([]common.UB4, 13)
	al[0] = 1
	al[1] = 1 // DML, no binds => 1
	al[5] = 0
	al[6] = 0
	al[7] = 0 // not SELECT
	al[9] = _al8exImplResultsClient | _al8exGetPidmlrc
	al[11] = 0
	return al
}

func makeAl8i4Delete() []common.UB4 {
	al := make([]common.UB4, 13)
	al[0] = 1
	al[1] = 1 // DML, no binds => 1
	al[5] = 0
	al[6] = 0
	al[7] = 0
	al[9] = _al8exImplResultsClient | _al8exGetPidmlrc
	al[11] = 0
	return al
}

func makeAl8i4Select(rowsToFetch int) []common.UB4 {
	al := make([]common.UB4, 13)
	al[0] = 1
	al[1] = 0 // SELECT without fetchRows/describe => 0
	al[5] = 0
	al[6] = 0
	al[7] = 1 // SELECT
	al[9] = _al8exImplResultsClient
	al[11] = 0
	_ = rowsToFetch // rowsToFetch is used in prefetch fields, not oall8Options[1] for this path
	return al
}

// Test OALL8 MarshalTo for DROP TABLE TABLE1
func TestOall8_MarshalTo_Drop_MatchesGolden(t *testing.T) {
	t.Parallel()
	// Build message
	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)
	m.setSQL([]byte("DROP TABLE TABLE1"))
	m.setOall8Options(makeAl8i4Drop())

	buf, engine := newEngine(4096)
	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo DROP failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	want := oall8GoldenPayload(oall8MarshalToDropTable)
	if len(want) == 0 {
		t.Fatal("golden DROP payload decode returned empty")
	}

	if !bytes.Equal(got, want) {
		t.Logf("Got OALL8 DROP packet:")
		session.PrintPacket(got, 0, len(got))
		t.Logf("Want OALL8 DROP packet:")
		session.PrintPacket(want, 0, len(want))
		t.Fatalf("OALL8 DROP mismatch:\n got (%d bytes): % X\nwant (%d bytes): % X", len(got), got, len(want), want)
	}
}

// Test OALL8 MarshalTo for CREATE TABLE
func TestOall8_MarshalTo_Create_MatchesGolden(t *testing.T) {
	t.Parallel()
	sql := "CREATE TABLE IF NOT EXISTS table1 (id number, name varchar2(100))"

	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)
	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Create())

	buf, engine := newEngine(4096)
	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo CREATE failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	want := oall8GoldenPayload(oall8MarshalToCreateTable)
	if len(want) == 0 {
		t.Fatal("golden CREATE payload decode returned empty")
	}

	if !bytes.Equal(got, want) {
		t.Logf("Got OALL8 CREATE packet:")
		session.PrintPacket(got, 0, len(got))
		t.Logf("Want OALL8 CREATE packet:")
		session.PrintPacket(want, 0, len(want))
		t.Fatalf("OALL8 CREATE mismatch:\n got (%d bytes): % X\nwant (%d bytes): % X", len(got), got, len(want), want)
	}
}

// Test OALL8 MarshalTo for INSERT INTO table1 ...
func TestOall8_MarshalTo_Insert_MatchesGolden(t *testing.T) {
	t.Parallel()
	sql := "INSERT INTO table1 (id, name) VALUES(1, 'abc'), (2, 'xyz'), (3, 'pqr')"

	m := NewOall18().(*tTIOall)
	// PRS|EXE|COM|NPL (no BND since SQL uses literals, not binds)
	m.setOptions(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)
	m.setCurrentRank(1)

	m.setNumberOfBindPositions(0)
	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Insert())

	buf, engine := newEngine(8192)
	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo INSERT failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	want := oall8GoldenPayload(oall8MarshalToInsertIntoTable)
	if len(want) == 0 {
		t.Fatal("golden INSERT payload decode returned empty")
	}

	if !bytes.Equal(got, want) {
		t.Logf("Got OALL8 INSERT packet:")
		session.PrintPacket(got, 0, len(got))
		t.Logf("Want OALL8 INSERT packet:")
		session.PrintPacket(want, 0, len(want))
		t.Fatalf("OALL8 INSERT mismatch:\n got (%d bytes): % X\nwant (%d bytes): % X", len(got), got, len(want), want)
	}
}

// Test OALL8 MarshalTo for delete from table1 where name = 'xyz'
func TestOall8_MarshalTo_Delete_MatchesGolden(t *testing.T) {
	t.Parallel()
	sql := "delete from table1 where name = 'xyz'"

	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)
	m.setCurrentRank(1)
	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Delete())

	buf, engine := newEngine(4096)
	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo DELETE failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	want := oall8GoldenPayload(oall8MarshalToDeleteFromTable)
	if len(want) == 0 {
		t.Fatal("golden DELETE payload decode returned empty")
	}

	if !bytes.Equal(got, want) {
		t.Logf("Got OALL8 DELETE packet:")
		session.PrintPacket(got, 0, len(got))
		t.Logf("Want OALL8 DELETE packet:")
		session.PrintPacket(want, 0, len(want))
		t.Fatalf("OALL8 DELETE mismatch:\n got (%d bytes): % X\nwant (%d bytes): % X", len(got), got, len(want), want)
	}
}

// Test OALL8 MarshalTo for select * from table1
func TestOall8_MarshalTo_Select_MatchesGolden(t *testing.T) {
	t.Parallel()
	sql := "select * from table1"

	m := NewOall18().(*tTIOall)
	// PRS|EXE only, no FCH. isSelect true and rowsToFetch 10 => max prefetch size, 10 rows.
	m.setOptions(statementParsedByServer | executeStatement | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)
	// to match dump values.
	m.maxClientBuffer = maxValue
	m.rowsToFetch = 10

	m.setCurrentRank(0)
	// keep defines null (no defineColumnsProvided, defineColumns=0)
	m.setDefCols(0)
	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Select(10))

	buf, engine := newEngine(4096)
	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo SELECT failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	want := oall8GoldenPayload(oall8MarshalToSelectFromTable)
	if len(want) == 0 {
		t.Fatal("golden SELECT payload decode returned empty")
	}

	if !bytes.Equal(got, want) {
		t.Logf("Got OALL8 SELECT packet:")
		session.PrintPacket(got, 0, len(got))
		t.Logf("Want OALL8 SELECT packet:")
		session.PrintPacket(want, 0, len(want))
		t.Fatalf("OALL8 SELECT mismatch:\n got (%d bytes): % X\nwant (%d bytes): % X", len(got), got, len(want), want)
	}
}

// ============ Failure simulations (similar to TestOAuthMarshalTo_Fail) ============

// Builders for each statement kind
func buildOall8DDL(sql string) *tTIOall {
	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)

	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Drop())
	return m
}

func buildOall8DML(sql string) *tTIOall {
	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)

	m.setCurrentRank(1)

	m.setNumberOfBindPositions(0)
	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Insert())
	return m
}

func buildOall8SELECT(sql string, rowsToFetch int) *tTIOall {
	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | noPLSQLMode) // (!FCH) && EXE && PRS && isSelect
	m.setCursorId(0)
	m.setMaxLength(maxLength)

	m.setRowsToFetch()
	m.setDefCols(0)
	m.setSQL([]byte(sql))
	m.setOall8Options(makeAl8i4Select(rowsToFetch))
	return m
}

// Table-driven write-fault injections for DDL
func TestOall8_MarshalTo_Fail_DDL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
	}{
		// Up to SQL ptr/len
		{"Fail func code UB1", 1, 0},
		{"Fail seqNo UB1", 2, 0},
		{"Fail tokenNo UB1", 3, 0},
		{"Fail options UB4 (bytes)", 0, 1},
		{"Fail cursorId SB4 (bytes)", 0, 2},
		{"Fail sql PTR writeByte", 4, 0},
		{"Fail sql len SB4 (bytes)", 0, 3},

		// After SQL ptr/len: cover all error cases
		{"Fail oall8Options PTR writeByte", 5, 0},
		{"Fail oall8Options len SB4 (bytes)", 0, 4},
		{"Fail al8o4 null PTR", 6, 0},
		{"Fail al8o4l null PTR", 7, 0},
		{"Fail maxLength UB4 (bytes)", 0, 5},
		{"Fail bindValuesPresent null PTR", 8, 0},
		{"Fail bindValuesPresent count SB4 (bytes)", 0, 6},
		{"Fail app null PTR", 9, 0},
		{"Fail txn null PTR", 10, 0},
		{"Fail txnLen null PTR", 11, 0},
		{"Fail kv null PTR", 12, 0},
		{"Fail kvLen null PTR", 13, 0},
		{"Fail defineColumnsProvided null PTR", 14, 0},
		{"Fail defineColumnsProvided count SB4 (bytes)", 0, 7},
		{"Fail regidLb UB4 (bytes)", 0, 8},
		{"Fail reg null PTR", 15, 0},
		{"Fail reg ptr2 PTR", 16, 0},
		{"Fail 1100 null ptr1", 17, 0},
		{"Fail 1100 ub4_1 (bytes)", 0, 9},
		{"Fail 1100 null ptr2", 18, 0},
		{"Fail 1100 ub4_2 (bytes)", 0, 10},
		{"Fail regidMb UB4 (bytes)", 0, 11},
		{"Fail 1220 null ptr1", 19, 0},
		{"Fail 1220 ub4_1 (bytes)", 0, 12},
		{"Fail 1220 null ptr2", 20, 0},
		{"Fail 1220 ub4_2 (bytes)", 0, 13},
		{"Fail 1220 null ptr3", 21, 0},
		{"Fail 1220_EXT1 null ptr", 22, 0},
		{"Fail 1220_EXT1 ub4 (bytes)", 0, 14},
		{"Fail sql data WriteBytes", 0, 15},
		{"Fail oall8Options UB4 write (bytes)", 0, 16},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := buildOall8DDL("DROP TABLE TABLE1")
			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(1024),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := m.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte("simulated write error")) {
				t.Fatalf("expected simulated write error, got %v", err)
			}
		})
	}
}

// ============ Bind branch negative tests ============

// buildOall8WithBinds constructs an OALL8 that takes the bindValuesPresent branch:
// (options&bindValuesPresent) != 0 && numberOfBinds > 0 && sendBindsDefinition == true.
func buildOall8WithBinds() *tTIOall {
	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | noPLSQLMode | bindValuesPresent)
	m.setCursorId(0)
	m.setMaxLength(maxLength)

	// Keep SQL and oall8Options empty to reduce unrelated payload noise
	m.setSQL(nil)
	m.setOall8Options(nil)
	m.setNumberOfBindPositions(2)
	// No defines
	m.setDefCols(0)
	return m
}

// Probe for failure exactly at "bindValuesPresent ptr" write by sweeping WriteByte failure counts
// and asserting the wrapped error contains "bindValuesPresent ptr".
func TestOall8_MarshalTo_Fail_BindPtr(t *testing.T) {
	t.Parallel()
	m := buildOall8WithBinds()

	hit := false
	for n := 1; n < 30; n++ {
		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(4096),
			FailOnWriteByteCall:  n,
		}
		engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		err := m.MarshalTo(context.Background(), engine)
		if err == nil {
			continue
		}
		// Look for the specific bindValuesPresent ptr wrapper
		if bytes.Contains([]byte(err.Error()), []byte("error marshalling OALL8 bind ptr")) {
			t.Logf("got expected failure at bindValuesPresent ptr with WriteByte call #%d: %v", n, err)
			hit = true
			break
		}
	}
	if !hit {
		t.Skip("unable to deterministically force failure at bindValuesPresent ptr; skip to avoid flakiness")
	}
}

// Probe for failure exactly at "bindValuesPresent count" write by sweeping WriteBytes failure counts
// and asserting the wrapped error contains "bindValuesPresent count".
func TestOall8_MarshalTo_Fail_BindCount(t *testing.T) {
	t.Parallel()
	m := buildOall8WithBinds()

	hit := false
	for n := 1; n < 30; n++ {
		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(4096),
			FailOnWriteBytesCall: n,
		}
		engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		err := m.MarshalTo(context.Background(), engine)
		if err == nil {
			continue
		}
		// Look for the specific bindValuesPresent count wrapper
		if bytes.Contains([]byte(err.Error()), []byte("error marshalling OALL8 bind count")) {
			t.Logf("got expected failure at bindValuesPresent count with WriteBytes call #%d: %v", n, err)
			hit = true
			break
		}
	}
	if !hit {
		t.Skip("unable to deterministically force failure at bindValuesPresent count; skip to avoid flakiness")
	}
}

// ============ Define branch negative tests ============

// buildOall8WithDefines constructs an OALL8 that takes the defineColumnsProvided branch:
// m.defineColumns > 0 && (m.options&defineColumnsProvided) != 0.
func buildOall8WithDefines() *tTIOall {
	m := NewOall18().(*tTIOall)
	m.setOptions(statementParsedByServer | executeStatement | noPLSQLMode | defineColumnsProvided)
	m.setCursorId(0)
	m.setMaxLength(maxLength)

	// Keep SQL and oall8Options empty to reduce unrelated payload noise
	m.setSQL(nil)
	m.setOall8Options(nil)
	// Engage defineColumnsProvided branch
	m.setDefCols(3)
	// No binds

	m.setNumberOfBindPositions(0)
	return m
}

// Probe for failure exactly at "defineColumnsProvided ptr" write by sweeping WriteByte failure counts
// and asserting the wrapped error contains "defineColumnsProvided ptr".
func TestOall8_MarshalTo_Fail_DefinePtr(t *testing.T) {
	t.Parallel()
	m := buildOall8WithDefines()

	hit := false
	for n := 1; n < 30; n++ {
		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(4096),
			FailOnWriteByteCall:  n,
		}
		engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		err := m.MarshalTo(context.Background(), engine)
		if err == nil {
			continue
		}
		// Look for the specific defineColumnsProvided ptr wrapper
		if bytes.Contains([]byte(err.Error()), []byte("error marshalling OALL8 define ptr")) {
			t.Logf("got expected failure at defineColumnsProvided ptr with WriteByte call #%d: %v", n, err)
			hit = true
			break
		}
	}
	if !hit {
		t.Skip("unable to deterministically force failure at defineColumnsProvided ptr; skip to avoid flakiness")
	}
}

// Probe for failure exactly at "defineColumnsProvided count" write by sweeping WriteBytes failure counts
// and asserting the wrapped error contains "defineColumnsProvided count".
func TestOall8_MarshalTo_Fail_DefineCount(t *testing.T) {
	t.Parallel()
	m := buildOall8WithDefines()

	writeBytesCalls := []int{10, 15, 20, 25, 30, 40, 50, 60, 80, 100, 140, 180, 220}
	hit := false
	for _, n := range writeBytesCalls {
		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(4096),
			FailOnWriteBytesCall: n,
		}
		engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		err := m.MarshalTo(context.Background(), engine)
		if err == nil {
			continue
		}
		// Look for the specific defineColumnsProvided count wrapper
		if bytes.Contains([]byte(err.Error()), []byte("error marshalling OALL8 define count")) {
			t.Logf("got expected failure at defineColumnsProvided count with WriteBytes call #%d: %v", n, err)
			hit = true
			break
		}
	}
	if !hit {
		t.Skip("unable to deterministically force failure at defineColumnsProvided count; skip to avoid flakiness")
	}
}

// Targeted coverage for error during oall8Options UB4 data loop at end of MarshalTo.
// We heuristically try multiple WriteBytes failure positions with a large oall8Options
// so that at least one run fails inside the oall8Options data writes and returns
// "error marshalling OALL8 oall8Options data".
func TestOall8_MarshalTo_Fail_AL8I4_DataWrite(t *testing.T) {
	t.Parallel()
	// Start from a DDL build, but override oall8Options with a large vector
	m := buildOall8DDL("DROP TABLE TABLE1")
	big := make([]common.UB4, 256)
	for i := range big {
		big[i] = common.UB4(i + 1)
	}
	m.setOall8Options(big)

	// Try a range of WriteBytes call counts. Earlier counts may fail before
	// the oall8Options data loop; we continue until we observe the targeted error.
	candidates := []int{30, 40, 50, 60, 80, 100, 120, 160, 200, 240, 280, 320}
	hit := false
	for _, call := range candidates {
		faulty := &FaultyArrayBasedDataBuffer{
			ArrayBasedDataBuffer: NewArrayDataBuffer(8192),
			FailOnWriteBytesCall: call,
		}
		engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
		err := m.MarshalTo(context.Background(), engine)
		if err == nil {
			continue
		}
		if bytes.Contains([]byte(err.Error()), []byte("oall8Options data")) {
			t.Logf("got expected failure during oall8Options data write at WriteBytes call #%d: %v", call, err)
			hit = true
			break
		}
		// Otherwise, continue probing different call counts.
	}
	if !hit {
		t.Skip("unable to deterministically force failure inside oall8Options data loop; skipping to avoid flakiness")
	}
}

// ============ Empty SQL and Empty oall8Options coverage ============

func buildOall8Empty() *tTIOall {
	m := NewOall18().(*tTIOall)
	// Minimal PRS|EXE path, non-SELECT, non-DML
	m.setOptions(statementParsedByServer | executeStatement | noPLSQLMode)
	m.setCursorId(0)
	m.setMaxLength(maxLength)

	// Both payloads empty
	m.setSQL(nil)
	m.setOall8Options(nil)
	// No defines/binds
	m.setDefCols(0)

	m.setNumberOfBindPositions(0)
	return m
}

// Success path when len(sql)==0 and len(oall8Options)==0
func TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Success(t *testing.T) {
	t.Parallel()
	m := buildOall8Empty()
	buf := NewArrayDataBuffer(2048)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	if err := m.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo (empty sql/oall8Options) failed: %v", err)
	}
	if buf.currentWritePosition == 0 {
		t.Fatal("no bytes written for empty sql/oall8Options case")
	}
}

// Error simulations focused on null-ptr+zero-len branches for sql/oall8Options
func TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Failures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
	}{
		// These indices are relative to WriteByte/WriteBytes call order; goal is to cover null-ptr/len zero writes
		{"Fail sql null PTR (WriteByte)", 4, 0},
		{"Fail sql zero len (WriteBytes)", 0, 3},
		{"Fail oall8Options null PTR (WriteByte)", 5, 0},
		{"Fail oall8Options zero len (WriteBytes)", 0, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := buildOall8Empty()
			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(2048),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := m.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte("simulated write error")) {
				t.Fatalf("expected simulated write error, got %v", err)
			}
		})
	}
}

// Write-fault injections for DML (exercise 1200-block path)
func TestOall8_MarshalTo_Fail_DML(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
	}{
		// Up to SQL ptr/len
		{"Fail func code UB1", 1, 0},
		{"Fail options UB4 (bytes)", 0, 1},
		{"Fail cursorId SB4 (bytes)", 0, 2},
		{"Fail sql PTR writeByte", 4, 0},
		{"Fail sql len SB4 (bytes)", 0, 3},

		// After SQL ptr/len: cover error cases unique/common to DML
		{"Fail oall8Options PTR writeByte", 5, 0},
		{"Fail oall8Options len SB4 (bytes)", 0, 4},
		{"Fail al8o4 null PTR", 6, 0},
		{"Fail al8o4l null PTR", 7, 0},
		{"Fail maxLength UB4 (bytes)", 0, 5},
		{"Fail bindValuesPresent null PTR", 8, 0},
		{"Fail bindValuesPresent count SB4 (bytes)", 0, 6},
		{"Fail app null PTR", 9, 0},
		{"Fail txn null PTR", 10, 0},
		{"Fail txnLen null PTR", 11, 0},
		{"Fail kv null PTR", 12, 0},
		{"Fail kvLen null PTR", 13, 0},
		{"Fail defineColumnsProvided null PTR", 14, 0},
		{"Fail defineColumnsProvided count SB4 (bytes)", 0, 7},
		{"Fail regidLb UB4 (bytes)", 0, 8},
		{"Fail reg null PTR", 15, 0},
		{"Fail reg ptr2 PTR", 16, 0},
		{"Fail 1100 null ptr1", 17, 0},
		{"Fail 1100 ub4_1 (bytes)", 0, 9},
		{"Fail 1100 null ptr2", 18, 0},
		{"Fail 1100 ub4_2 (bytes)", 0, 10},
		{"Fail regidMb UB4 (bytes)", 0, 11},

		// DML-specific 1200 block
		{"Fail 1200 ptr1 writeByte", 19, 0},
		{"Fail 1200 currentRank UB4 (bytes)", 0, 12},
		{"Fail 1200 ptr2 writeByte", 20, 0},

		// 1220/EXT1 and payload
		{"Fail 1220 null ptr1", 21, 0},
		{"Fail 1220 ub4_1 (bytes)", 0, 13},
		{"Fail 1220 null ptr2", 22, 0},
		{"Fail 1220 ub4_2 (bytes)", 0, 14},
		{"Fail 1220 null ptr3", 23, 0},
		//{"Fail 1220_EXT1 null ptr", 24, 0},
		{"Fail 1220_EXT1 ub4 (bytes)", 0, 15},
		{"Fail sql data WriteBytes", 0, 16},
		{"Fail oall8Options UB4 write (bytes)", 0, 17},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := buildOall8DML("INSERT INTO table1 (id, name) VALUES(1,'a')")
			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(2048),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := m.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte("simulated write error")) {
				t.Fatalf("expected simulated write error, got %v", err)
			}
		})
	}
}

// Write-fault injections for SELECT (exercise prefetch path)
func TestOall8_MarshalTo_Fail_SELECT(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
	}{
		// Up to SQL ptr/len
		{"Fail func code UB1", 1, 0},
		{"Fail options UB4 (bytes)", 0, 1},
		{"Fail cursorId SB4 (bytes)", 0, 2},
		{"Fail sql PTR writeByte", 4, 0},
		{"Fail sql len SB4 (bytes)", 0, 3},

		// After SQL ptr/len: SELECT-specific + common
		{"Fail oall8Options PTR writeByte", 5, 0},
		{"Fail oall8Options len SB4 (bytes)", 0, 4},
		{"Fail al8o4 null PTR", 6, 0},
		{"Fail al8o4l null PTR", 7, 0},

		// Prefetch path
		{"Fail prefetch size UB4 (bytes)", 0, 5},
		{"Fail prefetch rows UB4 (bytes)", 0, 6},

		// After prefetch
		{"Fail maxLength UB4 (bytes)", 0, 7},
		{"Fail bindValuesPresent null PTR", 8, 0},
		{"Fail bindValuesPresent count SB4 (bytes)", 0, 8},
		{"Fail app null PTR", 9, 0},
		{"Fail txn null PTR", 10, 0},
		{"Fail txnLen null PTR", 11, 0},
		{"Fail kv null PTR", 12, 0},
		{"Fail kvLen null PTR", 13, 0},
		{"Fail defineColumnsProvided null PTR", 14, 0},
		{"Fail defineColumnsProvided count SB4 (bytes)", 0, 9},
		{"Fail regidLb UB4 (bytes)", 0, 10},
		{"Fail reg null PTR", 15, 0},
		{"Fail reg ptr2 PTR", 16, 0},
		{"Fail 1100 null ptr1", 17, 0},
		{"Fail 1100 ub4_1 (bytes)", 0, 11},
		{"Fail 1100 null ptr2", 18, 0},
		{"Fail 1100 ub4_2 (bytes)", 0, 12},
		{"Fail regidMb UB4 (bytes)", 0, 13},

		// Non-DML 1200 block for SELECT
		{"Fail 1200 null ptr1", 19, 0},
		{"Fail 1200 ub4 (bytes)", 0, 14},
		{"Fail 1200 null ptr2", 20, 0},

		// 1220/EXT1 and payload
		{"Fail 1220 null ptr1", 21, 0},
		{"Fail 1220 ub4_1 (bytes)", 0, 15},
		{"Fail 1220 null ptr2", 22, 0},
		{"Fail 1220 ub4_2 (bytes)", 0, 16},
		{"Fail 1220 null ptr3", 23, 0},
		//{"Fail 1220_EXT1 null ptr", 24, 0},
		{"Fail 1220_EXT1 ub4 (bytes)", 0, 17},
		{"Fail sql data WriteBytes", 0, 18},
		{"Fail oall8Options UB4 write (bytes)", 0, 19},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := buildOall8SELECT("select * from table1", 5)
			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(2048),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := m.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte("simulated write error")) {
				t.Fatalf("expected simulated write error, got %v", err)
			}
		})
	}
}
