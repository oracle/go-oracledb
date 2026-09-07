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
	{Name: "TestError3113default", Categories: "unitary", Exclusive: false, Fn: TestError3113default},
	{Name: "TestErrorUnknownCode", Categories: "unitary", Exclusive: false, Fn: TestErrorUnknownCode},
	{Name: "TestErrorBasic3113", Categories: "unitary", Exclusive: false, Fn: TestErrorBasic3113},
	{Name: "TestErrorBasic3113Fr", Categories: "unitary", Exclusive: false, Fn: TestErrorBasic3113Fr},
	{Name: "TestError3113", Categories: "unitary", Exclusive: false, Fn: TestError3113},
	{Name: "TestError3113Fr", Categories: "unitary", Exclusive: false, Fn: TestError3113Fr},
	{Name: "TestError3113pt", Categories: "unitary", Exclusive: false, Fn: TestError3113pt},
	{Name: "TestError3113NoLanguage", Categories: "unitary", Exclusive: false, Fn: TestError3113NoLanguage},
	{Name: "TestErrorUnwrap", Categories: "unitary", Exclusive: false, Fn: TestErrorUnwrap},
	{Name: "TestError3113InvalidLanguage", Categories: "unitary", Exclusive: false, Fn: TestError3113InvalidLanguage},
	{Name: "TestNewOERMessageError", Categories: "unitary", Exclusive: false, Fn: TestNewOERMessageError},
	{Name: "TestConstants_GetLogonModeFromString", Categories: "unitary", Exclusive: false, Fn: TestConstants_GetLogonModeFromString},
	{Name: "TestConstants_LogonModeEnabled", Categories: "unitary", Exclusive: false, Fn: TestConstants_LogonModeEnabled},
	{Name: "TestConstants_LogonModeString", Categories: "unitary", Exclusive: false, Fn: TestConstants_LogonModeString},
	{Name: "TestProviderRegistryGetProviderReturnsFirstMatch", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistryGetProviderReturnsFirstMatch},
	{Name: "TestProviderRegistryRegisterProviderEvictsOldestWhenCapacityExceeded", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistryRegisterProviderEvictsOldestWhenCapacityExceeded},
	{Name: "TestProviderRegistryGetProviderReturnsRequestedInterface", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistryGetProviderReturnsRequestedInterface},
	{Name: "TestProviderRegistryGetProviderReturnsErrorWhenUninitialized", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistryGetProviderReturnsErrorWhenUninitialized},
	{Name: "TestProviderRegistrySupportsConcreteGenericType", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistrySupportsConcreteGenericType},
	{Name: "TestRegistryGetAllReturnsSnapshotInRegistrationOrder", Categories: "unitary", Exclusive: false, Fn: TestRegistryGetAllReturnsSnapshotInRegistrationOrder},
	{Name: "TestRegistryGetAllReturnsEmptySnapshotWhenUninitialized", Categories: "unitary", Exclusive: false, Fn: TestRegistryGetAllReturnsEmptySnapshotWhenUninitialized},
	{Name: "TestRegistryGetSkipsNilItems", Categories: "unitary", Exclusive: false, Fn: TestRegistryGetSkipsNilItems},
}
