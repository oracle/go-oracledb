# REF CURSOR support

Oracle can return query results from PL/SQL in two ways:

- **Explicit REF CURSOR OUT binds**: PL/SQL opens a cursor into an `OUT`
  parameter. Bind a `datatype.Cursor` value, then fetch it through an Oracle
  connection wrapper.
- **Implicit results**: PL/SQL calls `DBMS_SQL.RETURN_RESULT`. Execute the
  block with `QueryContext` and use `NextResultSet` to advance through the
  returned cursors.

Both mechanisms expose their data as standard `*sql.Rows`, so use `Next`,
`Scan`, `Err`, and `Close` as for an ordinary query.

## Explicit REF CURSOR OUT bind

An explicit REF CURSOR belongs to the physical Oracle connection that executed
the PL/SQL block. Acquire a dedicated `*sql.Conn`, keep it open while consuming
the cursor, and create an Oracle wrapper for that same connection.

```go
conn, err := db.Conn(ctx)
if err != nil {
    return err
}
defer conn.Close()

wrapper, err := oracle.NewConnectionWrapper(conn)
if err != nil {
    return err
}

var cursor datatype.Cursor
_, err = conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR
    SELECT employee_id, first_name
    FROM employees
    WHERE department_id = :2
    ORDER BY employee_id;
END;`,
    sql.Out{Dest: &cursor},
    10,
)
if err != nil {
    return err
}

// Fetch uses the same dedicated connection and returns ordinary *sql.Rows.
rows, err := wrapper.Fetch(ctx, &cursor)
if err != nil {
    return err
}
if rows == nil {
    // The PL/SQL block completed but left the OUT cursor NULL.
    return nil
}
defer rows.Close()

for rows.Next() {
    var employeeID int64
    var firstName string
    if err := rows.Scan(&employeeID, &firstName); err != nil {
        return err
    }
    fmt.Println(employeeID, firstName)
}
return rows.Err()
```

Do not use `db.ExecContext` for this pattern: it may use a different pooled
connection when the cursor is fetched. Do not close `conn` until the returned
`*sql.Rows` has been closed or fully consumed.

### Multiple explicit cursors

Use one `datatype.Cursor` value per OUT bind, and fetch each cursor through the
same wrapper:

```go
var employees, departments datatype.Cursor
_, err = conn.ExecContext(ctx, `
BEGIN
  OPEN :1 FOR SELECT employee_id FROM employees ORDER BY employee_id;
  OPEN :2 FOR SELECT department_id FROM departments ORDER BY department_id;
END;`,
    sql.Out{Dest: &employees},
    sql.Out{Dest: &departments},
)
if err != nil {
    return err
}

employeeRows, err := wrapper.Fetch(ctx, &employees)
if err != nil {
    return err
}
defer employeeRows.Close()

departmentRows, err := wrapper.Fetch(ctx, &departments)
if err != nil {
    return err
}
defer departmentRows.Close()
```

## Implicit results

Implicit results do not require OUT binds or `datatype.Cursor`. Return one or
more cursors from PL/SQL with `DBMS_SQL.RETURN_RESULT`, execute the block with
`QueryContext`, and read each cursor as a result set.

```go
rows, err := db.QueryContext(ctx, `
DECLARE
  employees SYS_REFCURSOR;
  departments SYS_REFCURSOR;
BEGIN
  OPEN employees FOR SELECT employee_id FROM employees ORDER BY employee_id;
  DBMS_SQL.RETURN_RESULT(employees);

  OPEN departments FOR SELECT department_id FROM departments ORDER BY department_id;
  DBMS_SQL.RETURN_RESULT(departments);
END;`)
if err != nil {
    return err
}
defer rows.Close()

for {
    for rows.Next() {
        var id int64
        if err := rows.Scan(&id); err != nil {
            return err
        }
        fmt.Println(id)
    }
    if err := rows.Err(); err != nil {
        return err
    }
    if !rows.NextResultSet() {
        break
    }
}
return rows.Err()
```

Always consume or close the outer `*sql.Rows`. Closing it releases the active
implicit result and any remaining implicit cursors.

## Resource and error handling

- Call `Close` on every explicit `*sql.Rows` result, including results that are
  only partially read.
- For implicit results, close the outer `*sql.Rows` after processing all result
  sets.
- Check `rows.Err()` after every `Next` loop and after `NextResultSet` stops.
- Pass a context with an appropriate deadline to `ExecContext`, `QueryContext`,
  and `wrapper.Fetch`.
