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

import "testing"

func TestNewSessionContext(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	if session == nil {
		t.Fatal("expected session context to be created")
	}

	if session.GetSessionProperties() == nil {
		t.Fatal("expected session context to initialize session properties")
	}
}

func TestSessionContext_SetTimeZoneVersionNumber(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	session.SetTimeZoneVersionNumber(42)

	if value, ok := session.GetClientProperties().GetProperty(TimeZoneVersionNumber).(byte); ok {
		if value != 42 {
			t.Fatalf("expected timeZoneVersionNumber to be %d, got %d", 42, value)
		}
	} else {
		t.Fatal("expected timeZoneVersionNumber to be if type byte")
	}

}

func TestSessionContext_SetSessionCharacterSets(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	session.SetSessionCharacterSets(UB2(873), UB2(2000))

	if got := session.DriverCharacterSet(); got != UB2(873) {
		t.Fatalf("expected driver character set %d, got %d", 873, got)
	}

	if got := session.SessionNCharCharacterSet(); got != UB2(2000) {
		t.Fatalf("expected NCHAR character set %d, got %d", 2000, got)
	}
}

func TestSessionContext_UpdateSessionProperties(t *testing.T) {
	t.Parallel()

	session := NewSessionContext()
	existing := session.GetSessionProperties()
	existing.SetProperty("AUTH_MODE", "initial")

	updates := NewProperties[string]()
	updates.SetProperty("AUTH_MODE", "oauth")
	updates.SetProperty("EDITION", "dev")

	session.UpdateSessionProperties(updates)

	if session.GetSessionProperties() != existing {
		t.Fatal("expected session context to update the existing properties object in place")
	}

	authMode, ok := session.GetSessionProperties().GetProperty("AUTH_MODE").(string)
	if !ok {
		t.Fatal("expected AUTH_MODE property to be present as a string")
	}
	if authMode != "oauth" {
		t.Fatalf("expected AUTH_MODE to be updated to %q, got %q", "oauth", authMode)
	}

	edition, ok := session.GetSessionProperties().GetProperty("EDITION").(string)
	if !ok {
		t.Fatal("expected EDITION property to be present as a string")
	}
	if edition != "dev" {
		t.Fatalf("expected EDITION to be %q, got %q", "dev", edition)
	}
}
