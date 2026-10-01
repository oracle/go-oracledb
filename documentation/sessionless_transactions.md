# Sessionless transactions

An Oracle sessionless transaction can be detached from one database connection
and later resumed on another connection. This is useful when a transaction must
survive a connection handoff, a worker change, or a period during which the
application does not need to hold a physical connection.

Sessionless transactions are exposed through the Oracle-specific connection
wrapper. They are not started with `sql.DB.Begin` or `sql.Conn.BeginTx`.

## Requirements

Sessionless transactions require:

- an Oracle Database and service that support sessionless transactions;
- a dedicated `*sql.Conn` obtained from the connection pool; and
- a connection wrapper created with `oracle.NewConnectionWrapper`.

The transaction can use the standard `sql.TxOptions` values supported by the
driver. Timeout arguments are in seconds. For `BeginSessionlessTx`, the timeout
controls how long the server keeps a suspended transaction available before
rolling it back. For `ResumeSessionlessTx`, the timeout controls how long the
server attempts to resume the transaction; it does not extend the original
suspension lifetime.

## Begin and commit a transaction

Use the wrapper around a dedicated connection to start the transaction. The
returned transaction provides the usual SQL methods plus `Suspend` and
`GlobalTransactionID`.

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/oracle/go-oracledb/v26/oracle"
)

func main() {
	ctx := context.Background()
	db, err := sql.Open("oracledb", os.Getenv("ORACLE_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	connection, err := oracle.NewConnectionWrapper(conn)
	if err != nil {
		log.Fatal(err)
	}

	tx, err := connection.BeginSessionlessTx(
		ctx,
		sql.TxOptions{Isolation: sql.LevelReadCommitted},
		300,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback() // Safe cleanup if a later operation fails.

	if _, err := tx.ExecContext(ctx,
		"UPDATE accounts SET balance = balance - 10 WHERE id = 1",
	); err != nil {
		log.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
}
```

The transaction is associated with the dedicated connection until it is
committed, rolled back, or suspended. Only one transaction can be active on a
connection at a time.

## Suspend and resume on another connection

`GlobalTransactionID` identifies the server-side transaction. Read it after the
first SQL round trip following `BeginSessionlessTx`, then save it before calling
`Suspend`. Pass it to `ResumeSessionlessTx` on a wrapper around a different
dedicated connection.

The following fragment assumes that `db` is already open and that it is inside
a function returning an `error`.

The timeout passed to resume does not extend the transaction's original
post-suspension lifetime. Set it according to how long a resume attempt should
wait, and use the begin timeout to control how long the suspended transaction
can remain available for resumption.

```go
ctx := context.Background()

conn1, err := db.Conn(ctx)
if err != nil {
	return err
}
connection1, err := oracle.NewConnectionWrapper(conn1)
if err != nil {
	conn1.Close()
	return err
}

tx, err := connection1.BeginSessionlessTx(ctx, sql.TxOptions{}, 300)
if err != nil {
	conn1.Close()
	return err
}

if _, err := tx.ExecContext(ctx, "INSERT INTO work_items (id) VALUES (1)"); err != nil {
	tx.Rollback()
	conn1.Close()
	return err
}

// Begin is piggybacked, so read the GTRID after the first SQL round trip. The
// server may provide the authoritative identifier when it acknowledges begin.
globalTransactionID := tx.GlobalTransactionID()

if err := tx.Suspend(); err != nil {
	conn1.Close()
	return err
}
// The old transaction handle is ended after Suspend.
conn1.Close()

conn2, err := db.Conn(ctx)
if err != nil {
	return err
}
defer conn2.Close()

connection2, err := oracle.NewConnectionWrapper(conn2)
if err != nil {
	return err
}

resumedTx, err := connection2.ResumeSessionlessTx(ctx, globalTransactionID, 300)
if err != nil {
	return err
}

if _, err := resumedTx.ExecContext(ctx,
	"UPDATE work_items SET processed = 1 WHERE id = 1",
); err != nil {
	resumedTx.Rollback()
	return err
}

return resumedTx.Commit()
```

The begin and resume requests are piggyback operations. The first SQL
round-trip after `BeginSessionlessTx` or `ResumeSessionlessTx` sends the
request to the server, so applications should perform a SQL operation after
each lifecycle call before assuming that the request has completed. In
particular, read `GlobalTransactionID` after the first SQL round trip following
begin because the server may replace the locally generated identifier.

The complete runnable version of this workflow is available in
[`examples/sessionless-transactions`](../examples/sessionless-transactions/README.md).

## Statements

Use the sessionless transaction to prepare and execute statements that belong
to it:

```go
stmt, err := tx.PrepareContext(ctx,
	"UPDATE accounts SET balance = balance + :1 WHERE id = :2",
)
if err != nil {
	return err
}
defer stmt.Close()

if _, err := stmt.ExecContext(ctx, 10, 2); err != nil {
	return err
}
```

Statements prepared through a sessionless transaction are owned by that
transaction. The driver closes them when the transaction is committed, rolled
back, or suspended. Explicitly closing them is still recommended when they are
no longer needed.

## Contexts and cancellation

The context passed to `BeginSessionlessTx` or `ResumeSessionlessTx` remains the
transaction's lifecycle context. Keep it valid until the transaction is
committed, rolled back, or suspended. If it is canceled while the transaction
is active, the driver attempts a bounded rollback. If the rollback result is
ambiguous, the connection is considered unusable and should not be returned to
application work.

Use the context passed to `ExecContext`, `QueryContext`, or
`PrepareContext` for the individual SQL operation. Canceling an individual SQL
operation does not replace an explicit transaction-ending operation.

## Ending and reusing a transaction

- `Commit` permanently ends the transaction.
- `Rollback` permanently ends the transaction.
- `Suspend` ends the transaction on the current connection but leaves it
  resumable by another sessionless transaction handle.
- After the transaction has ended, operations on its public handle return
  `sql.ErrTxDone`; `GlobalTransactionID` returns `nil`.
- Statements owned by the transaction are closed by the driver during each
  ending operation.

After suspending, do not use the old transaction handle. Resume the transaction
with its global transaction ID and a new dedicated connection instead.

