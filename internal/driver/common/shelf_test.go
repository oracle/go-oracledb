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

package common

import (
	"context"
	"errors"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func TestShelf_NewShelfTest(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	if shelf.GetLocalizationService() != nil {
		t.Fatal("shelf should not have a default localization service")
	}
}

// 1. Register a marshaller and check that the same marshaller is returned
// 2. Unregister marshaller and check that nil is returned
func TestShelf_GetMarshaller(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	marshaller := &testObject{}
	shelf.RegisterMarshaller((Marshaller)(marshaller))

	retrievedMarshaller := shelf.GetMarshaller()

	if retrievedMarshaller != marshaller {
		t.Fatal("the same marshaller should have been retrieved.")
	}

	shelf.RegisterMarshaller(nil)
	retrievedMarshaller = shelf.GetMarshaller()

	if retrievedMarshaller != nil {
		t.Fatal("marshaller on shelf should be nil")
	}

}

// 1. Register a factory and check that the same factory is returned
// 2. Unregister factory and check that nil is returned
func TestShelf_GetMessageFactory(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	factory := &testObject{}
	shelf.RegisterMessageFactory((Factory)(factory))

	retrievedFactory := shelf.GetMessageFactory()

	if retrievedFactory != factory {
		t.Fatal("the same factory should have been retrieved.")
	}

	shelf.RegisterMessageFactory(nil)
	retrievedFactory = shelf.GetMessageFactory()

	if retrievedFactory != nil {
		t.Fatal("factory on shelf should be nil")
	}

}

// 1. Register a localization service and check that the same service is returned
// 2. Unregister localization service and check that nil is returned
func TestShelf_GetLocalizationService(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	localizationService := &testLocalizationService{}
	shelf.RegisterLocalizationService(localizationService)

	retrievedLocalizationService := shelf.GetLocalizationService()

	if retrievedLocalizationService != localizationService {
		t.Fatal("the same localization service should have been retrieved.")
	}

	shelf.RegisterLocalizationService(nil)
	retrievedLocalizationService = shelf.GetLocalizationService()

	if retrievedLocalizationService != nil {
		t.Fatal("localization service on shelf should be nil")
	}
}

func TestShelf_LocalizeError(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	err := errors.New("raw error")

	if got := shelf.LocalizeError(err); got != err {
		t.Fatalf("expected error to be returned unchanged when no localization service is registered")
	}

	localizationService := &testLocalizationService{}
	shelf.RegisterLocalizationService(localizationService)

	if got := shelf.LocalizeError(err); got != err {
		t.Fatalf("expected shelf-localized error to be returned unchanged by test localization service")
	}
}

// 1. Register a streamer and check that the same streamer is returned
// 2. Unregister streamer and check that nil is returned
func TestShelf_GetStreamer(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	streamer := &testObject{}
	shelf.RegisterMessageStreamer((Streamer[int])(streamer))

	retrievedStreamer := shelf.GetMessageStreamer()

	if retrievedStreamer != streamer {
		t.Fatal("the same streamer should have been retrieved.")
	}

	shelf.RegisterMessageStreamer(nil)
	retrievedStreamer = shelf.GetMessageStreamer()

	if retrievedStreamer != nil {
		t.Fatal("streamer on shelf should be nil")
	}
}

// 1. Register capabilities and check that the same capabilities are returned
// 2. Unregister capabilities and check that nil is returned
func TestShelf_GetCapabilities(t *testing.T) {
	t.Parallel()
	shelf := NewShelf[int]()
	if shelf == nil {
		t.Fatal("shelf should not be nil")
	}

	capabilities := make(map[string]Capability, 1)
	capabilities["TEST_CAPABILITY"] = Capability{Value: 80, IsSet: true}
	shelf.RegisterCapabilities(capabilities)

	retrievedCapabilities := shelf.GetCapabilities()
	for key, value := range retrievedCapabilities {
		if value != capabilities[key] {
			t.Fatal("the same capabilities should have been retrieved.")
		}
	}

	shelf.RegisterCapabilities(nil)
	retrievedCapabilities = shelf.GetCapabilities()

	if retrievedCapabilities != nil {
		t.Fatal("capabilities on shelf should be nil")
	}
}

// testObject implements all of the needed interfaces for this test
type testObject struct{}

func (t *testObject) MarshalUB1(context.Context, UB1) error {
	return nil
}
func (t *testObject) MarshalUB2(context.Context, UB2) error {
	return nil
}
func (t *testObject) MarshalUB4(context.Context, UB4) error {
	return nil
}
func (t *testObject) MarshalUB8(context.Context, UB8) error {
	return nil
}
func (t *testObject) MarshalSB4(context.Context, SB4) error {
	return nil
}
func (t *testObject) MarshalB1Array(context.Context, B1Array) error {
	return nil
}
func (t *testObject) MarshalPTR(context.Context) error {
	return nil
}
func (t *testObject) MarshalNullPTR(context.Context) error {
	return nil
}
func (t *testObject) MarshalChar(context.Context, B1Array) error {
	return nil
}
func (t *testObject) MarshalCLR(context.Context, B1Array, int, int) error {
	return nil
}
func (t *testObject) UnmarshalUB1(context.Context) (UB1, error) {
	return 0, nil
}
func (t *testObject) UnmarshalUB2(context.Context) (UB2, error) {
	return 0, nil
}
func (t *testObject) UnmarshalUB4(context.Context) (UB4, error) {
	return 0, nil
}
func (t *testObject) UnmarshalUB8(context.Context) (UB8, error) {
	return 0, nil
}
func (t *testObject) UnmarshalSB1(context.Context) (SB1, error) {
	return -1, nil
}
func (t *testObject) UnmarshalSB2(context.Context) (SB2, error) {
	return -1, nil
}
func (t *testObject) UnmarshalSB4(context.Context) (SB4, error) {
	return -1, nil
}
func (t *testObject) UnmarshalCLR(context.Context, B1Array, int) (int, error) {
	return -1, nil
}
func (t *testObject) UnmarshalCLRColumnData(context.Context) (B1Array, int, error) {
	return nil, -1, nil
}
func (t *testObject) UnmarshalB1Array(context.Context, int) (B1Array, error) {
	return nil, nil
}
func (t *testObject) UnmarshalText(context.Context, int) (B1Array, error) {
	return nil, nil
}
func (t *testObject) Flush(context.Context) error {
	return nil
}

func (t *testObject) GetMessage(msgType MessageType) (Message[MessageType], error) {
	return nil, nil
}

func (t *testObject) GetMessageForFunction(msgType MessageType, funcType FunctionType) (Message[MessageType], error) {
	return nil, nil
}

func (t *testObject) Push(context.Context, Message[int]) error {
	return nil
}

func (t *testObject) Pull(context.Context, ...int) (Message[int], error) {
	return nil, nil
}

func (t *testObject) Drain(context.Context, StreamDirection) (int, int) {
	return -1, -1
}

type testLocalizationService struct{}

func (t *testLocalizationService) Format(code oracleErrors.ErrorCode, args ...interface{}) string {
	return message.NewPrinter(language.English).Sprintf(string(code), args...)
}

func (t *testLocalizationService) LocalizeError(err error) error {
	return err
}
