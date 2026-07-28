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
	"time"
)

func TestDriver_SQLNullTypes_BindInputs(t *testing.T) {
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
	table := createObjectName("t_sql_null_bind_inputs")
	cols := map[string]string{
		"id":            "number",
		"name":          "varchar2(100)",
		"amount":        "number",
		"created_on":    "date",
		"created_at":    "timestamp",
		"small_num":     "number",
		"tiny_num":      "number",
		"tiny_byte_num": "number",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	expectedDate := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	expectedTimestamp := time.Date(2026, time.January, 10, 11, 12, 13, 0, time.UTC)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO `+table+` (
			id, name, amount, created_on, created_at, small_num, tiny_num, tiny_byte_num
		) VALUES (
			:id, :name, :amount, :created_on, :created_at, :small_num, :tiny_num, :tiny_byte_num
		)`,
		sql.Named("id", sql.NullInt64{Int64: 501, Valid: true}),
		sql.Named("name", sql.NullString{String: "null-bind-input", Valid: true}),
		sql.Named("amount", sql.NullFloat64{Float64: 19.75, Valid: true}),
		sql.Named("created_on", sql.NullTime{Time: expectedDate, Valid: true}),
		sql.Named("created_at", sql.NullTime{Time: expectedTimestamp, Valid: true}),
		sql.Named("small_num", sql.NullInt32{Int32: 32, Valid: true}),
		sql.Named("tiny_num", sql.NullInt16{Int16: 16, Valid: true}),
		sql.Named("tiny_byte_num", sql.NullByte{Byte: 8, Valid: true}),
	); err != nil {
		t.Fatalf("insert with sql.Null* bind inputs failed: %v", err)
	}

	var (
		gotID        int64
		gotName      string
		gotAmount    float64
		gotCreatedOn time.Time
		gotCreatedAt time.Time
		gotSmallNum  int64
		gotTinyNum   int64
		gotByteNum   int64
	)
	if err := db.QueryRowContext(ctx,
		`SELECT id, name, amount, created_on, created_at, small_num, tiny_num, tiny_byte_num
		 FROM `+table+` WHERE id = :1`,
		int64(501),
	).Scan(
		&gotID,
		&gotName,
		&gotAmount,
		&gotCreatedOn,
		&gotCreatedAt,
		&gotSmallNum,
		&gotTinyNum,
		&gotByteNum,
	); err != nil {
		t.Fatalf("select inserted row failed: %v", err)
	}

	if gotID != 501 {
		t.Fatalf("unexpected id: got %d want 501", gotID)
	}
	if gotName != "null-bind-input" {
		t.Fatalf("unexpected name: got %q want %q", gotName, "null-bind-input")
	}
	if gotAmount != 19.75 {
		t.Fatalf("unexpected amount: got %v want %v", gotAmount, 19.75)
	}
	if gotSmallNum != 32 {
		t.Fatalf("unexpected small_num: got %d want 32", gotSmallNum)
	}
	if gotTinyNum != 16 {
		t.Fatalf("unexpected tiny_num: got %d want 16", gotTinyNum)
	}
	if gotByteNum != 8 {
		t.Fatalf("unexpected tiny_byte_num: got %d want 8", gotByteNum)
	}
	assertSameYMDHMSNanos(t, 0, "DATE bind input", gotCreatedOn, expectedDate)
	assertSameYMDHMSNanos(t, 0, "TIMESTAMP bind input", gotCreatedAt, expectedTimestamp)
}

func TestDriver_SQLNullTypes_DMLReturning_OutDest(t *testing.T) {
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
	table := createObjectName("t_sql_null_dml_ret")
	cols := map[string]string{
		"id":            "number",
		"name":          "varchar2(100)",
		"amount":        "binary_double",
		"created_at":    "timestamp",
		"small_num":     "number",
		"tiny_num":      "number",
		"tiny_byte_num": "number",
	}
	dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	expectedTimestamp := time.Date(2026, time.February, 11, 9, 8, 7, 0, time.UTC)

	stmt, err := db.PrepareContext(ctx,
		`INSERT INTO `+table+` (
			id, name, amount, created_at, small_num, tiny_num, tiny_byte_num
		) VALUES (
			:1, :2, :3, :4, :5, :6, :7
		) RETURNING
			id, name, amount, created_at, small_num, tiny_num, tiny_byte_num
		INTO
			:8, :9, :10, :11, :12, :13, :14`)
	if err != nil {
		t.Fatalf("prepare insert returning failed: %v", err)
	}
	defer stmt.Close()

	var (
		returnedID        sql.NullInt64
		returnedName      sql.NullString
		returnedAmount    sql.NullFloat64
		returnedCreatedAt sql.NullTime
		returnedSmallNum  sql.NullInt32
		returnedTinyNum   sql.NullInt16
		returnedByteNum   sql.NullByte
	)

	result, err := stmt.ExecContext(
		ctx,
		int64(601),
		"null-returning",
		19.75,
		expectedTimestamp,
		int32(31),
		int16(15),
		byte(7),
		sql.Out{Dest: &returnedID},
		sql.Out{Dest: &returnedName},
		sql.Out{Dest: &returnedAmount},
		sql.Out{Dest: &returnedCreatedAt},
		sql.Out{Dest: &returnedSmallNum},
		sql.Out{Dest: &returnedTinyNum},
		sql.Out{Dest: &returnedByteNum},
	)
	if err != nil {
		t.Fatalf("exec insert returning into sql.Null* destinations failed: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("rows affected retrieval failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("unexpected rows affected: got %d, want 1", rowsAffected)
	}

	if !returnedID.Valid || returnedID.Int64 != 601 {
		t.Fatalf("unexpected returned id: %+v", returnedID)
	}
	if !returnedName.Valid || returnedName.String != "null-returning" {
		t.Fatalf("unexpected returned name: %+v", returnedName)
	}
	if !returnedAmount.Valid || returnedAmount.Float64 != 19.75 {
		t.Fatalf("unexpected returned amount: %+v", returnedAmount)
	}
	if !returnedCreatedAt.Valid {
		t.Fatalf("expected returned timestamp to be valid")
	}
	if !returnedSmallNum.Valid || returnedSmallNum.Int32 != 31 {
		t.Fatalf("unexpected returned small_num: %+v", returnedSmallNum)
	}
	if !returnedTinyNum.Valid || returnedTinyNum.Int16 != 15 {
		t.Fatalf("unexpected returned tiny_num: %+v", returnedTinyNum)
	}
	if !returnedByteNum.Valid || returnedByteNum.Byte != 7 {
		t.Fatalf("unexpected returned tiny_byte_num: %+v", returnedByteNum)
	}
	assertSameYMDHMSNanos(t, 0, "RETURNING timestamp", returnedCreatedAt.Time, expectedTimestamp)
}

func TestDriver_SQLNullTypes_PLSQL_InOut(t *testing.T) {
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

	t.Run("varchar", func(t *testing.T) {
		value := sql.NullString{String: "plsql", Valid: true}
		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 || '-updated'; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullString IN OUT failed: %v", err)
		}
		if !value.Valid || value.String != "plsql-updated" {
			t.Fatalf("unexpected sql.NullString result: %+v", value)
		}
	})

	t.Run("int64", func(t *testing.T) {
		value := sql.NullInt64{Int64: 700, Valid: true}
		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 + 1; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullInt64 IN OUT failed: %v", err)
		}
		if !value.Valid || value.Int64 != 701 {
			t.Fatalf("unexpected sql.NullInt64 result: %+v", value)
		}
	})

	t.Run("int32", func(t *testing.T) {
		value := sql.NullInt32{Int32: 320, Valid: true}
		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 + 1; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullInt32 IN OUT failed: %v", err)
		}
		if !value.Valid || value.Int32 != 321 {
			t.Fatalf("unexpected sql.NullInt32 result: %+v", value)
		}
	})

	t.Run("int16", func(t *testing.T) {
		value := sql.NullInt16{Int16: 160, Valid: true}
		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 + 1; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullInt16 IN OUT failed: %v", err)
		}
		if !value.Valid || value.Int16 != 161 {
			t.Fatalf("unexpected sql.NullInt16 result: %+v", value)
		}
	})

	t.Run("byte", func(t *testing.T) {
		value := sql.NullByte{Byte: 9, Valid: true}
		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 + 1; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullByte IN OUT failed: %v", err)
		}
		if !value.Valid || value.Byte != 10 {
			t.Fatalf("unexpected sql.NullByte result: %+v", value)
		}
	})

	t.Run("float64", func(t *testing.T) {
		value := sql.NullFloat64{Float64: 40.5, Valid: true}
		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 + 1.25; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullFloat64 IN OUT failed: %v", err)
		}
		if !value.Valid || value.Float64 != 41.75 {
			t.Fatalf("unexpected sql.NullFloat64 result: %+v", value)
		}
	})

	t.Run("time", func(t *testing.T) {
		value := sql.NullTime{
			Time:  time.Date(2026, time.March, 3, 4, 5, 6, 0, time.UTC),
			Valid: true,
		}
		expected := value.Time.Add(time.Hour)

		if _, err := db.ExecContext(ctx,
			"BEGIN :1 := :1 + INTERVAL '1' HOUR; END;",
			sql.Out{Dest: &value, In: true},
		); err != nil {
			t.Fatalf("PL/SQL sql.NullTime IN OUT failed: %v", err)
		}
		if !value.Valid {
			t.Fatalf("expected sql.NullTime to remain valid")
		}
		assertSameYMDHMSNanos(t, 0, "PL/SQL sql.NullTime IN OUT", value.Time, expected)
	})
}

func TestDriver_SQLNullTypes_PLSQL_InOut_UsingSetupObjects(t *testing.T) {
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
	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	defer dropPlsqlInOutObjects(ctx, db, objects)

	name := sql.NullString{String: "sql-null", Valid: true}
	amount := sql.NullFloat64{Float64: 20.5, Valid: true}
	createdOn := sql.NullTime{
		Time:  time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
	createdAt := sql.NullTime{
		Time:  time.Date(2026, time.April, 5, 6, 7, 8, 123000000, time.UTC),
		Valid: true,
	}
	updatedAtInput := time.Date(2026, time.April, 5, 6, 7, 8, 456000000, time.FixedZone("+05:30", 5*60*60+30*60))
	updatedAt := sql.NullTime{
		Time:  updatedAtInput,
		Valid: true,
	}

	_, err = db.ExecContext(ctx,
		"BEGIN "+objects.updateAllTypes+"(:1, :2, :3, :4, :5, :6, :7); END;",
		int64(200),
		sql.Out{Dest: &name, In: true},
		sql.Out{Dest: &amount, In: true},
		sql.Out{Dest: &sql.NullInt64{Int64: 1, Valid: true}, In: true},
		sql.Out{Dest: &createdOn, In: true},
		sql.Out{Dest: &createdAt, In: true},
		sql.Out{Dest: &updatedAt, In: true},
	)
	if err != nil {
		t.Fatalf("PL/SQL setup-object IN OUT with sql.Null* failed: %v", err)
	}

	if !name.Valid || name.String != "sql-null-updated" {
		t.Fatalf("unexpected sql.NullString result: %+v", name)
	}
	if !amount.Valid || amount.Float64 != 22 {
		t.Fatalf("unexpected sql.NullFloat64 result: %+v", amount)
	}
	if !createdOn.Valid {
		t.Fatalf("expected created_on to remain valid")
	}
	if !createdAt.Valid {
		t.Fatalf("expected created_at to remain valid")
	}
	if !updatedAt.Valid {
		t.Fatalf("expected updated_at to remain valid")
	}

	assertSameYMDHMSNanos(t, 0, "PL/SQL DATE IN OUT via setup objects", createdOn.Time, time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC))
	assertSameYMDHMSNanos(t, 0, "PL/SQL TIMESTAMP IN OUT via setup objects", createdAt.Time, time.Date(2026, time.April, 5, 7, 7, 8, 123000000, time.UTC))
	assertSameYMDHMSNanos(t, 0, "PL/SQL TSTZ IN OUT via setup objects", updatedAt.Time, updatedAtInput.Add(90*time.Minute))
}

func TestDriver_SQLNullTypes_PLSQL_InOut_NullInputs(t *testing.T) {
	//t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	objects := newPlsqlInOutObjectNames()
	if err := setupPlsqlInOutObjects(ctx, db, objects); err != nil {
		t.Fatalf("setup PL/SQL objects failed: %v", err)
	}
	defer dropPlsqlInOutObjects(ctx, db, objects)

	name := sql.NullString{}
	count := sql.NullInt64{}
	amount := sql.NullFloat64{}
	atTime := sql.NullTime{}

	_, err = db.ExecContext(ctx,
		"BEGIN "+objects.updateNullInputs+"(:1, :2, :3, :4); END;",
		sql.Out{Dest: &name, In: true},
		sql.Out{Dest: &count, In: true},
		sql.Out{Dest: &amount, In: true},
		sql.Out{Dest: &atTime, In: true},
	)
	if err != nil {
		t.Fatalf("PL/SQL sql.Null* IN OUT with null inputs failed: %v", err)
	}

	if !name.Valid || name.String != "was-null" {
		t.Fatalf("unexpected sql.NullString result for null input: %+v", name)
	}
	if !count.Valid || count.Int64 != 1 {
		t.Fatalf("unexpected sql.NullInt64 result for null input: %+v", count)
	}
	if !amount.Valid || amount.Float64 != 1.25 {
		t.Fatalf("unexpected sql.NullFloat64 result for null input: %+v", amount)
	}
	if !atTime.Valid {
		t.Fatalf("expected sql.NullTime to become valid for null input")
	}
	assertSameYMDHMSNanos(t, 0, "PL/SQL sql.NullTime null input", atTime.Time, time.Date(2026, time.May, 1, 10, 11, 12, 0, time.UTC))
}
