package ttc

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
)

func TestPrepareBindsAndOAC_VectorBinds(t *testing.T) {
	args := []driver.Value{
		driverCommon.VectorFloat64{1.25, -2.5},
		driverCommon.VectorFloat32{3.5, 4.25},
		driverCommon.VectorInt8{1, -2, 3},
		driverCommon.VectorBinary{0xAA, 0x0F},
	}

	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterCodecFactory(NewCodecFactoryForProtocol(20))
	sp := &statementProcessor{shelf: shelf}
	if err := sp.prepareBindsAndOAC(args); err != nil {
		t.Fatalf("prepareBindsAndOAC: %v", err)
	}
	row := sp.encodedValues[0]
	if len(row) != len(args) || len(sp.currentOacs) != len(args) {
		t.Fatalf("unexpected bind/OAC counts: %d/%d", len(row), len(sp.currentOacs))
	}
	for i, bind := range row {
		if _, ok := bind.transport.(*vectorBindTransport); !ok {
			t.Fatalf("bind %d transport=%T, want *vectorBindTransport", i, bind.transport)
		}
		if bind.isNull || len(bind.payload) == 0 {
			t.Fatalf("bind %d unexpectedly empty", i)
		}
		if got := sp.currentOacs[i].(*tTIoac).dataType; got != driverCommon.UB1(DtyVec) {
			t.Fatalf("OAC %d datatype=%d, want %d", i, got, DtyVec)
		}
	}
}

func TestTTIrxdMarshalToVectorLocator(t *testing.T) {
	payload := driverCommon.B1Array{0xAA, 0xBB, 0xCC}
	bind, err := buildVectorBindValue(normalizedBindValue{}, payload)
	if err != nil {
		t.Fatalf("build vector bind: %v", err)
	}
	rxd := newTTIrxd().(*tTIrxd)
	rxd.setBindValues([]bindValue{bind})

	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 128)
	if err := rxd.MarshalTo(context.Background(), writer); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
	reader := createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0)
	locatorLen, err := reader.UnmarshalUB4(context.Background())
	if err != nil {
		t.Fatalf("read locator length: %v", err)
	}
	transport := bind.transport.(*vectorBindTransport)
	if int(locatorLen) != len(transport.locator) {
		t.Fatalf("locator length=%d, want %d", locatorLen, len(transport.locator))
	}
	locator, _, err := reader.UnmarshalCLRColumnData(context.Background())
	if err != nil || !bytes.Equal(locator, transport.locator) {
		t.Fatalf("locator mismatch: %v, err=%v", locator, err)
	}
	value, _, err := reader.UnmarshalCLRColumnData(context.Background())
	if err != nil || !bytes.Equal(value, payload) {
		t.Fatalf("payload mismatch: %v, err=%v", value, err)
	}
}

func TestVectorBindTransportNullAndErrors(t *testing.T) {
	ctx := context.Background()
	nullBind, err := buildVectorBindValue(normalizedBindValue{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 16)
	if err := nullBind.marshal(ctx, writer, TTIRXD, 0); err != nil {
		t.Fatalf("marshal null vector: %v", err)
	}
	if got := buf.bytes[:buf.currentWritePosition]; !bytes.Equal(got, []byte{0}) {
		t.Fatalf("null vector wire=%v", got)
	}
	if err := nullBind.marshal(ctx, failUB4Marshaller{writer}, TTIRXD, 0); err == nil {
		t.Fatal("expected null-vector marshal error")
	}

	bind, err := buildVectorBindValue(normalizedBindValue{}, driverCommon.B1Array{1})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		fail  FailOn
		count int
	}{
		{"locator length", failOnWriteBytes, 1},
		{"locator CLR", failOnWriteByte, 1},
		{"payload CLR", failOnWriteByte, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mar := createMarshaller(make([]byte, 128), tc.fail, tc.count)
			if err := bind.marshal(ctx, mar, TTIRXD, 0); err == nil {
				t.Fatal("expected marshal error")
			}
		})
	}
}

func TestBindValueUsesCLRTransportByDefault(t *testing.T) {
	bind := bindValue{payload: driverCommon.B1Array{0x42}}
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 16)
	if err := bind.marshal(context.Background(), writer, TTIRXD, 0); err != nil {
		t.Fatalf("default CLR marshal: %v", err)
	}
	if got := buf.bytes[:buf.currentWritePosition]; !bytes.Equal(got, []byte{1, 0x42}) {
		t.Fatalf("default CLR wire=%v", got)
	}
}

func TestVectorRXDErrorAndNullCases(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		write func(driverCommon.Marshaller) error
	}{
		{"null", func(m driverCommon.Marshaller) error { return m.MarshalUB4(ctx, 0) }},
		{"missing total length", func(m driverCommon.Marshaller) error { return m.MarshalUB4(ctx, 1) }},
		{"missing prefetch size", func(m driverCommon.Marshaller) error {
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			return m.MarshalUB8(ctx, 1)
		}},
		{"missing prefetch data", func(m driverCommon.Marshaller) error {
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			if err := m.MarshalUB8(ctx, 1); err != nil {
				return err
			}
			return m.MarshalUB4(ctx, 1)
		}},
		{"missing locator", func(m driverCommon.Marshaller) error {
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			if err := m.MarshalUB8(ctx, 1); err != nil {
				return err
			}
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			return m.MarshalCLR(ctx, driverCommon.B1Array{1}, 0, 1)
		}},
		{"incomplete prefetch", func(m driverCommon.Marshaller) error {
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			if err := m.MarshalUB8(ctx, 2); err != nil {
				return err
			}
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			if err := m.MarshalCLR(ctx, driverCommon.B1Array{1}, 0, 1); err != nil {
				return err
			}
			return m.MarshalCLR(ctx, driverCommon.B1Array{}, 0, 0)
		}},
		{"host overflow", func(m driverCommon.Marshaller) error {
			if err := m.MarshalUB4(ctx, 1); err != nil {
				return err
			}
			if err := m.MarshalUB8(ctx, ^driverCommon.UB8(0)); err != nil {
				return err
			}
			if err := m.MarshalUB4(ctx, 0); err != nil {
				return err
			}
			if err := m.MarshalCLR(ctx, driverCommon.B1Array{}, 0, 0); err != nil {
				return err
			}
			return m.MarshalCLR(ctx, driverCommon.B1Array{}, 0, 0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 32)
			if err := tc.write(writer); err != nil {
				t.Fatal(err)
			}
			rxd := newTTIrxd().(*tTIrxd)
			rxd.row = make([]driverCommon.B1Array, 1)
			err := rxd._unmarshalVectorColumn(ctx, createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0), 0)
			if tc.name == "null" {
				if err != nil || rxd.row[0] != nil {
					t.Fatalf("null vector: row=%v err=%v", rxd.row[0], err)
				}
			} else if err == nil {
				t.Fatal("expected unmarshal error")
			}
		})
	}
}

func TestUnmarshalColumnDispatchesVector(t *testing.T) {
	ctx := context.Background()
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 64)
	for _, value := range []driverCommon.UB4{1} {
		if err := writer.MarshalUB4(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.MarshalUB8(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalUB4(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalCLR(ctx, driverCommon.B1Array{7}, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.MarshalCLR(ctx, driverCommon.B1Array{}, 0, 0); err != nil {
		t.Fatal(err)
	}
	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = make([]driverCommon.B1Array, 1)
	if err := rxd._unmarshalColumn(ctx, DtyVec, createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0), 0); err != nil {
		t.Fatalf("VECTOR dispatch: %v", err)
	}
	if !bytes.Equal(rxd.row[0], []byte{7}) || len(rxd.lobColContext) != 1 {
		t.Fatalf("VECTOR dispatch state: row=%v lobContexts=%d", rxd.row, len(rxd.lobColContext))
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

func TestDecodeVectorColumnAndDefine(t *testing.T) {
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
	if _, err := DecodeVectorColumn(columnContext{}, driverCommon.B1Array{0}); err == nil {
		t.Fatal("expected wrapped VECTOR decode error")
	}
}

type failUB4Marshaller struct{ driverCommon.Marshaller }

func (failUB4Marshaller) MarshalUB4(context.Context, driverCommon.UB4) error {
	return errors.New("forced UB4 failure")
}
