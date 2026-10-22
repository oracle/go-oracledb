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
	"reflect"
	"testing"

	driverCommon "github.com/oracle/go-oracledb/v26/internal/driver/common"
)

// markDownHostsForTest records unique keys in the process-wide cache and
// removes them once the test completes.
func markDownHostsForTest(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		MarkDownHost(key)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			sharedDownHostCache.Remove(key)
		}
	})
}

// TestNewConnectionIterator_PrioritizesUncachedHosts verifies that new
// iterators try healthy addresses before cached-down addresses, while
// preserving the original order within each group.
func TestNewConnectionIterator_PrioritizesUncachedHosts(t *testing.T) {
	markDownHostsForTest(t, "192.0.2.2", "192.0.2.4")

	// Direct IP addresses keep this test local: no DNS lookup is required.
	connectionContext := &ConnectionContext{
		Addresses: []Address{
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.1", Port: 1521},
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.2", Port: 1521},
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.3", Port: 1521},
			{Protocol: driverCommon.ProtocolTCP, Host: "192.0.2.4", Port: 1521},
		},
	}
	iter := NewConnectionIterator(context.Background(), nil, connectionContext)

	var got []string
	for iter.HasNext() {
		got = append(got, iter.Next().Address.Host)
	}
	want := []string{"192.0.2.1", "192.0.2.3", "192.0.2.2", "192.0.2.4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connection order = %v, want %v", got, want)
	}
}

// TestConnectionIterator_UsesResolvedIPForDownHostCache verifies that cache
// status is shared by the resolved endpoint, rather than its hostname.
func TestConnectionIterator_UsesResolvedIPForDownHostCache(t *testing.T) {
	const (
		host      = "scan.example.com"
		downIP    = "198.51.100.10"
		healthyIP = "198.51.100.11"
	)
	markDownHostsForTest(t, downIP)

	addresses := []Address{
		{Host: host, ResolvedIP: downIP},
		{Host: host, ResolvedIP: healthyIP},
	}
	(&ConnectionIterator{}).reorderAddressesByDownHostStatus(addresses)

	if got := addresses[0].ResolvedIP; got != healthyIP {
		t.Fatalf("first resolved IP = %q, want %q", got, healthyIP)
	}
}

// TestConnectionIterator_ReordersDescriptionsWithOnlyDownHosts verifies that
// only a description whose every address is cached down moves behind other
// descriptions.
func TestConnectionIterator_ReordersDescriptionsWithOnlyDownHosts(t *testing.T) {
	markDownHostsForTest(t, "198.51.100.21", "198.51.100.22", "198.51.100.23")
	iter := &ConnectionIterator{}
	attempts := []DescriptionAttempts{
		{Addresses: []Address{{Host: "down-1", ResolvedIP: "198.51.100.21"}, {Host: "down-2", ResolvedIP: "198.51.100.22"}}},
		{Addresses: []Address{{Host: "down-3", ResolvedIP: "198.51.100.23"}, {Host: "healthy-1", ResolvedIP: "198.51.100.24"}}},
		{Addresses: []Address{{Host: "healthy-2", ResolvedIP: "198.51.100.25"}}},
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

// TestConnectionIterator_ReappliesDownHostOrdering verifies that retry cycles
// and Reset refresh ordering when cache state changes after iterator creation.
func TestConnectionIterator_ReappliesDownHostOrdering(t *testing.T) {
	// A retry cycle should use current cache state rather than its initial order.
	t.Run("retry cycle", func(t *testing.T) {
		iter := &ConnectionIterator{
			descAttempts: []DescriptionAttempts{{
				Addresses: []Address{
					{Protocol: driverCommon.ProtocolTCP, Host: "host-1", ResolvedIP: "198.51.100.31", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "host-2", ResolvedIP: "198.51.100.32", Port: 1521},
				},
				RetryCount: 1,
			}},
		}

		// The first cycle uses its initial order. Mark host-1 before retrying.
		iter.Next()
		iter.Next()
		markDownHostsForTest(t, "198.51.100.31")

		if got := iter.Next().Address.Host; got != "host-2" {
			t.Fatalf("first retry host = %q, want host-2", got)
		}
	})

	// Reset should move cached-down addresses behind healthy ones in a description.
	t.Run("reset", func(t *testing.T) {
		markDownHostsForTest(t, "198.51.100.41")
		iter := &ConnectionIterator{
			descAttempts: []DescriptionAttempts{{
				Addresses: []Address{
					{Protocol: driverCommon.ProtocolTCP, Host: "host-1", ResolvedIP: "198.51.100.41", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "host-2", ResolvedIP: "198.51.100.42", Port: 1521},
				},
			}},
		}

		iter.Reset()
		if got := iter.Next().Address.Host; got != "host-2" {
			t.Fatalf("first host after reset = %q, want host-2", got)
		}
	})

	// Reset should also move an entirely cached-down description behind a
	// description that retains a healthy address.
	t.Run("reset description list", func(t *testing.T) {
		iter := &ConnectionIterator{
			descAttempts: []DescriptionAttempts{
				// Every address in this description becomes cached down.
				{Addresses: []Address{
					{Protocol: driverCommon.ProtocolTCP, Host: "down-host-1", ResolvedIP: "198.51.100.51", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "down-host-2", ResolvedIP: "198.51.100.52", Port: 1521},
				}},
				// This description stays first because it still has one healthy address.
				{Addresses: []Address{
					{Protocol: driverCommon.ProtocolTCP, Host: "down-host-3", ResolvedIP: "198.51.100.53", Port: 1521},
					{Protocol: driverCommon.ProtocolTCP, Host: "healthy-host", ResolvedIP: "198.51.100.54", Port: 1521},
				}},
			},
		}

		// Cache state changes after the iterator is built.
		markDownHostsForTest(t, "198.51.100.51", "198.51.100.52", "198.51.100.53")
		iter.Reset()

		var got []string
		for iter.HasNext() {
			got = append(got, iter.Next().Address.Host)
		}
		want := []string{"healthy-host", "down-host-3", "down-host-1", "down-host-2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("connection order after reset = %v, want %v", got, want)
		}
	})
}
