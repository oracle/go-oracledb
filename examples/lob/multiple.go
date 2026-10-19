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

	"github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

// runMultipleLOBTest scans six query LOBs into independent typed values on one
// dedicated connection. It closes Rows before reading to verify every locator
// remains usable and keeps an independent cursor after adoption.
func runMultipleLOBTest(ctx context.Context, db *sql.DB, prefetchSize int) error {
	// Step 1: Create one table containing two columns from each LOB family.
	table, err := createTable(ctx, db, "id NUMBER PRIMARY KEY, blob_one BLOB, blob_two BLOB, clob_one CLOB, clob_two CLOB, nclob_one NCLOB, nclob_two NCLOB")
	if err != nil {
		return err
	}
	defer cleanupTable(db, table)

	// Step 2: Prepare different values that cross the configured prefetch
	// boundary. Different values make it possible to detect column mix-ups.
	blobOne := binaryPayload(prefetchSize + boundaryPayloadExtra64KiB)
	blobTwo := binaryPayload(prefetchSize + boundaryPayloadExtra64KiB + 17)
	clobOne := textPayload(prefetchSize + boundaryPayloadExtra64KiB)
	clobTwo := "second CLOB: " + textPayload(prefetchSize+boundaryPayloadExtra64KiB)
	nclobOne := "first NCLOB: " + textPayload(prefetchSize+boundaryPayloadExtra64KiB)
	nclobTwo := "second NCLOB: " + textPayload(prefetchSize+boundaryPayloadExtra64KiB+19)

	// Step 3: Insert all six values using the public LOB bind markers.
	insert := fmt.Sprintf("INSERT INTO %s (id, blob_one, blob_two, clob_one, clob_two, nclob_one, nclob_two) VALUES (:1, :2, :3, :4, :5, :6, :7)", table)
	if _, err := db.ExecContext(ctx, insert,
		int64(1),
		oracle.BindBlob(blobOne), oracle.BindBlob(blobTwo),
		oracle.BindClob(clobOne), oracle.BindClob(clobTwo),
		oracle.BindNClob(nclobOne), oracle.BindNClob(nclobTwo),
	); err != nil {
		return fmt.Errorf("insert multiple LOBs: %w", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get dedicated LOB connection: %w", err)
	}
	defer conn.Close()

	kinds := []datatype.Kind{datatype.BLOB, datatype.BLOB, datatype.CLOB, datatype.CLOB, datatype.NCLOB, datatype.NCLOB}
	values := make([]datatype.LOB, len(kinds))
	destinations := make([]any, len(kinds))
	for index, kind := range kinds {
		value, err := newExampleLOBScanner(kind)
		if err != nil {
			return fmt.Errorf("create typed scanner %d: %w", index+1, err)
		}
		values[index] = value
		destinations[index] = value
		defer func(value datatype.LOB) { _ = value.FreeContext(ctx) }(value)
	}

	// Step 4: Query through the dedicated connection and close Rows after Scan.
	query := fmt.Sprintf("SELECT blob_one, blob_two, clob_one, clob_two, nclob_one, nclob_two FROM %s WHERE id = :1", table)
	rows, err := conn.QueryContext(ctx, query, int64(1))
	if err != nil {
		return fmt.Errorf("query multiple LOBs: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate multiple LOBs: %w", err)
		}
		return fmt.Errorf("query multiple LOBs returned no row")
	}

	if err := rows.Scan(destinations...); err != nil {
		return fmt.Errorf("scan multiple LOBs: %w", err)
	}
	if rows.Next() {
		return fmt.Errorf("query multiple LOBs returned more than one row")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate multiple LOBs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close multiple LOB rows: %w", err)
	}

	checks := []struct {
		name      string
		value     datatype.LOB
		kind      datatype.Kind
		wantBytes []byte
	}{
		{name: "BLOB one", value: values[0], kind: datatype.BLOB, wantBytes: blobOne},
		{name: "BLOB two", value: values[1], kind: datatype.BLOB, wantBytes: blobTwo},
		{name: "CLOB one", value: values[2], kind: datatype.CLOB, wantBytes: []byte(clobOne)},
		{name: "CLOB two", value: values[3], kind: datatype.CLOB, wantBytes: []byte(clobTwo)},
		{name: "NCLOB one", value: values[4], kind: datatype.NCLOB, wantBytes: []byte(nclobOne)},
		{name: "NCLOB two", value: values[5], kind: datatype.NCLOB, wantBytes: []byte(nclobTwo)},
	}

	// Step 5: Read and compare each value after Rows.Close. This also verifies
	// that every locator has an independent cursor and buffer.
	for _, check := range checks {
		if check.value.Kind() != check.kind || !check.value.Valid() {
			return fmt.Errorf("%s typed LOB metadata is incorrect", check.name)
		}
		wantSize := int64(len(check.wantBytes))
		if check.kind != datatype.BLOB {
			wantSize = logicalTextSize(string(check.wantBytes))
		}
		if gotSize, err := check.value.SizeContext(ctx); err != nil || gotSize != wantSize {
			return fmt.Errorf("%s size = (%d, %v), want (%d, nil)", check.name, gotSize, err, wantSize)
		}
		var output bytes.Buffer
		if _, err := check.value.WriteToContext(ctx, &output); err != nil {
			return fmt.Errorf("read %s after Rows.Close: %w", check.name, err)
		}
		if !bytes.Equal(output.Bytes(), check.wantBytes) {
			return fmt.Errorf("%s value mismatch: got %d bytes, want %d", check.name, output.Len(), len(check.wantBytes))
		}
	}

	fmt.Println("six typed Blob/Clob/NClob scanners: ok")
	return nil
}

// newExampleLOBScanner returns a zero-value typed destination for the given
// LOB kind; Scan adopts the locator from the query's dedicated connection.
func newExampleLOBScanner(kind datatype.Kind) (datatype.LOB, error) {
	switch kind {
	case datatype.BLOB:
		return &datatype.Blob{}, nil
	case datatype.CLOB:
		return &datatype.Clob{}, nil
	case datatype.NCLOB:
		return &datatype.NClob{}, nil
	default:
		return nil, fmt.Errorf("unsupported LOB kind %v", kind)
	}
}
