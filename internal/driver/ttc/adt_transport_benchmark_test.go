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
	"reflect"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

var benchmarkADTResult any

// These benchmarks compare the current map-backed Object representation with
// a candidate runtime struct representation. The reflected type is created
// once, as a production implementation would cache it by ADT metadata.
func BenchmarkADTObjectMapSet(b *testing.B) {
	typ := benchmarkADTObjectType()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		object, err := newBenchmarkMapObject(typ)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkADTResult = object
	}
}

func BenchmarkADTObjectDynamicStructSet(b *testing.B) {
	dynamicType := benchmarkADTDynamicType()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value := newBenchmarkDynamicValue(dynamicType)
		benchmarkADTResult = value.Interface()
	}
}

// BenchmarkADTObjectDynamicStructSetByName measures the public-style dynamic
// field access a caller would use when the Oracle attribute name is known only
// at runtime. BenchmarkADTObjectDynamicStructSet uses cached field indexes,
// which is the optimized implementation path.
func BenchmarkADTObjectDynamicStructSetByName(b *testing.B) {
	dynamicType := benchmarkADTDynamicType()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value := reflect.New(dynamicType).Elem()
		value.FieldByName("ID").SetInt(42)
		value.FieldByName("CUSTOMER_NAME").SetString("Ada Lovelace")
		value.FieldByName("STATUS").SetString("APPROVED")
		value.FieldByName("TOTAL_AMOUNT").SetInt(12500)
		benchmarkADTResult = value.Interface()
	}
}

func BenchmarkADTObjectMapEncode(b *testing.B) {
	typ := benchmarkADTObjectType()
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	object, err := newBenchmarkMapObject(typ)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		image, err := encodeObjectImage(object, factory)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkADTResult = image
	}
}

func BenchmarkADTObjectDynamicStructEncode(b *testing.B) {
	typ := benchmarkADTObjectType()
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	value := newBenchmarkDynamicValue(benchmarkADTDynamicType())
	mapObject, err := newBenchmarkMapObject(typ)
	if err != nil {
		b.Fatal(err)
	}
	want, err := encodeObjectImage(mapObject, factory)
	if err != nil {
		b.Fatal(err)
	}
	got, err := benchmarkEncodeDynamicObjectImage(value, typ, factory)
	if err != nil {
		b.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		b.Fatalf("direct dynamic image = %x, want %x", got, want)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		image, err := benchmarkEncodeDynamicObjectImage(value, typ, factory)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkADTResult = image
	}
}

func BenchmarkADTObjectMapDecode(b *testing.B) {
	typ := benchmarkADTObjectType()
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	object, err := newBenchmarkMapObject(typ)
	if err != nil {
		b.Fatal(err)
	}
	image, err := encodeObjectImage(object, factory)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoded, err := decodeObjectImage(image, typ, factory)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkADTResult = decoded
	}
}

func BenchmarkADTObjectDynamicStructDecode(b *testing.B) {
	typ := benchmarkADTObjectType()
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	object, err := newBenchmarkMapObject(typ)
	if err != nil {
		b.Fatal(err)
	}
	image, err := encodeObjectImage(object, factory)
	if err != nil {
		b.Fatal(err)
	}
	dynamicType := benchmarkADTDynamicType()
	value, err := benchmarkDecodeDynamicObjectImage(image, typ, dynamicType, factory)
	if err != nil {
		b.Fatal(err)
	}
	if got := value.Field(0).Int(); got != 42 {
		b.Fatalf("direct decoded ID = %d, want 42", got)
	}
	if got := value.Field(1).String(); got != "Ada Lovelace" {
		b.Fatalf("direct decoded CUSTOMER_NAME = %q, want Ada Lovelace", got)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, err := benchmarkDecodeDynamicObjectImage(image, typ, dynamicType, factory)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkADTResult = value.Interface()
	}
}

func benchmarkADTObjectType() *datatype.ObjectType {
	return &datatype.ObjectType{
		Name: "BENCHMARK_ORDER",
		Attributes: map[string]datatype.ObjectAttribute{
			"ID":            {Name: "ID", Sequence: 1, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
			"CUSTOMER_NAME": {Name: "CUSTOMER_NAME", Sequence: 2, ObjectType: &datatype.ObjectType{ElementType: common.DtyVCS}},
			"STATUS":        {Name: "STATUS", Sequence: 3, ObjectType: &datatype.ObjectType{ElementType: common.DtyVCS}},
			"TOTAL_AMOUNT":  {Name: "TOTAL_AMOUNT", Sequence: 4, ObjectType: &datatype.ObjectType{ElementType: common.DtyNum}},
		},
	}
}

func benchmarkADTDynamicType() reflect.Type {
	return reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeFor[int64](), Tag: `oracle:"ID"`},
		{Name: "CUSTOMER_NAME", Type: reflect.TypeFor[string](), Tag: `oracle:"CUSTOMER_NAME"`},
		{Name: "STATUS", Type: reflect.TypeFor[string](), Tag: `oracle:"STATUS"`},
		{Name: "TOTAL_AMOUNT", Type: reflect.TypeFor[int64](), Tag: `oracle:"TOTAL_AMOUNT"`},
	})
}

func newBenchmarkMapObject(typ *datatype.ObjectType) (*datatype.Object, error) {
	object, err := typ.NewObject()
	if err != nil {
		return nil, err
	}
	if err = object.Set("ID", int64(42)); err != nil {
		return nil, err
	}
	if err = object.Set("CUSTOMER_NAME", "Ada Lovelace"); err != nil {
		return nil, err
	}
	if err = object.Set("STATUS", "APPROVED"); err != nil {
		return nil, err
	}
	if err = object.Set("TOTAL_AMOUNT", int64(12500)); err != nil {
		return nil, err
	}
	return object, nil
}

func newBenchmarkDynamicValue(typ reflect.Type) reflect.Value {
	value := reflect.New(typ).Elem()
	value.Field(0).SetInt(42)
	value.Field(1).SetString("Ada Lovelace")
	value.Field(2).SetString("APPROVED")
	value.Field(3).SetInt(12500)
	return value
}

// benchmarkEncodeDynamicObjectImage is a prototype direct dynamic-object
// encoder. It intentionally reads reflected fields in descriptor order and
// writes their scalar TTC values without creating datatype.Object or its
// Attributes map.
func benchmarkEncodeDynamicObjectImage(value reflect.Value, typ *datatype.ObjectType, factory codecFactory) (driverCommon.B1Array, error) {
	if typ == nil || !typ.IsObject() || factory == nil {
		return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
	}
	data := []byte{0x84, 1, 254, 0, 0, 0, 0}
	for index, name := range typ.AttributeNames() {
		attributeType := typ.Attributes[name].ObjectType
		if attributeType == nil || attributeType.ElementType == 0 {
			return nil, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		fieldValue := value.Field(index).Interface()
		encoder, err := factory.getCollectionEncoder(attributeType.ElementType)
		if err != nil {
			encoder, err = factory.getEncoder(normalizeBindValue(fieldValue))
			if err != nil {
				return nil, err
			}
		}
		encoded, err := encoder(fieldValue)
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

// benchmarkDecodeDynamicObjectImage is the matching prototype direct decoder.
// It assigns scalar decoder values directly into reflected fields rather than
// storing them in datatype.Object.Attributes first.
func benchmarkDecodeDynamicObjectImage(data driverCommon.B1Array, typ *datatype.ObjectType, dynamicType reflect.Type, factory codecFactory) (reflect.Value, error) {
	if typ == nil || !typ.IsObject() || factory == nil || len(data) < 3 || data[0]&0x80 == 0 {
		return reflect.Value{}, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	imageLength, pos, err := readPickleLength(data, 2)
	if err != nil || imageLength != len(data) {
		return reflect.Value{}, common.NewOracleError(oracleErrors.ADTEncodingError, err)
	}
	value := reflect.New(dynamicType).Elem()
	for index, name := range typ.AttributeNames() {
		attributeType := typ.Attributes[name].ObjectType
		if attributeType == nil || attributeType.ElementType == 0 || pos >= len(data) {
			return reflect.Value{}, common.NewOracleError(oracleErrors.ADTMetadataError, nil)
		}
		if data[pos] == 255 {
			pos++
			continue
		}
		length, next, err := readPickleLength(data, pos)
		if err != nil || length < 0 || next+length > len(data) {
			return reflect.Value{}, common.NewOracleError(oracleErrors.ADTEncodingError, err)
		}
		decoder, err := factory.getDecoder(attributeType.ElementType)
		if err != nil {
			return reflect.Value{}, err
		}
		decoded, err := decoder.decodeToType(columnContext{DataType: attributeType.ElementType}, data[next:next+length])
		if err != nil {
			return reflect.Value{}, err
		}
		value.Field(index).Set(reflect.ValueOf(decoded))
		pos = next + length
	}
	if pos != len(data) {
		return reflect.Value{}, common.NewOracleError(oracleErrors.ADTEncodingError, nil)
	}
	return value, nil
}
