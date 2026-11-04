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
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	"golang.org/x/text/language"
)

type mockCookieConnectionAuthenticator struct {
	err                                  error
	streamer                             *MessageStreamer
	authenticateCalled                   bool
	callbacksPresentDuringAuthentication bool
}

func (m *mockCookieConnectionAuthenticator) Authenticate(context.Context) error {
	m.authenticateCalled = true
	m.callbacksPresentDuringAuthentication = m.streamer.preUCallbacks[TTIDTY] != nil &&
		m.streamer.postUCallbacks[TTIDTY] != nil
	return m.err
}

type mockCookieConnectionFactory struct {
	connection *connection
	called     bool
}

func (m *mockCookieConnectionFactory) NewConnection(context.Context, *ttiShelf[driverCommon.MessageType], *driverCommon.SessionContext, driverCommon.NetworkSession) (*connection, error) {
	m.called = true
	return m.connection, nil
}

type cookieInstantiatorTestSetup struct {
	instantiator      *cookieConnectionInstantiator
	negotiator        *mockNegotiator
	authenticator     *mockCookieConnectionAuthenticator
	connectionFactory *mockCookieConnectionFactory
	streamer          *MessageStreamer
}

func newCookieInstantiatorTestSetup() *cookieInstantiatorTestSetup {
	shelf := newShelf[driverCommon.MessageType]()
	streamer := NewMessageStreamer(shelf)
	shelf.RegisterMessageStreamer(streamer)

	negotiator := &mockNegotiator{sessCtx: driverCommon.NewSessionContext(), shelf: shelf}
	authenticator := &mockCookieConnectionAuthenticator{streamer: streamer}
	connectionFactory := &mockCookieConnectionFactory{connection: &connection{}}
	instantiator := &cookieConnectionInstantiator{
		connectionInstantiatorCommon: &connectionInstantiatorCommon{
			negotiator:          negotiator,
			authenticator:       authenticator,
			ns:                  &mockNetworkSession{},
			driverConfig:        oracleconfig.NewOracleDriverConfig(),
			localizationService: common.NewLocalizationService(language.English),
			newConnectionFunc:   connectionFactory.NewConnection,
		},
	}
	return &cookieInstantiatorTestSetup{
		instantiator:      instantiator,
		negotiator:        negotiator,
		authenticator:     authenticator,
		connectionFactory: connectionFactory,
		streamer:          streamer,
	}
}

func registerCookieInstantiatorTestCallbacks(streamer *MessageStreamer) {
	streamer.RegisterPreUnmarshallCallback(TTIDTY, func(*messageHeader) (driverCommon.Message[driverCommon.MessageType], error) {
		return nil, nil
	})
	streamer.RegisterPostUnmarshallCallback(TTIDTY, func(driverCommon.Message[driverCommon.MessageType], error) (bool, error) {
		return true, nil
	})
}

func assertCookieInstantiatorCallbacksCleanedUp(t *testing.T, setup *cookieInstantiatorTestSetup) {
	t.Helper()
	if !setup.authenticator.callbacksPresentDuringAuthentication {
		t.Error("DTY callbacks were unavailable during authentication")
	}
	if setup.streamer.preUCallbacks[TTIDTY] != nil || setup.streamer.postUCallbacks[TTIDTY] != nil {
		t.Error("DTY callbacks remain registered after GetConnection returned")
	}
}

// TestNewTTCCookieConnectionInstantiator verifies cookie negotiator wiring
// and cookie authenticator selection for password and token credentials.
func TestNewTTCCookieConnectionInstantiator(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		useToken bool
	}{
		{name: "password authentication"},
		{name: "token authentication", useToken: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := oracleconfig.NewOracleDriverConfig()
			if !tc.useToken {
				config.Credentials.User = "test-user"
				config.Credentials.Password = "test-password"
			}
			ns := &struct {
				driverCommon.NetworkSession
				driverCommon.DataBuffer
			}{
				NetworkSession: &mockNetworkSession{},
				DataBuffer:     NewArrayDataBuffer(16),
			}
			cookie := &ttiCookie{connectionProtocolVersion: 6}
			registry := newTestProviderRegistry(mockTokenAuthenticationProvider{})

			instantiator, err := NewTTCCookieConnectionInstantiator(config, ns, registry, cookie)
			if err != nil {
				t.Fatalf("create cookie connection instantiator: %v", err)
			}
			setup := instantiator.connectionInstantiatorCommon
			negotiator, ok := setup.negotiator.(*cookieConnectionNegotiator)
			if !ok {
				t.Fatalf("negotiator type = %T, want *cookieConnectionNegotiator", setup.negotiator)
			}
			if negotiator.cookie != cookie {
				t.Error("negotiator did not retain the supplied cookie")
			}
			if tc.useToken {
				if _, ok := setup.authenticator.(*cookieTokenAuthenticator); !ok {
					t.Fatalf("authenticator type = %T, want *cookieTokenAuthenticator", setup.authenticator)
				}
			} else if _, ok := setup.authenticator.(*cookiePasswordAuthenticator); !ok {
				t.Fatalf("authenticator type = %T, want *cookiePasswordAuthenticator", setup.authenticator)
			}
		})
	}
}

// TestCookieConnectionInstantiatorGetConnection verifies success, early exit on
// negotiation failure, and callback cleanup after successful or failed authentication.
func TestCookieConnectionInstantiatorGetConnection(t *testing.T) {
	t.Parallel()

	t.Run("successful connection", func(t *testing.T) {
		setup := newCookieInstantiatorTestSetup()
		registerCookieInstantiatorTestCallbacks(setup.streamer)

		conn, err := setup.instantiator.GetConnection(context.Background())

		if err != nil {
			t.Fatalf("GetConnection failed: %v", err)
		}
		if !setup.authenticator.authenticateCalled {
			t.Error("authentication was not called")
		}
		if !setup.connectionFactory.called || conn != setup.connectionFactory.connection {
			t.Error("GetConnection did not return the created connection")
		}
		assertCookieInstantiatorCallbacksCleanedUp(t, setup)
	})

	t.Run("negotiation error", func(t *testing.T) {
		setup := newCookieInstantiatorTestSetup()
		wantErr := errors.New("negotiation failed")
		setup.negotiator.err = wantErr

		conn, err := setup.instantiator.GetConnection(context.Background())

		if !errors.Is(err, wantErr) {
			t.Fatalf("GetConnection error = %v, want %v", err, wantErr)
		}
		if conn != nil {
			t.Error("GetConnection returned a connection after negotiation failed")
		}
		if setup.authenticator.authenticateCalled {
			t.Error("authentication started after negotiation failed")
		}
		if setup.connectionFactory.called {
			t.Error("connection was created after negotiation failed")
		}
	})

	t.Run("authentication error", func(t *testing.T) {
		setup := newCookieInstantiatorTestSetup()
		registerCookieInstantiatorTestCallbacks(setup.streamer)
		wantErr := errors.New("authentication failed")
		setup.authenticator.err = wantErr

		conn, err := setup.instantiator.GetConnection(context.Background())

		if !errors.Is(err, wantErr) {
			t.Fatalf("GetConnection error = %v, want %v", err, wantErr)
		}
		if !setup.authenticator.authenticateCalled {
			t.Error("authentication was not called")
		}
		if conn != nil || setup.connectionFactory.called {
			t.Error("connection was created or returned after authentication failed")
		}
		assertCookieInstantiatorCallbacksCleanedUp(t, setup)
	})
}
