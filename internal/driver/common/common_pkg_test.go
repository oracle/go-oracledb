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
	"flag"
	"os"
	"strings"
	"testing"
)

// TestCategory category of tests to be un
var TestCategory string

func TestMain(m *testing.M) {
	flag.StringVar(&TestCategory, "test.category", "", "testing category, can be unitary, functional, performance, robustness")
	os.Exit(m.Run())
}

var testCases = []struct {
	name       string
	categories string
	exclusive  bool
	f          func(t *testing.T)
}{
	{"TestNewBitSet_SizeAlignment", "unitary", false, TestNewBitSet_SizeAlignment},
	{"TestGet", "unitary", false, TestGet},
	{"TestSetBytes", "unitary", false, TestSetBytes},
	{"TestCardinality", "unitary", false, TestCardinality},
	{"TestClearAllAndLength", "unitary", false, TestClearAllAndLength},
	{"TestStringFormat", "unitary", false, TestStringFormat},
	{"TestSetBytes_OutOfBounds", "unitary", false, TestSetBytes_OutOfBounds},
	{"TestShelf_NewShelfTest", "unitary", false, TestShelf_NewShelfTest},
	{"TestShelf_GetMarshaller", "unitary", false, TestShelf_GetMarshaller},
	{"TestShelf_GetMessageFactory", "unitary", false, TestShelf_GetMessageFactory},
	{"TestShelf_GetStreamer", "unitary", false, TestShelf_GetStreamer},
	{"TestShelf_GetCapabilities", "unitary", false, TestShelf_GetCapabilities},
	{"TestB1Array_String", "unitary", false, TestB1Array_String},
	{"TestB1Array_Equals", "unitary", false, TestB1Array_Equals},
	{"TestKeyValue_String", "unitary", false, TestKeyValue_String},
	{"TestKeyValue_Equals", "unitary", false, TestKeyValue_Equals},
	{"TestNewSessionContext", "unitary", false, TestNewSessionContext},
	{"TestSessionContext_SetTimeZoneVersionNumber", "unitary", false, TestSessionContext_SetTimeZoneVersionNumber},
	{"TestSessionContext_SetSessionCharacterSets", "unitary", false, TestSessionContext_SetSessionCharacterSets},
	{"TestSessionContext_UpdateSessionProperties", "unitary", false, TestSessionContext_UpdateSessionProperties},
	{"TestUtility_SimpleStringToB1Array", "unitary", false, TestUtility_SimpleStringToB1Array},
	{"TestUtility_EmptyStringToB1Array", "unitary", false, TestUtility_EmptyStringToB1Array},
	{"TestUtility_NonBMPStringToB1Array", "unitary", false, TestUtility_NonBMPStringToB1Array},
	{"TestUtility_GetTimeZoneBytes", "unitary", false, TestUtility_GetTimeZoneBytes},
	{"TestNibbleToHex", "unitary", false, TestNibbleToHex},
	{"TestBArray2Nibbles", "unitary", false, TestBArray2Nibbles},
	{"TestToBinArray", "unitary", false, TestToBinArray},
	{"TestNewProperties", "unitary", false, TestNewProperties},
	{"TestNewPropertiesWithSource", "unitary", false, TestNewPropertiesWithSource},
	{"TestProperties_ContainsKey", "unitary", false, TestProperties_ContainsKey},
	{"TestProperties_GetProperty", "unitary", false, TestProperties_GetProperty},
	{"TestProperties_GetTrimmedString", "unitary", false, TestProperties_GetTrimmedString},
	{"TestProperties_PutAll", "unitary", false, TestProperties_PutAll},
	{"TestProperties_Reset", "unitary", false, TestProperties_Reset},
	{"TestProperties_SetProperty", "unitary", false, TestProperties_SetProperty},
	{"TestProperties_Snapshot", "unitary", false, TestProperties_Snapshot},
	{"TestProperties_String", "unitary", false, TestProperties_String},
	{"TestShelf_GetLocalizationService", "unitary", false, TestShelf_GetLocalizationService},
	{"TestShelf_LocalizeError", "unitary", false, TestShelf_LocalizeError},
	{"TestStripSpacesOutsideQuotes", "unitary", false, TestStripSpacesOutsideQuotes},
	{"TestConstants_Protocol", "unitary", false, TestConstants_Protocol},
	{"TestConstants_ProtocolString", "unitary", false, TestConstants_ProtocolString},
}

func TestCategoryExecutor(t *testing.T) {
	var regularCases, exclusiveCases []struct {
		name       string
		categories string
		exclusive  bool
		f          func(t *testing.T)
	}

	for _, c := range testCases {
		cats := strings.Split(c.categories, ",")
		for _, p := range cats {
			if strings.Compare(strings.TrimSpace(p), TestCategory) == 0 {
				if c.exclusive {
					exclusiveCases = append(exclusiveCases, c)
				} else {
					regularCases = append(regularCases, c)
				}
				break
			}
		}
	}

	if len(regularCases) > 0 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			for _, c := range regularCases {
				t.Run(c.name, c.f)
			}
		})
	}
	for _, c := range exclusiveCases {
		t.Run(c.name, c.f)
	}
}
