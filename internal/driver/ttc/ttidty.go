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
	"context"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	al32Utf8CharSet   driverCommon.UB2 = 873
	al16Utf16CharSet  driverCommon.UB2 = 2000
	dbcsCharSet       driverCommon.UB2 = 0xffff // -1
	ucs2CharSet       driverCommon.UB2 = 0xfbff // -5
	al24UtffssCharSet driverCommon.UB2 = 870
	utf8CharSet       driverCommon.UB2 = 871
)

// "empty" timezone bytes to be sent based on caps
// see MashalTo method
var _emptyTimeZoneBytes = []byte{0, 0, 0, 0}

// tTIdty represents the type and data representation negotiation protocol for a database session.
type tTIdty struct {
	cliRIN                driverCommon.UB2 // Input character set code
	cliROUT               driverCommon.UB2 // Output character set code
	negotiatedCaps        *capability
	negotiatedCapsMap     map[string]driverCommon.Capability
	timeZoneVersionNumber byte
}

// NewTTIdty creates and returns a tTIdty protocol struct for database type/representations negotiation on the given connection.
func NewTTIdty() driverCommon.Message[driverCommon.MessageType] {
	obj := &tTIdty{
		cliRIN:                al32Utf8CharSet,
		cliROUT:               al32Utf8CharSet,
		negotiatedCaps:        nil,
		timeZoneVersionNumber: 0,
	}

	return obj
}

// SetNegotiatedCapabilities sets client capabilities
// must be set be for marshaling attemps
func (p *tTIdty) SetNegotiatedCapabilities(caps *capability) {
	// TODO : remove negotiatedCaps as it is not used
	// TODO : it seems that we enter twice here
	p.negotiatedCaps = caps
	p.negotiatedCapsMap = caps.toMap()
	common.Odl.Debug("tTIdty.SetCapabilities: Capabilities set in dty message", "capabilities", p.negotiatedCapsMap)
}

// GetMsgCode gets message code
func (p *tTIdty) GetMsgCode() driverCommon.MessageType {
	return TTIDTY
}

// MarshalTo serializes the tTIdty structure into the marshal engine.
func (p *tTIdty) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	var err error
	common.Odl.Debug("tTIdty.MarshalTo: Start marshalling dty message")

	if p.negotiatedCaps == nil {
		common.Odl.Warn("Negotiated capabilities not set, can't marshal")
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])

	}

	// Convert to Little Endian format for TTC protocol compatability
	// This message is used during protocol negotiation. At that stage, we assume that the marshaller
	// we are using is BIG ENDIAN (the default one used until negotiation is completed.)
	// By design, the cliRIN field is always encoded in LITTLE ENDIAN. We then have to change
	// the order before writing it to the buffer.
	var cliRINLE = (uint16(p.cliRIN)&0xFF)<<8 | (uint16(p.cliRIN) >> 8)
	if err = mar.MarshalUB2(ctx, driverCommon.UB2(cliRINLE)); err != nil {
		common.Odl.Warn("Failed to marshal message", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// Convert to Little Endian format for TTC protocol compatability
	// This message is used during protocol negotiation. At that stage, we assume that the marshaller
	// we are using is BIG ENDIAN (the default one used until negotiation is completed.)
	// By design, the cliROUT field is always encoded in LITTLE ENDIAN. We then have to change
	// the order before writing it to the buffer.
	var cliROUTLE = (uint16(p.cliROUT)&0xFF)<<8 | (uint16(p.cliROUT) >> 8)
	if err = mar.MarshalUB2(ctx, driverCommon.UB2(cliROUTLE)); err != nil {
		common.Odl.Warn("Failed to marshal message", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// marshal cliFlags (protocol flags) field
	if err = mar.MarshalUB1(ctx, driverCommon.UB1(typeRepresentationTable.getFlags())); err != nil {
		common.Odl.Warn("Failed to marshal protocol flags", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	if err = p.negotiatedCaps.MarshalTo(ctx, mar); err != nil {
		common.Odl.Warn("Failed to marshal the negotiated capabilities", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// Check if the Runtime Compatibility has time zone in it, if so
	// send the time zone bytes
	if p.negotiatedCapsMap[kpccapRtTzEx].IsSet {
		tza := driverCommon.GetTimeZoneBytes()
		if err := mar.MarshalB1Array(ctx, tza); err != nil {
			common.Odl.Warn("Failed to marshal timezone bytes", "error", err)
			return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		// Now if the server can understand the time zone version, send the
		// time zone version number to the server.
		// Since, we do not have any timezone on the client we send the
		// timezone version as "0"
		if p.negotiatedCapsMap[kpccapCtbTtc3Tzver].IsSet {
			if err := mar.MarshalB1Array(ctx, _emptyTimeZoneBytes); err != nil {
				common.Odl.Warn("Failed to marshal timezone bytes", "error", err)
				return common.NewOracleError(oracleErrors.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
			}
		}
	}
	common.Odl.Debug("tTIdty.MarshalTo: marshalling representation table")
	return typeRepresentationTable.MarshalTo(ctx, mar)
}

// UnMarshalFrom unmarshal's the response packet for type negotiation.
func (p *tTIdty) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	var err error
	common.Odl.Debug("tTIdty.UnMarshalFrom: Start Unmarshalling dty message")
	if p.negotiatedCaps == nil {
		common.Odl.Warn("Can't unmarshal,  capabilities not set")
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])

	}

	isTimeZoneInfoSupported := p.negotiatedCapsMap[kpccapRtTzEx].IsSet
	isTtc3Capability := p.negotiatedCapsMap[kpccapCtbTtc3Tzver].IsSet
	if isTimeZoneInfoSupported {
		// timezone info supported ? if so unmarshal it
		var ldiIntDaySecond int = 11
		_, err = mar.UnmarshalB1Array(ctx, ldiIntDaySecond)
		if err != nil {
			common.Odl.Warn("Failed to unmarshal ldiIntDaySecond", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		if isTtc3Capability {
			// it is a ub4 but not as universal byte format yet
			b2, err := mar.UnmarshalB1Array(ctx, 4) // ttchnrtzver
			if err != nil {
				common.Odl.Warn("Failed to unmarshal typerep tzversion", "error", err)
				return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])

			}
			p.timeZoneVersionNumber = byte((uint32((b2)[0]) << 24) | (uint32((b2)[1]) << 16) | (uint32((b2)[2]) << 8) | uint32((b2)[3]))
		}

	}

	err = typeRepresentationTable.UnMarshalFrom(ctx, mar)
	if err != nil {
		common.Odl.Warn("Server data type representations unmarshal failed", "error", err)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	return nil
}

// GetClientTTCVersion returns the TTC version supported by the server.
func (p *tTIdty) GetClientTTCVersion() byte {
	return p.negotiatedCapsMap[kpccapCtTtcFldVsn].Value
}

// GetTimeZoneVersionNumber returns the timezone version number.
func (p *tTIdty) GetTimeZoneVersionNumber() byte {
	return p.timeZoneVersionNumber
}

func isCharSetMultibyte(charSet driverCommon.UB2) bool {
	switch charSet {
	case al24UtffssCharSet, utf8CharSet, ucs2CharSet, dbcsCharSet, al32Utf8CharSet, al16Utf16CharSet:
		return true
	default:
		return false
	}
}
