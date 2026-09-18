package ttc

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"io"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	extensions "github.com/oracle/go-oracledb/v26/oracle/extensions"
)

const maxSessionlessGlobalTransactionIDSize = 64
const maxSessionlessBQUALSize = 64

type sessionlessTransaction struct {
	transaction

	globalTransactionID       extensions.GlobalTransactionID // globalTransactionID is the identifier of the sessionless transaction
	timeout                   uint16                         // transaction timeout in seconds
	isStartedOnServer         bool                           // isStartedOnServer indicates that the transaction has been started on the server
	isEndedOnServer           bool                           // isEndedOnServer indicates that the transaction has ended on the server
	xid                       driverCommon.B1Array           // calculate XID using globalTransactionID and instance name
	bqualLength               driverCommon.UB4               // calculated field needed for TTC messages
	globalTransactionIDLength driverCommon.UB4               // calculated field needed for TTC messages
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
func newSessionlessTransaction(ctx context.Context, conn *connection, globalTransactionID extensions.GlobalTransactionID, timeout uint16) *sessionlessTransaction {
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
func upgradeFromTransaction(tx *transaction, globalTransactionID extensions.GlobalTransactionID, timeout uint16) *sessionlessTransaction {
	sessionlessTx := &sessionlessTransaction{
		transaction:         *tx,
		timeout:             timeout,
		globalTransactionID: append(extensions.GlobalTransactionID(nil), globalTransactionID...),
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
func generateGlobalTransactionID() (extensions.GlobalTransactionID, error) {
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
func validateSessionlessGlobalTransactionID(globalTransactionID extensions.GlobalTransactionID) error {
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

// BeginSessionlessTx starts a sessionless transaction using the provided
// standard transaction options and returns a transaction object that exposes
// sessionless lifecycle operations.
//
// Parameters:
//   - ctx: Context used for the transaction start operation.
//   - opts: Standard transaction options.
//   - timeout: Sessionless transaction timeout in seconds.
//
// Returns:
//   - extensions.SessionlessTx: Started sessionless transaction.
//   - error: Error if validation, message construction, or message queuing fails.
func (c *connection) BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (extensions.SessionlessTx, error) {
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

	// return the sessionless transaction
	return tx, nil
}

// ResumeSessionlessTx resumes the sessionless transaction identified by its
// global transaction ID.
//
// Parameters:
//   - ctx: Context used for the resume operation.
//   - globalTransactionID: Identifier of the sessionless transaction to resume.
//
// Returns:
//   - extensions.SessionlessTx: Resumed sessionless transaction.
//   - error: Error if validation, message construction, or message queuing fails.
func (c *connection) ResumeSessionlessTx(ctx context.Context, globalTransactionID extensions.GlobalTransactionID) (extensions.SessionlessTx, error) {

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
	tx := newSessionlessTransaction(ctx, c, globalTransactionID, 0)
	c.shelf.registerTransaction(tx)
	if err := c.resumeSessionlessTx(ctx, tx); err != nil {
		// unregister as current transaction
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(err)
	}

	return tx, nil
}

// Suspend detaches the current sessionless transaction from the connection. If
// no transaction is active, Suspend is a no-op. A detach failure unregisters
// the local transaction when the server no longer has it or its end
// notification has already been received.
//
// Returns:
//   - error: Error if the detach operation fails.
func (t *sessionlessTransaction) Suspend() error {

	// check that there is no active transaction in the connection
	if !t.underlyingConnection().shelf.isInTransaction() {
		// no-op
		return nil
	}

	// check that there is an active transaction
	if !t.transaction.isCurrentTransaction() {
		return t.underlyingConnection().shelf.LocalizeError(newNotInTransactionError())
	}

	// detach the transaction from the connection
	if err := t.underlyingConnection().detachTransaction(t.transactionContext()); err != nil {
		// Unregister only when the server no longer has the transaction.
		t.underlyingConnection().unregisterTransactionOnError()
		// invalidate the connection
		t.underlyingConnection()._isValid = false
		return t.underlyingConnection().shelf.LocalizeError(
			common.NewOracleError(oracleErrors.ErrorInTransaction, err, "Suspend"),
		)
	}

	// validate the current connection state
	if err := t.underlyingConnection().shelf.checkCurrentState(common.BackgroundContext); err != nil {
		t.underlyingConnection().unregisterTransactionOnError()
		return err
	}

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
//   - extensions.GlobalTransactionID: Transaction identifier while the
//     transaction is registered on the connection; otherwise nil.
func (t *sessionlessTransaction) GlobalTransactionID() extensions.GlobalTransactionID {
	if !t.transaction.isCurrentTransaction() {
		return nil
	}
	return append(extensions.GlobalTransactionID(nil), t.globalTransactionID...)
}

// setStartedOnServer records that the server has acknowledged the start of the
// sessionless transaction. If the server supplies a different global
// transaction ID, the transaction identity and XID are updated to match it.
// Invalid global transaction IDs are ignored.
//
// Parameters:
//   - globalTransactionID: Global transaction ID reported by the server.
func (t *sessionlessTransaction) setStartedOnServer(globalTransactionID extensions.GlobalTransactionID) {
	// Validate global transaction ID
	if err := validateSessionlessGlobalTransactionID(globalTransactionID); err != nil {
		common.Osl.Debug("Ignoring invalid server global transaction ID", "error", err)
		return
	}
	// Update global transaction ID and XID if server ID does not match client ID
	if !bytes.Equal(globalTransactionID, t.globalTransactionID) {
		common.Osl.Debug("Global transaction ID mismatch", "server global transaction ID", globalTransactionID, "client global transaction ID", t.globalTransactionID)
		t.globalTransactionID = append(extensions.GlobalTransactionID(nil), globalTransactionID...)
		t.buildSessionlessXID()
	}
	// Set flag
	common.Odl.Debug("Transaction has started by client received by server")
	t.isStartedOnServer = true
}

// setEndedOnServer records that the server has acknowledged the end of the
// sessionless transaction. The server does not return the transaction's global
// transaction ID in the end notification, so the transaction identity is not
// changed. Invalid global transaction IDs are ignored.
//
// Parameters:
//   - globalTransactionID: Global transaction ID reported by the server.
func (t *sessionlessTransaction) setEndedOnServer(globalTransactionID extensions.GlobalTransactionID) {
	// no validation is needed in this case, just mark the transaction as ended
	// on the server
	common.Odl.Debug("Transaction has ended by client received by server")
	t.isEndedOnServer = true
}
