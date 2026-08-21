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
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// timeout duration to prevent the Close function from blocking indefinitely
	_connCloseTimeout time.Duration = 10000000000 // 10s
	// flag sent to the network layer when calling Disconnect
	_disconnectFlag int = 0
)

// Close closes the TTC connection. A timeout context is used to prevent this
// method from blocking indefinitely
func (c *connection) Close() error {
	var nsDisconnectErr error
	common.Odl.Debug("Closing connection")
	// Create a context with timeout, this context will prevent this action from
	// blocking indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), _connCloseTimeout)
	defer cancel()

	common.Odl.Debug("Closing statements")
	// We do close opened statements as they are automatically closed by the
	// server. So we just drain the list of statements
	_ = c.shelf.GetStatements(true)

	// Close the TTC connection.
	ttcCloseErr := c._closeTTCConnection(ctx)

	// Disconnect network connection, since the network layer call does not take
	// the context, we are checking it here.
	disconnect := make(chan error)
	go func() {
		common.Odl.Debug("Closing network connection")
		disconnect <- c.ns.Disconnect(ctx, _disconnectFlag)
	}()

	// Wait until timeout or connection disconnected.
	select {
	case <-ctx.Done():
		nsDisconnectErr = common.NewOracleError(oracleErrors.ConnCloseTimeout, ctx.Err(), nil)
	case nsDisconnectErr = <-disconnect:
		break
	}

	// Mark the connection as closed. This is done even if an error occurs.
	c._isClosed = true
	c.shelf.getEventService().post(connectionClosedEvent)

	// Check if closing connection returned error and return error to caller
	if ttcCloseErr != nil {
		return c.shelf.LocalizeError(ttcCloseErr)
	}
	if nsDisconnectErr != nil {
		return c.shelf.LocalizeError(nsDisconnectErr)
	}

	// the connection is closed, return.
	common.Odl.Info("Connection closed")
	return nil
}

// _closeTTCConnection sends the logOff message to disconnect form the database
func (c *connection) _closeTTCConnection(ctx context.Context) error {
	return c.runFunctionWithFunHeader(ctx, logOff)
}
