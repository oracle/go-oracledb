/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, the license is provided on an
** "AS IS" basis, WITHOUT WARRANTY OF ANY KIND, either express or implied.
 */

// Package main demonstrates the supported Oracle abstract data type APIs.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	_ "github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

const (
	numberVarrayType = "GO_DRIVER_ADT_NUMBERS"
	numberTableType  = "GO_DRIVER_ADT_NUMBER_TABLE"
	personType       = "GO_DRIVER_ADT_PERSON"
	packageName      = "GO_DRIVER_ADT_PKG"
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

	// An ADT descriptor is tied to its physical connection. Keep this connection
	// checked out for descriptor lookup, bind execution, and result decoding.
	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err := createTypes(ctx, conn); err != nil {
		log.Fatal(err)
	}
	defer dropTypes(context.Background(), conn)

	if err := demonstrateVARRAY(ctx, conn); err != nil {
		log.Fatal(err)
	}
	if err := demonstrateNestedTable(ctx, conn); err != nil {
		log.Fatal(err)
	}
	if err := demonstrateObject(ctx, conn); err != nil {
		log.Fatal(err)
	}
	if err := demonstrateAssociativeArray(ctx, conn); err != nil {
		log.Fatal(err)
	}
}

func createTypes(ctx context.Context, conn *sql.Conn) error {
	statements := []string{
		"CREATE TYPE " + numberVarrayType + " AS VARRAY(5) OF NUMBER",
		"CREATE TYPE " + numberTableType + " AS TABLE OF NUMBER",
		"CREATE TYPE " + personType + " AS OBJECT (ID NUMBER, NAME VARCHAR2(50), CREATED_AT TIMESTAMP(9))",
		"CREATE OR REPLACE PACKAGE " + packageName + ` AS
  TYPE NUMBER_BY_INDEX IS TABLE OF NUMBER INDEX BY PLS_INTEGER;
  PROCEDURE ECHO_NUMBERS(P_INPUT IN NUMBER_BY_INDEX, P_OUTPUT OUT NUMBER_BY_INDEX);
END;`,
		"CREATE OR REPLACE PACKAGE BODY " + packageName + ` AS
  PROCEDURE ECHO_NUMBERS(P_INPUT IN NUMBER_BY_INDEX, P_OUTPUT OUT NUMBER_BY_INDEX) AS
  BEGIN
    P_OUTPUT := P_INPUT;
  END;
END;`,
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func dropTypes(ctx context.Context, conn *sql.Conn) {
	for _, statement := range []string{
		"DROP PACKAGE " + packageName,
		"DROP TYPE " + personType + " FORCE",
		"DROP TYPE " + numberTableType + " FORCE",
		"DROP TYPE " + numberVarrayType + " FORCE",
	} {
		_, _ = conn.ExecContext(ctx, statement)
	}
}

func demonstrateVARRAY(ctx context.Context, conn *sql.Conn) error {
	values, err := datatype.NewCollection(ctx, conn, numberVarrayType)
	if err != nil {
		return err
	}
	if err := values.Set([]int64{10, 20, 30}); err != nil {
		return err
	}

	var out datatype.ObjectCollection
	if _, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, values); err != nil {
		return err
	}
	got, err := out.Get()
	if err != nil {
		return err
	}
	fmt.Printf("VARRAY: %v (bound %d)\n", got, out.UpperBound)
	return nil
}

func demonstrateNestedTable(ctx context.Context, conn *sql.Conn) error {
	values, err := datatype.NewCollection(ctx, conn, numberTableType)
	if err != nil {
		return err
	}
	if err := values.Set([]any{int64(10), nil, int64(30)}); err != nil {
		return err
	}

	var out datatype.ObjectCollection
	if _, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, values); err != nil {
		return err
	}
	got, err := out.Get()
	if err != nil {
		return err
	}
	fmt.Printf("Nested table: %v\n", got)
	return nil
}

func demonstrateObject(ctx context.Context, conn *sql.Conn) error {
	person, err := datatype.NewObject(ctx, conn, personType)
	if err != nil {
		return err
	}
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC)
	person.FieldByName("ID").Set(reflect.ValueOf(int64(42)))
	person.FieldByName("NAME").Set(reflect.ValueOf("Ada"))
	person.FieldByName("CREATED_AT").Set(reflect.ValueOf(createdAt))

	var out datatype.Object
	if _, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;", sql.Out{Dest: &out}, person); err != nil {
		return err
	}
	fmt.Printf("Object: ID=%v NAME=%q CREATED_AT=%v\n",
		out.FieldByName("ID").Interface(),
		out.FieldByName("NAME").Interface(),
		out.FieldByName("CREATED_AT").Interface(),
	)
	return nil
}

func demonstrateAssociativeArray(ctx context.Context, conn *sql.Conn) error {
	values, err := datatype.NewCollection(ctx, conn, packageName+".NUMBER_BY_INDEX")
	if err != nil {
		return err
	}
	if err := values.SetIndexed(map[int32]any{-2: int64(10), 3: int64(20), 9: int64(30)}); err != nil {
		return err
	}

	var out datatype.ObjectCollection
	if _, err := conn.ExecContext(ctx, "BEGIN "+packageName+".ECHO_NUMBERS(:1, :2); END;", values, sql.Out{Dest: &out}); err != nil {
		return err
	}
	got, err := out.GetIndexed()
	if err != nil {
		return err
	}
	fmt.Printf("Associative array: %v\n", got)
	return nil
}
