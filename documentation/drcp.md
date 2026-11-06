

# DRCP Support

This page describes the Go driver’s DRCP support.

References:
- [Using Database Resident Connection Pool](https://docs.oracle.com/en/database/oracle/oracle-database/26/adfns/performance-and-scalability.html#GUID-0B0DC596-17FF-4EEE-BAA1-9F428710C1AC)
- [Oracle Database Resident Connection Pooling](https://www.oracle.com/a/tech/docs/drcp-technical-brief.pdf)

DRCP (Database Resident Connection Pooling) is a connection pool built into Oracle Database.
It keeps a shared pool of database server processes and sessions that applications can use temporarily, then return to the pool when finished.
This helps applications reduce memory and process overhead and improves database scalability.
DRCP can also complement a connection pool in the application’s middle tier.

A client connects through Oracle’s connection broker. When it needs database access, the broker assigns an available pooled server, which handles the request.
When the client releases the connection, the server process returns to the pool for reuse. The session may remain intact but become temporarily disconnected from that server, allowing it to be reused according to the application’s configuration.

The connection class determines which requests can share pooled sessions. Purity indicates whether the client can reuse a session (SELF) or wants a new one (NEW).

## Configuration

DRCP relies on a few connection parameters, which are available in the driver configuration.

### Pool Configuration

```go
type DatabaseResidentConnectionPooling struct {
 ClassName         string // Sets the DRCP connection class.
 Purity            string // Sets the DRCP purity.
 PoolName          string // Sets the DRCP pool to be used.
}
```

**ClassName:** The DRCP connection class. The class name identifies a type of connection that can be shared across application processes or applications. This parameter is optional; if omitted, the default class, `_SYS.SHARED_`, is used.

**Purity:** Session purity. This attribute controls the reusability of pooled sessions in DRCP. It tells DRCP whether to provide a fresh session (NEW) or allow reuse of an existing pooled session (SELF).
Reusing a session can be more efficient, but applications should account for its session state. If ommitted the server use the default value "NEW"

**PoolName:** The name of the DRCP pool to use. If omitted, the default pool (`SYS_DEFAULT_CONNECTION_POOL`) is used.

Like other driver configuration parameters, these can also be specified in the connection string.

Note: The DRCP specification defines parameters that the Go driver does not use and therefore does not support:

- `Boundary`: Enables implicit attachment. The Go driver attaches and detaches connections automatically.
- `Tag`, `UseMultipleTag`, and `FixUpCallBackName`: Tagging is useful only with a client-side connection pool that supports it. Go’s `sql.DB` pool does not support tagging.


### Server Type Configuration

The client can specify the type of remote server process to use. The type can be `SHARED`, `DEDICATED`, or `POOLED`.

For DRCP to be used, the type must be set to `POOLED`.

Set the server type in `OracleConnectionProperties`:

```text
ServerType = POOLED | SHARED | DEDICATED
```

## How to enable DRCP?

To use DRCP, set the server type to `POOLED`. You can do this in any of the following ways:

- Set `OracleConnectionProperties.ServerType` in the driver configuration.
- Add `:pooled` to the EZConnect string, for example: `tcp://localhost:1521/freepdb1:pooled`.
- Set the server type in the connect descriptor:

  ```text
  (DESCRIPTION=
    (ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))
    (CONNECT_DATA=(SERVICE_NAME=freepdb1)(SERVER=POOLED))
  )
  ```
### Configuration examples

#### Using the driver configuration

```go
package main

import (
	"github.com/oracle/go-oracledb/v26/oracle"
	"github.com/oracle/go-oracledb/v26/internal/common"
)

func main() {
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "tcp://localhost:1521/freepdb1"
	cfg.Credentials.User = "myuser"
	cfg.Credentials.Password = "mypassword"

	// Enable DRCP and set the connection class and purity.
	cfg.ConnectionProperties.ServerType = common.ServerTypePooled
	cfg.ConnectionProperties.Drcp.ClassName = "Sales"
	cfg.ConnectionProperties.Drcp.Purity = common.DrcpPuritySelf
}
```

#### Using the DSN parameters

```go
dsn := fmt.Sprintf("myuser/mypassword@tcp://localhost:1521/freepdb1:pooled?pool_connection_class=%s&pool_purity=&s",
            "Sales",
            common.DrcpPuritySelf)
db := sql.OpenDB(dsn)
```

## Automatic Attachment and Detachment

The driver does not expose an attachment API; it attaches and detaches connections to and from the pool automatically.
