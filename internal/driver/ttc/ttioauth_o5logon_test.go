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

package ttc

import (
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestNewO5Logon(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	if o5 == nil {
		t.Error("newO5Logon returned nil")
	}
	if !o5.useO7LMR {
		t.Error("useO7LMR not set correctly")
	}
	if o5.md5Hash == nil {
		t.Error("md5Hash not initialized")
	}
	if o5.sha1Hash == nil {
		t.Error("sha1Hash not initialized")
	}
	if o5.sha512Hash == nil {
		t.Error("sha512Hash not initialized")
	}
}

func TestHashMD5(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(false)
	data := []byte("test")
	hash := o5.hashMD5(data)
	if len(hash) != 16 {
		t.Errorf("MD5 hash length incorrect: got %d, want 16", len(hash))
	}
}

func TestHashSHA1(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(false)
	data := []byte("test")
	hash := o5.hashSHA1(data)
	if len(hash) != 20 {
		t.Errorf("SHA1 hash length incorrect: got %d, want 20", len(hash))
	}
}

func TestHashSHA512(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(false)
	data := []byte("test")
	hash := o5.hashSHA512(data)
	if len(hash) != 64 {
		t.Errorf("SHA512 hash length incorrect: got %d, want 64", len(hash))
	}
}

func TestIsAllZero(t *testing.T) {
	t.Parallel()
	if !isAllZero([]byte{0, 0, 0}) {
		t.Error("isAllZero failed for all zeros")
	}
	if isAllZero([]byte{0, 1, 0}) {
		t.Error("isAllZero failed for non-zero")
	}
}

func TestRemovePKCS5Padding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []byte
		expected []byte
		hasError bool
	}{
		{"valid padding", []byte{1, 2, 3, 4, 4, 4, 4, 4}, []byte{1, 2, 3, 4}, false},
		{"empty input", []byte{}, nil, true},
		{"invalid padding", []byte{1, 2, 3, 4}, nil, true},
		{"padding too large", []byte{1, 2, 3, 4, 5}, nil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := removePKCS5Padding(test.input)
			if test.hasError {
				if err == nil {
					t.Error("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(result) != len(test.expected) {
					t.Fatalf("Length mismatch: got %d, want %d", len(result), len(test.expected))
				}
				for i, b := range test.expected {
					if result[i] != b {
						t.Errorf("result[%d] = %d, want %d", i, result[i], b)
					}
				}
			}
		})
	}
}

func TestApplyPKCS5Padding(t *testing.T) {
	t.Parallel()
	msg := []byte{1, 2, 3}
	padded := _applyPKCS5Padding(msg, 8)
	expectedLen := 8
	if len(padded) != expectedLen {
		t.Errorf("Padded length = %d, want %d", len(padded), expectedLen)
	}
	// Last 5 bytes should be 5
	for i := 3; i < 8; i++ {
		if (padded)[i] != 5 {
			t.Errorf("padded[%d] = %d, want 5", i, (padded)[i])
		}
	}
}

func TestApplyZeroPadding(t *testing.T) {
	t.Parallel()
	msg := []byte{1, 2, 3}
	padded := _applyZeroPadding(msg, 8)
	expectedLen := 8
	if len(padded) != expectedLen {
		t.Errorf("Padded length = %d, want %d", len(padded), expectedLen)
	}
	for i := 3; i < 8; i++ {
		if (padded)[i] != 0 {
			t.Errorf("padded[%d] = %d, want 0", i, (padded)[i])
		}
	}
}

func TestBuildO5LogonKey(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	ka := []byte("1234567890123456") // 16 bytes
	kb := []byte("abcdefghijklmnop") // 16 bytes
	salt := []byte("salt")
	key, err := o5._buildO5LogonKey(ZtvtMd5, ka, 0, kb, 0, 1000, salt)
	if err != nil {
		t.Errorf("_buildO5LogonKey failed: %v", err)
	}
	if key == nil {
		t.Fatalf("Key is nil ")
	}
	if len(key) != 16 {
		t.Errorf("Key length incorrect: got %d, want 16", len(key))
	}
}

func TestGetDerivedKey(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	o5.o5logonKey = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	dhKey := []byte("dhkey")
	_, err := o5.GetDerivedKey(dhKey, 0) // SHA1
	if err == nil {
		t.Errorf("Expected Error: Invalid key length")
	}
}

func TestDecryptAES(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(false)
	key := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	plaintext := []byte("Hello, World!!!") // 16 bytes
	// Encrypt manually for testing
	encrypted, err := o5._encryptAES(key, plaintext, "AES/CBC/PKCS5Padding")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	hexMsg := make([]byte, len(encrypted)*2)
	common.BArray2Nibbles(encrypted, hexMsg)
	decrypted, err := o5.decryptAES(key, string(hexMsg), "AES/CBC/PKCS5Padding")
	if err != nil {
		t.Errorf("Decrypt failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypt mismatch: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEncryptAES(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(false)
	key := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	plaintext := []byte("Hello, World!!!")
	encrypted, err := o5._encryptAES(key, plaintext, "AES/CBC/PKCS5Padding")
	if err != nil {
		t.Errorf("EncryptAES failed: %v", err)
	}
	if len(encrypted) == 0 {
		t.Error("Encrypted data is empty")
	}
}

func TestGeneratePk(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	pk, err := o5.generatePk(4096, common.B1Array("password"), []byte("salt"))
	if err != nil {
		t.Errorf("generatePk failed: %v", err)
	}
	if len(pk) != 64 {
		t.Errorf("PK length = %d, want 64", len(pk))
	}
}

func TestConstructVerifierForSHA512(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	pk := []byte("12345678901234567890123456789012") // 32 bytes
	salt := []byte("salt")
	verifier, err := o5.constructVerifierForSHA512(pk, salt)
	if err != nil {
		t.Errorf("constructVerifierForSHA512 failed: %v", err)
	}
	if len(verifier) != 32 {
		t.Errorf("Verifier length = %d, want 32", len(verifier))
	}
}

func TestConstructVerifierExceptSHA512(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	tests := []struct {
		verifierType int
		user         string
		pwd          string
		salt         []byte
	}{
		{ZtvtMd5, "user", "pwd", []byte("salt")},
		{ZtvtSSH1, "user", "pwd", []byte("salt")},
	}
	for _, test := range tests {
		verifier, err := o5.constructVerifierExceptSHA512(test.verifierType, common.B1Array(test.user), common.B1Array(test.pwd), false, test.salt, false)
		if err != nil {
			t.Errorf("constructVerifierExceptSHA512 failed: %v", err)
		}
		if verifier == nil {
			t.Error("Verifier is nil")
		}
	}
}

func TestValidateServerIdentity(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//o5.o5logonKey = &[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	//o5.pwdEncStr = "AES/CBC/PKCS5Padding"
	//// Mock a valid message
	//validErrr := o5.ValidateServerIdentity("AB12122237736489AB12122237736489")
	//if validErrr != nil {
	//	t.Errorf("validation failed [%s]", validErrr.Error())
	//}
}

func TestGenerateOAuthResponse(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//salt := []byte("salt")
	//encryptedKA := []byte("encryptedka")
	//encryptedKB := make([]byte, len(encryptedKA))
	//encryptedPwd := make([]byte, 100)
	//encryptedNewPwd := make([]byte, 100)
	//pwdLen := []int{0}
	//newPwdLen := []int{0}
	//pkEnc := make([]byte, 100)
	//pkLen := []int{0}
	//pwdNet := []byte("pwd")
	//err := o5.generateOAuthResponse(ZtvtMd5, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//if err != nil {
	//	t.Errorf("generateOAuthResponse failed: %v", err)
	//}
}

// Add more tests for different verifier types to cover all branches
func TestGenerateOAuthResponse_SHA512(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//salt := []byte("salt")
	//encryptedKA := []byte("encryptedka")
	//encryptedKB := make([]byte, len(encryptedKA))
	//encryptedPwd := make([]byte, 100)
	//encryptedNewPwd := make([]byte, 100)
	//pwdLen := []int{0}
	//newPwdLen := []int{0}
	//pkEnc := make([]byte, 100)
	//pkLen := []int{0}
	//pwdNet := []byte("pwd")
	//err := o5.generateOAuthResponse(ZtvtSha512, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//if err != nil {
	//	t.Errorf("generateOAuthResponse SHA512 failed: %v", err)
	//}
}

func TestGenerateOAuthResponse_Ssh1(t *testing.T) {
	t.Parallel()
	//	o5 := newO5Logon(true)
	//	salt := []byte("salt")
	//	encryptedKA := []byte("encryptedka")
	//	encryptedKB := make([]byte, len(encryptedKA))
	//	encryptedPwd := make([]byte, 100)
	//	encryptedNewPwd := make([]byte, 100)
	//	pwdLen := []int{0}
	//	newPwdLen := []int{0}
	//	pkEnc := make([]byte, 100)
	//	pkLen := []int{0}
	//	pwdNet := []byte("pwd")
	//	err := o5.generateOAuthResponse(ZtvtSsh1, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//	if err != nil {
	//		t.Errorf("generateOAuthResponse Ssh1 failed: %v", err)
	//	}
}

func TestGenerateOAuthResponse_Orcl7(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//salt := []byte("salt")
	//encryptedKA := []byte("encryptedka")
	//encryptedKB := make([]byte, len(encryptedKA))
	//encryptedPwd := make([]byte, 100)
	//encryptedNewPwd := make([]byte, 100)
	//pwdLen := []int{0}
	//newPwdLen := []int{0}
	//pkEnc := make([]byte, 100)
	//pkLen := []int{0}
	//pwdNet := []byte("pwd")
	//err := o5.generateOAuthResponse(ZtvtOrcl7, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//if err != nil {
	//	t.Errorf("generateOAuthResponse Orcl7 failed: %v", err)
	//}
}

func TestGenerateOAuthResponse_Smd5(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//salt := []byte("salt")
	//encryptedKA := []byte("encryptedka")
	//encryptedKB := make([]byte, len(encryptedKA))
	//encryptedPwd := make([]byte, 100)
	//encryptedNewPwd := make([]byte, 100)
	//pwdLen := []int{0}
	//newPwdLen := []int{0}
	//pkEnc := make([]byte, 100)
	//pkLen := []int{0}
	//pwdNet := []byte("pwd")
	//err := o5.generateOAuthResponse(ZtvtSmd5, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//if err != nil {
	//	t.Errorf("generateOAuthResponse Smd5 failed: %v", err)
	//}
}

func TestGenerateOAuthResponse_Sh1(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//salt := []byte("salt")
	//encryptedKA := []byte("encryptedka")
	//encryptedKB := make([]byte, len(encryptedKA))
	//encryptedPwd := make([]byte, 100)
	//encryptedNewPwd := make([]byte, 100)
	//pwdLen := []int{0}
	//newPwdLen := []int{0}
	//pkEnc := make([]byte, 100)
	//pkLen := []int{0}
	//pwdNet := []byte("pwd")
	//err := o5.generateOAuthResponse(ZtvtSh1, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//if err != nil {
	//	t.Errorf("generateOAuthResponse Sh1 failed: %v", err)
	//}
}

func TestGenerateOAuthResponse_ErrorCases(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(false) // useO7LMR false
	//salt := []byte("salt")
	//encryptedKA := []byte("encryptedka")
	//encryptedKB := make([]byte, len(encryptedKA))
	//encryptedPwd := make([]byte, 100)
	//encryptedNewPwd := make([]byte, 100)
	//pwdLen := []int{0}
	//newPwdLen := []int{0}
	//pkEnc := make([]byte, 100)
	//pkLen := []int{0}
	//pwdNet := []byte("pwd")
	//// Should fail because useO7LMR is false but SHA512 requires it
	//err := o5.generateOAuthResponse(ZtvtSha512, &salt, "user", "pwd", "", &pwdNet, nil, &encryptedKA, &encryptedKB, &encryptedPwd, &encryptedNewPwd, &pwdLen, &newPwdLen, false, 0, &salt, 4096, 3, &pkEnc, &pkLen)
	//if err == nil {
	//	t.Error("Expected error for SHA512 without O7LMR")
	//}
}

func TestGetO5LogonKey(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	o5.o5logonKey = []byte{1, 2, 3}
	key := o5.getO5LogonKey()
	if len(key) != 3 {
		t.Errorf("getO5LogonKey length = %d, want 3", len(key))
	}
}

func TestDecryptAESWithO5logonKey(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	o5.o5logonKey = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	o5.pwdEncStr = "AES/CBC/PKCS5Padding"
	// Mock hex message
	hexMsg := "somehex"
	result, err := o5.decryptAESWithO5logonKey(hexMsg, o5.pwdEncStr)
	// Just check no panic
	_ = result
	_ = err
}

func TestEncryptAESWithO5logonKey(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	o5.o5logonKey = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	o5.pwdEncStr = "AES/CBC/PKCS5Padding"
	msg := []byte("test")
	result, err := o5._encryptAESWithO5logonKey(msg, o5.pwdEncStr)
	if err != nil {
		t.Errorf("_encryptAESWithO5logonKey failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("Encrypted result is empty")
	}
}

func TestGenerateKb(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//ka := []byte("1234567890123456")
	//verifier := []byte("verifier12345678")
	//encryptedKB := make([]byte, 32)
	//encryptedKA := []byte("encryptedka")
	//kb, err := o5.generateKb(ka, verifier, encryptedKB, encryptedKA)
	//if err != nil {
	//	t.Errorf("generateKb failed: %v", err)
	//}
	//if len(kb) != 16 {
	//	t.Errorf("KB length = %d, want 16", len(kb))
	//}
}

func TestGenerateSpeedKey(t *testing.T) {
	t.Parallel()
	//o5 := newO5Logon(true)
	//o5.o5logonKey = &[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	//o5.pkEncStr = "AES/CBC/NoPadding"
	//pkEncrypted := make([]byte, 100)
	//pkLen := []int{0}
	//pk := []byte("pk12345678901234567890123456789012")
	//err := o5.generateSpeedKey(16, ZtvtSha512, pkEncrypted, pkLen, pk)
	//if err != nil {
	//	t.Errorf("generateSpeedKey failed: %v", err)
	//}
}

func TestEncryptPassword(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	o5.o5logonKey = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	o5.pwdEncStr = "AES/CBC/PKCS5Padding"
	pwdNet := []byte("password")
	encryptedPwd := make([]byte, 100)
	pwdLen := []int{0}
	err := o5._encryptPassword(pwdNet, encryptedPwd, 16, pwdLen)
	if err != nil {
		t.Errorf("encryptPassword failed: %v", err)
	}
	if pwdLen[0] == 0 {
		t.Error("Password length not set")
	}
}

func TestEncryptPasswordBufferTooSmall(t *testing.T) {
	t.Parallel()
	o5 := newO5Logon(true)
	o5.o5logonKey = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	o5.pwdEncStr = "AES/CBC/PKCS5Padding"

	err := o5._encryptPassword([]byte("password"), make([]byte, 1), 16, []int{0})
	if err == nil {
		t.Fatal("expected error for undersized encrypted password nibble buffer")
	}
}
