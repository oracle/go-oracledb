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
	"reflect"
	"sort"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// ObjectType describes an Oracle object or collection type.
//
// The driver populates its metadata fields from the database. Root values are
// created with NewObject or NewCollection; descriptors remain available from
// those values for metadata inspection and constructing nested values.
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

	// Collection reports whether this descriptor represents a collection type.
	Collection bool
	// VArray reports whether this collection is a VARRAY rather than another
	// collection kind.
	VArray bool
	// Associative reports whether this collection is a PL/SQL INDEX BY
	// PLS_INTEGER table.
	Associative bool
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
	// Attributes holds object values keyed by database attribute name. Values
	// absent from the map are encoded as NULL attributes.
	Attributes map[string]any
	// values holds collection elements when this object represents a collection.
	// Collection values are accessed through ObjectCollection.Set and Get.
	values []any
	// indexes holds the PLS_INTEGER key for each associative-array value.
	indexes []int32
	// Null distinguishes a NULL object from an object whose attributes are all NULL.
	Null bool
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

// NewObject creates a named object value. The driver resolves type metadata on
// the physical connection immediately before the object is used. Attributes,
// when supplied, are copied into the returned object.
func NewObject(typeName string, attributes ...map[string]any) *Object {
	values := make(map[string]any)
	for _, input := range attributes {
		for name, value := range input {
			values[name] = value
		}
	}
	return &Object{ObjectType: newUnresolvedObjectType(typeName, false), Attributes: values}
}

// NewCollection creates a named collection value. The driver resolves type
// metadata on the physical connection immediately before it is used.
func NewCollection(typeName string) ObjectCollection {
	return ObjectCollection{Object: &Object{ObjectType: newUnresolvedObjectType(typeName, true)}}
}

func newUnresolvedObjectType(typeName string, collection bool) *ObjectType {
	typ := &ObjectType{Attributes: make(map[string]ObjectAttribute), Collection: collection}
	typ.SetName(strings.TrimSpace(typeName))
	return typ
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
	return ObjectCollection{Object: &Object{ObjectType: t}}, nil
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

// SetNull marks o as a NULL object. NULL is distinct from an object whose
// individual attributes are NULL.
func (o *Object) SetNull() {
	if o != nil {
		o.Null = true
	}
}

// IsNull reports whether o represents a NULL object.
func (o *Object) IsNull() bool { return o == nil || o.Null }

// SetNull marks c as a NULL collection. NULL is distinct from an empty collection.
func (c *ObjectCollection) SetNull() {
	if c != nil {
		c.Null = true
		if c.Object != nil {
			c.Object.Null = true
		}
	}
}

// IsNull reports whether c represents a NULL collection.
func (c ObjectCollection) IsNull() bool { return c.Null || c.Object == nil || c.Object.Null }

// Len returns the number of elements in c.
func (c ObjectCollection) Len() (int, error) {
	if c.Object == nil || c.Closed {
		return 0, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return len(c.values), nil
}

// Set replaces all collection values from a Go slice or array. It returns an
// error when c is invalid, values is not a slice or array, or the supplied
// values exceed the VARRAY upper bound.
func (c *ObjectCollection) Set(values any) error {
	if c == nil || c.Object == nil || c.Object.Closed || c.ObjectType == nil || !c.ObjectType.Collection {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	items, err := collectionItems(values)
	if err != nil {
		return err
	}
	if c.UpperBound > 0 && int64(len(items)) > c.UpperBound {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.values = append(c.values[:0], items...)
	if c.Associative {
		c.indexes = make([]int32, len(items))
		for index := range c.indexes {
			c.indexes[index] = int32(index + 1)
		}
	} else {
		c.indexes = nil
	}
	c.Null = false
	c.Object.Null = false
	return nil
}

// SetIndexed replaces the values in an associative collection. Keys are
// PL/SQL PLS_INTEGER indexes and are encoded in ascending key order.
// It returns an error if c is not an associative collection.
func (c *ObjectCollection) SetIndexed(values map[int32]any) error {
	if c == nil || c.Object == nil || c.Object.Closed || c.ObjectType == nil || !c.ObjectType.Collection || ((c.VArray || c.ElementType != 0 || c.CollectionOf != nil) && !c.Associative) {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	indexes := make([]int32, 0, len(values))
	for index := range values {
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(left, right int) bool { return indexes[left] < indexes[right] })
	items := make([]any, len(indexes))
	for position, index := range indexes {
		items[position] = values[index]
	}
	c.Associative = true // Preserve the requested index semantics until bind-time validation.
	c.values = items
	c.indexes = indexes
	c.Null = false
	c.Object.Null = false
	return nil
}

func collectionItems(values any) ([]any, error) {
	if values == nil {
		return nil, nil
	}
	value := reflect.ValueOf(values)
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	items := make([]any, value.Len())
	for index := range items {
		items[index] = value.Index(index).Interface()
	}
	return items, nil
}

// Get returns all collection values in collection order.
func (c ObjectCollection) Get() ([]any, error) {
	if c.Object == nil || c.Object.Closed || c.IsNull() {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return append([]any(nil), c.values...), nil
}

// GetIndexed returns all values in an associative collection keyed by their
// PL/SQL PLS_INTEGER index. It returns an error for non-associative,
// invalid, and NULL collections.
func (c ObjectCollection) GetIndexed() (map[int32]any, error) {
	indexes, err := c.Indexes()
	if err != nil {
		return nil, err
	}
	values := make(map[int32]any, len(c.values))
	for position, index := range indexes {
		values[index] = c.values[position]
	}
	return values, nil
}

// Indexes returns the PLS_INTEGER indexes of an associative collection in
// collection order. It returns an error for non-associative, invalid, or NULL
// collections.
func (c ObjectCollection) Indexes() ([]int32, error) {
	if c.Object == nil || c.Object.Closed || c.IsNull() || !c.Associative || (c.indexes != nil && len(c.indexes) != len(c.values)) {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if c.indexes == nil {
		indexes := make([]int32, len(c.values))
		for i := range indexes {
			indexes[i] = int32(i + 1)
		}
		return indexes, nil
	}
	return append([]int32(nil), c.indexes...), nil
}
