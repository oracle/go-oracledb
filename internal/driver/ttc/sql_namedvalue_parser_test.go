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
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// helper to extract OracleError code
func getErrCode(t *testing.T, err error) oracleErrors.ErrorCode {
	t.Helper()
	oe, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected *common.OracleError, got %T: %v", err, err)
	}
	return oracleErrors.ErrorCode(oe.ErrorCode())
}

func TestParseAndOrder_Positional_Success_NoDup(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:1 and b=:2"
	args := []driver.NamedValue{
		{Ordinal: 1, Value: 10},
		{Ordinal: 2, Value: "x"},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := extractInputBindValues(ph, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 out values, got %d", len(out))
	}
	if !(out[0] == int(10) || out[0] == int64(10)) {
		t.Fatalf("unexpected first value: %v", out[0])
	}
	if out[1] != "x" {
		t.Fatalf("unexpected second value: %v", out[1])
	}
}

func TestParseAndOrder_Named_Success_WithDupRef(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:id or b=:id and c=:other"
	args := []driver.NamedValue{
		{Name: "id", Value: 10},
		{Name: "other", Value: "x"},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := extractInputBindValues(ph, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 out values, got %d", len(out))
	}
	if !(out[0] == 10 || out[0] == int64(10)) || !(out[1] == 10 || out[1] == int64(10)) || out[2] != "x" {
		t.Fatalf("unexpected ordered values: %v", out)
	}
}

func TestParseAndOrder_Named_Error_EmptyNameWithoutValidOrdinal(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:x"
	args := []driver.NamedValue{
		{Name: "", Ordinal: 0, Value: 1},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = extractInputBindValues(ph, args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingInvalidOrdinal {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestParseAndOrder_Named_Error_MissingName(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:a"
	args := []driver.NamedValue{
		{Name: "b", Value: 1},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = extractInputBindValues(ph, args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingNameNotFound {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestParseAndOrder_Named_Error_ExtraName(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:a"
	args := []driver.NamedValue{
		{Name: "a", Value: 1},
		{Name: "b", Value: 2},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = extractInputBindValues(ph, args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingNameNotFound {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestParsePlaceholders_Error_DanglingColon(t *testing.T) {
	t.Parallel()
	_, err := parsePlaceholders("select :")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingDanglingColon {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestParsePlaceholders_MixedIdentifiers_Supported(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:1 and b=:name and c=:2 and d=:name"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 4 {
		t.Fatalf("expected 4 placeholders, got %d", len(ph.bindNames))
	}
	if ph.bindNames[0] != "1" || ph.bindNames[1] != "name" || ph.bindNames[2] != "2" || ph.bindNames[3] != "name" {
		t.Fatalf("unexpected placeholders: %v", ph.bindNames)
	}
}

func TestParsePlaceholders_IgnoreInLineComment(t *testing.T) {
	t.Parallel()
	q := "SELECT * FROM emp -- this is an inline comment\n WHERE ename =:1"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestParsePlaceholders_IgnoreInBlockComment(t *testing.T) {
	t.Parallel()
	q := "select /* :1 ignored */ * from t where a=:1"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestParsePlaceholders_IgnoreInSingleQuotedString(t *testing.T) {
	t.Parallel()
	q := "select ':1' as c from dual where a=:1"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestParsePlaceholders_IgnoreInSingleQuotedStringWithEscape(t *testing.T) {
	t.Parallel()
	q := "select 'it''s :1' as c from dual where a=:1"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestParsePlaceholders_IgnoreInDoubleQuotedIdentifier(t *testing.T) {
	t.Parallel()
	q := `select "col:1" from t where a=:1`
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestParsePlaceholders_ParseAfterCommentClosed(t *testing.T) {
	t.Parallel()
	q := "select /* comment :1 */ * from t where a=:1"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestParsePlaceholders_SkipNonBindColonAssignment(t *testing.T) {
	t.Parallel()
	// Ensure a colon that is not followed by a digit or letter (e.g., ':=') is ignored,
	// and subsequent valid placeholders are still parsed.
	q := "select * from t where a := b and c = :1"
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ph.bindNames) != 1 || ph.bindNames[0] != "1" {
		t.Fatalf("expected one placeholder '1', got %v", ph.bindNames)
	}
}

func TestOrderNamedValues_NamePrecedenceOverOrdinal(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:p and b=:1 and c=:p"
	args := []driver.NamedValue{
		{Name: "p", Ordinal: 2, Value: 10}, // name precedence
		{Ordinal: 2, Value: 111},           // sets the second occurrence (index 2)
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := extractInputBindValues(ph, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 placeholders, got len(out)=%d", len(out))
	}
	// Expect name 'p' to be used for positions 1 and 3, ordinal 2 for middle
	if !(out[0] == 10 || out[0] == int64(10)) || out[1] != 111 || !(out[2] == 10 || out[2] == int64(10)) {
		t.Fatalf("unexpected values: %v, %v, %v", out[0], out[1], out[2])
	}
}

func TestOrderNamedValues_RepeatName_FromName(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:p or b=:p"
	args := []driver.NamedValue{
		{Name: "p", Value: 7}, // name provided; should apply to all occurrences of :p
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := extractInputBindValues(ph, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 placeholders, got len(out)=%d", len(out))
	}
	if out[0] != 7 || out[1] != 7 {
		t.Fatalf("expected values 7 duplicated, got %v,%v", out[0], out[1])
	}
}

func TestOrderNamedValues_Error_DuplicateOrdinal(t *testing.T) {
	t.Parallel()
	q := "select * from t where a=:1 and b=:2"
	args := []driver.NamedValue{
		{Ordinal: 1, Value: 10},
		{Ordinal: 1, Value: 20}, // duplicate ordinal should error
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = extractInputBindValues(ph, args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingDuplicateArg {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestPlSqlNamedValues_Error_MissingName(t *testing.T) {
	t.Parallel()
	q := "begin p(:first, :second); end;"
	args := []driver.NamedValue{
		{Name: "missing", Value: 1},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = extractInputBindValuesForPlSql(ph, args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingNameNotFound {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestPlSqlNamedValues_Error_DuplicateName(t *testing.T) {
	t.Parallel()
	q := "begin p(:first, :second); end;"
	args := []driver.NamedValue{
		{Name: "first", Value: 1},
		{Name: "first", Value: 2},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = extractInputBindValuesForPlSql(ph, args)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if getErrCode(t, err) != oracleErrors.StatementParsingDuplicateArg {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestPlSqlNamedValues_LaterNamedBindWithoutEarlierBind(t *testing.T) {
	t.Parallel()
	q := "begin p(:first, :second); end;"
	args := []driver.NamedValue{
		{Name: "second", Value: 2},
	}
	ph, err := parsePlaceholders(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := extractInputBindValuesForPlSql(ph, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0] != 2 {
		t.Fatalf("unexpected values: %v", out)
	}
}

func TestParsePlaceholders_Uint16IndexesDoNotWrapAfter255(t *testing.T) {
	t.Parallel()
	ph, err := parsePlaceholders(bindListStatement(257))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := ph.bindMap["p256"][0]; got != 256 {
		t.Fatalf("expected p256 occurrence index 256, got %d", got)
	}
	if got := ph.uniqueNames["p256"]; got != 256 {
		t.Fatalf("expected p256 unique-name index 256, got %d", got)
	}
}

func bindListStatement(n int) string {
	var b strings.Builder
	b.WriteString("begin p(")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, ":p%d", i)
	}
	b.WriteString("); end;")
	return b.String()
}
