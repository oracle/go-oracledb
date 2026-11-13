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
	"fmt"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// addBeginEndRequestCallbacks registers callbacks that mark the beginning and
// end of a application request when the server supports session-state
// operations. It adds the explicit-boundary flag when the server reports
// support for it.
//
// A request is a logical unit of work that typically begins when an application
// checks out a database connection and ends when the connection is returned to
// the pool.
func addBeginEndRequestCallbacks(callbacks *requestCallbacks, conn *connection) {
	// Check whether the server supports session-state operations.
	capability, ok := conn.shelf.GetCapabilities()[kpccapRtbTtcSessstateops]
	if !ok || !capability.IsSet {
		common.Odl.Debug("Server does not support request boundaries")
		return
	}
	// Include the explicit-boundary flag when the server supports it.
	var flags sessionStateOperationFlags
	capability, ok = conn.shelf.GetCapabilities()[kpccapCtbTtc4ExplBound]
	if ok && capability.IsSet {
		common.Odl.Debug("Server supports explicit boundaries")
		flags |= sessionStateExplBound
	}

	// Begin request is sent when the connection is created and each time it is
	// borrowed from the pool.
	callbacks.appendRequestStartCallback(&requestBoundaryCallback{
		function: func(ctx context.Context) error {
			// Queue the begin-request boundary.
			err := conn.pushSessionStateRequest(ctx, flags|sessionStateRequestBegin)
			if err != nil {
				// If queuing fails, report that the connection is bad.
				return driver.ErrBadConn
			}
			return nil
		},
		description: "begin-request marker",
	}, true)

	// End request is sent when the connection is returned to the pool.
	callbacks.appendRequestEndCallback(&requestBoundaryCallback{
		function: func(ctx context.Context) error {
			// Queue the end-request boundary.
			err := conn.pushSessionStateRequest(ctx, flags|sessionStateRequestEnd)
			if err != nil {
				// If queuing fails, report that the connection is bad.
				return driver.ErrBadConn
			}
			return nil
		},
		description: "end-request marker",
	})
}

// pushSessionStateRequest queues an OSESSSTATE piggyback carrying session-state
// operation flags.
func (c *connection) pushSessionStateRequest(ctx context.Context, flags sessionStateOperationFlags) error {
	common.Odl.Finest("Pushing OSESSSTATE piggyback message", "sessionStateOperationFlags", flags)
	capability, ok := c.shelf.GetCapabilities()[kpccapRtbTtcSessstateops]
	if !ok || !capability.IsSet {
		return nil
	}

	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIPFN, oSessState)
	if err != nil {
		common.Odl.Warn("Error creating OSESSSTATE message", "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}

	request, ok := msg.(*tTIOsessstate)
	if !ok {
		err := fmt.Errorf("unexpected OSESSSTATE message type %T", msg)
		common.Odl.Warn("Error creating OSESSSTATE message", "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}
	request.flags = driverCommon.UB8(flags)

	if err := c.shelf.GetMessageStreamer().Push(ctx, msg); err != nil {
		common.Odl.Warn("Error pushing OSESSSTATE message", "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}
	return nil
}
