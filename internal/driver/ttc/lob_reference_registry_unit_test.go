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
	"bytes"
	"errors"
	"testing"
)

// testReferenceLocator supplies stable identity and mutable locator bytes for
// registry state-transition tests.
type testReferenceLocator struct {
	identity lobReferenceID
	bytes    []byte
	released bool
	err      error
}

// Snapshot returns a copy of the test locator's current bytes.
func (locator *testReferenceLocator) Snapshot() []byte {
	return append([]byte(nil), locator.bytes...)
}

// MarkReleased records local release without changing the test locator bytes.
func (locator *testReferenceLocator) MarkReleased() { locator.released = true }

// LOBReferenceID returns the test identity or its configured error.
func (locator *testReferenceLocator) LOBReferenceID() (lobReferenceID, error) {
	return locator.identity, locator.err
}

// TestReferenceRegistryRejectsInvalidLocators verifies absent locators and
// identity failures are rejected before registry mutation.
func TestReferenceRegistryRejectsInvalidLocators(t *testing.T) {
	t.Parallel()
	identityErr := errors.New("identity unavailable")
	tests := []struct {
		name      string
		locator   lobReferenceLocator
		wantError error
	}{
		{name: "nil locator"},
		{name: "typed nil locator", locator: (*locator)(nil)},
		{name: "identity error", locator: &testReferenceLocator{identity: "bad", err: identityErr}, wantError: identityErr},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := newLobReferenceRegistry()
			reference, err := registry.Retain(test.locator)
			if reference != nil || err == nil {
				t.Fatalf("Retain(%v) = (%v, %v), want nil reference and error", test.locator, reference, err)
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("Retain error = %v, want %v", err, test.wantError)
			}
			if len(registry.entries) != 0 || registry.unreservedFreeCount != 0 {
				t.Fatal("invalid locator changed registry state")
			}
		})
	}
}

// TestReferenceRegistryReleaseAfterDiscardRejectsStaleReference verifies a
// retained owner cannot queue a free after session teardown discards its entry.
func TestReferenceRegistryReleaseAfterDiscardRejectsStaleReference(t *testing.T) {
	t.Parallel()
	registry := newLobReferenceRegistry()
	reference, err := registry.Retain(&testReferenceLocator{identity: "owned", bytes: []byte("locator")})
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	registry.Discard()
	if err := registry.Release(reference); err == nil {
		t.Fatal("Release accepted a reference discarded during session teardown")
	}
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after discarded reference = %v, %v; want no pending free", batch, err)
	}
}

// TestReferenceRegistryCompletionIgnoresStaleBatches verifies completion and
// restore do not remove or mark a locator that is absent or still referenced.
func TestReferenceRegistryCompletionIgnoresStaleBatches(t *testing.T) {
	t.Parallel()
	registry := newLobReferenceRegistry()
	locator := &testReferenceLocator{identity: "live", bytes: []byte("locator")}
	reference, err := registry.Retain(locator)
	if err != nil {
		t.Fatalf("retain live reference: %v", err)
	}
	staleBatch := &lobFreeBatch{referenceIDs: []lobReferenceID{"missing", "live"}}
	registry.CompletePending(staleBatch)
	registry.RestorePending(staleBatch)
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release live reference: %v", err)
	}
	if batch, err := registry.ReservePending(); err != nil || batch == nil {
		t.Fatalf("reserve after stale batch = %v, %v; want live locator free", batch, err)
	}

	// A restored batch must not complete a later reservation for the same ID.
	retryRegistry := newLobReferenceRegistry()
	retryLocator := &testReferenceLocator{identity: "retried", bytes: []byte("locator")}
	retryReference, err := retryRegistry.Retain(retryLocator)
	if err != nil {
		t.Fatalf("retain retry reference: %v", err)
	}
	if err := retryRegistry.Release(retryReference); err != nil {
		t.Fatalf("release retry reference: %v", err)
	}
	firstBatch, err := retryRegistry.ReservePending()
	if err != nil || firstBatch == nil {
		t.Fatalf("first reserve = %v, %v", firstBatch, err)
	}
	retryRegistry.RestorePending(firstBatch)
	secondBatch, err := retryRegistry.ReservePending()
	if err != nil || secondBatch == nil {
		t.Fatalf("second reserve = %v, %v", secondBatch, err)
	}
	retryRegistry.CompletePending(firstBatch)
	retryRegistry.RestorePending(secondBatch)
	if batch, err := retryRegistry.ReservePending(); err != nil || batch == nil {
		t.Fatalf("reserve after stale completion = %v, %v; want retried free", batch, err)
	}
}

// TestReferenceRegistryReferenceCountDefersFreeUntilLastAlias verifies a free
// is queued only after the last alias is released, using its latest locator.
func TestReferenceRegistryReferenceCountDefersFreeUntilLastAlias(t *testing.T) {
	registry := newLobReferenceRegistry()
	first := &testReferenceLocator{identity: "same-lob", bytes: []byte("first-locator")}
	second := &testReferenceLocator{identity: "same-lob", bytes: []byte("second-locator")}
	firstReference, err := registry.Retain(first)
	if err != nil {
		t.Fatalf("retain first: %v", err)
	}
	secondReference, err := registry.Retain(second)
	if err != nil {
		t.Fatalf("retain second: %v", err)
	}
	if err := registry.Release(firstReference); err != nil {
		t.Fatalf("release first: %v", err)
	}
	if !first.released || second.released {
		t.Fatal("release did not invalidate only the released alias")
	}
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after first release = %v, %v; want no pending free", batch, err)
	}
	second.bytes = []byte("latest-locator")
	if err := registry.Release(secondReference); err != nil {
		t.Fatalf("release second: %v", err)
	}
	if !second.released {
		t.Fatal("last released alias was not invalidated")
	}
	batch, err := registry.ReservePending()
	if err != nil {
		t.Fatalf("reserve final free: %v", err)
	}
	if batch == nil || !bytes.Equal(batch.Payload(), []byte("latest-locator")) {
		t.Fatalf("free payload = %v, want latest locator bytes", batch)
	}
	registry.CompletePending(batch)
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after commit = %v, %v; want no pending free", batch, err)
	}
}

// TestReferenceRegistryRestoreAllowsRetryAndRetain verifies a restored free
// batch permits a new alias to cancel its pending free.
func TestReferenceRegistryRestoreAllowsRetryAndRetain(t *testing.T) {
	registry := newLobReferenceRegistry()
	locator := &testReferenceLocator{identity: "retry", bytes: []byte("locator")}
	reference, err := registry.Retain(locator)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	batch, err := registry.ReservePending()
	if err != nil || batch == nil {
		t.Fatalf("reserve = %v, %v; want a batch", batch, err)
	}
	if _, err := registry.Retain(locator); err == nil {
		t.Fatal("retain succeeded while its free batch was reserved")
	}
	registry.RestorePending(batch)
	newAlias := &testReferenceLocator{identity: "retry", bytes: []byte("new-locator")}
	newReference, err := registry.Retain(newAlias)
	if err != nil {
		t.Fatalf("retain after restore: %v", err)
	}
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after new alias = %v, %v; want no pending free", batch, err)
	}
	if err := registry.Release(newReference); err != nil {
		t.Fatalf("release new alias: %v", err)
	}
	batch, err = registry.ReservePending()
	if err != nil || batch == nil || !bytes.Equal(batch.Payload(), newAlias.bytes) {
		t.Fatalf("reserve after new alias release = %v, %v; want latest locator", batch, err)
	}
}

// TestReferenceRegistryPayloadLimit verifies a batch exceeding the signed
// locator-payload limit is rejected, including when each locator fits alone.
func TestReferenceRegistryPayloadLimit(t *testing.T) {
	for _, test := range []struct {
		name     string
		locators []*testReferenceLocator
	}{
		{name: "one oversized locator", locators: []*testReferenceLocator{{identity: "oversize", bytes: []byte("long")}}},
		{name: "combined payload oversized", locators: []*testReferenceLocator{
			{identity: "first", bytes: []byte("ab")},
			{identity: "second", bytes: []byte("cd")},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := newLobReferenceRegistry()
			registry.maxBatchBytes = 3
			wantBytes := 0
			for _, locator := range test.locators {
				wantBytes += len(locator.bytes)
				reference, err := registry.Retain(locator)
				if err != nil {
					t.Fatalf("retain: %v", err)
				}
				if err := registry.Release(reference); err != nil {
					t.Fatalf("release: %v", err)
				}
			}
			if batch, err := registry.ReservePending(); err == nil || batch != nil {
				t.Fatalf("reserve oversized payload = %v, %v; want an error", batch, err)
			}
			// The failed reservation must leave every locator available for retry.
			registry.maxBatchBytes = 10
			if batch, err := registry.ReservePending(); err != nil || batch == nil || len(batch.Payload()) != wantBytes {
				t.Fatalf("reserve after raising payload limit = %v, %v; want all locator bytes", batch, err)
			}
		})
	}
}

// TestReferenceRegistryRejectsForeignReference verifies only the owning
// registry can release a reference and repeated release is harmless.
func TestReferenceRegistryRejectsForeignReference(t *testing.T) {
	owner := newLobReferenceRegistry()
	other := newLobReferenceRegistry()
	reference, err := owner.Retain(&testReferenceLocator{identity: "owned", bytes: []byte("locator")})
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := other.Release(reference); err == nil {
		t.Fatal("foreign registry accepted reference")
	}
	if err := owner.Release(reference); err != nil {
		t.Fatalf("owner release: %v", err)
	}
	if err := owner.Release(reference); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
}

// TestReferenceRegistryDiscardClearsPendingState verifies discarding the
// registry removes pending frees from later reservations.
func TestReferenceRegistryDiscardClearsPendingState(t *testing.T) {
	registry := newLobReferenceRegistry()
	locator := &testReferenceLocator{identity: "discard", bytes: []byte("locator")}
	reference, err := registry.Retain(locator)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	registry.Discard()
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after discard = %v, %v; want no pending free", batch, err)
	}
}

// TestFreeBatchPayloadReturnsCopy verifies callers cannot mutate the stored
// locator bytes through the payload accessor.
func TestFreeBatchPayloadReturnsCopy(t *testing.T) {
	batch := &lobFreeBatch{locatorBytes: []byte("locator")}
	payload := batch.Payload()
	payload[0] = 'X'
	if !bytes.Equal(batch.Payload(), []byte("locator")) {
		t.Fatalf("stored payload mutated through accessor: %q", batch.Payload())
	}
}
