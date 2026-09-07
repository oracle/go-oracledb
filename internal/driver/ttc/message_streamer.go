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
	"container/list"
	"context"
	"fmt"
	"log/slog"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// StreamerPreUnmarshallCallback incoming message pre-unmarshal callback signature.
// This callback is called on a new message arrival, before unmarshalling it.
// It is mainly used to allocate new messages based on an incoming message type.
// The basic example of this case is for RPA responses. A Simple call to the message factory is not
// enough to get the right RPA reponse structure. I.e All types of RPA messages have the same message code,
// but the associated structure can be different based on operation being performed.
// In that situation, the streamer can delegate the allocation of the correct RPA response to the callback
// Such callback must not perform any action on the incoming data in the channel.
// Arguments:
//   - messageHeader: the message header
//
// Returns:
//   - a message: An allocated message that the streamer will unmarshal
//     When nil, the streamer will call the factory on the given type to retrieve the correct implementation
//   - An error: if something when wrong during the processing of the callback.
type StreamerPreUnmarshallCallback func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error)

// StreamerPostUnmarshallCallback incoming message post-unmarshal callback signature
// This callback is called on a new message arrival, after it has been unmarshalled by the streamer.
// Such callbacks can be used for asynchronous operation like handling og piggy-back messages.
// The streamer will always call this callback even if previous steps (unmarshalling) ended in error. By doing so,
// the owner of the callback can then perform any required cleanup actions.
//
// Arguments:
//
//	message: This is a reference to the message being received.
//	error: error that may have happened during previous steps in the streamer.
//	       When not nil, the state of the passed message is not guarantied.
//
// This callback returns three values:
//
//   - a boolean: boolean that tells if the current message being consumed by the streamer should be kept or not.
//     That can be the case for session related message coming from the server.
//     In such a situation, we assumed that the callback took care of what to be done client side
//     and the message should not be placed in the incoming list. This boolean flags cna also be used in error condition.
//     When an error is passed as argument, it is usually expected that the callback return 'false' for this returned element.
//
//   - An error: if something went wrong during the processing of the callback.
type StreamerPostUnmarshallCallback func(driverCommon.Message[driverCommon.MessageType], error) (bool, error)

// simple structure to hold incoming messages
// and possibly the unmarshalling error
type incomingElement struct {
	message driverCommon.Message[driverCommon.MessageType]
	err     error
}

// MessageStreamerInterface interface for TTC messages streamer
type MessageStreamerInterface interface {
	driverCommon.Streamer[driverCommon.MessageType]
	// RegisterPostUnmarshallCallback register a post-unmarshal callback
	RegisterPostUnmarshallCallback(driverCommon.MessageType, StreamerPostUnmarshallCallback)
	// RegisterPreUnmarshallCallback register a pre-unmarshal callback
	RegisterPreUnmarshallCallback(driverCommon.MessageType, StreamerPreUnmarshallCallback)
	// UnRegisterPostUnmarshallCallback unregister a pre-unmarshal callback
	UnRegisterPostUnmarshallCallback(driverCommon.MessageType)
	// UnRegisterPreUnmarshallCallback unregister a pre-unmarshal callback
	UnRegisterPreUnmarshallCallback(driverCommon.MessageType)
}

// MessageStreamer TTC message implementation of Streamer interface
type MessageStreamer struct {
	incomingMessages *list.List
	outgoingMessages *list.List
	shelf            *ttiShelf[driverCommon.MessageType]
	// lobFreeBatches keeps the registry batch associated with each queued
	// array-free piggyback until the enclosing transport flush succeeds.
	lobFreeBatches map[*tTIlob]*lobFreeBatch
	postUCallbacks map[driverCommon.MessageType]StreamerPostUnmarshallCallback
	preUCallbacks  map[driverCommon.MessageType]StreamerPreUnmarshallCallback
}

// Safety net for memory.
// Unless we face an application bug, there is no chance
// that a caller piles more than 50 messages.
const outgoingMessagesListMaxLength = 50

// or that a caller let more than 1024 messages to be parked.
const incomingMessagesListMaxLength = 1024

// NewMessageStreamer creates a message streamer, associates it with shelf, and
// registers it as a connection validator.
func NewMessageStreamer(shelf *ttiShelf[driverCommon.MessageType]) *MessageStreamer {
	streamer := &MessageStreamer{
		incomingMessages: list.New(),
		outgoingMessages: list.New(),
		preUCallbacks:    make(map[driverCommon.MessageType]StreamerPreUnmarshallCallback),
		postUCallbacks:   make(map[driverCommon.MessageType]StreamerPostUnmarshallCallback),
		shelf:            shelf,
		lobFreeBatches:   make(map[*tTIlob]*lobFreeBatch),
	}
	// register the streamer as a state validator
	shelf.registerStateValidator(streamer)
	return streamer
}

// Push implementation. See Streamer interface
func (ms *MessageStreamer) Push(ctx context.Context, msg driverCommon.Message[driverCommon.MessageType]) error {

	if msg == nil {
		common.Odl.Warn("Null message pushed to the stream")
		return common.NewOracleError(oracleErrors.InternalError, nil)
	}

	common.Odl.Debug("New push of message", "message", msg)
	if ms.outgoingMessages.Len() > outgoingMessagesListMaxLength {
		common.Odl.Warn("MessageStreamer: outgoing message queue overflow, forcing flush")
		err := ms.Flush(ctx)
		if err != nil {
			common.Odl.Warn("MessageStreamer: oemergency flush failed, drain it", "error", err)
			ms.Drain(common.BackgroundContext, driverCommon.OUT)
		}
	}

	// Temporary-LOB cleanup is carried immediately before the next
	// ordinary TTC function. LOGOFF is the exception: physical session teardown
	// releases all session-duration LOBs, so queued local state is discarded.
	// This runs after the emergency overflow flush so TTIPFN is never separated
	// from the ordinary function that gives it its piggyback transport.
	if msg.GetMsgCode() == TTIFUN {
		if function, ok := msg.(driverCommon.Function); ok && function.GetFuncCode() == logOff {
			// LOGOFF terminates the physical session, so Oracle reclaims
			// session-duration temporary LOBs as part of teardown. Do not add a
			// temporary-free RPC to the closing exchange.
			// Remove cleanup piggybacks queued for earlier functions that have not
			// been flushed yet.
			ms.discardQueuedLobFreeBatches()
			discardLobSessionState(ms.shelf)
		} else {
			// Reserve the released locators before constructing the piggyback. A
			// reserved batch cannot be selected again until this queued exchange
			// either completes or is restored after a pre-transport failure.
			batch, err := ms.shelf.lobState.lobReferenceRegistry.reservePending()
			if err != nil {
				return err
			}
			if batch != nil {
				definition := &lobDefinition{
					sourceLocator: newLocator(batch.locators, 0),
					sendLobAmt:    false,
					operation:     kplobArrayTmpFree,
				}
				piggyback := newTTIlobPiggyback().(*tTIlob)
				if err := piggyback.SetDefinition(definition); err != nil {
					ms.shelf.lobState.lobReferenceRegistry.restorePending(batch)
					return err
				}
				// Queue cleanup immediately before the ordinary function that
				// carries it in the same TTC transport flush.
				ms.lobFreeBatches[piggyback] = batch
				ms.outgoingMessages.PushBack(piggyback)
			}
		}
	}
	ms.outgoingMessages.PushBack(msg)
	return nil
}

// discardQueuedLobFreeBatches removes temporary-free piggybacks that are
// still waiting in the outgoing queue and clears their streamer bookkeeping.
// LOGOFF uses this because session teardown makes those server-side frees
// redundant.
func (ms *MessageStreamer) discardQueuedLobFreeBatches() {
	ms.removeQueuedLobFreeBatches(false)
}

// removeQueuedLobFreeBatches removes tracked LOB-free piggybacks from the
// outgoing queue and releases their streamer reservations. When restore is
// true, the registry entries become pending again for a later retry; when it
// is false, the caller is discarding the physical session.
func (ms *MessageStreamer) removeQueuedLobFreeBatches(restore bool) {
	for outgoing := ms.outgoingMessages.Front(); outgoing != nil; {
		next := outgoing.Next()
		if piggyback, ok := outgoing.Value.(*tTIlob); ok {
			if _, tracked := ms.lobFreeBatches[piggyback]; tracked {
				ms.outgoingMessages.Remove(outgoing)
			}
		}
		outgoing = next
	}
	for piggyback, batch := range ms.lobFreeBatches {
		if restore {
			ms.shelf.lobState.lobReferenceRegistry.restorePending(batch)
		}
		delete(ms.lobFreeBatches, piggyback)
	}
}

// _isExpectedType checks if a given type is part of the type array
func _isExpectedType(ts []driverCommon.MessageType, t driverCommon.MessageType) bool {
	for _, v := range ts {
		if v == t {
			return true
		}
	}
	return false
}

// Pull implementation. See Streamer interface
func (ms *MessageStreamer) Pull(ctx context.Context, expectedMessageTypes ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
	// check the incomingMessages list, if found, pop and return
	for incoming := ms.incomingMessages.Front(); incoming != nil; incoming = incoming.Next() {
		if _isExpectedType(expectedMessageTypes, incoming.Value.(*incomingElement).message.GetMsgCode()) {
			ms.incomingMessages.Remove(incoming)
			return incoming.Value.(*incomingElement).message, incoming.Value.(*incomingElement).err
		}
	}

	// do we keep reading if a message of necessary type not found?
	nextHeader := &messageHeader{}
	for {
		// read next message header
		errHeader := nextHeader.UnMarshalFrom(ctx, ms.shelf.GetMarshaller())
		if errHeader != nil {
			common.Odl.Warn("Failed to unmarshal next message header", "error", errHeader)
			return nil, common.NewOracleError(oracleErrors.StreamerReadError, errHeader, nil)
		}

		common.Odl.Info("new incoming message ", "header", nextHeader)

		var isExpectedMessageType = _isExpectedType(expectedMessageTypes, nextHeader.GetType())

		var msg driverCommon.Message[driverCommon.MessageType]
		var keep bool = true
		var processingError error
		preUnmarshalCallback := ms.preUCallbacks[nextHeader.GetType()]
		// Call preUnMarshal if registered
		if preUnmarshalCallback != nil {
			msg, processingError = preUnmarshalCallback(nextHeader)
			common.Odl.Debug("Pre-unmarshal callback registered returned",
				"message", msg, "error", processingError)
			// there is nothing we can do with error if any at this stage.
			// we must go on to raise this error at the end either returning from Pull()
			// either inserting into the incoming list
			// it is important that we consume the bytes of the incoming message whatever the callback
			// have returned.
		}
		if msg == nil {
			// if we are here that mean that either a callback was defined but did not return any implementor
			//  either, we did not have any callback. in both cases, we have to call the factory
			var ferr error
			msg, ferr = ms.getMessageForHeader(nextHeader)
			common.Odl.Debug("Getting message out of factory",
				"message", msg, "error", processingError)
			if ferr != nil {
				// there is nothing we can do anymore. in that case raise the error
				common.Odl.Warn("Error getting message from factory", "error", ferr)
				return nil, common.NewOracleError(oracleErrors.StreamerReadError, ferr, nil)
			}
		}
		marshallable, isUnMarshallable := msg.(driverCommon.UnMarshallable)
		if !isUnMarshallable {
			// we received a message that we do not know how to unmarshal, throw
			// error
			common.Odl.Warn("Message does not implmente common.UnMarshallable", "msg", msg)
			return nil, common.NewOracleError(oracleErrors.StreamerReadError, nil, nil)
		}
		uerr := marshallable.UnMarshalFrom(ctx, ms.shelf.GetMarshaller())
		if uerr != nil {
			common.Odl.Info(fmt.Sprintf("can't unmarshal message: %v", uerr))
			processingError = uerr
		}
		// Call PostUnMarshal if registered
		// even in error case we call it so we can rely on the "keep" flag
		postUnmarshalCallback := ms.postUCallbacks[nextHeader.GetType()]
		if postUnmarshalCallback != nil {
			keep, processingError = postUnmarshalCallback(msg, processingError)
			common.Odl.Debug("Post-unmarshal callback registered returned",
				"keepIt", keep, "error", processingError)
			if processingError != nil {
				// let the caller deal with the error but log something anyway
				common.Odl.Info(fmt.Sprintf("Post-unmarshal error callback failed: %v",
					processingError))
			}
		}

		if !keep {
			common.Odl.Debug("Message flagged for discard, will discard the message")
			continue
		}
		if isExpectedMessageType {
			common.Odl.Debug("New message out of streamer", "Message", msg)
			return msg, processingError
		}

		// we do not keep a faulty message
		if ms.incomingMessages.Len() >= incomingMessagesListMaxLength {
			common.Odl.Error("MessageStreamer: incoming message queue overflow")
			ms.shelf.getEventService().post(streamerOverFlowEvent)
			return nil, common.NewOracleError(oracleErrors.InternalError, nil, nil)
		}
		ms.incomingMessages.PushBack(&incomingElement{message: msg, err: processingError})
	}
}

// RegisterPreUnmarshallCallback RegisterCallback registers a callback to be called when a given message type arrives
// This overwriting existing registration
// Parameters:
//   - msgType: type the message type we register the callback for.
//   - cb: the callback to be called.
func (ms *MessageStreamer) RegisterPreUnmarshallCallback(
	msgType driverCommon.MessageType,
	cb StreamerPreUnmarshallCallback,
) {
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("Registering pre-unmarshal callback", "messageType", toString(msgType))
	}
	ms.preUCallbacks[msgType] = cb
}

// RegisterPostUnmarshallCallback RegisterCallback registers a callback to be called when a given message type arrives
// This overwriting existing registration
// Parameters:
//   - msgType: type the message type we register the callback for.
//   - cb: the callback to be called.
func (ms *MessageStreamer) RegisterPostUnmarshallCallback(
	msgType driverCommon.MessageType,
	cb StreamerPostUnmarshallCallback,
) {
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("Registering post-unmarshal callback", "messageType", toString(msgType))
	}
	ms.postUCallbacks[msgType] = cb
}

// UnRegisterPreUnmarshallCallback unregisters the callback currently assigned for the given type
// of message and event
// Parameters:
//   - msgType: type the message type we unregister the callback for.
func (ms *MessageStreamer) UnRegisterPreUnmarshallCallback(
	msgType driverCommon.MessageType) {
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("Unregistering pre callback", "messageType", toString(msgType))
	}
	delete(ms.preUCallbacks, msgType)
}

// UnRegisterPostUnmarshallCallback unregisters the callback currently assigned for the given type
// of message and event
// Parameters:
//   - msgType: type the message type we unregister the callback for.
func (ms *MessageStreamer) UnRegisterPostUnmarshallCallback(
	msgType driverCommon.MessageType) {
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("Unregistering post callback", "messageType", toString(msgType))
	}
	delete(ms.postUCallbacks, msgType)
}

// Flush implementation. See Streamer interface
func (ms *MessageStreamer) Flush(ctx context.Context) error {
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("Flush requested", "outgoing length", ms.outgoingMessages.Len())
	}

	var err error
	var next *list.Element
	for outgoing := ms.outgoingMessages.Front(); outgoing != nil; outgoing = next {
		next = outgoing.Next()
		outgoingMsg := outgoing.Value.(driverCommon.Message[driverCommon.MessageType])
		marshalable, _ := outgoingMsg.(driverCommon.Marshallable)
		header := messageHeader{
			messageType: outgoingMsg.GetMsgCode(),
		}

		err = header.MarshalTo(ctx, ms.shelf.GetMarshaller())
		if err != nil {
			common.Odl.Warn("Failed to flush, can't marshall message code", "error", err)
			ms.restorePendingLobFreeBatches()
			return common.NewOracleError(oracleErrors.StreamerWriteError, err, nil)
		}

		err = marshalable.MarshalTo(ctx, ms.shelf.GetMarshaller())
		if err != nil {
			common.Odl.Warn("Failed to flush, can't mashall message", "error", err)
			ms.restorePendingLobFreeBatches()
			return common.NewOracleError(oracleErrors.StreamerWriteError, err, nil)
		}
		ms.outgoingMessages.Remove(outgoing)
	}

	// Do not complete reserved batches until every queued message has been
	// marshalled and the underlying transport flush has succeeded. Before that
	// point a later failure can still leave the piggyback unsent.
	err = ms.shelf.GetMarshaller().Flush(ctx)
	if err != nil {
		common.Odl.Warn("Failed to flush, can't flush underlying layer", "error", err)
		// The transport may have committed some or all of the piggyback. Retrying
		// it could free an already-freed LOB, so abandon session-local state and
		// invalidate the physical connection.
		discardLobSessionState(ms.shelf)
		clear(ms.lobFreeBatches)
		ms.shelf.getEventService().post(streamerStaleEvent)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err, nil)
	}
	// A successful marshaller flush is the commit point for the reserved
	// cleanup batches: the transport has accepted the piggybacks, so their
	// local registry entries can now be removed.
	for piggyback, batch := range ms.lobFreeBatches {
		ms.shelf.lobState.lobReferenceRegistry.completePending(batch)
		delete(ms.lobFreeBatches, piggyback)
	}
	return err
}

// restorePendingLobFreeBatches returns reserved batches to the pending registry
// when queued messages are discarded before the transport flush is attempted.
// It must not be used after a transport failure because the server may have
// already processed the array-free piggyback.
func (ms *MessageStreamer) restorePendingLobFreeBatches() {
	ms.removeQueuedLobFreeBatches(true)
}

// Drain implementation. See Streamer interface
func (ms *MessageStreamer) Drain(ctx context.Context, direction driverCommon.StreamDirection) (int, int) {
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug(fmt.Sprintf("Drain requested outgoing length [%d], incoming length [%d]",
			ms.outgoingMessages.Len(),
			ms.incomingMessages.Len()))
	}
	var iRes = -1
	var oRes = -1
	if direction == driverCommon.IN || direction == driverCommon.INOUT {
		iRes = ms.incomingMessages.Len()
		ms.incomingMessages.Init()
	}
	if direction == driverCommon.OUT || direction == driverCommon.INOUT {
		// Batches still tracked here have not reached Marshaller.Flush. Restore
		// them because Drain is discarding the queued, unsent messages.
		ms.restorePendingLobFreeBatches()
		oRes = ms.outgoingMessages.Len()
		ms.outgoingMessages.Init()
	}

	return iRes, oRes
}

func (ms *MessageStreamer) getMessageForHeader(header *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
	if isFunction(header.messageType) {
		return ms.shelf.GetMessageFactory().GetMessageForFunction(header.messageType, header.functionType)
	}
	return ms.shelf.GetMessageFactory().GetMessage(header.messageType)
}

// isValid implements the stateValidator interface. When called it reports
// whether the message streamer is in a valid state.
func (ms *MessageStreamer) isValid(ctx context.Context) bool {
	msgIn, _ := ms.Drain(ctx, driverCommon.IN)
	if msgIn == 0 {
		return true
	}

	common.Odl.Error("unexpected messages remained; invalidating connection",
		"remaining messageCount", msgIn)
	ms.shelf.getEventService().post(streamerStaleEvent)
	return false
}
