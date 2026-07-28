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
	"regexp"
	"testing"
)

// TestNewTTIoer14 ensures the constructor sets struct fields as expected.
func TestNewTTIoer14(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer14()
	if oer == nil {
		t.Fatal("NewTTIoer14 returned nil")
	}
	ttioer, ok := oer.(*tTIoer14)
	if !ok {
		t.Fatalf("NewTTIoer14 did not return *tTIoer14, got %T", oer)
	}
	if ttioer.sqlCommandType != 0 || ttioer.checksum != 0 {
		t.Errorf("Initial fields should be zero, got oertyp2=%d, oerchksm=%d", ttioer.sqlCommandType, ttioer.checksum)
	}
	if ttioer.tTIoer == nil {
		t.Error("tTIoer14 should embed a non-nil tTIoer")
	}
}

// TestTTIoer14_Init sets nonzero values, calls Init, and checks that all fields reset.
func TestTTIoer14_Init(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer14().(*tTIoer14)
	oer.sqlCommandType = 42
	oer.checksum = 99
	oer.tTIoer.oerrcd2 = 777
	oer.Init()
	if oer.sqlCommandType != 0 || oer.checksum != 0 {
		t.Errorf("Init should reset oertyp2 and oerchksm to zero; got oertyp2=%d, oerchksm=%d", oer.sqlCommandType, oer.checksum)
	}
	if oer.tTIoer.oerrcd2 != 0 {
		t.Errorf("Init should reset embedded tTIoer fields")
	}
}

// TestTTIoer14_GetMsgCode ensures GetMsgCode returns TTIOER.
func TestTTIoer14_GetMsgCode(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer14().(*tTIoer14)
	if code := oer.GetMsgCode(); code != TTIOER {
		t.Errorf("GetMsgCode() = %v; want %v", code, TTIOER)
	}
}

// TestTTIoer14_UnmarshalAttributes_Success unmarshals a valid payload and expects no error.
func TestTTIoer14_UnmarshalAttributes_Success(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer14WithEndOfCallStatusSupport().(*tTIoer14)
	buf := makeOerPayload(oerDump)
	mar := createMarshaller(buf, 0, 0)
	err := oer._unmarshalAttributes(context.Background(), mar)
	if err != nil {
		t.Errorf("expected success, got err: %v", err)
	}
}

// TestTTIoer14_UnmarshalAttributes_Fail is a modular table-driven test for failure cases.
func TestTTIoer14_UnmarshalAttributes_Fail(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		failOn    FailOn
		failCount int
		wantErr   string
	}{
		{"TTIoer_UnmarshalAttributes_Fail", failOnReadByte, 1, "simulated read error"},
		{"UnmarshalUB4_sqlCommandType_Fail", failOnReadByte, 31, "simulated read error"},
		{"UnmarshalUB4_oerchksm_Fail", failOnReadByte, 32, "simulated read error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oer := NewTTIoer14().(*tTIoer14)
			oer.setSupportsEndOfCallStatus(true)
			buf := makeOerPayload(oerDump)
			mar := createMarshaller(buf, tc.failOn, tc.failCount)
			err := oer._unmarshalAttributes(context.Background(), mar)
			assertErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestTTIoer14_UnMarshalFrom_Success verifies successful UnMarshalFrom paths:
// - when oerrcd2 == 0 (no error message section)
// - when oerrcd2 != 0, ensuring error message content is parsed as expected.
func TestTTIoer14_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name       string
		fillBuffer func() []byte
		ErrorMsg   string
	}

	cases := []testCase{
		{
			name: "Success",
			fillBuffer: func() []byte {
				// For UnMarshalFrom success, oerrcd2 must be 0
				return makeOerPayload(oerDump)
			},
			ErrorMsg: "",
		},
		{
			name: "UnmarshalErrorMessagePath_Success",
			fillBuffer: func() []byte {
				// Set oerrcd2 != 0 (triggers _unmarshalErrorMessage). Add a dummy CLR (0x00) at end for success.
				buf := makeOerErrPayload(14)
				return buf
			},
			ErrorMsg: "ORA-00942: table or view does not exist",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oer := NewTTIoer14().(*tTIoer14)
			oer.setSupportsEndOfCallStatus(true)
			buf := tc.fillBuffer()
			mar := createMarshaller(buf, 0, 0)
			err := oer.UnMarshalFrom(context.Background(), mar)
			assertErrorContains(t, err, "")
			// Match oer error
			if tc.ErrorMsg != "" {
				matched, matchErr := regexp.MatchString(tc.ErrorMsg, string(oer.errorMsg))
				if matchErr != nil {
					t.Errorf("failed to compile regex pattern: %v", matchErr)
				} else if !matched {
					t.Errorf("expected error to match pattern %v, got %v", tc.ErrorMsg, string(oer.errorMsg))
				}
			}
		})
	}
}

// TestTTIoer14_UnMarshalFrom_Fail triggers failures in different phases:
// - attribute unmarshalling failure
// - error-message unmarshalling failure (when oerrcd2 != 0),
// and asserts an error is returned in both cases.
func TestTTIoer14_UnMarshalFrom_Fail(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name       string
		fillBuffer func() []byte
		failOn     FailOn
		failCount  int
	}

	cases := []testCase{
		{
			name: "UnmarshalAttribute_Error",
			fillBuffer: func() []byte {
				buf := makeOerPayload(oerDump)
				return buf
			},
			failOn:    failOnReadByte,
			failCount: 3,
		},
		{
			name: "UnmarshalErrorMessagePath_Fail",
			fillBuffer: func() []byte {
				// Payload with oerrcd2 != 0 (triggers _unmarshalErrorMessage)
				buf := makeOerErrPayload(14)
				return buf
			},
			failOn:    failOnReadByte,
			failCount: 33,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oer := NewTTIoer14().(*tTIoer14)
			oer.setSupportsEndOfCallStatus(true)
			buf := tc.fillBuffer()
			mar := createMarshaller(buf, tc.failOn, tc.failCount)
			err := oer.UnMarshalFrom(context.Background(), mar)
			assertErrorContains(t, err, "error")
		})
	}
}

// TestTTIoer14_UpdateChecksum ensures the checksum computation includes
// the base tTIoer checksum plus the tTIoer14-specific fields (sqlCommandType and checksum).
func TestTTIoer14_UpdateChecksum(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer14().(*tTIoer14)
	// Set fields to known values
	oer.sqlCommandType = 0x12
	oer.checksum = 0x34
	start := uint64(0xabcdef01)
	got := oer._computeChecksum(start)
	want := oer.tTIoer._computeChecksum(start)
	want = CRC64UpdateChecksum(want, uint64(oer.sqlCommandType))
	want = CRC64UpdateChecksum(want, uint64(oer.checksum))
	if got != want {
		t.Errorf("_computeChecksum mismatch: got %x, want %x", got, want)
	}
}
