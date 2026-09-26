# REF CURSOR example

This example demonstrates both supported cursor-return patterns:

- a REF CURSOR returned through a PL/SQL `sql.Out` bind using `datatype.Rows`,
  then exposed as `*sql.Rows` with `GetRows`;
- implicit result cursors returned through `DBMS_SQL.RETURN_RESULT`:
  `QueryContext` exposes the first cursor as `*sql.Rows`, and
  `NextResultSet` advances to any additional cursors.

## Prerequisites

- An Oracle Database reachable from this machine.
- A connection string with database credentials.

## Run the example

Set `ORACLE_DSN` to a driver connection string, then run the example from the
repository root:

```bash
export ORACLE_DSN="user/password@localhost:1521/freepdb1"
go run ./examples/datatype/refcursor
```

Expected output is similar to:

```text
Columns: [ID LABEL]
Row: [1 first row]
Row: [2 second row]
Implicit result cursors
Result set 1, columns: [N]
Row: [11]
Result set 2, columns: [N]
Row: [22]
```

## How it works

1. The PL/SQL block opens a server cursor for its OUT bind using `OPEN :1 FOR`.
2. The Go program acquires a dedicated `*sql.Conn` and passes
   `sql.Out{Dest: &raw}`, where `raw` is a `datatype.Rows` value.
3. After `ExecContext` completes, the driver assigns the returned server cursor
   to `raw`.
4. `raw.GetRows(ctx, conn)` fetches the cursor and returns standard `*sql.Rows`.
   The program reads it with `Columns`, `Next`, and `Scan`, then closes it when
   finished.

Use one `datatype.Rows` destination for each REF CURSOR OUT bind. Always close
each `*sql.Rows`, including cursors that are not fully consumed.

## Implicit result cursors

The example also opens two local cursors and exposes them with
`DBMS_SQL.RETURN_RESULT`. `QueryContext` returns the first cursor as a
standard `*sql.Rows`; consume the current result set with `Next` and `Scan`.
Because this example returns a second cursor, it then calls `NextResultSet` to
advance to it. If PL/SQL returns only one implicit cursor, consume that
`*sql.Rows` normally; no `NextResultSet` call is needed. Close the outer
`*sql.Rows` when finished to release all remaining implicit cursors.
