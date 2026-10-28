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
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	lob "github.com/oracle/go-oracledb/v26/oracle/datatype/lob"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_TypedLOBStreamingReadLOBs verifies typed BLOB, CLOB, and NCLOB
// reads against Oracle after the source Rows have closed.
func TestDriver_TypedLOBStreamingReadLOBs(t *testing.T) {
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
		"id":   "NUMBER PRIMARY KEY",
		"bin":  "BLOB",
		"txt":  "CLOB",
		"ntxt": "NCLOB",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("drop table %s: %v", table, err)
		}
	})

	blobPayload := make([]byte, 256*1024+73)
	for index := range blobPayload {
		blobPayload[index] = byte(index)
	}
	clobPayload := strings.Repeat("CLOB streaming 🙂 ", 8*1024)
	nclobPayload := strings.Repeat("NCLOB streaming 🙂 ", 8*1024)

	insert := fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), blobPayload, clobPayload, nclobPayload); err != nil {
		t.Fatalf("insert LOBs: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated LOB connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	blobValue, err := newTypedLOBTarget(lob.BLOB)
	if err != nil {
		t.Fatalf("create BLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = blobValue.FreeContext(ctx) })
	clobValue, err := newTypedLOBTarget(lob.CLOB)
	if err != nil {
		t.Fatalf("create CLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = clobValue.FreeContext(ctx) })
	nclobValue, err := newTypedLOBTarget(lob.NCLOB)
	if err != nil {
		t.Fatalf("create NCLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = nclobValue.FreeContext(ctx) })

	query := fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table)
	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		t.Fatalf("streamed query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("streamed query returned no row: %v", rows.Err())
	}

	if err := rows.Scan(blobValue, clobValue, nclobValue); err != nil {
		t.Fatalf("scan streamed LOBs: %v", err)
	}
	if rows.Next() {
		t.Fatal("streamed query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("streamed rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close streamed rows: %v", err)
	}
	if size, err := blobValue.SizeContext(ctx); err != nil || size != int64(len(blobPayload)) {
		t.Fatalf("BLOB Size = (%d, %v), want (%d, nil)", size, err, len(blobPayload))
	}
	wantSize := int64(len(utf16.Encode([]rune(clobPayload))))
	if size, err := clobValue.SizeContext(ctx); err != nil || size != wantSize {
		t.Fatalf("CLOB Size = (%d, %v), want (%d, nil)", size, err, wantSize)
	}
	wantNClobSize := int64(len(utf16.Encode([]rune(nclobPayload))))
	if size, err := nclobValue.SizeContext(ctx); err != nil || size != wantNClobSize {
		t.Fatalf("NCLOB Size = (%d, %v), want (%d, nil)", size, err, wantNClobSize)
	}
	for _, test := range []struct {
		name  string
		value lob.LOB
	}{
		{name: "BLOB", value: blobValue},
		{name: "CLOB", value: clobValue},
		{name: "NCLOB", value: nclobValue},
	} {
		if chunkSize, err := test.value.ChunkSizeContext(ctx); err != nil || chunkSize <= 0 {
			t.Fatalf("%s ChunkSize = (%d, %v), want positive size", test.name, chunkSize, err)
		}
	}
	if got := readFunctionalTypedLOBValue(t, ctx, blobValue); !bytes.Equal(got, blobPayload) {
		t.Fatal("streamed BLOB payload mismatch")
	}
	if got := string(readFunctionalTypedLOBValue(t, ctx, clobValue)); got != clobPayload {
		at := 0
		for at < len(got) && at < len(clobPayload) && got[at] == clobPayload[at] {
			at++
		}
		gotEnd, wantEnd := min(at+20, len(got)), min(at+20, len(clobPayload))
		t.Fatalf("streamed CLOB payload differs at byte %d: got %q, want %q; length = %d, want %d", at, got[at:gotEnd], clobPayload[at:wantEnd], len(got), len(clobPayload))
	}
	if got := string(readFunctionalTypedLOBValue(t, ctx, nclobValue)); got != nclobPayload {
		t.Fatal("streamed NCLOB payload mismatch")
	}
}

// TestDriver_TypedLOBStreamingReadMultipleBLOBColumns verifies that two typed
// BLOB columns from one query keep independent offsets and buffers while
// sharing the physical session's cached BLOB executor, including after Rows
// closes.
func TestDriver_TypedLOBStreamingReadMultipleBLOBColumns(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openLobPrefetchTestDB(TestingConfig, functionalLobPrefetchSize)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	table := createLOBObjectName(t)
	if err := createTable(ctx, db, table, map[string]string{
		"id":     "NUMBER PRIMARY KEY",
		"first":  "BLOB",
		"second": "BLOB",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	registerLOBTableCleanup(t, db, table)

	firstPayload := binaryFixture(functionalLobPrefetchSize + 73)
	secondPayload := binaryFixture(functionalLobPrefetchSize + 211)
	insert := fmt.Sprintf("INSERT INTO %s (id, first, second) VALUES (:1, :2, :3)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), BindBlob(firstPayload), BindBlob(secondPayload)); err != nil {
		t.Fatalf("insert BLOB columns: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated LOB connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	firstValue, err := newTypedLOBTarget(lob.BLOB)
	if err != nil {
		t.Fatalf("create first BLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = firstValue.FreeContext(ctx) })
	secondValue, err := newTypedLOBTarget(lob.BLOB)
	if err != nil {
		t.Fatalf("create second BLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = secondValue.FreeContext(ctx) })

	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT first, second FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query BLOB columns: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("query returned no row: %v", rows.Err())
	}

	if err := rows.Scan(firstValue, secondValue); err != nil {
		t.Fatalf("scan BLOB columns: %v", err)
	}
	if rows.Next() {
		t.Fatal("query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close source rows: %v", err)
	}
	if size, err := firstValue.SizeContext(ctx); err != nil || size != int64(len(firstPayload)) {
		t.Fatalf("first BLOB Size = (%d, %v), want (%d, nil)", size, err, len(firstPayload))
	}
	if size, err := secondValue.SizeContext(ctx); err != nil || size != int64(len(secondPayload)) {
		t.Fatalf("second BLOB Size = (%d, %v), want (%d, nil)", size, err, len(secondPayload))
	}

	prefix := make([]byte, 23)
	if n, err := io.ReadFull(firstValue, prefix); err != nil || n != len(prefix) {
		t.Fatalf("read first BLOB prefix = (%d, %v), want (%d, nil)", n, err, len(prefix))
	}
	gotSecond := readFunctionalTypedLOBValue(t, ctx, secondValue)
	gotFirstRemainder := readFunctionalTypedLOBValue(t, ctx, firstValue)
	gotFirst := append(append([]byte(nil), prefix...), gotFirstRemainder...)
	if !bytes.Equal(gotFirst, firstPayload) {
		t.Fatalf("first BLOB payload mismatch after interleaved read")
	}
	if !bytes.Equal(gotSecond, secondPayload) {
		t.Fatalf("second BLOB payload mismatch after interleaved read")
	}
}

const functionalLobPrefetchSize = 32 * 1024

// TestDriver_LobPrefetchMaterializesSmallerValue verifies that a BLOB just
// below the connection prefetch limit is returned as a materialized value
// without opting in to locator-backed reading. The test lowers the prefetch
// limit to keep the same protocol boundary practical for functional runs.
func TestDriver_LobPrefetchMaterializesSmallerValue(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openLobPrefetchTestDB(TestingConfig, functionalLobPrefetchSize)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	table := createLOBObjectName(t)
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER PRIMARY KEY", "bin": "BLOB"}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	registerLOBTableCleanup(t, db, table)

	size := functionalLobPrefetchSize - 1
	t.Logf("create and fetch %d-byte prefetched BLOB", size)
	payload := binaryFixture(size)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:1, :2)", table), int64(1), BindBlob(payload)); err != nil {
		t.Fatalf("insert prefetched BLOB: %v", err)
	}
	var value lob.Bytes
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT bin FROM %s WHERE id = :1", table), int64(1)).Scan(&value); err != nil {
		t.Fatalf("scan prefetched BLOB: %v", err)
	}
	if len(value) != size {
		t.Fatalf("prefetched BLOB length = %d, want %d", len(value), size)
	}
	if gotDigest, wantDigest := sha256.Sum256(value), sha256.Sum256(payload); gotDigest != wantDigest {
		t.Fatalf("prefetched BLOB digest = %x, want %x", gotDigest, wantDigest)
	}
}

// TestDriver_TypedLOBStreamingReadExceedsPrefetch verifies that typed BLOB,
// CLOB, and NCLOB locator reads continue beyond the connection prefetch limit.
// Digests are calculated while reading small buffers, so the test never
// materializes a second copy of the large values.
func TestDriver_TypedLOBStreamingReadExceedsPrefetch(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openLobPrefetchTestDB(TestingConfig, functionalLobPrefetchSize)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	table := createLOBObjectName(t)
	if err := createTable(ctx, db, table, map[string]string{
		"id":   "NUMBER PRIMARY KEY",
		"bin":  "BLOB",
		"txt":  "CLOB",
		"ntxt": "NCLOB",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	registerLOBTableCleanup(t, db, table)

	blobPayload := binaryFixture(functionalLobPrefetchSize + 1)
	textPattern := "Aé中🙂"
	textPatternUnits := len(utf16.Encode([]rune(textPattern)))
	textPayload := strings.Repeat(textPattern, functionalLobPrefetchSize/textPatternUnits+1)
	textLogicalSize := len(utf16.Encode([]rune(textPayload)))
	if textLogicalSize <= functionalLobPrefetchSize {
		t.Fatalf("text payload logical size = %d, want greater than prefetch size %d", textLogicalSize, functionalLobPrefetchSize)
	}
	t.Logf("create and stream BLOB (%d bytes), CLOB (%d bytes), and NCLOB (%d bytes)", len(blobPayload), len(textPayload), len(textPayload))
	insert := fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), BindBlob(blobPayload), BindClob(textPayload), BindNClob(textPayload)); err != nil {
		t.Fatalf("insert streamed LOBs: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated LOB connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	blobValue, err := newTypedLOBTarget(lob.BLOB)
	if err != nil {
		t.Fatalf("create BLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = blobValue.FreeContext(ctx) })
	clobValue, err := newTypedLOBTarget(lob.CLOB)
	if err != nil {
		t.Fatalf("create CLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = clobValue.FreeContext(ctx) })
	nclobValue, err := newTypedLOBTarget(lob.NCLOB)
	if err != nil {
		t.Fatalf("create NCLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = nclobValue.FreeContext(ctx) })

	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query streamed LOBs: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("streamed query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(blobValue, clobValue, nclobValue); err != nil {
		t.Fatalf("scan streamed LOBs: %v", err)
	}
	if rows.Next() {
		t.Fatal("query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close source rows: %v", err)
	}
	wantTextSize := int64(textLogicalSize)
	for _, test := range []struct {
		name    string
		value   lob.LOB
		payload []byte
		units   int64
	}{
		{name: "BLOB", value: blobValue, payload: blobPayload, units: int64(len(blobPayload))},
		{name: "CLOB", value: clobValue, payload: []byte(textPayload), units: wantTextSize},
		{name: "NCLOB", value: nclobValue, payload: []byte(textPayload), units: wantTextSize},
	} {
		t.Run(test.name, func(t *testing.T) {
			if gotSize, err := test.value.SizeContext(ctx); err != nil || gotSize != test.units {
				t.Fatalf("Size = (%d, %v), want (%d, nil)", gotSize, err, test.units)
			}
			hash := sha256.New()
			buffer := make([]byte, 17)
			var read int64
			for {
				n, readErr := test.value.ReadContext(ctx, buffer)
				if n > 0 {
					_, _ = hash.Write(buffer[:n])
					read += int64(n)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					t.Fatalf("stream LOB: %v", readErr)
				}
				if n == 0 {
					t.Fatal("stream LOB made no progress")
				}
			}
			if read != int64(len(test.payload)) {
				t.Fatalf("streamed byte count = %d, want %d", read, len(test.payload))
			}
			if gotDigest, wantDigest := hash.Sum(nil), sha256.Sum256(test.payload); !bytes.Equal(gotDigest, wantDigest[:]) {
				t.Fatalf("streamed digest = %x, want %x", gotDigest, wantDigest)
			}
		})
	}
}

// TestDriver_LobTextMaterializesPastPrefetch verifies that CLOB and NCLOB
// materialization continues through locator reads after the prefetched prefix.
func TestDriver_LobTextMaterializesPastPrefetch(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	db, err := openLobPrefetchTestDB(TestingConfig, functionalLobPrefetchSize)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	table := createLOBObjectName(t)
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER PRIMARY KEY", "txt": "CLOB", "ntxt": "NCLOB"}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })
	payload := strings.Repeat("Aé中🙂", functionalLobPrefetchSize/len("Aé中🙂")+1)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, txt, ntxt) VALUES (:1, :2, :3)", table), int64(1), BindClob(payload), BindNClob(payload)); err != nil {
		t.Fatalf("insert text LOBs: %v", err)
	}
	var clobValue, nclobValue lob.Text
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT txt, ntxt FROM %s WHERE id = :1", table), int64(1)).Scan(&clobValue, &nclobValue); err != nil {
		t.Fatalf("scan text LOBs: %v", err)
	}
	if string(clobValue) != payload || string(nclobValue) != payload {
		t.Fatal("materialized text LOB mismatch")
	}
}

// openLobPrefetchTestDB opens a dedicated test connection with a small
// prefetch threshold. This validates the same below/above-prefetch behavior
// as the 32 MiB default without adding a multi-minute fixture to every run.
func openLobPrefetchTestDB(cfg *TestConfig, prefetchSize int) (*sql.DB, error) {
	dsn := cfg.GetConnectionString()
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn = fmt.Sprintf("%s%voracle.go.DriverProperties.DefaultLobPrefetchSize=%d", dsn, separator, prefetchSize)
	db, err := sql.Open(cfg.Driver.Name, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// TestDriver_TypedLOBLocatorOperations verifies typed Size, ChunkSize, and
// Close operations for BLOB, CLOB, and NCLOB locators after Rows closes.
func TestDriver_TypedLOBLocatorOperations(t *testing.T) {
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
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER PRIMARY KEY", "bin": "BLOB", "txt": "CLOB", "ntxt": "NCLOB"}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table), int64(1), BindBlob([]byte("abcdef")), BindClob("abcdef"), BindNClob("abcdef")); err != nil {
		t.Fatalf("insert LOBs: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("get dedicated LOB connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	blobValue, err := newTypedLOBTarget(lob.BLOB)
	if err != nil {
		t.Fatalf("create BLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = blobValue.FreeContext(ctx) })
	clobValue, err := newTypedLOBTarget(lob.CLOB)
	if err != nil {
		t.Fatalf("create CLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = clobValue.FreeContext(ctx) })
	nclobValue, err := newTypedLOBTarget(lob.NCLOB)
	if err != nil {
		t.Fatalf("create NCLOB scanner: %v", err)
	}
	t.Cleanup(func() { _ = nclobValue.FreeContext(ctx) })
	rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query LOBs: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(blobValue, clobValue, nclobValue); err != nil {
		t.Fatalf("scan LOBs: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close source rows: %v", err)
	}
	for _, test := range []struct {
		name  string
		value lob.LOB
	}{
		{name: "BLOB", value: blobValue}, {name: "CLOB", value: clobValue}, {name: "NCLOB", value: nclobValue},
	} {
		if size, err := test.value.SizeContext(ctx); err != nil || size != 6 {
			t.Fatalf("%s Size = (%d, %v), want (6, nil)", test.name, size, err)
		}
		if chunk, err := test.value.ChunkSizeContext(ctx); err != nil || chunk <= 0 {
			t.Fatalf("%s ChunkSize = (%d, %v), want positive size", test.name, chunk, err)
		}
		if err := test.value.Close(); err != nil {
			t.Fatalf("%s Close: %v", test.name, err)
		}
		requireLOBErrorCode(t, oracleErrors.LobValueClosed, func() error {
			_, err := test.value.SizeContext(ctx)
			return err
		})
	}
}

// TestDriver_TypedLOBTemporary verifies typed temporary LOB operations for
// BLOB, CLOB, and NCLOB, including server state, trim, and cleanup.
func TestDriver_TypedLOBTemporary(t *testing.T) {
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
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, test := range []struct {
		name    string
		kind    lob.Kind
		payload []byte
	}{
		{name: "BLOB", kind: lob.BLOB, payload: []byte("typed blob payload")},
		{name: "CLOB", kind: lob.CLOB, payload: []byte("typed clob payload")},
		{name: "NCLOB", kind: lob.NCLOB, payload: []byte("typed nclob payload")},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := newTemporaryTypedLOB(ctx, conn, test.kind)
			if err != nil {
				t.Fatalf("create temporary LOB: %v", err)
			}
			t.Cleanup(func() { _ = value.FreeContext(ctx) })
			if written, err := value.WriteContext(ctx, test.payload); err != nil || written != len(test.payload) {
				t.Fatalf("write = (%d, %v), want (%d, nil)", written, err, len(test.payload))
			}
			wantLength := int64(len(test.payload))
			if test.kind != lob.BLOB {
				wantLength = int64(len(utf16.Encode([]rune(string(test.payload)))))
			}
			if size, err := value.SizeContext(ctx); err != nil || size != wantLength {
				t.Fatalf("size = (%d, %v), want (%d, nil)", size, err, wantLength)
			}
			if chunk, err := value.ChunkSizeContext(ctx); err != nil || chunk <= 0 {
				t.Fatalf("chunk size = (%d, %v), want positive size", chunk, err)
			}
			if opened, err := value.OpenContext(ctx, lob.ReadWrite); err != nil || !opened {
				t.Fatalf("open = (%t, %v), want (true, nil)", opened, err)
			}
			if open, err := value.IsOpenContext(ctx); err != nil || !open {
				t.Fatalf("is open = (%t, %v), want (true, nil)", open, err)
			}
			if err := value.CloseServerContext(ctx); err != nil {
				t.Fatalf("close server: %v", err)
			}
			if open, err := value.IsOpenContext(ctx); err != nil || open {
				t.Fatalf("is open after CloseServer = (%t, %v), want (false, nil)", open, err)
			}
			if err := value.TrimContext(ctx, wantLength-1); err != nil {
				t.Fatalf("trim: %v", err)
			}
			if size, err := value.SizeContext(ctx); err != nil || size != wantLength-1 {
				t.Fatalf("size after trim = (%d, %v), want (%d, nil)", size, err, wantLength-1)
			}
			if err := value.Close(); err != nil {
				t.Fatalf("close local handle: %v", err)
			}
			if err := value.FreeContext(ctx); err != nil {
				t.Fatalf("free after close: %v", err)
			}
			var dual int
			if err := conn.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&dual); err != nil || dual != 1 {
				t.Fatalf("ordinary request after temporary Free = (%d, %v), want (1, nil)", dual, err)
			}
			requireLOBErrorCode(t, oracleErrors.LobValueClosed, func() error {
				_, err := value.SizeContext(ctx)
				return err
			})
		})
	}
}

// TestDriver_TypedLOBUnicodeLogicalSize verifies CLOB and NCLOB sizes count
// UTF-16 logical units for supplementary characters.
func TestDriver_TypedLOBUnicodeLogicalSize(t *testing.T) {
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
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, kind := range []lob.Kind{lob.CLOB, lob.NCLOB} {
		value, err := newTemporaryTypedLOB(ctx, conn, kind)
		if err != nil {
			t.Fatalf("create %v: %v", kind, err)
		}
		t.Cleanup(func() { _ = value.FreeContext(ctx) })
		payload := []byte(strings.Repeat("🙂", 1024))
		if _, err := value.WriteContext(ctx, payload); err != nil {
			t.Fatalf("write %v: %v", kind, err)
		}
		wantSize := int64(len(utf16.Encode([]rune(string(payload)))))
		if size, err := value.SizeContext(ctx); err != nil || size != wantSize {
			t.Fatalf("unicode size for %v = (%d, %v), want (%d, nil)", kind, size, err, wantSize)
		}
		if err := value.FreeContext(ctx); err != nil {
			t.Fatalf("free %v: %v", kind, err)
		}
	}
}

// TestDriver_TypedLOBPersistentHandoff verifies typed scans detach BLOB,
// CLOB, and NCLOB locators before Rows closes and retain direct operations.
func TestDriver_TypedLOBPersistentHandoff(t *testing.T) {
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
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin, txt, ntxt) VALUES (:1, :2, :3, :4)", table), int64(1), BindBlob([]byte("blob")), BindClob("clob 🙂"), BindNClob("nclob 🙂")); err != nil {
		t.Fatalf("insert persistent LOBs: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, test := range []struct {
		name    string
		column  string
		kind    lob.Kind
		payload []byte
	}{
		{name: "BLOB", column: "bin", kind: lob.BLOB, payload: []byte("blob")},
		{name: "CLOB", column: "txt", kind: lob.CLOB, payload: []byte("clob 🙂")},
		{name: "NCLOB", column: "ntxt", kind: lob.NCLOB, payload: []byte("nclob 🙂")},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE id = :1 FOR UPDATE", test.column, table), int64(1))
			if err != nil {
				t.Fatalf("query persistent LOB: %v", err)
			}
			value, err := newTypedLOBTarget(test.kind)
			if err != nil {
				_ = rows.Close()
				t.Fatalf("create typed scanner: %v", err)
			}
			t.Cleanup(func() { _ = value.FreeContext(ctx) })
			if !rows.Next() {
				_ = rows.Close()
				t.Fatalf("query returned no row: %v", rows.Err())
			}
			if err := rows.Scan(value); err != nil {
				_ = rows.Close()
				t.Fatalf("scan typed LOB: %v", err)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("close source rows: %v", err)
			}
			originalSize, err := value.SizeContext(ctx)
			if err != nil {
				t.Fatalf("size after Rows.Close: %v", err)
			}
			if got := readTypedLOBValue(t, ctx, value); !bytes.Equal(got, test.payload) {
				t.Fatalf("read after Rows.Close = %q, want %q", got, test.payload)
			} else if test.kind != lob.BLOB && !utf8.Valid(got) {
				t.Fatalf("reassembled %s read is not valid UTF-8: %x", test.name, got)
			}
			if _, err := value.WriteContext(ctx, []byte("!")); err != nil {
				t.Fatalf("write persistent LOB: %v", err)
			}
			if err := value.TrimContext(ctx, originalSize); err != nil {
				t.Fatalf("trim persistent LOB: %v", err)
			}
			if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
				t.Fatalf("commit persistent LOB mutation: %v", err)
			}
			if err := value.FreeContext(ctx); err != nil {
				t.Fatalf("free persistent LOB handle: %v", err)
			}
		})
	}
}

// TestDriver_TypedLOBLifecycle verifies a zero-value typed scan remains
// readable after Rows closes while its dedicated *sql.Conn stays open.
func TestDriver_TypedLOBLifecycle(t *testing.T) {
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
	if err := createTable(ctx, db, table, map[string]string{"id": "NUMBER PRIMARY KEY", "bin": "BLOB"}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:1, :2)", table), int64(1), BindBlob([]byte("payload"))); err != nil {
		t.Fatalf("insert LOB: %v", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open dedicated connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	query := fmt.Sprintf("SELECT bin FROM %s WHERE id = :1", table)
	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		t.Fatalf("query for zero-value scan: %v", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatalf("zero-value scan query returned no row: %v", rows.Err())
	}
	var value lob.Blob
	defer value.Close()
	if err := rows.Scan(&value); err != nil {
		_ = rows.Close()
		t.Fatalf("scan zero-value BLOB: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close adopted rows: %v", err)
	}
	if got := string(readFunctionalTypedLOBValue(t, ctx, &value)); got != "payload" {
		t.Fatalf("read after Rows.Close = %q, want payload", got)
	}
}

// readFunctionalTypedLOBValue drains one typed locator with deliberately small
// buffers to exercise UTF-8 carry across ReadContext calls.
func readFunctionalTypedLOBValue(t *testing.T, ctx context.Context, value lob.LOB) []byte {
	t.Helper()
	var output bytes.Buffer
	buffer := make([]byte, 17)
	for {
		count, err := value.ReadContext(ctx, buffer)
		if count != 0 {
			_, _ = output.Write(buffer[:count])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read typed LOB: %v", err)
		}
	}
	return output.Bytes()
}
