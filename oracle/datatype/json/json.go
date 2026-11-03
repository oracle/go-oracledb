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
// [JSONString] binds pre-serialized JSON text. It validates the text at the
// bind boundary but cannot represent OSON-native binary or temporal scalars.
//
// # Fetching
//
// Scan a JSON column into [JSON], then read it with [JSON.GetValue] or the
// lazy accessors [JSON.AsJSONObject], [JSON.AsJSONArray], and
// [JSON.AsJSONScalar]. Use sql.Null[JSON] for nullable columns.
//
// # Standard Library Interop
//
// [JSON], [JSONObject], [JSONArray], and [JSONScalar] implement
// [encoding/json.Marshaler], so [encoding/json.Marshal] renders JSON text and
// reports rendering errors. [JSON] also implements [encoding/json.Unmarshaler]:
// [encoding/json.Unmarshal] decodes standard JSON text into an OSON document,
// preserving the text representation of JSON numbers.
//
// # Options
//
// [Options] controls number materialization and time.Time encoding. Set options
// with [NewJSONWithOptions] or [JSON.SetOptions]; they propagate to all lazy
// views. Set fields directly in an Options struct literal.
package json

import (
	"bytes"
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

// JSONString is JSON text used as a database bind value. Its validity is
// checked at the bind boundary. It cannot represent OSON-native binary or
// temporal scalars; use [NewJSON] for those.
type JSONString string

// Value implements driver.Valuer.
func (jz JSONString) Value() (driver.Value, error) {
	if !json.Valid([]byte(jz)) {
		common.Odl.Debug("JSONString.Value: failed", "reason", "invalid JSON text", "length", len(jz))
		return nil, common.NewOracleError(oracleErrors.OsonEncodingError, nil)
	}
	if common.Odl.Enabled(common.BackgroundContext, common.OlFine) {
		common.Odl.Fine("JSONString.Value: completed", "length", len(jz))
	}
	return string(jz), nil
}

// NewJSONFromString returns value as a [JSONString] for binding. The text is
// validated at the bind boundary by [JSONString.Value].
func NewJSONFromString(value string) JSONString {
	if common.Odl.Enabled(common.BackgroundContext, common.OlFine) {
		common.Odl.Fine("NewJSONFromString: completed", "length", len(value))
	}
	return JSONString(value)
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
// creates JSON null. Typed nil maps and slices create empty objects and arrays;
// a nil []byte creates an empty binary scalar.
func NewJSON(value any) (JSON, error) {
	return NewJSONWithOptions(value, Options{})
}

// NewJSONWithOptions encodes value as an OSON document using opts. Time options
// select the OSON scalar used for time.Time values; number options control
// future GetValue calls.
func NewJSONWithOptions(value any, opts Options) (JSON, error) {
	document, err := oson.EncodeWithOptions(value, drvCommon.JSONConversionOptions(opts))
	if err != nil {
		return JSON{}, err
	}
	node, err := oson.Parse(document)
	if err != nil {
		return JSON{}, err
	}
	return JSON{node: node, document: document, options: opts}, nil
}

// SetOptions replaces all options associated with jz without re-encoding its
// document. Previously obtained views retain their options. Future views inherit
// the new options. NumberMode does not affect MarshalJSON or String.
func (jz *JSON) SetOptions(opts Options) {
	jz.options = opts
}

// UnmarshalJSON implements encoding/json.Unmarshaler. It replaces jz with an
// OSON document decoded from standard JSON text, preserving JSON numbers as
// json.Number values.
func (jz *JSON) UnmarshalJSON(data []byte) error {
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		common.Odl.Debug("JSON.UnmarshalJSON: failed", "reason", "invalid JSON text", "error", err)
		return common.NewOracleError(oracleErrors.OsonEncodingError, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		common.Odl.Debug("JSON.UnmarshalJSON: failed", "reason", "decode JSON text", "error", err)
		return common.NewOracleError(oracleErrors.OsonEncodingError, err)
	}

	doc, err := NewJSONWithOptions(value, jz.options)
	if err != nil {
		common.Odl.Debug("JSON.UnmarshalJSON: failed", "reason", "encode JSON document", "error", err)
		return err
	}
	*jz = doc
	return nil
}

// Scan implements sql.Scanner for a native Oracle JSON column. On success it
// replaces jz's document and preserves its options. Scan returns an error for
// SQL NULL; use sql.Null[JSON] for nullable columns.
func (jz *JSON) Scan(src any) error {
	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("JSON.Scan: start", "source_type", fmt.Sprintf("%T", src))
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
			if common.Odl.Enabled(common.BackgroundContext, common.OlFine) {
				common.Odl.Fine("JSON.Scan: completed", "length", len(value))
			}
			return nil
		}
		common.Odl.Debug("JSON.Scan: failed", "reason", "byte slice is not an OSON document", "length", len(value))
		return common.NewOracleError(oracleErrors.InternalError, nil)
	default:
		sourceType := fmt.Sprintf("%T", src)
		common.Odl.Debug("JSON.Scan: failed", "reason", "unsupported source type", "sourceType", sourceType)
		return common.NewOracleError(oracleErrors.JSONScanTypeUnsupportedError, nil, sourceType)
	}
}

// Value implements driver.Valuer. It returns the encoded OSON document for
// binding, or an error if jz is uninitialized. Child views are materialized
// and re-encoded using their current options, which may convert scalar values.
func (jz JSON) Value() (driver.Value, error) {
	if jz.node == nil {
		common.Odl.Debug("JSON.Value: failed", "reason", "uninitialized JSON value")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "Value")
	}
	if jz.document != nil {
		return append([]byte(nil), jz.document...), nil
	}

	if common.Odl.Enabled(common.BackgroundContext, common.OlFinest) {
		common.Odl.Finest("JSON.Value: encoding child view", "options", drvCommon.JSONConversionOptions(jz.options))
	}
	value, err := jz.node.GetValue(drvCommon.JSONConversionOptions(jz.options))
	if err != nil {
		return nil, err
	}
	document, err := oson.EncodeWithOptions(value, drvCommon.JSONConversionOptions(jz.options))
	if err != nil {
		return nil, err
	}
	return []byte(document), nil
}

// Kind reports whether the root of a fetched JSON value is JSONObjectKind,
// JSONArrayKind, or JSONScalarKind. JSON null is a scalar.
func (jz JSON) Kind() (JSONKind, error) {
	if jz.node == nil {
		common.Odl.Debug("JSON.Kind: failed", "reason", "uninitialized JSON value")
		return 0, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "Kind")
	}

	return jz.node.Kind(), nil
}

// AsJSONObject returns a lazy object view of a fetched JSON value. It returns
// an error if jz is uninitialized or its root is not an object.
func (jz JSON) AsJSONObject() (JSONObject, error) {
	if obj, ok := jz.node.(drvCommon.JSONObjectNode); ok {
		return JSONObject{node: obj, options: jz.options}, nil
	}
	common.Odl.Debug("JSON.AsJSONObject: failed", "reason", "object node has unexpected implementation", "nodeType", fmt.Sprintf("%T", jz.node))
	return JSONObject{}, common.NewOracleError(oracleErrors.JSONAccessError, nil, "object")
}

// AsJSONArray returns a lazy array view of a fetched JSON value. It returns an
// error if jz is uninitialized or its root is not an array.
func (jz JSON) AsJSONArray() (JSONArray, error) {
	if arr, ok := jz.node.(drvCommon.JSONArrayNode); ok {
		return JSONArray{node: arr, options: jz.options}, nil
	}
	common.Odl.Debug("JSON.AsJSONArray: failed", "reason", "array node has unexpected implementation", "nodeType", fmt.Sprintf("%T", jz.node))
	return JSONArray{}, common.NewOracleError(oracleErrors.JSONAccessError, nil, "array")
}

// AsJSONScalar returns a lazy scalar view of a fetched JSON value. Objects and
// arrays are not scalars; JSON null is. The method returns an error if jz is
// uninitialized or its root is not a scalar.
func (jz JSON) AsJSONScalar() (JSONScalar, error) {
	if scalar, ok := jz.node.(drvCommon.JSONScalarNode); ok {
		return JSONScalar{node: scalar, options: jz.options}, nil
	}
	common.Odl.Debug("JSON.AsJSONScalar: failed", "reason", "scalar node has unexpected implementation", "nodeType", fmt.Sprintf("%T", jz.node))
	return JSONScalar{}, common.NewOracleError(oracleErrors.JSONAccessError, nil, "scalar")
}

// GetValue materializes the complete document as Go values using jz's stored
// [Options]. Objects become map[string]any, arrays become []any, and scalars
// become their corresponding Go values.
func (jz JSON) GetValue() (any, error) {
	if jz.node == nil {
		common.Odl.Debug("JSON.GetValue: failed", "reason", "uninitialized JSON value")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "GetValue")
	}
	return jz.node.GetValue(drvCommon.JSONConversionOptions(jz.options))
}

// MarshalJSON implements encoding/json.Marshaler. It returns the JSON text for
// the document or an error if jz is uninitialized or cannot be rendered.
func (jz JSON) MarshalJSON() ([]byte, error) {
	if jz.node == nil {
		common.Odl.Debug("JSON.MarshalJSON: failed", "reason", "uninitialized JSON value")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "MarshalJSON")
	}
	return jz.node.MarshalJSON()
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
		common.Odl.Debug("JSONObject.GetValue: failed", "reason", "uninitialized object view")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "GetValue")
	}
	return obj.node.Value(drvCommon.JSONConversionOptions(obj.options))
}

// MarshalJSON implements encoding/json.Marshaler. It returns the JSON text for
// the object or an error if obj is uninitialized or cannot be rendered.
func (obj JSONObject) MarshalJSON() ([]byte, error) {
	if obj.node == nil {
		common.Odl.Debug("JSONObject.MarshalJSON: failed", "reason", "uninitialized object view")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "MarshalJSON")
	}
	return obj.node.MarshalJSON()
}

// Keys returns the names of the object's members. Their order is unspecified.
// It returns nil for the zero uninitialized JSONObject.
func (obj JSONObject) Keys() []string {
	if obj.node == nil {
		return nil
	}
	return obj.node.Keys()
}

// Get returns a lazy child JSON for key. It returns JSONObjectKeyNotFoundError
// if key is absent, or an error if the child node cannot be constructed. Scalar
// payload errors may be reported later by GetValue or MarshalJSON.
func (obj JSONObject) Get(key string) (JSON, error) {
	if obj.node == nil {
		common.Odl.Debug("JSONObject.Get: failed", "reason", "uninitialized object view")
		return JSON{}, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "Get")
	}
	node, err := obj.node.Get(key)
	if err != nil {
		return JSON{}, err
	}
	return JSON{node: node, options: obj.options}, nil
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
		common.Odl.Debug("JSONArray.GetValue: failed", "reason", "uninitialized array view")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "GetValue")
	}
	return arr.node.Value(drvCommon.JSONConversionOptions(arr.options))
}

// MarshalJSON implements encoding/json.Marshaler. It returns the JSON text for
// the array or an error if arr is uninitialized or cannot be rendered.
func (arr JSONArray) MarshalJSON() ([]byte, error) {
	if arr.node == nil {
		common.Odl.Debug("JSONArray.MarshalJSON: failed", "reason", "uninitialized array view")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "MarshalJSON")
	}
	return arr.node.MarshalJSON()
}

// Get returns the lazy child JSON at index i. It returns an error if i is
// outside [0, Len()), arr is uninitialized, or the child node cannot be
// constructed. Scalar payload errors may be reported later by GetValue or MarshalJSON.
func (arr JSONArray) Get(i int) (JSON, error) {
	if arr.node == nil {
		common.Odl.Debug("JSONArray.Get: failed", "reason", "uninitialized array view", "index", i)
		return JSON{}, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "Get")
	}

	node, err := arr.node.Get(i)
	if err != nil {
		return JSON{}, err
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
		common.Odl.Debug("JSONScalar.GetValue: failed", "reason", "uninitialized scalar view")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "GetValue")
	}
	return scalar.node.Value(drvCommon.JSONConversionOptions(scalar.options))
}

// MarshalJSON implements encoding/json.Marshaler. It returns the JSON text for
// the scalar or an error if scalar is uninitialized or cannot be rendered.
func (scalar JSONScalar) MarshalJSON() ([]byte, error) {
	if scalar.node == nil {
		common.Odl.Debug("JSONScalar.MarshalJSON: failed", "reason", "uninitialized scalar view")
		return nil, common.NewOracleError(oracleErrors.JSONUninitializedValue, nil, "MarshalJSON")
	}
	return scalar.node.MarshalJSON()
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
