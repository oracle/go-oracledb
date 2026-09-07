/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights in the Software, and under any and all patent rights owned or freely
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
	"errors"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestOperationCancellation_RequestBreakResetIsSingleUse verifies cleanup can
// reserve cancellation before a later recovery request.
func TestOperationCancellation_RequestBreakResetIsSingleUse(t *testing.T) {
	t.Parallel()

	state := newOperationCancellationState()
	state.abortBreakReset()
	if _, started := state.requestBreakReset(); started {
		t.Fatal("requestBreakReset started after cleanup had already claimed it")
	}
}

// TestOperationCancellation_RestoreRejectsUnavailableState verifies context
// cancellation is preserved when no break/reset state is available.
func TestOperationCancellation_RestoreRejectsUnavailableState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ctx  func() context.Context
	}{
		{
			name: "missing state",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
		},
		{
			name: "cleanup claimed state",
			ctx: func() context.Context {
				base, cancel := context.WithCancel(context.Background())
				state := newOperationCancellationState()
				state.abortBreakReset()
				ctx := context.WithValue(base, operationCancellationContextKey{}, state)
				cancel()
				return ctx
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Both cases must return the caller's cancellation rather than inventing a stream error.
			if _, err := restoreTTCStreamAfterCancellation(test.ctx(), &mockStreamer{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("restore error = %v, want context.Canceled", err)
			}
		})
	}
}

// TestOperationCancellation_RestoreRejectsWrongTerminalMessage verifies a
// break/reset exchange is unsafe when its terminal frame is not TTIOER.
func TestOperationCancellation_RestoreRejectsWrongTerminalMessage(t *testing.T) {
	t.Parallel()

	parent, cancelParent := context.WithCancel(context.Background())
	cancellation := newOperationCancellation()
	ctx, _, cleanup := cancellation.newCancelableOperationContext(parent, func(context.Context) error { return nil })
	defer cleanup()
	cancelParent()

	streamer := &mockStreamer{pullMsg: &dummyMsg{}}
	if message, err := cancellation.restoreTTCStream(ctx, streamer); message != nil || err == nil {
		t.Fatalf("restore result = (%v, %v), want nil ProtocolViolation", message, err)
	} else {
		requireErrorCode(t, err, oracleErrors.ProtocolViolation)
	}
}
