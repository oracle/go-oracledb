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
	"fmt"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// lobOperationCode enumerates TTC OLOBOPS operation codes.
type lobOperationCode common.UB4

const (
	// kplobGetLength instructs the server to return the length of the LOB pointed to by the
	// source locator. The amount (lobAmt) returned depends on the LOB type (character vs binary).
	kplobGetLength lobOperationCode = 0x0001

	// kplobRead requests a read operation starting at sourceOffset for up to lobAmt bytes or
	// characters, copying the data into the destination locator when supplied.
	kplobRead lobOperationCode = 0x0002

	// kplobTrim truncates or extends the target LOB to the supplied length.
	kplobTrim lobOperationCode = 0x0020

	// kplobWrite performs a write operation from the client buffer into the LOB identified by
	// destinationLocator, starting at destinationOffset and writing up to lobAmt units.
	kplobWrite lobOperationCode = 0x0040

	// kplobFileOpen opens the server-side file backing the BFILE locator.
	kplobFileOpen lobOperationCode = 0x0100

	// kplobFileClose closes the server-side file backing the BFILE locator.
	kplobFileClose lobOperationCode = 0x0200

	// kplobFileIsOpen checks whether the server-side file associated with the BFILE locator
	// remains open.
	kplobFileIsOpen lobOperationCode = 0x0400

	// kplobFileExists tests whether the server-side file referenced by the BFILE locator exists.
	kplobFileExists lobOperationCode = 0x0800

	// kplobTmpCreate asks the server to create a temporary LOB associated with the session, with
	// the resulting locator returned in destinationLocator.
	kplobTmpCreate lobOperationCode = 0x0110

	// kplobTmpFree releases a previously created temporary LOB.
	kplobTmpFree lobOperationCode = 0x0111

	// kplobPageSize retrieves the server page size for a LOB; this value influences client-side
	// buffering strategies when streaming large payloads.
	kplobPageSize lobOperationCode = 0x4000

	// kplobOpen opens the LOB for subsequent operations.
	kplobOpen lobOperationCode = 0x8000

	// kplobClose closes the LOB previously opened with kplobOpen.
	kplobClose lobOperationCode = 0x10000

	// kplobIsOpen verifies whether the LOB remains open on the server.
	kplobIsOpen lobOperationCode = 0x11000

	// kplobArrayOperation marks composite array-based LOB operations.
	kplobArrayOperation lobOperationCode = 0x80000

	// kplobArrayTmpFree releases temporary LOBs when operating in array mode.
	kplobArrayTmpFree lobOperationCode = kplobTmpFree | kplobArrayOperation
)

func (op lobOperationCode) String() string {
	switch op {
	case kplobGetLength:
		return "kplobGetLength"
	case kplobRead:
		return "kplobRead"
	case kplobTrim:
		return "kplobTrim"
	case kplobWrite:
		return "kplobWrite"
	case kplobFileOpen:
		return "kplobFileOpen"
	case kplobFileClose:
		return "kplobFileClose"
	case kplobFileIsOpen:
		return "kplobFileIsOpen"
	case kplobFileExists:
		return "kplobFileExists"
	case kplobTmpCreate:
		return "kplobTmpCreate"
	case kplobTmpFree:
		return "kplobTmpFree"
	case kplobPageSize:
		return "kplobPageSize"
	case kplobOpen:
		return "kplobOpen"
	case kplobClose:
		return "kplobClose"
	case kplobIsOpen:
		return "kplobIsOpen"
	case kplobArrayOperation:
		return "kplobArrayOperation"
	case kplobArrayTmpFree:
		return "kplobArrayTmpFree"
	default:
		return fmt.Sprintf("lobOperationCode(0x%X)", common.UB4(op))
	}
}

// IsValid reports whether the operation code matches one of the known TTC OLOBOPS operations.
func (op lobOperationCode) IsValid() bool {
	switch op {
	case kplobGetLength,
		kplobRead,
		kplobTrim,
		kplobWrite,
		kplobFileOpen,
		kplobFileClose,
		kplobFileIsOpen,
		kplobFileExists,
		kplobTmpCreate,
		kplobTmpFree,
		kplobPageSize,
		kplobOpen,
		kplobClose,
		kplobIsOpen,
		kplobArrayOperation,
		kplobArrayTmpFree:
		return true
	default:
		return false
	}
}

// lobDefinition mirrors the payload written alongside TTIFUN/TTILOBD for OLOBOPS.
type lobDefinition struct {
	// sourceLocator identifies the server-side LOB that acts as the source for read or copy
	// operations. The field may be absent for operations that purely target destinationLocator.
	sourceLocator *locator

	// destinationLocator holds the target LOB when the operation writes or copies data. For
	// read-only requests this can remain empty.
	destinationLocator *locator

	// lobAmt conveys the number of bytes/characters to transfer. Some operations request the
	// amount while others may use it to report results.
	lobAmt common.UB8

	// charsetID identifies the character set to interpret the payload when the LOB represents
	// textual data. It is typically 0 for binary LOBs (BLOBs).
	charsetID common.UB2

	// nullO2U flags whether NULL round-tripping (Oracle-to-User) applies to the operation. When
	// set, the driver communicates that the server should treat absent locators as NULL values.
	nullO2U bool

	// sendLobAmt indicates whether the client should marshal lobAmt into the request; certain
	// operations derive their size from auxiliary data and do not require this field.
	sendLobAmt bool

	// lobNull signals that the underlying LOB locator is NULL, instructing the server to handle
	// the request without expecting locator contents.
	lobNull bool

	// operation selects which lobOperationCode TTC should execute for this payload.
	operation lobOperationCode

	// fixedTemporaryLocator keeps compatibility with servers that return the
	// legacy fixed-width temporary locator instead of a length-prefixed locator.
	fixedTemporaryLocator bool

	// bytesTransferred accumulates the number of raw bytes streamed through TTILOBD payloads
	// for the current operation. This is primarily used by read operations to distinguish the
	// byte-oriented transfer size from the character-oriented amount reported by lobAmt.
	bytesTransferred common.UB8

	// lobscnl records the number of UB4 entries present in lobscn, effectively conveying the
	// length of the SCN fragment array that accompanies the locator metadata.
	lobscnl common.SB4

	// lobscn contains the high-order SCN pieces for the LOB, extending lobscnl to a full SCN
	// representation when present.
	lobscn []common.UB4

	// destinationLength captures the signed length metadata carried in TTILOB payloads, such as
	// the temporary LOB duration supplied during kplobTmpCreate requests.
	destinationLength common.SB4
}

// getSourceLocator exposes the source locator associated with the definition. Nil is returned
// when no locator is available so callers can perform appropriate nil checks before dereferencing.
func (def *lobDefinition) getSourceLocator() *locator {
	return def.sourceLocator
}

// getDestinationLocator exposes the destination locator for the definition, allowing callers
// to inspect or marshal it when present. Nil is returned when no destination locator applies.
func (def *lobDefinition) getDestinationLocator() *locator {
	return def.destinationLocator
}

// String returns a human-readable summary of the lobDefinition including locator offsets,
// buffer lengths, and the TTC operation being marshaled. Primarily used for logging and
// debugging TTC LOB flows.
func (def *lobDefinition) String() string {
	sourceOffset := common.UB8(0)
	sourceLocatorLen := 0
	if src := def.getSourceLocator(); src != nil {
		sourceOffset = src.getOffset()
		sourceLocatorLen = src.length()
	}

	destinationOffset := common.UB8(0)
	destinationLocatorLen := 0
	if dst := def.getDestinationLocator(); dst != nil {
		destinationOffset = dst.getOffset()
		destinationLocatorLen = dst.length()
	}

	return fmt.Sprintf(
		"lobDefinition{operation:%s, sendLobAmt:%t, lobAmt:%d, sourceOffset:%d, destinationOffset:%d, nullO2U:%t, lobNull:%t, bytesTransferred:%d, sourceLocatorLen:%d, destinationLocatorLen:%d, lobscnl:%d, lobscnLen:%d}",
		def.operation,
		def.sendLobAmt,
		def.lobAmt,
		sourceOffset,
		destinationOffset,
		def.nullO2U,
		def.lobNull,
		def.bytesTransferred,
		sourceLocatorLen,
		destinationLocatorLen,
		def.lobscnl,
		len(def.lobscn),
	)
}

// NewLobDefinitionForReadOperation initializes a lobDefinition for a kplobRead request.
//
// Parameters:
//   - sourceLocator: locator for the LOB to read from; must reference the source LOB instance.
//   - numBytes: maximum amount of data to fetch in this round trip.
//
// Returns:
//   - *lobDefinition: structure ready to marshal a read request with lobAmt serialized
//     and operation set to kplobRead.
func NewLobDefinitionForReadOperation(
	sourceLocator *locator,
	numBytes common.UB8,
) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		lobAmt:        numBytes,
		sendLobAmt:    true,
		operation:     kplobRead,
	}
}

// NewLobDefinitionForWriteOperation prepares a lobDefinition for kplobWrite requests,
// enabling callers to stream user buffers into a server-side LOB.
//
// Parameters:
//   - sourceLocator: locator that identifies the destination LOB to update.
//   - lobAmt: total amount of data the client expects to send for this call.
//
// Returns:
//   - *lobDefinition: structure populated with buffer metadata and a kplobWrite operation.
func NewLobDefinitionForWriteOperation(
	sourceLocator *locator,
	lobAmt common.UB8,
) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		lobAmt:        lobAmt,
		sendLobAmt:    true,
		operation:     kplobWrite,
	}
}

// NewLobDefinitionForGetLengthOperation crafts a lobDefinition that issues kplobGetLength,
// allowing callers to query the total length of a LOB pointed to by a locator.
//
// Parameters:
//   - sourceLocator: locator whose length should be measured by the server.
//
// Returns:
//   - *lobDefinition: structure configured to marshal a length request with kplobGetLength.
func NewLobDefinitionForGetLengthOperation(sourceLocator *locator) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		sendLobAmt:    true,
		operation:     kplobGetLength,
	}
}

// NewLobDefinitionForGetChunkSizeOperation produces a lobDefinition wired for kplobPageSize
// so that the driver can determine the server page/chunk size for a given LOB.
//
// Parameters:
//   - sourceLocator: locator whose chunk size information is required.
//
// Returns:
//   - *lobDefinition: structure that marshals a chunk-size probe using kplobPageSize.
func NewLobDefinitionForGetChunkSizeOperation(sourceLocator *locator) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		sendLobAmt:    true,
		operation:     kplobPageSize,
	}
}

// NewLobDefinitionForTrimOperation assembles a lobDefinition for kplobTrim, truncating or
// extending a LOB to the length supplied in newLength.
//
// Parameters:
//   - sourceLocator: locator referencing the LOB to modify.
//   - newLength: target length in bytes/characters after the trim completes.
//
// Returns:
//   - *lobDefinition: structure encoding the trim length and using kplobTrim.
func NewLobDefinitionForTrimOperation(sourceLocator *locator, newLength common.UB8) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		lobAmt:        newLength,
		sendLobAmt:    true,
		operation:     kplobTrim,
	}
}

// NewLobDefinitionForOpenOperation returns a lobDefinition configured for either kplobOpen or
// related operations that rely on the LobOpenMode as their payload.
//
// Parameters:
//   - sourceLocator: locator to open or otherwise operate on.
//   - mode: marshaling mode that indicates the client-side access pattern.
//   - operation: specific opcode to issue (e.g., kplobOpen or array variants).
//
// Returns:
//   - *lobDefinition: structure whose lobAmt encodes the LobOpenMode and uses
//     the supplied operation code.
func NewLobDefinitionForOpenOperation(sourceLocator *locator, mode LobOpenMode, operation lobOperationCode) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		lobAmt:        common.UB8(mode),
		sendLobAmt:    true,
		operation:     operation,
	}
}

// NewLobDefinitionForCloseOperation prepares a lobDefinition for LOB close-style commands
// such as kplobClose. These commands only require the locator and opcode; no amount is sent.
//
// Parameters:
//   - sourceLocator: locator to close or release.
//   - operation: opcode indicating the specific close variant to run.
//
// Returns:
//   - *lobDefinition: structure that references the locator and uses the provided opcode.
func NewLobDefinitionForCloseOperation(sourceLocator *locator, operation lobOperationCode) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		sendLobAmt:    false,
		operation:     operation,
	}
}

// NewLobDefinitionForIsOpenOperation builds a lobDefinition for kplobIsOpen-style probes that
// verify whether a server-side LOB remains open.
//
// Parameters:
//   - sourceLocator: locator to inspect for open state.
//   - operation: opcode defining the exact "is open" check to execute.
//
// Returns:
//   - *lobDefinition: structure with nullO2U enabled so NULL locators round-trip correctly
//     while the provided operation is executed.
func NewLobDefinitionForIsOpenOperation(sourceLocator *locator, operation lobOperationCode) *lobDefinition {
	return &lobDefinition{
		sourceLocator: sourceLocator,
		nullO2U:       true,
		operation:     operation,
	}
}

// NewLobDefinitionForTemporaryCreate constructs a lobDefinition ready for kplobTmpCreate,
// allowing clients to request server-managed temporary LOBs with fine-grained options.
//
// Parameters:
//   - tempLocatorSize: size of the locator buffer to allocate on the client side.
//   - formOfUse: encodes the character semantics (e.g., byte vs char) in sourceOffset.
//   - lobType: indicates the kind of temporary LOB (BLOB/CLOB/NCLOB) via destinationOffset.
//   - duration: life span of the temporary LOB, marshaled both in destinationLength and lobAmt.
//   - cache: whether the temporary LOB should be cached on the server (affects lobscn).
//   - charsetID: character set identifier to apply when dealing with text payloads.
//
// Returns:
//   - *lobDefinition: structure that provisions temporary-locator buffers and encodes
//     cache, duration, and charset metadata for a kplobTmpCreate request.
//
// The function preallocates the locator, encodes cache flags inside lobscn, and initializes the
// KOLBLLENB length header when the buffer is sufficiently large.
func NewLobDefinitionForTemporaryCreate(
	tempLocatorSize int,
	formOfUse common.UB2,
	lobType common.UB8,
	duration common.UB4,
	cache bool,
	charsetID common.UB2,
) *lobDefinition {
	cacheFlag := common.UB4(0)
	if cache {
		cacheFlag = 1
	}

	sourceBytes := make(common.B1Array, tempLocatorSize)

	def := &lobDefinition{
		sourceLocator:      newLocator(sourceBytes, common.UB8(formOfUse)),
		destinationLocator: newLocator(nil, lobType),
		destinationLength:  common.SB4(duration),
		lobAmt:             common.UB8(duration),
		sendLobAmt:         true,
		nullO2U:            true, // cacheFlag is sent as lobnull
		operation:          kplobTmpCreate,
		charsetID:          charsetID,
		lobscn:             []common.UB4{cacheFlag},
	}

	def.lobscnl = common.SB4(len(def.lobscn))

	if tempLocatorSize >= kolbLocatorLengthHeaderBytes {
		// The locator length is encoded as a big-endian UB2 in the first two bytes (KOLBLLENB).
		// kolllTempWithSignature fits within this range, so populate both bytes explicitly.
		length := tempLocatorSize - kolbLocatorLengthHeaderBytes
		def.sourceLocator.locatorBytes[0] = byte(length >> 8)
		def.sourceLocator.locatorBytes[1] = byte(length) // low-order byte (0x6A for current kolllTempWithSignature)
	}

	return def
}

// locator packages the raw TTC locator bytes alongside metadata used for marshaling
// LOB operations. The structure is shared between read and write definitions and can
// represent both source and destination locators.
type locator struct {
	locatorBytes common.B1Array
	// offset indicates the 1-based position within sourceLocator where the operation begins.
	offset common.UB8
}

// newLocator constructs a locator instance from the provided bytes and offset metadata.
func newLocator(bytes common.B1Array, offset common.UB8) *locator {
	return &locator{locatorBytes: bytes, offset: offset}
}

// getOffset returns the locator offset used for LOB operations.
func (l *locator) getOffset() common.UB8 {
	return l.offset
}

// length reports the number of raw bytes present in the locator payload.
func (l *locator) length() int {
	return len(l.locatorBytes)
}

// hasBytes reports whether the locator contains a byte slice.
func (l *locator) hasBytes() bool {
	return l.locatorBytes != nil
}

// isTemporaryLocator reports whether the provided locator references a temporary
// LOB by inspecting the temporary flag stored in the koll4 offset.
func (l *locator) isTemporaryLocator() bool {
	return len(l.locatorBytes) > koll4FlagOffset && (l.locatorBytes[koll4FlagOffset]&kolblTemporaryFlagByte) != 0
}

// isValueBasedLocator reports whether the provided locator is value-based (also known as a quasi locator).
// Value-based locators represent LOB contents without a server-side handle, limiting supported operations
// to read-only flows.
func (l *locator) isValueBasedLocator() bool {
	if len(l.locatorBytes) <= koll1FlagOffset {
		return false
	}
	return (l.locatorBytes[koll1FlagOffset] & kolblValueBasedLocatorFlag) == kolblValueBasedLocatorFlag
}

// isReadOnlyLocator reports whether the provided locator enforces read-only semantics.
func (l *locator) isReadOnlyLocator() bool {
	if len(l.locatorBytes) <= koll3FlagOffset {
		return false
	}
	return (l.locatorBytes[koll3FlagOffset] & kolblReadOnlyFlag) == kolblReadOnlyFlag
}

// isAbstractLocator reports whether the provided locator is marked as abstract,
// meaning it omits a server-managed handle and must be managed locally.
func (l *locator) isAbstractLocator() bool {
	return len(l.locatorBytes) > koll1FlagOffset && (l.locatorBytes[koll1FlagOffset]&kolblAbstractLocatorFlag) != 0
}

// isOpenLocator reports whether the locator's open flag is set, indicating the
// LOB is already open either locally (for temporary/abstract locators) or on the
// server.
func (l *locator) isOpenLocator() bool {
	return len(l.locatorBytes) > koll4FlagOffset && (l.locatorBytes[koll4FlagOffset]&kolblOpenFlagByte) != 0
}

// isQuasiLocator reports whether the supplied locator represents a value-based (V4) locator
// by checking the version byte against the quasi locator version defined in koll.h.
func (l *locator) isQuasiLocator() bool {
	if len(l.locatorBytes) <= kolbVersionOffset+1 {
		return false
	}

	return l.locatorBytes[kolbVersionOffset+1] == quasiLocatorVersion
}

// isLobCharsetVariableWidth reports whether the supplied locator describes a variable-width
// character set.
//
// Description:
//
//	Matches the behaviour defined in koll.h by inspecting the third flag byte (KOLL3FLG)
//	for the kolblVaryingWidthFlag bit to determine if the LOB is stored using a variable-width
//	character set representation.
//
// Outputs:
//   - bool: true when the kolblVaryingWidthFlag is set, false otherwise.
func (l *locator) isLobCharsetVariableWidth() bool {
	if len(l.locatorBytes) <= koll3FlagOffset {
		return false
	}
	return (l.locatorBytes[koll3FlagOffset] & kolblVaryingWidthFlag) == kolblVaryingWidthFlag
}

// isLobCharsetLittleEndian reports whether the locator indicates little-endian UTF-16 storage.
//
// Description:
//
//	Examines the fourth flag byte (KOLL4FLG) for the kolblVaryingWidthLittleEndianFlag used to
//	detect AL16UTF16LE storage so downstream conversions can emit or consume bytes in the proper
//	order.
//
// Outputs:
//   - bool: true when little-endian encoding is advertised, false otherwise.
func (l *locator) isLobCharsetLittleEndian() bool {
	if len(l.locatorBytes) <= koll4FlagOffset {
		return false
	}
	return (l.locatorBytes[koll4FlagOffset] & kolblVaryingWidthLittleEndianFlag) == kolblVaryingWidthLittleEndianFlag
}

// setOpenState marks the locator as open by asserting the kolblOpenFlagByte bit within the
// fourth flag byte (KOLL4FLG).
func (l *locator) setOpenState() {
	if len(l.locatorBytes) <= koll4FlagOffset {
		return
	}
	l.locatorBytes[koll4FlagOffset] |= kolblOpenFlagByte
}

// setAccessMode updates the locator's read/write semantics according to the provided
// LobOpenMode by toggling the kolblReadWriteFlagByte bit.
func (l *locator) setAccessMode(mode LobOpenMode) {
	if len(l.locatorBytes) <= koll4FlagOffset {
		return
	}

	if mode == LobOpenModeReadWrite {
		l.locatorBytes[koll4FlagOffset] |= kolblReadWriteFlagByte
	}
}

// clearAccessState resets the open and read/write state bits in the locator, marking it closed
// and removing any previously set access mode.
func (l *locator) clearAccessState() {
	if len(l.locatorBytes) <= koll4FlagOffset {
		return
	}
	l.locatorBytes[koll4FlagOffset] &^= (kolblOpenFlagByte | kolblReadWriteFlagByte)
}
