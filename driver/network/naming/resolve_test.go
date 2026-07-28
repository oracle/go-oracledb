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

package naming

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/oracle/go-driver/driver/common"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// neverCalled returns a DNS lookup stub that fails the test if invoked.
//
// This is used to assert branches where resolveAddresses must not perform DNS
// resolution (when the host is already an IP literal, or when ctx is
// already canceled and we exit early).
func neverCalled(t *testing.T) func(context.Context, string) ([]net.IPAddr, error) {
	t.Helper()
	return func(_ context.Context, host string) ([]net.IPAddr, error) {
		t.Fatalf("dnsLookupIPAddr must not be called (host=%q)", host)
		return nil, nil
	}
}

// makeIPs converts a list of string IPs into the net.IPAddr format returned by
// net.DefaultResolver.LookupIPAddr.
func makeIPs(addrs ...string) []net.IPAddr {
	out := make([]net.IPAddr, len(addrs))
	for i, a := range addrs {
		out[i] = net.IPAddr{IP: net.ParseIP(a)}
	}
	return out
}

// tcpAddr creates an Address used by tests. The port value is irrelevant for DNS
// resolution behavior, but Address requires it
func tcpAddr(host string) Address {
	return Address{Protocol: common.ProtocolTCP, Host: host, Port: 1521}
}

func TestResolveAddresses_DNSExpansion(t *testing.T) {
	t.Parallel()
	// One hostname → 3 IPs → 3 separate entries.
	// Direct IP passes through untouched. Total = 1 + 3 = 4 entries.
	in := []Address{tcpAddr("1.2.3.4"), tcpAddr("scan.test")}
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "scan.test" {
			return makeIPs("10.0.0.1", "10.0.0.2", "10.0.0.3"), nil
		}
		return nil, fmt.Errorf("unexpected host: %s", host)
	}

	resolved := make([]Address, 0, len(in)*2)
	for _, a := range in {
		expanded, err := resolveAddressWithLookup(context.Background(), a, lookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolved = append(resolved, expanded...)
	}

	wantIPs := []string{"1.2.3.4", "10.0.0.1", "10.0.0.2", "10.0.0.3"}
	gotIPs := make([]string, len(resolved))
	for i, a := range resolved {
		gotIPs[i] = a.ResolvedIP
	}
	if !reflect.DeepEqual(gotIPs, wantIPs) {
		t.Fatalf("want %v, got %v", wantIPs, gotIPs)
	}
	// Original hostname must be preserved on every expanded entry
	for _, a := range resolved[1:] {
		if a.Host != "scan.test" {
			t.Fatalf("expected Host=scan.test preserved, got %q", a.Host)
		}
	}
}

// ── Unit tests: resolveAddresses ──────────────────────────────────────────────

// canceled context should returns early without DNS call
func TestResolveAddresses_ContextHandling(t *testing.T) {
	t.Parallel()
	t.Run("canceled context returns early without DNS call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		lookup := neverCalled(t)

		resolved, err := resolveAddressWithLookup(ctx, tcpAddr("host.test"), lookup)
		if len(resolved) != 0 || !errors.Is(err, context.Canceled) {
			t.Fatalf("expected empty resolved + context.Canceled, got resolved=%v err=%v", resolved, err)
		}
	})
}

func TestResolveAddresses_HostClassification(t *testing.T) {
	t.Parallel()
	// Verifies that resolveAddress correctly categorizes inputs:
	//   - IP literals bypass DNS and populate ResolvedIP
	t.Run("direct IP bypasses DNS", func(t *testing.T) {
		lookup := neverCalled(t)
		resolved, err := resolveAddressWithLookup(context.Background(), tcpAddr("192.168.1.1"), lookup)
		if err != nil || resolved[0].ResolvedIP != "192.168.1.1" {
			t.Fatalf("unexpected result: resolved=%v err=%v", resolved, err)
		}
	})

	for _, tc := range []struct {
		name string
		stub func(context.Context, string) ([]net.IPAddr, error)
	}{
		{"dns error falls back to hostname", func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return nil, errors.New("dns failure")
		}},
		{"empty result falls back to hostname", func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := resolveAddressWithLookup(context.Background(), tcpAddr("fallback.test"), tc.stub)
			if err != nil || len(resolved) != 1 || resolved[0].Host != "fallback.test" || resolved[0].ResolvedIP != "" {
				t.Fatalf("expected hostname fallback, got resolved=%v err=%v", resolved, err)
			}
		})
	}
}
