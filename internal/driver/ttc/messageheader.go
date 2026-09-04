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
	"fmt"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type messageHeader struct { // we can argue of the need for interface as messageType is quite fix
	messageType  driverCommon.MessageType
	functionType driverCommon.FunctionType
}

func (mh *messageHeader) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	// messageType also implement the "marshaller" interface
	var err error
	msgType, err := engine.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("An error occurred while unmarshalling the message type", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "header")
	}

	mh.messageType = driverCommon.MessageType(msgType)
	if !isValid(mh.messageType) {
		common.Odl.Warn("Invalid header message type received", "message type", mh.messageType)
		return common.NewOracleError(oracleErrors.ProtocolViolation, nil)
	}

	if isFunction(mh.messageType) {
		funcType, err := engine.UnmarshalUB1(ctx)
		if err != nil {
			common.Odl.Warn("An error occurred while unmarshalling the function type", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, "function type")
		}
		mh.functionType = driverCommon.FunctionType(funcType)
	}
	return nil
}

func (mh *messageHeader) String() string {
	return fmt.Sprintf("messageHeader{type=[%s]}", toString(mh.messageType))
}

func (mh *messageHeader) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	if err := engine.MarshalUB1(ctx, driverCommon.UB1(mh.messageType)); err != nil {
		return err
	}
	return nil
}

func (mh *messageHeader) GetType() driverCommon.MessageType {
	return mh.messageType
}
