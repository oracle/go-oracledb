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

// Package main demonstrates opening an Oracle database connection with
// oracle.NewOracleConnector and sql.OpenDB.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle"
)

func main() {
	// Keep credentials separate from the connect descriptor when using a connector.
	connectDescriptor := os.Getenv("ORACLE_DSN")
	user := os.Getenv("ORACLE_USER")
	password := os.Getenv("ORACLE_PASSWORD")
	if connectDescriptor == "" || user == "" || password == "" {
		log.Fatal("set ORACLE_DSN, ORACLE_USER, and ORACLE_PASSWORD")
	}

	// Build an explicit driver configuration for NewOracleConnector.
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = connectDescriptor
	cfg.Credentials.User = user
	cfg.Credentials.Password = password
	cfg.ConnectionProperties.SendBufSize = 1024
	cfg.ConnectionProperties.SSLServerDNMatch = true
	cfg.ConnectDescriptor = "(DESCRIPTION_LIST=(LOAD_BALANCE=off)(FAILOVER=on)(DESCRIPTION=(CONNECT_TIMEOUT=5)(RETRY_COUNT=2)(ADDRESS_LIST=(LOAD_BALANCE=on)(ADDRESS=(PROTOCOL=tcps)(HOST=db-a.example.com)(PORT=2484))(ADDRESS=(PROTOCOL=tcps)(HOST=db-b.example.com)(PORT=2484)))(CONNECT_DATA=(SERVICE_NAME=orders.example.com))(SECURITY=(SSL_SERVER_DN_MATCH=yes)(SSL_SERVER_CERT_DN=\"CN=orders-primary, OU=DB\")))(DESCRIPTION=(CONNECT_TIMEOUT=30)(RETRY_COUNT=0)(ADDRESS=(PROTOCOL=tcps)(HOST=db-dr.example.com)(PORT=2484))(CONNECT_DATA=(SERVICE_NAME=orders.example.com))(SECURITY=(SSL_SERVER_DN_MATCH=yes)(SSL_SERVER_CERT_DN=\"CN=orders-dr, OU=DB\"))))"

	// sql.OpenDB accepts the connector returned by the driver API.
	connector, err := oracle.NewOracleConnector(cfg)
	if err != nil {
		log.Fatal(err)
	}

	db := sql.OpenDB(connector)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ping validates that authentication and session creation succeeded.
	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	// Run a minimal query using the established connection pool.
	var currentUser string
	if err := db.QueryRowContext(ctx, "select user from dual").Scan(&currentUser); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Connected to Oracle as %s\n", currentUser)
}
