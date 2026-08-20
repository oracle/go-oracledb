/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package ttc

import (
	"context"
	"strings"
	"testing"

	"github.com/oracle/go-driver/driver/common"
	"github.com/oracle/go-driver/driver/network/session"
)

func newOTxSeEngine(capacity int) (*ArrayBasedDataBuffer, *MarshalEngine) {
	buf := NewArrayDataBuffer(capacity)
	engine := NewMarshalEngine(buf, session.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
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
	factory := &SimpleFactory{ttcVersion: 18, msgregistry: NewRegistry[common.MessageType](), funcregistry: functionRegistry}

	msg, err := factory.GetMessageForFunction(TTIFUN, oTxSe)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIFUN, oTxSe) failed: %v", err)
	}
	if msg.GetMsgCode() != TTIFUN {
		t.Fatalf("TTIFUN registration returned msg code %v, want %v", msg.GetMsgCode(), TTIFUN)
	}
	if got := msg.(interface{ GetFuncCode() common.FunctionType }).GetFuncCode(); got != oTxSe {
		t.Fatalf("TTIFUN registration returned func code %v, want %v", got, oTxSe)
	}

	pfnMsg, err := factory.GetMessageForFunction(TTIPFN, oTxSe)
	if err != nil {
		t.Fatalf("GetMessageForFunction(TTIPFN, oTxSe) failed: %v", err)
	}
	if pfnMsg.GetMsgCode() != TTIPFN {
		t.Fatalf("TTIPFN registration returned msg code %v, want %v", pfnMsg.GetMsgCode(), TTIPFN)
	}
	if got := pfnMsg.(interface{ GetFuncCode() common.FunctionType }).GetFuncCode(); got != oTxSe {
		t.Fatalf("TTIPFN registration returned func code %v, want %v", got, oTxSe)
	}
}

// TestOTxSe_MarshalTo_StartSessionless verifies that a sessionless start
// request marshals without error and includes the expected function header and
// trailing variable-length client name payload.
func TestOTxSe_MarshalTo_StartSessionless(t *testing.T) {
	t.Parallel()

	msg := newOTxSe18().(*tTIOtxse)
	msg.setOperation(otxseStart)
	msg.setXID(common.B1Array{0x11, 0x22, 0x33, 0x44}, 2, 2)
	msg.setFlags(otxseTransSessionless | otxseTransNew)
	msg.setTimeout(30)
	msg.setApplicationValue(7)
	msg.setInternalName(common.B1Array("go-driver"))
	msg.setExternalName(common.B1Array("sessionless"))

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
	if got[len(got)-1] != byte('s') {
		t.Fatalf("expected external name payload to be marshalled at end, got last byte %d", got[len(got)-1])
	}
}

// TestOTxSe_MarshalTo_DetachWithTransactionContext verifies that a detach
// request using a non-sessionless format id includes the supplied transaction
// context bytes in the marshalled payload.
func TestOTxSe_MarshalTo_DetachWithTransactionContext(t *testing.T) {
	t.Parallel()

	msg := newOTxSe().(*tTIOtxse)
	msg.setOperation(otxseDetach)
	msg.setFormatID(0x1234)
	msg.setTransactionContext(common.B1Array{0xAA, 0xBB, 0xCC})

	buf, engine := newOTxSeEngine(256)
	if err := msg.MarshalTo(context.Background(), engine); err != nil {
		t.Fatalf("MarshalTo failed: %v", err)
	}

	got := buf.bytes[:buf.currentWritePosition]
	if len(got) == 0 {
		t.Fatal("expected non-empty OTXSE detach payload")
	}
	foundCtx := false
	for i := 0; i+2 < len(got); i++ {
		if got[i] == 0xAA && got[i+1] == 0xBB && got[i+2] == 0xCC {
			foundCtx = true
			break
		}
	}
	if !foundCtx {
		t.Fatal("detach payload did not marshal the transaction context bytes")
	}
}

// TestGenerateSessionlessGTRID verifies that the default generated GTRID uses
// the same 16-byte UUID-shaped layout as the JDBC driver.
func TestGenerateSessionlessGTRID(t *testing.T) {
	t.Parallel()

	gtrid, err := generateSessionlessGTRID()
	if err != nil {
		t.Fatalf("generateSessionlessGTRID failed: %v", err)
	}
	if len(gtrid) != 16 {
		t.Fatalf("generated GTRID length = %d, want 16", len(gtrid))
	}

	bytes := []byte(gtrid)
	if version := bytes[6] >> 4; version != 4 {
		t.Fatalf("generated GTRID UUID version nibble = %d, want 4", version)
	}
	if variant := bytes[8] >> 6; variant != 2 {
		t.Fatalf("generated GTRID UUID variant bits = %d, want 2", variant)
	}
}

// TestValidateSessionlessGTRID verifies that client-side validation only rejects
// clearly invalid values before deferring transaction existence checks to the
// server.
func TestValidateSessionlessGTRID(t *testing.T) {
	t.Parallel()

	t.Run("accepts non-empty gtrid within server size limit", func(t *testing.T) {
		if err := validateSessionlessGTRID("valid-gtrid"); err != nil {
			t.Fatalf("validateSessionlessGTRID returned unexpected error: %v", err)
		}
	})

	t.Run("rejects empty gtrid", func(t *testing.T) {
		err := validateSessionlessGTRID("")
		if err == nil {
			t.Fatal("validateSessionlessGTRID returned nil for empty gtrid")
		}
		sqlErr, ok := err.(common.SQLError)
		if !ok {
			t.Fatalf("validateSessionlessGTRID error type = %T, want common.SQLError", err)
		}
		if sqlErr.ErrorCode() != string(common.InvalidGTRIDValue) {
			t.Fatalf("validateSessionlessGTRID error code = %q, want %q", sqlErr.ErrorCode(), common.InvalidGTRIDValue)
		}
	})

	t.Run("rejects gtrid larger than server limit", func(t *testing.T) {
		err := validateSessionlessGTRID(strings.Repeat("a", maxSessionlessGTRIDSize+1))
		if err == nil {
			t.Fatal("validateSessionlessGTRID returned nil for oversized gtrid")
		}
		sqlErr, ok := err.(common.SQLError)
		if !ok {
			t.Fatalf("validateSessionlessGTRID error type = %T, want common.SQLError", err)
		}
		if sqlErr.ErrorCode() != string(common.InvalidGTRIDValue) {
			t.Fatalf("validateSessionlessGTRID error code = %q, want %q", sqlErr.ErrorCode(), common.InvalidGTRIDValue)
		}
	})
}

func TestNewSessionlessGTRIDSync(t *testing.T) {
	t.Parallel()

	sync, err := NewSessionlessGTRIDSync(common.B1Array{'a', 'b', sessionlessGTRIDSyncSet, 2})
	if err != nil {
		t.Fatalf("NewSessionlessGTRIDSync failed: %v", err)
	}
	if !sync.IsSet() {
		t.Fatal("expected decoded sync payload to be set")
	}
	if sync.IsUnset() {
		t.Fatal("did not expect decoded sync payload to be unset")
	}
	if sync.GlobalTransactionID() != "ab" {
		t.Fatalf("GlobalTransactionID = %q, want %q", sync.GlobalTransactionID(), "ab")
	}
	if sync.Version() != 2 {
		t.Fatalf("Version = %d, want 2", sync.Version())
	}
	if sync.Reason() != 0 {
		t.Fatalf("Reason = %d, want 0", sync.Reason())
	}
}
