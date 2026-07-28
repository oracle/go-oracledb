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

	"github.com/oracle/go-driver/driver/common"
)

// tTIiov represents the TTC IOV (input/output vector) message.
// It is received together with RXH and records, for each bind position,
// whether the bind is IN, OUT, or both.
type tTIiov struct {
	// rxh is the RXH received along with this IOV.
	rxh *tTIrxh

	// numberOfBindPositions is the number of bind positions expected in this IOV.
	numberOfBindPositions int

	// bindCount is the total number of bind variables.
	bindCount common.UB2
}

// newTTIiov creates a new TTC IOV message instance.
func newTTIiov() common.Message[common.MessageType] {
	return &tTIiov{}
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC IOV message code.
func (p *tTIiov) GetMsgCode() common.MessageType {
	return TTIIOV
}

// SetNumberOfBindPositions configures how many bind positions this IOV should decode.
func (p *tTIiov) SetNumberOfBindPositions(numberOfBindPositions int) {
	p.numberOfBindPositions = numberOfBindPositions
}

// UnMarshalFrom unmarshals a TTC IOV message from the network buffer.
//
// It first unmarshals the RXH companion message, then reads one bind-type byte
// per bind position and records whether each position is IN and/or OUT.
func (p *tTIiov) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	if p.rxh != nil {
		if err := p.rxh.UnMarshalFrom(ctx, mar); err != nil {
			return err
		}
		p.bindCount = p.rxh.numRequest
	}

	common.Odl.Debug("TTIiov: unmarshal start",
		"bindCount", p.bindCount,
		"bindPositions", p.numberOfBindPositions)

	for i := range p.numberOfBindPositions {
		bindType, err := mar.UnmarshalUB1(ctx)
		if err != nil {
			common.Odl.Warn("TTIiov: failed to unmarshal bind type", "index", i, "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		if bindType == 0 {
			common.Odl.Warn("TTIiov: invalid zero bind type", "index", i)
			return common.NewOracleError(common.FailUnmarshal, nil, TTCMsgTypeDescription[p.GetMsgCode()])
		}
	}

	common.Odl.Debug("TTIiov: unmarshal complete")
	return nil
}
