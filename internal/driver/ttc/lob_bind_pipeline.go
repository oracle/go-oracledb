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
	"bufio"
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	ttcLob "github.com/oracle/go-oracledb/v26/internal/driver/ttc/lob"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// lobLocatorBind is the private value substituted for ttcLob.BindInput after
// its source has been copied into a session-local LOB locator. Its locator bytes
// borrow the session locator until encoding; encodeLobLocatorBind snapshots them
// before cleanup can mutate locator flags.
type lobLocatorBind struct {
	// locator borrows the locator bytes until encodeLobLocatorBind snapshots them.
	locator driverCommon.B1Array
	// kind selects the TTC datatype family used by the bind encoder.
	kind internallob.Kind
	// charsetForm identifies the database or national character form for text binds.
	charsetForm driverCommon.UB1
	// charsetID identifies the character set used to encode text bind data.
	charsetID driverCommon.UB2
}

// lobCleanupDisposition states whether a created locator can be released on
// the current TTC stream or must be left to session teardown.
type lobCleanupDisposition uint8

const (
	// lobCleanupFreeNow permits releasing the temporary locator on this session.
	lobCleanupFreeNow lobCleanupDisposition = iota
	// lobCleanupAbandon leaves cleanup to session teardown after an ambiguous exchange.
	lobCleanupAbandon
)

// preparedLobBinds owns the references created while preparing TTC locator
// binds. It releases them in reverse order or abandons them with the session.
type preparedLobBinds struct {
	// manager owns the session registry used to release the created locators.
	manager *lobManager
	// references owns execution-scoped release obligations for created locators.
	references *ttcLob.PreparedReferences
}

func (cleanup *preparedLobBinds) ensureReferences() error {
	if cleanup == nil || cleanup.manager == nil {
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "prepared LOB binds")
	}
	if cleanup.references != nil {
		return nil
	}
	owner, err := ttcLob.NewPreparedReferences(cleanup.manager.state.lobReferenceRegistry, cleanup.manager.abandonSession)
	if err != nil {
		return err
	}
	cleanup.references = owner
	return nil
}

// add retains one connection-level reference immediately after temporary LOB
// creation succeeds.
//
// Parameters:
//   - loc: newly created locator to register for execution cleanup.
//
// Returns:
//   - error: non-nil when loc cannot be registered.
func (cleanup *preparedLobBinds) add(loc *locator) error {
	if err := cleanup.ensureReferences(); err != nil {
		return err
	}
	return cleanup.references.Add(ttcLobLocator{locator: loc})
}

// free releases registered locators in reverse creation order. A failed free
// discards the connection because session teardown is then the only reliable
// cleanup owner for the remaining temporary locator.
//
// Returns:
//   - error: the first cleanup error, if any.
func (cleanup *preparedLobBinds) free() error {
	if cleanup == nil || cleanup.references == nil || cleanup.references.Released() || cleanup.references.Len() == 0 {
		return nil
	}
	if err := cleanup.references.ReleaseAll(); err != nil {
		common.Odl.Error("temporary LOB cleanup failed", "error", err)
		return err
	}
	return nil
}

// abandon transfers cleanup to physical-session teardown after an ambiguous
// TTC failure. It does not return an error because sending kplobTmpFree on an
// unsynchronized stream is unsafe.
func (cleanup *preparedLobBinds) abandon() {
	if cleanup == nil || cleanup.manager == nil {
		return
	}
	if cleanup.references == nil {
		cleanup.manager.abandonSession()
		return
	}
	if cleanup.references.Released() {
		return
	}
	cleanup.references.Abandon()
}

// normalizeLobBindInputs converts public LOB bind markers into the
// private streamed-input carrier used by every temporary-LOB bind. It returns
// the original argument slice when no conversion is required.
//
// BindBlob values retain their byte-slice backing storage and are streamed with an
// exact declared size. BindClob and BindNClob values are streamed with their UTF-8 byte
// lengths; the CLOB path validates text and calculates Oracle logical units.
// Every marker therefore reuses shared LOB creation, OAC generation, and
// cleanup.
func normalizeLobBindInputs(args []driver.NamedValue) []driver.NamedValue {
	// Locate the first explicit LOB marker so ordinary arguments before it can
	// remain in the original slice without an unnecessary copy.
	for index := range args {
		input, ok := normalizeLobBindValue(args[index].Value)
		if !ok {
			continue
		}
		// Copy once, then normalize every later marker in the copied arguments.
		normalized := append([]driver.NamedValue(nil), args...)
		normalized[index].Value = input
		for valueIndex := index + 1; valueIndex < len(normalized); valueIndex++ {
			input, ok := normalizeLobBindValue(args[valueIndex].Value)
			if !ok {
				continue
			}
			normalized[valueIndex].Value = input
		}
		return normalized
	}
	return args
}

// normalizeLobBindValue converts one public convenience marker into an exact
// length private streamed input. It reports false for ordinary SQL bind values.
func normalizeLobBindValue(value any) (ttcLob.BindInput, bool) {
	// Public markers become exact-size readers. Ordinary values are left for the
	// normal database/sql bind conversion path.
	switch value := value.(type) {
	case internallob.BindBlob:
		return ttcLob.NewBindInput(bytes.NewReader(value), internallob.BLOB, int64(len(value))), true
	case internallob.BindClob:
		text := string(value)
		return ttcLob.NewBindInput(strings.NewReader(text), internallob.CLOB, int64(len(text))), true
	case internallob.BindNClob:
		text := string(value)
		return ttcLob.NewBindInput(strings.NewReader(text), internallob.NCLOB, int64(len(text))), true
	default:
		return ttcLob.BindInput{}, false
	}
}

// validateStreamedLobInputs validates every carrier without consuming a reader.
//
// Parameters:
//   - args: statement arguments that may contain ttcLob.BindInput values.
//
// Returns:
//   - int: first streamed-input index, or -1 when none is present.
//   - error: first validation error, if any.
func validateStreamedLobInputs(args []driver.NamedValue) (int, error) {
	firstInput := -1
	for index := range args {
		input, ok := args[index].Value.(ttcLob.BindInput)
		if !ok {
			continue
		}
		if firstInput < 0 {
			firstInput = index
		}
		// Validate every carrier before preparation can consume any reader. This
		// keeps an invalid later bind from leaving an earlier reader partially used.
		if err := input.Validate(); err != nil {
			return -1, err
		}
	}
	return firstInput, nil
}

// normalizeAndValidateStreamedLobInputs prepares statement arguments before
// session admission. Validation must happen before any reader is consumed.
//
// Returns:
//   - []driver.NamedValue: normalized argument copy, or the original slice when
//     no public LOB markers require conversion.
//   - int: first streamed-input index, or -1 when none is present.
//   - error: validation error, if any.
func normalizeAndValidateStreamedLobInputs(args []driver.NamedValue) ([]driver.NamedValue, int, error) {
	// Normalize only explicit LOB markers, then validate the resulting private
	// carriers without opening or reading their sources.
	normalized := normalizeLobBindInputs(args)
	firstInput, err := validateStreamedLobInputs(normalized)
	return normalized, firstInput, err
}

// prepareStreamedLobBinds converts normalized and validated streamed LOB inputs
// into locator binds before OALL/RXD marshalling begins.
//
// Parameters:
//   - ctx: context for temporary-LOB creation and streaming RPCs.
//   - shelf: session TTC state and LOB reference registry.
//   - sessionContext: character-set information for CLOB and NCLOB binds.
//   - args: normalized and validated statement arguments to copy and prepare.
//   - firstInput: first streamed-input index returned by validation.
//
// Returns:
//   - []driver.NamedValue: prepared argument copy, or nil on failure.
//   - *preparedLobBinds: owner of created locators, or nil when none exist.
//   - error: validation, streaming, or cleanup-registration error.
func prepareStreamedLobBinds(
	ctx context.Context,
	shelf *ttiShelf[driverCommon.MessageType],
	sessionContext *driverCommon.SessionContext,
	args []driver.NamedValue,
	firstInput int,
) ([]driver.NamedValue, *preparedLobBinds, error) {
	if firstInput < 0 {
		return args, nil, nil
	}
	// Construct the policy facade only when a streamed LOB actually needs the
	// session-owned executors and LOB reference registry.
	manager, err := newLobManager(shelf, sessionContext)
	if err != nil {
		return nil, nil, err
	}
	return manager.prepareBinds(ctx, args, firstInput)
}

// prepareBinds performs temporary creation, streaming, reference retention, and
// failure abandonment as one ownership transaction. args must already be
// normalized and validated by prepareStreamedLobBinds; firstInput is the index
// returned by that validation pass. The statement layer sees only encoded
// values plus the opaque cleanup owner.
func (m *lobManager) prepareBinds(ctx context.Context, args []driver.NamedValue, firstInput int) ([]driver.NamedValue, *preparedLobBinds, error) {
	// Keep statement arguments separate from the caller's slice because each LOB
	// marker is replaced with a locator-backed bind value.
	prepared := append([]driver.NamedValue(nil), args...)
	cleanup := &preparedLobBinds{manager: m}
	if err := cleanup.ensureReferences(); err != nil {
		return nil, nil, err
	}
	for index := firstInput; index < len(prepared); index++ {
		input, ok := prepared[index].Value.(ttcLob.BindInput)
		if !ok {
			continue
		}
		// Materialize the input before the statement encoder sees it, and retain
		// the locator before handling either success or a post-create error.
		bind, loc, disposition, err := m.streamInput(ctx, input)
		if loc != nil {
			if retainErr := cleanup.add(loc); retainErr != nil {
				// Without a stable LOB ID the locator cannot be queued safely. Tear
				// down the physical session rather than issuing a standalone free.
				cleanup.abandon()
				return nil, nil, retainErr
			}
		}
		if err != nil {
			// A completed exchange can queue cleanup; an ambiguous exchange must
			// abandon the physical session instead.
			if disposition == lobCleanupFreeNow {
				if cleanupErr := cleanup.free(); cleanupErr != nil {
					common.Odl.Error("temporary LOB cleanup after bind failure failed", "error", cleanupErr)
				}
			} else {
				cleanup.abandon()
			}
			return nil, nil, err
		}
		// The statement encoder will turn this private value into LOB-specific
		// RXD and OAC data.
		prepared[index].Value = bind
	}
	return prepared, cleanup, nil
}

// lobCleanupAfterRPC selects conservative session teardown after an RPC error.
//
// Parameters:
//   - err: error returned by one LOB write RPC.
//
// Returns:
//   - lobCleanupDisposition: abandon cleanup to physical-session teardown.
func lobCleanupAfterRPC(err error) lobCleanupDisposition {
	// Without a response-completion marker, no error proves that a follow-up
	// kplobTmpFree request is safe on the current TTC stream.
	return lobCleanupAbandon
}

// streamInput creates, fills, and describes one temporary LOB.
//
// Parameters:
//   - ctx: context for creation and write RPCs.
//   - input: validated application input stream.
//
// Returns:
//   - lobLocatorBind: RXD and OAC metadata after a successful stream.
//   - *locator: created locator as soon as ownership exists.
//   - lobCleanupDisposition: safe cleanup action if an error follows creation.
//   - error: creation, input, encoding, or write failure.
func (m *lobManager) streamInput(
	ctx context.Context,
	input ttcLob.BindInput,
) (bind lobLocatorBind, loc *locator, disposition lobCleanupDisposition, err error) {
	switch input.Kind() {
	case internallob.BLOB:
		// Create a session-duration temporary BLOB and retain the executor used for
		// all writes in this bind.
		executor := m.getBlobExecutor()
		locatorBytes, createErr := executor.createTemporaryLob(ctx, false, durationSession)
		if createErr != nil {
			return bind, nil, lobCleanupAbandon, createErr
		}
		loc = newLocator(append(driverCommon.B1Array(nil), locatorBytes...), 1)
		write := func(ctx context.Context, loc *locator, payload driverCommon.B1Array) (driverCommon.UB8, error) {
			return executor.write(ctx, loc, payload)
		}
		if disposition, err = streamBlobInput(ctx, write, loc, input); err != nil {
			return bind, loc, disposition, err
		}
		// Make an empty temporary BLOB explicitly non-NULL before binding it.
		// Oracle can represent a newly-created but untouched temporary locator as
		// SQL NULL when it is bound. An explicit zero-length trim preserves the
		// public BindBlob([]byte{}) contract as an empty, non-NULL BLOB.
		if loc.offset == 1 {
			if _, trimErr := executor.trim(ctx, loc, 0); trimErr != nil {
				return bind, loc, lobCleanupAfterRPC(trimErr), trimErr
			}
		}
		// Borrow the locator bytes until RXD encoding takes its independent snapshot.
		bind = lobLocatorBind{locator: loc.locatorBytes, kind: input.Kind()}
		return bind, loc, lobCleanupFreeNow, nil

	case internallob.CLOB, internallob.NCLOB:
		// Select character form and charset metadata before creating the temporary
		// locator. The same CLOB executor serves both CLOB and NCLOB operations.
		isNClob := input.Kind() == internallob.NCLOB
		form := driverCommon.UB2(FormChar)
		charsetID := m.sessionCtx.DriverCharacterSet()
		if isNClob {
			form = driverCommon.UB2(FormNChar)
			charsetID = m.sessionCtx.SessionNCharCharacterSet()
		}
		executor := m.getClobExecutor()
		locatorBytes, createErr := executor.createTemporaryLob(ctx, false, durationSession, form)
		if createErr != nil {
			return bind, nil, lobCleanupAbandon, createErr
		}
		loc = newLocator(append(driverCommon.B1Array(nil), locatorBytes...), 1)
		// CLOB uses the varying-width flag for the negotiated AL32UTF8 path;
		// NCLOB retains its national-character locator metadata.
		if !isNClob && len(loc.locatorBytes) > koll3FlagOffset {
			loc.locatorBytes[koll3FlagOffset] |= kolblVaryingWidthFlag
		}
		write := func(ctx context.Context, loc *locator, isNClob bool, runes []rune) (driverCommon.UB8, error) {
			return executor.write(ctx, loc, isNClob, runes, len(runes))
		}
		if disposition, err = streamClobInput(ctx, write, executor.logicalAmount, loc, input, isNClob); err != nil {
			return bind, loc, disposition, err
		}
		// Preserve the non-NULL meaning of an explicitly empty character LOB.
		// See the BLOB branch above. This applies equally to CLOB and NCLOB;
		// zero is expressed in their UTF-16 logical-unit offset space.
		if loc.offset == 1 {
			if _, trimErr := executor.trim(ctx, loc, 0); trimErr != nil {
				return bind, loc, lobCleanupAfterRPC(trimErr), trimErr
			}
		}
		// Carry the character form and charset ID into the LOB-specific OAC.
		bind = lobLocatorBind{
			locator:     loc.locatorBytes,
			kind:        input.Kind(),
			charsetForm: driverCommon.UB1(form),
			charsetID:   charsetID,
		}
		return bind, loc, lobCleanupFreeNow, nil
	default:
		return bind, nil, lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, nil, "LOB kind")
	}
}

// sizedLobReader returns an exact-size reader for a validated input.
//
// Parameters:
//   - input: validated application input stream.
//
// Returns:
//   - *io.LimitedReader: bounded reader that never consumes bytes past its
//     declared size and exposes the remaining byte count for short-source
//     detection.
func sizedLobReader(input ttcLob.BindInput) *io.LimitedReader {
	// The limiter enforces the declared source size while retaining its remaining
	// count for short-source detection at EOF.
	return &io.LimitedReader{R: input.Reader(), N: input.Size()}
}

// streamBlobInput copies input into loc as bounded binary chunks and advances
// loc only by the amount acknowledged in TTIRPA.
//
// Parameters:
//   - ctx: context for input consumption and write RPCs.
//   - write: sends one binary chunk at loc's current offset.
//   - loc: created locator whose offset is advanced.
//   - input: validated BLOB input stream.
//
// Returns:
//   - lobCleanupDisposition: safe cleanup action after an error.
//   - error: input or write failure.
func streamBlobInput(ctx context.Context, write func(context.Context, *locator, driverCommon.B1Array) (driverCommon.UB8, error), loc *locator, input ttcLob.BindInput) (lobCleanupDisposition, error) {
	// Read into one bounded buffer and send each populated portion as one LOB
	// write. The limiter prevents a source from being over-consumed.
	limited := sizedLobReader(input)
	buffer := make([]byte, internallob.DefaultBlobLobChunkBytes)
	for {
		// Check cancellation before consuming more application input or entering a
		// new TTC write.
		if err := ctx.Err(); err != nil {
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, err, "BLOB context")
		}
		n, readErr := limited.Read(buffer)
		if n > 0 {
			// The locator offset advances only after the server acknowledges the full
			// chunk.
			common.Odl.Debug("streamBlobInput: write chunk", "offset", loc.offset, "payloadLength", n)
			written, writeErr := write(ctx, loc, driverCommon.B1Array(buffer[:n]))
			if writeErr != nil {
				return lobCleanupAfterRPC(writeErr), writeErr
			}
			if written != driverCommon.UB8(n) {
				return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, io.ErrShortWrite, "BLOB write")
			}
			loc.offset += written
		}
		if readErr != nil {
			// EOF is valid only after the exact declared size was consumed; all other
			// reader failures are input errors.
			if errors.Is(readErr, io.EOF) {
				if limited.N != 0 {
					return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, io.ErrUnexpectedEOF, "BLOB length")
				}
				return lobCleanupFreeNow, nil
			}
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, readErr, "BLOB read")
		}
		if n == 0 {
			// A reader that reports no data and no error cannot make progress safely.
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, io.ErrNoProgress, "BLOB progress")
		}
	}
}

// streamClobInput incrementally decodes input's UTF-8 into bounded rune chunks.
// The buffered reader retains incomplete sequences across source reads; a
// one-byte RuneError is rejected as malformed UTF-8.
//
// Parameters:
//   - ctx: context for input consumption and write RPCs.
//   - write: sends one character chunk at loc's current offset.
//   - logicalAmount: translates runes to Oracle logical units.
//   - loc: created locator whose offset is advanced.
//   - input: validated CLOB or NCLOB input stream.
//   - isNClob: selects the CLOB or NCLOB write payload path.
//
// Returns:
//   - lobCleanupDisposition: safe cleanup action after an error.
//   - error: decoding, input, or write failure.
func streamClobInput(ctx context.Context, write func(context.Context, *locator, bool, []rune) (driverCommon.UB8, error), logicalAmount func([]rune) (driverCommon.UB8, error), loc *locator, input ttcLob.BindInput, isNClob bool) (lobCleanupDisposition, error) {
	// Limit the source by bytes, then decode it incrementally so a UTF-8 sequence
	// split across underlying reads is preserved by bufio.Reader.
	limited := sizedLobReader(input)
	buffered := bufio.NewReaderSize(limited, DefaultCharacterLobChunkChars)
	runes := make([]rune, 0, DefaultCharacterLobChunkChars)
	flush := func() (lobCleanupDisposition, error) {
		if len(runes) == 0 {
			return lobCleanupFreeNow, nil
		}
		// Convert the complete rune chunk to Oracle logical units before writing it.
		expected, translateErr := logicalAmount(runes)
		if translateErr != nil {
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, translateErr, "CLOB offset")
		}
		written, writeErr := write(ctx, loc, isNClob, runes)
		common.Odl.Debug("streamClobInput: write chunk", "offset", loc.offset, "runeCount", len(runes), "logicalAmount", expected, "isNCLOB", isNClob)
		if writeErr != nil {
			return lobCleanupAfterRPC(writeErr), writeErr
		}
		if written != expected {
			// Do not advance the locator when Oracle acknowledges anything other than
			// the complete logical chunk.
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, io.ErrShortWrite, "CLOB write")
		}
		loc.offset += written
		runes = runes[:0]
		return lobCleanupFreeNow, nil
	}

	for {
		// Cancellation is checked between chunks so a long source cannot continue
		// feeding the server after its statement context has ended.
		if err := ctx.Err(); err != nil {
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, err, "CLOB context")
		}
		r, size, readErr := buffered.ReadRune()
		if readErr != nil {
			// EOF is valid only after the exact byte count was consumed. Other errors
			// are reported as input-read failures.
			if errors.Is(readErr, io.EOF) {
				if limited.N != 0 {
					return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, io.ErrUnexpectedEOF, "CLOB length")
				}
				return flush()
			}
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, readErr, "CLOB read")
		}
		if r == utf8.RuneError && size == 1 {
			// A one-byte RuneError is the decoder's signal for malformed UTF-8; a
			// valid U+FFFD encoding occupies three bytes and is accepted.
			return lobCleanupFreeNow, common.NewOracleError(oracleErrors.InvalidLobInput, errors.New("malformed UTF-8"), "UTF-8 input")
		}
		runes = append(runes, r)
		if len(runes) == cap(runes) {
			if disposition, err := flush(); err != nil {
				return disposition, err
			}
		}
	}
}

// encodeLobLocatorBind supplies RXD payload and OAC metadata without routing a
// locator through RAW or VARCHAR encoders.
//
// Parameters:
//   - bind: prepared locator and LOB metadata.
//
// Returns:
//   - driverCommon.B1Array: copied locator payload for RXD.
//   - driverCommon.Marshallable: OAC metadata describing that payload.
//   - error: non-nil when bind has no locator.
func encodeLobLocatorBind(bind lobLocatorBind) (driverCommon.B1Array, driverCommon.Marshallable, error) {
	// Reject an incomplete prepared bind before constructing protocol metadata.
	if len(bind.locator) == 0 {
		return nil, nil, common.NewOracleError(oracleErrors.InvalidLobInput, nil, "temporary locator")
	}
	dtype := DtyBlob
	if bind.kind == internallob.CLOB || bind.kind == internallob.NCLOB {
		// Both character families use TTC's CLOB datatype; form and charset fields
		// distinguish CLOB from NCLOB in the OAC.
		dtype = DtyClob
	}
	// Return an independent locator payload because later cleanup mutates the
	// temporary locator's local flags.
	oac := newTTIoac(dtype, max_lob_length)
	oac.characterSetForm = bind.charsetForm
	oac.characterSetID = bind.charsetID
	return append(driverCommon.B1Array(nil), bind.locator...), oac, nil
}
