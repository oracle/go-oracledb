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
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestTTLCacheStoresStringPointerValue verifies the cache can store and return
// pointer values without losing the referenced string data.
func TestTTLCacheStoresStringPointerValue(t *testing.T) {
	cache := NewTTLCache[*string](2, time.Minute)
	ip := "192.168.1.10"

	cache.Put(ip, &ip)

	got, found := cache.Get(ip)
	if !found {
		t.Fatal("expected pointer value to be found")
	}
	if got == nil {
		t.Fatal("expected non-nil pointer value")
	}
	if *got != ip {
		t.Fatalf("expected IP %q, got %q", ip, *got)
	}
}

// TestTTLCacheStoresStringPointerValue verifies the cache can store and return
// pointer values without losing the referenced string data.
func TestTTLCacheStoresNilValue(t *testing.T) {
	cache := NewTTLCache[*string](2, time.Minute)
	ip := "192.168.1.10"

	cache.Put(ip, nil)

	got, found := cache.Get(ip)
	if !found {
		t.Fatal("expected pointer value to be found")
	}
	if got != nil {
		t.Fatal("expected nil pointer value")
	}

}

// TestTTLCacheExpiresEntriesIndependently verifies entries expire based on
// their own creation time.
func TestTTLCacheExpiresEntriesIndependently(t *testing.T) {
	cache := NewTTLCache[string](2, 5*time.Second)

	cache.Put("first", "192.168.1.10")
	time.Sleep(4 * time.Second)
	cache.Put("second", "192.168.1.11")
	time.Sleep(2 * time.Second)

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected first value to be expired, got %q", got)
	}

	got, found := cache.Get("second")
	if !found {
		t.Fatal("expected second value to be found")
	}
	if got != "192.168.1.11" {
		t.Fatalf("expected second value %q, got %q", "192.168.1.11", got)
	}
}

// TestTTLCacheRemovesAllExpiredEntries verifies expired entries are removed
// from the cache after the TTL has elapsed.
func TestTTLCacheRemovesAllExpiredEntries(t *testing.T) {
	cache := NewTTLCache[string](3, 3*time.Second)

	cache.Put("first", "192.168.1.10")
	cache.Put("second", "192.168.1.11")
	cache.Put("third", "192.168.1.12")

	time.Sleep(5 * time.Second)

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected first value to be expired, got %q", got)
	}
	if got, found := cache.Get("second"); found {
		t.Fatalf("expected first value to be expired, got %q", got)
	}
	if got, found := cache.Get("third"); found {
		t.Fatalf("expected first value to be expired, got %q", got)
	}

}

// TestTTLCacheNotFoundEntry verifies missing entries
func TestTTLCacheNotFoundEntry(t *testing.T) {
	cache := NewTTLCache[string](3, 3*time.Second)

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected cache to be empty, got %q", got)
	}
	cache.Put("second", "192.168.1.11")

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected entry to be missing, got %q", got)
	}
}

// TestTTLCacheClearEntries verifies missing entries
// expectations: once cleared, the cache must hold previously added values
func TestTTLCacheClearEntries(t *testing.T) {
	cache := NewTTLCache[string](3, 3*time.Second)

	cache.Put("first", "192.168.1.11")
	cache.Put("second", "192.168.1.11")
	cache.Clear()
	if got, found := cache.Get("first"); found {
		t.Fatalf("expected cache to be empty, got %q", got)
	}

	if got, found := cache.Get("second"); found {
		t.Fatalf("expected entry to be missing, got %q", got)
	}
}

// TestTTLCacheOverwriteEntry verifies overwrite of values
// expectations: Adding two different entries using the same key. Second value must overwrite the first one.
func TestTTLCacheOverwriteEntry(t *testing.T) {
	cache := NewTTLCache[string](2, time.Minute)
	if cache == nil {
		t.Fatal("expected cache instance")
	}

	previous := cache.Put("host", "10.0.0.1")
	if previous != "" {
		t.Fatalf("expected zero previous value for new key, got %q", previous)
	}

	previous = cache.Put("host", "10.0.0.2")
	if previous != "10.0.0.1" {
		t.Fatalf("expected previous value %q, got %q", "10.0.0.1", previous)
	}

	got, found := cache.Get("host")
	if !found {
		t.Fatal("expected overwritten value to be found")
	}
	if got != "10.0.0.2" {
		t.Fatalf("expected overwritten value %q, got %q", "10.0.0.2", got)
	}
}

// TestTTLCacheOverMaxSize verifies cache overflow
// expectations: Adding more values than the cache can hold, oldest values must be discarded.
func TestTTLCacheOverMaxSize(t *testing.T) {
	cache := NewTTLCache[string](2, time.Minute)

	cache.Put("first", "10.0.0.1")
	cache.Put("second", "10.0.0.2")
	cache.Put("third", "10.0.0.3")

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected oldest value to be removed, got %q", got)
	}

	got, found := cache.Get("second")
	if !found {
		t.Fatal("expected second value to remain in cache")
	}
	if got != "10.0.0.2" {
		t.Fatalf("expected second value %q, got %q", "10.0.0.2", got)
	}

	got, found = cache.Get("third")
	if !found {
		t.Fatal("expected third value to remain in cache")
	}
	if got != "10.0.0.3" {
		t.Fatalf("expected third value %q, got %q", "10.0.0.3", got)
	}
}

// TestLRUCacheStoresStringPointerValue verifies the cache can store and return
// pointer values without losing the referenced string data.
func TestLRUCacheStoresStringPointerValue(t *testing.T) {
	cache := NewLRUCache[*string](2)
	ip := "192.168.1.10"

	cache.Put(ip, &ip)

	got, found := cache.Get(ip)
	if !found {
		t.Fatal("expected pointer value to be found")
	}
	if got == nil {
		t.Fatal("expected non-nil pointer value")
	}
	if *got != ip {
		t.Fatalf("expected IP %q, got %q", ip, *got)
	}
}

// TestLRUCacheStoresNilValue verifies the cache can store and return a nil
// pointer value while still reporting that the key was found.
func TestLRUCacheStoresNilValue(t *testing.T) {
	cache := NewLRUCache[*string](2)
	ip := "192.168.1.10"

	cache.Put(ip, nil)

	got, found := cache.Get(ip)
	if !found {
		t.Fatal("expected pointer value to be found")
	}
	if got != nil {
		t.Fatal("expected nil pointer value")
	}
}

// TestLRUCacheOverMaxSize verifies the overflow behavior
// expctations : least recently used entry is evicted
//
//	when adding more values than the cache max size allows.
func TestLRUCacheOverMaxSize(t *testing.T) {
	cache := NewLRUCache[string](2)

	cache.Put("first", "10.0.0.1")
	cache.Put("second", "10.0.0.2")
	cache.Put("third", "10.0.0.3")

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected first value to be evicted, got %q", got)
	}

	got, found := cache.Get("second")
	if !found {
		t.Fatal("expected second value to remain in cache")
	}
	if got != "10.0.0.2" {
		t.Fatalf("expected second value %q, got %q", "10.0.0.2", got)
	}

	got, found = cache.Get("third")
	if !found {
		t.Fatal("expected third value to remain in cache")
	}
	if got != "10.0.0.3" {
		t.Fatalf("expected third value %q, got %q", "10.0.0.3", got)
	}
}

// TestLRUCacheGetUpdatesRecency the overflow behavior.
//   - create a cache of size 3
//   - add three elements
//   - do get call on two of them
//   - add a fourth one.
//
// expectations : the least used (the one never get) should have been discarded.
func TestLRUCacheGetUpdatesRecency(t *testing.T) {
	cache := NewLRUCache[string](3)

	cache.Put("first", "10.0.0.1")
	cache.Put("second", "10.0.0.2")
	cache.Put("third", "10.0.0.3")

	if got, found := cache.Get("second"); !found {
		t.Fatal("expected second value to be found")
	} else if got != "10.0.0.2" {
		t.Fatalf("expected second value %q, got %q", "10.0.0.2", got)
	}

	if got, found := cache.Get("third"); !found {
		t.Fatal("expected third value to be found")
	} else if got != "10.0.0.3" {
		t.Fatalf("expected third value %q, got %q", "10.0.0.3", got)
	}

	cache.Put("fourth", "10.0.0.4")

	if got, found := cache.Get("first"); found {
		t.Fatalf("expected first value to be evicted, got %q", got)
	}
}

func deleteWorker(t *testing.T, iterationCount int, cache Cache[string]) {
	t.Helper()
	for j := 0; j < iterationCount; j++ {
		key := fmt.Sprintf("k%d", j%25)
		value := fmt.Sprintf("v%d", j)

		cache.Put(key, value)

		if j%10 == 0 {
			cache.Remove(key)
		}

		if j%100 == 0 {
			cache.Clear()
		}
	}
}
func getWorker(t *testing.T, iterationCount int, cache Cache[string]) {
	t.Helper()
	for j := 0; j < iterationCount; j++ {
		key := fmt.Sprintf("k%d", j%25)
		value := fmt.Sprintf("v%d", j)
		cache.Put(key, value)
		cache.Get(key)
	}
}

func TestSafeLRUCacheConcurrency(t *testing.T) {
	cacheConcurrency(t, NewSafeLRUCache[string](20))
}

func TestSafeTTLCacheConcurrency(t *testing.T) {
	cacheConcurrency(t, NewSafeTTLCache[string](20, 3*time.Second))
}

func cacheConcurrency(t *testing.T, cache Cache[string]) {
	t.Helper()
	var workersCount = 20
	var iterations = 1000

	var wg sync.WaitGroup
	wg.Add(workersCount)
	for i := 0; i < workersCount; i++ {
		go func(flag int) {
			defer wg.Done()
			if flag == 0 {
				deleteWorker(t, iterations, cache)
			} else {
				getWorker(t, iterations, cache)
			}

		}(i % 5)
	}

	wg.Wait()
}
