# Working with Oracle LOBs

Oracle Database provides BLOBs for binary data, CLOBs for text in the database
character set, and NCLOBs for text in the national character set.

The Go Oracle Database Driver supports binding these values, reading them into
memory, and working with live LOB locators. Choose an API based on the size of
the data and whether you need to modify it.

## Choose an API

| What you need | API | Ownership and memory |
| --- | --- | --- |
| Bind binary data | `oracle.BindBlob(data)` | Marks a byte slice as a BLOB bind. The driver manages its temporary locator. |
| Bind text | `oracle.BindClob(text)` or `oracle.BindNClob(text)` | Marks a UTF-8 string with the required Oracle LOB type. |
| Read a complete BLOB | `lob.Bytes` | Materializes the complete result during Scan. |
| Read a complete CLOB or NCLOB | `lob.Text` | Materializes the complete result as UTF-8 text during Scan. |
| Stream or modify a persistent BLOB | `oracle/datatype/lob.Blob` | Holds a live locator tied to the connection that produced it. |
| Stream or modify a persistent CLOB or NCLOB | `oracle/datatype/lob.Clob` or `oracle/datatype/lob.NClob` | Provides UTF-8 data access and Oracle logical-unit operations. |
| Create a temporary LOB | `oracle/datatype/lob.CreateTemporaryBlob`, `CreateTemporaryClob`, or `CreateTemporaryNClob` | The application must call Free before closing the owning connection. |

Materialized bytes and text remain usable after Rows and the connection close.
Live LOB values require their owning connection to remain available.

The locator scan path is a go-oracledb-specific extension to `database/sql`.
Use the scan destinations listed above. Ordinary `string` and `[]byte`
destinations are not supported for these locator-backed results. These custom
scanner sources are outside the documented standard `driver.Value` result and
`sql.Scanner` source types.

## Contents

- [Requirements and example setup](#requirements-and-example-setup)
- [Quick start](#quick-start)
- [Bind LOB values](#bind-lob-values)
- [Read a complete LOB](#read-a-complete-lob)
- [Stream a BLOB](#stream-a-blob)
- [NULL and empty LOBs](#null-and-empty-lobs)
- [Create a temporary LOB](#create-a-temporary-lob)
- [Modify a persistent LOB](#modify-a-persistent-lob)
- [Ownership and cleanup](#ownership-and-cleanup)
- [Text encoding, lengths, and offsets](#text-encoding-lengths-and-offsets)
- [Contexts and concurrent use](#contexts-and-concurrent-use)
- [Prefetch and memory](#prefetch-and-memory)
- [Operation reference](#operation-reference)
- [Current limitations](#current-limitations)
- [Troubleshooting](#troubleshooting)
- [Run the complete example](#run-the-complete-example)

## Requirements and example setup

The driver supports Oracle Database 19c and later. The currently validated
character-set profile for LOB text is:

- Database character set `AL32UTF8` for CLOB.
- National character set `AL16UTF16` for NCLOB.

The snippets below assume an open `*sql.DB` named `db` and an operation
context named `ctx`. Imports used across the examples are:

```go
import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "io"
    "time"

    "github.com/oracle/go-oracledb/v26/oracle"
    datatypeLOB "github.com/oracle/go-oracledb/v26/oracle/datatype/lob"
    "github.com/oracle/go-oracledb/v26/oracle/datatype/lob"
)
```

## Quick start

This round trip creates a table with all three LOB types, inserts one row, and
reads each value into Go memory. Run the DDL once; drop the table before
repeating the example. The sections below are separate API reference examples
that reuse this table; they are not additional steps in this round trip.

```go
if _, err := db.ExecContext(ctx, `
    CREATE TABLE go_lob_documents (
        id            NUMBER PRIMARY KEY,
        payload       BLOB,
        body          CLOB,
        national_body NCLOB
    )`); err != nil {
    return err
}

_, err := db.ExecContext(ctx,
    `INSERT INTO go_lob_documents (id, payload, body, national_body)
     VALUES (:1, :2, :3, :4)`,
    int64(1),
    oracle.BindBlob([]byte{0x00, 0x01, 0xFE}),
    oracle.BindClob("CLOB text: A🙂B"),
    oracle.BindNClob("NCLOB text: 山田太郎"),
)
if err != nil {
    return err
}

var binary lob.Bytes
var clobText, nclobText lob.Text
if err := db.QueryRowContext(ctx,
    `SELECT payload, body, national_body
     FROM go_lob_documents WHERE id = :1`, int64(1),
).Scan(&binary, &clobText, &nclobText); err != nil {
    return err
}
fmt.Printf("BLOB %v\nCLOB %q\nNCLOB %q\n", binary, clobText, nclobText)
```

The runnable example creates and drops its own tables. See the
[example README](../examples/lob/README.md) for connection configuration,
scenario selection, and memory requirements.

## Bind LOB values

Use the marker matching the database column:

```go
_, err := db.ExecContext(ctx,
    "INSERT INTO go_lob_documents (id, payload, body, national_body) VALUES (:1, :2, :3, :4)",
    int64(1),
    oracle.BindBlob([]byte("binary payload")),
    oracle.BindClob("CLOB text: A🙂B"),
    oracle.BindNClob("NCLOB text: 山田太郎"),
)
if err != nil {
    return err
}
```

These markers distinguish LOB binds from scalar RAW and character binds and
support data beyond the corresponding scalar bind limits. Character inputs
must contain complete, valid UTF-8 text.

`BindBlob` shares the caller's byte-slice backing storage. Keep that slice
unchanged until ExecContext or QueryContext returns. The driver creates and
releases the temporary locator used by the bind; the application does not call
Free on a bind marker.

The marker inputs are byte slices and strings. Passing an arbitrary
`io.Reader` as a SQL argument is not this bind API.

## Read a complete LOB

Use `lob.Bytes` for a BLOB and `lob.Text` for a CLOB or NCLOB:

```go
var binary lob.Bytes
var text lob.Text

if err := db.QueryRowContext(ctx,
    "SELECT payload, body FROM go_lob_documents WHERE id = :1",
    int64(1),
).Scan(&binary, &text); err != nil {
    return err
}

// These values are ordinary Go memory and no longer depend on a locator.
fmt.Println(len(binary), string(text))
```

Each Scan reads the complete LOB and closes its query source before returning
success. SQL NULL is rejected by these materializing scanners.

For multiple rows, use QueryContext, scan inside the Next loop, close Rows, and
check Rows.Err after iteration. Each scanned result owns its materialized
contents.

Each value has a materialization limit of `math.MaxInt32` bytes. Large values
can require substantial memory below that limit; use a live LOB when the
complete value should not be held in Go memory.

## Stream a BLOB

Acquire a dedicated `*sql.Conn`, execute the query through it, and retain it
until all LOB operations and cleanup finish.

The examples use this cleanup helper to give cleanup its own bounded context
if the operation's context has expired:

```go
func freeLOB(value datatypeLOB.LOB) error {
    cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    return value.FreeContext(cleanupCtx)
}
```

Adjust the cleanup timeout to the application. A cleanup error must be
observed, especially when temporary storage is involved.

This function closes Rows before copying the adopted BLOB to a writer:

```go
func copyBlob(ctx context.Context, db *sql.DB, id int64, destination io.Writer) (err error) {
    conn, err := db.Conn(ctx)
    if err != nil {
        return err
    }
    defer func() {
        err = errors.Join(err, conn.Close())
    }()

    var value datatypeLOB.Blob
    defer func() {
        err = errors.Join(err, freeLOB(&value))
    }()

    rows, err := conn.QueryContext(ctx,
        "SELECT payload FROM go_lob_documents WHERE id = :1", id,
    )
    if err != nil {
        return err
    }
    defer rows.Close()

    if !rows.Next() {
        if err := rows.Err(); err != nil {
            return err
        }
        return sql.ErrNoRows
    }
    if err := rows.Scan(&value); err != nil {
        return err
    }
    if err := rows.Close(); err != nil {
        return err
    }
    if !value.Valid() {
        return fmt.Errorf("payload is SQL NULL")
    }

    _, err = value.WriteToContext(ctx, destination)
    return err
}
```

Scan adopts the persistent locator into the connection's current lease.
Closing Rows after this transfer does not end the dedicated connection's lease.

A query executed through `*sql.DB` does not provide the same post-Rows-close
guarantee: closing Rows can return the physical connection to the pool and
expire the handle. For a live handle that escapes Rows, query through the
dedicated connection as shown above.

For CLOB or NCLOB, use the corresponding typed value. Read and WriteTo expose
UTF-8 bytes. Use the Context methods to make later locator operations
cancellable.

`ReadAllContext` reads the remaining value into memory and returns `[]byte`
for BLOB or `string` for CLOB/NCLOB. It advances the cursor and leaves the
handle open. On failure, it can return a successfully read prefix with the
error. Call it before consuming the value if the complete contents are needed.

## NULL and empty LOBs

SQL NULL and an empty LOB are different:

| Value | Materializing scanner | Typed LOB |
| --- | --- | --- |
| SQL NULL | Scan returns an error. | Scan succeeds and Valid returns false. |
| Non-NULL, zero-length LOB | Scan succeeds with zero-length contents. | Valid returns true; Size reports zero. |
| Non-empty LOB | Scan returns its contents. | Valid returns true; locator operations are available while its owner is usable. |

Passing `nil` directly as a bind parameter represents SQL NULL. Use an
explicit LOB marker with empty data when binding an empty LOB.

To keep a nullable materialized value, wrap its scanner type with
`sql.Null[T]`:

```go
var body sql.Null[lob.Text]
if err := db.QueryRowContext(ctx,
    "SELECT body FROM go_lob_documents WHERE id = :1", int64(1),
).Scan(&body); err != nil {
    return err
}
if body.Valid {
    fmt.Println(string(body.V))
}
```

Typed values retain their declared Kind after a NULL scan. Valid is a
value-presence indicator: it may remain true after local Close or
invalidation, and it becomes false after successful Free. It does not prove
that a later database operation will succeed.

## Create a temporary LOB

Create a temporary LOB on a dedicated connection and release it before that
connection closes:

```go
func useTemporaryClob(ctx context.Context, conn *sql.Conn, text string) (err error) {
    value, err := datatypeLOB.CreateTemporaryClob(ctx, conn)
    if err != nil {
        return err
    }
    defer func() {
        err = errors.Join(err, freeLOB(value))
    }()

    n, err := value.WriteStringContext(ctx, text)
    if err != nil {
        return err
    }
    if n != len(text) {
        return io.ErrShortWrite
    }

    length, err := value.SizeContext(ctx)
    if err != nil {
        return err
    }
    fmt.Println("Oracle logical length:", length)
    return nil
}
```

Use the matching constructor for BLOB or NCLOB. Creation starts with an empty,
non-NULL LOB. Writes advance the sequential cursor; reading from that same
cursor immediately afterward reads the remaining suffix.

Use the explicit bind markers for SQL INSERT and UPDATE input. The temporary
constructor demonstrates direct locator operations; it does not imply that a
typed handle can be passed as an arbitrary SQL bind argument.

## Modify a persistent LOB

Perform persistent changes inside a transaction and obtain the locator with a
row-locking query such as SELECT ... FOR UPDATE.

This BLOB example replaces the contents. Write overwrites from the current
cursor and preserves any existing suffix, so the example also trims to the new
byte length:

```go
func replaceBlob(ctx context.Context, conn *sql.Conn, id int64, data []byte) (err error) {
    tx, err := conn.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    var value datatypeLOB.Blob
    defer func() {
        err = errors.Join(err, freeLOB(&value))
    }()

    if err := tx.QueryRowContext(ctx,
        "SELECT payload FROM go_lob_documents WHERE id = :1 FOR UPDATE", id,
    ).Scan(&value); err != nil {
        return err
    }
    if !value.Valid() {
        return fmt.Errorf("payload is SQL NULL")
    }

    n, err := value.WriteContext(ctx, data)
    if err != nil {
        return err
    }
    if n != len(data) {
        return io.ErrShortWrite
    }
    if err := value.TrimContext(ctx, int64(len(data))); err != nil {
        return err
    }

    // Release the handle before ending the transaction.
    if err := freeLOB(&value); err != nil {
        return err
    }
    return tx.Commit()
}
```

The caller owns conn and keeps it open for this function. The deferred Free is
a fallback; a repeated successful Free is a no-op.

For CLOB or NCLOB replacement, trim to zero first. This empties the value and
resets its sequential cursor, so the text is written from the beginning without
calculating a UTF-16 length from Go's UTF-8 bytes:

```go
func replaceClob(ctx context.Context, conn *sql.Conn, id int64, text string) (err error) {
    tx, err := conn.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    var value datatypeLOB.Clob
    defer func() {
        err = errors.Join(err, freeLOB(&value))
    }()

    if err := tx.QueryRowContext(ctx,
        "SELECT body FROM go_lob_documents WHERE id = :1 FOR UPDATE", id,
    ).Scan(&value); err != nil {
        return err
    }
    if !value.Valid() {
        return fmt.Errorf("body is SQL NULL")
    }

    if err := value.TrimContext(ctx, 0); err != nil {
        return err
    }
    n, err := value.WriteStringContext(ctx, text)
    if err != nil {
        return err
    }
    if n != len(text) { // WriteString reports acknowledged UTF-8 bytes.
        return io.ErrShortWrite
    }

    if err := freeLOB(&value); err != nil {
        return err
    }
    return tx.Commit()
}
```

The text must be valid UTF-8. `WriteStringContext` reports bytes accepted, so
`len(text)` is appropriate for checking that write count; it is not the CLOB
length to pass to Trim. Use the same pattern with `oracle/datatype/lob.NClob` for NCLOB.

Complete operations on the locator within the transaction that obtains the
row lock. CLOB/NCLOB trimming uses logical units rather than UTF-8 byte length.
A write may acknowledge a prefix before a later request fails; inspect both
the returned count and error and handle the transaction accordingly.

### Batch writes to indexed LOBs

When making several writes to a persistent LOB column with a function-based or
domain index, explicitly opening the LOB in read/write mode can defer index
maintenance until close. Pair every explicit open with `CloseServerContext`
before releasing the value or ending the transaction. See Oracle's
[guidance on indexing LOB columns](https://docs.oracle.com/en/database/oracle/oracle-database/26/adlob/creating-index-on-LOB-column.html).

## Ownership and cleanup

| Operation or event | Effect |
| --- | --- |
| Materializing Scan | Consumes and closes the query source; returned Go data remains independent. |
| Open / OpenContext | Establishes explicit Oracle LOB open state. Ordinary reads and writes do not require an explicit Open. |
| CloseServer / CloseServerContext | Ends explicit LOB open state and leaves the Go handle usable. For temporary LOBs this updates local flags. |
| Close | Ends local Go handle access. It does not release temporary Oracle storage. |
| Free / FreeContext on a temporary LOB | Queues temporary-locator cleanup with the owning session for its next ordinary TTC request. |
| Free / FreeContext on a persistent LOB | Releases the local handle without deleting the stored LOB data. |
| Close of the owning sql.Conn | Ends the connection lease; escaped typed handles cannot be used afterward. |

A successful temporary Free means cleanup has been accepted locally; it does
not guarantee an immediate server round trip or immediate storage reclamation.
Session teardown also reclaims session-duration temporary storage.

Finish cleanup in this order:

1. End any explicit persistent LOB open state with CloseServerContext.
2. Free the LOB and observe its error.
3. Close the owning connection after all its LOBs are finished.

For transaction-scoped work, finish the LOB operations and required explicit
close before committing or rolling back.

An ambiguous OpenContext or CloseServerContext failure can leave open state
unknown. Use IsOpenContext to reconcile it while the owning session remains
usable before attempting another transition or release.

## Text encoding, lengths, and offsets

Go-facing text is UTF-8. Oracle character-LOB logical positions use UTF-16
code units; supplementary characters occupy two units.

| Operation | BLOB | CLOB / NCLOB |
| --- | --- | --- |
| Read and Write data | Bytes | UTF-8 bytes |
| Read/Write return count | Bytes | UTF-8 bytes |
| WriteString return count | Not provided | UTF-8 bytes |
| Size and Trim length | Bytes | UTF-16 code units |
| ReadAt offset | Zero-based bytes | Not provided |
| ReadTextAt offset and returned unit count | Not provided | Zero-based UTF-16 code units |
| ChunkSize | Storage bytes | Storage bytes |

For example, `"A🙂B"` contains 6 UTF-8 bytes and 4 UTF-16 code units.
Its Go byte length and Oracle logical length are therefore different.

ReadTextAt returns complete UTF-8 characters and reports both bytes and
logical units. An offset inside a surrogate pair is invalid. A destination
that cannot fit the next complete character may produce `io.ErrShortBuffer`
alongside the complete-character prefix. Consume the prefix using `nBytes` and
advance the next offset by `nUnits`. This example copies from an arbitrary
zero-based logical offset without changing the sequential cursor:

```go
type textAtReader interface {
    ReadTextAtContext(context.Context, []byte, int64) (int, int64, error)
}

func copyTextFrom(ctx context.Context, value textAtReader, offset int64, dst io.Writer) error {
    buffer := make([]byte, 32*1024)
    for {
        nBytes, nUnits, readErr := value.ReadTextAtContext(ctx, buffer, offset)
        if nBytes > 0 {
            nWritten, writeErr := dst.Write(buffer[:nBytes])
            if writeErr != nil {
                return writeErr
            }
            if nWritten != nBytes {
                return io.ErrShortWrite
            }
            offset += nUnits
        }

        switch {
        case errors.Is(readErr, io.ErrShortBuffer):
            if nBytes == 0 {
                return io.ErrShortBuffer
            }
            continue
        case errors.Is(readErr, io.EOF):
            return nil
        case readErr != nil:
            return readErr
        case nBytes == 0:
            return io.ErrNoProgress
        }
    }
}
```

Pass either a `*oracle/datatype/lob.Clob` or `*oracle/datatype/lob.NClob` to this helper.
`ReadTextAtContext` is positional and does not move the sequential read
cursor. The typed values do not provide Seek. A successful
`TrimContext(ctx, 0)` clears the LOB and resets that cursor to the beginning.
Other trims preserve the cursor unless it lies beyond the new end, in which
case it is clamped to EOF.

For materialized NCLOB text, scan directly into `lob.Text`:

```go
var text lob.Text
if err := db.QueryRowContext(ctx,
    "SELECT national_body FROM go_lob_documents WHERE id = :1", int64(1),
).Scan(&text); err != nil {
    return err
}
fmt.Println(string(text)) // lob.Text contains UTF-8.
```

For locator operations, scan the same column into `oracle/datatype/lob.NClob` through a
dedicated connection, keep that connection open, and call `ReadAllContext` to
materialize a Go `string`:

```go
func readNationalBody(ctx context.Context, conn *sql.Conn, id int64) (text string, err error) {
    var value datatypeLOB.NClob
    defer func() {
        err = errors.Join(err, freeLOB(&value))
    }()

    if err = conn.QueryRowContext(ctx,
        "SELECT national_body FROM go_lob_documents WHERE id = :1", id,
    ).Scan(&value); err != nil {
        return "", err
    }
    if !value.Valid() {
        return "", fmt.Errorf("national_body is SQL NULL")
    }
    text, err = value.ReadAllContext(ctx)
    return text, err
}
```

`NClob.ReadAllContext` returns a UTF-8 Go `string`; convert it to `lob.Text`
with `lob.Text(text)` only when an API specifically requires that named type.
Both CLOB and NCLOB data are UTF-8 at the Go API boundary.

Each CLOB/NCLOB Write or WriteString call must contain complete, valid UTF-8
characters. Large calls are divided into bounded requests at character
boundaries. Arbitrary byte chunks from io.Copy do not necessarily meet that
requirement.

`ReadAllContext` returns the text read so far along with an error. On an error,
that partial string may end inside a UTF-8 character, so it is not guaranteed
to be valid UTF-8.

Sequential Read can divide a UTF-8 character across calls. Before changing a
character LOB after a partial-character read, finish reading that character.

## Contexts and concurrent use

Pass an operation context with an appropriate deadline. ReadContext,
WriteContext, WriteToContext, ReadAllContext, and the other Context variants
use the supplied context. Convenience methods without Context generally use
context.Background.

A typed LOB value is not safe for concurrent use. Serialize all operations on
one value, including Scan, reads, writes, Close, and Free. Do not copy a typed
LOB after its first use.

Multiple LOB values can belong to one connection, but they share its physical
TTC stream. This is not a promise of parallel network operations on that
connection.

Cancellation or a transport error can leave a mutation's outcome uncertain.
The driver can invalidate the handle or session. A returned byte count
describes acknowledged input; it does not establish that an errored request
made no change. Do not blindly retry a partially applied write.

## Prefetch and memory

Prefetch fetches an initial portion of LOB data with a query result. It is
independent of the choice between materializing a result and keeping a live
handle.

`oracle/config.DefaultLobPrefetchSize` defaults to the numeric count
`33,554,432` (`32 * 1024 * 1024`). The driver passes this count to Oracle,
which interprets it as bytes for BLOBs and characters for CLOBs and NCLOBs.
For character LOBs, `33,554,432` means that many characters; the actual memory
footprint depends on the encoding and can exceed `33,554,432` bytes. Larger
counts can reduce later locator reads while increasing query memory use. Rows
containing several LOB columns can multiply that cost. See Oracle's
[LOB prefetch documentation](https://docs.oracle.com/en/database/oracle/oracle-database/19/jjdbc/performance-extensions.html).

Streaming avoids accumulating the complete value in a second application
buffer, but the prefetched data still occupies memory. Set prefetch according
to expected row count, LOB columns, payload sizes, and memory budget.

ChunkSize reports an Oracle storage property. It is not the same setting as
prefetch size or the driver's bounded transfer buffer.

Set prefetch when constructing the connector, for example:

```go
cfg := oracle.NewOracleDriverConfig()
cfg.ConnectDescriptor = "user/password@localhost:1521/freepdb1"
cfg.DriverProperties.DefaultLobPrefetchSize = 32 * 1024

connector, err := oracle.NewOracleConnector(cfg)
if err != nil {
    return err
}
db := sql.OpenDB(connector)
defer db.Close()
```

This example sets the count to `32,768`: bytes for a BLOB and characters for a
CLOB or NCLOB. The character count is not a 32 KiB memory limit. See the
[example configuration](../examples/lob/main.go) and the
[driver configuration guide](../README.md#driver-configuration).

## Operation reference

The typed values share these operations. Use the Context variants to provide
an operation deadline or cancellation.

| Task | Methods |
| --- | --- |
| Read from the current cursor | Read, ReadContext |
| Copy remaining contents to a writer | WriteTo, WriteToContext |
| Materialize remaining contents | ReadAll, ReadAllContext on each concrete type |
| Write at the current cursor | Write, WriteContext |
| Inspect metadata | Kind, Valid, IsTemporary, Size, ChunkSize |
| Shorten the LOB | Trim, TrimContext |
| Manage explicit Oracle open state | Open, CloseServer, IsOpen and their Context variants |
| Release resources | Close, Free, FreeContext |

Blob additionally provides ReadAt and ReadAtContext. Clob and NClob provide
ReadTextAt, ReadTextAtContext, WriteString, and WriteStringContext. These typed
values do not provide a Seek operation.

## Current limitations

- Query locator scans support persistent table LOBs. Query results marked
  temporary or abstract are rejected; some SQL expressions and functions
  produce those kinds of LOBs. This also affects materializing scans because
  source validation happens before the scan destination is called.
- Character support is currently validated for AL32UTF8 CLOB and AL16UTF16
  NCLOB. Other character-set profiles are not yet validated.
- Materializing scanners limit complete output to math.MaxInt32 bytes. Typed
  ReadAll operations apply the same limit to the remaining contents returned
  by that call. Bounded streaming reads avoid full materialization.
- BLOB, CLOB, and NCLOB destinations are kind-specific. Text accepts either
  character kind; a typed Clob and a typed NClob each require their own kind.
- BFILE has no public typed value in this feature.
- Use the documented driver-specific scanners for locator-backed query
  results. Their custom Scanner input is not portable to arbitrary Go SQL
  drivers.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Scan into string or []byte fails | Use lob.Text or lob.Bytes for a complete value, or a matching datatype value for live operations. |
| LobValueInvalidated | Check whether the connection lease ended, the owner closed, or a prior operation made the handle/session unsafe. |
| InvalidLobSource for an expression or function result | Check whether Oracle returned a temporary or abstract query locator. |
| NULL scan error | Materializing scanners reject SQL NULL. Use the typed value's NULL handling or a suitable nullable destination. |
| Size differs from len(text) | Character LOB Size uses UTF-16 units; len(text) counts UTF-8 bytes. |
| A write leaves old trailing contents | Writes preserve the existing suffix. For replacement, trim to zero before writing or trim afterward using the LOB's logical units. |
| LobOpen or LobOpenStateUnknown during cleanup | End or reconcile the explicit Oracle open state before releasing the handle. |
| A failed write reports a nonzero count | Some requests were acknowledged before the failure. Handle the partial modification within the transaction. |
| Streaming still consumes substantial memory | Check prefetch size and the number of fetched LOB columns/rows. |

## Run the complete example

Set the connection string and run from the repository root:

```shell
export ORACLE_DSN='user/password@localhost:1521/freepdb1'
go run ./examples/lob
```

The sample requires table creation/drop privileges. Its default run includes
large prefetch and multiple-column scenarios; consult the
[example README](../examples/lob/README.md) for memory requirements and selection
of the boundary, multiple-column, or typed-operation scenarios.

The example covers materialized reads, live query LOBs, explicit input
markers, and temporary/persistent operations.
