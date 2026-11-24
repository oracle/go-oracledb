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

const (
	// maxXsctxKeyLength is the maximum OXSCTX key length in bytes
	maxXsctxKeyLength = 128
	// maxXsctxValueLength is the maximum OXSCTX value length in bytes
	maxXsctxValueLength = 64 * 1024
	// xsctxProcessInit requests initialization of the security context
	xsctxProcessInit = driverCommon.UB4(1)
)

// ttiOXSCTX is a client-to-server piggyback carrying end-user security context data.
type ttiOXSCTX struct {
	headerMarshaller driverCommon.Marshallable
	pairs            []*keywordValuePairWithName
}

// newOXSCTX creates a new OXSCTX client-to-server piggyback message.
func newOXSCTX() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOXSCTX{headerMarshaller: &ttiFunHeader{_funcType: oXsctx}}
}

// newOXSCTX18 creates an OXSCTX message with the protocol 18 function token.
func newOXSCTX18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOXSCTX{headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oXsctx}}}
}

// GetMsgCode returns the client-to-server piggyback message code.
func (msg *ttiOXSCTX) GetMsgCode() driverCommon.MessageType { return TTIPFN }

// GetFuncCode returns the OXSCTX TTC function code.
func (msg *ttiOXSCTX) GetFuncCode() driverCommon.FunctionType { return oXsctx }

// MarshalTo serializes the OXSCTX header, flags, named values, and pair count.
func (msg *ttiOXSCTX) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	// enforce protocol limits before writing the message header
	for _, pair := range msg.pairs {
		if len(pair.key.value) > maxXsctxKeyLength {
			common.Odl.Debug("OXSCTX key exceeds maximum length", "length", len(pair.key.value), "limit", maxXsctxKeyLength)
			return common.NewOracleError(oracleErrors.FailMarshal, nil, "OXSCTX key")
		}
		if len(pair.textValue.value) > maxXsctxValueLength || len(pair.binaryValue.value) > maxXsctxValueLength {
			common.Odl.Debug("OXSCTX value exceeds maximum length", "text length", len(pair.textValue.value),
				"binary length", len(pair.binaryValue.value), "limit", maxXsctxValueLength)
			return common.NewOracleError(oracleErrors.FailMarshal, nil, "OXSCTX value")
		}
	}
	if err := msg.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Warn("Error marshalling OXSCTX header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OXSCTX")
	}
	if err := engine.MarshalUB4(ctx, xsctxProcessInit); err != nil {
		common.Odl.Warn("Error marshalling OXSCTX flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OXSCTX flags")
	}
	// write the pair pointer and count before the pair data
	if len(msg.pairs) == 0 {
		if err := engine.MarshalNullPTR(ctx); err != nil {
			common.Odl.Warn("Error marshalling empty OXSCTX pair pointer", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "OXSCTX pair pointer")
		}
	} else if err := engine.MarshalPTR(ctx); err != nil {
		common.Odl.Warn("Error marshalling OXSCTX pair pointer", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OXSCTX pair pointer")
	}
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(len(msg.pairs))); err != nil {
		common.Odl.Warn("Error marshalling OXSCTX pair count", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OXSCTX pair count")
	}
	for _, pair := range msg.pairs {
		if err := pair.MarshalTo(ctx, engine); err != nil {
			return common.NewOracleError(oracleErrors.FailMarshal, err, "OXSCTX pair")
		}
	}
	return nil
}
