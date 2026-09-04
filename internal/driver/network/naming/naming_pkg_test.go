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

package naming

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
	{Name: "TestNewConnectionIterator_Basic", Categories: "unitary", Exclusive: false, Fn: TestNewConnectionIterator_Basic},
	{Name: "TestConnectionIterator_Next_Basic", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Next_Basic},
	{Name: "TestExtractDescription_DefaultSecurity", Categories: "unitary", Exclusive: false, Fn: TestExtractDescription_DefaultSecurity},
	{Name: "TestConnectionIterator_HasNext", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_HasNext},
	{Name: "TestConnectionIterator_Reset", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Reset},
	{Name: "TestConnectionIterator_Remaining_And_Total", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Remaining_And_Total},
	{Name: "TestConnectionIterator_ConnectStringFormat", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_ConnectStringFormat},
	{Name: "TestConnectionIterator_RetryCount_Single", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryCount_Single},
	{Name: "TestConnectionIterator_RetryCount_Multiple_Addresses", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryCount_Multiple_Addresses},
	{Name: "TestConnectionIterator_RetryCount_Zero", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryCount_Zero},
	{Name: "TestConnectionIterator_RetryCount_Negative", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryCount_Negative},
	{Name: "TestConnectionIterator_RetryDelay_Configuration", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryDelay_Configuration},
	{Name: "TestConnectionIterator_RetryDelay_Zero", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryDelay_Zero},
	{Name: "TestConnectionIterator_RetryDelay_ActualWait", Categories: "functional", Exclusive: false, Fn: TestConnectionIterator_RetryDelay_ActualWait},
	{Name: "TestConnectionIterator_RetryDelay_ContextCancellation", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RetryDelay_ContextCancellation},
	{Name: "TestConnectionIterator_Failover_DescriptionList_Enabled", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_DescriptionList_Enabled},
	{Name: "TestIterator_DescriptionList_RootMissing", Categories: "unitary", Exclusive: false, Fn: TestIterator_DescriptionList_RootMissing},
	{Name: "TestIterator_ExtractConnectDataNode_Cases", Categories: "unitary", Exclusive: false, Fn: TestIterator_ExtractConnectDataNode_Cases},
	{Name: "TestIterator_buildFromAddresses_Empty", Categories: "unitary", Exclusive: false, Fn: TestIterator_buildFromAddresses_Empty},
	{Name: "TestIterator_extractConnectDataNode_GetNodePath", Categories: "unitary", Exclusive: false, Fn: TestIterator_extractConnectDataNode_GetNodePath},
	{Name: "TestIterator_findDescriptionNode_WrongRootAndOutOfRange", Categories: "unitary", Exclusive: false, Fn: TestIterator_findDescriptionNode_WrongRootAndOutOfRange},
	{Name: "TestIterator_Remaining_ExhaustedEarlyReturn", Categories: "unitary", Exclusive: false, Fn: TestIterator_Remaining_ExhaustedEarlyReturn},
	{Name: "TestConnectionIterator_Failover_DescriptionList_Disabled", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_DescriptionList_Disabled},
	{Name: "TestConnectionIterator_Failover_DescriptionList_Default", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_DescriptionList_Default},
	{Name: "TestConnectionIterator_Failover_Description_Enabled", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_Description_Enabled},
	{Name: "TestConnectionIterator_Failover_Description_Disabled", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_Description_Disabled},
	{Name: "TestConnectionIterator_Failover_Mixed_Settings", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_Mixed_Settings},
	{Name: "TestConnectionIterator_LoadBalance_DescriptionList", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_LoadBalance_DescriptionList},
	{Name: "TestConnectionIterator_LoadBalance_Description", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_LoadBalance_Description},
	{Name: "TestConnectionIterator_LoadBalance_Off", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_LoadBalance_Off},
	{Name: "TestConnectionIterator_Complex_Scenario_All_Features", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Complex_Scenario_All_Features},
	{Name: "TestConnectionIterator_Failover_And_Retry_Combined", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Failover_And_Retry_Combined},
	{Name: "TestConnectionIterator_LoadBalance_And_Failover_Disabled", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_LoadBalance_And_Failover_Disabled},
	{Name: "TestConnectionIterator_CollectAllAddresses", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_CollectAllAddresses},
	{Name: "TestConnectionIterator_MultipleAddressLists", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_MultipleAddressLists},
	{Name: "TestConnectionIterator_SingleAddress", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_SingleAddress},
	{Name: "TestConnectionIterator_AddressesOnly", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_AddressesOnly},
	{Name: "TestConnectionIterator_AddressesOnly_ConnectStringStructure", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_AddressesOnly_ConnectStringStructure},
	{Name: "TestConnectionIterator_DescriptionList_Basic", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_DescriptionList_Basic},
	{Name: "TestConnectionIterator_DescriptionList_MultipleAddresses", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_DescriptionList_MultipleAddresses},
	{Name: "TestConnectionIterator_Empty_Context", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Empty_Context},
	{Name: "TestConnectionIterator_NoConnectData", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_NoConnectData},
	{Name: "TestConnectionIterator_ZeroAddressesAfterFailover", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_ZeroAddressesAfterFailover},
	{Name: "TestConnectionIterator_NilRootNode", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_NilRootNode},
	{Name: "TestConnectionIterator_NilDescription", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_NilDescription},
	{Name: "TestConnectionIterator_NilDescriptionList", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_NilDescriptionList},
	{Name: "TestConnectionIterator_MissingAddressFields", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_MissingAddressFields},
	{Name: "TestConnectionIterator_MixedValidInvalidAddresses", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_MixedValidInvalidAddresses},
	{Name: "TestConnectionIterator_InvalidConnectionString", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_InvalidConnectionString},
	{Name: "TestConnectionIterator_ParamsAssociationPerAddress", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_ParamsAssociationPerAddress},
	{Name: "TestConnectionIterator_DifferentConnectDataPerDescription", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_DifferentConnectDataPerDescription},
	{Name: "TestConnectionIterator_LargeNumberOfAttempts", Categories: "unitary", Exclusive: true, Fn: TestConnectionIterator_LargeNumberOfAttempts},
	{Name: "TestConnectionIterator_VeryLargeRetryCount", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_VeryLargeRetryCount},
	{Name: "TestConnectionIterator_Reset_WithLoadBalance", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Reset_WithLoadBalance},
	{Name: "TestConnectionIterator_Reset_PreservesTotal", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Reset_PreservesTotal},
	{Name: "TestConnectionIterator_MultipleResets", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_MultipleResets},
	{Name: "TestConnectionIterator_ExhaustionBehavior", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_ExhaustionBehavior},
	{Name: "TestConnectionIterator_SingleAttemptMultipleCalls", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_SingleAttemptMultipleCalls},
	{Name: "TestConnectionIterator_ManyDescriptions", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_ManyDescriptions},
	{Name: "TestConnectionIterator_ManyAddresses", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_ManyAddresses},
	{Name: "TestConnectionIterator_RoundRobin", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_RoundRobin},
	{Name: "TestParseSimple", Categories: "unitary", Exclusive: false, Fn: TestParseSimple},
	{Name: "TestParseComplex", Categories: "unitary", Exclusive: false, Fn: TestParseComplex},
	{Name: "TestParseSimpleWithConnectionProperties", Categories: "unitary", Exclusive: false, Fn: TestParseSimpleWithConnectionProperties},
	{Name: "TestParseEmpty", Categories: "unitary", Exclusive: false, Fn: TestParseEmpty},
	{Name: "TestParseWhitespaceOnly", Categories: "unitary", Exclusive: false, Fn: TestParseWhitespaceOnly},
	{Name: "TestParseInvalid", Categories: "unitary", Exclusive: false, Fn: TestParseInvalid},
	{Name: "TestGetValue", Categories: "unitary", Exclusive: false, Fn: TestGetValue},
	{Name: "TestGetValueNotFound", Categories: "unitary", Exclusive: false, Fn: TestGetValueNotFound},
	{Name: "TestGetNode", Categories: "unitary", Exclusive: false, Fn: TestGetNode},
	{Name: "TestToString", Categories: "unitary", Exclusive: false, Fn: TestToString},
	{Name: "TestChildCount", Categories: "unitary", Exclusive: false, Fn: TestChildCount},
	{Name: "TestGetChild", Categories: "unitary", Exclusive: false, Fn: TestGetChild},
	{Name: "TestParseUnexpectedOpeningParenWithoutName", Categories: "unitary", Exclusive: false, Fn: TestParseUnexpectedOpeningParenWithoutName},
	{Name: "TestParseUnexpectedClosingParen", Categories: "unitary", Exclusive: false, Fn: TestParseUnexpectedClosingParen},
	{Name: "TestParseUnexpectedToken", Categories: "unitary", Exclusive: false, Fn: TestParseUnexpectedToken},
	{Name: "TestGetValueNonLeafNode", Categories: "unitary", Exclusive: false, Fn: TestGetValueNonLeafNode},
	{Name: "TestGetNodeEmptyPath", Categories: "unitary", Exclusive: false, Fn: TestGetNodeEmptyPath},
	{Name: "TestGetNodeWrongRootName", Categories: "unitary", Exclusive: false, Fn: TestGetNodeWrongRootName},
	{Name: "TestGetNodeEmptySegment", Categories: "unitary", Exclusive: false, Fn: TestGetNodeEmptySegment},
	{Name: "TestGetChildInvalidIndex", Categories: "unitary", Exclusive: false, Fn: TestGetChildInvalidIndex},
	{Name: "TestGetNodeRootPath", Categories: "unitary", Exclusive: false, Fn: TestGetNodeRootPath},
	{Name: "TestResolveConnectStringUrl_EZConnect", Categories: "unitary", Exclusive: false, Fn: TestResolveConnectStringUrl_EZConnect},
	{Name: "TestResolveConnectStringUrl_TNS", Categories: "unitary", Exclusive: false, Fn: TestResolveConnectStringUrl_TNS},
	{Name: "TestParseDSNString_InvalidInputs", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_InvalidInputs},
	{Name: "TestParseDSNString_LongTNS_WithProperties_CleanConnectString", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_LongTNS_WithProperties_CleanConnectString},
	{Name: "TestParseDSNString_ParseError", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_ParseError},
	{Name: "TestParseDSNString_ExtractContextError", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_ExtractContextError},
	{Name: "TestParseDSNString_ConversionError", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_ConversionError},
	{Name: "TestParseDSNString_EmptyPassword", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_EmptyPassword},
	{Name: "TestParseIterative_NoTokens", Categories: "unitary", Exclusive: false, Fn: TestParseIterative_NoTokens},
	{Name: "TestExtractConnectionContext_Success", Categories: "unitary", Exclusive: false, Fn: TestExtractConnectionContext_Success},
	{Name: "TestExtractConnectionContext_Errors", Categories: "unitary", Exclusive: false, Fn: TestExtractConnectionContext_Errors},
	{Name: "TestExtractDescription_Complete", Categories: "unitary", Exclusive: false, Fn: TestExtractDescription_Complete},
	{Name: "TestExtractDescription_CompressionLevels", Categories: "unitary", Exclusive: false, Fn: TestExtractDescription_CompressionLevels},
	{Name: "TestExtractAddressList_FullEmptyAndErrors", Categories: "unitary", Exclusive: false, Fn: TestExtractAddressList_FullEmptyAndErrors},
	{Name: "TestExtractDescriptionList_FullEmptyAndErrors", Categories: "unitary", Exclusive: false, Fn: TestExtractDescriptionList_FullEmptyAndErrors},
	{Name: "TestExtractAddress_SuccessAndError", Categories: "unitary", Exclusive: false, Fn: TestExtractAddress_SuccessAndError},
	{Name: "TestExtractConnectData_SuccessEmptyAndError", Categories: "unitary", Exclusive: false, Fn: TestExtractConnectData_SuccessEmptyAndError},
	{Name: "TestExtractSecurity_SuccessEmptyAndError", Categories: "unitary", Exclusive: false, Fn: TestExtractSecurity_SuccessEmptyAndError},
	{Name: "TestHelpersAndMethods", Categories: "unitary", Exclusive: false, Fn: TestHelpersAndMethods},
	{Name: "TestExtractDescription_ErrorPropagation", Categories: "unitary", Exclusive: false, Fn: TestExtractDescription_ErrorPropagation},
	{Name: "TestCoverage_MissingFlags", Categories: "unitary", Exclusive: false, Fn: TestCoverage_MissingFlags},
	{Name: "TestParseEzConnect_SimpleHostAndService", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_SimpleHostAndService},
	{Name: "TestParseEzConnect_DefaultPort", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_DefaultPort},
	{Name: "TestParseEzConnect_InvalidPorts", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_InvalidPorts},
	{Name: "TestParseEzConnect_ValidPort_Boundary", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_ValidPort_Boundary},
	{Name: "TestParseEzConnect_Protocols", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_Protocols},
	{Name: "TestParseEzConnect_InvalidProtocol", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_InvalidProtocol},
	{Name: "TestParseEzConnect_MultipleHosts", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_MultipleHosts},
	{Name: "TestParseEzConnect_MultipleAddressGroups", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_MultipleAddressGroups},
	{Name: "TestParseEzConnect_ServerModeAndInstance", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_ServerModeAndInstance},
	{Name: "TestParseEzConnect_ExtendedParams", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_ExtendedParams},
	{Name: "TestParseEzConnect_QuotedParamValue", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_QuotedParamValue},
	{Name: "TestParseEzConnect_ParameterAliases", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_ParameterAliases},
	{Name: "TestParseEzConnect_HTTPSProxy", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_HTTPSProxy},
	{Name: "TestParseEzConnect_IPv6Address", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_IPv6Address},
	{Name: "TestParseEzConnect_IPv6WithMultipleHosts", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_IPv6WithMultipleHosts},
	{Name: "TestParseEzConnect_EmptyServiceVariants", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_EmptyServiceVariants},
	{Name: "TestParseEzConnect_WhitespaceRemoval", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_WhitespaceRemoval},
	{Name: "TestParseEzConnect_EmptyURL", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_EmptyURL},
	{Name: "TestParseEzConnect_SecurityParams", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_SecurityParams},
	{Name: "TestParseEzConnect_ConnectionPoolParams", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_ConnectionPoolParams},
	{Name: "TestParseEzConnect_ComplexMultiHostScenario", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_ComplexMultiHostScenario},
	{Name: "TestParseExtendedParams_InvalidFormat", Categories: "unitary", Exclusive: false, Fn: TestParseExtendedParams_InvalidFormat},
	{Name: "TestParseExtendedParams_EmptyString", Categories: "unitary", Exclusive: false, Fn: TestParseExtendedParams_EmptyString},
	{Name: "TestParseExtendedParams_MultipleParams", Categories: "unitary", Exclusive: false, Fn: TestParseExtendedParams_MultipleParams},
	{Name: "TestParseEzConnect_InvalidExtendedParams", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_InvalidExtendedParams},
	{Name: "TestParseEzConnect_EmptyAddressGroups", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_EmptyAddressGroups},
	{Name: "TestParseHostList_TrailingComma", Categories: "unitary", Exclusive: false, Fn: TestParseHostList_TrailingComma},
	{Name: "TestParseHostList_MultipleHosts", Categories: "unitary", Exclusive: false, Fn: TestParseHostList_MultipleHosts},
	{Name: "TestParseHostList_EmptyHost", Categories: "unitary", Exclusive: false, Fn: TestParseHostList_EmptyHost},
	{Name: "TestParseAddressGroups_MultipleGroups", Categories: "unitary", Exclusive: false, Fn: TestParseAddressGroups_MultipleGroups},
	{Name: "TestParseAddressGroups_EmptyString", Categories: "unitary", Exclusive: false, Fn: TestParseAddressGroups_EmptyString},
	{Name: "TestBuildAddress_TCPWithoutProxy", Categories: "unitary", Exclusive: false, Fn: TestBuildAddress_TCPWithoutProxy},
	{Name: "TestBuildAddress_TCPSWithProxy", Categories: "unitary", Exclusive: false, Fn: TestBuildAddress_TCPSWithProxy},
	{Name: "TestBuildAddress_IPv6", Categories: "unitary", Exclusive: false, Fn: TestBuildAddress_IPv6},
	{Name: "TestBuildConnectData_AllParams", Categories: "unitary", Exclusive: false, Fn: TestBuildConnectData_AllParams},
	{Name: "TestBuildConnectData_EmptyService", Categories: "unitary", Exclusive: false, Fn: TestBuildConnectData_EmptyService},
	{Name: "TestBuildSecurity_AllParams", Categories: "unitary", Exclusive: false, Fn: TestBuildSecurity_AllParams},
	{Name: "TestBuildSecurity_Empty", Categories: "unitary", Exclusive: false, Fn: TestBuildSecurity_Empty},
	{Name: "TestBuildDescriptionParams_AutoLoadBalance", Categories: "unitary", Exclusive: false, Fn: TestBuildDescriptionParams_AutoLoadBalance},
	{Name: "TestBuildDescriptionParams_ExplicitLoadBalance", Categories: "unitary", Exclusive: false, Fn: TestBuildDescriptionParams_ExplicitLoadBalance},
	{Name: "TestBuildAddressList_SingleGroup", Categories: "unitary", Exclusive: false, Fn: TestBuildAddressList_SingleGroup},
	{Name: "TestBuildAddressList_MultipleGroups", Categories: "unitary", Exclusive: false, Fn: TestBuildAddressList_MultipleGroups},
	{Name: "TestSplitAndParseExtendedParams_WithParams", Categories: "unitary", Exclusive: false, Fn: TestSplitAndParseExtendedParams_WithParams},
	{Name: "TestParseMainURL_AllComponents", Categories: "unitary", Exclusive: false, Fn: TestParseMainURL_AllComponents},
	{Name: "TestParseMainURL_MinimalURL", Categories: "unitary", Exclusive: false, Fn: TestParseMainURL_MinimalURL},
	{Name: "TestParseHostList_DefaultPort_InMiddle", Categories: "unitary", Exclusive: false, Fn: TestParseHostList_DefaultPort_InMiddle},
	{Name: "TestParseEzConnect_HTTPSProxyWithoutPort", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_HTTPSProxyWithoutPort},
	{Name: "TestSplitAndParseExtendedParams_ParensBeforeParams", Categories: "unitary", Exclusive: false, Fn: TestSplitAndParseExtendedParams_ParensBeforeParams},
	{Name: "TestConnectionIterator_Attempt_EZConnect_PreservesQuotedWalletLocation", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Attempt_EZConnect_PreservesQuotedWalletLocation},
	{Name: "TestConnectionIterator_Attempt_LongTNS_PreservesQuotedServerCertDN", Categories: "unitary", Exclusive: false, Fn: TestConnectionIterator_Attempt_LongTNS_PreservesQuotedServerCertDN},
	{Name: "TestParseDSNString_CommaBetweenNodes_FailsFast", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_CommaBetweenNodes_FailsFast},
	{Name: "TestParseDSNString_EZConnect_Quotes_Spaces", Categories: "unitary", Exclusive: false, Fn: TestParseDSNString_EZConnect_Quotes_Spaces},
	{Name: "TestParseEzConnect_Whitespace_Removal_Preserve", Categories: "unitary", Exclusive: false, Fn: TestParseEzConnect_Whitespace_Removal_Preserve},
	{Name: "TestParse_PreservesQuotedValues", Categories: "unitary", Exclusive: false, Fn: TestParse_PreservesQuotedValues},
	{Name: "TestResolveAddresses_ContextHandling", Categories: "unitary", Exclusive: false, Fn: TestResolveAddresses_ContextHandling},
	{Name: "TestResolveAddresses_DNSExpansion", Categories: "unitary", Exclusive: false, Fn: TestResolveAddresses_DNSExpansion},
	{Name: "TestResolveAddresses_HostClassification", Categories: "unitary", Exclusive: false, Fn: TestResolveAddresses_HostClassification},
}
