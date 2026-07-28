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
tTIuds20 holds column/type metadata in TTIuds17 and annotation key-value pairs for version 20 - 23.
NOTE: This is NOT a protocol message but a part of the DCB (Data Column Block) structure, used internally only.
*/
type tTIuds20 struct {
	*tTIuds17
	annotations map[string]string // annotations holds key-value metadata.
}

// newTTIuds20 returns a new TTIuds20 struct
func newTTIuds20() common.UnMarshallable {
	common.Odl.Debug("Instantiating TTIuds20 with new TTIuds17")
	return &tTIuds20{
		tTIuds17: newTTIuds17().(*tTIuds17),
	}
}

// UnMarshalFrom unmarshals column/type metadata and annotation key-value pairs from the network buffer
func (p *tTIuds20) UnMarshalFrom(ctx context.Context, mar common.Marshaller) error {
	var err error
	if err = p.tTIuds17.UnMarshalFrom(ctx, mar); err != nil {
		common.Odl.Warn("TTIuds17.UnMarshalFrom: failed to unmarshal", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}

	p.annotations = nil
	kvArrlen, err := mar.UnmarshalUB4(ctx)
	if err != nil {
		common.Odl.Warn("Failed to unmarshal annotation array length", "error", err)
		return common.NewOracleError(common.FailUnmarshal, err, "UDS")
	}
	if kvArrlen > 0 {
		if _, err = mar.UnmarshalUB1(ctx); err != nil {
			common.Odl.Warn("Failed to unmarshal annotation UB1", "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, "UDS")
		}

		// Number of pairs to follow
		numOfPairs, err := mar.UnmarshalUB4(ctx)
		if err != nil {
			common.Odl.Warn("Error unmarshalling UB4 for Server-To-Client Piggyback pair count", "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, "UDS")
		}

		// Additional flags (ignored)
		_, err = mar.UnmarshalUB1(ctx)
		if err != nil {
			common.Odl.Warn("Error unmarshalling UB1 for Server-To-Client Piggyback flags", "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, "UDS")
		}

		// Key/value list
		keyValueArr, err := newKeywordValueArray(numOfPairs)
		if err != nil {
			common.Odl.Warn("Server-To-Client Piggyback key/value pair count exceeds limit", "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, "keyword/value")
		}
		err = ((common.UnMarshallable)(keyValueArr)).UnMarshalFrom(ctx, mar)
		if err != nil {
			common.Odl.Warn("Unable to unmarshal Server-To-Client Piggyback key/value pairs", "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, "keyword/value")
		}

		// ignore Trailing flag
		_, err = mar.UnmarshalUB4(ctx)
		if err != nil {
			common.Odl.Warn("Error unmarshalling Server-To-Client Piggyback trailing flag", "error", err)
			return common.NewOracleError(common.FailUnmarshal, err, "UDS")
		}

		// fillAnnotations
		p.annotations = make(map[string]string)
		for _, kv := range *keyValueArr {
			key := common.B1ArrayToString(kv.textValue.value)
			value := common.B1ArrayToString(kv.binaryValue.value)
			p.annotations[key] = value
		}
	}
	if common.Odl.Enabled(common.BackgroundContext, slog.LevelDebug) {
		common.Odl.Debug("TTIuds20: fully unmarshalled", "struct", fmt.Sprintf("%+v", p))
	}
	return nil
}
