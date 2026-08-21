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
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
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
	{"TestCapabilityNew", "unitary", false, TestCapabilityNew},
	{"TestCapabilityNewDefault", "unitary", false, TestCapabilityNewDefault},
	{"TestCapabilityMarshalTo_Success", "unitary", false, TestCapabilityMarshalTo_Success},
	{"TestCapabilityMarshalTo_Fail", "unitary", false, TestCapabilityMarshalTo_Fail},
	{"TestCapabilityUnMarshalFrom_Success", "unitary", false, TestCapabilityUnMarshalFrom_Success},
	{"TestCapabilityUnMarshalFrom_Fail", "unitary", false, TestCapabilityUnMarshalFrom_Fail},
	{"TestCapability_AdjustCapabilityFrom", "unitary", false, TestCapability_AdjustCapabilityFrom},
	{"TestCapability_toMap", "unitary", false, TestCapability_toMap},
	{"TestConnectionCloser_Close", "unitary", false, TestConnectionCloser_Close},
	{"TestConnectionCloser_CloseWithTimeout", "unitary", false, TestConnectionCloser_CloseWithTimeout},
	{"TestEventServiceRegisterAndPost", "unitary", false, TestEventServiceRegisterAndPost},
	{"TestAuthencationFactoryWithNilParameters", "unitary", false, TestAuthencationFactoryWithNilParameters},
	{"TestAuthencationFactoryBasic", "unitary", false, TestAuthencationFactoryBasic},
	{"TestGetAuthenticator_UsesTokenAuthenticatorForOCIToken", "unitary", false, TestGetAuthenticator_UsesTokenAuthenticatorForOCIToken},
	{"TestGetAuthenticator_UsesTokenAuthenticatorForOAuth", "unitary", false, TestGetAuthenticator_UsesTokenAuthenticatorForOAuth},
	{"TestGetConnection", "unitary", false, TestGetConnection},
	{"TestConnectionPinger_Ping", "unitary", false, TestConnectionPinger_Ping},
	{"TestConnectionPinger_IsValid", "unitary", false, TestConnectionPinger_IsValid},
	{"TestConnectionPinger_IsValidWithInband", "unitary", false, TestConnectionPinger_IsValidWithInband},
	{"TestFactoryRegistries", "unitary", false, TestFactoryRegistries},
	{"TestReplaceMessage", "unitary", false, TestReplaceMessage},
	{"TestFactoryGetMessage", "unitary", false, TestFactoryGetMessage},
	{"TestFactoryGetMessageFromFunction", "unitary", false, TestFactoryGetMessageFromFunction},
	{"TestNewMarshalEngine", "unitary", false, TestNewMarshalEngine},
	{"TestMarshalUB1", "unitary", false, TestMarshalUB1},
	{"TestMarshalUB2", "unitary", false, TestMarshalUB2},
	{"TestMarshalSB4", "unitary", false, TestMarshalSB4},
	{"TestMarshalUB4", "unitary", false, TestMarshalUB4},
	{"TestMarshalUB8", "unitary", false, TestMarshalUB8},
	{"TestMarshalByteArray", "unitary", false, TestMarshalByteArray},
	{"TestMarshalChar", "unitary", false, TestMarshalChar},
	{"TestMarshalPTR", "unitary", false, TestMarshalPTR},
	{"TestMarshalNullPTR", "unitary", false, TestMarshalNullPTR},
	{"TestMarshalCLR", "unitary", false, TestMarshalCLR},
	{"TestMarshalKeyValue", "unitary", false, TestMarshalKeyValue},
	{"TestFlush", "unitary", false, TestFlush},
	{"TestUnmarshalUB1", "unitary", false, TestUnmarshalUB1},
	{"TestUnmarshalUB2", "unitary", false, TestUnmarshalUB2},
	{"TestUnmarshalSB1", "unitary", false, TestUnmarshalSB1},
	{"TestUnmarshalSB2", "unitary", false, TestUnmarshalSB2},
	{"TestUnmarshalSB4", "unitary", false, TestUnmarshalSB4},
	{"TestUnmarshalUB4", "unitary", false, TestUnmarshalUB4},
	{"TestUnmarshalUB8", "unitary", false, TestUnmarshalUB8},
	{"TestUnmarshalCLR", "unitary", false, TestUnmarshalCLR},
	{"TestUnmarshalByteArray", "unitary", false, TestUnmarshalByteArray},
	{"TestUnmarshalText", "unitary", false, TestUnmarshalText},
	{"TestUnmarshalKeyValue", "unitary", false, TestUnmarshalKeyValue},
	{"TestMarshalUB1NoSpace", "unitary", false, TestMarshalUB1NoSpace},
	{"TestMarshalCLRNoSpace", "unitary", false, TestMarshalCLRNoSpace},
	{"TestMarshalKeyValueNoSpace", "unitary", false, TestMarshalKeyValueNoSpace},
	{"TestUnmarshalKeyValueNoSpace", "unitary", false, TestUnmarshalKeyValueNoSpace},
	{"TestUnmarshalUB2NoSpace", "unitary", false, TestUnmarshalUB2NoSpace},
	{"TestUnmarshalUB4NoSpace", "unitary", false, TestUnmarshalUB4NoSpace},
	{"TestUnmarshalUB4WithNegativeZeroLength", "unitary", false, TestUnmarshalUB4WithNegativeZeroLength},
	{"TestUnmarshalUB1ArrayNoSpace", "unitary", false, TestUnmarshalUB1ArrayNoSpace},
	{"TestUnmarshalTextNoSpace", "unitary", false, TestUnmarshalTextNoSpace},
	{"TestUnmarshalCLREscapeValue", "unitary", false, TestUnmarshalCLREscapeValue},
	{"TestUnmarshalCLRZeroLength", "unitary", false, TestUnmarshalCLRZeroLength},
	{"TestUnmarshalCLRNullLengthIndicator", "unitary", false, TestUnmarshalCLRNullLengthIndicator},
	{"TestUnmarshalCLRNegativeLength", "unitary", false, TestUnmarshalCLRNegativeLength},
	{"TestUnmarshalCLRNotEnoughBytesForLength", "unitary", false, TestUnmarshalCLRNotEnoughBytesForLength},
	{"TestUnmarshalCLRBufferToSmallForLength", "unitary", false, TestUnmarshalCLRBufferToSmallForLength},
	{"TestUnmarshalCLRReadLessThatTotalLength", "unitary", false, TestUnmarshalCLRReadLessThatTotalLength},
	{"TestUnmarshalCLRReadLessThatTotalLengthNoSpace", "unitary", false, TestUnmarshalCLRReadLessThatTotalLengthNoSpace},
	{"TestUnmarshalSB2UniversalRangeValidation", "unitary", false, TestUnmarshalSB2UniversalRangeValidation},
	{"TestUnmarshalUnsignedUniversalRejectsNegativeFlag", "unitary", false, TestUnmarshalUnsignedUniversalRejectsNegativeFlag},
	{"TestUnmarshalUniversalRejectsByteCountsOverTargetWidth", "unitary", false, TestUnmarshalUniversalRejectsByteCountsOverTargetWidth},
	{"TestUnmarshalUniversalUnsignedRejectsNegativeEncodings", "unitary", false, TestUnmarshalUniversalUnsignedRejectsNegativeEncodings},
	{"TestUnmarshalSB4UniversalRangeValidation", "unitary", false, TestUnmarshalSB4UniversalRangeValidation},
	{"TestDynamicAllocatedArrayRejectsNegativeUniversalLength", "unitary", false, TestDynamicAllocatedArrayRejectsNegativeUniversalLength},
	{"TestMarshalUnmarshalUB8UsesB8Representation", "unitary", false, TestMarshalUnmarshalUB8UsesB8Representation},
	{"TestMarshalDALC", "unitary", false, TestMarshalDALC},
	{"TestMarshalKeywordValuePairs", "unitary", false, TestMarshalKeywordValuePairs},
	{"TestMarshalKeywordValuePairsWithEmptyValues", "unitary", false, TestMarshalKeywordValuePairsWithEmptyValues},
	{"TestMarshalSequenceNumber", "unitary", false, TestMarshalSequenceNumber},
	{"TestMarshalSequenceNumberRotation", "unitary", false, TestMarshalSequenceNumberRotation},
	{"TestMarshalTokenNumber", "unitary", false, TestMarshalTokenNumber},
	{"TestMessageStreamer_Flush", "unitary", false, TestMessageStreamer_Flush},
	{"TestMessageStreamer_NullPull", "unitary", false, TestMessageStreamer_NullPull},
	{"TestMessageStreamer_SimpleTypedPull", "unitary", false, TestMessageStreamer_SimpleTypedPull},
	{"TestMessageStreamer_CallbackRegisterPreUnmarshal", "unitary", false, TestMessageStreamer_CallbackRegisterPreUnmarshal},
	{"TestMessageStreamer_CallbackRegisterPostUnmarshal", "unitary", false, TestMessageStreamer_CallbackRegisterPostUnmarshal},
	{"TestMessageStreamer_CallbackUnregister", "unitary", false, TestMessageStreamer_CallbackUnregister},
	{"TestMessageStreamer_Drain", "unitary", false, TestMessageStreamer_Drain},
	{"TestMessageStreamer_SimplePush", "unitary", false, TestMessageStreamer_SimplePush},
	{"TestMessageStreamer_PullFromIncomings", "unitary", false, TestMessageStreamer_PullFromIncomings},
	{"TestMessageStreamer_OrderedPush", "unitary", false, TestMessageStreamer_OrderedPush},
	{"TestMessageStreamer_PullWithTimeout", "unitary", false, TestMessageStreamer_PullWithTimeout},
	{"TestMessageStreamer_PullWithTimeoutAndIncomings", "unitary", false, TestMessageStreamer_PullWithTimeoutAndIncomings},
	{"TestMessageStreamer_CallbackPullWithPreUnmarshallAlloc", "unitary", false, TestMessageStreamer_CallbackPullWithPreUnmarshallAlloc},
	{"TestMessageStreamer_CallbackPullWithPreUnmarshallError", "unitary", false, TestMessageStreamer_CallbackPullWithPreUnmarshallError},
	{"TestMessageStreamer_UnmarshallError", "unitary", false, TestMessageStreamer_UnmarshallError},
	{"TestMessageStreamer_CallbackPullWithPostUnmarshallKeepFalse", "unitary", false, TestMessageStreamer_CallbackPullWithPostUnmarshallKeepFalse},
	{"TestMessageStreamer_CallbackPullWithPostUnmarshallKeepTrueAndError", "unitary", false, TestMessageStreamer_CallbackPullWithPostUnmarshallKeepTrueAndError},
	{"TestMessageStreamer_getMessage", "unitary", false, TestMessageStreamer_getMessage},
	{"TestMessageHeader_UnMarshalFrom", "unitary", false, TestMessageHeader_UnMarshalFrom},
	{"TestOcca_New_Getters", "unitary", false, TestOcca_New_Getters},
	{"TestOcca_SetCursorIDs", "unitary", false, TestOcca_SetCursorIDs},
	{"TestOcca_MarshalTo_Success", "unitary", false, TestOcca_MarshalTo_Success},
	{"TestOcca_MarshalTo_EmptyCursorIDs", "unitary", false, TestOcca_MarshalTo_EmptyCursorIDs},
	{"TestOcca_MarshalTo_Failures", "unitary", false, TestOcca_MarshalTo_Failures},
	{"TestRegisterServerToClientPiggybacks", "unitary", false, TestRegisterServerToClientPiggybacks},
	{"TestHandleServerToClientPiggyback_ValidOCSSYNC", "unitary", false, TestHandleServerToClientPiggyback_ValidOCSSYNC},
	{"TestHandleServerToClientPiggyback_NotFunction", "unitary", false, TestHandleServerToClientPiggyback_NotFunction},
	{"TestHandleServerToClientPiggyback_UnknownFunction", "unitary", false, TestHandleServerToClientPiggyback_UnknownFunction},
	{"TestHandleServerToClientPiggyback_ErrorInMessage", "unitary", false, TestHandleServerToClientPiggyback_ErrorInMessage},
	{"TestParseAndOrder_Positional_Success_NoDup", "unitary", false, TestParseAndOrder_Positional_Success_NoDup},
	{"TestParseAndOrder_Named_Success_WithDupRef", "unitary", false, TestParseAndOrder_Named_Success_WithDupRef},
	{"TestParseAndOrder_Named_Error_EmptyNameWithoutValidOrdinal", "unitary", false, TestParseAndOrder_Named_Error_EmptyNameWithoutValidOrdinal},
	{"TestParseAndOrder_Named_Error_MissingName", "unitary", false, TestParseAndOrder_Named_Error_MissingName},
	{"TestParseAndOrder_Named_Error_ExtraName", "unitary", false, TestParseAndOrder_Named_Error_ExtraName},
	{"TestParsePlaceholders_Error_DanglingColon", "unitary", false, TestParsePlaceholders_Error_DanglingColon},
	{"TestParsePlaceholders_MixedIdentifiers_Supported", "unitary", false, TestParsePlaceholders_MixedIdentifiers_Supported},
	{"TestParsePlaceholders_IgnoreInLineComment", "unitary", false, TestParsePlaceholders_IgnoreInLineComment},
	{"TestParsePlaceholders_IgnoreInBlockComment", "unitary", false, TestParsePlaceholders_IgnoreInBlockComment},
	{"TestParsePlaceholders_IgnoreInSingleQuotedString", "unitary", false, TestParsePlaceholders_IgnoreInSingleQuotedString},
	{"TestParsePlaceholders_IgnoreInSingleQuotedStringWithEscape", "unitary", false, TestParsePlaceholders_IgnoreInSingleQuotedStringWithEscape},
	{"TestParsePlaceholders_IgnoreInDoubleQuotedIdentifier", "unitary", false, TestParsePlaceholders_IgnoreInDoubleQuotedIdentifier},
	{"TestParsePlaceholders_ParseAfterCommentClosed", "unitary", false, TestParsePlaceholders_ParseAfterCommentClosed},
	{"TestParsePlaceholders_SkipNonBindColonAssignment", "unitary", false, TestParsePlaceholders_SkipNonBindColonAssignment},
	{"TestOrderNamedValues_NamePrecedenceOverOrdinal", "unitary", false, TestOrderNamedValues_NamePrecedenceOverOrdinal},
	{"TestOrderNamedValues_RepeatName_FromName", "unitary", false, TestOrderNamedValues_RepeatName_FromName},
	{"TestOrderNamedValues_Error_DuplicateOrdinal", "unitary", false, TestOrderNamedValues_Error_DuplicateOrdinal},
	{"TestPlSqlNamedValues_Error_MissingName", "unitary", false, TestPlSqlNamedValues_Error_MissingName},
	{"TestPlSqlNamedValues_Error_DuplicateName", "unitary", false, TestPlSqlNamedValues_Error_DuplicateName},
	{"TestPlSqlNamedValues_LaterNamedBindWithoutEarlierBind", "unitary", false, TestPlSqlNamedValues_LaterNamedBindWithoutEarlierBind},
	{"TestParsePlaceholders_Uint16IndexesDoNotWrapAfter255", "unitary", false, TestParsePlaceholders_Uint16IndexesDoNotWrapAfter255},
	{"TestClassifySQL_AllKinds", "unitary", false, TestClassifySQL_AllKinds},
	{"TestSqlKind_InvalidValues", "unitary", false, TestSqlKind_InvalidValues},
	{"TestSqlKind_String_All", "unitary", false, TestSqlKind_String_All},
	{"TestGetQueryStatementExecutor_ReturnsSelect", "unitary", false, TestGetQueryStatementExecutor_ReturnsSelect},
	{"TestGetExecStatementExecutor_Mapping", "unitary", false, TestGetExecStatementExecutor_Mapping},
	{"TestGetQueryStatementExecutor_UnsupportedForNonSelect", "unitary", false, TestGetQueryStatementExecutor_UnsupportedForNonSelect},
	{"TestGetExecStatementExecutor_SelectIsError", "unitary", false, TestGetExecStatementExecutor_SelectIsError},
	{"TestStatementExecutor_Others_Drop_MarshalAndExec", "unitary", false, TestStatementExecutor_Others_Drop_MarshalAndExec},
	{"TestStatementExecutor_Others_Create_MarshalAndExec", "unitary", false, TestStatementExecutor_Others_Create_MarshalAndExec},
	{"TestStatementExecutor_DML_Insert_MarshalAndExec", "unitary", false, TestStatementExecutor_DML_Insert_MarshalAndExec},
	{"TestStatementExecutorDML_TTIFOBFlushesAndContinuesPull", "unitary", false, TestStatementExecutorDML_TTIFOBFlushesAndContinuesPull},
	{"TestStatementExecutor_Select_MarshalAndQuery", "unitary", false, TestStatementExecutor_Select_MarshalAndQuery},
	{"TestStatementExecutor_Select_DoesNotReuseStaleBVCStateAcrossExecutions", "unitary", false, TestStatementExecutor_Select_DoesNotReuseStaleBVCStateAcrossExecutions},
	{"TestStatementExecutor_PlSQL_MarshalAndExec", "unitary", false, TestStatementExecutor_PlSQL_MarshalAndExec},
	{"TestStatementExecutor_Select_FaultyFlush", "unitary", false, TestStatementExecutor_Select_FaultyFlush},
	{"TestStatementExecutor_Select_FaultyPull", "unitary", false, TestStatementExecutor_Select_FaultyPull},
	{"TestStatementExecutor_Select_OER_Error", "unitary", false, TestStatementExecutor_Select_OER_Error},
	{"TestStatementExecutor_Select_FaultyPush", "unitary", false, TestStatementExecutor_Select_FaultyPush},
	{"TestStatementExecutor_DML_FaultyFlush", "unitary", false, TestStatementExecutor_DML_FaultyFlush},
	{"TestStatementExecutor_DML_FaultyPull", "unitary", false, TestStatementExecutor_DML_FaultyPull},
	{"TestStatementExecutor_DML_OER_Error", "unitary", false, TestStatementExecutor_DML_OER_Error},
	{"TestStatementExecutor_Others_FaultyFlush", "unitary", false, TestStatementExecutor_Others_FaultyFlush},
	{"TestStatementExecutor_Others_FaultyPull", "unitary", false, TestStatementExecutor_Others_FaultyPull},
	{"TestStatementExecutor_Others_OER_Error", "unitary", false, TestStatementExecutor_Others_OER_Error},
	{"TestStatementExecutor_Select_Callback_GetMessage_RXD_Error_Integration", "unitary", false, TestStatementExecutor_Select_Callback_GetMessage_RXD_Error_Integration},
	{"TestStatementExecutor_Select_Callback_GetMessage_BVC_Error_Integration", "unitary", false, TestStatementExecutor_Select_Callback_GetMessage_BVC_Error_Integration},
	{"TestStatementExecutor_Select_OallRpaCallback_GetMessageForFunction_Error_Integration", "unitary", false, TestStatementExecutor_Select_OallRpaCallback_GetMessageForFunction_Error_Integration},
	{"TestStatementExecutor_Others_Factory_GetMessageForFunction_Error", "unitary", false, TestStatementExecutor_Others_Factory_GetMessageForFunction_Error},
	{"TestStatementExecutor_DML_Factory_GetMessageForFunction_Error", "unitary", false, TestStatementExecutor_DML_Factory_GetMessageForFunction_Error},
	{"TestStatementExecutor_Select_Factory_GetMessageForFunction_Error", "unitary", false, TestStatementExecutor_Select_Factory_GetMessageForFunction_Error},
	{"TestStatementExecutor_PLSQL_Factory_GetMessageForFunction_Error", "unitary", false, TestStatementExecutor_PLSQL_Factory_GetMessageForFunction_Error},
	{"TestStatementExecutor_DML_FaultyPush", "unitary", false, TestStatementExecutor_DML_FaultyPush},
	{"TestStatementExecutor_Others_FaultyPush", "unitary", false, TestStatementExecutor_Others_FaultyPush},
	{"TestPasswordAuthenticator_doOSESSKEY_Golden", "unitary", false, TestPasswordAuthenticator_doOSESSKEY_Golden},
	{"TestPasswordAuthenticator_doOAuth_Golden", "unitary", false, TestPasswordAuthenticator_doOAuth_Golden},
	{"TestOCITokenProviderResolveTokenPathDefault", "unitary", false, TestOCITokenProviderResolveTokenPathDefault},
	{"TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature", "unitary", false, TestOAuthSetTokenKeyValsForOAUTHAddsTokenHeaderAndSignature},
	{"TestOCITokenProviderGenerateTokenHeader", "unitary", false, TestOCITokenProviderGenerateTokenHeader},
	{"TestOAuthTokenProviderResolveTokenPathFile", "unitary", false, TestOAuthTokenProviderResolveTokenPathFile},
	{"TestOAuthTokenProviderApplyAuthDataAddsTokenOnly", "unitary", false, TestOAuthTokenProviderApplyAuthDataAddsTokenOnly},
	{"TestTokenAuthenticatorResolveAccessTokenUsesConfiguredAccessToken", "unitary", false, TestTokenAuthenticatorResolveAccessTokenUsesConfiguredAccessToken},
	{"TestTokenAuthenticatorResolveAccessTokenPrefersAccessTokenOverLocation", "unitary", false, TestTokenAuthenticatorResolveAccessTokenPrefersAccessTokenOverLocation},
	{"TestValidateJWTExpirationExpired", "unitary", false, TestValidateJWTExpirationExpired},
	{"TestConnectionNegotiator_Negotiate_Fail", "unitary", false, TestConnectionNegotiator_Negotiate_Fail},
	{"TestConnectionNegotiator_Negotiate_Success", "unitary", false, TestConnectionNegotiator_Negotiate_Success},
	{"TestStatement_QueryContext_JSONConstructor_NamedBindAfterQuotedKey", "unitary", false, TestStatement_QueryContext_JSONConstructor_NamedBindAfterQuotedKey},
	{"TestStatement_QueryContext_JSONConstructor_AdditionalNamedBindShapes", "unitary", false, TestStatement_QueryContext_JSONConstructor_AdditionalNamedBindShapes},
	{"TestTTIbvc_GetMsgCode", "unitary", false, TestTTIbvc_GetMsgCode},
	{"TestTTIbvc_SetNumberOfColumns", "unitary", false, TestTTIbvc_SetNumberOfColumns},
	{"TestTTIbvc_UnMarshalFrom", "unitary", false, TestTTIbvc_UnMarshalFrom},
	{"TestTTIbvc_SetBitVector", "unitary", false, TestTTIbvc_SetBitVector},
	{"TestTTIdcb24Constructor", "unitary", false, TestTTIdcb24Constructor},
	{"TestTTIdcb_GetMsgCode", "unitary", false, TestTTIdcb_GetMsgCode},
	{"TestTTIdcb24UnMarshalFrom_Success", "unitary", false, TestTTIdcb24UnMarshalFrom_Success},
	{"TestTTIdcb24UnMarshalFrom_Fail", "unitary", false, TestTTIdcb24UnMarshalFrom_Fail},
	{"TestTTIdcb_receiveCommon_FromOdny_Fail", "unitary", false, TestTTIdcb_receiveCommon_FromOdny_Fail},
	{"TestTTIdcb_PerColumnUDS_NoAliasing", "unitary", false, TestTTIdcb_PerColumnUDS_NoAliasing},
	{"TestTTIdtyNew", "unitary", false, TestTTIdtyNew},
	{"TestTTIdtyMarshalTo_Success", "unitary", false, TestTTIdtyMarshalTo_Success},
	{"TestTTIdtyMarshalTo_Fail", "unitary", false, TestTTIdtyMarshalTo_Fail},
	{"TestTTIdtyGetters", "unitary", false, TestTTIdtyGetters},
	{"TestTTIdtyUnmarshalFrom_Success", "unitary", false, TestTTIdtyUnmarshalFrom_Success},
	{"TestTTIdtyUnmarshalFrom_Failure", "unitary", false, TestTTIdtyUnmarshalFrom_Failure},
	{"TestTTIFOB_MessageStreamerFlushesOnlyMessageCode", "unitary", false, TestTTIFOB_MessageStreamerFlushesOnlyMessageCode},
	{"TestTTIFunNoPayload_MarshalTo", "unitary", false, TestTTIFunNoPayload_MarshalTo},
	{"TestNewLogOff", "unitary", false, TestNewLogOff},
	{"TestNewLogOff18", "unitary", false, TestNewLogOff18},
	{"TestTTIoac_NewTTIoac", "unitary", false, TestTTIoac_NewTTIoac},
	{"TestTTIoac_UnMarshalFrom_Success", "unitary", false, TestTTIoac_UnMarshalFrom_Success},
	{"TestTTIoac_UnMarshalFrom_Fail", "unitary", false, TestTTIoac_UnMarshalFrom_Fail},
	{"TestTTIoac_MarshalTo_Success", "unitary", false, TestTTIoac_MarshalTo_Success},
	{"TestTTIoac_MarshalTo_Fail", "unitary", false, TestTTIoac_MarshalTo_Fail},
	{"TestTTIoac_Setters", "unitary", false, TestTTIoac_Setters},
	{"TestTTIOallRPA_Unmarshal_Drop", "unitary", false, TestTTIOallRPA_Unmarshal_Drop},
	{"TestTTIOallRPA_Unmarshal_Create", "unitary", false, TestTTIOallRPA_Unmarshal_Create},
	{"TestTTIOallRPA_Unmarshal_Insert", "unitary", false, TestTTIOallRPA_Unmarshal_Insert},
	{"TestTTIOallRPA_Unmarshal_AlterSessionDrop", "unitary", false, TestTTIOallRPA_Unmarshal_AlterSessionDrop},
	{"TestTTIOallRPA_UnmarshalFrom_Fail_Truncated", "unitary", false, TestTTIOallRPA_UnmarshalFrom_Fail_Truncated},
	{"TestTTIOallRPA_UnmarshalFrom_FaultyBuffer", "unitary", false, TestTTIOallRPA_UnmarshalFrom_FaultyBuffer},
	{"TestTTIOallRPA_UnmarshalFrom_UnsupportedNonZeroValues", "unitary", false, TestTTIOallRPA_UnmarshalFrom_UnsupportedNonZeroValues},
	{"TestTTIOallRPA_UnmarshalDMLRows_FaultyBuffer", "unitary", false, TestTTIOallRPA_UnmarshalDMLRows_FaultyBuffer},
	{"TestTTIOallRPA_Unmarshal_TransactionContext", "unitary", false, TestTTIOallRPA_Unmarshal_TransactionContext},
	{"TestTTIOallRPA_Unmarshal_TransactionContext_Fail_Bytes", "unitary", false, TestTTIOallRPA_Unmarshal_TransactionContext_Fail_Bytes},
	{"TestOall8_New_Success", "unitary", false, TestOall8_New_Success},
	{"TestOall8_GetMsgCode", "unitary", false, TestOall8_GetMsgCode},
	{"TestOall8_getFuncCode", "unitary", false, TestOall8_getFuncCode},
	{"TestOall8_MarshalTo_Drop_MatchesGolden", "unitary", false, TestOall8_MarshalTo_Drop_MatchesGolden},
	{"TestOall8_MarshalTo_Create_MatchesGolden", "unitary", false, TestOall8_MarshalTo_Create_MatchesGolden},
	{"TestOall8_MarshalTo_Insert_MatchesGolden", "unitary", false, TestOall8_MarshalTo_Insert_MatchesGolden},
	{"TestOall8_MarshalTo_Delete_MatchesGolden", "unitary", false, TestOall8_MarshalTo_Delete_MatchesGolden},
	{"TestOall8_MarshalTo_Select_MatchesGolden", "unitary", false, TestOall8_MarshalTo_Select_MatchesGolden},
	{"TestOall8_MarshalTo_Fail_DDL", "unitary", false, TestOall8_MarshalTo_Fail_DDL},
	{"TestOall8_MarshalTo_Fail_BindPtr", "unitary", false, TestOall8_MarshalTo_Fail_BindPtr},
	{"TestOall8_MarshalTo_Fail_BindCount", "unitary", false, TestOall8_MarshalTo_Fail_BindCount},
	{"TestOall8_MarshalTo_Fail_DefinePtr", "unitary", false, TestOall8_MarshalTo_Fail_DefinePtr},
	{"TestOall8_MarshalTo_Fail_DefineCount", "unitary", false, TestOall8_MarshalTo_Fail_DefineCount},
	{"TestOall8_MarshalTo_Fail_AL8I4_DataWrite", "unitary", false, TestOall8_MarshalTo_Fail_AL8I4_DataWrite},
	{"TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Success", "unitary", false, TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Success},
	{"TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Failures", "unitary", false, TestOall8_MarshalTo_EmptySQL_EmptyAL8I4_Failures},
	{"TestOall8_MarshalTo_Fail_DML", "unitary", false, TestOall8_MarshalTo_Fail_DML},
	{"TestOall8_MarshalTo_Fail_SELECT", "unitary", false, TestOall8_MarshalTo_Fail_SELECT},
	{"TestNewO5Logon", "unitary", false, TestNewO5Logon},
	{"TestHashMD5", "unitary", false, TestHashMD5},
	{"TestHashSHA1", "unitary", false, TestHashSHA1},
	{"TestHashSHA512", "unitary", false, TestHashSHA512},
	{"TestIsAllZero", "unitary", false, TestIsAllZero},
	{"TestRemovePKCS5Padding", "unitary", false, TestRemovePKCS5Padding},
	{"TestApplyPKCS5Padding", "unitary", false, TestApplyPKCS5Padding},
	{"TestApplyZeroPadding", "unitary", false, TestApplyZeroPadding},
	{"TestBuildO5LogonKey", "unitary", false, TestBuildO5LogonKey},
	{"TestGetDerivedKey", "unitary", false, TestGetDerivedKey},
	{"TestDecryptAES", "unitary", false, TestDecryptAES},
	{"TestEncryptAES", "unitary", false, TestEncryptAES},
	{"TestGeneratePk", "unitary", false, TestGeneratePk},
	{"TestConstructVerifierForSHA512", "unitary", false, TestConstructVerifierForSHA512},
	{"TestConstructVerifierExceptSHA512", "unitary", false, TestConstructVerifierExceptSHA512},
	{"TestValidateServerIdentity", "unitary", false, TestValidateServerIdentity},
	{"TestGenerateOAuthResponse", "unitary", false, TestGenerateOAuthResponse},
	{"TestGenerateOAuthResponse_SHA512", "unitary", false, TestGenerateOAuthResponse_SHA512},
	{"TestGenerateOAuthResponse_Ssh1", "unitary", false, TestGenerateOAuthResponse_Ssh1},
	{"TestGenerateOAuthResponse_Orcl7", "unitary", false, TestGenerateOAuthResponse_Orcl7},
	{"TestGenerateOAuthResponse_Smd5", "unitary", false, TestGenerateOAuthResponse_Smd5},
	{"TestGenerateOAuthResponse_Sh1", "unitary", false, TestGenerateOAuthResponse_Sh1},
	{"TestGenerateOAuthResponse_ErrorCases", "unitary", false, TestGenerateOAuthResponse_ErrorCases},
	{"TestGetO5LogonKey", "unitary", false, TestGetO5LogonKey},
	{"TestDecryptAESWithO5logonKey", "unitary", false, TestDecryptAESWithO5logonKey},
	{"TestEncryptAESWithO5logonKey", "unitary", false, TestEncryptAESWithO5logonKey},
	{"TestGenerateKb", "unitary", false, TestGenerateKb},
	{"TestGenerateSpeedKey", "unitary", false, TestGenerateSpeedKey},
	{"TestEncryptPassword", "unitary", false, TestEncryptPassword},
	{"TestNewOAuth_Success", "unitary", false, TestNewOAuth_Success},
	{"TestOAuth_setSessionFields", "unitary", true, TestOAuth_setSessionFields},
	{"TestOAuth_MarshalTo_Success", "unitary", false, TestOAuth_MarshalTo_Success},
	{"TestOAuth_MarshalTo_WithOSESSKEYRPA_Success", "unitary", false, TestOAuth_MarshalTo_WithOSESSKEYRPA_Success},
	{"TestOAuthMarshalTo_Fail", "unitary", false, TestOAuthMarshalTo_Fail},
	{"TestOAuth_prepareForOAUTH", "unitary", false, TestOAuth_prepareForOAUTH},
	{"TestOAuth_initializeLogonModeForOAUTH", "unitary", false, TestOAuth_initializeLogonModeForOAUTH},
	{"TestOAuth_setPasswordKeyValsForOAUTH", "unitary", false, TestOAuth_setPasswordKeyValsForOAUTH},
	{"TestOAuth_setPasswordKeyValsForOAUTH_WithEncryptedKB", "unitary", false, TestOAuth_setPasswordKeyValsForOAUTH_WithEncryptedKB},
	{"TestOAuth_setDriverIdentityKeyValsForOAUTH", "unitary", false, TestOAuth_setDriverIdentityKeyValsForOAUTH},
	{"TestOAuth_setAlterSessionKeyValsForOAUTH", "unitary", false, TestOAuth_setAlterSessionKeyValsForOAUTH},
	{"TestOAuth_validateKeySizeForOAUTH_Success", "unitary", false, TestOAuth_validateKeySizeForOAUTH_Success},
	{"TestOAuth_validateKeySizeForOAUTH_Failure", "unitary", false, TestOAuth_validateKeySizeForOAUTH_Failure},
	{"TestOAuth_sanitizeInputCredential", "unitary", false, TestOAuth_sanitizeInputCredential},
	{"TestOAuth_validateO5VerifierType_Success", "unitary", false, TestOAuth_validateO5VerifierType_Success},
	{"TestOAuth_setters", "unitary", false, TestOAuth_setters},
	{"TestOAuthRPA_NewOAuthRPA", "unitary", false, TestOAuthRPA_NewOAuthRPA},
	{"TestOAuthRPA_UnMarshalFrom_Golden", "unitary", false, TestOAuthRPA_UnMarshalFrom_Golden},
	{"TestOAuthRPAUnMarshalFrom_Fail", "unitary", false, TestOAuthRPAUnMarshalFrom_Fail},
	{"TestOAuthRPA_UnMarshalFrom_Fail", "unitary", false, TestOAuthRPA_UnMarshalFrom_Fail},
	{"TestOAuthRPA_UnMarshalFrom_Failure", "unitary", false, TestOAuthRPA_UnMarshalFrom_Failure},
	{"TestNewTTIoer14", "unitary", false, TestNewTTIoer14},
	{"TestTTIoer14_Init", "unitary", false, TestTTIoer14_Init},
	{"TestTTIoer14_GetMsgCode", "unitary", false, TestTTIoer14_GetMsgCode},
	{"TestTTIoer14_UnmarshalAttributes_Success", "unitary", false, TestTTIoer14_UnmarshalAttributes_Success},
	{"TestTTIoer14_UnmarshalAttributes_Fail", "unitary", false, TestTTIoer14_UnmarshalAttributes_Fail},
	{"TestTTIoer14_UnMarshalFrom_Success", "unitary", false, TestTTIoer14_UnMarshalFrom_Success},
	{"TestTTIoer14_UnMarshalFrom_Fail", "unitary", false, TestTTIoer14_UnMarshalFrom_Fail},
	{"TestTTIoer14_UpdateChecksum", "unitary", false, TestTTIoer14_UpdateChecksum},
	{"TestNewTTIoer", "unitary", false, TestNewTTIoer},
	{"TestTTIoer_Init", "unitary", false, TestTTIoer_Init},
	{"TestTTIoer_GetMsgCode", "unitary", false, TestTTIoer_GetMsgCode},
	{"TestTTIoer_Getters", "unitary", false, TestTTIoer_Getters},
	{"TestTTIoer_GetError", "unitary", false, TestTTIoer_GetError},
	{"TestTTIoer_UnmarshalAttributes_Success", "unitary", false, TestTTIoer_UnmarshalAttributes_Success},
	{"TestTTIoer_UnmarshalAttributes_Fail", "unitary", false, TestTTIoer_UnmarshalAttributes_Fail},
	{"TestTTIoer_UnMarshalFrom", "unitary", false, TestTTIoer_UnMarshalFrom},
	{"TestTTIoer_UpdateChecksum", "unitary", false, TestTTIoer_UpdateChecksum},
	{"TestTTIoer_UnmarshalWarning", "unitary", false, TestTTIoer_UnmarshalWarning},
	{"TestOSesskeyNew", "unitary", false, TestOSesskeyNew},
	{"TestOSesskeyMarshalTo_Success", "unitary", false, TestOSesskeyMarshalTo_Success},
	{"TestOSesskeyMarshalTo_GoldenMatch", "unitary", false, TestOSesskeyMarshalTo_GoldenMatch},
	{"TestOSesskeyMarshalTo_Fail", "unitary", false, TestOSesskeyMarshalTo_Fail},
	{"TestOSesskeyRPANew", "unitary", false, TestOSesskeyRPANew},
	{"TestOSesskeyRPAUnMarshalFrom_Success", "unitary", false, TestOSesskeyRPAUnMarshalFrom_Success},
	{"TestOSesskeyRPAUnMarshalFrom_Fail", "unitary", false, TestOSesskeyRPAUnMarshalFrom_Fail},
	{"TestOSesskeyRPAGetters", "unitary", false, TestOSesskeyRPAGetters},
	{"TestOSesskeyRPAUnMarshalFrom_Golden", "unitary", false, TestOSesskeyRPAUnMarshalFrom_Golden},
	{"TestTTIproNew", "unitary", false, TestTTIproNew},
	{"TestTTIproMarshalTo_Success", "unitary", false, TestTTIproMarshalTo_Success},
	{"TestTTIproMarshalTo_Fail", "unitary", false, TestTTIproMarshalTo_Fail},
	{"TestTTIproGetters", "unitary", false, TestTTIproGetters},
	{"TestTTIproUnmarshalFrom_Success", "unitary", false, TestTTIproUnmarshalFrom_Success},
	{"TestTTIproUnmarshalFrom_FailInvalidData", "unitary", false, TestTTIproUnmarshalFrom_FailInvalidData},
	{"TestTTIproUnmarshalFrom_FailUnmarshal", "unitary", false, TestTTIproUnmarshalFrom_FailUnmarshal},
	{"TestTTIrxd_GetMsgCode", "unitary", false, TestTTIrxd_GetMsgCode},
	{"TestTTIrxd_BvcOnFirstRow_ReturnsError", "unitary", false, TestTTIrxd_BvcOnFirstRow_ReturnsError},
	{"TestTTIrxd_Setters", "unitary", false, TestTTIrxd_Setters},
	{"TestTTIrxd_UnmarshalFrom_ErrorCases", "unitary", false, TestTTIrxd_UnmarshalFrom_ErrorCases},
	{"TestTTIrxd_UnmarshalFrom", "unitary", false, TestTTIrxd_UnmarshalFrom},
	{"TestTTIrxd_bvc_IntegrationTest", "unitary", false, TestTTIrxd_bvc_IntegrationTest},
	{"TestTTIrxd_BvcPresentColumn_UnmarshalError", "unitary", false, TestTTIrxd_BvcPresentColumn_UnmarshalError},
	{"TestTTIrxd_BvcCarriedNullKeepsLobContextAligned", "unitary", false, TestTTIrxd_BvcCarriedNullKeepsLobContextAligned},
	{"TestTTIrxd_BvcCarriedClobPreservesLobContext", "unitary", false, TestTTIrxd_BvcCarriedClobPreservesLobContext},
	{"TestTTIrxd_MarshalTo_Success", "unitary", false, TestTTIrxd_MarshalTo_Success},
	{"TestTTIrxd_MarshalTo_FailOnNullIndicator", "unitary", false, TestTTIrxd_MarshalTo_FailOnNullIndicator},
	{"TestTTIrxd_MarshalTo_FailOnCLRDataWrite", "unitary", false, TestTTIrxd_MarshalTo_FailOnCLRDataWrite},
	{"TestTTIrxhConstructor", "unitary", false, TestTTIrxhConstructor},
	{"TestTTIrxhUnMarshalFrom_Success", "unitary", false, TestTTIrxhUnMarshalFrom_Success},
	{"TestTTIrxhUnMarshalFrom_Fail", "unitary", false, TestTTIrxhUnMarshalFrom_Fail},
	{"TestTTISPF_New", "unitary", false, TestTTISPF_New},
	{"TestTTISPF_Unmarshal_Success", "unitary", false, TestTTISPF_Unmarshal_Success},
	{"TestTTISPF_Unmarshal_Fail_TruncatedPayloads", "unitary", false, TestTTISPF_Unmarshal_Fail_TruncatedPayloads},
	{"TestTTISPF_Unmarshal_Fail_FaultyBuffer", "unitary", false, TestTTISPF_Unmarshal_Fail_FaultyBuffer},
	{"TestGetKeyValueFromKeyword_Timezone_WithRegionID_NameUsed", "unitary", false, TestGetKeyValueFromKeyword_Timezone_WithRegionID_NameUsed},
	{"TestGetKeyValueFromKeyword_Timezone_WithRegionID_FallbackGMT", "unitary", false, TestGetKeyValueFromKeyword_Timezone_WithRegionID_FallbackGMT},
	{"TestGetKeyValueFromKeyword_Nls_TextValueBranch", "unitary", false, TestGetKeyValueFromKeyword_Nls_TextValueBranch},
	{"TestGetKeyValueFromKeyword_PdbElasticPoolLdr_True", "unitary", false, TestGetKeyValueFromKeyword_PdbElasticPoolLdr_True},
	{"TestGetKeyValueFromKeyword_PdbAppRoot_True", "unitary", false, TestGetKeyValueFromKeyword_PdbAppRoot_True},
	{"Test_newTTISTA", "unitary", false, Test_newTTISTA},
	{"Test_newTTISTAWithEndOfCallStatusSupport", "unitary", false, Test_newTTISTAWithEndOfCallStatusSupport},
	{"Test_ttiSTA_UnMarshalFrom_WithoutSupport", "unitary", false, Test_ttiSTA_UnMarshalFrom_WithoutSupport},
	{"Test_ttiSTA_UnMarshalFrom_WithSupport", "unitary", false, Test_ttiSTA_UnMarshalFrom_WithSupport},
	{"Test_ttiSTA_UnMarshalFrom_WithSupport_DropFlag", "unitary", false, Test_ttiSTA_UnMarshalFrom_WithSupport_DropFlag},
	{"Test_ttiSTA_UnMarshalFrom_ErrorInUB2", "unitary", false, Test_ttiSTA_UnMarshalFrom_ErrorInUB2},
	{"Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInEOCS", "unitary", false, Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInEOCS},
	{"Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInElapsedTime", "unitary", false, Test_ttiSTA_UnMarshalFrom_WithSupport_ErrorInElapsedTime},
	{"TestTTIuds17_UnMarshalFrom_Success", "unitary", false, TestTTIuds17_UnMarshalFrom_Success},
	{"TestTTIuds17_UnMarshalFrom_Fail", "unitary", false, TestTTIuds17_UnMarshalFrom_Fail},
	{"TestTTIuds20_UnMarshalFrom_Success", "unitary", false, TestTTIuds20_UnMarshalFrom_Success},
	{"TestTTIuds20_UnMarshalFrom_Fail", "unitary", false, TestTTIuds20_UnMarshalFrom_Fail},
	{"TestTTIuds24_UnMarshalFrom_Success", "unitary", false, TestTTIuds24_UnMarshalFrom_Success},
	{"TestTTIuds24_UnMarshalFrom_Fail", "unitary", false, TestTTIuds24_UnMarshalFrom_Fail},
	{"TestTTIuds_UnMarshalFrom_Success", "unitary", false, TestTTIuds_UnMarshalFrom_Success},
	{"TestTTIuds_UnMarshalFrom_Fail", "unitary", false, TestTTIuds_UnMarshalFrom_Fail},
	{"TestTTIuds_GetColumnName", "unitary", false, TestTTIuds_GetColumnName},
	{"TestTTIuds_GetUdsArrayAndColCount", "unitary", false, TestTTIuds_GetUdsArrayAndColCount},
	{"TestTypeRep_NewTypeRep", "unitary", false, TestTypeRep_NewTypeRep},
	{"TestTypeRep_MarshalTo_Fail", "unitary", false, TestTypeRep_MarshalTo_Fail},
	{"TestTypeRep_UnMarshalFrom_Success", "unitary", false, TestTypeRep_UnMarshalFrom_Success},
	{"TestTypeRep_UnMarshalFrom_Fail", "unitary", false, TestTypeRep_UnMarshalFrom_Fail},
	{"TestTypeRep_MarshalTo_Success", "unitary", false, TestTypeRep_MarshalTo_Success},
	{"TestTypeRep_AddTypeRepToTable_Resize", "unitary", false, TestTypeRep_AddTypeRepToTable_Resize},
	{"TestTypeRep_SetRepAndGetRep", "unitary", false, TestTypeRep_SetRepAndGetRep},
	{"TestTypeRep_SetFlagsAndGetFlags", "unitary", false, TestTypeRep_SetFlagsAndGetFlags},
	{"TestTypeRep_Setters_Getters", "unitary", false, TestTypeRep_Setters_Getters},
	{"TestGetIDFromZone_Found", "unitary", false, TestGetIDFromZone_Found},
	{"TestGetIDFromZone_NotFound", "unitary", false, TestGetIDFromZone_NotFound},
	{"TestGetZoneFromID_Found", "unitary", false, TestGetZoneFromID_Found},
	{"TestGetZoneFromID_NotFound", "unitary", false, TestGetZoneFromID_NotFound},
	{"TestZoneMaps_Sanity", "unitary", false, TestZoneMaps_Sanity},
	{"TestCodecFactory_GetEncoder", "unitary", false, TestCodecFactory_GetEncoder},
	{"TestCodecFactory_GetDecoder", "unitary", false, TestCodecFactory_GetDecoder},
	{"TestCodecFactory_RegisterEncoderGeneric", "unitary", false, TestCodecFactory_RegisterEncoderGeneric},
	{"TestTTIShelf_NewShelf", "unitary", false, TestTTIShelf_NewShelf},
	{"TestTTIShelf_RegisterCodecFactoryAndGetter", "unitary", false, TestTTIShelf_RegisterCodecFactoryAndGetter},

	{"TestPrepareBindsAndOAC_Normalize_Supported", "unitary", false, TestPrepareBindsAndOAC_Normalize_Supported},
	{"TestPrepareBindsAndOAC_Normalize_Errors", "unitary", false, TestPrepareBindsAndOAC_Normalize_Errors},
	{"TestPrepareBindsAndOAC_Encode_Supported", "unitary", false, TestPrepareBindsAndOAC_Encode_Supported},
	{"TestPrepareBindsAndOAC_Encode_Errors", "unitary", false, TestPrepareBindsAndOAC_Encode_Errors},
	{"TestPushBindRowsIfAny_EncodeBindValues_Error", "unitary", false, TestPushBindRowsIfAny_EncodeBindValues_Error},
	{"TestPushBindRowsIfAny_GetMessage_RXD_Error", "unitary", false, TestPushBindRowsIfAny_GetMessage_RXD_Error},
	{"TestPushBindRowsIfAny_Push_RXD_Error", "unitary", false, TestPushBindRowsIfAny_Push_RXD_Error},
	{"TestPrepareBindsAndOAC_OAC_Supported", "unitary", false, TestPrepareBindsAndOAC_OAC_Supported},
	{"TestPrepareBindsAndOAC_OAC_Errors", "unitary", false, TestPrepareBindsAndOAC_OAC_Errors},
	{"TestStatementExecutor_DML_Insert_Prepared_MarshalAndExec", "unitary", false, TestStatementExecutor_DML_Insert_Prepared_MarshalAndExec},
	{"TestStatementExecutor_Select_Prepared_MarshalAndQuery", "unitary", false, TestStatementExecutor_Select_Prepared_MarshalAndQuery},

	{"TestOexfen_New_Getters", "unitary", false, TestOexfen_New_Getters},
	{"TestOexfen_MarshalTo_MatchesGolden", "unitary", false, TestOexfen_MarshalTo_MatchesGolden},
	{"TestOexfen_MarshalTo_Exeflg_NoCommit_And_Commit", "unitary", false, TestOexfen_MarshalTo_Exeflg_NoCommit_And_Commit},
	{"TestOexfen_MarshalTo_Failures", "unitary", false, TestOexfen_MarshalTo_Failures},

	{"TestLobExecutor_GetChunkSize", "unitary", false, TestLobExecutor_GetChunkSize},
	{"TestClobExecutor_GetChunkSizeErrors", "unitary", false, TestClobExecutor_GetChunkSizeErrors},
	{"TestClobExecutor_CreateTemporaryLob", "unitary", false, TestClobExecutor_CreateTemporaryLob},
	{"TestClobExecutor_CreateTemporaryLobErrors", "unitary", false, TestClobExecutor_CreateTemporaryLobErrors},
	{"TestClobExecutor_Write", "unitary", false, TestClobExecutor_Write},
	{"TestClobExecutor_WriteErrors", "unitary", false, TestClobExecutor_WriteErrors},
	{"TestClobExecutor_Read", "unitary", false, TestClobExecutor_Read},
	{"TestClobExecutor_ReadErrors", "unitary", false, TestClobExecutor_ReadErrors},
	{"TestClobExecutor_IsOpen", "unitary", false, TestClobExecutor_IsOpen},
	{"TestClobExecutor_IsOpenErrors", "unitary", false, TestClobExecutor_IsOpenErrors},
	{"TestClobExecutor_GetLength", "unitary", false, TestClobExecutor_GetLength},
	{"TestClobExecutor_GetLengthErrors", "unitary", false, TestClobExecutor_GetLengthErrors},
	{"TestClobExecutor_Trim", "unitary", false, TestClobExecutor_Trim},
	{"TestClobExecutor_TrimErrors", "unitary", false, TestClobExecutor_TrimErrors},
	{"TestLobExecutor_ConsumeLobResponses_DelayedOER", "unitary", false, TestLobExecutor_ConsumeLobResponses_DelayedOER},
	{"TestLobExecutor_ConsumeLobResponses_OERTermination", "unitary", false, TestLobExecutor_ConsumeLobResponses_OERTermination},
	{"TestLobExecutor_ConsumeLobResponses_PullError", "unitary", false, TestLobExecutor_ConsumeLobResponses_PullError},
	{"TestClobExecutor_ReadNCLOB", "unitary", false, TestClobExecutor_ReadNCLOB},
	{"TestConnection_FaultyOnDrain", "unitary", false, TestConnection_FaultyOnDrain},
	{"TestConnection_FaultyOnDrainInStatement", "unitary", false, TestConnection_FaultyOnDrainInStatement},
	{"TestClobExecutor_WriteNCLOB", "unitary", false, TestClobExecutor_WriteNCLOB},

	{"TestConnection_InvalidateOnOEROrSTA", "unitary", false, TestConnection_InvalidateOnOEROrSTA},
	{"TestTransactionCommitSuccess", "unitary", false, TestTransactionCommitSuccess},
	{"TestTransactionRollbackSuccess", "unitary", false, TestTransactionRollbackSuccess},
	{"TestCallBeginTxTwice", "unitary", false, TestCallBeginTxTwice},

	{"TestStatementExecutorExec_HandleRXDRow_UsesScannerDestination", "unitary", false, TestStatementExecutorExec_HandleRXDRow_UsesScannerDestination},
	{"TestStatementExecutorExec_HandleRXDRow_PropagatesScannerError", "unitary", false, TestStatementExecutorExec_HandleRXDRow_PropagatesScannerError},
	{"TestNormalizeBindValue_SQLNullTypes", "unitary", false, TestNormalizeBindValue_SQLNullTypes},
	{"TestUnmarshalCLRColumnDataRejectsInvalidLongChunkLengths", "unitary", false, TestUnmarshalCLRColumnDataRejectsInvalidLongChunkLengths},
	{"TestNeedToSendOACs_CountChanged", "unitary", false, TestNeedToSendOACs_CountChanged},

	{"TestAuthRPARejectsOversizedKeyValueListAllocations", "unitary", false, TestAuthRPARejectsOversizedKeyValueListAllocations},
	{"TestCodecFactory_GetBindOac", "unitary", false, TestCodecFactory_GetBindOac},
	{"TestCodecFactory_GetDefineOac", "unitary", false, TestCodecFactory_GetDefineOac},
	{"TestConnectionResetter_Reset", "unitary", false, TestConnectionResetter_Reset},
	{"TestConnection_ExecContext_LocalizesError", "unitary", false, TestConnection_ExecContext_LocalizesError},
	{"TestConnection_LocalizationStaysBoundToEachShelf", "unitary", false, TestConnection_LocalizationStaysBoundToEachShelf},
	{"TestConnection_ParseTimeZoneRejectsMalformedValues", "unitary", false, TestConnection_ParseTimeZoneRejectsMalformedValues},
	{"TestConnection_QueryContext_LocalizesError", "unitary", false, TestConnection_QueryContext_LocalizesError},
	{"TestConnection_cancelCurrentExecution", "unitary", false, TestConnection_cancelCurrentExecution},
	{"TestEncryptPasswordBufferTooSmall", "unitary", false, TestEncryptPasswordBufferTooSmall},
	{"TestGetConnectionMissingLocalizationService", "unitary", false, TestGetConnectionMissingLocalizationService},
	{"TestGetMaxLengthForOac_NoPreviousOACs", "unitary", false, TestGetMaxLengthForOac_NoPreviousOACs},
	{"TestGetMaxLengthForOac_PreservesPreviousLarger", "unitary", false, TestGetMaxLengthForOac_PreservesPreviousLarger},
	{"TestGetMaxLengthForOac_UsesCurrentIfLarger", "unitary", false, TestGetMaxLengthForOac_UsesCurrentIfLarger},
	{"TestHandleRXDRow_AssignsDecodedValue", "unitary", false, TestHandleRXDRow_AssignsDecodedValue},
	{"TestHandleRXDRow_MoreDestsThanReturnedValues", "unitary", false, TestHandleRXDRow_MoreDestsThanReturnedValues},
	{"TestHandleRXDRow_NilDestinationSkipped", "unitary", false, TestHandleRXDRow_NilDestinationSkipped},
	{"TestHandleRXDRow_NilWireValue_SkipsAssignment", "unitary", false, TestHandleRXDRow_NilWireValue_SkipsAssignment},
	{"TestHandleRXDRow_RawBytes_AssignedToByteSlice", "unitary", false, TestHandleRXDRow_RawBytes_AssignedToByteSlice},
	{"TestInitExecRunner_InOutBinds_Counted", "unitary", false, TestInitExecRunner_InOutBinds_Counted},
	{"TestInitExecRunner_NoOutBinds_FlagsNotSet", "unitary", false, TestInitExecRunner_NoOutBinds_FlagsNotSet},
	{"TestInitExecRunner_Reset", "unitary", false, TestInitExecRunner_Reset},
	{"TestInitExecRunner_WithOutBinds_FlagsSet", "unitary", false, TestInitExecRunner_WithOutBinds_FlagsSet},
	{"TestKeywordValueArrayRejectsOversizedDynamicValue", "unitary", false, TestKeywordValueArrayRejectsOversizedDynamicValue},
	{"TestKeywordValueArrayRejectsOversizedPairCount", "unitary", false, TestKeywordValueArrayRejectsOversizedPairCount},
	{"TestMessageStreamer_PullReturnsErrorForNonUnmarshallableFunction", "unitary", false, TestMessageStreamer_PullReturnsErrorForNonUnmarshallableFunction},
	{"TestNeedToSendOACs_LengthIncreased", "unitary", false, TestNeedToSendOACs_LengthIncreased},
	{"TestNeedToSendOACs_NoPreviousOACs", "unitary", false, TestNeedToSendOACs_NoPreviousOACs},
	{"TestNeedToSendOACs_SameOAC_ReturnsFalse", "unitary", false, TestNeedToSendOACs_SameOAC_ReturnsFalse},
	{"TestNeedToSendOACs_TypeChanged", "unitary", false, TestNeedToSendOACs_TypeChanged},
	{"TestNewConnectionReturnsServerTimezoneError", "unitary", false, TestNewConnectionReturnsServerTimezoneError},
	{"TestPasswordAuthenticatorValidatePasswordLength", "unitary", false, TestPasswordAuthenticatorValidatePasswordLength},
	{"TestStatementCancellationCleanupReleasesStartedAfterFunc", "unitary", false, TestStatementCancellationCleanupReleasesStartedAfterFunc},
	{"TestStatementExecContextTransactionCancellationBeforeSetup", "unitary", false, TestStatementExecContextTransactionCancellationBeforeSetup},
	{"TestStatementExecutor_Select_SuccessOERWithoutDCB", "unitary", false, TestStatementExecutor_Select_SuccessOERWithoutDCB},
	{"TestStatementHandleContextCancelledRunsBreakReset", "unitary", false, TestStatementHandleContextCancelledRunsBreakReset},
	{"TestStatementQueryContextLocalization", "unitary", false, TestStatementQueryContextLocalization},
	{"TestTTCRowsColumnTypeScanType", "unitary", false, TestTTCRowsColumnTypeScanType},
	{"TestTTCRowsImplementsColumnTypeInterfaces", "unitary", false, TestTTCRowsImplementsColumnTypeInterfaces},
	{"TestTTILobRpa_SetDefinition_NilDefinition", "unitary", false, TestTTILobRpa_SetDefinition_NilDefinition},
	{"TestTTILobRpa_UnMarshalFrom_Failure", "unitary", false, TestTTILobRpa_UnMarshalFrom_Failure},
	{"TestTTILobRpa_UnMarshalFrom_Success", "unitary", false, TestTTILobRpa_UnMarshalFrom_Success},
	{"TestTTILobd_MarshalTo_Fail", "unitary", false, TestTTILobd_MarshalTo_Fail},
	{"TestTTILobd_MarshalTo_Success", "unitary", false, TestTTILobd_MarshalTo_Success},
	{"TestTTILobd_UnMarshalFrom_Fail", "unitary", false, TestTTILobd_UnMarshalFrom_Fail},
	{"TestTTILobd_UnMarshalFrom_Success", "unitary", false, TestTTILobd_UnMarshalFrom_Success},
	{"TestTTIShelf_LocalizedStatementExecError", "unitary", false, TestTTIShelf_LocalizedStatementExecError},
	{"TestTTIShelf_StatementDrain", "unitary", false, TestTTIShelf_StatementDrain},
	{"TestTTIlob_GetFuncCode", "unitary", false, TestTTIlob_GetFuncCode},
	{"TestTTIlob_GetMsgCode", "unitary", false, TestTTIlob_GetMsgCode},
	{"TestTTIlob_MarshalTo_Fail", "unitary", false, TestTTIlob_MarshalTo_Fail},
	{"TestTTIlob_MarshalTo_Success", "unitary", false, TestTTIlob_MarshalTo_Success},
	{"TestTTIlob_MarshalTo_WithoutDefinition", "unitary", false, TestTTIlob_MarshalTo_WithoutDefinition},
	{"TestTTIlob_New", "unitary", false, TestTTIlob_New},
	{"TestTTIlob_SetDefinition", "unitary", false, TestTTIlob_SetDefinition},
	{"TestTTIlob_SetDefinition_NilDefinition", "unitary", false, TestTTIlob_SetDefinition_NilDefinition},
	{"TestTTIoer_getConnectionShouldBeDropped", "unitary", false, TestTTIoer_getConnectionShouldBeDropped},
	{"TestTTIrxd_MarshalTo_LargeCLR", "unitary", false, TestTTIrxd_MarshalTo_LargeCLR},
	{"TestTTIrxd_ProcessDMLPlSqlIndicator_ReadError", "unitary", false, TestTTIrxd_ProcessDMLPlSqlIndicator_ReadError},
	{"TestTTIrxd_SetNumberofReturningArgs_SwitchesMode", "unitary", false, TestTTIrxd_SetNumberofReturningArgs_SwitchesMode},
	{"TestTTIrxd_UnMarshalFrom_Returning_DataReadError", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_DataReadError},
	{"TestTTIrxd_UnMarshalFrom_Returning_IndicatorReadError", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_IndicatorReadError},
	{"TestTTIrxd_UnMarshalFrom_Returning_MultipleRows", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_MultipleRows},
	{"TestTTIrxd_UnMarshalFrom_Returning_NullValue", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_NullValue},
	{"TestTTIrxd_UnMarshalFrom_Returning_RowCountReadError", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_RowCountReadError},
	{"TestTTIrxd_UnMarshalFrom_Returning_SinglePosition_SingleRow", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_SinglePosition_SingleRow},
	{"TestTTIrxd_UnMarshalFrom_Returning_ThreePositionsMixed", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_ThreePositionsMixed},
	{"TestTTIrxd_UnMarshalFrom_Returning_TwoPositions_SingleRowEach", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_TwoPositions_SingleRowEach},
	{"TestTTIrxd_UnMarshalFrom_Returning_ZeroRowsForPosition", "unitary", false, TestTTIrxd_UnMarshalFrom_Returning_ZeroRowsForPosition},
	{"TestTTIwrnUnMarshalFrom", "unitary", false, TestTTIwrnUnMarshalFrom},
	{"TestTTIwrnUnMarshalFromRejectsTruncatedMessage", "unitary", false, TestTTIwrnUnMarshalFromRejectsTruncatedMessage},
	{"TestUnmarshalCLRColumnData", "unitary", false, TestUnmarshalCLRColumnData},
	{"TestUnmarshalCLRColumnDataRejectsAggregateLengthAboveMaximum", "unitary", false, TestUnmarshalCLRColumnDataRejectsAggregateLengthAboveMaximum},
	{"Test_defaultNumericValue", "unitary", false, Test_defaultNumericValue},
	{"Test_defaultValueForNull", "unitary", false, Test_defaultValueForNull},
	{"Test_defaultValueForNullUnknown", "unitary", false, Test_defaultValueForNullUnknown},
	{"Test_ttcRows_handleNullDefaulting", "unitary", false, Test_ttcRows_handleNullDefaulting},
	{"Test_ttcRows_handleNullStrict", "unitary", false, Test_ttcRows_handleNullStrict},
	{"Test_ttiSTA_getConnectionShouldBeDropped", "unitary", false, Test_ttiSTA_getConnectionShouldBeDropped},

	{"TestTypeRep_UnMarshalFrom_TooManyTypeRepresentations", "unitary", false, TestTypeRep_UnMarshalFrom_TooManyTypeRepresentations},
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
func NewMarshalEngineTest(byteOrder session.ByteOrder, typ byte, rep byte, bufSize int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	dataBuffer := NewArrayDataBuffer(bufSize)
	typeRep := NewTypeRep()
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
		return NewMarshalEngine(faulty, session.BIG_ENDIAN, rep)
	}
	tdb := NewTestDataBuffer()
	tdb.WriteBytesWithContext(context.Background(), buf)
	mar := NewMarshalEngine(tdb, session.BIG_ENDIAN, rep)
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
	pullCalled bool
	pullTypes  []common.MessageType
	pullMsg    common.Message[common.MessageType]
	pullErr    error
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
	return m.pullMsg, m.pullErr
}

func (m *mockStreamer) Flush(_ context.Context) error {
	return nil
}

func (m *mockStreamer) Drain(_ context.Context, direction common.StreamDirection) (int, int) {
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
}

// newTestConnection creates a connection without querying DBTIMEZONE. Tests that
// exercise connection behavior independently of initialization use this helper.
func newTestConnection(
	shelf *ttiShelf[common.MessageType],
	sessCtx *common.SessionContext,
	ns common.NetworkSession,
) *Connection {
	conn := &Connection{
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
