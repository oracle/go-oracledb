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
** one is included in the Software (each a "Larger Work" to which the Software
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
	if state == nil {
		t.Fatal("newLobSessionState returned nil")
	}
	if state.lobReferenceRegistry == nil {
		t.Fatal("new LOB session state has no LOB reference registry")
	}
	if state.isInvalidated() {
		t.Fatal("new LOB session state is invalidated")
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

// TestLobSessionState_Invalidation verifies that session invalidation is
// observable by LOB owners and remains set after repeated invalidation.
func TestLobSessionState_Invalidation(t *testing.T) {
	t.Parallel()

	var nilState *lobSessionState
	if !nilState.isInvalidated() {
		t.Fatal("nil LOB session state was treated as usable")
	}

	state := newLobSessionState()
	state.invalidate()
	if !state.isInvalidated() {
		t.Fatal("invalidated LOB session state was treated as usable")
	}
	state.invalidate()
	if !state.isInvalidated() {
		t.Fatal("repeated invalidation cleared LOB session state")
	}
}
