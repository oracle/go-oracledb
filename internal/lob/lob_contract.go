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

// Package lob contains private, transport-neutral contracts shared by the
// public oracle/lob package and the TTC implementation. Go's internal-package
// rule prevents applications from importing these carriers directly.
package lob

import "io"

const (
	// BLOB reads and ordinary streamed BLOB bind writes.
	DefaultBlobLobChunkBytes = 32 * 1024

	// CLOB and NCLOB reads and ordinary streamed character bind writes.
	DefaultCharacterLobChunkChars = 32 * 1024
)

// Kind identifies an Oracle LOB family without exposing TTC datatype numbers.
type Kind uint8

const (
	// Unknown is reserved for an absent or unrecognized LOB family.
	Unknown Kind = iota
	// BLOB identifies binary LOB data.
	BLOB
	// CLOB identifies database-character-set LOB data.
	CLOB
	// NCLOB identifies national-character-set LOB data.
	NCLOB
)

// ValidKind reports whether kind is a concrete supported LOB family.
//
// Parameters:
//   - kind: LOB-family discriminator to validate.
//
// Returns:
//   - bool: true for BLOB, CLOB, or NCLOB.
func ValidKind(kind Kind) bool { return kind == BLOB || kind == CLOB || kind == NCLOB }

// LOBSource is the private, transport-neutral source behind an application-
// facing oracle/lob.LOB. It is shared only by the public wrapper and the TTC
// implementation; Go's internal-package rule prevents applications from
// importing or implementing this contract.
//
// Read and WriteTo expose BLOB bytes or UTF-8 CLOB/NCLOB text. Size returns
// the server's logical length, and Kind identifies the LOB family.
type LOBSource interface {
	Read([]byte) (int, error)
	WriteTo(io.Writer) (int64, error)
	Close() error
	Size() (int64, error)
	ChunkSize() (int64, error)
	Kind() Kind
}

// PersistentLocatorSource is a query LOB source that can transfer ownership
// of an unread persistent locator to a connection-bound direct LOB handle.
// DetachPersistentLocator verifies the opaque target session identity and
// atomically detaches the locator bytes, which remain private to the driver.
type PersistentLocatorSource interface {
	LOBSource
	DetachPersistentLocator(sessionKey any) ([]byte, error)
}
