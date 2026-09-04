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
	"context"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
)

type factoryTestNetworkSession struct{}

func (factoryTestNetworkSession) WriteByteWithContext(context.Context, byte) error { return nil }

func (factoryTestNetworkSession) WriteBytesWithContext(context.Context, []byte) error { return nil }

func (factoryTestNetworkSession) ReadByteWithContext(context.Context) (byte, error) { return 0, nil }

func (factoryTestNetworkSession) ReadBytesWithContext(_ context.Context, n int32) (*[]byte, error) {
	buf := make([]byte, int(n))
	return &buf, nil
}

func (factoryTestNetworkSession) Flush(context.Context) error { return nil }

func (factoryTestNetworkSession) Disconnect(context.Context, int) error { return nil }

func (factoryTestNetworkSession) CancelOperation(context.Context) error { return nil }

func (factoryTestNetworkSession) CheckInbandNotification() bool { return false }

func (factoryTestNetworkSession) GetRemoteAddress() string { return "" }

func (factoryTestNetworkSession) GetRemotePort() int { return 0 }

// TestGetConnectionInstantiator verifies that a valid driver configuration and
// network session produce a usable connection instantiator.
func TestGetConnectionInstantiator(t *testing.T) {
	cfg := oracleconfig.NewOracleDriverConfig()
	cfg.ConnectDescriptor = "localhost:1521/service"
	cfg.Credentials.User = "scott"
	cfg.Credentials.Password = "tiger"

	var ns driverCommon.NetworkSession = factoryTestNetworkSession{}
	instantiator, err := GetConnectionInstantiator(cfg, ns, common.NewProviderRegistry())
	if err != nil {
		t.Fatalf("GetConnectionInstantiator returned error: %v", err)
	}
	if instantiator == nil {
		t.Fatal("GetConnectionInstantiator returned nil")
	}
}

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestGetConnectionInstantiator", Categories: "unitary", Exclusive: false, Fn: TestGetConnectionInstantiator},
}

func TestCategoryExecutor(t *testing.T) {
	oracleTest.RunCategoryExecutor(t, oracleTest.TestCategory, testCases)
}
