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
	{"TestNewNetworkSession", "unitary", false, TestNewNetworkSession},
	{"TestAcceptPacketClampsOversizedValues", "unitary", false, TestAcceptPacketClampsOversizedValues},
	{"TestSessionAttsSetFromParsedDescriptionKeepsDefaultDNMatch", "unitary", false, TestSessionAttsSetFromParsedDescriptionKeepsDefaultDNMatch},
	{"TestTransportConnect", "unitary", false, TestTransportConnect},
	{"TestConnectToOption", "unitary", false, TestConnectToOption},
	{"TestConnectSubtests", "unitary", false, TestConnectSubtests},
	{"TestSend", "unitary", false, TestSend},
	{"TestReset", "unitary", false, TestReset},
	{"TestDisconnect", "unitary", false, TestDisconnect},
	{"TestIsLittleEndian", "unitary", false, TestIsLittleEndian},
	{"TestPrintPacket", "unitary", false, TestPrintPacket},
	{"TestSendPacketError", "unitary", true, TestSendPacketError},
	{"TestSendConnect", "unitary", false, TestSendConnect},
	{"TestProcessPacket", "unitary", false, TestProcessPacket},
	{"TestCheckInbandNotification", "unitary", false, TestCheckInbandNotification},
	{"TestRecvPacket", "unitary", true, TestRecvPacket},
	{"TestRefuseArgs", "unitary", false, TestRefuseArgs},
	{"TestHandleRefuse", "unitary", false, TestHandleRefuse},
	{"TestHandleResend", "unitary", false, TestHandleResend},
	{"TestPrepareReadBuffer", "unitary", false, TestPrepareReadBuffer},
	{"TestPrepareReadBufferWithError", "unitary", false, TestPrepareReadBufferWithError},
	{"TestReadUI8", "unitary", false, TestReadUI8},
	{"TestReadUI8WithError", "unitary", false, TestReadUI8WithError},
	{"TestReadUI16", "unitary", false, TestReadUI16},
	{"TestReadUI16MultiPacket", "unitary", false, TestReadUI16MultiPacket},
	{"TestReadNativeUI16", "unitary", false, TestReadNativeUI16},
	{"TestReadUI32", "unitary", false, TestReadUI32},
	{"TestReadI64", "unitary", false, TestReadI64},
	{"TestReadText", "unitary", false, TestReadText},
	{"TestReadBA", "unitary", false, TestReadBA},
	{"TestReadBA2", "unitary", false, TestReadBA2},
	{"TestWriteUI8", "unitary", false, TestWriteUI8},
	{"TestWriteI32", "unitary", false, TestWriteI32},
	{"TestWriteI64", "unitary", false, TestWriteI64},
	{"TestWriteText", "unitary", false, TestWriteText},
	{"TestWriteBA", "unitary", false, TestWriteBA},
	{"TestSkipNBytes", "unitary", false, TestSkipNBytes},
	{"TestIsInBreakReset", "unitary", false, TestIsInBreakReset},
	{"TestReadMultiPacket", "unitary", false, TestReadMultiPacket},
	{"TestFlush", "unitary", false, TestFlush},
	{"TestWritePartial", "unitary", false, TestWritePartial},
	{"TestSendInterrupt", "unitary", false, TestSendInterrupt},
	{"TestSendReset", "unitary", false, TestSendReset},
	{"TestSendZDP", "unitary", false, TestSendZDP},
	{"TestSendZeroCopy", "unitary", false, TestSendZeroCopy},
	{"TestReceiveZeroCopy", "unitary", false, TestReceiveZeroCopy},
	{"TestPrepareReadBufferWithData", "unitary", false, TestPrepareReadBufferWithData},
	{"TestReadUI32MultiPacket", "unitary", false, TestReadUI32MultiPacket},
	{"TestWriteUI16WithFlush", "unitary", false, TestWriteUI16WithFlush},
	{"TestSendWithBreak", "unitary", false, TestSendWithBreak},
	{"TestCancelOperation", "unitary", false, TestCancelOperation},
	{"TestHeaderMarshalUnmarshal", "unitary", false, TestHeaderMarshalUnmarshal},
	{"TestConnectPacketMarshal", "unitary", false, TestConnectPacketMarshal},
	{"TestConnectPacketUnmarshal", "unitary", false, TestConnectPacketUnmarshal},
	{"TestDataPacketMarshal", "unitary", false, TestDataPacketMarshal},
	{"TestDataPacketFillBuf", "unitary", false, TestDataPacketFillBuf},
	{"TestDataPacketPrepare2Send", "unitary", false, TestDataPacketPrepare2Send},
	{"TestDataPacketReset", "unitary", false, TestDataPacketReset},
	{"TestDataPacketUnmarshal", "unitary", false, TestDataPacketUnmarshal},
	{"TestDataPacketReadByte", "unitary", false, TestDataPacketReadByte},
	{"TestDataPacketRead", "unitary", false, TestDataPacketRead},
	{"TestDataPacketRemaining", "unitary", false, TestDataPacketRemaining},
	{"TestAcceptPacketUnmarshal", "unitary", false, TestAcceptPacketUnmarshal},
	{"TestRefusePacketUnmarshal", "unitary", false, TestRefusePacketUnmarshal},
	{"TestRedirectPacketUnmarshal", "unitary", false, TestRedirectPacketUnmarshal},
	{"TestMarkerPacket", "unitary", false, TestMarkerPacket},
	{"TestControlPacketUnmarshal", "unitary", false, TestControlPacketUnmarshal},
	{"TestResendPacketUnmarshal", "unitary", false, TestResendPacketUnmarshal},
	{"TestContains", "unitary", false, TestContains},
	{"TestGenUUID", "unitary", false, TestGenUUID},
	{"TestNewSessionAttsDefaults", "unitary", false, TestNewSessionAttsDefaults},
	{"TestSessionAttsSetFromDescription", "unitary", false, TestSessionAttsSetFromDescription},
	{"TestSessionAttsSetFromNil", "unitary", false, TestSessionAttsSetFromNil},
	{"TestSessionAttsSetFromCompressionDefaultLevel", "unitary", false, TestSessionAttsSetFromCompressionDefaultLevel},
	{"TestSessionAttsSetFromCompressionThreshold", "unitary", false, TestSessionAttsSetFromCompressionThreshold},
	{"TestSessionAttsSetFromEnableNotBroken", "unitary", false, TestSessionAttsSetFromEnableNotBroken},
	{"TestSessionAttsSetFromSSLAllowWeakDNMatchNo", "unitary", false, TestSessionAttsSetFromSSLAllowWeakDNMatchNo},
	{"TestSessionAttsSetFromUseSNINo", "unitary", false, TestSessionAttsSetFromUseSNINo},
	{"TestReadWalletFile", "unitary", false, TestReadWalletFile},
	{"TestReadWalletFileError", "unitary", false, TestReadWalletFileError},
	{"TestPrepare", "unitary", false, TestPrepare},
	{"TestPrepareWithWallet", "unitary", false, TestPrepareWithWallet},
	{"TestPrepareWithSystemTrust", "unitary", false, TestPrepareWithSystemTrust},
	{"TestPrepareDefaultTimeout", "unitary", false, TestPrepareDefaultTimeout},
	{"TestPrepareWalletError", "unitary", false, TestPrepareWalletError},
	{"TestPrepareUUIDError", "unitary", false, TestPrepareUUIDError},
	{"TestPrepareNoPrefix", "unitary", false, TestPrepareNoPrefix},
	{"TestPrepareNonTCPSNoWalletLoad", "unitary", false, TestPrepareNonTCPSNoWalletLoad},
	{"TestWriteBytesWithContext", "unitary", false, TestWriteBytesWithContext},
	{"TestSetByteOrder", "unitary", false, TestSetByteOrder},
	{"TestGetByteOrder", "unitary", false, TestGetByteOrder},
	{"TestWriteB", "unitary", false, TestWriteB},
	{"TestReadByteWithContext", "unitary", false, TestReadByteWithContext},
	{"TestGetRemainingByteCount", "unitary", false, TestGetRemainingByteCount},
	{"TestReadBytesWithContext", "unitary", false, TestReadBytesWithContext},
	{"TestPrepareClampsSDU", "unitary", false, TestPrepareClampsSDU},
	{"TestReadBytesWithContextDoesNotTruncateLargeLength", "unitary", false, TestReadBytesWithContextDoesNotTruncateLargeLength},
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
