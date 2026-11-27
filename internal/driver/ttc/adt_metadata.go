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
	"database/sql"
	"database/sql/driver"
	"maps"
	"strings"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const getTypeShapeSQL = `BEGIN
  :1 := SYS.DBMS_PICKLER.GET_TYPE_SHAPE(:2, :3, :4, :5, :6, :7, :8, :9, :10);
END;`

// adtShape is the TTC-private parsed form of a TDS node. It is deliberately
// separate from datatype.ObjectType so parser and codec decisions never need
// to expose protocol state through the public ADT API.
type adtShape struct {
	dtyType     common.DtyType
	size        int
	precision   int16
	scale       int8
	fsPrecision uint8
	collection  bool
	varray      bool
	associative bool
	upperBound  int64
	element     *adtShape
	attributes  []*adtShape
	deferred    *adtShapePatch
	source      driverCommon.B1Array
}

type adtShapePatch struct {
	offset int
	code   byte
}

// adtShapeReader reads the bounded binary TDS stream. Keeping this reader in
// TTC prevents protocol bytes from crossing into the public datatype package.
type adtShapeReader struct {
	data []byte
	pos  int
}

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

func parseADTShape(data []byte) (*adtShape, error) {
	r := &adtShapeReader{data: data}
	if !r.remaining(18) {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if _, err := r.readUB4(); err != nil {
		return nil, err
	}
	opcode, err := r.readByte()
	if err != nil || opcode != tdsVersionOpcode {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if _, err = r.readByte(); err != nil {
		return nil, err
	}
	if !r.remaining(2) {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	r.pos += 2
	count, err := r.readUB2()
	if err != nil {
		return nil, err
	}
	if _, err = r.readByte(); err != nil {
		return nil, err
	}
	opcode, err = r.readByte()
	if err != nil || opcode != tdsStartADT {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if n, err := r.readUB2(); err != nil || n != 0 {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if _, err = r.readUB4(); err != nil {
		return nil, err
	}
	root, err := parseADTShapeRecords(r, tdsEndADT)
	if err != nil || len(root.attributes) != count {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, err)
	}
	return root, nil
}

func parseADTShapeRecords(r *adtShapeReader, end byte) (*adtShape, error) {
	root := &adtShape{source: r.data}
	for {
		opcode, err := r.readByte()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case end:
			return root, nil
		case tdsSubtypeMarker:
			continue
		case tdsEmbeddedInfo:
			if _, err := r.readByte(); err != nil {
				return nil, err
			}
		case tdsStartEmbeddedOpcode:
			child, err := parseADTShapeRecords(r, tdsEndEmbeddedOpcode)
			if err != nil {
				return nil, err
			}
			root.attributes = append(root.attributes, child)
		default:
			child, err := parseADTShapeRecord(r, opcode)
			if err != nil {
				return nil, err
			}
			root.attributes = append(root.attributes, child)
		}
	}
}

func parseADTShapeRecord(r *adtShapeReader, opcode byte) (*adtShape, error) {
	node := &adtShape{source: r.data}
	switch opcode {
	case 1, 7:
		size, err := r.readUB2()
		if err != nil {
			return nil, err
		}
		if !r.remaining(3) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		r.pos += 3
		node.dtyType, node.size = common.DtyVCS, size
	case 2:
		node.dtyType, node.size = common.DtyDat, 7
	case 3, 4, 5, 6:
		precision, err := r.readByte()
		if err != nil {
			return nil, err
		}
		scale, err := r.readByte()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size, node.precision, node.scale = common.DtyNum, 22, int16(precision), int8(scale)
	case 8:
		node.dtyType, node.size = common.DtyBol, 4
	case 19:
		size, err := r.readUB2()
		if err != nil {
			return nil, err
		}
		node.dtyType, node.size = common.DtyBin, size
	case 21, 23, 33:
		precision, err := r.readByte()
		if err != nil {
			return nil, err
		}
		node.fsPrecision = uint8(precision)
		if opcode == 21 {
			node.dtyType, node.size = common.DtyStamp, 11
		} else if opcode == 23 {
			node.dtyType, node.size = common.DtyStz, 13
		} else {
			node.dtyType, node.size = common.DtySitz, 11
		}
	case 37:
		node.dtyType, node.size = common.DtyIbFloat, 4
	case 45:
		node.dtyType, node.size = common.DtyIbDouble, 8
	case tdsCollectionOpcode:
		offset, err := r.readUB4()
		if err != nil {
			return nil, err
		}
		bound, err := r.readUB4()
		if err != nil {
			return nil, err
		}
		code, err := r.readByte()
		if err != nil || (code != 1 && code != 2 && code != 3) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if offset < 0 || offset >= len(r.data) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		childReader := &adtShapeReader{data: r.data, pos: offset}
		childOpcode, err := childReader.readByte()
		if err != nil {
			return nil, err
		}
		child, err := parseADTShapeRecord(childReader, childOpcode)
		if err != nil {
			return nil, err
		}
		node.collection, node.varray, node.associative, node.upperBound, node.element = true, code == 3, code == 1, int64(bound), child
	case tdsUPTOpcode:
		offset, err := r.readUB4()
		if err != nil {
			return nil, err
		}
		code, err := r.readByte()
		if err != nil || (code != 250 && code != 251) || offset < 0 || offset >= len(r.data) {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		node.deferred = &adtShapePatch{offset: offset, code: code}
	default:
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	return node, nil
}

func resolveADTShapePatches(node *adtShape) error {
	if node == nil {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if node.deferred != nil {
		start := node.deferred.offset + 1
		if node.deferred.code == 250 {
			start += 4
		}
		if start < 0 || start >= len(node.source) {
			return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		resolved, err := parseADTShape(node.source[start:])
		if err != nil {
			return err
		}
		if len(resolved.attributes) == 1 && resolved.attributes[0].collection {
			*node = *resolved.attributes[0]
		} else {
			*node = *resolved
		}
	}
	if node.element != nil {
		if err := resolveADTShapePatches(node.element); err != nil {
			return err
		}
	}
	for _, child := range node.attributes {
		if err := resolveADTShapePatches(child); err != nil {
			return err
		}
	}
	return nil
}

func (r *adtShapeReader) remaining(n int) bool { return n >= 0 && r.pos <= len(r.data)-n }

func (r *adtShapeReader) readByte() (byte, error) {
	if !r.remaining(1) {
		return 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	value := r.data[r.pos]
	r.pos++
	return value, nil
}

func (r *adtShapeReader) readUB2() (int, error) {
	if !r.remaining(2) {
		return 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	value := int(r.data[r.pos])<<8 | int(r.data[r.pos+1])
	r.pos += 2
	return value, nil
}

func (r *adtShapeReader) readUB4() (int, error) {
	if !r.remaining(4) {
		return 0, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	value := int(uint32(r.data[r.pos])<<24 | uint32(r.data[r.pos+1])<<16 | uint32(r.data[r.pos+2])<<8 | uint32(r.data[r.pos+3]))
	r.pos += 4
	return value, nil
}

// resolveADTMetadata resolves named-type placeholders before a statement builds
// OACs or encodes bind data. Resolution is scoped to this physical connection.
func (c *connection) resolveADTMetadata(ctx context.Context, args []driver.NamedValue) error {
	for _, arg := range args {
		if err := c.resolveADTBindValue(ctx, arg.Value, nil, 0); err != nil {
			return err
		}
	}
	return nil
}

func (c *connection) resolveADTBindValue(ctx context.Context, value any, declared *adtDescriptor, depth int) error {
	if out, ok := value.(sql.Out); ok {
		return c.resolveADTBindValue(ctx, out.Dest, declared, depth)
	}
	object, _ := objectValue(value)
	collection, requestedCollection := collectionForBind(value)
	if requestedCollection {
		object = collection.Object
	}
	// A zero-value OUT destination gets its descriptor from a compatible bind.
	if object == nil || object.ObjectType == nil {
		return nil
	}
	typ := object.ObjectType
	if depth > 255 || typ.Closed {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	name := strings.TrimSpace(typ.FullName())
	descriptor := declared
	if name != "" {
		var err error
		descriptor, err = c.resolveDescriptor(ctx, name)
		if err != nil {
			return err
		}
	}
	if descriptor == nil {
		return common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	if descriptor.Collection != requestedCollection || (typ.Associative && !descriptor.Associative) {
		return common.NewOracleError(oracleErrors.ADTValueError, nil)
	}
	*typ = *descriptor.public()
	if !requestedCollection {
		for name, attributeValue := range object.Attributes {
			if err := c.resolveADTBindValue(ctx, attributeValue, descriptor.Attributes[name].adtDescriptor, depth+1); err != nil {
				return err
			}
		}
	}
	if requestedCollection && !collection.IsNull() && descriptor.CollectionOf != nil {
		values, err := collection.Get()
		if err != nil {
			return err
		}
		for _, element := range values {
			if err := c.resolveADTBindValue(ctx, element, descriptor.CollectionOf, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveDescriptor publishes a complete descriptor graph only after all cursor reads succeed.
func (c *connection) resolveDescriptor(ctx context.Context, name string) (*adtDescriptor, error) {
	key := strings.ToUpper(name)
	descriptor := c.shelf.adtByName[key]
	if descriptor == nil {
		// Resolve into a private snapshot: a failed recursive load must not publish a partial descriptor.
		pending := maps.Clone(c.shelf.adtByName)
		var err error
		descriptor, err = loadObjectType(ctx, c, name, pending)
		if err != nil {
			return nil, err
		}
		if c.adtCodecs == nil {
			c.adtCodecs = &adtCodecContext{codecs: c.shelf.GetCodecFactory(), byName: c.shelf.adtByName}
		}
		descriptor.attachCodecs(c.adtCodecs)
		for alias, d := range pending {
			c.shelf.adtByName[alias] = d
			c.shelf.adtByName[strings.ToUpper(d.FullName())] = d
			c.shelf.adtByTOID[string(d.TOID)] = d
		}
	}
	return descriptor, nil
}

// resolveADTColumns resolves the server-described type on a cold connection before rows are decoded.
func (c *connection) resolveADTColumns(ctx context.Context, columns []columnContext) error {
	for i := range columns {
		col := &columns[i]
		if col.DataType != common.DtyINty && col.DataType != common.DtyNty {
			continue
		}
		d := c.shelf.adtByTOID[string(col.NamedTypeTOID)]
		if d == nil && len(col.DBTypeName) > 0 {
			var err error
			d, err = c.resolveDescriptor(ctx, qualifiedTypeName(string(col.SchemaName), "", string(col.DBTypeName)))
			if err != nil {
				return err
			}
		}
		col.objectType = d
	}
	return nil
}

// fetchTypeShape loads GET_TYPE_SHAPE metadata and fetches its cursor
// outputs on this connection before TTC parses the descriptor.
func (c *connection) fetchTypeShape(ctx context.Context, typeName string) (typeShapeResult, error) {
	var result typeShapeResult
	canonical := strings.TrimSpace(typeName)
	var attributes, subtypes datatype.Cursor
	args := []driver.NamedValue{
		{Ordinal: 1, Value: sql.Out{Dest: &result.ReturnCode}},
		{Ordinal: 2, Value: sql.Out{Dest: &canonical, In: true}},
		{Ordinal: 3, Value: sql.Out{Dest: &result.TOID}},
		{Ordinal: 4, Value: sql.Out{Dest: &result.Version}},
		{Ordinal: 5, Value: sql.Out{Dest: &result.TDS}},
		{Ordinal: 6, Value: sql.Out{Dest: &result.Instantiable}},
		{Ordinal: 7, Value: sql.Out{Dest: &result.SuperOwner}},
		{Ordinal: 8, Value: sql.Out{Dest: &result.SuperName}},
		{Ordinal: 9, Value: sql.Out{Dest: &attributes}},
		{Ordinal: 10, Value: sql.Out{Dest: &subtypes}},
	}
	_, execErr := c.ExecContext(ctx, getTypeShapeSQL, args)
	result.CanonicalName = canonical
	result.AttributeRows = attributes.DriverRows()
	result.SubtypeRows = subtypes.DriverRows()
	if execErr != nil {
		return result, execErr
	}
	if result.AttributeRows != nil {
		if err := c.GetRows(ctx, result.AttributeRows); err != nil {
			return result, err
		}
	}
	if result.SubtypeRows != nil {
		if err := c.GetRows(ctx, result.SubtypeRows); err != nil {
			return result, err
		}
	}
	return result, nil
}
