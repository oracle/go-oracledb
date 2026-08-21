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
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// TestTTIlob_New verifies that newTTIlob configures the TTIFUN header correctly.
func TestTTIlob_New(t *testing.T) {
	t.Parallel()
	msg := newTTIlob().(*tTIlob)
	if msg == nil {
		t.Fatal("expected non-nil tTIlob instance")
	}
	if _, ok := msg.header.(*ttiFunHeader18); !ok {
		t.Fatalf("expected header to be *ttiFunHeader18, got %T", msg.header)
	}
	if msg.lobPayloadDefinition != nil {
		t.Fatalf("expected definition to be nil by default")
	}
}

// TestTTIlob_SetDefinition confirms the definition setter assigns the provided struct.
func TestTTIlob_SetDefinition(t *testing.T) {
	t.Parallel()
	msg := newTTIlob().(*tTIlob)
	def := &lobDefinition{operation: kplobRead}
	msg.SetDefinition(def)
	if msg.lobPayloadDefinition != def {
		t.Fatalf("expected definition pointer to match the value passed to SetDefinition")
	}
}

// TestTTIlob_SetDefinition_NilDefinition ensures an error is returned when the
// definition argument is nil.
func TestTTIlob_SetDefinition_NilDefinition(t *testing.T) {
	t.Parallel()
	msg := newTTIlob().(*tTIlob)

	err := msg.SetDefinition(nil)
	if err == nil {
		t.Fatalf("expected error on nil definition, got nil")
	}

	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected common.SQLError, got %T", err)
	}

	if got, want := sqlErr.ErrorCode(), string(oracleErrors.InvalidLOBBuffer); got != want {
		t.Fatalf("unexpected error code: got %s want %s", got, want)
	}
	if !strings.Contains(err.Error(), "definition missing") {
		t.Fatalf("expected error message to mention missing definition, got %q", err.Error())
	}
}

// TestTTIlob_GetMsgCode ensures TTIFUN is returned for the message code.
func TestTTIlob_GetMsgCode(t *testing.T) {
	t.Parallel()
	msg := newTTIlob().(*tTIlob)
	if got := msg.GetMsgCode(); got != TTIFUN {
		t.Fatalf("GetMsgCode mismatch: want %v, got %v", TTIFUN, got)
	}
}

// TestTTIlob_GetFuncCode ensures oLobOps opcode is emitted for the function code.
func TestTTIlob_GetFuncCode(t *testing.T) {
	t.Parallel()
	msg := newTTIlob().(*tTIlob)
	if got := msg.GetFuncCode(); got != oLobOps {
		t.Fatalf("GetFuncCode mismatch: want %v, got %v", oLobOps, got)
	}
}

// TestTTIlob_MarshalTo_Success exercises successful marshalling paths for OLOBOPS requests.
func TestTTIlob_MarshalTo_Success(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		def  func() *lobDefinition
	}{
		{
			name: "read-minimal",
			def: func() *lobDefinition {
				return &lobDefinition{
					operation: kplobRead,
				}
			},
		},
		{
			name: "write-with-extended-fields",
			def: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator:      newLocator(driverCommon.B1Array{0x01, 0x02, 0x03, 0x04}, driverCommon.UB8(4)),
					destinationLocator: newLocator(driverCommon.B1Array{0x05, 0x06, 0x07}, driverCommon.UB8(8)),
					charsetID:          driverCommon.UB2(873),
					nullO2U:            true,
					sendLobAmt:         true,
					lobAmt:             driverCommon.UB8(99),
					lobscnl:            2,
					lobscn:             []driverCommon.UB4{0x0, 0x1},
					operation:          kplobWrite,
				}
			},
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			originalLogger := common.Odl
			common.Odl = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
			t.Cleanup(func() { common.Odl = originalLogger })
			msg := newTTIlob().(*tTIlob)
			msg.SetDefinition(tc.def())

			buf, mar := NewMarshalEngineTest(driverCommon.BIG_ENDIAN, Universal, Universal, 4096)
			if err := msg.MarshalTo(ctx, mar); err != nil {
				t.Fatalf("MarshalTo returned error: %v", err)
			}
			if buf.currentWritePosition == 0 {
				t.Fatalf("expected marshalling to produce output bytes")
			}
		})
	}
}

// TestTTIlob_MarshalTo_WithoutDefinition ensures MarshalTo returns an
// InvalidLOBBuffer error when invoked without a configured definition.
func TestTTIlob_MarshalTo_WithoutDefinition(t *testing.T) {
	t.Parallel()
	msg := newTTIlob().(*tTIlob)

	err := msg.MarshalTo(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error when definition is not configured")
	}

	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected error type common.SQLError, got %T", err)
	}

	if got, want := sqlErr.ErrorCode(), string(oracleErrors.InvalidLOBBuffer); got != want {
		t.Fatalf("unexpected error code: got %s want %s", got, want)
	}

	if !strings.Contains(err.Error(), "definition not configured") {
		t.Fatalf("expected error message to mention definition configuration, got %q", err.Error())
	}
}

// TestTTIlob_MarshalTo_Fail injects write failures via a faulty data buffer using a
// table-driven structure to ensure coverage across multiple write stages.
func TestTTIlob_MarshalTo_Fail(t *testing.T) {
	t.Parallel()
	makeCommonDefinition := func() *lobDefinition {
		return &lobDefinition{
			sourceLocator:      newLocator(driverCommon.B1Array{0x01, 0x02, 0x03, 0x04}, 0),
			destinationLocator: newLocator(driverCommon.B1Array{0x05, 0x06, 0x07, 0x08}, 0),
			charsetID:          driverCommon.UB2(873),
			sendLobAmt:         true,
			lobAmt:             driverCommon.UB8(1),
			lobscnl:            1,
			lobscn:             []driverCommon.UB4{0x01},
			operation:          kplobWrite,
		}
	}

	tests := []struct {
		name      string
		failCount int
		failType  FailOn
		mutate    func(*lobDefinition)
	}{
		{name: "header-write-byte", failCount: 1, failType: failOnWriteByte},
		{name: "lobsrc pointer-write", failCount: 3, failType: failOnWriteByte},
		{name: "lobslen write", failCount: 2, failType: failOnWriteBytes},
		{name: "lobdst pointer-write", failCount: 4, failType: failOnWriteByte},
		{name: "lobdlen write", failCount: 3, failType: failOnWriteBytes},
		{name: "lobsoff write", failCount: 4, failType: failOnWriteBytes},
		{name: "lobdoff write", failCount: 5, failType: failOnWriteBytes},
		{name: "lobchr pointer-write", failCount: 5, failType: failOnWriteByte},
		{name: "lobamt nullpointer-write", failCount: 6, failType: failOnWriteByte},
		{name: "lobnull pointer-write", failCount: 7, failType: failOnWriteByte},
		{name: "lobops write", failCount: 6, failType: failOnWriteBytes},
		{name: "lobscn pointer-write", failCount: 8, failType: failOnWriteByte},
		{name: "lobscnl write", failCount: 7, failType: failOnWriteBytes},
		{name: "src offset write", failCount: 8, failType: failOnWriteBytes},
		{name: "dest offset write", failCount: 9, failType: failOnWriteBytes},
		{name: "lobamt pointer-write", failCount: 9, failType: failOnWriteByte},
		{name: "nullpointer-write", failCount: 10, failType: failOnWriteByte},
		{name: "nullpointer-write", failCount: 11, failType: failOnWriteByte},
		{name: "nullpointer-write", failCount: 12, failType: failOnWriteByte},
		{name: "0 write", failCount: 10, failType: failOnWriteBytes},
		{name: "0 write", failCount: 11, failType: failOnWriteBytes},
		{name: "0 write", failCount: 12, failType: failOnWriteBytes},
		{name: "src locator write", failCount: 13, failType: failOnWriteBytes},
		{name: "dest locator write", failCount: 14, failType: failOnWriteBytes},
		{name: "charset write", failCount: 15, failType: failOnWriteBytes},
		{name: "lobscn write", failCount: 16, failType: failOnWriteBytes},
		{name: "lobamt write", failCount: 17, failType: failOnWriteBytes},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := newTTIlob().(*tTIlob)
			definition := makeCommonDefinition()
			if tc.mutate != nil {
				tc.mutate(definition)
			}
			msg.SetDefinition(definition)

			payload := make([]byte, 256)
			mar := createMarshaller(payload, tc.failType, tc.failCount)
			err := msg.MarshalTo(ctx, mar)
			if err == nil {
				t.Fatalf("expected MarshalTo to fail for %s", tc.name)
			}
			if !strings.Contains(err.Error(), "simulated write error") {
				t.Fatalf("expected error message to contain \"simulated write error\", got: %v", err)
			}
		})
	}
}
