# Oracle LOB example

This runnable example exercises BLOB, CLOB, and NCLOB support through
`database/sql`. For API choices, ownership, text lengths, and cleanup, see the
[LOB guide](../../documentation/lob.md).

## Requirements

- Go and an Oracle Database user that can create and drop tables.
- Database character set `AL32UTF8` and national character set `AL16UTF16` for
  the CLOB and NCLOB scenarios.

## Run

From the repository root:

```shell
export ORACLE_DSN='user/password@host:port/service'
go run ./examples/lob
```

The program runs all three scenario categories by default. Set
`ORACLE_LOB_TEST` to run one category:

| Value | Runs |
| --- | --- |
| `all` | All scenarios (the default). |
| `boundary` | Two BLOB/CLOB boundary checks with prefetch counts 32,768 and 41,943,040. |
| `multiple` | Six LOB columns with counts 32,768 and the configured count (8,388,608 by default). |
| `typed` | Temporary and persistent typed BLOB/CLOB operations. |

For example:

```shell
ORACLE_LOB_TEST=boundary go run ./examples/lob
```

To change the prefetch count used by the second `multiple` scenario:

```shell
ORACLE_LOB_TEST=multiple \
ORACLE_LOB_MULTIPLE_PREFETCH_SIZE=4194304 \
go run ./examples/lob
```

`ORACLE_LOB_MULTIPLE_PREFETCH_SIZE` must be a positive signed 32-bit count.
Oracle interprets it as bytes for BLOBs and characters for CLOBs and NCLOBs.
The multiple-column scenario creates six payloads at least 65,536 units larger
than the count, so large overrides can require substantial memory. The
boundary scenarios also allocate large payloads. For a smaller local run,
reduce `boundaryPrefetchCount32K`, `boundaryPrefetchCount40M`, and
`boundaryPayloadExtraCount64K` in [main.go](main.go).

## What the example demonstrates

1. Each scenario creates its own table and attempts to drop it when finished.
   Cleanup failures are printed as warnings.
2. It inserts binary and text values with `oracle.BindBlob`, `oracle.BindClob`,
   and `oracle.BindNClob`.
3. It reads complete values into memory, streams values across prefetch
   boundaries, and scans six independent typed LOB locators through a dedicated
   connection. The six-column scenario includes two BLOBs, two CLOBs, and two
   NCLOBs.
4. It creates and modifies temporary BLOB and CLOB values, then scans and reads
   persistent BLOB and CLOB values using the typed `oracle/datatype/lob` API. The
   multiple-column scenario demonstrates typed NCLOB scans.

The complete public API and its cleanup requirements are covered in the
[LOB guide](../../documentation/lob.md).

## Source files

- [main.go](main.go) — configuration and scenario selection.
- [query_reads.go](query_reads.go) — boundary scenarios with materialized and
  streaming reads.
- [multiple.go](multiple.go) — six independent LOB columns from one row.
- [typed_lob.go](typed_lob.go) — temporary and persistent typed LOB operations.
