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

package oracle

import (
	"bytes"
	"context"
	"testing"
)

// TestInsertAndSelectSmallRAW validates that small binary data (8 bytes) can be inserted and retrieved correctly.
func TestInsertAndSelectSmallRAW(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("t_raw_small")

	// Create table with RAW column
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"data": "RAW(100)",
	}
	if err := createTable(ctx, db, tableName, cols); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer dropTable(ctx, db, tableName)

	// Insert small binary data
	smallData := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0xFF, 0xFE, 0xFD}
	stmt, err := db.PrepareContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (:1, :2)")
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, 1, smallData)
	if err != nil {
		t.Fatalf("failed to insert RAW: %v", err)
	}

	// Select and verify
	var retrievedData []byte
	err = db.QueryRowContext(ctx, "SELECT data FROM "+tableName+" WHERE id = 1").Scan(&retrievedData)
	if err != nil {
		t.Fatalf("failed to select RAW: %v", err)
	}

	if !bytes.Equal(retrievedData, smallData) {
		t.Errorf("RAW mismatch: expected %v, got %v", smallData, retrievedData)
	}

	t.Logf("successfully inserted and retrieved small RAW (%d bytes)", len(retrievedData))
}

// TestInsertAndSelectLargeRAW validates RAW handling at the TTC protocol's maximum bind variable limit.
func TestInsertAndSelectLargeRAW(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("t_raw_large")

	// Create table with RAW(2000) - maximum size for RAW
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"data": "RAW(2000)",
	}
	if err := createTable(ctx, db, tableName, cols); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer dropTable(ctx, db, tableName)

	// Test 252 bytes (maximum TTC protocol limit for RAW bind variables)
	testData := make([]byte, 252)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	stmt, err := db.PrepareContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (:1, :2)")
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, 1, testData)
	if err != nil {
		t.Fatalf("failed to insert 252-byte RAW: %v", err)
	}

	// Select and verify
	var retrievedData []byte
	err = db.QueryRowContext(ctx, "SELECT data FROM "+tableName+" WHERE id = 1").Scan(&retrievedData)
	if err != nil {
		t.Fatalf("failed to select 252-byte RAW: %v", err)
	}

	if !bytes.Equal(retrievedData, testData) {
		t.Errorf("252-byte RAW mismatch: expected length %d, got %d", len(testData), len(retrievedData))
	}

	t.Logf("successfully handled large RAW size (%d bytes)", len(retrievedData))
}

// TestInsertAndSelectNullRAW validates that NULL RAW values are handled correctly.
func TestInsertAndSelectNullRAW(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("t_raw_null")

	// Create table with RAW column
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"data": "RAW(100)",
	}
	if err := createTable(ctx, db, tableName, cols); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer dropTable(ctx, db, tableName)

	// Insert NULL RAW
	_, err = db.ExecContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (1, NULL)")
	if err != nil {
		t.Fatalf("failed to insert NULL RAW: %v", err)
	}

	// Select and verify NULL handling
	var retrievedData []byte
	err = db.QueryRowContext(ctx, "SELECT data FROM "+tableName+" WHERE id = 1").Scan(&retrievedData)
	if err != nil {
		t.Fatalf("failed to select NULL RAW: %v", err)
	}

	if retrievedData != nil {
		t.Errorf("expected NULL RAW (nil), got: %v", retrievedData)
	}

	t.Log("successfully handled NULL RAW")
}

// TestRAWMultipleRows validates that RAW data can be retrieved from multiple rows using rows.Next().
func TestRAWMultipleRows(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("t_raw_multi")

	// Create table with RAW column
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"data": "RAW(100)",
	}
	if err := createTable(ctx, db, tableName, cols); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer dropTable(ctx, db, tableName)

	// Insert multiple rows
	stmt, err := db.PrepareContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (:1, :2)")
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	expectedData := make(map[int][]byte)
	for i := 1; i <= 5; i++ {
		data := make([]byte, i*2)
		for j := range data {
			data[j] = byte(i * 10)
		}
		expectedData[i] = data

		_, err = stmt.ExecContext(ctx, i, data)
		if err != nil {
			t.Fatalf("failed to insert row %d: %v", i, err)
		}
	}

	// Select all rows
	rows, err := db.QueryContext(ctx, "SELECT id, data FROM "+tableName+" ORDER BY id")
	if err != nil {
		t.Fatalf("failed to query RAW rows: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}

		expected := expectedData[id]
		if !bytes.Equal(data, expected) {
			t.Errorf("RAW mismatch for id=%d: expected %v, got %v", id, expected, data)
		}

		count++
		t.Logf("Row %d: RAW=%v (%d bytes)", id, data, len(data))
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration error: %v", err)
	}

	if count != 5 {
		t.Errorf("expected 5 rows, got %d", count)
	}

	t.Logf("successfully retrieved %d RAW rows", count)
}

// TestRAWUpdateOperation validates that RAW column values can be updated with new binary data.
func TestRAWUpdateOperation(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("t_raw_update")

	// Create table with RAW column
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"data": "RAW(100)",
	}
	if err := createTable(ctx, db, tableName, cols); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer dropTable(ctx, db, tableName)

	// Insert initial data
	initialData := []byte{0x01, 0x02, 0x03}
	_, err = db.ExecContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (:1, :2)", 1, initialData)
	if err != nil {
		t.Fatalf("failed to insert initial data: %v", err)
	}

	// Update with new data
	updatedData := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE}
	result, err := db.ExecContext(ctx, "UPDATE "+tableName+" SET data = :1 WHERE id = :2", updatedData, 1)
	if err != nil {
		t.Fatalf("failed to update RAW: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	t.Logf("rows affected by update: %d", rowsAffected)

	// Verify update
	var retrievedData []byte
	err = db.QueryRowContext(ctx, "SELECT data FROM "+tableName+" WHERE id = 1").Scan(&retrievedData)
	if err != nil {
		t.Fatalf("failed to verify update: %v", err)
	}

	if !bytes.Equal(retrievedData, updatedData) {
		t.Errorf("RAW mismatch after update: expected %v, got %v", updatedData, retrievedData)
	}

	t.Logf("successfully updated RAW from %v to %v", initialData, updatedData)
}

// TestRAWTypeSystemIntegration validates the driver's type system integration for RAW.
func TestRAWTypeSystemIntegration(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("t_raw_types")

	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"data": "RAW(100)",
	}
	if err := createTable(ctx, db, tableName, cols); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer dropTable(ctx, db, tableName)

	// Test different []byte slice types that should all use EncodeBinary
	testCases := []struct {
		name string
		data []byte
	}{
		{"single_byte", []byte{0x42}},
		{"zero_bytes", []byte{0x00, 0x00}},
		{"high_bytes", []byte{0xFF, 0xFE}},
		{"small_data", []byte{0x01, 0x02, 0x03, 0x04}},
	}

	stmt, err := db.PrepareContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (:1, :2)")
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	for i, tc := range testCases {
		// Insert - this exercises the driver's EncodeBinary path
		_, err = stmt.ExecContext(ctx, i+1, tc.data)
		if err != nil {
			t.Fatalf("failed to insert %s: %v", tc.name, err)
		}

		// Select - this exercises the driver's DecodeBinaryColumn path
		var retrieved []byte
		err = db.QueryRowContext(ctx, "SELECT data FROM "+tableName+" WHERE id = :1", i+1).Scan(&retrieved)
		if err != nil {
			t.Fatalf("failed to select %s: %v", tc.name, err)
		}

		// Verify the driver's pass-through behavior
		if !bytes.Equal(retrieved, tc.data) {
			t.Errorf("%s: expected %v, got %v", tc.name, tc.data, retrieved)
		}

		t.Logf("%s: driver correctly handled %d bytes", tc.name, len(retrieved))
	}
}

func TestIssue_DecodeBinaryColumnType(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	tableName := createObjectName("issue_raw_scan")
	createTable(ctx, db, tableName, map[string]string{
		"id":   "INTEGER PRIMARY KEY",
		"data": "RAW(16)",
	})
	defer dropTable(ctx, db, tableName)

	blob := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}
	db.ExecContext(ctx, "INSERT INTO "+tableName+" (id, data) VALUES (:1, :2)", 1, blob)

	// Scan into *string — standard database/sql usage for text-safe binary.
	var got string
	err = db.QueryRowContext(ctx,
		"SELECT data FROM "+tableName+" WHERE id = :1", 1).Scan(&got)
	if err != nil {
		t.Fatalf("Scan into *string failef: %v", err)
	}

	// Scan into interface{} — verify the concrete driver.Value type is plain []byte.
	rows, err := db.QueryContext(ctx,
		"SELECT data FROM "+tableName+" WHERE id = :1", 1)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		var raw interface{}
		rows.Scan(&raw)
		if _, ok := raw.([]byte); !ok {
			t.Errorf("driver.Value type = %T, want []byte — "+
				"named type alias breaks all caller type assertions", raw)
		}
	}
}
