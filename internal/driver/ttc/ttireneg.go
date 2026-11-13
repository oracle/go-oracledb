package ttc

import (
	"context"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
)

type tTIReneg struct {
}

// newTTIReneg creates the server's TTC renegotiation marker.
func newTTIReneg() common.Message[common.MessageType] {
	return &tTIReneg{}
}

// GetMsgCode identifies the TTC renegotiation message.
func (*tTIReneg) GetMsgCode() common.MessageType { return TTIRENEG }

// UnmarshalFrom leaves the buffer unchanged because TTIRENEG has no payload.
func (*tTIReneg) UnmarshalFrom(ctx context.Context, mar common.Marshaller) error {
	return nil // TTIRENEG itself has no payload
}
