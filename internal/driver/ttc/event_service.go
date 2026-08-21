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

import "github.com/oracle/go-oracledb/v26/internal/common"

type eventType int

const (
	connectionInvalidatedEvent eventType = iota
	connectionClosedEvent
	streamerOverFlowEvent // too many incoming messages
	streamerStaleEvent    // messages left unread
)

type eventService struct {
	listeners map[eventType][]eventListener
}

// eventListener handles posted connection events.
type eventListener interface {
	// notify is called when a registered event type is posted.
	//
	// Parameters:
	//   - event: the event type that was posted.
	//
	// Returns: none.
	notify(event eventType)
}

// newEventService creates an event dispatcher with no registered listeners.
//
// Parameters: none.
//
// Returns:
//   - *eventService: an initialized event dispatcher.
func newEventService() *eventService {
	return &eventService{
		listeners: make(map[eventType][]eventListener),
	}
}

// register subscribes listener to notifications for event.
//
// Parameters:
//   - listener: the listener to notify when event is posted. A nil listener is ignored.
//   - event: the event type to subscribe to.
//
// Returns: none.
func (s *eventService) register(listener eventListener, event eventType) {
	if listener == nil {
		// nothing else we can do and this does not deserves to add error case
		common.Odl.Warn("Event listener is nil")
		return
	}
	s.listeners[event] = append(s.listeners[event], listener)
}

// post notifies all listeners registered for event.
//
// Parameters:
//   - event: the event type to dispatch.
//
// Returns: none.
func (s *eventService) post(event eventType) {
	for _, listener := range s.listeners[event] {
		listener.notify(event)
	}
}
