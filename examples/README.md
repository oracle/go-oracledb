# Go Driver examples

This folder contains code samples for the Oracle Database Driver for Go.

## Connection examples

- [Simple connection](simple/main.go) — Opens a `database/sql` connection with
  an `ORACLE_DSN`, verifies it, and queries the database time. It also shows
  optional driver logging configuration.
- [Explicit connector](connector/main.go) — Builds an Oracle connector with a
  separate connect descriptor, username, and password, then opens the pool
  with `sql.OpenDB`.

## Feature-specific examples

- [Deep Data Security](deep_data_security/README.md) — Runs one employee query as
  different end users. Set the context explicitly or have a
  [provider supply it for each query](deep_data_security/provider/README.md).
  See the [OCI IAM and Autonomous Database setup](deep_data_security/SETUP.md).
- [Oracle JSON](json/main.go) — Creates a table with a native JSON column,
  inserts JSON text and Go values, and reads JSON documents back.
  See the [JSON guide](../documentation/json.md)
  and the [example notes](json/README.md).
- [REF CURSOR](datatype/refcursor/main.go) — Fetches a PL/SQL REF CURSOR OUT
  bind and reads implicit result cursors. See the
  [REF CURSOR guide](../documentation/refcursor.md) and
  [example notes](datatype/refcursor/README.md).
- [Sessionless transactions](sessionless-transactions/main.go) — Suspends a
  sessionless transaction and resumes it on another connection. See the
  [sessionless transactions guide](../documentation/sessionless_transactions.md)
  and [example notes](sessionless-transactions/README.md).
- [OAuth token authentication](token_authentication/oauth/main.go) — Registers
  a token provider and connects using OAuth database access tokens. See the
  [OAuth section of the authentication guide](../documentation/authentication.md#3-oauth-token-authentication)
  and [example setup](token_authentication/oauth/README.md).
- [OCI IAM token authentication](token_authentication/oci_token/main.go) —
  Registers a file-backed provider for OCI IAM database tokens and connects to
  an Autonomous Database. See the
  [OCI IAM section of the authentication guide](../documentation/authentication.md#4-oci-iam-signed-token-authentication)
  and [example setup](token_authentication/oci_token/README.md).
