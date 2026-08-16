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
	"testing"

	"github.com/oracle/go-oracledb/driver/adt"
	"github.com/oracle/go-oracledb/driver/common"
)

type adtMetadataExecer func(context.Context, string, ...any) (sql.Result, error)

func (f adtMetadataExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return f(ctx, query, args...)
}

func TestObjectCollectionOperations(t *testing.T) {
	typ := &adt.ObjectType{Name: "NUMBERS", Collection: true, VArray: true, UpperBound: 2}
	c, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Append(int64(1)); err != nil {
		t.Fatal(err)
	}
	if err := c.Append(int64(2)); err != nil {
		t.Fatal(err)
	}
	if err := c.Append(int64(3)); err == nil {
		t.Fatal("Append succeeded past VARRAY bound")
	}
	if n, err := c.Len(); err != nil || n != 2 {
		t.Fatalf("Len() = %d, %v", n, err)
	}
	if got, err := c.Get(2); err != nil || got != int64(2) {
		t.Fatalf("Get(2) = %v, %v", got, err)
	}
	if err := c.Set(2, int64(20)); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get(2); got != int64(20) {
		t.Fatalf("Set did not persist: %v", got)
	}
	if first, _ := c.First(); first != 1 {
		t.Fatalf("First() = %d", first)
	}
	if next, _ := c.Next(1); next != 2 {
		t.Fatalf("Next(1) = %d", next)
	}
	if err := c.Trim(1); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.Len(); n != 1 {
		t.Fatalf("Len after Trim = %d", n)
	}
}

func TestNamedTypeForZeroCollectionAndOutInference(t *testing.T) {
	if _, ok := namedTypeForBind(adt.ObjectCollection{}); ok {
		t.Fatal("zero ObjectCollection unexpectedly reported a named type")
	}
	typ := &adt.ObjectType{Name: "NUMBERS", Collection: true, VArray: true}
	in, err := typ.NewCollection()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := inferNamedTypeForOut([]sqldriver.Value{
		sql.Out{Dest: &adt.ObjectCollection{}},
		in,
	}, 0)
	if !ok || got != typ {
		t.Fatalf("inferred type = %v, %v; want %p, true", got, ok, typ)
	}
}

func TestObjectCollectionNullBind(t *testing.T) {
	typ := &adt.ObjectType{Name: "NUMBERS", Collection: true, VArray: true}
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
	if err := c.Append(int64(1)); err != nil {
		t.Fatal(err)
	}
	if c.IsNull() {
		t.Fatal("Append did not restore non-NULL collection state")
	}
}

func TestRefCursorOutBindUsesCursorOAC(t *testing.T) {
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	normalized := normalizeBindValue(sql.Out{Dest: &RefCursor{}})
	oac, err := factory.GetBindOac(normalized, 0)
	if err != nil {
		t.Fatal(err)
	}
	cursorOAC, ok := oac.(*tTIoac)
	if !ok {
		t.Fatalf("OAC type = %T, want *tTIoac", oac)
	}
	if cursorOAC.dataType != common.UB1(common.DtyCur) || cursorOAC.maxLength != 4 {
		t.Fatalf("REF CURSOR OAC = dataType %d, maxLength %d; want %d, 4", cursorOAC.dataType, cursorOAC.maxLength, common.DtyCur)
	}
}

func TestGetObjectTypeBindsUnusedMetadataAsRefCursors(t *testing.T) {
	sentinel := errors.New("stop after checking binds")
	_, err := loadADTObjectType(context.Background(), adtMetadataExecer(func(_ context.Context, _ string, args ...any) (sql.Result, error) {
		if len(args) != 10 {
			t.Fatalf("bind count = %d, want 10", len(args))
		}
		for _, index := range []int{8, 9} {
			out, ok := args[index].(sql.Out)
			if !ok {
				t.Fatalf("bind %d = %T, want sql.Out", index+1, args[index])
			}
			if _, ok := out.Dest.(*RefCursor); !ok {
				t.Fatalf("bind %d destination = %T, want *RefCursor", index+1, out.Dest)
			}
		}
		return nil, sentinel
	}), "SCHEMA.NUMBER_VARRAY")
	if !errors.Is(err, sentinel) {
		t.Fatalf("GetObjectType error = %v, want wrapped sentinel", err)
	}
}

func TestParseTDSHeaderCollection(t *testing.T) {
	tds := make([]byte, 29)
	tds[4] = 38  // KOPT_OP_VERSION
	tds[11] = 41 // KOPT_OP_STARTADT
	tds[18] = 28 // KOPM_OTS_COLLECTION
	tds[23], tds[24], tds[25], tds[26] = 0, 0, 0, 10
	tds[27] = 3  // KOPU_UPT_VARRAY
	tds[22] = 28 // element TDS offset
	tds[28] = 6  // KOPM_OTS_NUMBER
	typ := &adt.ObjectType{TDS: tds}
	if err := parseADTTDSHeader(typ); err != nil {
		t.Fatal(err)
	}
	if !typ.Collection || !typ.VArray || typ.UpperBound != 10 {
		t.Fatalf("unexpected collection metadata: %#v", typ)
	}
}

func TestDecodeCollectionImageJDBCHeader(t *testing.T) {
	typ := &adt.ObjectType{Collection: true, VArray: true, UpperBound: 5, ElementType: common.DtyNum}
	// JDBC's 8.1 image for VARRAY(10, 20, 30): prefix length 3, inline/type-
	// version flag 0x11, type version 1, collection flags 0, and three values.
	image := common.B1Array{
		0x88, 0x01, 0xFE, 0, 0, 0, 0x16,
		0x03, 0x11, 0x00, 0x01, 0x00, 0x03,
		0x02, 0xC1, 0x0B, 0x02, 0xC1, 0x15, 0x02, 0xC1, 0x1F,
	}
	value, err := decodeCollectionImage(image, typ, NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	if err != nil {
		t.Fatalf("decode JDBC collection image: %v", err)
	}
	collection, ok := value.(adt.ObjectCollection)
	if !ok {
		t.Fatalf("decoded value = %T, want ObjectCollection", value)
	}
	if got, err := collection.Len(); err != nil || got != 3 {
		t.Fatalf("decoded length = %d, %v; want 3", got, err)
	}
}

func TestObjectAttributes(t *testing.T) {
	typ := &adt.ObjectType{Attributes: map[string]adt.ObjectAttribute{"A": {Name: "A", Sequence: 1}}}
	o, err := typ.NewObject()
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Set("A", "x"); err != nil {
		t.Fatal(err)
	}
	if got, err := o.Get("A"); err != nil || got != "x" {
		t.Fatalf("Get = %v, %v", got, err)
	}
}

func TestNamedTypeOACCarriesTOIDAndVersion(t *testing.T) {
	typ := &adt.ObjectType{Name: "NUMBERS", TOID: make([]byte, 16), TypeVersion: 7}
	oac, err := newTTIOacNamedType(typ, 128)
	if err != nil {
		t.Fatal(err)
	}
	if oac.dataType != common.UB1(common.DtyNty) {
		t.Fatalf("dataType = %d, want %d", oac.dataType, common.DtyNty)
	}
	if oac.versionNumber != 7 || oac.toid == nil || len(*oac.toid) != 16 {
		t.Fatalf("missing named-type metadata: %#v", oac)
	}
}
