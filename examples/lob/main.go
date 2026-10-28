/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively, the "Software"), free of charge and under any and all
** copyright and patent rights owned or licensable by each licensor hereunder
** covering the Software, to deal in the Software without restriction, including
** without limitation the rights to use, copy, modify, merge, publish,
** distribute, sublicense, and/or sell copies of the Software, and to permit
** persons to whom the Software is furnished to do so, subject to the following
** conditions:
**
** The above copyright notice and this permission notice shall be included in all
** copies or substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

// Package main demonstrates the public materialized and streaming LOB APIs.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

const (
	// These are numeric prefetch counts. Oracle interprets them as bytes for
	// BLOBs and characters for CLOBs and NCLOBs. The first two scenarios use
	// counts below and above the default count of 33,554,432.
	boundaryPrefetchCount32K = 32 * 1024
	boundaryPrefetchCount40M = 40 * 1024 * 1024

	// The multi-column test uses a count smaller than the default while still
	// exercising prefetch.
	multiplePrefetchDefaultCount8M = 8 * 1024 * 1024

	boundaryPayloadExtraCount64K = 64 * 1024

	// Each test gets a fresh deadline. A single deadline around the whole program
	// would make a slow first test consume the time available to later tests.
	lobTestTimeout30Min = 30 * time.Minute
)

func main() {
	// Read the connection and test category, then let one runner dispatch the
	// selected category or all categories.
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		log.Fatal("set ORACLE_DSN, for example: user/password@host:port/service")
	}

	runLOBTests(dsn, parseLOBTestCategory(os.Getenv("ORACLE_LOB_TEST")))
}

// -----------------------------------------------------------------------------
// LOB example test selection and dispatch
// -----------------------------------------------------------------------------

type lobTestCategory int

const (
	// lobTestAll is the zero value so an unset category runs every test.
	lobTestAll lobTestCategory = iota
	lobTestBoundary
	lobTestMultiple
	lobTestTyped
)

// parseLOBTestCategory parses ORACLE_LOB_TEST. An empty value selects all
// tests; the other values select one of the three test categories.
func parseLOBTestCategory(value string) lobTestCategory {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return lobTestAll
	case "boundary":
		return lobTestBoundary
	case "multiple":
		return lobTestMultiple
	case "typed":
		return lobTestTyped
	default:
		log.Fatalf("invalid ORACLE_LOB_TEST %q; use all, boundary, multiple, or typed", value)
		return lobTestAll
	}
}

// runLOBTests runs all tests by default or one test category: boundary reads,
// multiple-column reads, or typed LOB operations.
func runLOBTests(dsn string, category lobTestCategory) {
	switch category {
	case lobTestAll:
		runLOBPrefetchBoundaryTests(dsn)
		runMultipleLOBTests(dsn)
		runTypedLOBTests(dsn)
	case lobTestBoundary:
		runLOBPrefetchBoundaryTests(dsn)
	case lobTestMultiple:
		runMultipleLOBTests(dsn)
	case lobTestTyped:
		runTypedLOBTests(dsn)
	}
}

// runLOBPrefetchBoundaryTests runs two BLOB/CLOB read tests with numeric
// prefetch counts below and above the default. Each payload exceeds its count
// in the units used by its LOB type; each test checks materialized and streaming
// reads.
func runLOBPrefetchBoundaryTests(dsn string) {
	for _, scenario := range []struct {
		name     string
		prefetch int
	}{
		{name: "prefetch count below default", prefetch: boundaryPrefetchCount32K},
		{name: "prefetch count above default", prefetch: boundaryPrefetchCount40M},
	} {
		prefetchSize := scenario.prefetch
		runConfiguredLOBTest(dsn, scenario.name, prefetchSize, func(ctx context.Context, db *sql.DB) error {
			return runLOBPrefetchBoundaryTest(ctx, db, prefetchSize)
		})
	}
}

// runMultipleLOBTests runs the six-column test with a lower numeric prefetch
// count and with the configurable count (8,388,608 by default).
func runMultipleLOBTests(dsn string) {
	multiplePrefetchValue := strings.TrimSpace(os.Getenv("ORACLE_LOB_MULTIPLE_PREFETCH_SIZE"))
	multipleDefaultPrefetch := parseMultiplePrefetchCount(multiplePrefetchValue)
	multipleScenarioName := fmt.Sprintf("multiple LOB columns at configured prefetch count %s", formatPrefetchCount(multipleDefaultPrefetch))
	if multiplePrefetchValue == "" {
		multipleScenarioName = fmt.Sprintf("multiple LOB columns, default prefetch count %s", formatPrefetchCount(multipleDefaultPrefetch))
	}

	for _, scenario := range []struct {
		name     string
		prefetch int
	}{
		{name: "multiple LOB columns with lower prefetch count", prefetch: boundaryPrefetchCount32K},
		{name: multipleScenarioName, prefetch: multipleDefaultPrefetch},
	} {
		prefetchSize := scenario.prefetch
		runConfiguredLOBTest(dsn, scenario.name, prefetchSize, func(ctx context.Context, db *sql.DB) error {
			return runMultipleLOBTest(ctx, db, prefetchSize)
		})
	}
}

// runTypedLOBTests covers temporary LOB operations and persistent typed scans.
func runTypedLOBTests(dsn string) {
	runConfiguredLOBTest(dsn, "typed LOB operations", oracleconfig.DefaultLobPrefetchSize, func(ctx context.Context, db *sql.DB) error {
		return runTypedLOBTest(ctx, db)
	})
}

// runConfiguredLOBTest runs one LOB test with the given prefetch setting.
// It manages the database pool, timeout, error handling, and cleanup shared by
// every test category.
func runConfiguredLOBTest(dsn, name string, prefetchSize int, run func(context.Context, *sql.DB) error) {
	fmt.Printf("\n--- %s (LOB prefetch count: %s) ---\n", name, formatPrefetchCount(prefetchSize))
	ctx, cancel := context.WithTimeout(context.Background(), lobTestTimeout30Min)
	db := openLOBDatabase(ctx, dsn, prefetchSize)
	err := run(ctx, db)
	cancel()
	if err != nil {
		_ = db.Close()
		log.Fatal(err)
	}
	if err := db.Close(); err != nil {
		log.Fatal(err)
	}
}

// parseMultiplePrefetchCount parses the optional positive signed 32-bit
// numeric count for the second multi-column test, defaulting to 8,388,608.
func parseMultiplePrefetchCount(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return multiplePrefetchDefaultCount8M
	}

	prefetch, err := strconv.ParseInt(value, 10, 32)
	if err != nil || prefetch <= 0 {
		log.Fatalf("invalid ORACLE_LOB_MULTIPLE_PREFETCH_SIZE %q; use a positive signed 32-bit numeric count", value)
	}
	return int(prefetch)
}

// formatPrefetchCount describes the numeric count and its type-dependent units.
func formatPrefetchCount(count int) string {
	return fmt.Sprintf("%d (bytes for BLOB; characters for CLOB/NCLOB)", count)
}

// openLOBDatabase opens a one-connection database pool with the requested LOB
// prefetch size and verifies that the database is reachable.
func openLOBDatabase(ctx context.Context, dsn string, prefetchSize int) *sql.DB {
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = dsn
	cfg.DriverProperties.DefaultLobPrefetchSize = prefetchSize

	connector, err := oracle.NewOracleConnector(cfg)
	if err != nil {
		log.Fatalf("create Oracle connector: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		log.Fatalf("connect to Oracle: %v", err)
	}
	return db
}

// createTable creates a uniquely named table for a database-pool test.
func createTable(ctx context.Context, db *sql.DB, columns string) (string, error) {
	// The name is generated locally and never contains user input, so it can be
	// embedded in the DDL and DML used by this example.
	table := fmt.Sprintf("GOLOB_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s (%s)", table, columns)); err != nil {
		return "", fmt.Errorf("create table %s: %w", table, err)
	}
	return table, nil
}

// dropTable removes a table created by a database-pool test.
func dropTable(ctx context.Context, db *sql.DB, table string) error {
	if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s PURGE", table)); err != nil {
		return fmt.Errorf("drop table %s: %w", table, err)
	}
	return nil
}

// createTableConn creates a uniquely named table on a dedicated connection.
func createTableConn(ctx context.Context, conn *sql.Conn, columns string) (string, error) {
	table := fmt.Sprintf("GOLOB_%d", time.Now().UnixNano())
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s (%s)", table, columns)); err != nil {
		return "", fmt.Errorf("create table %s: %w", table, err)
	}
	return table, nil
}

// dropTableConn removes a table created on a dedicated connection.
func dropTableConn(ctx context.Context, conn *sql.Conn, table string) error {
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s PURGE", table)); err != nil {
		return fmt.Errorf("drop table %s: %w", table, err)
	}
	return nil
}

// cleanupTable drops a database-pool test table with a cleanup timeout.
func cleanupTable(db *sql.DB, table string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := dropTable(cleanupCtx, db, table); err != nil {
		fmt.Printf("cleanup warning: %v\n", err)
	}
}

// cleanupTableConn drops a dedicated-connection scenario table with a cleanup
// timeout.
func cleanupTableConn(conn *sql.Conn, table string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := dropTableConn(cleanupCtx, conn, table); err != nil {
		fmt.Printf("cleanup warning: %v\n", err)
	}
}
