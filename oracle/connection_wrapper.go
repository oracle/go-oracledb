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

package oracle

import (
	"context"
	"database/sql"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// connectionWrapper provides Oracle specific operations for a dedicated
// database/sql connection.
//
// The wrapped connection must be a connection returned by this driver.
type connectionWrapper struct {
	connection *sql.Conn
}

// Include here all functions/interfaces we want a connection to implement in
// order to be wrapped by this wrapper
type canBeWrapped interface {
	BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (common.SessionlessTransaction, error)
	ResumeSessionlessTx(ctx context.Context, globalTransactionID []byte, timeout uint16) (common.SessionlessTransaction, error)
}

// NewConnectionWrapper validates and wraps a dedicated database/sql connection
// for Oracle specific operations.
//
// Parameters:
//   - connection: Dedicated database/sql connection to wrap.
//
// Returns:
//   - *connectionWrapper: Wrapper for the supplied connection.
//   - error: Error if the underlying driver connection type is not supported.
func NewConnectionWrapper(connection *sql.Conn) (*connectionWrapper, error) {
	var wrapper *connectionWrapper
	err := connection.Raw(func(c any) error {
		_, ok := c.(canBeWrapped)
		if !ok {
			return common.NewOracleError(oracleErrors.UnsupportedFeature, nil, "Sessionless Transactions")
		}
		wrapper = &connectionWrapper{connection: connection}
		return nil
	})
	return wrapper, err
}

// BeginSessionlessTx starts a sessionless transaction on the wrapped connection.
//
// Parameters:
//   - ctx: Context used for the transaction start operation.
//   - opts: Standard transaction options.
//   - timeout:  the time for how long (in seconds) after the suspension of
//     this Sessionless transaction should the server roll back this
//     transaction. This is an attempt to avoid a transaction from holding
//     on to database resources (such as row locks) indefinitely.
//
// Returns:
//   - extensions.SessionlessTx: Started sessionless transaction.
//   - error: Error if the transaction cannot be started.
func (wrapper *connectionWrapper) BeginSessionlessTx(ctx context.Context, opts sql.TxOptions, timeout uint16) (*sessionlessTx, error) {
	var publicSessionlessTransaction *sessionlessTx
	err := wrapper.connection.Raw(func(c any) error {
		internalTx, err := c.(canBeWrapped).BeginSessionlessTx(ctx, opts, timeout)
		if err == nil {
			publicSessionlessTransaction = &sessionlessTx{underlyingConn: wrapper.connection, transaction: internalTx}
		}
		return err
	})
	return publicSessionlessTransaction, err
}

// ResumeSessionlessTx resumes a sessionless transaction on connection.
//
// Parameters:
//   - ctx: Context used for the transaction resume operation.
//   - globalTransactionID: Identifier of the sessionless transaction to resume.
//   - timeout: the time for how long (in seconds) the server attempts to
//     resume the transaction. If multiple sessions connect to the same database
//     instance and request to resume the same Sessionless transaction, only one
//     can successfully resume at any given time
//
// Returns:
//   - extensions.SessionlessTx: Resumed sessionless transaction.
//   - error: Error if the transaction cannot be resumed.
func (wrapper *connectionWrapper) ResumeSessionlessTx(ctx context.Context, globalTransactionID GlobalTransactionID, timeout uint16) (*sessionlessTx, error) {
	var publicSessionlessTransaction *sessionlessTx
	err := wrapper.connection.Raw(func(c any) error {
		internalTx, err := c.(canBeWrapped).ResumeSessionlessTx(ctx, globalTransactionID, timeout)
		if err == nil {
			publicSessionlessTransaction = &sessionlessTx{underlyingConn: wrapper.connection, transaction: internalTx}
		}
		return err
	})
	return publicSessionlessTransaction, err
}
