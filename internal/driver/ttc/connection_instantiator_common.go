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

package ttc

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

// connectionInstantiatorCommon holds the dependencies shared by normal and TTIINIT setup.
type connectionInstantiatorCommon struct {
	negotiator          Negotiator
	authenticator       Authenticator
	ns                  driverCommon.NetworkSession
	driverConfig        *oracleconfig.OracleDriverConfig
	newConnectionFunc   func(context.Context, *ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext, driverCommon.NetworkSession) (*connection, error)
	localizationService common.LocalizationService
	providerRegistry    common.Registry[oracleProviders.Provider]
}

func newConnectionInstantiatorCommon(config *oracleconfig.OracleDriverConfig, ns driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider], negotiator Negotiator, authenticator Authenticator) *connectionInstantiatorCommon {
	return &connectionInstantiatorCommon{
		ns:                  ns,
		driverConfig:        config,
		newConnectionFunc:   newConnection,
		localizationService: common.NewLocalizationService(config.Locale.ClientLanguage),
		providerRegistry:    providerRegistry,
		negotiator:          negotiator,
		authenticator:       authenticator,
	}
}

func (shared *connectionInstantiatorCommon) prepareForAuthentication(sessCtx *driverCommon.SessionContext, shelf *ttiShelf[driverCommon.MessageType]) {
	// After the session context is created, set client properties that will be needed
	// by the client for authentication
	sessCtx.GetClientProperties().SetProperty(driverCommon.RemoteAddress, shared.ns.GetRemoteAddress())
	sessCtx.GetClientProperties().SetProperty(driverCommon.RemotePort, shared.ns.GetRemotePort())
	sessCtx.GetClientProperties().SetProperty(driverCommon.ConnectDescriptor, shared.driverConfig.ConnectDescriptor)

	// Register connection services for downstream consumers
	shelf.RegisterLocalizationService(shared.localizationService)
	shelf.registerProviderRegistry(shared.providerRegistry)

	// register server to client piggyback callbacks
	registerServerToClientPiggybacks(shelf, sessCtx)

	// Initialise authenticator
	if authenticator, ok := shared.authenticator.(ttiShelfUser); ok {
		authenticator.SetShelf(shelf)
	}
	if sessionContextUser, ok := shared.authenticator.(SessionContextUser); ok {
		sessionContextUser.SetSessionContext(sessCtx)
	}
}

func (shared *connectionInstantiatorCommon) finishConnection(ctx context.Context, shelf *ttiShelf[driverCommon.MessageType], sessCtx *driverCommon.SessionContext) (driver.Conn, error) {
	// snapshot session
	sessCtx.GetSessionProperties().Snapshot()

	common.Odl.Finest("Create the connection")
	conn, err := shared.newConnectionFunc(ctx, shelf, sessCtx, shared.ns)
	if err != nil {
		return nil, err
	}

	// Build and register request-boundary callbacks for the connection.
	common.Odl.Finest("Build and register request-boundary callbacks")
	callbacks := buildRequestBoundaryCallbacks(conn)
	conn.initializeRequestBoundaryCallbacks(callbacks.requestStartCallbacks, callbacks.requestEndCallbacks)

	// Run connection-initialization callbacks
	common.Odl.Finest("Running connection initialization callbacks")
	if err := runRequestBoundaryCallbacks(ctx, callbacks.newConnectionCallbacks); err != nil {
		return nil, err
	}

	common.Odl.Debug("Connect end")
	return conn, nil
}

// buildRequestBoundaryCallbacks builds request-boundary callbacks for the connection.
func buildRequestBoundaryCallbacks(conn *connection) *requestCallbacks {
	callbacks := newRequestBoundaryCallbacks()

	// Add end-request callback that rolls back active transaction.
	addRollbackEndRequestCallback(callbacks, conn)
	// Add begin-request and end-request callbacks.
	addBeginEndRequestCallbacks(callbacks, conn)

	return callbacks
}

// *** Negotiator Factory ***

// GetNegotiator selects normal or TTIINIT negotiation.
// serverProPending indicates a pending server TTIPRO during normal fallback.
func GetNegotiator(buffer driverCommon.DataBuffer, connectionProperties driverCommon.DriverProperties, mode driverCommon.ConnectionMode, serverProPending bool) Negotiator {
	var negotiator Negotiator
	switch mode {
	case driverCommon.ConnectionTTIInit:
		negotiator = newTTIInitConnectionNegotiator(buffer, connectionProperties)
	case driverCommon.ConnectionNormal:
		negotiator = newConnectionNegotiator(buffer, connectionProperties, serverProPending)
	}
	return negotiator
}

// *** Authenticator Factory ***

// GetAuthenticator selects the authenticator to use for the supplied driver
// configuration, provider registry, and connection mode.
//
// Parameters:
//   - parameters: the Oracle driver configuration to inspect.
//   - providerRegistry: the provider registry available to token-based authentication.
//   - mode: the connection establishment workflow.
//
// Returns:
//   - the authenticator matching the supplied configuration.
//   - an error if no suitable authenticator can be created.
func GetAuthenticator(parameters *oracleconfig.OracleDriverConfig, providerRegistry common.Registry[oracleProviders.Provider], mode driverCommon.ConnectionMode) (Authenticator, error) {
	common.Odl.Debug("New authenticator requested for", "parameters", parameters)

	if parameters == nil {
		// this should never happen
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	if len(parameters.Credentials.User) > 0 {
		if len(parameters.Credentials.Password) > 0 {
			return createPasswordAuthenticator(parameters, mode)
		}
	} else {
		if len(parameters.Credentials.Password) > 0 {
			// if there is a password but no username
			return nil, common.NewOracleError(oracleErrors.EmptyUsernameError, nil, nil)
		}
		provider, err := providerRegistry.Get(reflect.TypeOf((*oracleProviders.TokenAuthenticationProvider)(nil)).Elem())
		if err != nil {
			return nil, common.NewOracleError(oracleErrors.NoAuthenticatorError, err, nil)
		}
		if provider != nil {
			tokenProvider := provider.(oracleProviders.TokenAuthenticationProvider)
			switch mode {
			case driverCommon.ConnectionTTIInit:
				return newTTIInitTokenAuthenticator(tokenProvider), nil
			case driverCommon.ConnectionNormal:
				return newTokenAuthenticator(tokenProvider), nil
			}
		}

	}
	return nil, common.NewOracleError(oracleErrors.NoAuthenticatorError, nil, nil)
}

func createPasswordAuthenticator(parameters *oracleconfig.OracleDriverConfig, connectionMode driverCommon.ConnectionMode) (Authenticator, error) {
	var logonMode common.LogonMode
	if len(parameters.Credentials.LogonMode) > 0 {
		var err error
		logonMode, err = common.GetLogonModeFromString(parameters.Credentials.LogonMode)
		if err != nil {
			return nil, err
		}
		common.Odl.Debug(fmt.Sprintf("logonMode set to [%s]", logonMode))
	}

	// Resolving the logon mode first avoids separate default and explicit logon
	// branches for each connection path.
	switch connectionMode {
	case driverCommon.ConnectionTTIInit:
		return newTTIInitPasswordAuthenticator(parameters.Credentials.User,
			parameters.Credentials.Password,
			logonMode,
			parameters.ConnectDescriptor), nil
	case driverCommon.ConnectionNormal:
		return newPasswordAuthenticator(parameters.Credentials.User,
			parameters.Credentials.Password,
			logonMode,
			parameters.ConnectDescriptor), nil
	default:
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}
}
