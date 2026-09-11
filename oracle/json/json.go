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

// Package json binds and fetches values stored in the native Oracle Database
// JSON type. Native JSON columns require Oracle Database 21c or later.
//
// # Choosing how to bind JSON
//
// Use [JSONString] when the value is already JSON text. JSONString validates the
// complete document before it reaches the database:
//
//	_, err := db.ExecContext(ctx,
//		"insert into events (id, payload) values (:1, :2)",
//		1,
//		json.JSONString(`{"event":"created","attempt":1}`),
//	)
//
// Use [JSON] with [JSON.Data] when the value is made of supported Go values.
// The driver encodes Data directly as Oracle's binary JSON format, OSON:
//
//	_, err := db.ExecContext(ctx,
//		"insert into events (id, payload) values (:1, :2)",
//		2,
//		json.JSON{Data: map[string]any{
//			"event":   "created",
//			"attempt": int64(1),
//		}},
//	)
//
// These bind forms are intentionally different. JSONString is convenient when
// an application already has JSON text, but text cannot carry OSON-native
// values such as a binary value or time.Time, etc. JSON.Data supports those values
// and there are preserved during when fetched from the database.
//
// JSON.Data accepts only the concrete types documented on that field. It does
// not apply encoding/json marshaling to arbitrary structs, pointers, typed maps,
// typed slices, json.RawMessage, or user-defined aliases. Convert such a value
// to map[string]any, []any, or JSONString before binding it.
//
// # Binding and fetching use different JSON states
//
// A JSON value constructed with Data is a bind value. A JSON value populated by
// database/sql.Scan contains a parsed OSON node instead. If a JSON value has a
// parsed node, that fetched value takes precedence: [JSON.Value] and
// [JSON.String] ignore Data. Consequently, do not scan into a JSON value and
// then assign its Data field to replace the document. Construct a new JSON
// value for the new bind. A fetched JSON value can itself be rebound when the
// same document should be sent again (or its descendants).
//
// Fetch a non-NULL JSON column into *JSON:
//
//	var doc json.JSON
//	err := db.QueryRowContext(ctx,
//		"select payload from events where id = :1", 1,
//	).Scan(&doc)
//	if err != nil {
//		return err
//	}
//
// For a nullable column, use sql.Null[JSON]. Scanning SQL NULL directly into
// *JSON is not supported:
//
//	var doc sql.Null[json.JSON]
//	err := db.QueryRowContext(ctx, query, id).Scan(&doc)
//	if err != nil {
//		return err
//	}
//	if !doc.Valid {
//		// The database value was SQL NULL.
//	}
//
// SQL NULL and the JSON value null are different. SQL NULL makes the Null value
// invalid; a JSON null is a valid JSON document whose materialized Go value is
// nil.
//
// # Reading a fetched value
//
// [JSON.GetValue] materializes a complete document as ordinary Go values.
// Objects become map[string]any, arrays become []any, and scalars become their
// corresponding Go values. Number handling is selected for the whole subtree:
//
//   - [JSONOptDefault] returns numbers as float64. This is familiar and
//     convenient, but large integers and exact decimals can be rounded.
//   - [JSONOptNumberAsString] returns numbers as [Number], preserving their
//     decimal text.
//
// Use [JSON.Kind] and the object, array, or scalar wrapper when only part of a
// large document is needed. The wrappers navigate the encoded document lazily,
// meaning that they read only the metadata and offsets needed to locate a
// value; child values remain encoded until they are requested. GetValue then
// materializes the selected subtree, while String renders it as JSON text.
// String provides display text with diagnostic markers on rendering failure.
// Materialization options are supplied to GetValue. When navigating to a child
// JSON value, obtain its wrapper first and supply the desired option when
// materializing that child.
package json

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/oson"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// JSONOption controls how numbers are represented when a fetched JSON or one of
// its subtrees is materialized as Go values. An option applies recursively to
// every number in that materialized value.
type JSONOption = drvCommon.JSONOption

const (
	// JSONOptDefault materializes JSON numbers as float64, matching the default
	// behavior of encoding/json when decoding into any. A float64 cannot exactly
	// represent every JSON integer or decimal; use JSONOptNumberAsString when
	// exact decimal digits matter.
	JSONOptDefault JSONOption = drvCommon.JSONOptDefault
	// JSONOptNumberAsString materializes JSON numbers as [Number], preserving
	// their decimal text instead of first converting them to float64. Number is
	// still marshaled as a JSON number, not as a quoted JSON string.
	JSONOptNumberAsString JSONOption = drvCommon.JSONOptNumberAsString
)

// Number contains the decimal text of a JSON number without float64 rounding.
// It is analogous to encoding/json.Number and is marshaled as an unquoted JSON
// number even though its Go representation is string-backed.
//
// When used as bind input, Number must contain exactly one valid JSON number
// token with no surrounding whitespace. Invalid text causes [JSON.Value] to
// return an OSON encoding error.
type Number = drvCommon.JSONNumber

// JSONKind identifies whether the root of a fetched JSON value is an object, an
// array, or a scalar. JSON null is a scalar.
type JSONKind = drvCommon.Kind

const (
	// JSONObjectKind represents a JSON object.
	JSONObjectKind JSONKind = drvCommon.KindObject
	// JSONArrayKind represents a JSON array.
	JSONArrayKind JSONKind = drvCommon.KindArray
	// JSONScalarKind represents a JSON scalar.
	JSONScalarKind JSONKind = drvCommon.KindScalar
)

// JSONString is validated JSON text used as a database bind value.
//
// Use JSONString for a complete document that is already serialized. It must
// contain valid JSON; invalid text is rejected before execution. Do not use
// JSONString for plain application strings that should become JSON string
// scalars: use JSON{Data: value} so the driver applies the required JSON
// quoting.
type JSONString string

// Value implements driver.Valuer.
func (jz JSONString) Value() (driver.Value, error) {
	common.Odl.Debug("JSONString.Value: start", "length", len(jz))
	if !json.Valid([]byte(jz)) {
		cause := fmt.Errorf("JSONString contains invalid JSON text")
		common.Odl.Debug("JSONString.Value: failed", "error", cause)
		return nil, common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}
	common.Odl.Debug("JSONString.Value: completed", "length", len(jz))
	return string(jz), nil
}

// JSON is both the bind container for supported Go values and the scan target
// for a native Oracle Database JSON value.
//
// Construct a new JSON and set [JSON.Data] when binding a Go value. Scan a
// database JSON column into *JSON when fetching. These are separate states: a
// parsed value populated by Scan takes precedence over Data in Value and
// String. The zero JSON value binds and renders as the JSON value null, but it
// has no parsed node and therefore cannot be inspected with Kind, GetValue, or
// the shape-specific accessors until populated by Scan.
type JSON struct {
	// node provides access to the underlying JSON representation.
	node drvCommon.JSONNode
	// Data is the Go value to encode as OSON for a JSON bind. The supported
	// concrete types are:
	//   - nil, bool, string, int, int8, int16, int32, int64, uint, uint8,
	//     uint16, uint32, uint64, float32, float64, []byte, and time.Time
	//   - Number and encoding/json.Number containing valid JSON number text
	//   - map[string]any and []any, whose values recursively use these types
	//
	// The types are matched exactly. Structs, pointers, json.RawMessage,
	// user-defined aliases, typed maps such as map[string]string, and typed slices
	// such as []string are not supported. Convert them to one of the concrete
	// types above or bind already-serialized text with JSONString.
	//
	// []byte is encoded as an OSON binary scalar, not parsed as JSON text.
	//
	// Data is used only when this JSON has not been populated by Scan. Once Scan
	// has installed a parsed OSON node, that node is authoritative and Data is
	// ignored by Value and String. Use a new JSON value to bind replacement data.
	Data any
}

// Scan implements sql.Scanner.
func (jz *JSON) Scan(src any) error {
	common.Odl.Debug("JSON.Scan: start", "source_type", fmt.Sprintf("%T", src))
	if jz == nil {
		cause := fmt.Errorf("cannot scan Oracle JSON into a nil *JSON receiver")
		common.Odl.Debug("JSON.Scan: failed", "error", cause)
		return common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "Scan")
	}

	switch value := src.(type) {
	case []byte:
		if len(value) >= 4 && oson.IsOson(value) {
			// copy the buffer
			doc := append([]byte(nil), value...)
			// creating a OSON node
			node, err := oson.Parse(doc)
			if err != nil {
				common.Odl.Debug("JSON.Scan: failed", "error", err, "length", len(value))
				return err
			}
			*jz = JSON{node: node}
			common.Odl.Debug("JSON.Scan: completed", "length", len(value))
			return nil
		}
		cause := fmt.Errorf("cannot scan %d-byte []byte as Oracle JSON: expected an OSON document beginning with magic bytes FF 4A 5A", len(value))
		common.Odl.Debug("JSON.Scan: failed", "error", cause, "length", len(value))
		return common.NewOracleError(oracleErrors.OsonHeaderError, cause)
	default:
		cause := fmt.Errorf("source type %T cannot be scanned as Oracle JSON", src)
		common.Odl.Debug("JSON.Scan: failed", "error", cause)
		return common.NewOracleError(oracleErrors.JSONScanTypeUnsupportedError, cause, fmt.Sprintf("%T", src))
	}
}

// Value implements driver.Valuer.
func (jz JSON) Value() (driver.Value, error) {
	common.Odl.Debug("JSON.Value: start", "data_type", fmt.Sprintf("%T", jz.Data), "has_node", jz.node != nil)
	data := jz.Data

	if jz.node != nil {
		var err error
		data, err = jz.node.GetValue(drvCommon.JSONOptNumberAsString)
		if err != nil {
			common.Odl.Debug("JSON.Value: failed", "error", err)
			return nil, err
		}
	}

	doc, err := oson.Encode(data)
	if err != nil {
		common.Odl.Debug("JSON.Value: failed", "error", err)
		return nil, err
	}

	common.Odl.Debug("JSON.Value: completed", "length", len(doc))
	return []byte(doc), nil
}

// Kind reports whether the root of a fetched JSON value is JSONObjectKind,
// JSONArrayKind, or JSONScalarKind. JSON null is a scalar.
func (jz JSON) Kind() (JSONKind, error) {
	if jz.node == nil {
		cause := fmt.Errorf("JSON has no parsed OSON node to inspect")
		return 0, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "Kind")
	}

	return jz.node.Kind(), nil
}

// AsJSONObject returns a lazy object view of a fetched JSON value. It returns
// an error if jz is uninitialized or its root is not an object.
func (jz JSON) AsJSONObject() (JSONObject, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONObject{}, err
	}
	if kind != JSONObjectKind {
		cause := fmt.Errorf("JSON node kind %d cannot be accessed as an object", kind)
		return JSONObject{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "object")
	}

	if obj, ok := jz.node.(drvCommon.JSONObjectNode); ok {
		return JSONObject{node: obj}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports object kind but does not implement JSONObjectNode", jz.node)
	return JSONObject{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "object")
}

// AsJSONArray returns a lazy array view of a fetched JSON value. It returns an
// error if jz is uninitialized or its root is not an array.
func (jz JSON) AsJSONArray() (JSONArray, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONArray{}, err
	}
	if kind != JSONArrayKind {
		cause := fmt.Errorf("JSON node kind %d cannot be accessed as an array", kind)
		return JSONArray{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "array")
	}

	if arr, ok := jz.node.(drvCommon.JSONArrayNode); ok {
		return JSONArray{node: arr}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports array kind but does not implement JSONArrayNode", jz.node)
	return JSONArray{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "array")
}

// AsJSONScalar returns a lazy scalar view of a fetched JSON value. Objects and
// arrays are not scalars; JSON null is. The method returns an error if jz is
// uninitialized or its root is not a scalar.
func (jz JSON) AsJSONScalar() (JSONScalar, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONScalar{}, err
	}
	if kind != JSONScalarKind {
		cause := fmt.Errorf("JSON node kind %d cannot be accessed as a scalar", kind)
		return JSONScalar{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "scalar")
	}
	if scalar, ok := jz.node.(drvCommon.JSONScalarNode); ok {
		return JSONScalar{node: scalar}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports scalar kind but does not implement JSONScalarNode", jz.node)
	return JSONScalar{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "scalar")
}

// GetValue materializes an entire fetched JSON document as Go values, applying
// opts recursively. Objects become map[string]any, arrays become []any, and
// scalars become their corresponding Go values. It returns an error when jz has
// not been populated by Scan.
func (jz JSON) GetValue(opts JSONOption) (any, error) {
	if jz.node == nil {
		cause := fmt.Errorf("JSON has no parsed OSON node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return jz.node.GetValue(opts)
}

// String implements fmt.Stringer, returning JSON text for display.
// It returns "<JSON: rendering failed>" if rendering fails.
//
// For a fetched JSON, String renders the parsed OSON value and ignores Data. For
// a caller-constructed JSON, it marshals Data with encoding/json. String is
// therefore not a preview of OSON-native bind semantics: for example,
// encoding/json renders []byte as a base64 string, while Value encodes it as a
// binary scalar. String can also succeed for a Go type that Value does not
// support, or fail for an OSON value such as a non-finite float that Value does
// support. Use a successful Value or database execution to validate bind input.
// The zero JSON value renders as "null".
func (jz JSON) String() string {
	if jz.node != nil {
		text, err := jz.node.String()
		if err != nil {
			return "<JSON: rendering failed>"
		}
		return text
	}

	text, err := json.Marshal(jz.Data)
	if err != nil {
		return "<JSON: rendering failed>"
	}
	return string(text)
}

// JSONObject is a lazy view of an object in a fetched OSON document. Obtain one
// with [JSON.AsJSONObject]. Supply the materialization option to GetValue;
// String is independent of that option.
type JSONObject struct {
	// node provides access to the underlying JSON object representation.
	node drvCommon.JSONObjectNode
}

// GetValue materializes the complete object subtree as map[string]any using
// opts recursively.
func (obj JSONObject) GetValue(opts JSONOption) (map[string]any, error) {
	if obj.node == nil {
		cause := fmt.Errorf("JSONObject has no underlying object node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return obj.node.Value(opts)
}

// Keys returns the names of the object's members. Their order is unspecified.
// It returns nil for the zero uninitialized JSONObject.
func (obj JSONObject) Keys() []string {
	if obj.node == nil {
		return nil
	}
	return obj.node.Keys()
}

// Contains reports whether key exists in the object. It returns false for an
// absent key and for the zero uninitialized JSONObject.
func (obj JSONObject) Contains(key string) bool {
	if obj.node != nil {
		_, ok := obj.node.Get(key)
		return ok
	}

	return false
}

// Get returns a lazy child JSON value and true when key exists. It returns a
// zero JSON and false when the key is absent or obj is uninitialized.
//
// Supply the desired option when materializing the returned JSON.
func (obj JSONObject) Get(key string) (JSON, bool) {
	if obj.node != nil {
		node, ok := obj.node.Get(key)
		if !ok {
			return JSON{}, false
		}
		return JSON{node: node}, true
	}
	return JSON{}, false
}

// String implements fmt.Stringer, returning JSON text for display.
// It returns "<JSONObject: uninitialized>" for the zero value and
// "<JSONObject: rendering failed>" if rendering fails.
func (obj JSONObject) String() string {
	if obj.node == nil {
		return "<JSONObject: uninitialized>"
	}
	text, err := obj.node.String()
	if err != nil {
		return "<JSONObject: rendering failed>"
	}
	return text
}

// JSONArray is a lazy view of an array in a fetched OSON document. Obtain one
// with [JSON.AsJSONArray]. Supply the materialization option to GetValue;
// String is independent of that option.
type JSONArray struct {
	// node provides access to the underlying JSON array representation.
	node drvCommon.JSONArrayNode
}

// Len returns the number of elements in the JSON array, or -1 if arr is the zero
// uninitialized JSONArray.
func (arr JSONArray) Len() int {
	if arr.node == nil {
		return -1
	}
	return arr.node.Len()
}

// GetValue materializes the complete array subtree as []any, preserving element
// order, using opts recursively.
func (arr JSONArray) GetValue(opts JSONOption) ([]any, error) {
	if arr.node == nil {
		cause := fmt.Errorf("JSONArray has no underlying array node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return arr.node.Value(opts)
}

// Get returns the lazy child JSON value at zero-based index i. It returns an
// error when arr is uninitialized or i is outside [0, Len()).
//
// Supply the desired option when materializing the returned JSON.
func (arr JSONArray) Get(i int) (JSON, error) {
	if arr.node == nil {
		cause := fmt.Errorf("JSONArray has no underlying array node to index")
		return JSON{}, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "Get")
	}

	node, ok := arr.node.Get(i)
	if !ok {
		cause := fmt.Errorf("array index %d is outside the valid range [0,%d)", i, arr.node.Len())
		return JSON{}, common.NewOracleError(oracleErrors.JSONArrayIndexOutOfRangeError, cause, i)
	}
	return JSON{node: node}, nil
}

// String implements fmt.Stringer, returning JSON text for display.
// It returns "<JSONArray: uninitialized>" for the zero value and
// "<JSONArray: rendering failed>" if rendering fails.
func (arr JSONArray) String() string {
	if arr.node == nil {
		return "<JSONArray: uninitialized>"
	}
	text, err := arr.node.String()
	if err != nil {
		return "<JSONArray: rendering failed>"
	}
	return text
}

// JSONScalar is a lazy view of a scalar in a fetched OSON document. Scalars
// include JSON null, booleans, strings, numbers, and OSON-native date, timestamp,
// interval, and binary values. Obtain one with [JSON.AsJSONScalar].
type JSONScalar struct {
	// node provides access to the underlying JSON scalar representation.
	node drvCommon.JSONScalarNode
}

// GetValue materializes the scalar as its corresponding Go value, using opts
// for a number.
func (scalar JSONScalar) GetValue(opts JSONOption) (any, error) {
	if scalar.node == nil {
		cause := fmt.Errorf("JSONScalar has no underlying scalar node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return scalar.node.Value(opts)
}

// String implements fmt.Stringer, returning JSON text for display.
// It returns "<JSONScalar: uninitialized>" for the zero value and
// "<JSONScalar: rendering failed>" if rendering fails.
func (scalar JSONScalar) String() string {
	if scalar.node == nil {
		return "<JSONScalar: uninitialized>"
	}
	text, err := scalar.node.String()
	if err != nil {
		return "<JSONScalar: rendering failed>"
	}
	return text
}
