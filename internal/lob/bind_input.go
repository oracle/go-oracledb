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
package lob

import (
	"io"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// BindBlob is the private representation of an application byte slice
// explicitly requested as an Oracle BLOB bind. It is re-exported by oracle as
// BindBlob so TTC can recognize one controlled concrete type without depending on
// the public oracle package.
//
// BindBlob shares its backing storage with the caller's slice. Callers must
// not modify the slice until the ExecContext or QueryContext call has returned.
type BindBlob []byte

// BindClob is the private representation of an application string explicitly
// requested as an Oracle CLOB bind. It is re-exported by oracle as BindClob so TTC
// can select CLOB locator and character-set metadata without depending on the
// public oracle package.
//
// BindClob must contain valid UTF-8. The streamed bind path validates this
// before sending character data to Oracle.
type BindClob string

// BindNClob is the private representation of an application string explicitly
// requested as an Oracle NCLOB bind. It is re-exported by oracle as BindNClob so
// TTC can select national-character locator and character-set metadata without
// depending on the public oracle package.
//
// BindNClob must contain valid UTF-8. The streamed bind path validates this
// before sending character data to Oracle.
type BindNClob string

// Input is the private streamed-bind carrier constructed by the TTC bind path
// after it accepts a public oracle.BindBlob, oracle.BindClob, or oracle.BindNClob marker.
// Applications cannot import or name this type.
type Input struct {
	// reader is consumed only by the statement execution that owns this carrier.
	reader io.Reader
	// kind selects the temporary locator and bind metadata family.
	kind Kind
	// size is the exact source byte count.
	size int64
}

// NewInput creates an exact-length private streamed-bind carrier.
//
// Parameters:
//   - reader: source consumed by one statement execution.
//   - kind: requested BLOB, CLOB, or NCLOB family.
//   - size: exact source byte count.
//
// Returns:
//   - Input: carrier validated before its reader is consumed.
func NewInput(reader io.Reader, kind Kind, size int64) Input {
	return Input{reader: reader, kind: kind, size: size}
}

// Reader returns the application stream owned by the current execution.
//
// Returns:
//   - io.Reader: bind source reader.
func (input Input) Reader() io.Reader { return input.reader }

// Kind returns the Oracle LOB family requested for the bind.
//
// Returns:
//   - Kind: requested BLOB, CLOB, or NCLOB family.
func (input Input) Kind() Kind { return input.kind }

// Size returns the exact input size.
//
// Returns:
//   - int64: declared byte count.
func (input Input) Size() int64 { return input.size }

// ValidationError reports malformed private carrier state without reading it.
//
// Returns:
//   - error: InvalidLobInput for malformed state, or nil when valid.
func (input Input) ValidationError() error {
	switch {
	case input.reader == nil:
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "reader")
	case !ValidKind(input.kind):
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "LOB kind")
	case input.size < 0:
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "size marker")
	default:
		return nil
	}
}
