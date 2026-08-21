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
	"log/slog"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// This message type will be used for all TTI functions that do not contain any
// specific data like OCOMMIT, OROLLBACK, OCOMON, OCOMOFF and OLOGOFF. The only
// thing that is marshalled is the function header.
type ttiFunHeader struct {
	_funcType driverCommon.FunctionType
	_seqNo    driverCommon.UB1
}

// ttiFunHeader18 is ttiFunHeader with sequence number and token.
type ttiFunHeader18 struct {
	*ttiFunHeader
}

// newLogOff creates a new instance of logOff message (no payload)
func newLogOff() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader{
		_funcType: logOff,
	}
}

// newLogOff18 creates a new instance of logOff18 message (no payload)
func newLogOff18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader18{
		ttiFunHeader: newLogOff().(*ttiFunHeader),
	}
}

// newPing creates a new instance of ping message (no payload)
func newPing() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader{
		_funcType: ping,
	}
}

// newPing18 creates a new instance of ping18 message (no payload)
func newPing18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader18{
		ttiFunHeader: newPing().(*ttiFunHeader),
	}
}

// newCommit creates a new instance of commit message (no payload)
func newCommit() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader{
		_funcType: commit,
	}
}

// newCommit18 creates a new instance of commit18 message (no payload)
func newCommit18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader18{
		ttiFunHeader: newCommit().(*ttiFunHeader),
	}
}

// newRollback creates a new instance of rollback message (no payload)
func newRollback() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader{
		_funcType: rollback,
	}
}

// newRollback18 creates a new instance of rollback18 message (no payload)
func newRollback18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiFunHeader18{
		ttiFunHeader: newRollback().(*ttiFunHeader),
	}
}

// TODO: create other NewXXX fuctions for other functions with no payload, these
// function should just set the correct function type.

// GetMsgCode reutrns the message code for this message
func (msg *ttiFunHeader) GetMsgCode() driverCommon.MessageType {
	return TTIFUN
}

// MarshalTo marshals the message using the given marshaller.
// Note that this message does not have any payload, only the message code is
// marshalled.
func (msg *ttiFunHeader) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	err := engine.MarshalUB1(ctx, driverCommon.UB1(msg._funcType))
	if err != nil {
		common.Odl.Debug("Error marshalling function type for function with no data of type",
			"function type", msg._funcType, "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	msg._seqNo, err = engine.(*MarshalEngine).marshalSeqNo(ctx)
	if err != nil {
		common.Odl.Warn("Error marshalling function type for function with no data of type",
			"function type", msg._funcType, "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	return nil
}

// MarshalTo marshals the message using the given marshaller.
// Note that this message does not have any payload, only the message code is
// marshalled.
func (msg *ttiFunHeader18) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	err := msg.ttiFunHeader.MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Warn("Error marshalling function type for function with no data of type",
			"function type", msg._funcType, "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	_, err = engine.(*MarshalEngine).marshalTokenNo(ctx)
	if err != nil {
		common.Odl.Warn("Error marshalling function type for function with no data of type",
			"function type", msg._funcType, "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	return nil
}

// runFunctionWithFunHeader runs a function with no token that expects TTIOER as response.
//
// Returns: error if an error occurs otherwise nil
func (c *connection) runFunctionWithFunHeader(ctx context.Context, functiontype driverCommon.FunctionType) error {
	common.Odl.Debug("Running function of type", "function type", functiontype)
	msg, err := c.shelf.GetMessageFactory().GetMessageForFunction(TTIFUN, functiontype)
	if err != nil {
		common.Odl.Warn("Error creating message", "function type", functiontype, "error", err)
		return common.NewOracleError(oracleErrors.InternalError, err)
	}
	err = c.shelf.GetMessageStreamer().Push(ctx, msg)
	if err != nil {
		common.Odl.Warn("Error pushing message", "function type", functiontype, "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}
	err = c.shelf.GetMessageStreamer().Flush(ctx)
	if err != nil {
		common.Odl.Warn("Error flushing message", "function type", functiontype, "error", err)
		return common.NewOracleError(oracleErrors.StreamerWriteError, err)
	}
	retMsg, err := c.shelf.GetMessageStreamer().Pull(ctx, TTIOER, TTISTA)
	if err != nil {
		common.Odl.Warn("Error pulling message", "function type", functiontype, "error", err)
		return common.NewOracleError(oracleErrors.StreamerReadError, err)
	}
	switch retMsg.GetMsgCode() {
	case TTIOER:
		err = retMsg.(tTIOerIface).getError()
		if err != nil {
			return err
		}
	case TTISTA:
		sta := retMsg.(*ttiSTA)
		// TODO: nothing to do with this information for now
		if common.Odl.Enabled(context.Background(), slog.LevelDebug) {
			common.Odl.Debug("Ping returned", "STA", sta)
		}
	}
	return nil
}
