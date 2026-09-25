# Sessionless transaction example

This example demonstrates how to use an Oracle sessionless transaction with the
Go driver. A sessionless transaction can be detached from one database
connection and later resumed on another connection using its global transaction
ID.

The example performs the following operations:

1. Opens a database connection pool.
2. Gets a dedicated `*sql.Conn`, wraps it with `oracle.NewConnectionWrapper`,
   and begins a sessionless transaction.
3. Inserts a row on the first connection and verifies that it is visible there.
4. Suspends the transaction and keeps its global transaction ID.
5. Gets a second dedicated `*sql.Conn`, wraps it, and resumes the transaction
   using that ID.
6. Inserts a second row on the second connection and verifies that both rows are visible.
7. Commits the transaction and verifies that both rows are visible through the pool.

## Requirements

- The Go version specified by the repository's `go.mod` file.
- An Oracle Database that supports sessionless transactions.
- A database user with permission to create and drop tables.
- A valid Oracle DSN in the `ORACLE_DSN` environment variable.

## Run the example

From this directory, run:

```shell
ORACLE_DSN='user/password@localhost:1521/freepdb1' go run .
```

On PowerShell, use:

```powershell
$env:ORACLE_DSN = 'user/password@localhost:1521/freepdb1'
go run .
```

The DSN can also be supplied as a full Oracle connect descriptor. See the main
project [README](../../README.md) for supported DSN formats.

## Important details

`BeginSessionlessTx` and `ResumeSessionlessTx` are methods on the connection
wrapper returned by `oracle.NewConnectionWrapper`. The wrapper is created from
a dedicated `*sql.Conn`, not directly from `*sql.DB`, and validates that the
underlying driver connection supports sessionless transactions. For example:

```go
connectionWrapper, err := oracle.NewConnectionWrapper(conn)
if err != nil {
    return err
}
tx, err := connectionWrapper.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
```

Use another wrapper around the connection that resumes the transaction:

```go
resumeConnectionWrapper, err := oracle.NewConnectionWrapper(resumeConn)
if err != nil {
    return err
}
resumedTx, err := resumeConnectionWrapper.ResumeSessionlessTx(ctx, globalTransactionID, 300)
```

The global transaction ID identifies the server-side transaction and should be
retained until the transaction is committed or rolled back.

Sessionless start and resume requests are piggyback operations. The first SQL
operation after begin or resume sends the request to the server, so the example
executes an insert immediately after each lifecycle operation.

The example creates and drops a table for each run, so the database user must
be allowed to create and drop tables. The inserts are the transactional DML;
the `SELECT COUNT(*)` statements only verify which uncommitted rows are visible
from each connection. Replace `Commit` with `Rollback` when the inserts should
be discarded.
