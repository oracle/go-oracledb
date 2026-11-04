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
	"fmt"
	"log/slog"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// clobExecutor orchestrates CLOB operations on top of the shared lobExecutor while capturing
// session-specific charset metadata required for amount calculations.
type clobExecutor struct {
	*lobExecutor
	policy lobAmtPolicy
}

// newClobExecutor wires a clobExecutor with the supplied base executor and initialises the
// LOB amount policy based on the negotiated character sets stored in the session context.
//
// Inputs:
//   - shelf: message shelf shared across TTC executors.
//   - sessionCtx: negotiated session metadata used to derive character-set aware policies.
//
// Outputs:
//   - *clobExecutor: executor backed by the shared lobExecutor.
//
// Errors:
//   - None.
func newClobExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType], sessionCtx *driverCommon.SessionContext) *clobExecutor {
	base := newLobExecutor()
	base.SetShelf(shelf)

	driverCS := sessionCtx.DriverCharacterSet()
	ncharCS := sessionCtx.SessionNCharCharacterSet()

	clob := &clobExecutor{lobExecutor: base}
	clob.policy = newLobAmtPolicy(driverCS, ncharCS)

	return clob
}

// open invokes the shared open helper with the supplied locator and mode, ensuring the mode is
// correctly translated to the expected lobMarshalingMode for the base executor.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator to open on the server.
//   - mode: desired LOB open mode (read/write or read-only).
//
// Outputs:
//   - bool: true when the locator was opened by the server, false when already open.
//
// Errors:
//   - Returns InvalidLOBBuffer when attempting to open with BFILE-only read mode.
//   - Propagates failures from the underlying lobExecutor open call.
func (c *clobExecutor) open(ctx context.Context, lobLocator *locator, mode LobOpenMode) (bool, error) {
	if mode == BfileOpenModeReadOnly {
		err := common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "open", "clob", "unsupported BFILE open mode")
		common.Odl.Error("ClobExecutor.Open: invalid open mode", "mode", mode, "error", err)
		return false, err
	}

	return c.lobExecutor.open(ctx, lobLocator, mode)
}

// close closes the locator and clears client side flags via the base executor.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator to close on the server.
//
// Outputs:
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor close call.
func (c *clobExecutor) close(ctx context.Context, lobLocator *locator) error {
	return c.lobExecutor.close(ctx, lobLocator)
}

// GetChunkSize delegates to the shared executor to retrieve the server-reported chunk size
// (page size) for the provided locator.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator whose chunk size is requested.
//
// Outputs:
//   - common.UB8: chunk size reported by the server.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor.
func (c *clobExecutor) getChunkSize(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	return c.lobExecutor.GetChunkSize(ctx, lobLocator)
}

// getLength retrieves the server-reported length for the supplied locator.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator whose length is requested.
//
// Outputs:
//   - common.UB8: length in characters for CLOBs or code units for NCLOBs.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor.
func (c *clobExecutor) getLength(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	return c.lobExecutor.GetLength(ctx, lobLocator)
}

// trim truncates or extends the LOB to the provided length.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator whose length should be adjusted.
//   - newLength: desired size after trimming.
//
// Outputs:
//   - common.UB8: resulting length as reported by the server.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor trim call.
func (c *clobExecutor) trim(ctx context.Context, lobLocator *locator, newLength driverCommon.UB8) (driverCommon.UB8, error) {
	return c.lobExecutor.Trim(ctx, lobLocator, newLength)
}

// isOpen interrogates the locator open state using the base executor helper.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator to check for open state.
//
// Outputs:
//   - bool: true when the locator is open on the server.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor isOpen call.
func (c *clobExecutor) isOpen(ctx context.Context, lobLocator *locator) (bool, error) {
	return c.lobExecutor.IsOpen(ctx, lobLocator)
}

// CreateTemporaryLob provisions a temporary CLOB/NCLOB locator
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - cache: specifies if LOB should be read into buffer cache or not.
//   - duration: duration hint for the temporary LOB lifecycle.
//   - formOfUse: character form (char vs nchar) requested by the caller.
//
// Outputs:
//   - common.B1Array: locator representing the newly created temporary LOB.
//   - error: nil on success.
//
// Errors:
//   - Propagates execution errors from the underlying lobExecutor.
//
// Note:
//   - Temporary CLOBs are always created first and then assigned to table columns, at which point
//     the database promotes them to persistent LOBs.
func (c *clobExecutor) createTemporaryLob(ctx context.Context, cache bool, duration driverCommon.UB4, formOfUse driverCommon.UB2) (driverCommon.B1Array, error) {
	tempSize := kolllTempWithSignature

	// FormChar LOBs inherit the session database character set advertised by the driver
	// policy. This keeps locator creation aligned with the negotiated server character set which
	// is AL32UTF8.
	charsetID := c.policy.driverCS
	if formOfUse != FormChar {
		// FormNChar LOBs must advertise the session's national character set so the server
		// materialises the NCLOB using the negotiated NCHAR semantics.
		charsetID = c.policy.ncharCS
	}

	def := NewLobDefinitionForTemporaryCreate(tempSize, formOfUse, driverCommon.UB8(DtyClob), duration, cache, charsetID)

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"ClobExecutor.CreateTemporaryLob: before execute",
			"cache", cache,
			"duration", duration,
			"formOfUse", formOfUse,
			"tempSize", tempSize,
			"destinationLength", def.destinationLength,
			"lobAmt", def.lobAmt,
			"charsetID", def.charsetID,
			"lobscn", def.lobscn,
			"lobscnl", def.lobscnl,
			"nullO2U", def.nullO2U,
			"operation", def.operation,
		)
	}

	if err := c.lobExecutor.execute(ctx, def); err != nil {
		common.Odl.Error("ClobExecutor.CreateTemporaryLob: execute failed",
			"error", err,
			"operation", def.operation,
		)
		return nil, err
	}

	return def.sourceLocator.locatorBytes, nil
}

// Write mirrors the database CLOB write path, performing character to byte conversion before
// delegating to the shared lobExecutor write helper. The current implementation uses placeholder
// charset conversion logic and should be replaced once the converter package is available.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: destination locator to write into.
//   - isNCLOB: indicates whether the locator represents an NCLOB.
//   - inBuffer: rune slice containing source characters to write.
//   - numChars: number of characters to write from the buffer.
//
// Outputs:
//   - common.UB8: number of characters reported written to the LOB.
//   - error: nil on success.
//
// Errors:
//   - Propagates validation failures from validateLobOperation.
//   - Returns conversion-related errors when byte buffer sizing or encoding fails.
//   - Propagates execution errors from the underlying lobExecutor.
//
// Assumptions:
//   - Callers hand over locators that are eligible for writes. The defensive guard remains until
//     higher-level APIs enforce capability screening, and surfacing InvalidLOBBuffer early when
//     a read-only or value-based locator bypasses those layers.
func (c *clobExecutor) write(
	ctx context.Context,
	lobLocator *locator,
	isNCLOB bool,
	inBuffer []rune,
	numChars int,
) (driverCommon.UB8, error) {
	// validateLobOperation ensures mutating operations honor locator capabilities.
	if err := validateLobOperation(lobLocator, kplobWrite); err != nil {
		common.Odl.Error("ClobExecutor.Write: validateLobOperation failed",
			"error", err,
		)
		return 0, err
	}

	// first see if variable length character set.
	varWidthChar := lobLocator.isLobCharsetVariableWidth()
	littleEndian := lobLocator.isLobCharsetLittleEndian()

	// Estimate byte buffer size based on the requested characters.
	byteBufferSize := getByteBufferSizeForConversion(varWidthChar, numChars)
	binaryWriteBuffer := make([]byte, byteBufferSize)

	bytesConverted, codeUnits, codePoints := c.encodeLobCharPayload(
		inBuffer,
		0,
		numChars,
		binaryWriteBuffer,
		varWidthChar,
		isNCLOB,
		littleEndian,
	)
	if bytesConverted < 0 {
		err := common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"write",
			"clob",
			"character encoding failed",
		)
		common.Odl.Error("ClobExecutor.Write: encodeLobCharPayload failed",
			"error", err,
			"isNCLOB", isNCLOB,
		)
		return 0, err
	}

	count, sendLobAmt, err := c.policy.Translate(isNCLOB, codeUnits, codePoints)
	if err != nil {
		common.Odl.Error("ClobExecutor.Write: unsupported charset in Translate",
			"error", err,
			"isNCLOB", isNCLOB,
		)
		return 0, err
	}
	lobAmt := driverCommon.UB8(0)
	if sendLobAmt {
		lobAmt = driverCommon.UB8(count)
	}

	writeBuffer := driverCommon.B1Array(binaryWriteBuffer[:bytesConverted])
	def := NewLobDefinitionForWriteOperation(lobLocator, lobAmt)
	def.sendLobAmt = sendLobAmt

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"ClobExecutor.Write: prepared definition",
			"offset", lobLocator.offset,
			"isNCLOB", isNCLOB,
			"bytesConverted", bytesConverted,
			"lobAmt", lobAmt,
			"sendLobAmt", sendLobAmt,
		)
	}

	if err := c.lobExecutor.executeWrite(ctx, def, writeBuffer); err != nil {
		common.Odl.Error("ClobExecutor.Write: executeWrite failed",
			"error", err,
			"offset", lobLocator.offset,
			"isNCLOB", isNCLOB,
			"lobAmt", lobAmt,
		)
		return 0, err
	}

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"ClobExecutor.Write: completed",
			"offset", lobLocator.offset,
			"isNCLOB", isNCLOB,
			"charsWritten", def.lobAmt,
		)
	}

	return def.lobAmt, nil
}

// Read fetches TTC encoded bytes using the shared lobExecutor and converts them into Go runes using
// placeholder charset logic. Conversion follows these rules:
//
//	CLOB read:
//	  #1 When the database character set is fixed width, LOB data is sent in the network character
//	     set (UTF-8) and converted to UCS2.
//	  #2 When the database character set is variable width, LOB data is sent as a UB2 slice; each
//	     element represents a UCS2 character so no conversion is required.
//
//	NCLOB read:
//	  #1 When the database character set is fixed width, LOB data is sent in the server NCHAR
//	     character set and converted to UCS2.
//	  #2 When the database character set is variable width, LOB data is sent as a UB2 slice; each
//	     element represents a UCS2 character so no conversion is required.
//
// Inputs:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: source locator to read from.
//   - numChars: maximum number of characters to read.
//   - isNCLOB: indicates whether the locator represents an NCLOB.
//   - charOutBuffer: destination rune buffer for decoded characters.
//
// Outputs:
//   - common.UB8: number of characters decoded into the output buffer.
//   - error: nil on success.
//
// Errors:
//   - Propagates read errors from the underlying lobExecutor.
//   - Propagates conversion errors from decodeLobCharPayload.
func (c *clobExecutor) read(
	ctx context.Context,
	lobLocator *locator,
	numChars driverCommon.UB8,
	isNCLOB bool,
	charOutBuffer []rune,
) (driverCommon.UB8, error) {
	// now see if variable length character set.
	variableWidth := lobLocator.isLobCharsetVariableWidth()
	// If LE, we will have to swap bytes
	littleEndian := lobLocator.isLobCharsetLittleEndian()

	// Calculate how many bytes we need for the byte buffer:
	bufferSize := getByteBufferSizeForConversion(variableWidth, int(numChars))
	binaryReadBuffer := make(driverCommon.B1Array, bufferSize)

	common.Odl.Debug(
		"ClobExecutor.Read: initiating read",
		"offset", lobLocator.offset,
		"numChars", numChars,
		"isNCLOB", isNCLOB,
	)

	bytesTransferred, err := c.lobExecutor.read(ctx, lobLocator, numChars, binaryReadBuffer)
	if err != nil {
		common.Odl.Error("ClobExecutor.Read: base read failed",
			"error", err,
			"offset", lobLocator.offset,
			"numChars", numChars,
		)
		return 0, err
	}

	byteSlice := binaryReadBuffer[:bytesTransferred]

	decodedChars, err := c.decodeLobCharPayload(
		byteSlice,
		charOutBuffer,
		0,
		variableWidth,
		isNCLOB,
		littleEndian,
	)
	if err != nil {
		common.Odl.Error("ClobExecutor.Read: decodeLobCharPayload failed",
			"error", err,
			"bytesTransferred", bytesTransferred,
			"isNCLOB", isNCLOB,
		)
		return 0, err
	}

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"ClobExecutor.Read: completed",
			"offset", lobLocator.offset,
			"numCharsRequested", numChars,
			"decodedChars", decodedChars,
		)
	}

	return driverCommon.UB8(decodedChars), nil
}

// getByteBufferSizeForConversion calculates the byte buffer size required for TTC conversions.
//
// Description:
//
//	Variable-width database character sets are transmitted as UCS2 (2 bytes per character) while
//	fixed-width sets are sent as UTF-8 and therefore assume the worst-case of 3 bytes per
//	character. The function applies these heuristics to size temporary buffers before conversion.
//
// Inputs:
//   - variableWidth: true when the locator uses variable-width storage.
//   - numChars: number of characters slated for conversion.
//
// Outputs:
//   - int: buffer length in bytes
//
// Errors:
//   - None.
func getByteBufferSizeForConversion(variableWidth bool, numChars int) int {
	if variableWidth {
		return numChars * bytesPerUTF16CodeUnit
	}
	return numChars * 3
}

// encodeLobCharPayload converts rune slices into the TTC network representation expected by the
// database.
//
// Description:
//
//	Distinguishes between AL16UTF16/AL16UTF16LE (fixed-width) and AL32UTF8 (variable-width)
//	encodings. Fixed-width and NCLOB locators emit UTF-16 code units, honouring the little endian
//	storage flag (KOLBLVLE) when set, while variable-width locators emit UTF-8.
//
// Inputs:
//   - source: rune slice containing characters to encode.
//   - offset: index within the rune slice where encoding begins.
//   - numChars: number of characters to encode.
//   - destinationBuffer: byte buffer that receives the encoded payload.
//   - variableWidth: true when the locator expects variable-width encoding.
//   - isNCLOB: indicates whether the locator represents an NCLOB.
//   - littleEndian: toggles byte ordering for UTF-16 output.
//
// Outputs:
//   - int: number of bytes written into destinationBuffer (-1 when the buffer lacks capacity.)
//   - int: number of UTF-16 code units generated (or -1 when not applicable).
//   - int: number of Unicode code points processed from source.
func (c *clobExecutor) encodeLobCharPayload(
	source []rune,
	offset int,
	numChars int,
	destinationBuffer []byte,
	variableWidth bool,
	isNCLOB bool,
	littleEndian bool,
) (int, int, int) {
	// Clamp the upper bound so slicing never exceeds the source length; callers may request
	// more characters than remain in the buffer when writing the final chunk.
	end := offset + numChars
	if end > len(source) {
		end = len(source)
	}

	// Slice the runes that should be encoded. When the requested window falls outside the source
	// (e.g. offset == len(source)), we return early to avoid invoking conversion helpers with an
	// empty slice.
	runesToEncode := source[offset:end]
	if len(runesToEncode) == 0 {
		return 0, 0, 0
	}

	if variableWidth {
		return c.encodeVariableWidthCharSet(runesToEncode, destinationBuffer, littleEndian)
	}

	bytesWritten, codeUnits := c.encodeFixedWidthCharSet(runesToEncode, destinationBuffer, isNCLOB, littleEndian)
	return bytesWritten, codeUnits, len(runesToEncode)
}

// encodeVariableWidthCharSet emits UTF-16 code units for variable-width locator encodings.
//
// Description:
//
//	Produces UTF-16 code units for variable-width locator encodings (AL16UTF16/AL16UTF16LE),
//	respecting the locator byte-order flag and optionally recording the number of UTF-16 code units
//	produced so callers can populate OLOBOPS fields consistently.
//
// Inputs:
//   - runes: characters to encode.
//   - destinationBuffer: byte buffer that receives UTF-16 encoded data.
//   - littleEndian: toggles byte ordering for the UTF-16 emission.
//
// Outputs:
//   - int: number of bytes copied into destinationBuffer, or -1 when the buffer lacks capacity.
//   - int: number of UTF-16 code units produced, or -1 when emission fails.
//   - int: number of Unicode code points consumed from runes.
func (c *clobExecutor) encodeVariableWidthCharSet(
	runes []rune,
	destinationBuffer []byte,
	littleEndian bool,
) (int, int, int) {
	// Encode the rune slice into UTF-16 code units so we can emit TTC payloads in the
	// locator's variable-width character set (AL16UTF16/AL16UTF16LE).
	codeUnits := utf16.Encode(runes)

	// Write the UTF-16 code units into the destination buffer, honouring the locator
	// endianness. The helper returns -1 when the supplied buffer is too small.
	bytesWritten := writeUTF16ToBuffer(codeUnits, destinationBuffer, littleEndian)
	if bytesWritten < 0 {
		return -1, -1, len(runes)
	}

	// Surface the number of code units and runes produced so callers can compute LOB
	// amount metadata when required.
	return bytesWritten, len(codeUnits), len(runes)
}

// encodeFixedWidthCharSet emits UTF-16 code units for fixed-width locator encodings.
//
// Description:
//
//	For NCLOBs the data is emitted as UTF-16 code units using the locator-provided endianness,
//	matching the wire format expected by the server. For other CLOBs this remains a best-effort
//	implementation until charset-aware converters are available, while still recording the number of
//	UTF-16 code units when amount is supplied.
//
// Inputs:
//   - runes: characters to encode.
//   - destinationBuffer: byte buffer that receives UTF-16 encoded data.
//   - isNCLOB: indicates whether the locator represents an NCLOB.
//   - littleEndian: toggles byte ordering for the UTF-16 emission.
//
// Outputs:
//   - int: number of bytes copied into destinationBuffer, or -1 when the buffer lacks capacity.
//   - int: amount value corresponding to the emitted payload (code units or code points), or -1 when
//     amount computation is unsupported for the negotiated charset.
func (c *clobExecutor) encodeFixedWidthCharSet(
	runes []rune,
	destinationBuffer []byte,
	isNCLOB bool,
	littleEndian bool,
) (int, int) {
	// Use the negotiated character sets cached inside the lobAmtPolicy. For NCLOBs the session
	// NCHAR set is required on both legs, while CLOBs transmit AL32UTF8 but rely on the database
	// character set when determining amount semantics.
	encodedCharSet := c.policy.driverCS
	if isNCLOB {
		encodedCharSet = c.policy.ncharCS
	}

	// Determine which LOB amount unit (code units vs. code points) should be reported so the caller
	// can populate TTC amount fields consistently.
	unit, err := resolveLobAmtUnit(encodedCharSet)
	if err != nil {
		common.Odl.Error("ClobExecutor.encodeFixedWidthCharSet: unsupported charset",
			"encodedCharSet", encodedCharSet,
			"error", err,
		)
		return -1, -1
	}
	amount := -1

	// Encode the runes into UTF-16 code units so they can be emitted in the locator's fixed-width
	// representation.
	codeUnits := utf16.Encode(runes)

	// Serialise the UTF-16 code units into the destination buffer, respecting the endianness flag
	// advertised by the locator. writeUTF16ToBuffer returns -1 when the buffer is too small.
	bytesWritten := writeUTF16ToBuffer(codeUnits, destinationBuffer, littleEndian)
	if bytesWritten < 0 {
		return -1, -1
	}

	// Translate the byte emission into an amount value when the LOB protocol expects one, either in
	// code points or UTF-16 code units depending on the negotiated unit.
	switch unit {
	case lobAmtCodePoint:
		amount = len(runes)
	case lobAmtCodeUnit:
		amount = len(codeUnits)
	default:
		amount = -1
	}

	return bytesWritten, amount
}

// decodeLobCharPayload converts TTC-encoded bytes into runes using the placeholder charset rules
// shared with encodeLobCharPayload.
//
// Description:
//
//	Selects the decoding routine based on the locator metadata: variable-width payloads are treated
//	as UTF-16 code units while fixed-width or NCLOB payloads pass through the fixed-width helper.
//	Both branches populate the caller-provided rune slice starting at the requested offset and
//	return the number of decoded runes.
//
// Inputs:
//   - source: TTC byte payload read from the server.
//   - charOutBuffer: rune slice that receives decoded characters.
//   - offsetInOutBuffer: index in charOutBuffer where decoded runes should be written.
//   - variableWidth: true when the locator/database character set uses UTF-16 payloads.
//   - isNCLOB: indicates whether the locator represents an NCLOB; currently informational only.
//   - littleEndian: true when UTF-16 payloads are stored in little-endian order.
//
// Outputs:
//   - int: number of Unicode code points written to charOutBuffer.
//   - error: non-nil when decoding fails or the destination buffer is undersized.
//
// Errors:
//   - Returns InvalidLOBBuffer when charOutBuffer cannot hold the decoded runes.
//   - Propagates errors from decodeVariableWidthCharSet and decodeFixedWidthCharSet.
func (c *clobExecutor) decodeLobCharPayload(
	source []byte,
	charOutBuffer []rune,
	offsetInOutBuffer int,
	variableWidth bool,
	isNCLOB bool,
	littleEndian bool,
) (int, error) {
	if variableWidth {
		return c.decodeVariableWidthCharSet(source, charOutBuffer, offsetInOutBuffer, littleEndian)
	}

	return c.decodeFixedWidthCharSet(source, charOutBuffer, offsetInOutBuffer, isNCLOB, littleEndian)
}

// decodeVariableWidthCharSet decodes UTF-16 TTC payloads into runes.
//
// Description:
//
//	The input is interpreted as a sequence of UTF-16 code units using the indicated endianness. The
//	resulting runes are written into charOutBuffer at offsetInOutBuffer, preserving any existing
//	content before that position. Buffer bounds are enforced before writing so callers receive an
//	error instead of partially written output.
//
// Inputs:
//   - source: TTC byte payload containing UTF-16 code units.
//   - charOutBuffer: destination rune slice for decoded characters.
//   - offsetInOutBuffer: index in charOutBuffer where decoding begins.
//   - littleEndian: true when code units are stored in little-endian order.
//
// Outputs:
//   - int: number of runes decoded into charOutBuffer.
//   - error: non-nil when decoding fails or the destination buffer is undersized.
//
// Errors:
//   - Returns InvalidLOBBuffer when the output slice cannot accommodate the decoded runes.
//   - Propagates readUTF16CodeUnits errors for odd byte lengths or malformed UTF-16 payloads.
func (c *clobExecutor) decodeVariableWidthCharSet(
	source []byte,
	charOutBuffer []rune,
	offsetInOutBuffer int,
	littleEndian bool,
) (int, error) {
	codeUnits, err := readUTF16CodeUnits(source, littleEndian)
	if err != nil {
		return 0, err
	}
	runes := utf16.Decode(codeUnits)
	if offsetInOutBuffer+len(runes) > len(charOutBuffer) {
		return 0, common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"clob",
			fmt.Sprintf("output buffer too small: need %d runes", len(runes)),
		)
	}
	copy(charOutBuffer[offsetInOutBuffer:], runes)
	return len(runes), nil
}

// decodeFixedWidthCharSet decodes fixed-width CLOB payloads that arrive in the session character
// set and produces runes in the caller-provided buffer. NCLOB payloads are always emitted as UTF-16
// code units on the wire, so they reuse decodeVariableWidthCharSet to handle endianness and surrogate
// pairs without duplicating logic.
//
// Inputs:
//   - source: TTC byte payload containing character data.
//   - charOutBuffer: destination rune slice for decoded characters.
//   - offsetInOutBuffer: index in charOutBuffer where decoded runes should be written.
//   - isNCLOB: true when the payload was read from an NCLOB locator.
//   - littleEndian: indicates whether UTF-16 payloads are little endian; ignored for UTF-8 paths.
//
// Outputs:
//   - int: number of runes written to charOutBuffer.
//   - error: non-nil when decoding fails or the destination buffer is undersized.
//
// Errors:
//   - Returns InvalidLOBBuffer when charOutBuffer cannot hold the decoded runes.
//   - Propagates errors from decodeVariableWidthCharSet for NCLOB payloads.
func (c *clobExecutor) decodeFixedWidthCharSet(
	source []byte,
	charOutBuffer []rune,
	offsetInOutBuffer int,
	isNCLOB bool,
	littleEndian bool,
) (int, error) {
	if isNCLOB {
		// NCLOB data is always delivered as UTF-16 code units, so reuse the UTF-16 decoder to respect
		// locator byte ordering and surrogate handling without duplicating logic here.
		return c.decodeVariableWidthCharSet(source, charOutBuffer, offsetInOutBuffer, littleEndian)
	}

	decoded := []rune(string(source))
	if offsetInOutBuffer+len(decoded) > len(charOutBuffer) {
		return 0, common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"clob",
			fmt.Sprintf("output buffer too small: need %d runes", len(decoded)),
		)
	}

	copy(charOutBuffer[offsetInOutBuffer:], decoded)

	// Ensure invalid UTF-8 sequences produce a replacement rune instead of silently dropping bytes.
	// Iterating with DecodeRune guarantees progress even when source contains malformed UTF-8.
	count := len(decoded)
	if !utf8.Valid(source) {
		// Re-count using explicit decoding so callers receive the number of runes actually written,
		// which includes replacement runes for invalid sequences.
		count = 0
		tmp := source
		for len(tmp) > 0 {
			_, size := utf8.DecodeRune(tmp)
			count++
			tmp = tmp[size:]
		}
	}

	return count, nil
}

// readUTF16CodeUnits interprets TTC payload bytes as UTF-16 code units.
//
// Description:
//
//	Validates that the payload length is even (two bytes per UTF-16 code unit) before assembling the
//	resulting slice. The caller-provided littleEndian flag controls byte ordering so the helper can
//	service both AL16UTF16 and AL16UTF16LE encodings without duplicating logic.
//
// Inputs:
//   - data: UTF-16 encoded byte payload.
//   - littleEndian: toggles byte ordering when constructing code units.
//
// Outputs:
//   - []uint16: decoded UTF-16 code units.
//   - error: non-nil when data length is not divisible by two.
func readUTF16CodeUnits(data []byte, littleEndian bool) ([]uint16, error) {
	if len(data)%bytesPerUTF16CodeUnit != 0 {
		return nil, common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"clob",
			fmt.Sprintf("invalid UTF-16 byte length: %d", len(data)),
		)
	}
	codeUnits := make([]uint16, len(data)/bytesPerUTF16CodeUnit)
	for i := 0; i < len(codeUnits); i++ {
		base := i * bytesPerUTF16CodeUnit
		if littleEndian {
			codeUnits[i] = uint16(data[base]) | (uint16(data[base+1]) << 8)
		} else {
			codeUnits[i] = (uint16(data[base]) << 8) | uint16(data[base+1])
		}
	}
	return codeUnits, nil
}

// writeUTF16ToBuffer serialises UTF-16 code units into a destination byte buffer.
//
// Description:
//
//	Iterates over the provided code units, emitting bytes in the requested endianness. When the
//	destination buffer is too small the function returns -1 without writing partial data.
//
// Inputs:
//   - codeUnits: UTF-16 code units to serialise.
//   - destinationBuffer: byte buffer that receives the encoded payload.
//   - littleEndian: toggles byte ordering for encoding.
//
// Outputs:
//   - int: number of bytes written, or -1 when the destination lacks sufficient capacity.
//
// Errors:
//   - None.

func writeUTF16ToBuffer(codeUnits []uint16, destinationBuffer []byte, littleEndian bool) int {
	bytesNeeded := len(codeUnits) * bytesPerUTF16CodeUnit
	if bytesNeeded > len(destinationBuffer) {
		return -1
	}

	if littleEndian {
		for i, cu := range codeUnits {
			base := i * bytesPerUTF16CodeUnit
			// Emit the low byte first followed by the high byte to match little-endian UTF-16 layout.
			destinationBuffer[base] = byte(cu)
			destinationBuffer[base+1] = byte(cu >> 8)
		}
	} else {
		for i, cu := range codeUnits {
			base := i * bytesPerUTF16CodeUnit
			// Emit the high byte first for big-endian UTF-16 encodings.
			destinationBuffer[base] = byte(cu >> 8)
			destinationBuffer[base+1] = byte(cu)
		}
	}

	return bytesNeeded
}
