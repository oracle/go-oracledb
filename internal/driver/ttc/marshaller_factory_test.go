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

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// 1. Create a factory and check that the same default marshaller is returned
// 2. Create a factory with a nil default and check that nil is returned
func TestMarshallerFactory_GetMarshaller(t *testing.T) {
	t.Parallel()
	marshaller := newMockMarshaller()
	factory := NewMarshallerFactory(marshaller)
	if factory.GetMarshallerForMessageType(defaultMarshallerMessageType) != marshaller {
		t.Fatal("the same default marshaller should have been retrieved")
	}

	factory = NewMarshallerFactory(nil)
	if factory.GetMarshallerForMessageType(defaultMarshallerMessageType) != nil {
		t.Fatal("default marshaller should be nil")
	}
	if factory.GetMarshallerForMessageType(TTIPRO) != nil {
		t.Fatal("unregistered message type should use the nil default marshaller")
	}
}

// TestMarshallerFactory_GetMarshallerForMessageType verifies that registered message types
// use their assigned marshaller and unregistered types use the default.
// Registering a message-specific marshaller must leave the default unchanged.
func TestMarshallerFactory_GetMarshallerForMessageType(t *testing.T) {
	t.Parallel()
	marshaller := newMockMarshaller()
	defaultMarshaller := newMockMarshaller()
	factory := NewMarshallerFactory(defaultMarshaller)
	factory.RegisterMarshaller(marshaller, TTIPRO, TTIDTY)

	for _, messageType := range []common.MessageType{TTIPRO, TTIDTY} {
		if factory.GetMarshallerForMessageType(messageType) != marshaller {
			t.Fatalf("the assigned marshaller should have been retrieved for %v", messageType)
		}
	}
	if factory.GetMarshallerForMessageType(TTIRPA) != defaultMarshaller {
		t.Fatal("unregistered message type should use the default marshaller")
	}
	if factory.GetMarshallerForMessageType(defaultMarshallerMessageType) != defaultMarshaller {
		t.Fatal("registering a message override changed the default marshaller")
	}
}
