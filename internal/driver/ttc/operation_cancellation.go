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
	"sync"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// cancelTimeout bounds the break/reset exchange started by an operation's
	// cancellation callback.
	cancelTimeout = 10 * time.Second
)

// operationCancellationContextKey stores per-operation cancellation state on
// an operation context without colliding with caller-provided values.
type operationCancellationContextKey struct{}

// operationCancellationResult carries the timeout-bounded context created by
// the cancellation callback and the matching cancel function.
type operationCancellationResult struct {
	context.Context
	context.CancelFunc
	err error
}

// operationCancellationState coordinates one operation's cancellation
// callback with the exchange loop that authorizes break/reset.
type operationCancellationState struct {
	startOnce sync.Once
	start     chan bool
	completed chan operationCancellationResult
	done      chan struct{}
}

// newOperationCancellationState creates the channels used to coordinate one
// operation's cancellation callback.
func newOperationCancellationState() *operationCancellationState {
	return &operationCancellationState{
		start:     make(chan bool, 1),
		completed: make(chan operationCancellationResult, 1),
		done:      make(chan struct{}),
	}
}

// requestBreakReset allows the cancellation callback to run break/reset and
// waits for its timeout context. It returns false if cleanup already released
// the callback.
func (s *operationCancellationState) requestBreakReset() (operationCancellationResult, bool) {
	requested := false
	s.startOnce.Do(func() {
		requested = true
		s.start <- true
	})
	if !requested {
		return operationCancellationResult{}, false
	}
	return <-s.completed, true
}

// abortBreakReset releases a fired cancellation callback without running
// break/reset.
func (s *operationCancellationState) abortBreakReset() {
	s.startOnce.Do(func() {
		s.start <- false
	})
}

// operationCancellation coordinates context cancellation and TTC stream
// recovery for a physical session. It never acquires sessionSynchronizer,
// because a break/reset must interrupt the exchange that owns the stream.
type operationCancellation struct{}

// newOperationCancellation creates cancellation support for one physical TTC
// session.
func newOperationCancellation() *operationCancellation {
	return &operationCancellation{}
}

// newCancelableOperationContext arms one exchange for context-driven
// break/reset. The caller must already own sessionSynchronizer and must call
// cleanup before releasing it.
func (cancellation *operationCancellation) newCancelableOperationContext(parent context.Context, cancelExecution StmtCancellationFunction) (context.Context, context.CancelFunc, func()) {
	cancellationState := newOperationCancellationState()
	operationContext := context.WithValue(parent, operationCancellationContextKey{}, cancellationState)
	operationContext, cancelOperation := context.WithCancel(operationContext)

	stop := context.AfterFunc(operationContext, func() {
		defer close(cancellationState.done)
		cancelContext, cancel := context.WithTimeout(context.Background(), cancelTimeout)
		if start := <-cancellationState.start; !start {
			cancel()
			return
		}
		err := cancelExecution(cancelContext)
		if err != nil {
			common.Odl.Error("TTC operation break/reset failed", "error", err)
		}
		cancellationState.completed <- operationCancellationResult{
			Context:    cancelContext,
			CancelFunc: cancel,
			err:        err,
		}
	})
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			if !stop() {
				cancellationState.abortBreakReset()
				<-cancellationState.done
			}
			cancelOperation()
		})
	}
	return operationContext, cancelOperation, cleanup
}

// restoreTTCStream consumes the terminal TTIOER after an exchange observes
// context cancellation. A successful return proves the TTC stream is
// synchronized.
func (cancellation *operationCancellation) restoreTTCStream(ctx context.Context, streamer MessageStreamerInterface) (driverCommon.Message[driverCommon.MessageType], error) {
	return restoreTTCStreamAfterCancellation(ctx, streamer)
}

// restoreTTCStreamAfterCancellation completes break/reset recovery by consuming
// the terminal TTIOER. It is shared by statement and LOB executors.
func restoreTTCStreamAfterCancellation(ctx context.Context, streamer MessageStreamerInterface) (driverCommon.Message[driverCommon.MessageType], error) {
	cancellationState, ok := ctx.Value(operationCancellationContextKey{}).(*operationCancellationState)
	if !ok || cancellationState == nil {
		return nil, ctx.Err()
	}
	result, started := cancellationState.requestBreakReset()
	if !started {
		return nil, ctx.Err()
	}
	defer result.CancelFunc()
	if result.err != nil {
		return nil, result.err
	}
	message, err := streamer.Pull(result.Context, TTIOER)
	if err != nil {
		return nil, err
	}
	if message == nil || message.GetMsgCode() != TTIOER {
		return nil, common.NewOracleError(oracleErrors.ProtocolViolation, nil)
	}
	return message, nil
}
