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
	"strings"
	"sync"
	"testing"
	"time"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// lobEventRecorder records event delivery for tests in this package.
type lobEventRecorder struct {
	events []eventType
}

// streamedLobWriteFunc adapts a small write callback for WriterTo tests.
type streamedLobWriteFunc func([]byte) (int, error)

// admissionCancellationContext cancels between state validation and operation
// admission so callers can exercise the narrow cancellation race deterministically.
type admissionCancellationContext struct {
	checked chan struct{}
	done    chan struct{}
	once    sync.Once
}

// Deadline implements context.Context for admissionCancellationContext.
func (*admissionCancellationContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

// Done implements context.Context for admissionCancellationContext.
func (ctx *admissionCancellationContext) Done() <-chan struct{} { return ctx.done }

// Err returns nil for the initial state check and waits for cancellation on the
// subsequent operation-admission check.
func (ctx *admissionCancellationContext) Err() error {
	initial := false
	ctx.once.Do(func() {
		initial = true
		close(ctx.checked)
	})
	if initial {
		return nil
	}
	<-ctx.done
	return context.Canceled
}

// Value implements context.Context for admissionCancellationContext.
func (*admissionCancellationContext) Value(any) any { return nil }

// Write delegates one WriterTo write to the configured test behavior.
func (writer streamedLobWriteFunc) Write(data []byte) (int, error) {
	return writer(data)
}

// notify implements EventListener.
func (listener *lobEventRecorder) notify(event eventType) {
	listener.events = append(listener.events, event)
}

// newLobTestRows constructs the minimum live Rows owner needed by a streamed LOB.
func newLobTestRows() *ttcRows {
	rows := newTTCRows(nil)
	rows.shelf = newShelf[driverCommon.MessageType]()
	rows.setContext(context.Background())
	return rows
}

// newTestStreamedLobManager constructs a concrete LOB manager backed by the
// deterministic TTC fixture factory and streamer used by executor tests.
func newTestStreamedLobManager(t *testing.T, rows *ttcRows) (*lobManager, *fakeStreamer) {
	t.Helper()
	fixtureShelf, _, _ := newLobTestShelf(8192)
	rowsShelf := rows.shelf
	if rowsShelf == nil {
		t.Fatal("streamed LOB test owner has no TTC shelf")
	}
	rowsShelf.RegisterMessageFactory(fixtureShelf.GetMessageFactory())
	streamer := &fakeStreamer{
		events:    make([]driverCommon.Message[driverCommon.MessageType], 0),
		preHooks:  make(map[driverCommon.MessageType]StreamerPreUnmarshallCallback),
		postHooks: make(map[driverCommon.MessageType]StreamerPostUnmarshallCallback),
	}
	rowsShelf.RegisterMessageStreamer(streamer)
	manager, err := newLobManager(rowsShelf, newTestSessionContext())
	if err != nil {
		t.Fatalf("newLobManager: %v", err)
	}
	return manager, streamer
}

// appendTestLobResponse appends one deterministic OLOBOPS response to the
// fake stream. payload is a raw BLOB or encoded CLOB response, while logical
// is the server-reported Oracle amount.
func appendTestLobResponse(streamer *fakeStreamer, payload []byte, logical driverCommon.UB8, terminalErr error) {
	if payload != nil {
		streamer.events = append(streamer.events, newTTIlobd())
		streamer.lobdPayloads = append(streamer.lobdPayloads, payload)
	}
	streamer.events = append(streamer.events, newTTILobRPA(), &mockOer{err: terminalErr})
	streamer.lobRpaAmounts = append(streamer.lobRpaAmounts, logical)
}

func mustRegisterLob(t *testing.T, rows *ttcRows, value *streamedLob) {
	t.Helper()
	if !rows.registerLob(value) {
		t.Fatal("registerLob rejected live Rows")
	}
}

// TestStreamedLob_BoundedClobReadAmountUsesCharacterChunk verifies that CLOB
// refill amounts are capped in Oracle character units.
func TestStreamedLob_BoundedClobReadAmountUsesCharacterChunk(t *testing.T) {
	t.Parallel()

	wantMaximum := driverCommon.UB8(DefaultCharacterLobChunkChars)
	if got := boundedClobReadAmount(wantMaximum * 2); got != wantMaximum {
		t.Fatalf("bounded amount = %d, want %d", got, wantMaximum)
	}
	if got := boundedClobReadAmount(3); got != 3 {
		t.Fatalf("small bounded amount = %d, want 3", got)
	}
}

func newTestStreamedBlob(t *testing.T, rows *ttcRows, length driverCommon.UB8, prefix []byte) (*streamedLob, *fakeStreamer) {
	manager, streamer := newTestStreamedLobManager(t, rows)
	return &streamedLob{
		owner:       rows,
		kind:        internallob.BLOB,
		manager:     manager,
		locator:     newLocator(newTestLocator(false), 1),
		totalLength: length,
		prefix:      prefix,
		nextOffset:  driverCommon.UB8(len(prefix) + 1),
	}, streamer
}

// newTestStreamedClob constructs a CLOB-shaped LOB for lifecycle and
// refill tests backed by the deterministic TTC fixture streamer.
func newTestStreamedClob(t *testing.T, rows *ttcRows, length driverCommon.UB8, prefix []byte) (*streamedLob, *fakeStreamer) {
	manager, streamer := newTestStreamedLobManager(t, rows)
	return &streamedLob{
		owner:       rows,
		kind:        internallob.CLOB,
		manager:     manager,
		locator:     newLocator(newTestLocator(false), 1),
		totalLength: length,
		prefix:      prefix,
		nextOffset:  1,
	}, streamer
}

// TestStreamedLob_ValueConsumesPrefixThenUsesBoundedOffsets verifies that
// inline BLOB data is consumed before a length-bounded locator refill.
func TestStreamedLob_ValueConsumesPrefixThenUsesBoundedOffsets(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	value, streamer := newTestStreamedBlob(t, rows, 6, []byte("ab"))
	var offset driverCommon.UB8
	var amount driverCommon.UB8
	streamer.onPush = func(def *lobDefinition) {
		offset = def.sourceLocator.offset
		amount = def.lobAmt
	}
	appendTestLobResponse(streamer, []byte("cdef"), 4, nil)
	mustRegisterLob(t, rows, value)

	first := make([]byte, 1)
	if n, err := value.Read(first); n != 1 || err != nil || string(first) != "a" {
		t.Fatalf("first Read = (%d, %v, %q), want (1, nil, a)", n, err, first)
	}
	if offset != 0 {
		t.Fatalf("prefix Read performed an RPC at offset %d, want no RPC", offset)
	}
	var rest bytes.Buffer
	if _, err := value.WriteTo(&rest); err != nil || rest.String() != "bcdef" {
		t.Fatalf("WriteTo = (%q, %v), want (bcdef, nil)", rest.String(), err)
	}
	if offset != 3 || amount != 4 {
		t.Fatalf("RPC request = (offset %d, amount %d), want (3, 4)", offset, amount)
	}
	if len(rows.lifecycle.lobs) != 0 {
		t.Fatalf("outstanding LOB count = %d after EOF, want 0", len(rows.lifecycle.lobs))
	}
}

// TestStreamedLob_AdoptionRejectsConsumedData verifies that a streamed
// LOB cannot be promoted after its inline prefix or refill has been consumed.
func TestStreamedLob_AdoptionRejectsConsumedData(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		length    driverCommon.UB8
		prefix    []byte
		refill    []byte
		logical   driverCommon.UB8
		readBytes int
	}{
		{
			name:      "consumed inline prefix",
			length:    3,
			prefix:    []byte("abc"),
			readBytes: 3,
		},
		{
			name:      "consumed refill",
			length:    3,
			refill:    []byte("abc"),
			logical:   3,
			readBytes: 3,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			value, streamer := newTestStreamedBlob(t, rows, test.length, test.prefix)
			if test.refill != nil {
				appendTestLobResponse(streamer, test.refill, test.logical, nil)
			}
			mustRegisterLob(t, rows, value)

			if n, err := value.Read(make([]byte, test.readBytes)); n != test.readBytes || err != nil {
				t.Fatalf("Read = (%d, %v), want (%d, nil)", n, err, test.readBytes)
			}
			if _, err := value.detachPersistentState(rows.shelf); err == nil {
				t.Fatal("detachPersistentState succeeded after data was consumed")
			} else {
				requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
			}
		})
	}
}

// TestStreamedLob_DetachAllowsUnreadPrefetch verifies that prefetched
// data does not prevent promotion when the caller has not read from the LOB.
func TestStreamedLob_DetachAllowsUnreadPrefetch(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	value, streamer := newTestStreamedBlob(t, rows, 3, []byte("abc"))
	mustRegisterLob(t, rows, value)

	state, err := value.detachPersistentState(rows.shelf)
	if err != nil {
		t.Fatalf("detachPersistentState returned error: %v", err)
	}
	if len(state.Locator) == 0 {
		t.Fatal("detachPersistentState returned an empty locator")
	}
	if string(state.Prefix) != "abc" || state.FetchOffset != 4 || !state.PrefixComplete {
		t.Fatalf("detached prefix state = (%q, offset %d, complete %t), want (abc, 4, true)", state.Prefix, state.FetchOffset, state.PrefixComplete)
	}
	if len(streamer.pushed) != 0 {
		t.Fatalf("promotion issued %d TTC exchanges, want 0", len(streamer.pushed))
	}
	if len(rows.lifecycle.lobs) != 0 {
		t.Fatalf("outstanding LOB count = %d after promotion, want 0", len(rows.lifecycle.lobs))
	}
}

// TestStreamedLob_AdoptTransfersPrefetchState verifies typed-Lob promotion
// preserves public prefix bytes and resumes Oracle reads in logical units.
func TestStreamedLob_AdoptTransfersPrefetchState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		dtype         DtyType
		form          driverCommon.UB1
		variableWidth bool
		prefix        []byte
		length        driverCommon.UB8
		wantPrefix    string
		wantOffset    uint64
		wantComplete  bool
	}{
		{
			name:       "BLOB partial prefix",
			dtype:      DtyBlob,
			prefix:     []byte("abc"),
			length:     6,
			wantPrefix: "abc",
			wantOffset: 4,
		},
		{
			name:         "BLOB complete prefix",
			dtype:        DtyBlob,
			prefix:       []byte("abc"),
			length:       3,
			wantPrefix:   "abc",
			wantOffset:   4,
			wantComplete: true,
		},
		{
			name:         "CLOB unicode logical units",
			dtype:        DtyClob,
			form:         FormChar,
			prefix:       []byte("A🙂"),
			length:       3,
			wantPrefix:   "A🙂",
			wantOffset:   4,
			wantComplete: true,
		},
		{
			name:         "NCLOB utf16 logical units",
			dtype:        DtyClob,
			form:         FormNChar,
			prefix:       []byte{0xD8, 0x3D, 0xDE, 0x42},
			length:       2,
			wantPrefix:   "🙂",
			wantOffset:   3,
			wantComplete: true,
		},
		{
			name:          "CLOB trailing high surrogate",
			dtype:         DtyClob,
			form:          FormChar,
			variableWidth: true,
			prefix:        []byte{0xD8, 0x3D},
			length:        2,
			wantOffset:    1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			rows.sessionContext = newTestSessionContext()
			t.Cleanup(func() { _ = rows.Close() })
			locator := newTestLocator(test.variableWidth)
			value, err := newStreamedLob(rows, test.dtype, test.prefix, lobColumnContext{
				CharsetForm:    test.form,
				TotalLobLength: test.length,
				LobLocator:     locator,
			})
			if err != nil {
				t.Fatalf("newStreamedLob returned error: %v", err)
			}
			mustRegisterLob(t, rows, value)

			state, session, err := value.AdoptPersistentLocator(value.kind)
			if err != nil {
				t.Fatalf("AdoptPersistentLocator returned error: %v", err)
			}
			if session == nil {
				t.Fatal("AdoptPersistentLocator returned nil session capability")
			}
			if string(state.Prefix) != test.wantPrefix {
				t.Errorf("transferred prefix = %q, want %q", state.Prefix, test.wantPrefix)
			}
			if state.FetchOffset != test.wantOffset {
				t.Errorf("transferred fetch offset = %d, want %d", state.FetchOffset, test.wantOffset)
			}
			if state.PrefixComplete != test.wantComplete {
				t.Errorf("transferred PrefixComplete = %t, want %t", state.PrefixComplete, test.wantComplete)
			}
			if len(state.Locator) == 0 {
				t.Error("AdoptPersistentLocator returned an empty locator")
			}
			if len(rows.lifecycle.lobs) != 0 {
				t.Errorf("Rows retained %d LOBs after promotion, want none", len(rows.lifecycle.lobs))
			}
		})
	}
}

// TestStreamedLob_InlinePrefixCannotExceedDeclaredLength verifies that equal
// inline prefixes are accepted while oversized BLOB, CLOB, and NCLOB prefixes
// are rejected before the value becomes usable.
func TestStreamedLob_InlinePrefixCannotExceedDeclaredLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dtype  DtyType
		form   driverCommon.UB1
		prefix []byte
		length driverCommon.UB8
		wantOK bool
	}{
		{name: "BLOB equal", dtype: DtyBlob, prefix: []byte("abc"), length: 3, wantOK: true},
		{name: "BLOB overflow", dtype: DtyBlob, prefix: []byte("abc"), length: 2},
		{name: "CLOB equal", dtype: DtyClob, form: FormChar, prefix: []byte("A🙂"), length: 3, wantOK: true},
		{name: "CLOB overflow", dtype: DtyClob, form: FormChar, prefix: []byte("A🙂"), length: 2},
		{name: "NCLOB equal", dtype: DtyClob, form: FormNChar, prefix: []byte{0xD8, 0x3D, 0xDE, 0x42}, length: 2, wantOK: true},
		{name: "NCLOB overflow", dtype: DtyClob, form: FormNChar, prefix: []byte{0xD8, 0x3D, 0xDE, 0x42}, length: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			rows.sessionContext = driverCommon.NewSessionContext()
			rows.sessionContext.SetSessionCharacterSets(al32Utf8CharSet, al16Utf16CharSet)
			locator := make(driverCommon.B1Array, koll4FlagOffset+1)
			value, err := newStreamedLob(rows, test.dtype, test.prefix, lobColumnContext{
				CharsetForm:    test.form,
				TotalLobLength: test.length,
				LobLocator:     locator,
			})
			if test.wantOK {
				if err != nil {
					t.Fatalf("newStreamedLob returned error: %v", err)
				}
				if value == nil {
					t.Fatal("newStreamedLob returned nil value")
				}
				return
			}
			if value != nil {
				t.Fatal("newStreamedLob returned a value for an oversized prefix")
			}
			requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
		})
	}
}

// TestStreamedLob_ClobReadUsesDeclaredLengthAndStopsAtEOF verifies that CLOB
// requests use UTF-16 length units and that the final refill does not trigger
// an extra end-of-value locator RPC.
func TestStreamedLob_ClobReadUsesDeclaredLengthAndStopsAtEOF(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	var requested driverCommon.UB8
	value, streamer := newTestStreamedClob(t, rows, 2, nil)
	streamer.onPush = func(def *lobDefinition) { requested = def.lobAmt }
	appendTestLobResponse(streamer, []byte("🙂"), 2, nil)
	mustRegisterLob(t, rows, value)

	dst := make([]byte, len("🙂"))
	if n, err := value.Read(dst); n != len(dst) || err != nil || string(dst) != "🙂" {
		t.Fatalf("Read = (%d, %v, %q), want (%d, nil, 🙂)", n, err, dst, len(dst))
	}
	if requested != 2 {
		t.Fatalf("CLOB request amount = %d, want 2 UTF-16 units", requested)
	}
	if _, err := value.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("final Read error = %v, want io.EOF", err)
	}
	if len(streamer.pushed) != 1 {
		t.Fatalf("CLOB RPC count = %d, want 1", len(streamer.pushed))
	}
}

// TestStreamedLob_RefillRejectsInconsistentPayload verifies that a successful
// refill must contain both payload bytes and logical units within its request.
func TestStreamedLob_RefillRejectsInconsistentPayload(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   func(*testing.T, *ttcRows) (*streamedLob, *fakeStreamer)
		payload []byte
		logical driverCommon.UB8
	}{
		{
			name: "empty payload with nonzero amount",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 2, nil)
			},
			logical: 1,
		},
		{
			name: "nonempty payload with zero amount",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 2, nil)
			},
			payload: []byte("a"),
		},
		{
			name: "zero progress before declared end",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 2, nil)
			},
		},
		{
			name: "logical amount exceeds CLOB request",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 2, nil)
			},
			payload: []byte("a"),
			logical: 3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			value, streamer := test.value(t, rows)
			appendTestLobResponse(streamer, test.payload, test.logical, nil)
			mustRegisterLob(t, rows, value)

			n, err := value.Read(make([]byte, 1))
			if err == nil {
				t.Fatalf("inconsistent refill unexpectedly succeeded: n=%d definition=%+v", n, streamer.definition)
			} else {
				requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
			}
			rpcCount := len(streamer.pushed)
			if len(rows.lifecycle.lobs) != 0 {
				t.Fatalf("outstanding LOB count = %d after invalid refill, want 0", len(rows.lifecycle.lobs))
			}
			if _, err := value.Read(make([]byte, 1)); err == nil {
				t.Fatal("invalidated LOB unexpectedly remained readable")
			} else {
				requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
			}
			if len(streamer.pushed) != rpcCount {
				t.Fatalf("read after invalid refill sent another TTC exchange: got %d messages, want %d", len(streamer.pushed), rpcCount)
			}
		})
	}
}

// TestStreamedLob_ReadCancellationBeforeRPCInvalidatesValue verifies that a
// canceled query context prevents a locator exchange from starting.
func TestStreamedLob_ReadCancellationBeforeRPCInvalidatesValue(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows.setContext(ctx)
	value, streamer := newTestStreamedBlob(t, rows, 1, nil)
	mustRegisterLob(t, rows, value)
	if _, err := value.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	if len(streamer.pushed) != 0 {
		t.Fatal("canceled Read performed an RPC")
	}
}

// TestStreamedLob_AdmissionCancellationInvalidatesOperations verifies that a
// context canceled after state validation but before TTC admission invalidates
// Read, Size, and ChunkSize without sending an exchange.
func TestStreamedLob_AdmissionCancellationInvalidatesOperations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value func(*testing.T, *ttcRows) (*streamedLob, *fakeStreamer)
		call  func(*streamedLob) error
	}{
		// Read must fail closed when cancellation wins before locator admission.
		{
			name: "Read",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 1, nil)
			},
			call: func(value *streamedLob) error {
				_, err := value.Read(make([]byte, 1))
				return err
			},
		},
		// Size must not issue a metadata request after admission is canceled.
		{
			name: "Size",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 1, nil)
			},
			call: func(value *streamedLob) error {
				_, err := value.Size()
				return err
			},
		},
		// ChunkSize follows the same admission and invalidation contract.
		{
			name: "ChunkSize",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 1, nil)
			},
			call: func(value *streamedLob) error {
				_, err := value.ChunkSize()
				return err
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			admission := &admissionCancellationContext{
				checked: make(chan struct{}),
				done:    make(chan struct{}),
			}
			rows.mu.Lock()
			rows.lifecycle.ctx = admission
			rows.mu.Unlock()
			value, streamer := test.value(t, rows)
			mustRegisterLob(t, rows, value)

			result := make(chan error, 1)
			go func() { result <- test.call(value) }()
			select {
			case <-admission.checked:
			case err := <-result:
				t.Fatalf("operation completed before admission cancellation: %v", err)
			}
			close(admission.done)

			if err := <-result; err == nil {
				t.Fatal("operation unexpectedly succeeded after admission cancellation")
			} else {
				requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
			}
			if len(streamer.pushed) != 0 {
				t.Fatalf("canceled operation sent %d TTC messages, want 0", len(streamer.pushed))
			}
		})
	}
}

// TestStreamedLob_RowsCloseInvalidatesLob verifies that Rows.Close invalidates
// an unread locator-backed value before later reads can issue RPCs.
func TestStreamedLob_RowsCloseInvalidatesLob(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	value, streamer := newTestStreamedBlob(t, rows, 7, nil)
	mustRegisterLob(t, rows, value)
	if err := rows.Close(); err != nil {
		t.Fatalf("Rows.Close returned error: %v", err)
	}
	if _, err := value.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read after Rows.Close unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	if len(streamer.pushed) != 0 {
		t.Fatal("Read after Rows.Close performed an RPC")
	}
}

// TestStreamedLob_ConnectionInvalidationRejectsBeforeRPC verifies that a
// query LOB rejects reads after its physical connection is invalidated without
// sending a TTC message.
func TestStreamedLob_ConnectionInvalidationRejectsBeforeRPC(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	value, streamer := newTestStreamedBlob(t, rows, 1, nil)
	mustRegisterLob(t, rows, value)

	conn := &connection{shelf: rows.shelf, _isValid: true}
	conn.invalidate()

	if _, err := value.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read after connection invalidation unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	if len(streamer.pushed) != 0 {
		t.Fatal("Read after connection invalidation performed an RPC")
	}
}

// TestStreamedLob_RowsCloseDoesNotWaitForStalledLobRead verifies that Rows.Close
// remains non-blocking while a locator RPC ignores cancellation.
func TestStreamedLob_RowsCloseDoesNotWaitForStalledLobRead(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	entered, release := make(chan struct{}), make(chan struct{})
	value, streamer := newTestStreamedBlob(t, rows, 1, nil)
	streamer.onFlush = func() {
		close(entered)
		<-release
	}
	streamer.flushErr = errors.New("transport stopped")
	mustRegisterLob(t, rows, value)
	readDone := make(chan error, 1)
	go func() { _, err := value.Read(make([]byte, 1)); readDone <- err }()
	<-entered
	if err := rows.Close(); err != nil {
		t.Fatalf("Rows.Close returned error: %v", err)
	}
	close(release)
	if err := <-readDone; err == nil {
		t.Fatal("in-flight Read unexpectedly succeeded")
	}
}

// TestStreamedLob_InFlightSuccessAfterRowsCloseIsRejected verifies that a
// context-ignoring successful Read, Size, or ChunkSize cannot publish a result
// after Rows.Close has invalidated the owning value.
func TestStreamedLob_InFlightSuccessAfterRowsCloseIsRejected(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		value  func(*testing.T, *ttcRows) (*streamedLob, *fakeStreamer)
		setup  func(*fakeStreamer)
		invoke func(*streamedLob) (int64, error)
		result int64
	}{
		{
			name: "Read",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 1, nil)
			},
			setup: func(streamer *fakeStreamer) {
				appendTestLobResponse(streamer, []byte("l"), 1, nil)
			},
			invoke: func(value *streamedLob) (int64, error) {
				n, err := value.Read(make([]byte, 1))
				return int64(n), err
			},
		},
		{
			name: "Size",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 1, nil)
			},
			setup: func(streamer *fakeStreamer) {
				appendTestLobResponse(streamer, nil, 1, nil)
			},
			invoke: func(value *streamedLob) (int64, error) {
				return value.Size()
			},
		},
		{
			name: "ChunkSize",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 1, nil)
			},
			setup: func(streamer *fakeStreamer) {
				appendTestLobResponse(streamer, nil, 1, nil)
			},
			invoke: func(value *streamedLob) (int64, error) {
				return value.ChunkSize()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			entered, release := make(chan struct{}), make(chan struct{})
			value, streamer := test.value(t, rows)
			test.setup(streamer)
			streamer.onFlush = func() {
				close(entered)
				<-release
			}
			mustRegisterLob(t, rows, value)
			operationDone := make(chan struct {
				result int64
				err    error
			}, 1)
			go func() {
				result, err := test.invoke(value)
				operationDone <- struct {
					result int64
					err    error
				}{result: result, err: err}
			}()
			<-entered

			if err := rows.Close(); err != nil {
				t.Fatalf("Rows.Close returned error: %v", err)
			}
			close(release)
			result := <-operationDone
			if result.result != test.result {
				t.Fatalf("operation result = %d, want %d", result.result, test.result)
			}
			requireErrorCode(t, result.err, oracleErrors.LobValueInvalidated)
		})
	}
}

// TestStreamedLob_RPCFailuresApplySessionSafetyPolicy verifies streamed LOB
// errors conservatively mark the physical session stale without an explicit
// response-completion marker.
func TestStreamedLob_RPCFailuresApplySessionSafetyPolicy(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
	}{
		{name: "unsafe transport"},
		{name: "terminal Oracle error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			recorder := &lobEventRecorder{}
			rows.shelf.getEventService().register(recorder, streamerStaleEvent)
			value, streamer := newTestStreamedBlob(t, rows, 1, nil)
			if test.name == "terminal Oracle error" {
				appendTestLobResponse(streamer, nil, 0, errors.New("server rejected locator operation"))
			} else {
				streamer.pullErr = errors.New("transport failed")
			}
			mustRegisterLob(t, rows, value)
			if _, err := value.Read(make([]byte, 1)); err == nil {
				t.Fatal("Read returned nil error")
			}
			if !rows.isClosed() {
				t.Fatal("Rows remained open after a LOB RPC error")
			}
			if len(recorder.events) != 1 {
				t.Fatalf("stale events = %v", recorder.events)
			}
		})
	}
}

// TestStreamedLob_MetadataRPCFailuresApplySessionSafetyPolicy verifies that
// Size and ChunkSize conservatively invalidate the owning Rows after any RPC error.
func TestStreamedLob_MetadataRPCFailuresApplySessionSafetyPolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		call       func(*streamedLob) error
		wantClosed bool
	}{
		{
			name:       "Size terminal Oracle error",
			wantClosed: true,
			call: func(value *streamedLob) error {
				_, err := value.Size()
				return err
			},
		},
		{
			name:       "Size unsafe transport",
			wantClosed: true,
			call: func(value *streamedLob) error {
				_, err := value.Size()
				return err
			},
		},
		{
			name:       "ChunkSize terminal Oracle error",
			wantClosed: true,
			call: func(value *streamedLob) error {
				_, err := value.ChunkSize()
				return err
			},
		},
		{
			name:       "ChunkSize unsafe transport",
			wantClosed: true,
			call: func(value *streamedLob) error {
				_, err := value.ChunkSize()
				return err
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			value, streamer := newTestStreamedClob(t, rows, 1, nil)
			if strings.Contains(test.name, "terminal Oracle error") {
				appendTestLobResponse(streamer, nil, 0, errors.New("server rejected metadata request"))
			} else {
				streamer.pullErr = errors.New("metadata transport failed")
			}
			mustRegisterLob(t, rows, value)

			if err := test.call(value); err == nil {
				t.Fatal("metadata operation unexpectedly succeeded")
			}
			if rows.isClosed() != test.wantClosed {
				t.Fatalf("Rows closed = %t, want %t", rows.isClosed(), test.wantClosed)
			}
			if len(rows.lifecycle.lobs) != 0 {
				t.Fatalf("outstanding LOB count = %d, want 0", len(rows.lifecycle.lobs))
			}
		})
	}
}

// TestStreamedLob_CloseReleasesRowsOwnershipAndIsIdempotent verifies local
// close behavior and removal from the Rows LOB registry.
func TestStreamedLob_CloseReleasesRowsOwnershipAndIsIdempotent(t *testing.T) {
	t.Parallel()
	rows := newLobTestRows()
	value, _ := newTestStreamedBlob(t, rows, 1, nil)
	mustRegisterLob(t, rows, value)
	if err := value.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
	if _, err := value.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read after Close unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueClosed)
	}
	if _, err := value.ChunkSize(); err == nil {
		t.Fatal("ChunkSize after Close unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueClosed)
	}
	if _, err := value.Size(); err == nil {
		t.Fatal("Size after Close unexpectedly succeeded")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueClosed)
	}
	if len(rows.lifecycle.lobs) != 0 {
		t.Fatalf("outstanding LOB count = %d after Close, want 0", len(rows.lifecycle.lobs))
	}
}

// TestStreamedLob_ClobPrefixConversionUsesCorrectLogicalUnits verifies that
// text prefixes are decoded to UTF-8 while offsets count UTF-16 code units.
func TestStreamedLob_ClobPrefixConversionUsesCorrectLogicalUnits(t *testing.T) {
	t.Parallel()

	session := driverCommon.NewSessionContext()
	session.SetSessionCharacterSets(al32Utf8CharSet, al16Utf16CharSet)
	executor := newClobExecutor(newShelf[driverCommon.MessageType]().Shelf, session)
	loc := newLocator(make(driverCommon.B1Array, koll4FlagOffset+1), 1)

	payload, logical, _, err := executor.decodeReadPayload(loc, false, []byte("A🙂"), 0, false)
	if err != nil {
		t.Fatalf("CLOB decodePrefix returned error: %v", err)
	}
	if !bytes.Equal(payload, []byte("A🙂")) || logical != 3 {
		t.Fatalf("CLOB prefix = (%q, %d), want (A🙂, 3 UTF-16 units)", payload, logical)
	}

	payload, logical, _, err = executor.decodeReadPayload(loc, true, []byte{0xD8, 0x3D, 0xDE, 0x42}, 0, false)
	if err != nil {
		t.Fatalf("NCLOB decodePrefix returned error: %v", err)
	}
	if !bytes.Equal(payload, []byte("🙂")) || logical != 2 {
		t.Fatalf("NCLOB prefix = (%q, %d), want (🙂, 2 UTF-16 units)", payload, logical)
	}
}

// TestStreamedLob_JoinsSurrogateAcrossPrefetchBoundary reproduces the JDBC
// behavior where a prefetched high surrogate is followed by its low surrogate
// in the first locator response.
func TestStreamedLob_JoinsSurrogateAcrossPrefetchBoundary(t *testing.T) {
	t.Parallel()

	rows := newLobTestRows()
	rows.sessionContext = driverCommon.NewSessionContext()
	rows.sessionContext.SetSessionCharacterSets(al32Utf8CharSet, al16Utf16CharSet)
	_, streamer := newTestStreamedLobManager(t, rows)

	value, err := newStreamedLob(rows, DtyClob, []byte{0xD8, 0x3D}, lobColumnContext{
		CharsetForm:    FormChar,
		TotalLobLength: 2,
		LobLocator:     newTestLocator(true),
	})
	if err != nil {
		t.Fatalf("newStreamedLob returned error: %v", err)
	}
	appendTestLobResponse(streamer, []byte{0xDE, 0x42}, 1, nil)
	mustRegisterLob(t, rows, value)

	got, err := io.ReadAll(value)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(got) != "🙂" {
		t.Fatalf("ReadAll = %q, want 🙂", got)
	}
	if value.pendingHighSurrogate != 0 {
		t.Fatal("surrogate carry remained after locator read")
	}
}

// TestStreamedLob_CompletesSurrogatePastReadLimit verifies CLOB decoding can
// consume one extra UTF-16 unit when a requested chunk ends on a high surrogate.
func TestStreamedLob_CompletesSurrogatePastReadLimit(t *testing.T) {
	t.Parallel()

	rows := newLobTestRows()
	chunkUnits := DefaultCharacterLobChunkChars
	value, streamer := newTestStreamedClob(t, rows, driverCommon.UB8(chunkUnits+1), nil)
	value.locator = newLocator(newTestLocator(true), 1)
	var offsets []driverCommon.UB8
	streamer.onPush = func(definition *lobDefinition) {
		offsets = append(offsets, definition.sourceLocator.offset)
	}
	firstPayload := bytes.Repeat([]byte{0, 'A'}, chunkUnits-1)
	firstPayload = append(firstPayload, 0xD8, 0x3D)
	appendTestLobResponse(streamer, firstPayload, driverCommon.UB8(chunkUnits), nil)
	appendTestLobResponse(streamer, []byte{0xDE, 0x42}, 1, nil)
	mustRegisterLob(t, rows, value)

	got, err := io.ReadAll(value)
	want := append(bytes.Repeat([]byte("A"), chunkUnits-1), []byte("🙂")...)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("ReadAll = (%d bytes, %v), want (%d bytes, nil)", len(got), err, len(want))
	}
	if len(offsets) != 2 || offsets[0] != 1 || offsets[1] != driverCommon.UB8(chunkUnits+1) {
		t.Fatalf("locator offsets = %v, want [1 %d]", offsets, chunkUnits+1)
	}
}

// TestStreamedLob_ConstructorRejectsUnsupportedSources verifies constructor
// validation happens before a streamed value is exposed to callers.
func TestStreamedLob_ConstructorRejectsUnsupportedSources(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		owner     func() *ttcRows
		dtype     DtyType
		prefix    []byte
		locator   func() driverCommon.B1Array
		wantError oracleErrors.ErrorCode
	}{
		{name: "missing locator metadata", owner: newLobTestRows, dtype: DtyBlob, wantError: oracleErrors.InvalidLobSource},
		{
			name: "temporary locator", owner: newLobTestRows, dtype: DtyBlob,
			locator: func() driverCommon.B1Array {
				locator := make(driverCommon.B1Array, koll4FlagOffset+1)
				locator[koll4FlagOffset] = kolblTemporaryFlagByte
				return locator
			},
			wantError: oracleErrors.InvalidLobSource,
		},
		{
			name: "malformed CLOB prefix",
			owner: func() *ttcRows {
				rows := newLobTestRows()
				rows.sessionContext = newTestSessionContext()
				return rows
			},
			dtype:     DtyClob,
			prefix:    []byte{0xff},
			locator:   func() driverCommon.B1Array { return make(driverCommon.B1Array, koll4FlagOffset+1) },
			wantError: oracleErrors.InvalidLOBBuffer,
		},
		{
			name: "abstract locator", owner: newLobTestRows, dtype: DtyBlob,
			locator: func() driverCommon.B1Array {
				locator := make(driverCommon.B1Array, koll4FlagOffset+1)
				locator[koll1FlagOffset] = kolblAbstractLocatorFlag
				return locator
			},
			wantError: oracleErrors.InvalidLobSource,
		},
		{
			name: "unsupported datatype",
			owner: func() *ttcRows {
				rows := newLobTestRows()
				rows.sessionContext = newTestSessionContext()
				return rows
			},
			dtype:     DtyType(0xff),
			locator:   func() driverCommon.B1Array { return make(driverCommon.B1Array, koll4FlagOffset+1) },
			wantError: oracleErrors.InvalidLobSource,
		},
		{name: "missing session state", owner: func() *ttcRows { return newTTCRows(nil) }, dtype: DtyBlob, locator: func() driverCommon.B1Array { return make(driverCommon.B1Array, koll4FlagOffset+1) }, wantError: oracleErrors.InvalidLobInput},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			locator := test.locator
			if locator == nil {
				locator = func() driverCommon.B1Array { return nil }
			}
			_, err := newStreamedLob(test.owner(), test.dtype, test.prefix, lobColumnContext{LobLocator: locator()})
			if err == nil {
				t.Fatal("newStreamedLob unexpectedly succeeded")
			}
			requireErrorCode(t, err, test.wantError)
		})
	}
}

// TestStreamedLob_AdoptionRejectsWrongOwnerAndInvalidatedValues verifies
// promotion cannot cross connection or lifecycle boundaries.
func TestStreamedLob_AdoptionRejectsWrongOwnerAndInvalidatedValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		setup  func(*streamedLob)
		key    any
		verify func(*testing.T, error)
	}{
		{name: "different shelf", key: newShelf[driverCommon.MessageType](), verify: func(t *testing.T, err error) { requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer) }},
		{name: "invalidated value", setup: func(value *streamedLob) { value.invalidate() }, verify: func(t *testing.T, err error) { requireErrorCode(t, err, oracleErrors.LobValueInvalidated) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			value, _ := newTestStreamedBlob(t, rows, 1, nil)
			mustRegisterLob(t, rows, value)
			if test.setup != nil {
				test.setup(value)
			}
			_, err := value.detachPersistentState(test.key)
			if err == nil {
				t.Fatal("detachPersistentState unexpectedly succeeded")
			}
			test.verify(t, err)
			_ = value.Close()
		})
	}
}

// TestStreamedLob_BufferedRefillIsConsumedBeforeAnotherRPC verifies a partial
// caller read retains and then drains a converted refill.
func TestStreamedLob_BufferedRefillIsConsumedBeforeAnotherRPC(t *testing.T) {
	t.Parallel()

	rows := newLobTestRows()
	value, streamer := newTestStreamedBlob(t, rows, 4, nil)
	appendTestLobResponse(streamer, []byte("abcd"), 4, nil)
	mustRegisterLob(t, rows, value)
	if n, err := value.Read(nil); n != 0 || err != nil {
		t.Fatalf("zero-length Read = (%d, %v), want (0, nil)", n, err)
	}
	first := make([]byte, 1)
	if n, err := value.Read(first); n != 1 || err != nil || string(first) != "a" {
		t.Fatalf("first Read = (%d, %v, %q), want (1, nil, a)", n, err, first)
	}
	if len(value.pending) != 3 {
		t.Fatalf("pending bytes after partial refill = %d, want 3", len(value.pending))
	}
	remaining := make([]byte, 3)
	if n, err := value.Read(remaining); n != 3 || err != nil || string(remaining) != "bcd" {
		t.Fatalf("second Read = (%d, %v, %q), want (3, nil, bcd)", n, err, remaining)
	}
	if len(streamer.pushed) != 1 || len(rows.lifecycle.lobs) != 0 {
		t.Fatalf("after buffered refill: RPCs=%d outstanding=%d, want 1 and 0", len(streamer.pushed), len(rows.lifecycle.lobs))
	}
}

// TestStreamedLob_MetadataOperationsUseCachedAndServerLengths verifies BLOB
// sizes are local while CLOB sizes and chunk sizes use the TTC manager.
func TestStreamedLob_MetadataOperationsUseCachedAndServerLengths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		value  func(*testing.T, *ttcRows) (*streamedLob, *fakeStreamer)
		setup  func(*fakeStreamer)
		invoke func(*streamedLob) (int64, error)
		want   int64
	}{
		{
			name: "BLOB cached size",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 7, nil)
			},
			invoke: func(value *streamedLob) (int64, error) { return value.Size() },
			want:   7,
		},
		{
			name: "CLOB server size",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 1, nil)
			},
			setup:  func(streamer *fakeStreamer) { appendTestLobResponse(streamer, nil, 8, nil) },
			invoke: func(value *streamedLob) (int64, error) { return value.Size() },
			want:   8,
		},
		{
			name: "CLOB chunk size",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedClob(t, rows, 1, nil)
			},
			setup:  func(streamer *fakeStreamer) { appendTestLobResponse(streamer, nil, 16, nil) },
			invoke: func(value *streamedLob) (int64, error) { return value.ChunkSize() },
			want:   16,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			value, streamer := test.value(t, rows)
			if test.setup != nil {
				test.setup(streamer)
			}
			mustRegisterLob(t, rows, value)
			got, err := test.invoke(value)
			if err != nil || got != test.want {
				t.Fatalf("metadata result = (%d, %v), want (%d, nil)", got, err, test.want)
			}
			_ = value.Close()
		})
	}
}

// TestStreamedLob_WriteToPropagatesWriterAndReadFailures verifies WriterTo
// preserves bytes written and distinguishes short, writer, and read errors.
func TestStreamedLob_WriteToPropagatesWriterAndReadFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		value    func(*testing.T, *ttcRows) (*streamedLob, *fakeStreamer)
		writer   io.Writer
		readErr  bool
		wantN    int64
		checkErr func(*testing.T, error)
	}{
		{
			name: "nil writer",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 1, nil)
			},
			checkErr: func(t *testing.T, err error) {
				requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
			},
		},
		{
			name: "writer error",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 3, []byte("abc"))
			},
			writer: streamedLobWriteFunc(func([]byte) (int, error) {
				return 1, errors.New("destination failed")
			}),
			wantN: 1,
			checkErr: func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), "destination failed") {
					t.Fatalf("WriterTo error = %v, want destination failure", err)
				}
			},
		},
		{
			name: "short writer",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 3, []byte("abc"))
			},
			writer: streamedLobWriteFunc(func([]byte) (int, error) { return 1, nil }),
			wantN:  1,
			checkErr: func(t *testing.T, err error) {
				if !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("WriterTo error = %v, want io.ErrShortWrite", err)
				}
			},
		},
		{
			name: "read error",
			value: func(t *testing.T, rows *ttcRows) (*streamedLob, *fakeStreamer) {
				return newTestStreamedBlob(t, rows, 1, nil)
			},
			writer:  io.Discard,
			readErr: true,
			checkErr: func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("WriterTo unexpectedly succeeded after read failure")
				}
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rows := newLobTestRows()
			value, streamer := test.value(t, rows)
			if test.readErr {
				streamer.pullErr = errors.New("read failed")
			}
			if test.writer != nil {
				mustRegisterLob(t, rows, value)
			}
			got, err := value.WriteTo(test.writer)
			if got != test.wantN {
				t.Fatalf("WriterTo bytes = %d, want %d", got, test.wantN)
			}
			test.checkErr(t, err)
			_ = value.Close()
		})
	}
}

// TestStreamedLob_InvalidateAndLengthOverflow verifies explicit owner
// invalidation and public int64 size conversion remain fail-closed.
func TestStreamedLob_InvalidateAndLengthOverflow(t *testing.T) {
	t.Parallel()

	rows := newLobTestRows()
	value, _ := newTestStreamedBlob(t, rows, 1, nil)
	mustRegisterLob(t, rows, value)
	value.invalidate()
	if _, err := value.Read(make([]byte, 1)); err == nil {
		t.Fatal("Read unexpectedly succeeded after explicit invalidation")
	} else {
		requireErrorCode(t, err, oracleErrors.LobValueInvalidated)
	}
	_ = value.Close()

	if got, err := checkedLobLength(driverCommon.UB8(uint64(^uint64(0)>>1) + 1)); got != 0 || err == nil {
		t.Fatalf("checkedLobLength overflow = (%d, %v), want InvalidLOBBuffer", got, err)
	} else {
		requireErrorCode(t, err, oracleErrors.InvalidLOBBuffer)
	}
}

// TestStreamedLob_QueryRowCopiesLobPrefixLocatorAndMetadata verifies that row
// decoding transfers independent copies of locator inputs and LOB metadata.
func TestStreamedLob_QueryRowCopiesLobPrefixLocatorAndMetadata(t *testing.T) {
	t.Parallel()

	locatorBytes := make(driverCommon.B1Array, koll4FlagOffset+1)
	locatorBytes[koll4FlagOffset] = kolblTemporaryFlagByte
	metadata := &lobColumnContext{
		CharsetForm:       FormNChar,
		CharsetID:         al16Utf16CharSet,
		LocatorByteLength: 99,
		TotalLobLength:    6,
		LobLocator:        locatorBytes,
	}
	rxd := &tTIrxd{
		row: []columnPayload{{data: driverCommon.B1Array("prefix"), lob: metadata}},
	}
	state := &queryRunState{rows: newTTCRows(nil)}
	state.handleRXDRow(rxd)

	// Mutate every source category after ownership transfer.
	rxd.row[0].data.(driverCommon.B1Array)[0] = 'X'
	metadata.CharsetID = 0
	metadata.LobLocator[koll4FlagOffset] = 0

	copied := state.rows.rowData[0][0]
	if string(copied.data.(driverCommon.B1Array)) != "prefix" {
		t.Fatalf("copied prefix = %q, want prefix", copied.data)
	}
	if copied.lob.CharsetForm != FormNChar || copied.lob.CharsetID != al16Utf16CharSet || copied.lob.LocatorByteLength != 99 || copied.lob.TotalLobLength != 6 {
		t.Fatalf("copied metadata changed: %+v", copied.lob)
	}
	if copied.lob.LobLocator[koll4FlagOffset]&kolblTemporaryFlagByte == 0 {
		t.Fatalf("temporary locator flag was not copied: %+v", copied.lob)
	}
}
