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

	"github.com/oracle/go-driver/driver/common"
)

// tTIpro represents a TTIPRO protocol message exchanged for feature and charset negotiation.
// It stores protocol and server compatibility metadata as received from the database server.
type tTIpro struct {
	svrCharSet         common.UB2     // Database server charset (TTC response)
	svrCharSetElem     common.UB2     // Charset element value from server (TTC response)
	svrFlags           byte           // DB charset flags (TTC response)
	svrPortDescription common.B1Array // Server description/version string (e.g. "x86_64/Linux. 2.4.xx")
	proSvrVer          common.UB2     // Server protocol version
	oVersion           common.UB2     // Oracle server version
	NCharCharset       common.UB2     // Server NCHAR charset (TTC response)
	ServerCaps         *Capability    // Negotiated server runtime and compile-time capabilities
	ClientCaps         *Capability    // Client capabilities updated based on server capabilities
}

var (
	// proCliVerTTC8Slice declares the supported protocol version signature sent to the server.
	proCliVerTTC8Slice = []byte{6, 5, 4, 3, 2, 1, 0}
	// proCliStrTTC8Slice declares the protocol client identity string.
	proCliStrTTC8Slice = []byte("GO_TTC-8.2.0\x00")
)

// NewTTIpro constructs a zeroed tTIpro message with default capabilities.
func NewTTIpro() common.Message[common.MessageType] {
	common.Odl.Debug("tTIpro.NewTTIpro: Creating tTIpro message")
	return &tTIpro{
		NCharCharset: 0,
		ServerCaps:   NewCapability(),
		ClientCaps:   NewDefaultCapability(),
	}
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC protocol message code.
func (p *tTIpro) GetMsgCode() common.MessageType {
	return TTIPRO
}

// UnMarshalFrom reads, validates, and populates a tTIpro structure from a server TTC response.
// Returns an error for any protocol violation or underflow.
func (p *tTIpro) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	common.Odl.Debug("tTIpro.UnMarshalFrom: Start unmarshalling protocol message")
	ub1, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal protocol version UB1", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.proSvrVer = common.UB2(ub1)
	common.Odl.Debug("Server version from TTIpro", "version", p.proSvrVer)

	// Protocol version validation
	if p.proSvrVer < MinTtcverSupported || p.proSvrVer > MaxTtcverSupported {
		common.Odl.Warn("Invalid TTC version", "TTIPRO server version", p.proSvrVer)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// setting it to min version
	p.oVersion = MinOversionSupported

	// Skip reserved byte
	_, err = mar.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal reserved UB1", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// Unmarshal server port description
	p.svrPortDescription, err = mar.UnmarshalText(ctx, 50)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal port description", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	common.Odl.Debug("tTIpro.UnMarshalFrom: unmarshalled server port description",
		"proSvrVer", p.proSvrVer,
		"svrPortDescriptionRaw", p.svrPortDescription,
		"svrPortDescription", string(p.svrPortDescription))

	// Unmarshal server character set
	tmpBuffer, err := mar.UnmarshalB1Array(ctx, 2)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal server character set UB2", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.svrCharSet = ((common.UB2((tmpBuffer)[1]) << 8) & 0xFF00) | (common.UB2((tmpBuffer)[0]) & 0xFF)

	// Unmarshal server flags
	code, err := mar.UnmarshalUB1(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal server flags UB1", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.svrFlags = byte(code)

	// Unmarshal server character set element
	tmpBuffer, err = mar.UnmarshalB1Array(ctx, 2)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal server character set element UB2", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.svrCharSetElem = ((common.UB2((tmpBuffer)[1]) << 8) & 0xFF00) | (common.UB2((tmpBuffer)[0]) & 0xFF)

	// If there are charset elements, unmarshal them
	if p.svrCharSetElem > 0 {
		_, err := mar.UnmarshalB1Array(ctx, int(p.svrCharSetElem*5))
		if err != nil {
			common.Odl.Warn("Failed to unmarshal charset elements array", "length", int(p.svrCharSetElem*5), "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
	}

	// If a TTC4 server, then don't get the FDO.
	if p.proSvrVer < 5 {
		return nil
	}

	// Unmarshal FDO (Feature Data Object)
	fdoLength, err := mar.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal FDO length UB2", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	if fdoLength == 0 {
		common.Odl.Warn("Invalid protocol, FDO length is zero", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	fdo, err := mar.UnmarshalB1Array(ctx, int(fdoLength))
	if err != nil {
		common.Odl.Warn("Failed to unmarshal FDO array", "length", fdoLength, "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}

	// Calculate offset for NCHAR charset
	i := 6 + ((fdo)[5] & 0xff) + ((fdo)[6] & 0xff)

	// Unmarshal NCHAR charset
	if int(i+4) >= len(fdo) {
		common.Odl.Warn("Invalid FDO length or offset", "offset", i+4, "length", len(fdo), "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.NCharCharset = common.UB2((uint16((fdo)[i+3]) & 0xFF) << 8)
	p.NCharCharset |= common.UB2((fdo)[i+4]) & 0xFF

	if p.proSvrVer < 6 {
		return nil
	}

	err = p.ServerCaps.UnMarshalFrom(ctx, mar)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal server caps", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	p.ClientCaps.AdjustCapabilityFrom(p.ServerCaps)

	// too verbose, do not print this by default
	// logging.Odl.Debug("tTIpro.UnMarshalFrom: Completed unmarshalling protocol message", "serverCaps", p.ServerCaps, "clientCaps", p.ClientCaps)
	return nil
}

// MarshalTo serializes a tTIpro message into TTC format for wire transmission.
// Returns an error if the operation fails.
func (p *tTIpro) MarshalTo(ctx context.Context, mar common.Marshaller) error {
	common.Odl.Debug("tTIpro.MarshalTo: Start marshalling protocol message")
	err := mar.MarshalB1Array(ctx, proCliVerTTC8Slice)
	if err != nil {
		common.Odl.Warn("Error marshalling tTIpro", "error", err)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	err = mar.MarshalB1Array(ctx, proCliStrTTC8Slice)
	if err != nil {
		common.Odl.Warn("Error marshalling tTIpro", "error", err)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
	}
	common.Odl.Debug("tTIpro.MarshalTo: marshalling done")
	return nil
}

// GetProtocolVersion returns the protocol version reported by the server.
func (p *tTIpro) GetProtocolVersion() common.UB2 { return p.proSvrVer }

// GetOracleVersion returns the Oracle version value reported by the database.
func (p *tTIpro) GetOracleVersion() common.UB2 { return p.oVersion }

// GetServerRuntimeCapabilities returns the server runtime capability flags.
func (p *tTIpro) GetServerRuntimeCapabilities() *[]byte { return &p.ServerCaps.runTimeCapabilities }

// GetServerCompileTimeCapabilities returns the server compile-time capabilities flags.
func (p *tTIpro) GetServerCompileTimeCapabilities() *[]byte {
	return &p.ServerCaps.compileTimeCapabilities
}

// GetCharacterSet returns the main character set reported by the server.
func (p *tTIpro) GetCharacterSet() common.UB2 { return p.svrCharSet }

// GetNCharCharacterSet returns the NCHAR character set value.
func (p *tTIpro) GetNCharCharacterSet() common.UB2 { return p.NCharCharset }

// GetFlags returns tTIpro's protocol flags (bit flags).
func (p *tTIpro) GetFlags() byte { return p.svrFlags }

// GetSvrPortDescription returns the raw server description/version string.
func (p *tTIpro) GetSvrPortDescription() common.B1Array { return p.svrPortDescription }
