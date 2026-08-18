package ttc

import (
	"bytes"
	"context"
	"testing"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/network/session"
)

// TestUnmarshalVectorColumn verifies that a complete VECTOR prefetch envelope
// is stored as the raw VECTOR payload for later type decoding.
func TestUnmarshalVectorColumn(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		write func(t *testing.T, mar *MarshalEngine)
		check func(t *testing.T, data common.B1Array)
	}{
		{
			name: "complete prefetch payload",
			write: func(t *testing.T, mar *MarshalEngine) {
				t.Helper()
				locator := common.B1Array(bytes.Repeat([]byte{0x5A}, 9))
				prefetched := common.B1Array{1, 2, 3}
				if err := mar.MarshalUB4(ctx, common.UB4(len(locator))); err != nil {
					t.Fatalf("marshal locator length: %v", err)
				}
				if err := mar.MarshalUB8(ctx, 3); err != nil {
					t.Fatalf("marshal vector length: %v", err)
				}
				if err := mar.MarshalUB4(ctx, 3); err != nil {
					t.Fatalf("marshal prefetch chunk size: %v", err)
				}
				if err := mar.MarshalCLR(ctx, prefetched, 0, len(prefetched)); err != nil {
					t.Fatalf("marshal prefetched data: %v", err)
				}
				if err := mar.MarshalCLR(ctx, locator, 0, len(locator)); err != nil {
					t.Fatalf("marshal locator: %v", err)
				}
			},
			check: func(t *testing.T, data common.B1Array) {
				t.Helper()
				if !bytes.Equal(data, []byte{1, 2, 3}) {
					t.Fatalf("prefetched data mismatch: got %v", data)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf, writer := NewMarshalEngineTest(session.BIG_ENDIAN, B2, Universal, 256)
			tc.write(t, writer)

			rxd := newTTIrxd().(*tTIrxd)
			rxd.row = make([]common.B1Array, 1)
			reader := createMarshaller(buf.bytes[:buf.currentWritePosition], 0, 0)
			if err := rxd._unmarshalVectorColumn(ctx, reader, 0); err != nil {
				t.Fatalf("unmarshal vector column: %v", err)
			}
			tc.check(t, rxd.row[0])
		})
	}
}
