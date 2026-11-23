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
** is contributed to or provided by such licensors),
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

package oracle

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestDriver_Vector_Basic verifies INSERT, SELECT, UPDATE, and DELETE for all
// dense VECTOR element types supported by the driver.
func TestDriver_Vector_Basic(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 23 {
		t.Skip("VECTOR datatype requires Oracle Database 23c+")
	}

	ctx := context.Background()
	const dimensions = 1024
	values := make(datatype.VectorFloat64, dimensions)
	updatedValues := make(datatype.VectorFloat64, dimensions)
	for i := range values {
		values[i] = float64(i) / 4
		updatedValues[i] = -float64(i) / 8
	}

	tests := []struct {
		name        string
		table       string
		definition  string
		insert      any
		want        any
		update      any
		updatedWant any
	}{
		{"float64", "vector_f64", "VECTOR(1024, FLOAT64)", values, []float64(values), updatedValues, []float64(updatedValues)},
		{"float32", "vector_f32", "VECTOR(3, FLOAT32)", datatype.VectorFloat32{4.5, -5.25, 6.75}, []float32{4.5, -5.25, 6.75}, datatype.VectorFloat32{-1.5, 2.25, 9.75}, []float32{-1.5, 2.25, 9.75}},
		{"int8", "vector_i8", "VECTOR(3, INT8)", datatype.VectorInt8{1, -2, 3}, []int8{1, -2, 3}, datatype.VectorInt8{-8, 0, 7}, []int8{-8, 0, 7}},
		{"binary", "vector_binary", "VECTOR(16, BINARY)", datatype.VectorBinary{0xAA, 0x0F}, []byte{0xAA, 0x0F}, datatype.VectorBinary{0x55, 0xF0}, []byte{0x55, 0xF0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := openTestDBWithConfig(TestingConfig)
			if err != nil {
				t.Fatalf("open test database: %v", err)
			}
			defer db.Close()

			_ = dropTable(ctx, db, tc.table)
			if err := createTable(ctx, db, tc.table, map[string]string{"id": "NUMBER PRIMARY KEY", "vec": tc.definition}); err != nil {
				t.Fatalf("create table: %v", err)
			}
			defer func() { _ = dropTable(ctx, db, tc.table) }()

			if result, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, vec) VALUES (:1, :2)", tc.table), 1, tc.insert); err != nil {
				t.Fatalf("insert: %v", err)
			} else if count, err := result.RowsAffected(); err != nil || count != 1 {
				t.Fatalf("insert rows affected: got %d, err=%v", count, err)
			}

			assertVector := func(want any) {
				var got any
				switch want.(type) {
				case []float64:
					got = new([]float64)
				case []float32:
					got = new([]float32)
				case []int8:
					got = new([]int8)
				case []byte:
					got = new([]byte)
				}
				if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT vec FROM %s WHERE id = :1", tc.table), 1).Scan(got); err != nil {
					t.Fatalf("fetch: %v", err)
				}
				if !reflect.DeepEqual(reflect.ValueOf(got).Elem().Interface(), want) {
					t.Fatalf("vector mismatch: got %v want %v", reflect.ValueOf(got).Elem().Interface(), want)
				}
			}
			assertVector(tc.want)

			if result, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET vec = :1 WHERE id = :2", tc.table), tc.update, 1); err != nil {
				t.Fatalf("update: %v", err)
			} else if count, err := result.RowsAffected(); err != nil || count != 1 {
				t.Fatalf("update rows affected: got %d, err=%v", count, err)
			}
			assertVector(tc.updatedWant)

			if result, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = :1", tc.table), 1); err != nil {
				t.Fatalf("delete: %v", err)
			} else if count, err := result.RowsAffected(); err != nil || count != 1 {
				t.Fatalf("delete rows affected: got %d, err=%v", count, err)
			}
		})
	}
}
