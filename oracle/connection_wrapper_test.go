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

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
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
	beginTx      common.SessionlessTransaction
	beginErr     error
	beginCtx     context.Context
	beginOpts    sql.TxOptions
	beginTimeout uint16
	beginCalled  bool

	resumeTx      common.SessionlessTransaction
	resumeErr     error
	resumeCtx     context.Context
	resumeID      GlobalTransactionID
	resumeTimeout uint16
	resumeCalled  bool
	preparedStmt  driver.Stmt
}

// SetRunningFromSessionlessTx is a no-op for the wrapper test connection.
func (*sessionlessTransactionTestConn) SetRunningFromSessionlessTx(bool) {}

// Prepare returns the statement injected into the wrapper test connection.
func (c *sessionlessTransactionTestConn) Prepare(string) (driver.Stmt, error) {
	if c.preparedStmt != nil {
		return c.preparedStmt, nil
	}
	return nil, driver.ErrSkip
}

// QueryContext reports whether the test transaction has already ended.
func (c *sessionlessTransactionTestConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	transaction := c.beginTx
	if transaction == nil {
		transaction = c.resumeTx
	}
	if transaction != nil && transaction.IsTransactionEnded() {
		return nil, sql.ErrTxDone
	}
	return nil, driver.ErrSkip
}

func (*sessionlessTransactionTestConn) Close() error { return nil }

func (*sessionlessTransactionTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

// BeginSessionlessTx records the wrapper request and returns the configured
// test transaction or error.
func (c *sessionlessTransactionTestConn) BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (common.SessionlessTransaction, error) {
	c.beginCalled = true
	c.beginCtx = ctx
	c.beginOpts = opts
	c.beginTimeout = timeout
	return c.beginTx, c.beginErr
}

// ResumeSessionlessTx records the wrapper request and returns the configured
// test transaction or error.
func (c *sessionlessTransactionTestConn) ResumeSessionlessTx(ctx context.Context, globalTransactionID []byte, timeout uint16) (common.SessionlessTransaction, error) {
	c.resumeCalled = true
	c.resumeCtx = ctx
	c.resumeID = globalTransactionID
	c.resumeTimeout = timeout
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

type sessionlessTransactionTestTx struct {
	transactionEnded bool
	runningFromTx    bool
	commitErr        error
	rollbackErr      error
	suspendErr       error
}

// Commit returns the configured fake commit result and updates the simulated
// transaction state when the operation succeeds.
func (tx *sessionlessTransactionTestTx) Commit() error {
	if tx.commitErr == nil {
		tx.transactionEnded = true
	}
	return tx.commitErr
}

// Rollback returns the configured fake rollback result and updates the
// simulated transaction state when the operation succeeds.
func (tx *sessionlessTransactionTestTx) Rollback() error {
	if tx.rollbackErr == nil {
		tx.transactionEnded = true
	}
	return tx.rollbackErr
}

// Suspend returns the configured fake suspend result and updates the
// simulated transaction state when the operation succeeds.
func (tx *sessionlessTransactionTestTx) Suspend() error {
	if tx.suspendErr == nil {
		tx.transactionEnded = true
	}
	return tx.suspendErr
}

// SetRunningFromSessionlessTx records whether the wrapper authorized an
// operation on the fake transaction.
func (tx *sessionlessTransactionTestTx) SetRunningFromSessionlessTx(running bool) {
	tx.runningFromTx = running
}

// IsTransactionEnded reports whether the fake transaction has ended.
func (tx *sessionlessTransactionTestTx) IsTransactionEnded() bool { return tx.transactionEnded }

// GlobalTransactionID returns the fixed identifier used by wrapper tests.
func (*sessionlessTransactionTestTx) GlobalTransactionID() []byte {
	return []byte("test-global-transaction-id")
}

type sessionlessTransactionTestStmt struct {
	closeCount int
}

// Close records that the fake statement was closed.
func (stmt *sessionlessTransactionTestStmt) Close() error {
	stmt.closeCount++
	return nil
}

// NumInput reports that the fake statement accepts a variable number of inputs.
func (*sessionlessTransactionTestStmt) NumInput() int { return -1 }

// Exec succeeds and reports one affected row.
func (*sessionlessTransactionTestStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

// Query returns the unsupported-query error used by wrapper tests.
func (*sessionlessTransactionTestStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("query not implemented by sessionless test statement")
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
	if gotTx == nil {
		t.Fatal("BeginSessionlessTx returned a nil public transaction")
	}
	if gotTx.transaction != wantTx {
		t.Fatalf("BeginSessionlessTx returned underlying transaction %p, want %p", gotTx.transaction, wantTx)
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
	globalTransactionID := GlobalTransactionID("resume-global-transaction-id")
	wantTx := &sessionlessTransactionTestTx{}
	driverConn := &sessionlessTransactionTestConn{resumeTx: wantTx}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

	gotTx, err := connectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 123)
	if err != nil {
		t.Fatalf("ResumeSessionlessTx returned error: %v", err)
	}
	if gotTx == nil {
		t.Fatal("ResumeSessionlessTx returned a nil public transaction")
	}
	if gotTx.transaction != wantTx {
		t.Fatalf("ResumeSessionlessTx returned underlying transaction %p, want %p", gotTx.transaction, wantTx)
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
	if driverConn.resumeTimeout != 123 {
		t.Fatalf("ResumeSessionlessTx timeout = %d, want 123", driverConn.resumeTimeout)
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
		context.Background(), GlobalTransactionID("resume-global-transaction-id"), 300)
	if gotTx != nil {
		t.Fatalf("ResumeSessionlessTx returned transaction %p on error", gotTx)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSessionlessTx error = %v, want %v", err, wantErr)
	}
}

// TestSessionlessTransactionEndsAndClosesPreparedStatements
// verifies that transaction-owned prepared statements are closed by every
// terminal operation and that all error-returning transaction operations fail
// afterwards. The commit, rollback, and suspend subtests cover the three
// terminal paths independently.
func TestSessionlessTransactionEndsAndClosesPreparedStatements(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		end  func(*sessionlessTx) error
	}{
		// Commit closes statements before the public handle becomes unusable.
		{name: "commit", end: (*sessionlessTx).Commit},
		// Rollback closes statements before the public handle becomes unusable.
		{name: "rollback", end: (*sessionlessTx).Rollback},
		// Suspend closes statements because the suspended handle cannot resume.
		{name: "suspend", end: (*sessionlessTx).Suspend},
	} {
		t.Run(test.name, func(t *testing.T) {
			preparedStmt := &sessionlessTransactionTestStmt{}
			driverConn := &sessionlessTransactionTestConn{
				beginTx:      &sessionlessTransactionTestTx{},
				preparedStmt: preparedStmt,
			}
			connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)

			tx, err := connectionWrapper.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("BeginSessionlessTx returned error: %v", err)
			}
			if _, err := tx.Prepare("SELECT 1 FROM DUAL"); err != nil {
				t.Fatalf("Prepare returned error: %v", err)
			}

			if err := test.end(tx); err != nil {
				t.Fatalf("%s returned error: %v", test.name, err)
			}
			if preparedStmt.closeCount != 1 {
				t.Fatalf("prepared statement close count after %s = %d, want 1", test.name, preparedStmt.closeCount)
			}

			if _, err := tx.Exec("SELECT 1"); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("Exec after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if _, err := tx.ExecContext(context.Background(), "SELECT 1"); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("ExecContext after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if _, err := tx.Prepare("SELECT 1"); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("Prepare after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if _, err := tx.PrepareContext(context.Background(), "SELECT 1"); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("PrepareContext after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if _, err := tx.Query("SELECT 1"); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("Query after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if _, err := tx.QueryContext(context.Background(), "SELECT 1"); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("QueryContext after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if err := tx.QueryRow("SELECT 1").Scan(new(int)); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("QueryRow after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if err := tx.QueryRowContext(context.Background(), "SELECT 1").Scan(new(int)); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("QueryRowContext after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if tx.GlobalTransactionID() != nil {
				t.Errorf("GlobalTransactionID after %s returned a value, want nil", test.name)
			}
			if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("second Commit after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("Rollback after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
			if err := tx.Suspend(); !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("Suspend after %s error = %v, want %v", test.name, err, sql.ErrTxDone)
			}
		})
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

// TestSessionlessTransactionDirectSQLMethods verifies the non-context Exec and
// Query wrapper methods, the context-aware QueryRow wrapper method, and confirms
// that wrapper authorization is cleared when each operation returns.
func TestSessionlessTransactionDirectSQLMethods(t *testing.T) {
	t.Parallel()

	transaction := &sessionlessTransactionTestTx{}
	driverConn := &sessionlessTransactionTestConn{
		beginTx:      transaction,
		preparedStmt: &sessionlessTransactionTestStmt{},
	}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)
	tx, err := connectionWrapper.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx returned error: %v", err)
	}

	if _, err := tx.Exec("SELECT 1"); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if transaction.runningFromTx {
		t.Fatal("Exec left sessionless authorization enabled")
	}

	if _, err := tx.Query("SELECT 1"); err == nil {
		t.Fatal("Query unexpectedly succeeded with the unsupported fake statement")
	}
	if transaction.runningFromTx {
		t.Fatal("Query left sessionless authorization enabled")
	}

	if err := tx.QueryRowContext(context.Background(), "SELECT 1").Scan(new(int)); err == nil {
		t.Fatal("QueryRowContext unexpectedly succeeded with the unsupported fake statement")
	}
	if transaction.runningFromTx {
		t.Fatal("QueryRowContext left sessionless authorization enabled")
	}
}

// TestSessionlessTransactionGlobalTransactionID verifies that the wrapper
// returns the active driver's transaction ID and returns nil when the wrapped
// connection can no longer execute Raw callbacks.
func TestSessionlessTransactionGlobalTransactionID(t *testing.T) {
	t.Parallel()

	driverConn := &sessionlessTransactionTestConn{beginTx: &sessionlessTransactionTestTx{}}
	sqlConn := openSessionlessTransactionTestConnection(t, driverConn)
	connectionWrapper, err := NewConnectionWrapper(sqlConn)
	if err != nil {
		t.Fatalf("NewConnectionWrapper returned error: %v", err)
	}
	tx, err := connectionWrapper.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx returned error: %v", err)
	}

	if got, want := string(tx.GlobalTransactionID()), "test-global-transaction-id"; got != want {
		t.Fatalf("GlobalTransactionID() = %q, want %q", got, want)
	}

	if err := sqlConn.Close(); err != nil {
		t.Fatalf("sql.Conn.Close returned error: %v", err)
	}
	if got := tx.GlobalTransactionID(); got != nil {
		t.Fatalf("GlobalTransactionID() after connection close = %q, want nil", got)
	}
}

// TestSessionlessTransactionCloseStatementsIsIdempotent verifies that repeated
// cleanup closes each transaction-owned statement only once.
func TestSessionlessTransactionCloseStatementsIsIdempotent(t *testing.T) {
	t.Parallel()

	transaction := &sessionlessTransactionTestTx{}
	driverStmt := &sessionlessTransactionTestStmt{}
	driverConn := &sessionlessTransactionTestConn{beginTx: transaction, preparedStmt: driverStmt}
	connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)
	tx, err := connectionWrapper.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
	if err != nil {
		t.Fatalf("BeginSessionlessTx returned error: %v", err)
	}
	if _, err := tx.Prepare("SELECT 1"); err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}

	tx.closeStatements()
	tx.closeStatements()

	if driverStmt.closeCount != 1 {
		t.Fatalf("statement close count = %d, want 1", driverStmt.closeCount)
	}
}

// TestSessionlessTransactionClosesStatementsAfterEndingErrors verifies that
// Commit, Rollback, and Suspend close owned statements even when the driver
// operation returns an error.
func TestSessionlessTransactionClosesStatementsAfterEndingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*sessionlessTransactionTestTx, error)
		end   func(*sessionlessTx) error
	}{
		{
			name:  "commit",
			setup: func(tx *sessionlessTransactionTestTx, err error) { tx.commitErr = err },
			end:   (*sessionlessTx).Commit,
		},
		{
			name:  "rollback",
			setup: func(tx *sessionlessTransactionTestTx, err error) { tx.rollbackErr = err },
			end:   (*sessionlessTx).Rollback,
		},
		{
			name:  "suspend",
			setup: func(tx *sessionlessTransactionTestTx, err error) { tx.suspendErr = err },
			end:   (*sessionlessTx).Suspend,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wantErr := errors.New(test.name + " failed")
			transaction := &sessionlessTransactionTestTx{}
			test.setup(transaction, wantErr)
			stmt := &sessionlessTransactionTestStmt{}
			driverConn := &sessionlessTransactionTestConn{
				beginTx:      transaction,
				preparedStmt: stmt,
			}
			connectionWrapper := openSessionlessTransactionTestConnectionWrapper(t, driverConn)
			tx, err := connectionWrapper.BeginSessionlessTx(context.Background(), sql.TxOptions{}, 300)
			if err != nil {
				t.Fatalf("BeginSessionlessTx returned error: %v", err)
			}
			if _, err := tx.Prepare("SELECT 1"); err != nil {
				t.Fatalf("Prepare returned error: %v", err)
			}

			if err := test.end(tx); !errors.Is(err, wantErr) {
				t.Fatalf("%s error = %v, want %v", test.name, err, wantErr)
			}
			if stmt.closeCount != 1 {
				t.Fatalf("statement close count after %s = %d, want 1", test.name, stmt.closeCount)
			}
		})
	}
}
