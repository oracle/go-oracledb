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

// Package main demonstrates Oracle REF CURSOR OUT binds and implicit result cursors.
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	_ "github.com/oracle/go-oracledb/v26/oracle"
)

func main() {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		log.Fatal("set ORACLE_DSN, for example: user/password@localhost:1521/freepdb1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("oracledb", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := fetchRefCursor(ctx, db); err != nil {
		log.Fatal(err)
	}
	if err := fetchImplicitResults(ctx, db); err != nil {
		log.Fatal(err)
	}
}

// fetchRefCursor receives a REF CURSOR through a PL/SQL OUT bind.
func fetchRefCursor(ctx context.Context, db *sql.DB) error {
	var rows driver.Rows
	_, err := db.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR
    SELECT 1 AS id, 'first row' AS label FROM dual
    UNION ALL
    SELECT 2 AS id, 'second row' AS label FROM dual;
END;`, sql.Out{Dest: &rows})
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Println("REF CURSOR OUT bind")
	columns := rows.Columns()
	fmt.Printf("Columns: %v\n", columns)
	values := make([]driver.Value, len(columns))
	for {
		err = rows.Next(values)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("Row: %v\n", values)
	}
}

// fetchImplicitResults reads cursors returned through DBMS_SQL.RETURN_RESULT.
func fetchImplicitResults(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
DECLARE
  c1 SYS_REFCURSOR;
  c2 SYS_REFCURSOR;
BEGIN
  OPEN c1 FOR SELECT 11 AS n FROM dual;
  DBMS_SQL.RETURN_RESULT(c1);
  OPEN c2 FOR SELECT 22 AS n FROM dual;
  DBMS_SQL.RETURN_RESULT(c2);
END;`)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Println("Implicit result cursors")
	for resultSet := 1; ; resultSet++ {
		columns, err := rows.Columns()
		if err != nil {
			return err
		}
		fmt.Printf("Result set %d, columns: %v\n", resultSet, columns)
		for rows.Next() {
			var value int64
			if err := rows.Scan(&value); err != nil {
				return err
			}
			fmt.Printf("Row: [%d]\n", value)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !rows.NextResultSet() {
			return rows.Err()
		}
	}
}
