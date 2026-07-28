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
	"testing"
	"time"
)

func TestDriver_PLSQL_InOut_NumberFunction(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	var got int64
	_, err = db.ExecContext(ctx,
		"BEGIN :1 := "+objects.numberFunction+"(:2); END;",
		sql.Out{Dest: &got},
		int64(101),
	)
	if err != nil {
		t.Fatalf("PL/SQL NUMBER IN/OUT failed: %v", err)
	}

	if got != 102 {
		t.Fatalf("unexpected NUMBER result: got %d want 102", got)
	}
}

// TestDriver_PLSQL_InOut_NumberFunctionDoubleBind test that for PL/SQL if a
// placeholder appears twice it will still be sent correctly (only once)
func TestDriver_PLSQL_InOut_NumberFunctionDoubleBind(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	got := 1
	_, err = db.ExecContext(ctx,
		"BEGIN :a := :b - :a; END;",
		sql.Named("a", sql.Out{Dest: &got, In: true}),
		sql.Named("b", int64(101)),
	)
	if err != nil {
		t.Fatalf("PL/SQL NUMBER IN/OUT failed: %v", err)
	}

	if got != 100 {
		t.Fatalf("unexpected NUMBER result: got %d want 100", got)
	}

	got = 1
	_, err = db.ExecContext(ctx,
		"BEGIN :a := :a - :b; END;",
		sql.Named("a", sql.Out{Dest: &got, In: true}),
		sql.Named("b", int64(101)),
	)
	if err != nil {
		t.Fatalf("PL/SQL NUMBER IN/OUT failed: %v", err)
	}

	if got != -100 {
		t.Fatalf("unexpected NUMBER result: got %d want -100", got)
	}
}

func TestDriver_PLSQL_InOut_VarcharProcedure(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	var got string
	_, err = db.ExecContext(ctx,
		"BEGIN "+objects.varcharProcedure+"(:1, :2); END;",
		"plsql",
		sql.Out{Dest: &got},
	)
	if err != nil {
		t.Fatalf("PL/SQL VARCHAR IN/OUT failed: %v", err)
	}

	if got != "plsql-out" {
		t.Fatalf("unexpected VARCHAR result: got %q want %q", got, "plsql-out")
	}
}

func TestDriver_PLSQL_ProcedureWithInOut(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	salary := float64(1000)
	status := "INITIAL"

	_, err = db.ExecContext(ctx,
		"BEGIN "+objects.updateSalary+"(:1, :2, :3); END;",
		int64(100),
		sql.Out{Dest: &salary, In: true},
		sql.Out{Dest: &status, In: true},
	)
	if err != nil {
		t.Fatalf("PL/SQL procedure IN OUT failed: %v", err)
	}

	if salary != 1100 {
		t.Fatalf("unexpected salary after IN OUT call: got %v want 1100", salary)
	}
	if status != "INITIAL-UPDATED" {
		t.Fatalf("unexpected status after IN OUT call: got %q want %q", status, "INITIAL-UPDATED")
	}

	var storedSalary float64
	if err := db.QueryRowContext(ctx,
		"SELECT salary FROM "+objects.employeesTable+" WHERE employee_id = :1",
		int64(100),
	).Scan(&storedSalary); err != nil {
		t.Fatalf("query updated salary failed: %v", err)
	}

	if storedSalary != 1100 {
		t.Fatalf("unexpected persisted salary: got %v want 1100", storedSalary)
	}
}

func TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	stmt, err := db.PrepareContext(ctx, "BEGIN :1 := "+objects.numberFunction+"(:2); END;")
	if err != nil {
		t.Fatalf("prepare PL/SQL NUMBER IN/OUT failed: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	cases := []struct {
		in   int64
		want int64
	}{
		{in: 101, want: 102},
		{in: 205, want: 206},
		{in: 301, want: 302},
		{in: 305, want: 306},
	}

	for idx, tc := range cases {
		var got int64
		if _, err := stmt.ExecContext(ctx, sql.Out{Dest: &got}, tc.in); err != nil {
			t.Fatalf("re-execution %d failed: %v", idx, err)
		}
		if got != tc.want {
			t.Fatalf("re-execution %d returned %d, want %d", idx, got, tc.want)
		}
	}
}

func TestDriver_PLSQL_InOut_NumberFunction_ReExecuteSameStatement_NamedBinds(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	stmt, err := db.PrepareContext(ctx, "BEGIN :ret := "+objects.numberFunction+"(:in); END;")
	if err != nil {
		t.Fatalf("prepare named-bind PL/SQL NUMBER IN/OUT failed: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	cases := []struct {
		in   int64
		want int64
	}{
		{in: 401, want: 402},
		{in: 505, want: 506},
	}

	for idx, tc := range cases {
		var got int64
		if _, err := stmt.ExecContext(ctx,
			sql.Named("ret", sql.Out{Dest: &got}),
			sql.Named("in", tc.in),
		); err != nil {
			t.Fatalf("named-bind re-execution %d failed: %v", idx, err)
		}
		if got != tc.want {
			t.Fatalf("named-bind re-execution %d returned %d, want %d", idx, got, tc.want)
		}
	}
}

func TestDriver_PLSQL_ProcedureWithInOut_AllTypes(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	t.Cleanup(func() { dropPlsqlInOutObjects(ctx, db, objects) })

	name := "initial-name"
	amount := 10.5
	isActive := int64(1)

	inputCreatedOn := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	inputCreatedAt := time.Date(2026, time.January, 2, 3, 4, 5, 123000000, time.UTC)
	inputUpdatedAt := time.Date(2026, time.January, 2, 3, 4, 5, 456000000, time.FixedZone("+05:30", 5*60*60+30*60))

	createdOn := inputCreatedOn
	createdAt := inputCreatedAt
	updatedAt := inputUpdatedAt

	_, err = db.ExecContext(ctx,
		"BEGIN "+objects.updateAllTypes+"(:1, :2, :3, :4, :5, :6, :7); END;",
		int64(200),
		sql.Out{Dest: &name, In: true},
		sql.Out{Dest: &amount, In: true},
		sql.Out{Dest: &isActive, In: true},
		sql.Out{Dest: &createdOn, In: true},
		sql.Out{Dest: &createdAt, In: true},
		sql.Out{Dest: &updatedAt, In: true},
	)
	if err != nil {
		t.Fatalf("PL/SQL all-types IN OUT failed: %v", err)
	}

	if name != "initial-name-updated" {
		t.Fatalf("unexpected name after IN OUT call: got %q want %q", name, "initial-name-updated")
	}
	if amount != 12 {
		t.Fatalf("unexpected amount after IN OUT call: got %v want 12", amount)
	}
	if isActive != 1 {
		t.Fatalf("unexpected is_active after IN OUT call: got %v want 1", isActive)
	}

	wantCreatedOn := inputCreatedOn.AddDate(0, 0, 1)
	assertSameYMDHMSNanos(t, 0, "DATE IN OUT", createdOn, wantCreatedOn)

	wantCreatedAt := inputCreatedAt.Add(time.Hour)
	assertSameYMDHMSNanos(t, 0, "TIMESTAMP IN OUT", createdAt, wantCreatedAt)

	wantUpdatedAt := inputUpdatedAt.Add(90 * time.Minute)
	assertSameYMDHMSNanos(t, 0, "TSTZ IN OUT", updatedAt, wantUpdatedAt)
	if _, off := updatedAt.Zone(); off != 5*3600+30*60 {
		t.Fatalf("unexpected updated_at zone offset after IN OUT call: got %d want %d", off, 5*3600+30*60)
	}

	var (
		storedName      string
		storedAmount    float64
		storedIsActive  int64
		storedCreatedOn time.Time
		storedCreatedAt time.Time
		storedUpdatedAt time.Time
	)
	if err := db.QueryRowContext(ctx,
		"SELECT name, amount, is_active, created_on, created_at, updated_at FROM "+objects.allTypesTable+" WHERE id = :1",
		int64(200),
	).Scan(
		&storedName,
		&storedAmount,
		&storedIsActive,
		&storedCreatedOn,
		&storedCreatedAt,
		&storedUpdatedAt,
	); err != nil {
		t.Fatalf("query updated all-types row failed: %v", err)
	}

	if storedName != name {
		t.Fatalf("unexpected persisted name: got %q want %q", storedName, name)
	}
	if storedAmount != amount {
		t.Fatalf("unexpected persisted amount: got %v want %v", storedAmount, amount)
	}
	if storedIsActive != isActive {
		t.Fatalf("unexpected persisted is_active: got %v want %v", storedIsActive, isActive)
	}
	assertSameYMDHMSNanos(t, 0, "DATE persisted", storedCreatedOn, createdOn)
	assertSameYMDHMSNanos(t, 0, "TIMESTAMP persisted", storedCreatedAt, createdAt)
	assertSameYMDHMSNanos(t, 0, "TSTZ persisted", storedUpdatedAt, updatedAt)
	if _, off := storedUpdatedAt.Zone(); off != 5*3600+30*60 {
		t.Fatalf("unexpected persisted updated_at zone offset: got %d want %d", off, 5*3600+30*60)
	}
}

type plsqlInOutObjectNames struct {
	numberFunction   string
	varcharProcedure string
	updateSalary     string
	updateAllTypes   string
	updateNullInputs string
	employeesTable   string
	allTypesTable    string
}

func newPlsqlInOutObjectNames() plsqlInOutObjectNames {
	return plsqlInOutObjectNames{
		numberFunction:   createObjectName("plsql_in_out_number"),
		varcharProcedure: createObjectName("plsql_in_out_varchar"),
		updateSalary:     createObjectName("update_salary"),
		updateAllTypes:   createObjectName("update_all_types"),
		updateNullInputs: createObjectName("update_null_inputs"),
		employeesTable:   createObjectName("employees"),
		allTypesTable:    createObjectName("plsql_inout_all_types"),
	}
}

func setupPlsqlInOutObjects(ctx context.Context, db *sql.DB, objects plsqlInOutObjectNames) error {
	dropPlsqlInOutObjects(ctx, db, objects)

	createStatements := []string{
		fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s(p_in IN NUMBER)
			RETURN NUMBER IS
			BEGIN
  				RETURN p_in + 1;
			END;`, objects.numberFunction),
		fmt.Sprintf(`CREATE OR REPLACE PROCEDURE %s(
  			p_in IN VARCHAR2,
  			p_out OUT VARCHAR2
		) IS			
		BEGIN
  			p_out := p_in || '-out';
		END;`, objects.varcharProcedure),
		fmt.Sprintf(`CREATE TABLE %s (
  			employee_id NUMBER PRIMARY KEY,
  			salary NUMBER
		)`, objects.employeesTable),
		fmt.Sprintf(`INSERT INTO %s (employee_id, salary) VALUES (100, 1000)`, objects.employeesTable),
		fmt.Sprintf(`CREATE OR REPLACE PROCEDURE %s (
    		p_emp_id   IN     NUMBER,
    		p_salary   IN OUT NUMBER,
    		p_status   IN OUT VARCHAR2
		) AS
		BEGIN
    		p_salary := p_salary * 1.10;
    		p_status := p_status || '-UPDATED';
			UPDATE %s
			SET salary = p_salary
			WHERE employee_id = p_emp_id;
		END;`, objects.updateSalary, objects.employeesTable),
		fmt.Sprintf(`CREATE TABLE %s (
			id NUMBER PRIMARY KEY,
			name VARCHAR2(100),
			amount BINARY_DOUBLE,
			is_active NUMBER(1),
			created_on DATE,
			created_at TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE
		)`, objects.allTypesTable),
		fmt.Sprintf(`INSERT INTO %s (
			id, name, amount, is_active, created_on, created_at, updated_at
		) VALUES (
			200,
			'seed',
			0,
			0,
			DATE '2026-01-01',
			TIMESTAMP '2026-01-01 00:00:00',
			TIMESTAMP '2026-01-01 00:00:00 +00:00'
		)`, objects.allTypesTable),
		fmt.Sprintf(`CREATE OR REPLACE PROCEDURE %s (
			p_id         IN     NUMBER,
			p_name       IN OUT VARCHAR2,
			p_amount     IN OUT BINARY_DOUBLE,
			p_is_active  IN OUT NUMBER,
			p_created_on IN OUT DATE,
			p_created_at IN OUT TIMESTAMP,
			p_updated_at IN OUT TIMESTAMP WITH TIME ZONE
		) AS
		BEGIN
			p_name := p_name || '-updated';
			p_amount := p_amount + 1.5;
			p_is_active := 1;
			p_created_on := p_created_on + 1;
			p_created_at := p_created_at + INTERVAL '1' HOUR;
			p_updated_at := p_updated_at + INTERVAL '90' MINUTE;

			UPDATE %s
			SET name = p_name,
				amount = p_amount,
				is_active = p_is_active,
				created_on = p_created_on,
				created_at = p_created_at,
				updated_at = p_updated_at
			WHERE id = p_id;
		END;`, objects.updateAllTypes, objects.allTypesTable),
		fmt.Sprintf(`CREATE OR REPLACE PROCEDURE %s (
			p_name       IN OUT VARCHAR2,
			p_count      IN OUT NUMBER,
			p_amount     IN OUT BINARY_DOUBLE,
			p_created_at IN OUT TIMESTAMP
		) AS
		BEGIN
			IF p_name IS NULL THEN
				p_name := 'was-null';
			ELSE
				p_name := p_name || '-updated';
			END IF;

			IF p_count IS NULL THEN
				p_count := 1;
			ELSE
				p_count := p_count + 1;
			END IF;

			IF p_amount IS NULL THEN
				p_amount := 1.25;
			ELSE
				p_amount := p_amount + 1.25;
			END IF;

			IF p_created_at IS NULL THEN
				p_created_at := TIMESTAMP '2026-05-01 10:11:12';
			ELSE
				p_created_at := p_created_at + INTERVAL '1' HOUR;
			END IF;
		END;`, objects.updateNullInputs),
	}

	for _, stmt := range createStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}

func dropPlsqlInOutObjects(ctx context.Context, db *sql.DB, objects plsqlInOutObjectNames) {
	for _, stmt := range []string{
		"DROP FUNCTION " + objects.numberFunction,
		"DROP PROCEDURE " + objects.varcharProcedure,
		"DROP PROCEDURE " + objects.updateSalary,
		"DROP PROCEDURE " + objects.updateAllTypes,
		"DROP PROCEDURE " + objects.updateNullInputs,
		"DROP TABLE " + objects.employeesTable + " PURGE",
		"DROP TABLE " + objects.allTypesTable + " PURGE",
	} {
		_, _ = db.ExecContext(ctx, stmt)
	}
}
