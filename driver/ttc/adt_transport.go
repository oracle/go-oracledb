package ttc

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"

	"github.com/oracle/go-oracledb/driver/adt"
	"github.com/oracle/go-oracledb/driver/common"
)

// loadADTObjectType retrieves TTC metadata and materializes the public ADT
// descriptor. It deliberately owns all DBMS_PICKLER and TDS handling so the
// adt package remains transport independent.
func loadADTObjectType(ctx context.Context, ex Execer, typeName string) (*adt.ObjectType, error) {
	if ex == nil || strings.TrimSpace(typeName) == "" {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(common.ADTMetadataError, nil)
	}

	var rc int64
	canonical := strings.TrimSpace(typeName)
	var toid, tds []byte
	var version int64
	var instantiable, superOwner, superName string
	var attributes, subtypes RefCursor
	_, err := ex.ExecContext(ctx, `BEGIN
  :1 := SYS.DBMS_PICKLER.GET_TYPE_SHAPE(:2, :3, :4, :5, :6, :7, :8, :9, :10);
END;`,
		sql.Out{Dest: &rc}, sql.Out{Dest: &canonical, In: true},
		sql.Out{Dest: &toid}, sql.Out{Dest: &version}, sql.Out{Dest: &tds},
		sql.Out{Dest: &instantiable}, sql.Out{Dest: &superOwner}, sql.Out{Dest: &superName},
		sql.Out{Dest: &attributes}, sql.Out{Dest: &subtypes})
	defer attributes.Close()
	defer subtypes.Close()
	if err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(common.ADTMetadataError, err)
	}
	if rc != 0 || len(toid) != 16 || len(tds) == 0 {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(common.ADTMetadataError, nil)
	}

	typ := &adt.ObjectType{
		Attributes:  make(map[string]adt.ObjectAttribute),
		TOID:        append([]byte(nil), toid...),
		TypeVersion: int(version),
		TDS:         append([]byte(nil), tds...),
	}
	typ.SetName(canonical)
	if err := parseADTTDSHeader(typ); err != nil {
		return nil, err
	}
	return typ, nil
}

func parseADTTDSHeader(typ *adt.ObjectType) error {
	if len(typ.TDS) < 18 || typ.TDS[4] != 38 || typ.TDS[11] != 41 {
		common.Odl.Error("ADT metadata error")
		return common.NewOracleError(common.ADTMetadataError, nil)
	}
	attributeCount := int(typ.TDS[8])<<8 | int(typ.TDS[9])
	if attributeCount != 1 || len(typ.TDS) < 28 || typ.TDS[18] != 28 {
		return nil
	}
	typ.Collection = true
	typ.UpperBound = int64(uint32(typ.TDS[23])<<24 | uint32(typ.TDS[24])<<16 | uint32(typ.TDS[25])<<8 | uint32(typ.TDS[26]))
	typ.VArray = typ.TDS[27] == 3
	elementOffset := int(uint32(typ.TDS[19])<<24 | uint32(typ.TDS[20])<<16 | uint32(typ.TDS[21])<<8 | uint32(typ.TDS[22]))
	if elementOffset < 0 || elementOffset >= len(typ.TDS) {
		common.Odl.Error("ADT metadata error")
		return common.NewOracleError(common.ADTMetadataError, nil)
	}
	elementType, elementSize, err := parseADTElementType(typ.TDS[elementOffset:])
	if err != nil {
		return err
	}
	typ.ElementType = elementType
	typ.ElementSize = elementSize
	return nil
}

func parseADTElementType(data []byte) (common.DtyType, int, error) {
	if len(data) == 0 {
		common.Odl.Error("ADT metadata error")
		return 0, 0, common.NewOracleError(common.ADTMetadataError, nil)
	}
	pos := 0
	for data[pos] == 44 || data[pos] == 43 {
		if data[pos] == 44 {
			pos += 2
		} else {
			pos++
		}
		if pos >= len(data) {
			common.Odl.Error("ADT metadata error")
			return 0, 0, common.NewOracleError(common.ADTMetadataError, nil)
		}
	}
	switch data[pos] {
	case 6, 5:
		return common.DtyNum, 22, nil
	case 7, 1:
		if len(data) < pos+3 {
			return 0, 0, common.NewOracleError(common.ADTMetadataError, nil)
		}
		return common.DtyVCS, int(data[pos+1])<<8 | int(data[pos+2]), nil
	case 19:
		if len(data) < pos+3 {
			return 0, 0, common.NewOracleError(common.ADTMetadataError, nil)
		}
		return common.DtyBin, int(data[pos+1])<<8 | int(data[pos+2]), nil
	case 2:
		return common.DtyDat, 7, nil
	case 37:
		return common.DtyIbFloat, 4, nil
	case 45:
		return common.DtyIbDouble, 8, nil
	case 21:
		return common.DtyStamp, 11, nil
	case 23:
		return common.DtyStz, 13, nil
	case 33:
		return common.DtySitz, 11, nil
	case 8:
		return common.DtyBol, 4, nil
	default:
		common.Odl.Error("ADT metadata error")
		return 0, 0, common.NewOracleError(common.ADTMetadataError, nil)
	}
}

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func newTTIOacNamedType(typ *adt.ObjectType, maxLength common.UB4) (*tTIoac, error) {
	if typ == nil || len(typ.TOID) != 16 {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(common.ADTMetadataError, nil)
	}
	if typ.TypeVersion < 0 || typ.TypeVersion > int(^uint16(0)) {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(common.ADTMetadataError, nil)
	}
	// Named types use a fixed 11-byte OAC maximum, including NULL named-type
	// binds. The collection image length is carried separately in
	// the RXD envelope and must not determine this OAC field.
	oac := newTTIoac(common.DtyNty, 11)
	toid := common.B1Array(append([]byte(nil), typ.TOID...))
	oac.toid = &toid
	oac.versionNumber = common.UB2(typ.TypeVersion)
	return oac, nil
}

func namedTypeForBind(v any) (*adt.ObjectType, bool) {
	switch value := v.(type) {
	case adt.ObjectCollection:
		if value.Object != nil && value.ObjectType != nil {
			return value.ObjectType, true
		}
	case *adt.ObjectCollection:
		if value != nil && value.Object != nil {
			return value.ObjectType, value.ObjectType != nil
		}
	case *adt.Object:
		if value != nil {
			return value.ObjectType, value.ObjectType != nil
		}
	}
	return nil, false
}

func collectionForBind(v any) (adt.ObjectCollection, bool) {
	switch value := v.(type) {
	case adt.ObjectCollection:
		return value, value.Object != nil
	case *adt.ObjectCollection:
		if value != nil && value.Object != nil {
			return *value, true
		}
	}
	return adt.ObjectCollection{}, false
}

// encodeCollectionImage emits the 8.1 collection image used by the Thin
// protocol. Element values reuse the driver's scalar codecs; the descriptor's
// parsed element type guards against binding an unsupported collection shape.
func encodeCollectionImage(collection adt.ObjectCollection, factory codecFactory) (common.B1Array, error) {
	if collection.ObjectType == nil || !collection.ObjectType.Collection {
		common.Odl.Error("ADT collection type error")
		return nil, common.NewOracleError(common.ADTValueError, nil)
	}
	if collection.Null {
		return nil, nil
	}
	if collection.ElementType == 0 {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(common.ADTMetadataError, nil)
	}
	// The 8.1 collection image header contains a prefix segment with the
	// inline/type-version flag and the collection's TDS version. For versions
	// up to 245 the version is a two-byte signed value; larger versions use the
	// five-byte pickle-length form.
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
		var encoder encoderFunc
		var err error
		encoder, err = factory.GetCollectionEncoder(collection.ElementType)
		if err != nil {
			encoder, err = factory.GetEncoder(normalizeBindValue(value))
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
	return common.B1Array(data), nil
}

func appendPickleLength(data []byte, length int) []byte {
	if length <= 245 {
		return append(data, byte(length))
	}
	return append(data, 254, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
}

func decodeCollectionImage(data common.B1Array, typ *adt.ObjectType, factory codecFactory) (driver.Value, error) {
	if len(data) < 10 || data[0]&0x88 != 0x88 {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(common.ADTEncodingError, nil)
	}
	// The 8.1 collection header is: image flags/version, image length, prefix
	// segment length, prefix flags and data (including type version), followed
	// by collection flags and the element count.
	_, pos, err := readPickleLength(data, 2)
	if err != nil {
		return nil, err
	}
	prefixLength, next, err := readPickleLength(data, pos)
	if err != nil {
		return nil, err
	}
	pos = next
	if prefixLength < 1 || pos+prefixLength > len(data) {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(common.ADTEncodingError, nil)
	}
	pos += prefixLength // prefix flag plus prefix data
	if pos >= len(data) {
		common.Odl.Error("ADT collection image error")
		return nil, common.NewOracleError(common.ADTEncodingError, nil)
	}
	pos++ // collection flags
	count, next, err := readPickleLength(data, pos)
	if err != nil {
		return nil, err
	}
	pos = next
	collection, err := typ.NewCollection()
	if err != nil {
		return nil, err
	}
	decoder, err := factory.GetDecoder(typ.ElementType)
	if err != nil {
		return nil, err
	}
	for i := 0; i < count; i++ {
		if pos >= len(data) {
			common.Odl.Error("ADT collection image error")
			return nil, common.NewOracleError(common.ADTEncodingError, nil)
		}
		if data[pos] == 255 {
			collection.Object.Values = append(collection.Object.Values, nil)
			pos++
			continue
		}
		length, next, err := readPickleLength(data, pos)
		if err != nil {
			return nil, err
		}
		pos = next
		if length < 0 || pos+length > len(data) {
			common.Odl.Error("ADT collection image error")
			return nil, common.NewOracleError(common.ADTEncodingError, nil)
		}
		value, err := decoder.decodeToType(ColumnContext{DataType: typ.ElementType}, data[pos:pos+length])
		if err != nil {
			return nil, err
		}
		collection.Object.Values = append(collection.Object.Values, value)
		pos += length
	}
	return collection, nil
}

func readPickleLength(data []byte, pos int) (int, int, error) {
	if pos >= len(data) {
		common.Odl.Error("ADT collection image error")
		return 0, pos, common.NewOracleError(common.ADTEncodingError, nil)
	}
	if data[pos] <= 245 {
		return int(data[pos]), pos + 1, nil
	}
	if data[pos] != 254 || pos+5 > len(data) {
		common.Odl.Error("ADT collection image error")
		return 0, pos, common.NewOracleError(common.ADTEncodingError, nil)
	}
	return int(uint32(data[pos+1])<<24 | uint32(data[pos+2])<<16 | uint32(data[pos+3])<<8 | uint32(data[pos+4])), pos + 5, nil
}
