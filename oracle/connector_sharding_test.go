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

package oracle

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
	"github.com/oracle/go-oracledb/v26/internal/driver/network/naming"
	internalSharding "github.com/oracle/go-oracledb/v26/internal/sharding"
	shardingEncoding "github.com/oracle/go-oracledb/v26/internal/sharding/encoding"
	oracleconfig "github.com/oracle/go-oracledb/v26/oracle/config"
	oracleProviders "github.com/oracle/go-oracledb/v26/oracle/providers"
	"github.com/oracle/go-oracledb/v26/oracle/sharding"
)

func testShardKey(t *testing.T, value string) sharding.ShardingKey {
	t.Helper()
	k, e := sharding.NewKey(sharding.Subkey{Type: sharding.VARCHAR, Value: value})
	if e != nil {
		t.Fatal(e)
	}
	return k
}

type routingExpectationKey uint8

const testExpectedRouting routingExpectationKey = 0

// TestConnectorConcurrentShardingIsolation verifies concurrent keyed and keyless
// Connect calls keep routing contexts separate without mutating connection options.
func TestConnectorConcurrentShardingIsolation(t *testing.T) {
	cfg := newConnectorTestConfig(t)
	c, err := newOracleConnector(cfg, NewOracleDriverConfig(),
		func(ctx context.Context, o *naming.ConnectionOption, id string) (driverCommon.NetworkSession, error) {
			// Keys reach the network creator through ctx, not a modified option.
			if strings.Contains(o.ConnectString, "SHARDING_KEY") {
				return nil, errors.New("connector mutated connect data")
			}
			encoded, _ := internalSharding.Keys[sharding.ShardingKey](ctx)
			want, _ := ctx.Value(testExpectedRouting).(string)
			if encoded == nil {
				if want != "" {
					return nil, errors.New("lost routing context")
				}
			} else {
				got, err := shardingEncoding.EncodeKey(*encoded)
				if err != nil {
					return nil, err
				}
				if string(got) != want {
					return nil, errors.New("request keys contaminated")
				}
			}
			return &mockConnectorNetworkSession{}, nil
		},
		func(*oracleconfig.OracleDriverConfig, driverCommon.NetworkSession, common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error) {
			return mockConnectorConnectionInstantiator{conn: mockConnectorDriverConn{}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		ctx := context.Background()
		if i != 0 {
			ctx, err = sharding.WithSharding(ctx, sharding.ShardingRequest{ShardingKey: testShardKey(t, fmt.Sprint(i))})
			if err != nil {
				t.Fatal(err)
			}
		}
		if encoded, _ := internalSharding.Keys[sharding.ShardingKey](ctx); encoded != nil {
			expected, e := shardingEncoding.EncodeKey(*encoded)
			if e != nil {
				t.Fatal(e)
			}
			ctx = context.WithValue(ctx, testExpectedRouting, string(expected))
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, e := c.Connect(ctx)
			if e != nil {
				t.Error(e)
				return
			}
			conn.Close()
		}()
	}
	wg.Wait()
}

// TestConnectorShardingRetryAndCleanup verifies retries retain routing keys,
// authentication setup failure closes the network session, and cancellation prevents dialing.
func TestConnectorShardingRetryAndCleanup(t *testing.T) {
	cfg := newConnectorTestConfigWithConnectString(t, "(DESCRIPTION=(RETRY_COUNT=1)(RETRY_DELAY=0)(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=test)))")
	attempts := 0
	ns := &mockConnectorNetworkSession{}
	authErr := errors.New("authentication failed")
	c, err := newOracleConnector(cfg, NewOracleDriverConfig(),
		func(ctx context.Context, o *naming.ConnectionOption, id string) (driverCommon.NetworkSession, error) {
			attempts++
			if key, _ := internalSharding.Keys[sharding.ShardingKey](ctx); key == nil {
				t.Error("retry lost routing")
			}
			if attempts == 1 {
				return nil, errors.New("dial failed")
			}
			return ns, nil
		}, func(*oracleconfig.OracleDriverConfig, driverCommon.NetworkSession, common.Registry[oracleProviders.Provider]) (driverCommon.ConnectionInstantiator, error) {
			return nil, authErr
		})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := sharding.WithSharding(context.Background(), sharding.ShardingRequest{ShardingKey: testShardKey(t, "abc")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Connect(ctx)
	if err == nil || attempts != 2 || !ns.disconnectCalled {
		t.Fatalf("attempts=%d cleanup=%v err=%v", attempts, ns.disconnectCalled, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	attempts = 0
	_, err = c.Connect(cancelled)
	if err == nil || attempts != 0 {
		t.Fatalf("cancelled request dialed: %d %v", attempts, err)
	}
}

var _ driver.Connector = (*connector)(nil)
