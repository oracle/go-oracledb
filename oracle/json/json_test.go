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

package json

import (
	"bytes"
	"errors"
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestJSONErrorsIncludeCause verifies locally constructed public JSON errors
// retain a concrete explanation of the failed condition.
func TestJSONErrorsIncludeCause(t *testing.T) {
	var value JSON
	var nilValue *JSON

	driverValue, err := (JSONValue{Data: []any{true}}).Value()
	if err != nil {
		t.Fatalf("JSONValue.Value() failed: %v", err)
	}
	var arrayValue JSON
	if err := arrayValue.Scan(driverValue); err != nil {
		t.Fatalf("JSON.Scan() failed: %v", err)
	}
	_, kindMismatchErr := arrayValue.GetJSONObject(JSONOptDefault)
	array, err := arrayValue.GetJSONArray(JSONOptDefault)
	if err != nil {
		t.Fatalf("GetJSONArray() failed: %v", err)
	}
	_, indexErr := array.Get(1)

	tests := []struct {
		name string
		err  error
		code oracleErrors.ErrorCode
	}{
		{name: "nil scan receiver", err: nilValue.Scan(nil), code: oracleErrors.JSONNilReceiver},
		{name: "non-OSON bytes", err: value.Scan([]byte(`{}`)), code: oracleErrors.OsonHeaderError},
		{name: "unsupported scan source", err: value.Scan("{}"), code: oracleErrors.JSONScanTypeUnsupportedError},
		{name: "uninitialized value", err: func() error {
			_, err := value.GetValue(JSONOptDefault)
			return err
		}(), code: oracleErrors.JSONNilReceiver},
		{name: "kind mismatch", err: kindMismatchErr, code: oracleErrors.JSONAccessError},
		{name: "array index", err: indexErr, code: oracleErrors.JSONArrayIndexOutOfRangeError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err == nil {
				t.Fatal("error = nil, want failure")
			}
			cause := errors.Unwrap(test.err)
			if cause == nil || cause.Error() == "" {
				t.Fatalf("error %v has no detailed cause", test.err)
			}
			sqlErr, ok := test.err.(oracleErrors.SQLError)
			if !ok {
				t.Fatalf("error type = %T, want oracleErrors.SQLError", test.err)
			}
			if got := sqlErr.ErrorCode(); got != string(test.code) {
				t.Fatalf("ErrorCode() = %s, want %s", got, test.code)
			}
		})
	}
}

// TestJSONScanCopiesSourceBytes verifies that JSON.Scan takes ownership of OSON
// bytes instead of retaining the caller-provided source slice.
func TestJSONScanCopiesSourceBytes(t *testing.T) {
	input := JSONValue{
		Data: map[string]any{
			"id":   int64(1),
			"name": "first",
		},
	}

	driverValue, err := input.Value()
	if err != nil {
		t.Fatalf("JSONValue.Value() failed: %v", err)
	}

	src, ok := driverValue.([]byte)
	if !ok {
		t.Fatalf("JSONValue.Value() returned %T, want []byte", driverValue)
	}

	var got JSON
	if err := got.Scan(src); err != nil {
		t.Fatalf("JSON.Scan() failed: %v", err)
	}

	// keep the replacement byte length identical so OSON structural offsets stay valid
	at := bytes.Index(src, []byte("first"))
	if at < 0 {
		t.Fatal(`encoded OSON does not contain "first" payload`)
	}
	copy(src[at:at+len("pwned")], []byte("pwned"))

	obj, err := got.GetJSONObject(JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("GetJSONObject() failed: %v", err)
	}

	nameJSON, ok := obj.Get("name")
	if !ok {
		t.Fatal(`field "name" missing`)
	}

	nameScalar, err := nameJSON.GetJSONScalar(JSONOptNumberAsString)
	if err != nil {
		t.Fatalf("GetJSONScalar() failed: %v", err)
	}

	nameValue, err := nameScalar.GetValue()
	if err != nil {
		t.Fatalf("GetValue() failed: %v", err)
	}

	if nameValue != "first" {
		t.Fatalf(`name = %#v, want "first"; JSON.Scan retained caller-owned []byte`, nameValue)
	}
}
