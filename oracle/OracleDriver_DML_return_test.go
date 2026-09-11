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
	"database/sql"
	"testing"
	"time"
)

// TestDriver_DMLReturning_Insert_Ordinal verifies INSERT ... RETURNING with positional binds.
// Expectation: inserted row values are returned into OUT binds and the row is persisted.
func TestDriver_DMLReturning_Insert_Ordinal(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_ins_ord")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	stmt, err := db.PrepareContext(ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING id, name INTO :na, :nb")
	if err != nil {
		t.Fatalf("prepare insert returning failed: %v", err)
	}
	defer stmt.Close()

	var returnedID int64
	var returnedName string
	result, err := stmt.ExecContext(
		ctx,
		int64(101),
		"insert-returning", // oacmxl = 16
		sql.Named("na", sql.Out{Dest: &returnedID}),   // oacmxl = 32768
		sql.Named("nb", sql.Out{Dest: &returnedName}), // oacmxl = 22
	)
	if err != nil {
		t.Fatalf("exec insert returning failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 101 {
		t.Fatalf("unexpected returned id: got %d, want 101", returnedID)
	}
	if returnedName != "insert-returning" {
		t.Fatalf("unexpected returned name: got %q, want %q", returnedName, "insert-returning")
	}

	var persistedID int64
	var persistedName string
	if err := db.QueryRowContext(ctx,
		"SELECT id, name FROM "+table+" WHERE id = :1",
		int64(101),
	).Scan(&persistedID, &persistedName); err != nil {
		t.Fatalf("select persisted row failed: %v", err)
	}
	if persistedID != returnedID || persistedName != returnedName {
		t.Fatalf("persisted row mismatch: got (%d, %q), want (%d, %q)",
			persistedID, persistedName, returnedID, returnedName)
	}
}

// TestDriver_DMLReturning_Update_Named verifies UPDATE ... RETURNING with named binds.
// Expectation: updated row values are returned into OUT binds and the change is persisted.
func TestDriver_DMLReturning_Update_Named(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_upd_named")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	if _, err := db.ExecContext(ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2)",
		int64(202),
		"before-update",
	); err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}

	stmt, err := db.PrepareContext(ctx,
		"UPDATE "+table+" SET name = :new_name WHERE id = :id RETURNING id, name INTO :out_id, :out_name")
	if err != nil {
		t.Fatalf("prepare update returning failed: %v", err)
	}
	defer stmt.Close()

	var returnedID int64
	var returnedName string
	result, err := stmt.ExecContext(
		ctx,
		sql.Named("new_name", "after-update"),
		sql.Named("id", int64(202)),
		sql.Named("out_id", sql.Out{Dest: &returnedID}),
		sql.Named("out_name", sql.Out{Dest: &returnedName}),
	)
	if err != nil {
		t.Fatalf("exec update returning failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 202 {
		t.Fatalf("unexpected returned id: got %d, want 202", returnedID)
	}
	if returnedName != "after-update" {
		t.Fatalf("unexpected returned name: got %q, want %q", returnedName, "after-update")
	}

	var persistedName string
	if err := db.QueryRowContext(ctx,
		"SELECT name FROM "+table+" WHERE id = :1",
		int64(202),
	).Scan(&persistedName); err != nil {
		t.Fatalf("select updated row failed: %v", err)
	}
	if persistedName != returnedName {
		t.Fatalf("persisted name mismatch: got %q, want %q", persistedName, returnedName)
	}
}

// TestDriver_DMLReturning_Delete_Ordinal verifies DELETE ... RETURNING with positional binds.
// Expectation: deleted row values are returned into OUT binds and the row is removed.
func TestDriver_DMLReturning_Insert_MultipleScalarTypes(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_ins_multi_scalar")
	cols := map[string]string{
		"id":         "number",
		"name":       "varchar2(100)",
		"amount":     "binary_double",
		"is_active":  "number(1)",
		"created_on": "date",
		"created_at": "timestamp",
		"updated_at": "timestamp with time zone",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	expectedDate := time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)
	expectedTimestamp := time.Date(2025, 4, 8, 12, 34, 56, 0, time.UTC)
	expectedTSTZ := time.Date(2025, 4, 8, 18, 45, 12, 0, time.FixedZone("IST", 5*3600+30*60))

	stmt, err := db.PrepareContext(ctx,
		"INSERT INTO "+table+" (id, name, amount, is_active, created_on, created_at, updated_at) "+
			"VALUES (:1, :2, :3, :4, :5, :6, :7) "+
			"RETURNING id, name, amount, is_active, created_on, created_at, updated_at INTO :8, :9, :10, :11, :12, :13, :14")
	if err != nil {
		t.Fatalf("prepare insert returning failed: %v", err)
	}
	defer stmt.Close()

	var returnedID int64
	var returnedName string
	var returnedAmount float64
	var returnedIsActive int64
	var returnedCreatedOn time.Time
	var returnedCreatedAt time.Time
	var returnedUpdatedAt time.Time

	result, err := stmt.ExecContext(
		ctx,
		int64(404),
		"multi-scalar",
		123.75,
		int64(1),
		expectedDate,
		expectedTimestamp,
		expectedTSTZ,
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedName},
		sql.Out{Dest: &returnedAmount},
		sql.Out{Dest: &returnedIsActive},
		sql.Out{Dest: &returnedCreatedOn},
		sql.Out{Dest: &returnedCreatedAt},
		sql.Out{Dest: &returnedUpdatedAt},
	)
	if err != nil {
		t.Fatalf("exec insert returning with multiple scalar types failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 404 {
		t.Fatalf("unexpected returned id: got %d, want 404", returnedID)
	}
	if returnedName != "multi-scalar" {
		t.Fatalf("unexpected returned name: got %q, want %q", returnedName, "multi-scalar")
	}
	if returnedAmount != 123.75 {
		t.Fatalf("unexpected returned amount: got %v, want %v", returnedAmount, 123.75)
	}
	if returnedIsActive != 1 {
		t.Fatalf("unexpected returned is_active: got %d, want 1", returnedIsActive)
	}
	assertSameYMDHMSNanos(t, 0, "DATE", returnedCreatedOn, expectedDate)
	assertSameYMDHMSNanos(t, 0, "TIMESTAMP", returnedCreatedAt, expectedTimestamp)
	assertSameYMDHMSNanos(t, 0, "TSTZ", returnedUpdatedAt, expectedTSTZ)
	if _, off := returnedUpdatedAt.Zone(); off != 5*3600+30*60 {
		t.Fatalf("unexpected returned updated_at zone offset: got %d want %d", off, 5*3600+30*60)
	}

	var persistedID int64
	var persistedName string
	var persistedAmount float64
	var persistedIsActive int64
	var persistedCreatedOn time.Time
	var persistedCreatedAt time.Time
	var persistedUpdatedAt time.Time
	if err := db.QueryRowContext(ctx,
		"SELECT id, name, amount, is_active, created_on, created_at, updated_at FROM "+table+" WHERE id = :1",
		int64(404),
	).Scan(&persistedID, &persistedName, &persistedAmount, &persistedIsActive, &persistedCreatedOn, &persistedCreatedAt, &persistedUpdatedAt); err != nil {
		t.Fatalf("select persisted row failed: %v", err)
	}

	if persistedID != returnedID ||
		persistedName != returnedName ||
		persistedAmount != returnedAmount ||
		persistedIsActive != returnedIsActive {
		t.Fatalf(
			"persisted row mismatch: got (%d, %q, %v, %d), want (%d, %q, %v, %d)",
			persistedID, persistedName, persistedAmount, persistedIsActive,
			returnedID, returnedName, returnedAmount, returnedIsActive,
		)
	}
	assertSameYMDHMSNanos(t, 0, "DATE persisted", persistedCreatedOn, returnedCreatedOn)
	assertSameYMDHMSNanos(t, 0, "TIMESTAMP persisted", persistedCreatedAt, returnedCreatedAt)
	assertSameYMDHMSNanos(t, 0, "TSTZ persisted", persistedUpdatedAt, returnedUpdatedAt)
	if _, off := persistedUpdatedAt.Zone(); off != 5*3600+30*60 {
		t.Fatalf("unexpected persisted updated_at zone offset: got %d want %d", off, 5*3600+30*60)
	}
}

func TestDriver_DMLReturning_Delete_Ordinal(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_del_ord")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	if _, err := db.ExecContext(ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2)",
		int64(303),
		"delete-me",
	); err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}

	stmt, err := db.PrepareContext(ctx,
		"DELETE FROM "+table+" WHERE id = :1 RETURNING id, name INTO :2, :3")
	if err != nil {
		t.Fatalf("prepare delete returning failed: %v", err)
	}
	defer stmt.Close()

	var returnedID int64
	var returnedName string
	result, err := stmt.ExecContext(
		ctx,
		int64(303),
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedName},
	)
	if err != nil {
		t.Fatalf("exec delete returning failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 303 {
		t.Fatalf("unexpected returned id: got %d, want 303", returnedID)
	}
	if returnedName != "delete-me" {
		t.Fatalf("unexpected returned name: got %q, want %q", returnedName, "delete-me")
	}

	var remaining int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+table+" WHERE id = :1",
		int64(303),
	).Scan(&remaining); err != nil {
		t.Fatalf("count deleted row failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected deleted row to be absent, remaining count=%d", remaining)
	}
}

func TestDriver_DMLReturning_Update_Named_NoStmt(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_upd_named_nostmt")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	if _, err := db.ExecContext(ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2)",
		int64(202),
		"before-update",
	); err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}

	var returnedID int64
	var returnedName string
	result, err := db.ExecContext(
		ctx,
		"UPDATE "+table+" SET name = :new_name WHERE id = :id RETURNING id, name INTO :out_id, :out_name",
		sql.Named("new_name", "after-update"),
		sql.Named("id", int64(202)),
		sql.Named("out_id", sql.Out{Dest: &returnedID}),
		sql.Named("out_name", sql.Out{Dest: &returnedName}),
	)
	if err != nil {
		t.Fatalf("exec update returning failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 202 {
		t.Fatalf("unexpected returned id: got %d, want 202", returnedID)
	}
	if returnedName != "after-update" {
		t.Fatalf("unexpected returned name: got %q, want %q", returnedName, "after-update")
	}

	var persistedName string
	if err := db.QueryRowContext(ctx,
		"SELECT name FROM "+table+" WHERE id = :1",
		int64(202),
	).Scan(&persistedName); err != nil {
		t.Fatalf("select updated row failed: %v", err)
	}
	if persistedName != returnedName {
		t.Fatalf("persisted name mismatch: got %q, want %q", persistedName, returnedName)
	}
}

// TestDriver_DMLReturning_ZeroRowsAffected verifies that UPDATE ... RETURNING
// where the WHERE clause matches no existing rows returns rowsAffected == 0
// and leaves the OUT bind destinations at their initial zero values.
// This tests the zero-iteration RETURNING path: the server sends 0 rows for
// each RETURNING position, so no assignment into out-destinations occurs.
func TestDriver_DMLReturning_ZeroRowsAffected(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_zero_rows")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Table is empty - UPDATE will match nothing.
	var returnedID int64 = -1
	var returnedName string = "sentinel"

	result, err := db.ExecContext(
		ctx,
		"UPDATE "+table+" SET name = :new_name WHERE id = :id RETURNING id, name INTO :out_id, :out_name",
		sql.Named("new_name", "should-not-appear"),
		sql.Named("id", int64(999)),
		sql.Named("out_id", sql.Out{Dest: &returnedID}),
		sql.Named("out_name", sql.Out{Dest: &returnedName}),
	)
	if err != nil {
		t.Fatalf("exec update returning (no match) failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 0 {
		t.Fatalf("expected rowsAffected=0 for no-match UPDATE, got %d", rowsAffected)
	}

	// OUT destinations must not have been modified because no row was returned.
	if returnedID != -1 {
		t.Errorf("returnedID should be unchanged (-1), got %d", returnedID)
	}
	if returnedName != "sentinel" {
		t.Errorf("returnedName should be unchanged (sentinel), got %q", returnedName)
	}
}

// TestDriver_DMLReturning_Delete_ZeroRowsAffected verifies DELETE ... RETURNING
// where no rows match the WHERE clause: rowsAffected == 0 and OUT binds untouched.
func TestDriver_DMLReturning_Delete_ZeroRowsAffected(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_del_zero")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	var returnedID int64 = -1
	var returnedName string = "sentinel"

	result, err := db.ExecContext(
		ctx,
		"DELETE FROM "+table+" WHERE id = :1 RETURNING id, name INTO :2, :3",
		int64(777),
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedName},
	)
	if err != nil {
		t.Fatalf("exec delete returning (no match) failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 0 {
		t.Fatalf("expected rowsAffected=0, got %d", rowsAffected)
	}
	if returnedID != -1 {
		t.Errorf("returnedID should be unchanged (-1), got %d", returnedID)
	}
	if returnedName != "sentinel" {
		t.Errorf("returnedName should be unchanged, got %q", returnedName)
	}
}

// TestDriver_DMLReturning_Insert_RAW verifies INSERT ... RETURNING with a RAW column.
// Expectation: the raw binary bytes inserted are returned intact into a []byte OUT bind.
func TestDriver_DMLReturning_Insert_RAW(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_ins_raw")
	cols := map[string]string{
		"id":   "number",
		"data": "raw(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	rawData := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03}

	var returnedID int64
	var returnedData []byte

	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, data) VALUES (:1, :2) RETURNING id, data INTO :3, :4",
		int64(501),
		rawData,
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedData},
	)
	if err != nil {
		t.Fatalf("exec insert returning RAW failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 501 {
		t.Errorf("returnedID mismatch: got %d, want 501", returnedID)
	}
	if !bytes.Equal(returnedData, rawData) {
		t.Errorf("returnedData mismatch: got %v, want %v", returnedData, rawData)
	}

	// Confirm persistence.
	var persistedID int64
	var persistedData []byte
	if err := db.QueryRowContext(ctx,
		"SELECT id, data FROM "+table+" WHERE id = :1",
		int64(501),
	).Scan(&persistedID, &persistedData); err != nil {
		t.Fatalf("select persisted row failed: %v", err)
	}
	if !bytes.Equal(persistedData, returnedData) {
		t.Errorf("persisted RAW mismatch: got %v, want %v", persistedData, returnedData)
	}
}

// TestDriver_DMLReturning_Insert_CHAR verifies INSERT ... RETURNING with a fixed-length CHAR column.
// Oracle pads CHAR values with spaces to the declared width, so the returned string should
// match the padded form.
func TestDriver_DMLReturning_Insert_CHAR(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_ins_char")
	cols := map[string]string{
		"id":   "number",
		"code": "char(10)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	var returnedID int64
	var returnedCode string

	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, code) VALUES (:1, :2) RETURNING id, code INTO :3, :4",
		int64(601),
		"ABC",
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedCode},
	)
	if err != nil {
		t.Fatalf("exec insert returning CHAR failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if returnedID != 601 {
		t.Errorf("returnedID mismatch: got %d, want 601", returnedID)
	}
	// Oracle pads CHAR(10) to 10 chars; the first 3 chars must be "ABC".
	if len(returnedCode) < 3 || returnedCode[:3] != "ABC" {
		t.Errorf("returnedCode should start with 'ABC', got %q", returnedCode)
	}
	// Must be exactly 10 characters (CHAR(10) is fixed width).
	if len(returnedCode) != 10 {
		t.Errorf("returnedCode should be length 10 (CHAR(10)), got %d: %q", len(returnedCode), returnedCode)
	}
}

// TestDriver_DMLReturning_PreparedStmt_ReExecution verifies that re-executing a prepared
// statement with RETURNING works correctly across multiple executions.
// This exercises the OAC caching logic (needToSendOACs / getMaxLengthForOac):
// - First execution: OACs sent unconditionally.
// - Second execution with same type/length: OACs suppressed (re-uses server cursor).
// - Third execution with a longer string: OACs resent because maxLength increased.
func TestDriver_DMLReturning_PreparedStmt_ReExecution(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_reexec")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(200)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	stmt, err := db.PrepareContext(ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING id, name INTO :3, :4")
	if err != nil {
		t.Fatalf("prepare insert returning failed: %v", err)
	}
	defer stmt.Close()

	execAndVerify := func(id int64, name string) {
		var retID int64
		var retName string
		result, err := stmt.ExecContext(
			ctx,
			id,
			name,
			sql.Out{Dest: &retID},
			sql.Out{Dest: &retName},
		)
		if err != nil {
			t.Fatalf("exec insert returning failed (id=%d name=%q): %v", id, name, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("rows affected retrieval failed: %v", err)
		}
		if rowsAffected != 1 {
			t.Fatalf("expected rowsAffected=1 (id=%d), got %d", id, rowsAffected)
		}
		if retID != id {
			t.Errorf("returnedID mismatch (exec id=%d): got %d", id, retID)
		}
		if retName != name {
			t.Errorf("returnedName mismatch (exec id=%d): got %q, want %q", id, retName, name)
		}
	}

	// First execution: OACs are sent.
	execAndVerify(1001, "short")

	// Second execution: same type/length -> OACs are suppressed via caching.
	execAndVerify(1002, "same")

	// Third execution: longer name -> OAC maxLength increases, OACs resent.
	execAndVerify(1003, "this-is-a-considerably-longer-name-that-forces-a-new-oac")

	// Confirm all three rows persisted.
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 persisted rows, got %d", count)
	}
}

// TestDriver_DMLReturning_InTransaction_Rollback verifies that DML RETURNING inside
// an explicit transaction is NOT auto-committed and can be rolled back.
// The OUT bind should receive the returned value, but after rollback the row must
// not appear in the database.
func TestDriver_DMLReturning_InTransaction_Rollback(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_tx_rollback")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx failed: %v", err)
	}

	var returnedID int64
	var returnedName string

	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING id, name INTO :3, :4",
		int64(701),
		"will-be-rolled-back",
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedName},
	)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		t.Fatalf("exec insert returning in tx failed: %v", err)
	}

	// OUT binds must have been populated before commit/rollback.
	if returnedID != 701 {
		t.Errorf("returnedID inside tx: got %d, want 701", returnedID)
	}
	if returnedName != "will-be-rolled-back" {
		t.Errorf("returnedName inside tx: got %q, want %q", returnedName, "will-be-rolled-back")
	}

	// Roll back the transaction.
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// The row must NOT be present after rollback.
	var remaining int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+table+" WHERE id = :1",
		int64(701),
	).Scan(&remaining); err != nil {
		t.Fatalf("count query after rollback failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", remaining)
	}
}

// TestDriver_DMLReturning_InTransaction_Commit verifies that DML RETURNING inside
// an explicit transaction is correctly committed when tx.Commit() is called.
func TestDriver_DMLReturning_InTransaction_Commit(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_tx_commit")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx failed: %v", err)
	}

	var returnedID int64
	var returnedName string

	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING id, name INTO :3, :4",
		int64(702),
		"committed",
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedName},
	)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		t.Fatalf("exec insert returning in tx failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	if returnedID != 702 {
		t.Errorf("returnedID: got %d, want 702", returnedID)
	}

	// Row must be visible after commit.
	var persistedName string
	if err := db.QueryRowContext(ctx,
		"SELECT name FROM "+table+" WHERE id = :1",
		int64(702),
	).Scan(&persistedName); err != nil {
		t.Fatalf("select after commit failed: %v", err)
	}
	if persistedName != returnedName {
		t.Errorf("persisted name: got %q, want %q", persistedName, returnedName)
	}
}

// TestDriver_DMLReturning_Insert_InOut verifies INSERT with an IN OUT bind (sql.Out{In: true}).
// An IN OUT bind contributes its value as a regular input bind AND designates the destination
// for the RETURNING output. The returned value must reflect what Oracle actually stored.
func TestDriver_DMLReturning_Insert_InOut(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_inout")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	returnedName := "inout-value"
	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING name INTO :3",
		int64(801),
		"inout-value",
		sql.Out{Dest: &returnedName, In: true},
	)
	if err != nil {
		t.Fatalf("exec insert returning with IN OUT failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}
	if returnedName != "inout-value" {
		t.Errorf("returnedName: got %q, want %q", returnedName, "inout-value")
	}
}

// TestDriver_DMLReturning_Update_MultipleRows verifies UPDATE ... RETURNING when the WHERE
// clause matches multiple rows. The driver must process the multi-row server response; the
// destination OUT bind receives the value from the last matched row.
//
// Note: Oracle delivers one RETURNING payload per matched row when multiple rows are
// updated; the driver currently assigns each row's value in sequence, so the destination
// ends up holding the last returned value. This test documents that behaviour.
func TestDriver_DMLReturning_Update_MultipleRows(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_upd_multi")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Seed 3 rows.
	for _, id := range []int64{11, 12, 13} {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO "+table+" (id, name) VALUES (:1, :2)",
			id, "old-name",
		); err != nil {
			t.Fatalf("seed insert failed (id=%d): %v", id, err)
		}
	}

	var returnedName string
	result, err := db.ExecContext(
		ctx,
		"UPDATE "+table+" SET name = :new_name RETURNING name INTO :out_name",
		sql.Named("new_name", "new-name"),
		sql.Named("out_name", sql.Out{Dest: &returnedName}),
	)
	if err != nil {
		t.Fatalf("exec update returning (multi-row) failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 3 {
		t.Fatalf("expected rowsAffected=3, got %d", rowsAffected)
	}

	// OUT bind holds the last returned value from Oracle's iteration.
	if returnedName != "new-name" {
		t.Errorf("returnedName: got %q, want %q", returnedName, "new-name")
	}

	// All rows should have been updated.
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+table+" WHERE name = :1",
		"new-name",
	).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 rows with new-name, got %d", count)
	}
}

// TestDriver_DMLReturning_Insert_NullableColumn verifies INSERT ... RETURNING where a
// nullable column is returned as NULL (by inserting NULL into the column explicitly).
// When RETURNING into a plain string destination, the current driver behavior maps
// SQL NULL to the string zero value (""). The test separately verifies that the
// persisted database value is actually NULL using sql.NullString.
func TestDriver_DMLReturning_Insert_NullableColumn(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_null_col")
	cols := map[string]string{
		"id":      "number",
		"name":    "varchar2(100)",
		"remarks": "varchar2(200)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	var returnedID int64
	var returnedComment string

	// Insert with NULL remarks.
	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, name, remarks) VALUES (:1, :2, NULL) RETURNING id, remarks INTO :3, :4",
		int64(901),
		"has-null-remarks",
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedComment},
	)
	if err != nil {
		t.Fatalf("exec insert returning (null column) failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}
	if returnedID != 901 {
		t.Errorf("returnedID: got %d, want 901", returnedID)
	}
	// Plain string destinations currently receive the string zero value for SQL NULL.
	if returnedComment != "" {
		t.Errorf("returnedComment: got %q, want empty string for NULL RETURNING into string", returnedComment)
	}

	// Confirm the stored column value is actually SQL NULL.
	var persistedRemarks sql.NullString
	if err := db.QueryRowContext(ctx,
		"SELECT remarks FROM "+table+" WHERE id = :1",
		int64(901),
	).Scan(&persistedRemarks); err != nil {
		t.Fatalf("select remarks failed: %v", err)
	}
	if persistedRemarks.Valid {
		t.Errorf("expected persisted remarks to be NULL, got %q", persistedRemarks.String)
	}
}

// TestDriver_DMLReturning_Insert_NullBinaryFloatIntoNullFloat64 verifies that a NULL
// BINARY_FLOAT returned into sql.NullFloat64 remains invalid when strict null handling
// is enabled. This covers nullable numeric RETURNING semantics separately from NaN and
// ordinary finite float round trips.
func TestDriver_DMLReturning_Insert_NullBinaryFloatIntoNullFloat64(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "true"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("t_dml_ret_null_bf")
	cols := map[string]string{
		"id":     "number",
		"amount": "binary_float",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	var returnedID sql.NullInt64
	var returnedAmount sql.NullFloat64

	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, amount) VALUES (:1, NULL) RETURNING id, amount INTO :2, :3",
		int64(902),
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedAmount},
	)
	if err != nil {
		t.Fatalf("exec insert returning NULL BINARY_FLOAT failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}
	if !returnedID.Valid || returnedID.Int64 != 902 {
		t.Fatalf("unexpected returned id: %+v", returnedID)
	}
	if returnedAmount.Valid {
		t.Fatalf("expected returned amount to be NULL, got %+v", returnedAmount)
	}

	var persistedAmount sql.NullFloat64
	if err := db.QueryRowContext(ctx,
		"SELECT amount FROM "+table+" WHERE id = :1",
		int64(902),
	).Scan(&persistedAmount); err != nil {
		t.Fatalf("select persisted NULL BINARY_FLOAT failed: %v", err)
	}
	if persistedAmount.Valid {
		t.Fatalf("expected persisted amount to be NULL, got %+v", persistedAmount)
	}
}

// TestDriver_DMLReturning_Insert_TimestampWithLocalTZ verifies RETURNING a
// TIMESTAMP WITH LOCAL TIME ZONE column. This uses a different wire encoding path
// (DtySitz) compared to TIMESTAMP WITH TIME ZONE (DtyStz).
func TestDriver_DMLReturning_Insert_TimestampWithLocalTZ(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_tstz_local")
	cols := map[string]string{
		"id":         "number",
		"local_time": "timestamp with local time zone",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	expectedTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	var returnedID int64
	var returnedTime time.Time

	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, local_time) VALUES (:1, :2) RETURNING id, local_time INTO :3, :4",
		int64(1001),
		expectedTime,
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedTime},
	)
	if err != nil {
		t.Fatalf("exec insert returning TIMESTAMP WITH LOCAL TIME ZONE failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}
	if returnedID != 1001 {
		t.Errorf("returnedID: got %d, want 1001", returnedID)
	}
	// TIMESTAMP WITH LOCAL TIME ZONE is returned in the session timezone, so compare
	// as UTC instants (same point in time, possibly different local representation).
	if !returnedTime.UTC().Equal(expectedTime.UTC()) {
		t.Errorf("TIMESTAMP WITH LOCAL TIME ZONE mismatch: got %v (UTC: %v), want %v (UTC: %v)",
			returnedTime, returnedTime.UTC(), expectedTime, expectedTime.UTC())
	}
}

// TestDriver_DMLReturning_Insert_NumberScalePrecision verifies RETURNING a NUMBER column
// with explicit precision and scale (NUMBER(12,4)). The returned value must preserve
// the correct decimal representation.
func TestDriver_DMLReturning_Insert_NumberScalePrecision(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_num_scale")
	cols := map[string]string{
		"id":     "number",
		"amount": "number(12,4)",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// 3.1415 stored in NUMBER(12,4) -> Oracle rounds to 4 decimal places.
	expected := 3.1415

	var returnedID int64
	var returnedAmount float64

	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, amount) VALUES (:1, :2) RETURNING id, amount INTO :3, :4",
		int64(1101),
		expected,
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedAmount},
	)
	if err != nil {
		t.Fatalf("exec insert returning NUMBER(12,4) failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}
	if returnedID != 1101 {
		t.Errorf("returnedID: got %d, want 1101", returnedID)
	}
	// Allow small floating-point tolerance from NUMBER encoding.
	if diff := returnedAmount - expected; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("returnedAmount: got %v, want %v (diff %v)", returnedAmount, expected, diff)
	}
}

// TestDriver_DMLReturning_BinaryFloatColumn verifies INSERT/UPDATE/DELETE ... RETURNING
// for a BINARY_FLOAT column. The driver surfaces BINARY_FLOAT as float64 after Oracle's
// float32 storage rounding, so expectations are computed from float32 values.
func TestDriver_DMLReturning_BinaryFloatColumn(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("t_dml_ret_bin_float")
	cols := map[string]string{
		"id":     "number",
		"amount": "binary_float",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	f32 := func(v float64) float64 { return float64(float32(v)) }
	assertBinaryFloat := func(label string, got, want float64) {
		const tol = 1e-6
		diff := got - want
		if diff < 0 {
			diff = -diff
		}
		if diff > tol {
			t.Fatalf("%s: got %.9g want %.9g (tol %.1e)", label, got, want, tol)
		}
	}

	const rowID = int64(1151)
	insertValue := 123.456789
	updateValue := -987.6543

	var returnedID int64
	var returnedAmount float64

	result, err := db.ExecContext(
		ctx,
		"INSERT INTO "+table+" (id, amount) VALUES (:1, :2) RETURNING id, amount INTO :3, :4",
		rowID,
		insertValue,
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedAmount},
	)
	if err != nil {
		t.Fatalf("exec insert returning BINARY_FLOAT failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval after insert failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected insert rows affected: got %d, want 1", rowsAffected)
	}
	if returnedID != rowID {
		t.Fatalf("unexpected returned id after insert: got %d, want %d", returnedID, rowID)
	}
	assertBinaryFloat("insert returned amount", returnedAmount, f32(insertValue))

	var persistedAmount float64
	if err := db.QueryRowContext(ctx,
		"SELECT amount FROM "+table+" WHERE id = :1",
		rowID,
	).Scan(&persistedAmount); err != nil {
		t.Fatalf("select persisted BINARY_FLOAT after insert failed: %v", err)
	}
	assertBinaryFloat("insert persisted amount", persistedAmount, f32(insertValue))

	result, err = db.ExecContext(
		ctx,
		"UPDATE "+table+" SET amount = :1 WHERE id = :2 RETURNING amount INTO :3",
		updateValue,
		rowID,
		sql.Out{Dest: &returnedAmount},
	)
	if err != nil {
		t.Fatalf("exec update returning BINARY_FLOAT failed: %v", err)
	}

	rowsAffected, err = result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval after update failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected update rows affected: got %d, want 1", rowsAffected)
	}
	assertBinaryFloat("update returned amount", returnedAmount, f32(updateValue))

	if err := db.QueryRowContext(ctx,
		"SELECT amount FROM "+table+" WHERE id = :1",
		rowID,
	).Scan(&persistedAmount); err != nil {
		t.Fatalf("select persisted BINARY_FLOAT after update failed: %v", err)
	}
	assertBinaryFloat("update persisted amount", persistedAmount, f32(updateValue))

	result, err = db.ExecContext(
		ctx,
		"DELETE FROM "+table+" WHERE id = :1 RETURNING amount INTO :2",
		rowID,
		sql.Out{Dest: &returnedAmount},
	)
	if err != nil {
		t.Fatalf("exec delete returning BINARY_FLOAT failed: %v", err)
	}

	rowsAffected, err = result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval after delete failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected delete rows affected: got %d, want 1", rowsAffected)
	}
	assertBinaryFloat("delete returned amount", returnedAmount, f32(updateValue))

	var remaining int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+table+" WHERE id = :1",
		rowID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count deleted BINARY_FLOAT row failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected deleted row to be absent, found %d rows", remaining)
	}
}

// TestDriver_DMLReturning_Insert_BooleanColumn verifies RETURNING a boolean column.
// This exercises the DtyBol wire type in the RETURNING path.
// Requires Oracle 23c or later for native BOOLEAN support.
func TestDriver_DMLReturning_Insert_BooleanColumn(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 23 {
		t.Skip("Native BOOLEAN RETURNING requires Oracle 23c or later")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("t_dml_ret_bool")
	cols := map[string]string{
		"id":        "number",
		"is_active": "boolean",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	for _, tc := range []struct {
		id       int64
		value    bool
		wantBool bool
	}{
		{1201, true, true},
		{1202, false, false},
	} {
		var returnedID int64
		var returnedBool bool

		result, err := db.ExecContext(
			ctx,
			"INSERT INTO "+table+" (id, is_active) VALUES (:1, :2) RETURNING id, is_active INTO :3, :4",
			tc.id,
			tc.value,
			sql.Out{Dest: &returnedID},
			sql.Out{Dest: &returnedBool},
		)
		if err != nil {
			t.Fatalf("exec insert returning BOOLEAN (id=%d) failed: %v", tc.id, err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected != 1 {
			t.Fatalf("unexpected rows affected (id=%d): got %d, want 1", tc.id, rowsAffected)
		}
		if returnedID != tc.id {
			t.Errorf("returnedID (id=%d): got %d", tc.id, returnedID)
		}
		if returnedBool != tc.wantBool {
			t.Errorf("returnedBool (id=%d): got %v, want %v", tc.id, returnedBool, tc.wantBool)
		}
	}
}
