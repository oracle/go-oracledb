/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 */

package ttc

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/extensions"
)

// TestBeginSessionlessTxValidationPaths verifies that BeginSessionlessTx
// rejects active transactions, unsupported capabilities, and isolation levels.
func TestBeginSessionlessTxValidationPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*connection)
		opts  sql.TxOptions
		want  oracleErrors.ErrorCode
	}{
		{
			name: "already in transaction",
			setup: func(conn *connection) {
				conn.shelf.registerTransaction(newTransaction(conn, context.Background()))
			},
			want: oracleErrors.AlreadyInTransaction,
		},
		{
			name: "unsupported capability",
			setup: func(conn *connection) {
				conn.shelf.RegisterCapabilities(nil)
			},
			want: oracleErrors.UnsupportedFeature,
		},
		{
			name: "unsupported isolation level",
			opts: sql.TxOptions{Isolation: sql.IsolationLevel(12345)},
			want: oracleErrors.IsolationLevelNotSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			if tt.setup != nil {
				tt.setup(conn)
			}

			_, err := conn.BeginSessionlessTx(context.Background(), tt.opts, 300)
			if got := transactionErrorCode(t, err); got != tt.want {
				t.Fatalf("BeginSessionlessTx error code = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestResumeSessionlessTxValidationPaths verifies that ResumeSessionlessTx
// rejects active transactions and unsupported sessionless capabilities.
func TestResumeSessionlessTxValidationPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*connection)
		want  oracleErrors.ErrorCode
	}{
		{
			name: "already in transaction",
			setup: func(conn *connection) {
				conn.shelf.registerTransaction(newTransaction(conn, context.Background()))
			},
			want: oracleErrors.AlreadyInTransaction,
		},
		{
			name: "unsupported capability",
			setup: func(conn *connection) {
				conn.shelf.RegisterCapabilities(nil)
			},
			want: oracleErrors.UnsupportedFeature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			tt.setup(conn)

			_, err := conn.ResumeSessionlessTx(context.Background(), extensions.GlobalTransactionID("resume-id"))
			if got := transactionErrorCode(t, err); got != tt.want {
				t.Fatalf("ResumeSessionlessTx error code = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestBuildSessionlessXIDPaths verifies XID construction when session
// properties are unavailable and when transaction identifiers are truncated.
func TestBuildSessionlessXIDPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		configure      func(*connection)
		globalID       extensions.GlobalTransactionID
		wantGlobalSize int
		wantBqualSize  int
	}{
		{
			name: "missing session context",
			configure: func(conn *connection) {
				conn.sessCtx = nil
			},
			globalID:       extensions.GlobalTransactionID("global-id"),
			wantGlobalSize: len("global-id"),
		},
		{
			name: "missing instance name",
			configure: func(conn *connection) {
				conn.sessCtx = driverCommon.NewSessionContext()
			},
			globalID:       extensions.GlobalTransactionID("global-id"),
			wantGlobalSize: len("global-id"),
		},
		{
			name: "truncated identifiers",
			configure: func(conn *connection) {
				conn.sessCtx.GetSessionProperties().SetProperty(instanceName, strings.Repeat("i", maxSessionlessBQUALSize+1))
			},
			globalID:       extensions.GlobalTransactionID(strings.Repeat("g", maxSessionlessGlobalTransactionIDSize+1)),
			wantGlobalSize: maxSessionlessGlobalTransactionIDSize,
			wantBqualSize:  maxSessionlessBQUALSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			tt.configure(conn)
			tx := newSessionlessTransaction(context.Background(), conn, tt.globalID, 300)

			if got := int(tx.globalTransactionIDLength); got != tt.wantGlobalSize {
				t.Fatalf("global transaction ID length = %d, want %d", got, tt.wantGlobalSize)
			}
			if got := int(tx.bqualLength); got != tt.wantBqualSize {
				t.Fatalf("BQUAL length = %d, want %d", got, tt.wantBqualSize)
			}
			if !slices.Equal([]byte(tx.xid[:tt.wantGlobalSize]), []byte(tt.globalID[:tt.wantGlobalSize])) {
				t.Fatalf("XID global transaction ID does not match the expected prefix")
			}
		})
	}
}

// TestSessionlessTransactionServerStatePaths verifies client synchronization
// updates with matching and mismatched global transaction IDs.
func TestSessionlessTransactionServerStatePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setState    func(*sessionlessTransaction, extensions.GlobalTransactionID)
		initialID   string
		syncID      string
		wantID      string
		wantStarted bool
		wantEnded   bool
	}{
		{
			name:        "matching start",
			setState:    (*sessionlessTransaction).setStartedOnServer,
			initialID:   "same-id",
			syncID:      "same-id",
			wantID:      "same-id",
			wantStarted: true,
		},
		{
			name:        "mismatched start",
			setState:    (*sessionlessTransaction).setStartedOnServer,
			initialID:   "client-id",
			syncID:      "server-id",
			wantID:      "server-id",
			wantStarted: true,
		},
		{
			name:      "matching end",
			setState:  (*sessionlessTransaction).setEndedOnServer,
			initialID: "same-id",
			syncID:    "same-id",
			wantID:    "same-id",
			wantEnded: true,
		},
		{
			name:      "mismatched end",
			setState:  (*sessionlessTransaction).setEndedOnServer,
			initialID: "client-id",
			syncID:    "server-id",
			wantID:    "client-id",
			wantEnded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			tx := newSessionlessTransaction(context.Background(), conn, extensions.GlobalTransactionID(tt.initialID), 300)

			tt.setState(tx, extensions.GlobalTransactionID(tt.syncID))
			if got := string(tx.globalTransactionID); got != tt.wantID {
				t.Fatalf("global transaction ID = %q, want %q", got, tt.wantID)
			}
			if tx.isStartedOnServer != tt.wantStarted {
				t.Fatalf("startedOnServer = %v, want %v", tx.isStartedOnServer, tt.wantStarted)
			}
			if tx.isEndedOnServer != tt.wantEnded {
				t.Fatalf("endedOnServer = %v, want %v", tx.isEndedOnServer, tt.wantEnded)
			}
		})
	}
}

// TestResumeSessionlessTxHelperSetupFailures verifies errors creating the
// streamer and OTXSE message used by resumeSessionlessTx.
func TestResumeSessionlessTxHelperSetupFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*connection)
	}{
		{
			name: "streamer without callbacks",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageStreamer(&nonCallbackStreamer{})
			},
		},
		{
			name: "factory error",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
			},
		},
		{
			name: "unexpected message type",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			tt.setup(conn)
			tx := newSessionlessTransaction(context.Background(), conn, extensions.GlobalTransactionID("resume-id"), 300)

			if got := transactionErrorCode(t, conn.resumeSessionlessTx(context.Background(), tx)); got != oracleErrors.InternalError {
				t.Fatalf("resumeSessionlessTx error code = %s, want %s", got, oracleErrors.InternalError)
			}
		})
	}
}

// TestDetachSessionlessTxSetupFailures verifies errors creating the streamer
// and OTXSE message used by detachTransaction.
func TestDetachSessionlessTxSetupFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*connection)
	}{
		{
			name: "streamer without callbacks",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageStreamer(&nonCallbackStreamer{})
			},
		},
		{
			name: "factory error",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnErr: errors.New("factory failed")})
			},
		},
		{
			name: "unexpected message type",
			setup: func(conn *connection) {
				conn.shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, _ := newSessionlessTransactionTestConnection()
			tt.setup(conn)

			if got := transactionErrorCode(t, conn.detachTransaction(context.Background())); got != oracleErrors.InternalError {
				t.Fatalf("detachTransaction error code = %s, want %s", got, oracleErrors.InternalError)
			}
		})
	}
}

// TestDetachSessionlessTxAdditionalResponsePaths verifies that detach waits
// past TTIRPA and also accepts TTISTA as the terminal response.
func TestDetachSessionlessTxAdditionalResponsePaths(t *testing.T) {
	t.Parallel()

	streamer := &mockStreamer{
		pullMsgs: []driverCommon.Message[driverCommon.MessageType]{
			newOTxSeRPA(),
			newTTISTA(),
		},
	}
	conn, _ := newSessionlessTransactionTestConnection()
	conn.shelf.RegisterMessageStreamer(streamer)

	if err := conn.detachTransaction(context.Background()); err != nil {
		t.Fatalf("detachTransaction returned error: %v", err)
	}
	if streamer.pushedMsg.Len() != 1 {
		t.Fatalf("detachTransaction pushed %d messages, want 1", streamer.pushedMsg.Len())
	}
}
