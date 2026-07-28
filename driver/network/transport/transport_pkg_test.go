// Copyright (c) 2026, Oracle and/or its affiliates.
//
// This software is dual-licensed to you under the Universal Permissive License
// (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl and Apache License
// 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose
// either license.

package transport

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
	{"TestParsePKCS8EncryptedPrivateKey_HappyPath", "unitary", false, TestParsePKCS8EncryptedPrivateKey_HappyPath},
	{"TestParsePKCS8EncryptedPrivateKey_NilBlock", "unitary", false, TestParsePKCS8EncryptedPrivateKey_NilBlock},
	{"TestParsePKCS8EncryptedPrivateKey_WrongBlockType", "unitary", false, TestParsePKCS8EncryptedPrivateKey_WrongBlockType},
	{"TestParsePKCS8EncryptedPrivateKey_WrongKey", "unitary", false, TestParsePKCS8EncryptedPrivateKey_WrongKey},
	{"TestParsePKCS8EncryptedPrivateKey_CorruptedBytes", "unitary", false, TestParsePKCS8EncryptedPrivateKey_CorruptedBytes},
	{"TestStripPKCS7Padding_Valid", "unitary", false, TestStripPKCS7Padding_Valid},
	{"TestStripPKCS7Padding_PadByteZero", "unitary", false, TestStripPKCS7Padding_PadByteZero},
	{"TestStripPKCS7Padding_PadByteExceedsBlockSize", "unitary", false, TestStripPKCS7Padding_PadByteExceedsBlockSize},
	{"TestStripPKCS7Padding_InconsistentPadBytes", "unitary", false, TestStripPKCS7Padding_InconsistentPadBytes},
	{"TestStripPKCS7Padding_EmptyData", "unitary", false, TestStripPKCS7Padding_EmptyData},
	{"TestHashForPBKDF2PRF", "unitary", false, TestHashForPBKDF2PRF},
	{"TestKeyLengthForOID", "unitary", false, TestKeyLengthForOID},
	{"TestDecryptCBC_AES_RoundTrip", "unitary", false, TestDecryptCBC_AES_RoundTrip},
	{"TestDecryptCBC_IVLengthMismatch", "unitary", false, TestDecryptCBC_IVLengthMismatch},
	{"TestDecryptCBC_EmptyCiphertext", "unitary", false, TestDecryptCBC_EmptyCiphertext},
	{"TestDecryptCBC_CiphertextNotMultipleOfBlockSize", "unitary", false, TestDecryptCBC_CiphertextNotMultipleOfBlockSize},
	{"TestDecryptCBC_InvalidKey", "unitary", false, TestDecryptCBC_InvalidKey},
	{"TestDecryptCBC_3DES_RoundTrip", "unitary", false, TestDecryptCBC_3DES_RoundTrip},
	{"TestNTTCPSProcessWalletReusesParsedWalletAfterRawContentCleared", "unitary", false, TestNTTCPSProcessWalletReusesParsedWalletAfterRawContentCleared},
	{"TestNTTCPSVerifyPostAcceptDNMatchUsesFinalTLSCertificate", "unitary", false, TestNTTCPSVerifyPostAcceptDNMatchUsesFinalTLSCertificate},
	{"TestNTTCPSVerifyPostAcceptDNMatchRejectsMismatchedFinalCertificate", "unitary", false, TestNTTCPSVerifyPostAcceptDNMatchRejectsMismatchedFinalCertificate},
	{"TestNTTCPSVerifyPostAcceptDNMatchUsesWeakServiceNameFallback", "unitary", false, TestNTTCPSVerifyPostAcceptDNMatchUsesWeakServiceNameFallback},
	{"TestNTTCPSVerifyPostAcceptDNMatchSkipsWhenStrictMatchAlreadyEnabled", "unitary", false, TestNTTCPSVerifyPostAcceptDNMatchSkipsWhenStrictMatchAlreadyEnabled},
	{"TestVerifyDNWithMultiValuedRDN", "unitary", false, TestVerifyDNWithMultiValuedRDN},
	{"TestVerifyDNAllowsAliases", "unitary", false, TestVerifyDNAllowsAliases},
	{"TestVerifyDNAllowsRepeatedAttributeAcrossRDNs", "unitary", false, TestVerifyDNAllowsRepeatedAttributeAcrossRDNs},
	{"TestNTTCPSDisconnectPreservesProcessedWalletForRedirectReuse", "unitary", false, TestNTTCPSDisconnectPreservesProcessedWalletForRedirectReuse},
	{"TestNTTCPDisconnectClosesStreamWhenConnectedFlagFalse", "unitary", false, TestNTTCPDisconnectClosesStreamWhenConnectedFlagFalse},
	{"TestDecrypt_UnsupportedOID", "unitary", false, TestDecrypt_UnsupportedOID},
	{"TestNTTCPSProcessWalletRejectsWalletWithoutCertificatesEvenWithSystemTrust", "unitary", false, TestNTTCPSProcessWalletRejectsWalletWithoutCertificatesEvenWithSystemTrust},
	{"TestParsePKCS8EncryptedPrivateKey_AcceptsMatchingExplicitKeyLength", "unitary", false, TestParsePKCS8EncryptedPrivateKey_AcceptsMatchingExplicitKeyLength},
	{"TestParsePKCS8EncryptedPrivateKey_RejectsUnsafePBKDF2Params", "unitary", false, TestParsePKCS8EncryptedPrivateKey_RejectsUnsafePBKDF2Params},
	{"TestParsePKCS8EncryptedPrivateKey_UnsupportedEncryptionAlgorithmOID", "unitary", false, TestParsePKCS8EncryptedPrivateKey_UnsupportedEncryptionAlgorithmOID},
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
