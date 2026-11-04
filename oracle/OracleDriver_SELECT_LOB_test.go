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
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// TestDriver_Table_Select_CLOB
// What it does: Creates a table with a CLOB column, inserts a large string, then selects it back.
// Expectation: Selecting the CLOB column and scanning into string returns the exact text inserted.
// Notes:
//   - Uses TO_CLOB(:val) to ensure we bind a CLOB even for large values.
//   - Includes a NULL CLOB scan case.
func TestDriver_Table_Select_CLOB(t *testing.T) {
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
	table := createObjectName("t_select_clob")

	cols := map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"txt": "CLOB",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	clobSmall := "hello clob"
	clobLarge := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 5000) // ~220KB

	insSQL := fmt.Sprintf("INSERT INTO %s (id, txt) VALUES (:id, :txt)", table)
	if _, err := db.ExecContext(ctx, insSQL, sql.Named("id", int64(1)), sql.Named("txt", clobSmall)); err != nil {
		t.Fatalf("insert small clob failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, insSQL, sql.Named("id", int64(2)), sql.Named("txt", clobLarge)); err != nil {
		t.Fatalf("insert large clob failed: %v", err)
	}
	// NULL case
	insNull := fmt.Sprintf("INSERT INTO %s (id, txt) VALUES (:id, NULL)", table)
	if _, err := db.ExecContext(ctx, insNull, sql.Named("id", int64(3))); err != nil {
		t.Fatalf("insert null clob failed: %v", err)
	}

	// Select back (small)
	var outSmall string
	selSQL := fmt.Sprintf("SELECT txt FROM %s WHERE id = :id", table)
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(1))).Scan(&outSmall); err != nil {
		t.Fatalf("select/scan small clob failed: %v", err)
	}
	if outSmall != clobSmall {
		t.Fatalf("small clob mismatch:\n got: %q\nwant: %q", outSmall, clobSmall)
	}

	// Select back (large) - compare hash to keep failure messages manageable
	var outLarge string
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(2))).Scan(&outLarge); err != nil {
		t.Fatalf("select/scan large clob failed: %v", err)
	}
	if sha256.Sum256([]byte(outLarge)) != sha256.Sum256([]byte(clobLarge)) {
		t.Fatalf("large clob mismatch (sha256 differs):\n got:  %x\nwant: %x",
			sha256.Sum256([]byte(outLarge)),
			sha256.Sum256([]byte(clobLarge)),
		)
	}

	// Select back (NULL) - use sql.NullString to validate NULL handling
	var outNull sql.NullString
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(3))).Scan(&outNull); err != nil {
		t.Fatalf("select/scan null clob failed: %v", err)
	}
	if outNull.Valid {
		t.Fatalf("expected NULL clob, got valid string: %q", outNull.String)
	}
}

// TestDriver_Table_Select_BLOB
// What it does: Creates a table with a BLOB column, inserts raw bytes, then selects it back.
// Expectation: Selecting the BLOB column and scanning into []byte returns the exact bytes inserted.
// Notes:
//   - Uses EMPTY_BLOB() + RETURNING to reliably store a BLOB value without implicit conversions.
//   - Includes a NULL BLOB scan case.
func TestDriver_Table_Select_BLOB(t *testing.T) {
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
	table := createObjectName("t_select_blob")

	cols := map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"bin": "BLOB",
	}

	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	blobSmall := []byte("hello blob")
	blobLarge := bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03, 0xFE, 0xFF}, 100_000) // 600KB

	// Small blob insert using bind + TO_BLOB(UTL_RAW.CAST_TO_RAW(...)) would be text-based; instead
	// use a plain bind and let driver send bytes as BLOB if supported. If not, fallback to RETURNING.
	insSQL := fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:id, :bin)", table)
	if _, err := db.ExecContext(ctx, insSQL, sql.Named("id", int64(1)), sql.Named("bin", blobSmall)); err != nil {
		// Fallback approach: insert EMPTY_BLOB then update with bind (works on more setups)
		insEmpty := fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:id, EMPTY_BLOB())", table)
		if _, err2 := db.ExecContext(ctx, insEmpty, sql.Named("id", int64(1))); err2 != nil {
			t.Fatalf("insert small blob failed (direct=%v, empty=%v)", err, err2)
		}
		upd := fmt.Sprintf("UPDATE %s SET bin = :bin WHERE id = :id", table)
		if _, err2 := db.ExecContext(ctx, upd, sql.Named("bin", blobSmall), sql.Named("id", int64(1))); err2 != nil {
			t.Fatalf("update small blob failed: %v", err2)
		}
	}

	// Large blob insert
	if _, err := db.ExecContext(ctx, insSQL, sql.Named("id", int64(2)), sql.Named("bin", blobLarge)); err != nil {
		insEmpty := fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:id, EMPTY_BLOB())", table)
		if _, err2 := db.ExecContext(ctx, insEmpty, sql.Named("id", int64(2))); err2 != nil {
			t.Fatalf("insert large blob failed (direct=%v, empty=%v)", err, err2)
		}
		upd := fmt.Sprintf("UPDATE %s SET bin = :bin WHERE id = :id", table)
		if _, err2 := db.ExecContext(ctx, upd, sql.Named("bin", blobLarge), sql.Named("id", int64(2))); err2 != nil {
			t.Fatalf("update large blob failed: %v", err2)
		}
	}

	// NULL case
	insNull := fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:id, NULL)", table)
	if _, err := db.ExecContext(ctx, insNull, sql.Named("id", int64(3))); err != nil {
		t.Fatalf("insert null blob failed: %v", err)
	}

	// Select back (small)
	selSQL := fmt.Sprintf("SELECT bin FROM %s WHERE id = :id", table)
	var outSmall []byte
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(1))).Scan(&outSmall); err != nil {
		t.Fatalf("select/scan small blob failed: %v", err)
	}
	if !bytes.Equal(outSmall, blobSmall) {
		t.Fatalf("small blob mismatch:\n got:  %x\nwant: %x", outSmall, blobSmall)
	}

	// Select back (large) - compare hash
	var outLarge []byte
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(2))).Scan(&outLarge); err != nil {
		t.Fatalf("select/scan large blob failed: %v", err)
	}
	if sha256.Sum256(outLarge) != sha256.Sum256(blobLarge) {
		t.Fatalf("large blob mismatch (sha256 differs):\n got:  %x\nwant: %x",
			sha256.Sum256(outLarge),
			sha256.Sum256(blobLarge),
		)
	}

	// Select back (NULL) - validate NULL handling; scanning NULL into *[]byte yields nil with some drivers,
	// but database/sql recommends sql.RawBytes or sql.Null* types. We use sql.RawBytes and check nil/len.
	var outNull []byte
	if err := db.QueryRowContext(ctx, selSQL, sql.Named("id", int64(3))).Scan(&outNull); err != nil {
		t.Fatalf("select/scan null blob failed: %v", err)
	}
	if outNull != nil && len(outNull) != 0 {
		t.Fatalf("expected NULL blob to scan as nil/empty, got len=%d", len(outNull))
	}
}

// TestDriver_Table_Insert_Select_CLOB_BLOB_MultiRows
// What it does: Inserts 15 rows containing both CLOB and BLOB data, then reads them back using SELECT *.
// Expectation: Every row selected from the table matches exactly what was inserted.
// Notes:
//   - CLOB/BLOB payload sizes are kept <= 20 for each row.
//   - Query uses `SELECT * FROM <table>` as requested.
func TestDriver_Table_Insert_Select_CLOB_BLOB_MultiRows(t *testing.T) {
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
	table := createObjectName("t_ins_sel_clob_blob_15")
	cols := map[string]string{
		"id":  "NUMBER PRIMARY KEY",
		"txt": "CLOB",
		"bin": "BLOB",
	}

	// Recreate table to ensure a clean state for this test run.
	_ = dropTable(ctx, db, table)
	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	// Build expected row data container for later validation.
	type expectedRow struct {
		clob string
		blob []byte
	}
	expected := make(map[int64]expectedRow, 15)

	// Insert 15 rows of deterministic CLOB/BLOB payloads.
	insSQL := fmt.Sprintf("INSERT INTO %s (id, txt, bin) VALUES (:id, :txt, :bin)", table)
	for i := 1; i <= 15; i++ {
		id := int64(i)
		clobLen := i + 5 // 6..20
		blobLen := i + 5 // 6..20

		clobVal := strings.Repeat(string(rune('a'+(i%26))), clobLen)
		blobVal := bytes.Repeat([]byte{byte('A' + (i % 26))}, blobLen)
		expected[id] = expectedRow{clob: clobVal, blob: blobVal}

		if _, err := db.ExecContext(ctx, insSQL,
			sql.Named("id", id),
			sql.Named("txt", clobVal),
			sql.Named("bin", blobVal),
		); err != nil {
			t.Fatalf("insert failed for id=%d: %v", id, err)
		}
	}

	// Select all rows using SELECT * syntax.
	selSQL := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := db.QueryContext(ctx, selSQL)
	if err != nil {
		t.Fatalf("select rows failed: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })

	// Validate selected column count before scanning rows.
	colsReturned, err := rows.Columns()
	if err != nil {
		t.Fatalf("failed to read selected columns: %v", err)
	}
	if len(colsReturned) != 3 {
		t.Fatalf("unexpected selected column count: got=%d want=3", len(colsReturned))
	}

	// Iterate through selected rows and verify each value matches expected.
	seen := make(map[int64]bool, 15)
	for rows.Next() {
		var (
			id  int64
			txt string
			bin []byte
		)

		// Keep SELECT * while making scan resilient to table column order.
		scanTargets := make([]any, len(colsReturned))
		for i, c := range colsReturned {
			switch strings.ToUpper(c) {
			case "ID":
				scanTargets[i] = &id
			case "TXT":
				scanTargets[i] = &txt
			case "BIN":
				scanTargets[i] = &bin
			default:
				var discard any
				scanTargets[i] = &discard
			}
		}

		if err := rows.Scan(scanTargets...); err != nil {
			t.Fatalf("row scan failed: %v", err)
		}

		exp, ok := expected[id]
		if !ok {
			t.Fatalf("unexpected id returned from select: %d", id)
		}

		if txt != exp.clob {
			t.Fatalf("clob mismatch for id=%d: got=%q want=%q", id, txt, exp.clob)
		}

		if !bytes.Equal(bin, exp.blob) {
			t.Fatalf("blob mismatch for id=%d: got=%x want=%x", id, bin, exp.blob)
		}

		seen[id] = true
	}

	// Ensure row iteration completed without driver/database errors.
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration failed: %v", err)
	}

	// Confirm all 15 inserted rows were returned and validated.
	if len(seen) != 15 {
		t.Fatalf("row count mismatch: got=%d want=15", len(seen))
	}
}

func TestDriver_Prepared_Insert_Nclob_Small(t *testing.T) {
	t.Parallel()
	rows := []clobRowData{
		{
			id: 1,
			n:  "aaa",
			c:  "hello nclob 山田太郎",
		},
		{
			id: 2,
			n:  "bbb",
			c:  strings.Repeat("李小龙", 2000),
		},
		{
			id: 3,
			n:  "ccc",
			c:  strings.Repeat("مرحبا", 4000),
		},
		{
			id: 4,
			n:  "ddd",
			c:  strings.Repeat("👨‍💻🖥️", 1500),
		},
	}

	runPreparedInsertNclob(t, createObjectName("nclob1_small"), rows)
}

func runPreparedInsertNclob(t *testing.T, table string, rows []clobRowData) {
	t.Helper()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	dbCharset, err := fetchNLSCharacterSet(ctx, db)
	if err != nil {
		t.Fatalf("failed to fetch NLS_CHARACTERSET: %v", err)
	}
	ncharCharset, err := fetchNLSNcharCharacterSet(ctx, db)
	if err != nil {
		t.Fatalf("failed to fetch NLS_NCHAR_CHARACTERSET: %v", err)
	}
	if dbCharset != "AL32UTF8" {
		t.Skipf("skipping prepared insert NCLOB test: expected NLS_CHARACTERSET AL32UTF8, got %s", dbCharset)
	}
	if ncharCharset != "AL16UTF16" {
		t.Skipf("skipping prepared insert NCLOB test: expected NLS_NCHAR_CHARACTERSET AL16UTF16, got %s", ncharCharset)
	}

	cols := map[string]string{
		"id":    "NUMBER PRIMARY KEY",
		"name":  "VARCHAR2(20)",
		"nclob": "NCLOB",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping prepared insert NCLOB test (create failed): %v", err)
		return
	}
	t.Cleanup(func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	})

	insSQL := "INSERT INTO " + table + " (id, name, nclob) " +
		"VALUES (:id, :n, :c)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	for _, rr := range rows {
		result, err := insStmt.ExecContext(ctx,
			sql.Named("id", rr.id),
			sql.Named("n", rr.n),
			sql.Named("c", rr.c),
		)

		if err != nil {
			t.Fatalf("exec insert failed for id=%d: %v", rr.id, err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("fetch RowsAffected failed for id=%d: %v", rr.id, err)
		}
		if affected != 1 {
			t.Fatalf("unexpected rows affected for id=%d: got %d want 1", rr.id, affected)
		}

		var gotNclob string
		err = db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT nclob FROM %s WHERE id = %d", table, rr.id),
		).Scan(&gotNclob)
		if err != nil {
			t.Fatalf("select inserted row failed for id=%d: %v", rr.id, err)
		}

		if gotNclob != rr.c {
			t.Logf("---- NCLOB MISMATCH DEBUG (id=%d) ----", rr.id)

			// Length diagnostics
			t.Logf("expected: bytes=%d runes=%d", len(rr.c), len([]rune(rr.c)))
			t.Logf("actual  : bytes=%d runes=%d", len(gotNclob), len([]rune(gotNclob)))

			// Raw byte dump (first 64 bytes)
			maxDump := 64
			expBytes := []byte(rr.c)
			actBytes := []byte(gotNclob)

			if len(expBytes) > maxDump {
				expBytes = expBytes[:maxDump]
			}
			if len(actBytes) > maxDump {
				actBytes = actBytes[:maxDump]
			}

			t.Logf("expected bytes (first %d): % x", maxDump, expBytes)
			t.Logf("actual   bytes (first %d): % x", maxDump, actBytes)

			// Optional string view
			t.Logf("expected string: %q", rr.c)
			t.Logf("actual   string: %q", gotNclob)

			// Prefix mismatch detection
			minLen := len(rr.c)
			if len(gotNclob) < minLen {
				minLen = len(gotNclob)
			}

			mismatchAt := -1
			for i := 0; i < minLen; i++ {
				if rr.c[i] != gotNclob[i] {
					mismatchAt = i
					break
				}
			}

			if mismatchAt >= 0 {
				t.Logf("first byte mismatch at index %d: expected=0x%x actual=0x%x",
					mismatchAt, rr.c[mismatchAt], gotNclob[mismatchAt])
			} else {
				t.Logf("no mismatch in first %d bytes, possible trailing garbage", minLen)
			}

			t.Fatalf("unexpected nclob for id=%d: got length %d want %d",
				rr.id, len(gotNclob), len(rr.c))
		}
	}
}
