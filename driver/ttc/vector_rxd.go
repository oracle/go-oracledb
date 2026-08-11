package ttc

import (
	"context"
	"fmt"
	"math"

	"github.com/oracle/go-driver/driver/common"
)

// _unmarshalVectorColumn decodes a fully prefetched VECTOR column.
func (rxd *tTIrxd) _unmarshalVectorColumn(ctx context.Context, mar common.Marshaller, col int) error {
	first, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector leading length byte: %w", err)
	}

	if first > 4 {
		return fmt.Errorf("ttc: unsupported vector direct CLR payload")
	}

	locatorLength, err := decodeUniversalUB4FromPrefix(ctx, mar, first)
	if err != nil {
		return fmt.Errorf("ttc: failed to decode vector locator length: %w", err)
	}
	if locatorLength == 0 {
		rxd.row[col] = nil
		return nil
	}

	next, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		return fmt.Errorf("ttc: failed to read vector locator payload discriminator: %w", err)
	}

	if next > 8 {
		return fmt.Errorf("ttc: unsupported vector locator-only payload")
	}

	// Prefetch variant: UB4 locator length + UB8 total length + UB4 prefetch chunk +
	// CLR prefetched bytes + CLR locator bytes.
	totalLen, err := decodeUniversalUB8FromPrefix(ctx, mar, next)
	if err != nil {
		return fmt.Errorf("ttc: failed to decode vector prefetch length: %w", err)
	}
	if _, err = mar.UnmarshalUB4(ctx); err != nil {
		return fmt.Errorf("ttc: failed to read vector prefetch chunk size: %w", err)
	}

	prefetchedData, prefetchedLen, err := mar.UnmarshalCLRColumnData(ctx)
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
	if required < 0 {
		return fmt.Errorf("ttc: invalid vector length %d", totalLen)
	}

	if prefetchedLen != required || len(prefetchedData) != required {
		return fmt.Errorf("ttc: incomplete vector prefetch: got %d bytes, expected %d", prefetchedLen, required)
	}

	rxd.row[col] = append(common.B1Array(nil), prefetchedData...)
	common.Odl.Debug("VECTOR RXD: fully prefetched payload", "totalBytes", required)
	return nil
}

func decodeUniversalUB4FromPrefix(ctx context.Context, mar common.Marshaller, first common.UB1) (common.UB4, error) {
	if first == 0 {
		return 0, nil
	}
	if first > 4 {
		return 0, fmt.Errorf("invalid UB4 universal prefix %d", first)
	}

	bytes, err := mar.UnmarshalB1Array(ctx, int(first))
	if err != nil {
		return 0, err
	}
	var value common.UB4
	for _, b := range bytes {
		value = (value << 8) | common.UB4(b)
	}
	return value, nil
}

func decodeUniversalUB8FromPrefix(ctx context.Context, mar common.Marshaller, first common.UB1) (common.UB8, error) {
	if first == 0 {
		return 0, nil
	}
	if first > 8 {
		return 0, fmt.Errorf("invalid UB8 universal prefix %d", first)
	}

	bytes, err := mar.UnmarshalB1Array(ctx, int(first))
	if err != nil {
		return 0, err
	}
	var value common.UB8
	for _, b := range bytes {
		value = (value << 8) | common.UB8(b)
	}
	return value, nil
}
