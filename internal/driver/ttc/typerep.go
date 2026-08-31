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
	// RepUnv is the default representation for all types
	RepUnv byte = 1
	// RepBUnv is the universal UB1/bin representation.
	RepBUnv byte = 1
	// RepCUnv is the general character representation.
	RepCUnv byte = 1

	// RepIUnv is the universal integer representation: 1 byte sign+length followed by the digits.
	// Integers have the following names, where base 256 digits are numbered 8..1.
	// REPIrlh: | --- position of high order digit, 1 == first in memory
	//          | ---- position of low order digit
	//          | ----- representation: U == unsigned, T == 2's comp signed, O == 1's complement signed
	RepIUnv byte = 1

	// RepAUnv is the universal pointer address representation (0 == null, !0 == not null).
	RepAUnv byte = 1

	// RepNV51 is the Oracle number representation for version 5.1 numbers.
	RepNV51 byte = 10
	// RepDV51 is the Oracle date representation for version 5.1 dates.
	RepDV51 byte = 10

	// RepRUnv is the record representation where fields are byte packed.
	// TTCBUR always returns RepRUnv. TTCCLR notices if a record can be sent directly, and returns RepRUnv when this case occurs.
	RepRUnv byte = 1

	// Native is used for native data type representation.
	// Bit 1: 0 -- Native, 1 -- Universal
	// Bit 2: 0 -- MSB, 1 -- LSB
	// 00 (0) -> Native + MSB
	// 01 (1) -> UNIVERSAL + MSB
	// 10 (2) -> Native + LSB (not applicable, but maybe used)
	// 11 (3) -> UNIVERSAL + LSB
	Native byte = 0x00

	// Universal is used for universal data type representation (byte with length followed by data).
	Universal byte = 0x01

	// Lsb is used for Lsb data type representation (normally not used, but present for completeness).
	Lsb byte = 0x02

	// MaxRep is the max supported type representation (max possible conversion is 0x01 + 0x02 = 0x03).
	// If this is exceeded, an exception will be raised.
	MaxRep byte = 0x03

	// MaxType is the maximum supported type value.
	MaxType byte = 4

	// NumReps is the number of supported representations.
	NumReps = byte(MaxType + 1)

	// B1 index in basic type array
	B1 byte = 0
	// B2 index in basic type array
	B2 byte = 1
	// B4 index in basic type array
	B4 byte = 2
	// B8 index in basic type array
	B8 byte = 3
	// PTR index in basic type array
	PTR byte = 4

	// MaxReps has no specific meaning here. It's only used as a hint for the size of typeAndRep
	// so that we know that we have enough room and don't constantly reallocate a larger one.
	MaxReps int16 = 665

	// _maxReceivedReps is the cap for the number of type representations that will be unmarshalled,
	// if the number of type representations received exceeds this number a protocol violation error
	// will be returned.
	_maxReceivedReps int16 = 1024
)

// representationTable manages type representations and conversion flags for Oracle data types.
// It provides methods to set and get type representations, conversion flags, and server conversion state.
type representationTable struct {
	representations  []int16
	conversionFlags  byte
	serverConversion bool
	// keep track of native types being wether UNIVERSAL
	// or NATIVE
	nativeTypesRepresentation [5]byte
}

// this is filled in in package Init()
var typeRepresentationTable *representationTable = newTypeRep()

// newTypeRep makes a new blank TypeRep table
func newTypeRep() *representationTable {
	var t = &representationTable{
		conversionFlags:  0,
		serverConversion: false,
		representations:  make([]int16, MaxReps*4+1),
	}

	t.nativeTypesRepresentation[B1] = Native
	t.nativeTypesRepresentation[B2] = Universal
	t.nativeTypesRepresentation[B4] = Universal
	t.nativeTypesRepresentation[B8] = Universal
	t.nativeTypesRepresentation[PTR] = Universal

	// offset is the first bytes of this array
	// _oSessionKeyInit it to 1
	t.representations[0] = 1
	return t
}

func (t *representationTable) isNativeTypeAsUniversal(typ byte) bool {
	return t.nativeTypesRepresentation[typ] == Universal
}

// MarshalTo marshalTypeReps marshals the type representations based on capabilities.
func (t *representationTable) MarshalTo(ctx context.Context, mar driverCommon.Marshaller) error {
	// send dty and rep as UB2
	for i := 1; i < int(t.representations[0]); i++ {
		if err := mar.MarshalUB2(ctx, driverCommon.UB2(t.representations[i])); err != nil { // LSB is false
			return err
		}
	}
	if err := mar.MarshalUB2(ctx, 0); err != nil { // to mark the end
		return err
	}
	return nil
}

// UnMarshalFrom unmarshal the type representations.
func (t *representationTable) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	var b driverCommon.UB2
	var err error

	// It loops through the received data until two consecutive zeros are found,
	// indicating the end of the structure. No direct matching validation is performed
	// between sent and received types (commented out for compatibility with v80).
	//
	// The structure consists of type blocks starting with a non-zero byte, followed by
	// zero or more pairs of bytes. The first byte of each pair is non-zero, but the
	// second may be zero. A zero where a pair start is expected ends the block, and
	// a zero where a type block start is expected ends the entire structure.
	//
	// Examples:
	//
	//	NN 00
	//	NN WW ww 00
	//	NN XX xx YY yy 00
	//	00
	//

	// inTypeBlock is true when currently parsing UB2 pairs within a type block.
	// It is set to false when expecting the start of a new type block or the terminal zero.
	inTypeBlock := false

	// pairCount tracks the position within a pair in the block:
	// 0 when expecting the leading UB2 (type code) of a pair,
	// 1 when expecting the trailing UB2 (representation) of a pair.
	pairCount := 0

	var nbDty int16 = 0
	for {
		// Reading the next UB2
		b, err = mar.UnmarshalUB2(ctx)
		if err != nil {
			common.Odl.Warn("Failed to unmarshal typerep", "error", err)
			return common.NewOracleError(oracleErrors.FailUnmarshal, err, "TypeRep")
		}

		if !inTypeBlock {
			nbDty++
			// The type of the next block was read, or the terminal 0 was read
			if b == 0 {
				return nil
			}

			if nbDty > _maxReceivedReps {
				common.Odl.Warn("Failed to unmarshal typerep, number of type representations exceeded the maximum allowed")
				return common.NewOracleError(oracleErrors.ProtocolViolationLimitExceeded, nil, "nbDty", _maxReceivedReps, nbDty)
			}

			inTypeBlock = true
			pairCount = 0
		} else {
			switch pairCount {
			case 0:
				if b == 0 {
					inTypeBlock = false
				} else {
					pairCount = 1
				}
			case 1:
				pairCount = 0
			}
		}
	}
}

// addTypeRepToTable registers a data type with its type code, network type, and representation in the typeAndRep map.
func (t *representationTable) addTypeRepToTable(dty, ndty, rep int16) {
	if len(t.representations) < int(t.representations[0])+4 {
		typeAndRep2 := make([]int16, len(t.representations)*2)
		copy(typeAndRep2[0:], t.representations[0:t.representations[0]+1])
		t.representations = typeAndRep2
	}

	offset := t.representations[0]
	t.representations[offset] = dty
	t.representations[offset+1] = ndty

	if ndty == common.Dty0 {
		t.representations[0] = int16(offset + 2)
	} else {
		t.representations[offset+2] = rep
		t.representations[offset+3] = 0
		t.representations[0] = int16(offset + 4)
	}
}

// setRep sets the type representation for the given type.
// Returns an error if the type or representation is invalid.
func (t *representationTable) setRep(typ byte, rep byte) {
	t.nativeTypesRepresentation[typ] = rep
}

// getRep returns the representation for the given type.
// Returns an error if the type is invalid.
func (t *representationTable) getRep(typ byte) byte {
	return t.nativeTypesRepresentation[typ]
}

// SetFlags sets the conversion flags for the TypeRepresentationTable.
func (t *representationTable) SetFlags(flags byte) {
	t.conversionFlags = flags
}

// getFlags returns the current conversion flags value.
func (t *representationTable) getFlags() byte {
	return t.conversionFlags
}
