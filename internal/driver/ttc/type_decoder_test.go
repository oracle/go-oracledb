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
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// The destination keeps these decoder contexts package-private.
type ColumnContext = columnContext
type LobColumnContext = lobColumnContext

func TestDecodeNumberColumn(t *testing.T) {
	intVal, err := DecodeNumberColumn(ColumnContext{Scale: 0}, common.B1Array{0xC1, 0x02})
	if err != nil {
		t.Fatalf("DecodeNumberColumn integer returned error: %v", err)
	}
	if intVal != int64(1) {
		t.Fatalf("integer value = %v (%T), want int64(1)", intVal, intVal)
	}

	decimalWire, err := converters.EncodeDecimal(12.34)
	if err != nil {
		t.Fatalf("EncodeDecimal returned error: %v", err)
	}
	decimalVal, err := DecodeNumberColumn(ColumnContext{Precision: 4, Scale: 2}, decimalWire)
	if err != nil {
		t.Fatalf("DecodeNumberColumn decimal returned error: %v", err)
	}
	if decimalVal != "12.34" {
		t.Fatalf("decimal value = %v (%T), want 12.34", decimalVal, decimalVal)
	}

	floatWire, err := converters.EncodeFloat(1.5)
	if err != nil {
		t.Fatalf("EncodeFloat returned error: %v", err)
	}
	floatVal, err := DecodeNumberColumn(ColumnContext{Precision: 126, Scale: NumberScaleFloatSentinel}, floatWire)
	if err != nil {
		t.Fatalf("DecodeNumberColumn float returned error: %v", err)
	}
	if floatVal != float64(1.5) {
		t.Fatalf("float value = %v (%T), want float64(1.5)", floatVal, floatVal)
	}
}

func TestDecodeTextColumns(t *testing.T) {
	tests := []struct {
		name string
		fn   func(ColumnContext, common.B1Array) (driver.Value, error)
		ctx  ColumnContext
		data common.B1Array
		want string
	}{
		{name: "varchar", fn: DecodeVarcharColumn, data: common.B1Array("hello"), want: "hello"},
		{name: "nvarchar utf8", fn: DecodeVarcharColumn, ctx: ColumnContext{CharsetForm: 2, CharsetID: uint16(al32Utf8CharSet)}, data: common.B1Array("hello"), want: "hello"},
		{name: "nvarchar utf16", fn: DecodeVarcharColumn, ctx: ColumnContext{CharsetForm: 2, CharsetID: uint16(al16Utf16CharSet)}, data: common.B1Array{0, 'h', 0, 'i'}, want: "hi"},
		{name: "char", fn: DecodeCharColumn, data: common.B1Array("x"), want: "x"},
		{name: "nchar utf8", fn: DecodeCharColumn, ctx: ColumnContext{CharsetForm: 2, CharsetID: uint16(al32Utf8CharSet)}, data: common.B1Array("x"), want: "x"},
		{name: "nchar utf16", fn: DecodeCharColumn, ctx: ColumnContext{CharsetForm: 2, CharsetID: uint16(al16Utf16CharSet)}, data: common.B1Array{0, 'x'}, want: "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn(tt.ctx, tt.data)
			if err != nil {
				t.Fatalf("decode returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decode = %v (%T), want %q", got, got, tt.want)
			}
		})
	}
}

func TestDecodeBooleanColumn(t *testing.T) {
	got, err := DecodeBooleanColumn(ColumnContext{}, common.B1Array{1})
	if err != nil {
		t.Fatalf("DecodeBooleanColumn returned error: %v", err)
	}
	if got != true {
		t.Fatalf("DecodeBooleanColumn = %v, want true", got)
	}
}

func TestDecodeFloatingAndTemporalColumns(t *testing.T) {
	binaryFloat, err := converters.EncodeBinaryFloat(float32(1.5))
	if err != nil {
		t.Fatalf("EncodeBinaryFloat returned error: %v", err)
	}
	gotBinaryFloat, err := DecodeBinaryFloatColumn(ColumnContext{}, binaryFloat)
	if err != nil {
		t.Fatalf("DecodeBinaryFloatColumn returned error: %v", err)
	}
	if gotBinaryFloat != float64(1.5) {
		t.Fatalf("binary float = %v, want 1.5", gotBinaryFloat)
	}

	binaryDouble, err := converters.EncodeBinaryDouble(float64(2.5))
	if err != nil {
		t.Fatalf("EncodeBinaryDouble returned error: %v", err)
	}
	gotBinaryDouble, err := DecodeBinaryDoubleColumn(ColumnContext{}, binaryDouble)
	if err != nil {
		t.Fatalf("DecodeBinaryDoubleColumn returned error: %v", err)
	}
	if gotBinaryDouble != float64(2.5) {
		t.Fatalf("binary double = %v, want 2.5", gotBinaryDouble)
	}

	when := time.Date(2026, 5, 4, 12, 30, 45, 123000000, time.UTC)
	dateWire, err := converters.EncodeDate(when)
	if err != nil {
		t.Fatalf("EncodeDate returned error: %v", err)
	}
	if got, err := DecodeDateColumn(ColumnContext{}, dateWire); err != nil || got.(time.Time).Year() != 2026 {
		t.Fatalf("DecodeDateColumn = (%v, %v), want 2026 date", got, err)
	}
	tsWire, err := converters.EncodeTimestamp(when)
	if err != nil {
		t.Fatalf("EncodeTimestamp returned error: %v", err)
	}
	if got, err := DecodeTimestampColumn(ColumnContext{}, tsWire); err != nil || got.(time.Time).Year() != 2026 {
		t.Fatalf("DecodeTimestampColumn = (%v, %v), want 2026 timestamp", got, err)
	}
	tstzWire, err := converters.EncodeTimestampWithTimeZone(when)
	if err != nil {
		t.Fatalf("EncodeTimestampWithTimeZone returned error: %v", err)
	}
	if got, err := DecodeTimestampWithTimeZoneColumn(ColumnContext{}, tstzWire); err != nil || got.(time.Time).IsZero() {
		t.Fatalf("DecodeTimestampWithTimeZoneColumn = (%v, %v), want non-zero time", got, err)
	}
	if got, err := DecodeTimestampWithLocalTimeZoneColumn(ColumnContext{}, tsWire); err != nil || got.(time.Time).IsZero() {
		t.Fatalf("DecodeTimestampWithLocalTimeZoneColumn = (%v, %v), want non-zero time", got, err)
	}
}

func TestDecodeIntervalAndLobColumns(t *testing.T) {
	iymWire, err := converters.EncodeIntervalYearToMonth("02-03")
	if err != nil {
		t.Fatalf("EncodeIntervalYearToMonth returned error: %v", err)
	}
	if got, err := DecodeIntervalYearToMonthColumn(ColumnContext{}, iymWire); err != nil || got != "02-03" {
		t.Fatalf("DecodeIntervalYearToMonthColumn = (%v, %v), want 02-03", got, err)
	}

	idsWire, err := converters.EncodeIntervalDayToSecond("02 03:04:05.006")
	if err != nil {
		t.Fatalf("EncodeIntervalDayToSecond returned error: %v", err)
	}
	if got, err := DecodeIntervalDayToSecondColumn(ColumnContext{}, idsWire); err != nil || got != "02 03:04:05.006" {
		t.Fatalf("DecodeIntervalDayToSecondColumn = (%v, %v), want canonical interval", got, err)
	}

	if got, err := DecodeClob(ColumnContext{LobContext: &LobColumnContext{}}, common.B1Array("text")); err != nil || got != "text" {
		t.Fatalf("DecodeClob utf8 = (%v, %v), want text", got, err)
	}
	if got, err := DecodeClob(ColumnContext{LobContext: &LobColumnContext{CharsetID: al16Utf16CharSet}}, common.B1Array{0, 'O', 0, 'K'}); err != nil || got != "OK" {
		t.Fatalf("DecodeClob utf16 = (%v, %v), want OK", got, err)
	}
	if got, err := DecodeJson(ColumnContext{}, common.B1Array(`{"ok":true}`)); err != nil || got != `{"ok":true}` {
		t.Fatalf("DecodeJson = (%v, %v), want JSON string", got, err)
	}
	blob := common.B1Array{1, 2, 3}
	gotBlob, err := DecodeBlob(ColumnContext{}, blob)
	if err != nil {
		t.Fatalf("DecodeBlob returned error: %v", err)
	}
	if !reflect.DeepEqual(gotBlob, blob) {
		t.Fatalf("DecodeBlob = %v, want %v", gotBlob, blob)
	}
}

func TestRowDecodeError(t *testing.T) {
	err := rowDecodeError(ColumnContext{Name: common.B1Array("C1"), Index: 2}, errors.New("bad wire"), "NUMBER")
	var sqle oracleErrors.SQLError
	if !errors.As(err, &sqle) {
		t.Fatalf("rowDecodeError should return SQLError, got %T", err)
	}
	if sqle.ErrorCode() != string(oracleErrors.RowDecodeError) {
		t.Fatalf("error code = %s, want %s", sqle.ErrorCode(), oracleErrors.RowDecodeError)
	}
}
