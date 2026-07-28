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
	"strconv"

	"github.com/oracle/go-driver/driver/common"
)

// ttioallrpa represents the OALL8 RPA (Response Parameters) block as decoded from the wire.
// - al8o4 (UB4 vector) with SCN and cursorId
// - transaction context (length + bytes) is read/ignored
// - keyword-values is read
type ttioallrpa struct {
	// Decoded fields (subset)
	scn      uint64       // built from al8o4[0] and al8o4[1] without KSCNFVB
	cursorId common.SB4   // from al8o4[2]
	al8o4    []common.UB4 // raw UB4 vector for reference

	registrationFeedback []byte
	// pidmlRowCounts holds per-iteration DML row counts when present.
	pidmlRowCounts []common.UB8
}

func (p *ttioallrpa) GetMsgCode() common.MessageType {
	return TTIRPA
}

func (p *ttioallrpa) getCursorId() common.SB4 {
	return p.cursorId
}

func newTTIOallRPA() common.Message[common.MessageType] {
	return &ttioallrpa{}
}

// UnMarshalFrom decodes the OALL8 RPA core payload.
// It reads:
//   - UB2 al8o4l (length), then al8o4l times UB4 values into al8o4
//   - Builds SCN from al8o4[0] (low) and al8o4[1]&^KSCNFVB (high)
//   - Sets cursorId from al8o4[2]
//   - Reads transaction context length (UB2) and bytes (ignored)
//   - Reads keyword-values length (UB2); decoding of pairs is deferred
func (p *ttioallrpa) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	// read UB2 length for al8o4
	al8o4l, err := mar.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Error("ttioallrpa.UnMarshalFrom: failed to read al8o4 length",
			"error", err, "stage", "al8o4-len")
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// read al8o4 UB4 vector used for sequence numbers
	// al8o4[0] currentSCN (1/2 )
	// al8o4[1] current SCN (other 1/2)
	// al8o4[2] returned cursorId
	// al8o4[3] XA out flags
	// al8o4[4] number of rows cached on server
	// al8o4[5] checksum from server
	p.al8o4 = make([]common.UB4, int(al8o4l))
	for i := 0; i < int(al8o4l); i++ {
		val, err := mar.UnmarshalUB4(ctx)
		if err != nil {
			common.Odl.Error("ttioallrpa.UnMarshalFrom: failed to read al8o4 value",
				"error", err, "stage", "al8o4-value["+strconv.Itoa(i)+"]")
			return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		p.al8o4[i] = val
	}

	// Build SCN: al8o4[1] (high) without KSCNFVB, al8o4[0] (low)
	const KSCNFVB common.UB4 = 0x8000 // fully valid bit for kscnwrp
	if len(p.al8o4) >= 2 {
		least := p.al8o4[0]
		most := p.al8o4[1] &^ KSCNFVB
		p.scn = (uint64(most) << 32) | uint64(least)
	}
	// Cursor id
	if len(p.al8o4) >= 3 {
		p.cursorId = common.SB4(p.al8o4[2])
	}

	// Transaction context length + bytes (ignored as we don't support it)
	al8txl, err := mar.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Error("ttioallrpa.UnMarshalFrom: failed to read txn length",
			"error", err, "stage", "txn-len")
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	if al8txl > 0 {
		if _, err := mar.UnmarshalB1Array(ctx, int(al8txl)); err != nil {
			common.Odl.Error("ttioallrpa.UnMarshalFrom: failed to read txn bytes",
				"error", err, "stage", "txn-bytes")
			return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
	}

	// Keyword-values count (we should never receive the count>0 as kpccapCtbOci3Ocssync capability is
	// set. It will be received as part of OCSSYNC).
	if al8kvl, err := mar.UnmarshalUB2(ctx); err != nil || al8kvl != 0 {
		common.Odl.Error("ttioallrpa.UnMarshalFrom: failed to read kv-count",
			"error", err, "stage", "kv-count", "al8kvl", al8kvl)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// Registration feedback (regLen should not be > 0). We do not support query cache/dcn yet.
	regFeedbackLen, err := mar.UnmarshalUB4(ctx)
	if err != nil || regFeedbackLen != 0 {
		common.Odl.Error("ttioallrpa.UnMarshalFrom: failed to read reg-feedback-len",
			"error", err, "stage", "reg-feedback-len", "reg-feedback-len", regFeedbackLen)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	return nil
}

// UnMarshalDMLRows decodes optional per-iteration DML row counts (AL8PIDMLRC).
func (p *ttioallrpa) UnMarshalDMLRows(ctx context.Context, mar common.Marshaller) error {
	count, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Error("ttioallrpa.UnMarshalDMLRows: failed to read dml-count",
			"error", err, "stage", "dml-count")
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	if count > 0 {
		p.pidmlRowCounts = make([]common.UB8, int(count))
		for i := 0; i < int(count); i++ {
			u, err := mar.UnmarshalUB8(ctx)
			if err != nil {
				common.Odl.Error("ttioallrpa.UnMarshalDMLRows: failed to read dml-value",
					"error", err, "stage", "dml-value["+strconv.Itoa(i)+"]")
				return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
			}
			p.pidmlRowCounts[i] = u
		}
	}
	return nil
}

// getTotalAffectedRowsCount gets the count of affected rows of executed statements
// can be 0
func (p *ttioallrpa) getTotalAffectedRowsCount() common.UB8 {
	var total common.UB8 = 0
	for _, count := range p.pidmlRowCounts {
		total += count
	}
	return total
}
