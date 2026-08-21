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
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func newMockMarshaller() common.Marshaller {
	return NewNativeMarshalEngine(NewArrayDataBuffer(2048), common.BIG_ENDIAN)
}

type errorMarshaller struct{}

func (e *errorMarshaller) UnmarshalUB1() (common.UB1, error) {
	return 0, errors.New("mock error")
}

func (e *errorMarshaller) Flush() error {
	return nil
}

type mockMessage struct {
	msgType                common.MessageType
	funcType               common.FunctionType
	failureOnMarshalling   error
	failureOnUnMarshalling error
}

func NewMockTTIpro() common.Message[common.MessageType] {
	return NewMockMessage(TTIPRO)
}
func NewMockTTIdty() common.Message[common.MessageType] {
	return NewMockMessage(TTIDTY)
}
func NewMockTTIoer() common.Message[common.MessageType] {
	return NewMockMessage(TTIOER)
}
func NewMockTTIRpa() common.Message[common.MessageType] {
	return NewMockMessage(TTIRPA)
}
func NewMockRpa() common.Message[common.MessageType] {
	return NewMockMessage(TTIRPA)
}
func NewMockMessage(t common.MessageType) common.Message[common.MessageType] {
	return &mockMessage{msgType: t}
}
func (m *mockMessage) setMarshallFailure(err error) {
	m.failureOnMarshalling = err
}
func (m *mockMessage) setUnMarshallFailure(err error) {
	m.failureOnUnMarshalling = err
}
func (m *mockMessage) GetMsgCode() common.MessageType {
	return m.msgType
}

var (
	messageSPFOauth = &mockMessage{
		msgType:  TTISPF,
		funcType: oauth,
	}
	messageSPFOSesskey = &mockMessage{
		msgType:  TTISPF,
		funcType: oSesskey,
	}
)

func NewMockSPFOauth() common.Message[common.MessageType] {
	return messageSPFOauth
}

func NewMockSPFOSesskey() common.Message[common.MessageType] {
	return messageSPFOSesskey
}

type mockMarshalOnlyMessage struct {
	msgType  common.MessageType
	funcType common.FunctionType
}

func (m *mockMarshalOnlyMessage) GetMsgCode() common.MessageType {
	return m.msgType
}

func (m *mockMarshalOnlyMessage) MarshalTo(ctx context.Context, engine common.Marshaller) error {
	return engine.MarshalUB1(ctx, common.UB1(m.funcType))
}

// MarshalTo marshalls the mock msg
func (m *mockMessage) MarshalTo(ctx context.Context, engine common.Marshaller) error {
	if m.failureOnMarshalling != nil {
		return m.failureOnMarshalling
	}
	engine.MarshalUB2(ctx, 11)
	engine.MarshalUB2(ctx, 12)
	engine.MarshalUB2(ctx, 13)

	return nil
}

// unMarshalTo unmarshalls the mock msg
func (m *mockMessage) UnMarshalFrom(ctx context.Context, engine common.Marshaller) error {

	if m.failureOnUnMarshalling != nil {
		return m.failureOnUnMarshalling
	}

	var ub2 common.UB2
	var err error

	ub2, err = engine.UnmarshalUB2(ctx)
	if err != nil {
		return fmt.Errorf("error while unmarshalling UB2 [%s]", err)
	}
	if ub2 != 11 {
		return fmt.Errorf("wrong unmashalled value [%d] should be 11", ub2)
	}

	ub2, err = engine.UnmarshalUB2(ctx)
	if err != nil {
		return fmt.Errorf("error while unmarshalling UB2 [%s]", err)
	}
	if ub2 != 12 {
		return fmt.Errorf("wrong unmashalled value [%d] should be 11", ub2)
	}

	ub2, err = engine.UnmarshalUB2(ctx)
	if err != nil {
		return fmt.Errorf("error while unmarshalling UB2 [%s]", err)
	}
	if ub2 != 13 {
		return fmt.Errorf("wrong unmashalled value [%d] should be 11", ub2)
	}

	return nil

}

// Simple push test
// 1 - push a mock message
// 2 - flush the streamer
// expectations:
//   - data has been actually written to the underlying buffer, and we can unmarshal back the msg
func TestMessageStreamer_Flush(t *testing.T) {
	t.Parallel()

	var fakeBuffer = NewArrayDataBuffer(2048)
	marshaller := NewNativeMarshalEngine(fakeBuffer,
		common.LITTLE_ENDIAN)

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(marshaller)
	streamer := NewMessageStreamer(shelf)

	msg := &mockMessage{msgType: TTIPRO}
	err := streamer.Push(context.Background(), msg)
	if err != nil {
		t.Errorf("Push returned error: %v", err)
	}

	streamer.Flush(context.Background())

	mh := &messageHeader{}
	mh.UnMarshalFrom(context.Background(), marshaller)
	if mh.GetType() != TTIPRO {
		t.Errorf("unmarshal after flush wrong msdg type [%v] should be [%v]", mh.GetType(), TTIPRO)
	}
	err = msg.UnMarshalFrom(context.Background(), marshaller)
	if err != nil {
		t.Errorf("unexpected error raised [%v]", err)
	}
}

// Simple push null test
// 1 - push a null message
// expectations:
//   - receive an error
func TestMessageStreamer_NullPull(t *testing.T) {
	t.Parallel()
	streamer := NewMessageStreamer(_prepareShelf())
	err := streamer.Push(nil, nil)
	if err == nil {
		t.Errorf("pushing nil message did not raise an error")
	}
}

// Simple push test
// 1 - push a TTIPRO message
// 2 - push a TTIDTY message
// 3 - push a TTIRPA message
// 4 - pull 4 times
// expectations:
//   - receive messages matching the types of the pushed ones
//   - got OEF the fourth time we pull.
func TestMessageStreamer_SimpleTypedPull(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Push(context.Background(), NewMockTTIdty())
	streamer.Push(context.Background(), NewMockRpa())

	streamer.Flush(context.Background())

	msg, err := streamer.Pull(context.Background(), TTIRPA, TTIPRO, TTIDTY)
	if err != nil {
		t.Fatalf("unexpected error raised while pulling [%v]", err)
	}
	t.Logf("first message to come out [%v]", toString(msg.GetMsgCode()))
	if msg.GetMsgCode() != TTIPRO &&
		msg.GetMsgCode() != TTIDTY &&
		msg.GetMsgCode() != TTIRPA {
		t.Fatalf("unexpected error raised while pulling [%v]", err)
	}

	msg, err = streamer.Pull(context.Background(), TTIRPA, TTIPRO, TTIDTY)
	if err != nil {
		t.Fatalf("unexpected error raised while pulling [%v]", err)
	}
	t.Logf("second message to come out [%v]", toString(msg.GetMsgCode()))
	if msg.GetMsgCode() != TTIPRO &&
		msg.GetMsgCode() != TTIDTY &&
		msg.GetMsgCode() != TTIRPA {
		t.Fatalf("unexpected error raised while pulling [%v]", err)
	}

	msg, err = streamer.Pull(context.Background(), TTIRPA, TTIPRO, TTIDTY)
	if err != nil {
		t.Fatalf("unexpected error raised while pulling [%v]", err)
	}
	t.Logf("third message to come out [%v]", toString(msg.GetMsgCode()))
	if msg.GetMsgCode() != TTIPRO &&
		msg.GetMsgCode() != TTIDTY &&
		msg.GetMsgCode() != TTIRPA {
		t.Fatalf("unexpected error raised while pulling [%v]", err)
	}

	// now we should face OEF
	msg, err = streamer.Pull(context.Background(), TTIRPA, TTIPRO, TTIDTY)
	if err == nil {
		t.Fatalf("expected error not received")
	}

}

// pre-unmarshal-callback registration test
// 1 - register pre-unmarshal callback for TTIPRO
// 2 - push two TTIPRO messages
// 3 - push one TTIDTY message
// 4 - flush the streamer
// 5 - Pull a TTIPRO message
// 6 - Pull a TTIDTY message
// 7 - unregister the callback for TTIPRO
// 8 - Pull a message
// expectations:
//   - callback is called at #5 and receive expected type as parameter
//   - callback is not called at #6
//   - callback is not called at #8
func TestMessageStreamer_CallbackRegisterPreUnmarshal(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	isCBCalled := false
	isExpectedType := false
	cb := func(t *messageHeader) (message common.Message[common.MessageType], e error) {
		isCBCalled = true
		isExpectedType = (t.messageType == TTIPRO)
		return nil, nil
	}

	streamer.RegisterPreUnmarshallCallback(TTIPRO, cb)

	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Push(context.Background(), NewMockTTIdty())
	streamer.Flush(context.Background())

	streamer.Pull(context.Background(), TTIPRO)
	if !isCBCalled {
		t.Errorf("callabck did not get called")
	}
	if !isExpectedType {
		t.Errorf("callabck did not receive expected message type")
	}

	isCBCalled = false
	streamer.Pull(context.Background(), TTIDTY)
	if isCBCalled {
		t.Errorf("callabck should have get called for not register type")
	}

	isCBCalled = false
	streamer.UnRegisterPreUnmarshallCallback(TTIPRO)
	streamer.Pull(context.Background(), TTIPRO)
	if isCBCalled {
		t.Errorf("callabck should have get called for unregister type")
	}
}

// post-unmarshal-callback registration test
// 1 - register pre-unmarshal callback for TTIPRO
// 2 - push two TTIPRO messages
// 3 - push one TTIDTY message
// 4 - flush the streamer
// 5 - Pull a TTIPRO message
// 6 - Pull a TTIDTY message
// 7 - unregister the callback for TTIPRO
// 8 - Pull a message
// expectations:
//   - callback is called at #5 and receive expected type as parameter
//   - callback is not called at #6
//   - callback is not called at #8
func TestMessageStreamer_CallbackRegisterPostUnmarshal(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	pushedMsg := NewMockTTIpro()

	isCBCalled := false
	isExpectedMsg := false
	isExpectedErr := false
	cb := func(message common.Message[common.MessageType], err error) (b bool, e error) {
		isCBCalled = true
		isExpectedMsg = reflect.DeepEqual(message, pushedMsg)
		isExpectedErr = (err == nil)
		return true, nil
	}

	streamer.RegisterPostUnmarshallCallback(TTIPRO, cb)

	streamer.Push(context.Background(), pushedMsg)
	streamer.Push(context.Background(), NewMockTTIdty())
	streamer.Flush(context.Background())

	streamer.Pull(context.Background(), TTIPRO)
	if !isCBCalled {
		t.Errorf("callabck did not get called")
	}
	if !isExpectedMsg {
		t.Errorf("callback did not receive expected message")
	}
	if !isExpectedErr {
		t.Errorf("callback did not receive expected error")
	}

	isCBCalled = false
	streamer.Pull(context.Background(), TTIDTY)
	if isCBCalled {
		t.Errorf("callabck should have get called for not register type")
	}

	isCBCalled = false
	streamer.UnRegisterPreUnmarshallCallback(TTIPRO)
	streamer.Pull(context.Background(), TTIPRO)
	if isCBCalled {
		t.Errorf("callabck should have get called for unregister type")
	}
}

// Callback un-registration test
// 1 - create a streamer
// 2 - push  TTIPRO messages
// 3 - flush the streamer
// 4 - register pre-unmarshal callback for TTIPRO
// 5 - register post-unmarshal callback for TTIPRO
// 6 - umregister pre-unmarshal callback for TTIPRO
// 7 - unregister post-unmarshal callback for TTIPRO
// 8 - Pull a TTIPRO message
// expectations:
//   - none of the postUCallbacks have been called
func TestMessageStreamer_CallbackUnregister(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	isPreCBCalled := false
	isPostCBCalled := false
	cbpre := func(*messageHeader) (common.Message[common.MessageType], error) {
		isPreCBCalled = true
		return nil, nil
	}
	cbpost := func(common.Message[common.MessageType], error) (bool, error) {
		isPostCBCalled = true
		return true, nil
	}
	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Flush(context.Background())

	// RegisterMessage postUCallbacks
	streamer.RegisterPreUnmarshallCallback(TTIPRO, cbpre)
	streamer.RegisterPostUnmarshallCallback(TTIPRO, cbpost)

	// Unregister one
	streamer.UnRegisterPreUnmarshallCallback(TTIPRO)
	// Unregister the last one
	streamer.UnRegisterPostUnmarshallCallback(TTIPRO)

	streamer.Pull(context.Background(), TTIPRO)

	if isPreCBCalled == true || isPostCBCalled == true {
		t.Errorf("Unregistered callback should not have been called")
	}
}

// prepares a testing shelf
func _prepareShelf() *ttiShelf[common.MessageType] {

	marshaller := newMockMarshaller()

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(marshaller)
	var messageRegistry = NewRegistry[common.MessageType]()
	var functionRegistry = NewRegistry[functionRegistryKey]()
	messageRegistry.Register(TTIPRO, -1, NewMockTTIpro)
	messageRegistry.Register(TTIDTY, -1, NewMockTTIdty)
	messageRegistry.Register(TTIOER, -1, NewMockTTIoer)
	messageRegistry.Register(TTIRPA, -1, NewMockTTIRpa)
	functionRegistry.Register(functionRegistryKey{messageType: TTISPF, functionType: oauth}, -1, NewMockSPFOauth)
	functionRegistry.Register(functionRegistryKey{messageType: TTISPF, functionType: oSesskey}, -1, NewMockSPFOSesskey)
	f := &SimpleFactory{
		ttcVersion:   -1,
		msgregistry:  messageRegistry,
		funcregistry: functionRegistry,
	}
	shelf.RegisterMessageFactory(f)
	return shelf
}

// Drain test
// 1 - create a data buffer marshaller , shelf etc..
// 2 - push  TTIDTY messages
// 3 - push  TTIPRO messages
// 3 - flush the streamer
// 4 - pull a TTIPRO message
// 5 - push a tTIdty message
// 6 - drain the streamer
// 7 - flush the streamer again
// 8 - pull a TTIPRO message
// expectations:
//   - data buffer being populated at step #3
//   - DTY place in incoming queue at step #4
//   - outgoing populated with DTY at step #5
//   - no data written after flush at step #7
//   - nothing being pulled at step #8
func TestMessageStreamer_Drain(t *testing.T) {
	t.Parallel()

	aData := NewArrayDataBuffer(2048)

	marshaller := NewNativeMarshalEngine(aData, common.BIG_ENDIAN)

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(marshaller)
	var messageRegistry = NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIPRO, -1, NewMockTTIpro)
	messageRegistry.Register(TTIDTY, -1, NewMockTTIdty)
	shelf.RegisterMessageFactory(&SimpleFactory{
		ttcVersion:   -1,
		msgregistry:  messageRegistry,
		funcregistry: nil,
	})
	streamer := NewMessageStreamer(shelf)

	// Add some messages
	streamer.Push(context.Background(), NewMockTTIdty())
	streamer.Push(context.Background(), NewMockTTIpro())
	// Fush so they go to the data buffer
	streamer.Flush(context.Background())
	// pull so it will place dty in incoming list
	streamer.Pull(context.Background(), TTIPRO)
	// push so it will populate outgoing
	streamer.Push(context.Background(), NewMockTTIdty())

	streamer.Drain(context.Background(), common.INOUT)

	// both queues should be empty now
	// data buffer should remain empty after that
	streamer.Flush(context.Background())
	if aData.isDataTobeFlushed() {
		t.Errorf("all data from buffer not drained")
	}

	// data buffer should remain empty after that
	_, err := streamer.Pull(context.Background(), TTIPRO)
	if err == nil {
		t.Errorf("no message should be available")
	}
}

// simple push test
// 1 - create streamer
// 2 - push  TTIPRO messages
// 3 - flush the streamer
// 4 - pull a TTIPRO message
// 5 - get code of pulled message
// expectations:
//   - tTIpro message correctly pulled out of the streamer
func TestMessageStreamer_SimplePush(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	myMockedTTIpro := NewMockTTIpro()
	streamer.Push(context.Background(), myMockedTTIpro)

	streamer.Flush(context.Background())

	// at this point, we have one tTIpro message available on our faked wired
	msg, err := streamer.Pull(context.Background(), TTIPRO)
	if err != nil {
		t.Fatalf("Second Pull raised : %v", err)
	}
	if msg.GetMsgCode() != TTIPRO {
		t.Fatalf("Pull did not returned the expected message type")
	}
}

// simple pull from cache test
// 1 - create streamer
// 2 - push TTIPRO messages
// 3 - push TTIDTY messages
// 4 - flush the streamer
// 5 - pull a TTIDTY message
// 6 - pull a TTIPRO message
// expectations:
//   - all calls succeeded
func TestMessageStreamer_PullFromIncomings(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())
	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Push(context.Background(), NewMockTTIdty())

	err := streamer.Flush(context.Background())
	if err != nil {
		t.Errorf("simple flush should have succeed")
	}
	_, err = streamer.Pull(context.Background(), TTIDTY)
	if err != nil {
		t.Errorf("simple Pull should have succeed")
	}
	_, err = streamer.Pull(context.Background(), TTIPRO)
	if err != nil {
		t.Errorf("simple pull from cache should have succeed")
	}
}

// ordered pull test
// 1 - register pre-unmarshal callback for TTIPRO that allocate the object
// 2 - push two TTIPRO messages
// 3 - flush the streamer
// 4 - Pull twice TTIPRO message
// expectations:
//   - messages should be pulled out in reverse order. the streamer should not change ordering while sending
func TestMessageStreamer_OrderedPush(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Push(context.Background(), NewMockTTIdty())

	streamer.Flush(context.Background())

	// at this point, we have to tTIpro messages available on our faked wired

	msg, err := streamer.Pull(context.Background(), TTIPRO, TTIDTY, TTIRPA)
	if err != nil {
		t.Fatalf("First Pull raised an unexpected error : %v", err)
	}

	if msg.GetMsgCode() != TTIPRO {
		t.Fatalf("wrong message pulled out of the streamer expected message of type [%s] got [%s]",
			toString(TTIPRO), toString(msg.GetMsgCode()))
	}

	msg, err2 := streamer.Pull(context.Background(), TTIPRO, TTIDTY, TTIRPA)
	if err2 != nil {
		t.Fatalf("Second Pull raised an unexpected error : %v", err2)
	}

	if msg.GetMsgCode() != TTIDTY {
		t.Fatalf("wrong message pulled out of the streamer expected message of type [%s] got [%s]",
			toString(TTIDTY), toString(msg.GetMsgCode()))
	}

}

// timeout test
// 1 - register blocking databuffer
// 2 - pull mock message with timeout context
// expectations:
//   - should go out on timeout error
func TestMessageStreamer_PullWithTimeout(t *testing.T) {
	t.Parallel()
	t.Skipf("IMPLEMENT ME !")
}

// timeout test with incoming
// 1 - register blocking databuffer
// 2 - push mock TTIPRO message
// 3 - pull mock TTIDTY message with timeout context
// 4 - pull TTIPRO
// expectations:
//   - should go out on timeout error on step #3
//   - should pull out successfully on step #4
func TestMessageStreamer_PullWithTimeoutAndIncomings(t *testing.T) {
	t.Parallel()
	t.Skipf("IMPLEMENT ME !")
}

// pre-unmarshal callback with allocation  test
// 1 - register pre-unmarshal callback for TTIPRO that allocate the object
// 2 - push two TTIPRO messages
// 3 - flush the streamer
// 4 - Pull a TTIPRO message
// expectations:
//   - callback is called at #4
//   - object pulled out of the streamer is the one from the callback
func TestMessageStreamer_CallbackPullWithPreUnmarshallAlloc(t *testing.T) {
	t.Parallel()

	streamer := NewMessageStreamer(_prepareShelf())

	myMockedTTIpro := NewMockTTIpro()

	var cbCalled bool
	streamer.RegisterPreUnmarshallCallback(TTIPRO, func(*messageHeader) (common.Message[common.MessageType], error) {
		cbCalled = true
		return myMockedTTIpro, nil
	})

	streamer.Push(context.Background(), NewMockTTIpro())

	streamer.Flush(context.Background())

	msg, err := streamer.Pull(context.Background(), TTIPRO)
	if err != nil {
		t.Fatalf("Pull should not have raised an error [%v]", err)
	}

	if cbCalled == false {
		t.Fatalf("register callback not called")
	}

	if !reflect.DeepEqual(msg, myMockedTTIpro) {
		t.Fatalf("Streamer did not pull out message from the callback")
	}

}

// pre-unmarshal callback with error test
// 1 - register pre-unmarshal callback for TTIPRO that return an error and keep == true
// 2 - push two TTIPRO messages
// 3 - flush the streamer
// 4 - Pull a TTIPRO message
// expectations:
//   - pull return the error from the callback at step #4
//   - pull return a non nil message at step #4
func TestMessageStreamer_CallbackPullWithPreUnmarshallError(t *testing.T) {
	t.Parallel()
	streamer := NewMessageStreamer(_prepareShelf())

	var customError = errors.New("this is a test error")
	streamer.RegisterPreUnmarshallCallback(TTIPRO, func(*messageHeader) (common.Message[common.MessageType], error) {
		return nil, customError
	})

	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Flush(context.Background())

	msg, err := streamer.Pull(context.Background(), TTIPRO)
	if err == nil {
		t.Fatalf("Pull should have raised an error")
	}
	if !reflect.DeepEqual(err, customError) {
		t.Fatalf("pull did not returned error from the callback")
	}

	if msg == nil {
		t.Fatalf("nil message returned from failed pre-unmarshal callback")
	}

}

// unmarshal error test
// 2 - push two TTIPRO messages that raise unmarhsall error
// 3 - flush the streamer
// 4 - Pull a TTIPRO message
// expectations:
//   - pull return on error from the callback is returned at step #4
//   - pull return the error from the callback at step #4
//   - pull return a  message at step #4
func TestMessageStreamer_UnmarshallError(t *testing.T) {
	t.Parallel()

	marshaller := newMockMarshaller()

	var customError = errors.New("this is a test error")
	var tMsg = &mockMessage{msgType: TTIPRO}
	tMsg.setUnMarshallFailure(customError)

	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(marshaller)
	var messageRegistry = NewRegistry[common.MessageType]()
	messageRegistry.Register(TTIPRO, -1, func() common.Message[common.MessageType] {
		return tMsg
	})
	shelf.RegisterMessageFactory(&SimpleFactory{
		ttcVersion:   -1,
		msgregistry:  messageRegistry,
		funcregistry: nil,
	})

	streamer := NewMessageStreamer(shelf)

	streamer.Push(context.Background(), tMsg)
	streamer.Flush(context.Background())

	msg, err := streamer.Pull(context.Background(), TTIPRO)
	if err == nil {
		t.Fatalf("Pull should have raised an error")
	}
	if !reflect.DeepEqual(err, customError) {
		t.Fatalf("pull did not returned error from the callback")
	}
	if !reflect.DeepEqual(msg, tMsg) {
		t.Fatalf("pull did not returned rigth message from the callback")
	}

}

// This verifies that and error is thrown when a received message does not
// implement common.UnMarshallable.
func TestMessageStreamer_PullReturnsErrorForNonUnmarshallableFunction(t *testing.T) {
	t.Parallel()
	marshaller := newMockMarshaller()
	shelf := newShelf[common.MessageType]()
	shelf.RegisterMarshaller(marshaller)

	messageRegistry := NewRegistry[common.MessageType]()
	functionRegistry := NewRegistry[functionRegistryKey]()
	marshalOnly := &mockMarshalOnlyMessage{msgType: TTIFUN, funcType: oauth}
	if err := functionRegistry.Register(
		functionRegistryKey{messageType: TTIFUN, functionType: oauth},
		-1,
		func() common.Message[common.MessageType] {
			return marshalOnly
		},
	); err != nil {
		t.Fatalf("failed to register function: %v", err)
	}

	shelf.RegisterMessageFactory(&SimpleFactory{
		ttcVersion:   -1,
		msgregistry:  messageRegistry,
		funcregistry: functionRegistry,
	})

	streamer := NewMessageStreamer(shelf)
	if err := streamer.Push(context.Background(), marshalOnly); err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if err := streamer.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	msg, err := streamer.Pull(context.Background(), TTIRPA)
	if err == nil {
		t.Fatalf("expected Pull to reject non-unmarshallable message")
	}
	if msg != nil {
		t.Fatalf("expected nil message for non-unmarshallable function, got %v", msg)
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T: %v", err, err)
	}
	if sqlErr.ErrorCode() != string(oracleErrors.StreamerReadError) {
		t.Fatalf("expected %s, got %s", oracleErrors.StreamerReadError, sqlErr.ErrorCode())
	}
}

// post-unmarshal callback with keep false test
// 1 - register post-unmarshal callback for TTIPRO that return an error and keep == false
// 2 - push two TTIPRO messages
// 3 - flush the streamer
// 4 - Pull a TTIPRO message
// expectations:
//   - pull return on EOF error as a message should have been discarded

func TestMessageStreamer_CallbackPullWithPostUnmarshallKeepFalse(t *testing.T) {
	t.Parallel()
	streamer := NewMessageStreamer(_prepareShelf())

	streamer.RegisterPostUnmarshallCallback(TTIPRO, func(message common.Message[common.MessageType], err error) (bool, error) {
		return false, errors.New("this is a test error")
	})

	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Flush(context.Background())

	_, err := streamer.Pull(context.Background(), TTIPRO)
	if err == nil {
		t.Fatalf("Pull should not have raised an error [%v]", err)
	}

}

// post-unmarshal callback with keep true test and error
// 1 - register post-unmarshal callback for TTIPRO that return an error and keep == true
// 2 - push two TTIPRO messages
// 3 - flush the streamer
// 4 - Pull a TTIPRO message
// expectations:
//   - message should be returned at step #4 with expected error

func TestMessageStreamer_CallbackPullWithPostUnmarshallKeepTrueAndError(t *testing.T) {
	t.Parallel()
	streamer := NewMessageStreamer(_prepareShelf())

	expectedError := errors.New("this is a test error")
	streamer.RegisterPostUnmarshallCallback(TTIPRO, func(message common.Message[common.MessageType], err error) (bool, error) {
		return true, expectedError
	})

	streamer.Push(context.Background(), NewMockTTIpro())
	streamer.Flush(context.Background())

	msg, err := streamer.Pull(context.Background(), TTIPRO)
	if !reflect.DeepEqual(err, expectedError) {
		t.Fatalf("Pull should have raised an error [%v]", err)
	}
	if msg == nil {
		t.Fatalf("Pull should  have returned an msg")
	}

}

func TestMessageStreamer_getMessage(t *testing.T) {
	t.Parallel()
	streamer := NewMessageStreamer(_prepareShelf())
	// message TTIPRO
	header := messageHeader{
		messageType: TTIPRO,
	}
	message, err := streamer.getMessageForHeader(&header)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if message.GetMsgCode() != TTIPRO {
		t.Fatalf("Invalid message, expected message of type TTIPRO but was %d", message.GetMsgCode())
	}

	// message TTISPF function oauth
	header = messageHeader{
		messageType:  TTISPF,
		functionType: oauth,
	}
	message, err = streamer.getMessageForHeader(&header)

	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if message.GetMsgCode() != TTISPF {
		t.Fatalf("Invalid message, expected message of type TTISPF but was %d", message.GetMsgCode())
	}
	if message != messageSPFOauth {
		t.Fatalf("Invalid message returned, should be messageFunOauth")
	}

	// message TTISPF function oSesskey
	header = messageHeader{
		messageType:  TTISPF,
		functionType: oSesskey,
	}
	message, err = streamer.getMessageForHeader(&header)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	if message.GetMsgCode() != TTISPF {
		t.Fatalf("Invalid message, expected message of type TTISPF but was %d", message.GetMsgCode())
	}
	if message != messageSPFOSesskey {
		t.Fatalf("Invalid message returned, should be messageFunOauth")
	}

}
