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

// ttiSPFOCSSync models a Server Piggyback (TTISPF) packet used by the server
// to synchronize session state with the client (OCSSYNC, see function code ocssync).
// This message is received from the server and therefore only implements
// UnMarshalFrom and GetMsgCode; it does not support MarshalTo.
type ttiSPFOCSSync struct {
	// keyValueArr holds the list of keyword/value pairs carried by the piggyback.
	keyValueArr *keywordValueArray
	// keyValueArrFlag is the trailing UB4 returned by the server that may convey
	// additional attributes about the key/value array.
	keyValueArrFlag driverCommon.UB4
}

var _lsKeys = [64]string{
	0:  authNlsLxcCurrency,
	1:  authNlsLxcIsoCurr,
	2:  authNlsLxcNumerics,
	7:  authNlsLxcDateFm,
	8:  authNlsLxcDateLang,
	9:  authNlsLxcTerritory,
	10: sessionNlsLxcCharset,
	11: authNlsLxcSort,
	12: authNlsLxcCalendar,
	16: authNlsLxlan,
	50: authNlsLxNlsComp,
	52: authNlsLxcUnionCur,
	57: authNlsLxcTimeFm,
	58: authNlsLxcStmpFm,
	59: authNlsLxcTtznfm,
	60: authNlsLxcStznfm,
	61: sessionNlsLxcNlsLenSem,
	62: sessionNlsLxcNcharExcp,
	63: sessionNlsLxcNcharImp,
}

// GetNlsKeys gets nlsKeys maps AL8KW_* keyword indices from the OCSSYNC (TTISPF) piggyback
// to their corresponding AUTH/SESSION key names.
func (spf *ttiSPFOCSSync) GetNlsKeys() [64]string {
	return _lsKeys
}

// newttiSPFOCSSync allocates a new receiver for TTISPF/OCSSYNC payloads.
// The returned value implements common.Message and is intended to be populated
// via UnMarshalFrom by the MessageStreamer.
func newttiSPFOCSSync() driverCommon.Message[driverCommon.MessageType] {
	return &ttiSPFOCSSync{}
}

// GetMsgCode implements common.Message and identifies this message as TTISPF
// (Server-side piggyback).
func (spf *ttiSPFOCSSync) GetMsgCode() driverCommon.MessageType {
	return TTISPF
}

// getFuncCode returns the piggyback function code associated with this message.
// For OCSSYNC it is ocssync (see ttimsgconst.go).
func (spf *ttiSPFOCSSync) GetFuncCode() driverCommon.FunctionType {
	return driverCommon.FunctionType(ocssync)
}

// UnMarshalFrom reads a TTISPF/OCSSYNC payload from the wire.
// Expected layout (as observed from network traces):
func (spf *ttiSPFOCSSync) UnMarshalFrom(ctx context.Context, engine driverCommon.Marshaller) error {
	// Reserved/length (ignored)
	_, err := engine.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback Reserved/length", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session sync")
	}
	// Flags (ignored)
	_, err = engine.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback flags", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session sync")
	}

	// Number of pairs to follow
	numOfPairs, err := engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback number of pairs", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session sync")
	}

	// Additional flags (ignored)
	_, err = engine.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback additional flags", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, nil)
	}

	// Key/value list
	keyValueList, err := newKeywordValueArray(numOfPairs)
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

	// Trailing flag
	spf.keyValueArrFlag, err = engine.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Error unmarshalling UB2 for Server-To-Client Piggyback Flag", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "session sync")
	}
	return nil
}

// getKeyValueArr returns the key/value content of the piggyback as a Properties map.
// Keys are the numeric keyword codes stringified (e.g., "5"), and values are the
// underlying KeyValue entries as stored in the message.
func (spf *ttiSPFOCSSync) getKeyValueArr() *driverCommon.Properties[string] {
	props := driverCommon.NewProperties[string]()

	// Build a Properties view from the keyword/value array.
	for _, kv := range *spf.keyValueArr {
		key, value := getKeyValueFromKeyword(spf.GetNlsKeys(), kv)
		if key != "" {
			props.SetProperty(key, value)
		} else {
			common.Odl.Debug("For keyword %d getKeyValueFromKeyword returned empty string",
				"key", kv.keyword)
		}

	}
	return props
}
