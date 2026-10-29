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
	"errors"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// lobRegistryOrdinaryFunction is a TTC function used to carry a queued free.
type lobRegistryOrdinaryFunction struct{}

// GetMsgCode identifies this test message as a TTC function.
func (*lobRegistryOrdinaryFunction) GetMsgCode() common.MessageType { return TTIFUN }

// GetFuncCode identifies the ordinary function used by the test streamer.
func (*lobRegistryOrdinaryFunction) GetFuncCode() common.FunctionType { return ping }

// MarshalTo writes no payload so tests can isolate the free piggyback.
func (*lobRegistryOrdinaryFunction) MarshalTo(context.Context, common.Marshaller) error { return nil }

// lobRegistryFailingFunction fails while marshalling the ordinary function.
type lobRegistryFailingFunction struct{ lobRegistryOrdinaryFunction }

// MarshalTo simulates a failure after the array-free piggyback is marshalled.
func (*lobRegistryFailingFunction) MarshalTo(context.Context, common.Marshaller) error {
	return errors.New("test marshal failure after LOB free")
}

// lobRegistryEventRecorder records stale-stream events in integration tests.
type lobRegistryEventRecorder struct{ events []eventType }

// notify appends one event to the recorder.
func (recorder *lobRegistryEventRecorder) notify(event eventType) {
	recorder.events = append(recorder.events, event)
}

// TestLobReferenceRegistry_ReferenceCountDefersFreeUntilLastAlias verifies each
// alias is invalidated on its own release and no free is sent immediately.
func TestLobReferenceRegistry_ReferenceCountDefersFreeUntilLastAlias(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	first := newTestLobReferenceLocator(1)
	second := newLocator(append(common.B1Array(nil), first.locatorBytes...), 1)
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
	if first.isTemporaryLocator() {
		t.Fatal("released local alias still appears temporary")
	}
	if !second.isTemporaryLocator() {
		t.Fatal("live alias was modified by another alias release")
	}

	if err := registry.Release(secondReference); err != nil {
		t.Fatalf("release second: %v", err)
	}
	if streamer.outgoingMessages.Len() != 0 {
		t.Fatal("last alias sent a TTC message instead of queuing its free")
	}
	if second.isTemporaryLocator() {
		t.Fatal("last released alias still appears temporary")
	}
	if err := registry.Release(secondReference); err != nil {
		t.Fatalf("idempotent second release: %v", err)
	}
}

// TestLobReferenceRegistry_LastReleasePiggybacksBeforeNextFunction verifies the
// final release is piggybacked before the next TTC function.
func TestLobReferenceRegistry_LastReleasePiggybacksBeforeNextFunction(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(17)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	// Simulate TTIRPA refreshing mutable locator metadata after retain. The free
	// must use the last wrapper's current bytes, not its original snapshot.
	loc.locatorBytes[kolbLobIDOffset-1] = 0xA5
	wantLocator := append(common.B1Array(nil), loc.locatorBytes...)
	if err := registry.Release(reference); err != nil {
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
// temporary-LOB free cannot be bypassed by a later retain of the same ID.
func TestLobReferenceRegistry_RejectsRetainAfterFreeQueued(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(18)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}

	reused := newTestLobReferenceLocator(18)
	if _, err := registry.Retain(reused); err == nil {
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
	registry := shelf.lobState.lobReferenceRegistry
	buffer := NewArrayDataBuffer(4096)
	marshaller := NewMarshalEngine(buffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	shelf.RegisterMarshaller(marshaller)
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(71)
	wantLocator := loc.Snapshot()
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
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
	encoded := buffer.bytes[:buffer.currentWritePosition]
	if !buffer.hasBeenFlushed || len(encoded) == 0 || encoded[0] != byte(TTIPFN) {
		t.Fatalf("flushed TTC bytes = % X; want array-free piggyback first", encoded)
	}
	if encoded[len(encoded)-1] != byte(TTIFUN) {
		t.Fatalf("flushed TTC bytes = % X; want ordinary function after piggyback", encoded)
	}
	if !bytes.Contains(encoded, wantLocator) {
		t.Fatalf("flushed TTC bytes do not contain the free locator: % X", encoded)
	}
}

// TestLobReferenceRegistry_PiggybackBatchesPendingLocators verifies pending frees
// are combined into one piggyback batch.
func TestLobReferenceRegistry_PiggybackBatchesPendingLocators(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	first := newTestLobReferenceLocator(111)
	second := newTestLobReferenceLocator(112)
	firstSnapshot := first.Snapshot()
	secondSnapshot := second.Snapshot()
	for _, loc := range []*locator{first, second} {
		reference, err := registry.Retain(loc)
		if err != nil {
			t.Fatalf("retain: %v", err)
		}
		if err := registry.Release(reference); err != nil {
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
	payload := piggyback.lobPayloadDefinition.sourceLocator.locatorBytes
	forward := append(append([]byte(nil), firstSnapshot...), secondSnapshot...)
	reverse := append(append([]byte(nil), secondSnapshot...), firstSnapshot...)
	if !bytes.Equal(payload, forward) && !bytes.Equal(payload, reverse) {
		t.Fatalf("array-free locator bytes = % X, want both locators once", payload)
	}
}

// TestLobReferenceRegistry_DrainBeforeMarshalRestoresPendingFree verifies
// draining an unflushed queue restores the pending LOB free for retry.
func TestLobReferenceRegistry_DrainBeforeMarshalRestoresPendingFree(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(120)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	_, drained := streamer.Drain(context.Background(), common.OUT)
	if drained != 2 {
		t.Fatalf("drained outgoing messages = %d, want free piggyback and function", drained)
	}
	if shelf.lobState.isInvalid() {
		t.Fatal("draining an unflushed queue invalidated the session")
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push after drain: %v", err)
	}
	if got := streamer.outgoingMessages.Len(); got != 2 {
		t.Fatalf("outgoing messages after retry = %d, want restored free and function", got)
	}
	if got := len(streamer.lobFreeBatches); got != 1 {
		t.Fatalf("tracked free batches after retry = %d, want 1", got)
	}
}

// TestLobReferenceRegistry_PiggybackMarshallingFailureInvalidatesSession verifies
// an uncertain marshal failure never retries temporary-LOB cleanup.
func TestLobReferenceRegistry_PiggybackMarshallingFailureInvalidatesSession(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	shelf.RegisterMarshaller(NewMarshalEngine(NewArrayDataBuffer(0), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(121)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := streamer.Flush(context.Background()); err == nil {
		t.Fatal("Flush succeeded with a zero-capacity marshalling buffer")
	}
	if !shelf.lobState.isInvalid() {
		t.Fatal("marshal failure left the physical LOB session usable")
	}
	if len(streamer.lobFreeBatches) != 0 || streamer.outgoingMessages.Len() != 0 {
		t.Fatal("marshal failure retained queued messages or free batches")
	}
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after invalidation = %v, %v; want no pending free", batch, err)
	}
}

// TestLobReferenceRegistry_MarshalFailureAfterPiggybackInvalidatesSession
// verifies a later marshal failure abandons the queued free and LOB session.
func TestLobReferenceRegistry_MarshalFailureAfterPiggybackInvalidatesSession(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	shelf.RegisterMarshaller(NewMarshalEngine(NewArrayDataBuffer(4096), common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(122)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryFailingFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := streamer.Flush(context.Background()); err == nil {
		t.Fatal("Flush succeeded despite the function marshal failure")
	}
	if !shelf.lobState.isInvalid() || len(streamer.lobFreeBatches) != 0 || streamer.outgoingMessages.Len() != 0 {
		t.Fatal("marshal failure after piggyback did not abandon the session and queue")
	}
}

// TestLobReferenceRegistry_PiggybackTransportFailureInvalidatesAndDiscards verifies
// an ambiguous piggyback transport failure discards session state.
func TestLobReferenceRegistry_PiggybackTransportFailureInvalidatesAndDiscards(t *testing.T) {
	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	buffer := NewArrayDataBuffer(4096)
	buffer.returnFlushError = true
	shelf.RegisterMarshaller(NewMarshalEngine(buffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	recorder := &lobRegistryEventRecorder{}
	shelf.getEventService().register(recorder, streamerStaleEvent)
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(122)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
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
	if !shelf.lobState.isInvalid() {
		t.Fatal("transport failure left the LOB session usable")
	}
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after transport failure = %v, %v; want discarded frees", batch, err)
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
	registry := shelf.lobState.lobReferenceRegistry
	buffer := NewArrayDataBuffer(4096)
	shelf.RegisterMarshaller(NewMarshalEngine(buffer, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal}))
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(123)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := streamer.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, err := registry.Retain(newTestLobReferenceLocator(123)); err != nil {
		t.Fatalf("retain after flushed free: %v; want completed reservation", err)
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
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	otherPiggyback := newTTIlobPiggyback().(*tTIlob)
	if err := streamer.Push(context.Background(), otherPiggyback); err != nil {
		t.Fatalf("Push existing piggyback: %v", err)
	}
	loc := newTestLobReferenceLocator(125)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
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

// TestLobReferenceRegistry_LogoffDiscardsPendingFree verifies LOGOFF drops
// pending temporary-LOB cleanup and invalidates local LOB state.
func TestLobReferenceRegistry_LogoffDiscardsPendingFree(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(89)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
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
	if !shelf.lobState.isInvalid() {
		t.Fatal("LOGOFF left local LOB session state usable")
	}
	if batch, err := registry.ReservePending(); err != nil || batch != nil {
		t.Fatalf("reserve after LOGOFF = %v, %v; want discarded frees", batch, err)
	}
}

// TestLobReferenceRegistry_LogoffRemovesQueuedFree verifies LOGOFF removes an
// unflushed temporary-free piggyback while preserving other queued messages.
func TestLobReferenceRegistry_LogoffRemovesQueuedFree(t *testing.T) {
	t.Parallel()

	shelf := newShelf[common.MessageType]()
	registry := shelf.lobState.lobReferenceRegistry
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(90)
	reference, err := registry.Retain(loc)
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	if err := registry.Release(reference); err != nil {
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
