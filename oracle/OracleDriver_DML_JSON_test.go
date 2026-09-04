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
	"errors"
	"fmt"
	"strings"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_Prepared_Insert_InvalidThenValidJSON_ReusesStatement verifies an
// invalid JSON bind does not prevent a later valid execution of the statement.
func TestDriver_Prepared_Insert_InvalidThenValidJSON_ReusesStatement(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("JSON Type is not supported for DB < 21")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close DB failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("t_json_prep_reuse")
	cols := map[string]string{
		"id": "NUMBER PRIMARY KEY",
	}

	if err := createTableWithNativeJSON(ctx, db, table, cols, "jdoc"); err != nil {
		t.Fatalf("create native JSON table %s: %v", table, err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx failed: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("cleanup rollback failed: %v", err)
		}
	})

	insSQL := "INSERT INTO " + table + " (id, jdoc) VALUES (:id, :jdoc)"
	stmt, err := tx.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	t.Cleanup(func() {
		if err := stmt.Close(); err != nil {
			t.Errorf("cleanup close statement failed: %v", err)
		}
	})

	invalidID := int64(1)
	validID := int64(2)
	invalidJSON := `{"payload":"abc}`
	validJSON := `{"payload":"abc"}`

	if _, err := stmt.ExecContext(ctx, sql.Named("id", invalidID), sql.Named("jdoc", invalidJSON)); err == nil {
		t.Fatalf("expected invalid JSON insert to fail")
	} else if !isExpectedJSONInsertError(err) {
		t.Fatalf("expected JSON validation error, got: %v", err)
	}

	if _, err := stmt.ExecContext(ctx, sql.Named("id", validID), sql.Named("jdoc", validJSON)); err != nil {
		t.Fatalf("expected prepared statement reuse to succeed after JSON error, got: %v", err)
	}

	countSQL := "SELECT COUNT(*) FROM " + table + " WHERE id = :id"

	var invalidCount int
	if err := tx.QueryRowContext(ctx, countSQL, sql.Named("id", invalidID)).Scan(&invalidCount); err != nil {
		t.Fatalf("count failed for invalid id: %v", err)
	}
	if invalidCount != 0 {
		t.Fatalf("invalid JSON insert stored %d rows, want 0", invalidCount)
	}

	var validCount int
	if err := tx.QueryRowContext(ctx, countSQL, sql.Named("id", validID)).Scan(&validCount); err != nil {
		t.Fatalf("count failed for valid id: %v", err)
	}
	if validCount != 1 {
		t.Fatalf("valid JSON insert stored %d rows, want 1", validCount)
	}
}

// TestDriver_Prepared_Insert_InvalidJSON_NegativeCase verifies invalid JSON is
// rejected with the expected structured database error.
func TestDriver_Prepared_Insert_InvalidJSON_NegativeCase(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if TestingConfig.DatabaseVersion.Major < 21 {
		t.Skip("JSON Type is not supported for DB < 21")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close DB failed: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("t_invalid_json_prep")
	cols := map[string]string{
		"id": "NUMBER PRIMARY KEY",
	}

	if err := createTableWithNativeJSON(ctx, db, table, cols, "jdoc"); err != nil {
		t.Fatalf("create native JSON table %s: %v", table, err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	insSQL := "INSERT INTO " + table + " (id, jdoc) VALUES (:id, :jdoc)"
	stmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare invalid JSON insert failed: %v", err)
	}
	t.Cleanup(func() {
		if err := stmt.Close(); err != nil {
			t.Errorf("cleanup close statement failed: %v", err)
		}
	})

	testCases := []struct {
		id     int64
		name   string
		jsonIn string
	}{
		{id: 1, name: "unterminated-string", jsonIn: `{"payload":"abc}`},
		{id: 2, name: "invalid-escape-sequence", jsonIn: `{"payload":"abc\q"}`},
		{id: 3, name: "invalid-unicode-escape", jsonIn: `{"payload":"\u12X4"}`},
	}

	countSQL := "SELECT COUNT(*) FROM " + table + " WHERE id = :id"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := stmt.ExecContext(ctx, sql.Named("id", tc.id), sql.Named("jdoc", tc.jsonIn)); err == nil {
				t.Fatalf("expected prepared invalid JSON insert to fail for %s", tc.name)
			} else if !isExpectedJSONInsertError(err) {
				t.Fatalf("expected JSON validation error for %s, got: %v", tc.name, err)
			}

			var rowCount int
			if err := db.QueryRowContext(ctx, countSQL, sql.Named("id", tc.id)).Scan(&rowCount); err != nil {
				t.Fatalf("count failed for %s: %v", tc.name, err)
			}
			if rowCount != 0 {
				t.Fatalf("prepared invalid JSON insert for %s stored %d rows, want 0", tc.name, rowCount)
			}
		})
	}
}

func isExpectedJSONInsertError(err error) bool {
	for err != nil {
		if serr, ok := err.(oracleErrors.SQLError); ok {
			code := serr.ErrorCode()
			if code == "ORA-02290" || strings.HasPrefix(code, "ORA-404") || strings.HasPrefix(code, "ORA-405") {
				return true
			}
		}
		err = errors.Unwrap(err)
	}
	return false
}

func createTableWithNativeJSON(ctx context.Context, db *sql.DB, table string, desc map[string]string, jsonColumn string) error {
	if strings.TrimSpace(jsonColumn) == "" {
		return fmt.Errorf("json column name must be provided")
	}

	cols := make(map[string]string, len(desc)+1)
	for k, v := range desc {
		cols[k] = v
	}

	cols[jsonColumn] = "JSON"
	return createTable(ctx, db, table, cols)
}
