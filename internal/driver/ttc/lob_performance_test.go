/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** See the LICENSE file for license terms.
 */

package ttc

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"

	ttcLob "github.com/oracle/go-oracledb/v26/internal/driver/ttc/lob"
	internallob "github.com/oracle/go-oracledb/v26/internal/lob"
)

var lobBenchmarkByteSink byte

// BenchmarkLOBClobReaderStrategy compares the current string-backed reader to
// a byte-backed reader that would require materializing a []byte copy.
func BenchmarkLOBClobReaderStrategy(b *testing.B) {
	for _, size := range []int{4096, 65536} {
		value := internallob.BindClob(strings.Repeat("A", size))
		for _, useByteReader := range []bool{false, true} {
			name := "string-reader"
			if useByteReader {
				name = "byte-reader"
			}
			b.Run(fmt.Sprintf("%s/%dB", name, size), func(b *testing.B) {
				b.ReportAllocs()
				var reader interface{ Read([]byte) (int, error) }
				var firstByte [1]byte
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if useByteReader {
						reader = bytes.NewReader([]byte(value))
					} else {
						input, _ := normalizeLobBindValue(value)
						reader = input.Reader()
					}
					if _, err := reader.Read(firstByte[:]); err != nil {
						b.Fatal(err)
					}
					lobBenchmarkByteSink ^= firstByte[0]
				}
				runtime.KeepAlive(reader)
			})
		}
	}
}

// BenchmarkLOBBindNormalization records allocations while public LOB markers
// are converted to exact-size streaming inputs.
func BenchmarkLOBBindNormalization(b *testing.B) {
	for _, size := range []int{4096, 65536} {
		blob := make(internallob.BindBlob, size)
		clob := internallob.BindClob(strings.Repeat("A", size))
		nclob := internallob.BindNClob(strings.Repeat("A", size))
		for i := 0; i < size; i++ {
			blob[i] = 0x42
		}
		for _, test := range []struct {
			kind  string
			value any
		}{
			{kind: "BLOB", value: blob},
			{kind: "CLOB", value: clob},
			{kind: "NCLOB", value: nclob},
		} {
			b.Run(fmt.Sprintf("%s/%dB", test.kind, size), func(b *testing.B) {
				b.ReportAllocs()
				var input ttcLob.BindInput
				var firstByte [1]byte
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					input, _ = normalizeLobBindValue(test.value)
					if _, err := input.Reader().Read(firstByte[:]); err != nil {
						b.Fatal(err)
					}
					lobBenchmarkByteSink ^= firstByte[0]
				}
				runtime.KeepAlive(input)
			})
		}
	}
}

// BenchmarkLOBLocatorBindEncoding measures locator payload copying and OAC
// construction without TTC transport latency.
func BenchmarkLOBLocatorBindEncoding(b *testing.B) {
	bind := lobLocatorBind{
		locator: make([]byte, 86),
		kind:    internallob.BLOB,
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(bind.locator)))
	b.ResetTimer()
	var payload []byte
	var oac any
	for i := 0; i < b.N; i++ {
		var err error
		payload, oac, err = encodeLobLocatorBind(bind)
		if err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(payload)
	runtime.KeepAlive(oac)
}

type lobBenchmarkLocator struct {
	id   ttcLob.ReferenceID
	data []byte
}

func (locator *lobBenchmarkLocator) Snapshot() []byte {
	return append([]byte(nil), locator.data...)
}

func (*lobBenchmarkLocator) MarkReleased() {}

func (locator *lobBenchmarkLocator) ReferenceIdentity() (ttcLob.ReferenceID, error) {
	return locator.id, nil
}

// BenchmarkLOBFixedWidthUTF8Decode records the allocations and throughput of
// decoding database-character-set CLOB data into the caller's rune buffer.
func BenchmarkLOBFixedWidthUTF8Decode(b *testing.B) {
	for _, size := range []int{4096, 65536} {
		source := []byte(strings.Repeat("A🙂", size/5))
		output := make([]rune, len(source))
		executor := newClobExecutor(newShelf[driverCommon.MessageType]().Shelf, newTestSessionContext())
		b.Run(fmt.Sprintf("%dB", len(source)), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := executor.decodeFixedWidthCharSet(source, output, 0, false, false); err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(output)
		})
	}
}

// BenchmarkLOBLocatorBindCopyPath compares the bind-preparation plus RXD
// encoding path (two copies) with encoding a borrowed locator (one copy).
func BenchmarkLOBLocatorBindCopyPath(b *testing.B) {
	locator := make(driverCommon.B1Array, 86)
	for _, copyOnPreparation := range []bool{true, false} {
		name := "single-copy"
		if copyOnPreparation {
			name = "two-copies"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			var payload driverCommon.B1Array
			var oac any
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				prepared := locator
				if copyOnPreparation {
					prepared = append(driverCommon.B1Array(nil), locator...)
				}
				var err error
				payload, oac, err = encodeLobLocatorBind(lobLocatorBind{locator: prepared, kind: internallob.BLOB})
				if err != nil {
					b.Fatal(err)
				}
			}
			runtime.KeepAlive(payload)
			runtime.KeepAlive(oac)
		})
	}
}

// BenchmarkLOBManagerOpenCloseDispatch measures local temporary-BLOB open and
// close dispatch without TTC transport latency. The same benchmark source is
// run at the pre-refactor revision and after Phase 4.
func BenchmarkLOBManagerOpenCloseDispatch(b *testing.B) {
	manager, err := newLobManager(newShelf[driverCommon.MessageType](), newTestSessionContext())
	if err != nil {
		b.Fatal(err)
	}
	locatorBytes := newTestLocator(false)
	locatorBytes[koll1FlagOffset] = kolblBlobFlag
	locatorBytes[koll4FlagOffset] |= kolblTemporaryFlagByte
	loc := newLocator(locatorBytes, 1)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := manager.open(ctx, internallob.BLOB, loc, lobOpenModeReadWrite); err != nil {
			b.Fatal(err)
		}
		if err := manager.close(ctx, internallob.BLOB, loc); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLOBReferenceLifecycle measures local retain/release and cleanup
// batching without TTC network exchange costs.
func BenchmarkLOBReferenceLifecycle(b *testing.B) {
	registry := ttcLob.NewReferenceRegistry()
	locator := &lobBenchmarkLocator{
		id:   ttcLob.ReferenceID("benchmark-lob-id"),
		data: make([]byte, 86),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reference, err := registry.Retain(locator)
		if err != nil {
			b.Fatal(err)
		}
		if err := registry.Release(reference); err != nil {
			b.Fatal(err)
		}
		batch, err := registry.ReservePending()
		if err != nil {
			b.Fatal(err)
		}
		registry.CompletePending(batch)
	}
	runtime.KeepAlive(locator)
}
