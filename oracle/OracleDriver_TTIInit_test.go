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
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

// TestDriver_TTIInitPassword verifies a password connection using the combined exchange.
func TestDriver_TTIInitPassword(t *testing.T) {
	if TestingConfig == nil {
		t.Skip("No configuration available")
	}
	connector, err := openTestConnectorWithConfig(TestingConfig)
	if err != nil {
		t.Fatalf("create password connector: %v", err)
	}
	checkTTIInitConnection(t, connector)
}

// TestDriver_TTIInitOCIToken verifies a signed OCI IAM token connection using TTIINIT.
func TestDriver_TTIInitOCIToken(t *testing.T) {
	descriptor := os.Getenv("ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR")
	tokenLocation := os.Getenv("ORACLE_GO_OCI_TOKEN_LOCATION")
	if descriptor == "" || tokenLocation == "" {
		t.Skip("requires ORACLE_GO_OCI_TOKEN_CONNECT_DESCRIPTOR and ORACLE_GO_OCI_TOKEN_LOCATION")
	}

	cfg := NewOracleDriverConfig()
	cfg.ConnectDescriptor = descriptor
	connector, err := NewOracleConnector(cfg)
	if err != nil {
		t.Fatalf("create OCI token connector: %v", err)
	}
	connector.(oracleProviders.ProviderRegistrar).RegisterProvider(testOCITokenProvider{
		tokenPath:      filepath.Join(tokenLocation, testOCITokenFileName),
		privateKeyPath: filepath.Join(tokenLocation, testOCIPrivateKeyFileName),
	})
	checkTTIInitConnection(t, connector)
}

func checkTTIInitConnection(t *testing.T, connector driver.Connector) {
	t.Helper()
	c := connector.(*oracleConnector)
	c.connectorConfig.DriverProperties.TTIInitOptimizationEnabled = true
	unsupported := errors.New("database does not advertise fast authentication")
	factory := c.connInstantiatorFactory
	c.connInstantiatorFactory = func(cfg *oracleconfig.OracleDriverConfig, ns driverCommon.NetworkSession, providers common.Registry[oracleProviders.Provider], proPending bool) (driverCommon.ConnectionInstantiator, error) {
		if !ns.SupportsFastAuthentication() {
			return nil, unsupported
		}
		if proPending {
			return nil, errors.New("server requested TTIRENEG instead of accepting TTIINIT")
		}
		instantiator, err := factory(cfg, ns, providers, proPending)
		if err != nil {
			return nil, err
		}
		if got := fmt.Sprintf("%T", instantiator); got != "*ttc.ttiInitConnectionInstantiator" {
			return nil, fmt.Errorf("factory selected %s, want TTIINIT", got)
		}
		return instantiator, nil
	}

	db := sql.OpenDB(c)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var got int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM DUAL").Scan(&got)
	if errors.Is(err, unsupported) {
		t.Skip(unsupported.Error())
	}
	if err != nil {
		t.Fatalf("query using TTIINIT: %v", err)
	}
	if got != 1 {
		t.Fatalf("SELECT 1 returned %d", got)
	}
}
