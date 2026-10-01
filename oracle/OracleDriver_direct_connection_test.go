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
	"io"
	"strings"
	"testing"
	"time"
)

// directConnectionCycle is one physical connection from establishment through
// query and closure. Its counters describe client actions, not server sessions.
type directConnectionCycle struct {
	Connected bool
	Queried   bool
	Closed    bool
}

// executeDirectConnectionCycle is shared by the functional baseline and the
// concurrent stress workload. Unlike a database/sql handle, one Connect call
// here corresponds to exactly one driver connection.
func executeDirectConnectionCycle(ctx context.Context, connector driver.Connector) (result directConnectionCycle, err error) {
	conn, connectErr := connector.Connect(ctx)
	if connectErr != nil {
		return result, fmt.Errorf("connect: %w", connectErr)
	}
	if conn == nil {
		return result, errors.New("connect: returned a nil connection without an error")
	}
	result.Connected = true
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("connection close: %w", closeErr))
		} else {
			result.Closed = true
		}
	}()

	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		return result, errors.New("query: connection does not implement driver.QueryerContext")
	}
	rows, queryErr := queryer.QueryContext(ctx, "SELECT 1 FROM DUAL", nil)
	if queryErr != nil {
		return result, fmt.Errorf("query: %w", queryErr)
	}
	if rows == nil {
		return result, errors.New("query: returned nil rows without an error")
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("rows close: %w", closeErr))
		} else if err == nil {
			result.Queried = true
		}
	}()

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
	return result, nil
}

// TestDriver_DirectConnectionLifecycle proves the exact operation that the
// stress test repeats, before concurrency or volume complicate a failure.
func TestDriver_DirectConnectionLifecycle(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := executeDirectConnectionCycle(ctx, connector)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Connected || !result.Queried || !result.Closed {
		t.Fatalf("incomplete direct connection lifecycle: %+v", result)
	}
}

func TestDirectConnectionCycleUnit(t *testing.T) {
	connectError := errors.New("connect failed")
	queryError := errors.New("query failed")
	iterateError := errors.New("iterate failed")
	rowsCloseError := errors.New("rows close failed")
	connCloseError := errors.New("connection close failed")
	cases := []struct {
		name       string
		configure  func(*retentionFakeConnector)
		wantError  string
		wantClosed int64
		wantRows   int64
	}{
		{name: "success", wantClosed: 1, wantRows: 1},
		{name: "connect error", configure: func(c *retentionFakeConnector) {
			c.connect = func(context.Context, int64) error { return connectError }
		}, wantError: "connect", wantClosed: 0},
		{name: "query error", configure: func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) { return nil, queryError }
		}, wantError: "query", wantClosed: 1},
		{name: "no rows", configure: func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{closed: &c.rowsClosed}, nil
			}
		}, wantError: "no rows", wantClosed: 1, wantRows: 1},
		{name: "incorrect value", configure: func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(2)}, closed: &c.rowsClosed}, nil
			}
		}, wantError: "want 1", wantClosed: 1, wantRows: 1},
		{name: "extra row", configure: func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(1), int64(1)}, closed: &c.rowsClosed}, nil
			}
		}, wantError: "more than one", wantClosed: 1, wantRows: 1},
		{name: "iteration error", configure: func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(1)}, nextError: iterateError, closed: &c.rowsClosed}, nil
			}
		}, wantError: "iterate DUAL", wantClosed: 1, wantRows: 1},
		{name: "rows close error", configure: func(c *retentionFakeConnector) {
			c.query = func(context.Context, int64) (driver.Rows, error) {
				return &retentionFakeRows{values: []driver.Value{int64(1)}, closeError: rowsCloseError, closed: &c.rowsClosed}, nil
			}
		}, wantError: "rows close", wantClosed: 1, wantRows: 1},
		{name: "connection close error", configure: func(c *retentionFakeConnector) {
			c.closeError = connCloseError
		}, wantError: "connection close", wantClosed: 1, wantRows: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector := &retentionFakeConnector{}
			if tc.configure != nil {
				tc.configure(connector)
			}
			result, err := executeDirectConnectionCycle(context.Background(), connector)
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("got error %v, want %q", err, tc.wantError)
			}
			if got := connector.closed.Load(); got != tc.wantClosed {
				t.Errorf("connection close calls = %d, want %d", got, tc.wantClosed)
			}
			if got := connector.rowsClosed.Load(); got != tc.wantRows {
				t.Errorf("rows close calls = %d, want %d", got, tc.wantRows)
			}
			if tc.wantError == "" && (!result.Connected || !result.Queried || !result.Closed) {
				t.Errorf("incomplete successful lifecycle: %+v", result)
			}
		})
	}
}
