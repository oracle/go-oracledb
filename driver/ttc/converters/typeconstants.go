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

package converters

import "github.com/oracle/go-driver/driver/common"

// Maximum TTC byte lengths for encodings produced by this package.
// Values reflect Oracle's wire representations to prevent truncation when
// allocating buffers for binds/defines.
const (
	// EmptyStringOacLength is the minimum TTC OAC length to use for DtyVCS binds.
	// Empty strings still require an OAC length of 4.
	EmptyStringOacLength common.UB4 = 4

	// MaxBoolLength is the maximum number of bytes needed to encode a Go bool
	// as a TTC DtyBol value. The on-wire form typically uses 1–2 bytes, but 4 is
	// used here to align with UB4 and provide headroom when constructing OACs.
	MaxBoolLength common.UB4 = 4

	// MaxNumberLength is the maximum number of bytes required by Oracle NUMBER
	// in TTC (variable-length up to 22 bytes).
	MaxNumberLength common.UB4 = 22

	// MaxDateLength is the maximum number of bytes required by DATE/TIMESTAMP
	// without time zone in TTC (up to 11 bytes).
	MaxDateLength common.UB4 = 11

	// MaxTimeStampLength is the maximum number of bytes required by TIMESTAMP
	// WITH TIME ZONE in TTC (13 bytes).
	MaxTimeStampLength common.UB4 = 13

	// MaxNullLength is the maximum number of bytes required by Null DtyVcs
	// TTC (4 bytes).
	MaxNullLength common.UB4 = 4

	// MaxVarcharLength is the maximum TTC byte length used for VARCHAR2-style
	// variable-width character payloads, including space for the TTC length prefix.
	MaxVarcharLength common.UB4 = 32768
)
