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
	"bytes"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// signedDate is the first 7 bytes from the reference payload used in the
// formerly-commented table. It represents 2024-01-15 00:00:00 in the local
// time zone when encoded as an Oracle DATE.
var signedDate = []int{120, 124, 1, 15, 1, 1, 1}

// signedDateEndOfDay represents 1999-12-31 23:59:59 when encoded as an Oracle DATE payload.
var signedDateEndOfDay = []int{119, 199, 12, 31, 24, 60, 60}

// signedDateYearOne represents 0001-01-01 00:00:00 when encoded as an Oracle DATE payload.
var signedDateYearOne = []int{100, 101, 1, 1, 1, 1, 1}

// signedTimestampFull corresponds to the reference TIMESTAMP payload (11 bytes) used in
// the original commented tests. It represents 2024-01-15 10:20:30.123456000 in the local
// time zone.
var signedTimestampFull = []int{120, 124, 1, 15, 11, 21, 31, 7, 91, 202, 0}

// signedTimestampNoFrac encodes the same date/time components but without fractional seconds
// (payload length 7) to validate decode support for short TIMESTAMP encodings.
var signedTimestampNoFrac = []int{120, 124, 1, 15, 11, 21, 31}

// signedTimestampShortFrac encodes fractional seconds with only two trailing bytes to confirm
// decodeTimestamp correctly right-aligns short fractional payloads.
var signedTimestampShortFrac = []int{120, 124, 1, 15, 11, 21, 31, 0, 1}

// signedTimestampZeroFracExp is the expected 11-byte payload produced by Encode when the input
// time carries zero fractional seconds (all fractional bytes are zeroed).
var signedTimestampZeroFracExp = []int{120, 124, 1, 15, 11, 21, 31, 0, 0, 0, 0}

// signedTimestampMaxFracExp is the expected payload for a time with nanoseconds set to 999,999,999.
var signedTimestampMaxFracExp = []int{120, 124, 1, 15, 11, 21, 31, 59, 154, 201, 255}

// signedTimestampWithTZPositive encodes 2024-01-15 10:20:30.123456000 +05:30 with TZ bytes appended.
var signedTimestampWithTZPositive = []int{120, 124, 1, 15, 11, 21, 31, 7, 91, 202, 0, 89, 90}

// signedTimestampWithTZNegative encodes 1999-12-31 23:59:59.000000000 -07:45 with TZ bytes appended.
var signedTimestampWithTZNegative = []int{119, 199, 12, 31, 24, 60, 60, 0, 0, 0, 0, 77, 15}

// signedTimestampWithTZUTCShortFracDecode encodes 2024-01-15 10:20:30.000000001 +00:00 with only two fractional bytes.
var signedTimestampWithTZUTCShortFracDecode = []int{120, 124, 1, 15, 11, 21, 31, 0, 1, 20, 60}

// signedTimestampWithTZUTCShortFracEncode is the canonical payload with all four fractional bytes present.
var signedTimestampWithTZUTCShortFracEncode = []int{120, 124, 1, 15, 11, 21, 31, 0, 0, 0, 1, 84, 60}

// b1 converts a slice of signed integers to a common.B1Array, mirroring the
// helper that existed in the reference test block.
func b1(from []int) common.B1Array {
	out := make([]byte, len(from))
	for i, v := range from {
		if v < 0 {
			v += 256
		}
		out[i] = byte(v & 0xFF)
	}
	return common.B1Array(out)
}

// assertLocalDateTime verifies that the decoded value matches the expected
// date/time components (including the local time zone pointer).
func assertLocalDateTime(t *testing.T, got time.Time, want time.Time) {
	if got.Year() != want.Year() || got.Month() != want.Month() || got.Day() != want.Day() ||
		got.Hour() != want.Hour() || got.Minute() != want.Minute() || got.Second() != want.Second() || got.Nanosecond() != want.Nanosecond() {
		t.Fatalf("datetime mismatch: got %v, want %v", got, want)
	}
	if got.Location() != want.Location() {
		t.Fatalf("location mismatch: got %v, want %v", got.Location(), want.Location())
	}
}

// assertDateTimeWithOffset verifies date/time fields alongside the UTC offset in seconds.
func assertDateTimeWithOffset(t *testing.T, got time.Time, want time.Time, wantOffset int) {
	if got.Year() != want.Year() || got.Month() != want.Month() || got.Day() != want.Day() ||
		got.Hour() != want.Hour() || got.Minute() != want.Minute() || got.Second() != want.Second() || got.Nanosecond() != want.Nanosecond() {
		t.Fatalf("datetime mismatch: got %v, want %v", got, want)
	}
	_, gotOffset := got.Zone()
	if gotOffset != wantOffset {
		t.Fatalf("offset mismatch: got %d, want %d", gotOffset, wantOffset)
	}
}

// TestDateDecode exercises Date.Decode with a range of representative payloads
// including mid-range, end-of-day, and lower-bound calendar values.
func TestDateDecode(t *testing.T) {
	t.Parallel()

	/*
		Test cases:

		+--------------------+--------------------------------+---------------------------------------+
		| Name               | Payload (signed ints)          | Expected time                         |
		+--------------------+--------------------------------+---------------------------------------+
		| mid-range reference| 120 124 1 15 1 1 1            | 2024-01-15 00:00:00 Local              |
		| end-of-day         | 119 199 12 31 24 60 60        | 1999-12-31 23:59:59 Local              |
		| year-one           | 100 101 1 1 1 1 1             | 0001-01-01 00:00:00 Local              |
		+--------------------+--------------------------------+---------------------------------------+
	*/

	tests := []struct {
		name    string
		payload []int
		want    time.Time
	}{
		{
			name:    "mid-range reference",
			payload: signedDate,
			want:    time.Date(2024, time.January, 15, 0, 0, 0, 0, time.Local),
		},
		{
			name:    "end-of-day",
			payload: signedDateEndOfDay,
			want:    time.Date(1999, time.December, 31, 23, 59, 59, 0, time.Local),
		},
		{
			name:    "year-one",
			payload: signedDateYearOne,
			want:    time.Date(1, time.January, 1, 0, 0, 0, 0, time.Local),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeDate(b1(tt.payload))
			if err != nil {
				t.Fatalf("DATE decode error: %v", err)
			}
			assertLocalDateTime(t, got, tt.want)
		})
	}
}

// TestDateEncode validates that Date.Encode produces the expected Oracle DATE
// payloads for representative inputs, including edge cases at the supported
// calendar bounds.
func TestDateEncode(t *testing.T) {
	t.Parallel()

	/*
		Test cases:

		+-----------------------------+--------------------------------+---------------------------------------+
		| Name                        | Input time                     | Expected payload (signed ints)        |
		+-----------------------------+--------------------------------+---------------------------------------+
		| mid-range reference         | 2024-01-15 00:00:00 Local      | 120 124 1 15 1 1 1                    |
		| end-of-day                  | 1999-12-31 23:59:59 Local      | 119 199 12 31 24 60 60                |
		| year-one                    | 0001-01-01 00:00:00 Local      | 100 101 1 1 1 1 1                     |
		| non-local location preserved| 2024-01-15 10:20:30 +05:30     | 120 124 1 15 11 21 31                 |
		+-----------------------------+--------------------------------+---------------------------------------+
	*/

	tests := []struct {
		name string
		when time.Time
		want []int
	}{
		{
			name: "mid-range reference",
			when: time.Date(2024, time.January, 15, 0, 0, 0, 0, time.Local),
			want: signedDate,
		},
		{
			name: "end-of-day",
			when: time.Date(1999, time.December, 31, 23, 59, 59, 0, time.Local),
			want: signedDateEndOfDay,
		},
		{
			name: "year-one",
			when: time.Date(1, time.January, 1, 0, 0, 0, 0, time.Local),
			want: signedDateYearOne,
		},
		{
			name: "non-local location preserved",
			when: time.Date(2024, time.January, 15, 10, 20, 30, 0, time.FixedZone("UTC+0530", 5*3600+30*60)),
			want: []int{120, 124, 1, 15, 11, 21, 31},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := EncodeDate(tt.when)
			want := b1(tt.want)
			if !bytes.Equal([]byte(got), []byte(want)) {
				t.Fatalf("DATE encode mismatch for %s:\n got %v\nwant %v", tt.name, got, want)
			}
		})
	}
}

// TestTimestampDecode verifies that TIMESTAMP payloads with varying fractional encodings (11-byte,
// 7-byte, and short fractional byte counts) produce the expected local times when decoded.
func TestTimestampDecode(t *testing.T) {
	t.Parallel()

	/*
		Test cases:

		+--------------------------+--------------------------------+-------------------------------------------+
		| Name                     | Payload (signed ints)          | Expected time                              |
		+--------------------------+--------------------------------+-------------------------------------------+
		| full payload             | 120 124 1 15 11 21 31 7 91 202 0 | 2024-01-15 10:20:30.123456000 Local       |
		| no fractional (7 byte)   | 120 124 1 15 11 21 31          | 2024-01-15 10:20:30 Local                  |
		| short fractional (2 byte)| 120 124 1 15 11 21 31 0 1      | 2024-01-15 10:20:30.000000001 Local        |
		+--------------------------+--------------------------------+-------------------------------------------+
	*/

	tests := []struct {
		name    string
		payload []int
		want    time.Time
	}{
		{
			name:    "full payload",
			payload: signedTimestampFull,
			want:    time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.Local),
		},
		{
			name:    "no fractional (7 byte)",
			payload: signedTimestampNoFrac,
			want:    time.Date(2024, time.January, 15, 10, 20, 30, 0, time.Local),
		},
		{
			name:    "short fractional (2 byte)",
			payload: signedTimestampShortFrac,
			want:    time.Date(2024, time.January, 15, 10, 20, 30, 1, time.Local),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeTimestamp(b1(tt.payload))
			if err != nil {
				t.Fatalf("TIMESTAMP decode error: %v", err)
			}
			assertLocalDateTime(t, got, tt.want)
		})
	}
}

// TestTimestampEncode ensures that Timestamp.Encode emits the correct 11-byte payload for varying
// fractional second inputs, including zero and maximum nanoseconds.
func TestTimestampEncode(t *testing.T) {
	t.Parallel()

	/*
		Test cases:

		+---------------------+-------------------------------------------+------------------------------------------+
		| Name                | Input time                                 | Expected payload (signed ints)           |
		+---------------------+-------------------------------------------+------------------------------------------+
		| full fractional     | 2024-01-15 10:20:30.123456000 Local       | 120 124 1 15 11 21 31 7 91 202 0         |
		| zero fractional     | 2024-01-15 10:20:30.000000000 Local       | 120 124 1 15 11 21 31 0 0 0 0            |
		| max fractional      | 2024-01-15 10:20:30.999999999 Local       | 120 124 1 15 11 21 31 59 154 201 255     |
		+---------------------+-------------------------------------------+------------------------------------------+
	*/

	tests := []struct {
		name string
		when time.Time
		want []int
	}{
		{
			name: "full fractional",
			when: time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.Local),
			want: signedTimestampFull,
		},
		{
			name: "zero fractional",
			when: time.Date(2024, time.January, 15, 10, 20, 30, 0, time.Local),
			want: signedTimestampZeroFracExp,
		},
		{
			name: "max fractional",
			when: time.Date(2024, time.January, 15, 10, 20, 30, 999999999, time.Local),
			want: signedTimestampMaxFracExp,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := EncodeTimestamp(tt.when)
			want := b1(tt.want)
			if !bytes.Equal([]byte(got), []byte(want)) {
				t.Fatalf("TIMESTAMP encode mismatch for %s:\n got %v\nwant %v", tt.name, got, want)
			}
		})
	}
}

// TestTimestampWithTimeZoneDecode verifies TIMESTAMP WITH TIME ZONE payloads with varying
// offsets and fractional byte lengths are decoded with the correct instant and UTC offset.
func TestTimestampWithTimeZoneDecode(t *testing.T) {
	t.Parallel()

	locPositive := time.FixedZone("", 5*_timestampSecondsPerHour+30*_timestampSecondsPerMinute)
	locNegative := time.FixedZone("", -(7*_timestampSecondsPerHour + 45*_timestampSecondsPerMinute))
	locUTC := time.FixedZone("", 0)

	tests := []struct {
		name    string
		payload []int
		want    time.Time
		wantOff int
	}{
		{
			name:    "positive offset full fractional",
			payload: signedTimestampWithTZPositive,
			want:    time.Date(2024, time.January, 15, 10, 20, 30, 123456000, locPositive),
			wantOff: 5*_timestampSecondsPerHour + 30*_timestampSecondsPerMinute,
		},
		{
			name:    "negative offset zero fractional",
			payload: signedTimestampWithTZNegative,
			want:    time.Date(1999, time.December, 31, 23, 59, 59, 0, locNegative),
			wantOff: -(7*_timestampSecondsPerHour + 45*_timestampSecondsPerMinute),
		},
		{
			name:    "utc short fractional",
			payload: signedTimestampWithTZUTCShortFracDecode,
			want:    time.Date(2024, time.January, 15, 10, 20, 30, 1, locUTC),
			wantOff: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeTimestampWithTimeZone(b1(tt.payload))
			if err != nil {
				t.Fatalf("TIMESTAMP WITH TIME ZONE decode error: %v", err)
			}
			assertDateTimeWithOffset(t, got, tt.want, tt.wantOff)
		})
	}
}

// TestTimestampWithTimeZoneEncode ensures Encode emits the correct payload for a range of
// offsets and fractional second representations.
func TestTimestampWithTimeZoneEncode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		when time.Time
		want []int
	}{
		{
			name: "positive offset full fractional",
			when: time.Date(2024, time.January, 15, 10, 20, 30, 123456000, time.FixedZone("", 5*_timestampSecondsPerHour+30*_timestampSecondsPerMinute)),
			want: signedTimestampWithTZPositive,
		},
		{
			name: "negative offset zero fractional",
			when: time.Date(1999, time.December, 31, 23, 59, 59, 0, time.FixedZone("", -(7*_timestampSecondsPerHour+45*_timestampSecondsPerMinute))),
			want: signedTimestampWithTZNegative,
		},
		{
			name: "utc short fractional",
			when: time.Date(2024, time.January, 15, 10, 20, 30, 1, time.FixedZone("", 0)),
			want: signedTimestampWithTZUTCShortFracEncode,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := EncodeTimestampWithTimeZone(tt.when)
			want := b1(tt.want)
			if !bytes.Equal([]byte(got), []byte(want)) {
				t.Fatalf("TIMESTAMP WITH TIME ZONE encode mismatch for %s:\n got %v\nwant %v", tt.name, got, want)
			}
		})
	}
}

// TestTimestampWithLocalTimeZoneDecode ensures the decoder converts UTC encoded
// TSLTZ payloads into the client local zone without altering the local clock
// components.
func TestTimestampWithLocalTimeZoneDecode(t *testing.T) {
	t.Parallel()

	utcInstant := time.Date(2024, time.January, 15, 4, 50, 30, 123456000, time.UTC)
	encoded, err := EncodeTimestamp(utcInstant)
	if err != nil {
		t.Fatalf("TIMESTAMP encode error: %v", err)
	}

	got, err := DecodeTimestampWithLocalTimeZone(encoded, 0)
	if err != nil {
		t.Fatalf("TIMESTAMP WITH LOCAL TIME ZONE decode error: %v", err)
	}
	wanted := utcInstant.In(time.Local)
	assertLocalDateTime(t, got, wanted)
}

// TestTimestampWithLocalTimeZoneEncode validates that encoding normalizes the
// instant to UTC on the wire.
func TestTimestampWithLocalTimeZoneEncode(t *testing.T) {
	t.Parallel()

	utcInstant := time.Date(2024, time.January, 15, 4, 50, 30, 123456000, time.UTC)
	when := utcInstant.In(time.Local)
	got, _ := EncodeTimestampWithLocalTimeZone(when)
	want, _ := EncodeTimestamp(utcInstant)
	if !bytes.Equal([]byte(got), []byte(want)) {
		t.Fatalf("TIMESTAMP WITH LOCAL TIME ZONE encode mismatch:\n got %v\nwant %v", got, want)
	}
}
