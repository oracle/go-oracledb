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
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestDriver_Select_NumericFloatTypes
// Purpose: End-to-end test for numeric/float family:
//   - NUMBER(12,0) -> integer path (DecodeInt), scanned as int64
//   - NUMBER(10,7) -> decimal path (DecodeDecimal), scanned as string
//   - FLOAT         -> Oracle stores as NUMBER; driver may surface string/int64; we normalize to float64 for compare
//   - BINARY_FLOAT  -> scanned as float64 (IEEE-754 float32 source)
//   - BINARY_DOUBLE -> scanned as float64 (IEEE-754 float64 source)
//
// The test creates a table, inserts representative values (pos/neg/zero/tiny and NaN),
// selects back, logs the values, and validates against expected with appropriate tolerances.
func TestDriver_Select_NumericFloatTypes(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "false"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("numeric_float_types_test")
	cols := map[string]string{
		"id":       "NUMBER PRIMARY KEY",
		"number_i": "NUMBER(12, 0)",
		"number_s": "NUMBER(10, 7)",
		"float1":   "FLOAT",
		"bin1":     "BINARY_FLOAT",
		"bin2":     "BINARY_DOUBLE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	// Batch insert using INSERT ALL to reduce round trips
	insertAll := "INSERT ALL\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (1, 12345, 123.45, 123.456, 123.456, 123.456)\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (2, -987654, -987.6543, -987.654, CAST(-987.654 AS BINARY_FLOAT), CAST(-987.654 AS BINARY_DOUBLE))\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (3, 0, 0, 0, CAST(0 AS BINARY_FLOAT), CAST(0 AS BINARY_DOUBLE))\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (4, 0, 1E-30, 1E-30, CAST(1E-30 AS BINARY_FLOAT), CAST(1E-30 AS BINARY_DOUBLE))\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (5, NULL, NULL, NULL, BINARY_FLOAT_NAN, BINARY_DOUBLE_NAN)\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (6, 1, 0.0000001, 0.0000001, CAST(0.0000001 AS BINARY_FLOAT), CAST(0.0000001 AS BINARY_DOUBLE))\n" +
		"INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (7, TRUNC(123.89), 123.89, 123.89, CAST(123.89 AS BINARY_FLOAT), CAST(123.89 AS BINARY_DOUBLE))\n" +
		"SELECT 1 FROM DUAL"
	if _, err := db.ExecContext(ctx, insertAll); err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}

	q := "SELECT id, number_i, number_s, float1, bin1, bin2 FROM " + table + " ORDER BY id"
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	// Helper to normalize FLOAT column which may arrive as int64 or string from the driver
	toFloat64 := func(v any) (float64, error) {
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		case string:
			return strconv.ParseFloat(x, 64)
		default:
			return 0, fmt.Errorf("unsupported FLOAT scan type %T", v)
		}
	}

	type rowExp struct {
		id      int     // id
		ni      int64   // NUMBER(12,0) as int64
		ns      string  // NUMBER(10,7) as string
		f1      float64 // FLOAT normalized to float64
		bf      float64 // BINARY_FLOAT as float64 (source is float32)
		bd      float64 // BINARY_DOUBLE as float64
		bfIsNaN bool    // whether bf is NaN
		bdIsNaN bool    // whether bd is NaN
	}

	// Helper to compute float32-rounding expectation for BINARY_FLOAT values
	f32 := func(v float64) float64 { return float64(float32(v)) }

	// Expected results
	// Note: Oracle NUMBER(10,7) formatting should provide exactly 7 fractional digits.
	exp := []rowExp{
		{1, 12345, "123.4500000", 123.456, f32(123.456), 123.456, false, false},
		{2, -987654, "-987.6543000", -987.654, f32(-987.654), -987.654, false, false},
		{3, 0, "0.0000000", 0.0, 0.0, 0.0, false, false},
		// For NUMBER(10,7), 1E-30 is below scale and rounds to 0.0000000
		{4, 0, "0.0000000", 1e-30, f32(1e-30), 1e-30, false, false},
		// Driver maps NULL NUMBER/FLOAT to 0 or "0" when scanning nullable numeric (see rows_result.go)
		{5, 0, "0", 0.0, math.NaN(), math.NaN(), true, true},
		{6, 1, "0.0000001", 0.0000001, f32(0.0000001), 0.0000001, false, false},
		{7, 123, "123.8900000", 123.89, f32(123.89), 123.89, false, false},
	}

	const (
		// Tolerances: looser for BINARY_FLOAT (float32 source), tighter for DOUBLE
		tolBF = 1e-6
		tolBD = 1e-12
		// FLOAT column returns NUMBER wire decoded -> normalize; use double tolerance
		tolF1 = tolBD
	)

	equalWithin := func(a, b, tol float64) bool {
		if math.IsNaN(a) && math.IsNaN(b) {
			return true
		}
		return math.Abs(a-b) <= tol
	}

	idx := 0
	for rs.Next() {
		var (
			id int
			ni int64
			ns string
			f1 any
			bf float64
			bd float64
		)
		if err := rs.Scan(&id, &ni, &ns, &f1, &bf, &bd); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		f1f, err := toFloat64(f1)
		if err != nil {
			t.Fatalf("FLOAT scan type error: %v", err)
		}

		// Print all values for visibility
		t.Logf("Row %d: id=%d NUMBER(12,0)=%d NUMBER(10,7)=%q FLOAT=%.7f BINARY_FLOAT=%e BINARY_DOUBLE=%e", idx, id, ni, ns, f1f, bf, bd)

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		w := exp[idx]
		if id != w.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, w.id)
		}
		if ni != w.ni {
			t.Fatalf("NUMBER(12,0) mismatch at row %d: got %d want %d", idx, ni, w.ni)
		}
		if ns != w.ns {
			t.Fatalf("NUMBER(10,7) mismatch at row %d: got %q want %q", idx, ns, w.ns)
		}
		if !equalWithin(f1f, w.f1, tolF1) {
			t.Fatalf("FLOAT mismatch at row %d: got %.12g want %.12g (tol %.1e)", idx, f1f, w.f1, tolF1)
		}
		if w.bfIsNaN {
			if !math.IsNaN(bf) {
				t.Fatalf("BINARY_FLOAT mismatch at row %d: expected NaN, got %v", idx, bf)
			}
		} else if !equalWithin(bf, w.bf, tolBF) {
			t.Fatalf("BINARY_FLOAT mismatch at row %d: got %.9g want %.9g (tol %.1e)", idx, bf, w.bf, tolBF)
		}
		if w.bdIsNaN {
			if !math.IsNaN(bd) {
				t.Fatalf("BINARY_DOUBLE mismatch at row %d: expected NaN, got %v", idx, bd)
			}
		} else if !equalWithin(bd, w.bd, tolBD) {
			t.Fatalf("BINARY_DOUBLE mismatch at row %d: got %.15g want %.15g (tol %.1e)", idx, bd, w.bd, tolBD)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_NumericFloatTypes_Prepared_Named
// Prepared-statement variant of TestDriver_Select_NumericFloatTypes using named binds and Query + Next + Scan.
func TestDriver_Select_NumericFloatTypes_Prepared_Named(t *testing.T) {
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
	table := createObjectName("numeric_float_types_ps_test")
	cols := map[string]string{
		"id":       "NUMBER PRIMARY KEY",
		"number_i": "NUMBER(12, 0)",
		"number_s": "NUMBER(10, 7)",
		"float1":   "FLOAT",
		"bin1":     "BINARY_FLOAT",
		"bin2":     "BINARY_DOUBLE",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	})

	// Input rows mirroring the literal INSERT ALL in the non-prepared test.
	type inputRow struct {
		id int
		ni int64
		ns float64
		f1 float64
		bf float64
		bd float64
	}
	inputs := []inputRow{
		{1, int64(12345), 123.45, 123.456, 123.456, 123.456},
		{2, int64(-987654), -987.6543, -987.654, -987.654, -987.654},
		{3, int64(0), 0.0, 0.0, 0.0, 0.0},
		{4, int64(0), 1e-30, 1e-30, 1e-30, 1e-30},
		{6, int64(1), 0.0000001, 0.0000001, 0.0000001, 0.0000001},
		{7, int64(123), 123.89, 123.89, 123.89, 123.89},
	}

	// Prepared INSERT (named binds)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	ins := "INSERT INTO " + table + " (id, number_i, number_s, float1, bin1, bin2) VALUES (:id, :ni, :ns, :f1, :bf, :bd)"
	insStmt, err := db.PrepareContext(ctx, ins)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	t.Cleanup(func() { _ = insStmt.Close() })
	for i, r := range inputs {
		result, err := insStmt.ExecContext(ctx,
			sql.Named("id", r.id),
			sql.Named("ni", r.ni),
			sql.Named("ns", r.ns),
			sql.Named("f1", r.f1),
			sql.Named("bf", r.bf),
			sql.Named("bd", r.bd),
		)
		if err != nil {
			t.Fatalf("insert %d failed: %v", i+1, err)
		}
		_, err = result.RowsAffected()
		if err != nil {
			t.Fatalf("get result details for round %d failed: %v", i+1, err)
		}

	}

	// Helper to normalize FLOAT column which may arrive as int64 or string from the driver
	toFloat64 := func(v any) (float64, error) {
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		case string:
			return strconv.ParseFloat(x, 64)
		default:
			return 0, fmt.Errorf("unsupported FLOAT scan type %T", v)
		}
	}
	// Helper to compute float32-rounding expectation for BINARY_FLOAT values
	f32 := func(v float64) float64 { return float64(float32(v)) }
	equalWithin := func(a, b, tol float64) bool {
		if math.IsNaN(a) && math.IsNaN(b) {
			return true
		}
		return math.Abs(a-b) <= tol
	}
	const (
		tolBF = 1e-6
		tolBD = 1e-12
		tolF1 = tolBD
	)

	type rowExp struct {
		id      int
		ni      int64
		ns      string
		f1      float64
		bf      float64
		bd      float64
		bfIsNaN bool
		bdIsNaN bool
	}
	// Expected results identical to non-prepared test.
	exp := []rowExp{
		{1, 12345, "123.4500000", 123.456, f32(123.456), 123.456, false, false},
		{2, -987654, "-987.6543000", -987.654, f32(-987.654), -987.654, false, false},
		{3, 0, "0.0000000", 0.0, 0.0, 0.0, false, false},
		{4, 0, "0.0000000", 1e-30, f32(1e-30), 1e-30, false, false},
		{6, 1, "0.0000001", 0.0000001, f32(0.0000001), 0.0000001, false, false},
		{7, 123, "123.8900000", 123.89, f32(123.89), 123.89, false, false},
	}

	// Prepared SELECT (no bind)
	//noinspection SqlDialectInspection,SqlNoDataSourceInspection,SqlResolve
	sel := "SELECT id, number_i, number_s, float1, bin1, bin2 FROM " + table + " ORDER BY id"
	selStmt, err := db.PrepareContext(ctx, sel)
	if err != nil {
		t.Fatalf("prepare select failed: %v", err)
	}
	t.Cleanup(func() { _ = selStmt.Close() })

	rs, err := selStmt.QueryContext(ctx)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	idx := 0
	for rs.Next() {
		var (
			id int
			ni int64
			ns string
			f1 any
			bf float64
			bd float64
		)
		if err := rs.Scan(&id, &ni, &ns, &f1, &bf, &bd); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		f1f, err := toFloat64(f1)
		if err != nil {
			t.Fatalf("FLOAT scan type error: %v", err)
		}
		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		w := exp[idx]
		if id != w.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, w.id)
		}
		if ni != w.ni {
			t.Fatalf("NUMBER(12,0) mismatch at row %d: got %d want %d", idx, ni, w.ni)
		}
		if ns != w.ns {
			t.Fatalf("NUMBER(10,7) mismatch at row %d: got %q want %q", idx, ns, w.ns)
		}
		if !equalWithin(f1f, w.f1, tolF1) {
			t.Fatalf("FLOAT mismatch at row %d: got %.12g want %.12g (tol %.1e)", idx, f1f, w.f1, tolF1)
		}
		if w.bfIsNaN {
			if !math.IsNaN(bf) {
				t.Fatalf("BINARY_FLOAT mismatch at row %d: expected NaN, got %v", idx, bf)
			}
		} else if !equalWithin(bf, w.bf, tolBF) {
			t.Fatalf("BINARY_FLOAT mismatch at row %d: got %.9g want %.9g (tol %.1e)", idx, bf, w.bf, tolBF)
		}
		if w.bdIsNaN {
			if !math.IsNaN(bd) {
				t.Fatalf("BINARY_DOUBLE mismatch at row %d: expected NaN, got %v", idx, bd)
			}
		} else if !equalWithin(bd, w.bd, tolBD) {
			t.Fatalf("BINARY_DOUBLE mismatch at row %d: got %.15g want %.15g (tol %.1e)", idx, bd, w.bd, tolBD)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_Number_NoPrecisionScale
// Purpose: Validate behavior for a generic NUMBER column (no precision/scale declared).
// The driver may return int64 for integral values, or string for decimal values; we
// normalize to float64 for comparison with an appropriate tolerance.
func TestDriver_Select_Number_NoPrecisionScale(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "false"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("number_no_ps_test")
	cols := map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"num": "NUMBER",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	insertAll := "INSERT ALL\n" +
		"INTO " + table + " (id, num) VALUES (1, 42)\n" +
		"INTO " + table + " (id, num) VALUES (2, -987654321)\n" +
		"INTO " + table + " (id, num) VALUES (3, 0)\n" +
		"INTO " + table + " (id, num) VALUES (4, 123.456)\n" +
		"INTO " + table + " (id, num) VALUES (5, NULL)\n" +
		"INTO " + table + " (id, num) VALUES (6, 0.0000001)\n" +
		"SELECT 1 FROM DUAL"
	if _, err := db.ExecContext(ctx, insertAll); err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}

	q := "SELECT id, num FROM " + table + " ORDER BY id"
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	toFloat64 := func(v any) (float64, error) {
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		case string:
			return strconv.ParseFloat(x, 64)
		default:
			return 0, fmt.Errorf("unsupported NUMBER scan type %T", v)
		}
	}

	equalWithin := func(a, b, tol float64) bool { return math.Abs(a-b) <= tol }
	const tol = 1e-12

	type rowExp2 struct {
		id   int
		want float64
	}

	exp := []rowExp2{
		{1, 42},
		{2, -987654321},
		{3, 0},
		{4, 123.456},
		// Driver maps NULL NUMBER to 0 or "0" when scanning nullable numeric (see rows_result.go)
		{5, 0.0},
		{6, 0.0000001},
	}

	idx := 0
	for rs.Next() {
		var (
			id  int
			num any
		)
		if err := rs.Scan(&id, &num); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		v, err := toFloat64(num)
		if err != nil {
			t.Fatalf("NUMBER scan type error: %v", err)
		}
		t.Logf("Row %d: id=%d NUMBER=%v (type %T) => %.12g", idx, id, num, num, v)

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		w := exp[idx]
		if id != w.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, w.id)
		}
		if !equalWithin(v, w.want, tol) {
			t.Fatalf("NUMBER mismatch at row %d: got %.12g want %.12g (tol %.1e)", idx, v, w.want, tol)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_Number_MaxPrecisionScale
// Purpose: Validate driver behavior for NUMBER(38, 127), the maximum allowed precision
// and scale as per Oracle documentation. Ensures that values with extremely high scale
// are round-tripped correctly and exposed via Scan as strings preserving all digits.
func TestDriver_Select_Number_MaxPrecisionScale(t *testing.T) {
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
	table := createObjectName("number_max_ps_test")
	cols := map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"num": "NUMBER(38, 127)",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	const (
		significantDigits = "99999999999999999999999999999999999999"
	)
	// NUMBER(38, 127) allows 38 significant digits but pushes the decimal point 127 places right.
	// Because scale (127) is greater than precision (38), the value must start with 127 fractional digits.
	// The first (127 - 38 = 89) digits must therefore be zeros, and the remaining 38 digits can be nines.
	// That yields the maximum magnitude literal of roughly 8.999...e-90 (~1e-89) with 38 trailing 9s.
	// Any digits beyond the 127th decimal place are rounded away by Oracle.
	zeros := strings.Repeat("0", 127-len(significantDigits))
	frac := zeros + significantDigits
	maxScaleLiteral := "0." + frac
	maxScaleLiteralNeg := "-" + maxScaleLiteral

	insertAll := fmt.Sprintf(
		"INSERT ALL\n"+
			"INTO %s (id, num) VALUES (1, TO_NUMBER('%s'))\n"+
			"INTO %s (id, num) VALUES (2, TO_NUMBER('%s'))\n"+
			"SELECT 1 FROM DUAL",
		table, maxScaleLiteral,
		table, maxScaleLiteralNeg,
	)
	if _, err := db.ExecContext(ctx, insertAll); err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}

	q := fmt.Sprintf("SELECT id, num FROM %s ORDER BY id", table)
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type maxRow struct {
		id  int
		val string
	}

	exp := []maxRow{
		{1, maxScaleLiteral},
		{2, maxScaleLiteralNeg},
	}

	idx := 0
	for rs.Next() {
		var (
			id  int
			num string
		)
		if err := rs.Scan(&id, &num); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Each row log includes NUM length so we can ensure the driver returned all digits.
		t.Logf("Row %d: id=%d NUMBER=%s (len=%d)", idx, id, num, len(num))

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		w := exp[idx]
		if id != w.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, w.id)
		}
		if num != w.val {
			t.Fatalf("NUMBER(38,127) mismatch at row %d: got %q want %q", idx, num, w.val)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}

// TestDriver_Select_Number_And_BinaryDouble
// Purpose: Create a table with NUMBER, BINARY_DOUBLE and NUMBER columns, then
// round-trip representative reference values
func TestDriver_Select_Number_And_BinaryDouble(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	config := TestingConfig.Clone()
	config.ConnectionProperties.StrictNullValueHandling = "false"

	db, err := openTestDBWithConfig(config)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := createObjectName("num_bin_double_ref_test")
	cols := map[string]string{
		"id":      "NUMBER PRIMARY KEY",
		"num_txt": "NUMBER",
		"bin_dbl": "BINARY_DOUBLE",
		"num_fix": "NUMBER(38, 10)",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	type roundTripRow struct {
		id     int
		numTxt float64
		binDbl float64
		numFix string
	}
	rows := []roundTripRow{
		{1, 0, 0, "0.0000000000"},
		{2, 1, 1, "1.0000000000"},
		{3, 10, 10, "10.0000000000"},
		{4, 100, 100, "100.0000000000"},
		{5, 1000, 1000, "1000.0000000000"},
		{6, 0.1, 0.1, "0.1000000000"},
		{7, 0.01, 0.01, "0.0100000000"},
		{8, 0.001, 0.001, "0.0010000000"},
		{9, -2, -2, "-2.0000000000"},
		{10, -1.234, -1.234, "-1.2340000000"},
		{11, 3.14, 3.14, "3.1400000000"},
		{12, -3.14, -3.14, "-3.1400000000"},
		{13, 3.456789, 3.456789, "3.4567890000"},
		{14, 0.01, 0.01, "0.0100000000"},
		{15, -0.09, -0.09, "-0.0900000000"},
		{16, -0.89, -0.89, "-0.8900000000"},
		{17, 0.0000000001, 0.0000000001, "0.0000000001"},
		{18, 12340000, 12340000, "12340000.0000000000"},
		{19, 0.000001234, 0.000001234, "0.0000012340"},
		{20, 9876500, 9876500, "9876500.0000000000"},
		{21, 0.0000098765, 0.0000098765, "0.0000098765"},
		{22, 19.75, 19.75, "19.7500000000"},
		{23, -19.75, -19.75, "-19.7500000000"},
		{24, 197.5, 197.5, "197.5000000000"},
		{25, -197.5, -197.5, "-197.5000000000"},
		{26, 1.2345678901234569e+27, 1.2345678901234569e+27, "1234567890123456789012345678.0000000000"},
	}

	ins, err := db.PrepareContext(ctx,
		"INSERT INTO "+table+" (id, num_txt, bin_dbl, num_fix) VALUES (:1, :2, :3, TO_NUMBER(:4))")
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer ins.Close()

	for _, row := range rows {
		if _, err := ins.ExecContext(ctx, row.id, row.numTxt, row.binDbl, row.numFix); err != nil {
			t.Fatalf("insert row %d failed: %v", row.id, err)
		}
	}

	rs, err := db.QueryContext(ctx, "SELECT id, num_txt, bin_dbl, num_fix FROM "+table+" ORDER BY id")
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	const tolBinaryDouble = 1e-12
	idx := 0
	for rs.Next() {
		var (
			id     int
			numTxt float64
			binDbl float64
			numFix string
		)
		if err := rs.Scan(&id, &numTxt, &binDbl, &numFix); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if idx >= len(rows) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		want := rows[idx]
		t.Logf("Row %d: id=%d NUMBER=%.17g BINARY_DOUBLE=%.17g NUMBER(38,10)=%q", idx, id, numTxt, binDbl, numFix)

		if id != want.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, want.id)
		}
		if numTxt != want.numTxt {
			t.Fatalf("NUMBER mismatch at row %d: got %.17g want %.17g", idx, numTxt, want.numTxt)
		}
		if binDbl != want.binDbl {
			t.Fatalf("BINARY_DOUBLE mismatch at row %d: got %.17g want %.17g", idx, binDbl, want.binDbl)
		}
		if numFix != want.numFix {
			t.Fatalf("NUMBER(38,10) mismatch at row %d: got %q want %q", idx, numFix, want.numFix)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(rows) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(rows))
	}
}

func TestDriver_Select_Number_MaxPrecisionInteger(t *testing.T) {
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
	table := createObjectName("number_max_precision_test")
	cols := map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"num": "NUMBER(38, 0)",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	defer func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, err)
		}
	}()

	const maxPrecisionLiteral = "99999999999999999999999999999999999999"
	maxPrecisionLiteralNeg := "-" + maxPrecisionLiteral
	t.Logf("Max precision integer literal length=%d", len(maxPrecisionLiteral))

	insertAll := fmt.Sprintf(
		"INSERT ALL\n"+
			"INTO %s (id, num) VALUES (1, TO_NUMBER('%s'))\n"+
			"INTO %s (id, num) VALUES (2, TO_NUMBER('%s'))\n"+
			"SELECT 1 FROM DUAL",
		table, maxPrecisionLiteral, table, maxPrecisionLiteralNeg,
	)
	if _, err := db.ExecContext(ctx, insertAll); err != nil {
		t.Fatalf("batch insert failed: %v", err)
	}

	q := fmt.Sprintf("SELECT id, num FROM %s ORDER BY id", table)
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("select failed: %v", err)
	}
	defer rs.Close()

	type maxIntRow struct {
		id  int
		val string
	}

	exp := []maxIntRow{
		{1, maxPrecisionLiteral},
		{2, maxPrecisionLiteralNeg},
	}

	idx := 0
	for rs.Next() {
		var (
			id  int
			num string
		)
		if err := rs.Scan(&id, &num); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		t.Logf("Row %d: id=%d NUMBER=%s (len=%d)", idx, id, num, len(num))

		if idx >= len(exp) {
			t.Fatalf("received more rows than expected: idx=%d", idx)
		}
		w := exp[idx]
		if id != w.id {
			t.Fatalf("id mismatch at row %d: got %d want %d", idx, id, w.id)
		}
		if num != w.val {
			t.Fatalf("NUMBER(38,0) mismatch at row %d: got %q want %q", idx, num, w.val)
		}
		idx++
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if idx != len(exp) {
		t.Fatalf("row count mismatch: got %d want %d", idx, len(exp))
	}
}
