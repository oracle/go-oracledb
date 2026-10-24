/*
** Copyright (c) 2026 Oracle and/or its affiliates.
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

// Only started/finished counts change during a cycle. Stage totals are merged
// when that cycle returns, including after a failure and its cleanup.
type lifecycleCounts struct {
	CyclesStarted         uint64 `json:"cycles_started"`
	ConnectionAttempts    uint64 `json:"connection_attempts"`
	SuccessfulConnections uint64 `json:"successful_connections"`
	QueryCompletions      uint64 `json:"query_completions"`
	RowCloses             uint64 `json:"row_closes"`
	ConnectionCloses      uint64 `json:"connection_closes"`
	CompletedCycles       uint64 `json:"completed_cycles"`
	FailedCycles          uint64 `json:"failed_cycles"`
	WorkersFinished       uint64 `json:"workers_finished"`
}

type lifecycleProgress struct {
	sync.Mutex
	counts lifecycleCounts
}

func (p *lifecycleProgress) start()    { p.Lock(); p.counts.CyclesStarted++; p.Unlock() }
func (p *lifecycleProgress) finished() { p.Lock(); p.counts.WorkersFinished++; p.Unlock() }

func (p *lifecycleProgress) add(c directConnectionCycle, err error) {
	p.Lock()
	defer p.Unlock()
	if c.Attempted {
		p.counts.ConnectionAttempts++
	}
	if c.Connected {
		p.counts.SuccessfulConnections++
	}
	if c.Queried {
		p.counts.QueryCompletions++
	}
	if c.RowsClosed {
		p.counts.RowCloses++
	}
	if c.Closed {
		p.counts.ConnectionCloses++
	}
	if err == nil {
		p.counts.CompletedCycles++
	} else {
		p.counts.FailedCycles++
	}
}

func (p *lifecycleProgress) snapshot() lifecycleCounts {
	p.Lock()
	defer p.Unlock()
	return p.counts
}

func (c lifecycleCounts) validate(workers int, expected uint64) error {
	n := c.CompletedCycles
	if c.WorkersFinished != uint64(workers) || c.FailedCycles != 0 || n == 0 ||
		c.CyclesStarted != n || c.ConnectionAttempts != n || c.SuccessfulConnections != n ||
		c.QueryCompletions != n || c.RowCloses != n || c.ConnectionCloses != n ||
		(expected != 0 && n != expected) {
		return fmt.Errorf("incomplete lifecycle workload: %+v; expected cycles=%d workers=%d", c, expected, workers)
	}
	return nil
}

type connectionLifecycleRunResult struct {
	Counts        lifecycleCounts
	Active, Drain time.Duration
}

// This is deliberately not a Go-discoverable Test function. Only the manually
// selected exclusive stress category can launch the potentially multi-day run.
func runDriverConnectionLifecycleStress(t *testing.T) {
	if TestingConfig == nil || TestingConfig.Stress == nil || TestingConfig.Stress.ConnectionLifecycle == nil {
		t.Skip("no stress.connection_lifecycle profile; stress workload did not run")
	}
	config, err := resolveLifecycleConfig(*TestingConfig.Stress.ConnectionLifecycle)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRetentionTLS(TestingConfig); err != nil {
		t.Fatal(err)
	}
	timeout, _ := time.ParseDuration(config.Timeout)
	if deadline, ok := t.Deadline(); ok && time.Until(deadline) <= timeout {
		t.Fatalf("Go test timeout must exceed JSON timeout %s", config.Timeout)
	}
	runID := newLifecycleRunID()
	t.Logf("connection_lifecycle run_id=%s pid=%d samples=%s workers=%d cycles_per_worker=%d duration=%s; external evidence required for leak sign-off",
		runID, os.Getpid(), filepath.Join(config.OutputDirectory, runID, "samples.jsonl"), config.Workers, config.CyclesPerWorker, config.Duration)
	result, err := executeConnectionLifecycleStress(context.Background(), config, runID, TestingConfig,
		func() (driver.Connector, error) { return openTestConnectorWithConfig(TestingConfig) }, 2*time.Minute)
	// The coordinator has checked file Sync/Close before returning. Go's test
	// outcome, not the last JSON record, is authoritative for the complete run.
	t.Logf("connection_lifecycle workload_status=%s measured=%+v warmup=%+v active=%s drain=%s wall=%s samples=%s",
		result.WorkloadStatus, result.Workload.Counts, result.Warmup.Counts, result.Workload.Active, result.Workload.Drain, result.Wall, result.Artifact)
	if err != nil {
		t.Fatal(lifecycleSafeError(err, TestingConfig))
	}
}

// The caller validates/normalizes config and supplies the run's safety context.
// The admission deadline never cancels an otherwise healthy in-flight cycle.
func runConnectionLifecycleWorkers(parent context.Context, config oracleTest.ConnectionLifecycleConfig, factory lifecycleConnectorFactory, runID string, progress *lifecycleProgress) (result connectionLifecycleRunResult, err error) {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	operationTimeout, _ := time.ParseDuration(config.OperationTimeout)
	duration, _ := time.ParseDuration(config.Duration)
	start := make(chan struct{})
	var workers sync.WaitGroup
	var admitUntil time.Time // Published by closing start.
	for worker := 0; worker < config.Workers; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			defer progress.finished()
			<-start
			for cycle := 0; ; cycle++ {
				if ctx.Err() != nil {
					return
				}
				if duration > 0 {
					if !time.Now().Before(admitUntil) {
						return
					}
				} else if cycle >= config.CyclesPerWorker {
					return
				}
				progress.start()
				opCtx, stop := context.WithTimeout(ctx, operationTimeout)
				c, cycleErr := executeDirectConnectionCycle(opCtx, factory, runID)
				stop()
				progress.add(c, cycleErr)
				if cycleErr != nil {
					cancel(fmt.Errorf("worker %d cycle %d: %w", worker+1, cycle+1, cycleErr))
					return
				}
			}
		}(worker)
	}
	began := time.Now()
	admitUntil = began.Add(duration)
	close(start)
	workers.Wait() // Never abandon workers; Go's -timeout bounds uncooperative calls.
	result.Active = time.Since(began)
	if duration > 0 && result.Active > duration {
		result.Drain = result.Active - duration
		result.Active = duration
	}
	result.Counts = progress.snapshot()
	err = context.Cause(ctx)
	if err == nil {
		var expected uint64
		if duration == 0 {
			expected = uint64(config.Workers * config.CyclesPerWorker)
		}
		err = result.Counts.validate(config.Workers, expected)
	}
	return
}

type lifecycleMemory struct {
	HeapAlloc     uint64  `json:"heap_alloc"`
	HeapObjects   uint64  `json:"heap_objects"`
	HeapInuse     uint64  `json:"heap_inuse"`
	HeapSys       uint64  `json:"heap_sys"`
	HeapReleased  uint64  `json:"heap_released"`
	StackInuse    uint64  `json:"stack_inuse"`
	TotalAlloc    uint64  `json:"total_alloc"`
	Mallocs       uint64  `json:"mallocs"`
	Frees         uint64  `json:"frees"`
	NumGC         uint32  `json:"num_gc"`
	PauseTotalNs  uint64  `json:"pause_total_ns"`
	GCCPUFraction float64 `json:"gc_cpu_fraction"`
}

type lifecycleSample struct {
	Phase      string          `json:"phase"`
	RunID      string          `json:"run_id"`
	PID        int             `json:"pid"`
	Timestamp  time.Time       `json:"timestamp_utc"`
	Elapsed    time.Duration   `json:"elapsed_ns"`
	Counts     lifecycleCounts `json:"counts"`
	Memory     lifecycleMemory `json:"memory"`
	Goroutines int             `json:"goroutines"`
}

func writeLifecycleSample(w io.Writer, phase, runID string, began time.Time, p *lifecycleProgress) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return json.NewEncoder(w).Encode(lifecycleSample{
		Phase: phase, RunID: runID, PID: os.Getpid(), Timestamp: time.Now().UTC(), Elapsed: time.Since(began),
		Counts: p.snapshot(), Goroutines: runtime.NumGoroutine(),
		Memory: lifecycleMemory{
			HeapAlloc: m.HeapAlloc, HeapObjects: m.HeapObjects, HeapInuse: m.HeapInuse,
			HeapSys: m.HeapSys, HeapReleased: m.HeapReleased, StackInuse: m.StackInuse,
			TotalAlloc: m.TotalAlloc, Mallocs: m.Mallocs, Frees: m.Frees,
			NumGC: m.NumGC, PauseTotalNs: m.PauseTotalNs, GCCPUFraction: m.GCCPUFraction,
		},
	})
}

// During work this is the only writer. The caller stops and joins it exactly
// once before writing endpoints, so no writer lock or finalization state is needed.
func startLifecycleSampler(ctx context.Context, interval time.Duration, w io.Writer, runID string, began time.Time, p *lifecycleProgress, fail context.CancelCauseFunc) func() error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := writeLifecycleSample(w, "natural", runID, began, p); err != nil {
					fail(fmt.Errorf("runtime sample: %w", err))
					done <- err
					return
				}
			}
		}
	}()
	return func() error { cancel(); return <-done }
}

func writeLifecycleEndpoint(w io.Writer, phase, runID string, began time.Time, p *lifecycleProgress, profiles bool, dir string) error {
	runtime.GC() // Only drained endpoints use controlled GC.
	err := writeLifecycleSample(w, phase+"_post_gc", runID, began, p)
	if profiles {
		err = errors.Join(err, writeRetentionProfile(filepath.Join(dir, phase+"-heap.pprof"), "heap", 0))
	}
	return err
}

type lifecycleStressResult struct {
	Warmup, Workload connectionLifecycleRunResult
	Artifact         string
	Wall             time.Duration
	WorkloadStatus   string
}

// Resolve a value copy: neither the selected JSON nor retention config changes.
func resolveLifecycleConfig(config oracleTest.ConnectionLifecycleConfig) (oracleTest.ConnectionLifecycleConfig, error) {
	if err := config.Validate(); err != nil {
		return config, err
	}
	config = config.Normalized()
	dir, err := filepath.Abs(config.OutputDirectory)
	config.OutputDirectory = dir
	return config, err
}

// config has already been validated and normalized. No production APIs, pool,
// platform subprocesses, or generic collection framework are involved here.
func executeConnectionLifecycleStress(parent context.Context, config oracleTest.ConnectionLifecycleConfig, runID string, testConfig *TestConfig, factory lifecycleConnectorFactory, observeFor time.Duration) (result lifecycleStressResult, err error) {
	began := time.Now()
	result.WorkloadStatus = "not_started"
	timeout, _ := time.ParseDuration(config.Timeout)
	deadlineCtx, stop := context.WithTimeout(parent, timeout)
	defer stop()
	ctx, cancel := context.WithCancelCause(deadlineCtx)
	defer cancel(nil)

	dir := filepath.Join(config.OutputDirectory, runID)
	if err = os.MkdirAll(config.OutputDirectory, 0700); err != nil {
		return
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return
	} // Never overwrite old evidence.
	result.Artifact = filepath.Join(dir, "samples.jsonl")
	file, openErr := os.OpenFile(result.Artifact, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if openErr != nil {
		return result, openErr
	}
	encoder := json.NewEncoder(file)
	defer func() {
		err = errors.Join(err, context.Cause(ctx))
		// This record describes the workload, not future file Sync/Close success.
		err = errors.Join(err, encoder.Encode(map[string]any{
			"phase": "result", "run_id": runID, "timestamp_utc": time.Now().UTC(), "workload_status": result.WorkloadStatus,
			"counts": result.Workload.Counts, "warmup_counts": result.Warmup.Counts,
			"active_ns": result.Workload.Active.Nanoseconds(), "drain_ns": result.Workload.Drain.Nanoseconds(),
			"error": lifecycleSafeError(err, testConfig),
		}))
		err = errors.Join(err, file.Sync(), file.Close())
		if err != nil && config.ProfilesEnabled() {
			err = errors.Join(err, writeRetentionProfile(filepath.Join(dir, "failure-goroutines.txt"), "goroutine", 2))
		}
		result.Wall = time.Since(began)
	}()

	configName := ""
	if testConfig != nil {
		configName = testConfig.ConfigName
	}
	if err = encoder.Encode(map[string]any{
		"phase": "configuration", "run_id": runID, "pid": os.Getpid(), "timestamp_utc": began.UTC(),
		"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "config_name": configName, "profile": config,
	}); err != nil {
		return
	}

	warmupConfig := config
	warmupConfig.Duration, warmupConfig.CyclesPerWorker = "", 1
	result.Warmup, err = runConnectionLifecycleWorkers(ctx, warmupConfig, factory, runID, &lifecycleProgress{})
	err = errors.Join(err, encoder.Encode(map[string]any{"phase": "warmup", "run_id": runID, "counts": result.Warmup.Counts}))
	if err != nil {
		return
	}

	p := &lifecycleProgress{}
	err = writeLifecycleSample(file, "baseline_natural", runID, began, p)
	err = errors.Join(err, writeLifecycleEndpoint(file, "baseline", runID, began, p, config.ProfilesEnabled(), dir))
	if err != nil {
		return
	}

	interval, _ := time.ParseDuration(config.SampleInterval)
	stopSamples := startLifecycleSampler(ctx, interval, file, runID, began, p, cancel)
	result.Workload, err = runConnectionLifecycleWorkers(ctx, config, factory, runID, p)
	result.WorkloadStatus = "complete"
	if err != nil {
		result.WorkloadStatus = "failed"
	}
	err = errors.Join(err, stopSamples())

	err = errors.Join(err, writeLifecycleSample(file, "final_natural", runID, began, p))
	err = errors.Join(err, writeLifecycleEndpoint(file, "final", runID, began, p, config.ProfilesEnabled(), dir))
	if err == nil && observeFor > 0 {
		err = encoder.Encode(map[string]any{"phase": "observation_start", "run_id": runID, "timestamp_utc": time.Now().UTC(), "planned_ns": observeFor.Nanoseconds()})
		if err == nil {
			timer := time.NewTimer(observeFor)
			select {
			case <-timer.C:
			case <-ctx.Done():
				err = context.Cause(ctx)
			}
			timer.Stop()
		}
	}
	err = errors.Join(err, writeLifecycleEndpoint(file, "settled", runID, began, p, config.ProfilesEnabled(), dir))
	return
}

func lifecycleSafeError(err error, config *TestConfig) string {
	if err == nil {
		return ""
	}
	return redactRetentionText(err.Error(), config)
}
