# OCI IAM token authentication example

This directory contains a runnable Go sample that connects using OCI IAM token authentication.

## 1. Prerequisites

You need:

- an Autonomous Database
- an OCI IAM user that should be allowed to connect
- OCI CLI installed and authenticated
- ADMIN access to the target database

You also need these identifiers:

- tenancy OCID
- compartment OCID
- Autonomous Database OCID
- IAM user name or IAM user OCID

## 2. Configure the database for OCI IAM authentication

For Autonomous Database, the database-side setup has three parts:

- create or identify an IAM group for database access
- create an IAM policy that allows the group to connect to the database
- enable `OCI_IAM` as the external authentication provider in the database, and map IAM identities to database users

At a high level:

1. Create or identify an IAM group for database users.
2. Add the IAM user to that group.
3. Create a policy that allows the group to connect to the target Autonomous Database.
4. In the database, enable external authentication with `type => 'OCI_IAM'`.
5. Map the IAM principal to a global database user.

The IAM user must be in a group that is allowed to connect to the Autonomous Database.

A narrow database-scoped policy looks like this:

```text
Allow group ADB_IAM_DB_USERS to use autonomous-database-family in compartment id <COMPARTMENT_OCID> where target.id = '<AUTONOMOUS_DATABASE_OCID>'
Allow group ADB_IAM_DB_USERS to use database-connections in compartment id <COMPARTMENT_OCID> where target.id = '<AUTONOMOUS_DATABASE_OCID>'
```

You can widen the scope to the whole compartment or tenancy if that is what your environment needs, but database scope is the safest default.

As `ADMIN`, enable OCI IAM authentication in the database:

```sql
BEGIN
  DBMS_CLOUD_ADMIN.ENABLE_EXTERNAL_AUTHENTICATION(
    type => 'OCI_IAM'
  );
END;
/
```

If another external authentication provider is already enabled and you intentionally want to replace it:

```sql
BEGIN
  DBMS_CLOUD_ADMIN.ENABLE_EXTERNAL_AUTHENTICATION(
    type  => 'OCI_IAM',
    force => TRUE
  );
END;
/
```

You can verify the database setting with:

```sql
SELECT name, value
FROM v$parameter
WHERE name = 'identity_provider_type';
```

Expected value:

```text
OCI_IAM
```

After the provider is enabled, map IAM identities to the database.

Two common patterns are:

- direct mapping of an IAM user to a global schema
- shared mapping of an IAM group to a global schema

Example direct mapping:

```sql
CREATE USER <database user> IDENTIFIED GLOBALLY AS 'IAM_PRINCIPAL_NAME=<principal>';
GRANT CREATE SESSION TO <database user>;
```

If the user already exists:

```sql
ALTER USER <database user> IDENTIFIED GLOBALLY AS 'IAM_PRINCIPAL_NAME=<principal>';
GRANT CREATE SESSION TO <database user>;
```

The `<principal>` value is usually one of:

- `<user>`
- `<domain>/<user>` for non-default identity domains
- `<tenancy_ocid>:<domain>/<user>` for cross-tenancy cases

Example shared mapping through an IAM group:

```sql
CREATE USER <database user> IDENTIFIED GLOBALLY AS 'IAM_GROUP_NAME=<group>';
GRANT CREATE SESSION TO <database user>;
```

## 3. Generate a token using OCI CLI

Use OCI CLI to generate a fresh database token bundle:

```bash
oci iam db-token get --db-token-location "$HOME/.oci/db-token"
```

This command writes the token bundle to the target directory. The Go driver expects that directory to contain:

- `token`
- `oci_db_key.pem`

If you want to limit the token to a specific database, compartment, or tenancy, pass `--scope`. For example:

```bash
oci iam db-token get \
  --db-token-location "$HOME/.oci/db-token" \
  --scope "urn:oracle:db::id::<COMPARTMENT_OR_DATABASE_SCOPE>"
```

The token is short-lived. Regenerate it when it expires.

## 4. Put the token bundle on disk

The sample registers a token provider on the connector. That provider reads the OCI IAM token bundle from the directory in `ORACLE_GO_OCI_TOKEN_LOCATION`.

The OCI CLI default bundle directory is:

```text
$HOME/.oci/db-token
```

In that directory the expected files are:

- `token`
- `oci_db_key.pem`

## 5. Run the Go sample

The sample reads its configuration from these environment variables:

- `ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR`: the TCPS connect descriptor for the target database
- `ORACLE_GO_OCI_TOKEN_LOCATION`: the OCI IAM token bundle directory containing `token` and `oci_db_key.pem`

Set them before running [main.go](C:/work/driver/go-driver/go-oracledb/examples/token_authentication/oci_token/main.go).

```bash
export ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR="(description=(address=(protocol=tcps)(port=1522)(host=<adb-host>))(connect_data=(service_name=<service-name>))(security=(ssl_server_dn_match=yes)))"
export ORACLE_GO_OCI_TOKEN_LOCATION="$HOME/.oci/db-token"
go run ./examples/token_authentication/oci_token
```

If the sample connects successfully, it prints the connected database user:

```text
Username: <database user>
```

## 6. Common checks when login fails

- Confirm that the IAM user is in the expected IAM group.
- Confirm that the policy includes `database-connections`.
- Confirm that `identity_provider_type` is `OCI_IAM`.
- Confirm that the database user is mapped with the correct `IAM_PRINCIPAL_NAME` or `IAM_GROUP_NAME`.
- Confirm that `ORACLE_GO_OCI_TOKEN_LOCATION` points to a directory containing a fresh `token` file and `oci_db_key.pem`.
- Confirm that the connect descriptor uses TCPS and the correct Autonomous Database service name.

## References

- OCI CLI `oci iam db-token get`: <https://docs.oracle.com/en-us/iaas/tools/oci-cli/latest/oci_cli_docs/cmdref/iam/db-token/get.html>
- Enable IAM authentication on Autonomous Database: <https://docs.oracle.com/en-us/iaas/autonomous-database-shared/doc/enable-iam-authentication.html>
- `DBMS_CLOUD_ADMIN.ENABLE_EXTERNAL_AUTHENTICATION`: <https://docs.oracle.com/en-us/iaas/autonomous-database-serverless/doc/dbms-cloud-admin.html>
