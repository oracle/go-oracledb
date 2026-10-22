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

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// Authenticator implementors authenticate against a remote database
type Authenticator interface {
	// Authenticate performs authentication.
	// return error on failure. One of the ORACode
	Authenticate(context.Context) error
}

// ShelfUser implementors need the shelf to execute their actions
type ShelfUser interface {
	// SetShelf sets the shelf
	SetShelf(*common.Shelf[common.MessageType])
}

// SessionContextUser implementors need the session context to execute their actions
type SessionContextUser interface {
	// SetSessionContext sets the session context
	SetSessionContext(*common.SessionContext)
}

// Negotiator defines the interface for connection negotiation.
type Negotiator interface {
	// Negotiate negotiates a connection
	// returns:
	//  - a session context initialized for the remote server
	//  - a shelf that host connection common assets
	//  - error in case of failure
	Negotiate(context.Context) (*common.SessionContext,
		*ttiShelf[common.MessageType], error)
}

// codecFactoryUser identifies messages that require protocol-matched codecs.
// The message factory injects its codec factory immediately after creating a
// message through GetMessage or GetMessageForFunction, before it is used.
// Implementations do not need access to the connection shelf for codec lookup.
type codecFactoryUser interface {
	// setCodecFactory stores the codec factory for subsequent codec lookups.
	// factory uses the same TTC protocol version as the creating message factory
	// and is shared by messages created by that factory.
	setCodecFactory(factory codecFactory)
}

// messageFactoryUser identifies messages that create nested TTC messages while
// unmarshalling. The message factory injects itself immediately after creating
// a message so nested messages use the same negotiated protocol version.
// Implementations do not need access to the connection shelf for message
// allocation.
type messageFactoryUser interface {
	// setMessageFactory stores the factory used to allocate nested messages.
	setMessageFactory(factory Factory)
}
