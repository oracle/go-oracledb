/*
** Copyright (c) 2026 Oracle and/or its affiliates.
** The Universal Permissive License (UPL), Version 1.0
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
	"strings"
	"sync"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

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
				err = errors.Join(err, fmt.Errorf("connection close: %w", closeErr))
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
				err = errors.Join(err, fmt.Errorf("rows close: %w", closeErr))
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
				err = errors.Join(err, fmt.Errorf("protocol verification rows close: %w", closeErr))
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

// The functional baseline executes the same operation as every stress cycle.
func TestDriver_DirectConnectionLifecycle(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	if !strings.EqualFold(strings.TrimSpace(TestingConfig.Database.Protocol), "tcps") {
		t.Skip("direct TCPS lifecycle baseline requires a TCPS profile; no TCPS coverage ran")
	}
	if err := validateRetentionTLS(TestingConfig); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := executeDirectConnectionCycle(ctx, func() (driver.Connector, error) {
		return openTestConnectorWithConfig(TestingConfig)
	}, newLifecycleRunID())
	if err != nil {
		t.Fatal(lifecycleSafeError(err, TestingConfig))
	}
	if !result.Attempted || !result.Connected || !result.Queried || !result.RowsClosed || !result.Closed {
		t.Fatalf("incomplete lifecycle: %+v", result)
	}
}

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
