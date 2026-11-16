/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed to or provided by such licensors), and
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
**
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

// newCLRBindEncoder adapts an ordinary value encoder to TTC's standard CLR
// bind representation.
func newCLRBindEncoder(encode encoderFunc) *typeEncoder {
	return newTypeEncoder(encode, wrapCLRColumn)
}

// wrapCLRColumn writes payload as an ordinary TTC CLR bind. A nil payload is
// written as TTC's null length indicator.
func wrapCLRColumn(ctx context.Context, mar driverCommon.Marshaller, payload driverCommon.B1Array) error {
	if payload == nil {
		if err := mar.MarshalUB1(ctx, driverCommon.UB1(0)); err != nil {
			common.Odl.Error("wrapCLRColumn: failed to write null length indicator", "error", err, "stage", "null-indicator")
			return common.NewOracleError(oracleErrors.FailMarshal, err)
		}
		return nil
	}

	if err := mar.MarshalCLR(ctx, payload, 0, len(payload)); err != nil {
		common.Odl.Error("wrapCLRColumn: failed to write CLR", "error", err, "stage", "clr")
		return common.NewOracleError(oracleErrors.FailMarshal, err)
	}
	return nil
}
