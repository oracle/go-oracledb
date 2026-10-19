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
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_TypedLOBValidation verifies typed temporary LOB argument
// validation using stable error codes.
func TestDriver_TypedLOBValidation(t *testing.T) {
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

	if _, err := datatype.CreateTemporaryBlob(ctx, nil); err == nil {
		t.Fatal("CreateTemporaryBlob accepted a nil connection")
	} else {
		requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error { return err })
	}

	for _, test := range []struct {
		name string
		kind datatype.Kind
	}{
		{name: "BLOB", kind: datatype.BLOB},
		{name: "CLOB", kind: datatype.CLOB},
		{name: "NCLOB", kind: datatype.NCLOB},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := newTemporaryTypedLOB(ctx, conn, test.kind)
			if err != nil {
				t.Fatalf("create temporary %s: %v", test.name, err)
			}
			t.Cleanup(func() { _ = value.FreeContext(ctx) })

			requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
				_, err := value.OpenContext(ctx, datatype.OpenMode(99))
				return err
			})
			requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
				return value.TrimContext(ctx, -1)
			})
			requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
				_, err := value.WriteTo(nil)
				return err
			})
			if test.kind != datatype.BLOB {
				requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
					_, err := value.WriteContext(ctx, []byte{0xff})
					return err
				})
			}
		})
	}
}
