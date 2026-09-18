/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** and patent rights owned by each licensor hereunder covering either (i) the
** unmodified Software as contributed to or provided by such licensor, or (ii)
** the Larger Works (as defined below), to deal in both (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors), without restriction, including without
** limitation the rights to copy, create derivative works of, display, perform,
** and distribute the Software and the Larger Work(s), and to sublicense the
** foregoing rights on either these or other terms.
**
** This license is subject to the condition that the above copyright notice and
** either this complete permission notice or at a minimum a reference to the UPL
** must be included in all copies or substantial portions of the Software.
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

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/extensions"
)

type sessionlessTransactionTestConnector struct {
	connection driver.Conn
}

func (c sessionlessTransactionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return c.connection, nil
}

func (c sessionlessTransactionTestConnector) Driver() driver.Driver {
	return sessionlessTransactionTestDriver{}
}

type sessionlessTransactionTestDriver struct{}

func (sessionlessTransactionTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("sessionless test driver does not use Open")
}

type sessionlessTransactionTestConn struct {
	beginTx      extensions.SessionlessTx
	beginErr     error
	beginCtx     context.Context
	beginOpts    sql.TxOptions
	beginTimeout uint16
	beginCalled  bool

	resumeTx     extensions.SessionlessTx
	resumeErr    error
	resumeCtx    context.Context
	resumeID     extensions.GlobalTransactionID
	resumeCalled bool
}

func (*sessionlessTransactionTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (*sessionlessTransactionTestConn) Close() error { return nil }

func (*sessionlessTransactionTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (c *sessionlessTransactionTestConn) BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (extensions.SessionlessTx, error) {
	c.beginCalled = true
	c.beginCtx = ctx
	c.beginOpts = opts
	c.beginTimeout = timeout
	return c.beginTx, c.beginErr
}

func (c *sessionlessTransactionTestConn) ResumeSessionlessTx(ctx context.Context, globalTransactionID extensions.GlobalTransactionID) (extensions.SessionlessTx, error) {
	c.resumeCalled = true
	c.resumeCtx = ctx
	c.resumeID = globalTransactionID
	return c.resumeTx, c.resumeErr
}

type sessionlessTransactionTestPlainConn struct{}

func (*sessionlessTransactionTestPlainConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (*sessionlessTransactionTestPlainConn) Close() error { return nil }

func (*sessionlessTransactionTestPlainConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

type sessionlessTransactionTestTx struct{}

func (*sessionlessTransactionTestTx) Commit() error   { return nil }
func (*sessionlessTransactionTestTx) Rollback() error { return nil }
func (*sessionlessTransactionTestTx) Suspend() error  { return nil }
func (*sessionlessTransactionTestTx) GlobalTransactionID() extensions.GlobalTransactionID {
	return extensions.GlobalTransactionID("test-global-transaction-id")
}

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

func openSessionlessTransactionTestConnectionWrapper(t *testing.T, conn driver.Conn) *connectionWrapper {
	t.Helper()
	sqlConn := openSessionlessTransactionTestConnection(t, conn)
	wrapper, err := NewConnectionWrapper(sqlConn)
	if err != nil {
		t.Fatalf("wrap test connection: %v", err)
	}
	return wrapper
}

// TestBeginSessionlessTxDelegates verifies that the public wrapper begin method
// forwards its arguments and returns the driver-provided transaction.
func TestBeginSessionlessTxDelegates(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{}{}, "begin")
	opts := sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true}
	wantTx := &sessionlessTransactionTestTx{}
	driverConn := &sessionlessTransactionTestConn{beginTx: wantTx}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

	gotTx, err := connectionWrapper.BeginSessionlessTx(ctx, opts, 123)
	if err != nil {
		t.Fatalf("BeginSessionlessTx returned error: %v", err)
	}
	if gotTx != wantTx {
		t.Fatalf("BeginSessionlessTx returned %p, want %p", gotTx, wantTx)
	}
	if !driverConn.beginCalled {
		t.Fatal("BeginSessionlessTx did not call the driver connection")
	}
	if driverConn.beginCtx != ctx {
		t.Fatal("BeginSessionlessTx did not forward the context")
	}
	if driverConn.beginOpts != opts {
		t.Fatalf("BeginSessionlessTx options = %+v, want %+v", driverConn.beginOpts, opts)
	}
	if driverConn.beginTimeout != 123 {
		t.Fatalf("BeginSessionlessTx timeout = %d, want 123", driverConn.beginTimeout)
	}
}

// TestBeginSessionlessTxPropagatesError verifies that an error returned by the
// driver-level begin operation is propagated through the public wrapper.
func TestBeginSessionlessTxPropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("begin sessionless transaction failed")
	driverConn := &sessionlessTransactionTestConn{beginErr: wantErr}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

	gotTx, err := connectionWrapper.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
	if gotTx != nil {
		t.Fatalf("BeginSessionlessTx returned transaction %p on error", gotTx)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("BeginSessionlessTx error = %v, want %v", err, wantErr)
	}
}

// TestResumeSessionlessTxDelegates verifies that the public wrapper resume
// method forwards its arguments and returns the driver-provided transaction.
func TestResumeSessionlessTxDelegates(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{}{}, "resume")
	globalTransactionID := extensions.GlobalTransactionID("resume-global-transaction-id")
	wantTx := &sessionlessTransactionTestTx{}
	driverConn := &sessionlessTransactionTestConn{resumeTx: wantTx}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

	gotTx, err := connectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx returned error: %v", err)
	}
	if gotTx != wantTx {
		t.Fatalf("ResumeSessionlessTx returned %p, want %p", gotTx, wantTx)
	}
	if !driverConn.resumeCalled {
		t.Fatal("ResumeSessionlessTx did not call the driver connection")
	}
	if driverConn.resumeCtx != ctx {
		t.Fatal("ResumeSessionlessTx did not forward the context")
	}
	if string(driverConn.resumeID) != string(globalTransactionID) {
		t.Fatalf("ResumeSessionlessTx global transaction ID = %q, want %q", driverConn.resumeID, globalTransactionID)
	}
}

// TestResumeSessionlessTxPropagatesError verifies that an error returned by the
// driver-level resume operation is propagated through the public wrapper.
func TestResumeSessionlessTxPropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("resume sessionless transaction failed")
	driverConn := &sessionlessTransactionTestConn{resumeErr: wantErr}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

	gotTx, err := connectionWrapper.ResumeSessionlessTx(
		context.Background(), extensions.GlobalTransactionID("resume-global-transaction-id"))
	if gotTx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %p on error", gotTx)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSessionlessTx error = %v, want %v", err, wantErr)
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
