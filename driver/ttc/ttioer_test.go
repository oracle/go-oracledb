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

// TestNewTTIoer ensures the constructor sets struct fields as expected.
func TestNewTTIoer(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer()
	if oer == nil {
		t.Fatal("NewTTIoer returned nil")
	}
	ttioer, ok := oer.(*tTIoer)
	if !ok {
		t.Fatalf("NewTTIoer did not return *tTIoer, got %T", oer)
	}
	if ttioer.retCode != 0 || ttioer.curRowNumber != 0 {
		t.Errorf("Initial fields should be zero, got retCode=%d, curRowNumber=%d", ttioer.retCode, ttioer.curRowNumber)
	}
	if ttioer.oercn2 != 0 || ttioer.oerrcd2 != 0 {
		t.Errorf("Initial fields should be zero, got oertyp2=%d, oerchksm=%d", ttioer.oercn2, ttioer.oerrcd2)
	}
}

// TestTTIoer_Init sets nonzero values, calls Init, and checks that all fields reset.
func TestTTIoer_Init(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer().(*tTIoer)
	oer.retCode = 1234
	oer.errorMsg = []byte("stuff")
	oer.oerepa = []byte("x")
	oer.startErrorOffset = 22
	oer.endErrorOffset = 33
	oer.batchErrorOffsetArray = []int{99, 98}
	oer.oerrcd2 = 77
	oer.oercn2 = 88
	oer.Init()
	if oer.retCode != 0 {
		t.Errorf("Init should reset retCode to zero; got %d", oer.retCode)
	}
	if len(oer.errorMsg) != 0 {
		t.Errorf("Init should reset errorMsg")
	}
	if oer.oerepa != nil {
		t.Errorf("Init should reset oerepa to nil")
	}
	if oer.startErrorOffset != 0 {
		t.Errorf("Init should reset startErrorOffset to 0")
	}
	if oer.endErrorOffset != 0 {
		t.Errorf("Init should reset endErrorOffset to 0")
	}
	if oer.batchErrorOffsetArray != nil {
		t.Errorf("Init should reset batchErrorOffsetArray to nil")
	}
	if oer.oerrcd2 != 0 || oer.oercn2 != 0 {
		t.Errorf("Init should reset fields to zero; got oerrcd2=%d oercn2=%d", oer.oerrcd2, oer.oercn2)
	}
}

// TestTTIoer_GetMsgCode ensures GetMsgCode returns TTIOER.
func TestTTIoer_GetMsgCode(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer().(*tTIoer)
	if code := oer.GetMsgCode(); code != TTIOER {
		t.Errorf("GetMsgCode() = %v; want %v", code, TTIOER)
	}
}

// TestTTIoer_Getters validates GetCurRowNumber and GetRetCode on tTIoer.
func TestTTIoer_Getters(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer().(*tTIoer)
	oer.oercn2 = 99999999
	oer.oerrcd2 = 8888
	if got := oer.GetCurRowNumber(); got != 99999999 {
		t.Errorf("GetCurRowNumber() = %v, want %v", got, 99999999)
	}
	if got := oer.GetRetCode(); got != 8888 {
		t.Errorf("GetRetCode() = %v, want %v", got, 8888)
	}
}

// TestTTIoer_GetError covers both nil and error return paths of getError().
func TestTTIoer_GetError(t *testing.T) {
	t.Parallel()
	oer := &tTIoer{retCode: 0}
	if err := oer.getError(); err != nil {
		t.Errorf("Expected nil error when retCode=0, got: %v", err)
	}

	oer = &tTIoer{retCode: 942, errorMsg: []byte("ORA-00942 - table or view does not exist")}
	err := oer.getError()
	if err == nil {
		t.Fatal("Expected error when retCode != 0, got nil")
	}
	expected := "ORA-00942 - table or view does not exist"
	if got := err.Error(); got != expected {
		t.Errorf("Unexpected error string. Got: %q, want: %q", got, expected)
	}
}

// TestTTIoer_UnmarshalAttributes_Success unmarshals valid payload variants
// (default, EOCS elapsed time, planned downtime) and expects no error.
func TestTTIoer_UnmarshalAttributes_Success(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dump []string
	}{
		{
			name: "Default Success Path (no EOCS elapsed time)",
			dump: oerDump,
		},
		{
			name: "EOCS Elapsed Time Present",
			dump: oerDumpElapsedTime,
		},
		{
			name: "EOCS Planned down time Present",
			dump: oerDumpPlannedDowntime,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := makeOerPayload(tc.dump)
			oer := NewTTIoer().(*tTIoer)
			oer.setSupportsEndOfCallStatus(true)
			mar := createMarshaller(buf, 0, 0)
			err := oer._unmarshalAttributes(context.Background(), mar)
			if err != nil {
				t.Errorf("UnmarshalAttributes failed for %s: %v", tc.name, err)
			}
		})
	}
}

// TestTTIoer_UnmarshalAttributes_Fail is a table-driven test that triggers read failures
// at various positions during attribute unmarshalling and asserts an error is returned.
func TestTTIoer_UnmarshalAttributes_Fail(t *testing.T) {
	t.Parallel()
	failpoints := []struct {
		name       string
		failOn     FailOn
		failCount  int
		wantErrMsg string
	}{
		{"eocsCap", failOnReadByte, 1, "error"},
		{"elapsedTime", failOnReadByte, 2, "error"},
		{"endToEndECIDSequenceNumber", failOnReadByte, 3, "error"},
		{"curRowNumber", failOnReadByte, 4, "error"},
		{"retCode", failOnReadByte, 5, "error"},
		{"arrayElemWError", failOnReadByte, 6, "error"},
		{"arrayElemErrno", failOnReadByte, 7, "error"},
		{"currCursorID", failOnReadByte, 8, "error"},
		{"errorPosition", failOnReadByte, 9, "error"},
		{"sqlType", failOnReadByte, 10, "error"},
		{"oerFatal", failOnReadByte, 11, "error"},
		{"flags", failOnReadByte, 12, "error"},
		{"userCursorOpt", failOnReadByte, 13, "error"},
		{"upiParam", failOnReadByte, 14, "error"},
		{"warningFlag", failOnReadByte, 15, "error"},
		{"rba", failOnReadByte, 16, "error"},
		{"partitionID", failOnReadByte, 17, "error"},
		{"tableID", failOnReadByte, 18, "error"},
		{"blockNumber", failOnReadByte, 19, "error"},
		{"slotNumber", failOnReadByte, 20, "error"},
		{"osError", failOnReadByte, 21, "error"},
		{"stmtNumber", failOnReadByte, 22, "error"},
		{"callNumber", failOnReadByte, 23, "error"},
		{"pad1", failOnReadByte, 24, "error"},
		{"successIters", failOnReadByte, 25, "error"},
		{"oerrdd_UnMarshalFrom", failOnReadByte, 26, "error"},
		{"oerrar_UnMarshalFrom", failOnReadByte, 27, "error"},
		{"oerepa_UnMarshalFrom", failOnReadByte, 28, "error"},
		{"oermarl", failOnReadByte, 29, "error"},
		{"oerrcd2", failOnReadByte, 30, "error"},
		{"UnmarshalUB8_oercn2_Fail", failOnReadByte, 31, "error"},
	}

	for _, fp := range failpoints {
		t.Run(fp.name, func(t *testing.T) {
			oer := NewTTIoer().(*tTIoer)
			oer.setSupportsEndOfCallStatus(true)
			buf := makeOerPayload(oerDumpElapsedTime)
			mar := createMarshaller(buf, fp.failOn, fp.failCount)
			err := oer._unmarshalAttributes(context.Background(), mar)
			assertErrorContains(t, err, fp.wantErrMsg)
		})
	}
}

// TestTTIoer_UnMarshalFrom verifies success and failure paths including:
// - attribute unmarshalling failure
// - error-message unmarshalling path when retCode != 0
func TestTTIoer_UnMarshalFrom(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name         string
		fillBuffer   func() []byte
		failRead     int
		wantErrRegex string
		ErrorMsg     string
	}
	cases := []testCase{
		{
			name: "Success",
			fillBuffer: func() []byte {
				return makeOerPayload(oerDump)
			},
			wantErrRegex: "",
		},
		{
			name: "UnmarshalAttributes_Error",
			fillBuffer: func() []byte {
				return makeOerPayload(oerDump)
			},
			failRead:     1,
			wantErrRegex: "error",
		},
		{
			name: "UnmarshalErrorMessage_Success",
			fillBuffer: func() []byte {
				// Set retCode != 0 to trigger _unmarshalErrorMessage
				// Here we must ensure buf[retCode position] is nonzero; assume helper does so if needed
				return makeOerErrPayload(0)
			},
			wantErrRegex: "",
			ErrorMsg:     "ORA-00942: table or view does not exist",
		},
		{
			name: "UnmarshalErrorMessage_Fail",
			fillBuffer: func() []byte {
				buf := makeOerErrPayload(0)
				return buf
			},
			failRead:     31,
			wantErrRegex: "simulated|error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oer := NewTTIoer().(*tTIoer)
			oer.setSupportsEndOfCallStatus(true)
			mar := createMarshaller(tc.fillBuffer(), failOnReadByte, tc.failRead)
			err := oer.UnMarshalFrom(context.Background(), mar)
			assertErrorContains(t, err, tc.wantErrRegex)
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

// TestTTIoer_UpdateChecksum ensures the checksum computation includes all relevant fields
// and the bytes of errorMsg in the calculation.
func TestTTIoer_UpdateChecksum(t *testing.T) {
	t.Parallel()
	oer := NewTTIoer().(*tTIoer)
	oer.retCode = 0x1234
	oer.curRowNumber = 0xDEADBEEF
	oer.errorPosition = 77
	oer.sqlType = 2
	oer.oerFatal = 1
	oer.flags = 8
	oer.userCursorOpt = 9
	oer.upiParam = 10
	oer.warningFlag = 11
	oer.osError = 77777777
	oer.successIters = 333333
	oer.oerrcd2 = 0x42
	oer.oercn2 = 0xDEADBEEF
	oer.errorMsg = []byte("errormsg")
	start := uint64(987654321)
	got := oer._computeChecksum(start)

	// Manually calculate expected (same as logic in _computeChecksum)
	want := start
	want = CRC64UpdateChecksum(want, uint64(oer.retCode))
	want = CRC64UpdateChecksum(want, uint64(oer.curRowNumber))
	want = CRC64UpdateChecksum(want, uint64(oer.errorPosition))
	want = CRC64UpdateChecksum(want, uint64(oer.sqlType))
	want = CRC64UpdateChecksum(want, uint64(oer.oerFatal))
	want = CRC64UpdateChecksum(want, uint64(oer.flags))
	want = CRC64UpdateChecksum(want, uint64(oer.userCursorOpt))
	want = CRC64UpdateChecksum(want, uint64(oer.upiParam))
	want = CRC64UpdateChecksum(want, uint64(oer.warningFlag))
	want = CRC64UpdateChecksum(want, uint64(oer.osError))
	want = CRC64UpdateChecksum(want, uint64(oer.successIters))
	want = CRC64UpdateChecksumWithBytes(want, oer.errorMsg)
	want = CRC64UpdateChecksum(want, uint64(oer.oerrcd2))
	want = CRC64UpdateChecksum(want, uint64(oer.oercn2))
	if got != want {
		t.Errorf("_computeChecksum mismatch: got %x, want %x", got, want)
	}
}

// TestTTIoer_UnmarshalWarning covers the success path, field read failures, and branch where warnLength=0.
func TestTTIoer_UnmarshalWarning(t *testing.T) {
	t.Parallel()
	type warnTest struct {
		name         string
		retCode      uint16
		warnLength   uint16
		warnFlag     uint16
		wantErrorMsg []byte
		failRead     int
		wantErr      string
		failOn       FailOn
	}
	tests := []warnTest{
		{
			name:         "Success_with_warning",
			retCode:      55,
			warnLength:   3,
			warnFlag:     1,
			wantErrorMsg: []byte{0x41, 0x42, 0x43},
		},
		{
			name:     "Fail_at_retCode",
			failOn:   failOnReadByte,
			failRead: 1,
			wantErr:  "error",
		},
		{
			name:     "Fail_at_warnLength",
			retCode:  22,
			failOn:   failOnReadByte,
			failRead: 2, // will simulate error reading warnLength
			wantErr:  "error",
		},
		{
			name:       "Fail_at_warnFlag",
			retCode:    22,
			warnLength: 2,
			failOn:     failOnReadByte,
			failRead:   3, // will simulate error reading warnFlag
			wantErr:    "error",
		},
		{
			name:       "Fail_at_errorMsg_Read",
			retCode:    55,
			warnLength: 3,
			warnFlag:   1,
			failOn:     failOnReadBytes,
			failRead:   4, // when B1Array is read after RC, WL, WF
			wantErr:    "error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oer := NewTTIoer().(*tTIoer)
			// Build buffer: retCode (2 bytes), warnLength (2 bytes), warnFlag (2 bytes), B1Array (warnLength bytes)
			buf := []byte{}
			// retCode
			buf = append(buf, 2, byte(tc.retCode>>8), byte(tc.retCode))
			// warnLength
			buf = append(buf, 2, byte(tc.retCode>>8), byte(tc.warnLength))
			// warnFlag
			buf = append(buf, 2, byte(tc.retCode>>8), byte(tc.warnFlag))
			// If expected to read errorMsg
			if tc.retCode != 0 && tc.warnLength > 0 {
				if tc.wantErrorMsg != nil {
					buf = append(buf, tc.wantErrorMsg...)
				} else {
					// Fill with predictable pattern
					for i := 0; i < int(tc.warnLength); i++ {
						buf = append(buf, byte(0x61+i))
					}
				}
			}

			mar := createMarshaller(buf, tc.failOn, tc.failRead)
			err := oer._unmarshalWarning(context.Background(), mar)
			assertErrorContains(t, err, tc.wantErr)
			if err == nil {
				// Check errorMsg as needed
				if tc.wantErrorMsg != nil && string(oer.errorMsg) != string(tc.wantErrorMsg) {
					t.Errorf("expected errorMsg %v, got %v", tc.wantErrorMsg, oer.errorMsg)
				}
				if tc.warnLength == 0 && len(oer.errorMsg) != 0 {
					t.Errorf("expected errorMsg to remain zero when warnLength==0")
				}
				if tc.retCode == 0 && len(oer.errorMsg) != 0 {
					t.Errorf("expected errorMsg to remain zero when retCode==0")
				}
			}
		})
	}
}

func TestTTIoer_getConnectionShouldBeDropped(t *testing.T) {
	t.Parallel()
	msg := &tTIoer{
		_supportsEndOfCallStatus: false,
		eocStatus:                nil,
	}
	if msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
	msg = &tTIoer{
		_supportsEndOfCallStatus: true,
		eocStatus:                nil,
	}
	if msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
	msg = &tTIoer{
		_supportsEndOfCallStatus: true,
		eocStatus: &endOfCallStatus{
			elapsedTime:               0,
			connectionShouldBeDropped: false,
		},
	}
	if msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
	msg = &tTIoer{
		_supportsEndOfCallStatus: true,
		eocStatus: &endOfCallStatus{
			elapsedTime:               0,
			connectionShouldBeDropped: true,
		},
	}
	if !msg.isBeingDrainned() {
		t.Fatalf("Wrong value returned by connectionStatus.getConnectionShouldBeDropped()")
	}
}
