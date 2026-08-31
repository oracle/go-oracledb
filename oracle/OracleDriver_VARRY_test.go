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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestDriver_VARRAY_TypeShapeDebug is a temporary reproduction for
// DBMS_PICKLER.GET_TYPE_SHAPE cursor OUT binds on a scalar VARRAY.
func TestDriver_VARRAY_TypeShapeDebug(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const typeName = "JDBC_VARRAY_NUMBERS"
	if _, err = db.ExecContext(ctx, "CREATE OR REPLACE TYPE "+typeName+" AS VARRAY(5) OF NUMBER"); err != nil {
		t.Fatalf("create VARRAY type: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

	ex := datatype.Execer(db)
	var rc int64
	canonical := strings.TrimSpace(typeName)
	var toid, tds []byte
	var version int64
	var instantiable, superOwner, superName string
	// The type-shape API always returns attribute and subtype REF CURSORs. They
	// are not consumed by the scalar-VARRAY path yet, but must be typed cursor
	// OUT binds. A scalar placeholder causes PLS-00306 during overload
	// resolution, while an untyped nil is treated as an input bind.
	var unusedAttributes, unusedSubtypes datatype.RefCursor
	// Use the current full-name overload because it canonicalizes the supplied
	// name and returns hierarchy and
	// attribute metadata in addition to the TOID/version/TDS triplet.
	_, err = ex.ExecContext(ctx, `BEGIN
  :1 := SYS.DBMS_PICKLER.GET_TYPE_SHAPE(:2, :3, :4, :5, :6, :7, :8, :9, :10);
END;`,
		sql.Out{Dest: &rc}, sql.Out{Dest: &canonical, In: true},
		sql.Out{Dest: &toid}, sql.Out{Dest: &version}, sql.Out{Dest: &tds},
		sql.Out{Dest: &instantiable}, sql.Out{Dest: &superOwner}, sql.Out{Dest: &superName},
		sql.Out{Dest: &unusedAttributes}, sql.Out{Dest: &unusedSubtypes})
	// The metadata cursors are only needed by object-attribute support. Close
	// them here after the call has completed; this VARRAY path uses the TDS.
	defer unusedAttributes.Close()
	defer unusedSubtypes.Close()
	if err != nil {
		t.Fatalf("get VARRAY type shape: %v", err)
	}

	t.Logf("type shape: rc=%d canonical=%q version=%d toid=%x tds=%x instantiable=%q super=%q.%q", rc, canonical, version, toid, tds, instantiable, superOwner, superName)
}

// TestDriver_VARRAY_InsertSelectPLSQL validates the complete scalar-VARRAY
// path: metadata discovery on the physical connection, an INSERT bind, a
// SELECT decode, and a PL/SQL IN/OUT bind. The descriptor and every statement
// intentionally use one *sql.Conn because named-type metadata is connection
// scoped.
func TestDriver_VARRAY_InsertSelectPLSQL(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	typeName := "VARRAY_NUMBERS"
	tableName := createObjectName("varray_tab")
	if _, err = db.ExecContext(ctx, "CREATE TYPE "+typeName+" AS VARRAY(5) OF NUMBER"); err != nil {
		t.Fatalf("create VARRAY type: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })
	if _, err = db.ExecContext(ctx, "CREATE TABLE "+tableName+" (id NUMBER PRIMARY KEY, values_col "+typeName+")"); err != nil {
		t.Fatalf("create VARRAY table: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TABLE "+tableName) })

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire physical connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	typ, err := datatype.GetObjectType(ctx, conn, typeName)
	if err != nil {
		t.Fatalf("get VARRAY type metadata: %v", err)
	}

	in := mustNumberVARRAY(t, typ, 10, 20, 30)
	if _, err = conn.ExecContext(ctx, "INSERT INTO "+tableName+" (id, values_col) VALUES (:1, :2)", int64(1), in); err != nil {
		t.Fatalf("insert VARRAY: %v", err)
	}

	var selected datatype.ObjectCollection
	if err = conn.QueryRowContext(ctx, "SELECT values_col FROM "+tableName+" WHERE id = :1", int64(1)).Scan(&selected); err != nil {
		t.Fatalf("select VARRAY: %v", err)
	}
	assertNumberVARRAY(t, selected, 10, 20, 30)

	var out datatype.ObjectCollection
	// A VARRAY value is copied by the anonymous block, exercising both the IN
	// collection image and the named OUT descriptor/decoder path.
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL VARRAY IN/OUT: %v", err)
	}
	assertNumberVARRAY(t, out, 10, 20, 30)
}

// TestDriver_VARRAY_NullBind verifies that a typed NULL collection is sent
// with the named-type descriptor (rather than being treated as an untyped
// input NULL) and that the zero-length named-type image is unmarshalled back
// into a typed, NULL ObjectCollection.
func TestDriver_VARRAY_NullBind(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	typeName := strings.ToUpper(createObjectName("varray_null"))
	if _, err = db.ExecContext(ctx, "CREATE TYPE "+typeName+" AS VARRAY(5) OF NUMBER"); err != nil {
		t.Fatalf("create NULL VARRAY type: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire physical connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	typ, err := datatype.GetObjectType(ctx, conn, typeName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", typeName, err)
	}
	nullIn, err := typ.NewCollection()
	if err != nil {
		t.Fatalf("NewCollection(%s): %v", typeName, err)
	}
	nullIn.SetNull()
	if !nullIn.IsNull() {
		t.Fatal("SetNull did not mark the collection as NULL")
	}

	var out datatype.ObjectCollection
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, nullIn); err != nil {
		t.Fatalf("PL/SQL NULL VARRAY IN/OUT: %v", err)
	}
	if !out.IsNull() {
		t.Fatal("NULL VARRAY OUT bind was not marked NULL")
	}
	if got, err := out.Len(); err != nil || got != 0 {
		t.Fatalf("NULL VARRAY Len() = %d, %v; want 0", got, err)
	}
}

func mustNumberVARRAY(t *testing.T, typ *datatype.ObjectType, values ...int64) datatype.ObjectCollection {
	t.Helper()
	collection, err := typ.NewCollection()
	if err != nil {
		t.Fatalf("new VARRAY: %v", err)
	}
	for _, value := range values {
		if err = collection.Append(value); err != nil {
			t.Fatalf("append VARRAY value %d: %v", value, err)
		}
	}
	return collection
}

func assertNumberVARRAY(t *testing.T, collection datatype.ObjectCollection, want ...int64) {
	t.Helper()
	gotLen, err := collection.Len()
	if err != nil || gotLen != len(want) {
		t.Fatalf("VARRAY length = %d, %v; want %d", gotLen, err, len(want))
	}
	for i, expected := range want {
		got, err := collection.Get(i + 1)
		if err != nil {
			t.Fatalf("VARRAY element %d: %v", i+1, err)
		}
		if got != expected {
			t.Fatalf("VARRAY element %d = %v (%T), want %d", i+1, got, got, expected)
		}
	}
}

// TestDriver_VARRAY_PublicAPIs exercises named-type discovery and the public
// collection API for scalar VARRAY element types beyond NUMBER. Each call is
// kept on one physical connection because ObjectType descriptors are scoped to
// the connection that obtained the TOID/type-version metadata.
func TestDriver_VARRAY_PublicAPIs(t *testing.T) {
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

	tests := []struct {
		name    string
		element string
		values  []any
		check   func(*testing.T, datatype.ObjectCollection)
	}{
		{name: "varchar2", element: "VARCHAR2(30)", values: []any{"alpha", "beta"}, check: func(t *testing.T, c datatype.ObjectCollection) {
			assertVARRAYValues(t, c, "alpha", "beta")
		}},
		{name: "raw", element: "RAW(16)", values: []any{[]byte{1, 2, 3}, []byte{0xFE, 0xED}, nil}, check: func(t *testing.T, c datatype.ObjectCollection) {
			assertVARRAYValues(t, c, []byte{1, 2, 3}, []byte{0xFE, 0xED}, nil)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typeName := "varray_" + tc.name
			typeName = strings.ToUpper(typeName)
			db.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE")
			if _, err := db.ExecContext(ctx, "CREATE TYPE "+typeName+" AS VARRAY(5) OF "+tc.element); err != nil {
				t.Fatalf("create %s VARRAY: %v", tc.name, err)
			}
			t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

			typ, err := datatype.GetObjectType(ctx, conn, typeName)
			if err != nil {
				t.Fatalf("GetObjectType(%s): %v", typeName, err)
			}
			if typ.IsObject() || typ.String() != typ.FullName() || typ.FullName() == "" {
				t.Fatalf("unexpected VARRAY descriptor: String=%q FullName=%q IsObject=%v", typ.String(), typ.FullName(), typ.IsObject())
			}
			collection, err := typ.NewCollection()
			if err != nil {
				t.Fatalf("NewCollection: %v", err)
			}
			for _, value := range tc.values {
				if err := collection.Append(value); err != nil {
					t.Fatalf("Append(%#v): %v", value, err)
				}
			}
			if first, _ := collection.First(); first != 1 {
				t.Fatalf("First() = %d, want 1", first)
			}
			if last, _ := collection.Last(); last != len(tc.values) {
				t.Fatalf("Last() = %d, want %d", last, len(tc.values))
			}
			if next, _ := collection.Next(1); len(tc.values) > 1 && next != 2 {
				t.Fatalf("Next(1) = %d, want 2", next)
			}
			if len(tc.values) > 1 {
				if err := collection.Set(1, tc.values[0]); err != nil {
					t.Fatalf("Set(1): %v", err)
				}
			}

			var out datatype.ObjectCollection
			if _, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, collection); err != nil {
				t.Fatalf("PL/SQL %s VARRAY IN/OUT: %v", tc.name, err)
			}
			tc.check(t, out)
			if err := typ.Close(); err != nil {
				t.Fatalf("ObjectType.Close: %v", err)
			}
			if _, err := typ.NewCollection(); err == nil {
				t.Fatal("NewCollection succeeded after ObjectType.Close")
			}
		})
	}
}

func assertVARRAYValues(t *testing.T, collection datatype.ObjectCollection, want ...any) {
	t.Helper()
	gotLen, err := collection.Len()
	if err != nil || gotLen != len(want) {
		t.Fatalf("VARRAY length = %d, %v; want %d", gotLen, err, len(want))
	}
	for i, expected := range want {
		got, err := collection.Get(i + 1)
		if err != nil {
			t.Fatalf("VARRAY element %d: %v", i+1, err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("VARRAY element %d = %#v (%T), want %#v (%T)", i+1, got, got, expected, expected)
		}
	}
}

// TestDriver_VARRAY_GetObjectTypeOtherScalarTypes exercises the public
// GetObjectType API and named collection operations for scalar VARRAY element
// types other than NUMBER and VARCHAR2.
func TestDriver_VARRAY_GetObjectTypeOtherScalarTypes(t *testing.T) {
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

	dateValue := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	timestampValue := time.Date(2026, time.January, 2, 3, 4, 5, 123000000, time.UTC)
	cases := []struct {
		name    string
		element string
		values  []any
		want    []any
		check   func(*testing.T, int, any, any)
	}{
		{name: "raw", element: "RAW(16)", values: []any{[]byte{1, 2, 3}, []byte{0xFE, 0xED}, nil}},
		{name: "date", element: "DATE", values: []any{dateValue, dateValue.AddDate(0, 0, 1)}, check: assertVARRAYTimestamp},
		{name: "timestamp", element: "TIMESTAMP(6)", values: []any{timestampValue, timestampValue.Add(time.Hour)}, check: assertVARRAYTimestamp},
		{name: "binary_float", element: "BINARY_FLOAT", values: []any{float32(1.25), float32(-2.5)}, want: []any{float64(1.25), float64(-2.5)}},
		{name: "binary_double", element: "BINARY_DOUBLE", values: []any{float64(1.25), float64(-2.5)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typeName := createObjectName("varray_" + tc.name)
			typeName = strings.ToUpper(typeName)
			if _, err := db.ExecContext(ctx, "CREATE TYPE "+typeName+" AS VARRAY(5) OF "+tc.element); err != nil {
				t.Fatalf("create %s VARRAY: %v", tc.name, err)
			}
			t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

			// Use the public API with sql.Conn so metadata retrieval and subsequent
			// collection binds use the same physical database connection.
			typ, err := datatype.GetObjectType(ctx, conn, typeName)
			if err != nil {
				t.Fatalf("GetObjectType(%s): %v", typeName, err)
			}
			collection, err := typ.NewCollection()
			if err != nil {
				t.Fatalf("NewCollection(%s): %v", tc.name, err)
			}
			for _, value := range tc.values {
				if err := collection.Append(value); err != nil {
					t.Fatalf("Append(%#v): %v", value, err)
				}
			}
			if got, err := collection.Len(); err != nil || got != len(tc.values) {
				t.Fatalf("Len() = %d, %v; want %d", got, err, len(tc.values))
			}

			var out datatype.ObjectCollection
			if _, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, collection); err != nil {
				t.Fatalf("PL/SQL %s VARRAY IN/OUT: %v", tc.name, err)
			}
			want := tc.values
			if tc.want != nil {
				want = tc.want
			}
			if tc.check != nil {
				for i, expected := range want {
					got, err := out.Get(i + 1)
					if err != nil {
						t.Fatalf("VARRAY element %d: %v", i+1, err)
					}
					tc.check(t, i, got, expected)
				}
			} else {
				assertVARRAYValues(t, out, want...)
			}
		})
	}
}

func assertVARRAYTimestamp(t *testing.T, index int, got, want any) {
	t.Helper()
	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("VARRAY element %d = %#v (%T), want time.Time", index+1, got, got)
	}
	wantTime, ok := want.(time.Time)
	if !ok {
		t.Fatalf("expected VARRAY timestamp %d = %#v (%T), want time.Time", index+1, want, want)
	}
	assertSameYMDHMSNanos(t, index, "VARRAY timestamp", gotTime, wantTime)
}
