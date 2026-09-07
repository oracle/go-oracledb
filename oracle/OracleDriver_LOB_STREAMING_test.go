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
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/lob"
)

// TestDriver_LobStreamingReadLOBs verifies locator-backed BLOB, CLOB, and
// NCLOB reads against Oracle. Values are inserted through established
// materialized binds so the test isolates the streaming read API.
func TestDriver_LobStreamingReadLOBs(t *testing.T) {
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

	query := fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table)
	rows, err := db.QueryContext(ctx, query, int64(1))
	if err != nil {
		t.Fatalf("streamed query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("streamed query returned no row: %v", rows.Err())
	}

	var blobValue, clobValue, nclobValue lob.LOB
	if err := rows.Scan(&blobValue, &clobValue, &nclobValue); err != nil {
		t.Fatalf("scan streamed LOBs: %v", err)
	}
	if size, err := blobValue.Size(); err != nil || size != int64(len(blobPayload)) {
		t.Fatalf("BLOB Size = (%d, %v), want (%d, nil)", size, err, len(blobPayload))
	}
	wantSize := int64(len(utf16.Encode([]rune(clobPayload))))
	if size, err := clobValue.Size(); err != nil || size != wantSize {
		t.Fatalf("CLOB Size = (%d, %v), want (%d, nil)", size, err, wantSize)
	}
	wantNClobSize := int64(len(utf16.Encode([]rune(nclobPayload))))
	if size, err := nclobValue.Size(); err != nil || size != wantNClobSize {
		t.Fatalf("NCLOB Size = (%d, %v), want (%d, nil)", size, err, wantNClobSize)
	}
	for _, test := range []struct {
		name  string
		value *lob.LOB
	}{
		{name: "BLOB", value: &blobValue},
		{name: "CLOB", value: &clobValue},
		{name: "NCLOB", value: &nclobValue},
	} {
		if chunkSize, err := test.value.ChunkSize(); err != nil || chunkSize <= 0 {
			t.Fatalf("%s ChunkSize = (%d, %v), want positive size", test.name, chunkSize, err)
		}
	}
	if got := readFunctionalLobValue(t, &blobValue); !bytes.Equal(got, blobPayload) {
		t.Fatal("streamed BLOB payload mismatch")
	}
	if got := string(readFunctionalLobValue(t, &clobValue)); got != clobPayload {
		at := 0
		for at < len(got) && at < len(clobPayload) && got[at] == clobPayload[at] {
			at++
		}
		gotEnd, wantEnd := min(at+20, len(got)), min(at+20, len(clobPayload))
		t.Fatalf("streamed CLOB payload differs at byte %d: got %q, want %q; length = %d, want %d", at, got[at:gotEnd], clobPayload[at:wantEnd], len(got), len(clobPayload))
	}
	if got := string(readFunctionalLobValue(t, &nclobValue)); got != nclobPayload {
		t.Fatal("streamed NCLOB payload mismatch")
	}
	if rows.Next() {
		t.Fatal("streamed query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("streamed rows: %v", err)
	}
}

// TestDriver_LobStreamingReadMultipleBLOBColumns verifies that two streamed
// BLOB columns from one query keep independent offsets and buffers while
// sharing the physical session's cached BLOB executor.
func TestDriver_LobStreamingReadMultipleBLOBColumns(t *testing.T) {
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
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	firstPayload := binaryFixture(functionalLobPrefetchSize + 73)
	secondPayload := binaryFixture(functionalLobPrefetchSize + 211)
	insert := fmt.Sprintf("INSERT INTO %s (id, first, second) VALUES (:1, :2, :3)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), BindBlob(firstPayload), BindBlob(secondPayload)); err != nil {
		t.Fatalf("insert BLOB columns: %v", err)
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT first, second FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query BLOB columns: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("query returned no row: %v", rows.Err())
	}

	var firstValue, secondValue lob.LOB
	if err := rows.Scan(&firstValue, &secondValue); err != nil {
		t.Fatalf("scan BLOB columns: %v", err)
	}
	if size, err := firstValue.Size(); err != nil || size != int64(len(firstPayload)) {
		t.Fatalf("first BLOB Size = (%d, %v), want (%d, nil)", size, err, len(firstPayload))
	}
	if size, err := secondValue.Size(); err != nil || size != int64(len(secondPayload)) {
		t.Fatalf("second BLOB Size = (%d, %v), want (%d, nil)", size, err, len(secondPayload))
	}

	prefix := make([]byte, 23)
	if n, err := io.ReadFull(&firstValue, prefix); err != nil || n != len(prefix) {
		t.Fatalf("read first BLOB prefix = (%d, %v), want (%d, nil)", n, err, len(prefix))
	}
	gotSecond := readFunctionalLobValue(t, &secondValue)
	gotFirstRemainder := readFunctionalLobValue(t, &firstValue)
	gotFirst := append(append([]byte(nil), prefix...), gotFirstRemainder...)
	if !bytes.Equal(gotFirst, firstPayload) {
		t.Fatalf("first BLOB payload mismatch after interleaved read")
	}
	if !bytes.Equal(gotSecond, secondPayload) {
		t.Fatalf("second BLOB payload mismatch after interleaved read")
	}
	if rows.Next() {
		t.Fatal("query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("streamed rows: %v", err)
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
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

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

// TestDriver_LobStreamingReadExceedsPrefetch verifies that a locator read
// continues beyond the connection prefetch limit. Its digest is calculated
// while reading fixed-size buffers, so the test never materializes a second
// copy of the large value.
func TestDriver_LobStreamingReadExceedsPrefetch(t *testing.T) {
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
		"id":  "NUMBER PRIMARY KEY",
		"bin": "BLOB",
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = dropTable(ctx, db, table) })

	size := functionalLobPrefetchSize + 1
	t.Logf("create and stream %d-byte BLOB", size)
	payload := binaryFixture(size)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, bin) VALUES (:1, :2)", table), int64(1), BindBlob(payload)); err != nil {
		t.Fatalf("insert streamed BLOB: %v", err)
	}
	wantDigest := sha256.Sum256(payload)

	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT bin FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query streamed BLOB: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("streamed query returned no row: %v", rows.Err())
	}
	var value lob.LOB
	if err := rows.Scan(&value); err != nil {
		t.Fatalf("scan streamed BLOB: %v", err)
	}
	if gotSize, err := value.Size(); err != nil || gotSize != int64(size) {
		t.Fatalf("streamed BLOB Size = (%d, %v), want (%d, nil)", gotSize, err, size)
	}

	hash := sha256.New()
	if _, err := io.CopyBuffer(hash, &value, make([]byte, internallob.DefaultBlobLobChunkBytes)); err != nil {
		t.Fatalf("stream large BLOB: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("close streamed BLOB: %v", err)
	}
	if gotDigest := hash.Sum(nil); !bytes.Equal(gotDigest, wantDigest[:]) {
		t.Fatalf("streamed BLOB digest = %x, want %x", gotDigest, wantDigest)
	}
	if rows.Next() {
		t.Fatal("streamed query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("streamed rows: %v", err)
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

// TestDriver_LobLocatorOperations verifies every public operation supported by
// a scanned locator-backed LOB: Size, ChunkSize, and Close. It runs each
// operation for BLOB, CLOB, and NCLOB against Oracle. Server-side open/close
// state is an executor-only protocol concern, not a query LOB API.
func TestDriver_LobLocatorOperations(t *testing.T) {
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
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT bin, txt, ntxt FROM %s WHERE id = :1", table), int64(1))
	if err != nil {
		t.Fatalf("query LOBs: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("query returned no row: %v", rows.Err())
	}
	var blobValue, clobValue, nclobValue lob.LOB
	if err := rows.Scan(&blobValue, &clobValue, &nclobValue); err != nil {
		t.Fatalf("scan LOBs: %v", err)
	}
	for _, test := range []struct {
		name  string
		value *lob.LOB
	}{
		{name: "BLOB", value: &blobValue}, {name: "CLOB", value: &clobValue}, {name: "NCLOB", value: &nclobValue},
	} {
		if size, err := test.value.Size(); err != nil || size != 6 {
			t.Fatalf("%s Size = (%d, %v), want (6, nil)", test.name, size, err)
		}
		if chunk, err := test.value.ChunkSize(); err != nil || chunk <= 0 {
			t.Fatalf("%s ChunkSize = (%d, %v), want positive size", test.name, chunk, err)
		}
		if err := test.value.Close(); err != nil {
			t.Fatalf("%s Close: %v", test.name, err)
		}
		requireLOBErrorCode(t, oracleErrors.LobValueClosed, func() error {
			_, err := test.value.Size()
			return err
		})
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}
}

// TestDriver_LobDirectTemporary verifies each direct temporary LOB operation
// for BLOB, CLOB, and NCLOB: write, size, chunk size, trim,
// server open/close state, local close, and unified Free cleanup.
func TestDriver_LobDirectTemporary(t *testing.T) {
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
		length  int64
	}{
		{name: "BLOB", kind: lob.BLOB, payload: []byte("direct blob payload"), length: int64(len("direct blob payload"))},
		{name: "CLOB", kind: lob.CLOB, payload: []byte("direct clob payload"), length: int64(len("direct clob payload"))},
		{name: "NCLOB", kind: lob.NCLOB, payload: []byte("direct nclob payload"), length: int64(len("direct nclob payload"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := lob.CreateTemporary(ctx, conn, test.kind)
			if err != nil {
				t.Fatalf("create temporary LOB: %v", err)
			}
			if written, err := value.WriteContext(ctx, test.payload); err != nil || written != len(test.payload) {
				t.Fatalf("write = (%d, %v), want (%d, nil)", written, err, len(test.payload))
			}
			if size, err := value.Size(ctx); err != nil || size != test.length {
				t.Fatalf("size = (%d, %v), want (%d, nil)", size, err, test.length)
			}
			if chunk, err := value.ChunkSize(ctx); err != nil || chunk <= 0 {
				t.Fatalf("chunk size = (%d, %v), want positive size", chunk, err)
			}
			if opened, err := value.Open(ctx, lob.ReadWrite); err != nil || !opened {
				t.Fatalf("open = (%t, %v), want (true, nil)", opened, err)
			}
			if open, err := value.IsOpen(ctx); err != nil || !open {
				t.Fatalf("is open = (%t, %v), want (true, nil)", open, err)
			}
			if err := value.CloseServer(ctx); err != nil {
				t.Fatalf("close server: %v", err)
			}
			if open, err := value.IsOpen(ctx); err != nil || open {
				t.Fatalf("is open after CloseServer = (%t, %v), want (false, nil)", open, err)
			}
			if err := value.Trim(ctx, test.length-1); err != nil {
				t.Fatalf("trim: %v", err)
			}
			if size, err := value.Size(ctx); err != nil || size != test.length-1 {
				t.Fatalf("size after trim = (%d, %v), want (%d, nil)", size, err, test.length-1)
			}
			if err := value.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if err := value.Free(ctx); err != nil {
				t.Fatalf("free after close: %v", err)
			}
			var dual int
			if err := conn.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&dual); err != nil || dual != 1 {
				t.Fatalf("ordinary request after temporary Free = (%d, %v), want (1, nil)", dual, err)
			}
			requireLOBErrorCode(t, oracleErrors.LobValueClosed, func() error {
				_, err := value.Size(ctx)
				return err
			})
		})
	}
}

// TestDriver_LobDirectUnicodeAndCancellation verifies split supplementary
// characters, cancellation before direct reads/writes/frees, and successful
// recovery with a live context.
func TestDriver_LobDirectUnicodeAndCancellation(t *testing.T) {
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
		value, err := lob.CreateTemporary(ctx, conn, kind)
		if err != nil {
			t.Fatalf("create %v: %v", kind, err)
		}
		payload := []byte(strings.Repeat("🙂", 1024))
		if _, err := value.WriteContext(ctx, payload); err != nil {
			t.Fatalf("write %v: %v", kind, err)
		}
		wantSize := int64(len(utf16.Encode([]rune(string(payload)))))
		if size, err := value.Size(ctx); err != nil || size != wantSize {
			t.Fatalf("unicode size for %v = (%d, %v), want (%d, nil)", kind, size, err, wantSize)
		}
		if err := value.Free(ctx); err != nil {
			t.Fatalf("free %v: %v", kind, err)
		}
	}

	value, err := lob.CreateTemporary(ctx, conn, lob.BLOB)
	if err != nil {
		t.Fatalf("create cancellation LOB: %v", err)
	}
	if _, err := value.Write([]byte("cancellation payload")); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := value.ReadContext(canceled, make([]byte, 8)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read error = %v, want context.Canceled", err)
	}
	if _, err := value.Size(ctx); err != nil {
		t.Fatalf("size after canceled read: %v", err)
	}
	if _, err := value.WriteContext(canceled, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write error = %v, want context.Canceled", err)
	}
	if _, err := value.WriteContext(ctx, []byte("x")); err != nil {
		t.Fatalf("write after cancellation: %v", err)
	}
	if err := value.Free(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled free error = %v, want context.Canceled", err)
	}
	if err := value.Free(ctx); err != nil {
		t.Fatalf("free after cancellation: %v", err)
	}
}

// TestDriver_LobDirectPersistentHandoff verifies persistent BLOB, CLOB, and
// NCLOB promotion from rows to DirectLOB, including cursor close, read, write,
// trim, and local Free cleanup on the same dedicated connection.
func TestDriver_LobDirectPersistentHandoff(t *testing.T) {
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
		payload []byte
	}{
		{name: "BLOB", column: "bin", payload: []byte("blob")},
		{name: "CLOB", column: "txt", payload: []byte("clob 🙂")},
		{name: "NCLOB", column: "ntxt", payload: []byte("nclob 🙂")},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := fmt.Sprintf("SELECT %s FROM %s WHERE id = :1 FOR UPDATE", test.column, table)
			rows, err := conn.QueryContext(ctx, query, int64(1))
			if err != nil {
				t.Fatalf("query persistent LOB: %v", err)
			}
			if !rows.Next() {
				t.Fatalf("query returned no row: %v", rows.Err())
			}
			var queryLOB lob.LOB
			if err := rows.Scan(&queryLOB); err != nil {
				t.Fatalf("scan persistent LOB: %v", err)
			}
			value, err := lob.OpenPersistent(ctx, conn, &queryLOB)
			if err != nil {
				_ = rows.Close()
				t.Fatalf("open persistent LOB: %v", err)
			}
			if err := rows.Close(); err != nil {
				t.Fatalf("close source rows: %v", err)
			}
			// Valid reports whether Scan saw SQL NULL, not the locator lifecycle.
			// Promotion closes the source locator while retaining that scan fact.
			requireLOBErrorCode(t, oracleErrors.LobValueClosed, func() error {
				_, err := queryLOB.Size()
				return err
			})
			originalSize, err := value.Size(ctx)
			if err != nil {
				t.Fatalf("size before persistent read: %v", err)
			}
			if got := readDirectLobValue(t, ctx, value); !bytes.Equal(got, test.payload) {
				t.Fatalf("read after Rows.Close = %q, want %q", got, test.payload)
			} else if test.column != "bin" && !utf8.Valid(got) {
				t.Fatalf("reassembled %s read is not valid UTF-8: %x", test.name, got)
			}
			if _, err := value.WriteContext(ctx, []byte("!")); err != nil {
				t.Fatalf("write persistent LOB: %v", err)
			}
			var appended []byte
			if test.column == "bin" {
				var binary lob.Bytes
				if err := conn.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE id = :1", test.column, table), int64(1)).Scan(&binary); err != nil {
					t.Fatalf("query persisted LOB after append: %v", err)
				}
				appended = binary
			} else {
				var text lob.Text
				if err := conn.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE id = :1", test.column, table), int64(1)).Scan(&text); err != nil {
					t.Fatalf("query persisted LOB after append: %v", err)
				}
				appended = []byte(text)
			}
			wantAppended := append(append([]byte(nil), test.payload...), '!')
			if !bytes.Equal(appended, wantAppended) {
				t.Fatalf("persisted LOB after append = %q, want %q", appended, wantAppended)
			}
			if err := value.Trim(ctx, originalSize); err != nil {
				t.Fatalf("trim persistent LOB: %v", err)
			}
			if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
				t.Fatalf("commit persistent LOB mutation: %v", err)
			}
			verifyConn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("open independent verification connection: %v", err)
			}
			var got []byte
			var databaseLength int64
			verificationQuery := fmt.Sprintf("SELECT %s, DBMS_LOB.GETLENGTH(%s) FROM %s WHERE id = :1", test.column, test.column, table)
			if test.column == "bin" {
				var binary lob.Bytes
				err = verifyConn.QueryRowContext(ctx, verificationQuery, int64(1)).Scan(&binary, &databaseLength)
				got = binary
			} else {
				var text lob.Text
				err = verifyConn.QueryRowContext(ctx, verificationQuery, int64(1)).Scan(&text, &databaseLength)
				got = []byte(text)
			}
			closeErr := verifyConn.Close()
			if err != nil {
				t.Fatalf("query committed persisted LOB: %v", err)
			}
			if closeErr != nil {
				t.Fatalf("close independent verification connection: %v", closeErr)
			}
			wantDatabaseLength := originalSize
			if !bytes.Equal(got, test.payload) || databaseLength != wantDatabaseLength {
				t.Fatalf("committed persisted LOB = (%q, length %d), want (%q, length %d)", got, databaseLength, test.payload, wantDatabaseLength)
			}
			if err := value.Free(ctx); err != nil {
				t.Fatalf("free persistent LOB: %v", err)
			}
			requireLOBErrorCode(t, oracleErrors.LobValueClosed, func() error {
				_, err := value.Size(ctx)
				return err
			})
		})
	}
}

// TestDriver_LobDirectLifecycleRejections verifies connection identity, partial
// query reads, and a closed dedicated connection reject direct LOB operations.
func TestDriver_LobDirectLifecycleRejections(t *testing.T) {
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
		t.Fatalf("open first connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	otherConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	t.Cleanup(func() { _ = otherConn.Close() })
	query := fmt.Sprintf("SELECT bin FROM %s WHERE id = :1", table)

	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		t.Fatalf("query for connection mismatch: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("connection mismatch query returned no row: %v", rows.Err())
	}
	var queryLOB lob.LOB
	if err := rows.Scan(&queryLOB); err != nil {
		t.Fatalf("scan for connection mismatch: %v", err)
	}
	requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
		_, err := lob.OpenPersistent(ctx, otherConn, &queryLOB)
		return err
	})
	promoted, err := lob.OpenPersistent(ctx, conn, &queryLOB)
	if err != nil {
		t.Fatalf("promote source after connection mismatch: %v", err)
	}
	if err := promoted.Close(); err != nil {
		t.Fatalf("close promoted persistent LOB: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close mismatched rows: %v", err)
	}

	rows, err = conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		t.Fatalf("query for partial read: %v", err)
	}
	if !rows.Next() {
		t.Fatalf("partial read query returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&queryLOB); err != nil {
		t.Fatalf("scan for partial read: %v", err)
	}
	if _, err := queryLOB.Read(make([]byte, 1)); err != nil {
		t.Fatalf("partial read: %v", err)
	}
	requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
		_, err := lob.OpenPersistent(ctx, conn, &queryLOB)
		return err
	})
	if err := queryLOB.Close(); err != nil {
		t.Fatalf("close partial source LOB: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close partial rows: %v", err)
	}

	closedConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open closed-connection test connection: %v", err)
	}
	value, err := lob.CreateTemporary(ctx, closedConn, lob.BLOB)
	if err != nil {
		t.Fatalf("create temporary for closed connection: %v", err)
	}
	if err := closedConn.Close(); err != nil {
		t.Fatalf("close dedicated connection: %v", err)
	}
	if _, err := value.Size(ctx); err == nil {
		t.Fatal("size on closed connection unexpectedly succeeded")
	}
}

// readDirectLobValue drains one DirectLOB using small buffers so CLOB and
// NCLOB conversion carry is exercised across public Read calls.
func readDirectLobValue(t *testing.T, ctx context.Context, value *lob.DirectLOB) []byte {
	t.Helper()
	var output bytes.Buffer
	buffer := make([]byte, 3)
	for {
		n, err := value.ReadContext(ctx, buffer)
		if n != 0 {
			_, _ = output.Write(buffer[:n])
		}
		if err == io.EOF {
			return output.Bytes()
		}
		if err != nil {
			t.Fatalf("read direct LOB: %v", err)
		}
	}
}

// readFunctionalLobValue drains one public locator-backed LOB with deliberately
// small buffers and closes it after EOF.
func readFunctionalLobValue(t *testing.T, value *lob.LOB) []byte {
	t.Helper()
	var output bytes.Buffer
	buffer := make([]byte, 17)
	for {
		count, err := value.Read(buffer)
		if count != 0 {
			_, _ = output.Write(buffer[:count])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read streamed LOB: %v", err)
		}
	}
	if err := value.Close(); err != nil {
		t.Fatalf("close streamed LOB: %v", err)
	}
	return output.Bytes()
}
