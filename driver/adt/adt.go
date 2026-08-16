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

// Package adt contains the public Oracle ADT and collection value model.
package adt

import (
	"strings"

	"github.com/oracle/go-oracledb/driver/common"
)

// ObjectType describes an Oracle object or collection type. The transport
// package populates the metadata fields while applications use the public
// descriptor and value methods.
type ObjectType struct {
	CollectionOf                        *ObjectType
	Attributes                          map[string]ObjectAttribute
	Schema, Name, PackageName           string
	DBSize, ClientSizeInBytes, CharSize int
	Precision                           int16
	Scale                               int8
	FsPrecision                         uint8

	TOID        []byte
	TypeVersion int
	TDS         []byte
	Collection  bool
	VArray      bool
	UpperBound  int64
	ElementType common.DtyType
	ElementSize int
	Closed      bool
}

type ObjectAttribute struct {
	*ObjectType
	Name     string
	Sequence uint32
}

type Object struct {
	*ObjectType
	Attributes map[string]any
	Values     []any
	Closed     bool
}

type ObjectCollection struct {
	*Object
	Null bool
}

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
func (t *ObjectType) FullName() string {
	if t.PackageName != "" {
		return t.Schema + "." + t.PackageName + "." + t.Name
	}
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}
func (t *ObjectType) IsObject() bool { return t != nil && !t.Collection }
func (t *ObjectType) String() string { return t.FullName() }
func (t *ObjectType) Close() error {
	if t != nil {
		t.Closed = true
	}
	return nil
}
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
func (t *ObjectType) NewObject() (*Object, error) {
	if t == nil || t.Closed {
		return nil, common.NewOracleError(common.ADTValueError, nil)
	}
	if !t.IsObject() {
		return nil, common.NewOracleError(common.ADTValueError, nil)
	}
	return &Object{ObjectType: t, Attributes: make(map[string]any)}, nil
}
func (t *ObjectType) NewCollection() (ObjectCollection, error) {
	if t == nil || t.Closed || !t.Collection {
		return ObjectCollection{}, common.NewOracleError(common.ADTValueError, nil)
	}
	return ObjectCollection{Object: &Object{ObjectType: t, Attributes: make(map[string]any)}}, nil
}

func (o *Object) Close() error {
	if o != nil {
		o.Closed = true
	}
	return nil
}
func (o *Object) Collection() ObjectCollection {
	if o != nil && o.ObjectType != nil && o.ObjectType.Collection {
		return ObjectCollection{Object: o}
	}
	return ObjectCollection{}
}
func (o *Object) Get(name string) (any, error) {
	if o == nil || o.Closed {
		return nil, common.NewOracleError(common.ADTValueError, nil)
	}
	v, ok := o.Attributes[name]
	if !ok {
		return nil, common.NewOracleError(common.ADTValueError, nil)
	}
	return v, nil
}
func (o *Object) Set(name string, value any) error {
	if o == nil || o.Closed {
		return common.NewOracleError(common.ADTValueError, nil)
	}
	if _, ok := o.Attributes[name]; !ok && len(o.Attributes) != 0 {
		return common.NewOracleError(common.ADTValueError, nil)
	}
	o.Attributes[name] = value
	return nil
}

func (c *ObjectCollection) SetNull() {
	if c != nil {
		c.Null = true
	}
}
func (c ObjectCollection) IsNull() bool { return c.Null }
func (c ObjectCollection) Len() (int, error) {
	if c.Object == nil || c.Closed {
		return 0, common.NewOracleError(common.ADTValueError, nil)
	}
	return len(c.Values), nil
}
func (c ObjectCollection) Append(v any) error {
	if c.Object == nil || c.ObjectType == nil || !c.ObjectType.Collection {
		return common.NewOracleError(common.ADTValueError, nil)
	}
	if c.UpperBound > 0 && int64(len(c.Values)+1) > c.UpperBound {
		return common.NewOracleError(common.ADTValueError, nil)
	}
	c.Null = false
	c.Values = append(c.Values, v)
	return nil
}
func (c ObjectCollection) Get(i int) (any, error) {
	if i < 1 || i > len(c.Values) {
		return nil, common.NewOracleError(common.ADTValueError, nil)
	}
	return c.Values[i-1], nil
}
func (c ObjectCollection) Set(i int, v any) error {
	if i < 1 || i > len(c.Values) {
		return common.NewOracleError(common.ADTValueError, nil)
	}
	c.Values[i-1] = v
	return nil
}
func (c ObjectCollection) First() (int, error) {
	if len(c.Values) == 0 {
		return 0, nil
	}
	return 1, nil
}
func (c ObjectCollection) Last() (int, error) { return len(c.Values), nil }
func (c ObjectCollection) Next(i int) (int, error) {
	if i < 0 || i >= len(c.Values) {
		return 0, nil
	}
	return i + 1, nil
}
func (c ObjectCollection) Trim(n int) error {
	if n < 0 || n > len(c.Values) {
		return common.NewOracleError(common.ADTValueError, nil)
	}
	c.Values = c.Values[:len(c.Values)-n]
	return nil
}
