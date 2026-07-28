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

package common

import (
	"strings"
	"testing"
)

func TestEnquoteLiteral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "Hello", want: "'Hello'"},
		{name: "single quote", in: "G'Day", want: "'G''Day'"},
		{name: "already quoted", in: "'G''Day'", want: "'''G''''Day'''"},
		{name: "many quotes", in: "I'''M", want: "'I''''''M'"},
		{name: "empty", in: "", want: "''"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EnquoteLiteral(tc.in)
			if got != tc.want {
				t.Fatalf("unexpected value: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestEnquoteNCharLiteral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "Hello", want: "N'Hello'"},
		{name: "single quote", in: "G'Day", want: "N'G''Day'"},
		{name: "already quoted", in: "'G''Day'", want: "N'''G''''Day'''"},
		{name: "many quotes", in: "I'''M", want: "N'I''''''M'"},
		{name: "already N prefixed text", in: "N'Hello'", want: "N'N''Hello'''"},
		{name: "empty", in: "", want: "N''"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EnquoteNCharLiteral(tc.in)
			if got != tc.want {
				t.Fatalf("unexpected value: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIsSimpleIdentifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "one char", in: "A", want: true},
		{name: "simple", in: "Hello", want: true},
		{name: "with underscore and digits", in: "A_12", want: true},
		{name: "starts with digit", in: "1abc", want: false},
		{name: "single quote", in: "G'Day", want: false},
		{name: "double quoted", in: `"Bruce Wayne"`, want: false},
		{name: "dollar sign", in: "GoodDay$", want: false},
		{name: "contains double quote", in: `Hello"World`, want: false},
		{name: "max length valid", in: "A" + strings.Repeat("b", 127), want: true},
		{name: "too long", in: "A" + strings.Repeat("b", 128), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSimpleIdentifier(tc.in)
			if got != tc.want {
				t.Fatalf("unexpected value: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEnquoteIdentifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty", in: "", wantErr: true},
		{name: "simple always quoted", in: "Hello", want: `"Hello"`, wantErr: false},
		{name: "not simple quoted", in: "G'Day", want: `"G'Day"`, wantErr: false},
		{name: "already quoted kept quoted", in: `"Bruce Wayne"`, want: `"Bruce Wayne"`, wantErr: false},
		{name: "special char quoted", in: "GoodDay$", want: `"GoodDay$"`, wantErr: false},
		{name: "contains double quote", in: `Hello"World`, wantErr: true},
		{name: "quoted with inner double quote", in: `"Hello"World"`, wantErr: true},
		{name: "contains null char", in: "ab\x00cd", wantErr: true},
		{name: "too long", in: "A" + strings.Repeat("b", 128), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EnquoteIdentifier(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				serr, ok := err.(SQLError)
				if !ok {
					t.Fatalf("expected SQLError, got %T: %v", err, err)
				}
				if serr.ErrorCode() != string(InvalidIdentifier) {
					t.Fatalf("unexpected error code: got %s want %s", serr.ErrorCode(), InvalidIdentifier)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected value: got %q want %q", got, tc.want)
			}
		})
	}
}
