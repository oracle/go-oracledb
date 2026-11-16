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
** The above copyright notice and this permission notice shall be included in
** all copies or substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
** FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
** IN THE SOFTWARE.
 */

package ttc

import (
	"sort"

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
)

// cursorCache owns the locally retained server cursors for one physical
// connection. It records use order and returns cursors that must be closed on
// the server when the configured capacity is exceeded.
type cursorCache struct {
	entries     *internalCommon.LRUCache[*Statement]
	maxSize     int
	useSequence uint64
}

// newCursorCache creates a cursor cache, or returns nil when caching is disabled.
func newCursorCache(maxSize int) *cursorCache {
	if maxSize == 0 {
		return nil
	}
	return &cursorCache{
		entries: internalCommon.NewLRUCache[*Statement](maxSize),
		maxSize: maxSize,
	}
}

// markUsed records a successful execution for LRU eviction ordering.
func (c *cursorCache) markUsed(statement *Statement) {
	if statement.qualifiedQuery.cursorID == 0 {
		return
	}
	c.useSequence++
	statement.cursorLastUsed = c.useSequence
}

// remove forgets a statement cursor before it is explicitly closed.
func (c *cursorCache) remove(statement *Statement) {
	if statement.qualifiedQuery.cursorID == 0 {
		return
	}
	c.entries.Remove(statement.cursorCacheKey())
	statement.cursorCachedAt = 0
}

// clear forgets every locally cached cursor after server reset state has closed them.
func (c *cursorCache) clear() {
	c.entries.Clear()
}

// store adds eligible statement cursors to the cache and returns cursors that
// must be closed because they no longer fit.
func (c *cursorCache) store(statements []*Statement) []*Statement {
	sort.Slice(statements, func(i, j int) bool {
		return statements[i].cursorLastUsed < statements[j].cursorLastUsed
	})

	cached := 0
	for _, statement := range statements {
		if statement.qualifiedQuery.cursorID != 0 && statement.cursorCachedAt != 0 {
			cached++
		}
	}

	var evicted []*Statement
	for _, statement := range statements {
		if statement.qualifiedQuery.cursorID == 0 || statement.cursorCachedAt == statement.cursorLastUsed {
			continue
		}

		alreadyCached := statement.cursorCachedAt != 0
		if !alreadyCached && cached == c.maxSize {
			for _, displaced := range statements {
				if displaced.qualifiedQuery.cursorID == 0 || displaced.cursorCachedAt == 0 {
					continue
				}
				c.entries.Remove(displaced.cursorCacheKey())
				displaced.cursorCachedAt = 0
				evicted = append(evicted, displaced)
				cached--
				break
			}
		}

		c.entries.Put(statement.cursorCacheKey(), statement)
		statement.cursorCachedAt = statement.cursorLastUsed
		if !alreadyCached {
			cached++
		}
	}
	return evicted
}
