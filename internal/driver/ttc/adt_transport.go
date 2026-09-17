package ttc

import (
	"database/sql/driver"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func newTTIOacNamedType(typ *datatype.ObjectType, maxLength driverCommon.UB4) (*tTIoac, error) {
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
	return oac, nil
}

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

// encodeCollectionImage emits the 8.1 collection image used by the Thin
// protocol. Scalar elements reuse the driver's codecs; object elements carry
// a complete object image selected by CollectionOf.
func encodeCollectionImage(collection datatype.ObjectCollection, factory codecFactory) (driverCommon.B1Array, error) {
	if collection.ObjectType == nil || !collection.ObjectType.Collection {
		common.Odl.Error("ADT collection type error")
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if collection.IsNull() {
		return nil, nil
	}
	if collection.ElementType == 0 && collection.CollectionOf == nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	// VARRAYs and nested tables use the same dense, inline 8.1 collection image.
	// The prefix carries the inline/type-version flag and TDS version. Versions
	// up to 245 use a two-byte signed value; larger versions use the five-byte
	// pickle-length form.
	version := collection.ObjectType.TypeVersion
	prefix := []byte{0x11}
	if version <= 245 {
		prefix = append(prefix, byte(version>>8), byte(version))
	} else {
		prefix = appendPickleLength(prefix, version)
	}
	data := []byte{0x88, 1, 254, 0, 0, 0, 0}
	data = appendPickleLength(data, len(prefix))
	data = append(data, prefix...)
	data = append(data, 0) // collection flags
	data = appendPickleLength(data, len(collection.Object.Values))
	for _, value := range collection.Object.Values {
		if value == nil {
			data = append(data, 255)
			continue
		}
		if collection.CollectionOf != nil {
			object, ok := objectValue(value)
			if !ok || object.IsNull() {
				data = append(data, 255)
				continue
			}
			image, err := encodeObjectImage(object, factory)
			if err != nil {
				return nil, err
			}
			data = append(data, image...)
			continue
		}
		var encoder encoderFunc
		var err error
		encoder, err = factory.getCollectionEncoder(collection.ElementType)
		if err != nil {
			encoder, err = factory.getEncoder(normalizeBindValue(value))
			if err != nil {
				return nil, err
			}
		}
		encoded, err := encoder(value)
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

// encodeObjectImage emits an inline 8.1 image for an object with scalar
// attributes. Attribute order is the declaration order retained by the public
// descriptor; attribute values reuse the scalar codec registered for each
// declared Oracle datatype.
func encodeObjectImage(object *datatype.Object, factory codecFactory) (driverCommon.B1Array, error) {
	if object == nil || object.ObjectType == nil || !object.ObjectType.IsObject() {
		common.Odl.Error("ADT object type error")
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if object.IsNull() {
		return nil, nil
	}
	if factory == nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	data := []byte{0x84, 1, 254, 0, 0, 0, 0}
	if object.ObjectType.TypeVersion > 1 || object.ObjectType.SuperTypeName != "" {
		if len(object.ObjectType.TOID) != 16 {
			common.Odl.Error("ADT metadata error")
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		// A polymorphic object image identifies its runtime subtype with a TOID
		// and type-version prefix. The top-level named OAC still identifies the
		// declared bind type.
		data[0] = 0x80
		prefix := []byte{0x05}
		prefix = append(prefix, object.ObjectType.TOID...)
		if object.ObjectType.TypeVersion > 1 {
			prefix[0] |= 0x10
			if object.ObjectType.TypeVersion <= 245 {
				prefix = append(prefix, byte(object.ObjectType.TypeVersion>>8), byte(object.ObjectType.TypeVersion))
			} else {
				prefix = appendPickleLength(prefix, object.ObjectType.TypeVersion)
			}
		}
		data = appendPickleLength(data, len(prefix))
		data = append(data, prefix...)
	}
	var err error
	data, err = encodeObjectRecord(data, object, factory, 0)
	if err != nil {
		return nil, err
	}
	size := len(data)
	data[3], data[4], data[5], data[6] = byte(size>>24), byte(size>>16), byte(size>>8), byte(size)
	return driverCommon.B1Array(data), nil
}

func encodeObjectRecord(data []byte, object *datatype.Object, factory codecFactory, depth byte) ([]byte, error) {
	for index, name := range object.ObjectType.AttributeNames() {
		attributeType := object.ObjectType.Attributes[name].ObjectType
		if attributeType == nil {
			common.Odl.Error("ADT metadata error")
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		value, present := object.Attributes[name]
		if isEmbeddedObject(attributeType) {
			child, childOK := objectValue(value)
			if !present || !childOK || child.IsNull() {
				if index == 0 && depth > 0 {
					return append(data, 252, depth), nil
				}
				return append(data, 253), nil
			}
			var err error
			data, err = encodeObjectRecord(data, child, factory, depth+1)
			if err != nil {
				return nil, err
			}
			continue
		}
		if !present || value == nil {
			data = append(data, 255)
			continue
		}
		if attributeType.Collection {
			collection, collectionOK := collectionForBind(value)
			if !collectionOK || collection.IsNull() {
				data = append(data, 255)
				continue
			}
			image, err := encodeCollectionImage(collection, factory)
			if err != nil {
				return nil, err
			}
			data = append(data, image...)
			continue
		}
		if attributeType.ElementType == 0 {
			common.Odl.Error("ADT metadata error")
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		encoder, err := factory.getCollectionEncoder(attributeType.ElementType)
		if err != nil {
			encoder, err = factory.getEncoder(normalizeBindValue(value))
			if err != nil {
				return nil, err
			}
		}
		encoded, err := encoder(value)
		if err != nil {
			return nil, err
		}
		data = appendPickleLength(data, len(encoded))
		data = append(data, encoded...)
	}
	return data, nil
}

func isEmbeddedObject(typ *datatype.ObjectType) bool {
	return typ != nil && !typ.Collection && typ.ElementType == 0
}

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

func appendPickleLength(data []byte, length int) []byte {
	if length <= 245 {
		return append(data, byte(length))
	}
	return append(data, 254, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
}

func decodeCollectionImage(data driverCommon.B1Array, typ *datatype.ObjectType, factory codecFactory) (driver.Value, error) {
	const (
		pickle81ImageFlag      = 0x80
		pickleCollectionFlag   = 0x08
		pickleDegenerateFlag   = 0x10
		pickleInlineCollection = 0x01
		collectionIndexesFlag  = 0x10
		collectionNoLengthFlag = 0x08
	)
	if typ == nil || !typ.Collection || len(data) < 3 || data[0]&pickle81ImageFlag == 0 {
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
	if collectionFlags&(collectionIndexesFlag|collectionNoLengthFlag) != 0 {
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
	var decoder *typeDecoder
	if typ.CollectionOf == nil {
		decoder, err = factory.getDecoder(typ.ElementType)
		if err != nil {
			return nil, err
		}
	}
	for i := 0; i < count; i++ {
		if pos >= len(data) {
			common.Odl.Error("ADT collection image error")
			return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
		}
		if data[pos] == 255 {
			collection.Object.Values = append(collection.Object.Values, nil)
			pos++
			continue
		}
		if typ.CollectionOf != nil {
			end, imageErr := namedImageEnd(data, pos)
			if imageErr != nil {
				return nil, imageErr
			}
			value, decodeErr := decodeObjectImage(data[pos:end], typ.CollectionOf, factory)
			if decodeErr != nil {
				return nil, decodeErr
			}
			collection.Object.Values = append(collection.Object.Values, value)
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
		collection.Object.Values = append(collection.Object.Values, value)
		pos += length
	}
	return collection, nil
}

// decodeObjectImage decodes an inline 8.1 object image. Scalar attributes use
// their registered codec, collection attributes carry a complete collection
// image, and embedded object attributes use recursive ADT records.
func decodeObjectImage(data driverCommon.B1Array, typ *datatype.ObjectType, factory codecFactory) (driver.Value, error) {
	const (
		pickle81ImageFlag  = 0x80
		pickleNoPrefixFlag = 0x04
	)
	if typ == nil || !typ.IsObject() || factory == nil || len(data) < 3 || data[0]&pickle81ImageFlag == 0 {
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
	object, pos, err := decodeObjectRecord(data, pos, decodeType, factory, 0)
	if err != nil {
		return nil, err
	}
	if pos != len(data) {
		common.Odl.Error("ADT object image error")
		return nil, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	return *object, nil
}

func decodeObjectRecord(data driverCommon.B1Array, pos int, typ *datatype.ObjectType, factory codecFactory, depth byte) (*datatype.Object, int, error) {
	object, err := typ.NewObject()
	if err != nil {
		return nil, pos, err
	}
	for index, name := range typ.AttributeNames() {
		attribute := typ.Attributes[name]
		attributeType := attribute.ObjectType
		if attributeType == nil || pos >= len(data) {
			common.Odl.Error("ADT metadata error")
			return nil, pos, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if isEmbeddedObject(attributeType) {
			if data[pos] == 253 {
				object.Attributes[name] = nil
				pos++
				continue
			}
			if data[pos] == 252 {
				if index != 0 || depth == 0 || pos+1 >= len(data) {
					common.Odl.Error("ADT object image error")
					return nil, pos, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
				}
				object.Attributes[name] = nil
				pos += 2
				continue
			}
			child, next, childErr := decodeObjectRecord(data, pos, attributeType, factory, depth+1)
			if childErr != nil {
				return nil, pos, childErr
			}
			object.Attributes[name] = *child
			pos = next
			continue
		}
		if data[pos] == 255 {
			object.Attributes[name] = nil
			pos++
			continue
		}
		if attributeType.Collection {
			end, imageErr := namedImageEnd(data, pos)
			if imageErr != nil {
				return nil, pos, imageErr
			}
			value, decodeErr := decodeCollectionImage(data[pos:end], attributeType, factory)
			if decodeErr != nil {
				return nil, pos, decodeErr
			}
			object.Attributes[name] = value
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
		decoder, decoderErr := factory.getDecoder(attributeType.ElementType)
		if decoderErr != nil {
			return nil, pos, decoderErr
		}
		value, decodeErr := decoder.decodeToType(columnContext{DataType: attributeType.ElementType}, data[next:next+length])
		if decodeErr != nil {
			return nil, pos, decodeErr
		}
		object.Attributes[name] = value
		pos = next + length
	}
	return object, pos, nil
}

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
