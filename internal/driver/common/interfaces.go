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
	"context"
	"database/sql/driver"
)

const (
	NSFIMM = 0x0040 // non graceful disconnect
)

// NetworkSession implementors can disconnect a network connection
type NetworkSession interface {
	// Disconnect disconnects the network connection
	// Parameters:
	//   - context : the context to be used
	//   - flags: disconnect flags mask. Possible flags NSFIMM
	Disconnect(ctx context.Context, flags int) error

	// CancelOperation sends a message to the database to cancel the current
	// execution
	CancelOperation(ctx context.Context) error

	// CheckInbandNotification non-blocking call that checks if a inband
	// notification has been received.
	// Returns: true if an inband notification has been received otherwise false.
	CheckInbandNotification() bool

	// GetRemoteAddress returns the remote network address when it is available,
	// or an empty string otherwise.
	GetRemoteAddress() string

	// GetRemotePort returns the remote network port when it is available, or 0
	// otherwise.
	GetRemotePort() int
}

// DataBuffer The Marshaller uses this interface to marshal data
type DataBuffer interface {
	// WriteByteWithContext Writes one byte.
	WriteByteWithContext(context.Context, byte) error
	// WriteBytesWithContext WriteBytes Writes the entire content of a byte array.
	WriteBytesWithContext(context.Context, []byte) error

	// ReadByteWithContext ReadByte Reads one byte.
	ReadByteWithContext(context.Context) (byte, error)
	// ReadBytesWithContext ReadBytes Read the specified number of bytes and returns a byte array of that size.
	ReadBytesWithContext(context.Context, int32) (*[]byte, error)

	// Flush Flushes the data
	Flush(context.Context) error
}

// ConnectionInstantiator implementors can create connections to the database
type ConnectionInstantiator interface {
	// GetConnection returns a new connection to the database
	GetConnection(ctx context.Context) (driver.Conn, error)
}

// Kind identifies the high-level JSON node category.
type Kind uint8

const (

	// KindObject represents a JSON object node.
	KindObject Kind = iota
	// KindArray represents a JSON array node.
	KindArray
	// KindScalar represents a scalar JSON node.
	KindScalar
)

// JSONNumber is representation for number as string
type JSONNumber string

// MarshalJSON implements json.Marshaler.MarshalJSON
func (num JSONNumber) MarshalJSON() ([]byte, error) {
	return []byte(num), nil
}

// JSONOption controls JSON materialization behavior such as Number etc
type JSONOption uint8

const (
	// JSONOptDefault: returns value stored as NUMBER in DB as float64
	JSONOptDefault JSONOption = iota
	// JSONOptNumberAsString: returns value stored as NUMBER in DB as JSONNumber
	JSONOptNumberAsString
)

// JSONNode is a lazily decoded JSON value.
type JSONNode interface {
	// Kind returns the JSON value kind.
	Kind() Kind

	// GetValue materializes the JSON value using opts.
	GetValue(opts JSONOption) (any, error)

	// StringWithOption returns the JSON representation using opts.
	StringWithOption(opts JSONOption) (string, error)
}

// JSONObjectNode is a lazily decoded JSON object.
type JSONObjectNode interface {
	JSONNode

	// Get returns the value for key and whether key exists.
	Get(key string) (JSONNode, bool)

	// Len returns the number of object members.
	Len() int

	// Keys returns the object member names.
	Keys() []string

	// Value materializes the object using opts.
	Value(opts JSONOption) (map[string]any, error)
}

// JSONArrayNode is a lazily decoded JSON array.
type JSONArrayNode interface {
	JSONNode

	// Get returns the value at index and whether index is in range.
	Get(index int) (JSONNode, bool)

	// Len returns the number of array elements.
	Len() int

	// Value materializes the array using opts.
	Value(opts JSONOption) ([]any, error)
}

// JSONScalarNode is a lazily decoded JSON scalar.
type JSONScalarNode interface {
	JSONNode

	// Value materializes the scalar using opts.
	Value(opts JSONOption) (any, error)
}
