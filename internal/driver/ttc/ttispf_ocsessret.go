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

// ttiSPFOCSessret server’s piggyback “session-return values” message for a pooled-session GET/attach.
// This message is received from the server and therefore only implements
// UnMarshalFrom and GetMsgCode; it does not support MarshalTo.
type ttiSPFOCSessret struct {
	// keyValueArr holds the list of keyword/value pairs carried by the piggyback.
	keyValueArr *keywordValueArray
	// keyValueArrFlag is the trailing UB4 returned by the server that holds Metadata about returned session
	sessretflags driverCommon.UB4
	// Assigned session ID
	sessretidx driverCommon.UB4
	// Session serial number
	sessretser driverCommon.UB2
}

const (
	sessgetTagMismatch      = 0x00000001
	sessgetPurityNew        = 0x00000002
	sessgetSessionChanged   = 0x00000004
	sessgetStmtCacheDestroy = 0x00000008
)

func (spf *ttiSPFOCSessret) getSessretidx() driverCommon.UB4 {
	return spf.sessretidx
}

func (spf *ttiSPFOCSessret) getSessretser() driverCommon.UB2 {
	return spf.sessretser
}

// newttiSPFOCSessret allocates a new receiv
// er for TTISPF/OCSSESSRET payloads.
// The returned value implements common.Message and is intended to be populated
// via UnMarshalFrom by the MessageStreamer.
func newttiSPFOCSessret() driverCommon.Message[driverCommon.MessageType] {
	return &ttiSPFOCSessret{}
}

// GetMsgCode implements common.Message and identifies this message as TTISPF
// (Server-side piggyback).
func (spf *ttiSPFOCSessret) GetMsgCode() driverCommon.MessageType {
	return TTISPF
}

// GetFuncCode returns the piggyback function code associated with this message.
// For OCSSESSRET it is ocsessret (see ttimsgconst.go).
func (spf *ttiSPFOCSessret) GetFuncCode() driverCommon.FunctionType {
	return ocsessret
}

// UnMarshalFrom reads a TTISPF/OCSSESSRET payload from the wire.
// Expected layout (as observed from network traces):
func (spf *ttiSPFOCSessret) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	common.Odl.Debug("Unmarshal ttiSPFOCSessret")
	// Reserved/length (ignored)
	_, err := engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback Reserved/length", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session ret")
	}
	// Flags (ignored)
	_, err = engine.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback flags", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session ret")
	}

	// Number of pairs to follow
	numOfPairs, err := engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback number of pairs", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session ret")
	}
	if numOfPairs > 0 {
		// Key/value list
		keyValueList, err := newKeywordValueArray(driverCommon.UB4(numOfPairs))
		if err != nil {
			common.Odl.Warn("Server-To-Client Piggyback key/value pair count exceeds limit", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, "keyword/value")
		}
		spf.keyValueArr = keyValueList
		err = ((driverCommon.UnMarshallable)(keyValueList)).UnMarshalFrom(ctx, engine)
		if err != nil {
			common.Odl.Warn("Unable to unmarshal Server-To-Client Piggyback, cant' unmarshal key/value pairs", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, "keyword/value")
		}
	}

	spf.sessretflags, err = engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB4 for Server-To-Client Piggyback ret flags", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session ret")
	}

	if (spf.sessretflags & sessgetTagMismatch) > 0 {
		// the requested tag was not found in server pool
		common.Odl.Info("tag mismatch received", "error", err)
	}

	spf.sessretidx, err = engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB4 for Server-To-Client Piggyback session ID", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session ret")
	}
	common.Odl.Debug("ttiSPFOCSessret.Unmarshal", "sessionID", spf.sessretidx)

	spf.sessretser, err = engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback session serial", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session ret")
	}
	common.Odl.Debug("ttiSPFOCSessret.Unmarshal", "sessionSerial", spf.sessretser)

	return nil
}
