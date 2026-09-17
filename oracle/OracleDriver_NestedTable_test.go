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
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

func TestDriver_NestedTableScalarPLSQL(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire physical connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	typeName := strings.ToUpper(createObjectName("number_nested_table"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+typeName+" AS TABLE OF NUMBER"); err != nil {
		t.Fatalf("create nested table type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

	typ, err := datatype.GetObjectType(ctx, conn, typeName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", typeName, err)
	}
	if typ.IsObject() || typ.VArray || !typ.Collection {
		t.Fatalf("unexpected nested-table descriptor: %#v", typ)
	}
	in, err := typ.NewCollection()
	if err != nil {
		t.Fatalf("NewCollection: %v", err)
	}
	for _, value := range []any{int64(10), nil, int64(30)} {
		if err := in.Append(value); err != nil {
			t.Fatalf("Append(%v): %v", value, err)
		}
	}

	var out datatype.ObjectCollection
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL nested-table IN/OUT: %v", err)
	}
	if out.IsNull() || out.VArray {
		t.Fatalf("nested-table OUT = %#v", out)
	}
	if length, err := out.Len(); err != nil || length != 3 {
		t.Fatalf("nested-table length = %d, %v; want 3", length, err)
	}
	if got, err := out.Get(1); err != nil || got != int64(10) {
		t.Fatalf("nested-table element 1 = %#v, %v; want 10", got, err)
	}
	if got, err := out.Get(2); err != nil || got != nil {
		t.Fatalf("nested-table element 2 = %#v, %v; want nil", got, err)
	}
	if got, err := out.Get(3); err != nil || got != int64(30) {
		t.Fatalf("nested-table element 3 = %#v, %v; want 30", got, err)
	}

	nullIn, err := typ.NewCollection()
	if err != nil {
		t.Fatalf("NewCollection for NULL: %v", err)
	}
	nullIn.SetNull()
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, nullIn); err != nil {
		t.Fatalf("PL/SQL NULL nested-table IN/OUT: %v", err)
	}
	if !out.IsNull() {
		t.Fatal("NULL nested-table OUT bind was not marked NULL")
	}
}
