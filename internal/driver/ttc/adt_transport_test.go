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
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

type adtMetadataExecer func(context.Context, string, ...any) (sql.Result, error)

func (f adtMetadataExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return f(ctx, query, args...)
}

func setObjectField(t *testing.T, object *datatype.Object, name string, value any) {
	t.Helper()
	field := object.Value.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		t.Fatalf("%s is not a settable reflected object field", name)
	}
	if value == nil {
		field.SetZero()
		return
	}
	field.Set(reflect.ValueOf(value))
}

func objectField(t *testing.T, object datatype.Object, name string) any {
	t.Helper()
	field := object.Value.FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("%s is not a reflected object field", name)
	}
	return field.Interface()
}

func TestObjectCollectionOperations(t *testing.T) {
	typ := &datatype.ObjectType{Name: "NUMBERS", Collection: true, VArray: true, UpperBound: 2}
	c, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set([]int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := c.Set([]int64{1, 2, 3}); err == nil {
		t.Fatal("Set succeeded past VARRAY bound")
	}
	if n, err := c.Len(); err != nil || n != 2 {
		t.Fatalf("Len() = %d, %v", n, err)
	}
	if err := c.Set([]any{int64(1), int64(20)}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Get(); err != nil || !reflect.DeepEqual(got, []any{int64(1), int64(20)}) {
		t.Fatalf("Get() = %#v, %v; want [1 20]", got, err)
	}
}

func TestNamedTypeForZeroCollectionAndOutInference(t *testing.T) {
	if _, ok := namedTypeForBind(datatype.ObjectCollection{}); ok {
		t.Fatal("zero ObjectCollection unexpectedly reported a named type")
	}
	typ := &datatype.ObjectType{Name: "NUMBERS", Collection: true, VArray: true}
	in, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := inferNamedTypeForOut([]sqldriver.Value{
		sql.Out{Dest: &datatype.ObjectCollection{}},
		in,
	}, 0)
	if !ok || got != typ {
		t.Fatalf("inferred type = %v, %v; want %p, true", got, ok, typ)
	}
}

func TestNamedTypeForObjectValue(t *testing.T) {
	typ := &datatype.ObjectType{Name: "SCALAR_OBJECT"}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}

	// normalizeBindValue dereferences *datatype.Object before named-type
	// discovery, so both pointer and value forms must retain the descriptor.
	if got, ok := namedTypeForBind(*object); !ok || got != typ {
		t.Fatalf("named type = %v, %v; want %p, true", got, ok, typ)
	}
	if got, ok := objectForBind(*object); !ok || got.ObjectType != typ {
		t.Fatalf("object bind = %v, %v; want object for %p", got, ok, typ)
	}
}

func TestObjectCollectionNullBind(t *testing.T) {
	typ := &datatype.ObjectType{Name: "NUMBERS", Collection: true, VArray: true}
	c, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	c.SetNull()
	if !c.IsNull() {
		t.Fatal("SetNull did not mark collection NULL")
	}
	if _, ok := namedTypeForBind(c); !ok {
		t.Fatal("NULL collection lost its named type")
	}
	image, err := encodeCollectionImage(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if image != nil {
		t.Fatalf("NULL collection encoded image = %x; want nil", image)
	}
	if err := c.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	if c.IsNull() {
		t.Fatal("Set did not restore non-NULL collection state")
	}
}

func TestRefCursorOutBindUsesCursorOAC(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	var cursor datatype.Cursor
	normalized := normalizeBindValue(sql.Out{Dest: &cursor})
	oac, err := factory.getBindOac(normalized, 0)
	if err != nil {
		t.Fatal(err)
	}
	cursorOAC, ok := oac.(*tTIoac)
	if !ok {
		t.Fatalf("OAC type = %T, want *tTIoac", oac)
	}
	if cursorOAC.dataType != driverCommon.UB1(common.DtyCur) || cursorOAC.maxLength != 4 {
		t.Fatalf("REF CURSOR OAC = dataType %d, maxLength %d; want %d, 4", cursorOAC.dataType, cursorOAC.maxLength, common.DtyCur)
	}
}

func TestNewCollectionBindsUnusedMetadataAsRefCursors(t *testing.T) {
	sentinel := errors.New("stop after checking binds")
	_, err := datatype.NewCollection(context.Background(), adtMetadataExecer(func(_ context.Context, _ string, args ...any) (sql.Result, error) {
		if len(args) != 10 {
			t.Fatalf("bind count = %d, want 10", len(args))
		}
		for _, index := range []int{8, 9} {
			out, ok := args[index].(sql.Out)
			if !ok {
				t.Fatalf("bind %d = %T, want sql.Out", index+1, args[index])
			}
			if _, ok := out.Dest.(*datatype.Cursor); !ok {
				t.Fatalf("bind %d destination = %T, want *datatype.Cursor", index+1, out.Dest)
			}
		}
		return nil, sentinel
	}), "SCHEMA.NUMBER_VARRAY")
	if !errors.Is(err, sentinel) {
		t.Fatalf("NewCollection error = %v, want wrapped sentinel", err)
	}
}

func TestDecodeCollectionImageJDBCHeader(t *testing.T) {
	typ := &datatype.ObjectType{Collection: true, VArray: true, UpperBound: 5, ElementType: common.DtyNum}
	// JDBC's 8.1 image for VARRAY(10, 20, 30): prefix length 3, inline/type-
	// version flag 0x11, type version 1, collection flags 0, and three values.
	image := driverCommon.B1Array{
		0x88, 0x01, 0xFE, 0, 0, 0, 0x16,
		0x03, 0x11, 0x00, 0x01, 0x00, 0x03,
		0x02, 0xC1, 0x0B, 0x02, 0xC1, 0x15, 0x02, 0xC1, 0x1F,
	}
	value, err := decodeCollectionImage(image, typ, NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	if err != nil {
		t.Fatalf("decode JDBC collection image: %v", err)
	}
	collection, ok := value.(datatype.ObjectCollection)
	if !ok {
		t.Fatalf("decoded value = %T, want ObjectCollection", value)
	}
	if got, err := collection.Len(); err != nil || got != 3 {
		t.Fatalf("decoded length = %d, %v; want 3", got, err)
	}
}

func TestNestedTableInlineCollectionImage(t *testing.T) {
	typ := &datatype.ObjectType{Collection: true, ElementType: common.DtyNum, TypeVersion: 1}
	collection, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{int64(10), nil, int64(30)}); err != nil {
		t.Fatal(err)
	}
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeCollectionImage(collection, factory)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCollectionImage(image, typ, factory)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(datatype.ObjectCollection)
	if !ok {
		t.Fatalf("decoded value = %T, want ObjectCollection", decoded)
	}
	if got.VArray || got.IsNull() {
		t.Fatalf("decoded nested table = %#v", got)
	}
	if length, err := got.Len(); err != nil || length != 3 {
		t.Fatalf("nested table length = %d, %v; want 3", length, err)
	}
}

func TestAssociativeCollectionImageRoundTrip(t *testing.T) {
	typ := &datatype.ObjectType{Collection: true, Associative: true, ElementType: common.DtyNum, TypeVersion: 1}
	collection, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	want := map[int32]any{-2: int64(10), 3: nil, 9: int64(30)}
	if err := collection.SetIndexed(want); err != nil {
		t.Fatal(err)
	}
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeCollectionImage(collection, factory)
	if err != nil {
		t.Fatal(err)
	}
	// JDBC sets KOPI20_CF_INDX (0x10) and places a UB4 PLS_INTEGER key
	// immediately before each collection element.
	if len(image) < 17 || image[11] != collectionIndexesFlag || !reflect.DeepEqual([]byte(image[13:17]), []byte{0xFF, 0xFF, 0xFF, 0xFE}) {
		t.Fatalf("associative collection image header = % X", []byte(image))
	}
	decoded, err := decodeCollectionImage(image, typ, factory)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(datatype.ObjectCollection)
	if !ok {
		t.Fatalf("decoded value = %T, want ObjectCollection", decoded)
	}
	indexed, err := got.GetIndexed()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(indexed, want) {
		t.Fatalf("indexed values = %#v, want %#v", indexed, want)
	}
}

func TestDecodeCollectionImageRejectsLocatorBackedNestedTable(t *testing.T) {
	typ := &datatype.ObjectType{Collection: true, ElementType: common.DtyNum}
	// 0x90 is an 8.1 degenerate image header. It carries a locator prefix,
	// not an inline collection data segment.
	_, err := decodeCollectionImage(driverCommon.B1Array{0x90, 0x01, 0x03, 0x01, 0x00}, typ, NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	if err == nil {
		t.Fatal("locator-backed nested table image was decoded as inline data")
	}
}

func TestObjectAttributes(t *testing.T) {
	typ := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{"A": {Name: "A", Sequence: 1}}}
	o, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, o, "A", "x")
	if got := objectField(t, *o, "A"); got != "x" {
		t.Fatalf("A = %v, want x", got)
	}
	if field := o.Value.FieldByName("MISSING"); field.IsValid() {
		t.Fatal("dynamic object exposes an undeclared attribute")
	}
}

func TestObjectImageRoundTrip(t *testing.T) {
	typ := &datatype.ObjectType{
		Name:        "SCALAR_OBJECT",
		TypeVersion: 1,
		Attributes: map[string]datatype.ObjectAttribute{
			"ID": {
				Name:       "ID",
				Sequence:   1,
				ObjectType: &datatype.ObjectType{ElementType: common.DtyNum},
			},
			"NAME": {
				Name:       "NAME",
				Sequence:   2,
				ObjectType: &datatype.ObjectType{ElementType: common.DtyVCS},
			},
		},
	}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, object, "ID", int64(42))
	setObjectField(t, object, "NAME", "forty-two")
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeObjectImage(object, factory)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeObjectImage(image, typ, factory)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(datatype.Object)
	if !ok {
		t.Fatalf("decoded value = %T, want datatype.Object", decoded)
	}
	if value := objectField(t, got, "ID"); value != int64(42) {
		t.Fatalf("ID = %v, want 42", value)
	}
	if value := objectField(t, got, "NAME"); value != "forty-two" {
		t.Fatalf("NAME = %v, want forty-two", value)
	}
}

func TestObjectNullBind(t *testing.T) {
	typ := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	object.SetNull()
	if !object.IsNull() {
		t.Fatal("SetNull did not mark object NULL")
	}
	image, err := encodeObjectImage(object, NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	if err != nil {
		t.Fatal(err)
	}
	if image != nil {
		t.Fatalf("NULL object image = %x; want nil", image)
	}
}

func TestCompositeObjectImageRoundTrip(t *testing.T) {
	childType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"LABEL": {Name: "LABEL", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyVCS}},
	}}
	collectionType := &datatype.ObjectType{Collection: true, VArray: true, UpperBound: 3, ElementType: common.DtyNum, TypeVersion: 1}
	parentType := &datatype.ObjectType{TypeVersion: 1, Attributes: map[string]datatype.ObjectAttribute{
		"CHILD":   {Name: "CHILD", Sequence: 1, ObjectType: childType},
		"NUMBERS": {Name: "NUMBERS", Sequence: 2, ObjectType: collectionType},
	}}
	child, err := childType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, child, "LABEL", "nested")
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{int64(7)}); err != nil {
		t.Fatal(err)
	}
	parent, err := parentType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, parent, "CHILD", child)
	setObjectField(t, parent, "NUMBERS", collection)
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeObjectImage(parent, factory)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeObjectImage(image, parentType, factory)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(datatype.Object)
	nestedValue := objectField(t, got, "CHILD")
	nested, ok := nestedValue.(datatype.Object)
	if !ok {
		t.Fatalf("CHILD = %T, want datatype.Object", nestedValue)
	}
	if label := objectField(t, nested, "LABEL"); label != "nested" {
		t.Fatalf("CHILD.LABEL = %v, want nested", label)
	}
	numbersValue := objectField(t, got, "NUMBERS")
	numbers, ok := numbersValue.(datatype.ObjectCollection)
	if !ok {
		t.Fatalf("NUMBERS = %T, want ObjectCollection", numbersValue)
	}
	if values, err := numbers.Get(); err != nil || len(values) != 1 || values[0] != int64(7) {
		t.Fatalf("NUMBERS.Get() = %#v, %v; want [7]", values, err)
	}
}

func TestCompositeObjectImageNullMembers(t *testing.T) {
	childType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"LABEL": {Name: "LABEL", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyVCS}},
	}}
	collectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum, TypeVersion: 1}
	parentType := &datatype.ObjectType{TypeVersion: 1, Attributes: map[string]datatype.ObjectAttribute{
		"CHILD":   {Name: "CHILD", Sequence: 1, ObjectType: childType},
		"NUMBERS": {Name: "NUMBERS", Sequence: 2, ObjectType: collectionType},
	}}
	child, err := childType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	child.SetNull()
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	collection.SetNull()
	parent, err := parentType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, parent, "CHILD", child)
	setObjectField(t, parent, "NUMBERS", collection)
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeObjectImage(parent, factory)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeObjectImage(image, parentType, factory)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(datatype.Object)
	childValue := objectField(t, got, "CHILD")
	numbersValue := objectField(t, got, "NUMBERS")
	if childValue != nil || numbersValue != nil {
		t.Fatalf("NULL members decoded as CHILD=%#v, NUMBERS=%#v", childValue, numbersValue)
	}
}

func TestObjectCollectionImageRoundTrip(t *testing.T) {
	elementType := &datatype.ObjectType{TypeVersion: 1, Attributes: map[string]datatype.ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	collectionType := &datatype.ObjectType{Collection: true, VArray: true, UpperBound: 3, TypeVersion: 1, CollectionOf: elementType}
	first, err := elementType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, first, "ID", int64(1))
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{first, nil}); err != nil {
		t.Fatal(err)
	}
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeCollectionImage(collection, factory)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCollectionImage(image, collectionType, factory)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(datatype.ObjectCollection)
	values, err := got.Get()
	if err != nil {
		t.Fatal(err)
	}
	value := values[0]
	object, ok := value.(datatype.Object)
	if !ok {
		t.Fatalf("element = %T, want datatype.Object", value)
	}
	if id := objectField(t, object, "ID"); id != int64(1) {
		t.Fatalf("element.ID = %v, want 1", id)
	}
	if len(values) != 2 || values[1] != nil {
		t.Fatalf("element 2 = %#v, want nil", values)
	}
}

func TestPolymorphicObjectImageRoundTrip(t *testing.T) {
	baseType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	subtype := &datatype.ObjectType{TypeVersion: 1, SuperTypeName: "OWNER.BASE", TOID: make([]byte, 16), Attributes: map[string]datatype.ObjectAttribute{
		"ID":    {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
		"EXTRA": {Name: "EXTRA", Sequence: 2, ObjectType: &datatype.ObjectType{ElementType: common.DtyVCS}},
	}}
	for i := range subtype.TOID {
		subtype.TOID[i] = byte(i + 1)
	}
	baseType.SubTypes = []*datatype.ObjectType{subtype}
	object, err := subtype.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, object, "ID", int64(1))
	setObjectField(t, object, "EXTRA", "derived")
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeObjectImage(object, factory)
	if err != nil {
		t.Fatal(err)
	}
	if image[0] != 0x80 {
		t.Fatalf("polymorphic image flags = %x, want 80", image[0])
	}
	decoded, err := decodeObjectImage(image, baseType, factory)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(datatype.Object)
	if got.ObjectType != subtype {
		t.Fatalf("decoded type = %p, want subtype %p", got.ObjectType, subtype)
	}
	if value := objectField(t, got, "EXTRA"); value != "derived" {
		t.Fatalf("EXTRA = %v, want derived", value)
	}
}

func TestNamedTypeOACCarriesTOIDAndVersion(t *testing.T) {
	typ := &datatype.ObjectType{Name: "NUMBERS", TOID: make([]byte, 16), TypeVersion: 7}
	oac, err := newTTIOacNamedType(typ, 128)
	if err != nil {
		t.Fatal(err)
	}
	if oac.dataType != driverCommon.UB1(common.DtyNty) {
		t.Fatalf("dataType = %d, want %d", oac.dataType, common.DtyNty)
	}
	if oac.versionNumber != 7 || oac.toid == nil || len(*oac.toid) != 16 {
		t.Fatalf("missing named-type metadata: %#v", oac)
	}
}
