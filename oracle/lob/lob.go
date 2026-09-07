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

// Package lob provides the supported application API for streaming Oracle
// BLOB, CLOB, and NCLOB values. TTC locators and connection ownership remain
// private to the driver.
//
// The current CLOB/NCLOB implementation is validated for sessions whose
// database character set is AL32UTF8 and whose national character set is
// AL16UTF16. BLOBs are unaffected by this text-character-set limitation.
// CLOB and NCLOB data is exposed to Go as UTF-8, while their Oracle logical
// sizes and offsets use UTF-16 code units. Other database or national
// character-set combinations are not currently validated; applications using
// them should treat CLOB/NCLOB behavior as unsupported until the driver adds
// explicit charset validation or conversion.
package lob

import (
	"errors"
	"io"
	"reflect"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Kind identifies the Oracle LOB family represented by an LOB. It is shared
// with the private driver contracts and remains independent of TTC datatype
// numbers used on the wire.
type Kind = internallob.Kind

const (
	// Unknown identifies a NULL LOB or an LOB that has not scanned a LOB.
	Unknown = internallob.Unknown
	// BLOB identifies a binary large object whose Read and Size units are bytes.
	BLOB = internallob.BLOB
	// CLOB identifies a character large object exposed by Read as UTF-8.
	CLOB = internallob.CLOB
	// NCLOB identifies a national character large object exposed by Read as UTF-8.
	NCLOB = internallob.NCLOB
)

// LOB is a locator-backed BLOB, CLOB, or NCLOB query result. Scan a query
// column into LOB to read it incrementally. It is valid only while its
// producing Rows and Statement remain open; Rows.Close, Statement.Close, or
// query context cancellation invalidates it. Rows.Next does not invalidate an
// unread LOB, so applications may retain it while the result set remains open.
// Reaching EOF while iterating Rows automatically closes Rows. Read a retained
// LOB before the iteration ends, or call OpenPersistent on the same dedicated
// *sql.Conn before advancing past it when it must outlive Rows. OpenPersistent
// can promote an unread persistent LOB to a DirectLOB tied to that connection.
// Temporary and abstract Oracle locators are not supported for query results.
//
// WARNING: Do not scan an LOB from QueryRow or QueryRowContext.
// Those database/sql helpers close their statement after Scan, so subsequent
// LOB operations fail. Use QueryContext, keep Rows and its statement open,
// then either consume the LOB before closing Rows or call OpenPersistent on
// the same dedicated *sql.Conn before closing Rows.
//
// Read and WriteTo expose BLOB bytes directly and CLOB/NCLOB text as UTF-8.
// Within the currently validated charset scope (database AL32UTF8 and
// national AL16UTF16), Size reports bytes for BLOB and UTF-16 code units for
// CLOB/NCLOB. CLOB data is exposed as UTF-8, so its Size can differ from both
// UTF-8 bytes and Go rune count when it contains supplementary characters. LOB
// contains mutexes and must not be copied after first use.
type LOB struct {
	scanMu sync.Mutex
	mu     sync.Mutex
	source internallob.LOBSource
	kind   Kind
	valid  bool
	closed bool
}

// Scan implements sql.Scanner. It validates the replacement before closing
// the currently scanned source, then publishes the replacement only after the
// previous source has been released. If source cleanup fails, the replacement
// is closed and the cleanup errors are joined. It accepts NULL or a private
// locator-backed value produced by this driver. Materialized []byte and string
// values are rejected because query LOB columns are locator-backed.
//
// Parameters:
//   - input: scanned database value, either nil or a driver locator source.
//
// Returns:
//   - error: nil when input replaces the current LOB value.
func (l *LOB) Scan(input any) error {
	// Validate the replacement before changing the currently scanned LOB. This
	// keeps invalid input from closing a source that could otherwise remain in use.
	var scannedSource internallob.LOBSource
	if input != nil {
		var ok bool
		scannedSource, ok = input.(internallob.LOBSource)
		if !ok || isNilSource(scannedSource) {
			return common.NewOracleError(oracleErrors.InvalidLobSource, nil, "source type")
		}
		if !internallob.ValidKind(scannedSource.Kind()) {
			_ = scannedSource.Close()
			return common.NewOracleError(oracleErrors.InvalidLobSource, nil, "LOB kind")
		}
	}

	// Serialize scans so that only one replacement can close and publish a
	// source at a time.
	l.scanMu.Lock()
	defer l.scanMu.Unlock()

	// Detach the current source while holding the wrapper lock, then release the
	// lock before invoking source cleanup.
	l.mu.Lock()
	previousSource := l.source
	l.source = nil
	l.closed = true
	l.mu.Unlock()

	// Close both sides when replacing the old source fails, preserving the
	// cleanup error from the old source and any error from the replacement.
	if previousSource != nil {
		if previousErr := previousSource.Close(); previousErr != nil {
			var replacementErr error
			if scannedSource != nil {
				replacementErr = scannedSource.Close()
			}
			return errors.Join(previousErr, replacementErr)
		}
	}

	// Publish the replacement only after the previous source has been closed.
	l.mu.Lock()
	l.source = scannedSource
	l.closed = false
	if scannedSource == nil {
		l.kind = Unknown
		l.valid = false
	} else {
		l.kind = scannedSource.Kind()
		l.valid = true
	}
	l.mu.Unlock()
	return nil
}

// Read reads the remaining LOB incrementally. CLOB and NCLOB bytes are UTF-8.
//
// Parameters:
//   - buffer: destination for the next LOB bytes.
//
// Returns:
//   - int: bytes copied into buffer.
//   - error: io.EOF at end of the LOB or an operation error.
func (l *LOB) Read(buffer []byte) (int, error) {
	source, err := l._activeSource()
	if err != nil {
		return 0, err
	}
	return source.Read(buffer)
}

// WriteTo copies the remaining LOB to writer using bounded buffering.
//
// Parameters:
//   - writer: destination for LOB bytes.
//
// Returns:
//   - int64: bytes written.
//   - error: write or LOB read error.
func (l *LOB) WriteTo(writer io.Writer) (int64, error) {
	source, err := l._activeSource()
	if err != nil {
		return 0, err
	}
	return source.WriteTo(writer)
}

// Close releases this LOB's client resources and Rows ownership.
// It is idempotent.
//
// Close does not send kplobClose. kplobClose is Oracle's server-side companion
// to an explicit kplobOpen operation; query LOB reads do not require that
// stateful protocol sequence. Keeping this method as lifecycle cleanup makes
// it safe to use as an io.Closer and prevents a query result from accidentally
// closing a server LOB opened for a different operation.
//
// Returns:
//   - error: nil when local resources are released.
func (l *LOB) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}

	// Mark the wrapper closed and detach its source before releasing the lock so
	// concurrent operations cannot obtain a new reference to it.
	l.closed = true
	source := l.source
	l.source = nil
	l.mu.Unlock()

	if source == nil {
		return nil
	}
	return source.Close()
}

// Size returns the logical LOB length: bytes for BLOB and UTF-16 code units
// for CLOB and NCLOB. It may perform a locator RPC.
//
// Returns:
//   - int64: logical LOB length.
//   - error: locator RPC or lifecycle error.
func (l *LOB) Size() (int64, error) {
	source, err := l._activeSource()
	if err != nil {
		return 0, err
	}
	return source.Size()
}

// ChunkSize reports Oracle's storage chunk size for LOB in bytes.
//
// Returns:
//   - int64: server storage chunk size.
//   - error: locator RPC or lifecycle error.
func (l *LOB) ChunkSize() (int64, error) {
	source, err := l._activeSource()
	if err != nil {
		return 0, err
	}
	return source.ChunkSize()
}

// Kind returns the family of the most recently scanned non-NULL LOB.
//
// Returns:
//   - Kind: BLOB, CLOB, NCLOB, or Unknown.
func (l *LOB) Kind() Kind {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.kind
}

// Valid reports whether the most recently scanned database value was non-NULL.
// It does not report whether the locator is still open.
//
// Returns:
//   - bool: true when the most recent scan produced a non-NULL LOB.
func (l *LOB) Valid() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.valid
}

// _activeSource returns the active private source after checking the public
// NULL and close states. The source owns its own operation locking.
func (l *LOB) _activeSource() (internallob.LOBSource, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.valid {
		return nil, common.NewOracleError(oracleErrors.NullLobValue, nil, "unscanned value")
	}
	if l.closed || l.source == nil {
		return nil, common.NewOracleError(oracleErrors.LobValueClosed, nil, "after Close")
	}
	return l.source, nil
}

// isNilSource detects a typed nil hidden in the private source interface.
func isNilSource(source internallob.LOBSource) bool {
	if source == nil {
		return true
	}
	reflected := reflect.ValueOf(source)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
