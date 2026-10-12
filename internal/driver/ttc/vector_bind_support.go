package ttc

import (
	"context"
	"database/sql/driver"
	"reflect"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

var (
	vectorFloat64Type = reflect.TypeOf(driverCommon.VectorFloat64(nil))
	vectorFloat32Type = reflect.TypeOf(driverCommon.VectorFloat32(nil))
	vectorInt8Type    = reflect.TypeOf(driverCommon.VectorInt8(nil))
	vectorBinaryType  = reflect.TypeOf(driverCommon.VectorBinary(nil))
)

func isVectorBindType(goType reflect.Type) bool {
	switch goType {
	case vectorFloat64Type, vectorFloat32Type, vectorInt8Type, vectorBinaryType:
		return true
	default:
		return false
	}
}

func checkVectorNamedValue(nv *driver.NamedValue) error {
	if nv == nil {
		return driver.ErrSkip
	}
	if isVectorBindType(reflect.TypeOf(nv.Value)) {
		return nil
	}
	return driver.ErrSkip
}

func newVectorBindValue(payload driverCommon.B1Array) bindValue {
	return bindValue{kind: bindValueKindVector, payload: payload}
}

// marshalVectorBind writes the native VECTOR payload with its BLOB quasi-locator.
func marshalVectorBind(
	ctx context.Context,
	engine driverCommon.Marshaller,
	msgCode driverCommon.MessageType,
	index int,
	payload driverCommon.B1Array,
) error {
	if payload == nil {
		if err := engine.MarshalUB4(ctx, 0); err != nil {
			common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector null length",
				"error", err, "stage", "vector-null-length", "index", index)
			return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
		}
		return nil
	}

	locator := buildVectorQuasiLocator(len(payload))
	locLen := len(locator)
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(locLen)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector locator length",
			"error", err, "stage", "vector-locator-length", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	if err := engine.MarshalCLR(ctx, locator, 0, locLen); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector locator",
			"error", err, "stage", "vector-locator", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	if err := engine.MarshalCLR(ctx, payload, 0, len(payload)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector payload",
			"error", err, "stage", "vector-payload", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	return nil
}
