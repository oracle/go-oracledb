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
	"fmt"
	"os"
	"testing"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

func TestMain(m *testing.M) {
	err := oracleTest.InitConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "InitConfig failed: %v\n", err)
		os.Exit(1)
	}
	TestEnvironement = oracleTest.TestEnvironement
	TestingConfig = oracleTest.TestingConfig
	DefaultTestConfig = oracleTest.DefaultTestConfig
	os.Exit(m.Run())
}

func TestCategoryExecutor(t *testing.T) {
	oracleTest.RunCategoryExecutor(t, oracleTest.TestCategory, testCases)
}

type Version = oracleTest.Version
type TestConfig = oracleTest.TestConfig
type TestingEnvironment = oracleTest.TestingEnvironment

var DefaultTestConfig *TestConfig
var TestEnvironement TestingEnvironment
var TestingConfig *TestConfig

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestNewBitSet_SizeAlignment", Categories: "unitary", Exclusive: false, Fn: TestNewBitSet_SizeAlignment},
	{Name: "TestGet", Categories: "unitary", Exclusive: false, Fn: TestGet},
	{Name: "TestSetBytes", Categories: "unitary", Exclusive: false, Fn: TestSetBytes},
	{Name: "TestCardinality", Categories: "unitary", Exclusive: false, Fn: TestCardinality},
	{Name: "TestClearAllAndLength", Categories: "unitary", Exclusive: false, Fn: TestClearAllAndLength},
	{Name: "TestStringFormat", Categories: "unitary", Exclusive: false, Fn: TestStringFormat},
	{Name: "TestSetBytes_OutOfBounds", Categories: "unitary", Exclusive: false, Fn: TestSetBytes_OutOfBounds},
	{Name: "TestShelf_NewShelfTest", Categories: "unitary", Exclusive: false, Fn: TestShelf_NewShelfTest},
	{Name: "TestShelf_GetMarshaller", Categories: "unitary", Exclusive: false, Fn: TestShelf_GetMarshaller},
	{Name: "TestShelf_GetMessageFactory", Categories: "unitary", Exclusive: false, Fn: TestShelf_GetMessageFactory},
	{Name: "TestShelf_GetStreamer", Categories: "unitary", Exclusive: false, Fn: TestShelf_GetStreamer},
	{Name: "TestShelf_GetCapabilities", Categories: "unitary", Exclusive: false, Fn: TestShelf_GetCapabilities},
	{Name: "TestB1Array_String", Categories: "unitary", Exclusive: false, Fn: TestB1Array_String},
	{Name: "TestB1Array_Equals", Categories: "unitary", Exclusive: false, Fn: TestB1Array_Equals},
	{Name: "TestKeyValue_String", Categories: "unitary", Exclusive: false, Fn: TestKeyValue_String},
	{Name: "TestKeyValue_Equals", Categories: "unitary", Exclusive: false, Fn: TestKeyValue_Equals},
	{Name: "TestNewSessionContext", Categories: "unitary", Exclusive: false, Fn: TestNewSessionContext},
	{Name: "TestSessionContext_SetTimeZoneVersionNumber", Categories: "unitary", Exclusive: false, Fn: TestSessionContext_SetTimeZoneVersionNumber},
	{Name: "TestSessionContext_SetSessionCharacterSets", Categories: "unitary", Exclusive: false, Fn: TestSessionContext_SetSessionCharacterSets},
	{Name: "TestSessionContext_UpdateSessionProperties", Categories: "unitary", Exclusive: false, Fn: TestSessionContext_UpdateSessionProperties},
	{Name: "TestUtility_SimpleStringToB1Array", Categories: "unitary", Exclusive: false, Fn: TestUtility_SimpleStringToB1Array},
	{Name: "TestUtility_EmptyStringToB1Array", Categories: "unitary", Exclusive: false, Fn: TestUtility_EmptyStringToB1Array},
	{Name: "TestUtility_NonBMPStringToB1Array", Categories: "unitary", Exclusive: false, Fn: TestUtility_NonBMPStringToB1Array},
	{Name: "TestUtility_GetTimeZoneBytes", Categories: "unitary", Exclusive: false, Fn: TestUtility_GetTimeZoneBytes},
	{Name: "TestNibbleToHex", Categories: "unitary", Exclusive: false, Fn: TestNibbleToHex},
	{Name: "TestBArray2Nibbles", Categories: "unitary", Exclusive: false, Fn: TestBArray2Nibbles},
	{Name: "TestToBinArray", Categories: "unitary", Exclusive: false, Fn: TestToBinArray},
	{Name: "TestNewProperties", Categories: "unitary", Exclusive: false, Fn: TestNewProperties},
	{Name: "TestNewPropertiesWithSource", Categories: "unitary", Exclusive: false, Fn: TestNewPropertiesWithSource},
	{Name: "TestProperties_ContainsKey", Categories: "unitary", Exclusive: false, Fn: TestProperties_ContainsKey},
	{Name: "TestProperties_GetProperty", Categories: "unitary", Exclusive: false, Fn: TestProperties_GetProperty},
	{Name: "TestProperties_GetTrimmedString", Categories: "unitary", Exclusive: false, Fn: TestProperties_GetTrimmedString},
	{Name: "TestProperties_PutAll", Categories: "unitary", Exclusive: false, Fn: TestProperties_PutAll},
	{Name: "TestProperties_Reset", Categories: "unitary", Exclusive: false, Fn: TestProperties_Reset},
	{Name: "TestProperties_SetProperty", Categories: "unitary", Exclusive: false, Fn: TestProperties_SetProperty},
	{Name: "TestProperties_Snapshot", Categories: "unitary", Exclusive: false, Fn: TestProperties_Snapshot},
	{Name: "TestProperties_String", Categories: "unitary", Exclusive: false, Fn: TestProperties_String},
	{Name: "TestShelf_GetLocalizationService", Categories: "unitary", Exclusive: false, Fn: TestShelf_GetLocalizationService},
	{Name: "TestShelf_LocalizeError", Categories: "unitary", Exclusive: false, Fn: TestShelf_LocalizeError},
	{Name: "TestStripSpacesOutsideQuotes", Categories: "unitary", Exclusive: false, Fn: TestStripSpacesOutsideQuotes},
	{Name: "TestConstants_Protocol", Categories: "unitary", Exclusive: false, Fn: TestConstants_Protocol},
	{Name: "TestConstants_ProtocolString", Categories: "unitary", Exclusive: false, Fn: TestConstants_ProtocolString},
	{Name: "TestShelf_ConnectionProperties", Categories: "unitary", Exclusive: false, Fn: TestShelf_ConnectionProperties},
	{Name: "TestUtility_B1ArrayToString", Categories: "unitary", Exclusive: false, Fn: TestUtility_B1ArrayToString},
}
