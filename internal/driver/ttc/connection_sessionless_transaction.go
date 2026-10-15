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
	"crypto/rand"
	"database/sql"
	"io"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const maxSessionlessGlobalTransactionIDSize = 64
const maxSessionlessBQUALSize = 64

// TransactionStartOperation identifies the sessionless start or resume request
// prepared for the following BeginTx call.
type TransactionStartOperation uint8

const (
	// SessionlessStart marks a prepared sessionless start.
	SessionlessStart TransactionStartOperation = iota
	// SessionlessResume marks a prepared sessionless resume.
	SessionlessResume
)

// sessionlessTransaction holds the sessionless transaction identifier, XID,
// timeout, and the operation state used to start or resume it.
type sessionlessTransaction struct {
	// transaction embeds shared transaction state and connection identity.
	transaction
	// globalTransactionID is the identifier assigned to this transaction.
	globalTransactionID []byte
	// Start: suspended lifetime; resume: wait for the transaction to become available.
	timeout uint16
	// startOperation indicates whether BeginTx starts or resumes this transaction.
	startOperation TransactionStartOperation
	// endOperationMutex prevents suspend and rollback from running simultaneously
	// when the transaction is rolled back due to context cancellation
	endOperationMutex sync.Mutex
}

// newSessionlessTransaction creates a sessionless transaction associated with
// conn.
//
// Parameters:
//   - ctx: Context used by transaction operations.
//   - conn: Connection associated with the transaction.
//   - globalTransactionID: Identifier of the sessionless transaction.
//   - timeout: Timeout in seconds supplied for the start or resume operation.
//
// Returns:
//   - *sessionlessTransaction: Initialized sessionless transaction.
func newSessionlessTransaction(ctx context.Context, conn *connection, globalTransactionID []byte, timeout uint16) *sessionlessTransaction {
	sessionlessTx := &sessionlessTransaction{
		transaction: transaction{
			_underlyingConnection: conn,
			_transactionContext:   ctx,
			_transactionState:     transactionPrepared,
		},
		timeout:             timeout,
		globalTransactionID: append([]byte(nil), globalTransactionID...),
	}
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
// object and used in OTXSE messages.
func (t *sessionlessTransaction) buildSessionlessXID() (driverCommon.B1Array, driverCommon.UB4, driverCommon.UB4) {
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

	return xid, driverCommon.UB4(globalTransactionIDLength), driverCommon.UB4(bqualLength)
}

// PrepareForSessionlessStartOperation validates the connection and registers a
// prepared sessionless transaction for the following database/sql BeginTx.
func (c *connection) PrepareForSessionlessStartOperation(ctx context.Context, timeout uint16) (common.SessionlessTransaction, error) {
	// check that there is no active transaction in the connection
	if c.shelf.isInTransaction() {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.AlreadyInTransaction, nil, nil))
	}

	// check that the server supports sessionless transactions
	capabilities := c.shelf.GetCapabilities()
	if cap, ok := capabilities[kpccapCtbTtc5SessionlessTxn]; !ok || !cap.IsSet {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.UnsupportedFeature, nil, "Sessionless Transactions"))
	}

	// generate a global transaction id
	globalTransactionID, err := generateGlobalTransactionID()
	if err != nil {
		return nil, c.shelf.LocalizeError(err)
	}

	common.Odl.With("GTRID", globalTransactionID).Fine("Prepare start sessionless transaction")
	// create and register the sessionless transaction object
	tx := newSessionlessTransaction(ctx, c, globalTransactionID, timeout)
	tx.startOperation = SessionlessStart
	c.shelf.registerTransaction(tx)
	return tx, nil
}

// ClearPrepareForSessionlessStartOperation removes a prepared start after
// database/sql fails to begin the transaction.
func (c *connection) ClearPrepareForSessionlessStartOperation() {
	currentTx, currentOk := c.shelf.getTransaction().(*sessionlessTransaction)
	if currentOk {
		common.Odl.With("GTRID", currentTx.globalTransactionID).Fine("An error occurred, unregister transaciton")
		c.shelf.unregisterTransaction()
	}
}

// PrepareForSessionlessResumeOperation validates the identifier and registers
// a prepared resume for the following database/sql BeginTx.
func (c *connection) PrepareForSessionlessResumeOperation(ctx context.Context, globalTransactionID []byte, timeout uint16) (common.SessionlessTransaction, error) {
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

	common.Odl.With("GTRID", globalTransactionID).Fine("Prepare resume sessionless transaction")
	// create and register the sessionless transaction object
	tx := newSessionlessTransaction(ctx, c, globalTransactionID, timeout)
	tx.startOperation = SessionlessResume
	c.shelf.registerTransaction(tx)
	return tx, nil
}

// ClearPrepareForSessionlessResumeOperation removes the prepared resume if
// BeginTx failed and it is still the current transaction. On success, the
// started transaction remains registered.
func (c *connection) ClearPrepareForSessionlessResumeOperation(tx common.SessionlessTransaction, err error) {
	if err != nil {
		// if this transaction is the current one, remove it
		currentTx, currentOk := c.shelf.getTransaction().(*sessionlessTransaction)
		thisTx, thisOk := tx.(*sessionlessTransaction)
		if currentOk && thisOk && thisTx == currentTx {
			common.Odl.With("GTRID", thisTx.globalTransactionID).Fine("An error occurred, unregister transaciton")
			c.shelf.unregisterTransaction()
			return
		}
	}
}

// Commit sends the commit operation for this sessionless transaction. It
// returns sql.ErrTxDone when the server has already reported that the
// transaction ended.
func (t *sessionlessTransaction) Commit() error {
	if t._transactionState == transactionEndedServer {
		return sql.ErrTxDone
	}
	return commitTransaction(t)
}

// Rollback sends the rollback operation for this sessionless transaction with
// a bounded background context. It returns sql.ErrTxDone when the server has
// already reported that the transaction ended.
func (t *sessionlessTransaction) Rollback() error {
	common.Odl.Fine("Transaction rollback")
	t.endOperationMutex.Lock()
	defer t.endOperationMutex.Unlock()
	// transaction already ended
	if t._transactionState == transactionEndedServer {
		return sql.ErrTxDone
	}
	// Rollback should be allowed to complete even if the transaction context
	// has ended, use another context to execute the rollback operation.
	rollbackCtx, cancel := context.WithTimeout(common.BackgroundContext, _rollbackTimeout)
	defer cancel()
	return endTransaction(t, rollbackCtx, t.rollback)
}

// Suspend detaches the sessionless transaction from this connection using the
// transaction context. After a successful detach, the transaction is
// unregistered locally; its identifier should be saved before suspension. It
// returns sql.ErrTxDone when the server has already reported that it ended.
func (t *sessionlessTransaction) Suspend() error {
	common.Odl.With("GTRID", t.globalTransactionID).Fine("Transaction suspend")
	t.endOperationMutex.Lock()
	defer t.endOperationMutex.Unlock()
	if t._transactionState == transactionEndedServer {
		return sql.ErrTxDone
	}
	return endTransaction(t, t._transactionContext, t.detachTransaction)
}

// endTransaction clears the transaction identifier, marks the transaction as
// ended on the server, and unregisters it from the connection. This method
// should be called when the server reports that the transaction has ended.
func (t *sessionlessTransaction) endTransaction() {
	t.globalTransactionID = nil
	t.transaction.endTransaction()
}

// detachTransaction executes an OTXSE request and waits for the
// terminal TTIOER or TTISTA response.
//
// Because of a bug, detachTransaction send OTXSE as a piggyback
// operation of a ROLLBACK. This is not the expected behavior, but
// a workaround.
//
// Parameters:
//   - ctx: Context used for the detach operation.
//
// Returns:
//   - error: Error if the message cannot be sent or the server returns an error.
func (t *transaction) detachTransaction(ctx context.Context) error {
	// get the streamer
	stmr, ok := t.underlyingConnection().shelf.GetMessageStreamer().(MessageStreamerInterface)
	if !ok {
		common.Odl.Warn("Sessionless transactions require a message streamer with callback support")
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// create the message
	msg, err := t.underlyingConnection().shelf.GetMessageFactory().GetMessageForFunction(TTIPFN, oTxSe)
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
	err = stmr.Push(ctx, msg)
	if err != nil {
		common.Odl.Warn("Error pushing OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}
	// This is a workaround for a bug that causes suspend not to reset the state
	// Suspend is now sent as a piggyback message followed by a rollback that
	// will reset the state. This should be removed and suspend should be sent
	// as a TTIFUN as soon as the bug is fixed.
	return t.rollback(t.transactionContext())
}

// GlobalTransactionID returns a copy of the identifier currently stored on
// this transaction handle. It returns nil after endTransaction clears it.
//
// Returns:
//   - []byte: Copy of the current identifier, or nil if it has been cleared.
func (t *sessionlessTransaction) GlobalTransactionID() []byte {
	return append([]byte(nil), t.globalTransactionID...)
}
