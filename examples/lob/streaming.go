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
	"io"
	"strings"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// runStreamingLOBTest validates BLOB and CLOB streaming reads from one row.
// The zero-value typed scan targets adopt the query locators on this dedicated
// connection, so the LOB values remain usable after Rows is closed.
func runStreamingLOBTest(ctx context.Context, db *sql.DB, table string, wantBlob []byte, wantClob string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get dedicated LOB connection: %w", err)
	}
	defer conn.Close()

	var blobValue datatype.Blob
	defer blobValue.FreeContext(ctx)
	var clobValue datatype.Clob
	defer clobValue.FreeContext(ctx)

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
	if err := checkLOBMetadata("BLOB", &blobValue, datatype.BLOB, int64(len(wantBlob))); err != nil {
		return err
	}
	if err := checkLOBMetadata("CLOB", &clobValue, datatype.CLOB, logicalTextSize(wantClob)); err != nil {
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
// LOB before its content is read.
func checkLOBMetadata(name string, value datatype.LOB, wantKind datatype.Kind, wantSize int64) error {
	if !value.Valid() {
		return fmt.Errorf("%s is not valid after Scan", name)
	}
	if gotKind := value.Kind(); gotKind != wantKind {
		return fmt.Errorf("%s kind = %v, want %v", name, gotKind, wantKind)
	}
	if gotSize, err := value.Size(); err != nil {
		return fmt.Errorf("get %s size: %w", name, err)
	} else if gotSize != wantSize {
		return fmt.Errorf("%s size = %d, want %d", name, gotSize, wantSize)
	}
	if chunkSize, err := value.ChunkSize(); err != nil {
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
func compareLOBContent(ctx context.Context, name string, value datatype.LOB, expected io.Reader) error {
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
