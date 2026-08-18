package ttc

import (
	"context"

	"github.com/oracle/go-driver/driver/common"
)

// bindTransport owns the TTC wire representation for a prepared bind value.
type bindTransport interface {
	marshal(context.Context, common.Marshaller, common.MessageType, int, bindValue) error
}

// bindValue stores an encoded bind value together with the transport used to marshal it.
type bindValue struct {
	transport bindTransport
	payload   common.B1Array
	isNull    bool
}

var defaultCLRBindTransport bindTransport = clrBindTransport{}

func newCLRBindValue(payload common.B1Array) bindValue {
	return bindValue{
		transport: defaultCLRBindTransport,
		payload:   payload,
		isNull:    payload == nil,
	}
}

func (v bindValue) marshal(ctx context.Context, engine common.Marshaller, msgCode common.MessageType, index int) error {
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
	engine common.Marshaller,
	msgCode common.MessageType,
	index int,
	bind bindValue,
) error {
	if bind.isNull || bind.payload == nil {
		if err := engine.MarshalUB1(ctx, common.UB1(0)); err != nil {
			common.Odl.Error("tTIrxd.MarshalTo: failed to write null length indicator",
				"error", err, "stage", "null-indicator", "index", index)
			return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[msgCode])
		}
		return nil
	}
	if err := engine.MarshalCLR(ctx, bind.payload, 0, len(bind.payload)); err != nil {
		common.Odl.Error("tTIrxd.MarshalTo: failed to write CLR",
			"error", err, "stage", "clr", "index", index)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[msgCode])
	}
	return nil
}
