/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

const (
	// Each batch contains a fixed number of completed operations so that
	// post-GC checkpoints can be compared across equal workloads.
	resourceRetentionDefaultCyclesPerBatch = 1024
	// The warm-up batch establishes connections and other one-time runtime
	// state. It is excluded from leak-trend comparisons.
	resourceRetentionWarmupBatches = 1
	// Measured batches are repeated after warm-up to reveal retained state.
	resourceRetentionDefaultMeasureBatches = 5
)

var resourceRetentionStressLevel = flag.String(
	"stress.level",
	"1",
	"resource-retention stress level: 1, 2, 3, 4, or all",
)

var resourceRetentionBatchTimeout = flag.Duration(
	"stress.batch-timeout",
	10*time.Minute,
	"maximum duration for one resource-retention batch; the context is cancelled when this expires",
)

var resourceRetentionCyclesPerBatch = flag.Int(
	"stress.cycles",
	resourceRetentionDefaultCyclesPerBatch,
	"number of resource-retention cycles per batch",
)

var resourceRetentionMeasureBatches = flag.Int(
	"stress.batches",
	resourceRetentionDefaultMeasureBatches,
	"number of measured resource-retention batches after warm-up",
)

type resourceRetentionLevel struct {
	// Workers controls concurrent workload execution. MaxOpenConnections
	// controls the database/sql pool used by this level.
	Name               string
	Workers            int
	MaxOpenConnections int
}

var resourceRetentionLevels = []resourceRetentionLevel{
	{Name: "level-1", Workers: 1, MaxOpenConnections: 1},
	{Name: "level-2", Workers: 4, MaxOpenConnections: 2},
	{Name: "level-3", Workers: 16, MaxOpenConnections: 8},
	{Name: "level-4", Workers: 32, MaxOpenConnections: 16},
}

type resourceRetentionPhase string

const (
	// Each phase isolates one resource-owning operation so a retention trend
	// can be associated with a specific driver/database/sql path.
	resourceRetentionSelectPhase     resourceRetentionPhase = "select"
	resourceRetentionPreparedPhase   resourceRetentionPhase = "prepared-statement"
	resourceRetentionCommitPhase     resourceRetentionPhase = "transaction-commit"
	resourceRetentionRollbackPhase   resourceRetentionPhase = "transaction-rollback"
	resourceRetentionConnectionPhase resourceRetentionPhase = "connection-checkout"
)

type resourceRetentionMemoryStats struct {
	// These values come from runtime.ReadMemStats. HeapAlloc and HeapObjects
	// after an explicit GC are the primary retained-memory signals; cumulative
	// allocation and GC counters describe workload pressure only.
	HeapAlloc     uint64  `json:"heap_alloc"`
	HeapObjects   uint64  `json:"heap_objects"`
	HeapInuse     uint64  `json:"heap_inuse"`
	HeapSys       uint64  `json:"heap_sys"`
	HeapReleased  uint64  `json:"heap_released"`
	TotalAlloc    uint64  `json:"total_alloc"`
	Mallocs       uint64  `json:"mallocs"`
	Frees         uint64  `json:"frees"`
	NumGC         uint32  `json:"num_gc"`
	PauseTotalNs  uint64  `json:"pause_total_ns"`
	GCCPUFraction float64 `json:"gc_cpu_fraction"`
}

type resourceRetentionDBStats struct {
	// These values come from database/sql DB.Stats and show whether the pool
	// has returned connections after each batch.
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration"`
	MaxIdleClosed      int64         `json:"max_idle_closed"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed"`
	MaxIdleTimeClosed  int64         `json:"max_idle_time_closed"`
}

type resourceRetentionCheckpoint struct {
	// A checkpoint records the runtime and pool state at one well-defined point
	// in the workload. JSON logging makes long manual runs easy to compare.
	Timestamp       time.Time                    `json:"timestamp"`
	Level           string                       `json:"level"`
	Phase           resourceRetentionPhase       `json:"phase"`
	Batch           int                          `json:"batch"`
	Checkpoint      string                       `json:"checkpoint"`
	CompletedCycles int                          `json:"completed_cycles"`
	Goroutines      int                          `json:"goroutines"`
	Memory          resourceRetentionMemoryStats `json:"memory"`
	DB              resourceRetentionDBStats     `json:"db"`
}

// TestDriver_ResourceRetentionMemoryLeakStress runs the first manual stress
// workload for retained Go heap and database/sql resources. It is intentionally
// registered in the explicit stress category and is not part of the normal
// unit, functional, or robustness runs.
func TestDriver_ResourceRetentionMemoryLeakStress(t *testing.T) {
	if !resourceRetentionStressCategoryEnabled() {
		t.Skip("stress category is not enabled; use -test.category=stress")
	}
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if *resourceRetentionBatchTimeout <= 0 {
		t.Fatal("-stress.batch-timeout must be greater than zero")
	}
	if *resourceRetentionCyclesPerBatch <= 0 {
		t.Fatal("-stress.cycles must be greater than zero")
	}
	if *resourceRetentionMeasureBatches <= 0 {
		t.Fatal("-stress.batches must be greater than zero")
	}

	// Read-only runtime settings are recorded for interpretation of the
	// measurements. The test deliberately does not change GOGC or GOMEMLIMIT.
	t.Logf(
		"stress_runtime_config GOGC=%q GOMEMLIMIT=%q runtime_memory_limit=%d cycles_per_batch=%d measured_batches=%d",
		os.Getenv("GOGC"),
		os.Getenv("GOMEMLIMIT"),
		debug.SetMemoryLimit(-1),
		*resourceRetentionCyclesPerBatch,
		*resourceRetentionMeasureBatches,
	)

	levels, err := selectedResourceRetentionLevels(*resourceRetentionStressLevel)
	if err != nil {
		t.Fatal(err)
	}

	for _, level := range levels {
		t.Run(level.Name, func(t *testing.T) {
			runResourceRetentionLevel(t, level)
		})
	}
}

func resourceRetentionStressCategoryEnabled() bool {
	for _, category := range oracleTest.TestCategories {
		if strings.EqualFold(strings.TrimSpace(category), "stress") {
			return true
		}
	}
	return false
}

func selectedResourceRetentionLevels(value string) ([]resourceRetentionLevel, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "all" {
		return resourceRetentionLevels, nil
	}

	levelNumber, err := strconv.Atoi(value)
	if err != nil || levelNumber < 1 || levelNumber > len(resourceRetentionLevels) {
		return nil, fmt.Errorf("invalid -stress.level %q: expected 1, 2, 3, 4, or all", value)
	}
	return []resourceRetentionLevel{resourceRetentionLevels[levelNumber-1]}, nil
}

func runResourceRetentionLevel(t *testing.T, level resourceRetentionLevel) {
	t.Helper()

	// A separate *sql.DB is used for each level so that pool state from one
	// level cannot affect the measurements for another level.
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close test DB: %v", closeErr)
		}
	}()

	// Keep the pool configuration fixed for the entire level. This makes
	// changes in InUse, Idle, and WaitCount attributable to the workload.
	db.SetMaxOpenConns(level.MaxOpenConnections)
	db.SetMaxIdleConns(level.MaxOpenConnections)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("level %s ping failed: %v", level.Name, err)
	}

	// Run each resource pattern separately. Separating phases avoids mixing
	// allocations from rows, statements, transactions, and connections.
	phases := []resourceRetentionPhase{
		resourceRetentionSelectPhase,
		resourceRetentionPreparedPhase,
		resourceRetentionCommitPhase,
		resourceRetentionRollbackPhase,
		resourceRetentionConnectionPhase,
	}
	for _, phase := range phases {
		if *resourceRetentionCyclesPerBatch%level.Workers != 0 {
			t.Fatalf("-stress.cycles=%d must be divisible by %d workers at %s", *resourceRetentionCyclesPerBatch, level.Workers, level.Name)
		}
		runResourceRetentionPhase(t, db, level, phase)
	}

	finalCheckpoint := captureResourceRetentionCheckpoint(
		level,
		"final-before-db-close",
		resourceRetentionPhase("final"),
		*resourceRetentionMeasureBatches,
		0,
		db,
	)
	logResourceRetentionCheckpoint(t, finalCheckpoint)

	if stats := db.Stats(); stats.InUse != 0 {
		t.Fatalf("level %s has %d in-use connections before database close", level.Name, stats.InUse)
	}
}

func runResourceRetentionPhase(
	t *testing.T,
	db *sql.DB,
	level resourceRetentionLevel,
	phase resourceRetentionPhase,
) {
	t.Helper()

	checkpoints := make([]resourceRetentionCheckpoint, 0, *resourceRetentionMeasureBatches)
	totalBatches := resourceRetentionWarmupBatches + *resourceRetentionMeasureBatches
	for batch := 0; batch < totalBatches; batch++ {
		// The before-batch checkpoint shows the natural state inherited from
		// the previous batch. No GC is forced at this point.
		before := captureResourceRetentionCheckpoint(
			level,
			"before-batch",
			phase,
			batch,
			batch**resourceRetentionCyclesPerBatch,
			db,
		)
		logResourceRetentionCheckpoint(t, before)

		// All workers in this batch share one deadline. If a database operation
		// stalls, cancellation is propagated to the operation instead of
		// allowing the whole test to wait indefinitely.
		batchContext, cancel := context.WithTimeout(context.Background(), *resourceRetentionBatchTimeout)
		err := runResourceRetentionBatch(batchContext, db, level, phase)
		cancel()
		if err != nil {
			t.Fatalf("level %s phase %s batch %d failed: %v", level.Name, phase, batch, err)
		}

		// Resources should already have been closed by each cycle. This
		// checkpoint captures the natural post-workload state before GC.
		after := captureResourceRetentionCheckpoint(
			level,
			"after-batch",
			phase,
			batch,
			(batch+1)**resourceRetentionCyclesPerBatch,
			db,
		)
		logResourceRetentionCheckpoint(t, after)

		// This is a controlled comparison point, not a production behavior
		// assertion. It helps distinguish temporary garbage from retained heap.
		runtime.GC()
		postGC := captureResourceRetentionCheckpoint(
			level,
			"post-gc",
			phase,
			batch,
			(batch+1)**resourceRetentionCyclesPerBatch,
			db,
		)
		logResourceRetentionCheckpoint(t, postGC)

		// Exclude warm-up from trend analysis because it may include one-time
		// allocations for connections, statements, and runtime structures.
		if batch >= resourceRetentionWarmupBatches {
			checkpoints = append(checkpoints, postGC)
		}
	}

	if stats := db.Stats(); stats.InUse != 0 {
		t.Fatalf("level %s phase %s left %d connections in use", level.Name, phase, stats.InUse)
	}
	reportResourceRetentionTrend(t, level, phase, checkpoints)
}

func runResourceRetentionBatch(
	ctx context.Context,
	db *sql.DB,
	level resourceRetentionLevel,
	phase resourceRetentionPhase,
) error {
	if *resourceRetentionCyclesPerBatch%level.Workers != 0 {
		return fmt.Errorf("batch size %d is not divisible by %d workers", *resourceRetentionCyclesPerBatch, level.Workers)
	}

	cyclesPerWorker := *resourceRetentionCyclesPerBatch / level.Workers
	// Starting all workers together creates a repeatable concurrency burst.
	// The channel is closed only after every worker has been created.
	start := make(chan struct{})
	errCh := make(chan error, 1)
	batchContext, cancel := context.WithCancel(ctx)
	defer cancel()
	var once sync.Once
	var wg sync.WaitGroup

	for worker := 0; worker < level.Workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for cycle := 0; cycle < cyclesPerWorker; cycle++ {
				// Each cycle must finish its own cleanup before the next cycle
				// starts. This prevents deferred cleanup from accumulating in a
				// high-volume loop.
				if err := runResourceRetentionCycle(batchContext, db, phase, cycle); err != nil {
					once.Do(func() {
						errCh <- err
						cancel()
					})
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)
	return <-errCh
}

func runResourceRetentionCycle(
	ctx context.Context,
	db *sql.DB,
	phase resourceRetentionPhase,
	cycle int,
) error {
	switch phase {
	case resourceRetentionSelectPhase:
		return runResourceRetentionSelect(ctx, db)
	case resourceRetentionPreparedPhase:
		return runResourceRetentionPrepared(ctx, db, cycle)
	case resourceRetentionCommitPhase:
		return runResourceRetentionTransaction(ctx, db, true)
	case resourceRetentionRollbackPhase:
		return runResourceRetentionTransaction(ctx, db, false)
	case resourceRetentionConnectionPhase:
		return runResourceRetentionConnection(ctx, db)
	default:
		return fmt.Errorf("unknown resource-retention phase %q", phase)
	}
}

func runResourceRetentionSelect(ctx context.Context, db *sql.DB) error {
	// QueryContext obtains rows from database/sql. The complete iteration and
	// explicit Close are intentional: an unclosed Rows is exactly the kind of
	// application resource that can make a leak appear to be driver-owned.
	rows, err := db.QueryContext(ctx, "SELECT 1 FROM DUAL")
	if err != nil {
		return err
	}

	var value int
	for rows.Next() {
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

func runResourceRetentionPrepared(ctx context.Context, db *sql.DB, cycle int) error {
	// The statement is created and closed for every cycle to exercise repeated
	// prepared-statement ownership rather than statement reuse.
	stmt, err := db.PrepareContext(ctx, "SELECT :1 FROM DUAL")
	if err != nil {
		return err
	}

	rows, err := stmt.QueryContext(ctx, cycle)
	if err != nil {
		_ = stmt.Close()
		return err
	}

	var value int
	for rows.Next() {
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			_ = stmt.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		_ = stmt.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		_ = stmt.Close()
		return err
	}
	return stmt.Close()
}

func runResourceRetentionTransaction(ctx context.Context, db *sql.DB, commit bool) error {
	// Both commit and rollback are tested because each transaction path must
	// release its rows, transaction, and underlying pooled connection.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, "SELECT 1 FROM DUAL")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	var value int
	for rows.Next() {
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		_ = tx.Rollback()
		return err
	}
	if err := rows.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if commit {
		return tx.Commit()
	}
	return tx.Rollback()
}

func runResourceRetentionConnection(ctx context.Context, db *sql.DB) error {
	// db.Conn checks out one pooled connection. Close must return it to the
	// pool immediately; db.Stats().InUse should therefore return to zero.
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}

	pingErr := conn.PingContext(ctx)
	closeErr := conn.Close()
	if pingErr != nil {
		return pingErr
	}
	return closeErr
}

func captureResourceRetentionCheckpoint(
	level resourceRetentionLevel,
	checkpoint string,
	phase resourceRetentionPhase,
	batch int,
	completedCycles int,
	db *sql.DB,
) resourceRetentionCheckpoint {
	// ReadMemStats is sampled without changing the runtime's GC policy. The
	// explicit GC happens in the caller immediately before post-GC samples.
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	stats := db.Stats()

	return resourceRetentionCheckpoint{
		Timestamp:       time.Now().UTC(),
		Level:           level.Name,
		Phase:           phase,
		Batch:           batch,
		Checkpoint:      checkpoint,
		CompletedCycles: completedCycles,
		Goroutines:      runtime.NumGoroutine(),
		Memory: resourceRetentionMemoryStats{
			HeapAlloc:     mem.HeapAlloc,
			HeapObjects:   mem.HeapObjects,
			HeapInuse:     mem.HeapInuse,
			HeapSys:       mem.HeapSys,
			HeapReleased:  mem.HeapReleased,
			TotalAlloc:    mem.TotalAlloc,
			Mallocs:       mem.Mallocs,
			Frees:         mem.Frees,
			NumGC:         mem.NumGC,
			PauseTotalNs:  mem.PauseTotalNs,
			GCCPUFraction: mem.GCCPUFraction,
		},
		DB: resourceRetentionDBStats{
			MaxOpenConnections: stats.MaxOpenConnections,
			OpenConnections:    stats.OpenConnections,
			InUse:              stats.InUse,
			Idle:               stats.Idle,
			WaitCount:          stats.WaitCount,
			WaitDuration:       stats.WaitDuration,
			MaxIdleClosed:      stats.MaxIdleClosed,
			MaxLifetimeClosed:  stats.MaxLifetimeClosed,
			MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		},
	}
}

func logResourceRetentionCheckpoint(t *testing.T, checkpoint resourceRetentionCheckpoint) {
	t.Helper()
	data, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatalf("failed to serialize resource-retention checkpoint: %v", err)
	}
	t.Logf("stress_metrics=%s", data)
}

func reportResourceRetentionTrend(
	t *testing.T,
	level resourceRetentionLevel,
	phase resourceRetentionPhase,
	checkpoints []resourceRetentionCheckpoint,
) {
	t.Helper()
	if len(checkpoints) < 2 {
		return
	}

	// A non-decreasing trend is an investigation signal, not an automatic
	// failure. Small changes can result from normal runtime behavior, so the
	// checkpoints must be reviewed and reproduced before classifying a leak.
	heapAllocIncreasing := true
	heapObjectsIncreasing := true
	for index := 1; index < len(checkpoints); index++ {
		previous := checkpoints[index-1].Memory
		current := checkpoints[index].Memory
		if current.HeapAlloc < previous.HeapAlloc {
			heapAllocIncreasing = false
		}
		if current.HeapObjects < previous.HeapObjects {
			heapObjectsIncreasing = false
		}
	}

	if heapAllocIncreasing || heapObjectsIncreasing {
		t.Logf(
			"resource-retention investigation: level=%s phase=%s post-GC memory shows a non-decreasing trend; heap_alloc=%d->%d heap_objects=%d->%d",
			level.Name,
			phase,
			checkpoints[0].Memory.HeapAlloc,
			checkpoints[len(checkpoints)-1].Memory.HeapAlloc,
			checkpoints[0].Memory.HeapObjects,
			checkpoints[len(checkpoints)-1].Memory.HeapObjects,
		)
	}
}
