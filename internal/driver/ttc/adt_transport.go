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

package ttc

import (
	"database/sql/driver"
	"reflect"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	collectionIndexesFlag  = 0x10
	collectionNoLengthFlag = 0x08
	// Oracle's named-type OAC uses the ADT charset sentinel, rather than the
	// connection character set used for ordinary scalar binds.
	namedTypeCharsetID driverCommon.UB2 = 2
)

// newTTIOacNamedType creates the OAC descriptor used to bind an ADT value.
// Named-type OACs carry the descriptor TOID and type version. Their wire
// maximum is fixed by the protocol, so maxLength is intentionally not used.
//
// Parameters:
//   - typ: Public descriptor for the object or collection type being bound.
//   - maxLength: Requested bind length; ignored for named-type OACs.
//
// Returns:
//   - *tTIoac: OAC containing the named type TOID, version, and charset.
//   - error: Error if typ has invalid TOID or version metadata.
func newTTIOacNamedType(typ *adtDescriptor, maxLength driverCommon.UB4) (*tTIoac, error) {
	if typ == nil || len(typ.TOID) != 16 {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if typ.TypeVersion < 0 || typ.TypeVersion > int(^uint16(0)) {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	// Named types use a fixed 11-byte OAC maximum, including NULL named-type
	// binds. The collection image length is carried separately in
	// the RXD envelope and must not determine this OAC field.
	oac := newTTIoac(common.DtyNty, 11)
	toid := driverCommon.B1Array(append([]byte(nil), typ.TOID...))
	oac.toid = &toid
	oac.versionNumber = driverCommon.UB2(typ.TypeVersion)
	oac.characterSetID = namedTypeCharsetID
	return oac, nil
}

// namedTypeForBind returns the ADT descriptor retained by an object or
// collection bind value.
//
// Parameters:
//   - v: Candidate bind value in pointer or value form.
//
// Returns:
//   - *datatype.ObjectType: Descriptor associated with v.
//   - bool: True when v is an ADT value with a descriptor.
func namedTypeForBind(v any) (*datatype.ObjectType, bool) {
	switch value := v.(type) {
	case datatype.ObjectCollection:
		if value.Object != nil && value.ObjectType != nil {
			return value.ObjectType, true
		}
	case *datatype.ObjectCollection:
		if value != nil && value.Object != nil {
			return value.ObjectType, value.ObjectType != nil
		}
	case *datatype.Object:
		if value != nil {
			return value.ObjectType, value.ObjectType != nil
		}
	case datatype.Object:
		return value.ObjectType, value.ObjectType != nil
	}
	return nil, false
}

// adtBindCodecs supplies the resolved bind descriptor at the registry boundary.
type adtBindCodecs struct {
	codecFactory
	descriptor *adtDescriptor
}

// encodeObjectCollectionBind encodes a collection bind value as an ADT image.
// It returns a nil image for values that are not ObjectCollection values.
//
// Parameters:
//   - value: Driver bind value to inspect and encode.
//   - factory: Registry context carrying the resolved collection descriptor.
//
// Returns:
//   - driverCommon.B1Array: Encoded collection image, or nil for another value type.
//   - error: Error returned while encoding the collection or its elements.
func encodeObjectCollectionBind(value driver.Value, factory codecFactory) (driverCommon.B1Array, error) {
	collection, ok := collectionForBind(value)
	if !ok {
		return nil, nil
	}
	ctx, ok := factory.(adtBindCodecs)
	if !ok || ctx.descriptor == nil {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return ctx.descriptor.encodeCollectionImage(collection)
}

// encodeObjectBind encodes an object bind value as an ADT image. It returns a
// nil image for values that are not Object values.
//
// Parameters:
//   - value: Driver bind value to inspect and encode.
//   - factory: Registry context carrying the resolved object descriptor.
//
// Returns:
//   - driverCommon.B1Array: Encoded object image, or nil for another value type.
//   - error: Error returned while encoding the object or its attributes.
func encodeObjectBind(value driver.Value, factory codecFactory) (driverCommon.B1Array, error) {
	object, ok := objectForBind(value)
	if !ok {
		return nil, nil
	}
	ctx, ok := factory.(adtBindCodecs)
	if !ok || ctx.descriptor == nil {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return ctx.descriptor.encodeObjectImage(object)
}

// newNamedTypeBindOAC creates an ADT bind OAC from normalized bind metadata.
//
// Parameters:
//   - metadata: Normalized metadata containing the ADT descriptor.
//   - maxLength: Requested bind length passed to the named OAC constructor.
//
// Returns:
//   - driverCommon.Marshallable: Named-type OAC for the bind.
//   - error: Error if ADT descriptor metadata is unavailable or invalid.
func newNamedTypeBindOAC(metadata bindValueMetadata, maxLength driverCommon.UB4) (driverCommon.Marshallable, error) {
	if metadata.descriptor == nil {
		common.Odl.Error("Named ADT bind is missing type metadata")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return newTTIOacNamedType(metadata.descriptor, maxLength)
}

// resolveNamedTypeMetadata supplies a known ADT descriptor to untyped named
// OUT binds in the same call.
//
// Parameters:
//   - metadata: Bind metadata to update in place.
func resolveNamedTypeMetadata(metadata []bindValueMetadata) error {
	for i := range metadata {
		out := &metadata[i]
		if !out.isOut || out.descriptor != nil || !isNamedTypeGoType(out.goType) {
			continue
		}
		collection := out.goType == reflect.TypeFor[datatype.ObjectCollection]()
		var candidate *adtDescriptor
		for _, input := range metadata {
			t := input.descriptor
			if t == nil || t.Collection != collection {
				continue
			}
			if candidate != nil && candidate != t {
				candidate = nil
				break
			}
			candidate = t
		}
		if candidate == nil {
			common.Odl.Error("Cannot infer a unique ADT OUT bind descriptor", "position", i+1)
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		out.descriptor = candidate
	}
	return nil
}

// isNamedTypeGoType reports whether goType represents a public ADT value.
//
// Parameters:
//   - goType: Go type derived from a normalized bind value.
//
// Returns:
//   - bool: True for Object and ObjectCollection values.
func isNamedTypeGoType(goType reflect.Type) bool {
	return goType == reflect.TypeFor[datatype.Object]() || goType == reflect.TypeFor[datatype.ObjectCollection]()
}

// collectionForBind extracts a non-zero collection value from v.
//
// Parameters:
//   - v: Candidate collection value in pointer or value form.
//
// Returns:
//   - datatype.ObjectCollection: Extracted collection value.
//   - bool: True when v contains a collection descriptor.
func collectionForBind(v any) (datatype.ObjectCollection, bool) {
	switch value := v.(type) {
	case datatype.ObjectCollection:
		return value, value.Object != nil
	case *datatype.ObjectCollection:
		if value != nil && value.Object != nil {
			return *value, true
		}
	}
	return datatype.ObjectCollection{}, false
}

// objectForBind extracts a non-collection object value from v.
//
// Parameters:
//   - v: Candidate object value in pointer or value form.
//
// Returns:
//   - *datatype.Object: Extracted non-collection object.
//   - bool: True when v is an object with a descriptor.
func objectForBind(v any) (*datatype.Object, bool) {
	switch object := v.(type) {
	case *datatype.Object:
		return object, object != nil && object.ObjectType != nil && !object.ObjectType.Collection
	case datatype.Object:
		return &object, object.ObjectType != nil && !object.ObjectType.Collection
	default:
		return nil, false
	}
}

// encodeCollectionImage emits the inline 8.1 collection image used by the TTC
// protocol. Scalar elements reuse the driver's codecs; object elements carry a
// complete object image selected by CollectionOf.
//
// Parameters:
//   - collection: Public collection value to encode.
//
// Returns:
//   - driverCommon.B1Array: Complete inline collection image, or nil for NULL.
//   - error: Error if collection metadata, values, or element encoding is invalid.
func (typ *adtDescriptor) encodeCollectionImage(collection datatype.ObjectCollection) (driverCommon.B1Array, error) {
	if collection.Object == nil || typ == nil || !typ.Collection {
		common.Odl.Error("ADT collection type error")
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if collection.IsNull() {
		return nil, nil
	}
	values, err := collection.Get()
	if err != nil {
		return nil, err
	}
	if typ.VArray && typ.UpperBound > 0 && int64(len(values)) > typ.UpperBound {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	var indexes []int32
	if typ.Associative {
		indexes, err = collection.Indexes()
		if err != nil || len(indexes) != len(values) {
			common.Odl.Error("ADT collection image error")
			return nil, common.NewOracleError(oracleErrors.ADTEncodingError, err)
		}
	}
	if typ.adtCodecContext == nil || typ.codecs == nil || (typ.ElementType == 0 && typ.CollectionOf == nil) {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	// VARRAYs and nested tables use the same dense, inline 8.1 collection image.
	// The prefix carries the inline/type-version flag and TDS version. Versions
	// up to 245 use a two-byte signed value; larger versions use the five-byte
	// pickle-length form.
	version := typ.TypeVersion
	prefix := []byte{0x11}
	if version <= 245 {
		prefix = append(prefix, byte(version>>8), byte(version))
	} else {
		prefix = appendPickleLength(prefix, version)
	}
	data := []byte{0x88, 1, 254, 0, 0, 0, 0}
	data = appendPickleLength(data, len(prefix))
	data = append(data, prefix...)
	collectionFlags := byte(0)
	if typ.Associative {
		collectionFlags |= collectionIndexesFlag
	}
	data = append(data, collectionFlags)
	data = appendPickleLength(data, len(values))
	for position, value := range values {
		if typ.Associative {
			data = appendCollectionIndex(data, indexes[position])
		}
		if value == nil {
			data = append(data, 255)
			continue
		}
		if typ.CollectionOf != nil {
			object, ok := objectValue(value)
			if !ok || object.IsNull() {
				data = append(data, 255)
				continue
			}
			element := typ.CollectionOf
			if object.ObjectType != nil {
				if named := typ.byName[strings.ToUpper(object.FullName())]; named != nil {
					element = named
				}
			}
			image, err := element.encodeObjectImage(object)
			if err != nil {
				return nil, err
			}
			// Object collection elements are UPT records. Prefix each complete
			// object image with its pickle length before placing it in the
			// collection data segment.
			data = appendPickleLength(data, len(image))
			data = append(data, image...)
			continue
		}
		metadata := extractBindValueMetadata(value)
		encoder, err := typ.codecs.getEncoderForDtyType(typ.ElementType, metadata)
		if err != nil {
			return nil, err
		}
		encoded, err := encoder.encode(value, typ.codecs)
		if err != nil {
			return nil, err
		}
		data = appendPickleLength(data, len(encoded))
		data = append(data, encoded...)
	}
	size := len(data)
	data[3], data[4], data[5], data[6] = byte(size>>24), byte(size>>16), byte(size>>8), byte(size)
	return driverCommon.B1Array(data), nil
}

// appendCollectionIndex appends an associative-array PLS_INTEGER index in
// network byte order.
//
// Parameters:
//   - data: Image bytes to extend.
//   - index: Associative-array index to encode.
//
// Returns:
//   - []byte: data with the four-byte signed index appended.
func appendCollectionIndex(data []byte, index int32) []byte {
	value := uint32(index)
	return append(data, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

// encodeObjectImage emits an inline 8.1 image for an object. Attribute order
// comes from the public descriptor, and scalar attributes reuse their declared
// Oracle datatype codec.
//
// Parameters:
//   - object: Public object value to encode.
//
// Returns:
//   - driverCommon.B1Array: Complete inline object image, or nil for NULL.
//   - error: Error if object metadata, state, or attribute encoding is invalid.
func (typ *adtDescriptor) encodeObjectImage(object *datatype.Object) (driverCommon.B1Array, error) {
	if object == nil || object.Closed || typ == nil || !typ.IsObject() {
		common.Odl.Error("ADT object type error")
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if object.IsNull() {
		return nil, nil
	}
	if typ.adtCodecContext == nil || typ.codecs == nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	data := []byte{0x84, 1, 254, 0, 0, 0, 0}
	if typ.TypeVersion > 1 || typ.SuperTypeName != "" {
		if len(typ.TOID) != 16 {
			common.Odl.Error("ADT metadata error")
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		// A polymorphic object image identifies its runtime subtype with a TOID
		// and type-version prefix. The top-level named OAC still identifies the
		// declared bind type.
		data[0] = 0x80
		prefix := []byte{0x05}
		prefix = append(prefix, typ.TOID...)
		if typ.TypeVersion > 1 {
			prefix[0] |= 0x10
			if typ.TypeVersion <= 245 {
				prefix = append(prefix, byte(typ.TypeVersion>>8), byte(typ.TypeVersion))
			} else {
				prefix = appendPickleLength(prefix, typ.TypeVersion)
			}
		}
		data = appendPickleLength(data, len(prefix))
		data = append(data, prefix...)
	}
	var err error
	data, err = typ.encodeObjectRecord(data, object, 0)
	if err != nil {
		return nil, err
	}
	size := len(data)
	data[3], data[4], data[5], data[6] = byte(size>>24), byte(size>>16), byte(size>>8), byte(size)
	return driverCommon.B1Array(data), nil
}

// encodeObjectRecord appends an object's attributes in declaration order to an
// enclosing object image. depth tracks recursive embedded-object records.
//
// Parameters:
//   - data: Object image bytes to extend.
//   - object: Object whose attributes are encoded.
//   - depth: Embedded-object nesting depth.
//
// Returns:
//   - []byte: data with the encoded object record appended.
//   - error: Error if object metadata, attribute values, or codecs are invalid.
func (typ *adtDescriptor) encodeObjectRecord(data []byte, object *datatype.Object, depth byte) ([]byte, error) {
	for index, name := range typ.AttributeNames() {
		attributeType := typ.Attributes[name].adtDescriptor
		if attributeType == nil {
			common.Odl.Error("ADT metadata error")
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if object.Attributes == nil {
			common.Odl.Error("ADT object image error")
			return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
		}
		value := object.Attributes[name]
		if isEmbeddedObject(attributeType) {
			child, childOK := objectValue(value)
			if !childOK || child.IsNull() {
				if index == 0 && depth > 0 {
					return append(data, 252, depth), nil
				}
				// A NULL embedded object occupies one attribute slot; it does not
				// terminate the containing object record. Later attributes still
				// need to be encoded in declaration order.
				data = append(data, 253)
				continue
			}
			var err error
			data, err = attributeType.encodeObjectRecord(data, child, depth+1)
			if err != nil {
				return nil, err
			}
			continue
		}
		if value == nil {
			data = append(data, 255)
			continue
		}
		if attributeType.Collection {
			collection, collectionOK := collectionForBind(value)
			if !collectionOK || collection.IsNull() {
				data = append(data, 255)
				continue
			}
			image, err := attributeType.encodeCollectionImage(collection)
			if err != nil {
				return nil, err
			}
			// A named collection attribute is represented by a UPT in its
			// containing object's TDS. The UPT record wraps the complete
			// collection image in a pickle length.
			data = appendPickleLength(data, len(image))
			data = append(data, image...)
			continue
		}
		if attributeType.ElementType == 0 {
			common.Odl.Error("ADT metadata error")
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		metadata := extractBindValueMetadata(value)
		encoder, err := typ.codecs.getEncoderForDtyType(attributeType.ElementType, metadata)
		if err != nil {
			return nil, err
		}
		encoded, err := encoder.encode(value, typ.codecs)
		if err != nil {
			return nil, err
		}
		data = appendPickleLength(data, len(encoded))
		data = append(data, encoded...)
	}
	return data, nil
}

// isEmbeddedObject reports whether typ is an object attribute represented by
// an inline object record rather than a collection or scalar value.
//
// Parameters:
//   - typ: Attribute descriptor to classify.
//
// Returns:
//   - bool: True when typ represents an embedded object record.
func isEmbeddedObject(typ *adtDescriptor) bool {
	return typ != nil && !typ.Collection && typ.ElementType == 0
}

// objectValue extracts an object from either its pointer or value form.
//
// Parameters:
//   - value: Candidate object value.
//
// Returns:
//   - *datatype.Object: Extracted object pointer.
//   - bool: True when value is an object.
func objectValue(value any) (*datatype.Object, bool) {
	switch object := value.(type) {
	case *datatype.Object:
		return object, object != nil
	case datatype.Object:
		return &object, true
	default:
		return nil, false
	}
}

// appendPickleLength appends length using the 8.1 image pickle-length format.
//
// Parameters:
//   - data: Image bytes to extend.
//   - length: Number of bytes represented by the encoded length.
//
// Returns:
//   - []byte: data with a compact or extended pickle length appended.
func appendPickleLength(data []byte, length int) []byte {
	if length <= 245 {
		return append(data, byte(length))
	}
	return append(data, 254, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
}

// decodeCollectionImage decodes a dense, inline 8.1 collection image using
// this descriptor and its connection’s scalar codecs.
//
// Parameters:
//   - data: Raw inline collection image.
//
// Returns:
//   - driver.Value: Decoded ObjectCollection value.
//   - error: Error if the image framing, metadata, or elements are invalid.
func (typ *adtDescriptor) decodeCollectionImage(data driverCommon.B1Array) (driver.Value, error) {
	const (
		pickle81ImageFlag      = 0x80
		pickleCollectionFlag   = 0x08
		pickleDegenerateFlag   = 0x10
		pickleInlineCollection = 0x01
	)
	if typ == nil || typ.adtCodecContext == nil || typ.codecs == nil || !typ.Collection || len(data) < 3 || data[0]&pickle81ImageFlag == 0 {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	// Nested-table columns can be returned as locator-backed, degenerate images.
	// Locator materialization is intentionally separate from the dense inline
	// collection path, so reject it instead of interpreting locator bytes as an
	// element count.
	if data[0]&pickleDegenerateFlag != 0 {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	if len(data) < 10 || data[0]&pickleCollectionFlag == 0 {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	// The 8.1 collection header is: image flags/version, image length, prefix
	// segment length, prefix flags and data (including type version), followed
	// by collection flags and the element count.
	imageLength, pos, err := readPickleLength(data, 2)
	if err != nil {
		return nil, err
	}
	if imageLength != len(data) {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	prefixLength, next, err := readPickleLength(data, pos)
	if err != nil {
		return nil, err
	}
	pos = next
	if prefixLength < 1 || pos+prefixLength > len(data) {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	prefixFlags := data[pos]
	if prefixFlags&pickleInlineCollection == 0 {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	pos += prefixLength // prefix flag plus prefix data
	if pos >= len(data) {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	collectionFlags := data[pos]
	indexed := collectionFlags&collectionIndexesFlag != 0
	if collectionFlags&collectionNoLengthFlag != 0 || indexed != typ.Associative {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	pos++
	count, next, err := readPickleLength(data, pos)
	if err != nil {
		return nil, err
	}
	pos = next
	collection, err := typ.NewCollection()
	if err != nil {
		return nil, err
	}
	values := make([]any, 0, count)
	indexes := make([]int32, 0, count)
	var decoder *typeDecoder
	if typ.CollectionOf == nil {
		decoder, err = typ.codecs.getDecoder(typ.ElementType)
		if err != nil {
			return nil, err
		}
	}
	for i := 0; i < count; i++ {
		if indexed {
			index, next, indexErr := readCollectionIndex(data, pos)
			if indexErr != nil {
				return nil, indexErr
			}
			indexes = append(indexes, index)
			pos = next
		}
		if pos >= len(data) {
			common.Odl.Error("ADT collection image error")
			return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
		}
		if data[pos] == 255 {
			values = append(values, nil)
			pos++
			continue
		}
		if typ.CollectionOf != nil {
			imageLength, next, imageErr := readPickleLength(data, pos)
			if imageErr != nil {
				return nil, imageErr
			}
			if imageLength < 0 || next+imageLength > len(data) {
				common.Odl.Error("ADT collection image error")
				return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
			}
			end := next + imageLength
			value, decodeErr := typ.CollectionOf.decodeObjectImage(data[next:end])
			if decodeErr != nil {
				return nil, decodeErr
			}
			values = append(values, value)
			pos = end
			continue
		}
		length, next, err := readPickleLength(data, pos)
		if err != nil {
			return nil, err
		}
		pos = next
		if length < 0 || pos+length > len(data) {
			common.Odl.Error("ADT collection image error")
			return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
		}
		value, err := decoder.decodeToType(columnContext{DataType: typ.ElementType}, data[pos:pos+length])
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		pos += length
	}
	if indexed {
		indexedValues := make(map[int32]any, len(values))
		for position, index := range indexes {
			if _, exists := indexedValues[index]; exists {
				common.Odl.Error("ADT collection image error")
				return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
			}
			indexedValues[index] = values[position]
		}
		if err := collection.SetIndexed(indexedValues); err != nil {
			return nil, err
		}
	} else if err := collection.Set(values); err != nil {
		return nil, err
	}
	return collection, nil
}

// decodeNamedTypeColumn decodes a registered named-type column using the ADT
// descriptor retained in the column context. A descriptor is optional for an
// unknown named type, in which case the raw image is preserved.
//
// Parameters:
//   - column: Named-type column metadata, including the optional ADT descriptor.
//   - value: Raw named-type image supplied by the named-type wire unwrapping path.
//
// Returns:
//   - driver.Value: Decoded Object or ObjectCollection, or raw data when unknown.
//   - error: Error if typed image decoding fails.
func decodeNamedTypeColumn(column columnContext, value any) (driver.Value, error) {
	typ := column.objectType
	if typ == nil {
		return value, nil
	}
	data, ok := value.(driverCommon.B1Array)
	if !ok {
		return value, nil
	}
	if typ.Collection {
		return typ.decodeCollectionImage(data)
	}
	return typ.decodeObjectImage(data)
}

// decodeNullNamedTypeColumn constructs a typed NULL value for a named-type
// column so an OUT destination retains its ADT descriptor.
//
// Parameters:
//   - column: Named-type column metadata, including the optional ADT descriptor.
//
// Returns:
//   - driver.Value: NULL Object or ObjectCollection when a descriptor is known.
//   - error: Error if the descriptor cannot construct its public value.
func decodeNullNamedTypeColumn(column columnContext) (driver.Value, error) {
	typ := column.objectType
	if typ == nil {
		return nil, nil
	}
	if typ.Collection {
		value, err := typ.NewCollection()
		if err != nil {
			return nil, err
		}
		value.SetNull()
		return value, nil
	}
	value, err := typ.NewObject()
	if err != nil {
		return nil, err
	}
	value.SetNull()
	return *value, nil
}

// readCollectionIndex reads one signed associative-array index from data.
//
// Parameters:
//   - data: Collection image containing the index.
//   - pos: Offset of the four-byte index.
//
// Returns:
//   - int32: Decoded signed index.
//   - int: Offset immediately after the index.
//   - error: Error if fewer than four index bytes remain.
func readCollectionIndex(data driverCommon.B1Array, pos int) (int32, int, error) {
	if pos < 0 || pos+4 > len(data) {
		common.Odl.Error("ADT collection image error")
		return 0, pos, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	value := uint32(data[pos])<<24 | uint32(data[pos+1])<<16 | uint32(data[pos+2])<<8 | uint32(data[pos+3])
	return int32(value), pos + 4, nil
}

// decodeObjectImage decodes an inline 8.1 object image. Scalar attributes use
// their registered codec, collection attributes carry complete collection
// images, and embedded object attributes use recursive ADT records.
//
// Parameters:
//   - data: Raw inline object image.
//
// Returns:
//   - driver.Value: Decoded Object value.
//   - error: Error if the image framing, subtype, metadata, or attributes are invalid.
func (typ *adtDescriptor) decodeObjectImage(data driverCommon.B1Array) (driver.Value, error) {
	const (
		pickle81ImageFlag  = 0x80
		pickleNoPrefixFlag = 0x04
	)
	if typ == nil || !typ.IsObject() || typ.adtCodecContext == nil || typ.codecs == nil || len(data) < 3 || data[0]&pickle81ImageFlag == 0 {
		common.Odl.Error("ADT object image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	imageLength, pos, err := readPickleLength(data, 2)
	if err != nil || imageLength != len(data) {
		common.Odl.Error("ADT object image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, err)
	}
	decodeType := typ
	if data[0]&pickleNoPrefixFlag == 0 {
		prefixLength, next, prefixErr := readPickleLength(data, pos)
		if prefixErr != nil || prefixLength < 1 || next+prefixLength > len(data) {
			common.Odl.Error("ADT object image error")
			return nil, common.NewOracleError(oracleErrors.ADTEncodingError, prefixErr)
		}
		prefix := data[next : next+prefixLength]
		if prefix[0]&0x0C == 0x04 {
			if len(prefix) < 17 {
				common.Odl.Error("ADT object image error")
				return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
			}
			if subtype := typ.SubTypeForTOID(prefix[1:17]); subtype != nil {
				decodeType = subtype
			}
		}
		pos = next + prefixLength
	}
	object, pos, err := decodeType.decodeObjectRecord(data, pos, 0)
	if err != nil {
		return nil, err
	}
	if pos != len(data) {
		common.Odl.Error("ADT object image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	return *object, nil
}

// decodeObjectRecord decodes attributes at pos and returns the populated object
// with the position immediately following its inline record.
//
// Parameters:
//   - data: Object image containing the record.
//   - pos: Offset of the first attribute record.
//   - depth: Embedded-object nesting depth.
//
// Returns:
//   - *datatype.Object: Decoded object value.
//   - int: Offset immediately after the decoded record.
//   - error: Error if record metadata, framing, or attributes are invalid.
func (typ *adtDescriptor) decodeObjectRecord(data driverCommon.B1Array, pos int, depth byte) (*datatype.Object, int, error) {
	object, err := typ.NewObject()
	if err != nil {
		return nil, pos, err
	}
	for index, name := range typ.AttributeNames() {
		attribute := typ.Attributes[name]
		attributeType := attribute.adtDescriptor
		if attributeType == nil || pos >= len(data) {
			common.Odl.Error("ADT metadata error")
			return nil, pos, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if isEmbeddedObject(attributeType) {
			if data[pos] == 253 {
				if err := setObjectAttribute(object, name, nil); err != nil {
					return nil, pos, err
				}
				pos++
				continue
			}
			if data[pos] == 252 {
				if index != 0 || depth == 0 || pos+1 >= len(data) {
					common.Odl.Error("ADT object image error")
					return nil, pos, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
				}
				if err := setObjectAttribute(object, name, nil); err != nil {
					return nil, pos, err
				}
				pos += 2
				continue
			}
			child, next, childErr := attributeType.decodeObjectRecord(data, pos, depth+1)
			if childErr != nil {
				return nil, pos, childErr
			}
			if err := setObjectAttribute(object, name, *child); err != nil {
				return nil, pos, err
			}
			pos = next
			continue
		}
		if data[pos] == 255 {
			if err := setObjectAttribute(object, name, nil); err != nil {
				return nil, pos, err
			}
			pos++
			continue
		}
		if attributeType.Collection {
			imageLength, next, imageErr := readPickleLength(data, pos)
			if imageErr != nil {
				return nil, pos, imageErr
			}
			if imageLength < 0 || next+imageLength > len(data) {
				common.Odl.Error("ADT object image error")
				return nil, pos, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
			}
			end := next + imageLength
			value, decodeErr := attributeType.decodeCollectionImage(data[next:end])
			if decodeErr != nil {
				return nil, pos, decodeErr
			}
			if err := setObjectAttribute(object, name, value); err != nil {
				return nil, pos, err
			}
			pos = end
			continue
		}
		if attributeType.ElementType == 0 {
			common.Odl.Error("ADT metadata error")
			return nil, pos, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		length, next, lengthErr := readPickleLength(data, pos)
		if lengthErr != nil || length < 0 || next+length > len(data) {
			common.Odl.Error("ADT object image error")
			return nil, pos, common.NewOracleError(oracleErrors.ADTEncodingError, lengthErr)
		}
		decoder, decoderErr := typ.codecs.getDecoder(attributeType.ElementType)
		if decoderErr != nil {
			return nil, pos, decoderErr
		}
		value, decodeErr := decoder.decodeToType(columnContext{DataType: attributeType.ElementType}, data[next:next+length])
		if decodeErr != nil {
			return nil, pos, decodeErr
		}
		if err := setObjectAttribute(object, name, value); err != nil {
			return nil, pos, err
		}
		pos = next + length
	}
	return object, pos, nil
}

// setObjectAttribute assigns value to an object attribute, preserving nil as a
// NULL attribute.
//
// Parameters:
//   - object: Object whose attribute map is updated.
//   - name: Database attribute name.
//   - value: Decoded attribute value, or nil for a NULL attribute.
//
// Returns:
//   - error: Error if object state or field mutability is invalid.
func setObjectAttribute(object *datatype.Object, name string, value any) error {
	if object == nil || object.Closed || object.ObjectType == nil {
		common.Odl.Error("ADT object image error")
		return common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	if _, ok := object.ObjectType.Attributes[name]; !ok {
		common.Odl.Error("ADT object image error")
		return common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	if object.Attributes == nil {
		object.Attributes = make(map[string]any)
	}
	object.Attributes[name] = value
	return nil
}

// namedImageEnd returns the end position of the complete named-type image at
// pos after validating its 8.1 image header and pickle length.
//
// Parameters:
//   - data: Buffer containing a named-type image.
//   - pos: Offset of the image header.
//
// Returns:
//   - int: Offset immediately after the complete image.
//   - error: Error if the header or length is invalid or truncated.
func namedImageEnd(data driverCommon.B1Array, pos int) (int, error) {
	if pos+3 > len(data) || data[pos]&0x80 == 0 {
		common.Odl.Error("ADT object image error")
		return 0, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	length, _, err := readPickleLength(data, pos+2)
	if err != nil || length < 3 || pos+length > len(data) {
		common.Odl.Error("ADT object image error")
		return 0, common.NewOracleError(oracleErrors.ADTEncodingError, err)
	}
	return pos + length, nil
}

// readPickleLength reads an 8.1 image pickle length and returns its value and
// the position immediately following its encoding.
//
// Parameters:
//   - data: Buffer containing a pickle length.
//   - pos: Offset of the length encoding.
//
// Returns:
//   - int: Decoded byte length.
//   - int: Offset immediately after the length encoding.
//   - error: Error if the length encoding is invalid or truncated.
func readPickleLength(data []byte, pos int) (int, int, error) {
	if pos >= len(data) {
		common.Odl.Error("ADT collection image error")
		return 0, pos, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	if data[pos] <= 245 {
		return int(data[pos]), pos + 1, nil
	}
	if data[pos] != 254 || pos+5 > len(data) {
		common.Odl.Error("ADT collection image error")
		return 0, pos, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	return int(uint32(data[pos+1])<<24 | uint32(data[pos+2])<<16 | uint32(data[pos+3])<<8 | uint32(data[pos+4])), pos + 5, nil
}
