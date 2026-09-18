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
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

// Implementation of Pinger and Validator interfaces

// *** Pinger ***

// Ping pings the database to check if the connection is in a valid state
//
// Returns: driver.ErrBadConn if the connection is not in a valid state
//
//	otherwise nil
func (c *connection) Ping(ctx context.Context) error {
	if !c.isValid() {
		return driver.ErrBadConn
	}
	err := c.runFunctionWithFunHeader(ctx, ping)
	if !c.isValid() {
		return driver.ErrBadConn
	}
	if err != nil {
		return err
	}
	return nil
}

// *** Validator ***

const (
	// timeout duration to prevent the IsValid function from blocking indefinitely
	_rollbackTimeout time.Duration = 10000000000 // 10s
)

// IsValid checks if the connection is valid. This method is used by the connection
// pool prior to placing the connection into the connection pool. If the server
// has reported an ongoing transaction, it will be rolled back.
//
// Returns: true if the connection is valid otherwise false
func (c *connection) IsValid() bool {

	c.isValid()

	// check for ongoing transaction and rollback
	if c._isInTransaction {
		ctx, cancel := context.WithTimeout(common.BackgroundContext, _rollbackTimeout)
		defer cancel()
		if err := c.rollbackActiveTransaction(ctx); err != nil {
			common.Odl.Warn("Rollback of active transaction during reset has failed", "error", err)
			// only invalidate the connection if the transaction is not closed after the call
			// independent on whether there was an error.
			if c._isInTransaction {
				c._isValid = false
			}
		}
	}

	return c._isValid
}

// isValid checks connection status without ending active transactions
func (c *connection) isValid() bool {
	// Check if inband notification has been received.
	c._isValid = c._isValid && !c.ns.CheckInbandNotification()
	return c._isValid
}

// rollbackActiveTransaction rolls back the transaction reported as active by
// the server before the connection is returned to the pool.
//
// Parameters:
//   - ctx: Context used for the rollback operation.
//
// Returns:
//   - error: Error if the rollback message cannot be sent or completed.
func (c *connection) rollbackActiveTransaction(ctx context.Context) error {
	transaction := c.shelf.getTransaction()
	if transaction == nil {
		// if the transaction was not create using the API, create it
		transaction = newTransaction(c, ctx)
	}

	if err := c.runOTxEn(ctx, otxenAbort, transaction); err != nil {
		c.unregisterTransactionOnError()
		return err
	}

	c._isInTransaction = false
	c.shelf.unregisterTransaction()
	return nil
}
