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

// tTIlobd represents the TTC LOB data message used to stream LOB chunks in
// either direction between client and server.
type tTIlobd struct {
	// buffer holds the destination buffer when unmarshalling data from the
	// wire into a LOB or source buffer when marshalling LOB data to wire
	buffer driverCommon.B1Array

	// lastBytesRead tracks the number of bytes copied during the most recent
	// UnmarshalFrom invocation so callers can update bookkeeping such as
	// lobDefinition.bytesTransferred.
	lastBytesRead driverCommon.UB8
}

// newTTIlobd constructs a new TTC LOB data message instance.
//
// Outputs:
//   - common.Message[common.MessageType]: allocator for TTILOBD TTC messages.
func newTTIlobd() driverCommon.Message[driverCommon.MessageType] {
	return &tTIlobd{}
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC LOBD message code.
//
// Outputs:
//   - common.MessageType: TTILOBD message identifier.
func (p *tTIlobd) GetMsgCode() driverCommon.MessageType {
	return TTILOBD
}

// UnmarshalFrom populates the output buffer with bytes read from the marshaller.
//
// Inputs:
//   - ctx context.Context: governs cancellation and deadlines for unmarshalling.
//   - mar common.Marshaller: source of TTC payload bytes.
//
// Outputs:
//   - error:common.NewOracleError with code FailUnmarshal when unmarshalling from the wire fails.
//
// Preconditions: call setOutputBuffer before invoking UnmarshalFrom.
func (p *tTIlobd) UnmarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTILobd.UnmarshalFrom: start")

	bytesRead, err := mar.UnmarshalCLR(ctx, p.buffer, len(p.buffer))
	if err != nil {
		common.Odl.Error("TTILobd.UnmarshalFrom: read error", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	common.Odl.Debug("TTILobd.UnmarshalFrom: completed", "bytes_read", bytesRead)
	if bytesRead >= 0 {
		p.lastBytesRead = driverCommon.UB8(bytesRead)
	} else {
		p.lastBytesRead = 0
	}
	return nil
}

// MarshalTo writes the configured input buffer to the marshaller. The input
// buffer must be primed via setInputBuffer before invocation.
//
// Outputs:
//   - error: common.NewOracleError with code FailMarshal when marshalling to the wire fails.
func (p *tTIlobd) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTILobd.MarshalTo: start")

	if err := mar.MarshalCLR(ctx, p.buffer, 0, len(p.buffer)); err != nil {
		common.Odl.Error("TTILobd.MarshalTo: write failed", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if common.Odl.Enabled(context.TODO(), slog.LevelDebug) {
		common.Odl.Debug("TTILobd.MarshalTo: completed", "bytes_written", len(p.buffer))
	}
	return nil
}

// setBuffer configures the shared LOB buffer used during marshalling and
// unmarshalling operations.
//
// Inputs:
//   - buf common.B1Array: LOB byte slice acting as either the inbound payload
//     (for UnmarshalFrom) or outbound payload (for MarshalTo).
func (p *tTIlobd) setBuffer(buf driverCommon.B1Array) {
	p.buffer = buf
}

// getLastBytesRead reports the number of bytes copied into the output buffer
// during the most recent UnmarshalFrom invocation.
func (p *tTIlobd) getLastBytesRead() driverCommon.UB8 {
	return p.lastBytesRead
}
