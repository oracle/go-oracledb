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
	"sync"
	"sync/atomic"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// lobSessionState contains the LOB resources owned by one physical Oracle
// session. It must be discarded with that session; a locator cannot be
// transferred to a replacement connection.
type lobSessionState struct {
	lobReferenceRegistry *lobReferenceRegistry
	// invalidated is set when the physical session can no longer safely carry
	// out LOB operations. It is read by escaped query LOBs before admission to
	// the TTC stream.
	invalidated atomic.Bool

	// executorMu protects lazy initialization of the immutable type-specific
	// executors shared by all LOB operations on this physical session. The
	// executors are protocol helpers, not stream locks; every caller must hold
	// the complete physical-session synchronizer while using one.
	executorMu sync.Mutex
	// blobExecutor is the session-scoped BLOB protocol helper.
	blobExecutor *blobExecutor
	// clobExecutor is the session-scoped CLOB/NCLOB protocol helper.
	clobExecutor *clobExecutor
}

// newLobSessionState constructs empty LOB state for a physical session.
func newLobSessionState() *lobSessionState {
	return &lobSessionState{lobReferenceRegistry: newLobReferenceRegistry()}
}

// invalidate marks the physical session unusable for all LOB values.
func (state *lobSessionState) invalidate() {
	if state != nil {
		state.invalidated.Store(true)
	}
}

// isInvalidated reports whether the physical session can no longer carry out
// LOB operations. A missing state is treated as invalid rather than usable.
func (state *lobSessionState) isInvalidated() bool {
	return state == nil || state.invalidated.Load()
}

// getBlobExecutor returns the session-scoped BLOB executor, initializing it
// on first use. The executor contains no LOB value state; locators, offsets,
// and buffers remain owned by their individual callers. Its caller must hold
// the complete physical-session synchronizer while performing an exchange.
func (state *lobSessionState) getBlobExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType]) *blobExecutor {
	state.executorMu.Lock()
	defer state.executorMu.Unlock()
	if state.blobExecutor == nil {
		state.blobExecutor = newBlobExecutor(shelf)
	}
	return state.blobExecutor
}

// getClobExecutor returns the session-scoped CLOB/NCLOB executor, initializing
// it with the negotiated character-set policy on first use. Operation state is
// local to each lobDefinition, but callback registration still targets the
// shared physical MessageStreamer; its caller must hold the complete
// physical-session synchronizer while performing an exchange.
func (state *lobSessionState) getClobExecutor(
	shelf *driverCommon.Shelf[driverCommon.MessageType],
	sessionCtx *driverCommon.SessionContext,
) *clobExecutor {
	state.executorMu.Lock()
	defer state.executorMu.Unlock()
	if state.clobExecutor == nil {
		state.clobExecutor = newClobExecutor(shelf, sessionCtx)
	}
	return state.clobExecutor
}
