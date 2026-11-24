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
	"testing"

	common2 "github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestConnectionResetter_ResetPreservesStatements(t *testing.T) {
	t.Parallel()
	mockFac := &mockFactory{
		returnMsg: newOall18(),
		logger:    &common2.Odl,
	}
	mockStr := &mockStreamer{
		pullMsg: &mockOer{err: nil},
	}

	mockNs := &mockNetworkSession{
		disconnectErr: nil,
		sleepDuration: 0,
		cancelErr:     nil,
	}
	mockNs.disconnectCalls.Store(0)
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMessageFactory(mockFac)
	shelf.RegisterMessageStreamer(mockStr)

	sessionCtx := common.NewSessionContext()

	connection := newTestConnection(shelf, sessionCtx, mockNs)

	prepared, err := connection.PrepareContext(context.Background(), "SELECT * FROM DUAL")
	if err != nil {
		t.Fatalf("PrepareContext() error = %v", err)
	}
	statement := prepared.(*Statement)
	statement.qualifiedQuery.cursorID = 1

	sessionCtx.GetSessionProperties().SetProperty("pasta", "carbonara")

	if err := connection.ResetSession(context.Background()); err != nil {
		t.Fatalf("ResetSession() error = %v", err)
	}
	if statement.qualifiedQuery.cursorID != 1 {
		t.Errorf("prepared statement cursor = %d, want 1", statement.qualifiedQuery.cursorID)
	}

	if v := sessionCtx.GetSessionProperties().GetProperty("pasta"); v != nil {
		t.Errorf("session properties should have been reset")
	}

}
