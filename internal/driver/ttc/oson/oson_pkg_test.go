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
** either these or other term.
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

package oson

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
	os.Exit(m.Run())
}

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestArrayNode_NestedObjectArrayTraversal", Categories: "unitary", Exclusive: false, Fn: TestArrayNode_NestedObjectArrayTraversal},
	{Name: "TestArrayNode_RejectsMalformedLayouts", Categories: "unitary", Exclusive: false, Fn: TestArrayNode_RejectsMalformedLayouts},

	{Name: "TestEncodeStringScalar_UsesExpectedStringOpcodes", Categories: "unitary", Exclusive: false, Fn: TestEncodeStringScalar_UsesExpectedStringOpcodes},
	{Name: "TestEncodeStringScalar_UsesUB4TreeSegmentSizeWhenTreeExceedsUB2", Categories: "unitary", Exclusive: false, Fn: TestEncodeStringScalar_UsesUB4TreeSegmentSizeWhenTreeExceedsUB2},
	{Name: "TestEncodeContainers_EncodesNestedObjectAndArray", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_EncodesNestedObjectAndArray},
	{Name: "TestEncodeContainers_EncodesArrayRootWithoutDictionary", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_EncodesArrayRootWithoutDictionary},
	{Name: "TestEncodeContainers_ProducesDeterministicBytesForObjectMaps", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_ProducesDeterministicBytesForObjectMaps},
	{Name: "TestFieldNameSortingOrder", Categories: "unitary", Exclusive: false, Fn: TestFieldNameSortingOrder},
	{Name: "TestEncodeScalarValues_CoverEssentialScalarOpcodes", Categories: "unitary", Exclusive: false, Fn: TestEncodeScalarValues_CoverEssentialScalarOpcodes},
	{Name: "TestEncodeScalarValues_SupportsEveryIntegerType", Categories: "unitary", Exclusive: false, Fn: TestEncodeScalarValues_SupportsEveryIntegerType},
	{Name: "TestEncodeUnsignedInteger_UsesExplicitOracleNumber", Categories: "unitary", Exclusive: false, Fn: TestEncodeUnsignedInteger_UsesExplicitOracleNumber},
	{Name: "TestEncodeStringNumber_RejectsUB1LengthOverflow", Categories: "unitary", Exclusive: false, Fn: TestEncodeStringNumber_RejectsUB1LengthOverflow},
	{Name: "TestEncodeContainers_UsesUB2FieldIDs", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_UsesUB2FieldIDs},
	{Name: "TestEncodeContainers_UsesUB4PrimaryDictionaryOffsets", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_UsesUB4PrimaryDictionaryOffsets},
	{Name: "TestEncodeContainers_UsesUB4SecondaryDictionaryOffsets", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_UsesUB4SecondaryDictionaryOffsets},
	{Name: "TestEncodeContainers_UsesAllContainerCountWidths", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_UsesAllContainerCountWidths},
	{Name: "TestEncodeTimestampScalar_UsesTimestampPayload", Categories: "unitary", Exclusive: false, Fn: TestEncodeTimestampScalar_UsesTimestampPayload},
	{Name: "TestEncodeBinaryFloatAndDoubleScalars_CoverSpecialValues", Categories: "unitary", Exclusive: false, Fn: TestEncodeBinaryFloatAndDoubleScalars_CoverSpecialValues},
	{Name: "TestEncodeBinaryScalars_CoverLengthBoundaries", Categories: "unitary", Exclusive: false, Fn: TestEncodeBinaryScalars_CoverLengthBoundaries},
	{Name: "TestEncodeInvalidValues_ReturnOsonEncodingError", Categories: "unitary", Exclusive: false, Fn: TestEncodeInvalidValues_ReturnOsonEncodingError},
	{Name: "TestEncodeContainers_UsesUB4OffsetsWhenTreeExceedsUB2", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_UsesUB4OffsetsWhenTreeExceedsUB2},
	{Name: "TestEncodeContainers_SupportsLongFieldNames", Categories: "unitary", Exclusive: false, Fn: TestEncodeContainers_SupportsLongFieldNames},
	{Name: "TestOsonWriteBufferPatchUint_WritesExpectedWidths", Categories: "unitary", Exclusive: false, Fn: TestOsonWriteBufferPatchUint_WritesExpectedWidths},
	{Name: "TestOsonWriteBufferPatchUint_RejectsInvalidPatch", Categories: "unitary", Exclusive: false, Fn: TestOsonWriteBufferPatchUint_RejectsInvalidPatch},
	{Name: "TestOsonEncoder_BufferPatchError", Categories: "unitary", Exclusive: false, Fn: TestOsonEncoder_BufferPatchError},

	{Name: "TestNewNodeAt_RejectsMissingContext", Categories: "unitary", Exclusive: false, Fn: TestNewNodeAt_RejectsMissingContext},
	{Name: "TestNewNodeAt_ResolvesRedirectChainsAndRejectsCycles", Categories: "unitary", Exclusive: false, Fn: TestNewNodeAt_ResolvesRedirectChainsAndRejectsCycles},
	{Name: "TestNewNodeAt_RejectsUpdateHeaderOffset", Categories: "unitary", Exclusive: false, Fn: TestNewNodeAt_RejectsUpdateHeaderOffset},
	{Name: "TestNodeOffsets_CoverAddressWidths", Categories: "unitary", Exclusive: false, Fn: TestNodeOffsets_CoverAddressWidths},
	{Name: "TestParse_RejectsOutOfRangeRelativeChildOffset", Categories: "unitary", Exclusive: false, Fn: TestParse_RejectsOutOfRangeRelativeChildOffset},
	{Name: "TestRedirectedNodeOffset_ValidatesEveryMarker", Categories: "unitary", Exclusive: false, Fn: TestRedirectedNodeOffset_ValidatesEveryMarker},
	{Name: "TestParse_RejectsInvalidUpdateTargets", Categories: "unitary", Exclusive: false, Fn: TestParse_RejectsInvalidUpdateTargets},
	{Name: "TestParse_RejectsForwardingCycle", Categories: "unitary", Exclusive: false, Fn: TestParse_RejectsForwardingCycle},
	{Name: "TestOsonDecoderFixtures", Categories: "unitary", Exclusive: false, Fn: TestOsonDecoderFixtures},
	{Name: "TestOsonDecoder_RejectsNonJSONBinaryFloatText", Categories: "unitary", Exclusive: false, Fn: TestOsonDecoder_RejectsNonJSONBinaryFloatText},

	{Name: "TestObjectNode_KindReportsObject", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_KindReportsObject},
	{Name: "TestObjectNode_SimpleObjectTraversal", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_SimpleObjectTraversal},
	{Name: "TestObjectNode_GetRejectsMalformedChild", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_GetRejectsMalformedChild},
	{Name: "TestObjectNode_SecondaryDictionaryTraversal", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_SecondaryDictionaryTraversal},
	{Name: "TestObjectNode_RejectsMalformedLayouts", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_RejectsMalformedLayouts},
	{Name: "TestObjectNode_SharedOverflowUsesPrimaryTreeOffsets", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_SharedOverflowUsesPrimaryTreeOffsets},
	{Name: "TestObjectNode_RejectsInvalidDelegateReferences", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_RejectsInvalidDelegateReferences},
	{Name: "TestReadFieldIDEntriesAt_ReadsAllSupportedWidths", Categories: "unitary", Exclusive: false, Fn: TestReadFieldIDEntriesAt_ReadsAllSupportedWidths},

	{Name: "TestOsonBuffer_NewBufferInitialState", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_NewBufferInitialState},
	{Name: "TestOsonBuffer_RejectsInvalidInternalCursor", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_RejectsInvalidInternalCursor},
	{Name: "TestOsonBuffer_SetPositionValidatesBounds", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_SetPositionValidatesBounds},
	{Name: "TestOsonBuffer_ReadsSequentialValues", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_ReadsSequentialValues},
	{Name: "TestOsonBuffer_AbsoluteReads", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_AbsoluteReads},
	{Name: "TestOsonBuffer_RejectsInvalidSequentialReads", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_RejectsInvalidSequentialReads},
	{Name: "TestOsonBuffer_RejectsInvalidAbsoluteRanges", Categories: "unitary", Exclusive: false, Fn: TestOsonBuffer_RejectsInvalidAbsoluteRanges},

	{Name: "TestOsonHeader_SampleOson", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_SampleOson},
	{Name: "TestIsOson", Categories: "unitary", Exclusive: false, Fn: TestIsOson},
	{Name: "TestOsonHeader_ScalarDocumentUsesPostHeaderTreeOffset", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_ScalarDocumentUsesPostHeaderTreeOffset},
	{Name: "TestOsonHeader_UpdatedTinyScalarFixture", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_UpdatedTinyScalarFixture},
	{Name: "TestOsonHeader_RejectsMalformedUpdateMetadata", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsMalformedUpdateMetadata},
	{Name: "TestOsonHeader_RejectsV1UpdateMetadata", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsV1UpdateMetadata},
	{Name: "TestOsonHeader_RejectsOutOfRangeUpdateMappings", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsOutOfRangeUpdateMappings},
	{Name: "TestOsonHeader_NestedObjectArrayFixture", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_NestedObjectArrayFixture},
	{Name: "TestOsonHeader_SecondaryDictionaryFixture", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_SecondaryDictionaryFixture},
	{Name: "TestOsonHeader_RejectsMissingInlineLeafFlag", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsMissingInlineLeafFlag},
	{Name: "TestOsonHeader_RejectsTruncatedTreeSegment", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsTruncatedTreeSegment},
	{Name: "TestOsonHeader_RejectsCorruptPrimaryDictionaryOffset", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsCorruptPrimaryDictionaryOffset},
	{Name: "TestOsonHeader_RejectsMalformedFixedHeader", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsMalformedFixedHeader},
	{Name: "TestOsonHeader_RejectsReservedFlags", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsReservedFlags},
	{Name: "TestOsonHeader_ScalarTreeSizeUB4", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_ScalarTreeSizeUB4},
	{Name: "TestOsonHeader_ReadHeaderWidthVariants", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_ReadHeaderWidthVariants},
	{Name: "TestOsonHeader_MetadataHelpersReflectFlagsAndBounds", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_MetadataHelpersReflectFlagsAndBounds},
	{Name: "TestOsonHeader_ForwardingHelpers", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_ForwardingHelpers},
	{Name: "TestOsonHeader_AddForwardingAddressValidatesMappings", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_AddForwardingAddressValidatesMappings},
	{Name: "TestOsonHeader_RejectsNilBuffer", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsNilBuffer},
	{Name: "TestOsonHeader_RejectsCorruptDictionaryHeaps", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsCorruptDictionaryHeaps},
	{Name: "TestOsonHeader_DictionaryReadersHandleBothOffsetWidths", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_DictionaryReadersHandleBothOffsetWidths},
	{Name: "TestOsonHeader_DictionaryReadersRejectTruncation", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_DictionaryReadersRejectTruncation},

	{Name: "TestScalarNode_ValueCoversSupportedDecodeUseCases", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_ValueCoversSupportedDecodeUseCases},
	{Name: "TestScalarNode_NumberAsStringOption", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_NumberAsStringOption},
	{Name: "TestScalarNode_DefaultOracleNumberAllowsLargePrecisionFloat", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_DefaultOracleNumberAllowsLargePrecisionFloat},
	{Name: "TestScalarNode_KindAndStringWithOption", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_KindAndStringWithOption},
	{Name: "TestScalarNode_MalformedScalarPayloads", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_MalformedScalarPayloads},
	{Name: "TestScalarNode_BinaryFloatSpecialValue", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_BinaryFloatSpecialValue},
	{Name: "TestOsonHeader_RejectsTruncatedInput", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsTruncatedInput},
	{Name: "TestOsonHeader_RejectsInvalidSecondaryDictionary", Categories: "unitary", Exclusive: false, Fn: TestOsonHeader_RejectsInvalidSecondaryDictionary},
	{Name: "TestNode_ReadHelpersRejectMalformedInput", Categories: "unitary", Exclusive: false, Fn: TestNode_ReadHelpersRejectMalformedInput},
	{Name: "TestScalarNode_RejectsTruncatedPayloads", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_RejectsTruncatedPayloads},
	{Name: "TestScalarNode_RejectsUnsupportedOpcode", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_RejectsUnsupportedOpcode},
	{Name: "TestNode_RedirectReadsRejectTruncatedPayloads", Categories: "unitary", Exclusive: false, Fn: TestNode_RedirectReadsRejectTruncatedPayloads},
	{Name: "TestObjectNode_ReadersRejectTruncatedLayouts", Categories: "unitary", Exclusive: false, Fn: TestObjectNode_ReadersRejectTruncatedLayouts},
	{Name: "TestScalarNode_RejectsUnsupportedOpcode", Categories: "unitary", Exclusive: false, Fn: TestScalarNode_RejectsUnsupportedOpcode},
}

func TestCategoryExecutor(t *testing.T) {
	oracleTest.RunCategoryExecutor(t, oracleTest.TestCategory, testCases)
}
