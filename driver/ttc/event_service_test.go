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

import "testing"

// testEventListener records received events for event service tests.
type testEventListener struct {
	events []eventType
}

// notify appends the received event to the listener history.
//
// Parameters:
//   - event: the event type posted by the event service.
//
// Returns: none.
func (l *testEventListener) notify(event eventType) {
	l.events = append(l.events, event)
}

// TestEventServiceRegisterAndPost verifies that the event service notifies only
// listeners registered for the posted event type.
//
// Parameters:
//   - t: the active test handle.
//
// Returns: none.
func TestEventServiceRegisterAndPost(t *testing.T) {
	t.Parallel()

	service := newEventService()
	listener := &testEventListener{}

	service.register(listener, connectionInvalidatedEvent)
	service.post(connectionClosedEvent)
	if len(listener.events) != 0 {
		t.Fatalf("listener received unregistered event: %v", listener.events)
	}

	service.post(connectionInvalidatedEvent)
	if len(listener.events) != 1 || listener.events[0] != connectionInvalidatedEvent {
		t.Fatalf("listener events = %v, want [%v]", listener.events, connectionInvalidatedEvent)
	}
}
