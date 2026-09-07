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
)

// Implementation of Pinger and Validator interfaces

// *** Pinger ***

// Ping verifies the physical connection with one serialized TTC ping exchange.
// It checks the connection state before and after waiting for session admission.
// Context cancellation while waiting for admission is returned unchanged;
// failure of the TTC ping is reported as driver.ErrBadConn.
//
// Returns:
//   - driver.ErrBadConn if the connection is closed or invalid, or the TTC ping fails.
//   - the context error if admission is canceled before the ping starts.
//   - nil when the ping completes successfully.
func (c *connection) Ping(ctx context.Context) error {
	closed, valid := c.connectionState()
	if closed || !valid {
		return driver.ErrBadConn
	}
	release, guardErr := c.shelf.synchronizer.begin(ctx)
	if guardErr != nil {
		return guardErr
	}
	defer release()

	// Close or invalidation may have won while Ping was waiting for admission.
	closed, valid = c.connectionState()
	if closed || !valid {
		return driver.ErrBadConn
	}

	err := c.runFunctionWithFunHeader(ctx, ping)
	if err != nil {
		return driver.ErrBadConn
	}
	return nil
}

// *** Validator ***

// IsValid checks if the connection is valid
//
// Returns: true if the connection is valid otherwise false
func (c *connection) IsValid() bool {
	closed, valid := c.connectionState()
	if closed || !valid {
		return false
	}
	// Check if inband notification has been received.
	if c.ns.CheckInbandNotification() {
		c.invalidate()
	}
	closed, valid = c.connectionState()
	return !closed && valid
}
