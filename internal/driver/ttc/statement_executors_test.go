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
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Oall8Payload extracts TTC OALL8 payload (post header) from dump, same convention as ttioall_test.go
func Oall8Payload(lines []string) []byte {
	buf, _ := ExtractBytesFromDump(lines)
	if len(buf) <= 11 {
		return []byte{}
	}
	return buf[11:]
}

// Faulty shelf using FaultyArrayBasedDataBuffer via createMarshaller to inject read/write failures.
func newFaultyExecShelf(buf []byte, failOn FailOn, callN int) (*ttiShelf[common.MessageType], *MessageStreamer) {
	mar := createMarshaller(buf, failOn, callN)

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(mar)

	funcReg := NewRegistry[functionRegistryKey]()
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, -1, NewOall)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 18, NewOall18)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oAll8}, -1, newTTIOallRPA)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, -1, newOexfen)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, 18, newOexfen18)

	msgReg := NewRegistry[common.MessageType]()
	_ = msgReg.Register(TTIOER, 14, newTTIoer14WithEndOfCallStatusSupport)
	_ = msgReg.Register(TTIDCB, 24, newTTIdcb24)
	_ = msgReg.Register(TTIRXH, 0, newTTIrxh)
	_ = msgReg.Register(TTIRXD, 0, newTTIrxd)
	_ = msgReg.Register(TTIBVC, 0, newTTIbvc)
	_ = msgReg.Register(TTIFOB, 0, newTTIfob)

	factory := &SimpleFactory{
		ttcVersion:   24,
		msgregistry:  msgReg,
		funcregistry: funcReg,
	}
	shelf.RegisterMessageFactory(factory)

	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	return shelf, streamer
}

// Helper: use versatileStreamer to fail on Pull for a specific message type.
func makeVersatilePullShelf(failCode common.MessageType, errMsg string) *ttiShelf[common.MessageType] {
	shelf, _, _ := newExecTestShelf(1024)
	vs := &versatileStreamer{
		failCode:  failCode,
		failWhere: wherePull,
		failErr:   errors.New(errMsg),
	}
	shelf.RegisterMessageStreamer(vs)
	return shelf
}

// newExecTestShelf wires a Shelf with an in-memory marshaller and real MessageStreamer.
func newExecTestShelf(bufSize int) (*ttiShelf[common.MessageType], *MessageStreamer, *ArrayBasedDataBuffer) {
	buf := NewArrayDataBuffer(bufSize)
	mar := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(mar)

	// Local registries limited to what this test needs.
	funcReg := NewRegistry[functionRegistryKey]()
	// OALL8 request
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, -1, NewOall)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 18, NewOall18)
	// OALL8 response
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oAll8}, -1, newTTIOallRPA)

	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, -1, newOexfen)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, 18, newOexfen18)

	msgReg := NewRegistry[common.MessageType]()
	_ = msgReg.Register(TTIOER, 14, newTTIoer14WithEndOfCallStatusSupport)
	_ = msgReg.Register(TTIDCB, 24, newTTIdcb24)
	_ = msgReg.Register(TTIRXH, 0, newTTIrxh)
	_ = msgReg.Register(TTIRXD, 0, newTTIrxd)
	_ = msgReg.Register(TTIBVC, 0, newTTIbvc)
	_ = msgReg.Register(TTIFOB, 0, newTTIfob)

	// Simple factory over local registries
	factory := &SimpleFactory{
		ttcVersion:   24,
		msgregistry:  msgReg,
		funcregistry: funcReg,
	}
	shelf.RegisterMessageFactory(factory)

	// Real streamer bound to this shelf
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)

	return shelf, streamer, buf
}

func makeOall8RPAPayloadFromDump(dump []string) []byte {
	buf, _ := ExtractBytesFromDump(dump)
	if len(buf) <= 11 {
		return []byte{}
	}
	return buf[11:]
}

// -----------------------
// Happy-path integration
// -----------------------

type testStringScanner struct {
	value string
	valid bool
}

func (s *testStringScanner) Scan(src any) error {
	ns := &sql.NullString{}
	if err := ns.Scan(src); err != nil {
		return err
	}
	s.value = ns.String
	s.valid = ns.Valid
	return nil
}

func TestStatementExecutorExec_HandleRXDRow_UsesScannerDestination(t *testing.T) {
	t.Parallel()
	decoderRegistry := newCodecRegistry[DtyType, *typeDecoder]()
	if err := decoderRegistry.Register(
		DtyVCS,
		-1,
		newTypeDecoder(func(columnContext, common.B1Array) (sqldriver.Value, error) {
			return "scanner-value", nil
		}, nil),
	); err != nil {
		t.Fatalf("register decoder failed: %v", err)
	}

	shelf := newShelf[common.MessageType]()
	shelf.RegisterCodecFactory(&CodecFactoryImpl{
		ttcVersion: 24,
		decoders:   decoderRegistry,
	})

	dest := &testStringScanner{}
	exec := &statementExecutorExec{
		statementProcessor: statementProcessor{
			shelf: shelf,
		},
		outDestPtrs: []any{dest},
		outColumnContexts: []columnContext{
			{DataType: DtyVCS},
		},
	}

	rxd := &tTIrxd{
		row: []common.B1Array{
			common.B1Array("ignored-wire-value"),
		},
	}

	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatalf("handleRXDRow returned error: %v", err)
	}
	if !dest.valid {
		t.Fatalf("expected scanner destination to be marked valid")
	}
	if dest.value != "scanner-value" {
		t.Fatalf("unexpected scanner destination value: got %q want %q", dest.value, "scanner-value")
	}
}

func TestStatementExecutorExec_HandleRXDRow_PropagatesScannerError(t *testing.T) {
	t.Parallel()
	decoderRegistry := newCodecRegistry[DtyType, *typeDecoder]()
	if err := decoderRegistry.Register(
		DtyVCS,
		-1,
		newTypeDecoder(func(columnContext, common.B1Array) (sqldriver.Value, error) {
			return "scanner-value", nil
		}, nil),
	); err != nil {
		t.Fatalf("register decoder failed: %v", err)
	}

	shelf := newShelf[common.MessageType]()
	shelf.RegisterCodecFactory(&CodecFactoryImpl{
		ttcVersion: 24,
		decoders:   decoderRegistry,
	})

	dest := &sql.NullInt64{}
	exec := &statementExecutorExec{
		statementProcessor: statementProcessor{
			shelf: shelf,
		},
		outDestPtrs: []any{dest},
		outColumnContexts: []columnContext{
			{DataType: DtyVCS},
		},
	}

	rxd := &tTIrxd{
		row: []common.B1Array{
			common.B1Array("ignored-wire-value"),
		},
	}

	if err := exec.handleRXDRow(rxd); err == nil {
		t.Fatal("expected scanner error, got nil")
	}
}

// TestStatementExecutor_Others_Drop_MarshalAndExec verifies DROP path marshal+exec using a seeded TTIRPA payload.
// Expectation: ExecContext completes without error.
func TestStatementExecutor_Others_Drop_MarshalAndExec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Pre-seed incoming wire: TTIRPA (DROP)
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOall8RPAPayloadFromDump(validTTIRPADropDump)); err != nil {
		t.Fatalf("write OALL8 RPA (DROP) payload failed: %v", err)
	}

	exec := newStatementExecutorOthers()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("DROP TABLE T")
	_, err := exec.ExecContext(ctx, q, nil)
	if err != nil {
		t.Fatalf("ExecContext DROP failed: %v", err)
	}
}

// TestStatementExecutor_Others_Create_MarshalAndExec verifies CREATE path marshal+exec with TTIRPA then TTISTA.
// Expectation: ExecContext completes without error.
func TestStatementExecutor_Others_Create_MarshalAndExec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Pre-seed incoming wire: TTIRPA (CREATE) then TTISTA
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOall8RPAPayloadFromDump(validTTIRPACreateDump)); err != nil {
		t.Fatalf("write OALL8 RPA (CREATE) payload failed: %v", err)
	}
	if err := dbuf.WriteByteWithContext(ctx, byte(TTISTA)); err != nil {
		t.Fatalf("write TTISTA header failed: %v", err)
	}

	exec := newStatementExecutorOthers()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("CREATE TABLE t (x number)")
	_, err := exec.ExecContext(ctx, q, nil)
	if err != nil {
		t.Fatalf("ExecContext CREATE failed: %v", err)
	}
}

// TestStatementExecutor_DML_Insert_MarshalAndExec verifies DML INSERT marshal+exec using a seeded TTIRPA payload.
// Expectation: ExecContext completes without error.
func TestStatementExecutor_DML_Insert_MarshalAndExec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(16384)

	// Pre-seed incoming wire: TTIRPA (INSERT) then TTISTA
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOall8RPAPayloadFromDump(validTTIRPAInsertDump)); err != nil {
		t.Fatalf("write OALL8 RPA (INSERT) payload failed: %v", err)
	}

	exec := newStatementExecutorDML()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("INSERT INTO t (x) VALUES(1)")
	_, err := exec.ExecContext(ctx, q, nil)
	if err != nil {
		t.Fatalf("ExecContext INSERT failed: %v", err)
	}
}

func TestStatementExecutorDML_TTIFOBFlushesAndContinuesPull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(32768)

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIFOB)); err != nil {
		t.Fatalf("write TTIFOB header failed: %v", err)
	}
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOall8RPAPayloadFromDump(validTTIRPAInsertDump)); err != nil {
		t.Fatalf("write OALL8 RPA (INSERT) payload failed: %v", err)
	}

	exec := newStatementExecutorDML()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("INSERT INTO t (x) VALUES(1)")
	if _, err := exec.ExecContext(ctx, q, nil); err != nil {
		t.Fatalf("ExecContext INSERT with TTIFOB failed: %v", err)
	}

	if got := dbuf.bytes[dbuf.currentWritePosition-1]; got != byte(TTIFOB) {
		t.Fatalf("TTIFOB acknowledgement mismatch: got % X want % X", got, byte(TTIFOB))
	}
}

func TestStatementExecutor_DML_Insert_Prepared_MarshalAndExec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(16384)

	// Pre-seed incoming wire: TTIRPA payload for prepared INSERT
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOall8RPAPayloadFromDump(validInsertPreparedDump)); err != nil {
		t.Fatalf("write OALL8 RPA (INSERT prepared) payload failed: %v", err)
	}

	exec := newStatementExecutorDML()
	registerTestCodecs(shelf, 20)
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})

	table := "t_dml_prepared_ord"
	sql := "INSERT INTO " + table + " (id, name) VALUES(:1, :2)"
	args := []sqldriver.NamedValue{
		{Value: int64(1001), Ordinal: 1},
		{Value: "prepared", Ordinal: 2}}
	q, _ := newQualifiedSQLStatement(sql)
	if _, err := exec.ExecContext(ctx, q, args); err != nil {
		t.Fatalf("ExecContext INSERT prepared failed: %v", err)
	}
}

// TestStatementExecutor_Select_MarshalAndQuery verifies SELECT marshal+query over a full seeded stream.
// Expectation: QueryContext completes without error.
func TestStatementExecutor_Select_MarshalAndQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Pre-seed incoming wire with the full valid SELECT query stream dump.
	stream := Oall8Payload(validSelectQueryDump)
	if len(stream) == 0 {
		t.Fatal("validSelectQueryDump decode returned empty")
	}
	// Minimal priming: DCB header to start, then rest of stream which contains DCB/RXD/.. and OER-01403
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIDCB)); err != nil {
		t.Fatalf("write TTIDCB header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, stream); err != nil {
		t.Fatalf("write validSelectQueryDump stream failed: %v", err)
	}

	// Run QueryContext using the real path
	exec := newStatementExecutorSelect()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from t")
	if _, err := exec.QueryContext(ctx, q, nil); err != nil {
		t.Fatalf("QueryContext failed: %v", err)
	}
}

func TestStatementExecutor_Select_Prepared_MarshalAndQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Pre-seed incoming wire with the full valid SELECT prepared stream dump.
	stream := Oall8Payload(validSelectPreparedDump)
	if len(stream) == 0 {
		t.Fatal("validSelectPreparedDump decode returned empty")
	}
	// Minimal priming: DCB header to start, then rest of stream
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIDCB)); err != nil {
		t.Fatalf("write TTIDCB header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, stream); err != nil {
		t.Fatalf("write validSelectPreparedDump stream failed: %v", err)
	}

	// Run QueryContext using the real path with a prepared-style query
	exec := newStatementExecutorSelect()
	registerTestCodecs(shelf, 20)
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})

	table := "t_dml_prepared_ord"
	sql := "SELECT id, name FROM " + table + " WHERE id = :1"
	args := []sqldriver.NamedValue{
		{Value: int64(1001), Ordinal: 1}}
	q, _ := newQualifiedSQLStatement(sql)
	if _, err := exec.QueryContext(ctx, q, args); err != nil {
		t.Fatalf("QueryContext (prepared) failed: %v", err)
	}
}

// PL/SQL: Marshal and exec using statementExecutorPlSql with pre-seeded validplsqlResponse.
// TestStatementExecutor_PlSQL_MarshalAndExec verifies PL/SQL (BEGIN block) marshal+exec with seeded payload.
// Expectation: ExecContext completes without error.
func TestStatementExecutor_PlSQL_MarshalAndExec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Pre-seed incoming wire: TTIRPA payload from validplsqlResponse
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOall8RPAPayloadFromDump(validplsqlResponse)); err != nil {
		t.Fatalf("write OALL8 RPA (PLSQL) payload failed: %v", err)
	}

	plsql := "BEGIN " +
		"  INSERT INTO table1 (id, name) VALUES (10, 'aaa'); " +
		"  INSERT INTO table1 (id, name) VALUES (20, 'bbb'); " +
		"  UPDATE table1 SET name = 'ccc' WHERE id = 10; " +
		"END;"

	exec := newStatementExecutorPlSql()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement(plsql)
	if _, err := exec.ExecContext(ctx, q, nil); err != nil {
		t.Fatalf("ExecContext PLSQL failed: %v", err)
	}
}

// ------------------------------------
// Fault-injection error path tests
// ------------------------------------

// Query: Flush failure due to write error during header/message marshalling.
// TestStatementExecutor_Select_FaultyFlush verifies flush failures are surfaced during SELECT execution.
// Expectation: QueryContext returns an error containing "flush".
func TestStatementExecutor_Select_FaultyFlush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Any buffer; fail on first WriteByte to hit header MarshalTo
	shelf, _ := newFaultyExecShelf(nil, failOnWriteByte, 1)

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from dual")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "flush") {
		t.Fatalf("expected flush failure, got err=%v", err)
	}
}

// Query: Pull failure due to read error while unmarshalling next message header.
// TestStatementExecutor_Select_FaultyPull verifies pull failures are surfaced during SELECT execution.
// Expectation: QueryContext returns an error containing "pull".
func TestStatementExecutor_Select_FaultyPull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// runQuery pulls with TTIOER first; have versatileStreamer fail Pull for TTIOER.
	shelf := makeVersatilePullShelf(TTIOER, "Pull TTIOER failed")

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from dual")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "pull") {
		t.Fatalf("expected pull failure, got err=%v", err)
	}
}

// Query: OER error path (non-01403) should return error.
// TestStatementExecutor_Select_OER_Error verifies non-01403 OER errors are returned on SELECT.
// Expectation: QueryContext returns an error containing "ttioer".
func TestStatementExecutor_Select_OER_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Seed TTIOER header + payload with a generic error (not ORA-01403)
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIOER)); err != nil {
		t.Fatalf("write TTIOER header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOerErrPayload(14)); err != nil {
		t.Fatalf("write OER14 payload failed: %v", err)
	}

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from dual")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "table or view does not exist") {
		t.Fatalf("expected OER error, got err=%v", err)
	}
}

// A successful terminal OER without preceding DCB metadata is a malformed
// SELECT response and must not reach the rows dereference in QueryContext.
func TestStatementExecutor_Select_SuccessOERWithoutDCB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, _ := newExecTestShelf(1024)
	shelf.RegisterMessageStreamer(&mockStreamer{pullMsg: &mockOer{}})

	exec := newStatementExecutorSelect()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from dual")

	rows, err := exec.QueryContext(ctx, q, nil)
	if rows != nil {
		t.Fatalf("expected no rows for malformed SELECT response, got %v", rows)
	}
	if err == nil {
		t.Fatal("expected protocol violation for successful TTIOER before TTIDCB")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T: %v", err, err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.ProtocolViolation) {
		t.Fatalf("expected %s, got %s: %v", oracleErrors.ProtocolViolation, sqlErr.ErrorCode(), err)
	}
}

// Query: Push failure using versatileStreamer (simulate runQuery Push error)
// TestStatementExecutor_Select_FaultyPush verifies push failures are surfaced during SELECT execution.
// Expectation: QueryContext returns an error containing "push".
func TestStatementExecutor_Select_FaultyPush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, _ := newExecTestShelf(1024)

	// Cause Push to fail for TTIFUN messages
	vs := &versatileStreamer{
		failCode:  TTIFUN,
		failWhere: wherePush,
		failErr:   errors.New("Push TTIFUN failed"),
	}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from t")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("expected push failure (runQuery: failed to Push), got err=%v", err)
	}
}

// Exec (DML): Flush failure
// TestStatementExecutor_DML_FaultyFlush verifies flush failures are surfaced during DML execution.
// Expectation: ExecContext returns an error containing "flush".
func TestStatementExecutor_DML_FaultyFlush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _ := newFaultyExecShelf(nil, failOnWriteByte, 1)

	exec := &statementExecutorDML{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("insert into t values(1)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "flush") {
		t.Fatalf("expected flush failure on DML, got err=%v", err)
	}
}

// Exec (DML): Pull failure
// TestStatementExecutor_DML_FaultyPull verifies pull failures are surfaced during DML execution.
// Expectation: ExecContext returns an error containing "pull".
func TestStatementExecutor_DML_FaultyPull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// runExec pulls with TTIRPA first; fail Pull for TTIRPA.
	shelf := makeVersatilePullShelf(TTIRPA, "Pull TTIRPA failed")

	exec := &statementExecutorDML{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("insert into t values(1)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "pull") {
		t.Fatalf("expected pull failure on DML, got err=%v", err)
	}
}

// Exec (DML): OER error
// TestStatementExecutor_DML_OER_Error verifies OER errors are returned on DML execution.
// Expectation: ExecContext returns an error containing "ttioer".
func TestStatementExecutor_DML_OER_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	// Seed TTIOER header + payload
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIOER)); err != nil {
		t.Fatalf("write TTIOER header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOerErrPayload(14)); err != nil {
		t.Fatalf("write OER14 payload failed: %v", err)
	}

	exec := &statementExecutorDML{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("insert into t values(1)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "table or view does not exist") {
		t.Fatalf("expected OER error on DML, got err=%v", err)
	}
}

// Exec (Others): Flush failure
// TestStatementExecutor_Others_FaultyFlush verifies flush failures during Others execution.
// Expectation: ExecContext returns an error containing "flush".
func TestStatementExecutor_Others_FaultyFlush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _ := newFaultyExecShelf(nil, failOnWriteByte, 1)

	exec := &statementExecutorOthers{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("create table t(x number)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "flush") {
		t.Fatalf("expected flush failure on Others, got err=%v", err)
	}
}

// Exec (Others): Pull failure
// TestStatementExecutor_Others_FaultyPull verifies pull failures during Others execution.
// Expectation: ExecContext returns an error containing "pull".
func TestStatementExecutor_Others_FaultyPull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// runExec pulls with TTIRPA first; fail Pull for TTIRPA.
	shelf := makeVersatilePullShelf(TTIRPA, "Pull TTIRPA failed")

	exec := newStatementExecutorOthers()
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("create table t(x number)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "pull") {
		t.Fatalf("expected pull failure on Others, got err=%v", err)
	}
}

// Exec (Others): OER error
// TestStatementExecutor_Others_OER_Error verifies OER errors during Others execution.
// Expectation: ExecContext returns an error containing "ttioer".
func TestStatementExecutor_Others_OER_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIOER)); err != nil {
		t.Fatalf("write TTIOER header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeOerErrPayload(14)); err != nil {
		t.Fatalf("write OER14 payload failed: %v", err)
	}

	exec := &statementExecutorOthers{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("create table t(x number)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "table or view does not exist") {
		t.Fatalf("expected OER error on Others, got err=%v", err)
	}
}

// Integration: registerRunQueryCallbacks should fail when factory lacks TTIRXD registration.
// TestStatementExecutor_Select_Callback_GetMessage_RXD_Error_Integration verifies RXD factory absence is reported.
// Expectation: QueryContext returns an error indicating requested message type is unavailable.
func TestStatementExecutor_Select_Callback_GetMessage_RXD_Error_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Build payload: TTIDCB header + rest of SELECT stream from dump
	stream := Oall8Payload(validSelectQueryDump)
	if len(stream) == 0 {
		t.Fatal("validSelectQueryDump decode returned empty")
	}
	payload := append([]byte{byte(TTIDCB)}, stream...)

	// Use faulty shelf to seed incoming wire with payload (no faults injected).
	shelf, _ := newFaultyExecShelf(payload, 0, 0)

	// Replace message factory with one that DOES NOT register TTIRXD to force GetMessage(TTIRXD) error.
	funcReg := NewRegistry[functionRegistryKey]()
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, -1, NewOall)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 18, NewOall18)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oAll8}, -1, newTTIOallRPA)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, -1, newOexfen)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, 18, newOexfen18)

	msgReg := NewRegistry[common.MessageType]()
	_ = msgReg.Register(TTIOER, 14, newTTIoer14WithEndOfCallStatusSupport)
	_ = msgReg.Register(TTIDCB, 24, newTTIdcb24)
	_ = msgReg.Register(TTIRXH, 0, newTTIrxh)
	// Intentionally omit TTIRXD registration
	_ = msgReg.Register(TTIBVC, 0, newTTIbvc)

	shelf.RegisterMessageFactory(&SimpleFactory{
		ttcVersion:   24,
		msgregistry:  msgReg,
		funcregistry: funcReg,
	})

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from t")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil {
		t.Fatalf("expected RXD factory error (GetMessage TTIRXD), got err=%v", err)
	} else {
		sqlError, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("expected error to be SQLError")
		}
		if sqlError.ErrorCode() != string(oracleErrors.RunQueryError) {
			t.Fatalf("Expected error %s, but cas %s", oracleErrors.RunQueryError, sqlError.ErrorCode())
		}
	}
}

// Integration: registerRunQueryCallbacks should fail when factory lacks TTIBVC registration.
// TestStatementExecutor_Select_Callback_GetMessage_BVC_Error_Integration verifies BVC factory absence is reported.
// Expectation: QueryContext returns an error indicating requested message type is unavailable.
func TestStatementExecutor_Select_Callback_GetMessage_BVC_Error_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Build payload: TTIDCB header + rest of SELECT stream from dump
	stream := Oall8Payload(validSelectQueryDump)
	if len(stream) == 0 {
		t.Fatal("validSelectQueryDump decode returned empty")
	}
	payload := append([]byte{byte(TTIDCB)}, stream...)

	// Use faulty shelf to seed incoming wire with payload (no faults injected).
	shelf, _ := newFaultyExecShelf(payload, 0, 0)

	// Replace message factory with one that DOES NOT register TTIBVC to force GetMessage(TTIBVC) error.
	funcReg := NewRegistry[functionRegistryKey]()
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, -1, NewOall)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 18, NewOall18)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oAll8}, -1, newTTIOallRPA)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, -1, newOexfen)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oExfen}, 18, newOexfen18)

	msgReg := NewRegistry[common.MessageType]()
	_ = msgReg.Register(TTIOER, 14, newTTIoer14WithEndOfCallStatusSupport)
	_ = msgReg.Register(TTIDCB, 24, newTTIdcb24)
	_ = msgReg.Register(TTIRXH, 0, newTTIrxh)
	_ = msgReg.Register(TTIRXD, 0, newTTIrxd)
	// Intentionally omit TTIBVC registration

	shelf.RegisterMessageFactory(&SimpleFactory{
		ttcVersion:   24,
		msgregistry:  msgReg,
		funcregistry: funcReg,
	})

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from t")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil {
		t.Fatalf("expected BVC factory error (GetMessage TTIBVC), got err=%v", err)
	} else {
		sqlError, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("expected error to be SQLError")
		}
		if sqlError.ErrorCode() != string(oracleErrors.RunQueryError) {
			t.Fatalf("Expected error %s, but cas %s", oracleErrors.RunQueryError, sqlError.ErrorCode())
		}
	}
}

type capturingFactory struct {
	base            Factory
	rxdMsgs         []*tTIrxd
	oexfenRequested bool
}

func (f *capturingFactory) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	msg, err := f.base.GetMessage(msgType)
	if err == nil && msgType == TTIRXD {
		if rxd, ok := msg.(*tTIrxd); ok {
			f.rxdMsgs = append(f.rxdMsgs, rxd)
		}
	}
	return msg, err
}

func (f *capturingFactory) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	if msgType == TTIFUN && funcType == oExfen {
		f.oexfenRequested = true
	}
	return f.base.GetMessageForFunction(msgType, funcType)
}

// Composite factory that forces GetMessageForFunction(msgType, funcType) to fail for a specific pair,
// while delegating other calls to the wrapped base factory.
type compositeFuncFailFactory struct {
	base     Factory
	failMsg  common.MessageType
	failFunc common.FunctionType
	failErr  error
}

func (f *compositeFuncFailFactory) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	return f.base.GetMessage(msgType)
}
func (f *compositeFuncFailFactory) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	if msgType == f.failMsg && funcType == f.failFunc {
		return nil, f.failErr
	}
	return f.base.GetMessageForFunction(msgType, funcType)
}

// Integration: registerOallRpaCallback should fail when GetMessageForFunction(TTIRPA, oAll8) returns error.
// TestStatementExecutor_Select_OallRpaCallback_GetMessageForFunction_Error_Integration verifies OALLRPA GetMessageForFunction failure path.
// Expectation: QueryContext returns an error indicating the requested message type is unavailable.
func TestStatementExecutor_Select_OallRpaCallback_GetMessageForFunction_Error_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Prime incoming wire with TTIRPA header to trigger OALLRPA pre-unmarshal callback.
	payload := []byte{byte(TTIRPA)}
	shelf, _ := newFaultyExecShelf(payload, 0, 0)

	// Wrap the existing factory to fail only for GetMessageForFunction(TTIRPA, oAll8)
	base := shelf.GetMessageFactory().(Factory)
	shelf.RegisterMessageFactory(&compositeFuncFailFactory{
		base:     base,
		failMsg:  TTIRPA,
		failFunc: oAll8,
		failErr:  errors.New("boom OALLRPA"),
	})

	// Use a standard streamer; error is raised during callback on TTIRPA pre-unmarshal.
	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from t")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil {
		t.Fatalf("expected OALLRPA factory error, got err=%v", err)
	} else {
		sqlError, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("expected error to be SQLError")
		}
		if sqlError.ErrorCode() != string(oracleErrors.RunQueryError) {
			t.Fatalf("Expected error %s, but cas %s", oracleErrors.RunQueryError, sqlError.ErrorCode())
		}
	}
}

// -----------------------------
// Factory error simulation tests
// -----------------------------

// Factory that allows controlling GetMessageForFunction return values for TTIFUN/oAll8.
type factoryForOall8 struct {
	returnErr error
	returnMsg common.Message[common.MessageType]
}

func (f *factoryForOall8) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	return nil, errors.New("unused")
}

func (f *factoryForOall8) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return f.returnMsg, nil
}

// Exec (Others): fail when factory can't provide OALL8 message
// TestStatementExecutor_Others_Factory_GetMessageForFunction_Error verifies factory failure is surfaced for Others.
// Expectation: ExecContext returns an error containing "GetMessageForFunction(TTIFUN,oAll8) failed".
func TestStatementExecutor_Others_Factory_GetMessageForFunction_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	shelf := newShelf[common.MessageType]()
	// Register a basic marshaller to satisfy downstream interfaces (won't be used due to early failure).
	mar := NewMarshalEngine(NewArrayDataBuffer(256), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf.RegisterMarshaller(mar)

	f := &factoryForOall8{returnErr: errors.New("boom")}
	shelf.RegisterMessageFactory(f)

	// Register versatile streamer (not used before failure, but per request).
	vs := &versatileStreamer{}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorOthers{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("create table t(x number)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "failed to prepare") {
		t.Fatalf("expected factory GetMessageForFunction failure, got err=%v", err)
	}
}

// Exec (DML): fail when factory can't provide OALL8 message
// TestStatementExecutor_DML_Factory_GetMessageForFunction_Error verifies factory failure is surfaced for DML.
// Expectation: ExecContext returns an error containing "GetMessageForFunction(TTIFUN,oAll8) failed".
func TestStatementExecutor_DML_Factory_GetMessageForFunction_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	shelf := newShelf[common.MessageType]()
	mar := NewMarshalEngine(NewArrayDataBuffer(256), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf.RegisterMarshaller(mar)

	f := &factoryForOall8{returnErr: errors.New("boom")}
	shelf.RegisterMessageFactory(f)

	vs := &versatileStreamer{}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorDML{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("insert into t values(1)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "failed to prepare") {
		t.Fatalf("expected factory GetMessageForFunction failure for DML, got err=%v", err)
	}
}

// Query (Select): fail when factory can't provide OALL8 message
// TestStatementExecutor_Select_Factory_GetMessageForFunction_Error verifies factory failure is surfaced for Select.
// Expectation: QueryContext returns an error containing "GetMessageForFunction(TTIFUN,oAll8) failed".
func TestStatementExecutor_Select_Factory_GetMessageForFunction_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	shelf := newShelf[common.MessageType]()
	mar := NewMarshalEngine(NewArrayDataBuffer(256), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf.RegisterMarshaller(mar)

	f := &factoryForOall8{returnErr: errors.New("boom")}
	shelf.RegisterMessageFactory(f)

	vs := &versatileStreamer{}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorSelect{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("select * from t")
	if _, err := exec.QueryContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "failed to prepare") {
		t.Fatalf("expected factory GetMessageForFunction failure for Select, got err=%v", err)
	}
}

// Exec (PLSQL): fail when factory can't provide OALL8 message
// TestStatementExecutor_PLSQL_Factory_GetMessageForFunction_Error verifies factory failure surfaced for PL/SQL.
// Expectation: ExecContext returns an error containing "GetMessageForFunction(TTIFUN,oAll8) failed".
func TestStatementExecutor_PLSQL_Factory_GetMessageForFunction_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	shelf := newShelf[common.MessageType]()
	mar := NewMarshalEngine(NewArrayDataBuffer(256), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf.RegisterMarshaller(mar)

	f := &factoryForOall8{returnErr: errors.New("boom")}
	shelf.RegisterMessageFactory(f)

	vs := &versatileStreamer{}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorPlSql{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	plsql, _ := newQualifiedSQLStatement("BEGIN NULL; END;")
	if _, err := exec.ExecContext(ctx, plsql, nil); err == nil || !strings.Contains(err.Error(), "failed to prepare") {
		t.Fatalf("expected factory GetMessageForFunction failure for PLSQL, got err=%v", err)
	}
}

// Exec (DML): Push failure using versatileStreamer (simulate runExec Push error)
// TestStatementExecutor_DML_FaultyPush verifies push failures are surfaced during DML execution.
// Expectation: ExecContext returns an error containing "push".
func TestStatementExecutor_DML_FaultyPush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, _ := newExecTestShelf(1024)

	// Cause Push to fail for TTIFUN messages
	vs := &versatileStreamer{
		failCode:  TTIFUN,
		failWhere: wherePush,
		failErr:   errors.New("Push TTIFUN failed"),
	}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorDML{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("insert into t values(1)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("expected push failure (runExec: failed to Push), got err=%v", err)
	}
}

// Exec (Others): Push failure using versatileStreamer (simulate runExec Push error)
// TestStatementExecutor_Others_FaultyPush verifies push failures are surfaced during Others execution.
// Expectation: ExecContext returns an error containing "push".
func TestStatementExecutor_Others_FaultyPush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, _ := newExecTestShelf(1024)

	// Cause Push to fail for TTIFUN messages
	vs := &versatileStreamer{
		failCode:  TTIFUN,
		failWhere: wherePush,
		failErr:   errors.New("Push TTIFUN failed"),
	}
	shelf.RegisterMessageStreamer(vs)

	exec := &statementExecutorOthers{}
	exec.SetShelf(shelf)
	exec.SetSessionContext(&common.SessionContext{})
	q, _ := newQualifiedSQLStatement("create table t(x number)")
	if _, err := exec.ExecContext(ctx, q, nil); err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("expected push failure (runExec: failed to Push), got err=%v", err)
	}
}

func TestStatementExecutor_Select_DoesNotReuseStaleBVCStateAcrossExecutions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stream := Oall8Payload(validSelectPreparedDump)
	if len(stream) == 0 {
		t.Fatal("validSelectPreparedDump decode returned empty")
	}
	fullResponse := append([]byte{byte(TTIDCB)}, stream...)

	// OEXFEN re-execution responses reuse the cursor's DCB metadata and begin
	// with row/result messages. Decode the leading DCB once to obtain that shape.
	probeShelf, _, probeBuffer := newExecTestShelf(8192)
	if err := probeBuffer.WriteBytesWithContext(ctx, fullResponse); err != nil {
		t.Fatalf("seed DCB probe response: %v", err)
	}
	if _, err := probeBuffer.ReadByteWithContext(ctx); err != nil {
		t.Fatalf("read DCB probe header: %v", err)
	}
	dcbMessage, err := probeShelf.GetMessageFactory().GetMessage(TTIDCB)
	if err != nil {
		t.Fatalf("create DCB probe message: %v", err)
	}
	if err := dcbMessage.(common.UnMarshallable).UnMarshalFrom(ctx, probeShelf.GetMarshaller()); err != nil {
		t.Fatalf("decode DCB probe message: %v", err)
	}
	responseWithoutDCB := append([]byte(nil), fullResponse[probeBuffer.currentReadPosition:]...)
	if len(responseWithoutDCB) == 0 || common.MessageType(responseWithoutDCB[0]) == TTIDCB {
		t.Fatal("failed to build a response without leading DCB metadata")
	}

	newShelfWithResponse := func(execution int, response []byte) (*ttiShelf[common.MessageType], *capturingFactory) {
		shelf, _, dbuf := newExecTestShelf(8192)
		registerTestCodecs(shelf, 20)
		if err := dbuf.WriteBytesWithContext(ctx, response); err != nil {
			t.Fatalf("write SELECT response for execution %d failed: %v", execution, err)
		}
		baseFactory := shelf.GetMessageFactory().(Factory)
		captureFactory := &capturingFactory{base: baseFactory}
		shelf.RegisterMessageFactory(captureFactory)
		return shelf, captureFactory
	}

	firstShelf, firstCaptureFactory := newShelfWithResponse(1, fullResponse)
	exec := &statementExecutorSelect{
		statementProcessor: statementProcessor{
			shelf:   firstShelf,
			sessCtx: &common.SessionContext{},
		},
	}
	q, _ := newQualifiedSQLStatement("select * from t where id = :1")
	firstArgs := []sqldriver.NamedValue{{Ordinal: 1, Value: int64(1001)}}
	_, queryErr := exec.QueryContext(ctx, q, firstArgs)
	if len(firstCaptureFactory.rxdMsgs) == 0 {
		t.Fatalf("expected first execution to create an RXD message; QueryContext error: %v", queryErr)
	}
	if queryErr != nil {
		if sqlErr, ok := queryErr.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.NoDataFound) {
			t.Fatalf("first QueryContext returned unexpected error: %v", queryErr)
		}
	}

	secondShelf, secondCaptureFactory := newShelfWithResponse(2, responseWithoutDCB)
	exec.SetShelf(secondShelf)
	secondArgs := []sqldriver.NamedValue{{Ordinal: 1, Value: int64(1002)}}
	_, queryErr = exec.QueryContext(ctx, q, secondArgs)
	if !secondCaptureFactory.oexfenRequested {
		t.Fatal("expected second execution to use the OEXFEN fast path")
	}
	if len(secondCaptureFactory.rxdMsgs) < 2 {
		t.Fatalf("expected second execution to create an RXD message; QueryContext error: %v", queryErr)
	}

	// The first RXD of the second execution must not inherit row or BVC carry
	// state accumulated while decoding the first execution.
	// The first captured RXD contains outgoing bind data; the next one is the
	// first incoming result row configured by the query callback.
	rxd := secondCaptureFactory.rxdMsgs[1]
	if rxd.rowCount != 0 || rxd.bvcColSent != nil || rxd.bvcFound ||
		rxd.prevRow != nil || rxd.prevLobColContext != nil {
		t.Fatalf("SELECT decoding state leaked into the second execution: rowCount=%d bvcColSent=%v bvcFound=%t prevRow=%v prevLobColContext=%v",
			rxd.rowCount, rxd.bvcColSent, rxd.bvcFound, rxd.prevRow, rxd.prevLobColContext)
	}
	if rxd.numberOfColumns == 0 || len(rxd.columnContexts) != int(rxd.numberOfColumns) {
		t.Fatalf("cached column metadata was not reused: columns=%d contexts=%d",
			rxd.numberOfColumns, len(rxd.columnContexts))
	}
	if queryErr != nil {
		if sqlErr, ok := queryErr.(oracleErrors.SQLError); !ok || sqlErr.ErrorCode() != string(oracleErrors.NoDataFound) {
			t.Fatalf("second QueryContext returned unexpected error: %v", queryErr)
		}
	}
}
