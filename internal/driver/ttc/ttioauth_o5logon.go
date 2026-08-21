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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"hash"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// ztvLenOrcl represents the length of Oracle verifier type
	ztvLenOrcl = 16
	// fixedResponseLength represents the response length
	fixedResponseLength = 16
	// authFlagSha2 represents the auth flag for SHA2
	authFlagSha2 = 1
	// authOraDebugJdwpEncSession represents the debug flag
	authOraDebugJdwpEncSession byte = 1

	validationString = "SERVER_TO_CLIENT"
)

const (
	// ZtvtOrcl7 represents Oracle(7) verifier type
	ZtvtOrcl7 = 0x0939
	// ZtvtSSH1 represents SHA1 hash verifier type
	ZtvtSSH1 = 0x1b25
	// ZtvtSmd5 represents MD5 hash
	ZtvtSmd5 = 0xe92e
	// ZtvtMd5 represents MD5 hash
	ZtvtMd5 = 0x9ee2
	// ZtvtSh1 represents SHA1 Hash
	ZtvtSh1 = 0xb152
	// ZtvtSha512 SHA512 hash
	ZtvtSha512 = 0x4815
)

// o5Logon handles Oracle database authentication and encryption for logon processes.
// It manages cryptographic operations including key generation, password encryption, and session key derivation.
type o5Logon struct {
	// connection driver.Connection
	// use if driver compiletime capabilities expose this. Its exposed if the
	// server supports it and PBKDF2WithHmacSHA512 is available in the env
	useO7LMR bool // Indicates if O7LMR (Oracle 7LMR) logon is supported

	md5Hash    hash.Hash // MD5 hash instance
	sha1Hash   hash.Hash // SHA1 hash instance
	sha512Hash hash.Hash // SHA512 hash instance

	// O5LOGON key (includes Ka and Kb)
	o5logonKey []byte // The derived O5LOGON session key
	sessKeyStr string // Session key encryption string
	pwdEncStr  string // Password encryption string
	pkEncStr   string // PK encryption string
}

// newO5Logon creates a new o5Logon instance with the specified O7LMR capability.
func newO5Logon(logonCapabilityO7LMR bool) *o5Logon {
	return &o5Logon{
		// connection: conn,
		useO7LMR:   logonCapabilityO7LMR,
		md5Hash:    md5.New(),
		sha1Hash:   sha1.New(),
		sha512Hash: sha512.New(),
	}
}

// hashMD5 computes the MD5 hash of the provided data.
func (o *o5Logon) hashMD5(data []byte) []byte {
	o.md5Hash.Reset()
	o.md5Hash.Write(data)
	return o.md5Hash.Sum(nil)
}

// hashSHA1 computes the SHA1 hash of the provided data.
func (o *o5Logon) hashSHA1(data []byte) []byte {
	o.sha1Hash.Reset()
	o.sha1Hash.Write(data)
	return o.sha1Hash.Sum(nil)
}

// hashSHA512 computes the SHA512 hash of the provided data.
func (o *o5Logon) hashSHA512(data []byte) []byte {
	o.sha512Hash.Reset()
	o.sha512Hash.Write(data)
	return o.sha512Hash.Sum(nil)
}

// Returns true if the PBKDF2 algorithm used for OL7MR logon is available.
func (o *o5Logon) isOL7MRCapable() bool {
	return true // HMAC-SHA512 is always supported in Go
}

// getO5LogonKey returns o5Logon decrypted session key.
func (o *o5Logon) getO5LogonKey() []byte {
	return o.o5logonKey
}

// Construct O5logon key. This is dependent on the verifier.
// O5logon key size is going to be:
//   - 16 bytes for ZtvtOrcl7 and ZtvtMd5 and AES128 can be used
//   - 24 bytes for ZtvtSsh1 and ZtvtSh1 and AES192 can be used
//   - 32 bytes for ZtvtSha512
func (o *o5Logon) _buildO5LogonKey(verifier int, ka []byte, start1 int,
	kb []byte, start2 int, sha512PBKDF2SderCount int, sha512SaltPBKDF2Nibbles []byte) ([]byte, error) {
	var ret []byte
	keyLen := 0

	if o.useO7LMR {
		// The Ka and Kb exchanged are 32bytes long but a substring of them is used
		// for each verifier type based on its keysize requirement.
		// The o5Logon key generation algorithm is PBKDEF2 with SHA-1
		switch verifier {
		case ZtvtSmd5, ZtvtMd5, ZtvtOrcl7:
			keyLen = 16 // AES128_KEY_SIZE_BYTES
			break
		case ZtvtSh1, ZtvtSSH1:
			keyLen = 24 // AES192_KEY_SIZE_BYTES
			break
		case ZtvtSha512:
			keyLen = 32 // AES256_KEY_SIZE_BYTES
		}

		iterations := sha512PBKDF2SderCount

		kbka := make([]byte, 2*keyLen)
		copy(kbka[0:keyLen], kb[:keyLen])
		copy(kbka[keyLen:], ka[:keyLen])
		kakbNibbles := make([]byte, len(kbka)*2)
		driverCommon.BArray2Nibbles(kbka, kakbNibbles)
		saltBinary := driverCommon.ToBinArray(string(sha512SaltPBKDF2Nibbles))

		key := pbkdf2.Key([]byte(string(kakbNibbles)), saltBinary, iterations, keyLen, sha512.New)
		if isAllZero(key) {
			return nil, errors.New("generated key is all zeros — possible PBKDF2 failure")
		}

		ret = key
	}
	return ret, nil
}

// GetDerivedKey derives a session key from O5logon key for Secure Id Propagation.
// The key derivation will use PBKDF2WithHmacSHA1/SHA512.
func (o *o5Logon) GetDerivedKey(dhKey []byte, mode int) ([]byte, error) {
	var secretKey []byte
	var secretKeyHex string
	algorithm := "PBKDF2WithHmacSHA512"
	keyLength := 512

	// secret key is o5Logon key
	secretKey = o.o5logonKey

	if (mode & authFlagSha2) != authFlagSha2 {
		algorithm = "PBKDF2WithHmacSHA1"
		keyLength = 160
	}

	secretKeyHex = strings.ToUpper(hex.EncodeToString(secretKey))
	passwordBytes := []byte(secretKeyHex)
	var derivedKey []byte
	iterations := 1000
	keyLenBytes := keyLength / 8

	switch algorithm {
	case "PBKDF2WithHmacSHA1":
		derivedKey = pbkdf2.Key(passwordBytes, dhKey, iterations, keyLenBytes, sha1.New)
	case "PBKDF2WithHmacSHA512":
		derivedKey = pbkdf2.Key(passwordBytes, dhKey, iterations, keyLenBytes, sha512.New)
	default:
		return nil, errors.New("unsupported algorithm")
	}

	ret := make([]byte, len(derivedKey))
	copy(ret, derivedKey)

	// Optionally verify that the derived key is valid for AES (length 16, 24, or 32 bytes)
	if keyLenBytes != 16 && keyLenBytes != 24 && keyLenBytes != 32 {
		return nil, errors.New("invalid AES key length")
	}

	_, err := aes.NewCipher(ret)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// ValidateServerIdentity validates the server's identity by decrypting and checking a message.
// return error for validation failure with details about it
func (o *o5Logon) ValidateServerIdentity(msgHex string) error {

	decryptedBytes, err := o.decryptAESWithO5logonKey(msgHex, o.pwdEncStr)
	if err != nil {
		common.Odl.Warn("Server identity validation failed", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	compareWithString := string(decryptedBytes[16:])
	if compareWithString != validationString {
		common.Odl.Warn("Server identity validation failed")
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, nil)
	}

	return nil
}

// decryptAESWithO5logonKey decrypts a hex-encoded message using the o5Logon key.
func (o *o5Logon) decryptAESWithO5logonKey(msgHex string, decStr string) ([]byte, error) {
	return o.decryptAES(o.o5logonKey, msgHex, decStr)
}

// decryptAES decrypts a hex-encoded message using the provided AES key and decryption string.
func (o *o5Logon) decryptAES(key []byte, msgHex string, decStr string) ([]byte, error) {
	if key == nil {
		return []byte{}, nil
	}

	ciphertext, err := hex.DecodeString(msgHex)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}

	// Use zero IV
	iv := make([]byte, aes.BlockSize)

	// CBC mode decryption
	mode := cipher.NewCBCDecrypter(block, iv)
	decryptedMsg := make([]byte, len(ciphertext))
	mode.CryptBlocks(decryptedMsg, ciphertext)

	// Handle padding
	decStr = strings.ToUpper(decStr)
	switch {
	case strings.HasSuffix(decStr, "PKCS5PADDING"):
		decryptedMsg, err = removePKCS5Padding(decryptedMsg)
		if err != nil {
			return nil, err
		}
	}

	return decryptedMsg, nil
}

// removePKCS5Padding removes PKCS5 padding from the decrypted data.
func removePKCS5Padding(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("invalid PKCS5 padding (empty input)")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > len(data) {
		return nil, errors.New("invalid PKCS5 padding")
	}
	// Check padding validity (optional strict mode)
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, errors.New("invalid PKCS5 padding content")
		}
	}
	return data[:len(data)-padding], nil
}

// _encryptAESWithO5logonKey encrypts a message using the o5Logon key.
func (o *o5Logon) _encryptAESWithO5logonKey(msg []byte, encStr string) ([]byte, error) {
	return o._encryptAES(o.o5logonKey, msg, encStr)
}

// _encryptAES encrypts a message using the provided AES key and encryption string.
func (o *o5Logon) _encryptAES(key []byte, msg []byte, encStr string) ([]byte, error) {
	if key == nil {
		return []byte{}, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, aes.BlockSize)

	encStr = strings.ToUpper(encStr)
	switch {
	case strings.HasSuffix(encStr, "PKCS5PADDING"):
		msg = _applyPKCS5Padding(msg, aes.BlockSize)
	case strings.HasSuffix(encStr, "Zeros"):
		msg = _applyZeroPadding(msg, aes.BlockSize)
	default:
		if len(msg)%aes.BlockSize != 0 {
			return nil, errors.New("plaintext is not a multiple of block size and no padding specified")
		}
	}

	mode := cipher.NewCBCEncrypter(block, iv)

	encryptedMsg := make([]byte, len(msg))
	mode.CryptBlocks(encryptedMsg, msg)
	return encryptedMsg, nil
}

// _applyPKCS5Padding applies PKCS5 padding to the message.
func _applyPKCS5Padding(msg []byte, blockSize int) []byte {
	padding := blockSize - len(msg)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	res := append(msg, padText...)
	return res
}

// _applyZeroPadding applies zero padding to the message.
func _applyZeroPadding(msg []byte, blockSize int) []byte {
	padding := blockSize - len(msg)%blockSize
	padText := bytes.Repeat([]byte{0}, padding)
	res := append(msg, padText...)
	return res
}

// generateOAuthResponse generates OAUTH response message to send back to the server.
//
// The reason why we pass both UTF8 and network character set encoded password
// :
//
// During the first part of the o5logon protocol (handshake), a hash verifier
// of the password is calculated. This hash verifier is always calculated
// based on UTF8 encoding. Since the verifier is calculated and stored when a
// user is created, it is independent of network character set.
//
// The UTF8 bytes of the password itself is NOT transmitted on the network. It
// is calculated on the client to match the same verifier stored on the
// server.
//
// After the server decrypts the password, it will decode the bytes into
// characters using the network character set. The server always expects the
// encoding to be based on the character set negotiated on the network, no
// matter it comes from sqlplus client or jdbc client.
//
// The network label code here does not have the context to generate the
// network character set encoded bytes, therefore it needs to be passed from
// the DBJAVA layer.
//
// In case of changing the user password the verifier is calculated with the
// new password and both passwords are encrypted using that verfier.
func (o *o5Logon) generateOAuthResponse(
	verifierType int,
	salt []byte,
	noQuotesUser driverCommon.B1Array,
	noQuotesPwd driverCommon.B1Array, // password string
	noQuotesPwdNetBytes driverCommon.B1Array, // password represented in network char set
	encryptedKANibbles []byte, // passed from server
	encryptedKBNibbles []byte, // allocated (but empty) buffer provided by caller
	encryptedPasswordNibbles []byte, // allocated buffer provided by caller
	encryptedPasswordNibblesLength []int, // to return the size
	svrCSMultibyte bool,
	_hasO5LNPSupport bool,
	saltPBKDF2Nibbles []byte, // salt for encryption sent by server
	PBKDF2VgenCount int, // iteration count for PBKDF2 encryption of password for verifier (min 4096)
	PBKDF2SderCount int, // iteration count for PBKDF2 encryption of KbKa for o5Logon key (min 3)
	pkEncryptedNibbles []byte, // SPEEDY KEY
	pkEncryptedNibblesLength []int) error {
	var verifier []byte = nil
	var pk []byte = nil // PBKDF2 encryption of password
	confounderLen := 16
	var err error

	if o.sha1Hash == nil || (o.md5Hash == nil && !o.useO7LMR) ||
		(!(o.isOL7MRCapable()) && o.useO7LMR) || o.sha512Hash == nil {
		return errors.New("resource A missing")
	}

	// we use encryptedPasswordNibblesLength provided by caller to return
	// the number of bytes to be sent to the server.
	if len(encryptedPasswordNibblesLength) != 1 {
		return errors.New("resource B missing")
	}

	//	Step #1: construct verifier:
	if verifierType == ZtvtSha512 {
		pk, err = o.generatePk(PBKDF2VgenCount, noQuotesPwd, salt)
		if err != nil {
			return err
		}

		verifier, err = o.constructVerifierForSHA512(pk, salt)
		if err != nil {
			return err
		}
	} else {
		verifier, err = o.constructVerifierExceptSHA512(verifierType, noQuotesUser,
			noQuotesPwd, svrCSMultibyte, salt, _hasO5LNPSupport)
		if err != nil {
			return err
		}
	}

	// Step #2: decrypt Ka using verifier (Server Sesssion key decription)
	ka, err := o.decryptAES(verifier, string(encryptedKANibbles), o.sessKeyStr)
	if err != nil {
		return err
	}

	// Step #3: generate Kb and encrypt it with verifier
	kb, err := o.generateKb(ka, verifier, encryptedKBNibbles, encryptedKANibbles)
	if err != nil {
		return err
	}

	// Step #4: constructs O5logon key (concatenate Ka and Kb (Ka XOR Kb, and MD5 digest))
	o.o5logonKey, err = o._buildO5LogonKey(verifierType, ka, fixedResponseLength,
		kb, fixedResponseLength, PBKDF2SderCount, saltPBKDF2Nibbles)
	if err != nil {
		return err
	}

	// Step 4.1 - Generate SPEEDY KEY
	// The SPEEDY KEY is a AES encryption of the ((PBKDF2 key of password) and a confounder).
	// The SPEEDY KEY is sent for O7L_MR login as a time saving measure. The server can get
	// the client verifier by unencrypting the SPEEDY KEY and doing a SHA512. The server does not
	// have to calculate the PBKDF2 key of the password again.

	if err = o.generateSpeedKey(confounderLen, verifierType, pkEncryptedNibbles, pkEncryptedNibblesLength, pk); err != nil {
		return err
	}

	// Step #5: encrypt password with o5logon key

	return o._encryptPassword(noQuotesPwdNetBytes, encryptedPasswordNibbles, confounderLen, encryptedPasswordNibblesLength)
}

func (o *o5Logon) constructVerifierForSHA512(pk []byte, salt []byte) ([]byte, error) {
	if pk == nil {
		return nil, errors.New("pk (derived key) is nil")
	}

	h := sha512.New()
	h.Write(pk)
	if salt != nil {
		h.Write(driverCommon.ToBinArray(string(salt)))
	}

	return h.Sum(nil)[:32], nil
}

func (o *o5Logon) constructVerifierExceptSHA512(verifierType int,
	noQuotesUser driverCommon.B1Array,
	noQuotesPwd driverCommon.B1Array,
	svrCSMultibyte bool,
	salt []byte,
	_hasO5LNPSupport bool) ([]byte, error) {
	var verifier []byte
	/* 10g database verifier */
	if verifierType == ZtvtSSH1 || verifierType == ZtvtSh1 {
		// OVD with SUN-LDAP
		// ZtvtSsh1: salted sha1 is the new O5LOGON password verifier
		// ZtvtSh1: non-salted sha1 hash of the password is used in
		// conjunction with OID
		if _hasO5LNPSupport {
			o.sessKeyStr = "AES/CBC/NoPadding"
		} else {
			o.sessKeyStr = "AES/CBC/PKCS5Padding"
		}
		o.pwdEncStr = "AES/CBC/PKCS5Padding"
		// calculate SHA1 verifier
		o.sha1Hash.Reset()
		_, err := o.sha1Hash.Write([]byte(noQuotesPwd))
		if err != nil {
			return nil, err
		}

		if verifierType == ZtvtSSH1 && salt != nil {
			_, err = o.sha1Hash.Write(driverCommon.ToBinArray(string(salt)))
			if err != nil {
				return nil, err
			}
		}

		sha1vfr := o.sha1Hash.Sum(nil)
		verifier = make([]byte, 24) // AES192 key size
		copy(verifier, sha1vfr)
	} else if verifierType == ZtvtMd5 || verifierType == ZtvtSmd5 {
		o.sessKeyStr = "AES/CBC/NoPadding"
		o.pwdEncStr = "AES/CBC/PKCS5Padding"
		o.md5Hash.Reset()
		_, err := o.md5Hash.Write([]byte(noQuotesPwd))
		if err != nil {
			return nil, err
		}
		// if verifier is ZtvtSsh1 we use the salt
		if verifierType == ZtvtSmd5 {
			_, err = o.md5Hash.Write(driverCommon.ToBinArray(string(salt)))
			if err != nil {
				return nil, err
			}
		}
		verifier = o.md5Hash.Sum(nil)
	} else {
		return nil, errors.New("resource C missing")
	}
	return verifier, nil
}

func (o *o5Logon) generatePk(PBKDF2VgenCount int, noQuotesPwd driverCommon.B1Array, salt []byte) ([]byte, error) {

	// The verifier for this type is generated in the following steps
	// 1. PBEKDF2 encryption of password with the salt.
	// The salt here is a concatenation of the salt sent by server and a
	// fixed string "AUTH_PBKDF2_SPEEDY_KEY".
	// 2. SHA512 hash of the above with the salt send by server.

	o.sessKeyStr = "AES/CBC/NoPadding"
	o.pwdEncStr = "AES/CBC/PKCS5Padding"
	o.pkEncStr = "AES/CBC/NoPadding"

	iterations := PBKDF2VgenCount
	pmkLength := 512 / 8 // length in bytes

	saltForPBKDF2Part1 := driverCommon.ToBinArray(string(salt))
	authPbkdf2SpeedyKey := "AUTH_PBKDF2_SPEEDY_KEY"
	saltForPBKDF2Part2 := []byte(authPbkdf2SpeedyKey)
	saltForPBKDF2 := append(saltForPBKDF2Part1, saltForPBKDF2Part2...)

	// Generate PBKDF2 key
	key := pbkdf2.Key([]byte(noQuotesPwd), saltForPBKDF2, iterations, pmkLength, sha512.New)

	if isAllZero(key) {
		return nil, errors.New("generated key is all zeros — invalid key")
	}

	return key, nil
}

// isAllZero checks if all bytes in the data are zero.
func isAllZero(data []byte) bool {
	return bytes.Equal(data, make([]byte, len(data)))
}

func (o *o5Logon) generateKb(ka []byte, verifier []byte,
	encryptedKBNibbles []byte, encryptedKANibbles []byte) ([]byte, error) {

	kb := make([]byte, len(ka))
	if err := driverCommon.GenerateRandomBytes(kb); err != nil {
		common.Odl.Warn("Failed to generate random kb", "error", err)
		return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	encryptedKBBytes, err := o._encryptAES(verifier, kb, o.sessKeyStr)
	if err != nil {
		common.Odl.Warn("AES encryption failed", "error", err)
		return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	if encryptedKBNibbles == nil || len(encryptedKBNibbles) != len(encryptedKANibbles) {
		common.Odl.Warn("Resource D missing", "error", err)
		return nil, common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	if err = validateNibbleBufferLength(encryptedKBBytes, encryptedKBNibbles); err != nil {
		return nil, err
	}
	driverCommon.BArray2Nibbles(encryptedKBBytes, encryptedKBNibbles)
	return kb, nil
}

func (o *o5Logon) generateSpeedKey(confounderLen int, verifierType int,
	pkEncryptedNibbles []byte, pkEncryptedNibblesLength []int,
	pk []byte) error {
	confounder := make([]byte, confounderLen)

	if verifierType == ZtvtSha512 {
		if err := driverCommon.GenerateRandomBytes(confounder); err != nil {
			common.Odl.Warn("Failed to generate confounder", "error", err)
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}

		cfpk := append(confounder, pk...)

		// Encrypt using AES with O5logon key
		pkEncrypted, err := o._encryptAESWithO5logonKey(cfpk, o.pkEncStr)
		if err != nil {
			common.Odl.Warn("AES encryption failed", "error", err)
			return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
		}

		if err = validateNibbleBufferLength(pkEncrypted, pkEncryptedNibbles); err != nil {
			return err
		}
		pkEncryptedNibblesLength[0] = driverCommon.BArray2Nibbles(pkEncrypted, pkEncryptedNibbles)
	}

	return nil
}

func (o *o5Logon) _encryptPassword(noQuotesPwdNetBytes []byte,
	encryptedPasswordNibbles []byte,
	confounderLen int,
	encryptedPasswordNibblesLength []int) error {
	if encryptedPasswordNibbles == nil {
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, "resource E missing")
	}

	confounder := make([]byte, confounderLen)
	if err := driverCommon.GenerateRandomBytes(confounder); err != nil {
		common.Odl.Warn("Failed to generate confounder", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}

	cfpwd := append(confounder, noQuotesPwdNetBytes...)
	jenc, err := o._encryptAESWithO5logonKey(cfpwd, o.pwdEncStr)
	if err != nil {
		common.Odl.Warn("AES encryption failed", "error", err)
		return common.NewOracleError(oracleErrors.AuthenticatorError, err, nil)
	}
	if err = validateNibbleBufferLength(jenc, encryptedPasswordNibbles); err != nil {
		return err
	}
	encryptedPasswordNibblesLength[0] = driverCommon.BArray2Nibbles(jenc, encryptedPasswordNibbles)

	return nil
}

func validateNibbleBufferLength(array []byte, nibbles []byte) error {
	if len(nibbles) < len(array)*2 {
		return common.NewOracleError(oracleErrors.AuthenticatorError, nil, "nibbles buffer too small")
	}
	return nil
}
