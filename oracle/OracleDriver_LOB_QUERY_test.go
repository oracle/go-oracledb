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
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/lob"
)

// TestDriver_LocatorBackedLOBQueryLifecycle verifies the public locator-backed
// LOB lifecycle:
// QueryContext keeps a scanned LOB usable while Rows is open, whereas
// QueryRowContext closes its Rows before Scan returns and therefore cannot
// return a usable locator-backed LOB.
func TestDriver_LocatorBackedLOBQueryLifecycle(t *testing.T) {
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
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table),
		int64(1), BindBlob([]byte("blob")), BindClob("clob 🙂"), BindNClob("nclob 🙂")); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	for _, test := range []struct {
		name   string
		column string
		kind   lob.Kind
	}{
		{name: "BLOB", column: "bin", kind: lob.BLOB},
		{name: "CLOB", column: "txt", kind: lob.CLOB},
		{name: "NCLOB", column: "ntxt", kind: lob.NCLOB},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := fmt.Sprintf("SELECT %s FROM %s WHERE id = :1", test.column, table)

			rows, err := db.QueryContext(ctx, query, int64(1))
			if err != nil {
				t.Fatalf("QueryContext: %v", err)
			}
			if !rows.Next() {
				_ = rows.Close()
				t.Fatalf("QueryContext returned no row: %v", rows.Err())
			}
			var value lob.LOB
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan QueryContext LOB: %v", err)
			}
			if !value.Valid() || value.Kind() != test.kind {
				_ = rows.Close()
				t.Fatalf("QueryContext LOB state = (valid=%t, kind=%v), want (true, %v)", value.Valid(), value.Kind(), test.kind)
			}
			if _, err := value.Size(); err != nil {
				_ = rows.Close()
				t.Fatalf("Size before Rows.Close: %v", err)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("close QueryContext Rows: %v", err)
			}
			requireLOBErrorCode(t, oracleErrors.LobValueInvalidated, func() error {
				_, err := value.Size()
				return err
			})

			var rowValue lob.LOB
			if err := db.QueryRowContext(ctx, query, int64(1)).Scan(&rowValue); err != nil {
				t.Fatalf("QueryRowContext Scan: %v", err)
			}
			if !rowValue.Valid() || rowValue.Kind() != test.kind {
				t.Fatalf("QueryRowContext LOB state = (valid=%t, kind=%v), want (true, %v)", rowValue.Valid(), rowValue.Kind(), test.kind)
			}
			requireLOBErrorCode(t, oracleErrors.LobValueInvalidated, func() error {
				_, err := rowValue.Size()
				return err
			})
		})
	}
}

// TestDriver_LobQueryLocatorSurvivesRowsNext verifies that a locator from an
// earlier row stays usable after Rows advances, then is invalidated by Rows.Close.
func TestDriver_LobQueryLocatorSurvivesRowsNext(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openLobPrefetchTestDB(TestingConfig, functionalLobPrefetchSize)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	table := createLOBObjectName(t)
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER PRIMARY KEY", "bin": "BLOB"}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(context.Background(), db, table) })
	firstPayload := binaryFixture(functionalLobPrefetchSize + 1024)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:1, :2)", table), int64(1), BindBlob(firstPayload)); err != nil {
		t.Fatalf("insert first row: %v", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:1, :2)", table), int64(2), BindBlob([]byte("second"))); err != nil {
		t.Fatalf("insert second row: %v", err)
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT bin FROM %s ORDER BY id", table))
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("first row missing: %v", rows.Err())
	}
	var value lob.LOB
	if err := rows.Scan(&value); err != nil {
		t.Fatalf("scan first LOB: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("Rows.Next with unread locator returned no second row: %v", rows.Err())
	}
	var second lob.LOB
	if err := rows.Scan(&second); err != nil {
		t.Fatalf("scan second LOB: %v", err)
	}
	hash := sha256.New()
	if _, err := io.CopyBuffer(hash, &value, make([]byte, internallob.DefaultBlobLobChunkBytes)); err != nil {
		t.Fatalf("read first LOB after Rows.Next: %v", err)
	}
	wantDigest := sha256.Sum256(firstPayload)
	if gotDigest := hash.Sum(nil); !bytes.Equal(gotDigest, wantDigest[:]) {
		t.Fatalf("first LOB digest after Rows.Next = %x, want %x", gotDigest, wantDigest)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close Rows: %v", err)
	}
	requireLOBErrorCode(t, oracleErrors.LobValueInvalidated, func() error {
		_, err := value.Size()
		return err
	})
}

// requireLOBErrorCode checks the driver's documented error-code contract
// without coupling a functional test to localized error text.
func requireLOBErrorCode(t *testing.T, want oracleErrors.ErrorCode, operation func() error) {
	t.Helper()
	err := operation()
	if err == nil {
		t.Fatalf("operation succeeded, want %s", want)
	}
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(want) {
		t.Fatalf("operation error = %v, want error code %s", err, want)
	}
}
