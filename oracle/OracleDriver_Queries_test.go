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

// TestDriver_AnalyticQueryWithJoinAndWindowFunctions exercises named binds,
// joins, inline views, and analytic functions through database/sql.
func TestDriver_AnalyticQueryWithJoinAndWindowFunctions(t *testing.T) {
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
	departments := createObjectName("analytic_dept")
	employees := createObjectName("analytic_emp")
	_ = dropTable(ctx, db, employees)
	_ = dropTable(ctx, db, departments)
	if err := createTable(ctx, db, departments, map[string]string{
		"department_id":   "NUMBER PRIMARY KEY",
		"department_name": "VARCHAR2(20) NOT NULL",
	}); err != nil {
		t.Fatalf("create departments: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, employees); err != nil {
			t.Errorf("drop employees: %v", err)
		}
		if err := dropTable(ctx, db, departments); err != nil {
			t.Errorf("drop departments: %v", err)
		}
	}()
	if err := createTable(ctx, db, employees, map[string]string{
		"employee_id":   "NUMBER PRIMARY KEY",
		"department_id": "NUMBER NOT NULL",
		"salary":        "NUMBER NOT NULL",
	}); err != nil {
		t.Fatalf("create employees: %v", err)
	}

	for _, row := range []struct {
		id   int
		name string
	}{{10, "Engineering"}, {20, "Support"}} {
		if _, err := db.ExecContext(ctx, "INSERT INTO "+departments+" (department_id, department_name) VALUES (:1, :2)", row.id, row.name); err != nil {
			t.Fatalf("insert department %d: %v", row.id, err)
		}
	}
	for _, row := range []struct {
		id, departmentID, salary int
	}{{101, 10, 100}, {102, 10, 80}, {103, 10, 40}, {201, 20, 90}, {202, 20, 70}, {203, 20, 60}} {
		if _, err := db.ExecContext(ctx, "INSERT INTO "+employees+" (employee_id, department_id, salary) VALUES (:1, :2, :3)", row.id, row.departmentID, row.salary); err != nil {
			t.Fatalf("insert employee %d: %v", row.id, err)
		}
	}

	query := `SELECT employee_id, department_name, salary, department_rank, department_payroll
FROM (
  SELECT e.employee_id, d.department_name, e.salary,
         ROW_NUMBER() OVER (PARTITION BY e.department_id ORDER BY e.salary DESC, e.employee_id) AS department_rank,
         SUM(e.salary) OVER (PARTITION BY e.department_id) AS department_payroll
  FROM ` + employees + ` e
  JOIN ` + departments + ` d ON d.department_id = e.department_id
  WHERE e.salary >= :minimum_salary
)
WHERE department_rank <= :top_n
ORDER BY department_name, department_rank`
	rows, err := db.QueryContext(ctx, query,
		sql.Named("minimum_salary", 50),
		sql.Named("top_n", 2),
	)
	if err != nil {
		t.Fatalf("query analytic inline view: %v", err)
	}
	defer rows.Close()

	type result struct {
		id, salary, rank, payroll int
		department                string
	}
	var got []result
	for rows.Next() {
		var row result
		if err := rows.Scan(&row.id, &row.department, &row.salary, &row.rank, &row.payroll); err != nil {
			t.Fatalf("scan analytic row: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate analytic rows: %v", err)
	}
	want := []result{
		{id: 101, department: "Engineering", salary: 100, rank: 1, payroll: 180},
		{id: 102, department: "Engineering", salary: 80, rank: 2, payroll: 180},
		{id: 201, department: "Support", salary: 90, rank: 1, payroll: 220},
		{id: 202, department: "Support", salary: 70, rank: 2, payroll: 220},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestDriver_SelectWithCTE verifies that a SELECT beginning with a WITH clause
// is routed through the query path and returns the expected row.
func TestDriver_SelectWithCTE(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	var value int
	query := "WITH one_row AS (SELECT 1 AS n FROM dual) SELECT n FROM one_row"
	if err := db.QueryRowContext(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("query CTE: %v", err)
	}
	if value != 1 {
		t.Fatalf("CTE value = %d, want 1", value)
	}
}

// TestDriver_MergeMatchedUpdateAndUnmatchedInsert verifies both MERGE branches,
// bind handling, affected-row counts, and the resulting stored values.
func TestDriver_MergeMatchedUpdateAndUnmatchedInsert(t *testing.T) {
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
	table := createObjectName("merge_test")
	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"payload": "VARCHAR2(30) NOT NULL",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("drop table: %v", err)
		}
	}()
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, payload) VALUES (:1, :2)", 1, "before"); err != nil {
		t.Fatalf("insert seed row: %v", err)
	}

	merge := "MERGE INTO " + table + ` tgt
USING (SELECT :id AS id, :payload AS payload FROM dual) src
ON (tgt.id = src.id)
WHEN MATCHED THEN UPDATE SET tgt.payload = src.payload
WHEN NOT MATCHED THEN INSERT (id, payload) VALUES (src.id, src.payload)`
	for _, input := range []struct {
		id      int
		payload string
	}{{1, "updated"}, {2, "inserted"}} {
		result, err := db.ExecContext(ctx, merge,
			sql.Named("id", input.id), sql.Named("payload", input.payload))
		if err != nil {
			t.Fatalf("MERGE id %d: %v", input.id, err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			t.Fatalf("MERGE id %d RowsAffected() = %d, err=%v; want 1", input.id, affected, err)
		}
	}

	rows, err := db.QueryContext(ctx, "SELECT id, payload FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select merged rows: %v", err)
	}
	defer rows.Close()
	want := []struct {
		id      int
		payload string
	}{{1, "updated"}, {2, "inserted"}}
	for i, expected := range want {
		if !rows.Next() {
			t.Fatalf("missing merged row %d: rows.Err()=%v", i, rows.Err())
		}
		var got struct {
			id      int
			payload string
		}
		if err := rows.Scan(&got.id, &got.payload); err != nil {
			t.Fatalf("scan merged row %d: %v", i, err)
		}
		if got.id != expected.id || got.payload != expected.payload {
			t.Errorf("row %d = %+v, want %+v", i, got, expected)
		}
	}
	if rows.Next() {
		t.Fatal("unexpected extra merged row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate merged rows: %v", err)
	}
}
