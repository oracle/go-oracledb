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

package config

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
	TestCategories = oracleTest.TestCategories
	os.Exit(m.Run())
}

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestValidateLoggingLevel", Categories: "unitary", Fn: TestValidateLoggingLevel},
	{Name: "TestConnectionAttemptIteratorPrioritizesHealthyDescriptionsAndTargets", Categories: "unitary", Fn: TestConnectionAttemptIteratorPrioritizesHealthyDescriptionsAndTargets},
	{Name: "TestConnectionAttemptIteratorRotatesTargetsOnRetry", Categories: "unitary", Fn: TestConnectionAttemptIteratorRotatesTargetsOnRetry},
	{Name: "TestConnectionAttemptIteratorResolvesDescriptionConfigWhenReached", Categories: "unitary", Fn: TestConnectionAttemptIteratorResolvesDescriptionConfigWhenReached},
	{Name: "TestConnectionAttemptIteratorAppliesDescriptionFailover", Categories: "unitary", Fn: TestConnectionAttemptIteratorAppliesDescriptionFailover},
	{Name: "TestConnectionAttemptIteratorAppliesAddressGroupFailover", Categories: "unitary", Fn: TestConnectionAttemptIteratorAppliesAddressGroupFailover},
	{Name: "TestConnectionAttemptIteratorKeepsConnectDataPerDescription", Categories: "unitary", Fn: TestConnectionAttemptIteratorKeepsConnectDataPerDescription},
	{Name: "TestConnectionAttemptIteratorSharesConnectDataAcrossTargets", Categories: "unitary", Fn: TestConnectionAttemptIteratorSharesConnectDataAcrossTargets},
	{Name: "TestConnectionAttemptIteratorRetryDelayHonorsContext", Categories: "unitary", Fn: TestConnectionAttemptIteratorRetryDelayHonorsContext},
	{Name: "TestConnectionAttemptIteratorRetryCountRepeatsTargets", Categories: "unitary", Fn: TestConnectionAttemptIteratorRetryCountRepeatsTargets},
	{Name: "TestConnectionAttemptIteratorHonorsZeroRetryCount", Categories: "unitary", Fn: TestConnectionAttemptIteratorHonorsZeroRetryCount},
	{Name: "TestConnectionAttemptIteratorReordersDownHostsOnRetry", Categories: "unitary", Fn: TestConnectionAttemptIteratorReordersDownHostsOnRetry},
	{Name: "TestConnectionAttemptIteratorHandlesManyTargets", Categories: "unitary", Fn: TestConnectionAttemptIteratorHandlesManyTargets},
	{Name: "TestConnectionAttemptIteratorNormalizesNegativeRetryCount", Categories: "unitary", Fn: TestConnectionAttemptIteratorNormalizesNegativeRetryCount},
	{Name: "TestHandleDsnBuildsConnectionPlans", Categories: "unitary", Fn: TestHandleDsnBuildsConnectionPlans},
	{Name: "TestHandleDsnNormalizesEZConnectTargets", Categories: "unitary", Fn: TestHandleDsnNormalizesEZConnectTargets},
	{Name: "TestHandleDsnParsesTNSDescriptionFields", Categories: "unitary", Fn: TestHandleDsnParsesTNSDescriptionFields},
	{Name: "TestHandleDsnUsesDefaultSecurityValues", Categories: "unitary", Fn: TestHandleDsnUsesDefaultSecurityValues},
	{Name: "TestHandleDsnRejectsInvalidTNSValues", Categories: "unitary", Fn: TestHandleDsnRejectsInvalidTNSValues},
	{Name: "TestHandleDsnNormalizesNegativeRetryCount", Categories: "unitary", Fn: TestHandleDsnNormalizesNegativeRetryCount},
	{Name: "TestHandleDsnAppliesEZConnectQuerySettings", Categories: "unitary", Fn: TestHandleDsnAppliesEZConnectQuerySettings},
	{Name: "TestHandleDsnPreservesQuotedSecurityValues", Categories: "unitary", Fn: TestHandleDsnPreservesQuotedSecurityValues},
	{Name: "TestHandleDsnAppliesQueryPropertiesAndParsesTNS", Categories: "unitary", Fn: TestHandleDsnAppliesQueryPropertiesAndParsesTNS},
	{Name: "TestHandleDsnAcceptsDescriptorWithoutCredentials", Categories: "unitary", Fn: TestHandleDsnAcceptsDescriptorWithoutCredentials},
	{Name: "TestHandleDsnParsesCredentialsFromDSN", Categories: "unitary", Fn: TestHandleDsnParsesCredentialsFromDSN},
	{Name: "TestHandleDsnRejectsInvalidOrConflictingCredentials", Categories: "unitary", Fn: TestHandleDsnRejectsInvalidOrConflictingCredentials},
	{Name: "TestHandleDsnRejectsInvalidLegacyDSNInputs", Categories: "unitary", Fn: TestHandleDsnRejectsInvalidLegacyDSNInputs},
	{Name: "TestHandleRedirectDsnAllowsConnectDataInRedirectPacket", Categories: "unitary", Fn: TestHandleRedirectDsnAllowsConnectDataInRedirectPacket},
	{Name: "TestHandleDsnRejectsMalformedDescriptors", Categories: "unitary", Fn: TestHandleDsnRejectsMalformedDescriptors},
	{Name: "TestConnectionAttemptBuildsConnectDescriptorWithClientInfo", Categories: "unitary", Fn: TestConnectionAttemptBuildsConnectDescriptorWithClientInfo},
	{Name: "TestConnectionAttemptSerializesAddressProxy", Categories: "unitary", Fn: TestConnectionAttemptSerializesAddressProxy},
	{Name: "TestHandleDsnPreservesExplicitEmptyConnectDataQuery", Categories: "unitary", Fn: TestHandleDsnPreservesExplicitEmptyConnectDataQuery},
	{Name: "TestParseSimple", Categories: "unitary", Fn: TestParseSimple},
	{Name: "TestParseComplex", Categories: "unitary", Fn: TestParseComplex},
	{Name: "TestParseEmpty", Categories: "unitary", Fn: TestParseEmpty},
	{Name: "TestParseWhitespaceOnly", Categories: "unitary", Fn: TestParseWhitespaceOnly},
	{Name: "TestParseInvalid", Categories: "unitary", Fn: TestParseInvalid},
	{Name: "TestGetValue", Categories: "unitary", Fn: TestGetValue},
	{Name: "TestGetValueNotFound", Categories: "unitary", Fn: TestGetValueNotFound},
	{Name: "TestGetNode", Categories: "unitary", Fn: TestGetNode},
	{Name: "TestToString", Categories: "unitary", Fn: TestToString},
	{Name: "TestParse_PreservesQuotedValues", Categories: "unitary", Fn: TestParse_PreservesQuotedValues},
	{Name: "TestChildCount", Categories: "unitary", Fn: TestChildCount},
	{Name: "TestGetChild", Categories: "unitary", Fn: TestGetChild},
	{Name: "TestParseUnexpectedOpeningParenWithoutName", Categories: "unitary", Fn: TestParseUnexpectedOpeningParenWithoutName},
	{Name: "TestParseUnexpectedClosingParen", Categories: "unitary", Fn: TestParseUnexpectedClosingParen},
	{Name: "TestParseUnexpectedToken", Categories: "unitary", Fn: TestParseUnexpectedToken},
	{Name: "TestGetValueNonLeafNode", Categories: "unitary", Fn: TestGetValueNonLeafNode},
	{Name: "TestGetNodeEmptyPath", Categories: "unitary", Fn: TestGetNodeEmptyPath},
	{Name: "TestGetNodeWrongRootName", Categories: "unitary", Fn: TestGetNodeWrongRootName},
	{Name: "TestGetNodeEmptySegment", Categories: "unitary", Fn: TestGetNodeEmptySegment},
	{Name: "TestGetChildInvalidIndex", Categories: "unitary", Fn: TestGetChildInvalidIndex},
	{Name: "TestGetNodeRootPath", Categories: "unitary", Fn: TestGetNodeRootPath},
	{Name: "TestParseIterative_NoTokens", Categories: "unitary", Fn: TestParseIterative_NoTokens},
	{Name: "TestParseMainURL_AllComponents", Categories: "unitary", Fn: TestParseMainURL_AllComponents},
	{Name: "TestParseMainURL_MinimalURL", Categories: "unitary", Fn: TestParseMainURL_MinimalURL},
	{Name: "TestParseEzConnect_Protocols", Categories: "unitary", Fn: TestParseEzConnect_Protocols},
	{Name: "TestParseEzConnect_InvalidProtocol", Categories: "unitary", Fn: TestParseEzConnect_InvalidProtocol},
	{Name: "TestParseEzConnect_InvalidPorts", Categories: "unitary", Fn: TestParseEzConnect_InvalidPorts},
	{Name: "TestParseEzConnect_ValidPort_Boundary", Categories: "unitary", Fn: TestParseEzConnect_ValidPort_Boundary},
	{Name: "TestValidatePort", Categories: "unitary", Fn: TestValidatePort},
	{Name: "TestParseEzConnect_EmptyAddressGroups", Categories: "unitary", Fn: TestParseEzConnect_EmptyAddressGroups},
	{Name: "TestParseHostList_TrailingComma", Categories: "unitary", Fn: TestParseHostList_TrailingComma},
	{Name: "TestParseHostList_MultipleHosts", Categories: "unitary", Fn: TestParseHostList_MultipleHosts},
	{Name: "TestParseHostList_NormalizesBracketedIPv6Hosts", Categories: "unitary", Fn: TestParseHostList_NormalizesBracketedIPv6Hosts},
	{Name: "TestParseHostList_DefaultPort_InMiddle", Categories: "unitary", Fn: TestParseHostList_DefaultPort_InMiddle},
	{Name: "TestParseAddressGroups_MultipleGroups", Categories: "unitary", Fn: TestParseAddressGroups_MultipleGroups},
	{Name: "TestParseAddressGroups_EmptyString", Categories: "unitary", Fn: TestParseAddressGroups_EmptyString},
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
var TestCategories oracleTest.TestCategoryList
