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
// public oracle/datatype/lob package and the TTC implementation. Go's internal-package
// rule prevents applications from importing these carriers directly.
package lob

import (
	"context"
	"io"
	"reflect"
)

const (
	// DefaultBlobLobChunkBytes is the bounded request size for BLOB reads and
	// ordinary streamed BLOB bind writes.
	DefaultBlobLobChunkBytes = 32 * 1024
	// DefaultCharacterLobChunkChars is the bounded text chunk capacity used by
	// CLOB/NCLOB reads and streamed character binds. Oracle locator amounts
	// remain measured in UTF-16 code units.
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

// LOBSource is the private, transport-neutral source behind typed datatype
// scanners and the oracle/datatype/lob materializers. It is shared only by driver
// packages; Go's internal-package rule prevents applications from importing or
// implementing this contract.
//
// Read and WriteTo expose BLOB bytes or UTF-8 CLOB/NCLOB text. Size returns
// the server's logical length, and Kind identifies the LOB family.
type LOBSource interface {
	// Read returns BLOB bytes or UTF-8 CLOB/NCLOB text from the sequential cursor.
	Read([]byte) (int, error)
	// WriteTo copies the remaining LOB value to writer.
	WriteTo(io.Writer) (int64, error)
	// Close releases this source's local ownership and buffered data.
	Close() error
	// Size returns bytes for BLOBs or UTF-16 logical units for text LOBs.
	Size() (int64, error)
	// ChunkSize returns the Oracle storage chunk size in bytes.
	ChunkSize() (int64, error)
	// Kind identifies the Oracle LOB family.
	Kind() Kind
}

// IsNilSource reports whether source is nil, including a typed nil stored in
// the LOBSource interface.
func IsNilSource(source LOBSource) bool {
	if source == nil {
		return true
	}
	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}

// LOBHandleDriver is the private low-level operation surface used by typed
// connection-scoped LOB handles. Implementations must serialize each request
// on the owning physical session and reject operations after its pool lease has
// ended.
type LOBHandleDriver interface {
	// LobCreate creates a session-duration temporary locator for kind.
	LobCreate(context.Context, uint8) ([]byte, error)
	// LobRead reads up to amount logical units from the one-based locator offset;
	// character-LOB amounts use UTF-16 code units. A text read can consume one
	// extra unit when needed to complete a supplementary character split across
	// TTC responses, so callers must use the returned logical amount.
	LobRead(context.Context, uint8, []byte, uint64, uint64) ([]byte, uint64, error)
	// LobWrite writes data at the one-based locator offset and reports accepted logical units.
	LobWrite(context.Context, uint8, []byte, uint64, []byte) (uint64, error)
	// LobLength reports bytes for BLOBs or UTF-16 logical units for text LOBs.
	LobLength(context.Context, uint8, []byte) (uint64, error)
	// LobChunkSize reports Oracle's storage chunk size in bytes.
	LobChunkSize(context.Context, uint8, []byte) (uint64, error)
	// LobTrim sets the locator length in bytes or UTF-16 logical units.
	LobTrim(context.Context, uint8, []byte, uint64) (uint64, error)
	// LobOpen opens the locator with the requested server access mode.
	LobOpen(context.Context, uint8, []byte, uint8) (bool, []byte, error)
	// LobClose closes a persistent locator's server-side open state.
	LobClose(context.Context, uint8, []byte) ([]byte, error)
	// LobIsOpen reports the locator's server-side open state.
	LobIsOpen(context.Context, uint8, []byte) (bool, error)
	// LobFree queues temporary-locator cleanup on the owning session.
	LobFree(context.Context, []byte) error
	// LobSessionKey returns an opaque identity for the owning physical session.
	LobSessionKey() any
}

// LOBSession is a lease-scoped operation capability for a persistent typed
// locator. ValidateLOBLease performs a local check so buffered reads are also
// rejected after the owning physical connection is returned to the pool.
type LOBSession interface {
	LOBHandleDriver
	// ValidateLOBLease reports whether the owning database/sql connection lease is still active.
	ValidateLOBLease() error
	// AcquireLOBLease pins the connection lease until the returned release function is called.
	AcquireLOBLease() (func(), error)
}

// PersistentLOB carries the unread row-prefetched state needed to continue a
// query LOB without rereading bytes already received with the row. FetchOffset
// is one-based and uses bytes for BLOBs or UTF-16 code units for CLOBs/NCLOBs.
// Prefix contains public BLOB bytes or UTF-8 text. PrefixComplete means the
// prefix covers the complete value; it lets readers return EOF without a
// locator request after consuming it. An adopter transfers ownership of the
// Prefix slice to the caller, which may retain it after Rows closes.
type PersistentLOB struct {
	// Locator is an independent opaque locator copy for the adopted handle.
	Locator []byte
	// Prefix is the unread row-prefetched data in the public representation.
	Prefix []byte
	// FetchOffset is the one-based Oracle position for the next locator read.
	FetchOffset uint64
	// PrefixComplete reports whether Prefix contains the complete LOB value.
	PrefixComplete bool
}

// PersistentLocatorAdopter atomically transfers an unread persistent query
// locator and its row-prefetched bytes out of Rows ownership. It also returns
// a driver capability bound to the physical connection lease that produced
// the locator. The capability must expire when database/sql returns the
// physical connection to its pool.
type PersistentLocatorAdopter interface {
	LOBSource
	// AdoptPersistentLocator transfers an unread locator, prefix, and fetch
	// position and returns its owning session capability.
	AdoptPersistentLocator(expected Kind) (PersistentLOB, LOBSession, error)
}
