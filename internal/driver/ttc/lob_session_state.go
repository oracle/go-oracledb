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
	"sync"
	"sync/atomic"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// lobSessionState contains the LOB resources owned by one physical Oracle
// session. It must be discarded with that session; a locator cannot be
// transferred to a replacement connection.
type lobSessionState struct {
	// lobReferenceRegistry tracks session-owned temporary and abstract LOB
	// references and queues server frees after their final local release.
	lobReferenceRegistry *lobReferenceRegistry
	// invalid reports whether the physical session can no longer carry out LOB operations.
	invalid atomic.Bool
	// leaseGeneration advances before this physical session is returned to
	// database/sql's pool. Adopted typed LOB handles capture the generation and
	// reject operations once their owning *sql.Conn lease has ended.
	leaseGeneration atomic.Uint64
	// leaseMu lets an adopted LOB operation hold the owning sql.Conn lease for
	// its full duration. Pool return takes the write lock before advancing the
	// generation, so it waits for active operations and blocks new ones.
	leaseMu sync.RWMutex

	// executorMu protects initialization of the session's BLOB and CLOB executors.
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

// markInvalid makes the physical session unusable for LOB operations.
func (state *lobSessionState) markInvalid() {
	state.invalid.Store(true)
}

// isInvalid reports whether the physical session can no longer carry out LOB operations.
func (state *lobSessionState) isInvalid() bool {
	return state.invalid.Load()
}

// currentLeaseGeneration returns the generation an adopted locator belongs to.
func (state *lobSessionState) currentLeaseGeneration() uint64 {
	return state.leaseGeneration.Load()
}

// expireLeases revokes all connection-scoped LOB capabilities created during
// the current database/sql lease. The lease mutex waits for any LOB operation
// that has pinned its capability before advancing the generation.
func (state *lobSessionState) expireLeases() {
	state.leaseMu.Lock()
	state.leaseGeneration.Add(1)
	state.leaseMu.Unlock()
}

// getBlobExecutor returns the session-scoped BLOB executor, initializing it
// on first use. The executor contains no LOB value state; locators, offsets,
// and buffers remain owned by their individual callers.
func (state *lobSessionState) getBlobExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType]) *blobExecutor {
	state.executorMu.Lock()
	defer state.executorMu.Unlock()
	if state.blobExecutor == nil {
		executor := newBlobExecutor()
		executor.SetShelf(shelf)
		state.blobExecutor = executor
	}
	return state.blobExecutor
}

// getClobExecutor returns the session-scoped CLOB/NCLOB executor, initializing
// it with the negotiated character-set policy on first use. Operation state is
// local to each lobDefinition, but callback registration still targets the
// shared physical MessageStreamer.
func (state *lobSessionState) getClobExecutor(
	shelf *driverCommon.Shelf[driverCommon.MessageType],
	sessionCtx *driverCommon.SessionContext,
) *clobExecutor {
	state.executorMu.Lock()
	defer state.executorMu.Unlock()
	if state.clobExecutor == nil {
		executor := newClobExecutor()
		executor.SetSessionContext(sessionCtx)
		executor.SetShelf(shelf)
		state.clobExecutor = executor
	}
	return state.clobExecutor
}
