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
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

type mockStreamerWithFlushError struct {
	*mockStreamer
	flushErr error
}

func (m *mockStreamerWithFlushError) Flush(_ context.Context) error {
	return m.flushErr
}

func newSessionlessTransactionTestConnection() (*Connection, *mockStreamer) {
	messageRegistry := NewRegistry[common.MessageType]()
	_ = messageRegistry.Register(TTIOER, 1, NewTTIoer)
	_ = messageRegistry.Register(TTISTA, 1, newTTISTA)

	functionRegistry := NewRegistry[functionRegistryKey]()
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxSe}, 18, newOTxSe18)

	messageFactory := &SimpleFactory{
		ttcVersion:   18,
		msgregistry:  messageRegistry,
		funcregistry: functionRegistry,
	}
	mockStr := &mockStreamer{pullMsg: &mockOer{}}
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).RegisterMessageStreamer(mockStr)

	return newTestConnection(shelf, nil, nil), mockStr
}

// TestBeginSessionlessTx verifies that starting a new sessionless transaction
// returns the Oracle-specific SessionlessTx contract and sends an OTXSE start
// request with the generated GTRID and new-sessionless flags.
func TestBeginSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()

	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}
	if tx == nil {
		t.Fatal("BeginSessionlessTx returned nil transaction")
	}
	if !tx.IsSessionlessTx() {
		t.Fatal("BeginSessionlessTx returned a transaction that is not marked sessionless")
	}

	oracleTx, ok := tx.(*transaction)
	if !ok {
		t.Fatalf("BeginSessionlessTx returned %T, want *transaction", tx)
	}
	if got := oracleTx.GlobalTransactionID(); got == "" {
		t.Fatal("BeginSessionlessTx returned an empty GTRID")
	} else if len([]byte(got)) != 16 {
		t.Fatalf("generated GTRID length = %d, want 16", len([]byte(got)))
	}

	if mockStr.pushedMsg.Len() != 2 {
		t.Fatalf("pushed message count = %d, want 2", mockStr.pushedMsg.Len())
	}

	first := mockStr.pushedMsg.Front()
	setupMsg := first.Value.(*common.Message[common.MessageType])
	if _, ok := (*setupMsg).(*tTIOall); !ok {
		t.Fatalf("first pushed message = %T, want *tTIOall", *setupMsg)
	}

	second := first.Next()
	if second == nil {
		t.Fatal("missing OTXSE message")
	}
	pushedMsg := second.Value.(*common.Message[common.MessageType])
	otxse, ok := (*pushedMsg).(*tTIOtxse)
	if !ok {
		t.Fatalf("second pushed message = %T, want *tTIOtxse", *pushedMsg)
	}
	if otxse.operation != otxseStart {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseStart)
	}
	if otxse.flags != otxseTransSessionless|otxseTransNew {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, otxseTransSessionless|otxseTransNew)
	}
	if otxse.formatID != k2gSessionless {
		t.Fatalf("OTXSE formatID = %#x, want %#x", otxse.formatID, k2gSessionless)
	}
	if got := string(otxse.xid); got != oracleTx.GlobalTransactionID() {
		t.Fatalf("OTXSE xid = %q, want %q", got, oracleTx.GlobalTransactionID())
	}
	if otxse.gtridLength != common.UB4(len(otxse.xid)) {
		t.Fatalf("OTXSE gtridLength = %d, want %d", otxse.gtridLength, len(otxse.xid))
	}
}

// TestResumeSessionlessTx verifies that resuming a sessionless transaction
// preserves the caller-supplied GTRID and sends an OTXSE start request with the
// resume-sessionless flags.
func TestResumeSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	gtrid := "resume-gtrid"

	tx, err := conn.ResumeSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	}, gtrid)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx failed: %v", err)
	}
	if tx == nil {
		t.Fatal("ResumeSessionlessTx returned nil transaction")
	}
	if !tx.IsSessionlessTx() {
		t.Fatal("ResumeSessionlessTx returned a transaction that is not marked sessionless")
	}
	if got := tx.GlobalTransactionID(); got != gtrid {
		t.Fatalf("GlobalTransactionID = %q, want %q", got, gtrid)
	}

	if mockStr.pushedMsg.Len() != 2 {
		t.Fatalf("pushed message count = %d, want 2", mockStr.pushedMsg.Len())
	}

	second := mockStr.pushedMsg.Front().Next()
	if second == nil {
		t.Fatal("missing OTXSE message")
	}
	pushedMsg := second.Value.(*common.Message[common.MessageType])
	otxse, ok := (*pushedMsg).(*tTIOtxse)
	if !ok {
		t.Fatalf("second pushed message = %T, want *tTIOtxse", *pushedMsg)
	}
	if otxse.operation != otxseStart {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseStart)
	}
	if otxse.flags != otxseTransSessionless|otxseTransResume {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, otxseTransSessionless|otxseTransResume)
	}
	if got := string(otxse.xid); got != gtrid {
		t.Fatalf("OTXSE xid = %q, want %q", got, gtrid)
	}
	if otxse.gtridLength != common.UB4(len(gtrid)) {
		t.Fatalf("OTXSE gtridLength = %d, want %d", otxse.gtridLength, len(gtrid))
	}
}

// TestBeginSessionlessTxPushFailure verifies that a write failure during the
// OTXSE start request is surfaced and unregisters the partially started
// transaction.
func TestBeginSessionlessTxPushFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pushErr = errors.New("push failed")

	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err == nil {
		t.Fatal("BeginSessionlessTx returned nil error on push failure")
	}
	if tx != nil {
		t.Fatalf("BeginSessionlessTx returned transaction %T on push failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("BeginSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("BeginSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx push failure should unregister the transaction")
	}
}

// TestBeginSessionlessTxFlushFailure verifies that a flush failure during the
// OTXSE start request is surfaced and unregisters the partially started
// transaction.
func TestBeginSessionlessTxFlushFailure(t *testing.T) {
	t.Parallel()

	baseConn, baseMock := newSessionlessTransactionTestConnection()
	streamer := &mockStreamerWithFlushError{mockStreamer: baseMock}
	baseConn.shelf.RegisterMessageStreamer(streamer)
	streamer.flushErr = errors.New("flush failed")

	tx, err := baseConn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err == nil {
		t.Fatal("BeginSessionlessTx returned nil error on flush failure")
	}
	if tx != nil {
		t.Fatalf("BeginSessionlessTx returned transaction %T on flush failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("BeginSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("BeginSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if baseConn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx flush failure should unregister the transaction")
	}
}

// TestBeginSessionlessTxPullFailure verifies that a read failure while waiting
// for the OTXSE start response is surfaced and unregisters the transaction.
func TestBeginSessionlessTxPullFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullErr = errors.New("pull failed")

	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err == nil {
		t.Fatal("BeginSessionlessTx returned nil error on pull failure")
	}
	if tx != nil {
		t.Fatalf("BeginSessionlessTx returned transaction %T on pull failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("BeginSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("BeginSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx pull failure should unregister the transaction")
	}
}

// TestBeginSessionlessTxOERFailure verifies that a server-side start error is
// surfaced and unregisters the transaction locally.
func TestBeginSessionlessTxOERFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullMsg = &mockOer{err: common.NewOERMessageError("ORA-24776", "start failed")}

	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err == nil {
		t.Fatal("BeginSessionlessTx returned nil error on OER failure")
	}
	if tx != nil {
		t.Fatalf("BeginSessionlessTx returned transaction %T on OER failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("BeginSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("BeginSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("BeginSessionlessTx OER failure should unregister the transaction")
	}
}

// TestResumeSessionlessTxInvalidGTRID verifies that invalid caller-provided
// identifiers are rejected before any transaction is started.
func TestResumeSessionlessTxInvalidGTRID(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()

	tx, err := conn.ResumeSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	}, "")
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error for invalid gtrid")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T for invalid gtrid, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.InvalidGTRIDValue) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.InvalidGTRIDValue)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx invalid gtrid should not register a transaction")
	}
}

// TestResumeSessionlessTxPushFailure verifies that a write failure during the
// OTXSE resume request is surfaced and unregisters the partially started
// transaction.
func TestResumeSessionlessTxPushFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pushErr = errors.New("push failed")

	tx, err := conn.ResumeSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	}, "resume-gtrid")
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error on push failure")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T on push failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx push failure should unregister the transaction")
	}
}

// TestResumeSessionlessTxFlushFailure verifies that a flush failure during the
// OTXSE resume request is surfaced and unregisters the partially started
// transaction.
func TestResumeSessionlessTxFlushFailure(t *testing.T) {
	t.Parallel()

	baseConn, baseMock := newSessionlessTransactionTestConnection()
	streamer := &mockStreamerWithFlushError{mockStreamer: baseMock}
	baseConn.shelf.RegisterMessageStreamer(streamer)
	streamer.flushErr = errors.New("flush failed")

	tx, err := baseConn.ResumeSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	}, "resume-gtrid")
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error on flush failure")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T on flush failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if baseConn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx flush failure should unregister the transaction")
	}
}

// TestResumeSessionlessTxPullFailure verifies that a read failure while waiting
// for the OTXSE resume response is surfaced and unregisters the transaction.
func TestResumeSessionlessTxPullFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullErr = errors.New("pull failed")

	tx, err := conn.ResumeSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	}, "resume-gtrid")
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error on pull failure")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T on pull failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx pull failure should unregister the transaction")
	}
}

// TestResumeSessionlessTxOERFailure verifies that a server-side resume error is
// surfaced and unregisters the transaction locally.
func TestResumeSessionlessTxOERFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	mockStr.pullMsg = &mockOer{err: common.NewOERMessageError("ORA-24776", "resume failed")}

	tx, err := conn.ResumeSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	}, "resume-gtrid")
	if err == nil {
		t.Fatal("ResumeSessionlessTx returned nil error on OER failure")
	}
	if tx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %T on OER failure, want nil", tx)
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("ResumeSessionlessTx error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ConfigureTransactionError) {
		t.Fatalf("ResumeSessionlessTx error code = %q, want %q", sqlErr.ErrorCode(), common.ConfigureTransactionError)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("ResumeSessionlessTx OER failure should unregister the transaction")
	}
}

// TestSuspendSessionlessTx verifies that suspending the active sessionless
// transaction uses the immediate OTXSE detach path, clears the transaction
// GTRID, and unregisters the transaction from the connection shelf.
func TestSuspendSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	mockStr.pushedMsg.Init()

	if err := tx.Suspend(); err != nil {
		t.Fatalf("Suspend failed: %v", err)
	}
	if tx.IsSessionlessTx() {
		t.Fatal("Suspend should clear the sessionless transaction flag")
	}
	if got := tx.GlobalTransactionID(); got != "" {
		t.Fatalf("GlobalTransactionID after suspend = %q, want empty", got)
	}
	if conn.shelf.isInTransaction() {
		t.Fatal("Suspend should unregister the active transaction from the shelf")
	}
	if mockStr.pushedMsg.Len() != 1 {
		t.Fatalf("pushed message count = %d, want 1", mockStr.pushedMsg.Len())
	}

	pushedMsg := mockStr.pushedMsg.Front().Value.(*common.Message[common.MessageType])
	otxse, ok := (*pushedMsg).(*tTIOtxse)
	if !ok {
		t.Fatalf("pushed message = %T, want *tTIOtxse", *pushedMsg)
	}
	if otxse.operation != otxseDetach {
		t.Fatalf("OTXSE operation = %d, want %d", otxse.operation, otxseDetach)
	}
	if otxse.flags != otxseTransSessionless {
		t.Fatalf("OTXSE flags = %#x, want %#x", otxse.flags, otxseTransSessionless)
	}
	if len(otxse.xid) != 0 {
		t.Fatalf("OTXSE xid length = %d, want 0", len(otxse.xid))
	}
	if otxse.gtridLength != 0 {
		t.Fatalf("OTXSE gtridLength = %d, want 0", otxse.gtridLength)
	}
	if otxse.timeout != 0 {
		t.Fatalf("OTXSE timeout = %d, want 0", otxse.timeout)
	}
}

// TestSuspendNonSessionlessTx verifies that Suspend rejects regular
// transactions that do not carry a sessionless GTRID.
func TestSuspendNonSessionlessTx(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	sessionlessTx, ok := tx.(SessionlessTx)
	if !ok {
		t.Fatalf("BeginTx returned %T, want value implementing SessionlessTx", tx)
	}

	err = sessionlessTx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil for non-sessionless transaction")
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.NotInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), common.NotInTransaction)
	}
}

// TestSuspendSessionlessTxPushFailure verifies that a write failure while
// sending the OTXSE detach leaves the local sessionless transaction registered
// so the caller can still recover explicitly.
func TestSuspendSessionlessTxPushFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	gtrid := tx.GlobalTransactionID()
	mockStr.pushedMsg.Init()
	mockStr.pushErr = errors.New("push failed")

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on push failure")
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), common.ErrorInTransaction)
	}
	if !tx.IsSessionlessTx() {
		t.Fatal("Suspend push failure should keep transaction marked sessionless")
	}
	if got := tx.GlobalTransactionID(); got != gtrid {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want %q", got, gtrid)
	}
	if !conn.shelf.isInTransaction() {
		t.Fatal("Suspend push failure should keep the transaction registered")
	}
}

// TestSuspendSessionlessTxFlushFailure verifies that a flush failure while
// sending the OTXSE detach leaves the active sessionless transaction intact.
func TestSuspendSessionlessTxFlushFailure(t *testing.T) {
	t.Parallel()

	baseConn, baseMock := newSessionlessTransactionTestConnection()
	streamer := &mockStreamerWithFlushError{
		mockStreamer: baseMock,
	}
	baseConn.shelf.RegisterMessageStreamer(streamer)

	tx, err := baseConn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	gtrid := tx.GlobalTransactionID()
	streamer.pushedMsg.Init()
	streamer.flushErr = errors.New("flush failed")

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on flush failure")
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), common.ErrorInTransaction)
	}
	if !tx.IsSessionlessTx() {
		t.Fatal("Suspend flush failure should keep transaction marked sessionless")
	}
	if got := tx.GlobalTransactionID(); got != gtrid {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want %q", got, gtrid)
	}
	if !baseConn.shelf.isInTransaction() {
		t.Fatal("Suspend flush failure should keep the transaction registered")
	}
}

// TestSuspendSessionlessTxPullFailure verifies that a read failure while waiting
// for the detach response does not clear the local sessionless transaction.
func TestSuspendSessionlessTxPullFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	gtrid := tx.GlobalTransactionID()
	mockStr.pushedMsg.Init()
	mockStr.pullErr = errors.New("pull failed")

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on pull failure")
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), common.ErrorInTransaction)
	}
	if !tx.IsSessionlessTx() {
		t.Fatal("Suspend pull failure should keep transaction marked sessionless")
	}
	if got := tx.GlobalTransactionID(); got != gtrid {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want %q", got, gtrid)
	}
	if !conn.shelf.isInTransaction() {
		t.Fatal("Suspend pull failure should keep the transaction registered")
	}
}

// TestSuspendSessionlessTxOERFailure verifies that a server-side detach error is
// surfaced and leaves the active sessionless transaction registered locally.
func TestSuspendSessionlessTxOERFailure(t *testing.T) {
	t.Parallel()

	conn, mockStr := newSessionlessTransactionTestConnection()
	tx, err := conn.BeginSessionlessTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})
	if err != nil {
		t.Fatalf("BeginSessionlessTx failed: %v", err)
	}

	gtrid := tx.GlobalTransactionID()
	mockStr.pushedMsg.Init()
	mockStr.pullMsg = &mockOer{err: common.NewOERMessageError("ORA-24776", "detach failed")}

	err = tx.Suspend()
	if err == nil {
		t.Fatal("Suspend returned nil on OER failure")
	}
	sqlErr, ok := err.(common.SQLError)
	if !ok {
		t.Fatalf("Suspend error type = %T, want common.SQLError", err)
	}
	if sqlErr.ErrorCode() != string(common.ErrorInTransaction) {
		t.Fatalf("Suspend error code = %q, want %q", sqlErr.ErrorCode(), common.ErrorInTransaction)
	}
	if !tx.IsSessionlessTx() {
		t.Fatal("Suspend OER failure should keep transaction marked sessionless")
	}
	if got := tx.GlobalTransactionID(); got != gtrid {
		t.Fatalf("GlobalTransactionID after failed suspend = %q, want %q", got, gtrid)
	}
	if !conn.shelf.isInTransaction() {
		t.Fatal("Suspend OER failure should keep the transaction registered")
	}
}
