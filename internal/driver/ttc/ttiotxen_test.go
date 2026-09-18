/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	extensions "github.com/oracle/go-oracledb/v26/oracle/extensions"
)

func newOTxEnEngine(capacity int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	buf := NewArrayDataBuffer(capacity)
	engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	return buf, engine
}

// TestOTxEnFactoryRegistration verifies that OTXEN request messages are
// registered for both legacy and TTC 18+ protocol versions.
func TestOTxEnFactoryRegistration(t *testing.T) {
	t.Parallel()

	functionRegistry := NewRegistry[functionRegistryKey]()
	if err := functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxEn}, 18, newOTxEn18); err != nil {
		t.Fatalf("register TTC 18+ OTXEN failed: %v", err)
	}
	if err := functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxEn}, MinTTCProtocolVersion, newOTxEn); err != nil {
		t.Fatalf("register legacy OTXEN failed: %v", err)
	}

	factory := &SimpleFactory{
		ttcVersion:   18,
		msgregistry:  NewRegistry[driverCommon.MessageType](),
		funcregistry: functionRegistry,
	}
	msg, err := factory.GetMessageForFunction(TTIFUN, oTxEn)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIFUN, oTxEn) failed: %v", err)
	}
	if msg.GetMsgCode() != TTIFUN {
		t.Fatalf("message code = %v, want %v", msg.GetMsgCode(), TTIFUN)
	}
	if got := msg.(driverCommon.Function).GetFuncCode(); got != oTxEn {
		t.Fatalf("function code = %v, want %v", got, oTxEn)
	}
}

// TestOTxEnMarshalTo verifies that a sessionless commit OTXEN message matches
// the expected TTC wire layout.
func TestOTxEnMarshalTo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	xid := driverCommon.B1Array{0x11, 0x22, 0x33, 0x44}
	tx := &sessionlessTransaction{
		globalTransactionID:       extensions.GlobalTransactionID("g1"),
		xid:                       xid,
		globalTransactionIDLength: 2,
		bqualLength:               2,
		timeout:                   30,
	}

	msg := newOTxEn18().(*tTIOtxen)
	msg.configureForCommit(tx)

	buf, engine := newOTxEnEngine(256)
	if err := msg.MarshalTo(ctx, engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]
	if got[0] != byte(oTxEn) {
		t.Fatalf("function code = %d, want %d", got[0], oTxEn)
	}

	idx := 3 // TTC 18+ function, sequence, and token fields.
	assertUniversal := func(name string, want int) {
		t.Helper()
		value, size, ok := decodeUniversalAt(got, idx)
		if !ok {
			t.Fatalf("could not decode %s at offset %d", name, idx)
		}
		if value != want {
			t.Fatalf("%s = %d, want %d", name, value, want)
		}
		idx += size
	}
	assertPointer := func(name string, want byte) {
		t.Helper()
		if idx >= len(got) {
			t.Fatalf("%s is missing, want %#x", name, want)
		}
		if got[idx] != want {
			t.Fatalf("%s = %#x, want %#x", name, got[idx], want)
		}
		idx++
	}

	assertUniversal("operation", int(otxenCommit))
	assertPointer("transaction context pointer", 0)
	assertUniversal("transaction context length", 0)
	assertUniversal("format ID", int(k2gSessionless))
	assertUniversal("global transaction ID length", 2)
	assertUniversal("BQUAL length", 2)
	assertPointer("XID pointer", 1)
	assertUniversal("XID length", len(xid))
	assertUniversal("timeout", 30)
	assertUniversal("in state", int(k2cmdCommit))
	assertPointer("out-state pointer", 1)
	assertUniversal("transaction state change flags", 0)

	if !bytes.Equal(got[idx:idx+len(xid)], xid) {
		t.Fatalf("XID bytes = % X, want % X", got[idx:idx+len(xid)], xid)
	}
	idx += len(xid)
	if idx != len(got) {
		t.Fatalf("unexpected trailing OTXEN bytes: % X", got[idx:])
	}
}

// TestOTxEnMarshalToEmptyVariableData verifies that OTXEN encodes null
// pointers correctly when transaction context and XID data are absent.
func TestOTxEnMarshalToEmptyVariableData(t *testing.T) {
	t.Parallel()

	msg := newOTxEn().(*tTIOtxen)
	msg.configureForAbort(nil)
	buf, engine := newOTxEnEngine(128)
	if err := msg.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}
	got := buf.bytes[:buf.currentWritePosition]

	idx := 2 // legacy header contains the function code and sequence number.
	_, size, ok := decodeUniversalAt(got, idx)
	if !ok {
		t.Fatal("could not decode operation")
	}
	idx += size
	if got[idx] != 0 {
		t.Fatalf("transaction context pointer = %#x, want null", got[idx])
	}
	idx++
	idx += universalSizeAt(t, got, idx, "transaction context length")
	idx += universalSizeAt(t, got, idx, "format ID")
	idx += universalSizeAt(t, got, idx, "global transaction ID length")
	idx += universalSizeAt(t, got, idx, "BQUAL length")
	if got[idx] != 0 {
		t.Fatalf("XID pointer = %#x, want null", got[idx])
	}
	idx++
	idx += universalSizeAt(t, got, idx, "XID length")
	idx += universalSizeAt(t, got, idx, "timeout")
	idx += universalSizeAt(t, got, idx, "in state")
	if got[idx] != 1 {
		t.Fatalf("out-state pointer = %#x, want non-null", got[idx])
	}
	idx++
	idx += universalSizeAt(t, got, idx, "transaction state change flags")
}

// TestOTxEnMarshalToErrors verifies that OTXEN returns FailMarshal for every
// field-level write failure, including both null and non-null variable pointers.
func TestOTxEnMarshalToErrors(t *testing.T) {
	t.Parallel()

	newMessage := func(withContext, withXID bool) *tTIOtxen {
		msg := newOTxEn18().(*tTIOtxen)
		msg.operation = driverCommon.SB4(otxenCommit)
		msg.formatID = k2gSessionless
		msg.globalTransactionIDLength = 2
		msg.bqualLength = 2
		msg.timeout = 30
		msg.inState = k2cmdCommit
		if withContext {
			msg.transactionContext = driverCommon.B1Array{0xAA, 0xBB}
		}
		if withXID {
			msg.xid = driverCommon.B1Array{0x11, 0x22, 0x33, 0x44}
		}
		return msg
	}

	tests := []struct {
		name      string
		failOn    FailOn
		failCount int
		message   func() *tTIOtxen
	}{
		{name: "header", failOn: failOnWriteByte, failCount: 1, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "operation", failOn: failOnWriteBytes, failCount: 2, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "transaction context pointer", failOn: failOnWriteByte, failCount: 3, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "null transaction context pointer", failOn: failOnWriteByte, failCount: 3, message: func() *tTIOtxen { return newMessage(false, true) }},
		{name: "transaction context length", failOn: failOnWriteBytes, failCount: 3, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "format ID", failOn: failOnWriteBytes, failCount: 4, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "global transaction ID length", failOn: failOnWriteBytes, failCount: 5, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "BQUAL length", failOn: failOnWriteBytes, failCount: 6, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "XID pointer", failOn: failOnWriteByte, failCount: 4, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "null XID pointer", failOn: failOnWriteByte, failCount: 4, message: func() *tTIOtxen { return newMessage(true, false) }},
		{name: "XID length", failOn: failOnWriteBytes, failCount: 7, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "timeout", failOn: failOnWriteBytes, failCount: 8, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "in state", failOn: failOnWriteBytes, failCount: 9, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "out-state pointer", failOn: failOnWriteByte, failCount: 5, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "transaction state change flags", failOn: failOnWriteBytes, failCount: 10, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "transaction context", failOn: failOnWriteBytes, failCount: 11, message: func() *tTIOtxen { return newMessage(true, true) }},
		{name: "XID", failOn: failOnWriteBytes, failCount: 12, message: func() *tTIOtxen { return newMessage(true, true) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := test.message()
			err := msg.MarshalTo(context.Background(), createMarshaller(make([]byte, 256), test.failOn, test.failCount))
			assertFailMarshalError(t, err)
		})
	}
}

func assertFailMarshalError(t *testing.T, err error) {
	t.Helper()
	assertOracleErrorCode(t, err, oracleErrors.FailMarshal)
}

func assertOracleErrorCode(t *testing.T, err error, want oracleErrors.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", want)
	}
	sqlErr, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("error type = %T, want oracle error, got %v", err, err)
	}
	if sqlErr.ErrorCode() != string(want) {
		t.Fatalf("error code = %q, want %q", sqlErr.ErrorCode(), want)
	}
}

// TestOTxEnConfigureOperations verifies that commit and rollback operations
// populate the expected OTXEN opcode, K2 state, XID, and timeout.
func TestOTxEnConfigureOperations(t *testing.T) {
	t.Parallel()

	tx := &sessionlessTransaction{
		globalTransactionID:       extensions.GlobalTransactionID("g1"),
		xid:                       driverCommon.B1Array{0x11, 0x22, 0x33, 0x44},
		globalTransactionIDLength: 2,
		bqualLength:               2,
		timeout:                   30,
	}
	tests := []struct {
		name      string
		configure func(*tTIOtxen, oracleTx)
		operation driverCommon.SB4
		inState   driverCommon.UB4
	}{
		{name: "commit", configure: func(msg *tTIOtxen, tx oracleTx) { msg.configureForCommit(tx) }, operation: driverCommon.SB4(otxenCommit), inState: k2cmdCommit},
		{name: "abort", configure: func(msg *tTIOtxen, tx oracleTx) { msg.configureForAbort(tx) }, operation: driverCommon.SB4(otxenAbort), inState: k2cmdAbort},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := newOTxEn().(*tTIOtxen)
			test.configure(msg, tx)

			if msg.operation != test.operation {
				t.Fatalf("operation = %d, want %d", msg.operation, test.operation)
			}
			if msg.inState != test.inState {
				t.Fatalf("in-state = %d, want %d", msg.inState, test.inState)
			}
			if msg.flags != 0 {
				t.Fatalf("flags = %#x, want 0", msg.flags)
			}
			if msg.formatID != k2gSessionless {
				t.Fatalf("format ID = %#x, want %#x", msg.formatID, k2gSessionless)
			}
			if !bytes.Equal(msg.xid, tx.xid) {
				t.Fatalf("XID = % X, want % X", msg.xid, tx.xid)
			}
			if msg.timeout != driverCommon.UB2(tx.timeout) {
				t.Fatalf("timeout = %d, want %d", msg.timeout, tx.timeout)
			}
		})
	}
}

// TestOTxEnRPAFactoryRegistration verifies that the OTXEN return-parameter
// decoder is registered for TTIRPA responses.
func TestOTxEnRPAFactoryRegistration(t *testing.T) {
	t.Parallel()

	functionRegistry := NewRegistry[functionRegistryKey]()
	if err := functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxEn}, MinTTCProtocolVersion, newOTxEnRPA); err != nil {
		t.Fatalf("register OTXEN RPA failed: %v", err)
	}
	factory := &SimpleFactory{
		ttcVersion:   18,
		msgregistry:  NewRegistry[driverCommon.MessageType](),
		funcregistry: functionRegistry,
	}

	msg, err := factory.GetMessageForFunction(TTIRPA, oTxEn)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIRPA, oTxEn) failed: %v", err)
	}
	if _, ok := msg.(*ttiOTxEnRPA); !ok {
		t.Fatalf("message type = %T, want *ttiOTxEnRPA", msg)
	}
}

// TestOTxEnRPAUnMarshalFrom verifies that an OTXEN return-state value is
// decoded from a TTIRPA payload.
func TestOTxEnRPAUnMarshalFrom(t *testing.T) {
	t.Parallel()

	buf := NewTestDataBuffer()
	engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	if err := engine.MarshalUB4(context.Background(), 0x12345678); err != nil {
		t.Fatalf("MarshalUB4 failed: %v", err)
	}

	msg := newOTxEnRPA().(*ttiOTxEnRPA)
	if err := msg.UnMarshalFrom(context.Background(), engine); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	if got := msg.GetOutState(); got != 0x12345678 {
		t.Fatalf("GetOutState = %#x, want %#x", got, 0x12345678)
	}
}

// TestOTxEnRPAUnMarshalFromFailure verifies that malformed OTXEN return
// parameters are reported as unmarshalling errors.
func TestOTxEnRPAUnMarshalFromFailure(t *testing.T) {
	t.Parallel()

	msg := newOTxEnRPA().(*ttiOTxEnRPA)
	err := msg.UnMarshalFrom(context.Background(), createMarshaller([]byte{0x01}, failOnReadByte, 1))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated read error") {
		t.Fatalf("expected simulated read error, got %v", err)
	}
}

func universalSizeAt(t *testing.T, payload []byte, index int, name string) int {
	t.Helper()
	_, size, ok := decodeUniversalAt(payload, index)
	if !ok {
		t.Fatalf("could not decode %s at offset %d", name, index)
	}
	return size
}
