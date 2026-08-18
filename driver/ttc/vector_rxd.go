package ttc

import (
	"context"
	"fmt"
	"math"

	"github.com/oracle/go-driver/driver/common"
)

// _unmarshalVectorColumn decodes a fully prefetched VECTOR column.
func (rxd *tTIrxd) _unmarshalVectorColumn(ctx context.Context, mar common.Marshaller, col int) error {
	locatorLength, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector locator length: %w", err)
	}
	if locatorLength == 0 {
		rxd.row[col] = nil
		return nil
	}

	// Prefetch envelope: locator length, total length, prefetch chunk size,
	// prefetched VECTOR bytes, then locator bytes.
	totalLen, err := mar.UnmarshalUB8(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector prefetch length: %w", err)
	}
	if _, err = mar.UnmarshalUB4(ctx); err != nil {
		return fmt.Errorf("ttc: failed to read vector prefetch chunk size: %w", err)
	}

	prefetchedData, _, err := mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector prefetched data: %w", err)
	}
	_, _, err = mar.UnmarshalCLRColumnData(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector locator payload: %w", err)
	}

	if totalLen > common.UB8(math.MaxInt) {
		return fmt.Errorf("ttc: vector length %d exceeds host capacity", totalLen)
	}
	required := int(totalLen)

	if len(prefetchedData) != required {
		return fmt.Errorf("ttc: incomplete vector prefetch: got %d bytes, expected %d", len(prefetchedData), required)
	}

	rxd.row[col] = append(common.B1Array(nil), prefetchedData...)
	common.Odl.Debug("VECTOR RXD: fully prefetched payload", "totalBytes", required)
	return nil
}
