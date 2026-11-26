/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */
package compattests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	config "github.com/oracle/go-oracledb/v26/internal/tests"
	_ "github.com/oracle/go-oracledb/v26/oracle"
)

const (
	defaultConfigFile = "../test-config.json"
	defaultConfigName = "iam-tcps"
	queryRowTable     = "CODEX_COMPAT_MANY_QUERY_ROW"
	preparedTable     = "CODEX_COMPAT_PREPARED_STMT"
)

// TestManyQueryRow reproduces go-sql-test's repeated QueryRow workload:
// 10,000 QueryRow calls on one table and connection pool.
func TestManyQueryRow(t *testing.T) {
	db, cfg := openDatabase(t)
	resetTable(t, db, cfg, queryRowTable,
		"CREATE TABLE "+queryRowTable+" (id INTEGER PRIMARY KEY, name VARCHAR2(50))")
	t.Cleanup(func() { dropTable(t, db, cfg, queryRowTable) })

	if _, err := db.Exec("INSERT INTO "+queryRowTable+" (id, name) VALUES (:1, :2)", 1, "bob"); err != nil {
		t.Fatalf("insert fixture row: %s", safeError(err, cfg))
	}

	const iterations = 10000
	t.Logf("running %d QueryRow calls", iterations)
	for i := 0; i < iterations; i++ {
		var name string
		if err := db.QueryRow("SELECT name FROM "+queryRowTable+" WHERE id = :1", 1).Scan(&name); err != nil {
			t.Fatalf("QueryRow iteration %d: %s", i, safeError(err, cfg))
		}
		if name != "bob" {
			t.Fatalf("QueryRow iteration %d returned name %q, want %q", i, name, "bob")
		}
	}
}

// TestPreparedStmt reproduces go-sql-test's concurrent prepared SELECT and
// INSERT workload, including concurrent reuse of each database/sql Stmt.
func TestPreparedStmt(t *testing.T) {
	db, cfg := openDatabase(t)
	resetTable(t, db, cfg, preparedTable,
		"CREATE TABLE "+preparedTable+" (count NUMBER(10))")
	t.Cleanup(func() { dropTable(t, db, cfg, preparedTable) })

	if _, err := db.Exec("INSERT INTO "+preparedTable+" (count) VALUES (:1)", 1); err != nil {
		t.Fatalf("insert fixture row: %s", safeError(err, cfg))
	}
	if _, err := db.Exec("INSERT INTO "+preparedTable+" (count) VALUES (:1)", 2); err != nil {
		t.Fatalf("insert fixture row: %s", safeError(err, cfg))
	}
	if _, err := db.Exec("INSERT INTO "+preparedTable+" (count) VALUES (:1)", 3); err != nil {
		t.Fatalf("insert fixture row: %s", safeError(err, cfg))
	}

	sel, err := db.Prepare("SELECT count FROM " + preparedTable + " ORDER BY count DESC")
	if err != nil {
		t.Fatalf("prepare SELECT: %s", safeError(err, cfg))
	}
	defer sel.Close()

	ins, err := db.Prepare("INSERT INTO " + preparedTable + " (count) VALUES (:1)")
	if err != nil {
		t.Fatalf("prepare INSERT: %s", safeError(err, cfg))
	}
	defer ins.Close()

	const workers = 10
	const iterationsPerWorker = 10
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	t.Logf("running %d workers with %d prepared SELECT/INSERT pairs each", workers, iterationsPerWorker)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterationsPerWorker; i++ {
				var count int64
				if err := sel.QueryRow().Scan(&count); err != nil && !errors.Is(err, sql.ErrNoRows) {
					errCh <- fmt.Errorf("worker %d SELECT iteration %d: %w", worker, i, err)
					return
				}
				if _, err := ins.Exec(int64(i * 7)); err != nil {
					errCh <- fmt.Errorf("worker %d INSERT iteration %d: %w", worker, i, err)
					return
				}
			}
		}(worker)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(safeError(err, cfg))
	}
}

// TestCleanupArtifacts removes tables left behind if the Go test timeout
// terminated a reproduction before its cleanup callback could run.
func TestCleanupArtifacts(t *testing.T) {
	db, cfg := openDatabase(t)
	for _, table := range []string{queryRowTable, preparedTable} {
		dropTable(t, db, cfg, table)
	}
}

func openDatabase(t *testing.T) (*sql.DB, *config.TestConfig) {
	t.Helper()

	configFile := os.Getenv("GO_ORACLEDB_TEST_CONFIG")
	if configFile == "" {
		configFile = defaultConfigFile
	}
	if !filepath.IsAbs(configFile) {
		configFile = filepath.Clean(configFile)
	}
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		t.Skipf("Oracle compatibility config %q is absent; set GO_ORACLEDB_TEST_CONFIG to a JSON config file", configFile)
	} else if err != nil {
		t.Fatalf("stat Oracle compatibility config: %v", err)
	}

	configName := os.Getenv("GO_ORACLEDB_TEST_CONFIG_NAME")
	if configName == "" {
		configName = defaultConfigName
	}
	environment, err := config.NewTestingEnvironment(configFile)
	if err != nil {
		t.Fatalf("load Oracle compatibility config: %v", err)
	}
	cfg, err := environment.GetConfig(configName)
	if err != nil {
		t.Fatalf("load config profile %q: %v", configName, err)
	}

	// config-iam.sh exports the Node driver variable. The Go driver expects its
	// own name, so bridge the value in-process without printing it.
	if os.Getenv("ORACLE_GO_WALLET_PASSWORD") == "" {
		if walletPassword := os.Getenv("NODE_ORACLEDB_WALLET_PASSWORD"); walletPassword != "" {
			if err := os.Setenv("ORACLE_GO_WALLET_PASSWORD", walletPassword); err != nil {
				t.Fatalf("set Go wallet password environment variable: %v", err)
			}
		}
	}

	driverName := cfg.Driver.Name
	if driverName == "" {
		driverName = "oracledb"
	}
	db, err := sql.Open(driverName, cfg.GetConnectionString())
	if err != nil {
		t.Fatalf("open Oracle connection: %s", safeError(err, cfg))
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to Oracle with profile %q: %s", configName, safeError(err, cfg))
	}
	return db, cfg
}

func resetTable(t *testing.T, db *sql.DB, cfg *config.TestConfig, table, createSQL string) {
	t.Helper()
	dropTable(t, db, cfg, table)
	if _, err := db.Exec(createSQL); err != nil {
		t.Fatalf("create table %s: %s", table, safeError(err, cfg))
	}
}

func dropTable(t *testing.T, db *sql.DB, cfg *config.TestConfig, table string) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM user_tables WHERE table_name = :1", table).Scan(&count); err != nil {
		t.Fatalf("check for table %s: %s", table, safeError(err, cfg))
	}
	if count == 0 {
		return
	}
	if _, err := db.Exec("DROP TABLE " + table); err != nil {
		t.Fatalf("drop table %s: %s", table, safeError(err, cfg))
	}
}

func safeError(err error, cfg *config.TestConfig) string {
	message := err.Error()
	for _, secret := range []string{
		cfg.Credentials.Username,
		cfg.Credentials.Password,
		os.Getenv("ORACLE_GO_WALLET_PASSWORD"),
		os.Getenv("NODE_ORACLEDB_WALLET_PASSWORD"),
		cfg.GetConnectionString(),
	} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return message
}
