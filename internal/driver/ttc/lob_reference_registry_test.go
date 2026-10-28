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
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	ttcLob "github.com/oracle/go-oracledb/v26/internal/driver/ttc/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type lobRegistryOrdinaryFunction struct{}

func (*lobRegistryOrdinaryFunction) GetMsgCode() common.MessageType                     { return TTIFUN }
func (*lobRegistryOrdinaryFunction) GetFuncCode() common.FunctionType                   { return ping }
func (*lobRegistryOrdinaryFunction) MarshalTo(context.Context, common.Marshaller) error { return nil }

type lobRegistryEventRecorder struct{ events []eventType }

func (recorder *lobRegistryEventRecorder) notify(event eventType) {
	recorder.events = append(recorder.events, event)
}

// newTestLobReferenceLocator returns a structurally complete test locator whose
// stable ten-byte LOB ID is deterministic. Mutable flags are outside the
// identity comparison performed by the reference registry.
func newTestLobReferenceLocator(seed byte) *locator {
	data := make(common.B1Array, kolbLobIDOffset+kolbLobIDLength)
	data[koll2FlagOffset] = kolblInitializedFlag
	data[koll4FlagOffset] = kolblTemporaryFlagByte
	for index := 0; index < kolbLobIDLength; index++ {
		data[kolbLobIDOffset+index] = seed + byte(index)
	}
	return newLocator(data, 1)
}

// TestLobReferenceRegistry_ReferenceCountDefersFreeUntilLastAlias verifies a free
// is deferred until the final local alias is released.
func TestLobReferenceRegistry_ReferenceCountDefersFreeUntilLastAlias(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	first := newTestLobReferenceLocator(1)
	second := newLocator(append(common.B1Array(nil), first.locatorBytes...), 1)
	firstReference, err := shelf.retainLobReference(first)
	if err != nil {
		t.Fatalf("retain first: %v", err)
	}
	secondReference, err := shelf.retainLobReference(second)
	if err != nil {
		t.Fatalf("retain second: %v", err)
	}

	if err := releaseLobReference(shelf, firstReference); err != nil {
		t.Fatalf("release first: %v", err)
	}
	if first.isTemporaryLocator() {
		t.Fatal("released local alias still appears temporary")
	}
	if !second.isTemporaryLocator() {
		t.Fatal("live alias was modified by another alias release")
	}

	if err := releaseLobReference(shelf, secondReference); err != nil {
		t.Fatalf("release second: %v", err)
	}
	if streamer.outgoingMessages.Len() != 0 {
		t.Fatal("last alias sent a TTC message instead of queuing its free")
	}
	if second.isTemporaryLocator() {
		t.Fatal("last released alias still appears temporary")
	}
	if err := releaseLobReference(shelf, secondReference); err != nil {
		t.Fatalf("idempotent second release: %v", err)
	}
}

// TestLobReferenceRegistry_LastReleasePiggybacksBeforeNextFunction verifies the
// final release is piggybacked before the next TTC function.
func TestLobReferenceRegistry_LastReleasePiggybacksBeforeNextFunction(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(17)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	// Simulate TTIRPA refreshing mutable locator metadata after retain. The free
	// must use the last wrapper's current bytes, not its original snapshot.
	loc.locatorBytes[kolbLobIDOffset-1] = 0xA5
	wantLocator := append(common.B1Array(nil), loc.locatorBytes...)
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if streamer.outgoingMessages.Len() != 0 {
		t.Fatal("release performed an immediate TTC operation on a piggyback-capable session")
	}

	ordinary := &tTIOall{}
	if err := streamer.Push(context.Background(), ordinary); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 2 {
		t.Fatalf("outgoing message count = %d, want piggyback plus ordinary function", got)
	}
	first := streamer.outgoingMessages.Front().Value.(common.Message[common.MessageType])
	piggyback, ok := first.(*tTIlob)
	if !ok {
		t.Fatalf("first message type = %T, want *tTIlob", first)
	}
	if piggyback.GetMsgCode() != TTIPFN || piggyback.lobPayloadDefinition.operation != kplobArrayTmpFree {
		t.Fatalf("piggyback code/operation = %v/%v", piggyback.GetMsgCode(), piggyback.lobPayloadDefinition.operation)
	}
	if !bytes.Equal(piggyback.lobPayloadDefinition.sourceLocator.locatorBytes, wantLocator) {
		t.Fatalf("piggyback locator = % X, want % X", piggyback.lobPayloadDefinition.sourceLocator.locatorBytes, wantLocator)
	}
	if streamer.outgoingMessages.Back().Value != ordinary {
		t.Fatal("ordinary TTC function was not queued after the LOB-free piggyback")
	}
}

// TestLobReferenceRegistry_RejectsRetainAfterFreeQueued verifies a queued
// temporary- or abstract-LOB free cannot be bypassed by a later retain.
func TestLobReferenceRegistry_RejectsRetainAfterFreeQueued(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(18)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}

	reused := newTestLobReferenceLocator(18)
	if _, err := shelf.retainLobReference(reused); err == nil {
		t.Fatal("retain accepted a locator whose free piggyback was already queued")
	} else {
		requireErrorCode(t, err, oracleErrors.InternalError)
	}
	if got := streamer.outgoingMessages.Len(); got != 2 {
		t.Fatalf("outgoing messages after rejected retain = %d, want queued free and ordinary function", got)
	}
}

// TestLobReferenceRegistry_ArrayPiggybackMarshalsWithOrdinaryFunction verifies an
// array-free piggyback marshals with an ordinary function.
func TestLobReferenceRegistry_ArrayPiggybackMarshalsWithOrdinaryFunction(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	buffer := NewArrayDataBuffer(4096)
	marshaller := NewMarshalEngine(buffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf.RegisterMarshaller(marshaller)
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(71)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(streamer.lobFreeBatches) != 1 {
		t.Fatalf("tracked free batches before Flush = %d, want 1", len(streamer.lobFreeBatches))
	}
	if err := streamer.Flush(context.Background()); err != nil {
		t.Fatalf("Flush piggyback plus ordinary function: %v", err)
	}
	if len(streamer.lobFreeBatches) != 0 {
		t.Fatalf("tracked free batches after Flush = %d, want 0", len(streamer.lobFreeBatches))
	}
	if buffer.currentWritePosition == 0 {
		t.Fatal("piggyback and ordinary function produced no TTC bytes")
	}
}

// TestLobReferenceRegistry_PiggybackBatchesPendingLocators verifies pending frees
// are combined into one piggyback batch.
func TestLobReferenceRegistry_PiggybackBatchesPendingLocators(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	first := newTestLobReferenceLocator(111)
	second := newTestLobReferenceLocator(112)
	for _, loc := range []*locator{first, second} {
		reference, err := shelf.retainLobReference(loc)
		if err != nil {
			t.Fatalf("retain: %v", err)
		}
		if err := releaseLobReference(shelf, reference); err != nil {
			t.Fatalf("release: %v", err)
		}
	}

	ordinary := &lobRegistryOrdinaryFunction{}
	if err := streamer.Push(context.Background(), ordinary); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}
	if streamer.outgoingMessages.Len() != 2 {
		t.Fatalf("outgoing message count = %d, want one piggyback plus ordinary function", streamer.outgoingMessages.Len())
	}
	piggyback, ok := streamer.outgoingMessages.Front().Value.(*tTIlob)
	if !ok {
		t.Fatalf("first outgoing message = %T, want *tTIlob", streamer.outgoingMessages.Front().Value)
	}
	if got, want := len(piggyback.lobPayloadDefinition.sourceLocator.locatorBytes), len(first.locatorBytes)+len(second.locatorBytes); got != want {
		t.Fatalf("array-free locator bytes = %d, want %d for both pending locators", got, want)
	}
}

// TestLobReferenceRegistry_PiggybackMarshallingFailureRestoresPendingBatch verifies
// a pre-transport marshal failure restores pending cleanup.
func TestLobReferenceRegistry_PiggybackMarshallingFailureRestoresPendingBatch(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(NewMarshalEngine(NewArrayDataBuffer(0), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(121)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	ordinary := &lobRegistryOrdinaryFunction{}
	if err := streamer.Push(context.Background(), ordinary); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := streamer.Flush(context.Background()); err == nil {
		t.Fatal("Flush succeeded with a zero-capacity marshalling buffer")
	}
	if len(streamer.lobFreeBatches) != 0 {
		t.Fatalf("tracked free batches after pre-transport marshal failure = %d, want 0", len(streamer.lobFreeBatches))
	}
	shelf.RegisterMarshaller(NewMarshalEngine(NewArrayDataBuffer(4096), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push after safe restore: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 3 {
		t.Fatalf("outgoing messages after retry = %d, want retained ordinary function plus piggyback and retry function", got)
	}
	piggybacks := 0
	for outgoing := streamer.outgoingMessages.Front(); outgoing != nil; outgoing = outgoing.Next() {
		if _, ok := outgoing.Value.(*tTIlob); ok {
			piggybacks++
		}
	}
	if piggybacks != 1 {
		t.Fatalf("queued LOB-free piggybacks after retry = %d, want 1", piggybacks)
	}
	if streamer.outgoingMessages.Front().Value != ordinary {
		t.Fatal("preexisting ordinary function was not retained after marshal failure")
	}
	if err := streamer.Flush(context.Background()); err != nil {
		t.Fatalf("Flush after retry: %v", err)
	}
}

// TestLobReferenceRegistry_PiggybackTransportFailureInvalidatesAndDiscards verifies
// an ambiguous piggyback transport failure discards session state.
func TestLobReferenceRegistry_PiggybackTransportFailureInvalidatesAndDiscards(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	buffer := NewArrayDataBuffer(4096)
	buffer.returnFlushError = true
	shelf.RegisterMarshaller(NewMarshalEngine(buffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	recorder := &lobRegistryEventRecorder{}
	shelf.getEventService().register(recorder, streamerStaleEvent)
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(122)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := streamer.Flush(context.Background()); err == nil {
		t.Fatal("Flush succeeded despite transport failure")
	}
	if len(recorder.events) != 1 || recorder.events[0] != streamerStaleEvent {
		t.Fatalf("events = %v, want [streamerStaleEvent]", recorder.events)
	}
	streamer.Drain(context.Background(), common.OUT)
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push after transport failure: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 1 {
		t.Fatalf("outgoing messages after ambiguous failure = %d, want ordinary function only", got)
	}
}

// TestLobReferenceRegistry_PiggybackIsNotRestoredAfterMainResponseFailure verifies
// cleanup is not restored after the main response fails.
func TestLobReferenceRegistry_PiggybackIsNotRestoredAfterMainResponseFailure(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	buffer := NewArrayDataBuffer(4096)
	shelf.RegisterMarshaller(NewMarshalEngine(buffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(123)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := streamer.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	buffer.currentReadPosition = buffer.currentWritePosition
	if _, err := streamer.Pull(context.Background(), TTIRPA); err == nil {
		t.Fatal("Pull succeeded without a main-RPC response")
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push after response failure: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 1 {
		t.Fatalf("outgoing messages after response failure = %d, want ordinary function only", got)
	}
}

// TestLobReferenceRegistry_PiggybackPreservesExistingPiggybackOrdering verifies an
// existing piggyback remains before the temporary-free piggyback.
func TestLobReferenceRegistry_PiggybackPreservesExistingPiggybackOrdering(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	otherPiggyback := newTTIlobPiggyback().(*tTIlob)
	if err := streamer.Push(context.Background(), otherPiggyback); err != nil {
		t.Fatalf("Push existing piggyback: %v", err)
	}
	loc := newTestLobReferenceLocator(125)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	ordinary := &lobRegistryOrdinaryFunction{}
	if err := streamer.Push(context.Background(), ordinary); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 3 {
		t.Fatalf("outgoing messages = %d, want existing piggyback, temp-free piggyback, ordinary function", got)
	}
	if got := streamer.outgoingMessages.Front().Value; got != otherPiggyback {
		t.Fatalf("first outgoing message = %T, want existing piggyback", got)
	}
	tempFree := streamer.outgoingMessages.Front().Next().Value.(*tTIlob)
	if tempFree.lobPayloadDefinition.operation != kplobArrayTmpFree {
		t.Fatalf("second piggyback operation = %v, want %v", tempFree.lobPayloadDefinition.operation, kplobArrayTmpFree)
	}
	if got := streamer.outgoingMessages.Back().Value; got != ordinary {
		t.Fatal("ordinary function was not queued after temporary-LOB piggyback")
	}
}

// TestLobReferenceRegistry_LogoffDiscardsPendingFree verifies LOGOFF drops pending
// temporary-LOB cleanup without adding a piggyback.
func TestLobReferenceRegistry_LogoffDiscardsPendingFree(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(89)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}

	logoff := newLogOff()
	if err := streamer.Push(context.Background(), logoff); err != nil {
		t.Fatalf("Push LOGOFF: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 1 {
		t.Fatalf("outgoing message count = %d, want LOGOFF without a LOB-free piggyback", got)
	}
	if streamer.outgoingMessages.Front().Value != logoff {
		t.Fatal("LOGOFF was not the only queued message")
	}
}

// TestLobReferenceRegistry_LogoffRemovesQueuedFree verifies LOGOFF removes an
// unflushed temporary-free piggyback while preserving other queued messages.
func TestLobReferenceRegistry_LogoffRemovesQueuedFree(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(90)
	reference, err := shelf.retainLobReference(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := releaseLobReference(shelf, reference); err != nil {
		t.Fatalf("release: %v", err)
	}

	ordinary := &lobRegistryOrdinaryFunction{}
	if err := streamer.Push(context.Background(), ordinary); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 2 {
		t.Fatalf("outgoing messages before LOGOFF = %d, want 2", got)
	}

	logoff := newLogOff()
	if err := streamer.Push(context.Background(), logoff); err != nil {
		t.Fatalf("Push LOGOFF: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 2 {
		t.Fatalf("outgoing messages after LOGOFF = %d, want ordinary function and LOGOFF", got)
	}
	if got := streamer.outgoingMessages.Front().Value; got != ordinary {
		t.Fatalf("first outgoing message after LOGOFF = %T, want ordinary function", got)
	}
	if got := streamer.outgoingMessages.Back().Value; got != logoff {
		t.Fatalf("last outgoing message after LOGOFF = %T, want LOGOFF", got)
	}
	for outgoing := streamer.outgoingMessages.Front(); outgoing != nil; outgoing = outgoing.Next() {
		if _, ok := outgoing.Value.(*tTIlob); ok {
			t.Fatal("LOGOFF left a temporary-free piggyback in the outgoing queue")
		}
	}
	if got := len(streamer.lobFreeBatches); got != 0 {
		t.Fatalf("tracked temporary-free batches after LOGOFF = %d, want 0", got)
	}
}

// TestLobReferenceRegistry_UsesStableLobIDNotMutableFlags verifies mutable locator
// flags do not change reference identity.
func TestLobReferenceRegistry_UsesStableLobIDNotMutableFlags(t *testing.T) {
	t.Parallel()
	first := newTestLobReferenceLocator(31)
	second := newLocator(append(common.B1Array(nil), first.locatorBytes...), 1)
	second.locatorBytes[koll1FlagOffset] |= kolblAbstractLocatorFlag
	firstID, err := (ttcLobLocator{locator: first}).ReferenceIdentity()
	if err != nil {
		t.Fatalf("first identity: %v", err)
	}
	secondID, err := (ttcLobLocator{locator: second}).ReferenceIdentity()
	if err != nil {
		t.Fatalf("second identity: %v", err)
	}
	if firstID != secondID {
		t.Fatalf("mutable flags changed identity: %q != %q", firstID, secondID)
	}
}

// TestLobReferenceRegistry_ReferenceRejectsLocatorWithoutCompleteID verifies an
// incomplete locator identity cannot be retained.
func TestLobReferenceRegistry_ReferenceRejectsLocatorWithoutCompleteID(t *testing.T) {
	t.Parallel()

	data := make(common.B1Array, koll4FlagOffset+1)
	data[koll2FlagOffset] = kolblInitializedFlag
	data[koll4FlagOffset] = kolblTemporaryFlagByte
	if _, err := (ttcLobLocator{locator: newLocator(data, 1)}).ReferenceIdentity(); err == nil {
		t.Fatal("identity accepted a temporary locator without its complete ten-byte LOB ID")
	}
}

// TestLobReferenceRegistry_ReleaseReferenceAcceptsNil verifies nil release is a
// harmless no-op.
func TestLobReferenceRegistry_ReleaseReferenceAcceptsNil(t *testing.T) {
	t.Parallel()

	if err := releaseLobReference(newShelf[common.MessageType](), nil); err != nil {
		t.Fatalf("nil reference release returned error: %v", err)
	}
}

// TestLobReferenceRegistry_ExtractRejectsIneligibleLocators verifies that only
// initialized temporary or abstract locators enter the reference registry.
func TestLobReferenceRegistry_ExtractRejectsIneligibleLocators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		locator  func() *locator
		wantCode oracleErrors.ErrorCode
	}{
		{
			name: "nil locator",
			locator: func() *locator {
				return nil
			},
			wantCode: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "short locator",
			locator: func() *locator {
				return newLocator(make(common.B1Array, kolbLobIDOffset), 1)
			},
			wantCode: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "quasi locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(131)
				loc.locatorBytes[kolbVersionOffset+1] = quasiLocatorVersion
				return loc
			},
			wantCode: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "value based locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(132)
				loc.locatorBytes[koll1FlagOffset] |= kolblValueBasedLocatorFlag
				return loc
			},
			wantCode: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "uninitialized locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(133)
				loc.locatorBytes[koll2FlagOffset] &^= kolblInitializedFlag
				return loc
			},
			wantCode: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "persistent locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(134)
				loc.locatorBytes[koll4FlagOffset] &^= kolblTemporaryFlagByte
				return loc
			},
			wantCode: oracleErrors.InvalidLOBBuffer,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := (ttcLobLocator{locator: test.locator()}).ReferenceIdentity(); err == nil {
				t.Fatal("ReferenceIdentity unexpectedly succeeded")
			} else {
				requireErrorCode(t, err, test.wantCode)
			}
		})
	}
}

// TestLobReferenceRegistry_ExtractAcceptsAbstractLocator verifies that an
// initialized abstract locator has a stable registry identity.
func TestLobReferenceRegistry_ExtractAcceptsAbstractLocator(t *testing.T) {
	t.Parallel()

	loc := newTestLobReferenceLocator(135)
	loc.locatorBytes[koll4FlagOffset] &^= kolblTemporaryFlagByte
	loc.locatorBytes[koll1FlagOffset] |= kolblAbstractLocatorFlag

	id, err := (ttcLobLocator{locator: loc}).ReferenceIdentity()
	if err != nil {
		t.Fatalf("ReferenceIdentity returned error: %v", err)
	}
	want := ttcLob.ReferenceID(string(loc.locatorBytes[kolbLobIDOffset : kolbLobIDOffset+kolbLobIDLength]))
	if id != want {
		t.Fatalf("LOB ID = %v, want %v", id, want)
	}
}
