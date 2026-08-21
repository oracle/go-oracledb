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
	"strconv"
	"strings"

	"reflect"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// connection implements database/sql/driver.Conn and holds negotiated session
// artifacts.
type connection struct {
	shelf   *ttiShelf[driverCommon.MessageType]
	sessCtx *driverCommon.SessionContext
	ns      driverCommon.NetworkSession
	// _isClosed is used to know if the connection has been closed. If _isClosed
	// is set to true the connection cannot be used.
	_isClosed bool
	// _isValid will be set to true when the connection is created and should be
	// set to false when the connection is no longer valid. This can happen when
	// an operation failed and could not be stopped in the server (break-reset
	// failure) or when then connectionShouldBeDropped flag is received on an STA
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
  - error: Non-nil if the connection is closed/invalid or statement creation fails
    (propagated from PrepareContext).
*/
func (c *connection) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(common.BackgroundContext, query)
}

/*
PrepareContext creates a new statement for the provided SQL query using the supplied context.

Description:
  - Validates the connection is open and usable; if closed or invalid, returns an Oracle error.
  - Creates a TTC statement bound to this connection and returns it. The provided context is
    accepted for API symmetry and cancellation/deadline propagation for future extensions.

Parameters:
- ctx: Context for the prepare operation (cancellation, deadlines).
- query: SQL text to prepare.

Returns:
- driver.Stmt: Prepared statement bound to this connection and query.
- error: Non-nil if the connection is closed/invalid.
*/
func (c *connection) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	common.Odl.Debug("Connection.PrepareContext: creating statement...")
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	return stmt, nil
}

// ExecContext implements driver.ExecerContext.
// It creates a TTC Statement and delegates the execution.
func (c *connection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	common.Odl.Debug("Connection.ExecContext: creating statement...")
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	defer stmt.Close()
	result, execErr := stmt.ExecContext(ctx, args)
	return result, c.shelf.LocalizeError(execErr)
}

// QueryContext implements driver.QueryerContext.
// It creates a TTC Statement and delegates the query.
func (c *connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	defer stmt.Close()
	result, err := stmt.QueryContext(ctx, args)
	return result, c.shelf.LocalizeError(err)
}

// cancelCurrentExecution sends a request to the database to cancel the current
// operation
func (c *connection) cancelCurrentExecution(ctx context.Context) error {
	if err := c.ns.CancelOperation(ctx); err != nil {
		// If an error occurs during cancellation, mark the connection as invalid so that
		// it will be dropped form the pool
		c._isValid = false
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
	c._isValid = !msg.(connectionStatusProvider).isBeingDrainned()
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

// notify implements eventListener.
func (c *connection) notify(event eventType) {
	var wasValid = c._isValid == true
	switch event {
	case streamerStaleEvent:
		common.Odl.Debug("Connection.notify: streamer stale received")
		c._isValid = false
	case streamerOverFlowEvent:
		common.Odl.Debug("Connection.notify: streamer overflow received")
		c._isValid = false
	default:
		common.Odl.Debug("Connection.notify: received", "evt", event)
	}
	if wasValid == true && c._isValid == false {
		c.shelf.getEventService().post(connectionInvalidatedEvent)
	}
}

// CheckNamedValue allows sql.Out binds to pass through database/sql conversion.
// For all other values, we delegate back to database/sql default conversion.
func (c *connection) CheckNamedValue(nv *driver.NamedValue) error {
	return c.shelf.LocalizeError(checkNamedValue(nv))
}

// checkNamedValue validates sql.Out destinations and returns shelf-localized
// Oracle errors for binding problems.
func checkNamedValue(nv *driver.NamedValue) error {
	if out, ok := nv.Value.(sql.Out); ok {
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
