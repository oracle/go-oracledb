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
	"context"
	"strings"
	"testing"
)

func TestTTIuds20_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	payload := makeTtiudsUnmarshalPayload(validTtiudsUnmarshalDump)
	mar := createMarshaller(payload, 0, 0)
	obj := newTTIuds20()
	err := obj.UnMarshalFrom(ctx, mar)
	if err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	// Only check annotations field
	typed := obj.(*tTIuds20)
	expectedAnnotations := map[string]string{
		"IDENTITY": "",
		"DISPLAY":  "Employee ID",
		"Group":    "Emp_Info",
	}
	for k, v := range expectedAnnotations {
		if val, ok := typed.annotations[k]; !ok {
			t.Errorf("Expected annotation key %q not found in annotations map", k)
		} else if val != v {
			t.Errorf("For annotation key %q: expected value %q, got %q", k, v, val)
		}
	}
	if len(typed.annotations) != len(expectedAnnotations) {
		t.Errorf("Expected %d annotations, but got %d: %+v", len(expectedAnnotations), len(typed.annotations), typed.annotations)
	}
}

func TestTTIuds20_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type faultyTest struct {
		name      string
		failCount int
	}
	faults := []faultyTest{
		{"Fail on TTIuds17 UnmarshalFrom", 1},
		{"Fail on kvArrlen", 24},
		{"Fail on ignored byte", 25},
		{"Fail on numOfPairs", 26},
		{"Fail on ignored flag byte", 27},
		{"Fail on annotations", 28},
		{"Fail on ignore Trailing flag", 42},
	}
	payload := makeTtiudsUnmarshalPayload(validTtiudsUnmarshalDump)

	for _, tc := range faults {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mar := createMarshaller(payload, failOnReadByte, tc.failCount)
			obj := newTTIuds20()
			err := obj.UnMarshalFrom(ctx, mar)
			if err == nil {
				t.Errorf("expected error, got nil for case: %s", tc.name)
			} else {
				if !strings.Contains(err.Error(), "simulated read error") {
					t.Errorf("expected error message to contain 'simulated read error', got: %v", err)
				}
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}
