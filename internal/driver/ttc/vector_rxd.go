package ttc

import (
	"context"
	"fmt"
	"math"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// _unmarshalVectorColumn reads the LOB-prefetch envelope used for VECTOR results.
// The fixed VECTOR prefetch size covers every supported dense VECTOR, so a complete
// envelope contains all vector bytes followed by a locator that is not retained.
func (rxd *tTIrxd) _unmarshalVectorColumn(ctx context.Context, mar driverCommon.Marshaller, col int) error {
	locatorLength, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector locator length: %w", err)
	}
	if locatorLength == 0 {
		rxd.row[col] = nil
		return nil
	}

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
	if _, _, err = mar.UnmarshalCLRColumnData(ctx); err != nil {
		return fmt.Errorf("ttc: failed to read vector locator: %w", err)
	}
	if totalLen > driverCommon.UB8(math.MaxInt) {
		return fmt.Errorf("ttc: vector length %d exceeds host capacity", totalLen)
	}
	if len(prefetchedData) != int(totalLen) {
		return fmt.Errorf("ttc: incomplete vector prefetch: got %d bytes, expected %d", len(prefetchedData), totalLen)
	}

	rxd.row[col] = append(driverCommon.B1Array(nil), prefetchedData...)
	common.Odl.Debug("VECTOR RXD: fully prefetched payload", "totalBytes", totalLen)
	return nil
}
