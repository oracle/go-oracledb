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
	"errors"
	"fmt"
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestColumnUnwrapSelection verifies that decoder version selection also
// selects the wire reader, with CLR fallback for unavailable readers.
func TestColumnUnwrapSelection(t *testing.T) {
	t.Parallel()
	registry := newCodecRegistry[DtyType, *typeDecoder]()
	first := func(context.Context, driverCommon.Marshaller, columnUnwrapContext) (columnPayload, error) {
		return columnPayload{data: "first"}, nil
	}
	second := func(context.Context, driverCommon.Marshaller, columnUnwrapContext) (columnPayload, error) {
		return columnPayload{data: "second"}, nil
	}
	firstDecoder := newTypeDecoder(nil, nil, first)
	secondDecoder := newTypeDecoder(nil, nil, second)
	_ = registry.Register(DtyCur, 12, firstDecoder)
	_ = registry.Register(DtyCur, 20, firstDecoder)
	_ = registry.Register(DtyCur, 20, secondDecoder)
	_ = registry.Register(DtyClob, 12, newTypeDecoder(nil, nil, nil))
	_ = registry.Register(DtyBlob, 12, nil)
	for _, tc := range []struct {
		version int8
		want    string
		decoder *typeDecoder
	}{
		{12, "first", firstDecoder}, {19, "first", firstDecoder}, {20, "second", secondDecoder}, {24, "second", secondDecoder}, {-1, "second", secondDecoder},
	} {
		factory := &CodecFactoryImpl{ttcVersion: tc.version, decoders: registry}
		decoder, err := factory.getDecoder(DtyCur)
		if err != nil || decoder != tc.decoder {
			t.Fatalf("decoder version %d: %v %v", tc.version, decoder, err)
		}
		rxd := newTTIrxd().(*tTIrxd)
		rxd.setCodecFactory(factory)
		rxd.setColumnContexts([]columnContext{{DataType: DtyCur}})
		rxd.row = make([]columnPayload, 1)
		if err := rxd._unmarshalColumn(context.Background(), DtyCur, nil, 0); err != nil {
			t.Fatal(err)
		}
		if rxd.row[0].data != tc.want {
			t.Fatalf("reader version %d: %#v", tc.version, rxd.row[0])
		}
	}
	for _, tc := range []struct {
		name    string
		version int8
		dty     DtyType
	}{
		{"unregistered", 12, DtyVCS}, {"incompatible", 11, DtyCur}, {"nil-reader", 12, DtyClob}, {"nil-decoder", 12, DtyBlob},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory := &CodecFactoryImpl{ttcVersion: tc.version, decoders: registry}
			rxd := newTTIrxd().(*tTIrxd)
			rxd.setCodecFactory(factory)
			rxd.setColumnContexts([]columnContext{{DataType: tc.dty}})
			rxd.row = make([]columnPayload, 1)
			if err := rxd._unmarshalColumn(context.Background(), tc.dty, createMarshaller([]byte{1, 42}, 0, 0), 0); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rxd.row[0].data, driverCommon.B1Array{42}) {
				t.Fatalf("CLR fallback: %#v", rxd.row[0])
			}
		})
	}
}

// TestColumnDecoderRegistrations checks every built-in decoder version has an
// explicit wire reader and that only CLOB, BLOB and JSON use specialized framing.
func TestColumnDecoderRegistrations(t *testing.T) {
	t.Parallel()
	specialized := map[DtyType]map[int8]columnUnwrapFunc{
		DtyClob: {MinTTCProtocolVersion: unwrapClobColumn},
		DtyBlob: {MinTTCProtocolVersion: unwrapBlobColumn},
		DtyJSON: {MinTTCProtocolVersion: unwrapJSONColumn},
		DtyCur: {
			MinTTCProtocolVersion: unwrapRefCursorColumn12,
			17:                    unwrapRefCursorColumn17,
			20:                    unwrapRefCursorColumn20,
			24:                    unwrapRefCursorColumn24,
		},
	}
	for dty, entries := range DecoderRegistry.entries {
		for _, entry := range entries {
			expected := unwrapCLRColumn
			if handler, ok := specialized[dty][entry.fromTTCProtocolVersion]; ok {
				expected = handler
			}
			decoder := entry.makeFunc
			if decoder == nil || decoder.unwrapColumn == nil {
				t.Fatalf("datatype %d version %d has no reader", dty, entry.fromTTCProtocolVersion)
			}
			if reflect.ValueOf(decoder.unwrapColumn).Pointer() != reflect.ValueOf(expected).Pointer() {
				t.Fatalf("datatype %d version %d has wrong reader", dty, entry.fromTTCProtocolVersion)
			}
		}
	}
	for dty := range specialized {
		if len(DecoderRegistry.getCandidates(dty)) == 0 {
			t.Fatalf("missing specialized decoder %d", dty)
		}
	}
}

// TestColumnUnwrapWire checks real wire consumption, including a following
// column, then truncates each fixture at every byte to exercise read failures.
func TestColumnUnwrapWire(t *testing.T) {
	t.Parallel()
	blob := []byte{1, 1, 1, 1, 1, 1, 1, 42, 1, 7}
	clob := []byte{1, 1, 1, 1, 1, 1, 1, 2, 3, 105, 1, 1, 42, 1, 7}
	clobDB := []byte{1, 1, 1, 1, 1, 1, 0, 1, 1, 42, 1, 7}
	clobN := []byte{1, 1, 1, 1, 1, 1, 0, 2, 1, 42, 1, 7}
	env := columnUnwrapContext{index: 2, sessCharSet: 873, sessNCharSet: 2000}
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	for _, tc := range []struct {
		name    string
		dty     DtyType
		wire    []byte
		want    any
		lob     bool
		charset driverCommon.UB2
	}{
		{"scalar", DtyChr, []byte{1, 42}, driverCommon.B1Array{42}, false, 0},
		{"scalar-null", DtyChr, []byte{0}, nil, false, 0},
		{"clob", DtyClob, clob, driverCommon.B1Array{42}, true, 873},
		{"clob-db-charset", DtyClob, clobDB, driverCommon.B1Array{42}, true, 873},
		{"nclob", DtyClob, clobN, driverCommon.B1Array{42}, true, 2000},
		{"clob-null", DtyClob, []byte{0}, nil, true, 0},
		{"blob", DtyBlob, blob, driverCommon.B1Array{42}, true, 0},
		{"blob-null", DtyBlob, []byte{0}, nil, true, 0},
		{"json", DtyJSON, []byte{1, 1, 1, 1, 1, 1, 1, 42, 1, 7}, driverCommon.B1Array{42}, true, 0},
		{"json-null", DtyJSON, []byte{0}, nil, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder, err := factory.getDecoder(tc.dty)
			if err != nil {
				t.Fatal(err)
			}
			handler := decoder.unwrapColumn
			mar := createMarshaller(append(append([]byte{}, tc.wire...), 0x55), 0, 0)
			got, err := handler(context.Background(), mar, env)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.data, tc.want) || (got.lob != nil) != tc.lob {
				t.Fatalf("payload: %#v", got)
			}
			if got.lob != nil && got.lob.CharsetID != tc.charset {
				t.Fatalf("charset: %d", got.lob.CharsetID)
			}
			if got.lob != nil && got.data != nil && !reflect.DeepEqual(got.lob.LobLocator, driverCommon.B1Array{7}) {
				t.Fatalf("locator: %v", got.lob.LobLocator)
			}
			next, err := mar.UnmarshalUB1(context.Background())
			if err != nil || next != 0x55 {
				t.Fatalf("next column corrupted: %d %v", next, err)
			}
			for end := 0; end < len(tc.wire); end++ {
				t.Run(fmt.Sprintf("truncated-%d", end), func(t *testing.T) {
					got, err := handler(context.Background(), createMarshaller(tc.wire[:end], 0, 0), env)
					if err == nil {
						t.Fatalf("accepted truncated wire: %#v", got)
					}
					if got.data != nil || got.lob != nil {
						t.Fatalf("published partial payload: %#v", got)
					}
				})
			}
		})
	}
}

// TestColumnUnwrapDispatch verifies that the dispatcher publishes only a
// successfully read payload, and that PL/SQL indicators remain outside handlers.
func TestColumnUnwrapDispatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	factory := NewCodecFactoryForProtocol(MinTTCProtocolVersion)
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setCodecFactory(factory)
	rxd.row = make([]columnPayload, 1)
	rxd.setColumnContexts([]columnContext{{DataType: DtyVCS}})
	if err := rxd._unmarshalColumn(ctx, DtyVCS, createMarshaller([]byte{1, 42}, 0, 0), 0); err != nil {
		t.Fatal(err)
	}
	before := rxd.row[0]
	if err := rxd._unmarshalColumn(ctx, DtyVCS, createMarshaller(nil, 0, 0), 0); err == nil {
		t.Fatal("expected read error")
	}
	if !reflect.DeepEqual(before, rxd.row[0]) {
		t.Fatal("failed handler changed row")
	}
	rxd.setCodecFactory(&CodecFactoryImpl{ttcVersion: 0, decoders: DecoderRegistry})
	if err := rxd._unmarshalColumn(ctx, DtyClob, createMarshaller([]byte{1, 43}, 0, 0), 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rxd.row[0].data, driverCommon.B1Array{43}) {
		t.Fatalf("expected CLR fallback: %#v", rxd.row[0])
	}
	rxd.setCodecFactory(factory)
	rxd.setNumberofReturningArgs(1)
	rxd.setColumnContexts([]columnContext{{DataType: DtyJSON}})
	// Native JSON has no inner indicators; only the outer PL/SQL indicator follows.
	mar := createMarshaller([]byte{0, 0, 0x55}, 0, 0)
	if err := rxd.UnmarshalFrom(ctx, mar); err != nil {
		t.Fatal(err)
	}
	if len(rxd.row) != 1 || rxd.row[0].data != nil || rxd.row[0].lob == nil {
		t.Fatalf("NULL JSON: %#v", rxd.row)
	}
	if next, err := mar.UnmarshalUB1(ctx); err != nil || next != 0x55 {
		t.Fatalf("next field corrupted: %d %v", next, err)
	}
}

// TestColumnPayloadConstructedValues follows a constructed cursor through RXD,
// row buffering, BVC carry, Rows.Next and OUT assignment without a byte decoder.
func TestColumnPayloadConstructedValues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cursor := newTTCRows(nil)
	registry := newCodecRegistry[DtyType, *typeDecoder]()
	reads := 0
	_ = registry.Register(DtyCur, 12, newTypeDecoder(nil, nil, func(context.Context, driverCommon.Marshaller, columnUnwrapContext) (columnPayload, error) {
		reads++
		return columnPayload{data: cursor}, nil
	}))
	factory := &CodecFactoryImpl{ttcVersion: 12, decoders: registry}
	shelf := newShelf[driverCommon.MessageType]().RegisterCodecFactory(factory)
	columns := []columnContext{{DataType: DtyCur}}
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setCodecFactory(factory)
	rxd.setColumnContexts(columns)
	rxd.setNumberOfColumns(1)
	if err := rxd.UnmarshalFrom(ctx, createMarshaller(nil, 0, 0)); err != nil {
		t.Fatal(err)
	}
	rows := newTTCRows(columns)
	rows.SetShelf(shelf)
	state := &queryRunState{rows: rows}
	state.handleRXDRow(rxd)
	rxd.setPrevRow(state.prevRow)
	rxd.setBvcState(nil, true)
	// A carried cursor must not call a reader or clone the cursor itself.
	if err := rxd.UnmarshalFrom(ctx, createMarshaller(nil, 0, 0)); err != nil {
		t.Fatal(err)
	}
	state.handleRXDRow(rxd)
	if reads != 1 {
		t.Fatalf("BVC re-read cursor: %d reads", reads)
	}
	rows.numOfRows = 2
	for i := 0; i < 2; i++ {
		dest := make([]driver.Value, 1)
		if err := rows.Next(dest); err != nil {
			t.Fatal(err)
		}
		if dest[0] != cursor {
			t.Fatalf("cursor identity lost: %T", dest[0])
		}
	}
	var out driver.Rows
	exec := &statementExecutorExec{statementProcessor: statementProcessor{shelf: shelf}, outDestPtrs: []any{&out}, outColumnContexts: columns}
	if err := exec.handleRXDRow(rxd); err != nil {
		t.Fatal(err)
	}
	if out != cursor {
		t.Fatal("OUT cursor identity lost")
	}
	wire := driverCommon.B1Array{42}
	locator := driverCommon.B1Array{9}
	lob := &lobColumnContext{CharsetID: 873, LobLocator: locator}
	copy := (columnPayload{data: wire, lob: lob}).clone()
	wire[0] = 7
	locator[0] = 8
	if copy.data.(driverCommon.B1Array)[0] != 42 || copy.lob == lob || copy.lob.LobLocator[0] != 9 {
		t.Fatal("row copy did not detach value and locator bytes")
	}
	if (columnPayload{}).clone().data != nil {
		t.Fatal("NULL clone changed")
	}
}

// TestTTIrxdTracksRefCursorRows verifies that RXD retains every constructed
// REF CURSOR until its statement executor registers the cursors on the shelf.
func TestTTIrxdTracksRefCursorRows(t *testing.T) {
	t.Parallel()
	cursor := newRefCursorResultRows(newTTCRows(nil), 42)
	registry := newCodecRegistry[DtyType, *typeDecoder]()
	_ = registry.Register(DtyCur, MinTTCProtocolVersion, newTypeDecoder(nil, nil,
		func(context.Context, driverCommon.Marshaller, columnUnwrapContext) (columnPayload, error) {
			return columnPayload{data: cursor}, nil
		}))
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setCodecFactory(&CodecFactoryImpl{ttcVersion: MinTTCProtocolVersion, decoders: registry})
	rxd.setColumnContexts([]columnContext{{DataType: DtyCur}})
	rxd.row = make([]columnPayload, 1)
	if err := rxd._unmarshalColumn(context.Background(), DtyCur, nil, 0); err != nil {
		t.Fatalf("unmarshal REF CURSOR column: %v", err)
	}
	if len(rxd.refCursorRows) != 1 || rxd.refCursorRows[0] != cursor {
		t.Fatalf("tracked REF CURSOR rows = %#v, want %#v", rxd.refCursorRows, cursor)
	}
	shelf := newShelf[driverCommon.MessageType]()
	for _, row := range rxd.getRefCursorRows() {
		shelf.addRefCursor(row)
	}
	if !shelf.hasRefCursor(cursor) {
		t.Fatal("REF CURSOR was not registered on the receiving shelf")
	}
}

// TestColumnPayloadDecode exercises raw decoding, constructed values, NULL,
// decoder lookup errors, and decoder errors at the OUT conversion boundary.
func TestColumnPayloadDecode(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("decoder failure")
	registry := newCodecRegistry[DtyType, *typeDecoder]()
	_ = registry.Register(DtyVCS, 12, newTypeDecoder(func(columnContext, any) (driver.Value, error) { return nil, sentinel }, nil, unwrapCLRColumn))
	factory := &CodecFactoryImpl{ttcVersion: 12, decoders: registry}
	if _, err := decodeOutColumn(factory, columnContext{DataType: DtyNum}, columnPayload{data: driverCommon.B1Array{1}}); err == nil {
		t.Fatal("expected lookup error")
	}
	if _, err := decodeOutColumn(factory, columnContext{DataType: DtyVCS}, columnPayload{data: driverCommon.B1Array{1}}); !errors.Is(err, sentinel) {
		t.Fatalf("decode error: %v", err)
	}
	if value, err := decodeOutColumn(factory, columnContext{}, columnPayload{}); value != nil || err != nil {
		t.Fatalf("NULL: %v %v", value, err)
	}
	shelf := newShelf[driverCommon.MessageType]().RegisterCodecFactory(&testCodecFactory{decode: "decoded"})
	rows := newTTCRows([]columnContext{{DataType: DtyVCS}})
	rows.SetShelf(shelf)
	rows.rowData = [][]columnPayload{{{data: driverCommon.B1Array{42}}}, {{}}}
	value, err := rows.decodeColumnPayload(rows.rowData[0], 0)
	if err != nil || value != "decoded" {
		t.Fatalf("raw: %v %v", value, err)
	}
	shelf.RegisterCodecFactory(factory)
	if _, err = rows.decodeColumnPayload(rows.rowData[0], 0); err == nil {
		t.Fatal("expected row decoder error")
	}
	rows.columnContexts[0].DataType = DtyNum
	value, err = rows.decodeColumnPayload(rows.rowData[0], 0)
	if err != nil || !reflect.DeepEqual(value, driverCommon.B1Array{42}) {
		t.Fatalf("unknown decoder fallback: %v %v", value, err)
	}
	value, err = rows.decodeColumnPayload(rows.rowData[1], 0)
	if err != nil || value != nil {
		t.Fatalf("NULL: %v %v", value, err)
	}
}

// TestColumnUnwrapExecutors verifies that every RXD setup path uses the
// codec factory supplied during message creation.
func TestColumnUnwrapExecutors(t *testing.T) {
	t.Parallel()
	if rxd := newTTIrxd().(*tTIrxd); rxd.codecFactory != nil {
		t.Fatal("constructor configured a codec factory")
	}
	shelf, _, _ := newExecTestShelf(128)
	registry := newCodecRegistry[DtyType, *typeDecoder]()
	_ = registry.Register(DtyCur, 20, newTypeDecoder(nil, nil, func(context.Context, driverCommon.Marshaller, columnUnwrapContext) (columnPayload, error) {
		return columnPayload{data: "v20"}, nil
	}))
	messageFactory := NewMessageFactoryForProtocol(20, nil).(*SimpleFactory)
	messageFactory.codecFactory.(*CodecFactoryImpl).decoders = registry
	shelf.RegisterMessageFactory(messageFactory)
	processor := statementProcessor{shelf: shelf, sessCtx: driverCommon.NewSessionContext()}
	exec := statementExecutorExec{statementProcessor: processor, outDestPtrs: []any{new(string)}, outColumnContexts: []columnContext{{DataType: DtyCur}}}
	plsql := &statementExecutorPlSql{statementExecutorExec: exec}
	dml := &statementExecutorDML{statementExecutorExec: exec}
	query := &statementExecutorSelect{statementProcessor: processor, resultMetadata: selectResultMetadata{columns: exec.outColumnContexts}}
	create := []func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error){plsql.createRXD, dml.createRXD, func(h *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return query.createRXD(&queryRunState{}, h)
	}}
	for _, makeRXD := range create {
		msg, err := makeRXD(nil)
		if err != nil {
			t.Fatal(err)
		}
		rxd := msg.(*tTIrxd)
		if rxd.codecFactory != messageFactory.codecFactory {
			t.Fatal("message factory did not inject its codec factory")
		}
		rxd.row = make([]columnPayload, 1)
		rxd.setColumnContexts(exec.outColumnContexts)
		if err := rxd._unmarshalColumn(context.Background(), DtyCur, nil, 0); err != nil {
			t.Fatal(err)
		}
		if rxd.row[0].data != "v20" {
			t.Fatalf("negotiated reader: %#v", rxd.row[0])
		}
		// RXD retains its injected dependency when the shelf's factory changes.
		shelf.RegisterCodecFactory(&CodecFactoryImpl{ttcVersion: 12, decoders: registry})
		if err := rxd._unmarshalColumn(context.Background(), DtyCur, nil, 0); err != nil {
			t.Fatal(err)
		}
		if rxd.row[0].data != "v20" {
			t.Fatal("RXD did not retain its injected codec factory")
		}
	}
	shelf.RegisterMessageFactory(&SimpleFactory{msgregistry: NewRegistry[driverCommon.MessageType]()})
	for _, makeRXD := range create {
		if _, err := makeRXD(nil); err == nil {
			t.Fatal("expected message factory error")
		}
	}
}

// TestColumnUnwrapPLSQLFailures checks the row-level indicator boundary for
// fresh and BVC rows; indicator bytes must not be consumed by scalar handlers.
func TestColumnUnwrapPLSQLFailures(t *testing.T) {
	t.Parallel()
	for _, bvc := range []bool{false, true} {
		for _, wire := range [][]byte{{1, 42}, {1, 42, 0}} {
			rxd := newTTIrxd().(*tTIrxd)
			rxd.setCodecFactory(NewCodecFactoryForProtocol(MinTTCProtocolVersion))
			rxd.setNumberofReturningArgs(1)
			rxd.setColumnContexts([]columnContext{{DataType: DtyVCS}})
			rxd.setPrevRow([]columnPayload{{}})
			bits := driverCommon.NewBitSet(1)
			bits.SetBytes(0, []byte{1})
			rxd.setBvcState(bits, bvc)
			err := rxd.UnmarshalFrom(context.Background(), createMarshaller(wire, 0, 0))
			if (err != nil) != (len(wire) == 2) {
				t.Fatalf("BVC %t wire %v: %v", bvc, wire, err)
			}
		}
	}
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setNumberOfColumns(1)
	if err := rxd.UnmarshalFrom(context.Background(), createMarshaller(nil, 0, 0)); err == nil {
		t.Fatal("expected column context mismatch")
	}
}
