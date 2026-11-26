# Deep Data Security

Oracle Deep Data Security (DDS) lets you define application data access rules in Oracle AI Database. The database enforces those rules at the row, column, and cell levels. It uses an *end-user security context* to identify the user and evaluate their access. [Learn more about Oracle Deep Data Security](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/what-is-oracle-deep-data-security.html)

The Oracle Database Driver for Go supports Oracle Deep Data Security by either setting the end-user security context on a dedicated connection or supply it through a provider.

## Set and clear the context on a connection

Call `SetEndUserSecurityContext` before running the user's SQL. The setter selects the context locally, and Oracle Database validates it when SQL executes. Call `ClearEndUserSecurityContext` when the user's work is complete.

### Build the context

An `EndUserSecurityContext` carries the identity and authorization details for the SQL operation. Import the driver's SQL package to create it.

```go
import osql "github.com/oracle/go-oracledb/v26/oracle/sql"
```

| Field | Go type | What to supply |
| --- | --- | --- |
| `DatabaseAccessToken` | `string` | Required in every flow. An access token issued for the database resource. |
| `EndUserToken` | `string` | The signed-in user's access token when an identity provider manages their identity. |
| `NamedUser` | `osql.NamedEndUser` | The name of a database-defined end user and an optional lookup key. |
| `DataRoles` | `[]string` | Optional. Additional data role names to request. Oracle Database determines which roles it can enable. |
| `Attributes` | `map[string]json.JSONObject` | Optional. Map each fully qualified end-user context name, such as `HR.EMPLOYEE_CONTEXT`, to an initialized `JSONObject` from `oracle/datatype/json`. |

The `NamedUser` field takes an `osql.NamedEndUser` with these fields.

| Field | Go type | What to supply |
| --- | --- | --- |
| `Username` | `string` | The database-defined end-user name associated with the authenticated application user. |
| `LookupKey` | `string` | Optional. Oracle Database uses this key to look up an existing end-user security context for the named user. A lookup key requires a username. |

Choose one of the following three combinations. Supply at most one of `EndUserToken` and `NamedUser`. [Learn more about End-User Security Context](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/end-user-security-context.html)

**Database-access token only.** Use this form when your database and authentication flow support deriving the identity from the database-access token. A token obtained through the client credentials flow represents the application. A token obtained through the on-behalf-of flow represents the end user.

```go
secCtx := osql.EndUserSecurityContext{
	DatabaseAccessToken: databaseToken,
	// ...
}
```

**Database-access token and end-user token.** Use this form when the user's access token supplies their identity.

```go
secCtx := osql.EndUserSecurityContext{
	DatabaseAccessToken: databaseToken,
	EndUserToken:        endUserToken,
	// ...
}
```

**Database-access token and username.** Use this form when your authenticated application user maps to an end user defined in the database.

```go
secCtx := osql.EndUserSecurityContext{
	DatabaseAccessToken: databaseToken,
	NamedUser:           osql.NamedEndUser{Username: username},
	// ...
}
```

### Use the context

Obtain a dedicated `*sql.Conn` as `conn` and set the end-user security context of as `secCtx`.

```go
defer conn.Close()

security, err := oracle.NewConnectionWrapper(conn)
if err != nil {
	return err
}

if err := security.SetEndUserSecurityContext(secCtx); err != nil {
	return err
}
// clear the end-user sec ctx before returning the conn to the pool
defer security.ClearEndUserSecurityContext()

// run the user's SQL through conn
```

After setting the context, you can use `conn` or a transaction started on it to run multiple statements for the same user as long as the context remains valid. If a token expires, set a fresh context before continuing.

## Supply the context through a provider

Implement `providers.DeepDataSecurityProvider` when the end-user identity varies by SQL operation. Register the provider on the connector before opening the pool. For example:

```go
connector, err := oracle.NewOracleConnector(cfg)
if err != nil {
	return err
}

registrar := connector.(providers.ProviderRegistrar)
registrar.RegisterProvider(provider)

db := sql.OpenDB(connector)
defer db.Close()
```

The driver calls `EndUserSecurityContext` before each execution that uses the provider. Return the authenticated user's security context with a current database-access token.

**Notes:**

- The provider receives the same Go `context.Context` passed to the SQL operation, such as `QueryContext` or `ExecContext`. 
- An explicit context takes precedence on its connection. Clearing it lets the provider supply the context for the next execution.


## Error codes

| Code | Meaning |
| --- | --- |
| `OGD-00400` | The database service does not support Deep Data Security. |
| `OGD-00401` | The connection does not use TLS. Connect using `TCPS`. |
| `OGD-00402` | The end-user security context is invalid. Check for a missing database-access token, an expired token, conflicting identity fields, a lookup key without a username, or an invalid attribute object. |

Check errors when setting the context and when executing SQL.

## References

- [Oracle Deep Data Security Guide](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/index.html)
- [End-User Security Context](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/end-user-security-context.html)
- [Fine-Grained Data Authorization](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/fine-grained-data-authorization.html)
