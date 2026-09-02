// Tests migrated from OraHub MRs.

package oracle

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func TestDriver_Select_BinaryDouble_SpecialValues(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("binary_double_special_values_test")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"bin_dbl": "BINARY_DOUBLE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	insertAll := "INSERT ALL\n" +
		"INTO " + table + " (id, bin_dbl) VALUES (1, BINARY_DOUBLE_INFINITY)\n" +
		"INTO " + table + " (id, bin_dbl) VALUES (2, -BINARY_DOUBLE_INFINITY)\n" +
		"INTO " + table + " (id, bin_dbl) VALUES (3, BINARY_DOUBLE_NAN)\n" +
		"SELECT 1 FROM DUAL"
	if _, err := db.ExecContext(ctx, insertAll); err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT id, bin_dbl FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	t.Cleanup(func() {
		if err := rows.Close(); err != nil {
			t.Errorf("cleanup close rows: %v", err)
		}
	})

	type rowExp struct {
		id      int
		isInf   bool
		infSign int
		isNaN   bool
	}
	exp := []rowExp{
		{id: 1, isInf: true, infSign: 1},
		{id: 2, isInf: true, infSign: -1},
		{id: 3, isNaN: true},
	}

	idx := 0
	for rows.Next() {
		var (
			id  int
			val float64
		)
		if err := rows.Scan(&id, &val); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		want := exp[idx]
		if id != want.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, want.id)
		}
		switch {
		case want.isNaN:
			if !math.IsNaN(val) {
				t.Fatalf("BINARY_DOUBLE mismatch at row %d: expected NaN, got %v", idx, val)
			}
		case want.isInf:
			if !math.IsInf(val, want.infSign) {
				t.Fatalf("BINARY_DOUBLE mismatch at row %d: expected Inf sign %d, got %v", idx, want.infSign, val)
			}
		default:
			t.Fatalf("invalid test expectation at row %d", idx)
		}
		idx++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

func TestDriver_Prepared_BinaryDouble_SpecialValues_RoundTrip(t *testing.T) {
	t.Parallel()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup close database: %v", err)
		}
	})

	ctx := context.Background()
	table := createObjectName("binary_double_bind_roundtrip_test")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"bin_dbl": "BINARY_DOUBLE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	ins, err := db.PrepareContext(ctx, "INSERT INTO "+table+" (id, bin_dbl) VALUES (:1, :2)")
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() {
		if err := ins.Close(); err != nil {
			t.Errorf("cleanup close insert statement: %v", err)
		}
	})

	type rejectCase struct {
		name string
		id   int
		val  float64
	}
	rejects := []rejectCase{
		{name: "+Inf", id: 1, val: math.Inf(1)},
		{name: "-Inf", id: 2, val: math.Inf(-1)},
		{name: "NaN", id: 3, val: math.NaN()},
	}

	for _, tc := range rejects {
		_, err := ins.ExecContext(ctx, tc.id, tc.val)
		if err == nil {
			t.Fatalf("insert %s should fail for non-finite bind", tc.name)
		}
		var sqle oracleErrors.SQLError
		if !errors.As(err, &sqle) {
			t.Fatalf("insert %s error is not SQLError: %v", tc.name, err)
		}
		if sqle.ErrorCode() != string(oracleErrors.ConverterExpectedFormat) {
			t.Fatalf("insert %s wrong error code: got %s want %s", tc.name, sqle.ErrorCode(), oracleErrors.ConverterExpectedFormat)
		}
		if !strings.Contains(err.Error(), "finite number") {
			t.Fatalf("insert %s wrong error text: %v", tc.name, err)
		}
	}

	if _, err := ins.ExecContext(ctx, 4, math.Copysign(0, -1)); err != nil {
		t.Fatalf("insert negative zero failed: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT id, bin_dbl FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	t.Cleanup(func() {
		if err := rows.Close(); err != nil {
			t.Errorf("cleanup close rows: %v", err)
		}
	})

	rowCount := 0
	for rows.Next() {
		rowCount++
		var (
			id  int
			val float64
		)
		if err := rows.Scan(&id, &val); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if id != 4 {
			t.Fatalf("unexpected row id after rejected binds: got %d want 4", id)
		}
		if val != 0 {
			t.Fatalf("BINARY_DOUBLE mismatch for negative zero row: expected zero, got %v", val)
		}
		if math.Signbit(val) {
			t.Fatalf("negative zero should be normalized to unsigned zero, got signbit=true")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("row count mismatch: got %d want 1", rowCount)
	}
}
