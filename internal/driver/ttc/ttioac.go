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
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc/converters"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	// _uacFlagIndicator is true if using indicators.
	_uacFlagIndicator driverCommon.UB1 = 0x01
	// _uacFlagLengthVector is true if using length vector.
	_uacFlagLengthVector driverCommon.UB1 = 0x02
	// _uacFlagContinuationNoadj is default V8 behaviour, ttcoac does not change value.
	_uacFlagContinuationNoadj driverCommon.UB8 = 0x00000010
	// _oacMaxLengthNumber is the fixed max length for NUMBER columns.
	_oacMaxLengthNumber driverCommon.UB4 = 22
	// _oacMaxLengthDate is the fixed max length for DATE columns.
	_oacMaxLengthDate driverCommon.UB4 = 7
	// _oacMaxLengthStampTZ is the fixed max length for TIMESTAMP WITH TIMEZONE columns.
	_oacMaxLengthStampTZ driverCommon.UB4 = 13
)

// tTIoac represents the Oracle TTIOAC structure, which holds metadata and state for column binding and definition
// in the Oracle network protocol (TTC layer). It contains information about data types, character sets, precision,
// scale, array sizes, and other attributes required for marshalling and unmarshalling data between Go and Oracle.
type tTIoac struct {
	// characterSetID is the character set ID for this column.
	characterSetID driverCommon.UB2
	// characterSetForm specifies the character set form (e.g., implicit/explicit, AL16UTF16, etc.)
	characterSetForm driverCommon.UB1
	// dataType is the wire protocol data type (e.g., DtyNum, DtyChr, etc.) for this column.
	dataType driverCommon.UB1
	// flags holds protocol flags for binding/definition properties.
	flags driverCommon.UB1
	// precision specifies the numeric precision (for NUMBER columns) or string length semantics.
	precision driverCommon.UB1
	// scale specifies the numeric scale (for NUMBER columns).
	scale driverCommon.SB1
	// maxLength is the maximum number of bytes allowed for this column's value.
	maxLength driverCommon.UB4
	// codepointLengthLimit is the column length in codepoints (character count), if applicable.
	codepointLengthLimit driverCommon.UB4
	// collationID specifies the collation ID of this column.
	collationID driverCommon.UB4
	// nbArrayElements is the total number of elements for array/batch operations.
	nbArrayElements driverCommon.UB4
	// versionNumber is the type version for Oracle ADT types.
	versionNumber driverCommon.UB2
	// flagsContinuation stores additional ("continuation") flags for the server attribute.
	flagsContinuation driverCommon.UB8
	// toid holds the type object identifier if dynamic types are used (object, collection types).
	toid *driverCommon.B1Array
	// requestedtype is the client-original Go type or source type requested.
	requestedtype int16
}

/*
newTTIoac creates TTIoac instance with type and buffer length metadata for a column.
It sets all protocol-relevant fields (dataType, maxLength, flags, etc.) according to the provided type
and buffer size to the specified column.

Parameters:
  - typ: Data type code (int16); determines the protocol or wire format for this column.
  - maxLength: Maximum length in bytes (int) for the buffer to use for this value.

Usage:

	Call this constructore before Marshalling the TTIoac for binding or defining a column.
*/
func newTTIoac(typ DtyType, maxLength driverCommon.UB4) *tTIoac {
	common.Odl.Debug("TTIoac.newTTIoac called", "requestedtype", typ, "maxLength", maxLength)
	obj := &tTIoac{}
	obj.requestedtype = typ
	/*
	   For each typ, we need the network representation. For most of the types,
	   there is a direct match so we set datatype = _type.
	   However for some types, we choose a different
	   representation on the network. The network represenation for each
	   DTY type is negotiated with server during the TTC handshake.
	   For example:

	   For DtyVCS, we use DtyChr instead of DtyVCS because:
	      typeRepresentationTable.addTypeRepToTable(DtyVCS, DtyChr, int16(RepCUnv))
	   refer to pkg_init for the mappings.
	*/
	switch typ {
	case DtyVCS, DtyChr:
		obj.dataType = driverCommon.UB1(DtyChr)
	case DtyRdd:
		obj.dataType = driverCommon.UB1(DtyRiD)
	case DtyVnu, DtyNum:
		obj.dataType = driverCommon.UB1(DtyNum)
	case DtyVbi:
		obj.dataType = driverCommon.UB1(DtyBin)
	case DtyRSet:
		obj.dataType = driverCommon.UB1(DtyCur)
	default:
		obj.dataType = driverCommon.UB1(typ)
	}

	if obj.dataType == driverCommon.UB1(DtyChr) || obj.dataType == driverCommon.UB1(DtyAfc) {
		obj.flagsContinuation = _uacFlagContinuationNoadj
	} else {
		obj.flagsContinuation = 0
	}

	if obj.dataType == driverCommon.UB1(DtyCur) {
		obj.maxLength = 4
	} else {
		obj.maxLength = maxLength
	}

	obj.flags = _uacFlagIndicator | _uacFlagLengthVector
	obj.scale = 0
	obj.characterSetID = al32Utf8CharSet
	obj.characterSetForm = FormChar

	common.Odl.Debug("TTIoac.Init completed",
		"dataType", obj.dataType,
		"flags", obj.flags,
		"maxLength", obj.maxLength,
		"flagsContinuation", obj.flagsContinuation,
	)
	return obj
}

/*
newTTIOAcDefine creates a define-specific TTIoac instance using column metadata and prefetch configuration.

Parameters:
  - typ: Data type code for the define OAC.
  - maxLength: Maximum define length in bytes.
  - colCtx: Column metadata providing charset information.
  - flags: Additional continuation flags to OR with the standard define flag.
  - prefetchSize: LOB prefetch size applied to codepointLengthLimit.
*/
func newTTIOAcDefine(
	typ DtyType,
	maxLength driverCommon.UB4,
	colCtx columnContext,
	flags driverCommon.UB8,
	prefetchSize driverCommon.UB4,
) *tTIoac {
	oac := newTTIoac(typ, maxLength)
	oac.characterSetForm = driverCommon.UB1(colCtx.CharsetForm)
	oac.characterSetID = driverCommon.UB2(colCtx.CharsetID)
	oac.flagsContinuation = uacflsz | flags
	oac.codepointLengthLimit = prefetchSize
	return oac
}

// newTTIOacString creates an OAC descriptor for string bind values using VARCHAR semantics.
func newTTIOacString(maxLength driverCommon.UB4) driverCommon.Marshallable {
	if maxLength == 0 {
		maxLength = converters.EmptyStringOacLength
	}
	return newTTIoac(DtyVCS, maxLength)
}

// newTTIOacNumber creates an OAC descriptor for Oracle NUMBER values using the negotiated maximum NUMBER length.
func newTTIOacNumber() driverCommon.Marshallable {
	return newTTIoac(DtyNum, converters.MaxNumberLength)
}

// newTTIOacBytes creates an OAC descriptor for raw byte bind values.
func newTTIOacBytes(maxLength driverCommon.UB4) driverCommon.Marshallable {
	return newTTIoac(DtyVbi, maxLength)
}

// newTTIOacNull creates an OAC descriptor for null bind values using the driver's null placeholder length.
func newTTIOacNull() driverCommon.Marshallable {
	return newTTIoac(DtyVCS, converters.MaxNullLength)
}

// newTTIOacBoolV17 creates an OAC descriptor for pre-native-boolean representations that are sent as NUMBER values.
func newTTIOacBoolV17(maxLength driverCommon.UB4) driverCommon.Marshallable {
	return newTTIoac(DtyNum, maxLength)
}

// newTTIOacBool creates an OAC descriptor for native Oracle BOOLEAN values.
func newTTIOacBool() driverCommon.Marshallable {
	return newTTIoac(DtyBol, converters.MaxBoolLength)
}

// newTTIOacTime creates an OAC descriptor for timestamp-with-time-zone values.
func newTTIOacTime() driverCommon.Marshallable {
	return newTTIoac(DtyStz, converters.MaxTimeStampLength)
}

// newTTIOacJSONDefine creates a define OAC descriptor for JSON values transported as LOBs with prefetch enabled.
func newTTIOacJSONDefine(columnContext columnContext, lobPrefetchSize driverCommon.UB4) driverCommon.Marshallable {
	return newTTIOAcDefine(DtyBlob, max_lob_length, columnContext, uacfsald, lobPrefetchSize)
}

// newTTIOacClobDefine creates a define OAC descriptor for CLOB values using column metadata and LOB prefetch settings.
func newTTIOacClobDefine(columnContext columnContext, lobPrefetchSize driverCommon.UB4) driverCommon.Marshallable {
	return newTTIOAcDefine(columnContext.DataType, max_lob_length, columnContext, 0, lobPrefetchSize)
}

// newTTIOacBlobDefine creates a define OAC descriptor for BLOB values using column metadata and LOB prefetch settings.
func newTTIOacBlobDefine(columnContext columnContext, lobPrefetchSize driverCommon.UB4) driverCommon.Marshallable {
	return newTTIOAcDefine(columnContext.DataType, max_lob_length, columnContext, 0, lobPrefetchSize)
}

// newTTIOacVarcharDefine creates a define OAC descriptor for VARCHAR-like scalar columns.
func newTTIOacVarcharDefine(columnContext columnContext) driverCommon.Marshallable {
	return newTTIoac(columnContext.DataType, define_maxlength_varchar)
}

// newTTIOacScalarDefine creates a define OAC descriptor for fixed-size scalar column values.
func newTTIOacScalarDefine(columnContext columnContext) driverCommon.Marshallable {
	return newTTIoac(columnContext.DataType, define_maxlength_scalar)
}

// MarshalTo serializes the TTIoac type information into the network buffer.
// It returns an error if marshalling fails.
func (p *tTIoac) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTIoac.MarshalTo called")
	if err := mar.MarshalUB1(ctx, p.dataType); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal dataType", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB1(ctx, p.flags); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")

	}

	if err := mar.MarshalUB1(ctx, p.precision); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal precision", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if p.dataType == driverCommon.UB1(DtyNum) ||
		p.dataType == driverCommon.UB1(DtyStamp) ||
		p.dataType == driverCommon.UB1(DtyStz) ||
		p.dataType == driverCommon.UB1(DtySitz) ||
		p.dataType == driverCommon.UB1(DtyIds) {
		if err := mar.MarshalUB2(ctx, driverCommon.UB2(p.scale)); err != nil {
			common.Odl.Warn("MarshalTo: Failed to marshal scale as UB2", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
		}
	} else {
		if err := mar.MarshalUB1(ctx, driverCommon.UB1(p.scale)); err != nil {
			common.Odl.Warn("MarshalTo: Failed to marshal scale as UB1", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
		}
	}

	if err := mar.MarshalUB4(ctx, p.maxLength); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal maxLengthn", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB4(ctx, p.nbArrayElements); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal nbArrayElements", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB8(ctx, p.flagsContinuation); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal flagsContinuation", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	var dalc dynamicAllocatedArray
	if p.toid != nil {
		dalc.value = *p.toid
		if err := dalc.MarshalTo(ctx, mar); err != nil {
			common.Odl.Error("MarshalTo: Failed to marshal toid/dalc", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "TTIOAC")
		}
	} else {
		if err := mar.MarshalNullPTR(ctx); err != nil {
			common.Odl.Error("MarshalTo: Failed to marshal toid/dalc", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, "TTIOAC")
		}
	}

	if err := mar.MarshalUB2(ctx, p.versionNumber); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal versionNumber", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB2(ctx, p.characterSetID); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal characterSetID", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB1(ctx, p.characterSetForm); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal characterSetForm", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB4(ctx, p.codepointLengthLimit); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal codepointLengthLimit", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	if err := mar.MarshalUB4(ctx, p.collationID); err != nil {
		common.Odl.Warn("MarshalTo: Failed to marshal collationID", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, "OAC")
	}

	common.Odl.Debug("TTIoac.MarshalTo completed successfully")
	return nil
}

// UnMarshalFrom extracts information from the network buffer and populates the TTIoac fields.
// It returns an error if unmarshalling fails.
func (p *tTIoac) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTIoac.UnMarshalFrom called")
	var err error

	if p.dataType, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal dataType", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.flags, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal flags", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.precision, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal precision", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.scale, err = mar.UnmarshalSB1(ctx); err != nil {
		common.Odl.Debug("UnMarshalFrom: failed to unmarshal scale", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.maxLength, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal maxLength", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.nbArrayElements, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Debug("UnMarshalFrom: failed to unmarshal nbArrayElements", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.flagsContinuation, err = mar.UnmarshalUB8(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal flagsContinuation", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	var dalc dynamicAllocatedArray
	if err = dalc.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal toid/dalc", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}
	p.toid = &dalc.value

	if p.versionNumber, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal versionNumber", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.characterSetID, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal characterSetID", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.characterSetForm, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal characterSetForm", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.codepointLengthLimit, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal codepointLengthLimit", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.collationID, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal collationID", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, "OAC")
	}

	if p.maxLength > 0 {
		switch p.dataType {
		case driverCommon.UB1(DtyNum):
			p.maxLength = _oacMaxLengthNumber
		case driverCommon.UB1(DtyDat):
			p.maxLength = _oacMaxLengthDate
		case driverCommon.UB1(DtyStz):
			p.maxLength = _oacMaxLengthStampTZ
		}
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIoac.UnMarshalFrom completed", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}

// addFlagsContinuation adds a flag to the FlagsContinuation field.
func (p *tTIoac) addFlagsContinuation(flag driverCommon.UB8) {
	p.flagsContinuation = p.flagsContinuation | flag
}
