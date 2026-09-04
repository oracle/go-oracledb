/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software and associated documentation files
** (the "Software"), to deal in the Software without restriction, including
** without limitation the rights to use, copy, modify, merge, publish,
** distribute, sublicense, and/or sell copies of the Software, and to permit
** persons to whom the Software is furnished to do so, subject to the following
** conditions:
**
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or substantial
** portions of the Software.
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
	"sync"
	"time"
)

// ExpiringCache remembers comparable keys for a fixed amount of time.
//
// It is safe for concurrent use. Entries are checked for expiry on every
// lookup. A complete cleanup is throttled so that a large cache is not scanned
// for every lookup.
type ExpiringCache[K comparable] struct {
	mu              sync.Mutex
	entries         map[K]time.Time
	ttl             time.Duration
	cleanupInterval time.Duration
	lastCleanup     time.Time
	now             func() time.Time
}

// NewExpiringCache creates an empty cache. A key remains present for ttl after
// Mark is called. cleanupInterval only controls how often expired entries are
// removed in bulk; expired keys are never reported as present.
func NewExpiringCache[K comparable](ttl, cleanupInterval time.Duration) *ExpiringCache[K] {
	return newExpiringCache(ttl, cleanupInterval, time.Now)
}

func newExpiringCache[K comparable](ttl, cleanupInterval time.Duration, now func() time.Time) *ExpiringCache[K] {
	return &ExpiringCache[K]{
		entries:         make(map[K]time.Time),
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
		now:             now,
	}
}

// Mark records key as present starting at the current time.
func (c *ExpiringCache[K]) Mark(key K) {
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeExpiredLocked(now)
	c.entries[key] = now
}

// Contains reports whether key was marked and has not expired.
func (c *ExpiringCache[K]) Contains(key K) bool {
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeExpiredLocked(now)
	markedAt, ok := c.entries[key]
	if !ok {
		return false
	}
	if now.Sub(markedAt) >= c.ttl {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *ExpiringCache[K]) removeExpiredLocked(now time.Time) {
	if c.cleanupInterval > 0 && now.Sub(c.lastCleanup) < c.cleanupInterval {
		return
	}

	for key, markedAt := range c.entries {
		if now.Sub(markedAt) >= c.ttl {
			delete(c.entries, key)
		}
	}
	c.lastCleanup = now
}
