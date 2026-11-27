# Oracle abstract data types (ADTs)

The `oracle/datatype` package supports Oracle named types through the standard
`database/sql` APIs. The supported ADTs are listed below.

| Oracle type | Constructor | Value API | Supported use |
| --- | --- | --- | --- |
| Object type | `datatype.NewObject` | Attribute values in `map[string]any` | SQL column binds and scans; PL/SQL IN, OUT, and IN OUT binds |
| VARRAY | `datatype.NewCollection` | `Set`, `Get`, `Len` | Scalar and object elements; SQL column binds and scans; PL/SQL IN, OUT, and IN OUT binds |
| Nested table | `datatype.NewCollection` | `Set`, `Get`, `Len` | Scalar values, including NULL elements; PL/SQL IN, OUT, and IN OUT binds |
| Associative array | `datatype.NewCollection` | `SetIndexed`, `GetIndexed`, `Indexes` | `PLS_INTEGER`-indexed PL/SQL package types and PL/SQL IN, OUT, and IN OUT binds |

The runnable counterpart is [examples/datatype/adt](../examples/datatype/adt).

## When metadata is resolved

Construct values by name without a connection. During Exec or Query the driver loads and caches the Oracle descriptor on the physical connection. For a named result column, the server description identifies the type even if no ADT was previously bound. Type mismatches and database lookup errors are reported by execution, not construction.

## VARRAY

A VARRAY is an ordered, bounded SQL collection. `Set` replaces its complete
contents; `Get` returns its complete contents in collection order.

```sql
CREATE TYPE app_number_varray AS VARRAY(5) OF NUMBER;

CREATE TABLE app_varray_rows (
    id NUMBER PRIMARY KEY,
    values_col app_number_varray
);
```

```go
numbers := datatype.NewCollection("APP_NUMBER_VARRAY")
if err := numbers.Set([]int64{10, 20, 30}); err != nil {
    return err
}

_, err := conn.ExecContext(ctx,
    "INSERT INTO app_varray_rows (id, values_col) VALUES (:1, :2)",
    int64(1), numbers,
)
if err != nil {
    return err
}

var selected datatype.ObjectCollection
if err := conn.QueryRowContext(ctx,
    "SELECT values_col FROM app_varray_rows WHERE id = :1", int64(1),
).Scan(&selected); err != nil {
    return err
}
values, err := selected.Get() // []any{int64(10), int64(20), int64(30)}
```

Once resolved, `numbers.UpperBound` exposes the VARRAY bound. A value that exceeds the bound is rejected when bound, or by `Set` if the bound is already known. Supported scalar element coverage includes
`NUMBER`, `VARCHAR2`, `RAW`, `DATE`, `TIMESTAMP`, `BINARY_FLOAT`, and
`BINARY_DOUBLE`. Object-valued VARRAY elements are also supported.

## Object type

Object values store attributes by their database names. Unquoted Oracle identifiers use uppercase keys. Missing attributes encode as NULL.

```sql
CREATE TYPE app_person AS OBJECT (
    id         NUMBER,
    name       VARCHAR2(50),
    created_at TIMESTAMP(9)
);
```

```go
person := datatype.NewObject("APP_PERSON", map[string]any{
    "ID": int64(42), "NAME": "Ada", "CREATED_AT": time.Now().UTC(),
})
person.Attributes["NAME"] = "Grace"

var out datatype.Object
_, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;",
    sql.Out{Dest: &out}, person,
)
if err != nil {
    return err
}
fmt.Println(out.Attributes["NAME"])
```

Object types can be inserted into and scanned from relational table columns.
After resolving a parent, you can construct nested values from the attribute metadata on
the parent object type:

```go
childType := person.ObjectType.Attributes["CHILD"].ObjectType
child, err := childType.NewObject()
```

For anonymous element shapes without SQL attribute names, use the positional keys supplied by the driver (such as `F_1`).

## Nested table

A nested table is an ordered SQL collection without a VARRAY upper bound. Use
the same `NewCollection`, `Set`, and `Get` APIs as a VARRAY.

```sql
CREATE TYPE app_number_table AS TABLE OF NUMBER;
```

```go
numbers := datatype.NewCollection("APP_NUMBER_TABLE")
if err := numbers.Set([]any{int64(10), nil, int64(30)}); err != nil {
    return err
}

var out datatype.ObjectCollection
_, err := conn.ExecContext(ctx, "BEGIN :1 := :2; END;",
    sql.Out{Dest: &out}, numbers,
)
if err != nil {
    return err
}
values, err := out.Get() // preserves the nil element
```

`ObjectType.Collection` is true for both VARRAYs and nested tables. Use
`ObjectType.VArray` to distinguish a VARRAY from a nested table.

## Associative array

Associative arrays are PL/SQL-only package types. The current support is for
`PLS_INTEGER` indexes. Use the indexed APIs so sparse and negative indexes are
preserved.

```sql
CREATE OR REPLACE PACKAGE app_adt_pkg AS
    TYPE number_by_index IS TABLE OF NUMBER INDEX BY PLS_INTEGER;
    PROCEDURE echo_numbers(
        p_input  IN  number_by_index,
        p_output OUT number_by_index
    );
END;
/

CREATE OR REPLACE PACKAGE BODY app_adt_pkg AS
    PROCEDURE echo_numbers(
        p_input  IN  number_by_index,
        p_output OUT number_by_index
    ) AS
    BEGIN
        p_output := p_input;
    END;
END;
/
```

```go
indexed := datatype.NewCollection("APP_ADT_PKG.NUMBER_BY_INDEX")
if err := indexed.SetIndexed(map[int32]any{-2: int64(10), 3: int64(20)}); err != nil {
    return err
}

var out datatype.ObjectCollection
_, err := conn.ExecContext(ctx, "BEGIN APP_ADT_PKG.ECHO_NUMBERS(:1, :2); END;",
    indexed, sql.Out{Dest: &out},
)
if err != nil {
    return err
}
values, err := out.GetIndexed() // map[int32]any{-2: int64(10), 3: int64(20)}
```

`ObjectType.Associative` identifies an associative collection. `Set` is also
valid for an associative array when consecutive one-based indexes are wanted;
`SetIndexed` is required to preserve caller-selected indexes.

## NULL and empty values

An empty collection and a typed Oracle NULL collection are distinct:

```go
empty := datatype.NewCollection("APP_NUMBER_VARRAY")
// empty has zero elements and is not NULL.

nullValue := datatype.NewCollection("APP_NUMBER_VARRAY")
nullValue.SetNull()
if nullValue.IsNull() {
    // Binds a typed Oracle NULL.
}
```

`Object.SetNull` similarly marks a typed NULL object. A normal SQL NULL result
is returned with `IsNull() == true`; do not call `Get` or `GetIndexed` on that
value.

## Type inspection

`ObjectType` exposes the loaded descriptor metadata. The following checks
distinguish the supported forms:

```go
typ := value.ObjectType
switch {
case typ.IsObject():
    // Object type.
case typ.Associative:
    // PLS_INTEGER associative array.
case typ.VArray:
    // VARRAY.
case typ.Collection:
    // Nested table.
}
```

Binding a value created with the wrong constructor returns an error during Exec or Query.

## Current scope

The supported behavior is exercised by the functional tests in
`oracle/OracleDriver_VARRY_test.go`, `oracle/OracleDriver_ObjectType_test.go`,
`oracle/OracleDriver_NestedTable_test.go`, and
`oracle/OracleDriver_AssociativeArray_test.go`.
