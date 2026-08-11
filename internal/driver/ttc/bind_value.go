package ttc

import (
	"context"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// bindTransport owns the TTC wire representation for a prepared bind value.
type bindTransport interface {
	marshal(context.Context, driverCommon.Marshaller, driverCommon.MessageType, int, bindValue) error
}

// bindValue stores an encoded bind value together with the transport used to marshal it.
type bindValue struct {
	transport bindTransport
	payload   driverCommon.B1Array
	isNull    bool
}

var defaultCLRBindTransport bindTransport = clrBindTransport{}

func newCLRBindValue(payload driverCommon.B1Array) bindValue {
	return bindValue{
		transport: defaultCLRBindTransport,
		payload:   payload,
		isNull:    payload == nil,
	}
}

func (v bindValue) marshal(ctx context.Context, engine driverCommon.Marshaller, msgCode driverCommon.MessageType, index int) error {
	transport := v.transport
	if transport == nil {
		transport = defaultCLRBindTransport
	}
	return transport.marshal(ctx, engine, msgCode, index, v)
}

// clrBindTransport marshals binds using the generic CLR encoding path.
type clrBindTransport struct{}

func (clrBindTransport) marshal(
	ctx context.Context,
	engine driverCommon.Marshaller,
	msgCode driverCommon.MessageType,
	index int,
	bind bindValue,
) error {
	if bind.isNull || bind.payload == nil {
		if err := engine.MarshalUB1(ctx, driverCommon.UB1(0)); err != nil {
			common.Odl.Error("tTIrxd.MarshalTo: failed to write null length indicator",
				"error", err, "stage", "null-indicator", "index", index)
			return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
		}
		return nil
	}
	if err := engine.MarshalCLR(ctx, bind.payload, 0, len(bind.payload)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to write CLR",
			"error", err, "stage", "clr", "index", index)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	return nil
}
