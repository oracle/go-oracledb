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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
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

type retentionFakeStmt struct {
	conn  *retentionFakeConn
	query string
}

func (s *retentionFakeStmt) Close() error {
	s.conn.connector.statementsClosed.Add(1)
	return s.conn.connector.statementCloseError
}
func (*retentionFakeStmt) NumInput() int { return -1 }
func (*retentionFakeStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("unexpected statement Exec")
}
func (*retentionFakeStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("use QueryContext")
}
func (s *retentionFakeStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

type retentionFakeTx struct{ connector *retentionFakeConnector }

func (t *retentionFakeTx) Commit() error {
	t.connector.committed.Add(1)
	return t.connector.commitError
}
func (t *retentionFakeTx) Rollback() error {
	t.connector.rolledBack.Add(1)
	return t.connector.rollbackError
}

// Generate one row at a time. Even the fake must not cache large result sets.
type retentionSyntheticRows struct {
	columns               []string
	count, index          int
	row                   func(int) []driver.Value
	nextError, closeError error
	closed                *atomic.Int64
}

func (r *retentionSyntheticRows) Columns() []string { return r.columns }
func (r *retentionSyntheticRows) Close() error {
	if r.closed != nil {
		r.closed.Add(1)
	}
	return r.closeError
}
func (r *retentionSyntheticRows) Next(values []driver.Value) error {
	if r.index == r.count {
		if r.nextError != nil {
			return r.nextError
		}
		return io.EOF
	}
	r.index++
	copy(values, r.row(r.index))
	return nil
}

func retentionExpectedFakeRow(kind string, id, rows int) []driver.Value {
	if kind == "lob_select" {
		return []driver.Value{int64(id), []byte(strings.Repeat(string([]byte{byte(id % 251)}), retentionLOBBytes)), strings.Repeat(string(rune('a'+id%26)), retentionLOBBytes)}
	}
	if kind == "json_select" {
		return []driver.Value{int64(id), fmt.Sprintf(`{"padding":%q,"active":true,"items":[%d,%d,%d],"nested":{"name":"row-%d"},"id":%d}`, strings.Repeat(string(rune('a'+id%26)), retentionJSONPadding), id, id+1, id+2, id, id)}
	}
	group := (id-1)%8 + 1
	total := 0
	for i := group; i <= rows; i += 8 {
		total += i
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Second)
	var unicode driver.Value = fmt.Sprintf("東京-%d", id)
	state := "VALUE"
	if id%16 == 0 {
		unicode = nil
		state = "NULL"
	}
	return []driver.Value{int64(id), int64(group), fmt.Sprintf("group-%d", group), int64(id), fmt.Sprintf("%d.%03d000", id, id), float64(id) / 4, float64(id) / 8, strings.Repeat(string(rune('a'+id%26)), 2000), fmt.Sprintf("Ω%06d ", id), unicode, []byte(strings.Repeat(string([]byte{byte(id % 251)}), 32)), base, base.Add(123456789 * time.Nanosecond), base.Add(123456789 * time.Nanosecond).In(time.FixedZone("+0530", 19800)), state, int64(id), int64(total)}
}
func retentionFixtureFake(kind string, count int) *retentionFakeConnector {
	c := &retentionFakeConnector{}
	c.request = func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		n := 17
		if kind == "lob_select" {
			n = 3
		}
		if kind == "json_select" {
			n = 2
		}
		return &retentionSyntheticRows{columns: make([]string, n), count: count, closed: &c.rowsClosed, row: func(id int) []driver.Value { return retentionExpectedFakeRow(kind, id, count) }}, nil
	}
	return c
}

// Check provisioning safeguards without creating database objects.
func TestResourceRetentionFixtureSetupUnit(t *testing.T) {
	cause := errors.New("injected setup failure")
	cases := []struct {
		name, fail, wantError             string
		wantDDL, wantCommit, wantRollback int
	}{
		{name: "success", wantDDL: 4, wantCommit: 1},
		{name: "unlimited privilege", fail: "unlimited privilege", wantDDL: 4, wantCommit: 1},
		{name: "unlimited quota", fail: "unlimited quota", wantDDL: 4, wantCommit: 1},
		{name: "wrong schema", fail: "schema", wantError: "own schema"},
		{name: "configured other owner", fail: "owner", wantError: "own schema"},
		{name: "existing object", fail: "existing", wantError: "refusing to replace"},
		{name: "object row close", fail: "rows close", wantError: "setup object preflight"},
		{name: "no create privilege", fail: "privilege", wantError: "CREATE TABLE"},
		{name: "quota missing", fail: "quota missing", wantError: "quota preflight"},
		{name: "quota insufficient", fail: "quota", wantError: "quota is below"},
		{name: "release unsupported", fail: "version", wantError: "release preflight"},
		{name: "partial DDL failure", fail: "DDL", wantError: "create ORA_STRESS_LOB", wantDDL: 2},
		{name: "population fails", fail: "populate", wantError: "populate fixture", wantDDL: 4, wantRollback: 1},
		{name: "population rollback error", fail: "rollback", wantError: "fixture population rollback", wantDDL: 4, wantRollback: 1},
		{name: "row count mismatch", fail: "counts", wantError: "row-count mismatch", wantDDL: 4, wantRollback: 1},
		{name: "LOB length mismatch", fail: "length", wantError: "LOB length mismatch", wantDDL: 4, wantRollback: 1},
		{name: "commit error", fail: "commit", wantError: "commit fixture population", wantDDL: 4, wantCommit: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &retentionFakeConnector{}
			one := func(values ...driver.Value) driver.Rows {
				return &retentionSyntheticRows{
					columns: make([]string, len(values)), count: 1, closed: &c.rowsClosed,
					row: func(int) []driver.Value { return values },
				}
			}
			c.request = func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
				switch {
				case strings.Contains(query, "SESSION_USER"):
					schema := "STRESS_USER"
					if tc.fail == "schema" {
						schema = "OTHER_OWNER"
					}
					return one("STRESS_USER", schema, "DATA"), nil
				case strings.Contains(query, "user_objects"):
					r := &retentionSyntheticRows{columns: []string{"object_name", "object_type"}, closed: &c.rowsClosed}
					if tc.fail == "existing" {
						r.count = 1
						r.row = func(int) []driver.Value { return []driver.Value{"ORA_STRESS_CORE", "TABLE"} }
					}
					if tc.fail == "rows close" {
						r.closeError = cause
					}
					return r, nil
				case strings.Contains(query, "session_privs"):
					create, unlimited := int64(1), int64(0)
					if tc.fail == "privilege" {
						create = 0
					}
					if tc.fail == "unlimited privilege" {
						unlimited = 1
					}
					return one(create, unlimited), nil
				case strings.Contains(query, "user_ts_quotas"):
					if tc.fail == "quota missing" {
						return &retentionSyntheticRows{columns: []string{"bytes", "max_bytes"}, closed: &c.rowsClosed}, nil
					}
					limit := int64(2 << 30)
					if tc.fail == "quota" {
						limit = 1 << 20
					}
					if tc.fail == "unlimited quota" {
						limit = -1
					}
					return one(int64(0), limit), nil
				case strings.Contains(query, "COUNT(*) FROM ora_stress_groups"):
					count := int64(512)
					if tc.fail == "counts" {
						count = 511
					}
					return one(int64(8), count, int64(512), int64(512)), nil
				case strings.Contains(query, "DBMS_LOB.GETLENGTH"):
					size := int64(retentionLOBBytes)
					if tc.fail == "length" {
						size--
					}
					return one(size, size, size, size), nil
				default:
					return nil, fmt.Errorf("unexpected setup query %q", query)
				}
			}
			var created []string
			c.exec = func(_ context.Context, query string, _ []driver.NamedValue) error {
				if strings.Contains(strings.ToUpper(query), "DROP ") {
					t.Fatal("setup must never DROP")
				}
				switch {
				case strings.Contains(query, "DBMS_DB_VERSION"):
					if tc.fail == "version" {
						return cause
					}
				case strings.HasPrefix(query, "CREATE TABLE"):
					if tc.fail == "DDL" && strings.Contains(query, "ora_stress_lob") {
						return cause
					}
					created = append(created, query)
				case query == retentionFixturePopulationSQL:
					if tc.fail == "populate" || tc.fail == "rollback" {
						return cause
					}
				default:
					return fmt.Errorf("unexpected setup statement %q", query)
				}
				return nil
			}
			if tc.fail == "rollback" {
				c.rollbackError = cause
			}
			if tc.fail == "commit" {
				c.commitError = cause
			}
			db := sql.OpenDB(c)
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			config := oracleTest.ResourceRetentionConfig{
				Workers: 1, TotalOperations: 1, MaxOpenConnections: 1, Timeout: "10m",
				Workload: "select_dual",
			}
			if tc.fail == "owner" {
				config.FixtureSchema = "OTHER_OWNER"
			}
			err = provisionResourceRetentionFixtures(context.Background(), conn, config, func(string, ...any) {})
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("got %v; want %q", err, tc.wantError)
			}
			if len(created) != tc.wantDDL || c.committed.Load() != int64(tc.wantCommit) || c.rolledBack.Load() != int64(tc.wantRollback) {
				t.Fatalf("created=%d committed=%d rolled_back=%d", len(created), c.committed.Load(), c.rolledBack.Load())
			}
			for i := range created {
				if created[i] != retentionFixtureDDL[i].sql {
					t.Fatal("fixture DDL order changed")
				}
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if c.opened.Load() != 1 || c.closed.Load() != 1 {
				t.Fatal("setup connection not closed")
			}
		})
	}
}

func TestResourceRetentionLargeResults(t *testing.T) {
	t.Run("complex SQL starts with supported SELECT keyword", func(t *testing.T) {
		config := oracleTest.ResourceRetentionConfig{FixtureSchema: "SCOTT", RowsPerQuery: 300}
		for _, kind := range []string{"complex_select", "prepared_select", "transaction_commit", "transaction_rollback"} {
			query := retentionQuery(config, kind)
			if !strings.HasPrefix(strings.TrimSpace(query), "SELECT ") || strings.Contains(query, "WITH selected") {
				t.Fatalf("%s must start SELECT rather than an unsupported leading WITH", kind)
			}
			for _, fragment := range []string{
				"FROM (SELECT * FROM SCOTT.ORA_STRESS_CORE WHERE id BETWEEN :first_id AND :last_id) c",
				"JOIN SCOTT.ORA_STRESS_GROUPS g ON g.group_id=c.group_id",
				"CASE WHEN c.unicode_value IS NULL",
				"ROW_NUMBER() OVER (ORDER BY c.id)",
				"SUM(c.int_value) OVER (PARTITION BY c.group_id)",
				"ORDER BY c.id",
			} {
				if !strings.Contains(query, fragment) {
					t.Fatalf("%s lost query component %q", kind, fragment)
				}
			}
			if strings.Count(query, ":first_id") != 1 || strings.Count(query, ":last_id") != 1 {
				t.Fatalf("%s changed range binds", kind)
			}
		}
	})
	t.Run("fixture metadata and change guard", func(t *testing.T) {
		for _, badJSON := range []bool{false, true} {
			c := &retentionFakeConnector{}
			c.request = func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				if strings.Contains(query, "all_tab_columns") {
					table := fmt.Sprint(args[1].Value)
					pairs := [][2]string{}
					switch table {
					case "ORA_STRESS_CORE":
						pairs = [][2]string{{"ID", "NUMBER"}, {"GROUP_ID", "NUMBER"}, {"FIXTURE_VERSION", "NUMBER"}, {"INT_VALUE", "NUMBER"}, {"DECIMAL_VALUE", "NUMBER"}, {"FLOAT_VALUE", "BINARY_FLOAT"}, {"DOUBLE_VALUE", "BINARY_DOUBLE"}, {"TEXT_VALUE", "VARCHAR2"}, {"NCHAR_VALUE", "NCHAR"}, {"UNICODE_VALUE", "NVARCHAR2"}, {"RAW_VALUE", "RAW"}, {"DATE_VALUE", "DATE"}, {"TIMESTAMP_VALUE", "TIMESTAMP(9)"}, {"TZ_VALUE", "TIMESTAMP(9) WITH TIME ZONE"}}
					case "ORA_STRESS_GROUPS":
						pairs = [][2]string{{"GROUP_ID", "NUMBER"}, {"GROUP_NAME", "VARCHAR2"}}
					case "ORA_STRESS_JSON":
						pairs = [][2]string{{"ID", "NUMBER"}, {"DOCUMENT", "CLOB"}}
					}
					return &retentionSyntheticRows{columns: []string{"column_name", "data_type"}, count: len(pairs), closed: &c.rowsClosed, row: func(i int) []driver.Value { return []driver.Value{pairs[i-1][0], pairs[i-1][1]} }}, nil
				}
				if strings.Contains(query, "MIN(fixture_version)") {
					return &retentionSyntheticRows{columns: make([]string, 5), count: 1, closed: &c.rowsClosed, row: func(int) []driver.Value { return []driver.Value{int64(512), int64(1), int64(512), int64(1), int64(1)} }}, nil
				}
				if strings.Contains(query, "ORA_ROWSCN") {
					n := int64(512)
					if strings.Contains(query, "ORA_STRESS_GROUPS") {
						n = 8
					}
					return &retentionSyntheticRows{columns: make([]string, 5), count: 1, closed: &c.rowsClosed, row: func(int) []driver.Value { return []driver.Value{n, int64(42), int64(1), n, n} }}, nil
				}
				return &retentionSyntheticRows{columns: make([]string, 17), count: 300, closed: &c.rowsClosed, row: func(id int) []driver.Value { return retentionExpectedFakeRow("complex_select", id, 300) }}, nil
			}
			db := sql.OpenDB(c)
			config := oracleTest.ResourceRetentionConfig{Workload: "complex_select", RowsPerQuery: 300}
			if badJSON {
				config.Workload = "json_select"
			}
			err := preflightRetentionFixtures(context.Background(), db, config)
			if badJSON {
				if err == nil || !strings.Contains(err.Error(), "want JSON") {
					t.Fatalf("JSON substitution accepted: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if c.queries.Load() != 3 {
					t.Fatal("metadata preflight executed a workload query")
				}
				// Functional baseline: metadata plus one validated cycle, not two.
				if _, err = executeRetentionCycle(context.Background(), db, config, 0, 0); err != nil || c.queries.Load() != 4 {
					t.Fatalf("baseline duplicated workload: queries=%d err=%v", c.queries.Load(), err)
				}
				if _, err = retentionFixtureFingerprint(context.Background(), db, config); err != nil {
					t.Fatal(err)
				}
				// Full data validation is now warm-up, with its own deadline.
				originalRequest := c.request
				config.OperationTimeout = "20ms"
				config.Workers = 1
				c.request = func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
					if strings.Contains(query, "ROW_NUMBER()") {
						if _, ok := ctx.Deadline(); !ok {
							return nil, errors.New("warm-up query has no deadline")
						}
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return originalRequest(ctx, query, args)
				}
				if err = warmResourceRetention(context.Background(), db, config); !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "complex_select") {
					t.Fatalf("warm-up workload deadline lost: %v", err)
				}
				c.request = func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if err = preflightRetentionFixtures(context.Background(), db, config); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("metadata deadline lost: %v", err)
				}
				if _, err = retentionFixtureFingerprint(context.Background(), db, config, 20*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("fingerprint deadline lost: %v", err)
				}
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
		}
	})
	for _, kind := range []string{"complex_select", "lob_select", "json_select"} {
		t.Run(kind, func(t *testing.T) {
			n := 3
			if kind == "complex_select" {
				n = 300
			}
			c := retentionFixtureFake(kind, n)
			db := sql.OpenDB(c)
			defer db.Close()
			counts, err := consumeRetentionQuery(context.Background(), db, "fixture", kind, n)
			if err != nil || counts.Rows != uint64(n) || counts.Selects != 1 || c.rowsClosed.Load() != 1 {
				t.Fatalf("counts=%+v err=%v closed=%d", counts, err, c.rowsClosed.Load())
			}
		})
	}
	for _, name := range []string{"empty", "extra", "incorrect", "out of order", "scan", "iteration", "close", "LOB length", "LOB contents", "JSON contents"} {
		t.Run(name, func(t *testing.T) {
			kind := "complex_select"
			if strings.HasPrefix(name, "LOB") {
				kind = "lob_select"
			}
			if strings.HasPrefix(name, "JSON") {
				kind = "json_select"
			}
			c := retentionFixtureFake(kind, 3)
			cause := errors.New("synthetic error")
			c.request = func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				n := 17
				if kind == "lob_select" {
					n = 3
				}
				if kind == "json_select" {
					n = 2
				}
				r := &retentionSyntheticRows{columns: make([]string, n), count: 3, closed: &c.rowsClosed}
				if name == "empty" {
					r.count = 0
				}
				if name == "extra" {
					r.count = 4
				}
				if name == "iteration" {
					r.nextError = cause
				}
				if name == "close" {
					r.closeError = cause
				}
				r.row = func(id int) []driver.Value {
					v := retentionExpectedFakeRow(kind, id, 3)
					switch name {
					case "incorrect":
						v[3] = int64(999)
					case "out of order":
						v[0] = int64(2)
					case "scan":
						v[0] = "bad"
					case "LOB length":
						v[1] = []byte{1}
					case "LOB contents":
						v[1].([]byte)[0] ^= 1
					case "JSON contents":
						v[1] = `{"id":0}`
					}
					return v
				}
				return r, nil
			}
			db := sql.OpenDB(c)
			defer db.Close()
			_, err := consumeRetentionQuery(context.Background(), db, "fixture", kind, 3)
			if err == nil || c.rowsClosed.Load() != 1 {
				t.Fatalf("err=%v closed=%d", err, c.rowsClosed.Load())
			}
			if (name == "iteration" || name == "close") && !errors.Is(err, cause) {
				t.Fatal("lost error cause")
			}
		})
	}
}

func TestResourceRetentionLifecycles(t *testing.T) {
	for _, kind := range []string{"prepared_select", "transaction_commit", "transaction_rollback", "connection_checkout"} {
		t.Run(kind, func(t *testing.T) {
			fakeKind := "complex_select"
			if kind == "connection_checkout" {
				fakeKind = "lob_select"
			}
			c := retentionFixtureFake(fakeKind, 3)
			db := sql.OpenDB(c)
			defer db.Close()
			config := oracleTest.ResourceRetentionConfig{Workload: kind, RowsPerQuery: 3}
			counts, err := executeRetentionCycle(context.Background(), db, config, 0, 0)
			if err != nil || counts.Cycles != 1 || db.Stats().InUse != 0 {
				t.Fatalf("%+v %v", counts, err)
			}
			if kind == "prepared_select" && (counts.Selects != 2 || c.statementsClosed.Load() != 1) {
				t.Fatal("statement lifecycle")
			}
			if kind == "transaction_commit" && c.committed.Load() != 1 {
				t.Fatal("commit lifecycle")
			}
			if kind == "transaction_rollback" && c.rolledBack.Load() != 1 {
				t.Fatal("rollback lifecycle")
			}
		})
	}
	for _, stage := range []string{"prepare", "statement close", "begin", "commit", "rollback", "query in transaction"} {
		t.Run(stage, func(t *testing.T) {
			c := retentionFixtureFake("complex_select", 3)
			cause := errors.New(stage)
			kind := "prepared_select"
			switch stage {
			case "prepare":
				c.prepareError = cause
			case "statement close":
				c.statementCloseError = cause
			case "begin":
				kind = "transaction_commit"
				c.beginError = cause
			case "commit":
				kind = "transaction_commit"
				c.commitError = cause
			case "rollback":
				kind = "transaction_rollback"
				c.rollbackError = cause
			case "query in transaction":
				kind = "transaction_commit"
				c.request = func(context.Context, string, []driver.NamedValue) (driver.Rows, error) { return nil, cause }
			}
			db := sql.OpenDB(c)
			defer db.Close()
			_, err := executeRetentionCycle(context.Background(), db, oracleTest.ResourceRetentionConfig{Workload: kind, RowsPerQuery: 3}, 0, 0)
			if !errors.Is(err, cause) || db.Stats().InUse != 0 {
				t.Fatalf("cleanup failure lost %v", err)
			}
			if stage == "query in transaction" && c.rolledBack.Load() != 1 {
				t.Fatal("failed query did not roll back")
			}
		})
	}
}

func TestResourceRetentionEpochsAndArtifacts(t *testing.T) {
	config := oracleTest.ResourceRetentionConfig{Workers: 4, MaxOpenConnections: 4, TotalOperations: 12, Timeout: "2s", OperationTimeout: "1s"}.Normalized()
	indices := make([]int, 4)
	p := &retentionProgress{}
	var inFlight atomic.Int64
	for p.snapshot().Cycles < 12 {
		_, _, err := runRetentionEpoch(context.Background(), config, indices, time.Millisecond, p, func(context.Context, int, int) (retentionCounts, error) {
			inFlight.Add(1)
			defer inFlight.Add(-1)
			time.Sleep(2 * time.Millisecond)
			return retentionCounts{Cycles: 1, Selects: 1}, nil
		})
		if err != nil || inFlight.Load() != 0 {
			t.Fatalf("epoch not drained %v", err)
		}
	}
	if p.snapshot().Cycles != 12 {
		t.Fatal("fixed accounting")
	}
	for _, i := range indices {
		if i != 3 {
			t.Fatal("worker distribution changed across checkpoints")
		}
	}
	t.Run("deadline and panic", func(t *testing.T) {
		for _, panicWorker := range []bool{false, true} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			_, _, err := runRetentionEpoch(ctx, config, make([]int, 4), time.Hour, &retentionProgress{}, func(ctx context.Context, w, i int) (retentionCounts, error) {
				if panicWorker {
					panic("worker")
				}
				<-ctx.Done()
				return retentionCounts{}, ctx.Err()
			})
			cancel()
			if err == nil {
				t.Fatal("accepted failure")
			}
		}
	})
	t.Run("per operation timeout", func(t *testing.T) {
		c := config
		c.OperationTimeout = "2ms"
		_, _, err := runRetentionEpoch(context.Background(), c, make([]int, 4), time.Hour, &retentionProgress{}, func(ctx context.Context, w, i int) (retentionCounts, error) {
			<-ctx.Done()
			return retentionCounts{}, nil
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("duration ends without cancellation", func(t *testing.T) {
		c := config
		c.TotalOperations = 0
		c.Duration = "10ms"
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, drain, err := runRetentionEpoch(ctx, c, make([]int, 4), 5*time.Millisecond, &retentionProgress{}, func(ctx context.Context, w, i int) (retentionCounts, error) {
			time.Sleep(10 * time.Millisecond)
			return retentionCounts{Cycles: 1}, ctx.Err()
		})
		if err != nil || drain <= 0 {
			t.Fatalf("duration canceled operation: %v drain=%s", err, drain)
		}
	})
	t.Run("checkpoint ownership and writer errors", func(t *testing.T) {
		db := sql.OpenDB(&retentionFakeConnector{})
		defer db.Close()
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		r := &retentionRecorder{writer: io.Discard, started: time.Now()}
		if _, err = r.checkpoint(0, db, &retentionProgress{}, false, ""); err == nil {
			t.Fatal("checkpoint accepted live checkout")
		}
		conn.Close()
		r.writer = retentionBrokenWriter{}
		if _, err = r.checkpoint(0, db, &retentionProgress{}, false, ""); err == nil {
			t.Fatal("ignored artifact error")
		}
	})
	t.Run("compact snapshots without automatic interpretation", func(t *testing.T) {
		db := sql.OpenDB(&retentionFakeConnector{})
		defer db.Close()
		var output strings.Builder
		r := &retentionRecorder{writer: &output, started: time.Now()}
		if _, err := r.checkpoint(3, db, &retentionProgress{}, false, ""); err != nil {
			t.Fatal(err)
		}
		for _, removed := range []string{"retained_growth_review", "investigate", "PauseNs", "PauseEnd", "BySize"} {
			if strings.Contains(output.String(), removed) {
				t.Fatalf("unexpected reporting machinery: %s", removed)
			}
		}
		decoder := json.NewDecoder(strings.NewReader(output.String()))
		for _, phase := range []string{"drained_natural", "post_gc"} {
			var record struct {
				Phase  string
				Memory map[string]json.RawMessage `json:"runtime_memstats"`
			}
			if err := decoder.Decode(&record); err != nil || record.Phase != phase || len(record.Memory) != 12 {
				t.Fatalf("compact record: %+v err=%v", record, err)
			}
			for _, field := range []string{"HeapAlloc", "HeapObjects", "HeapInuse", "HeapSys", "HeapReleased", "TotalAlloc", "Mallocs", "Frees", "NumGC", "NumForcedGC", "PauseTotalNs", "GCCPUFraction"} {
				if _, ok := record.Memory[field]; !ok {
					t.Fatalf("missing %s", field)
				}
			}
		}
	})
	t.Run("duration with repeated checkpoints", func(t *testing.T) {
		c := config
		c.TotalOperations = 0
		c.Duration = "150ms"
		c.Timeout = "16m"
		c.CheckpointInterval = "3ms"
		c.SampleInterval = "1ms"
		c.OutputDirectory = t.TempDir()
		fake := &retentionFakeConnector{}
		fake.query = func(ctx context.Context, n int64) (driver.Rows, error) {
			select {
			case <-time.After(time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &retentionFakeRows{values: []driver.Value{int64(1)}, closed: &fake.rowsClosed}, nil
		}
		result, err := executeExpandedResourceRetention(c, "duration-test", nil, func(context.Context) (*sql.DB, error) { return sql.OpenDB(fake), nil })
		if err != nil || result.Duration < 150*time.Millisecond || result.Counts.Cycles == 0 || result.Pauses == 0 || fake.opened.Load() != fake.closed.Load() {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	})
	t.Run("full smoke artifacts and closure", func(t *testing.T) {
		c := config
		c.OutputDirectory = t.TempDir()
		c.DiagnosticProfiles = true
		connector := &retentionFakeConnector{}
		result, err := executeExpandedResourceRetention(c, "test-run", nil, func(context.Context) (*sql.DB, error) { return sql.OpenDB(connector), nil })
		if err != nil || result.Completed != 12 || connector.closed.Load() != 4 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		data, err := os.ReadFile(result.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"status":"complete"`) || !strings.Contains(string(data), `"phase":"post_gc"`) {
			t.Fatal("missing artifacts")
		}
		if !strings.Contains(string(data), `"artifact_format_version":2`) || strings.Contains(string(data), "retained_growth_review") {
			t.Fatal("incorrect artifact version or obsolete heuristic")
		}
		if _, err = os.Stat(filepath.Join(filepath.Dir(result.Artifact), "goroutines.txt")); err != nil {
			t.Fatal(err)
		}
	})
}

type retentionBrokenWriter struct{}

func TestResourceRetentionSetupDiagnostics(t *testing.T) {
	t.Run("setup deadline records failure and cleans up", func(t *testing.T) {
		config := oracleTest.ResourceRetentionConfig{Workers: 1, TotalOperations: 1, MaxOpenConnections: 1, Timeout: "5s", OperationTimeout: "10ms", SampleInterval: "1ms", OutputDirectory: t.TempDir()}
		fake := &retentionFakeConnector{}
		fake.query = func(ctx context.Context, _ int64) (driver.Rows, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		result, err := executeExpandedResourceRetention(config, "failed-setup", nil, func(context.Context) (*sql.DB, error) { return sql.OpenDB(fake), nil })
		if !errors.Is(err, context.DeadlineExceeded) || result.Completed != 0 || fake.closed.Load() != fake.opened.Load() {
			t.Fatalf("setup deadline/cleanup: result=%+v err=%v", result, err)
		}
		data, err := os.ReadFile(result.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{`"stage":"pool_priming"`, `"phase":"stage_end"`, `"status":"failed"`, `"pool_closed":true`} {
			if !strings.Contains(string(data), expected) {
				t.Fatalf("missing setup failure evidence: %s", expected)
			}
		}
		time.Sleep(5 * time.Millisecond)
		after, err := os.ReadFile(result.Artifact)
		if err != nil || string(after) != string(data) {
			t.Fatal("sampler still writing after setup failure")
		}
	})
	t.Run("stage failures, redaction and reporting", func(t *testing.T) {
		var output strings.Builder
		config := &TestConfig{}
		config.Credentials.Password = "unit-secret"
		r := &retentionRecorder{writer: &output, runID: "stage-test", started: time.Now(), testConfig: config}
		cause := errors.New("unit-secret query failure")
		if err := r.progress("warm_up", nil, &retentionProgress{}); err != nil {
			t.Fatal(err)
		}
		err := r.finish(cause, nil, &retentionProgress{})
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), "warm_up") {
			t.Fatalf("lost stage/cause: %v", err)
		}
		for _, expected := range []string{`"phase":"stage_start"`, `"phase":"stage_end"`, `"status":"failed"`, `"pool_available":false`, "[redacted]"} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("missing %s", expected)
			}
		}
		if strings.Contains(output.String(), "unit-secret") {
			t.Fatal("stage evidence contains password")
		}
		r.writer = retentionBrokenWriter{}
		if err := r.progress("setup", nil, &retentionProgress{}); err == nil {
			t.Fatal("ignored progress reporting failure")
		}
	})
	t.Run("priming deadline and parent deadline", func(t *testing.T) {
		fake := &retentionFakeConnector{}
		fake.query = func(ctx context.Context, _ int64) (driver.Rows, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		db := sql.OpenDB(fake)
		defer db.Close()
		if err := primeResourceRetentionPool(context.Background(), db, 1, 10*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) || db.Stats().InUse != 0 {
			t.Fatalf("unbounded priming or retained checkout: %v", err)
		}
		parent, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		child, done := retentionOperationContext(parent, time.Hour)
		defer done()
		parentDeadline, _ := parent.Deadline()
		childDeadline, _ := child.Deadline()
		if !childDeadline.Equal(parentDeadline) {
			t.Fatal("operation extended overall deadline")
		}
	})
	t.Run("sampling starts in setup and joins before pool close", func(t *testing.T) {
		config := oracleTest.ResourceRetentionConfig{Workers: 1, TotalOperations: 2, MaxOpenConnections: 1, Timeout: "5s", OperationTimeout: "3s", SampleInterval: "2ms", OutputDirectory: t.TempDir()}
		artifact := filepath.Join(config.OutputDirectory, "early-test", "samples.jsonl")
		release := make(chan struct{})
		defer func() {
			select {
			case <-release:
			default:
				close(release)
			}
		}()
		fake := &retentionFakeConnector{}
		fake.query = func(ctx context.Context, number int64) (driver.Rows, error) {
			if number == 1 {
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return &retentionFakeRows{values: []driver.Value{int64(1)}, closed: &fake.rowsClosed}, nil
		}
		type outcome struct {
			result resourceRetentionResult
			err    error
		}
		finished := make(chan outcome, 1)
		go func() {
			result, err := executeExpandedResourceRetention(config, "early-test", nil, func(context.Context) (*sql.DB, error) { return sql.OpenDB(fake), nil })
			finished <- outcome{result, err}
		}()
		// Observe an in-use checkout in a periodic sample while QueryContext is
		// deliberately blocked. Boundary snapshots alone cannot satisfy this.
		found := false
		deadline := time.Now().Add(2 * time.Second)
		for !found && time.Now().Before(deadline) {
			data, _ := os.ReadFile(artifact)
			for _, line := range strings.Split(string(data), "\n") {
				var sample retentionSample
				if json.Unmarshal([]byte(line), &sample) == nil && sample.Phase == "natural" && sample.Stage == "pool_priming" && sample.Pool.InUse == 1 {
					found = true
					if sample.Counts.Cycles != 0 {
						t.Error("setup inflated measured count")
					}
				}
			}
			if !found {
				time.Sleep(time.Millisecond)
			}
		}
		close(release)
		run := <-finished
		if !found || run.err != nil || run.result.Completed != 2 || fake.closed.Load() != fake.opened.Load() {
			t.Fatalf("early sampling=%v result=%+v err=%v", found, run.result, run.err)
		}
		data, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		var stages []string
		active := ""
		poolClosed := false
		for {
			var record struct {
				Phase, Stage, Status string
				PoolAvailable        bool `json:"pool_available"`
			}
			if err := decoder.Decode(&record); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if record.Phase == "stage_start" {
				if active != "" {
					t.Fatalf("overlapping stages: %s/%s", active, record.Stage)
				}
				active = record.Stage
				stages = append(stages, record.Stage)
			}
			if record.Phase == "stage_end" {
				if active != record.Stage || record.Status != "complete" {
					t.Fatalf("unmatched/failed stage: %+v", record)
				}
				active = ""
				poolClosed = record.Stage == "database_cleanup"
			}
			if poolClosed && record.PoolAvailable {
				t.Fatal("sampled closed pool")
			}
		}
		want := "database_initialization,pool_priming,fixture_checks,warm_up,baseline_checkpoint,measured_workload,final_fixture_verification,final_checkpoint,database_cleanup"
		if active != "" || strings.Join(stages, ",") != want {
			t.Fatalf("stage order: %v", stages)
		}
		// Once the runner returns there must be no collector left writing.
		time.Sleep(10 * time.Millisecond)
		after, err := os.ReadFile(artifact)
		if err != nil || string(after) != string(data) {
			t.Fatal("artifact changed after runner returned")
		}
	})
}

func (retentionBrokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("artifact disk failure")
}

func TestResourceRetentionTLSAndMixedSchedule(t *testing.T) {
	for _, workers := range []int{1, 4, 16} {
		t.Run(fmt.Sprintf("mixed warm-up workers=%d", workers), func(t *testing.T) {
			fake := &retentionFakeConnector{}
			var queries [3]atomic.Int64 // complex, LOB, JSON
			fake.request = func(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
				kind, columns, index := "complex_select", 17, 0
				if strings.Contains(query, "ORA_STRESS_LOB") {
					kind, columns, index = "lob_select", 3, 1
				} else if strings.Contains(query, "ORA_STRESS_JSON") {
					kind, columns, index = "json_select", 2, 2
				}
				queries[index].Add(1)
				return &retentionSyntheticRows{columns: make([]string, columns), count: 3, closed: &fake.rowsClosed, row: func(id int) []driver.Value { return retentionExpectedFakeRow(kind, id, 3) }}, ctx.Err()
			}
			db := sql.OpenDB(fake)
			defer db.Close()
			db.SetMaxOpenConns(workers)
			config := oracleTest.ResourceRetentionConfig{Workers: workers, TotalOperations: workers, Workload: "mixed", RowsPerQuery: 3, OperationTimeout: "1s"}
			before := config
			want := map[string]int{}
			for worker := 0; worker < workers; worker++ {
				want[retentionVariant("mixed", worker, 0)]++
			}
			for _, kind := range retentionWorkloadKinds("mixed") {
				if want[kind] == 0 {
					want[kind] = 1
				}
			}
			check := func() {
				t.Helper()
				expected := [3]int{want["complex_select"] + 2*want["prepared_select"] + want["transaction_commit"] + want["transaction_rollback"], want["lob_select"] + want["connection_checkout"], want["json_select"]}
				for index, count := range expected {
					if queries[index].Load() != int64(count) {
						t.Fatalf("query class %d count=%d want=%d", index, queries[index].Load(), count)
					}
				}
				if fake.statementsClosed.Load() != int64(want["prepared_select"]) || fake.committed.Load() != int64(want["transaction_commit"]) || fake.rolledBack.Load() != int64(want["transaction_rollback"]) || db.Stats().InUse != 0 {
					t.Fatal("mixed warm-up omitted a lifecycle or retained a checkout")
				}
			}
			if err := warmResourceRetention(context.Background(), db, config); err != nil {
				t.Fatal(err)
			}
			check()
			if config != before {
				t.Fatal("warm-up changed selected configuration")
			}
			p := &retentionProgress{}
			_, _, err := runRetentionEpoch(context.Background(), config, make([]int, workers), 0, p, func(ctx context.Context, worker, index int) (retentionCounts, error) {
				if index != 0 {
					return retentionCounts{}, errors.New("warm-up advanced measured indices")
				}
				return executeRetentionCycle(ctx, db, config, worker, index)
			})
			if err != nil || p.snapshot().Cycles != uint64(workers) {
				t.Fatalf("warm-up inflated measured work: %+v %v", p.snapshot(), err)
			}
			for worker := 0; worker < workers; worker++ {
				want[retentionVariant("mixed", worker, 0)]++
			}
			check()
		})
	}
	t.Run("standalone LOB warm-up and one measured query", func(t *testing.T) {
		fake := retentionFixtureFake("lob_select", 3)
		db := sql.OpenDB(fake)
		defer db.Close()
		config := oracleTest.ResourceRetentionConfig{Workers: 1, Workload: "lob_select", RowsPerQuery: 3, OperationTimeout: "1s"}
		if err := warmResourceRetention(context.Background(), db, config); err != nil {
			t.Fatal(err)
		}
		count, err := executeRetentionCycle(context.Background(), db, config, 0, 0)
		if err != nil || count.Cycles != 1 || fake.queries.Load() != 2 || fake.rowsClosed.Load() != 2 || db.Stats().InUse != 0 {
			t.Fatalf("duplicate LOB setup/cleanup: %+v queries=%d err=%v", count, fake.queries.Load(), err)
		}
	})
	c := &TestConfig{}
	if validateRetentionTLS(c) == nil {
		t.Fatal("TCP accepted")
	}
	c.Database.Protocol = "tcps"
	if validateRetentionTLS(c) == nil {
		t.Fatal("missing identity policy accepted")
	}
	c.Security.SslServerDnMatch = "on"
	if err := validateRetentionTLS(c); err != nil {
		t.Fatal(err)
	}
	c.Security.SslAllowWeakDnMatch = "on"
	if validateRetentionTLS(c) == nil {
		t.Fatal("weak matching accepted")
	}
	counts := map[string]int{}
	for i := 0; i < 10; i++ {
		counts[retentionVariant("mixed", 0, i)]++
	}
	if counts["complex_select"] != 2 || counts["lob_select"] != 2 || counts["prepared_select"] != 2 || len(counts) != 7 {
		t.Fatal(counts)
	}
	if retentionVariant("mixed", 1, 0) == retentionVariant("mixed", 0, 0) {
		t.Fatal("worker offsets missing")
	}
	for _, stage := range []string{"success", "tcp", "protocol query", "protocol close", "tag"} {
		t.Run(stage, func(t *testing.T) {
			fake := &retentionFakeConnector{}
			cause := errors.New(stage)
			fake.request = func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				if stage == "protocol query" {
					return nil, cause
				}
				protocol := "tcps"
				if stage == "tcp" {
					protocol = "tcp"
				}
				r := &retentionSyntheticRows{columns: []string{"protocol"}, count: 1, closed: &fake.rowsClosed, row: func(int) []driver.Value { return []driver.Value{protocol} }}
				if stage == "protocol close" {
					r.closeError = cause
				}
				return r, nil
			}
			fake.exec = func(context.Context, string, []driver.NamedValue) error {
				if stage == "tag" {
					return cause
				}
				return nil
			}
			conn, err := (retentionTaggedConnector{Connector: fake, runID: "test"}).Connect(context.Background())
			if stage == "success" {
				if err != nil || conn == nil {
					t.Fatal(err)
				}
				conn.Close()
			} else {
				if err == nil || conn != nil || fake.closed.Load() != 1 {
					t.Fatalf("failed TLS setup cleanup: conn=%v err=%v", conn, err)
				}
			}
		})
	}
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
	query                                                                     func(context.Context, int64) (driver.Rows, error)
	request                                                                   func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
	exec                                                                      func(context.Context, string, []driver.NamedValue) error
	prepareError, statementCloseError, beginError, commitError, rollbackError error
	statementsClosed, committed, rolledBack                                   atomic.Int64
	connect                                                                   func(context.Context, int64) error
	onClose                                                                   func()
	closeError                                                                error
	opened                                                                    atomic.Int64
	closed                                                                    atomic.Int64
	queries                                                                   atomic.Int64
	rowsClosed                                                                atomic.Int64
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

func (c *retentionFakeConn) Prepare(query string) (driver.Stmt, error) {
	if c.connector.request == nil {
		return nil, errors.New("unexpected Prepare")
	}
	if c.connector.prepareError != nil {
		return nil, c.connector.prepareError
	}
	return &retentionFakeStmt{conn: c, query: query}, nil
}
func (c *retentionFakeConn) Begin() (driver.Tx, error) {
	if c.connector.request == nil {
		return nil, errors.New("unexpected Begin")
	}
	if c.connector.beginError != nil {
		return nil, c.connector.beginError
	}
	return &retentionFakeTx{connector: c.connector}, nil
}
func (c *retentionFakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (c *retentionFakeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.connector.exec == nil {
		return nil, errors.New("unexpected Exec")
	}
	err := c.connector.exec(ctx, query, args)
	return driver.RowsAffected(0), err
}
func (c *retentionFakeConn) Close() error {
	c.connector.closed.Add(1)
	if c.connector.onClose != nil {
		c.connector.onClose()
	}
	return c.connector.closeError
}
func (c *retentionFakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.connector.request != nil {
		c.connector.queries.Add(1)
		return c.connector.request(ctx, query, args)
	}
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
	config.OutputDirectory = t.TempDir()
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
				if runErr != nil || result.Completed != 12 || connector.opened.Load() != 4 || connector.closed.Load() != 4 || connector.queries.Load() != 20 || connector.rowsClosed.Load() != 20 {
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
