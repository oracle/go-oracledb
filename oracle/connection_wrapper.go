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

// canBeWrapped lists the driver operations required by connectionWrapper.
type canBeWrapped interface {
	// PrepareForSessionlessStartOperation registers a sessionless transaction
	// that will be started by the following database/sql BeginTx call.
	PrepareForSessionlessStartOperation(ctx context.Context, timeout uint16) (common.SessionlessTransaction, error)
	// ClearPrepareForSessionlessStartOperation removes a failed prepared start.
	ClearPrepareForSessionlessStartOperation()
	// PrepareForSessionlessResumeOperation registers a sessionless transaction
	// that will be resumed by the following database/sql BeginTx call.
	PrepareForSessionlessResumeOperation(ctx context.Context, globalTransactionID []byte, timeout uint16) (common.SessionlessTransaction, error)
	// ClearPrepareForSessionlessResumeOperation completes or removes a prepared
	// resume after database/sql returns from BeginTx.
	ClearPrepareForSessionlessResumeOperation(common.SessionlessTransaction, error)
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
//   - timeout: Maximum number of seconds the server retains a suspended
//     transaction before rolling it back to release database resources.
//
// Returns:
//   - *sessionlessTx: Started sessionless transaction.
//   - error: Error if the transaction cannot be started.
func (wrapper *connectionWrapper) BeginSessionlessTx(ctx context.Context, opts *sql.TxOptions, timeout uint16) (*sessionlessTx, error) {
	var publicSessionlessTransaction *sessionlessTx
	var internalTx common.SessionlessTransaction
	var err error
	err = wrapper.connection.Raw(func(c any) error {
		// register the transaction with status prepare and start function start
		internalTx, err = c.(canBeWrapped).PrepareForSessionlessStartOperation(ctx, timeout)
		return err
	})
	if err != nil {
		return nil, err
	}
	sqlTx, err := wrapper.connection.BeginTx(ctx, opts)
	if err != nil {
		_ = wrapper.connection.Raw(func(c any) error {
			// unregister the transaction if an error occurred on begin transaction
			c.(canBeWrapped).ClearPrepareForSessionlessStartOperation()
			return nil
		})
		return nil, err
	}
	publicSessionlessTransaction = &sessionlessTx{underlyingTx: internalTx, transaction: sqlTx, transactionContext: ctx}
	return publicSessionlessTransaction, err
}

// ResumeSessionlessTx resumes a sessionless transaction on connection.
//
// Parameters:
//   - ctx: Context used for the transaction resume operation.
//   - globalTransactionID: Identifier of the sessionless transaction to resume.
//   - timeout: Maximum number of seconds this connection waits for another
//     session on the same instance to suspend the transaction so it can be
//     resumed. If no other session has the transaction active, it resumes
//     immediately. A timeout of zero fails immediately when another session
//     still has it active.
//
// Returns:
//   - *sessionlessTx: Resumed sessionless transaction.
//   - error: Error if the transaction cannot be resumed.
func (wrapper *connectionWrapper) ResumeSessionlessTx(ctx context.Context, globalTransactionID GlobalTransactionID, timeout uint16) (*sessionlessTx, error) {
	var publicSessionlessTransaction *sessionlessTx
	var internalTx common.SessionlessTransaction
	var err error
	err = wrapper.connection.Raw(func(c any) error {
		// register the transaction with status prepare and start function resume
		internalTx, err = c.(canBeWrapped).PrepareForSessionlessResumeOperation(ctx, globalTransactionID, timeout)
		return err
	})
	if err != nil {
		return nil, err
	}
	sqlTx, err := wrapper.connection.BeginTx(ctx, &sql.TxOptions{})
	_ = wrapper.connection.Raw(func(c any) error {
		// unregister the transaction if an error occurred on resume transaction
		c.(canBeWrapped).ClearPrepareForSessionlessResumeOperation(internalTx, err)
		return nil
	})
	if err != nil {
		return nil, err
	}
	publicSessionlessTransaction = &sessionlessTx{underlyingTx: internalTx, transaction: sqlTx, transactionContext: ctx}
	return publicSessionlessTransaction, err
}
