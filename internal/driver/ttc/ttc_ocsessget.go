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

// ttiOCSessget server's "session-release values" message for a pooled-session release.
// This message is received from the server and therefore only implements
// UnMarshalFrom and GetMsgCode; it does not support MarshalTo.
type ttiOCSessget struct {
	header        driverCommon.Marshallable
	sessgetOkvn   driverCommon.UB2 // Number of keyvalue pair
	sessgetFlags  driverCommon.UB4 // (oracle to user) SessionGet flags
	sessigetFlags driverCommon.UB2 // (user to oracle) SessionGet flags to enable partial match
	returnTag     string           // Return Connection Tags
}

const multiPropertyTagMatchAny = 0x00000001

// newttiOCSessget allocates a new receiver for OCSSESSGRET payloads.
// The returned value implements common.Message and is intended to be populated
// via UnMarshalFrom by the MessageStreamer.
func newttiOCSessget() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOCSessget{
		header: &ttiFunHeader{_funcType: ocsessget},
	}
}

// newttiOCSessget18 allocates an OCSSESSGET message using the protocol 18
// function header, which includes the token field.
func newttiOCSessget18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOCSessget{
		header: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: ocsessget}},
	}
}

// SetMultiTagMatchAny enable
func (fun *ttiOCSessget) SetMultiTagMatchAny(on bool) {
	if on {
		fun.sessigetFlags |= multiPropertyTagMatchAny
	} else {
		fun.sessigetFlags &^= multiPropertyTagMatchAny
	}
}

// GetMsgCode implements common.Message and identifies this message as ttiSPFOCSessget
func (fun *ttiOCSessget) GetMsgCode() driverCommon.MessageType {
	return TTIFUN
}

// GetFuncCode returns the piggyback function code associated with this message.
func (fun *ttiOCSessget) GetFuncCode() driverCommon.FunctionType {
	return ocsessget
}

// MarshalTo writes a OCSSESSGET payload to the wire.
func (fun *ttiOCSessget) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	err := fun.header.MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	// no flag used for now
	fun.sessigetFlags = 0
	buf := dynamicAllocatedArray{value: []byte{}}

	err = buf.MarshalTo(ctx, engine) // kvals from srv. (O2U)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // num kvals.. (O2U)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // sessgetflags - Metadata about returned session (O2U)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalUB2(ctx, fun.sessigetFlags)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // Return Tag Pointer (O2U)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	err = engine.MarshalPTR(ctx) // Return Tag Length Pointer (O2U)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSGET", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	return nil
}

// OCSSESSGET function response
type ttiOCSessgetRpa struct {
	sessgetOkvn  driverCommon.UB2     // Number of keyvalue pair
	sessgetFlags driverCommon.UB4     // (oracle to user) SessionGet flags
	returnTag    driverCommon.B1Array // Return Connection Tags
}

// newTTILobRPA constructs a new LOB RPA message instance.
//
// Returns:
//   - common.Message[common.MessageType]: a zero-initialised LOB RPA message ready for use.
func newTtiOCSessgetRpa() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOCSessgetRpa{}
}

func (p *ttiOCSessgetRpa) GetMsgCode() driverCommon.MessageType {
	return TTIRPA
}

// UnMarshalFrom read OCSSESSGET RPA response from the wire
// arguments:
//   - ctx : context
//   - engine : marshalling engine
func (rpa *ttiOCSessgetRpa) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {

	common.Odl.Debug("ttiOCSessgetRpa unmarshalling from ttiFun")

	var err error
	rpa.sessgetOkvn, err = engine.UnmarshalUB2(ctx) // Read the key value length
	if err != nil {
		common.Odl.Warn("Failed to unmarshall OCSSESSGET RPA", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	if rpa.sessgetOkvn > 0 {
		nbOfBytes, err := engine.UnmarshalUB1(ctx) // Read key value pair only if the length is more than 0
		if err != nil {
			common.Odl.Warn("Failed to unmarshall OCSSESSGET RPAKeyvalue pair length", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
		// Keyvalue pair is not used as of now, so ignoring the value
		_, err = engine.UnmarshalB1Array(ctx, int(nbOfBytes))
		if err != nil {
			common.Odl.Warn("Failed to unmarshall OCSSESSGET RPA Keyvalue pair", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
		}
	}

	// 0x1 — TAG_MISMATCH
	// 0x2 — PURITY_NEW
	// 0x4 — SESSION_CHANGED
	// 0x8 — STMTCACHE_DESTROY
	// TODO deal with TAG_MISMATCH as user may want to know
	rpa.sessgetFlags, err = engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshall OCSSESSGET RPA flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	returnTagLength, err := engine.UnmarshalUB2(ctx) // ReturnTag Length (number of chars)
	if err != nil {
		common.Odl.Warn("Failed to unmarshall OCSSESSGET RPA flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}

	rpa.returnTag, err = engine.UnmarshalB1Array(ctx, int(returnTagLength)) // Read the ReturnTag value
	if err != nil {
		common.Odl.Warn("Failed to unmarshall OCSSESSGET RPA flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	common.Odl.Debug("ttiOCSessgetRpa returned", "returnTag", rpa.returnTag)
	return nil
}
