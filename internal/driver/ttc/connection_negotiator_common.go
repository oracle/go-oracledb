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
	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// connectionNegotiatorCommon provides resource creation and client configuration
// shared by normal and TTIINIT negotiation for one connection.
type connectionNegotiatorCommon struct {
	_dataBuffer           driverCommon.DataBuffer
	_connectionProperties driverCommon.DriverProperties
}

// initializeNegotiationResources creates the shelf, session context, marshallers,
// streamer, and initial message factory without exchanging negotiation messages.
func (shared *connectionNegotiatorCommon) initializeNegotiationResources() (*ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext, *MessageStreamer, driverCommon.Factory) {
	common.Odl.Fine("connectionNegotiator: Negotiation start")
	// 1. Create Shelf & Session Context and install connection properties
	shelf, sessCtx := _createShelfAndSessionContext()
	shelf.UpdateConnectionProperties(shared._connectionProperties)

	// 2. Fetch new Marshaller & TTC Msg Streamer
	marshallerFactory := _createMarshallerFactory(shared._dataBuffer, shelf)
	msgStmr := _createAndRegisterMessageStreamer(shelf, marshallerFactory)

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
	marshallerFactory *MarshallerFactory,
) *MessageStreamer {
	msgStmr := NewMessageStreamer(shelf, marshallerFactory)
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

// _createMarshallerFactory creates default and native message registrations
// using marshallers bound to the same connection buffer.
func _createMarshallerFactory(
	buf driverCommon.DataBuffer,
	shelf *ttiShelf[driverCommon.MessageType],
) *MarshallerFactory {

	native := NewNativeMarshalEngine(buf, driverCommon.BIG_ENDIAN)
	common.Odl.Fine("connectionNegotiator: Creating Native Marshaller")

	universal := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	common.Odl.Fine("connectionNegotiator: Creating TTC Marshaller")

	if props := shelf.GetConnectionProperties(); props != nil {
		native.setDefaultLobPrefetchSize(int64(props.GetDefaultLobPrefetchSize()))
		universal.setDefaultLobPrefetchSize(int64(props.GetDefaultLobPrefetchSize()))
	}
	factory := NewMarshallerFactory(universal)
	factory.RegisterMarshaller(native, TTIPRO, TTIDTY, TTIINIT)
	common.Odl.Fine("connectionNegotiator: Marshaller created/updated and registered")
	return factory
}
