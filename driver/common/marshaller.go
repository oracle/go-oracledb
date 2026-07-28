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

package common

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

// UB1 Unsigned type
type UB1 uint8

// UB2 Unsigned type
type UB2 uint16

// UB4 Unsigned type
type UB4 uint32

// UB8 Unsigned type
type UB8 uint64

// SB1 Signed type
type SB1 int8

// SB2 Signed types
type SB2 int16

// SB4 Signed types
type SB4 int32

// B1Array Arrays
type B1Array []byte

func (b1a B1Array) String() string {
	return B1ArrayToString(b1a)
}
func (b1a B1Array) Equals(other B1Array) bool {
	return bytes.Equal(b1a, other)
}

// KeyValue Key-value pairs
type KeyValue struct {
	Key   B1Array
	Value B1Array
	Flag  SB4
}

func (kvl *KeyValue) String() string {
	res := strings.Builder{}
	res.WriteString("[")
	res.WriteString(B1ArrayToString(kvl.Key))
	res.WriteString("=")
	res.WriteString(B1ArrayToString(kvl.Value))
	res.WriteString(",")
	res.WriteString(strconv.Itoa(int(kvl.Flag)))
	res.WriteString("]")
	return res.String()
}
func (kv *KeyValue) Equals(other *KeyValue) bool {
	if kv == other {
		return true
	}
	if kv.Flag != other.Flag {
		return false
	}
	if !kv.Key.Equals(other.Key) {
		return false
	}
	if !kv.Value.Equals(other.Value) {
		return false
	}
	return true
}

// Marshaller Interface to read and write data from data packets
type Marshaller interface {
	// MarshalUB1 Writes an unsigned byte
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalUB1(context.Context, UB1) error
	// MarshalUB2 Writes two unsigned bytes
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalUB2(context.Context, UB2) error
	// MarshalUB4 Writes four unsigned bytes
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalUB4(context.Context, UB4) error
	// MarshalUB8 Writes eight unsigned bytes
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalUB8(context.Context, UB8) error

	// MarshalSB4 Writes four signed bytes
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalSB4(context.Context, SB4) error

	// MarshalB1Array Writes a byte array, or slice
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalB1Array(context.Context, B1Array) error

	// MarshalPTR Writes a non null pointer
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalPTR(context.Context) error
	// MarshalNullPTR Writes a null pointer
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalNullPTR(context.Context) error

	// MarshalChar Writes a byte array representing an array of Chars
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalChar(context.Context, B1Array) error

	// MarshalCLR Write a CLR
	// Returns:
	//  - An error if the marshaling operation fails.
	MarshalCLR(context.Context, B1Array, int, int) error

	// UnmarshalUB1 Reads an unsigned byte
	// Returns:
	//  - The unsigned byte
	//  - An error if the marshaling operation fails.
	UnmarshalUB1(context.Context) (UB1, error)

	// UnmarshalUB2 Reads a two unsigned bytes long value
	// Returns:
	//  - The two unsigned bytes long value
	//  - An error if the marshaling operation fails.
	UnmarshalUB2(context.Context) (UB2, error)

	// UnmarshalUB4 Reads a four unsigned bytes long value
	// Returns:
	//  - The four unsigned bytes long value
	//  - An error if the marshaling operation fails.
	UnmarshalUB4(context.Context) (UB4, error)

	// UnmarshalUB8 Reads a eight unsigned bytes long value
	// Returns:
	//  - The four unsigned bytes long value
	//  - An error if the marshaling operation fails.
	UnmarshalUB8(context.Context) (UB8, error)

	// UnmarshalSB1 Reads a signed byte
	// Returns:
	//  - The signed byte
	//  - An error if the marshaling operation fails.
	UnmarshalSB1(context.Context) (SB1, error)

	// UnmarshalSB2 Reads a two signed bytes long value
	// Returns:
	//  - The two signed bytes long value
	//  - An error if the marshaling operation fails.
	UnmarshalSB2(context.Context) (SB2, error)

	// UnmarshalSB4 Reads a four signed bytes long value
	// Returns:
	//  - The four signed bytes long value
	//  - An error if the marshaling operation fails.
	UnmarshalSB4(context.Context) (SB4, error)

	// UnmarshalCLR Reads a CLR
	// Parameters:
	//  - The address of a B1Array where the bytes will be written
	//  - The maximum length to read
	// Returns:
	//  - The actual length read
	//  - An error if the marshaling operation fails.
	UnmarshalCLR(context.Context, B1Array, int) (int, error)

	// UnmarshalCLRColumnData Reads a CLR
	// Returns:
	//  - The address of a B1Array containing the bytes
	//  - The actual length read
	//  - An error if the marshaling operation fails.
	UnmarshalCLRColumnData(context.Context) (B1Array, int, error)

	// UnmarshalB1Array Reads an array of bytes
	// Parameters:
	//  - The length of the byte array to read
	// Returns:
	//  - The byte array
	//  - An error if the marshaling operation fails.
	UnmarshalB1Array(context.Context, int) (B1Array, error) // previously called UnmarshalNBytes2

	// UnmarshalText Reads a text of specified length. The read stop if the end os test marker is found.
	// Parameters:
	//  - The maximum length of the text to read
	// Returns:
	//  - The byte array
	//  - An error if the marshaling operation fails.
	UnmarshalText(context.Context, int) (B1Array, error)

	// Flush Flushes the buffer
	// Returns:
	//  - An error if the flush operation fails.
	Flush(context.Context) error
}
