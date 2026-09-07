/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights and any and all patent rights owned or freely licensable by each licensor
** hereunder covering either (i) the unmodified Software as contributed to or provided
** by such licensor, or (ii) the Larger Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software, and to
** permit persons to whom the Software is furnished to do so, subject to the
** following conditions:
**
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

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestLobExecutor_ErrorClassificationAndCancellation verifies fallback error
// coding, unsupported streamer handling, and server errors found during cancel
// recovery.
func TestLobExecutor_ErrorClassificationAndCancellation(t *testing.T) {
	t.Parallel()
	plain := errors.New("plain completed error")
	completed := &completedLobResponseError{err: plain}
	if got := completed.ErrorCode(); got != string(oracleErrors.LobExecError) {
		t.Fatalf("uncoded ErrorCode = %s, want %s", got, oracleErrors.LobExecError)
	}
	if !isCompletedLobResponseError(completed) || isCompletedLobResponseError(plain) {
		t.Fatal("completed LOB response classification is incorrect")
	}

	unsupportedShelf := newShelf[common.MessageType]()
	unsupportedShelf.RegisterMessageStreamer(&lobExecutorBasicStreamer{})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	executor := newLobExecutor(unsupportedShelf.Shelf)
	transportErr := errors.New("transport failed")
	err := executor.operationError(canceled, transportErr, "pull")
	if isCompletedLobResponseError(err) || !errors.Is(err, transportErr) {
		t.Fatalf("unsupported streamer operation error = %v, want uncompleted wrapped error", err)
	}

	shelf := newShelf[common.MessageType]()
	shelf.registerCancelExecution(func(context.Context) error { return nil })
	parent, cancelParent := context.WithCancel(context.Background())
	serverErr := errors.New("server rejected operation")
	pulls := 0
	streamer := &fakeStreamer{pullHook: func(ctx context.Context, _ ...common.MessageType) (common.Message[common.MessageType], error) {
		pulls++
		if pulls == 1 {
			cancelParent()
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &mockOer{err: serverErr}, nil
	}}
	shelf.RegisterMessageStreamer(streamer)
	operationContext, _, cleanup := shelf.cancellation.newCancelableOperationContext(parent, shelf.cancelExecution)
	defer cleanup()
	err = newLobExecutor(shelf.Shelf).execute(operationContext,
		newLobDefinitionForGetLengthOperation(newLocator(common.B1Array("locator"), 1)))
	if !isCompletedLobResponseError(err) || !errors.Is(err, serverErr) {
		t.Fatalf("cancel recovery error = %v, want completed server error", err)
	}
}

// lobExecutorBasicStreamer implements only the common stream contract. It is
// intentionally not a MessageStreamerInterface, so tests can exercise the
// executor's unsupported-streamer fallback.
type lobExecutorBasicStreamer struct{}

func (*lobExecutorBasicStreamer) Push(context.Context, common.Message[common.MessageType]) error {
	return nil
}

func (*lobExecutorBasicStreamer) Pull(context.Context, ...common.MessageType) (common.Message[common.MessageType], error) {
	return nil, nil
}

func (*lobExecutorBasicStreamer) Flush(context.Context) error { return nil }

func (*lobExecutorBasicStreamer) Drain(context.Context, common.StreamDirection) (int, int) {
	return 0, 0
}

// TestLobExecutor_LocalLocatorAndValidationBranches verifies quasi-locator
// no-ops, temporary open/close state, and operation validation decisions.
func TestLobExecutor_LocalLocatorAndValidationBranches(t *testing.T) {
	t.Parallel()
	executor := newLobExecutor(newShelf[common.MessageType]().Shelf)
	ctx := context.Background()

	quasiBytes := newTestLocator(false)
	quasiBytes[kolbVersionOffset+1] = quasiLocatorVersion
	quasi := newLocator(quasiBytes, 1)
	if opened, err := executor.open(ctx, quasi, lobOpenModeReadOnly); opened || err != nil {
		t.Fatalf("quasi open = (%v, %v), want (false, nil)", opened, err)
	}
	if err := executor.close(ctx, quasi); err != nil {
		t.Fatalf("quasi close: %v", err)
	}
	if opened, err := executor.isOpen(ctx, quasi); opened || err != nil {
		t.Fatalf("quasi isOpen = (%v, %v), want (false, nil)", opened, err)
	}

	temporary := newTestLobReferenceLocator(81)
	if opened, err := executor.open(ctx, temporary, lobOpenModeReadWrite); !opened || err != nil {
		t.Fatalf("temporary open = (%v, %v), want (true, nil)", opened, err)
	}
	if _, err := executor.open(ctx, temporary, lobOpenModeReadWrite); err == nil {
		t.Fatal("opening an already-open temporary locator succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobOpen)
	}
	if err := executor.close(ctx, temporary); err != nil {
		t.Fatalf("temporary close: %v", err)
	}
	if err := executor.close(ctx, temporary); err == nil {
		t.Fatal("closing an already-closed temporary locator succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobClosed)
	}
	if opened, err := executor.isOpen(ctx, temporary); opened || err != nil {
		t.Fatalf("temporary isOpen = (%v, %v), want (false, nil)", opened, err)
	}

	if err := validateLobOperation(newLocator(newTestLocator(false), 1), kplobRead); err != nil {
		t.Fatalf("read validation: %v", err)
	}
	if err := validateLobOperation(newLocator(newTestLocator(false), 1), lobOperationCode(0xffff)); err == nil {
		t.Fatal("unsupported LOB operation validation succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
}

// TestLobExecutor_PersistentOperationFailures verifies operation-specific
// wrapping for open, close, and is-open failures and the reported open state.
func TestLobExecutor_PersistentOperationFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	loc := func() *locator { return newLocator(newTestLocator(false), 1) }
	cases := []struct {
		name string
		call func(*lobExecutor, *locator) error
	}{
		{name: "open", call: func(executor *lobExecutor, locator *locator) error {
			_, err := executor.open(ctx, locator, lobOpenModeReadWrite)
			return err
		}},
		{name: "close", call: func(executor *lobExecutor, locator *locator) error {
			return executor.close(ctx, locator)
		}},
		{name: "isOpen", call: func(executor *lobExecutor, locator *locator) error {
			_, err := executor.isOpen(ctx, locator)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setup := newClobExecutorWithStub(lobExecutorScenario{flushErr: errors.New("flush failed")})
			err := tc.call(setup.clob.lobExecutor, loc())
			requireErrorCode(t, err, oracleErrors.LobExecError)
		})
	}

	setup := newClobExecutorWithStub(lobExecutorScenario{})
	setup.stub.lobRpaAmounts = []common.UB8{1}
	opened, err := setup.clob.lobExecutor.open(ctx, loc(), lobOpenModeReadWrite)
	if err != nil || !opened {
		t.Fatalf("persistent open = (%v, %v), want (true, nil)", opened, err)
	}
}

// TestLobExecutor_ExecutionFailurePaths verifies primary request push errors,
// write-payload errors, and the terminal TTISTA response path.
func TestLobExecutor_ExecutionFailurePaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	definition := newLobDefinitionForGetLengthOperation(newLocator(newTestLocator(false), 1))
	setup := newClobExecutorWithStub(lobExecutorScenario{pushErr: errors.New("request push failed")})
	if err := setup.clob.lobExecutor.execute(ctx, definition); err == nil {
		t.Fatal("generic execute succeeded after request push failure")
	} else {
		requireErrorCode(t, err, oracleErrors.LobExecError)
	}

	setup = newClobExecutorWithStub(lobExecutorScenario{pushErr: errors.New("request push failed")})
	if err := setup.clob.lobExecutor.executeWrite(ctx,
		newLobDefinitionForWriteOperation(newLocator(newTestLocator(false), 1), 1), []byte("A")); err == nil {
		t.Fatal("write execute succeeded after request push failure")
	} else {
		requireErrorCode(t, err, oracleErrors.LobExecError)
	}

	shelf, _, _ := newLobTestShelf(8192)
	writeStub := &fakeStreamer{
		preHooks:  make(map[common.MessageType]StreamerPreUnmarshallCallback),
		postHooks: make(map[common.MessageType]StreamerPostUnmarshallCallback),
	}
	shelf.RegisterMessageStreamer(&lobExecutorWritePayloadErrorStreamer{
		fakeStreamer: writeStub,
		err:          errors.New("write payload push failed"),
	})
	if err := newLobExecutor(shelf).executeWrite(ctx,
		newLobDefinitionForWriteOperation(newLocator(newTestLocator(false), 1), 1), []byte("A")); err == nil {
		t.Fatal("write execute succeeded after payload push failure")
	} else {
		requireErrorCode(t, err, oracleErrors.LobExecError)
	}

	setup = newClobExecutorWithStub(lobExecutorScenario{events: []common.Message[common.MessageType]{newTTISTA()}})
	if err := setup.clob.lobExecutor._consumeLobResponses(ctx); err != nil {
		t.Fatalf("TTISTA termination: %v", err)
	}
}

// lobExecutorMessageFactoryError forces GetMessage to fail while delegating
// function-specific message creation to the normal factory.
type lobExecutorMessageFactoryError struct {
	base Factory
	err  error
}

func (f *lobExecutorMessageFactoryError) GetMessage(common.MessageType) (common.Message[common.MessageType], error) {
	return nil, f.err
}

func (f *lobExecutorMessageFactoryError) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	return f.base.GetMessageForFunction(msgType, funcType)
}

// TestLobExecutor_LobCallbacks verifies LOBD buffer accounting and callback
// factory failures without requiring a database response stream.
func TestLobExecutor_LobCallbacks(t *testing.T) {
	t.Parallel()
	setup := newClobExecutorWithStub(lobExecutorScenario{})
	executor := setup.clob.lobExecutor
	definition := newLobDefinitionForReadOperation(newLocator(newTestLocator(false), 1), 1)
	buffer := common.B1Array{0}
	if err := executor._registerLobdCallback(buffer, definition); err != nil {
		t.Fatalf("register LOBD callback: %v", err)
	}
	pre := setup.stub.preHooks[TTILOBD]
	post := setup.stub.postHooks[TTILOBD]

	message, err := pre(nil)
	if err != nil {
		t.Fatalf("LOBD pre callback: %v", err)
	}
	lobd := message.(*tTIlobd)
	lobd.lastBytesRead = 1
	if done, err := post(lobd, nil); !done || err != nil || definition.bytesTransferred != 1 {
		t.Fatalf("LOBD success callback = (%v, %v), bytes = %d", done, err, definition.bytesTransferred)
	}

	callbackErr := errors.New("previous LOBD error")
	if done, err := post(nil, callbackErr); done || !errors.Is(err, callbackErr) {
		t.Fatalf("LOBD previous-error callback = (%v, %v)", done, err)
	}
	definition.bytesTransferred = 2
	if _, err := pre(nil); err == nil {
		t.Fatal("LOBD callback accepted a total larger than its buffer")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	definition.bytesTransferred = 0
	oversized := newTTIlobd().(*tTIlobd)
	oversized.lastBytesRead = 2
	if done, err := post(oversized, nil); done || err == nil {
		t.Fatalf("oversized LOBD frame callback = (%v, %v), want failure", done, err)
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}

	factoryErr := errors.New("TTILOBD factory failed")
	base := setup.shelf.GetMessageFactory().(Factory)
	setup.shelf.RegisterMessageFactory(&lobExecutorMessageFactoryError{base: base, err: factoryErr})
	if _, err := pre(nil); err == nil {
		t.Fatal("LOBD callback succeeded after factory failure")
	} else {
		requireErrorCode(t, err, oracleErrors.CallbackFactoryError)
	}

	rpaSetup := newClobExecutorWithStub(lobExecutorScenario{})
	rpaExecutor := rpaSetup.clob.lobExecutor
	rpaExecutor._registerLobRPACallback(definition)
	rpaBase := rpaSetup.shelf.GetMessageFactory().(Factory)
	rpaSetup.shelf.RegisterMessageFactory(&compositeFuncFailFactory{
		base: rpaBase, failMsg: TTIRPA, failFunc: oLobOps, failErr: errors.New("TTIRPA factory failed"),
	})
	if _, err := rpaSetup.stub.preHooks[TTIRPA](nil); err == nil {
		t.Fatal("RPA callback succeeded after factory failure")
	} else {
		requireErrorCode(t, err, oracleErrors.CallbackFactoryError)
	}
}

// lobExecutorWritePayloadErrorStreamer fails only when the outgoing TTILOBD
// write payload is pushed, allowing the primary request to succeed first.
type lobExecutorWritePayloadErrorStreamer struct {
	*fakeStreamer
	err error
}

func (s *lobExecutorWritePayloadErrorStreamer) Push(ctx context.Context, msg common.Message[common.MessageType]) error {
	if msg.GetMsgCode() == TTILOBD {
		return s.err
	}
	return s.fakeStreamer.Push(ctx, msg)
}
