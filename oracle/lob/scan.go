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
	"bytes"
	"errors"
	"io"
	"math"

	"github.com/oracle/go-oracledb/v26/internal/common"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Bytes is a BLOB scan destination. Its underlying type is []byte. Scan fully
// reads the BLOB into memory and releases the query LOB. SQL NULL is rejected;
// use a pointer or a separate nullable value when
// NULL must be represented.
type Bytes []byte

// Text is a CLOB or NCLOB scan destination. Its underlying type is string and
// its contents are UTF-8. Scan fully reads the CLOB or NCLOB into memory and
// releases the query LOB. SQL NULL is rejected;
// use a pointer or a separate nullable value when NULL must be represented.
type Text string

// Scan implements sql.Scanner for a BLOB query column. It fully reads the BLOB
// into memory, assigns its bytes to value, and closes the locator-backed query
// source. SQL NULL and non-BLOB sources are rejected.
//
// Parameters:
//   - input: locator-backed BLOB source supplied by the driver.
//
// Returns:
//   - error: scan, read, close, or source-validation error.
func (value *Bytes) Scan(input any) error {
	source, err := _scanSource(input, BLOB)
	if err != nil {
		return err
	}
	data, err := _readAll(source)
	if err != nil {
		return err
	}
	*value = Bytes(data)
	return nil
}

// Scan implements sql.Scanner for a CLOB or NCLOB query column. It fully reads
// the LOB into memory as UTF-8 text, assigns it to value, and closes the
// locator-backed query source. SQL NULL and non-character LOB sources are
// rejected.
//
// Parameters:
//   - input: locator-backed CLOB or NCLOB source supplied by the driver.
//
// Returns:
//   - error: scan, read, close, or source-validation error.
func (value *Text) Scan(input any) error {
	source, err := _scanSource(input, CLOB, NCLOB)
	if err != nil {
		return err
	}
	data, err := _readAll(source)
	if err != nil {
		return err
	}
	*value = Text(string(data))
	return nil
}

// _scanSource validates input as a non-nil locator source of one of allowed
// kinds. It closes an incompatible source before returning an error.
//
// Parameters:
//   - input: value supplied by database/sql during Scan.
//   - allowed: accepted BLOB, CLOB, or NCLOB source kinds.
//
// Returns:
//   - internallob.LOBSource: validated locator source.
//   - error: NULL, source-type, or source-kind validation error.
func _scanSource(input any, allowed ...Kind) (internallob.LOBSource, error) {
	if input == nil {
		return nil, common.NewOracleError(oracleErrors.NullLobValue, nil, "scan")
	}
	source, ok := input.(internallob.LOBSource)
	if !ok || isNilSource(source) {
		return nil, common.NewOracleError(oracleErrors.InvalidLobSource, nil, "source type")
	}
	for _, kind := range allowed {
		if source.Kind() == kind {
			return source, nil
		}
	}
	_ = source.Close()
	return nil, common.NewOracleError(oracleErrors.InvalidLobSource, nil, "LOB kind")
}

// _readAll consumes source with bounded reads, enforces the in-memory size
// limit, and closes source before returning its bytes or an error.
//
// Parameters:
//   - source: validated locator source to consume and close.
//
// Returns:
//   - []byte: complete BLOB bytes or UTF-8 CLOB/NCLOB text.
//   - error: read, size-limit, or source-close error.
func _readAll(source internallob.LOBSource) (data []byte, err error) {
	defer func() {
		if closeErr := source.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if source.Kind() == BLOB {
		if size, sizeErr := source.Size(); sizeErr != nil {
			return nil, sizeErr
		} else if size > math.MaxInt32 {
			return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "scan", "BLOB", "value exceeds MaxInt32")
		}
	}
	var output bytes.Buffer
	buffer := make([]byte, 32*1024)
	for {
		n, err := source.Read(buffer)
		if n != 0 {
			if int64(output.Len())+int64(n) > math.MaxInt32 {
				return nil, common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "scan", "LOB", "value exceeds MaxInt32")
			}
			_, _ = output.Write(buffer[:n])
		}
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}
