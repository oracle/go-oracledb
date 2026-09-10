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
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
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
	oracleTest.RunCategoryExecutor(t, oracleTest.TestCategories, testCases)
}

type Version = oracleTest.Version
type TestConfig = oracleTest.TestConfig
type TestingEnvironment = oracleTest.TestingEnvironment

var DefaultTestConfig *TestConfig
var TestEnvironement TestingEnvironment
var TestingConfig *TestConfig

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestCapabilityNew", Categories: "unitary", Exclusive: false, Fn: TestCapabilityNew},
	{Name: "TestCapabilityNewDefault", Categories: "unitary", Exclusive: false, Fn: TestCapabilityNewDefault},
	{Name: "TestCapabilityMarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestCapabilityMarshalTo_Success},
	{Name: "TestCapabilityMarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestCapabilityMarshalTo_Fail},
	{Name: "TestCapabilityUnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestCapabilityUnMarshalFrom_Success},
	{Name: "TestCapabilityUnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestCapabilityUnMarshalFrom_Fail},
	{Name: "TestCapability_AdjustCapabilityFrom", Categories: "unitary", Exclusive: false, Fn: TestCapability_AdjustCapabilityFrom},
	{Name: "TestCapability_toMap", Categories: "unitary", Exclusive: false, Fn: TestCapability_toMap},
	{Name: "TestConnectionCloser_Close", Categories: "unitary", Exclusive: false, Fn: TestConnectionCloser_Close},
	{Name: "TestConnectionCloser_CloseWithTimeout", Categories: "unitary", Exclusive: false, Fn: TestConnectionCloser_CloseWithTimeout},
	{Name: "TestEventServiceRegisterAndPost", Categories: "unitary", Exclusive: false, Fn: TestEventServiceRegisterAndPost},
	{Name: "TestAuthencationFactoryWithNilParameters", Categories: "unitary", Exclusive: false, Fn: TestAuthencationFactoryWithNilParameters},
	{Name: "TestAuthencationFactoryBasic", Categories: "unitary", Exclusive: false, Fn: TestAuthencationFactoryBasic},
	{Name: "TestGetAuthenticator_UsesTokenAuthenticatorForSignedToken", Categories: "unitary", Exclusive: false, Fn: TestGetAuthenticator_UsesTokenAuthenticatorForSignedToken},
	{Name: "TestGetAuthenticator_UsesTokenAuthenticatorForOAuth", Categories: "unitary", Exclusive: false, Fn: TestGetAuthenticator_UsesTokenAuthenticatorForOAuth},
	{Name: "TestGetConnection", Categories: "unitary", Exclusive: false, Fn: TestGetConnection},
	{Name: "TestConnectionPinger_Ping", Categories: "unitary", Exclusive: false, Fn: TestConnectionPinger_Ping},
	{Name: "TestConnectionPinger_IsValid", Categories: "unitary", Exclusive: false, Fn: TestConnectionPinger_IsValid},
	{Name: "TestConnectionPinger_IsValidWithInband", Categories: "unitary", Exclusive: false, Fn: TestConnectionPinger_IsValidWithInband},
	{Name: "TestFactoryRegistries", Categories: "unitary", Exclusive: false, Fn: TestFactoryRegistries},
	{Name: "TestReplaceMessage", Categories: "unitary", Exclusive: false, Fn: TestReplaceMessage},
	{Name: "TestFactoryGetMessage", Categories: "unitary", Exclusive: false, Fn: TestFactoryGetMessage},
	{Name: "TestFactoryGetMessageFromFunction", Categories: "unitary", Exclusive: false, Fn: TestFactoryGetMessageFromFunction},
	{Name: "TestNewMarshalEngine", Categories: "unitary", Exclusive: false, Fn: TestNewMarshalEngine},
	{Name: "TestMarshalUB1", Categories: "unitary", Exclusive: false, Fn: TestMarshalUB1},
	{Name: "TestMarshalUB2", Categories: "unitary", Exclusive: false, Fn: TestMarshalUB2},
	{Name: "TestMarshalSB4", Categories: "unitary", Exclusive: false, Fn: TestMarshalSB4},
	{Name: "TestMarshalUB4", Categories: "unitary", Exclusive: false, Fn: TestMarshalUB4},
	{Name: "TestMarshalUB8", Categories: "unitary", Exclusive: false, Fn: TestMarshalUB8},
	{Name: "TestMarshalByteArray", Categories: "unitary", Exclusive: false, Fn: TestMarshalByteArray},
	{Name: "TestMarshalChar", Categories: "unitary", Exclusive: false, Fn: TestMarshalChar},
	{Name: "TestMarshalPTR", Categories: "unitary", Exclusive: false, Fn: TestMarshalPTR},
	{Name: "TestMarshalNullPTR", Categories: "unitary", Exclusive: false, Fn: TestMarshalNullPTR},
	{Name: "TestMarshalCLR", Categories: "unitary", Exclusive: false, Fn: TestMarshalCLR},
	{Name: "TestMarshalKeyValue", Categories: "unitary", Exclusive: false, Fn: TestMarshalKeyValue},
	{Name: "TestFlush", Categories: "unitary", Exclusive: false, Fn: TestFlush},
	{Name: "TestUnmarshalUB1", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB1},
	{Name: "TestUnmarshalUB2", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB2},
	{Name: "TestUnmarshalSB1", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalSB1},
	{Name: "TestUnmarshalSB2", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalSB2},
	{Name: "TestUnmarshalSB4", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalSB4},
	{Name: "TestUnmarshalUB4", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB4},
	{Name: "TestUnmarshalUB8", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB8},
	{Name: "TestUnmarshalCLR", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLR},
	{Name: "TestUnmarshalByteArray", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalByteArray},
	{Name: "TestUnmarshalText", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalText},
	{Name: "TestUnmarshalKeyValue", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalKeyValue},
	{Name: "TestMarshalUB1NoSpace", Categories: "unitary", Exclusive: false, Fn: TestMarshalUB1NoSpace},
	{Name: "TestMarshalCLRNoSpace", Categories: "unitary", Exclusive: false, Fn: TestMarshalCLRNoSpace},
	{Name: "TestMarshalKeyValueNoSpace", Categories: "unitary", Exclusive: false, Fn: TestMarshalKeyValueNoSpace},
	{Name: "TestUnmarshalKeyValueNoSpace", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalKeyValueNoSpace},
	{Name: "TestUnmarshalUB2NoSpace", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB2NoSpace},
	{Name: "TestUnmarshalUB4NoSpace", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB4NoSpace},
	{Name: "TestUnmarshalUB4WithNegativeZeroLength", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB4WithNegativeZeroLength},
	{Name: "TestUnmarshalUB1ArrayNoSpace", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUB1ArrayNoSpace},
	{Name: "TestUnmarshalTextNoSpace", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalTextNoSpace},
	{Name: "TestUnmarshalCLREscapeValue", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLREscapeValue},
	{Name: "TestUnmarshalCLRZeroLength", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRZeroLength},
	{Name: "TestUnmarshalCLRNullLengthIndicator", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRNullLengthIndicator},
	{Name: "TestUnmarshalCLRNegativeLength", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRNegativeLength},
	{Name: "TestUnmarshalCLRNotEnoughBytesForLength", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRNotEnoughBytesForLength},
	{Name: "TestUnmarshalCLRBufferToSmallForLength", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRBufferToSmallForLength},
	{Name: "TestUnmarshalCLRReadLessThatTotalLength", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRReadLessThatTotalLength},
	{Name: "TestUnmarshalCLRReadLessThatTotalLengthNoSpace", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRReadLessThatTotalLengthNoSpace},
	{Name: "TestUnmarshalSB2UniversalRangeValidation", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalSB2UniversalRangeValidation},
	{Name: "TestUnmarshalUnsignedUniversalRejectsNegativeFlag", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUnsignedUniversalRejectsNegativeFlag},
	{Name: "TestUnmarshalUniversalRejectsByteCountsOverTargetWidth", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUniversalRejectsByteCountsOverTargetWidth},
	{Name: "TestUnmarshalUniversalUnsignedRejectsNegativeEncodings", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalUniversalUnsignedRejectsNegativeEncodings},
	{Name: "TestUnmarshalSB4UniversalRangeValidation", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalSB4UniversalRangeValidation},
	{Name: "TestDynamicAllocatedArrayRejectsNegativeUniversalLength", Categories: "unitary", Exclusive: false, Fn: TestDynamicAllocatedArrayRejectsNegativeUniversalLength},
	{Name: "TestMarshalUnmarshalUB8UsesB8Representation", Categories: "unitary", Exclusive: false, Fn: TestMarshalUnmarshalUB8UsesB8Representation},
	{Name: "TestMarshalDALC", Categories: "unitary", Exclusive: false, Fn: TestMarshalDALC},
	{Name: "TestMarshalKeywordValuePairs", Categories: "unitary", Exclusive: false, Fn: TestMarshalKeywordValuePairs},
	{Name: "TestMarshalKeywordValuePairsWithEmptyValues", Categories: "unitary", Exclusive: false, Fn: TestMarshalKeywordValuePairsWithEmptyValues},
	{Name: "TestMarshalSequenceNumber", Categories: "unitary", Exclusive: false, Fn: TestMarshalSequenceNumber},
	{Name: "TestMarshalSequenceNumberRotation", Categories: "unitary", Exclusive: false, Fn: TestMarshalSequenceNumberRotation},
	{Name: "TestMarshalTokenNumber", Categories: "unitary", Exclusive: false, Fn: TestMarshalTokenNumber},
	{Name: "TestMessageStreamer_Flush", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_Flush},
	{Name: "TestMessageStreamer_NullPull", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_NullPull},
	{Name: "TestMessageStreamer_SimpleTypedPull", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_SimpleTypedPull},
	{Name: "TestMessageStreamer_CallbackRegisterPreUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackRegisterPreUnmarshal},
	{Name: "TestMessageStreamer_CallbackRegisterPostUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackRegisterPostUnmarshal},
	{Name: "TestMessageStreamer_CallbackUnregister", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackUnregister},
	{Name: "TestMessageStreamer_Drain", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_Drain},
	{Name: "TestMessageStreamer_IsValidReturnsFalseAndRaisesStaleEvent", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_IsValidReturnsFalseAndRaisesStaleEvent},
	{Name: "TestMessageStreamer_SimplePush", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_SimplePush},
	{Name: "TestMessageStreamer_PullFromIncomings", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_PullFromIncomings},
	{Name: "TestMessageStreamer_OrderedPush", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_OrderedPush},
	{Name: "TestMessageStreamer_PullWithTimeout", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_PullWithTimeout},
	{Name: "TestMessageStreamer_PullWithTimeoutAndIncomings", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_PullWithTimeoutAndIncomings},
	{Name: "TestMessageStreamer_CallbackPullWithPreUnmarshallAlloc", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackPullWithPreUnmarshallAlloc},
	{Name: "TestMessageStreamer_CallbackPullWithPreUnmarshallError", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackPullWithPreUnmarshallError},
	{Name: "TestMessageStreamer_UnmarshallError", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_UnmarshallError},
	{Name: "TestMessageStreamer_CallbackPullWithPostUnmarshallKeepFalse", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackPullWithPostUnmarshallKeepFalse},
	{Name: "TestMessageStreamer_CallbackPullWithPostUnmarshallKeepTrueAndError", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_CallbackPullWithPostUnmarshallKeepTrueAndError},
	{Name: "TestMessageStreamer_getMessage", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_getMessage},
	{Name: "TestMessageHeader_UnMarshalFrom", Categories: "unitary", Exclusive: false, Fn: TestMessageHeader_UnMarshalFrom},
	{Name: "TestOcca_New_Getters", Categories: "unitary", Exclusive: false, Fn: TestOcca_New_Getters},
	{Name: "TestOcca_SetCursorIDs", Categories: "unitary", Exclusive: false, Fn: TestOcca_SetCursorIDs},
	{Name: "TestOcca_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestOcca_MarshalTo_Success},
	{Name: "TestOcca_MarshalTo_EmptyCursorIDs", Categories: "unitary", Exclusive: false, Fn: TestOcca_MarshalTo_EmptyCursorIDs},
	{Name: "TestOcca_MarshalTo_Failures", Categories: "unitary", Exclusive: false, Fn: TestOcca_MarshalTo_Failures},
	{Name: "TestRegisterServerToClientPiggybacks", Categories: "unitary", Exclusive: false, Fn: TestRegisterServerToClientPiggybacks},
	{Name: "TestHandleServerToClientPiggyback_ValidOCSSYNC", Categories: "unitary", Exclusive: false, Fn: TestHandleServerToClientPiggyback_ValidOCSSYNC},
	{Name: "TestHandleServerToClientPiggyback_NotFunction", Categories: "unitary", Exclusive: false, Fn: TestHandleServerToClientPiggyback_NotFunction},
	{Name: "TestHandleServerToClientPiggyback_UnknownFunction", Categories: "unitary", Exclusive: false, Fn: TestHandleServerToClientPiggyback_UnknownFunction},
	{Name: "TestHandleServerToClientPiggyback_ErrorInMessage", Categories: "unitary", Exclusive: false, Fn: TestHandleServerToClientPiggyback_ErrorInMessage},
	{Name: "TestParseAndOrder_Positional_Success_NoDup", Categories: "unitary", Exclusive: false, Fn: TestParseAndOrder_Positional_Success_NoDup},
	{Name: "TestParseAndOrder_Named_Success_WithDupRef", Categories: "unitary", Exclusive: false, Fn: TestParseAndOrder_Named_Success_WithDupRef},
	{Name: "TestParseAndOrder_Named_Error_EmptyNameWithoutValidOrdinal", Categories: "unitary", Exclusive: false, Fn: TestParseAndOrder_Named_Error_EmptyNameWithoutValidOrdinal},
	{Name: "TestParseAndOrder_Named_Error_MissingName", Categories: "unitary", Exclusive: false, Fn: TestParseAndOrder_Named_Error_MissingName},
	{Name: "TestParseAndOrder_Named_Error_ExtraName", Categories: "unitary", Exclusive: false, Fn: TestParseAndOrder_Named_Error_ExtraName},
	{Name: "TestParsePlaceholders_Error_DanglingColon", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_Error_DanglingColon},
	{Name: "TestParsePlaceholders_MixedIdentifiers_Supported", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_MixedIdentifiers_Supported},
	{Name: "TestParsePlaceholders_IgnoreInLineComment", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_IgnoreInLineComment},
	{Name: "TestParsePlaceholders_IgnoreInBlockComment", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_IgnoreInBlockComment},
	{Name: "TestParsePlaceholders_IgnoreInSingleQuotedString", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_IgnoreInSingleQuotedString},
	{Name: "TestParsePlaceholders_IgnoreInSingleQuotedStringWithEscape", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_IgnoreInSingleQuotedStringWithEscape},
	{Name: "TestParsePlaceholders_IgnoreInDoubleQuotedIdentifier", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_IgnoreInDoubleQuotedIdentifier},
	{Name: "TestParsePlaceholders_ParseAfterCommentClosed", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_ParseAfterCommentClosed},
	{Name: "TestParsePlaceholders_SkipNonBindColonAssignment", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_SkipNonBindColonAssignment},
	{Name: "TestOrderNamedValues_NamePrecedenceOverOrdinal", Categories: "unitary", Exclusive: false, Fn: TestOrderNamedValues_NamePrecedenceOverOrdinal},
	{Name: "TestOrderNamedValues_RepeatName_FromName", Categories: "unitary", Exclusive: false, Fn: TestOrderNamedValues_RepeatName_FromName},
	{Name: "TestOrderNamedValues_Error_DuplicateOrdinal", Categories: "unitary", Exclusive: false, Fn: TestOrderNamedValues_Error_DuplicateOrdinal},
	{Name: "TestPlSqlNamedValues_Error_MissingName", Categories: "unitary", Exclusive: false, Fn: TestPlSqlNamedValues_Error_MissingName},
	{Name: "TestPlSqlNamedValues_Error_DuplicateName", Categories: "unitary", Exclusive: false, Fn: TestPlSqlNamedValues_Error_DuplicateName},
	{Name: "TestPlSqlNamedValues_LaterNamedBindWithoutEarlierBind", Categories: "unitary", Exclusive: false, Fn: TestPlSqlNamedValues_LaterNamedBindWithoutEarlierBind},
	{Name: "TestParsePlaceholders_Uint16IndexesDoNotWrapAfter255", Categories: "unitary", Exclusive: false, Fn: TestParsePlaceholders_Uint16IndexesDoNotWrapAfter255},
	{Name: "TestClassifySQL_AllKinds", Categories: "unitary", Exclusive: false, Fn: TestClassifySQL_AllKinds},
	{Name: "TestSqlKind_InvalidValues", Categories: "unitary", Exclusive: false, Fn: TestSqlKind_InvalidValues},
	{Name: "TestSqlKind_String_All", Categories: "unitary", Exclusive: false, Fn: TestSqlKind_String_All},
	{Name: "TestGetQueryStatementExecutor_ReturnsSelect", Categories: "unitary", Exclusive: false, Fn: TestGetQueryStatementExecutor_ReturnsSelect},
	{Name: "TestGetExecStatementExecutor_Mapping", Categories: "unitary", Exclusive: false, Fn: TestGetExecStatementExecutor_Mapping},
	{Name: "TestGetQueryStatementExecutor_UnsupportedForNonSelect", Categories: "unitary", Exclusive: false, Fn: TestGetQueryStatementExecutor_UnsupportedForNonSelect},
	{Name: "TestGetExecStatementExecutor_SelectIsError", Categories: "unitary", Exclusive: false, Fn: TestGetExecStatementExecutor_SelectIsError},
	{Name: "TestStatementExecutor_Others_Drop_MarshalAndExec", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_Drop_MarshalAndExec},
	{Name: "TestStatementExecutor_Others_Create_MarshalAndExec", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_Create_MarshalAndExec},
	{Name: "TestStatementExecutor_DML_Insert_MarshalAndExec", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_Insert_MarshalAndExec},
	{Name: "TestStatementExecutorDML_TTIFOBFlushesAndContinuesPull", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutorDML_TTIFOBFlushesAndContinuesPull},
	{Name: "TestStatementExecutor_Select_MarshalAndQuery", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_MarshalAndQuery},
	{Name: "TestStatementExecutor_Select_DoesNotReuseStaleBVCStateAcrossExecutions", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_DoesNotReuseStaleBVCStateAcrossExecutions},
	{Name: "TestStatementExecutor_PlSQL_MarshalAndExec", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_PlSQL_MarshalAndExec},
	{Name: "TestStatementExecutor_Select_FaultyFlush", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_FaultyFlush},
	{Name: "TestStatementExecutor_Select_FaultyPull", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_FaultyPull},
	{Name: "TestStatementExecutor_Select_OER_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_OER_Error},
	{Name: "TestStatementExecutor_Select_FaultyPush", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_FaultyPush},
	{Name: "TestStatementExecutor_DML_FaultyFlush", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_FaultyFlush},
	{Name: "TestStatementExecutor_DML_FaultyPull", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_FaultyPull},
	{Name: "TestStatementExecutor_DML_OER_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_OER_Error},
	{Name: "TestStatementExecutor_Others_FaultyFlush", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_FaultyFlush},
	{Name: "TestStatementExecutor_Others_FaultyPull", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_FaultyPull},
	{Name: "TestStatementExecutor_Others_OER_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_OER_Error},
	{Name: "TestStatementExecutor_Select_Callback_GetMessage_RXD_Error_Integration", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_Callback_GetMessage_RXD_Error_Integration},
	{Name: "TestStatementExecutor_Select_Callback_GetMessage_BVC_Error_Integration", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_Callback_GetMessage_BVC_Error_Integration},
	{Name: "TestStatementExecutor_Select_OallRpaCallback_GetMessageForFunction_Error_Integration", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_OallRpaCallback_GetMessageForFunction_Error_Integration},
	{Name: "TestStatementExecutor_Others_Factory_GetMessageForFunction_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_Factory_GetMessageForFunction_Error},
	{Name: "TestStatementExecutor_DML_Factory_GetMessageForFunction_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_Factory_GetMessageForFunction_Error},
	{Name: "TestStatementExecutor_Select_Factory_GetMessageForFunction_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_Factory_GetMessageForFunction_Error},
	{Name: "TestStatementExecutor_PLSQL_Factory_GetMessageForFunction_Error", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_PLSQL_Factory_GetMessageForFunction_Error},
	{Name: "TestStatementExecutor_DML_FaultyPush", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_FaultyPush},
	{Name: "TestStatementExecutor_Others_FaultyPush", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Others_FaultyPush},
	{Name: "TestPasswordAuthenticator_doOSESSKEY_Golden", Categories: "unitary", Exclusive: false, Fn: TestPasswordAuthenticator_doOSESSKEY_Golden},
	{Name: "TestPasswordAuthenticator_doOAuth_Golden", Categories: "unitary", Exclusive: false, Fn: TestPasswordAuthenticator_doOAuth_Golden},
	{Name: "TestGetAuthenticator_SelectionLogic", Categories: "unitary", Exclusive: false, Fn: TestGetAuthenticator_SelectionLogic},
	{Name: "TestProviderRegistryReturnsFirstRegisteredTokenProvider", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistryReturnsFirstRegisteredTokenProvider},
	{Name: "TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature", Categories: "unitary", Exclusive: false, Fn: TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature},
	{Name: "TestSignedTokenProviderGenerateTokenHeader", Categories: "unitary", Exclusive: false, Fn: TestSignedTokenProviderGenerateTokenHeader},
	{Name: "TestProviderRegistryReturnsNilWhenTokenProviderMissing", Categories: "unitary", Exclusive: false, Fn: TestProviderRegistryReturnsNilWhenTokenProviderMissing},
	{Name: "TestOAuthSetTokenKeyValsForOAUTHAddsTokenOnlyWithoutHeader", Categories: "unitary", Exclusive: false, Fn: TestOAuthSetTokenKeyValsForOAUTHAddsTokenOnlyWithoutHeader},
	{Name: "TestTokenAuthenticatorSignHeaderForSignedProvider", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorSignHeaderForSignedProvider},
	{Name: "TestTokenAuthenticatorSignHeaderForOAuthProviderReturnsEmpty", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorSignHeaderForOAuthProviderReturnsEmpty},
	{Name: "TestValidateJWTExpirationExpired", Categories: "unitary", Exclusive: false, Fn: TestValidateJWTExpirationExpired},
	{Name: "TestTokenAuthenticatorAuthenticateValidation", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorAuthenticateValidation},
	{Name: "TestTokenAuthenticatorHelpers", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorHelpers},
	{Name: "TestValidateJWTExpirationNonExpiredAndMalformed", Categories: "unitary", Exclusive: false, Fn: TestValidateJWTExpirationNonExpiredAndMalformed},
	{Name: "TestTokenAuthenticatorAuthenticateBuildsOAuthMessage", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorAuthenticateBuildsOAuthMessage},
	{Name: "TestTokenAuthenticatorAuthenticateRequiresRPAForSuccessfulOER", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorAuthenticateRequiresRPAForSuccessfulOER},
	{Name: "TestTokenAuthenticatorSettersAndHeaderValidation", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorSettersAndHeaderValidation},
	{Name: "TestTokenAuthenticatorSignHeaderErrors", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorSignHeaderErrors},
	{Name: "TestTokenAuthenticatorAuthenticateStreamerErrors", Categories: "unitary", Exclusive: false, Fn: TestTokenAuthenticatorAuthenticateStreamerErrors},
	{Name: "TestOAuth_prepareForTokenOAUTH", Categories: "unitary", Exclusive: false, Fn: TestOAuth_prepareForTokenOAUTH},
	{Name: "TestConnectionNegotiator_Negotiate_Fail", Categories: "unitary", Exclusive: false, Fn: TestConnectionNegotiator_Negotiate_Fail},
	{Name: "TestConnectionNegotiator_Negotiate_Success", Categories: "unitary", Exclusive: false, Fn: TestConnectionNegotiator_Negotiate_Success},
	{Name: "TestStatement_QueryContext_JSONConstructor_NamedBindAfterQuotedKey", Categories: "unitary", Exclusive: false, Fn: TestStatement_QueryContext_JSONConstructor_NamedBindAfterQuotedKey},
	{Name: "TestStatement_QueryContext_JSONConstructor_AdditionalNamedBindShapes", Categories: "unitary", Exclusive: false, Fn: TestStatement_QueryContext_JSONConstructor_AdditionalNamedBindShapes},
	{Name: "TestTTIbvc_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestTTIbvc_GetMsgCode},
	{Name: "TestTTIbvc_SetNumberOfColumns", Categories: "unitary", Exclusive: false, Fn: TestTTIbvc_SetNumberOfColumns},
	{Name: "TestTTIbvc_UnMarshalFrom", Categories: "unitary", Exclusive: false, Fn: TestTTIbvc_UnMarshalFrom},
	{Name: "TestTTIbvc_SetBitVector", Categories: "unitary", Exclusive: false, Fn: TestTTIbvc_SetBitVector},
	{Name: "TestTTIdcb24Constructor", Categories: "unitary", Exclusive: false, Fn: TestTTIdcb24Constructor},
	{Name: "TestTTIdcb_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestTTIdcb_GetMsgCode},
	{Name: "TestTTIdcb24UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIdcb24UnMarshalFrom_Success},
	{Name: "TestTTIdcb24UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIdcb24UnMarshalFrom_Fail},
	{Name: "TestTTIdcb_receiveCommon_FromOdny_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIdcb_receiveCommon_FromOdny_Fail},
	{Name: "TestTTIdcb_PerColumnUDS_NoAliasing", Categories: "unitary", Exclusive: false, Fn: TestTTIdcb_PerColumnUDS_NoAliasing},
	{Name: "TestTTIdtyNew", Categories: "unitary", Exclusive: false, Fn: TestTTIdtyNew},
	{Name: "TestTTIdtyMarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIdtyMarshalTo_Success},
	{Name: "TestTTIdtyMarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIdtyMarshalTo_Fail},
	{Name: "TestTTIdtyGetters", Categories: "unitary", Exclusive: false, Fn: TestTTIdtyGetters},
	{Name: "TestTTIdtyUnmarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIdtyUnmarshalFrom_Success},
	{Name: "TestTTIdtyUnmarshalFrom_Failure", Categories: "unitary", Exclusive: false, Fn: TestTTIdtyUnmarshalFrom_Failure},
	{Name: "TestTTIFOB_MessageStreamerFlushesOnlyMessageCode", Categories: "unitary", Exclusive: false, Fn: TestTTIFOB_MessageStreamerFlushesOnlyMessageCode},
	{Name: "TestTTIFunNoPayload_MarshalTo", Categories: "unitary", Exclusive: false, Fn: TestTTIFunNoPayload_MarshalTo},
	{Name: "TestNewLogOff", Categories: "unitary", Exclusive: false, Fn: TestNewLogOff},
	{Name: "TestNewLogOff18", Categories: "unitary", Exclusive: false, Fn: TestNewLogOff18},
	{Name: "TestTTIoac_NewTTIoac", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_NewTTIoac},
	{Name: "TestTTIoac_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_UnMarshalFrom_Success},
	{Name: "TestTTIoac_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_UnMarshalFrom_Fail},
	{Name: "TestTTIoac_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_MarshalTo_Success},
	{Name: "TestTTIoac_MarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_MarshalTo_Fail},
	{Name: "TestTTIoac_SignedArrayElementCount",Categories: "unitary", Exclusive: false, Fn: TestTTIoac_SignedArrayElementCount},
	{Name: "TestTTIoac_AddFlagsContinuation", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_AddFlagsContinuation},
	{Name: "TestTTIoac_UnmarshalNormalizesNumberLength", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_UnmarshalNormalizesNumberLength},
	{Name: "TestTTIoac_UnmarshalNormalizesDateAndTimestampTZLength", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_UnmarshalNormalizesDateAndTimestampTZLength},
	{Name: "TestTTIoac_Setters", Categories: "unitary", Exclusive: false, Fn: TestTTIoac_Setters},
	{Name: "TestImplicitResultRowsNextResultSet", Categories: "unitary", Exclusive: false, Fn: TestImplicitResultRowsNextResultSet},
	{Name: "TestTTCRows_RefCursorNextAndClose", Categories: "unitary", Exclusive: false, Fn: TestTTCRows_RefCursorNextAndClose},
	{Name: "TestTTIimplres_ZeroResultSets", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_ZeroResultSets},
	{Name: "TestTTIimplres_MultipleResultSets", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_MultipleResultSets},
	{Name: "TestTTIimplres_RejectsUnconfiguredAndTruncatedMessages", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_RejectsUnconfiguredAndTruncatedMessages},
	{Name: "TestTTIimplres_PrefetchCompletion", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_PrefetchCompletion},
	{Name: "TestTTIimplres_PrefetchColumnPresenceVector", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_PrefetchColumnPresenceVector},
	{Name: "TestTTIimplres_ConfigurationAndUnexpectedPrefetchMessage", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_ConfigurationAndUnexpectedPrefetchMessage},
	{Name: "TestTTIimplres_DecodeErrors", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_DecodeErrors},
	{Name: "TestTTIimplres_RefCursorDCBHeaderErrors", Categories: "unitary", Exclusive: false, Fn: TestTTIimplres_RefCursorDCBHeaderErrors},
	{Name: "TestTTIOallRPA_Unmarshal_Drop", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_Unmarshal_Drop},
	{Name: "TestTTIOallRPA_Unmarshal_Create", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_Unmarshal_Create},
	{Name: "TestTTIOallRPA_Unmarshal_Insert", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_Unmarshal_Insert},
	{Name: "TestTTIOallRPA_Unmarshal_AlterSessionDrop", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_Unmarshal_AlterSessionDrop},
	{Name: "TestTTIOallRPA_UnmarshalFrom_Fail_Truncated", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_UnmarshalFrom_Fail_Truncated},
	{Name: "TestTTIOallRPA_UnmarshalFrom_FaultyBuffer", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_UnmarshalFrom_FaultyBuffer},
	{Name: "TestTTIOallRPA_UnmarshalFrom_UnsupportedNonZeroValues", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_UnmarshalFrom_UnsupportedNonZeroValues},
	{Name: "TestTTIOallRPA_UnmarshalDMLRows_FaultyBuffer", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_UnmarshalDMLRows_FaultyBuffer},
	{Name: "TestTTIOallRPA_Unmarshal_TransactionContext", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_Unmarshal_TransactionContext},
	{Name: "TestTTIOallRPA_Unmarshal_TransactionContext_Fail_Bytes", Categories: "unitary", Exclusive: false, Fn: TestTTIOallRPA_Unmarshal_TransactionContext_Fail_Bytes},
	{Name: "TestOall8_New_Success", Categories: "unitary", Exclusive: false, Fn: TestOall8_New_Success},
	{Name: "TestOall8_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestOall8_GetMsgCode},
	{Name: "TestOall8_getFuncCode", Categories: "unitary", Exclusive: false, Fn: TestOall8_getFuncCode},
	{Name: "TestOall8_MarshalTo_Drop_MatchesGolden", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Drop_MatchesGolden},
	{Name: "TestOall8_MarshalTo_Create_MatchesGolden", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Create_MatchesGolden},
	{Name: "TestOall8_MarshalTo_Insert_MatchesGolden", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Insert_MatchesGolden},
	{Name: "TestOall8_MarshalTo_Delete_MatchesGolden", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Delete_MatchesGolden},
	{Name: "TestOall8_MarshalTo_Select_MatchesGolden", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Select_MatchesGolden},
	{Name: "TestOall8_MarshalTo_Fail_DDL", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_DDL},
	{Name: "TestOall8_MarshalTo_Fail_BindPtr", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_BindPtr},
	{Name: "TestOall8_MarshalTo_Fail_BindCount", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_BindCount},
	{Name: "TestOall8_MarshalTo_Fail_DefinePtr", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_DefinePtr},
	{Name: "TestOall8_MarshalTo_Fail_DefineCount", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_DefineCount},
	{Name: "TestOall8_MarshalTo_Fail_AL8I4_DataWrite", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_AL8I4_DataWrite},
	{Name: "TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Success", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Success},
	{Name: "TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Failures", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Failures},
	{Name: "TestOall8_MarshalTo_Fail_DML", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_DML},
	{Name: "TestOall8_MarshalTo_Fail_SELECT", Categories: "unitary", Exclusive: false, Fn: TestOall8_MarshalTo_Fail_SELECT},
	{Name: "TestNewO5Logon", Categories: "unitary", Exclusive: false, Fn: TestNewO5Logon},
	{Name: "TestHashMD5", Categories: "unitary", Exclusive: false, Fn: TestHashMD5},
	{Name: "TestHashSHA1", Categories: "unitary", Exclusive: false, Fn: TestHashSHA1},
	{Name: "TestHashSHA512", Categories: "unitary", Exclusive: false, Fn: TestHashSHA512},
	{Name: "TestIsAllZero", Categories: "unitary", Exclusive: false, Fn: TestIsAllZero},
	{Name: "TestRemovePKCS5Padding", Categories: "unitary", Exclusive: false, Fn: TestRemovePKCS5Padding},
	{Name: "TestApplyPKCS5Padding", Categories: "unitary", Exclusive: false, Fn: TestApplyPKCS5Padding},
	{Name: "TestApplyZeroPadding", Categories: "unitary", Exclusive: false, Fn: TestApplyZeroPadding},
	{Name: "TestBuildO5LogonKey", Categories: "unitary", Exclusive: false, Fn: TestBuildO5LogonKey},
	{Name: "TestGetDerivedKey", Categories: "unitary", Exclusive: false, Fn: TestGetDerivedKey},
	{Name: "TestDecryptAES", Categories: "unitary", Exclusive: false, Fn: TestDecryptAES},
	{Name: "TestEncryptAES", Categories: "unitary", Exclusive: false, Fn: TestEncryptAES},
	{Name: "TestGeneratePk", Categories: "unitary", Exclusive: false, Fn: TestGeneratePk},
	{Name: "TestConstructVerifierForSHA512", Categories: "unitary", Exclusive: false, Fn: TestConstructVerifierForSHA512},
	{Name: "TestConstructVerifierExceptSHA512", Categories: "unitary", Exclusive: false, Fn: TestConstructVerifierExceptSHA512},
	{Name: "TestValidateServerIdentity", Categories: "unitary", Exclusive: false, Fn: TestValidateServerIdentity},
	{Name: "TestGenerateOAuthResponse", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse},
	{Name: "TestGenerateOAuthResponse_SHA512", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse_SHA512},
	{Name: "TestGenerateOAuthResponse_Ssh1", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse_Ssh1},
	{Name: "TestGenerateOAuthResponse_Orcl7", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse_Orcl7},
	{Name: "TestGenerateOAuthResponse_Smd5", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse_Smd5},
	{Name: "TestGenerateOAuthResponse_Sh1", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse_Sh1},
	{Name: "TestGenerateOAuthResponse_ErrorCases", Categories: "unitary", Exclusive: false, Fn: TestGenerateOAuthResponse_ErrorCases},
	{Name: "TestGetO5LogonKey", Categories: "unitary", Exclusive: false, Fn: TestGetO5LogonKey},
	{Name: "TestDecryptAESWithO5logonKey", Categories: "unitary", Exclusive: false, Fn: TestDecryptAESWithO5logonKey},
	{Name: "TestEncryptAESWithO5logonKey", Categories: "unitary", Exclusive: false, Fn: TestEncryptAESWithO5logonKey},
	{Name: "TestGenerateKb", Categories: "unitary", Exclusive: false, Fn: TestGenerateKb},
	{Name: "TestGenerateSpeedKey", Categories: "unitary", Exclusive: false, Fn: TestGenerateSpeedKey},
	{Name: "TestEncryptPassword", Categories: "unitary", Exclusive: false, Fn: TestEncryptPassword},
	{Name: "TestNewOAuth_Success", Categories: "unitary", Exclusive: false, Fn: TestNewOAuth_Success},
	{Name: "TestOAuth_setSessionFields", Categories: "unitary", Exclusive: true, Fn: TestOAuth_setSessionFields},
	{Name: "TestOAuth_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestOAuth_MarshalTo_Success},
	{Name: "TestOAuth_MarshalTo_WithOSESSKEYRPA_Success", Categories: "unitary", Exclusive: false, Fn: TestOAuth_MarshalTo_WithOSESSKEYRPA_Success},
	{Name: "TestOAuthMarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestOAuthMarshalTo_Fail},
	{Name: "TestOAuth_prepareForOAUTH", Categories: "unitary", Exclusive: false, Fn: TestOAuth_prepareForOAUTH},
	{Name: "TestOAuth_initializeLogonModeForOAUTH", Categories: "unitary", Exclusive: false, Fn: TestOAuth_initializeLogonModeForOAUTH},
	{Name: "TestOAuth_setPasswordKeyValsForOAUTH", Categories: "unitary", Exclusive: false, Fn: TestOAuth_setPasswordKeyValsForOAUTH},
	{Name: "TestOAuth_setPasswordKeyValsForOAUTH_WithEncryptedKB", Categories: "unitary", Exclusive: false, Fn: TestOAuth_setPasswordKeyValsForOAUTH_WithEncryptedKB},
	{Name: "TestOAuth_setVSessionKeyValsForOAUTHIsConnectionLocal", Categories: "unitary", Exclusive: true, Fn: TestOAuth_setVSessionKeyValsForOAUTHIsConnectionLocal},
	{Name: "TestOAuth_setDriverIdentityKeyValsForOAUTH", Categories: "unitary", Exclusive: false, Fn: TestOAuth_setDriverIdentityKeyValsForOAUTH},
	{Name: "TestOAuth_setAlterSessionKeyValsForOAUTH", Categories: "unitary", Exclusive: false, Fn: TestOAuth_setAlterSessionKeyValsForOAUTH},
	{Name: "TestOAuth_validateKeySizeForOAUTH_Success", Categories: "unitary", Exclusive: false, Fn: TestOAuth_validateKeySizeForOAUTH_Success},
	{Name: "TestOAuth_validateKeySizeForOAUTH_Failure", Categories: "unitary", Exclusive: false, Fn: TestOAuth_validateKeySizeForOAUTH_Failure},
	{Name: "TestOAuth_sanitizeInputCredential", Categories: "unitary", Exclusive: false, Fn: TestOAuth_sanitizeInputCredential},
	{Name: "TestOAuth_validateO5VerifierType_Success", Categories: "unitary", Exclusive: false, Fn: TestOAuth_validateO5VerifierType_Success},
	{Name: "TestOAuth_setters", Categories: "unitary", Exclusive: false, Fn: TestOAuth_setters},
	{Name: "TestOAuthRPA_NewOAuthRPA", Categories: "unitary", Exclusive: false, Fn: TestOAuthRPA_NewOAuthRPA},
	{Name: "TestOAuthRPA_UnMarshalFrom_Golden", Categories: "unitary", Exclusive: false, Fn: TestOAuthRPA_UnMarshalFrom_Golden},
	{Name: "TestOAuthRPAUnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestOAuthRPAUnMarshalFrom_Fail},
	{Name: "TestOAuthRPA_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestOAuthRPA_UnMarshalFrom_Fail},
	{Name: "TestOAuthRPA_UnMarshalFrom_Failure", Categories: "unitary", Exclusive: false, Fn: TestOAuthRPA_UnMarshalFrom_Failure},
	{Name: "TestNewTTIoer14", Categories: "unitary", Exclusive: false, Fn: TestNewTTIoer14},
	{Name: "TestTTIoer14_Init", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_Init},
	{Name: "TestTTIoer14_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_GetMsgCode},
	{Name: "TestTTIoer14_UnmarshalAttributes_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_UnmarshalAttributes_Success},
	{Name: "TestTTIoer14_UnmarshalAttributes_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_UnmarshalAttributes_Fail},
	{Name: "TestTTIoer14_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_UnMarshalFrom_Success},
	{Name: "TestTTIoer14_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_UnMarshalFrom_Fail},
	{Name: "TestTTIoer14_UpdateChecksum", Categories: "unitary", Exclusive: false, Fn: TestTTIoer14_UpdateChecksum},
	{Name: "TestNewTTIoer", Categories: "unitary", Exclusive: false, Fn: TestNewTTIoer},
	{Name: "TestTTIoer_Init", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_Init},
	{Name: "TestTTIoer_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_GetMsgCode},
	{Name: "TestTTIoer_Getters", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_Getters},
	{Name: "TestTTIoer_GetError", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_GetError},
	{Name: "TestTTIoer_UnmarshalAttributes_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_UnmarshalAttributes_Success},
	{Name: "TestTTIoer_UnmarshalAttributes_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_UnmarshalAttributes_Fail},
	{Name: "TestTTIoer_UnMarshalFrom", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_UnMarshalFrom},
	{Name: "TestTTIoer_UpdateChecksum", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_UpdateChecksum},
	{Name: "TestTTIoer_UnmarshalWarning", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_UnmarshalWarning},
	{Name: "TestOSesskeyNew", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyNew},
	{Name: "TestOSesskeyMarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyMarshalTo_Success},
	{Name: "TestOSesskeyMarshalTo_GoldenMatch", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyMarshalTo_GoldenMatch},
	{Name: "TestOSesskeyMarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyMarshalTo_Fail},
	{Name: "TestOSesskeyRPANew", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyRPANew},
	{Name: "TestOSesskeyRPAUnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyRPAUnMarshalFrom_Success},
	{Name: "TestOSesskeyRPAUnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyRPAUnMarshalFrom_Fail},
	{Name: "TestOSesskeyRPAGetters", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyRPAGetters},
	{Name: "TestOSesskeyRPAUnMarshalFrom_Golden", Categories: "unitary", Exclusive: false, Fn: TestOSesskeyRPAUnMarshalFrom_Golden},
	{Name: "TestTTIproNew", Categories: "unitary", Exclusive: false, Fn: TestTTIproNew},
	{Name: "TestTTIproMarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIproMarshalTo_Success},
	{Name: "TestTTIproMarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIproMarshalTo_Fail},
	{Name: "TestTTIproGetters", Categories: "unitary", Exclusive: false, Fn: TestTTIproGetters},
	{Name: "TestTTIproUnmarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIproUnmarshalFrom_Success},
	{Name: "TestTTIproUnmarshalFrom_FailInvalidData", Categories: "unitary", Exclusive: false, Fn: TestTTIproUnmarshalFrom_FailInvalidData},
	{Name: "TestTTIproUnmarshalFrom_FailUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestTTIproUnmarshalFrom_FailUnmarshal},
	{Name: "TestTTIrxd_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_GetMsgCode},
	{Name: "TestTTIrxd_BvcOnFirstRow_ReturnsError", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_BvcOnFirstRow_ReturnsError},
	{Name: "TestTTIrxd_Setters", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_Setters},
	{Name: "TestTTIrxd_UnmarshalRefCursorColumn", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnmarshalRefCursorColumn},
	{Name: "TestTTIrxd_RefCursorZeroAndBVCReuse", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_RefCursorZeroAndBVCReuse},
	{Name: "TestTTIrxd_RefCursorFactoriesRequired", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_RefCursorFactoriesRequired},
	{Name: "TestTTIrxd_RefCursorDecodeErrors", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_RefCursorDecodeErrors},
	{Name: "TestTTIrxd_UnmarshalFrom_ErrorCases", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnmarshalFrom_ErrorCases},
	{Name: "TestTTIrxd_UnmarshalFrom", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnmarshalFrom},
	{Name: "TestTTIrxd_bvc_IntegrationTest", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_bvc_IntegrationTest},
	{Name: "TestTTIrxd_BvcPresentColumn_UnmarshalError", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_BvcPresentColumn_UnmarshalError},
	{Name: "TestTTIrxd_BvcCarriedNullKeepsLobContextAligned", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_BvcCarriedNullKeepsLobContextAligned},
	{Name: "TestTTIrxd_BvcCarriedClobPreservesLobContext", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_BvcCarriedClobPreservesLobContext},
	{Name: "TestTTIrxd_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_MarshalTo_Success},
	{Name: "TestTTIrxd_MarshalTo_FailOnNullIndicator", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_MarshalTo_FailOnNullIndicator},
	{Name: "TestTTIrxd_MarshalTo_FailOnCLRDataWrite", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_MarshalTo_FailOnCLRDataWrite},
	{Name: "TestTTIrxhConstructor", Categories: "unitary", Exclusive: false, Fn: TestTTIrxhConstructor},
	{Name: "TestTTIrxhUnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIrxhUnMarshalFrom_Success},
	{Name: "TestTTIrxhUnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIrxhUnMarshalFrom_Fail},
	{Name: "TestTTISPF_New", Categories: "unitary", Exclusive: false, Fn: TestTTISPF_New},
	{Name: "TestTTISPF_Unmarshal_Success", Categories: "unitary", Exclusive: false, Fn: TestTTISPF_Unmarshal_Success},
	{Name: "TestTTISPF_Unmarshal_Fail_TruncatedPayloads", Categories: "unitary", Exclusive: false, Fn: TestTTISPF_Unmarshal_Fail_TruncatedPayloads},
	{Name: "TestTTISPF_Unmarshal_Fail_FaultyBuffer", Categories: "unitary", Exclusive: false, Fn: TestTTISPF_Unmarshal_Fail_FaultyBuffer},
	{Name: "TestGetKeyValueFromKeyword_Timezone_WithRegionID_NameUsed", Categories: "unitary", Exclusive: false, Fn: TestGetKeyValueFromKeyword_Timezone_WithRegionID_NameUsed},
	{Name: "TestGetKeyValueFromKeyword_Timezone_WithRegionID_FallbackGMT", Categories: "unitary", Exclusive: false, Fn: TestGetKeyValueFromKeyword_Timezone_WithRegionID_FallbackGMT},
	{Name: "TestGetKeyValueFromKeyword_Nls_TextValueBranch", Categories: "unitary", Exclusive: false, Fn: TestGetKeyValueFromKeyword_Nls_TextValueBranch},
	{Name: "TestGetKeyValueFromKeyword_PdbElasticPoolLdr_True", Categories: "unitary", Exclusive: false, Fn: TestGetKeyValueFromKeyword_PdbElasticPoolLdr_True},
	{Name: "TestGetKeyValueFromKeyword_PdbAppRoot_True", Categories: "unitary", Exclusive: false, Fn: TestGetKeyValueFromKeyword_PdbAppRoot_True},
	{Name: "Test_newTTISTA", Categories: "unitary", Exclusive: false, Fn: Test_newTTISTA},
	{Name: "Test_newTTISTAWithEndOfCallStatusSupport", Categories: "unitary", Exclusive: false, Fn: Test_newTTISTAWithEndOfCallStatusSupport},
	{Name: "Test_ttiSTA_UnMarshalFrom_WithoutSupport", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_UnMarshalFrom_WithoutSupport},
	{Name: "Test_ttiSTA_UnMarshalFrom_WithSupport", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_UnMarshalFrom_WithSupport},
	{Name: "Test_ttiSTA_UnMarshalFrom_WithSupport_DropFlag", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_UnMarshalFrom_WithSupport_DropFlag},
	{Name: "Test_ttiSTA_UnMarshalFrom_ErrorInUB2", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_UnMarshalFrom_ErrorInUB2},
	{Name: "Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInEOCS", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInEOCS},
	{Name: "Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInElapsedTime", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInElapsedTime},
	{Name: "TestTTIuds17_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIuds17_UnMarshalFrom_Success},
	{Name: "TestTTIuds17_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIuds17_UnMarshalFrom_Fail},
	{Name: "TestTTIuds20_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIuds20_UnMarshalFrom_Success},
	{Name: "TestTTIuds20_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIuds20_UnMarshalFrom_Fail},
	{Name: "TestTTIuds24_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIuds24_UnMarshalFrom_Success},
	{Name: "TestTTIuds24_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIuds24_UnMarshalFrom_Fail},
	{Name: "TestTTIuds_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIuds_UnMarshalFrom_Success},
	{Name: "TestTTIuds_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIuds_UnMarshalFrom_Fail},
	{Name: "TestTTIuds_GetColumnName", Categories: "unitary", Exclusive: false, Fn: TestTTIuds_GetColumnName},
	{Name: "TestTTIuds_GetUdsArrayAndColCount", Categories: "unitary", Exclusive: false, Fn: TestTTIuds_GetUdsArrayAndColCount},
	{Name: "TestTypeRep_NewTypeRep", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_NewTypeRep},
	{Name: "TestTypeRep_MarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_MarshalTo_Fail},
	{Name: "TestTypeRep_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_UnMarshalFrom_Success},
	{Name: "TestTypeRep_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_UnMarshalFrom_Fail},
	{Name: "TestTypeRep_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_MarshalTo_Success},
	{Name: "TestTypeRep_AddTypeRepToTable_Resize", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_AddTypeRepToTable_Resize},
	{Name: "TestTypeRep_SetRepAndGetRep", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_SetRepAndGetRep},
	{Name: "TestTypeRep_SetFlagsAndGetFlags", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_SetFlagsAndGetFlags},
	{Name: "TestTypeRep_Setters_Getters", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_Setters_Getters},
	{Name: "TestGetIDFromZone_Found", Categories: "unitary", Exclusive: false, Fn: TestGetIDFromZone_Found},
	{Name: "TestGetIDFromZone_NotFound", Categories: "unitary", Exclusive: false, Fn: TestGetIDFromZone_NotFound},
	{Name: "TestGetZoneFromID_Found", Categories: "unitary", Exclusive: false, Fn: TestGetZoneFromID_Found},
	{Name: "TestGetZoneFromID_NotFound", Categories: "unitary", Exclusive: false, Fn: TestGetZoneFromID_NotFound},
	{Name: "TestZoneMaps_Sanity", Categories: "unitary", Exclusive: false, Fn: TestZoneMaps_Sanity},
	{Name: "TestCodecFactory_getEncoder", Categories: "unitary", Exclusive: false, Fn: TestCodecFactory_getEncoder},
	{Name: "TestCodecFactory_getDecoder", Categories: "unitary", Exclusive: false, Fn: TestCodecFactory_getDecoder},
	{Name: "TestCodecFactory_RegisterEncoderGeneric", Categories: "unitary", Exclusive: false, Fn: TestCodecFactory_RegisterEncoderGeneric},
	{Name: "TestTTIShelf_NewShelf", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_NewShelf},
	{Name: "TestTTIShelf_RegisterCodecFactoryAndGetter", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_RegisterCodecFactoryAndGetter},

	{Name: "TestPrepareBindsAndOAC_Normalize_Supported", Categories: "unitary", Exclusive: false, Fn: TestPrepareBindsAndOAC_Normalize_Supported},
	{Name: "TestPrepareBindsAndOAC_Normalize_Errors", Categories: "unitary", Exclusive: false, Fn: TestPrepareBindsAndOAC_Normalize_Errors},
	{Name: "TestPrepareBindsAndOAC_Encode_Supported", Categories: "unitary", Exclusive: false, Fn: TestPrepareBindsAndOAC_Encode_Supported},
	{Name: "TestPrepareBindsAndOAC_Encode_Errors", Categories: "unitary", Exclusive: false, Fn: TestPrepareBindsAndOAC_Encode_Errors},
	{Name: "TestPushBindRowsIfAny_EncodeBindValues_Error", Categories: "unitary", Exclusive: false, Fn: TestPushBindRowsIfAny_EncodeBindValues_Error},
	{Name: "TestPushBindRowsIfAny_GetMessage_RXD_Error", Categories: "unitary", Exclusive: false, Fn: TestPushBindRowsIfAny_GetMessage_RXD_Error},
	{Name: "TestPushBindRowsIfAny_Push_RXD_Error", Categories: "unitary", Exclusive: false, Fn: TestPushBindRowsIfAny_Push_RXD_Error},
	{Name: "TestPrepareBindsAndOAC_OAC_Supported", Categories: "unitary", Exclusive: false, Fn: TestPrepareBindsAndOAC_OAC_Supported},
	{Name: "TestPrepareBindsAndOAC_OAC_Errors", Categories: "unitary", Exclusive: false, Fn: TestPrepareBindsAndOAC_OAC_Errors},
	{Name: "TestStatementExecutor_DML_Insert_Prepared_MarshalAndExec", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_DML_Insert_Prepared_MarshalAndExec},
	{Name: "TestStatementExecutor_Select_Prepared_MarshalAndQuery", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_Prepared_MarshalAndQuery},

	{Name: "TestOexfen_New_Getters", Categories: "unitary", Exclusive: false, Fn: TestOexfen_New_Getters},
	{Name: "TestOexfen_MarshalTo_MatchesGolden", Categories: "unitary", Exclusive: false, Fn: TestOexfen_MarshalTo_MatchesGolden},
	{Name: "TestOexfen_MarshalTo_Exeflg_NoCommit_And_Commit", Categories: "unitary", Exclusive: false, Fn: TestOexfen_MarshalTo_Exeflg_NoCommit_And_Commit},
	{Name: "TestOexfen_MarshalTo_Failures", Categories: "unitary", Exclusive: false, Fn: TestOexfen_MarshalTo_Failures},

	{Name: "TestLobExecutor_GetChunkSize", Categories: "unitary", Exclusive: false, Fn: TestLobExecutor_GetChunkSize},
	{Name: "TestClobExecutor_GetChunkSizeErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_GetChunkSizeErrors},
	{Name: "TestClobExecutor_CreateTemporaryLob", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_CreateTemporaryLob},
	{Name: "TestClobExecutor_CreateTemporaryLobErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_CreateTemporaryLobErrors},
	{Name: "TestClobExecutor_Write", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_Write},
	{Name: "TestClobExecutor_WriteErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_WriteErrors},
	{Name: "TestClobExecutor_Read", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_Read},
	{Name: "TestClobExecutor_ReadErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_ReadErrors},
	{Name: "TestClobExecutor_IsOpen", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_IsOpen},
	{Name: "TestClobExecutor_IsOpenErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_IsOpenErrors},
	{Name: "TestClobExecutor_GetLength", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_GetLength},
	{Name: "TestClobExecutor_GetLengthErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_GetLengthErrors},
	{Name: "TestClobExecutor_Trim", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_Trim},
	{Name: "TestClobExecutor_TrimErrors", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_TrimErrors},
	{Name: "TestLobExecutor_ConsumeLobResponses_DelayedOER", Categories: "unitary", Exclusive: false, Fn: TestLobExecutor_ConsumeLobResponses_DelayedOER},
	{Name: "TestLobExecutor_ConsumeLobResponses_OERTermination", Categories: "unitary", Exclusive: false, Fn: TestLobExecutor_ConsumeLobResponses_OERTermination},
	{Name: "TestLobExecutor_ConsumeLobResponses_PullError", Categories: "unitary", Exclusive: false, Fn: TestLobExecutor_ConsumeLobResponses_PullError},
	{Name: "TestClobExecutor_ReadNCLOB", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_ReadNCLOB},
	{Name: "TestConnection_FaultyOnDrain", Categories: "unitary", Exclusive: false, Fn: TestConnection_FaultyOnDrain},
	{Name: "TestConnection_FaultyOnDrainInStatement", Categories: "unitary", Exclusive: false, Fn: TestConnection_FaultyOnDrainInStatement},
	{Name: "TestClobExecutor_WriteNCLOB", Categories: "unitary", Exclusive: false, Fn: TestClobExecutor_WriteNCLOB},

	{Name: "TestConnection_InvalidateOnOEROrSTA", Categories: "unitary", Exclusive: false, Fn: TestConnection_InvalidateOnOEROrSTA},
	{Name: "TestTransactionCommitSuccess", Categories: "unitary", Exclusive: false, Fn: TestTransactionCommitSuccess},
	{Name: "TestTransactionRollbackSuccess", Categories: "unitary", Exclusive: false, Fn: TestTransactionRollbackSuccess},
	{Name: "TestCallBeginTxTwice", Categories: "unitary", Exclusive: false, Fn: TestCallBeginTxTwice},
	{Name: "TestConnectionBeginUsesDefaultIsolationLevel", Categories: "unitary", Exclusive: false, Fn: TestConnectionBeginUsesDefaultIsolationLevel},
	{Name: "TestConnectionBeginTxRejectsUnsupportedIsolationLevel", Categories: "unitary", Exclusive: false, Fn: TestConnectionBeginTxRejectsUnsupportedIsolationLevel},
	{Name: "TestConnectionBeginTxUnregistersAfterSetupErrors", Categories: "unitary", Exclusive: false, Fn: TestConnectionBeginTxUnregistersAfterSetupErrors},
	{Name: "TestTransactionOperationErrors", Categories: "unitary", Exclusive: false, Fn: TestTransactionOperationErrors},
	{Name: "TestTransactionOperationRejectsStaleMessages", Categories: "unitary", Exclusive: false, Fn: TestTransactionOperationRejectsStaleMessages},

	{Name: "TestStatementExecutorExec_HandleRXDRow_UsesScannerDestination", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutorExec_HandleRXDRow_UsesScannerDestination},
	{Name: "TestStatementExecutorExec_HandleRXDRow_PropagatesScannerError", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutorExec_HandleRXDRow_PropagatesScannerError},
	{Name: "TestNormalizeBindValue_SQLNullTypes", Categories: "unitary", Exclusive: false, Fn: TestNormalizeBindValue_SQLNullTypes},
	{Name: "TestUnmarshalCLRColumnDataRejectsInvalidLongChunkLengths", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRColumnDataRejectsInvalidLongChunkLengths},
	{Name: "TestNeedToSendOACs_CountChanged", Categories: "unitary", Exclusive: false, Fn: TestNeedToSendOACs_CountChanged},

	{Name: "TestAuthRPARejectsOversizedKeyValueListAllocations", Categories: "unitary", Exclusive: false, Fn: TestAuthRPARejectsOversizedKeyValueListAllocations},
	{Name: "TestCodecFactory_getBindOac", Categories: "unitary", Exclusive: false, Fn: TestCodecFactory_getBindOac},
	{Name: "TestCodecFactory_getDefineOac", Categories: "unitary", Exclusive: false, Fn: TestCodecFactory_getDefineOac},
	{Name: "TestConnectionResetter_Reset", Categories: "unitary", Exclusive: false, Fn: TestConnectionResetter_Reset},
	{Name: "TestConnection_ExecContext_LocalizesError", Categories: "unitary", Exclusive: false, Fn: TestConnection_ExecContext_LocalizesError},
	{Name: "TestConnection_LocalizationStaysBoundToEachShelf", Categories: "unitary", Exclusive: false, Fn: TestConnection_LocalizationStaysBoundToEachShelf},
	{Name: "TestConnection_ParseTimeZoneRejectsMalformedValues", Categories: "unitary", Exclusive: false, Fn: TestConnection_ParseTimeZoneRejectsMalformedValues},
	{Name: "TestConnection_QueryContext_LocalizesError", Categories: "unitary", Exclusive: false, Fn: TestConnection_QueryContext_LocalizesError},
	{Name: "TestConnection_cancelCurrentExecution", Categories: "unitary", Exclusive: false, Fn: TestConnection_cancelCurrentExecution},
	{Name: "TestEncryptPasswordBufferTooSmall", Categories: "unitary", Exclusive: false, Fn: TestEncryptPasswordBufferTooSmall},
	{Name: "TestGetConnectionMissingLocalizationService", Categories: "unitary", Exclusive: false, Fn: TestGetConnectionMissingLocalizationService},
	{Name: "TestGetMaxLengthForOac_NoPreviousOACs", Categories: "unitary", Exclusive: false, Fn: TestGetMaxLengthForOac_NoPreviousOACs},
	{Name: "TestGetMaxLengthForOac_PreservesPreviousLarger", Categories: "unitary", Exclusive: false, Fn: TestGetMaxLengthForOac_PreservesPreviousLarger},
	{Name: "TestGetMaxLengthForOac_UsesCurrentIfLarger", Categories: "unitary", Exclusive: false, Fn: TestGetMaxLengthForOac_UsesCurrentIfLarger},
	{Name: "TestHandleRXDRow_AssignsDecodedValue", Categories: "unitary", Exclusive: false, Fn: TestHandleRXDRow_AssignsDecodedValue},
	{Name: "TestHandleRXDRow_MoreDestsThanReturnedValues", Categories: "unitary", Exclusive: false, Fn: TestHandleRXDRow_MoreDestsThanReturnedValues},
	{Name: "TestHandleRXDRow_NilDestinationSkipped", Categories: "unitary", Exclusive: false, Fn: TestHandleRXDRow_NilDestinationSkipped},
	{Name: "TestHandleRXDRow_NilWireValue_SkipsAssignment", Categories: "unitary", Exclusive: false, Fn: TestHandleRXDRow_NilWireValue_SkipsAssignment},
	{Name: "TestHandleRXDRow_RawBytes_AssignedToByteSlice", Categories: "unitary", Exclusive: false, Fn: TestHandleRXDRow_RawBytes_AssignedToByteSlice},
	{Name: "TestInitExecRunner_InOutBinds_Counted", Categories: "unitary", Exclusive: false, Fn: TestInitExecRunner_InOutBinds_Counted},
	{Name: "TestInitExecRunner_NoOutBinds_FlagsNotSet", Categories: "unitary", Exclusive: false, Fn: TestInitExecRunner_NoOutBinds_FlagsNotSet},
	{Name: "TestInitExecRunner_Reset", Categories: "unitary", Exclusive: false, Fn: TestInitExecRunner_Reset},
	{Name: "TestInitExecRunner_WithOutBinds_FlagsSet", Categories: "unitary", Exclusive: false, Fn: TestInitExecRunner_WithOutBinds_FlagsSet},
	{Name: "TestKeywordValueArrayRejectsOversizedDynamicValue", Categories: "unitary", Exclusive: false, Fn: TestKeywordValueArrayRejectsOversizedDynamicValue},
	{Name: "TestKeywordValueArrayRejectsOversizedPairCount", Categories: "unitary", Exclusive: false, Fn: TestKeywordValueArrayRejectsOversizedPairCount},
	{Name: "TestMessageStreamer_PullReturnsErrorForNonUnmarshallableFunction", Categories: "unitary", Exclusive: false, Fn: TestMessageStreamer_PullReturnsErrorForNonUnmarshallableFunction},
	{Name: "TestNeedToSendOACs_LengthIncreased", Categories: "unitary", Exclusive: false, Fn: TestNeedToSendOACs_LengthIncreased},
	{Name: "TestNeedToSendOACs_NoPreviousOACs", Categories: "unitary", Exclusive: false, Fn: TestNeedToSendOACs_NoPreviousOACs},
	{Name: "TestNeedToSendOACs_SameOAC_ReturnsFalse", Categories: "unitary", Exclusive: false, Fn: TestNeedToSendOACs_SameOAC_ReturnsFalse},
	{Name: "TestNeedToSendOACs_TypeChanged", Categories: "unitary", Exclusive: false, Fn: TestNeedToSendOACs_TypeChanged},
	{Name: "TestNewConnectionReturnsServerTimezoneError", Categories: "unitary", Exclusive: false, Fn: TestNewConnectionReturnsServerTimezoneError},
	{Name: "TestPasswordAuthenticatorValidatePasswordLength", Categories: "unitary", Exclusive: false, Fn: TestPasswordAuthenticatorValidatePasswordLength},
	{Name: "TestStatementCancellationCleanupReleasesStartedAfterFunc", Categories: "unitary", Exclusive: false, Fn: TestStatementCancellationCleanupReleasesStartedAfterFunc},
	{Name: "TestStatementExecContextTransactionCancellationBeforeSetup", Categories: "unitary", Exclusive: false, Fn: TestStatementExecContextTransactionCancellationBeforeSetup},
	{Name: "TestStatementExecutor_Select_SuccessOERWithoutDCB", Categories: "unitary", Exclusive: false, Fn: TestStatementExecutor_Select_SuccessOERWithoutDCB},
	{Name: "TestStatementHandleContextCancelledRunsBreakReset", Categories: "unitary", Exclusive: false, Fn: TestStatementHandleContextCancelledRunsBreakReset},
	{Name: "TestStatementQueryContextLocalization", Categories: "unitary", Exclusive: false, Fn: TestStatementQueryContextLocalization},
	{Name: "TestTTCRowsColumnTypeScanType", Categories: "unitary", Exclusive: false, Fn: TestTTCRowsColumnTypeScanType},
	{Name: "TestTTCRowsImplementsColumnTypeInterfaces", Categories: "unitary", Exclusive: false, Fn: TestTTCRowsImplementsColumnTypeInterfaces},
	{Name: "TestTTILobRpa_SetDefinition_NilDefinition", Categories: "unitary", Exclusive: false, Fn: TestTTILobRpa_SetDefinition_NilDefinition},
	{Name: "TestTTILobRpa_UnMarshalFrom_Failure", Categories: "unitary", Exclusive: false, Fn: TestTTILobRpa_UnMarshalFrom_Failure},
	{Name: "TestTTILobRpa_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTILobRpa_UnMarshalFrom_Success},
	{Name: "TestTTILobd_MarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTILobd_MarshalTo_Fail},
	{Name: "TestTTILobd_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTTILobd_MarshalTo_Success},
	{Name: "TestTTILobd_UnMarshalFrom_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTILobd_UnMarshalFrom_Fail},
	{Name: "TestTTILobd_UnMarshalFrom_Success", Categories: "unitary", Exclusive: false, Fn: TestTTILobd_UnMarshalFrom_Success},
	{Name: "TestTTIShelf_LocalizedStatementExecError", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_LocalizedStatementExecError},
	{Name: "TestTTIShelf_StatementDrain", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_StatementDrain},
	{Name: "TestTTIShelf_ValidateConnection", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_ValidateConnection},
	{Name: "TestNewMessageStreamerRegistersConnectionValidator", Categories: "unitary", Exclusive: false, Fn: TestNewMessageStreamerRegistersConnectionValidator},
	{Name: "TestTTIShelf_ValidateConnectionStopsAtFirstInvalidValidator", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_ValidateConnectionStopsAtFirstInvalidValidator},
	{Name: "TestTTIShelf_RegisterProviderRegistry", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_RegisterProviderRegistry},
	{Name: "TestTTIShelf_RegisterProviderRegistry_ReplacesExistingRegistry", Categories: "unitary", Exclusive: false, Fn: TestTTIShelf_RegisterProviderRegistry_ReplacesExistingRegistry},
	{Name: "TestTTIlob_GetFuncCode", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_GetFuncCode},
	{Name: "TestTTIlob_GetMsgCode", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_GetMsgCode},
	{Name: "TestTTIlob_MarshalTo_Fail", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_MarshalTo_Fail},
	{Name: "TestTTIlob_MarshalTo_Success", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_MarshalTo_Success},
	{Name: "TestTTIlob_MarshalTo_WithoutDefinition", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_MarshalTo_WithoutDefinition},
	{Name: "TestTTIlob_New", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_New},
	{Name: "TestTTIlob_SetDefinition", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_SetDefinition},
	{Name: "TestTTIlob_SetDefinition_NilDefinition", Categories: "unitary", Exclusive: false, Fn: TestTTIlob_SetDefinition_NilDefinition},
	{Name: "TestTTIoer_getConnectionShouldBeDropped", Categories: "unitary", Exclusive: false, Fn: TestTTIoer_getConnectionShouldBeDropped},
	{Name: "TestTTIrxd_MarshalTo_LargeCLR", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_MarshalTo_LargeCLR},
	{Name: "TestTTIrxd_ProcessDMLPlSqlIndicator_ReadError", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_ProcessDMLPlSqlIndicator_ReadError},
	{Name: "TestTTIrxd_SetNumberofReturningArgs_SwitchesMode", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_SetNumberofReturningArgs_SwitchesMode},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_DataReadError", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_DataReadError},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_IndicatorReadError", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_IndicatorReadError},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_MultipleRows", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_MultipleRows},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_NullValue", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_NullValue},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_RowCountReadError", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_RowCountReadError},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_SinglePosition_SingleRow", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_SinglePosition_SingleRow},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_ThreePositionsMixed", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_ThreePositionsMixed},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_TwoPositions_SingleRowEach", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_TwoPositions_SingleRowEach},
	{Name: "TestTTIrxd_UnMarshalFrom_Returning_ZeroRowsForPosition", Categories: "unitary", Exclusive: false, Fn: TestTTIrxd_UnMarshalFrom_Returning_ZeroRowsForPosition},
	{Name: "TestTTIwrnUnMarshalFrom", Categories: "unitary", Exclusive: false, Fn: TestTTIwrnUnMarshalFrom},
	{Name: "TestTTIwrnUnMarshalFromRejectsTruncatedMessage", Categories: "unitary", Exclusive: false, Fn: TestTTIwrnUnMarshalFromRejectsTruncatedMessage},
	{Name: "TestUnmarshalCLRColumnData", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRColumnData},
	{Name: "TestUnmarshalCLRColumnDataRejectsAggregateLengthAboveMaximum", Categories: "unitary", Exclusive: false, Fn: TestUnmarshalCLRColumnDataRejectsAggregateLengthAboveMaximum},
	{Name: "Test_defaultNumericValue", Categories: "unitary", Exclusive: false, Fn: Test_defaultNumericValue},
	{Name: "Test_defaultValueForNull", Categories: "unitary", Exclusive: false, Fn: Test_defaultValueForNull},
	{Name: "Test_defaultValueForNullUnknown", Categories: "unitary", Exclusive: false, Fn: Test_defaultValueForNullUnknown},
	{Name: "Test_ttcRows_handleNullDefaulting", Categories: "unitary", Exclusive: false, Fn: Test_ttcRows_handleNullDefaulting},
	{Name: "Test_ttcRows_handleNullStrict", Categories: "unitary", Exclusive: false, Fn: Test_ttcRows_handleNullStrict},
	{Name: "Test_ttiSTA_getConnectionShouldBeDropped", Categories: "unitary", Exclusive: false, Fn: Test_ttiSTA_getConnectionShouldBeDropped},

	{Name: "TestTypeRep_UnMarshalFrom_TooManyTypeRepresentations", Categories: "unitary", Exclusive: false, Fn: TestTypeRep_UnMarshalFrom_TooManyTypeRepresentations},
}

// ArrayBasedDataBuffer is an implementation of DataBuffer for testing purposes.
type ArrayBasedDataBuffer struct {
	bytes                []byte
	currentWritePosition int
	currentReadPosition  int
	hasBeenFlushed       bool
	returnFlushError     bool
}

// NewArrayDataBuffer creates a new ArrayBasedDataBuffer with the specified size.
func NewArrayDataBuffer(size int) *ArrayBasedDataBuffer {
	arrayBasedDataBuffer := ArrayBasedDataBuffer{
		bytes:                make([]byte, size),
		currentWritePosition: 0,
		currentReadPosition:  0,
		hasBeenFlushed:       false,
		returnFlushError:     false,
	}
	return &arrayBasedDataBuffer
}

func (b *ArrayBasedDataBuffer) Equals(compared *ArrayBasedDataBuffer) bool {
	// enough for now
	return ByteArrayCompare(b.bytes, compared.bytes) == nil
	//for idx, _b := range b.bytes[:b.currentWritePosition] {
	//	if compared.bytes[idx] != _b {
	//		return false
	//	}
	//}
	//return true
}

// WriteByteWithContext writes one byte.
func (array *ArrayBasedDataBuffer) WriteByteWithContext(ctx context.Context, value byte) error {
	if len(array.bytes) <= array.currentWritePosition {
		return fmt.Errorf("Buffer overflow")
	}
	array.bytes[array.currentWritePosition] = value
	array.currentWritePosition++
	return nil
}

// WriteBytesWithContext writes the entire content of a byte array.
func (array *ArrayBasedDataBuffer) WriteBytesWithContext(ctx context.Context, source []byte) error {
	for i := 0; i < len(source); i++ {
		err := array.WriteByteWithContext(ctx, source[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// Flush flushes the buffer.
func (array *ArrayBasedDataBuffer) Flush(ctx context.Context) error {
	array.hasBeenFlushed = true
	if array.returnFlushError {
		return fmt.Errorf("Error flushing")
	}
	return nil
}

// ReadByteWithContext reads one byte.
func (array *ArrayBasedDataBuffer) ReadByteWithContext(ctx context.Context) (byte, error) {
	if array.currentReadPosition >= array.currentWritePosition {
		return 0, io.EOF
	}
	b := array.bytes[array.currentReadPosition]
	array.currentReadPosition++
	return b, nil
}

// isDataTobeFlushed checks if there is data to be flushed.
func (array *ArrayBasedDataBuffer) isDataTobeFlushed() bool {
	return array.currentWritePosition > array.currentReadPosition
}

// ReadBytesWithContext reads a byte array of the specified length.
func (array *ArrayBasedDataBuffer) ReadBytesWithContext(ctx context.Context, length int32) (*[]byte, error) {
	if array.currentReadPosition+int(length) > array.currentWritePosition {
		return nil, io.EOF
	}
	buf := array.bytes[array.currentReadPosition : array.currentReadPosition+int(length)]
	array.currentReadPosition += int(length)
	res := make([]byte, length)
	copy(res, buf)
	return &res, nil
}

// FaultyArrayBasedDataBuffer is an ArrayBasedDataBuffer that can simulate read/write errors for testing.
type FaultyArrayBasedDataBuffer struct {
	*ArrayBasedDataBuffer

	// Failure injection by Nth call
	WriteByteCallCount   int
	FailOnWriteByteCall  int // fail on Nth call
	WriteBytesCallCount  int
	FailOnWriteBytesCall int // fail on Nth call
	ReadByteCallCount    int
	FailOnReadByteCall   int // fail on Nth call
	ReadBytesCallCount   int
	FailOnReadBytesCall  int // fail on Nth call
}

// WriteByteWithContext creates a new FaultyArrayBasedDataBuffer with the specified size.
func (f *FaultyArrayBasedDataBuffer) WriteByteWithContext(ctx context.Context, b byte) error {
	f.WriteByteCallCount++
	if f.FailOnWriteByteCall > 0 && f.WriteByteCallCount == f.FailOnWriteByteCall {
		return errors.New("simulated write error (WriteByte)")
	}
	return f.ArrayBasedDataBuffer.WriteByteWithContext(ctx, b)
}

// WriteBytesWithContext writes bytes with context, simulating errors if configured.
func (f *FaultyArrayBasedDataBuffer) WriteBytesWithContext(ctx context.Context, p []byte) error {
	f.WriteBytesCallCount++
	if f.FailOnWriteBytesCall > 0 && f.WriteBytesCallCount == f.FailOnWriteBytesCall {
		return errors.New("simulated write error (WriteBytes)")
	}
	return f.ArrayBasedDataBuffer.WriteBytesWithContext(ctx, p)
}

// Flush flushes the buffer.
func (f *FaultyArrayBasedDataBuffer) Flush(ctx context.Context) error {
	return nil
}

// ReadBytesWithContext reads bytes with context, simulating errors if configured.
func (f *FaultyArrayBasedDataBuffer) ReadBytesWithContext(ctx context.Context, length int32) (*[]byte, error) {
	f.ReadBytesCallCount++
	if f.FailOnReadBytesCall > 0 && f.ReadBytesCallCount == f.FailOnReadBytesCall {
		return nil, errors.New("simulated read error (ReadBytes)")
	}
	return f.ArrayBasedDataBuffer.ReadBytesWithContext(ctx, length)
}

// ReadByteWithContext reads a byte with context, simulating errors if configured.
func (f *FaultyArrayBasedDataBuffer) ReadByteWithContext(ctx context.Context) (byte, error) {
	f.ReadByteCallCount++
	if f.FailOnReadByteCall > 0 && f.ReadByteCallCount == f.FailOnReadByteCall {
		return 0, errors.New("simulated read error (ReadByte)")
	}
	return f.ArrayBasedDataBuffer.ReadByteWithContext(ctx)
}

// NewMarshalEngineTest creates a new MarshalEngine for testing purposes.
func NewMarshalEngineTest(byteOrder common.ByteOrder, typ byte, rep byte, bufSize int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	dataBuffer := NewArrayDataBuffer(bufSize)
	typeRep := newTypeRep()
	typeRep.setRep(typ, rep)
	engine := NewMarshalEngine(dataBuffer, byteOrder, [5]byte{rep, rep, rep, rep, rep})
	return dataBuffer, engine
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" && err != nil {
		t.Errorf("expected no error, got: %v", err)
		return
	}
	if want != "" {
		if err == nil {
			t.Errorf("expected error to match pattern %v, got nil", want)
			return
		}
		matched, matchErr := regexp.MatchString(want, err.Error())
		if matchErr != nil {
			t.Errorf("failed to compile regex pattern: %v", matchErr)
		} else if !matched {
			t.Errorf("expected error to match pattern %v, got %v", want, err)
		}
	}
}

func ByteArrayCompare(a []byte, b []byte) error {
	var err error = nil
	if len(a) != len(b) {
		err = errors.New(fmt.Sprintf("array size mismatch [%d] </> [%d]", len(a), len(b)))
	}
	var stop = int(math.Min(float64(len(a)), float64(len(b))))
	for i := 0; i < stop; i++ {
		if a[i] != b[i] {
			err = errors.Join(err, errors.New(fmt.Sprintf("array content mismatch at idx [%d] : [%d] </> [%d]", i, a[i], b[i])))
			break
		}
	}
	return err
}

// FailOn type is used for faulty data buffer creation in tests
type FailOn int

const (
	failOnReadByte   FailOn = 1
	failOnReadBytes  FailOn = 2
	failOnWriteByte  FailOn = 3
	failOnWriteBytes FailOn = 4
)

func createMarshaller(buf []byte, failOn FailOn, callCount int) common.Marshaller {
	rep := [5]byte{Native, Universal, Universal, Universal, Universal}
	if failOn > 0 {
		faulty := &FaultyArrayBasedDataBuffer{}

		if failOn == failOnReadByte || failOn == failOnReadBytes {
			faulty = &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: &ArrayBasedDataBuffer{
					bytes:                buf,
					currentWritePosition: len(buf),
					currentReadPosition:  0,
				},
			}
		}

		if failOn == failOnWriteByte || failOn == failOnWriteBytes {
			faulty = &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: &ArrayBasedDataBuffer{
					bytes:                buf,
					currentWritePosition: 0,
					currentReadPosition:  0,
				},
			}
		}

		switch failOn {
		case failOnReadByte:
			faulty.FailOnReadByteCall = callCount
		case failOnReadBytes:
			faulty.FailOnReadBytesCall = callCount
		case failOnWriteByte:
			faulty.FailOnWriteByteCall = callCount
		case failOnWriteBytes:
			faulty.FailOnWriteBytesCall = callCount
		}
		return NewMarshalEngine(faulty, common.BIG_ENDIAN, rep)
	}
	tdb := NewTestDataBuffer()
	tdb.WriteBytesWithContext(context.Background(), buf)
	mar := NewMarshalEngine(tdb, common.BIG_ENDIAN, rep)
	return mar
}

// *** Factory ***

// mockFactory implements common.Factory for testing
type mockFactory struct {
	getMsgForFuncCalled bool
	msgType             common.MessageType
	funcType            common.FunctionType
	returnMsg           common.Message[common.MessageType]
	returnErr           error
}

func (m *mockFactory) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	return m.returnMsg, m.returnErr
}

func (m *mockFactory) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	m.getMsgForFuncCalled = true
	m.msgType = msgType
	m.funcType = funcType
	return m.returnMsg, m.returnErr
}

// mockFactory implements common.Factory for testing
type mockFactoryWithList struct {
	returnMsg  []common.Message[common.MessageType]
	currentMsg int
}

func (m *mockFactoryWithList) GetMessage(msgType common.MessageType) (common.Message[common.MessageType], error) {
	msg := m.returnMsg[m.currentMsg]
	m.currentMsg++
	return msg, nil
}

func (m *mockFactoryWithList) GetMessageForFunction(msgType common.MessageType, funcType common.FunctionType) (common.Message[common.MessageType], error) {
	msg := m.returnMsg[m.currentMsg]
	m.currentMsg++
	return msg, nil
}

// TestDataBuffer is a test implementation of DataBuffer for use in unit tests across packages.
type TestDataBuffer struct {
	buf                  bytes.Buffer
	OnFlush              func(*TestDataBuffer)
	OnWriteDefaults      func(*TestDataBuffer)
	unmarshalFromCounter int
}

// ResetBuf resets the internal buffer (for use in tests)
func (t *TestDataBuffer) ResetBuf() {
	t.buf.Reset()
}

// WriteBuf writes to the internal buffer (for use in tests)
func (t *TestDataBuffer) WriteBuf(b []byte) (int, error) {
	return t.buf.Write(b)
}

// NewTestDataBuffer creates a new test buffer
func NewTestDataBuffer() *TestDataBuffer {
	return &TestDataBuffer{}
}

// WriteByteWithContext write one byte
func (t *TestDataBuffer) WriteByteWithContext(ctx context.Context, b byte) error {
	return t.buf.WriteByte(b)
}

// WriteBytesWithContext write bytes
func (t *TestDataBuffer) WriteBytesWithContext(ctx context.Context, b []byte) error {
	_, err := t.buf.Write(b)
	return err
}

// Flush the test buffer
func (t *TestDataBuffer) Flush(ctx context.Context) error {
	fmt.Printf("Flush called ... \n")
	if t.OnFlush != nil {
		t.OnFlush(t)
	}
	if t.OnWriteDefaults != nil {
		t.OnWriteDefaults(t)
	}
	return nil
}

// ReadByteWithContext reads one byte
func (t *TestDataBuffer) ReadByteWithContext(ctx context.Context) (byte, error) {
	b, err := t.buf.ReadByte()
	return b, err
}

// ReadBytesWithContext reads bytes
func (t *TestDataBuffer) ReadBytesWithContext(ctx context.Context, n int32) (*[]byte, error) {
	b := make([]byte, int(n))
	_, err := io.ReadFull(&t.buf, b)
	if err != nil {
		if err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return nil, err
	}
	return &b, nil
}

// *** Streamer ***

// mockStreamer implements common.Streamer[common.MessageType] for testing
type mockStreamer struct {
	pushCalled bool
	pushedMsg  list.List
	pushErr    error
	flushErr   error
	pullCalled bool
	pullTypes  []common.MessageType
	pullMsg    common.Message[common.MessageType]
	pullMsgs   []common.Message[common.MessageType]
	pullErr    error
	drainIn    int
}

// wrapped mock stream combines both a pre-filled message incoming queue
// and a real streamer. When the incoming queue is empty all Pull call are delegated
// to the real implementation. All other methods are directly delegated.
type wrappedMockStreamer struct {
	incoming *list.List
	streamer *MessageStreamer
}

// Makes a new NewWrappedMockStreamer
// arguments:
//   - msgs a list of common.Message[common.MessageType]
//   - streamer a streamer
func NewWrappedMockStreamer(msgs *list.List, streamer *MessageStreamer) *wrappedMockStreamer {
	new := &wrappedMockStreamer{}
	new.incoming = msgs
	new.streamer = streamer
	return new
}

func (m *wrappedMockStreamer) Push(ctx context.Context, msg common.Message[common.MessageType]) error {
	return m.streamer.Push(ctx, msg)
}
func (m *wrappedMockStreamer) Pull(ctx context.Context, types ...common.MessageType) (common.Message[common.MessageType], error) {
	for incoming := m.incoming.Front(); incoming != nil; incoming = incoming.Next() {
		if _isExpectedType(types, incoming.Value.(common.Message[common.MessageType]).GetMsgCode()) {
			m.incoming.Remove(incoming)
			return incoming.Value.(common.Message[common.MessageType]), nil
		}
	}

	return m.streamer.Pull(ctx, types...)
}
func (m *wrappedMockStreamer) Flush(ctx context.Context) error {
	return m.streamer.Flush(ctx)
}

func (m *wrappedMockStreamer) Drain(ctx context.Context, direction common.StreamDirection) (int, int) {
	var icomingLen = m.incoming.Len()
	var i, o = m.streamer.Drain(ctx, direction)
	return icomingLen + i, o
}

func (m *wrappedMockStreamer) isValid(ctx context.Context) bool {
	msgIn, _ := m.Drain(ctx, common.IN)
	if msgIn == 0 {
		return true
	}
	m.streamer.shelf.getEventService().post(streamerStaleEvent)
	return false
}

func (m *wrappedMockStreamer) RegisterPostUnmarshallCallback(t common.MessageType, cb StreamerPostUnmarshallCallback) {
	m.streamer.RegisterPostUnmarshallCallback(t, cb)
}

func (m *wrappedMockStreamer) RegisterPreUnmarshallCallback(t common.MessageType, cb StreamerPreUnmarshallCallback) {
	m.streamer.RegisterPreUnmarshallCallback(t, cb)
}

func (m *wrappedMockStreamer) UnRegisterPostUnmarshallCallback(t common.MessageType) {
	m.streamer.UnRegisterPostUnmarshallCallback(t)
}

func (m *wrappedMockStreamer) UnRegisterPreUnmarshallCallback(t common.MessageType) {
	m.streamer.UnRegisterPreUnmarshallCallback(t)
}

func (m *mockStreamer) Push(_ context.Context, msg common.Message[common.MessageType]) error {
	m.pushCalled = true
	m.pushedMsg.PushBack(&msg)
	return m.pushErr
}

func (m *mockStreamer) Pull(_ context.Context, types ...common.MessageType) (common.Message[common.MessageType], error) {
	m.pullCalled = true
	m.pullTypes = types
	if len(m.pullMsgs) > 0 {
		msg := m.pullMsgs[0]
		m.pullMsgs = m.pullMsgs[1:]
		return msg, nil
	}
	return m.pullMsg, m.pullErr
}

func (m *mockStreamer) Flush(_ context.Context) error {
	return m.flushErr
}

func (m *mockStreamer) Drain(_ context.Context, direction common.StreamDirection) (int, int) {
	if direction == common.IN {
		return m.drainIn, 0
	}
	return 0, 0
}

func (m *mockStreamer) RegisterPostUnmarshallCallback(common.MessageType, StreamerPostUnmarshallCallback) {
}

func (m *mockStreamer) RegisterPreUnmarshallCallback(common.MessageType, StreamerPreUnmarshallCallback) {
}

func (m *mockStreamer) UnRegisterPostUnmarshallCallback(common.MessageType) {}

func (m *mockStreamer) UnRegisterPreUnmarshallCallback(common.MessageType) {}

// dummyMsg is a dummy implementation of common.Message[common.MessageType]
type dummyMsg struct{}

func (d *dummyMsg) GetMsgCode() common.MessageType { return TTIFUN }

// mockOer implements tTIOerIface and common.Message[common.MessageType]
type mockOer struct {
	err error
}

func (m *mockOer) getError() error                { return m.err }
func (m *mockOer) GetMsgCode() common.MessageType { return TTIOER }

type mockNetworkSession struct {
	cancelCalls     int
	disconnectCalls int
	disconnectErr   error
	sleepDuration   time.Duration
	cancelErr       error
	inband          bool
	remoteAddress   string
	remotePort      int
}

// newTestConnection creates a connection without querying DBTIMEZONE. Tests that
// exercise connection behavior independently of initialization use this helper.
func newTestConnection(
	shelf *ttiShelf[common.MessageType],
	sessCtx *common.SessionContext,
	ns common.NetworkSession,
) *connection {
	conn := &connection{
		shelf:     shelf,
		sessCtx:   sessCtx,
		ns:        ns,
		_isValid:  true,
		_isClosed: false,
	}
	conn.registerEventListeners(conn.shelf.getEventService())
	_registerHandleConnectionShouldBeDropped(shelf, conn)
	shelf.registerCancelExecution(conn.cancelCurrentExecution)
	return conn
}

// CheckInbandNotification implements [common.NetworkSession].
func (m *mockNetworkSession) CheckInbandNotification() bool {
	return m.inband
}

func (m *mockNetworkSession) GetRemoteAddress() string {
	return m.remoteAddress
}

func (m *mockNetworkSession) GetRemotePort() int {
	return m.remotePort
}

func (m *mockNetworkSession) CancelOperation(ctx context.Context) error {
	m.cancelCalls++
	return m.cancelErr
}

func (m *mockNetworkSession) Disconnect(ctx context.Context, flags int) error {
	m.disconnectCalls++
	time.Sleep(m.sleepDuration)
	return m.disconnectErr
}

// *** Negotiator ***

// mockNegotiator is a mock implementation of the Negotiator interface for testing.
type mockNegotiator struct {
	sessCtx         *common.SessionContext
	shelf           *ttiShelf[common.MessageType]
	err             error
	negotiateCalled bool
}

func (m *mockNegotiator) Negotiate(ctx context.Context) (*common.SessionContext, *ttiShelf[common.MessageType], error) {
	m.negotiateCalled = true
	return m.sessCtx, m.shelf, m.err
}

// *** Authenticator ***

// mockAuthenticator is a mock implementation of the Authenticator interface for testing.
type mockAuthenticator struct {
	err                error
	authenticateCalled bool
}

func (m *mockAuthenticator) Authenticate(ctx context.Context) error {
	m.authenticateCalled = true
	return m.err
}
