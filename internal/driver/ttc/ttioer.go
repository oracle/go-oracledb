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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// tTIOerIface defines an interface for Oracle error data protocol unmarshalling and processing.
type tTIOerIface interface {
	// getError gets the wrapped error sent back from the server.
	// in some context. Oer can be received but that do not indicate an error situation.
	// when that is the case, this method return nil.
	// 'getError() == nil' is equivalent as 'GetErrorCode() == 0'
	getError() error
}

// tTIoer represents the Oracle error (OER) structure for version 0-6.
type tTIoer struct {
	curRowNumber               driverCommon.UB4 // curRowNumber is the number of rows processed.
	retCode                    driverCommon.UB2 // retCode is the error code (if any) returned by the server.
	arrayElemWError            driverCommon.UB2
	arrayElemErrno             driverCommon.UB2
	currCursorID               driverCommon.UB2
	errorPosition              driverCommon.UB2
	sqlType                    driverCommon.UB1
	oerFatal                   driverCommon.UB2
	flags                      driverCommon.UB2
	userCursorOpt              driverCommon.UB2
	upiParam                   driverCommon.UB1
	warningFlag                driverCommon.UB1
	osError                    driverCommon.UB4
	stmtNumber                 driverCommon.UB1
	callNumber                 driverCommon.UB1
	pad1                       driverCommon.UB2
	successIters               driverCommon.UB4
	partitionID                driverCommon.UB2 // partitionID is rowid
	tableID                    driverCommon.UB1
	slotNumber                 driverCommon.UB2
	rba                        driverCommon.UB4
	blockNumber                driverCommon.UB4
	warnLength                 driverCommon.UB2
	warnFlag                   driverCommon.UB2
	endToEndECIDSequenceNumber driverCommon.UB2
	oercn2                     driverCommon.UB8 // Number of rows processed
	oerrcd2                    driverCommon.UB4 // Error code
	startErrorOffset           int
	endErrorOffset             int
	batchErrorOffsetArray      []int

	// Length of error message (if any), this is an array because it is
	// also used in extractRowData
	errorLength [1]int
	errorMsg    []byte

	// Store oerepa for DPL. This represents start and end stream offset of data in error.
	oerepa []byte

	// EOCS is activated through a compile time capability and is sent by the server at the beginning of either TTIOER or TTISTA.
	_supportsEndOfCallStatus bool
	// eocStatus end of call status
	eocStatus *endOfCallStatus
}

func (e tTIoer) String() string {
	return fmt.Sprintf("TTIoer {endToEndECIDSequenceNumber: [%v], callNumber: [%v], retCode: [%v], oerrcd2: [%v], errorMsg: [%v]}",
		e.endToEndECIDSequenceNumber,
		e.callNumber,
		e.retCode,
		e.oerrcd2,
		string(e.errorMsg))
}

// endOfCallStatus contains information received as End of Call Status (shared between OER and STA)
type endOfCallStatus struct {
	// elapsedTime elapsed call time
	elapsedTime driverCommon.UB8
	// connectionShouldBeDropped indicates this connection is affected by a
	// planned-down
	connectionShouldBeDropped bool
}

func (e *endOfCallStatus) String() string {
	return fmt.Sprintf("endOfCallStatus {elapsedTime: [%v], connectionShouldBeDropped: [%v]}",
		e.elapsedTime,
		e.connectionShouldBeDropped)
}

// newTTIoer creates a new instance of tTIoer.
func newTTIoer() driverCommon.Message[driverCommon.MessageType] {
	return &tTIoer{
		_supportsEndOfCallStatus: false,
	}
}

// newTTIoerWithEndOfCallStatusSupport creates a new instance of tTIoer that supports end of call status.
func newTTIoerWithEndOfCallStatusSupport() driverCommon.Message[driverCommon.MessageType] {
	return &tTIoer{
		_supportsEndOfCallStatus: true,
	}
}

// GetMsgCode returns the message type code for tTIoer.
func (o *tTIoer) GetMsgCode() driverCommon.MessageType {
	return TTIOER
}

// Init initializes or resets the tTIoer fields.
func (o *tTIoer) init() {
	o.retCode = 0
	o.errorMsg = nil
	o.oerepa = nil
	o.startErrorOffset = 0
	o.endErrorOffset = 0
	o.batchErrorOffsetArray = nil
	o.oerrcd2 = 0
	o.oercn2 = 0
}

// UnMarshalFrom unmarshals the error data
func (o *tTIoer) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTIoer.UnMarshalFrom: start")
	err := o._unmarshalAttributes(ctx, mar)
	if err != nil {
		common.Odl.Error("TTIoer.UnMarshalFrom: unmarshalAttributes failed",
			"error", err,
			"retCode", o.retCode,
			"oerrcd2", o.oerrcd2,
			"curRowNumber", o.curRowNumber,
			"errorPosition", o.errorPosition,
			"sqlType", o.sqlType,
			"oerFatal", o.oerFatal,
			"flags", o.flags,
			"warningFlag", o.warningFlag,
			"osError", o.osError,
			"successIters", o.successIters,
		)
		return err
	}

	// If the retCode (Error #) is not zero or oerrcd2 != 0, extract the error-string
	if o.retCode != 0 || o.oerrcd2 != 0 {
		common.Odl.Debug("TTIoer.UnMarshalFrom: error code != 0, unmarshalling error message")
		err = o._unmarshalErrorMessage(ctx, mar)
		if err != nil {
			common.Odl.Error("TTIoer.UnMarshalFrom: unmarshalErrorMessage failed",
				"error", err,
				"retCode", o.retCode,
				"oerrcd2", o.oerrcd2,
				"curRowNumber", o.curRowNumber,
				"errorPosition", o.errorPosition,
				"sqlType", o.sqlType,
			)
			return err
		}
	}

	common.Odl.Debug("TTIoer.UnMarshalFrom: end")

	return nil
}

const errorMessageMaxLength = 2048

func (o *tTIoer) _unmarshalErrorMessage(ctx context.Context, mar driverCommon.Marshaller) error {
	b := make(driverCommon.B1Array, errorMessageMaxLength)
	n, err := mar.UnmarshalCLR(ctx, b, errorMessageMaxLength)
	if err != nil {
		common.Odl.Error("TTIoer._unmarshalErrorMessage: UnmarshalCLR failed",
			"error", err,
			"retCode", o.retCode,
			"oerrcd2", o.oerrcd2,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	o.errorMsg = b[:n]
	o.errorLength[0] = n
	common.Odl.Debug("TTIoer.unmarshalErrorMessage: extracted error message", "errorMessage", o.errorMsg)
	return nil
}

// _unmarshalAttributes unmarshals the tTIoer attributes from the marshaller.
// To note: As batch processing is not supported in v1, the batch error related fields are not processed.
func (o *tTIoer) _unmarshalAttributes(ctx context.Context, mar driverCommon.Marshaller) error {
	var err error
	common.Odl.Debug("TTIoer.UnmarshalAttributes: start ", "supportsEndOfCallStatus", o._supportsEndOfCallStatus)
	if o._supportsEndOfCallStatus {
		o.eocStatus, err = unmarshalEndOfCallStatus(ctx, mar)
		if err != nil {
			common.Odl.Error("TTIoer.UnmarshalAttributes: unmarshalEndOfCallStatus failed", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
		}
	}

	if o.endToEndECIDSequenceNumber, err = mar.UnmarshalUB2(ctx); err != nil { // for all ttcversion above 3
		common.Odl.Error("TTIoer.UnmarshalAttributes: endToEndECIDSequenceNumber unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.curRowNumber, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: curRowNumber unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.retCode, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: retCode unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.arrayElemWError, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: arrayElemWError unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.arrayElemErrno, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: arrayElemErrno unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.currCursorID, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: currCursorID unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.errorPosition, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: errorPosition unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.sqlType, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: sqlType unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.oerFatal, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oerFatal unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.flags, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: flags unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.userCursorOpt, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: userCursorOpt unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.upiParam, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: upiParam unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.warningFlag, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: warningFlag unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	// RowId related
	if o.rba, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: rba unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.partitionID, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: partitionID unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.tableID, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: tableID unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.blockNumber, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: blockNumber unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.slotNumber, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: slotNumber unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.osError, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: osError unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.stmtNumber, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: stmtNumber unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.callNumber, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: callNumber unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.pad1, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: pad1 unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.successIters, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: successIters unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	var oerdd dynamicAllocatedArray
	if err = oerdd.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oerdd unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	// The next 3 fields are related to error batching.
	// Error batching is about continuing the batch even if an
	// error occurs.
	// This is not supported in v1, so we just read and discard the values.
	var oerrar dynamicAllocatedArray
	if err = oerrar.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oerrar unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	var oerepa dynamicAllocatedArray
	if err = oerepa.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oerepa unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.oerepa != nil && len(oerrar.value) > 0 {
		common.Odl.Error("more than 1 error present, Processing batched error not supported currently")
	}

	// ignore oermarl for now
	_, err = mar.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oermarl unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.oerrcd2, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oerrcd2 unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.oercn2, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalAttributes: oercn2 unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIoer.UnmarshalAttributes: end", "struct", fmt.Sprintf("%+v", o))
	}
	return nil
}

// _unmarshalWarning reads warning bytes if present
func (o *tTIoer) _unmarshalWarning(ctx context.Context, mar driverCommon.Marshaller) error {
	var err error
	common.Odl.Debug("TTIoer.UnmarshalWarning: start")
	if o.retCode, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalWarning: retCode unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.warnLength, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalWarning: warnLength unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}
	if o.warnFlag, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Error("TTIoer.UnmarshalWarning: warnFlag unmarshal failed",
			"error", err,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
	}

	if o.retCode != 0 && o.warnLength > 0 {
		if b, err := mar.UnmarshalB1Array(ctx, int(o.warnLength)); err == nil {
			o.errorMsg = b
			o.errorLength[0] = int(o.warnLength)
			common.Odl.Debug("TTIoer.UnmarshalWarning: extracted warning message ", "errorMessage", o.errorMsg)
		} else {
			common.Odl.Error("TTIoer.UnmarshalWarning: error message unmarshal failed",
				"error", err,
			)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[o.GetMsgCode()])
		}
	}
	common.Odl.Debug("TTIoer.UnmarshalWarning: end")
	return nil
}

func unmarshalEndOfCallStatus(ctx context.Context, mar driverCommon.Marshaller) (*endOfCallStatus, error) {
	var ucaeocs driverCommon.UB4
	var err error
	retVal := &endOfCallStatus{connectionShouldBeDropped: false, elapsedTime: 0}
	if ucaeocs, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("unmarshalEndOfCallStatus: ucaeocs unmarshal failed",
			"error", err,
		)
		return nil, common.NewOracleError(oracleErrors.FailUnmarshal, err, "EndOfCallStatus")
	}

	if (ucaeocs & TtiEocEct) != 0 {
		var elapsedTime driverCommon.UB8
		if elapsedTime, err = mar.UnmarshalUB8(ctx); err != nil {
			common.Odl.Error("unmarshalEndOfCallStatus: elapsedTime unmarshal failed",
				"error", err,
			)
			return nil, common.NewOracleError(oracleErrors.FailUnmarshal, err, "EndOfCallStatus")
		}
		retVal.elapsedTime = elapsedTime
		common.Odl.Debug("tTIoer.UnMarshalFrom: EOCS ", "Elapsed time", elapsedTime)
	}

	// server sends this bit to indicate that connection is affected by planned down
	if (ucaeocs & TtiEocfDropWhenReturned) != 0 {
		common.Odl.Debug("TTIoer.UnMarshalFrom: EOCS got in-band planned down bit, mark connection for close")
		retVal.connectionShouldBeDropped = true
		// TODO: set connection to be closed when returned to pool
	}
	return retVal, nil
}

// SetEocsCap sets End Of Call Status capability
func (o *tTIoer) setSupportsEndOfCallStatus(supportsEndOfCallStatus bool) {
	o._supportsEndOfCallStatus = supportsEndOfCallStatus
}

// isBeingDrainned returns true if the connection should be dropped
// due to a planned-down, otherwise false
func (o *tTIoer) isBeingDrainned() bool {
	return o._supportsEndOfCallStatus && o.eocStatus != nil && o.eocStatus.connectionShouldBeDropped
}

// getError return nil if the tTIoer does not represent an error, otherwise and
// error is returned
func (o *tTIoer) getError() error {
	if o.retCode == 0 && o.oerrcd2 == 0 {
		return nil
	}
	return common.NewOERMessageError(fmt.Sprintf("ORA-%05d", o.retCode), string(o.errorMsg))
}

// GetCurRowNumber returns the number of rows that were returned in the oer message.
func (o *tTIoer) GetCurRowNumber() driverCommon.UB8 {
	return o.oercn2
}

// GetRetCode returns the error code (oerrcd2).
func (o *tTIoer) GetRetCode() driverCommon.UB4 {
	return o.oerrcd2
}

// _computeChecksum updates the CRC with fields
func (o *tTIoer) _computeChecksum(localCheckSum uint64) uint64 {
	cs := localCheckSum
	cs = CRC64UpdateChecksum(cs, uint64(o.retCode))
	cs = CRC64UpdateChecksum(cs, uint64(o.curRowNumber))
	cs = CRC64UpdateChecksum(cs, uint64(o.errorPosition))
	cs = CRC64UpdateChecksum(cs, uint64(o.sqlType))
	cs = CRC64UpdateChecksum(cs, uint64(o.oerFatal))
	cs = CRC64UpdateChecksum(cs, uint64(o.flags))
	cs = CRC64UpdateChecksum(cs, uint64(o.userCursorOpt))
	cs = CRC64UpdateChecksum(cs, uint64(o.upiParam))
	cs = CRC64UpdateChecksum(cs, uint64(o.warningFlag))
	cs = CRC64UpdateChecksum(cs, uint64(o.osError))
	cs = CRC64UpdateChecksum(cs, uint64(o.successIters))
	cs = CRC64UpdateChecksum(cs, uint64(o.oerrcd2))
	cs = CRC64UpdateChecksum(cs, uint64(o.oercn2))

	if len(o.errorMsg) > 0 {
		cs = CRC64UpdateChecksumWithBytes(cs, o.errorMsg)
	}
	return cs
}

// CRC64UpdateChecksum updates the CRC64 checksum with the given value.
func CRC64UpdateChecksum(cs uint64, v uint64) uint64 {
	return cs ^ v
}

// CRC64UpdateChecksumWithBytes updates the CRC64 checksum with the given byte array.
func CRC64UpdateChecksumWithBytes(cs uint64, b []byte) uint64 {
	for _, x := range b {
		cs = CRC64UpdateChecksum(cs, uint64(x))
	}
	return cs
}

// RegisterOerWithCapability register OER messages that support end of call status on
// message registry. Replaces the existing messages
func RegisterOerWithCapability() {
	err := MessageRegistry.Register(TTIOER, 14, newTTIoer14WithEndOfCallStatusSupport)
	if err != nil {
		common.Odl.Debug("Failed to register message TTIOER version 2", "error", err)
	}

	err = MessageRegistry.Register(TTIOER, MinTTCProtocolVersion, newTTIoerWithEndOfCallStatusSupport)
	if err != nil {
		common.Odl.Debug("Failed to register message TTIOER version 1", "error", err)
	}
}
