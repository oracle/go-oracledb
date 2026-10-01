package ttc

import (
	"context"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

type bindValueKind uint8

const (
	bindValueKindCLR bindValueKind = iota
	bindValueKindVector
)

// bindValue keeps the encoded bytes and the TTC representation TTIRXD must use.
type bindValue struct {
	kind    bindValueKind
	payload driverCommon.B1Array
}

func newCLRBindValue(payload driverCommon.B1Array) bindValue {
	return bindValue{
		kind:    bindValueKindCLR,
		payload: payload,
	}
}

func (v bindValue) marshal(ctx context.Context, engine driverCommon.Marshaller, msgCode driverCommon.MessageType, index int) error {
	if v.kind == bindValueKindVector {
		return marshalVectorBind(ctx, engine, msgCode, index, v.payload)
	}
	return marshalCLRBind(ctx, engine, msgCode, index, v.payload)
}

// marshalCLRBind marshals ordinary binds using a single CLR value.
func marshalCLRBind(
	ctx context.Context,
	engine driverCommon.Marshaller,
	msgCode driverCommon.MessageType,
	index int,
	payload driverCommon.B1Array,
) error {
	if payload == nil {
		if err := engine.MarshalUB1(ctx, driverCommon.UB1(0)); err != nil {
			common.Odl.Error("tTIrxd.MarshalTo: failed to write null length indicator",
				"error", err, "stage", "null-indicator", "index", index)
			return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
		}
		return nil
	}
	if err := engine.MarshalCLR(ctx, payload, 0, len(payload)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to write CLR",
			"error", err, "stage", "clr", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	return nil
}
