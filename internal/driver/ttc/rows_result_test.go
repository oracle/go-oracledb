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
	"database/sql/driver"
	"reflect"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

// Test_defaultNumericValue verifies numeric defaults for NULL values across
// integer, floating-point sentinel, and arbitrary precision NUMBER columns.
func Test_defaultNumericValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		scale int8
		want  any
	}{
		{name: "integer scale -> int64 zero", scale: 0, want: int64(0)},
		{name: "float sentinel -> float64 zero", scale: NumberScaleFloatSentinel, want: float64(0)},
		{name: "other scale -> decimal string", scale: 5, want: "0"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := defaultNumericValue(tc.scale)
			if !ok {
				t.Fatalf("defaultNumericValue returned ok=false for scale %d", tc.scale)
			}
			if !valuesEqual(got, tc.want) {
				t.Fatalf("default numeric value mismatch for scale %d: got %#v (%T), want %#v (%T)", tc.scale, got, got, tc.want, tc.want)
			}
		})
	}
}

// Test_defaultValueForNull exercises the mapping of TTC data types to default
// Go driver values when strict null handling is disabled.
func Test_defaultValueForNull(t *testing.T) {
	t.Parallel()

	zeroTime := time.Time{}
	cases := []struct {
		name  string
		dtype DtyType
		scale int8
		want  any
	}{
		{name: "NUMBER scale 0 -> int64 zero", dtype: DtyNum, scale: 0, want: int64(0)},
		{name: "NUMBER float sentinel -> float64 zero", dtype: DtyNum, scale: NumberScaleFloatSentinel, want: float64(0)},
		{name: "NUMBER default -> string zero", dtype: DtyNum, scale: 3, want: "0"},
		{name: "VARCHAR2 -> empty string", dtype: DtyVCS, want: ""},
		{name: "RAW -> empty byte slice", dtype: DtyBin, want: common.B1Array{}},
		{name: "BOOLEAN -> false", dtype: DtyBol, want: false},
		{name: "DATE -> zero time", dtype: DtyDat, want: zeroTime},
		{name: "TIMESTAMP WITH TZ -> zero time", dtype: DtyTtz, want: zeroTime},
		{name: "INTERVAL YEAR TO MONTH -> 00-00", dtype: DtyIym, want: "00-00"},
		{name: "INTERVAL DAY TO SECOND -> 00 00:00:00.0", dtype: DtyIds, want: "00 00:00:00.0"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := _defaultValueForNull(tc.dtype, tc.scale)
			if !ok {
				t.Fatalf("_defaultValueForNull returned ok=false for dtype=%d scale=%d", tc.dtype, tc.scale)
			}
			if !valuesEqual(got, tc.want) {
				t.Fatalf("NULL default mismatch for dtype=%d scale=%d: got %#v (%T), want %#v (%T)", tc.dtype, tc.scale, got, got, tc.want, tc.want)
			}
		})
	}
}

// Test_defaultValueForNullUnknown verifies that unrecognised TTC datatypes do not provide defaults.
func Test_defaultValueForNullUnknown(t *testing.T) {
	t.Parallel()

	if got, ok := _defaultValueForNull(DtyType(0x7FFF), 0); ok || got != nil {
		t.Fatalf("expected unknown type to return (nil, false), got (%#v, %v)", got, ok)
	}
}

// Test_ttcRows_handleNullStrict validates that strict null handling returns nil values.
func Test_ttcRows_handleNullStrict(t *testing.T) {
	t.Parallel()

	rows := buildTestTTCRows(true, DtyNum, 0, nil)

	val := rows.handleNull(0, DtyNum, 0)
	if val != nil {
		t.Fatalf("strict null handling should return nil, got %#v (%T)", val, val)
	}

	rows = buildTestTTCRows(true, DtyVCS, 0, new(true))
	val = rows.handleNull(0, DtyVCS, 0)
	if val != nil {
		t.Fatalf("strict null handling should return nil when property is true, got %#v (%T)", val, val)
	}
}

// Test_ttcRows_handleNullDefaulting validates that disabling strict null handling surfaces type-specific defaults.
func Test_ttcRows_handleNullDefaulting(t *testing.T) {
	t.Parallel()

	strict := false
	cases := []struct {
		name  string
		dtype DtyType
		scale int8
		want  any
	}{
		{name: "NUMBER int64 default", dtype: DtyNum, scale: 0, want: int64(0)},
		{name: "NUMBER float default", dtype: DtyNum, scale: NumberScaleFloatSentinel, want: float64(0)},
		{name: "VARCHAR2 default", dtype: DtyVCS, want: ""},
		{name: "RAW default", dtype: DtyBin, want: common.B1Array{}},
		{name: "BOOLEAN default", dtype: DtyBol, want: false},
		{name: "TIMESTAMP default", dtype: DtyStamp, want: time.Time{}},
		{name: "INTERVAL YM default", dtype: DtyIym, want: "00-00"},
		{name: "INTERVAL DS default", dtype: DtyIds, want: "00 00:00:00.0"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rows := buildTestTTCRows(true, tc.dtype, tc.scale, &strict)

			val := rows.handleNull(0, tc.dtype, tc.scale)
			if !valuesEqual(val, tc.want) {
				t.Fatalf("defaulted value mismatch: got %#v (%T), want %#v (%T)", val, val, tc.want, tc.want)
			}
		})
	}
}

// TestTTCRowsImplementsColumnTypeInterfaces verifies that ttcRows satisfies
// the optional database/sql Rows column metadata interfaces.
func TestTTCRowsImplementsColumnTypeInterfaces(t *testing.T) {
	t.Parallel()
	var rows any = (*ttcRows)(nil)

	if _, ok := rows.(driver.RowsColumnTypeDatabaseTypeName); !ok {
		t.Fatal("ttcRows does not implement driver.RowsColumnTypeDatabaseTypeName")
	}
	if _, ok := rows.(driver.RowsColumnTypeLength); !ok {
		t.Fatal("ttcRows does not implement driver.RowsColumnTypeLength")
	}
	if _, ok := rows.(driver.RowsColumnTypeNullable); !ok {
		t.Fatal("ttcRows does not implement driver.RowsColumnTypeNullable")
	}
	if _, ok := rows.(driver.RowsColumnTypePrecisionScale); !ok {
		t.Fatal("ttcRows does not implement driver.RowsColumnTypePrecisionScale")
	}
	if _, ok := rows.(driver.RowsColumnTypeScanType); !ok {
		t.Fatal("ttcRows does not implement driver.RowsColumnTypeScanType")
	}
}

// buildTestTTCRows constructs a minimal ttcRows populated with the supplied metadata.
func buildTestTTCRows(nullable bool, dtype DtyType, scale int8, strictNull *bool) *ttcRows {
	shelf := newShelf[common.MessageType]()
	if strictNull != nil {
		props := oracleconfig.OracleDriverProperties{}
		props.StrictNullValueHandling = *strictNull
		shelf.Shelf.UpdateConnectionProperties(&props)
	}

	rows := &ttcRows{
		columnContexts: []columnContext{
			{
				Index:    0,
				Name:     common.StringToB1Array("col0"),
				DataType: int16(dtype),
				Scale:    scale,
				Nullable: nullable,
			},
		},
	}
	rows.SetShelf(shelf)
	return rows
}

// valuesEqual compares two driver.Value instances with special handling for slices and times.
func valuesEqual(got, want any) bool {
	switch w := want.(type) {
	case common.B1Array:
		switch gv := got.(type) {
		case common.B1Array:
			return bytes.Equal(gv, w)
		case []byte:
			return bytes.Equal(gv, []byte(w))
		default:
			return false
		}
	case []byte:
		gv, ok := got.([]byte)
		return ok && bytes.Equal(gv, w)
	case time.Time:
		gv, ok := got.(time.Time)
		if !ok {
			return false
		}
		return gv.Equal(w)
	default:
		return reflect.DeepEqual(got, want)
	}
}

// TestTTCRowsColumnTypeScanType verifies scan type mappings for all supported column types.
func TestTTCRowsColumnTypeScanType(t *testing.T) {
	cases := []struct {
		name  string
		dtype DtyType
		scale int8
		want  reflect.Type
	}{
		{name: "NUMBER as INT", dtype: DtyNum, scale: 0, want: reflect.TypeFor[int64]()},
		{name: "NUMBER as FLOAT", dtype: DtyNum, scale: NumberScaleFloatSentinel, want: reflect.TypeFor[float64]()},
		{name: "NUMBER as STRING", dtype: DtyNum, scale: 1, want: reflect.TypeFor[string]()},

		{name: "VARCHAR", dtype: DtyChr, want: reflect.TypeFor[string]()},
		{name: "CHAR", dtype: DtyAfc, want: reflect.TypeFor[string]()},

		{name: "BOOLEAN", dtype: DtyBol, want: reflect.TypeFor[bool]()},

		{name: "BINARY_FLOAT", dtype: DtyIbFloat, want: reflect.TypeFor[float64]()},
		{name: "BINARY_DOUBLE", dtype: DtyIbDouble, want: reflect.TypeFor[float64]()},

		{name: "INTERVAL YEAR TO MONTH", dtype: DtyIym, want: reflect.TypeFor[string]()},
		{name: "extended INTERVAL YEAR TO MONTH", dtype: DtyEiym, want: reflect.TypeFor[string]()},
		{name: "INTERVAL DAY TO SECOND", dtype: DtyIds, want: reflect.TypeFor[string]()},
		{name: "extended INTERVAL DAY TO SECOND", dtype: DtyEids, want: reflect.TypeFor[string]()},

		{name: "DATE", dtype: DtyDat, want: reflect.TypeFor[time.Time]()},
		{name: "extended DATE", dtype: DtyEdate, want: reflect.TypeFor[time.Time]()},

		{name: "TIMESTAMP", dtype: DtyStamp, want: reflect.TypeFor[time.Time]()},
		{name: "extended TIMESTAMP", dtype: DtyEstamp, want: reflect.TypeFor[time.Time]()},
		{name: "TIMESTAMP WITH TIME ZONE", dtype: DtyStz, want: reflect.TypeFor[time.Time]()},
		{name: "extended TIMESTAMP WITH TIME ZONE", dtype: DtyEstz, want: reflect.TypeFor[time.Time]()},
		{name: "TIMESTAMP WITH LOCAL TIME ZONE", dtype: DtySitz, want: reflect.TypeFor[time.Time]()},
		{name: "extended TIMESTAMP WITH LOCAL TIME ZONE", dtype: DtyEsitz, want: reflect.TypeFor[time.Time]()},

		{name: "RAW", dtype: DtyBin, want: reflect.TypeFor[[]byte]()},

		{name: "CLOB", dtype: DtyClob, want: reflect.TypeFor[string]()},
		{name: "BLOB", dtype: DtyBlob, want: reflect.TypeFor[[]byte]()},

		{name: "JSON", dtype: DtyJSON, want: reflect.TypeFor[string]()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := buildTestTTCRows(true, tc.dtype, tc.scale, nil)
			rows.shelf.RegisterCodecFactory(NewCodecFactoryForProtocol(MinTTCProtocolVersion))

			if got := rows.ColumnTypeScanType(0); got != tc.want {
				t.Fatalf("ColumnTypeScanType() = %v (%s), want %v (%s)",
					got, got.Kind(), tc.want, tc.want.Kind())
			}
		})
	}
}
