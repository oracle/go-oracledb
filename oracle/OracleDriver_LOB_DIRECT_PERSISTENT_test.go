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
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/lob"
)

// TestDriver_LobDirectPersistentWriteTo verifies WriteTo copies the remaining
// content of an unread persistent locator after OpenPersistent has released
// the query Rows. The existing FOR UPDATE mutation test covers write
// authorization and persistence; this test covers the public streaming helper.
func TestDriver_LobDirectPersistentWriteTo(t *testing.T) {
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
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table),
		int64(1), BindBlob([]byte("blob write-to")), BindClob("clob write-to 🙂"), BindNClob("nclob write-to 🙂")); err != nil {
		t.Fatalf("insert persistent values: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, test := range []struct {
		name   string
		column string
		want   []byte
	}{
		{name: "BLOB", column: "bin", want: []byte("blob write-to")},
		{name: "CLOB", column: "txt", want: []byte("clob write-to 🙂")},
		{name: "NCLOB", column: "ntxt", want: []byte("nclob write-to 🙂")},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows, err := conn.QueryContext(ctx,
				fmt.Sprintf("SELECT %s FROM %s WHERE id = :1", test.column, table), int64(1))
			if err != nil {
				t.Fatalf("query persistent LOB: %v", err)
			}
			if !rows.Next() {
				_ = rows.Close()
				t.Fatalf("query returned no row: %v", rows.Err())
			}
			var source lob.LOB
			if err := rows.Scan(&source); err != nil {
				_ = rows.Close()
				t.Fatalf("scan persistent LOB: %v", err)
			}
			value, err := lob.OpenPersistent(ctx, conn, &source)
			if err != nil {
				_ = rows.Close()
				t.Fatalf("open persistent LOB: %v", err)
			}
			t.Cleanup(func() { _ = value.Free(ctx) })
			if err := rows.Close(); err != nil {
				t.Fatalf("close source Rows: %v", err)
			}
			var output bytes.Buffer
			written, err := value.WriteTo(&output)
			if err != nil {
				t.Fatalf("WriteTo: %v", err)
			}
			if written != int64(len(test.want)) || !bytes.Equal(output.Bytes(), test.want) {
				t.Fatalf("WriteTo = (%d, %q), want (%d, %q)", written, output.Bytes(), len(test.want), test.want)
			}
		})
	}
}
