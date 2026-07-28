# Oracle Database Driver for Go

Oracle Database Driver for Go is a native Go driver for Go's [database/sql](https://pkg.go.dev/database/sql) package. It supports Oracle Database versions
19c and higher.

## Features
  - Native Go implementation of Go's [sql/driver](https://pkg.go.dev/database/sql/driver) package
  - Supports Oracle Database versions: 19c and higher
  - Authentication: supports username and password authentication
  - Data source: supports Connect Descriptor and EZConnect
  - Protocols: TCP and TCPS
  - Transactions
  - Statements with in parameters and out parameters (using `sql.Out`)
  - PL/SQL In/Out parameters (using `sql.Out`)
  - Inband notifications
  - JSON support returning JSON as `string`
  - BLOB support using prefetch and returning `[]byte`
  - CLOB support using prefetch and returning `string`

## Usage

For end-to-end examples, go to the examples subdirectory.

Oracle Database Driver for Go is an implementation of Go's database/sql/driver interface.
All you need to do, is to import the driver, and you can use the full database/sql API.

The driver name is "oracle-db" and the dataSourceName supports both easy connect and Connect Descriptor.

``` go
  db, err := sql.Open("oracle-db", "myuser/mypassword@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=my_host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=my_service_name)))")
  if err != nil {
    return nil, err
  }
  rows, err := db.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
  if err != nil {
    log.Fatal(err)
  }
  defer rows.Close()

  var val int
  if rows.Next() {
    if err := rows.Scan(&val); err != nil {
      log.Fatal(err)
    }
  }
```

### Data Source Name as a string

When specified as a string, the format of the Data Source Name is:

```shell
[username/password@]<connection_string>[?<query_string>]
```
The "connection string" format can be either "EZConnect":

```
myuser/mypassword@tcps://my_host:1521/my_service_name?transport_connect_timeout=10
```

Or TNS Connect Descriptor:

_Note that query parameters are not supported when using the TNS format._
```
myuser/mypassword@(DESCRIPTION=(ADDRESS=(HOST=my_host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=my_service_name)))
```


### Driver configuration

#### Public API contract

Application code should import and depend on `github.com/oracle/go-driver/oracle`.
Packages under `github.com/oracle/go-driver/driver/...` are implementation details
and may change without compatibility guarantees.

The driver supports several sources of configuration. They apply in the following order of precedence:

1. Properties in the Data Source Name.
2. Environment variables.
3. CLI flags.
4. Configuration set in the Oracle Connector.

The list of supported configuration items are the fields of the `oracle.OracleDriverConfig` struct.
This type can be used to create a new Oracle [connector](https://pkg.go.dev/database/sql/driver#Connector).
All Easy connect parameters are supported by `oracle.OracleConnectionProperties`.

#### Configuration item naming

Configuration item names are prefixed with "oracle.go", e.g. "oracle.go.connectDescriptor".
When specified as an environment variable, dots are replaced by underscores and the name is made
in uppercase. As an example, the property oracle.foo.bar maps to the ORACLE_FOO_BAR environment variable.

There is a direct mapping of `oracle.OracleDriverConfig` fields and nested structs' fields to configuration properties and vice versa.

As an example let's look at connection descriptor:

``` go
type OracleDriverConfig struct {
  //...
  ConnectDescriptor      string 
  //...
}
```

This configuration field maps:
1. As a CLI flag, e.g. -oracle.go.ConnectDescriptor="../".
2. As an environment variable, e.g.  ORACLE_GO_CONNECTDESCRIPTOR="...".

Another example using failover connection property:
``` go
connectorConfig := oracle.NewOracleDriverConfig()
connectorConfig.ConnectionProperties.Failover=true
```

This configuration field maps:
1. As a CLI flag, e.g. -oracle.go.DriverProperties.Failover_="true".
2. As an environment variable, e.g.  ORACLE_GO_DRIVERPROPERTIES_FAILOVER_="true".
3. As a query parameter, e.g. oracle.go.DriverProperties.Failover_="true"

###### Easy connect plus query parameters

For compatibility with other Oracle drivers, the Easy connect parameters name set in the Data Source Name query parameters
are the ones listed here https://docs.oracle.com/en/database/oracle/oracle-database/26/netag/support-easy-connect-plus.html
As an example, the failover property _oracle.go.ConnectionProperties.Failover_ when set as query parameter. translates to 
myuser/mypassword@//my_host:1521/my_service_name?failover=true.

##### Display configuration

The `oracle.OracleDriverConfig` struct implements the go Stringer interface.

```go
 conf := oracle.NewOracleDriverConfig()
 fmt.Printf("Oracle configuration: [%v]", conf)
```

The _-oracle-db-config-help_ flag can also be set in the application, in that case, all available configuration flags are displayed on STDOUT.

#### Configurations types

The driver uses the following Golang types for configuration: `oracle.OracleDriverConfig`,
`oracle.OracleLoggingConfig`, `oracle.OracleCredentials`, `oracle.OracleNLSParameters`,
`oracle.OracleDriverProperties`, and `oracle.OracleConnectionProperties`.
To instantiate such objects the *oracle.NewOracleDriverConfig()* and *oracle.NewOracleLoggingConfig()* functions **must** be used.
They are the only supported way to get new references.
This method ensures that default values and proper initialization is performed on the struct before the caller can use it.

The `oracle.OracleDriverConfig` type has the _Validate()_ method which validates that all fields
and all nested type fields are set with valid values.

#### Programmatic configuration interfaces

##### Connector Configuration

When specific configuration is required, the _sql.OpenDB_ can be used. The required connector is then created with the
_oracle.NewOracleDriverConfig_() API. Once the `oracle.OracleDriverConfig` instance is created and populated with custom
values, the Validate method will be called before creating connectors.
Note that when this API is used, the credentials must be provided using Credentials struct. 

``` go
  connectorConfig := oracle.NewOracleDriverConfig()
  connectorConfig.ConnectDescriptor = "(description=(address=(protocol=tcps)(host=127.0.0.1)(port=1521))(connect_data=(service_name=freepdb1)))"
  connectorConfig.Credentials.User = "scott"
  connectorConfig.Credentials.Password = "tiger"

  // Optional validation operation. 
  err  := connectorConfig.Validate()
	if err != nil {
		// ...
	}

  connector, err := oracle.NewOracleConnector(connectorConfig)
  if err != nil {
	// ...
  }
  db := sql.OpenDB(connector)
  rows, err := db.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
  if err != nil {
	// ...
  }
  defer rows.Close()
  // ... 

```
##### Logging Configuration

The driver logging is configured by using the _ApplyDriverLoggingConfig_ API. The configuration items are set by assigning an `oracle.OracleLoggingConfig`
struct. The configuration items are:
1. Level: The logging level as a string. Should be one of the slog.Level.
2. Destination: The destination of the logging, it can be a file path, "STDOUT", "STDERR" or "NULL".
3. IncludeSensitive: Is sensitive information allowed in the logs ?
4. Truncate: Does the driver truncate the file at startup ? 

The _ApplyDriverLoggingConfig_ can be call more than once.

Example :
``` go
    loggingConfig := oracle.NewOracleLoggingConfig()
    loggingConfig.Destination = "STDOUT"
    loggingConfig.Level = "DEBUG"
    oracle.GetDefaultDriver().ApplyDriverLoggingConfig(loggingConfig)
```
Example using flags:
``` shell
    /bin/go -oracle.go.logging.Level=DEBUG ....
```


##### Environment variables

Besides driver properties, here is the list of environment variables that can be set.

- ORACLE_GO_DRIVER_DEBUG_PACKETS activates dumps of exchanged packets (requires sensitive logging parameter to be enabled)

### Errors
Errors are returned as `oracle.SQLError` which implements Go's `Error` interface and adds an `ErrorCode() string` function that allows to retrieve the error code which will be either "ORA-XXXXX" for Oracle Database errors, or "OGD-XXXXX" for driver errors.

``` go
  db, err := sql.Open("oracle-db", "myuser/mypassword@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=my_host)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=my_service_name)))")
  if err != nil {
    if sqlError, ok := err.(oracle.SQLError); ok {
      if sqlError.ErrorCode() == string(oracle.InvalidCredential) {
        log.Fatal("Invalid username or password")
      }
    }
  }
```

## Data-type support

### Character and Text Types

| Oracle Type  | Driver returns       |
|--------------|----------------------|
| `CHAR`       | `string`             |
| `NCHAR`      | `string`             |
| `VARCHAR2`   | `string`             |
| `NVARCHAR2`  | `string`             |
| `LONG`       | `string` or `[]byte` |
| `CLOB`       | `string` or `[]byte` |
| `NCLOB`      | `string`             |
| `XMLTYPE`    | `string`             |


### Numeric Types

| Oracle NUMBER            | Driver returns        |
|--------------------------|-----------------------|
| `NUMBER(p,0)`            | `int64`               |
| `NUMBER(p,s)`            | `string`              |
| `NUMBER` (unknown scale) | `string`              |
| `FLOAT`                  | `float64`             |
| `BINARY_FLOAT`           | `float32` → `float64` |
| `BINARY_DOUBLE`          | `float64`             |

> **Important**  
> Mapping `NUMBER` to `float64` by default is discouraged due to precision loss.  
> Returning `string` allows users to choose the correct numeric representation.

---

### Date and Time Types

| Oracle Type                      | Driver returns |
|----------------------------------|----------------|
| `DATE`                           | `time.Time`    |
| `TIMESTAMP`                      | `time.Time`    |
| `TIMESTAMP WITH TIME ZONE`       | `time.Time`    |
| `TIMESTAMP WITH LOCAL TIME ZONE` | `time.Time`    |
| `INTERVAL YEAR TO MONTH`         | `string`       |
| `INTERVAL DAY TO SECOND`         | `string`       |

---

### Binary and RAW Types

| Oracle Type | Driver returns |
|-------------|----------------|
| `RAW`       | `[]byte`       |
| `LONG RAW`  | `[]byte`       |
| `BLOB`      | `[]byte`       |
| `BFILE`     | `[]byte`       |

---

### Boolean Types

| Oracle Type        | Driver returns |
|--------------------|----------------|
| `BOOLEAN` (PL/SQL) | `bool`         |

---

### ROWID Types

| Oracle Type | Driver returns |
|-------------|----------------|
| `ROWID`     | `string`       |
| `UROWID`    | `string`       |

---

### Advanced and Complex Types

| Oracle Type   | Driver returns       |
|---------------|----------------------|
| `JSON` (21c+) | `string`             |

## Contributing
see [CONTRIBUTING](CONTRIBUTING.md)
