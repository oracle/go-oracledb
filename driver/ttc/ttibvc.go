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

// tTIbvc encapsulates Bit Vector Column (BVC) logic for presence-tracking of columns
// in network protocol operations; used to optimize fetch of sparse data.
type tTIbvc struct {
	// bvcColSent is the BitSet indicating which columns are present in this row according to the BVC protocol.
	bvcColSent *common.BitSet
	// numberOfColumns is the total number of columns this structure expects to track.
	numberOfColumns common.UB4
	// bvcFound is true if a BVC bit vector has been provided/parsed for this row.
	bvcFound bool
}

// newTTIbvc instantiates a TTIbvc struct configured to decode plain BVC messages from Oracle's TTC protocol.
func newTTIbvc() common.Message[common.MessageType] {
	return &tTIbvc{}
}

// GetMsgCode implements Message.GetMsgCode, returning the TTC BVC message code.
func (bvc *tTIbvc) GetMsgCode() common.MessageType {
	return TTIBVC
}

// SetNumberOfColumns initializes TTIbvc for the given number of columns,
// allocating/clearing the bit vector if necessary and marking bvcFound false.
func (bvc *tTIbvc) SetNumberOfColumns(noOfCols common.UB4) {
	bvc.numberOfColumns = noOfCols
	bvc.bvcColSent = common.NewBitSet(int(bvc.numberOfColumns))
	bvc.bvcFound = false
}

// UnMarshalFrom reads a bit vector column (BVC) from the marshaller for fast fetch optimization.
// It reads enough bytes to account for all columns. After this call, bvc.bvcColSent is set,
// and each column's presence can be checked with bvc.bvcColSent.Get(col).
func (bvc *tTIbvc) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	// Read the number of columns that will be described by the incoming BVC.
	// Save the value so we can perform a sanity check after bitvector initialization.
	numColsSent, err := mar.UnmarshalUB2(ctx)
	if err != nil {
		common.Odl.Warn("TTIbvc.UnMarshalFrom: failed to read BVC column count", "error", err)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[bvc.GetMsgCode()])
	}

	// Compute number of bytes required to store bitflags for all columns (1 bit per column)
	nbOfUB1 := bvc.numberOfColumns / 8
	if bvc.numberOfColumns%8 != 0 {
		nbOfUB1++
	}

	// Efficiently read the required BVC bytes in one operation, then assign using SetBytes.
	bvcBuf, err := mar.UnmarshalB1Array(ctx, int(nbOfUB1))
	if err != nil {
		common.Odl.Warn("TTIbvc.UnMarshalFrom: failed to read BVC byte array", "error", err)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[bvc.GetMsgCode()])
	}
	bvc.bvcColSent.SetBytes(0, bvcBuf)

	// Mark that a BVC bit vector has been found and applied
	bvc.bvcFound = true

	// Sanity check: count set columns in bvcColSent and compare with numCols (as received in this message)
	setCols := bvc.bvcColSent.Cardinality()
	if setCols != int(numColsSent) {
		common.Odl.Warn("TTIbvc.UnMarshalFrom: column count sanity check failed",
			"presence-vector has columns", setCols, "message specified columns", numColsSent)
		return common.NewOracleError(common.FailMarshal, err, TTCMsgTypeDescription[bvc.GetMsgCode()])
	}
	return nil
}

// SetBitVector copies a provided bit vector into this TTIbvc, marking which columns are present.
// Used, for example, with RXH messages that include a ready-made vector from the server.
func (bvc *tTIbvc) SetBitVector(bitVec []byte, length int) {
	// Reset the bitmap
	bvc.bvcColSent.ClearAll()

	if length == 0 {
		bvc.bvcFound = false
	} else {
		bvc.bvcColSent.SetBytes(0, bitVec[:length])
		bvc.bvcFound = true
	}
}
