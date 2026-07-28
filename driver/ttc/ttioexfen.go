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

// Internal flags for OEXFEN marshal fields (exerof/exeflg).
const (
	// exerof flags (operation requested)
	_kpuCxExe common.UB4 = 0x20 // Execute to be performed
	_kpuCxFch common.UB4 = 0x40 // Fetch to be performed

	// exeflg flags (execution modifiers)
	_exeCommitOnSuccess common.UB4 = 0x00000001 // Commit on success
	_maxfetchSize       common.UB4 = 2000000000 // max fetch size for oexfen
)

// tTIOexfen represents the OEXFEN fast-path execute+fetch TTC function request.
// This is a TTIFUN message carrying a minimal payload: cursor id, iteration count,
// operation flags (exerof) and execution flags (exeflg).
type tTIOexfen struct {
	headerMarshaller common.Marshallable // marshals the function code w/wo seq and token numbers
	cursorId         common.SB4          // execid: cursor id
	fetchSize        common.UB4          // exenit: number of rows to fetch
	options          common.UB4          // options (used to derive commit-on-success)
}

// NewOexfen constructs an OEXFEN TTC function message (pre-18 protocol header).
func NewOexfen() common.Message[common.MessageType] {
	return &tTIOexfen{
		headerMarshaller: &ttiFunHeader{_funcType: oExfen},
	}
}

// NewOexfen18 constructs an OEXFEN TTC function message (18+ protocol header).
func NewOexfen18() common.Message[common.MessageType] {
	return &tTIOexfen{
		headerMarshaller: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oExfen}},
	}
}

// GetMsgCode implements common.Message. OEXFEN is a TTIFUN message.
func (m *tTIOexfen) GetMsgCode() common.MessageType { return TTIFUN }

// GetFuncCode returns the TTC function code associated with OEXFEN.
func (m *tTIOexfen) GetFuncCode() common.FunctionType { return oExfen }

// setCursorId sets the cursor id.
func (m *tTIOexfen) setCursorId(c common.SB4) { m.cursorId = c }

// setFetchSize sets the number of iterations (rows) for execute+fetch.
func (m *tTIOexfen) setFetchSize() { m.fetchSize = _maxfetchSize }

// setOptions sets operation flags (used to compute exeflg e.g., commit on success).
func (m *tTIOexfen) setOptions(opts common.UB4) { m.options = opts }

// MarshalTo serializes the OEXFEN function message.
//
// Wire layout:
// - execid (cursorId)  -> SWORD (marshalled here as SB4 for consistency with engine usage)
// - exenit (iters)   -> SWORD (marshalled as SB4)
// - exerof (ops)     -> SWORD (marshalled as UB4): KPUCXEXE|KPUCXFCH
// - exeflg (modifiers)-> SWORD (marshalled as UB4): commit-on-success if requested
func (m *tTIOexfen) MarshalTo(ctx context.Context, engine common.Marshaller) error {
	// 1) Header: function code (and sequence/token if applicable)
	if err := m.headerMarshaller.MarshalTo(ctx, engine); err != nil {
		common.Odl.Error("TTIOexfen.MarshalTo: header marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// 2) Payload fields
	// execid (cursorId)
	if err := engine.MarshalSB4(ctx, m.cursorId); err != nil {
		common.Odl.Error("TTIOexfen.MarshalTo: cursorId marshal failed",
			"error", err,
			"cursorId", m.cursorId,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// exenit (iterations)
	if err := engine.MarshalUB4(ctx, m.fetchSize); err != nil {
		common.Odl.Error("TTIOexfen.MarshalTo: iterations marshal failed",
			"error", err,
			"fetchSize", m.fetchSize,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// exerof: execute + fetch
	exerof := _kpuCxExe | _kpuCxFch
	if err := engine.MarshalUB4(ctx, exerof); err != nil {
		common.Odl.Error("TTIOexfen.MarshalTo: exerof marshal failed",
			"error", err,
			"exerof", exerof,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// exeflg: commit-on-success if requested by options
	var exeflg common.UB4 = 0
	if (m.options & commitAfterExecution) != 0 {
		exeflg |= _exeCommitOnSuccess
	}
	if err := engine.MarshalUB4(ctx, exeflg); err != nil {
		common.Odl.Error("TTIOexfen.MarshalTo: exeflg marshal failed",
			"error", err,
			"exeflg", exeflg,
			"options", m.options,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	return nil
}
