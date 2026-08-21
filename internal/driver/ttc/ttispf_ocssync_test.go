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
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestTTISPF_New(t *testing.T) {
	t.Parallel()
	msg := newttiSPFOCSSync()
	if msg == nil {
		t.Fatal("newttiSPFOCSSync returned nil")
	}
	if msg.GetMsgCode() != TTISPF {
		t.Errorf("GetMsgCode = %v, want %v", msg.GetMsgCode(), TTISPF)
	}

	if msg.(*ttiSPFOCSSync).GetFuncCode() != common.FunctionType(ocssync) {
		t.Errorf("getFuncCode = %v, want %v", msg.(*ttiSPFOCSSync).GetFuncCode(), ocssync)
	}
}

func TestTTISPF_Unmarshal_Success(t *testing.T) {
	t.Parallel()
	// Build payload from captured dump
	buf, err := ExtractBytesFromDump(validttispfUnmarshalDump)
	if err != nil {
		t.Fatalf("ExtractBytesFromDump failed: %v", err)
	}
	if len(buf) == 0 {
		t.Fatal("validttispfUnmarshalDump produced empty payload")
	}

	// Use universal for UB2/UB4 as in other tests to exercise _decodeUniversal path
	data := NewArrayDataBuffer(len(buf) + 64)
	if werr := data.WriteBytesWithContext(context.Background(), buf); werr != nil {
		t.Fatalf("failed to seed buffer: %v", werr)
	}
	engine := NewMarshalEngine(data, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	msg := newttiSPFOCSSync()
	unmarshallable, _ := msg.(common.UnMarshallable)
	if err := unmarshallable.UnMarshalFrom(context.Background(), engine); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}

	props := msg.(*ttiSPFOCSSync).getKeyValueArr()
	// Validate presence of a representative subset of NLS/session keys
	wantKeys := []string{
		authNlsLxlan,         // AMERICAN
		authNlsLxcTerritory,  // AMERICA
		authNlsLxcCurrency,   // $
		authNlsLxcIsoCurr,    // AMERICA
		authNlsLxcNumerics,   // ".,"
		authNlsLxcDateFm,     // DD-MON-RR
		authNlsLxcCalendar,   // GREGORIAN
		authNlsLxcSort,       // BINARY
		sessionNlsLxcCharset, // AL32UTF8
	}
	for _, k := range wantKeys {
		if !props.ContainsKey(k) {
			t.Errorf("missing expected key: %s", k)
		}
	}
}

func TestTTISPF_Unmarshal_Fail_TruncatedPayloads(t *testing.T) {
	t.Parallel()
	full, err := ExtractBytesFromDump(validttispfUnmarshalDump)
	if err != nil {
		t.Fatalf("ExtractBytesFromDump failed: %v", err)
	}
	if len(full) == 0 {
		t.Fatal("validttispfUnmarshalDump produced empty payload")
	}

	cases := []struct {
		name string
		n    int // number of bytes to keep from the front (truncate to n)
	}{
		{"Empty", 0},      // fail reading first UB2 (ReadByte)
		{"SingleByte", 1}, // fail reading UB2 (ReadBytes)
		{"UB2Only", 2},    // fail at next UB1
		{"UB2AndUB1", 3},  // fail starting UB4
		{"ShortUB4", 5},   // fail within UB4 universal decode
		{"EarlyKV", 16},   // fail entering KV array

		// Trailing flag truncations: ensure KV array parses but final UB4 flag read fails
		{"CutBeforeTrailingFlag", len(full) - 4}, // drop entire trailing UB4
		{"Missing3BytesOfFlag", len(full) - 3},   // drop 3 bytes of final UB4
		{"Missing2BytesOfFlag", len(full) - 2},   // drop 2 bytes of final UB4
		{"Missing1ByteOfFlag", len(full) - 1},    // drop 1 byte of final UB4
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := make([]byte, 0)
			if tc.n > 0 && tc.n <= len(full) {
				payload = append(payload, full[:tc.n]...)
			}

			data := NewArrayDataBuffer(len(payload) + 16)
			_ = data.WriteBytesWithContext(context.Background(), payload)
			engine := NewMarshalEngine(data, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

			msg := newttiSPFOCSSync()
			unmarshallable, _ := msg.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(context.Background(), engine)
			if err == nil {
				t.Fatalf("expected error for truncated payload (n=%d), got nil", tc.n)
			}
			// Do not assert exact error text; IO wrapping varies by stage.
		})
	}
}

func TestTTISPF_Unmarshal_Fail_FaultyBuffer(t *testing.T) {
	t.Parallel()
	full, err := ExtractBytesFromDump(validttispfUnmarshalDump)
	if err != nil {
		t.Fatalf("ExtractBytesFromDump failed: %v", err)
	}
	if len(full) == 0 {
		t.Fatal("validttispfUnmarshalDump produced empty payload")
	}

	type tc struct {
		name     string
		failOn   FailOn
		call     int
		wantLike string
	}
	cases := []tc{
		{"FailOnReadByte_First", failOnReadByte, 1, "simulated read error"},
		{"FailOnReadBytes_First", failOnReadBytes, 1, "simulated read error"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mar := createMarshaller(full, c.failOn, c.call)
			msg := newttiSPFOCSSync()
			unmarshallable, _ := msg.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(context.Background(), mar)
			if err == nil {
				t.Fatalf("expected error but got nil")
			}
			if c.wantLike != "" && err != nil && !contains(err.Error(), c.wantLike) {
				t.Fatalf("want error like %q, got %v", c.wantLike, err)
			}
		})
	}
}

func TestGetKeyValueFromKeyword_Timezone_WithRegionID_NameUsed(t *testing.T) {
	t.Parallel()
	// Use a known regid that maps to a zone name: America/Los_Angeles -> 103
	regid := 103

	// Encode regid into v[2]/v[3] per protocol:
	// v[2]: high 7 bits of regid with top bit set to indicate region id present
	// v[3]: low 6 bits of regid placed in bits 2..7 (<< 2)
	v2 := byte(((regid >> 6) & 0x7F) | 0x80) // ensure > ldiRegIDFlag (0x80)
	v3 := byte((regid & 0x3F) << 2)

	// Hour/minute fields when region present are offset by ldiRegIDSet / ldiMaxTimeField,
	// but they are ignored if regionName is resolved. Provide zero offset.
	v4 := byte(ldiRegIDSet)     // hour = 0
	v5 := byte(ldiMaxTimeField) // minute = 0

	payload := common.B1Array{0x00, 0x00, v2, v3, v4, v5}
	kv := keywordValuePair{
		binaryValue: dynamicAllocatedArray{value: payload},
		keyword:     al8kwTimezone,
	}

	sync := newttiSPFOCSSync()
	key, val := getKeyValueFromKeyword(sync.(*ttiSPFOCSSync).GetNlsKeys(), kv)
	if key != SessionTimeZone {
		t.Fatalf("key = %q, want %q", key, SessionTimeZone)
	}
	tz, ok := val.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", val)
	}
	want, ok := getZoneFromID(regid)
	if !ok {
		t.Fatalf("regid %d not found in zone map", regid)
	}
	if tz != want {
		t.Fatalf("timezone = %q, want %q", tz, want)
	}
}

func TestGetKeyValueFromKeyword_Timezone_WithRegionID_FallbackGMT(t *testing.T) {
	t.Parallel()
	// Choose a regid that is not mapped to force GMT fallback
	regid := 8191

	// Encode regid into v[2]/v[3] and set a positive offset +5:30
	v2 := byte(((regid >> 6) & 0x7F) | 0x80) // ensure region id present
	v3 := byte((regid & 0x3F) << 2)
	hour := 5
	minute := 30
	v4 := byte(ldiRegIDSet + byte(hour))       // encoded hour with base offset when region id present
	v5 := byte(ldiMaxTimeField + byte(minute)) // encoded minute with base offset

	payload := common.B1Array{0x00, 0x00, v2, v3, v4, v5}
	kv := keywordValuePair{
		binaryValue: dynamicAllocatedArray{value: payload},
		keyword:     al8kwTimezone,
	}

	sync := newttiSPFOCSSync()
	key, val := getKeyValueFromKeyword(sync.(*ttiSPFOCSSync).GetNlsKeys(), kv)
	if key != SessionTimeZone {
		t.Fatalf("key = %q, want %q", key, SessionTimeZone)
	}
	tz, ok := val.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", val)
	}
	want := "GMT+5:30"
	if tz != want {
		t.Fatalf("timezone = %q, want %q", tz, want)
	}
}

func TestGetKeyValueFromKeyword_Nls_TextValueBranch(t *testing.T) {
	t.Parallel()
	// Use an index < 64 with a non-empty key in nlsKeys: 8 -> AUTH_NLS_LXCDATELANG
	kv := keywordValuePair{
		textValue:   dynamicAllocatedArray{value: common.StringToB1Array("ENGLISH")}, // text present
		binaryValue: dynamicAllocatedArray{},                                         // binary nil to force else branch
		keyword:     common.UB2(8),
	}

	sync := newttiSPFOCSSync()
	key, val := getKeyValueFromKeyword(sync.(*ttiSPFOCSSync).GetNlsKeys(), kv)
	if key != authNlsLxcDateLang {
		t.Fatalf("key = %q, want %q", key, authNlsLxcDateLang)
	}
	d, ok := val.(dynamicAllocatedArray)
	if !ok {
		t.Fatalf("value type = %T, want dynamicAllocatedArray", val)
	}
	got := common.B1ArrayToString(d.value)
	if got != "ENGLISH" {
		t.Fatalf("text value = %q, want %q", got, "ENGLISH")
	}
}

func TestGetKeyValueFromKeyword_PdbElasticPoolLdr_True(t *testing.T) {
	t.Parallel()
	// First 4 bytes interpreted as big-endian int; >0 => "TRUE"
	payload := common.B1Array{0x00, 0x00, 0x00, 0x01}
	kv := keywordValuePair{
		binaryValue: dynamicAllocatedArray{value: payload},
		keyword:     al8kwPdbElasticPoolLdr,
	}
	sync := newttiSPFOCSSync()
	key, val := getKeyValueFromKeyword(sync.(*ttiSPFOCSSync).GetNlsKeys(), kv)
	if key != al8kwPdbElasticPoolLdrStr {
		t.Fatalf("key = %q, want %q", key, al8kwPdbElasticPoolLdrStr)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", val)
	}
	if s != "TRUE" {
		t.Fatalf("value = %q, want %q", s, "TRUE")
	}
}

func TestGetKeyValueFromKeyword_PdbAppRoot_True(t *testing.T) {
	t.Parallel()
	// First 4 bytes interpreted as big-endian int; >0 => "TRUE"
	payload := common.B1Array{0x00, 0x00, 0x00, 0x02}
	kv := keywordValuePair{
		binaryValue: dynamicAllocatedArray{value: payload},
		keyword:     al8kwPdbAppRoot,
	}
	sync := newttiSPFOCSSync()
	key, val := getKeyValueFromKeyword(sync.(*ttiSPFOCSSync).GetNlsKeys(), kv)
	if key != al8kwPdbAppRootStr {
		t.Fatalf("key = %q, want %q", key, al8kwPdbAppRootStr)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", val)
	}
	if s != "TRUE" {
		t.Fatalf("value = %q, want %q", s, "TRUE")
	}
}

// contains is a tiny helper to avoid importing strings in this file.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	// naive search sufficient for tests
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i <= n-m; i++ {
		match := true
		for j := 0; j < m; j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
