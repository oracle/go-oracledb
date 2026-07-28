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
	"github.com/oracle/go-driver/driver/common"
)

// getQueryStatementExecutor returns a concrete executor that implements
// common.QueryWithContext for SELECT-like statements.
//
// Behavior:
// - Constructs *statementExecutorSelect for SELECT (noPLSQLMode) and result fetching.
// - For Other kinds *statementExecutorOperationNotSupported is returned.
//
// Usage:
//   - The returned executor implements ShelfUser; callers must inject the Shelf using:
//     if su, ok := exec.(ShelfUser); ok { su.SetShelf(shelf) }
//
// Returns:
// - *statementExecutorSelect which exposes QueryContext(ctx, query, args).
//
// Notes:
// - Lives in the ttc package to avoid cycles and colocate TTC-specific execution logic.
// - The returned executor keeps only a pointer to shelf (once injected) and is intended to be short‑lived.
func getQueryStatementExecutorFor(qQuery *qualifiedSQLStatement) QueryWithContext {

	switch qQuery.kind {
	case select_:
		common.Odl.Debug("getQueryStatementExecutor: requested query-executor.")
		return newStatementExecutorSelect()
	default:
		return &statementExecutorOperationNotSupported{kind: qQuery.kind}
	}
}

// getExecStatementExecutor returns a concrete executor that implements
// common.ExecWithContext for non-SELECT statements.
//
// Routing (based on classifySQL(qualifiedSQLStatement)):
// - SELECT -> statementExecutorOperationNotSupported
// - INSERT/UPDATE/DELETE/MERGE -> statementExecutorDML
// - BEGIN/DECLARE/CALL (PL/SQL) -> statementExecutorPlSql
// - Everything else (DDL/ALTER/TRUNCATE/etc.) -> statementExecutorOthers
//
// Usage:
//   - The returned executor implements ShelfUser; callers must inject the Shelf using:
//     if su, ok := exec.(ShelfUser); ok { su.SetShelf(shelf) }
//
// Parameters:
// - qQuery: Raw SQL text used for routing to the appropriate executor.
//
// Returns:
// - A concrete executor exposing ExecContext(ctx, qQuery, args) for the chosen kind.
//
// Concurrency/lifecycle:
// - Executors do not cache per-statement state. Creating a new executor per call is cheap and recommended.
func getExecStatementExecutorFor(qQuery *qualifiedSQLStatement) ExecWithContext {

	switch qQuery.kind {
	case select_:
		return &statementExecutorOperationNotSupported{kind: qQuery.kind}
	case dml:
		common.Odl.Debug("getExecStatementExecutor: requested dml-executor.")
		return newStatementExecutorDML()
	case plsql:
		common.Odl.Debug("getExecStatementExecutor: requested plsql-executor.")
		return newStatementExecutorPlSql()
	default:
		common.Odl.Debug("getExecStatementExecutor: requested other-executor.")
		return newStatementExecutorOthers()
	}
}
