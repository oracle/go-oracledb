package ttc

import (
	"bytes"
	"context"
	"database/sql/driver"
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
)

func TestPrepareVectorBinds(t *testing.T) {
	args := []driver.Value{
		driverCommon.VectorFloat64{1.25, -2.5},
		driverCommon.VectorFloat32{3.5, 4.25},
		driverCommon.VectorInt8{1, -2, 3},
		driverCommon.VectorBinary{0xAA, 0x0F},
	}
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterCodecFactory(NewCodecFactoryForProtocol(20))
	rows, _, oacs, err := prepareBindDataForTest(&statementProcessor{shelf: shelf}, args)
	if err != nil {
		t.Fatalf("prepare vector binds: %v", err)
	}
	if len(rows) != 1 || len(rows[0]) != len(args) || len(oacs) != len(args) {
		t.Fatalf("unexpected bind shape: rows=%d columns=%d oacs=%d", len(rows), len(rows[0]), len(oacs))
	}
	for i, payload := range rows[0] {
		if len(payload) == 0 {
			t.Fatalf("VECTOR payload %d is empty", i)
		}
		if got := oacs[i].(*tTIoac).dataType; got != driverCommon.UB1(DtyVec) {
			t.Fatalf("VECTOR OAC %d datatype=%d, want %d", i, got, DtyVec)
		}
	}
}

func TestWrapVectorColumn(t *testing.T) {
	payload := driverCommon.B1Array{0xAA, 0xBB, 0xCC}
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 128)
	if err := wrapVectorColumn(context.Background(), writer, payload); err != nil {
		t.Fatalf("wrap vector: %v", err)
	}
	reader := createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0)
	locatorLen, err := reader.UnmarshalUB4(context.Background())
	if err != nil {
		t.Fatalf("read locator length: %v", err)
	}
	wantLocator := buildVectorQuasiLocator(len(payload))
	if int(locatorLen) != len(wantLocator) {
		t.Fatalf("locator length=%d, want %d", locatorLen, len(wantLocator))
	}
	locator, _, err := reader.UnmarshalCLRColumnData(context.Background())
	if err != nil || !bytes.Equal(locator, wantLocator) {
		t.Fatalf("locator mismatch: %v, err=%v", locator, err)
	}
	value, _, err := reader.UnmarshalCLRColumnData(context.Background())
	if err != nil || !bytes.Equal(value, payload) {
		t.Fatalf("payload mismatch: %v, err=%v", value, err)
	}
}

func TestUnwrapVectorColumn(t *testing.T) {
	ctx := context.Background()
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 256)
	locator := driverCommon.B1Array(bytes.Repeat([]byte{0x5A}, 9))
	prefetched := driverCommon.B1Array{1, 2, 3}
	if err := writer.MarshalUB4(ctx, driverCommon.UB4(len(locator))); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalUB8(ctx, driverCommon.UB8(len(prefetched))); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalUB4(ctx, driverCommon.UB4(len(prefetched))); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalCLR(ctx, prefetched, 0, len(prefetched)); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalCLR(ctx, locator, 0, len(locator)); err != nil {
		t.Fatal(err)
	}
	payload, err := unwrapVectorColumn(ctx, createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0), columnUnwrapContext{})
	if err != nil {
		t.Fatalf("unwrap vector column: %v", err)
	}
	if got, ok := payload.data.(driverCommon.B1Array); !ok || !bytes.Equal(got, prefetched) {
		t.Fatalf("prefetched data=%v, want %v", payload.data, prefetched)
	}
}

func TestVectorDecoderAndDefine(t *testing.T) {
	payload, err := converters.EncodeVectorFloat32(driverCommon.VectorFloat32{1.5, -2.5})
	if err != nil {
		t.Fatalf("encode vector: %v", err)
	}
	value, err := DecodeVectorColumn(columnContext{}, payload)
	if err != nil {
		t.Fatalf("decode vector column: %v", err)
	}
	if got := value.([]float32); !reflect.DeepEqual(got, []float32{1.5, -2.5}) {
		t.Fatalf("decoded vector=%v", got)
	}
	oac := newTTIOacVectorDefine(columnContext{}, 1).(*tTIoac)
	if oac.dataType != driverCommon.UB1(DtyVec) || oac.codepointLengthLimit != vectorPrefetchSize {
		t.Fatalf("VECTOR define OAC=%+v", oac)
	}
}

func TestCheckNamedValueAcceptsVector(t *testing.T) {
	if err := checkNamedValue(&driver.NamedValue{Ordinal: 1, Value: driverCommon.VectorFloat64{1}}); err != nil {
		t.Fatalf("VECTOR bind was rejected: %v", err)
	}
	if err := checkNamedValue(&driver.NamedValue{Ordinal: 1, Value: "ordinary"}); err != driver.ErrSkip {
		t.Fatalf("ordinary bind error=%v, want ErrSkip", err)
	}
}
