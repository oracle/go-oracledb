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
	"reflect"
	"slices"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/oson"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
	osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

// deepDataSecurity holds connection-wide end-user security context selection.
type deepDataSecurity struct {
	connection *connection
	mu         sync.Mutex
	context    osql.EndUserSecurityContext
}

// SetEndUserSecurityContext validates context and selects it for subsequent
// database calls on this connection.
func (c *connection) SetEndUserSecurityContext(selectedCtx osql.EndUserSecurityContext) error {
	if c._isClosed || !c._isValid {
		return c.shelf.LocalizeError(common.NewOracleError(oracleErrors.InvalidConnection, nil))
	}
	if selectedCtx == nil {
		common.Odl.Debug("Deep Data Security: nil context")
		return c.shelf.LocalizeError(common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil))
	}

	// tls enforcement is not implemented yet
	if err := c.checkDeepDataSecuritySupport(); err != nil {
		return c.shelf.LocalizeError(err)
	}

	// reject invalid values before replacing the current selection
	if _, err := validateEndUserSecurityContext(selectedCtx); err != nil {
		return c.shelf.LocalizeError(err)
	}
	state := c.shelf._deepDataSecurity
	state.mu.Lock()
	state.context = selectedCtx
	state.mu.Unlock()
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security: explicit context selected")
	}
	return nil
}

// ClearEndUserSecurityContext stops propagating the selected context.
// No database call is made.
func (c *connection) ClearEndUserSecurityContext() {
	state := c.shelf._deepDataSecurity
	if state == nil {
		return
	}
	state.mu.Lock()
	// clearing the explicit context allows the provider to be consulted again
	state.context = nil
	state.mu.Unlock()
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security: explicit context cleared")
	}
}

// checkDeepDataSecuritySupport rejects servers without context propagation support.
func (c *connection) checkDeepDataSecuritySupport() error {
	if !c.shelf.GetCapabilities()[kpccapCtbBackportFeature1Xsctx].IsSet {
		return common.NewOracleError(oracleErrors.DeepDataSecurityNotSupported, nil)
	}
	return nil
}

// selectContext returns the explicit context or asks the registered provider.
func (state *deepDataSecurity) selectContext(ctx context.Context) (osql.EndUserSecurityContext, error) {
	// an explicitly selected context takes precedence over the provider
	state.mu.Lock()
	selected := state.context
	state.mu.Unlock()
	if selected != nil {
		return selected, nil
	}

	// call the provider outside the lock so it can obtain request data
	registry := state.connection.shelf.getProviderRegistry()
	if registry == nil {
		return nil, nil
	}
	provider, err := registry.Get(reflect.TypeOf((*providers.DeepDataSecurityProvider)(nil)).Elem())
	if err != nil {
		return nil, common.NewOracleError(oracleErrors.InternalError, err)
	}
	if provider == nil {
		return nil, nil
	}
	selected, err = provider.(providers.DeepDataSecurityProvider).EndUserSecurityContext(ctx)
	if err != nil {
		common.Odl.Debug("Deep Data Security: provider failed")
		return nil, common.NewOracleError(oracleErrors.InternalError, err)
	}
	return selected, nil
}

// prepare encodes the selected context before any bytes of a database call
// are written. Without a selection, the call has no security context.
func (state *deepDataSecurity) prepare(ctx context.Context) (*ttiOXSCTX, error) {
	selected, err := state.selectContext(ctx)
	if err != nil || selected == nil {
		return nil, err
	}

	// check support for provider contexts as well as explicit contexts
	if err := state.connection.checkDeepDataSecuritySupport(); err != nil {
		return nil, err
	}
	payload, err := encodeEndUserSecurityContext(selected)
	if err != nil {
		return nil, err
	}
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security: full context prepared")
	}
	// attach the complete encoded context as the piggyback value
	message, err := state.connection.shelf.GetMessageFactory().GetMessageForFunction(TTIPFN, oXsctx)
	if err != nil {
		return nil, err
	}
	msg := message.(*ttiOXSCTX)
	msg.pairs = []*keywordValuePairWithName{
		newKeywordValuePairWithName("ORCL_XS_AUTHZ_CONTEXT", "", payload, 0),
	}
	return msg, nil
}

// validatedEndUserSecurityContext holds the values checked for one use.
type validatedEndUserSecurityContext struct {
	accessToken  []byte
	endUserToken []byte
	tokenBased   bool
	endUserName  string
	nameBased    bool
	lookupKey    []byte
	hasKey       bool
	roles        []string
	attributes   []any
}

// validateEndUserSecurityContext reads and validates the context for one use.
func validateEndUserSecurityContext(context osql.EndUserSecurityContext) (*validatedEndUserSecurityContext, error) {
	// capture mutable values once so validation and encoding use the same data
	accessToken := slices.Clone(context.DatabaseAccessToken())
	endUserToken, tokenBased := context.EndUserToken()
	endUserToken = slices.Clone(endUserToken)
	endUserName, nameBased := context.EndUserName()
	lookupKey, hasKey := context.LookUpKey()
	lookupKey = slices.Clone(lookupKey)

	// validate credentials
	if len(accessToken) == 0 || !utf8.Valid(accessToken) {
		common.Odl.Debug("Deep Data Security: invalid database access token")
		return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
	}
	if (tokenBased && nameBased) || (tokenBased && (len(endUserToken) == 0 || !utf8.Valid(endUserToken))) || (nameBased && (endUserName == "" || !utf8.ValidString(endUserName))) {
		common.Odl.Debug("Deep Data Security: invalid end-user principal")
		return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
	}

	// check expiration before each use because tokens may expire after selection
	now := time.Now()
	expires := context.DatabaseAccessTokenExpiration()
	if !now.Before(expires) {
		common.Odl.Debug("Deep Data Security: database access token expired")
		return nil, common.NewOracleError(oracleErrors.ExpiredToken, nil)
	}
	if tokenBased {
		userExpires := context.EndUserTokenExpiration()
		if !now.Before(userExpires) {
			common.Odl.Debug("Deep Data Security: end-user token expired")
			return nil, common.NewOracleError(oracleErrors.ExpiredToken, nil)
		}
	}

	if nameBased && hasKey && len(lookupKey) == 0 {
		common.Odl.Debug("Deep Data Security: invalid lookup key")
		return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
	}

	// normalize roles without changing the context's slice
	roles := slices.Clone(context.DataRoles())
	slices.Sort(roles)
	roles = slices.Compact(roles)
	for _, role := range roles {
		if role == "" || !utf8.ValidString(role) {
			common.Odl.Debug("Deep Data Security: invalid data role")
			return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
		}
	}

	// retain the decoded values used by the payload so each object is read once
	attributes := context.Attributes()
	attributeNames := make([]string, 0, len(attributes))
	for name := range attributes {
		attributeNames = append(attributeNames, name)
	}
	slices.Sort(attributeNames)
	values := make([]any, 0, len(attributeNames))
	for _, name := range attributeNames {
		if name == "" || !utf8.ValidString(name) {
			common.Odl.Debug("Deep Data Security: invalid attribute name")
			return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
		}
		value, err := attributes[name].GetValue()
		if err != nil {
			common.Odl.Debug("Deep Data Security: cannot read attributes")
			return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, err)
		}
		values = append(values, map[string]any{"name": name, "values": value})
	}
	return &validatedEndUserSecurityContext{
		accessToken: accessToken, endUserToken: endUserToken, tokenBased: tokenBased,
		endUserName: endUserName, nameBased: nameBased, lookupKey: lookupKey, hasKey: hasKey,
		roles: roles, attributes: values,
	}, nil
}

// encodeEndUserSecurityContext encodes a validated context using the database's OSON schema.
func encodeEndUserSecurityContext(context osql.EndUserSecurityContext) ([]byte, error) {
	validated, err := validateEndUserSecurityContext(context)
	if err != nil {
		return nil, err
	}

	// add the principal and optional fields expected by the database
	payload := map[string]any{"ver": "1.0", "database_access_token": string(validated.accessToken)}
	if validated.tokenBased {
		payload["end_user_token"] = string(validated.endUserToken)
	}
	if validated.nameBased {
		payload["end_user_name"] = validated.endUserName
		if validated.hasKey {
			payload["end_user_contextid"] = string(validated.lookupKey)
		}
	}
	if len(validated.roles) > 0 {
		roles := make([]any, len(validated.roles))
		for index, role := range validated.roles {
			roles[index] = role
		}
		payload["data_roles"] = roles
	}
	if len(validated.attributes) > 0 {
		payload["attributes"] = validated.attributes
	}

	encoded, err := oson.Encode(payload)
	if err != nil {
		common.Odl.Debug("Deep Data Security: cannot encode context")
		return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, err)
	}

	// reject oversized payloads before they reach the protocol writer
	if len(encoded) > maxXsctxValueLength {
		common.Odl.Debug("Deep Data Security: context exceeds 65536 bytes")
		return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
	}
	return encoded, nil
}
