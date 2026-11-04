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
	"encoding/hex"
	"errors"
	"io"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// mockCookiePasswordStreamer supplies a matching server proof once the client
// has prepared OAUTH with its generated session key. This tests flow, not crypto.
type mockCookiePasswordStreamer struct {
	*mockStreamer
	oauthRPA *OAuthRPA
}

func (s *mockCookiePasswordStreamer) Push(ctx context.Context, msg driverCommon.Message[driverCommon.MessageType]) error {
	if oauthMsg, ok := msg.(*oAuth); ok {
		helper := oauthMsg.getLogonHelper()
		plaintext := append(make([]byte, 16), []byte(validationString)...)
		proof, err := helper._encryptAESWithO5logonKey(plaintext, helper.pwdEncStr)
		if err != nil {
			return err
		}
		s.oauthRPA.connectionValues.SetProperty(authServerResponse, driverCommon.StringToB1Array(hex.EncodeToString(proof)))
	}
	return s.mockStreamer.Push(ctx, msg)
}

func newCookiePasswordAuthenticatorForTest(streamer MessageStreamerInterface) (*cookiePasswordAuthenticator, *driverCommon.SessionContext) {
	shelf, _, _ := newAuthTestShelf(8192)
	shelf.RegisterMessageStreamer(streamer)
	authenticator := newCookiePasswordAuthenticator("username_test", "username_test",
		"(DESCRIPTION=(ADDRESS=(HOST=localhost)(PORT=1521)(PROTOCOL=tcp))(CONNECT_DATA=(SERVICE_NAME=freepdb1)))")
	sessionContext := driverCommon.NewSessionContext()
	authenticator.SetShelf(shelf)
	authenticator.SetSessionContext(sessionContext)
	return authenticator, sessionContext
}

func assertCookiePasswordRequests(t *testing.T, streamer *mockStreamer, want ...driverCommon.FunctionType) {
	t.Helper()
	if streamer.pushedMsg.Len() != len(want) {
		t.Fatalf("sent %d requests, want %d", streamer.pushedMsg.Len(), len(want))
	}
	request := streamer.pushedMsg.Front()
	for _, function := range want {
		msg := *request.Value.(*driverCommon.Message[driverCommon.MessageType])
		if got := msg.(driverCommon.Function).GetFuncCode(); got != function {
			t.Errorf("sent function %v, want %v", got, function)
		}
		request = request.Next()
	}
}

// TestCookiePasswordAuthenticator_Success verifies the combined PRO, DTY, and
// OSESSKEY response flow, followed by OAUTH and session updates.
func TestCookiePasswordAuthenticator_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Reuse the existing server OSESSKEY fixture for the password challenge.
	buffer := NewArrayDataBuffer(8192)
	// prepare osesskeyRPA
	if err := buffer.WriteBytesWithContext(ctx, makeOSesskeyRPAPayload()); err != nil {
		t.Fatalf("write OSESSKEY fixture: %v", err)
	}
	marshaller := NewMarshalEngine(buffer, driverCommon.BIG_ENDIAN, [5]byte{Native, Universal, Universal, Universal, Universal})
	osesskeyRPA := NewOSesskeyRPA().(*oSesskeyRPA)
	if err := osesskeyRPA.UnmarshalFrom(ctx, marshaller); err != nil {
		t.Fatalf("decode OSESSKEY fixture: %v", err)
	}
	// prepare oauthRpa
	oauthRPA := &OAuthRPA{connectionValues: driverCommon.NewProperties[string]()}
	oauthRPA.connectionValues.SetProperty(authDbMountID, driverCommon.StringToB1Array("123"))
	//prepare stremer with RPAs needed
	streamer := &mockCookiePasswordStreamer{
		mockStreamer: &mockStreamer{
			pullMsgs: []driverCommon.Message[driverCommon.MessageType]{
				newTTIpro(), NewTTIdty(), osesskeyRPA, &mockOer{}, oauthRPA, &mockOer{},
			},
			pullErr: io.EOF,
		},
		oauthRPA: oauthRPA,
	}
	authenticator, sessionContext := newCookiePasswordAuthenticatorForTest(streamer)

	if err := authenticator.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	assertCookiePasswordRequests(t, streamer.mockStreamer, oSesskey, oauth)
	if len(streamer.pullMsgs) != 0 {
		t.Error("authentication returned before consuming all responses")
	}
	// check if we updated the session with teh same osesskeyRPA connection values
	properties := sessionContext.GetSessionProperties()
	if properties.GetProperty(authSesskey) != osesskeyRPA.connectionValues.GetProperty(authSesskey) {
		t.Error("OSESSKEY response was not applied to the session")
	}
	mountID, ok := properties.GetProperty(authDbMountID).(driverCommon.B1Array)
	if !ok || driverCommon.B1ArrayToString(mountID) != "123" {
		t.Error("OAUTH response was not applied to the session")
	}
}

// TestCookiePasswordAuthenticator_TTIRENEG verifies that cookie rejection
// returns ErrTTIRENEG without proceeding to OAUTH.
func TestCookiePasswordAuthenticator_TTIRENEG(t *testing.T) {
	t.Parallel()
	streamer := &mockStreamer{pullMsg: &tTIReneg{}}
	authenticator, _ := newCookiePasswordAuthenticatorForTest(streamer)

	err := authenticator.Authenticate(context.Background())

	if !errors.Is(err, driverCommon.ErrTTIRENEG) {
		t.Fatalf("got error %v, want ErrTTIRENEG", err)
	}
	assertCookiePasswordRequests(t, streamer, oSesskey)
}

// TestCookiePasswordAuthenticator_ServerError verifies that an OSESSKEY server
// error is returned without proceeding to password OAUTH.
func TestCookiePasswordAuthenticator_ServerError(t *testing.T) {
	t.Parallel()
	serverErr := errors.New("OSESSKEY rejected")
	streamer := &mockStreamer{pullMsg: &mockOer{err: serverErr}}
	authenticator, _ := newCookiePasswordAuthenticatorForTest(streamer)

	err := authenticator.Authenticate(context.Background())

	if !errors.Is(err, serverErr) {
		t.Fatalf("got error %v, want server rejection %v", err, serverErr)
	}
	assertCookiePasswordRequests(t, streamer, oSesskey)
}
