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

	"github.com/oracle/go-oracledb/v26/oracle/lob"
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

// binaryFixture returns a non-uniform payload that includes both zero and
// high-bit bytes. Exact byte comparisons catch same-length corruption that a
// repeated-byte fixture could conceal.
func binaryFixture(size int) []byte {
	payload := make([]byte, size)
	for index := range payload {
		payload[index] = byte(index*131 + 17)
	}
	if size > 0 {
		payload[0] = 0x00
	}
	if size > 1 {
		payload[1] = 0xFF
	}
	return payload
}

// TestDriver_LobPreparedInsertClobSmall ensures the driver can bind and transmit
// CLOB payloads up to 32KB via prepared statements. It exercises INSERT
// execution and verifies the RowsAffected metadata returned by the driver.
func TestDriver_LobPreparedInsertClobSmall(t *testing.T) {
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

	runPreparedInsertBindClob(t, createLOBObjectName(t), rows)
}

// TestDriver_LobPreparedInsertClobLarge covers CLOB payloads larger than 32KB to
// ensure the driver can stream large data through prepared statements.
func TestDriver_LobPreparedInsertClobLarge(t *testing.T) {
	t.Parallel()
	rows := []clobRowData{
		{id: 5, n: "eee", c: strings.Repeat("o", 64*1024)},
		{id: 6, n: "fff", c: strings.Repeat("m", 256*1024)},
		{id: 7, n: "ggg", c: strings.Repeat("t", 1024*1024)},
	}

	runPreparedInsertBindClob(t, createLOBObjectName(t), rows)
}

// TestDriver_LobPreparedInsertBlobSmall mirrors the CLOB small insertion test
// but uses BLOB columns bound through the public BindBlob marker.
func TestDriver_LobPreparedInsertBlobSmall(t *testing.T) {
	t.Parallel()
	rows := []blobRowData{
		{id: 1, n: "aaa", b: []byte{0x00, 0xFF, 0x01, 0x7F, 0x80, 0x42}},
		{id: 2, n: "bbb", b: binaryFixture(4000)},
		{id: 3, n: "ccc", b: binaryFixture(20000)},
		{id: 4, n: "ddd", b: binaryFixture(32768)},
	}

	runPreparedInsertBindBlob(t, createLOBObjectName(t), rows)
}

// TestDriver_LobPreparedInsertBlobLarge covers large BLOB payloads to verify
// inserts via prepared statements continues to work for >32KB data.
func TestDriver_LobPreparedInsertBlobLarge(t *testing.T) {
	t.Parallel()
	rows := []blobRowData{
		{id: 5, n: "eee", b: binaryFixture(64 * 1024)},
		{id: 6, n: "fff", b: binaryFixture(256 * 1024)},
		{id: 7, n: "ggg", b: binaryFixture(1024 * 1024)},
	}

	runPreparedInsertBindBlob(t, createLOBObjectName(t), rows)
}

func runPreparedInsertBindClob(t *testing.T, table string, rows []clobRowData) {
	t.Helper()
	t.Logf("CLOB prepared bind: table=%s rows=%d", table, len(rows))

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
		"clob": "CLOB",
	}

	if err := createTable(ctx, db, table, cols); err != nil {
		t.Fatalf("create prepared insert CLOB table: %v", err)
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
		t.Logf("CLOB prepared bind: inserting id=%d bytes=%d", rr.id, len(rr.c))
		result, err := insStmt.ExecContext(ctx,
			sql.Named("id", rr.id),
			sql.Named("n", rr.n),
			sql.Named("c", BindClob(rr.c)),
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
		t.Logf("CLOB prepared bind: inserted id=%d", rr.id)

		t.Logf("CLOB prepared bind: verifying id=%d", rr.id)
		var gotClob lob.Text
		err = db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT clob FROM %s WHERE id = %d", table, rr.id),
		).Scan(&gotClob)
		if err != nil {
			t.Fatalf("select inserted row failed for id=%d: %v", rr.id, err)
		}
		if string(gotClob) != rr.c {
			t.Fatalf("unexpected clob for id=%d: got length %d want %d", rr.id, len(gotClob), len(rr.c))
		}
	}
}

func runPreparedInsertBindBlob(t *testing.T, table string, rows []blobRowData) {
	t.Helper()
	t.Logf("BLOB prepared bind: table=%s rows=%d", table, len(rows))

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
		t.Fatalf("create prepared insert BLOB table: %v", err)
	}
	t.Cleanup(func() {
		if e := dropTable(ctx, db, table); e != nil {
			t.Errorf("cleanup drop table %s failed: %v", table, e)
		}
	})

	insSQL := "INSERT INTO " + table + " (id, name, blob) " +
		"VALUES (:id, :n, :b)"
	insStmt, err := db.PrepareContext(ctx, insSQL)
	if err != nil {
		t.Fatalf("prepare insert failed: %v", err)
	}
	defer func() { _ = insStmt.Close() }()

	for _, rr := range rows {
		t.Logf("BLOB prepared bind: inserting id=%d bytes=%d", rr.id, len(rr.b))
		result, err := insStmt.ExecContext(ctx,
			sql.Named("id", rr.id),
			sql.Named("n", rr.n),
			sql.Named("b", BindBlob(rr.b)),
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
		t.Logf("BLOB prepared bind: inserted id=%d", rr.id)

		t.Logf("BLOB prepared bind: verifying id=%d", rr.id)
		var gotBlob lob.Bytes
		err = db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT blob FROM %s WHERE id = %d", table, rr.id),
		).Scan(&gotBlob)
		if err != nil {
			t.Fatalf("select inserted row failed for id=%d: %v", rr.id, err)
		}
		if !bytes.Equal(gotBlob, rr.b) {
			t.Fatalf("unexpected BLOB content for id=%d: got length %d want %d", rr.id, len(gotBlob), len(rr.b))
		}
	}
}
