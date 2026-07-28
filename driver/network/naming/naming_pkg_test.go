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
	{"TestNewConnectionIterator_Basic", "unitary", false, TestNewConnectionIterator_Basic},
	{"TestConnectionIterator_Next_Basic", "unitary", false, TestConnectionIterator_Next_Basic},
	{"TestExtractDescription_DefaultSecurity", "unitary", false, TestExtractDescription_DefaultSecurity},
	{"TestConnectionIterator_HasNext", "unitary", false, TestConnectionIterator_HasNext},
	{"TestConnectionIterator_Reset", "unitary", false, TestConnectionIterator_Reset},
	{"TestConnectionIterator_Remaining_And_Total", "unitary", false, TestConnectionIterator_Remaining_And_Total},
	{"TestConnectionIterator_ConnectStringFormat", "unitary", false, TestConnectionIterator_ConnectStringFormat},
	{"TestConnectionIterator_RetryCount_Single", "unitary", false, TestConnectionIterator_RetryCount_Single},
	{"TestConnectionIterator_RetryCount_Multiple_Addresses", "unitary", false, TestConnectionIterator_RetryCount_Multiple_Addresses},
	{"TestConnectionIterator_RetryCount_Zero", "unitary", false, TestConnectionIterator_RetryCount_Zero},
	{"TestConnectionIterator_RetryCount_Negative", "unitary", false, TestConnectionIterator_RetryCount_Negative},
	{"TestConnectionIterator_RetryDelay_Configuration", "unitary", false, TestConnectionIterator_RetryDelay_Configuration},
	{"TestConnectionIterator_RetryDelay_Zero", "unitary", false, TestConnectionIterator_RetryDelay_Zero},
	{"TestConnectionIterator_RetryDelay_ActualWait", "functional", false, TestConnectionIterator_RetryDelay_ActualWait},
	{"TestConnectionIterator_RetryDelay_ContextCancellation", "unitary", false, TestConnectionIterator_RetryDelay_ContextCancellation},
	{"TestConnectionIterator_Failover_DescriptionList_Enabled", "unitary", false, TestConnectionIterator_Failover_DescriptionList_Enabled},
	{"TestIterator_DescriptionList_RootMissing", "unitary", false, TestIterator_DescriptionList_RootMissing},
	{"TestIterator_ExtractConnectDataNode_Cases", "unitary", false, TestIterator_ExtractConnectDataNode_Cases},
	{"TestIterator_buildFromAddresses_Empty", "unitary", false, TestIterator_buildFromAddresses_Empty},
	{"TestIterator_extractConnectDataNode_GetNodePath", "unitary", false, TestIterator_extractConnectDataNode_GetNodePath},
	{"TestIterator_findDescriptionNode_WrongRootAndOutOfRange", "unitary", false, TestIterator_findDescriptionNode_WrongRootAndOutOfRange},
	{"TestIterator_Remaining_ExhaustedEarlyReturn", "unitary", false, TestIterator_Remaining_ExhaustedEarlyReturn},
	{"TestConnectionIterator_Failover_DescriptionList_Disabled", "unitary", false, TestConnectionIterator_Failover_DescriptionList_Disabled},
	{"TestConnectionIterator_Failover_DescriptionList_Default", "unitary", false, TestConnectionIterator_Failover_DescriptionList_Default},
	{"TestConnectionIterator_Failover_Description_Enabled", "unitary", false, TestConnectionIterator_Failover_Description_Enabled},
	{"TestConnectionIterator_Failover_Description_Disabled", "unitary", false, TestConnectionIterator_Failover_Description_Disabled},
	{"TestConnectionIterator_Failover_Mixed_Settings", "unitary", false, TestConnectionIterator_Failover_Mixed_Settings},
	{"TestConnectionIterator_LoadBalance_DescriptionList", "unitary", false, TestConnectionIterator_LoadBalance_DescriptionList},
	{"TestConnectionIterator_LoadBalance_Description", "unitary", false, TestConnectionIterator_LoadBalance_Description},
	{"TestConnectionIterator_LoadBalance_Off", "unitary", false, TestConnectionIterator_LoadBalance_Off},
	{"TestConnectionIterator_Complex_Scenario_All_Features", "unitary", false, TestConnectionIterator_Complex_Scenario_All_Features},
	{"TestConnectionIterator_Failover_And_Retry_Combined", "unitary", false, TestConnectionIterator_Failover_And_Retry_Combined},
	{"TestConnectionIterator_LoadBalance_And_Failover_Disabled", "unitary", false, TestConnectionIterator_LoadBalance_And_Failover_Disabled},
	{"TestConnectionIterator_CollectAllAddresses", "unitary", false, TestConnectionIterator_CollectAllAddresses},
	{"TestConnectionIterator_MultipleAddressLists", "unitary", false, TestConnectionIterator_MultipleAddressLists},
	{"TestConnectionIterator_SingleAddress", "unitary", false, TestConnectionIterator_SingleAddress},
	{"TestConnectionIterator_AddressesOnly", "unitary", false, TestConnectionIterator_AddressesOnly},
	{"TestConnectionIterator_AddressesOnly_ConnectStringStructure", "unitary", false, TestConnectionIterator_AddressesOnly_ConnectStringStructure},
	{"TestConnectionIterator_DescriptionList_Basic", "unitary", false, TestConnectionIterator_DescriptionList_Basic},
	{"TestConnectionIterator_DescriptionList_MultipleAddresses", "unitary", false, TestConnectionIterator_DescriptionList_MultipleAddresses},
	{"TestConnectionIterator_Empty_Context", "unitary", false, TestConnectionIterator_Empty_Context},
	{"TestConnectionIterator_NoConnectData", "unitary", false, TestConnectionIterator_NoConnectData},
	{"TestConnectionIterator_ZeroAddressesAfterFailover", "unitary", false, TestConnectionIterator_ZeroAddressesAfterFailover},
	{"TestConnectionIterator_NilRootNode", "unitary", false, TestConnectionIterator_NilRootNode},
	{"TestConnectionIterator_NilDescription", "unitary", false, TestConnectionIterator_NilDescription},
	{"TestConnectionIterator_NilDescriptionList", "unitary", false, TestConnectionIterator_NilDescriptionList},
	{"TestConnectionIterator_MissingAddressFields", "unitary", false, TestConnectionIterator_MissingAddressFields},
	{"TestConnectionIterator_MixedValidInvalidAddresses", "unitary", false, TestConnectionIterator_MixedValidInvalidAddresses},
	{"TestConnectionIterator_InvalidConnectionString", "unitary", false, TestConnectionIterator_InvalidConnectionString},
	{"TestConnectionIterator_ParamsAssociationPerAddress", "unitary", false, TestConnectionIterator_ParamsAssociationPerAddress},
	{"TestConnectionIterator_DifferentConnectDataPerDescription", "unitary", false, TestConnectionIterator_DifferentConnectDataPerDescription},
	{"TestConnectionIterator_LargeNumberOfAttempts", "unitary", true, TestConnectionIterator_LargeNumberOfAttempts},
	{"TestConnectionIterator_VeryLargeRetryCount", "unitary", false, TestConnectionIterator_VeryLargeRetryCount},
	{"TestConnectionIterator_Reset_WithLoadBalance", "unitary", false, TestConnectionIterator_Reset_WithLoadBalance},
	{"TestConnectionIterator_Reset_PreservesTotal", "unitary", false, TestConnectionIterator_Reset_PreservesTotal},
	{"TestConnectionIterator_MultipleResets", "unitary", false, TestConnectionIterator_MultipleResets},
	{"TestConnectionIterator_ExhaustionBehavior", "unitary", false, TestConnectionIterator_ExhaustionBehavior},
	{"TestConnectionIterator_SingleAttemptMultipleCalls", "unitary", false, TestConnectionIterator_SingleAttemptMultipleCalls},
	{"TestConnectionIterator_ManyDescriptions", "unitary", false, TestConnectionIterator_ManyDescriptions},
	{"TestConnectionIterator_ManyAddresses", "unitary", false, TestConnectionIterator_ManyAddresses},
	{"TestConnectionIterator_RoundRobin", "unitary", false, TestConnectionIterator_RoundRobin},
	{"TestParseSimple", "unitary", false, TestParseSimple},
	{"TestParseComplex", "unitary", false, TestParseComplex},
	{"TestParseSimpleWithConnectionProperties", "unitary", false, TestParseSimpleWithConnectionProperties},
	{"TestParseEmpty", "unitary", false, TestParseEmpty},
	{"TestParseWhitespaceOnly", "unitary", false, TestParseWhitespaceOnly},
	{"TestParseInvalid", "unitary", false, TestParseInvalid},
	{"TestGetValue", "unitary", false, TestGetValue},
	{"TestGetValueNotFound", "unitary", false, TestGetValueNotFound},
	{"TestGetNode", "unitary", false, TestGetNode},
	{"TestToString", "unitary", false, TestToString},
	{"TestChildCount", "unitary", false, TestChildCount},
	{"TestGetChild", "unitary", false, TestGetChild},
	{"TestParseUnexpectedOpeningParenWithoutName", "unitary", false, TestParseUnexpectedOpeningParenWithoutName},
	{"TestParseUnexpectedClosingParen", "unitary", false, TestParseUnexpectedClosingParen},
	{"TestParseUnexpectedToken", "unitary", false, TestParseUnexpectedToken},
	{"TestGetValueNonLeafNode", "unitary", false, TestGetValueNonLeafNode},
	{"TestGetNodeEmptyPath", "unitary", false, TestGetNodeEmptyPath},
	{"TestGetNodeWrongRootName", "unitary", false, TestGetNodeWrongRootName},
	{"TestGetNodeEmptySegment", "unitary", false, TestGetNodeEmptySegment},
	{"TestGetChildInvalidIndex", "unitary", false, TestGetChildInvalidIndex},
	{"TestGetNodeRootPath", "unitary", false, TestGetNodeRootPath},
	{"TestResolveConnectStringUrl_EZConnect", "unitary", false, TestResolveConnectStringUrl_EZConnect},
	{"TestResolveConnectStringUrl_TNS", "unitary", false, TestResolveConnectStringUrl_TNS},
	{"TestParseDSNString_InvalidInputs", "unitary", false, TestParseDSNString_InvalidInputs},
	{"TestParseDSNString_LongTNS_WithProperties_CleanConnectString", "unitary", false, TestParseDSNString_LongTNS_WithProperties_CleanConnectString},
	{"TestParseDSNString_ParseError", "unitary", false, TestParseDSNString_ParseError},
	{"TestParseDSNString_ExtractContextError", "unitary", false, TestParseDSNString_ExtractContextError},
	{"TestParseDSNString_ConversionError", "unitary", false, TestParseDSNString_ConversionError},
	{"TestParseDSNString_EmptyPassword", "unitary", false, TestParseDSNString_EmptyPassword},
	{"TestParseIterative_NoTokens", "unitary", false, TestParseIterative_NoTokens},
	{"TestExtractConnectionContext_Success", "unitary", false, TestExtractConnectionContext_Success},
	{"TestExtractConnectionContext_Errors", "unitary", false, TestExtractConnectionContext_Errors},
	{"TestExtractDescription_Complete", "unitary", false, TestExtractDescription_Complete},
	{"TestExtractDescription_CompressionLevels", "unitary", false, TestExtractDescription_CompressionLevels},
	{"TestExtractAddressList_FullEmptyAndErrors", "unitary", false, TestExtractAddressList_FullEmptyAndErrors},
	{"TestExtractDescriptionList_FullEmptyAndErrors", "unitary", false, TestExtractDescriptionList_FullEmptyAndErrors},
	{"TestExtractAddress_SuccessAndError", "unitary", false, TestExtractAddress_SuccessAndError},
	{"TestExtractConnectData_SuccessEmptyAndError", "unitary", false, TestExtractConnectData_SuccessEmptyAndError},
	{"TestExtractSecurity_SuccessEmptyAndError", "unitary", false, TestExtractSecurity_SuccessEmptyAndError},
	{"TestHelpersAndMethods", "unitary", false, TestHelpersAndMethods},
	{"TestExtractDescription_ErrorPropagation", "unitary", false, TestExtractDescription_ErrorPropagation},
	{"TestCoverage_MissingFlags", "unitary", false, TestCoverage_MissingFlags},
	{"TestParseEzConnect_SimpleHostAndService", "unitary", false, TestParseEzConnect_SimpleHostAndService},
	{"TestParseEzConnect_DefaultPort", "unitary", false, TestParseEzConnect_DefaultPort},
	{"TestParseEzConnect_InvalidPorts", "unitary", false, TestParseEzConnect_InvalidPorts},
	{"TestParseEzConnect_ValidPort_Boundary", "unitary", false, TestParseEzConnect_ValidPort_Boundary},
	{"TestParseEzConnect_Protocols", "unitary", false, TestParseEzConnect_Protocols},
	{"TestParseEzConnect_InvalidProtocol", "unitary", false, TestParseEzConnect_InvalidProtocol},
	{"TestParseEzConnect_MultipleHosts", "unitary", false, TestParseEzConnect_MultipleHosts},
	{"TestParseEzConnect_MultipleAddressGroups", "unitary", false, TestParseEzConnect_MultipleAddressGroups},
	{"TestParseEzConnect_ServerModeAndInstance", "unitary", false, TestParseEzConnect_ServerModeAndInstance},
	{"TestParseEzConnect_ExtendedParams", "unitary", false, TestParseEzConnect_ExtendedParams},
	{"TestParseEzConnect_QuotedParamValue", "unitary", false, TestParseEzConnect_QuotedParamValue},
	{"TestParseEzConnect_ParameterAliases", "unitary", false, TestParseEzConnect_ParameterAliases},
	{"TestParseEzConnect_HTTPSProxy", "unitary", false, TestParseEzConnect_HTTPSProxy},
	{"TestParseEzConnect_IPv6Address", "unitary", false, TestParseEzConnect_IPv6Address},
	{"TestParseEzConnect_IPv6WithMultipleHosts", "unitary", false, TestParseEzConnect_IPv6WithMultipleHosts},
	{"TestParseEzConnect_EmptyServiceVariants", "unitary", false, TestParseEzConnect_EmptyServiceVariants},
	{"TestParseEzConnect_WhitespaceRemoval", "unitary", false, TestParseEzConnect_WhitespaceRemoval},
	{"TestParseEzConnect_EmptyURL", "unitary", false, TestParseEzConnect_EmptyURL},
	{"TestParseEzConnect_SecurityParams", "unitary", false, TestParseEzConnect_SecurityParams},
	{"TestParseEzConnect_ConnectionPoolParams", "unitary", false, TestParseEzConnect_ConnectionPoolParams},
	{"TestParseEzConnect_ComplexMultiHostScenario", "unitary", false, TestParseEzConnect_ComplexMultiHostScenario},
	{"TestParseExtendedParams_InvalidFormat", "unitary", false, TestParseExtendedParams_InvalidFormat},
	{"TestParseExtendedParams_EmptyString", "unitary", false, TestParseExtendedParams_EmptyString},
	{"TestParseExtendedParams_MultipleParams", "unitary", false, TestParseExtendedParams_MultipleParams},
	{"TestParseEzConnect_InvalidExtendedParams", "unitary", false, TestParseEzConnect_InvalidExtendedParams},
	{"TestParseEzConnect_EmptyAddressGroups", "unitary", false, TestParseEzConnect_EmptyAddressGroups},
	{"TestParseHostList_TrailingComma", "unitary", false, TestParseHostList_TrailingComma},
	{"TestParseHostList_MultipleHosts", "unitary", false, TestParseHostList_MultipleHosts},
	{"TestParseHostList_EmptyHost", "unitary", false, TestParseHostList_EmptyHost},
	{"TestParseAddressGroups_MultipleGroups", "unitary", false, TestParseAddressGroups_MultipleGroups},
	{"TestParseAddressGroups_EmptyString", "unitary", false, TestParseAddressGroups_EmptyString},
	{"TestBuildAddress_TCPWithoutProxy", "unitary", false, TestBuildAddress_TCPWithoutProxy},
	{"TestBuildAddress_TCPSWithProxy", "unitary", false, TestBuildAddress_TCPSWithProxy},
	{"TestBuildAddress_IPv6", "unitary", false, TestBuildAddress_IPv6},
	{"TestBuildConnectData_AllParams", "unitary", false, TestBuildConnectData_AllParams},
	{"TestBuildConnectData_EmptyService", "unitary", false, TestBuildConnectData_EmptyService},
	{"TestBuildSecurity_AllParams", "unitary", false, TestBuildSecurity_AllParams},
	{"TestBuildSecurity_Empty", "unitary", false, TestBuildSecurity_Empty},
	{"TestBuildDescriptionParams_AutoLoadBalance", "unitary", false, TestBuildDescriptionParams_AutoLoadBalance},
	{"TestBuildDescriptionParams_ExplicitLoadBalance", "unitary", false, TestBuildDescriptionParams_ExplicitLoadBalance},
	{"TestBuildAddressList_SingleGroup", "unitary", false, TestBuildAddressList_SingleGroup},
	{"TestBuildAddressList_MultipleGroups", "unitary", false, TestBuildAddressList_MultipleGroups},
	{"TestSplitAndParseExtendedParams_WithParams", "unitary", false, TestSplitAndParseExtendedParams_WithParams},
	{"TestParseMainURL_AllComponents", "unitary", false, TestParseMainURL_AllComponents},
	{"TestParseMainURL_MinimalURL", "unitary", false, TestParseMainURL_MinimalURL},
	{"TestParseHostList_DefaultPort_InMiddle", "unitary", false, TestParseHostList_DefaultPort_InMiddle},
	{"TestParseEzConnect_HTTPSProxyWithoutPort", "unitary", false, TestParseEzConnect_HTTPSProxyWithoutPort},
	{"TestSplitAndParseExtendedParams_ParensBeforeParams", "unitary", false, TestSplitAndParseExtendedParams_ParensBeforeParams},
	{"TestConnectionIterator_Attempt_EZConnect_PreservesQuotedWalletLocation", "unitary", false, TestConnectionIterator_Attempt_EZConnect_PreservesQuotedWalletLocation},
	{"TestConnectionIterator_Attempt_LongTNS_PreservesQuotedServerCertDN", "unitary", false, TestConnectionIterator_Attempt_LongTNS_PreservesQuotedServerCertDN},
	{"TestParseDSNString_CommaBetweenNodes_FailsFast", "unitary", false, TestParseDSNString_CommaBetweenNodes_FailsFast},
	{"TestParseDSNString_EZConnect_Quotes_Spaces", "unitary", false, TestParseDSNString_EZConnect_Quotes_Spaces},
	{"TestParseEzConnect_Whitespace_Removal_Preserve", "unitary", false, TestParseEzConnect_Whitespace_Removal_Preserve},
	{"TestParse_PreservesQuotedValues", "unitary", false, TestParse_PreservesQuotedValues},
	{"TestResolveAddresses_ContextHandling", "unitary", false, TestResolveAddresses_ContextHandling},
	{"TestResolveAddresses_DNSExpansion", "unitary", false, TestResolveAddresses_DNSExpansion},
	{"TestResolveAddresses_HostClassification", "unitary", false, TestResolveAddresses_HostClassification},
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
