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
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
)

// TestNewOAuth_Success tests the NewOAuth constructor.
func TestNewOAuth_Success(t *testing.T) {
	t.Parallel()
	oAuth := NewOAuth().(*oAuth)
	if oAuth == nil {
		t.Fatal("NewOAuth returned nil")
	}
	if oAuth.GetMsgCode() != TTIFUN {
		t.Errorf("Expected message code TTIFUN, got %v", oAuth.GetMsgCode())
	}
	if oAuth.GetFuncCode() != oauth {
		t.Errorf("Expected function code oauth, got %v", oAuth.GetFuncCode())
	}
}

// TestOAuth_setSessionFields tests the setSessionFields method.
func TestOAuth_setSessionFields(t *testing.T) {
	// Helper to get alterSession as string (it may be 0-terminated)
	asString := func(b []byte) string {
		s := common.B1ArrayToString(b)
		// Ensure we don't fail due to a trailing null terminator
		return strings.TrimRight(s, "\x00")
	}

	t.Run("Defaults when env vars are empty", func(t *testing.T) {
		t.Setenv("NLS_LANGUAGE", "")
		t.Setenv("NLS_TERRITORY", "")
		oauth := NewOAuth().(*oAuth)
		oauth.isSessionTZ = true // Force alter session to be set

		oauth.setSessionFields()

		if oauth.sessionTimeZone == "" {
			t.Error("Expected sessionTimeZone to be set")
		}
		got := asString(oauth.alterSession)
		if got == "" {
			t.Fatal("Expected alterSession to be set when isSessionTZ is true")
		}
		if !strings.Contains(got, "ALTER SESSION SET ") {
			t.Errorf("alterSession missing prefix, got: %q", got)
		}
		if !strings.Contains(got, "TIME_ZONE='") {
			t.Errorf("alterSession missing TIME_ZONE, got: %q", got)
		}
		if !strings.Contains(got, "NLS_LANGUAGE='AMERICAN'") {
			t.Errorf("Expected default NLS_LANGUAGE='AMERICAN', got: %q", got)
		}
		if !strings.Contains(got, "NLS_TERRITORY='AMERICA'") {
			t.Errorf("Expected default NLS_TERRITORY='AMERICA', got: %q", got)
		}
	})

	t.Run("Both env vars set", func(t *testing.T) {
		t.Setenv("NLS_LANGUAGE", "FRENCH")
		t.Setenv("NLS_TERRITORY", "FRANCE")
		oauth := NewOAuth().(*oAuth)
		oauth.isSessionTZ = true

		oauth.setSessionFields()

		got := asString(oauth.alterSession)
		if !strings.Contains(got, "NLS_LANGUAGE='FRENCH'") {
			t.Errorf("Expected NLS_LANGUAGE='FRENCH', got: %q", got)
		}
		if !strings.Contains(got, "NLS_TERRITORY='FRANCE'") {
			t.Errorf("Expected NLS_TERRITORY='FRANCE', got: %q", got)
		}
	})

	t.Run("Only language set", func(t *testing.T) {
		t.Setenv("NLS_LANGUAGE", "SPANISH")
		t.Setenv("NLS_TERRITORY", "")
		oauth := NewOAuth().(*oAuth)
		oauth.isSessionTZ = true

		oauth.setSessionFields()

		got := asString(oauth.alterSession)
		if !strings.Contains(got, "NLS_LANGUAGE='SPANISH'") {
			t.Errorf("Expected NLS_LANGUAGE='SPANISH', got: %q", got)
		}
		if !strings.Contains(got, "NLS_TERRITORY='AMERICA'") {
			t.Errorf("Expected default NLS_TERRITORY='AMERICA', got: %q", got)
		}
	})

	t.Run("Only territory set", func(t *testing.T) {
		t.Setenv("NLS_LANGUAGE", "")
		t.Setenv("NLS_TERRITORY", "GERMANY")
		oauth := NewOAuth().(*oAuth)
		oauth.isSessionTZ = true

		oauth.setSessionFields()

		got := asString(oauth.alterSession)
		if !strings.Contains(got, "NLS_LANGUAGE='AMERICAN'") {
			t.Errorf("Expected default NLS_LANGUAGE='AMERICAN', got: %q", got)
		}
		if !strings.Contains(got, "NLS_TERRITORY='GERMANY'") {
			t.Errorf("Expected NLS_TERRITORY='GERMANY', got: %q", got)
		}
	})

	t.Run("Escapes single quotes in env-provided literals", func(t *testing.T) {
		t.Setenv("NLS_LANGUAGE", "AMER'ICAN")
		t.Setenv("NLS_TERRITORY", "A'MERICA")
		oauth := NewOAuth().(*oAuth)
		oauth.isSessionTZ = true

		oauth.setSessionFields()

		got := asString(oauth.alterSession)
		if !strings.Contains(got, "NLS_LANGUAGE='AMER''ICAN'") {
			t.Errorf("Expected escaped NLS_LANGUAGE literal, got: %q", got)
		}
		if !strings.Contains(got, "NLS_TERRITORY='A''MERICA'") {
			t.Errorf("Expected escaped NLS_TERRITORY literal, got: %q", got)
		}
	})
}

// TestOAuth_MarshalTo_Success tests successful marshaling.
func TestOAuth_MarshalTo_Success(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth().(*oAuth)
	buf := NewArrayDataBuffer(1500)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	// Set up minimal oAuth data
	oauth.user = common.B1Array("testuser")
	oauth.logonMode = KpzLogon
	k := newKeyValueList()
	k.PushBack(&common.KeyValue{Key: common.StringToB1Array("key"), Value: common.StringToB1Array("value")})
	oauth.keyValList = k

	err := oauth.MarshalTo(context.Background(), engine)
	if err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	if buf.currentWritePosition == 0 {
		t.Error("Expected data to be written to buffer")
	}
}

func TestOAuth_MarshalTo_WithOSESSKEYRPA_Success(t *testing.T) {
	t.Parallel()
	// 1) Unmarshal oSesskeyRPA from golden payload
	payload := makeOSesskeyRPAPayload()
	if len(payload) == 0 {
		t.Fatal("oSesskey RPA payload decode returned empty")
	}
	rpa := NewOSesskeyRPA()
	rpaBuf := NewArrayDataBuffer(8192)
	_ = rpaBuf.WriteBytesWithContext(context.Background(), payload)
	rpaEngine := NewMarshalEngine(rpaBuf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	unmarshallable, _ := rpa.(common.UnMarshallable)
	if err := unmarshallable.UnMarshalFrom(context.Background(), rpaEngine); err != nil {
		t.Fatalf("UnMarshalFrom oSesskeyRPA failed: %v", err)
	}

	// 2) Build oAuth and set fields as in _doOAuth (o5logon_authenticator._doOAuth)
	oauth := NewOAuth18().(*oAuth)
	// Fields sourced from oSesskeyRPA
	sessionProperties := rpa.(*oSesskeyRPA).connectionValues

	oauth.setEncryptedSK(sessionProperties.GetProperty(authSesskey).(*common.KeyValue).Value)
	oauth.setSalt(sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Value)
	oauth.setVerifierType(int(sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Flag))
	oauth.setPBKDF2Salt(sessionProperties.GetProperty("AUTH_PBKDF2_CSK_SALT").(*common.KeyValue).Value)
	vgen, _ := strconv.Atoi(string(sessionProperties.GetProperty(authPbkdf2VgenCount).(*common.KeyValue).Value))
	oauth.setPBKDF2VgenCount(vgen)
	sder, _ := strconv.Atoi(common.B1ArrayToString(sessionProperties.GetProperty(authPbkdf2SderCount).(*common.KeyValue).Value))
	oauth.setPBKDF2SderCount(sder)

	// Fields typically supplied by the authenticator (capability, username, connect string)

	oauth.setUser(currentUserName)
	oauth.setConnectString("(DESCRIPTION=(ADDRESS=(HOST=localhost)(PORT=1521)(PROTOCOL=tcp))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")
	oauth._bUseO5Logon = true // TODO : why are we forced to hard code it.
	oauth._hasO7LMRSupport = true
	// 3) Initialize oauth response to populate key/value list
	if err := oauth._initializeOAuthResponse(currentUserName); err != nil {
		t.Fatalf("_initializeOAuthResponse failed: %v", err)
	}

	// 4) Marshal oAuth and assert bytes were produced
	buf := NewArrayDataBuffer(8192)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	if err := oauth.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]

	gotPacket := parseAuthPacketDump(t, got)
	wantPacket := parseLegacyAuthPacketDump(t, validOauthMarshalDump)

	assertAuthPacketMetadata(t, gotPacket, wantPacket)
	assertKeyValuesMatch(t, gotPacket.keyValues, wantPacket.keyValues, skipKeySet(
		authProgramNm,
		authMachine,
		authPid,
		authSesskey,
		authPassword,
		authPbkdf2SpeedyKey,
		authAlterSession,
	))
}

func TestOAuthMarshalTo_Fail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
		wantErr   string
		withUser  bool
	}{
		{"Error Writing function code (UB1)", 1, 0, "simulated write error", true},
		{"Error Writing sequence number (UB1)", 2, 0, "simulated write error", true},
		{"Error Writing zero UB1", 3, 0, "simulated write error", true},
		{"Error Writing authusr PTR (WriteByte)", 4, 0, "simulated write error", true},
		{"Error Writing user pointer/len (WriteBytes)", 0, 1, "simulated write error", true},
		{"Error Writing null PTR when no user (WriteByte)", 4, 0, "simulated write error", false},
		{"Error Writing user pointer/len (WriteBytes)", 0, 1, "simulated write error", false},
		{"Error Writing logon mode (UB4)", 5, 0, "simulated write error", true},
		{"Error Writing authivl PTR", 6, 0, "simulated write error", true},
		{"Error Writing authivln (UB4)", 0, 2, "simulated write error", true},
		{"Error Writing authovl PTR", 7, 0, "simulated write error", true},
		{"Error Writing authovln PTR", 0, 3, "simulated write error", true},
		{"Error Writing user bytes (WriteBytes)", 0, 4, "simulated write error", true},
		{"Error Writing keyValueList (WriteBytes)", 0, 5, "simulated write error", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oauth := NewOAuth().(*oAuth)
			oauth.user = common.StringToB1Array("testuser")
			oauth.logonMode = KpzLogon
			k := newKeyValueList()
			k.PushBack(&common.KeyValue{Key: common.StringToB1Array("key"), Value: common.StringToB1Array("value")})

			oauth.keyValList = k

			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(1024),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			err := oauth.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.wantErr)) {
				t.Fatalf("expected error to contain %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestOAuth_prepareForOAUTH tests the prepareForOAUTH method.
func TestOAuth_prepareForOAUTH(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth18().(*oAuth)
	user := "testuser"
	password := common.StringToB1Array("testpass")

	err := oauth.prepareForOAUTH(common.B1Array(user), password, nil)
	if err != nil {
		t.Fatalf("prepareForOAUTH failed: %v", err)
	}

	if oauth.logonMode != KpzLogon|KpzPasswdEncrypted {
		t.Errorf("Expected logonMode %d, got %d", KpzLogon|KpzPasswdEncrypted, oauth.logonMode)
	}
	if oauth.keyValList == nil || oauth.keyValList.Len() == 0 {
		t.Error("keyValueList not initialized")
	}
}

// TestOAuth_initializeLogonModeForOAUTH tests logon mode initialization.
func TestOAuth_initializeLogonModeForOAUTH(t *testing.T) {
	t.Parallel()
	pass := []byte("pass")
	cases := []struct {
		name         string
		user         string
		logonMode    int64
		password     []byte
		expectedMode int64
	}{
		{"With user and password", "user", 0, pass, KpzLogon | KpzPasswdEncrypted},
		{"With user no password", "user", 0, nil, KpzLogon},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oauth := NewOAuth().(*oAuth)
			oauth.initializeLogonModeForOAUTH(common.B1Array(tc.user), tc.logonMode, tc.password)
			if oauth.logonMode != tc.expectedMode {
				t.Errorf("Expected logonMode %d, got %d", tc.expectedMode, oauth.logonMode)
			}
		})
	}
}

// TestOAuth_setPasswordKeyValsForOAUTH tests password key-value setting.
func TestOAuth_setPasswordKeyValsForOAUTH(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth().(*oAuth)
	k := newKeyValueList()
	oauth.keyValList = k

	password := common.StringToB1Array("testpass")
	speedyKey := common.StringToB1Array("speedy")

	oauth.setPasswordKeyValsForOAUTH(password, speedyKey)

	if oauth.keyValList.Len() != 2 {
		t.Errorf("Expected 2 key-value pairs, got %d", oauth.keyValList.Len())
	}

	foundPassword := false
	foundSpeedy := false
	for e := oauth.keyValList.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*common.KeyValue)
		keyStr := string(kv.Key)
		if keyStr == "AUTH_PASSWORD" {
			foundPassword = true
			if !bytes.Equal(kv.Value, password) {
				t.Error("Password value not set correctly")
			}
		} else if keyStr == "AUTH_PBKDF2_SPEEDY_KEY" {
			foundSpeedy = true
			if !bytes.Equal(kv.Value, speedyKey) {
				t.Error("Speedy key value not set correctly")
			}
		}
	}

	if !foundPassword {
		t.Error("Password key not found")
	}
	if !foundSpeedy {
		t.Error("Speedy key not found")
	}
}

// TestOAuth_setPasswordKeyValsForOAUTH_WithEncryptedKB tests with encrypted KB.
func TestOAuth_setPasswordKeyValsForOAUTH_WithEncryptedKB(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth().(*oAuth)
	k := newKeyValueList()
	oauth.keyValList = k
	oauth.encryptedKB = []byte("encryptedKB")

	password := common.StringToB1Array("testpass")

	oauth.setPasswordKeyValsForOAUTH(password, nil)

	foundSessKey := false
	for e := oauth.keyValList.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*common.KeyValue)
		keyStr := string(kv.Key)
		if keyStr == "AUTH_SESSKEY" {
			foundSessKey = true
			if !bytes.Equal(kv.Value, oauth.encryptedKB) {
				t.Error("Session key value not set correctly")
			}
			if kv.Flag != 1 {
				t.Errorf("Expected flag 1, got %d", kv.Flag)
			}
		}
	}

	if !foundSessKey {
		t.Error("Session key not found")
	}
}

// TestOAuth_setDriverIdentityKeyValsForOAUTH tests driver identity key-value setting.
func TestOAuth_setDriverIdentityKeyValsForOAUTH(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth().(*oAuth)
	k := newKeyValueList()
	oauth.keyValList = k

	edition := common.StringToB1Array("edition")
	oauth.editionName = edition

	oauth.setDriverIdentityKeyValsForOAUTH()

	foundEdition := false
	foundDriverName := false
	foundDriverVersion := false

	for e := oauth.keyValList.Front(); e != nil; e = e.Next() {
		kv := e.Value.(*common.KeyValue)
		keyStr := string(kv.Key)
		switch keyStr {
		case "AUTH_ORA_EDITION":
			foundEdition = true
			if !bytes.Equal(kv.Value, edition) {
				t.Error("Edition value not set correctly")
			}
		case "SESSION_CLIENT_DRIVER_NAME":
			foundDriverName = true
			if len(kv.Value) == 0 {
				t.Error("Driver name not set")
			}
		case "SESSION_CLIENT_VERSION":
			foundDriverVersion = true
			if len(kv.Value) == 0 {
				t.Error("Driver version not set")
			}
		}
	}

	if !foundEdition {
		t.Error("Edition key not found")
	}
	if !foundDriverName {
		t.Error("Driver name key not found")
	}
	if !foundDriverVersion {
		t.Error("Driver version key not found")
	}
}

// TestOAuth_setAlterSessionKeyValsForOAUTH tests alter session key-value setting.
func TestOAuth_setAlterSessionKeyValsForOAUTH(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth().(*oAuth)
	k := newKeyValueList()
	oauth.keyValList = k

	oauth.alterSession = common.StringToB1Array("ALTER SESSION SET something")

	oauth.setAlterSessionKeyValsForOAUTH()

	if oauth.keyValList.Len() != 1 {
		t.Errorf("Expected 1 key-value pair, got %d", oauth.keyValList.Len())
	}

	kv := oauth.keyValList.Front()
	if string(kv.Value.(*common.KeyValue).Key) != "AUTH_ALTER_SESSION" {
		t.Errorf("Expected key %s, got %s", "AUTH_ALTER_SESSION", string(kv.Value.(*common.KeyValue).Key))
	}
	if kv.Value.(*common.KeyValue).Flag != 1 {
		t.Errorf("Expected flag 1, got %d", kv.Value.(*common.KeyValue).Flag)
	}
}

// TestOAuth_validateKeySizeForOAUTH_Success tests successful key size validation.
func TestOAuth_validateKeySizeForOAUTH_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		encryptedSK []byte
		bUseO5Logon bool
	}{
		{"Non-O5 logon valid", make([]byte, 16), false},
		{"O5 logon valid 64 bytes", make([]byte, 64), true},
		{"O5 logon valid 96 bytes", make([]byte, 96), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oauth := NewOAuth().(*oAuth)
			oauth.encryptedSK = tc.encryptedSK
			oauth._bUseO5Logon = tc.bUseO5Logon

			err := oauth.validateKeySizeForOAUTH()
			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

// TestOAuth_validateKeySizeForOAUTH_Failure tests key size validation failures.
func TestOAuth_validateKeySizeForOAUTH_Failure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		encryptedSK []byte
		bUseO5Logon bool
	}{
		{"Non-O5 logon too long", make([]byte, 17), false},
		{"O5 logon nil", nil, true},
		{"O5 logon wrong size", make([]byte, 32), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oauth := NewOAuth().(*oAuth)
			oauth.encryptedSK = tc.encryptedSK
			oauth._bUseO5Logon = tc.bUseO5Logon

			err := oauth.validateKeySizeForOAUTH()
			if err == nil {
				t.Error("Expected error, got nil")
			}
		})
	}
}

// TestOAuth_sanitizeInputCredential tests credential sanitization.
func TestOAuth_sanitizeInputCredential(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"Normal string", "test", "test"},
		{"With spaces", " test ", "test"},
		{"Quoted string", "\"test\"", "test"},
		{"Empty string", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeInputCredential(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestPasswordAuthenticatorValidatePasswordLength(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{"AtLimit", maxPasswordBytes, false},
		{"OverLimit", maxPasswordBytes + 1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pa := newPasswordAuthenticator("user", strings.Repeat("a", tc.length), "connect")
			err := pa.validatePasswordLength()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validatePasswordLength error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestOAuth_validateO5VerifierType_Success tests successful verifier type validation.
func TestOAuth_validateO5VerifierType_Success(t *testing.T) {
	t.Parallel()
	validTypes := []int{ZtvtOrcl7, ZtvtMd5, ZtvtSmd5, ZtvtSh1, ZtvtSSH1, ZtvtSha512}

	for _, vt := range validTypes {
		t.Run(string(rune(vt)), func(t *testing.T) {
			oauth := NewOAuth().(*oAuth)
			oauth.verifierType = vt

			err := oauth.setVerifierType(vt)
			if err != nil {
				t.Errorf("Expected no error for verifier type %d, got %v", vt, err)
			}
		})
	}
}

// TestOAuth_setters tests all setter methods.
func TestOAuth_setters(t *testing.T) {
	t.Parallel()
	oauth := NewOAuth().(*oAuth)

	// Test setSalt
	salt := common.StringToB1Array("salt")
	oauth.setSalt(salt)
	if !bytes.Equal(oauth.salt, salt) {
		t.Error("setSalt failed")
	}

	// Test setEncryptedSK
	sk := common.StringToB1Array("encryptedSK")
	oauth.setEncryptedSK(sk)
	if !bytes.Equal(oauth.encryptedSK, sk) {
		t.Error("setEncryptedSK failed")
	}

	// Test setVerifierType
	oauth.setVerifierType(ZtvtMd5)
	if oauth.verifierType != ZtvtMd5 {
		t.Error("setVerifierType failed")
	}

	// Test setPBKDF2Salt
	pbkdfSalt := common.StringToB1Array("pbkdfSalt")
	oauth.setPBKDF2Salt(pbkdfSalt)
	if !bytes.Equal(oauth.PBKDF2Salt, pbkdfSalt) {
		t.Error("setPBKDF2Salt failed")
	}

	// Test setPBKDF2VgenCount
	oauth.setPBKDF2VgenCount(1000)
	if oauth.PBKDF2VgenCount != 1000 {
		t.Error("setPBKDF2VgenCount failed")
	}

	// Test setPBKDF2SderCount
	oauth.setPBKDF2SderCount(2000)
	if oauth.PBKDF2SderCount != 2000 {
		t.Error("setPBKDF2SderCount failed")
	}

	// Test setUser
	user := "testuser"
	oauth.setUser(common.StringToB1Array(user))
	if string(oauth.user) != user {
		t.Error("setUser failed")
	}

	// Test setConnectString
	connStr := "connectString"
	oauth.setConnectString(connStr)
	if string(oauth.connectString) != connStr {
		t.Error("setConnectString failed")
	}
}

// TestOAuthRPA_NewOAuthRPA tests OAuthRPA constructor.
func TestOAuthRPA_NewOAuthRPA(t *testing.T) {
	t.Parallel()
	rpa := NewOAuthRPA()
	if rpa == nil {
		t.Fatal("NewOAuthRPA returned nil")
	}
	if rpa.GetMsgCode() != TTIRPA {
		t.Errorf("Expected message code TTIRPA, got %v", rpa.GetMsgCode())
	}
}

func TestOAuthRPA_UnMarshalFrom_Golden(t *testing.T) {
	t.Parallel()
	payload := makeOauthRPAPayload()
	if len(payload) == 0 {
		t.Fatal("golden oAuth RPA payload decode returned empty")
	}

	rpa := NewOAuthRPA()
	buf := NewArrayDataBuffer(8192)
	_ = buf.WriteBytesWithContext(context.Background(), payload)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	unmarshallable, _ := rpa.(common.UnMarshallable)
	if err := unmarshallable.UnMarshalFrom(context.Background(), engine); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	cv := rpa.(*OAuthRPA).connectionValues
	// Required by UnMarshalFrom; presence indicates success parsing
	if !cv.ContainsKey(authDbMountID) {
		t.Errorf("missing key %s", authDbMountID)
	}
	// Verify mapped connection properties are present
	if cv.GetProperty(serverHost) == "" {
		t.Error("SERVER_HOST not set")
	}
	if cv.GetProperty(instanceName) == "" {
		t.Error("INSTANCE_NAME not set")
	}
	if cv.GetProperty(databaseName) == "" {
		t.Error("DATABASE_NAME not set")
	}
	if cv.GetProperty(serviceName) == "" {
		t.Error("SERVICE_NAME not set")
	}
}

func TestOAuthRPAUnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		payload      []byte
		failByte     int
		failBytes    int
		wantContains string
	}{
		// Simulated read failures
		{"Error Reading Number of Pairs (ReadByte)", []byte{0x00}, 1, 0, "simulated read error"},
		{"Error Reading Key Value List (ReadBytes)", makeOauthRPAPayload(), 0, 2, "simulated read error"},
		// Logical failures based on content
		{"Missing AUTH_DB_MOUNT_ID", []byte{0x00}, 0, 0, "Failed to unmarshal message"},
		{"Invalid AUTH_DB_MOUNT_ID (parse error)", nil, 0, 0, "Failed to unmarshal message"},
		{"NOMOUNT AUTH_DB_MOUNT_ID == 0", nil, 0, 0, "NOMOUNT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf *ArrayBasedDataBuffer
			var payload []byte

			switch tc.name {
			case "Invalid AUTH_DB_MOUNT_ID (parse error)":
				p := make([]byte, 0)
				p = append(p, makeOauthRPAPayload()...)
				overwriteValueAfterKey(p, authDbMountID, []byte("cbcddfegeg"))
				payload = p
			case "NOMOUNT AUTH_DB_MOUNT_ID == 0":
				payload = makeOauthRPANoMount()
			default:
				if tc.payload != nil {
					payload = tc.payload
				} else {
					payload = []byte{}
				}
			}

			if payload != nil {
				buf = NewArrayDataBuffer(8192)
				_ = buf.WriteBytesWithContext(context.Background(), payload)
			} else {
				buf = NewArrayDataBuffer(16)
			}

			var engine common.Marshaller
			if tc.failByte != 0 || tc.failBytes != 0 {
				faulty := &FaultyArrayBasedDataBuffer{
					ArrayBasedDataBuffer: buf,
					FailOnReadByteCall:   tc.failByte,
					FailOnReadBytesCall:  tc.failBytes,
				}
				engine = NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			} else {
				engine = NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			}
			session.PrintPacket(payload, 0, len(payload))
			rpa := NewOAuthRPA()
			unmarshallable, _ := rpa.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(context.Background(), engine)

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.wantContains)) {
				t.Fatalf("error mismatch: want to contain %q, got %v", tc.wantContains, err)
			}
		})
	}
}

// TestOAuthRPA_UnMarshalFrom_Fail tests failed unmarshaling.
func TestOAuthRPA_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	rpa := NewOAuthRPA().(*OAuthRPA)
	buf := NewArrayDataBuffer(1024)

	// Create minimal RPA payload
	payload := []byte{0, 0, 0, 0} // nbPairs = 0
	_ = buf.WriteBytesWithContext(context.Background(), payload)

	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	err := rpa.UnMarshalFrom(context.Background(), engine)
	if err == nil {
		t.Error("Expected Error")
	}
}

// TestOAuthRPA_UnMarshalFrom_Failure tests unmarshaling failure.
func TestOAuthRPA_UnMarshalFrom_Failure(t *testing.T) {
	t.Parallel()
	rpa := NewOAuthRPA().(*OAuthRPA)
	buf := NewArrayDataBuffer(10)

	// Write invalid data
	payload := []byte{1, 0} // nbPairs = 1, but no data
	_ = buf.WriteBytesWithContext(context.Background(), payload)

	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	err := rpa.UnMarshalFrom(context.Background(), engine)
	if err == nil {
		t.Error("Expected error for invalid data")
	}
}
