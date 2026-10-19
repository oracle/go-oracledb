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

	"github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// runTypedLOBTest demonstrates the connection-scoped typed API: temporary
// LOB creation and mutation, followed by typed scans that remain usable after
// the producing Rows have closed.
func runTypedLOBTest(ctx context.Context, db *sql.DB) error {
	// Reserve one dedicated connection. Typed query and temporary LOB values
	// remain tied to this *sql.Conn.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open dedicated LOB connection: %w", err)
	}
	defer conn.Close()

	// Exercise temporary BLOB and CLOB creation, mutation, and cleanup first.
	if err := runTemporaryTypedLOBs(ctx, conn); err != nil {
		return err
	}
	// Then scan persistent LOBs and use the typed values after Rows closes.
	if err := runPersistentTypedLOBs(ctx, conn); err != nil {
		return err
	}

	fmt.Println("typed LOB API: temporary and persistent usage: ok")
	return nil
}

// runTemporaryTypedLOBs tests temporary BLOB and CLOB handles.
// It covers writing, metadata, open state, trimming, and freeing temporary
// LOB storage.
func runTemporaryTypedLOBs(ctx context.Context, conn *sql.Conn) error {
	for _, test := range []struct {
		name    string
		kind    datatype.Kind
		payload []byte
	}{
		{name: "BLOB", kind: datatype.BLOB, payload: []byte("temporary BLOB payload")},
		{name: "CLOB", kind: datatype.CLOB, payload: []byte("temporary CLOB payload")},
	} {
		// Create a typed temporary LOB of the requested kind.
		var value datatype.LOB
		var err error
		if test.kind == datatype.BLOB {
			value, err = datatype.CreateTemporaryBlob(ctx, conn)
		} else {
			value, err = datatype.CreateTemporaryClob(ctx, conn)
		}
		if err != nil {
			return fmt.Errorf("create temporary %s: %w", test.name, err)
		}
		// Keep a fallback cleanup until the explicit Free below succeeds.
		freed := false
		defer func() {
			if !freed {
				_ = value.FreeContext(ctx)
			}
		}()

		// Check the new handle's declared kind and temporary status.
		if value.Kind() != test.kind || !value.IsTemporary() {
			return fmt.Errorf("temporary %s metadata is incorrect", test.name)
		}

		// Write the payload. Use the context-free convenience method for BLOB and
		// the context-aware string method for CLOB to show both public forms.
		var written int
		if test.kind == datatype.BLOB {
			written, err = value.Write(test.payload)
		} else {
			clob, ok := value.(*datatype.Clob)
			if !ok {
				return fmt.Errorf("temporary %s has type %T, want *datatype.Clob", test.name, value)
			}
			written, err = clob.WriteStringContext(ctx, string(test.payload))
		}
		if err != nil || written != len(test.payload) {
			return fmt.Errorf("write temporary %s = (%d, %v), want (%d, nil)", test.name, written, err, len(test.payload))
		}

		// Size reports logical length; ChunkSize reports Oracle's storage chunk
		// size. Neither operation changes the sequential cursor.
		wantSize := int64(len(test.payload))
		if size, err := value.SizeContext(ctx); err != nil || size != wantSize {
			return fmt.Errorf("temporary %s size = (%d, %v), want (%d, nil)", test.name, size, err, wantSize)
		}
		if chunkSize, err := value.ChunkSizeContext(ctx); err != nil || chunkSize <= 0 {
			return fmt.Errorf("temporary %s chunk size = (%d, %v), want positive size", test.name, chunkSize, err)
		}

		// Open the temporary LOB for read/write access and verify the open state.
		if opened, err := value.OpenContext(ctx, datatype.ReadWrite); err != nil || !opened {
			return fmt.Errorf("open temporary %s = (%t, %v), want (true, nil)", test.name, opened, err)
		}
		if open, err := value.IsOpenContext(ctx); err != nil || !open {
			return fmt.Errorf("temporary %s IsOpen = (%t, %v), want (true, nil)", test.name, open, err)
		}

		// CloseServer ends the explicit server-side open while leaving the Go
		// handle usable for later operations.
		if err := value.CloseServerContext(ctx); err != nil {
			return fmt.Errorf("close temporary %s on server: %w", test.name, err)
		}
		if open, err := value.IsOpenContext(ctx); err != nil || open {
			return fmt.Errorf("temporary %s IsOpen after CloseServer = (%t, %v), want (false, nil)", test.name, open, err)
		}

		// Trim one logical unit and confirm the resulting logical length.
		if err := value.TrimContext(ctx, wantSize-1); err != nil {
			return fmt.Errorf("trim temporary %s: %w", test.name, err)
		}
		if size, err := value.SizeContext(ctx); err != nil || size != wantSize-1 {
			return fmt.Errorf("temporary %s size after Trim = (%d, %v), want (%d, nil)", test.name, size, err, wantSize-1)
		}

		// Free is the terminal cleanup operation for a temporary LOB. It releases
		// both local handle state and Oracle temporary storage before conn closes.
		if err := value.FreeContext(ctx); err != nil {
			return fmt.Errorf("free temporary %s: %w", test.name, err)
		}
		freed = true
	}
	return nil
}

// runPersistentTypedLOBs tests persistent BLOB and CLOB handles. It queries
// one row, scans both query LOBs into connection-scoped values, closes the
// source Rows, reads the values, and cleans them up.
func runPersistentTypedLOBs(ctx context.Context, conn *sql.Conn) error {
	// Create a persistent table and insert one BLOB and one CLOB.
	table, err := createTableConn(ctx, conn, "id NUMBER PRIMARY KEY, blob_value BLOB, clob_value CLOB")
	if err != nil {
		return err
	}
	defer cleanupTableConn(conn, table)

	blobPayload := []byte("persistent BLOB payload")
	clobPayload := "persistent CLOB payload 🙂"
	insert := fmt.Sprintf("INSERT INTO %s (id, blob_value, clob_value) VALUES (:1, :2, :3)", table)
	// BindBlob and BindClob select the matching Oracle LOB bind type.
	if _, err := conn.ExecContext(ctx, insert, int64(1), oracle.BindBlob(blobPayload), oracle.BindClob(clobPayload)); err != nil {
		return fmt.Errorf("insert persistent LOBs: %w", err)
	}

	// Typed zero values are scan targets. The query's row locator adopts the
	// physical connection lease during Scan.
	var blobValue datatype.Blob
	defer blobValue.FreeContext(ctx)
	var clobValue datatype.Clob
	defer clobValue.FreeContext(ctx)

	// Query and scan the persistent values; Scan adopts their locators.
	query := fmt.Sprintf("SELECT blob_value, clob_value FROM %s WHERE id = :1", table)
	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		return fmt.Errorf("query persistent LOBs: %w", err)
	}
	if !rows.Next() {
		_ = rows.Close()
		return fmt.Errorf("query persistent LOBs returned no row: %v", rows.Err())
	}
	if err := rows.Scan(&blobValue, &clobValue); err != nil {
		_ = rows.Close()
		return fmt.Errorf("scan persistent LOBs: %w", err)
	}
	// Rows.Close releases the query result. Because this query used a dedicated
	// *sql.Conn, its physical session stays checked out and the adopted typed
	// values remain usable until conn.Close.
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close source LOB rows: %w", err)
	}
	persistentValues := []datatype.LOB{&blobValue, &clobValue}

	values := []struct {
		name      string
		kind      datatype.Kind
		value     datatype.LOB
		wantBytes []byte
		wantText  string
		readTo    bool
	}{
		{name: "BLOB", kind: datatype.BLOB, value: &blobValue, wantBytes: blobPayload},
		{name: "CLOB", kind: datatype.CLOB, value: &clobValue, wantText: clobPayload, readTo: true},
	}

	// Check each typed value's kind, lifetime, logical size, and content while
	// the owning connection remains open.
	for _, test := range values {
		// Kind and IsTemporary confirm that the query value became the expected
		// persistent LOB family rather than a temporary handle.
		if test.value.Kind() == datatype.Unknown || test.value.IsTemporary() {
			return fmt.Errorf("persistent %s metadata is incorrect", test.name)
		}
		wantSize := int64(len(test.wantBytes))
		if test.kind != datatype.BLOB {
			wantSize = logicalTextSize(test.wantText)
		}
		// Size uses bytes for BLOBs and UTF-16 code units for CLOBs.
		if size, err := test.value.SizeContext(ctx); err != nil || size != wantSize {
			return fmt.Errorf("persistent %s size = (%d, %v), want (%d, nil)", test.name, size, err, wantSize)
		}

		var got []byte
		if test.readTo {
			// WriteToContext reads the remaining CLOB into this in-memory writer
			// while honoring the scenario context.
			var output bytes.Buffer
			if _, err := test.value.WriteToContext(ctx, &output); err != nil {
				return fmt.Errorf("read persistent %s with WriteTo: %w", test.name, err)
			}
			got = output.Bytes()
		} else {
			var err error
			got, err = readTypedLOB(ctx, test.value)
			if err != nil {
				return fmt.Errorf("read persistent %s: %w", test.name, err)
			}
		}
		if test.kind == datatype.BLOB {
			if !bytes.Equal(got, test.wantBytes) {
				return fmt.Errorf("persistent %s value mismatch: got %d bytes, want %d", test.name, len(got), len(test.wantBytes))
			}
		} else if string(got) != test.wantText {
			return fmt.Errorf("persistent %s value mismatch: got %d UTF-8 bytes, want %d", test.name, len(got), len(test.wantText))
		}
	}

	// Free releases each local persistent handle without deleting its column.
	for _, value := range persistentValues {
		if err := value.Close(); err != nil {
			return fmt.Errorf("close persistent LOB: %w", err)
		}
		if err := value.FreeContext(ctx); err != nil {
			return fmt.Errorf("free persistent LOB: %w", err)
		}
	}
	return nil
}

// readTypedLOB reads a typed LOB to completion with a small buffer and honors
// the caller's context for every database read.
func readTypedLOB(ctx context.Context, value datatype.LOB) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, 7)
	for {
		count, err := value.ReadContext(ctx, buffer)
		if count != 0 {
			_, _ = output.Write(buffer[:count])
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, io.ErrNoProgress
		}
	}
}
