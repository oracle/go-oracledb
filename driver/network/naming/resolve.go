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
	"fmt"
	"net"

	"github.com/oracle/go-driver/driver/common"
)

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

// resolveAddress resolves a single Address.
//
// If Host is a hostname, it is resolved via DNS and expanded to one Address per resolved IP.
// If Host is already an IP literal, it bypasses DNS and sets ResolvedIP=Host.
//
// The caller is responsible for setting any overall timeout on ctx before calling this.
func resolveAddress(ctx context.Context, address Address) ([]Address, error) {
	return resolveAddressWithLookup(ctx, address, net.DefaultResolver.LookupIPAddr)
}

// resolveAddressWithLookup is identical to resolveAddress but takes an explicit DNS lookup
// function. This is used by tests to avoid real DNS/network calls without mutating global state.
func resolveAddressWithLookup(ctx context.Context, address Address, lookupIPAddr lookupIPAddrFunc) ([]Address, error) {
	if err := ctx.Err(); err != nil {
		// Context cancelled/expired: stop resolution.
		return nil, err
	}

	// Check if Host is already an IP address - bypass DNS entirely.
	if net.ParseIP(address.Host) != nil {
		address.ResolvedIP = address.Host
		return []Address{address}, nil
	}

	// Perform DNS lookup, relying on the caller's ctx for timeout/cancellation.
	ips, err := lookupIPAddr(ctx, address.Host)
	if err != nil {
		common.Odl.Debug(fmt.Sprintf("DNS resolution failed for host '%s': %v", address.Host, err))
		return []Address{address}, nil
	}

	if len(ips) == 0 {
		common.Odl.Debug(fmt.Sprintf("DNS resolution returned no IPs for host '%s'", address.Host))
		return []Address{address}, nil
	}

	// Create one Address entry per resolved IP.
	resolved := make([]Address, 0, len(ips))
	for _, ip := range ips {
		newAddr := address
		newAddr.ResolvedIP = ip.IP.String()
		resolved = append(resolved, newAddr)
		common.Odl.Debug(fmt.Sprintf("Resolved '%s' to IP '%s'", address.Host, newAddr.ResolvedIP))
	}

	return resolved, nil
}

// GetDNSErrors returns any DNS resolution errors that occurred during iterator creation.
// Currently always returns nil.
/*
func (ci *ConnectionIterator) GetDNSErrors() []error {
	return ci.dnsErrors
}
*/
