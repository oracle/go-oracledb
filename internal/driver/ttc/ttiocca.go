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

// tTIOcca represents the OCCA (cursorId close/cancel) TTC function request.

type tTIOcca struct {
	headerMarshaller driverCommon.Marshallable
	cursorIDs        []driverCommon.UB4 // array of cursorId IDs
}

// newOcca18 creates a new instance of OCCA message with sequence number and token.
func newOcca18() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOcca{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: occa}},
	}
}

// newOcca creates a new instance of OCCA message.
func newOcca() driverCommon.Message[driverCommon.MessageType] {
	return &tTIOcca{
		headerMarshaller: &ttiFunHeader{_funcType: occa},
	}
}

// GetMsgCode implements common.Message.
func (m *tTIOcca) GetMsgCode() driverCommon.MessageType { return TTIPFN }

// GetFuncCode returns the TTC function code associated with OCCA.
func (m *tTIOcca) GetFuncCode() driverCommon.FunctionType { return occa }

// setCursorIDs sets the cursorId ids to be closed or canceled.
func (m *tTIOcca) setCursorIDs(ids []driverCommon.UB4) { m.cursorIDs = ids }

// MarshalTo serializes the OCCA function message.
func (m *tTIOcca) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	// marshal function header
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OCCA header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OCCA")
	}

	// PTR (cursorId pointer sentinel)
	if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OCCA cursorId ptr", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OCCA")
	}

	// UB4 offset (number of cursorId ids)
	offset := driverCommon.UB4(len(m.cursorIDs))
	if err := engine.MarshalUB4(ctx, offset); err != nil {
		common.Odl.Warn("Error marshalling OCCA offset", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OCCA")
	}

	// UB4 cursorId repeated offset times
	for i := 0; i < int(offset); i++ {
		if err := engine.MarshalUB4(ctx, m.cursorIDs[i]); err != nil {
			common.Odl.Warn("Error marshalling OCCA cursorID", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "OCCA")
		}
	}

	return nil
}
