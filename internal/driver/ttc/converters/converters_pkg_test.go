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

package converters

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
	{Name: "TestVarchar_EncodeDecode_Roundtrip", Categories: "unitary", Exclusive: false, Fn: TestVarchar_EncodeDecode_Roundtrip},
	{Name: "TestVarchar_Encode_Table", Categories: "unitary", Exclusive: false, Fn: TestVarchar_Encode_Table},
	{Name: "TestVarchar_Decode_Table", Categories: "unitary", Exclusive: false, Fn: TestVarchar_Decode_Table},
	{Name: "TestVarchar_Encode_Output", Categories: "unitary", Exclusive: false, Fn: TestVarchar_Encode_Output},
	{Name: "TestVarchar_AL32UTF8_Encode_Output", Categories: "unitary", Exclusive: false, Fn: TestVarchar_AL32UTF8_Encode_Output},
	{Name: "TestChar_Encode_Table", Categories: "unitary", Exclusive: false, Fn: TestChar_Encode_Table},
	{Name: "TestChar_Decode_Padded_Table", Categories: "unitary", Exclusive: false, Fn: TestChar_Decode_Padded_Table},
	{Name: "TestChar_Decode_NoTrim_PreservesPadding", Categories: "unitary", Exclusive: false, Fn: TestChar_Decode_NoTrim_PreservesPadding},
	{Name: "TestChar_AL32UTF8_Encode_Output", Categories: "unitary", Exclusive: false, Fn: TestChar_AL32UTF8_Encode_Output},
	{Name: "TestChar_DecodeNullString", Categories: "unitary", Exclusive: false, Fn: TestChar_DecodeNullString},
	{Name: "TestNVarchar2_Encode_Table", Categories: "unitary", Exclusive: false, Fn: TestNVarchar2_Encode_Table},
	{Name: "TestNVarchar2_Decode_Table", Categories: "unitary", Exclusive: false, Fn: TestNVarchar2_Decode_Table},
	{Name: "TestNVarchar2UTF8_Decode_Table", Categories: "unitary", Exclusive: false, Fn: TestNVarchar2UTF8_Decode_Table},
	{Name: "TestNChar_Encode_Table", Categories: "unitary", Exclusive: false, Fn: TestNChar_Encode_Table},
	{Name: "TestNChar_Decode_Table", Categories: "unitary", Exclusive: false, Fn: TestNChar_Decode_Table},
	{Name: "TestNCharUTF8_Decode_Table", Categories: "unitary", Exclusive: false, Fn: TestNCharUTF8_Decode_Table},
	{Name: "TestNCharAL16UTF16_OddLength_ReturnsOracleError", Categories: "unitary", Exclusive: false, Fn: TestNCharAL16UTF16_OddLength_ReturnsOracleError},
	{Name: "TestDecodeUTF16BEToString_OddLength_ReturnsOracleError", Categories: "unitary", Exclusive: false, Fn: TestDecodeUTF16BEToString_OddLength_ReturnsOracleError},
	{Name: "TestDateDecode", Categories: "unitary", Exclusive: false, Fn: TestDateDecode},
	{Name: "TestDateEncode", Categories: "unitary", Exclusive: false, Fn: TestDateEncode},
	{Name: "TestTimestampDecode", Categories: "unitary", Exclusive: false, Fn: TestTimestampDecode},
	{Name: "TestTimestampEncode", Categories: "unitary", Exclusive: false, Fn: TestTimestampEncode},
	{Name: "TestTimestampWithTimeZoneDecode", Categories: "unitary", Exclusive: false, Fn: TestTimestampWithTimeZoneDecode},
	{Name: "TestTimestampWithTimeZoneEncode", Categories: "unitary", Exclusive: false, Fn: TestTimestampWithTimeZoneEncode},
	{Name: "TestTimestampWithLocalTimeZoneDecode", Categories: "unitary", Exclusive: false, Fn: TestTimestampWithLocalTimeZoneDecode},
	{Name: "TestTimestampWithLocalTimeZoneEncode", Categories: "unitary", Exclusive: false, Fn: TestTimestampWithLocalTimeZoneEncode},
	{Name: "TestFloat_Decode", Categories: "unitary", Exclusive: false, Fn: TestFloat_Decode},
	{Name: "TestFloat_Encode", Categories: "unitary", Exclusive: false, Fn: TestFloat_Encode},
	{Name: "TestBinaryFloat_Encode", Categories: "unitary", Exclusive: false, Fn: TestBinaryFloat_Encode},
	{Name: "TestBinaryFloat_Decode", Categories: "unitary", Exclusive: false, Fn: TestBinaryFloat_Decode},
	{Name: "TestBinaryDouble_Encode", Categories: "unitary", Exclusive: false, Fn: TestBinaryDouble_Encode},
	{Name: "TestBinaryDouble_Decode", Categories: "unitary", Exclusive: false, Fn: TestBinaryDouble_Decode},
	{Name: "TestFloat_Encode_Specials", Categories: "unitary", Exclusive: false, Fn: TestFloat_Encode_Specials},
	{Name: "TestIntervalYearToMonth_Decode", Categories: "unitary", Exclusive: false, Fn: TestIntervalYearToMonth_Decode},
	{Name: "TestIntervalYearToMonth_Encode", Categories: "unitary", Exclusive: false, Fn: TestIntervalYearToMonth_Encode},
	{Name: "TestIntervalYearToMonth_Encode_Errors", Categories: "unitary", Exclusive: false, Fn: TestIntervalYearToMonth_Encode_Errors},
	{Name: "TestIntervalYearToMonth_Decode_Errors", Categories: "unitary", Exclusive: false, Fn: TestIntervalYearToMonth_Decode_Errors},
	{Name: "TestIntervalDayToSecond_Decode", Categories: "unitary", Exclusive: false, Fn: TestIntervalDayToSecond_Decode},
	{Name: "TestIntervalDayToSecond_Encode", Categories: "unitary", Exclusive: false, Fn: TestIntervalDayToSecond_Encode},
	{Name: "TestIntervalDayToSecond_Decode_Errors", Categories: "unitary", Exclusive: false, Fn: TestIntervalDayToSecond_Decode_Errors},
	{Name: "TestIntervalDayToSecond_Encode_Errors", Categories: "unitary", Exclusive: false, Fn: TestIntervalDayToSecond_Encode_Errors},
	{Name: "TestIntervalYearToMonth_Encode_Whitespace", Categories: "unitary", Exclusive: false, Fn: TestIntervalYearToMonth_Encode_Whitespace},
	{Name: "TestIntervalYearToMonth_Encode_TrimWhitespace_Success", Categories: "unitary", Exclusive: false, Fn: TestIntervalYearToMonth_Encode_TrimWhitespace_Success},
	{Name: "TestIntervalDayToSecond_Encode_Errors_Whitespace", Categories: "unitary", Exclusive: false, Fn: TestIntervalDayToSecond_Encode_Errors_Whitespace},
	{Name: "TestIntervalDayToSecond_Encode_TrimWhitespace_Success", Categories: "unitary", Exclusive: false, Fn: TestIntervalDayToSecond_Encode_TrimWhitespace_Success},
	{Name: "TestNumber_KnownWireFormats", Categories: "unitary", Exclusive: false, Fn: TestNumber_KnownWireFormats},
	{Name: "TestNumber_DecodeDecimal", Categories: "unitary", Exclusive: false, Fn: TestNumber_DecodeDecimal},
	{Name: "TestNumber_OverlongWire", Categories: "unitary", Exclusive: false, Fn: TestNumber_OverlongWire},
	{Name: "TestNumber_FromNumberBig_WireLengthBoundary", Categories: "unitary", Exclusive: false, Fn: TestNumber_FromNumberBig_WireLengthBoundary},
	{Name: "TestNumber_DecodeDecimal_ScaleBounds", Categories: "unitary", Exclusive: false, Fn: TestNumber_DecodeDecimal_ScaleBounds},
	{Name: "TestNumber_DecodeDecimal_ZeroScaleBounds", Categories: "unitary", Exclusive: false, Fn: TestNumber_DecodeDecimal_ZeroScaleBounds},
	{Name: "TestNumber_DecodeDecimal_PrecisionBounds", Categories: "unitary", Exclusive: false, Fn: TestNumber_DecodeDecimal_PrecisionBounds},
	{Name: "TestNumber_EncodeDecimal", Categories: "unitary", Exclusive: false, Fn: TestNumber_EncodeDecimal},
	{Name: "TestNumber_EncodeDecode_Roundtrip", Categories: "unitary", Exclusive: false, Fn: TestNumber_EncodeDecode_Roundtrip},
	{Name: "TestNumber_OverflowHandling", Categories: "unitary", Exclusive: false, Fn: TestNumber_OverflowHandling},
	{Name: "TestNumber_DecodeInt_MinInt64", Categories: "unitary", Exclusive: false, Fn: TestNumber_DecodeInt_MinInt64},
	{Name: "TestNumber_ErrorCodes", Categories: "unitary", Exclusive: false, Fn: TestNumber_ErrorCodes},
	{Name: "TestNumber_EncodeDecimal_StrictErrors", Categories: "unitary", Exclusive: false, Fn: TestNumber_EncodeDecimal_StrictErrors},
	{Name: "TestNumber_DecodeDecimal_NegativeScale_Rounding", Categories: "unitary", Exclusive: false, Fn: TestNumber_DecodeDecimal_NegativeScale_Rounding},
	{Name: "TestEncodeUInt_KnownWireFormats_Positive", Categories: "unitary", Exclusive: false, Fn: TestEncodeUInt_KnownWireFormats_Positive},
	{Name: "TestEncodeUInt_EncodeDecode_Roundtrip", Categories: "unitary", Exclusive: false, Fn: TestEncodeUInt_EncodeDecode_Roundtrip},
	{Name: "TestEncodeUInt_OverflowOnDecodeInt", Categories: "unitary", Exclusive: false, Fn: TestEncodeUInt_OverflowOnDecodeInt},
	{Name: "TestEncodeBoolean_Wires", Categories: "unitary", Exclusive: false, Fn: TestEncodeBoolean_Wires},
	{Name: "TestDecodeBoolean_FromKnownWires", Categories: "unitary", Exclusive: false, Fn: TestDecodeBoolean_FromKnownWires},
	{Name: "TestEncodeBinary_ReturnsSameBytes", Categories: "unitary", Exclusive: false, Fn: TestEncodeBinary_ReturnsSameBytes},
	{Name: "TestEncodeNull", Categories: "unitary", Exclusive: false, Fn: TestEncodeNull},
	{Name: "TestEncodeBooleanAsNumber", Categories: "unitary", Exclusive: false, Fn: TestEncodeBooleanAsNumber},
}
