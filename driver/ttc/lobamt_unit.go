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

import "github.com/oracle/go-driver/driver/common"

// bytesPerUTF16CodeUnit is the number of bytes consumed by a single UTF-16 code unit.
const bytesPerUTF16CodeUnit = 2

// lobAmtUnit expresses the logical interpretation of the lobAmt field
// that accompanies write operations. The unit depends on the negotiated
// character set pairing between the client and the database.
type lobAmtUnit int

const (
	// lobAmtCodeUnit signals the amount should be counted in UTF-16 code units.
	// A code unit represents the number of bits consumed by a character set when
	// storing details about a code point. UTF-16 uses 16-bit code units, and
	// characters above 0x0000FFFF consume two 16-bit code units (a surrogate
	// pair) to represent a single code point.
	lobAmtCodeUnit lobAmtUnit = iota

	// lobAmtCodePoint signals the amount should be counted in Unicode code
	// points. A code point is the integer assigned to a character—'a' maps to 97
	// (0x61) while a grinning cat emoji maps to 128,568 (0x01F638). Different
	// encodings express the same code point using distinct byte sequences, such
	// as UTF-8 emitting {0xF0, 0x9F, 0x98, 0xB8} and UTF-16 emitting
	// {0xD83D, 0xDE38} for that emoji, yet both represent a single code point.
	lobAmtCodePoint

	// lobAmtUnknown instructs callers to suppress the amount when marshalling
	// the payload because the expected unit cannot be determined confidently.
	// The protocol lacks a formal specification for every character set pairing,
	// so the only way to discover the correct unit is by testing combinations and
	// observing which ones avoid errors such as ORA-03137. Behaviour can vary by
	// database character set; for example, matching UTF-8 pairs accept code points,
	// while a UTF-8 client paired with a TH8TISASCII database rejects them.
	lobAmtUnknown
)

// resolveLobAmtUnit determines the appropriate lobAmt unit for the negotiated
// driver character set. Only two well-understood character sets are recognised.
// The TTC wire protocol requires this mapping so the server interprets the
// amount field correctly and avoids ORA-03137. Unsupported character sets
// return an error because the Go driver does not yet implement the conversions
// required for other encodings.
func resolveLobAmtUnit(charSet common.UB2) (lobAmtUnit, error) {
	switch charSet {
	case al16Utf16CharSet:
		return lobAmtCodeUnit, nil
	case al32Utf8CharSet:
		return lobAmtCodePoint, nil
	default:
		return lobAmtUnknown, common.NewOracleError(
			common.UnsupportedCharacterSet,
			nil,
			charSet,
		)
	}
}

// lobAmtPolicy captures the negotiated character set pairing so callers can derive consistent lobAmt
// semantics when writing CLOB/NCLOB payloads. The driver character set currently reflects
// cliRIN/cliROUT (AL32UTF8 today), and ncharCS captures the server NCHAR character set.
// Note: The server database character set is not tracked because the driver presently supports only
// AL32UTF8 pairings.
type lobAmtPolicy struct {
	driverCS common.UB2
	ncharCS  common.UB2
}

// newLobAmtPolicy constructs a policy from the negotiated TTC character set identifiers.
func newLobAmtPolicy(driverCS, ncharCS common.UB2) lobAmtPolicy {
	return lobAmtPolicy{
		driverCS: driverCS,
		ncharCS:  ncharCS,
	}
}

// unitFor returns the lobAmt unit that should be advertised for the given LOB flavour.
func (p lobAmtPolicy) unitFor(isNCLOB bool) (lobAmtUnit, error) {
	if isNCLOB {
		return resolveLobAmtUnit(p.ncharCS)
	}
	return resolveLobAmtUnit(p.driverCS)
}

// Translate maps the supplied counts to a lobAmt value, returning false when the amount should be
// suppressed. Unsupported character sets trigger an error so callers can fail fast.
//
// Input parameters:
//
//	isNCLOB   – selects the NCLOB (true) or CLOB (false) pairing when resolving the expected unit.
//	codeUnits – caller-provided count of UTF-16 code units available for the write operation.
//	codePoints – caller-provided count of Unicode code points available for the write operation.
//
// Return values:
//
//	int  – the amount that should be marshalled into the TTC lobAmt field when the unit is known;
//	        zero when the amount must be suppressed or an error occurs.
//	bool – indicates whether the amount should be emitted (true) or suppressed (false).
//	error – non-nil when the negotiated character set pairing is unsupported. Callers should treat
//	        a non-nil error as a hard failure and avoid marshaling the amount.
func (p lobAmtPolicy) Translate(isNCLOB bool, codeUnits, codePoints int) (int, bool, error) {
	unit, err := p.unitFor(isNCLOB)
	if err != nil {
		return 0, false, err
	}

	switch unit {
	case lobAmtCodePoint:
		return codePoints, true, nil
	case lobAmtCodeUnit:
		return codeUnits, true, nil
	default:
		return 0, false, nil
	}
}
