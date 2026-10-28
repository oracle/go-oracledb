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
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// BindInput carries a validated public LOB bind marker into TTC bind preparation.
// It is exported only to cross the internal package boundary; applications use
// the public bind-marker types instead.
type BindInput struct {
	source   io.Reader
	lobKind  internallob.Kind
	byteSize int64
}

// NewBindInput creates an internal carrier for a streamed LOB bind.
func NewBindInput(source io.Reader, kind internallob.Kind, size int64) BindInput {
	return BindInput{source: source, lobKind: kind, byteSize: size}
}

// Reader returns the input stream.
func (input BindInput) Reader() io.Reader { return input.source }

// Kind returns the Oracle LOB kind.
func (input BindInput) Kind() internallob.Kind { return input.lobKind }

// Size returns the declared input size in bytes.
func (input BindInput) Size() int64 { return input.byteSize }

// Validate checks that the carrier has a reader, supported LOB kind, and
// non-negative byte size.
func (input BindInput) Validate() error {
	switch {
	case input.source == nil:
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "reader")
	case !internallob.ValidKind(input.lobKind):
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "LOB kind")
	case input.byteSize < 0:
		return common.NewOracleError(oracleErrors.InvalidLobInput, nil, "size marker")
	default:
		return nil
	}
}
