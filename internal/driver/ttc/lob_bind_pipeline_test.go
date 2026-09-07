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
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// recordingBlobBindWriter records bounded chunks and locator offsets.
type recordingBlobBindWriter struct {
	// chunks contains the payloads presented to each simulated write.
	chunks [][]byte
	// offsets contains the locator offset used for each simulated write.
	offsets []driverCommon.UB8
	// short makes the simulated server acknowledge fewer bytes than requested.
	short bool
}

// failIfReadReader records accidental source consumption during bind preflight.
type failIfReadReader struct {
	// read is set when the pipeline consumes the source.
	read bool
}

// Read marks the source consumed and fails immediately.
func (reader *failIfReadReader) Read([]byte) (int, error) {
	reader.read = true
	return 0, errors.New("reader must not be consumed")
}

// zeroProgressLobReader models a broken source that cannot make progress.
type zeroProgressLobReader struct{}

// Read reports no data and no error so the bind path must stop safely.
func (*zeroProgressLobReader) Read([]byte) (int, error) { return 0, nil }

// configureStreamInputFixture stages deterministic RPA/OER pairs for temporary
// LOB creation, streaming, and optional zero-length trimming.
func configureStreamInputFixture(streamer *fakeStreamer, amounts []driverCommon.UB8) {
	streamer.events = nil
	streamer.lobRpaAmounts = nil
	for _, amount := range amounts {
		streamer.events = append(streamer.events, newTTILobRPA(), &mockOer{})
		streamer.lobRpaAmounts = append(streamer.lobRpaAmounts, amount)
	}
	streamer.onFlush = func() {
		if streamer.definition.operation == kplobTmpCreate {
			streamer.definition.sourceLocator.locatorBytes = append(
				driverCommon.B1Array(nil),
				newTestLobReferenceLocator(151).locatorBytes...,
			)
		}
	}
}

// write records one simulated BLOB write.
func (writer *recordingBlobBindWriter) write(_ context.Context, loc *locator, payload driverCommon.B1Array) (driverCommon.UB8, error) {
	writer.offsets = append(writer.offsets, loc.offset)
	writer.chunks = append(writer.chunks, append([]byte(nil), payload...))
	if writer.short && len(payload) > 0 {
		return driverCommon.UB8(len(payload) - 1), nil
	}
	return driverCommon.UB8(len(payload)), nil
}

// recordingClobBindWriter records rune chunks and Oracle logical offsets.
type recordingClobBindWriter struct {
	// chunks contains the rune text presented to each simulated write.
	chunks []string
	// offsets contains the locator offset used for each simulated write.
	offsets []driverCommon.UB8
	// short makes the simulated server acknowledge fewer logical units than requested.
	short bool
}

// logicalAmount returns the simulated Oracle logical amount.
func (*recordingClobBindWriter) logicalAmount(runes []rune) (driverCommon.UB8, error) {
	return driverCommon.UB8(lobCharacterUnits(runes)), nil
}

// write records one simulated CLOB or NCLOB write.
func (writer *recordingClobBindWriter) write(_ context.Context, loc *locator, _ bool, runes []rune) (driverCommon.UB8, error) {
	writer.offsets = append(writer.offsets, loc.offset)
	writer.chunks = append(writer.chunks, string(runes))
	amount, _ := writer.logicalAmount(runes)
	if writer.short && amount > 0 {
		amount--
	}
	return amount, nil
}

// TestLobBindPipeline_StreamBlobInputIsBoundedAndHonorsDeclaredSize verifies
// that BLOB input is bounded by its declared size.
func TestLobBindPipeline_StreamBlobInputIsBoundedAndHonorsDeclaredSize(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte("x"), internallob.DefaultBlobLobChunkBytes+7)
	source := bytes.NewBuffer(append(append([]byte(nil), payload...), 'z'))
	writer := &recordingBlobBindWriter{}
	loc := newLocator(driverCommon.B1Array("locator"), 1)
	input := internallob.NewInput(source, internallob.BLOB, int64(len(payload)))

	if _, err := streamBlobInput(context.Background(), writer.write, loc, input); err != nil {
		t.Fatalf("streamBlobInput returned error: %v", err)
	}
	if len(writer.chunks) != 2 || len(writer.chunks[0]) != internallob.DefaultBlobLobChunkBytes || len(writer.chunks[1]) != 7 {
		t.Fatalf("chunk lengths = [%d %d], want [%d 7]", len(writer.chunks[0]), len(writer.chunks[1]), internallob.DefaultBlobLobChunkBytes)
	}
	if len(writer.offsets) != 2 || writer.offsets[0] != 1 || writer.offsets[1] != driverCommon.UB8(internallob.DefaultBlobLobChunkBytes+1) {
		t.Fatalf("offsets = %v, want [1 %d]", writer.offsets, internallob.DefaultBlobLobChunkBytes+1)
	}
	remaining, err := io.ReadAll(source)
	if err != nil || string(remaining) != "z" {
		t.Fatalf("bytes after declared size = %q, %v; want z", remaining, err)
	}
}

// TestLobBindPipeline_NormalizeLobBindInputsConvertsMarkersToInputs verifies that every
// public LOB marker enters the existing streamed-LOB bind path rather than a
// LOB-specific executor or OAC registry path.
func TestLobBindPipeline_NormalizeLobBindInputsConvertsMarkersToInputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		value   any
		kind    internallob.Kind
		payload []byte
	}{
		{name: "BLOB", value: internallob.BindBlob{1, 2, 3}, kind: internallob.BLOB, payload: []byte{1, 2, 3}},
		{name: "CLOB", value: internallob.BindClob("CLOB text"), kind: internallob.CLOB, payload: []byte("CLOB text")},
		{name: "NCLOB", value: internallob.BindNClob("NCLOB text"), kind: internallob.NCLOB, payload: []byte("NCLOB text")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			args := []driver.NamedValue{
				{Ordinal: 1, Value: testCase.value},
				{Ordinal: 2, Value: int64(7)},
				{Ordinal: 3, Value: testCase.value},
			}

			normalized := normalizeLobBindInputs(args)
			if &normalized[0] == &args[0] {
				t.Fatal("normalization returned the original argument slice")
			}
			input, ok := normalized[0].Value.(internallob.Input)
			if !ok {
				t.Fatalf("normalized LOB type = %T, want internallob.Input", normalized[0].Value)
			}
			if input.Kind() != testCase.kind || input.Size() != int64(len(testCase.payload)) {
				t.Fatalf("normalized LOB metadata = (%v, %d), want (%v, %d)", input.Kind(), input.Size(), testCase.kind, len(testCase.payload))
			}
			if got, err := io.ReadAll(input.Reader()); err != nil || !bytes.Equal(got, testCase.payload) {
				t.Fatalf("normalized LOB reader = %v, %v; want %v, nil", got, err, testCase.payload)
			}
			if _, ok := normalized[1].Value.(int64); !ok {
				t.Fatalf("non-LOB argument type = %T, want int64", normalized[1].Value)
			}
			if _, ok := normalized[2].Value.(internallob.Input); !ok {
				t.Fatalf("later normalized LOB type = %T, want internallob.Input", normalized[2].Value)
			}
		})
	}
}

// TestLobBindPipeline_NormalizeLobBindInputsLeavesOrdinaryValuesUntouched verifies ordinary
// binds avoid an unnecessary argument-slice allocation.
func TestLobBindPipeline_NormalizeLobBindInputsLeavesOrdinaryValuesUntouched(t *testing.T) {
	t.Parallel()

	args := []driver.NamedValue{{Ordinal: 1, Value: int64(7)}}
	normalized := normalizeLobBindInputs(args)
	if &normalized[0] != &args[0] {
		t.Fatal("ordinary bind arguments were unnecessarily copied")
	}
}

// TestLobBindPipeline_CheckNamedValueAcceptsLOBMarkers verifies that database/sql does not
// flatten an explicit LOB marker before bind normalization.
func TestLobBindPipeline_CheckNamedValueAcceptsLOBMarkers(t *testing.T) {
	t.Parallel()

	for _, value := range []any{internallob.BindBlob{1, 2, 3}, internallob.BindClob("CLOB"), internallob.BindNClob("NCLOB")} {
		if err := checkNamedValue(&driver.NamedValue{Ordinal: 1, Value: value}); err != nil {
			t.Fatalf("checkNamedValue(%T) returned error: %v", value, err)
		}
	}
}

// TestLobBindPipeline_NormalizeAndValidateInputsBeforeReading verifies all
// streamed inputs are validated before any reader is consumed.
func TestLobBindPipeline_NormalizeAndValidateInputsBeforeReading(t *testing.T) {
	t.Parallel()

	firstSource := &failIfReadReader{}
	args := []driver.NamedValue{
		{Ordinal: 1, Value: internallob.NewInput(firstSource, internallob.BLOB, 1)},
		{Ordinal: 2, Value: internallob.NewInput(nil, internallob.CLOB, 0)},
	}
	_, _, err := normalizeAndValidateStreamedLobInputs(args)
	if err == nil {
		t.Fatal("prepareStreamedLobBinds unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLobInput)
	}
	if firstSource.read {
		t.Fatal("first LOB source was consumed before all inputs were validated")
	}
}

// TestLobBindPipeline_IsTerminalOracleError verifies terminal Oracle errors are
// distinguished from wrapped and transport errors.
func TestLobBindPipeline_IsTerminalOracleError(t *testing.T) {
	t.Parallel()

	serverErr := common.NewOracleError(oracleErrors.ErrorCode("ORA-00001"), nil)
	if !isTerminalOracleError(serverErr) {
		t.Fatal("direct terminal ORA error was not recognized")
	}
	wrapped := common.NewOracleError(oracleErrors.RunExecError, serverErr, "execute")
	if isTerminalOracleError(wrapped) {
		t.Fatal("wrapped error was incorrectly accepted as a direct terminal response")
	}
	if isTerminalOracleError(common.NewOracleError(oracleErrors.StreamerReadError, io.ErrUnexpectedEOF)) {
		t.Fatal("transport EOF was classified as a synchronized TTC exchange")
	}
}

// TestLobBindPipeline_StreamBlobInputRejectsShortSourceAndAcknowledgement
// verifies short sources and short acknowledgements are rejected.
func TestLobBindPipeline_StreamBlobInputRejectsShortSourceAndAcknowledgement(t *testing.T) {
	t.Parallel()

	loc := newLocator(driverCommon.B1Array("locator"), 1)
	disposition, err := streamBlobInput(
		context.Background(),
		(&recordingBlobBindWriter{}).write,
		loc,
		internallob.NewInput(bytes.NewReader([]byte("abc")), internallob.BLOB, 4),
	)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short source error = %v, want ErrUnexpectedEOF", err)
	}
	requireErrorCode(t, err, oracleErrors.InvalidLobInput)
	if disposition != lobCleanupFreeNow {
		t.Fatalf("short source disposition = %v, want free now", disposition)
	}

	disposition, err = streamBlobInput(
		context.Background(),
		(&recordingBlobBindWriter{short: true}).write,
		loc,
		internallob.NewInput(bytes.NewReader([]byte("abc")), internallob.BLOB, 3),
	)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short acknowledgement error = %v, want ErrShortWrite", err)
	}
	requireErrorCode(t, err, oracleErrors.InvalidLobInput)
	if disposition != lobCleanupFreeNow {
		t.Fatalf("short acknowledgement disposition = %v, want free now", disposition)
	}
}

// TestLobBindPipeline_StreamBlobInputAbandonsCleanupAfterAmbiguousWriteFailure
// verifies ambiguous write failures abandon cleanup on the unsafe session.
func TestLobBindPipeline_StreamBlobInputAbandonsCleanupAfterAmbiguousWriteFailure(t *testing.T) {
	t.Parallel()

	disposition, err := streamBlobInput(
		context.Background(),
		func(context.Context, *locator, driverCommon.B1Array) (driverCommon.UB8, error) {
			return 0, errors.New("write transport failure")
		},
		newLocator(driverCommon.B1Array("locator"), 1),
		internallob.NewInput(bytes.NewReader([]byte("abc")), internallob.BLOB, 3),
	)
	if err == nil {
		t.Fatal("ambiguous write unexpectedly succeeded")
	}
	if disposition != lobCleanupAbandon {
		t.Fatalf("ambiguous write disposition = %v, want abandon", disposition)
	}
}

// TestLobBindPipeline_StreamCLOBAndNCLOBInputUseUCS2AmountsAndOffsets verifies
// character binds use Oracle UCS-2 amounts and offsets.
func TestLobBindPipeline_StreamCLOBAndNCLOBInputUseUCS2AmountsAndOffsets(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		kind internallob.Kind
	}{
		{name: "CLOB", kind: internallob.CLOB},
		{name: "NCLOB", kind: internallob.NCLOB},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &recordingClobBindWriter{}
			loc := newLocator(driverCommon.B1Array("locator"), 1)
			payload := []byte("A🙂")
			source := bytes.NewBuffer(append(append([]byte(nil), payload...), 'z'))
			input := internallob.NewInput(source, tc.kind, int64(len(payload)))
			if _, err := streamClobInput(context.Background(), func(ctx context.Context, loc *locator, isNClob bool, runes []rune) (driverCommon.UB8, error) {
				return writer.write(ctx, loc, isNClob, runes)
			}, writer.logicalAmount, loc, input, tc.kind == internallob.NCLOB); err != nil {
				t.Fatalf("streamClobInput returned error: %v", err)
			}
			if len(writer.chunks) != 1 || writer.chunks[0] != "A🙂" {
				t.Fatalf("chunks = %q, want A🙂", writer.chunks)
			}
			if len(writer.offsets) != 1 || writer.offsets[0] != 1 || loc.offset != 4 {
				t.Fatalf("offsets = %v final=%d, want [1] final=4", writer.offsets, loc.offset)
			}
			if _, err := writer.write(context.Background(), loc, tc.kind == internallob.NCLOB, []rune("B")); err != nil {
				t.Fatalf("following write returned error: %v", err)
			}
			if len(writer.offsets) != 2 || writer.offsets[1] != 4 {
				t.Fatalf("following write offset = %v, want [1 4]", writer.offsets)
			}
			remaining, err := io.ReadAll(source)
			if err != nil || string(remaining) != "z" {
				t.Fatalf("bytes after declared size = %q, %v; want z", remaining, err)
			}
		})
	}
}

// TestLobBindPipeline_StreamClobInputRejectsMalformedUTF8 verifies malformed
// UTF-8 is rejected before a character bind is written.
func TestLobBindPipeline_StreamClobInputRejectsMalformedUTF8(t *testing.T) {
	t.Parallel()

	_, err := streamClobInput(
		context.Background(),
		func(ctx context.Context, loc *locator, isNClob bool, runes []rune) (driverCommon.UB8, error) {
			return (&recordingClobBindWriter{}).write(ctx, loc, isNClob, runes)
		},
		(&recordingClobBindWriter{}).logicalAmount,
		newLocator(driverCommon.B1Array("locator"), 1),
		internallob.NewInput(bytes.NewReader([]byte{0xf0, 0x9f}), internallob.CLOB, 2),
		false,
	)
	if err == nil {
		t.Fatal("malformed UTF-8 input unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLobInput)
	}
}

// TestLobBindPipeline_StreamClobInputHandlesBoundaries verifies empty input,
// exact chunk flushing, short sources, and write failures.
func TestLobBindPipeline_StreamClobInputHandlesBoundaries(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("character write failed")
	cases := []struct {
		name       string
		payload    []byte
		size       int64
		write      func(context.Context, *locator, bool, []rune) (driverCommon.UB8, error)
		wantErr    error
		wantChunks int
	}{
		// An empty declared source needs no server write.
		{name: "empty input", size: 0, wantChunks: 0},
		// A complete application-sized chunk is flushed before EOF is read.
		{name: "full rune chunk", payload: []byte(strings.Repeat("a", internallob.DefaultCharacterLobChunkChars)), size: int64(internallob.DefaultCharacterLobChunkChars), wantChunks: 1},
		// EOF before the declared byte count is an input error.
		{name: "short source", payload: []byte("a"), size: 2, wantErr: io.ErrUnexpectedEOF},
		// A write failure leaves the TTC stream boundary to the caller's policy.
		{name: "write error", payload: []byte("a"), size: 1, write: func(context.Context, *locator, bool, []rune) (driverCommon.UB8, error) { return 0, writeErr }, wantErr: writeErr},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			writer := &recordingClobBindWriter{}
			write := test.write
			if write == nil {
				write = writer.write
			}
			_, err := streamClobInput(context.Background(), write, writer.logicalAmount, newLocator(driverCommon.B1Array("locator"), 1), internallob.NewInput(bytes.NewReader(test.payload), internallob.CLOB, test.size), false)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("streamClobInput returned error: %v", err)
				}
			} else if !errors.Is(err, test.wantErr) {
				t.Fatalf("streamClobInput error = %v, want %v", err, test.wantErr)
			}
			if len(writer.chunks) != test.wantChunks {
				t.Fatalf("written chunks = %d, want %d", len(writer.chunks), test.wantChunks)
			}
		})
	}
}

// TestLobBindPipeline_EncodeLobLocatorBindUsesLobOAC verifies locator binds
// carry LOB-specific OAC metadata.
func TestLobBindPipeline_EncodeLobLocatorBindUsesLobOAC(t *testing.T) {
	t.Parallel()

	encoded, marshalled, err := encodeLobLocatorBind(lobLocatorBind{
		locator:     driverCommon.B1Array("locator"),
		kind:        internallob.NCLOB,
		charsetForm: FormNChar,
		charsetID:   al16Utf16CharSet,
	})
	if err != nil {
		t.Fatalf("encodeLobLocatorBind returned error: %v", err)
	}
	oac := marshalled.(*tTIoac)
	if string(encoded) != "locator" || DtyType(oac.dataType) != DtyClob || oac.characterSetForm != FormNChar || oac.characterSetID != al16Utf16CharSet {
		t.Fatalf("encoded=%q oac=%+v, want NCLOB locator metadata", encoded, oac)
	}
	if _, _, err := encodeLobLocatorBind(lobLocatorBind{}); err == nil {
		t.Fatal("encodeLobLocatorBind accepted an empty locator")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLobInput)
	}
}

// TestLobBindPipeline_EnqueueLobFreeDefersAndInvalidatesLocator
// verifies temporary frees are queued and local locators are invalidated.
func TestLobBindPipeline_EnqueueLobFreeDefersAndInvalidatesLocator(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)
	loc := newTestLobReferenceLocator(101)
	loc.locatorBytes[koll1FlagOffset] |= kolblAbstractLocatorFlag
	loc.locatorBytes[koll4FlagOffset] |= kolblOpenFlagByte | kolblReadWriteFlagByte

	if err := shelf.enqueueLobFree(loc); err != nil {
		t.Fatalf("enqueueLobFree returned error: %v", err)
	}
	if streamer.outgoingMessages.Len() != 0 {
		t.Fatal("enqueueLobFree sent a standalone TTC request")
	}
	if loc.locatorBytes[koll1FlagOffset]&kolblAbstractLocatorFlag != 0 ||
		loc.locatorBytes[koll2FlagOffset]&kolblInitializedFlag != 0 ||
		loc.locatorBytes[koll4FlagOffset]&(kolblTemporaryFlagByte|kolblOpenFlagByte|kolblReadWriteFlagByte) != 0 {
		t.Fatalf("locator flags were not cleared: %v", loc.locatorBytes)
	}
	if err := shelf.enqueueLobFree(loc); err == nil {
		t.Fatal("second enqueue accepted an already-freed locator")
	}
	if err := streamer.Push(context.Background(), &lobRegistryOrdinaryFunction{}); err != nil {
		t.Fatalf("Push ordinary function: %v", err)
	}
	if streamer.outgoingMessages.Len() != 2 {
		t.Fatalf("outgoing message count = %d, want piggyback plus ordinary function", streamer.outgoingMessages.Len())
	}
}

// TestLobBindPipeline_CanceledLobExchangeUsesBreakResetAndRestoresStream
// verifies cancellation recovery restores a reusable TTC stream.
func TestLobBindPipeline_CanceledLobExchangeUsesBreakResetAndRestoresStream(t *testing.T) {
	t.Parallel()

	shelf := newShelf[driverCommon.MessageType]()
	cancelCalled := make(chan struct{}, 1)
	shelf.registerCancelExecution(func(context.Context) error {
		cancelCalled <- struct{}{}
		return nil
	})
	parent, cancelParent := context.WithCancel(context.Background())
	pulls := 0
	streamer := &fakeStreamer{}
	streamer.pullHook = func(ctx context.Context, _ ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
		pulls++
		if pulls == 1 {
			cancelParent()
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &mockOer{}, nil
	}
	shelf.RegisterMessageStreamer(streamer)
	operationContext, _, cleanup := shelf.cancellation.newCancelableOperationContext(parent, shelf.cancelExecution)
	defer cleanup()
	executor := newLobExecutor(shelf.Shelf)

	err := executor.execute(operationContext, newLobDefinitionForGetLengthOperation(newLocator(driverCommon.B1Array("locator"), 1)))
	if !isCompletedLobResponseError(err) {
		t.Fatalf("execute error = %v, want completed cancellation error", err)
	}
	if pulls != 2 {
		t.Fatalf("Pull count = %d, want canceled pull plus terminal TTIOER pull", pulls)
	}
	select {
	case <-cancelCalled:
	default:
		t.Fatal("break/reset callback was not invoked")
	}
}

// TestLobBindPipeline_CanceledLobExchangeDiscardsStreamWhenRecoveryFails verifies that a LOB
// cancellation remains unsafe unless break/reset also consumes its terminal
// response.
func TestLobBindPipeline_CanceledLobExchangeDiscardsStreamWhenRecoveryFails(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		breakErr    error
		terminalErr error
	}{
		{name: "break reset failure", breakErr: errors.New("break/reset failed")},
		{name: "terminal pull failure", terminalErr: errors.New("terminal response unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			shelf := newShelf[driverCommon.MessageType]()
			shelf.registerCancelExecution(func(context.Context) error { return test.breakErr })
			parent, cancelParent := context.WithCancel(context.Background())
			streamer := &fakeStreamer{}
			pulls := 0
			streamer.pullHook = func(ctx context.Context, _ ...driverCommon.MessageType) (driverCommon.Message[driverCommon.MessageType], error) {
				pulls++
				if pulls == 1 {
					cancelParent()
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return nil, test.terminalErr
			}
			shelf.RegisterMessageStreamer(streamer)
			operationContext, _, cleanup := shelf.cancellation.newCancelableOperationContext(parent, shelf.cancelExecution)
			defer cleanup()
			executor := newLobExecutor(shelf.Shelf)

			err := executor.execute(operationContext, newLobDefinitionForGetLengthOperation(newLocator(driverCommon.B1Array("locator"), 1)))
			if isCompletedLobResponseError(err) {
				t.Fatalf("execute error = %v, unexpectedly marked as completed", err)
			}
			if !_shouldDiscardLobSession(err) {
				t.Fatalf("execute error = %v, want session discard", err)
			}
			if test.breakErr != nil && pulls != 1 {
				t.Fatalf("Pull count = %d, want 1 when break/reset fails", pulls)
			}
			if test.terminalErr != nil && pulls != 2 {
				t.Fatalf("Pull count = %d, want canceled pull plus terminal pull", pulls)
			}
		})
	}
}

// TestLobBindPipeline_PreparedLobBindsLifecycle verifies successful registration,
// idempotent cleanup, invalid registration, and session abandonment.
func TestLobBindPipeline_PreparedLobBindsLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("add and free", func(t *testing.T) {
		rows := newLobTestRows()
		manager, _ := newTestStreamedLobManager(t, rows)
		cleanup := &preparedLobBinds{manager: manager}
		loc := newTestLobReferenceLocator(141)
		if err := cleanup.add(loc); err != nil {
			t.Fatalf("add returned error: %v", err)
		}
		if err := cleanup.free(); err != nil {
			t.Fatalf("free returned error: %v", err)
		}
		if err := cleanup.free(); err != nil {
			t.Fatalf("idempotent free returned error: %v", err)
		}
		if !cleanup.freed || loc.isTemporaryLocator() {
			t.Fatalf("cleanup state = freed:%t temporary:%t, want freed and released", cleanup.freed, loc.isTemporaryLocator())
		}
	})

	t.Run("invalid locator", func(t *testing.T) {
		rows := newLobTestRows()
		manager, _ := newTestStreamedLobManager(t, rows)
		cleanup := &preparedLobBinds{manager: manager}
		if err := cleanup.add(newLocator(nil, 1)); err == nil {
			t.Fatal("add accepted an incomplete locator")
		} else {
			requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
		}
		if len(cleanup.leases) != 0 {
			t.Fatalf("leases after rejected add = %d, want 0", len(cleanup.leases))
		}
	})

	t.Run("failed free abandons session", func(t *testing.T) {
		rows := newLobTestRows()
		manager, _ := newTestStreamedLobManager(t, rows)
		cleanup := &preparedLobBinds{
			manager: manager,
			// A lease owned by another registry models a cleanup ownership failure.
			leases: []*lobReferenceLease{{registry: newLobReferenceRegistry()}},
		}
		if err := cleanup.free(); err == nil {
			t.Fatal("free unexpectedly succeeded with a mismatched lease")
		} else {
			requireErrorCode(t, err, oracleErrors.InternalError)
		}
		if !rows.shelf.lobState.isInvalidated() {
			t.Fatal("failed free did not abandon the LOB session")
		}
	})

	t.Run("nil and empty cleanup", func(t *testing.T) {
		var nilCleanup *preparedLobBinds
		if err := nilCleanup.free(); err != nil {
			t.Fatalf("nil free returned error: %v", err)
		}
		nilCleanup.abandon()

		rows := newLobTestRows()
		manager, _ := newTestStreamedLobManager(t, rows)
		empty := &preparedLobBinds{manager: manager}
		if err := empty.free(); err != nil {
			t.Fatalf("empty free returned error: %v", err)
		}
		empty.abandon()
		if !rows.shelf.lobState.isInvalidated() {
			t.Fatal("empty abandon did not invalidate the LOB session")
		}
	})
}

// TestLobBindPipeline_CleanupDispositionClassifiesRPCResults verifies that
// only a completed terminal response permits immediate temporary-LOB cleanup.
func TestLobBindPipeline_CleanupDispositionClassifiesRPCResults(t *testing.T) {
	t.Parallel()

	completed := &completedLobResponseError{err: errors.New("terminal response consumed")}
	for _, test := range []struct {
		name string
		err  error
		want lobCleanupDisposition
	}{
		{name: "completed response", err: completed, want: lobCleanupFreeNow},
		{name: "transport error", err: errors.New("stream boundary unknown"), want: lobCleanupAbandon},
		{name: "nil error", want: lobCleanupAbandon},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A nil or ordinary error carries no proof that the TTC response was consumed.
			if got := lobCleanupAfterRPC(test.err); got != test.want {
				t.Fatalf("cleanup disposition = %v, want %v", got, test.want)
			}
		})
	}
}

// TestLobBindPipeline_PrepareAndRunHelpers verifies no-input preparation,
// cancellation-aware RPC wrapping, and creation failure classification.
func TestLobBindPipeline_PrepareAndRunHelpers(t *testing.T) {
	t.Parallel()

	rows := newLobTestRows()
	args := []driver.NamedValue{{Ordinal: 1, Value: int64(7)}}
	prepared, cleanup, err := prepareStreamedLobBinds(context.Background(), rows.shelf, newTestSessionContext(), args, -1)
	if err != nil || cleanup != nil || &prepared[0] != &args[0] {
		t.Fatalf("no-input preparation = (%v, %v, %v), want original args and no cleanup", prepared, cleanup, err)
	}
	if _, _, err := prepareStreamedLobBinds(context.Background(), nil, newTestSessionContext(), args, 0); err == nil {
		t.Fatal("preparation accepted a missing TTC session")
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLobInput)
	}

	for _, test := range []struct {
		name string
		want error
	}{
		{name: "success", want: nil},
		{name: "callback error", want: errors.New("callback failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := runCancelableLobRPC[string](context.Background(), rows.shelf, func(context.Context) (string, error) {
				return "result", test.want
			})
			if got != "result" || !errors.Is(err, test.want) {
				t.Fatalf("runCancelableLobRPC = (%q, %v), want (result, %v)", got, err, test.want)
			}
		})
	}

	for _, test := range []struct {
		name string
		kind internallob.Kind
	}{
		{name: "BLOB", kind: internallob.BLOB},
		{name: "CLOB", kind: internallob.CLOB},
		{name: "NCLOB", kind: internallob.NCLOB},
	} {
		t.Run(test.name, func(t *testing.T) {
			localRows := newLobTestRows()
			localManager, streamer := newTestStreamedLobManager(t, localRows)
			streamer.pushErr = errors.New("create push failed")
			input := internallob.NewInput(bytes.NewReader([]byte("x")), test.kind, 1)
			_, loc, disposition, err := localManager.streamInput(context.Background(), input)
			if err == nil || loc != nil || disposition != lobCleanupAbandon {
				t.Fatalf("streamInput result = (loc:%v, disposition:%v, err:%v), want creation failure with abandon", loc, disposition, err)
			}
			requireErrorCode(t, err, oracleErrors.LobExecError)
		})
	}

	// An ordinary argument is skipped before the canceled streamed input is prepared.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	localRows := newLobTestRows()
	localManager, streamer := newTestStreamedLobManager(t, localRows)
	streamer.pushErr = errors.New("prepare push failed")
	args = []driver.NamedValue{
		{Ordinal: 1, Value: int64(7)},
		{Ordinal: 2, Value: internallob.NewInput(bytes.NewReader([]byte("x")), internallob.BLOB, 1)},
	}
	if _, _, err := localManager.prepareBinds(ctx, args, 0); err == nil {
		t.Fatal("prepareBinds unexpectedly succeeded after streamed creation failure")
	}
	if !localRows.shelf.lobState.isInvalidated() {
		t.Fatal("prepareBinds failure did not abandon the LOB session")
	}

	for _, test := range []struct {
		name       string
		kind       internallob.Kind
		payload    []byte
		amounts    []driverCommon.UB8
		wantOffset driverCommon.UB8
	}{
		// Non-empty BLOB input exercises create followed by a binary write.
		{name: "BLOB data", kind: internallob.BLOB, payload: []byte("x"), amounts: []driverCommon.UB8{0, 1}, wantOffset: 2},
		// Non-empty CLOB input exercises UTF-8 decoding and a character write.
		{name: "CLOB data", kind: internallob.CLOB, payload: []byte("x"), amounts: []driverCommon.UB8{0, 1}, wantOffset: 2},
		// Non-empty NCLOB input uses the national-character bind metadata.
		{name: "NCLOB data", kind: internallob.NCLOB, payload: []byte("x"), amounts: []driverCommon.UB8{0, 1}, wantOffset: 2},
		// Empty BLOB input is made non-NULL by a zero-length trim.
		{name: "empty BLOB", kind: internallob.BLOB, amounts: []driverCommon.UB8{0, 0, 0}, wantOffset: 1},
		// Empty CLOB input follows the same non-NULL preservation rule.
		{name: "empty CLOB", kind: internallob.CLOB, amounts: []driverCommon.UB8{0, 0, 0}, wantOffset: 1},
		// Empty NCLOB input also uses a zero-length trim in UTF-16 units.
		{name: "empty NCLOB", kind: internallob.NCLOB, amounts: []driverCommon.UB8{0, 0, 0}, wantOffset: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			manager, streamer := newTestStreamedLobManager(t, rows)
			configureStreamInputFixture(streamer, test.amounts)
			input := internallob.NewInput(bytes.NewReader(test.payload), test.kind, int64(len(test.payload)))
			bind, loc, disposition, err := manager.streamInput(context.Background(), input)
			if err != nil || loc == nil || disposition != lobCleanupFreeNow {
				t.Fatalf("streamInput result = (bind:%+v, loc:%v, disposition:%v, err:%v), want success", bind, loc, disposition, err)
			}
			if bind.kind != test.kind || loc.offset != test.wantOffset || !bytes.Equal(bind.locator, loc.locatorBytes) {
				t.Fatalf("streamInput metadata = (kind:%v, offset:%d, locator:% X), want (kind:%v, offset:%d, matching locator)", bind.kind, loc.offset, bind.locator, test.kind, test.wantOffset)
			}
		})
	}

	// Invalid kinds are rejected before any temporary LOB is created.
	invalidRows := newLobTestRows()
	invalidManager, _ := newTestStreamedLobManager(t, invalidRows)
	_, loc, disposition, err := invalidManager.streamInput(context.Background(), internallob.NewInput(bytes.NewReader([]byte("x")), internallob.Kind(99), 1))
	if err == nil || loc != nil || disposition != lobCleanupFreeNow {
		t.Fatalf("invalid-kind streamInput result = (loc:%v, disposition:%v, err:%v), want InvalidLobInput", loc, disposition, err)
	}
	requireErrorCode(t, err, oracleErrors.InvalidLobInput)

	// A successful prepare replaces only streamed values and returns their cleanup owner.
	preparedRows := newLobTestRows()
	_, preparedStreamer := newTestStreamedLobManager(t, preparedRows)
	configureStreamInputFixture(preparedStreamer, []driverCommon.UB8{0, 1})
	args = []driver.NamedValue{
		{Ordinal: 1, Value: int64(7)},
		{Ordinal: 2, Value: internallob.NewInput(bytes.NewReader([]byte("x")), internallob.BLOB, 1)},
	}
	prepared, preparedCleanup, err := prepareStreamedLobBinds(context.Background(), preparedRows.shelf, newTestSessionContext(), args, 1)
	if err != nil || preparedCleanup == nil {
		t.Fatalf("successful preparation = (%v, %v), want prepared args and cleanup", prepared, err)
	}
	if _, ok := prepared[1].Value.(lobLocatorBind); !ok {
		t.Fatalf("prepared streamed value = %T, want lobLocatorBind", prepared[1].Value)
	}
	if err := preparedCleanup.free(); err != nil {
		t.Fatalf("successful preparation cleanup returned error: %v", err)
	}
}

// TestLobBindPipeline_StreamBlobInputRejectsSourceFailures verifies source
// cancellation, read errors, and no-progress readers are rejected safely.
func TestLobBindPipeline_StreamBlobInputRejectsSourceFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		ctx    func() context.Context
		reader io.Reader
	}{
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			reader: bytes.NewReader([]byte("x")),
		},
		{name: "source read error", ctx: context.Background, reader: &failIfReadReader{}},
		{name: "no progress", ctx: context.Background, reader: &zeroProgressLobReader{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := streamBlobInput(test.ctx(), func(context.Context, *locator, driverCommon.B1Array) (driverCommon.UB8, error) {
				t.Fatal("source failure reached the write callback")
				return 0, nil
			}, newLocator(driverCommon.B1Array("locator"), 1), internallob.NewInput(test.reader, internallob.BLOB, 1))
			if err == nil {
				t.Fatal("streamBlobInput unexpectedly succeeded")
			}
			requireErrorCode(t, err, oracleErrors.InvalidLobInput)
		})
	}

}

// TestLobBindPipeline_StreamClobInputRejectsSourceAndAcknowledgementFailures
// verifies character conversion, source, translation, and write failures.
func TestLobBindPipeline_StreamClobInputRejectsSourceAndAcknowledgementFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		ctx           func() context.Context
		reader        io.Reader
		logicalAmount func([]rune) (driverCommon.UB8, error)
		write         func(context.Context, *locator, bool, []rune) (driverCommon.UB8, error)
	}{
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			reader: bytes.NewReader([]byte("x")),
		},
		{name: "source read error", ctx: context.Background, reader: &failIfReadReader{}},
		{
			name:   "logical amount error",
			ctx:    context.Background,
			reader: bytes.NewReader([]byte("x")),
			logicalAmount: func([]rune) (driverCommon.UB8, error) {
				return 0, errors.New("cannot calculate logical amount")
			},
		},
		{
			name:   "short acknowledgement",
			ctx:    context.Background,
			reader: bytes.NewReader([]byte("x")),
			write: func(context.Context, *locator, bool, []rune) (driverCommon.UB8, error) {
				return 0, nil
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logicalAmount := test.logicalAmount
			if logicalAmount == nil {
				logicalAmount = func(runes []rune) (driverCommon.UB8, error) {
					return driverCommon.UB8(lobCharacterUnits(runes)), nil
				}
			}
			write := test.write
			if write == nil {
				write = func(context.Context, *locator, bool, []rune) (driverCommon.UB8, error) {
					t.Fatal("source failure reached the write callback")
					return 0, nil
				}
			}
			_, err := streamClobInput(test.ctx(), write, logicalAmount, newLocator(driverCommon.B1Array("locator"), 1), internallob.NewInput(test.reader, internallob.CLOB, 1), false)
			if err == nil {
				t.Fatal("streamClobInput unexpectedly succeeded")
			}
			requireErrorCode(t, err, oracleErrors.InvalidLobInput)
		})
	}
}
