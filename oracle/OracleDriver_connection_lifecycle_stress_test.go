/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package oracle

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
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
	Counts                 lifecycleCounts
	Elapsed, Active, Drain time.Duration
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
	active, drain := "unavailable", "unavailable"
	if result.WorkloadStatus == "complete" {
		active, drain = result.Workload.Active.String(), result.Workload.Drain.String()
	}
	t.Logf("connection_lifecycle workload_status=%s measured=%+v warmup=%+v workload_elapsed=%s active=%s drain=%s observation=%s observation_completed=%t wall=%s samples=%s",
		result.WorkloadStatus, result.Workload.Counts, result.Warmup.Counts, result.Workload.Elapsed, active, drain,
		result.Observation, result.ObservationCompleted, result.Wall, result.Artifact)
	if err != nil {
		t.Fatal(lifecycleSafeError(err, TestingConfig))
	}
}

// Direct physical connection operation.

// The result describes completed client actions, not Oracle session cleanup.
type directConnectionCycle struct {
	Attempted, Connected, Queried, RowsClosed, Closed bool
}

type lifecycleConnectorFactory func() (driver.Connector, error)

// Each call constructs a new connector and owns one physical connection. The
// separate defers close rows before the connection, even on an early return or
// panic. They finish before another iteration starts; panics are not recovered.
func executeDirectConnectionCycle(ctx context.Context, factory lifecycleConnectorFactory, runID string) (result directConnectionCycle, err error) {
	defer func() {
		if err == nil && ctx.Err() != nil {
			err = fmt.Errorf("operation deadline: %w", ctx.Err())
		}
	}()
	connector, createErr := factory()
	if createErr != nil {
		return result, fmt.Errorf("create connector: %w", createErr)
	}
	if connector == nil {
		return result, errors.New("create connector: returned nil without an error")
	}
	result.Attempted = true
	conn, connectErr := connector.Connect(ctx)
	// A partial connection returned with an error is still our responsibility.
	if conn != nil {
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				err = errors.Join(err, &lifecycleCleanupError{"connection", closeErr})
			} else {
				result.Closed = true
			}
		}()
	}
	if connectErr != nil {
		return result, fmt.Errorf("connect: %w", connectErr)
	}
	if conn == nil {
		return result, errors.New("connect: returned nil without an error")
	}
	result.Connected = true
	if err = initializeLifecycleSession(ctx, conn, runID); err != nil {
		return result, err
	}

	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		return result, errors.New("query: connection lacks driver.QueryerContext")
	}
	rows, queryErr := queryer.QueryContext(ctx, "SELECT 1 FROM DUAL", nil)
	if rows != nil {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				err = errors.Join(err, &lifecycleCleanupError{"rows", closeErr})
			} else {
				result.RowsClosed = true
			}
		}()
	}
	if queryErr != nil {
		return result, fmt.Errorf("query: %w", queryErr)
	}
	if rows == nil {
		return result, errors.New("query: returned nil rows without an error")
	}
	columns := rows.Columns()
	if len(columns) != 1 {
		return result, fmt.Errorf("query: DUAL returned %d columns, want 1", len(columns))
	}
	values := make([]driver.Value, 1)
	if nextErr := rows.Next(values); nextErr != nil {
		if errors.Is(nextErr, io.EOF) {
			return result, errors.New("query: DUAL returned no rows")
		}
		return result, fmt.Errorf("query: iterate DUAL: %w", nextErr)
	}
	if fmt.Sprint(values[0]) != "1" {
		return result, fmt.Errorf("query: DUAL returned %v, want 1", values[0])
	}
	if nextErr := rows.Next(values); nextErr == nil {
		return result, errors.New("query: DUAL returned more than one row")
	} else if !errors.Is(nextErr, io.EOF) {
		return result, fmt.Errorf("query: iterate DUAL: %w", nextErr)
	}
	result.Queried = true
	return result, nil
}

const lifecycleModule = "go-connection-lifecycle-stress"

func newLifecycleRunID() string { return "cnx-" + strings.TrimPrefix(newRetentionRunID(), "ret-") }

func initializeLifecycleSession(ctx context.Context, conn driver.Conn, runID string) error {
	if err := verifyLifecycleTCPS(ctx, conn); err != nil {
		return err
	}
	execer, ok := conn.(driver.ExecerContext)
	if !ok {
		return errors.New("session tagging: connection lacks driver.ExecerContext")
	}
	_, err := execer.ExecContext(ctx, "BEGIN DBMS_SESSION.SET_IDENTIFIER(:run_id); DBMS_APPLICATION_INFO.SET_MODULE(:module, 'workload'); END;", []driver.NamedValue{
		{Name: "run_id", Ordinal: 1, Value: runID}, {Name: "module", Ordinal: 2, Value: lifecycleModule},
	})
	if err != nil {
		return fmt.Errorf("session tagging: %w", err)
	}
	return nil
}

func verifyLifecycleTCPS(ctx context.Context, conn driver.Conn) (err error) {
	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		return errors.New("protocol verification: connection lacks driver.QueryerContext")
	}
	rows, queryErr := queryer.QueryContext(ctx, "SELECT SYS_CONTEXT('USERENV', 'NETWORK_PROTOCOL') FROM DUAL", nil)
	if rows != nil {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				err = errors.Join(err, &lifecycleCleanupError{"protocol verification rows", closeErr})
			}
		}()
	}
	if queryErr != nil {
		return fmt.Errorf("protocol verification: %w", queryErr)
	}
	if rows == nil {
		return errors.New("protocol verification: returned nil rows")
	}
	values := make([]driver.Value, 1)
	if len(rows.Columns()) != 1 {
		return errors.New("protocol verification: expected one column")
	}
	if err = rows.Next(values); err != nil {
		return fmt.Errorf("protocol verification: %w", err)
	}
	if !strings.EqualFold(fmt.Sprint(values[0]), "tcps") {
		return errors.New("protocol verification: server connection is not TCPS")
	}
	if err = rows.Next(values); err != io.EOF {
		if err == nil {
			return errors.New("protocol verification: returned extra rows")
		}
		return fmt.Errorf("protocol verification: %w", err)
	}
	return nil
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
	// Each worker owns one slot. The coordinator reads only after Wait.
	failures := make([]struct {
		cycle int
		err   error
	}, config.Workers)
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
					failure := fmt.Errorf("worker %d cycle %d: %w", worker+1, cycle+1, cycleErr)
					failures[worker].cycle, failures[worker].err = cycle+1, failure
					cancel(failure)
					return
				}
			}
		}(worker)
	}
	began := time.Now()
	admitUntil = began.Add(duration)
	close(start)
	workers.Wait() // Never abandon workers; Go's -timeout bounds uncooperative calls.
	result.Elapsed = time.Since(began)
	result.Counts = progress.snapshot()
	primary := context.Cause(ctx)
	err = primary
	for worker, failure := range failures {
		if failure.err == nil || failure.err == primary {
			continue
		}
		// Our outer worker/cycle wrapper is reapplied once after filtering.
		if additional := lifecycleIndependentError(errors.Unwrap(failure.err), primary); additional != nil {
			err = joinLifecycleErrors(err, fmt.Errorf("worker %d cycle %d: %w", worker+1, failure.cycle, additional))
		}
	}
	if err == nil {
		var expected uint64
		if duration == 0 {
			expected = uint64(config.Workers * config.CyclesPerWorker)
		}
		err = result.Counts.validate(config.Workers, expected)
	}
	// An early failure may spend time draining before this routine returns.
	// Do not claim that elapsed time is a precise admission/drain split.
	if err == nil {
		result.Active = result.Elapsed
		if duration > 0 && result.Active > duration {
			result.Drain = result.Active - duration
			result.Active = duration
		}
	}
	return
}

// A close error is evidence in its own right, even if it wraps cancellation.
type lifecycleCleanupError struct {
	resource string
	err      error
}

func (e *lifecycleCleanupError) Error() string { return fmt.Sprintf("%s close: %v", e.resource, e.err) }
func (e *lifecycleCleanupError) Unwrap() error { return e.err }

// Only known cancellation-only chains are suppressed. A wrapper containing an
// independent error is retained intact, preserving its type and stage context.
func lifecycleCancellationOnly(err, primary error) bool {
	if _, cleanup := err.(*lifecycleCleanupError); cleanup {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !lifecycleCancellationOnly(child, primary) {
				return false
			}
		}
		return true
	}
	child := errors.Unwrap(err)
	if child != nil {
		// Match the cause node itself, not a join merely containing it.
		if primary != nil && errors.Is(err, primary) && !errors.Is(child, primary) {
			return true
		}
		return lifecycleCancellationOnly(child, primary)
	}
	return errors.Is(err, context.Canceled) || (primary != nil && errors.Is(err, primary))
}

func lifecycleIndependentError(err, primary error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var independent error
		for _, child := range joined.Unwrap() {
			independent = joinLifecycleErrors(independent, lifecycleIndependentError(child, primary))
		}
		return independent
	}
	if lifecycleCancellationOnly(err, primary) {
		return nil
	}
	return err
}

// Deduplicate already-contained causes; never discard a mixed error simply
// because one of its children matches the existing error.
func joinLifecycleErrors(first error, more ...error) error {
	for _, next := range more {
		if next == nil || errors.Is(first, next) {
			continue
		}
		if first == nil {
			first = next
		} else {
			first = errors.Join(first, next)
		}
	}
	return first
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
	Warmup, Workload     connectionLifecycleRunResult
	Artifact             string
	Wall                 time.Duration
	Observation          time.Duration
	ObservationCompleted bool
	WorkloadStatus       string
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
		err = joinLifecycleErrors(err, context.Cause(ctx))
		// This record describes the workload, not future file Sync/Close success.
		var active, drain any
		if result.WorkloadStatus == "complete" {
			active, drain = result.Workload.Active.Nanoseconds(), result.Workload.Drain.Nanoseconds()
		}
		err = joinLifecycleErrors(err, encoder.Encode(map[string]any{
			"phase": "result", "run_id": runID, "timestamp_utc": time.Now().UTC(), "workload_status": result.WorkloadStatus,
			"counts": result.Workload.Counts, "warmup_counts": result.Warmup.Counts,
			"workload_elapsed_ns": result.Workload.Elapsed.Nanoseconds(),
			"active_ns":           active, "drain_ns": drain,
			"observation_ns": result.Observation.Nanoseconds(), "observation_completed": result.ObservationCompleted,
			"elapsed_ns": time.Since(began).Nanoseconds(),
			"error":      lifecycleSafeError(err, testConfig),
		}))
		err = joinLifecycleErrors(err, file.Sync(), file.Close())
		if err != nil && config.ProfilesEnabled() {
			err = joinLifecycleErrors(err, writeRetentionProfile(filepath.Join(dir, "failure-goroutines.txt"), "goroutine", 2))
		}
		result.Wall = time.Since(began)
	}()

	configName := ""
	if testConfig != nil {
		configName = testConfig.ConfigName
	}
	executable, executableErr := os.Executable()
	if executableErr != nil {
		executable = "unavailable"
	}
	build, _ := debug.ReadBuildInfo()
	if err = encoder.Encode(map[string]any{
		"phase": "configuration", "run_id": runID, "pid": os.Getpid(), "timestamp_utc": began.UTC(),
		"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "config_name": configName, "profile": config,
		"executable_path": executable, "build": lifecycleBuildMetadata(build),
	}); err != nil {
		return
	}

	warmupConfig := config
	warmupConfig.Duration, warmupConfig.CyclesPerWorker = "", 1
	result.Warmup, err = runConnectionLifecycleWorkers(ctx, warmupConfig, factory, runID, &lifecycleProgress{})
	err = joinLifecycleErrors(err, encoder.Encode(map[string]any{"phase": "warmup", "run_id": runID, "counts": result.Warmup.Counts}))
	if err != nil {
		return
	}

	p := &lifecycleProgress{}
	err = writeLifecycleSample(file, "baseline_natural", runID, began, p)
	err = joinLifecycleErrors(err, writeLifecycleEndpoint(file, "baseline", runID, began, p, config.ProfilesEnabled(), dir))
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
	err = joinLifecycleErrors(err, stopSamples())

	err = joinLifecycleErrors(err, writeLifecycleSample(file, "final_natural", runID, began, p))
	err = joinLifecycleErrors(err, writeLifecycleEndpoint(file, "final", runID, began, p, config.ProfilesEnabled(), dir))
	if err == nil && observeFor > 0 {
		err = encoder.Encode(map[string]any{"phase": "observation_start", "run_id": runID, "timestamp_utc": time.Now().UTC(), "planned_ns": observeFor.Nanoseconds()})
		if err == nil {
			observedAt := time.Now()
			timer := time.NewTimer(observeFor)
			select {
			case <-timer.C:
				result.ObservationCompleted = true
			case <-ctx.Done():
				err = fmt.Errorf("observation: %w", context.Cause(ctx))
			}
			timer.Stop()
			result.Observation = time.Since(observedAt)
		}
	}
	err = joinLifecycleErrors(err, writeLifecycleEndpoint(file, "settled", runID, began, p, config.ProfilesEnabled(), dir))
	return
}

func lifecycleSafeError(err error, config *TestConfig) string {
	if err == nil {
		return ""
	}
	return redactRetentionText(err.Error(), config)
}

// Only report allowlisted data embedded in this executable, never the current
// checkout's revision. Go test binaries commonly have no VCS settings.
func lifecycleBuildMetadata(info *debug.BuildInfo) map[string]any {
	metadata := map[string]any{
		"source": "runtime/debug.ReadBuildInfo", "available": info != nil,
		"vcs_revision": "unavailable", "vcs_time": "unavailable", "vcs_modified": nil,
	}
	if info == nil {
		return metadata
	}
	metadata["package_path"] = info.Path
	metadata["module_path"], metadata["module_version"] = info.Main.Path, info.Main.Version
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if setting.Value != "" {
				metadata["vcs_revision"] = setting.Value
			}
		case "vcs.time":
			if setting.Value != "" {
				metadata["vcs_time"] = setting.Value
			}
		case "vcs.modified":
			if setting.Value == "true" || setting.Value == "false" {
				metadata["vcs_modified"] = setting.Value == "true"
			}
		}
	}
	return metadata
}

// Database-free harness tests. These do not launch functional database workloads.

// Local wrappers add protocol/tagging behavior without changing retention fakes.
type lifecycleSessionFake struct {
	*retentionFakeConn
	protocol                                      []driver.Value
	protocolError, errorOnProtocolClose, tagError error
	tagged                                        bool
}

func (c *lifecycleSessionFake) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "NETWORK_PROTOCOL") {
		if c.protocolError != nil {
			return nil, c.protocolError
		}
		return &retentionFakeRows{values: c.protocol, closeError: c.errorOnProtocolClose, closed: &c.connector.rowsClosed}, nil
	}
	return c.retentionFakeConn.QueryContext(ctx, query, args)
}
func (c *lifecycleSessionFake) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "SET_IDENTIFIER") || len(args) != 2 ||
		!strings.HasPrefix(fmt.Sprint(args[0].Value), "cnx-") || args[1].Value != lifecycleModule {
		return nil, errors.New("bad session identification")
	}
	c.tagged = true
	return driver.RowsAffected(0), c.tagError
}

type lifecycleTestConnector struct{ *retentionFakeConnector }

func (c lifecycleTestConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.retentionFakeConnector.Connect(ctx)
	if conn == nil {
		return nil, err
	}
	return &lifecycleSessionFake{retentionFakeConn: conn.(*retentionFakeConn), protocol: []driver.Value{"tcps"}}, err
}

func lifecycleTestFactory(c *retentionFakeConnector) lifecycleConnectorFactory {
	return func() (driver.Connector, error) { return lifecycleTestConnector{c}, nil }
}

type lifecycleSessionConnector struct {
	conn driver.Conn
	err  error
}

func (c lifecycleSessionConnector) Connect(context.Context) (driver.Conn, error) {
	return c.conn, c.err
}
func (lifecycleSessionConnector) Driver() driver.Driver { return retentionFakeDriver{} }

func TestDirectConnectionCycleUnit(t *testing.T) {
	primary, cleanup := errors.New("operation failed"), errors.New("cleanup failed")
	cases := []struct {
		name, stage string
		configure   func(*retentionFakeConnector)
		wantRows    int64
	}{
		{"success", "", nil, 2},
		{"connect error", "connect", func(c *retentionFakeConnector) { c.connect = func(context.Context, int64) error { return primary } }, 0},
		{"query error", "query", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) { return nil, primary }
		}, 1},
		{"nil rows", "query", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) { return nil, nil }
		}, 1},
		{"no rows", "no rows", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{closed: &c.rowsClosed}, nil
			}
		}, 2},
		{"incorrect value", "want 1", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(2)}, closed: &c.rowsClosed}, nil
			}
		}, 2},
		{"extra row", "more than one", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(1), int64(1)}, closed: &c.rowsClosed}, nil
			}
		}, 2},
		{"iteration error", "iterate DUAL", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(1)}, nextError: primary, closed: &c.rowsClosed}, nil
			}
		}, 2},
		{"rows close error", "rows close", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(1)}, closeError: cleanup, closed: &c.rowsClosed}, nil
			}
		}, 2},
		{"connection close error", "connection close", func(c *retentionFakeConnector) { c.closeError = cleanup }, 2},
		{"query error with rows", "query", func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{closed: &c.rowsClosed}, primary
			}
			c.closeError = cleanup
		}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &retentionFakeConnector{}
			if tc.configure != nil {
				tc.configure(c)
			}
			result, err := executeDirectConnectionCycle(context.Background(), lifecycleTestFactory(c), "cnx-unit")
			if tc.stage == "" {
				if err != nil || !result.Attempted || !result.Connected || !result.Queried || !result.RowsClosed || !result.Closed {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.stage) {
				t.Fatalf("got %v, want %s", err, tc.stage)
			}
			wantClose := int64(1)
			if tc.name == "connect error" {
				wantClose = 0
			}
			if c.closed.Load() != wantClose || c.rowsClosed.Load() != tc.wantRows {
				t.Fatalf("cleanup calls: connections=%d rows=%d", c.closed.Load(), c.rowsClosed.Load())
			}
			if tc.name == "query error with rows" && (!errors.Is(err, primary) || !errors.Is(err, cleanup)) {
				t.Fatalf("lost operation/cleanup error: %v", err)
			}
		})
	}
	for _, kind := range []string{"factory error", "nil connector", "nil connection", "partial connection"} {
		t.Run(kind, func(t *testing.T) {
			c := &retentionFakeConnector{closeError: cleanup}
			factory := lifecycleConnectorFactory(func() (driver.Connector, error) { return nil, primary })
			switch kind {
			case "nil connector":
				factory = func() (driver.Connector, error) { return nil, nil }
			case "nil connection":
				factory = func() (driver.Connector, error) { return lifecycleSessionConnector{}, nil }
			case "partial connection":
				factory = func() (driver.Connector, error) {
					return lifecycleSessionConnector{&retentionFakeConn{connector: c}, primary}, nil
				}
			}
			result, err := executeDirectConnectionCycle(context.Background(), factory, "cnx-unit")
			if err == nil || result.Connected {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if kind == "partial connection" && (!errors.Is(err, primary) || !errors.Is(err, cleanup) || c.closed.Load() != 1) {
				t.Fatalf("partial cleanup lost: %v", err)
			}
		})
	}
}

func lifecycleUnitConfig() oracleTest.ConnectionLifecycleConfig {
	return (oracleTest.ConnectionLifecycleConfig{Workers: 4, CyclesPerWorker: 5, Timeout: "5s"}).Normalized()
}

type lifecycleUnitQueryError struct{ message string }

func (e *lifecycleUnitQueryError) Error() string { return e.message }

func TestConnectionLifecycleWorkersUnit(t *testing.T) {
	t.Run("fresh connector and same connection query", func(t *testing.T) {
		var mu sync.Mutex
		var connectors []*retentionFakeConnector
		factory := func() (driver.Connector, error) {
			c := &retentionFakeConnector{}
			mu.Lock()
			connectors = append(connectors, c)
			mu.Unlock()
			return lifecycleTestConnector{c}, nil
		}
		result, err := runConnectionLifecycleWorkers(context.Background(), lifecycleUnitConfig(), factory, "cnx-unit", &lifecycleProgress{})
		if err != nil || result.Counts.CompletedCycles != 20 || len(connectors) != 20 {
			t.Fatalf("result=%+v factories=%d err=%v", result, len(connectors), err)
		}
		for _, c := range connectors {
			if c.opened.Load() != 1 || c.closed.Load() != 1 || c.queries.Load() != 1 || c.rowsClosed.Load() != 2 {
				t.Fatal("connector reused, query on wrong connection, or cleanup missing")
			}
		}
	})
	t.Run("first error cancels peers and joins workers", func(t *testing.T) {
		primary := errors.New("injected query error")
		entered := make(chan struct{})
		c := &retentionFakeConnector{
			connect: func(ctx context.Context, number int64) error {
				if number == 4 {
					close(entered)
				}
				if number == 1 {
					select {
					case <-entered:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				<-ctx.Done()
				return ctx.Err()
			},
			query: func(context.Context, int64) (driver.Rows, error) { return nil, primary },
		}
		result, err := runConnectionLifecycleWorkers(context.Background(), lifecycleUnitConfig(), lifecycleTestFactory(c), "cnx-unit", &lifecycleProgress{})
		if !errors.Is(err, primary) || !strings.Contains(err.Error(), "worker ") || !strings.Contains(err.Error(), "cycle ") || !strings.Contains(err.Error(), "query") {
			t.Fatalf("missing original worker/cycle/stage: %v", err)
		}
		if result.Counts.WorkersFinished != 4 || result.Counts.FailedCycles == 0 || result.Counts.SuccessfulConnections != result.Counts.ConnectionCloses {
			t.Fatalf("cleanup/join: %+v", result.Counts)
		}
	})
	t.Run("duration drains healthy cycle", func(t *testing.T) {
		config := lifecycleUnitConfig()
		config.Workers, config.CyclesPerWorker, config.Duration = 1, 0, "20ms"
		config.Timeout = "20m"
		started, release := make(chan struct{}), make(chan struct{})
		c := &retentionFakeConnector{}
		c.query = func(ctx context.Context, _ int64) (driver.Rows, error) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &retentionFakeRows{values: []driver.Value{int64(1)}, closed: &c.rowsClosed}, nil
		}
		var result connectionLifecycleRunResult
		var err error
		done := make(chan struct{})
		go func() {
			defer close(done)
			result, err = runConnectionLifecycleWorkers(context.Background(), config, lifecycleTestFactory(c), "cnx-unit", &lifecycleProgress{})
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("cycle did not start")
		}
		time.Sleep(40 * time.Millisecond)
		close(release)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("worker did not drain")
		}
		if err != nil || result.Counts.CompletedCycles != 1 || result.Drain <= 0 {
			t.Fatalf("healthy cycle cancelled: %+v %v", result, err)
		}
	})
	for _, kind := range []string{"operation", "overall"} {
		t.Run(kind+" deadline", func(t *testing.T) {
			config := lifecycleUnitConfig()
			config.Workers, config.OperationTimeout = 1, "10ms"
			ctx := context.Background()
			if kind == "overall" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
				config.OperationTimeout = "2s"
			}
			c := &retentionFakeConnector{connect: func(ctx context.Context, _ int64) error { <-ctx.Done(); return ctx.Err() }}
			result, err := runConnectionLifecycleWorkers(ctx, config, lifecycleTestFactory(c), "cnx-unit", &lifecycleProgress{})
			if !errors.Is(err, context.DeadlineExceeded) || result.Counts.WorkersFinished != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
	t.Run("bounded independent errors after primary cancellation", func(t *testing.T) {
		config := lifecycleUnitConfig()
		primary := errors.New("primary query failed")
		independent := &lifecycleUnitQueryError{"independent query failed"}
		rowClose, connClose := errors.New("independent row close failed"), errors.New("independent connection close failed")
		var mu sync.Mutex
		created, entered := 0, 0
		allEntered := make(chan struct{})
		var connectors []*retentionFakeConnector
		factory := func() (driver.Connector, error) {
			mu.Lock()
			created++
			number := created
			c := &retentionFakeConnector{}
			connectors = append(connectors, c)
			mu.Unlock()
			c.query = func(ctx context.Context, _ int64) (driver.Rows, error) {
				mu.Lock()
				entered++
				if entered == config.Workers {
					close(allEntered)
				}
				mu.Unlock()
				if number == 1 {
					select {
					case <-allEntered:
						return nil, primary
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				<-ctx.Done()
				switch number {
				case 2:
					return &retentionFakeRows{closed: &c.rowsClosed, closeError: rowClose}, independent
				case 3:
					// A real transport path can wrap the original cancellation
					// cause, not merely context.Canceled.
					return nil, fmt.Errorf("transport read: %w", context.Cause(ctx))
				default:
					return nil, ctx.Err()
				}
			}
			if number == 2 {
				c.closeError = connClose
			}
			if number == 3 {
				c.closeError = context.Canceled
			}
			return lifecycleTestConnector{c}, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		result, err := runConnectionLifecycleWorkers(ctx, config, factory, "cnx-unit", &lifecycleProgress{})
		var queryDetail *lifecycleUnitQueryError
		if !errors.Is(err, primary) || !errors.As(err, &queryDetail) || queryDetail != independent ||
			!errors.Is(err, rowClose) || !errors.Is(err, connClose) || !errors.Is(err, context.Canceled) {
			t.Fatalf("lost primary, independent, or cleanup failure: %v", err)
		}
		if strings.Count(err.Error(), primary.Error()) != 1 || !strings.Contains(err.Error(), "connection close: context canceled") {
			t.Fatalf("nested primary cause repeated or cancellation close lost: %v", err)
		}
		if created != 4 || result.Counts.CyclesStarted != 4 || result.Counts.FailedCycles != 4 ||
			result.Counts.WorkersFinished != 4 || result.Active != 0 || result.Drain != 0 || result.Elapsed <= 0 {
			t.Fatalf("unbounded work, lost joins, or misleading failure timing: %+v", result)
		}
		// The primary record is first. Additional workers are in stable index
		// order, with no extra record for the cancellation-only fourth worker.
		last, records := 0, 0
		for _, line := range strings.Split(err.Error(), "\n") {
			var worker, cycle int
			if n, _ := fmt.Sscanf(line, "worker %d cycle %d:", &worker, &cycle); n == 2 {
				if records > 0 && worker <= last {
					t.Fatalf("additional workers out of order: %v", err)
				}
				if records > 0 {
					last = worker
				}
				records++
			}
		}
		if records != 3 {
			t.Fatalf("want primary plus two independent worker records: %v", err)
		}
		for _, c := range connectors {
			if c.closed.Load() != 1 {
				t.Fatal("worker connection was not closed")
			}
		}
	})
	t.Run("coordinator error dedup preserves mixed errors", func(t *testing.T) {
		writeErr, cleanup := errors.New("sample failed"), errors.New("cleanup failed")
		cause := fmt.Errorf("runtime sample: %w", writeErr)
		combined := joinLifecycleErrors(cause, writeErr, cause)
		if combined != cause {
			t.Fatalf("repeated contained cause: %v", combined)
		}
		combined = joinLifecycleErrors(combined, errors.Join(cause, cleanup))
		if !errors.Is(combined, cleanup) {
			t.Fatal("mixed error lost cleanup")
		}
		// Filtering a joined cause must retain independent close errors,
		// including close errors whose underlying value is cancellation.
		closeErr := &lifecycleCleanupError{"rows", context.Canceled}
		extra := lifecycleIndependentError(errors.Join(fmt.Errorf("query: %w", cause), closeErr, cleanup), cause)
		if !errors.Is(extra, closeErr) || !errors.Is(extra, cleanup) || strings.Contains(extra.Error(), writeErr.Error()) {
			t.Fatalf("mixed cancellation filtering lost evidence: %v", extra)
		}
	})
	t.Run("aggregate only returned cycles", func(t *testing.T) {
		p := &lifecycleProgress{}
		p.start()
		if c := p.snapshot(); c.CyclesStarted != 1 || c.ConnectionAttempts != 0 {
			t.Fatal(c)
		}
		p.add(directConnectionCycle{Attempted: true, Connected: true, Closed: true}, errors.New("query failed"))
		p.finished()
		c := p.snapshot()
		if c.ConnectionAttempts != 1 || c.SuccessfulConnections != 1 || c.ConnectionCloses != 1 || c.FailedCycles != 1 || c.CompletedCycles != 0 {
			t.Fatal(c)
		}
		if c.validate(1, 1) == nil {
			t.Fatal("failed cycle passed validation")
		}
	})
}

func TestConnectionLifecycleSessionUnit(t *testing.T) {
	for _, kind := range []string{"success", "tcp", "empty", "extra", "protocol query", "protocol close", "tag"} {
		t.Run(kind, func(t *testing.T) {
			primary, cleanup := errors.New("initialization failed"), errors.New("connection cleanup failed")
			c := &retentionFakeConnector{}
			conn := &lifecycleSessionFake{retentionFakeConn: &retentionFakeConn{connector: c}, protocol: []driver.Value{"tcps"}}
			switch kind {
			case "tcp":
				conn.protocol = []driver.Value{"tcp"}
			case "empty":
				conn.protocol = nil
			case "extra":
				conn.protocol = []driver.Value{"tcps", "tcps"}
			case "protocol query":
				conn.protocolError = primary
			case "protocol close":
				conn.errorOnProtocolClose = primary
			case "tag":
				conn.tagError, c.closeError = primary, cleanup
			}
			result, err := executeDirectConnectionCycle(context.Background(), func() (driver.Connector, error) { return lifecycleSessionConnector{conn: conn}, nil }, "cnx-unit")
			if (kind == "success") != (err == nil) || !result.Connected || c.closed.Load() != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if kind == "success" && (!conn.tagged || !result.Queried || !result.RowsClosed || c.rowsClosed.Load() != 2) {
				t.Fatal("initialization/query cleanup missing")
			}
			if kind != "success" && (result.Queried || c.queries.Load() != 0) {
				t.Fatal("query ran after failed initialization")
			}
			if kind == "tag" && (!errors.Is(err, primary) || !errors.Is(err, cleanup)) {
				t.Fatalf("lost initialization/cleanup error: %v", err)
			}
		})
	}
}

type lifecycleFailWriter struct{}

func (lifecycleFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestConnectionLifecycleMeasurementsUnit(t *testing.T) {

	t.Run("embedded build provenance only", func(t *testing.T) {
		missing := lifecycleBuildMetadata(nil)
		if missing["available"] != false || missing["vcs_revision"] != "unavailable" || missing["vcs_modified"] != nil {
			t.Fatalf("unavailable provenance invented: %v", missing)
		}
		info := &debug.BuildInfo{Path: "oracle.test", Main: debug.Module{Path: "example/driver", Version: "(devel)"}}
		unstamped := lifecycleBuildMetadata(info)
		if unstamped["available"] != true || unstamped["vcs_revision"] != "unavailable" || unstamped["module_version"] != "(devel)" {
			t.Fatalf("unstamped binary attributed to checkout: %v", unstamped)
		}
		info.Settings = []debug.BuildSetting{
			{Key: "vcs.revision", Value: "embedded-binary-revision"},
			{Key: "vcs.time", Value: "2026-10-29T00:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
			{Key: "-ldflags", Value: "DO-NOT-RECORD-THIS"},
		}
		stamped := lifecycleBuildMetadata(info)
		data, err := json.Marshal(stamped)
		if err != nil || stamped["source"] != "runtime/debug.ReadBuildInfo" || stamped["vcs_revision"] != "embedded-binary-revision" ||
			stamped["vcs_modified"] != true || bytes.Contains(data, []byte("DO-NOT-RECORD-THIS")) {
			t.Fatalf("incorrect or excessive build metadata: %v err=%v", stamped, err)
		}
	})
	t.Run("observation failure does not undo completed workload", func(t *testing.T) {
		config := lifecycleUnitConfig()
		config.Workers, config.CyclesPerWorker, config.OutputDirectory = 1, 1, t.TempDir()
		profiles := false
		config.DiagnosticProfiles = &profiles
		config, err := resolveLifecycleConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		cause := errors.New("observation interrupted")
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		var result lifecycleStressResult
		var runErr error
		done := make(chan struct{})
		go func() {
			defer close(done)
			result, runErr = executeConnectionLifecycleStress(ctx, config, "cnx-observation", nil, lifecycleTestFactory(&retentionFakeConnector{}), time.Minute)
		}()
		// Cancel only after the drained observation begins, not during workers
		// or their final endpoint. This tests outcome separation, not timing.
		ticker, deadline := time.NewTicker(time.Millisecond), time.NewTimer(3*time.Second)
		defer ticker.Stop()
		defer deadline.Stop()
		waiting := true
		for waiting {
			select {
			case <-ticker.C:
				data, _ := os.ReadFile(filepath.Join(config.OutputDirectory, "cnx-observation", "samples.jsonl"))
				if bytes.Contains(data, []byte("\"phase\":\"observation_start\"")) {
					cancel(cause)
					waiting = false
				}
			case <-deadline.C:
				cancel(errors.New("test observation did not start"))
				<-done
				t.Fatal("observation did not start")
			}
		}
		<-done
		if !errors.Is(runErr, cause) || !strings.Contains(runErr.Error(), "observation:") ||
			strings.Count(runErr.Error(), cause.Error()) != 1 || result.WorkloadStatus != "complete" ||
			result.Workload.Counts.CompletedCycles != 1 || result.ObservationCompleted || result.Observation <= 0 {
			t.Fatalf("workload and observation outcomes confused: %+v %v", result, runErr)
		}
		data, err := os.ReadFile(result.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		var record map[string]any
		if err := json.Unmarshal(lines[len(lines)-1], &record); err != nil {
			t.Fatal(err)
		}
		if record["workload_status"] != "complete" || record["observation_completed"] != false ||
			record["error"] == "" || record["active_ns"] == nil || record["wall_ns"] != nil {
			t.Fatalf("misleading final record: %v", record)
		}
		if float64(result.Wall.Nanoseconds()) < record["elapsed_ns"].(float64) {
			t.Fatal("wall excludes final cleanup")
		}
	})
	t.Run("reporting failure after completed workload", func(t *testing.T) {
		config := lifecycleUnitConfig()
		config.Workers, config.CyclesPerWorker, config.OutputDirectory = 1, 1, t.TempDir()
		config, err := resolveLifecycleConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		c := &retentionFakeConnector{}
		var hookErr error
		c.onClose = func() {
			if c.closed.Load() == 2 { // Warm-up is separate; only measured close injects a collision.
				hookErr = os.WriteFile(filepath.Join(config.OutputDirectory, "cnx-reporting", "final-heap.pprof"), []byte("existing evidence"), 0600)
			}
		}
		result, runErr := executeConnectionLifecycleStress(context.Background(), config, "cnx-reporting", nil, lifecycleTestFactory(c), time.Minute)
		if hookErr != nil {
			t.Fatal(hookErr)
		}
		if !errors.Is(runErr, os.ErrExist) || result.WorkloadStatus != "complete" ||
			result.Workload.Counts.CompletedCycles != 1 || result.ObservationCompleted || result.Observation != 0 {
			t.Fatalf("recording failure overridden by workload success: %+v %v", result, runErr)
		}
	})
	t.Run("failed workload timing is not an admission estimate", func(t *testing.T) {
		config := lifecycleUnitConfig()
		config.Workers, config.CyclesPerWorker, config.OutputDirectory = 1, 1, t.TempDir()
		profiles := false
		config.DiagnosticProfiles = &profiles
		config, err := resolveLifecycleConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		primary := errors.New("measured connection failed")
		c := &retentionFakeConnector{connect: func(_ context.Context, number int64) error {
			if number > 1 {
				return primary
			}
			return nil
		}}
		result, runErr := executeConnectionLifecycleStress(context.Background(), config, "cnx-timing", nil, lifecycleTestFactory(c), 0)
		if !errors.Is(runErr, primary) || result.WorkloadStatus != "failed" || result.Workload.Elapsed <= 0 {
			t.Fatalf("%+v %v", result, runErr)
		}
		data, err := os.ReadFile(result.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		var record map[string]any
		if err := json.Unmarshal(lines[len(lines)-1], &record); err != nil {
			t.Fatal(err)
		}
		if record["active_ns"] != nil || record["drain_ns"] != nil || record["workload_elapsed_ns"].(float64) <= 0 {
			t.Fatalf("invented failed-run split: %v", record)
		}
	})

	t.Run("samples and failed sampler", func(t *testing.T) {
		var out bytes.Buffer
		p := &lifecycleProgress{}
		if err := writeLifecycleSample(&out, "natural", "cnx-unit", time.Now(), p); err != nil {
			t.Fatal(err)
		}
		var sample lifecycleSample
		if err := json.Unmarshal(out.Bytes(), &sample); err != nil || sample.Phase != "natural" || sample.RunID != "cnx-unit" || sample.PID != os.Getpid() {
			t.Fatalf("sample=%+v err=%v", sample, err)
		}
		if strings.Contains(out.String(), "delta") || strings.Contains(out.String(), "pool") {
			t.Fatal("obsolete fields in sample")
		}
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		stop := startLifecycleSampler(ctx, time.Millisecond, lifecycleFailWriter{}, "cnx-unit", time.Now(), p, cancel)
		c := &retentionFakeConnector{connect: func(ctx context.Context, _ int64) error { <-ctx.Done(); return ctx.Err() }}
		result, err := runConnectionLifecycleWorkers(ctx, lifecycleUnitConfig(), lifecycleTestFactory(c), "cnx-unit", p)
		if !errors.Is(err, io.ErrClosedPipe) || !errors.Is(stop(), io.ErrClosedPipe) || result.Counts.WorkersFinished != 4 {
			t.Fatalf("sampler failure did not stop/join workers: %+v %v", result, err)
		}
	})
	t.Run("warmup output profiles and paths", func(t *testing.T) {
		for _, profiles := range []bool{false, true} {
			config := lifecycleUnitConfig()
			config.Workers, config.CyclesPerWorker, config.Timeout, config.SampleInterval = 2, 3, "15s", "1ms"
			cwd, _ := os.Getwd()
			dir := t.TempDir()
			relative, err := filepath.Rel(cwd, dir)
			if err != nil {
				t.Fatal(err)
			}
			config.OutputDirectory, config.DiagnosticProfiles = relative, &profiles
			resolved, err := resolveLifecycleConfig(config)
			if err != nil || resolved.OutputDirectory != dir || config.OutputDirectory != relative {
				t.Fatalf("path changed source: %+v %v", resolved, err)
			}
			factory := func() (driver.Connector, error) {
				return lifecycleTestConnector{&retentionFakeConnector{connect: func(ctx context.Context, _ int64) error {
					select {
					case <-time.After(3 * time.Millisecond):
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}}}, nil
			}
			result, err := executeConnectionLifecycleStress(context.Background(), resolved, "cnx-artifact", nil, factory, time.Millisecond)
			if err != nil || result.Warmup.Counts.CompletedCycles != 2 || result.Workload.Counts.CompletedCycles != 6 || result.WorkloadStatus != "complete" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if result.Artifact != filepath.Join(dir, "cnx-artifact", "samples.jsonl") {
				t.Fatal("wrong artifact path")
			}
			data, err := os.ReadFile(result.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			phases := map[string]int{}
			var order []string
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
				var record map[string]any
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatal(err)
				}
				phase := record["phase"].(string)
				phases[phase]++
				order = append(order, phase)
				if phase == "configuration" {
					profile := record["profile"].(map[string]any)
					if profile["output_directory"] != dir {
						t.Fatal("metadata path mismatch")
					}
				}
				if phase == "result" && (record["workload_status"] != "complete" || record["artifact_status"] != nil || record["result"] != nil) {
					t.Fatal("invalid/obsolete final record")
				}
			}
			for _, phase := range []string{"configuration", "warmup", "baseline_natural", "baseline_post_gc", "natural", "final_natural", "final_post_gc", "observation_start", "settled_post_gc", "result"} {
				if phases[phase] == 0 {
					t.Fatalf("missing phase %s", phase)
				}
			}
			finalSeen := false
			for _, phase := range order {
				if phase == "final_natural" {
					finalSeen = true
				}
				if phase == "natural" && finalSeen {
					t.Fatal("sampler wrote after final endpoint")
				}
			}
			for _, name := range []string{"baseline-heap.pprof", "final-heap.pprof", "settled-heap.pprof"} {
				data, err := os.ReadFile(filepath.Join(filepath.Dir(result.Artifact), name))
				if profiles {
					if err != nil || len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
						t.Fatalf("invalid %s: %v", name, err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf("disabled profile %s exists", name)
				}
			}
			if _, err := executeConnectionLifecycleStress(context.Background(), resolved, "cnx-artifact", nil, factory, 0); err == nil {
				t.Fatal("overwrote evidence")
			}
		}
	})
	t.Run("warmup failure evidence and redaction", func(t *testing.T) {
		config := lifecycleUnitConfig()
		config.OutputDirectory = t.TempDir()
		config, err := resolveLifecycleConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		primary := errors.New("secret-test-password /secret/wallet")
		testConfig := &TestConfig{}
		testConfig.Credentials.Password, testConfig.Security.WalletLocation = "secret-test-password", "/secret/wallet"
		c := &retentionFakeConnector{connect: func(context.Context, int64) error { return primary }}
		result, err := executeConnectionLifecycleStress(context.Background(), config, "cnx-failure", testConfig, lifecycleTestFactory(c), time.Minute)
		if !errors.Is(err, primary) || result.WorkloadStatus != "not_started" || result.Workload.Counts.CyclesStarted != 0 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		data, readErr := os.ReadFile(result.Artifact)
		if readErr != nil || bytes.Contains(data, []byte("secret-test-password")) || bytes.Contains(data, []byte("/secret/wallet")) {
			t.Fatalf("unsafe/missing saved error: %v", readErr)
		}
		data, readErr = os.ReadFile(filepath.Join(filepath.Dir(result.Artifact), "failure-goroutines.txt"))
		if readErr != nil || !bytes.Contains(data, []byte("goroutine")) {
			t.Fatalf("missing failure dump: %v", readErr)
		}
		if strings.Contains(lifecycleSafeError(primary, testConfig), "secret-test-password") {
			t.Fatal("unsafe terminal error")
		}
	})
}
