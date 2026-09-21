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
** Software as contributed to or provided as such to this licensor, or (ii) the
** Larger Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors), and
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or substantial
** portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package naming

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

func TestNewConnectionIterator_PrioritizesUncachedHosts(t *testing.T) {
	t.Parallel()

	cache := common.NewSafeTTLCache[struct{}](4, time.Hour)
	cache.Put("192.0.2.2", struct{}{})
	cache.Put("192.0.2.4", struct{}{})

	// Direct IP addresses keep this test local: no DNS lookup is required.
	connectionContext := &ConnectionContext{
		Addresses: []Address{
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.1", Port: 1521},
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.2", Port: 1521},
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.3", Port: 1521},
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.4", Port: 1521},
		},
	}
	iter := newConnectionIterator(context.Background(), nil, connectionContext, cache)

	var got []string
	for iter.HasNext() {
		got = append(got, iter.Next().Address.Host)
	}
	want := []string{"192.0.2.1", "192.0.2.3", "192.0.2.2", "192.0.2.4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connection order = %v, want %v", got, want)
	}
}

func TestMarkDownHost_RecordsHost(t *testing.T) {
	t.Parallel()

	// A unique key lets this test use the shared cache without affecting
	// ordering assertions in other parallel tests.
	host := fmt.Sprintf("down-host-cache-test-%d", time.Now().UnixNano())
	MarkDownHost(host)

	if _, found := sharedDownHostCache.Get(host); !found {
		t.Fatalf("host %q was not recorded in the shared down-host cache", host)
	}
}

func TestConnectionIterator_ReordersDescriptionsWithOnlyDownHosts(t *testing.T) {
	t.Parallel()

	cache := common.NewSafeTTLCache[struct{}](4, time.Hour)
	for _, host := range []string{"down-1", "down-2", "down-3"} {
		cache.Put(host, struct{}{})
	}
	iter := &ConnectionIterator{downHostCache: cache}
	attempts := []DescriptionAttempts{
		{Addresses: []Address{{Host: "down-1"}, {Host: "down-2"}}},
		{Addresses: []Address{{Host: "down-3"}, {Host: "healthy-1"}}},
		{Addresses: []Address{{Host: "healthy-2"}}},
	}

	iter.reorderDescriptionsByDownHostStatus(attempts)

	got := []string{
		attempts[0].Addresses[0].Host,
		attempts[1].Addresses[0].Host,
		attempts[2].Addresses[0].Host,
	}
	want := []string{"down-3", "healthy-2", "down-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("description order = %v, want %v", got, want)
	}
}

func TestConnectionIterator_ReappliesDownHostOrdering(t *testing.T) {
	t.Parallel()

	t.Run("retry cycle", func(t *testing.T) {
		cache := common.NewSafeTTLCache[struct{}](2, time.Hour)
		iter := &ConnectionIterator{
			downHostCache: cache,
			descAttempts: []DescriptionAttempts{{
				Addresses: []Address{
					{Protocol: driverCommon.ProtocolTCP, Host: "host-1", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "host-2", Port: 1521},
				},
				RetryCount: 1,
			}},
		}

		// The first cycle uses its initial order. Mark host-1 before retrying.
		iter.Next()
		iter.Next()
		cache.Put("host-1", struct{}{})

		if got := iter.Next().Address.Host; got != "host-2" {
			t.Fatalf("first retry host = %q, want host-2", got)
		}
	})

	t.Run("reset", func(t *testing.T) {
		cache := common.NewSafeTTLCache[struct{}](2, time.Hour)
		cache.Put("host-1", struct{}{})
		iter := &ConnectionIterator{
			downHostCache: cache,
			descAttempts: []DescriptionAttempts{{
				Addresses: []Address{
					{Protocol: driverCommon.ProtocolTCP, Host: "host-1", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "host-2", Port: 1521},
				},
			}},
		}

		iter.Reset()
		if got := iter.Next().Address.Host; got != "host-2" {
			t.Fatalf("first host after reset = %q, want host-2", got)
		}
	})
}
