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
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math/big"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	testObjectNamePrefix = "godrivertest"
	testObjectRandomMin  = 100000
	testObjectRandomMax  = 999999
)

// selectDualQueryer is implemented by both *sql.DB and *sql.Conn. The same
// validated operation is used by functional, cursor-leak, stress, and benchmark
// coverage rather than calling whole tests from worker goroutines.
type selectDualQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func executeSelectDual(ctx context.Context, queryer selectDualQueryer) (err error) {
	rows, err := queryer.QueryContext(ctx, "SELECT 1 FROM DUAL")
	if err != nil {
		return fmt.Errorf("query DUAL: %w", err)
	}
	// This defer belongs to one operation, not the caller's high-volume loop.
	// It closes rows before the helper returns, including validation failures.
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close DUAL rows: %w", closeErr))
		}
	}()
	count := 0
	for rows.Next() {
		count++
		if count > 1 {
			return fmt.Errorf("DUAL returned more than one row")
		}
		var value int
		if err := rows.Scan(&value); err != nil {
			return fmt.Errorf("scan DUAL: %w", err)
		}
		if value != 1 {
			return fmt.Errorf("DUAL returned %d, want 1", value)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate DUAL: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("DUAL returned no rows")
	}
	return nil
}

// createObjectName returns an Oracle identifier unique to this test run. The
// logical name keeps failed-test artifacts identifiable while the date and
// random suffix prevent collisions between concurrent test runs.
func createObjectName(logicalName string) string {
	randomRange := big.NewInt(testObjectRandomMax - testObjectRandomMin + 1)
	randomNumber, _ := rand.Int(rand.Reader, randomRange)

	return fmt.Sprintf("%s_%s_%s_%06d",
		testObjectNamePrefix,
		time.Now().Format("060102"),
		logicalName,
		randomNumber.Int64()+testObjectRandomMin,
	)
}

// openTestConnectorWithConfig opens a test database connector using the provided test configuration
func openTestConnectorWithConfig(cfg *TestConfig) (driver.Connector, error) {
	if cfg == nil {
		return nil, sql.ErrConnDone
	}
	dsn := cfg.GetConnectionString()
	if v := strings.TrimSpace(cfg.ConnectionProperties.StrictNullValueHandling); v != "" {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn = fmt.Sprintf("%s%voracle.go.DriverProperties.StrictNullValueHandling=%s", dsn, separator, v)
	}
	oc := NewOracleDriverConfig()
	oc.ConnectDescriptor = dsn

	return NewOracleConnector(oc)

}

// openTestDBWithConfig opens a test database connection using the provided test configuration
// and performs a ping. The caller owns the returned *sql.DB and must close it.
func openTestDBWithConfig(cfg *TestConfig) (*sql.DB, error) {
	if cfg == nil {
		return nil, sql.ErrConnDone
	}
	dsn := cfg.GetConnectionString()
	if v := strings.TrimSpace(cfg.ConnectionProperties.StrictNullValueHandling); v != "" {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn = fmt.Sprintf("%s%voracle.go.DriverProperties.StrictNullValueHandling=%s", dsn, separator, v)
	}

	db, err := sql.Open(cfg.Driver.Name, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// openTestDBWithDriverConfig opens a test database connection using the provided driver configuration
// and performs a ping. The caller owns the returned *sql.DB and must close it.
func openTestDBWithDriverConfig(cfg *oracleconfig.OracleDriverConfig) (*sql.DB, error) {
	db, err := sql.Open("oracledb", cfg.ConnectDescriptor)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// createTable creates a table with the provided column definition map {columnName: type}.
func createTable(ctx context.Context, db *sql.DB, table string, desc map[string]string) error {
	if len(desc) == 0 {
		return fmt.Errorf("supply column descriptions to create table")
	}

	var b strings.Builder
	b.WriteString("CREATE TABLE ")
	b.WriteString(table)
	b.WriteString(" (")

	index := 0
	for key, value := range desc {
		if index > 0 {
			b.WriteString(", ")
		}
		b.WriteString(key)
		b.WriteString(" ")
		b.WriteString(value)
		index++
	}
	b.WriteString(")")

	_, err := db.ExecContext(ctx, b.String())
	return err
}

// dropTable drops a table deterministically. Tests should only drop tables they created.
func dropTable(ctx context.Context, db *sql.DB, table string) error {
	_, err := db.ExecContext(ctx, "DROP TABLE "+table+" PURGE")
	return err
}

// deleteAllRows deletes all rows from the given table.
func deleteAllRows(ctx context.Context, db *sql.DB, table string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM "+table)
	return err
}

// fetchNLSCharacterSet queries the database NLS_CHARACTERSET parameter value.
func fetchNLSCharacterSet(ctx context.Context, db *sql.DB) (string, error) {
	const q = "SELECT value FROM nls_database_parameters WHERE parameter = 'NLS_CHARACTERSET'"
	var charset string
	if err := db.QueryRowContext(ctx, q).Scan(&charset); err != nil {
		return "", err
	}
	return charset, nil
}

// fetchNLSNcharCharacterSet queries the database NLS_NCHAR_CHARACTERSET parameter value.
func fetchNLSNcharCharacterSet(ctx context.Context, db *sql.DB) (string, error) {
	const q = "SELECT value FROM nls_database_parameters WHERE parameter = 'NLS_NCHAR_CHARACTERSET'"
	var charset string
	if err := db.QueryRowContext(ctx, q).Scan(&charset); err != nil {
		return "", err
	}
	return charset, nil
}

// checkErrorRaised verifies that an error is a SQLError with the expected error code and cause.
// This helper is used across multiple test files to validate error handling.
func checkErrorRaised(t *testing.T, err error, expectedError oracleErrors.ErrorCode,
	expectedCause oracleErrors.ErrorCode) {
	if serr, ok := err.(oracleErrors.SQLError); ok {
		if serr.ErrorCode() != string(expectedError) {
			t.Fatalf("Expected %v error but got %s", expectedError, err)
		}

		if cause, ok := errors.Unwrap(serr).(oracleErrors.SQLError); ok {
			if cause.ErrorCode() != string(expectedCause) {
				t.Fatalf("Expected %v error as cause but got %s", expectedCause, cause)
			}
		}
	} else {
		t.Fatalf("error raise not an SQLError")
	}
}

// getRemoteDbTimeZoneOffset gets the remote server timezone offset
func getRemoteDbAndSessionTimeZoneOffset(ctx context.Context, db *sql.DB) (int, int, error) {
	rs, err := db.QueryContext(ctx, "SELECT SESSIONTIMEZONE FROM DUAL")
	if err != nil {
		return 0, 0, err
	}
	rs.Next()
	var sessOffSet string
	rs.Scan(&sessOffSet)

	SESSTZH, SESSTZM := parseTimeZone(sessOffSet)
	return SESSTZH, SESSTZM, nil
}

func parseTimeZone(timezone string) (int, int) {
	var sign int = 1
	if strings.Compare(timezone[0:1], "-") == 0 {
		sign = -1
	}
	items := strings.Split(timezone[1:], ":")
	TZH, _ := strconv.Atoi(items[0])
	TZM, _ := strconv.Atoi(items[1])
	return sign * TZH, sign * TZM
}

// Database-free verification for the shared SELECT helper and retention runner.
// A small database/sql-backed fake exercises real Rows ownership and scan
// behavior without Oracle, a network connection, or a new mock dependency.
type retentionFakeConnector struct {
	query      func(context.Context, int64) (driver.Rows, error)
	connect    func(context.Context, int64) error
	onClose    func()
	closeError error
	opened     atomic.Int64
	closed     atomic.Int64
	queries    atomic.Int64
	rowsClosed atomic.Int64
}

func (c *retentionFakeConnector) Connect(ctx context.Context) (driver.Conn, error) {
	number := c.opened.Add(1)
	if c.connect != nil {
		if err := c.connect(ctx, number); err != nil {
			return nil, err
		}
	}
	return &retentionFakeConn{connector: c}, nil
}
func (*retentionFakeConnector) Driver() driver.Driver { return retentionFakeDriver{} }

type retentionFakeDriver struct{}

func (retentionFakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type retentionFakeConn struct{ connector *retentionFakeConnector }

func (*retentionFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}
func (*retentionFakeConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected Begin") }
func (c *retentionFakeConn) Close() error {
	c.connector.closed.Add(1)
	if c.connector.onClose != nil {
		c.connector.onClose()
	}
	return c.connector.closeError
}
func (c *retentionFakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if query != "SELECT 1 FROM DUAL" || len(args) != 0 {
		return nil, fmt.Errorf("unexpected request %q", query)
	}
	number := c.connector.queries.Add(1)
	if c.connector.query != nil {
		return c.connector.query(ctx, number)
	}
	return &retentionFakeRows{values: []driver.Value{int64(1)}, closed: &c.connector.rowsClosed}, nil
}

type retentionFakeRows struct {
	values                []driver.Value
	index                 int
	nextError, closeError error
	closed                *atomic.Int64
}

func (*retentionFakeRows) Columns() []string { return []string{"value"} }
func (r *retentionFakeRows) Close() error {
	r.closed.Add(1)
	return r.closeError
}
func (r *retentionFakeRows) Next(values []driver.Value) error {
	if r.index == len(r.values) {
		if r.nextError != nil {
			return r.nextError
		}
		return io.EOF
	}
	values[0] = r.values[r.index]
	r.index++
	return nil
}

func TestResourceRetentionSelectHelper(t *testing.T) {
	iterationError := errors.New("iteration failed")
	closeError := errors.New("close failed")
	queryError := errors.New("query failed")
	cases := []struct {
		name                              string
		values                            []driver.Value
		nextError, closeError, queryError error
		want                              string
		cause                             error
	}{
		{name: "one correct row", values: []driver.Value{int64(1)}},
		{name: "empty", want: "no rows"},
		{name: "extra row", values: []driver.Value{int64(1), int64(1)}, want: "more than one"},
		{name: "incorrect value", values: []driver.Value{int64(2)}, want: "want 1"},
		{name: "scan error", values: []driver.Value{"not an integer"}, want: "scan DUAL"},
		{name: "iteration error", values: []driver.Value{int64(1)}, nextError: iterationError, want: "iterate DUAL", cause: iterationError},
		{name: "query error", queryError: queryError, want: "query DUAL", cause: queryError},
		{name: "EOF close error", values: []driver.Value{int64(1)}, closeError: closeError, want: "close failed", cause: closeError},
		{name: "validation and close errors", values: []driver.Value{int64(2)}, closeError: closeError, want: "want 1", cause: closeError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector := &retentionFakeConnector{}
			connector.query = func(context.Context, int64) (driver.Rows, error) {
				if tc.queryError != nil {
					return nil, tc.queryError
				}
				return &retentionFakeRows{values: tc.values, nextError: tc.nextError, closeError: tc.closeError, closed: &connector.rowsClosed}, nil
			}
			db := sql.OpenDB(connector)
			err := executeSelectDual(context.Background(), db)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("lost error cause: %v", err)
			}
			wantClosed := int64(1)
			if tc.queryError != nil {
				wantClosed = 0
			}
			if connector.rowsClosed.Load() != wantClosed {
				t.Fatalf("rows closed %d times, want %d", connector.rowsClosed.Load(), wantClosed)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResourceRetentionWorkers(t *testing.T) {
	t.Run("distribution and accounting", func(t *testing.T) {
		for _, total := range []int{4, 10, 1024} {
			var counts [4]int
			completed, err := runResourceRetentionWorkers(context.Background(), 4, total,
				func(ctx context.Context, worker, operation int) error { counts[worker]++; return nil })
			if err != nil || completed != total {
				t.Fatalf("completed=%d error=%v", completed, err)
			}
			for worker, count := range counts {
				want := total / 4
				if worker < total%4 {
					want++
				}
				if count != want || resourceRetentionWorkerOperations(total, 4, worker) != want {
					t.Fatalf("worker %d count=%d want=%d", worker, count, want)
				}
			}
		}
	})
	t.Run("concurrent start", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var entered atomic.Int64
		release := make(chan struct{})
		completed, err := runResourceRetentionWorkers(ctx, 4, 4, func(ctx context.Context, worker, operation int) error {
			if entered.Add(1) == 4 {
				close(release)
			}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil || completed != 4 {
			t.Fatalf("workers did not overlap: %d %v", completed, err)
		}
	})
	t.Run("first error cancels and joins siblings", func(t *testing.T) {
		cause := errors.New("SELECT failed")
		var exited atomic.Int64
		completed, err := runResourceRetentionWorkers(context.Background(), 4, 1024, func(ctx context.Context, worker, operation int) error {
			defer exited.Add(1)
			if worker == 0 {
				return cause
			}
			<-ctx.Done()
			return ctx.Err()
		})
		if completed != 0 || !errors.Is(err, cause) || !strings.Contains(err.Error(), "worker 1 operation 1") {
			t.Fatalf("completed=%d error=%v", completed, err)
		}
		// Some siblings can observe cancellation before entering the callback.
		if exited.Load() < 1 {
			t.Fatal("failing worker was not joined")
		}
	})
	t.Run("partial successful work", func(t *testing.T) {
		cause := errors.New("second operation failed")
		completed, err := runResourceRetentionWorkers(context.Background(), 1, 4, func(ctx context.Context, worker, operation int) error {
			if operation == 1 {
				return cause
			}
			return nil
		})
		if completed != 1 || !errors.Is(err, cause) {
			t.Fatalf("completed=%d error=%v", completed, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		completed, err := runResourceRetentionWorkers(ctx, 4, 4, func(ctx context.Context, worker, operation int) error {
			<-ctx.Done()
			return ctx.Err()
		})
		if completed != 0 || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("completed=%d error=%v", completed, err)
		}
	})
	t.Run("panic becomes failure", func(t *testing.T) {
		completed, err := runResourceRetentionWorkers(context.Background(), 1, 1, func(context.Context, int, int) error { panic("unexpected panic") })
		if completed != 0 || err == nil || !strings.Contains(err.Error(), "panicked") {
			t.Fatalf("completed=%d error=%v", completed, err)
		}
	})
	t.Run("invalid workload", func(t *testing.T) {
		for _, settings := range [][2]int{{0, 1}, {-1, 1}, {4, 3}} {
			if _, err := runResourceRetentionWorkers(context.Background(), settings[0], settings[1], nil); err == nil {
				t.Fatal("accepted invalid workload")
			}
		}
	})
}

// No t.Parallel: SetMemoryLimit is process-wide. The category registration is
// exclusive too. The target below belongs to fake-driver unit verification,
// not a default or recommendation for real database executions.
func TestResourceRetentionMemoryTargetRestoration(t *testing.T) {
	// Start with a non-default setting so baseline checks catch an accidental
	// reset of a pre-existing limit, not just an unwanted new override.
	original := debug.SetMemoryLimit(384 * (1 << 20))
	defer debug.SetMemoryLimit(original)
	previous := debug.SetMemoryLimit(-1)
	limit := int64(256)
	config := oracleTest.ResourceRetentionConfig{Workers: 4, TotalOperations: 12, MaxOpenConnections: 4, Timeout: "1s", MemoryLimitMiB: &limit}
	target := limit * (1 << 20)
	cause := errors.New("injected unit-test failure")
	for _, name := range []string{"success", "initialize error", "warm-up query error", "warm-up connect error", "workload error", "close error", "timeout", "initialize panic", "invalid config", "baseline success", "baseline initialize error", "baseline workload error", "baseline close error", "baseline timeout", "baseline initialize panic"} {
		t.Run(name, func(t *testing.T) {
			scenario := strings.TrimPrefix(name, "baseline ")
			baseline := scenario != name
			expectedTarget := target
			if baseline {
				expectedTarget = previous
			}
			connector := &retentionFakeConnector{}
			var wrongTarget atomic.Bool
			checkTarget := func() {
				if debug.SetMemoryLimit(-1) != expectedTarget {
					wrongTarget.Store(true)
				}
			}
			connector.onClose = checkTarget
			connector.query = func(ctx context.Context, number int64) (driver.Rows, error) {
				checkTarget()
				if scenario == "warm-up query error" && number == 2 || scenario == "workload error" && number > 4 {
					return nil, cause
				}
				if scenario == "timeout" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return &retentionFakeRows{values: []driver.Value{int64(1)}, closed: &connector.rowsClosed}, nil
			}
			if scenario == "warm-up connect error" {
				connector.connect = func(ctx context.Context, number int64) error {
					if number == 2 {
						return cause
					}
					return nil
				}
			}
			if scenario == "close error" {
				connector.closeError = cause
			}
			current := config
			if baseline {
				current.MemoryLimitMiB = nil
			}
			if scenario == "timeout" {
				current.Timeout = "10ms"
			}
			if scenario == "invalid config" {
				invalidLimit := int64(0)
				current.MemoryLimitMiB = &invalidLimit
			}
			var result resourceRetentionResult
			var runErr error
			var panicked any
			func() {
				defer func() { panicked = recover() }()
				result, runErr = executeResourceRetention(current, func(ctx context.Context) (*sql.DB, error) {
					checkTarget()
					if _, ok := ctx.Deadline(); !ok {
						return nil, errors.New("no initialization deadline")
					}
					if scenario == "initialize error" {
						return nil, cause
					}
					if scenario == "initialize panic" {
						panic("factory panic")
					}
					return sql.OpenDB(connector), nil
				})
			}()
			if panicked == nil && scenario != "invalid config" && result.MemoryLimitBytes != expectedTarget {
				t.Fatalf("reported memory target %d, want %d", result.MemoryLimitBytes, expectedTarget)
			}
			if debug.SetMemoryLimit(-1) != previous {
				t.Fatal("memory target was not restored")
			}
			if wrongTarget.Load() {
				t.Fatal("memory target was not active through database cleanup")
			}
			if scenario == "initialize panic" {
				if panicked == nil {
					t.Fatal("expected factory panic")
				}
				return
			}
			if panicked != nil {
				t.Fatalf("unexpected panic: %v", panicked)
			}
			switch scenario {
			case "success":
				if runErr != nil || result.Completed != 12 || connector.opened.Load() != 4 || connector.closed.Load() != 4 || connector.queries.Load() != 16 || connector.rowsClosed.Load() != 16 {
					t.Fatalf("result=%+v error=%v opened=%d closed=%d queries=%d rowsClosed=%d", result, runErr, connector.opened.Load(), connector.closed.Load(), connector.queries.Load(), connector.rowsClosed.Load())
				}
				if result.Baseline.HeapAlloc == 0 || result.Final.HeapAlloc == 0 {
					t.Fatal("missing heap samples")
				}
			case "timeout":
				if !errors.Is(runErr, context.DeadlineExceeded) {
					t.Fatalf("expected deadline, got %v", runErr)
				}
			case "invalid config":
				if runErr == nil || connector.opened.Load() != 0 {
					t.Fatal("invalid config initialized database")
				}
			default:
				if !errors.Is(runErr, cause) {
					t.Fatalf("lost injected cause: %v", runErr)
				}
			}
			wantClosed := connector.opened.Load()
			if scenario == "warm-up connect error" {
				wantClosed--
			} // Failed Connect returns no application-owned connection.
			if connector.closed.Load() != wantClosed {
				t.Fatalf("unclosed physical connections: opened=%d closed=%d", connector.opened.Load(), connector.closed.Load())
			}
		})
	}
}
