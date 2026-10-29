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
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// newTestLobReferenceLocator returns an initialized temporary locator with a
// deterministic ten-byte LOB ID.
func newTestLobReferenceLocator(seed byte) *locator {
	data := make(common.B1Array, kolbLobIDOffset+kolbLobIDLength)
	data[koll2FlagOffset] = kolblInitializedFlag
	data[koll4FlagOffset] = kolblTemporaryFlagByte
	for index := 0; index < kolbLobIDLength; index++ {
		data[kolbLobIDOffset+index] = seed + byte(index)
	}
	return newLocator(data, 1)
}

// TestLobLocator_LOBReferenceIDIgnoresMutableFlags verifies mutable flags
// do not change the locator's stable LOB identity.
func TestLobLocator_LOBReferenceIDIgnoresMutableFlags(t *testing.T) {
	t.Parallel()
	first := newTestLobReferenceLocator(31)
	second := newLocator(append(common.B1Array(nil), first.locatorBytes...), 1)
	second.locatorBytes[koll4FlagOffset] |= kolblOpenFlagByte | kolblReadWriteFlagByte
	firstID, err := first.LOBReferenceID()
	if err != nil {
		t.Fatalf("first identity: %v", err)
	}
	secondID, err := second.LOBReferenceID()
	if err != nil {
		t.Fatalf("second identity: %v", err)
	}
	if firstID != secondID {
		t.Fatalf("mutable flags changed identity: %q != %q", firstID, secondID)
	}
}

// TestLobLocator_LOBReferenceIDRejectsIneligibleLocators verifies incomplete,
// value-based, uninitialized, and persistent locators have no registry identity.
func TestLobLocator_LOBReferenceIDRejectsIneligibleLocators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		locator func() *locator
	}{
		{
			name: "incomplete LOB ID",
			locator: func() *locator {
				data := make(common.B1Array, kolbLobIDOffset+kolbLobIDLength-1)
				data[koll2FlagOffset] = kolblInitializedFlag
				data[koll4FlagOffset] = kolblTemporaryFlagByte
				return newLocator(data, 1)
			},
		},
		{
			name: "quasi locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(131)
				loc.locatorBytes[kolbVersionOffset+1] = quasiLocatorVersion
				return loc
			},
		},
		{
			name: "value based locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(132)
				loc.locatorBytes[koll1FlagOffset] |= kolblValueBasedLocatorFlag
				return loc
			},
		},
		{
			name: "uninitialized locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(133)
				loc.locatorBytes[koll2FlagOffset] &^= kolblInitializedFlag
				return loc
			},
		},
		{
			name: "persistent locator",
			locator: func() *locator {
				loc := newTestLobReferenceLocator(134)
				loc.locatorBytes[koll4FlagOffset] &^= kolblTemporaryFlagByte
				return loc
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.locator().LOBReferenceID()
			requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
		})
	}
}

// TestLobLocator_LOBReferenceIDAcceptsAbstractLocator verifies an initialized
// abstract locator has a stable registry identity.
func TestLobLocator_LOBReferenceIDAcceptsAbstractLocator(t *testing.T) {
	t.Parallel()

	loc := newTestLobReferenceLocator(135)
	loc.locatorBytes[koll4FlagOffset] &^= kolblTemporaryFlagByte
	loc.locatorBytes[koll1FlagOffset] |= kolblAbstractLocatorFlag

	id, err := loc.LOBReferenceID()
	if err != nil {
		t.Fatalf("LOBReferenceID returned error: %v", err)
	}
	want := lobReferenceID(string(loc.locatorBytes[kolbLobIDOffset : kolbLobIDOffset+kolbLobIDLength]))
	if id != want {
		t.Fatalf("LOB ID = %v, want %v", id, want)
	}
}
