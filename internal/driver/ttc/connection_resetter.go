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
	"sort"
	"strconv"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// resetStateLevel2 indicates that the server resets session state, including
// open cursors, when a connection is returned to the pool.
const resetStateLevel2 = 0x02

// addCursorCacheCallbacks configures cursor cache work before the end-request
// marker, so OCCA messages for evicted cursors remain part of that request.
func addCursorCacheCallbacks(callbacks *requestCallbacks, c *connection) {
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

// initializeCursorCache creates the connection's cursor cache from the driver
// configuration. A zero size disables cursor caching.
func (c *connection) initializeCursorCache() {
	cacheSize := 10
	if properties := c.shelf.GetConnectionProperties(); properties != nil {
		cacheSize = properties.GetCursorCacheSize()
	}
	c.shelf.initializeCursorCache(cacheSize)
}

// resetStateEnabled reports whether the negotiated session reset level closes
// cursors on the server, so the client must invalidate them locally instead.
func (c *connection) resetStateEnabled() bool {
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

// cacheCursors stores the connection's live cursors in least-recently-used
// order and closes cursors that do not fit in its configured cache. When
// caching is disabled, it closes every live cursor. A failed cursor-close
// operation leaves the server-side cursor state uncertain, so it returns
// driver.ErrBadConn and prevents the connection from being reused.
func (c *connection) cacheCursors(context.Context) error {
	statements := c.shelf.GetStatements(false)
	cache := c.shelf._cursorCache
	if cache == nil {
		for _, statement := range statements {
			if err := statement._closeCursor(); err != nil {
				return driver.ErrBadConn
			}
		}
		return nil
	}

	sort.Slice(statements, func(i, j int) bool {
		return statements[i].cursorLastUsed < statements[j].cursorLastUsed
	})

	cached := 0
	for _, statement := range statements {
		if statement.qualifiedQuery.cursorID != 0 && statement.cursorCachedAt != 0 {
			cached++
		}
	}

	var evicted []*Statement
	for _, statement := range statements {
		if statement.qualifiedQuery.cursorID == 0 || statement.cursorCachedAt == statement.cursorLastUsed {
			continue
		}

		alreadyCached := statement.cursorCachedAt != 0
		if !alreadyCached && cached == c.shelf._cursorCacheSize {
			for _, displaced := range statements {
				if displaced.qualifiedQuery.cursorID == 0 || displaced.cursorCachedAt == 0 {
					continue
				}
				cache.Remove(displaced.cursorCacheKey())
				displaced.cursorCachedAt = 0
				evicted = append(evicted, displaced)
				cached--
				break
			}
		}

		cache.Put(statement.cursorCacheKey(), statement)
		statement.cursorCachedAt = statement.cursorLastUsed
		if !alreadyCached {
			cached++
		}
	}

	for _, statement := range evicted {
		if err := statement._closeCursor(); err != nil {
			return driver.ErrBadConn
		}
	}
	return nil
}

// invalidateCursors mirrors server reset state locally. The reset has already
// closed server cursors, so this clears their local IDs and cache entries
// without sending OCCA messages.
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
