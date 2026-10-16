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
	stdjson "encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/oracle/go-oracledb/v26/oracle"
	ojson "github.com/oracle/go-oracledb/v26/oracle/datatype/json"
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

	const table = "go_driver_json_example"
	if _, err := db.ExecContext(ctx, "create table "+table+" (id number primary key, doc JSON)"); err != nil {
		log.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "drop table "+table+" purge")
	}()

	// inserting a JSON text.
	jsonText := `{
		"order": {
			"customer": {
				"name": "Alice",
				"addresses": [
					{"city": "London", "country": "UK"},
					{"city": "Paris", "country": "France"}
				]
			},
			"id": 9007199254740993,
			"items": [
				{"sku": "A-100", "quantity": 2},
				{"sku": "B-200", "quantity": 1}
			]
		}
	}`
	textDoc := ojson.NewJSONFromString(jsonText)
	if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", 1, textDoc); err != nil {
		log.Fatal(err)
	}

	// inserting a JSON from Go values.
	mapDoc, err := ojson.NewJSON(map[string]any{
		"order": map[string]any{
			"customer": map[string]any{
				"name": "Bob",
				"addresses": []any{
					map[string]any{"city": "New York", "country": "USA"},
					map[string]any{"city": "Toronto", "country": "Canada"},
				},
			},
			"id": int64(9007199254740994),
			"items": []any{
				map[string]any{"sku": "C-300", "quantity": 3},
				map[string]any{"sku": "D-400", "quantity": 4},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "insert into "+table+" (id, doc) values (:1, :2)", 2, mapDoc); err != nil {
		log.Fatal(err)
	}

	// Fetch all JSON documents.
	rows, err := db.QueryContext(ctx, "select doc from "+table+" order by id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var doc ojson.JSON
		if err := rows.Scan(&doc); err != nil {
			log.Fatal(err)
		}

		// Marshal the document to JSON text.
		text, err := stdjson.Marshal(doc)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("JSON text: %s\n", text)
		// Expected output:
		// First row: JSON text: {"order":{"customer":{"addresses":[{"city":"London","country":"UK"},{"city":"Paris","country":"France"}],"name":"Alice"},"id":9007199254740993,"items":[{"quantity":2,"sku":"A-100"},{"quantity":1,"sku":"B-200"}]}}
		// Second row: JSON text: {"order":{"customer":{"addresses":[{"city":"New York","country":"USA"},{"city":"Toronto","country":"Canada"}],"name":"Bob"},"id":9007199254740994,"items":[{"quantity":3,"sku":"C-300"},{"quantity":4,"sku":"D-400"}]}}

		// The default mode already chooses an appropriate Go number type. Set this
		// option only when every JSON number should be json.Number. Use
		// NumberAsFloat64 instead when every JSON number should be float64.
		doc.SetOptions(ojson.Options{NumberMode: ojson.NumberAsJSONNumber})

		// Use the access API to view the document root as an object.
		object, err := doc.AsJSONObject()
		if err != nil {
			log.Fatal(err)
		}

		// Access the "order" member without decoding the whole document.
		orderJSON, ok := object.Get("order")
		if !ok {
			log.Fatal("order is missing")
		}

		// Access nested object members from the order.
		order, err := orderJSON.AsJSONObject()
		if err != nil {
			log.Fatal(err)
		}

		// Access the order ID as a scalar. NumberAsJSONNumber returns json.Number.
		idJSON, ok := order.Get("id")
		if !ok {
			log.Fatal("order ID is missing")
		}
		id, err := idJSON.AsJSONScalar()
		if err != nil {
			log.Fatal(err)
		}
		idValue, err := id.GetValue()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Order ID (%T): %v\n", idValue, idValue)
		// Expected output:
		// First row: Order ID (json.Number): 9007199254740993
		// Second row: Order ID (json.Number): 9007199254740994

		// Access the items array from the order object.
		itemsJSON, ok := order.Get("items")
		if !ok {
			log.Fatal("items are missing")
		}
		items, err := itemsJSON.AsJSONArray()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Item count: %d\n", items.Len())
		// Expected output:
		// First row: Item count: 2
		// Second row: Item count: 2

		// Access the first item and then its SKU scalar.
		firstItemJSON, err := items.Get(0)
		if err != nil {
			log.Fatal(err)
		}
		firstItem, err := firstItemJSON.AsJSONObject()
		if err != nil {
			log.Fatal(err)
		}
		skuJSON, ok := firstItem.Get("sku")
		if !ok {
			log.Fatal("item SKU is missing")
		}
		sku, err := skuJSON.AsJSONScalar()
		if err != nil {
			log.Fatal(err)
		}
		skuValue, err := sku.GetValue()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("First item SKU: %v\n", skuValue)
		// Expected output:
		// First row: First item SKU: A-100
		// Second row: First item SKU: C-300
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}
