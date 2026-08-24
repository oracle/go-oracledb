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
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

type clobRowData struct {
	id int64
	n  string
	c  string
}

type blobRowData struct {
	id int64
	n  string
	b  []byte
}

// TestDriver_Prepared_Insert_Clob_Small ensures the driver can bind and transmit
// CLOB payloads up to 32KB via prepared statements. It exercises INSERT
// execution and verifies the RowsAffected metadata returned by the driver.
func TestDriver_Prepared_Insert_Clob_Small(t *testing.T) {
	t.Parallel()
	rows := []clobRowData{
		{
			id: 1,
			n:  "aaa",
			c:  "hello prepared",
		},
		{
			id: 2,
			n:  "bbb",
			c:  strings.Repeat("a", 4000),
		},
		{
			id: 3,
			n:  "ccc",
			c:  strings.Repeat("b", 20000),
		},
		{
			id: 4,
			n:  "ddd",
			c:  strings.Repeat("c", 32768),
		},
	}

	runPreparedInsertClob(t, createObjectName("clob1_small"), rows)
}

// TestDriver_Prepared_Insert_Clob_Large covers CLOB payloads larger than 32KB to
// ensure the driver can stream large data through prepared statements.
func TestDriver_Prepared_Insert_Clob_Large(t *testing.T) {
	t.Parallel()
	rows := []clobRowData{
		{
			id: 5,
			n:  "eee",
			c:  strings.Repeat("o", 11534336), // 11MB
		},
		{
			id: 6,
			n:  "fff",
			c:  strings.Repeat("m", 33554432), // 32MB
		},
		{
			id: 7,
			n:  "ggg",
			c:  strings.Repeat("t", 41943040), // 40MB
		},
	}

	runPreparedInsertClob(t, createObjectName("clob1_large"), rows)
}

// TestDriver_Prepared_Insert_Blob_Small verifies BLOB marker binds, including a
// payload over 32KB positioned before another bind variable.
func TestDriver_Prepared_Insert_Blob_Small(t *testing.T) {
	t.Parallel()
	rows := []blobRowData{
		{
			id: 1,
			n:  "aaa",
			b:  []byte("hello prepared"),
		},
		{
			id: 2,
			n:  "bbb",
			b:  bytes.Repeat([]byte{0xAB}, 4000),
		},
		{
			id: 3,
			n:  "ccc",
			b:  bytes.Repeat([]byte{0xBC}, 20000),
		},
		{
			id: 4,
			n:  "ddd",
			b:  bytes.Repeat([]byte{0xCD}, 32768),
		},
	}

	runPreparedInsertBlob(t, createObjectName("blob1_small"), rows)
}

// TestDriver_Prepared_Insert_Blob_Large covers large BLOB marker payloads to
// verify temporary locator upload and statement binding above the RAW limit.
func TestDriver_Prepared_Insert_Blob_Large(t *testing.T) {
	t.Parallel()
	rows := []blobRowData{
		{
			id: 5,
			n:  "eee",
			b:  bytes.Repeat([]byte{0xDE}, 11534336), // 11MB
		},
		{
			id: 6,
			n:  "fff",
			b:  bytes.Repeat([]byte{0xEF}, 33554432), // 32MB
		},
		{
			id: 7,
			n:  "ggg",
			b:  bytes.Repeat([]byte{0xFA}, 41943040), // 40MB
		},
	}

	runPreparedInsertBlob(t, createObjectName("blob1_large"), rows)
}

func runPreparedInsertClob(t *testing.T, table string, rows []clobRowData) {
	t.Helper()

	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	prefetchSize := 33554432 // 32 MB
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(20)",
		"clob": "CLOB",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping prepared insert CLOB test (create failed): %v", err)
		return
	}
	t.Cleanup(func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	})

	insSQL := "INSERT INTO " + table + " (id, name, clob) " +
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

		if len(rr.c) > prefetchSize {
			t.Skip()
		}
		var gotClob string
		err = db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT clob FROM %s WHERE id = %d", table, rr.id),
		).Scan(&gotClob)
		if err != nil {
			t.Fatalf("select inserted row failed for id=%d: %v", rr.id, err)
		}
		if gotClob != rr.c {
			t.Fatalf("unexpected clob for id=%d: got length %d want %d", rr.id, len(gotClob), len(rr.c))
		}
	}
}

func runPreparedInsertBlob(t *testing.T, table string, rows []blobRowData) {
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
	cols := map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"name": "VARCHAR2(20)",
		"blob": "BLOB",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Skipf("Skipping prepared insert BLOB test (create failed): %v", err)
		return
	}
	t.Cleanup(func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	})

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin BLOB insert transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	insSQL := "INSERT INTO " + table + " (id, blob, name) " +
		"VALUES (:id, :b, :n)"
	insStmt, err := tx.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	for _, rr := range rows {
		result, err := insStmt.ExecContext(ctx,
			sql.Named("id", rr.id),
			sql.Named("b", Blob(rr.b)),
			sql.Named("n", rr.n),
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

		if len(rr.b) > 33554432 {
			continue
		}
		var gotBlob []byte
		err = tx.QueryRowContext(ctx,
			fmt.Sprintf("SELECT blob FROM %s WHERE id = %d", table, rr.id),
		).Scan(&gotBlob)
		if err != nil {
			t.Fatalf("select inserted row failed for id=%d: %v", rr.id, err)
		}
		if !bytes.Equal(gotBlob, rr.b) {
			t.Fatalf("unexpected blob for id=%d: got length %d want %d", rr.id, len(gotBlob), len(rr.b))
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit BLOB insert transaction: %v", err)
	}
}
