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
	"encoding/json"
	"fmt"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/oson"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// JSONOption controls JSON materialization behavior.
type JSONOption = drvCommon.JSONOption

const (
	// JSONOptDefault returns JSON numbers as float64 when materializing values.
	JSONOptDefault JSONOption = drvCommon.JSONOptDefault
	// JSONOptNumberAsString returns JSON numbers as json.Number.
	JSONOptNumberAsString JSONOption = drvCommon.JSONOptNumberAsString
)

// JSONNumber is the string representation of a JSON number.
type Number = drvCommon.JSONNumber

// JSONKind identifies the high-level JSON value category.
type JSONKind = drvCommon.Kind

const (
	// JSONObjectKind represents a JSON object.
	JSONObjectKind JSONKind = drvCommon.KindObject
	// JSONArrayKind represents a JSON array.
	JSONArrayKind JSONKind = drvCommon.KindArray
	// JSONScalarKind represents a JSON scalar.
	JSONScalarKind JSONKind = drvCommon.KindScalar
)

// JSONString is the bind-facing for JSON text.
type JSONString string

// Value implements driver.Valuer.
func (jz JSONString) Value() (driver.Value, error) {
	if !json.Valid([]byte(jz)) {
		cause := fmt.Errorf("JSONString contains invalid JSON text")
		return nil, common.NewOracleError(oracleErrors.OsonEncodingError, cause)
	}
	return string(jz), nil
}

// JSON is the primary public container for Oracle JSON values.
type JSON struct {
	// node provides access to the underlying JSON representation.
	node drvCommon.JSONNode
	// Data is the Go value to encode as OSON for a JSON bind. It accepts:
	//   - nil, bool, string, int, int8, int16, int32, int64, uint, uint8,
	//     uint16, uint32, uint64, float32, float64, []byte, and time.Time
	//   - Number and encoding/json.Number containing valid JSON number text
	//   - map[string]any and []any, whose values recursively use these types
	Data any
}

// Scan implements sql.Scanner.
func (jz *JSON) Scan(src any) error {
	if jz == nil {
		cause := fmt.Errorf("cannot scan Oracle JSON into a nil *JSON receiver")
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
				return err
			}
			*jz = JSON{node: node}
			return nil
		}
		cause := fmt.Errorf("cannot scan %d-byte []byte as Oracle JSON: expected an OSON document beginning with magic bytes FF 4A 5A", len(value))
		return common.NewOracleError(oracleErrors.OsonHeaderError, cause)
	default:
		cause := fmt.Errorf("source type %T cannot be scanned as Oracle JSON", src)
		return common.NewOracleError(oracleErrors.JSONScanTypeUnsupportedError, cause, fmt.Sprintf("%T", src))
	}
}

// Value implements driver.Valuer.
func (jz JSON) Value() (driver.Value, error) {
	data := jz.Data

	if jz.node != nil {
		var err error
		data, err = jz.node.GetValue(drvCommon.JSONOptNumberAsString)
		if err != nil {
			return nil, err
		}
	}

	doc, err := oson.Encode(data)
	if err != nil {
		return nil, err
	}

	return []byte(doc), nil
}

// Kind reports the high-level JSON value category.
func (jz JSON) Kind() (JSONKind, error) {
	if jz.node == nil {
		cause := fmt.Errorf("JSON has no parsed OSON node to inspect")
		return 0, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "Kind")
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
		cause := fmt.Errorf("JSON node kind %d cannot be accessed as an object", kind)
		return JSONObject{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "object")
	}

	if obj, ok := jz.node.(drvCommon.JSONObjectNode); ok {
		return JSONObject{node: obj, opts: opts}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports object kind but does not implement JSONObjectNode", jz.node)
	return JSONObject{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "object")
}

// GetJSONArray returns jz as a JSON array.
func (jz JSON) GetJSONArray(opts JSONOption) (JSONArray, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONArray{}, err
	}
	if kind != JSONArrayKind {
		cause := fmt.Errorf("JSON node kind %d cannot be accessed as an array", kind)
		return JSONArray{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "array")
	}

	if arr, ok := jz.node.(drvCommon.JSONArrayNode); ok {
		return JSONArray{node: arr, opts: opts}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports array kind but does not implement JSONArrayNode", jz.node)
	return JSONArray{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "array")
}

// GetJSONScalar returns jz as a JSON scalar.
func (jz JSON) GetJSONScalar(opts JSONOption) (JSONScalar, error) {
	kind, err := jz.Kind()
	if err != nil {
		return JSONScalar{}, err
	}
	if kind != JSONScalarKind {
		cause := fmt.Errorf("JSON node kind %d cannot be accessed as a scalar", kind)
		return JSONScalar{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "scalar")
	}
	if scalar, ok := jz.node.(drvCommon.JSONScalarNode); ok {
		return JSONScalar{node: scalar, opts: opts}, nil
	}
	cause := fmt.Errorf("JSON node of type %T reports scalar kind but does not implement JSONScalarNode", jz.node)
	return JSONScalar{}, common.NewOracleError(oracleErrors.JSONAccessError, cause, "scalar")
}

// GetValue materializes the JSON value with the supplied options.
func (jz JSON) GetValue(opts JSONOption) (any, error) {
	if jz.node == nil {
		cause := fmt.Errorf("JSON has no parsed OSON node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return jz.node.GetValue(opts)
}

// String returns the JSON text form.
func (jz JSON) String() (string, error) {
	if jz.node != nil {
		return jz.node.String()
	}

	text, err := json.Marshal(jz.Data)
	if err != nil {
		return "", common.NewOracleError(oracleErrors.JSONRenderingError, err)
	}
	return string(text), nil
}

// JSONObject is a public JSON object wrapper.
type JSONObject struct {
	// node provides access to the underlying JSON object representation.
	node drvCommon.JSONObjectNode
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
		cause := fmt.Errorf("JSONObject has no underlying object node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
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
func (obj JSONObject) String() (string, error) {
	if obj.node == nil {
		cause := fmt.Errorf("JSONObject has no underlying object node to render")
		return "", common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "String")
	}
	return obj.node.String()
}

// JSONArray is a public JSON array wrapper.
type JSONArray struct {
	// node provides access to the underlying JSON array representation.
	node drvCommon.JSONArrayNode
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
		cause := fmt.Errorf("JSONArray has no underlying array node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return arr.node.Value(arr.opts)
}

// Get returns the child JSON value at i.
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

// String returns the array as JSON text.
func (arr JSONArray) String() (string, error) {
	if arr.node == nil {
		cause := fmt.Errorf("JSONArray has no underlying array node to render")
		return "", common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "String")
	}
	return arr.node.String()
}

// JSONScalar is a public JSON scalar wrapper.
type JSONScalar struct {
	// node provides access to the underlying JSON scalar representation.
	node drvCommon.JSONScalarNode
	// opts controls how this scalar is materialized.
	opts JSONOption
}

// GetValue materializes the scalar value.
func (scalar JSONScalar) GetValue() (any, error) {
	if scalar.node == nil {
		cause := fmt.Errorf("JSONScalar has no underlying scalar node to materialize")
		return nil, common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "GetValue")
	}
	return scalar.node.Value(scalar.opts)
}

// String returns the scalar as JSON text.
func (scalar JSONScalar) String() (string, error) {
	if scalar.node == nil {
		cause := fmt.Errorf("JSONScalar has no underlying scalar node to render")
		return "", common.NewOracleError(oracleErrors.JSONNilReceiver, cause, "String")
	}
	return scalar.node.String()
}
