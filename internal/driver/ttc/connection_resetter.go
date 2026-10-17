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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// ResetSession implements driver.SessionResetter.
// - Closes statements that have been left open
// - Resets session properties
// - Flushes piggyback messages
func (c *connection) ResetSession(ctx context.Context) error {
	common.Odl.Debug("ResetSession called", "cnx", (*c).String())

	if c._isClosed || !c._isValid {
		return driver.ErrBadConn
	}
	statements := c.shelf.GetStatements(true)
	for _, statement := range statements {
		if err := statement.Close(); err != nil {
			common.Odl.Warn("Stale statement left in connection cannot be closed", "error", err)
			c._isValid = false
			return driver.ErrBadConn
		}
	}

	// push end-request for previous request and begin request for current request
	if err := c.pushSessionStateRequest(ctx, sessionStateExplBound|sessionStateRequestEnd); err != nil {
		common.Odl.Warn("An error occurred when pushing end request", "error", err)
		return driver.ErrBadConn
	}
	if err := c.pushSessionStateRequest(ctx, sessionStateExplBound|sessionStateRequestBegin); err != nil {
		common.Odl.Warn("An error occurred when pushing begin request", "error", err)
		return driver.ErrBadConn
	}
	c.sessCtx.GetSessionProperties().Reset()
	err := c.shelf.GetMessageStreamer().Flush(ctx)
	if err != nil {
		common.Odl.Warn("Flush of messages during reset has failed", "error", err)
		return driver.ErrBadConn
	}

	return nil
}

// pushSessionStateRequest queues an OSESSSTATE request-boundary piggyback when
// the server advertises support for session-state operations.
func (c *connection) pushSessionStateRequest(ctx context.Context, flags sessionState) error {
	capability, ok := c.shelf.GetCapabilities()[kpccapRtbTtcSessstateops]
	if !ok || !capability.IsSet {
		return nil
	}

	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIPFN, oSessState)
	if err != nil {
		common.Odl.Warn("Error creating OSESSSTATE message", "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}

	request, _ := msg.(*tTIOsessstate)
	request.flags = driverCommon.UB8(flags)

	if err := c.shelf.GetMessageStreamer().Push(ctx, msg); err != nil {
		common.Odl.Warn("Error pushing OSESSSTATE message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}
	return nil
}
