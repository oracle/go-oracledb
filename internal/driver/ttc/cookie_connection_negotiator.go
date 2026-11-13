package ttc

import (
	"context"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

type cookieConnectionNegotiator struct {
	*connectionNegotiatorCommon
	cookie *ttiCookie
}

// newCookieConnectionNegotiator binds cached negotiation data to the connection buffer.
func newCookieConnectionNegotiator(buf driverCommon.DataBuffer, cookie *ttiCookie) *cookieConnectionNegotiator {
	shared := &connectionNegotiatorCommon{
		_dataBuffer: buf,
	}
	return &cookieConnectionNegotiator{
		connectionNegotiatorCommon: shared,
		cookie:                     cookie,
	}
}

// Negotiate queues cookie negotiation and returns the prepared session and shelf.
// It performs no I/O and returns no new cookie; queueing errors are propagated.
func (c cookieConnectionNegotiator) Negotiate(ctx context.Context) (*driverCommon.SessionContext, *ttiShelf[driverCommon.MessageType], *ttiCookie, error) {
	shelf, sessCtx, msgStmr, _ := c.initializeNegotiationResources()
	pro := c.cookie.newProtocolMessage()
	// Register negotiated server capabilities
	shelf.RegisterCapabilities(pro.serverCaps.toMap())
	if !isCharSetMultibyte(pro.svrCharSet) {
		typeRepresentationTable.SetFlags(TTCLXMCONV)
	}

	dty := NewTTIdty().(*tTIdty)
	dty.SetNegotiatedCapabilities(pro.clientCaps)

	err := msgStmr.Push(ctx, pro)
	if err != nil {
		return nil, nil, nil, err
	}

	err = msgStmr.Push(ctx, c.cookie)
	if err != nil {
		return nil, nil, nil, err
	}

	err = msgStmr.Push(ctx, dty)
	if err != nil {
		return nil, nil, nil, err
	}
	common.Odl.Fine("cookieConnectionNegotiator: TTIPRO, TTICOOKIE, and TTIDTY queued for the first authentication request")

	c.applyNegotiatedSettings(shelf, sessCtx, pro, dty)

	prepareDTYResponse := func(header *messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		msg, err := shelf.GetMessageFactory().(Factory).GetMessage(TTIDTY)
		if err != nil {
			return nil, err
		}
		msg.(*tTIdty).SetNegotiatedCapabilities(pro.clientCaps)
		return msg, nil
	}
	updateTimeZoneVersion := func(msg driverCommon.Message[driverCommon.MessageType], decodeErr error) (bool, error) {
		if decodeErr != nil {
			return true, decodeErr
		}
		dty := msg.(*tTIdty)
		sessCtx.SetTimeZoneVersionNumber(dty.GetTimeZoneVersionNumber())
		common.Odl.Fine("cookieConnectionNegotiator: Cookie datatype negotiation completed", "timeZoneVersion", dty.GetTimeZoneVersionNumber())

		return true, nil
	}
	// DTY arrives during authentication, after Negotiate returns. Prepare its
	// client capabilities before decoding and apply the returned timezone version
	// afterward. The cookie instantiator removes these callbacks when setup ends.
	msgStmr.RegisterPostUnmarshallCallback(TTIDTY, updateTimeZoneVersion)
	msgStmr.RegisterPreUnmarshallCallback(TTIDTY, prepareDTYResponse)

	return sessCtx, shelf, nil, nil
}

// unregisterCookieNegotiationCallbacks removes setup-only DTY callbacks.
func unregisterCookieNegotiationCallbacks(
	msgStmr MessageStreamerInterface,
) {
	msgStmr.UnRegisterPreUnmarshallCallback(TTIDTY)
	msgStmr.UnRegisterPostUnmarshallCallback(TTIDTY)
}
