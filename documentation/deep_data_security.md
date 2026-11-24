# Deep Data Security

Oracle Deep Data Security is a database-enforced authorization framework for
enterprise applications, analytics platforms, and AI agents. It lets applications
define access policies in the database and enforce them at the row, column, and
cell levels. Access decisions use the end user's identity, roles, and attributes.
See [What Is Oracle Deep Data Security](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/what-is-oracle-deep-data-security.html)
for an overview of the authorization model.


The Go driver passes the end-user security context to the database when executing
queries and statements. Your application obtains the tokens and selects the
end-user identity.

## Requirements

- A Deep Data Security-enabled database with the required identities, data roles,
  data grants, and privileges to establish end-user security contexts.
- A TCPS connection and database credentials.
- A database-access token and, for token-based end-user identities, an end-user
  token. The application is responsible for obtaining and renewing tokens.

See the [Oracle Deep Data Security Guide](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/)
for database and identity-provider configuration requirements.

## Choose how to supply the context

The Go driver supports two ways to supply a context:

- **Connection context**: set a context on a dedicated `*sql.Conn` before running
  operations for an end user.
- **Provider**: register an implementation that supplies a context for each
  query or statement execution.

An explicitly set connection context takes precedence over a registered provider.
While it is selected, the driver uses it for queries and statement executions
without calling the provider. Clearing the explicit context allows the provider
to supply a context for subsequent executions.

## Create a security context

Use `oracle/sql.EndUserSecurityContext` to describe the context. The
`DatabaseAccessToken` is required; the remaining fields are optional.

| Field | Description |
| --- | --- |
| `DatabaseAccessToken` | Token authorizing the application to access the database |
| `EndUserToken` | Token identifying an end user |
| `NamedUser` | A database-managed end user, identified by username and optionally a lookup key |
| `DataRoles` | Additional application-managed data roles to enable; each name must be nonempty |
| `Attributes` | JSON objects keyed by fully qualified end-user context names |

For example, use an end-user token:

```go
secCtx := osql.EndUserSecurityContext{
    DatabaseAccessToken: dbToken,
    EndUserToken:        userToken,
}
```

Or identify a database-managed end user by username:

```go
secCtx := osql.EndUserSecurityContext{
    DatabaseAccessToken: dbToken,
    NamedUser: osql.NamedEndUser{
        Username: "DDS_EMPLOYEE",
    },
}
```

Supply either `EndUserToken` or `NamedUser`, not both. A lookup key
requires a username. You can also supply only a database-access token and the
database determines whether the resulting context is authorized.

## Set a context on a connection

Acquire a dedicated connection from `database/sql`, wrap it, and set the context.
Use the same connection for the operations that need that context:

```go
conn, err := db.Conn(ctx)
if err != nil {
    return err
}
defer conn.Close()

security, err := oracle.NewConnectionWrapper(conn)
if err != nil {
    return err
}
if err := security.SetEndUserSecurityContext(secCtx); err != nil {
    return err
}
defer security.ClearEndUserSecurityContext()

rows, err := conn.QueryContext(ctx, "SELECT ORA_END_USER_CONTEXT.username FROM dual")
if err != nil {
    return err
}
defer rows.Close()

for rows.Next() {
    var name string
    if err := rows.Scan(&name); err != nil {
        return err
    }
    fmt.Println(name)
}
return rows.Err()
```

## Use a provider

Implement `providers.DeepDataSecurityProvider` when the application needs to
supply a context for each operation, for example to obtain or refresh tokens.
The provider receives the operation's `context.Context`. Returning an error prevents the operation.

```go
import (
    "context"
    "database/sql"
    "database/sql/driver"
    "fmt"

    "github.com/oracle/go-oracledb/v26/oracle/providers"
    osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

type securityProvider struct {
    getTokens func(context.Context) (dbToken, userToken string, err error)
}

func (p *securityProvider) EndUserSecurityContext(ctx context.Context) (osql.EndUserSecurityContext, error) {
    dbToken, userToken, err := p.getTokens(ctx)
    if err != nil {
        return osql.EndUserSecurityContext{}, err
    }
    return osql.EndUserSecurityContext{
        DatabaseAccessToken: dbToken,
        EndUserToken:        userToken,
    }, nil
}

func openWithSecurityProvider(connector driver.Connector, provider *securityProvider) (*sql.DB, error) {
    registrar, ok := connector.(providers.ProviderRegistrar)
    if !ok {
        return nil, fmt.Errorf("connector does not support providers")
    }
    registrar.RegisterProvider(provider)
    return sql.OpenDB(connector), nil
}
```

Create the connector with `oracle.NewOracleConnector` and register the provider. A
provider must return a nonempty database-access token. An explicitly selected
connection context takes precedence until cleared.

## Add data roles and attributes

Use `DataRoles` to request additional data roles. The database determines whether
the roles are valid for the selected identity:

```go
secCtx.DataRoles = []string{"APP_READER"}
```

Use `Attributes` to supply initialized JSON objects for end-user contexts
configured in the database. The map key is the fully qualified context name,
for example `APP.CONTEXT_NAME`:

```go
doc, err := ojson.NewJSON(map[string]any{"region": "west"})
if err != nil {
    return err
}
attributes, err := doc.AsJSONObject()
if err != nil {
    return err
}
secCtx.Attributes = map[string]ojson.JSONObject{
    "APP.CONTEXT_NAME": attributes,
}
```

See the [JSON guide](json.md) for constructing JSON values.


## References

- [Oracle Deep Data Security Guide](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/): database configuration, authorization concepts, and administration.
- [Deep Data Security example](../examples/deep_data_security/README.md): database setup and Go examples for named and token-based end users.
