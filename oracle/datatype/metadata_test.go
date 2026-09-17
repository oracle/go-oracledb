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

package datatype

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

type metadataExecer func(context.Context, string, ...any) (sql.Result, error)

func (f metadataExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return f(ctx, query, args...)
}

type metadataRows struct {
	values [][]driver.Value
	index  int
	closed bool
}

func (r *metadataRows) Columns() []string {
	return []string{
		"CURSOR_VERSION", "ATTR_NAME", "ATTR_NUM", "ATTR_TYPE_OWNER", "ATTR_TYPE_NAME",
		"ATTR_TYPE_PACKAGE", "ATTR_TYPE_OID", "ATTR_INSTANTIABLE", "ATTR_SUPERTYPE_OWNER", "ATTR_SUPERTYPE_NAME",
	}
}

func (r *metadataRows) Close() error {
	r.closed = true
	return nil
}

func (r *metadataRows) Next(dest []driver.Value) error {
	if r.index == len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func TestParseTDSCollection(t *testing.T) {
	// The collection element is a deferred TDS patch. The top-level stream ends
	// before the element's record, as it does in a database-supplied TDS.
	tds := []byte{
		0, 0, 0, 31, // TDS length
		38, 1, 0, 0, // version record
		0, 1, 0, // one attribute and descriptor flags
		41, 0, 0, 0, 0, 0, 0, // STARTADT and index-table offset
		28, 0, 0, 0, 29, // COLLECTION, deferred element offset
		0, 0, 0, 10, 3, // upper bound and VARRAY user code
		42,       // ENDADT
		6, 38, 0, // NUMBER(38, 0) deferred element record
	}
	typ := &ObjectType{TDS: tds}
	if err := parseTDS(typ); err != nil {
		t.Fatal(err)
	}
	if !typ.Collection || !typ.VArray || typ.UpperBound != 10 || typ.ElementType == 0 || typ.ElementSize != 22 {
		t.Fatalf("unexpected collection metadata: %#v", typ)
	}
}

func TestParseTDSNestedTable(t *testing.T) {
	tds := []byte{
		0, 0, 0, 31, 38, 1, 0, 0, 0, 1, 0, 41, 0, 0, 0, 0, 0, 0,
		28, 0, 0, 0, 29, 0, 0, 0, 0, 2, 42, 7, 0, 12, 0, 0, 0,
	}
	typ := &ObjectType{TDS: tds}
	if err := parseTDS(typ); err != nil {
		t.Fatal(err)
	}
	if !typ.Collection || typ.VArray || typ.UpperBound != 0 || typ.ElementType == 0 || typ.ElementSize != 12 {
		t.Fatalf("unexpected nested table metadata: %#v", typ)
	}
}

func TestParseTDSObjectWithEmbeddedObject(t *testing.T) {
	tds := []byte{
		0, 0, 0, 27, 38, 1, 0, 0, 0, 2, 0, 41, 0, 0, 0, 0, 0, 0,
		6, 38, 0, // NUMBER attribute
		39, 7, 0, 10, 0, 0, 0, 40, // embedded VARCHAR2 attribute
		42,
	}
	typ := &ObjectType{TDS: tds}
	if err := parseTDS(typ); err != nil {
		t.Fatal(err)
	}
	if typ.Collection || typ.tdsType == nil || len(typ.tdsType.attributes) != 2 {
		t.Fatalf("unexpected object TDS: %#v", typ.tdsType)
	}
	embedded := typ.tdsType.attributes[1]
	if len(embedded.attributes) != 1 || embedded.attributes[0].dtyType == 0 {
		t.Fatalf("embedded attribute was not parsed: %#v", embedded)
	}
}

func TestParseTDSResolvesDeferredCollectionPatch(t *testing.T) {
	nested := []byte{
		0, 0, 0, 31, 38, 1, 0, 0, 0, 1, 0, 41, 0, 0, 0, 0, 0, 0,
		28, 0, 0, 0, 29, 0, 0, 0, 2, 3, 42, 6, 38, 0,
	}
	tds := []byte{
		0, 0, 0, 25, 38, 1, 0, 0, 0, 1, 0, 41, 0, 0, 0, 0, 0, 0,
		27, 0, 0, 0, 25, 251, 42, // UPT collection patch and ENDADT
		0, // normal-patch opcode
	}
	tds = append(tds, nested...)
	typ := &ObjectType{TDS: tds}
	if err := parseTDS(typ); err != nil {
		t.Fatal(err)
	}
	if !typ.Collection || !typ.VArray || typ.ElementType == 0 {
		t.Fatalf("deferred collection patch was not resolved: %#v", typ)
	}
}

func TestParseTDSRejectsTruncatedMetadata(t *testing.T) {
	if err := parseTDS(&ObjectType{TDS: []byte{0, 0, 0, 1}}); err == nil {
		t.Fatal("parseTDS succeeded for truncated TDS")
	}
}

func TestGetObjectTypePopulatesAttributesFromMetadataCursor(t *testing.T) {
	tds := []byte{
		0, 0, 0, 28, 38, 1, 0, 0, 0, 2, 0, 41, 0, 0, 0, 0, 0, 0,
		6, 38, 0, // NUMBER(38, 0)
		7, 0, 20, 0, 0, 0, // VARCHAR2(20)
		42,
	}
	attributes := &metadataRows{values: [][]driver.Value{
		{int64(1), "NAME", int64(2), "APP", "VARCHAR2", nil, nil, "YES", nil, nil},
		{int64(1), "ID", int64(1), "APP", "NUMBER", nil, nil, "YES", nil, nil},
	}}
	typ, err := GetObjectType(context.Background(), metadataExecer(func(_ context.Context, _ string, args ...any) (sql.Result, error) {
		*args[0].(sql.Out).Dest.(*int64) = 0
		*args[1].(sql.Out).Dest.(*string) = "APP.PERSON_T"
		*args[2].(sql.Out).Dest.(*[]byte) = make([]byte, 16)
		*args[3].(sql.Out).Dest.(*int64) = 1
		*args[4].(sql.Out).Dest.(*[]byte) = tds
		*args[8].(sql.Out).Dest.(*driver.Rows) = attributes
		return nil, nil
	}), "APP.PERSON_T")
	if err != nil {
		t.Fatal(err)
	}
	if !attributes.closed {
		t.Fatal("attribute metadata cursor was not closed")
	}
	if got, want := typ.AttributeNames(), []string{"ID", "NAME"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("AttributeNames() = %v, want %v", got, want)
	}
	if got := typ.Attributes["ID"].ObjectType; got == nil || got.ElementType != common.DtyNum || got.Precision != 38 {
		t.Fatalf("ID metadata = %#v, want NUMBER(38)", got)
	}
	if got := typ.Attributes["NAME"].ObjectType; got == nil || got.ElementType != common.DtyVCS || got.ElementSize != 20 {
		t.Fatalf("NAME metadata = %#v, want VARCHAR2(20)", got)
	}
	object, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if err := object.Set("ID", int64(1)); err != nil {
		t.Fatal(err)
	}
	if err := object.Set("MISSING", "value"); err == nil {
		t.Fatal("Set accepted an attribute absent from metadata")
	}
}
