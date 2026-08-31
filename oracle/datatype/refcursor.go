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

// Package datatype provides the typed OUT-bind holder for Oracle REF CURSORs.
package datatype

import "database/sql/driver"

// RefCursor is a typed OUT-bind holder for an Oracle REF CURSOR.
//
// Pass a pointer to RefCursor as the destination of sql.Out. Rows returns the
// server cursor after execution. A RefCursor may also be used for internal
// typed cursor binds when a database API requires REF CURSOR OUT parameters.
type RefCursor struct {
	rows driver.Rows
}

// Rows returns the cursor result set. It is nil for a NULL cursor.
func (c *RefCursor) Rows() driver.Rows {
	if c == nil {
		return nil
	}
	return c.rows
}

// SetRows associates rows with c. It is intended for driver transport
// implementations that populate a typed REF CURSOR OUT bind.
func (c *RefCursor) SetRows(rows driver.Rows) {
	if c != nil {
		c.rows = rows
	}
}

// Close closes the server cursor, if it was returned.
func (c *RefCursor) Close() error {
	if c == nil || c.rows == nil {
		return nil
	}
	err := c.rows.Close()
	c.rows = nil
	return err
}
