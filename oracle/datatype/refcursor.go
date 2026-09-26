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

// Package datatype contains public Oracle-specific datatype helpers.
package datatype

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
)

// RefCursorQuery is the private statement text used to expose an already
// fetched REF CURSOR through database/sql. Applications should use GetRows
// rather than invoking it directly.
const RefCursorQuery = "refcursor"

// Rows is an Oracle REF CURSOR OUT-bind value. Call GetRows with the execution
// context and same *sql.Conn that opened the cursor to expose it as *sql.Rows.
type Rows struct {
	rows driver.Rows
}

type refCursorFetcher interface {
	Fetch(context.Context) error
}

// setDriverRows records the driver cursor decoded for this OUT bind.
func (r *Rows) setDriverRows(rows driver.Rows) {
	r.rows = rows
}

// Scan implements sql.Scanner for REF CURSOR OUT-bind assignment. The TTC
// driver supplies an already-open driver.Rows value; NULL clears the cursor.
func (r *Rows) Scan(src any) error {
	if src == nil {
		r.rows = nil
		return nil
	}
	rows, ok := src.(driver.Rows)
	if !ok {
		return fmt.Errorf("REF CURSOR source has type %T, want driver.Rows", src)
	}
	r.setDriverRows(rows)
	return nil
}

// GetRows fetches the REF CURSOR using ctx and exposes it as *sql.Rows on conn.
// conn must be the same dedicated connection that executed the OUT bind.
func (r *Rows) GetRows(ctx context.Context, conn *sql.Conn) (*sql.Rows, error) {
	if r == nil || r.rows == nil {
		return nil, nil
	}
	if fetcher, ok := r.rows.(refCursorFetcher); ok {
		if err := fetcher.Fetch(ctx); err != nil {
			return nil, err
		}
	}
	return conn.QueryContext(ctx, RefCursorQuery, r.rows)
}

var _ sql.Scanner = (*Rows)(nil)
