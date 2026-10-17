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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type sessionState driverCommon.UB8

const (
	// sessionStateRequestBegin marks the start of a transaction request.
	sessionStateRequestBegin sessionState = 0x04
	// sessionStateRequestEnd marks the end of a transaction request.
	sessionStateRequestEnd sessionState = 0x08
	// marks the current request boundary as being explicit
	sessionStateExplBound sessionState = 0x40
)

// tTIOsessstate represents an OSESSSTATE piggyback carrying request-boundary flags.
type tTIOsessstate struct {
	// headerMarshaller writes the OSESSSTATE TTIPFN function header.
	headerMarshaller driverCommon.Marshallable
	// msgCode identifies this message as a piggyback function.
	msgCode driverCommon.MessageType
	// flags contains OSESSSTATE request-boundary operations.
	flags driverCommon.UB8
}

// newOSessStatePfn creates a pre-TTC-18 OSESSSTATE piggyback message.
func newOSessStatePfn() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOsessstate{
		headerMarshaller: &ttiFunHeader{_funcType: oSessState},
		msgCode:          TTIPFN,
	}
}

// newOSessStatePfn18 creates a TTC-18 OSESSSTATE piggyback message.
func newOSessStatePfn18() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOsessstate{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oSessState}},
		msgCode:          TTIPFN,
	}
}

// GetMsgCode returns the TTC message category used for this OSESSSTATE request.
func (m *tTIOsessstate) GetMsgCode() driverCommon.MessageType { return m.msgCode }

// GetFuncCode returns the OSESSSTATE function code.
func (m *tTIOsessstate) GetFuncCode() driverCommon.FunctionType { return oSessState }

// MarshalTo serializes the OSESSSTATE header and its UB8 request-boundary flags.
func (m *tTIOsessstate) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OSESSSTATE header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	if err := engine.MarshalUB8(ctx, m.flags); err != nil {
		common.Odl.Warn("Error marshalling OSESSSTATE flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	return nil
}
