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

// LOBSession is the private lease-scoped operation surface used by typed
// connection-scoped LOB handles. Implementations use the owning physical
// session and reject operations after its pool lease has ended. These
// operations share an ordered TTC response stream with normal session traffic
// and require session-level coordination before they can safely overlap it.
// The TTC implementation defers that coordination; callers must avoid
// concurrent LOB and non-LOB operations on one physical connection.
type LOBSession interface {
	// LobCreate creates a session-duration temporary locator for kind.
	LobCreate(context.Context, Kind) ([]byte, error)
	// LobRead reads up to amount logical units from the one-based locator offset;
	// character-LOB amounts use UTF-16 code units. A text read can consume one
	// extra unit when needed to complete a supplementary character split across
	// TTC responses, so callers must use the returned logical amount.
	LobRead(context.Context, Kind, []byte, uint64, uint64) ([]byte, uint64, error)
	// LobWrite writes data at the one-based locator offset and reports accepted logical units.
	LobWrite(context.Context, Kind, []byte, uint64, []byte) (uint64, error)
	// LobLength reports bytes for BLOBs or UTF-16 logical units for text LOBs.
	LobLength(context.Context, Kind, []byte) (uint64, error)
	// LobChunkSize reports Oracle's storage chunk size in bytes.
	LobChunkSize(context.Context, Kind, []byte) (uint64, error)
	// LobTrim sets the locator length in bytes or UTF-16 logical units.
	LobTrim(context.Context, Kind, []byte, uint64) (uint64, error)
	// LobOpen opens the locator with the requested server access mode.
	LobOpen(context.Context, Kind, []byte, uint8) (bool, []byte, error)
	// LobClose closes a persistent locator's server-side open state.
	LobClose(context.Context, Kind, []byte) ([]byte, error)
	// LobIsOpen reports the locator's server-side open state.
	LobIsOpen(context.Context, Kind, []byte) (bool, error)
	// LobFree queues temporary-locator cleanup on the owning session.
	LobFree(context.Context, []byte) error
	// LobSessionKey returns an opaque identity for the owning physical session.
	LobSessionKey() any
	// ValidateLOBLease checks the connection lease locally, including for buffered reads.
	ValidateLOBLease() error
	// AcquireLOBLease pins the connection lease until the returned release function is called.
	AcquireLOBLease() (func(), error)
}
