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

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/lob"
)

// TestDriver_LobDirectValidation verifies public argument validation for
// temporary direct LOBs. It intentionally checks stable error codes rather
// than driver error strings.
func TestDriver_LobDirectValidation(t *testing.T) {
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

	requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
		_, err := lob.CreateTemporary(ctx, nil, lob.BLOB)
		return err
	})
	requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
		_, err := lob.CreateTemporary(ctx, conn, lob.Unknown)
		return err
	})

	for _, test := range []struct {
		name string
		kind lob.Kind
	}{
		{name: "BLOB", kind: lob.BLOB},
		{name: "CLOB", kind: lob.CLOB},
		{name: "NCLOB", kind: lob.NCLOB},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := lob.CreateTemporary(ctx, conn, test.kind)
			if err != nil {
				t.Fatalf("create temporary %s: %v", test.name, err)
			}
			t.Cleanup(func() { _ = value.Free(ctx) })

			requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
				_, err := value.Open(ctx, lob.OpenMode(99))
				return err
			})
			requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
				return value.Trim(ctx, -1)
			})
			requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
				_, err := value.WriteTo(nil)
				return err
			})
			if test.kind != lob.BLOB {
				requireLOBErrorCode(t, oracleErrors.InvalidLOBBuffer, func() error {
					_, err := value.Write([]byte{0xff})
					return err
				})
			}
		})
	}
}
