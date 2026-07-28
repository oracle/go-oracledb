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
	{"TestSetAndGet", "unitary", false, TestSetAndGet},
	{"TestSetBytesAndSetByte", "unitary", false, TestSetBytesAndSetByte},
	{"TestCardinality", "unitary", false, TestCardinality},
	{"TestClearAllAndLength", "unitary", false, TestClearAllAndLength},
	{"TestStringFormat", "unitary", false, TestStringFormat},
	{"TestSetBytes_OutOfBounds", "unitary", false, TestSetBytes_OutOfBounds},
	{"TestGetSet_OutOfBounds", "unitary", false, TestGetSet_OutOfBounds},
	{"TestNewBitSetFromBytes", "unitary", false, TestNewBitSetFromBytes},
	{"TestConstants_Protocol", "unitary", false, TestConstants_Protocol},
	{"TestErrorUnknownCode", "unitary", false, TestErrorUnknownCode},
	{"TestErrorBasic3113", "unitary", false, TestErrorBasic3113},
	{"TestErrorBasic3113Fr", "unitary", false, TestErrorBasic3113Fr},
	{"TestError3113", "unitary", false, TestError3113},
	{"TestError3113Fr", "unitary", false, TestError3113Fr},
	{"TestError3113pt", "unitary", false, TestError3113pt},
	{"TestError3113NoLanguage", "unitary", false, TestError3113NoLanguage},
	{"TestErrorUnwrap", "unitary", false, TestErrorUnwrap},
	{"TestError3113InvalidLanguage", "unitary", false, TestError3113InvalidLanguage},
	{"TestNewOERMessageError", "unitary", false, TestNewOERMessageError},
	{"TestShelf_NewShelfTest", "unitary", false, TestShelf_NewShelfTest},
	{"TestShelf_GetMarshaller", "unitary", false, TestShelf_GetMarshaller},
	{"TestShelf_GetMessageFactory", "unitary", false, TestShelf_GetMessageFactory},
	{"TestShelf_GetStreamer", "unitary", false, TestShelf_GetStreamer},
	{"TestShelf_GetCapabilities", "unitary", false, TestShelf_GetCapabilities},
	{"TestUtility_SimpleStringToB1Array", "unitary", false, TestUtility_SimpleStringToB1Array},
	{"TestUtility_EmptyStringToB1Array", "unitary", false, TestUtility_EmptyStringToB1Array},
	{"TestUtility_NonBMPStringToB1Array", "unitary", false, TestUtility_NonBMPStringToB1Array},
	{"TestUtility_GetTimeZoneBytes", "unitary", false, TestUtility_GetTimeZoneBytes},
	{"TestNibbleToHex", "unitary", false, TestNibbleToHex},
	{"TestBArray2Nibbles", "unitary", false, TestBArray2Nibbles},
	{"TestToBinArray", "unitary", false, TestToBinArray},
	{"TestConfiguration_AssignFromEmptyMap", "unitary", false, TestConfiguration_AssignFromEmptyMap},
	{"TestConfiguration_AssignFromMapUnknownKey", "unitary", false, TestConfiguration_AssignFromMapUnknownKey},
	{"TestConfiguration_AssignFromMap", "unitary", false, TestConfiguration_AssignFromMap},
	{"TestConfiguration_AssignFromMapValidatedIntString", "unitary", false, TestConfiguration_AssignFromMapValidatedIntString},
	{"TestConfiguration_AssignFromEnv", "unitary", true, TestConfiguration_AssignFromEnv},
	{"TestConfiguration_AssignFromEnvValidatedIntString", "unitary", true, TestConfiguration_AssignFromEnvValidatedIntString},
	{"TestConfiguration_AssignFromEmptyFlags", "unitary", false, TestConfiguration_AssignFromEmptyFlags},
	{"TestConfiguration_Clone", "unitary", false, TestConfiguration_Clone},
	{"TestInitLoggingWithConfigFileDestination", "unitary", false, TestInitLoggingWithConfigFileDestination},
	{"TestEnquoteLiteral", "unitary", false, TestEnquoteLiteral},
	{"TestEnquoteNCharLiteral", "unitary", false, TestEnquoteNCharLiteral},
	{"TestIsSimpleIdentifier", "unitary", false, TestIsSimpleIdentifier},
	{"TestEnquoteIdentifier", "unitary", false, TestEnquoteIdentifier},
	{"TestConfiguration_DefaultClientLanguageIsLanguageTag", "unitary", false, TestConfiguration_DefaultClientLanguageIsLanguageTag},
	{"TestConfiguration_AssignFromMapClientLanguageTag", "unitary", false, TestConfiguration_AssignFromMapClientLanguageTag},
	{"TestConfiguration_AssignFromEnvClientLanguageTag", "unitary", true, TestConfiguration_AssignFromEnvClientLanguageTag},
	{"TestConfiguration_toNSConnectionParameters", "unitary", false, TestConfiguration_toNSConnectionParameters},
	{"TestConstants_ProtocolString", "unitary", false, TestConstants_ProtocolString},
	{"TestConstants_GetLogonModeFromString", "unitary", false, TestConstants_GetLogonModeFromString},
	{"TestConstants_LogonModeEnabled", "unitary", false, TestConstants_LogonModeEnabled},
	{"TestConstants_LogonModeString", "unitary", false, TestConstants_LogonModeString},
	{"TestError3113default", "unitary", false, TestError3113default},
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
