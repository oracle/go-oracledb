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
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

const (
	nclobReadOffset = driverCommon.UB8(6)
	nclobTestPhrase = "こんにちは – नमस्ते – Привет – مرحبا – 你好 – 😀 "
)

var (
	nclobExpectedString = strings.Repeat(nclobTestPhrase, 10)
	nclobReadNumChars   = driverCommon.UB8(32523) //common.UB8(len([]rune(nclobExpectedString)))
)

const nclobReadIsNCLOB = true

// TestClobExecutor_ReadNCLOB validates the NCLOB read path decodes multilingual
// UTF-16 payloads, reproduces the captured marshal bytes, and slices expected
// data by rune boundaries.
func TestClobExecutor_ReadNCLOB(t *testing.T) {
	t.Parallel()
	originalLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { common.Odl = originalLogger })
	ctx := context.Background()
	wantMarshal, shelf, dbuf, marshalWritePosition := setUpReadScenario(t, nclobReadMarshalGoldenPayload, nclobReadResponseGoldenPayload, 131072)

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	totalRuneCapacity := int(nclobReadNumChars)
	charOutBuffer := make([]rune, totalRuneCapacity)

	readAmt, err := lobExec.read(ctx, newLocator(nclobReadLocator, nclobReadOffset), nclobReadNumChars, nclobReadIsNCLOB, charOutBuffer)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if readAmt == 0 {
		t.Fatalf("expected read to return a positive amount")
	}
	decoded := string(charOutBuffer[:int(readAmt)])
	// Update once NCLOB response payload is available.
	// Skip the first five characters rather than five bytes so multi-byte runes are
	// handled correctly when slicing the expected NCLOB payload.
	expectedRunes := []rune(nclobExpectedString)
	expected := string(expectedRunes[5:])
	if decoded != expected {
		t.Fatalf("decoded payload mismatch:\n got: %q\nwant: %q", decoded, expected)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_WriteNCLOB confirms NCLOB writes marshal the UTF-16 payload
// captured from reference traces and report the server-counted code units for
// multilingual input.
func TestClobExecutor_WriteNCLOB(t *testing.T) {
	t.Parallel()
	originalLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { common.Odl = originalLogger })
	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(65536)
	locator := make(driverCommon.B1Array, len(nclobWriteLocator))
	copy(locator, nclobWriteLocator)

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(nclobWriteRPAGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (Write) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(nclobWriteMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	inputRunes := []rune(nclobExpectedString)

	written, err := lobExec.write(ctx, newLocator(locator, driverCommon.UB8(1)), true, inputRunes, len(inputRunes))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	expectedCodeUnits := len(utf16.Encode(inputRunes))
	if written != driverCommon.UB8(expectedCodeUnits) {
		t.Fatalf("unexpected written amount: got %d want %d", written, expectedCodeUnits)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

var nclobReadLocator = driverCommon.B1Array{
	0, 112, 0, 2, 4, 76, 2, 0, 0, 2, 0, 0, 0, 1, 0, 0,
	0, 23, 144, 159, 0, 1, 45, 216, 0, 1, 45, 215, 0, 3, 0, 3,
	7, 208, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 105, 186, 195, 168,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 53, 214, 149, 245,
	0, 0, 0, 0, 0, 0, 222, 173, 190, 239, 0, 1, 0, 34, 0, 0,
	0, 0, 0, 28, 55, 168, 0, 1, 0, 18, 0, 0, 6, 5, 0, 64, 2,
	15, 1, 83, 10, 0, 0, 0, 0, 1, 45, 215, 0, 65, 115, 196, 0, 0,
}

var nclobReadMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"72 00 00 00 00 00 00 00"`,
	`"01 02 00 00 01 06 00 01"`,
	`"00 00 00 00 00 00 00 70"`,
	`"00 02 04 4C 02 00 00 02"`,
	`"00 00 00 01 00 00 00 17"`,
	`"90 9F 00 01 2D D8 00 01"`,
	`"2D D7 00 03 00 03 07 D0"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 04 69 BA C3 A8 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 35 D6 95 F5 00 00"`,
	`"00 00 00 00 DE AD BE EF"`,
	`"00 01 00 22 00 00 00 00"`,
	`"00 1C 37 A8 00 01 00 12"`,
	`"00 00 06 05 00 40 02 0F"`,
	`"01 53 0A 00 00 00 00 01"`,
	`"2D D7 00 41 73 C4 00 00"`,
	`"02 7F 0B"`,
}

var nclobReadResponseGoldenPayload = []string{
	`"0E FE 02 03 3E 00"`,
	`"20 20 13 00 20 09 28 09"`,
	`"2E 09 38 09 4D 09 24 09"`,
	`"47 00 20 20 13 00 20 04"`,
	`"1F 04 40 04 38 04 32 04"`,
	`"35 04 42 00 20 20 13 00"`,
	`"20 06 45 06 31 06 2D 06"`,
	`"28 06 27 00 20 20 13 00"`,
	`"20 4F 60 59 7D 00 20 20"`,
	`"13 00 20 D8 3D DE 00 00"`,
	`"20 30 53 30 93 30 6B 30"`,
	`"61 30 6F 00 20 20 13 00"`,
	`"20 09 28 09 2E 09 38 09"`,
	`"4D 09 24 09 47 00 20 20"`,
	`"13 00 20 04 1F 04 40 04"`,
	`"38 04 32 04 35 04 42 00"`,
	`"20 20 13 00 20 06 45 06"`,
	`"31 06 2D 06 28 06 27 00"`,
	`"20 20 13 00 20 4F 60 59"`,
	`"7D 00 20 20 13 00 20 D8"`,
	`"3D DE 00 00 20 30 53 30"`,
	`"93 30 6B 30 61 30 6F 00"`,
	`"20 20 13 00 20 09 28 09"`,
	`"2E 09 38 09 4D 09 24 09"`,
	`"47 00 20 20 13 00 20 04"`,
	`"1F 04 40 04 38 04 32 04"`,
	`"35 04 42 00 20 20 13 00"`,
	`"20 06 45 06 31 06 2D 06"`,
	`"28 06 27 00 20 20 13 00"`,
	`"20 4F 60 59 7D 00 20 20"`,
	`"13 00 20 D8 3D DE 00 00"`,
	`"20 30 53 30 93 30 6B 30"`,
	`"61 30 6F 00 20 20 13 00"`,
	`"20 09 28 09 2E 09 38 09"`,
	`"4D 09 24 09 47 00 20 20"`,
	`"13 00 20 04 1F 04 40 04"`,
	`"38 04 32 04 35 04 42 00"`,
	`"20 20 13 00 20 06 45 06"`,
	`"31 06 2D 06 28 06 27 00"`,
	`"20 20 13 00 20 4F 60 59"`,
	`"7D 00 20 20 13 00 20 D8"`,
	`"3D DE 00 00 20 30 53 30"`,
	`"93 30 6B 30 61 30 6F 00"`,
	`"20 20 13 00 20 09 28 09"`,
	`"2E 09 38 09 4D 09 24 09"`,
	`"47 00 20 20 13 00 20 04"`,
	`"1F 04 40 04 38 04 32 04"`,
	`"35 04 42 00 20 20 13 00"`,
	`"20 06 45 06 31 06 2D 06"`,
	`"28 06 27 00 20 20 13 00"`,
	`"20 4F 60 59 7D 00 20 20"`,
	`"13 00 20 D8 3D DE 00 00"`,
	`"20 30 53 30 93 30 6B 30"`,
	`"61 30 6F 00 20 20 13 00"`,
	`"20 09 28 09 2E 09 38 09"`,
	`"4D 09 24 09 47 00 20 20"`,
	`"13 00 20 04 1F 04 40 04"`,
	`"38 04 32 04 35 04 42 00"`,
	`"20 20 13 00 20 06 45 06"`,
	`"31 06 2D 06 28 06 27 00"`,
	`"20 20 13 00 20 4F 60 59"`,
	`"7D 00 20 20 13 00 20 D8"`,
	`"3D DE 00 00 20 30 53 30"`,
	`"93 30 6B 30 61 30 6F 00"`,
	`"20 20 13 00 20 09 28 09"`,
	`"2E 09 38 09 4D 09 24 09"`,
	`"47 00 20 20 13 00 20 04"`,
	`"1F 04 40 04 38 04 32 04"`,
	`"35 04 42 00 20 20 13 00"`,
	`"20 06 45 06 31 06 2D 06"`,
	`"28 06 27 00 20 20 13 00"`,
	`"20 4F 60 59 7D 00 20 20"`,
	`"13 00 20 D8 3D DE 00 00"`,
	`"20 30 53 30 93 30 6B 30"`,
	`"61 30 6F 00 20 20 13 00"`,
	`"20 09 28 09 2E 09 38 09"`,
	`"4D 09 24 09 47 00 20 20"`,
	`"13 00 20 04 1F 04 40 04"`,
	`"38 04 32 04 35 04 42 00"`,
	`"20 20 13 00 20 06 45 06"`,
	`"31 06 2D 06 28 06 27 00"`,
	`"20 20 13 00 20 4F 60 59"`,
	`"7D 00 20 20 13 00 20 D8"`,
	`"3D DE 00 00 20 30 53 30"`,
	`"93 30 6B 30 61 30 6F 00"`,
	`"20 20 13 00 20 09 28 09"`,
	`"2E 09 38 09 4D 09 24 09"`,
	`"47 00 20 20 13 00 20 04"`,
	`"1F 04 40 04 38 04 32 04"`,
	`"35 04 42 00 20 20 13 00"`,
	`"20 06 45 06 31 06 2D 06"`,
	`"28 06 27 00 20 20 13 00"`,
	`"20 4F 60 59 7D 00 20 20"`,
	`"13 00 20 D8 3D DE 00 00"`,
	`"20 30 53 30 93 30 6B 30"`,
	`"61 30 6F 00 20 20 13 00"`,
	`"20 09 28 09 2E 09 38 09"`,
	`"4D 09 24 09 47 00 20 20"`,
	`"13 00 20 04 1F 04 40 04"`,
	`"38 04 32 04 35 04 42 00"`,
	`"20 20 13 00 20 06 45 06"`,
	`"31 06 2D 06 28 06 27 00"`,
	`"20 20 13 00 20 4F 60 59"`,
	`"7D 00 20 20 13 00 20 D8"`,
	`"3D DE 00 00 20 00 08 00"`,
	`"70 00 02 04 4C 02 00 00"`,
	`"02 00 00 00 01 00 00 00"`,
	`"17 90 9F 00 01 2D D8 00"`,
	`"01 2D D7 00 03 00 03 07"`,
	`"D0 00 00 00 00 00 00 00"`,
	`"00 00 04 69 BA C3 A8 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 35 D6 95 F5 00"`,
	`"00 00 00 00 00 DE AD BE"`,
	`"EF 00 01 00 22 00 00 00"`,
	`"00 00 1C 37 A8 00 01 00"`,
	`"12 00 00 06 05 00 40 02"`,
	`"0F 01 53 0A 00 00 00 00"`,
	`"01 2D D7 00 41 73 C4 00"`,
	`"00 02 01 9F 04 01 02 01"`,
	`"0D 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 03 01 2D"`,
	`"D7 01 01 00 03 01 73 C4"`,
	`"00 00 00 0D 00 00 00 00"`,
	`"00 00 00 00 00 00"`,
}

var nclobWriteMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"28 00 00 00 00 00 00 00"`,
	`"01 40 00 00 01 01 00 01"`,
	`"00 00 00 00 00 00 00 26"`,
	`"00 01 84 48 00 03 00 02"`,
	`"56 EA 00 00 00 40 00 00"`,
	`"00 01 07 D0 00 0A 00 00"`,
	`"00 01 00 00 35 D6 95 F5"`,
	`"00 00 00 01 00 00 02 01"`,
	`"A4 0E FE 02 03 48 30 53"`,
	`"30 93 30 6B 30 61 30 6F"`,
	`"00 20 20 13 00 20 09 28"`,
	`"09 2E 09 38 09 4D 09 24"`,
	`"09 47 00 20 20 13 00 20"`,
	`"04 1F 04 40 04 38 04 32"`,
	`"04 35 04 42 00 20 20 13"`,
	`"00 20 06 45 06 31 06 2D"`,
	`"06 28 06 27 00 20 20 13"`,
	`"00 20 4F 60 59 7D 00 20"`,
	`"20 13 00 20 D8 3D DE 00"`,
	`"00 20 30 53 30 93 30 6B"`,
	`"30 61 30 6F 00 20 20 13"`,
	`"00 20 09 28 09 2E 09 38"`,
	`"09 4D 09 24 09 47 00 20"`,
	`"20 13 00 20 04 1F 04 40"`,
	`"04 38 04 32 04 35 04 42"`,
	`"00 20 20 13 00 20 06 45"`,
	`"06 31 06 2D 06 28 06 27"`,
	`"00 20 20 13 00 20 4F 60"`,
	`"59 7D 00 20 20 13 00 20"`,
	`"D8 3D DE 00 00 20 30 53"`,
	`"30 93 30 6B 30 61 30 6F"`,
	`"00 20 20 13 00 20 09 28"`,
	`"09 2E 09 38 09 4D 09 24"`,
	`"09 47 00 20 20 13 00 20"`,
	`"04 1F 04 40 04 38 04 32"`,
	`"04 35 04 42 00 20 20 13"`,
	`"00 20 06 45 06 31 06 2D"`,
	`"06 28 06 27 00 20 20 13"`,
	`"00 20 4F 60 59 7D 00 20"`,
	`"20 13 00 20 D8 3D DE 00"`,
	`"00 20 30 53 30 93 30 6B"`,
	`"30 61 30 6F 00 20 20 13"`,
	`"00 20 09 28 09 2E 09 38"`,
	`"09 4D 09 24 09 47 00 20"`,
	`"20 13 00 20 04 1F 04 40"`,
	`"04 38 04 32 04 35 04 42"`,
	`"00 20 20 13 00 20 06 45"`,
	`"06 31 06 2D 06 28 06 27"`,
	`"00 20 20 13 00 20 4F 60"`,
	`"59 7D 00 20 20 13 00 20"`,
	`"D8 3D DE 00 00 20 30 53"`,
	`"30 93 30 6B 30 61 30 6F"`,
	`"00 20 20 13 00 20 09 28"`,
	`"09 2E 09 38 09 4D 09 24"`,
	`"09 47 00 20 20 13 00 20"`,
	`"04 1F 04 40 04 38 04 32"`,
	`"04 35 04 42 00 20 20 13"`,
	`"00 20 06 45 06 31 06 2D"`,
	`"06 28 06 27 00 20 20 13"`,
	`"00 20 4F 60 59 7D 00 20"`,
	`"20 13 00 20 D8 3D DE 00"`,
	`"00 20 30 53 30 93 30 6B"`,
	`"30 61 30 6F 00 20 20 13"`,
	`"00 20 09 28 09 2E 09 38"`,
	`"09 4D 09 24 09 47 00 20"`,
	`"20 13 00 20 04 1F 04 40"`,
	`"04 38 04 32 04 35 04 42"`,
	`"00 20 20 13 00 20 06 45"`,
	`"06 31 06 2D 06 28 06 27"`,
	`"00 20 20 13 00 20 4F 60"`,
	`"59 7D 00 20 20 13 00 20"`,
	`"D8 3D DE 00 00 20 30 53"`,
	`"30 93 30 6B 30 61 30 6F"`,
	`"00 20 20 13 00 20 09 28"`,
	`"09 2E 09 38 09 4D 09 24"`,
	`"09 47 00 20 20 13 00 20"`,
	`"04 1F 04 40 04 38 04 32"`,
	`"04 35 04 42 00 20 20 13"`,
	`"00 20 06 45 06 31 06 2D"`,
	`"06 28 06 27 00 20 20 13"`,
	`"00 20 4F 60 59 7D 00 20"`,
	`"20 13 00 20 D8 3D DE 00"`,
	`"00 20 30 53 30 93 30 6B"`,
	`"30 61 30 6F 00 20 20 13"`,
	`"00 20 09 28 09 2E 09 38"`,
	`"09 4D 09 24 09 47 00 20"`,
	`"20 13 00 20 04 1F 04 40"`,
	`"04 38 04 32 04 35 04 42"`,
	`"00 20 20 13 00 20 06 45"`,
	`"06 31 06 2D 06 28 06 27"`,
	`"00 20 20 13 00 20 4F 60"`,
	`"59 7D 00 20 20 13 00 20"`,
	`"D8 3D DE 00 00 20 30 53"`,
	`"30 93 30 6B 30 61 30 6F"`,
	`"00 20 20 13 00 20 09 28"`,
	`"09 2E 09 38 09 4D 09 24"`,
	`"09 47 00 20 20 13 00 20"`,
	`"04 1F 04 40 04 38 04 32"`,
	`"04 35 04 42 00 20 20 13"`,
	`"00 20 06 45 06 31 06 2D"`,
	`"06 28 06 27 00 20 20 13"`,
	`"00 20 4F 60 59 7D 00 20"`,
	`"20 13 00 20 D8 3D DE 00"`,
	`"00 20 30 53 30 93 30 6B"`,
	`"30 61 30 6F 00 20 20 13"`,
	`"00 20 09 28 09 2E 09 38"`,
	`"09 4D 09 24 09 47 00 20"`,
	`"20 13 00 20 04 1F 04 40"`,
	`"04 38 04 32 04 35 04 42"`,
	`"00 20 20 13 00 20 06 45"`,
	`"06 31 06 2D 06 28 06 27"`,
	`"00 20 20 13 00 20 4F 60"`,
	`"59 7D 00 20 20 13 00 20"`,
	`"D8 3D DE 00 00 20 00"`,
}

var nclobWriteRPAGoldenPayload = []string{
	`"00 26 00 01 84"`,
	`"48 00 03 00 02 56 EA 00"`,
	`"00 00 40 00 00 00 01 07"`,
	`"D0 00 0A 00 00 00 01 00"`,
	`"00 35 D6 95 F5 00 00 00"`,
	`"01 00 00 02 01 A4 04 01"`,
	`"01 01 08 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 08 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00"`,
}

var nclobWriteLocator = driverCommon.B1Array{
	0, 38, 0, 1, 132, 72, 0, 3, 0, 2, 86, 234, 0, 0,
	0, 64, 0, 0, 0, 1, 7, 208, 0, 10, 0, 0, 0, 1, 0, 0,
	53, 214, 149, 245, 0, 0, 0, 1, 0, 0,
}
