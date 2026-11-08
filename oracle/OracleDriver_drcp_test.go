/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

func getDBUsingConfig(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error) {

	drvConfig := NewOracleDriverConfig()

	drvConfig.Credentials.LogonMode = config.Credentials.LogonMode
	drvConfig.Credentials.User = config.Credentials.Username
	drvConfig.Credentials.Password = config.Credentials.Password

	drvConfig.ConnectionProperties.ServerType = common.ServerTypePooled

	drvConfig.ConnectionProperties.Drcp.ClassName = className
	drvConfig.ConnectionProperties.Drcp.PoolName = poolName
	drvConfig.ConnectionProperties.Drcp.Purity = purity

	drvConfig.ConnectDescriptor = config.GetConnectionDSN()
	connector, err := NewOracleConnector(drvConfig)
	if err != nil {
		return nil, err
	}

	db := sql.OpenDB(connector)
	return db, nil
}

func getDBUsingEZConnect(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error) {

	var err error
	var db *sql.DB

	var dsn = fmt.Sprintf("%s/%s@tcp://%s:%d/%s:POOLED?",
		config.Credentials.Username,
		config.Credentials.Password,
		config.Database.Host,
		config.Database.Port,
		config.Database.ServiceName)

	var added = false
	if className != "" {
		dsn = dsn + "oracle.go.ConnectionProperties.Drcp.ClassName=" + className
		added = true
	}
	if purity != "" {
		if added {
			dsn = dsn + "&"
		} else {
			added = true
		}
		dsn = dsn + "oracle.go.ConnectionProperties.Drcp.Purity=" + purity
	}
	if poolName != "" {
		if added {
			dsn = dsn + "&"
		} else {
			added = true
		}
		dsn = dsn + "oracle.go.ConnectionProperties.Drcp.PoolName=" + poolName
	}
	if config.Credentials.LogonMode != "" {
		if added {
			dsn = dsn + "&"
		}
		dsn = fmt.Sprintf("%soracle.go.Credentials.logon_mode=%s",
			dsn, config.Credentials.LogonMode)
	}
	db, err = sql.Open("oracledb", dsn)
	db.SetMaxOpenConns(1)
	return db, err
}
func getDBUsingDescriptor(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error) {
	var err error
	var db *sql.DB
	var dsn = fmt.Sprintf("%s/%s@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=%s)(PORT=%d))(CONNECT_DATA=(SERVICE_NAME=%s)(SERVER=POOLED)",
		config.Credentials.Username,
		config.Credentials.Password,
		config.Database.Host,
		config.Database.Port,
		config.Database.ServiceName)
	if className != "" {
		dsn = fmt.Sprintf("%s(POOL_CONNECTION_CLASS=%s)", dsn, className)
	}
	if purity != "" {
		dsn = fmt.Sprintf("%s(POOL_PURITY=%s)", dsn, purity)
	}
	if poolName != "" {
		dsn = fmt.Sprintf("%s(POOL_NAME=%s)", dsn, poolName)
	}
	dsn = dsn + "))"
	if config.Credentials.LogonMode != "" {
		dsn = fmt.Sprintf("%s?oracle.go.Credentials.logon_mode=%s",
			dsn, config.Credentials.LogonMode)
	}
	db, err = sql.Open("oracledb", dsn)
	db.SetMaxOpenConns(1)
	return db, err
}

// TestDriver_DRCP_Basic executes a trivial SELECT to validate DRCP query flow.
// Expectations: we should be attached and detached accordingly.
func TestDriver_DRCP_Basic(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	tests := []struct {
		name      string
		cfunc     func(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error)
		className string
	}{
		{
			name:      "using config",
			cfunc:     getDBUsingConfig,
			className: "TestDriverDRCPBasic" + drvCommon.GenRandomString(2),
		},
		{
			name:      "using EZconnect",
			cfunc:     getDBUsingEZConnect,
			className: "TestDriverDRCPBasic" + drvCommon.GenRandomString(2),
		},
		{
			name:      "using descriptor",
			cfunc:     getDBUsingDescriptor,
			className: "TestDriverDRCPBasic" + drvCommon.GenRandomString(2),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := tt.cfunc("", tt.className, "", TestingConfig)
			if err != nil {
				t.Fatalf("open of DB failed [%v]", err)
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Errorf("close db: %v", err)
				}
			}()
			t.Logf("Grabbing cnx from DB")
			cnx, err := db.Conn(context.Background())
			if err != nil {
				t.Fatalf("get sql connection failed: %v", err)
			}

			rows, err := cnx.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
			if err != nil {
				t.Fatalf("select from DUAL failed: %v", err)
			}
			rows.Close()

			// Checking out a connection from the pool will attach a connection to a DRCP pooled server.
			if !isDrcpAttached(t, tt.className, "", "") {
				t.Fatalf("Should be attached now")
			}

			t.Logf("closing the cnx")
			// that will trigger a detach from DRCP pool
			err = cnx.Close()
			if err != nil {
				t.Fatalf("close of DRCP connection failed: %v", err)
			}

			if isDrcpAttached(t, tt.className, "", "") {
				t.Fatalf("Should be detached now")
			}

			t.Logf("TestDriver_DRCP_Basic Success")
		})
	}

}

// TestDriver_DRCP_ClassAutoGen verifies that class name is optional.
//
//	server uses "<SCHEMA>.SHARED" as class name then
//
// Expectations: DRCP attachment on <SCHEMA>.SHARED should be found
func TestDriver_DRCP_ClassAutoGen(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	tests := []struct {
		name  string
		cfunc func(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error)
	}{
		{
			name:  "using config",
			cfunc: getDBUsingConfig,
		},
		{
			name:  "using EZconnect",
			cfunc: getDBUsingEZConnect,
		},
		{
			name:  "using descriptor",
			cfunc: getDBUsingDescriptor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := tt.cfunc("", "", "", TestingConfig)
			if err != nil {
				t.Fatalf("open of DB failed [%v]", err)
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Errorf("close db: %v", err)
				}
			}()
			cnx, err := db.Conn(context.Background())
			if err != nil {
				t.Fatalf("get dedicated sql connection failed: %v", err)
			}
			defer cnx.Close()
			err = cnx.PingContext(context.Background())
			if err != nil {
				t.Fatalf("Can't ping on connection: %v", err)
			}
			if !isDrcpAttached(t, "SHARED", "", "") {
				t.Fatalf("Should be attached now")
			}
		})
	}

}

// isDrcpAttached checks that a DRCP class is attached
// parameters:
//
//		t: testing
//		db:  DB object to perform priv selection on stat views
//		expectedClass: the DRCP connection class to look for (without prefix)
//		expectedPool: the DRCP pool to look for (if not empty)
//	 expectedPurity: the DRCP purity to look for (if not empty)
//	 Note: this method requires a privileged connection
//
// returns:
//
//	true if attached, false otherwise.
func isDrcpAttached(t *testing.T, expectedClass string, expectedPool string, expectedPurity string) bool {
	t.Helper()

	var attachedClass sql.NullString
	var poolName sql.NullString
	var purity sql.NullString
	var tag sql.NullString
	var connection_status sql.NullString

	drvConfig := NewOracleDriverConfig()

	drvConfig.Credentials.User = TestingConfig.Credentials.Username
	drvConfig.Credentials.Password = TestingConfig.Credentials.Password
	drvConfig.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
	drvConfig.ConnectDescriptor = TestingConfig.GetConnectionDSN()

	privConnector, err := NewOracleConnector(drvConfig)
	if err != nil {
		t.Fatalf("create priv connector failed: %v", err)
	}

	privdb := sql.OpenDB(privConnector)
	defer func() {
		if err := privdb.Close(); err != nil {
			t.Errorf("close privdb: %v", err)
		}
	}()

	dumpActivePooledConnections(privdb, t)

	t.Logf("isDrcpAttached looking for class=[%q], pool=[%q], purity=[%q]", expectedClass, expectedPool, expectedPurity)
	query := `SELECT cclass_name, pool_name, purity, tag, connection_status
		   FROM v$cpool_conn_info
		  WHERE UPPER(cclass_name) LIKE '%.' || UPPER(:1)`

	args := []any{expectedClass}

	if expectedPool != "" {
		query += fmt.Sprintf(` AND pool_name = :%d`, len(args)+1)
		args = append(args, strings.ToUpper(expectedPool))
	}
	if expectedPurity != "" {
		query += fmt.Sprintf(` AND purity = :%d`, len(args)+1)
		args = append(args, expectedPurity)
	}

	rows := privdb.QueryRowContext(context.Background(), query, args...)
	if rows.Err() != nil {
		if rows.Err().(oracleErrors.SQLError).ErrorCode() == string(oracleErrors.TableOrViewNotFound) {
			// requires "Grant SELECT ON SYS.V_$CPOOL_CONN_INFO"
			t.Skipf("cannot verify DRCP attachment (missing privileges?): %v", rows.Err())
		}
		if errors.Is(sql.ErrNoRows, rows.Err()) {
			t.Logf("DRCP connection class %q was not found in v$cpool_conn_info", expectedClass)
			return false
		}
		t.Fatalf("query v$cpool_conn_info for attached DRCP class %q failed: %v", expectedClass, rows.Err())
	}

	err = rows.Scan(&attachedClass, &poolName, &purity, &tag, &connection_status)
	if err != nil {
		t.Fatalf("scan v$cpool_conn_info for attached DRCP class failed: %v", err)
	}

	t.Logf("DRCP conn info class %q , pool %q , purity %q, tag %q, status %q",
		attachedClass.String, poolName.String, purity.String, tag.String, connection_status.String)

	if connection_status.String != "ACTIVE" {
		return false
	}

	return true
}

func dumpActivePooledConnections(privdb *sql.DB, t *testing.T) {
	var attachedClass sql.NullString
	var poolName sql.NullString
	var purity sql.NullString
	var tag sql.NullString
	var connection_status sql.NullString

	allRows, _ := privdb.QueryContext(context.Background(),
		"SELECT cclass_name, pool_name, purity, tag, connection_status FROM v$cpool_conn_info")
	defer allRows.Close()

	for allRows.Next() {
		err := allRows.Scan(&attachedClass, &poolName, &purity, &tag, &connection_status)
		if err == nil {
			t.Logf("class=%q, pool=%q, purity=%q, tag=%q, connection_status=%q",
				attachedClass.String, poolName.String, purity.String, tag.String, connection_status.String)
		} else {
			t.Logf("v$cpool_conn_info Scan failed %v", err)
		}

	}
}

// _DRCP_SetupPool setup a DRCP pool
// parameters
//   - poolName the name of the new pool
func _DRCP_SetupPool(t *testing.T, poolName string) error {
	t.Helper()

	drvConfig := NewOracleDriverConfig()

	drvConfig.Credentials.User = TestingConfig.Credentials.Username
	drvConfig.Credentials.Password = TestingConfig.Credentials.Password
	drvConfig.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
	drvConfig.ConnectDescriptor = TestingConfig.GetConnectionDSN()

	privConnector, err := NewOracleConnector(drvConfig)
	if err != nil {
		t.Fatalf("create priv connector failed: %v", err)
	}

	privdb := sql.OpenDB(privConnector)
	defer func() {
		if err := privdb.Close(); err != nil {
			t.Errorf("close privdb: %v", err)
		}
	}()

	_, err = privdb.ExecContext(context.Background(),
		"BEGIN DBMS_CONNECTION_POOL.ADD_POOL(:1); END;",
		poolName,
	)
	if err != nil {
		t.Logf("add pool failed: %v", err)
		return err
	}

	_, err = privdb.ExecContext(context.Background(),
		"BEGIN DBMS_CONNECTION_POOL.START_POOL(:1); END;",
		poolName,
	)
	if err != nil {
		t.Logf("start pool failed: %v", err)
		return err
	}
	return nil
}

// _DRCP_CleanupPool remove a DRCP pool
// parameters
//   - poolName the name of the pool to be removed
func _DRCP_CleanupPool(t *testing.T, poolName string) error {
	t.Helper()

	drvConfig := NewOracleDriverConfig()

	drvConfig.Credentials.User = TestingConfig.Credentials.Username
	drvConfig.Credentials.Password = TestingConfig.Credentials.Password
	drvConfig.Credentials.LogonMode = TestingConfig.Credentials.LogonMode
	drvConfig.ConnectDescriptor = TestingConfig.GetConnectionDSN()

	privConnector, err := NewOracleConnector(drvConfig)
	if err != nil {
		t.Fatalf("create priv connector failed: %v", err)
	}

	privdb := sql.OpenDB(privConnector)
	defer func() {
		if err := privdb.Close(); err != nil {
			t.Errorf("close privdb: %v", err)
		}
	}()

	t.Logf("stopping pool %q", poolName)
	privdb.ExecContext(context.Background(),
		"BEGIN DBMS_CONNECTION_POOL.STOP_POOL(:1); END;",
		poolName,
	)

	t.Logf("removing pool %q", poolName)
	_, err = privdb.ExecContext(context.Background(),
		"BEGIN DBMS_CONNECTION_POOL.REMOVE_POOL(:1); END;",
		poolName,
	)
	if err != nil {
		t.Logf("remove pool failed: %v", err)
		return err
	}
	t.Logf("pool %q dropped", poolName)
	return nil
}

// TestDriver_DRCP_PoolName attempt to use DRCP with a specific pool name.
// Expectations: we should be attached to the specified pool
func TestDriver_DRCP_PoolName(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	testPoolName := "TestDriverDRCPPoolName"

	err := _DRCP_SetupPool(t, testPoolName)
	if err != nil {
		t.Fatalf("setup pool failed: %v", err)
	}

	defer _DRCP_CleanupPool(t, testPoolName)

	tests := []struct {
		name      string
		cfunc     func(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error)
		className string
		poolName  string
	}{
		{
			name:      "using config",
			cfunc:     getDBUsingConfig,
			className: "TestDriverDRCPPoolName" + drvCommon.GenRandomString(2),
			poolName:  testPoolName,
		},
		{
			name:      "using EZconnect",
			cfunc:     getDBUsingEZConnect,
			className: "TestDriverDRCPPoolName" + drvCommon.GenRandomString(2),
			poolName:  testPoolName,
		},
		{
			name:      "using descriptor",
			cfunc:     getDBUsingDescriptor,
			className: "TestDriverDRCPPoolName" + drvCommon.GenRandomString(2),
			poolName:  testPoolName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("running %q.", tt.name)
			db, err := tt.cfunc(tt.poolName, tt.className, "", TestingConfig)
			if err != nil {
				t.Fatalf("open of DB failed [%v]", err)
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Errorf("close db: %v", err)
				}
			}()
			cnx, err := db.Conn(context.Background())
			if err != nil {
				t.Fatalf("get dedicated sql connection failed: %v", err)
			}
			defer cnx.Close()
			rows, err := cnx.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
			if err != nil {
				t.Fatalf("select from DUAL failed: %v", err)
			}
			rows.Close()
			t.Logf("new connection created [%v]", cnx)

			// here we are already attached as going out of th pool attaches us.
			if !isDrcpAttached(t, tt.className, tt.poolName, "") {
				t.Errorf("Should be attached now")
			}

			t.Logf("TestDriver_DRCP_PoolName/%s Success", tt.poolName)
		})
	}
}

// TestDriver_DRCP_WrongPoolName attempt to use DRCP with a pool name that do not exists.
// Expectations: connection fail on
//
//	OGD-00091 - An error occurred during authentication: ORA-56500: DRCP: Pool not found
func TestDriver_DRCP_WrongPoolName(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	tConf := TestingConfig.Clone()
	tConf.Database.ServerType = common.ServerTypePooled

	config := NewOracleDriverConfig()

	config.Credentials.LogonMode = tConf.Credentials.LogonMode

	config.Credentials.User = tConf.Credentials.Username
	config.Credentials.Password = tConf.Credentials.Password

	config.ConnectionProperties.ServerType = common.ServerTypePooled
	config.ConnectionProperties.Drcp.ClassName = "TestDriverDRCPWrongPoolName" + drvCommon.GenRandomString(2)
	config.ConnectionProperties.Drcp.PoolName = "IDONOTEXIST"

	config.ConnectDescriptor = tConf.GetConnectionDSN()
	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("create connector failed: %v", err)
	}

	db := sql.OpenDB(connector)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	}()

	_, err = db.Conn(context.Background())
	if err == nil {
		t.Fatalf("get sql connection expected to fail")
	}
	checkErrorRaised(t, err, oracleErrors.AuthenticatorError,
		"ORA-56500")
}

// TestDriver_DRCP_PoolName attempt to use DRCP with a specific pool name.
// Expectations: we should be attached to the specified pool
func TestDriver_DRCP_Purity(t *testing.T) {
	t.Parallel()
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}

	tests := []struct {
		name      string
		cfunc     func(poolName string, className string, purity string, config *TestConfig) (*sql.DB, error)
		className string
		purity    string
	}{
		{
			name:      "using config",
			cfunc:     getDBUsingConfig,
			className: "TestDriverDRCPPurity" + drvCommon.GenRandomString(2),
			purity:    common.DrcpPurityNew,
		},
		{
			name:      "using EZconnect",
			cfunc:     getDBUsingEZConnect,
			className: "TestDriverDRCPPurity" + drvCommon.GenRandomString(2),
			purity:    common.DrcpPurityNew,
		},
		{
			name:      "using descriptor",
			cfunc:     getDBUsingDescriptor,
			className: "TestDriverDRCPPurity" + drvCommon.GenRandomString(2),
			purity:    common.DrcpPurityNew,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := tt.cfunc("", tt.className, tt.purity, TestingConfig)
			defer func() {
				if err := db.Close(); err != nil {
					t.Errorf("close db: %v", err)
				}
			}()
			cnx, err := db.Conn(context.Background())
			if err != nil {
				t.Fatalf("get dedicated sql connection failed: %v", err)
			}

			t.Logf("new connection created [%v]", cnx)

			// here we are already attached as going out of th pool attaches us.
			if !isDrcpAttached(t, tt.className, "",
				tt.purity) {
				t.Errorf("Should be attached now")
			}
			cnx.Close()
			t.Logf("TestDriver_DRCP_Purity Success")
		})
	}

}
