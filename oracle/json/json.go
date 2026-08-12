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

// Package json provides the public Oracle JSON API.
package json

import (
	"database/sql/driver"
	"fmt"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/ttc/oson"
)

// JSONOption controls JSON materialization behavior.
type JSONOption = common.JSONOption

const (
	// JSONOptDefault returns JSON numbers as float64 when materializing values.
	JSONOptDefault JSONOption = common.JSONOptDefault
	// JSONOptNumberAsString returns JSON numbers as json.Number.
	JSONOptNumberAsString JSONOption = common.JSONOptNumberAsString
)

// JSONNumber is the string representation of a JSON number.
type Number = common.JSONNumber

// JSONKind identifies the high-level JSON value category.
type JSONKind = common.Kind

const (
	// JSONObjectKind represents a JSON object.
	JSONObjectKind JSONKind = common.KindObject
	// JSONArrayKind represents a JSON array.
	JSONArrayKind JSONKind = common.KindArray
	// JSONScalarKind represents a JSON scalar.
	JSONScalarKind JSONKind = common.KindScalar
)

// JSONString is the bind-facing wrapper for JSON text.
type JSONString struct {
	// Data is the JSON text input.
	Data string
}

// Value implements driver.Valuer.
func (jz JSONString) Value() (driver.Value, error) {
	return jz.Data, nil
}

// JSONValue is the bind-facing JSON value wrapper.
type JSONValue struct {
	// Data is the Go value to encode as OSON for a JSON bind. It accepts:
	//   - nil, bool, string, int, int8, int16, int32, int64, uint, uint8,
	//     uint16, uint32, uint64, float32, float64, []byte, and time.Time
	//   - Number and encoding/json.Number containing valid JSON number text
	//   - map[string]any and []any, whose values recursively use these types
	Data any
}

// Value implements driver.Valuer
func (jz JSONValue) Value() (driver.Value, error) {
	doc, err := oson.Encode(jz.Data)
	if err != nil {
		return nil, err
	}

	return []byte(doc), nil
}

// JSON is the primary public container for Oracle JSON values.
type JSON struct {
	// node provides access to the underlying JSON representation.
	node common.JSONNode
}

// Scan implements sql.Scanner.
func (jz *JSON) Scan(src any) error {
	if jz == nil {
		return common.NewOracleError(common.JSONNilReceiver, nil, "Scan")
	}
	switch value := src.(type) {
	case []byte:
		if len(value) >= 4 && oson.IsOson(value) {
			// copy the buffer
			doc := append([]byte(nil), value...)
			// creating a OSON node
			node, err := oson.Parse(doc)
			if err != nil {
				return err
			}
			*jz = JSON{node: node}
			return nil
		}
		return common.NewOracleError(common.OsonParsingError, nil, "oracle/json text scan")
	default:
		return common.NewOracleError(common.JSONScanTypeUnsupportedError, nil, fmt.Sprintf("%T", src))
	}
}

// Value implements driver.Valuer.
func (jz JSON) Value() (driver.Value, error) {
	if jz.node == nil {
		return nil, common.NewOracleError(common.JSONNilReceiver, nil, "Value")
	}

	node, err := jz.node.GetValue(JSONOptNumberAsString)
	if err != nil {
		return nil, err
	}

	in := JSONValue{Data: node}
	return in.Value()
}

// Kind reports the high-level JSON value category.
func (jz JSON) Kind() (JSONKind, error) {
	if jz.node == nil {
		return 0, common.NewOracleError(common.JSONNilReceiver, nil, "Kind")
	}

	return jz.node.Kind(), nil
}

// GetJSONObject returns jz as a JSON object.
func (jz JSON) GetJSONObject(opts JSONOption) (JSONObject, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONObject{}, err
	}
	if kind != JSONObjectKind {
		return JSONObject{}, common.NewOracleError(common.JSONAccessError, nil, "object")
	}

	if obj, ok := jz.node.(common.JSONObjectNode); ok {
		return JSONObject{node: obj, opts: opts}, nil
	}
	return JSONObject{}, common.NewOracleError(common.JSONAccessError, nil, "object")
}

// GetJSONArray returns jz as a JSON array.
func (jz JSON) GetJSONArray(opts JSONOption) (JSONArray, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONArray{}, err
	}
	if kind != JSONArrayKind {
		return JSONArray{}, common.NewOracleError(common.JSONAccessError, nil, "array")
	}

	if arr, ok := jz.node.(common.JSONArrayNode); ok {
		return JSONArray{node: arr, opts: opts}, nil
	}
	return JSONArray{}, common.NewOracleError(common.JSONAccessError, nil, "array")
}

// GetJSONScalar returns jz as a JSON scalar.
func (jz JSON) GetJSONScalar(opts JSONOption) (JSONScalar, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONScalar{}, err
	}
	if kind != JSONScalarKind {
		return JSONScalar{}, common.NewOracleError(common.JSONAccessError, nil, "scalar")
	}
	if scalar, ok := jz.node.(common.JSONScalarNode); ok {
		return JSONScalar{node: scalar, opts: opts}, nil
	}
	return JSONScalar{}, common.NewOracleError(common.JSONAccessError, nil, "scalar")
}

// GetValue materializes the JSON value with the supplied options.
func (jz JSON) GetValue(opts JSONOption) (any, error) {
	if jz.node == nil {
		return nil, common.NewOracleError(common.JSONNilReceiver, nil, "GetValue")
	}
	return jz.node.GetValue(opts)
}

// String returns the JSON text form.
func (jz JSON) String() string {
	if jz.node == nil {
		return ""
	}

	text, err := jz.StringWithOption(JSONOptNumberAsString)
	if err != nil {
		return ""
	}
	return text
}

// StringWithOption returns the JSON text form using the supplied options.
func (jz JSON) StringWithOption(opts JSONOption) (string, error) {
	if jz.node == nil {
		return "", common.NewOracleError(common.JSONNilReceiver, nil, "StringWithOption")
	}
	return jz.node.StringWithOption(opts)
}

// JSONObject is a public JSON object wrapper.
type JSONObject struct {
	// node provides access to the underlying JSON object representation.
	node common.JSONObjectNode
	// opts controls how values returned from this object are materialized.
	opts JSONOption
}

// Len returns the number of members in the JSON object, or -1 if the object is uninitialized.
func (obj JSONObject) Len() int {
	if obj.node == nil {
		return -1
	}
	return obj.node.Len()
}

// GetValue materializes the object as a Go map.
func (obj JSONObject) GetValue() (map[string]any, error) {
	if obj.node == nil {
		return nil, common.NewOracleError(common.JSONNilReceiver, nil, "GetValue")
	}
	return obj.node.Value(obj.opts)
}

// Keys returns the object members names.
func (obj JSONObject) Keys() []string {
	if obj.node == nil {
		return nil
	}
	return obj.node.Keys()
}

// Has reports whether key exists in the object.
func (obj JSONObject) Has(key string) bool {
	if obj.node != nil {
		_, ok := obj.node.Get(key)
		return ok
	}

	return false
}

// Get returns the child JSON value for key.
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

// String returns the object as JSON text.
func (obj JSONObject) String() string {
	if obj.node != nil {
		text, err := obj.node.StringWithOption(obj.opts)
		if err != nil {
			return ""
		}
		return text
	}
	return ""
}

// JSONArray is a public JSON array wrapper.
type JSONArray struct {
	// node provides access to the underlying JSON array representation.
	node common.JSONArrayNode
	// opts controls how values returned from this array are materialized.
	opts JSONOption
}

// Len returns the number of members in the JSON array, or -1 if the array is uninitialized.
func (arr JSONArray) Len() int {
	if arr.node == nil {
		return -1
	}
	return arr.node.Len()
}

// GetValue materializes the array as a Go slice.
func (arr JSONArray) GetValue() ([]any, error) {
	if arr.node == nil {
		return nil, common.NewOracleError(common.JSONNilReceiver, nil, "GetValue")
	}
	return arr.node.Value(arr.opts)
}

// Get returns the child JSON value at i.
func (arr JSONArray) Get(i int) (JSON, error) {
	if arr.node == nil {
		return JSON{}, common.NewOracleError(common.JSONNilReceiver, nil, "Get")
	}

	node, ok := arr.node.Get(i)
	if !ok {
		return JSON{}, common.NewOracleError(common.JSONArrayIndexOutOfRangeError, nil, i)
	}
	return JSON{node: node}, nil
}

// String returns the array as JSON text.
func (arr JSONArray) String() string {
	if arr.node != nil {
		text, err := arr.node.StringWithOption(arr.opts)
		if err != nil {
			return ""
		}
		return text
	}
	return ""
}

// JSONScalar is a public JSON scalar wrapper.
type JSONScalar struct {
	// node provides access to the underlying JSON scalar representation.
	node common.JSONScalarNode
	// opts controls how this scalar is materialized.
	opts JSONOption
}

// GetValue materializes the scalar value.
func (scalar JSONScalar) GetValue() (any, error) {
	if scalar.node == nil {
		return nil, common.NewOracleError(common.JSONNilReceiver, nil, "GetValue")
	}
	return scalar.node.Value(scalar.opts)
}
