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
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

func TestDriver_ObjectTypeMetadata(t *testing.T) {
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
		t.Fatalf("acquire test connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	typeName := strings.ToUpper(createObjectName("object_metadata"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+typeName+" AS OBJECT (ID NUMBER, NAME VARCHAR2(20), CREATED_AT TIMESTAMP(9))"); err != nil {
		t.Fatalf("create object type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

	typ, err := datatype.GetObjectType(ctx, conn, typeName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", typeName, err)
	}
	if !typ.IsObject() {
		t.Fatalf("descriptor is not an object: %#v", typ)
	}
	if got, want := typ.AttributeNames(), []string{"ID", "NAME", "CREATED_AT"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("AttributeNames() = %v, want %v", got, want)
	}
	if got := typ.Attributes["ID"].ObjectType; got == nil || got.ElementType != common.DtyNum {
		t.Fatalf("ID metadata = %#v, want NUMBER", got)
	}
	if got := typ.Attributes["NAME"].ObjectType; got == nil || got.ElementType != common.DtyVCS || got.ElementSize != 20 {
		t.Fatalf("NAME metadata = %#v, want VARCHAR2(20)", got)
	}
	if got := typ.Attributes["CREATED_AT"].ObjectType; got == nil || got.ElementType != common.DtyStamp || got.FsPrecision != 9 {
		t.Fatalf("CREATED_AT metadata = %#v, want TIMESTAMP(9)", got)
	}
}

func TestDriver_ObjectTypeScalarPLSQL(t *testing.T) {
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
		t.Fatalf("acquire test connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	typeName := strings.ToUpper(createObjectName("object_scalar_plsql"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+typeName+" AS OBJECT (ID NUMBER, NAME VARCHAR2(20), CREATED_AT TIMESTAMP(9))"); err != nil {
		t.Fatalf("create object type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

	typ, err := datatype.GetObjectType(ctx, conn, typeName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", typeName, err)
	}
	in, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	wantTime := time.Date(2025, 4, 5, 6, 7, 8, 123456789, time.UTC)
	for name, value := range map[string]any{"ID": int64(42), "NAME": "answer", "CREATED_AT": wantTime} {
		if err := in.Set(name, value); err != nil {
			t.Fatalf("Set(%s): %v", name, err)
		}
	}
	var out datatype.Object
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL object IN/OUT: %v", err)
	}
	if out.IsNull() {
		t.Fatal("PL/SQL object IN/OUT returned NULL")
	}
	if got, err := out.Get("ID"); err != nil || got != int64(42) {
		t.Fatalf("ID = %v, %v; want 42, nil", got, err)
	}
	if got, err := out.Get("NAME"); err != nil || got != "answer" {
		t.Fatalf("NAME = %v, %v; want answer, nil", got, err)
	}
	gotTime, ok := out.Attributes["CREATED_AT"].(time.Time)
	if !ok {
		t.Fatalf("CREATED_AT = %T, want time.Time", out.Attributes["CREATED_AT"])
	}
	assertSameYMDHMSNanos(t, 0, "object TIMESTAMP", gotTime, wantTime)

	in.SetNull()
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL NULL object IN/OUT: %v", err)
	}
	if !out.IsNull() {
		t.Fatal("PL/SQL NULL object IN/OUT did not return NULL")
	}
}

func TestDriver_CompositeObjectPLSQL(t *testing.T) {
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
		t.Fatalf("acquire test connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	collectionName := strings.ToUpper(createObjectName("object_numbers"))
	childName := strings.ToUpper(createObjectName("object_child"))
	parentName := strings.ToUpper(createObjectName("object_parent"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+collectionName+" AS VARRAY(3) OF NUMBER"); err != nil {
		t.Fatalf("create collection type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+collectionName+" FORCE") })
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+childName+" AS OBJECT (LABEL VARCHAR2(20))"); err != nil {
		t.Fatalf("create child type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+childName+" FORCE") })
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+parentName+" AS OBJECT (CHILD "+childName+", NUMBERS "+collectionName+")"); err != nil {
		t.Fatalf("create parent type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+parentName+" FORCE") })

	parentType, err := datatype.GetObjectType(ctx, conn, parentName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", parentName, err)
	}
	childType := parentType.Attributes["CHILD"].ObjectType
	collectionType := parentType.Attributes["NUMBERS"].ObjectType
	if childType == nil || len(childType.Attributes) != 1 || collectionType == nil || !collectionType.Collection {
		t.Fatalf("composite metadata: child=%#v collection=%#v", childType, collectionType)
	}
	child, err := childType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Set("LABEL", "nested"); err != nil {
		t.Fatal(err)
	}
	numbers, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := numbers.Append(int64(7)); err != nil {
		t.Fatal(err)
	}
	parent, err := parentType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Set("CHILD", child); err != nil {
		t.Fatal(err)
	}
	if err := parent.Set("NUMBERS", numbers); err != nil {
		t.Fatal(err)
	}
	var out datatype.Object
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, parent); err != nil {
		t.Fatalf("PL/SQL composite object IN/OUT: %v", err)
	}
	nested, ok := out.Attributes["CHILD"].(datatype.Object)
	if !ok {
		t.Fatalf("CHILD = %T, want datatype.Object", out.Attributes["CHILD"])
	}
	if label, err := nested.Get("LABEL"); err != nil || label != "nested" {
		t.Fatalf("CHILD.LABEL = %v, %v; want nested, nil", label, err)
	}
	gotNumbers, ok := out.Attributes["NUMBERS"].(datatype.ObjectCollection)
	if !ok {
		t.Fatalf("NUMBERS = %T, want ObjectCollection", out.Attributes["NUMBERS"])
	}
	if value, err := gotNumbers.Get(1); err != nil || value != int64(7) {
		t.Fatalf("NUMBERS[1] = %v, %v; want 7, nil", value, err)
	}
}

func TestDriver_ObjectVARRAYPLSQL(t *testing.T) {
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
		t.Fatalf("acquire test connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	elementName := strings.ToUpper(createObjectName("varray_element"))
	collectionName := strings.ToUpper(createObjectName("object_varray"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+elementName+" AS OBJECT (ID NUMBER)"); err != nil {
		t.Fatalf("create element type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+elementName+" FORCE") })
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+collectionName+" AS VARRAY(3) OF "+elementName); err != nil {
		t.Fatalf("create object VARRAY type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+collectionName+" FORCE") })

	collectionType, err := datatype.GetObjectType(ctx, conn, collectionName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", collectionName, err)
	}
	if collectionType.CollectionOf == nil || len(collectionType.CollectionOf.Attributes) != 1 {
		t.Fatalf("object collection metadata = %#v", collectionType.CollectionOf)
	}
	element, err := collectionType.CollectionOf.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if err := element.Set("ID", int64(1)); err != nil {
		t.Fatal(err)
	}
	in, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Append(element); err != nil {
		t.Fatal(err)
	}
	if err := in.Append(nil); err != nil {
		t.Fatal(err)
	}
	var out datatype.ObjectCollection
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL object VARRAY IN/OUT: %v", err)
	}
	value, err := out.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := value.(datatype.Object)
	if !ok {
		t.Fatalf("element = %T, want datatype.Object", value)
	}
	if id, err := got.Get("ID"); err != nil || id != int64(1) {
		t.Fatalf("element.ID = %v, %v; want 1, nil", id, err)
	}
	if value, err := out.Get(2); err != nil || value != nil {
		t.Fatalf("element 2 = %v, %v; want nil, nil", value, err)
	}
}

func TestDriver_ObjectTypeHierarchyMetadata(t *testing.T) {
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
		t.Fatalf("acquire test connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	baseName := strings.ToUpper(createObjectName("object_base"))
	subtypeName := strings.ToUpper(createObjectName("object_subtype"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+baseName+" AS OBJECT (ID NUMBER) NOT FINAL"); err != nil {
		t.Fatalf("create base type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+baseName+" FORCE") })
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+subtypeName+" UNDER "+baseName+" (EXTRA VARCHAR2(20))"); err != nil {
		t.Fatalf("create subtype: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+subtypeName+" FORCE") })

	base, err := datatype.GetObjectType(ctx, conn, baseName)
	if err != nil {
		t.Fatalf("GetObjectType(%s): %v", baseName, err)
	}
	if len(base.SubTypes) != 1 {
		t.Fatalf("subtypes = %d, want 1", len(base.SubTypes))
	}
	subtype := base.SubTypes[0]
	if subtype.SuperTypeName == "" || subtype.SubTypeForTOID(subtype.TOID) != nil {
		t.Fatalf("subtype metadata = %#v", subtype)
	}
	if base.SubTypeForTOID(subtype.TOID) != subtype {
		t.Fatal("base descriptor did not resolve subtype TOID")
	}
}
