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

// ttiLobRpa represents the TTC LOB RPA message used to exchange locator metadata between client and server.
type ttiLobRpa struct {
	lobDefinition *lobDefinition // lobDefinition captures the LOB definition populated while decoding the message.
}

// GetMsgCode returns the TTC message code for the LOB RPA payload.
//
// Returns:
//   - common.MessageType: TTC message identifier associated with LOB RPA payloads.
func (p *ttiLobRpa) GetMsgCode() driverCommon.MessageType {
	return TTIRPA
}

// newTTILobRPA constructs a new LOB RPA message instance.
//
// Returns:
//   - common.Message[common.MessageType]: a zero-initialised LOB RPA message ready for use.
func newTTILobRPA() driverCommon.Message[driverCommon.MessageType] {
	return &ttiLobRpa{}
}

// UnMarshalFrom decodes the LOB RPA core payload.
// It reads:
//   - UB2 al8o4l (length), then al8o4l times UB4 values into al8o4
//   - Builds SCN from al8o4[0] (low) and al8o4[1]&^KSCNFVB (high)
//   - Sets cursorId from al8o4[2]
//   - Reads transaction context length (UB2) and bytes (ignored)
//   - Reads keyword-values length (UB2); decoding of pairs is deferred
//
// Parameters:
//   - ctx: execution context controlling cancellation and deadlines.
//   - mar: marshaller providing access to TTC encoded payload data.
//
// Returns:
//   - error: FailUnmarshal oracleError when unmarshalling fails; nil on success or when the definition is nil.
func (p *ttiLobRpa) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTILobRpa.UnmarshalFrom: begin", "operation", p.lobDefinition.operation)

	var err error
	// (1) retrieve source Lob Locator if necessary
	sourceLocator := p.lobDefinition.sourceLocator
	if sourceLocator != nil && sourceLocator.hasBytes() {
		sourceLocatorBytes := sourceLocator.locatorBytes
		if p.lobDefinition.operation == kplobTmpCreate && !p.lobDefinition.fixedTemporaryLocator {
			common.Odl.Debug("TTILobRpa.UnmarshalFrom: reading temporary locator header")
			header, err := mar.UnmarshalB1Array(ctx, 2)
			if err != nil {
				common.Odl.Error("TTILobRpa.UnmarshalFrom: temporary locator header unmarshal failed",
					"error", err,
					"operation", p.lobDefinition.operation,
				)
				return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
			}

			locatorLen := int(header[0])<<8 | int(header[1])
			common.Odl.Debug("TTILobRpa.UnmarshalFrom: temporary locator header processed",
				"raw_len", locatorLen,
				"allocated_len", len(sourceLocatorBytes))
			expectedLen := locatorLen + 2
			extraUnused := 0

			if len(sourceLocatorBytes) != expectedLen {
				if len(sourceLocatorBytes) > expectedLen {
					extraUnused = len(sourceLocatorBytes) - expectedLen
				}
				sourceLocatorBytes = make(driverCommon.B1Array, expectedLen)
			}

			sourceLocatorBytes[0] = header[0]
			sourceLocatorBytes[1] = header[1]

			if locatorLen > 0 {
				body, err := mar.UnmarshalB1Array(ctx, locatorLen)
				if err != nil {
					common.Odl.Error("TTILobRpa.UnmarshalFrom: temporary locator body unmarshal failed",
						"error", err,
						"operation", p.lobDefinition.operation,
					)
					return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
				}
				copy(sourceLocatorBytes[2:], body)
				common.Odl.Debug("TTILobRpa.UnmarshalFrom: temporary locator body populated",
					"locator_length", len(sourceLocatorBytes),
					"source_locator", append([]byte(nil), []byte(sourceLocatorBytes)...))
			}

			if extraUnused > 0 {
				eu, err := mar.UnmarshalB1Array(ctx, extraUnused)
				if err != nil {
					common.Odl.Error("TTILobRpa.UnmarshalFrom: discard unused locator bytes failed",
						"error", err,
						"operation", p.lobDefinition.operation,
					)
					return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
				}
				common.Odl.Debug("TTILobRpa.UnmarshalFrom: discarded unused locator bytes",
					"discarded", extraUnused,
					"bytes", eu)
			}

			sourceLocator.locatorBytes = sourceLocatorBytes
		} else {
			length := len(sourceLocatorBytes)
			if length > 0 {
				bytes, err := mar.UnmarshalB1Array(ctx, length)
				if err != nil {
					common.Odl.Error("TTILobRpa.UnmarshalFrom: locator bytes unmarshal failed",
						"error", err,
						"operation", p.lobDefinition.operation,
					)
					return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
				}
				copy(sourceLocatorBytes, bytes)
				common.Odl.Debug("TTILobRpa.UnmarshalFrom: locator bytes populated",
					"locator_length", len(sourceLocatorBytes),
					"source_locator", append([]byte(nil), []byte(sourceLocatorBytes)...))
			}
			sourceLocator.locatorBytes = sourceLocatorBytes
		}
	}

	// (2) retrieve destination Lob Locator
	destinationLocator := p.lobDefinition.destinationLocator
	if destinationLocator != nil && destinationLocator.hasBytes() {
		length, err := mar.UnmarshalUB2(ctx)
		if err != nil {
			common.Odl.Error("TTILobRpa.UnmarshalFrom: destination locator length unmarshal failed",
				"error", err,
			)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		destinationLocator.locatorBytes, err = mar.UnmarshalB1Array(ctx, int(length))
		if err != nil {
			common.Odl.Error("TTILobRpa.UnmarshalFrom: destination locator bytes unmarshal failed",
				"error", err,
			)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		common.Odl.Debug("TTILobRpa.UnmarshalFrom: destination locator populated",
			"locator_length", len(destinationLocator.locatorBytes),
			"destination_locator", append([]byte(nil), []byte(destinationLocator.locatorBytes)...))
	}

	// (3) retrieve LOB character set
	if p.lobDefinition.charsetID != 0 {
		p.lobDefinition.charsetID, err = mar.UnmarshalUB2(ctx)
		if err != nil {
			common.Odl.Error("TTILobRpa.UnmarshalFrom: charsetID unmarshal failed",
				"error", err,
			)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		common.Odl.Debug("TTILobRpa.UnmarshalFrom: charset ID unmarshalled", "charset_id", p.lobDefinition.charsetID)
	}

	// (4) retrieve lobamt
	if p.lobDefinition.sendLobAmt {
		p.lobDefinition.lobAmt, err = mar.UnmarshalUB8(ctx)
		if err != nil {
			common.Odl.Error("TTILobRpa.UnmarshalFrom: lobAmt unmarshal failed",
				"error", err,
			)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}
		common.Odl.Debug("TTILobRpa.UnmarshalFrom: lob amount unmarshalled", "lob_amount", p.lobDefinition.lobAmt)
	}

	// (5) retrieve NULL value -- only retrieve if null value is expected
	if p.lobDefinition.nullO2U {
		// NOTE: lobnull is defined as a UB1 but it is sent by the
		// server as an SB2. This is a bug in the server TTC code but
		// cannot change this now -- so always process lobnull as
		// output SB2.
		isNull, err := mar.UnmarshalSB4(ctx)
		if err != nil {
			common.Odl.Error("TTILobRpa.UnmarshalFrom: lobNull unmarshal failed",
				"error", err,
			)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[p.GetMsgCode()])
		}

		p.lobDefinition.lobNull = isNull != 0
		common.Odl.Debug("TTILobRpa.UnmarshalFrom: lob null flag unmarshalled", "is_null", p.lobDefinition.lobNull)
	}

	common.Odl.Debug("TTILobRpa.UnmarshalFrom: completed", "operation", p.lobDefinition.operation)
	return nil
}

// SetDefinition attaches the target lobDefinition to populate during unmarshalling.
//
// Parameters:
//   - lobDef: pointer to the lobDefinition structure that will be populated when unmarshalling occurs.
func (p *ttiLobRpa) SetDefinition(lobDef *lobDefinition) error {
	if lobDef == nil {
		common.Odl.Error("TTILobRpa.SetDefinition: nil lobDefinition provided")
		return common.NewOracleError(oracleErrors.InvalidLOBBuffer, nil, "marshal", TTCMsgTypeDescription[p.GetMsgCode()], "definition missing")
	}
	p.lobDefinition = lobDef
	return nil
}
