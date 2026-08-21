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
	"database/sql/driver"
	"errors"
	"log/slog"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drv "github.com/oracle/go-oracledb/v26/internal/driver"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type ConnInstantiatorFactory func(config *oracleconfig.OracleDriverConfig, ns driverCommon.NetworkSession) (driverCommon.ConnectionInstantiator, error)
type ConnCreator func(ctx context.Context, option *naming.ConnectionOption, connectionID string) (driverCommon.NetworkSession, error)

// connector implements the database/sql/driver.connector interface for Oracle databases,
// allowing connections to Oracle via Go's standard database/sql package.
type connector struct {
	driver                  driver.Driver
	config                  *naming.ParsedConfig
	connectorConfig         *oracleconfig.OracleDriverConfig
	connCreator             ConnCreator
	connInstantiatorFactory ConnInstantiatorFactory
}

// used in tests
type oracleConnector = connector

// NewOracleConnector returns a new OracleConnector using the provided ParsedConfig.
// This is the main entry point for database/sql to acquire an Oracle database Connector.
func buildOracleConnector(driver driver.Driver, cfg *naming.ParsedConfig, drvConfig *oracleconfig.OracleDriverConfig) driver.Connector {
	// this should never return an error, both functions are being set to not nil values
	connector, _ := newOracleConnector(cfg, drvConfig, session.ConnectToOptionWithConnectionID, drv.GetConnectionInstantiator)
	return connector
}

// newOracleConnector private creator of OracleConnector for testing, returns an error if one of the functions is nil
func newOracleConnector(cfg *naming.ParsedConfig, drvConfig *oracleconfig.OracleDriverConfig, connCreator ConnCreator, connInstantiatorFactory ConnInstantiatorFactory) (driver.Connector, error) {
	if connCreator == nil || connInstantiatorFactory == nil {
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}
	return &connector{
		config:                  cfg,
		connectorConfig:         drvConfig,
		connCreator:             connCreator,
		connInstantiatorFactory: connInstantiatorFactory,
	}, nil
}

// Connect establishes a new database/sql/driver.Conn to Oracle.
// It opens a wire-level Oracle connection, performs authentication, and configures the session.
func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	var err error
	var ns driverCommon.NetworkSession
	var savedErr error                       // last error raised during attempt loop
	var savedOption *naming.ConnectionOption // last option tried during attempt loop
	isNsConnected := false
	isConnectionEstablished := false

	localizationService := common.NewLocalizationService(c.connectorConfig.Locale.ClientLanguage)

	sessionUid, _ := session.GenUUID()

	iterator := c.config.NewConnectionAttemptIterator(ctx)
	if !iterator.HasNext() {
		// No attempts means the connect string (or parsed config) didn't yield any usable
		// ADDRESS entries, so there is nothing we can connect to.
		err = common.NewOracleError(oracleErrors.ConnectFailed, errors.New("no connection attempts available"))
		return nil, localizationService.LocalizeError(err)
	}

	// clean-up function that will close network session if the connection could
	// not be established and the network session was created
	defer func() {
		if !isConnectionEstablished {
			if isNsConnected {
				ns.Disconnect(ctx, 0)
			}
		}
	}()

	var tctxToBeUsed context.Context
	var tCancelToBeUsed context.CancelFunc
	for iterator.HasNext() {
		option := iterator.Next()
		if option == nil {
			err = common.NewOracleError(oracleErrors.ConnectFailed, ctx.Err())
			return nil, localizationService.LocalizeError(err)
		}
		if err := ctx.Err(); err != nil {
			e := common.NewOracleError(oracleErrors.ConnectFailed, err)
			return nil, localizationService.LocalizeError(e)
		}
		tctxToBeUsed = ctx
		if common.Odl.Enabled(context.Background(), slog.LevelDebug) {
			common.Odl.Debug("Connector.Connect",
				"option",
				option)
		}
		desc := option.Description
		if desc != nil && desc.ConnectTimeout > 0 {
			// take precedence over the passed context if it is a shorter timeout
			tctxToBeUsed, tCancelToBeUsed =
				context.WithTimeoutCause(tctxToBeUsed, time.Duration(desc.ConnectTimeout)*time.Millisecond,
					common.NewCtxTimeoutCauseError("ConnectTimeout", uint(desc.ConnectTimeout),
						sessionUid))
			defer tCancelToBeUsed()
		}
		ns, err = c.connCreator(tctxToBeUsed, option, sessionUid)
		if err == nil {
			// the network session has successfully connected, it should be
			// disconnected the connection establishment fails.
			isNsConnected = true
			savedErr = nil
			savedOption = nil
			break
		}
		savedErr = err
		savedOption = option
	}

	if savedErr != nil {
		if ctxTErr, ok := savedErr.(common.CtxTimeoutCauseError); ok {
			//"%s Timeout of %d for %s.(CONNECTION_ID=%s)")
			localized := common.NewOracleError(oracleErrors.ConnectFailed,
				common.NewOracleError(oracleErrors.ConnectTimeout, nil, ctxTErr.GetSource(),
					ctxTErr.GetValue(), savedOption.Address.String(), ctxTErr.GetEmitterID()))
			return nil, localizationService.LocalizeError(localized)
		}
		e := common.NewOracleError(oracleErrors.ConnectFailed, savedErr)
		return nil, localizationService.LocalizeError(e)
	}

	common.Odl.Debug("Network session established")

	connInstantiator, err := c.connInstantiatorFactory(c.connectorConfig, ns)
	if err != nil {
		e := common.NewOracleError(oracleErrors.ConnectFailed, err)
		return nil, localizationService.LocalizeError(e)
	}
	connection, err := connInstantiator.GetConnection(ctx)
	if err == nil {
		// Connection established successfully no clean-up needed.
		isConnectionEstablished = true
	}
	return connection, localizationService.LocalizeError(err)
}

// Driver returns the database/sql/driver.Driver for this connector.
func (c *connector) Driver() driver.Driver {
	return c.driver
}
