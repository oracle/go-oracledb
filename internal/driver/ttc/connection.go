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
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
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
//
// Parameters:
//   - ctx: Context used while initializing the connection.
//   - shelf: TTC message, capability, and event state for the connection.
//   - sessCtx: Session context negotiated with the server.
//   - ns: Network session used by the connection.
//
// Returns:
//   - *connection: Initialized connection.
//   - error: Error if connection initialization fails.
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
	_registerHandleEndOfCallStatus(shelf, conn)
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
	common.Odl.Fine("Connection.PrepareContext: creating statement...")
	stmt, err := newStatement(c.shelf, c.sessCtx, query)
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	return stmt, nil
}

// ExecContext implements driver.ExecerContext.
// It creates a TTC Statement and delegates the execution.
func (c *connection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	common.Odl.Fine("Connection.ExecContext: creating statement...")
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

// connectionStatusProvider is implemented by messages that may receive
// end-of-call status, such as TTIOER and TTISTA.
//
// End-of-call status message contains information about the current
// connection such as: the connection is being drained or there is an
// active transaction in the connection. Messages that implement this
// interface are handled by the connection to update its status.
type connectionStatusProvider interface {
	// isBeingDrained returns true if the connection should be dropped due to a
	// planned-down; otherwise, it returns false.
	//
	// Returns:
	//   - bool: Whether the connection should be dropped.
	isBeingDrained() bool
	// transactionState returns the End-of-Call transaction status reported by the server
	//
	// Returns:
	//   - endOfCallStatusTransactionState: the status reported by the server
	transactionState() endOfCallStatusTransactionState
}

// _registerHandleEndOfCallStatus registers post-unmarshal callbacks that
// handles end-of-call status messages.
//
// Parameters:
//   - shelf: TTC shelf whose message streamer receives the callbacks.
//   - connection: Connection whose validity is updated by the callbacks.
//
// Returns:
//   - None. The callbacks are registered on the message streamer.
func _registerHandleEndOfCallStatus(shelf *ttiShelf[driverCommon.MessageType], connection *connection) {
	messageStreamer := shelf.GetMessageStreamer().(MessageStreamerInterface)
	messageStreamer.RegisterPostUnmarshallCallback(TTIOER, connection._handleEndOfCallStatus)
	messageStreamer.RegisterPostUnmarshallCallback(TTISTA, connection._handleEndOfCallStatus)
}

// _handleEndOfCallStatus is a post-unmarshal callback that handles end-of-call
// messages. It can invalidate connections that should be dropped, or update the
// connection's active transaction status. Messages are kept in the queue to be
// handled by the caller.
//
// Parameters:
//   - msg: Message containing end-of-call status.
//   - _: Previous unmarshalling error, ignored by this callback.
//
// Returns:
//   - bool: Always true so the message remains available to the caller.
//   - error: Always nil.
func (c *connection) _handleEndOfCallStatus(msg driverCommon.Message[driverCommon.MessageType], _ error) (bool, error) {
	// If the connectionShouldBeDropped flag was received in TTISTA or TTIOER, it means
	// that the connection is being drained and it should be closed and not
	// released to the connection pool
	c._isValid = !msg.(connectionStatusProvider).isBeingDrained()
	// check for active transaction
	transactionState := msg.(connectionStatusProvider).transactionState()
	currentTransaction := c.shelf.getTransaction()
	switch transactionState {
	case active:
		if currentTransaction == nil {
			// This should never happen, by default the driver is in auto-commit mode
			// so no transaction is created.
			common.Odl.Debug("The server has reported an active transaction and there is no active transaction in the client")
		} else {
			// The state is received in all round-trips of the transaction
			// we only change the state if the state is still started on
			// client
			if currentTransaction.currentTransactionState() == transactionStartedClient {
				common.Odl.Fine("Transaction started on server")
				// Standard transactions are acknowledged through End-of-Call
				// status. Sessionless transactions use the SESSIONLESS_GTRID
				// session-property event instead.
				currentTransaction.transactionState(transactionStartedServer)
			}
		}
	case inactive:
		if currentTransaction != nil {
			common.Odl.Fine("Transaction ended on server")
			// The server no longer has a standard transaction that was
			// registered locally.
			currentTransaction.endTransaction()
		} else {
			// If this is a sessionless transaction, this happens if the
			// transaction is a sessionless transaction. The GTRID update
			// message is received before and unregisters the transaction
			common.Odl.Fine("The server has reported the end of a transaction")
		}
	}
	// return always true, the incoming message should be kept
	return true, nil
}

// String implements the Stringer interface.
//
// Returns:
//   - string: Human-readable connection state.
func (c *connection) String() string {
	return fmt.Sprintf("Connection { isOpen: %v, isValid: %v }", !c._isClosed, c._isValid)
}

// registerEventListeners registers connection handlers for connection and
// session-property events.
//
// Parameters:
//   - service: Event service receiving the connection handlers.
//
// Returns:
//   - None. The listeners are registered on service.
func (c *connection) registerEventListeners(service *eventService) {
	service.register(c, streamerStaleEvent)
	service.register(c, streamerOverFlowEvent)
	service.register(c, sessionPropertiesUpdateEvent)
}

// notify implements eventListener.
//
// Parameters:
//   - event: Event received by the connection.
//
// Returns:
//   - None. Connection state and transaction state may be updated in place.
func (c *connection) notify(event eventType, e eventData) {
	var wasValid = c._isValid == true
	switch event {
	case streamerStaleEvent:
		common.Odl.Fine("Connection.notify: streamer stale received")
		c._isValid = false
	case streamerOverFlowEvent:
		common.Odl.Fine("Connection.notify: streamer overflow received")
		c._isValid = false
	case sessionPropertiesUpdateEvent:
		common.Odl.Debug("Connection.notify: session properties changed")
		c.handleSessionPropertyChange(e)
	default:
		common.Odl.Fine("Connection.notify: received", "evt", event)
	}
	if wasValid == true && c._isValid == false {
		c.shelf.getEventService().post(connectionInvalidatedEvent, nil)
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

// handleSessionPropertyChange applies a sessionless transaction state change
// reported through the SESSIONLESS_GTRID session property.
func (c *connection) handleSessionPropertyChange(eventData eventData) {
	updateEventData, _ := eventData.(sessionPropertiesUpdateEventData)
	newValue := updateEventData.sessionProperties().GetProperty(sessionlessGlobalTransactionIDProperty)
	sync, ok := newValue.(*sessionlessGlobalTransactionIDSync)
	if !ok || sync == nil {
		return
	}

	switch {
	case sync.isSet() && sync.isStartSuspendClient():
		// the client transaction has started on the server
		common.Odl.Fine("Client transaction started", "global transaction ID", hex.EncodeToString(sync.globalTransactionID))
		if sessionlessTx, ok := c.shelf.getTransaction().(*sessionlessTransaction); ok {
			if !slices.Equal(sessionlessTx.globalTransactionID, sync.GlobalTransactionID()) {
				sessionlessTx.globalTransactionID = sync.GlobalTransactionID()
			}
			sessionlessTx.transactionState(transactionStartedServer)
		} else {
			// This should never happen as this message indicates that the client
			// started the transaction.
			common.Odl.Debug("Got client sync message and no current sessionless transaction is registered")
		}
	case sync.isUnset() && sync.isStartSuspendClient():
		// the client transaction has ended on the server
		common.Odl.Fine("Client transaction suspended", "global transaction ID", hex.EncodeToString(sync.globalTransactionID))
		if sessionlessTx, ok := c.shelf.getTransaction().(*sessionlessTransaction); ok {
			sessionlessTx.endTransaction()
		} else {
			// This should never happen as this message indicates that the client
			// started the transaction.
			common.Odl.Debug("Got client sync message and no current sessionless transaction is registered")
		}
	case sync.isUnset() && sync.isEnded():
		// the client transaction has ended on the server
		common.Odl.Fine("Transaction ended", "global transaction ID", hex.EncodeToString(sync.globalTransactionID))
		if sessionlessTx, ok := c.shelf.getTransaction().(*sessionlessTransaction); ok {
			sessionlessTx.endTransaction()
		} else {
			common.Odl.Finest("Server initiated sessionless transaction ended on server.")
		}
	case sync.isSet() && sync.isStartSuspendServer():
		common.Odl.Finest("Server transaction started, starting implicit transaction", "global transaction ID", hex.EncodeToString(sync.globalTransactionID))
	case sync.isUnset() && sync.isStartSuspendServer():
		common.Odl.Finest("Server transaction suspended, ending implicit transaction", "global transaction ID", hex.EncodeToString(sync.globalTransactionID))
	}
}
