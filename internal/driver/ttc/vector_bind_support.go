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

type vectorBindTransport struct {
	locator driverCommon.B1Array
}

func buildVectorBindValue(_ normalizedBindValue, payload driverCommon.B1Array) (bindValue, error) {
	transport := &vectorBindTransport{}
	if payload != nil {
		transport.locator = buildVectorQuasiLocator(len(payload))
	}
	return bindValue{
		transport: transport,
		payload:   payload,
		isNull:    payload == nil,
	}, nil
}

func (t *vectorBindTransport) marshal(
	ctx context.Context,
	engine driverCommon.Marshaller,
	msgCode driverCommon.MessageType,
	index int,
	bind bindValue,
) error {
	if bind.isNull {
		if err := engine.MarshalUB4(ctx, 0); err != nil {
			common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector null length",
				"error", err, "stage", "vector-null-length", "index", index)
			return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
		}
		return nil
	}

	locLen := len(t.locator)
	if err := engine.MarshalUB4(ctx, driverCommon.UB4(locLen)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector locator length",
			"error", err, "stage", "vector-locator-length", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	if err := engine.MarshalCLR(ctx, t.locator, 0, locLen); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector locator",
			"error", err, "stage", "vector-locator", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	if err := engine.MarshalCLR(ctx, bind.payload, 0, len(bind.payload)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector payload",
			"error", err, "stage", "vector-payload", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	return nil
}
