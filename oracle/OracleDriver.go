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
	"database/sql"
	"database/sql/driver"
	"log/slog"
	"sync"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// Driver is exported to make the driver directly accessible.
// In general the driver is used via the database/sql package.
type Driver struct {
	connectorConfig *oracleconfig.OracleDriverConfig
}

var oracleDriver = NewDriver()

// GetDriverInterface is implemented by types providing a database/sql/driver.Driver instance.
type GetDriverInterface interface {
	Driver() driver.Driver
}

func init() {
	sql.Register(common.DriverName, oracleDriver)
}

// GetDefaultDriver returns the singleton OracleDriver instance used for global driver registration.
func GetDefaultDriver() *Driver {
	return oracleDriver
}

// NewDriver creates a new OracleDriver instance for use with database/sql
func NewDriver() *Driver {
	return &Driver{}
}

// NewDriverWithConfig creates a new OracleDriver instance for use with database/sql
// with specific OracleDriverConfig
func NewDriverWithConfig(cfg *oracleconfig.OracleDriverConfig) *Driver {
	return &Driver{connectorConfig: cfg}
}

// NewOracleDriverConfig creates a new driver configuration.
func NewOracleDriverConfig() *oracleconfig.OracleDriverConfig {
	return oracleconfig.NewOracleDriverConfig()
}

// NewOracleLoggingConfig creates a new driver logging configuration.
func NewOracleLoggingConfig() *oracleconfig.OracleLoggingConfig {
	return oracleconfig.NewOracleLoggingConfig()
}

// ApplyDriverLoggingConfig applies the configuration to this driver
// Logging configuration is also applied
// config must not be nil.
func (drv *Driver) ApplyDriverLoggingConfig(config *oracleconfig.OracleLoggingConfig) {
	if config != nil {
		common.InitLoggingWithConfig(config)
	}
}

// NewOracleConnector creates a new Oracle database connector
// Parameters:
//
//	config : the connector configuration
//
// Returns:
//
//	. A new connector
//	. En error if creation has failed
func NewOracleConnector(config *oracleconfig.OracleDriverConfig) (driver.Connector, error) {
	drv := NewDriverWithConfig(config)
	var connector driver.Connector
	var err error
	if config != nil {
		connector, err = drv.openConnector(config.ConnectDescriptor)
	} else {
		connector, err = drv.openConnector("")
	}

	if err != nil {
		return nil, err
	}
	return connector, nil
}

// Open implements database/sql/driver.Driver interface for OracleDriver.
// It establishes a new Oracle connection using the given Data Source Name.
func (drv *Driver) Open(dsn string) (driver.Conn, error) {
	connector, err := drv.openConnector(dsn)
	if err != nil {
		return nil, err
	}
	return connector.Connect(common.BackgroundContext)
}

var _initLoggingOnce sync.Once

// openConnector returns a new database/sql/driver.Connector for this OracleDriver, using the Data Source Name.
func (drv *Driver) openConnector(dsn string) (driver.Connector, error) {

	_initLoggingOnce.Do(func() {
		// This is delayed until now because we cannot assume the start sequence of the application.
		// Doing this in init() may end up defining flags after the CLI has been parsed.
		common.InitLoggingWithConfig(NewOracleLoggingConfig())
	})

	var confToUse *oracleconfig.OracleDriverConfig
	if drv.connectorConfig != nil {
		confToUse = drv.connectorConfig
	} else {
		confToUse = NewOracleDriverConfig()
	}

	if err := confToUse.AssignFromFlags(); err != nil {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, err, dsn)
	}

	if err := confToUse.AssignFromEnv(); err != nil {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, err, dsn)
	}

	connectionPlans, err := oracleconfig.HandleDsn(dsn, confToUse)
	if err != nil {
		common.Odl.Debug("Failed to parse DSN", "err", err)
		return nil, err
	}

	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("new connector created", "configuration", confToUse)
	}

	if validateErr := confToUse.Validate(); validateErr != nil {
		return nil, common.NewOracleError(oracleErrors.NamingDSNInvalid, validateErr, dsn)
	}

	common.Odl.Fine("connection descriptions", "count", len(connectionPlans.Plan.Descriptions))

	return buildOracleConnector(drv, connectionPlans), nil
}
