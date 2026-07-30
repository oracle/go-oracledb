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

// Package main demonstrates opening an Oracle database connection with database/sql
// using the globally registered oracledb driver.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oracle/go-driver/oracle"
)

func main() {
	// The DSN includes credentials and the connect descriptor.
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		log.Fatal("set ORACLE_DSN, for example: user/password@localhost:1521/freepdb1")
	}

	// Bound all database operations in this example to a single timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure driver logging before opening the first connection.
	loggingConfig := oracle.NewOracleLoggingConfig()

	// Defines the logging level from the environment.
	// For example: ORACLE_LOG_LEVEL=DEBUG.
	// See README.md for more information.
	if level := os.Getenv("ORACLE_LOG_LEVEL"); level != "" {
		loggingConfig.Level = level
	}

	// Defines the logging destination from the environment.
	// By default, logging activity is discarded.
	// For example, to send log messages to the console, use ORACLE_LOG_DESTINATION=STDOUT.
	// See README.md for more information.
	if destination := os.Getenv("ORACLE_LOG_DESTINATION"); destination != "" {
		loggingConfig.Destination = destination
	}
	oracle.GetDefaultDriver().ApplyDriverLoggingConfig(loggingConfig)

	// sql.Open uses the oracledb driver registered by the oracle package.
	db, err := sql.Open("oracledb", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	// Run a simple query to verify the connection can execute SQL.
	var currentTime string
	if err := db.QueryRowContext(ctx, "select to_char(systimestamp, 'YYYY-MM-DD HH24:MI:SS TZH:TZM') from dual").Scan(&currentTime); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Connected to Oracle. Database time: %s\n", currentTime)
}
