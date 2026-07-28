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

package ttc

import (
	"testing"
)

// verifies the factory returns a non-nil SELECT executor for query use.
// Expectation: returned value is of type *statementExecutorSelect.
func TestGetQueryStatementExecutor_ReturnsSelect(t *testing.T) {
	t.Parallel()
	q, _ := newQualifiedSQLStatement("SELECT 1 FROM DUAL")
	exec := getQueryStatementExecutorFor(q)
	if exec == nil {
		t.Fatalf("getQueryStatementExecutor returned nil")
	}
	if _, ok := exec.(*statementExecutorSelect); !ok {
		t.Fatalf("expected *statementExecutorSelect, got %T", exec)
	}
}

// verifies SQL classification maps to the correct executor type (DML/PLSQL/Others).
// Expectation: each provided SQL yields an executor of the expected concrete type.
func TestGetExecStatementExecutor_Mapping(t *testing.T) {
	t.Parallel()
	type tc struct {
		name string
		kind sqlKind
		want any
	}
	cases := []tc{
		{"SELECT -> error", select_, (*statementExecutorOperationNotSupported)(nil)},
		{"INSERT -> DML", dml, (*statementExecutorDML)(nil)},
		{"CALL -> PLSQL", plsql, (*statementExecutorPlSql)(nil)},
		{"COMMIT -> Others", other, (*statementExecutorOthers)(nil)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, _ := newQualifiedSQLStatement(c.name)
			exec := getExecStatementExecutorFor(q)

			if c.kind == select_ {
				switch v := exec.(type) {
				case *statementExecutorOperationNotSupported:
				default:
					t.Fatalf("wrong type expected statementExecutorOperationNotSupported but was : %v", v)
				}
				return
			}

			switch c.want.(type) {
			case *statementExecutorDML:
				if _, ok := exec.(*statementExecutorDML); !ok {
					t.Fatalf("expected *statementExecutorDML, got %T", exec)
				}
			case *statementExecutorOthers:
				if _, ok := exec.(*statementExecutorOthers); !ok {
					t.Fatalf("expected *statementExecutorOthers, got %T", exec)
				}
			case *statementExecutorPlSql:
				if _, ok := exec.(*statementExecutorPlSql); !ok {
					t.Fatalf("expected *statementExecutorPlSql, got %T", exec)
				}
			default:
				t.Fatalf("test has unknown want type")
			}
		})
	}
}

// verifies getQueryStatementExecutor fails for non-SELECT SQL.
// Expectation: returns a non-nil error and nil executor.
func TestGetQueryStatementExecutor_UnsupportedForNonSelect(t *testing.T) {
	t.Parallel()
	q, _ := newQualifiedSQLStatement("CREATE TABLE")
	exec := getQueryStatementExecutorFor(q)
	switch v := exec.(type) {
	case *statementExecutorOperationNotSupported:
	default:
		t.Fatalf("wrong type expected statementExecutorOperationNotSupported but was : %v", v)
	}
}

// verifies getExecStatementExecutor fails when invoked for SELECT SQL.
// Expectation: returns a non-nil error and nil executor.
func TestGetExecStatementExecutor_SelectIsError(t *testing.T) {
	t.Parallel()
	q, _ := newQualifiedSQLStatement("SELECT * FROM DUAL")
	exec := getExecStatementExecutorFor(q)
	switch v := exec.(type) {
	case *statementExecutorOperationNotSupported:
	default:
		t.Fatalf("wrong type expected statementExecutorOperationNotSupported but was : %v", v)
	}
}
