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
	"context"
	"fmt"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

func TestConnectionPinger_Ping(t *testing.T) {
	t.Parallel()
	mockFac := &mockFactory{
		returnMsg: NewOall18(),
	}
	mockStr := &mockStreamer{
		pullMsg: &mockOer{err: nil},
	}

	mockNs := &mockNetworkSession{
		disconnectCalls: 0,
		disconnectErr:   nil,
		sleepDuration:   0,
		cancelErr:       nil,
	}

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(mockFac)
	shelf.RegisterMessageStreamer(mockStr)

	connection := newTestConnection(shelf, nil, mockNs)

	// Clean error message so that connection close succeeds
	mockStr.pullMsg = &mockOer{}
	err := connection.Ping(context.Background())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !mockFac.getMsgForFuncCalled {
		t.Error("GetMessageForFunction not called")
	}
	if mockFac.msgType != TTIFUN || mockFac.funcType != ping {
		t.Errorf("GetMessageForFunction called with wrong params: got %v, %v want TTIFUN, logOff", mockFac.msgType, mockFac.funcType)
	}

	if !mockStr.pushCalled {
		t.Error("Push not called")
	}
	msg := *(mockStr.pushedMsg.Front().Value.(*common.Message[common.MessageType]))
	if msg != mockFac.returnMsg {
		t.Error("Push called with wrong message")
	}

	if !mockStr.pullCalled {
		t.Error("Pull not called")
	}
	if len(mockStr.pullTypes) != 2 || (mockStr.pullTypes[0] != TTIOER && mockStr.pullTypes[1] != TTISTA) {
		t.Errorf("Pull called with wrong types: got %v want [TTIOER, TTISTA]", mockStr.pullTypes)
	}

}

func TestConnectionPinger_IsValid(t *testing.T) {
	t.Parallel()
	mockFac := &mockFactory{
		returnMsg: NewOall18(),
	}
	mockStr := &mockStreamer{
		pullMsg: &mockOer{err: nil},
	}

	mockNs := &mockNetworkSession{
		disconnectCalls: 0,
		disconnectErr:   nil,
		sleepDuration:   0,
		cancelErr:       nil,
		inband:          false,
	}

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(mockFac)
	shelf.RegisterMessageStreamer(mockStr)

	connection := newTestConnection(shelf, nil, mockNs)

	isValid := connection.IsValid()
	if !isValid {
		t.Errorf("Connection should be valid")
	}

}

func TestConnectionPinger_IsValidWithInband(t *testing.T) {
	t.Parallel()
	mockFac := &mockFactory{
		returnMsg: NewOall18(),
	}
	mockStr := &mockStreamer{
		pullMsg: &mockOer{err: fmt.Errorf("an error occurred")},
	}

	mockNs := &mockNetworkSession{
		disconnectCalls: 0,
		disconnectErr:   nil,
		sleepDuration:   0,
		cancelErr:       nil,
		inband:          true,
	}

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(mockFac)
	shelf.RegisterMessageStreamer(mockStr)

	connection := newTestConnection(shelf, nil, mockNs)

	isValid := connection.IsValid()
	if isValid {
		t.Errorf("Connection should be invalid")
	}

}
