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

// Package datatype provides public values and metadata for Oracle abstract data
// types (ADTs), including object types and collection types such as VARRAYs.
//
// ObjectType describes a database type. Object and ObjectCollection hold values
// associated with an ObjectType and can be passed to the driver as named binds.
package datatype

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// ObjectType describes an Oracle object or collection type.
//
// The driver populates its metadata fields from the database. Applications use
// the descriptor to construct Object and ObjectCollection values for binds.
type ObjectType struct {
	// CollectionOf describes the element object type when the collection contains
	// object values.
	CollectionOf *ObjectType
	// Attributes contains object attributes keyed by their database names.
	Attributes map[string]ObjectAttribute
	// Schema, Name, and PackageName identify the database type.
	Schema, Name, PackageName string
	// DBSize, ClientSizeInBytes, and CharSize describe the size constraints of
	// the database type.
	DBSize, ClientSizeInBytes, CharSize int
	// Precision, Scale, and FsPrecision describe numeric or temporal precision.
	Precision   int16
	Scale       int8
	FsPrecision uint8

	// TOID is the database type object identifier.
	TOID []byte
	// TypeVersion identifies the current version of the database type.
	TypeVersion int
	// TDS contains the type descriptor stream returned by the database.
	TDS []byte
	// Collection reports whether this descriptor represents a collection type.
	Collection bool
	// Instantiable reports whether Oracle permits direct instances of this type.
	Instantiable bool
	// SuperTypeName is the fully qualified immediate supertype name, if any.
	SuperTypeName string
	// SubTypes contains descriptors for the immediate subtypes reported by Oracle.
	SubTypes []*ObjectType
	// VArray reports whether this collection is a VARRAY rather than another
	// collection kind.
	VArray bool
	// UpperBound is the maximum number of elements allowed in a VARRAY.
	UpperBound int64
	// ElementType and ElementSize describe scalar collection elements.
	ElementType common.DtyType
	ElementSize int
	// Closed reports whether Close has been called for this descriptor.
	Closed bool

	// tdsType is the parsed root of TDS. It is deliberately private: callers
	// work with ObjectType and ObjectAttribute, while the driver uses this tree
	// to preserve type order and shape while decoding ADT metadata.
	tdsType        *tdsType
	subTypesByTOID map[string]*ObjectType
}

// tdsType is one type record in an Oracle type descriptor stream. Named type
// references are initially represented by their TDS record and resolved when
// their deferred TDS patch is processed.
type tdsType struct {
	dtyType     common.DtyType
	size        int
	precision   int16
	scale       int8
	fsPrecision uint8

	collection bool
	varray     bool
	upperBound int64
	element    *tdsType
	attributes []*tdsType
	deferred   *tdsPatch
	source     []byte
}

type tdsPatch struct {
	offset int
	code   byte
}

// ObjectAttribute describes one attribute of an Oracle object type.
type ObjectAttribute struct {
	// ObjectType describes the attribute's Oracle type.
	*ObjectType
	// Name is the attribute name as declared by the database type.
	Name string
	// Sequence is the one-based attribute position in the database type.
	Sequence uint32
}

// Object is a value of an Oracle object type.
type Object struct {
	// ObjectType identifies the database type represented by this value.
	*ObjectType
	// Attributes holds attribute values keyed by database attribute name.
	Attributes map[string]any
	// Values holds collection elements when this object represents a collection.
	Values []any
	// Null distinguishes a NULL object from an object whose attributes are all NULL.
	Null bool
	// Closed reports whether Close has been called for this value.
	Closed bool
}

// ObjectCollection is an Oracle collection value, including a VARRAY.
type ObjectCollection struct {
	// Object contains the collection type and its element values.
	*Object
	// Null distinguishes a NULL collection from an empty collection.
	Null bool
}

// Execer executes a statement using database/sql-style positional arguments.
// Both *sql.DB and *sql.Conn implement this interface.
type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// GetObjectType retrieves the metadata for typeName and returns its public ADT
// descriptor. ex should use the same physical database connection that will
// later bind values created from the returned descriptor.
func GetObjectType(ctx context.Context, ex Execer, typeName string) (*ObjectType, error) {
	if ex == nil || strings.TrimSpace(typeName) == "" {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return loadObjectType(ctx, ex, strings.TrimSpace(typeName), make(map[string]*ObjectType))
}

func loadObjectType(ctx context.Context, ex Execer, typeName string, cache map[string]*ObjectType) (*ObjectType, error) {
	canonicalKey := strings.ToUpper(strings.TrimSpace(typeName))
	if typ := cache[canonicalKey]; typ != nil {
		return typ, nil
	}

	var rc int64
	canonical := typeName
	var toid, tds []byte
	var version int64
	var instantiable, superOwner, superName string
	var attributes, subtypes driver.Rows
	_, err := ex.ExecContext(ctx, `BEGIN
  :1 := SYS.DBMS_PICKLER.GET_TYPE_SHAPE(:2, :3, :4, :5, :6, :7, :8, :9, :10);
END;`,
		sql.Out{Dest: &rc}, sql.Out{Dest: &canonical, In: true},
		sql.Out{Dest: &toid}, sql.Out{Dest: &version}, sql.Out{Dest: &tds},
		sql.Out{Dest: &instantiable}, sql.Out{Dest: &superOwner}, sql.Out{Dest: &superName},
		sql.Out{Dest: &attributes}, sql.Out{Dest: &subtypes})
	defer closeRows(attributes)
	defer closeRows(subtypes)
	if err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	if rc != 0 || len(toid) != 16 || len(tds) == 0 {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}

	typ := &ObjectType{
		Attributes:  make(map[string]ObjectAttribute),
		TOID:        append([]byte(nil), toid...),
		TypeVersion: int(version),
		TDS:         append([]byte(nil), tds...),
	}
	typ.SetName(canonical)
	typ.Instantiable = strings.EqualFold(instantiable, "YES")
	typ.SuperTypeName = qualifiedTypeName(superOwner, "", superName)
	cache[canonicalKey] = typ
	if err := parseTDS(typ); err != nil {
		return nil, err
	}
	if err := populateObjectAttributes(ctx, ex, typ, attributes, cache); err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	if err := populateObjectSubtypes(ctx, ex, typ, subtypes, cache); err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	return typ, nil
}

func closeRows(rows driver.Rows) {
	if rows != nil {
		_ = rows.Close()
	}
}

func populateObjectSubtypes(ctx context.Context, ex Execer, typ *ObjectType, rows driver.Rows, cache map[string]*ObjectType) error {
	if typ == nil {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if rows == nil {
		return nil
	}
	if len(rows.Columns()) != 4 {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	for {
		values := make([]driver.Value, 4)
		err := rows.Next(values)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		version, ok := adtMetadataInt(values[0])
		if !ok || version != 1 {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		owner, ok := adtMetadataString(values[1])
		if !ok || owner == "" {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		name, ok := adtMetadataString(values[2])
		if !ok || name == "" {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		oid, ok := adtMetadataBytes(values[3])
		if !ok || len(oid) != 16 {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		subtype, err := loadObjectType(ctx, ex, qualifiedTypeName(owner, "", name), cache)
		if err != nil {
			return err
		}
		if typ.subTypesByTOID == nil {
			typ.subTypesByTOID = make(map[string]*ObjectType)
		}
		key := string(oid)
		if typ.subTypesByTOID[key] != nil {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		typ.subTypesByTOID[key] = subtype
		typ.SubTypes = append(typ.SubTypes, subtype)
	}
}

// SubTypeForTOID returns the immediate subtype associated with toid, or nil
// when toid is not a subtype of t.
func (t *ObjectType) SubTypeForTOID(toid []byte) *ObjectType {
	if t == nil {
		return nil
	}
	if subtype := t.subTypesByTOID[string(toid)]; subtype != nil {
		return subtype
	}
	for _, subtype := range t.SubTypes {
		if subtype != nil && string(subtype.TOID) == string(toid) {
			return subtype
		}
	}
	return nil
}

// populateObjectAttributes maps the DBMS_PICKLER attribute cursor onto the
// declaration-ordered TDS tree. The cursor identifies names and named-type
// identities; TDS supplies the scalar and collection shape for each position.
func populateObjectAttributes(ctx context.Context, ex Execer, typ *ObjectType, rows driver.Rows, cache map[string]*ObjectType) error {
	if typ == nil || typ.tdsType == nil {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if rows == nil {
		return nil
	}
	if len(rows.Columns()) != 10 {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	for {
		values := make([]driver.Value, 10)
		err := rows.Next(values)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		version, ok := adtMetadataInt(values[0])
		if !ok || version != 1 {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		name, ok := adtMetadataString(values[1])
		if !ok || name == "" {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		sequence, ok := adtMetadataInt(values[2])
		metadataType := typ
		nodes := typ.tdsType.attributes
		if typ.Collection && typ.CollectionOf != nil && len(nodes) == 1 && nodes[0].collection && nodes[0].element != nil && len(nodes[0].element.attributes) != 0 {
			metadataType = typ.CollectionOf
			nodes = nodes[0].element.attributes
		}
		if !ok || sequence < 1 || sequence > len(nodes) {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if _, exists := metadataType.Attributes[name]; exists {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		owner, _ := adtMetadataString(values[3])
		typeName, _ := adtMetadataString(values[4])
		packageName, _ := adtMetadataString(values[5])
		oid, ok := adtMetadataBytes(values[6])
		if !ok {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		node := nodes[sequence-1]
		attributeType := newObjectTypeFromTDSType(node, owner, typeName, packageName, oid)
		// Named embedded objects and collection attributes need their own
		// descriptor: the parent TDS provides the wire shape, while the named
		// descriptor supplies attribute names and the collection type version.
		if (len(node.attributes) != 0 || node.collection) && typeName != "" {
			fullName := qualifiedTypeName(owner, packageName, typeName)
			loaded, loadErr := loadObjectType(ctx, ex, fullName, cache)
			if loadErr != nil {
				return loadErr
			}
			attributeType = loaded
		}
		metadataType.Attributes[name] = ObjectAttribute{
			ObjectType: attributeType,
			Name:       name,
			Sequence:   uint32(sequence),
		}
	}
}

func qualifiedTypeName(owner, packageName, name string) string {
	if packageName != "" {
		return owner + "." + packageName + "." + name
	}
	if owner != "" {
		return owner + "." + name
	}
	return name
}

func newObjectTypeFromTDSType(node *tdsType, schema, name, packageName string, oid []byte) *ObjectType {
	typ := &ObjectType{
		Attributes:  make(map[string]ObjectAttribute),
		Schema:      schema,
		Name:        name,
		PackageName: packageName,
		TOID:        append([]byte(nil), oid...),
		tdsType:     node,
	}
	if node == nil {
		return typ
	}
	if node.collection {
		typ.Collection = true
		typ.VArray = node.varray
		typ.UpperBound = node.upperBound
		if node.element != nil {
			typ.ElementType = node.element.dtyType
			typ.ElementSize = node.element.size
			typ.Precision = node.element.precision
			typ.Scale = node.element.scale
			typ.FsPrecision = node.element.fsPrecision
			if len(node.element.attributes) != 0 {
				typ.CollectionOf = newObjectTypeFromTDSType(node.element, "", "", "", nil)
			}
		}
		return typ
	}
	typ.ElementType = node.dtyType
	typ.ElementSize = node.size
	typ.Precision = node.precision
	typ.Scale = node.scale
	typ.FsPrecision = node.fsPrecision
	return typ
}

func adtMetadataString(value driver.Value) (string, bool) {
	switch value := value.(type) {
	case nil:
		return "", true
	case string:
		return value, true
	case []byte:
		return string(value), true
	default:
		return "", false
	}
}

func adtMetadataBytes(value driver.Value) ([]byte, bool) {
	switch value := value.(type) {
	case nil:
		return nil, true
	case []byte:
		return value, true
	case string:
		return []byte(value), true
	default:
		return nil, false
	}
}

func adtMetadataInt(value driver.Value) (int, bool) {
	switch value := value.(type) {
	case int64:
		return int(value), int64(int(value)) == value
	case int:
		return value, true
	case float64:
		return int(value), value == float64(int(value))
	case string:
		parsed, err := strconv.Atoi(value)
		return parsed, err == nil
	case []byte:
		parsed, err := strconv.Atoi(string(value))
		return parsed, err == nil
	default:
		return 0, false
	}
}

// TDS opcodes are defined by the TTC type descriptor stream. The parser only
// accepts records whose byte layout is known, so malformed metadata cannot
// advance beyond the returned TDS buffer.
const (
	tdsVersionOpcode       = 38
	tdsStartEmbeddedOpcode = 39
	tdsEndEmbeddedOpcode   = 40
	tdsStartADT            = 41
	tdsEndADT              = 42
	tdsSubtypeMarker       = 43
	tdsEmbeddedInfo        = 44
	tdsCollectionOpcode    = 28
	tdsUPTOpcode           = 27
)

type tdsReader struct {
	data []byte
	pos  int
}

func (r *tdsReader) remaining(n int) bool { return n >= 0 && r.pos <= len(r.data)-n }

func (r *tdsReader) readByte() (byte, error) {
	if !r.remaining(1) {
		return 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *tdsReader) readUB2() (int, error) {
	if !r.remaining(2) {
		return 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	v := int(r.data[r.pos])<<8 | int(r.data[r.pos+1])
	r.pos += 2
	return v, nil
}

func (r *tdsReader) readUB4() (int, error) {
	if !r.remaining(4) {
		return 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	v := int(uint32(r.data[r.pos])<<24 | uint32(r.data[r.pos+1])<<16 | uint32(r.data[r.pos+2])<<8 | uint32(r.data[r.pos+3]))
	r.pos += 4
	return v, nil
}

func parseTDS(typ *ObjectType) error {
	root, err := parseTDSBytes(typ.TDS)
	if err != nil {
		common.Odl.Error("ADT metadata error")
		return err
	}
	if err := resolveTDSPatches(root); err != nil {
		common.Odl.Error("ADT metadata error")
		return err
	}
	typ.tdsType = root
	applyTDSType(root, typ)
	return nil
}

func parseTDSBytes(data []byte) (*tdsType, error) {
	r := &tdsReader{data: data}
	if !r.remaining(18) {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if _, err := r.readUB4(); err != nil { // TDS byte length
		return nil, err
	}
	opcode, err := r.readByte()
	if err != nil || opcode != tdsVersionOpcode {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if _, err = r.readByte(); err != nil { // TDS version
		return nil, err
	}
	if !r.remaining(2) {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	r.pos += 2
	attributeCount, err := r.readUB2()
	if err != nil {
		return nil, err
	}
	if _, err = r.readByte(); err != nil { // descriptor flags
		return nil, err
	}
	opcode, err = r.readByte()
	if err != nil || opcode != tdsStartADT {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if n, err := r.readUB2(); err != nil || n != 0 {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if _, err = r.readUB4(); err != nil { // index-table offset
		return nil, err
	}

	root, err := parseTDSRecords(r, tdsEndADT)
	if err != nil {
		return nil, err
	}
	if len(root.attributes) != attributeCount {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return root, nil
}

func parseTDSRecords(r *tdsReader, endOpcode byte) (*tdsType, error) {
	root := &tdsType{source: r.data}
	for {
		opcode, err := r.readByte()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case endOpcode:
			return root, nil
		case tdsSubtypeMarker:
			continue
		case tdsEmbeddedInfo:
			if _, err := r.readByte(); err != nil {
				return nil, err
			}
			continue
		case tdsStartEmbeddedOpcode:
			embedded, err := parseTDSRecords(r, tdsEndEmbeddedOpcode)
			if err != nil {
				return nil, err
			}
			root.attributes = append(root.attributes, embedded)
		default:
			node, err := parseTDSRecord(r, opcode)
			if err != nil {
				return nil, err
			}
			root.attributes = append(root.attributes, node)
		}
	}
}

func parseTDSRecord(r *tdsReader, opcode byte) (*tdsType, error) {
	node := &tdsType{source: r.data}
	switch opcode {
	case 1, 7: // CHAR, VARCHAR2
		size, err := r.readUB2()
		if err != nil {
			return nil, err
		}
		if !r.remaining(3) { // form and character set
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		r.pos += 3
		node.dtyType, node.size = common.DtyVCS, size
	case 2: // DATE
		node.dtyType, node.size = common.DtyDat, 7
	case 3, 4, 5, 6: // DECIMAL, DOUBLE, FLOAT, NUMBER
		precision, err := r.readByte()
		if err != nil {
			return nil, err
		}
		scale, err := r.readByte()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size = common.DtyNum, 22
		node.precision, node.scale = int16(precision), int8(scale)
	case 8: // SINT32 / PLS_INTEGER
		node.dtyType, node.size = common.DtyBol, 4
	case 19: // RAW
		size, err := r.readUB2()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size = common.DtyBin, size
	case 21: // TIMESTAMP
		precision, err := r.readByte()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size, node.fsPrecision = common.DtyStamp, 11, uint8(precision)
	case 23: // TIMESTAMP WITH TIME ZONE
		precision, err := r.readByte()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size, node.fsPrecision = common.DtyStz, 13, uint8(precision)
	case 33: // TIMESTAMP WITH LOCAL TIME ZONE
		precision, err := r.readByte()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size, node.fsPrecision = common.DtySitz, 11, uint8(precision)
	case 37:
		node.dtyType, node.size = common.DtyIbFloat, 4
	case 45:
		node.dtyType, node.size = common.DtyIbDouble, 8
	case tdsCollectionOpcode:
		elementOffset, err := r.readUB4()
		if err != nil {
			return nil, err
		}
		upperBound, err := r.readUB4()
		if err != nil {
			return nil, err
		}
		userCode, err := r.readByte()
		if err != nil || (userCode != 2 && userCode != 3) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if elementOffset < 0 || elementOffset >= len(r.data) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		elementReader := &tdsReader{data: r.data, pos: elementOffset}
		elementOpcode, err := elementReader.readByte()
		if err != nil {
			return nil, err
		}
		element, err := parseTDSRecord(elementReader, elementOpcode)
		if err != nil {
			return nil, err
		}
		node.collection, node.varray = true, userCode == 3
		node.upperBound, node.element = int64(upperBound), element
	case tdsUPTOpcode:
		offset, err := r.readUB4()
		if err != nil {
			return nil, err
		}
		code, err := r.readByte()
		if err != nil || (code != 250 && code != 251) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if offset < 0 || offset >= len(r.data) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		node.deferred = &tdsPatch{offset: offset, code: code}
	default:
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return node, nil
}

func resolveTDSPatches(node *tdsType) error {
	if node == nil {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if node.deferred != nil {
		patch := node.deferred
		// A normal UPT patch starts with its patch opcode. ADT patches carry a
		// four-byte ADT-length field before the nested TDS; collection patches
		// start the TDS immediately after that opcode.
		start := patch.offset + 1
		if patch.code == 250 {
			start += 4
		}
		if start < 0 || start >= len(node.source) {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		resolved, err := parseTDSBytes(node.source[start:])
		if err != nil {
			return err
		}
		// A named collection is wrapped in a one-attribute ADT TDS. Match the
		// JDBC cleanup step by exposing that collection record to its parent;
		// an ADT patch retains its ordered attribute tree.
		if len(resolved.attributes) == 1 && resolved.attributes[0].collection {
			*node = *resolved.attributes[0]
		} else {
			*node = *resolved
		}
	}
	if node.element != nil {
		if err := resolveTDSPatches(node.element); err != nil {
			return err
		}
	}
	for _, attribute := range node.attributes {
		if err := resolveTDSPatches(attribute); err != nil {
			return err
		}
	}
	return nil
}

func applyTDSType(root *tdsType, typ *ObjectType) {
	if len(root.attributes) != 1 || !root.attributes[0].collection {
		return
	}
	collection := root.attributes[0]
	typ.Collection = true
	typ.VArray = collection.varray
	typ.UpperBound = collection.upperBound
	typ.ElementType = collection.element.dtyType
	typ.ElementSize = collection.element.size
	typ.Precision = collection.element.precision
	typ.Scale = collection.element.scale
	typ.FsPrecision = collection.element.fsPrecision
	if len(collection.element.attributes) != 0 {
		typ.CollectionOf = newObjectTypeFromTDSType(collection.element, "", "", "", nil)
	}
}

// SetName parses name and assigns the descriptor's schema, package, and type
// name components. The expected form is TYPE, SCHEMA.TYPE, or
// SCHEMA.PACKAGE.TYPE.
func (t *ObjectType) SetName(name string) {
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		t.Schema = parts[0]
		t.Name = parts[len(parts)-1]
		if len(parts) > 2 {
			t.PackageName = strings.Join(parts[1:len(parts)-1], ".")
		}
		return
	}
	t.Name = name
}

// FullName returns the fully qualified database type name.
func (t *ObjectType) FullName() string {
	if t.PackageName != "" {
		return t.Schema + "." + t.PackageName + "." + t.Name
	}
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// IsObject reports whether t represents an object type rather than a collection.
func (t *ObjectType) IsObject() bool { return t != nil && !t.Collection }

// String returns the fully qualified database type name.
func (t *ObjectType) String() string { return t.FullName() }

// Close marks t as closed. A closed descriptor cannot create new values.
func (t *ObjectType) Close() error {
	if t != nil {
		t.Closed = true
	}
	return nil
}

// AttributeNames returns object attribute names in database declaration order.
func (t *ObjectType) AttributeNames() []string {
	names := make([]string, 0, len(t.Attributes))
	for i := uint32(1); i <= uint32(len(t.Attributes)); i++ {
		for _, a := range t.Attributes {
			if a.Sequence == i {
				names = append(names, a.Name)
				break
			}
		}
	}
	return names
}

// NewObject creates an empty Object value for t.
// It returns an error if t is closed or represents a collection.
func (t *ObjectType) NewObject() (*Object, error) {
	if t == nil || t.Closed {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if !t.IsObject() {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return &Object{ObjectType: t, Attributes: make(map[string]any)}, nil
}

// NewCollection creates an empty ObjectCollection value for t.
// It returns an error if t is closed or does not represent a collection.
func (t *ObjectType) NewCollection() (ObjectCollection, error) {
	if t == nil || t.Closed || !t.Collection {
		return ObjectCollection{}, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return ObjectCollection{Object: &Object{ObjectType: t, Attributes: make(map[string]any)}}, nil
}

// Close marks o as closed. A closed object cannot be read or updated.
func (o *Object) Close() error {
	if o != nil {
		o.Closed = true
	}
	return nil
}

// Collection returns o as an ObjectCollection when it represents a collection.
// It returns a zero ObjectCollection for ordinary object values.
func (o *Object) Collection() ObjectCollection {
	if o != nil && o.ObjectType != nil && o.ObjectType.Collection {
		return ObjectCollection{Object: o}
	}
	return ObjectCollection{}
}

// Get returns the value of the named object attribute.
func (o *Object) Get(name string) (any, error) {
	if o == nil || o.Closed || o.Null {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	v, ok := o.Attributes[name]
	if !ok {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return v, nil
}

// Set assigns value to the named object attribute.
// It returns an error when o is closed or name is not an attribute of o's type.
func (o *Object) Set(name string, value any) error {
	if o == nil || o.Closed || o.ObjectType == nil {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if _, ok := o.ObjectType.Attributes[name]; !ok {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if o.Attributes == nil {
		o.Attributes = make(map[string]any)
	}
	o.Null = false
	o.Attributes[name] = value
	return nil
}

// SetNull marks o as a NULL object. NULL is distinct from an object whose
// individual attributes are NULL.
func (o *Object) SetNull() {
	if o != nil {
		o.Null = true
	}
}

// IsNull reports whether o represents a NULL object.
func (o *Object) IsNull() bool { return o == nil || o.Null }

// SetNull marks c as a NULL collection. NULL is distinct from an empty collection.
func (c *ObjectCollection) SetNull() {
	if c != nil {
		c.Null = true
		if c.Object != nil {
			c.Object.Null = true
		}
	}
}

// IsNull reports whether c represents a NULL collection.
func (c ObjectCollection) IsNull() bool { return c.Null || c.Object == nil || c.Object.Null }

// Len returns the number of elements in c.
func (c ObjectCollection) Len() (int, error) {
	if c.Object == nil || c.Closed {
		return 0, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return len(c.Values), nil
}

// Append adds v to the end of c. It returns an error when c is invalid or the
// VARRAY upper bound would be exceeded.
func (c *ObjectCollection) Append(v any) error {
	if c.Object == nil || c.ObjectType == nil || !c.ObjectType.Collection {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	if c.UpperBound > 0 && int64(len(c.Values)+1) > c.UpperBound {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.Null = false
	c.Object.Null = false
	c.Values = append(c.Values, v)
	return nil
}

// Get returns the one-based element at index i.
func (c ObjectCollection) Get(i int) (any, error) {
	if i < 1 || i > len(c.Values) {
		return nil, common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	return c.Values[i-1], nil
}

// Set replaces the one-based element at index i with v.
func (c ObjectCollection) Set(i int, v any) error {
	if i < 1 || i > len(c.Values) {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.Values[i-1] = v
	return nil
}

// First returns the first one-based collection index, or zero for an empty collection.
func (c ObjectCollection) First() (int, error) {
	if len(c.Values) == 0 {
		return 0, nil
	}
	return 1, nil
}

// Last returns the last one-based collection index, or zero for an empty collection.
func (c ObjectCollection) Last() (int, error) { return len(c.Values), nil }

// Next returns the one-based index following i, or zero when there is no next element.
func (c ObjectCollection) Next(i int) (int, error) {
	if i < 0 || i >= len(c.Values) {
		return 0, nil
	}
	return i + 1, nil
}

// Trim removes n elements from the end of c.
func (c ObjectCollection) Trim(n int) error {
	if n < 0 || n > len(c.Values) {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	c.Values = c.Values[:len(c.Values)-n]
	return nil
}
