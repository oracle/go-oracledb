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
	"reflect"
	"sync"
)

// maxItems is the maximum number of items allowed in the registry.
const maxItems = 50

// Registry stores values and exposes safe access to registered items.
type Registry[T any] interface {
	// Register appends an item to the registry.
	// When the registry already contains maxItems entries, the oldest
	// registered item is removed before item is appended.
	//
	// Parameters:
	//   - item: the item to register.
	Register(item T)
	// GetAll returns a snapshot of all items currently registered, in
	// registration order.
	GetAll() []T
	// Get returns the first item found in the registry that implements the
	// desired type.
	//
	// Parameters:
	//   - requestedType: the interface or concrete item type to resolve from
	//     the registry.
	//
	// Returns:
	//   - the first registered item matching requestedType, or the zero value if
	//     no matching item was found
	//   - an error if requestedType is invalid
	Get(requestedType reflect.Type) (T, error)
}

// registry stores registry items without synchronization.
type registry[T any] struct {
	items []T
}

// safeRegistry is the thread-safe Registry implementation.
type safeRegistry[T any] struct {
	items         registry[T]
	registryMutex sync.RWMutex
}

// NewSafeRegistry creates an empty thread-safe registry.
//
// Returns:
//   - the initialized registry.
func NewSafeRegistry[T any]() *safeRegistry[T] {
	return &safeRegistry[T]{}
}

// NewRegistry creates an empty registry.
//
// Returns:
//   - the initialized registry.
func NewRegistry[T any]() *registry[T] {
	return &registry[T]{}
}

// Register appends an item to the registry.
// When the registry already contains maxItems entries, the oldest
// registered item is removed before item is appended.
//
// Parameters:
//   - item: the item to register.
func (registry *safeRegistry[T]) Register(item T) {
	registry.registryMutex.Lock()
	defer registry.registryMutex.Unlock()
	registry.items.Register(item)
}

// Register appends an item to the registry.
// When the registry already contains maxItems entries, the oldest
// registered item is removed before item is appended.
//
// Parameters:
//   - item: the item to register.
func (registry *registry[T]) Register(item T) {
	if len(registry.items) >= maxItems {
		Odl.Warn("Number of registered items exceeded the maximum allowed, removing the oldest item")
		registry.items = registry.items[1:]
	}
	registry.items = append(registry.items, item)
}

// GetAll returns a snapshot of all items currently registered, in registration
// order. Changes to the returned slice do not affect the registry.
func (registry *safeRegistry[T]) GetAll() []T {
	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()

	allItems := registry.items.GetAll()
	items := make([]T, len(allItems))
	copy(items, allItems)
	return items
}

// GetAll returns a snapshot of all items currently registered, in registration
// order. Changes to the returned slice do not affect the registry.
func (registry *registry[T]) GetAll() []T {
	return registry.items
}

// Get returns the first item found in the registry that implements the desired type.
//
// Parameters:
//   - requestedType: the interface or concrete item type to resolve from
//     the registry.
//
// Returns:
//   - the first registered item matching requestedType, or the zero value if
//     no matching item was found
//   - an error if requestedType is invalid
func (registry *safeRegistry[T]) Get(requestedType reflect.Type) (T, error) {
	registry.registryMutex.RLock()
	defer registry.registryMutex.RUnlock()

	return registry.items.Get(requestedType)
}

// Get returns the first item found in the registry that implements the desired type.
//
// Parameters:
//   - requestedType: the interface or concrete item type to resolve from
//     the registry.
//
// Returns:
//   - the first registered item matching requestedType, or the zero value if
//     no matching item was found
//   - an error if requestedType is invalid
func (registry *registry[T]) Get(requestedType reflect.Type) (T, error) {
	for _, item := range registry.items {
		itemType := reflect.TypeOf(item)
		if itemType == nil {
			continue
		}
		if itemType.AssignableTo(requestedType) || (requestedType.Kind() == reflect.Interface && itemType.Implements(requestedType)) {
			return item, nil
		}
	}
	var zero T
	return zero, nil
}
