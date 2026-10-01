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

// TestDriver_PreparedInsertReuseAfterError verifies a prepared INSERT can be
// reused after an invalid JSON value fails.
func TestDriver_PreparedInsertReuseAfterError(t *testing.T) {
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
		"id":   "NUMBER PRIMARY KEY",
		"jdoc": "JSON",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
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
	}

	if _, err := stmt.ExecContext(ctx, sql.Named("id", validID), sql.Named("jdoc", validJSON)); err != nil {
		t.Fatalf("expected prepared statement reuse to succeed after JSON error, got: %v", err)
	}
}

// TestDriver_PreparedInsertRejectsInvalidJSON verifies invalid JSON values
// are rejected by a prepared INSERT.
func TestDriver_PreparedInsertRejectsInvalidJSON(t *testing.T) {
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
		"id":   "NUMBER PRIMARY KEY",
		"jdoc": "JSON",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := stmt.ExecContext(ctx, sql.Named("id", tc.id), sql.Named("jdoc", tc.jsonIn)); err == nil {
				t.Fatalf("expected prepared invalid JSON insert to fail for %s", tc.name)
			}
		})
	}
}
