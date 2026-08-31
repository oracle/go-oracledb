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

// Package datatype provides public values and metadata for Oracle abstract data
// types (ADTs), including object types and collection types such as VARRAYs.
//
// ObjectType describes a database type. Object and ObjectCollection hold values
// associated with an ObjectType and can be passed to the driver as named binds.
package datatype

import (
	"context"
	"database/sql"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// ObjectType describes an Oracle object or collection type.
//
// The driver populates its metadata fields from the database. Applications use
// the descriptor to construct Object and ObjectCollection values for binds.
type ObjectType struct {
	// CollectionOf describes the element object type when the collection contains
	// object values.
	CollectionOf *ObjectType
	// Attributes contains object attributes keyed by their database names.
	Attributes map[string]ObjectAttribute
	// Schema, Name, and PackageName identify the database type.
	Schema, Name, PackageName string
	// DBSize, ClientSizeInBytes, and CharSize describe the size constraints of
	// the database type.
	DBSize, ClientSizeInBytes, CharSize int
	// Precision, Scale, and FsPrecision describe numeric or temporal precision.
	Precision   int16
	Scale       int8
	FsPrecision uint8

	// TOID is the database type object identifier.
	TOID []byte
	// TypeVersion identifies the current version of the database type.
	TypeVersion int
	// TDS contains the type descriptor stream returned by the database.
	TDS []byte
	// Collection reports whether this descriptor represents a collection type.
	Collection bool
	// VArray reports whether this collection is a VARRAY rather than another
	// collection kind.
	VArray bool
	// UpperBound is the maximum number of elements allowed in a VARRAY.
	UpperBound int64
	// ElementType and ElementSize describe scalar collection elements.
	ElementType common.DtyType
	ElementSize int
	// Closed reports whether Close has been called for this descriptor.
	Closed bool
}

// ObjectAttribute describes one attribute of an Oracle object type.
type ObjectAttribute struct {
	// ObjectType describes the attribute's Oracle type.
	*ObjectType
	// Name is the attribute name as declared by the database type.
	Name string
	// Sequence is the one-based attribute position in the database type.
	Sequence uint32
}

// Object is a value of an Oracle object type.
type Object struct {
	// ObjectType identifies the database type represented by this value.
	*ObjectType
	// Attributes holds attribute values keyed by database attribute name.
	Attributes map[string]any
	// Values holds collection elements when this object represents a collection.
	Values []any
	// Closed reports whether Close has been called for this value.
	Closed bool
}

// ObjectCollection is an Oracle collection value, including a VARRAY.
type ObjectCollection struct {
	// Object contains the collection type and its element values.
	*Object
	// Null distinguishes a NULL collection from an empty collection.
	Null bool
}

// Execer executes a statement using database/sql-style positional arguments.
// Both *sql.DB and *sql.Conn implement this interface.
type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// GetObjectType retrieves the metadata for typeName and returns its public ADT
// descriptor. ex should use the same physical database connection that will
// later bind values created from the returned descriptor.
func GetObjectType(ctx context.Context, ex Execer, typeName string) (*ObjectType, error) {
	if ex == nil || strings.TrimSpace(typeName) == "" {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}

	var rc int64
	canonical := strings.TrimSpace(typeName)
	var toid, tds []byte
	var version int64
	var instantiable, superOwner, superName string
	var attributes, subtypes RefCursor
	_, err := ex.ExecContext(ctx, `BEGIN
  :1 := SYS.DBMS_PICKLER.GET_TYPE_SHAPE(:2, :3, :4, :5, :6, :7, :8, :9, :10);
END;`,
		sql.Out{Dest: &rc}, sql.Out{Dest: &canonical, In: true},
		sql.Out{Dest: &toid}, sql.Out{Dest: &version}, sql.Out{Dest: &tds},
		sql.Out{Dest: &instantiable}, sql.Out{Dest: &superOwner}, sql.Out{Dest: &superName},
		sql.Out{Dest: &attributes}, sql.Out{Dest: &subtypes})
	defer attributes.Close()
	defer subtypes.Close()
	if err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	if rc != 0 || len(toid) != 16 || len(tds) == 0 {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}

	typ := &ObjectType{
		Attributes:  make(map[string]ObjectAttribute),
		TOID:        append([]byte(nil), toid...),
		TypeVersion: int(version),
		TDS:         append([]byte(nil), tds...),
	}
	typ.SetName(canonical)
	if err := parseTDSHeader(typ); err != nil {
		return nil, err
	}
	return typ, nil
}

func parseTDSHeader(typ *ObjectType) error {
	if len(typ.TDS) < 18 || typ.TDS[4] != 38 || typ.TDS[11] != 41 {
		common.Odl.Error("ADT metadata error")
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	attributeCount := int(typ.TDS[8])<<8 | int(typ.TDS[9])
	if attributeCount != 1 || len(typ.TDS) < 28 || typ.TDS[18] != 28 {
		return nil
	}
	typ.Collection = true
	typ.UpperBound = int64(uint32(typ.TDS[23])<<24 | uint32(typ.TDS[24])<<16 | uint32(typ.TDS[25])<<8 | uint32(typ.TDS[26]))
	typ.VArray = typ.TDS[27] == 3
	elementOffset := int(uint32(typ.TDS[19])<<24 | uint32(typ.TDS[20])<<16 | uint32(typ.TDS[21])<<8 | uint32(typ.TDS[22]))
	if elementOffset < 0 || elementOffset >= len(typ.TDS) {
		common.Odl.Error("ADT metadata error")
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	elementType, elementSize, err := parseElementType(typ.TDS[elementOffset:])
	if err != nil {
		return err
	}
	typ.ElementType = elementType
	typ.ElementSize = elementSize
	return nil
}

func parseElementType(data []byte) (common.DtyType, int, error) {
	if len(data) == 0 {
		common.Odl.Error("ADT metadata error")
		return 0, 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	pos := 0
	for data[pos] == 44 || data[pos] == 43 {
		if data[pos] == 44 {
			pos += 2
		} else {
			pos++
		}
		if pos >= len(data) {
			common.Odl.Error("ADT metadata error")
			return 0, 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
	}
	switch data[pos] {
	case 6, 5:
		return common.DtyNum, 22, nil
	case 7, 1:
		if len(data) < pos+3 {
			return 0, 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		return common.DtyVCS, int(data[pos+1])<<8 | int(data[pos+2]), nil
	case 19:
		if len(data) < pos+3 {
			return 0, 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		return common.DtyBin, int(data[pos+1])<<8 | int(data[pos+2]), nil
	case 2:
		return common.DtyDat, 7, nil
	case 37:
		return common.DtyIbFloat, 4, nil
	case 45:
		return common.DtyIbDouble, 8, nil
	case 21:
		return common.DtyStamp, 11, nil
	case 23:
		return common.DtyStz, 13, nil
	case 33:
		return common.DtySitz, 11, nil
	case 8:
		return common.DtyBol, 4, nil
	default:
		common.Odl.Error("ADT metadata error")
		return 0, 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
}

// SetName parses name and assigns the descriptor's schema, package, and type
// name components. The expected form is TYPE, SCHEMA.TYPE, or
// SCHEMA.PACKAGE.TYPE.
func (t *ObjectType) SetName(name string) {
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		t.Schema = parts[0]
		t.Name = parts[len(parts)-1]
		if len(parts) > 2 {
			t.PackageName = strings.Join(parts[1:len(parts)-1], ".")
		}
		return
	}
	t.Name = name
}

// FullName returns the fully qualified database type name.
func (t *ObjectType) FullName() string {
	if t.PackageName != "" {
		return t.Schema + "." + t.PackageName + "." + t.Name
	}
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// IsObject reports whether t represents an object type rather than a collection.
func (t *ObjectType) IsObject() bool { return t != nil && !t.Collection }

// String returns the fully qualified database type name.
func (t *ObjectType) String() string { return t.FullName() }

// Close marks t as closed. A closed descriptor cannot create new values.
func (t *ObjectType) Close() error {
	if t != nil {
		t.Closed = true
	}
	return nil
}

// AttributeNames returns object attribute names in database declaration order.
func (t *ObjectType) AttributeNames() []string {
	names := make([]string, 0, len(t.Attributes))
	for i := uint32(1); i <= uint32(len(t.Attributes)); i++ {
		for _, a := range t.Attributes {
			if a.Sequence == i {
				names = append(names, a.Name)
				break
			}
		}
	}
	return names
}

// NewObject creates an empty Object value for t.
// It returns an error if t is closed or represents a collection.
func (t *ObjectType) NewObject() (*Object, error) {
	if t == nil || t.Closed {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if !t.IsObject() {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return &Object{ObjectType: t, Attributes: make(map[string]any)}, nil
}

// NewCollection creates an empty ObjectCollection value for t.
// It returns an error if t is closed or does not represent a collection.
func (t *ObjectType) NewCollection() (ObjectCollection, error) {
	if t == nil || t.Closed || !t.Collection {
		return ObjectCollection{}, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return ObjectCollection{Object: &Object{ObjectType: t, Attributes: make(map[string]any)}}, nil
}

// Close marks o as closed. A closed object cannot be read or updated.
func (o *Object) Close() error {
	if o != nil {
		o.Closed = true
	}
	return nil
}

// Collection returns o as an ObjectCollection when it represents a collection.
// It returns a zero ObjectCollection for ordinary object values.
func (o *Object) Collection() ObjectCollection {
	if o != nil && o.ObjectType != nil && o.ObjectType.Collection {
		return ObjectCollection{Object: o}
	}
	return ObjectCollection{}
}

// Get returns the value of the named object attribute.
func (o *Object) Get(name string) (any, error) {
	if o == nil || o.Closed {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	v, ok := o.Attributes[name]
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return v, nil
}

// Set assigns value to the named object attribute.
// It returns an error when o is closed or name is not an attribute of o's type.
func (o *Object) Set(name string, value any) error {
	if o == nil || o.Closed {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if _, ok := o.Attributes[name]; !ok && len(o.Attributes) != 0 {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	o.Attributes[name] = value
	return nil
}

// SetNull marks c as a NULL collection. NULL is distinct from an empty collection.
func (c *ObjectCollection) SetNull() {
	if c != nil {
		c.Null = true
	}
}

// IsNull reports whether c represents a NULL collection.
func (c ObjectCollection) IsNull() bool { return c.Null }

// Len returns the number of elements in c.
func (c ObjectCollection) Len() (int, error) {
	if c.Object == nil || c.Closed {
		return 0, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return len(c.Values), nil
}

// Append adds v to the end of c. It returns an error when c is invalid or the
// VARRAY upper bound would be exceeded.
func (c *ObjectCollection) Append(v any) error {
	if c.Object == nil || c.ObjectType == nil || !c.ObjectType.Collection {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if c.UpperBound > 0 && int64(len(c.Values)+1) > c.UpperBound {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.Null = false
	c.Values = append(c.Values, v)
	return nil
}

// Get returns the one-based element at index i.
func (c ObjectCollection) Get(i int) (any, error) {
	if i < 1 || i > len(c.Values) {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return c.Values[i-1], nil
}

// Set replaces the one-based element at index i with v.
func (c ObjectCollection) Set(i int, v any) error {
	if i < 1 || i > len(c.Values) {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.Values[i-1] = v
	return nil
}

// First returns the first one-based collection index, or zero for an empty collection.
func (c ObjectCollection) First() (int, error) {
	if len(c.Values) == 0 {
		return 0, nil
	}
	return 1, nil
}

// Last returns the last one-based collection index, or zero for an empty collection.
func (c ObjectCollection) Last() (int, error) { return len(c.Values), nil }

// Next returns the one-based index following i, or zero when there is no next element.
func (c ObjectCollection) Next(i int) (int, error) {
	if i < 0 || i >= len(c.Values) {
		return 0, nil
	}
	return i + 1, nil
}

// Trim removes n elements from the end of c.
func (c ObjectCollection) Trim(n int) error {
	if n < 0 || n > len(c.Values) {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.Values = c.Values[:len(c.Values)-n]
	return nil
}
