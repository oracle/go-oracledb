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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/session"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type connectionInstantiator struct {
	negotiator           Negotiator
	authenticator        Authenticator
	dataBuffer           driverCommon.DataBuffer
	ns                   driverCommon.NetworkSession
	connectionProperties *oracleconfig.OracleDriverProperties
	newConnection        func(context.Context, *ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext, driverCommon.NetworkSession) (*Connection, error)
	localizationService  common.LocalizationService
}

// NewTTCConnectionInstantiator creates a new TTC connection instantiator
func NewTTCConnectionInstantiator(config *oracleconfig.OracleDriverConfig, ns *session.NetworkSession) (driverCommon.ConnectionInstantiator, error) {
	dataBuffer := driverCommon.DataBuffer(ns)
	negotiator := GetNegotiator(dataBuffer)
	localizationService := common.NewLocalizationService(config.Locale.ClientLanguage)

	authenticator, err := GetAuthenticator(config)
	if err != nil {
		return nil, err
	}
	return &connectionInstantiator{
		negotiator:           negotiator,
		authenticator:        authenticator,
		dataBuffer:           dataBuffer,
		ns:                   ns,
		connectionProperties: &config.DriverProperties,
		newConnection:        NewConnection,
		localizationService:  localizationService,
	}, nil
}

// GetConnection Returns a TTC connection to the database that implements "driver.Conn".
func (connInstantiator *connectionInstantiator) GetConnection(ctx context.Context) (driver.Conn, error) {
	if connInstantiator.localizationService == nil {
		return nil, common.NewOracleError(oracleErrors.MissingLocalizationService, nil)
	}

	// Perform wire negotiation; capture session context, shelf and capability
	sessCtx, shelf, err := connInstantiator.negotiator.Negotiate(ctx)
	if err != nil {
		return nil, connInstantiator.localizationService.LocalizeError(err)
	}

	remoteAddsProperty := driverCommon.NewProperties[string]()
	remoteAddsProperty.SetProperty("REMOTE_ADDRESS", connInstantiator.ns.GetRemoteAddress())
	sessCtx.UpdateSessionProperties(remoteAddsProperty)

	// Add connection properties to shelf for downstream consumers
	shelf.UpdateConnectionProperties(connInstantiator.connectionProperties)
	shelf.RegisterLocalizationService(connInstantiator.localizationService)

	// register server to client piggyback callbacks
	registerServerToClientPiggybacks(shelf, sessCtx)

	// Initialise authenticator
	if authenticator, ok := connInstantiator.authenticator.(ttiShelfUser); ok {
		authenticator.SetShelf(shelf)
	}
	if sessionContextUser, ok := connInstantiator.authenticator.(SessionContextUser); ok {
		sessionContextUser.SetSessionContext(sessCtx)
	}

	// Authenticate
	err = connInstantiator.authenticator.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	// snapshot session
	sessCtx.GetSessionProperties().Snapshot()

	// conn = drv.NewConnection(ctx, *c.config, sessCtx, shelf)
	common.Odl.Debug("Connect end")
	newConnection := connInstantiator.newConnection
	if newConnection == nil {
		newConnection = NewConnection
	}
	return newConnection(ctx, shelf, sessCtx, connInstantiator.ns)
}

// *** Authenticator Factory ***

// GetAuthenticator gets an authenticator suitable to parameters
// Parameters:
//   - parameters: the parameters used to get the authenticator
//
// Returns:
//   - an authenticator
//   - error if no candidate can be found
func GetAuthenticator(parameters *oracleconfig.OracleDriverConfig) (Authenticator, error) {
	common.Odl.Debug("New authenticator requested for", "parameters", parameters)

	if parameters == nil {
		// this should never happen
		return nil, common.NewOracleError(oracleErrors.InternalError, nil)
	}

	if parameters.Credentials.TokenAuthentication.IsValid() {
		return createTokenAuthenticator(parameters)
	}

	if len(parameters.Credentials.Password) > 0 {
		if len(parameters.Credentials.User) == 0 {
			return nil, common.NewOracleError(oracleErrors.EmptyUsernameError, nil, nil)
		}
		return createPasswordAuthenticator(parameters)
	}
	return nil, common.NewOracleError(oracleErrors.NoAuthenticatorError, nil, nil)
}

func createPasswordAuthenticator(parameters *oracleconfig.OracleDriverConfig) (Authenticator, error) {
	if len(parameters.Credentials.LogonMode) > 0 {
		var mode, _ = common.GetLogonModeFromString(parameters.Credentials.LogonMode)
		common.Odl.Debug(fmt.Sprintf("logonMode set to [%s]", mode))
		return NewPasswordAuthenticatorWithLogonMode(parameters.Credentials.User,
			parameters.Credentials.Password,
			mode,
			parameters.ConnectDescriptor), nil
	}
	return NewPasswordAuthenticator(parameters.Credentials.User,
		parameters.Credentials.Password,
		parameters.ConnectDescriptor), nil
}

func createTokenAuthenticator(parameters *oracleconfig.OracleDriverConfig) (Authenticator, error) {
	return NewTokenAuthenticator(
		parameters.Credentials.TokenAuthentication,
		parameters.Credentials.AccessToken,
		parameters.Credentials.TokenLocation,
		parameters.ConnectDescriptor,
	), nil
}

// *** Negotiator Factory ***

// GetNegotiator returns the default Negotiator implementation.
// TODO : we should not have to pass the buffer here...
func GetNegotiator(transport driverCommon.DataBuffer) Negotiator {
	n := NewConnectionNegotiator()
	n.SetDataBuffer(transport)
	return n
}
