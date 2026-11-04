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
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

// Resource-retention entry point and worker/pool helpers.

type resourceRetentionHeap struct {
	HeapAlloc   uint64
	HeapObjects uint64
}

type resourceRetentionResult struct {
	Completed                  int
	Duration                   time.Duration
	Baseline                   resourceRetentionHeap
	Final                      resourceRetentionHeap
	MemoryLimitBytes           int64
	Counts                     retentionCounts
	Drain                      time.Duration
	Pauses                     time.Duration
	Wall                       time.Duration
	Artifact                   string
	BaselineCheckpointComplete bool
	FinalCheckpointComplete    bool
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
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := validateRetentionTLS(TestingConfig); err != nil {
		t.Fatal(err)
	}
	config = config.Normalized()
	timeout, _ := time.ParseDuration(config.Timeout)
	if deadline, ok := t.Deadline(); ok && time.Until(deadline) <= timeout {
		t.Fatalf("Go test timeout must exceed JSON timeout %s", config.Timeout)
	}
	runID := newRetentionRunID()
	t.Logf("starting run_id=%s workload=%s artifacts=%s (startup metadata is outside heap checkpoints)", runID, config.Workload, config.OutputDirectory)
	result, err := executeExpandedResourceRetention(config, runID, TestingConfig, func(ctx context.Context) (*sql.DB, error) {
		connector, err := openTestConnectorWithConfig(TestingConfig)
		if err != nil {
			return nil, err
		}
		// OpenDB is lazy. Warm-up below bounds physical connection creation with
		// the configured context, unlike the existing helper's unbounded Ping.
		return sql.OpenDB(retentionTaggedConnector{Connector: connector, runID: runID}), nil
	})
	// Delay reporting until after the final sample so these log strings do not
	// themselves become retained allocations in the measured workload.
	target := "unchanged"
	if config.MemoryLimitMiB != nil {
		target = fmt.Sprint(*config.MemoryLimitMiB)
	}
	mode, requested := "count", fmt.Sprint(config.TotalOperations)
	if config.Duration != "" {
		mode, requested = "duration", config.Duration
	}
	t.Logf("configuration: mode=%s workers=%d total_operations=%d duration=%s max_open_connections=%d timeout=%s soft_go_memory_target_mib=%s effective_go_memory_limit_bytes=%d go=%s",
		mode, config.Workers, config.TotalOperations, config.Duration, config.MaxOpenConnections, config.Timeout, target, result.MemoryLimitBytes, runtime.Version())
	t.Logf("result: mode=%s requested=%s completed=%d workers=%d workload_elapsed=%s soft_go_memory_target_mib=%s baseline_checkpoint_complete=%t final_checkpoint_complete=%t baseline_heap_alloc=%d final_heap_alloc=%d baseline_heap_objects=%d final_heap_objects=%d diagnostic_only=true",
		mode, requested, result.Completed, config.Workers, result.Duration, target, result.BaselineCheckpointComplete, result.FinalCheckpointComplete,
		result.Baseline.HeapAlloc, result.Final.HeapAlloc, result.Baseline.HeapObjects, result.Final.HeapObjects)
	if err != nil {
		t.Fatal(redactRetentionText(err.Error(), TestingConfig))
	}
	t.Logf("cycles=%d SELECTs=%d rows=%d payload_bytes=%d workload_elapsed=%s drain=%s checkpoint_pause=%s wall=%s artifacts=%s", result.Counts.Cycles, result.Counts.Selects, result.Counts.Rows, result.Counts.PayloadBytes, result.Duration, result.Drain, result.Pauses, result.Wall, result.Artifact)
}

func resourceRetentionStressEnabled() bool {
	for _, category := range oracleTest.TestCategories {
		if strings.EqualFold(strings.TrimSpace(category), "stress") {
			return true
		}
	}
	return false
}

func primeResourceRetentionPool(ctx context.Context, db *sql.DB, count int, operationTimeout ...time.Duration) (err error) {
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
		if err := func() error {
			opCtx, cancel := retentionOperationContext(ctx, operationTimeout...)
			defer cancel()
			conn, err := db.Conn(opCtx)
			if err != nil {
				return fmt.Errorf("acquire warm-up connection %d: %w", index+1, err)
			}
			connections = append(connections, conn)
			if err := executeSelectDual(opCtx, conn); err != nil {
				return fmt.Errorf("query warm-up connection %d: %w", index+1, err)
			}
			return opCtx.Err()
		}(); err != nil {
			return err
		}
	}
	return nil
}

// Each setup operation gets its own budget, bounded by the overall deadline.
// Omitted budgets preserve existing functional-helper callers' parent context.
func retentionOperationContext(ctx context.Context, timeout ...time.Duration) (context.Context, context.CancelFunc) {
	if len(timeout) != 0 {
		return context.WithTimeout(ctx, timeout[0])
	}
	return context.WithCancel(ctx)
}

func resourceRetentionWorkerOperations(total, workers, worker int) int {
	count := total / workers
	if worker < total%workers {
		count++
	}
	return count
}

// Warm-up uses the same worker runner, but separate counts and schedule indices.
// Mixed runs validate any variant not covered by workers' first cycles once.
func warmResourceRetention(ctx context.Context, db *sql.DB, config oracleTest.ResourceRetentionConfig) error {
	warm := config
	warm.Duration, warm.TotalOperations = "", config.Workers
	_, _, err := runRetentionEpoch(ctx, warm, make([]int, warm.Workers), 0, &retentionProgress{}, func(ctx context.Context, worker, index int) (retentionCounts, error) {
		return executeRetentionCycle(ctx, db, config, worker, index)
	})
	if err != nil || config.Workload != "mixed" {
		return err
	}
	covered := make(map[string]bool)
	for worker := 0; worker < config.Workers; worker++ {
		covered[retentionVariant("mixed", worker, 0)] = true
	}
	warm.Workers, warm.TotalOperations = 1, 1
	for _, kind := range retentionWorkloadKinds("mixed") {
		if covered[kind] {
			continue
		}
		warm.Workload = kind
		_, _, err = runRetentionEpoch(ctx, warm, []int{0}, 0, &retentionProgress{}, func(ctx context.Context, worker, index int) (retentionCounts, error) {
			return executeRetentionCycle(ctx, db, warm, worker, index)
		})
		if err != nil {
			return fmt.Errorf("warm-up %s: %w", kind, err)
		}
	}
	return nil
}

// Large-data workloads, result validation, fixture preflight, and TLS helpers.

const retentionFixtureVersion = 1
const retentionLOBBytes = 1 << 20
const retentionJSONPadding = 16 << 10

var retentionMixedSchedule = [...]string{
	"complex_select", "lob_select", "prepared_select", "transaction_commit", "json_select",
	"complex_select", "lob_select", "prepared_select", "transaction_rollback", "connection_checkout",
}

type retentionCounts struct {
	Cycles       uint64 `json:"cycles"`
	Selects      uint64 `json:"selects"`
	Rows         uint64 `json:"rows"`
	PayloadBytes uint64 `json:"payload_bytes"`
}

func (c *retentionCounts) add(v retentionCounts) {
	c.Cycles += v.Cycles
	c.Selects += v.Selects
	c.Rows += v.Rows
	c.PayloadBytes += v.PayloadBytes
}

func retentionVariant(workload string, worker, operation int) string {
	if workload == "mixed" {
		return retentionMixedSchedule[(worker%len(retentionMixedSchedule)+operation%len(retentionMixedSchedule))%len(retentionMixedSchedule)]
	}
	return workload
}

func retentionTable(config oracleTest.ResourceRetentionConfig, table string) string {
	if config.FixtureSchema != "" {
		return strings.ToUpper(config.FixtureSchema) + "." + table
	}
	return table
}

// All SQL is owned by the test. Only the centrally validated schema identifier
// is interpolated; row limits are binds, never configuration-supplied SQL.
func retentionQuery(config oracleTest.ResourceRetentionConfig, kind string) string {
	core := retentionTable(config, "ORA_STRESS_CORE")
	if kind == "lob_select" {
		return "SELECT c.id, p.blob_value, p.clob_value FROM " + core + " c JOIN " + retentionTable(config, "ORA_STRESS_LOB") + " p ON p.id=c.id WHERE c.id BETWEEN :first_id AND :last_id ORDER BY c.id"
	}
	if kind == "json_select" {
		return "SELECT c.id, j.document FROM " + core + " c JOIN " + retentionTable(config, "ORA_STRESS_JSON") + " j ON j.id=c.id WHERE c.id BETWEEN :first_id AND :last_id ORDER BY c.id"
	}
	// This checkout routes queries by their leading keyword and rejects WITH.
	// Keep the filtered input as an inline view so the statement starts SELECT.
	return `SELECT c.id, c.group_id, g.group_name, c.int_value, c.decimal_value,
 c.float_value, c.double_value, c.text_value, c.nchar_value, c.unicode_value,
 c.raw_value, c.date_value, c.timestamp_value, c.tz_value,
 CASE WHEN c.unicode_value IS NULL THEN 'NULL' ELSE 'VALUE' END,
 ROW_NUMBER() OVER (ORDER BY c.id), SUM(c.int_value) OVER (PARTITION BY c.group_id)
FROM (SELECT * FROM ` + core + ` WHERE id BETWEEN :first_id AND :last_id) c
JOIN ` + retentionTable(config, "ORA_STRESS_GROUPS") + ` g ON g.group_id=c.group_id ORDER BY c.id`
}

// A cycle owns one lifecycle. Its local defers finish before another cycle
// starts; no Rows, Stmt, Tx, Conn, or large payload is stored by the runner.
func executeRetentionCycle(ctx context.Context, db *sql.DB, config oracleTest.ResourceRetentionConfig, worker, operation int) (count retentionCounts, err error) {
	kind := retentionVariant(config.Workload, worker, operation)
	if kind == "select_dual" {
		err = executeSelectDual(ctx, db)
		if err == nil {
			count = retentionCounts{Cycles: 1, Selects: 1, Rows: 1}
		}
		return
	}
	query := retentionQuery(config, "complex_select")
	switch kind {
	case "prepared_select":
		// Bind the statement to a normal pooled checkout. database/sql suppresses
		// some driver statement-close errors for DB-level cached statements; a
		// Conn-bound statement exposes the close result we need to check.
		conn, acquireErr := db.Conn(ctx)
		if acquireErr != nil {
			return count, fmt.Errorf("prepared checkout: %w", acquireErr)
		}
		defer func() { err = errors.Join(err, retentionCleanup("prepared connection", conn.Close())) }()
		stmt, prepareErr := conn.PrepareContext(ctx, query)
		if prepareErr != nil {
			return count, fmt.Errorf("prepare: %w", prepareErr)
		}
		defer func() { err = errors.Join(err, retentionCleanup("statement", stmt.Close())) }()
		for repeat := 0; repeat < 2; repeat++ {
			var c retentionCounts
			c, err = consumeRetentionQuery(ctx, stmt, "", "complex_select", config.RowsPerQuery)
			if err != nil {
				return
			}
			count.add(c)
		}
	case "transaction_commit", "transaction_rollback":
		tx, beginErr := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
		if beginErr != nil {
			return count, fmt.Errorf("begin transaction: %w", beginErr)
		}
		ended := false
		defer func() {
			if !ended {
				e := tx.Rollback()
				if !errors.Is(e, sql.ErrTxDone) {
					err = errors.Join(err, retentionCleanup("rollback after failure", e))
				}
			}
		}()
		count, err = consumeRetentionQuery(ctx, tx, query, "complex_select", config.RowsPerQuery)
		if err != nil {
			return
		}
		if kind == "transaction_commit" {
			err = retentionCleanup("commit", tx.Commit())
		} else {
			err = retentionCleanup("rollback", tx.Rollback())
		}
		ended = err == nil
	case "connection_checkout":
		conn, acquireErr := db.Conn(ctx)
		if acquireErr != nil {
			return count, fmt.Errorf("checkout: %w", acquireErr)
		}
		defer func() { err = errors.Join(err, retentionCleanup("return connection", conn.Close())) }()
		count, err = consumeRetentionQuery(ctx, conn, retentionQuery(config, "lob_select"), "lob_select", config.RowsPerQuery)
	default:
		count, err = consumeRetentionQuery(ctx, db, retentionQuery(config, kind), kind, config.RowsPerQuery)
	}
	if err == nil {
		count.Cycles = 1
	}
	return
}

func retentionCleanup(resource string, err error) error {
	if err == nil {
		return nil
	}
	return &retentionCleanupError{resource: resource, err: err}
}

// A failed close/rollback is independent evidence, even when it wraps cancellation.
type retentionCleanupError struct {
	resource string
	err      error
}

func (e *retentionCleanupError) Error() string {
	return fmt.Sprintf("cleanup %s: %v", e.resource, e.err)
}
func (e *retentionCleanupError) Unwrap() error { return e.err }

func retentionCancellationOnly(err, primary error) bool {
	if _, cleanup := err.(*retentionCleanupError); cleanup {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !retentionCancellationOnly(child, primary) {
				return false
			}
		}
		return true
	}
	if child := errors.Unwrap(err); child != nil {
		if primary != nil && errors.Is(err, primary) && !errors.Is(child, primary) {
			return true
		}
		return retentionCancellationOnly(child, primary)
	}
	return errors.Is(err, context.Canceled) || primary != nil && errors.Is(err, primary)
}

// Split explicit joins only. Keep unfamiliar mixed wrappers intact so filtering
// cannot discard an independent cause or its original error type/stage context.
func retentionIndependentError(err, primary error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var independent error
		for _, child := range joined.Unwrap() {
			independent = errors.Join(independent, retentionIndependentError(child, primary))
		}
		return independent
	}
	if retentionCancellationOnly(err, primary) {
		return nil
	}
	return err
}

// Stmt.QueryContext lacks the SQL argument used by DB/Tx/Conn.QueryContext.
// Adapt it here without changing the production driver or its interfaces.
func consumeRetentionQuery(ctx context.Context, source any, query, kind string, expected int) (count retentionCounts, err error) {
	args := []any{sql.Named("first_id", 1), sql.Named("last_id", expected)}
	var rows *sql.Rows
	if stmt, ok := source.(*sql.Stmt); ok {
		rows, err = stmt.QueryContext(ctx, args...)
	} else {
		rows, err = source.(selectDualQueryer).QueryContext(ctx, query, args...)
	}
	if err != nil {
		return count, fmt.Errorf("query %s: %w", kind, err)
	}
	defer func() { err = errors.Join(err, retentionCleanup("rows", rows.Close())) }()
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return
		}
		index := int(count.Rows) + 1
		if index > expected {
			return count, fmt.Errorf("%s returned extra rows", kind)
		}
		var bytes uint64
		bytes, err = validateRetentionRow(rows, kind, index, expected)
		if err != nil {
			return count, fmt.Errorf("%s row %d: %w", kind, index, err)
		}
		count.Rows++
		count.PayloadBytes += bytes
	}
	if err = rows.Err(); err != nil {
		return count, fmt.Errorf("iterate %s: %w", kind, err)
	}
	if int(count.Rows) != expected {
		return count, fmt.Errorf("%s returned %d rows, want %d", kind, count.Rows, expected)
	}
	if err = ctx.Err(); err != nil {
		return
	}
	count.Selects = 1
	return
}

func validateRetentionRow(rows *sql.Rows, kind string, expected, rowCount int) (uint64, error) {
	if kind == "lob_select" {
		var id int
		var blob []byte
		var clob string
		if err := rows.Scan(&id, &blob, &clob); err != nil {
			return 0, fmt.Errorf("scan: %w", err)
		}
		if id != expected {
			return 0, fmt.Errorf("id=%d want %d", id, expected)
		}
		if len(blob) != retentionLOBBytes || len(clob) != retentionLOBBytes {
			return 0, fmt.Errorf("LOB lengths=%d/%d want %d", len(blob), len(clob), retentionLOBBytes)
		}
		if sha256.Sum256(blob) != retentionRepeatedDigest(byte(id%251), retentionLOBBytes) || retentionStringDigest(clob) != retentionRepeatedDigest(byte('a'+id%26), retentionLOBBytes) {
			return 0, errors.New("LOB checksum mismatch")
		}
		return uint64(len(blob) + len(clob)), nil
	}
	if kind == "json_select" {
		var id int
		var document string
		if err := rows.Scan(&id, &document); err != nil {
			return 0, fmt.Errorf("scan: %w", err)
		}
		if id != expected {
			return 0, fmt.Errorf("id=%d want %d", id, expected)
		}
		if err := validateRetentionJSON(document, id); err != nil {
			return 0, err
		}
		return uint64(len(document)), nil
	}
	var id, group, integer, sequence, total int64
	var groupName, decimal, text, nchar, state string
	var unicode sql.NullString
	var floatValue, doubleValue float64
	var raw []byte
	var date, timestamp, tz time.Time
	if err := rows.Scan(&id, &group, &groupName, &integer, &decimal, &floatValue, &doubleValue, &text, &nchar, &unicode, &raw, &date, &timestamp, &tz, &state, &sequence, &total); err != nil {
		return 0, fmt.Errorf("scan: %w", err)
	}
	g := int64((expected-1)%8 + 1)
	wantTotal := int64(0)
	for i := int(g); i <= rowCount; i += 8 {
		wantTotal += int64(i)
	}
	d, ok := new(big.Rat).SetString(decimal)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(expected) * time.Second)
	if id != int64(expected) || group != g || integer != id || sequence != id || total != wantTotal || groupName != fmt.Sprintf("group-%d", g) || !ok || d.Cmp(big.NewRat(id*1001, 1000)) != 0 || floatValue != float64(id)/4 || doubleValue != float64(id)/8 {
		return 0, errors.New("scalar/analytic mismatch")
	}
	if len(text) != 2000 || retentionStringDigest(text) != retentionRepeatedDigest(byte('a'+expected%26), 2000) || nchar != fmt.Sprintf("Ω%06d ", expected) || len(raw) != 32 {
		return 0, errors.New("text/RAW mismatch")
	}
	for _, b := range raw {
		if b != byte(expected%251) {
			return 0, errors.New("RAW checksum mismatch")
		}
	}
	if expected%16 == 0 {
		if unicode.Valid || state != "NULL" {
			return 0, errors.New("NULL mismatch")
		}
	} else if !unicode.Valid || unicode.String != fmt.Sprintf("東京-%d", expected) || state != "VALUE" {
		return 0, errors.New("Unicode mismatch")
	}
	_, offset := tz.Zone()
	// DATE/TIMESTAMP contain no zone; compare calendar fields, not an invented
	// location. TIMESTAMP WITH TIME ZONE must preserve instant AND offset.
	if date.Format("2006-01-02T15:04:05.999999999") != base.Format("2006-01-02T15:04:05.999999999") || timestamp.Format("2006-01-02T15:04:05.999999999") != base.Add(123456789*time.Nanosecond).Format("2006-01-02T15:04:05.999999999") || !tz.Equal(base.Add(123456789*time.Nanosecond)) || offset != 19800 {
		return 0, errors.New("date/time mismatch")
	}
	return uint64(len(text) + len(nchar) + len(unicode.String) + len(raw)), nil
}

// Hash expected data in fixed-size chunks rather than constructing another
// MiB payload per row. Hash strings without a full []byte copy.
func retentionRepeatedDigest(value byte, length int) [32]byte {
	var chunk [4096]byte
	for i := range chunk {
		chunk[i] = value
	}
	h := sha256.New()
	for length > 0 {
		n := min(length, len(chunk))
		_, _ = h.Write(chunk[:n])
		length -= n
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}
func retentionStringDigest(value string) [32]byte {
	h := sha256.New()
	// hash.Hash does not implement StringWriter; io.WriteString on the whole
	// payload would allocate a second MiB-sized buffer. Bound conversion to 4 KiB.
	for len(value) > 0 {
		n := min(len(value), 4096)
		_, _ = io.WriteString(h, value[:n])
		value = value[n:]
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func validateRetentionJSON(document string, id int) error {
	var v struct {
		ID     int `json:"id"`
		Nested struct {
			Name string `json:"name"`
		} `json:"nested"`
		Items   []int  `json:"items"`
		Active  bool   `json:"active"`
		Padding string `json:"padding"`
	}
	if err := json.Unmarshal([]byte(document), &v); err != nil {
		return fmt.Errorf("JSON decode: %w", err)
	}
	if v.ID != id || v.Nested.Name != fmt.Sprintf("row-%d", id) || !v.Active || len(v.Items) != 3 || v.Items[0] != id || v.Items[1] != id+1 || v.Items[2] != id+2 || len(v.Padding) != retentionJSONPadding || retentionStringDigest(v.Padding) != retentionRepeatedDigest(byte('a'+id%26), retentionJSONPadding) {
		return errors.New("JSON semantic/payload mismatch")
	}
	return nil
}

func validateRetentionTLS(config *TestConfig) error {
	if config == nil || !strings.EqualFold(strings.TrimSpace(config.Database.Protocol), "tcps") {
		return errors.New("stress requires database.protocol=tcps; TCP is not TLS signoff coverage")
	}
	if !strings.EqualFold(config.Security.SslServerDnMatch, "on") && !strings.EqualFold(config.Security.SslServerDnMatch, "true") {
		return errors.New("stress requires explicit ssl_server_dn_match=on")
	}
	if config.Security.SslAllowWeakDnMatch != "" && !strings.EqualFold(config.Security.SslAllowWeakDnMatch, "off") && !strings.EqualFold(config.Security.SslAllowWeakDnMatch, "false") {
		return errors.New("stress requires weak DN matching disabled")
	}
	return nil
}

// Tag and verify every physical connection, including replacements. Returning
// the original Conn preserves all optional database/sql driver interfaces.
type retentionTaggedConnector struct {
	driver.Connector
	runID string
}

func (c retentionTaggedConnector) Connect(ctx context.Context) (conn driver.Conn, err error) {
	conn, err = c.Connector.Connect(ctx)
	defer func() {
		if err != nil && conn != nil {
			err = errors.Join(err, retentionCleanup("failed session initialization", conn.Close()))
			conn = nil
		}
	}()
	if err != nil {
		return
	}
	if conn == nil {
		return nil, errors.New("connector returned nil connection")
	}
	queryer, ok := conn.(driver.QueryerContext)
	if !ok {
		return conn, errors.New("connection lacks QueryerContext")
	}
	rows, e := queryer.QueryContext(ctx, "SELECT SYS_CONTEXT('USERENV', 'NETWORK_PROTOCOL') FROM DUAL", nil)
	if e != nil {
		if rows != nil {
			e = errors.Join(e, retentionCleanup("protocol rows", rows.Close()))
		}
		return conn, fmt.Errorf("verify network protocol: %w", e)
	}
	if rows == nil {
		return conn, errors.New("protocol query returned nil rows")
	}
	if len(rows.Columns()) != 1 {
		return conn, errors.Join(errors.New("protocol query must return one column"), retentionCleanup("protocol rows", rows.Close()))
	}
	values := make([]driver.Value, 1)
	err = rows.Next(values)
	if err == nil && !strings.EqualFold(fmt.Sprint(values[0]), "tcps") {
		err = errors.New("server connection is not TCPS")
	}
	if err == nil {
		e = rows.Next(values)
		if e != io.EOF {
			err = fmt.Errorf("unexpected protocol rows: %v", e)
		}
	}
	err = errors.Join(err, retentionCleanup("protocol rows", rows.Close()))
	if err != nil {
		return
	}
	execer, ok := conn.(driver.ExecerContext)
	if !ok {
		return conn, errors.New("connection lacks ExecerContext for session tagging")
	}
	_, err = execer.ExecContext(ctx, `BEGIN DBMS_SESSION.SET_IDENTIFIER(:run_id); DBMS_APPLICATION_INFO.SET_MODULE('go-retention-stress', 'workload'); END;`, []driver.NamedValue{{Name: "run_id", Ordinal: 1, Value: c.runID}})
	if err != nil {
		err = fmt.Errorf("tag session: %w", err)
	}
	return
}

// Functional baseline deliberately consumes the same provisioned fixtures as
// stress. Missing fixtures are an actionable failure, not a silent skip.
func TestDriver_ResourceRetentionLargeDataBaseline(t *testing.T) {
	if TestingConfig == nil || TestingConfig.Stress == nil || TestingConfig.Stress.ResourceRetention == nil {
		t.Skip("requires provisioned retention profile")
	}
	config := TestingConfig.Stress.ResourceRetention.Normalized()
	if config.Workload == "select_dual" {
		t.Skip("requires expanded workload profile")
	}
	if err := validateRetentionTLS(TestingConfig); err != nil {
		t.Fatal(err)
	}
	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(retentionTaggedConnector{Connector: connector, runID: newRetentionRunID()})
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	timeout, _ := time.ParseDuration(config.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := preflightRetentionFixtures(ctx, db, config); err != nil {
		t.Fatal(err)
	}
	for _, kind := range retentionWorkloadKinds(config.Workload) {
		t.Run(kind, func(t *testing.T) {
			opTimeout, _ := time.ParseDuration(config.OperationTimeout)
			opCtx, done := retentionOperationContext(ctx, opTimeout)
			defer done()
			c := config
			c.Workload = kind
			if _, err := executeRetentionCycle(opCtx, db, c, 0, 0); err != nil {
				t.Fatal(err)
			}
			if err := opCtx.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func retentionWorkloadKinds(workload string) []string {
	if workload == "mixed" {
		return []string{"complex_select", "lob_select", "json_select", "prepared_select", "transaction_commit", "transaction_rollback", "connection_checkout"}
	}
	return []string{workload}
}

func preflightRetentionFixtures(ctx context.Context, db *sql.DB, config oracleTest.ResourceRetentionConfig) error {
	if config.Workload == "select_dual" {
		return nil
	}
	operationTimeout, _ := time.ParseDuration(config.Normalized().OperationTimeout)
	// Metadata checks distinguish native JSON from a CLOB/text substitute.
	required := map[string]map[string]string{
		"ORA_STRESS_CORE":   {"ID": "NUMBER", "GROUP_ID": "NUMBER", "FIXTURE_VERSION": "NUMBER", "INT_VALUE": "NUMBER", "DECIMAL_VALUE": "NUMBER", "FLOAT_VALUE": "BINARY_FLOAT", "DOUBLE_VALUE": "BINARY_DOUBLE", "TEXT_VALUE": "VARCHAR2", "NCHAR_VALUE": "NCHAR", "UNICODE_VALUE": "NVARCHAR2", "RAW_VALUE": "RAW", "DATE_VALUE": "DATE", "TIMESTAMP_VALUE": "TIMESTAMP(9)", "TZ_VALUE": "TIMESTAMP(9) WITH TIME ZONE"},
		"ORA_STRESS_GROUPS": {"GROUP_ID": "NUMBER", "GROUP_NAME": "VARCHAR2"},
	}
	for _, kind := range retentionWorkloadKinds(config.Workload) {
		if kind == "lob_select" || kind == "connection_checkout" {
			required["ORA_STRESS_LOB"] = map[string]string{"ID": "NUMBER", "BLOB_VALUE": "BLOB", "CLOB_VALUE": "CLOB"}
		}
		if kind == "json_select" {
			required["ORA_STRESS_JSON"] = map[string]string{"ID": "NUMBER", "DOCUMENT": "JSON"}
		}
	}
	for table, columns := range required {
		opCtx, cancel := retentionOperationContext(ctx, operationTimeout)
		err := checkRetentionFixtureColumns(opCtx, db, config, table, columns)
		cancel()
		if err != nil {
			return err
		}
	}
	var count, minID, maxID, minVersion, maxVersion int
	opCtx, cancel := retentionOperationContext(ctx, operationTimeout)
	err := errors.Join(db.QueryRowContext(opCtx, "SELECT COUNT(*), MIN(id), MAX(id), MIN(fixture_version), MAX(fixture_version) FROM "+retentionTable(config, "ORA_STRESS_CORE")).Scan(&count, &minID, &maxID, &minVersion, &maxVersion), opCtx.Err())
	cancel()
	if err != nil {
		return fmt.Errorf("fixture identity: %w", err)
	}
	if count != 512 || minID != 1 || maxID != 512 || minVersion != retentionFixtureVersion || maxVersion != retentionFixtureVersion {
		return errors.New("fixture version/ID range mismatch; expected version 1 and IDs 1..512")
	}
	return nil
}

// Preflight reads metadata only. Full value checks belong to validated warm-up
// (or the functional baseline), not another duplicate large-data query.
func checkRetentionFixtureColumns(ctx context.Context, db *sql.DB, config oracleTest.ResourceRetentionConfig, table string, columns map[string]string) (err error) {
	rows, err := db.QueryContext(ctx, `SELECT column_name, data_type FROM all_tab_columns WHERE owner=COALESCE(:owner, SYS_CONTEXT('USERENV','CURRENT_SCHEMA')) AND table_name=:table_name`, sql.Named("owner", strings.ToUpper(config.FixtureSchema)), sql.Named("table_name", table))
	if err != nil {
		return fmt.Errorf("fixture metadata %s: %w", table, err)
	}
	defer func() { err = errors.Join(err, retentionCleanup("metadata rows", rows.Close())) }()
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return err
		}
		if expected, ok := columns[name]; ok {
			if kind != expected {
				return fmt.Errorf("%s.%s has type %s, want %s", table, name, kind, expected)
			}
			delete(columns, name)
		}
	}
	if err := errors.Join(rows.Err(), ctx.Err()); err != nil {
		return err
	}
	if len(columns) != 0 {
		return fmt.Errorf("missing fixture columns in %s: %v", table, columns)
	}
	return nil
}

// Setup is never registered with the category executor. This additional opt-in
// prevents ordinary test discovery from creating ~1 GiB of fixture LOB data.
var retentionFixtureSetupEnabled = flag.Bool("stress.fixture-setup", false,
	"explicitly create/populate new resource-retention fixtures in the selected account schema")

// TestDriver_ResourceRetentionFixtureSetup is a separate MANUAL provisioning
// operation, not part of the measured stress run. DDL commits independently.
// Failure never triggers DROP or replacement; inspect partial objects manually.
func TestDriver_ResourceRetentionFixtureSetup(t *testing.T) {
	if !*retentionFixtureSetupEnabled {
		t.Skip("manual setup requires explicit -stress.fixture-setup=true")
	}
	if TestingConfig == nil || TestingConfig.Stress == nil || TestingConfig.Stress.ResourceRetention == nil {
		t.Fatal("fixture setup requires a selected stress.resource_retention configuration")
	}
	config := TestingConfig.Stress.ResourceRetention.Normalized()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := validateRetentionTLS(TestingConfig); err != nil {
		t.Fatal(err)
	}
	const setupTimeout = 30 * time.Minute
	if deadline, ok := t.Deadline(); ok && time.Until(deadline) <= setupTimeout {
		t.Fatal("Go test timeout must exceed the 30-minute fixture-setup deadline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		t.Fatal(redactRetentionText(err.Error(), TestingConfig))
	}
	db := sql.OpenDB(retentionTaggedConnector{Connector: connector, runID: newRetentionRunID()})
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(redactRetentionText(err.Error(), TestingConfig))
		}
	}()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(redactRetentionText(err.Error(), TestingConfig))
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(redactRetentionText(err.Error(), TestingConfig))
		}
	}()
	if err := provisionResourceRetentionFixtures(ctx, conn, config, t.Logf); err != nil {
		t.Fatal(redactRetentionText(err.Error(), TestingConfig),
			"; setup stopped: already-created tables may remain; no objects were dropped")
	}
	t.Log("fixture setup complete: 512 core/LOB/JSON rows, 8 groups; run the large-data functional baseline next")
}

// Provision only the authenticated account's own ordinary schema. Read-only
// prechecks run before the first CREATE. Quota is a lower-bound capacity check,
// not a guarantee of free physical storage or enough overhead. No privilege
// grants, schema changes, cleanup drops, or automatic retries are performed.
func provisionResourceRetentionFixtures(ctx context.Context, conn *sql.Conn,
	config oracleTest.ResourceRetentionConfig, logf func(string, ...any)) (err error) {
	var user, schema, tablespace string
	if err = conn.QueryRowContext(ctx, "SELECT SYS_CONTEXT('USERENV','SESSION_USER'), SYS_CONTEXT('USERENV','CURRENT_SCHEMA'), (SELECT default_tablespace FROM user_users) FROM DUAL").Scan(&user, &schema, &tablespace); err != nil {
		return fmt.Errorf("setup schema preflight: %w", err)
	}
	if schema != user || (config.FixtureSchema != "" && !strings.EqualFold(config.FixtureSchema, schema)) {
		return errors.New("setup must use the authenticated account's own schema; fixture_schema must match it or be omitted")
	}
	config.FixtureSchema = schema
	if err = config.Validate(); err != nil {
		return fmt.Errorf("setup configuration: %w", err)
	}
	rows, err := conn.QueryContext(ctx, "SELECT object_name, object_type FROM user_objects WHERE object_name IN ('ORA_STRESS_GROUPS','ORA_STRESS_CORE','ORA_STRESS_LOB','ORA_STRESS_JSON') ORDER BY object_name")
	if err != nil {
		return fmt.Errorf("setup existing-object preflight: %w", err)
	}
	var existing []string
	for rows.Next() {
		var name, kind string
		if err = rows.Scan(&name, &kind); err != nil {
			break
		}
		existing = append(existing, name+" ("+kind+")")
	}
	err = errors.Join(err, rows.Err(), retentionCleanup("setup object rows", rows.Close()))
	if err != nil {
		return fmt.Errorf("setup object preflight: %w", err)
	}
	if len(existing) != 0 {
		return fmt.Errorf("refusing to replace existing fixture objects: %s", strings.Join(existing, ", "))
	}
	var canCreate, unlimited int
	if err = conn.QueryRowContext(ctx, "SELECT COUNT(CASE WHEN privilege IN ('CREATE TABLE','CREATE ANY TABLE') THEN 1 END), COUNT(CASE WHEN privilege='UNLIMITED TABLESPACE' THEN 1 END) FROM session_privs").Scan(&canCreate, &unlimited); err != nil {
		return fmt.Errorf("setup privilege preflight: %w", err)
	}
	if canCreate == 0 {
		return errors.New("fixture setup requires CREATE TABLE in the selected account")
	}
	if unlimited == 0 {
		var used, limit int64
		if err = conn.QueryRowContext(ctx, "SELECT bytes, max_bytes FROM user_ts_quotas WHERE tablespace_name=:tablespace", sql.Named("tablespace", tablespace)).Scan(&used, &limit); err != nil {
			return fmt.Errorf("setup quota preflight: no usable quota information for the default tablespace: %w", err)
		}
		if limit != -1 && (limit < used || limit-used < 1<<30) {
			return errors.New("remaining default-tablespace quota is below the ~1 GiB fixture payload requirement; additional overhead is also needed")
		}
		if limit == -1 {
			logf("fixture schema=%s default_tablespace=%s quota=unlimited; physical capacity and overhead are not guaranteed", schema, tablespace)
		} else {
			logf("fixture schema=%s default_tablespace=%s remaining_quota_bytes=%d; additional storage overhead is needed", schema, tablespace, limit-used)
		}
	} else {
		logf("fixture schema=%s default_tablespace=%s UNLIMITED TABLESPACE privilege present; physical capacity is not guaranteed", schema, tablespace)
	}
	// Check native-JSON-capable release before any DDL. Actual JSON DDL errors
	// still fail clearly; privileges/quota/version do not prove storage capacity.
	if _, err = conn.ExecContext(ctx, "BEGIN IF DBMS_DB_VERSION.VERSION < 21 THEN RAISE_APPLICATION_ERROR(-20001, 'Native JSON fixture requires Oracle 21c+'); END IF; END;"); err != nil {
		return fmt.Errorf("setup database release preflight: %w", err)
	}
	for _, fixture := range retentionFixtureDDL {
		if _, err = conn.ExecContext(ctx, fixture.sql); err != nil {
			return fmt.Errorf("create %s: %w", fixture.name, err)
		}
		logf("created %s.%s; DDL is already committed", schema, fixture.name)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fixture population: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, retentionCleanup("fixture population rollback", rollbackErr))
		}
	}()
	if _, err = tx.ExecContext(ctx, retentionFixturePopulationSQL); err != nil {
		return fmt.Errorf("populate fixture v1: %w", err)
	}
	var groups, core, lob, jsonRows int
	if err = tx.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM ora_stress_groups), (SELECT COUNT(*) FROM ora_stress_core), (SELECT COUNT(*) FROM ora_stress_lob), (SELECT COUNT(*) FROM ora_stress_json) FROM DUAL").Scan(&groups, &core, &lob, &jsonRows); err != nil {
		return fmt.Errorf("setup row-count validation: %w", err)
	}
	if groups != 8 || core != 512 || lob != 512 || jsonRows != 512 {
		return fmt.Errorf("fixture row-count mismatch: groups=%d core=%d lob=%d json=%d", groups, core, lob, jsonRows)
	}
	var minBlob, maxBlob, minClob, maxClob int
	if err = tx.QueryRowContext(ctx, "SELECT MIN(DBMS_LOB.GETLENGTH(blob_value)), MAX(DBMS_LOB.GETLENGTH(blob_value)), MIN(DBMS_LOB.GETLENGTH(clob_value)), MAX(DBMS_LOB.GETLENGTH(clob_value)) FROM ora_stress_lob").Scan(&minBlob, &maxBlob, &minClob, &maxClob); err != nil {
		return fmt.Errorf("setup LOB-length validation: %w", err)
	}
	if minBlob != retentionLOBBytes || maxBlob != retentionLOBBytes || minClob != retentionLOBBytes || maxClob != retentionLOBBytes {
		return errors.New("fixture LOB length mismatch; expected 1 MiB BLOB and ASCII CLOB per row")
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit fixture population: %w", err)
	}
	return nil
}

var retentionFixtureDDL = [...]struct{ name, sql string }{
	{"ORA_STRESS_GROUPS", `CREATE TABLE ora_stress_groups (
  group_id NUMBER(12,0) PRIMARY KEY,
  group_name VARCHAR2(32) NOT NULL
) ROWDEPENDENCIES`},
	{"ORA_STRESS_CORE", `CREATE TABLE ora_stress_core (
  id NUMBER(12,0) PRIMARY KEY,
  group_id NUMBER(12,0) NOT NULL REFERENCES ora_stress_groups(group_id),
  fixture_version NUMBER(12,0) NOT NULL,
  int_value NUMBER(12,0) NOT NULL,
  decimal_value NUMBER(18,6) NOT NULL,
  float_value BINARY_FLOAT NOT NULL,
  double_value BINARY_DOUBLE NOT NULL,
  text_value VARCHAR2(2000 BYTE) NOT NULL,
  nchar_value NCHAR(8) NOT NULL,
  unicode_value NVARCHAR2(128),
  raw_value RAW(32) NOT NULL,
  date_value DATE NOT NULL,
  timestamp_value TIMESTAMP(9) NOT NULL,
  tz_value TIMESTAMP(9) WITH TIME ZONE NOT NULL
) ROWDEPENDENCIES`},
	{"ORA_STRESS_LOB", `CREATE TABLE ora_stress_lob (
  id NUMBER(12,0) PRIMARY KEY REFERENCES ora_stress_core(id),
  blob_value BLOB NOT NULL,
  clob_value CLOB NOT NULL
) ROWDEPENDENCIES`},
	{"ORA_STRESS_JSON", `CREATE TABLE ora_stress_json (
  id NUMBER(12,0) PRIMARY KEY REFERENCES ora_stress_core(id),
  document JSON NOT NULL
) ROWDEPENDENCIES`},
}

// Populate only newly created objects; the caller owns the transaction.
const retentionFixturePopulationSQL = `DECLARE
  b BLOB;
  c CLOB;
  hex_byte VARCHAR2(2);
  raw_chunk RAW(8192);
  text_chunk VARCHAR2(8192);
  letter VARCHAR2(1);
  document_text VARCHAR2(32767);
BEGIN
  FOR g IN 1..8 LOOP
    INSERT INTO ora_stress_groups VALUES (g, 'group-' || TO_CHAR(g, 'FM9990'));
  END LOOP;
  FOR i IN 1..512 LOOP
    letter := CHR(ASCII('a') + MOD(i,26));
    hex_byte := LPAD(TO_CHAR(MOD(i,251),'FMXX'),2,'0');
    INSERT INTO ora_stress_core VALUES (
      i, MOD(i-1,8)+1, 1, i, i*1.001,
      CAST(i/4 AS BINARY_FLOAT), CAST(i/8 AS BINARY_DOUBLE),
      RPAD(letter,2000,letter), N'Ω' || TO_CHAR(i,'FM000000'),
      CASE WHEN MOD(i,16)=0 THEN NULL ELSE N'東京-' || TO_CHAR(i,'FM9990') END,
      HEXTORAW(RPAD(hex_byte,64,hex_byte)),
      DATE '2026-01-01' + i/86400,
      TIMESTAMP '2026-01-01 00:00:00.123456789' + NUMTODSINTERVAL(i,'SECOND'),
      TO_TIMESTAMP_TZ('2026-01-01 05:30:00.123456789 +05:30',
                     'YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM') + NUMTODSINTERVAL(i,'SECOND')
    );
    INSERT INTO ora_stress_lob VALUES (i, EMPTY_BLOB(), EMPTY_CLOB())
      RETURNING blob_value, clob_value INTO b,c;
    raw_chunk := HEXTORAW(RPAD(hex_byte,16384,hex_byte));
    text_chunk := RPAD(letter,8192,letter);
    FOR chunk IN 1..128 LOOP
      DBMS_LOB.WRITEAPPEND(b,8192,raw_chunk);
      DBMS_LOB.WRITEAPPEND(c,8192,text_chunk);
    END LOOP;
    document_text := '{"id":' || TO_CHAR(i,'FM9990') ||
      ',"nested":{"name":"row-' || TO_CHAR(i,'FM9990') ||
      '"},"items":[' || TO_CHAR(i,'FM9990') || ',' || TO_CHAR(i+1,'FM9990') ||
      ',' || TO_CHAR(i+2,'FM9990') || '],"active":true,"padding":"' ||
      RPAD(letter,16384,letter) || '"}';
    INSERT INTO ora_stress_json VALUES (i, JSON(document_text));
  END LOOP;
END;`

// Workload coordination, runtime/pool measurements, and diagnostic artifacts.

func newRetentionRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "ret-" + hex.EncodeToString(b[:])
}

type retentionProgress struct {
	sync.Mutex
	counts retentionCounts
}

func (p *retentionProgress) add(c retentionCounts)     { p.Lock(); p.counts.add(c); p.Unlock() }
func (p *retentionProgress) snapshot() retentionCounts { p.Lock(); defer p.Unlock(); return p.counts }

// Deliberately omit MemStats' pause histories and allocation-size arrays.
// Field names preserve existing PowerShell paths under runtime_memstats.
type retentionMemory struct {
	HeapAlloc, HeapObjects, HeapInuse, HeapSys, HeapReleased uint64
	TotalAlloc, Mallocs, Frees, PauseTotalNs                 uint64
	NumGC, NumForcedGC                                       uint32
	GCCPUFraction                                            float64
}

type retentionSample struct {
	RunID           string          `json:"run_id"`
	PID             int             `json:"pid"`
	Timestamp       time.Time       `json:"timestamp_utc"`
	ElapsedNS       int64           `json:"elapsed_ns"`
	Phase           string          `json:"phase"`
	Stage           string          `json:"stage,omitempty"`
	StageStatus     string          `json:"stage_status,omitempty"`
	Checkpoint      int             `json:"checkpoint"`
	Counts          retentionCounts `json:"completed"`
	Memory          retentionMemory `json:"runtime_memstats"`
	Goroutines      int             `json:"goroutines"`
	Pool            sql.DBStats     `json:"pool"`
	PoolAvailable   bool            `json:"pool_available"`
	AllocationDelta uint64          `json:"total_alloc_delta"`
	MallocDelta     uint64          `json:"mallocs_delta"`
	FreeDelta       uint64          `json:"frees_delta"`
	GCDelta         uint32          `json:"gc_delta"`
	PauseDelta      uint64          `json:"gc_pause_ns_delta"`
}

// Stream samples to one file. Keep only the previous state for deltas.
type retentionRecorder struct {
	sync.Mutex
	writer       io.Writer
	previous     retentionMemory
	runID        string
	started      time.Time
	stage        string
	stageStatus  string
	stageStarted time.Time
	testConfig   *TestConfig
}

// Progress records observations only: callers execute work and own deadlines.
// Beginning the next stage finishes the previous successful stage.
func (r *retentionRecorder) progress(name string, db *sql.DB, p *retentionProgress) error {
	r.Lock()
	defer r.Unlock()
	if err := r.finishLocked(nil, db, p); err != nil {
		return err
	}
	r.stage, r.stageStatus = name, "running"
	r.stageStarted = time.Now()
	return r.writeStageLocked("stage_start", name, r.stageStarted, nil)
}

func (r *retentionRecorder) finish(err error, db *sql.DB, p *retentionProgress) error {
	r.Lock()
	defer r.Unlock()
	return r.finishLocked(err, db, p)
}

func (r *retentionRecorder) finishLocked(err error, db *sql.DB, p *retentionProgress) error {
	if r.stageStatus != "running" {
		return err
	}
	r.stageStatus = "complete"
	if err != nil {
		r.stageStatus = "failed"
		err = fmt.Errorf("stage %s: %w", r.stage, err)
	}
	err = errors.Join(err, r.writeStageLocked("stage_end", r.stage, r.stageStarted, err))
	if err != nil {
		// Preserve failed-stage observations without duplicating every successful
		// transition. Periodic and drained samples provide normal runtime state.
		_, sampleErr := r.sampleLocked("natural", -1, db, p)
		err = errors.Join(err, sampleErr)
	}
	return err
}

func (r *retentionRecorder) writeStageLocked(phase, name string, began time.Time, err error) error {
	return json.NewEncoder(r.writer).Encode(map[string]any{
		"phase": phase, "stage": name, "status": r.stageStatus,
		"run_id": r.runID, "pid": os.Getpid(), "timestamp_utc": time.Now().UTC(),
		"elapsed_ns": time.Since(r.started).Nanoseconds(), "stage_duration_ns": time.Since(began).Nanoseconds(),
		"error": redactRetentionText(safeRetentionError(err), r.testConfig),
	})
}

func (r *retentionRecorder) event(v any) error {
	r.Lock()
	defer r.Unlock()
	return json.NewEncoder(r.writer).Encode(v)
}
func (r *retentionRecorder) sample(phase string, index int, db *sql.DB, p *retentionProgress) (resourceRetentionHeap, error) {
	r.Lock()
	defer r.Unlock()
	return r.sampleLocked(phase, index, db, p)
}
func (r *retentionRecorder) sampleLocked(phase string, index int, db *sql.DB, p *retentionProgress) (resourceRetentionHeap, error) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	mem := retentionMemory{
		HeapAlloc: stats.HeapAlloc, HeapObjects: stats.HeapObjects, HeapInuse: stats.HeapInuse,
		HeapSys: stats.HeapSys, HeapReleased: stats.HeapReleased, TotalAlloc: stats.TotalAlloc,
		Mallocs: stats.Mallocs, Frees: stats.Frees, NumGC: stats.NumGC, NumForcedGC: stats.NumForcedGC,
		PauseTotalNs: stats.PauseTotalNs, GCCPUFraction: stats.GCCPUFraction,
	}
	s := retentionSample{RunID: r.runID, PID: os.Getpid(), Timestamp: time.Now().UTC(), ElapsedNS: time.Since(r.started).Nanoseconds(), Phase: phase, Stage: r.stage, StageStatus: r.stageStatus, Checkpoint: index, Counts: p.snapshot(), Memory: mem, Goroutines: runtime.NumGoroutine(), PoolAvailable: db != nil}
	if db != nil {
		s.Pool = db.Stats()
	}
	if r.previous.TotalAlloc != 0 {
		s.AllocationDelta = mem.TotalAlloc - r.previous.TotalAlloc
		s.MallocDelta = mem.Mallocs - r.previous.Mallocs
		s.FreeDelta = mem.Frees - r.previous.Frees
		s.GCDelta = mem.NumGC - r.previous.NumGC
		s.PauseDelta = mem.PauseTotalNs - r.previous.PauseTotalNs
	}
	r.previous = mem
	err := json.NewEncoder(r.writer).Encode(s)
	return resourceRetentionHeap{HeapAlloc: mem.HeapAlloc, HeapObjects: mem.HeapObjects}, err
}

func (r *retentionRecorder) checkpoint(index int, db *sql.DB, p *retentionProgress, profiles bool, dir string) (resourceRetentionHeap, error) {
	r.Lock()
	defer r.Unlock() // No periodic report allocations between GC and snapshot.
	if db.Stats().InUse != 0 {
		return resourceRetentionHeap{}, errors.New("drained checkpoint has pooled connections in use")
	}
	if _, err := r.sampleLocked("drained_natural", index, db, p); err != nil {
		return resourceRetentionHeap{}, err
	}
	runtime.GC()
	heap, err := r.sampleLocked("post_gc", index, db, p)
	if err == nil && profiles {
		err = writeRetentionProfile(filepath.Join(dir, fmt.Sprintf("heap-%03d.pprof", index)), "heap", 0)
	}
	return heap, err
}

func writeRetentionProfile(path, name string, level int) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return pprof.Lookup(name).WriteTo(f, level)
}

// An epoch ends by closing admission, not canceling in-flight operations.
// Workers are joined before GC. Operation indices survive epoch boundaries.
func runRetentionEpoch(ctx context.Context, config oracleTest.ResourceRetentionConfig, indices []int, span time.Duration, p *retentionProgress, operation func(context.Context, int, int) (retentionCounts, error)) (elapsed, drain time.Duration, err error) {
	workCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(config.Workers)
	done.Add(config.Workers)
	errorsByWorker := make([]error, config.Workers)
	operationTimeout, _ := time.ParseDuration(config.Normalized().OperationTimeout)
	var admissionDeadline time.Time // Published by the common start barrier.
	for worker := 0; worker < config.Workers; worker++ {
		go func(worker int) {
			defer done.Done()
			defer func() {
				if v := recover(); v != nil {
					errorsByWorker[worker] = fmt.Errorf("worker %d panicked: %v", worker+1, v)
					cancel(errorsByWorker[worker])
				}
			}()
			ready.Done()
			<-start
			for workCtx.Err() == nil {
				if !admissionDeadline.IsZero() && !time.Now().Before(admissionDeadline) {
					return
				}
				if config.TotalOperations > 0 && indices[worker] >= resourceRetentionWorkerOperations(config.TotalOperations, config.Workers, worker) {
					return
				}
				counts, e := func() (retentionCounts, error) {
					opCtx, opCancel := context.WithTimeout(workCtx, operationTimeout)
					defer opCancel()
					c, e := operation(opCtx, worker, indices[worker])
					if e == nil {
						e = opCtx.Err()
					}
					return c, e
				}()
				if e != nil {
					errorsByWorker[worker] = fmt.Errorf("worker %d operation %d: %w", worker+1, indices[worker]+1, e)
					cancel(errorsByWorker[worker])
					return
				}
				indices[worker]++
				if counts.Cycles != 1 {
					errorsByWorker[worker] = fmt.Errorf("worker %d: missing completion count", worker+1)
					cancel(errorsByWorker[worker])
					return
				}
				p.add(counts)
			}
		}(worker)
	}
	ready.Wait()
	began := time.Now()
	if span > 0 {
		admissionDeadline = began.Add(span)
	}
	close(start)
	done.Wait()
	elapsed = time.Since(began)
	if !admissionDeadline.IsZero() {
		drain = max(time.Duration(0), time.Since(admissionDeadline))
	}
	primary := context.Cause(workCtx)
	err = primary
	for worker, e := range errorsByWorker {
		if e == nil || e == primary {
			continue
		}
		if inner := errors.Unwrap(e); inner != nil {
			if independent := retentionIndependentError(inner, primary); independent != nil {
				err = errors.Join(err, fmt.Errorf("worker %d operation %d: %w", worker+1, indices[worker]+1, independent))
			}
		} else {
			// Panic/accounting errors already carry their worker context.
			err = errors.Join(err, e)
		}
	}
	return
}

// Report only allowlisted information embedded in the running executable. The
// checkout's HEAD is not evidence of which source built a copied test binary.
func retentionBuildMetadata(info *debug.BuildInfo) map[string]any {
	metadata := map[string]any{
		"source": "runtime/debug.ReadBuildInfo", "available": info != nil,
		"vcs_revision": "unavailable", "vcs_modified": nil,
	}
	if info != nil {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if setting.Value != "" {
					metadata["vcs_revision"] = setting.Value
				}
			case "vcs.modified":
				if setting.Value == "true" || setting.Value == "false" {
					metadata["vcs_modified"] = setting.Value == "true"
				}
			}
		}
	}
	return metadata
}

func executeExpandedResourceRetention(config oracleTest.ResourceRetentionConfig, runID string, testConfig *TestConfig, openDB func(context.Context) (*sql.DB, error)) (result resourceRetentionResult, err error) {
	if err = config.Validate(); err != nil {
		return
	}
	config = config.Normalized()
	result.MemoryLimitBytes = debug.SetMemoryLimit(-1)
	var previousLimit int64
	limitApplied := false
	defer func() {
		if limitApplied {
			debug.SetMemoryLimit(previousLimit)
		}
	}()
	began := time.Now()
	defer func() { result.Wall = time.Since(began) }()
	dir := filepath.Join(config.OutputDirectory, runID)
	if err = os.MkdirAll(config.OutputDirectory, 0700); err != nil {
		return
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return
	}
	result.Artifact = filepath.Join(dir, "samples.jsonl")
	f, e := os.OpenFile(result.Artifact, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return result, e
	}
	recorder := &retentionRecorder{writer: f, runID: runID, started: began, testConfig: testConfig}
	defer func() { err = errors.Join(err, f.Sync(), f.Close()) }()
	if config.MemoryLimitMiB != nil {
		result.MemoryLimitBytes = *config.MemoryLimitMiB * (1 << 20)
		previousLimit = debug.SetMemoryLimit(result.MemoryLimitBytes)
		limitApplied = true
	}
	metadata := map[string]any{
		"artifact_format_version": 2, "phase": "configuration", "run_id": runID, "pid": os.Getpid(), "run_start_utc": began.UTC(),
		"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpu": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0),
		"gogc": os.Getenv("GOGC"), "gomemlimit": os.Getenv("GOMEMLIMIT"), "memory_target_bytes": result.MemoryLimitBytes,
		"profile": config, "fixture_version": retentionFixtureVersion, "lob_bytes_per_column": retentionLOBBytes, "json_padding_bytes": retentionJSONPadding,
		"oracle_monitoring": "external_optional", "os_monitoring": "external_windows_optional", "tls_version_cipher": "externally_verified_or_unavailable", "commit": "unavailable",
	}
	build, _ := debug.ReadBuildInfo()
	buildMetadata := retentionBuildMetadata(build)
	metadata["build"], metadata["commit"] = buildMetadata, buildMetadata["vcs_revision"]
	executable, executableErr := os.Executable()
	if executableErr != nil {
		executable = "unavailable"
	}
	metadata["executable_path"] = executable
	if testConfig != nil {
		metadata["config_name"] = testConfig.ConfigName
		metadata["database_version"] = testConfig.DatabaseVersion
		// Inspect the test connector's assigned configuration (including env and
		// flags), not just the constructor defaults. No database call is made.
		c, e := openTestConnectorWithConfig(testConfig)
		if e != nil {
			return result, e
		}
		if oracleConnector, ok := c.(*connector); ok {
			metadata["lob_prefetch_bytes"] = oracleConnector.connectorConfig.DriverProperties.DefaultLobPrefetchSize
			metadata["strict_null_handling"] = oracleConnector.connectorConfig.DriverProperties.StrictNullValueHandling
			if config.Workload != "select_dual" && !oracleConnector.connectorConfig.DriverProperties.StrictNullValueHandling {
				return result, errors.New("expanded fixtures require StrictNullValueHandling=true")
			}
		}
	}
	metadata["lob_prefetch_default_bytes"] = NewOracleDriverConfig().DriverProperties.DefaultLobPrefetchSize
	if err = recorder.event(metadata); err != nil {
		return
	}
	timeout, _ := time.ParseDuration(config.Timeout)
	operationTimeout, _ := time.ParseDuration(config.OperationTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	p := &retentionProgress{}
	var db *sql.DB
	var stopSampling context.CancelFunc
	var sampled chan struct{}
	var sampleErr error
	// One cleanup path: join the sampler, close the pool, then record the result.
	// A panic still propagates after best-effort cleanup and failed-stage evidence.
	defer func() {
		panicValue := recover()
		if panicValue != nil {
			err = errors.Join(err, fmt.Errorf("runner panic: %v", panicValue))
		}
		if stopSampling != nil {
			stopSampling()
			<-sampled
			err = errors.Join(err, sampleErr)
		}
		err = recorder.finish(err, db, p)
		poolCloseAttempted := db != nil
		var poolCloseErr error
		if db != nil {
			err = errors.Join(err, recorder.progress("database_cleanup", db, p))
			poolCloseErr = db.Close()
			closeReportErr := recorder.finish(poolCloseErr, db, p)
			err = errors.Join(err, closeReportErr)
			if closeReportErr == nil {
				// Stats remain available after Close. A nil handle would fabricate
				// zero-valued pool fields instead of observing the closed pool.
				_, sampleErr := recorder.sample("natural", -1, db, p)
				err = errors.Join(err, sampleErr)
			}
		}
		status := "complete"
		if err != nil {
			status = "failed"
		}
		err = errors.Join(err, recorder.event(map[string]any{
			"phase": "result", "run_id": runID, "timestamp_utc": time.Now().UTC(), "status": status,
			"counts": result.Counts, "workload_elapsed_ns": result.Duration.Nanoseconds(),
			// Legacy active_ns also includes draining; retain its value for readers.
			"active_ns": result.Duration.Nanoseconds(), "drain_ns": result.Drain.Nanoseconds(),
			"checkpoint_pause_ns": result.Pauses.Nanoseconds(), "wall_ns": time.Since(began).Nanoseconds(),
			"baseline": result.Baseline, "final": result.Final,
			"baseline_checkpoint_complete": result.BaselineCheckpointComplete, "final_checkpoint_complete": result.FinalCheckpointComplete,
			"pool_closed": poolCloseAttempted && poolCloseErr == nil, "pool_close_attempted": poolCloseAttempted,
			"pool_close_error": redactRetentionText(safeRetentionError(poolCloseErr), testConfig),
			"error":            redactRetentionText(safeRetentionError(err), testConfig), "diagnostic_only": true,
		}))
		if panicValue != nil {
			panic(panicValue)
		}
	}()
	if err = recorder.progress("database_initialization", nil, p); err != nil {
		return
	}
	opCtx, done := retentionOperationContext(ctx, operationTimeout)
	db, err = openDB(opCtx)
	err = errors.Join(err, opCtx.Err())
	done()
	if err != nil {
		return
	}
	if db == nil {
		return result, errors.New("database initialization returned no handle")
	}
	db.SetMaxOpenConns(config.MaxOpenConnections)
	db.SetMaxIdleConns(config.MaxOpenConnections)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	// Start before the first query. Keep the sampler alive at every checkpoint,
	// and join it before closing the pool or artifact file.
	var samplingCtx context.Context
	samplingCtx, stopSampling = context.WithCancel(ctx)
	defer stopSampling()
	sampled = make(chan struct{})
	sampleInterval, _ := time.ParseDuration(config.SampleInterval)
	go func() {
		defer close(sampled)
		ticker := time.NewTicker(sampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-samplingCtx.Done():
				return
			case <-ticker.C:
				if _, e := recorder.sample("natural", -1, db, p); e != nil {
					sampleErr = e
					cancel()
					return
				}
			}
		}
	}()

	if testConfig != nil {
		if err = recorder.progress("database_identity", db, p); err != nil {
			return
		}
		var version, name, pdb, service string
		opCtx, done = retentionOperationContext(ctx, operationTimeout)
		err = errors.Join(db.QueryRowContext(opCtx, "SELECT version_full FROM product_component_version WHERE product LIKE 'Oracle Database%' OR product LIKE 'Oracle AI Database%'").Scan(&version), opCtx.Err())
		done()
		if err != nil {
			return result, fmt.Errorf("database version preflight: %w", err)
		}
		opCtx, done = retentionOperationContext(ctx, operationTimeout)
		err = errors.Join(db.QueryRowContext(opCtx, "SELECT SYS_CONTEXT('USERENV','DB_NAME'),SYS_CONTEXT('USERENV','CON_NAME'),SYS_CONTEXT('USERENV','SERVICE_NAME') FROM DUAL").Scan(&name, &pdb, &service), opCtx.Err())
		done()
		if err != nil {
			return result, fmt.Errorf("database identity preflight: %w", err)
		}
		if err = recorder.event(map[string]any{"phase": "database_identity", "run_id": runID, "database_version": version, "database": name, "pdb": pdb, "service": service}); err != nil {
			return
		}
	}
	if err = recorder.progress("pool_priming", db, p); err != nil {
		return
	}
	if err = primeResourceRetentionPool(ctx, db, config.MaxOpenConnections, operationTimeout); err != nil {
		return
	}
	if _, err = recorder.sample("natural", -1, db, p); err != nil {
		return
	}
	if err = recorder.progress("fixture_checks", db, p); err != nil {
		return
	}
	if err = preflightRetentionFixtures(ctx, db, config); err != nil {
		return
	}
	fingerprint, e := retentionFixtureFingerprint(ctx, db, config, operationTimeout)
	if e != nil {
		return result, e
	}
	if err = recorder.progress("warm_up", db, p); err != nil {
		return
	}
	if err = warmResourceRetention(ctx, db, config); err != nil {
		return
	}
	if err = recorder.progress("baseline_checkpoint", db, p); err != nil {
		return
	}
	result.Baseline, err = recorder.checkpoint(0, db, p, config.DiagnosticProfiles, dir)
	result.BaselineCheckpointComplete = err == nil
	if err != nil {
		return
	}
	if err = recorder.event(map[string]any{"phase": "tls_verified", "run_id": runID, "protocol": "tcps", "fixture_fingerprint": fingerprint}); err != nil {
		return
	}

	indices := make([]int, config.Workers)
	checkpointEvery, _ := time.ParseDuration(config.CheckpointInterval)
	duration, _ := time.ParseDuration(config.Duration)
	checkpoint := 1
	for {
		if err = recorder.progress("measured_workload", db, p); err != nil {
			return
		}
		span := checkpointEvery
		if duration > 0 {
			span = min(span, duration-result.Duration)
		}
		elapsed, drain, epochErr := runRetentionEpoch(ctx, config, indices, span, p, func(ctx context.Context, worker, index int) (retentionCounts, error) {
			return executeRetentionCycle(ctx, db, config, worker, index)
		})
		result.Duration += elapsed
		result.Drain += drain
		result.Counts = p.snapshot()
		result.Completed = int(result.Counts.Cycles)
		if epochErr != nil {
			err = epochErr
			break
		}
		if duration > 0 && result.Duration >= duration || config.TotalOperations > 0 && result.Completed == config.TotalOperations {
			break
		}
		if err = recorder.progress("drained_checkpoint", db, p); err != nil {
			return
		}
		pauseStart := time.Now()
		_, err = recorder.checkpoint(checkpoint, db, p, config.DiagnosticProfiles, dir)
		result.Pauses += time.Since(pauseStart)
		checkpoint++
		if err != nil {
			break
		}
	}
	if err == nil && (result.Completed == 0 || config.TotalOperations > 0 && result.Completed != config.TotalOperations) {
		err = errors.New("incomplete measured workload")
	}
	err = recorder.finish(err, db, p)
	if reportErr := recorder.progress("final_fixture_verification", db, p); reportErr != nil {
		return result, errors.Join(err, reportErr)
	}
	finalFingerprint, verifyErr := retentionFixtureFingerprint(ctx, db, config, operationTimeout)
	if verifyErr == nil && finalFingerprint != fingerprint {
		verifyErr = errors.New("fixture changed during run")
	}
	err = errors.Join(err, recorder.finish(verifyErr, db, p))
	if reportErr := recorder.progress("final_checkpoint", db, p); reportErr != nil {
		return result, errors.Join(err, reportErr)
	}
	result.Final, e = recorder.checkpoint(checkpoint, db, p, config.DiagnosticProfiles, dir)
	result.FinalCheckpointComplete = e == nil
	e = errors.Join(e, ctx.Err())
	if config.DiagnosticProfiles {
		e = errors.Join(e, writeRetentionProfile(filepath.Join(dir, "goroutines.txt"), "goroutine", 2))
	}
	err = errors.Join(err, recorder.finish(e, db, p))
	return
}

func safeRetentionError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func redactRetentionText(value string, config *TestConfig) string {
	if config == nil {
		return value
	}
	for _, secret := range []string{config.GetConnectionString(), config.Credentials.Password, config.Security.WalletLocation} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return value
}

// ROWSCN/count is a change guard, not a cryptographic audit. DBA freeze and
// SELECT-only fixture privileges remain prerequisites for repeatability.
func retentionFixtureFingerprint(ctx context.Context, db *sql.DB, config oracleTest.ResourceRetentionConfig, operationTimeout ...time.Duration) (string, error) {
	if config.Workload == "select_dual" {
		return "smoke-no-fixtures", nil
	}
	tables := []string{"ORA_STRESS_CORE", "ORA_STRESS_GROUPS"}
	for _, kind := range retentionWorkloadKinds(config.Workload) {
		if kind == "lob_select" || kind == "connection_checkout" {
			if !containsRetentionTable(tables, "ORA_STRESS_LOB") {
				tables = append(tables, "ORA_STRESS_LOB")
			}
		}
		if kind == "json_select" {
			tables = append(tables, "ORA_STRESS_JSON")
		}
	}
	result := ""
	for _, table := range tables {
		var count, scn, minID, maxID, distinctID int64
		idColumn := "id"
		expected := int64(512)
		if table == "ORA_STRESS_GROUPS" {
			idColumn = "group_id"
			expected = 8
		}
		err := func() error {
			opCtx, cancel := retentionOperationContext(ctx, operationTimeout...)
			defer cancel()
			return errors.Join(db.QueryRowContext(opCtx, "SELECT COUNT(*), COALESCE(MAX(ORA_ROWSCN),0),MIN("+idColumn+"),MAX("+idColumn+"),COUNT(DISTINCT "+idColumn+") FROM "+retentionTable(config, table)).Scan(&count, &scn, &minID, &maxID, &distinctID), opCtx.Err())
		}()
		if err != nil {
			return "", err
		}
		if count != expected || distinctID != expected || minID != 1 || maxID != expected {
			return "", fmt.Errorf("%s fixture must contain exactly IDs 1..%d", table, expected)
		}
		result += fmt.Sprintf("%s:%d:%d;", table, count, scn)
	}
	return result, nil
}
func containsRetentionTable(tables []string, want string) bool {
	for _, v := range tables {
		if v == want {
			return true
		}
	}
	return false
}
