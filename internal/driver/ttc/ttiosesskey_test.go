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
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestOSesskeyNew asserts that NewOSesskey returns a valid, non-nil object.
func TestOSesskeyNew(t *testing.T) {
	t.Parallel()
	sess := NewOSesskey()
	if sess == nil {
		t.Fatal("NewOSesskey returned nil")
	}
	if sess.GetMsgCode() != TTIFUN {
		t.Errorf("GetMsgCode = %v, want %v", sess.GetMsgCode(), TTIFUN)
	}
}

// TestOSesskeyMarshalTo_Success checks success path marshaling.
func TestOSesskeyMarshalTo_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		user      string
		logonMode uint8
	}{
		{"WithoutUser", "", 0},
		{"WithUser", "testuser", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := NewOSesskey()
			if tc.user != "" {
				(sess).(*oSessionKey).setUser(common.B1Array(tc.user))
			}
			sess.(*oSessionKey).logonMode = tc.logonMode
			buf := NewArrayDataBuffer(1024)
			engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			marshallable, _ := sess.(common.Marshallable)
			err := marshallable.MarshalTo(context.Background(), engine)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			// Basic check that data was written
			if buf.currentWritePosition == 0 {
				t.Error("No data written to buffer")
			}
		})
	}
}

// TestOSesskeyMarshalTo_GoldenMatch creates an oSesskey matching validOSesskeyMarshalDump,
// marshals it, and compares the produced bytes against the decoded golden buffer.
func TestOSesskeyMarshalTo_GoldenMatch(t *testing.T) {
	t.Parallel()
	// Create oSesskey and initialize fields to match the golden capture
	want := NewOSesskey18()
	impl := want.(*oSessionKey)

	impl.logonMode = 1 // KPZ_LOGON set in buildKeyValueList; keep explicit to be safe

	// Marshal using specified engine configuration
	buf := NewArrayDataBuffer(1024)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	marshallable, _ := want.(common.Marshallable)
	if err := marshallable.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	gotPacket := parseAuthPacketDump(t, buf.bytes[:buf.currentWritePosition])
	wantPacket := parseLegacyAuthPacketDump(t, validOSesskeyMarshalDump)

	assertAuthPacketMetadata(t, gotPacket, wantPacket)
	assertKeyValuesMatch(t, gotPacket.keyValues, wantPacket.keyValues, skipKeySet(
		authProgramNm,
		authMachine,
		authPid,
		authSid,
	))
}

// TestOSesskeyMarshalTo_Fail checks error marshaling.
func TestOSesskeyMarshalTo_Fail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		failByte  int
		failBytes int
		wantErr   string
		user      string
	}{
		{"Error Writing function code (UB1)", 1, 0, "simulated write error", "testuser"},
		{"Error Writing sequence number (first UB1)", 2, 0, "simulated write error", "testuser"},
		{"Error Writing 0", 3, 0, "simulated write error", "testuser"},
		{"Error Writing Ptr", 4, 0, "simulated write error", "testuser"},
		{"Error Writing Ptr", 4, 0, "simulated write error", ""},
		{"Error Writing user pointer/len (WriteBytes)", 0, 1, "simulated write error", "testuser"},
		{"Error Writing user pointer/len (WriteBytes)", 0, 1, "simulated write error", ""},
		{"Error Writing logon mode (UB4)", 5, 0, "simulated write error", "testuser"},
		{"Error Writing key value list (WriteBytes)", 0, 2, "simulated write error", "testuser"},
		{"Error Writing authivln", 0, 3, "simulated write error", "testuser"},
		{"Error Writing Ptr", 6, 0, "simulated write error", "testuser"},
		{"Error Writing Ptr", 7, 0, "simulated write error", "testuser"},
		{"Error Writing Ptr", 0, 4, "simulated write error", "testuser"},
		{"Error Writing Writing key value", 8, 5, "simulated write error", "testuser"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := NewOSesskey()
			// Optional: set user to ensure user path is exercised in some cases
			(sess).(*oSessionKey).setUser(common.B1Array(tc.user))

			buf := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: NewArrayDataBuffer(1024),
				FailOnWriteByteCall:  tc.failByte,
				FailOnWriteBytesCall: tc.failBytes,
			}
			engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			marshallable, _ := sess.(common.Marshallable)
			err := marshallable.MarshalTo(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			matched, matchErr := regexp.MatchString(tc.wantErr, err.Error())
			if matchErr != nil {
				t.Fatalf("failed to compile regex pattern: %v", matchErr)
			}
			if !matched {
				t.Fatalf("expected error to contain %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestOSesskeyRPANew asserts that NewOSesskeyRPA returns a valid, non-nil object.
func TestOSesskeyRPANew(t *testing.T) {
	t.Parallel()
	rpa := NewOSesskeyRPA()
	if rpa == nil {
		t.Fatal("NewOSesskeyRPA returned nil")
	}
	if rpa.GetMsgCode() != TTIRPA {
		t.Errorf("GetMsgCode = %v, want %v", rpa.GetMsgCode(), TTIRPA)
	}
}

func makeOSesskeyRPASuccessPayload() []byte {
	// Simplified payload for testing UnMarshalFrom
	// UB2 number of pairs (2)
	// Then key-value pairs
	// AUTH_SESSKEY: "sessionkey"
	// AUTH_VFR_DATA: "verifier"
	// AUTH_PBKDF2_CSK_SALT: "salt"
	// AUTH_PBKDF2_VGEN_COUNT: "4096"
	// AUTH_PBKDF2_SDER_COUNT: "3"
	buf := make([]byte, 0)
	// Number of pairs: 5
	buf = append(buf, 0, 5)

	// AUTH_SESSKEY
	buf = append(buf, 0, 0, 0, 12, 12) // key length 10
	buf = append(buf, 'A', 'U', 'T', 'H', '_', 'S', 'E', 'S', 'S', 'K', 'E', 'Y')
	buf = append(buf, 0, 0, 0, 10, 10) // value length 10
	buf = append(buf, 's', 'e', 's', 's', 'i', 'o', 'n', 'k', 'e', 'y')
	buf = append(buf, 0, 0, 0, 0) // flag

	// AUTH_VFR_DATA
	buf = append(buf, 0, 0, 0, 13, 13)
	buf = append(buf, 'A', 'U', 'T', 'H', '_', 'V', 'F', 'R', '_', 'D', 'A', 'T', 'A')
	buf = append(buf, 0, 0, 0, 8, 8)
	buf = append(buf, 'v', 'e', 'r', 'i', 'f', 'i', 'e', 'r')
	buf = append(buf, 0, 0, 0, 1) // flag 1

	// AUTH_PBKDF2_CSK_SALT
	buf = append(buf, 0, 0, 0, 20, 20)
	buf = append(buf, 'A', 'U', 'T', 'H', '_', 'P', 'B', 'K', 'D', 'F', '2', '_', 'C', 'S', 'K', '_', 'S', 'A', 'L', 'T')
	buf = append(buf, 0, 0, 0, 4, 4)
	buf = append(buf, 's', 'a', 'l', 't')
	buf = append(buf, 0, 0, 0, 0)

	// AUTH_PBKDF2_VGEN_COUNT
	buf = append(buf, 0, 0, 0, 22, 22)
	buf = append(buf, 'A', 'U', 'T', 'H', '_', 'P', 'B', 'K', 'D', 'F', '2', '_', 'V', 'G', 'E', 'N', '_', 'C', 'O', 'U', 'N', 'T')
	buf = append(buf, 0, 0, 0, 4, 4)
	buf = append(buf, '4', '0', '9', '6')
	buf = append(buf, 0, 0, 0, 0)

	// AUTH_PBKDF2_SDER_COUNT
	buf = append(buf, 0, 0, 0, 22, 22)
	buf = append(buf, 'A', 'U', 'T', 'H', '_', 'P', 'B', 'K', 'D', 'F', '2', '_', 'S', 'D', 'E', 'R', '_', 'C', 'O', 'U', 'N', 'T')
	buf = append(buf, 0, 0, 0, 1, 1)
	buf = append(buf, '3')
	buf = append(buf, 0, 0, 0, 0)

	return buf
}

// TestOSesskeyRPAUnMarshalFrom_Success checks success path unmarshaling.
func TestOSesskeyRPAUnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	payload := makeOSesskeyRPASuccessPayload()
	rpa := NewOSesskeyRPA()
	buf := NewArrayDataBuffer(1024)
	buf.WriteBytesWithContext(context.Background(), payload)
	engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
	unmarshallable, _ := rpa.(common.UnMarshallable)
	err := unmarshallable.UnMarshalFrom(context.Background(), engine)
	if err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	if !rpa.(*oSesskeyRPA).connectionValues.ContainsKey("AUTH_SESSKEY") {
		t.Error("Missing AUTH_SESSKEY")
	}
	if !rpa.(*oSesskeyRPA).connectionValues.ContainsKey("AUTH_VFR_DATA") {
		t.Error("Missing AUTH_VFR_DATA")
	}
	if !rpa.(*oSesskeyRPA).connectionValues.ContainsKey("AUTH_PBKDF2_CSK_SALT") {
		t.Error("Missing AUTH_PBKDF2_CSK_SALT")
	}
	if !rpa.(*oSesskeyRPA).connectionValues.ContainsKey("AUTH_PBKDF2_VGEN_COUNT") {
		t.Error("Missing AUTH_PBKDF2_VGEN_COUNT")
	}
	if !rpa.(*oSesskeyRPA).connectionValues.ContainsKey("AUTH_PBKDF2_SDER_COUNT") {
		t.Error("Missing AUTH_PBKDF2_SDER_COUNT")
	}

	sessionProperties := rpa.(*oSesskeyRPA).connectionValues

	if len(sessionProperties.GetProperty(authSesskey).(*common.KeyValue).Value) == 0 {
		t.Error("Encrypted SK is empty")
	}
	if int(sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Flag) != 1 {
		t.Errorf("Verifier type = %d, want 1", int(sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Flag))
	}

	PBKDF2VgenCount, err := strconv.Atoi(string(sessionProperties.GetProperty(authPbkdf2VgenCount).(*common.KeyValue).Value))
	if PBKDF2VgenCount != 4096 {
		t.Errorf("PBKDF2VgenCount = %d, want 4096", PBKDF2VgenCount)
	}
	PBKDF2SderCount, err := strconv.Atoi(common.B1ArrayToString(sessionProperties.GetProperty(authPbkdf2SderCount).(*common.KeyValue).Value))
	if PBKDF2SderCount != 3 {
		t.Errorf("PBKDF2SderCount = %d, want 3", PBKDF2SderCount)
	}
}

// TestOSesskeyRPAUnMarshalFrom_Fail checks error unmarshaling.
func TestOSesskeyRPAUnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		payload   []byte
		failByte  int
		failBytes int
		wantErr   string
	}{
		// Simulated read failures using FaultyArrayBasedDataBuffer
		{"Error Reading Number of Pairs (ReadByte)", []byte{0x00, 0x05}, 1, 0, "simulated read error"},
		{"Error Reading Key Value List (ReadBytes)", makeOSesskeyRPAPayload(), 0, 2, "simulated read error"},
		// Missing keys in payload
		{"Missing AUTH_SESSKEY", makeOSesskeyRPAMissingKeyPayload("AUTH_SESSKEY"), 0, 0, "Failed to unmarshal message"},
		{"Missing AUTH_VFR_DATA", makeOSesskeyRPAMissingKeyPayload("AUTH_VFR_DATA"), 0, 0, "Failed to unmarshal message"},
		{"Missing AUTH_PBKDF2_CSK_SALT", makeOSesskeyRPAMissingKeyPayload("AUTH_PBKDF2_CSK_SALT"), 0, 0, "Failed to unmarshal message"},
		{"Missing AUTH_PBKDF2_VGEN_COUNT", makeOSesskeyRPAMissingKeyPayload("AUTH_PBKDF2_VGEN_COUNT"), 0, 0, "Failed to unmarshal message"},
		{"Missing AUTH_PBKDF2_SDER_COUNT", makeOSesskeyRPAMissingKeyPayload("AUTH_PBKDF2_SDER_COUNT"), 0, 0, "Failed to unmarshal message"},
		// Invalid counts that must fail strconv.Atoi
		{"Invalid VGEN_COUNT", makeOSesskeyRPAInvalidCountPayload("VGEN", "abcd"), 0, 0, "strconv.Atoi"},
		{"Invalid SDER_COUNT", makeOSesskeyRPAInvalidCountPayload("SDER", "d"), 0, 0, "strconv.Atoi"},
		{"Invalid VGEN_COUNT_1", makeOSesskeyRPAInvalidCountPayload("VGEN", "1000"), 0, 0, ""},
		{"Invalid SDER_COUNT_2", makeOSesskeyRPAInvalidCountPayload("SDER", "1"), 0, 0, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rpa := NewOSesskeyRPA()
			var dataBuf *ArrayBasedDataBuffer
			if tc.payload != nil {
				dataBuf = NewArrayDataBuffer(4096)
				_ = dataBuf.WriteBytesWithContext(context.Background(), tc.payload)
			} else {
				dataBuf = NewArrayDataBuffer(16)
			}

			faulty := &FaultyArrayBasedDataBuffer{
				ArrayBasedDataBuffer: dataBuf,
				FailOnReadByteCall:   tc.failByte,
				FailOnReadBytesCall:  tc.failBytes,
			}

			engine := NewMarshalEngine(faulty, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
			unmarshallable, _ := rpa.(common.UnMarshallable)

			// zsession.PrintPacket(tc.payload, 0, len(tc.payload))
			err := unmarshallable.UnMarshalFrom(context.Background(), engine)
			if tc.name == "Invalid VGEN_COUNT_1" || tc.name == "Invalid SDER_COUNT_2" {
				return
			}
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error mismatch: want to contain %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func makeOSesskeyRPAMissingKeyPayload(missingKey string) []byte {
	buf := make([]byte, 0)
	// Number of pairs: 4 (missing one)
	count := 4
	if missingKey == "" {
		count = 5
	}
	buf = append(buf, 1, byte(count))

	keys := []string{"AUTH_SESSKEY", "AUTH_VFR_DATA", "AUTH_PBKDF2_CSK_SALT", "AUTH_PBKDF2_VGEN_COUNT", "AUTH_PBKDF2_SDER_COUNT"}
	for _, key := range keys {
		if key == missingKey {
			continue
		}
		buf = append(buf, 1, byte(len(key)), byte(len(key)))
		buf = append(buf, []byte(key)...)
		buf = append(buf, 1, 1, 1)
		buf = append(buf, '1')
		buf = append(buf, 0)
	}
	return buf
}

func makeOSesskeyRPAInvalidCountPayload(countType, value string) []byte {
	buf := makeOSesskeyRPAPayload()
	// Replace the count value
	if countType == "VGEN" {
		// Find AUTH_PBKDF2_VGEN_COUNT and replace value
		idx := 0
		for i := 0; i < len(buf); i++ {
			if i+22 < len(buf) && string(buf[i:i+22]) == "AUTH_PBKDF2_VGEN_COUNT" {
				// Skip key, then value length, then replace value
				idx = i + 22 + 3
				break
			}
		}
		if idx > 0 {
			copy(buf[idx:], []byte(value))
		}
	} else if countType == "SDER" {
		idx := 0
		for i := 0; i < len(buf); i++ {
			if i+22 < len(buf) && string(buf[i:i+22]) == "AUTH_PBKDF2_SDER_COUNT" {
				idx = i + 22 + 3
				break
			}
		}
		if idx > 0 {
			copy(buf[idx:], []byte(value))
		}
	}
	return buf
}

// TestOSesskeyRPAGetters tests the getter methods.
func TestOSesskeyRPAGetters(t *testing.T) {
	t.Parallel()
	payload := makeOSesskeyRPASuccessPayload()
	rpa := NewOSesskeyRPA()
	buf := NewArrayDataBuffer(1024)
	buf.WriteBytesWithContext(context.Background(), payload)
	engine := NewNativeMarshalEngine(buf, common.BIG_ENDIAN)
	unmarshallable, _ := rpa.(common.UnMarshallable)
	err := unmarshallable.UnMarshalFrom(context.Background(), engine)
	if err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	sessionProperties := rpa.(*oSesskeyRPA).connectionValues
	sk := sessionProperties.GetProperty(authSesskey).(*common.KeyValue).Value
	if len(sk) == 0 {
		t.Error("getEncryptedSK returned empty")
	}

	salt := sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Value
	if len(salt) == 0 {
		t.Error("getSalt returned empty")
	}

	vt := int(sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Flag)
	if vt != 1 {
		t.Errorf("getVerifierType = %d, want 1", vt)
	}

	pbkdfSalt := sessionProperties.GetProperty("AUTH_PBKDF2_CSK_SALT").(*common.KeyValue).Value
	if len(pbkdfSalt) == 0 {
		t.Error("getPBKDF2Salt returned empty")
	}

	vgen, err := strconv.Atoi(string(sessionProperties.GetProperty(authPbkdf2VgenCount).(*common.KeyValue).Value))
	if vgen != 4096 {
		t.Errorf("getPBKDF2VgenCount = %d, want 4096", vgen)
	}

	sder, err := strconv.Atoi(common.B1ArrayToString(sessionProperties.GetProperty(authPbkdf2SderCount).(*common.KeyValue).Value))
	if sder != 3 {
		t.Errorf("getPBKDF2SderCount = %d, want 3", sder)
	}
}

// TestOSesskeyRPAUnMarshalFrom_Golden unmarshals the golden RPA hex dump and
// verifies the parsed connection values and derived fields.
func TestOSesskeyRPAUnMarshalFrom_Golden(t *testing.T) {
	t.Parallel()
	payload := makeOSesskeyRPAPayload()
	if len(payload) == 0 {
		t.Fatal("golden RPA payload decode returned empty")
	}

	rpa := NewOSesskeyRPA()
	buf := NewArrayDataBuffer(4096)
	buf.WriteBytesWithContext(context.Background(), payload)
	engine := NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	unmarshallable, _ := rpa.(common.UnMarshallable)
	if err := unmarshallable.UnMarshalFrom(context.Background(), engine); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	cv := rpa.(*oSesskeyRPA).connectionValues
	requiredKeys := []string{
		"AUTH_SESSKEY",
		"AUTH_VFR_DATA",
		"AUTH_PBKDF2_CSK_SALT",
		"AUTH_PBKDF2_VGEN_COUNT",
		"AUTH_PBKDF2_SDER_COUNT",
		"AUTH_GLOBALLY_UNIQUE_DBID",
	}
	for _, k := range requiredKeys {
		if !cv.ContainsKey(k) {
			t.Errorf("missing key %s", k)
		}
	}

	sessionProperties := rpa.(*oSesskeyRPA).connectionValues
	// Basic field validations
	if len(sessionProperties.GetProperty(authSesskey).(*common.KeyValue).Value) == 0 {
		t.Error("encryptedSK is empty")
	}
	if len(sessionProperties.GetProperty("AUTH_VFR_DATA").(*common.KeyValue).Value) == 0 {
		t.Error("salt is empty")
	}

	// Validate PBKDF2 counts if present (expect 4096 and 3 from the golden dump)
	vgen, _ := strconv.Atoi(string(sessionProperties.GetProperty(authPbkdf2VgenCount).(*common.KeyValue).Value))
	if vgen != 4096 {
		t.Errorf("PBKDF2VgenCount = %d, want 4096", vgen)
	}
	sder, _ := strconv.Atoi(common.B1ArrayToString(sessionProperties.GetProperty(authPbkdf2SderCount).(*common.KeyValue).Value))
	if sder != 3 {
		t.Errorf("PBKDF2SderCount = %d, want 3", sder)
	}
}
