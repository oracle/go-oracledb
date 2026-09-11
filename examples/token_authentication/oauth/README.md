# OAuth token authentication example

This directory contains a runnable Go sample that connects using generic OAuth token authentication, such as a Microsoft Entra ID database access token.

## 1. Prerequisites

You need:

- a database configured to trust your OAuth identity provider
- a user, group, or app-role mapping in the database
- a valid database access token from the identity provider
- a TCPS connect descriptor for the target database

For Microsoft Entra ID on Autonomous Database, the database must be configured with external authentication using `AZURE_AD`, and the Entra principal must be mapped to a database user or role.

## 2. Configure the database for OAuth using Microsoft Entra ID

For Autonomous Database, the database-side setup has two parts:

- register and configure the database application in Microsoft Entra ID
- enable `AZURE_AD` as the external authentication provider in the database, and map Entra identities to database users or roles

At a high level:

1. In Microsoft Entra ID, register the database as an application.
2. Create the app roles you want to use for database authorization.
3. Assign users, groups, or applications to those app roles.
4. In the database, enable external authentication with `type => 'AZURE_AD'`.
5. Map the Entra user or app role to a global database user or global role.

As `ADMIN`, enable Microsoft Entra ID on the database:

```sql
BEGIN
  DBMS_CLOUD_ADMIN.ENABLE_EXTERNAL_AUTHENTICATION(
      type   => 'AZURE_AD',
      params => JSON_OBJECT(
                  'tenant_id' VALUE '<tenant-id>',
                  'application_id' VALUE '<application-id>',
                  'application_id_uri' VALUE '<application-id-uri>'),
      force  => TRUE
  );
END;
/
```

The required values come from the Entra app registration:

- `tenant_id`: the Entra tenant ID
- `application_id`: the application or client ID of the database app registration
- `application_id_uri`: the Application ID URI for that app registration

You can verify the database setting with:

```sql
SELECT name, value
FROM v$parameter
WHERE name = 'identity_provider_type';
```

Expected value:

```text
AZURE_AD
```

After the provider is enabled, map Entra identities to the database.

Two common patterns are:

- direct mapping of an Entra user to a global schema
- shared mapping of an Entra app role to a global schema or global role

Example direct mapping:

```sql
CREATE USER <database user> IDENTIFIED GLOBALLY AS 'AZURE_USER=<entra-user>';
GRANT CREATE SESSION TO <database user>;
```

Example shared mapping through an app role:

```sql
CREATE USER <database user> IDENTIFIED GLOBALLY AS 'AZURE_ROLE=<app-role-name>';
GRANT CREATE SESSION TO <database user>;
```

The exact global mapping string depends on your Entra configuration and on whether you are mapping a user, group, or app role. Use the mapping pattern documented for your configuration and token claims.

## 3. Generate a token using Microsoft Entra ID

There are several supported Entra flows. For a quick example, you can use Azure CLI to fetch a token for a signed-in user.

First, sign in:

```bash
az login
```

Then request a token for the database application. For v1-style resource-based requests:

```bash
az account get-access-token \
  --resource "<application-id-uri>" \
  --tenant "<tenant-id>"
```

If your application is configured for v2 scopes, request a scope instead:

```bash
az account get-access-token \
  --scope "<application-id-uri>/.default" \
  --tenant "<tenant-id>"
```

The command returns JSON. Write the `accessToken` field either to a file or to a file named `token` in a directory that you will pass to the Go driver.

Example:

```bash
mkdir -p "$HOME/tokens/oauth"
az account get-access-token \
  --resource "<application-id-uri>" \
  --tenant "<tenant-id>" \
  --query accessToken \
  --output tsv > "$HOME/tokens/oauth/token"
```

The token is short-lived. Regenerate it when it expires.

## 4. Put the token on disk

The sample registers a token provider on the connector. That provider reads the access token from the file path in `ORACLE_GO_OAUTH_TOKEN_FILE`.

Example token file contents:

```text
eyJ0eXAiOiJKV1QiLCJhbGciOiJ...
```

Store the token as a single line of UTF-8 text.

## 5. Run the Go sample

The sample reads its configuration from these environment variables:

- `ORACLE_GO_OAUTH_CONNECT_DESCRIPTOR`: the TCPS connect descriptor for the target database
- `ORACLE_GO_OAUTH_TOKEN_FILE`: the token file path

Set them before running [main.go](C:/work/driver/go-driver/go-oracledb/examples/token_authentication/oauth/main.go).

```bash
export ORACLE_GO_OAUTH_CONNECT_DESCRIPTOR="(description=(address=(protocol=tcps)(port=1522)(host=<db-host>))(connect_data=(service_name=<service-name>))(security=(ssl_server_dn_match=yes)))"
printf '%s' '<database-access-token>' > "$HOME/tokens/oauth/token"
export ORACLE_GO_OAUTH_TOKEN_FILE="$HOME/tokens/oauth/token"
go run ./examples/token_authentication/oauth
```

If the sample connects successfully, it prints the connected database user:

```text
Username: <database user>
```

## 6. Common checks when login fails

- Confirm that the database is configured for the external identity provider you are using.
- Confirm that the token has not expired.
- Confirm that the token is a database access token, not just a generic API token.
- Confirm that the mapped user or role exists in the database.
- Confirm that `ORACLE_GO_OAUTH_TOKEN_FILE` points to the expected token file and that it contains only the token text.
- Confirm that the connect descriptor uses TCPS and the correct service name.

## References

- JDBC token authentication with `OAUTH`: <https://docs.oracle.com/en/database/oracle/oracle-database/21/jjdbc/client-side-security.html>
- Microsoft Entra ID integration with Autonomous Database: <https://docs.oracle.com/en/cloud/paas/autonomous-database/serverless/adbsb/autonomous--azure-ad-about.html>
- Enable Microsoft Entra ID Authentication on Autonomous Database: <https://docs.oracle.com/en-us/iaas/autonomous-database-shared/doc/autonomous-azure-ad-enable.html>
- External authentication with Microsoft Entra ID on Autonomous Database tools: <https://docs.oracle.com/en-us/iaas/autonomous-database-serverless/doc/use-external-authentication-with-database-tools.html>
- Azure CLI `az account get-access-token`: <https://learn.microsoft.com/en-us/cli/azure/account?view=azure-cli-latest#az-account-get-access-token>
