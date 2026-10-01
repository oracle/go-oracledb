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
** is contributed to or provided by such licensors),
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

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// tTIimplresOer decodes the base OER layout that terminates one prefetched
// implicit result set. Unlike a top-level TTIOER, TTIIMPLOER has neither EOCS
// nor the end-to-end ECID field.
type tTIimplresOer struct {
	*tTIoer
}

// newTTIimplresOer creates the base-layout OER decoder used by TTIIMPLOER.
func newTTIimplresOer() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplresOer{tTIoer: newTTIoer().(*tTIoer)}
}

// GetMsgCode identifies this decoder as an implicit-result OER terminator.
func (o *tTIimplresOer) GetMsgCode() driverCommon.MessageType { return TTIIMPLOER }

// UnMarshalFrom decodes the base implicit-result OER layout. ORA-01403 is
// retained as an OER value for the caller to treat as successful end-of-data.
func (o *tTIimplresOer) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	if err := o._unmarshalAttributeBody(ctx, mar); err != nil {
		return err
	}
	return o.unmarshalErrorMessage(ctx, mar)
}

// unmarshalErrorMessage consumes the CLR error text only when the terminal
// OER carries a primary or extended Oracle error code.
func (o *tTIimplresOer) unmarshalErrorMessage(ctx context.Context, mar driverCommon.Marshaller) error {
	if o.retCode != 0 || o.oerrcd2 != 0 {
		return o._unmarshalErrorMessage(ctx, mar)
	}
	return nil
}

// tTIimplresOer14 decodes the versioned suffix sent after the shared
// implicit-result OER fields by TTC 14 and later servers.
type tTIimplresOer14 struct {
	*tTIimplresOer
	sqlCommandType driverCommon.UB4
	checksum       driverCommon.UB4
}

// newTTIimplresOer14 creates the versioned implicit-result OER decoder.
func newTTIimplresOer14() driverCommon.Message[driverCommon.MessageType] {
	return &tTIimplresOer14{
		tTIimplresOer: newTTIimplresOer().(*tTIimplresOer),
	}
}

// UnMarshalFrom decodes a TTC 14+ implicit-result OER, including the trailing
// SQL command type and checksum that precede a CLR error message.
func (o *tTIimplresOer14) UnMarshalFrom(ctx context.Context, mar driverCommon.Marshaller) error {
	if err := o._unmarshalAttributeBody(ctx, mar); err != nil {
		return err
	}
	var err error
	if o.sqlCommandType, err = mar.UnmarshalUB4(ctx); err != nil {
		return err
	}
	if o.checksum, err = mar.UnmarshalUB4(ctx); err != nil {
		return err
	}
	return o.unmarshalErrorMessage(ctx, mar)
}
