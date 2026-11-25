# Oracle abstract data types (ADTs)

The `oracle/datatype` package supports Oracle named types through the standard
`database/sql` APIs. The supported ADTs are listed below.

| Oracle type | Constructor | Value API | Supported use |
| --- | --- | --- | --- |
| Object type | `datatype.NewObject` | Reflected fields via `FieldByName` or `Field` | SQL column binds and scans; PL/SQL IN, OUT, and IN OUT binds |
| VARRAY | `datatype.NewCollection` | `Set`, `Get`, `Len` | Scalar and object elements; SQL column binds and scans; PL/SQL IN, OUT, and IN OUT binds |
| Nested table | `datatype.NewCollection` | `Set`, `Get`, `Len` | Scalar values, including NULL elements; PL/SQL IN, OUT, and IN OUT binds |
| Associative array | `datatype.NewCollection` | `SetIndexed`, `GetIndexed`, `Indexes` | `PLS_INTEGER`-indexed PL/SQL package types and PL/SQL IN, OUT, and IN OUT binds |

The runnable counterpart is [examples/datatype/adt](../examples/datatype/adt).

## Connection requirement

Create the ADT value and use it on the same physical `*sql.Conn`. An ADT
descriptor contains database metadata, including its TOID and type version,
which is registered on the physical connection when it is bound or decoded.

```go
conn, err := db.Conn(ctx)
if err != nil {
    return err
}
defer conn.Close()

numbers, err := datatype.NewCollection(ctx, conn, "APP_NUMBER_VARRAY")
if err != nil {
    return err
}
```

Do not load a descriptor through `*sql.DB` and later bind it through another
operation on `*sql.DB`: the pool may select a different physical connection.

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
numbers, err := datatype.NewCollection(ctx, conn, "APP_NUMBER_VARRAY")
if err != nil {
    return err
}
if err := numbers.Set([]int64{10, 20, 30}); err != nil {
    return err
}

_, err = conn.ExecContext(ctx,
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

The VARRAY bound is exposed through `numbers.UpperBound`; `Set` rejects a
slice or array that exceeds it. Supported scalar element coverage includes
`NUMBER`, `VARCHAR2`, `RAW`, `DATE`, `TIMESTAMP`, `BINARY_FLOAT`, and
`BINARY_DOUBLE`. Object-valued VARRAY elements are also supported.

## Object type

An object value exposes a runtime-generated struct. Each Oracle attribute is
represented by an exported `interface{}` field, so values are assigned and
read through reflection.

```sql
CREATE TYPE app_person AS OBJECT (
    id         NUMBER,
    name       VARCHAR2(50),
    created_at TIMESTAMP(9)
);
```

```go
person, err := datatype.NewObject(ctx, conn, "APP_PERSON")
if err != nil {
    return err
}
person.FieldByName("ID").Set(reflect.ValueOf(int64(42)))
person.FieldByName("NAME").Set(reflect.ValueOf("Ada"))
person.FieldByName("CREATED_AT").Set(reflect.ValueOf(time.Now().UTC()))

var out datatype.Object
_, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;",
    sql.Out{Dest: &out}, person,
)
if err != nil {
    return err
}
fmt.Println(out.FieldByName("NAME").Interface())
```

Object types can be inserted into and scanned from relational table columns.
For nested object and collection attributes, use the descriptor available from
the parent object type:

```go
childType := person.ObjectType.Attributes["CHILD"].ObjectType
child, err := childType.NewObject()
```

Use positional access (`Field(0)`, `Field(1)`) for object elements whose TDS
contains their shape but not SQL attribute names.

## Nested table

A nested table is an ordered SQL collection without a VARRAY upper bound. Use
the same `NewCollection`, `Set`, and `Get` APIs as a VARRAY.

```sql
CREATE TYPE app_number_table AS TABLE OF NUMBER;
```

```go
numbers, err := datatype.NewCollection(ctx, conn, "APP_NUMBER_TABLE")
if err != nil {
    return err
}
if err := numbers.Set([]any{int64(10), nil, int64(30)}); err != nil {
    return err
}

var out datatype.ObjectCollection
_, err = conn.ExecContext(ctx, "BEGIN :1 := :2; END;",
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
indexed, err := datatype.NewCollection(ctx, conn, "APP_ADT_PKG.NUMBER_BY_INDEX")
if err != nil {
    return err
}
if err := indexed.SetIndexed(map[int32]any{-2: int64(10), 3: int64(20)}); err != nil {
    return err
}

var out datatype.ObjectCollection
_, err = conn.ExecContext(ctx, "BEGIN APP_ADT_PKG.ECHO_NUMBERS(:1, :2); END;",
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
empty, _ := datatype.NewCollection(ctx, conn, "APP_NUMBER_VARRAY")
// empty has zero elements and is not NULL.

nullValue, _ := datatype.NewCollection(ctx, conn, "APP_NUMBER_VARRAY")
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

`NewObject` returns an error for a collection type. `NewCollection` returns an
error for an object type.

## Current scope

The supported behavior is exercised by the functional tests in
`oracle/OracleDriver_VARRY_test.go`, `oracle/OracleDriver_ObjectType_test.go`,
`oracle/OracleDriver_NestedTable_test.go`, and
`oracle/OracleDriver_AssociativeArray_test.go`.
