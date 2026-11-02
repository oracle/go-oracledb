

# DRCP Support

API for DRCP support in the Go driver.

Reference: 
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

DRCP relies on a few connection parameters. These parameters are available as part of the driver configuration.

### Pool Configuration

```go
type DatabaseResidentConnectionPooling struct {
 ClassName         string // sets the DRCP connection class, this is optional when not specified default SYS.SHARED is used
 Purity            string // sets the DRCP purity
 PoolName          string // Name of the DRCP pool to be used.
}
```


**PoolName:** The name of the DRCP pool to be used. The default pool (SYS_DEFAULT_CONNECTION_POOL) is used when no pool name is specified.

**ClassName:** The DRCP connection class. The connection class defines a logical name for the type of connection an application wants to use and share across multiple application processes or other applications.

**Purity:** Session purity. This attribute controls the reusability of pooled sessions in DRCP. It tells DRCP whether to provide a fresh session (NEW) or allow reuse of an existing pooled session (SELF).
Reusing a session can be more efficient, but applications should account for its session state.

Note: The DRCP specification includes the following parameters, which have no usage in the go driver. As such, they are not supported.

_Boundary_:  This activates implicit attachment. The driver automatically attach and detach
connection

_Tag_, _UseMultipleTag_ and _FixUpCallBackName_ The use of tagging makes sense only
when this is coupled with a client-side pooling tagging mechanism. The sql.DB pool do not have tagging capability.


### Server Type Configuration

The client can specify the type of remote server process to be used. The type can be shared, dedicated, or pooled.

For DRCP to be used, the type must be set to `POOLED`.

The property is within `OracleConnectionProperties`:

```text
ServerType = POOLED | SHARED | DEDICATED
```

## APIs

### Pool Enablement

In order to enable the use of DRCP the server type set in `OracleConnectionProperties.ServerType` must
be set to `POOLED`.

### Attachment Points

The connection must be attached to the pool in order to perform operations and detached to release remote
resources when the connection becomes idle. 

There are two scenarios: when the connection is used directly by the client application and when it is used implicitly by the `sql.DB` object.

This is when the connection is handled by the Go connection pool, or when `ExecContext` and `QueryContext` are executed against the `sql.DB` object.

In such situations, the driver attaches the connection automatically when the connection is used and detaches it when it returned to the pool.

