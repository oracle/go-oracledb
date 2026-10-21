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
	cryptoRand "crypto/rand"
	"database/sql"
	"database/sql/driver"

	"errors"
	"strings"
	"testing"

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// newSessionlessTransactionTestConnection creates a capable connection and
// streamer for sessionless transaction tests.
func newSessionlessTransactionTestConnection() (*connection, *mockStreamer) {
	messageRegistry := NewRegistry[driverCommon.MessageType]()
	_ = messageRegistry.Register(TTIOER, 1, newTTIoer)
	_ = messageRegistry.Register(TTISTA, 1, newTTISTA)

	functionRegistry := NewRegistry[functionRegistryKey]()
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxSe}, 18, newOTxSe18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxEn}, 18, newOTxEn18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: oTxSe}, 18, newOTxSePfn18)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxSe}, 1, newOTxSeRPA)
	_ = functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxEn}, 1, newOTxEnRPA)

	messageFactory := &SimpleFactory{
		ttcVersion:   18,
		msgregistry:  messageRegistry,
		funcregistry: functionRegistry,
	}
	streamer := &mockStreamer{}
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).
		RegisterMessageStreamer(streamer).
		RegisterCapabilities(map[string]driverCommon.Capability{
			kpccapCtbTtc5SessionlessTxn: {IsSet: true},
		})
	sessionContext := driverCommon.NewSessionContext()
	sessionContext.GetSessionProperties().SetProperty(instanceName, "test-instance")

	return newTestConnection(shelf, sessionContext, nil), streamer
}

// sessionlessSyncEventData builds a SESSIONLESS_GTRID property-change event.
func sessionlessSyncEventData(t *testing.T, globalTransactionID string, mode byte, reason sessionlessTxSyncReason) eventData {
	t.Helper()
	raw := driverCommon.B1Array(append([]byte(globalTransactionID), mode|byte(reason), sessionlessGlobalTransactionIDSyncVersion))
	sync, err := newSessionlessGlobalTransactionIDSync(raw)
	if err != nil {
		t.Fatalf("newSessionlessGlobalTransactionIDSync failed: %v", err)
	}
	properties := driverCommon.NewProperties[string]()
	properties.SetProperty(sessionlessGlobalTransactionIDProperty, sync)
	return propertiesEventData{properties: properties}
}

// pushedOTxSe returns the single OTXSE message queued by a test operation.
func pushedOTxSe(t *testing.T, streamer *mockStreamer) *tTIOtxse {
	t.Helper()
	if streamer.pushedMsg.Len() != 1 {
		t.Fatalf("pushed messages = %d, want 1", streamer.pushedMsg.Len())
	}
	message, ok := streamer.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
	if !ok {
		t.Fatalf("pushed message value = %T, want TTC message pointer", streamer.pushedMsg.Front().Value)
	}
	otxse, ok := (*message).(*tTIOtxse)
	if !ok {
		t.Fatalf("pushed message = %T, want *tTIOtxse", *message)
	}
	return otxse
}

// TestConnectionBeginTxStartsOrResumesSessionlessTransaction verifies that a
// prepared start or resume is encoded correctly and becomes the active local
// transaction when BeginTx consumes it.
func TestConnectionBeginTxStartsOrResumesSessionlessTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resume    bool
		globalID  []byte
		wantFlags driverCommon.UB4
	}{
		{
			name:      "start",
			wantFlags: otxseTransNew | otxseTransReadWrite | otxseTransSessionless,
		},
		{
			name:      "resume",
			resume:    true,
			globalID:  []byte("resume-global-id"),
			wantFlags: otxseTransResume | otxseTransSessionless,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			ctx := context.Background()

			var preparedTx internalCommon.SessionlessTransaction
			var err error
			if test.resume {
				preparedTx, err = conn.PrepareForSessionlessResumeOperation(ctx, test.globalID, 45)
			} else {
				preparedTx, err = conn.PrepareForSessionlessStartOperation(ctx, 45)
			}
			if err != nil {
				t.Fatalf("prepare sessionless transaction: %v", err)
			}

			startedTx, err := conn.BeginTx(ctx, driver.TxOptions{})
			if err != nil {
				t.Fatalf("BeginTx returned error: %v", err)
			}
			if test.resume {
				conn.ClearPrepareForSessionlessResumeOperation(preparedTx, nil)
			}

			preparedSessionlessTx := preparedTx.(*sessionlessTransaction)
			startedSessionlessTx, ok := startedTx.(*sessionlessTransaction)
			if !ok || startedSessionlessTx != preparedSessionlessTx {
				t.Fatalf("BeginTx transaction = %T, want the prepared sessionless transaction", startedTx)
			}
			if conn.shelf.getTransaction() != preparedSessionlessTx {
				t.Fatal("BeginTx did not leave the prepared transaction registered")
			}
			if got := preparedSessionlessTx.currentTransactionState(); got != transactionStartedClient {
				t.Fatalf("transaction state = %d, want client-started", got)
			}
			if test.resume && !bytes.Equal(preparedTx.GlobalTransactionID(), test.globalID) {
				t.Fatalf("resumed global transaction ID = %q, want %q", preparedTx.GlobalTransactionID(), test.globalID)
			}

			message := pushedOTxSe(t, streamer)
			if message.operation != otxseStart || message.flags != test.wantFlags {
				t.Fatalf("OTXSE operation/flags = (%d, %#x), want (%d, %#x)",
					message.operation, message.flags, otxseStart, test.wantFlags)
			}
			if message.timeout != 45 || message.formatID != k2gSessionless {
				t.Fatalf("OTXSE timeout/format ID = (%d, %#x), want (45, %#x)",
					message.timeout, message.formatID, k2gSessionless)
			}
			xid, _, _ := preparedSessionlessTx.buildSessionlessXID()
			if !bytes.Equal(message.xid, xid) {
				t.Fatal("OTXSE XID differs from the prepared sessionless transaction")
			}
		})
	}
}

// TestConnectionBeginTxPushFailureClearsSessionlessPreparation verifies that
// a failed OTXSE start or resume leaves no prepared transaction on the shelf.
func TestConnectionBeginTxPushFailureClearsSessionlessPreparation(t *testing.T) {
	t.Parallel()

	for _, resume := range []bool{false, true} {
		name := "start"
		if resume {
			name = "resume"
		}
		t.Run(name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			streamer.pushErr = errors.New("write failed")
			ctx := context.Background()

			var preparedTx internalCommon.SessionlessTransaction
			var err error
			if resume {
				preparedTx, err = conn.PrepareForSessionlessResumeOperation(ctx, []byte("resume-id"), 30)
			} else {
				preparedTx, err = conn.PrepareForSessionlessStartOperation(ctx, 30)
			}
			if err != nil {
				t.Fatalf("prepare sessionless transaction: %v", err)
			}

			_, err = conn.BeginTx(ctx, driver.TxOptions{})
			if got, want := transactionErrorCode(t, err), oracleErrors.StreamerWriteError; got != want {
				t.Fatalf("BeginTx error code = %s, want %s", got, want)
			}
			if resume {
				conn.ClearPrepareForSessionlessResumeOperation(preparedTx, err)
			} else {
				conn.ClearPrepareForSessionlessStartOperation()
			}
			if conn.shelf.getTransaction() != nil {
				t.Fatal("failed BeginTx left a sessionless transaction registered")
			}
			if got := preparedTx.(*sessionlessTransaction).currentTransactionState(); got != transactionPrepared {
				t.Fatalf("failed BeginTx transaction state = %d, want prepared", got)
			}
		})
	}
}

// TestSessionlessTransactionSuspendGTRIDSyncEndsTransaction verifies that
// Suspend queues a detach followed by a rollback, and that a GTRID update
// returned with the rollback marks the transaction ended and unregisters it.
func TestSessionlessTransactionSuspendGTRIDSyncEndsTransaction(t *testing.T) {
	t.Parallel()

	conn, streamer := newSessionlessTransactionTestConnection()
	streamer.pullMsgs = []driverCommon.Message[driverCommon.MessageType]{newTTISTA()}
	streamer.pullHook = func(driverCommon.Message[driverCommon.MessageType]) {
		conn.handleSessionPropertyChange(sessionlessSyncEventData(
			t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessSyncStartSuspendClient))
	}
	tx := newSessionlessTransaction(context.Background(), conn, []byte("suspend-id"), 30)
	tx.transactionState(transactionStartedClient)
	conn.shelf.registerTransaction(tx)

	if err := tx.Suspend(); err != nil {
		t.Fatalf("Suspend returned error: %v", err)
	}
	if conn.shelf.getTransaction() != nil {
		t.Fatal("Suspend response with GTRID sync left the transaction registered")
	}
	if got := tx.currentTransactionState(); got != transactionEndedServer {
		t.Fatalf("transaction state after successful Suspend = %d, want server-ended", got)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("global transaction ID after successful Suspend = %q, want nil", got)
	}
	if got, want := streamer.pushedMsg.Len(), 2; got != want {
		t.Fatalf("Suspend pushed %d messages, want detach and rollback (%d)", got, want)
	}
	pushedMessages := make([]driverCommon.Message[driverCommon.MessageType], 0, 2)
	for element := streamer.pushedMsg.Front(); element != nil; element = element.Next() {
		message, ok := element.Value.(*driverCommon.Message[driverCommon.MessageType])
		if !ok || message == nil {
			t.Fatalf("pushed message value = %T, want TTC message pointer", element.Value)
		}
		pushedMessages = append(pushedMessages, *message)
	}
	suspendMessage, ok := pushedMessages[0].(*tTIOtxse)
	if !ok {
		t.Fatalf("first pushed message = %T, want *tTIOtxse suspend piggyback", pushedMessages[0])
	}
	if suspendMessage.GetMsgCode() != TTIPFN || suspendMessage.operation != otxseDetach || suspendMessage.flags != otxseTransSessionless || len(suspendMessage.xid) != 0 {
		t.Fatalf("suspend OTXSE fields = (message code %d, operation %d, flags %#x, XID length %d)",
			suspendMessage.GetMsgCode(), suspendMessage.operation, suspendMessage.flags, len(suspendMessage.xid))
	}
	rollbackMessage, ok := pushedMessages[1].(*tTIOtxen)
	if !ok {
		t.Fatalf("second pushed message = %T, want *tTIOtxen rollback", pushedMessages[1])
	}
	if rollbackMessage.GetMsgCode() != TTIFUN || rollbackMessage.GetFuncCode() != oTxEn || rollbackMessage.operation != driverCommon.SB4(otxenAbort) {
		t.Fatalf("rollback OTXEN fields = (message code %d, function code %d, operation %d), want TTIFUN/%d/abort",
			rollbackMessage.GetMsgCode(), rollbackMessage.GetFuncCode(), rollbackMessage.operation, oTxEn)
	}
	if len(streamer.pullMsgs) != 0 {
		t.Fatalf("Suspend left %d response messages unread", len(streamer.pullMsgs))
	}
}

// TestSessionlessTransactionSuspendFailureRestoresStartedState verifies that
// transport and server errors restore the prior state and keep the transaction
// registered.
func TestSessionlessTransactionSuspendFailureRestoresStartedState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*mockStreamer)
	}{
		{name: "push", setup: func(streamer *mockStreamer) { streamer.pushErr = errors.New("write failed") }},
		{name: "flush", setup: func(streamer *mockStreamer) { streamer.flushErr = errors.New("flush failed") }},
		{name: "pull", setup: func(streamer *mockStreamer) { streamer.pullErr = errors.New("read failed") }},
		{name: "server rejection", setup: func(streamer *mockStreamer) {
			streamer.pullMsgs = []driverCommon.Message[driverCommon.MessageType]{&mockOer{err: errors.New("detach rejected")}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			test.setup(streamer)
			tx := newSessionlessTransaction(context.Background(), conn, []byte("suspend-id"), 30)
			tx.transactionState(transactionStartedClient)
			conn.shelf.registerTransaction(tx)

			err := tx.Suspend()
			if got, want := transactionErrorCode(t, err), oracleErrors.ErrorInTransaction; got != want {
				t.Fatalf("Suspend error code = %s, want %s", got, want)
			}
			if conn.shelf.getTransaction() != tx {
				t.Fatal("failed Suspend removed the current transaction")
			}
			if got := tx.currentTransactionState(); got != transactionStartedClient {
				t.Fatalf("transaction state after failed Suspend = %d, want client-started", got)
			}
		})
	}
}

// TestSessionlessTransactionEndOperations covers successful and failed commit
// and rollback operations, plus repeated end calls after server confirmation.
func TestSessionlessTransactionEndOperations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		end       func(*sessionlessTransaction) error
		operation driverCommon.SB4
	}{
		{name: "commit", end: (*sessionlessTransaction).Commit, operation: driverCommon.SB4(otxenCommit)},
		{name: "rollback", end: (*sessionlessTransaction).Rollback, operation: driverCommon.SB4(otxenAbort)},
	} {
		t.Run(test.name+" succeeds", func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			streamer.pullMsgs = []driverCommon.Message[driverCommon.MessageType]{newTTISTA()}
			streamer.pullHook = func(driverCommon.Message[driverCommon.MessageType]) {
				conn.handleSessionPropertyChange(sessionlessSyncEventData(
					t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessSyncStartSuspendClient))
			}
			tx := newSessionlessTransaction(context.Background(), conn, []byte("end-id"), 30)
			tx.transactionState(transactionStartedClient)
			conn.shelf.registerTransaction(tx)

			if err := test.end(tx); err != nil {
				t.Fatalf("%s returned error: %v", test.name, err)
			}
			if conn.shelf.getTransaction() != nil {
				t.Fatal("server end notification left the transaction registered")
			}
			if got := tx.currentTransactionState(); got != transactionEndedServer {
				t.Fatalf("transaction state = %d, want server-ended", got)
			}
			if got := tx.GlobalTransactionID(); got != nil {
				t.Fatalf("global transaction ID = %q, want nil", got)
			}
			if got := streamer.pushedMsg.Len(); got != 1 {
				t.Fatalf("end operation pushed %d messages, want 1", got)
			}
			message := streamer.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
			otxen, ok := (*message).(*tTIOtxen)
			if !ok {
				t.Fatalf("end message = %T, want *tTIOtxen", *message)
			}
			if got := otxen.operation; got != test.operation {
				t.Fatalf("OTXEN operation = %d, want %d", got, test.operation)
			}
		})

		t.Run(test.name+" transport failure restores state", func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			streamer.pushErr = errors.New("write failed")
			tx := newSessionlessTransaction(context.Background(), conn, []byte("end-id"), 30)
			tx.transactionState(transactionStartedClient)
			conn.shelf.registerTransaction(tx)

			err := test.end(tx)
			if got, want := transactionErrorCode(t, err), oracleErrors.ErrorInTransaction; got != want {
				t.Fatalf("end operation error code = %s, want %s", got, want)
			}
			if got := tx.currentTransactionState(); got != transactionStartedClient {
				t.Fatalf("transaction state after failed end = %d, want client-started", got)
			}
			if conn.shelf.getTransaction() != tx {
				t.Fatal("failed end operation unregistered the transaction")
			}
		})
	}

	for _, test := range []struct {
		name string
		end  func(*sessionlessTransaction) error
	}{
		{name: "commit", end: (*sessionlessTransaction).Commit},
		{name: "rollback", end: (*sessionlessTransaction).Rollback},
		{name: "suspend", end: (*sessionlessTransaction).Suspend},
	} {
		t.Run(test.name+" already ended", func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			tx := newSessionlessTransaction(context.Background(), conn, []byte("end-id"), 30)
			tx.transactionState(transactionEndedServer)

			if err := test.end(tx); err != sql.ErrTxDone {
				t.Fatalf("%s after server end error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if streamer.pushCalled {
				t.Fatalf("%s sent a message after the server ended the transaction", test.name)
			}
		})
	}
}

// TestSessionlessTransactionPrepareValidation covers active-transaction,
// capability, and caller-supplied ID validation on both preparation paths.
func TestSessionlessTransactionPrepareValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		resume     bool
		active     bool
		capability bool
		globalID   []byte
		want       oracleErrors.ErrorCode
	}{
		{name: "start active transaction", active: true, capability: true, want: oracleErrors.AlreadyInTransaction},
		{name: "start unsupported capability", capability: false, want: oracleErrors.UnsupportedFeature},
		{name: "resume active transaction", resume: true, active: true, capability: true, globalID: []byte("id"), want: oracleErrors.AlreadyInTransaction},
		{name: "resume unsupported capability", resume: true, capability: false, globalID: []byte("id"), want: oracleErrors.UnsupportedFeature},
		{name: "resume empty ID", resume: true, capability: true, want: oracleErrors.InvalidGlobalTransactionIDValue},
		{name: "resume oversized ID", resume: true, capability: true, globalID: make([]byte, maxSessionlessGlobalTransactionIDSize+1), want: oracleErrors.InvalidGlobalTransactionIDValue},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			if !test.capability {
				conn.shelf.RegisterCapabilities(map[string]driverCommon.Capability{})
			}
			if test.active {
				conn.shelf.registerTransaction(newTransaction(conn, context.Background()))
			}

			var err error
			if test.resume {
				_, err = conn.PrepareForSessionlessResumeOperation(context.Background(), test.globalID, 30)
			} else {
				_, err = conn.PrepareForSessionlessStartOperation(context.Background(), 30)
			}
			if got := transactionErrorCode(t, err); got != test.want {
				t.Fatalf("error code = %s, want %s", got, test.want)
			}
			if streamer.pushCalled {
				t.Fatal("preparation validation unexpectedly pushed a message")
			}
		})
	}
}

// TestPrepareForSessionlessStartOperationRandomFailure verifies that an entropy
// source failure is returned without registering a prepared transaction. This
// test replaces crypto/rand.Reader and must not run in parallel.
func TestPrepareForSessionlessStartOperationRandomFailure(t *testing.T) {
	originalReader := cryptoRand.Reader
	cryptoRand.Reader = bytes.NewReader(nil)
	defer func() { cryptoRand.Reader = originalReader }()

	conn, streamer := newSessionlessTransactionTestConnection()
	tx, err := conn.PrepareForSessionlessStartOperation(context.Background(), 30)
	if err == nil {
		t.Fatal("PrepareForSessionlessStartOperation returned nil error for entropy failure")
	}
	if tx != nil {
		t.Fatalf("PrepareForSessionlessStartOperation returned transaction %p on error", tx)
	}
	if conn.shelf.getTransaction() != nil {
		t.Fatal("entropy failure left a transaction registered")
	}
	if streamer.pushCalled {
		t.Fatal("entropy failure unexpectedly pushed a message")
	}
}

// TestSessionlessTransactionPrepareCleanup checks that failed resume startup
// unregisters its provisional handle while clearing a successful preparation
// leaves the active handle registered.
func TestSessionlessTransactionPrepareCleanup(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	_, err := conn.PrepareForSessionlessStartOperation(context.Background(), 30)
	if err != nil {
		t.Fatalf("prepare start: %v", err)
	}
	conn.ClearPrepareForSessionlessStartOperation()
	if conn.shelf.getTransaction() != nil {
		t.Fatal("clearing a prepared start left a transaction registered")
	}

	resumeTx, err := conn.PrepareForSessionlessResumeOperation(context.Background(), []byte("resume-id"), 30)
	if err != nil {
		t.Fatalf("prepare resume: %v", err)
	}
	conn.ClearPrepareForSessionlessResumeOperation(resumeTx, errors.New("BeginTx failed"))
	if conn.shelf.getTransaction() != nil {
		t.Fatal("failed resume preparation left a transaction registered")
	}

	resumeTx, err = conn.PrepareForSessionlessResumeOperation(context.Background(), []byte("resume-id"), 30)
	if err != nil {
		t.Fatalf("prepare second resume: %v", err)
	}
	conn.ClearPrepareForSessionlessResumeOperation(resumeTx, nil)
	preparedResume, ok := resumeTx.(*sessionlessTransaction)
	if !ok {
		t.Fatalf("prepared resume has type %T, want *sessionlessTransaction", resumeTx)
	}
	if conn.shelf.getTransaction() != preparedResume {
		t.Fatal("successful resume preparation was removed from the connection")
	}
}

// TestSessionlessTransactionBuildsBoundedXID verifies that the TTC XID fields
// truncate oversized internal identifiers to their wire limits.
func TestSessionlessTransactionBuildsBoundedXID(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	globalID := bytes.Repeat([]byte{'g'}, maxSessionlessGlobalTransactionIDSize+8)
	instanceNameValue := strings.Repeat("i", maxSessionlessBQUALSize+8)
	conn.sessCtx.GetSessionProperties().SetProperty(instanceName, instanceNameValue)
	tx := newSessionlessTransaction(context.Background(), conn, globalID, 30)

	xid, globalTransactionIDLength, bqualLength := tx.buildSessionlessXID()

	if got, want := globalTransactionIDLength, driverCommon.UB4(maxSessionlessGlobalTransactionIDSize); got != want {
		t.Fatalf("global ID wire length = %d, want %d", got, want)
	}
	if got, want := bqualLength, driverCommon.UB4(maxSessionlessBQUALSize); got != want {
		t.Fatalf("BQUAL wire length = %d, want %d", got, want)
	}
	if !bytes.Equal(xid[:maxSessionlessGlobalTransactionIDSize], globalID[:maxSessionlessGlobalTransactionIDSize]) {
		t.Fatal("XID global transaction ID was not truncated at the wire limit")
	}
	if !bytes.Equal(xid[maxSessionlessGlobalTransactionIDSize:], []byte(instanceNameValue[:maxSessionlessBQUALSize])) {
		t.Fatal("XID BQUAL was not truncated at the wire limit")
	}
}

// TestSessionlessTransactionClientSyncLifecycle verifies that client sync
// updates the server state and ID, then clears and unregisters the handle when
// the server reports that the transaction ended.
func TestSessionlessTransactionClientSyncLifecycle(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx := newSessionlessTransaction(context.Background(), conn, []byte("client-id"), 30)
	tx.transactionState(transactionStartedClient)
	conn.shelf.registerTransaction(tx)

	conn.handleSessionPropertyChange(sessionlessSyncEventData(
		t, "server-client-id", sessionlessGlobalTransactionIDSyncSet, sessionlessSyncStartSuspendClient))
	if got := tx.currentTransactionState(); got != transactionStartedServer {
		t.Fatalf("transaction state after client start = %d, want %d", got, transactionStartedServer)
	}
	if string(tx.globalTransactionID) != "server-client-id" {
		t.Fatalf("global transaction ID = %q, want %q", tx.globalTransactionID, "server-client-id")
	}

	conn.handleSessionPropertyChange(sessionlessSyncEventData(
		t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessSyncStartSuspendClient))
	if got := tx.currentTransactionState(); got != transactionEndedServer {
		t.Fatalf("transaction state after client end = %d, want %d", got, transactionEndedServer)
	}
	if conn.shelf.getTransaction() != nil {
		t.Fatal("client end notification left the transaction registered")
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("global transaction ID after end = %q, want nil", got)
	}
}

// TestSessionlessServerSyncDoesNotEndClientTransaction protects a client-owned
// transaction from a server-initiated suspend notification.
func TestSessionlessServerSyncDoesNotEndClientTransaction(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx := newSessionlessTransaction(context.Background(), conn, []byte("client-id"), 30)
	tx.transactionState(transactionStartedServer)
	conn.shelf.registerTransaction(tx)

	conn.handleSessionPropertyChange(sessionlessSyncEventData(
		t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessSyncStartSuspendServer))
	if conn.shelf.getTransaction() != tx {
		t.Fatal("server suspend notification unregistered a client-owned transaction")
	}
	if got := tx.currentTransactionState(); got != transactionStartedServer {
		t.Fatalf("transaction state = %d, want %d", got, transactionStartedServer)
	}
}

// TestSessionlessGlobalTransactionIDSyncRejectsShortPayloads covers malformed
// SESSIONLESS_GTRID payloads that cannot contain both flags and version bytes.
func TestSessionlessGlobalTransactionIDSyncRejectsShortPayloads(t *testing.T) {
	t.Parallel()

	for _, raw := range []driverCommon.B1Array{nil, {}, {'x'}} {
		if _, err := newSessionlessGlobalTransactionIDSync(raw); err == nil {
			t.Fatalf("newSessionlessGlobalTransactionIDSync accepted short payload % X", raw)
		}
	}
}

// TestSessionlessGlobalTransactionIDSyncRejectsUnsupportedPayloadValues checks
// that decoding rejects identifiers above the supported limit and unknown
// payload versions.
func TestSessionlessGlobalTransactionIDSyncRejectsUnsupportedPayloadValues(t *testing.T) {
	t.Parallel()

	oversizedID := make(driverCommon.B1Array, maxSessionlessGlobalTransactionIDSize+3)
	oversizedID[len(oversizedID)-1] = sessionlessGlobalTransactionIDSyncVersion
	unsupportedVersion := driverCommon.B1Array{
		'x', sessionlessGlobalTransactionIDSyncSet | byte(sessionlessSyncStartSuspendServer),
		sessionlessGlobalTransactionIDSyncVersion + 1,
	}
	for name, raw := range map[string]driverCommon.B1Array{
		"oversized global ID": oversizedID,
		"unsupported version": unsupportedVersion,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newSessionlessGlobalTransactionIDSync(raw); err == nil {
				t.Fatalf("newSessionlessGlobalTransactionIDSync accepted %s", name)
			}
		})
	}
}

// TestSessionlessTransactionDetachResponsePaths verifies that detach queues a
// suspend piggyback and handles the rollback response paths.
func TestSessionlessTransactionDetachResponsePaths(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("detach rejected")
	tests := []struct {
		name     string
		pushErr  error
		flushErr error
		pull     []driverCommon.Message[driverCommon.MessageType]
		pullErr  error
		wantErr  error
		wantCode oracleErrors.ErrorCode
	}{
		{name: "OTXEN RPA then terminal status", pull: []driverCommon.Message[driverCommon.MessageType]{newOTxEnRPA(), newTTISTA()}},
		{name: "terminal OER", pull: []driverCommon.Message[driverCommon.MessageType]{&mockOer{err: wantErr}}, wantErr: wantErr},
		{name: "push failure", pushErr: errors.New("write failed"), wantCode: oracleErrors.StreamerWriteError},
		{name: "flush failure", flushErr: errors.New("flush failed"), wantCode: oracleErrors.StreamerWriteError},
		{name: "pull failure", pullErr: errors.New("read failed"), wantCode: oracleErrors.StreamerReadError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			streamer.pushErr = test.pushErr
			streamer.flushErr = test.flushErr
			streamer.pullMsgs = test.pull
			streamer.pullErr = test.pullErr
			tx := newSessionlessTransaction(context.Background(), conn, []byte("detach-id"), 30)

			err := tx.detachTransaction(context.Background())
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("detachTransaction error = %v, want %v", err, test.wantErr)
			}
			if test.wantCode != "" {
				if got := transactionErrorCode(t, err); got != test.wantCode {
					t.Fatalf("detachTransaction error code = %s, want %s", got, test.wantCode)
				}
			} else if test.wantErr == nil && err != nil {
				t.Fatalf("detachTransaction returned error: %v", err)
			}
			if !streamer.pushCalled {
				t.Fatal("detach did not push the suspend piggyback")
			}
			if streamer.flushCalled != (test.pushErr == nil) {
				t.Fatalf("Flush called = %t, want %t", streamer.flushCalled, test.pushErr == nil)
			}
			if streamer.pullCalled != (test.pushErr == nil && test.flushErr == nil) {
				t.Fatalf("Pull called = %t, want %t", streamer.pullCalled, test.pushErr == nil && test.flushErr == nil)
			}
			if len(streamer.pullMsgs) != 0 {
				t.Fatalf("detach left %d server response messages unread", len(streamer.pullMsgs))
			}

			wantPushed := 2
			if test.pushErr != nil {
				wantPushed = 1
			}
			if got := streamer.pushedMsg.Len(); got != wantPushed {
				t.Fatalf("detach pushed %d messages, want %d", got, wantPushed)
			}
			messages := make([]driverCommon.Message[driverCommon.MessageType], 0, wantPushed)
			for element := streamer.pushedMsg.Front(); element != nil; element = element.Next() {
				message, ok := element.Value.(*driverCommon.Message[driverCommon.MessageType])
				if !ok || message == nil {
					t.Fatalf("pushed message value = %T, want TTC message pointer", element.Value)
				}
				messages = append(messages, *message)
			}
			suspend, ok := messages[0].(*tTIOtxse)
			if !ok {
				t.Fatalf("first pushed message = %T, want *tTIOtxse suspend piggyback", messages[0])
			}
			if suspend.GetMsgCode() != TTIPFN || suspend.GetFuncCode() != oTxSe ||
				suspend.operation != otxseDetach || suspend.flags != otxseTransSessionless || len(suspend.xid) != 0 {
				t.Fatalf("suspend OTXSE fields = (message code %d, function code %d, operation %d, flags %#x, XID length %d)",
					suspend.GetMsgCode(), suspend.GetFuncCode(), suspend.operation, suspend.flags, len(suspend.xid))
			}
			if test.pushErr == nil {
				rollback, ok := messages[1].(*tTIOtxen)
				if !ok {
					t.Fatalf("second pushed message = %T, want *tTIOtxen rollback", messages[1])
				}
				if rollback.GetMsgCode() != TTIFUN || rollback.GetFuncCode() != oTxEn || rollback.operation != driverCommon.SB4(otxenAbort) {
					t.Fatalf("rollback OTXEN fields = (message code %d, function code %d, operation %d), want TTIFUN/%d/abort",
						rollback.GetMsgCode(), rollback.GetFuncCode(), rollback.operation, oTxEn)
				}
			}
		})
	}
}

// TestSessionlessTransactionDetachSetupFailures covers errors before the
// suspend piggyback can be queued.
func TestSessionlessTransactionDetachSetupFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*connection, *mockStreamer)
		wantCause string
	}{
		{
			name: "streamer lacks callback support",
			setup: func(conn *connection, streamer *mockStreamer) {
				conn.shelf.RegisterMessageStreamer(streamerWithoutCallbacks{Streamer: streamer})
			},
		},
		{
			name: "factory error",
			setup: func(conn *connection, _ *mockStreamer) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
			},
			wantCause: "factory failed",
		},
		{
			name: "unexpected message type",
			setup: func(conn *connection, _ *mockStreamer) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnMsg: &mockOer{}})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, streamer := newSessionlessTransactionTestConnection()
			test.setup(conn, streamer)
			tx := newSessionlessTransaction(context.Background(), conn, []byte("detach-id"), 30)

			err := tx.detachTransaction(context.Background())
			if got, want := transactionErrorCode(t, err), oracleErrors.InternalError; got != want {
				t.Fatalf("detach error code = %s, want %s", got, want)
			}
			if test.wantCause != "" && !strings.Contains(err.Error(), test.wantCause) {
				t.Fatalf("detach error = %v, want it to include %q", err, test.wantCause)
			}
			if streamer.pushCalled {
				t.Fatal("detach queued a message after setup failed")
			}
		})
	}
}

type streamerWithoutCallbacks struct {
	driverCommon.Streamer[driverCommon.MessageType]
}
