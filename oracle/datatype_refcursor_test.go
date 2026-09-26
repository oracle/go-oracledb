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
** one is included with the Software (each a "Larger Work") to which the Software
** is contributed by such licensors,
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
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

type refCursorTestDriverRows struct{}

func (*refCursorTestDriverRows) Columns() []string         { return nil }
func (*refCursorTestDriverRows) Close() error              { return nil }
func (*refCursorTestDriverRows) Next([]driver.Value) error { return nil }

type refCursorTestFetchRows struct {
	refCursorTestDriverRows
	fetchErr   error
	fetchCalls int
}

func (r *refCursorTestFetchRows) Fetch(context.Context) error {
	r.fetchCalls++
	return r.fetchErr
}

type refCursorTestDriver struct{}

func (refCursorTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("Open is not used")
}

type refCursorTestConnector struct {
	conn *refCursorTestConn
}

func (c refCursorTestConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (refCursorTestConnector) Driver() driver.Driver                          { return refCursorTestDriver{} }

type refCursorTestConn struct {
	query string
	rows  driver.Rows
}

func (*refCursorTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("Prepare is not used")
}
func (*refCursorTestConn) Close() error              { return nil }
func (*refCursorTestConn) Begin() (driver.Tx, error) { return nil, errors.New("Begin is not used") }
func (*refCursorTestConn) CheckNamedValue(*driver.NamedValue) error {
	return nil
}
func (c *refCursorTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 1 {
		return nil, errors.New("unexpected argument count")
	}
	rows, ok := args[0].Value.(driver.Rows)
	if !ok {
		return nil, errors.New("argument is not driver.Rows")
	}
	c.query = query
	c.rows = rows
	return rows, nil
}

// TestRefCursorRowsGetRowsNil verifies that a NULL REF CURSOR returns no sql.Rows.
func TestRefCursorRowsGetRowsNil(t *testing.T) {
	t.Parallel()

	var rows datatype.Rows
	got, err := rows.GetRows(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetRows for NULL REF CURSOR: %v", err)
	}
	if got != nil {
		t.Fatalf("GetRows for NULL REF CURSOR = %v, want nil", got)
	}
}

// TestRefCursorRowsScan verifies the Scanner path accepts a decoded REF CURSOR,
// clears a NULL cursor, and rejects other driver values.
func TestRefCursorRowsScan(t *testing.T) {
	t.Parallel()

	var rows datatype.Rows
	if err := rows.Scan(&refCursorTestDriverRows{}); err != nil {
		t.Fatalf("Scan driver.Rows: %v", err)
	}
	if err := rows.Scan(nil); err != nil {
		t.Fatalf("Scan nil: %v", err)
	}
	if err := rows.Scan("not rows"); err == nil {
		t.Fatal("Scan non-rows value unexpectedly succeeded")
	}
}

// TestRefCursorRowsGetRowsFetchesAndWraps verifies deferred fetching and the
// internal refcursor query used to wrap the cursor as standard sql.Rows.
func TestRefCursorRowsGetRowsFetchesAndWraps(t *testing.T) {
	t.Parallel()

	cursor := &refCursorTestFetchRows{}
	driverConn := &refCursorTestConn{}
	db := sql.OpenDB(refCursorTestConnector{conn: driverConn})
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("acquire test connection: %v", err)
	}
	defer conn.Close()

	var rows datatype.Rows
	if err = rows.Scan(cursor); err != nil {
		t.Fatalf("Scan cursor: %v", err)
	}
	wrapped, err := rows.GetRows(context.Background(), conn)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	defer wrapped.Close()
	if cursor.fetchCalls != 1 {
		t.Fatalf("Fetch calls = %d, want 1", cursor.fetchCalls)
	}
	if driverConn.query != datatype.RefCursorQuery || driverConn.rows != cursor {
		t.Fatalf("internal query = (%q, %v), want (%q, %v)", driverConn.query, driverConn.rows, datatype.RefCursorQuery, cursor)
	}
}

// TestRefCursorRowsGetRowsFetchError verifies that a deferred-fetch failure is
// returned before the internal refcursor query is attempted.
func TestRefCursorRowsGetRowsFetchError(t *testing.T) {
	t.Parallel()

	want := errors.New("fetch failed")
	var rows datatype.Rows
	if err := rows.Scan(&refCursorTestFetchRows{fetchErr: want}); err != nil {
		t.Fatalf("Scan cursor: %v", err)
	}
	if _, err := rows.GetRows(context.Background(), nil); !errors.Is(err, want) {
		t.Fatalf("GetRows error = %v, want %v", err, want)
	}
}
