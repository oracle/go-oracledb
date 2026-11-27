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
	"context"
	"database/sql/driver"
	"io"
	"strconv"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// adtDescriptor owns the database identity, hierarchy and wire shape of a named type.
type adtDescriptor struct {
	*adtCodecContext
	// CollectionOf describes the element object type when the collection contains
	// object values.
	CollectionOf *adtDescriptor
	// Attributes contains object attributes keyed by their database names.
	Attributes map[string]adtAttribute
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
	SubTypes []*adtDescriptor
	// VArray reports whether this collection is a VARRAY rather than another
	// collection kind.
	VArray bool
	// Associative reports whether this collection is a PL/SQL INDEX BY
	// PLS_INTEGER table.
	Associative bool
	// UpperBound is the maximum number of elements allowed in a VARRAY.
	UpperBound int64
	// ElementType and ElementSize describe scalar collection elements.
	ElementType common.DtyType
	ElementSize int
	// Closed reports whether Close has been called for this descriptor.
	Closed bool

	shape          *adtShape
	subTypesByTOID map[string]*adtDescriptor
}

// adtCodecContext is shared by descriptors on one physical connection. The name
// index selects the runtime type of named objects inside anonymous collection shapes.
type adtCodecContext struct {
	codecs codecFactory
	byName map[string]*adtDescriptor
}

// attachCodecs initializes named and anonymous children before publishing a descriptor.
// Marking the node first handles shared or recursive type definitions.
func (d *adtDescriptor) attachCodecs(ctx *adtCodecContext) {
	if d == nil || d.adtCodecContext == ctx {
		return
	}
	d.adtCodecContext = ctx
	d.CollectionOf.attachCodecs(ctx)
	for _, a := range d.Attributes {
		a.adtDescriptor.attachCodecs(ctx)
	}
	for _, sub := range d.SubTypes {
		sub.attachCodecs(ctx)
	}
}

type adtAttribute struct {
	// adtDescriptor describes the attribute's Oracle type.
	*adtDescriptor
	// Name is the attribute name as declared by the database type.
	Name string
	// Sequence is the one-based attribute position in the database type.
	Sequence uint32
}

// SetName parses name and assigns the descriptor's schema, package, and type
// name components. The expected form is TYPE, SCHEMA.TYPE, or
// SCHEMA.PACKAGE.TYPE.
func (t *adtDescriptor) SetName(name string) {
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
func (t *adtDescriptor) FullName() string {
	if t.PackageName != "" {
		return t.Schema + "." + t.PackageName + "." + t.Name
	}
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// AttributeNames returns object attribute names in database declaration order.
func (t *adtDescriptor) AttributeNames() []string {
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

// public returns a detached view. Callers can change it without affecting the connection cache.
func (d *adtDescriptor) public() *datatype.ObjectType {
	return d.describe(make(map[*adtDescriptor]*datatype.ObjectType))
}

func (d *adtDescriptor) describe(seen map[*adtDescriptor]*datatype.ObjectType) *datatype.ObjectType {
	if d == nil {
		return nil
	}
	if p := seen[d]; p != nil {
		return p
	}
	p := &datatype.ObjectType{
		Schema: d.Schema, Name: d.Name, PackageName: d.PackageName,
		Collection: d.Collection, VArray: d.VArray, Associative: d.Associative, UpperBound: d.UpperBound,
		ElementType: d.ElementType, ElementSize: d.ElementSize, Precision: d.Precision, Scale: d.Scale, FsPrecision: d.FsPrecision,
		DBSize: d.DBSize, ClientSizeInBytes: d.ClientSizeInBytes, CharSize: d.CharSize,
		Closed: d.Closed, Attributes: make(map[string]datatype.ObjectAttribute, len(d.Attributes)),
	}
	seen[d] = p
	for k, a := range d.Attributes {
		p.Attributes[k] = datatype.ObjectAttribute{Name: a.Name, Sequence: a.Sequence, ObjectType: a.adtDescriptor.describe(seen)}
	}
	p.CollectionOf = d.CollectionOf.describe(seen)
	return p
}
func (d *adtDescriptor) NewObject() (*datatype.Object, error) { return d.public().NewObject() }
func (d *adtDescriptor) NewCollection() (datatype.ObjectCollection, error) {
	return d.public().NewCollection()
}
func (d *adtDescriptor) IsObject() bool { return d != nil && !d.Collection }

// parseDescriptor applies the resolved TDS shape to its owning descriptor.
func parseDescriptor(d *adtDescriptor) error {
	root, err := parseADTShape(d.TDS)
	if err != nil {
		return err
	}
	if err = resolveADTShapePatches(root); err != nil {
		return err
	}
	d.shape = root
	applyTDSType(root, d)
	return nil
}

// typeShapeResult is the result of a physical-connection ADT metadata
// lookup. The transport owns cursor execution; this package owns parsing and
// shaping the public descriptor.
type typeShapeResult struct {
	ReturnCode    int64
	CanonicalName string
	TOID          []byte
	Version       int64
	TDS           []byte
	Instantiable  string
	SuperOwner    string
	SuperName     string
	AttributeRows driver.Rows
	SubtypeRows   driver.Rows
}

// adtMetadataLoader retrieves the metadata for an Oracle named type.
type adtMetadataLoader interface {
	fetchTypeShape(context.Context, string) (typeShapeResult, error)
}

func loadObjectType(ctx context.Context, loader adtMetadataLoader, typeName string, cache map[string]*adtDescriptor) (*adtDescriptor, error) {
	canonicalKey := strings.ToUpper(strings.TrimSpace(typeName))
	if typ := cache[canonicalKey]; typ != nil {
		return typ, nil
	}

	metadata, err := loader.fetchTypeShape(ctx, typeName)
	defer closeRows(metadata.AttributeRows)
	defer closeRows(metadata.SubtypeRows)
	if err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	attributeRows := metadata.AttributeRows
	subtypeRows := metadata.SubtypeRows
	if metadata.ReturnCode != 0 || len(metadata.TOID) != 16 || len(metadata.TDS) == 0 {
		common.Odl.Error("ADT metadata error", "type", metadata.CanonicalName, "returnCode", metadata.ReturnCode,
			"toidLength", len(metadata.TOID), "tdsLength", len(metadata.TDS))
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}

	typ := &adtDescriptor{
		Attributes:  make(map[string]adtAttribute),
		TOID:        append([]byte(nil), metadata.TOID...),
		TypeVersion: int(metadata.Version),
		TDS:         append([]byte(nil), metadata.TDS...),
	}
	typ.SetName(metadata.CanonicalName)
	typ.Instantiable = strings.EqualFold(metadata.Instantiable, "YES")
	typ.SuperTypeName = qualifiedTypeName(metadata.SuperOwner, "", metadata.SuperName)
	cache[canonicalKey] = typ
	if err := parseDescriptor(typ); err != nil {
		return nil, err
	}
	if err := populateObjectAttributes(ctx, loader, typ, attributeRows, cache); err != nil {
		common.Odl.Error("ADT metadata error")
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	if err := populateObjectSubtypes(ctx, loader, typ, subtypeRows, cache); err != nil {
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

func populateObjectSubtypes(ctx context.Context, loader adtMetadataLoader, typ *adtDescriptor, rows driver.Rows, cache map[string]*adtDescriptor) error {
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
		subtype, err := loadObjectType(ctx, loader, qualifiedTypeName(owner, "", name), cache)
		if err != nil {
			return err
		}
		if typ.subTypesByTOID == nil {
			typ.subTypesByTOID = make(map[string]*adtDescriptor)
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
func (t *adtDescriptor) SubTypeForTOID(toid []byte) *adtDescriptor {
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
func populateObjectAttributes(ctx context.Context, loader adtMetadataLoader, typ *adtDescriptor, rows driver.Rows, cache map[string]*adtDescriptor) error {
	if typ == nil || typ.shape == nil {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if rows == nil {
		return nil
	}
	// For scalar collections Oracle returns an attribute-cursor row describing
	// the scalar element (for example NUMBER or VARCHAR2). Its complete shape
	// is already present in the collection TDS; it is not a named child type
	// that can be loaded through GET_TYPE_SHAPE.
	if typ.Collection && typ.CollectionOf == nil {
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
		// The sequence, not the possibly empty attribute name, determines wire order.
		if !ok {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		sequence, ok := adtMetadataInt(values[2])
		metadataType := typ
		nodes := typ.shape.attributes
		if typ.Collection && typ.CollectionOf != nil && len(nodes) == 1 && nodes[0].collection && nodes[0].element != nil && len(nodes[0].element.attributes) != 0 {
			metadataType = typ.CollectionOf
			nodes = nodes[0].element.attributes
		}
		if !ok || sequence < 1 || sequence > len(nodes) {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		// A collection-element UPT may have supplied an anonymous positional
		// attribute from its TDS. Replace it when the metadata cursor supplies
		// the declared database name for the same position.
		for existingName, existing := range metadataType.Attributes {
			if existing.Sequence == uint32(sequence) && existingName != name {
				delete(metadataType.Attributes, existingName)
			}
		}
		if _, exists := metadataType.Attributes[name]; exists {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		// GET_TYPE_SHAPE returns the attribute type name before its owner.
		// Preserve that order when resolving nested named object and collection
		// descriptors; reversing these fields produces TYPE.OWNER names.
		typeName, _ := adtMetadataString(values[3])
		owner, _ := adtMetadataString(values[4])
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
			loaded, loadErr := loadObjectType(ctx, loader, fullName, cache)
			if loadErr != nil {
				return loadErr
			}
			attributeType = loaded
		}
		metadataType.Attributes[name] = adtAttribute{
			adtDescriptor: attributeType,
			Name:          name,
			Sequence:      uint32(sequence),
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

func newObjectTypeFromTDSType(node *adtShape, schema, name, packageName string, oid []byte) *adtDescriptor {
	typ := &adtDescriptor{
		Attributes:  make(map[string]adtAttribute),
		Schema:      schema,
		Name:        name,
		PackageName: packageName,
		TOID:        append([]byte(nil), oid...),
		shape:       node,
	}
	if node == nil {
		return typ
	}
	if node.collection {
		typ.Collection = true
		typ.VArray = node.varray
		typ.Associative = node.associative
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
	// A UPT embedded in a collection TDS preserves the element's complete wire
	// shape but has no SQL name or attribute-name metadata. Keep a usable
	// anonymous positional descriptor here, without issuing an additional
	// dictionary query.
	for index, attribute := range node.attributes {
		attributeName := "F_" + strconv.Itoa(index+1)
		typ.Attributes[attributeName] = adtAttribute{
			adtDescriptor: newObjectTypeFromTDSType(attribute, "", "", "", nil),
			Name:          attributeName,
			Sequence:      uint32(index + 1),
		}
	}
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

func applyTDSType(root *adtShape, typ *adtDescriptor) {
	if len(root.attributes) != 1 || !root.attributes[0].collection {
		return
	}
	collection := root.attributes[0]
	typ.Collection = true
	typ.VArray = collection.varray
	typ.Associative = collection.associative
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
