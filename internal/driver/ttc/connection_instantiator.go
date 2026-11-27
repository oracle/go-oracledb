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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
)

type connectionInstantiator struct {
	*connectionInstantiatorCommon
}

// NewTTCConnectionInstantiator creates a new TTC connection instantiator.
//
// Parameters:
//   - config: the Oracle driver configuration for the connection attempt.
//   - ns: the established network session used by TTC.
//   - providerRegistry: the provider registry available to the authenticator for this attempt.
//   - serverProPending: whether negotiation starts by reading a pending server TTIPRO.
//
// Returns:
//   - the TTC connection instantiator bound to ns.
//   - an error if the authenticator or instantiator cannot be initialized.
func NewTTCConnectionInstantiator(config *oracleconfig.OracleDriverConfig, ns driverCommon.NetworkSession, providerRegistry common.Registry[oracleProviders.Provider], serverProPending bool) (driverCommon.ConnectionInstantiator, error) {
	buffer := ns.(driverCommon.DataBuffer)
	negotiator := GetNegotiator(buffer, config.DriverProperties, driverCommon.ConnectionNormal, serverProPending)
	authenticator, err := GetAuthenticator(config, providerRegistry, driverCommon.ConnectionNormal)
	if err != nil {
		return nil, err
	}

	shared := newConnectionInstantiatorCommon(config, ns, providerRegistry, negotiator, authenticator)
	return &connectionInstantiator{connectionInstantiatorCommon: shared}, nil
}

// GetConnection returns a TTC connection to the database that implements
// driver.Conn.
//
// Parameters:
//   - ctx: the context controlling negotiation, authentication, and connection setup.
//
// Returns:
//   - the established driver connection.
//   - an error if negotiation, authentication, or connection creation fails.
func (ci *connectionInstantiator) GetConnection(ctx context.Context) (driver.Conn, error) {
	if ci.localizationService == nil {
		return nil, common.NewOracleError(oracleErrors.MissingLocalizationService, nil)
	}

	// Perform wire negotiation; capture session context, shelf and capability
	sessCtx, shelf, err := ci.negotiator.Negotiate(ctx)
	if err != nil {
		return nil, ci.localizationService.LocalizeError(err)
	}

	ci.prepareForAuthentication(sessCtx, shelf)
	if err := ci.authenticator.Authenticate(ctx); err != nil {
		return nil, err
	}
	return ci.finishConnection(ctx, shelf, sessCtx)
}
