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

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestDriver_ConfigurationWithConnectorBasic", Categories: "functional", Exclusive: true, Fn: TestDriver_ConfigurationWithConnectorBasic},
	{Name: "TestDriver_ConfigurationWithConnectorWithEnvOverwrite", Categories: "functional", Exclusive: true, Fn: TestDriver_ConfigurationWithConnectorWithEnvOverwrite},
	{Name: "TestDriver_ConfigurationWithConnectorWithFlagOverwrite", Categories: "functional", Exclusive: true, Fn: TestDriver_ConfigurationWithConnectorWithFlagOverwrite},
	{Name: "TestConfiguration_AssignFromEmptyMap", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_AssignFromEmptyMap},
	{Name: "TestConfiguration_AssignFromMapUnknownKey", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_AssignFromMapUnknownKey},
	{Name: "TestConfiguration_AssignFromMap", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_AssignFromMap},
	{Name: "TestConfiguration_AssignFromMapValidatedIntString", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_AssignFromMapValidatedIntString},
	{Name: "TestConfiguration_AssignFromEnv", Categories: "unitary", Exclusive: true, Fn: TestConfiguration_AssignFromEnv},
	{Name: "TestConfiguration_AssignFromEnvValidatedIntString", Categories: "unitary", Exclusive: true, Fn: TestConfiguration_AssignFromEnvValidatedIntString},
	{Name: "TestConfiguration_AssignFromEmptyFlags", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_AssignFromEmptyFlags},
	{Name: "TestConfiguration_Clone", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_Clone},
	{Name: "TestConfiguration_DefaultClientLanguageIsLanguageTag", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_DefaultClientLanguageIsLanguageTag},
	{Name: "TestConfiguration_AssignFromMapClientLanguageTag", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_AssignFromMapClientLanguageTag},
	{Name: "TestConfiguration_AssignFromEnvClientLanguageTag", Categories: "unitary", Exclusive: true, Fn: TestConfiguration_AssignFromEnvClientLanguageTag},
	{Name: "TestConfiguration_toNSConnectionParameters", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_toNSConnectionParameters},
	{Name: "TestConfiguration_InitLoggingWithConfigFileDestination", Categories: "unitary", Exclusive: false, Fn: TestConfiguration_InitLoggingWithConfigFileDestination},
	{Name: "TestEnquoteLiteral", Categories: "unitary", Exclusive: false, Fn: TestEnquoteLiteral},
	{Name: "TestEnquoteNCharLiteral", Categories: "unitary", Exclusive: false, Fn: TestEnquoteNCharLiteral},
	{Name: "TestIsSimpleIdentifier", Categories: "unitary", Exclusive: false, Fn: TestIsSimpleIdentifier},
	{Name: "TestEnquoteIdentifier", Categories: "unitary", Exclusive: false, Fn: TestEnquoteIdentifier},
	{Name: "TestDriver_ConfigurationWithCredentialsWithDsnNegative", Categories: "unitary", Exclusive: false, Fn: TestDriver_ConfigurationWithCredentialsWithDsnNegative},
	{Name: "TestDriver_ConfigurationLogging", Categories: "unitary", Exclusive: false, Fn: TestDriver_ConfigurationLogging},
	{Name: "TestDriver_OpenConnectorUsesNSParamOverConfig", Categories: "unitary", Exclusive: false, Fn: TestDriver_OpenConnectorUsesNSParamOverConfig},
	{Name: "TestDriver_Table_Create", Categories: "sanity", Exclusive: false, Fn: TestDriver_Table_Create},
	{Name: "TestDriver_DropTable_DeniesAccess", Categories: "functional", Exclusive: false, Fn: TestDriver_DropTable_DeniesAccess},
	{Name: "TestDriver_AlterSessionSetLanguage", Categories: "functional", Exclusive: false, Fn: TestDriver_AlterSessionSetLanguage},
	{Name: "TestDriver_Table_Insert", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Insert},
	{Name: "TestDriver_Insert_Select", Categories: "functional", Exclusive: false, Fn: TestDriver_Insert_Select},
	{Name: "TestDriver_PLSQL_AnonymousBlock_Sanity", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_AnonymousBlock_Sanity},
	{Name: "TestDriver_PLSQL_CreateInsertSelectDrop", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_CreateInsertSelectDrop},
	{Name: "TestDriver_RefCursorOut", Categories: "functional", Exclusive: false, Fn: TestDriver_RefCursorOut},
	{Name: "TestDriver_RefCursorMultipleOut", Categories: "functional", Exclusive: false, Fn: TestDriver_RefCursorMultipleOut},
	{Name: "TestDriver_ImplicitResults", Categories: "functional", Exclusive: false, Fn: TestDriver_ImplicitResults},
	{Name: "TestDriver_RefCursorOutWithScalar", Categories: "functional", Exclusive: false, Fn: TestDriver_RefCursorOutWithScalar},
	{Name: "TestDriver_PLSQL_BreakCausedByTimeout", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_BreakCausedByTimeout},
	{Name: "TestDriver_Select_BooleanTypes_23c", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_BooleanTypes_23c},
	{Name: "TestDriver_Select_CharacterTypes", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_CharacterTypes},
	{Name: "TestDriver_Select_DATE", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_DATE},
	{Name: "TestDriver_Select_TIMESTAMP", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TIMESTAMP},
	{Name: "TestDriver_Select_TimestampWithTimeZone", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TimestampWithTimeZone},
	{Name: "TestDriver_Select_TimestampWithLocalTimeZone", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TimestampWithLocalTimeZone},
	{Name: "TestDriver_Select_Intervals", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_Intervals},
	{Name: "TestDriver_Select_NumericFloatTypes", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_NumericFloatTypes},
	{Name: "TestDriver_Select_Number_NoPrecisionScale", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_Number_NoPrecisionScale},
	{Name: "TestDriver_Select_NumberPrecision", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_NumberPrecision},
	{Name: "TestDriver_VarcharLargePayload", Categories: "functional", Exclusive: false, Fn: TestDriver_VarcharLargePayload},
	{Name: "TestDriver_Select_Number_MaxPrecisionScale", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_Number_MaxPrecisionScale},
	{Name: "TestDriver_Select_Number_MaxPrecisionInteger", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_Number_MaxPrecisionInteger},
	{Name: "TestDriver_Select_Number_And_BinaryDouble", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_Number_And_BinaryDouble},
	{Name: "TestDriver_Table_Select", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Select},
	{Name: "TestTimeoutConnectWithTransportConnectTimeout", Categories: "functional", Exclusive: false, Fn: TestTimeoutConnectWithTransportConnectTimeout},
	{Name: "TestTimeoutConnectWithTransportConnectTimeoutAndContext", Categories: "functional", Exclusive: false, Fn: TestTimeoutConnectWithTransportConnectTimeoutAndContext},
	{Name: "TestTimeoutConnectWithTransportConnectTimeoutAndContextGreater", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectWithTransportConnectTimeoutAndContextGreater},
	{Name: "TestTimeoutConnectWithRecvTimeout", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectWithRecvTimeout},
	{Name: "TestTimeoutConnectWithConnectTimeout", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectWithConnectTimeout},
	{Name: "TestTimeoutConnectWithRecvConnectTimeoutAndContext", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectWithRecvConnectTimeoutAndContext},
	{Name: "TestTimeoutConnectWithRecvConnectTimeoutAndContextGreater", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectWithRecvConnectTimeoutAndContextGreater},
	{Name: "TestTimeoutConnectTimeoutPrecedence1", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectTimeoutPrecedence1},
	{Name: "TestTimeoutConnectTimeoutPrecedence2", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectTimeoutPrecedence2},
	{Name: "TestTimeoutConnectTimeoutPrecedence3", Categories: "functional/cyclops", Exclusive: false, Fn: TestTimeoutConnectTimeoutPrecedence3},
	{Name: "TestTimeoutConnectTimeoutPrecedence4", Categories: "functional", Exclusive: false, Fn: TestTimeoutConnectTimeoutPrecedence4},
	{Name: "TestDriver_Functional_SelectDual", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_Functional_SelectDual},
	{Name: "TestDriver_SimpleConnection", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_SimpleConnection},
	{Name: "TestDriver_Authentication_TTIWRN", Categories: "functional", Exclusive: false, Fn: TestDriver_Authentication_TTIWRN},
	{Name: "TestDriver_Authentication_OCIToken", Categories: "functional", Exclusive: false, Fn: TestDriver_Authentication_OCIToken},
	{Name: "TestDriver_Authentication_OAuth", Categories: "functional", Exclusive: false, Fn: TestDriver_Authentication_OAuth},
	{Name: "TestDriver_TCPS_Pipeline_SelectDual", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_TCPS_Pipeline_SelectDual},
	{Name: "TestDriver_TCPS_Pipeline_InvalidCertDn", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_TCPS_Pipeline_InvalidCertDn},
	{Name: "TestDriver_TCPS_Pipeline_InvalidWalletLocation", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_TCPS_Pipeline_InvalidWalletLocation},
	{Name: "TestDriver_TCPS_Pipeline_DNMatchOff_AllowsInvalidDN", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_TCPS_Pipeline_DNMatchOff_AllowsInvalidDN},
	{Name: "TestDriver_Prepared_Insert_Select_Ordinal", Categories: "sanity,functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Select_Ordinal},
	{Name: "TestDriver_Prepared_Insert_Select_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Select_Named},
	{Name: "TestDriver_PLSQL_Prepared_Binds", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_Prepared_Binds},
	{Name: "TestDriver_Select_BooleanTypes_23c_Prepared_Statement", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_BooleanTypes_23c_Prepared_Statement},
	{Name: "TestDriver_Select_BooleanTypes_23c_Prepared_Statement_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_BooleanTypes_23c_Prepared_Statement_Named},
	{Name: "TestDriver_Select_BooleanTypes_19c", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_BooleanTypes_19c},
	{Name: "TestDriver_Select_BooleanTypes_19c_Prepared_Statement", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_BooleanTypes_19c_Prepared_Statement},
	{Name: "TestDriver_Select_BooleanTypes_19c_Prepared_Statement_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_BooleanTypes_19c_Prepared_Statement_Named},
	{Name: "TestDriver_Select_CharacterTypes_Ordinal", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_CharacterTypes_Ordinal},
	{Name: "TestDriver_Select_CharacterTypes_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_CharacterTypes_Named},
	{Name: "TestDriver_Select_DATE_Prepared_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_DATE_Prepared_Named},
	{Name: "TestDriver_Select_TIMESTAMP_Prepared_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TIMESTAMP_Prepared_Named},
	{Name: "TestDriver_Select_TimestampWithTimeZone_Prepared_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TimestampWithTimeZone_Prepared_Named},
	{Name: "TestDriver_Select_TimestampWithLocalTimeZone_Prepared_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TimestampWithLocalTimeZone_Prepared_Named},
	{Name: "TestDriver_Select_TimestampWithTimeZone_Prepared_LoadLocation_And_Offset", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_TimestampWithTimeZone_Prepared_LoadLocation_And_Offset},
	{Name: "TestDriver_Prepared_InsertAndSelect_AllTypes", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_InsertAndSelect_AllTypes},
	{Name: "TestDriver_Prepared_InsertAndSelect_AllTypes_StrictNulls", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_InsertAndSelect_AllTypes_StrictNulls},
	{Name: "TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls},
	{Name: "TestDriver_Prepared_Statement_Re_exec_With_Different_Arg_Counts_Negative", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Statement_Re_exec_With_Different_Arg_Counts_Negative},
	{Name: "TestDriver_Exec_Query_cursor_leak", Categories: "robustness", Exclusive: false, Fn: TestDriver_Exec_Query_cursor_leak},
	{Name: "TestDriver_Select_Query_cursor_leak", Categories: "robustness", Exclusive: false, Fn: TestDriver_Select_Query_cursor_leak},
	{Name: "TestDriver_PreparedStatement_Query_cursor_leak", Categories: "robustness", Exclusive: false, Fn: TestDriver_PreparedStatement_Query_cursor_leak},
	{Name: "TestQueryNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestQueryNonExistentTable_NegativeCase},
	{Name: "TestPreparedStatementNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestPreparedStatementNonExistentTable_NegativeCase},
	{Name: "TestSelectSpecificColumnsNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestSelectSpecificColumnsNonExistentTable_NegativeCase},
	{Name: "TestCountQueryNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestCountQueryNonExistentTable_NegativeCase},
	{Name: "TestJoinWithNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestJoinWithNonExistentTable_NegativeCase},
	{Name: "TestSubqueryWithNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestSubqueryWithNonExistentTable_NegativeCase},
	{Name: "TestDescribeNonExistentTable_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestDescribeNonExistentTable_NegativeCase},
	{Name: "TestInvalidTableNameSyntax_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestInvalidTableNameSyntax_NegativeCase},
	{Name: "TestQuerySystemTableWithoutPrivilege_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestQuerySystemTableWithoutPrivilege_NegativeCase},
	{Name: "TestQueryAccessibleTable_PositiveCase", Categories: "functional", Exclusive: false, Fn: TestQueryAccessibleTable_PositiveCase},
	{Name: "TestQueryDictionaryViewWithoutPrivilege_NegativeCase", Categories: "functional", Exclusive: false, Fn: TestQueryDictionaryViewWithoutPrivilege_NegativeCase},
	{Name: "TestInsertAndSelectSmallRAW", Categories: "functional", Exclusive: false, Fn: TestInsertAndSelectSmallRAW},
	{Name: "TestInsertAndSelectLargeRAW", Categories: "functional", Exclusive: false, Fn: TestInsertAndSelectLargeRAW},
	{Name: "TestInsertAndSelectNullRAW", Categories: "functional", Exclusive: false, Fn: TestInsertAndSelectNullRAW},
	{Name: "TestRAWMultipleRows", Categories: "functional", Exclusive: false, Fn: TestRAWMultipleRows},
	{Name: "TestRAWUpdateOperation", Categories: "functional", Exclusive: false, Fn: TestRAWUpdateOperation},
	{Name: "TestRAWTypeSystemIntegration", Categories: "functional", Exclusive: false, Fn: TestRAWTypeSystemIntegration},
	{Name: "TestDriver_Prepared_InsertMultipleRows_Re_exec", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_InsertMultipleRows_Re_exec},
	{Name: "TestDriver_Bind_ReusedNamedParameter", Categories: "functional", Exclusive: false, Fn: TestDriver_Bind_ReusedNamedParameter},
	{Name: "TestDriver_Prepared_SelectMultipleRows_Re_exec_BindTypeChange", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_SelectMultipleRows_Re_exec_BindTypeChange},
	{Name: "TestDriver_Prepared_SelectMultipleRows_Re_exec", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_SelectMultipleRows_Re_exec},

	{Name: "TestCommit", Categories: "functional", Exclusive: false, Fn: TestCommit},
	{Name: "TestRollback", Categories: "functional", Exclusive: false, Fn: TestRollback},
	{Name: "TestRollbackThroughContextServerSleep", Categories: "functional", Exclusive: false, Fn: TestRollbackThroughContextServerSleep},
	{Name: "TestRollbackThroughContextCancel", Categories: "functional", Exclusive: false, Fn: TestRollbackThroughContextCancel},

	{Name: "TestDriver_Prepared_Insert_Clob_Small", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Clob_Small},
	{Name: "TestDriver_Prepared_Insert_Clob_Large", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Clob_Large},
	{Name: "TestDriver_Prepared_Insert_Blob_Small", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Blob_Small},
	{Name: "TestDriver_Prepared_Insert_Blob_Large", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Blob_Large},

	{Name: "TestReadOnlyTransaction", Categories: "functional", Exclusive: false, Fn: TestReadOnlyTransaction},

	{Name: "TestDriver_Table_Select_JSON", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Select_JSON},
	{Name: "TestDriver_Table_Select_NullJSON", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Select_NullJSON},
	{Name: "TestDriver_Table_Select_CLOB", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Select_CLOB},
	{Name: "TestDriver_Table_Select_BLOB", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Select_BLOB},
	{Name: "TestDriver_Table_Insert_Select_CLOB_BLOB_MultiRows", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Insert_Select_CLOB_BLOB_MultiRows},
	{Name: "TestDriver_Table_Insert_Select_JSON_MultiRows", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Insert_Select_JSON_MultiRows},

	{Name: "TestDriver_DMLReturning_Insert_Ordinal", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_Ordinal},
	{Name: "TestDriver_DMLReturning_Update_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Update_Named},
	{Name: "TestDriver_DMLReturning_Insert_MultipleScalarTypes", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_MultipleScalarTypes},
	{Name: "TestDriver_DMLReturning_Delete_Ordinal", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Delete_Ordinal},
	{Name: "TestDriver_DMLReturning_Update_Named_NoStmt", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Update_Named_NoStmt},
	{Name: "TestDriver_DMLReturning_ZeroRowsAffected", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_ZeroRowsAffected},
	{Name: "TestDriver_DMLReturning_Delete_ZeroRowsAffected", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Delete_ZeroRowsAffected},
	{Name: "TestDriver_DMLReturning_Insert_RAW", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_RAW},
	{Name: "TestDriver_DMLReturning_Insert_CHAR", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_CHAR},
	{Name: "TestDriver_DMLReturning_PreparedStmt_ReExecution", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_PreparedStmt_ReExecution},
	{Name: "TestDriver_DMLReturning_InTransaction_Rollback", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_InTransaction_Rollback},
	{Name: "TestDriver_DMLReturning_InTransaction_Commit", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_InTransaction_Commit},
	{Name: "TestDriver_DMLReturning_Insert_InOut", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_InOut},
	{Name: "TestDriver_DMLReturning_Update_MultipleRows", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Update_MultipleRows},
	{Name: "TestDriver_DMLReturning_Insert_NullableColumn", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_NullableColumn},
	{Name: "TestDriver_DMLReturning_Insert_NullBinaryFloatIntoNullFloat64", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_NullBinaryFloatIntoNullFloat64},
	{Name: "TestDriver_DMLReturning_Insert_TimestampWithLocalTZ", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_TimestampWithLocalTZ},
	{Name: "TestDriver_DMLReturning_Insert_NumberScalePrecision", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_NumberScalePrecision},
	{Name: "TestDriver_DMLReturning_BinaryFloatColumn", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_BinaryFloatColumn},
	{Name: "TestDriver_DMLReturning_Insert_BooleanColumn", Categories: "functional", Exclusive: false, Fn: TestDriver_DMLReturning_Insert_BooleanColumn},
	{Name: "TestDriver_PLSQL_InOut_NumberFunction", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_InOut_NumberFunction},
	{Name: "TestDriver_PLSQL_InOut_VarcharProcedure", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_InOut_VarcharProcedure},
	{Name: "TestDriver_PLSQL_ProcedureWithInOut", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_ProcedureWithInOut},
	{Name: "TestDriver_PLSQL_ProcedureWithInOut_AllTypes", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_ProcedureWithInOut_AllTypes},
	{Name: "TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement},
	{Name: "TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement_NamedBinds", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement_NamedBinds},

	{Name: "TestDriver_PLSQL_InOut_NumberFunctionDoubleBind", Categories: "functional", Exclusive: false, Fn: TestDriver_PLSQL_InOut_NumberFunctionDoubleBind},

	{Name: "TestDriver_TRIGGER_GormTest", Categories: "functional", Exclusive: false, Fn: TestDriver_TRIGGER_GormTest},

	{Name: "TestDriver_SQLNullTypes_BindInputs", Categories: "functional", Exclusive: false, Fn: TestDriver_SQLNullTypes_BindInputs},
	{Name: "TestDriver_SQLNullTypes_DMLReturning_OutDest", Categories: "functional", Exclusive: false, Fn: TestDriver_SQLNullTypes_DMLReturning_OutDest},
	{Name: "TestDriver_SQLNullTypes_PLSQL_InOut", Categories: "functional", Exclusive: false, Fn: TestDriver_SQLNullTypes_PLSQL_InOut},
	{Name: "TestDriver_SQLNullTypes_PLSQL_InOut_UsingSetupObjects", Categories: "functional", Exclusive: false, Fn: TestDriver_SQLNullTypes_PLSQL_InOut_UsingSetupObjects},
	{Name: "TestDriver_SQLNullTypes_PLSQL_InOut_NullInputs", Categories: "functional", Exclusive: false, Fn: TestDriver_SQLNullTypes_PLSQL_InOut_NullInputs},

	{Name: "TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails", Categories: "unitary", Exclusive: false, Fn: TestConnectorConnectDisconnectsNetworkSessionWhenInstantiatorFails},
	{Name: "TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails", Categories: "unitary", Exclusive: false, Fn: TestConnectorConnectDisconnectsNetworkSessionWhenGetConnectionFails},
	{Name: "TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess", Categories: "unitary", Exclusive: false, Fn: TestConnectorConnectLeavesNetworkSessionOpenAfterSuccess},
	{Name: "TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess", Categories: "unitary", Exclusive: false, Fn: TestConnectorConnectDoesNotReturnStaleAttemptErrorAfterLaterSuccess},
	{Name: "TestDriver_InsertForeignKeyViolation", Categories: "functional", Exclusive: false, Fn: TestDriver_InsertForeignKeyViolation},
	{Name: "TestDriver_Varchar2_TrailingSpacesPreserved", Categories: "functional", Exclusive: false, Fn: TestDriver_Varchar2_TrailingSpacesPreserved},
	{Name: "TestDriver_Varchar2_EmptyStringIsNull", Categories: "functional", Exclusive: false, Fn: TestDriver_Varchar2_EmptyStringIsNull},
	{Name: "TestDriver_Varchar2_EmbeddedNULRoundTrip", Categories: "functional", Exclusive: false, Fn: TestDriver_Varchar2_EmbeddedNULRoundTrip},
	{Name: "TestDriver_Varchar2_BoundaryLengths", Categories: "functional", Exclusive: false, Fn: TestDriver_Varchar2_BoundaryLengths},
	{Name: "TestDriver_Select_ZeroRows_FilterCondition", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_ZeroRows_FilterCondition},
	{Name: "TestDriver_Select_RowWithNullColumn", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_RowWithNullColumn},
	{Name: "TestDriver_Select_MultipleRows_SomeNulls", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_MultipleRows_SomeNulls},
	{Name: "TestDriver_Select_AllNullExceptPK", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_AllNullExceptPK},
	{Name: "TestDriver_Select_NullFromComputedExpression", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_NullFromComputedExpression},
	{Name: "TestDriver_OpenConnectorReturnsInvalidDSNParameterError", Categories: "unitary", Exclusive: false, Fn: TestDriver_OpenConnectorReturnsInvalidDSNParameterError},
	{Name: "TestDriver_OpenConnectorStoresConnectDescriptorFromDSN", Categories: "unitary", Exclusive: true, Fn: TestDriver_OpenConnectorStoresConnectDescriptorFromDSN},
	{Name: "TestDriver_OpenConnectorUsesFallbackConnectDescriptor", Categories: "unitary", Exclusive: false, Fn: TestDriver_OpenConnectorUsesFallbackConnectDescriptor},
	{Name: "TestDriver_OpenConnectorUsesNSParam", Categories: "unitary", Exclusive: false, Fn: TestDriver_OpenConnectorUsesNSParam},
	{Name: "TestDriver_OpenConnectorUsesNSProperty", Categories: "unitary", Exclusive: false, Fn: TestDriver_OpenConnectorUsesNSProperty},
	{Name: "TestDriver_OpenConnectorUsesParam", Categories: "unitary", Exclusive: false, Fn: TestDriver_OpenConnectorUsesParam},
	{Name: "TestConnection_ResetSessionKo", Categories: "functional", Exclusive: false, Fn: TestConnection_ResetSessionKo},
	{Name: "TestConnection_ResetSessionOk", Categories: "functional", Exclusive: false, Fn: TestConnection_ResetSessionOk},
	{Name: "TestConnection_ResetSessionPool", Categories: "functional", Exclusive: false, Fn: TestConnection_ResetSessionPool},
	{Name: "TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls_NullScanners", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_InsertAndSelect_AllTypes_DefaultValuesForNulls_NullScanners},
	{Name: "TestDriver_Prepared_Insert_Nclob_Small", Categories: "functional", Exclusive: false, Fn: TestDriver_Prepared_Insert_Nclob_Small},
	{Name: "TestDriver_Select_NumericFloatTypes_Prepared_Named", Categories: "functional", Exclusive: false, Fn: TestDriver_Select_NumericFloatTypes_Prepared_Named},
	{Name: "TestDriver_TCPS_DN_Components_WhiteSpaces", Categories: "manual", Exclusive: false, Fn: TestDriver_TCPS_DN_Components_WhiteSpaces},
	{Name: "TestDriver_TCPS_Handshake_EnforcesDNMatching_WhitespaceMismatchRejection", Categories: "manual", Exclusive: false, Fn: TestDriver_TCPS_Handshake_EnforcesDNMatching_WhitespaceMismatchRejection},
	{Name: "TestDriver_TCPS_InvalidCertDn", Categories: "manual", Exclusive: false, Fn: TestDriver_TCPS_InvalidCertDn},
	{Name: "TestDriver_TCPS_SSL_SERVER_DN_MATCH_DEFAULT", Categories: "manual", Exclusive: false, Fn: TestDriver_TCPS_SSL_SERVER_DN_MATCH_DEFAULT},
	{Name: "TestDriver_TCPS_SSL_SERVER_DN_MATCH_OFF", Categories: "manual", Exclusive: false, Fn: TestDriver_TCPS_SSL_SERVER_DN_MATCH_OFF},
	{Name: "TestDriver_Table_Create_Multiple_Connections", Categories: "functional", Exclusive: false, Fn: TestDriver_Table_Create_Multiple_Connections},
	{Name: "TestIssue_ColumnTypeDatabaseCharTypeName", Categories: "functional", Exclusive: false, Fn: TestIssue_ColumnTypeDatabaseCharTypeName},
	{Name: "TestIssue_ColumnTypeDatabaseTypeName", Categories: "functional", Exclusive: false, Fn: TestIssue_ColumnTypeDatabaseTypeName},
	{Name: "TestIssue_ColumnTypePrecisionScale", Categories: "functional", Exclusive: false, Fn: TestIssue_ColumnTypePrecisionScale},
	{Name: "TestIssue_DecodeBinaryColumnType", Categories: "functional", Exclusive: false, Fn: TestIssue_DecodeBinaryColumnType},
	{Name: "TestServerError", Categories: "functional", Exclusive: false, Fn: TestServerError},
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
