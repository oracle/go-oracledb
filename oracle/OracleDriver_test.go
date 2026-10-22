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
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_SimpleConnection executes a simple connection.
func TestDriver_SimpleConnection(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	fmt.Printf("connecting to %q\n", TestingConfig.GetConnectionString())
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection : %v", err)
	}
	defer db.Close()
}

// TestDriver_Functional_SelectDual executes a trivial SELECT to validate basic query flow.
func TestDriver_Functional_SelectDual(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
	if err != nil {
		t.Fatalf("select from DUAL failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("no row returned from DUAL")
	}
	var val int
	if err := rows.Scan(&val); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if val != 1 {
		t.Fatalf("unexpected value from DUAL: got %d, want 1", val)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
}

// TestDriver_Functional_ConnectionWrapper verifies that a dedicated
// database/sql connection opened by go-oracledb can be wrapped for Oracle
// specific operations. The wrapper must retain that connection, and wrapping it
// after Close must return sql.ErrConnDone and no wrapper.
func TestDriver_Functional_ConnectionWrapper(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to acquire dedicated connection: %v", err)
	}
	defer conn.Close()
	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("NewConnectionWrapper failed: %v", err)
	}
	if wrapper == nil || wrapper.connection != conn {
		t.Fatal("NewConnectionWrapper did not wrap the supplied Oracle connection")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("closing dedicated connection failed: %v", err)
	}

	if closedWrapper, err := NewConnectionWrapper(conn); !errors.Is(err, sql.ErrConnDone) || closedWrapper != nil {
		t.Fatalf("NewConnectionWrapper on a closed connection returned (%v, %v), want nil wrapper and sql.ErrConnDone", closedWrapper, err)
	}
}

// TestDriver_Functional_NetworkCompression verifies Oracle Net compression
// through the public OracleDriverConfig.ConnectionProperties API.
//
// The test enables Compression, creates a connector from that configuration,
// and queries a highly compressible 4,000-byte value that exceeds the default
// 1,024-byte compression threshold. It verifies end-to-end interoperability
// for the public configuration path, but query success alone does not prove
// that the server negotiated compression.
//
// The database server must enable a compatible compression level in sqlnet.ora:
//
//	SQLNET.COMPRESSION=on
//	SQLNET.COMPRESSION_LEVELS=(high)
//
// Packet construction, negotiation flags, compression, decompression, server
// refusal, and unsupported server compression schemes are covered separately
// by unit tests in internal/driver/network/session.
func TestDriver_Functional_NetworkCompression(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	config := NewOracleDriverConfig()
	config.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
	config.Credentials.User = TestingConfig.Credentials.Username
	config.Credentials.Password = TestingConfig.Credentials.Password
	config.ConnectDescriptor = TestingConfig.GetConnectionDSN()
	config.ConnectionProperties.Compression = true
	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("create compressed connection connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close compressed connection: %v", err)
		}
	}()

	var value string
	if err := db.QueryRowContext(context.Background(), "SELECT RPAD('x', 4000, 'x') FROM DUAL").Scan(&value); err != nil {
		t.Fatalf("query over compressed connection: %v", err)
	}
	if len(value) != 4000 {
		t.Fatalf("result length: got %d, want 4000", len(value))
	}
}

// TestDriver_Table_Create_Multiple_Connections
// Opens two independent connections from a shared sql.DB pool and pings each to
// validate basic multi-connection handling via database/sql.
func TestDriver_Table_Create_Multiple_Connections(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	dsn := TestingConfig.GetConnectionString()
	pool, err := sql.Open(TestingConfig.Driver.Name, dsn)
	if err != nil {
		t.Fatalf("open pool failed: %v", err)
	}
	defer pool.Close()

	conn1, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn1 failed: %v", err)
	}
	defer conn1.Close()
	if err := conn1.PingContext(context.Background()); err != nil {
		t.Fatalf("Conn1 ping failed: %v", err)
	}

	conn2, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn2 failed: %v", err)
	}
	defer conn2.Close()
	if err := conn2.PingContext(context.Background()); err != nil {
		t.Fatalf("Conn2 ping failed: %v", err)
	}

	fmt.Println("multi-connection sanity OK")
}

// Tests TestConnection_ResetSession on valid connection
// expectation:
//
//	reset return no error
func TestConnection_ResetSessionPool(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	t.Logf("connecting to %q\n", TestingConfig.GetConnectionString())
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection : %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to get connection out of pool : %v", err)
	}
	// send it back to the pool
	conn.Close()

	conn, err = db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to get connection out of pool : %v", err)
	}
	conn.Raw(func(c any) error {
		err := c.(driver.SessionResetter).ResetSession(context.Background())
		return err
	})
	conn.Close()
}

// Tests TestConnection_ResetSession on valid connection
// expectation:
//
//	reset return no error
func TestConnection_ResetSessionOk(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	t.Logf("connecting to %q\n", TestingConfig.GetConnectionString())
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection : %v", err)
	}
	defer db.Close()

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to get connection out of pool : %v", err)
	}
	defer conn.Close()

	r_Err := conn.Raw(func(c any) error {
		err := c.(driver.SessionResetter).ResetSession(context.Background())
		return err
	})
	if r_Err != nil {
		t.Fatalf("Should not have received an error, received : %v", r_Err)
	}

}

// Tests TestConnection_ResetSession on invalid connection
// expectation:
//
//	reset returns driver.ErrBadConn
func TestConnection_ResetSessionKo(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	t.Logf("connecting to %q\n", TestingConfig.GetConnectionString())
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open connection : %v", err)
	}
	defer db.Close()

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to get connection out of pool : %v", err)
	}

	conn.Close()

	r_Err := conn.Raw(func(c any) error {
		err := c.(driver.SessionResetter).ResetSession(context.Background())
		return err
	})

	if r_Err == nil {
		t.Fatalf("Should have received an error for invalid cnx")
	}

	t.Logf("Invalid cnx ResetSession returned expected [%v]", r_Err)

}

func TestIssue_ColumnTypeDatabaseTypeName(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(context.Background(),
		"SELECT 1 AS id, 'hello' AS name FROM DUAL")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	cols, err := rows.ColumnTypes()
	if err != nil {
		t.Fatalf("ColumnTypes: %v", err)
	}
	for _, col := range cols {
		if got := col.DatabaseTypeName(); got == "" {
			t.Errorf("col %q: DatabaseTypeName() = %q, want non-empty string", col.Name(), got)
		}
	}
}
func TestIssue_ColumnTypeDatabaseCharTypeName(t *testing.T) {
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
	table := createObjectName("char_test")
	cols := map[string]string{
		"code":       "CHAR(5)",
		"name_vc":    "VARCHAR2(20)",
		"name_nvc":   "NVARCHAR2(20)",
		"country_nc": "NCHAR(3)",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Insert using normal (literal) statements – no bind variables
	inserts := []string{
		"INSERT INTO " + table + " (code, name_vc, name_nvc, country_nc) VALUES ('A1', 'John', 'do', 'US')",
	}
	for i, sql := range inserts {
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	q := "SELECT code, name_vc, name_nvc, country_nc " +
		"FROM " + table + " ORDER BY code"

	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	expectedTypeNames := []string{
		"CHAR",
		"VARCHAR2",
		"NVARCHAR2",
		"NCHAR",
	}

	cts, err := rs.ColumnTypes()
	if err != nil {
		t.Fatalf("ColumnTypes: %v", err)
	}
	var idx = 0
	for _, col := range cts {
		if got := col.DatabaseTypeName(); strings.Compare(expectedTypeNames[idx], got) != 0 {
			t.Errorf("col %q: DatabaseTypeName() = %q, want [%v]", col.Name(), got, expectedTypeNames[idx])
		}
		idx++
	}

}

func TestIssue_ColumnTypePrecisionScale(t *testing.T) {
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
	table := createObjectName("issue_col_type_precision_scale")
	_ = dropTable(ctx, db, table)
	colsDef := map[string]string{
		"amount":     "NUMBER(5,2)",
		"whole":      "NUMBER(12)",
		"ratio":      "NUMBER(9,4)",
		"created_at": "TIMESTAMP",
		"created_on": "DATE",
		"name":       "VARCHAR2(10)",
	}
	if err := createTable(ctx, db, table, colsDef); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	rows, err := db.QueryContext(ctx,
		"SELECT amount, whole, ratio, created_at, created_on, name FROM "+table)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	expectedPrecisionScale := []struct {
		name      string
		precision int64
		scale     int64
		ok        bool
	}{
		{name: "AMOUNT", precision: 5, scale: 2, ok: true},
		{name: "WHOLE", precision: 12, scale: 0, ok: true},
		{name: "RATIO", precision: 9, scale: 4, ok: true},
		{name: "CREATED_AT", precision: 0, scale: 0, ok: false},
		{name: "CREATED_ON", precision: 0, scale: 0, ok: false},
		{name: "NAME", precision: 0, scale: 0, ok: false},
	}

	cols, err := rows.ColumnTypes()
	if err != nil {
		t.Fatalf("ColumnTypes: %v", err)
	}

	if len(cols) != len(expectedPrecisionScale) {
		t.Fatalf("ColumnTypes() returned %d columns, want %d", len(cols), len(expectedPrecisionScale))
	}

	for i, col := range cols {
		precision, scale, ok := col.DecimalSize()
		want := expectedPrecisionScale[i]
		if strings.ToUpper(col.Name()) != want.name {
			t.Fatalf("column %d name = %q, want %q", i, col.Name(), want.name)
		}
		if precision != want.precision || scale != want.scale || ok != want.ok {
			t.Errorf(
				"col %q: DecimalSize() = (%d, %d, %t), want (%d, %d, %t)",
				col.Name(),
				precision,
				scale,
				ok,
				want.precision,
				want.scale,
				want.ok,
			)
		}
	}
}

// TestDriver_InsertForeignKeyViolation verifies that a DML RETURNING statement
// fails cleanly when Oracle rejects the row for a missing parent key.
//
// The test creates parent and child tables, adds a child-to-parent foreign key,
// then inserts a child row whose parent_id does not exist. The INSERT also uses
// RETURNING INTO so the driver exercises the DML error path that may otherwise
// try to decode returned bind data. The expected result is ORA-02291, and the
// output bind must remain unchanged because the row was not inserted.
func TestDriver_InsertForeignKeyViolation(t *testing.T) {
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
	parentTable := createObjectName("fk_parent_test")
	childTable := createObjectName("fk_child_test")

	_ = dropTable(ctx, db, childTable)
	_ = dropTable(ctx, db, parentTable)

	if err := createTable(ctx, db, parentTable, map[string]string{
		"id": "NUMBER PRIMARY KEY",
	}); err != nil {
		t.Fatalf("create parent table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, parentTable); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", parentTable, err)
		}
	}()

	if err := createTable(ctx, db, childTable, map[string]string{
		"id":        "NUMBER PRIMARY KEY",
		"parent_id": "NUMBER",
	}); err != nil {
		t.Fatalf("create child table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, childTable); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", childTable, err)
		}
	}()

	constraintName := createObjectName("fk_child_parent_test")
	if _, err := db.ExecContext(ctx,
		"ALTER TABLE "+childTable+" ADD CONSTRAINT "+constraintName+
			" FOREIGN KEY (parent_id) REFERENCES "+parentTable+" (id)"); err != nil {
		t.Fatalf("add foreign key constraint failed: %v", err)
	}

	var insertedChildID int64
	_, err = db.ExecContext(ctx,
		"INSERT INTO "+childTable+" (id, parent_id) VALUES (:1, :2) RETURNING id INTO :3",
		1,
		9999999+2,
		sql.Out{Dest: &insertedChildID},
	)
	if err == nil {
		t.Fatalf("expected foreign key violation, got nil error")
	}

	errText := err.Error()
	if !strings.Contains(errText, string(oracleErrors.ForeignKeyViolation)) {
		t.Fatalf("expected ORA-02291 foreign key violation, got: %v", err)
	}
	if insertedChildID != 0 {
		t.Fatalf("expected returned child id to remain unset on foreign key violation, got %d", insertedChildID)
	}
}

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

// TestDriver_ConstraintViolationsReturnOracleErrors checks representative
// uniqueness, CHECK, and NOT NULL failures and verifies the connection remains
// usable after the server rejects each statement.
func TestDriver_ConstraintViolationsReturnOracleErrors(t *testing.T) {
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
	table := createObjectName("constraint_test")
	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"amount": "NUMBER CHECK (amount >= 0)",
		"label":  "VARCHAR2(20) NOT NULL",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("drop table: %v", err)
		}
	}()
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, amount, label) VALUES (:1, :2, :3)", 1, 10, "original"); err != nil {
		t.Fatalf("insert seed row: %v", err)
	}

	cases := []struct {
		name   string
		id     any
		amount any
		label  any
		code   string
	}{
		{name: "duplicate primary key", id: 1, amount: 20, label: "duplicate", code: "ORA-00001"},
		{name: "check constraint", id: 2, amount: -1, label: "negative", code: "ORA-02290"},
		{name: "not null constraint", id: 3, amount: 30, label: nil, code: "ORA-01400"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, amount, label) VALUES (:1, :2, :3)", tc.id, tc.amount, tc.label)
			if err == nil {
				t.Fatalf("insert succeeded, want %s", tc.code)
			}
			sqlErr, ok := err.(oracleErrors.SQLError)
			if !ok {
				t.Fatalf("error %T does not expose Oracle error code: %v", err, err)
			}
			if sqlErr.ErrorCode() != tc.code {
				t.Fatalf("ErrorCode() = %s, want %s (%v)", sqlErr.ErrorCode(), tc.code, err)
			}
		})
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count rows after constraint failures: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count after failed inserts = %d, want 1", count)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, amount, label) VALUES (:1, :2, :3)", 4, 40, "connection reused"); err != nil {
		t.Fatalf("valid insert after constraint failures: %v", err)
	}
}
