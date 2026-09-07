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

package session

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
	{Name: "TestNewNetworkSession", Categories: "unitary", Exclusive: false, Fn: TestNewNetworkSession},
	{Name: "TestHandleAcceptRequiredANO", Categories: "unitary", Exclusive: false, Fn: TestHandleAcceptRequiredANO},
	{Name: "TestAcceptPacketClampsOversizedValues", Categories: "unitary", Exclusive: false, Fn: TestAcceptPacketClampsOversizedValues},
	{Name: "TestSessionAttsSetFromParsedDescriptionKeepsDefaultDNMatch", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromParsedDescriptionKeepsDefaultDNMatch},
	{Name: "TestTransportConnect", Categories: "unitary", Exclusive: false, Fn: TestTransportConnect},
	{Name: "TestConnectToOption", Categories: "unitary", Exclusive: false, Fn: TestConnectToOption},
	{Name: "TestConnectSubtests", Categories: "unitary", Exclusive: false, Fn: TestConnectSubtests},
	{Name: "TestSend", Categories: "unitary", Exclusive: false, Fn: TestSend},
	{Name: "TestReset", Categories: "unitary", Exclusive: false, Fn: TestReset},
	{Name: "TestDisconnect", Categories: "unitary", Exclusive: false, Fn: TestDisconnect},
	{Name: "TestPrintPacket", Categories: "unitary", Exclusive: false, Fn: TestPrintPacket},
	{Name: "TestSendPacketError", Categories: "unitary", Exclusive: true, Fn: TestSendPacketError},
	{Name: "TestSendConnect", Categories: "unitary", Exclusive: false, Fn: TestSendConnect},
	{Name: "TestProcessPacket", Categories: "unitary", Exclusive: false, Fn: TestProcessPacket},
	{Name: "TestCheckInbandNotification", Categories: "unitary", Exclusive: false, Fn: TestCheckInbandNotification},
	{Name: "TestRecvPacket", Categories: "unitary", Exclusive: true, Fn: TestRecvPacket},
	{Name: "TestRefuseArgs", Categories: "unitary", Exclusive: false, Fn: TestRefuseArgs},
	{Name: "TestHandleRefuse", Categories: "unitary", Exclusive: false, Fn: TestHandleRefuse},
	{Name: "TestHandleResend", Categories: "unitary", Exclusive: false, Fn: TestHandleResend},
	{Name: "TestPrepareReadBuffer", Categories: "unitary", Exclusive: false, Fn: TestPrepareReadBuffer},
	{Name: "TestPrepareReadBufferWithError", Categories: "unitary", Exclusive: false, Fn: TestPrepareReadBufferWithError},
	{Name: "TestReadUI8", Categories: "unitary", Exclusive: false, Fn: TestReadUI8},
	{Name: "TestReadUI8WithError", Categories: "unitary", Exclusive: false, Fn: TestReadUI8WithError},
	{Name: "TestReadUI16", Categories: "unitary", Exclusive: false, Fn: TestReadUI16},
	{Name: "TestReadUI16MultiPacket", Categories: "unitary", Exclusive: false, Fn: TestReadUI16MultiPacket},
	{Name: "TestReadNativeUI16", Categories: "unitary", Exclusive: false, Fn: TestReadNativeUI16},
	{Name: "TestReadUI32", Categories: "unitary", Exclusive: false, Fn: TestReadUI32},
	{Name: "TestReadText", Categories: "unitary", Exclusive: false, Fn: TestReadText},
	{Name: "TestReadBA", Categories: "unitary", Exclusive: false, Fn: TestReadBA},
	{Name: "TestWriteUI8", Categories: "unitary", Exclusive: false, Fn: TestWriteUI8},
	{Name: "TestWriteI32", Categories: "unitary", Exclusive: false, Fn: TestWriteI32},
	{Name: "TestWriteBA", Categories: "unitary", Exclusive: false, Fn: TestWriteBA},
	{Name: "TestSkipNBytes", Categories: "unitary", Exclusive: false, Fn: TestSkipNBytes},
	{Name: "TestIsInBreakReset", Categories: "unitary", Exclusive: false, Fn: TestIsInBreakReset},
	{Name: "TestReadMultiPacket", Categories: "unitary", Exclusive: false, Fn: TestReadMultiPacket},
	{Name: "TestFlush", Categories: "unitary", Exclusive: false, Fn: TestFlush},
	{Name: "TestSendReset", Categories: "unitary", Exclusive: false, Fn: TestSendReset},
	{Name: "TestPrepareReadBufferWithData", Categories: "unitary", Exclusive: false, Fn: TestPrepareReadBufferWithData},
	{Name: "TestReadUI32MultiPacket", Categories: "unitary", Exclusive: false, Fn: TestReadUI32MultiPacket},
	{Name: "TestWriteUI16WithFlush", Categories: "unitary", Exclusive: false, Fn: TestWriteUI16WithFlush},
	{Name: "TestSendWithBreak", Categories: "unitary", Exclusive: false, Fn: TestSendWithBreak},
	{Name: "TestCancelOperation", Categories: "unitary", Exclusive: false, Fn: TestCancelOperation},
	{Name: "TestHeaderMarshalUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestHeaderMarshalUnmarshal},
	{Name: "TestConnectPacketMarshal", Categories: "unitary", Exclusive: false, Fn: TestConnectPacketMarshal},
	{Name: "TestDataPacketMarshal", Categories: "unitary", Exclusive: false, Fn: TestDataPacketMarshal},
	{Name: "TestDataPacketFillBuf", Categories: "unitary", Exclusive: false, Fn: TestDataPacketFillBuf},
	{Name: "TestDataPacketPrepare2Send", Categories: "unitary", Exclusive: false, Fn: TestDataPacketPrepare2Send},
	{Name: "TestDataPacketReset", Categories: "unitary", Exclusive: false, Fn: TestDataPacketReset},
	{Name: "TestDataPacketUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestDataPacketUnmarshal},
	{Name: "TestDataPacketReadByte", Categories: "unitary", Exclusive: false, Fn: TestDataPacketReadByte},
	{Name: "TestDataPacketRead", Categories: "unitary", Exclusive: false, Fn: TestDataPacketRead},
	{Name: "TestDataPacketRemaining", Categories: "unitary", Exclusive: false, Fn: TestDataPacketRemaining},
	{Name: "TestAcceptPacketUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestAcceptPacketUnmarshal},
	{Name: "TestRefusePacketUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestRefusePacketUnmarshal},
	{Name: "TestRedirectPacketUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestRedirectPacketUnmarshal},
	{Name: "TestMarkerPacket", Categories: "unitary", Exclusive: false, Fn: TestMarkerPacket},
	{Name: "TestControlPacketUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestControlPacketUnmarshal},
	{Name: "TestResendPacketUnmarshal", Categories: "unitary", Exclusive: false, Fn: TestResendPacketUnmarshal},
	{Name: "TestContains", Categories: "unitary", Exclusive: false, Fn: TestContains},
	{Name: "TestGenUUID", Categories: "unitary", Exclusive: false, Fn: TestGenUUID},
	{Name: "TestNewSessionAttsDefaults", Categories: "unitary", Exclusive: false, Fn: TestNewSessionAttsDefaults},
	{Name: "TestSessionAttsSetFromDescription", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromDescription},
	{Name: "TestSessionAttsSetFromNil", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromNil},
	{Name: "TestSessionAttsSetFromCompressionDefaultLevel", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromCompressionDefaultLevel},
	{Name: "TestSessionAttsSetFromCompressionThreshold", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromCompressionThreshold},
	{Name: "TestSessionAttsSetFromEnableNotBroken", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromEnableNotBroken},
	{Name: "TestSessionAttsSetFromSSLAllowWeakDNMatchNo", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromSSLAllowWeakDNMatchNo},
	{Name: "TestSessionAttsSetFromUseSNINo", Categories: "unitary", Exclusive: false, Fn: TestSessionAttsSetFromUseSNINo},
	{Name: "TestReadWalletFile", Categories: "unitary", Exclusive: false, Fn: TestReadWalletFile},
	{Name: "TestReadWalletFileError", Categories: "unitary", Exclusive: false, Fn: TestReadWalletFileError},
	{Name: "TestPrepare", Categories: "unitary", Exclusive: false, Fn: TestPrepare},
	{Name: "TestPrepareWithWallet", Categories: "unitary", Exclusive: false, Fn: TestPrepareWithWallet},
	{Name: "TestPrepareWithSystemTrust", Categories: "unitary", Exclusive: false, Fn: TestPrepareWithSystemTrust},
	{Name: "TestPrepareDefaultTimeout", Categories: "unitary", Exclusive: false, Fn: TestPrepareDefaultTimeout},
	{Name: "TestPrepareWalletError", Categories: "unitary", Exclusive: false, Fn: TestPrepareWalletError},
	{Name: "TestPrepareUUIDError", Categories: "unitary", Exclusive: false, Fn: TestPrepareUUIDError},
	{Name: "TestPrepareNoPrefix", Categories: "unitary", Exclusive: false, Fn: TestPrepareNoPrefix},
	{Name: "TestPrepareNonTCPSNoWalletLoad", Categories: "unitary", Exclusive: false, Fn: TestPrepareNonTCPSNoWalletLoad},
	{Name: "TestWriteBytesWithContext", Categories: "unitary", Exclusive: false, Fn: TestWriteBytesWithContext},
	{Name: "TestReadByteWithContext", Categories: "unitary", Exclusive: false, Fn: TestReadByteWithContext},
	{Name: "TestReadBytesWithContext", Categories: "unitary", Exclusive: false, Fn: TestReadBytesWithContext},
	{Name: "TestPrepareClampsSDU", Categories: "unitary", Exclusive: false, Fn: TestPrepareClampsSDU},
	{Name: "TestReadBytesWithContextDoesNotTruncateLargeLength", Categories: "unitary", Exclusive: false, Fn: TestReadBytesWithContextDoesNotTruncateLargeLength},
}
