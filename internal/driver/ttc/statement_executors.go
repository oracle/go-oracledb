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
	sqldriver "database/sql/driver"
	"errors"
	"reflect"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Internal AL8I4 flags used in oall8Options[9]
const (
	_al8exGetPidmlrc        driverCommon.UB4 = 0x4000
	_al8exImplResultsClient driverCommon.UB4 = 0x8000
)

/*
QueryWithContext executes a read-only SQL query and returns a streaming
database/sql/driver.Rows.

Implementations:
  - Must honor ctx cancellation and deadlines; network I/O and server work should
    be aborted promptly when ctx is done.
  - Interpret args as positional bind values aligned with placeholders in query.
    Values should already be convertible to database/sql/driver.Value by the caller.
  - Typical TTC-based implementations build and push an OALL8/OEXFEN request, push TTIRXD
    rows for bind payloads when present, flush the stream, and pull TTIDCB/TTIBVC/
    TTIRXD messages until completion.

Contract:
  - The returned Rows streams result data; callers must iterate to completion and
    call Close to release resources.
  - The passed query host the request cursor Id that would be updated after round-trips

Parameters:
- ctx: request-scoped context for cancellation and timeouts.
- query: SQL qualified query
- args: named values ([]driver.NamedValue); may be empty.

Returns:
- driver.Rows on success (the caller must Close it).
- error on failure (e.g., factory/push/flush/pull issues or server-side errors).
*/
type QueryWithContext interface {
	// QueryContext defines the ability to execute a query with context and positional args.
	// returns:
	//   - the selected rows
	//   - the cursorId ID, of this request
	//   - error if request has failed
	QueryContext(ctx context.Context, query *qualifiedSQLStatement, args []driver.NamedValue) (driver.Rows, error)
}

/*
ExecWithContext executes a non-query SQL statement and returns a
database/sql/driver.Result.

Implementations:
  - Must honor ctx cancellation and deadlines; underlying operations should stop
    promptly when ctx is done.
  - Interpret args as positional bind values aligned with placeholders in query.
  - Typical TTC-based implementations build and push an OALL8 request, push TTIRXD
    rows if binds exist, flush the stream, and pull TTIOER to detect completion.
    For DML, implementations may expose rows-affected metadata when available.

Contract:
- Intended for DML/DDL/PL/SQL statements that do not produce a result set.
- Should return a Result that may include RowsAffected when provided by the server.
- The passed query host the request cursor Id that would be updated after round-trips

Parameters:
- ctx: request-scoped context for cancellation and timeouts.
- query: SQL qualified query.
- args: named values ([]driver.NamedValue); may be empty.

Returns:
- driver.Result on success.
- error on failure (e.g., factory/push/flush/pull issues or server-side errors).
*/
type ExecWithContext interface {
	// ExecContext defines the ability to execute a statement with context and positional args.
	ExecContext(ctx context.Context, query *qualifiedSQLStatement, args []driver.NamedValue) (driver.Result, error)
}

// statementProcessor provides common state shared by all statement executors
// (SELECT, DML, OTHER, PL/SQL), including shelf reference, OALL8 options and flags.
type statementProcessor struct {
	shelf         *ttiShelf[driverCommon.MessageType] // Message shelf used to get factory/streamer/marshaller/codec-factory for statement execution.
	sessCtx       *driverCommon.SessionContext        // The session context used to get the session character set
	opts          driverCommon.UB4                    // OALL8 option bitmask (parse/execute/commit/no-PLSQL/binds-present flags etc).
	al8i4         []driverCommon.UB4                  // AL8I4 options vector attached to OALL8 (iterations, select flag, extra protocol flags).
	bindValues    []any                               // Original bind values (ordered by position) for the current execution.
	encodedValues [][]driverCommon.B1Array            // Wire-encoded bind payloads per iteration (each inner slice is a TTIRXD bind row).
	currentOacs   []driverCommon.Marshallable         // Per-bind OAC descriptors (type/size metadata) sent alongside bind values.
	previousOacs  []driverCommon.Marshallable         // Cached OACs from previous execution of the statement
}

// SetShelf injects the message shelf used by the SELECT executor.
func (e *statementProcessor) SetShelf(s *ttiShelf[driverCommon.MessageType]) { e.shelf = s }

// SetSessionContext injects the session context used by the executor.
func (e *statementProcessor) SetSessionContext(sessCtx *driverCommon.SessionContext) {
	e.sessCtx = sessCtx
}

// statementExecutorSelect implements queries and statements for SELECTs.
type statementExecutorSelect struct {
	statementProcessor
	resultMetadata selectResultMetadata
}

// selectResultMetadata owns the column descriptors for a SELECT statement.
// Oracle may omit DCB metadata on OEXFEN re-execution, so these descriptors are
// retained across executions of the statement.
type selectResultMetadata struct {
	columns []columnContext
}

// columnCount returns the number of columns described by the cached metadata.
// Deriving the count from the slice keeps both values consistent.
func (m selectResultMetadata) columnCount() driverCommon.UB4 {
	return driverCommon.UB4(len(m.columns))
}

// replace updates the cached result metadata from a newly received DCB
// description. It copies the top-level slice so later changes to the source
// slice cannot alter the cache.
func (m *selectResultMetadata) replace(columns []columnContext) {
	m.columns = append(m.columns[:0], columns...)
}

// newRows creates an empty result container using the cached result metadata.
// It returns nil until the first DCB description has been received.
func (m selectResultMetadata) newRows(shelf *ttiShelf[driverCommon.MessageType]) *ttcRows {
	if len(m.columns) == 0 {
		return nil
	}
	rows := newTTCRows(m.columns)
	rows.SetShelf(shelf)
	return rows
}

// queryRunState contains state whose lifetime is one runQuery invocation. BVC
// carry data, LOB metadata, and rows must never be reused by a later protocol
// round trip.
type queryRunState struct {
	rowCount   driverCommon.UB4
	bvcColSent *driverCommon.BitSet
	bvcFound   bool
	// prevRow and prevLobColContext form the aligned previous-row state used by
	// BVC carry.
	prevRow           []driverCommon.B1Array
	prevLobColContext []*lobColumnContext
	rows              *ttcRows
}

// newQueryRunState creates clean row and BVC state for one runQuery invocation.
// When result metadata is already cached, it also creates a fresh rows object
// so an OEXFEN response can be decoded without receiving another DCB message.
func (e *statementExecutorSelect) newQueryRunState() *queryRunState {
	return &queryRunState{rows: e.resultMetadata.newRows(e.shelf)}
}

// newStatementExecutorSelect constructs a SELECT executor with server-parse and execute flags,
// initializes AL8I4 for SELECT, and disables PL/SQL mode.
func newStatementExecutorSelect() *statementExecutorSelect {
	return &statementExecutorSelect{
		statementProcessor: statementProcessor{},
	}
}

// statementExecutorExec executes the statement.
type statementExecutorExec struct {
	statementProcessor
	_currentRank        driverCommon.UB4 // currentRank represents current row in processing, incremented at each execution
	numberOfInOutParams int              // Number of IN OUT bind positions detected for the current execution.
	outDestPtrs         []any            // Destination pointers that receive decoded OUT or RETURNING values.
	outColumnContexts   []columnContext  // Decoder contexts derived from bind OAC metadata for returned values.
}

/*
initExecRunner resets and rebuilds the DML execution state related to OUT, IN OUT,
and RETURNING binds for the current execution.

Description:
  - Clears counters and cached slices that store per-execution RETURNING/OUT bind state.
  - Scans the provided bind arguments for sql.Out values and records their destination
    pointers, IN OUT status, and codec-derived column metadata.
  - Enables the protocol options required for DML RETURNING when at least one OUT bind
    is present so the server returns the IOV vector and treats the statement as capable
    of returning bind values.

Parameters:
  - args: ordered bind values for the current execution.

Notes:
  - Metadata extraction from getBindOac is best-effort; if it fails, the execution
    still proceeds without populating the corresponding output column context.
*/
func (e *statementExecutorExec) initExecRunner(args []sqldriver.Value) {
	// Reset per-execution counters and cached destinations before inspecting new binds.
	// the pointer type to an outDestPtrs may change during re-execution. Reset and
	// rebuild the contexts and pointers
	e.numberOfInOutParams = 0
	e.outDestPtrs = e.outDestPtrs[:0]
	e.outColumnContexts = e.outColumnContexts[:0]

	codecFactory := e.shelf.GetCodecFactory()
	for outIndex, arg := range args {
		// Only sql.Out binds participate in DML RETURNING / OUT parameter handling.
		if outArg, ok := arg.(sql.Out); ok {
			e.outDestPtrs = append(e.outDestPtrs, outArg.Dest)
			if outArg.In {
				e.numberOfInOutParams++
			}

			// Capture decoder-relevant metadata from the bind OAC when it is available.
			bindOac, err := codecFactory.getBindOac(normalizeBindValue(outArg), 0)
			if err == nil {
				if oac, ok := bindOac.(*tTIoac); ok {
					e.outColumnContexts = append(e.outColumnContexts, columnContext{
						Index:       outIndex,
						DataType:    int16(oac.dataType),
						Precision:   int64(oac.precision),
						Scale:       int8(oac.scale),
						CharsetForm: uint8(oac.characterSetForm),
						CharsetID:   uint16(oac.characterSetID),
					})
				}
			}
			continue
		}
	}

	// DML with OUT/RETURNING binds must request the IOV vector and disable noPLSQLMode.
	if len(e.outDestPtrs) > 0 {
		e.opts &= ^noPLSQLMode
		e.opts = e.opts | returnIOVVector
	}
}

// statementExecutorDML implements queries and statements for DMLs.
type statementExecutorDML struct {
	statementExecutorExec
}

// newStatementExecutorDML constructs a DML executor with server-parse, execute and
// commit-after-execution flags, and initializes AL8I4 for DML.
func newStatementExecutorDML() *statementExecutorDML {
	return &statementExecutorDML{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{
				opts:  driverCommon.UB4(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode),
				al8i4: buildAl8i4(1, false, _al8exImplResultsClient|_al8exGetPidmlrc, 1),
			},
			_currentRank: 1,
		},
	}
}

// statementExecutorOthers implements queries and statements for other statement types (DDL/other).
type statementExecutorOthers struct {
	statementExecutorExec
}

// newStatementExecutorOthers constructs an executor for DDL/OTHER statements with
// commit-after-execution and initializes AL8I4 for non-SELECT/PLSQL flows.
func newStatementExecutorOthers() *statementExecutorOthers {
	return &statementExecutorOthers{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{
				opts:  driverCommon.UB4(statementParsedByServer | executeStatement | commitAfterExecution | noPLSQLMode),
				al8i4: buildAl8i4(1, false, _al8exImplResultsClient, 1),
			},
			_currentRank: 1,
		},
	}
}

// statementExecutorPlSql implements execution for PL/SQL calls (BEGIN/DECLARE/CALL).
// Unlike DML/Others, it does NOT set the noPLSQLMode flag in opts.
type statementExecutorPlSql struct {
	statementExecutorExec
}

func newStatementExecutorPlSql() *statementExecutorPlSql {
	return &statementExecutorPlSql{
		statementExecutorExec: statementExecutorExec{
			statementProcessor: statementProcessor{
				opts:  driverCommon.UB4(statementParsedByServer | executeStatement | commitAfterExecution),
				al8i4: buildAl8i4(1, false, _al8exImplResultsClient, 1),
			},
			_currentRank: 1,
		},
	}
}

// isNoDataFoundError reports whether err is Oracle's end-of-fetch signal.
// NoDataFound is handled as a completed query round trip rather than a fatal
// execution error because the returned state may contain valid rows.
func isNoDataFoundError(err error) bool {
	if err == nil {
		return false
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	return ok && sqlErr.ErrorCode() == string(oracleErrors.NoDataFound)
}

// QueryContext builds and submits an OALL8 request for SELECT statements and
// runs the TTC pipeline to fetch rows.
func (e *statementExecutorSelect) QueryContext(ctx context.Context, query *qualifiedSQLStatement, namedValues []driver.NamedValue) (sqldriver.Rows, error) {
	var err error
	// validate the arguments against the parsed bind placeholders.
	args, err := extractInputBindValues(query.binds, namedValues)
	if err != nil {
		return nil, err
	}
	if len(args) > 0 {
		if err := e.prepareBindsAndOAC(args); err != nil {
			return nil, err
		}
	}
	e.opts = e.buildOAll8Options(false)
	e.al8i4 = buildAl8i4(0, true, _al8exImplResultsClient, e.opts&statementParsedByServer)
	// A cursorId is provided by the server the first time we execute
	// the next times, the cursor ID is stored in the qualifiedQuery
	// and do nto have to be updated
	var needToUpdateCursorId bool = true
	var messageToBeExecuted driverCommon.Message[driverCommon.MessageType]
	if e.needToSendOACs() {
		messageToBeExecuted, err = e.createOAll8Msg(query, args)
		if err != nil {
			return nil, err
		}
	} else {
		messageToBeExecuted, err = e.createOexfenMsg(query)
		if err != nil {
			return nil, err
		}
		needToUpdateCursorId = false
	}
	state, cursorId, err := e.runQuery(ctx, messageToBeExecuted)
	// if the server returned any error other than common.NoDataFound
	// return it back to the user.
	if err != nil && !isNoDataFoundError(err) {
		return nil, err
	}
	if needToUpdateCursorId {
		query.cursorId = cursorId
	}
	// If rows were returned, or the server reported NoDataFound, this execution
	// is complete and no define/fetch round trip is required.
	if err != nil || (state.rows != nil && state.rows.numOfRows > 0) {
		return state.rows, nil
	}
	// define needs to be sent in other cases
	e.opts = e.buildOAll8Options(true)
	e.al8i4 = buildAl8i4(_maxfetchSize, true, 0, e.opts&statementParsedByServer)
	messageToBeExecuted, err = e.createOAll8Msg(query, args)
	if err != nil {
		return nil, err
	}
	e.prepareDefines(messageToBeExecuted)
	state, _, err = e.runQuery(ctx, messageToBeExecuted)
	// if the server returned any error other than common.NoDataFound
	// return it back to the user.
	if err != nil && !isNoDataFoundError(err) {
		return nil, err
	}
	return state.rows, nil
}

/*
buildOAll8Options returns the OALL8 option bitmask for the current SELECT phase.

Description:
  - During the initial parse/execute round-trip, the executor requests server-side
    parse and statement execution without sending defines.
  - During the follow-up define/fetch round-trip, the executor switches to fetch mode
    and indicates that column define metadata is included in the OALL8 payload.
  - Both paths force noPLSQLMode because this executor only handles SELECT statements.

Parameters:
  - isDefine: true when building the follow-up fetch request that carries define OACs;
    false for the initial parse/execute request.

Returns:
  - common.UB4: OALL8 options appropriate for either parse/execute or define/fetch.
*/
func (e *statementExecutorSelect) buildOAll8Options(isDefine bool) driverCommon.UB4 {
	if isDefine {
		return fetchRows | defineColumnsProvided | noPLSQLMode
	}
	return statementParsedByServer | executeStatement | noPLSQLMode
}

/*
prepareDefines populates define OACs on an OALL8 message before a SELECT fetch round-trip.

Description:
  - Builds one define descriptor per selected column using the codec factory and the
    column contexts discovered from the initial query response.
  - Reads the optional LOB prefetch size from connection properties and passes it to
    define builders so LOB columns can advertise the desired prefetch behavior.
  - Updates the outgoing OALL8 message with the number of define columns, the define
    OAC slice, and resets rows-to-fetch so the next execution performs the actual fetch.

Parameters:
  - messageToBeExecuted: the OALL8 message that will be enriched with define metadata.

Panics / assumptions:
  - Assumes messageToBeExecuted is a *tTIOall, as this helper is only used for the
    second-phase SELECT OALL8 request.
  - Assumes resultMetadata was initialized by a prior DCB metadata round-trip.
*/
func (e *statementExecutorSelect) prepareDefines(messageToBeExecuted driverCommon.Message[driverCommon.MessageType]) {
	defines := make([]driverCommon.Marshallable, len(e.resultMetadata.columns))
	connectionProperties := e.shelf.GetConnectionProperties()
	for i, colContext := range e.resultMetadata.columns {
		defines[i] = e.shelf.GetCodecFactory().getDefineOac(colContext.DataType, colContext, connectionProperties)
	}
	messageToBeExecuted.(*tTIOall).setDefCols(driverCommon.SB4(len(defines)))
	messageToBeExecuted.(*tTIOall).setDefineOACs(defines)
	messageToBeExecuted.(*tTIOall).resetRowsToFetch()
}

// ExecContext executes a DML statement (INSERT/UPDATE/DELETE). It builds an OALL8 request,
func (e *statementExecutorDML) ExecContext(ctx context.Context, query *qualifiedSQLStatement, namedValues []driver.NamedValue) (sqldriver.Result, error) {
	// validate the arguments against the parsed bind placeholders.
	args, err := extractInputBindValues(query.binds, namedValues)
	if err != nil {
		return nil, err
	}
	e.initExecRunner(args)
	messageToBeExecuted, err := e.prepareForExec(query, args)
	if err != nil {
		return nil, err
	}

	messageToBeExecuted.(*tTIOall).setCurrentRank(e._currentRank)
	e._currentRank++

	e.registerDMLCallbacks(ctx)
	defer e.unregisterDMLCallbacks()
	result, id, err := e.runExec(ctx, messageToBeExecuted)
	if err == nil {
		query.cursorId = id
	}
	return result, err
}

// unregisterDMLCallbacks removes the DML-specific TTC callbacks registered for statement execution.
func (e *statementExecutorDML) unregisterDMLCallbacks() {
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.UnRegisterPostUnmarshallCallback(TTIRPA)
	stmr.UnRegisterPreUnmarshallCallback(TTIRXD)
	stmr.UnRegisterPreUnmarshallCallback(TTIIOV)
	stmr.UnRegisterPostUnmarshallCallback(TTIIOV)
}

// initForTransaction when in a transaction it removes the auto-commit flag of
// the statement execution message
func (e *statementProcessor) initForTransaction() {
	if e.shelf.isInTransaction() {
		e.opts &= ^commitAfterExecution
	}
}

/*
createOAll8Msg builds an OALL8 (TTIFUN oAll8) request to execute/parse a statement.

Description:
  - Initializes an OALL8 message with execution options (opts), cursor id, prefetch settings,
    SQL text, and the AL8I4 options vector.
  - If bind arguments are supplied, it:
  - Normalizes sql/driver.NamedValue into a 0-based []any slice aligned to bind positions.
  - Encodes bind values into wire format [][]common.B1Array (currently 1 iteration).
  - Builds per-bind OAC descriptors and sets bind-related fields and flags on the OALL8.

Parameters:
- query: SQL text to be executed.
- args: Optional sql/driver.NamedValue bind arguments (by position or name).

Returns:
- *tTIOall: Initialized OALL8 request ready to be pushed to the MessageStreamer.
- error: Non-nil on factory retrieval, normalization, or encoding/OAC build failures.
*/
func (e *statementProcessor) createOAll8Msg(qualifiedStmt *qualifiedSQLStatement, args []sqldriver.Value) (driverCommon.Message[driverCommon.MessageType], error) {
	common.Odl.Debug("statementProcessor.createOAll8Msg: called", "stmt", qualifiedStmt)
	factory := e.shelf.GetMessageFactory()
	reqMsg, err := factory.GetMessageForFunction(TTIFUN, oAll8)
	// map factory/type issues -> OGD-00050 SelectErrorCode
	if err != nil {
		common.Odl.Error("createOAll8Msg: GetMessageForFunction failed",
			"error", err, "stage", "GetMessageForFunction", "func", "oAll8")
		return nil, common.NewOracleError(oracleErrors.StatementExecutionFailed, err, select_.String(), "QueryContext failed")
	}
	m, _ := reqMsg.(*tTIOall)
	m.setOptions(e.opts)
	m.setMaxLength(maxLength)
	m.setRowsToFetch()
	m.setDefCols(0)
	if e.opts&statementParsedByServer != 0 {
		m.setSQL(qualifiedStmt.query)
	}
	m.setOall8Options(e.al8i4)
	m.setCursorId(qualifiedStmt.cursorId)
	if len(args) > 0 {
		m.setNumberOfBindPositions(driverCommon.SB4(len(e.bindValues)))
		if e.needToSendOACs() {
			m.setBindOACs(e.currentOacs)
			// Ensure bindValuesPresent flag is set when we have binds.
			m.setOptions(e.opts | bindValuesPresent)
		}
		e.previousOacs = e.currentOacs
	}

	return reqMsg, nil
}

/*
createOexfenMsg builds an OEXFEN (TTIFUN oExfen) fast-path execute+fetch request.

Description:
  - Initializes an OEXFEN message using the message factory.
  - Sets the current cursor id, carries over executor options (used for commit-on-success),
    and sets the fetch size to the protocol-defined maximum to stream all rows.
  - Intended for subsequent executions when bind definitions are unchanged, avoiding
    re-sending the full OALL8 parse/exec payload.

Parameters:
  - query : the qualifiedQuery used in the statement

Returns:
- *tTIOexfen: Initialized OEXFEN request ready to be pushed to the MessageStreamer.
- error: Non-nil on factory retrieval failures.
*/
func (e *statementExecutorSelect) createOexfenMsg(query *qualifiedSQLStatement) (driverCommon.Message[driverCommon.MessageType], error) {
	common.Odl.Debug("statementExecutorSelect.createOexfenMsg: called")
	factory := e.shelf.GetMessageFactory()
	reqMsg, err := factory.GetMessageForFunction(TTIFUN, oExfen)

	if err != nil {
		common.Odl.Error("createOAll8Msg: GetMessageForFunction failed",
			"error", err, "stage", "GetMessageForFunction", "func", "oexfen")
		return nil, common.NewOracleError(oracleErrors.StatementExecutionFailed, err, select_.String(), "QueryContext failed")
	}
	m, _ := reqMsg.(*tTIOexfen)
	m.setOptions(e.opts)
	m.setFetchSize()
	m.setCursorId(query.cursorId)
	return reqMsg, nil

}

/*
buildAl8i4 creates the AL8I4 options vector used by the OALL8 request.

Description:
  - Populates the 13-element AL8I4 array with server-parse indicator, iteration count,
    SCN placeholders, SELECT flag, and additional bit flags.

Parameters:
- iterations: Number of iterations (rows of binds) to execute.
- selectStmt: Whether the statement is a SELECT (affects AL8I4[7]).
- flags: Bitmask of AL8I4 extra flags (stored in AL8I4[9]).

Returns:
- []common.UB4: Fully initialized AL8I4 vector.
*/
func buildAl8i4(iterations driverCommon.UB4, selectStmt bool, flags driverCommon.UB4, parseOption driverCommon.UB4) []driverCommon.UB4 {
	common.Odl.Debug("buildAl8i4: called",
		"iterations", iterations, "select", selectStmt, "flags", flags)
	al := make([]driverCommon.UB4, 13)
	al[0] = parseOption // server needs to parse
	al[1] = iterations
	al[5] = 0 // SCN lo
	al[6] = 0 // SCN hi
	if selectStmt {
		al[7] = 1
	}
	al[9] = flags
	al[11] = 0
	return al
}

/*
prepareBindsAndOAC prepares bind payloads and OAC descriptors for an OALL8 execution.

Description:
  - Encodes each bind value using the TypeCodecFactory encoder for reflect.TypeOf(value)
    and stores the result in e.encodedValues as a single "iteration" (one TTIRXD row).
  - Creates an OAC for each bind using the TypeCodecFactory OAC builder for reflect.TypeOf(value),
    and sizes the OAC using the encoded byte length for that bind.

Parameters:
- args: The ordered sqldriver.Value for each bind position.

Notes / limitations:
  - Currently supports only a single bind iteration (batching is TODO).
  - This function does not perform sql.NamedValue name/ordinal normalization; callers must
    provide args in the final bind position order.

Returns:
  - error: non-nil if encoder/OAC factory lookup fails or if encoding fails for any bind value.
*/
func (e *statementProcessor) prepareBindsAndOAC(args []sqldriver.Value) error {
	n := len(args)

	e.bindValues = make([]any, n)
	// TODO : currently, just do it for single row, later when
	//        batching support is added, make it dynamic.
	e.encodedValues = make([][]driverCommon.B1Array, 1)
	currentRow := 0
	e.encodedValues[currentRow] = make([]driverCommon.B1Array, n)
	e.currentOacs = make([]driverCommon.Marshallable, n)
	for i, v := range args {
		e.bindValues[i] = v
		// sql.Out{}, int, float, string,time
		var err error
		normalized := normalizeBindValue(v)
		encoder, err := e.shelf.GetCodecFactory().getEncoder(normalized)
		if err != nil {
			return err
		}

		e.encodedValues[currentRow][i], err = encoder(normalized.value)
		if err != nil {
			return err
		}

		e.currentOacs[i], err = e.shelf.GetCodecFactory().getBindOac(
			normalized,
			e.getMaxLengthForOac(i, len(e.encodedValues[currentRow][i])),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

/*
pushBindRows pushes TTIRXD rows for bind data when binds are present.

Description:
  - If the OALL8 request indicates bind values are present, iterates over encoded rows and
    pushes a TTIRXD message per row with the row's bind payloads set.
  - Intended to be invoked between pushing OALL8 and flushing the streamer.

Parameters:
- ctx: Context used while pushing rows to the MessageStreamer.
- req: The OALL8 request whose options and bind count determine whether rows are pushed.
- caller: String used for logging context.
- errCode: Error code to wrap underlying push/get failures.

Returns:
- error: Non-nil on factory errors, push failures, or inconsistent bind state; nil otherwise.
*/
func (e *statementProcessor) pushBindRows(
	ctx context.Context,
	caller string,
	errCode oracleErrors.ErrorCode,
) error {
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	// encode and push RXD rows before flushing
	for _, row := range e.encodedValues {
		msg, gerr := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIRXD)
		if gerr != nil {
			common.Odl.Error(caller+": GetMessage(TTIRXD) failed", "error", gerr, "stage", "get-rxd")
			return common.NewOracleError(errCode, gerr, "push")
		}
		rxd := msg.(*tTIrxd)
		rxd.setBindValues(row)
		if err := stmr.Push(ctx, rxd); err != nil {
			common.Odl.Error(caller+": Push RXD failed", "error", err, "stage", "push", "msgCode", rxd.GetMsgCode())
			return common.NewOracleError(errCode, err, "push")
		}
	}
	return nil
}

/*
needToSendOACs reports whether OACs (Oracle Attribute Context) need to be sent.

OACs should be sent if:
  - different number of OACs than previous, or
  - the data type differs from previous execution, or
  - the maximum length increased (current required length exceeds previously declared length).

Returns:
  - bool: true if the OACs should be sent
*/
func (e *statementProcessor) needToSendOACs() bool {
	if len(e.currentOacs) != len(e.previousOacs) {
		return true
	}
	var ret = true
	for position := 0; position < len(e.currentOacs); position++ {
		if e.previousOacs[position].(*tTIoac).dataType != e.currentOacs[position].(*tTIoac).dataType ||
			e.previousOacs[position].(*tTIoac).maxLength < e.currentOacs[position].(*tTIoac).maxLength {
			return true
		} else {
			ret = false
		}
	}
	return ret
}

/*
getMaxLengthForOac computes the maxLength to use for the current bind OAC at the given position.
It preserves the previous OAC's maxLength when it is larger than the currently encoded value length,
ensuring the declared size is monotonic non-decreasing across executions (to avoid unnecessary
redefinitions/reallocations on the server).

Parameters:
  - position:   zero-based bind position whose previous OAC (if any) is considered.
  - currLength: byte length of the currently encoded value for that bind.

Returns:
  - common.UB4: the maximum of the previous OAC maxLength (if present) and the current value length.
*/
func (e *statementProcessor) getMaxLengthForOac(position int, currLength int) driverCommon.UB4 {
	if e.previousOacs == nil || len(e.previousOacs) <= position {
		return driverCommon.UB4(currLength)
	}
	return max(e.previousOacs[position].(*tTIoac).maxLength, driverCommon.UB4(currLength))
}

// runQuery executes one TTC push/flush/pull round trip. It owns a fresh
// queryRunState, registers callbacks that capture that state, and returns the
// completed state so QueryContext can inspect or return its rows. Fatal errors
// return a nil state; NoDataFound retains state because it marks end-of-fetch.
func (e *statementExecutorSelect) runQuery(ctx context.Context, message driverCommon.Message[driverCommon.MessageType]) (*queryRunState, driverCommon.SB4, error) {
	common.Odl.Debug("runQuery: start", "msgCode", message.GetMsgCode())
	var err error
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)

	if err = stmr.Push(ctx, message); err != nil {
		// map push failure -> OGD-00053 RunQueryError("push")
		common.Odl.Error("runQuery: Push failed", "error", err, "stage", "push", "msgCode", message.GetMsgCode())
		return nil, -1, common.NewOracleError(oracleErrors.RunQueryError, err, "push")
	}

	// If binds exist and we are not sending defines, encode and push RXD rows before flushing
	if len(e.encodedValues) > 0 && (e.opts&defineColumnsProvided == 0) {
		if err = e.pushBindRows(ctx, "runQuery", oracleErrors.RunQueryError); err != nil {
			return nil, -1, err
		}
	}
	// flush Streamer
	common.Odl.Debug("runQuery: flushing streamer")
	if err = stmr.Flush(ctx); err != nil {
		// map flush failure -> OGD-00053 RunQueryError("flush")
		common.Odl.Error("runQuery: Flush failed", "error", err, "stage", "flush", "msgCode", message.GetMsgCode())
		return nil, -1, common.NewOracleError(oracleErrors.RunQueryError, err, "flush")
	}
	common.Odl.Debug("runQuery: streamer flushed")

	// Response state is only needed after the request has been submitted.
	state := e.newQueryRunState()
	e.registerRunQueryCallbacks(state)
	defer unregisterRunQueryCallbacks(stmr)

	oerFound := false
	var returnedCursorID driverCommon.SB4
	var msg driverCommon.Message[driverCommon.MessageType]
	for {
		msg, err = stmr.Pull(ctx,
			TTIOER, TTIDCB, TTIBVC, TTIRXD, TTIRPA,
		)
		// map pull failure -> OGD-00053 RunQueryError("pull")
		if err != nil {
			common.Odl.Error("runQuery: Pull failed", "error", err, "stage", "pull")
			if errors.Is(err, ctx.Err()) {
				// The context has been cancelled, cancel current execution and return
				// error
				msg, err = e.handleContextCancelled(ctx)
			}
			if err != nil {
				// Return error
				return nil, -1, common.NewOracleError(oracleErrors.RunQueryError, err, "pull")
			}
		}
		common.Odl.Debug("runQuery: Pulled message", "msgCode", msg.GetMsgCode())
		switch msg.GetMsgCode() {
		case TTIDCB:
			common.Odl.Debug("runQuery: TTIDCB (column metadata) received")
			if err := e.handleDCB(state, msg); err != nil {
				return nil, -1, err
			}
		case TTIBVC:
			common.Odl.Debug("runQuery: TTIBVC (column presence vector) received")
			state.handleBVC(msg)
		case TTIRXD:
			common.Odl.Debug("runQuery: TTIRXD (row data) received", "rowNum", state.rowCount)
			state.handleRXDRow(msg)
		case TTIRPA:
			returnedCursorID = msg.(*ttioallrpa).getCursorId()
			common.Odl.Debug("runQuery: TTIRPA received", "cursorID", returnedCursorID)
		case TTIOER:
			common.Odl.Debug("runQuery: TTIOER (error) received")
			err = msg.(tTIOerIface).getError()
			if err == nil && state.rows == nil {
				common.Odl.Debug("runQuery: successful TTIOER received before TTIDCB metadata")
				return nil, -1, common.NewOracleError(oracleErrors.ProtocolViolation, nil)
			}
			oerFound = true
		default:
			common.Odl.Warn("Unexpected msg received", "message", msg)
			return nil, -1, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		if oerFound {
			common.Odl.Debug("runQuery: OER found, breaking")
			break
		}
	}
	common.Odl.Debug("runQuery: End of function")
	if state.rows != nil {
		state.rows.numOfRows = len(state.rows.rowData)
	}
	return state, returnedCursorID, err
}

func (e *statementExecutorExec) prepareForExec(query *qualifiedSQLStatement, args []sqldriver.Value) (driverCommon.Message[driverCommon.MessageType], error) {
	if len(args) > 0 {
		if err := e.prepareBindsAndOAC(args); err != nil {
			return nil, err
		}
	}
	e.initForTransaction()
	return e.createOAll8Msg(query, args)
}

// ExecContext executes DDLs/Others.
func (e *statementExecutorExec) ExecContext(ctx context.Context, query *qualifiedSQLStatement, namedValues []driver.NamedValue) (sqldriver.Result, error) {
	// validate the arguments against the parsed bind placeholders.
	args, err := extractInputBindValues(query.binds, namedValues)
	if err != nil {
		return nil, err
	}
	messageToExecute, err := e.prepareForExec(query, args)
	if err != nil {
		return nil, err
	}
	result, cursorId, err := e.runExec(ctx, messageToExecute)
	if err == nil {
		query.cursorId = cursorId
	}
	return result, err
}

// ExecContext executes PL/SQL (BEGIN/DECLARE/CALL). Does not set noPLSQLMode.
func (e *statementExecutorPlSql) ExecContext(ctx context.Context, query *qualifiedSQLStatement, namedValues []driver.NamedValue) (sqldriver.Result, error) {
	// validate the arguments against the parsed bind placeholders.
	args, err := extractInputBindValuesForPlSql(query.binds, namedValues)
	if err != nil {
		return nil, err
	}
	e.initExecRunner(args)
	messageToExecute, err := e.prepareForExec(query, args)
	if err != nil {
		return nil, err
	}
	e.registerPlSqlCallbacks(ctx)
	defer e.unregisterPlSqlCallbacks()
	result, cursorId, err := e.runExec(ctx, messageToExecute)
	if err == nil {
		query.cursorId = cursorId
	}
	return result, err
}

// unregisterPlSqlCallbacks removes the TTC callbacks registered for PL/SQL OUT and IN OUT bind handling.
func (e *statementExecutorPlSql) unregisterPlSqlCallbacks() {
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.UnRegisterPreUnmarshallCallback(TTIRXD)
	stmr.UnRegisterPreUnmarshallCallback(TTIIOV)
	stmr.UnRegisterPostUnmarshallCallback(TTIIOV)
}

/*
registerPlSqlCallbacks registers the TTC callbacks required for PL/SQL execution
that may return OUT or IN OUT bind values.

Description:
  - Registers a TTIRXD pre-unmarshal callback that allocates and configures RXD
    messages for decoding returned PL/SQL bind values.
  - Reuses the shared IOV callback registration so TTIIOV messages are prepared
    and consumed correctly when the server returns bind metadata for OUT-style
    parameters.
  - Limits the registration scope to the active PL/SQL execution and relies on
    unregisterPlSqlCallbacks to remove the callbacks afterward.

Parameters:
  - ctx: execution context associated with the current PL/SQL execution. It is
    forwarded to the shared IOV callback registration flow.

Notes:
  - This helper is used by PL/SQL exec paths only; query-style row handling is
    not configured here.
*/
func (e *statementExecutorPlSql) registerPlSqlCallbacks(ctx context.Context) {
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.RegisterPreUnmarshallCallback(TTIRXD, e.createRXD)
	e.registerIOVCallbacks(ctx)
}

func (e *statementExecutorPlSql) createRXD(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
	msg, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIRXD)
	// map factory failure -> OGD-00073 CallbackFactoryError("get-rxd")
	if err != nil {
		common.Odl.Error("createRXD: GetMessage(TTIRXD) failed", "error", err, "stage", "get-rxd")
		return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "createRXD failed")
	}
	rxd := msg.(*tTIrxd)
	rxd.setNumberofReturningArgs(len(e.outDestPtrs))
	rxd.setColumnContexts(e.outColumnContexts)
	return rxd, nil
}

// runExec executes the TTC push/flush/pull for executing a statement (no rows).
// parameters:
//   - the context to execute
//
// returns:
//   - the result of the execution
//   - the cursorID
//   - the error if any
func (e *statementExecutorExec) runExec(ctx context.Context, message driverCommon.Message[driverCommon.MessageType]) (sqldriver.Result, driverCommon.SB4, error) {
	var rowsAffected int64
	common.Odl.Debug("runExec: start", "msgCode", message.GetMsgCode())
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)

	if err := stmr.Push(ctx, message); err != nil {
		common.Odl.Error("runExec: Push failed", "error", err, "stage", "push", "msgCode", message.GetMsgCode())
		return nil, -1, common.NewOracleError(oracleErrors.RunExecError, err, "push")
	}
	// If binds exist, encode and push RXD rows before flushing
	if len(e.encodedValues) > 0 {
		if err := e.pushBindRows(ctx, "runExec", oracleErrors.RunExecError); err != nil {
			return nil, -1, err
		}
	}

	if err := stmr.Flush(ctx); err != nil {
		common.Odl.Error("runExec: Flush failed", "error", err, "stage", "flush", "msgCode", message.GetMsgCode())
		return nil, -1, common.NewOracleError(oracleErrors.RunExecError, err, "flush")
	}
	registerRunExecCallbacks(stmr, e.shelf)
	defer unregisterRunExecCallbacks(stmr)

	oerFound := false
	var receivedCursorID driverCommon.SB4
	for {
		msg, err := stmr.Pull(ctx, TTIOER, TTIRPA, TTIIOV, TTIRXD, TTIFOB)
		// map pull failure -> OGD-00060 RunExecError("pull")
		if err != nil {
			if errors.Is(err, ctx.Err()) {
				msg, err = e.handleContextCancelled(ctx)
			}
			if err != nil {
				// Return error
				return nil, -1, common.NewOracleError(oracleErrors.RunExecError, err, "pull")
			}
		}
		common.Odl.Debug("runExec: Pulled message", "msgCode", msg.GetMsgCode())

		switch msg.GetMsgCode() {
		case TTIRXD:
			if err := e.handleRXDRow(msg); err != nil {
				return nil, -1, err
			}
		case TTIOER:
			ttioer := msg.(tTIOerIface)
			err = ttioer.getError()
			if err != nil {
				common.Odl.Debug("runExec: TTIOER error", "error", err, "stage", ttioer)
				return nil, -1, err
			}
			oerFound = true
			e.opts &^= driverCommon.UB4(statementParsedByServer)
		case TTIRPA:
			receivedCursorID = msg.(*ttioallrpa).getCursorId()
			common.Odl.Debug("runExec: TTIRPA received", "cursorID", receivedCursorID)
			rowsAffected = int64(msg.(*ttioallrpa).getTotalAffectedRowsCount())
		case TTIFOB:
			if err := stmr.Push(ctx, msg); err != nil {
				common.Odl.Error("StatementExecutorDML.ExecContext: TTIFOB push failed",
					"error", err, "stage", "ttifob-push")
				return nil, -1, common.NewOracleError(oracleErrors.RunExecError, err, "ttifob-push")
			}
			if err := stmr.Flush(ctx); err != nil {
				common.Odl.Error("StatementExecutorDML.ExecContext: TTIFOB flush failed",
					"error", err, "stage", "ttifob-flush")
				return nil, -1, common.NewOracleError(oracleErrors.RunExecError, err, "ttifob-flush")
			}
		default:
			common.Odl.Warn("Unexpected msg received", "message", msg)
			return nil, -1, common.NewOracleError(oracleErrors.InternalError, nil)
		}
		if oerFound {
			common.Odl.Debug("runExec: OER found, breaking")
			break
		}

	}

	common.Odl.Debug("runExec: End of function", "rowsAffected", rowsAffected)
	return &ttcResult{rowsAffected: rowsAffected, shelf: e.shelf}, receivedCursorID, nil
}

// handleContextCancelled cancels statement execution and reads TTIOER message
//
//	returned by the server
func (e *statementProcessor) handleContextCancelled(ctx context.Context) (driverCommon.Message[driverCommon.MessageType], error) {
	cancellationState, ok := ctx.Value(statementCancellationContextKey{}).(*statementCancellationState)
	if ok && cancellationState != nil {
		common.Odl.Debug("Context error received using break-reset protocol, allow after function to start")
		// allow after func to start break-reset
		cancellationCtx, started := cancellationState.requestBreakReset()
		if !started {
			return nil, ctx.Err()
		}
		defer cancellationCtx.CancelFunc()
		common.Odl.Debug("Break-reset completed, fetch OER")
		// The context has been cancelled, cancel current execution and return
		// error
		return e.shelf.GetMessageStreamer().Pull(cancellationCtx.Context, TTIOER)
	}
	return nil, ctx.Err()
}

// handleDCB refreshes the SELECT statement's result metadata and creates a
// fresh rows container for the current execution. Later OEXFEN executions reuse
// the cached descriptors when Oracle does not send another DCB message.
func (e *statementExecutorSelect) handleDCB(state *queryRunState, msg driverCommon.Message[driverCommon.MessageType]) error {
	dcb := msg.(*tTIdcb)
	columns, err := dcb.getColumnContexts()
	// column metadata extraction failure -> OGD-00053 RunQueryError("column-meta")
	if err != nil {
		common.Odl.Error("runQuery: populateColumnMetaData failed", "error", err, "stage", "column-meta")
		return common.NewOracleError(oracleErrors.RunQueryError, err, "column-meta")
	}
	e.resultMetadata.replace(columns)
	state.rows = e.resultMetadata.newRows(e.shelf)
	return nil
}

// createBVC allocates a BVC message for the pre-unmarshal callback and sets its
// column count from the cached result metadata.
func (e *statementExecutorSelect) createBVC(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
	msg, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIBVC)
	// map factory failure -> OGD-00073 CallbackFactoryError("get-bvc")
	if err != nil {
		common.Odl.Error("createBVC: GetMessage(TTIBVC) failed", "error", err, "stage", "get-bvc")
		return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "createBVC failed")
	}
	bvc := msg.(*tTIbvc)
	bvc.SetNumberOfColumns(e.resultMetadata.columnCount())
	return bvc, nil
}

// handleBVC records the column-presence vector for the next row in the current
// execution. handleRXDRow clears the flag after that row has been consumed.
func (s *queryRunState) handleBVC(msg driverCommon.Message[driverCommon.MessageType]) {
	bvc := msg.(*tTIbvc)
	s.bvcFound = bvc.bvcFound
	s.bvcColSent = bvc.bvcColSent
}

// createRXD allocates an RXD message for the pre-unmarshal callback. Cached
// result metadata configures column decoding, while state supplies only the
// current execution's row number and BVC carry information.
func (e *statementExecutorSelect) createRXD(state *queryRunState, t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
	msg, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIRXD)
	// map factory failure -> OGD-00073 CallbackFactoryError("get-rxd")
	if err != nil {
		common.Odl.Error("createRXD: GetMessage(TTIRXD) failed", "error", err, "stage", "get-rxd")
		return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "createRXD failed")
	}
	rxd := msg.(*tTIrxd)

	// supply BVC state and previous row for RXD decoding (delta/continuation)
	rxd.setBvcState(state.bvcColSent, state.bvcFound)
	rxd.setRowCount(state.rowCount)
	rxd.setNumberOfColumns(e.resultMetadata.columnCount())
	// Reuse the existing column metadata slice to avoid per-row datatype allocations.
	rxd.setColumnContexts(e.resultMetadata.columns)
	if state.prevRow != nil {
		rxd.setPrevRow(state.prevRow)
		rxd.setPrevLobColumnContext(state.prevLobColContext)
	}
	// Pass the character set to RXD so that it can be set in lobContext if
	// no character set is returned
	rxd.setSessionNCharacterSet(e.sessCtx.SessionNCharCharacterSet())
	rxd.setSessionCharacterSet(e.sessCtx.DriverCharacterSet())
	state.rowCount++
	return rxd, nil
}

/*
createRXD builds a TTIRXD message for DML execution flows that use RETURNING binds.

Description:
  - Allocates a fresh TTIRXD instance from the message factory for use by the
    pre-unmarshal callback registered during DML execution.
  - Marks the RXD as a DML RETURNING payload when sql.Out binds were detected so
    unmarshalling follows the RETURNING row format instead of regular query-row decoding.
  - Sets the number of returning arguments to the number of OUT bind positions so
    the decoder can extract and populate those returned values correctly.

Parameters:
  - t: incoming message header for the TTIRXD frame. It is currently unused and is
    provided to satisfy the pre-unmarshal callback signature.

Returns:
  - common.Message[common.MessageType]: the initialized *tTIrxd instance.
  - error: non-nil if the message factory cannot allocate TTIRXD.
*/
func (e *statementExecutorDML) createRXD(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
	msg, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIRXD)
	// map factory failure -> OGD-00073 CallbackFactoryError("get-rxd")
	if err != nil {
		common.Odl.Error("createRXD: GetMessage(TTIRXD) failed", "error", err, "stage", "get-rxd")
		return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "createRXD failed")
	}
	rxd := msg.(*tTIrxd)
	rxd.setNumberofReturningArgs(len(e.outDestPtrs))
	rxd.setDmlReturning()
	return rxd, nil
}

/*
handleRXDRow decodes a TTIRXD row for DML/PLSQL OUT or RETURNING binds and writes
the decoded values into the registered destination pointers.

Description:
  - Asserts the incoming TTC message to *tTIrxd and iterates over the RETURNING/OUT
    values carried in the row payload.
  - Uses the stored output column contexts to select the appropriate type decoder for
    each returned value.
  - Decodes each wire value into a Go value and assigns it into the corresponding
    destination pointer when the decoded type is assignable or convertible.

Parameters:
  - msg: the TTIRXD message received during statement execution.

Returns:
  - error: non-nil if decoder lookup or value decoding fails; otherwise nil.

Notes:
  - Nil destinations, out-of-range returned values, and nil decoded values are skipped.
  - Assignment is best-effort and only occurs for non-nil pointer destinations whose
    target type can accept the decoded value directly or via conversion.
*/
func (e *statementExecutorExec) handleRXDRow(msg driverCommon.Message[driverCommon.MessageType]) error {
	// TTIRXD carries the returned row values for OUT and DML RETURNING binds.
	rxd := msg.(*tTIrxd)
	codecFactory := e.shelf.GetCodecFactory()

	for i, dest := range e.outDestPtrs {
		// Skip destinations that have no matching returned value
		// or no data received from server.
		if i >= len(rxd.row) || len(rxd.row[i]) == 0 {
			continue
		}

		// Use captured OAC-derived metadata when available to choose the correct decoder.
		columnContext := columnContext{}
		if i < len(e.outColumnContexts) {
			columnContext = e.outColumnContexts[i]
		}

		// Decode the TTC payload for this returned bind position into a Go value.
		decoder, err := codecFactory.getDecoder(columnContext.DataType)
		if err != nil {
			return err
		}

		value, err := decoder.decodeToType(columnContext, rxd.row[i])
		if err != nil {
			return err
		}
		if value == nil {
			continue
		}
		if dest == nil {
			continue
		}

		// If the destination implements sql.Scanner, delegate the decoded value to it.
		if scanner, ok := dest.(sql.Scanner); ok {
			if err := scanner.Scan(value); err != nil {
				return err
			}
			continue
		}

		// Assign directly when types match, otherwise fall back to conversion when allowed.
		decodedValue := reflect.ValueOf(value)
		target := reflect.ValueOf(dest).Elem()
		if !decodedValue.IsValid() {
			continue
		}
		if decodedValue.Type().AssignableTo(target.Type()) {
			target.Set(decodedValue)
			continue
		}
		if decodedValue.Type().ConvertibleTo(target.Type()) {
			target.Set(decodedValue.Convert(target.Type()))
		}
	}

	return nil
}

/*
registerDMLCallbacks uses the TTC callbacks required to execute DML statements
that may return OUT, IN OUT, or RETURNING bind values.

Description:
  - Registers a TTIRXD pre-unmarshal callback that allocates and configures RXD
    messages for DML RETURNING payloads.
  - Registers a TTIIOV pre-unmarshal callback that prepares the IOV container and
    nested RXH state needed for returned bind metadata.
  - Registers a TTIRPA post-unmarshal callback that consumes optional per-iteration
    DML row count data appended after the normal OALL8 response.
  - Registers a dropper callback for TTIIOV so the message streamer treats that
    message as handled even though execution logic does not switch on it directly.

Parameters:
  - ctx: execution context used when unmarshalling trailing DML row-count metadata
    from the TTIRPA callback.

Notes:
  - All callbacks registered here must be removed by unregisterDMLCallbacks to avoid
    leaking DML-specific behavior into subsequent executions.
*/
func (e *statementExecutorDML) registerDMLCallbacks(ctx context.Context) {
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.RegisterPreUnmarshallCallback(TTIRXD, e.createRXD)

	common.Odl.Debug("registerDMLOallRpaCallbacks: registering II-RPA callback")
	// For DML, capture optional per-iteration row counts carried after TTIRPA by calling UnMarshalDMLRows.
	// Register a post-unmarshal callback for TTIRPA (oAll8) so we can consume the trailing AL8PIDMLRC section.
	stmr.RegisterPostUnmarshallCallback(TTIRPA, func(msg driverCommon.Message[driverCommon.MessageType], prevErr error) (bool, error) {
		if prevErr != nil {
			return false, prevErr
		}
		if rpa, ok := msg.(*ttioallrpa); ok {
			// map DML row count parsing failure -> OGD-00060 RunExecError("unmarshal-dml-rows")
			if err := rpa.UnMarshalDMLRows(ctx, e.shelf.GetMarshaller()); err != nil {
				common.Odl.Error("StatementExecutorDML.ExecContext: UnMarshalDMLRows failed",
					"error", err, "stage", "unmarshal-dml-rows")
				return false, common.NewOracleError(oracleErrors.RunExecError, err, "ExecContext failed")
			}
		}
		return true, nil
	})
	e.registerIOVCallbacks(ctx)
}

/*
registerIOVCallbacks registers TTC callbacks for the IOV message sequence used by
OUT, IN OUT, and DML RETURNING bind handling.

Description:
  - Registers a TTIIOV pre-unmarshal callback that allocates a fresh IOV message
    instance from the message factory for each incoming frame.
  - Initializes the nested RXH message required by TTIIOV unmarshalling and sets
    the number of bind positions so the IOV decoder can size its internal state
    for the current execution.
  - Registers a TTIIOV post-unmarshal callback that marks the message as handled
    without surfacing it to the main execution loop, since the relevant state is
    consumed during unmarshalling.

Parameters:
  - ctx: execution context associated with the current statement execution. It is
    currently unused in this helper but kept to align with the surrounding callback
    registration flow.

Notes:
  - Callers must unregister the TTIIOV callbacks after execution completes to avoid
    leaking IOV-specific unmarshalling behavior into subsequent statements.
*/
func (e *statementExecutorExec) registerIOVCallbacks(ctx context.Context) {
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.RegisterPreUnmarshallCallback(TTIIOV, func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		msg, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIIOV)
		// map factory failure -> OGD-00073 CallbackFactoryError("get-rxd")
		if err != nil {
			common.Odl.Error("createRXD: GetMessage(TTIIOV) failed", "error", err, "stage", "get-rxd")
			return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "createRXD failed")
		}
		ttiiov := msg.(*tTIiov)

		rxh, err := e.shelf.GetMessageFactory().(Factory).GetMessage(TTIRXH)
		ttiiov.rxh = rxh.(*tTIrxh)
		ttiiov.numberOfBindPositions = len(e.bindValues)
		return ttiiov, nil
	})
	// Post-unmarshal dropper for unhandled messages in query Pull.
	dropUnhandled := func(msg driverCommon.Message[driverCommon.MessageType], prevErr error) (bool, error) {
		if prevErr != nil {
			return false, prevErr
		}
		return false, nil
	}
	stmr.RegisterPostUnmarshallCallback(TTIIOV, dropUnhandled)
}

// handleRXDRow processes an RXD row message and retains its aligned data and
// LOB metadata for possible BVC carry into the next row.
func (s *queryRunState) handleRXDRow(msg driverCommon.Message[driverCommon.MessageType]) {
	if rxd, ok := msg.(*tTIrxd); ok && rxd != nil {
		currRow := make([]driverCommon.B1Array, len(rxd.row))
		for i := range rxd.row {
			currRow[i] = append(driverCommon.B1Array(nil), rxd.row[i]...)
		}
		// RXD messages are created per row and their LOB contexts are read-only
		// after unmarshalling, so rows and BVC state can safely share this slice.
		currLobColContext := rxd.getLobColumnContext()
		s.rows.rowData = append(s.rows.rowData, currRow)
		s.rows.lobColContext = append(s.rows.lobColContext, currLobColContext)
		s.prevRow = currRow
		common.Odl.Debug("handleRXDRow: appended RXD row", "len", len(rxd.row))
		s.prevLobColContext = currLobColContext
		common.Odl.Debug("handleRXDRow: appended RXD row", "len", len(rxd.row))
	}
	s.bvcColSent = nil
	s.bvcFound = false
}

// registerRunQueryCallbacks sets up all required pre-unmarshal callbacks for a query context.
func (e *statementExecutorSelect) registerRunQueryCallbacks(state *queryRunState) {
	common.Odl.Debug("registerRunQueryCallbacks: starting")
	// Registers pre-unmarshal callback for TTIRXD message
	stmr := e.shelf.GetMessageStreamer().(MessageStreamerInterface)
	stmr.RegisterPreUnmarshallCallback(TTIRXD, func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return e.createRXD(state, t)
	})
	stmr.RegisterPreUnmarshallCallback(TTIBVC, e.createBVC)
	registerOallRpaCallbacks(stmr, e.shelf)
	// Post-unmarshal dropper for unhandled messages in query Pull.
	dropUnhandled := func(msg driverCommon.Message[driverCommon.MessageType], prevErr error) (bool, error) {
		if prevErr != nil {
			return false, prevErr
		}
		return false, nil
	}
	// In runQuery Pull: TTIOER, TTIDCB, TTIBVC, TTIRXD
	// Unhandled in switch: TTIRPA, TTIRXH
	// todo: when we need the flags from RPA, BVC when more types
	// todo: are added, remove it from here and add them to pull and handle them.
	stmr.RegisterPostUnmarshallCallback(TTIRXH, dropUnhandled)
}

// unregisterRunQueryCallbacks removes query context pre-unmarshal callbacks.
func unregisterRunQueryCallbacks(stmr MessageStreamerInterface) {
	stmr.UnRegisterPreUnmarshallCallback(TTIRXD)
	stmr.UnRegisterPreUnmarshallCallback(TTIBVC)
	stmr.UnRegisterPreUnmarshallCallback(TTIRPA)

	// Unregister post-unmarshal droppers for unhandled query messages.
	stmr.UnRegisterPostUnmarshallCallback(TTIRPA)
	stmr.UnRegisterPostUnmarshallCallback(TTIRXH)
}

// registerRunExecCallbacks sets up required pre-unmarshal OALLRPA callback for exec context.
func registerRunExecCallbacks(stmr MessageStreamerInterface, shelf *ttiShelf[driverCommon.MessageType]) {
	registerOallRpaCallbacks(stmr, shelf)
}

// unregisterRunExecCallbacks removes TTIRPA OALLRPA callback just for exec context.
func unregisterRunExecCallbacks(stmr MessageStreamerInterface) {
	stmr.UnRegisterPreUnmarshallCallback(TTIRPA)
	stmr.UnRegisterPostUnmarshallCallback(TTIRPA)
}

// registerOallRpaCallbacks registers OALLRPA for TTIRPA (used in both query and exec contexts).
func registerOallRpaCallbacks(stmr MessageStreamerInterface, shelf *ttiShelf[driverCommon.MessageType]) {
	common.Odl.Debug("registerOallRpaCallback: registering II-RPA callback")
	// map factory failure -> OGD-00073 CallbackFactoryError("get-oallrpa")
	stmr.RegisterPreUnmarshallCallback(TTIRPA, func(t *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oAll8)
		if err != nil {
			common.Odl.Error("registerOallRpaCallback: GetMessageForFunction(TTIRPA,oAll8) failed", "error", err, "stage", "get-oallrpa")
			return nil, common.NewOracleError(oracleErrors.CallbackFactoryError, err, "registerOallRpaCallback failed")
		}
		return msg, nil
	})
}

// registerDMLOallRpaCallbacks registers OALLRPA for TTIRPA (used in both query and exec contexts).
func registerDMLOallRpaCallbacks(ctx context.Context, stmr MessageStreamerInterface, shelf *ttiShelf[driverCommon.MessageType]) {
	common.Odl.Debug("registerDMLOallRpaCallbacks: registering II-RPA callback")
	stmr.RegisterPostUnmarshallCallback(TTIRPA, func(msg driverCommon.Message[driverCommon.MessageType], prevErr error) (bool, error) {
		if prevErr != nil {
			return false, prevErr
		}
		if rpa, ok := msg.(*ttioallrpa); ok {
			// map DML row count parsing failure -> OGD-00060 RunExecError("unmarshal-dml-rows")
			// todo: once ttcResult is implemented, consume the rows returned.
			if err := rpa.UnMarshalDMLRows(ctx, shelf.GetMarshaller()); err != nil {
				common.Odl.Error("StatementExecutorDML.ExecContext: UnMarshalDMLRows failed",
					"error", err, "stage", "unmarshal-dml-rows")
				return false, common.NewOracleError(oracleErrors.RunExecError, err, "ExecContext failed")
			}
		}
		return true, nil
	})
}

// EXECUTORS FOR ERROR CASES

// statementExecutorOperationNotSupported implements queries and exec that return errors
type statementExecutorOperationNotSupported struct {
	kind sqlKind
}

/*
ExecContext implements common.ExecWithContext for the "operation not supported" executor.

This method is used when a statement was classified as a query (row-producing, e.g. SELECT)
but database/sql attempts to execute it via an Exec path.

Why this can happen:
  - Caller uses Exec/ExecContext on a prepared statement that the driver classified as a query.
  - database/sql may choose an exec path in some edge cases depending on the interfaces a Stmt exposes.

Behavior:
  - Logs an error indicating the kind mismatch.
  - Returns a driver-level error (StatementExecutionFailed) describing the unsupported operation.

Notes:
  - The provided ctx/query/args are intentionally ignored because no wire round-trip is performed.
*/
func (s statementExecutorOperationNotSupported) ExecContext(_ context.Context, _ *qualifiedSQLStatement, _ []driver.NamedValue) (driver.Result, error) {
	common.Odl.Error("QueryStatement: unsupported type for Exec",
		"error", nil, "sqlKind", s.kind)
	return nil, common.NewOracleError(oracleErrors.StatementExecutionFailed, nil,
		s.kind.String(), "Exec")
}

/*
QueryContext implements common.QueryWithContext for the "operation not supported" executor.

This method is used when a statement was classified as a non-query (non row-producing,
e.g. INSERT/UPDATE/DELETE/DDL/PLSQL) but database/sql attempts to execute it via a Query path.

Why this can happen:
  - Caller uses Query/QueryContext on a prepared statement that the driver classified as exec.
  - database/sql may call Query even for exec statements depending on which Stmt interfaces are used.

Behavior:
  - Logs an error indicating the kind mismatch.
  - Returns a driver-level error (StatementExecutionFailed) describing the unsupported operation.

Notes:
  - The provided ctx,query/args are intentionally ignored because no wire round-trip is performed.
*/
func (s statementExecutorOperationNotSupported) QueryContext(_ context.Context, _ *qualifiedSQLStatement, _ []sqldriver.NamedValue) (sqldriver.Rows, error) {
	common.Odl.Error("ExecStatement: unsupported type for Query",
		"error", nil, "sqlKind", s.kind)
	return nil, common.NewOracleError(oracleErrors.StatementExecutionFailed, nil,
		s.kind.String(), "Query")
}
