/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

type resourceRetentionHeap struct {
	HeapAlloc   uint64
	HeapObjects uint64
}

type resourceRetentionResult struct {
	Completed        int
	Duration         time.Duration
	Baseline         resourceRetentionHeap
	Final            resourceRetentionHeap
	MemoryLimitBytes int64
}

func TestDriver_ResourceRetentionMemoryLeakStress(t *testing.T) {
	// Direct Go test discovery bypasses the category executor.
	if !resourceRetentionStressEnabled() {
		t.Skip("requires explicit -test.category=stress")
	}
	if TestingConfig == nil || TestingConfig.Stress == nil || TestingConfig.Stress.ResourceRetention == nil {
		t.Fatal("resource-retention stress requires stress.resource_retention in the selected JSON configuration")
	}
	config := *TestingConfig.Stress.ResourceRetention
	result, err := executeResourceRetention(config, func(ctx context.Context) (*sql.DB, error) {
		connector, err := openTestConnectorWithConfig(TestingConfig)
		if err != nil {
			return nil, err
		}
		// OpenDB is lazy. Warm-up below bounds physical connection creation with
		// the configured context, unlike the existing helper's unbounded Ping.
		return sql.OpenDB(connector), nil
	})
	// Delay reporting until after the final sample so these log strings do not
	// themselves become retained allocations in the measured workload.
	target := "unchanged"
	if config.MemoryLimitMiB != nil {
		target = fmt.Sprint(*config.MemoryLimitMiB)
	}
	t.Logf("configuration: workers=%d total_operations=%d max_open_connections=%d timeout=%s soft_go_memory_target_mib=%s effective_go_memory_limit_bytes=%d go=%s",
		config.Workers, config.TotalOperations, config.MaxOpenConnections, config.Timeout, target, result.MemoryLimitBytes, runtime.Version())
	t.Logf("result: requested=%d completed=%d workers=%d elapsed=%s soft_go_memory_target_mib=%s baseline_heap_alloc=%d final_heap_alloc=%d baseline_heap_objects=%d final_heap_objects=%d diagnostic_only=true",
		config.TotalOperations, result.Completed, config.Workers, result.Duration, target,
		result.Baseline.HeapAlloc, result.Final.HeapAlloc, result.Baseline.HeapObjects, result.Final.HeapObjects)
	if err != nil {
		t.Fatal(err)
	}
}

func resourceRetentionStressEnabled() bool {
	for _, category := range oracleTest.TestCategories {
		if strings.EqualFold(strings.TrimSpace(category), "stress") {
			return true
		}
	}
	return false
}

// executeResourceRetention owns the pool and the process-wide memory setting.
// It is deliberately not run in parallel with other category tests.
func executeResourceRetention(config oracleTest.ResourceRetentionConfig, openDB func(context.Context) (*sql.DB, error)) (result resourceRetentionResult, err error) {
	if err := config.Validate(); err != nil {
		return result, fmt.Errorf("stress.resource_retention: %w", err)
	}
	// This is a soft Go-runtime memory target, NOT an RSS cap, termination
	// mechanism, or leak detector. Leave the runtime's GC percentage unchanged.
	// A negative argument only reads the current setting. Baseline runs do not
	// override a pre-existing GOMEMLIMIT or install a new runtime target.
	result.MemoryLimitBytes = debug.SetMemoryLimit(-1)
	if config.MemoryLimitMiB != nil {
		result.MemoryLimitBytes = *config.MemoryLimitMiB * (1 << 20)
		previous := debug.SetMemoryLimit(result.MemoryLimitBytes)
		defer debug.SetMemoryLimit(previous)
	}

	timeout, _ := time.ParseDuration(config.Timeout) // Already validated.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	db, err := openDB(ctx)
	if err != nil {
		return result, fmt.Errorf("initialize database: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}() // Runs before the memory target is restored, also on panic.
	db.SetMaxOpenConns(config.MaxOpenConnections)
	db.SetMaxIdleConns(config.MaxOpenConnections)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if err := primeResourceRetentionPool(ctx, db, config.MaxOpenConnections); err != nil {
		return result, fmt.Errorf("warm up pool: %w", err)
	}

	// Compare the same initialized, still-open pool at both checkpoints.
	result.Baseline = captureResourceRetentionHeap()
	started := time.Now()
	result.Completed, err = runResourceRetentionWorkers(ctx, config.Workers, config.TotalOperations,
		func(ctx context.Context, worker, operation int) error {
			return executeSelectDual(ctx, db)
		})
	result.Duration = time.Since(started)
	result.Final = captureResourceRetentionHeap()
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil && result.Completed != config.TotalOperations {
		err = fmt.Errorf("incomplete workload: completed %d of %d", result.Completed, config.TotalOperations)
	}
	return result, err
}

func primeResourceRetentionPool(ctx context.Context, db *sql.DB, count int) (err error) {
	connections := make([]*sql.Conn, 0, count)
	defer func() {
		for index, conn := range connections {
			if closeErr := conn.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("return warm-up connection %d: %w", index+1, closeErr))
			}
		}
	}()
	// Hold all checkouts until all have been acquired. Returning each connection
	// immediately could prime the same physical connection repeatedly.
	for index := 0; index < count; index++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			return fmt.Errorf("acquire warm-up connection %d: %w", index+1, err)
		}
		connections = append(connections, conn)
		if err := executeSelectDual(ctx, conn); err != nil {
			return fmt.Errorf("query warm-up connection %d: %w", index+1, err)
		}
	}
	return nil
}

func resourceRetentionWorkerOperations(total, workers, worker int) int {
	count := total / workers
	if worker < total%workers {
		count++
	}
	return count
}

func runResourceRetentionWorkers(ctx context.Context, workers, total int, operation func(context.Context, int, int) error) (int, error) {
	if workers <= 0 || total < workers {
		return 0, fmt.Errorf("workload requires positive workers and at least one operation per worker")
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	completed := make([]int, workers) // Each worker writes only its own element.
	workerErrors := make([]error, workers)
	start := make(chan struct{})
	var ready, finished sync.WaitGroup
	ready.Add(workers)
	finished.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer finished.Done()
			defer func() {
				if value := recover(); value != nil {
					workerErrors[worker] = fmt.Errorf("worker %d panicked: %v", worker+1, value)
					cancel(workerErrors[worker])
				}
			}()
			ready.Done()
			<-start
			for index := 0; index < resourceRetentionWorkerOperations(total, workers, worker); index++ {
				if workCtx.Err() != nil {
					return
				}
				if err := operation(workCtx, worker, index); err != nil {
					// The first cause wins; successful operations alone are counted.
					workerErrors[worker] = fmt.Errorf("worker %d operation %d: %w", worker+1, index+1, err)
					cancel(workerErrors[worker])
					return
				}
				completed[worker]++
			}
		}(worker)
	}
	ready.Wait()
	close(start)
	// Context deadlines are cooperative. The go test -timeout remains the final
	// bound if a driver call ignores cancellation; do not abandon live workers.
	finished.Wait()
	count := 0
	for _, value := range completed {
		count += value
	}
	firstError := context.Cause(workCtx)
	err := firstError
	for _, workerError := range workerErrors {
		if workerError != nil && workerError != firstError {
			// Preserve additional cleanup/operation errors, not just the first
			// failure that triggered sibling cancellation.
			err = errors.Join(err, workerError)
		}
	}
	return count, err
}

func captureResourceRetentionHeap() resourceRetentionHeap {
	// Only the two endpoints force GC; there is no forced GC in the workload.
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return resourceRetentionHeap{HeapAlloc: stats.HeapAlloc, HeapObjects: stats.HeapObjects}
}
