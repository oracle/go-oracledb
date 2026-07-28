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
	"testing"
)

// TestDriver_TRIGGER_GormTest verifies that TRIGGER :NEW and :OLD keywords
// do not get mistaken for placeholders
func TestDriver_TRIGGER_GormTest(t *testing.T) {
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
	baseTable := createObjectName("test_trigger")
	changesTable := createObjectName("test_trigger_changes")
	triggerName := createObjectName("trigger_test_test_trigger")
	if err := createTable(ctx, db, baseTable, map[string]string{
		"id":    "NUMBER PRIMARY KEY",
		"refer": "NUMBER",
	}); err != nil {
		t.Fatalf("Create table failed: %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, baseTable); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", baseTable, e)
		}
	}()

	if err := createTable(ctx, db, changesTable, map[string]string{
		"id":        "NUMBER PRIMARY KEY",
		"refer":     "NUMBER",
		"old_refer": "NUMBER",
	}); err != nil {
		t.Fatalf("Create table failed: %v", err)
		return
	}
	defer func() {
		if e := dropTable(ctx, db, changesTable); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", changesTable, e)
		}
	}()

	createTriggerStmt := "CREATE OR REPLACE TRIGGER " + triggerName + " " +
		"AFTER UPDATE OF refer ON " + baseTable + " " +
		"FOR EACH ROW " +
		"BEGIN " +
		"INSERT INTO " + changesTable + " (id, refer, old_refer) VALUES (:NEW.id, :NEW.refer, :OLD.refer); " +
		"END;"

	_, err = db.ExecContext(ctx, createTriggerStmt)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	_, err = db.ExecContext(ctx, "INSERT INTO "+baseTable+" (id, refer) values (1, 2)")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	_, err = db.ExecContext(ctx, "UPDATE "+baseTable+" set refer = 3 where id = 1")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT id, refer, old_refer FROM "+changesTable)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	var id, refer, old_refer int
	if rows.Next() {
		rows.Scan(&id, &refer, &old_refer)
		if id != 1 {
			t.Fatalf("Expected id to be 1 but was %d", id)
		}
		if refer != 3 {
			t.Fatalf("Expected refer to be 3 but was %d", refer)
		}
		if old_refer != 2 {
			t.Fatalf("Expected old_refer to be 2 but was %d", old_refer)
		}
	} else {
		t.Fatalf("Expected line to be returned")
	}
}
