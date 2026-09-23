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

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// connectionWrapper provides Oracle specific operations for a dedicated
// database/sql connection.
//
// The wrapped connection must be a connection returned by this driver.
type connectionWrapper struct {
	connection *sql.Conn
}

// NewConnectionWrapper validates and wraps a dedicated database/sql connection
// for Oracle specific operations.
//
// Parameters:
//   - connection: Dedicated database/sql connection to wrap.
//
// Returns:
//   - *connectionWrapper: Wrapper for the supplied connection.
//   - error: Error if the underlying driver connection type is not supported.
func NewConnectionWrapper(connection *sql.Conn) (*connectionWrapper, error) {
	var wrapper *connectionWrapper
	err := connection.Raw(func(c any) error {
		// Include here all functions/interfaces we want a connection to implement in
		// order to be wrapped by this wrapper
		type canBeWrapped interface {
		}
		_, ok := c.(canBeWrapped)
		if !ok {
			return common.NewOracleError(oracleErrors.InvalidConnection, nil)
		}
		wrapper = &connectionWrapper{connection: connection}
		return nil
	})
	return wrapper, err
}
