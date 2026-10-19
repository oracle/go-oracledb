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
	"fmt"
	"strings"

	"github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/lob"
)

// runLOBPrefetchBoundaryTest writes one BLOB and one CLOB whose values are just
// larger than the configured prefetch size. It demonstrates materializing
// into lob.Bytes/lob.Text and streaming through typed connection-scoped LOBs.
func runLOBPrefetchBoundaryTest(ctx context.Context, db *sql.DB, prefetchSize int) error {
	// Step 1: Create a table for this run and remove it when the scenario ends.
	table, err := createTable(ctx, db, "id NUMBER PRIMARY KEY, blob_value BLOB, clob_value CLOB")
	if err != nil {
		return err
	}
	defer cleanupTable(db, table)

	// Step 2: Insert values slightly larger than the configured prefetch size.
	blobPayload := binaryPayload(prefetchSize + boundaryPayloadExtra64KiB)
	clobPayload := textPayload(prefetchSize + boundaryPayloadExtra64KiB)
	insert := fmt.Sprintf("INSERT INTO %s (id, blob_value, clob_value) VALUES (:1, :2, :3)", table)
	if _, err := db.ExecContext(ctx, insert, int64(1), oracle.BindBlob(blobPayload), oracle.BindClob(clobPayload)); err != nil {
		return fmt.Errorf("insert boundary values: %w", err)
	}

	// Step 3: Materialize complete values with lob.Bytes and lob.Text. These
	// scanners consume the query LOBs during Scan, so QueryRowContext is correct.
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

// binaryPayload creates deterministic binary data of at least size bytes.
func binaryPayload(size int) []byte {
	payload := make([]byte, size)
	for index := range payload {
		payload[index] = byte(index*31 + 7)
	}
	return payload
}

// textPayload creates UTF-8 text of at least minBytes, including multi-byte and
// supplementary characters for CLOB boundary checks.
func textPayload(minBytes int) string {
	const pattern = "Go Oracle LOB example: Aé中🙂\n"
	return strings.Repeat(pattern, minBytes/len(pattern)+1)
}
