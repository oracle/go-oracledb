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

// ttiOCSessrel server's "session-release values" message for a pooled-session release.
type ttiOCSessrel struct {
	header      driverCommon.Marshallable
	sessrlstag  string // not used as we use OCSessrel  as ONWAYFN
	sessrlsmode driverCommon.UB4
	tag         string
}

const (
	sessRlsRetag            = 0x00000004
	sessRlsMultiPropertyTag = 0x00000008
)

// newttiOCSessrel allocates a new receiver for OCSSESSREL payloads.
// The returned value implements common.Message and is intended to be populated
// via UnMarshalFrom by the MessageStreamer.
func newttiOCSessrel() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOCSessrel{
		header:      &ttiFunHeader{_funcType: ocsessrls},
		sessrlsmode: 0,
	}
}

// newttiOCSessrel18 allocates an OCSSESSREL message using the protocol 18
// function header, which includes the token field.
func newttiOCSessrel18() driverCommon.Message[driverCommon.MessageType] {
	return &ttiOCSessrel{
		header:      &ttiFunHeader18{ttiFunHeader: &ttiFunHeader{_funcType: ocsessrls}},
		sessrlsmode: 0,
	}
}

// GetMsgCode implements common.Message and identifies this message as a
// TTIONEWAYFN request.
func (spf *ttiOCSessrel) GetMsgCode() driverCommon.MessageType {
	return TTIONEWAYFN
}

// GetFuncCode returns the function code associated with this message.
func (spf *ttiOCSessrel) GetFuncCode() driverCommon.FunctionType {
	return ocsessrls
}

func (spf *ttiOCSessrel) SetTag(tag string) {
	spf.tag = tag
	if len(spf.tag) > 0 {
		spf.sessrlsmode |= sessRlsRetag
	} else {
		spf.sessrlsmode &^= sessRlsRetag
	}
}

func (spf *ttiOCSessrel) SetMultiPropertyTagMode(toggle bool) {
	if toggle {
		spf.sessrlsmode |= sessRlsMultiPropertyTag
	} else {
		spf.sessrlsmode &^= sessRlsMultiPropertyTag
	}
}

// MarshalTo writes a OCSSESSREL payload from the wire.
func (spf *ttiOCSessrel) MarshalTo(ctx context.Context, engine driverCommon.Marshaller) error {
	err := spf.header.MarshalTo(ctx, engine)
	if err != nil {
		common.Odl.Warn("Failed to marshall OCSSESSREL header", "error", err)
		return common.NewOracleError(oracleErrors.FailMarshal, err, nil)
	}
	tagLen := len(spf.tag)
	var B1Tag driverCommon.B1Array
	if len(spf.tag) > 0 {
		err := engine.MarshalPTR(ctx)
		if err != nil {
			common.Odl.Warn("Failed to marshall OCSSESSREL tag", "error", err)
			return err
		}
		B1Tag = driverCommon.StringToB1Array(spf.tag)
		err = engine.MarshalSB4(ctx, driverCommon.SB4(tagLen))
		if err != nil {
			return err
		}
	} else {
		engine.MarshalSB4(ctx, 0)
		engine.MarshalNullPTR(ctx)
	}
	engine.MarshalUB4(ctx, spf.sessrlsmode)
	if len(B1Tag) > 0 {
		err := engine.MarshalB1Array(ctx, B1Tag)
		//err := engine.MarshalCLR(ctx, B1Tag,0,len(B1Tag))
		if err != nil {
			return err
		}
	}

	return nil
}
