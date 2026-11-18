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
	"database/sql"
	"errors"
	"testing"
)

// TestDriver_Functional_ConnectionWrapper verifies that a dedicated
// database/sql connection opened by go-oracledb can be wrapped for Oracle
// specific operations. The wrapper must retain that connection, and wrapping it
// after Close must return sql.ErrConnDone and no wrapper.
func TestDriver_Functional_ConnectionWrapper(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer db.Close()

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("failed to acquire dedicated connection: %v", err)
	}
	defer conn.Close()
	wrapper, err := NewConnectionWrapper(conn)
	if err != nil {
		t.Fatalf("NewConnectionWrapper failed: %v", err)
	}
	if wrapper == nil || wrapper.connection != conn {
		t.Fatal("NewConnectionWrapper did not wrap the supplied Oracle connection")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("closing dedicated connection failed: %v", err)
	}

	if closedWrapper, err := NewConnectionWrapper(conn); !errors.Is(err, sql.ErrConnDone) || closedWrapper != nil {
		t.Fatalf("NewConnectionWrapper on a closed connection returned (%v, %v), want nil wrapper and sql.ErrConnDone", closedWrapper, err)
	}
}
