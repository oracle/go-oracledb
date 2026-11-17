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
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
)

// TestAuthenticationFactoryWithNilParameters verifies nil configuration is rejected.
func TestAuthenticationFactoryWithNilParameters(t *testing.T) {
	t.Parallel()

	auth, err := GetAuthenticator(nil, nil)
	if err == nil {
		t.Fatalf("get authenticator test failed %v", err)
	}
	if auth != nil {
		t.Fatalf("Authenticator should be null")
	}
}

// TestAuthenticationFactoryBasic verifies credentials create an authenticator.
func TestAuthenticationFactoryBasic(t *testing.T) {
	t.Parallel()
	c := oracleconfig.NewOracleDriverConfig()
	c.Credentials.User = "foo"
	c.Credentials.Password = "bar"
	c.ConnectDescriptor = "connection string"
	auth, err := GetAuthenticator(c, nil)
	if err != nil {
		t.Fatalf("get authenticator test failed %v", err)
	}
	if auth == nil {
		t.Errorf("should have returned an authenticator")
	}
}

// TestGetConnection verifies connection creation, error propagation, and user properties.
func TestGetConnection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ns := &mockNetworkSession{
		cancelErr: nil,
	}
	mockFac := &mockFactory{
		returnMsg: NewOall18(),
	}
	mockStr := &mockStreamer{
		pullMsg: &mockOer{err: nil},
	}

	tests := []struct {
		name          string
		negotiatorErr error
		authErr       error
		expectErr     bool
	}{
		{
			name:      "successful connection",
			expectErr: false,
		},
		{
			name:          "negotiation error",
			negotiatorErr: errors.New("negotiation failed"),
			expectErr:     true,
		},
		{
			name:      "authentication error",
			authErr:   errors.New("auth failed"),
			expectErr: true,
		},
		{
			name:      "with shelf user",
			expectErr: false,
		},
		{
			name:      "with session context user",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shelf := newShelf[driverCommon.MessageType]()
			shelf.RegisterMessageStreamer(mockStr)
			shelf.RegisterMessageFactory(mockFac)
			oracleconfig := &oracleconfig.OracleDriverConfig{
				DriverProperties: oracleconfig.OracleDriverProperties{},
			}
			ci := &connectionInstantiator{
				negotiator: &mockNegotiator{
					sessCtx: driverCommon.NewSessionContext(),
					shelf:   shelf,
					err:     tt.negotiatorErr,
				},
				authenticator:       &mockAuthenticator{err: tt.authErr},
				ns:                  ns,
				driverConfig:        oracleconfig,
				localizationService: common.NewLocalizationService(language.English),
				newConnectionFunc: func(_ context.Context, shelf *ttiShelf[driverCommon.MessageType], sessCtx *driverCommon.SessionContext, ns driverCommon.NetworkSession) (*connection, error) {
					return newTestConnection(shelf, sessCtx, ns), nil
				},
			}

			conn, err := ci.GetConnection(ctx)

			if tt.expectErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if tt.authErr != nil {
					if err.Error() != tt.authErr.Error() {
						t.Errorf("Wrong error, expected %v, but was %v", tt.authErr.Error(), err.Error())
					}
				}
				if tt.negotiatorErr != nil {
					if err.Error() != tt.negotiatorErr.Error() {
						t.Errorf("Wrong error, expected %v, but was %v", tt.negotiatorErr.Error(), err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if conn == nil {
					t.Error("expected a connection, got nil")
				}

				_, ok := conn.(*connection)
				if !ok {
					t.Errorf("Wrong connection type")
				}
			}
		})
	}
}

// TestGetConnectionQueuesInitialRequestBegin verifies a fresh connection queues
// an initial begin-request boundary when the server supports it.
func TestGetConnectionQueuesInitialRequestBegin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		sessionStateSupported  bool
		explicitBoundSupported bool
		want                   []driverCommon.UB8
	}{
		{
			name:                   "session-state operations and explicit boundaries supported",
			sessionStateSupported:  true,
			explicitBoundSupported: true,
			want: []driverCommon.UB8{
				driverCommon.UB8(sessionStateExplBound | sessionStateRequestBegin),
			},
		},
		{
			name: "session-state operations unsupported",
		},
		{
			name:                  "explicit boundaries unsupported",
			sessionStateSupported: true,
			want: []driverCommon.UB8{
				driverCommon.UB8(sessionStateRequestBegin),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			caps := make(map[string]driverCommon.Capability)
			if tt.sessionStateSupported {
				caps[kpccapRtbTtcSessstateops] = driverCommon.Capability{IsSet: true}
			}
			if tt.explicitBoundSupported {
				caps[kpccapCtbTtc4ExplBound] = driverCommon.Capability{IsSet: true}
			}
			shelf := newShelf[driverCommon.MessageType]()
			shelf.RegisterCapabilities(caps)
			streamer := &mockStreamer{}
			shelf.RegisterMessageStreamer(streamer)
			shelf.RegisterMessageFactory(NewMessageFactoryForProtocol(18, caps))

			instantiator := &connectionInstantiator{
				negotiator: &mockNegotiator{
					sessCtx: driverCommon.NewSessionContext(),
					shelf:   shelf,
				},
				authenticator:       &mockAuthenticator{},
				ns:                  &mockNetworkSession{},
				driverConfig:        &oracleconfig.OracleDriverConfig{},
				localizationService: common.NewLocalizationService(language.English),
				newConnectionFunc: func(_ context.Context, shelf *ttiShelf[driverCommon.MessageType], sessCtx *driverCommon.SessionContext, ns driverCommon.NetworkSession) (*connection, error) {
					return newTestConnection(shelf, sessCtx, ns), nil
				},
			}

			conn, err := instantiator.GetConnection(context.Background())
			if err != nil {
				t.Fatalf("GetConnection() error = %v", err)
			}
			if _, ok := conn.(*connection); !ok {
				t.Fatalf("connection type = %T, want *connection", conn)
			}
			got := pushedSessionStateFlags(t, streamer)
			if len(got) != len(tt.want) {
				t.Fatalf("pushed flags = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("pushed flags = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestGetConnectionMissingLocalizationService verifies connection creation fails with the expected error when localization is unavailable.
func TestGetConnectionMissingLocalizationService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shelf := newShelf[driverCommon.MessageType]()
	shelf.RegisterMessageStreamer(&mockStreamer{pullMsg: &mockOer{err: nil}})
	shelf.RegisterMessageFactory(&mockFactory{returnMsg: NewOall18()})

	oracleconfig := &oracleconfig.OracleDriverConfig{
		DriverProperties: oracleconfig.OracleDriverProperties{},
	}

	ci := &connectionInstantiator{
		negotiator: &mockNegotiator{
			sessCtx: driverCommon.NewSessionContext(),
			shelf:   shelf,
		},
		authenticator: &mockAuthenticator{},
		ns:            &mockNetworkSession{},
		newConnectionFunc: func(_ context.Context, shelf *ttiShelf[driverCommon.MessageType], sessCtx *driverCommon.SessionContext, ns driverCommon.NetworkSession) (*connection, error) {
			return newTestConnection(shelf, sessCtx, ns), nil
		},
		driverConfig:        oracleconfig,
		localizationService: nil,
	}

	conn, err := ci.GetConnection(ctx)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if conn != nil {
		t.Fatalf("expected no connection, got %#v", conn)
	}
	sqle, ok := err.(oracleErrors.SQLError)
	if !ok {
		t.Fatalf("expected SQLError, got %T", err)
	}
	if sqle.ErrorCode() != string(oracleErrors.MissingLocalizationService) {
		t.Fatalf("expected error code %s, got %s", oracleErrors.MissingLocalizationService, sqle.ErrorCode())
	}
	expected := "OGD-00067 - missing localization service on shelf"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}
