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
** is contributed to or provided by such licensors),
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

// Package main demonstrates binding and fetching a dense Oracle VECTOR value.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

func main() {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		log.Fatal("set ORACLE_DSN, for example: user/password@localhost:1521/freepdb1")
	}

	ctx := context.Background()
	db, err := sql.Open("oracledb", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	const table = "go_driver_vector_example"
	if _, err = db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER PRIMARY KEY, value VECTOR(3, FLOAT32))"); err != nil {
		log.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE "+table+" PURGE")
	}()

	value := datatype.VectorFloat32{1.5, -2.25, 3.75}
	if _, err = db.ExecContext(ctx, "INSERT INTO "+table+" (id, value) VALUES (:1, :2)", 1, value); err != nil {
		log.Fatal(err)
	}

	var fetched []float32
	if err = db.QueryRowContext(ctx, "SELECT value FROM "+table+" WHERE id = :1", 1).Scan(&fetched); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("VECTOR: %v\n", fetched)
}
