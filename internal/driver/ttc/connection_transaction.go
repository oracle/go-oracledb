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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Begin starts and returns a new transaction with read-committed isolation.
//
// Returns:
//   - driver.Tx: Started transaction.
//   - error: Error if the transaction cannot be started.
func (c *connection) Begin() (driver.Tx, error) {
	ctx := common.BackgroundContext
	opts := driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
		ReadOnly:  false,
	}
	return c.BeginTx(ctx, opts)
}

// BeginTx starts and returns a new transaction.
//
// Parameters:
//   - ctx: Context used for the transaction start operation.
//   - opts: Transaction isolation and read-only options.
//
// Returns:
//   - driver.Tx: Started transaction.
//   - error: Error if the transaction cannot be started.
func (c *connection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	common.Odl.Debug("Starting transaction")

	if c.shelf.isInTransaction() {
		return nil, c.shelf.LocalizeError(common.NewOracleError(errors.AlreadyInTransaction, nil, nil))
	}

	if !isSupportedIsolationLevel(opts) {
		return nil, common.NewOracleError(errors.IsolationLevelNotSupported, nil, nil)
	}

	tx := newTransaction(c, ctx)
	c.shelf.registerTransaction(tx)

	err := c.beginTransaction(ctx, tx, opts)
	if err != nil {
		c.shelf.unregisterTransaction()
		return nil, err
	}

	return tx, nil
}

// isSupportedIsolationLevel reports whether opts uses an isolation level
// supported by the driver.
//
// Parameters:
//   - opts: Transaction options to validate.
//
// Returns:
//   - bool: True when the isolation level is supported.
func isSupportedIsolationLevel(opts driver.TxOptions) bool {
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault, sql.LevelReadCommitted, sql.LevelSerializable:
		return true
	default:
		return false
	}
}

// beginTransaction queues an OTXSE transaction start operation.
//
// Parameters:
//   - ctx: Context used for queuing the operation.
//   - transaction: Transaction to start.
//   - opts: Transaction isolation and read-only options.
//
// Returns:
//   - error: Error if the OTXSE message cannot be created or queued.
func (c *connection) beginTransaction(ctx context.Context, transaction oracleTx, opts driver.TxOptions) error {
	// get streamer
	stmr, ok := c.shelf.GetMessageStreamer().(MessageStreamerInterface)
	if !ok {
		common.Odl.Warn("beginTransaction requires a message streamer with callback support")
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// create message
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

	// configure message for operation and transaction options
	otxse.configureForStart(transaction, opts)

	// push message, this is a piggyback message, it will be sent on the next
	// round-trip
	err = stmr.Push(ctx, msg)
	if err != nil {
		common.Odl.Warn("Error pushing OTXSE message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}

	return nil

}

// runOTxEn sends a transaction end operation and waits for its return state and
// terminal status. OTXEN returns the transaction state in TTIRPA and completes
// with TTIOER or TTISTA.
//
// Parameters:
//   - ctx: Context used for the transaction-end operation.
//   - operation: OTXEN state-change operation to execute.
//   - transaction: Transaction whose state should be changed.
//
// Returns:
//   - error: Error if the operation cannot be sent or the server reports a failure.
func (c *connection) runOTxEn(ctx context.Context, operation txStateChangeOperation, transaction oracleTx) error {
	common.Odl.Debug("Running OTXEN", "operation", operation)

	// get the streamer
	stmr, ok := c.shelf.GetMessageStreamer().(MessageStreamerInterface)
	if !ok {
		common.Odl.Warn("OTXEN requires a message streamer with callback support")
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// create the transaction end message
	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIFUN, oTxEn)
	if err != nil {
		common.Odl.Warn("Error creating OTXEN message", "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}
	otxen, ok := msg.(*tTIOtxen)
	if !ok {
		common.Odl.Warn("Unexpected message type for OTXEN", "message", msg)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// configure the message for the operation
	switch operation {
	case otxenCommit:
		otxen.configureForCommit(transaction)
	case otxenAbort:
		otxen.configureForAbort(transaction)
	default:
		common.Odl.Warn("Unsupported OTXEN operation", "operation", operation)
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	// push and flush the message
	if err := stmr.Push(ctx, msg); err != nil {
		common.Odl.Warn("Error pushing OTXEN message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}
	if err := stmr.Flush(ctx); err != nil {
		common.Odl.Warn("Error flushing OTXEN message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}

	// register message specific RPA
	stmr.RegisterPreUnmarshallCallback(TTIRPA, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return c.shelf.GetMessageFactory().GetMessageForFunction(TTIRPA, oTxEn)
	})
	defer stmr.UnRegisterPreUnmarshallCallback(TTIRPA)

	// handle the message result
	for {
		retMsg, err := stmr.Pull(ctx, TTIRPA, TTIOER, TTISTA)
		if err != nil {
			common.Odl.Warn("Error pulling OTXEN response", "error", err)
			return common.NewOracleError(oracleErrors.StreamerReadError, err)
		}

		switch retMsg.GetMsgCode() {
		case TTIRPA:
			if rpa, ok := retMsg.(*ttiOTxEnRPA); ok {
				common.Odl.Debug("OTXEN returned transaction state", "outState", rpa.GetOutState())
			}
		case TTIOER:
			if err := retMsg.(tTIOerIface).getError(); err != nil {
				return err
			}
			return nil
		case TTISTA:
			return nil
		}
	}
}

// unregisterTransactionOnError removes the local transaction registration when
// the server no longer reports an active transaction. A sessionless
// transaction is also removed when its end notification has been received,
// even if the connection status still reports an active transaction.
func (c *connection) unregisterTransactionOnError() {
	if !c._isInTransaction {
		c.shelf.unregisterTransaction()
		return
	}

	if sessionlessTx, ok := c.shelf.getTransaction().(*sessionlessTransaction); ok {
		if sessionlessTx.isEndedOnServer || !sessionlessTx.isStartedOnServer {
			c.shelf.unregisterTransaction()
		}
	}
}

// convertTxOptionsToFlags converts standard transaction options to OTXSE flags.
//
// Note that otxseTransReadOnly, otxseTransSerializable and otxseTransReadWrite
// cannot be combined. If opts.ReadOnly is set the isolation level will be
// ignored.
//
// Parameters:
//   - opts: Transaction isolation and read-only options.
//
// Returns:
//   - driverCommon.UB4: OTXSE flags representing opts.
func convertTxOptionsToFlags(opts driver.TxOptions) driverCommon.UB4 {
	flags := otxseTransNew
	if opts.ReadOnly {
		flags |= otxseTransReadOnly
		return flags
	}
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelSerializable:
		flags |= otxseTransSerializable
	case sql.LevelReadCommitted, sql.LevelDefault:
		flags |= otxseTransReadWrite
	}
	return flags
}
