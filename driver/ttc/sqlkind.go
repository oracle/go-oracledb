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
	"strings"

	"github.com/oracle/go-driver/driver/common"
)

/*
sqlKind represents the broad class of a SQL statement detected by lightweight
lexical inspection. It is unexported to keep classification details internal
to the ttc package, and is used to select the appropriate execution path
(e.g., query vs exec; PL/SQL vs DML vs others).

This is not a full SQL parser; it relies on leading tokens and is intended only
for driver routing decisions.
*/
type sqlKind int

const (
	select_ sqlKind = iota // SELECT statements (read-only queries)
	dml                    // DML statements: INSERT/UPDATE/DELETE/MERGE
	plsql                  // PL/SQL blocks and CALL statements
	other                  // All other statements (e.g., DDL, ALTER SESSION, TRUNCATE, COMMIT)
)

// String returns the canonical, upper-case name of the sqlKind (e.g., "SELECT").
func (k sqlKind) String() string {
	switch k {
	case select_:
		return "SELECT"
	case dml:
		return "DML"
	case plsql:
		return "PLSQL"
	default:
		return "OTHER"
	}
}

// type to host details about a query
type qualifiedSQLStatement struct {
	query    common.B1Array // the original SQL query
	kind     sqlKind        // the kind of the SQL query
	binds    *bindDetails   // the binds of the SQL query
	cursorId common.SB4     // id of the cursorId while this query is executed
}

// newQualifiedSQLStatement creates a new qualifiedSQLStatement
// the SQL query is parsed to get the type of execution then the placeholders (if any)
// are parsed to build bind details
func newQualifiedSQLStatement(query string) (*qualifiedSQLStatement, error) {
	k, err := _classifySQL(query)
	if err != nil {
		common.Odl.Debug("Can't clasify SQL", "error", err)
		return nil, err
	}
	b, err := parsePlaceholders(query)
	if err != nil {
		common.Odl.Debug("Can't parse placeholders", "error", err)
		return nil, err
	}
	return &qualifiedSQLStatement{query: common.StringToB1Array(query), kind: k, binds: b}, nil
}

/*
classifySQL inspects the leading token(s) of q and returns its coarse sqlKind.

Input:
- q: Raw SQL text. Leading ASCII whitespace is ignored. Case-insensitive.

Output:
- (sqlKind, nil) for recognized kinds:
  - "select"                        -> select_
  - "insert|update|delete|merge"    -> dml
  - "call|begin|declare"            -> plsql
  - anything else                   -> other (e.g., DDL/TRUNCATE/COMMIT/ALTER SESSION)

Error:
- Returns (0, OracleError) with code OGD-00050 (StatementExecutionFailed) when:
  - q is empty/whitespace (cause: "empty sql"), or
  - the leading token is not recognized (cause includes the token).

Notes:
- This is a lightweight lexical classifier to route driver execution paths.
- It is not a full SQL parser and does not validate statement correctness.
*/
func _classifySQL(q string) (sqlKind, error) {
	// Fast-path: trim leading ASCII whitespace without allocating a whole-lowercased copy
	s := strings.TrimLeft(q, " \t\r\n")
	if s == "" {
		common.Odl.Error("classifySQL: empty SQL", "error", "empty sql")
		return 0, common.NewOracleError(common.StatementExecutionFailed, nil, "query", "empty SQL")
	}
	// Extract the first token up to the first whitespace or '('
	end := 0
	for end < len(s) {
		c := s[end]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '(' {
			break
		}
		end++
	}
	stmt := strings.ToLower(s[:end])
	k, ok := _sqlKindMap[stmt]
	if !ok {
		common.Odl.Error("classifySQL: unsupported SQL leading token", "token", stmt)
		return 0, common.NewOracleError(common.StatementExecutionFailed, nil, "qualifiedSQLStatement", "unrecognised query")
	}
	return k, nil

}
