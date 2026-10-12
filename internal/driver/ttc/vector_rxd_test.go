package ttc

import (
	"bytes"
	"context"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestUnmarshalVectorColumn(t *testing.T) {
	ctx := context.Background()
	buf, writer := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, B2, Universal, 256)
	locator := driverCommon.B1Array(bytes.Repeat([]byte{0x5A}, 9))
	prefetched := driverCommon.B1Array{1, 2, 3}
	if err := writer.MarshalUB4(ctx, driverCommon.UB4(len(locator))); err != nil {
		t.Fatalf("marshal locator length: %v", err)
	}
	if err := writer.MarshalUB8(ctx, driverCommon.UB8(len(prefetched))); err != nil {
		t.Fatalf("marshal vector length: %v", err)
	}
	if err := writer.MarshalUB4(ctx, driverCommon.UB4(len(prefetched))); err != nil {
		t.Fatalf("marshal prefetch chunk size: %v", err)
	}
	if err := writer.MarshalCLR(ctx, prefetched, 0, len(prefetched)); err != nil {
		t.Fatalf("marshal prefetched data: %v", err)
	}
	if err := writer.MarshalCLR(ctx, locator, 0, len(locator)); err != nil {
		t.Fatalf("marshal locator: %v", err)
	}

	rxd := newTTIrxd().(*tTIrxd)
	rxd.row = make([]driverCommon.B1Array, 1)
	reader := createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0)
	if err := rxd._unmarshalVectorColumn(ctx, reader, 0); err != nil {
		t.Fatalf("unmarshal vector column: %v", err)
	}
	if !bytes.Equal(rxd.row[0], prefetched) {
		t.Fatalf("prefetched data mismatch: got %v want %v", rxd.row[0], prefetched)
	}
}
