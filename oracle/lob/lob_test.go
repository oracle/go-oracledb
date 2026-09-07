/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and data
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
	"testing"

	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type testSource struct {
	kind       Kind
	data       []byte
	offset     int
	size       int64
	chunkSize  int64
	readErr    error
	writeToErr error
	sizeErr    error
	chunkErr   error
	closeErr   error
	closeCalls int
}

func (source *testSource) Read(dst []byte) (int, error) {
	if source.readErr != nil {
		return 0, source.readErr
	}
	if source.offset == len(source.data) {
		return 0, io.EOF
	}
	n := copy(dst, source.data[source.offset:])
	source.offset += n
	return n, nil
}

func (source *testSource) WriteTo(writer io.Writer) (int64, error) {
	if source.writeToErr != nil {
		return 0, source.writeToErr
	}
	n, err := writer.Write(source.data[source.offset:])
	source.offset += n
	return int64(n), err
}

func (source *testSource) Close() error {
	source.closeCalls++
	return source.closeErr
}

func (source *testSource) Size() (int64, error) {
	if source.sizeErr != nil {
		return 0, source.sizeErr
	}
	return source.size, nil
}

func (source *testSource) ChunkSize() (int64, error) {
	if source.chunkErr != nil {
		return 0, source.chunkErr
	}
	return source.chunkSize, nil
}

func (source *testSource) Kind() Kind { return source.kind }

func requireLOBTestErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	var sqlErr oracleErrors.SQLError
	if !errors.As(err, &sqlErr) || sqlErr.ErrorCode() != string(want) {
		t.Fatalf("error = %v, want %s", err, want)
	}
}

// TestLOB_ScanDelegatesOperations verifies Scan installs a source and all LOB
// operations delegate to it.
func TestLOB_ScanDelegatesOperations(t *testing.T) {
	t.Parallel()

	source := &testSource{kind: BLOB, data: []byte("abc"), size: 9, chunkSize: 4}
	var value LOB
	if err := value.Scan(source); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if !value.Valid() || value.Kind() != BLOB {
		t.Fatalf("LOB state = valid:%t kind:%d, want valid BLOB", value.Valid(), value.Kind())
	}

	buffer := make([]byte, 2)
	if n, err := value.Read(buffer); err != nil || n != 2 || string(buffer) != "ab" {
		t.Fatalf("Read = (%d, %v, %q), want (2, nil, ab)", n, err, buffer)
	}
	var output bytes.Buffer
	if n, err := value.WriteTo(&output); err != nil || n != 1 || output.String() != "c" {
		t.Fatalf("WriteTo = (%d, %v, %q), want (1, nil, c)", n, err, output.String())
	}
	if size, err := value.Size(); err != nil || size != 9 {
		t.Fatalf("Size = (%d, %v), want (9, nil)", size, err)
	}
	if chunkSize, err := value.ChunkSize(); err != nil || chunkSize != 4 {
		t.Fatalf("ChunkSize = (%d, %v), want (4, nil)", chunkSize, err)
	}
}

// TestLOB_ScanRejectsInvalidSources verifies Scan accepts NULL and rejects
// invalid source types, kinds, and typed-nil pointer sources.
func TestLOB_ScanRejectsInvalidSources(t *testing.T) {
	t.Parallel()

	var value LOB
	if err := value.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) returned error: %v", err)
	}
	if value.Valid() || value.Kind() != Unknown {
		t.Fatalf("Scan(nil) state = valid:%t kind:%d, want invalid Unknown", value.Valid(), value.Kind())
	}
	_, err := value.Size()
	requireLOBTestErrorCode(t, err, oracleErrors.NullLobValue)
	requireLOBTestErrorCode(t, value.Scan("not a LOB source"), oracleErrors.InvalidLobSource)
	invalidKindSource := &testSource{kind: Unknown}
	requireLOBTestErrorCode(t, value.Scan(invalidKindSource), oracleErrors.InvalidLobSource)
	if invalidKindSource.closeCalls != 1 {
		t.Fatalf("invalid-kind Close calls = %d, want 1", invalidKindSource.closeCalls)
	}
	var nilSource *testSource
	requireLOBTestErrorCode(t, value.Scan(nilSource), oracleErrors.InvalidLobSource)
}

// TestLOB_ScanInvalidReplacementPreservesCurrentSource verifies invalid input
// does not close or replace the currently scanned source.
func TestLOB_ScanInvalidReplacementPreservesCurrentSource(t *testing.T) {
	t.Parallel()

	source := &testSource{kind: BLOB, data: []byte("data")}
	var value LOB
	if err := value.Scan(source); err != nil {
		t.Fatalf("initial Scan returned error: %v", err)
	}
	if err := value.Scan("not a LOB source"); err == nil {
		t.Fatal("Scan accepted an invalid replacement")
	} else {
		requireLOBTestErrorCode(t, err, oracleErrors.InvalidLobSource)
	}
	if source.closeCalls != 0 {
		t.Fatalf("current source Close calls = %d, want 0", source.closeCalls)
	}
	if !value.Valid() || value.Kind() != BLOB {
		t.Fatalf("LOB state = valid:%t kind:%d, want valid BLOB", value.Valid(), value.Kind())
	}

	buffer := make([]byte, len(source.data))
	if n, err := value.Read(buffer); err != nil || n != len(source.data) || string(buffer) != string(source.data) {
		t.Fatalf("Read after rejected replacement = (%d, %v, %q), want (%d, nil, %q)", n, err, buffer, len(source.data), source.data)
	}
}

// TestLOB_CloseIsIdempotentAndInvalidates verifies Close releases a source at
// most once and later operations report LobValueClosed.
func TestLOB_CloseIsIdempotentAndInvalidates(t *testing.T) {
	t.Parallel()

	source := &testSource{kind: BLOB}
	var value LOB
	if err := value.Scan(source); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
	if source.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", source.closeCalls)
	}

	_, readErr := value.Read(make([]byte, 1))
	_, writeToErr := value.WriteTo(io.Discard)
	_, sizeErr := value.Size()
	_, chunkErr := value.ChunkSize()
	for _, err := range []error{readErr, writeToErr, sizeErr, chunkErr} {
		requireLOBTestErrorCode(t, err, oracleErrors.LobValueClosed)
	}
}

// TestLOB_ScanReplacesPreviousSource verifies rescanning closes the previous
// source and delegates future operations to the replacement source.
func TestLOB_ScanReplacesPreviousSource(t *testing.T) {
	t.Parallel()

	previous := &testSource{kind: BLOB}
	replacement := &testSource{kind: CLOB, size: 12}
	var value LOB
	if err := value.Scan(previous); err != nil {
		t.Fatalf("first Scan returned error: %v", err)
	}
	if err := value.Scan(replacement); err != nil {
		t.Fatalf("replacement Scan returned error: %v", err)
	}
	if previous.closeCalls != 1 {
		t.Fatalf("previous Close calls = %d, want 1", previous.closeCalls)
	}
	if !value.Valid() || value.Kind() != CLOB {
		t.Fatalf("replacement state = valid:%t kind:%d, want valid CLOB", value.Valid(), value.Kind())
	}
	if size, err := value.Size(); err != nil || size != 12 {
		t.Fatalf("replacement Size = (%d, %v), want (12, nil)", size, err)
	}
	if err := value.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) returned error: %v", err)
	}
	if replacement.closeCalls != 1 {
		t.Fatalf("replacement Close calls = %d, want 1", replacement.closeCalls)
	}
	if value.Valid() || value.Kind() != Unknown {
		t.Fatalf("NULL replacement state = valid:%t kind:%d, want invalid Unknown", value.Valid(), value.Kind())
	}
}

// TestLOB_ScanJoinsCloseErrors verifies a failed rescan joins both source-close
// errors and leaves the LOB unusable.
func TestLOB_ScanJoinsCloseErrors(t *testing.T) {
	t.Parallel()

	previousErr := errors.New("previous close failed")
	replacementErr := errors.New("replacement close failed")
	previous := &testSource{kind: BLOB, closeErr: previousErr}
	replacement := &testSource{kind: CLOB, closeErr: replacementErr}
	var value LOB
	if err := value.Scan(previous); err != nil {
		t.Fatalf("first Scan returned error: %v", err)
	}
	err := value.Scan(replacement)
	if !errors.Is(err, previousErr) || !errors.Is(err, replacementErr) {
		t.Fatalf("Scan error = %v, want both close errors", err)
	}
	if previous.closeCalls != 1 || replacement.closeCalls != 1 {
		t.Fatalf("Close calls = previous:%d replacement:%d, want 1:1", previous.closeCalls, replacement.closeCalls)
	}
	_, operationErr := value.Size()
	requireLOBTestErrorCode(t, operationErr, oracleErrors.LobValueClosed)
}

// TestLOB_DelegatesSourceErrors verifies source Read, WriteTo, Size, and
// ChunkSize errors propagate unchanged.
func TestLOB_DelegatesSourceErrors(t *testing.T) {
	t.Parallel()

	readErr := errors.New("read failed")
	writeToErr := errors.New("write failed")
	sizeErr := errors.New("size failed")
	chunkErr := errors.New("chunk failed")
	tests := []struct {
		name      string
		source    *testSource
		operation func(*LOB) error
		wantErr   error
	}{
		{
			name:   "Read",
			source: &testSource{kind: BLOB, readErr: readErr},
			operation: func(value *LOB) error {
				_, err := value.Read(make([]byte, 1))
				return err
			},
			wantErr: readErr,
		},
		{
			name:   "WriteTo",
			source: &testSource{kind: BLOB, writeToErr: writeToErr},
			operation: func(value *LOB) error {
				_, err := value.WriteTo(io.Discard)
				return err
			},
			wantErr: writeToErr,
		},
		{
			name:   "Size",
			source: &testSource{kind: BLOB, sizeErr: sizeErr},
			operation: func(value *LOB) error {
				_, err := value.Size()
				return err
			},
			wantErr: sizeErr,
		},
		{
			name:   "ChunkSize",
			source: &testSource{kind: BLOB, chunkErr: chunkErr},
			operation: func(value *LOB) error {
				_, err := value.ChunkSize()
				return err
			},
			wantErr: chunkErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value LOB
			if err := value.Scan(test.source); err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			if err := test.operation(&value); err != test.wantErr {
				t.Fatalf("operation error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// TestLOB_ClosePropagatesSourceError verifies Close returns a source cleanup
// error while keeping the LOB terminal and preventing a second close attempt.
func TestLOB_ClosePropagatesSourceError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("source close failed")
	source := &testSource{kind: BLOB, closeErr: closeErr}
	var value LOB
	if err := value.Scan(source); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if err := value.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v, want %v", err, closeErr)
	}
	if err := value.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
	if source.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", source.closeCalls)
	}

	// A zero-value LOB has no source, so Close must still complete successfully.
	var empty LOB
	if err := empty.Close(); err != nil {
		t.Fatalf("Close on zero-value LOB returned error: %v", err)
	}
}

// TestLOB_ScanAcceptsNonPointerSource verifies Scan accepts a non-pointer
// source, covering the non-pointer reflection path in isNilSource.
func TestLOB_ScanAcceptsNonPointerSource(t *testing.T) {
	t.Parallel()

	// A nil interface is handled before reflection and must be recognized as nil.
	if !isNilSource(nil) {
		t.Fatal("isNilSource(nil) = false, want true")
	}

	var value LOB
	if err := value.Scan(valueLOBSource{kind: BLOB}); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if !value.Valid() || value.Kind() != BLOB {
		t.Fatalf("LOB state = valid:%t kind:%d, want valid BLOB", value.Valid(), value.Kind())
	}
}

// valueLOBSource is a value-typed LOB source used to exercise reflection of a
// non-pointer implementation in LOB.Scan.
type valueLOBSource struct {
	kind Kind
}

func (source valueLOBSource) Read([]byte) (int, error)         { return 0, io.EOF }
func (source valueLOBSource) WriteTo(io.Writer) (int64, error) { return 0, nil }
func (valueLOBSource) Close() error                            { return nil }
func (valueLOBSource) Size() (int64, error)                    { return 0, nil }
func (valueLOBSource) ChunkSize() (int64, error)               { return 0, nil }
func (source valueLOBSource) Kind() Kind                       { return source.kind }
