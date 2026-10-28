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
	"errors"
	"fmt"
	ttcLob "github.com/oracle/go-oracledb/v26/internal/driver/ttc/lob"
	"strconv"
	"strings"

	"reflect"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// connection implements database/sql/driver.Conn and holds negotiated session
// artifacts.
type connection struct {
	shelf   *ttiShelf[driverCommon.MessageType]
	sessCtx *driverCommon.SessionContext
	ns      driverCommon.NetworkSession
	// leaseGeneration is set only on the private LOB-operation facade returned
	// to a typed query LOB. It makes that capability invalid as soon as
	// database/sql releases the owning physical connection to its pool.
	leaseGeneration *uint64
	// _isClosed is used to know if the connection has been closed. If _isClosed
	// is set to true the connection cannot be used.
	_isClosed bool
	// _isValid will be set to true when the connection is created and should be
	// set to false when the connection is no longer valid. This can happen when
	// an operation failed and could not be stopped in the server (break-reset
	// failure) or when the connectionShouldBeDropped flag is received on an STA
	// or OER message (TODO).
	_isValid bool
}

// newConnection constructs a new Oracle connection wrapping negotiated state.
// It returns an error when the server timezone cannot be initialized.
func newConnection(
	ctx context.Context,
	shelf *ttiShelf[driverCommon.MessageType],
	sessCtx *driverCommon.SessionContext,
	ns driverCommon.NetworkSession,
) (*connection, error) {
	conn := &connection{
		shelf:     shelf,
		sessCtx:   sessCtx,
		ns:        ns,
		_isClosed: false,
		_isValid:  true,
	}
	conn.registerEventListeners(conn.shelf.getEventService())
	_registerHandleConnectionShouldBeDropped(shelf, conn)
	shelf.registerCancelExecution(conn.cancelCurrentExecution)
	if err := conn._registerServerTimezoneOffset(ctx); err != nil {
		return nil, err
	}
	return conn, nil
}

/*
Prepare creates a new statement for the provided SQL query. This function is deprecated.
It is preferred to call Conn.PrepareContext(context, query)

Description:
  - Implements database/sql/driver.Conn.Prepare by delegating to PrepareContext
    with a background context.

Parameters:
- query: SQL text to prepare.

Output:
  - driver.Stmt: Statement bound to this connection and query.
  - error: Non-nil if SQL classification or statement construction fails
    (propagated from PrepareContext).
*/
func (c *connection) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(common.BackgroundContext, query)
}

/*
PrepareContext creates a new statement for the provided SQL query using the supplied context.

Description:
  - Parses and classifies the SQL and creates a TTC statement bound to this connection.
  - The provided context is accepted for API symmetry and future cancellation/deadline
    propagation during statement construction.

Parameters:
- ctx: Context for the prepare operation (cancellation, deadlines).
- query: SQL text to prepare.

Returns:
- driver.Stmt: Prepared statement bound to this connection and query.
- error: Non-nil if SQL classification or statement construction fails.
*/
func (c *connection) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	common.Odl.Fine("Connection.PrepareContext: creating statement...")
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	return stmt, nil
}

// ExecContext implements driver.ExecerContext for direct, unprepared
// execution. It creates a short-lived Statement, executes the supplied SQL
// binds, and closes the Statement before returning. Public streamed-LOB
// markers are materialized into temporary locator binds during execution.
func (c *connection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	common.Odl.Fine("Connection.ExecContext: creating statement...")
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	defer stmt.Close()
	result, execErr := stmt.execContext(ctx, args)
	return result, c.shelf.LocalizeError(execErr)
}

// QueryContext implements driver.QueryerContext for direct queries. It creates a Statement, and on
// success transfers Statement/cursor ownership to the returned ttcRows. Error
// paths retain ownership and close the Statement immediately. The ownership
// transfer keeps database/sql's physical connection checked out until Rows is
// closed, which is required for later locator-backed LOB RPCs.
func (c *connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	result, err := stmt.queryContext(ctx, args)
	if err != nil {
		_ = stmt.Close()
		return nil, c.shelf.LocalizeError(err)
	}
	switch rows := result.(type) {
	case *ttcRows:
		if !rows.takeStatementOwnership(stmt) {
			_ = stmt.Close()
			return nil, c.shelf.LocalizeError(common.NewOracleError(
				oracleErrors.InternalError,
				errors.New("query Rows closed before Statement ownership transfer"),
			))
		}
		return rows, nil
	case *ttcRowsRefCursor:
		// The internal REF CURSOR wrapper returns a cursor already owned by this
		// physical connection; its short-lived wrapper Statement owns no TTC cursor.
		if err := stmt.Close(); err != nil {
			return nil, c.shelf.LocalizeError(err)
		}
		return rows, nil
	case *ttcRowsRefCursorImplicitFetch:
		// Implicit result cursors own their result-set lifetime and retain this
		// Statement until Rows.Close.
		if !rows.takeStatementOwnership(stmt) {
			_ = stmt.Close()
			return nil, c.shelf.LocalizeError(common.NewOracleError(
				oracleErrors.InternalError,
				errors.New("query Rows closed before Statement ownership transfer"),
			))
		}
		return rows, nil
	default:
		_ = stmt.Close()
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.InternalError, nil))
	}
}

// GetRows fetches an already-decoded REF CURSOR on its owning connection.
// It is called by oracle.connectionWrapper before the cursor is exposed as
// standard database/sql rows.
func (c *connection) GetRows(ctx context.Context, rows driver.Rows) error {
	cursor, _ := rows.(*ttcRowsRefCursor)
	// Check ownership before setting any fetch dependencies. Doing this after
	// injection would let a cursor returned by another physical connection be claimed.
	if !c.shelf.hasRefCursor(cursor) {
		common.Odl.Error("REF CURSOR fetch rejected because cursor belongs to another connection", "cursorID", cursor.cursorID)
		return common.NewOracleError(oracleErrors.RefCursorConnectionMismatch, nil)
	}
	// Configure the rows with their owning connection's dependencies.
	cursor.attachShelf(c.shelf)
	cursor.attachSessionContext(c.sessCtx)
	cursor.ttcRows.setContext(ctx)
	err := cursor.Fetch(ctx)
	if err != nil {
		common.Odl.Error("REF CURSOR fetch failed", "error", err)
		return err
	}
	return nil
}

// cancelCurrentExecution sends a request to the database to cancel the current
// operation
func (c *connection) cancelCurrentExecution(ctx context.Context) error {
	if err := c.ns.CancelOperation(ctx); err != nil {
		// If an error occurs during cancellation, mark the connection as invalid so that
		// it will be dropped form the pool
		c.invalidate()
		e := common.NewOracleError(oracleErrors.CancelOperationError, err, nil)
		return c.shelf.LocalizeError(e)
	}
	return nil
}

// Handle invalidating connections when connectionStatusReceiver is received in
// TTIOER or TTISTA due to planned-down.

// connectionStatusProvider implemented by messages that may received end of
// call status like TTIOER and TTISTA
type connectionStatusProvider interface {
	// isBeingDrainned returns true if the connection should be dropped due to a
	// planned-down, otherwise false
	isBeingDrainned() bool
}

// _registerHandleConnectionShouldBeDropped registers post unmarshal callbacks
// that invalidates connections that should be dropped due to a planned-down
func _registerHandleConnectionShouldBeDropped(shelf *ttiShelf[driverCommon.MessageType], connection *connection) {
	messageStreamer := shelf.GetMessageStreamer().(MessageStreamerInterface)
	messageStreamer.RegisterPostUnmarshallCallback(TTIOER, connection._handleConnectionShouldBeDropped)
	messageStreamer.RegisterPostUnmarshallCallback(TTISTA, connection._handleConnectionShouldBeDropped)
}

// _handleConnectionShouldBeDropped post unmarshal callback that invalidates
// connections that should be dropped. Messages are kept in the queue to be
// handled by the caller
func (c *connection) _handleConnectionShouldBeDropped(msg driverCommon.Message[driverCommon.MessageType], _ error) (bool, error) {
	// if connectionSouldBeDropped flag was received in TTISTA or TTIOER, it means
	// that the connection is being drainned and it should be closed and not
	// released to the connection pool
	if msg.(connectionStatusProvider).isBeingDrainned() {
		c.invalidate()
	}
	// return always true, the incoming message should be kept
	return true, nil
}

// String implements the Stringer interface
func (c *connection) String() string {
	return fmt.Sprintf("Connection { isOpen: %v, isValid: %v }", !c._isClosed, c._isValid)
}

func (c *connection) registerEventListeners(service *eventService) {
	service.register(c, streamerStaleEvent)
	service.register(c, streamerOverFlowEvent)
}

// notify implements EventListener interface
func (c *connection) notify(event eventType) {
	switch event {
	case streamerStaleEvent:
		common.Odl.Fine("Connection.notify: streamer stale received")
		c.invalidate()
	case streamerOverFlowEvent:
		common.Odl.Fine("Connection.notify: streamer overflow received")
		c.invalidate()
	default:
		common.Odl.Fine("Connection.notify: received", "evt", event)
	}
}

// invalidate marks the connection unusable and posts the invalidation event.
func (c *connection) invalidate() {
	wasValid := c._isValid
	c._isValid = false
	if c.shelf != nil && c.shelf.lobState != nil {
		c.shelf.lobState.invalidate()
	}
	if wasValid {
		discardLobSessionState(c.shelf)
		c.shelf.getEventService().post(connectionInvalidatedEvent)
	}
}

// CheckNamedValue implements driver.NamedValueChecker. It admits public Oracle LOB markers and validated private streamed LOB
// carriers without asking database/sql's default converter to flatten them.
// sql.Out receives the existing destination validation; all ordinary values
// return driver.ErrSkip.
func (c *connection) CheckNamedValue(nv *driver.NamedValue) error {
	return c.shelf.LocalizeError(checkNamedValue(nv))
}

// checkNamedValue performs package-level NamedValue classification shared by
// Connection and Statement.
func checkNamedValue(nv *driver.NamedValue) error {
	// connectionWrapper.Fetch passes an already-decoded driver.Rows to the internal
	// REF CURSOR statement executor. This is not a REF CURSOR OUT-bind destination.
	if _, ok := nv.Value.(driver.Rows); ok {
		return nil
	}
	switch value := nv.Value.(type) {
	case internallob.BindBlob, internallob.BindClob, internallob.BindNClob:
		return nil
	case ttcLob.BindInput:
		return value.Validate()
	}
	if out, ok := nv.Value.(sql.Out); ok {
		if isRefCursorDestination(out.Dest) {
			return nil
		}
		// REF CURSOR OUT binds must use *datatype.Cursor, not the driver.Rows
		// protocol interface.
		if _, ok := out.Dest.(*driver.Rows); ok {
			return common.NewOracleError(oracleErrors.InvalidRefCursorOutParameter, nil)
		}
		// Destination must be provided for output binding.
		if out.Dest == nil {
			return common.NewOracleError(oracleErrors.InvalidSqlOutParameter, errors.New("nil destination"))
		}

		destInfo := reflect.ValueOf(out.Dest)
		// Destination must be a pointer so the driver can write back the value.
		if destInfo.Kind() != reflect.Ptr {
			return common.NewOracleError(oracleErrors.InvalidSqlOutParameter, errors.New("non pointer"))
		}

		// Destination pointer must not be nil.
		if destInfo.IsNil() {
			return common.NewOracleError(oracleErrors.InvalidSqlOutParameter, errors.New("nil pointer"))
		}

		pointedValue := reflect.Indirect(destInfo)
		// Pointer-to-pointer is not supported; only pointer-to-value is allowed.
		if pointedValue.Kind() == reflect.Ptr {
			return common.NewOracleError(oracleErrors.InvalidSqlOutParameter, errors.New("double pointer"))
		}

		_, err := driver.DefaultParameterConverter.ConvertValue(out.Dest)
		return err
	}
	return driver.ErrSkip
}

// isRefCursorDestination reports whether v is a destination pointer for a REF CURSOR OUT bind.
func isRefCursorDestination(v any) bool {
	_, ok := v.(*datatype.Cursor)
	return ok
}

func (c *connection) _registerServerTimezoneOffset(ctx context.Context) error {
	// DBTIMEZONE can return either a region name or an offset; TZ_OFFSET normalizes
	// both forms to the +/-HH:MM format expected by parseTimeZone.
	rows, err := c.QueryContext(ctx, "SELECT TZ_OFFSET(DBTIMEZONE) FROM SYS.DUAL", nil)
	if err != nil {
		return c.shelf.LocalizeError(common.NewOracleError(oracleErrors.ServerTimeZoneError, err, "query"))
	}
	defer rows.Close()
	values := make([]driver.Value, 1)
	var serverTimeZone string
	values[0] = &serverTimeZone
	if err := rows.Next(values); err != nil {
		return c.shelf.LocalizeError(common.NewOracleError(oracleErrors.ServerTimeZoneError, err, "retrieve"))
	}
	serverTimeZoneValue := values[0].(string)
	TZH, TZM, err := parseTimeZone(serverTimeZoneValue)
	if err != nil {
		return err
	}
	c.shelf.registerServerTimeZoneOffset(int16(TZH*3600 + TZM*60))
	return nil
}

// parseTimeZone parses an Oracle-style timezone string such as "+05:30" or "-08:15".
//
// Parameters:
//   - timezone: The timezone string to parse.
//
// Returns:
//   - hours: The parsed hour offset.
//   - minutes: The parsed minute offset.
//   - err: An error describing why parsing failed, or nil on success.
func parseTimeZone(timezone string) (int, int, error) {
	var sign int = 1
	trimmedTimezone := timezone
	if strings.HasPrefix(trimmedTimezone, "-") {
		sign = -1
		trimmedTimezone = trimmedTimezone[1:]
	} else if strings.HasPrefix(trimmedTimezone, "+") {
		trimmedTimezone = trimmedTimezone[1:]
	}

	items := strings.Split(trimmedTimezone, ":")
	if len(items) != 2 {
		return 0, 0, common.NewOracleError(oracleErrors.ServerTimeZoneError, nil, "parse")
	}

	TZH, err := strconv.Atoi(items[0])
	if err != nil {
		return 0, 0, common.NewOracleError(oracleErrors.ServerTimeZoneError, err, "parse")
	}
	TZM, err := strconv.Atoi(items[1])
	if err != nil {
		return 0, 0, common.NewOracleError(oracleErrors.ServerTimeZoneError, err, "parse")
	}

	return sign * TZH, sign * TZM, nil
}
