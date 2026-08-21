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

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// tTIoer14 represents the Oracle error (OER) structure for version 14 and above.
// It embeds tTIoer and adds fields for SQL command type and checksum.
type tTIoer14 struct {
	*tTIoer
	sqlCommandType driverCommon.UB4 // SQL command type
	checksum       driverCommon.UB4
}

// newTTIoer14 creates a new instance of tTIoer.
func newTTIoer14() driverCommon.Message[driverCommon.MessageType] {
	return &tTIoer14{
		tTIoer:         newTTIoer().(*tTIoer),
		sqlCommandType: 0,
		checksum:       0,
	}
}

// newTTIoer14WithEndOfCallStatusSupport creates a new instance of tTIoer that supports end of call status.
func newTTIoer14WithEndOfCallStatusSupport() driverCommon.Message[driverCommon.MessageType] {
	return &tTIoer14{
		tTIoer:         newTTIoerWithEndOfCallStatusSupport().(*tTIoer),
		sqlCommandType: 0,
		checksum:       0,
	}
}

// Init resets the tTIoer14 and its embedded tTIoer fields to their zero values.
func (t *tTIoer14) init() {
	t.tTIoer.init()
	t.sqlCommandType = 0
	t.checksum = 0
}

// UnMarshalFrom reads and processes error attributes from the network buffer.
// It returns the current cursorId ID and an error if unmarshalling fails.
func (t *tTIoer14) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTIoer14.UnMarshalFrom: start")
	if err := t._unmarshalAttributes(ctx, mar); err != nil {
		common.Odl.Error("TTIoer14.UnMarshalFrom: unmarshalAttributes failed",
			"error", err,
			"retCode", t.retCode,
			"oerrcd2", t.oerrcd2,
			"sqlCommandType", t.sqlCommandType,
			"checksum", t.checksum,
		)
		return err
	}

	if t.oerrcd2 != 0 {
		common.Odl.Debug("TTIoer14.UnMarshalFrom: oerrcd2 != 0, unmarshalling error message")
		if err := t._unmarshalErrorMessage(ctx, mar); err != nil {
			common.Odl.Error("TTIoer14.UnMarshalFrom: unmarshalErrorMessage failed",
				"error", err,
				"retCode", t.retCode,
				"oerrcd2", t.oerrcd2,
				"sqlCommandType", t.sqlCommandType,
				"checksum", t.checksum,
			)
			return err
		}
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIoer14.UnMarshalFrom: end", "struct", fmt.Sprintf("%+v", t))
	}
	return nil
}

// _unmarshalAttributes reads error attributes from the network buffer and populates the struct fields.
func (t *tTIoer14) _unmarshalAttributes(ctx context.Context, mar driverCommon.Marshaller) error {
	common.Odl.Debug("TTIoer14.UnmarshalAttributes: start")
	var err error
	if err = t.tTIoer._unmarshalAttributes(ctx, mar); err != nil {
		common.Odl.Error("TTIoer14._unmarshalAttributes: base _unmarshalAttributes failed",
			"error", err,
			"retCode", t.retCode,
			"oerrcd2", t.oerrcd2,
		)
		return err
	}
	if t.sqlCommandType, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer14._unmarshalAttributes: sqlCommandType unmarshal failed",
			"error", err,
			"retCode", t.retCode,
			"oerrcd2", t.oerrcd2,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[t.GetMsgCode()])
	}

	if t.checksum, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Error("TTIoer14._unmarshalAttributes: checksum unmarshal failed",
			"error", err,
			"retCode", t.retCode,
			"oerrcd2", t.oerrcd2,
			"sqlCommandType", t.sqlCommandType,
		)
		return common.NewOracleError(oracleErrors.FailUnmarshal, err, TTCMsgTypeDescription[t.GetMsgCode()])
	}
	common.Odl.Debug("TTIoer14.UnmarshalAttributes: end")
	return nil
}

// UpdateChecksum updates the checksum with the tTIoer14 fields and returns the new value.
func (t *tTIoer14) _computeChecksum(localChecksum uint64) uint64 {
	localChecksum = t.tTIoer._computeChecksum(localChecksum)
	localChecksum = CRC64UpdateChecksum(localChecksum, uint64(t.sqlCommandType))
	localChecksum = CRC64UpdateChecksum(localChecksum, uint64(t.checksum))
	return localChecksum
}
