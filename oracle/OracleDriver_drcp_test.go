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
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drvCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestDriver_DRCP_Basic executes a trivial SELECT to validate DRCP query flow.
// Expectations: we should be attached and detached accordingly.
func TestDriver_DRCP_Basic(t *testing.T) {
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
	config.ConnectionProperties.Drcp.ClassName = "TestDriver_DRCP_Basic_" + drvCommon.GenRandomString(2)

	config.ConnectDescriptor = tConf.GetConnectionDSN()
	connector, err := NewOracleConnector(config)
	if err != nil {
		t.Fatalf("create connector failed: %v", err)
	}

	t.Logf("Opening DB")
	db := sql.OpenDB(connector)
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	}()
	db.SetMaxOpenConns(1)

	t.Logf("Grabbing cnx from DB")
	cnx, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get dedicated sql connection failed: %v", err)
	}

	t.Logf("grabbed cnx from DB [%v]", cnx)

	rows, err := cnx.QueryContext(context.Background(), "SELECT 1 FROM DUAL")
	if err != nil {
		t.Fatalf("select from DUAL failed: %v", err)
	}
	rows.Close()

	// Checking out a connection from the pool will attach a connection to a DRCP pooled server.
	if !isDrcpAttached(t, config, config.ConnectionProperties.Drcp.ClassName, "", "") {
		t.Fatalf("Should be attached now")
	}

	t.Logf("closing the cnx")
	// that will trigger a detach from DRCP pool
	err = cnx.Close()
	if err != nil {
		t.Fatalf("close of DRCP connection failed: %v", err)
	}

	if isDrcpAttached(t, config, config.ConnectionProperties.Drcp.ClassName, "", "") {
		t.Fatalf("Should be detached now")
	}

	t.Logf("TestDriver_DRCP_Basic Success")
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

	tConf := TestingConfig.Clone()
	tConf.Database.ServerType = common.ServerTypePooled
	config := NewOracleDriverConfig()
	config.Credentials.LogonMode = tConf.Credentials.LogonMode
	config.Credentials.User = tConf.Credentials.Username
	config.Credentials.Password = tConf.Credentials.Password
	config.ConnectionProperties.ServerType = common.ServerTypePooled

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
	db.SetMaxOpenConns(1)

	cnx, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get dedicated sql connection failed: %v", err)
	}
	defer cnx.Close()
	err = cnx.PingContext(context.Background())
	if err != nil {
		t.Fatalf("Can't ping on connection: %v", err)
	}
	if !isDrcpAttached(t, config, "SHARED", "", "") {
		t.Fatalf("Should be attached now")
	}

}

// isDrcpAttached checks that a DRCP class is attached
// parameters:
//
//	t: testing
//	db:  DB object to perform priv selection on stat views
//	expectedClass: the DRCP connection class to look for (without prefix)
//	expectedTag: the DRCP class tag to look for (if not empty)
//
// returns:
//
//	true if attached, false otherwise.
func isDrcpAttached(t *testing.T, config *config.OracleDriverConfig, expectedClass string, expectedPool string, expectedPurity string) bool {
	t.Helper()

	var attachedClass sql.NullString
	var poolName sql.NullString
	var purity sql.NullString
	var tag sql.NullString
	var connection_status sql.NullString

	// conf to be used for request that valid current attachment
	privConfig := config.Clone()
	privConfig.ConnectionProperties.ServerType = common.ServerTypeDedicated
	privConfig.ConnectionProperties.Drcp.ClassName = ""

	privConnector, err := NewOracleConnector(privConfig)
	if err != nil {
		t.Fatalf("create priv connector failed: %v", err)
	}

	privdb := sql.OpenDB(privConnector)
	defer func() {
		if err := privdb.Close(); err != nil {
			t.Errorf("close privdb: %v", err)
		}
	}()

	t.Logf("isDrcpAttached looking for class=[%s]", expectedClass)
	query := `SELECT cclass_name, pool_name, purity, tag, connection_status
		   FROM v$cpool_conn_info
		  WHERE UPPER(cclass_name) LIKE '%.' || UPPER(:1)`
	args := []any{expectedClass}

	if expectedPool != "" {
		query += fmt.Sprintf(` AND pool_name = :%d`, len(args)+1)
		args = append(args, expectedPool)
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

// TestDriver_DRCP_PoolName attempt to use DRCP with a specific pool name.
// Expectations: we should be attached to the specified pool
func TestDriver_DRCP_PoolName(t *testing.T) {
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
	config.ConnectionProperties.Drcp.ClassName = "TestDriver_DRCP_PoolName" + drvCommon.GenRandomString(2)
	config.ConnectionProperties.Drcp.PoolName = "MYPOOL"

	// we cannot rely on tConf.GetConnectionDSN() as it build connect descriptor that ignore what is in the conf
	//  that would drop the pool name info
	config.ConnectDescriptor = fmt.Sprintf("tcp://%s:%d/%s?oracle.go.Credentials.logonMode=sysdba",
		tConf.Database.Host, tConf.Database.Port, tConf.Database.ServiceName)
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
	db.SetMaxOpenConns(1)

	cnx, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get dedicated sql connection failed: %v", err)
	}

	cnx.QueryContext(context.Background(), "SELECT 1 FROM DUAL")

	t.Logf("new connection created [%v]", cnx)

	// here we are already attached as going out of th pool attaches us.
	if !isDrcpAttached(t, config, config.ConnectionProperties.Drcp.ClassName, config.ConnectionProperties.Drcp.PoolName, "") {
		t.Fatalf("Should be attached now")
	}

	t.Logf("TestDriver_DRCP_PoolName Success")
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
	config.ConnectionProperties.Drcp.ClassName = "TestDriver_DRCP_WrongPoolName" + drvCommon.GenRandomString(2)
	config.ConnectionProperties.Drcp.PoolName = "I_DO_NOT_EXIST"

	// we cannot rely on tConf.GetConnectionDSN() as it build connect descriptor that ignore what is in the conf
	//  that would drop the pool name info
	config.ConnectDescriptor = fmt.Sprintf("tcp://%s:%d/%s?oracle.go.Credentials.logonMode=sysdba",
		tConf.Database.Host, tConf.Database.Port, tConf.Database.ServiceName)
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

	tConf := TestingConfig.Clone()
	tConf.Database.ServerType = common.ServerTypePooled

	config := NewOracleDriverConfig()

	config.Credentials.LogonMode = tConf.Credentials.LogonMode

	config.Credentials.User = tConf.Credentials.Username
	config.Credentials.Password = tConf.Credentials.Password

	config.ConnectionProperties.ServerType = common.ServerTypePooled
	config.ConnectionProperties.Drcp.ClassName = "TestDriver_DRCP_Purity" + drvCommon.GenRandomString(2)
	config.ConnectionProperties.Drcp.Purity = common.DrcpPuritySelf

	// we cannot rely on tConf.GetConnectionDSN() as it build connect descriptor that ignore what is in the conf
	//  that would drop the pool name info
	config.ConnectDescriptor = fmt.Sprintf("tcp://%s:%d/%s?oracle.go.Credentials.logonMode=sysdba",
		tConf.Database.Host, tConf.Database.Port, tConf.Database.ServiceName)
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

	cnx, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get dedicated sql connection failed: %v", err)
	}

	t.Logf("new connection created [%v]", cnx)

	// here we are already attached as going out of th pool attaches us.
	if !isDrcpAttached(t, config, config.ConnectionProperties.Drcp.ClassName, config.ConnectionProperties.Drcp.PoolName,
		common.DrcpPuritySelf) {
		t.Fatalf("Should be attached now")
	}

	t.Logf("TestDriver_DRCP_PoolName Success")
}
