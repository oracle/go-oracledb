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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"

	"golang.org/x/crypto/pbkdf2"
)

var (
	oidPBES2  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}

	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 10}
	oidHMACWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}

	oidAES128CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidDESEDE3   = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7} // 3DES-CBC
)

// encryptedPrivateKeyInfo is the top-level structure of a
// "BEGIN ENCRYPTED PRIVATE KEY" PEM block.
//
// Data is declared as asn1.RawValue so we can use Data.Bytes which gives us
// only the payload — without the ASN.1 tag+length header bytes prepended.
// Using []byte here would include those extra header bytes, making the
// ciphertext length wrong and breaking block ciphers.
type encryptedPrivateKeyInfo struct {
	Algorithm algorithmIdentifier
	Data      asn1.RawValue
}

// algorithmIdentifier holds an OID plus its raw params bytes.
type algorithmIdentifier struct {
	OID    asn1.ObjectIdentifier
	Params asn1.RawValue
}

// pbes2Params holds the two sub-algorithms inside PBES2:
// the key-derivation function (PBKDF2) and the encryption scheme (AES-CBC or 3DES-CBC).
type pbes2Params struct {
	KDF        algorithmIdentifier
	Encryption algorithmIdentifier
}

// pbkdf2Params holds the inputs needed to re-derive the encryption key.
// PRF is optional in ASN.1; when omitted, PKCS#5 defaults it to HMAC-SHA1.
type pbkdf2Params struct {
	Salt       []byte
	Iterations int
	KeyLength  int                 `asn1:"optional"`
	PRF        algorithmIdentifier `asn1:"optional"`
}

const (
	maxPBKDF2SaltLength = 1024
	maxPBKDF2Iterations = 1000000
)

// ParsePKCS8EncryptedPrivateKey decrypts a PKCS#8 "ENCRYPTED PRIVATE KEY" PEM block
// using the given password and returns the private key.
//
// Supports PBES2 + PBKDF2 with AES-128-CBC, AES-192-CBC, AES-256-CBC, and 3DES-CBC.
//
// The returned key is one of *rsa.PrivateKey or *ecdsa.PrivateKey
// depending on what was stored in the wallet.
func ParsePKCS8EncryptedPrivateKey(block *pem.Block, password []byte) (interface{}, error) {
	if block == nil {
		return nil, errors.New("pem_decrypt: pem block is nil")
	}
	if block.Type != "ENCRYPTED PRIVATE KEY" {
		return nil, fmt.Errorf("pem_decrypt: unexpected PEM type %q, want ENCRYPTED PRIVATE KEY", block.Type)
	}

	// Step 1: Decode the outer EncryptedPrivateKeyInfo envelope directly from block.Bytes.
	var encInfo encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(block.Bytes, &encInfo); err != nil {
		return nil, fmt.Errorf("pem_decrypt: failed to parse EncryptedPrivateKeyInfo: %w", err)
	}

	if !encInfo.Algorithm.OID.Equal(oidPBES2) {
		return nil, fmt.Errorf("pem_decrypt: unsupported key encryption algorithm OID: %v", encInfo.Algorithm.OID)
	}

	// Step 2: Decode the PBES2 parameters (KDF + encryption scheme).
	var pbes2 pbes2Params
	if _, err := asn1.Unmarshal(encInfo.Algorithm.Params.FullBytes, &pbes2); err != nil {
		return nil, fmt.Errorf("pem_decrypt: failed to parse PBES2 params: %w", err)
	}
	if !pbes2.KDF.OID.Equal(oidPBKDF2) {
		return nil, fmt.Errorf("pem_decrypt: unsupported KDF OID: %v", pbes2.KDF.OID)
	}

	// Step 3: Decode the PBKDF2 parameters (salt + iteration count).
	var kdf pbkdf2Params
	if _, err := asn1.Unmarshal(pbes2.KDF.Params.FullBytes, &kdf); err != nil {
		return nil, fmt.Errorf("pem_decrypt: failed to parse PBKDF2 params: %w", err)
	}

	// Step 4: Decode the IV from the encryption scheme parameters.
	var iv []byte
	if _, err := asn1.Unmarshal(pbes2.Encryption.Params.FullBytes, &iv); err != nil {
		return nil, fmt.Errorf("pem_decrypt: failed to parse IV: %w", err)
	}

	// Step 5: Determine and validate the key-derivation work before doing it.
	keyLen, err := keyLengthForOID(pbes2.Encryption.OID)
	if err != nil {
		return nil, fmt.Errorf("pem_decrypt: %w", err)
	}
	if err := validatePBKDF2Params(kdf, keyLen); err != nil {
		return nil, fmt.Errorf("pem_decrypt: %w", err)
	}

	hashFunc, err := hashForPBKDF2PRF(kdf.PRF)
	if err != nil {
		return nil, fmt.Errorf("pem_decrypt: %w", err)
	}
	encKey := pbkdf2.Key(password, kdf.Salt, kdf.Iterations, keyLen, hashFunc)

	// Step 6: Decrypt the ciphertext using the correct cipher.
	// Use encInfo.Data.Bytes (not FullBytes) to get the raw ciphertext
	// without the ASN.1 tag+length header.
	plainDER, err := decrypt(pbes2.Encryption.OID, encKey, iv, encInfo.Data.Bytes)
	if err != nil {
		return nil, fmt.Errorf("pem_decrypt: decryption failed: %w", err)
	}

	// Step 7: Parse the now-plain DER bytes as a PKCS#8 private key.
	privateKey, err := x509.ParsePKCS8PrivateKey(plainDER)
	if err != nil {
		return nil, fmt.Errorf("pem_decrypt: failed to parse private key: %w", err)
	}

	return privateKey, nil
}

func validatePBKDF2Params(kdf pbkdf2Params, expectedKeyLen int) error {
	if len(kdf.Salt) == 0 {
		return errors.New("PBKDF2 salt is empty")
	}
	if len(kdf.Salt) > maxPBKDF2SaltLength {
		return fmt.Errorf("PBKDF2 salt length %d exceeds maximum %d", len(kdf.Salt), maxPBKDF2SaltLength)
	}
	if kdf.Iterations <= 0 || kdf.Iterations > maxPBKDF2Iterations {
		return fmt.Errorf("invalid PBKDF2 iteration count %d", kdf.Iterations)
	}
	if expectedKeyLen <= 0 {
		return fmt.Errorf("invalid PBKDF2 key length %d", expectedKeyLen)
	}
	if kdf.KeyLength < 0 {
		return fmt.Errorf("invalid PBKDF2 key length %d", kdf.KeyLength)
	}
	if kdf.KeyLength != 0 && kdf.KeyLength != expectedKeyLen {
		return fmt.Errorf("PBKDF2 key length %d does not match cipher key length %d", kdf.KeyLength, expectedKeyLen)
	}
	return nil
}

func hashForPBKDF2PRF(prf algorithmIdentifier) (func() hash.Hash, error) {
	if len(prf.OID) == 0 || prf.OID.Equal(oidHMACWithSHA1) {
		return sha1.New, nil
	}

	switch {
	case prf.OID.Equal(oidHMACWithSHA256):
		return sha256.New, nil
	case prf.OID.Equal(oidHMACWithSHA384):
		return sha512.New384, nil
	case prf.OID.Equal(oidHMACWithSHA512):
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("unsupported PBKDF2 PRF OID: %v", prf.OID)
	}
}

// keyLengthForOID returns the default key length in bytes for the given cipher OID.
// This is used as a fallback when kdf.KeyLength is not explicitly set in the PBKDF2 params.
func keyLengthForOID(oid asn1.ObjectIdentifier) (int, error) {
	switch {
	case oid.Equal(oidAES256CBC):
		return 32, nil // AES-256 needs a 32-byte key
	case oid.Equal(oidAES192CBC): // [ADDED #5]
		return 24, nil // AES-192 needs a 24-byte key
	case oid.Equal(oidAES128CBC):
		return 16, nil // AES-128 needs a 16-byte key
	case oid.Equal(oidDESEDE3):
		return 24, nil // 3DES needs a 24-byte key
	default:
		return 0, fmt.Errorf("unsupported encryption OID: %v", oid)
	}
}

// decrypt picks the right cipher based on the OID and decrypts the ciphertext.
func decrypt(oid asn1.ObjectIdentifier, key, iv, ciphertext []byte) ([]byte, error) {
	switch {
	case oid.Equal(oidAES256CBC), oid.Equal(oidAES192CBC), oid.Equal(oidAES128CBC):
		return decryptCBC(key, iv, ciphertext, aes.NewCipher, aes.BlockSize)
	case oid.Equal(oidDESEDE3):
		return decryptCBC(key, iv, ciphertext, des.NewTripleDESCipher, des.BlockSize)
	default:
		return nil, fmt.Errorf("unsupported encryption OID: %v", oid)
	}
}

// decryptCBC is a generic CBC decryptor that works for both AES and 3DES.
// newCipher is either aes.NewCipher or des.NewTripleDESCipher.
// blockSize is used to validate IV/ciphertext length and strip padding.
func decryptCBC(key, iv, ciphertext []byte, newCipher func([]byte) (cipher.Block, error), blockSize int) ([]byte, error) {
	block, err := newCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != blockSize {
		return nil, fmt.Errorf("IV length %d does not match block size %d", len(iv), blockSize)
	}
	if len(ciphertext) == 0 {
		return nil, errors.New("empty ciphertext")
	}

	if len(ciphertext)%blockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(ciphertext), blockSize)
	}

	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)

	return stripPKCS7Padding(plaintext, blockSize)
}

// stripPKCS7Padding removes the PKCS#7 padding appended during encryption.
// blockSize is passed in so it works for both AES (16 bytes) and 3DES (8 bytes).
func stripPKCS7Padding(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data after decryption")
	}
	if len(data)%blockSize != 0 {
		return nil, fmt.Errorf("decrypted data length %d is not a multiple of block size %d", len(data), blockSize)
	}

	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errors.New("invalid PKCS#7 padding — wrong password?")
	}

	padding := data[len(data)-padLen:]
	if !bytes.Equal(padding, bytes.Repeat([]byte{byte(padLen)}, padLen)) {
		return nil, errors.New("invalid PKCS#7 padding — wrong password?")
	}

	return data[:len(data)-padLen], nil
}
