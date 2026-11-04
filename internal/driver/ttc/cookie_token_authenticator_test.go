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
	"errors"
	"io"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func newCookieTokenAuthenticatorForTest(streamer *mockStreamer) (*cookieTokenAuthenticator, *driverCommon.SessionContext) {
	shelf, _, _ := newAuthTestShelf(8192)
	shelf.RegisterMessageStreamer(streamer)
	authenticator := newCookieTokenAuthenticator(mockTokenAuthenticationProvider{token: "test-token"})
	sessionContext := driverCommon.NewSessionContext()
	authenticator.SetShelf(shelf)
	authenticator.SetSessionContext(sessionContext)
	return authenticator, sessionContext
}

func assertCookieTokenOAuthRequest(t *testing.T, streamer *mockStreamer) {
	t.Helper()
	if streamer.pushedMsg.Len() != 1 {
		t.Fatalf("sent %d requests, want one OAUTH request", streamer.pushedMsg.Len())
	}
	msg := *streamer.pushedMsg.Front().Value.(*driverCommon.Message[driverCommon.MessageType])
	if _, ok := msg.(*oAuth); !ok {
		t.Fatalf("sent message %T, want OAUTH", msg)
	}
}

// TestCookieTokenAuthenticator_Success verifies the combined PRO, DTY, and
// token OAUTH response flow and that the OAUTH response updates the session.
func TestCookieTokenAuthenticator_Success(t *testing.T) {
	t.Parallel()
	oauthRPA := &OAuthRPA{connectionValues: driverCommon.NewProperties[string]()}
	oauthRPA.connectionValues.SetProperty(authDbMountID, driverCommon.StringToB1Array("123"))
	streamer := &mockStreamer{
		pullMsgs: []driverCommon.Message[driverCommon.MessageType]{
			newTTIpro(), NewTTIdty(), oauthRPA, &mockOer{},
		},
		pullErr: io.EOF,
	}
	authenticator, sessionContext := newCookieTokenAuthenticatorForTest(streamer)

	if err := authenticator.Authenticate(context.Background()); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	assertCookieTokenOAuthRequest(t, streamer)
	if len(streamer.pullMsgs) != 0 {
		t.Error("authentication returned before consuming all responses")
	}
	mountID, ok := sessionContext.GetSessionProperties().GetProperty(authDbMountID).(driverCommon.B1Array)
	if !ok || driverCommon.B1ArrayToString(mountID) != "123" {
		t.Error("OAUTH response was not applied to the session")
	}
}

// TestCookieTokenAuthenticator_TTIRENEG verifies that cookie rejection returns
// the fallback signal to the caller without sending another OAUTH request.
func TestCookieTokenAuthenticator_TTIRENEG(t *testing.T) {
	t.Parallel()
	streamer := &mockStreamer{pullMsg: &tTIReneg{}}
	authenticator, _ := newCookieTokenAuthenticatorForTest(streamer)

	err := authenticator.Authenticate(context.Background())

	if !errors.Is(err, driverCommon.ErrTTIRENEG) {
		t.Fatalf("got error %v, want ErrTTIRENEG", err)
	}
	assertCookieTokenOAuthRequest(t, streamer)
}

// TestCookieTokenAuthenticator_ServerError verifies that a token OAUTH server
// rejection is returned to the caller without sending another request.
func TestCookieTokenAuthenticator_ServerError(t *testing.T) {
	t.Parallel()
	serverErr := errors.New("token OAUTH rejected")
	streamer := &mockStreamer{pullMsg: &mockOer{err: serverErr}}
	authenticator, _ := newCookieTokenAuthenticatorForTest(streamer)

	err := authenticator.Authenticate(context.Background())

	if !errors.Is(err, serverErr) {
		t.Fatalf("got error %v, want server rejection %v", err, serverErr)
	}
	assertCookieTokenOAuthRequest(t, streamer)
}
