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
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

type adtMetadataExecer func(context.Context, string, ...any) (sql.Result, error)

func (f adtMetadataExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return f(ctx, query, args...)
}

type adtCodecFactoryStub struct {
	encoderErr error
	decoderErr error
	encoder    *typeEncoder
	decoder    *typeDecoder
}

func (f adtCodecFactoryStub) getEncoderForGoType(bindValueMetadata) (*typeEncoder, error) {
	return nil, f.encoderErr
}

func (f adtCodecFactoryStub) getEncoderForDtyType(common.DtyType, bindValueMetadata) (*typeEncoder, error) {
	return f.encoder, f.encoderErr
}

func (f adtCodecFactoryStub) getDecoder(common.DtyType) (*typeDecoder, error) {
	return f.decoder, f.decoderErr
}

func (adtCodecFactoryStub) getBindOac(bindValueMetadata, driverCommon.UB4) (driverCommon.Marshallable, error) {
	return nil, nil
}

func (adtCodecFactoryStub) getDefineOac(common.DtyType, columnContext, driverCommon.DriverProperties) driverCommon.Marshallable {
	return nil
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
	metadata := extractBindMetadata([]sqldriver.Value{
		sql.Out{Dest: &datatype.ObjectCollection{}},
		in,
	})
	if metadata[0].objectType != typ {
		t.Fatalf("inferred type = %v; want %p", metadata[0].objectType, typ)
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
	metadata := extractBindValueMetadata(sql.Out{Dest: &cursor})
	oac, err := factory.getBindOac(metadata, 0)
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
	encoder, err := factory.getEncoderForGoType(metadata)
	if err != nil {
		t.Fatal(err)
	}
	buffer, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 8)
	if err := encoder.wrapColumn(context.Background(), mar, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := buffer.bytes[:buffer.currentWritePosition], []byte{1, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("REF CURSOR RXD bind framing = %x, want %x", got, want)
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

func TestObjectImageUsesDeclaredTimestampEncoder(t *testing.T) {
	typ := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"CREATED_AT": {
			Name:       "CREATED_AT",
			Sequence:   1,
			ObjectType: &datatype.ObjectType{ElementType: common.DtyStamp},
		},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 17, 14, 40, 56, 987654321, time.UTC)
	setObjectField(t, object, "CREATED_AT", want)

	image, err := encodeObjectImage(object, NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	if err != nil {
		t.Fatal(err)
	}
	// The seven-byte object header is followed by the attribute pickle length.
	// TIMESTAMP is an eleven-byte value; a generic time.Time bind would instead
	// incorrectly produce the thirteen-byte TIMESTAMP WITH TIME ZONE payload.
	if len(image) != 19 || image[7] != 11 {
		t.Fatalf("TIMESTAMP object image = %x; want 7-byte header plus 11-byte value", image)
	}

	decoded, err := decodeObjectImage(image, typ, NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := objectField(t, decoded.(datatype.Object), "CREATED_AT").(time.Time)
	if !ok {
		t.Fatalf("decoded CREATED_AT = %T, want time.Time", objectField(t, decoded.(datatype.Object), "CREATED_AT"))
	}
	if got.Year() != want.Year() || got.Month() != want.Month() || got.Day() != want.Day() ||
		got.Hour() != want.Hour() || got.Minute() != want.Minute() || got.Second() != want.Second() ||
		got.Nanosecond() != want.Nanosecond() {
		t.Fatalf("decoded CREATED_AT = %s, want wall time %s", got, want)
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
	// The CHILD record occupies its six-byte string pickle at offset seven.
	// NUMBERS is a UPT collection attribute, so it must then start with the
	// pickle length of its complete collection image before the 0x88 image flag.
	if len(image) < 16 || image[14] != 16 || image[15] != 0x88 {
		t.Fatalf("embedded collection record = %x; want UPT length followed by collection image", []byte(image))
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
	// A named object element is a UPT record: its complete object image follows
	// a pickle length in the collection data segment.
	if len(image) < 15 || image[13] != 10 || image[14] != 0x84 {
		t.Fatalf("object collection element = %x; want UPT length followed by object image", []byte(image))
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
	if oac.characterSetID != namedTypeCharsetID {
		t.Fatalf("characterSetID = %d, want %d", oac.characterSetID, namedTypeCharsetID)
	}
}

func TestDecodeOutColumnUsesRegisteredNamedTypeDecoder(t *testing.T) {
	typ := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, object, "ID", int64(42))
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	image, err := encodeObjectImage(object, factory)
	if err != nil {
		t.Fatal(err)
	}
	column := columnContext{DataType: common.DtyNty, objectType: typ}
	value, err := decodeOutColumn(factory, column, columnPayload{data: image})
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := value.(datatype.Object)
	if !ok || objectField(t, decoded, "ID") != int64(42) {
		t.Fatalf("decoded named OUT value = %#v", value)
	}

	value, err = decodeOutColumn(factory, column, columnPayload{})
	if err != nil {
		t.Fatal(err)
	}
	nullObject, ok := value.(datatype.Object)
	if !ok || !nullObject.IsNull() || nullObject.ObjectType != typ {
		t.Fatalf("decoded NULL named OUT value = %#v", value)
	}

	collectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	value, err = decodeOutColumn(factory, columnContext{DataType: common.DtyNty, objectType: collectionType}, columnPayload{})
	if err != nil {
		t.Fatal(err)
	}
	nullCollection, ok := value.(datatype.ObjectCollection)
	if !ok || !nullCollection.IsNull() || nullCollection.ObjectType != collectionType {
		t.Fatalf("decoded NULL named collection OUT value = %#v", value)
	}
}

func TestADTTransportBindHelpersAndValidation(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	objectType := &datatype.ObjectType{TOID: make([]byte, 16), Attributes: map[string]datatype.ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	collectionType := &datatype.ObjectType{TOID: make([]byte, 16), Collection: true, VArray: true, ElementType: common.DtyNum}
	object, err := objectType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, object, "ID", int64(1))
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}

	for _, typ := range []*datatype.ObjectType{nil, {}, {TOID: []byte{1}}, {TOID: make([]byte, 16), TypeVersion: -1}, {TOID: make([]byte, 16), TypeVersion: 1 << 16}} {
		if _, err := newTTIOacNamedType(typ, 0); err == nil {
			t.Fatalf("newTTIOacNamedType(%#v) succeeded", typ)
		}
	}
	oac, err := newTTIOacNamedType(objectType, 99)
	if err != nil {
		t.Fatal(err)
	}
	objectType.TOID[0] = 99
	if (*oac.toid)[0] == 99 {
		t.Fatal("named type OAC aliases descriptor TOID")
	}
	if _, err := newNamedTypeBindOAC(bindValueMetadata{}, 0); err == nil {
		t.Fatal("newNamedTypeBindOAC without metadata succeeded")
	}
	if _, err := newNamedTypeBindOAC(bindValueMetadata{objectType: objectType}, 0); err != nil {
		t.Fatal(err)
	}

	if _, ok := namedTypeForBind((*datatype.Object)(nil)); ok {
		t.Fatal("nil object reported named metadata")
	}
	if _, ok := namedTypeForBind((*datatype.ObjectCollection)(nil)); ok {
		t.Fatal("nil collection reported named metadata")
	}
	if got, ok := namedTypeForBind(collection); !ok || got != collectionType {
		t.Fatalf("collection named type = %p, %v", got, ok)
	}
	if got, ok := namedTypeForBind(&collection); !ok || got != collectionType {
		t.Fatalf("collection pointer named type = %p, %v", got, ok)
	}
	if got, ok := namedTypeForBind(object); !ok || got != objectType {
		t.Fatalf("object named type = %p, %v", got, ok)
	}
	if _, ok := collectionForBind("not a collection"); ok {
		t.Fatal("non-collection accepted")
	}
	if got, ok := collectionForBind(&collection); !ok || got.ObjectType != collectionType {
		t.Fatalf("collection pointer = %#v, %v", got, ok)
	}
	if _, ok := objectForBind(collection); ok {
		t.Fatal("collection accepted as object")
	}
	if got, ok := objectForBind(*object); !ok || got.ObjectType != objectType {
		t.Fatalf("object value = %#v, %v", got, ok)
	}
	if _, ok := objectForBind("not an object"); ok {
		t.Fatal("non-object accepted")
	}

	if image, err := encodeObjectCollectionBind("not a collection", factory); err != nil || image != nil {
		t.Fatalf("invalid collection bind = %x, %v", image, err)
	}
	if image, err := encodeObjectBind("not an object", factory); err != nil || image != nil {
		t.Fatalf("invalid object bind = %x, %v", image, err)
	}
	if image, err := encodeObjectCollectionBind(collection, factory); err != nil || len(image) == 0 {
		t.Fatalf("collection bind = %x, %v", image, err)
	}
	if image, err := encodeObjectBind(*object, factory); err != nil || len(image) == 0 {
		t.Fatalf("object bind = %x, %v", image, err)
	}

	metadata := []bindValueMetadata{
		{goType: reflect.TypeFor[datatype.Object](), isOut: true},
		{objectType: objectType},
		{goType: reflect.TypeFor[string](), isOut: true},
		{goType: reflect.TypeFor[datatype.ObjectCollection](), isOut: true},
	}
	resolveNamedTypeMetadata(metadata)
	if metadata[0].objectType != objectType || metadata[2].objectType != nil || metadata[3].objectType != objectType {
		t.Fatalf("resolved metadata = %#v", metadata)
	}
	resolveNamedTypeMetadata([]bindValueMetadata{{goType: reflect.TypeFor[datatype.Object](), isOut: true}})
	if isNamedTypeGoType(reflect.TypeFor[string]()) {
		t.Fatal("string identified as named type")
	}
}

func TestADTTransportPickleAndMalformedImages(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	collectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	image, err := encodeCollectionImage(collection, factory)
	if err != nil {
		t.Fatal(err)
	}
	if got := appendPickleLength(nil, 245); !reflect.DeepEqual(got, []byte{245}) {
		t.Fatalf("short pickle = %x", got)
	}
	if got := appendPickleLength(nil, 246); !reflect.DeepEqual(got, []byte{254, 0, 0, 0, 246}) {
		t.Fatalf("long pickle = %x", got)
	}
	if length, pos, err := readPickleLength([]byte{245}, 0); err != nil || length != 245 || pos != 1 {
		t.Fatalf("short pickle = %d, %d, %v", length, pos, err)
	}
	if length, pos, err := readPickleLength([]byte{254, 0, 0, 1, 0}, 0); err != nil || length != 256 || pos != 5 {
		t.Fatalf("long pickle = %d, %d, %v", length, pos, err)
	}
	for _, data := range [][]byte{nil, {255}, {254, 0, 0, 0}} {
		if _, _, err := readPickleLength(data, 0); err == nil {
			t.Fatalf("readPickleLength(%x) succeeded", data)
		}
	}
	if index, pos, err := readCollectionIndex(driverCommon.B1Array{255, 255, 255, 254}, 0); err != nil || index != -2 || pos != 4 {
		t.Fatalf("collection index = %d, %d, %v", index, pos, err)
	}
	if _, _, err := readCollectionIndex(nil, 0); err == nil {
		t.Fatal("truncated collection index succeeded")
	}
	if end, err := namedImageEnd(image, 0); err != nil || end != len(image) {
		t.Fatalf("namedImageEnd = %d, %v", end, err)
	}
	for _, malformed := range []driverCommon.B1Array{nil, {0x01, 0, 0}, {0x88, 1, 4}, {0x88, 1, 254, 0, 0, 0, 255}} {
		if _, err := namedImageEnd(malformed, 0); err == nil {
			t.Fatalf("namedImageEnd(%x) succeeded", malformed)
		}
	}
	for _, mutate := range []func(driverCommon.B1Array){
		func(data driverCommon.B1Array) { data[0] = 0 },
		func(data driverCommon.B1Array) { data[0] = 0x90 },
		func(data driverCommon.B1Array) { data[0] = 0x80 },
		func(data driverCommon.B1Array) { data[3]++ },
		func(data driverCommon.B1Array) { data[8] = 0 },
		func(data driverCommon.B1Array) { data[11] = collectionNoLengthFlag },
	} {
		bad := append(driverCommon.B1Array(nil), image...)
		mutate(bad)
		if _, err := decodeCollectionImage(bad, collectionType, factory); err == nil {
			t.Fatalf("malformed collection image accepted: %x", bad)
		}
	}
	if _, err := decodeCollectionImage(image, nil, factory); err == nil {
		t.Fatal("nil collection type accepted")
	}
	if _, err := decodeCollectionImage(image, &datatype.ObjectType{}, factory); err == nil {
		t.Fatal("object type accepted as collection")
	}
}

func TestADTTransportObjectValidation(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	if _, err := encodeObjectImage(nil, factory); err == nil {
		t.Fatal("nil object encoded")
	}
	closed := &datatype.Object{Closed: true}
	if _, err := encodeObjectImage(closed, factory); err == nil {
		t.Fatal("closed object encoded")
	}
	collectionType := &datatype.ObjectType{Collection: true}
	collectionObject, err := collectionType.NewObject()
	if err == nil || collectionObject != nil {
		t.Fatal("collection descriptor created an object")
	}
	typ := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, object, "ID", int64(1))
	if _, err := encodeObjectImage(object, nil); err == nil {
		t.Fatal("object encoded without codec factory")
	}
	polymorphic := &datatype.ObjectType{TypeVersion: 2, Attributes: typ.Attributes}
	polyObject, err := polymorphic.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, polyObject, "ID", int64(1))
	if _, err := encodeObjectImage(polyObject, factory); err == nil {
		t.Fatal("polymorphic object without TOID encoded")
	}

	image, err := encodeObjectImage(object, factory)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []driverCommon.B1Array{nil, {0x01, 0, 0}, {0x84, 1, 4}, {0x84, 1, 254, 0, 0, 0, 255}, append(driverCommon.B1Array(nil), image[:len(image)-1]...)} {
		if _, err := decodeObjectImage(bad, typ, factory); err == nil {
			t.Fatalf("malformed object image accepted: %x", bad)
		}
	}
	if _, err := decodeObjectImage(image, nil, factory); err == nil {
		t.Fatal("nil object type accepted")
	}
	if _, err := decodeObjectImage(image, typ, nil); err == nil {
		t.Fatal("nil codec factory accepted")
	}
	if err := setReflectedObjectField(nil, 0, nil); err == nil {
		t.Fatal("nil object field target accepted")
	}
	if err := setReflectedObjectField(object, -1, nil); err == nil {
		t.Fatal("negative object field accepted")
	}
	if err := setReflectedObjectField(object, 0, "value"); err != nil {
		t.Fatal(err)
	}
}

func TestADTTransportAdditionalEncodeAndDecodePaths(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	if _, err := encodeCollectionImage(datatype.ObjectCollection{}, factory); err == nil {
		t.Fatal("zero collection encoded")
	}
	missingElementType := &datatype.ObjectType{Collection: true}
	missingElement, err := missingElementType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encodeCollectionImage(missingElement, factory); err == nil {
		t.Fatal("collection without element metadata encoded")
	}
	longVersionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum, TypeVersion: 246}
	longVersionCollection, err := longVersionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := longVersionCollection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	if image, err := encodeCollectionImage(longVersionCollection, factory); err != nil || len(image) == 0 {
		t.Fatalf("long-version collection = %x, %v", image, err)
	}
	objectType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := objectType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if image, err := encodeObjectImage(object, factory); err != nil || len(image) == 0 {
		t.Fatalf("object with nil scalar field = %x, %v", image, err)
	}
	badAttributeType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1},
	}}
	badAttributeObject, err := badAttributeType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encodeObjectImage(badAttributeObject, factory); err == nil {
		t.Fatal("object with missing attribute type encoded")
	}
	longObjectType := &datatype.ObjectType{TOID: make([]byte, 16), TypeVersion: 246, Attributes: objectType.Attributes}
	longObject, err := longObjectType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, longObject, "VALUE", int64(1))
	if image, err := encodeObjectImage(longObject, factory); err != nil || image[0] != 0x80 {
		t.Fatalf("long-version polymorphic object = %x, %v", image, err)
	}
	if _, ok := objectValue(*object); !ok {
		t.Fatal("object value was not recognized")
	}
	if _, ok := objectValue("not an object"); ok {
		t.Fatal("non-object was recognized")
	}
}

func TestADTTransportRejectsTruncatedObjectAndCollectionRecords(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	collectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	collectionImage, err := encodeCollectionImage(collection, factory)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []driverCommon.B1Array{
		{0x88, 1, 254, 0, 0, 0, 10, 255, 0, 0},
		{0x88, 1, 254, 0, 0, 0, 10, 254, 0, 0},
		{0x88, 1, 254, 0, 0, 0, 11, 1, 0, 0, 0},
		{0x88, 1, 254, 0, 0, 0, 12, 1, 0, 0, 0, 0},
	} {
		if _, err := decodeCollectionImage(suffix, collectionType, factory); err == nil {
			t.Fatalf("truncated collection record accepted: %x", suffix)
		}
	}
	missingValue := append(driverCommon.B1Array(nil), collectionImage[:len(collectionImage)-2]...)
	missingValue[3], missingValue[4], missingValue[5], missingValue[6] = 0, 0, 0, byte(len(missingValue))
	if _, err := decodeCollectionImage(missingValue, collectionType, factory); err == nil {
		t.Fatal("collection without element payload decoded")
	}
	unsupportedCollectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyType(255)}
	if _, err := decodeCollectionImage(collectionImage, unsupportedCollectionType, factory); err == nil {
		t.Fatal("collection with unsupported decoder decoded")
	}

	objectType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := objectType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, object, "VALUE", int64(1))
	objectImage, err := encodeObjectImage(object, factory)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []driverCommon.B1Array{
		{0x80, 1, 254, 0, 0, 0, 7},
		{0x80, 1, 254, 0, 0, 0, 8, 0},
		{0x80, 1, 254, 0, 0, 0, 8, 1},
		{0x84, 1, 254, 0, 0, 0, 7},
	} {
		if _, err := decodeObjectImage(bad, objectType, factory); err == nil {
			t.Fatalf("truncated object record accepted: %x", bad)
		}
	}
	missingAttribute := append(driverCommon.B1Array(nil), objectImage[:7]...)
	missingAttribute[3], missingAttribute[4], missingAttribute[5], missingAttribute[6] = 0, 0, 0, byte(len(missingAttribute))
	if _, err := decodeObjectImage(missingAttribute, objectType, factory); err == nil {
		t.Fatal("object without attribute decoded")
	}
	badDecoderType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyType(255)}},
	}}
	if _, err := decodeObjectImage(objectImage, badDecoderType, factory); err == nil {
		t.Fatal("object with unsupported decoder decoded")
	}
}

func TestADTTransportRecordErrorPaths(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	scalarType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	if _, _, err := decodeObjectRecord(driverCommon.B1Array{1}, 0, &datatype.ObjectType{Closed: true}, factory, 0); err == nil {
		t.Fatal("closed object type decoded")
	}
	if _, _, err := decodeObjectRecord(driverCommon.B1Array{1}, 0, &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1},
	}}, factory, 0); err == nil {
		t.Fatal("missing object attribute metadata decoded")
	}
	for _, data := range []driverCommon.B1Array{nil, {254}, {5, 1}} {
		if _, _, err := decodeObjectRecord(data, 0, scalarType, factory, 0); err == nil {
			t.Fatalf("invalid scalar record decoded: %x", data)
		}
	}
	embeddedType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"CHILD": {Name: "CHILD", Sequence: 1, ObjectType: scalarType},
	}}
	if _, _, err := decodeObjectRecord(driverCommon.B1Array{252}, 0, embeddedType, factory, 0); err == nil {
		t.Fatal("root-level 252 embedded marker decoded")
	}
	if value, pos, err := decodeObjectRecord(driverCommon.B1Array{252, 1}, 0, embeddedType, factory, 1); err != nil || pos != 2 || objectField(t, *value, "CHILD") != nil {
		t.Fatalf("nested 252 marker = %#v, %d, %v", value, pos, err)
	}
	if _, _, err := decodeObjectRecord(driverCommon.B1Array{254}, 0, embeddedType, factory, 0); err == nil {
		t.Fatal("truncated embedded object decoded")
	}
	collectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	objectWithCollection := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUES": {Name: "VALUES", Sequence: 1, ObjectType: collectionType},
	}}
	for _, data := range []driverCommon.B1Array{{254}, {1, 0}} {
		if _, _, err := decodeObjectRecord(data, 0, objectWithCollection, factory, 0); err == nil {
			t.Fatalf("invalid collection record decoded: %x", data)
		}
	}
	nonSettable := &datatype.Object{ObjectType: scalarType, Value: reflect.ValueOf(struct{ VALUE any }{})}
	if err := setReflectedObjectField(nonSettable, 0, int64(1)); err == nil {
		t.Fatal("non-settable object field was set")
	}
	if _, ok := objectForBind((*datatype.Object)(nil)); ok {
		t.Fatal("nil object pointer was accepted")
	}
	if _, ok := objectForBind(&datatype.Object{ObjectType: &datatype.ObjectType{Collection: true}}); ok {
		t.Fatal("collection object pointer was accepted as object")
	}
}

func TestADTTransportRemainingBranches(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	scalarType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	if _, err := encodeObjectRecord(nil, &datatype.Object{ObjectType: scalarType}, factory, 0); err == nil {
		t.Fatal("object with invalid reflected value encoded")
	}

	shortPolymorphic := &datatype.ObjectType{TOID: make([]byte, 16), TypeVersion: 2, Attributes: scalarType.Attributes}
	shortObject, err := shortPolymorphic.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, shortObject, "VALUE", int64(1))
	if _, err := encodeObjectImage(shortObject, factory); err != nil {
		t.Fatal(err)
	}

	childType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"GRANDCHILD": {Name: "GRANDCHILD", Sequence: 1, ObjectType: scalarType},
	}}
	parentType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"CHILD": {Name: "CHILD", Sequence: 1, ObjectType: childType},
	}}
	child, err := childType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	parent, err := parentType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, parent, "CHILD", child)
	if encoded, err := encodeObjectRecord(nil, parent, factory, 0); err != nil || !reflect.DeepEqual(encoded, []byte{252, 1}) {
		t.Fatalf("nested NULL object record = %x, %v", encoded, err)
	}

	collectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	collection, err := collectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	if err := collection.Object.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := encodeCollectionImage(collection, factory); err == nil {
		t.Fatal("closed collection encoded")
	}

	objectCollectionType := &datatype.ObjectType{Collection: true, VArray: true, CollectionOf: scalarType}
	objectCollection, err := objectCollectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := objectCollection.Set([]any{"not an object"}); err != nil {
		t.Fatal(err)
	}
	if image, err := encodeCollectionImage(objectCollection, factory); err != nil || image[len(image)-1] != 255 {
		t.Fatalf("non-object collection element = %x, %v", image, err)
	}

	validCollectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	validCollection, err := validCollectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := validCollection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	image, err := encodeCollectionImage(validCollection, factory)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(driverCommon.B1Array){
		func(data driverCommon.B1Array) { data[2] = 255 },
		func(data driverCommon.B1Array) { data[7] = 255 },
		func(data driverCommon.B1Array) { data[7] = 20 },
		func(data driverCommon.B1Array) { data[12] = 254 },
	} {
		bad := append(driverCommon.B1Array(nil), image...)
		mutate(bad)
		if _, err := decodeCollectionImage(bad, validCollectionType, factory); err == nil {
			t.Fatalf("malformed collection image decoded: %x", bad)
		}
	}
	missingElement := append(driverCommon.B1Array(nil), image[:11]...)
	missingElement[3], missingElement[4], missingElement[5], missingElement[6] = 0, 0, 0, byte(len(missingElement))
	if _, err := decodeCollectionImage(missingElement, validCollectionType, factory); err == nil {
		t.Fatal("collection without element decoded")
	}
	noElementData := driverCommon.B1Array{0x88, 1, 254, 0, 0, 0, 13, 3, 0x11, 0, 0, 0, 1}
	if _, err := decodeCollectionImage(noElementData, validCollectionType, factory); err == nil {
		t.Fatal("collection with no element bytes decoded")
	}
	truncatedIndexType := &datatype.ObjectType{Collection: true, Associative: true, ElementType: common.DtyNum}
	truncatedIndex := append(driverCommon.B1Array(nil), image[:13]...)
	truncatedIndex[11] = collectionIndexesFlag
	truncatedIndex[3], truncatedIndex[4], truncatedIndex[5], truncatedIndex[6] = 0, 0, 0, byte(len(truncatedIndex))
	if _, err := decodeCollectionImage(truncatedIndex, truncatedIndexType, factory); err == nil {
		t.Fatal("collection with truncated index decoded")
	}

	indexedType := &datatype.ObjectType{Collection: true, Associative: true, ElementType: common.DtyNum}
	indexed, err := indexedType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := indexed.SetIndexed(map[int32]any{1: int64(1), 2: int64(2)}); err != nil {
		t.Fatal(err)
	}
	indexedImage, err := encodeCollectionImage(indexed, factory)
	if err != nil {
		t.Fatal(err)
	}
	copy(indexedImage[20:24], indexedImage[13:17])
	if _, err := decodeCollectionImage(indexedImage, indexedType, factory); err == nil {
		t.Fatal("duplicate associative indexes decoded")
	}

	shortPrefixObjectType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{}}
	shortPrefix := driverCommon.B1Array{0x80, 1, 254, 0, 0, 0, 9, 1, 0x04}
	if _, err := decodeObjectImage(shortPrefix, shortPrefixObjectType, factory); err == nil {
		t.Fatal("object with short type prefix decoded")
	}
	validObjectType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	validObject, err := validObjectType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, validObject, "VALUE", int64(1))
	validObjectImage, err := encodeObjectImage(validObject, factory)
	if err != nil {
		t.Fatal(err)
	}
	validObjectImage = append(validObjectImage, 0)
	validObjectImage[3], validObjectImage[4], validObjectImage[5], validObjectImage[6] = 0, 0, 0, byte(len(validObjectImage))
	if _, err := decodeObjectImage(validObjectImage, validObjectType, factory); err == nil {
		t.Fatal("object with trailing bytes decoded")
	}
	collectionAttributeType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUES": {Name: "VALUES", Sequence: 1, ObjectType: validCollectionType},
	}}
	collectionAttribute, err := collectionAttributeType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	closedCollection, err := validCollectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := closedCollection.Object.Close(); err != nil {
		t.Fatal(err)
	}
	setObjectField(t, collectionAttribute, "VALUES", closedCollection)
	if _, err := encodeObjectImage(collectionAttribute, factory); err == nil {
		t.Fatal("object encoded with closed collection attribute")
	}
}

func TestADTTransportCodecFailures(t *testing.T) {
	encodeFailure := adtCodecFactoryStub{encoderErr: errors.New("encode failure")}
	decodeFailure := adtCodecFactoryStub{decoderErr: errors.New("decode failure")}
	encodeValueFailure := adtCodecFactoryStub{encoder: &typeEncoder{encodeToType: func(sqldriver.Value) (driverCommon.B1Array, error) {
		return nil, errors.New("encode value failure")
	}}}
	decodeValueFailure := adtCodecFactoryStub{decoder: &typeDecoder{decodeToType: func(columnContext, any) (sqldriver.Value, error) {
		return nil, errors.New("decode value failure")
	}}}
	scalarCollectionType := &datatype.ObjectType{Collection: true, VArray: true, ElementType: common.DtyNum}
	scalarCollection, err := scalarCollectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := scalarCollection.Set([]any{int64(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := encodeCollectionImage(scalarCollection, encodeFailure); err == nil {
		t.Fatal("collection encoded with failing scalar encoder")
	}
	if _, err := encodeCollectionImage(scalarCollection, encodeValueFailure); err == nil {
		t.Fatal("collection encoded with failing scalar value encoder")
	}

	scalarObjectType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"VALUE": {Name: "VALUE", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
	}}
	scalarObject, err := scalarObjectType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, scalarObject, "VALUE", int64(1))
	if _, err := encodeObjectImage(scalarObject, encodeFailure); err == nil {
		t.Fatal("object encoded with failing scalar encoder")
	}
	if _, err := encodeObjectImage(scalarObject, encodeValueFailure); err == nil {
		t.Fatal("object encoded with failing scalar value encoder")
	}

	embeddedType := &datatype.ObjectType{Attributes: map[string]datatype.ObjectAttribute{
		"CHILD": {Name: "CHILD", Sequence: 1, ObjectType: scalarObjectType},
	}}
	embedded, err := embeddedType.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	setObjectField(t, embedded, "CHILD", scalarObject)
	if _, err := encodeObjectImage(embedded, encodeFailure); err == nil {
		t.Fatal("embedded object encoded with failing scalar encoder")
	}

	objectCollectionType := &datatype.ObjectType{Collection: true, VArray: true, CollectionOf: scalarObjectType}
	objectCollection, err := objectCollectionType.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := objectCollection.Set([]any{scalarObject}); err != nil {
		t.Fatal(err)
	}
	if _, err := encodeCollectionImage(objectCollection, encodeFailure); err == nil {
		t.Fatal("object collection encoded with failing child encoder")
	}

	validFactory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	collectionImage, err := encodeCollectionImage(scalarCollection, validFactory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCollectionImage(collectionImage, scalarCollectionType, decodeFailure); err == nil {
		t.Fatal("collection decoded with failing scalar decoder")
	}
	if _, err := decodeCollectionImage(collectionImage, scalarCollectionType, decodeValueFailure); err == nil {
		t.Fatal("collection decoded with failing scalar value decoder")
	}
	objectImage, err := encodeObjectImage(scalarObject, validFactory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeObjectImage(objectImage, scalarObjectType, decodeFailure); err == nil {
		t.Fatal("object decoded with failing scalar decoder")
	}
	if _, err := decodeObjectImage(objectImage, scalarObjectType, decodeValueFailure); err == nil {
		t.Fatal("object decoded with failing scalar value decoder")
	}
	if _, err := decodeObjectImage(objectImage, embeddedType, decodeFailure); err == nil {
		t.Fatal("embedded-object decode failure was not returned")
	}
	objectCollectionImage, err := encodeCollectionImage(objectCollection, validFactory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCollectionImage(objectCollectionImage, objectCollectionType, decodeFailure); err == nil {
		t.Fatal("object collection decode failure was not returned")
	}
}
