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
	"log/slog"

	"github.com/oracle/go-driver/driver/common"
)

// tTIlob is the TTIFUN wrapper emitted for OLOBOPS requests.
//
// It combines a generic TTIFUN header with per-request LOB metadata so the
// marshalling layer can stream OLOBOPS invocations to the database.
type tTIlob struct {
	// header is the TTC-compatible envelope that emits the oLobOps function
	// metadata.
	header common.Marshallable
	// lobPayloadDefinition holds the LOB descriptors, offsets, and payload data used to marshal
	// the current request.
	lobPayloadDefinition *lobDefinition
	// definitionConfigured indicates SetDefinition has been invoked with a non-nil definition.
	definitionConfigured bool
}

// newTTIlob creates a TTC message preconfigured for OLOBOPS execution.
//
// Returns:
//   - *tTIlob: message with a TTIFUN header targeting the oLobOps function.
func newTTIlob() common.Message[common.MessageType] {
	return &tTIlob{
		header: &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: oLobOps}},
	}
}

// SetDefinition assigns the LOB definition that will be applied during
// marshalling.
//
// Parameters:
//   - lobDef (*lobDefinition): execution metadata describing source/destination
//     locators, offsets, and buffers. The pointer must be non-nil;
//
// Error:
//
//		passing a nil definition returns an InvalidLOBBuffer oracle error indicating the
//	    definition is missing.
func (m *tTIlob) SetDefinition(lobDef *lobDefinition) error {
	if lobDef == nil {
		common.Odl.Error("TTILob.SetDefinition: nil lobDefinition provided")
		return common.NewOracleError(common.InvalidLOBBuffer, nil, "marshal", TTCMsgTypeDescription[m.GetMsgCode()], "definition missing")
	}
	m.lobPayloadDefinition = lobDef
	m.definitionConfigured = true
	return nil
}

// GetMsgCode reports the TTC message identifier associated with TTIFUN
// envelopes.
//
// Returns:
//   - common.MessageType: TTIFUN, indicating a function wrapper message.
func (m *tTIlob) GetMsgCode() common.MessageType {
	return TTIFUN
}

// GetFuncCode exposes the TTC function opcode emitted for this message.
//
// Returns:
//   - common.FunctionType: the oLobOps function code expected by the server.
func (m *tTIlob) GetFuncCode() common.FunctionType {
	return oLobOps
}

// MarshalTo serializes the TTC OLOBOPS request.
//
// Parameters:
//   - ctx (context.Context): provides cancellation and timeout control for the
//     marshalling workflow.
//   - engine (common.Marshaller): encoder that writes TTC-formatted bytes to
//     the outbound stream.
//
// Returns:
//   - error: non-nil if required LOB metadata is missing or if encoding fails.
//
// Errors:
//   - OGD-00011 (FailMarshal) when pointer, header, or payload marshalling
//     fails, including invalid locator lengths, offsets, or buffer ranges.
func (m *tTIlob) MarshalTo(ctx context.Context, engine common.Marshaller) error {
	if !m.definitionConfigured {
		common.Odl.Error("TTILob.MarshalTo: definition not configured")
		return common.NewOracleError(common.InvalidLOBBuffer, nil, "marshal", TTCMsgTypeDescription[m.GetMsgCode()], "definition not configured")
	}
	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"TTILob.MarshalTo: start",
			"lobDefinition", m.lobPayloadDefinition,
		)
	}

	if err := m.header.MarshalTo(ctx, engine); err != nil {
		common.Odl.Error("TTILob.MarshalTo: header marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobsrc = address of sourceLobLocator
	sourceLocLen := 0
	sourceLocator := m.lobPayloadDefinition.sourceLocator
	sourceLocatorHasBytes := false
	if sourceLocator != nil {
		sourceLocatorHasBytes = sourceLocator.hasBytes()
	}
	if err := m._marshalPointer(ctx, engine, !sourceLocatorHasBytes, "source locator"); err != nil {
		return err
	}
	if sourceLocatorHasBytes {
		sourceLocLen = sourceLocator.length()
	}
	// lobslen is sb4
	if err := engine.MarshalSB4(ctx, common.SB4(sourceLocLen)); err != nil {
		common.Odl.Error("TTILob.MarshalTo: source locator length marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// address of destinationLobLocator
	destinationLocator := m.lobPayloadDefinition.destinationLocator
	destinationLocatorHasBytes := false
	if destinationLocator != nil {
		destinationLocatorHasBytes = destinationLocator.hasBytes()
	}
	if err := m._marshalPointer(ctx, engine, !destinationLocatorHasBytes, "destination locator"); err != nil {
		return err
	}
	if destinationLocatorHasBytes {
		m.lobPayloadDefinition.destinationLength = common.SB4(destinationLocator.length())
	}
	// lobdlen is sb4
	if err := engine.MarshalSB4(ctx, common.SB4(m.lobPayloadDefinition.destinationLength)); err != nil {
		common.Odl.Error("TTILob.MarshalTo: destination locator length marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobsoff = offset into source Lob
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Error("TTILob.MarshalTo: source offset marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobdoff = offset into destination Lob
	if err := engine.MarshalUB4(ctx, 0); err != nil {
		common.Odl.Error("TTILob.MarshalTo: destination offset marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobchr is address of Character Set id
	if err := m._marshalPointer(ctx, engine, m.lobPayloadDefinition.charsetID == 0, "charset"); err != nil {
		return err
	}

	// lobamt is address of ub4 containing # bytes
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Error("TTILob.MarshalTo: lob amount pointer marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobnull is address of ub1 containing null value
	// this is an output parameter and is never sent.
	// However, if expected the address must be non-null
	// to avoid a segment violation in kpolob.c. If
	// nullO2U = true then send non-null address.
	if err := m._marshalPointer(ctx, engine, !m.lobPayloadDefinition.nullO2U, "lob null"); err != nil {
		return err
	}

	// lobops = ub4 containing the OLOBOPS function code
	if err := engine.MarshalUB4(ctx, common.UB4(m.lobPayloadDefinition.operation)); err != nil {
		common.Odl.Error("TTILob.MarshalTo: lob operation marshal failed",
			"error", err,
			"operation", m.lobPayloadDefinition.operation,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobscn = address of ub4 array
	if err := m._marshalPointer(ctx, engine, m.lobPayloadDefinition.lobscnl == 0, "lobscn"); err != nil {
		return err
	}

	// lobscnl = size of lobscn ub4 array
	if err := engine.MarshalSB4(ctx, m.lobPayloadDefinition.lobscnl); err != nil {
		common.Odl.Error("TTILob.MarshalTo: lobscnl marshal failed",
			"error", err,
			"lobscnl", m.lobPayloadDefinition.lobscnl,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	sourceOffset := common.UB8(0)
	if src := m.lobPayloadDefinition.getSourceLocator(); src != nil {
		sourceOffset = src.getOffset()
	}

	if err := engine.MarshalUB8(ctx, sourceOffset); err != nil {
		common.Odl.Error("TTILob.MarshalTo: source UB8 offset marshal failed",
			"error", err,
			"sourceOffset", sourceOffset,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	destinationOffset := common.UB8(0)
	if dst := m.lobPayloadDefinition.getDestinationLocator(); dst != nil {
		destinationOffset = dst.getOffset()
	}

	if err := engine.MarshalUB8(ctx, destinationOffset); err != nil {
		common.Odl.Error("TTILob.MarshalTo: destination UB8 offset marshal failed",
			"error", err,
			"destinationOffset", destinationOffset,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	if err := m._marshalPointer(ctx, engine, !m.lobPayloadDefinition.sendLobAmt, "lob amount"); err != nil {
		return err
	}

	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Error("TTILob.MarshalTo: auxiliary null pointer marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	if err := engine.MarshalSB4(ctx, 0); err != nil {
		common.Odl.Error("TTILob.MarshalTo: auxiliary SB4 marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Error("TTILob.MarshalTo: auxiliary null pointer marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	if err := engine.MarshalSB4(ctx, 0); err != nil {
		common.Odl.Error("TTILob.MarshalTo: auxiliary SB4 marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	if err := engine.MarshalNullPTR(ctx); err != nil {
		common.Odl.Error("TTILob.MarshalTo: auxiliary null pointer marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	if err := engine.MarshalSB4(ctx, 0); err != nil {
		common.Odl.Error("TTILob.MarshalTo: auxiliary SB4 marshal failed",
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}

	// lobdef is built. Now send input parameters. The following input parameters need to be checked. If
	// non-null or non-0 then send the parameter. NOTE that parameter order must be identical to defin-
	// ition in opidef.h

	// see if lobsrc must be sent
	if sourceLocatorHasBytes {
		if err := engine.MarshalB1Array(ctx, m.lobPayloadDefinition.sourceLocator.locatorBytes); err != nil {
			common.Odl.Error("TTILob.MarshalTo: source locator data marshal failed",
				"error", err,
				"msgCode", m.GetMsgCode(),
			)
			return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
		}
	}

	// see if lobdst must be sent
	if destinationLocatorHasBytes {
		if err := engine.MarshalB1Array(ctx, m.lobPayloadDefinition.destinationLocator.locatorBytes); err != nil {
			common.Odl.Error("TTILob.MarshalTo: destination locator data marshal failed",
				"error", err,
				"msgCode", m.GetMsgCode(),
			)
			return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
		}
	}

	// see if lobchr must be sent
	if m.lobPayloadDefinition.charsetID != 0 {
		if err := engine.MarshalUB2(ctx, m.lobPayloadDefinition.charsetID); err != nil {
			common.Odl.Error("TTILob.MarshalTo: charset id marshal failed",
				"error", err,
				"charsetID", m.lobPayloadDefinition.charsetID,
				"msgCode", m.GetMsgCode(),
			)
			return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
		}
	}

	// NOTE: lobnull is strictly an output parameter and is this never sent
	// see if lobscn array needs to be sent
	if m.lobPayloadDefinition.lobscnl != 0 && len(m.lobPayloadDefinition.lobscn) > 0 {
		if len(m.lobPayloadDefinition.lobscn) < int(m.lobPayloadDefinition.lobscnl) {
			common.Odl.Error("TTILob.MarshalTo: lobscn length validation failed - lobscn length shorter than lobscnl",
				"lobscnLength", len(m.lobPayloadDefinition.lobscn),
				"lobscnl", m.lobPayloadDefinition.lobscnl,
				"msgCode", m.GetMsgCode(),
			)
			return common.NewOracleError(common.FailMarshal, nil, TTCMsgTypeDescription[m.GetMsgCode()])
		}
		for i := 0; i < int(m.lobPayloadDefinition.lobscnl); i++ {
			if err := engine.MarshalUB4(ctx, m.lobPayloadDefinition.lobscn[i]); err != nil {
				common.Odl.Error("TTILob.MarshalTo: lobscn entry marshal failed",
					"error", err,
					"index", i,
					"msgCode", m.GetMsgCode(),
				)
				return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
			}
		}
	}

	if m.lobPayloadDefinition.sendLobAmt {
		if err := engine.MarshalUB8(ctx, m.lobPayloadDefinition.lobAmt); err != nil {
			common.Odl.Error("TTILob.MarshalTo: lob amount marshal failed",
				"error", err,
				"lobAmt", m.lobPayloadDefinition.lobAmt,
				"msgCode", m.GetMsgCode(),
			)
			return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
		}
	}

	if common.Odl.Enabled(ctx, slog.LevelDebug) {
		common.Odl.Debug(
			"TTILob.MarshalTo: completed",
			"msgCode", m.GetMsgCode(),
			"lobDefinition", m.lobPayloadDefinition,
		)
	}

	m.definitionConfigured = false
	return nil
}

// _marshalPointer is a helper that conditionally marshals a pointer or null
// pointer based on the provided flag.
//
// Parameters:
//   - ctx (context.Context): provides cancellation and timeout control for the
//     marshalling workflow.
//   - engine (common.Marshaller): encoder that writes TTC-formatted bytes to
//     the outbound stream.
//   - isNullPointer (bool): if true, a null pointer is marshalled; otherwise a
//     non-null pointer is emitted.
//   - pointerType (string): descriptor used to build the log message.
//
// Returns:
//   - error: non-nil if encoding fails.
//
// Errors:
//   - OGD-00011 (FailMarshal) when the marshaller cannot encode the requested
//     pointer representation.
func (m *tTIlob) _marshalPointer(ctx context.Context, engine common.Marshaller, isNullPointer bool, pointerType string) error {
	var err error
	if isNullPointer {
		err = engine.MarshalNullPTR(ctx)
	} else {
		err = engine.MarshalPTR(ctx)
	}
	if err != nil {
		common.Odl.Error(
			"TTILob.MarshalTo: "+pointerType,
			"error", err,
			"msgCode", m.GetMsgCode(),
		)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[m.GetMsgCode()])
	}
	return nil
}
