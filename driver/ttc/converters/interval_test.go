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
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

type intervalCase struct {
	name  string
	bytes common.B1Array
	canon string
}

var intervalY2MCases = []intervalCase{
	{"custom 2y3m", common.B1Array{0x80, 0x00, 0x00, 0x02, 0x3F}, "02-03"},
	{"custom negative 1y0m", common.B1Array{0x7F, 0xFF, 0xFF, 0xFF, 0x3C}, "-01-00"},
	{"custom negative -0y5m", common.B1Array{0x80, 0x00, 0x00, 0x00, 0x37}, "-00-05"},
	{"custom negative -0y11m", common.B1Array{0x80, 0x00, 0x00, 0x00, 0x31}, "-00-11"}, // min month
	{"custom negative -1y2m", common.B1Array{0x7F, 0xFF, 0xFF, 0xFF, 0x3A}, "-01-02"},
	{"custom negative -2y11m", common.B1Array{0x7F, 0xFF, 0xFF, 0xFE, 0x31}, "-02-11"},
	{"custom +0y11m", common.B1Array{0x80, 0x00, 0x00, 0x00, 0x47}, "00-11"}, // max month
	{"custom +0y5m", common.B1Array{0x80, 0x00, 0x00, 0x00, 0x41}, "00-05"},
	{"custom 5y6m", common.B1Array{0x80, 0x00, 0x00, 0x05, 0x42}, "05-06"},
	{"custom 123y2m", common.B1Array{0x80, 0x00, 0x00, 0x7B, 0x3E}, "123-02"},
	{"custom 123y", common.B1Array{0x80, 0x00, 0x00, 0x7B, 0x3C}, "123-00"},
	{"custom 300m", common.B1Array{0x80, 0x00, 0x00, 0x19, 0x3C}, "25-00"},
	{"custom 4y", common.B1Array{0x80, 0x00, 0x00, 0x04, 0x3C}, "04-00"},
	{"custom 50m", common.B1Array{0x80, 0x00, 0x00, 0x04, 0x3E}, "04-02"},
	{"custom 123456789y", common.B1Array{0x87, 0x5B, 0xCD, 0x15, 0x3C}, "123456789-00"},
}

// TestIntervalYearToMonth_Decode decodes valid Y2M wire payloads to canonical strings
// and verifies they match expected forms.
func TestIntervalYearToMonth_Decode(t *testing.T) {
	t.Parallel()
	for _, c := range intervalY2MCases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DecodeIntervalYearToMonth(c.bytes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.canon {
				t.Fatalf("mismatch: got %q, want %q", got, c.canon)
			}
		})
	}
}

// TestIntervalYearToMonth_Encode encodes canonical Y2M strings and ensures the
// produced 5-byte wire payload matches expected bytes.
func TestIntervalYearToMonth_Encode(t *testing.T) {
	t.Parallel()
	for _, c := range intervalY2MCases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EncodeIntervalYearToMonth(c.canon)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.bytes) {
				t.Fatalf("mismatch: got %#v, want %#v", got, c.bytes)
			}
		})
	}
}

// TestIntervalYearToMonth_Encode_Errors covers invalid canonical inputs for Y2M encoding.
func TestIntervalYearToMonth_Encode_Errors(t *testing.T) {
	t.Parallel()
	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{"empty", "", "empty input"},
		{"invalid_canonical_dash_only", "-", "invalid-format"},
		{"invalid_canonical_no_sep", "12", "invalid-format"},
		{"years_precision_exceeds", "1234567890-00", "precision-exceeded"},
		{"parse_years", "xx-00", "parse-years"},
		{"parse_months", "01-xx", "parse-months"},
		{"months_out_of_range", "00-12", "month-out-of-range"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := EncodeIntervalYearToMonth(c.in)
			fmt.Printf("Error : %s", err)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error mismatch: got %q, want contains %q", err.Error(), c.want)
			}
		})
	}
}

// TestIntervalYearToMonth_Decode_Errors covers empty input and unsupported-length
// payloads, asserting appropriate error messages.
func TestIntervalYearToMonth_Decode_Errors(t *testing.T) {
	t.Parallel()
	type errTc struct {
		name string
		in   common.B1Array
		want string
	}
	cases := []errTc{
		// unsupported length (e.g., 3 bytes)
		{"unsupported length < 5bytes", common.B1Array{0x00, 0x00, 0x00}, "invalid-length"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := DecodeIntervalYearToMonth(c.in)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error mismatch: got %q, want contains %q", err.Error(), c.want)
			}
		})
	}
}

var intervalD2SCases = []intervalCase{
	{"10 5:30:2.0 ", common.B1Array{0x80, 0x00, 0x00, 0x0A, 0x41, 0x5A, 0x3E, 0x80, 0x00, 0x00, 0x00}, "10 05:30:02.0"},
	{"2 12:15:30.0", common.B1Array{0x80, 0x00, 0x00, 0x02, 0x48, 0x4B, 0x5A, 0x80, 0x00, 0x00, 0x00}, "02 12:15:30.0"},
	{"-1 6:45:0.0", common.B1Array{0x7F, 0xFF, 0xFF, 0xFF, 0x36, 0x0F, 0x3C, 0x80, 0x00, 0x00, 0x00}, "-1 06:45:00.0"},
	{"5 0:0:0.0 ", common.B1Array{0x80, 0x00, 0x00, 0x05, 0x3C, 0x3C, 0x3C, 0x80, 0x00, 0x00, 0x00}, "05 00:00:00.0"},
	{"0 10:15:45.0", common.B1Array{0x80, 0x00, 0x00, 0x00, 0x46, 0x4B, 0x69, 0x80, 0x00, 0x00, 0x00}, "00 10:15:45.0"},
	{"123456789 12:55:17.123456", common.B1Array{0x87, 0x5B, 0xCD, 0x15, 0x48, 0x73, 0x4D, 0x87, 0x5B, 0xCA, 0x00}, "123456789 12:55:17.123456"},
}

// TestIntervalDayToSecond_Decode decodes valid D2S wire payloads into canonical
// strings "[+|-]DD HH:MM:SS[.FFFFFFFFF]" and validates correctness.
func TestIntervalDayToSecond_Decode(t *testing.T) {
	t.Parallel()
	for _, c := range intervalD2SCases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DecodeIntervalDayToSecond(c.bytes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.canon {
				t.Fatalf("mismatch: got %q, want %q", got, c.canon)
			}
		})
	}
}

// TestIntervalDayToSecond_Encode encodes canonical D2S strings and ensures the
// produced 11-byte wire payload matches expected bytes.
func TestIntervalDayToSecond_Encode(t *testing.T) {
	t.Parallel()
	for _, c := range intervalD2SCases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EncodeIntervalDayToSecond(c.canon)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.bytes) {
				t.Fatalf("mismatch: got %#v, want %#v", got, c.bytes)
			}
		})
	}
}

// TestIntervalDayToSecond_Decode_Errors covers invalid wire payloads for D2S decoding.
func TestIntervalDayToSecond_Decode_Errors(t *testing.T) {
	t.Parallel()
	makeBase := func() common.B1Array {
		b := make([]byte, _intervalDSEncodingLen)
		// day = 0
		binary.BigEndian.PutUint32(b[0:4], uint32(int32(0))^_yearXorBias)
		// hour/minute/second = 0 (monthBias)
		b[4] = _monthBias
		b[5] = _monthBias
		b[6] = _monthBias
		// fractional ns = 0
		binary.BigEndian.PutUint32(b[7:11], uint32(int32(0))^_yearXorBias)
		return b
	}
	type tc struct {
		name string
		in   common.B1Array
		want string
	}
	cases := []tc{
		{"unsupported length", common.B1Array{0, 0, 0, 0, 0}, "invalid-length"},
		{"invalid hour bias", func() common.B1Array { b := makeBase(); b[4] = 0x00; return b }(), "invalid-hour-bias"},
		{"invalid minute bias", func() common.B1Array { b := makeBase(); b[5] = 0x00; return b }(), "invalid-minute-bias"},
		{"invalid second bias", func() common.B1Array { b := makeBase(); b[6] = 0x00; return b }(), "invalid-second-bias"},
		{"invalid fractional nanoseconds", func() common.B1Array {
			b := makeBase()
			raw := uint32(int32(1000000000)) ^ _yearXorBias
			binary.BigEndian.PutUint32(b[7:11], raw)
			return b
		}(), "invalid-fraction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := DecodeIntervalDayToSecond(c.in)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error mismatch: got %q, want contains %q", err.Error(), c.want)
			}
		})
	}
}

// TestIntervalDayToSecond_Encode_Errors covers invalid canonical inputs for D2S encoding.
func TestIntervalDayToSecond_Encode_Errors(t *testing.T) {
	t.Parallel()
	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{"empty", "", "empty input"},
		{"invalid canonical no space", "123", "invalid-format"},
		{"invalid canonical dash only", "-", "invalid-format"},
		{"parse day", "xx 10:10:10", "parse-day"},
		{"invalid time token", "1 10:20", "invalid-format"},
		{"parse hour", "1 xx:00:00", "parse-hour"},
		{"parse minute", "1 00:xx:00", "parse-minute"},
		{"parse second", "1 00:00:xx", "parse-second"},
		{"hour out of range", "1 24:00:00", "hour-out-of-range"},
		{"minute out of range", "1 00:60:00", "minute-out-of-range"},
		{"second out of range", "1 00:00:60", "second-out-of-range"},
		{"fractional precision exceeds", "1 00:00:00.1234567890", "precision-exceeded"},
		{"parse fractional seconds", "1 00:00:00.abc", "parse-fraction"},
		{"days out of int32 range", "2147483648 00:00:00", "days-overflow"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := EncodeIntervalDayToSecond(c.in)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error mismatch: got %q, want contains %q", err.Error(), c.want)
			}
		})
	}
}

// Additional whitespace robustness tests

// TestIntervalYearToMonth_Encode_Whitespace ensures Y2M encoding
// fails gracefully on whitespace-only inputs after trimming.
func TestIntervalYearToMonth_Encode_Whitespace(t *testing.T) {
	t.Parallel()
	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{"space only", " ", "empty input"},
		{"tab only", "\t", "empty input"},
		{"newline only", "\n", "empty input"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := EncodeIntervalYearToMonth(c.in)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error mismatch: got %q, want contains %q", err.Error(), c.want)
			}
		})
	}
}

// TestIntervalYearToMonth_Encode_TrimWhitespace_Success verifies that
// valid Y2M values surrounded by whitespace are accepted and normalized.
func TestIntervalYearToMonth_Encode_TrimWhitespace_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		in    string
		canon string
	}{
		{"leading and trailing spaces", "   10-10   ", "10-10"},
		{"trailing newline", "10-10\n", "10-10"},
		{"sign with whitespace", "  -01-05  ", "-01-05"},
		{"plus sign with whitespace", " +12-00 ", "12-00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			enc, err := EncodeIntervalYearToMonth(c.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := DecodeIntervalYearToMonth(enc)
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if got != c.canon {
				t.Fatalf("mismatch: got %q, want %q", got, c.canon)
			}
		})
	}
}

// TestIntervalDayToSecond_Encode_Errors_Whitespace covers D2S whitespace-only
// inputs and time-only strings with extra spaces/newlines.
func TestIntervalDayToSecond_Encode_Errors_Whitespace(t *testing.T) {
	t.Parallel()
	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{"space only", " ", "empty input"},
		{"tab only", "\t", "empty input"},
		{"newline only", "\n", "empty input"},
		// Missing day part even after trimming
		{"only time with spaces", " 10:10:10   ", "invalid-format"},
		{"only time trailing newline", "10:10:10\n", "invalid-format"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := EncodeIntervalDayToSecond(c.in)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error mismatch: got %q, want contains %q", err.Error(), c.want)
			}
		})
	}
}

// TestIntervalDayToSecond_Encode_TrimWhitespace_Success verifies that
// valid D2S values surrounded by whitespace are accepted and normalized.
func TestIntervalDayToSecond_Encode_TrimWhitespace_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		in    string
		canon string
	}{
		{"leading and trailing spaces", "   1 10:10:10   ", "01 10:10:10.0"},
		{"sign with whitespace", "\n-2 00:00:00.5\t", "-2 00:00:00.5"},
		{"plus sign with whitespace", " +3 09:08:07 ", "03 09:08:07.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			enc, err := EncodeIntervalDayToSecond(c.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := DecodeIntervalDayToSecond(enc)
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if got != c.canon {
				t.Fatalf("mismatch: got %q, want %q", got, c.canon)
			}
		})
	}
}
