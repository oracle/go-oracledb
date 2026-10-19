/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and under any and all patent rights owned or freely
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
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

func newTemporaryTypedLOB(ctx context.Context, conn *sql.Conn, kind datatype.Kind) (datatype.LOB, error) {
	switch kind {
	case datatype.BLOB:
		return datatype.CreateTemporaryBlob(ctx, conn)
	case datatype.CLOB:
		return datatype.CreateTemporaryClob(ctx, conn)
	case datatype.NCLOB:
		return datatype.CreateTemporaryNClob(ctx, conn)
	default:
		return nil, fmt.Errorf("unsupported LOB kind %v", kind)
	}
}

func newTypedLOBTarget(kind datatype.Kind) (datatype.LOB, error) {
	// Return actual zero-value typed destinations; the scanned row source
	// supplies the owning physical session.
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

// registerLOBTableCleanup drops a functional-test table with its own bounded
// context. A test's operation context may already be canceled when cleanups
// run, and cleanup failures should be visible rather than silently ignored.
func registerLOBTableCleanup(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := dropTable(ctx, db, table); err != nil {
			t.Errorf("drop table %s: %v", table, err)
		}
	})
}

// readTypedLOBValue drains one connection-scoped typed LOB using small
// buffers so CLOB and NCLOB conversion carry is exercised across Read calls.
func readTypedLOBValue(t *testing.T, ctx context.Context, value datatype.LOB) []byte {
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
			t.Fatalf("read typed LOB: %v", err)
		}
	}
}
