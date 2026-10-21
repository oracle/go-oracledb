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
	"strconv"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

const resetStateLevel2 = 0x02

// addCursorLifecycleCallbacks configures cursor cleanup before the end-request
// marker, so any required OCCA messages remain part of the completed request.
func addCursorLifecycleCallbacks(callbacks *requestCallbacks, c *connection) {
	cacheSize := 10
	if properties := c.shelf.GetConnectionProperties(); properties != nil {
		cacheSize = properties.GetCursorCacheSize()
	}
	c.shelf.initializeCursorCache(cacheSize)

	if c.resetStateEnabled() {
		callbacks.appendRequestEndCallback(&requestBoundaryCallback{
			description: "Invalidate cursors after reset state",
			function:    c.invalidateCursors,
		})
		return
	}
	callbacks.appendRequestEndCallback(&requestBoundaryCallback{
		description: "Cache idle cursors",
		function:    c.cacheCursors,
	})
}

func (c *connection) resetStateEnabled() bool {
	if c.sessCtx == nil {
		return false
	}
	value := c.sessCtx.GetSessionProperties().GetProperty(authResetState)
	var resetState string
	switch value := value.(type) {
	case driverCommon.B1Array:
		resetState = driverCommon.B1ArrayToString(value)
	case *driverCommon.KeyValue:
		resetState = driverCommon.B1ArrayToString(value.Value)
	case string:
		resetState = value
	default:
		return false
	}
	level, err := strconv.Atoi(resetState)
	return err == nil && level&resetStateLevel2 != 0
}

func (c *connection) cacheCursors(context.Context) error {
	for _, statement := range c.shelf.cacheCursors() {
		if err := statement._closeCursor(); err != nil {
			return driver.ErrBadConn
		}
	}
	return nil
}

func (c *connection) invalidateCursors(context.Context) error {
	for _, statement := range c.shelf.GetStatements(false) {
		statement.markCursorClosed()
	}
	c.shelf.clearCachedCursors()
	return nil
}

// ResetSession implements driver.SessionResetter.
// - Runs request-start callbacks when the connection is borrowed from the pool
// - Resets session properties
// - Flushes piggyback messages
func (c *connection) ResetSession(ctx context.Context) error {
	common.Odl.Debug("ResetSession called", "cnx", (*c).String())

	if c._isClosed || !c._isValid {
		return driver.ErrBadConn
	}
	// Run request-start callbacks. Return if a callback marks the connection bad.
	common.Odl.Finest("Running request-start callbacks on ResetSession")
	if err := runRequestBoundaryCallbacks(ctx, c._requestStartCallbacks); err != nil {
		return err
	}

	c.sessCtx.GetSessionProperties().Reset()
	err := c.shelf.GetMessageStreamer().Flush(ctx)
	if err != nil {
		common.Odl.Warn("Flush of messages during reset has failed", "error", err)
		return driver.ErrBadConn
	}

	return nil
}
