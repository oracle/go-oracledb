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
	"container/list"
	"sync"
	"time"
)

// Cache interface for cache mechanism in the go driver
type Cache[T any] interface {
	// Get gets a cached value (including nullable ones) referenced by key (non-nil).
	// returns the cached value and whether it was found.
	Get(key string) (value T, found bool)
	// Put puts a value into the cache referenced by non-nil key.
	// replace existing cache value
	// returns the value previously assigned to the that key or nil.
	Put(key string, value T) T
	// Remove removed a value from the cache referenced by non-nil key.
	// return true of a value was removed
	Remove(key string) bool
	// Clear clears the caches from all values
	Clear()
}

// ttlCacheEntry stores a cached value within TTLCache.
type ttlCacheEntry[T any] struct {
	value T         // cached value
	ctime time.Time // cached value creation time
}

// TTLCache is a cache implementation that has a fix size.
// This caches stores value that are automatically existed after a given TTL
// when the cache become full the oldest element is remove to make room for
// the new entry
type TTLCache[T any] struct {
	maxSize        int                         // max size of the cache
	ttl            time.Duration               // TTL of cached entries
	nextExpiration time.Time                   // time for the next cleanup to happen.
	entries        map[string]ttlCacheEntry[T] // cached values
}

// NewTTLCache creates a TTLCache with the given maximum size and entry TTL.
// It may return nil when maxSize is not positive or ttl is zero.
func NewTTLCache[T any](maxSize int, ttl time.Duration) *TTLCache[T] {
	if maxSize <= 0 {
		Odl.Error("maxSize must be positive")
		return nil
	}
	if ttl.Seconds() <= 0 {
		Odl.Error("TTL can't be zero")
		return nil
	}

	return &TTLCache[T]{
		maxSize: maxSize,
		ttl:     ttl,
		entries: make(map[string]ttlCacheEntry[T]),
	}
}

func (c *TTLCache[T]) Get(key string) (value T, found bool) {
	if c.shouldRemoveExpired() {
		c.removeExpired()
	}

	entry, ok := c.entries[key]
	if !ok {
		var zero T
		Odl.Debug("cache miss", "key", key)
		return zero, false
	}

	return entry.value, true
}

func (c *TTLCache[T]) Put(key string, value T) T {
	now := time.Now()
	var previous T
	if entry, ok := c.entries[key]; ok {
		previous = entry.value
	}

	c.entries[key] = ttlCacheEntry[T]{
		value: value,
		ctime: now,
	}
	expiresAt := now.Add(c.ttl)
	if c.nextExpiration.IsZero() || expiresAt.Before(c.nextExpiration) {
		c.nextExpiration = expiresAt
	}

	if c.maxSize > 0 && len(c.entries) > c.maxSize {
		Odl.Debug("cache overflow")
		c.removeOldest()
	}

	return previous
}

func (c *TTLCache[T]) Remove(key string) bool {
	if _, ok := c.entries[key]; ok {
		delete(c.entries, key)
		c.recomputeNextExpiration()
		return true
	}
	return false
}

func (c *TTLCache[T]) Clear() {
	clear(c.entries)
	c.nextExpiration = time.Time{}
}

// removeExpired removes entries whose TTL has elapsed and updates the next
// scheduled expiration time for the remaining entries.
func (c *TTLCache[T]) removeExpired() {
	now := time.Now()
	var nextExpiration time.Time
	first := true
	for key, entry := range c.entries {
		expiresAt := entry.ctime.Add(c.ttl)
		if !now.Before(expiresAt) {
			delete(c.entries, key)
			continue
		}
		if first || expiresAt.Before(nextExpiration) {
			nextExpiration = expiresAt
			first = false
		}
	}
	if first {
		c.nextExpiration = time.Time{}
		return
	}
	c.nextExpiration = nextExpiration
}

// removeOldest evicts the entry with the oldest fixed creation timestamp.
// TTLCache uses this when a Put would exceed maxSize.
func (c *TTLCache[T]) removeOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.entries {
		if first || entry.ctime.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.ctime
			first = false
		}
	}

	if !first {
		Odl.Debug("oldest discarded", "key", oldestKey)
		delete(c.entries, oldestKey)
		c.recomputeNextExpiration()
	}
}

// shouldRemoveExpired reports whether the cache has reached the next known
// expiration time and should scan for expired entries.
func (c *TTLCache[T]) shouldRemoveExpired() bool {
	if c.nextExpiration.IsZero() {
		return false
	}
	return !time.Now().Before(c.nextExpiration)
}

// recomputeNextExpiration recalculates the earliest expiration time across all
// cached entries, or clears it when the cache is empty.
func (c *TTLCache[T]) recomputeNextExpiration() {
	if len(c.entries) == 0 {
		c.nextExpiration = time.Time{}
		return
	}

	var next time.Time
	first := true
	for _, entry := range c.entries {
		expiresAt := entry.ctime.Add(c.ttl)
		if first || expiresAt.Before(next) {
			next = expiresAt
			first = false
		}
	}

	c.nextExpiration = next
}

// SafeTTLCache Thread-safe version of the TTLCache
type SafeTTLCache[T any] struct {
	cache TTLCache[T]
	lock  sync.Mutex
}

func (c *SafeTTLCache[T]) Get(key string) (value T, found bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.cache.Get(key)
}

func (c *SafeTTLCache[T]) Put(key string, value T) T {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.cache.Put(key, value)
}

func (c *SafeTTLCache[T]) Remove(key string) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.cache.Remove(key)
}

func (c *SafeTTLCache[T]) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache.Clear()
}

// NewSafeTTLCache creates a SafeTTLCache, A thread-safe version of TTLCache.
// parameters and returns : see NewTTLCache()
func NewSafeTTLCache[T any](maxSize int, ttl time.Duration) *SafeTTLCache[T] {
	ttlCache := NewTTLCache[T](maxSize, ttl)
	if ttlCache == nil {
		return nil
	}
	newC := &SafeTTLCache[T]{
		cache: *ttlCache,
		lock:  sync.Mutex{},
	}
	return newC
}

type lruCacheEntry[T any] struct {
	key   string
	value T
}

// LRUCache implements Cache using least-recently-used eviction.
type LRUCache[T any] struct {
	maxSize int
	entries map[string]*list.Element
	order   *list.List
}

// NewLRUCache creates an LRUCache with the given maximum size.
// It returns nil when maxSize is not positive.
func NewLRUCache[T any](maxSize int) *LRUCache[T] {
	if maxSize <= 0 {
		Odl.Error("maxSize must be positive")
		return nil
	}

	return &LRUCache[T]{
		maxSize: maxSize,
		entries: make(map[string]*list.Element),
		order:   list.New(),
	}
}

// Get returns the cached value for key and marks the entry as recently used.
func (c *LRUCache[T]) Get(key string) (value T, found bool) {
	element, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, false
	}

	c.order.MoveToFront(element)
	return element.Value.(lruCacheEntry[T]).value, true
}

// Put adds or replaces a cached value and marks the entry as recently used.
// It returns the previous value for key, or the zero value when key is new.
func (c *LRUCache[T]) Put(key string, value T) T {
	// Existing keys are updated in place and promoted to most recently used.
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(lruCacheEntry[T])
		previous := entry.value
		entry.value = value
		element.Value = entry
		c.order.MoveToFront(element)
		return previous
	}

	// New keys are inserted at the front of the recency list.
	element := c.order.PushFront(lruCacheEntry[T]{
		key:   key,
		value: value,
	})
	c.entries[key] = element

	// Evict the least recently used entry when the cache exceeds its capacity.
	if c.order.Len() > c.maxSize {
		Odl.Debug("cache overflow")
		c.removeOldest()
	}

	// New keys do not have a previous value.
	var zero T
	return zero
}

// Remove deletes a cached value by key and reports whether an entry was removed.
func (c *LRUCache[T]) Remove(key string) bool {
	element, ok := c.entries[key]
	if !ok {
		return false
	}

	c.order.Remove(element)
	delete(c.entries, key)
	return true
}

// Clear removes all values from the cache.
func (c *LRUCache[T]) Clear() {
	clear(c.entries)
	c.order.Init()
}

// removeOldest evicts the least-recently-used cache entry.
func (c *LRUCache[T]) removeOldest() {
	element := c.order.Back()
	if element == nil {
		return
	}

	entry := element.Value.(lruCacheEntry[T])
	delete(c.entries, entry.key)
	c.order.Remove(element)
}

// SafeLRUCache Thread-safe version of the TTLCache
type SafeLRUCache[T any] struct {
	cache LRUCache[T]
	lock  sync.Mutex
}

func (c *SafeLRUCache[T]) Get(key string) (value T, found bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.cache.Get(key)
}

func (c *SafeLRUCache[T]) Put(key string, value T) T {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.cache.Put(key, value)
}

func (c *SafeLRUCache[T]) Remove(key string) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.cache.Remove(key)
}

func (c *SafeLRUCache[T]) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache.Clear()
}

func NewSafeLRUCache[T any](maxSize int) *SafeLRUCache[T] {
	cache := NewLRUCache[T](maxSize)
	if cache == nil {
		return nil
	}
	newC := &SafeLRUCache[T]{
		cache: *cache,
		lock:  sync.Mutex{},
	}
	return newC
}
