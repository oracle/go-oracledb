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

// ttiSPFOCOspid OS PID for MTS connection
// This message is received from the server and therefore only implements
// UnMarshalFrom and GetMsgCode; it does not support MarshalTo.
type ttiSPFOCOspid struct {
}

// ttiSPFOCOspid allocates a new receiver for TTISPF/OCOSPID payloads.
// The returned value implements common.Message and is intended to be populated
// via UnMarshalFrom by the MessageStreamer.
func newttiSPFOCOspid() driverCommon.Message[driverCommon.MessageType] {
	return &ttiSPFOCOspid{}
}

// GetMsgCode implements common.Message and identifies this message as TTISPF
// (Server-side piggyback).
func (spf *ttiSPFOCOspid) GetMsgCode() driverCommon.MessageType {
	return TTISPF
}

// GetFuncCode returns the piggyback function code associated with this message.
// For OCOSPID it is ocospid (see ttimsgconst.go).
func (spf *ttiSPFOCOspid) GetFuncCode() driverCommon.FunctionType {
	return driverCommon.FunctionType(ocospid)
}

// UnMarshalFrom reads a TTISPF/OCOSPID payload from the wire.
// Expected layout (as observed from network traces):
func (spf *ttiSPFOCOspid) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	common.Odl.Debug("Unmarshalling TTISPF/OCOSPID")
	nbOfDtys, err := engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback nbOfDtys", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OCOSPID")
	}

	_, err = engine.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback lengthOfDty", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OCOSPID")
	}

	_, err = engine.UnmarshalB1Array(ctx, int(nbOfDtys))
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback ospid", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OCOSPID")
	}

	return nil
}
