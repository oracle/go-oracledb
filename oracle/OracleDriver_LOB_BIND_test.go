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
	"fmt"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/lob"
)

// TestDriver_LobBindLargeAndEmptyValues verifies the public marker types use
// streamed temporary LOB binds for both INSERT and UPDATE. Ordinary scans
// verify the stored database values; locator-backed query behavior is covered
// by the query-lifecycle group.
func TestDriver_LobBindLargeAndEmptyValues(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	table := createLOBObjectName(t)
	if err := createTable(ctx, db, table, map[string]string{
		"id": "NUMBER PRIMARY KEY", "bin": "BLOB", "txt": "CLOB", "ntxt": "NCLOB",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	blobLarge := bytes.Repeat([]byte{0x00, 0xFF, 0x41}, 128*1024/3+1)
	clobLarge := strings.Repeat("CLOB Aé中🙂 ", 32*1024)
	nclobLarge := strings.Repeat("NCLOB Aé中🙂 ", 32*1024)
	insert := fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), BindBlob(blobLarge), BindClob(clobLarge), BindNClob(nclobLarge)); err != nil {
		t.Fatalf("insert large marker values: %v", err)
	}

	var gotBlob lob.Bytes
	var gotClob, gotNClob lob.Text
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table), int64(1)).Scan(&gotBlob, &gotClob, &gotNClob); err != nil {
		t.Fatalf("scan inserted values: %v", err)
	}
	if !bytes.Equal(gotBlob, blobLarge) || string(gotClob) != clobLarge || string(gotNClob) != nclobLarge {
		t.Fatal("large marker bind round trip mismatch")
	}

	update := fmt.Sprintf("UPDATE %s SET bin = :1, txt = :2, ntxt = :3 WHERE id = :4", table)
	if _, err := db.ExecContext(ctx, update, BindBlob([]byte{}), BindClob(""), BindNClob(""), int64(1)); err != nil {
		t.Fatalf("update empty marker values: %v", err)
	}

	// A materialized empty BLOB may be represented by a nil or zero-length Go
	// slice. Ask Oracle directly to distinguish empty LOBs from SQL NULLs.
	var binNull, txtNull, ntxtNull int
	var binLength, txtLength, ntxtLength int64
	emptyState := fmt.Sprintf(`SELECT
		CASE WHEN bin IS NULL THEN 1 ELSE 0 END,
		DBMS_LOB.GETLENGTH(bin),
		CASE WHEN txt IS NULL THEN 1 ELSE 0 END,
		DBMS_LOB.GETLENGTH(txt),
		CASE WHEN ntxt IS NULL THEN 1 ELSE 0 END,
		DBMS_LOB.GETLENGTH(ntxt)
	FROM %s WHERE id = :1`, table)
	if err := db.QueryRowContext(ctx, emptyState, int64(1)).Scan(
		&binNull, &binLength, &txtNull, &txtLength, &ntxtNull, &ntxtLength,
	); err != nil {
		t.Fatalf("check empty LOB database state: %v", err)
	}
	if binNull != 0 || txtNull != 0 || ntxtNull != 0 || binLength != 0 || txtLength != 0 || ntxtLength != 0 {
		t.Fatalf("empty LOB database state = nulls(%d, %d, %d) lengths(%d, %d, %d), want all zero", binNull, txtNull, ntxtNull, binLength, txtLength, ntxtLength)
	}
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table), int64(1)).Scan(&gotBlob, &gotClob, &gotNClob); err != nil {
		t.Fatalf("scan updated values: %v", err)
	}
	if len(gotBlob) != 0 || gotClob != "" || gotNClob != "" {
		t.Fatalf("empty marker bind values = (%x, %q, %q), want empty LOBs", gotBlob, gotClob, gotNClob)
	}
}
