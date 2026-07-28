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
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

// Dummy message structs for testing
// testDummyMessage is a dummy implementation of the Message interface
// used for testing message registration and retrieval in factory tests.
type testDummyMessage struct {
	greetings string
}

func (p *testDummyMessage) GetMsgCode() common.MessageType {
	return TTIPRO
}

var dummy1 = testDummyMessage{greetings: "newTestDummyMessage1"}
var dummy2 = testDummyMessage{greetings: "newTestDummyMessage2"}
var dummy3 = testDummyMessage{greetings: "newTestDummyMessage3"}

// testFuncMessage is a dummy implementation of the Message interface
// used for testing function-based message creation in factory tests.
type testFuncMessage struct {
	name string
}

func (tf *testFuncMessage) GetMsgCode() common.MessageType {
	return TTIPRO // Or a default value
}

var testFuncMsg1 = &testFuncMessage{name: "testFuncMsg1"}
var testFuncMsg2 = &testFuncMessage{name: "testFuncMsg2"}

func newTestFunction1() common.Message[common.MessageType] {
	return testFuncMsg1
}

func newTestFunction2() common.Message[common.MessageType] {
	return testFuncMsg2
}

func newTestDummyMessage1() common.Message[common.MessageType] {
	return &dummy1
}

func newTestDummyMessage2() common.Message[common.MessageType] {
	return &dummy2
}

func newTestDummyMessage3() common.Message[common.MessageType] {
	return &dummy3
}

// resetRegistries resets all global registries to their default state.
// It should be called before each test to ensure test isolation.
//func resetRegistries() {
//	// Reset the default registries
//	MessageRegistry = NewRegistry[common.MessageType]()
//	FunctionRegistry = NewRegistry[common.FunctionType]()
//	PiggyBackFunctionRegistry = NewRegistry[common.FunctionType]()
//	OneWayFunctionRegistry = NewRegistry[common.FunctionType]()
//	FunctionResponseRegistry = NewRegistry[common.FunctionType]()
//}

func _createTestFactory(pv int8,
	mr *Registry[common.MessageType],
	fr *Registry[functionRegistryKey]) Factory {
	var factory = &SimpleFactory{
		ttcVersion:   pv,
		msgregistry:  mr,
		funcregistry: fr,
	}
	return factory
}

// TestFactoryRegistries verifies the correct registration and retrieval of
// messages and functions in all registry types, including special behavior
// such as response function registration.
func TestFactoryRegistries(t *testing.T) {
	t.Parallel()
	type regCase struct {
		name    string
		setup   func() interface{}
		key     interface{}
		minVer  int8
		factory MessageCreationFunc
		extra   func(reg interface{}, t *testing.T)
	}
	cases := []regCase{
		{
			name: "MessageRegistry valid meddageType",
			setup: func() interface{} {
				return NewRegistry[common.MessageType]()
			},
			key:     TTIPRO,
			minVer:  1,
			factory: newTestDummyMessage1,
		},
		{
			name: "FunctionRegistry",
			setup: func() interface{} {
				return NewRegistry[functionRegistryKey]()
			},
			key:     functionRegistryKey{messageType: common.MessageType(1), functionType: common.FunctionType(123)},
			minVer:  1,
			factory: MessageCreationFunc(newTestFunction1),
		},
		{
			name: "PiggyBackFunctionRegistry",
			setup: func() interface{} {
				return NewRegistry[functionRegistryKey]()
			},
			key:     functionRegistryKey{messageType: common.MessageType(1), functionType: common.FunctionType(201)},
			minVer:  1,
			factory: newTestFunction1,
		},
		{
			name: "OneWayFunctionRegistry",
			setup: func() interface{} {
				return NewRegistry[functionRegistryKey]()
			},
			key:     functionRegistryKey{messageType: common.MessageType(1), functionType: common.FunctionType(202)},
			minVer:  1,
			factory: newTestFunction2,
		},
		{
			name: "FunctionResponseRegistry",
			setup: func() interface{} {
				return NewRegistry[functionRegistryKey]()
			},
			key:     functionRegistryKey{messageType: common.MessageType(1), functionType: common.FunctionType(21)},
			minVer:  1,
			factory: newTestFunction1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch reg := tc.setup().(type) {
			case *Registry[common.MessageType]:
				err := reg.Register(tc.key.(common.MessageType), tc.minVer, tc.factory)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				found := false
				for _, item := range reg.getCandidates(tc.key.(common.MessageType)) {
					if item.minTTCProtocolVersion == tc.minVer {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected candidate to be registered for key %v", tc.key)
				}
			case *Registry[functionRegistryKey]:
				err := reg.Register(tc.key.(functionRegistryKey), tc.minVer, tc.factory)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				found := false
				for _, item := range reg.getCandidates(tc.key.(functionRegistryKey)) {
					if item.minTTCProtocolVersion == tc.minVer {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected candidate to be registered for key %v", tc.key)
				}
				// Additional FunctionResponseRegistry assertion
				if tc.extra != nil {
					tc.extra(reg, t)
				}
			default:
				t.Error("unhandled registry type")
			}
		})
	}
}

func TestReplaceMessage(t *testing.T) {
	t.Parallel()
	reg := NewRegistry[common.MessageType]()
	err := reg.Register(TTIPRO, 1, newTestDummyMessage1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	err = reg.Register(TTIPRO, 1, newTestDummyMessage2)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	candidates := reg.getCandidates(TTIPRO)
	if len(candidates) != 1 {
		t.Fatalf("there should be only on message, message should have been replaced")
	}
	// call message creating function and check that correct message is created
	msg := candidates[0].makeFunc()
	testMsg, ok := msg.(*testDummyMessage)
	if !ok {
		t.Fatalf("Wrong message type, should implement testDummyMessage")
	}
	if testMsg.greetings != newTestDummyMessage2().(*testDummyMessage).greetings {
		t.Errorf("Wrong greetings, should be %s, but was %s", newTestDummyMessage2().(*testDummyMessage).greetings, testMsg.greetings)
	}
}

// TestFactoryGetMessage tests message creation via the MessageRegistry
// for different message types, protocol versions, and unregistered scenarios.
func TestFactoryGetMessage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		protocol  int8
		msgType   common.MessageType
		want      common.Message[common.MessageType]
		expectErr bool
	}
	cases := []testCase{
		{
			name:      "valid message",
			protocol:  2,
			msgType:   TTIPRO,
			want:      &dummy1,
			expectErr: false,
		},
		{
			name:      "different valid message",
			protocol:  4,
			msgType:   TTIPRO,
			want:      &dummy2,
			expectErr: false,
		},
		{
			name:      "another valid message (TTIDTY)",
			protocol:  3,
			msgType:   TTIDTY,
			want:      &dummy3,
			expectErr: false,
		},
		{
			name:      "TTIDTY not available",
			protocol:  1,
			msgType:   TTIDTY,
			want:      nil,
			expectErr: true,
		},
		{
			name:      "message not registered",
			protocol:  1,
			msgType:   TTIRPA,
			want:      nil,
			expectErr: true,
		},
	}

	reg := NewRegistry[common.MessageType]()
	reg.Register(TTIPRO, 1, newTestDummyMessage1)
	reg.Register(TTIPRO, 4, newTestDummyMessage2)
	reg.Register(TTIDTY, 2, newTestDummyMessage3)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory := _createTestFactory(tc.protocol, reg, nil)
			msg, err := factory.GetMessage(tc.msgType)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if msg != nil {
					t.Fatalf("expected nil message, got %v", msg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if msg != tc.want {
					t.Errorf("expected message %v, got %v", tc.want, msg)
				}
			}
		})
	}
}

// TestFactoryGetMessageFromFunction tests retrieval of messages created
// via function type registrations for different protocols and scenarios.
func TestFactoryGetMessageFromFunction(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name      string
		msgType   common.MessageType
		funcType  common.FunctionType
		want      common.Message[common.MessageType]
		expectErr bool
		facSetup  func() Factory
	}
	cases := []testCase{
		{
			name:      "valid function",
			msgType:   1,
			funcType:  123,
			want:      testFuncMsg1,
			expectErr: false,
			facSetup: func() Factory {
				reg := NewRegistry[functionRegistryKey]()
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 1, newTestFunction1)
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 3, newTestFunction2)
				FunctionRegistry = reg
				return _createTestFactory(2, nil, reg)
			},
		},
		{
			name:      "valid function for protocol 9",
			msgType:   1,
			funcType:  123,
			want:      testFuncMsg2,
			expectErr: false,
			facSetup: func() Factory {
				reg := NewRegistry[functionRegistryKey]()
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 1, newTestFunction1)
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 4, newTestFunction2)
				return _createTestFactory(9, nil, reg)
			},
		},
		{
			name:      "another valid function for protocol 3",
			msgType:   1,
			funcType:  123,
			want:      testFuncMsg1,
			expectErr: false,
			facSetup: func() Factory {
				reg := NewRegistry[functionRegistryKey]()
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 1, newTestFunction1)
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 4, newTestFunction2)
				return _createTestFactory(3, nil, reg)
			},
		},
		{
			name:      "function not registered",
			msgType:   1,
			funcType:  3,
			want:      nil,
			expectErr: true,
			facSetup: func() Factory {
				reg := NewRegistry[functionRegistryKey]()
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 1, newTestFunction1)
				reg.Register(functionRegistryKey{messageType: 1, functionType: 123}, 4, newTestFunction2)
				return _createTestFactory(1, nil, reg)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory := tc.facSetup()
			msg, err := factory.GetMessageForFunction(tc.msgType, tc.funcType)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if msg != nil {
					t.Fatalf("expected nil message, got %v", msg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if msg != tc.want {
					t.Errorf("expected function message %v, got %v", tc.want, msg)
				}
			}
		})
	}
}
