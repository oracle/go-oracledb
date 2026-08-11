package ttc

import (
	"context"
	"database/sql/driver"
	"reflect"

	"github.com/oracle/pure-go-driver/driver/common"
	"github.com/oracle/pure-go-driver/logging"
)

var (
	vectorFloat64Type = reflect.TypeOf(common.VectorFloat64(nil))
	vectorFloat32Type = reflect.TypeOf(common.VectorFloat32(nil))
	vectorInt8Type    = reflect.TypeOf(common.VectorInt8(nil))
	vectorBinaryType  = reflect.TypeOf(common.VectorBinary(nil))
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
	locator common.B1Array
}

func buildVectorBindValue(_ normalizedBindValue, payload common.B1Array) (bindValue, error) {
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
	engine common.Marshaller,
	msgCode common.MessageType,
	index int,
	bind bindValue,
) error {
	if bind.isNull {
		if err := engine.MarshalUB4(ctx, 0); err != nil {
			logging.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector null length",
				"error", err, "stage", "vector-null-length", "index", index)
			return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[msgCode])
		}
		return nil
	}

	locLen := len(t.locator)
	if err := engine.MarshalUB4(ctx, common.UB4(locLen)); err != nil {
		logging.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector locator length",
			"error", err, "stage", "vector-locator-length", "index", index)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	if err := engine.MarshalCLR(ctx, t.locator, 0, locLen); err != nil {
		logging.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector locator",
			"error", err, "stage", "vector-locator", "index", index)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	if err := engine.MarshalCLR(ctx, bind.payload, 0, len(bind.payload)); err != nil {
		logging.Odl.Error("tTIrxd.MarshalTo: failed to marshal vector payload",
			"error", err, "stage", "vector-payload", "index", index)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	return nil
}
