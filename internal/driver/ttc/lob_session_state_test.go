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

package ttc

import (
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestLobSessionState_InitializesAndCachesExecutors verifies that session LOB
// state creates its registry and lazily reuses one executor per LOB family.
func TestLobSessionState_InitializesAndCachesExecutors(t *testing.T) {
	t.Parallel()

	state := newLobSessionState()
	if state.lobReferenceRegistry == nil {
		t.Fatal("new LOB session state has no LOB reference registry")
	}
	if state.isInvalid() {
		t.Fatal("new LOB session state is invalidated")
	}
	if state.blobExecutor != nil || state.clobExecutor != nil {
		t.Fatal("new LOB session state initialized executors before use")
	}

	shelf := newShelf[common.MessageType]()
	sessionContext := newTestSessionContext()
	blob := state.getBlobExecutor(shelf.Shelf)
	if blob == nil || state.getBlobExecutor(shelf.Shelf) != blob {
		t.Fatal("BLOB executor was not initialized and cached")
	}
	clob := state.getClobExecutor(shelf.Shelf, sessionContext)
	if clob == nil || state.getClobExecutor(shelf.Shelf, sessionContext) != clob {
		t.Fatal("CLOB executor was not initialized and cached")
	}
}

// TestLobSessionState_ConcurrentExecutorInitialization verifies simultaneous
// first requests share one executor of each LOB family.
func TestLobSessionState_ConcurrentExecutorInitialization(t *testing.T) {
	t.Parallel()

	state := newLobSessionState()
	shelf := newShelf[common.MessageType]()
	sessionContext := newTestSessionContext()
	const callers = 16
	type executors struct {
		blob *blobExecutor
		clob *clobExecutor
	}
	start := make(chan struct{})
	results := make(chan executors, callers)
	for range callers {
		go func() {
			<-start
			results <- executors{
				blob: state.getBlobExecutor(shelf.Shelf),
				clob: state.getClobExecutor(shelf.Shelf, sessionContext),
			}
		}()
	}
	close(start)
	first := <-results
	if first.blob == nil || first.clob == nil {
		t.Fatal("concurrent initialization returned nil executor")
	}
	for range callers - 1 {
		got := <-results
		if got.blob != first.blob || got.clob != first.clob {
			t.Fatal("concurrent initialization returned different executors")
		}
	}
}

// TestLobSessionState_Invalidation verifies that marking the LOB session
// invalid sets a state flag that remains set after repeated calls.
func TestLobSessionState_Invalidation(t *testing.T) {
	t.Parallel()

	state := newLobSessionState()
	state.markInvalid()
	if !state.isInvalid() {
		t.Fatal("invalidated LOB session state was treated as usable")
	}
	state.markInvalid()
	if !state.isInvalid() {
		t.Fatal("repeated invalidation cleared LOB session state")
	}
}
