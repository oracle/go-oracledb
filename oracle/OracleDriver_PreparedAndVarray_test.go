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
	"fmt"
	"strings"
	"testing"
)

// TestDriver_PreparedInsertReuse verifies one prepared INSERT can be executed
// repeatedly and that each execution inserts one row.
func TestDriver_PreparedInsertReuse(t *testing.T) {
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
	table := createObjectName("t_gorm_prepared_insert")
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
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	for i := 1; i <= 6; i++ {
		res, err := stmt.ExecContext(ctx, int64(i), fmt.Sprintf("prepared_insert_%d", i), int64(20+i))
		if err != nil {
			t.Fatalf("prepared insert row %d failed: %v", i, err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("prepared insert row %d rows affected: %v", i, err)
		}
		if rows != 1 {
			t.Fatalf("prepared insert row %d affected %d rows, want 1", i, rows)
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

// TestDriver_PreparedUpdateReuse verifies one prepared UPDATE can be executed
// repeatedly and updates every seeded row.
func TestDriver_PreparedUpdateReuse(t *testing.T) {
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
	table := createObjectName("t_gorm_prepared_update")
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
			int64(i), fmt.Sprintf("prepared%d", i), int64(i),
		); err != nil {
			t.Fatalf("seed row %d failed: %v", i, err)
		}
	}

	updateStmt, err := db.PrepareContext(ctx, "UPDATE "+table+" SET age = :1 WHERE id = :2")
	if err != nil {
		t.Fatalf("prepare update failed: %v", err)
	}
	t.Cleanup(func() { _ = updateStmt.Close() })

	for i := 1; i <= 5; i++ {
		res, err := updateStmt.ExecContext(ctx, int64(99), int64(i))
		if err != nil {
			t.Fatalf("prepared update row %d failed: %v", i, err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("prepared update row %d rows affected: %v", i, err)
		}
		if rows != 1 {
			t.Fatalf("prepared update row %d affected %d rows, want 1", i, rows)
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

// TestDriver_JSONPreparedInsertAndUpdate verifies repeated prepared JSON
// inserts followed by an update of all inserted rows.
func TestDriver_JSONPreparedInsertAndUpdate(t *testing.T) {
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
	table := createObjectName("t_gorm_json_prepared")
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
		t.Fatalf("prepare JSON insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insertStmt.Close() })

	ids := make([]any, 0, 50)
	t.Run("insert rows", func(t *testing.T) {
		for i := 1; i <= 50; i++ {
			id := int64(i)
			if _, err := insertStmt.ExecContext(ctx, id, fmt.Sprintf(`{"a":%d}`, i)); err != nil {
				t.Fatalf("JSON insert row %d failed: %v", i, err)
			}
			ids = append(ids, id)
		}
	})

	t.Run("update rows", func(t *testing.T) {
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
			t.Fatalf("JSON update failed: %v", err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("JSON update rows affected: %v", err)
		}
		if rows != 50 {
			t.Fatalf("JSON update affected %d rows, want 50", rows)
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
