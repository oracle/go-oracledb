/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, the license is provided on an
** "AS IS" basis, WITHOUT WARRANTY OF ANY KIND, either express or implied.
 */

package oracle

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/oracle/datatype"
)

func TestDriver_AssociativeArrayPLSQL(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	ctx := context.Background()
	db, err := openTestDBWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire test connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	packageName := strings.ToUpper(createObjectName("associative_array"))
	if _, err = conn.ExecContext(ctx, "CREATE OR REPLACE PACKAGE "+packageName+` AS
  TYPE NUMBER_BY_INDEX IS TABLE OF NUMBER INDEX BY PLS_INTEGER;
  TYPE VARCHAR_BY_INDEX IS TABLE OF VARCHAR2(50) INDEX BY PLS_INTEGER;
  PROCEDURE ECHO_NUMBERS(P_INPUT IN NUMBER_BY_INDEX, P_OUTPUT OUT NUMBER_BY_INDEX);
  PROCEDURE ECHO_VARCHARS(P_INPUT IN VARCHAR_BY_INDEX, P_OUTPUT OUT VARCHAR_BY_INDEX);
END;`); err != nil {
		t.Fatalf("create associative-array package: %v", err)
	}
	if _, err = conn.ExecContext(ctx, "CREATE OR REPLACE PACKAGE BODY "+packageName+` AS
  PROCEDURE ECHO_NUMBERS(P_INPUT IN NUMBER_BY_INDEX, P_OUTPUT OUT NUMBER_BY_INDEX) AS
  BEGIN
    P_OUTPUT := P_INPUT;
  END;
  PROCEDURE ECHO_VARCHARS(P_INPUT IN VARCHAR_BY_INDEX, P_OUTPUT OUT VARCHAR_BY_INDEX) AS
  BEGIN
    P_OUTPUT := P_INPUT;
  END;
END;`); err != nil {
		t.Fatalf("create associative-array package body: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, "DROP PACKAGE "+packageName) })

	tests := []struct {
		name      string
		typeName  string
		procedure string
		values    map[int32]any
		want      map[int32]any
	}{
		{name: "number", typeName: "NUMBER_BY_INDEX", procedure: "ECHO_NUMBERS", values: map[int32]any{-2: int64(10), 3: int64(20), 9: int64(30)}, want: map[int32]any{-2: int64(10), 3: int64(20), 9: int64(30)}},
		{name: "varchar2", typeName: "VARCHAR_BY_INDEX", procedure: "ECHO_VARCHARS", values: map[int32]any{-2: "one", 3: "two", 9: "three"}, want: map[int32]any{-2: "one", 3: "two", 9: "three"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typeName := packageName + "." + tc.typeName
			in, err := datatype.NewCollection(ctx, conn, typeName)
			if err != nil {
				t.Fatalf("NewCollection(%s): %v", typeName, err)
			}
			if !in.Associative || in.VArray || !in.ObjectType.Collection {
				t.Fatalf("associative-array descriptor: Collection=%v VArray=%v Associative=%v", in.ObjectType.Collection, in.VArray, in.Associative)
			}
			if err = in.SetIndexed(tc.values); err != nil {
				t.Fatalf("SetIndexed associative-array values: %v", err)
			}

			var out datatype.ObjectCollection
			if _, err = conn.ExecContext(ctx, "BEGIN "+packageName+"."+tc.procedure+"(:1, :2); END;", in, sql.Out{Dest: &out}); err != nil {
				t.Fatalf("PL/SQL associative-array IN/OUT: %v", err)
			}
			values, err := out.GetIndexed()
			if err != nil {
				t.Fatalf("GetIndexed associative-array values: %v", err)
			}
			if !reflect.DeepEqual(values, tc.want) {
				t.Fatalf("associative-array values = %#v, want %#v", values, tc.want)
			}
		})
	}
}
