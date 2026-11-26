# DRCP example

This example shows how to use Oracle Database Resident Connection Pooling (DRCP) with the Go driver. 

## Prerequisites

- An Oracle Database service configured to use DRCP.
- A database username and password with permission to connect to that service.

### Enable DRCP on the remote server

At least one DRCP pool should be started on the remote server.

The default DRCP pool (SYS_DEFAULT_CONNECTION_POOL) can be started with the following commands.

```sql
sqlplus /nolog
SQL> connect / as sysdba
SQL> execute dbms_connection_pool.start_pool()
```

The customPool example uses a user-defined pool named MYPOOL. To configure and start the pool, connect to the remote database as SYSDBA and run the following commands:

```sql
sqlplus /nolog
SQL> connect / as sysdba
SQL> execute dbms_connection_pool.add_pool('MYPOOL')
SQL> execute dbms_connection_pool.configure_pool(pool_name => 'MYPOOL',minsize => 4,maxsize => 40,incrsize => 2,session_cached_cursors => 20,inactivity_timeout => 300,max_think_time => 600,max_use_session => 500000,max_lifetime_session => 86400)
SQL> execute dbms_connection_pool.start_pool('MYPOOL')
```

## Configure the connection

The following environment variables must be set accordingly:

- `ORACLE_DSN`: an Oracle connect descriptor or Easy Connect string for the DRCP-enabled service.
- `ORACLE_USER`: the database username.
- `ORACLE_PASSWORD`: the database password.

For example, on Linux platforms:

```sh
# The ':pooled' suffix must be added to the DSN to activate DRCP.
export ORACLE_DSN='tcps://127.0.0.1:1521/freepdb1:pooled'
export ORACLE_USER='myuser'
export ORACLE_PASSWORD='mypassword'
```

## Run the example

From this directory, run:

```sh
go run .
```

## What does the program do?

The program creates a driver configuration with DRCP enabled. It also sets the DRCP class to `Sales`; change this value if your application uses a different class.

It then runs two examples:

- **Basic usage:** Creates a connector, opens a `database/sql` pool with `sql.OpenDB`, and queries the remote server using both `sql.DB` and `sql.Conn` APIs.
- **Custom pool:** Sets the DRCP pool name to `MYPOOL` and obtains a connection from that pool.


### Monitor DRCP pool activity

The following views can be used to see current DRCP statistics.

```sql
SELECT cclass_name, pool_name, purity, tag, connection_status FROM v$cpool_conn_info;
SELECT pool_name, num_requests FROM V$CPOOL_STATS;
```

## References

- [DRCP support](../documentation/drcp.md)

