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

package driver

import (
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/ttc"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

// Only the UUID is needed for selection; no network or buffer operations are expected.
type factoryTestNetworkSession struct {
	driverCommon.NetworkSession
	driverCommon.DataBuffer
	databaseUUID string
}

func (s *factoryTestNetworkSession) GetDatabaseUUID() string {
	return s.databaseUUID
}

// TestGetConnectionInstantiatorCacheHit verifies that an eligible connection
// with a cached cookie selects the optimized instantiator.
func TestGetConnectionInstantiatorCacheHit(t *testing.T) {
	databaseUUID := t.Name()
	cleanup := ttc.SeedTTCCookieForTest(databaseUUID)
	t.Cleanup(cleanup)

	config := oracleconfig.NewOracleDriverConfig()
	config.Credentials.User = "test-user"
	config.Credentials.Password = "test-password"
	config.DriverProperties.TTCCookieOptimizationEnabled = true
	ns := &factoryTestNetworkSession{databaseUUID: databaseUUID}

	got, err := GetConnectionInstantiator(config, ns, nil, false)
	if err != nil {
		t.Fatalf("get connection instantiator: %v", err)
	}

	if got == nil {
		t.Fatal("expected an instantiator, got nil")
	}
	wantType := "*ttc.cookieConnectionInstantiator"
	if gotType := reflect.TypeOf(got).String(); gotType != wantType {
		t.Fatalf("instantiator type = %s, want %s", gotType, wantType)
	}
}

// TestGetConnectionInstantiatorSelectsNormal verifies the cases that must not
// select the cookie instantiator. The constructor's mode wiring is tested in TTC.
func TestGetConnectionInstantiatorSelectsNormal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enabled    bool
		seedCookie bool
		reneg      bool
	}{
		{
			name:    "cache miss returns normal",
			enabled: true,
		},
		{
			name:       "optimization disabled returns normal despite cached cookie",
			seedCookie: true,
		},
		{
			name:       "renegotiation bypasses cached cookie", // this means that we have  the standard instantiator with reneg flag
			enabled:    true,
			seedCookie: true,
			reneg:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			databaseUUID := t.Name()
			if tc.seedCookie {
				t.Cleanup(ttc.SeedTTCCookieForTest(databaseUUID))
			}

			config := oracleconfig.NewOracleDriverConfig()
			config.Credentials.User = "test-user"
			config.Credentials.Password = "test-password"
			config.DriverProperties.TTCCookieOptimizationEnabled = tc.enabled
			ns := &factoryTestNetworkSession{databaseUUID: databaseUUID}

			got, err := GetConnectionInstantiator(config, ns, nil, tc.reneg)
			if err != nil {
				t.Fatalf("get connection instantiator: %v", err)
			}
			if got == nil {
				t.Fatal("expected an instantiator, got nil")
			}
			wantType := "*ttc.connectionInstantiator"
			if gotType := reflect.TypeOf(got).String(); gotType != wantType {
				t.Fatalf("instantiator type = %s, want %s", gotType, wantType)
			}
		})
	}
}
