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
	"strings"
	"testing"
)

// TestQueryNonExistentTable_NegativeCase validates that querying a non-existent table
// returns the appropriate ORA-00942 error.
func TestQueryNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT * FROM NONEXISTENT_TABLE_XYZ_12345"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error when querying non-existent table, but got none")
	}

	// ORA-00942 (table or view does not exist)
	errMsg := err.Error()
	if !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected error to contain %s, got: %v", TableOrViewNotFound, err)
	}
	t.Logf("correctly received error for non-existent table: %v", err)
}

// TestPreparedStatementNonExistentTable_NegativeCase validates that preparing a statement
// on a non-existent table returns an error (Prepare not implemented).
func TestPreparedStatementNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT * FROM non_existent_table"
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		// Prepare is not implemented, so we expect an error
		expectedErr := "Prepare not implemented"
		if err.Error() != expectedErr {
			t.Fatalf("expected '%s', got: %v", expectedErr, err)
		}
		t.Logf("correctly received error for prepared statement: %v", err)
		return
	}
	defer stmt.Close()

	// If Prepare succeeds, try to execute the statement
	_, err = stmt.QueryContext(ctx)
	if err == nil {
		t.Fatal("expected error when executing prepared statement on non-existent table")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected error to contain %s, got: %v", TableOrViewNotFound, err)
	}
	t.Logf("correctly received error for prepared statement execution: %v", err)
}

// TestSelectSpecificColumnsNonExistentTable_NegativeCase validates that selecting specific
// columns from a non-existent table returns ORA-00942 error.
func TestSelectSpecificColumnsNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT id, name FROM NONEXISTENT_TABLE_DEF"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error when selecting specific columns from non-existent table")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected error to contain %s, got: %v", TableOrViewNotFound, err)
	}
	t.Logf("correctly received error: %v", err)
}

// TestCountQueryNonExistentTable_NegativeCase validates that a COUNT query on a non-existent
// table returns ORA-00942 error.
func TestCountQueryNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT COUNT(*) FROM NONEXISTENT_TABLE_GHI"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error when executing COUNT on non-existent table")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected error to contain %s, got: %v", TableOrViewNotFound, err)
	}
	t.Logf("correctly received error for COUNT query: %v", err)
}

// TestJoinWithNonExistentTable_NegativeCase validates that a JOIN with a non-existent table
// returns ORA-00942 error.
func TestJoinWithNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT d.* FROM DUAL d JOIN NONEXISTENT_TABLE_JKL n ON 1=1"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error when joining with non-existent table")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected error to contain %s, got: %v", TableOrViewNotFound, err)
	}
	t.Logf("correctly received error for JOIN query: %v", err)
}

// TestSubqueryWithNonExistentTable_NegativeCase validates that a subquery with a non-existent
// table returns ORA-00942 error.
func TestSubqueryWithNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT * FROM (SELECT * FROM NONEXISTENT_TABLE_MNO)"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error when using subquery with non-existent table")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected error to contain %s, got: %v", TableOrViewNotFound, err)
	}
	t.Logf("correctly received error for subquery: %v", err)
}

// TestDescribeNonExistentTable_NegativeCase validates that querying metadata for a non-existent
// table returns no rows (not an error).
func TestDescribeNonExistentTable_NegativeCase(t *testing.T) {
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
	query := "SELECT column_name FROM user_tab_columns WHERE table_name = 'NONEXISTENT_TABLE_PQR'"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Fatal("expected no rows for non-existent table")
	}
	t.Logf("correctly returned no rows for non-existent table metadata")
}

// TestInvalidTableNameSyntax_NegativeCase validates that a query with invalid table name syntax
// returns an appropriate Oracle error (ORA-00903 invalid table name, or ORA-00942 if table doesn't exist).
func TestInvalidTableNameSyntax_NegativeCase(t *testing.T) {
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
	query := "SELECT * FROM 123INVALID_TABLE_NAME"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error for invalid table name syntax")
	}

	// syntax error (ORA-00903 invalid table name)
	errMsg := err.Error()
	if !strings.Contains(errMsg, string(InvalidTableName)) && !strings.Contains(errMsg, string(TableOrViewNotFound)) {
		t.Errorf("expected %s or %s, got: %v", InvalidTableName, TableOrViewNotFound, err)
	}
	t.Logf("correctly received error for invalid syntax: %v", err)
}
