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
	"errors"
	"fmt"
	"io"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	datatypeLOB "github.com/oracle/go-oracledb/v26/oracle/datatype/lob"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_TypedLOBQueryLifecycle verifies the go-oracledb-specific direct
// database/sql scanner extension for BLOB, CLOB, and NCLOB values. Both
// Rows.Scan and QueryRow.Scan receive locator-backed sources and transfer each
// locator to its typed value rather than using the lob.Bytes/Text materializers.
// Scanning detaches each locator before query rows close, so the value remains
// usable afterward.
func TestDriver_TypedLOBQueryLifecycle(t *testing.T) {
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
		name    string
		column  string
		kind    datatypeLOB.Kind
		payload string
	}{
		{name: "BLOB", column: "bin", kind: datatypeLOB.BLOB, payload: "blob"},
		{name: "CLOB", column: "txt", kind: datatypeLOB.CLOB, payload: "clob 🙂"},
		{name: "NCLOB", column: "ntxt", kind: datatypeLOB.NCLOB, payload: "nclob 🙂"},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("get dedicated connection: %v", err)
			}
			defer conn.Close()

			query := fmt.Sprintf("SELECT %s FROM %s WHERE id = :1", test.column, table)

			value, err := newTypedLOBTarget(test.kind)
			if err != nil {
				t.Fatalf("create typed scanner: %v", err)
			}
			defer value.FreeContext(ctx)
			rows, err := conn.QueryContext(ctx, query, int64(1))
			if err != nil {
				t.Fatalf("QueryContext: %v", err)
			}
			if !rows.Next() {
				_ = rows.Close()
				t.Fatalf("QueryContext returned no row: %v", rows.Err())
			}
			if err := rows.Scan(value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan typed QueryContext LOB: %v", err)
			}
			if !value.Valid() || value.Kind() != test.kind {
				_ = rows.Close()
				t.Fatalf("QueryContext LOB state = (valid=%t, kind=%v), want (true, %v)", value.Valid(), value.Kind(), test.kind)
			}
			if _, err := value.SizeContext(ctx); err != nil {
				_ = rows.Close()
				t.Fatalf("Size before Rows.Close: %v", err)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("close QueryContext Rows: %v", err)
			}
			got := string(readTypedLOBValue(t, ctx, value))
			if got != test.payload {
				t.Fatalf("query value = %q, want %q", got, test.payload)
			}

			rowValue, err := newTypedLOBTarget(test.kind)
			if err != nil {
				t.Fatalf("create QueryRow scanner: %v", err)
			}
			defer rowValue.FreeContext(ctx)
			if err := conn.QueryRowContext(ctx, query, int64(1)).Scan(rowValue); err != nil {
				t.Fatalf("QueryRowContext typed Scan: %v", err)
			}
			if !rowValue.Valid() || rowValue.Kind() != test.kind {
				t.Fatalf("QueryRowContext LOB state = (valid=%t, kind=%v), want (true, %v)", rowValue.Valid(), rowValue.Kind(), test.kind)
			}
			if _, err := rowValue.SizeContext(ctx); err != nil {
				t.Fatalf("QueryRowContext typed value after Scan: %v", err)
			}
			if got := string(readTypedLOBValue(t, ctx, rowValue)); got != test.payload {
				t.Fatalf("QueryRowContext value = %q, want %q", got, test.payload)
			}
		})
	}
}

// TestDriver_TypedLOBZeroValueScanAfterRowsClose verifies zero-value BLOB,
// CLOB, and NCLOB destinations remain readable after Rows closes while their
// dedicated *sql.Conn stays open.
func TestDriver_TypedLOBZeroValueScanAfterRowsClose(t *testing.T) {
	t.Parallel()
	db, ctx, table := createTypedLOBScanFixture(t)
	blobPayload := []byte{0, 1, 2, 255, 128}
	clobPayload := "CLOB: Aé中🙂 e\u0301"
	nclobPayload := "NCLOB: न्‍न🙂"

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var blobScanner datatypeLOB.Blob
	var clobScanner datatypeLOB.Clob
	var nclobScanner datatypeLOB.NClob
	defer blobScanner.Close()
	defer clobScanner.Close()
	defer nclobScanner.Close()
	query := fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table)
	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		t.Fatalf("query typed scanners: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatalf("typed scanner row missing: %v", rows.Err())
	}
	if err := rows.Scan(&blobScanner, &clobScanner, &nclobScanner); err != nil {
		_ = rows.Close()
		t.Fatalf("scan typed values: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close typed scanner rows: %v", err)
	}

	if !blobScanner.Valid() || blobScanner.Kind() != datatypeLOB.BLOB || !clobScanner.Valid() || clobScanner.Kind() != datatypeLOB.CLOB || !nclobScanner.Valid() || nclobScanner.Kind() != datatypeLOB.NCLOB {
		t.Fatal("zero-value scans did not adopt their LOB kinds")
	}
	if got := readTypedLOBValue(t, ctx, &blobScanner); !bytes.Equal(got, blobPayload) {
		t.Fatalf("typed BLOB after Rows.Close = %x, want %x", got, blobPayload)
	}
	if got := string(readTypedLOBValue(t, ctx, &clobScanner)); got != clobPayload {
		t.Fatalf("typed CLOB after Rows.Close = %q, want %q", got, clobPayload)
	}
	if got := string(readTypedLOBValue(t, ctx, &nclobScanner)); got != nclobPayload {
		t.Fatalf("typed NCLOB after Rows.Close = %q, want %q", got, nclobPayload)
	}
	if size, err := clobScanner.SizeContext(ctx); err != nil || size != int64(len(utf16.Encode([]rune(clobPayload)))) {
		t.Fatalf("typed CLOB logical size = (%d, %v)", size, err)
	}
	if size, err := nclobScanner.SizeContext(ctx); err != nil || size != int64(len(utf16.Encode([]rune(nclobPayload)))) {
		t.Fatalf("typed NCLOB logical size = (%d, %v)", size, err)
	}
}

// TestDriver_TypedLOBNullScans verifies SQL NULL scans preserve the declared
// kind and leave zero-value BLOB, CLOB, and NCLOB destinations invalid.
func TestDriver_TypedLOBNullScans(t *testing.T) {
	t.Parallel()
	db, ctx, table := createTypedLOBScanFixture(t)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated connection: %v", err)
	}
	defer conn.Close()
	var blob datatypeLOB.Blob
	var clob datatypeLOB.Clob
	var nclob datatypeLOB.NClob
	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table), int64(2))
	if err != nil {
		t.Fatalf("query NULL LOBs: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatalf("NULL LOB row missing: %v", rows.Err())
	}
	if err := rows.Scan(&blob, &clob, &nclob); err != nil {
		_ = rows.Close()
		t.Fatalf("scan NULL LOBs: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close NULL LOB rows: %v", err)
	}
	if blob.Valid() || blob.Kind() != datatypeLOB.BLOB || clob.Valid() || clob.Kind() != datatypeLOB.CLOB || nclob.Valid() || nclob.Kind() != datatypeLOB.NCLOB {
		t.Fatal("typed NULL LOB state did not preserve kind and invalid status")
	}
}

// TestDriver_TypedLOBWrongKindScan verifies typed destinations reject a column
// with a different LOB kind.
func TestDriver_TypedLOBWrongKindScan(t *testing.T) {
	t.Parallel()
	db, ctx, table := createTypedLOBScanFixture(t)
	for _, test := range []struct {
		name       string
		column     string
		sourceKind datatypeLOB.Kind
		targetKind datatypeLOB.Kind
	}{
		{name: "BLOB_to_CLOB", column: "bin", sourceKind: datatypeLOB.BLOB, targetKind: datatypeLOB.CLOB},
		{name: "BLOB_to_NCLOB", column: "bin", sourceKind: datatypeLOB.BLOB, targetKind: datatypeLOB.NCLOB},
		{name: "CLOB_to_BLOB", column: "txt", sourceKind: datatypeLOB.CLOB, targetKind: datatypeLOB.BLOB},
		{name: "CLOB_to_NCLOB", column: "txt", sourceKind: datatypeLOB.CLOB, targetKind: datatypeLOB.NCLOB},
		{name: "NCLOB_to_BLOB", column: "ntxt", sourceKind: datatypeLOB.NCLOB, targetKind: datatypeLOB.BLOB},
		{name: "NCLOB_to_CLOB", column: "ntxt", sourceKind: datatypeLOB.NCLOB, targetKind: datatypeLOB.CLOB},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("get dedicated connection: %v", err)
			}
			defer conn.Close()
			target, err := newTypedLOBTarget(test.targetKind)
			if err != nil {
				t.Fatalf("create typed %v scanner: %v", test.targetKind, err)
			}
			defer target.Close()
			rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE id = :1", test.column, table), int64(1))
			if err != nil {
				t.Fatalf("query %v: %v", test.sourceKind, err)
			}
			if !rows.Next() {
				_ = rows.Close()
				t.Fatalf("%v row missing: %v", test.sourceKind, rows.Err())
			}
			wrongKindErr := rows.Scan(target)
			_ = rows.Close()
			if wrongKindErr == nil {
				t.Fatalf("typed %v scanner accepted %v", test.targetKind, test.sourceKind)
			}
			var wrongKindSQLError oracleErrors.SQLError
			if !errors.As(wrongKindErr, &wrongKindSQLError) || wrongKindSQLError.ErrorCode() != string(oracleErrors.InvalidLobSource) {
				t.Fatalf("wrong-kind scan error = %v", wrongKindErr)
			}
		})
	}
}

// TestDriver_TypedLOBPositionalBLOBRead verifies zero-based BLOB ReadAt
// offsets, EOF behavior, and independence from sequential reads after Rows.Close.
func TestDriver_TypedLOBPositionalBLOBRead(t *testing.T) {
	t.Parallel()
	db, ctx, table := createTypedLOBScanFixture(t)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var blob datatypeLOB.Blob
	defer blob.FreeContext(ctx)
	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT bin FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query BLOB: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatalf("BLOB row missing: %v", rows.Err())
	}
	if err := rows.Scan(&blob); err != nil {
		_ = rows.Close()
		t.Fatalf("scan BLOB: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close BLOB rows: %v", err)
	}

	sequential := make([]byte, 1)
	if n, err := blob.ReadContext(ctx, sequential); n != 1 || err != nil || sequential[0] != 0 {
		t.Fatalf("first sequential read = (%d, %v, %x), want (1, nil, 00)", n, err, sequential[:n])
	}

	start := make([]byte, 2)
	if n, err := blob.ReadAt(start, 0); n != 2 || err != nil || !bytes.Equal(start, []byte{0, 1}) {
		t.Fatalf("ReadAt at zero = (%d, %v, %x), want (2, nil, 0001)", n, err, start)
	}
	middle := make([]byte, 2)
	if n, err := blob.ReadAtContext(ctx, middle, 2); n != 2 || err != nil || !bytes.Equal(middle, []byte{2, 255}) {
		t.Fatalf("ReadAtContext in middle = (%d, %v, %x), want (2, nil, 02ff)", n, err, middle)
	}
	end := make([]byte, 2)
	if n, err := blob.ReadAt(end, 4); n != 1 || !errors.Is(err, io.EOF) || end[0] != 128 {
		t.Fatalf("ReadAt short final range = (%d, %v, %x), want (1, io.EOF, 80)", n, err, end[:n])
	}
	atEOF := make([]byte, 1)
	if n, err := blob.ReadAt(atEOF, 5); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt at EOF = (%d, %v), want (0, io.EOF)", n, err)
	}

	remaining := make([]byte, 4)
	if n, err := blob.ReadContext(ctx, remaining); n != 4 || err != nil || !bytes.Equal(remaining, []byte{1, 2, 255, 128}) {
		t.Fatalf("sequential read after positional reads = (%d, %v, %x), want 0102ff80", n, err, remaining)
	}
}

// TestDriver_TypedLOBPositionalTextRead verifies CLOB and NCLOB positions use
// UTF-16 units and return complete UTF-8 characters after Rows.Close.
func TestDriver_TypedLOBPositionalTextRead(t *testing.T) {
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
		"id": "NUMBER PRIMARY KEY", "txt": "CLOB", "ntxt": "NCLOB",
	}); err != nil {
		t.Fatalf("create text LOB table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })
	const text = "Aé🙂Z"
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (id, txt, ntxt) VALUES (:1, :2, :3)", table),
		int64(1), BindClob(text), BindNClob(text)); err != nil {
		t.Fatalf("insert text LOBs: %v", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var clob datatypeLOB.Clob
	var nclob datatypeLOB.NClob
	defer clob.FreeContext(ctx)
	defer nclob.FreeContext(ctx)
	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT txt, ntxt FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query text LOBs: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatalf("text LOB row missing: %v", rows.Err())
	}
	if err := rows.Scan(&clob, &nclob); err != nil {
		_ = rows.Close()
		t.Fatalf("scan text LOBs: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close text LOB rows: %v", err)
	}

	for _, test := range []struct {
		name  string
		value interface {
			ReadTextAt([]byte, int64) (int, int64, error)
			ReadTextAtContext(context.Context, []byte, int64) (int, int64, error)
		}
	}{
		{name: "CLOB", value: &clob},
		{name: "NCLOB", value: &nclob},
	} {
		t.Run(test.name, func(t *testing.T) {
			exactPrefix := make([]byte, len("Aé"))
			n, units, err := test.value.ReadTextAt(exactPrefix, 0)
			if n != len(exactPrefix) || units != 2 || err != nil || string(exactPrefix) != "Aé" {
				t.Fatalf("ReadTextAt exact prefix = (%d, %d, %v, %q), want (%d, 2, nil, Aé)", n, units, err, exactPrefix, len(exactPrefix))
			}

			nearSupplement := make([]byte, 4)
			n, units, err = test.value.ReadTextAt(nearSupplement, 0)
			if n != 3 || units != 2 || !errors.Is(err, io.ErrShortBuffer) || string(nearSupplement[:n]) != "Aé" || !utf8.Valid(nearSupplement[:n]) {
				t.Fatalf("ReadTextAt before supplementary character = (%d, %d, %v, %q), want (3, 2, ErrShortBuffer, Aé)", n, units, err, nearSupplement[:n])
			}

			tooSmall := make([]byte, 3)
			n, units, err = test.value.ReadTextAtContext(ctx, tooSmall, 2)
			if n != 0 || units != 0 || !errors.Is(err, io.ErrShortBuffer) {
				t.Fatalf("ReadTextAt supplementary character in small buffer = (%d, %d, %v), want (0, 0, ErrShortBuffer)", n, units, err)
			}

			whole := make([]byte, len(text))
			n, units, err = test.value.ReadTextAtContext(ctx, whole, 0)
			if n != len(text) || units != 5 || err != nil || string(whole) != text || !utf8.Valid(whole) {
				t.Fatalf("ReadTextAt whole value = (%d, %d, %v, %q), want (%d, 5, nil, %q)", n, units, err, whole, len(text), text)
			}

			insidePair := make([]byte, 4)
			if n, units, err := test.value.ReadTextAt(insidePair, 3); n != 0 || units != 0 || err == nil {
				t.Fatalf("ReadTextAt inside supplementary pair = (%d, %d, %v), want error", n, units, err)
			}

			trailing := make([]byte, 4)
			n, units, err = test.value.ReadTextAt(trailing, 4)
			if n != 1 || units != 1 || !errors.Is(err, io.EOF) || string(trailing[:n]) != "Z" {
				t.Fatalf("ReadTextAt trailing character = (%d, %d, %v, %q), want (1, 1, io.EOF, Z)", n, units, err, trailing[:n])
			}
		})
	}
}

func createTypedLOBScanFixture(t *testing.T) (*sql.DB, context.Context, string) {
	t.Helper()
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
		int64(1), BindBlob([]byte{0, 1, 2, 255, 128}), BindClob("CLOB: Aé中🙂 e\u0301"), BindNClob("NCLOB: न्‍न🙂")); err != nil {
		t.Fatalf("insert LOBs: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, NULL, NULL, NULL)", table),
		int64(2)); err != nil {
		t.Fatalf("insert NULL LOB row: %v", err)
	}
	return db, ctx, table
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
