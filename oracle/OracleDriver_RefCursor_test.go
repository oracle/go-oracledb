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
	"database/sql/driver"
	"io"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestDriver_RefCursorOut verifies the godror-compatible sql.Out API. The
// returned driver.Rows owns a child server cursor and is fetched on its first
// Next call.
func TestDriver_RefCursorOut(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var rows driver.Rows
	_, err = db.ExecContext(context.Background(), `
BEGIN
  OPEN :1 FOR SELECT 42 AS n, 'cursor' AS label FROM dual;
END;`, sql.Out{Dest: &rows})
	if err != nil {
		t.Fatalf("open REF CURSOR: %v", err)
	}
	if rows == nil {
		t.Fatal("REF CURSOR returned nil rows")
	}
	t.Cleanup(func() { _ = rows.Close() })

	values := make([]driver.Value, len(rows.Columns()))
	if err = rows.Next(values); err != nil {
		t.Fatalf("fetch REF CURSOR row: %v", err)
	}
	if got, ok := values[0].(int64); !ok || got != 42 {
		t.Fatalf("REF CURSOR number = %#v (%T), want int64(42)", values[0], values[0])
	}
	if got, ok := values[1].(string); !ok || got != "cursor" {
		t.Fatalf("REF CURSOR label = %#v (%T), want cursor", values[1], values[1])
	}
	if err = rows.Next(values); err != io.EOF {
		t.Fatalf("REF CURSOR final Next = %v, want io.EOF", err)
	}
}

// TestDriver_RefCursorMultipleOut verifies that multiple REF CURSOR OUT binds
// retain their positional order and can be consumed independently.
func TestDriver_RefCursorMultipleOut(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var first, second driver.Rows
	_, err = db.ExecContext(context.Background(), `
BEGIN
  OPEN :1 FOR SELECT 11 AS n FROM dual;
  OPEN :2 FOR SELECT 22 AS n FROM dual;
END;`, sql.Out{Dest: &first}, sql.Out{Dest: &second})
	if err != nil {
		t.Fatalf("open multiple REF CURSORs: %v", err)
	}
	if first == nil || second == nil {
		t.Fatal("multiple REF CURSOR bind returned nil rows")
	}
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })

	for name, cursor := range map[string]struct {
		rows driver.Rows
		want int64
	}{"first": {first, 11}, "second": {second, 22}} {
		values := make([]driver.Value, len(cursor.rows.Columns()))
		if err := cursor.rows.Next(values); err != nil {
			t.Fatalf("fetch %s REF CURSOR: %v", name, err)
		}
		if got, ok := values[0].(int64); !ok || got != cursor.want {
			t.Fatalf("%s REF CURSOR value = %#v (%T), want %d", name, values[0], values[0], cursor.want)
		}
		if err := cursor.rows.Next(values); err != io.EOF {
			t.Fatalf("%s REF CURSOR final Next = %v, want io.EOF", name, err)
		}
	}
}

// TestDriver_RefCursorTypedHolder verifies the public datatype.RefCursor holder,
// including retrieving and closing the rows through its Rows method.
func TestDriver_RefCursorTypedHolder(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var cursor datatype.RefCursor
	_, err = db.ExecContext(context.Background(), `
BEGIN
  OPEN :1 FOR SELECT 'typed' AS label FROM dual;
END;`, sql.Out{Dest: &cursor})
	if err != nil {
		t.Fatalf("open typed REF CURSOR: %v", err)
	}
	rows := cursor.Rows()
	if rows == nil {
		t.Fatal("typed REF CURSOR returned nil rows")
	}
	values := make([]driver.Value, len(rows.Columns()))
	if err := rows.Next(values); err != nil {
		t.Fatalf("fetch typed REF CURSOR: %v", err)
	}
	if got, ok := values[0].(string); !ok || got != "typed" {
		t.Fatalf("typed REF CURSOR value = %#v (%T), want typed", values[0], values[0])
	}
	if err := cursor.Close(); err != nil {
		t.Fatalf("close typed REF CURSOR: %v", err)
	}
	if cursor.Rows() != nil {
		t.Fatal("typed REF CURSOR rows retained after Close")
	}
}
