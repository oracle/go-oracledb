package ttc

import (
	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// connectionNegotiatorCommon provides resource creation and client configuration
// shared by normal and cookie negotiation for one connection.
type connectionNegotiatorCommon struct {
	_dataBuffer driverCommon.DataBuffer
}

// initializeNegotiationResources creates the shelf, session context, marshallers,
// streamer, and initial message factory without exchanging negotiation messages.
func (shared *connectionNegotiatorCommon) initializeNegotiationResources() (*ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext, *MessageStreamer, Factory) {
	common.Odl.Fine("connectionNegotiator: Negotiation start")
	// 1. Create Shelf & Session Context
	shelf, sessCtx := _createShelfAndSessionContext()

	// 2. Fetch new Marshaller & TTC Msg Streamer
	_createAndRegisterMarshallers(shared._dataBuffer, shelf)
	msgStmr := _createAndRegisterMessageStreamer(shelf)

	// 3. Create and register Initial MessageFactory
	msgfactory := _createAndRegisterMessageFactory(shelf, -1)

	return shelf, sessCtx, msgStmr, msgfactory
}

// applyNegotiatedSettings installs the session character sets and the message
// and codec factories for the negotiated TTC version.
func (shared *connectionNegotiatorCommon) applyNegotiatedSettings(shelf *ttiShelf[driverCommon.MessageType], sessCtx *driverCommon.SessionContext, pro *tTIpro, dty *tTIdty) {
	// The driver character set is hard-coded to AL32UTF8 because the Go client currently negotiates
	// AL32UTF8 for cliRIN/cliROUT in TTIDTY and only supports that pairing.
	sessCtx.SetSessionCharacterSets(
		al32Utf8CharSet,
		pro.getNCharCharacterSet(),
	)

	// Determine negotiated TTC version (minimum of client and server)
	clientVersion := dty.GetClientTTCVersion()
	serverVersion := shelf.GetCapabilities()[kpccapCtTtcFldVsn].Value
	negotiatedTTCVersion := serverVersion
	if clientVersion < serverVersion {
		negotiatedTTCVersion = clientVersion
	}
	common.Odl.Fine("Negotiated TTC version", "client version", clientVersion, "server version", serverVersion, "negotiated version", negotiatedTTCVersion)

	// 7. Update Version based MessageFactory
	_ = _createAndRegisterMessageFactory(shelf, int8(negotiatedTTCVersion))

	_createAndRegisterCodecFactory(shelf, int8(negotiatedTTCVersion))

	common.Odl.Fine("connectionNegotiator: Negotiated client settings applied")
}

// _createShelfAndSessionContext manages creation of Shelf and SessionContext.
func _createShelfAndSessionContext() (*ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext) {
	shelf := newShelf[driverCommon.MessageType]()
	common.Odl.Fine("connectionNegotiator: Shelf created")

	sessCtx := driverCommon.NewSessionContext()
	common.Odl.Fine("connectionNegotiator: SessionContext created")
	return shelf, sessCtx
}

// _createAndRegisterMessageStreamer creates a new message streamer and registers it to the shelf.
func _createAndRegisterMessageStreamer(
	shelf *ttiShelf[driverCommon.MessageType],
) *MessageStreamer {
	msgStmr := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(msgStmr)
	common.Odl.Fine("connectionNegotiator: MessageStreamer created and registered")
	return msgStmr
}

// _createAndRegisterMessageFactory creates a new message factory and registers it to the shelf.
func _createAndRegisterMessageFactory(
	shelf *ttiShelf[driverCommon.MessageType], version int8) driverCommon.Factory {
	msgfactory := NewMessageFactoryForProtocol(version, shelf.GetCapabilities())
	shelf.RegisterMessageFactory(msgfactory)
	common.Odl.Fine("connectionNegotiator: MessageFactory created and registered")
	return msgfactory
}

// _createAndRegisterCodecFactory creates a new codec factory and registers it to the shelf.
func _createAndRegisterCodecFactory(
	shelf *ttiShelf[driverCommon.MessageType], version int8) {
	codecFactory := NewCodecFactoryForProtocol(version)
	shelf.RegisterCodecFactory(codecFactory)
	common.Odl.Fine("connectionNegotiator: codecFactory created and registered")
}

// _createAndRegisterMarshallers registers the default marshaller and the native
// marshaller used by negotiation messages, both using the connection buffer.
func _createAndRegisterMarshallers(
	buf driverCommon.DataBuffer,
	shelf *ttiShelf[driverCommon.MessageType],
) {

	native := NewNativeMarshalEngine(buf, driverCommon.BIG_ENDIAN)
	common.Odl.Fine("connectionNegotiator: Creating Native Marshaller")

	universal := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	common.Odl.Fine("connectionNegotiator: Creating TTC Marshaller")

	if props := shelf.GetConnectionProperties(); props != nil {
		native.setDefaultLobPrefetchSize(int64(props.GetDefaultLobPrefetchSize()))
		universal.setDefaultLobPrefetchSize(int64(props.GetDefaultLobPrefetchSize()))
	}
	shelf.RegisterMarshaller(universal)
	shelf.RegisterMarshaller(native, TTIPRO, TTICOOKIE, TTIDTY)
	common.Odl.Fine("connectionNegotiator: Marshaller created/updated and registered")
}
