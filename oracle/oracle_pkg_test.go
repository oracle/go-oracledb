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

package oracle

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	err := InitConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "InitConfig failed: %v\n", err)
		os.Exit(1)
	} else {
		os.Exit(m.Run())
	}
}

var testCases = []struct {
	name       string
	categories string
	exclusive  bool
	f          func(t *testing.T)
}{
	{"TestDriver_ConfigurationWithConnectorBasic", "functional", true, TestDriver_ConfigurationWithConnectorBasic},
	{"TestDriver_ConfigurationWithConnectorWithEnvOverwrite", "functional", true, TestDriver_ConfigurationWithConnectorWithEnvOverwrite},
	{"TestDriver_ConfigurationWithConnectorWithFlagOverwrite", "functional", true, TestDriver_ConfigurationWithConnectorWithFlagOverwrite},
	{"TestConfiguration_AssignFromEmptyMap", "unitary", false, TestConfiguration_AssignFromEmptyMap},
	{"TestConfiguration_AssignFromMapUnknownKey", "unitary", false, TestConfiguration_AssignFromMapUnknownKey},
	{"TestConfiguration_AssignFromMap", "unitary", false, TestConfiguration_AssignFromMap},
	{"TestConfiguration_AssignFromMapValidatedIntString", "unitary", false, TestConfiguration_AssignFromMapValidatedIntString},
	{"TestConfiguration_AssignFromEnv", "unitary", true, TestConfiguration_AssignFromEnv},
	{"TestConfiguration_AssignFromEnvValidatedIntString", "unitary", true, TestConfiguration_AssignFromEnvValidatedIntString},
	{"TestConfiguration_AssignFromEmptyFlags", "unitary", false, TestConfiguration_AssignFromEmptyFlags},
	{"TestConfiguration_Clone", "unitary", false, TestConfiguration_Clone},
	{"TestConfiguration_DefaultClientLanguageIsLanguageTag", "unitary", false, TestConfiguration_DefaultClientLanguageIsLanguageTag},
	{"TestConfiguration_AssignFromMapClientLanguageTag", "unitary", false, TestConfiguration_AssignFromMapClientLanguageTag},
	{"TestConfiguration_AssignFromEnvClientLanguageTag", "unitary", true, TestConfiguration_AssignFromEnvClientLanguageTag},
	{"TestConfiguration_toNSConnectionParameters", "unitary", false, TestConfiguration_toNSConnectionParameters},
	{"TestConfiguration_InitLoggingWithConfigFileDestination", "unitary", false, TestConfiguration_InitLoggingWithConfigFileDestination},
	{"TestEnquoteLiteral", "unitary", false, TestEnquoteLiteral},
	{"TestEnquoteNCharLiteral", "unitary", false, TestEnquoteNCharLiteral},
	{"TestIsSimpleIdentifier", "unitary", false, TestIsSimpleIdentifier},
	{"TestEnquoteIdentifier", "unitary", false, TestEnquoteIdentifier},
	{"TestDriver_ConfigurationWithCredentialsWithDsnNegative", "unitary", false, TestDriver_ConfigurationWithCredentialsWithDsnNegative},
	{"TestDriver_ConfigurationLogging", "unitary", false, TestDriver_ConfigurationLogging},
	{"TestDriver_OpenConnectorUsesNSParamOverConfig", "unitary", false, TestDriver_OpenConnectorUsesNSParamOverConfig},
	{"TestDriver_Table_Create", "sanity", false, TestDriver_Table_Create},
	{"TestDriver_DropTable_DeniesAccess", "functional", false, TestDriver_DropTable_DeniesAccess},
	{"TestDriver_AlterSessionSetLanguage", "functional", false, TestDriver_AlterSessionSetLanguage},
	{"TestDriver_Table_Insert", "functional", false, TestDriver_Table_Insert},
	{"TestDriver_Insert_Select", "functional", false, TestDriver_Insert_Select},
	{"TestDriver_PLSQL_AnonymousBlock_Sanity", "functional", false, TestDriver_PLSQL_AnonymousBlock_Sanity},
	{"TestDriver_PLSQL_CreateInsertSelectDrop", "functional", false, TestDriver_PLSQL_CreateInsertSelectDrop},
	{"TestDriver_PLSQL_BreakCausedByTimeout", "functional", false, TestDriver_PLSQL_BreakCausedByTimeout},
	{"TestDriver_Select_BooleanTypes_23c", "functional", false, TestDriver_Select_BooleanTypes_23c},
	{"TestDriver_Select_CharacterTypes", "functional", false, TestDriver_Select_CharacterTypes},
	{"TestDriver_Select_DATE", "functional", false, TestDriver_Select_DATE},
	{"TestDriver_Select_TIMESTAMP", "functional", false, TestDriver_Select_TIMESTAMP},
	{"TestDriver_Select_TimestampWithTimeZone", "functional", false, TestDriver_Select_TimestampWithTimeZone},
	{"TestDriver_Select_TimestampWithLocalTimeZone", "functional", false, TestDriver_Select_TimestampWithLocalTimeZone},
	{"TestDriver_Select_Intervals", "functional", false, TestDriver_Select_Intervals},
	{"TestDriver_Select_NumericFloatTypes", "functional", false, TestDriver_Select_NumericFloatTypes},
	{"TestDriver_Select_Number_NoPrecisionScale", "functional", false, TestDriver_Select_Number_NoPrecisionScale},
	{"TestDriver_Select_NumberPrecision", "functional", false, TestDriver_Select_NumberPrecision},
	{"TestDriver_VarcharLargePayload", "functional", false, TestDriver_VarcharLargePayload},
	{"TestDriver_Select_Number_MaxPrecisionScale", "functional", false, TestDriver_Select_Number_MaxPrecisionScale},
	{"TestDriver_Select_Number_MaxPrecisionInteger", "functional", false, TestDriver_Select_Number_MaxPrecisionInteger},
	{"TestDriver_Select_Number_And_BinaryDouble", "functional", false, TestDriver_Select_Number_And_BinaryDouble},
	{"TestDriver_Table_Select", "functional", false, TestDriver_Table_Select},
	{"TestTimeoutConnectWithTransportConnectTimeout", "functional", false, TestTimeoutConnectWithTransportConnectTimeout},
	{"TestTimeoutConnectWithTransportConnectTimeoutAndContext", "functional", false, TestTimeoutConnectWithTransportConnectTimeoutAndContext},
	{"TestTimeoutConnectWithTransportConnectTimeoutAndContextGreater", "functional/cyclops", false, TestTimeoutConnectWithTransportConnectTimeoutAndContextGreater},
	{"TestTimeoutConnectWithRecvTimeout", "functional/cyclops", false, TestTimeoutConnectWithRecvTimeout},
	{"TestTimeoutConnectWithConnectTimeout", "functional/cyclops", false, TestTimeoutConnectWithConnectTimeout},
	{"TestTimeoutConnectWithRecvConnectTimeoutAndContext", "functional/cyclops", false, TestTimeoutConnectWithRecvConnectTimeoutAndContext},
	{"TestTimeoutConnectWithRecvConnectTimeoutAndContextGreater", "functional/cyclops", false, TestTimeoutConnectWithRecvConnectTimeoutAndContextGreater},
	{"TestTimeoutConnectTimeoutPrecedence1", "functional/cyclops", false, TestTimeoutConnectTimeoutPrecedence1},
	{"TestTimeoutConnectTimeoutPrecedence2", "functional/cyclops", false, TestTimeoutConnectTimeoutPrecedence2},
	{"TestTimeoutConnectTimeoutPrecedence3", "functional/cyclops", false, TestTimeoutConnectTimeoutPrecedence3},
	{"TestTimeoutConnectTimeoutPrecedence4", "functional", false, TestTimeoutConnectTimeoutPrecedence4},
	{"TestDriver_Functional_SelectDual", "sanity", false, TestDriver_Functional_SelectDual},
	{"TestDriver_SimpleConnection", "sanity", false, TestDriver_SimpleConnection},
	{"TestDriver_Authentication_TTIWRN", "functional", false, TestDriver_Authentication_TTIWRN},
	{"TestDriver_Authentication_OCIToken", "functional", false, TestDriver_Authentication_OCIToken},
	{"TestDriver_Authentication_OAuth", "functional", false, TestDriver_Authentication_OAuth},
	{"TestDriver_TCPS_Pipeline_SelectDual", "sanity", false, TestDriver_TCPS_Pipeline_SelectDual},
	{"TestDriver_TCPS_Pipeline_InvalidCertDn", "sanity", false, TestDriver_TCPS_Pipeline_InvalidCertDn},
	{"TestDriver_TCPS_Pipeline_InvalidWalletLocation", "sanity", false, TestDriver_TCPS_Pipeline_InvalidWalletLocation},
	{"TestDriver_TCPS_Pipeline_DNMatchOff_AllowsInvalidDN", "sanity", false, TestDriver_TCPS_Pipeline_DNMatchOff_AllowsInvalidDN},
	{"TestDriver_Prepared_Insert_Select_Ordinal", "sanity", false, TestDriver_Prepared_Insert_Select_Ordinal},
	{"TestDriver_Prepared_Insert_Select_Named", "functional", false, TestDriver_Prepared_Insert_Select_Named},
	{"TestDriver_PLSQL_Prepared_Binds", "functional", false, TestDriver_PLSQL_Prepared_Binds},
	{"TestDriver_Select_BooleanTypes_23c_Prepared_Statement", "functional", false, TestDriver_Select_BooleanTypes_23c_Prepared_Statement},
	{"TestDriver_Select_BooleanTypes_23c_Prepared_Statement_Named", "functional", false, TestDriver_Select_BooleanTypes_23c_Prepared_Statement_Named},
	{"TestDriver_Select_BooleanTypes_19c", "functional", false, TestDriver_Select_BooleanTypes_19c},
	{"TestDriver_Select_BooleanTypes_19c_Prepared_Statement", "functional", false, TestDriver_Select_BooleanTypes_19c_Prepared_Statement},
	{"TestDriver_Select_BooleanTypes_19c_Prepared_Statement_Named", "functional", false, TestDriver_Select_BooleanTypes_19c_Prepared_Statement_Named},
	{"TestDriver_Select_CharacterTypes_Ordinal", "functional", false, TestDriver_Select_CharacterTypes_Ordinal},
	{"TestDriver_Select_CharacterTypes_Named", "functional", false, TestDriver_Select_CharacterTypes_Named},
	{"TestDriver_Select_DATE_Prepared_Named", "functional", false, TestDriver_Select_DATE_Prepared_Named},
	{"TestDriver_Select_TIMESTAMP_Prepared_Named", "functional", false, TestDriver_Select_TIMESTAMP_Prepared_Named},
	{"TestDriver_Select_TimestampWithTimeZone_Prepared_Named", "functional", false, TestDriver_Select_TimestampWithTimeZone_Prepared_Named},
	{"TestDriver_Select_TimestampWithLocalTimeZone_Prepared_Named", "functional", false, TestDriver_Select_TimestampWithLocalTimeZone_Prepared_Named},
	{"TestDriver_Select_TimestampWithTimeZone_Prepared_LoadLocation_And_Offset", "functional", false, TestDriver_Select_TimestampWithTimeZone_Prepared_LoadLocation_And_Offset},
	{"TestDriver_Prepared_InsertAndSelect_AllTypes", "functional", false, TestDriver_Prepared_InsertAndSelect_AllTypes},
	{"TestDriver_Prepared_InsertAndSelect_AllTypes_StrictNulls", "functional", false, TestDriver_Prepared_InsertAndSelect_AllTypes_StrictNulls},
	{"TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls", "functional", false, TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls},
	{"TestDriver_Prepared_Statement_Re_exec_With_Different_Arg_Counts_Negative", "functional", false, TestDriver_Prepared_Statement_Re_exec_With_Different_Arg_Counts_Negative},
	{"TestDriver_Exec_Query_cursor_leak", "robustness", false, TestDriver_Exec_Query_cursor_leak},
	{"TestDriver_Select_Query_cursor_leak", "robustness", false, TestDriver_Select_Query_cursor_leak},
	{"TestDriver_PreparedStatement_Query_cursor_leak", "robustness", false, TestDriver_PreparedStatement_Query_cursor_leak},
	{"TestQueryNonExistentTable_NegativeCase", "functional", false, TestQueryNonExistentTable_NegativeCase},
	{"TestPreparedStatementNonExistentTable_NegativeCase", "functional", false, TestPreparedStatementNonExistentTable_NegativeCase},
	{"TestSelectSpecificColumnsNonExistentTable_NegativeCase", "functional", false, TestSelectSpecificColumnsNonExistentTable_NegativeCase},
	{"TestCountQueryNonExistentTable_NegativeCase", "functional", false, TestCountQueryNonExistentTable_NegativeCase},
	{"TestJoinWithNonExistentTable_NegativeCase", "functional", false, TestJoinWithNonExistentTable_NegativeCase},
	{"TestSubqueryWithNonExistentTable_NegativeCase", "functional", false, TestSubqueryWithNonExistentTable_NegativeCase},
	{"TestDescribeNonExistentTable_NegativeCase", "functional", false, TestDescribeNonExistentTable_NegativeCase},
	{"TestInvalidTableNameSyntax_NegativeCase", "functional", false, TestInvalidTableNameSyntax_NegativeCase},
	{"TestQuerySystemTableWithoutPrivilege_NegativeCase", "functional", false, TestQuerySystemTableWithoutPrivilege_NegativeCase},
	{"TestQueryAccessibleTable_PositiveCase", "functional", false, TestQueryAccessibleTable_PositiveCase},
	{"TestQueryDictionaryViewWithoutPrivilege_NegativeCase", "functional", false, TestQueryDictionaryViewWithoutPrivilege_NegativeCase},
	{"TestInsertAndSelectSmallRAW", "functional", false, TestInsertAndSelectSmallRAW},
	{"TestInsertAndSelectLargeRAW", "functional", false, TestInsertAndSelectLargeRAW},
	{"TestInsertAndSelectNullRAW", "functional", false, TestInsertAndSelectNullRAW},
	{"TestRAWMultipleRows", "functional", false, TestRAWMultipleRows},
	{"TestRAWUpdateOperation", "functional", false, TestRAWUpdateOperation},
	{"TestRAWTypeSystemIntegration", "functional", false, TestRAWTypeSystemIntegration},
	{"TestDriver_Prepared_InsertMultipleRows_Re_exec", "functional", false, TestDriver_Prepared_InsertMultipleRows_Re_exec},
	{"TestDriver_Bind_ReusedNamedParameter", "functional", false, TestDriver_Bind_ReusedNamedParameter},
	{"TestDriver_Prepared_SelectMultipleRows_Re_exec_BindTypeChange", "functional", false, TestDriver_Prepared_SelectMultipleRows_Re_exec_BindTypeChange},
	{"TestDriver_Prepared_SelectMultipleRows_Re_exec", "functional", false, TestDriver_Prepared_SelectMultipleRows_Re_exec},

	{"TestCommit", "functional", false, TestCommit},
	{"TestRollback", "functional", false, TestRollback},
	{"TestRollbackThroughContextServerSleep", "functional", false, TestRollbackThroughContextServerSleep},
	{"TestRollbackThroughContextCancel", "functional", false, TestRollbackThroughContextCancel},

	{"TestDriver_Prepared_Insert_Clob_Small", "functional", false, TestDriver_Prepared_Insert_Clob_Small},
	{"TestDriver_Prepared_Insert_Clob_Large", "functional", false, TestDriver_Prepared_Insert_Clob_Large},
	{"TestDriver_Prepared_Insert_Blob_Small", "functional", false, TestDriver_Prepared_Insert_Blob_Small},
	{"TestDriver_Prepared_Insert_Blob_Large", "functional", false, TestDriver_Prepared_Insert_Blob_Large},

	{"TestReadOnlyTransaction", "functional", false, TestReadOnlyTransaction},

	{"TestDriver_Table_Select_JSON", "functional", false, TestDriver_Table_Select_JSON},
	{"TestDriver_Table_Select_NullJSON", "functional", false, TestDriver_Table_Select_NullJSON},
	{"TestDriver_Table_Select_CLOB", "functional", false, TestDriver_Table_Select_CLOB},
	{"TestDriver_Table_Select_BLOB", "functional", false, TestDriver_Table_Select_BLOB},
	{"TestDriver_Table_Insert_Select_CLOB_BLOB_MultiRows", "functional", false, TestDriver_Table_Insert_Select_CLOB_BLOB_MultiRows},
	{"TestDriver_Table_Insert_Select_JSON_MultiRows", "functional", false, TestDriver_Table_Insert_Select_JSON_MultiRows},

	{"TestDriver_DMLReturning_Insert_Ordinal", "functional", false, TestDriver_DMLReturning_Insert_Ordinal},
	{"TestDriver_DMLReturning_Update_Named", "functional", false, TestDriver_DMLReturning_Update_Named},
	{"TestDriver_DMLReturning_Insert_MultipleScalarTypes", "functional", false, TestDriver_DMLReturning_Insert_MultipleScalarTypes},
	{"TestDriver_DMLReturning_Delete_Ordinal", "functional", false, TestDriver_DMLReturning_Delete_Ordinal},
	{"TestDriver_DMLReturning_Update_Named_NoStmt", "functional", false, TestDriver_DMLReturning_Update_Named_NoStmt},
	{"TestDriver_DMLReturning_ZeroRowsAffected", "functional", false, TestDriver_DMLReturning_ZeroRowsAffected},
	{"TestDriver_DMLReturning_Delete_ZeroRowsAffected", "functional", false, TestDriver_DMLReturning_Delete_ZeroRowsAffected},
	{"TestDriver_DMLReturning_Insert_RAW", "functional", false, TestDriver_DMLReturning_Insert_RAW},
	{"TestDriver_DMLReturning_Insert_CHAR", "functional", false, TestDriver_DMLReturning_Insert_CHAR},
	{"TestDriver_DMLReturning_PreparedStmt_ReExecution", "functional", false, TestDriver_DMLReturning_PreparedStmt_ReExecution},
	{"TestDriver_DMLReturning_InTransaction_Rollback", "functional", false, TestDriver_DMLReturning_InTransaction_Rollback},
	{"TestDriver_DMLReturning_InTransaction_Commit", "functional", false, TestDriver_DMLReturning_InTransaction_Commit},
	{"TestDriver_DMLReturning_Insert_InOut", "functional", false, TestDriver_DMLReturning_Insert_InOut},
	{"TestDriver_DMLReturning_Update_MultipleRows", "functional", false, TestDriver_DMLReturning_Update_MultipleRows},
	{"TestDriver_DMLReturning_Insert_NullableColumn", "functional", false, TestDriver_DMLReturning_Insert_NullableColumn},
	{"TestDriver_DMLReturning_Insert_NullBinaryFloatIntoNullFloat64", "functional", false, TestDriver_DMLReturning_Insert_NullBinaryFloatIntoNullFloat64},
	{"TestDriver_DMLReturning_Insert_TimestampWithLocalTZ", "functional", false, TestDriver_DMLReturning_Insert_TimestampWithLocalTZ},
	{"TestDriver_DMLReturning_Insert_NumberScalePrecision", "functional", false, TestDriver_DMLReturning_Insert_NumberScalePrecision},
	{"TestDriver_DMLReturning_BinaryFloatColumn", "functional", false, TestDriver_DMLReturning_BinaryFloatColumn},
	{"TestDriver_DMLReturning_Insert_BooleanColumn", "functional", false, TestDriver_DMLReturning_Insert_BooleanColumn},
	{"TestDriver_PLSQL_InOut_NumberFunction", "functional", false, TestDriver_PLSQL_InOut_NumberFunction},
	{"TestDriver_PLSQL_InOut_VarcharProcedure", "functional", false, TestDriver_PLSQL_InOut_VarcharProcedure},
	{"TestDriver_PLSQL_ProcedureWithInOut", "functional", false, TestDriver_PLSQL_ProcedureWithInOut},
	{"TestDriver_PLSQL_ProcedureWithInOut_AllTypes", "functional", false, TestDriver_PLSQL_ProcedureWithInOut_AllTypes},
	{"TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement", "functional", false, TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement},
	{"TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement_NamedBinds", "functional", false, TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement_NamedBinds},

	{"TestDriver_PLSQL_InOut_NumberFunctionDoubleBind", "functional", false, TestDriver_PLSQL_InOut_NumberFunctionDoubleBind},

	{"TestDriver_TRIGGER_GormTest", "functional", false, TestDriver_TRIGGER_GormTest},

	{"TestDriver_SQLNullTypes_BindInputs", "functional", false, TestDriver_SQLNullTypes_BindInputs},
	{"TestDriver_SQLNullTypes_DMLReturning_OutDest", "functional", false, TestDriver_SQLNullTypes_DMLReturning_OutDest},
	{"TestDriver_SQLNullTypes_PLSQL_InOut", "functional", false, TestDriver_SQLNullTypes_PLSQL_InOut},
	{"TestDriver_SQLNullTypes_PLSQL_InOut_UsingSetupObjects", "functional", false, TestDriver_SQLNullTypes_PLSQL_InOut_UsingSetupObjects},
	{"TestDriver_SQLNullTypes_PLSQL_InOut_NullInputs", "functional", false, TestDriver_SQLNullTypes_PLSQL_InOut_NullInputs},

	{"TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails", "unitary", false, TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails},
	{"TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails", "unitary", false, TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails},
	{"TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess", "unitary", false, TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess},
	{"TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess", "unitary", false, TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess},
	{"TestDriver_InsertForeignKeyViolation", "functional", false, TestDriver_InsertForeignKeyViolation},
	{"TestDriver_Varchar2_TrailingSpacesPreserved", "functional", false, TestDriver_Varchar2_TrailingSpacesPreserved},
	{"TestDriver_Varchar2_EmptyStringIsNull", "functional", false, TestDriver_Varchar2_EmptyStringIsNull},
	{"TestDriver_Varchar2_EmbeddedNULRoundTrip", "functional", false, TestDriver_Varchar2_EmbeddedNULRoundTrip},
	{"TestDriver_Varchar2_BoundaryLengths", "functional", false, TestDriver_Varchar2_BoundaryLengths},
	{"TestDriver_Select_ZeroRows_FilterCondition", "functional", false, TestDriver_Select_ZeroRows_FilterCondition},
	{"TestDriver_Select_RowWithNullColumn", "functional", false, TestDriver_Select_RowWithNullColumn},
	{"TestDriver_Select_MultipleRows_SomeNulls", "functional", false, TestDriver_Select_MultipleRows_SomeNulls},
	{"TestDriver_Select_AllNullExceptPK", "functional", false, TestDriver_Select_AllNullExceptPK},
	{"TestDriver_Select_NullFromComputedExpression", "functional", false, TestDriver_Select_NullFromComputedExpression},
	{"TestDriver_OpenConnectorReturnsInvalidDSNParameterError", "unitary", false, TestDriver_OpenConnectorReturnsInvalidDSNParameterError},
	{"TestDriver_OpenConnectorStoresConnectDescriptorFromDSN", "unitary", false, TestDriver_OpenConnectorStoresConnectDescriptorFromDSN},
	{"TestDriver_OpenConnectorUsesFallbackConnectDescriptor", "unitary", false, TestDriver_OpenConnectorUsesFallbackConnectDescriptor},
	{"TestDriver_OpenConnectorUsesNSParam", "unitary", false, TestDriver_OpenConnectorUsesNSParam},
	{"TestDriver_OpenConnectorUsesNSProperty", "unitary", false, TestDriver_OpenConnectorUsesNSProperty},
	{"TestDriver_OpenConnectorUsesParam", "unitary", false, TestDriver_OpenConnectorUsesParam},
	{"TestConnection_ResetSessionKo", "functional", false, TestConnection_ResetSessionKo},
	{"TestConnection_ResetSessionOk", "functional", false, TestConnection_ResetSessionOk},
	{"TestConnection_ResetSessionPool", "functional", false, TestConnection_ResetSessionPool},
	{"TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls_NullScanners", "functional", false, TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls_NullScanners},
	{"TestDriver_Prepared_Insert_Nclob_Small", "functional", false, TestDriver_Prepared_Insert_Nclob_Small},
	{"TestDriver_Select_NumericFloatTypes_Prepared_Named", "functional", false, TestDriver_Select_NumericFloatTypes_Prepared_Named},
	{"TestDriver_TCPS_DN_Components_WhiteSpaces", "manual", false, TestDriver_TCPS_DN_Components_WhiteSpaces},
	{"TestDriver_TCPS_Handshake_EnforcesDNMatching_WhitespaceMismatchRejection", "manual", false, TestDriver_TCPS_Handshake_EnforcesDNMatching_WhitespaceMismatchRejection},
	{"TestDriver_TCPS_InvalidCertDn", "manual", false, TestDriver_TCPS_InvalidCertDn},
	{"TestDriver_TCPS_SSL_SERVER_DN_MATCH_DEFAULT", "manual", false, TestDriver_TCPS_SSL_SERVER_DN_MATCH_DEFAULT},
	{"TestDriver_TCPS_SSL_SERVER_DN_MATCH_OFF", "manual", false, TestDriver_TCPS_SSL_SERVER_DN_MATCH_OFF},
	{"TestDriver_Table_Create_Multiple_Connections", "functional", false, TestDriver_Table_Create_Multiple_Connections},
	{"TestIssue_ColumnTypeDatabaseCharTypeName", "functional", false, TestIssue_ColumnTypeDatabaseCharTypeName},
	{"TestIssue_ColumnTypeDatabaseTypeName", "functional", false, TestIssue_ColumnTypeDatabaseTypeName},
	{"TestIssue_ColumnTypePrecisionScale", "functional", false, TestIssue_ColumnTypePrecisionScale},
	{"TestIssue_DecodeBinaryColumnType", "functional", false, TestIssue_DecodeBinaryColumnType},
	{"TestServerError", "functional", false, TestServerError},
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

// TestConfig structure that represents a testing configuration
// A configuration contains any information neede to connect to a database
type TestConfig struct {
	ConfigName      string `json:"config_name"`
	DatabaseVersion int    `json:"database_version"`
	Enabled         bool

	Driver struct {
		Name string
	}

	Database struct {
		ServiceName  string
		SIDName      string `json:",omitempty"`
		InstanceName string `json:",omitempty"`
		Port         int16
		Host         string
		Protocol     string
		ServerType   string `json:",omitempty"` // dedicated/shared
	}

	Credentials struct {
		Username  string
		Password  string
		LogonMode string
	}

	Security struct {
		WalletLocation      string `json:"wallet_location,omitempty"`
		SslServerDnMatch    string `json:"ssl_server_dn_match,omitempty"`
		SslServerCertDn     string `json:"ssl_server_cert_dn,omitempty"`
		SslAllowWeakDnMatch string `json:"ssl_allow_weak_dn_match,omitempty"`
	}

	ConnectionProperties struct {
		StrictNullValueHandling string `json:"oracle.go.StrictNullValueHandling"`
	}
}

// _assignStringIfNeeded assign "from" value to src if from is a valid string
func _assignStringIfNeeded(src *string, from string) {
	if len(strings.TrimSpace(from)) > 0 {
		*src = from
	}
}

// _assignIntIfNeeded assign "from" value to src if from is a valid int
func _assignIntIfNeeded(src *int16, from int16) {
	if from >= 0 {
		*src = from
	}
}

// Clone clones a test config
func (t *TestConfig) Clone() *TestConfig {
	newOne := &TestConfig{}
	newOne.Driver.Name = t.Driver.Name
	newOne.Database.ServiceName = t.Database.ServiceName
	newOne.Database.SIDName = t.Database.SIDName
	newOne.Database.InstanceName = t.Database.InstanceName
	newOne.Database.Host = t.Database.Host
	newOne.Database.Port = t.Database.Port
	newOne.Database.Protocol = t.Database.Protocol
	newOne.Database.ServerType = t.Database.ServerType

	newOne.Credentials.Username = t.Credentials.Username
	newOne.Credentials.Password = t.Credentials.Password
	newOne.Credentials.LogonMode = t.Credentials.LogonMode

	newOne.Security = t.Security

	return newOne
}

// MergeWith merges config "from" with value currently assigned
//
//	returned the merged config
func (t *TestConfig) MergeWith(from *TestConfig) {

	_assignStringIfNeeded(&(t.Driver.Name), from.Driver.Name)

	_assignStringIfNeeded(&(t.Database.ServiceName), from.Database.ServiceName)
	_assignStringIfNeeded(&(t.Database.SIDName), from.Database.SIDName)
	_assignStringIfNeeded(&(t.Database.InstanceName), from.Database.InstanceName)
	_assignStringIfNeeded(&(t.Database.ServerType), from.Database.ServerType)
	_assignIntIfNeeded(&(t.Database.Port), from.Database.Port)
	_assignStringIfNeeded(&(t.Database.Host), from.Database.Protocol)
	_assignStringIfNeeded(&(t.Database.Protocol), from.Database.Protocol)

	_assignStringIfNeeded(&(t.Credentials.Username), from.Credentials.Username)
	_assignStringIfNeeded(&(t.Credentials.Password), from.Credentials.Password)
	_assignStringIfNeeded(&(t.Credentials.LogonMode), from.Credentials.LogonMode)

	_assignStringIfNeeded(&(t.Security.WalletLocation), from.Security.WalletLocation)
	_assignStringIfNeeded(&(t.Security.SslServerDnMatch), from.Security.SslServerDnMatch)
	_assignStringIfNeeded(&(t.Security.SslServerCertDn), from.Security.SslServerCertDn)
	_assignStringIfNeeded(&(t.Security.SslAllowWeakDnMatch), from.Security.SslAllowWeakDnMatch)
}

// GetConnectionString Build a connection string from a test config
func (t *TestConfig) GetConnectionDSN() string {
	dsn := t.GetConnectionStringWithProperties(nil)
	s := strings.SplitN(dsn, "@", 2)
	return s[1]
}

// GetConnectionString Build a connection string from a test config
func (t *TestConfig) GetConnectionString() string {
	return t.GetConnectionStringWithProperties(nil)
}

// GetConnectionStringWithProperties Build a connection string from a test config and some properties
func (t *TestConfig) GetConnectionStringWithProperties(properties map[string]string) string {
	var b strings.Builder
	if properties != nil {
		for k, v := range properties {
			b.WriteString(fmt.Sprintf("(%s=%s)", k, v))
		}
	}
	var res = fmt.Sprintf("%s/%s@(description=%s(address=(protocol=%s)(host=%s)(port=%d))(connect_data=",
		t.Credentials.Username,
		t.Credentials.Password,
		b.String(),
		t.Database.Protocol,
		t.Database.Host,
		t.Database.Port)

	var resC strings.Builder
	resC.WriteString(res)
	if len(t.Database.ServiceName) > 0 {
		resC.WriteString(fmt.Sprintf("(service_name=%s)", t.Database.ServiceName))
		if len(t.Database.InstanceName) > 0 {
			resC.WriteString(fmt.Sprintf("(instance_name=%s)", t.Database.InstanceName))
		}
	} else {
		if len(t.Database.SIDName) > 0 {
			resC.WriteString(fmt.Sprintf("(sid=%s)", t.Database.SIDName))
		}
	}

	if len(t.Database.ServerType) > 0 {
		resC.WriteString(fmt.Sprintf("(server=%s)", t.Database.ServerType))
	}

	resC.WriteString(")") // close connect_data

	// Add security if present
	if t.Security.WalletLocation != "" || t.Security.SslServerDnMatch != "" ||
		t.Security.SslServerCertDn != "" || t.Security.SslAllowWeakDnMatch != "" {
		resC.WriteString("(security=")
		if t.Security.WalletLocation != "" {
			resC.WriteString(fmt.Sprintf("(wallet_location=%s)", t.Security.WalletLocation))
		}
		if t.Security.SslServerDnMatch != "" {
			resC.WriteString(fmt.Sprintf("(ssl_server_dn_match=%s)", t.Security.SslServerDnMatch))
		}
		if t.Security.SslAllowWeakDnMatch != "" {
			resC.WriteString(fmt.Sprintf("(ssl_allow_weak_dn_match=%s)", t.Security.SslAllowWeakDnMatch))
		}
		if t.Security.SslServerCertDn != "" {
			resC.WriteString(fmt.Sprintf("(ssl_server_cert_dn=\"%s\")", t.Security.SslServerCertDn))
		}
		resC.WriteString(")")
	}

	resC.WriteString(")") // close description

	if len(t.Credentials.LogonMode) > 0 {
		resC.WriteString(fmt.Sprintf("?oracle.go.Credentials.logonMode=%s", t.Credentials.LogonMode))
	}
	return resC.String()
}

// GetConnectionStringWithMergedConfig Build a connection string from a test config after merging with a given config
func (t *TestConfig) GetConnectionStringWithMergedConfig(config *TestConfig) string {

	_c := t.Clone()
	_c.MergeWith(config)
	return _c.GetConnectionStringWithProperties(nil)

}

// TestingEnvironment Holds driver configuration for tests
type TestingEnvironment struct {
	// Testing configuration array parsec from YAML file
	driverConfigs []TestConfig
}

// DefaultTestConfig Default reference to TestEnvironement
// That should not be that way but we need
// a way to pass config to sub package.
// that will be removed after refactoring
var DefaultTestConfig *TestConfig

// NewTestingEnvironment creates a new environment for given file
// On failure, error is returned
func NewTestingEnvironment(fileName string) (TestingEnvironment, error) {

	var driverConfigs []TestConfig

	// load YAML file
	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return TestingEnvironment{},
			fmt.Errorf("specified configuration file %s do not exists", fileName)
	}
	f, err := os.Open(fileName)
	if err != nil {
		return TestingEnvironment{},
			fmt.Errorf("unable to open configuration %s: %v",
				fileName,
				err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			//ignored
		}
	}(f)

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&driverConfigs)
	if err != nil {
		return TestingEnvironment{},
			fmt.Errorf("unable to read configuration %s: %w", fileName, err)
	}

	return TestingEnvironment{
		driverConfigs: driverConfigs,
	}, nil
}

// GetConfig gets a configuration by name.
// Returns null if the configuration is not found
func (e *TestingEnvironment) GetConfig(name string) (*TestConfig, error) {
	if e.driverConfigs == nil {
		return nil, fmt.Errorf("attempt to get a configuration but not configuration available")
	}
	for _, config := range e.driverConfigs {
		if config.ConfigName == name {
			return &config, nil
		}
	}
	return nil, fmt.Errorf("no configuration %s found", name)

}

// Test configuration flag. This flag gives configuration file path
//
//	is mandatory to run the test
var configFileName string

// Test configuration flag. This flag will specify which
// configuration to use.
var configName string

var TestEnvironement TestingEnvironment

// TestingConfig Usable by tests, may be nil if not flag provided
var TestingConfig *TestConfig

// TestCategory category of tests to be un
var TestCategory string

// InitConfig init the environment configuration
// Main task is to parse dirver config flags and populate the
// default configuration
func InitConfig() error {

	flag.StringVar(&configFileName, "driver.config.filename", "", "testing config name")
	flag.StringVar(&configName, "driver.config.name", "", "testing config name")

	flag.StringVar(&TestCategory, "test.category", "", "testing category, can be unitary, functional, performance, robustness")
	if !flag.Parsed() {
		flag.Parse()
	}
	if len(configFileName) != 0 {
		env, err := NewTestingEnvironment(configFileName)
		if err != nil {
			return fmt.Errorf("cannot get test environment : %w", err)
		}
		TestEnvironement = env

		if len(configName) != 0 {
			TestingConfig, _ = TestEnvironement.GetConfig(configName)
			// Keep DefaultTestConfig in sync for legacy driver API
			DefaultTestConfig = TestingConfig
		}
	}
	return nil
}
