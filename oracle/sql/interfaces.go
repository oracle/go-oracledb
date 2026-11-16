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

package sql

import (
	"time"

	"github.com/oracle/go-oracledb/v26/oracle/datatype/json"
)

// EndUserSecurityContext contains the identity and authorization data used to
// establish a database end-user security context. The database access token is
// required. A separate end-user token or name is optional, and the two are
// mutually exclusive. When both are absent, the database determines the identity
// from the database access token, including an end user it acts on behalf of.
//
// Tokens and names must be valid UTF-8. Applicable expiration times must be
// non-zero and in the future when selected and used. Implementations must keep
// returned values immutable while the driver uses the context. Attribute values
// must be JSON objects.
type EndUserSecurityContext interface {
	// DatabaseAccessToken returns the token that authorizes the application to
	// access the database.
	//
	// Parameters: None.
	//
	// Returns:
	//   - []byte: The required database access token.
	//
	// Errors: None.
	DatabaseAccessToken() []byte

	// DatabaseAccessTokenExpiration returns the non-zero expiration time of the
	// database access token.
	//
	// Parameters: None.
	//
	// Returns:
	//   - time.Time: The database access token's expiration time.
	//
	// Errors: None.
	DatabaseAccessTokenExpiration() time.Time

	// EndUserToken returns the optional separate end-user token.
	//
	// Parameters: None.
	//
	// Returns:
	//   - []byte: The end-user token, or nil when no separate token is present.
	//   - bool: True when a separate end-user token is present.
	//
	// Errors: None.
	EndUserToken() ([]byte, bool)

	// EndUserTokenExpiration returns the non-zero expiration time of the end-user
	// token. The value is ignored when EndUserToken returns false.
	//
	// Parameters: None.
	//
	// Returns:
	//   - time.Time: The separate end-user token's expiration time, if applicable.
	//
	// Errors: None.
	EndUserTokenExpiration() time.Time

	// EndUserName returns the optional named end user.
	//
	// Parameters: None.
	//
	// Returns:
	//   - string: The end-user name, or "" when no name is selected.
	//   - bool: True when the context identifies the end user by name.
	//
	// Errors: None.
	EndUserName() (string, bool)

	// DataRoles returns application-managed data roles requested for the end user.
	// The driver sorts the names and removes duplicates. This method does not
	// report all roles enabled by the database.
	//
	// Parameters: None.
	//
	// Returns:
	//   - []string: Additional role names. Nil or empty requests no additional roles.
	//
	// Errors: None.
	DataRoles() []string

	// Attributes returns JSON objects keyed by fully qualified end-user context
	// names (SCHEMA.CONTEXT_NAME).
	//
	// Parameters: None.
	//
	// Returns:
	//   - map[string]json.JSONObject: Context attributes. Nil or empty supplies none.
	//
	// Errors: None.
	Attributes() map[string]json.JSONObject

	// LookUpKey returns an optional UTF-8 end-user context identifier. For a
	// name-based principal, it identifies the attributes associated with the
	// context. For token-based principals, a returned key may differ from the
	// key used by a particular database version.
	//
	// Parameters: None.
	//
	// Returns:
	//   - []byte: The lookup key, or nil when absent.
	//   - bool: True when a lookup key is present.
	//
	// Errors: None.
	LookUpKey() ([]byte, bool)
}

// DeepDataSecurity manages the end-user security context for a connection,
// including its statements and transactions.
type DeepDataSecurity interface {
	// SetEndUserSecurityContext selects a context for subsequent operations.
	//
	// Parameters:
	//   - context: Non-nil context. Keep its values immutable while selected.
	//
	// Returns:
	//   - error: Nil on success.
	//
	// Errors: The context is invalid or expired, or the connection cannot use
	// Deep Data Security. Payload encoding and size errors are returned when a
	// database call is sent.
	SetEndUserSecurityContext(context EndUserSecurityContext) error

	// ClearEndUserSecurityContext clears the selection without making a database call.
	//
	// Parameters: None.
	//
	// Returns: None.
	//
	// Errors: None.
	ClearEndUserSecurityContext()
}
