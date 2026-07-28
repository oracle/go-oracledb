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

	"github.com/oracle/go-driver/driver/common"
)

type ttiSTA struct {
	_supportsEndOfCallStatus bool
	// eocStatus end of call status
	eocStatus *endOfCallStatus
	// endToEndECIDSequenceNumber end to end ECID sequence number
	endToEndECIDSequenceNumber common.UB2
}

func (t ttiSTA) String() string {
	return fmt.Sprintf("ttiSTA {_supportsEndOfCallStatus: [%v], eocStatus [%v], endToEndECIDSequenceNumber: [%v]}",
		t._supportsEndOfCallStatus,
		t.eocStatus,
		t.endToEndECIDSequenceNumber)
}

// newTTISTA creates a new instance of ttiSTA.
func newTTISTA() common.Message[common.MessageType] {
	return &ttiSTA{
		_supportsEndOfCallStatus: false,
	}
}

// newTTISTAWithEndOfCallStatusSupport creates a new instance of ttiSTA that supports end of call status.
func newTTISTAWithEndOfCallStatusSupport() common.Message[common.MessageType] {
	return &ttiSTA{
		_supportsEndOfCallStatus: true,
	}
}

// GetMsgCode returns the message code
func (sta *ttiSTA) GetMsgCode() common.MessageType {
	return TTISTA
}

// UnMarshalFrom unmarshals the STA message
func (sta *ttiSTA) UnMarshalFrom(ctx context.Context, engine common.Marshaller) error {
	var err error
	if sta._supportsEndOfCallStatus {
		sta.eocStatus, err = unmarshalEndOfCallStatus(ctx, engine)
		if err != nil {
			return err
		}
	}
	sta.endToEndECIDSequenceNumber, err = engine.UnmarshalUB2(ctx)
	if err != nil {
		return err
	}

	return nil
}

// RegisterSTAWithCapability register STA messages that support end of call status on
// message registry. Replaces the existing messages
func RegisterSTAWithCapability() {
	err := MessageRegistry.Register(TTISTA, 1, newTTISTAWithEndOfCallStatusSupport)
	if err != nil {
		common.Odl.Debug("Failed to register STA function", "error", err)

	}

}

// isBeingDrainned returns true if the connection should be dropped
// due to a planned-down, otherwise false
func (sta *ttiSTA) isBeingDrainned() bool {
	return sta._supportsEndOfCallStatus && sta.eocStatus != nil && sta.eocStatus.connectionShouldBeDropped
}
