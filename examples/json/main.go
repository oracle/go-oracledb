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

// Package main demonstrates binding, fetching, and decoding Oracle JSON values.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/oracle/go-driver/oracle"
	ojson "github.com/oracle/go-driver/oracle/json"
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

	// sql.Open uses the oracledb driver registered by the oracle package.
	db, err := sql.Open("oracledb", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Recreate the example table so the program can be run repeatedly.
	const table = "go_driver_json_example"
	_, _ = db.ExecContext(ctx, "drop table "+table+" purge")
	if _, err := db.ExecContext(ctx, "create table "+table+" (id number primary key, doc JSON)"); err != nil {
		log.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "drop table "+table+" purge")
	}()

	// JSONString binds JSON that is already represented as text.
	if _, err := db.ExecContext(ctx,
		"insert into "+table+" (id, doc) values (:1, :2)",
		1, ojson.JSONString{Data: `{"name":"Alice","score":9007199254740993}`},
	); err != nil {
		log.Fatal(err)
	}

	// JSONValue encodes a supported Go value and binds it as Oracle JSON.
	if _, err := db.ExecContext(ctx,
		"insert into "+table+" (id, doc) values (:1, :2)",
		2, ojson.JSONValue{Data: map[string]any{"name": "Bob", "active": true}},
	); err != nil {
		log.Fatal(err)
	}

	rows, err := db.QueryContext(ctx, "select doc from "+table+" order by id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		// Scan an Oracle JSON column into JSON.
		var doc ojson.JSON
		if err := rows.Scan(&doc); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("JSON text: %s\n", doc.String())

		// Decode JSON numbers as ojson.Number to preserve their precision.
		value, err := doc.GetValue(ojson.JSONOptNumberAsString)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Go value:  %#v\n", value)
	}
	// expected output:
	// 	JSON text: {"name":"Alice","score":9007199254740993}
	// 	Go value:  map[string]interface {}{"name":"Alice", "score":"9007199254740993"}
	// 	JSON text: {"active":true,"name":"Bob"}
	// 	Go value:  map[string]interface {}{"active":true, "name":"Bob"}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}
