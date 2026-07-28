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
	{"TestVarchar_EncodeDecode_Roundtrip", "unitary", false, TestVarchar_EncodeDecode_Roundtrip},
	{"TestVarchar_Encode_Table", "unitary", false, TestVarchar_Encode_Table},
	{"TestVarchar_Decode_Table", "unitary", false, TestVarchar_Decode_Table},
	{"TestVarchar_Encode_Output", "unitary", false, TestVarchar_Encode_Output},
	{"TestVarchar_AL32UTF8_Encode_Output", "unitary", false, TestVarchar_AL32UTF8_Encode_Output},
	{"TestChar_Encode_Table", "unitary", false, TestChar_Encode_Table},
	{"TestChar_Decode_Padded_Table", "unitary", false, TestChar_Decode_Padded_Table},
	{"TestChar_Decode_NoTrim_PreservesPadding", "unitary", false, TestChar_Decode_NoTrim_PreservesPadding},
	{"TestChar_AL32UTF8_Encode_Output", "unitary", false, TestChar_AL32UTF8_Encode_Output},
	{"TestChar_DecodeNullString", "unitary", false, TestChar_DecodeNullString},
	{"TestNVarchar2_Encode_Table", "unitary", false, TestNVarchar2_Encode_Table},
	{"TestNVarchar2_Decode_Table", "unitary", false, TestNVarchar2_Decode_Table},
	{"TestNVarchar2UTF8_Decode_Table", "unitary", false, TestNVarchar2UTF8_Decode_Table},
	{"TestNChar_Encode_Table", "unitary", false, TestNChar_Encode_Table},
	{"TestNChar_Decode_Table", "unitary", false, TestNChar_Decode_Table},
	{"TestNCharUTF8_Decode_Table", "unitary", false, TestNCharUTF8_Decode_Table},
	{"TestNCharAL16UTF16_OddLength_ReturnsOracleError", "unitary", false, TestNCharAL16UTF16_OddLength_ReturnsOracleError},
	{"TestDecodeUTF16BEToString_OddLength_ReturnsOracleError", "unitary", false, TestDecodeUTF16BEToString_OddLength_ReturnsOracleError},
	{"TestDateDecode", "unitary", false, TestDateDecode},
	{"TestDateEncode", "unitary", false, TestDateEncode},
	{"TestTimestampDecode", "unitary", false, TestTimestampDecode},
	{"TestTimestampEncode", "unitary", false, TestTimestampEncode},
	{"TestTimestampWithTimeZoneDecode", "unitary", false, TestTimestampWithTimeZoneDecode},
	{"TestTimestampWithTimeZoneEncode", "unitary", false, TestTimestampWithTimeZoneEncode},
	{"TestTimestampWithLocalTimeZoneDecode", "unitary", false, TestTimestampWithLocalTimeZoneDecode},
	{"TestTimestampWithLocalTimeZoneEncode", "unitary", false, TestTimestampWithLocalTimeZoneEncode},
	{"TestFloat_Decode", "unitary", false, TestFloat_Decode},
	{"TestFloat_Encode", "unitary", false, TestFloat_Encode},
	{"TestBinaryFloat_Encode", "unitary", false, TestBinaryFloat_Encode},
	{"TestBinaryFloat_Decode", "unitary", false, TestBinaryFloat_Decode},
	{"TestBinaryDouble_Encode", "unitary", false, TestBinaryDouble_Encode},
	{"TestBinaryDouble_Decode", "unitary", false, TestBinaryDouble_Decode},
	{"TestFloat_Encode_Specials", "unitary", false, TestFloat_Encode_Specials},
	{"TestIntervalYearToMonth_Decode", "unitary", false, TestIntervalYearToMonth_Decode},
	{"TestIntervalYearToMonth_Encode", "unitary", false, TestIntervalYearToMonth_Encode},
	{"TestIntervalYearToMonth_Encode_Errors", "unitary", false, TestIntervalYearToMonth_Encode_Errors},
	{"TestIntervalYearToMonth_Decode_Errors", "unitary", false, TestIntervalYearToMonth_Decode_Errors},
	{"TestIntervalDayToSecond_Decode", "unitary", false, TestIntervalDayToSecond_Decode},
	{"TestIntervalDayToSecond_Encode", "unitary", false, TestIntervalDayToSecond_Encode},
	{"TestIntervalDayToSecond_Decode_Errors", "unitary", false, TestIntervalDayToSecond_Decode_Errors},
	{"TestIntervalDayToSecond_Encode_Errors", "unitary", false, TestIntervalDayToSecond_Encode_Errors},
	{"TestIntervalYearToMonth_Encode_Whitespace", "unitary", false, TestIntervalYearToMonth_Encode_Whitespace},
	{"TestIntervalYearToMonth_Encode_TrimWhitespace_Success", "unitary", false, TestIntervalYearToMonth_Encode_TrimWhitespace_Success},
	{"TestIntervalDayToSecond_Encode_Errors_Whitespace", "unitary", false, TestIntervalDayToSecond_Encode_Errors_Whitespace},
	{"TestIntervalDayToSecond_Encode_TrimWhitespace_Success", "unitary", false, TestIntervalDayToSecond_Encode_TrimWhitespace_Success},
	{"TestNumber_KnownWireFormats", "unitary", false, TestNumber_KnownWireFormats},
	{"TestNumber_DecodeDecimal", "unitary", false, TestNumber_DecodeDecimal},
	{"TestNumber_OverlongWire", "unitary", false, TestNumber_OverlongWire},
	{"TestNumber_FromNumberBig_WireLengthBoundary", "unitary", false, TestNumber_FromNumberBig_WireLengthBoundary},
	{"TestNumber_DecodeDecimal_ScaleBounds", "unitary", false, TestNumber_DecodeDecimal_ScaleBounds},
	{"TestNumber_DecodeDecimal_ZeroScaleBounds", "unitary", false, TestNumber_DecodeDecimal_ZeroScaleBounds},
	{"TestNumber_DecodeDecimal_PrecisionBounds", "unitary", false, TestNumber_DecodeDecimal_PrecisionBounds},
	{"TestNumber_EncodeDecimal", "unitary", false, TestNumber_EncodeDecimal},
	{"TestNumber_EncodeDecode_Roundtrip", "unitary", false, TestNumber_EncodeDecode_Roundtrip},
	{"TestNumber_OverflowHandling", "unitary", false, TestNumber_OverflowHandling},
	{"TestNumber_DecodeInt_MinInt64", "unitary", false, TestNumber_DecodeInt_MinInt64},
	{"TestNumber_ErrorCodes", "unitary", false, TestNumber_ErrorCodes},
	{"TestNumber_EncodeDecimal_StrictErrors", "unitary", false, TestNumber_EncodeDecimal_StrictErrors},
	{"TestNumber_DecodeDecimal_NegativeScale_Rounding", "unitary", false, TestNumber_DecodeDecimal_NegativeScale_Rounding},
	{"TestEncodeUInt_KnownWireFormats_Positive", "unitary", false, TestEncodeUInt_KnownWireFormats_Positive},
	{"TestEncodeUInt_EncodeDecode_Roundtrip", "unitary", false, TestEncodeUInt_EncodeDecode_Roundtrip},
	{"TestEncodeUInt_OverflowOnDecodeInt", "unitary", false, TestEncodeUInt_OverflowOnDecodeInt},
	{"TestEncodeBoolean_Wires", "unitary", false, TestEncodeBoolean_Wires},
	{"TestDecodeBoolean_FromKnownWires", "unitary", false, TestDecodeBoolean_FromKnownWires},
	{"TestEncodeBinary_ReturnsSameBytes", "unitary", false, TestEncodeBinary_ReturnsSameBytes},
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
