# Authentication guide

The Oracle Database Driver for Go supports the following authentication methods:

| Method | Credentials supplied by the application |
| --- | --- |
| Username and password | Database username and password |
| Username and password with proxy | `proxyUser[clientUser]` and the proxy user's password |
| OAuth token | A provider implementing `TokenAuthenticationProvider` |
| OCI IAM signed token | The provided OCI IAM provider |

Applications use `database/sql` and the public packages under `github.com/oracle/go-oracledb/v26/oracle`.

## 1. Username and password

Use `oracle.NewOracleDriverConfig` to configure credentials separately from the connection descriptor. This avoids putting credentials into the connection string.

Set the following environment variables for this example:

```sh
export DB_CONNECT_DESCRIPTOR='(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=db.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=myservice))(SECURITY=(SSL_SERVER_DN_MATCH=YES)))'
export DB_USER='app_user'
export DB_PASSWORD='<database-password>'
```

Replace the host, port, service, and credentials with your database settings. Use the TCPS descriptor and certificate configuration supplied for your database. A database configured for TCP can use a TCP descriptor for password authentication.

The environment variable names in this guide are read by the example programs; they are not additional driver configuration properties.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = os.Getenv("DB_CONNECT_DESCRIPTOR")
	cfg.Credentials.User = os.Getenv("DB_USER")
	cfg.Credentials.Password = os.Getenv("DB_PASSWORD")
	if cfg.ConnectDescriptor == "" || cfg.Credentials.User == "" || cfg.Credentials.Password == "" {
		return fmt.Errorf("set DB_CONNECT_DESCRIPTOR, DB_USER, and DB_PASSWORD")
	}

	connector, err := oracle.NewOracleConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	var user string
	if err := db.QueryRowContext(ctx,
		"SELECT SYS_CONTEXT('USERENV', 'SESSION_USER') FROM DUAL").Scan(&user); err != nil {
		return err
	}
	fmt.Println("Session user:", user)
	return nil
}
```

`sql.OpenDB` creates a connection pool. `PingContext` establishes a connection and reports authentication errors before the first application query.

### Using a DSN

Password authentication also accepts a data source name in this format:

```text
username/password@connection_string
```

For example, inside an application that imports the driver:

```go
db, err := sql.Open("oracledb", "app_user/example_password@tcp://localhost:1521/freepdb1")
if err != nil {
	return err
}
defer db.Close()
if err := db.PingContext(ctx); err != nil {
	return err
}
```

If the application only uses `sql.Open`, register the driver with a blank import:

```go
import _ "github.com/oracle/go-oracledb/v26/oracle"
```

Use the connector configuration when credentials contain connection-string delimiters, or when you want to keep credentials separate from the descriptor.

### Administrative logon

For an account authorized to connect in an administrative mode, set the mode before creating the connector:

```go
cfg.Credentials.User = "sys"
cfg.Credentials.Password = os.Getenv("DB_PASSWORD")
cfg.Credentials.LogonMode = "SYSDBA"
```

The account must have the corresponding database privilege.

## 2. Username and password with proxy authentication

A proxy account can establish a session on behalf of another database user. Supply the username as:

```text
proxyUser[clientUser]
```

Use the **proxy user's password**. The session runs as the client user.

### Database setup

For existing accounts `APP_PROXY` and `APP_CLIENT`, have a database administrator authorize the connection:

```sql
GRANT CREATE SESSION TO APP_PROXY;
GRANT CREATE SESSION TO APP_CLIENT;
ALTER USER APP_CLIENT GRANT CONNECT THROUGH APP_PROXY;
```

Grant application privileges to the client account as required by the application.

### Connect as the client

Run the username/password program above with:

```sh
export DB_USER='APP_PROXY[APP_CLIENT]'
export DB_PASSWORD='<proxy-user-password>'
```

Equivalently, set the fields directly before creating the connector:

```go
cfg.Credentials.User = "APP_PROXY[APP_CLIENT]"
cfg.Credentials.Password = os.Getenv("DB_PASSWORD")
```

The corresponding DSN form is:

```text
APP_PROXY[APP_CLIENT]/proxy_password@tcp://localhost:1521/freepdb1
```

### Quoted identifiers

Either identifier may be double quoted. In Go, raw string literals preserve those quotes:

```go
cfg.Credentials.User = `"APP_PROXY"[APP_CLIENT]`
cfg.Credentials.User = `APP_PROXY["App Client"]`
cfg.Credentials.User = `"App Proxy"["App Client"]`
```

These are alternative assignments. Use the spelling and case of the database accounts you created. Quoting the whole value, such as `"APP_PROXY[APP_CLIENT]"`, represents one username containing brackets, rather than a proxy login.

### Check the session identity

After connecting, run:

```sql
SELECT SYS_CONTEXT('USERENV', 'SESSION_USER') AS session_user,
       SYS_CONTEXT('USERENV', 'PROXY_USER') AS proxy_user
FROM DUAL;
```

For the unquoted example, the expected values are `APP_CLIENT` and `APP_PROXY`. A normal password session has a null `PROXY_USER`.

## 3. OAuth token authentication

Use a database configured to trust your identity provider, a mapped database identity, a valid database access token, and a TCPS connection descriptor.

Register a `providers.TokenAuthenticationProvider` on the connector before opening connections. Leave `Credentials.User` and `Credentials.Password` empty for token authentication. The database determines the session identity from the token and its configured identity mapping.

The following complete program reads a token from `DB_TOKEN_FILE` for each authentication attempt. It uses `DB_CONNECT_DESCRIPTOR` as in the password example.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
)

type fileTokenProvider struct {
	path string
}

func (p fileTokenProvider) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	descriptor := os.Getenv("DB_CONNECT_DESCRIPTOR")
	tokenPath := os.Getenv("DB_TOKEN_FILE")
	if descriptor == "" || tokenPath == "" {
		return fmt.Errorf("set DB_CONNECT_DESCRIPTOR and DB_TOKEN_FILE")
	}
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = descriptor
	connector, err := oracle.NewOracleConnector(cfg)
	if err != nil {
		return err
	}
	registrar, ok := connector.(providers.ProviderRegistrar)
	if !ok {
		return fmt.Errorf("connector does not support provider registration")
	}
	registrar.RegisterProvider(fileTokenProvider{path: tokenPath})

	db := sql.OpenDB(connector)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	var user string
	if err := db.QueryRowContext(ctx,
		"SELECT SYS_CONTEXT('USERENV', 'SESSION_USER') FROM DUAL").Scan(&user); err != nil {
		return err
	}
	fmt.Println("Session user:", user)
	return nil
}
```

Set `DB_TOKEN_FILE` to a file containing only the access token text:

```sh
export DB_TOKEN_FILE='/path/to/database-access-token'
```

The driver calls the provider during authentication; it does not acquire or refresh tokens from the identity provider. For this example, replace the token file with a fresh token before it expires. A production provider can obtain or refresh tokens itself and should support concurrent calls from the connection pool.

See the [OAuth example](../examples/token_authentication/oauth/README.md) for identity-provider setup and a runnable sample using its own `ORACLE_GO_OAUTH_*` environment variables.

## 4. OCI IAM signed token authentication

OCI IAM authentication uses a database token and its associated private key. Configure the database for OCI IAM and map the IAM identity to a database user before connecting over TCPS.

Use the provided [OCI IAM authentication provider](https://github.com/oracle/go-oracledb/tree/main/providers/oci). See its documentation for setup and usage.

## Authentication selection and troubleshooting

- Supplying both username and password selects password authentication, even if a token provider is registered. Leave both empty when using tokens.
- Register token providers before calling `PingContext` or issuing queries. New physical connections in the pool authenticate through the provider.
- For proxy authentication, check the `CONNECT THROUGH` grant and supply the proxy account's password. Use the session-identity query above to confirm both identities.
- For token authentication, check token expiry, the database identity mapping, and the TCPS descriptor. Signed tokens also require the matching private key.
- Token examples use database identity mapping. The `proxyUser[clientUser]` examples in this guide apply to password authentication.

For connection configuration beyond authentication, see the [driver README](../README.md#driver-configuration).
