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

const (
	_maxColCount driverCommon.UB4 = 64 * 1024
)

// tTIdcb handles the Data Column Buffer unmarshalling logic for Oracle statements.
type tTIdcb struct {
	numUDS          driverCommon.UB4
	queryCompileKey driverCommon.B1Array
	colNames        []driverCommon.B1Array

	// udsArr holds all column UDS entries, one per column, as common.UnMarshallable
	udsArr []driverCommon.UnMarshallable
	// newUDS is a factory for creating a fresh UDS of the correct protocol version
	newUDS func() driverCommon.UnMarshallable
}

// newTTIdcb creates and initializes a new TTIDcb instance
func newTTIdcb() driverCommon.Message[driverCommon.MessageType] {
	obj := &tTIdcb{}
	obj.newUDS = newTTIuds
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("tTIdcb: newTTIdcb constructor", "struct", fmt.Sprintf("%+v", obj))
	}
	return obj
}

// GetMsgCode gets message code for DCB message
func (p *tTIdcb) GetMsgCode() driverCommon.MessageType {
	return TTIDCB
}

// getNumberOfColumns returns the number of columns
func (p *tTIdcb) getNumberOfColumns() driverCommon.UB4 {
	return p.numUDS
}

// getColumnContexts returns column contexts extracted from DCB.
func (p *tTIdcb) getColumnContexts() ([]columnContext, error) {
	if p.numUDS == 0 || p.udsArr == nil {
		common.Odl.Error("populateColumnMetaData: invalid DCB", "error", nil, "stage", "invalid-dcb", "cols", p.numUDS, "udsArrNil", p.udsArr == nil)
		return nil, common.NewOracleError(oracleErrors.RunQueryError, nil, "populateColumnMetaData failed ", p.numUDS, p.udsArr == nil)
	}
	metaData := make([]columnContext, p.numUDS)
	for i := 0; i < int(p.numUDS); i++ {
		udsProv := p.udsArr[i].(udsProvider)
		if udsProv != nil {
			oac := udsProv.getOac()
			metaData[i] = columnContext{
				Index:          i,
				Name:           udsProv.getColumnName(),
				SchemaName:     udsProv.getSchemaName(),
				DBTypeName:     udsProv.getTypeName(),
				Length:         int64(oac.maxLength),
				DataType:       DtyType(oac.dataType),
				Precision:      int64(oac.precision),
				Scale:          int8(oac.scale),
				KernelPosition: int(udsProv.getKernelPosition()),
				ColumnFlags:    uint32(udsProv.getColumnFlags()),
				CharsetForm:    uint8(oac.characterSetForm),
				CharsetID:      uint16(oac.characterSetID),
				Nullable:       udsProv.nullable(),
			}
			common.Odl.Debug("populateColumnMetaData: added column", "colName", metaData[i].Name)
		}
	}
	return metaData, nil
}

// UnMarshalFrom unmarshal's column description buffers
// It returns an error if any.
func (p *tTIdcb) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("tTIdcb: UnMarshalFrom start")
	var err error
	length, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("TIdcb.UnMarshalFrom: failed to unmarshal length", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// ignore buffer
	if _, err = mar.UnmarshalB1Array(ctx, int(length)); err != nil {
		common.Odl.Warn("tTIdcb.UnMarshalFrom: failed to unmarshal buffer", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// ignore maxSizeOfOneRow
	if _, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Warn("tTIdcb.UnMarshalFrom: failed to unmarshal maxSizeOfOneRow", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if err = p.receiveCommon(ctx, mar, false); err != nil {
		common.Odl.Warn("tTIdcb.UnMarshalFrom: failed in receiveCommon", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("tTIdcb: UnMarshalFrom done", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}

// receiveCommon unmarshals column information.
// If fromOdny is true, unmarshalling logic is adapted for ODNY sources.
func (p *tTIdcb) receiveCommon(ctx context.Context, mar driverCommon.Marshaller, fromOdny bool) error {
	common.Odl.Debug("tTIdcb: receiveCommon start", "fromOdny", fromOdny)
	var err error

	if fromOdny {
		v, err := mar.UnmarshalUB2(ctx)
		if err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal numUDS (ODNY)", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		p.numUDS = driverCommon.UB4(v)
	} else {
		if p.numUDS, err = mar.UnmarshalUB4(ctx); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal numUDS", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		if p.numUDS > 0 {
			if _, err = mar.UnmarshalUB1(ctx); err != nil {
				common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal UB1 after numUDS", "error", err)
				return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
			}
		}
	}

	// Check if the number of columns return is not larger than the maximum number
	// of columns before allocation
	if p.numUDS > _maxColCount {
		common.Odl.Warn(fmt.Sprintf("tTIdcb.receiveCommon: column count of [%d] exceeds maximum of %d columns, ", p.numUDS, _maxColCount))
		return common.NewOracleError(oracleErrors.FailUnmarshal, nil, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// Allocate colNames if number of UDS > 0
	if p.numUDS > 0 {
		p.colNames = make([]driverCommon.B1Array, p.numUDS)
		p.udsArr = make([]driverCommon.UnMarshallable, p.numUDS)
	}

	if p.numUDS > 0 && p.newUDS == nil {
		common.Odl.Warn("tTIdcb.receiveCommon: newUDS factory not initialized")
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	for i := 0; i < int(p.numUDS); i++ {
		uds := p.newUDS()
		if err := uds.UnMarshalFrom(ctx, mar); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal column", "index", i, "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		p.udsArr[i] = uds
		cn := uds.(interface{ getColumnName() driverCommon.B1Array })
		p.colNames[i] = cn.getColumnName()
		common.Odl.Debug("tTIdcb: unmarshalled column", "index", i, "colName", p.colNames[i])
	}

	if !fromOdny {
		// current date - ignore
		var date dynamicAllocatedArray
		if err = date.UnMarshalFrom(ctx, mar); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal current date", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		// dcb flag
		if _, err = mar.UnmarshalUB4(ctx); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal dcb flag", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		// dcbmdbz
		if _, err = mar.UnmarshalUB4(ctx); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal dcbmdbz", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		// dcbmnpr
		if _, err = mar.UnmarshalUB4(ctx); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal dcbmnpr", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		// dcbmxpr
		if _, err = mar.UnmarshalUB4(ctx); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal dcbmxpr", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		// query compile key
		var queryCKey dynamicAllocatedArray
		if err = queryCKey.UnMarshalFrom(ctx, mar); err != nil {
			common.Odl.Warn("tTIdcb.receiveCommon: failed to unmarshal query compile key", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		p.queryCompileKey = queryCKey.value
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("tTIdcb: receiveCommon done", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}

// newTTIdcb17 creates and initializes a new tTIdcb17 instance (protocol version 17-19)
func newTTIdcb17() driverCommon.Message[driverCommon.MessageType] {
	obj := &tTIdcb{}
	obj.newUDS = newTTIuds17
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("tTIdcb17: newTTIdcb17 constructor", "struct", fmt.Sprintf("%+v", obj))
	}
	return obj
}

// newTTIdcb20 creates and initializes a new tTIdcb20 instance (protocol version 20-23)
func newTTIdcb20() driverCommon.Message[driverCommon.MessageType] {
	obj := &tTIdcb{}
	obj.newUDS = newTTIuds20
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("tTIdcb20: newTTIdcb20 constructor", "struct", fmt.Sprintf("%+v", obj))
	}
	return obj
}

// newTTIdcb24 creates and initializes a new tTIdcb24 instance (protocol version 24+)
func newTTIdcb24() driverCommon.Message[driverCommon.MessageType] {
	obj := &tTIdcb{}
	obj.newUDS = newTTIuds24
	common.Odl.Debug("tTIdcb24: newTTIdcb24 constructor", "struct", fmt.Sprintf("%+v", obj))
	return obj
}
