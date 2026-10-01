/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"bytes"
	"context"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

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

// TestSessionlessServerSyncLifecycle verifies that an implicit transaction is
// registered only for a server start and removed only by its matching end.
func TestSessionlessServerSyncLifecycle(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	conn.handleSessionPropertyChange(sessionlessSyncEventData(t, "server-id", sessionlessGlobalTransactionIDSyncSet, sessionlessGlobalTransactionIDSyncServer))

	implicit, ok := conn.shelf.getTransaction().(*sessionlessTransaction)
	if !ok {
		t.Fatalf("current transaction = %T, want *sessionlessTransaction", conn.shelf.getTransaction())
	}
	if implicit.transactionState != transactionStartedServer {
		t.Fatal("implicit transaction was not marked as started on the server")
	}

	conn.handleSessionPropertyChange(sessionlessSyncEventData(t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessGlobalTransactionIDSyncServer))
	if conn.shelf.getTransaction() != nil {
		t.Fatal("matching server end notification did not unregister the implicit transaction")
	}
}

// TestSessionlessServerEndDoesNotEndClientTransaction verifies that the empty
// server end notification cannot unregister a client-owned transaction.
func TestSessionlessServerEndDoesNotEndClientTransaction(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx := newSessionlessTransaction(context.Background(), conn, []byte("client-id"), 300)
	conn.shelf.registerTransaction(tx)

	conn.handleSessionPropertyChange(sessionlessSyncEventData(t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessGlobalTransactionIDSyncServer))
	if conn._isValid {
		t.Fatal("server end notification should invalidate the connection")
	}
	if conn.shelf.getTransaction() != tx {
		t.Fatal("client-owned transaction was unregistered by server end notification")
	}
}

// TestSessionlessClientSyncUpdatesTransactionState verifies that client-side
// session-property notifications update the transaction ID and server-state
// flags without unregistering the client-owned transaction.
func TestSessionlessClientSyncUpdatesTransactionState(t *testing.T) {
	t.Parallel()

	conn, _ := newSessionlessTransactionTestConnection()
	tx := newSessionlessTransaction(context.Background(), conn, []byte("client-id"), 300)
	conn.shelf.registerTransaction(tx)

	// An invalid server ID is ignored and must not mark the transaction started.
	tx.setStartedOnServer(nil)
	if tx.transactionState != transactionStartedClient {
		t.Fatal("invalid server start ID marked the transaction as started")
	}

	conn.handleSessionPropertyChange(sessionlessSyncEventData(
		t, "server-client-id", sessionlessGlobalTransactionIDSyncSet, sessionlessGlobalTransactionIDSyncClient))
	if tx.transactionState != transactionStartedServer {
		t.Fatal("client start notification did not mark the transaction as started")
	}
	if !bytes.Equal(tx.globalTransactionID, []byte("server-client-id")) {
		t.Fatalf("global transaction ID = %q, want %q", tx.globalTransactionID, "server-client-id")
	}

	conn.handleSessionPropertyChange(sessionlessSyncEventData(
		t, "", sessionlessGlobalTransactionIDSyncUnset, sessionlessGlobalTransactionIDSyncClient))
	if tx.transactionState != transactionEndedServer {
		t.Fatal("client end notification did not mark the transaction as ended")
	}
	if conn.shelf.getTransaction() != tx {
		t.Fatal("client synchronization notification unregistered the transaction")
	}
}

// TestSessionlessGlobalTransactionIDSyncRejectsMalformedPayload verifies that
// invalid synchronization metadata cannot create or alter transaction state.
func TestSessionlessGlobalTransactionIDSyncRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  driverCommon.B1Array
	}{
		{name: "missing payload", raw: nil},
		{name: "empty server start ID", raw: driverCommon.B1Array{sessionlessGlobalTransactionIDSyncSet | byte(sessionlessGlobalTransactionIDSyncServer), sessionlessGlobalTransactionIDSyncVersion}},
		{name: "unsupported version", raw: driverCommon.B1Array{'i', sessionlessGlobalTransactionIDSyncSet | byte(sessionlessGlobalTransactionIDSyncServer), 1}},
		{name: "unsupported mode", raw: driverCommon.B1Array{'i', byte(sessionlessGlobalTransactionIDSyncServer), sessionlessGlobalTransactionIDSyncVersion}},
		{name: "unsupported reason", raw: driverCommon.B1Array{'i', sessionlessGlobalTransactionIDSyncSet | 3, sessionlessGlobalTransactionIDSyncVersion}},
		{name: "oversized ID", raw: append(driverCommon.B1Array(bytes.Repeat([]byte{'i'}, maxSessionlessGlobalTransactionIDSize+1)), sessionlessGlobalTransactionIDSyncSet|byte(sessionlessGlobalTransactionIDSyncServer), sessionlessGlobalTransactionIDSyncVersion)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newSessionlessGlobalTransactionIDSync(test.raw); err == nil {
				t.Fatal("malformed synchronization payload was accepted")
			}
		})
	}
}
