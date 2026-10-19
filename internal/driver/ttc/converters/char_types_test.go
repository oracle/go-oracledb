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

package converters

import (
	"errors"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Overview
// This test file validates string-type converters in a consistent, type-first flow.
// Shared test data and helpers live in testdata_string_types_test.go to avoid duplication.
// Sections below are ordered by Oracle types: VARCHAR2, CHAR, NVARCHAR2, and NCHAR.
// Each section contains encode tests, decode tests, UTF-8/AL16UTF16 behavior when applicable,
// and round-trip checks where meaningful.

// -----------------------------------------------------------------------------
// VARCHAR2
// -----------------------------------------------------------------------------

// Test roundtrip: Encode then Decode returns original string for a variety of inputs
// including empty, ASCII, multi-byte, embedded NUL, and repeated characters.
func TestVarchar_EncodeDecode_Roundtrip(t *testing.T) {
	t.Parallel()
	tests := []string{
		"",
		"hello",
		"ä¸–ç•Œ",
		"with \x00 byte",
		strings.Repeat("A", 10),
	}
	for _, input := range tests {
		enc, _ := EncodeVarchar(input)
		got, err := DecodeVarchar(enc)
		if err != nil {
			t.Fatalf("decode error: %v (input: %q)", err, input)
		}
		if got != input {
			t.Fatalf("roundtrip failed: got %q, want %q", got, input)
		}
	}
}

// Encode table-driven test: verify raw bytes match expected UTF-8 payloads.
func TestVarchar_Encode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range varcharRows {
		got, _ := EncodeVarchar(r.val)
		assertEqBytes(t, got, r.expEnc)
	}
}

// Decode table-driven test: verify UTF-8 bytes decode into the expected values.
func TestVarchar_Decode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range varcharRows {
		got, err := DecodeVarchar(r.expEnc)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if got != r.val {
			t.Fatalf("varchar decode mismatch: got %q want %q", got, r.val)
		}
	}
}

// Encode output spot checks with simple inputs and embedded NULs.
func TestVarchar_Encode_Output(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s   string
		exp common.B1Array
	}{
		{"foo", common.B1Array{'f', 'o', 'o'}},
		{"", common.B1Array{}},
		{"abc\x00def", common.B1Array{'a', 'b', 'c', 0, 'd', 'e', 'f'}},
	}
	for _, c := range cases {
		got, _ := EncodeVarchar(c.s)
		for i := range got {
			if got[i] != c.exp[i] {
				t.Fatalf("byte mismatch for %q at %d: got %v want %v", c.s, i, got, c.exp)
			}
		}
	}
}

// UTF-8 compliance: confirm known code points are encoded and decoded correctly.
func TestVarchar_AL32UTF8_Encode_Output(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s   string
		exp common.B1Array
	}{
		{"€", common.B1Array{0xE2, 0x82, 0xAC}},       // U+20AC EURO SIGN
		{"é", common.B1Array{0xC3, 0xA9}},             // U+00E9 LATIN SMALL LETTER E WITH ACUTE
		{"𐐷", common.B1Array{0xF0, 0x90, 0x90, 0xB7}}, // U+10437 DESERET CAPITAL LETTER H
		{"ا", common.B1Array{0xD8, 0xA7}},             // U+0627 ARABIC LETTER ALEF
	}
	for _, c := range cases {
		got, _ := EncodeVarchar(c.s)
		if len(got) != len(c.exp) {
			t.Fatalf("length mismatch for %q: got %d want %d", c.s, len(got), len(c.exp))
		}
		for i := range got {
			if got[i] != c.exp[i] {
				t.Fatalf("byte mismatch for %q at %d: got % X want % X", c.s, i, []byte(got), []byte(c.exp))
			}
		}
		dec, err := DecodeVarchar(c.exp)
		if err != nil {
			t.Fatalf("decode error for %q: %v", c.s, err)
		}
		if dec != c.s {
			t.Fatalf("decode mismatch for %q: got %q", c.s, dec)
		}
	}
}

// -----------------------------------------------------------------------------
// CHAR
// -----------------------------------------------------------------------------

// Encode table-driven test: verify raw bytes for CHAR values (no padding at encode side).
func TestChar_Encode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range charRows {
		got, _ := EncodeChar(r.val)
		assertEqBytes(t, got, r.expEnc)
	}
}

// Decode with trimming: user-facing values should remove trailing space padding.
func TestChar_Decode_Padded_Table(t *testing.T) {
	t.Parallel()
	for _, r := range charRows {
		got, err := DecodeChar(r.expDec)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if strings.TrimRight(got, " ") != r.val {
			t.Fatalf("decode mismatch: got %q want %q", got, r.val)
		}
	}
}

// Decode without trimming: verify padding is preserved when requested by the caller.
func TestChar_Decode_NoTrim_PreservesPadding(t *testing.T) {
	t.Parallel()
	for _, r := range charRows {
		got, err := DecodeChar(r.expDec)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if got != string(r.expDec) {
			t.Fatalf("no-trim decode should preserve padding: got %q want %q", got, string(r.expDec))
		}
	}
}

// UTF-8 compliance for CHAR: verify encoding/decoding of representative code points.
func TestChar_AL32UTF8_Encode_Output(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s   string
		exp common.B1Array
	}{
		{"€", common.B1Array{0xE2, 0x82, 0xAC}},       // U+20AC
		{"é", common.B1Array{0xC3, 0xA9}},             // U+00E9
		{"𐐷", common.B1Array{0xF0, 0x90, 0x90, 0xB7}}, // U+10437
		{"ا", common.B1Array{0xD8, 0xA7}},             // U+0627
	}
	for _, c := range cases {
		got, _ := EncodeChar(c.s)
		if len(got) != len(c.exp) {
			t.Fatalf("length mismatch for %q: got %d want %d", c.s, len(got), len(c.exp))
		}
		for i := range got {
			if got[i] != c.exp[i] {
				t.Fatalf("byte mismatch for %q at %d: got % X want % X", c.s, i, []byte(got), []byte(c.exp))
			}
		}
		dec, err := DecodeChar(c.exp)
		if err != nil {
			t.Fatalf("decode error for %q: %v", c.s, err)
		}
		if dec != c.s {
			t.Fatalf("decode mismatch for %q: got %q", c.s, dec)
		}
	}
}

// Decode NullString helper: ensure proper trimming and Valid/String fields behavior.
func TestChar_DecodeNullString(t *testing.T) {
	t.Parallel()
	// nil -> empty string, Valid=true
	ns, err := DecodeCharNullString(nil)
	if err != nil {
		t.Fatalf("unexpected error for nil: %v", err)
	}
	if !ns.Valid {
		t.Fatalf("expected Valid=true for nil input")
	}
	if ns.String != "" {
		t.Fatalf("expected empty string for nil input, got %q", ns.String)
	}

	// empty slice -> empty string, Valid=true
	ns, err = DecodeCharNullString(common.B1Array{})
	if err != nil {
		t.Fatalf("unexpected error for empty slice: %v", err)
	}
	if !ns.Valid {
		t.Fatalf("expected Valid=true for empty slice input")
	}
	if ns.String != "" {
		t.Fatalf("expected empty string for empty slice input, got %q", ns.String)
	}

	// padded value -> trimmed, Valid=true
	payload := common.B1Array{'A', '1', ' ', ' ', ' '}
	ns, err = DecodeCharNullString(payload)
	if err != nil {
		t.Fatalf("unexpected error for padded payload: %v", err)
	}
	if !ns.Valid || strings.TrimRight(ns.String, " ") != "A1" {
		t.Fatalf("expected Valid=true and String=\"A1\"; got Valid=%v String=%q", ns.Valid, ns.String)
	}

	// no padding -> same value, Valid=true
	payload2 := common.B1Array{'X'}
	ns, err = DecodeCharNullString(payload2)
	if err != nil {
		t.Fatalf("unexpected error for single-char payload: %v", err)
	}
	if !ns.Valid || ns.String != "X" {
		t.Fatalf("expected Valid=true and String=\"X\"; got Valid=%v String=%q", ns.Valid, ns.String)
	}
}

// -----------------------------------------------------------------------------
// NVARCHAR2
// -----------------------------------------------------------------------------

// Encode table-driven test for NVARCHAR2 (UTF-8 input -> AL16UTF16 on-the-wire as needed).
func TestNVarchar2_Encode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range nvarcharRows {
		got, _ := EncodeNVarchar2(r.val)
		assertEqBytes(t, got, r.expEnc)
	}
}

// Decode table-driven test for NVARCHAR2 when payload is AL16UTF16.
func TestNVarchar2_Decode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range nvarcharRows {
		got, err := DecodeNVarchar2AL16UTF16(r.expDec)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if got != r.val {
			t.Fatalf("nvarchar2 decode mismatch: got %s want %s", got, r.val)
		}
	}
}

// Decode table-driven test for NVARCHAR2 when payload is UTF-8.
func TestNVarchar2UTF8_Decode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range nvarcharRows {
		got, err := DecodeNVarchar2UTF8(r.expEnc)
		if err != nil {
			t.Fatalf("DecodeNVarchar2UTF8 error: %v", err)
		}
		if got != r.val {
			t.Fatalf("nvarchar2 UTF-8 decode mismatch: got %q want %q", got, r.val)
		}
	}
}

// -----------------------------------------------------------------------------
// NCHAR
// -----------------------------------------------------------------------------

// Encode table-driven test for NCHAR.
func TestNChar_Encode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range ncharRows {
		got, _ := EncodeNChar(r.val)
		assertEqBytes(t, got, r.expEnc)
	}
}

// Decode table-driven test for NCHAR when payload is AL16UTF16 (padded on the wire).
func TestNChar_Decode_Table(t *testing.T) {
	t.Parallel()
	for _, r := range ncharRows {
		got, err := DecodeNCharAL16UTF16(r.expDecPadded)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if strings.TrimRight(got, " ") != r.val {
			t.Fatalf("nchar decode mismatch: got %q want %q", got, r.val)
		}
	}
}

// Decode table-driven test for NCHAR when payload is UTF-8 with trailing space padding.
func TestNCharUTF8_Decode_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		val     string
		payload common.B1Array // UTF-8 with trailing space padding
	}{
		{"US", common.B1Array{'U', 'S', ' '}},
		{"IN", common.B1Array{'I', 'N', ' '}},
		{"CN", common.B1Array{'C', 'N', ' '}},
	}
	for _, c := range cases {
		got, err := DecodeNCharUTF8(c.payload)
		if err != nil {
			t.Fatalf("DecodeNCharUTF8 error: %v", err)
		}
		if strings.TrimRight(got, " ") != c.val {
			t.Fatalf("nchar UTF-8 decode mismatch: got %q want %q", got, c.val)
		}
	}
}

// -----------------------------------------------------------------------------
// Error cases
// -----------------------------------------------------------------------------

// Odd-length AL16UTF16 payload should return an OracleError with ConverterExpectedFormat.
func TestNCharAL16UTF16_OddLength_ReturnsOracleError(t *testing.T) {
	t.Parallel()
	oddPayloads := []common.B1Array{
		{0x00},
		{0x00, 0x01, 0x02},
	}
	for _, p := range oddPayloads {
		_, err := DecodeNCharAL16UTF16(p)
		if err == nil {
			t.Fatalf("expected error for odd-length payload % X, got nil", []byte(p))
		}
		var sqle oracleErrors.SQLError
		if !errors.As(err, &sqle) {
			t.Fatalf("expected oracleErrors.SQLError, got %T: %v", err, err)
		}
		if sqle.ErrorCode() != string(oracleErrors.ConverterExpectedFormat) {
			t.Fatalf("unexpected error code %s, want %s", sqle.ErrorCode(), oracleErrors.ConverterExpectedFormat)
		}
		msg := err.Error()
		if !strings.Contains(msg, string(common.ReasonInvalidLength)) || !strings.Contains(msg, "expected=even length") {
			t.Fatalf("error message should mention invalid-length and expected even length, got: %q", msg)
		}
	}
}

// Odd-length UTF-16BE buffer to low-level decoder should also error similarly.
func TestDecodeUTF16BEToString_OddLength_ReturnsOracleError(t *testing.T) {
	t.Parallel()
	p := common.B1Array{0xAB}
	_, err := DecodeUTF16BEToString(p)
	if err == nil {
		t.Fatal("expected error for odd-length input, got nil")
	}
	var sqle oracleErrors.SQLError
	if !errors.As(err, &sqle) {
		t.Fatalf("expected oracleErrors.SQLError, got %T: %v", err, err)
	}
	if sqle.ErrorCode() != string(oracleErrors.ConverterExpectedFormat) {
		t.Fatalf("unexpected error code %s, want %s", sqle.ErrorCode(), oracleErrors.ConverterExpectedFormat)
	}
	if msg := err.Error(); !strings.Contains(msg, string(common.ReasonInvalidLength)) || !strings.Contains(msg, "expected=even length") {
		t.Fatalf("error message should mention invalid-length and expected even length, got: %q", msg)
	}
}
