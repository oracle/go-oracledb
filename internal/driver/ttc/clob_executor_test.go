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
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// makeLobPayloadFromDump converts a TTC hex dump emitted in tests into a raw
// byte slice so TTC marshaling assertions can compare against captured traces.
func makeLobPayloadFromDump(dump []string) []byte {
	buf, _ := ExtractBytesFromDump(dump)
	return buf
}

// newLobTestShelf wires a Shelf with an in-memory marshaller and real message
// streamer so tests exercise the same marshaling logic as production executors.
func newLobTestShelf(bufSize int) (*driverCommon.Shelf[driverCommon.MessageType], *MessageStreamer, *ArrayBasedDataBuffer) {
	buf := NewArrayDataBuffer(bufSize)
	mar := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	ttiShelf := newShelf[driverCommon.MessageType]()
	shelf := ttiShelf.Shelf
	ttiShelf.RegisterMarshaller(mar)

	funcReg := NewRegistry[functionRegistryKey]()
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oLobOps}, 1, newTTIlob)
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oLobOps}, 1, newTTILobRPA)

	msgReg := NewRegistry[driverCommon.MessageType]()
	_ = msgReg.Register(TTILOBD, 1, newTTIlobd)
	_ = msgReg.Register(TTIOER, 14, newTTIoer14WithEndOfCallStatusSupport)

	factory := &SimpleFactory{
		ttcVersion:   24,
		msgregistry:  msgReg,
		funcregistry: funcReg,
	}
	ttiShelf.RegisterMessageFactory(factory)

	streamer := NewMessageStreamer(ttiShelf)
	// The captured LOB fixtures contain the data and RPA payloads. Add a zero-status
	// OER after a successfully decoded RPA so the fixture includes the terminal frame.
	streamer.RegisterPostUnmarshallCallback(TTIRPA, func(
		_ driverCommon.Message[driverCommon.MessageType],
		unmarshalErr error,
	) (bool, error) {
		if unmarshalErr != nil {
			return true, unmarshalErr
		}
		streamer.incomingMessages.PushBack(&incomingElement{message: &mockOer{}})
		return true, nil
	})
	ttiShelf.RegisterMessageStreamer(streamer)

	return shelf, streamer, buf
}

func newTestSessionContext() *driverCommon.SessionContext {
	sess := driverCommon.NewSessionContext()
	sess.SetSessionCharacterSets(al32Utf8CharSet, al16Utf16CharSet)
	return sess
}

// newSrcLocator clones the golden locator used by unit tests so assertions do
// not mutate shared state across test cases.
func newSrcLocator() driverCommon.B1Array {
	dst := make(driverCommon.B1Array, len(srcLocatorGolden))
	copy(dst, srcLocatorGolden)
	return dst
}

// TestLobExecutor_GetChunkSize verifies the CLOB executor issues the correct
// marshaling sequence when requesting the server-reported page size.
func TestLobExecutor_GetChunkSize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(8192)
	locatorBytes := newSrcLocator()
	locator := newLocator(locatorBytes, 0)

	// Pre-seed incoming wire: TTIRPA (GetPageSize)
	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(lobRpaPageSizeGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (GetChunkSize) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(lobGetChunkSizeMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	lobAmt, err := lobExec.getChunkSize(ctx, locator)
	if err != nil {
		t.Fatalf("ExecContext GetChunkSize failed: %v", err)
	}

	if lobAmt != driverCommon.UB8(8132) {
		t.Fatalf("ExecContext didnt match Lobamt: expected %d, got: %d", 8132, lobAmt)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_CreateTemporaryLob ensures temporary LOB creation returns
// the locator provided by the TTC response and emits the precise marshal bytes.
func TestClobExecutor_CreateTemporaryLob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	originalLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { common.Odl = originalLogger })
	shelf, _, dbuf := newLobTestShelf(8192)
	expectedLocator := newSrcLocator()

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(clobTempLocatorRPAGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (CreateTemporary) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(clobTempLocatorMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	gotLocator, err := lobExec.createTemporaryLob(ctx, true, 10, 1)
	if err != nil {
		t.Fatalf("CreateTemporaryLob failed: %v", err)
	}

	if !bytes.Equal(gotLocator, expectedLocator) {
		t.Fatalf("locator mismatch:\n got: % X\nwant: % X", gotLocator, expectedLocator)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_Write confirms text writes marshal the expected payload and
// report the number of runes persisted.
func TestClobExecutor_Write(t *testing.T) {
	t.Parallel()
	originalLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { common.Odl = originalLogger })
	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(65536)
	locator := newSrcLocator()

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(clobWriteRPAGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (Write) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(clobWriteMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	inputText := strings.Repeat("This is a large text example. ", 20)
	inputRunes := []rune(inputText)

	written, err := lobExec.write(ctx, newLocator(locator, driverCommon.UB8(1)), false, inputRunes, len(inputRunes))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if written != driverCommon.UB8(len(inputRunes)) {
		t.Fatalf("unexpected written amount: got %d want %d", written, len(inputRunes))
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_CLOBAndNCLOBAmountsUseUCS2Units verifies Oracle CLOB and NCLOB
// operations count supplementary characters as two UCS-2 units.
func TestClobExecutor_CLOBAndNCLOBAmountsUseUCS2Units(t *testing.T) {
	t.Parallel()

	const want = 3 // A (one UCS-2 unit) + 🙂 (surrogate pair)
	got := lobCharacterUnits([]rune("A🙂"))
	if got != want {
		t.Fatalf("lobCharacterUnits(A🙂) = %d, want %d UCS-2 units", got, want)
	}
	if got == len([]rune("A🙂")) {
		t.Fatalf("lobCharacterUnits used rune/code-point count %d instead of UCS-2 units", got)
	}
}

const (
	clobReadOffset   driverCommon.UB8 = 6
	clobReadNumChars driverCommon.UB8 = 32523
)

const clobReadIsNCLOB = false

// TestClobExecutor_Read validates the read path decodes UTF-16 payloads and
// tracks marshaled bytes exactly as captured from TTC traces.
func TestClobExecutor_Read(t *testing.T) {
	t.Parallel()
	originalLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { common.Odl = originalLogger })
	ctx := context.Background()
	wantMarshal, shelf, dbuf, marshalWritePosition := setUpReadScenario(t, clobReadMarshalGoldenPayload, clobReadResponseGoldenPayload, 131072)

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	payload, logical, err := lobExec.read(ctx, newLocator(clobReadLocator, clobReadOffset), clobReadNumChars, clobReadIsNCLOB, 0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if logical == 0 {
		t.Fatalf("expected read to return a positive amount")
	}
	decoded := string(payload)
	expected := strings.Repeat("This is a large text example. ", 20)[5:]
	if decoded != expected {
		t.Fatalf("decoded payload mismatch:\n got: %q\nwant: %q", decoded, expected)
	}
	if wantLogical := driverCommon.UB8(lobCharacterUnits([]rune(expected))); logical != wantLogical {
		t.Fatalf("logical units = %d, want %d", logical, wantLogical)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_ReadRejectsResponseBeyondRequest verifies that a CLOB
// response cannot advance the locator by more UTF-16 units than requested.
func TestClobExecutor_ReadRejectsResponseBeyondRequest(t *testing.T) {
	t.Parallel()

	shelf, _, _ := newLobTestShelf(8192)
	streamer := &fakeStreamer{
		events: []driverCommon.Message[driverCommon.MessageType]{
			newTTIlobd(),
			newTTILobRPA(),
			&mockOer{},
		},
		preHooks:      make(map[driverCommon.MessageType]StreamerPreUnmarshallCallback),
		postHooks:     make(map[driverCommon.MessageType]StreamerPostUnmarshallCallback),
		lobdPayloads:  [][]byte{[]byte("🙂")},
		lobRpaAmounts: []driverCommon.UB8{2},
	}
	shelf.RegisterMessageStreamer(streamer)

	executor := newClobExecutor(shelf, newTestSessionContext())
	_, _, err := executor.read(context.Background(), newLocator(newTestLocator(false), 1), 1, false, 0)
	requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
}

// TestClobExecutor_DecodeReadPayloadRejectsIncompleteCharacter verifies
// incomplete CLOB characters are rejected instead of decoded as U+FFFD.
func TestClobExecutor_DecodeReadPayloadRejectsIncompleteCharacter(t *testing.T) {
	t.Parallel()

	executor := newClobExecutor(newShelf[driverCommon.MessageType]().Shelf, newTestSessionContext())
	loc := newLocator(make(driverCommon.B1Array, koll4FlagOffset+1), 1)

	_, _, _, err := executor.decodeReadPayload(loc, false, []byte{0xF0, 0x9F, 0x99}, 0, false)
	requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)

	_, _, _, err = executor.decodeReadPayload(loc, true, []byte{0xD8, 0x3D}, 0, false)
	requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)

	if payload, logical, _, err := executor.decodeReadPayload(loc, false, nil, 0, false); err != nil || payload != nil || logical != 0 {
		t.Fatalf("empty decodeReadPayload = (%q, %d, %v), want (nil, 0, nil)", payload, logical, err)
	}
}

// TestClobExecutor_DecodeReadPayloadJoinsPrefetchSurrogate verifies that a
// high surrogate at the end of an inline CLOB prefix is joined with the low
// surrogate returned by the first locator read.
func TestClobExecutor_DecodeReadPayloadJoinsPrefetchSurrogate(t *testing.T) {
	t.Parallel()

	executor := newClobExecutor(newShelf[driverCommon.MessageType]().Shelf, newTestSessionContext())
	loc := newLocator(newTestLocator(true), 1)
	highSurrogate := uint16(0xD83D)

	prefix, prefixUnits, carry, err := executor.decodeReadPayload(
		loc,
		false,
		[]byte{0xD8, 0x3D},
		0,
		true,
	)
	if err != nil {
		t.Fatalf("decodeReadPayload for prefix returned error: %v", err)
	}
	if len(prefix) != 0 || prefixUnits != 1 || carry != highSurrogate {
		t.Fatalf("prefix result = (%q, %d, %#04x), want (empty, 1, %#04x)", prefix, prefixUnits, carry, highSurrogate)
	}

	decoded, locatorUnits, nextCarry, err := executor.decodeReadPayload(
		loc,
		false,
		[]byte{0xDE, 0x42},
		carry,
		false,
	)
	if err != nil {
		t.Fatalf("decode locator remainder returned error: %v", err)
	}
	if string(decoded) != "🙂" || locatorUnits != 1 || nextCarry != 0 {
		t.Fatalf("locator result = (%q, %d, %#04x), want (🙂, 1, 0)", decoded, locatorUnits, nextCarry)
	}
}

// TestClobExecutor_CharacterConversionHelpers verifies both byte orders,
// surrogate handling, and destination validation without a TTC exchange.
func TestClobExecutor_CharacterConversionHelpers(t *testing.T) {
	t.Parallel()

	executor := newClobExecutor(newShelf[driverCommon.MessageType]().Shelf, newTestSessionContext())
	runes := []rune("A🙂")
	bigEndian := []byte{0x00, 0x41, 0xD8, 0x3D, 0xDE, 0x42}
	littleEndian := []byte{0x41, 0x00, 0x3D, 0xD8, 0x42, 0xDE}

	for _, test := range []struct {
		name     string
		variable bool
		little   bool
		want     []byte
	}{
		{name: "fixed big endian", want: bigEndian},
		{name: "fixed little endian", little: true, want: littleEndian},
		{name: "variable big endian", variable: true, want: bigEndian},
		{name: "variable little endian", variable: true, little: true, want: littleEndian},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := make([]byte, len(test.want))
			bytesWritten, units, processed := executor.encodeLobCharPayload(runes, 0, len(runes), encoded, test.variable, test.little)
			if bytesWritten != len(test.want) || units != 3 || processed != len(runes) || !bytes.Equal(encoded, test.want) {
				t.Fatalf("encoded = (%d, %d, %d, % X), want (%d, 3, 2, % X)", bytesWritten, units, processed, encoded, len(test.want), test.want)
			}

			decoded := make([]rune, 3)
			decoded[0] = 'x'
			count, err := executor.decodeVariableWidthCharSet(test.want, decoded, 1, test.little)
			if err != nil || count != 2 || string(decoded) != "xA🙂" {
				t.Fatalf("decoded = (%q, %d, %v), want (xA🙂, 2, nil)", string(decoded), count, err)
			}
		})
	}

	for _, test := range []struct {
		name string
		call func() (int, int, int)
	}{
		{
			name: "empty source window",
			call: func() (int, int, int) {
				return executor.encodeLobCharPayload(runes, len(runes), 1, make([]byte, 4), false, false)
			},
		},
		{
			name: "variable buffer too small",
			call: func() (int, int, int) {
				return executor.encodeLobCharPayload(runes, 0, len(runes), make([]byte, 1), true, false)
			},
		},
		{
			name: "fixed buffer too small",
			call: func() (int, int, int) {
				return executor.encodeLobCharPayload(runes, 0, len(runes), make([]byte, 1), false, false)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bytesWritten, units, processed := test.call()
			if test.name == "empty source window" {
				if bytesWritten != 0 || units != 0 || processed != 0 {
					t.Fatalf("empty window result = (%d, %d, %d), want zeros", bytesWritten, units, processed)
				}
				return
			}
			if bytesWritten != -1 || units != -1 || processed != len(runes) {
				t.Fatalf("short buffer result = (%d, %d, %d), want (-1, -1, 2)", bytesWritten, units, processed)
			}
		})
	}

	for _, test := range []struct {
		name   string
		data   []byte
		little bool
		want   []uint16
	}{
		{name: "big endian", data: bigEndian, want: []uint16{0x41, 0xD83D, 0xDE42}},
		{name: "little endian", data: littleEndian, little: true, want: []uint16{0x41, 0xD83D, 0xDE42}},
	} {
		t.Run(test.name, func(t *testing.T) {
			units, err := readUTF16CodeUnits(test.data, test.little)
			if err != nil || !reflect.DeepEqual(units, test.want) {
				t.Fatalf("readUTF16CodeUnits = (%v, %v), want (%v, nil)", units, err, test.want)
			}
			encoded := make([]byte, len(test.data))
			if written := writeUTF16ToBuffer(test.want, encoded, test.little); written != len(encoded) || !bytes.Equal(encoded, test.data) {
				t.Fatalf("writeUTF16ToBuffer = (%d, % X), want (%d, % X)", written, encoded, len(encoded), test.data)
			}
		})
	}

	if got := getByteBufferSizeForConversion(false, 0); got != 0 {
		t.Fatalf("zero-character buffer size = %d, want 0", got)
	}
	if got := getByteBufferSizeForConversion(true, -1); got != 0 {
		t.Fatalf("negative-character buffer size = %d, want 0", got)
	}
	if got, err := executor.logicalAmount(runes); err != nil || got != 3 {
		t.Fatalf("logicalAmount = (%d, %v), want (3, nil)", got, err)
	}

	for _, test := range []struct {
		name string
		data []byte
		want []rune
	}{
		{name: "fixed UTF-8", data: []byte("A🙂"), want: []rune("A🙂")},
		{name: "fixed malformed UTF-8", data: []byte{0xFF}, want: []rune(string([]byte{0xEF, 0xBF, 0xBD}))},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := make([]rune, len(test.want))
			count, err := executor.decodeFixedWidthCharSet(test.data, out, 0, false, false)
			if err != nil || count != len(test.want) || !reflect.DeepEqual(out, test.want) {
				t.Fatalf("decodeFixedWidthCharSet = (%q, %d, %v), want (%q, %d, nil)", string(out), count, err, string(test.want), len(test.want))
			}
		})
	}

	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "unpaired high surrogate", data: []byte{0xD8, 0x3D}},
		{name: "unpaired low surrogate", data: []byte{0xDE, 0x42}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCompleteUTF16Payload(test.data, false); err == nil {
				t.Fatal("validateCompleteUTF16Payload unexpectedly accepted an unpaired surrogate")
			} else {
				requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
			}
		})
	}

	if _, err := executor.decodeVariableWidthCharSet(bigEndian, make([]rune, 1), 0, false); err == nil {
		t.Fatal("decodeVariableWidthCharSet accepted an undersized output buffer")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if _, err := executor.decodeVariableWidthCharSet([]byte{0x00}, make([]rune, 1), 0, false); err == nil {
		t.Fatal("decodeVariableWidthCharSet accepted an odd UTF-16 payload")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if _, err := executor.decodeFixedWidthCharSet([]byte("A🙂"), make([]rune, 1), 0, false, false); err == nil {
		t.Fatal("decodeFixedWidthCharSet accepted an undersized output buffer")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
	if got := writeUTF16ToBuffer([]uint16{1}, make([]byte, 1), false); got != -1 {
		t.Fatalf("short UTF-16 write = %d, want -1", got)
	}
}

// TestClobExecutor_OpenRejectsBFileMode verifies CLOB executors reject the
// BFILE-only open mode before touching the TTC stream.
func TestClobExecutor_OpenRejectsBFileMode(t *testing.T) {
	t.Parallel()

	executor := newClobExecutor(newShelf[driverCommon.MessageType]().Shelf, newTestSessionContext())
	if opened, err := executor.open(context.Background(), newLocator(newTestLocator(false), 1), bfileOpenModeReadOnly); opened || err == nil {
		t.Fatalf("open result = (%t, %v), want UnsupportedLobOperation", opened, err)
	} else {
		requireErrorCode(t, err, oracleErrors.UnsupportedLobOperation)
	}
}

// TestClobExecutor_ReadRejectsLogicalAmountMismatch verifies a decoded payload
// cannot contradict the server-reported UTF-16 amount.
func TestClobExecutor_ReadRejectsLogicalAmountMismatch(t *testing.T) {
	t.Parallel()

	setup := newClobExecutorWithStub(lobExecutorScenario{
		events: []driverCommon.Message[driverCommon.MessageType]{newTTIlobd(), newTTILobRPA(), &mockOer{}},
	})
	setup.stub.lobdPayloads = [][]byte{[]byte("a")}
	setup.stub.lobRpaAmounts = []driverCommon.UB8{2}
	_, _, err := setup.clob.read(context.Background(), newLocator(newTestLocator(false), 1), 2, false, 0)
	if err == nil {
		t.Fatal("read unexpectedly accepted a mismatched logical amount")
	}
	requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
}

// setUpReadScenario provisions the shared TTC shelves and buffers necessary to
// stage read responses without duplicating boilerplate in multiple tests.
func setUpReadScenario(t *testing.T, marshalDump, responseDump []string, outBufferLen int) ([]byte, *driverCommon.Shelf[driverCommon.MessageType], *ArrayBasedDataBuffer, int) {
	t.Helper()

	mPayload, err := ExtractBytesFromDump(marshalDump)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	responsePayload, err := ExtractBytesFromDump(responseDump)
	if err != nil {
		t.Fatalf("failed to build expected response payload: %v", err)
	}

	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(outBufferLen)

	if err := dbuf.WriteBytesWithContext(ctx, responsePayload); err != nil {
		t.Fatalf("write response payload failed: %v", err)
	}

	marshalWritePosition := dbuf.currentWritePosition

	return mPayload, shelf, dbuf, marshalWritePosition
}

// TestClobExecutor_IsOpen verifies local is-open requests marshal correctly and
// interpret the server response state.
func TestClobExecutor_IsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(8192)
	locator := make(driverCommon.B1Array, len(clobIsOpenSourceLocator))
	copy(locator, clobIsOpenSourceLocator)

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(clobIsOpenRPAGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (IsOpen) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(clobIsOpenMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	isOpen, err := lobExec.isOpen(ctx, newLocator(locator, 0))
	if err != nil {
		t.Fatalf("IsOpen failed: %v", err)
	}

	if isOpen {
		t.Fatalf("expected locator to report closed state")
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_GetLength ensures length queries return the server-reported
// size and match the marshaled payload captured from reference traces.
func TestClobExecutor_GetLength(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(8192)
	locator := make(driverCommon.B1Array, len(clobGetLengthSourceLocator))
	copy(locator, clobGetLengthSourceLocator)

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(clobGetLengthRPAGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (GetLength) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(clobGetLengthMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	gotLength, err := lobExec.getLength(ctx, newLocator(locator, 0))
	if err != nil {
		t.Fatalf("GetLength failed: %v", err)
	}

	if gotLength != driverCommon.UB8(30) {
		t.Fatalf("unexpected length: got %d want %d", gotLength, 30)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// TestClobExecutor_Trim checks that trim operations marshal the TTC payload and
// return the new length supplied by the server.
func TestClobExecutor_Trim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, dbuf := newLobTestShelf(8192)
	locator := make(driverCommon.B1Array, len(clobTrimSourceLocator))
	copy(locator, clobTrimSourceLocator)

	if err := dbuf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := dbuf.WriteBytesWithContext(ctx, makeLobPayloadFromDump(clobTrimRPAGoldenPayload)); err != nil {
		t.Fatalf("write oLobOps RPA (Trim) payload failed: %v", err)
	}
	marshalWritePosition := dbuf.currentWritePosition

	wantMarshal, err := ExtractBytesFromDump(clobTrimMarshalGoldenPayload)
	if err != nil {
		t.Fatalf("failed to build expected marshal payload: %v", err)
	}

	lobExec := newClobExecutor(shelf, newTestSessionContext())
	const newLength driverCommon.UB8 = 30
	trimmedLength, err := lobExec.trim(ctx, newLocator(locator, 0), newLength)
	if err != nil {
		t.Fatalf("Trim failed: %v", err)
	}

	if trimmedLength != newLength {
		t.Fatalf("unexpected trimmed length: got %d want %d", trimmedLength, newLength)
	}

	gotMarshal := dbuf.bytes[marshalWritePosition:dbuf.currentWritePosition]
	if !bytes.Equal(gotMarshal, wantMarshal) {
		t.Fatalf("marshal payload mismatch:\n got: % X\nwant: % X", gotMarshal, wantMarshal)
	}
}

// -------------------------------------------- Error Test ---------------------------------------------------------

// TestClobExecutor_ReadErrors documents validation and transport failures that
// should surface while executing CLOB reads, ensuring diagnostic coverage.
func TestClobExecutor_ReadErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, driverCommon.UB8, bool, []rune, int)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "base read push error",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, driverCommon.UB8, bool, []rune, int) {
				ts := newClobExecutorWithStub(lobExecutorScenario{pushErr: errors.New("push failed")})
				return ts.clob, clobReadLocator, 0, 1, false, make([]rune, 1), 0
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
		{
			name: "decode error",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, driverCommon.UB8, bool, []rune, int) {
				numChars := driverCommon.UB8(1)
				locator := newTestLocator(true)
				ts := newClobExecutorWithStub(lobExecutorScenario{
					locator: locator,
					onFlush: func(def *lobDefinition) {
						def.bytesTransferred = 1
					},
				})
				return ts.clob, locator, 0, numChars, false, make([]rune, 2), 0
			},
			expectErrContains: "invalid UTF-16 byte length",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, locator, offset, numChars, isNCLOB, _, _ := tc.setup()
			_, _, err := exec.read(ctx, newLocator(locator, offset), numChars, isNCLOB, 0)
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestClobExecutor_WriteErrors captures expected failure modes when writing to
// CLOB locators, validating both input validation and marshaling errors.
func TestClobExecutor_WriteErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, bool, []rune, int, int)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "value based locator",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, bool, []rune, int, int) {
				exec := newClobExecutor(&driverCommon.Shelf[driverCommon.MessageType]{}, newTestSessionContext())
				locator := newTestLocator(false)
				locator[koll1FlagOffset] = kolblValueBasedLocatorFlag
				return exec, locator, 1, false, []rune("a"), 0, 1
			},
			expectErrCode:     string(oracleErrors.InvalidLOBBuffer),
			expectErrContains: "value-based locator not supported",
		},
		{
			name: "read only locator",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, bool, []rune, int, int) {
				exec := newClobExecutor(&driverCommon.Shelf[driverCommon.MessageType]{}, newTestSessionContext())
				locator := newTestLocator(false)
				locator[koll3FlagOffset] = kolblReadOnlyFlag
				return exec, locator, 1, false, []rune("a"), 0, 1
			},
			expectErrCode:     string(oracleErrors.InvalidLOBBuffer),
			expectErrContains: "locator is read-only",
		},
		{
			name: "base execute error",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8, bool, []rune, int, int) {
				scenario := lobExecutorScenario{flushErr: errors.New("flush failed")}
				ts := newClobExecutorWithStub(scenario)
				locator := newTestLocator(false)
				return ts.clob, locator, 1, false, []rune("hi"), 0, 2
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, locator, offset, isNCLOB, inBuffer, _, numChars := tc.setup()
			_, err := exec.write(ctx, newLocator(locator, offset), isNCLOB, inBuffer, numChars)
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestClobExecutor_CreateTemporaryLobErrors verifies temporary LOB creation
// surfaces validation and execution errors with the appropriate error codes.
func TestClobExecutor_CreateTemporaryLobErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, bool, driverCommon.UB4, driverCommon.UB2)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "base execute error",
			setup: func() (*clobExecutor, bool, driverCommon.UB4, driverCommon.UB2) {
				ts := newClobExecutorWithStub(lobExecutorScenario{flushErr: errors.New("flush failed")})
				return ts.clob, true, 5, 1
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, cache, duration, formOfUse := tc.setup()
			_, err := exec.createTemporaryLob(ctx, cache, duration, formOfUse)
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestClobExecutor_GetLengthErrors verifies CLOB length execution errors are
// returned with the expected driver code.
func TestClobExecutor_GetLengthErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, driverCommon.B1Array)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "base execute error",
			setup: func() (*clobExecutor, driverCommon.B1Array) {
				ts := newClobExecutorWithStub(lobExecutorScenario{flushErr: errors.New("flush failed")})
				return ts.clob, newTestLocator(false)
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, locator := tc.setup()
			_, err := exec.getLength(ctx, newLocator(locator, 0))
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestClobExecutor_IsOpenErrors verifies CLOB open-state execution errors are
// returned with the expected driver code.
func TestClobExecutor_IsOpenErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, driverCommon.B1Array)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "base execute error",
			setup: func() (*clobExecutor, driverCommon.B1Array) {
				locator := newTestLocator(false)
				ts := newClobExecutorWithStub(lobExecutorScenario{flushErr: errors.New("flush failed")})
				return ts.clob, locator
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, locator := tc.setup()
			_, err := exec.isOpen(ctx, newLocator(locator, 0))
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestClobExecutor_TrimErrors verifies CLOB trim validation and execution
// errors are returned with the expected driver code.
func TestClobExecutor_TrimErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "value based locator",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8) {
				exec := newClobExecutor(&driverCommon.Shelf[driverCommon.MessageType]{}, newTestSessionContext())
				locator := newTestLocator(false)
				locator[koll1FlagOffset] = kolblValueBasedLocatorFlag
				return exec, locator, 1
			},
			expectErrCode:     string(oracleErrors.InvalidLOBBuffer),
			expectErrContains: "value-based locator not supported",
		},
		{
			name: "read only locator",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8) {
				exec := newClobExecutor(&driverCommon.Shelf[driverCommon.MessageType]{}, newTestSessionContext())
				locator := newTestLocator(false)
				locator[koll3FlagOffset] = kolblReadOnlyFlag
				return exec, locator, 1
			},
			expectErrCode:     string(oracleErrors.InvalidLOBBuffer),
			expectErrContains: "locator is read-only",
		},
		{
			name: "base execute error",
			setup: func() (*clobExecutor, driverCommon.B1Array, driverCommon.UB8) {
				ts := newClobExecutorWithStub(lobExecutorScenario{flushErr: errors.New("flush failed")})
				locator := newTestLocator(false)
				return ts.clob, locator, 5
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, locator, newLength := tc.setup()
			_, err := exec.trim(ctx, newLocator(locator, 0), newLength)
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestClobExecutor_GetChunkSizeErrors verifies CLOB chunk-size execution
// errors are returned with the expected driver code.
func TestClobExecutor_GetChunkSizeErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name              string
		setup             func() (*clobExecutor, driverCommon.B1Array)
		expectErrContains string
		expectErrCode     string
	}{
		{
			name: "base execute error",
			setup: func() (*clobExecutor, driverCommon.B1Array) {
				ts := newClobExecutorWithStub(lobExecutorScenario{flushErr: errors.New("flush failed")})
				return ts.clob, newTestLocator(false)
			},
			expectErrCode: string(oracleErrors.LobExecError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, locator := tc.setup()
			_, err := exec.getChunkSize(ctx, newLocator(locator, 0))
			if err == nil {
				t.Fatalf("expected error")
			}
			if tc.expectErrCode != "" {
				requireErrorCode(t, err, oracleErrors.ErrorCode(tc.expectErrCode))
			}
			if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestLobExecutor_ConsumeLobResponses_DelayedOER verifies that a LOB operation
// waits after TTIRPA and consumes a delayed terminal TTIOER before returning.
func TestLobExecutor_ConsumeLobResponses_DelayedOER(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("delayed TTIOER")
	tests := []struct {
		name     string
		prefix   []driverCommon.Message[driverCommon.MessageType]
		terminal *mockOer
		wantErr  error
	}{
		{
			name: "read data RPA and delayed successful OER",
			prefix: []driverCommon.Message[driverCommon.MessageType]{
				newTTIlobd(),
				newTTILobRPA(),
			},
			terminal: &mockOer{},
		},
		{
			name:     "RPA and delayed error OER",
			prefix:   []driverCommon.Message[driverCommon.MessageType]{newTTILobRPA()},
			terminal: &mockOer{err: wantErr},
			wantErr:  wantErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oerReady := make(chan struct{})
			waitingForOER := make(chan struct{})
			pullCount := 0

			st := newClobExecutorWithStub(lobExecutorScenario{
				events: tt.prefix,
				pullHook: func(ctx context.Context, expected ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
					if !_isExpectedType(expected, TTIOER) {
						return nil, errors.New("TTIOER is missing from the blocking Pull set")
					}
					pullCount++
					if pullCount <= len(tt.prefix) {
						return nil, nil
					}

					close(waitingForOER)
					select {
					case <-oerReady:
						return tt.terminal, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				},
			})

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- st.clob.lobExecutor._consumeLobResponses(ctx)
			}()

			select {
			case <-waitingForOER:
			case err := <-result:
				t.Fatalf("LOB response handling returned before delayed TTIOER arrived: %v", err)
			case <-ctx.Done():
				t.Fatal("LOB response handling did not wait for terminal TTIOER")
			}

			close(oerReady)
			if err := <-result; !errors.Is(err, tt.wantErr) {
				t.Fatalf("unexpected result: got %v, want %v", err, tt.wantErr)
			}
			if len(st.stub.events) != 0 {
				t.Fatalf("LOB response handling left %d stale message(s)", len(st.stub.events))
			}
		})
	}
}

// TestLobExecutor_ConsumeLobResponses_OERTermination verifies successful OER
// completion after TTIRPA and error OER completion received before TTIRPA.
func TestLobExecutor_ConsumeLobResponses_OERTermination(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("LOB terminal error")
	tests := []struct {
		name    string
		events  []driverCommon.Message[driverCommon.MessageType]
		wantErr error
	}{
		{
			name: "RPA and successful OER",
			events: []driverCommon.Message[driverCommon.MessageType]{
				newTTILobRPA(),
				&mockOer{},
			},
		},
		{
			name:    "error OER before RPA",
			events:  []driverCommon.Message[driverCommon.MessageType]{&mockOer{err: wantErr}},
			wantErr: wantErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newClobExecutorWithStub(lobExecutorScenario{events: tt.events})
			err := st.clob.lobExecutor._consumeLobResponses(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("unexpected result: got %v, want %v", err, tt.wantErr)
			}
			if len(st.stub.events) != 0 {
				t.Fatalf("LOB response handling left %d stale message(s)", len(st.stub.events))
			}
		})
	}
}

// TestLobExecutor_ConsumeLobResponses_PullError verifies that a Pull failure
// after TTIRPA is preserved and classified as a LOB execution error.
func TestLobExecutor_ConsumeLobResponses_PullError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("network read failed")
	st := newClobExecutorWithStub(lobExecutorScenario{
		events:  []driverCommon.Message[driverCommon.MessageType]{newTTILobRPA()},
		pullErr: wantErr,
	})

	err := st.clob.lobExecutor._consumeLobResponses(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected Pull error %v, got %v", wantErr, err)
	}
	requireErrorCode(t, err, oracleErrors.LobExecError)
	if len(st.stub.events) != 0 {
		t.Fatalf("LOB response handling left %d stale message(s)", len(st.stub.events))
	}
}

// -------------------------------------------- Helpers for test ---------------------------------------------------------

// newClobExecutorWithStub constructs a CLOB executor backed by a fake
// streamer, allowing unit tests to simulate TTC failures deterministically.
func newClobExecutorWithStub(s lobExecutorScenario) testSetup {
	shelf, _, _ := newLobTestShelf(8192)
	stub := &fakeStreamer{
		pushErr:   s.pushErr,
		flushErr:  s.flushErr,
		pullErr:   s.pullErr,
		locator:   s.locator,
		events:    make([]driverCommon.Message[driverCommon.MessageType], 0),
		preHooks:  make(map[driverCommon.MessageType]StreamerPreUnmarshallCallback),
		postHooks: make(map[driverCommon.MessageType]StreamerPostUnmarshallCallback),
		pullHook:  s.pullHook,
	}
	shelf.RegisterMessageStreamer(stub)
	clob := newClobExecutor(shelf, newTestSessionContext())

	if len(s.events) == 0 {
		msg, err := shelf.GetMessageFactory().(Factory).GetMessageForFunction(TTIRPA, oLobOps)
		if err != nil {
			panic(err)
		}
		s.events = []driverCommon.Message[driverCommon.MessageType]{msg, &mockOer{}}
	}
	stub.events = append(stub.events, s.events...)

	if s.onFlush != nil {
		stub.onFlush = func() { s.onFlush(stub.definition) }
	}

	return testSetup{clob: clob, stub: stub, shelf: shelf}
}

// lobExecutorScenario describes the fake streamer behaviors needed to exercise
// specific lobExecutor execution paths under test.

type testSetup struct {
	clob  *clobExecutor
	stub  *fakeStreamer
	shelf *driverCommon.Shelf[driverCommon.MessageType]
}

type lobExecutorScenario struct {
	pushErr  error
	flushErr error
	pullErr  error
	events   []driverCommon.Message[driverCommon.MessageType]
	locator  driverCommon.B1Array
	onFlush  func(*lobDefinition)
	pullHook func(context.Context, ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error)
}

// newTestLocator constructs a locator with sufficient length and optional
// variable width bit so tests can toggle locator capabilities.
func newTestLocator(variableWidth bool) driverCommon.B1Array {
	locator := make(driverCommon.B1Array, koll4FlagOffset+1)
	if variableWidth {
		locator[koll3FlagOffset] = kolblVaryingWidthFlag
	}
	return locator
}

type fakeStreamer struct {
	pushErr       error
	pushed        []driverCommon.Message[driverCommon.MessageType]
	flushErr      error
	pullErr       error
	events        []driverCommon.Message[driverCommon.MessageType]
	preHooks      map[driverCommon.MessageType]StreamerPreUnmarshallCallback
	postHooks     map[driverCommon.MessageType]StreamerPostUnmarshallCallback
	pullHook      func(context.Context, ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error)
	definition    *lobDefinition
	lobdPayloads  [][]byte
	lobRpaAmounts []driverCommon.UB8
	onPush        func(*lobDefinition)
	onFlush       func()
	locator       driverCommon.B1Array
}

func (s *fakeStreamer) Push(_ context.Context, msg driverCommon.Message[driverCommon.MessageType]) error {
	if s.pushErr != nil {
		return s.pushErr
	}
	if lob, ok := msg.(*tTIlob); ok {
		s.definition = lob.lobPayloadDefinition
		if s.onPush != nil {
			s.onPush(s.definition)
		}
	}
	s.pushed = append(s.pushed, msg)
	return nil
}

func (s *fakeStreamer) Pull(ctx context.Context, expected ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
	if s.pullHook != nil {
		msg, err := s.pullHook(ctx, expected...)
		if err != nil || msg != nil {
			return msg, err
		}
	}
	if len(s.events) == 0 {
		if s.pullErr != nil {
			return nil, s.pullErr
		}
		return nil, context.DeadlineExceeded
	}
	msg := s.events[0]
	s.events = s.events[1:]
	if cb := s.preHooks[msg.GetMsgCode()]; cb != nil {
		allocated, err := cb(nil)
		if err != nil {
			return nil, err
		}
		if allocated != nil {
			msg = allocated
		}
	}
	if lobd, ok := msg.(*tTIlobd); ok && len(s.lobdPayloads) > 0 {
		payload := s.lobdPayloads[0]
		s.lobdPayloads = s.lobdPayloads[1:]
		copy(lobd.buffer, payload)
		lobd.lastBytesRead = driverCommon.UB8(len(payload))
	}
	if rpa, ok := msg.(*ttiLobRpa); ok && len(s.lobRpaAmounts) > 0 {
		rpa.lobDefinition.lobAmt = s.lobRpaAmounts[0]
		s.lobRpaAmounts = s.lobRpaAmounts[1:]
	}
	if cb := s.postHooks[msg.GetMsgCode()]; cb != nil {
		if _, err := cb(msg, nil); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

func (s *fakeStreamer) Flush(context.Context) error {
	if s.onFlush != nil {
		s.onFlush()
	}
	return s.flushErr
}

func (s *fakeStreamer) Drain(context.Context, driverCommon.StreamDirection) (int, int) { return -1, -1 }

func (s *fakeStreamer) RegisterPostUnmarshallCallback(msgType driverCommon.MessageType, cb StreamerPostUnmarshallCallback) {
	if s.postHooks == nil {
		s.postHooks = make(map[driverCommon.MessageType]StreamerPostUnmarshallCallback)
	}
	s.postHooks[msgType] = cb
}

func (s *fakeStreamer) RegisterPreUnmarshallCallback(msgType driverCommon.MessageType, cb StreamerPreUnmarshallCallback) {
	if s.preHooks == nil {
		s.preHooks = make(map[driverCommon.MessageType]StreamerPreUnmarshallCallback)
	}
	s.preHooks[msgType] = cb
}

func (s *fakeStreamer) UnRegisterPostUnmarshallCallback(msgType driverCommon.MessageType) {
	delete(s.postHooks, msgType)
}

func (s *fakeStreamer) UnRegisterPreUnmarshallCallback(msgType driverCommon.MessageType) {
	delete(s.preHooks, msgType)
}

// -------------------------------------------- Payloads for test ---------------------------------------------------------

var srcLocatorGolden = driverCommon.B1Array{
	0, 38, 0, 1, 130, 8, 128, 3, 0, 2, 41, 37, 0, 0, 1, 23, 0,
	0, 0, 1, 3, 105, 0, 10, 0, 0, 0, 1, 0, 0, 73, 46, 50, 9,
	0, 0, 0, 1, 0, 0}

var lobGetChunkSizeMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"28 00 00 00 00 00 00 00"`,
	`"02 40 00 00 00 00 00 01"`,
	`"00 00 00 00 00 00 00 26"`,
	`"00 01 82 08 80 03 00 02"`,
	`"29 25 00 00 01 17 00 00"`,
	`"00 01 03 69 00 0A 00 00"`,
	`"00 01 00 00 49 2E 32 09"`,
	`"00 00 00 01 00 00 00"`,
}

var clobTempLocatorMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"6C 00 01 0A 00 00 01 00"`,
	`"01 02 01 10 01 01 01 01"`,
	`"01 01 70 01 00 00 00 00"`,
	`"00 00 00 6A 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 02 03"`,
	`"69 01 01 01 0A"`,
}

var clobTempLocatorRPAGoldenPayload = []string{
	`"00 26 00 01 82"`,
	`"08 80 03 00 02 29 25 00"`,
	`"00 01 17 00 00 00 01 03"`,
	`"69 00 0A 00 00 00 01 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"01 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 02"`,
	`"03 69 01 60 01 01 04 01"`,
	`"01 02 20 0D 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 06"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00"`,
}

var clobWriteMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"28 00 00 00 00 00 00 00"`,
	`"01 40 00 00 01 01 00 01"`,
	`"00 00 00 00 00 00 00 26"`,
	`"00 01 82 08 80 03 00 02"`,
	`"29 25 00 00 01 17 00 00"`,
	`"00 01 03 69 00 0A 00 00"`,
	`"00 01 00 00 49 2E 32 09"`,
	`"00 00 00 01 00 00 02 02"`,
	`"58 0E FE 02 04 B0 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00 54"`,
	`"00 68 00 69 00 73 00 20"`,
	`"00 69 00 73 00 20 00 61"`,
	`"00 20 00 6C 00 61 00 72"`,
	`"00 67 00 65 00 20 00 74"`,
	`"00 65 00 78 00 74 00 20"`,
	`"00 65 00 78 00 61 00 6D"`,
	`"00 70 00 6C 00 65 00 2E"`,
	`"00 20 00 54 00 68 00 69"`,
	`"00 73 00 20 00 69 00 73"`,
	`"00 20 00 61 00 20 00 6C"`,
	`"00 61 00 72 00 67 00 65"`,
	`"00 20 00 74 00 65 00 78"`,
	`"00 74 00 20 00 65 00 78"`,
	`"00 61 00 6D 00 70 00 6C"`,
	`"00 65 00 2E 00 20 00"`,
}

var clobWriteRPAGoldenPayload = []string{
	`"00 26 00 01 82"`,
	`"08 80 03 00 02 29 25 00"`,
	`"00 01 17 00 00 00 01 03"`,
	`"69 00 0A 00 00 00 01 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"01 00 00 02 02 58 04 01"`,
	`"01 02 20 0F 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 08"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00"`,
}

// Read golden payloads captured from a representative TTC trace.
var clobReadMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"72 00 00 00 00 00 00 00"`,
	`"01 02 00 00 01 06 00 01"`,
	`"00 00 00 00 00 00 00 70"`,
	`"00 02 02 0C 82 00 00 02"`,
	`"00 00 00 01 00 00 00 1C"`,
	`"5B 61 00 01 38 4F 00 01"`,
	`"38 4E 00 03 00 03 03 69"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 06 69 8A 07 E1 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 49 2E 32 09 00 00"`,
	`"00 00 00 00 DE AD BE EF"`,
	`"00 01 00 22 00 00 00 00"`,
	`"00 26 EF E2 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 01"`,
	`"38 4E 00 40 8B 27 00 00"`,
	`"02 7F 0B"`,
}

var clobReadResponseGoldenPayload = []string{
	`"0E FE 02 04 A6 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 54 00"`,
	`"68 00 69 00 73 00 20 00"`,
	`"69 00 73 00 20 00 61 00"`,
	`"20 00 6C 00 61 00 72 00"`,
	`"67 00 65 00 20 00 74 00"`,
	`"65 00 78 00 74 00 20 00"`,
	`"65 00 78 00 61 00 6D 00"`,
	`"70 00 6C 00 65 00 2E 00"`,
	`"20 00 54 00 68 00 69 00"`,
	`"73 00 20 00 69 00 73 00"`,
	`"20 00 61 00 20 00 6C 00"`,
	`"61 00 72 00 67 00 65 00"`,
	`"20 00 74 00 65 00 78 00"`,
	`"74 00 20 00 65 00 78 00"`,
	`"61 00 6D 00 70 00 6C 00"`,
	`"65 00 2E 00 20 00 08 00"`,
	`"70 00 02 02 0C 82 00 00"`,
	`"02 00 00 00 01 00 00 00"`,
	`"1C 5B 61 00 01 38 4F 00"`,
	`"01 38 4E 00 03 00 03 03"`,
	`"69 00 00 00 00 00 00 00"`,
	`"00 00 06 69 8A 07 E1 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 49 2E 32 09 00"`,
	`"00 00 00 00 00 DE AD BE"`,
	`"EF 00 01 00 22 00 00 00"`,
	`"00 00 26 EF E2 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"01 38 4E 00 40 8B 27 00"`,
	`"00 02 02 53 04 01 01 02"`,
	`"20 13 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 03 01"`,
	`"38 4E 01 01 00 02 8B 27"`,
	`"00 00 00 0C 00 00 00 00"`,
	`"00 00 00 00 00 00"`,
}

var clobReadLocator = driverCommon.B1Array{
	0, 112, 0, 2, 2, 12, 130, 0, 0, 2, 0, 0, 0, 1, 0, 0,
	0, 28, 91, 97, 0, 1, 56, 79, 0, 1, 56, 78, 0, 3, 0, 3,
	3, 105, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 105, 138, 7, 225,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 73, 46, 50, 9,
	0, 0, 0, 0, 0, 0, 222, 173, 190, 239, 0, 1, 0, 34, 0, 0,
	0, 0, 0, 38, 239, 226, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 1, 56, 78, 0, 64, 139, 39, 0, 0,
}

var clobIsOpenSourceLocator = driverCommon.B1Array{
	0, 112, 0, 2, 2, 12, 130, 0, 0, 2, 0, 0, 0, 1, 0, 0,
	0, 28, 140, 153, 0, 1, 59, 41, 0, 1, 59, 40, 0, 3, 0, 3,
	3, 105, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 105, 139, 23, 90,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 73, 46, 50, 9,
	0, 0, 0, 0, 0, 0, 222, 173, 190, 239, 0, 1, 0, 34, 0, 0,
	0, 0, 0, 40, 151, 233, 0, 2, 0, 22, 0, 0, 11, 162, 0, 64,
	0, 30, 6, 26, 24, 0, 0, 0, 0, 1, 59, 40, 0, 64, 143, 79,
	0, 0,
}

var clobIsOpenMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"72 00 00 00 00 00 00 01"`,
	`"03 01 10 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"70 00 02 02 0C 82 00 00"`,
	`"02 00 00 00 01 00 00 00"`,
	`"1C 8C 99 00 01 3B 29 00"`,
	`"01 3B 28 00 03 00 03 03"`,
	`"69 00 00 00 00 00 00 00"`,
	`"00 00 06 69 8B 17 5A 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 49 2E 32 09 00"`,
	`"00 00 00 00 00 DE AD BE"`,
	`"EF 00 01 00 22 00 00 00"`,
	`"00 00 28 97 E9 00 02 00"`,
	`"16 00 00 0B A2 00 40 00"`,
	`"1E 06 1A 18 00 00 00 00"`,
	`"01 3B 28 00 40 8F 4F 00"`,
	`"00"`,
}

var clobIsOpenRPAGoldenPayload = []string{
	`"00 70 00 02 02"`,
	`"0C 82 00 00 02 00 00 00"`,
	`"01 00 00 00 1C 8C 99 00"`,
	`"01 3B 29 00 01 3B 28 00"`,
	`"03 00 03 03 69 00 00 00"`,
	`"00 00 00 00 00 00 06 69"`,
	`"8B 17 5A 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 49"`,
	`"2E 32 09 00 00 00 00 00"`,
	`"00 DE AD BE EF 00 01 00"`,
	`"22 00 00 00 00 00 28 97"`,
	`"E9 00 02 00 16 00 00 0B"`,
	`"A2 00 40 00 1E 06 1A 18"`,
	`"00 00 00 00 01 3B 28 00"`,
	`"40 8F 4F 00 00 00 04 01"`,
	`"02 02 22 3B 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"03 01 3B 28 01 01 00 02"`,
	`"8F 4F 00 00 00 10 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
}

var clobGetLengthSourceLocator = driverCommon.B1Array{
	0, 112, 0, 2, 2, 12, 130, 0, 0, 2, 0, 0, 0, 1, 0, 0,
	0, 28, 140, 153, 0, 1, 59, 41, 0, 1, 59, 40, 0, 3, 0, 3,
	3, 105, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 105, 139, 23, 90,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 73, 46, 50, 9,
	0, 0, 0, 0, 0, 0, 222, 173, 190, 239, 0, 1, 0, 34, 0, 0,
	0, 0, 0, 40, 151, 233, 0, 2, 0, 22, 0, 0, 11, 162, 0, 64,
	0, 30, 6, 26, 24, 0, 0, 0, 0, 1, 59, 40, 0, 64, 143, 79,
	0, 0,
}

var clobGetLengthMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"72 00 00 00 00 00 00 00"`,
	`"01 01 00 00 00 00 01 00"`,
	`"00 00 00 00 00 00 70 00"`,
	`"02 02 0C 82 00 00 02 00"`,
	`"00 00 01 00 00 00 1C 8C"`,
	`"99 00 01 3B 29 00 01 3B"`,
	`"28 00 03 00 03 03 69 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"06 69 8B 17 5A 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"00 00 00 DE AD BE EF 00"`,
	`"01 00 22 00 00 00 00 00"`,
	`"28 97 E9 00 02 00 16 00"`,
	`"00 0B A2 00 40 00 1E 06"`,
	`"1A 18 00 00 00 00 01 3B"`,
	`"28 00 40 8F 4F 00 00 00"`,
}

var clobGetLengthRPAGoldenPayload = []string{
	`"00 70 00 02 02"`,
	`"0C 82 00 00 02 00 00 00"`,
	`"01 00 00 00 1C 8C 99 00"`,
	`"01 3B 29 00 01 3B 28 00"`,
	`"03 00 03 03 69 00 00 00"`,
	`"00 00 00 00 00 00 06 69"`,
	`"8B 17 5A 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 49"`,
	`"2E 32 09 00 00 00 00 00"`,
	`"00 DE AD BE EF 00 01 00"`,
	`"22 00 00 00 00 00 28 97"`,
	`"E9 00 02 00 16 00 00 0B"`,
	`"A2 00 40 00 1E 06 1A 18"`,
	`"00 00 00 00 01 3B 28 00"`,
	`"40 8F 4F 00 00 01 1E 04"`,
	`"01 02 02 22 39 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 03 01 3B 28 01 01 00"`,
	`"02 8F 4F 00 00 00 0E 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00"`,
}

var clobTrimSourceLocator = driverCommon.B1Array{
	0, 112, 0, 2, 2, 12, 130, 0, 0, 2, 0, 0, 0, 1, 0, 0,
	0, 28, 140, 153, 0, 1, 59, 41, 0, 1, 59, 40, 0, 3, 0, 3,
	3, 105, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 105, 139, 23, 90,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 73, 46, 50, 9,
	0, 0, 0, 0, 0, 0, 222, 173, 190, 239, 0, 1, 0, 34, 0, 0,
	0, 0, 0, 40, 151, 231, 0, 2, 0, 22, 0, 0, 11, 162, 0, 64,
	0, 30, 6, 26, 23, 0, 0, 0, 0, 1, 59, 40, 0, 64, 143, 79,
	0, 0,
}

var clobTrimMarshalGoldenPayload = []string{
	`"03 60 01 00 01 01"`,
	`"72 00 00 00 00 00 00 00"`,
	`"01 20 00 00 00 00 01 00"`,
	`"00 00 00 00 00 00 70 00"`,
	`"02 02 0C 82 00 00 02 00"`,
	`"00 00 01 00 00 00 1C 8C"`,
	`"99 00 01 3B 29 00 01 3B"`,
	`"28 00 03 00 03 03 69 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"06 69 8B 17 5A 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"00 00 00 DE AD BE EF 00"`,
	`"01 00 22 00 00 00 00 00"`,
	`"28 97 E7 00 02 00 16 00"`,
	`"00 0B A2 00 40 00 1E 06"`,
	`"1A 17 00 00 00 00 01 3B"`,
	`"28 00 40 8F 4F 00 00 01"`,
	`"1E"`,
}

var clobTrimRPAGoldenPayload = []string{
	`"00 70 00 02 02"`,
	`"0C 82 00 00 02 00 00 00"`,
	`"01 00 00 00 1C 8C 99 00"`,
	`"01 3B 29 00 01 3B 28 00"`,
	`"03 00 03 03 69 00 00 00"`,
	`"00 00 00 00 00 00 06 69"`,
	`"8B 17 5A 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 49"`,
	`"2E 32 09 00 00 00 00 00"`,
	`"00 DE AD BE EF 00 01 00"`,
	`"22 00 00 00 00 00 28 97"`,
	`"E9 00 02 00 16 00 00 0B"`,
	`"A2 00 40 00 1E 06 1A 18"`,
	`"00 00 00 00 01 3B 28 00"`,
	`"40 8F 4F 00 00 01 1E 04"`,
	`"01 02 02 22 38 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 03 01 3B 28 01 01 00"`,
	`"02 8F 4F 00 00 00 0D 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00"`,
}
