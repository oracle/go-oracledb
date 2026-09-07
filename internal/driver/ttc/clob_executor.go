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
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// clobExecutor orchestrates CLOB operations on top of the shared lobExecutor while capturing
// session-specific charset metadata required for payload conversion.
type clobExecutor struct {
	*lobExecutor
	policy lobCharacterSetPolicy
}

// lobCharacterSetPolicy holds negotiated character sets used solely to select
// CLOB and NCLOB payload encodings. It does not affect Oracle LOB offsets or
// amounts, which always use UCS-2 units.
type lobCharacterSetPolicy struct {
	driverCS driverCommon.UB2
	ncharCS  driverCommon.UB2
}

// bytesPerUTF16CodeUnit is the number of bytes consumed by a UTF-16 code unit.
const bytesPerUTF16CodeUnit = 2

// lobCharacterUnits returns Oracle's logical unit count for CLOB and NCLOB
// LOB operations. Oracle defines their offsets and amounts in UCS-2 units for
// every database and national character set. Supplementary characters therefore
// occupy two units, just as they do in UTF-16.
func lobCharacterUnits(runes []rune) int {
	units := len(runes)
	for _, r := range runes {
		if r >= 0x10000 && r <= utf8.MaxRune {
			units++
		}
	}
	return units
}

// boundedClobReadAmount converts a public refill request to a safe CLOB
// character request.
//
// Parameters:
//   - requested: requested refill capacity in bytes.
//
// Returns:
//   - driverCommon.UB8: character amount capped at the driver's CLOB read
//     limit. A supplementary code point can require four bytes in both TTC
//     UTF-16 and returned UTF-8.
func boundedClobReadAmount(requested driverCommon.UB8) driverCommon.UB8 {
	maximum := driverCommon.UB8(internallob.DefaultCharacterLobChunkChars)
	if maximum == 0 {
		maximum = 1
	}
	if requested > maximum {
		return maximum
	}
	return requested
}

// logicalAmount returns the Oracle locator offset units represented by runes.
// Oracle defines CLOB and NCLOB LOB-operation offsets and amounts in UCS-2
// units, independent of their database or national character-set encodings.
// It is used by streamed binds to verify each TTIRPA acknowledgement before
// advancing a locator offset.
//
// Parameters:
//   - runes: Unicode code points whose locator offset units are required.
//
// Returns:
//   - driverCommon.UB8: Oracle offset units for runes.
//   - error: always nil.
func (c *clobExecutor) logicalAmount(runes []rune) (driverCommon.UB8, error) {
	return driverCommon.UB8(lobCharacterUnits(runes)), nil
}

// newClobExecutor wires a clobExecutor with the supplied base executor and initialises the
// payload character-set policy based on the negotiated character sets stored in the session context.
//
// Parameters:
//   - shelf: message shelf shared across TTC executors.
//   - sessionCtx: negotiated session metadata used to derive character-set aware policies.
//
// Returns:
//   - *clobExecutor: executor backed by the shared lobExecutor.
//
// Errors:
//   - None.
func newClobExecutor(shelf *driverCommon.Shelf[driverCommon.MessageType], sessionCtx *driverCommon.SessionContext) *clobExecutor {
	base := newLobExecutor(shelf)

	driverCS := sessionCtx.DriverCharacterSet()
	ncharCS := sessionCtx.SessionNCharCharacterSet()

	clob := &clobExecutor{lobExecutor: base}
	clob.policy = lobCharacterSetPolicy{driverCS: driverCS, ncharCS: ncharCS}

	return clob
}

// open invokes the shared open helper with the supplied locator and mode, ensuring the mode is
// correctly translated to the expected lobMarshalingMode for the base executor.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator to open on the server.
//   - mode: desired LOB open mode (read/write or read-only).
//
// Returns:
//   - bool: true when the locator was opened by the server, false when already open.
//
// Errors:
//   - Returns UnsupportedLobOperation when attempting to open with BFILE-only read mode.
//   - Propagates failures from the underlying lobExecutor open call.
func (c *clobExecutor) open(ctx context.Context, lobLocator *locator, mode lobOpenMode) (bool, error) {
	if mode == bfileOpenModeReadOnly {
		err := common.NewOracleError(oracleErrors.UnsupportedLobOperation, nil, "BFILE open")
		common.Odl.Error("clobExecutor.open: invalid open mode", "mode", mode, "error", err)
		return false, err
	}

	return c.lobExecutor.open(ctx, lobLocator, mode)
}

// close closes the locator and clears client side flags via the base executor.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator to close on the server.
//
// Returns:
//   - error: nil when the operation succeeds.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor close call.
func (c *clobExecutor) close(ctx context.Context, lobLocator *locator) error {
	return c.lobExecutor.close(ctx, lobLocator)
}

// getChunkSize delegates to the shared executor to retrieve the server-reported chunk size
// (page size) for the provided locator.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator whose chunk size is requested.
//
// Returns:
//   - driverCommon.UB8: chunk size reported by the server.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor.
func (c *clobExecutor) getChunkSize(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	return c.lobExecutor.getChunkSize(ctx, lobLocator)
}

// getLength retrieves the server-reported length for the supplied locator.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator whose length is requested.
//
// Returns:
//   - driverCommon.UB8: length in the UTF-16 code-unit offset space used by TTC LOB
//     locator operations.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor.
func (c *clobExecutor) getLength(ctx context.Context, lobLocator *locator) (driverCommon.UB8, error) {
	return c.lobExecutor.getLength(ctx, lobLocator)
}

// trim truncates or extends the LOB to the provided length.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator whose length should be adjusted.
//   - newLength: desired size after trimming.
//
// Returns:
//   - driverCommon.UB8: resulting length as reported by the server.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor trim call.
func (c *clobExecutor) trim(ctx context.Context, lobLocator *locator, newLength driverCommon.UB8) (driverCommon.UB8, error) {
	return c.lobExecutor.trim(ctx, lobLocator, newLength)
}

// isOpen interrogates the locator open state using the base executor helper.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: locator to check for open state.
//
// Returns:
//   - bool: true when the locator is open on the server.
//   - error: nil on success.
//
// Errors:
//   - Propagates failures from the underlying lobExecutor isOpen call.
func (c *clobExecutor) isOpen(ctx context.Context, lobLocator *locator) (bool, error) {
	return c.lobExecutor.isOpen(ctx, lobLocator)
}

// createTemporaryLob provisions a temporary CLOB/NCLOB locator
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - cache: specifies if LOB should be read into buffer cache or not.
//   - duration: duration hint for the temporary LOB lifecycle.
//   - formOfUse: character form (char vs nchar) requested by the caller.
//
// Returns:
//   - driverCommon.B1Array: locator representing the newly created temporary LOB.
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

	// FormChar LOBs use the driver's configured database-character-set policy. The
	// current supported and validated profile is AL32UTF8.
	charsetID := c.policy.driverCS
	if formOfUse != FormChar {
		// FormNChar LOBs use the configured national-character-set policy. The
		// current supported and validated profile is AL16UTF16.
		charsetID = c.policy.ncharCS
	}

	def := newLobDefinitionForTemporaryCreate(tempSize, formOfUse, driverCommon.UB8(DtyClob), duration, cache, charsetID)

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"clobExecutor.createTemporaryLob: before execute",
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
		common.Odl.Error("clobExecutor.createTemporaryLob: execute failed",
			"error", err,
			"operation", def.operation,
		)
		return nil, err
	}

	return def.sourceLocator.locatorBytes, nil
}

// write mirrors the database CLOB write path, converting characters according
// to the current LOB character-set policy before delegating to the shared
// lobExecutor write helper.
//
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: destination locator to write into.
//   - isNCLOB: indicates whether the locator represents an NCLOB.
//   - inBuffer: rune slice containing source characters to write.
//   - numChars: number of characters to write from the buffer.
//
// Returns:
//   - driverCommon.UB8: number of characters reported written to the LOB.
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
		common.Odl.Error("clobExecutor.write: validateLobOperation failed",
			"error", err,
		)
		return 0, err
	}

	// first see if variable length character set.
	// Temporary CLOB locators do not reliably carry the variable-width flag
	// before their first write. For database CLOBs, use the configured driver
	// character-set policy; NCLOB retains locator-derived AL16UTF16 semantics.
	varWidthChar := lobLocator.isLobCharsetVariableWidth()
	if !isNCLOB && c.policy.driverCS == al32Utf8CharSet {
		varWidthChar = true
	}
	littleEndian := lobLocator.isLobCharsetLittleEndian()

	// Estimate byte buffer size based on the requested characters.
	byteBufferSize := getByteBufferSizeForConversion(varWidthChar, numChars)
	binaryWriteBuffer := make([]byte, byteBufferSize)

	bytesConverted, codeUnits, _ := c.encodeLobCharPayload(
		inBuffer,
		0,
		numChars,
		binaryWriteBuffer,
		varWidthChar,
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
		common.Odl.Error("clobExecutor.write: encodeLobCharPayload failed",
			"error", err,
			"isNCLOB", isNCLOB,
		)
		return 0, err
	}

	// Oracle defines CLOB and NCLOB LOB-operation amounts in UCS-2 units,
	// independent of the negotiated character set and payload encoding.
	lobAmt := driverCommon.UB8(codeUnits)

	writeBuffer := driverCommon.B1Array(binaryWriteBuffer[:bytesConverted])
	def := newLobDefinitionForWriteOperation(lobLocator, lobAmt)
	def.sendLobAmt = true

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"clobExecutor.write: prepared definition",
			"offset", lobLocator.offset,
			"isNCLOB", isNCLOB,
			"bytesConverted", bytesConverted,
			"lobAmt", lobAmt,
			"sendLobAmt", true,
		)
	}

	if err := c.lobExecutor.executeWrite(ctx, def, writeBuffer); err != nil {
		common.Odl.Error("clobExecutor.write: executeWrite failed",
			"error", err,
			"offset", lobLocator.offset,
			"isNCLOB", isNCLOB,
			"lobAmt", lobAmt,
		)
		return 0, err
	}

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"clobExecutor.write: completed",
			"offset", lobLocator.offset,
			"isNCLOB", isNCLOB,
			"charsWritten", def.lobAmt,
		)
	}

	return def.lobAmt, nil
}

// read fetches one complete TTC CLOB or NCLOB locator response and converts it
// to UTF-8. Ordinary locator reads are character-boundary aligned. A streamed
// query LOB may, however, have a high UTF-16 surrogate in its prefetched RXD
// prefix, with the matching low surrogate arriving in the first locator read;
// prefixHighSurrogate carries that state into this same read method.
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
// Parameters:
//   - ctx: request-scoped context for cancellation and deadlines.
//   - locator: source locator to read from.
//   - numUnits: maximum Oracle UCS-2/UTF-16 units to read.
//   - isNCLOB: indicates whether the locator represents an NCLOB.
//   - prefixHighSurrogate: high surrogate already consumed from a prefetched
//     prefix, or zero when no carry is pending.
//
// Returns:
//   - []byte: UTF-8 payload decoded from the locator response.
//   - driverCommon.UB8: Oracle logical units consumed, expressed as UTF-16
//     code units for both CLOB and NCLOB.
//   - error: nil on success.
//
// Errors:
//   - Propagates read errors from the underlying lobExecutor.
//   - Propagates conversion errors from decodeLobCharPayload.
func (c *clobExecutor) read(
	ctx context.Context,
	lobLocator *locator,
	numUnits driverCommon.UB8,
	isNCLOB bool,
	prefixHighSurrogate uint16,
) ([]byte, driverCommon.UB8, error) {
	numUnits = boundedClobReadAmount(numUnits)
	// now see if variable length character set.
	variableWidth := lobLocator.isLobCharsetVariableWidth()
	// Calculate how many bytes we need for the byte buffer:
	bufferSize := getByteBufferSizeForConversion(variableWidth, int(numUnits))
	binaryReadBuffer := make(driverCommon.B1Array, bufferSize)

	common.Odl.Debug(
		"clobExecutor.read: initiating read",
		"offset", lobLocator.offset,
		"numUnits", numUnits,
		"isNCLOB", isNCLOB,
	)

	bytesTransferred, serverUnits, err := c.lobExecutor.read(ctx, lobLocator, numUnits, binaryReadBuffer)
	if err != nil {
		common.Odl.Error("clobExecutor.read: base read failed",
			"error", err,
			"offset", lobLocator.offset,
			"numUnits", numUnits,
		)
		return nil, 0, err
	}
	if serverUnits > numUnits {
		common.Odl.Error("clobExecutor.read: server amount exceeds request",
			"serverUnits", serverUnits,
			"numUnits", numUnits,
		)
		return nil, 0, &completedLobResponseError{err: common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"clob",
			"lobAmt exceeds requested amount",
		)}
	}

	payload, derivedUnits, _, err := c.decodeReadPayload(
		lobLocator,
		isNCLOB,
		binaryReadBuffer[:bytesTransferred],
		prefixHighSurrogate,
		false,
	)
	if err != nil {
		common.Odl.Error("clobExecutor.read: decodeLobCharPayload failed",
			"error", err,
			"bytesTransferred", bytesTransferred,
			"isNCLOB", isNCLOB,
		)
		return nil, 0, &completedLobResponseError{err: err}
	}
	if derivedUnits != serverUnits {
		common.Odl.Error("clobExecutor.read: lobAmt mismatch",
			"serverUnits", serverUnits,
			"derivedUnits", derivedUnits,
		)
		return nil, 0, &completedLobResponseError{err: common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"clob",
			"lobAmt mismatch",
		)}
	}

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"clobExecutor.read: completed",
			"offset", lobLocator.offset,
			"numUnitsRequested", numUnits,
			"logicalUnits", serverUnits,
		)
	}

	return payload, serverUnits, nil
}

// decodeReadPayload converts a TTC CLOB/NCLOB payload to UTF-8 and reports its
// Oracle UTF-16 logical-unit count. When allowTrailingHighSurrogate is true,
// the payload is an inline RXD prefix and a trailing high surrogate is returned
// as carry for the next locator read. Otherwise the payload must be complete.
func (c *clobExecutor) decodeReadPayload(
	lobLocator *locator,
	isNCLOB bool,
	payload []byte,
	prefixHighSurrogate uint16,
	allowTrailingHighSurrogate bool,
) ([]byte, driverCommon.UB8, uint16, error) {
	if len(payload) == 0 {
		if prefixHighSurrogate != 0 {
			return nil, 0, 0, invalidUTF16SurrogateError()
		}
		return nil, 0, 0, nil
	}
	variableWidth := lobLocator.isLobCharsetVariableWidth()
	if prefixHighSurrogate != 0 && !variableWidth && !isNCLOB {
		return nil, 0, 0, invalidUTF16SurrogateError()
	}
	if !variableWidth && !isNCLOB && !utf8.Valid(payload) {
		return nil, 0, 0, common.NewOracleError(
			oracleErrors.InvalidLOBBuffer,
			nil,
			"read",
			"clob",
			"invalid UTF-8 payload",
		)
	}
	if variableWidth || isNCLOB {
		return decodeUTF16Payload(
			payload,
			lobLocator.isLobCharsetLittleEndian(),
			prefixHighSurrogate,
			allowTrailingHighSurrogate,
		)
	}
	runes := make([]rune, len(payload))
	decoded, err := c.decodeLobCharPayload(payload, runes, 0, variableWidth, isNCLOB, lobLocator.isLobCharsetLittleEndian())
	if err != nil {
		return nil, 0, 0, err
	}
	runes = runes[:decoded]
	return []byte(string(runes)), driverCommon.UB8(lobCharacterUnits(runes)), 0, nil
}

// decodeUTF16Payload decodes a UTF-16 payload while optionally joining a high
// surrogate retained from an earlier payload. logical is deliberately based
// only on units in payload, because a carried prefix unit was already counted
// by the caller's LOB offset.
func decodeUTF16Payload(
	payload []byte,
	littleEndian bool,
	prefixHighSurrogate uint16,
	allowTrailingHighSurrogate bool,
) ([]byte, driverCommon.UB8, uint16, error) {
	units, err := readUTF16CodeUnits(payload, littleEndian)
	if err != nil {
		return nil, 0, 0, err
	}
	logical := driverCommon.UB8(len(units))
	if prefixHighSurrogate != 0 {
		if !isUTF16HighSurrogate(prefixHighSurrogate) || len(units) == 0 || !isUTF16LowSurrogate(units[0]) {
			return nil, 0, 0, invalidUTF16SurrogateError()
		}
		joined := make([]uint16, 0, len(units)+1)
		joined = append(joined, prefixHighSurrogate)
		joined = append(joined, units...)
		units = joined
	}

	var nextHighSurrogate uint16
	if allowTrailingHighSurrogate && len(units) > 0 && isUTF16HighSurrogate(units[len(units)-1]) {
		nextHighSurrogate = units[len(units)-1]
		units = units[:len(units)-1]
	}
	if err := validateCompleteUTF16Units(units); err != nil {
		return nil, 0, 0, err
	}
	return []byte(string(utf16.Decode(units))), logical, nextHighSurrogate, nil
}

func invalidUTF16SurrogateError() error {
	return common.NewOracleError(
		oracleErrors.InvalidLOBBuffer,
		nil,
		"read",
		"clob",
		"invalid UTF-16 surrogate pair",
	)
}

func isUTF16HighSurrogate(unit uint16) bool {
	return unit >= 0xD800 && unit <= 0xDBFF
}

func isUTF16LowSurrogate(unit uint16) bool {
	return unit >= 0xDC00 && unit <= 0xDFFF
}

// validateCompleteUTF16Payload rejects a partial code unit or surrogate pair
// before conversion can replace it with U+FFFD.
func validateCompleteUTF16Payload(payload []byte, littleEndian bool) error {
	units, err := readUTF16CodeUnits(payload, littleEndian)
	if err != nil {
		return err
	}
	return validateCompleteUTF16Units(units)
}

func validateCompleteUTF16Units(units []uint16) error {
	for index := 0; index < len(units); index++ {
		unit := units[index]
		switch {
		case isUTF16HighSurrogate(unit):
			if index+1 >= len(units) || !isUTF16LowSurrogate(units[index+1]) {
				return invalidUTF16SurrogateError()
			}
			index++
		case isUTF16LowSurrogate(unit):
			return invalidUTF16SurrogateError()
		}
	}
	return nil
}

// getByteBufferSizeForConversion calculates the byte buffer size required for TTC conversions.
//
// Description:
//
//	The current TTC conversion paths encode both variable-width and fixed-width locator payloads
//	as UTF-16 code units. A supplementary Unicode code point requires a surrogate pair, so every
//	requested rune needs capacity for as many as two two-byte code units.
//
// Parameters:
//   - variableWidth: true when the locator uses variable-width storage.
//   - numChars: number of characters slated for conversion.
//
// Returns:
//   - int: buffer length in bytes
//
// Errors:
//   - None.
func getByteBufferSizeForConversion(variableWidth bool, numChars int) int {
	if numChars <= 0 {
		return 0
	}
	// Keep the width argument in the helper signature because callers derive it from locator
	// metadata and future charset-aware converters may size these representations differently.
	_ = variableWidth
	return numChars * utf8.UTFMax
}

// encodeLobCharPayload converts rune slices into the TTC network representation expected by the
// database.
//
// Description:
//
//	Uses locator metadata to choose the TTC payload conversion path and byte
//	order. That transport conversion is independent of CLOB and NCLOB LOB
//	amounts, which are always counted in UCS-2 units.
//
// Parameters:
//   - source: rune slice containing characters to encode.
//   - offset: index within the rune slice where encoding begins.
//   - numChars: number of characters to encode.
//   - destinationBuffer: byte buffer that receives the encoded payload.
//   - variableWidth: true when the locator expects variable-width encoding.
//   - littleEndian: toggles byte ordering for UTF-16 output.
//
// Returns:
//   - int: number of bytes written into destinationBuffer (-1 when the buffer lacks capacity.)
//   - int: number of UTF-16 code units generated (or -1 when not applicable).
//   - int: number of Unicode code points processed from source.
func (c *clobExecutor) encodeLobCharPayload(
	source []rune,
	offset int,
	numChars int,
	destinationBuffer []byte,
	variableWidth bool,
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

	bytesWritten, codeUnits := c.encodeFixedWidthCharSet(runesToEncode, destinationBuffer, littleEndian)
	return bytesWritten, codeUnits, len(runesToEncode)
}

// encodeVariableWidthCharSet emits UTF-16 code units for this TTC payload path.
//
// Description:
//
//	Produces UTF-16 code units, respecting the locator byte-order flag. The
//	transport representation does not determine the Oracle LOB amount unit.
//
// Parameters:
//   - runes: characters to encode.
//   - destinationBuffer: byte buffer that receives UTF-16 encoded data.
//   - littleEndian: toggles byte ordering for the UTF-16 emission.
//
// Returns:
//   - int: number of bytes copied into destinationBuffer, or -1 when the buffer lacks capacity.
//   - int: number of UTF-16 code units produced, or -1 when emission fails.
//   - int: number of Unicode code points consumed from runes.
func (c *clobExecutor) encodeVariableWidthCharSet(
	runes []rune,
	destinationBuffer []byte,
	littleEndian bool,
) (int, int, int) {
	// Encode the rune slice into UTF-16 code units for the TTC payload.
	codeUnits := utf16.Encode(runes)

	// write the UTF-16 code units into the destination buffer, honouring the locator
	// endianness. The helper returns -1 when the supplied buffer is too small.
	bytesWritten := writeUTF16ToBuffer(codeUnits, destinationBuffer, littleEndian)
	if bytesWritten < 0 {
		return -1, -1, len(runes)
	}

	// Surface the number of code units and runes produced. LOB amounts use the
	// code-unit count under Oracle's UCS-2 LOB API semantics.
	return bytesWritten, len(codeUnits), len(runes)
}

// encodeFixedWidthCharSet emits UTF-16 code units for fixed-width locator encodings.
//
// Description:
//
//	The emitted bytes follow locator encoding metadata. CLOB data is stored in
//	the database character set and NCLOB data in the national character set;
//	this conversion is separate from the UCS-2 unit rule for LOB amounts.
//
// Parameters:
//   - runes: characters to encode.
//   - destinationBuffer: byte buffer that receives UTF-16 encoded data.
//   - littleEndian: toggles byte ordering for the UTF-16 emission.
//
// Returns:
//   - int: number of bytes copied into destinationBuffer, or -1 when the buffer lacks capacity.
//   - int: number of UTF-16 code units emitted, or -1 when encoding fails.
func (c *clobExecutor) encodeFixedWidthCharSet(
	runes []rune,
	destinationBuffer []byte,
	littleEndian bool,
) (int, int) {
	// Encode the runes into UTF-16 code units so they can be emitted in the locator's fixed-width
	// representation.
	codeUnits := utf16.Encode(runes)

	// Serialise the UTF-16 code units into the destination buffer, respecting the endianness flag
	// advertised by the locator. writeUTF16ToBuffer returns -1 when the buffer is too small.
	bytesWritten := writeUTF16ToBuffer(codeUnits, destinationBuffer, littleEndian)
	if bytesWritten < 0 {
		return -1, -1
	}

	return bytesWritten, len(codeUnits)
}

// decodeLobCharPayload converts TTC-encoded bytes into runes using the same
// locator-metadata-based character-set rules as encodeLobCharPayload.
//
// Description:
//
//	Selects the decoding routine based on the locator metadata: variable-width payloads are treated
//	as UTF-16 code units while fixed-width or NCLOB payloads pass through the fixed-width helper.
//	Both branches populate the caller-provided rune slice starting at the requested offset and
//	return the number of decoded runes.
//
// Parameters:
//   - source: TTC byte payload read from the server.
//   - charOutBuffer: rune slice that receives decoded characters.
//   - offsetInOutBuffer: index in charOutBuffer where decoded runes should be written.
//   - variableWidth: true when the locator/database character set uses UTF-16 payloads.
//   - isNCLOB: indicates whether the locator represents an NCLOB; currently informational only.
//   - littleEndian: true when UTF-16 payloads are stored in little-endian order.
//
// Returns:
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
// Parameters:
//   - source: TTC byte payload containing UTF-16 code units.
//   - charOutBuffer: destination rune slice for decoded characters.
//   - offsetInOutBuffer: index in charOutBuffer where decoding begins.
//   - littleEndian: true when code units are stored in little-endian order.
//
// Returns:
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
// Parameters:
//   - source: TTC byte payload containing character data.
//   - charOutBuffer: destination rune slice for decoded characters.
//   - offsetInOutBuffer: index in charOutBuffer where decoded runes should be written.
//   - isNCLOB: true when the payload was read from an NCLOB locator.
//   - littleEndian: indicates whether UTF-16 payloads are little endian; ignored for UTF-8 paths.
//
// Returns:
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
// Parameters:
//   - data: UTF-16 encoded byte payload.
//   - littleEndian: toggles byte ordering when constructing code units.
//
// Returns:
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
// Parameters:
//   - codeUnits: UTF-16 code units to serialise.
//   - destinationBuffer: byte buffer that receives the encoded payload.
//   - littleEndian: toggles byte ordering for encoding.
//
// Returns:
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
