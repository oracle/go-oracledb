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
	"fmt"
	"io"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	drv "github.com/oracle/go-oracledb/v26/internal/driver"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

// ConnInstantiatorFactory creates a connection instantiator for an established
// network session and the connector's currently registered providers.
//
// Parameters:
//   - config: the base Oracle driver configuration for the connection.
//   - ns: the established network session to bind to the instantiator.
//   - providerRegistry: the provider registry registered on the connector for this attempt.
//
// Returns:
//   - a connection instantiator bound to ns.
//   - an error if the instantiator cannot be created.
type ConnInstantiatorFactory func(config *oracleconfig.OracleDriverConfig, ns driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error)

// ConnCreator opens a network session for a fully resolved connection attempt.
//
// Parameters:
//   - ctx: the context controlling the network connection attempt.
//   - attempt: the resolved endpoint, connection identifier, and effective
//     transport options.
//
// Returns:
//   - the connected network session.
//   - an error if the network session cannot be established.
type ConnCreator func(ctx context.Context, attempt *oracleconfig.ConnectionAttempt) (driverCommon.NetworkSession, error)

// connector implements the database/sql/driver.connector interface for Oracle databases,
// allowing connections to Oracle via Go's standard database/sql package.
type connector struct {
	driver                  driver.Driver
	connectionPlans         *oracleconfig.ConnectionPlans
	connCreator             ConnCreator
	connInstantiatorFactory ConnInstantiatorFactory
	providerRegistry        common.Registry[oracleProviders.Provider]
}

// used in tests
type oracleConnector = connector

// buildOracleConnector returns a connector configured from the normalized DSN plans.
//
// Input:
//   - driver: database/sql driver associated with the connector.
//   - plans: parsed connection plans containing configuration, query settings,
//     and remote targets.
//
// Output:
//   - driver.Connector: connector that iterates the normalized connection plans.
//   - error: error creating the connector when plans or connection functions are invalid.
func buildOracleConnector(driver driver.Driver, plans *oracleconfig.ConnectionPlans) (driver.Connector, error) {
	// Wire the plan-based network opener and the existing instantiator factory
	// into one connector. Propagate constructor validation errors to openConnector.
	conn, err := newOracleConnector(
		plans,
		session.ConnectToAttempt,
		drv.GetConnectionInstantiator,
	)
	if err != nil {
		return nil, err
	}
	conn.(*connector).driver = driver
	return conn, nil
}

// newOracleConnector creates a connector from its normalized plans and connection functions.
//
// Input:
//   - plans: parsed configuration and target alternatives used during Connect.
//   - connCreator: function that opens a network session for a resolved attempt.
//   - connInstantiatorFactory: function that creates the database connection
//     instantiator after the network session is open.
//
// Output:
//   - driver.Connector: connector holding the supplied plan and functions.
//   - error: internal configuration error if a required input is nil.
func newOracleConnector(plans *oracleconfig.ConnectionPlans, connCreator ConnCreator, connInstantiatorFactory ConnInstantiatorFactory) (driver.Connector, error) {
	if plans == nil || plans.BaseConfig == nil || connCreator == nil || connInstantiatorFactory == nil {
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}
	return &connector{
		connectionPlans:         plans,
		connCreator:             connCreator,
		connInstantiatorFactory: connInstantiatorFactory,
		providerRegistry:        common.NewSafeRegistry[oracleProviders.Provider](),
	}, nil
}

// Connect establishes a new database/sql/driver.Conn to Oracle.
// It opens a wire-level Oracle connection, performs authentication, and configures the session.
func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	var err error
	var ns driverCommon.NetworkSession
	var savedErr error
	var savedAttempt *oracleconfig.ConnectionAttempt
	attempted := false
	isNsConnected := false
	isConnectionEstablished := false

	localizationService := common.NewLocalizationService(c.connectionPlans.BaseConfig.Locale.ClientLanguage)
	sessionUid, err := session.GenUUID()
	if err != nil {
		return nil, localizationService.LocalizeError(common.NewOracleError(oracleErrors.ConnectFailed, err))
	}

	common.OdlT(sessionUid).Log(common.BackgroundContext, common.OlFine, "Connecting to Oracle")

	iterator, err := c.connectionPlans.NewConnectionAttemptIterator(ctx, sessionUid)
	if err != nil {
		return nil, localizationService.LocalizeError(common.NewOracleError(oracleErrors.ConnectFailed, err))
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

	for {
		// Fetch the next fully resolved target. EOF means every description and
		// retry pass has failed; another error means resolution or waiting failed.
		attempt, nextErr := iterator.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			savedErr = nextErr
			break
		}
		attempted = true
		if err := ctx.Err(); err != nil {
			e := common.NewOracleError(oracleErrors.ConnectFailed, err)
			return nil, localizationService.LocalizeError(e)
		}
		// Bound transport setup by this attempt's effective connect timeout while
		// retaining the caller's cancellation and deadline.
		attemptCtx := ctx
		var cancel context.CancelFunc
		if timeout := attempt.GetConnectTimeout(); timeout > 0 {
			attemptCtx, cancel = context.WithTimeoutCause(ctx, timeout,
				common.NewCtxTimeoutCauseError("ConnectTimeout", uint(timeout/time.Millisecond), sessionUid))
		}
		if common.OdlT(sessionUid).Enabled(context.Background(), common.OlFine) {
			common.OdlT(sessionUid).Fine("Connector.Connect",
				"transportOptions",
				attempt.GetTransportOptions())
		}
		// Open the network session using the same resolved attempt that supplied
		// the transport options and CONNECT_DATA descriptor.
		ns, err = c.connCreator(attemptCtx, attempt)
		if cancel != nil {
			cancel()
		}
		if err == nil {
			// the network session has successfully connected, it should be
			// disconnected the connection establishment fails.
			isNsConnected = true
			savedErr = nil
			savedAttempt = attempt
			break
		}
		savedErr = err
		savedAttempt = attempt
	}

	if savedErr != nil {
		if ctxTErr, ok := savedErr.(common.CtxTimeoutCauseError); ok {
			//"%s Timeout of %d for %s.(CONNECTION_ID=%s)")
			address := "unknown"
			if savedAttempt != nil {
				address = fmt.Sprintf("%s:%d", savedAttempt.GetHost(), savedAttempt.GetPort())
			}
			localized := common.NewOracleError(oracleErrors.ConnectFailed,
				common.NewOracleError(oracleErrors.ConnectTimeout, nil, ctxTErr.GetSource(),
					ctxTErr.GetValue(), address, ctxTErr.GetEmitterID()))
			return nil, localizationService.LocalizeError(localized)
		}
		e := common.NewOracleError(oracleErrors.ConnectFailed, savedErr)
		return nil, localizationService.LocalizeError(e)
	}
	if !attempted || savedAttempt == nil || ns == nil {
		err = common.NewOracleError(oracleErrors.ConnectFailed, errors.New("no connection attempts available"))
		return nil, localizationService.LocalizeError(err)
	}

	common.OdlT(sessionUid).Fine("Network session established")

	// Authenticate and initialize the database connection with the effective
	// config from the successful attempt, not the connector-wide defaults.
	connInstantiator, err := c.connInstantiatorFactory(savedAttempt.GetEffectiveConfig(), ns, c.providerRegistry)
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

// RegisterProvider adds a runtime provider that will be made available to
// future connection attempts created from this connector.
//
// Parameters:
//   - provider: the provider to append to the connector registry.
func (c *connector) RegisterProvider(provider oracleProviders.Provider) {
	c.providerRegistry.Register(provider)
}
