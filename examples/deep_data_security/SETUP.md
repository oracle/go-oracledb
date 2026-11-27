# Set up OCI IAM and Autonomous Database

Use this setup for the [example](README.md) and the [provider example](provider/README.md). Both use Autonomous AI Database with Deep Data Security, an OCI IAM identity domain, and the same employee table and data grants. The `HR_POOL` database account signs in over mutual TLS (mTLS) with a client wallet, username, and password. The examples obtain a database-access token from OCI IAM.

You need permission to administer the identity domain and the database. Record the HTTPS identity domain URL from **Identity & Security → Domains → your domain**.

## 1. Register the database in OCI IAM

In the identity domain, open **Integrated applications**, add a **Confidential Application**, and name it `HR Database`. In its OAuth configuration:

1. Configure it as a resource server. Set primary audience to `HRDatabase` and add a scope named `DB_ACCESS`.
2. Configure it as a client and allow the **Client credentials** grant. Under client resources, authorize its own database scope. The service uses this app’s client ID and secret as HTTP Basic credentials to request a database-access token. The database also uses this client to retrieve signing keys.
3. Save and activate the application. Record its **Application ID**, **Client ID**, and **Client secret** separately.

The database scope in this example is `HRDatabaseDB_ACCESS`, the primary audience followed by the scope name. Confirm the exact scope shown by OCI.

## 2. Register the application in OCI IAM

Create a second **Confidential Application**, named `HR Application`, in the same domain. Enable **Enforce grants as authorization**. In its OAuth configuration:

1. Configure it as a client and allow **Client credentials**.
2. Under client resources, add the database scope `HRDatabaseDB_ACCESS`.
3. Save and activate it. Record its client ID and client secret.

## 3. Configure Autonomous Database

Use an Autonomous AI Database that supports Deep Data Security and mTLS. In the OCI Console, open the database's connection details and download its client wallet. Extract it to a local directory. The driver reads `ewallet.pem` from that directory; the wallet supplies the client certificate and key for the mTLS connection. Use the database's TCPS service address and retain server certificate verification.

Connect to the database as `ADMIN` and configure OCI IAM with the database application's **Application ID** and domain URL:

```sql
BEGIN
  DBMS_CLOUD_ADMIN.ENABLE_EXTERNAL_AUTHENTICATION(
    type => 'OCI_IAM',
    params => JSON_OBJECT(
      'app_id' VALUE '<DATABASE_APPLICATION_ID>',
      'domain_url' VALUE '<IDENTITY_DOMAIN_URL>'
    )
  );
END;
/

BEGIN
  DBMS_CLOUD.CREATE_CREDENTIAL(
    credential_name => 'OCI_IAM_DOMAIN_DB_CRED$',
    username => '<DATABASE_APPLICATION_CLIENT_ID>',
    password => '<DATABASE_APPLICATION_CLIENT_SECRET>'
  );
END;
/
```

Substitute your values securely. If OCI IAM authentication or the credential already exists, review the existing configuration before changing it. `force => TRUE` changes existing external authentication configuration.

## 4. Set the environment variables

The HTTP service and provider demo use the same settings.

```sh
export DDS_CONNECT_DESCRIPTOR='tcps://<database-host>:<port>/<service-name>?wallet_location="/path/to/wallet"&ssl_server_dn_match=true'
export DDS_DB_USER='HR_POOL'
export DDS_DB_PASSWORD='<password for HR_POOL>'
export DDS_IAM_DOMAIN_URL='https://<identity-domain>'
export DDS_DATABASE_CLIENT_ID='<Database client ID>'
export DDS_DATABASE_CLIENT_SECRET='<Database client secret>'
export DDS_DATABASE_SCOPE='HRDatabaseDB_ACCESS'
```

Use the database mTLS host, port, and service name, and the directory containing `ewallet.pem`. Set `ORACLE_GO_WALLET_PASSWORD` if the wallet key is encrypted. Use the exact database scope displayed in OCI. Make these variables available in the environment before starting either program. For the token endpoint, see how to [configure an IAM-managed end user](#configure-an-iam-managed-end-user).

## 5. Create the sample schema

Connect as `ADMIN` to the Autonomous AI Database you configured in step 3.
Run the following SQL to create the sample database accounts, local end users,
and data roles, then assign the roles to the end users.

```sql
CREATE USER HR_POOL IDENTIFIED BY <HR_POOL_PASSWORD>;
GRANT CREATE SESSION, CREATE END USER SECURITY CONTEXT TO HR_POOL;

CREATE USER HR_APP IDENTIFIED BY <HR_APP_PASSWORD>;
GRANT CREATE SESSION TO HR_APP;
GRANT CREATE TABLE TO HR_APP;
GRANT CREATE DATA GRANT TO HR_APP;
ALTER USER HR_APP QUOTA 10M ON DATA;

CREATE END USER ebaker;
CREATE END USER manderson;
CREATE DATA ROLE EMPLOYEE_ROLE;
CREATE DATA ROLE MANAGER_ROLE;

GRANT DATA ROLE EMPLOYEE_ROLE TO ebaker;
GRANT DATA ROLE EMPLOYEE_ROLE TO manderson;
GRANT DATA ROLE MANAGER_ROLE TO manderson;
```

### Create the table and data grants

Now connect to the same database as `HR_APP` using the password set above.

```sql
CREATE TABLE EMPLOYEES (
    employee_id NUMBER PRIMARY KEY,
    first_name VARCHAR2(100) NOT NULL,
    last_name VARCHAR2(100) NOT NULL,
    user_name VARCHAR2(256) NOT NULL,
    manager_name VARCHAR2(256),
    ssn VARCHAR2(11) NOT NULL
);

INSERT INTO EMPLOYEES VALUES (100, 'Victoria', 'Williams', 'vwilliams', NULL, '000-00-0100');
INSERT INTO EMPLOYEES VALUES (200, 'Marvin', 'Anderson', 'manderson', 'vwilliams', '000-00-0200');
INSERT INTO EMPLOYEES VALUES (300, 'Chris', 'Evans', 'cevans', 'vwilliams', '000-00-0300');
INSERT INTO EMPLOYEES VALUES (400, 'Emma', 'Baker', 'ebaker', 'manderson', '000-00-0400');
INSERT INTO EMPLOYEES VALUES (500, 'Taylor', 'Mills', 'tmills', 'manderson', '000-00-0500');
COMMIT;

CREATE DATA GRANT EMPLOYEE_OWN_RECORD AS SELECT
  ON EMPLOYEES
  WHERE user_name = ORA_END_USER_CONTEXT.username
  TO EMPLOYEE_ROLE;

CREATE DATA GRANT MANAGER_DIRECT_REPORTS AS SELECT (ALL COLUMNS EXCEPT ssn)
  ON EMPLOYEES
  WHERE manager_name = ORA_END_USER_CONTEXT.username
  TO MANAGER_ROLE;
```

## Configure an IAM-managed end user

Complete these steps only for `POST /employees/token`. This local demo uses
the OAuth 2.0 Resource Owner Password Credentials grant to exchange a test
user's OCI IAM username and password for an end-user access token.
This grant is for local testing only.

1. Create a dedicated IAM-managed test user and an `HR_TEAM` group. Add the user to the group and assign the user or group to **HR Application**.
2. As an **Identity Domain Administrator**, use the OCI IAM identity domains REST API to configure a custom claim named `group` in end-user access tokens. The claim must contain the display names of the end user's OCI IAM groups. Check for an existing `group` claim before creating one. See [Configure Custom Claims for Group Information in OCI IAM][oci-iam-group-claim].
3. Export the HR Application client credentials from step 2 and its exact scope.

```sh
export DDS_APP_CLIENT_ID='<HR Application client ID>'
export DDS_APP_CLIENT_SECRET='<HR Application client secret>'
export DDS_APP_SCOPE='HRApplicationAPP_ACCESS'
```

See how to
[call the endpoint as an IAM-managed end user](README.md#query-as-an-iam-managed-end-user).

### Add the HR data role and sample record

As `ADMIN`, create the data role mapped to the IAM group:

```sql
CREATE DATA ROLE HR_ROLE MAPPED TO 'IAM_OAUTH_GROUP=HR_TEAM';
```

As `HR_APP`, insert the HR user's employee record. Replace `<IAM_HR_USERNAME>` with the exact `sub` claim from that user's OCI IAM **end-user access token**. This is the username DDS exposes as `ORA_END_USER_CONTEXT.username`.

```sql
INSERT INTO EMPLOYEES (employee_id, first_name, last_name, user_name, manager_name, ssn)
VALUES (600, 'IAM', 'HR User', '<IAM_HR_USERNAME>', NULL, '000-00-0600');
COMMIT;

CREATE DATA GRANT HR_ALL_EMPLOYEES AS SELECT
  ON EMPLOYEES
  TO HR_ROLE;
```

The `user_name` column identifies whose employee record this is. Membership in `HR_TEAM` enables `HR_ROLE`. The example's `HR_ALL_EMPLOYEES` grant has no row predicate, so members can read every row and column, including the fictional SSNs. In production, limit each role to the rows and columns its members need.

## References

- [Register the Database in OCI IAM](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/register-database-oci-iam.html)
- [Register the Application in OCI IAM](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/register-application-oci-iam.html)
- [Download the Autonomous Database Client Wallet](https://docs.oracle.com/en-us/iaas/autonomous-database-serverless/doc/connect-download-wallet.html)
- [Configure Database–IAM Integration](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/configure-database-iam-integration.html)
- [Oracle Database Driver for Go Authentication Guide](../../documentation/authentication.md)
- [Fine-Grained Data Authorization](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/fine-grained-data-authorization.html)
- [Add a Confidential Application](https://docs.oracle.com/en-us/iaas/Content/Identity/applications/add-confidential-application.htm)
- [Resource Owner Password Credentials Grant Type](https://docs.oracle.com/en-us/iaas/Content/Identity/api-getstarted/ROPCGT.htm)
- [Create Users and Assign Groups in OCI IAM](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/create-users-and-assign-groups-oci-iam.html)
- [Configure Custom Claims for Group Information in OCI IAM][oci-iam-group-claim]
- [How the Database Server Manages an End-User Security Context](https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/how-database-server-manages-end-user-security-context.html)

[oci-iam-group-claim]: https://docs.oracle.com/en/database/oracle/oracle-database/26/ddscg/configure-custom-claims-group-information-oci-iam.html
