/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all copyright
** rights in the Software, and under any and all patent rights owned or freely
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

package oracle

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// TestDriver_LongSelect covers materialized LONG bind and fetch behavior.
// Cases:
//   - null: SQL NULL bound and scanned with strict NULL handling.
//   - short: short non-NULL text.
//   - chunked_4k_multibyte: UTF-8 text over 4 KiB.
//   - chunked_2gib_minus_one: payload of 2,147,483,647 bytes (the LONG maximum).
//   - chunked_2gib: one byte over the LONG maximum; round-trip is capped at the maximum.
func TestDriver_LongSelect(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	testConfig := TestingConfig.Clone()
	testConfig.ConnectionProperties.StrictNullValueHandling = "true"
	twoGiBBytes := int64(2) * 1024 * 1024 * 1024

	db, err := openTestDBWithConfig(testConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("select_long")
	createSQL := fmt.Sprintf(`CREATE TABLE %s (
		pk NUMBER(9) NOT NULL PRIMARY KEY,
		long_col LONG
	)`, table)
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		t.Fatalf("create LONG table failed: %v", err)
	}
	t.Cleanup(func() {
		cleanupDB, err := openTestDBWithConfig(testConfig)
		if err != nil {
			if t.Failed() {
				t.Logf("cleanup could not open a fresh connection to drop %s after test failure: %v", table, err)
			} else {
				t.Errorf("cleanup could not open a fresh connection to drop %s: %v", table, err)
			}
			return
		}
		defer cleanupDB.Close()
		if err := dropTable(ctx, cleanupDB, table); err != nil {
			if t.Failed() {
				t.Logf("cleanup drop table %s failed after test failure: %v", table, err)
			} else {
				t.Errorf("cleanup drop table %s failed: %v", table, err)
			}
		}
	})

	tests := []struct {
		name                    string
		description             string
		value                   func() string
		isNull                  bool
		expectMaxLengthTruncate bool
	}{
		{
			name:        "null",
			description: "SQL NULL input scanned as an invalid sql.NullString",
			isNull:      true,
		},
		{
			name:        "short",
			description: "short non-NULL text",
			value:       func() string { return "short LONG value" },
		},
		{
			name:        "chunked_4k_multibyte",
			description: "4 KiB+ UTF-8 text across materialized LONG chunks",
			value:       func() string { return strings.Repeat("LONG-€-", 470) },
		},
		{
			name:        "chunked_2gib_minus_one",
			description: "2,147,483,647-byte LONG maximum payload (2 GiB - 1)",
			value:       func() string { return strings.Repeat("L", int(twoGiBBytes-1)) },
		},
		{
			name:                    "chunked_2gib",
			description:             "2,147,483,648-byte LONG payload; returned value is capped at 2,147,483,647 bytes",
			value:                   func() string { return strings.Repeat("L", int(twoGiBBytes)) },
			expectMaxLengthTruncate: true,
		},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if strconv.IntSize < 64 && i >= len(tests)-2 {
				t.Skip("2 GiB boundary payloads require 64-bit Go")
			}
			var value string
			if !tc.isNull {
				value = tc.value()
			}
			caseDB, err := openTestDBWithConfig(testConfig)
			if err != nil {
				t.Fatalf("open case DB: %v", err)
			}
			defer caseDB.Close()
			insertSQL := fmt.Sprintf("INSERT INTO %s (pk, long_col) VALUES (%d, :payload)", table, i+1)
			var bindValue any = datatype.Long(value)
			if tc.isNull {
				// A typed empty LONG is not SQL NULL; bind nil for this NULL case.
				bindValue = nil
			}
			if _, err := caseDB.ExecContext(ctx, insertSQL, sql.Named("payload", bindValue)); err != nil {
				t.Fatalf("insert LONG row failed: %v", err)
			}

			query := fmt.Sprintf("SELECT long_col FROM %s WHERE pk = %d", table, i+1)
			var got sql.NullString
			if err := caseDB.QueryRowContext(ctx, query).Scan(&got); err != nil {
				t.Fatalf("select/scan LONG failed: %v", err)
			}
			if tc.isNull {
				if got.Valid {
					t.Fatalf("LONG NULL scanned as valid value %q", got.String)
				}
				return
			}
			if !got.Valid {
				t.Fatal("LONG column unexpectedly scanned as NULL")
			}
			if tc.expectMaxLengthTruncate {
				maxLength := int(twoGiBBytes - 1)
				if len(got.String) != maxLength || got.String != value[:maxLength] {
					t.Fatalf("over-limit LONG value = %d bytes, want the first %d bytes", len(got.String), maxLength)
				}
				return
			}
			if got.String != value {
				t.Fatalf("LONG value mismatch: got %d bytes, want %d bytes", len(got.String), len(value))
			}
		})
	}
}

// TestDriver_LongRawSelect covers materialized LONG RAW bind and fetch behavior.
// Cases:
//   - null: SQL NULL bound and scanned as nil with strict NULL handling.
//   - short: binary data with zero and high-bit bytes.
//   - chunked_4k: binary data over 4 KiB.
//   - chunked_2gib_minus_one: payload of 2,147,483,647 bytes (the LONG RAW maximum).
//   - chunked_2gib: one byte over the LONG RAW maximum; round-trip is capped at the maximum.
func TestDriver_LongRawSelect(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	testConfig := TestingConfig.Clone()
	testConfig.ConnectionProperties.StrictNullValueHandling = "true"
	twoGiBBytes := int64(2) * 1024 * 1024 * 1024

	db, err := openTestDBWithConfig(testConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("select_long_raw")
	createSQL := fmt.Sprintf(`CREATE TABLE %s (
		pk NUMBER(9) NOT NULL PRIMARY KEY,
		raw_col LONG RAW
	)`, table)
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		t.Fatalf("create LONG RAW table failed: %v", err)
	}
	t.Cleanup(func() {
		cleanupDB, err := openTestDBWithConfig(testConfig)
		if err != nil {
			if t.Failed() {
				t.Logf("cleanup could not open a fresh connection to drop %s after test failure: %v", table, err)
			} else {
				t.Errorf("cleanup could not open a fresh connection to drop %s: %v", table, err)
			}
			return
		}
		defer cleanupDB.Close()
		if err := dropTable(ctx, cleanupDB, table); err != nil {
			if t.Failed() {
				t.Logf("cleanup drop table %s failed after test failure: %v", table, err)
			} else {
				t.Errorf("cleanup drop table %s failed: %v", table, err)
			}
		}
	})

	makeRawValue := func(size int) []byte {
		value := make([]byte, size)
		for i := range value {
			value[i] = byte(i*31 + 7)
		}
		// Ensure each generated payload covers zero and high-bit bytes.
		value[0] = 0x00
		value[1] = 0x80
		value[2] = 0xFF
		return value
	}
	makeRawBoundaryValue := func(size int) []byte {
		value := make([]byte, size)
		// The zero-filled payload includes zero bytes; set high-bit values at
		// both ends without an O(size) initialization loop.
		value[1] = 0x80
		value[2] = 0xFF
		value[len(value)-1] = 0xFF
		return value
	}
	tests := []struct {
		name                    string
		description             string
		value                   func() []byte
		isNull                  bool
		expectMaxLengthTruncate bool
	}{
		{
			name:        "null",
			description: "SQL NULL input scanned as nil",
			isNull:      true,
		},
		{
			name:        "short",
			description: "short non-NULL binary value with zero and high-bit bytes",
			value:       func() []byte { return []byte{0x00, 0x01, 0x80, 0xFF} },
		},
		{
			name:        "chunked_4k",
			description: "4 KiB+ binary data including zero and high-bit bytes",
			value:       func() []byte { return makeRawValue(4*1024 + 257) },
		},
		{
			name:        "chunked_2gib_minus_one",
			description: "2,147,483,647-byte LONG RAW maximum payload (2 GiB - 1)",
			value:       func() []byte { return makeRawBoundaryValue(int(twoGiBBytes - 1)) },
		},
		{
			name:                    "chunked_2gib",
			description:             "2,147,483,648-byte LONG RAW payload; returned value is capped at 2,147,483,647 bytes",
			value:                   func() []byte { return makeRawBoundaryValue(int(twoGiBBytes)) },
			expectMaxLengthTruncate: true,
		},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if strconv.IntSize < 64 && i >= len(tests)-2 {
				t.Skip("2 GiB boundary payloads require 64-bit Go")
			}
			var value []byte
			if !tc.isNull {
				value = tc.value()
			}
			caseDB, err := openTestDBWithConfig(testConfig)
			if err != nil {
				t.Fatalf("open case DB: %v", err)
			}
			defer caseDB.Close()
			insertSQL := fmt.Sprintf("INSERT INTO %s (pk, raw_col) VALUES (%d, :payload)", table, i+1)
			var bindValue any = datatype.LongRaw(value)
			if tc.isNull {
				// A nil LONG RAW slice is an empty typed payload; bind nil for SQL NULL.
				bindValue = nil
			}
			if _, err := caseDB.ExecContext(ctx, insertSQL, sql.Named("payload", bindValue)); err != nil {
				t.Fatalf("insert LONG RAW row failed: %v", err)
			}

			query := fmt.Sprintf("SELECT raw_col FROM %s WHERE pk = %d", table, i+1)
			if tc.isNull {
				var got any
				if err := caseDB.QueryRowContext(ctx, query).Scan(&got); err != nil {
					t.Fatalf("select/scan LONG RAW NULL failed: %v", err)
				}
				if got != nil {
					t.Fatalf("LONG RAW NULL scanned as %T(%v), want nil", got, got)
				}
				return
			}
			var got []byte
			if err := caseDB.QueryRowContext(ctx, query).Scan(&got); err != nil {
				t.Fatalf("select/scan LONG RAW (%d bytes) failed: %v", len(value), err)
			}
			if tc.expectMaxLengthTruncate {
				maxLength := int(twoGiBBytes - 1)
				if len(got) != maxLength || !bytes.Equal(got, value[:maxLength]) {
					t.Fatalf("over-limit LONG RAW value = %d bytes, want the first %d bytes", len(got), maxLength)
				}
				return
			}
			if !bytes.Equal(got, value) {
				t.Fatalf("LONG RAW value mismatch: got %d bytes, want %d", len(got), len(value))
			}
		})
	}
}

const plsqlLongBaselineLength = 32_760
const plsqlLongMidTestLength = 33_000

type plsqlLongPLSQLTestCase struct {
	length      int
	expectError bool
}

func plsqlLongPLSQLTestCases() []plsqlLongPLSQLTestCase {
	return []plsqlLongPLSQLTestCase{
		{length: plsqlLongBaselineLength},
		{length: plsqlLongMidTestLength, expectError: true},
	}
}

type plsqlLongBindCase struct {
	name       string
	column     string
	columnType string
	value      any
}

func plsqlLongBindCases(length int) []plsqlLongBindCase {
	rawValue := bytes.Repeat([]byte{0x00, 0x80, 0xA5, 0xFF}, (length+3)/4)[:length]
	return []plsqlLongBindCase{
		{
			name:       "LONG",
			column:     "long_value",
			columnType: "LONG",
			value:      datatype.Long(strings.Repeat("L", length)),
		},
		{
			name:       "LONG_RAW",
			column:     "raw_value",
			columnType: "LONG RAW",
			value:      datatype.LongRaw(rawValue),
		},
	}
}

// createPLSQLLongBindTable creates a table for one LONG-family type and
// registers cleanup on a fresh connection so failed OUT-bind tests can clean up.
func createPLSQLLongBindTable(t *testing.T, db *sql.DB, tc plsqlLongBindCase) string {
	t.Helper()
	ctx := context.Background()
	table := createObjectName("plsql_long_bind")
	createSQL := fmt.Sprintf(`CREATE TABLE %s (
		id NUMBER(9) NOT NULL PRIMARY KEY,
		%s %s
	)`, table, tc.column, tc.columnType)
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		t.Fatalf("create %s table failed: %v", tc.columnType, err)
	}
	t.Cleanup(func() {
		// An OUT-bind decode error may leave the execution connection unusable.
		cleanupDB, err := openTestDBWithConfig(TestingConfig)
		if err != nil {
			if t.Failed() {
				t.Logf("open cleanup connection for %s after test failure: %v", table, err)
			} else {
				t.Errorf("open cleanup connection for %s: %v", table, err)
			}
			return
		}
		defer cleanupDB.Close()
		if err := dropTable(ctx, cleanupDB, table); err != nil {
			if t.Failed() {
				t.Logf("drop table %s after test failure: %v", table, err)
			} else {
				t.Errorf("drop table %s: %v", table, err)
			}
		}
	})

	return table
}

// selectLongBindValue verifies that SQL SELECT returns the complete LONG-family
// value inserted by the test.
func selectLongBindValue(t *testing.T, db *sql.DB, table string, tc plsqlLongBindCase) {
	t.Helper()
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = 1", tc.column, table)
	switch want := tc.value.(type) {
	case datatype.Long:
		var got sql.NullString
		if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
			t.Fatalf("SQL SELECT LONG failed: %v", err)
		}
		if !got.Valid || got.String != string(want) {
			t.Fatalf("LONG SELECT mismatch: got %d bytes (valid=%t), want %d bytes", len(got.String), got.Valid, len(want))
		}
	case datatype.LongRaw:
		var got []byte
		if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
			t.Fatalf("SQL SELECT LONG RAW failed: %v", err)
		}
		if !bytes.Equal(got, []byte(want)) {
			t.Fatalf("LONG RAW SELECT mismatch: got %d bytes, want %d bytes", len(got), len(want))
		}
	default:
		t.Fatalf("unsupported LONG bind test value %T", tc.value)
	}
}

// TestDriver_LongPLSQLInsertSQLSelect verifies LONG and LONG RAW PL/SQL INSERT
// behavior at 32,760 bytes and expects an error at 33,000 bytes.
func TestDriver_LongPLSQLInsertSQLSelect(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	for _, testCase := range plsqlLongPLSQLTestCases() {
		length := testCase.length
		for _, tc := range plsqlLongBindCases(length) {
			t.Run(fmt.Sprintf("%s/%d_bytes", tc.name, length), func(t *testing.T) {
				db, err := openTestDBWithConfig(TestingConfig)
				if err != nil {
					t.Fatalf("failed to open test DB: %v", err)
				}
				t.Cleanup(func() { _ = db.Close() })

				ctx := context.Background()
				table := createPLSQLLongBindTable(t, db, tc)
				insertSQL := fmt.Sprintf("BEGIN INSERT INTO %s (id, %s) VALUES (1, :payload); END;", table, tc.column)
				_, insertErr := db.ExecContext(ctx, insertSQL, sql.Named("payload", tc.value))
				if testCase.expectError {
					if insertErr == nil {
						t.Fatalf("PL/SQL INSERT accepted %s value of %d bytes; want an error", tc.name, length)
					}
					t.Logf("PL/SQL INSERT rejected %s value of %d bytes: %v", tc.name, length, insertErr)
					return
				}
				if insertErr != nil {
					t.Fatalf("PL/SQL INSERT %s (%d bytes) failed: %v", tc.name, length, insertErr)
				}
				selectLongBindValue(t, db, table, tc)
			})
		}
	}
}

// TestDriver_LongPLSQLSelectOut verifies that PL/SQL SELECT INTO
// OUT binds round-trip LONG and LONG RAW values at 32,760 bytes and return an
// error when asked to retrieve a 33,000-byte value.
func TestDriver_LongPLSQLSelectOut(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	for _, testCase := range plsqlLongPLSQLTestCases() {
		length := testCase.length
		for _, tc := range plsqlLongBindCases(length) {
			t.Run(fmt.Sprintf("%s/%d_bytes", tc.name, length), func(t *testing.T) {
				db, err := openTestDBWithConfig(TestingConfig)
				if err != nil {
					t.Fatalf("failed to open test DB: %v", err)
				}
				t.Cleanup(func() { _ = db.Close() })

				ctx := context.Background()
				// Seed the row through SQL so this test isolates PL/SQL OUT-bind
				// decoding for both payload sizes.
				table := createPLSQLLongBindTable(t, db, tc)
				insertSQL := fmt.Sprintf("INSERT INTO %s (id, %s) VALUES (1, :payload)", table, tc.column)
				if _, err := db.ExecContext(ctx, insertSQL, sql.Named("payload", tc.value)); err != nil {
					t.Fatalf("SQL INSERT %s (%d bytes) failed: %v", tc.name, length, err)
				}
				selectLongBindValue(t, db, table, tc)

				selectBlock := fmt.Sprintf("BEGIN SELECT %s INTO :selected FROM %s WHERE id = 1; END;",
					tc.column, table)
				switch want := tc.value.(type) {
				case datatype.Long:
					// Seed the destination to the expected output length.
					got := datatype.Long(strings.Repeat("?", len(want)))
					_, selectErr := db.ExecContext(ctx, selectBlock,
						sql.Named("selected", sql.Out{Dest: &got}))
					if testCase.expectError {
						assertPLSQLLongOutError(t, selectErr, tc.name, length)
						return
					}
					if selectErr != nil {
						t.Fatalf("PL/SQL SELECT INTO LONG OUT bind (%d bytes) failed: %v", length, selectErr)
					}
					if got != want {
						t.Fatalf("LONG OUT value mismatch: got %d bytes, want %d bytes", len(got), len(want))
					}
				case datatype.LongRaw:
					// Use a different seed so an unchanged destination cannot pass.
					got := datatype.LongRaw(bytes.Repeat([]byte{0x5A}, len(want)))
					_, selectErr := db.ExecContext(ctx, selectBlock,
						sql.Named("selected", sql.Out{Dest: &got}))
					if testCase.expectError {
						assertPLSQLLongOutError(t, selectErr, tc.name, length)
						return
					}
					if selectErr != nil {
						t.Fatalf("PL/SQL SELECT INTO LONG RAW OUT bind (%d bytes) failed: %v", length, selectErr)
					}
					if !bytes.Equal(got, []byte(want)) {
						t.Fatalf("LONG RAW OUT value mismatch: got %d bytes, want %d bytes", len(got), len(want))
					}
				default:
					t.Fatalf("unsupported LONG bind test value %T", tc.value)
				}
			})
		}
	}
}

func assertPLSQLLongOutError(t *testing.T, err error, typeName string, length int) {
	t.Helper()
	if err == nil {
		t.Fatalf("PL/SQL SELECT INTO accepted %s OUT value of %d bytes; want an error above %d bytes", typeName, length, plsqlLongBaselineLength)
	}
	t.Logf("PL/SQL SELECT INTO rejected %s OUT value of %d bytes: %v", typeName, length, err)
}

// TestDriver_LongBindOrdering verifies named placeholder association when
// TTC sends ordinary bind values before explicitly typed LONG/LONG RAW values.
// LONG and LONG RAW use separate tables because Oracle allows one LONG-family
// column per table.
//
// Test cases:
//   - LONG and LONG RAW placeholders appear before or after an ordinary bind.
//   - Named arguments are supplied in the reverse of placeholder order.
//   - Multi-kilobyte LONG text and LONG RAW bytes round-trip; LONG RAW includes
//     zero and high-bit bytes.
func TestDriver_LongBindOrdering(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()

	orderCases := []struct {
		name          string
		columns       string
		values        string
		argumentOrder []string
		note          string
		after         bool
	}{
		{
			name:          "long_before_ordinary_placeholder",
			columns:       "long_col, note",
			values:        ":value, :note",
			argumentOrder: []string{"note", "value"},
			note:          "long-first",
		},
		{
			name:          "long_after_ordinary_placeholder",
			columns:       "note, long_col",
			values:        ":note, :value",
			argumentOrder: []string{"value", "note"},
			note:          "long-after",
			after:         true,
		},
	}

	payloadCases := []struct {
		name       string
		columnType string
		makeValue  func(after bool) any
		makeBind   func(value any) any
		assertRow  func(t *testing.T, row *sql.Row, wantNote string, want any)
	}{
		{
			name:       "LONG",
			columnType: "LONG",
			makeValue: func(after bool) any {
				value := strings.Repeat("LONG-€-", 800)
				if after {
					value += "-after"
				}
				return value
			},
			makeBind: func(value any) any { return datatype.Long(value.(string)) },
			assertRow: func(t *testing.T, row *sql.Row, wantNote string, want any) {
				t.Helper()
				var gotNote, got string
				if err := row.Scan(&gotNote, &got); err != nil {
					t.Fatalf("select/scan LONG row failed: %v", err)
				}
				if gotNote != wantNote || got != want.(string) {
					t.Fatalf("LONG row mismatch: got (%q, %d bytes), want (%q, %d bytes)",
						gotNote, len(got), wantNote, len(want.(string)))
				}
			},
		},
		{
			name:       "LONG_RAW",
			columnType: "LONG RAW",
			makeValue: func(after bool) any {
				value := make([]byte, 6000)
				for i := range value {
					value[i] = byte(i*31 + 7)
				}
				value[0], value[1], value[2] = 0x00, 0x80, 0xff
				if after {
					value = append(value, 0xfe)
				}
				return value
			},
			makeBind: func(value any) any { return datatype.LongRaw(value.([]byte)) },
			assertRow: func(t *testing.T, row *sql.Row, wantNote string, want any) {
				t.Helper()
				var gotNote string
				var got []byte
				if err := row.Scan(&gotNote, &got); err != nil {
					t.Fatalf("select/scan LONG RAW row failed: %v", err)
				}
				wantRaw := want.([]byte)
				if gotNote != wantNote || !bytes.Equal(got, wantRaw) {
					t.Fatalf("LONG RAW row mismatch: got (%q, %d bytes), want (%q, %d bytes)",
						gotNote, len(got), wantNote, len(wantRaw))
				}
			},
		},
	}

	for _, payloadCase := range payloadCases {
		payloadCase := payloadCase
		t.Run(payloadCase.name, func(t *testing.T) {
			db, err := openTestDBWithConfig(TestingConfig)
			if err != nil {
				t.Fatalf("open test DB failed: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			table := createObjectName("long_bind_order")
			createSQL := fmt.Sprintf(`CREATE TABLE %s (
				note VARCHAR2(80) NOT NULL PRIMARY KEY,
				long_col %s
			)`, table, payloadCase.columnType)
			if _, err := db.ExecContext(ctx, createSQL); err != nil {
				t.Fatalf("create %s table failed: %v", payloadCase.columnType, err)
			}
			t.Cleanup(func() {
				// Close the execution pool before dropping the table, in case a
				// failed protocol exchange left its session holding a table lock.
				_ = db.Close()
				cleanupDB, err := openTestDBWithConfig(TestingConfig)
				if err != nil {
					if t.Failed() {
						t.Logf("cleanup could not open a fresh connection to drop %s after test failure: %v", table, err)
					} else {
						t.Errorf("cleanup could not open a fresh connection to drop %s: %v", table, err)
					}
					return
				}
				defer cleanupDB.Close()
				if err := dropTable(ctx, cleanupDB, table); err != nil {
					if t.Failed() {
						t.Logf("cleanup drop table %s failed after test failure: %v", table, err)
					} else {
						t.Errorf("cleanup drop table %s failed: %v", table, err)
					}
				}
			})

			for _, orderCase := range orderCases {
				orderCase := orderCase
				t.Run(orderCase.name, func(t *testing.T) {
					want := payloadCase.makeValue(orderCase.after)
					values := map[string]any{
						"value": sql.Named("value", payloadCase.makeBind(want)),
						"note":  sql.Named("note", orderCase.note),
					}
					args := make([]any, 0, len(orderCase.argumentOrder))
					for _, name := range orderCase.argumentOrder {
						args = append(args, values[name])
					}

					insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, orderCase.columns, orderCase.values)
					if _, err := db.ExecContext(ctx, insertSQL, args...); err != nil {
						t.Fatalf("insert %s row failed: %v", payloadCase.name, err)
					}

					selectSQL := fmt.Sprintf("SELECT note, long_col FROM %s WHERE note = :note", table)
					row := db.QueryRowContext(ctx, selectSQL, sql.Named("note", orderCase.note))
					payloadCase.assertRow(t, row, orderCase.note, want)
				})
			}
		})
	}
}

// TestDriver_LongClobNclobSelect verifies that LONG, CLOB, and NCLOB columns
// can be selected and scanned together. It creates a table with one row and
// checks that all three sql.NullString destinations contain the inserted text.
func TestDriver_LongClobNclobSelect(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	testConfig := TestingConfig.Clone()
	db, err := openTestDBWithConfig(testConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}

	ctx := context.Background()
	table := createObjectName("select_long_clob_nclob")
	createSQL := fmt.Sprintf(`CREATE TABLE %s (
		pk NUMBER(9) NOT NULL PRIMARY KEY,
		long_col LONG,
		clob_col CLOB,
		nclob_col NCLOB
	)`, table)
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		t.Fatalf("create LONG/CLOB/NCLOB table failed: %v", err)
	}
	t.Cleanup(func() {
		// Close the query pool before dropping the table in case a failed read
		// left its session holding a lock.
		_ = db.Close()
		cleanupDB, err := openTestDBWithConfig(testConfig)
		if err != nil {
			t.Errorf("open cleanup connection: %v", err)
			return
		}
		defer cleanupDB.Close()
		if err := dropTable(ctx, cleanupDB, table); err != nil {
			t.Errorf("drop table %s: %v", table, err)
		}
	})

	const (
		longValue  = "long value"
		clobValue  = "clob value"
		nclobValue = "nclob value"
	)
	insertSQL := fmt.Sprintf(`INSERT INTO %s (pk, long_col, clob_col, nclob_col)
		VALUES (1, '%s', TO_CLOB('%s'), TO_NCLOB(N'%s'))`, table, longValue, clobValue, nclobValue)
	if _, err := db.ExecContext(ctx, insertSQL); err != nil {
		t.Fatalf("insert LONG/CLOB/NCLOB row failed: %v", err)
	}

	query := fmt.Sprintf("SELECT long_col, clob_col, nclob_col FROM %s WHERE pk = 1", table)
	var long, clob, nclob sql.NullString
	if err := db.QueryRowContext(ctx, query).Scan(&long, &clob, &nclob); err != nil {
		t.Fatalf("select/scan LONG, CLOB, and NCLOB failed: %v", err)
	}
	if !long.Valid || long.String != longValue {
		t.Errorf("LONG = (%q, valid=%t), want (%q, valid=true)", long.String, long.Valid, longValue)
	}
	if !clob.Valid || clob.String != clobValue {
		t.Errorf("CLOB = (%q, valid=%t), want (%q, valid=true)", clob.String, clob.Valid, clobValue)
	}
	if !nclob.Valid || nclob.String != nclobValue {
		t.Errorf("NCLOB = (%q, valid=%t), want (%q, valid=true)", nclob.String, nclob.Valid, nclobValue)
	}
}
