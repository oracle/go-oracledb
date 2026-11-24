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
	"crypto/sha256"
	"encoding/base64"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"unicode/utf16"

	"github.com/oracle/go-oracledb/v26/internal/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/oson"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/providers"
	osql "github.com/oracle/go-oracledb/v26/oracle/sql"
)

// deepDataSecurity holds connection-wide end-user security context selection.
type deepDataSecurity struct {
	connection      *connection
	context         osql.EndUserSecurityContext
	hashSupported   bool
	previousContext *[sha256.Size]byte
}

// newDeepDataSecurity initializes the connection state from the authentication response.
func newDeepDataSecurity(conn *connection) *deepDataSecurity {
	dds := &deepDataSecurity{connection: conn}
	if flag, err := conn.sessCtx.GetSessionProperties().GetTrimmedString("AUTH_CLIENT_EUSC_FLAG"); err == nil {
		if value, err := strconv.Atoi(flag); err == nil {
			dds.hashSupported = value&0x01 != 0
		}
	}
	return dds
}

// SetEndUserSecurityContext validates context and selects it for subsequent
// database calls on this connection.
func (c *connection) SetEndUserSecurityContext(selectedCtx osql.EndUserSecurityContext) error {
	if c._isClosed || !c._isValid {
		return c.shelf.LocalizeError(common.NewOracleError(oracleErrors.InvalidConnection, nil))
	}

	if err := c.checkDeepDataSecuritySupport(); err != nil {
		return c.shelf.LocalizeError(err)
	}

	// reject invalid values before replacing the current selection
	if !isValidEndUserSecCtx(selectedCtx) {
		return c.shelf.LocalizeError(common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil))
	}

	// copy caller-owned roles and attributes while sharing read-only json values
	selectedCtx.DataRoles = slices.Clone(selectedCtx.DataRoles)
	selectedCtx.Attributes = maps.Clone(selectedCtx.Attributes)
	c.shelf._deepDataSecurity.context = selectedCtx
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security: explicit context selected")
	}
	return nil
}

// ClearEndUserSecurityContext stops propagating the selected context.
// No database call is made.
func (c *connection) ClearEndUserSecurityContext() {
	c.clearEndUserSecurityContext()
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security: explicit context clear requested")
	}
}

// clearEndUserSecurityContext clears the current end user sec ctx.
func (c *connection) clearEndUserSecurityContext() {
	if c.shelf._deepDataSecurity == nil {
		return
	}

	c.shelf._deepDataSecurity.context = osql.EndUserSecurityContext{}
	c.shelf._deepDataSecurity.previousContext = nil
}

// checkDeepDataSecuritySupport requires server support and a TLS connection.
func (c *connection) checkDeepDataSecuritySupport() error {
	if !c.shelf.GetCapabilities()[kpccapCtbBackportFeature1Xsctx].IsSet {
		return common.NewOracleError(oracleErrors.DeepDataSecurityNotSupported, nil)
	}
	if !c.ns.IsTLS() {
		common.Odl.Debug("Deep Data Security: TLS connection required")
		return common.NewOracleError(oracleErrors.DeepDataSecurityRequiresTLS, nil)
	}
	return nil
}

// preparePiggyback selects a context and creates its OXSCTX message.
// It returns nil when neither an explicit context nor a provider is present.
func (dds *deepDataSecurity) preparePiggyback(ctx context.Context) (*ttiOXSCTX, *[sha256.Size]byte, error) {
	// an explicitly selected context takes precedence over the provider
	selected := dds.context
	if selected.DatabaseAccessToken == "" {
		registry := dds.connection.shelf.getProviderRegistry()
		if registry == nil {
			return nil, nil, nil
		}
		provider, err := registry.Get(reflect.TypeOf((*providers.DeepDataSecurityProvider)(nil)).Elem())
		if err != nil {
			return nil, nil, common.NewOracleError(oracleErrors.InternalError, err)
		}
		if provider == nil {
			// no explicit context or provider is available
			return nil, nil, nil
		}
		if common.Odl.Enabled(context.Background(), common.OlDebug) {
			common.Odl.Debug("Deep Data Security: calling context provider")
		}
		selected, err = provider.(providers.DeepDataSecurityProvider).EndUserSecurityContext(ctx)
		if err != nil {
			common.Odl.Debug("Deep Data Security: provider failed")
			return nil, nil, common.NewOracleError(oracleErrors.InternalError, err)
		}
	}

	// check support for provider contexts as well as explicit contexts
	if err := dds.connection.checkDeepDataSecuritySupport(); err != nil {
		return nil, nil, err
	}
	if !isValidEndUserSecCtx(selected) {
		return nil, nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, nil)
	}
	useHash := dds.hashSupported && selected.NamedUser.Username == ""
	var authHash string
	if useHash {
		authHash = secCtxHash(selected)
	}

	// compare the full payload before selecting the smaller representation
	fullPayload, err := encodeEndUserSecCtx(selected, true)
	if err != nil {
		return nil, nil, err
	}
	fingerprint := sha256.Sum256(fullPayload)
	sendFull := !useHash || dds.previousContext == nil || *dds.previousContext != fingerprint
	payload := fullPayload
	if !sendFull {
		payload, err = encodeEndUserSecCtx(selected, false)
		if err != nil {
			return nil, nil, err
		}
	}

	message, err := dds.connection.shelf.GetMessageFactory().GetMessageForFunction(TTIPFN, oXsctx)
	if err != nil {
		return nil, nil, err
	}
	msg := message.(*ttiOXSCTX)
	msg.pairs = nil
	if useHash {
		msg.pairs = append(msg.pairs, newKeywordValuePairWithName("ORCL_XS_AUTH_HASH", authHash, nil, 0))
	}
	msg.pairs = append(msg.pairs, newKeywordValuePairWithName("ORCL_XS_AUTHZ_CONTEXT", "", payload, 0))
	if common.Odl.Enabled(context.Background(), common.OlDebug) {
		common.Odl.Debug("Deep Data Security: context prepared", "hash enabled", useHash, "full context", sendFull)
	}
	if !useHash {
		return msg, nil, nil
	}
	return msg, &fingerprint, nil
}

// queueSecurityContext queues the context and returns a function that records the round trip outcome.
func (dds *deepDataSecurity) queueSecurityContext(ctx context.Context, streamer MessageStreamerInterface, code oracleErrors.ErrorCode) (func(bool), error) {
	if dds == nil {
		return func(bool) {}, nil
	}
	msg, pending, err := dds.preparePiggyback(ctx)
	if err != nil {
		dds.previousContext = nil
		return nil, dds.connection.shelf.LocalizeError(err)
	}
	if msg != nil {
		if err := streamer.Push(ctx, msg); err != nil {
			dds.previousContext = nil
			return nil, common.NewOracleError(code, err, "push")
		}
	}
	return func(success bool) {
		dds.previousContext = nil
		if success {
			dds.previousContext = pending
		}
	}, nil
}

// secCtxHash computes the hash of the context tokens and data roles
func secCtxHash(selected osql.EndUserSecurityContext) string {
	roles := slices.Clone(selected.DataRoles)
	// order roles as jdbc does when generating the token hash
	slices.SortFunc(roles, func(a, b string) int {
		return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
	})
	roles = slices.Compact(roles)
	hash := sha256.New()
	_, _ = hash.Write([]byte(selected.EndUserToken))
	_, _ = hash.Write([]byte(selected.DatabaseAccessToken))
	for _, role := range roles {
		_, _ = hash.Write([]byte(strconv.Itoa(len(role))))
		_, _ = hash.Write([]byte(":"))
		_, _ = hash.Write([]byte(role))
	}
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// isValidEndUserSecCtx checks the fields required for a security context.
func isValidEndUserSecCtx(selected osql.EndUserSecurityContext) bool {
	// the database access token is mandatory
	if selected.DatabaseAccessToken == "" {
		return false
	}

	// the end-user token and named user are mutually exclusive
	if selected.EndUserToken != "" && selected.NamedUser.Username != "" {
		return false
	}

	// a lookup key requires a name
	if selected.NamedUser.LookupKey != "" && selected.NamedUser.Username == "" {
		return false
	}

	// reject tokens only when a readable expiration has passed
	if validateJWTExpiration(selected.DatabaseAccessToken) != nil || validateJWTExpiration(selected.EndUserToken) != nil {
		return false
	}

	return true
}

// encodeEndUserSecCtx encodes a previously validated security context as OSON.
func encodeEndUserSecCtx(secCtx osql.EndUserSecurityContext, full bool) ([]byte, error) {
	// prepare the payload map with the required fields
	payload := map[string]any{"ver": "1.0"}

	if full {
		payload["database_access_token"] = secCtx.DatabaseAccessToken
		if secCtx.EndUserToken != "" {
			payload["end_user_token"] = secCtx.EndUserToken
		}
		if secCtx.NamedUser.Username != "" {
			payload["end_user_name"] = secCtx.NamedUser.Username
			if secCtx.NamedUser.LookupKey != "" {
				payload["end_user_contextid"] = secCtx.NamedUser.LookupKey
			}
		}

		// sort and deduplicate a copy to preserve the selected roles
		if len(secCtx.DataRoles) > 0 {
			roles := slices.Clone(secCtx.DataRoles)
			slices.Sort(roles)
			roles = slices.Compact(roles)
			values := make([]any, len(roles))
			for i, role := range roles {
				values[i] = role
			}
			payload["data_roles"] = values
		}
	}

	// prepare attributes in a stable order
	if len(secCtx.Attributes) > 0 {
		names := slices.Sorted(maps.Keys(secCtx.Attributes))
		values := make([]any, len(names))
		for i, name := range names {
			value, err := secCtx.Attributes[name].GetValue()
			if err != nil {
				return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, err)
			}
			values[i] = map[string]any{"name": name, "values": value}
		}
		payload["attributes"] = values
	}

	// encode the payload as OSON
	encoded, err := oson.Encode(payload)
	if err != nil {
		common.Odl.Debug("Deep Data Security: cannot encode context")
		return nil, common.NewOracleError(oracleErrors.InvalidEndUserSecurityContext, err)
	}

	return encoded, nil
}
