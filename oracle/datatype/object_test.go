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
	if err := object.Set("ID", nil); err != nil {
		t.Fatal(err)
	}
	if object.IsNull() {
		t.Fatal("object with a NULL attribute is a NULL object")
	}
	if value, err := object.Get("ID"); err != nil || value != nil {
		t.Fatalf("Get(ID) = %v, %v; want nil, nil", value, err)
	}

	object.SetNull()
	if !object.IsNull() {
		t.Fatal("SetNull did not mark the object NULL")
	}
	if _, err := object.Get("ID"); err == nil {
		t.Fatal("Get succeeded for a NULL object")
	}
	if err := object.Set("ID", int64(1)); err != nil {
		t.Fatal(err)
	}
	if object.IsNull() {
		t.Fatal("Set did not restore non-NULL object state")
	}
}
