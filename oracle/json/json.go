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

// Package json binds and fetches Oracle Database native JSON values. Native
// JSON columns require Oracle Database 21c or later.
//
// # Binding
//
// [NewJSON] encodes supported Go values as OSON, Oracle's binary JSON format:
//
//	doc, err := json.NewJSON(map[string]any{"event": "created", "attempt": int64(1)})
//
// The returned [JSON] implements [database/sql/driver.Valuer] and can be passed
// directly to database/sql. Passing nil creates JSON null.
//
// [JSONString] binds pre-serialized JSON text. It validates the text but cannot
// represent OSON-native binary or temporal scalars.
//
// # Fetching
//
// Scan a JSON column into [JSON], then read it with [JSON.GetValue] or the
// lazy accessors [JSON.AsJSONObject], [JSON.AsJSONArray], and
// [JSON.AsJSONScalar]. Use sql.Null[JSON] for nullable columns.
//
// # Options
//
// [Options] controls number materialization and time.Time encoding. Set options
// with [NewJSONWithOptions] or [JSON.SetOptions]; they propagate to all lazy
// views. Use [JoinOptions] to combine independently selected options.
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

// JSONString is validated JSON text used as a database bind value. It cannot
// represent OSON-native binary or temporal scalars; use [NewJSON] for those.
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

// NewJSONFromString validates value as JSON text and returns it as a
// [JSONString] for binding. It rejects empty, whitespace-only, and malformed
// input. Use this for eager validation; [JSONString] defers validation to the
// bind boundary.
func NewJSONFromString(value string) (JSONString, error) {
	common.Odl.Debug("NewJSONFromString: start", "length", len(value))
	if !json.Valid([]byte(value)) {
		cause := fmt.Errorf("input is not valid JSON text")
		common.Odl.Debug("NewJSONFromString: failed", "error", cause)
		return "", common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}
	common.Odl.Debug("NewJSONFromString: completed", "length", len(value))
	return JSONString(value), nil
}

// JSON is an OSON-encoded JSON document that can be bound and scanned through
// database/sql.
//
// Construct a JSON with [NewJSON] or [NewJSONWithOptions], or scan one from a
// native Oracle JSON column. A successfully constructed or scanned JSON can be
// rebound without re-encoding.
//
// The zero JSON is uninitialized, not JSON null. Accessors return an error
// until NewJSON or Scan provides a document.
type JSON struct {
	node     drvCommon.JSONNode
	document []byte
	options  Options
}

// NewJSON encodes value as an OSON document using default options. Passing nil
// creates JSON null.
func NewJSON(value any) (JSON, error) {
	return NewJSONWithOptions(value, Options{})
}

// NewJSONWithOptions encodes value as an OSON document using opts. Time options
// select the OSON scalar used for time.Time values; number options control
// future GetValue calls.
func NewJSONWithOptions(value any, opts Options) (JSON, error) {
	document, err := oson.EncodeWithOptions(value, opts.conversion)
	if err != nil {
		return JSON{}, err
	}
	node, err := oson.Parse(document)
	if err != nil {
		return JSON{}, err
	}
	return JSON{node: node, document: document, options: opts}, nil
}

// SetOptions replaces the options associated with jz. It returns an error for a
// nil receiver.
func (jz *JSON) SetOptions(opts Options) error {
	if jz == nil {
		cause := fmt.Errorf("cannot set JSON options on a nil *JSON receiver")
		return common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "SetOptions")
	}
	jz.options = opts
	return nil
}

// Scan implements sql.Scanner for a native Oracle JSON column. On success it
// replaces jz's document and preserves its options. Scan returns an error for
// SQL NULL; use sql.Null[JSON] for nullable columns.
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
			jz.node = node
			jz.document = doc
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

// Value implements driver.Valuer. It returns the encoded OSON document for
// binding, or an error if jz is uninitialized. Child views are materialized
// and re-encoded using their current options, which may convert scalar values.
func (jz JSON) Value() (driver.Value, error) {
	if jz.node == nil {
		cause := fmt.Errorf("JSON has no OSON document to bind")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "Value")
	}
	if jz.document != nil {
		return append([]byte(nil), jz.document...), nil
	}

	common.Odl.Debug("JSON.Value: encoding child view", "options", jz.options.conversion)
	value, err := jz.node.GetValue(jz.options.conversion)
	if err != nil {
		return nil, err
	}
	document, err := oson.EncodeWithOptions(value, jz.options.conversion)
	if err != nil {
		return nil, err
	}
	return []byte(document), nil
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
		return JSONObject{node: obj, options: jz.options}, nil
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
		return JSONArray{node: arr, options: jz.options}, nil
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
		return JSONScalar{node: scalar, options: jz.options}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports scalar kind but does not implement JSONScalarNode", jz.node)
	return JSONScalar{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "scalar")
}

// GetValue materializes the complete document as Go values using jz's stored
// [Options]. Objects become map[string]any, arrays become []any, and scalars
// become their corresponding Go values.
func (jz JSON) GetValue() (any, error) {
	if jz.node == nil {
		cause := fmt.Errorf("JSON has no parsed OSON node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return jz.node.GetValue(jz.options.conversion)
}

// String returns JSON text for diagnostics. It returns "<JSON: uninitialized>"
// for the zero value and "<JSON: rendering failed>" if rendering fails.
func (jz JSON) String() string {
	if jz.node != nil {
		text, err := jz.node.String()
		if err != nil {
			return "<JSON: rendering failed>"
		}
		return text
	}

	return "<JSON: uninitialized>"
}

// JSONObject is a lazy view of an object in a fetched JSON document. Obtain one
// with [JSON.AsJSONObject].
type JSONObject struct {
	// node provides access to the underlying JSON object representation.
	node    drvCommon.JSONObjectNode
	options Options
}

// GetValue materializes the object subtree as map[string]any.
func (obj JSONObject) GetValue() (map[string]any, error) {
	if obj.node == nil {
		cause := fmt.Errorf("JSONObject has no underlying object node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return obj.node.Value(obj.options.conversion)
}

// Keys returns the names of the object's members. Their order is unspecified.
// It returns nil for the zero uninitialized JSONObject.
func (obj JSONObject) Keys() []string {
	if obj.node == nil {
		return nil
	}
	return obj.node.Keys()
}

// Get returns a lazy child JSON for key and true if it exists, or a zero JSON
// and false otherwise.
func (obj JSONObject) Get(key string) (JSON, bool) {
	if obj.node != nil {
		node, ok := obj.node.Get(key)
		if !ok {
			return JSON{}, false
		}
		return JSON{node: node, options: obj.options}, true
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

// JSONArray is a lazy view of an array in a fetched JSON document. Obtain one
// with [JSON.AsJSONArray].
type JSONArray struct {
	// node provides access to the underlying JSON array representation.
	node    drvCommon.JSONArrayNode
	options Options
}

// Len returns the number of elements in the JSON array, or -1 if arr is the zero
// uninitialized JSONArray.
func (arr JSONArray) Len() int {
	if arr.node == nil {
		return -1
	}
	return arr.node.Len()
}

// GetValue materializes the array subtree as []any.
func (arr JSONArray) GetValue() ([]any, error) {
	if arr.node == nil {
		cause := fmt.Errorf("JSONArray has no underlying array node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return arr.node.Value(arr.options.conversion)
}

// Get returns the lazy child JSON at index i. It returns an error if i is
// outside [0, Len()).
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
	return JSON{node: node, options: arr.options}, nil
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

// JSONScalar is a lazy view of a scalar in a fetched JSON document. Scalars
// include JSON null, booleans, strings, numbers, and OSON-native date,
// timestamp, interval, and binary values. Obtain one with [JSON.AsJSONScalar].
type JSONScalar struct {
	// node provides access to the underlying JSON scalar representation.
	node    drvCommon.JSONScalarNode
	options Options
}

// GetValue materializes the scalar as its corresponding Go value.
func (scalar JSONScalar) GetValue() (any, error) {
	if scalar.node == nil {
		cause := fmt.Errorf("JSONScalar has no underlying scalar node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return scalar.node.Value(scalar.options.conversion)
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
