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
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// _monthBias is the bias applied to the month byte on the wire (60 decimal).
	_monthBias byte = 0x3C

	// _yearXorBias is the XOR bias applied to the 32-bit signed years field.
	_yearXorBias uint32 = 0x80000000

	// intervalYTMEncodingLen is the expected wire size for INTERVAL YEAR TO MONTH.
	_intervalYTMEncodingLen = 5

	// monthsPerYear defines the number of months in a year.
	_monthsPerYear = 12

	// canonicalSep is the separator used in the canonical string "[+|-]YY-MM".
	_canonicalSep byte = '-'
	// canonicalFracSep is the separator between seconds and fractional seconds in D2S.
	_canonicalFracSep byte = '.'

	// minEncodedMonth and maxEncodedMonth are the valid bounds (inclusive) for the
	// encoded month byte on the wire. They correspond to 0..11 after removing the bias.
	_minEncodedMonth = _monthBias
	_maxEncodedMonth = _monthBias + byte(_monthsPerYear-1)

	// maxYearPrecisionDigits is the Oracle YEAR precision (0..9). It limits the number
	// of decimal digits allowed in the canonical years token. This is a precision rule,
	// not a wire-storage constraint; the years field is stored as a 4-byte signed int.
	_maxYearPrecisionDigits = 9

	// _intervalDSEncodingLen is the expected wire size for INTERVAL DAY TO SECOND.
	_intervalDSEncodingLen = 11

	// Time unit bounds
	_hoursPerDay      = 24
	_minutesPerHour   = 60
	_secondsPerMinute = 60

	// Nanoseconds units
	_nsPerSecond int64 = 1000000000
	_nsPerMinute       = int64(_secondsPerMinute) * _nsPerSecond
	_nsPerHour         = int64(_minutesPerHour) * _nsPerMinute
	_nsPerDay          = int64(_hoursPerDay) * _nsPerHour

	// Encoded bounds for hour/minute/second allow signed components with +60 bias.
	// For negative values, bytes may be below 0x3C (e.g., hour=-6 -> 0x36).
	_minEncodedHour   = _monthBias - byte(_hoursPerDay-1)
	_maxEncodedHour   = _monthBias + byte(_hoursPerDay-1)
	_minEncodedMinute = _monthBias - byte(_minutesPerHour-1)
	_maxEncodedMinute = _monthBias + byte(_minutesPerHour-1)
	_minEncodedSecond = _monthBias - byte(_secondsPerMinute-1)
	_maxEncodedSecond = _monthBias + byte(_secondsPerMinute-1)

	// Maximum fractional second value in nanoseconds
	_maxFractionNs = 999999999

	// Wire indexes for INTERVAL YEAR TO MONTH (5 bytes)
	_ytmYearsStart = 0
	_ytmYearsLen   = 4
	_ytmMonthIdx   = 4

	// Wire indexes for INTERVAL DAY TO SECOND (11 bytes)
	_dsDaysStart = 0
	_dsDaysLen   = 4
	_dsHourIdx   = 4
	_dsMinuteIdx = 5
	_dsSecondIdx = 6
	_dsFracStart = 7
	_dsFracLen   = 4
)

// INTERVAL YEAR TO MONTH (Y2M)
// Wire (5 bytes):
//   [0..3]: big-endian int32 years XOR 0x80000000
//   [4]:    month = 60 + mon(0..11)
// Canonical string: "[+|-]Y...Y-MM"
// - Years are printed with at least 2 digits (zero-padded when < 100); may use up to 9 digits (Oracle YEAR precision).
// - Months are zero-padded to 2 digits.
// - Leading "+" is omitted for positive values.
// Note: The 9-digit limit is a precision constraint on the canonical text, not a storage limit;
//       on the wire years are a 4-byte signed integer (int32), which can represent values far beyond 9 digits in text.

// DecodeIntervalYearToMonth converts a 5-byte INTERVAL YEAR TO MONTH wire value
// into its canonical string.
//
// Input:
//   - value: a 5-byte buffer with the layout:
//   - bytes 0..3: big-endian int32 years, XOR'ed with 0x80000000
//   - byte 4    : month encoded as 60 + mon, where mon is in [-11..+11]
//
// Output:
//   - Canonical string in the form "[+|-]Y...Y-MM" where:
//   - Years are zero-padded to at least 2 digits (max precision 9 digits)
//   - Months are zero-padded to 2 digits
//   - A leading '+' is omitted for positive values
//
// Possible errors:
//   - invalid length (must be exactly 5): ConverterExpectedFormat/invalid-length
//   - invalid month bias byte (outside 60±11): ConverterRange/invalid-month-bias
//
// Examples:
//   - 2y 3m   -> "02-03"
//   - -1y 0m  -> "-01-00"
//   - 123y 2m -> "123-02"
func DecodeIntervalYearToMonth(value driverCommon.B1Array) (string, error) {
	if len(value) != _intervalYTMEncodingLen {
		common.Odl.Error(
			"DecodeIntervalYearToMonth: unexpected interval value length",
			"value",
			value,
			"expected-length",
			_intervalYTMEncodingLen)
		return "", common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalYearToMonth", "Decode", driverCommon.ReasonInvalidLength, _intervalYTMEncodingLen)
	}

	// Read 32-bit big-endian years field and remove XOR bias to get signed years
	rawYears := binary.BigEndian.Uint32(value[_ytmYearsStart : _ytmYearsStart+_ytmYearsLen])
	years := int(int32(rawYears ^ _yearXorBias))

	// Extract month using +60 bias as a signed component:
	// - Oracle's Y2M wire format allows the month byte to carry the sign (60 + mon),
	//   where mon can be in the range -11..+11.
	// - This is required so that canonical "-00-05" decodes to years=0, months=-5 (not -1y 7m).
	// - We therefore accept values below/above 0x3C (60) for negative/positive months respectively.
	monthByte := value[_ytmMonthIdx]
	monthsSigned := int(int(monthByte) - int(_monthBias))
	if monthsSigned < -(_monthsPerYear-1) || monthsSigned > (_monthsPerYear-1) {
		expectedMin := byte(int(_monthBias) - (_monthsPerYear - 1))
		expectedMax := byte(int(_monthBias) + (_monthsPerYear - 1))
		common.Odl.Error("DecodeIntervalYearToMonth: invalid month bias", "byte", monthByte, "expectedMin", expectedMin, "expectedMax", expectedMax)
		return "", common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalYearToMonth", "Decode", driverCommon.ReasonInvalidMonthBias, expectedMin, expectedMax)
	}

	// Combine into a single months count (may be negative)
	totalMonths := years*_monthsPerYear + monthsSigned

	isNegative := false
	if totalMonths < 0 {
		isNegative = true
		totalMonths = -totalMonths
	}

	// Split absolute months back into (years, months) components
	yearsAbs := totalMonths / _monthsPerYear
	monthsAbs := totalMonths % _monthsPerYear

	// Zero-pad years to at least 2 digits; no '+' for positive values.
	if yearsAbs < 100 {
		if isNegative {
			return fmt.Sprintf("-%02d-%02d", yearsAbs, monthsAbs), nil
		}
		return fmt.Sprintf("%02d-%02d", yearsAbs, monthsAbs), nil
	}
	if isNegative {
		return fmt.Sprintf("-%d-%02d", yearsAbs, monthsAbs), nil
	}
	return fmt.Sprintf("%d-%02d", yearsAbs, monthsAbs), nil
}

// EncodeIntervalYearToMonth converts a canonical string into a 5-byte
// INTERVAL YEAR TO MONTH wire value.
//
// Input:
//   - canon: string in the form "[+|-]Y...Y-MM"
//   - Optional leading '+'; '-' indicates a negative interval
//   - Years token obeys YEAR precision 0..9 (leading zeros allowed)
//   - Months token is 0..11, zero-padded to 2 digits in canonical form
//
// Output:
//   - 5-byte buffer with layout:
//   - bytes 0..3: big-endian int32 years, XOR'ed with 0x80000000
//   - byte 4    : month encoded as 60 + mon (mon may be negative if canon is negative)
//
// Possible errors:
//   - empty input: ConverterEmptyInput
//   - invalid canonical form (missing '-' separator or bad tokens):
//     ConverterExpectedFormat/invalid-format (expected "YY-MM")
//   - years precision exceeds 9 significant digits (ignoring leading zeros):
//     ConverterExpectedFormat/precision-exceeded
//   - years parse failure or negative years token: ConverterExpectedFormat/parse-years
//   - months parse failure: ConverterExpectedFormat/parse-months
//   - months out of range [0..11]: ConverterRange/month-out-of-range
//   - years outside int32 range after sign: ConverterExpectedFormat/years-overflow
//
// Examples:
//   - "02-03"  -> years=2, monthByte=0x3C+3
//   - "-01-00" -> years=-1, monthByte=0x3C+0
func EncodeIntervalYearToMonth(v driver.Value) (driverCommon.B1Array, error) {
	canon := v.(string)
	if strings.TrimSpace(canon) == "" {
		common.Odl.Error("EncodeIntervalYearToMonth: empty input")
		return nil, common.NewOracleError(oracleErrors.ConverterEmptyInput, nil, "IntervalYearToMonth", "Encode")
	}

	// Trim surrounding whitespace and handle optional leading sign.
	unsigned := strings.TrimSpace(canon)
	isNegative := false
	if len(unsigned) > 0 {
		switch unsigned[0] {
		case '+':
			unsigned = unsigned[1:]
		case '-':
			isNegative = true
			unsigned = unsigned[1:]
		}
	}

	// Locate the canonical separator without allocating intermediate slices.
	sepIdx := strings.IndexByte(unsigned, _canonicalSep)
	if sepIdx <= 0 || sepIdx == len(unsigned)-1 {
		common.Odl.Error("EncodeIntervalYearToMonth: invalid canonical form", "canon", canon)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalYearToMonth", "Encode", driverCommon.ReasonInvalidFormat, "YY-MM")
	}
	yearsToken := unsigned[:sepIdx]
	monthsToken := unsigned[sepIdx+1:]

	// Enforce precision on the years token before numeric parsing.
	// Leading zeros do not contribute to precision (e.g., "0000000001" counts as 1 digit).
	leadingZeros := 0
	for leadingZeros < len(yearsToken) && yearsToken[leadingZeros] == '0' {
		leadingZeros++
	}
	yearsDigits := len(yearsToken) - leadingZeros
	if yearsDigits == 0 {
		yearsDigits = 1 // all zeros -> effectively "0"
	}
	if yearsDigits > _maxYearPrecisionDigits {
		common.Odl.Error("EncodeIntervalYearToMonth: years exceed max precision", "yearsToken", yearsToken, "maxDigits", _maxYearPrecisionDigits)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalYearToMonth", "Encode", driverCommon.ReasonPrecisionExceeded, "<="+strconv.Itoa(_maxYearPrecisionDigits)+" digits")
	}

	yearsPart, err := strconv.ParseInt(yearsToken, 10, 32)
	if err != nil || yearsPart < 0 {
		common.Odl.Error("EncodeIntervalYearToMonth: parse years failed", "token", yearsToken, "error", err)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalYearToMonth", "Encode", driverCommon.ReasonParseYears, "number")
	}
	monthsPart, err := strconv.ParseInt(monthsToken, 10, 32)
	if err != nil {
		common.Odl.Error("EncodeIntervalYearToMonth: parse months failed", "token", monthsToken, "error", err)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalYearToMonth", "Encode", driverCommon.ReasonParseMonths, "number")
	}
	if monthsPart < 0 || monthsPart > _monthsPerYear-1 {
		common.Odl.Error("EncodeIntervalYearToMonth: months out of range", "value", monthsPart, "max", _monthsPerYear-1)
		return nil, common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalYearToMonth", "Encode", driverCommon.ReasonMonthOutOfRange, 0, _monthsPerYear-1)
	}

	// Do NOT normalize negative canonical into (years, months) with borrow/carry.
	// Tests expect that "-00-05" encodes as years=0 and a negative month byte (60-5=0x37),
	// rather than years=-1 and months=7. So we:
	//  - Parse absolute numeric tokens
	//  - Apply the sign to both components
	//  - Store years as int32 with XOR bias; store month as signed with +60 bias
	yearsComponent := int(yearsPart)
	monthsComponent := int(monthsPart)
	if isNegative {
		yearsComponent = -yearsComponent
		monthsComponent = -monthsComponent
	}

	if yearsComponent < math.MinInt32 || yearsComponent > math.MaxInt32 {
		common.Odl.Error("EncodeIntervalYearToMonth: years out of int32 range", "value", yearsComponent)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalYearToMonth", "Encode", driverCommon.ReasonYearsOverflow, "int32")
	}

	// Assemble the 5-byte wire encoding
	encoded := make(driverCommon.B1Array, _intervalYTMEncodingLen)
	rawYears := uint32(int32(yearsComponent)) ^ _yearXorBias
	binary.BigEndian.PutUint32(encoded[_ytmYearsStart:_ytmYearsStart+_ytmYearsLen], rawYears)
	encoded[_ytmMonthIdx] = byte(int(_monthBias) + monthsComponent)
	return encoded, nil
}

// INTERVAL DAY TO SECOND (D2S)
// Wire (11 bytes):
//   [0..3]:  big-endian int32 days XOR 0x80000000
//   [4]:     hour   = 60 + hh (0..23)
//   [5]:     minute = 60 + mi (0..59)
//   [6]:     second = 60 + ss (0..59)
//   [7..10]: big-endian uint32 fractional seconds in nanoseconds (0..999,999,999)
// Canonical string: "[+|-]DD HH:MM:SS[.FFFFFFFFF]"

// DecodeIntervalDayToSecond converts an 11-byte INTERVAL DAY TO SECOND wire value
// to its canonical string representation.
//
// Input:
//   - value: an 11-byte buffer with the layout:
//   - bytes 0..3 : big-endian int32 days, XOR'ed with 0x80000000
//   - byte  4    : hour   encoded as 60 + hh (hh in [-23..+23])
//   - byte  5    : minute encoded as 60 + mi (mi in [-59..+59])
//   - byte  6    : second encoded as 60 + ss (ss in [-59..+59])
//   - bytes 7..10: big-endian int32 fractional nanoseconds XOR'ed with 0x80000000
//
// Output:
//   - Canonical string "[+|-]DD HH:MM:SS[.FFFFFFFFF]" where:
//   - '-' is emitted for negative values; '+' is omitted for positive
//   - DD is zero-padded to at least 2 digits (more digits printed as needed)
//   - HH/MM/SS are zero-padded to 2 digits
//   - Fractional part is ".0" when zero; otherwise 1..9 digits, trailing zeros trimmed
//
// Possible errors:
//   - invalid length (must be exactly 11): ConverterExpectedFormat/invalid-length
//   - invalid hour bias byte:   ConverterRange/invalid-hour-bias
//   - invalid minute bias byte: ConverterRange/invalid-minute-bias
//   - invalid second bias byte: ConverterRange/invalid-second-bias
//   - invalid fractional nanoseconds (abs > 999,999,999): ConverterRange/invalid-fraction
func DecodeIntervalDayToSecond(value driverCommon.B1Array) (string, error) {
	if len(value) != _intervalDSEncodingLen {
		common.Odl.Error(
			"DecodeIntervalDayToSecond: unexpected interval value length",
			"value",
			value,
			"expected-length",
			_intervalDSEncodingLen)
		return "", common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Decode", driverCommon.ReasonInvalidLength, _intervalDSEncodingLen)
	}

	// Days: int32 with XOR bias
	rawDays := binary.BigEndian.Uint32(value[_dsDaysStart : _dsDaysStart+_dsDaysLen])
	daysSigned := int32(rawDays ^ _yearXorBias)

	// Hour/minute/second with +60 bias (allow negative components)
	hb := value[_dsHourIdx]
	mb := value[_dsMinuteIdx]
	sb := value[_dsSecondIdx]
	if hb < _minEncodedHour || hb > _maxEncodedHour {
		common.Odl.Error("DecodeIntervalDayToSecond: invalid hour bias", "byte", hb, "expectedMin", _minEncodedHour, "expectedMax", _maxEncodedHour)
		return "", common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Decode", driverCommon.ReasonInvalidHourBias, _minEncodedHour, _maxEncodedHour)
	}
	if mb < _minEncodedMinute || mb > _maxEncodedMinute {
		common.Odl.Error("DecodeIntervalDayToSecond: invalid minute bias", "byte", mb, "expectedMin", _minEncodedMinute, "expectedMax", _maxEncodedMinute)
		return "", common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Decode", driverCommon.ReasonInvalidMinuteBias, _minEncodedMinute, _maxEncodedMinute)
	}
	if sb < _minEncodedSecond || sb > _maxEncodedSecond {
		common.Odl.Error("DecodeIntervalDayToSecond: invalid second bias", "byte", sb, "expectedMin", _minEncodedSecond, "expectedMax", _maxEncodedSecond)
		return "", common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Decode", driverCommon.ReasonInvalidSecondBias, _minEncodedSecond, _maxEncodedSecond)
	}
	hoursSigned := int(hb) - int(_monthBias)
	minsSigned := int(mb) - int(_monthBias)
	secsSigned := int(sb) - int(_monthBias)

	// Fractional seconds (nanoseconds) with XOR/offset bias
	rawNs := binary.BigEndian.Uint32(value[_dsFracStart : _dsFracStart+_dsFracLen])
	nsSigned := int64(int32(rawNs ^ _yearXorBias))
	nsAbs := nsSigned
	if nsAbs < 0 {
		nsAbs = -nsAbs
	}
	if nsAbs > _maxFractionNs {
		common.Odl.Error("DecodeIntervalDayToSecond: invalid fractional nanoseconds", "value", nsSigned)
		return "", common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Decode", driverCommon.ReasonInvalidFraction, 0, _maxFractionNs)
	}

	// Determine sign from any component (all components share the same sign by encoding contract)
	neg := daysSigned < 0 || hoursSigned < 0 || minsSigned < 0 || secsSigned < 0 || nsSigned < 0

	// Absolute components for formatting
	dd := daysSigned
	if dd < 0 {
		dd = -dd
	}
	hh := hoursSigned
	if hh < 0 {
		hh = -hh
	}
	mm := minsSigned
	if mm < 0 {
		mm = -mm
	}
	ss := secsSigned
	if ss < 0 {
		ss = -ss
	}
	fs := nsAbs // 0..999,999,999

	// Days formatting per tests
	var daysStr string
	if neg {
		daysStr = fmt.Sprintf("%d", dd)
	} else if dd < 100 {
		daysStr = fmt.Sprintf("%02d", dd)
	} else {
		daysStr = fmt.Sprintf("%d", dd)
	}

	// Fractional seconds string (always emit .<digits>, with ".0" when zero)
	var fracStr string
	if fs == 0 {
		fracStr = "0"
	} else {
		fracStr = fmt.Sprintf("%09d", fs)
		fracStr = strings.TrimRight(fracStr, "0")
	}

	if neg {
		return fmt.Sprintf("-%s %02d:%02d:%02d.%s", daysStr, hh, mm, ss, fracStr), nil
	}
	return fmt.Sprintf("%s %02d:%02d:%02d.%s", daysStr, hh, mm, ss, fracStr), nil
}

// EncodeIntervalDayToSecond converts a canonical string into an 11-byte
// INTERVAL DAY TO SECOND wire value.
//
// Input:
//   - canon: string in the form "[+|-]DD HH:MM:SS[.FFFFFFFFF]"
//   - Optional leading '+'; '-' indicates a negative interval
//   - DD must be a non-negative integer (the sign applies to the whole value)
//   - HH in [0..23], MM in [0..59], SS in [0..59]
//   - Fractional part 1..9 digits for nanoseconds; right-padded to 9 digits on the wire
//
// Output:
//   - 11-byte buffer with layout:
//   - bytes 0..3 : big-endian int32 days, XOR'ed with 0x80000000
//   - byte  4    : 60 + hh (may be below/above 0x3C when negative/positive)
//   - byte  5    : 60 + mi
//   - byte  6    : 60 + ss
//   - bytes 7..10: big-endian int32 fractional nanoseconds XOR'ed with 0x80000000
//
// Possible errors:
//   - empty or whitespace input: ConverterEmptyInput
//   - invalid canonical form (missing space or bad time token):
//     ConverterExpectedFormat/invalid-format (expected "DD HH:MM:SS[.FFFFFFFFF]")
//   - day parse failure or negative value: ConverterExpectedFormat/parse-day
//   - time token not in "HH:MM:SS[.FFFFFFFFF]" form: ConverterExpectedFormat/invalid-format
//   - hour/minute/second parse failure: ConverterExpectedFormat/parse-hour|parse-minute|parse-second
//   - hour/minute/second out of range:  ConverterRange/hour-out-of-range|minute-out-of-range|second-out-of-range
//   - fractional precision > 9:          ConverterExpectedFormat/precision-exceeded
//   - fractional parse failure or negative: ConverterExpectedFormat/parse-fraction
//   - days outside int32 range after sign:  ConverterExpectedFormat/days-overflow
func EncodeIntervalDayToSecond(v driver.Value) (driverCommon.B1Array, error) {
	canon := v.(string)
	if strings.TrimSpace(canon) == "" {
		common.Odl.Error("EncodeIntervalDayToSecond: empty input")
		return nil, common.NewOracleError(oracleErrors.ConverterEmptyInput, nil, "IntervalDayToSecond", "Encode")
	}

	unsigned := strings.TrimSpace(canon)
	isNegative := false
	if len(unsigned) > 0 {
		switch unsigned[0] {
		case '+':
			unsigned = strings.TrimSpace(unsigned[1:])
		case '-':
			isNegative = true
			unsigned = strings.TrimSpace(unsigned[1:])
		}
	}
	// Expect "<days> <time>"
	spaceIdx := strings.IndexByte(unsigned, ' ')
	if spaceIdx <= 0 || spaceIdx == len(unsigned)-1 {
		common.Odl.Error("EncodeIntervalDayToSecond: invalid canonical form", "canon", canon)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonInvalidFormat, "DD HH:MM:SS[.FFFFFFFFF]")
	}
	dayToken := unsigned[:spaceIdx]
	timeToken := unsigned[spaceIdx+1:]

	// Parse day (non-negative)
	dayPart64, err := strconv.ParseInt(dayToken, 10, 64)
	if err != nil || dayPart64 < 0 {
		common.Odl.Error("EncodeIntervalDayToSecond: parse day failed", "token", dayToken, "error", err)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonParseDay, "number")
	}

	// Parse time "HH:MM:SS[.fffffffff]"
	tparts := strings.SplitN(timeToken, ":", 3)
	if len(tparts) != 3 {
		common.Odl.Error("EncodeIntervalDayToSecond: invalid time token", "token", timeToken)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonInvalidFormat, "HH:MM:SS[.FFFFFFFFF]")
	}
	hhPart64, err := strconv.ParseInt(tparts[0], 10, 64)
	if err != nil {
		common.Odl.Error("EncodeIntervalDayToSecond: parse hour failed", "token", tparts[0], "error", err)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonParseHour, "number")
	}
	mmPart64, err := strconv.ParseInt(tparts[1], 10, 64)
	if err != nil {
		common.Odl.Error("EncodeIntervalDayToSecond: parse minute failed", "token", tparts[1], "error", err)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonParseMinute, "number")
	}

	secToken := tparts[2]
	secWhole := secToken
	fracToken := ""
	if dot := strings.IndexByte(secToken, _canonicalFracSep); dot != -1 {
		secWhole = secToken[:dot]
		fracToken = secToken[dot+1:]
	}
	ssPart64, err := strconv.ParseInt(secWhole, 10, 64)
	if err != nil {
		common.Odl.Error("EncodeIntervalDayToSecond: parse second failed", "token", secWhole, "error", err)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonParseSecond, "number")
	}
	if hhPart64 < 0 || hhPart64 >= _hoursPerDay {
		common.Odl.Error("EncodeIntervalDayToSecond: hour out of range", "value", hhPart64, "max", _hoursPerDay-1)
		return nil, common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonHourOutOfRange, 0, _hoursPerDay-1)
	}
	if mmPart64 < 0 || mmPart64 >= _minutesPerHour {
		common.Odl.Error("EncodeIntervalDayToSecond: minute out of range", "value", mmPart64, "max", _minutesPerHour-1)
		return nil, common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonMinuteOutOfRange, 0, _minutesPerHour-1)
	}
	if ssPart64 < 0 || ssPart64 >= _secondsPerMinute {
		common.Odl.Error("EncodeIntervalDayToSecond: second out of range", "value", ssPart64, "max", _secondsPerMinute-1)
		return nil, common.NewOracleError(oracleErrors.ConverterRange, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonSecondOutOfRange, 0, _secondsPerMinute-1)
	}

	// Fractional seconds parsing (nanoseconds)
	var ns uint32 = 0
	if fracToken != "" {
		if len(fracToken) > 9 {
			common.Odl.Error("EncodeIntervalDayToSecond: fractional precision exceeds", "digits", len(fracToken))
			return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonPrecisionExceeded, "<=9 digits")
		}
		// Right-pad to 9 digits
		for len(fracToken) < 9 {
			fracToken += "0"
		}
		fv, err := strconv.ParseInt(fracToken, 10, 32)
		if err != nil || fv < 0 {
			common.Odl.Error("EncodeIntervalDayToSecond: parse fractional seconds failed", "token", fracToken, "error", err)
			return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonParseFraction, "number")
		}
		ns = uint32(fv)
	}

	// Apply sign to all components directly (Java mapping)
	daySigned := int64(dayPart64)
	hhSigned := int64(hhPart64)
	mmSigned := int64(mmPart64)
	ssSigned := int64(ssPart64)
	fracSigned := int64(ns)
	if isNegative {
		daySigned = -daySigned
		hhSigned = -hhSigned
		mmSigned = -mmSigned
		ssSigned = -ssSigned
		fracSigned = -fracSigned
	}

	if daySigned < math.MinInt32 || daySigned > math.MaxInt32 {
		common.Odl.Error("EncodeIntervalDayToSecond: days out of int32 range", "value", daySigned)
		return nil, common.NewOracleError(oracleErrors.ConverterExpectedFormat, nil, "IntervalDayToSecond", "Encode", driverCommon.ReasonDaysOverflow, "int32")
	}

	// Assemble the 11-byte wire encoding using +60 bias for H/M/S and 0x80000000 XOR bias for day and fractional
	encoded := make(driverCommon.B1Array, _intervalDSEncodingLen)
	rawDays := uint32(int32(daySigned)) ^ _yearXorBias
	binary.BigEndian.PutUint32(encoded[_dsDaysStart:_dsDaysStart+_dsDaysLen], rawDays)
	encoded[_dsHourIdx] = byte(int(_monthBias) + int(hhSigned)) // may be below/above 0x3C due to negative/positive hour
	encoded[_dsMinuteIdx] = byte(int(_monthBias) + int(mmSigned))
	encoded[_dsSecondIdx] = byte(int(_monthBias) + int(ssSigned))
	rawFrac := uint32(int32(fracSigned)) ^ _yearXorBias
	binary.BigEndian.PutUint32(encoded[_dsFracStart:_dsFracStart+_dsFracLen], rawFrac)
	return encoded, nil
}
