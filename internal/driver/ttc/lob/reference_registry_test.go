/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** See the LICENSE file for license terms.
 */

package lob

import (
	"bytes"
	"testing"
)

type testReferenceLocator struct {
	identity ReferenceID
	bytes    []byte
	released bool
	err      error
}

func (locator *testReferenceLocator) Snapshot() []byte {
	return append([]byte(nil), locator.bytes...)
}

func (locator *testReferenceLocator) MarkReleased() { locator.released = true }

func (locator *testReferenceLocator) ReferenceIdentity() (ReferenceID, error) {
	return locator.identity, locator.err
}

func TestReferenceRegistryReferenceCountDefersFreeUntilLastAlias(t *testing.T) {
	registry := newReferenceRegistry()
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

func TestReferenceRegistryRestoreAllowsRetryAndRetain(t *testing.T) {
	registry := newReferenceRegistry()
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
	if _, err := registry.Retain(locator); err != nil {
		t.Fatalf("retain after restore: %v", err)
	}
}

func TestReferenceRegistryPayloadLimit(t *testing.T) {
	registry := newReferenceRegistry()
	registry.maxPendingBytes = 2
	locator := &testReferenceLocator{identity: "oversize", bytes: []byte("long")}
	reference, err := registry.Retain(locator)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if batch, err := registry.ReservePending(); err == nil || batch != nil {
		t.Fatalf("reserve oversized payload = %v, %v; want an error", batch, err)
	}
}

func TestReferenceRegistryRejectsForeignReference(t *testing.T) {
	owner := newReferenceRegistry()
	other := newReferenceRegistry()
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

func TestReferenceRegistryDiscardClearsPendingState(t *testing.T) {
	registry := newReferenceRegistry()
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

func TestFreeBatchPayloadReturnsCopy(t *testing.T) {
	batch := &freeBatch{locators: []byte("locator")}
	payload := batch.Payload()
	payload[0] = 'X'
	if !bytes.Equal(batch.Payload(), []byte("locator")) {
		t.Fatalf("stored payload mutated through accessor: %q", batch.Payload())
	}
}
