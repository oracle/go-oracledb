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
** one is included with the Software (each a "Larger Work") to which the Software
** is contributed by such licensors,
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

package lob

import (
	"fmt"
	"os"
	"testing"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

func TestMain(m *testing.M) {
	if err := oracleTest.InitConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "InitConfig failed: %v\n", err)
		os.Exit(1)
	}
	TestEnvironement = oracleTest.TestEnvironement
	TestingConfig = oracleTest.TestingConfig
	DefaultTestConfig = oracleTest.DefaultTestConfig
	TestCategories = oracleTest.TestCategories
	os.Exit(m.Run())
}

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestLOBValue_ImplementsPublicContracts", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_ImplementsPublicContracts},
	{Name: "TestLOBScan_ValidatesKindAndSupportsReadAll", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_ValidatesKindAndSupportsReadAll},
	{Name: "TestLOBScan_RejectsWrongKindAndInvalidSourceAndHandlesNull", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_RejectsWrongKindAndInvalidSourceAndHandlesNull},
	{Name: "TestLOBScan_ReadAllPropagatesReadError", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_ReadAllPropagatesReadError},
	{Name: "TestLOBValue_TemporaryConstructors", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_TemporaryConstructors},
	{Name: "TestLOBValue_ContextMethodsCheckCancellation", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_ContextMethodsCheckCancellation},
	{Name: "TestLOBScan_ZeroValueAdoptsUnreadSource", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_ZeroValueAdoptsUnreadSource},
	{Name: "TestLOBScan_AdoptedPrefixSurvivesRowsClose", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_AdoptedPrefixSurvivesRowsClose},
	{Name: "TestLOBScan_CompletePrefixNeedsNoLocatorRead", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_CompletePrefixNeedsNoLocatorRead},
	{Name: "TestLOBHandle_AcceptedWriteBytes", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_AcceptedWriteBytes},
	{Name: "TestLOBHandle_IsTemporary", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_IsTemporary},
	{Name: "TestLOBHandle_TrimClearsPendingCLOB", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_TrimClearsPendingCLOB},
	{Name: "TestLOBHandle_ReadAheadDoesNotMoveWriteCursor", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_ReadAheadDoesNotMoveWriteCursor},
	{Name: "TestLOBHandle_PreCanceledMutationsDoNotInvalidate", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_PreCanceledMutationsDoNotInvalidate},
	{Name: "TestLOBHandle_MutationsUpdatePrefixCompleteness", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_MutationsUpdatePrefixCompleteness},
	{Name: "TestLOBHandle_CLOBWriteRequiresCharacterBoundary", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_CLOBWriteRequiresCharacterBoundary},
	{Name: "TestLOBHandle_TrimTextCarryRequiresCharacterBoundary", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_TrimTextCarryRequiresCharacterBoundary},
	{Name: "TestLOBHandle_TrimZeroResetsCursor", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_TrimZeroResetsCursor},
	{Name: "TestLOBHandle_OpenRequiresCloseServerBeforeClose", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_OpenRequiresCloseServerBeforeClose},
	{Name: "TestLOBHandle_OpenRequiresCloseServerBeforeFree", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_OpenRequiresCloseServerBeforeFree},
	{Name: "TestLOBHandle_CloseServerFailureRequiresReconciliation", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_CloseServerFailureRequiresReconciliation},
	{Name: "TestLOBHandle_OpenCancellationRequiresIsOpenReconciliation", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_OpenCancellationRequiresIsOpenReconciliation},
	{Name: "TestLOBHandle_CloseServerCancellationRequiresIsOpenReconciliation", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_CloseServerCancellationRequiresIsOpenReconciliation},
	{Name: "TestLOBHandle_BlobReadCapsRefillAndConsumesPending", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_BlobReadCapsRefillAndConsumesPending},
	{Name: "TestLOBHandle_OneBasedLOBPosition", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_OneBasedLOBPosition},
	{Name: "TestLOBHandle_ReadAtKeepsSequentialCursorAndReadAhead", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_ReadAtKeepsSequentialCursorAndReadAhead},
	{Name: "TestLOBHandle_ReadAtValidationAndChunking", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_ReadAtValidationAndChunking},
	{Name: "TestLOBHandle_ReadTextAtUsesUTF16UnitsAndKeepsSequentialState", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_ReadTextAtUsesUTF16UnitsAndKeepsSequentialState},
	{Name: "TestLOBHandle_CreateTemporaryValidation", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_CreateTemporaryValidation},
	{Name: "TestLOBHandle_ReadWriteAndWriteTo", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_ReadWriteAndWriteTo},
	{Name: "TestLOBHandle_WriteContextChunksBoundsRequestsAndPreservesTextCharacters", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_WriteContextChunksBoundsRequestsAndPreservesTextCharacters},
	{Name: "TestLOBHandle_WriteContextPartialProgress", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_WriteContextPartialProgress},
	{Name: "TestLOBHandle_TextWriteRequiresCompleteUTF8PerCall", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_TextWriteRequiresCompleteUTF8PerCall},
	{Name: "TestLOBValue_WriteStringMethods", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_WriteStringMethods},
	{Name: "TestLOBValue_WriteStringContextReturnsAcknowledgedPrefix", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_WriteStringContextReturnsAcknowledgedPrefix},
	{Name: "TestLOBHandle_OperationsAndLifecycle", Categories: "unitary", Exclusive: false, Fn: TestLOBHandle_OperationsAndLifecycle},
	{Name: "TestLOBValue_TemporaryConstructorsRejectNilConn", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_TemporaryConstructorsRejectNilConn},
	{Name: "TestLOBValue_ZeroValueStateAndOperations", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_ZeroValueStateAndOperations},
	{Name: "TestLOBValue_ForwardsOperationsToHandle", Categories: "unitary", Exclusive: false, Fn: TestLOBValue_ForwardsOperationsToHandle},
	{Name: "TestLOBScan_InternalRejectsInvalidExpectedKindAndMissingAdoption", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_InternalRejectsInvalidExpectedKindAndMissingAdoption},
	{Name: "TestLOBScan_BytesReadsAndCloses", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_BytesReadsAndCloses},
	{Name: "TestLOBScan_TextReadsCharacterKindsAndCloses", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_TextReadsCharacterKindsAndCloses},
	{Name: "TestLOBScan_RejectsNullAndWrongKind", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_RejectsNullAndWrongKind},
	{Name: "TestLOBScan_WrongKindReturnsCloseError", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_WrongKindReturnsCloseError},
	{Name: "TestLOBScan_RejectsTypedNilSource", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_RejectsTypedNilSource},
	{Name: "TestLOBScan_ScanPropagatesReadErrors", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_ScanPropagatesReadErrors},
	{Name: "TestLOBScan_ReadAllSizeErrors", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_ReadAllSizeErrors},
	{Name: "TestLOBScan_ReadAllErrors", Categories: "unitary", Exclusive: false, Fn: TestLOBScan_ReadAllErrors},
}

func TestCategoryExecutor(t *testing.T) {
	oracleTest.RunCategoryExecutor(t, TestCategories, testCases)
}

type Version = oracleTest.Version
type TestConfig = oracleTest.TestConfig
type TestingEnvironment = oracleTest.TestingEnvironment

var DefaultTestConfig *TestConfig
var TestEnvironement TestingEnvironment
var TestingConfig *TestConfig
var TestCategories oracleTest.TestCategoryList
