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
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// Used as part of the ALTER SESSION to set isolation level to READ COMMITTED
	_isolationLevelReadCommitted = "ALTER SESSION SET ISOLATION_LEVEL = READ COMMITTED"
	// Used as part of the ALTER SESSION to set isolation level to SERIALIZABLE
	_isolationLevelSerializable = "ALTER SESSION SET ISOLATION_LEVEL = SERIALIZABLE"
	// Used as part of the SET TRANSACTION to set transaction READ ONLY
	_transactionReadOnly = "SET TRANSACTION READ ONLY"
)

// Begin starts and returns a new transaction with isolation level read
// committed.
func (c *connection) Begin() (driver.Tx, error) {
	context := context.Background()
	opts := driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
		ReadOnly:  false,
	}
	return c.BeginTx(context, opts)
}

// BeginTx starts and returns a new transaction.
func (c *connection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	common.Odl.Debug("Starting transaction")

	if c.shelf.isInTransaction() {
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.AlreadyInTransaction, nil, nil))
	}

	var isolationLevelStmt string
	// set isolation level
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault, sql.LevelReadCommitted:
		isolationLevelStmt = _isolationLevelReadCommitted
	case sql.LevelSerializable:
		isolationLevelStmt = _isolationLevelSerializable
	default:
		return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.IsolationLevelNotSupported, nil, nil))
	}

	tx := newTransaction(c, ctx)
	c.shelf.registerTransaction(tx)

	// Set transaction isolation level
	if isolationLevelStmt != "" {
		_, err := c.ExecContext(ctx, isolationLevelStmt, nil)
		if err != nil {
			c.shelf.unregisterTransaction()
			return nil, common.NewOracleError(oracleErrors.ConfigureTransactionError, err, nil)
		}
	}

	// set read only
	if opts.ReadOnly {
		_, err := c.ExecContext(ctx, _transactionReadOnly, nil)
		if err != nil {
			c.shelf.unregisterTransaction()
			return nil, c.shelf.LocalizeError(common.NewOracleError(oracleErrors.ConfigureTransactionError, err, nil))
		}
	}

	return tx, nil
}
