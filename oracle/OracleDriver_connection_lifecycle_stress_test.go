/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"flag"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

const (
	// Batch 0 warms up the workload. It is logged and checked just like later
	// batches, but should be identified separately when reviewing measurements.
	connectionLifecycleWarmupBatches = 1
	// Cycles are counted across the whole batch, not separately for each worker.
	connectionLifecycleDefaultCyclesPerBatch = 1024
	// These batches run after warm-up for each scenario at every selected level.
	connectionLifecycleDefaultMeasureBatches = 5
)

// Separate flag names allow this workload and the resource-retention workload
// to be configured independently when both are selected through the executor.
var connectionLifecycleStressLevel = flag.String(
	"stress.connection.level",
	"1",
	"connection-lifecycle stress level: 1, 2, 3, 4, or all",
)

var connectionLifecycleCyclesPerBatch = flag.Int(
	"stress.connection.cycles",
	connectionLifecycleDefaultCyclesPerBatch,
	"number of connection-lifecycle cycles per batch",
)

var connectionLifecycleMeasureBatches = flag.Int(
	"stress.connection.batches",
	connectionLifecycleDefaultMeasureBatches,
	"number of measured connection-lifecycle batches after warm-up",
)

// The batch deadline is shared by all workers and all cycles in that batch.
// Go's -timeout applies separately to the entire test process, including every
// level, scenario, and warm-up batch, so it must allow for their combined time.
var connectionLifecycleBatchTimeout = flag.Duration(
	"stress.connection.batch-timeout",
	30*time.Minute,
	"maximum duration for one connection-lifecycle batch",
)

// connectionLifecycleLevel controls concurrent workers. Each worker processes
// one database handle at a time; there is no shared pool between workers.
type connectionLifecycleLevel struct {
	Name    string
	Workers int
}

var connectionLifecycleLevels = []connectionLifecycleLevel{
	{Name: "level-1", Workers: 1},
	{Name: "level-2", Workers: 4},
	{Name: "level-3", Workers: 16},
	{Name: "level-4", Workers: 32},
}

type connectionLifecycleScenario string

const (
	// Repeated workers begin as they are scheduled and run independently.
	connectionLifecycleRepeatedScenario connectionLifecycleScenario = "repeated"
	// Burst workers wait for a common start signal once per batch. Subsequent
	// cycles run independently; this does not synchronize every connection.
	connectionLifecycleBurstScenario connectionLifecycleScenario = "burst"
)

// connectionLifecycleBatchReport summarizes logical cycle steps, not physical
// socket or Oracle session counts. See runConnectionLifecycleCycle for the
// effect of the zero-idle pool on the ping and query connections.
type connectionLifecycleBatchReport struct {
	Attempts         int64 // Cycles started, including the cycle that fails.
	Connections      int64 // Cycles whose PingContext completed successfully.
	Queries          int64 // Cycles whose query, validation, and rows.Close succeeded.
	Closed           int64 // DB handles closed with zero remaining pool connections.
	CompletedWorkers int64 // Workers that finished every assigned cycle successfully.
	// Process-wide diagnostic snapshots; a temporary increase is not by itself
	// a goroutine leak and is not an automatic test failure.
	GoroutinesBefore int
	GoroutinesAfter  int
	Duration         time.Duration
}

// connectionLifecycleCycleResult preserves partial progress even on error.
// For example, a failed ping can still have Closed=true after handle cleanup.
// Pool snapshots are used for cleanup assertions, not emitted as per-cycle logs.
type connectionLifecycleCycleResult struct {
	Connected        bool
	Queried          bool
	Closed           bool
	StatsBeforeClose sql.DBStats
	StatsAfterClose  sql.DBStats
}

// TestDriver_ConnectionLifecycleStress validates repeated physical connection
// creation, session setup, one valid operation, and connection closure. It is
// limited to valid normal-load scenarios. Cancellation is used only to stop a
// failed or expired batch, rather than as an injected workload scenario.
//
// Execution order is level -> scenario -> batch -> worker -> cycle. Defaults
// produce 2 scenarios * (1 warm-up + 5 measured batches) * 1024 = 12,288 logical
// cycles per level. Level 1 has one worker, so it cannot exercise concurrency.
func TestDriver_ConnectionLifecycleStress(t *testing.T) {
	// Go discovers Test functions even outside TestCategoryExecutor. This gate
	// ensures an ordinary test invocation does not start the stress workload.
	if !connectionLifecycleStressCategoryEnabled() {
		t.Skip("stress category is not enabled; use -test.category=stress")
	}
	// TestMain loads the selected configuration from the JSON test-config file.
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if *connectionLifecycleCyclesPerBatch <= 0 {
		t.Fatal("-stress.connection.cycles must be greater than zero")
	}
	if *connectionLifecycleMeasureBatches <= 0 {
		t.Fatal("-stress.connection.batches must be greater than zero")
	}
	if *connectionLifecycleBatchTimeout <= 0 {
		t.Fatal("-stress.connection.batch-timeout must be greater than zero")
	}

	levels, err := selectedConnectionLifecycleLevels(*connectionLifecycleStressLevel)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf(
		"connection_lifecycle_config level=%q cycles_per_batch=%d measured_batches=%d batch_timeout=%s",
		*connectionLifecycleStressLevel,
		*connectionLifecycleCyclesPerBatch,
		*connectionLifecycleMeasureBatches,
		*connectionLifecycleBatchTimeout,
	)

	// Levels run serially; only the explicit worker goroutines run concurrently.
	for _, level := range levels {
		t.Run(level.Name, func(t *testing.T) {
			runConnectionLifecycleLevel(t, level)
		})
	}
}

// connectionLifecycleStressCategoryEnabled checks the explicitly selected
// categories; the category registration alone does not gate direct invocations.
func connectionLifecycleStressCategoryEnabled() bool {
	for _, category := range oracleTest.TestCategories {
		if strings.EqualFold(strings.TrimSpace(category), "stress") {
			return true
		}
	}
	return false
}

// selectedConnectionLifecycleLevels accepts a one-based level number or "all".
// Selecting "all" preserves the increasing worker order in the level table.
func selectedConnectionLifecycleLevels(value string) ([]connectionLifecycleLevel, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "all" {
		return connectionLifecycleLevels, nil
	}

	levelIndex, err := strconv.Atoi(value)
	if err != nil || levelIndex < 1 || levelIndex > len(connectionLifecycleLevels) {
		return nil, fmt.Errorf("invalid -stress.connection.level %q: expected 1, 2, 3, 4, or all", value)
	}
	return []connectionLifecycleLevel{connectionLifecycleLevels[levelIndex-1]}, nil
}

// runConnectionLifecycleLevel runs both scenarios with the same batch size.
// Each worker receives cycles/workers cycles so every batch has equal work.
func runConnectionLifecycleLevel(t *testing.T, level connectionLifecycleLevel) {
	t.Helper()
	if *connectionLifecycleCyclesPerBatch%level.Workers != 0 {
		t.Fatalf(
			"-stress.connection.cycles=%d must be divisible by %d workers at %s",
			*connectionLifecycleCyclesPerBatch,
			level.Workers,
			level.Name,
		)
	}

	for _, scenario := range []connectionLifecycleScenario{
		connectionLifecycleRepeatedScenario,
		connectionLifecycleBurstScenario,
	} {
		if !t.Run(string(scenario), func(t *testing.T) {
			totalBatches := connectionLifecycleWarmupBatches + *connectionLifecycleMeasureBatches
			for batch := 0; batch < totalBatches; batch++ {
				// A fresh deadline starts for each batch, including warm-up. Cancel
				// immediately after the workers return to release the context timer.
				batchContext, cancel := context.WithTimeout(context.Background(), *connectionLifecycleBatchTimeout)
				started := time.Now()
				report, err := runConnectionLifecycleBatch(batchContext, level, scenario)
				cancel()
				report.Duration = time.Since(started)

				// Log partial counters before reporting errors. A failed cycle can
				// close its DB handle even when its ping or query did not succeed.
				t.Logf(
					"connection_lifecycle_metrics level=%s scenario=%s batch=%d attempts=%d connections=%d queries=%d closed=%d completed_workers=%d goroutines_before=%d goroutines_after=%d duration=%s",
					level.Name,
					scenario,
					batch,
					report.Attempts,
					report.Connections,
					report.Queries,
					report.Closed,
					report.CompletedWorkers,
					report.GoroutinesBefore,
					report.GoroutinesAfter,
					report.Duration,
				)

				if err != nil {
					t.Fatalf("level %s scenario %s batch %d failed: %v", level.Name, scenario, batch, err)
				}
				// A successful batch must account for every configured cycle and
				// every worker; returning without an error alone is insufficient.
				if report.Attempts != int64(*connectionLifecycleCyclesPerBatch) {
					t.Fatalf("level %s scenario %s batch %d attempted %d cycles, want %d", level.Name, scenario, batch, report.Attempts, *connectionLifecycleCyclesPerBatch)
				}
				if report.Connections != report.Attempts || report.Queries != report.Attempts || report.Closed != report.Attempts {
					t.Fatalf("level %s scenario %s batch %d lifecycle counts do not match: attempts=%d connections=%d queries=%d closed=%d", level.Name, scenario, batch, report.Attempts, report.Connections, report.Queries, report.Closed)
				}
				if report.CompletedWorkers != int64(level.Workers) {
					t.Fatalf("level %s scenario %s batch %d completed %d workers, want %d", level.Name, scenario, batch, report.CompletedWorkers, level.Workers)
				}
			}
		}) {
			// Do not start the next scenario at this level after a failed batch.
			return
		}
	}
}

// runConnectionLifecycleBatch owns the worker lifetime for one batch. It stops
// further cycles on the first error and waits for workers to finish cleanup.
func runConnectionLifecycleBatch(
	ctx context.Context,
	level connectionLifecycleLevel,
	scenario connectionLifecycleScenario,
) (connectionLifecycleBatchReport, error) {
	if *connectionLifecycleCyclesPerBatch%level.Workers != 0 {
		return connectionLifecycleBatchReport{}, fmt.Errorf("batch size %d is not divisible by %d workers", *connectionLifecycleCyclesPerBatch, level.Workers)
	}

	// Prepare configuration once per worker before starting network activity.
	// A connector is a connection factory, not an open database connection.
	connectors := make([]driver.Connector, level.Workers)
	for worker := range connectors {
		connector, err := openTestConnectorWithConfig(TestingConfig)
		if err != nil {
			return connectionLifecycleBatchReport{}, fmt.Errorf("worker %d connector setup failed: %w", worker, err)
		}
		connectors[worker] = connector
	}

	// The parent supplies the deadline; this child also lets a worker error
	// stop its siblings without waiting for that deadline to expire.
	workContext, cancel := context.WithCancel(ctx)
	defer cancel()

	// Workers update counters concurrently. sync.Once retains the first error;
	// workers.Wait below ensures it is written before the coordinator reads it.
	var attempts atomic.Int64
	var connections atomic.Int64
	var queries atomic.Int64
	var closed atomic.Int64
	var completedWorkers atomic.Int64
	var firstErr error
	var firstErrOnce sync.Once
	var workers sync.WaitGroup
	// Buffered readiness signals let every burst worker announce itself before
	// the coordinator starts receiving; closing start releases all ready workers.
	ready := make(chan struct{}, level.Workers)
	start := make(chan struct{})
	goroutinesBefore := runtime.NumGoroutine()

	cyclesPerWorker := *connectionLifecycleCyclesPerBatch / level.Workers
	for worker := 0; worker < level.Workers; worker++ {
		workers.Add(1)
		go func(worker int) {
			// This defer is once per worker, outside the high-volume cycle loop.
			defer workers.Done()
			if scenario == connectionLifecycleBurstScenario {
				ready <- struct{}{}
				select {
				case <-start:
				case <-workContext.Done():
					// A deadline while waiting at the barrier must also release workers.
					return
				}
			}

			for cycle := 0; cycle < cyclesPerWorker; cycle++ {
				if workContext.Err() != nil {
					return
				}
				attempts.Add(1)
				result, err := runConnectionLifecycleCycle(workContext, connectors[worker])
				if result.Connected {
					connections.Add(1)
				}
				if result.Queried {
					queries.Add(1)
				}
				if result.Closed {
					closed.Add(1)
				}
				if err != nil {
					// Preserve the first observed error rather than overwriting it
					// with cancellation errors returned by the other workers.
					firstErrOnce.Do(func() {
						firstErr = fmt.Errorf("worker %d cycle %d: %w", worker, cycle, err)
						cancel()
					})
					return
				}
			}
			completedWorkers.Add(1)
		}(worker)
	}

	if scenario == connectionLifecycleBurstScenario {
		for worker := 0; worker < level.Workers; worker++ {
			select {
			case <-ready:
			case <-workContext.Done():
				workers.Wait()
				return connectionLifecycleBatchReport{
					Attempts:         attempts.Load(),
					Connections:      connections.Load(),
					Queries:          queries.Load(),
					Closed:           closed.Load(),
					CompletedWorkers: completedWorkers.Load(),
					GoroutinesBefore: goroutinesBefore,
					GoroutinesAfter:  runtime.NumGoroutine(),
				}, workContext.Err()
			}
		}
		close(start)
	}

	// Waiting here accounts for every workload worker before sampling counters.
	// Context deadlines are cooperative: Wait cannot forcibly stop a stuck driver
	// call, and db.Close has no context argument. Go's -timeout is the final bound.
	workers.Wait()
	report := connectionLifecycleBatchReport{
		Attempts:         attempts.Load(),
		Connections:      connections.Load(),
		Queries:          queries.Load(),
		Closed:           closed.Load(),
		CompletedWorkers: completedWorkers.Load(),
		GoroutinesBefore: goroutinesBefore,
		GoroutinesAfter:  runtime.NumGoroutine(),
	}

	if firstErr != nil {
		return report, firstErr
	}
	if err := ctx.Err(); err != nil {
		return report, fmt.Errorf("connection-lifecycle batch timed out or was cancelled: %w", err)
	}
	if report.CompletedWorkers != int64(level.Workers) {
		return report, fmt.Errorf("completed %d workers, want %d", report.CompletedWorkers, level.Workers)
	}
	return report, nil
}

// runConnectionLifecycleCycle owns one DB handle from creation to cleanup.
// OpenDB is lazy: PingContext performs the first physical connection attempt.
//
// With MaxIdleConns(0), PingContext releases and closes its connection before
// QueryContext runs. The query therefore normally establishes another physical
// connection. A successful cycle counts one ping, one query, and one closed DB
// handle; it is not a count of exactly one physical connection.
func runConnectionLifecycleCycle(ctx context.Context, connector driver.Connector) (connectionLifecycleCycleResult, error) {
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	// Disable age-based recycling so it does not introduce another churn source.
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	result := connectionLifecycleCycleResult{}
	if err := db.PingContext(ctx); err != nil {
		return finishConnectionLifecycleCycle(db, result, err)
	}
	result.Connected = true

	rows, err := db.QueryContext(ctx, "SELECT 1 FROM DUAL")
	if err != nil {
		return finishConnectionLifecycleCycle(db, result, err)
	}

	// Validate the whole one-row result: missing rows, wrong values, extra rows,
	// and iteration errors all fail the cycle. No tables or test data are created.
	var value int
	var operationErr error
	if !rows.Next() {
		operationErr = rows.Err()
		if operationErr == nil {
			operationErr = fmt.Errorf("SELECT 1 FROM DUAL returned no rows")
		}
	} else if err := rows.Scan(&value); err != nil {
		operationErr = err
	} else if value != 1 {
		operationErr = fmt.Errorf("SELECT 1 FROM DUAL returned %d", value)
	} else if rows.Next() {
		operationErr = fmt.Errorf("SELECT 1 FROM DUAL returned more than one row")
	} else if err := rows.Err(); err != nil {
		operationErr = err
	}
	// Close explicitly on every query path before inspecting pool state. The
	// first query/validation error takes precedence over a later rows-close error.
	if err := rows.Close(); err != nil && operationErr == nil {
		operationErr = err
	}
	if operationErr == nil {
		result.Queried = true
	}

	return finishConnectionLifecycleCycle(db, result, operationErr)
}

// finishConnectionLifecycleCycle is the common exit path for successful and
// failed operations, including failed pings. Always attempt DB cleanup before
// returning the error to the worker.
func finishConnectionLifecycleCycle(
	db *sql.DB,
	result connectionLifecycleCycleResult,
	operationErr error,
) (connectionLifecycleCycleResult, error) {
	// Query rows have already been closed, so no connection should still be in
	// use. Preserve an existing operation error if cleanup finds another problem.
	result.StatsBeforeClose = db.Stats()
	if operationErr == nil && result.StatsBeforeClose.InUse != 0 {
		operationErr = fmt.Errorf("%d database connections remain in use before close", result.StatsBeforeClose.InUse)
	}

	closeErr := db.Close()
	// These checks verify database/sql's pool accounting. They do not measure
	// operating-system sockets, file descriptors, or Oracle session cleanup.
	result.StatsAfterClose = db.Stats()
	if result.StatsAfterClose.OpenConnections != 0 || result.StatsAfterClose.InUse != 0 || result.StatsAfterClose.Idle != 0 {
		if operationErr == nil {
			operationErr = fmt.Errorf("database handle retained connections after close: open=%d in_use=%d idle=%d", result.StatsAfterClose.OpenConnections, result.StatsAfterClose.InUse, result.StatsAfterClose.Idle)
		}
	} else if closeErr == nil {
		result.Closed = true
	}

	// Report both errors when the operation and DB.Close fail, so cleanup does
	// not hide the original failure (and its own failure is not lost either).
	if operationErr != nil && closeErr != nil {
		return result, fmt.Errorf("operation error: %v; close error: %w", operationErr, closeErr)
	}
	if operationErr != nil {
		return result, operationErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if !result.Closed {
		return result, fmt.Errorf("database handle was not confirmed closed")
	}
	return result, nil
}
