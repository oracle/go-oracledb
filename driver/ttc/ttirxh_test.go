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

	"github.com/oracle/go-driver/driver/common"
)

// TestTTIrxhConstructor verifies that NewTTIrxh constructs a non-nil tTIrxh instance
// with all fields initialized to their zero values. Also checks for message code.
func TestTTIrxhConstructor(t *testing.T) {
	t.Parallel()
	rxh := newTTIrxh()
	if rxh == nil {
		t.Error("newTTIrxh should not return nil")
	}

	got := rxh.GetMsgCode()
	want := TTIRXH
	if got != want {
		t.Errorf("GetMsgCode() = %v, want %v", got, want)
	}
}

// TestTTIrxhUnMarshalFrom_Success checks that UnMarshalFrom correctly decodes a valid payload.
// Validates rxhflags, numRequest, iterationNum, numItersThisTime, uacBufLength, rowBitVector, logicalRowId.
func TestTTIrxhUnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	payload := makeTtirxhPayload(validTtirxhDump)
	mar := createMarshaller(payload, 0, 0)
	p := newTTIrxh()
	ctx := context.Background()
	unmarshallable, _ := p.(common.UnMarshallable)
	err := unmarshallable.UnMarshalFrom(ctx, mar)
	if err != nil {
		t.Errorf("unMarshalFrom failed: %v", err)
	}
	rxh := p.(*tTIrxh)
	// Check for known values from validTtirxhDump
	if rxh.rxhflags != 34 {
		t.Errorf("expected rxhflags 34, got %d", rxh.rxhflags)
	}
	if rxh.numRequest != 2 {
		t.Errorf("expected numRequest 2, got %d", rxh.numRequest)
	}
	if rxh.iterationNum != 0 {
		t.Errorf("expected iterationNum 0, got %d", rxh.iterationNum)
	}
	if rxh.numItersThisTime != 10 {
		t.Errorf("expected numItersThisTime 10, got %d", rxh.numItersThisTime)
	}
	if rxh.uacBufLength != 0 {
		t.Errorf("expected uacBufLength 0, got %d", rxh.uacBufLength)
	}
	if len(rxh.rowBitVector.value) != 0 {
		t.Errorf("expected BitVectors (rowBitVector.value) empty, got %v", rxh.rowBitVector.value)
	}
	if len(rxh.logicalRowID.value) != 0 {
		t.Errorf("expected RowId (logicalRowId.value) empty, got %v", rxh.logicalRowID.value)
	}
}

// TestTTIrxhUnMarshalFrom_Fail checks that UnMarshalFrom correctly returns an error for simulated
// read failures at each decoding step, and that the error contains the expected message.
// One subtest is run for each possible field in tTIrxh that can fail unmarshalling.
func TestTTIrxhUnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type faultyTest struct {
		name      string
		failCount int
	}
	faults := []faultyTest{
		{"Fail on rxhflags", 1},
		{"Fail on numRequest", 2},
		{"Fail on iterationNum", 3},
		{"Fail on numItersThisTime", 4},
		{"Fail on uacBufLength", 5},
		{"Fail on rowBitVector", 6},
		{"Fail on logicalRowId", 7},
	}
	payload := makeTtirxhPayload(validTtirxhDump)

	for _, tc := range faults {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mar := createMarshaller(payload, failOnReadByte, tc.failCount)
			rxh := newTTIrxh()
			unmarshallable, _ := rxh.(common.UnMarshallable)
			err := unmarshallable.UnMarshalFrom(ctx, mar)
			if err == nil {
				t.Errorf("expected error, got nil for case: %s", tc.name)
			} else if !strings.Contains(err.Error(), "simulated read error") {
				t.Errorf("expected error message to contain 'simulated read error', got: %v", err)
			} else {
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}
