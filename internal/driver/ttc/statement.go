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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// statementCloseTimeout bounds waiting for the physical TTC stream and
	// flushing a cursor-close request from driver.Stmt.Close.
	statementCloseTimeout = 10 * time.Second
)

// releaseLobBindsAfterExecution frees execution-scoped LOB locators only when
// executionErr proves that the TTC stream can safely accept another RPC.
//
// Parameters:
//   - cleanup: locators created while preparing streamed LOB binds.
//   - executionErr: result of the statement's query or execution exchange.
//
// Returns:
//   - error: executionErr, unless cleanup fails after a successful execution.
func releaseLobBindsAfterExecution(cleanup *preparedLobBinds, executionErr error) error {
	if cleanup == nil {
		return executionErr
	}
	if executionErr != nil && !isCompletedLobResponseError(executionErr) && !isTerminalOracleError(executionErr) {
		cleanup.abandon()
		return executionErr
	}
	if cleanupErr := cleanup.free(); executionErr == nil {
		return cleanupErr
	}
	return executionErr
}

// isTerminalOracleError reports whether err is an Oracle server error received
// in a terminal TTIOER response. Statement executors return TTIOER errors
// directly, so no error-tree traversal is required here.
//
// Parameters:
//   - err: result of a statement query or execution exchange.
//
// Returns:
//   - bool: true when err is a direct SQLError with an ORA error code.
func isTerminalOracleError(err error) bool {
	sqlErr, ok := err.(oracleErrors.SQLError)
	return ok && strings.HasPrefix(sqlErr.ErrorCode(), "ORA-")
}

// preflightStatementBindShape validates named and ordinal SQL bind references
// before streamed LOB preparation can consume an application reader. The
// statement executors repeat this conversion defensively after preparation;
// this preflight exists specifically to keep malformed LOB-bearing calls free
// of reader consumption and temporary-LOB RPCs.
//
// Parameters:
//   - query: parsed SQL statement whose bind metadata is validated.
//   - args: normalized statement arguments, including private streamed inputs.
//
// Returns:
//   - error: bind-shape error, or an internal error when statement metadata is
//     incomplete.
func preflightStatementBindShape(query *qualifiedSQLStatement, args []driver.NamedValue) error {
	if query == nil || query.binds == nil {
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}
	if query.kind == plsql {
		_, err := extractInputBindValuesForPlSql(query.binds, args)
		return err
	}
	_, err := extractInputBindValues(query.binds, args)
	return err
}

// Statement is the physical-driver representation of one parsed SQL statement
// and its Oracle cursor. It may be owned by database/sql as a prepared
// statement or temporarily created by Connection.QueryContext/ExecContext.
//
// A successful direct query transfers Statement ownership to ttcRows. A
// prepared query attaches Rows non-owningly so Rows.Close detaches the result
// without destroying the reusable cursor. mu protects only close/result
// ownership state; TTC traffic is serialized separately by the shelf operation
// guard.
type Statement struct {
	// mu protects closed and _rows. It must not be held while entering Rows or
	// performing TTC I/O to avoid Statement/Rows close lock cycles.
	mu sync.Mutex
	// closed makes Close idempotent and prevents duplicate cursor cleanup.
	closed bool
	// shelf is the physical connection's TTC dependency and lifetime registry.
	shelf *ttiShelf[driverCommon.MessageType]
	// sessionContext supplies negotiated character sets for streamed CLOB/NCLOB
	// bind creation and conversion.
	sessionContext *driverCommon.SessionContext
	// qualifiedQuery contains parsed binds, SQL kind, text, and server cursor ID.
	qualifiedQuery *qualifiedSQLStatement
	// queryStatementExecutor owns SELECT protocol execution.
	queryStatementExecutor QueryWithContext
	// execStatementExecutor owns non-query protocol execution.
	execStatementExecutor ExecWithContext
	// _rows is the currently attached result. It is owning only when the matching
	// ttcRows has ownsStatement set.
	_rows *ttcRows // reference on created Rows.
}

/*
newStatement constructs a Statement for a given SQL text.

It performs the one-time parsing/classification work needed by subsequent executions:
  - classifies the SQL text into a sqlKind (SELECT, DML, PL/SQL, etc.)
  - parses and records bind placeholders (positional and/or named)
  - selects the appropriate query/exec executors for the sqlKind and injects the
    TTC shelf and negotiated session context when supported

The returned Statement is safe to hand to database/sql as a driver.Stmt;
cancellation support is provided by the shelf's operation-cancellation
coordinator on per-execution sub-contexts.

Parameters:
  - shelf: the per-connection TTC shelf used by downstream executor implementations.
  - sessionCtx: the session context used to get the session character set
  - query: the SQL text for this statement.

Returns:
  - (*Statement, nil) on success, or (nil, error) if SQL classification or placeholder parsing fails.
*/
func newStatement(
	shelf *ttiShelf[driverCommon.MessageType],
	sessionCtx *driverCommon.SessionContext,
	query string,
) (*Statement, error) {
	classifiedQ, err := newQualifiedSQLStatement(query)
	if err != nil {
		return nil, err
	}

	queryExecutor := getQueryStatementExecutorFor(classifiedQ)
	execExecutor := getExecStatementExecutorFor(classifiedQ)
	if su, ok := queryExecutor.(ttiShelfUser); ok {
		su.SetShelf(shelf)
	}
	if scu, ok := queryExecutor.(SessionContextUser); ok {
		scu.SetSessionContext(sessionCtx)
	}
	if su, ok := execExecutor.(ttiShelfUser); ok {
		su.SetShelf(shelf)
	}
	if scu, ok := execExecutor.(SessionContextUser); ok {
		scu.SetSessionContext(sessionCtx)
	}
	stmt := &Statement{
		shelf:                  shelf,
		sessionContext:         sessionCtx,
		qualifiedQuery:         classifiedQ,
		queryStatementExecutor: queryExecutor,
		execStatementExecutor:  execExecutor,
		_rows:                  nil,
	}
	// register myself to the shelf so connection close can garbage me.
	stmt.shelf.AddStatement(stmt)
	return stmt, nil
}

/*
QueryContext executes the statement as a query and returns a streaming Rows.

It is the primary entry point used by database/sql when the driver implements
driver.StmtQueryContext.

Implementation details:
  - Validates args against the parsed bind placeholders (count and names).
  - Normalizes public LOB bind markers and prepares streamed LOB inputs as
    temporary locator binds before the statement executor marshals SQL binds.
  - Creates a child context with a cancellation after-function that can attempt
    a server-side break/reset when ctx is canceled or times out.
  - Delegates execution to queryStatementExecutor, which performs the TTC
    pipeline and returns a driver.Rows implementation.
  - Releases execution-scoped temporary LOB locators only after the exchange has
    a known terminal boundary; ambiguous failures discard the session instead.

Callers must fully consume and Close the returned Rows.
*/
func (s *Statement) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.queryContext(ctx, args)
}

// queryContext normalizes and prepares binds, executes one query exchange, and
// transfers the resulting Rows ownership to the statement/Rows lifecycle.
func (s *Statement) queryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	normalizedArgs, firstLobInput, err := normalizeAndValidateStreamedLobInputs(args)
	if err != nil {
		return nil, s.shelf.LocalizeError(err)
	}
	if firstLobInput >= 0 {
		// Validate SQL bind names and ordinals before preparation can consume any
		// streamed LOB source or create a temporary locator.
		if err := preflightStatementBindShape(s.qualifiedQuery, normalizedArgs); err != nil {
			return nil, s.shelf.LocalizeError(err)
		}
	}
	unlock, err := s.shelf.synchronizer.begin(ctx)
	if err != nil {
		return nil, s.shelf.LocalizeError(err)
	}
	defer unlock()

	subContext, cancelSubContext, cleanup := s.createSubContextWithCancelAfterFunc(ctx)
	defer cleanup()

	if transaction := s.shelf.getTransaction(); transaction != nil {
		// if in a transaction, add an after function on the transaction context
		// that will cancel the statements context if the transaction context is
		// cancelled triggering the break/reset protocol
		stopTransAfterFunction := context.AfterFunc(transaction.getTransactionContext(), func() {
			cancelSubContext()
		})
		defer stopTransAfterFunction()
	}
	preparedArgs, lobCleanup, e := prepareStreamedLobBinds(subContext, s.shelf, s.sessionContext, normalizedArgs, firstLobInput)
	if e != nil {
		return nil, s.shelf.LocalizeError(e)
	}
	selectedRows, executionErr := s.queryStatementExecutor.QueryContext(subContext, s.qualifiedQuery, preparedArgs)
	executionErr = releaseLobBindsAfterExecution(lobCleanup, executionErr)
	if executionErr != nil && selectedRows != nil {
		_ = selectedRows.Close()
		selectedRows = nil
	}
	if executionErr == nil {
		rows, ok := selectedRows.(*ttcRows)
		if !ok || rows == nil {
			if selectedRows != nil {
				_ = selectedRows.Close()
			}
			executionErr = common.NewOracleError(
				oracleErrors.InternalError,
				fmt.Errorf("query executor returned Rows type %T", selectedRows),
			)
		} else {
			// Locator-backed values borrow Rows after this exchange-only subContext
			// is cleaned up, so derive their lifetime from the original query ctx.
			rows.setContext(ctx)
			rows.attachStatement(s)
			if !s.attachRows(rows) {
				_ = rows.closeFromStatement(s)
				executionErr = common.NewOracleError(
					oracleErrors.StatementExecutionFailed,
					errors.New("statement closed while query was completing"),
					"query",
				)
			}
		}
	}

	if err := s.shelf.checkCurrentState(ctx); err != nil {
		return nil, err
	}

	if executionErr != nil {
		return nil, s.shelf.LocalizeError(executionErr)
	}
	return selectedRows, nil
}

// _closeCursor closes the statement cursorID if not 0
func (s *Statement) _closeCursor() error {
	ctx, cancel := context.WithTimeout(context.Background(), statementCloseTimeout)
	defer cancel()

	unlock, err := s.shelf.synchronizer.begin(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.closeCursorLocked(ctx)
}

// closeCursorLocked closes the cursor while the caller owns the shelf
// operation guard.
func (s *Statement) closeCursorLocked(ctx context.Context) error {
	if s.qualifiedQuery.cursorId == 0 {
		return nil
	}
	// Build OCCA from factory.
	factory := s.shelf.GetMessageFactory()
	msg, err := factory.GetMessageForFunction(TTIPFN, occa)
	if err != nil {
		common.Odl.Error("Statement.Close: GetMessageForFunction(TTIPFN,occa) failed", "error", err)
		return s.shelf.LocalizeError(err)
	}

	common.Odl.Debug("Closing cursorId", "ID", s.qualifiedQuery.cursorId)
	occaMsg := msg.(*tTIOcca)
	occaMsg.setCursorIDs([]driverCommon.UB4{driverCommon.UB4(s.qualifiedQuery.cursorId)})

	// OCCA is a one-way cursor cleanup request, but it must be flushed before the
	// connection can be returned to the pool. Otherwise a later operation could
	// accidentally carry this statement's cleanup in its own exchange.
	stmr := s.shelf.GetMessageStreamer().(MessageStreamerInterface)
	if err := stmr.Push(ctx, occaMsg); err != nil {
		common.Odl.Error("Statement.Close: Push(OCCA) failed", "error", err)
		return s.shelf.LocalizeError(err)
	}
	if err := stmr.Flush(ctx); err != nil {
		common.Odl.Error("Statement.Close: Flush(OCCA) failed", "error", err)
		return s.shelf.LocalizeError(err)
	}
	s.qualifiedQuery.cursorId = 0
	return nil
}

// Close implements driver.Stmt.Close.
func (s *Statement) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	rows := s._rows
	s._rows = nil
	s.mu.Unlock()

	var finalErr error

	// Mark associated Rows closed without allowing it to recursively close this
	// Statement. The cursor is closed below exactly once.
	if rows != nil {
		finalErr = rows.closeFromStatement(s)
	}
	// Do not queue cursor traffic after temporary-LOB cleanup made the session
	// unusable. Connection teardown will release the cursor in that case.
	if finalErr == nil {
		err := s._closeCursor()
		if err != nil {
			common.Odl.Debug("Failed to close statement", "error", err)
			// The cursor-close exchange did not complete. Do not allow database/sql to
			// reuse a session whose outgoing TTC state may be ambiguous.
			s.shelf.getEventService().post(streamerStaleEvent)
			finalErr = s.shelf.LocalizeError(common.NewOracleError(oracleErrors.StatementCloseFailed, err))
		}
	}

	s.shelf.RemoveStatement(s)

	return finalErr
}

// attachRows records the result produced by the latest execution. It returns
// false when Close already won the lifecycle race, allowing the caller to
// invalidate the result instead of attaching Rows to a closed Statement.
func (s *Statement) attachRows(rows *ttcRows) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s._rows = rows
	return true
}

// detachRows clears only the matching result, allowing a prepared Statement to
// remain open and reusable after its Rows close.
func (s *Statement) detachRows(rows *ttcRows) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s._rows == rows {
		s._rows = nil
	}
}

// closeAfterRows finalizes an internally-created direct-query Statement after
// its owning Rows has already detached itself.
func (s *Statement) closeAfterRows() error {
	return s.Close()
}

// NumInput implements driver.Stmt.NumInput. It returns -1 so database/sql does
// not reject a QueryContext call before the driver's parsed-placeholder
// validation handles SQL-specific names and ordinals before any TTC request.
func (s *Statement) NumInput() int {
	return -1
}

// CheckNamedValue allows sql.Out binds to pass through database/sql conversion.
// For all other values, we delegate back to database/sql default conversion.
func (s *Statement) CheckNamedValue(nv *driver.NamedValue) error {
	return s.shelf.LocalizeError(checkNamedValue(nv))
}

/*
ExecContext executes the statement as a non-query (DML/DDL/PLSQL) and returns a Result.

It is the primary entry point used by database/sql when the driver implements
driver.StmtExecContext.

Implementation details:
  - Validates args against the parsed bind placeholders (count and names).
  - Normalizes public LOB bind markers and prepares streamed LOB inputs as
    temporary locator binds before the statement executor marshals SQL binds.
  - Creates a child context with a cancellation after-function that can attempt
    a server-side break/reset when ctx is canceled or times out.
  - Delegates execution to the execStatementExecutor, which performs the TTC
    operations and returns a driver.Result.
  - Releases execution-scoped temporary LOB locators after the exchange when its
    terminal boundary is known; ambiguous failures discard the session instead.

The returned Result may expose rows-affected metadata when the server provides it.
*/
func (s *Statement) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.execContext(ctx, args)
}

// execContext normalizes and prepares binds before delegating one execution to
// the statement executor while the physical-session guard is held.
func (s *Statement) execContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	normalizedArgs, firstLobInput, err := normalizeAndValidateStreamedLobInputs(args)
	if err != nil {
		return nil, s.shelf.LocalizeError(err)
	}
	if firstLobInput >= 0 {
		// Keep bind-shape failures ahead of reader consumption and temporary-LOB
		// creation; the executor still validates its final prepared arguments.
		if err := preflightStatementBindShape(s.qualifiedQuery, normalizedArgs); err != nil {
			return nil, s.shelf.LocalizeError(err)
		}
	}
	unlock, err := s.shelf.synchronizer.begin(ctx)
	if err != nil {
		return nil, s.shelf.LocalizeError(err)
	}
	defer unlock()

	subContext, cancelSubContext, cleanup := s.createSubContextWithCancelAfterFunc(ctx)
	defer cleanup()

	if transaction := s.shelf.getTransaction(); transaction != nil {
		// if in a transaction, add an after function on the transaction context
		// that will cancel the statements context if the transaction context is
		// cancelled triggering the break/reset protocol
		stopTransAfterFunction := context.AfterFunc(transaction.getTransactionContext(), func() {
			cancelSubContext()
		})
		defer stopTransAfterFunction()
	}
	preparedArgs, lobCleanup, err := prepareStreamedLobBinds(subContext, s.shelf, s.sessionContext, normalizedArgs, firstLobInput)
	if err != nil {
		return nil, s.shelf.LocalizeError(err)
	}
	result, err := s.execStatementExecutor.ExecContext(subContext, s.qualifiedQuery, preparedArgs)
	err = releaseLobBindsAfterExecution(lobCleanup, err)
	if err := s.shelf.checkCurrentState(ctx); err != nil {
		return nil, err
	}
	return result, s.shelf.LocalizeError(err)
}

/*
Exec implements driver.Stmt.Exec.

database/sql calls this legacy method when context-aware execution is not used.
The implementation adapts positional []driver.Value to []driver.NamedValue
(1-based Ordinal) and delegates to ExecContext with context.Background().
*/
func (s *Statement) Exec(args []driver.Value) (driver.Result, error) {
	nvs := make([]driver.NamedValue, len(args))
	for i, v := range args {
		nvs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.ExecContext(context.Background(), nvs)
}

/*
Query implements driver.Stmt.Query.

database/sql calls this legacy method when context-aware execution is not used.
The implementation adapts positional []driver.Value to []driver.NamedValue
(1-based Ordinal) and delegates to QueryContext with context.Background().
*/
func (s *Statement) Query(args []driver.Value) (driver.Rows, error) {
	nvs := make([]driver.NamedValue, len(args))
	for i, v := range args {
		nvs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.QueryContext(context.Background(), nvs)
}

// createSubContextWithCancelAfterFunc creates a sub-context with an AfterFunc
// that can perform break/reset when the operation is canceled. An
// operationCancellationState value is attached so the execution loop can
// authorize recovery when it observes cancellation. Cleanup releases a fired
// AfterFunc when execution finishes before cancellation handling.
//
// Parameters:
//   - ctx the parent context
//
// Returns:
//   - the new child context
//   - function that cancels the sub-context
//   - cleanup function that stops or releases the cancellation after-function
func (s *Statement) createSubContextWithCancelAfterFunc(ctx context.Context) (context.Context, context.CancelFunc, func()) {
	return s.shelf.cancellation.newCancelableOperationContext(ctx, s.shelf.cancelExecution)
}
