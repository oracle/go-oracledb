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
	"testing"
)

// TestDriver_Table_Create
// Ensures we can create the test table (starting from a clean state).
// Expectation: CREATE TABLE succeeds; table is then dropped to avoid residual schema.
func TestDriver_Table_Create(t *testing.T) {
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
	table := createObjectName("t_ddl_create")
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

	// Quick sanity access (optional)
	if _, err := db.QueryContext(ctx, "SELECT 1 FROM "+table+" WHERE 1=0"); err != nil {
		t.Fatalf("table inaccessible after create: %v", err)
	}
}

// TestDriver_Table_Drop
// Creates and then drops the test table.
// Expectation: DROP succeeds; subsequent simple access fails.
func TestDriver_DropTable_DeniesAccess(t *testing.T) {
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
	table := createObjectName("t_ddl_drop")
	cols := map[string]string{
		"id":   "number",
		"name": "varchar2(100)",
	}
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	if err := dropTable(ctx, db, table); err != nil {
		t.Fatalf("drop table failed: %v", err)
	}
	// todo: Check for the error code once the break-reset MR is merged
	if _, err := db.ExecContext(ctx, "SELECT 1 FROM "+table); err == nil {
		t.Fatalf("expected error selecting from dropped table")
	}
}

// TestDriver_AlterSessionSetLanguage
// Executes an ALTER SESSION statement to set the NLS language to 'AMERICAN'
// to validate that the driver can perform session-level ALTER statements.
func TestDriver_AlterSessionSetLanguage(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	// Open connection
	dsn := TestingConfig.GetConnectionString()
	db, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("failed to open connection to %q: %v", dsn, err)
	}
	defer db.Close()

	// Ping
	if err := db.Ping(); err != nil {
		t.Fatalf("database ping failed: %v", err)
	}

	// Execute ALTER SESSION to set NLS language (sanity: ensures driver can run ALTER SESSION)
	if _, err := db.ExecContext(context.Background(), "ALTER SESSION SET NLS_LANGUAGE = 'AMERICAN'"); err != nil {
		t.Fatalf("ALTER SESSION SET NLS_LANGUAGE failed: %v", err)
	}
}

func TestDriver_CreateTrigger_WithPseudoRecords(t *testing.T) {
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
	parentTable := createObjectName("fk_parent_ref")
	childTable := createObjectName("fk_child_ref")
	triggerName := createObjectName("fk_parent_ref")
	uniqueConstraint := createObjectName("fk_parent_ref_uk")
	foreignKey := createObjectName("fk_child_ref_fk")

	if _, err := db.ExecContext(ctx, `CREATE TABLE `+parentTable+` (
		id NUMBER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		refer NUMBER,
		name VARCHAR2(100),
		CONSTRAINT `+uniqueConstraint+` UNIQUE (refer)
	)`); err != nil {
		t.Fatalf("create parent table failed: %v", err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "DROP TRIGGER "+triggerName); err != nil {
			t.Errorf("cleanup drop trigger %s failed: %v", triggerName, err)
		}
		if err := dropTable(ctx, db, childTable); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", childTable, err)
		}
		if err := dropTable(ctx, db, parentTable); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", parentTable, err)
		}
	}()

	if _, err := db.ExecContext(ctx, `CREATE TABLE `+childTable+` (
		id NUMBER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		member_id NUMBER,
		name VARCHAR2(100),
		CONSTRAINT `+foreignKey+` FOREIGN KEY (member_id) REFERENCES `+parentTable+` (refer) ON DELETE CASCADE
	)`); err != nil {
		t.Fatalf("create child table failed: %v", err)
	}

	triggerDDL := `CREATE OR REPLACE TRIGGER ` + triggerName + `
AFTER UPDATE OF refer ON ` + parentTable + `
FOR EACH ROW
BEGIN
  UPDATE ` + childTable + `
  SET member_id = :NEW.refer
  WHERE member_id = :OLD.refer;
END;`

	if _, err := db.ExecContext(ctx, triggerDDL); err != nil {
		t.Fatalf("create trigger with :NEW/:OLD pseudo-records failed: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		"INSERT INTO "+parentTable+" (refer, name) VALUES (:1, :2)",
		int64(1), "parent-row",
	); err != nil {
		t.Fatalf("insert parent row failed: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO "+childTable+" (member_id, name) VALUES (:1, :2)",
		int64(1), "child-row",
	); err != nil {
		t.Fatalf("insert child row failed: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		"UPDATE "+parentTable+" SET refer = :1 WHERE refer = :2",
		int64(2), int64(1),
	); err != nil {
		t.Fatalf("update parent refer failed: %v", err)
	}

	var childMemberID int64
	if err := db.QueryRowContext(ctx,
		"SELECT member_id FROM "+childTable+" WHERE name = :1",
		"child-row",
	).Scan(&childMemberID); err != nil {
		t.Fatalf("select child row failed: %v", err)
	}

	if childMemberID != 2 {
		t.Fatalf("unexpected child member_id after trigger cascade: got %d, want 2", childMemberID)
	}
}
