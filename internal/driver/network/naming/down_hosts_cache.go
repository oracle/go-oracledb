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

package naming

import (
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
)

const (
	downHostCacheTTL             = 10 * time.Minute
	downHostCacheCleanupInterval = time.Minute
)

// sharedDownHostCache is process-wide. New iterators consult it so a failed
// connection can make later connection requests try healthier hosts first.
var sharedDownHostCache = common.NewExpiringCache[string](
	downHostCacheTTL,
	downHostCacheCleanupInterval,
)

// MarkDownHost records a host that could not be reached. The cache only
// influences connection order; it never removes a host from the attempt list.
func MarkDownHost(host string) {
	if host != "" {
		sharedDownHostCache.Mark(host)
	}
}
