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

package common

import (
	"cmp"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

// Properties represent a set of key-value pairs.
// This special type of map support one level snapshotting
// mapped values can be reverted to their original state
type Properties[T cmp.Ordered] struct {
	_source  map[T]any // original mapping, populated with AddProperty
	_updated map[T]any // updated mapping, populated with SetProperty
}

// NewProperties creates a new Properties
func NewProperties[T cmp.Ordered]() *Properties[T] {
	return &Properties[T]{
		_source:  make(map[T]any),
		_updated: make(map[T]any),
	}
}

// NewPropertiesWithSource creates a new pre-populated Properties
// parameters:
//   - source : key/value to initialize the Properties
func NewPropertiesWithSource[T cmp.Ordered](source map[T]any) *Properties[T] {
	return &Properties[T]{
		_source:  maps.Clone(source),
		_updated: make(map[T]any),
	}
}

// SetProperty sets a property key to the specified value.
func (p *Properties[T]) SetProperty(key T, value any) {
	p._updated[key] = value
}

// Reset resets the Properties object to its original state when
// NewProperties or NewPropertiesWithSource have been called
func (p *Properties[T]) Reset() {
	clear(p._updated)
}

// Snapshot snapshots the Properties object to its current state.
func (p *Properties[T]) Snapshot() {
	maps.Insert(p._source, maps.All(p._updated))
	clear(p._updated)
}

// _fetchAllKeys gets all keys of underlying maps
func _fetchAllKeys[T cmp.Ordered](_s, _u map[T]any) iter.Seq[T] {

	var _allKeys map[T]any
	_allKeys = map[T]any{}

	if _s != nil {
		for k := range maps.Keys(_s) {
			_allKeys[k] = nil
		}
	}
	for k := range maps.Keys(_u) {
		_allKeys[k] = nil
	}
	return maps.Keys(_allKeys)
}

// String implements the Stringer interfaces
// returns an array sorted by keys like
//
//	[key1=value1,key2=value2]
func (p Properties[T]) String() string {
	var b strings.Builder
	b.WriteString("[")
	var allK = _fetchAllKeys(p._source, p._updated)

	var firstOne bool = true
	for _, k := range slices.Sorted[T](allK) {
		if !firstOne {
			b.WriteString(",")
		} else {
			firstOne = false
		}

		f, ok := any(k).(string)
		if ok {
			if strings.Contains(strings.ToLower(f), "password") {
				// dummy protection
				b.WriteString(fmt.Sprintf("%s=%s", f, "*****"))
			} else {
				b.WriteString(fmt.Sprintf("%s=%v", f, p.GetProperty(k)))
			}
		} else {
			b.WriteString(fmt.Sprintf("%s=%v", f, p.GetProperty(k)))
		}
	}

	b.WriteString("]")
	return b.String()
}

// GetProperty retrieves the value associated with the specified key.
//   - returns : the mapped value or nil if mapping is missing
func (p *Properties[T]) GetProperty(key T) any {
	// first look into updated values.
	value, exists := p._updated[key]
	if exists {
		return value
	}
	if p._source != nil {
		return p._source[key]
	}
	return nil
}

// GetTrimmedString retrieves the string value associated with the specified key and trims whitespace.
// parameter:
//   - the property key
//
// returns:
//
//	a trimmed verison of associated value
//
// errors: if associated value cannot be asserted to a string (or mapping is missing)
func (p *Properties[T]) GetTrimmedString(key T) (string, error) {
	var value = p.GetProperty(key)
	switch value.(type) {
	case string:
		return strings.TrimSpace(value.(string)), nil
	case B1Array:
		return strings.TrimSpace(B1ArrayToString(value.(B1Array))), nil
	default:
		return "", errors.New("value can't be assigned to string")
	}
}

// ContainsKey checks if the specified key exists in the properties.
func (p *Properties[T]) ContainsKey(key T) bool {
	if _, ok := p._updated[key]; ok {
		return true
	} else {
		if p._source != nil {
			if _, ok := p._source[key]; ok {
				return true
			}
		}
	}
	return false
}

// PutAll copies all key-value pairs from another Properties instance.
// This call is equivalent to call SetProperty() for all keys of 'other'
func (p *Properties[T]) PutAll(other *Properties[T]) {
	maps.Insert(p._updated, maps.All(other._source))
	maps.Insert(p._updated, maps.All(other._updated))
}
