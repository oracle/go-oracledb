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
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

// newAuthTestShelf wires a Shelf with:
// - marshaller over an in-memory buffer
// - Simple factory providing TTIFUN implementors for oSesskey and oauth
// - Message streamer registered on the shelf
func newAuthTestShelf(bufSize int) (*ttiShelf[driverCommon.MessageType], *MessageStreamer, *ArrayBasedDataBuffer) {
	buf := NewArrayDataBuffer(bufSize)
	mar := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})

	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMarshaller(mar)

	// Function registry for TTIFUN implementors used by the authenticator.
	funcReg := NewRegistry[functionRegistryKey]()
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oSesskey}, -1, func() driverCommon.Message[driverCommon.MessageType] {
		return NewOSesskey()
	})
	// Use NewOAuth18 to mirror production behaviour where the key/value list is pre-initialized.
	_ = funcReg.Register(functionRegistryKey{messageType: TTIFUN, functionType: oauth}, -1, func() driverCommon.Message[driverCommon.MessageType] {
		return NewOAuth18()
	})
	// RPA responses used by the authenticator Pull paths
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oSesskey}, -1, func() driverCommon.Message[driverCommon.MessageType] {
		return NewOSesskeyRPA()
	})
	_ = funcReg.Register(functionRegistryKey{messageType: TTIRPA, functionType: oauth}, -1, func() driverCommon.Message[driverCommon.MessageType] {
		return NewOAuthRPA()
	})
	_ = funcReg.Register(functionRegistryKey{messageType: TTISPF, functionType: driverCommon.FunctionType(ocssync)}, -1, newttiSPFOCSSync)

	msgReg := NewRegistry[driverCommon.MessageType]()
	_ = msgReg.Register(TTIOER, 14, newTTIoer14)
	_ = msgReg.Register(TTIWRN, -1, newTTIwrn)

	// Simple factory over our function registry.
	factory := &SimpleFactory{
		ttcVersion:   -1,
		msgregistry:  msgReg,
		funcregistry: funcReg,
	}
	shelf.RegisterMessageFactory(factory)

	// Streamer bound to this shelf.
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)

	return shelf, streamer, buf
}

// TestPasswordAuthenticator_doOSESSKEY_Golden
// Seeds the incoming wire with a TTIRPA + oSesskeyRPA golden payload,
// then runs _doOSESSKEY to assert unmarshalling/flow works end-to-end.
func TestPasswordAuthenticator_doOSESSKEY_Golden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf, _, buf := newAuthTestShelf(1 << 16)
	var logOutput bytes.Buffer
	previousLogger := common.Odl
	common.Odl = slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelWarn}))
	defer func() { common.Odl = previousLogger }()

	// Pre-seed incoming wire with: [TTIWRN][TTIRPA][golden oSesskeyRPA payload]
	writeAuthWarning(t, ctx, buf, 28002, "ORA-28002: the password will expire within 7 days", 0x05)
	if err := buf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := buf.WriteBytesWithContext(ctx, makeOSesskeyRPAPayload()); err != nil {
		t.Fatalf("write oSesskeyRPA payload failed: %v", err)
	}

	// Initialize authenticator with requested capability = 239
	pa := newPasswordAuthenticator("username_test", "username_test",
		"(DESCRIPTION=(ADDRESS=(HOST=localhost)(PORT=1521)(PROTOCOL=tcp))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")
	pa.SetShelf(shelf)
	pa.SetSessionContext(driverCommon.NewSessionContext())

	err := pa._doOSESSKEY(ctx)
	if err != nil {
		t.Fatalf("_doOSESSKEY returned error: %v", err)
	}

	// Basic validations derived from the golden dump

	sessionProperties := pa._sessionContext.GetSessionProperties()
	if v, _ := strconv.Atoi(string(sessionProperties.GetProperty(authPbkdf2VgenCount).(*driverCommon.KeyValue).Value)); v != 4096 {
		t.Errorf("PBKDF2VgenCount = %d, want 4096", v)
	}
	if sder, _ := strconv.Atoi(driverCommon.B1ArrayToString(sessionProperties.GetProperty(authPbkdf2SderCount).(*driverCommon.KeyValue).Value)); sder != 3 {
		t.Errorf("PBKDF2SderCount = %d, want 3", sder)
	}
	if len(sessionProperties.GetProperty(authSesskey).(*driverCommon.KeyValue).Value) == 0 {
		t.Error("encrypted session key is empty")
	}
	if len(sessionProperties.GetProperty("AUTH_VFR_DATA").(*driverCommon.KeyValue).Value) == 0 {
		t.Error("salt is empty")
	}
	if output := logOutput.String(); !strings.Contains(output, "warningNumber=28002") ||
		!strings.Contains(output, "ORA-28002: the password will expire within 7 days") {
		t.Errorf("authentication warning was not logged: %s", output)
	}
}

// TestPasswordAuthenticator_doOAuth_Golden
// 1) Build an oSesskeyRPA by unmarshalling the golden dump.
// 2) Seed the incoming wire with TTIRPA + golden oAuth RPA payload.
// 3) Run _doOAuth to assert marshalling/unmarshalling works end-to-end.
func TestPasswordAuthenticator_doOAuth_Golden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Build oSesskeyRPA input for _doOAuth from the golden payload.
	osrpaMsg := NewOSesskeyRPA()
	rpaBuf := NewArrayDataBuffer(8192)
	if err := rpaBuf.WriteBytesWithContext(ctx, makeOSesskeyRPAPayload()); err != nil {
		t.Fatalf("write oSesskeyRPA payload to temp buffer failed: %v", err)
	}
	rpaEngine := NewMarshalEngine(rpaBuf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	unmarshallable, _ := osrpaMsg.(driverCommon.UnMarshallable)
	if err := unmarshallable.UnMarshalFrom(ctx, rpaEngine); err != nil {
		t.Fatalf("UnMarshalFrom(oSesskeyRPA) failed: %v", err)
	}

	// Wire up authenticator shelf for oAuth.
	shelf, _, buf := newAuthTestShelf(1 << 16)

	// Pre-seed incoming wire with: [TTIWRN][TTIRPA][golden oAuth RPA payload]
	writeAuthWarning(t, ctx, buf, 28002, "ORA-28002: the password will expire within 7 days", 0x05)
	if err := buf.WriteByteWithContext(ctx, byte(TTIRPA)); err != nil {
		t.Fatalf("write TTIRPA header failed: %v", err)
	}
	if err := buf.WriteBytesWithContext(ctx, makeOauthRPAPayload()); err != nil {
		t.Fatalf("write oAuth RPA payload failed: %v", err)
	}

	pa := newPasswordAuthenticator(
		"username_test",
		"username_test",
		"(DESCRIPTION=(ADDRESS=(HOST=localhost)(PORT=1521)(PROTOCOL=tcp))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")
	pa.SetShelf(shelf)
	pa.SetSessionContext(driverCommon.NewSessionContext())
	pa._sessionContext.UpdateSessionProperties(osrpaMsg.(*oSesskeyRPA).connectionValues)
	if err := pa._doOAuth(ctx); err != nil {
		sqlError, ok := err.(oracleErrors.SQLError)
		if !ok {
			t.Fatalf("expected error to be SQLError")
		}
		if sqlError.ErrorCode() != string(oracleErrors.AuthenticatorError) {
			t.Fatalf("Expected error %s, but got %s", oracleErrors.AuthenticatorError, sqlError.ErrorCode())
		}
	}
}

func writeAuthWarning(t *testing.T, ctx context.Context, buf *ArrayBasedDataBuffer, number driverCommon.UB2, message string, flags driverCommon.UB2) {
	t.Helper()

	if err := buf.WriteByteWithContext(ctx, byte(TTIWRN)); err != nil {
		t.Fatalf("write TTIWRN header failed: %v", err)
	}
	mar := NewMarshalEngine(buf, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	if err := mar.MarshalUB2(ctx, number); err != nil {
		t.Fatalf("write warning number failed: %v", err)
	}
	if err := mar.MarshalUB2(ctx, driverCommon.UB2(len(message))); err != nil {
		t.Fatalf("write warning length failed: %v", err)
	}
	if err := mar.MarshalUB2(ctx, flags); err != nil {
		t.Fatalf("write warning flags failed: %v", err)
	}
	if err := buf.WriteBytesWithContext(ctx, []byte(message)); err != nil {
		t.Fatalf("write warning message failed: %v", err)
	}
}
