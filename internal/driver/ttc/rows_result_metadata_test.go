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
	"io"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestTTCRowsColumnsNextAndMetadata(t *testing.T) {
	rows := newTTCRows([]ColumnContext{
		{Name: common.B1Array("NAME"), DataType: DtyChr, Length: 20, Nullable: true},
		{Name: common.B1Array("AMOUNT"), DataType: DtyNum, Precision: 8, Scale: 0},
	})
	shelf := newShelf[common.MessageType]()
	shelf.RegisterCodecFactory(NewCodecFactoryForProtocol(MinTTCProtocolVersion))
	rows.SetShelf(shelf)
	rows.rowData = [][]common.B1Array{{common.B1Array("abc"), common.B1Array{0xC1, 0x02}}}
	rows.lobColContext = [][]*LobColumnContext{{nil, nil}}
	rows.numOfRows = 1

	if got := rows.Columns(); !reflect.DeepEqual(got, []string{"NAME", "AMOUNT"}) {
		t.Fatalf("Columns() = %v, want NAME/AMOUNT", got)
	}
	if got := rows.ColumnTypeDatabaseTypeName(0); got != "VARCHAR2" {
		t.Fatalf("ColumnTypeDatabaseTypeName(0) = %q, want VARCHAR2", got)
	}
	if got := rows.ColumnTypeDatabaseTypeName(1); got != "NUMBER" {
		t.Fatalf("ColumnTypeDatabaseTypeName(1) = %q, want NUMBER", got)
	}
	if length, ok := rows.ColumnTypeLength(0); !ok || length != 20 {
		t.Fatalf("ColumnTypeLength(0) = (%d, %v), want (20, true)", length, ok)
	}
	if length, ok := rows.ColumnTypeLength(-1); ok || length != 0 {
		t.Fatalf("ColumnTypeLength(-1) = (%d, %v), want (0, false)", length, ok)
	}
	if nullable, ok := rows.ColumnTypeNullable(0); !ok || !nullable {
		t.Fatalf("ColumnTypeNullable(0) = (%v, %v), want (true, true)", nullable, ok)
	}
	if precision, scale, ok := rows.ColumnTypePrecisionScale(1); !ok || precision != 8 || scale != 0 {
		t.Fatalf("ColumnTypePrecisionScale(1) = (%d, %d, %v), want (8, 0, true)", precision, scale, ok)
	}
	if precision, scale, ok := rows.ColumnTypePrecisionScale(0); ok || precision != 0 || scale != 0 {
		t.Fatalf("ColumnTypePrecisionScale(0) = (%d, %d, %v), want (0, 0, false)", precision, scale, ok)
	}
	dest := make([]driver.Value, 2)
	if err := rows.Next(dest); err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if dest[0] != "abc" || dest[1] != int64(1) {
		t.Fatalf("decoded row = %#v, want abc/int64(1)", dest)
	}
	if err := rows.Next(dest); !errors.Is(err, io.EOF) {
		t.Fatalf("second Next error = %v, want io.EOF", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestTTCRowsColumnTypeDatabaseTypeNameMappings(t *testing.T) {
	tests := []struct {
		name string
		ctx  ColumnContext
		want string
	}{
		{name: "nvarchar", ctx: ColumnContext{DataType: DtyChr, CharsetForm: 2}, want: "NVARCHAR2"},
		{name: "float number", ctx: ColumnContext{DataType: DtyNum, Precision: -127}, want: "FLOAT"},
		{name: "long", ctx: ColumnContext{DataType: DtyLng}, want: "LONG"},
		{name: "date", ctx: ColumnContext{DataType: DtyDat}, want: "DATE"},
		{name: "raw", ctx: ColumnContext{DataType: DtyBin}, want: "RAW"},
		{name: "long raw", ctx: ColumnContext{DataType: DtyLbi}, want: "LONG RAW"},
		{name: "nchar", ctx: ColumnContext{DataType: DtyAfc, CharsetForm: 2}, want: "NCHAR"},
		{name: "char", ctx: ColumnContext{DataType: DtyAfc}, want: "CHAR"},
		{name: "binary float", ctx: ColumnContext{DataType: DtyIbFloat}, want: "BINARY_FLOAT"},
		{name: "binary double", ctx: ColumnContext{DataType: DtyIbDouble}, want: "BINARY_DOUBLE"},
		{name: "cursor", ctx: ColumnContext{DataType: DtyCur}, want: "REFCURSOR"},
		{name: "rowid", ctx: ColumnContext{DataType: DtyRdd}, want: "ROWID"},
		{name: "internal named", ctx: ColumnContext{DataType: DtyINty}, want: "Internal Named Type"},
		{name: "iref", ctx: ColumnContext{DataType: DtyIref}, want: "Internal Named Type"},
		{name: "nclob", ctx: ColumnContext{DataType: DtyClob, CharsetForm: 2}, want: "NCLOB"},
		{name: "clob", ctx: ColumnContext{DataType: DtyClob}, want: "CLOB"},
		{name: "blob", ctx: ColumnContext{DataType: DtyBlob}, want: "BLOB"},
		{name: "bfile", ctx: ColumnContext{DataType: DtyBFil}, want: "BFILE"},
		{name: "json", ctx: ColumnContext{DataType: DtyJSON}, want: "JSON"},
		{name: "vector", ctx: ColumnContext{DataType: DtyVec}, want: "VECTOR"},
		{name: "timestamp", ctx: ColumnContext{DataType: DtyStamp}, want: "TIMESTAMP"},
		{name: "timestamp tz", ctx: ColumnContext{DataType: DtyStz}, want: "TIMESTAMP WITH TIME ZONE"},
		{name: "interval ym", ctx: ColumnContext{DataType: DtyIym}, want: "INTERVALYM"},
		{name: "interval ds", ctx: ColumnContext{DataType: DtyIds}, want: "INTERVALDS"},
		{name: "timestamp ltz", ctx: ColumnContext{DataType: DtySitz}, want: "TIMESTAMP WITH LOCAL TIME ZONE"},
		{name: "boolean", ctx: ColumnContext{DataType: DtyBol}, want: "BOOLEAN"},
		{name: "unknown", ctx: ColumnContext{DataType: DtyType(9999)}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := newTTCRows([]ColumnContext{tt.ctx})
			if got := rows.ColumnTypeDatabaseTypeName(0); got != tt.want {
				t.Fatalf("ColumnTypeDatabaseTypeName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTTCResult(t *testing.T) {
	result := &ttcResult{rowsAffected: 3, shelf: newShelf[common.MessageType]()}
	if got, err := result.RowsAffected(); err != nil || got != 3 {
		t.Fatalf("RowsAffected = (%d, %v), want (3, nil)", got, err)
	}
	if got, err := result.LastInsertId(); err == nil || got != 0 {
		t.Fatalf("LastInsertId = (%d, %v), want unsupported error", got, err)
	}
}
