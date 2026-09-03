// SPDX-License-Identifier: UPL-1.0

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
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// TestDriver_CreateInBatchesEquivalent verifies repeated prepared inserts and
// expects six rows with the expected aggregate age value.
func TestDriver_CreateInBatchesEquivalent(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_gorm_batch_insert")
	if err := createTable(ctx, db, table, map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
		"age":  "NUMBER",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	stmt, err := db.PrepareContext(ctx, "INSERT INTO "+table+" (id, name, age) VALUES (:1, :2, :3)")
	if err != nil {
		t.Fatalf("prepare batch insert failed: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	for i := 1; i <= 6; i++ {
		res, err := stmt.ExecContext(ctx, int64(i), fmt.Sprintf("create_in_batches_%d", i), int64(20+i))
		if err != nil {
			t.Fatalf("batch insert row %d failed: %v", i, err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("batch insert row %d rows affected: %v", i, err)
		}
		if rows != 1 {
			t.Fatalf("batch insert row %d affected %d rows, want 1", i, rows)
		}
	}

	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows failed: %v", err)
	}
	if count != 6 {
		t.Fatalf("inserted row count mismatch: got %d, want 6", count)
	}

	var sumAge int64
	if err := db.QueryRowContext(ctx, "SELECT SUM(age) FROM "+table).Scan(&sumAge); err != nil {
		t.Fatalf("sum age failed: %v", err)
	}
	if sumAge != 141 {
		t.Fatalf("age sum mismatch: got %d, want 141", sumAge)
	}
}

// TestDriver_BatchUpdateSliceEquivalent verifies repeated prepared updates
// and expects every seeded row to have the updated age.
func TestDriver_BatchUpdateSliceEquivalent(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_gorm_batch_update")
	if err := createTable(ctx, db, table, map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
		"age":  "NUMBER",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	for i := 1; i <= 5; i++ {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO "+table+" (id, name, age) VALUES (:1, :2, :3)",
			int64(i), fmt.Sprintf("batch%d", i), int64(i),
		); err != nil {
			t.Fatalf("seed row %d failed: %v", i, err)
		}
	}

	updateStmt, err := db.PrepareContext(ctx, "UPDATE "+table+" SET age = :1 WHERE id = :2")
	if err != nil {
		t.Fatalf("prepare batch update failed: %v", err)
	}
	t.Cleanup(func() { _ = updateStmt.Close() })

	for i := 1; i <= 5; i++ {
		res, err := updateStmt.ExecContext(ctx, int64(99), int64(i))
		if err != nil {
			t.Fatalf("batch update row %d failed: %v", i, err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("batch update row %d rows affected: %v", i, err)
		}
		if rows != 1 {
			t.Fatalf("batch update row %d affected %d rows, want 1", i, rows)
		}
	}

	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE age = 99").Scan(&count); err != nil {
		t.Fatalf("count updated rows failed: %v", err)
	}
	if count != 5 {
		t.Fatalf("updated row count mismatch: got %d, want 5", count)
	}
}

// TestDriver_MixedSaveBatchEquivalent verifies updating existing rows and
// inserting a new row, expecting three rows with the final age value.
func TestDriver_MixedSaveBatchEquivalent(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_gorm_mixed_save")
	if err := createTable(ctx, db, table, map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(100)",
		"age":  "NUMBER",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	for i := 1; i <= 2; i++ {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO "+table+" (id, name, age) VALUES (:1, :2, :3)",
			int64(i), fmt.Sprintf("existing%d", i), int64(30+i),
		); err != nil {
			t.Fatalf("seed existing row %d failed: %v", i, err)
		}
	}

	for i := 1; i <= 2; i++ {
		if _, err := db.ExecContext(ctx,
			"UPDATE "+table+" SET age = :1 WHERE id = :2",
			int64(99), int64(i),
		); err != nil {
			t.Fatalf("mixed save update row %d failed: %v", i, err)
		}
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO "+table+" (id, name, age) VALUES (:1, :2, :3)",
		int64(3), "new_user", int64(99),
	); err != nil {
		t.Fatalf("mixed save insert failed: %v", err)
	}

	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE age = 99").Scan(&count); err != nil {
		t.Fatalf("count mixed save rows failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("mixed save row count mismatch: got %d, want 3", count)
	}
}

// TestDriver_JSONCreateInBatchesAndBulkUpdateEquivalent verifies JSON batch
// inserts followed by a bulk JSON_TRANSFORM update of all inserted rows.
func TestDriver_JSONCreateInBatchesAndBulkUpdateEquivalent(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("JSON type is not supported for DB < 21")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createObjectName("t_gorm_json_batch")
	if err := createTable(ctx, db, table, map[string]string{
		"record_id": "NUMBER PRIMARY KEY",
		"doc":       "JSON",
	}); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	insertStmt, err := db.PrepareContext(ctx, "INSERT INTO "+table+" (record_id, doc) VALUES (:1, :2)")
	if err != nil {
		t.Fatalf("prepare JSON batch insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insertStmt.Close() })

	ids := make([]any, 0, 50)
	t.Run("insert batch", func(t *testing.T) {
		for i := 1; i <= 50; i++ {
			id := int64(i)
			if _, err := insertStmt.ExecContext(ctx, id, fmt.Sprintf(`{"a":%d}`, i)); err != nil {
				t.Fatalf("JSON batch insert row %d failed: %v", i, err)
			}
			ids = append(ids, id)
		}
	})

	t.Run("bulk update", func(t *testing.T) {
		placeholders := make([]string, 0, len(ids))
		args := make([]any, 0, len(ids)+1)
		args = append(args, "x")
		for i, id := range ids {
			placeholders = append(placeholders, fmt.Sprintf(":%d", i+2))
			args = append(args, id)
		}

		updateSQL := fmt.Sprintf(
			"UPDATE %s SET doc = JSON_TRANSFORM(doc, SET '$.b' = :1) WHERE record_id IN (%s)",
			table,
			strings.Join(placeholders, ", "),
		)
		res, err := db.ExecContext(ctx, updateSQL, args...)
		if err != nil {
			t.Fatalf("JSON bulk update failed: %v", err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("JSON bulk update rows affected: %v", err)
		}
		if rows != 50 {
			t.Fatalf("JSON bulk update affected %d rows, want 50", rows)
		}
	})

	t.Run("verify updated rows", func(t *testing.T) {
		var updatedCount int64
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE JSON_VALUE(doc, '$.b') = :1",
			"x",
		).Scan(&updatedCount); err != nil {
			t.Fatalf("count JSON updated rows failed: %v", err)
		}
		if updatedCount != 50 {
			t.Fatalf("JSON updated row count mismatch: got %d, want 50", updatedCount)
		}
	})
}

// TestDriver_StringVarrayEquivalent verifies string VARRAY DDL and insertion;
// result decoding remains explicitly skipped because it is unsupported.
func TestDriver_StringVarrayEquivalent(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	tableName := createObjectName("email_varray_table")
	varrayType := createObjectName("email_list_arr")

	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE TYPE "`+varrayType+`" AS VARRAY(10) OF VARCHAR2(80)`); err != nil {
		t.Fatalf("create %s failed: %v", varrayType, err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE "`+tableName+`" (
		"ID" NUMBER PRIMARY KEY,
		"EMAILS" "`+varrayType+`"
	)`); err != nil {
		t.Fatalf("create email_varray_tables failed: %v", err)
	}
	t.Cleanup(func() { dropVarrayTestObjects(t, ctx, db, tableName, varrayType, "") })

	if _, err := db.ExecContext(ctx, `INSERT INTO "`+tableName+`" ("ID", "EMAILS")
		VALUES (1, "`+varrayType+`"('alice@example.com','bob@example.com','gorm@oracle.com'))`); err != nil {
		t.Fatalf("insert string VARRAY row failed: %v", err)
	}

	// Oracle collection result decoding is not supported by the pure Go driver.
	// Keep the DDL and bind coverage above, but do not treat an unsupported scan
	// as a functional failure while this GORM-derived scenario remains unported.
	t.Skip("VARRAY result decoding is not supported by pure-go-driver")

}

// TestDriver_VarrayOfObjectEquivalent verifies object VARRAY DDL and insertion;
// result decoding remains explicitly skipped because it is unsupported.
func TestDriver_VarrayOfObjectEquivalent(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	tableName := createObjectName("dept_phone_list")
	varrayType := createObjectName("phone_varray_typ")
	objectType := createObjectName("phone_typ")

	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE TYPE "`+objectType+`" AS OBJECT (
		"country_code" VARCHAR2(2),
		"area_code" VARCHAR2(3),
		"ph_number" VARCHAR2(7)
	)`); err != nil {
		t.Fatalf("create phone_typ failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE TYPE "`+varrayType+`" AS VARRAY(5) OF "`+objectType+`"`); err != nil {
		t.Fatalf("create phone_varray_typ failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE "`+tableName+`" (
		"dept_no" NUMBER(5) PRIMARY KEY,
		"phone_list" "`+varrayType+`"
	)`); err != nil {
		t.Fatalf("create dept_phone_lists failed: %v", err)
	}
	t.Cleanup(func() { dropVarrayTestObjects(t, ctx, db, tableName, varrayType, objectType) })

	if _, err := db.ExecContext(ctx, `INSERT INTO "`+tableName+`" ("dept_no", "phone_list") VALUES (
		100,
		"`+varrayType+`"(
			"`+objectType+`"('01', '650', '5550123'),
			"`+objectType+`"('01', '650', '5550148'),
			"`+objectType+`"('01', '650', '5550192')
		)
	)`); err != nil {
		t.Fatalf("insert object VARRAY row failed: %v", err)
	}

	// Oracle collection result decoding is not supported by the pure Go driver.
	// Keep the DDL and bind coverage above, but do not treat an unsupported scan
	// as a functional failure while this GORM-derived scenario remains unported.
	t.Skip("VARRAY result decoding is not supported by pure-go-driver")

}

func dropVarrayTestObjects(t *testing.T, ctx context.Context, db *sql.DB, tableName string, varrayType string, objectType string) {
	t.Helper()
	if tableName != "" {
		if _, err := db.ExecContext(ctx, `DROP TABLE "`+tableName+`" PURGE`); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", tableName, err)
		}
	}
	if varrayType != "" {
		if _, err := db.ExecContext(ctx, `DROP TYPE "`+varrayType+`" FORCE`); err != nil {
			t.Errorf("cleanup drop type %s failed: %v", varrayType, err)
		}
	}
	if objectType != "" {
		if _, err := db.ExecContext(ctx, `DROP TYPE "`+objectType+`" FORCE`); err != nil {
			t.Errorf("cleanup drop type %s failed: %v", objectType, err)
		}
	}
}
