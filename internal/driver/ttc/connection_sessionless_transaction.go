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
** one is included in the Software (each a "Larger Work" to which the Software
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

package ttc

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const maxSessionlessGlobalTransactionIDSize = 64
const maxSessionlessBQUALSize = 64

type sessionlessTransaction struct {
	transaction

	lifecycleMu        sync.Mutex
	contextWatcherStop func() bool

	globalTransactionID       []byte               // globalTransactionID is the identifier of the sessionless transaction
	xid                       driverCommon.B1Array // calculate XID using globalTransactionID and instance name
	bqualLength               driverCommon.UB4     // calculated field needed for TTC messages
	globalTransactionIDLength driverCommon.UB4     // calculated field needed for TTC messages
	timeout                   uint16               // transaction timeout in seconds

	isServerOriginated bool // indicates whether the transaction was started using PL/SQL
	fromSessionlessTx  bool
}

// newSessionlessTransaction creates a sessionless transaction associated with
// conn.
//
// Parameters:
//   - ctx: Context used by transaction operations.
//   - conn: Connection associated with the transaction.
//   - globalTransactionID: Identifier of the sessionless transaction.
//   - timeout: Transaction timeout in seconds.
//
// Returns:
//   - *sessionlessTransaction: Initialized sessionless transaction.
func newSessionlessTransaction(ctx context.Context, conn *connection, globalTransactionID []byte, timeout uint16) *sessionlessTransaction {
	tx := newTransaction(conn, ctx)
	return upgradeFromTransaction(tx, globalTransactionID, timeout)
}

// upgradeFromTransaction converts a regular transaction into a sessionless
// transaction while preserving its connection and context.
//
// Parameters:
//   - tx: Existing transaction to upgrade.
//   - globalTransactionID: Identifier of the sessionless transaction.
//   - timeout: Transaction timeout in seconds.
//
// Returns:
//   - *sessionlessTransaction: Upgraded sessionless transaction.
func upgradeFromTransaction(tx *transaction, globalTransactionID []byte, timeout uint16) *sessionlessTransaction {
	sessionlessTx := &sessionlessTransaction{
		transaction:         *tx,
		timeout:             timeout,
		globalTransactionID: append([]byte(nil), globalTransactionID...),
	}
	// keep the transaction identity on upgrade, this allows to identify a
	// transaction that has been upgraded to sessionless after a sessionless
	// transaction was started using PL/SQL.
	sessionlessTx._transactionIdentity = tx.transactionIdentity()
	sessionlessTx.buildSessionlessXID()
	return sessionlessTx
}

// generateGlobalTransactionID creates a default global transaction ID using 16
// random bytes encoded with UUID version and variant bits. The returned
// identifier stores the raw bytes directly so it can be passed unchanged to TTC
// payloads.
func generateGlobalTransactionID() ([]byte, error) {
	var globalTransactionID [16]byte
	if _, err := io.ReadFull(rand.Reader, globalTransactionID[:]); err != nil {
		return nil, err
	}

	globalTransactionID[6] = (globalTransactionID[6] & 0x0F) | 0x40
	globalTransactionID[8] = (globalTransactionID[8] & 0x3F) | 0x80

	return globalTransactionID[:], nil
}

// validateSessionlessGlobalTransactionID validates a caller-provided global transaction ID.
//
// Parameters:
//   - globalTransactionID: Global transaction ID to validate.
//
// Returns:
//   - error: InvalidGlobalTransactionIDValue when globalTransactionID is empty
//     or exceeds the server limit; otherwise nil.
func validateSessionlessGlobalTransactionID(globalTransactionID []byte) error {
	size := len(globalTransactionID)
	if size == 0 {
		return common.NewOracleError(oracleErrors.InvalidGlobalTransactionIDValue, nil)
	}
	if size > maxSessionlessGlobalTransactionIDSize {
		return common.NewOracleError(oracleErrors.InvalidGlobalTransactionIDValue, nil)
	}
	return nil
}

// buildSessionlessXID builds the XID from the global transaction ID and the
// connection instance name. Value and lengths are stored in the transaction
// object and used on TTI messages.
func (t *sessionlessTransaction) buildSessionlessXID() {
	globalTransactionIDBytes := []byte(t.globalTransactionID)
	globalTransactionIDLength := len(globalTransactionIDBytes)
	if globalTransactionIDLength > maxSessionlessGlobalTransactionIDSize {
		globalTransactionIDLength = maxSessionlessGlobalTransactionIDSize
	}

	var bqualBytes []byte
	c := t.underlyingConnection()
	if c != nil && c.sessCtx != nil {
		if instance, err := c.sessCtx.GetSessionProperties().GetTrimmedString(instanceName); err == nil {
			bqualBytes = []byte(instance)
		}
	}
	bqualLength := len(bqualBytes)
	if bqualLength > maxSessionlessBQUALSize {
		bqualLength = maxSessionlessBQUALSize
	}

	xid := make(driverCommon.B1Array, maxSessionlessGlobalTransactionIDSize+maxSessionlessBQUALSize)
	copy(xid, globalTransactionIDBytes[:globalTransactionIDLength])
	copy(xid[globalTransactionIDLength:], bqualBytes[:bqualLength])

	t.xid = xid
	t.bqualLength = driverCommon.UB4(bqualLength)
	t.globalTransactionIDLength = driverCommon.UB4(globalTransactionIDLength)
}

// startContextWatcher starts the cancellation watcher for an attached public
// sessionless transaction. A watcher is deliberately not installed by the
// constructor because server-created implicit transactions use a background
// context and are not owned by this API lifecycle.
func (t *sessionlessTransaction) startContextWatcher() {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()
	t.startContextWatcherLocked()
}

// startContextWatcherLocked installs the cancellation callback while the
// lifecycle mutex is held.
func (t *sessionlessTransaction) startContextWatcherLocked() {
	if !t.transactionState.isStarted() || t.contextWatcherStop != nil {
		return
	}
	ctx := t.transactionContext()
	if ctx == nil || ctx.Done() == nil {
		return
	}
	t.contextWatcherStop = context.AfterFunc(ctx, t.rollbackOnContextCancellation)
}

// stopContextWatcherLocked prevents a pending cancellation callback from
// starting. If the callback has already started, it waits for lifecycleMu and
// rechecks the transaction state before doing any cleanup.
func (t *sessionlessTransaction) stopContextWatcherLocked() {
	if stop := t.contextWatcherStop; stop != nil {
		t.contextWatcherStop = nil
		stop()
	}
}

// rollbackOnContextCancellation rolls back an attached sessionless
// transaction after its owning context is canceled. The rollback uses a
// bounded background context because the transaction context is already
// canceled.
func (t *sessionlessTransaction) rollbackOnContextCancellation() {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	if !t.transactionState.isStarted() || !t.transaction.isCurrentTransaction() {
		t.contextWatcherStop = nil
		return
	}

	t.contextWatcherStop = nil
	err := t.rollbackWithCleanupContextLocked()

	if err != nil {
		// The rollback result is unknown. Do not allow this physical connection
		// to return to the pool with a potentially active transaction.
		t.underlyingConnection()._isValid = false
		common.Odl.Warn("Rollback of sessionless transaction after context cancellation failed", "error", err)
	}
}

// rollbackWithCleanupContextLocked rolls back the current transaction with a
// bounded background context. The caller must hold lifecycleMu.
func (t *sessionlessTransaction) rollbackWithCleanupContextLocked() error {
	rollbackContext, cancel := context.WithTimeout(common.BackgroundContext, _rollbackTimeout)
	defer cancel()
	previousState := t.transactionState
	err := t.transaction.rollback(rollbackContext)
	// Context cleanup is an internal rollback. Restore the local state unless a
	// server end notification superseded it, so the public wrapper can continue
	// using the connection when the local cleanup result is not terminal.
	t.restoreTransactionState(previousState)
	return err
}

// finishTransactionOperationLocked records the result of a terminal operation.
// On an operation error, retain the watcher only if the transaction is still
// current and the connection remains usable; a canceled context will invoke
// the watcher immediately and complete cleanup.
func (t *sessionlessTransaction) finishTransactionOperationLocked(err error) {
	if err != nil && t.transaction.isCurrentTransaction() && t.underlyingConnection()._isValid {
		t.startContextWatcherLocked()
	}
}

// BeginSessionlessTx starts a sessionless transaction using the provided
// standard transaction options and returns a transaction object that exposes
// sessionless lifecycle operations.
//
// Parameters:
//   - ctx: Context used for the transaction start operation.
//   - opts: Standard transaction options.
//   - timeout:  the time for how long (in seconds) after the suspension of
//     this Sessionless transaction should the server roll back this
//     transaction. This is an attempt to avoid a transaction from holding
//     on to database resources (such as row locks) indefinitely.

// Returns:
//   - extensions.SessionlessTx: Started sessionless transaction.
//   - error: Error if validation, message construction, or message queuing fails.
func (c *connection) BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (common.SessionlessTransaction, error) {
	common.Odl.Debug("Starting sessionless transaction")

	// check that there is no active transaction in the connection
	if c.shelf.isInTransaction() {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.AlreadyInTransaction, nil, nil))
	}

	// check that the server supports sessionless transactions
	capabilities := c.shelf.GetCapabilities()
	if cap, ok := capabilities[kpccapCtbTtc5SessionlessTxn]; !ok || !cap.IsSet {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.UnsupportedFeature, nil, "Sessionless Transactions"))
	}

	// convert to driver.TxOptions since this is what is received by BeginTx
	driverOpts := driver.TxOptions{
		Isolation: driver.IsolationLevel(opts.Isolation),
		ReadOnly:  opts.ReadOnly,
	}

	// check that the isolation level is supported
	if !isSupportedIsolationLevel(driverOpts) {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.IsolationLevelNotSupported, nil, nil))
	}

	// generate a global transaction id
	globalTransactionID, err := generateGlobalTransactionID()
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}

	// create and register the sessionless transaction object
	tx := newSessionlessTransaction(ctx, c, globalTransactionID, timeout)
	c.shelf.registerTransaction(tx)
	if err := c.beginTransaction(ctx, tx, driverOpts); err != nil {
		// unregister as current transaction
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.StartResumeTransactionFailure, err, nil))
	}
	tx.startContextWatcher()

	// return the sessionless transaction
	return tx, nil
}

// ResumeSessionlessTx resumes the sessionless transaction identified by its
// global transaction ID.
//
// Parameters:
//   - ctx: Context used for the resume operation.
//   - globalTransactionID: Identifier of the sessionless transaction to resume.
//   - timeout: the time for how long (in seconds) the server attempts to
//     resume the transaction. If multiple sessions connect to the same database
//     instance and request to resume the same Sessionless transaction, only one
//     can successfully resume at any given time
//
// Returns:
//   - extensions.SessionlessTx: Resumed sessionless transaction.
//   - error: Error if validation, message construction, or message queuing fails.
func (c *connection) ResumeSessionlessTx(ctx context.Context, globalTransactionID []byte, timeout uint16) (common.SessionlessTransaction, error) {

	// check that the global transaction ID is valid
	if err := validateSessionlessGlobalTransactionID(globalTransactionID); err != nil {
		return nil, c.shelf.LocalizeError(err)
	}

	// check that there is no active transaction in the connection
	if c.shelf.isInTransaction() {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.AlreadyInTransaction, nil, nil))
	}

	// check that the server supports sessionless transactions
	capabilities := c.shelf.GetCapabilities()
	if cap, ok := capabilities[kpccapCtbTtc5SessionlessTxn]; !ok || !cap.IsSet {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.UnsupportedFeature, nil, "Sessionless Transactions"))
	}

	// create and register the sessionless transaction object
	tx := newSessionlessTransaction(ctx, c, globalTransactionID, timeout)
	c.shelf.registerTransaction(tx)
	if err := c.resumeSessionlessTx(ctx, tx); err != nil {
		// unregister as current transaction
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(err)
	}
	tx.startContextWatcher()

	return tx, nil
}

// Commit commits the sessionless transaction and coordinates with the context
// cancellation watcher so that exactly one terminal operation is in progress.
func (t *sessionlessTransaction) Commit() error {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	if err := t.transaction.checkCurrentTransaction(); err != nil {
		return t.underlyingConnection().shelf.LocalizeError(err)
	}
	previousState := t.transactionState
	t.transactionState = transactionEndedClient
	t.stopContextWatcherLocked()
	err := t.transaction.Commit()
	if err != nil {
		t.restoreTransactionState(previousState)
	}
	t.finishTransactionOperationLocked(err)
	return err
}

// Rollback rolls back the sessionless transaction and coordinates with the
// context cancellation watcher so that exactly one terminal operation is in
// progress.
func (t *sessionlessTransaction) Rollback() error {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	if err := t.transaction.checkCurrentTransaction(); err != nil {
		return t.underlyingConnection().shelf.LocalizeError(err)
	}

	previousState := t.transactionState
	t.transactionState = transactionEndedClient
	t.stopContextWatcherLocked()
	err := t.transaction.Rollback()
	if err != nil {
		t.restoreTransactionState(previousState)
	}
	t.finishTransactionOperationLocked(err)
	return err
}

// Suspend detaches the current sessionless transaction from the connection.
// A successful suspend ends the transaction on this session while leaving it
// resumable by another sessionless transaction object. A detach failure
// unregisters the local transaction when the server no longer has it or its
// end notification has already been received.
//
// Returns:
//   - error: Error if the detach operation fails.
func (t *sessionlessTransaction) Suspend() error {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()

	if !t.underlyingConnection().shelf.isInTransaction() {
		// no-op
		t.stopContextWatcherLocked()
		return nil
	}

	// check that there is an active transaction
	if err := t.transaction.checkCurrentTransaction(); err != nil {
		return t.underlyingConnection().shelf.LocalizeError(err)
	}
	previousState := t.transactionState
	t.transactionState = transactionEndedClient
	t.stopContextWatcherLocked()
	// detach the transaction from the connection
	if err := t.underlyingConnection().detachTransaction(t.transactionContext()); err != nil {
		t.restoreTransactionState(previousState)
		// Unregister only when the server no longer has the transaction.
		t.underlyingConnection().unregisterTransactionOnError()
		// If cancellation interrupted detach, make a bounded rollback attempt
		// before invalidating the connection. The watcher is stopped while the
		// operation is in progress, so cleanup must be performed here.
		if t.transactionContext() != nil && t.transactionContext().Err() != nil && t.transaction.isCurrentTransaction() {
			if rollbackErr := t.rollbackWithCleanupContextLocked(); rollbackErr != nil {
				common.Odl.Warn("Rollback after canceled sessionless suspend failed", "error", rollbackErr)
			}
		}
		// invalidate the connection
		t.underlyingConnection()._isValid = false
		return t.underlyingConnection().shelf.LocalizeError(
			common.NewOracleError(oracleErrors.ErrorInTransaction, err, "Suspend"),
		)
	}

	// validate the current connection state
	if err := t.underlyingConnection().shelf.checkCurrentState(common.BackgroundContext); err != nil {
		t.restoreTransactionState(previousState)
		t.underlyingConnection().unregisterTransactionOnError()
		t.finishTransactionOperationLocked(err)
		return err
	}

	// Detach ends the transaction on this session. It remains resumable by a
	// different sessionless transaction object.
	t.transactionState = transactionEndedServer
	// unregister the transaction
	t.underlyingConnection().shelf.unregisterTransaction()
	return nil
}

// resumeSessionlessTx queues the OTXSE resume operation for tx.
//
// Parameters:
//   - ctx: Context used for the resume operation.
//   - tx: Sessionless transaction to resume.
//
// Returns:
//   - error: Error if the message cannot be created or queued.
func (c *connection) resumeSessionlessTx(ctx context.Context, tx *sessionlessTransaction) error {
	common.Odl.Debug("Running sessionless transaction resume")
	// get the streamer
	stmr, ok := c.shelf.GetMessageStreamer().(MessageStreamerInterface)
	if !ok {
		common.Odl.Warn("Sessionless transactions require a message streamer with callback support")
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// create the message
	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIPFN, oTxSe)
	if err != nil {
		common.Odl.Warn("Error creating OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}

	otxse, ok := msg.(*tTIOtxse)
	if !ok {
		common.Odl.Warn("Unexpected message type for OTXSE", "message", msg)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// configure the message
	otxse.configureForResume(tx)

	// push the message, this is a piggyback, it will be flushed on the next
	// round trip
	err = stmr.Push(ctx, msg)
	if err != nil {
		common.Odl.Warn("Error pushing OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.StartResumeTransactionFailure, err)
	}

	return nil
}

// detachTransaction executes an OTXSE request and waits for the
// terminal TTIOER or TTISTA response.
//
// Parameters:
//   - ctx: Context used for the detach operation.
//
// Returns:
//   - error: Error if the message cannot be sent or the server returns an error.
func (c *connection) detachTransaction(ctx context.Context) error {
	common.Odl.Debug("Running sessionless transaction detach")

	// get the streamer
	stmr, ok := c.shelf.GetMessageStreamer().(MessageStreamerInterface)
	if !ok {
		common.Odl.Warn("Sessionless transactions require a message streamer with callback support")
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// create the message
	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIFUN, oTxSe)
	if err != nil {
		common.Odl.Warn("Error creating OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}

	otxse, ok := msg.(*tTIOtxse)
	if !ok {
		common.Odl.Warn("Unexpected message type for OTXSE", "message", msg)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// configure the message
	otxse.configureForSuspend()

	// push the message
	err = c.shelf.GetMessageStreamer().Push(ctx, msg)
	if err != nil {
		common.Odl.Warn("Error pushing OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}

	// flush
	err = stmr.Flush(ctx)
	if err != nil {
		common.Odl.Warn("Error flushing OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}

	// register OTXSE RPA
	stmr.RegisterPreUnmarshallCallback(TTIRPA, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return c.shelf.GetMessageFactory().GetMessageForFunction(TTIRPA, oTxSe)
	})
	defer stmr.UnRegisterPreUnmarshallCallback(TTIRPA)

	for {
		retMsg, err := stmr.Pull(ctx, TTIRPA, TTIOER, TTISTA)
		if err != nil {
			common.Odl.Warn("Error pulling OTXSE response", "error", err)
			return common.NewOracleError(oracleErrors.StreamerReadError, err)
		}
		switch retMsg.GetMsgCode() {
		case TTIRPA:
			// OTXSE returns context/application return values in TTIRPA, but the
			// transaction operation completes only when terminal status follows.
			continue
		case TTIOER:
			err = retMsg.(tTIOerIface).getError()
			if err != nil {
				return err
			}
			return nil
		case TTISTA:
			return nil
		}
	}
}

// GlobalTransactionID returns the identifier associated with this sessionless
// transaction.
//
// Returns:
//   - []byte: Transaction identifier while the
//     transaction is registered on the connection; otherwise nil.
func (t *sessionlessTransaction) GlobalTransactionID() []byte {
	if !t.transaction.isCurrentTransaction() {
		return nil
	}
	return append([]byte(nil), t.globalTransactionID...)
}

// setStartedOnServer records that the server has acknowledged the start of the
// sessionless transaction. If the server supplies a different global
// transaction ID, the transaction identity and XID are updated to match it.
// Invalid global transaction IDs are ignored.
//
// Parameters:
//   - globalTransactionID: Global transaction ID reported by the server.
func (t *sessionlessTransaction) setStartedOnServer(globalTransactionID []byte) {
	// Validate global transaction ID
	if err := validateSessionlessGlobalTransactionID(globalTransactionID); err != nil {
		common.Osl.Debug("Ignoring invalid server global transaction ID", "error", err)
		return
	}
	// Update global transaction ID and XID if server ID does not match client ID
	if !bytes.Equal(globalTransactionID, t.globalTransactionID) {
		common.Osl.Debug("Global transaction ID mismatch", "server global transaction ID", globalTransactionID, "client global transaction ID", t.globalTransactionID)
		t.globalTransactionID = append([]byte(nil), globalTransactionID...)
		t.buildSessionlessXID()
	}
	// Set state
	common.Odl.Debug("Transaction has started by client received by server")
	t.transactionState = transactionStartedServer
}

// setEndedOnServer records that the server has acknowledged the end of the
// sessionless transaction. The server does not return the transaction's global
// transaction ID in the end notification, so the transaction identity is not
// changed. Invalid global transaction IDs are ignored.
//
// Parameters:
//   - globalTransactionID: Global transaction ID reported by the server.
func (t *sessionlessTransaction) setEndedOnServer(globalTransactionID []byte) {
	// no validation is needed in this case, just mark the transaction as ended
	// on the server
	common.Odl.Debug("Transaction has ended by client received by server")
	t.transactionState = transactionEndedServer
}

// SetRunningFromSessionlessTx records whether the current operation was
// authorized by the public sessionless transaction wrapper.
func (t *sessionlessTransaction) SetRunningFromSessionlessTx(fromSessionlessTx bool) {
	t.fromSessionlessTx = fromSessionlessTx
}

// IsTransactionEnded reports whether the public sessionless transaction handle
// is in either terminal transaction state.
func (t *sessionlessTransaction) IsTransactionEnded() bool {
	t.lifecycleMu.Lock()
	defer t.lifecycleMu.Unlock()
	return t.transactionState == transactionEndedClient || t.transactionState == transactionEndedServer
}
