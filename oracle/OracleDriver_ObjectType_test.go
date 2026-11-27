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

func setObjectValue(t *testing.T, object *datatype.Object, name string, value any) {
	t.Helper()
	object.Attributes[name] = value
}

func objectValue(t *testing.T, object datatype.Object, name string) any {
	t.Helper()
	return object.Attributes[name]
}

func TestDriver_ObjectTypeMapValue(t *testing.T) {
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

	typeName := strings.ToUpper(createObjectName("object_map_value"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+typeName+" AS OBJECT (ID NUMBER, NAME VARCHAR2(50), CREATED_AT TIMESTAMP(9))"); err != nil {
		t.Fatalf("create object type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })

	wantTime := time.Date(2026, 9, 22, 10, 30, 0, 123456789, time.UTC)
	object := datatype.NewObject(typeName, map[string]any{"ID": int64(42), "NAME": "Ada"})
	object.Attributes["CREATED_AT"] = wantTime

	var out datatype.Object
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, object); err != nil {
		t.Fatalf("PL/SQL object IN/OUT: %v", err)
	}
	if got := out.Attributes["ID"]; got != int64(42) {
		t.Fatalf("ID = %v, want 42", got)
	}
	if got := out.Attributes["NAME"]; got != "Ada" {
		t.Fatalf("NAME = %v, want Ada", got)
	}
	gotTime, ok := out.Attributes["CREATED_AT"].(time.Time)
	if !ok {
		t.Fatalf("CREATED_AT = %T, want time.Time", out.Attributes["CREATED_AT"])
	}
	assertSameYMDHMSNanos(t, 0, "map-backed object TIMESTAMP", gotTime, wantTime)
}

// TestDriver_ObjectTypeTableColumn verifies that an object can be bound into a
// relational table column and decoded back from a SELECT on the same physical
// connection that owns its ADT metadata.
func TestDriver_ObjectTypeTableColumn(t *testing.T) {
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

	typeName := strings.ToUpper(createObjectName("object_table_column"))
	tableName := strings.ToUpper(createObjectName("object_table"))
	if _, err = conn.ExecContext(ctx, "CREATE TYPE "+typeName+" AS OBJECT (ID NUMBER, NAME VARCHAR2(50), CREATED_AT TIMESTAMP(9))"); err != nil {
		t.Fatalf("create object type: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TYPE "+typeName+" FORCE") })
	if _, err = conn.ExecContext(ctx, "CREATE TABLE "+tableName+" (ROW_ID NUMBER PRIMARY KEY, OBJECT_COL "+typeName+")"); err != nil {
		t.Fatalf("create object table: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP TABLE "+tableName) })

	var cold datatype.Object
	if err := conn.QueryRowContext(ctx, "SELECT "+typeName+"(7,'cold',NULL) FROM dual").Scan(&cold); err != nil || cold.Attributes["ID"] != int64(7) {
		t.Fatalf("select without previous ADT bind: %#v, %v", cold.Attributes, err)
	}
	in := datatype.NewObject(typeName)
	wantTime := time.Date(2026, 9, 30, 11, 12, 13, 987654321, time.UTC)
	setObjectValue(t, in, "ID", int64(42))
	setObjectValue(t, in, "NAME", "stored object")
	setObjectValue(t, in, "CREATED_AT", wantTime)
	if _, err = conn.ExecContext(ctx, "INSERT INTO "+tableName+" (ROW_ID, OBJECT_COL) VALUES (:1, :2)", int64(1), in); err != nil {
		t.Fatalf("insert object table row: %v", err)
	}

	var out datatype.Object
	if err = conn.QueryRowContext(ctx, "SELECT OBJECT_COL FROM "+tableName+" WHERE ROW_ID = :1", int64(1)).Scan(&out); err != nil {
		t.Fatalf("select object table row: %v", err)
	}
	if out.IsNull() {
		t.Fatal("selected object is NULL")
	}
	if got := objectValue(t, out, "ID"); got != int64(42) {
		t.Fatalf("ID = %v, want 42", got)
	}
	if got := objectValue(t, out, "NAME"); got != "stored object" {
		t.Fatalf("NAME = %v, want stored object", got)
	}
	gotTime, ok := objectValue(t, out, "CREATED_AT").(time.Time)
	if !ok {
		t.Fatalf("CREATED_AT = %T, want time.Time", objectValue(t, out, "CREATED_AT"))
	}
	assertSameYMDHMSNanos(t, 0, "object table TIMESTAMP", gotTime, wantTime)
}

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

	object := datatype.NewObject(typeName)
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :1; END;", sql.Out{Dest: object, In: true}); err != nil {
		t.Fatalf("resolve object metadata: %v", err)
	}
	typ := object.ObjectType
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

	in := datatype.NewObject(typeName)
	wantTime := time.Date(2025, 4, 5, 6, 7, 8, 123456789, time.UTC)
	for name, value := range map[string]any{"ID": int64(42), "NAME": "answer", "CREATED_AT": wantTime} {
		setObjectValue(t, in, name, value)
	}
	var out datatype.Object
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL object IN/OUT: %v", err)
	}
	if out.IsNull() {
		t.Fatal("PL/SQL object IN/OUT returned NULL")
	}
	if got := objectValue(t, out, "ID"); got != int64(42) {
		t.Fatalf("ID = %v, want 42", got)
	}
	if got := objectValue(t, out, "NAME"); got != "answer" {
		t.Fatalf("NAME = %v, want answer", got)
	}
	createdAt := objectValue(t, out, "CREATED_AT")
	gotTime, ok := createdAt.(time.Time)
	if !ok {
		t.Fatalf("CREATED_AT = %T, want time.Time", createdAt)
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

	parent := datatype.NewObject(parentName)
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :1; END;", sql.Out{Dest: parent, In: true}); err != nil {
		t.Fatalf("resolve composite metadata: %v", err)
	}
	parentType := parent.ObjectType
	childType := parentType.Attributes["CHILD"].ObjectType
	collectionType := parentType.Attributes["NUMBERS"].ObjectType
	if childType == nil || len(childType.Attributes) != 1 || collectionType == nil || !collectionType.Collection {
		t.Fatalf("composite metadata: child=%#v collection=%#v", childType, collectionType)
	}
	child, err := childType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectValue(t, child, "LABEL", "nested")
	numbers, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := numbers.Set([]any{int64(7)}); err != nil {
		t.Fatal(err)
	}
	setObjectValue(t, parent, "CHILD", child)
	setObjectValue(t, parent, "NUMBERS", numbers)
	var out datatype.Object
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, parent); err != nil {
		t.Fatalf("PL/SQL composite object IN/OUT: %v", err)
	}
	nestedValue := objectValue(t, out, "CHILD")
	nested, ok := nestedValue.(datatype.Object)
	if !ok {
		t.Fatalf("CHILD = %T, want datatype.Object", nestedValue)
	}
	if label := objectValue(t, nested, "LABEL"); label != "nested" {
		t.Fatalf("CHILD.LABEL = %v, want nested", label)
	}
	numbersValue := objectValue(t, out, "NUMBERS")
	gotNumbers, ok := numbersValue.(datatype.ObjectCollection)
	if !ok {
		t.Fatalf("NUMBERS = %T, want ObjectCollection", numbersValue)
	}
	if values, err := gotNumbers.Get(); err != nil || len(values) != 1 || values[0] != int64(7) {
		t.Fatalf("NUMBERS.Get() = %#v, %v; want [7]", values, err)
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

	in := datatype.NewCollection(collectionName)
	element := datatype.NewObject(elementName, map[string]any{"ID": int64(1)})
	if err := in.Set([]any{element, nil}); err != nil {
		t.Fatal(err)
	}
	var out datatype.ObjectCollection
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, in); err != nil {
		t.Fatalf("PL/SQL object VARRAY IN/OUT: %v", err)
	}
	values, err := out.Get()
	if err != nil {
		t.Fatal(err)
	}
	value := values[0]
	got, ok := value.(datatype.Object)
	if !ok {
		t.Fatalf("element = %T, want datatype.Object", value)
	}
	if id := got.Attributes["F_1"]; id != int64(1) {
		t.Fatalf("element field F_1 = %v, want 1", id)
	}
	if len(values) != 2 || values[1] != nil {
		t.Fatalf("element 2 = %#v, want nil", values)
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

	baseValue := datatype.NewObject(baseName)
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := :1; END;", sql.Out{Dest: baseValue, In: true}); err != nil {
		t.Fatalf("resolve hierarchy metadata: %v", err)
	}
	if _, err = conn.ExecContext(ctx, "BEGIN :1 := "+subtypeName+"(42, 'child'); END;", sql.Out{Dest: baseValue}); err != nil {
		t.Fatalf("fetch subtype through base OUT bind: %v", err)
	}
	if baseValue.Name != subtypeName || baseValue.Attributes["ID"] != int64(42) || baseValue.Attributes["EXTRA"] != "child" {
		t.Fatalf("decoded subtype %q: %#v", baseValue.Name, baseValue.Attributes)
	}
}
