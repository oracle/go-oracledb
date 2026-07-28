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
	"database/sql/driver"
	"strconv"
	"time"

	"github.com/oracle/go-driver/driver/common"
)

const (
	_centuryBias               = 100
	_yearBias                  = 100
	_yearsPerCentury           = 100
	_hourBias                  = 1
	_minuteBias                = 1
	_secondBias                = 1
	_dateLength                = 7
	_timestampLength           = 11
	_fractionalByteCount       = 4
	_tzHourBias                = 20
	_tzMinuteBias              = 60
	_timestampSecondsPerHour   = 3600
	_timestampSecondsPerMinute = 60
	_tzTrailingBytes           = 2
	_tzPayloadLength           = _timestampLength + _tzTrailingBytes
	_oracleTZRegionMask        = 0x80
	_oracleTZExtendedBit       = 0x40
	_oracleTZRegionIDMask      = 0x7F
	_oracleTZHourMask          = 0x3F
)

// Oracle DATE is a 7-byte encoding:
// [century+100, yearWithinCentury+100, month(1-12), day(1-31), hour+1, minute+1, second+1]

// encodeDate encodes a time.Time as an Oracle 7-byte DATE payload.
func EncodeDate(v driver.Value) (common.B1Array, error) {
	t := v.(time.Time)
	y := t.Year()
	cent := byte(y/_yearsPerCentury + _centuryBias)
	yr := byte(y%_yearsPerCentury + _yearBias)
	mon := byte(t.Month())               // 1..12
	day := byte(t.Day())                 // 1..31
	hh := byte(t.Hour() + _hourBias)     // 1..24
	mm := byte(t.Minute() + _minuteBias) // 1..60
	ss := byte(t.Second() + _secondBias) // 1..60
	return common.B1Array{cent, yr, mon, day, hh, mm, ss}, nil
}

// decodeDate decodes an Oracle 7-byte DATE payload into a time.Time (in local time zone).
func DecodeDate(b common.B1Array) (time.Time, error) {
	if len(b) != _dateLength {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "DATE", "Decode", common.ReasonInvalidLength, strconv.Itoa(_dateLength))
	}
	cent := int(b[0]) - _centuryBias
	yr := int(b[1]) - _yearBias
	year := cent*_yearsPerCentury + yr
	month := time.Month(b[2])
	day := int(b[3])
	hour := int(b[4]) - _hourBias
	minute := int(b[5]) - _minuteBias
	second := int(b[6]) - _secondBias
	if year <= 0 {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "DATE", "Decode", common.ReasonInvalidValue, year)
	}
	// Build time in local zone; adjust as needed by callers.
	return time.Date(year, month, day, hour, minute, second, 0, time.Local), nil
}

// Oracle TIMESTAMP (without timezone) is an 11-byte encoding:
// First 7 bytes same as DATE, followed by 4 bytes of fractional seconds (nanoseconds) big-endian.

// encodeTimestamp encodes a time.Time as an Oracle 11-byte TIMESTAMP payload.
func EncodeTimestamp(v driver.Value) (common.B1Array, error) {
	// First 7 same as DATE
	t := v.(time.Time)
	date, _ := EncodeDate(t)
	ns := uint32(t.Nanosecond())
	// Fractional seconds: 4 bytes big-endian
	fs := []byte{
		byte((ns >> 24) & 0xFF),
		byte((ns >> 16) & 0xFF),
		byte((ns >> 8) & 0xFF),
		byte(ns & 0xFF),
	}
	out := make(common.B1Array, 0, _timestampLength) // _timestampLength already count len(date) and len(fs)
	out = append(out, date...)
	out = append(out, fs...)

	return out, nil
}

// decodeTimestamp decodes an Oracle 11-byte TIMESTAMP payload into a time.Time (in local time zone).
func DecodeTimestamp(b common.B1Array) (time.Time, error) {
	var (
		dt  time.Time
		err error
		ns  uint32
	)
	length := len(b)
	if length < _dateLength {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP", "Decode", common.ReasonInvalidLength, ">="+strconv.Itoa(_dateLength))
	}

	switch length {
	case _dateLength:
		dt, err = DecodeDate(b)
		ns = 0
	default:
		if length > _timestampLength {
			return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP", "Decode", common.ReasonInvalidLength, "<="+strconv.Itoa(_timestampLength))
		}
		fracLen := length - _dateLength
		dt, err = DecodeDate(b[:_dateLength])
		if err == nil {
			var fracBytes [_fractionalByteCount]byte
			copy(fracBytes[_fractionalByteCount-fracLen:], b[_dateLength:])
			ns = (uint32(fracBytes[0]) << 24) | (uint32(fracBytes[1]) << 16) | (uint32(fracBytes[2]) << 8) | uint32(fracBytes[3])
		}
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second(), int(ns), time.Local), nil
}

// TimestampWithTimeZone provides encode/decode for Oracle TIMESTAMP WITH TIME ZONE.
// Encoding format (13 bytes total):
//   - First 7 bytes: same as DATE
//   - Next 4 bytes: fractional seconds (nanoseconds) big-endian
//   - Next 1 byte: timezone hour offset encoded as (TZH + 20)
//   - Next 1 byte: timezone minute offset encoded as (TZM + 60)
//
// Offsets are derived from time.Time.Zone() (seconds east of UTC).

// Encode converts a time.Time into an Oracle TIMESTAMP WITH TIME ZONE payload.
// Returns a common.B1Array carrying the 13-byte encoding.
func EncodeTimestampWithTimeZone(v driver.Value) (common.B1Array, error) {
	// Base timestamp (DATE + fractional seconds)
	t := v.(time.Time)
	base, _ := EncodeTimestamp(t) // expected 11 bytes
	if len(base) != _timestampLength {
		// Normalize any non-canonical lengths (e.g., future protocol variants) by right-aligning
		// fractional seconds within the 4-byte field.
		padded := make(common.B1Array, _timestampLength)
		copy(padded, base[:_dateLength])
		frac := base[_dateLength:]
		copy(padded[_timestampLength-len(frac):], frac)
		base = padded
	}
	_, off := t.Zone() // seconds east of UTC (may be negative)
	h := off / _timestampSecondsPerHour
	m := (off - (h * _timestampSecondsPerHour)) / _timestampSecondsPerMinute
	if m < -59 || m > 59 {
		// Adjust when integer division leaves hours truncated toward zero for negative offsets.
		if off < 0 {
			h--
			m = (off - (h * _timestampSecondsPerHour)) / _timestampSecondsPerMinute
		} else {
			h++
			m = (off - (h * _timestampSecondsPerHour)) / _timestampSecondsPerMinute
		}
	}
	// Oracle stores TZ as hour+20, minute+60.
	tzh := byte(h + _tzHourBias)
	tzm := byte(m + _tzMinuteBias)
	out := make(common.B1Array, 0, _tzPayloadLength) // // _timestampLength already count len(base) and len(tzh + tzm)
	out = append(out, base...)
	out = append(out, tzh, tzm)

	// we are setting the capability, kpccapRtbTtcTzlt (KPCCAP_RTB_TTC_TZLT).
	// with this capability set, the extended bit is to be sent.
	out[11] |= _oracleTZExtendedBit

	return out, nil
}

// Decode converts a common.B1Array containing a TIMESTAMP WITH TIME ZONE encoding into a time.Time value (expects 13-byte value)
// The time.Time is returned in a FixedZone matching the encoded offset.
func DecodeTimestampWithTimeZone(b common.B1Array) (time.Time, error) {

	if len(b) < _dateLength+2 {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP WITH TIME ZONE", "Decode", common.ReasonInvalidLength, ">="+strconv.Itoa(_dateLength+_tzTrailingBytes))
	}
	// Date/time components
	dt, err := DecodeDate(b[:_dateLength])
	if err != nil {
		return time.Time{}, err
	}
	fracLen := len(b) - (_dateLength + _tzTrailingBytes) // total minus date (7) minus tz hour/min (2)
	if fracLen > _fractionalByteCount {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP WITH TIME ZONE", "Decode", common.ReasonInvalidFraction, fracLen)
	}
	var ns uint32
	if fracLen > 0 {
		var fracBytes [_fractionalByteCount]byte
		copy(fracBytes[_fractionalByteCount-fracLen:], b[_dateLength:_dateLength+fracLen])
		ns = (uint32(fracBytes[0]) << 24) | (uint32(fracBytes[1]) << 16) | (uint32(fracBytes[2]) << 8) | uint32(fracBytes[3])
	}
	// Time zone offset
	hourIdx := len(b) - 2
	hourByte := b[hourIdx]
	minuteByte := b[hourIdx+1]
	if hourByte&_oracleTZRegionMask != 0 {
		// Region ID encoded time zones are not yet supported by this decoder.
		regionID := (int(hourByte&_oracleTZRegionIDMask) << 8) | int(minuteByte)
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP WITH TIME ZONE", "Decode", common.ReasonInvalidValue, regionID)
	}
	// servers sets _oracleTZExtendedBit
	// to signal extended time zone metadata when the capability is kpccapRtbTtcTzlt (KPCCAP_RTB_TTC_TZLT).
	// The capability lets the server take LocalTime into account. the Mask it off before
	// decoding the numeric offset. The lower 6 bits contain the encoded hour value.
	if hourByte&_oracleTZExtendedBit != 0 {
		hourByte &= _oracleTZHourMask
	}
	tzh := int(hourByte) - _tzHourBias
	tzm := int(int8(int(minuteByte) - _tzMinuteBias))
	offset := tzh*_timestampSecondsPerHour + tzm*_timestampSecondsPerMinute
	loc := time.FixedZone("", offset)
	return time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second(), int(ns), loc), nil
}

// TimestampWithLocalTimeZone provides encode/decode for Oracle TIMESTAMP WITH LOCAL TIME ZONE (TSLTZ).
// Wire encoding uses 11 bytes (same as TIMESTAMP); timezone is not stored and the client/session local
// time zone is applied on encode/decode.

// Encode converts a time.Time into an Oracle TIMESTAMP WITH LOCAL TIME ZONE payload.
// The payload is 11 bytes (same as TIMESTAMP) and does not carry an explicit time zone.
func EncodeTimestampWithLocalTimeZone(v driver.Value) (common.B1Array, error) {
	// Normalize the instant to UTC before extracting components so the wire
	// payload carries a canonical representation independent of the client zone.
	t := v.(time.Time)
	return EncodeTimestamp(t.In(time.UTC))
}

// Decode converts a common.B1Array containing a TIMESTAMP WITH LOCAL TIME ZONE encoding into a time.Time value in the local time zone.
func DecodeTimestampWithLocalTimeZone(value common.B1Array, serverTimeZoneOffset int16) (time.Time, error) {

	if len(value) < _dateLength {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP WITH LOCAL TIME ZONE", "Decode", common.ReasonInvalidLength, ">="+strconv.Itoa(_dateLength))
	}

	year := int(value[0]) - _centuryBias
	year = year*_yearsPerCentury + int(value[1]) - _yearBias
	if year < 0 {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP WITH LOCAL TIME ZONE", "Decode", common.ReasonInvalidValue, year)
	}
	month := time.Month(value[2])
	day := int(value[3])
	hour := int(value[4]) - _hourBias
	minute := int(value[5]) - _minuteBias
	second := int(value[6]) - _secondBias

	fracLen := len(value) - _dateLength
	if fracLen > _fractionalByteCount {
		return time.Time{}, common.NewOracleError(common.ConverterExpectedFormat, nil, "TIMESTAMP WITH LOCAL TIME ZONE", "Decode", common.ReasonInvalidFraction, fracLen)
	}

	var ns uint32
	if fracLen > 0 {
		var fracBytes [_fractionalByteCount]byte
		copy(fracBytes[_fractionalByteCount-fracLen:], value[_dateLength:])
		ns = (uint32(fracBytes[0]) << 24) | (uint32(fracBytes[1]) << 16) | (uint32(fracBytes[2]) << 8) | uint32(fracBytes[3])
	}

	// We get the time in server time zone, since we want to create it as UTC, we
	// need to add the server time zone offset (in seconds)
	utc := time.Date(year, month, day, hour, minute, second, int(ns), time.UTC)
	utc = utc.Add(time.Second * time.Duration(-1*serverTimeZoneOffset))
	return utc.In(time.Local), nil
}
