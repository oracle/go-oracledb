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

package lob

import (
	"math"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// lobReference represents one private Go wrapper or bind owner referring
// to a temporary or abstract Oracle LOB. released is protected by the owning
// registry mutex, making release idempotent without exposing synchronization to
// streamedLob or bind cleanup code.
type lobReference struct {
	// registry is the physical-session registry that owns this reference.
	registry *referenceRegistry
	// id is the stable identity used to find the shared reference entry.
	id ReferenceID
	// locator is a private snapshot used if the local locator is no longer
	// available when the final reference is released.
	locator driverCommon.B1Array
	// local is the caller-owned locator whose local flags are cleared on release.
	local Locator
	// released prevents the same reference from decrementing its entry twice.
	released bool
}

// referenceRegistry is owned by one physical Oracle session. It groups local
// wrappers for temporary and abstract Oracle LOBs by their identity span and
// schedules server cleanup only after the final local reference is released.
type referenceRegistry struct {
	// mu protects entries and all state reachable through them.
	mu sync.Mutex
	// entries groups local references by the stable identity embedded in each locator.
	entries map[ReferenceID]*lobReferenceEntry
	// maxPendingBytes is the maximum combined locator size accepted by one
	// kplobArrayTmpFree payload.
	maxPendingBytes int
}

// lobReferenceEntry stores the number of live local wrappers. pending is set
// after the count reaches zero and is freed by the next array piggyback.
type lobReferenceEntry struct {
	// references is the number of active local references for this LOB identity.
	references uint64
	// pending indicates that the final release has made this locator eligible
	// for an array-free piggyback.
	pending bool
	// queued indicates that this entry is already included in an unsent or
	// uncommitted array-free piggyback.
	queued bool
	// locator contains the latest locator bytes to send when the entry is freed.
	locator driverCommon.B1Array
}

// freeBatch is one reserved array-free piggyback. Its entries remain
// in the registry until the piggyback's transport flush completes successfully.
type freeBatch struct {
	// ids identifies the registry entries reserved by this batch.
	ids []ReferenceID
	// locators is the concatenated locator payload for kplobArrayTmpFree.
	locators driverCommon.B1Array
}

var _ Reference = (*lobReference)(nil)
var _ FreeBatch = (*freeBatch)(nil)
var _ ReferenceRegistry = (*referenceRegistry)(nil)

// NewReferenceRegistry creates an empty registry for one physical session.
func NewReferenceRegistry() ReferenceRegistry {
	return &referenceRegistry{
		entries: make(map[ReferenceID]*lobReferenceEntry),
		// OLOBOPS carries the concatenated locator array with an SB4 length.
		maxPendingBytes: math.MaxInt32,
	}
}

func newReferenceRegistry() *referenceRegistry {
	return NewReferenceRegistry().(*referenceRegistry)
}

// extractReferenceID asks the TTC locator adapter for its opaque identity.
//
// Parameters:
//   - loc: locator eligible for local reference tracking.
//
// Returns:
//   - ReferenceID: opaque identity supplied by the TTC adapter.
//   - error: InvalidLOBBuffer for an ineligible or malformed locator.
func extractReferenceID(loc Locator) (ReferenceID, error) {
	if loc == nil {
		return "", common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"lob-reference",
			"lob",
			"missing complete LOB ID",
		)
	}
	return loc.ReferenceIdentity()
}

// Retain registers one local owner. A new owner cancels an unqueued pending
// free for the same LOB ID. It rejects a retain after the free has been queued
// because the corresponding TTC piggyback can no longer be canceled here.
//
// Parameters:
//   - loc: temporary or abstract locator owned by the caller.
//
// Returns:
//   - Reference: registry-owned reference for later release.
//   - error: locator validation or queued-free failure.
func (registry *referenceRegistry) Retain(loc Locator) (Reference, error) {
	id, err := extractReferenceID(loc)
	if err != nil {
		return nil, err
	}
	snapshot := driverCommon.B1Array(loc.Snapshot())

	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry := registry.entries[id]
	if entry == nil {
		entry = &lobReferenceEntry{}
		registry.entries[id] = entry
	}
	if entry.queued {
		return nil, common.NewOracleError(oracleErrors.InternalError, nil, "LOB free already queued")
	}
	entry.references++
	entry.pending = false
	entry.locator = snapshot
	return &lobReference{registry: registry, id: id, locator: snapshot, local: loc}, nil
}

func (*lobReference) lobReferenceToken() {}

// Release decrements one local owner. The final release schedules its locator
// for the next kplobArrayTmpFree piggyback.
//
// Parameters:
//   - reference: previously retained local owner.
//   - error: ownership or registry-consistency error.
func (registry *referenceRegistry) Release(value Reference) error {
	if value == nil {
		return nil
	}
	reference, ok := value.(*lobReference)
	if !ok || reference == nil || reference.registry == nil {
		return common.NewOracleError(oracleErrors.InternalError, nil, "invalid LOB reference")
	}
	if reference.registry != registry {
		return common.NewOracleError(oracleErrors.InternalError, nil, "LOB connection mismatch")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if reference.released {
		return nil
	}
	reference.released = true
	entry := registry.entries[reference.id]
	if entry == nil || entry.references == 0 {
		return common.NewOracleError(oracleErrors.InternalError, nil, "LOB reference count underflow")
	}
	entry.references--
	if entry.references != 0 {
		if reference.local != nil {
			reference.local.MarkReleased()
		}
		return nil
	}

	// Locator RPCs may refresh mutable locator metadata or signatures. Free the
	// latest bytes held by the last wrapper, while the registry key continues to
	// use only the stable LOB ID captured at retain time.
	latest := reference.locator
	if reference.local != nil {
		latest = driverCommon.B1Array(reference.local.Snapshot())
	}
	entry.locator = append(entry.locator[:0], latest...)
	entry.pending = true
	if reference.local != nil {
		reference.local.MarkReleased()
	}
	return nil
}

// ReservePending reserves all pending locators for one TTIPFN
// kplobArrayTmpFree message. The concatenated locator payload is bounded by
// OLOBOPS' signed 32-bit locator length. Reserved entries remain in the
// registry until completePending confirms the transport flush succeeded.
//
// Returns:
//   - FreeBatch: reserved locators, or nil when none are pending.
//   - error: InvalidLOBBuffer when the array-free payload exceeds OLOBOPS' limit.
func (registry *referenceRegistry) ReservePending() (FreeBatch, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	total := 0
	for _, entry := range registry.entries {
		if !entry.pending || entry.queued || entry.references != 0 {
			continue
		}
		if len(entry.locator) > registry.maxPendingBytes-total {
			return nil, common.NewOracleError(
				oracleErrors.InvalidLOBBuffer,
				nil,
				"temporary-lob-free",
				"lob",
				"array-free payload too large",
			)
		}
		total += len(entry.locator)
	}

	batch := &freeBatch{}
	batch.ids = make([]ReferenceID, 0)
	batch.locators = make(driverCommon.B1Array, 0, total)
	for id, entry := range registry.entries {
		if !entry.pending || entry.queued || entry.references != 0 {
			continue
		}
		entry.queued = true
		batch.ids = append(batch.ids, id)
		batch.locators = append(batch.locators, entry.locator...)
	}
	if len(batch.ids) == 0 {
		return nil, nil
	}
	return batch, nil
}

// CompletePending removes entries released by a successfully flushed
// array-free piggyback. A new local reference cancels deletion for that LOB.
//
// Parameters:
//   - batch: reserved locators whose enclosing TTC flush succeeded.
func (registry *referenceRegistry) CompletePending(value FreeBatch) {
	if value == nil {
		return
	}
	batch, ok := value.(*freeBatch)
	if !ok || batch == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, id := range batch.ids {
		entry := registry.entries[id]
		if entry != nil && entry.queued && entry.pending && entry.references == 0 {
			delete(registry.entries, id)
		}
	}
}

// RestorePending makes a reserved batch eligible for a later piggyback when
// its queued TTC messages are discarded without a successful flush.
//
// Parameters:
//   - batch: reserved locators whose queued TTC messages were discarded.
func (registry *referenceRegistry) RestorePending(value FreeBatch) {
	if value == nil {
		return
	}
	batch, ok := value.(*freeBatch)
	if !ok || batch == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, id := range batch.ids {
		entry := registry.entries[id]
		if entry != nil && entry.references == 0 {
			entry.queued = false
			entry.pending = true
		}
	}
}

// Discard clears all local counts and pending frees for temporary and abstract
// locators. Physical connection close or invalidation transfers cleanup to
// Oracle session teardown, so no further TTC request is safe or necessary.
func (registry *referenceRegistry) Discard() {
	registry.mu.Lock()
	clear(registry.entries)
	registry.mu.Unlock()
}

func (batch *freeBatch) Payload() []byte {
	if batch == nil {
		return nil
	}
	return append([]byte(nil), batch.locators...)
}
