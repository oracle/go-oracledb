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

// provider for the uds fields.
type udsProvider interface {
	getOac() *tTIoac
	nullable() bool
	getColumnName() common.B1Array
	getSchemaName() common.B1Array
	getTypeName() common.B1Array
	getKernelPosition() common.UB2
	getColumnFlags() common.UB4
}

// compile-time assertion
var _ udsProvider = (*tTIuds)(nil)

/*
tTIuds holds base column/type metadata returned by the backend protocol for version 0 - 17.
NOTE: This is NOT a protocol message but a part of the DCB (Data Column Block) structure as used internally by the driver and mappings.
It includes column name, schema, type name, kernel position, and flags.
*/
type tTIuds struct {
	// oac holds the oacdef struct for column info.
	oac *tTIoac
	// isNullable is true if a NULL value is possible.
	isNullable bool
	// columnNameLen is the length of the column name for this type.
	columnNameLen common.UB1
	// columnName stores the column name.
	columnName common.B1Array
	// schemaName stores the schema name.
	schemaName common.B1Array
	// typeName stores the type name.
	typeName common.B1Array
	// kernelPosition is the column kernel position.
	kernelPosition common.UB2
	// columnFlags is the bitmasked flag
	columnFlags common.UB4
}

// getColumnName returns the column name B1Array.
func (p *tTIuds) getColumnName() common.B1Array {
	return p.columnName
}

// getOac returns the column OAC definition.
func (p *tTIuds) getOac() *tTIoac {
	return p.oac
}

// nullable returns true if the column accepts NULLs.
func (p *tTIuds) nullable() bool {
	return p.isNullable
}

// getSchemaName returns the schema name B1Array.
func (p *tTIuds) getSchemaName() common.B1Array {
	return p.schemaName
}

// getTypeName returns the type name B1Array.
func (p *tTIuds) getTypeName() common.B1Array {
	return p.typeName
}

// getKernelPosition returns the kernel position for this column.
func (p *tTIuds) getKernelPosition() common.UB2 {
	return p.kernelPosition
}

// getColumnFlags returns the column flags.
func (p *tTIuds) getColumnFlags() common.UB4 {
	return p.columnFlags
}

// newTTIuds initializes a TTIuds struct with a new network buffer and an oac object for column metadata.
func newTTIuds() common.UnMarshallable {
	return &tTIuds{
		oac: &tTIoac{},
	}
}

// UnMarshalFrom extracts column/type metadata fields from the network buffer.
func (p *tTIuds) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	common.Odl.Debug("TTIuds: UnMarshalFrom start")
	var nullAllowed common.UB1
	var err error

	if err = p.oac.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("TTIuds.UnMarshalFrom: failed to unmarshal oac", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	if nullAllowed, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("TTIuds.UnMarshalFrom: failed to unmarshal null flag", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	p.isNullable = nullAllowed != 0

	if p.columnNameLen, err = mar.UnmarshalUB1(ctx); err != nil {
		common.Odl.Warn("TTIuds.UnMarshalFrom: failed to unmarshal columnNameLen", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	var colName dynamicAllocatedArray
	if err = colName.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal columnName", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	p.columnName = colName.value
	p.columnNameLen = common.UB1((len(p.columnName)))

	var schName dynamicAllocatedArray
	if err = schName.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal schemaName", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	p.schemaName = schName.value

	var typName dynamicAllocatedArray
	if err = typName.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("UnMarshalFrom: failed to unmarshal typeName", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	p.typeName = typName.value

	// position of the column in the row
	if p.kernelPosition, err = mar.UnmarshalUB2(ctx); err != nil {
		common.Odl.Warn("TTIuds.UnMarshalFrom: failed to unmarshal kernelPosition", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	if p.columnFlags, err = mar.UnmarshalUB4(ctx); err != nil {
		common.Odl.Warn("TTIuds.UnMarshalFrom: failed to unmarshal columnFlags", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIuds: fully unmarshalled", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}
