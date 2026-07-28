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
	"testing"
)

func TestTTIuds24_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	payload := makeTtiudsUnmarshalPayload(validTtiudsUnmarshalDump)
	mar := createMarshaller(payload, 0, 0)
	obj := newTTIuds24()
	err := obj.UnMarshalFrom(ctx, mar)
	if err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	// Only check vectorDim, vectorType, vectorFlag fields
	typed := obj.(*tTIuds24)
	if typed.vectorDim != 0 {
		t.Errorf("expected vectorDim 0, got %d", typed.vectorDim)
	}
	if typed.vectorType != 0 {
		t.Errorf("expected vectorType 0, got %d", typed.vectorType)
	}
	if typed.vectorFlag != 0 {
		t.Errorf("expected vectorFlag 0, got %d", typed.vectorFlag)
	}
}

func TestTTIuds24_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type faultyTest struct {
		name      string
		failCount int
	}
	faults := []faultyTest{
		{"Fail on TTIuds20 UnmarshalFrom", 1},
		{"Fail on vectorDim", 43},
		{"Fail on vectorType", 44},
		{"Fail on vectorFlag", 45},
	}
	payload := makeTtiudsUnmarshalPayload(validTtiudsUnmarshalDump)

	for _, tc := range faults {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mar := createMarshaller(payload, failOnReadByte, tc.failCount)
			obj := newTTIuds24()
			err := obj.UnMarshalFrom(ctx, mar)
			if err == nil {
				t.Errorf("expected error, got nil for fault case: %s", tc.name)
			}
		})
	}
}
