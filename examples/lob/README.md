# Oracle LOB example

This runnable example demonstrates BLOB, CLOB, and NCLOB support through
`database/sql`:

- bind large values with `oracle.BindBlob`, `oracle.BindClob`, and
  `oracle.BindNClob`;
- read a complete value with `lob.Bytes` or `lob.Text`;
- stream a query result with `datatype.Blob`, `datatype.Clob`, and
  `datatype.NClob`; and
- create and modify LOBs through the typed `datatype` API.

The program creates its own tables, verifies deterministic data, and drops the
tables when each test finishes.

## Requirements

- Go and an Oracle Database user that can create and drop tables.
- Database character set `AL32UTF8` and national character set `AL16UTF16` for
  the CLOB and NCLOB tests.

## Run

From the repository root:

```shell
export ORACLE_DSN='user/password@host:port/service'
go run ./examples/lob
```

By default, the example runs all tests:

1. a boundary read with 32 KiB LOB prefetch;
2. a boundary read with 40 MiB LOB prefetch;
3. six independent LOB columns with 32 KiB prefetch;
4. the same six-column test with 8 MiB prefetch; and
5. temporary and persistent typed operations for BLOB and CLOB values.

To run one test category, set `ORACLE_LOB_TEST`:

| Value | Category |
| --- | --- |
| `all` | All three test categories (the default). |
| `boundary` | Two boundary checks: 32 KiB and 40 MiB prefetch. |
| `multiple` | Two six-column checks: 32 KiB and 8 MiB prefetch. |
| `typed` | Temporary and persistent typed BLOB/CLOB operations. |

For example:

```shell
ORACLE_LOB_TEST=boundary go run ./examples/lob
```

To change the prefetch used by the multiple-default test:

```shell
ORACLE_LOB_TEST=multiple \
ORACLE_LOB_MULTIPLE_PREFETCH_SIZE=4194304 \
go run ./examples/lob
```

`ORACLE_LOB_MULTIPLE_PREFETCH_SIZE` is a positive byte count. The boundary
tests use 32 KiB and 40 MiB and can allocate large values. For a smaller
local run, reduce `boundaryPrefetch32KiB`, `boundaryPrefetch40MiB`, and
`boundaryPayloadExtra64KiB` in [main.go](main.go).

## Public API patterns

### Bind LOB values

Use the marker matching the database column:

```go
_, err := db.ExecContext(ctx, query,
    oracle.BindBlob(binaryData),
    oracle.BindClob(clobText),
    oracle.BindNClob(nclobText),
)
```

### Materialize a query result

`lob.Bytes` is for BLOBs. `lob.Text` is for CLOBs and NCLOBs. These scanners
consume the value during `Scan`, so `QueryRowContext` is appropriate.

```go
var blob lob.Bytes
var text lob.Text
err := db.QueryRowContext(ctx, query).Scan(&blob, &text)
```

Materialization keeps the complete value in memory.

### Stream a query result

Use a dedicated `*sql.Conn` for a typed scan target and execute the query
through that connection. Declare the typed value directly; `Scan` adopts a
non-NULL persistent locator into a private, lease-checked connection handle.
The typed value remains usable after `Rows.Close` and before `conn.Close`.
Rows from `*sql.DB` do not provide that guarantee: closing them may return the
LOB's physical session to the pool and expire the typed handle's session
capability. This prevents operations against a reused session; it does not
itself destroy the Oracle locator or delete the persistent LOB in Oracle. If a
`*sql.DB` query only needs the data, call `blob.ReadAllContext(ctx)` for a BLOB
or the matching `ReadAllContext` method for a CLOB/NCLOB after `Scan` and before
consuming the LOB or advancing to another row/closing `Rows`. On success, the
returned `[]byte` or `string` remains usable after `Rows.Close`; the live
locator-backed handle does not.
Reads use bounded buffers, and the value supports writes, trimming, open-state,
and lifecycle operations.

```go
import "github.com/oracle/go-oracledb/v26/oracle/datatype"

conn, err := db.Conn(ctx)
if err != nil {
    return err
}
defer conn.Close()

var blob datatype.Blob
defer blob.FreeContext(ctx)

rows, err := conn.QueryContext(ctx, query)
if err != nil {
    return err
}
if !rows.Next() {
    rows.Close()
    if err := rows.Err(); err != nil {
        return err
    }
    return sql.ErrNoRows
}
if err := rows.Scan(&blob); err != nil {
    rows.Close()
    return err
}
if err := rows.Close(); err != nil {
    return err
}

_, err = blob.WriteToContext(ctx, destination)
return err
```

`datatype.Blob`, `datatype.Clob`, and `datatype.NClob` report LOB lengths in
Oracle logical units: bytes for BLOBs and UTF-16 code units for CLOBs and
NCLOBs. Use `ReadAll` to materialize the remaining value; it returns `[]byte`
for BLOB and UTF-8 `string` for CLOB and NCLOB.

### Create and modify LOBs

Create temporary typed values on the same dedicated connection:

```go
temporary, err := datatype.CreateTemporaryClob(ctx, conn)
if err != nil {
    return err
}
defer temporary.FreeContext(ctx)

_, err = temporary.WriteStringContext(ctx, "temporary text")
```

The typed values provide context-aware read, write, metadata, trim, open-state,
and cleanup methods. CLOB and NCLOB also provide `WriteString` and
`WriteStringContext` for UTF-8 text. `Close` releases only local Go state; it
does not free temporary Oracle storage. Call `Free` or `FreeContext` before
closing the owning connection when the value is temporary. Use a row-locking
query such as `FOR UPDATE` when modifying a persistent query value.

The typed temporary-LOB example uses BLOB and CLOB to keep the lifecycle
example small; NCLOB is covered by the multiple-column example.

## Source files

- [main.go](main.go) — configuration and test-category selection.
- [boundary.go](boundary.go) — boundary scenario with materialized and streaming reads.
- [streaming.go](streaming.go) — typed streaming reads after `Rows.Close`.
- [multiple.go](multiple.go) — independent LOBs from one row.
- [typed_lob.go](typed_lob.go) — temporary and persistent typed LOB operations.
