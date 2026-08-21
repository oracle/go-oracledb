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
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// -----------------------------------------------------------------------------
// Golden payload fixtures
// -----------------------------------------------------------------------------

// lobRpaReadGoldenPayload holds the TTC message body (post-header) for the LOB RPA
// response used by TestTTILobRpa_UnMarshalFrom_Success.
var lobRpaReadGoldenPayload = []string{
	`"00 70 00"`,
	`"02 02 0C 82 00 00 02 00"`,
	`"00 00 01 00 00 00 2D 71"`,
	`"67 00 01 3E 99 00 01 3E"`,
	`"98 00 03 00 03 03 69 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"03 69 7A 13 BA 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 3B 31 5B 6F 00 00 00"`,
	`"00 00 00 DE AD BE EF 00"`,
	`"01 00 22 00 00 00 00 00"`,
	`"2A 03 FE 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 01 3E"`,
	`"98 00 40 8F 3E 00 00 02"`,
	`"11 8F"`,
}

var lobRpaCreateGoldenPayloadBytes = []string{
	`"00 26 00 01 82"`,
	`"08 80 03 00 02 F2 CB 00"`,
	`"00 00 3B 00 00 00 01 03"`,
	`"69 00 0A 00 00 00 01 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"01 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 02"`,
	`"03 69 01 60 01 01 04 01"`,
	`"01 01 3D 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 06 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00"`,
}

var lobRpaPageSizeGoldenPayload = []string{
	`"00 26 00 01 82"`,
	`"08 80 03 00 02 F2 CB 00"`,
	`"00 00 3B 00 00 00 01 03"`,
	`"69 00 0A 00 00 00 01 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"01 00 00 02 1F C4 04 01"`,
	`"01 01 3E 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 07 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00"`,
}

var lobRpaWriteGoldenPayload = []string{
	`"00 26 00 01 82"`,
	`"08 80 03 00 02 F2 CB 00"`,
	`"00 00 3B 00 00 00 01 03"`,
	`"69 00 0A 00 00 00 01 00"`,
	`"00 49 2E 32 09 00 00 00"`,
	`"01 00 00 02 11 94 04 01"`,
	`"01 01 3F 00 00 00 00 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00 00 00 00 00 00 08 00"`,
	`"00 00 00 00 00 00 00 00"`,
	`"00"`,
}

// -----------------------------------------------------------------------------
// Test helper functions
// -----------------------------------------------------------------------------

// newLobRPAMarshaller creates a BIG_ENDIAN marshaller primed with the provided payload.
func newLobRPAMarshaller(payload []byte) common.Marshaller {
	buf := NewArrayDataBuffer(len(payload))
	_ = buf.WriteBytesWithContext(context.Background(), payload)
	return NewMarshalEngine(buf, common.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
}

// -----------------------------------------------------------------------------
// Expected locator fixtures
// -----------------------------------------------------------------------------

var expectedReadSourceLocator = common.B1Array{
	0, 112, 0, 2, 2, 12, 130, 0, 0, 2, 0, 0, 0, 1, 0, 0,
	0, 45, 113, 103, 0, 1, 62, 153, 0, 1, 62, 152, 0, 3,
	0, 3, 3, 105, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 105, 122,
	19, 186, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 59, 49,
	91, 111, 0, 0, 0, 0, 0, 0, 222, 173, 190, 239, 0, 1,
	0, 34, 0, 0, 0, 0, 0, 42, 3, 254, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 62, 152,
	0, 64, 143, 62, 0, 0,
}

var expectedCreateSourceLocator = common.B1Array{
	0, 38, 0, 1, 130, 8, 128, 3, 0, 2, 242, 203, 0, 0, 0,
	59, 0, 0, 0, 1, 3, 105, 0, 10, 0, 0, 0, 1, 0, 0, 73, 46,
	50, 9, 0, 0, 0, 1, 0, 0,
}

var expectedPageSizeSourceLocator = common.B1Array{
	0, 38, 0, 1, 130, 8, 128, 3, 0, 2, 242, 203, 0, 0, 0,
	59, 0, 0, 0, 1, 3, 105, 0, 10, 0, 0, 0, 1, 0, 0, 73, 46,
	50, 9, 0, 0, 0, 1, 0, 0,
}

var expectedWriteSourceLocator = common.B1Array{
	0, 38, 0, 1, 130, 8, 128, 3, 0, 2, 242, 203, 0, 0, 0,
	59, 0, 0, 0, 1, 3, 105, 0, 10, 0, 0, 0, 1, 0, 0, 73, 46,
	50, 9, 0, 0, 0, 1, 0, 0}

// lobDefinitionExpectations groups optional expectation fields for verifying a lobDefinition.
type lobDefinitionExpectations struct {
	sourceLocatorLen      *int
	sourceLocator         []byte
	destinationLocatorLen *int
	charsetID             *common.UB2
	sendLobAmt            *bool
	lobAmt                *common.UB8
	lobNull               *bool
}

func assertLobDefinition(t *testing.T, def *lobDefinition, exp lobDefinitionExpectations) {
	t.Helper()
	sourceLocatorLen := 0
	if def.sourceLocator != nil {
		sourceLocatorLen = len(def.sourceLocator.locatorBytes)
	}
	if exp.sourceLocatorLen != nil && sourceLocatorLen != *exp.sourceLocatorLen {
		t.Fatalf("unexpected source locator length: got %d want %d", sourceLocatorLen, *exp.sourceLocatorLen)
	}
	destinationLocatorLen := 0
	if def.destinationLocator != nil {
		destinationLocatorLen = len(def.destinationLocator.locatorBytes)
	}
	if exp.destinationLocatorLen != nil && destinationLocatorLen != *exp.destinationLocatorLen {
		t.Fatalf("unexpected destination locator length: got %d want %d", destinationLocatorLen, *exp.destinationLocatorLen)
	}
	if exp.charsetID != nil && def.charsetID != *exp.charsetID {
		t.Fatalf("unexpected charsetID: got %d want %d", def.charsetID, *exp.charsetID)
	}
	if exp.sendLobAmt != nil && def.sendLobAmt != *exp.sendLobAmt {
		t.Fatalf("unexpected sendLobAmt: got %t want %t", def.sendLobAmt, *exp.sendLobAmt)
	}
	if exp.lobAmt != nil && def.lobAmt != *exp.lobAmt {
		t.Fatalf("unexpected lobAmt: got %d want %d", def.lobAmt, *exp.lobAmt)
	}
	if exp.lobNull != nil && def.lobNull != *exp.lobNull {
		t.Fatalf("unexpected lobNull: got %t want %t", def.lobNull, *exp.lobNull)
	}
	if exp.sourceLocator != nil {
		if def.sourceLocator == nil || !bytes.Equal(def.sourceLocator.locatorBytes, exp.sourceLocator) {
			var got []byte
			if def.sourceLocator != nil {
				got = append([]byte(nil), []byte(def.sourceLocator.locatorBytes)...)
			}
			t.Fatalf("unexpected source locator: got %v want %v", got, exp.sourceLocator)
		}
	}
}

func intPtr(v int) *int { return &v }

func ub2Ptr(v common.UB2) *common.UB2 { return &v }

func ub8Ptr(v common.UB8) *common.UB8 { return &v }

func boolPtr(v bool) *bool { return &v }

type lobRpaSuccessCase struct {
	name    string
	payload []string
	setup   func() *lobDefinition
	expect  lobDefinitionExpectations
}

// runLobRpaSuccessCase executes the happy-path unmarshalling scenario for TTC LOB RPA messages.
func runLobRpaSuccessCase(t *testing.T, tc lobRpaSuccessCase) {
	t.Helper()
	ctx := context.Background()
	buf, err := ExtractBytesFromDump(tc.payload)
	if err != nil {
		t.Fatalf("ExtractBytesFromDump returned error: %v", err)
	}
	mar := newLobRPAMarshaller(buf)

	msg := newTTILobRPA().(*ttiLobRpa)
	if msg.GetMsgCode() != TTIRPA {
		t.Fatalf("GetMsgCode mismatch: got %v want %v", msg.GetMsgCode(), TTIRPA)
	}

	def := tc.setup()
	msg.SetDefinition(def)

	if err := msg.UnMarshalFrom(ctx, mar); err != nil {
		t.Fatalf("UnMarshalFrom returned error: %v", err)
	}

	assertLobDefinition(t, def, tc.expect)
}

func TestTTILobRpa_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()

	cases := []lobRpaSuccessCase{
		{
			name:    "read operation",
			payload: lobRpaReadGoldenPayload,
			setup: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator:      &locator{locatorBytes: make(common.B1Array, 114)},
					destinationLocator: nil,
					charsetID:          common.UB2(0),
					sendLobAmt:         true,
					nullO2U:            false,
					operation:          kplobRead,
				}
			},
			expect: lobDefinitionExpectations{
				sourceLocatorLen:      intPtr(114),
				sourceLocator:         append([]byte(nil), expectedReadSourceLocator...),
				destinationLocatorLen: intPtr(0),
				charsetID:             ub2Ptr(common.UB2(0)),
				sendLobAmt:            boolPtr(true),
				lobAmt:                ub8Ptr(common.UB8(4495)),
				lobNull:               boolPtr(false),
			},
		},
		{
			name:    "create temporary LOB operation",
			payload: lobRpaCreateGoldenPayloadBytes,
			setup: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator:      &locator{locatorBytes: make(common.B1Array, 108)},
					destinationLocator: nil,
					charsetID:          873,
					sendLobAmt:         true,
					nullO2U:            true,
					operation:          kplobTmpCreate,
				}
			},
			expect: lobDefinitionExpectations{
				sourceLocatorLen: intPtr(40),
				sourceLocator:    append([]byte(nil), expectedCreateSourceLocator...),
				charsetID:        ub2Ptr(common.UB2(873)),
				sendLobAmt:       boolPtr(true),
				lobAmt:           ub8Ptr(common.UB8(96)),
				lobNull:          boolPtr(true),
			},
		},
		{
			name:    "page size LOB operation",
			payload: lobRpaPageSizeGoldenPayload,
			setup: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator:      &locator{locatorBytes: make(common.B1Array, 40)},
					destinationLocator: nil,
					charsetID:          common.UB2(0),
					sendLobAmt:         true,
					nullO2U:            false,
					operation:          kplobPageSize,
				}
			},
			expect: lobDefinitionExpectations{
				sourceLocatorLen: intPtr(40),
				sourceLocator:    append([]byte(nil), expectedPageSizeSourceLocator...),
				charsetID:        ub2Ptr(common.UB2(0)),
				sendLobAmt:       boolPtr(true),
				lobAmt:           ub8Ptr(common.UB8(8132)),
				lobNull:          boolPtr(false),
			},
		},
		{
			name:    "write LOB operation",
			payload: lobRpaWriteGoldenPayload,
			setup: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator:      &locator{locatorBytes: make(common.B1Array, 40)},
					destinationLocator: nil,
					charsetID:          common.UB2(0),
					sendLobAmt:         true,
					nullO2U:            false,
					operation:          kplobWrite,
				}
			},
			expect: lobDefinitionExpectations{
				sourceLocatorLen: intPtr(40),
				sourceLocator:    append([]byte(nil), expectedWriteSourceLocator...),
				charsetID:        ub2Ptr(common.UB2(0)),
				sendLobAmt:       boolPtr(true),
				lobAmt:           ub8Ptr(common.UB8(4500)),
				lobNull:          boolPtr(false),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runLobRpaSuccessCase(t, tc)
		})
	}
}

type lobRpaFailureCase struct {
	name       string
	payload    []string
	definition func() *lobDefinition
	failsOn    FailOn
	failCount  int
}

// runLobRpaFailureCase executes an unmarshalling test that is expected to fail with a simulated read error.
func runLobRpaFailureCase(t *testing.T, tc lobRpaFailureCase) {
	t.Helper()
	payloadBytes, err := ExtractBytesFromDump(tc.payload)
	if err != nil {
		t.Fatalf("ExtractBytesFromDump returned error: %v", err)
	}
	mar := createMarshaller(payloadBytes, tc.failsOn, tc.failCount)

	msg := newTTILobRPA().(*ttiLobRpa)
	msg.SetDefinition(tc.definition())

	err = msg.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated read error") {
		t.Fatalf("expected simulated read error, got %v", err)
	}
}

func TestTTILobRpa_UnMarshalFrom_Failure(t *testing.T) {
	t.Parallel()

	readPayload := lobRpaReadGoldenPayload
	createPayload := lobRpaCreateGoldenPayloadBytes

	cases := []lobRpaFailureCase{
		{
			name:    "temporary locator header read failure",
			payload: createPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator: &locator{locatorBytes: make(common.B1Array, 40)},
					operation:     kplobTmpCreate,
				}
			},
			failsOn:   failOnReadBytes,
			failCount: 1,
		},
		{
			name:    "temporary locator body read failure",
			payload: createPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator: &locator{locatorBytes: make(common.B1Array, 40)},
					operation:     kplobTmpCreate,
				}
			},
			failsOn:   failOnReadBytes,
			failCount: 2,
		},
		{
			name:    "temporary locator discard unused bytes failure",
			payload: createPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator: &locator{locatorBytes: make(common.B1Array, 60)},
					operation:     kplobTmpCreate,
				}
			},
			failsOn:   failOnReadBytes,
			failCount: 3,
		},
		{
			name:    "source locator read failure",
			payload: readPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					sourceLocator: &locator{locatorBytes: make(common.B1Array, 8)},
					operation:     kplobRead,
				}
			},
			failsOn:   failOnReadBytes,
			failCount: 1,
		},
		{
			name:    "destination locator length read failure",
			payload: readPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					destinationLocator: &locator{locatorBytes: make(common.B1Array, 10)},
				}
			},
			failsOn:   failOnReadByte,
			failCount: 1,
		},
		{
			name:    "destination locator body read failure",
			payload: readPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					destinationLocator: &locator{locatorBytes: make(common.B1Array, 10)},
				}
			},
			failsOn:   failOnReadBytes,
			failCount: 1,
		},
		{
			name:    "charset id read failure",
			payload: readPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					charsetID: common.UB2(42),
				}
			},
			failsOn:   failOnReadByte,
			failCount: 1,
		},
		{
			name:    "lob amount read failure",
			payload: readPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					sendLobAmt: true,
				}
			},
			failsOn:   failOnReadByte,
			failCount: 1,
		},
		{
			name:    "lob null read failure",
			payload: readPayload,
			definition: func() *lobDefinition {
				return &lobDefinition{
					nullO2U: true,
				}
			},
			failsOn:   failOnReadByte,
			failCount: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runLobRpaFailureCase(t, tc)
		})
	}
}

func TestTTILobRpa_SetDefinition_NilDefinition(t *testing.T) {
	t.Parallel()

	msg := newTTILobRPA().(*ttiLobRpa)

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
