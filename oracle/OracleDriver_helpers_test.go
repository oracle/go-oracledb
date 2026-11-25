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
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	testObjectNamePrefix = "godrivertest"
	testObjectRandomMin  = 100000
	testObjectRandomMax  = 999999
)

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
