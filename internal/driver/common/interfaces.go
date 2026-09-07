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

// Kind identifies the shape of a JSONNode.
//
// OSON has several scalar encodings, but callers generally need to distinguish
// only objects, arrays, and scalars. Kind deliberately describes that JSON
// shape rather than an OSON opcode or an Oracle database type.
type Kind uint8

const (
	// KindObject identifies an object node.
	KindObject Kind = iota
	// KindArray identifies an array node.
	KindArray
	// KindScalar identifies a scalar node, including null.
	KindScalar
)

// JSONNumber holds the decimal text of a JSON number.
//
// JSONNumber exists so an OSON NUMBER can be materialized without first being
// rounded to float64. It is analogous to [encoding/json.Number]: although its
// Go representation is a string, MarshalJSON writes it as a JSON number rather
// than as a quoted JSON string.
//
// A JSONNumber is expected to contain a valid JSON number. MarshalJSON does not
// validate values constructed directly by a caller.
type JSONNumber string

// MarshalJSON returns num as an unquoted JSON number.
func (num JSONNumber) MarshalJSON() ([]byte, error) {
	return []byte(num), nil
}

// JSONOption controls how an OSON node is converted to ordinary Go values.
// Options affect the whole value: an object or array passes the selected option
// to every descendant it materializes.
type JSONOption uint8

const (
	// JSONOptDefault materializes JSON numbers as float64. This matches the
	// default representation used when encoding/json decodes into an any, but
	// conversion can round integers and decimals that float64 cannot represent
	// exactly.
	JSONOptDefault JSONOption = iota

	// JSONOptNumberAsString materializes JSON numbers as JSONNumber. Use this
	// option when decimal digits must survive decoding, comparison, and a later
	// JSON or OSON encoding without float64 rounding. The result remains a JSON
	// number, not a JSON string.
	JSONOptNumberAsString
)

// JSONNode represents one value in an OSON byte sequence.
//
// Decoding is lazy: creating a node reads only the OSON bytes needed to identify
// that node and locate its direct children. It does not decode the child values.
// Get creates another lazy node for one child. GetValue, Value, and
// StringWithOption convert values to Go types and, for a container, decode the
// complete subtree. This lets callers inspect a large document without
// converting parts they do not need.
//
// Implementations must propagate the requested JSONOption through the entire
// subtree and return an error if any selected child cannot be decoded. The
// materialized forms are map[string]any for objects, []any for arrays, and the
// corresponding Go value for scalars.
type JSONNode interface {
	// Kind reports whether the node is an object, array, or scalar.
	Kind() Kind

	// GetValue recursively materializes the node using opts.
	//
	// GetValue returns any because the result type depends on Kind. The Value
	// methods on JSONObjectNode, JSONArrayNode, and JSONScalarNode provide the
	// same operation with a shape-specific result type. An implementation of a
	// specialized node must make Value(opts) and GetValue(opts) describe the
	// same value; Value exists separately because Go interface methods cannot
	// refine an any return type to map[string]any or []any.
	GetValue(opts JSONOption) (any, error)

	// StringWithOption returns the JSON text representation of the node using
	// opts while materializing numeric descendants. It returns an error if the
	// node cannot be decoded or represented as JSON.
	StringWithOption(opts JSONOption) (string, error)
}

// JSONObjectNode is a JSONNode whose Kind is KindObject.
//
// An object member is one field name and its associated child value. Creating
// an object node reads the object opcode, member count, field names, and the
// byte offset of each child from the OSON bytes. It stops there: the child
// values remain encoded. Get creates a lazy node for one child; Value decodes
// them all.
type JSONObjectNode interface {
	JSONNode

	// Get returns the child node named by key. The result is false when key is
	// absent or its node cannot be constructed.
	Get(key string) (JSONNode, bool)

	// Len returns the number of object members. Each member is one field name and
	// its associated child value.
	Len() int

	// Keys returns the field name of every object member. Their order is
	// unspecified.
	Keys() []string

	// Value recursively materializes the object as a map. It is the typed form
	// of GetValue; both methods must apply opts to every member.
	Value(opts JSONOption) (map[string]any, error)
}

// JSONArrayNode is a JSONNode whose Kind is KindArray.
//
// Creating an array node reads the array opcode, element count, and the byte
// offset of each child from the OSON bytes. It stops there: the child values
// remain encoded. Get creates a lazy node for one child; Value decodes them all.
type JSONArrayNode interface {
	JSONNode

	// Get returns the child node at the zero-based index. The result is false
	// when index is out of range or its node cannot be constructed.
	Get(index int) (JSONNode, bool)

	// Len returns the number of array elements.
	Len() int

	// Value recursively materializes the array as a slice, preserving element
	// order. It is the typed form of GetValue; both methods must apply opts to
	// every element.
	Value(opts JSONOption) ([]any, error)
}

// JSONScalarNode is a JSONNode whose Kind is KindScalar.
//
// In addition to the RFC 8259 scalar values, OSON can represent native Oracle
// scalar values such as dates, timestamps, intervals, and binary data. Value
// returns the driver's corresponding Go representation for those values.
type JSONScalarNode interface {
	JSONNode

	// Value decodes the scalar using opts. It is the typed-shape counterpart of
	// GetValue; for a scalar both return any because the concrete Go type depends
	// on the OSON scalar encoding.
	Value(opts JSONOption) (any, error)
}
