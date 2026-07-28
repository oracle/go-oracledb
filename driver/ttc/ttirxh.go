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
	"log/slog"

	"github.com/oracle/go-driver/driver/common"
)

// tTIrxh represents the RXH (Receive Header) structure used in the TTC protocol.
type tTIrxh struct {
	// rxhflags contains flags relating to this RXH header or message fragment.
	rxhflags common.UB1
	// numRequest specifies the number of requests or operations in this message.
	numRequest common.UB2
	// iterationNum is the current iteration number or sequence for this message, e.g., in batch or array DML.
	iterationNum common.UB2
	// numItersThisTime reports the number of iterations performed in this message.
	numItersThisTime common.UB4
	// uacBufLength is the length of the user access control buffer or auxiliary data.
	uacBufLength common.UB2
	// rowBitVector contains the bit vector indicating which rows are present in this RXH message.
	rowBitVector dynamicAllocatedArray
	// logicalRowID stores the logical row identifiers associated with this RXH message.
	logicalRowID dynamicAllocatedArray
}

// newTTIrxh creates and returns a pointer to a new tTIrxh, which represents
// the RXH (Receive Header) structure used during TTC protocol unmarshalling.
func newTTIrxh() common.Message[common.MessageType] {
	return &tTIrxh{}
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC RXH message code.
func (p *tTIrxh) GetMsgCode() common.MessageType {
	return TTIRXH
}

// UnMarshalFrom reads the RXH header data for the TTC protocol from the provided Marshaller,
// unmarshalling it into the receiver. Returns an error if decoding fails at any stage.
func (p *tTIrxh) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	var err error
	common.Odl.Debug("TTIrxh: UnMarshalFrom start")

	if p.rxhflags, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal rxhflags", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if p.numRequest, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal numRequest", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if p.iterationNum, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal iterationNum", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	p.numRequest = p.numRequest + p.iterationNum*256
	if p.numItersThisTime, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Debug("UnMarshalFrom: failed to unmarshal numItersThisTime", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if p.uacBufLength, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal uacBufLength", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if err = p.rowBitVector.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal bitVectorBytes", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if err = p.logicalRowID.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal logicalRowId", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIrxh: fully unmarshalled", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}
