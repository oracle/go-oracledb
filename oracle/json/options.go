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

package json

import drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"

// NumberMode selects the Go representation of materialized JSON numbers.
type NumberMode = drvCommon.JSONNumberMode

const (
	// NumberDefault preserves the numeric category represented by OSON.
	NumberDefault = drvCommon.JSONNumberDefault
	// NumberAsJSONNumber returns finite JSON numbers as encoding/json.Number.
	NumberAsJSONNumber = drvCommon.JSONNumberAsJSONNumber
	// NumberAsFloat64 converts JSON numbers to float64.
	NumberAsFloat64 = drvCommon.JSONNumberAsFloat64
)

// TimeEncoding selects the OSON scalar used to encode time.Time values.
type TimeEncoding = drvCommon.JSONTimeEncoding

const (
	// TimeAsTimestamp preserves calendar fields and nanoseconds without a time zone.
	TimeAsTimestamp = drvCommon.JSONTimeAsTimestamp
	// TimeAsTimestampTZ preserves calendar fields, nanoseconds, and the numeric UTC offset.
	TimeAsTimestampTZ = drvCommon.JSONTimeAsTimestampTZ
	// TimeAsDate preserves calendar fields through whole seconds without a time zone.
	TimeAsDate = drvCommon.JSONTimeAsDate
)

// Options configures JSON construction and materialization. Create options with
// [NumberModeOption] and [TimeEncodingOption], and combine them with
// [JoinOptions]. The zero value selects [NumberDefault] and [TimeAsTimestamp].
//
// A [JSON] retains its options through [JSON.Scan] and propagates them to lazy
// views returned by [JSON.AsJSONObject], [JSON.AsJSONArray], and
// [JSON.AsJSONScalar].
type Options struct {
	set        optionSet
	conversion drvCommon.JSONConversionOptions
}

// optionSet records the explicitly selected Options properties.
type optionSet uint64

const (
	numberModeSet optionSet = 1 << iota
	timeEncodingSet
)

// NumberModeOption returns an Options that selects mode for number
// materialization.
func NumberModeOption(mode NumberMode) Options {
	return Options{set: numberModeSet, conversion: drvCommon.JSONConversionOptions{NumberMode: mode}}
}

// TimeEncodingOption returns an Options that selects encoding for time.Time.
func TimeEncodingOption(encoding TimeEncoding) Options {
	return Options{set: timeEncodingSet, conversion: drvCommon.JSONConversionOptions{TimeEncoding: encoding}}
}

// JoinOptions combines options. Later arguments override earlier ones.
func JoinOptions(opts ...Options) Options {
	var result Options
	for _, option := range opts {
		if option.set&numberModeSet != 0 {
			result.conversion.NumberMode = option.conversion.NumberMode
		}
		if option.set&timeEncodingSet != 0 {
			result.conversion.TimeEncoding = option.conversion.TimeEncoding
		}
		result.set |= option.set
	}
	return result
}
