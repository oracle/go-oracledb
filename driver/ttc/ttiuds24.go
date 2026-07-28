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
	"fmt"
	"log/slog"

	"github.com/oracle/go-driver/driver/common"
)

/*
tTIuds24 holds column/type metadata (TTIuds20) and vector properties for version 24 onwards.
NOTE: This is NOT a protocol message but a part of the DCB (Data Column Block) structure, used internally only.
*/
type tTIuds24 struct {
	*tTIuds20
	vectorDim  common.UB4 // vectorDim is the vector dimension (for VECTOR type columns).
	vectorType common.UB1 // vectorType is the type of data in the vector.
	vectorFlag common.UB1 // vectorFlag is the vector flag.
}

// newTTIuds24 initializes a TTIuds24 struct
func newTTIuds24() common.UnMarshallable {
	return &tTIuds24{
		tTIuds20: newTTIuds20().(*tTIuds20),
	}
}

// UnMarshalFrom extracts column/type metadata from the network buffer into the TTIuds struct.
// It populates vector properties.
func (p *tTIuds24) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	common.Odl.Debug("TTIuds24: UnMarshalFrom start")
	var err error
	if err = p.tTIuds20.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("TTIuds20.UnMarshalFrom: failed to unmarshal", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	if p.vectorDim, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Warn("TTIuds24.UnMarshalFrom: failed to unmarshal vectorDim", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	if p.vectorType, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("TTIuds24.UnMarshalFrom: failed to unmarshal vectorType", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	if p.vectorFlag, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("TTIuds24.UnMarshalFrom: failed to unmarshal vectorFlag", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIuds24: fully unmarshalled", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}
