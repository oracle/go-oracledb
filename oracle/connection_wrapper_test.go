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

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// sessionlessTransactionTestConnector exposes a test driver connection to
// database/sql without registering a real driver name.
type sessionlessTransactionTestConnector struct {
	// connection is the fake connection returned to database/sql.
	connection driver.Conn
}

// Connect returns the configured fake connection.
func (c sessionlessTransactionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return c.connection, nil
}

// Driver returns the test driver descriptor.
func (c sessionlessTransactionTestConnector) Driver() driver.Driver {
	return sessionlessTransactionTestDriver{}
}

// sessionlessTransactionTestDriver is the driver descriptor used by wrapper tests.
type sessionlessTransactionTestDriver struct{}

// Open is unsupported because tests inject an already-created connection.
func (sessionlessTransactionTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("sessionless test driver does not use Open")
}

// sessionlessTransactionTestConn implements the wrapper interface and records
// whether database/sql reaches BeginTx after a failed resume preparation.
type sessionlessTransactionTestConn struct {
	// resumeErr is returned by the prepared-resume operation.
	resumeErr error
	// driverTxCalls counts calls made to BeginTx.
	driverTxCalls int
}

// PrepareForSessionlessStartOperation implements the wrapper interface.
func (c *sessionlessTransactionTestConn) PrepareForSessionlessStartOperation(
	context.Context, uint16,
) (common.SessionlessTransaction, error) {
	return nil, nil
}

// ClearPrepareForSessionlessStartOperation implements the wrapper interface.
func (*sessionlessTransactionTestConn) ClearPrepareForSessionlessStartOperation() {}

// PrepareForSessionlessResumeOperation returns the configured preparation error.
func (c *sessionlessTransactionTestConn) PrepareForSessionlessResumeOperation(
	context.Context, []byte, uint16,
) (common.SessionlessTransaction, error) {
	return nil, c.resumeErr
}

// ClearPrepareForSessionlessResumeOperation implements the wrapper interface.
func (*sessionlessTransactionTestConn) ClearPrepareForSessionlessResumeOperation(common.SessionlessTransaction, error) {
}

// Prepare reports that statements are unsupported by the fake connection.
func (*sessionlessTransactionTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

// Close closes the fake connection.
func (*sessionlessTransactionTestConn) Close() error { return nil }

// Begin reports that the legacy transaction API is unused by these tests.
func (*sessionlessTransactionTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

// BeginTx counts calls made by database/sql and returns an empty result.
func (c *sessionlessTransactionTestConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	c.driverTxCalls++
	return nil, nil
}

// sessionlessTransactionTestPlainConn is a connection that cannot be wrapped.
type sessionlessTransactionTestPlainConn struct{}

// Prepare reports that statements are unsupported by the plain connection.
func (*sessionlessTransactionTestPlainConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

// Close closes the plain fake connection.
func (*sessionlessTransactionTestPlainConn) Close() error { return nil }

// Begin reports that the legacy transaction API is unsupported.
func (*sessionlessTransactionTestPlainConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

// openSessionlessTransactionTestConnection opens an injected driver connection.
func openSessionlessTransactionTestConnection(t *testing.T, conn driver.Conn) *sql.Conn {
	t.Helper()
	db := sql.OpenDB(sessionlessTransactionTestConnector{connection: conn})
	t.Cleanup(func() { _ = db.Close() })

	sqlConn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get test connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlConn.Close() })
	return sqlConn
}

// openSessionlessTransactionTestConnectionWrapper opens and wraps an injected connection.
func openSessionlessTransactionTestConnectionWrapper(t *testing.T, conn driver.Conn) *connectionWrapper {
	t.Helper()
	sqlConn := openSessionlessTransactionTestConnection(t, conn)
	wrapper, err := NewConnectionWrapper(sqlConn)
	if err != nil {
		t.Fatalf("wrap test connection: %v", err)
	}
	return wrapper
}

// newSessionlessTransactionTestConnection builds a fake connection for wrapper tests.
func newSessionlessTransactionTestConnection() *sessionlessTransactionTestConn {
	return &sessionlessTransactionTestConn{}
}

// TestResumeSessionlessTxPropagatesError verifies that a preparation error is
// returned before database/sql begins a transaction.
func TestResumeSessionlessTxPropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("prepare sessionless resume failed")
	driverConn := newSessionlessTransactionTestConnection()
	driverConn.resumeErr = wantErr
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

	gotTx, err := connectionWrapper.ResumeSessionlessTx(
		context.Background(), GlobalTransactionID("resume-global-transaction-id"), 300)
	if gotTx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %p on error", gotTx)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSessionlessTx error = %v, want %v", err, wantErr)
	}
	if driverConn.driverTxCalls != 0 {
		t.Fatalf("database/sql BeginTx calls = %d, want 0 after preparation failure", driverConn.driverTxCalls)
	}
}

// TestSessionlessTransactionWrappersRejectUnsupportedConnection verifies that
// the public wrapper rejects connections without sessionless support.
func TestSessionlessTransactionWrappersRejectUnsupportedConnection(t *testing.T) {
	t.Parallel()

	sqlConn := openSessionlessTransactionTestConnection(t, &sessionlessTransactionTestPlainConn{})
	if _, err := NewConnectionWrapper(sqlConn); err == nil {
		t.Fatal("NewConnectionWrapper unexpectedly accepted an unsupported connection")
	} else {
		sqlError, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("unexpected wrapper error type %T: %v", err, err)
		}
		if sqlError.ErrorCode() != string(oracleErrors.UnsupportedFeature) {
			t.Fatalf("unexpected wrapper error code: %s", sqlError.ErrorCode())
		}
	}
}

// TestGlobalTransactionIDString verifies that the public transaction ID uses
// hexadecimal encoding for its string representation.
func TestGlobalTransactionIDString(t *testing.T) {
	t.Parallel()

	if got, want := (GlobalTransactionID{0x01, 0xab, 0xff}).String(), "01abff"; got != want {
		t.Fatalf("GlobalTransactionID.String() = %q, want %q", got, want)
	}
}
