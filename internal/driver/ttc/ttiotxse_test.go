/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"slices"
	"strings"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/go-oracledb/v26/oracle/extensions"
)

func newOTxSeEngine(capacity int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	buf := NewArrayDataBuffer(capacity)
	engine := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	return buf, engine
}

// TestOTxSe_FactoryRegistration_MessageCodes verifies that OTXSE is registered
// for both direct TTIFUN calls and TTIPFN piggyback calls, and that each lookup
// returns a message with the expected TTC message code.
func TestOTxSe_FactoryRegistration_MessageCodes(t *testing.T) {
	t.Parallel()

	functionRegistry := NewRegistry[functionRegistryKey]()
	if err := functionRegistry.Register(functionRegistryKey{messageType: TTIFUN, functionType: oTxSe}, 18, newOTxSe18); err != nil {
		t.Fatalf("register TTIFUN OTXSE 18 failed: %v", err)
	}
	if err := functionRegistry.Register(functionRegistryKey{messageType: TTIPFN, functionType: oTxSe}, 18, newOTxSePfn18); err != nil {
		t.Fatalf("register TTIPFN OTXSE 18 failed: %v", err)
	}
	factory := &SimpleFactory{ttcVersion: 18, msgregistry: NewRegistry[driverCommon.MessageType](), funcregistry: functionRegistry}

	msg, err := factory.GetMessageForFunction(TTIFUN, oTxSe)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIFUN, oTxSe) failed: %v", err)
	}
	if msg.GetMsgCode() != TTIFUN {
		t.Fatalf("TTIFUN registration returned msg code %v, want %v", msg.GetMsgCode(), TTIFUN)
	}
	if got := msg.(interface {
		GetFuncCode() driverCommon.FunctionType
	}).GetFuncCode(); got != oTxSe {
		t.Fatalf("TTIFUN registration returned func code %v, want %v", got, oTxSe)
	}

	pfnMsg, err := factory.GetMessageForFunction(TTIPFN, oTxSe)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIPFN, oTxSe) failed: %v", err)
	}
	if pfnMsg.GetMsgCode() != TTIPFN {
		t.Fatalf("TTIPFN registration returned msg code %v, want %v", pfnMsg.GetMsgCode(), TTIPFN)
	}
	if got := pfnMsg.(interface {
		GetFuncCode() driverCommon.FunctionType
	}).GetFuncCode(); got != oTxSe {
		t.Fatalf("TTIPFN registration returned func code %v, want %v", got, oTxSe)
	}
}

// TestOTxSe_MarshalTo_StartSessionless verifies that a sessionless start
// request is configured with the expected transaction identifier and options.
func TestOTxSe_MarshalTo_StartSessionless(t *testing.T) {
	t.Parallel()

	msg := newOTxSe18().(*tTIOtxse)
	xid := driverCommon.B1Array{0x11, 0x22, 0x33, 0x44}
	tx := &sessionlessTransaction{
		globalTransactionID:       extensions.GlobalTransactionID("g1"),
		xid:                       xid,
		timeout:                   30,
		bqualLength:               2,
		globalTransactionIDLength: 2,
	}
	msg.configureForStart(tx, driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadCommitted),
	})

	buf, engine := newOTxSeEngine(512)
	if err := msg.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	if len(got) == 0 {
		t.Fatal("expected non-empty OTXSE payload")
	}
	if got[0] != byte(oTxSe) {
		t.Fatalf("first byte = %d, want function code %d", got[0], oTxSe)
	}
	// The test engine uses universal UB4 encoding, where UB4(0) is one byte.
	const applicationValueSize = 1
	if len(got) < len(xid)+applicationValueSize ||
		!bytes.Equal(got[len(got)-applicationValueSize-len(xid):len(got)-applicationValueSize], xid) {
		t.Fatalf("expected XID payload before application value, got % X", got[len(got)-applicationValueSize-len(xid):])
	}
	if !bytes.Equal(got[len(got)-applicationValueSize:], []byte{0}) {
		t.Fatalf("expected trailing application value UB4(0), got % X", got[len(got)-applicationValueSize:])
	}
	if msg.operation != otxseStart {
		t.Fatalf("operation = %d, want %d", msg.operation, otxseStart)
	}
	if !bytes.Equal(msg.xid, xid) {
		t.Fatalf("XID = % X, want % X", msg.xid, xid)
	}
	if msg.formatID != k2gSessionless {
		t.Fatalf("format ID = %#x, want %#x", msg.formatID, k2gSessionless)
	}
	if msg.globalTransactionIDLength != 2 || msg.bqualLength != 2 {
		t.Fatalf("XID lengths = (%d, %d), want (2, 2)", msg.globalTransactionIDLength, msg.bqualLength)
	}
	if msg.flags != otxseTransSessionless|otxseTransNew|otxseTransReadWrite {
		t.Fatalf("flags = %#x, want %#x", msg.flags, otxseTransSessionless|otxseTransNew|otxseTransReadWrite)
	}
	if msg.timeout != 30 {
		t.Fatalf("timeout = %d, want 30", msg.timeout)
	}
}

// TestOTxSe_MarshalTo_Suspend verifies that a suspend request is configured
// with the sessionless detach operation and no transaction identifier.
func TestOTxSe_MarshalTo_Suspend(t *testing.T) {
	t.Parallel()

	msg := newOTxSe().(*tTIOtxse)
	msg.configureForSuspend()

	buf, engine := newOTxSeEngine(256)
	if err := msg.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	if len(got) == 0 {
		t.Fatal("expected non-empty OTXSE detach payload")
	}
	if msg.operation != otxseDetach {
		t.Fatalf("operation = %d, want %d", msg.operation, otxseDetach)
	}
	if msg.flags != otxseTransSessionless {
		t.Fatalf("flags = %#x, want %#x", msg.flags, otxseTransSessionless)
	}
	if msg.formatID != k2gSessionless {
		t.Fatalf("format ID = %#x, want %#x", msg.formatID, k2gSessionless)
	}
	if len(msg.xid) != 0 {
		t.Fatalf("XID length = %d, want 0", len(msg.xid))
	}
	if msg.applicationValue == nil || *msg.applicationValue != 0 {
		t.Fatalf("application value = %v, want non-null UB4(0)", msg.applicationValue)
	}
}

// TestOTxSeMarshalToErrors verifies that OTXSE returns FailMarshal for every
// field-level write failure, including null and non-null optional pointers.
func TestOTxSeMarshalToErrors(t *testing.T) {
	t.Parallel()

	newStartMessage := func() *tTIOtxse {
		msg := newOTxSe18().(*tTIOtxse)
		msg.configureForStart(&sessionlessTransaction{
			xid:                       driverCommon.B1Array{0x11, 0x22, 0x33, 0x44},
			globalTransactionIDLength: 2,
			bqualLength:               2,
			timeout:                   30,
		}, driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelReadCommitted)})
		return msg
	}
	newSuspendMessage := func() *tTIOtxse {
		msg := newOTxSe18().(*tTIOtxse)
		msg.configureForSuspend()
		return msg
	}
	newMessageWithoutApplicationValue := func() *tTIOtxse {
		return newOTxSe18().(*tTIOtxse)
	}

	tests := []struct {
		name      string
		failOn    FailOn
		failCount int
		message   func() *tTIOtxse
	}{
		{name: "header", failOn: failOnWriteByte, failCount: 1, message: newStartMessage},
		{name: "operation", failOn: failOnWriteBytes, failCount: 2, message: newStartMessage},
		{name: "null transaction context pointer", failOn: failOnWriteByte, failCount: 3, message: newStartMessage},
		{name: "transaction context length", failOn: failOnWriteBytes, failCount: 3, message: newStartMessage},
		{name: "format ID", failOn: failOnWriteBytes, failCount: 4, message: newStartMessage},
		{name: "global transaction ID length", failOn: failOnWriteBytes, failCount: 5, message: newStartMessage},
		{name: "BQUAL length", failOn: failOnWriteBytes, failCount: 6, message: newStartMessage},
		{name: "XID pointer", failOn: failOnWriteByte, failCount: 4, message: newStartMessage},
		{name: "null XID pointer", failOn: failOnWriteByte, failCount: 4, message: newSuspendMessage},
		{name: "XID length", failOn: failOnWriteBytes, failCount: 7, message: newStartMessage},
		{name: "flags", failOn: failOnWriteBytes, failCount: 8, message: newStartMessage},
		{name: "timeout", failOn: failOnWriteBytes, failCount: 9, message: newStartMessage},
		{name: "application value pointer", failOn: failOnWriteByte, failCount: 5, message: newStartMessage},
		{name: "null application value pointer", failOn: failOnWriteByte, failCount: 5, message: newMessageWithoutApplicationValue},
		{name: "return application value pointer", failOn: failOnWriteByte, failCount: 6, message: newStartMessage},
		{name: "return context pointer", failOn: failOnWriteByte, failCount: 7, message: newStartMessage},
		{name: "null internal name pointer", failOn: failOnWriteByte, failCount: 8, message: newStartMessage},
		{name: "internal name length", failOn: failOnWriteBytes, failCount: 10, message: newStartMessage},
		{name: "null external name pointer", failOn: failOnWriteByte, failCount: 9, message: newStartMessage},
		{name: "external name length", failOn: failOnWriteBytes, failCount: 11, message: newStartMessage},
		{name: "XID", failOn: failOnWriteBytes, failCount: 12, message: newStartMessage},
		{name: "application value", failOn: failOnWriteBytes, failCount: 13, message: newStartMessage},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := test.message()
			err := msg.MarshalTo(context.Background(), createMarshaller(make([]byte, 256), test.failOn, test.failCount))
			assertFailMarshalError(t, err)
		})
	}
}

// TestGenerateSessionlessGlobalTransactionID verifies that the default generated
// global transaction ID uses
// the same 16-byte UUID-shaped layout.
func TestGenerateSessionlessGlobalTransactionID(t *testing.T) {
	t.Parallel()

	globalTransactionID, err := generateGlobalTransactionID()
	if err != nil {
		t.Fatalf("generateGlobalTransactionID failed: %v", err)
	}
	if len(globalTransactionID) != 16 {
		t.Fatalf("generated global transaction ID length = %d, want 16", len(globalTransactionID))
	}

	bytes := []byte(globalTransactionID)
	if version := bytes[6] >> 4; version != 4 {
		t.Fatalf("generated global transaction ID UUID version nibble = %d, want 4", version)
	}
	if variant := bytes[8] >> 6; variant != 2 {
		t.Fatalf("generated global transaction ID UUID variant bits = %d, want 2", variant)
	}
}

// TestValidateSessionlessGlobalTransactionID verifies that client-side validation only rejects
// clearly invalid values before deferring transaction existence checks to the
// server.
func TestValidateSessionlessGlobalTransactionID(t *testing.T) {
	t.Parallel()

	t.Run("accepts non-empty global transaction ID within server size limit", func(t *testing.T) {
		if err := validateSessionlessGlobalTransactionID(extensions.GlobalTransactionID("valid-global-transaction-id")); err != nil {
			t.Fatalf("validateSessionlessGlobalTransactionID returned unexpected error: %v", err)
		}
	})

	t.Run("rejects empty global transaction ID", func(t *testing.T) {
		err := validateSessionlessGlobalTransactionID(extensions.GlobalTransactionID(""))
		if err == nil {
			t.Fatal("validateSessionlessGlobalTransactionID returned nil for empty global transaction ID")
		}
		sqlErr, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("validateSessionlessGlobalTransactionID error type = %T, want common.SQLError", err)
		}
		if sqlErr.ErrorCode() != string(oracleErrors.InvalidGlobalTransactionIDValue) {
			t.Fatalf("validateSessionlessGlobalTransactionID error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.InvalidGlobalTransactionIDValue)
		}
	})

	t.Run("rejects global transaction ID larger than server limit", func(t *testing.T) {
		err := validateSessionlessGlobalTransactionID(extensions.GlobalTransactionID(strings.Repeat("a", maxSessionlessGlobalTransactionIDSize+1)))
		if err == nil {
			t.Fatal("validateSessionlessGlobalTransactionID returned nil for oversized global transaction ID")
		}
		sqlErr, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("validateSessionlessGlobalTransactionID error type = %T, want common.SQLError", err)
		}
		if sqlErr.ErrorCode() != string(oracleErrors.InvalidGlobalTransactionIDValue) {
			t.Fatalf("validateSessionlessGlobalTransactionID error code = %q, want %q", sqlErr.ErrorCode(), oracleErrors.InvalidGlobalTransactionIDValue)
		}
	})
}

// TestNewSessionlessGlobalTransactionIDSync verifies that a SESSIONLESS_GTRID payload is
// decoded correctly and that returned byte slices cannot mutate the value.
func TestNewSessionlessGlobalTransactionIDSync(t *testing.T) {
	t.Parallel()

	sync, err := newSessionlessGlobalTransactionIDSync(driverCommon.B1Array{'a', 'b', sessionlessGlobalTransactionIDSyncSet, 2})
	if err != nil {
		t.Fatalf("NewSessionlessGlobalTransactionIDSync failed: %v", err)
	}
	if !sync.IsSet() {
		t.Fatal("expected decoded sync payload to be set")
	}
	if sync.IsUnset() {
		t.Fatal("did not expect decoded sync payload to be unset")
	}
	globalTransactionID := sync.GlobalTransactionID()
	if !slices.Equal(globalTransactionID, extensions.GlobalTransactionID("ab")) {
		t.Fatalf("GlobalTransactionID = %q, want %q", sync.GlobalTransactionID(), "ab")
	}
	globalTransactionID[0] = 'z'
	if !slices.Equal(sync.GlobalTransactionID(), extensions.GlobalTransactionID("ab")) {
		t.Fatalf("GlobalTransactionID changed through returned slice: %q", sync.GlobalTransactionID())
	}
	if sync.Version() != 2 {
		t.Fatalf("Version = %d, want 2", sync.Version())
	}
	if sync.Reason() != 0 {
		t.Fatalf("Reason = %d, want 0", sync.Reason())
	}
}

// TestOTxSeRPA_UnMarshalFrom_Success verifies the OTXSE RPA unmarshals
// correctly: UB4 application value, UB2 context length, then raw bytes.
func TestOTxSeRPA_UnMarshalFrom_Success(t *testing.T) {
	t.Parallel()

	buf := NewTestDataBuffer()
	mar := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	if err := mar.MarshalUB4(context.Background(), 42); err != nil {
		t.Fatalf("MarshalUB4 failed: %v", err)
	}
	if err := mar.MarshalUB2(context.Background(), 4); err != nil {
		t.Fatalf("MarshalUB2 failed: %v", err)
	}
	if err := mar.MarshalB1Array(context.Background(), []byte{0xDE, 0xAD, 0xBE, 0xEF}); err != nil {
		t.Fatalf("MarshalB1Array failed: %v", err)
	}

	msg := newOTxSeRPA().(*ttiOTxSeRPA)
	if msg.GetMsgCode() != TTIRPA {
		t.Fatalf("GetMsgCode = %v, want %v", msg.GetMsgCode(), TTIRPA)
	}
	if err := msg.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	if got := msg.GetApplicationValue(); got != 42 {
		t.Fatalf("GetApplicationValue = %d, want 42", got)
	}
	if !bytes.Equal(msg.GetContext(), []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Fatalf("GetContext = %v, want %v", msg.GetContext(), []byte{0xDE, 0xAD, 0xBE, 0xEF})
	}
}

// TestOTxSeRPA_UnMarshalFrom_EmptyContext verifies that zero-length contexts
// are accepted and normalized to a nil context slice.
func TestOTxSeRPA_UnMarshalFrom_EmptyContext(t *testing.T) {
	t.Parallel()

	buf := NewTestDataBuffer()
	mar := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	if err := mar.MarshalUB4(context.Background(), 7); err != nil {
		t.Fatalf("MarshalUB4 failed: %v", err)
	}
	if err := mar.MarshalUB2(context.Background(), 0); err != nil {
		t.Fatalf("MarshalUB2 failed: %v", err)
	}

	msg := newOTxSeRPA().(*ttiOTxSeRPA)
	if err := msg.UnMarshalFrom(context.Background(), mar); err != nil {
		t.Fatalf("UnMarshalFrom failed: %v", err)
	}
	if got := msg.GetApplicationValue(); got != 7 {
		t.Fatalf("GetApplicationValue = %d, want 7", got)
	}
	if msg.GetContext() != nil {
		t.Fatalf("GetContext = %v, want nil", msg.GetContext())
	}
}

// TestOTxSeRPA_UnMarshalFrom_Failure verifies TTC read errors are surfaced
// while decoding the OTXSE reply payload.
func TestOTxSeRPA_UnMarshalFrom_Failure(t *testing.T) {
	t.Parallel()

	payload := []byte{
		0x00, 0x00, 0x00, 0x2A,
		0x00, 0x04,
		0xDE, 0xAD,
	}
	mar := createMarshaller(payload, failOnReadByte, 1)

	msg := newOTxSeRPA().(*ttiOTxSeRPA)
	err := msg.UnMarshalFrom(context.Background(), mar)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated read error") {
		t.Fatalf("expected simulated read error, got %v", err)
	}
}

// TestOTxSeRPA_UnMarshalFrom_FieldFailures verifies errors reading each field
// of an OTXSE return-parameter message.
func TestOTxSeRPA_UnMarshalFrom_FieldFailures(t *testing.T) {
	t.Parallel()

	buf, engine := newOTxSeEngine(128)
	if err := engine.MarshalUB4(context.Background(), 42); err != nil {
		t.Fatalf("MarshalUB4 failed: %v", err)
	}
	if err := engine.MarshalUB2(context.Background(), 4); err != nil {
		t.Fatalf("MarshalUB2 failed: %v", err)
	}
	if err := engine.MarshalB1Array(context.Background(), driverCommon.B1Array{0xDE, 0xAD, 0xBE, 0xEF}); err != nil {
		t.Fatalf("MarshalB1Array failed: %v", err)
	}
	payload := append([]byte(nil), buf.bytes[:buf.currentWritePosition]...)

	tests := []struct {
		name      string
		failOn    FailOn
		failCount int
	}{
		{name: "application value", failOn: failOnReadByte, failCount: 1},
		{name: "context length", failOn: failOnReadByte, failCount: 2},
		{name: "context bytes", failOn: failOnReadBytes, failCount: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := newOTxSeRPA().(*ttiOTxSeRPA)
			err := msg.UnMarshalFrom(context.Background(), createMarshaller(payload, test.failOn, test.failCount))
			assertOracleErrorCode(t, err, oracleErrors.FailUnmarshal)
		})
	}
}

// TestOTxSeRPAFactoryRegistration verifies the function registry can resolve
// the OTXSE-specific TTIRPA decoder needed by the message streamer callback.
func TestOTxSeRPAFactoryRegistration(t *testing.T) {
	t.Parallel()

	functionRegistry := NewRegistry[functionRegistryKey]()
	if err := functionRegistry.Register(functionRegistryKey{messageType: TTIRPA, functionType: oTxSe}, 1, newOTxSeRPA); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	factory := &SimpleFactory{
		ttcVersion:   18,
		msgregistry:  NewRegistry[driverCommon.MessageType](),
		funcregistry: functionRegistry,
	}

	msg, err := factory.GetMessageForFunction(TTIRPA, oTxSe)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIRPA, oTxSe) failed: %v", err)
	}
	if _, ok := msg.(*ttiOTxSeRPA); !ok {
		t.Fatalf("GetMessageForFunction returned %T, want *ttiOTxSeRPA", msg)
	}
}
