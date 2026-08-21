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
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvierCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
)

// execContextFunc adapts a function literal into ExecWithContext for focused
// Statement.ExecContext tests.
type execContextFunc func(context.Context, *qualifiedSQLStatement, []driver.NamedValue) (driver.Result, error)

// ExecContext calls the wrapped function literal.
func (f execContextFunc) ExecContext(ctx context.Context, query *qualifiedSQLStatement, args []driver.NamedValue) (driver.Result, error) {
	return f(ctx, query, args)
}

// queryContextFunc adapts a function literal into QueryWithContext for focused
// Statement.QueryContext tests.
type queryContextFunc func(context.Context, *qualifiedSQLStatement, []driver.NamedValue) (driver.Rows, error)

// QueryContext calls the wrapped function literal.
func (f queryContextFunc) QueryContext(ctx context.Context, query *qualifiedSQLStatement, args []driver.NamedValue) (driver.Rows, error) {
	return f(ctx, query, args)
}

// Tests cancelCurrentExecution
// Checks that ns.CancelOperation has been called and that possible errors have
// been treated correctly
func TestConnection_cancelCurrentExecution(t *testing.T) {
	t.Parallel()
	mockFac := &mockFactory{
		returnMsg: NewOall18(),
	}
	mockStr := &mockStreamer{}
	shelf := newShelf[drvierCommon.MessageType]()
	shelf.RegisterMessageStreamer(mockStr)
	shelf.RegisterMessageFactory(mockFac)

	tests := []struct {
		name        string
		cancelErr   error
		wantErr     bool
		wantInvalid bool
	}{
		{
			name:        "successful cancel",
			cancelErr:   nil,
			wantErr:     false,
			wantInvalid: false,
		},
		{
			name:        "cancel with error",
			cancelErr:   errors.New("mock cancel error"),
			wantErr:     true,
			wantInvalid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &mockNetworkSession{
				disconnectCalls: 0,
				disconnectErr:   nil,
				sleepDuration:   0,
				cancelErr:       tt.cancelErr,
			}

			conn := newTestConnection(shelf, nil, ns)

			// Clean error message so that connection close succeeds
			mockStr.pullMsg = &mockOer{}
			ctx := context.Background()
			err := conn.cancelCurrentExecution(ctx)

			if ns.cancelCalls != 1 {
				t.Errorf("Wrong number of calls to CancelOperations expected 1 but was %d", ns.cancelCalls)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("cancelCurrentExecution() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				if sqlError, ok := err.(oracleErrors.SQLError); ok {
					if sqlError.ErrorCode() != string(oracleErrors.CancelOperationError) {
						t.Errorf("Wrong error code, expected %s but was %s", string(oracleErrors.CancelOperationError), sqlError.ErrorCode())
					}
					cause := errors.Unwrap(sqlError)
					if cause == nil {
						t.Errorf("Expected error to have a cause")
					}
					if !errors.Is(cause, tt.cancelErr) {
						t.Errorf("Expected cause to be cancelErr")
					}
				} else {
					t.Errorf("Expected error to be SQLError")
				}
			}
			if conn._isValid != !tt.wantInvalid {
				t.Errorf("_isValid = %v, want %v", conn._isValid, !tt.wantInvalid)
			}
		})
	}
}

// TestStatementCancellationCleanupReleasesStartedAfterFunc verifies cleanup
// releases a fired cancellation callback when execution exits early.
func TestStatementCancellationCleanupReleasesStartedAfterFunc(t *testing.T) {
	t.Parallel()
	shelf := newShelf[drvierCommon.MessageType]()
	cancelCalled := make(chan struct{}, 1)
	shelf.registerCancelExecution(func(context.Context) error {
		cancelCalled <- struct{}{}
		return nil
	})

	stmt := &Statement{shelf: shelf}
	parentCtx, cancelParent := context.WithCancel(context.Background())
	subCtx, _, cleanup := stmt.createSubContextWithCancelAfterfunction(parentCtx)
	cancellationState := subCtx.Value(statementCancellationContextKey{}).(*statementCancellationState)

	cancelParent()
	select {
	case <-cancellationState.started:
	case <-time.After(time.Second):
		t.Fatal("statement cancellation after-function did not start")
	}

	cleanup()
	select {
	case <-cancellationState.done:
	default:
		t.Fatal("statement cancellation after-function was not released by cleanup")
	}
	select {
	case <-cancelCalled:
		t.Fatal("cleanup should release early cancellation without running break-reset")
	default:
	}
}

// TestStatementHandleContextCancelledRunsBreakReset verifies the pull loop can
// authorize break/reset and then read the cancellation TTIOER.
func TestStatementHandleContextCancelledRunsBreakReset(t *testing.T) {
	t.Parallel()
	mockStr := &mockStreamer{pullMsg: &mockOer{}}
	shelf := newShelf[drvierCommon.MessageType]()
	shelf.RegisterMessageStreamer(mockStr)
	cancelCalled := make(chan struct{}, 1)
	shelf.registerCancelExecution(func(context.Context) error {
		cancelCalled <- struct{}{}
		return nil
	})

	stmt := &Statement{shelf: shelf}
	parentCtx, cancelParent := context.WithCancel(context.Background())
	subCtx, _, cleanup := stmt.createSubContextWithCancelAfterfunction(parentCtx)
	defer cleanup()

	cancelParent()
	msg, err := (&statementProcessor{shelf: shelf}).handleContextCancelled(subCtx)
	if err != nil {
		t.Fatalf("handleContextCancelled returned error: %v", err)
	}
	if msg == nil || msg.GetMsgCode() != TTIOER {
		t.Fatalf("handleContextCancelled returned %v, want TTIOER", msg)
	}
	select {
	case <-cancelCalled:
	case <-time.After(time.Second):
		t.Fatal("break-reset cancellation was not called")
	}
	if !mockStr.pullCalled {
		t.Fatal("expected TTIOER pull after break-reset")
	}
}

// TestStatementExecContextTransactionCancellationBeforeSetup verifies a
// transaction cancellation cannot race an uninitialized statement cancel func.
func TestStatementExecContextTransactionCancellationBeforeSetup(t *testing.T) {
	t.Parallel()
	shelf := newShelf[drvierCommon.MessageType]()
	shelf.RegisterMessageStreamer(&mockStreamer{})
	txCtx, cancelTx := context.WithCancel(context.Background())
	cancelTx()
	shelf.registerTransaction(newTransaction(&connection{shelf: shelf}, txCtx))
	defer shelf.unregisterTransaction()

	query, err := newQualifiedSQLStatement("insert into t values(1)")
	if err != nil {
		t.Fatalf("newQualifiedSQLStatement failed: %v", err)
	}
	stmt := &Statement{
		shelf:          shelf,
		qualifiedQuery: query,
		execStatementExecutor: execContextFunc(func(ctx context.Context, _ *qualifiedSQLStatement, _ []driver.NamedValue) (driver.Result, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
				return nil, errors.New("timed out waiting for transaction cancellation")
			}
		}),
	}

	_, err = stmt.ExecContext(context.Background(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecContext error = %v, want context.Canceled", err)
	}
}

// TestStatementQueryContextLocalization verifies query-level errors are
// localized before the error leaves the statement boundary.
func TestStatementQueryContextLocalization(t *testing.T) {
	t.Parallel()

	mockStr := &mockStreamer{}

	shelf := newShelf[drvierCommon.MessageType]()
	shelf.RegisterLocalizationService(common.NewLocalizationService(language.French))
	shelf.RegisterMessageStreamer(mockStr)

	stmt := &Statement{
		shelf: shelf,
		queryStatementExecutor: queryContextFunc(func(context.Context, *qualifiedSQLStatement, []driver.NamedValue) (driver.Rows, error) {
			return nil, common.NewOracleError(oracleErrors.InternalError, nil)
		}),
	}

	_, err := stmt.QueryContext(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if sqle, ok := err.(oracleErrors.SQLError); !ok {
		t.Fatalf("expected SQLError, got %T", err)
	} else if sqle.ErrorCode() != string(oracleErrors.InternalError) {
		t.Fatalf("expected error code %s, got %s", oracleErrors.InternalError, sqle.ErrorCode())
	}
	if got, want := err.Error(), "OGD-00062 - erreur interne factice."; got != want {
		t.Fatalf("unexpected localized error %q, want %q", got, want)
	}
}

func seedPreparedSelectResponse(t *testing.T, ctx context.Context, dbuf *ArrayBasedDataBuffer) {
	t.Helper()

	stream := Oall8Payload(validSelectPreparedDump)
	if len(stream) == 0 {
		t.Fatal("validSelectPreparedDump decode returned empty")
	}
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIDCB)); err != nil {
		t.Fatalf("write TTIDCB header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, stream); err != nil {
		t.Fatalf("write validSelectPreparedDump stream failed: %v", err)
	}
}

func runSeededStatementQuery(t *testing.T, query string, args []driver.NamedValue) {
	t.Helper()

	ctx := context.Background()
	shelf, _, dbuf := newExecTestShelf(8192)
	registerTestCodecs(shelf, 20)
	seedPreparedSelectResponse(t, ctx, dbuf)

	stmt, err := newStatement(shelf, &drvierCommon.SessionContext{}, query)
	if err != nil {
		t.Fatalf("newStatement failed: %v", err)
	}

	rows, err := stmt.QueryContext(ctx, args)
	if err != nil {
		t.Fatalf("QueryContext failed: %v", err)
	}
	if rows == nil {
		t.Fatal("expected rows, got nil")
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("rows.Close failed: %v", err)
	}
}

func TestStatement_QueryContext_JSONConstructor_NamedBindAfterQuotedKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "bind after quoted key with space",
			query: `select json { '_id' : any_value("DUMMY"),  :bind value count(1) } from dual group by 1`,
		},
		{
			name:  "bind after quoted key without extra space",
			query: `select json { '_id':any_value("DUMMY"), :bind value count(1) } from dual group by 1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSeededStatementQuery(t, tt.query, []driver.NamedValue{
				{Name: "bind", Value: "payload"},
			})
		})
	}
}

func TestStatement_QueryContext_JSONConstructor_AdditionalNamedBindShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		args  []driver.NamedValue
	}{
		{
			name:  "named bind as json value",
			query: `SELECT JSON {'key' VALUE :bind} FROM dual`,
			args: []driver.NamedValue{
				{Name: "bind", Value: "abc"},
			},
		},
		{
			name:  "named bind as json key",
			query: `SELECT JSON {:auto VALUE 'value'} FROM dual`,
			args: []driver.NamedValue{
				{Name: "auto", Value: "auto"},
			},
		},
		{
			name:  "named bind as json key before n tick literal",
			query: `SELECT JSON {:company VALUE N'Oracle'} FROM dual`,
			args: []driver.NamedValue{
				{Name: "company", Value: "name"},
			},
		},
		{
			name:  "named bind as json key before q tick literal",
			query: `SELECT JSON {:equilibrium VALUE Q'<Oracle''s>'} FROM dual`,
			args: []driver.NamedValue{
				{Name: "equilibrium", Value: "Bind Value"},
			},
		},
		{
			name:  "named bind as json key before u tick literal",
			query: "SELECT JSON {:auto VALUE U'👍'} FROM dual",
			args: []driver.NamedValue{
				{Name: "auto", Value: "Thumbs Up"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSeededStatementQuery(t, tt.query, tt.args)
		})
	}
}
