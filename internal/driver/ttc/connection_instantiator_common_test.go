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
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

// TestAuthencationFactoryWithNilParameters verifies nil configuration is rejected.
func TestAuthencationFactoryWithNilParameters(t *testing.T) {
	t.Parallel()

	auth, err := GetAuthenticator(nil, nil, driverCommon.ConnectionNormal)
	if err == nil {
		t.Fatalf("get authenticator test failed %v", err)
	}
	if auth != nil {
		t.Fatalf("Authenticator should be null")
	}
}

// TestAuthencationFactoryBasic verifies username/password configuration creates an authenticator.
func TestAuthencationFactoryBasic(t *testing.T) {
	t.Parallel()
	c := oracleconfig.NewOracleDriverConfig()
	c.Credentials.User = "foo"
	c.Credentials.Password = "bar"
	c.ConnectDescriptor = "connection string"
	auth, err := GetAuthenticator(c, nil, driverCommon.ConnectionNormal)
	if err != nil {
		t.Fatalf("get authenticator test failed %v", err)
	}
	if auth == nil {
		t.Errorf("should have returned an authenticator")
	}
}

// TestConnectionSetupAppliesLobPrefetchAfterNegotiation verifies that the configured
// LOB prefetch size is applied to marshallers created before authentication setup.
func TestConnectionSetupAppliesLobPrefetchAfterNegotiation(t *testing.T) {
	t.Skip("TODO: apply LOB prefetch configuration after connection properties become available")

	negotiation := &connectionNegotiatorCommon{_dataBuffer: NewArrayDataBuffer(128)}
	shelf, sessionContext, _, _ := negotiation.initializeNegotiationResources()
	config := oracleconfig.NewOracleDriverConfig()
	config.DriverProperties.DefaultLobPrefetchSize = 8192
	setup := &connectionInstantiatorCommon{
		ns:            &mockNetworkSession{},
		driverConfig:  config,
		authenticator: &mockAuthenticator{},
	}

	setup.prepareForAuthentication(sessionContext, shelf)

	for _, messageType := range []driverCommon.MessageType{TTIFUN, TTIPRO} {
		marshaller := shelf.GetMarshallerForMessageType(messageType).(*MarshalEngine)
		if got := marshaller._defaultLobPrefetchSize; got != 8192 {
			t.Errorf("message %v: LOB prefetch size = %d, want 8192", messageType, got)
		}
	}
}
