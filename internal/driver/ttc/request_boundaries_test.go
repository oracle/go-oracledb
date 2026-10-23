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
	"database/sql/driver"
	"errors"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// TestAddRequestBoundariesRegistersAndRunsCallbacks verifies callback registration and emitted flags.
func TestAddRequestBoundariesRegistersAndRunsCallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		sessionStateSupported  bool
		explicitBoundSupported bool
		want                   []driverCommon.UB8
	}{
		{
			name: "session-state operations unsupported",
		},
		{
			name:                   "explicit boundaries supported",
			sessionStateSupported:  true,
			explicitBoundSupported: true,
			want: []driverCommon.UB8{
				driverCommon.UB8(sessionStateExplBound | sessionStateRequestBegin),
				driverCommon.UB8(sessionStateExplBound | sessionStateRequestBegin),
				driverCommon.UB8(sessionStateExplBound | sessionStateRequestEnd),
			},
		},
		{
			name:                  "explicit boundaries unsupported",
			sessionStateSupported: true,
			want: []driverCommon.UB8{
				driverCommon.UB8(sessionStateRequestBegin),
				driverCommon.UB8(sessionStateRequestBegin),
				driverCommon.UB8(sessionStateRequestEnd),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conn, streamer := newSessionStateTestConnection(
				18,
				driverCommon.Capability{IsSet: tt.sessionStateSupported},
				tt.explicitBoundSupported,
			)
			callbacks := newRequestBoundaryCallbacks()
			addBeginEndRequestCallbacks(callbacks, conn)

			if !tt.sessionStateSupported {
				if len(callbacks.newConnectionCallbacks) != 0 || len(callbacks.requestStartCallbacks) != 0 || len(callbacks.requestEndCallbacks) != 0 {
					t.Fatalf("unsupported server registered callbacks: %+v", callbacks)
				}
			} else {
				if len(callbacks.newConnectionCallbacks) != 1 || len(callbacks.requestStartCallbacks) != 1 || len(callbacks.requestEndCallbacks) != 1 {
					t.Fatalf("registered callbacks = %+v, want one callback for each phase", callbacks)
				}
				if err := runRequestBoundaryCallbacks(context.Background(), callbacks.newConnectionCallbacks); err != nil {
					t.Fatalf("run creation callbacks error = %v", err)
				}
				if err := runRequestBoundaryCallbacks(context.Background(), callbacks.requestStartCallbacks); err != nil {
					t.Fatalf("run request-start callbacks error = %v", err)
				}
				if err := runRequestBoundaryCallbacks(context.Background(), callbacks.requestEndCallbacks); err != nil {
					t.Fatalf("run request-end callbacks error = %v", err)
				}
			}

			got := pushedSessionStateFlags(t, streamer)
			if len(got) != len(tt.want) {
				t.Fatalf("pushed flags = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("pushed flags[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestAddRequestBoundariesReturnsBadConnOnPushFailure verifies boundary callbacks discard connections when queuing fails.
func TestAddRequestBoundariesReturnsBadConnOnPushFailure(t *testing.T) {
	t.Parallel()

	factoryErr := errors.New("message factory failed")
	tests := []struct {
		name      string
		callbacks func(*requestCallbacks) []*requestBoundaryCallback
	}{
		{
			name: "connection creation",
			callbacks: func(callbacks *requestCallbacks) []*requestBoundaryCallback {
				return callbacks.newConnectionCallbacks
			},
		},
		{
			name: "request start",
			callbacks: func(callbacks *requestCallbacks) []*requestBoundaryCallback {
				return callbacks.requestStartCallbacks
			},
		},
		{
			name: "request end",
			callbacks: func(callbacks *requestCallbacks) []*requestBoundaryCallback {
				return callbacks.requestEndCallbacks
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			caps := map[string]driverCommon.Capability{
				kpccapRtbTtcSessstateops: {IsSet: true},
			}
			shelf := newShelf[driverCommon.MessageType]()
			shelf.RegisterCapabilities(caps)
			shelf.RegisterMessageFactory(&mockFactory{returnErr: factoryErr})
			shelf.RegisterMessageStreamer(&mockStreamer{})
			conn := newTestConnection(shelf, driverCommon.NewSessionContext(), &mockNetworkSession{})

			callbacks := newRequestBoundaryCallbacks()
			addBeginEndRequestCallbacks(callbacks, conn)
			if err := runRequestBoundaryCallbacks(context.Background(), tt.callbacks(callbacks)); err != driver.ErrBadConn {
				t.Errorf("boundary callback error = %v, want %v", err, driver.ErrBadConn)
			}
		})
	}
}
