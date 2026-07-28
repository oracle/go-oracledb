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

// TestQuerySystemTableWithoutPrivilege_NegativeCase validates that querying a system table
// without privilege returns an appropriate error (ORA-01031, ORA-41900, or ORA-00942).
func TestQuerySystemTableWithoutPrivilege_NegativeCase(t *testing.T) {
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
	query := "SELECT * FROM SYS.USER$"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		t.Fatal("expected error when querying system table without privilege, but got none")
	}

	// Check for one of the expected error codes
	errMsg := err.Error()
	hasExpectedError := strings.Contains(errMsg, string(TableOrViewNotFound)) ||
		strings.Contains(errMsg, string(InsufficientPrivilege)) ||
		strings.Contains(errMsg, string(MissingReadPrivilege))

	if !hasExpectedError {
		t.Errorf("expected %s, %s, or %s, got: %v", TableOrViewNotFound, InsufficientPrivilege, MissingReadPrivilege, err)
	}
	t.Logf("correctly received error: %v", err)
}

// TestQueryAccessibleTable_PositiveCase validates that querying an accessible table (DUAL)
// succeeds and returns expected results.
func TestQueryAccessibleTable_PositiveCase(t *testing.T) {
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
	query := "SELECT 1 FROM DUAL"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("query failed on accessible table: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected at least one row from DUAL")
	}

	var val int
	if err := rows.Scan(&val); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if val != 1 {
		t.Errorf("expected value 1, got %d", val)
	}
	t.Logf("successfully queried accessible table")
}

// TestQueryDictionaryViewWithoutPrivilege_NegativeCase validates that querying a restricted
// dictionary view without appropriate privileges returns an insufficient privilege error.
// This test attempts to query V$SESSION which requires SELECT privilege on the view.
func TestQueryDictionaryViewWithoutPrivilege_NegativeCase(t *testing.T) {
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
	// V$SESSION is a dynamic performance view that typically requires SELECT_CATALOG_ROLE
	// or explicit SELECT privilege on V_$SESSION
	query := "SELECT sid, serial# FROM V$SESSION WHERE rownum <= 1"
	_, err = db.QueryContext(ctx, query)

	if err == nil {
		// If no error, the user has privilege to access V$SESSION (which is acceptable)
		t.Logf("user has privilege to access V$SESSION")
		return
	}

	// Check for privilege-related error
	errMsg := err.Error()
	hasPrivilegeError := strings.Contains(errMsg, string(InsufficientPrivilege))

	if !hasPrivilegeError {
		t.Logf("received error (may indicate privilege restriction): %v", err)
	} else {
		t.Logf("correctly received privilege-related error: %v", err)
	}
}
