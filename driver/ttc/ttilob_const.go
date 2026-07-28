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

// This file groups reusable constants related to TTC LOB locators. The layout roughly
// follows the lifecycle of a locator: binary layout, flag semantics, high-level modes,
// character-set hints, and finally temporary LOB handling constants.

// -----------------------------------------------------------------------------
// Locator layout and structural limits
// -----------------------------------------------------------------------------
const (
	// kolbLocatorLengthOffset is the byte offset of the length field in the locator (KOLBLLENB).
	kolbLocatorLengthOffset = 0
	// kolbLocatorLengthHeaderBytes captures the width of the length header (UB2) stored at KOLBLLENB.
	kolbLocatorLengthHeaderBytes = 2
	// kolbVersionOffset is the byte offset of the locator version (KOLBLVSNB).
	kolbVersionOffset = 2

	// Offset of the flags in the locator (KOLL* values mirror koll.h definitions).
	koll1FlagOffset = 0x04
	koll2FlagOffset = koll1FlagOffset + 1
	koll3FlagOffset = koll1FlagOffset + 2
	koll4FlagOffset = koll1FlagOffset + 3

	// quasiLocatorVersion identifies the version associated with value-based locators (a.k.a V4).
	quasiLocatorVersion = 0x04

	// kolblTempLocatorMaxLength mirrors Oracle's maximum temporary/abstract LOB locator size definition.
	kolblTempLocatorMaxLength = 40
	// kolllTemp is the maximum size for a secure temporary LOB locator without signature metadata.
	kolllTemp = kolblTempLocatorMaxLength

	// ztchLenSH512 defines the byte length for SHA-512 signatures.
	ztchLenSH512 = 64
	// kolblSigMetadataSize encapsulates the signature metadata (spare + hash function code + length).
	kolblSigMetadataSize = 4
	// kolblSignatureSize allocates space for future SHA-512 signatures to avoid client updates.
	kolblSignatureSize = ztchLenSH512 + kolblSigMetadataSize
	// kolllTempWithSignature represents the maximum secure temporary LOB locator size with signature.
	kolllTempWithSignature = kolllTemp + kolblSignatureSize
)

// -----------------------------------------------------------------------------
// Locator flag masks (see $SRCHOME/rdbms/src/hdir/koll.h)
// -----------------------------------------------------------------------------
const (
	// Byte 1 --- basic locator type information.
	kolblBlobFlag              byte = 0x01 // Identifies a binary LOB when present in the first flag byte.
	kolblClobFlag              byte = 0x02 // Identifies a character LOB when present in the first flag byte.
	kolblValueBasedLocatorFlag byte = 0x20 // Indicates a quasi/value-based locator (V4 locator).
	kolblAbstractLocatorFlag   byte = 0x40 // Denotes an abstract LOB locator in the first flag byte.

	// Byte 2 --- initialization state and character-set metadata.
	kolblInitializedFlag byte = 0x08 // Indicates whether the locator has been initialized (KOLBLINI).
	kolblEmptyFlag       byte = 0x10 // Marks an empty locator (KOLBLEMP).
	kolblCharSetFormBit0 byte = 0x40 // Represents bit 0 of the 2-bit character set form flag (KOLBL0FRM).
	kolblCharSetFormBit1 byte = 0x80 // Represents bit 1 of the 2-bit character set form flag (KOLBL1FRM).

	// Byte 3 --- lob accessibility and storage hints.
	kolblReadOnlyFlag      byte = 0x01 // Marks a read-only LOB locator (KOLBLRDO).
	kolblMemoryFlag        byte = 0x02 // Indicates the LOB is in-memory (KOLBLMEM).
	kolblDataInLocatorFlag byte = 0x08 // Signals data plus inode are embedded in the locator (KOLBLDIL).
	kolblIovOffFlag        byte = 0x10 // Requests server to disable IOV when reading the locator (KOLBLIOVOF).
	kolblVaryingWidthFlag  byte = 0x80 // Denotes varying-width text data (KOLBLVAR).

	// Byte 4 --- temporary LOB details and encoding variations.
	kolblTemporaryFlagByte            byte = 0x01 // Marks a temporary LOB (KOLBLTMP).
	kolblOpenFlagByte                 byte = 0x08 // Indicates an open temporary LOB (KOLBLOPEN).
	kolblReadWriteFlagByte            byte = 0x10 // Differentiates read-write temporary LOBs (KOLBLRDWR).
	kolblVaryingWidthLittleEndianFlag byte = 0x40 // Notes AL16UTF16LE storage for varying-width text (KOLBLVLE).
	kolblLocalLobFlag                 byte = 0x80 // Identifies in-session local temporary LOBs (KOLBLLCL).
)

// -----------------------------------------------------------------------------
// LOB open modes
// -----------------------------------------------------------------------------

// LobOpenMode indicates the open mode requested while marshaling a locator.
type LobOpenMode uint8

const (
	// LobOpenModeInvalid marks a zero-value or otherwise unsupported mode.
	LobOpenModeInvalid LobOpenMode = 0
	// LobOpenModeReadOnly opens the locator in read-only mode (KOKLORDONLY).
	LobOpenModeReadOnly LobOpenMode = 1
	// LobOpenModeReadWrite opens the locator in read-write mode (KOKLORDWR).
	LobOpenModeReadWrite LobOpenMode = 2
	// BfileOpenModeReadOnly opens a BFILE locator in read-only mode (KOLFORDONLY).
	BfileOpenModeReadOnly LobOpenMode = 11
)

// IsValid reports whether the mode is one of the supported LOB/BFILE open modes.
func (m LobOpenMode) IsValid() bool {
	switch m {
	case LobOpenModeReadOnly, LobOpenModeReadWrite, BfileOpenModeReadOnly:
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// Character-set form hints
// -----------------------------------------------------------------------------
const (
	// FormChar identifies SQL CHAR family LOBs (CHAR, VARCHAR2, CLOB).
	FormChar = 1
	// FormNChar identifies SQL NCHAR family LOBs (NCHAR, NVARCHAR2, NCLOB).
	FormNChar = 2
)

// -----------------------------------------------------------------------------
// Temporary LOB duration semantics (see $SRCHOME/rdbms/src/hdir/oro.h)
// -----------------------------------------------------------------------------
const (
	// durationInvalid denotes invalid duration.
	durationInvalid = -1
	// durationSession denotes the end of user session.
	durationSession = 10
	// durationCall denotes the end of user client/server call.
	durationCall = 12

	// old wrong constants for temporary LOB that are still required for backward compatibility.
	oldWrongDurationSession = 1
	oldWrongDurationCall    = 2
)
