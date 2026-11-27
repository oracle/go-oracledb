/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package datatype

import (
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
	object.Attributes["ID"] = nil
	if object.IsNull() {
		t.Fatal("object with a NULL attribute is a NULL object")
	}
	if value := object.Attributes["ID"]; value != nil {
		t.Fatalf("ID = %v, want nil", value)
	}

	object.SetNull()
	if !object.IsNull() {
		t.Fatal("SetNull did not mark the object NULL")
	}
}

func TestObjectUsesAttributeMap(t *testing.T) {
	typ := &ObjectType{Attributes: map[string]ObjectAttribute{
		"ID": {Name: "ID", Sequence: 1, ObjectType: &ObjectType{ElementType: common.DtyNum}},
	}}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	object.Attributes["ID"] = int64(42)
	if got := object.Attributes["ID"]; got != int64(42) {
		t.Fatalf("ID = %v, want 42", got)
	}
}

func TestNewObjectCreatesNamedMapBackedPlaceholder(t *testing.T) {
	input := map[string]any{"ID": int64(42)}
	object := NewObject("APP.PERSON_T", input)
	if object.ObjectType == nil || object.FullName() != "APP.PERSON_T" || object.ObjectType.Collection {
		t.Fatalf("object placeholder = %#v", object)
	}
	if got := object.Attributes["ID"]; got != int64(42) {
		t.Fatalf("ID = %#v, want 42", got)
	}
	input["ID"] = int64(7)
	if got := object.Attributes["ID"]; got != int64(42) {
		t.Fatalf("object retained caller map mutation: %#v", got)
	}
	collection := NewCollection("APP.NUMBERS_T")
	if collection.ObjectType == nil || !collection.ObjectType.Collection || collection.FullName() != "APP.NUMBERS_T" {
		t.Fatalf("collection placeholder = %#v", collection)
	}
}
