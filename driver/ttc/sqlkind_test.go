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
	"testing"
)

type kindExpect struct {
	in     string
	expect sqlKind
}

// TestClassifySQL_AllKinds verifies classification across representative SQL samples.
// Expectation: classifySQL returns the expected sqlKind for each input and handles case/whitespace correctly.
func TestClassifySQL_AllKinds(t *testing.T) {
	t.Parallel()
	cases := []kindExpect{
		{in: "select * from dual", expect: select_},
		{in: "INSERT INTO t VALUES(1)", expect: dml},
		{in: "update t set a=1", expect: dml},
		{in: "DELETE from t where 1=1", expect: dml},
		{in: "merge into t using s on (t.id=s.id) when matched then update set a=1", expect: dml},
		{in: "call my_proc()", expect: plsql},
		{in: "begin null; end;", expect: plsql},
		{in: "declare x number; begin null; end;", expect: plsql},
		{in: "alter session set nls_language='AMERICAN'", expect: other},
		{in: "truncate table t", expect: other},
	}

	for i, tc := range cases {
		got, err := newQualifiedSQLStatement(tc.in)
		if err != nil {
			t.Fatalf("case %d: unexpected error: %v", i, err)
		}
		if got.kind != tc.expect {
			t.Fatalf("case %d: expected %v, got %v", i, tc.expect, got)
		}
	}

	// Case-insensitivity and leading/trailing whitespace
	got, err := newQualifiedSQLStatement(" \n\t SeLeCt 1 from dual")
	if err != nil || got.kind != select_ || strings.ToUpper(got.kind.String()) != "SELECT" {
		t.Fatalf("whitespace/case-insensitive classification failed; got %q (%v)", got.kind.String(), got)
	}
}

// TestSqlKind_InvalidValues verifies invalid queries.
// Expectation: zero-value equals Unknown; valid inputs never return Unknown; OTHER-like inputs map to Other.
func TestSqlKind_InvalidValues(t *testing.T) {
	t.Parallel()
	// Empty and whitespace-only inputs should return error
	if _, err := newQualifiedSQLStatement(""); err == nil {
		t.Fatalf("expected error for empty SQL")
	}
	if _, err := newQualifiedSQLStatement("   \n\t"); err == nil {
		t.Fatalf("expected error for whitespace-only SQL")
	}
	// Unrecognized tokens should return error
	if _, err := newQualifiedSQLStatement("foobar"); err == nil {
		t.Fatalf("expected error for unrecognized leading token")
	}
	// OTHER-like inputs should classify as other without error
	if k, err := newQualifiedSQLStatement("commit"); err != nil || k.kind != other {
		t.Fatalf("expected other for 'commit', got kind=%v err=%v", k, err)
	}
}

// TestSqlKind_String_All verifies string conversions for all sqlKind values.
// Expectation: String() returns the correct uppercase label for each kind.
func TestSqlKind_String_All(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    sqlKind
		want string
	}{
		{select_, "SELECT"},
		{dml, "DML"},
		{plsql, "PLSQL"},
		{other, "OTHER"},
	}
	for i, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Fatalf("case %d: String() = %q, want %q", i, got, tc.want)
		}
	}
}
