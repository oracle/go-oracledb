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
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestTransactionCommitSuccess create a transaction and commits with different
// combination of isolation level and read only options and checks that the
// correct SQL is executed and that the correct error message is returned on the
// second commit when the transaction is not longer active.
func TestTransactionCommitSuccess(t *testing.T) {
	t.Parallel()
	messageRegistry := NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIOER, 1, newTTIoer)
	functionRegistry := NewRegistry[functionRegistryKey]()
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: commit}, 1, newCommit)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	messageFactory := &SimpleFactory{ttcVersion: 1, msgregistry: messageRegistry, funcregistry: functionRegistry}
	mockStr := &mockStreamer{}
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).RegisterMessageStreamer(mockStr)

	conn := newTestConnection(shelf, nil, nil)

	// Clean error message so that connection close succeeds
	mockStr.pullMsg = &mockOer{}
	tests := []struct {
		isolationLevel sql.IsolationLevel
		readOnly       bool
		expectedSQL    []string
	}{
		{
			isolationLevel: sql.LevelReadCommitted,
			readOnly:       true,
			expectedSQL:    []string{_isolationLevelReadCommitted, _transactionReadOnly},
		},
		{
			isolationLevel: sql.LevelReadCommitted,
			readOnly:       false,
			expectedSQL:    []string{_isolationLevelReadCommitted},
		},
		{
			isolationLevel: sql.LevelSerializable,
			readOnly:       true,
			expectedSQL:    []string{_isolationLevelSerializable, _transactionReadOnly},
		},
		{
			isolationLevel: sql.LevelSerializable,
			readOnly:       false,
			expectedSQL:    []string{_isolationLevelSerializable},
		},
	}

	for _, testItem := range tests {
		mockStr.pushedMsg.Init()

		tx, err := conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(testItem.isolationLevel), ReadOnly: testItem.readOnly})
		if err != nil {
			t.Fatalf("Unexpected error %v", err)
		}

		if mockStr.pushedMsg.Len() != len(testItem.expectedSQL) {
			t.Errorf("Wrong number of statements pushed, expected %d transaction setup statements but was %d", len(testItem.expectedSQL), mockStr.pushedMsg.Len())
		}

		// Check that OAll8 was pushed for transaction setup.
		setupElement := mockStr.pushedMsg.Front()
		for _, expectedSQL := range testItem.expectedSQL {
			if setupElement == nil {
				t.Errorf("Missing transaction setup statement %s", expectedSQL)
				break
			}
			pushedMsg := setupElement.Value.(*common.Message[common.MessageType])
			oAll8, ok := (*pushedMsg).(*tTIOall)
			if !ok {
				t.Errorf("Message pushed was not oAll8")
				setupElement = setupElement.Next()
				continue
			}
			if sql := common.B1ArrayToString(oAll8.sql); sql != expectedSQL {
				t.Errorf("Expected transaction setup statement %s, but was %s", expectedSQL, sql)
			}
			if oAll8.options&commitAfterExecution != 0 {
				t.Errorf("Transaction setup should not commit after execution, options=%x", oAll8.options)
			}
			setupElement = setupElement.Next()
		}

		mockStr.pushedMsg.Init()
		err = tx.Commit()
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}
		msg := mockStr.pushedMsg.Front().Value.(*common.Message[common.MessageType])
		msgHeader, ok := (*msg).(*ttiFunHeader)
		if !ok {
			t.Errorf("Message pushed was not ttiFunHeader")
		}
		if msgHeader._funcType != commit {
			t.Errorf("Expected function type to be %d, but was %d", commit, msgHeader._funcType)
		}
		// try to commit again and check that not in transaction is thrown
		err = tx.Commit()
		if err == nil {
			t.Errorf("Transaction should be closed, expected error")
		}
		if sqlErr, ok := err.(oracleErrors.SQLError); ok {
			if sqlErr.ErrorCode() != string(oracleErrors.NotInTransaction) {
				t.Errorf("Wrong error, expected %s but was %s", oracleErrors.NotInTransaction, sqlErr.ErrorCode())
			}
		}
		// try to rollback and check that not in transaction is thrown
		err = tx.Rollback()
		if err == nil {
			t.Errorf("Transaction should be closed, expected error")
		}
		if sqlErr, ok := err.(oracleErrors.SQLError); ok {
			if sqlErr.ErrorCode() != string(oracleErrors.NotInTransaction) {
				t.Errorf("Wrong error, expected %s but was %s", oracleErrors.NotInTransaction, sqlErr.ErrorCode())
			}
		}
	}
}

// TestTransactionRollbackSuccess create a transaction and rollbacks with different
// combination of isolation level and read only options and checks that the
// correct SQL is executed and that the correct error message is returned on the
// second rollback when the transaction is not longer active.
func TestTransactionRollbackSuccess(t *testing.T) {
	t.Parallel()
	messageRegistry := NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIOER, 1, newTTIoer)
	functionRegistry := NewRegistry[functionRegistryKey]()
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: rollback}, 1, newRollback)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	messageFactory := &SimpleFactory{ttcVersion: 1, msgregistry: messageRegistry, funcregistry: functionRegistry}
	mockStr := &mockStreamer{pullMsg: &mockOer{err: nil}}
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).RegisterMessageStreamer(mockStr)

	conn := newTestConnection(shelf, nil, nil)

	// Clean error message so that connection close succeeds
	mockStr.pullMsg = &mockOer{}
	tests := []struct {
		isolationLevel sql.IsolationLevel
		readOnly       bool
		expectedSQL    []string
	}{
		{
			isolationLevel: sql.LevelReadCommitted,
			readOnly:       true,
			expectedSQL:    []string{_isolationLevelReadCommitted, _transactionReadOnly},
		},
		{
			isolationLevel: sql.LevelReadCommitted,
			readOnly:       false,
			expectedSQL:    []string{_isolationLevelReadCommitted},
		},
		{
			isolationLevel: sql.LevelSerializable,
			readOnly:       true,
			expectedSQL:    []string{_isolationLevelSerializable, _transactionReadOnly},
		},
		{
			isolationLevel: sql.LevelSerializable,
			readOnly:       false,
			expectedSQL:    []string{_isolationLevelSerializable},
		},
	}

	for _, testItem := range tests {
		mockStr.pushedMsg.Init()
		tx, err := conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(testItem.isolationLevel), ReadOnly: testItem.readOnly})
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}

		if mockStr.pushedMsg.Len() != len(testItem.expectedSQL) {
			t.Errorf("Wrong number of statements pushed, expected %d transaction setup statements but was %d", len(testItem.expectedSQL), mockStr.pushedMsg.Len())
		}

		// Check that OAll8 was pushed for transaction setup.
		setupElement := mockStr.pushedMsg.Front()
		for _, expectedSQL := range testItem.expectedSQL {
			if setupElement == nil {
				t.Errorf("Missing transaction setup statement %s", expectedSQL)
				break
			}
			pushedMsg := setupElement.Value.(*common.Message[common.MessageType])
			oAll8, ok := (*pushedMsg).(*tTIOall)
			if !ok {
				t.Errorf("Message pushed was not oAll8")
				setupElement = setupElement.Next()
				continue
			}
			if sql := common.B1ArrayToString(oAll8.sql); sql != expectedSQL {
				t.Errorf("Expected transaction setup statement %s, but was %s", expectedSQL, sql)
			}
			if oAll8.options&commitAfterExecution != 0 {
				t.Errorf("Transaction setup should not commit after execution, options=%x", oAll8.options)
			}
			setupElement = setupElement.Next()
		}

		mockStr.pushedMsg.Init()
		err = tx.Rollback()
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}
		msg := mockStr.pushedMsg.Front().Value.(*common.Message[common.MessageType])
		msgHeader, ok := (*msg).(*ttiFunHeader)
		if !ok {
			t.Errorf("Message pushed was not ttiFunHeader")
		}
		if msgHeader._funcType != rollback {
			t.Errorf("Expected function type to be %d, but was %d", commit, msgHeader._funcType)
		}
		// try to commit again and check that not in transaction is thrown
		err = tx.Rollback()
		if err == nil {
			t.Errorf("Transaction should be closed, expected error")
		}
		if sqlErr, ok := err.(oracleErrors.SQLError); ok {
			if sqlErr.ErrorCode() != string(oracleErrors.NotInTransaction) {
				t.Errorf("Wrong error, expected %s but was %s", oracleErrors.NotInTransaction, sqlErr.ErrorCode())
			}
		}
	}
}

// TestCallBeginTxTwice calls beginTx twice in the same connection. Check for
// already in a transaction error
func TestCallBeginTxTwice(t *testing.T) {
	t.Parallel()
	mockNs := &mockNetworkSession{disconnectCalls: 0, disconnectErr: nil, sleepDuration: 0}
	messageRegistry := NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIOER, 1, newTTIoer)
	functionRegistry := NewRegistry[functionRegistryKey]()
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: logOff}, 1, newLogOff)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: rollback}, 1, newRollback)
	functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oAll8}, 1, NewOall18)
	messageFactory := &SimpleFactory{ttcVersion: 1, msgregistry: messageRegistry, funcregistry: functionRegistry}
	mockStr := &mockStreamer{pullMsg: &mockOer{err: nil}}
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(messageFactory).RegisterMessageStreamer(mockStr)

	conn := newTestConnection(shelf, nil, mockNs)

	// Clean error message so that connection close succeeds
	mockStr.pullMsg = &mockOer{}
	_, err := conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted), ReadOnly: false})
	if err != nil {
		t.Fatalf("Unexpected error starting transaction %v", err)
	}

	_, err = conn.BeginTx(context.Background(), driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted), ReadOnly: false})
	if err == nil {
		t.Fatalf("Expected already in transaction error")
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("Expected error to be OracleError")
	}
	if sqlErr.ErrorCode() != string(oracleErrors.AlreadyInTransaction) {
		t.Fatalf("Wrong error expected %s, but was %s", oracleErrors.AlreadyInTransaction, sqlErr.ErrorCode())
	}

}
