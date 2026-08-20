package ttc

import (
	"context"
	"crypto/rand"
	"database/sql/driver"
	"io"
	"math"
	"time"

	"github.com/oracle/go-driver/driver/common"
)

const maxSessionlessGTRIDSize = 64

// ConnBeginSessionlessTx is implemented by connections that support Oracle
// sessionless transaction lifecycle operations in addition to standard BeginTx.
type ConnBeginSessionlessTx interface {
	// BeginSessionlessTx starts a new sessionless transaction using the provided
	// standard transaction options.
	BeginSessionlessTx(ctx context.Context, opts driver.TxOptions) (SessionlessTx, error)
	// ResumeSessionlessTx resumes the sessionless transaction identified by gtrid
	// using the provided standard transaction options.
	ResumeSessionlessTx(ctx context.Context, opts driver.TxOptions, gtrid string) (SessionlessTx, error)
}

// SessionlessTx extends driver.Tx with Oracle-specific suspend and transaction
// identity operations for sessionless transactions.
type SessionlessTx interface {
	driver.Tx
	// IsSessionlessTx reports whether this transaction has an associated GTRID.
	IsSessionlessTx() bool
	// Suspend detaches the current sessionless transaction from the connection.
	Suspend() error
	// GlobalTransactionID returns the identifier associated with this
	// sessionless transaction.
	GlobalTransactionID() string
}

// generateSessionlessGTRID creates a JDBC-compatible default GTRID using 16
// random bytes encoded with UUID version and variant bits. The returned string
// stores the raw bytes directly so it can be passed unchanged to TTC payloads.
func generateSessionlessGTRID() (string, error) {
	var gtrid [16]byte
	if _, err := io.ReadFull(rand.Reader, gtrid[:]); err != nil {
		return "", err
	}

	// Match UUID.randomUUID() layout used by the JDBC thin driver.
	gtrid[6] = (gtrid[6] & 0x0F) | 0x40
	gtrid[8] = (gtrid[8] & 0x3F) | 0x80

	return string(gtrid[:]), nil
}

func validateSessionlessGTRID(gtrid string) error {
	size := len([]byte(gtrid))
	if size == 0 {
		return common.NewOracleError(common.InvalidGTRIDValue, nil)
	}
	if size > maxSessionlessGTRIDSize {
		return common.NewOracleError(common.InvalidGTRIDValue, nil)
	}
	return nil
}

func sessionlessTxTimeoutFromContext(ctx context.Context) common.UB2 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}

	seconds := math.Ceil(remaining.Seconds())
	if seconds > float64(math.MaxUint16) {
		return math.MaxUint16
	}

	return common.UB2(seconds)
}

// runSessionlessTransaction executes an OTXSE request and waits for the
// terminal TTIOER or TTISTA response.
func (c *Connection) runSessionlessTransaction(ctx context.Context, operation common.SB4, flags common.UB4, gtrid string, timeout common.UB2) error {
	common.Odl.Debug("Running sessionless transaction operation", "operation", operation)

	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIFUN, oTxSe)
	if err != nil {
		common.Odl.Warn("Error creating OTXSE message", "error", err)
		return common.NewOracleError(common.InternalError, err)
	}

	otxse, ok := msg.(*tTIOtxse)
	if !ok {
		common.Odl.Warn("Unexpected message type for OTXSE", "message", msg)
		return common.NewOracleError(common.InternalError, nil)
	}

	otxse.setOperation(operation)
	otxse.setTimeout(timeout)
	otxse.setFlags(flags)
	otxse.setFormatID(k2gSessionless)
	if gtrid != "" {
		xid := common.B1Array([]byte(gtrid))
		otxse.setXID(xid, common.UB4(len(xid)), 0)
	}

	err = c.shelf.GetMessageStreamer().Push(ctx, msg)
	if err != nil {
		common.Odl.Warn("Error pushing OTXSE message", "error", err)
		return common.NewOracleError(common.StreamerWriteError, err)
	}
	err = c.shelf.GetMessageStreamer().Flush(ctx)
	if err != nil {
		common.Odl.Warn("Error flushing OTXSE message", "error", err)
		return common.NewOracleError(common.StreamerWriteError, err)
	}

	retMsg, err := c.shelf.GetMessageStreamer().Pull(ctx, TTIOER, TTISTA)
	if err != nil {
		common.Odl.Warn("Error pulling OTXSE response", "error", err)
		return common.NewOracleError(common.StreamerReadError, err)
	}
	switch retMsg.GetMsgCode() {
	case TTIOER:
		err = retMsg.(tTIOerIface).getError()
		if err != nil {
			return err
		}
	case TTISTA:
	}
	return nil
}

// BeginSessionlessTx starts a sessionless transaction using the provided
// standard transaction options and returns a transaction object that exposes
// sessionless lifecycle operations.
func (c *Connection) BeginSessionlessTx(ctx context.Context, opts driver.TxOptions) (SessionlessTx, error) {
	tx, err := c.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	oracleTx, ok := tx.(*transaction)
	if !ok {
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(common.NewOracleError(common.InternalError, nil))
	}
	gtrid, err := generateSessionlessGTRID()
	if err != nil {
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(common.NewOracleError(common.InternalError, err))
	}
	oracleTx.GTRID = gtrid

	err = c.runSessionlessTransaction(ctx, otxseStart, otxseTransSessionless|otxseTransNew, oracleTx.GTRID, sessionlessTxTimeoutFromContext(ctx))
	if err != nil {
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(common.NewOracleError(common.ConfigureTransactionError, err, nil))
	}

	return oracleTx, nil
}

// ResumeSessionlessTx resumes the sessionless transaction identified by gtrid
// using the provided standard transaction options and returns a transaction
// object that exposes sessionless lifecycle operations.
func (c *Connection) ResumeSessionlessTx(ctx context.Context, opts driver.TxOptions, gtrid string) (SessionlessTx, error) {
	if err := validateSessionlessGTRID(gtrid); err != nil {
		return nil, c.shelf.LocalizeError(err)
	}
	tx, err := c.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	oracleTx, ok := tx.(*transaction)
	if !ok {
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(common.NewOracleError(common.InternalError, nil))
	}
	oracleTx.GTRID = gtrid

	err = c.runSessionlessTransaction(ctx, otxseStart, otxseTransSessionless|otxseTransResume, gtrid, sessionlessTxTimeoutFromContext(ctx))
	if err != nil {
		c.shelf.unregisterTransaction()
		return nil, c.shelf.LocalizeError(common.NewOracleError(common.ConfigureTransactionError, err, nil))
	}

	return oracleTx, nil
}

// Suspend detaches the current sessionless transaction from the connection.
func (t *transaction) Suspend() error {
	if !t.IsSessionlessTx() {
		return t._underlyingConnection.shelf.LocalizeError(
			common.NewOracleError(common.NotInTransaction, nil, nil),
		)
	}
	err := t._underlyingConnection.runSessionlessTransaction(
		common.BackgroundContext,
		otxseDetach,
		otxseTransSessionless,
		"",
		0,
	)
	if err != nil {
		return t._underlyingConnection.shelf.LocalizeError(
			common.NewOracleError(common.ErrorInTransaction, err, "Suspend"),
		)
	}
	t.GTRID = ""
	t._underlyingConnection.shelf.unregisterTransaction()
	return nil
}

// GlobalTransactionID returns the identifier associated with this sessionless
// transaction.
func (t *transaction) GlobalTransactionID() string {
	return t.GTRID
}
