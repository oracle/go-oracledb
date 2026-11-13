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
	"database/sql/driver"
	"errors"
	"testing"
)

// TestNewRequestBoundaryCallbacksCreatesEmptyLists verifies callback lists start initialized and empty.
func TestNewRequestBoundaryCallbacksCreatesEmptyLists(t *testing.T) {
	t.Parallel()

	callbacks := newRequestBoundaryCallbacks()
	if callbacks == nil {
		t.Fatal("newRequestBoundaryCallbacks() = nil")
	}
	if callbacks.newConnectionCallbacks == nil || len(callbacks.newConnectionCallbacks) != 0 {
		t.Errorf("new connection callbacks = %v, want an empty initialized slice", callbacks.newConnectionCallbacks)
	}
	if callbacks.requestStartCallbacks == nil || len(callbacks.requestStartCallbacks) != 0 {
		t.Errorf("request start callbacks = %v, want an empty initialized slice", callbacks.requestStartCallbacks)
	}
	if callbacks.requestEndCallbacks == nil || len(callbacks.requestEndCallbacks) != 0 {
		t.Errorf("request end callbacks = %v, want an empty initialized slice", callbacks.requestEndCallbacks)
	}
}

// TestRequestCallbacksAppend verifies callbacks are added to their intended phases.
func TestRequestCallbacksAppend(t *testing.T) {
	t.Parallel()

	callbacks := newRequestBoundaryCallbacks()
	newConnection := &requestBoundaryCallback{description: "initialize", function: func(context.Context) error { return nil }}
	startOnCreation := &requestBoundaryCallback{description: "start on creation", function: func(context.Context) error { return nil }}
	startOnBorrow := &requestBoundaryCallback{description: "start on borrow", function: func(context.Context) error { return nil }}
	endOnReturn := &requestBoundaryCallback{description: "end on return", function: func(context.Context) error { return nil }}

	callbacks.appendNewConnectionCallback(newConnection)
	callbacks.appendRequestStartCallback(startOnCreation, true)
	callbacks.appendRequestStartCallback(startOnBorrow, false)
	callbacks.appendRequestEndCallback(endOnReturn)

	if len(callbacks.newConnectionCallbacks) != 2 || callbacks.newConnectionCallbacks[0] != newConnection || callbacks.newConnectionCallbacks[1] != startOnCreation {
		t.Errorf("new connection callbacks = %v, want initialize and start-on-creation callbacks in order", callbacks.newConnectionCallbacks)
	}
	if len(callbacks.requestStartCallbacks) != 2 || callbacks.requestStartCallbacks[0] != startOnCreation || callbacks.requestStartCallbacks[1] != startOnBorrow {
		t.Errorf("request start callbacks = %v, want both start callbacks in order", callbacks.requestStartCallbacks)
	}
	if len(callbacks.requestEndCallbacks) != 1 || callbacks.requestEndCallbacks[0] != endOnReturn {
		t.Errorf("request end callbacks = %v, want the end-on-return callback", callbacks.requestEndCallbacks)
	}
}

// TestRunRequestBoundaryCallbacksVerifiesErrorHandling verifies callback ordering and error handling.
func TestRunRequestBoundaryCallbacksVerifiesErrorHandling(t *testing.T) {
	t.Parallel()

	callbackError := errors.New("callback failed")
	tests := []struct {
		name           string
		callbackErrors []error
		wantCalls      []int
		wantErr        error
	}{
		{
			name: "empty callback list",
		},
		{
			name:           "runs callbacks in order",
			callbackErrors: []error{nil, nil},
			wantCalls:      []int{1, 2},
		},
		{
			name:           "continues after ordinary error",
			callbackErrors: []error{callbackError, nil},
			wantCalls:      []int{1, 2},
		},
		{
			name:           "stops after bad connection error",
			callbackErrors: []error{driver.ErrBadConn, nil},
			wantCalls:      []int{1},
			wantErr:        driver.ErrBadConn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := make([]int, 0, len(tt.callbackErrors))
			callbacks := make([]*requestBoundaryCallback, len(tt.callbackErrors))
			for i, callbackErr := range tt.callbackErrors {
				callNumber := i + 1
				callbackErr := callbackErr
				callbacks[i] = &requestBoundaryCallback{
					function: func(context.Context) error {
						calls = append(calls, callNumber)
						return callbackErr
					},
				}
			}

			err := runRequestBoundaryCallbacks(context.Background(), callbacks)
			if err != tt.wantErr {
				t.Errorf("runRequestBoundaryCallbacks() error = %v, want %v", err, tt.wantErr)
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("callback calls = %v, want %v", calls, tt.wantCalls)
			}
			for i := range tt.wantCalls {
				if calls[i] != tt.wantCalls[i] {
					t.Errorf("callback calls = %v, want %v", calls, tt.wantCalls)
					break
				}
			}
		})
	}
}
