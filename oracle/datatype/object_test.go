/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package datatype

import (
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

func TestObjectNullIsDistinctFromNullAttribute(t *testing.T) {
	typ := &ObjectType{Attributes: map[string]ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	object.Value.FieldByName("ID").SetZero()
	if object.IsNull() {
		t.Fatal("object with a NULL attribute is a NULL object")
	}
	if value := object.Value.FieldByName("ID").Interface(); value != nil {
		t.Fatalf("ID = %v, want nil", value)
	}

	object.SetNull()
	if !object.IsNull() {
		t.Fatal("SetNull did not mark the object NULL")
	}
}

func TestObjectUsesReflectedAttributeFields(t *testing.T) {
	typ := &ObjectType{Attributes: map[string]ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if !object.Value.IsValid() || object.Value.Kind() != reflect.Struct {
		t.Fatalf("object value = %#v, want valid struct", object.Value)
	}
	field := object.Value.FieldByName("ID")
	if !field.IsValid() || !field.CanSet() {
		t.Fatal("ID is not a settable reflected field")
	}
	field.Set(reflect.ValueOf(int64(42)))
	if got := field.Interface(); got != int64(42) {
		t.Fatalf("ID = %v, want 42", got)
	}
}
