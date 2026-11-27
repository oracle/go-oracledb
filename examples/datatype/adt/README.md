# Oracle ADT example

This runnable example demonstrates the Oracle ADTs currently supported by the
driver:

- `VARRAY` of `NUMBER`;
- SQL nested table of `NUMBER`, including a NULL element;
- scalar object type with `NUMBER`, `VARCHAR2`, and `TIMESTAMP` attributes;
- `PLS_INTEGER`-indexed associative array declared in a PL/SQL package.

It creates temporary schema objects with the `GO_DRIVER_ADT_` prefix, performs
PL/SQL IN/OUT round trips, and drops those objects on exit. Run it in a schema
where creating and dropping types and packages is appropriate.

## Run

From the repository root:

```bash
export ORACLE_DSN='user/password@localhost:1521/freepdb1'
go run ./examples/datatype/adt
```

The output includes collection values, object attribute values, and the sparse
associative-array indexes returned by Oracle.

## Key points

- Constructors take a type name, not a connection. The executing connection resolves and caches type metadata.
- Collections are populated and read as whole values with `Set`/`Get`, or
  `SetIndexed`/`GetIndexed` for associative arrays.
- Object attributes are stored by database name: `person.Attributes["NAME"] = "Ada"`.

See [the ADT guide](../../../documentation/adt.md) for API details and SQL
examples.
