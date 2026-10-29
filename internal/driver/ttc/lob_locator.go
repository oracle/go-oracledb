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

	internalCommon "github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

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

// String returns a concise description of the locator for logs and debugging.
// It is safe to call on a nil locator.
func (l *locator) String() string {
	if l == nil {
		return "<nil>"
	}
	return fmt.Sprintf("locator{offset:%d, length:%d}", l.offset, len(l.locatorBytes))
}

// hasBytes reports whether the locator contains a byte slice.
func (l *locator) hasBytes() bool {
	return l.locatorBytes != nil
}

// MarkReleased clears the local locator flags when its reference is released.
func (l *locator) MarkReleased() {
	if len(l.locatorBytes) <= koll4FlagOffset {
		return
	}
	l.locatorBytes[koll1FlagOffset] &^= kolblAbstractLocatorFlag
	l.locatorBytes[koll2FlagOffset] &^= kolblInitializedFlag
	l.locatorBytes[koll4FlagOffset] &^= kolblTemporaryFlagByte | kolblOpenFlagByte | kolblReadWriteFlagByte
}

// Snapshot returns a copy of the locator bytes for a server-side free.
func (loc *locator) Snapshot() []byte {
	return append([]byte(nil), loc.locatorBytes...)
}

// LOBReferenceID validates the locator and returns its stable server LOB ID.
func (loc *locator) LOBReferenceID() (lobReferenceID, error) {
	if loc == nil || len(loc.locatorBytes) < kolbLobIDOffset+kolbLobIDLength {
		return "", internalCommon.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "missing complete LOB ID")
	}
	if loc.isQuasiLocator() || loc.isValueBasedLocator() {
		return "", internalCommon.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "value-based locator")
	}
	if loc.locatorBytes[koll2FlagOffset]&kolblInitializedFlag == 0 {
		return "", internalCommon.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "uninitialized locator")
	}
	if !loc.isTemporaryLocator() && !loc.isAbstractLocator() {
		return "", internalCommon.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "lob-reference", "lob", "persistent locator")
	}
	return lobReferenceID(string(loc.locatorBytes[kolbLobIDOffset : kolbLobIDOffset+kolbLobIDLength])), nil
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
// Returns:
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
// Returns:
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
// lobOpenMode by toggling the kolblReadWriteFlagByte bit.
func (l *locator) setAccessMode(mode lobOpenMode) {
	if len(l.locatorBytes) <= koll4FlagOffset {
		return
	}

	if mode == lobOpenModeReadWrite {
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
