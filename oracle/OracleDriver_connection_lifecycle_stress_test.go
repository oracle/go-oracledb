/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package oracle

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

type connectionLifecycleWorkerResult struct {
	Attempts int
	Queries  int
	Closes   int
}

// runDriverConnectionLifecycleStress is invoked only by TestCategoryExecutor.
// Keeping it non-Test-named prevents ordinary `go test ./...` from discovering
// a long-running manual workload. A profile runs one selected load level.
func runDriverConnectionLifecycleStress(t *testing.T) {
	if TestingConfig == nil || TestingConfig.Stress == nil || TestingConfig.Stress.ConnectionLifecycle == nil {
		t.Skip("no stress.connection_lifecycle profile in the selected JSON configuration; stress workload did not run")
	}
	config := *TestingConfig.Stress.ConnectionLifecycle
	if err := config.Validate(); err != nil {
		t.Fatalf("stress.connection_lifecycle: %v", err)
	}
	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	result, err := runConnectionLifecycleWorkers(config, connector)
	expected := config.Workers * config.CyclesPerWorker
	t.Logf("connection_lifecycle workers=%d cycles_per_worker=%d expected=%d attempts=%d queries=%d closes=%d duration=%s",
		config.Workers, config.CyclesPerWorker, expected, result.Attempts, result.Queries, result.Closes, result.Duration)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempts != expected || result.Queries != expected || result.Closes != expected {
		t.Fatalf("incomplete connection lifecycles: expected=%d attempts=%d queries=%d closes=%d",
			expected, result.Attempts, result.Queries, result.Closes)
	}
}

type connectionLifecycleRunResult struct {
	connectionLifecycleWorkerResult
	Duration time.Duration
}

// runConnectionLifecycleWorkers synchronizes the first attempt, then lets each
// worker repeat the same validated cycle independently. All workers report
// before the function returns, even after one worker fails and cancels peers.
func runConnectionLifecycleWorkers(config oracleTest.ConnectionLifecycleConfig, connector driver.Connector) (connectionLifecycleRunResult, error) {
	if err := config.Validate(); err != nil {
		return connectionLifecycleRunResult{}, err
	}
	timeout, _ := time.ParseDuration(config.Timeout) // validated above
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	ready := make(chan struct{}, config.Workers)
	start := make(chan struct{})
	reports := make(chan connectionLifecycleWorkerResult, config.Workers)
	var firstErr error
	var firstErrOnce sync.Once
	recordFailure := func(err error) {
		firstErrOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	for worker := 0; worker < config.Workers; worker++ {
		go func(worker int) {
			result := connectionLifecycleWorkerResult{}
			defer func() { reports <- result }()
			ready <- struct{}{}
			select {
			case <-start:
			case <-ctx.Done():
				return
			}
			for cycle := 0; cycle < config.CyclesPerWorker; cycle++ {
				if err := ctx.Err(); err != nil {
					recordFailure(fmt.Errorf("worker %d cycle %d: workload timeout or cancellation: %w", worker, cycle, err))
					return
				}
				result.Attempts++
				cycleResult, err := executeDirectConnectionCycle(ctx, connector)
				if cycleResult.Queried {
					result.Queries++
				}
				if cycleResult.Closed {
					result.Closes++
				}
				if err != nil {
					recordFailure(fmt.Errorf("worker %d cycle %d: %w", worker, cycle, err))
					return
				}
			}
		}(worker)
	}

	// A readiness barrier makes the first physical connection attempts overlap.
	// It is not a second workload or a synchronized burst before every cycle.
	for worker := 0; worker < config.Workers; worker++ {
		select {
		case <-ready:
		case <-ctx.Done():
			recordFailure(fmt.Errorf("workload timed out before all workers were ready: %w", ctx.Err()))
			close(start)
			return collectConnectionLifecycleReports(config.Workers, reports, started, &firstErr)
		}
	}
	close(start)
	return collectConnectionLifecycleReports(config.Workers, reports, started, &firstErr)
}

func collectConnectionLifecycleReports(workers int, reports <-chan connectionLifecycleWorkerResult, started time.Time, firstErr *error) (connectionLifecycleRunResult, error) {
	result := connectionLifecycleRunResult{}
	for worker := 0; worker < workers; worker++ {
		report := <-reports
		result.Attempts += report.Attempts
		result.Queries += report.Queries
		result.Closes += report.Closes
	}
	result.Duration = time.Since(started)
	return result, *firstErr
}

func TestConnectionLifecycleWorkersUnit(t *testing.T) {
	config := oracleTest.ConnectionLifecycleConfig{Workers: 4, CyclesPerWorker: 5, Timeout: "2s"}
	connector := &retentionFakeConnector{}
	result, err := runConnectionLifecycleWorkers(config, connector)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempts != 20 || result.Queries != 20 || result.Closes != 20 || connector.opened.Load() != 20 || connector.closed.Load() != 20 || connector.rowsClosed.Load() != 20 {
		t.Fatalf("unexpected complete-run counts: result=%+v opened=%d closed=%d rows_closed=%d",
			result, connector.opened.Load(), connector.closed.Load(), connector.rowsClosed.Load())
	}

	connector = &retentionFakeConnector{}
	connector.connect = func(context.Context, int64) error { return errors.New("injected connect error") }
	result, err = runConnectionLifecycleWorkers(config, connector)
	if err == nil || !strings.Contains(err.Error(), "connect:") || !strings.Contains(err.Error(), "worker ") || !strings.Contains(err.Error(), "cycle ") {
		t.Fatalf("got %v, want worker/cycle/stage failure", err)
	}
	if result.Closes != 0 || connector.closed.Load() != 0 {
		t.Fatalf("failed connects should not produce connection closes: result=%+v closed=%d", result, connector.closed.Load())
	}
}
