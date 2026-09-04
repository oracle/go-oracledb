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

package transport

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
	{Name: "TestParsePKCS8EncryptedPrivateKey_HappyPath", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_HappyPath},
	{Name: "TestParsePKCS8EncryptedPrivateKey_NilBlock", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_NilBlock},
	{Name: "TestParsePKCS8EncryptedPrivateKey_WrongBlockType", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_WrongBlockType},
	{Name: "TestParsePKCS8EncryptedPrivateKey_WrongKey", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_WrongKey},
	{Name: "TestParsePKCS8EncryptedPrivateKey_CorruptedBytes", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_CorruptedBytes},
	{Name: "TestStripPKCS7Padding_Valid", Categories: "unitary", Exclusive: false, Fn: TestStripPKCS7Padding_Valid},
	{Name: "TestStripPKCS7Padding_PadByteZero", Categories: "unitary", Exclusive: false, Fn: TestStripPKCS7Padding_PadByteZero},
	{Name: "TestStripPKCS7Padding_PadByteExceedsBlockSize", Categories: "unitary", Exclusive: false, Fn: TestStripPKCS7Padding_PadByteExceedsBlockSize},
	{Name: "TestStripPKCS7Padding_InconsistentPadBytes", Categories: "unitary", Exclusive: false, Fn: TestStripPKCS7Padding_InconsistentPadBytes},
	{Name: "TestStripPKCS7Padding_EmptyData", Categories: "unitary", Exclusive: false, Fn: TestStripPKCS7Padding_EmptyData},
	{Name: "TestHashForPBKDF2PRF", Categories: "unitary", Exclusive: false, Fn: TestHashForPBKDF2PRF},
	{Name: "TestKeyLengthForOID", Categories: "unitary", Exclusive: false, Fn: TestKeyLengthForOID},
	{Name: "TestDecryptCBC_AES_RoundTrip", Categories: "unitary", Exclusive: false, Fn: TestDecryptCBC_AES_RoundTrip},
	{Name: "TestDecryptCBC_IVLengthMismatch", Categories: "unitary", Exclusive: false, Fn: TestDecryptCBC_IVLengthMismatch},
	{Name: "TestDecryptCBC_EmptyCiphertext", Categories: "unitary", Exclusive: false, Fn: TestDecryptCBC_EmptyCiphertext},
	{Name: "TestDecryptCBC_CiphertextNotMultipleOfBlockSize", Categories: "unitary", Exclusive: false, Fn: TestDecryptCBC_CiphertextNotMultipleOfBlockSize},
	{Name: "TestDecryptCBC_InvalidKey", Categories: "unitary", Exclusive: false, Fn: TestDecryptCBC_InvalidKey},
	{Name: "TestDecryptCBC_3DES_RoundTrip", Categories: "unitary", Exclusive: false, Fn: TestDecryptCBC_3DES_RoundTrip},
	{Name: "TestNTTCPSProcessWalletReusesParsedWalletAfterRawContentCleared", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSProcessWalletReusesParsedWalletAfterRawContentCleared},
	{Name: "TestNTTCPSVerifyPostAcceptDNMatchUsesFinalTLSCertificate", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSVerifyPostAcceptDNMatchUsesFinalTLSCertificate},
	{Name: "TestNTTCPSVerifyPostAcceptDNMatchRejectsMismatchedFinalCertificate", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSVerifyPostAcceptDNMatchRejectsMismatchedFinalCertificate},
	{Name: "TestNTTCPSVerifyPostAcceptDNMatchUsesWeakServiceNameFallback", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSVerifyPostAcceptDNMatchUsesWeakServiceNameFallback},
	{Name: "TestNTTCPSVerifyPostAcceptDNMatchSkipsWhenStrictMatchAlreadyEnabled", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSVerifyPostAcceptDNMatchSkipsWhenStrictMatchAlreadyEnabled},
	{Name: "TestVerifyDNWithMultiValuedRDN", Categories: "unitary", Exclusive: false, Fn: TestVerifyDNWithMultiValuedRDN},
	{Name: "TestVerifyDNAllowsAliases", Categories: "unitary", Exclusive: false, Fn: TestVerifyDNAllowsAliases},
	{Name: "TestVerifyDNAllowsRepeatedAttributeAcrossRDNs", Categories: "unitary", Exclusive: false, Fn: TestVerifyDNAllowsRepeatedAttributeAcrossRDNs},
	{Name: "TestNTTCPSDisconnectPreservesProcessedWalletForRedirectReuse", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSDisconnectPreservesProcessedWalletForRedirectReuse},
	{Name: "TestNTTCPDisconnectClosesStreamWhenConnectedFlagFalse", Categories: "unitary", Exclusive: false, Fn: TestNTTCPDisconnectClosesStreamWhenConnectedFlagFalse},
	{Name: "TestDecrypt_UnsupportedOID", Categories: "unitary", Exclusive: false, Fn: TestDecrypt_UnsupportedOID},
	{Name: "TestNTTCPSProcessWalletRejectsWalletWithoutCertificatesEvenWithSystemTrust", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSProcessWalletRejectsWalletWithoutCertificatesEvenWithSystemTrust},
	{Name: "TestParsePKCS8EncryptedPrivateKey_AcceptsMatchingExplicitKeyLength", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_AcceptsMatchingExplicitKeyLength},
	{Name: "TestParsePKCS8EncryptedPrivateKey_RejectsUnsafePBKDF2Params", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_RejectsUnsafePBKDF2Params},
	{Name: "TestParsePKCS8EncryptedPrivateKey_UnsupportedEncryptionAlgorithmOID", Categories: "unitary", Exclusive: false, Fn: TestParsePKCS8EncryptedPrivateKey_UnsupportedEncryptionAlgorithmOID},
	{Name: "TestNTTCPSendReceiveAndDisconnect", Categories: "unitary", Exclusive: false, Fn: TestNTTCPSendReceiveAndDisconnect},
	{Name: "TestNTTCPReceiveBufferTooSmall", Categories: "unitary", Exclusive: false, Fn: TestNTTCPReceiveBufferTooSmall},
	{Name: "TestNTTCPDisconnectWhenNotConnected", Categories: "unitary", Exclusive: false, Fn: TestNTTCPDisconnectWhenNotConnected},
	{Name: "TestNewNTTCPSAndClear", Categories: "unitary", Exclusive: false, Fn: TestNewNTTCPSAndClear},
	{Name: "TestProcessWalletEmptyAndUnknownBlock", Categories: "unitary", Exclusive: false, Fn: TestProcessWalletEmptyAndUnknownBlock},
	{Name: "TestParseAndVerifyDN", Categories: "unitary", Exclusive: false, Fn: TestParseAndVerifyDN},
}
