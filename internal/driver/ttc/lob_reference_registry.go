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
	"math"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// lobReferenceLocator exposes the identity, bytes, and release transition
// needed to track temporary and abstract LOB locators on a physical session.
type lobReferenceLocator interface {
	// Snapshot returns a copy of the current locator bytes for a server free.
	Snapshot() []byte
	// MarkReleased clears the local flags that permit further locator use.
	MarkReleased()
	// LOBReferenceID validates the locator and returns its stable server ID.
	LOBReferenceID() (lobReferenceID, error)
}

// lobReferenceID groups local aliases of the same server-side LOB.
type lobReferenceID string

// lobReference represents one private Go wrapper or bind owner referring
// to a temporary or abstract Oracle LOB. released is protected by the owning
// registry mutex, making release idempotent without exposing synchronization to
// streamedLob or bind cleanup code.
type lobReference struct {
	// registry is the physical-session registry that owns this reference.
	registry *lobReferenceRegistry
	// id is the stable identity used to find the shared reference entry.
	id lobReferenceID
	// ownerLocator is this owner's locator. Release clears its local flags and
	// uses its latest bytes if this is the final owner.
	ownerLocator lobReferenceLocator
	// released prevents the same reference from decrementing its entry twice.
	released bool
}

// lobReferenceEntry stores the number of live local wrappers. The final
// release makes its locator pending for an array-free piggyback; a successful
// transport flush removes the entry.
type lobReferenceEntry struct {
	// referenceCount is the number of active local owners of this LOB.
	referenceCount uint64
	// freePending means the final owner released the LOB and its server free has
	// not completed. A reserved free remains pending until transport flush.
	freePending bool
	// reservedFreeBatch owns this entry's uncommitted array-free piggyback.
	reservedFreeBatch *lobFreeBatch
	// freeLocatorBytes is the last owner's locator snapshot for the server free.
	freeLocatorBytes driverCommon.B1Array
}

// lobFreeBatch is one reserved array-free piggyback. Its entries remain
// in the registry until the piggyback's transport flush completes successfully.
type lobFreeBatch struct {
	// referenceIDs identify the registry entries reserved by this batch.
	referenceIDs []lobReferenceID
	// locatorBytes is the concatenated locator payload for kplobArrayTmpFree.
	locatorBytes driverCommon.B1Array
}

// Payload returns a copy of the locators encoded in this array-free batch.
func (batch *lobFreeBatch) Payload() []byte {
	return append([]byte(nil), batch.locatorBytes...)
}

// lobReferenceRegistry is owned by one physical Oracle session. It groups local
// wrappers for temporary and abstract Oracle LOBs by their stable server ID
// and schedules a free after the final local reference is released.
type lobReferenceRegistry struct {
	// mu protects entries and all state reachable through them.
	mu sync.Mutex
	// entries groups local references by the stable identity embedded in each locator.
	entries map[lobReferenceID]*lobReferenceEntry
	// maxBatchBytes bounds the combined locator payload of one array-free message.
	maxBatchBytes int
	// unreservedFreeCount permits an O(1) no-work check before TTC functions.
	unreservedFreeCount int
}

// newLobReferenceRegistry creates an empty registry for one physical session.
func newLobReferenceRegistry() *lobReferenceRegistry {
	return &lobReferenceRegistry{
		// OLOBOPS carries the concatenated locator array with an SB4 length.
		maxBatchBytes: math.MaxInt32,
	}
}

// Retain registers one local owner. A new owner cancels an unreserved pending
// free for the same LOB ID. It rejects a retain after the free is reserved
// because the corresponding TTC piggyback can no longer be canceled here.
//
// Parameters:
//   - loc: temporary or abstract locator owned by the caller.
//
// Returns:
//   - *lobReference: registry-owned reference for later release.
//   - error: locator validation or already-reserved free.
func (registry *lobReferenceRegistry) Retain(loc lobReferenceLocator) (*lobReference, error) {
	if loc == nil {
		return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "missing complete LOB ID")
	}
	// Validate before changing counts. LOBReferenceID also rejects locators
	// that cannot own a server-side temporary or abstract LOB.
	id, err := loc.LOBReferenceID()
	if err != nil {
		return nil, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.entries == nil {
		registry.entries = make(map[lobReferenceID]*lobReferenceEntry)
	}
	entry := registry.entries[id]
	if entry == nil {
		entry = &lobReferenceEntry{}
		registry.entries[id] = entry
	}
	// Once a free is reserved for a TTC piggyback, another alias cannot cancel
	// it: the piggyback may already have reached the server.
	if entry.reservedFreeBatch != nil {
		return nil, common.NewOracleError(oracleErrors.InternalError, nil, "LOB free already reserved")
	}
	// A free not yet reserved can be canceled when a new alias is adopted.
	if entry.freePending {
		registry.unreservedFreeCount--
	}
	entry.referenceCount++
	entry.freePending = false
	return &lobReference{registry: registry, id: id, ownerLocator: loc}, nil
}

// Release decrements one local owner. The final release schedules its locator
// for the next kplobArrayTmpFree piggyback.
//
// Parameters:
//   - reference: non-nil local owner returned by Retain.
//
// Returns:
//   - error: ownership or registry-consistency error.
func (registry *lobReferenceRegistry) Release(reference *lobReference) error {
	if reference.registry != registry {
		return common.NewOracleError(oracleErrors.InternalError, nil, "LOB connection mismatch")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	// A second release of the same owner must not decrement the shared count.
	if reference.released {
		return nil
	}
	// Discard may have removed the entry after the owner was retained. Never
	// recreate it or queue a free for a session that is being torn down.
	entry := registry.entries[reference.id]
	if entry == nil || entry.referenceCount == 0 {
		return common.NewOracleError(oracleErrors.InternalError, nil, "LOB reference count underflow")
	}
	reference.released = true
	entry.referenceCount--
	if entry.referenceCount != 0 {
		// Other local aliases still own the server LOB. Only this locator ends.
		reference.ownerLocator.MarkReleased()
		return nil
	}

	// Locator RPCs may refresh mutable locator metadata or signatures. Free the
	// latest bytes held by the last wrapper before clearing its local flags.
	// The registry key remains the stable LOB ID captured at retain time.
	entry.freeLocatorBytes = driverCommon.B1Array(reference.ownerLocator.Snapshot())
	entry.freePending = true
	registry.unreservedFreeCount++
	reference.ownerLocator.MarkReleased()
	return nil
}

// ReservePending reserves all unreserved pending locators for one TTIPFN
// kplobArrayTmpFree message. The concatenated locator payload is bounded by
// OLOBOPS' signed 32-bit locator length. Reserved entries remain in the
// registry until CompletePending confirms the transport flush succeeded.
//
// Returns:
//   - *lobFreeBatch: reserved locators, or nil when none are pending.
//   - error: InvalidLOBBuffer when the array-free payload exceeds OLOBOPS' limit.
func (registry *lobReferenceRegistry) ReservePending() (*lobFreeBatch, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.unreservedFreeCount == 0 {
		return nil, nil
	}

	// Check the complete payload size before reserving any entry, so an
	// oversized array leaves all pending frees untouched.
	payloadBytes := 0
	for _, entry := range registry.entries {
		if !entry.freePending || entry.reservedFreeBatch != nil {
			continue
		}
		if len(entry.freeLocatorBytes) > registry.maxBatchBytes-payloadBytes {
			return nil, common.NewOracleError(
				oracleErrors.InvalidLOBBuffer,
				nil,
				"temporary-lob-free",
				"lob",
				"array-free payload too large",
			)
		}
		payloadBytes += len(entry.freeLocatorBytes)
	}

	// Reserve and copy the payload under the same lock. A later release cannot
	// change the bytes of this batch while the streamer queues its piggyback.
	batch := &lobFreeBatch{}
	batch.referenceIDs = make([]lobReferenceID, 0, registry.unreservedFreeCount)
	batch.locatorBytes = make(driverCommon.B1Array, 0, payloadBytes)
	for id, entry := range registry.entries {
		if !entry.freePending || entry.reservedFreeBatch != nil {
			continue
		}
		entry.reservedFreeBatch = batch
		batch.referenceIDs = append(batch.referenceIDs, id)
		batch.locatorBytes = append(batch.locatorBytes, entry.freeLocatorBytes...)
	}
	registry.unreservedFreeCount = 0
	if len(batch.referenceIDs) == 0 {
		return nil, nil
	}
	return batch, nil
}

// CompletePending removes entries owned by a successfully flushed array-free
// piggyback. Entries discarded or reserved again by another batch are untouched.
//
// Parameters:
//   - batch: non-nil reserved batch whose enclosing TTC flush succeeded.
func (registry *lobReferenceRegistry) CompletePending(batch *lobFreeBatch) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, id := range batch.referenceIDs {
		entry := registry.entries[id]
		// The batch identity prevents a late completion from deleting an entry
		// that was restored and reserved again by a newer piggyback.
		if entry != nil && entry.reservedFreeBatch == batch {
			delete(registry.entries, id)
		}
	}
}

// RestorePending makes a reserved batch eligible for a later piggyback when
// its queued TTC messages are discarded without a successful flush.
//
// Parameters:
//   - batch: non-nil reserved batch whose queued TTC messages were discarded.
func (registry *lobReferenceRegistry) RestorePending(batch *lobFreeBatch) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, id := range batch.referenceIDs {
		entry := registry.entries[id]
		// Restore only entries still owned by this reservation. A stale batch
		// cannot make a newer reservation pending twice.
		if entry != nil && entry.reservedFreeBatch == batch {
			entry.reservedFreeBatch = nil
			entry.freePending = true
			registry.unreservedFreeCount++
		}
	}
}

// Discard clears all local counts and pending frees for temporary and abstract
// locators. Physical connection close or invalidation leaves server cleanup to
// session teardown, so no further LOB-free TTC request is sent.
func (registry *lobReferenceRegistry) Discard() {
	registry.mu.Lock()
	clear(registry.entries)
	registry.unreservedFreeCount = 0
	registry.mu.Unlock()
}
