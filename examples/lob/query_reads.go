/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all
** copyright and patent rights owned or licensable by each licensor hereunder
** covering the Software, to deal in the Software without restriction, including
** without limitation the rights to use, copy, modify, merge, publish,
** distribute, sublicense, and/or sell copies of the Software, and to permit
** persons to whom the Software is furnished to do so, subject to the following
** conditions:
**
** The above copyright notice and this permission notice shall be included in all
** copies or substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	lob "github.com/oracle/go-oracledb/v26/oracle/datatype/lob"

	"github.com/oracle/go-oracledb/v26/oracle"
)

// -----------------------------------------------------------------------------
// Prefetch boundary and materialized-read scenario
// -----------------------------------------------------------------------------

// runLOBPrefetchBoundaryTest writes one BLOB and one CLOB at least 65,536 units
// larger than the configured prefetch count, then verifies materialized and
// typed reads. The count is bytes for the BLOB and characters for the CLOB.
func runLOBPrefetchBoundaryTest(ctx context.Context, db *sql.DB, prefetchSize int) error {
	// Step 1: Create a table for this run and remove it when the scenario ends.
	table, err := createTable(ctx, db, "id NUMBER PRIMARY KEY, blob_value BLOB, clob_value CLOB")
	if err != nil {
		return err
	}
	defer cleanupTable(db, table)

	// Step 2: Insert values larger than the configured prefetch count in the
	// units Oracle uses for each LOB type.
	blobPayload := binaryPayload(prefetchSize + boundaryPayloadExtraCount64K)
	clobPayload := textPayload(prefetchSize + boundaryPayloadExtraCount64K)
	insert := fmt.Sprintf("INSERT INTO %s (id, blob_value, clob_value) VALUES (:1, :2, :3)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), oracle.BindBlob(blobPayload), oracle.BindClob(clobPayload)); err != nil {
		return fmt.Errorf("insert boundary values: %w", err)
	}

	// Step 3: Materialize complete values with lob.Bytes and lob.Text. These
	// scanners consume the query LOBs during Scan; QueryRowContext is used here
	// because this query returns one row.
	materializedQuery := fmt.Sprintf("SELECT blob_value, clob_value FROM %s WHERE id = :1", table)
	var gotBlob lob.Bytes
	var gotClob lob.Text
	if err := db.QueryRowContext(ctx, materializedQuery, int64(1)).Scan(&gotBlob, &gotClob); err != nil {
		return fmt.Errorf("scan materialized LOBs: %w", err)
	}
	if !bytes.Equal(gotBlob, blobPayload) {
		return fmt.Errorf("materialized BLOB mismatch: got %d bytes, want %d", len(gotBlob), len(blobPayload))
	}
	if string(gotClob) != clobPayload {
		return fmt.Errorf("materialized CLOB mismatch: got %d bytes, want %d", len(gotClob), len(clobPayload))
	}
	fmt.Println("materialized read: BLOB -> []byte, CLOB -> string: ok")

	// Step 4: Run the typed streaming read, which closes Rows before consuming
	// the locators. The same expected values verify both read paths.
	if err := runStreamingLOBTest(ctx, db, table, blobPayload, clobPayload); err != nil {
		return err
	}
	fmt.Println("streaming read: typed BLOB/CLOB after Rows.Close: ok")
	return nil
}

// binaryPayload returns deterministic binary data of exactly size bytes.
func binaryPayload(size int) []byte {
	payload := make([]byte, size)
	for index := range payload {
		payload[index] = byte(index*31 + 7)
	}
	return payload
}

// textPayload creates at least minCharacters ASCII characters followed by
// multi-byte and supplementary characters for CLOB and NCLOB checks.
func textPayload(minCharacters int) string {
	const unicodeSuffix = "Go Oracle LOB example: Aé中🙂\n"
	return strings.Repeat("A", minCharacters) + unicodeSuffix
}

// -----------------------------------------------------------------------------
// Typed streaming-read scenario
// -----------------------------------------------------------------------------

// runStreamingLOBTest validates BLOB and CLOB streaming reads from one row.
// It scans into zero-value typed targets on a dedicated connection, closes
// Rows, and verifies the locators remain usable while that connection stays open.
func runStreamingLOBTest(ctx context.Context, db *sql.DB, table string, wantBlob []byte, wantClob string) (retErr error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get dedicated LOB connection: %w", err)
	}
	defer conn.Close()

	var blobValue lob.Blob
	defer func() {
		if err := blobValue.FreeContext(ctx); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("free streaming BLOB during cleanup: %w", err))
		}
	}()
	var clobValue lob.Clob
	defer func() {
		if err := clobValue.FreeContext(ctx); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("free streaming CLOB during cleanup: %w", err))
		}
	}()

	// Step 1: Query through the same connection that owns both scan targets.
	query := fmt.Sprintf("SELECT blob_value, clob_value FROM %s WHERE id = :1", table)
	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		return fmt.Errorf("query streaming LOBs: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate streaming LOBs: %w", err)
		}
		return fmt.Errorf("query streaming LOBs returned no row")
	}
	if err := rows.Scan(&blobValue, &clobValue); err != nil {
		return fmt.Errorf("scan streaming LOBs: %w", err)
	}
	if rows.Next() {
		return fmt.Errorf("query streaming LOBs returned more than one row")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate streaming LOBs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close source LOB rows: %w", err)
	}

	// Step 2: Verify the LOB kinds and logical sizes before reading their data.
	if err := checkLOBMetadata(ctx, "BLOB", &blobValue, lob.BLOB, int64(len(wantBlob))); err != nil {
		return err
	}
	if err := checkLOBMetadata(ctx, "CLOB", &clobValue, lob.CLOB, logicalTextSize(wantClob)); err != nil {
		return err
	}

	// Step 3: Read and compare both values incrementally. The expected values are
	// also readers, so verification uses only bounded buffers.
	if err := compareLOBContent(ctx, "streaming BLOB", &blobValue, bytes.NewReader(wantBlob)); err != nil {
		return err
	}
	if err := compareLOBContent(ctx, "streaming CLOB", &clobValue, strings.NewReader(wantClob)); err != nil {
		return err
	}
	if err := blobValue.FreeContext(ctx); err != nil {
		return fmt.Errorf("free streaming BLOB: %w", err)
	}
	if err := clobValue.FreeContext(ctx); err != nil {
		return fmt.Errorf("free streaming CLOB: %w", err)
	}
	return nil
}

// checkLOBMetadata verifies the kind, logical size, and chunk size of a query
// LOB before reading it. It uses ctx for metadata operations.
func checkLOBMetadata(ctx context.Context, name string, value lob.LOB, wantKind lob.Kind, wantSize int64) error {
	if !value.Valid() {
		return fmt.Errorf("%s is not valid after Scan", name)
	}
	if gotKind := value.Kind(); gotKind != wantKind {
		return fmt.Errorf("%s kind = %v, want %v", name, gotKind, wantKind)
	}
	if gotSize, err := value.SizeContext(ctx); err != nil {
		return fmt.Errorf("get %s size: %w", name, err)
	} else if gotSize != wantSize {
		return fmt.Errorf("%s size = %d, want %d", name, gotSize, wantSize)
	}
	if chunkSize, err := value.ChunkSizeContext(ctx); err != nil {
		return fmt.Errorf("get %s chunk size: %w", name, err)
	} else if chunkSize <= 0 {
		return fmt.Errorf("%s chunk size = %d, want a positive value", name, chunkSize)
	}
	return nil
}

// logicalTextSize returns the Oracle UTF-16 code-unit length of text.
func logicalTextSize(value string) int64 {
	var size int64
	for _, character := range value {
		if character > 0xffff {
			size += 2
			continue
		}
		size++
	}
	return size
}

// compareLOBContent reads a query LOB with repeated Read calls and compares it
// with expected. Both sides are processed in bounded chunks, so the helper
// verifies the actual content without materializing another complete LOB.
func compareLOBContent(ctx context.Context, name string, value lob.LOB, expected io.Reader) error {
	gotBuffer := make([]byte, 32*1024)
	wantBuffer := make([]byte, len(gotBuffer))
	var offset int
	for {
		count, readErr := value.ReadContext(ctx, gotBuffer)
		if count > len(wantBuffer) {
			return fmt.Errorf("%s returned %d bytes into a %d-byte buffer", name, count, len(wantBuffer))
		}
		wantCount, wantErr := io.ReadFull(expected, wantBuffer[:count])
		if wantCount != count || wantErr != nil || !bytes.Equal(gotBuffer[:count], wantBuffer[:wantCount]) {
			return fmt.Errorf("%s value mismatch at byte %d", name, offset+wantCount)
		}
		if readErr == io.EOF {
			var extra [1]byte
			extraCount, extraErr := expected.Read(extra[:])
			if extraCount != 0 {
				return fmt.Errorf("%s value ended early at byte %d", name, offset+count)
			}
			if extraErr != io.EOF {
				if extraErr == nil {
					return fmt.Errorf("read expected %s: %w", name, io.ErrNoProgress)
				}
				return fmt.Errorf("read expected %s: %w", name, extraErr)
			}
			return nil
		}
		offset += count
		if readErr != nil {
			return fmt.Errorf("read %s: %w", name, readErr)
		}
		if count == 0 {
			return fmt.Errorf("read %s: %w", name, io.ErrNoProgress)
		}
	}
}
